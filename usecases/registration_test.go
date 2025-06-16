package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type mockTransport struct {
	returnError bool
	respFunc    func() *http.Response
	hits        int
	err         error
}

func newMockTransport(respFunc func() *http.Response, v int, err error) mockTransport {
	t := mockTransport{
		hits:     v,
		respFunc: respFunc,
		err:      err,
	}
	http.DefaultClient.Transport = t
	return t
}

func (t mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.hits -= 1
	if t.hits == 0 {
		return nil, t.err
	}
	resp := t.respFunc()
	resp.Request = req
	return resp, nil
}

type timeoutError struct{}

type errorReadCloser struct{}

func (e *errorReadCloser) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (e *errorReadCloser) Close() error {
	return errors.New("Forced close error")
}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var errHTTP error = fmt.Errorf("bad http request")

var brokenUrl = string([]byte{0x7f})

type UnitAsset struct {
	Name        string              `json:"name"`    // Must be a unique name, ie. a sensor ID
	Owner       *components.System  `json:"-"`       // The parent system this UA is part of
	Details     map[string][]string `json:"details"` // Metadata or details about this UA
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	test int `json:"-"`
}

type ServiceRecord_v1 struct {
	Id                int                 `json:"registryID"`
	ServiceDefinition string              `json:"definition"`
	SystemName        string              `json:"systemName"`
	ServiceNode       string              `json:"serviceNode"`
	IPAddresses       []string            `json:"ipAddresses"`
	ProtoPort         map[string]int      `json:"protoPort"`
	Details           map[string][]string `json:"details"`
	Certificate       string              `json:"certificate"`
	SubPath           string              `json:"subpath"`
	RegLife           int                 `json:"registrationLife"`
	Version           string              `json:"version"`
	Created           string              `json:"created"`
	Updated           string              `json:"updated"`
	EndOfValidity     string              `json:"endOfValidity"`
	SubscribeAble     bool                `json:"subscribeAble"`
	ACost             float64             `json:"activityCost"`
	CUnit             string              `json:"costUnit"`
}

func (mua *UnitAsset) GetName() string {
	return mua.Name
}

func (mua *UnitAsset) GetServices() components.Services {
	return mua.ServicesMap
}

func (mua *UnitAsset) GetCervices() components.Cervices {
	return mua.CervicesMap
}

func (mua *UnitAsset) GetDetails() map[string][]string {
	return mua.Details
}

func (mua *UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	return
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("forced read error")
}

func createTestSystem(ctx context.Context) components.System {
	sys := components.NewSystem("testSystem", ctx)

	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
	}

	orchestrator := &components.CoreSystem{
		Name: "orchestrator",
		Url:  "https://orchestrator",
	}
	leadingRegistrar := &components.CoreSystem{
		Name: "serviceregistrar",
		Url:  "https://leadingregistrar",
	}
	test := &components.CoreSystem{
		Name: "test",
		Url:  "https://test",
	}
	sys.CoreS = []*components.CoreSystem{
		orchestrator,
		leadingRegistrar,
		test,
	}

	setTest := &components.Service{
		ID:            1,
		Definition:    "test",
		SubPath:       "test",
		Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
		Description:   "A test service",
		RegPeriod:     45,
		RegTimestamp:  "now",
		RegExpiration: "45",
	}
	ServicesMap := &components.Services{
		setTest.SubPath: setTest,
	}
	mua := &UnitAsset{
		Name:        "mockUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	var muaInterface components.UnitAsset = mua
	sys.UAssets[mua.GetName()] = &muaInterface

	return sys
}

type confirmLeadingRegistrarParams struct {
	testCase         string
	mockTransportErr int
	errHTTP          error
	expectedOutput   *components.CoreSystem
}

func createTestBody(testCase string) (respFunc func() *http.Response) {
	if testCase == "No errors" {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
			}
		}
		return respFunc
	}
	if testCase == "Read error" {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(errorReader{}),
			}
		}
		return respFunc
	}
	if testCase == "Prefix error" {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("Wrong prefix"))),
			}
		}
		return respFunc
	}
	if testCase == "Broken URL" {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
			}
		}
		return respFunc
	}
	return nil
}

func TestForconfirmLeadingRegistrar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testParams := []confirmLeadingRegistrarParams{
		{"No errors", 0, nil, testSys.CoreS[1]},
		{"Read error", 0, nil, nil},
		{"Prefix error", 0, nil, nil},
		{"Broken URL", 0, nil, nil},
	}

	for _, test := range testParams {
		respFunc := createTestBody(test.testCase)
		if respFunc == nil {
			t.Errorf("---\tError occurred while creating test data")
		}
		newMockTransport(respFunc, test.mockTransportErr, test.errHTTP)
		if test.testCase == "Broken URL" {
			testSys.CoreS[1].Url = brokenUrl
		}

		// Do the test
		res := confirmLeadingRegistrar(testSys.CoreS[1])
		if test.testCase == "No errors" {
			if res != test.expectedOutput {
				t.Errorf("Test case: %s got error: %v", test.testCase, res)
			}
		} else {
			if res != test.expectedOutput {
				t.Errorf("---\tTest case: expected leading registrar to be nil, got: %v", res)
			}
		}
	}
}

type findLeadingRegistrarParams struct {
	testCase         string
	mockTransportErr int
	errHTTP          error
	expectedOutput   *components.CoreSystem
}

func TestForfindLeadingRegistrar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testParams := []findLeadingRegistrarParams{
		{"No errors", 0, nil, testSys.CoreS[1]},
		{"Read error", 0, nil, nil},
		{"Prefix error", 0, nil, nil},
		{"Broken URL", 0, nil, nil},
	}

	for _, test := range testParams {
		respFunc := createTestBody(test.testCase)
		if respFunc == nil {
			t.Errorf("---\tError occurred while creating test data")
		}
		newMockTransport(respFunc, test.mockTransportErr, test.errHTTP)
		if test.testCase == "Broken URL" {
			testSys.CoreS[1].Url = brokenUrl
		}

		// Do the test
		res := findLeadingRegistrar(&testSys, nil)
		if test.testCase == "No errors" {
			if res != test.expectedOutput {
				t.Errorf("Test case: %s got error: %v", test.testCase, res)
			}
		} else {
			if res != test.expectedOutput {
				t.Errorf("---\tTest case: expected leading registrar to be nil, got: %v", res)
			}
		}
	}
}

func TestFordeepCopyMap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	mua := testSys.UAssets["mockUnitAsset"]
	original := (*mua).GetDetails()

	// -- -- -- -- -- -- -- -- -- -- //
	// Create a Deep Copy Map of the mockUnitAsset's Details
	// -- -- -- -- -- -- -- -- -- -- //

	test := deepCopyMap((*mua).GetDetails())

	// -- -- -- -- -- -- -- -- -- -- //
	// If they are not equal from the beginning then the copy was not successful
	// -- -- -- -- -- -- -- -- -- -- //

	if !reflect.DeepEqual(original, test) {
		t.Errorf("Expected deep copied map to be equal to original, Expected: %v, got: %v", original, test)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// When we change something in the original, the deep copied map should not change
	// -- -- -- -- -- -- -- -- -- -- //

	original["Test"][0] = "changed original"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in original affected the deep copied map. Expected: %v, got %v", original, test)
	}
	original["Test"][0] = "test"

	// -- -- -- -- -- -- -- -- -- -- //
	// When we change something in the deep copied map, the original should not change
	// -- -- -- -- -- -- -- -- -- -- //

	test["Test"][0] = "changed deep copy"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in deep copied map affected the original. Expected: %v, got %v", original, test)
	}
}

func TestForserviceRegistrationForm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	mua := testSys.UAssets["mockUnitAsset"]
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]
	version := "ServiceRecord_v1"

	// -- -- -- -- -- -- -- -- -- -- //
	// Call the ServiceRegistrationForm with the correct parameters
	// -- -- -- -- -- -- -- -- -- -- //

	payload, err := serviceRegistrationForm(&testSys, mua, serv, version)

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that there was no error in the function (can only be when wrong Service Record version is sent in)
	// -- -- -- -- -- -- -- -- -- -- //

	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr forms.ServiceRecord_v1
	if err := json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that the ServiceNode is created correctly
	// -- -- -- -- -- -- -- -- -- -- //

	expectedNode := testSys.Host.Name + "_" + testSys.Name + "_" + (*testSys.UAssets["mockUnitAsset"]).GetName() + "_" + (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"].Definition
	if sr.ServiceNode != expectedNode {
		t.Errorf("Expected ServiceNode %q, got: %q", expectedNode, sr.ServiceNode)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that the ProtoPorts that are equal to 0 gets removed
	// -- -- -- -- -- -- -- -- -- -- //

	if len(sr.ProtoPort) != 1 {
		t.Errorf("Expected: one proto port (excluding 0s), got: %v", sr.ProtoPort)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that the unit asset details exists and are ok
	// -- -- -- -- -- -- -- -- -- -- //

	if v, ok := sr.Details["Test"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect unit asset details. Expected: %v, got: %v", (*mua).GetDetails(), v)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that the service forms exists and are ok
	// -- -- -- -- -- -- -- -- -- -- //

	if v, ok := sr.Details["Forms"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect service forms. Expected: %v, got: %v", (*serv).Details, v)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: Sent in version is not supported
	// -- -- -- -- -- -- -- -- -- -- //

	version = "UnknownVersion"
	_, err = serviceRegistrationForm(&testSys, mua, serv, version)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if err.Error() != "unsupported service registration form version" {
		t.Errorf("Expected error: unsupported service registration form version, got: %v", err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that when the Service RegPeriod equals 0, ServiceRegistrationForm defaults to its RegLife default value of 30
	// -- -- -- -- -- -- -- -- -- -- //

	(*testSys.UAssets["mockUnitAsset"]).GetServices()["test"].RegPeriod = 0
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

func TestForderegisterService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	var registrar *components.CoreSystem
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]

	respFunc := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(string("test body"))),
		}
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case: No errors when a service not registered tries to get deregistered
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	err := deregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case: No errors when a service registered tries to get deregistered
	// -- -- -- -- -- -- -- -- -- -- //

	registrar = testSys.CoreS[1]

	err = deregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// bad case: response body error
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 1, errHTTP)
	err = deregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while sending http request")
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// bad case: URL broken
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)
	registrar.Url = brokenUrl
	err = deregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while creating http request")
	}
}

func TestServiceRegistrationFormList(t *testing.T) {
	list := []string{
		"ServiceRecord_v1",
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Check that the return value of ServiceRegistrationFormsList is equal to the expected list of ServiceRegistrationForms
	// -- -- -- -- -- -- -- -- -- -- //

	test := ServiceRegistrationFormsList()
	if !reflect.DeepEqual(list, test) {
		t.Errorf("Expected lists to be equal. Expected: %v, got: %v", list, test)
	}
}

func TestForregisterService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	mua := testSys.UAssets["mockUnitAsset"]
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]
	registrar := testSys.CoreS[1]

	payload, err := serviceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")

	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr forms.ServiceRecord_v1
	if err := json.Unmarshal(payload, &sr); err != nil {
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

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case, everything works, service gets registered
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	test := registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when NewRequest with PUT method fails
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)
	registrar.Url = brokenUrl
	test = registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with PUT method should have failed, got: %d", int(test.Seconds()))
	}

	registrar.Url = "https://leadingregistrar"
	serv.ID = 0

	payload, err = serviceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")

	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	if err := json.Unmarshal(payload, &sr); err != nil {
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

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case when making POST instead
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	test = registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when NewRequest with POST method fails
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	registrar.Url = brokenUrl
	test = registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with POST method should have failed, got: %d", int(test.Seconds()))
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when http.DefaultClient.Do() fails with a err.Timeout()
	// -- -- -- -- -- -- -- -- -- -- //

	registrar.Url = "https://leadingregistrar"
	timeoutErr := timeoutError{}
	newMockTransport(respFunc, 1, timeoutErr)
	test = registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the executed request should fail, got %d", int(test.Seconds()))
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when http.DefaultClient.Do() fails but not with a err.Timeout()
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 1, errHTTP)
	test = registerService(&testSys, mua, serv, registrar)
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

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when io.ReadAll() returns an error
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	test = registerService(&testSys, mua, serv, registrar)
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

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: when Unpack() fails because of a non-existent "Content-Type" in the Header of the response
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	test = registerService(&testSys, mua, serv, registrar)
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

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: Error parsing the EndOfValidity into the RFC3339 time format
	// -- -- -- -- -- -- -- -- -- -- //

	newMockTransport(respFunc, 0, nil)

	test = registerService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the EndOfValidity has a faulty time format, got: %d", int(test.Seconds()))
	}
}
