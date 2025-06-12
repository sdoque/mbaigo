package usecases

import (
	//"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	//"sync"
	"time"

	//"encoding/json"
	//"errors"
	"fmt"
	//"io"
	//"log"
	//"net"
	"net/http"
	"net/http/httptest"

	//"strconv"
	//"strings"
	"testing"
	//"time"
	"reflect"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	//"github.com/sdoque/mbaigo/forms"
)

type mockTransport struct {
	returnError bool
	resp        *http.Response
	hits        map[string]int
	err         error
}

func newMockTransport(resp *http.Response, retErr bool, err error) mockTransport {
	t := mockTransport{
		returnError: retErr,
		resp:        resp,
		err:         err,
	}
	http.DefaultClient.Transport = t
	return t
}

func (t mockTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if t.err != nil {
		return nil, t.err
	}
	if t.returnError != false {
		req.GetBody = func() (io.ReadCloser, error) {
			return nil, errHTTP
		}
	}
	t.resp.Request = req
	return t.resp, nil
}

type timeoutError struct{}

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

func TestDiscoverLeadingRegistrar(t *testing.T) {
	statusCode := http.StatusOK
	responseBody := "lead Service Registrar since"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
	defer ts.Close()
	testURL := ts.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testSys.CoreS[1].Url = testURL

	manualTicker := 10 * time.Millisecond

	resultCh := make(chan *components.CoreSystem, 1)
	defer close(resultCh)
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res.Name != "serviceregistrar" {
			t.Errorf("Expected %s, got: %s", "serviceregistrar", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	statusCode = http.StatusOK
	responseBody = "wrong response"
	testURL = ts.URL
	testSys.CoreS[1].Url = testURL
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res != nil {
			t.Errorf("Expected %s, got: %s", "leadingRegistrar be nil", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	statusCode = http.StatusOK
	responseBody = "lead Service Registrar since"
	testURL = ts.URL
	testSys.CoreS[1].Url = testURL
	ts.Close()
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res != nil {
			t.Errorf("Expected %s, got: %s", "leadingRegistrar be nil since Get() error", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	/*
		result := DiscoverLeadingRegistrar(&testSys, testURL, false)
		if result != true {
			t.Errorf("Expected %t, got: %t", true, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "lead Service Registrar since"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusOK
		responseBody = "wrong response"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "wrong response"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusOK
		responseBody = "lead Service Registrar since"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, true)
		if result != true {
			t.Errorf("Expected %t, got: %t", true, result)
		}

		statusCode = http.StatusOK
		responseBody = "wrong response"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "lead Service Registrar since"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}

		statusCode = http.StatusBadRequest
		responseBody = "wrong response"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/
}

/*
func TestHandleLeadingRegistrar(t *testing.T) {
	statusCode := http.StatusOK
	responseBody := "lead Service Registrar since"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
	defer ts.Close()
	testURL := ts.URL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testSys.CoreS[1].Url = testURL

	manualTicker := 10 * time.Millisecond

	go func() {
		HandleLeadingRegistrar(&testSys, manualTicker, false)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	go func() {
		HandleLeadingRegistrar(&testSys, manualTicker, true)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
}
*/

func TestDeepCopyMap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	// Use the correct key to access the map; here we use "mockUnitAsset" as set in createTestSystem
	mua := testSys.UAssets["mockUnitAsset"]
	original := (*mua).GetDetails()
	test := DeepCopyMap((*mua).GetDetails())

	if !reflect.DeepEqual(original, test) {
		t.Errorf("Expected deep copied map to be equal to original, Expected: %v, got: %v", original, test)
	}

	original["Test"][0] = "changed original"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in original affected the deep copied map. Expected: %v, got %v", original, test)
	}
	original["Test"][0] = "test"

	test["Test"][0] = "changed deep copy"
	if reflect.DeepEqual(original, test) {
		t.Errorf("Deep copy failed, changes in deep copied map affected the original. Expected: %v, got %v", original, test)
	}
}

func TestServiceRegistrationForm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	mua := testSys.UAssets["mockUnitAsset"]
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]
	version := "ServiceRecord_v1"

	payload, err := ServiceRegistrationForm(&testSys, mua, serv, version)

	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr forms.ServiceRecord_v1
	if err := json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Set the first part to your Hosts name
	expectedNode := testSys.Host.Name + "_" + testSys.Name + "_" + (*testSys.UAssets["mockUnitAsset"]).GetName() + "_" + (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"].Definition
	if sr.ServiceNode != expectedNode {
		t.Errorf("Expected ServiceNode %q, got: %q", expectedNode, sr.ServiceNode)
	}

	if len(sr.ProtoPort) != 1 {
		t.Errorf("Expected: one proto port (excluding 0s), got: %v", sr.ProtoPort)
	}

	if v, ok := sr.Details["Test"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect unit asset details. Expected: %v, got: %v", (*mua).GetDetails(), v)
	}

	if v, ok := sr.Details["Forms"]; !ok || len(v) != 1 {
		t.Errorf("Missing or incorrect unit asset details. Expected: %v, got: %v", (*serv).Details, v)
	}

	version = "UnknownVersion"
	_, err = ServiceRegistrationForm(&testSys, mua, serv, version)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if err.Error() != "unsupported service registration form version" {
		t.Errorf("Expected error: unsupported service registration form version, got: %v", err)
	}

	(*testSys.UAssets["mockUnitAsset"]).GetServices()["test"].RegPeriod = 0
	version = "ServiceRecord_v1"
	payload, err = ServiceRegistrationForm(&testSys, mua, serv, version)
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

func TestDeregisterService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	var registrar *components.CoreSystem
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]

	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string("test body"))),
	}
	newMockTransport(resp, false, nil)

	err := DeregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	registrar = testSys.CoreS[1]

	err = DeregisterService(registrar, serv)
	if err != nil {
		t.Errorf("Expected error: %v, got: %v", nil, err)
	}

	// bad case: response body error
	newMockTransport(resp, true, errHTTP)
	err = DeregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while sending http request")
	}

	// bad case: URL broken
	newMockTransport(resp, false, nil)
	registrar.Url = brokenUrl
	err = DeregisterService(registrar, serv)
	if err == nil {
		t.Errorf("Expected error while creating http request")
	}
}

func TestServiceRegistrationFormList(t *testing.T) {
	list := []string{
		"ServiceRecord_v1",
	}
	test := ServiceRegistrationFormsList()
	if !reflect.DeepEqual(list, test) {
		t.Errorf("Expected lists to be equal. Expected: %v, got: %v", list, test)
	}
}

func TestRegisterService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
	mua := testSys.UAssets["mockUnitAsset"]
	serv := (*testSys.UAssets["mockUnitAsset"]).GetServices()["test"]
	registrar := testSys.CoreS[1]

	payload, err := ServiceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")

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

	// Good case
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
	}
	newMockTransport(resp, false, nil)

	test := RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}
	fmt.Println("Delay is: ", int(test.Seconds()))

	// Bad case: when NewRequest with PUT method fails:
	newMockTransport(resp, false, nil)
	registrar.Url = brokenUrl
	test = RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with PUT method should have failed, got: %d", int(test.Seconds()))
	}
	fmt.Println("Delay is: ", int(test.Seconds()))

	// Good case when making POST instead
	registrar.Url = "https://leadingregistrar"
	serv.ID = 0

	payload, err = ServiceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")

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
	resp = &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
	}
	newMockTransport(resp, false, nil)
	test = RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative, got: %d", int(test.Seconds()))
	}
	fmt.Println("Delay is: ", int(test.Seconds()))

	// Bad case: when NewRequest with POST method fails:
	newMockTransport(resp, false, nil)
	registrar.Url = brokenUrl
	test = RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since NewRequest with POST method should have failed, got: %d", int(test.Seconds()))
	}
	fmt.Println("Delay is: ", int(test.Seconds()))

	// Bad case: when http.DefaultClient.Do() fails with a err.Timeout()
	registrar.Url = "https://leadingregistrar"
	timeoutErr := timeoutError{}
	newMockTransport(resp, true, timeoutErr)
	test = RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the executed request should fail, got %d", int(test.Seconds()))
	}
	fmt.Println("Delay is :", int(test.Seconds()))

	// Bad case: when http.DefaultClient.Do() fails but not with a err.Timeout()
	newMockTransport(resp, true, errHTTP)
	test = RegisterService(&testSys, mua, serv, registrar)
	if int(test.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 since the executed request should fail, got %d", int(test.Seconds()))
	}
	fmt.Println("Delay is :", int(test.Seconds()))
}
