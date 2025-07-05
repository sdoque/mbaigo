package usecases

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/forms"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("forced read error")
}

func TestDeepCopyMap(t *testing.T) {
	testSys := createTestSystem(false)
	mua := testSys.UAssets["testUnitAsset"]
	original := (*mua).GetDetails()

	// Create a Deep Copy Map of the mockUnitAsset's Details
	test := deepCopyMap((*mua).GetDetails())
	// If they are not equal from the beginning then the copy was not successful
	if !reflect.DeepEqual(original, test) {
		t.Errorf("Expected deep copied map to be equal to original, Expected: %v, got: %v", original, test)
	}

	// When we change something in the original, the deep copied map should not change
	original["Test"][0] = "changed original"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in original affected the deep copied map. Expected: %v, got %v", original, test)
	}
	original["Test"][0] = "test"

	// When we change something in the deep copied map, the original should not change
	test["Test"][0] = "changed deep copy"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in deep copied map affected the original. Expected: %v, got %v", original, test)
	}
}

func TestServiceRegistrationForm(t *testing.T) {
	testSys := createTestSystem(false)
	mua := testSys.UAssets["testUnitAsset"]
	serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]
	version := "ServiceRecord_v1"

	// Call the ServiceRegistrationForm with the correct parameters
	payload, err := serviceRegistrationForm(&testSys, mua, serv, version)
	// Check that there was no error in the function (can only be when wrong Service Record version is sent in)
	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}
	var sr forms.ServiceRecord_v1
	if err = json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Check that the ServiceNode is created correctly
	expectedNode := testSys.Host.Name + "_" + testSys.Name + "_" +
		(*testSys.UAssets["testUnitAsset"]).GetName() + "_" +
		(*testSys.UAssets["testUnitAsset"]).GetServices()["test"].Definition
	if sr.ServiceNode != expectedNode {
		t.Errorf("Expected ServiceNode %q, got: %q", expectedNode, sr.ServiceNode)
	}

	// Check that the ProtoPorts that are equal to 0 gets removed
	if len(sr.ProtoPort) != 1 {
		t.Errorf("Expected: one proto port (excluding 0s), got: %v", sr.ProtoPort)
	}

	// Check that the unit asset details exists and are ok
	if v, ok := sr.Details["Test"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect unit asset details. Expected: %v, got: %v", (*mua).GetDetails(), v)
	}

	// Check that the service forms exists and are ok
	if v, ok := sr.Details["Forms"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect service forms. Expected: %v, got: %v", (*serv).Details, v)
	}

	// Bad case: Sent in version is not supported
	version = "UnknownVersion"
	_, err = serviceRegistrationForm(&testSys, mua, serv, version)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if err.Error() != "unsupported service registration form version" {
		t.Errorf("Expected error: unsupported service registration form version, got: %v", err)
	}

	// Check that when the Service RegPeriod equals 0, ServiceRegistrationForm defaults to its RegLife default value of 30
	(*testSys.UAssets["testUnitAsset"]).GetServices()["test"].RegPeriod = 0
	version = "ServiceRecord_v1"
	payload, err = serviceRegistrationForm(&testSys, mua, serv, version)
	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	if err := json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if sr.RegLife != 30 {
		t.Errorf("Expected RegLife: 30, got: %d", sr.RegLife)
	}
}

func TestUnregisterService(t *testing.T) {
	testSys := createTestSystem(false)
	registrar := testSys.CoreS[0].Url
	serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]
	respFunc := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(string("test body"))),
		}
	}

	// Good case: No errors when a service not registered tries to get deregistered
	newMockTransport(respFunc, 0, nil)
	err := unregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	// Good case: No errors when a service registered tries to get deregistered
	err = unregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	// bad case: response body error
	newMockTransport(respFunc, 1, errHTTP)
	err = unregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while sending http request")
	}

	// bad case: URL broken
	newMockTransport(respFunc, 0, nil)
	registrar = brokenUrl
	err = unregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while creating http request")
	}
}

func TestServiceRegistrationFormList(t *testing.T) {
	list := []string{
		"ServiceRecord_v1",
	}
	// Check that the return value of ServiceRegistrationFormsList is equal to the expected list of ServiceRegistrationForms
	test := ServiceRegistrationFormsList()
	if !reflect.DeepEqual(list, test) {
		t.Errorf("Expected lists to be equal. Expected: %v, got: %v", list, test)
	}
}

func TestRegisterService(t *testing.T) {
	testSys := createTestSystem(false)
	registrar := testSys.CoreS[0].Url
	mua := testSys.UAssets["testUnitAsset"]
	serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]

	payload, err := serviceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")
	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}
	var sr forms.ServiceRecord_v1
	if err = json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	sr.EndOfValidity = time.Now().Format(time.RFC3339)
	fakeBody, err := json.Marshal(sr)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}

	respFunc := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}

	// Good case, everything works, service gets registered
	newMockTransport(respFunc, 0, nil)
	test, err := registerService(&testSys, registrar, mua, serv)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}

	// Bad case: when NewRequest with PUT method fails
	newMockTransport(respFunc, 0, nil)
	registrar = brokenUrl
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with PUT method should have failed, got: %d", int(test.Seconds()))
	}
	registrar = testSys.CoreS[0].Url
	serv.ID = 0

	payload, err = serviceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")
	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	if err = json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	sr.EndOfValidity = time.Now().Format(time.RFC3339)
	fakeBody, err = json.Marshal(sr)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}
	respFunc = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}

	// Good case when making POST instead
	newMockTransport(respFunc, 0, nil)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}

	// Bad case: when NewRequest with POST method fails
	newMockTransport(respFunc, 0, nil)
	registrar = brokenUrl
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with POST method should have failed, got: %d", int(test.Seconds()))
	}
	registrar = testSys.CoreS[0].Url

	// Bad case: when http.DefaultClient.Do() fails with a err.Timeout()
	timeoutErr := timeoutError{}
	newMockTransport(respFunc, 1, timeoutErr)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the executed request should fail, got %d", int(test.Seconds()))
	}

	// Bad case: when http.DefaultClient.Do() fails but not with a err.Timeout()
	newMockTransport(respFunc, 1, errHTTP)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the executed request should fail, got %d", int(test.Seconds()))
	}

	respFunc = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(errorReader{}),
		}
	}

	// Bad case: when io.ReadAll() returns an error
	newMockTransport(respFunc, 0, nil)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the io.ReadAll() call should fail, got %d", int(test.Seconds()))
	}

	respFunc = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}

	// Bad case: when Unpack() fails because of a non-existent "Content-Type" in the Header of the response
	newMockTransport(respFunc, 0, nil)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the Header had a non-existent/invalid Content-Type, got: %d", int(test.Seconds()))
	}

	sr.EndOfValidity = ""
	fakeBody, err = json.Marshal(sr)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}
	respFunc = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}

	// Bad case: Error parsing the EndOfValidity into the RFC3339 time format
	newMockTransport(respFunc, 0, nil)
	test, _ = registerService(&testSys, registrar, mua, serv)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the EndOfValidity has a faulty time format, got: %d", int(test.Seconds()))
	}
}
