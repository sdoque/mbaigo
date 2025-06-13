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

/*
func TestDiscoverLeadingRegistrar(t *testing.T) {
	// Create a new Test System to run the test on
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	testSys1 := createTestSystem(ctx1)

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case: Everything works in the case that there is no immediate leading registrar
	// -- -- -- -- -- -- -- -- -- -- //

	respFunc1 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
		}
	}
	newMockTransport(respFunc1, 0, nil)

	manualTicker := 10 * time.Millisecond

	resultCh1 := make(chan *components.CoreSystem, 1)
	defer close(resultCh1)
	go func() {
		res1 := DiscoverLeadingRegistrar(&testSys1, manualTicker, false)
		resultCh1 <- res1
	}()
	time.Sleep(5 * manualTicker)
	cancel1()
	leadReg1 := <-resultCh1
	if leadReg1.Name != "serviceregistrar" {
		t.Errorf("Expected %s, got: %s", "serviceregistrar", leadReg1.Name)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: Broken URL
	// -- -- -- -- -- -- -- -- -- -- //

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	testSys2 := createTestSystem(ctx2)

	respFunc2 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
		}
	}

	newMockTransport(respFunc2, 0, nil)
	testSys2.CoreS[1].Url = brokenUrl
	resultCh2 := make(chan *components.CoreSystem, 1)
	defer close(resultCh2)
	go func() {
		res2 := DiscoverLeadingRegistrar(&testSys2, manualTicker, false)
		resultCh2 <- res2
	}()
	time.Sleep(5 * manualTicker)
	cancel2()
	leadReg2 := <-resultCh2
	if leadReg2 != nil {
		t.Errorf("Expected the leading registrar to be nil, got: %v", leadReg2)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: io.ReadAll() returns an error
	// -- -- -- -- -- -- -- -- -- -- //

	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	testSys3 := createTestSystem(ctx3)

	respFunc3 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(errorReader{}),
		}
	}

	newMockTransport(respFunc3, 0, nil)
	resultCh3 := make(chan *components.CoreSystem, 1)
	defer close(resultCh3)
	go func() {
		res3 := DiscoverLeadingRegistrar(&testSys3, manualTicker, false)
		resultCh3 <- res3
	}()
	time.Sleep(5 * manualTicker)
	cancel3()
	leadReg3 := <-resultCh3
	if leadReg3 != nil {
		t.Errorf("Expected an error when reading the response body, got: %v", leadReg3)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: Closing the response body returns an error (no real way to see this as the error handling for that does nothing)
	// -- -- -- -- -- -- -- -- -- -- //

	ctx4, cancel4 := context.WithCancel(context.Background())
	defer cancel4()
	testSys4 := createTestSystem(ctx4)

	respFunc4 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &errorReadCloser{},
		}
	}

	newMockTransport(respFunc4, 0, nil)
	resultCh4 := make(chan *components.CoreSystem, 1)
	defer close(resultCh4)
	go func() {
		res4 := DiscoverLeadingRegistrar(&testSys4, manualTicker, false)
		resultCh4 <- res4
	}()
	time.Sleep(5 * manualTicker)
	cancel4()
	leadReg4 := <-resultCh4
	if leadReg4 != nil {
		t.Errorf("Expected an error when closing the response body, got: %v", leadReg4)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case: But the leading registrar is not null in the beginning
	// -- -- -- -- -- -- -- -- -- -- //

	ctx5, cancel5 := context.WithCancel(context.Background())
	defer cancel5()
	testSys5 := createTestSystem(ctx5)

	respFunc5 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
		}
	}

	newMockTransport(respFunc5, 0, nil)
	resultCh5 := make(chan *components.CoreSystem, 1)
	defer close(resultCh5)
	go func() {
		res5 := DiscoverLeadingRegistrar(&testSys5, manualTicker, true)
		resultCh5 <- res5
	}()
	time.Sleep(5 * manualTicker)
	cancel5()
	leadReg5 := <-resultCh5
	if leadReg5.Name != "serviceregistrar" {
		t.Errorf("Expected the lead registrars name to be: %s, got: %s", "serviceregistrar", leadReg5.Name)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: Broken URL, with leading registrar from the beginning
	// -- -- -- -- -- -- -- -- -- -- //

	ctx6, cancel6 := context.WithCancel(context.Background())
	defer cancel6()
	testSys6 := createTestSystem(ctx6)

	respFunc6 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("lead Service Registrar since"))),
		}
	}

	newMockTransport(respFunc6, 0, nil)
	testSys6.CoreS[1].Url = brokenUrl
	resultCh6 := make(chan *components.CoreSystem, 1)
	defer close(resultCh6)
	go func() {
		res6 := DiscoverLeadingRegistrar(&testSys6, manualTicker, true)
		resultCh6 <- res6
	}()
	time.Sleep(5 * manualTicker)
	cancel6()
	leadReg6 := <-resultCh6
	if leadReg6 != nil {
		t.Errorf("Expected leading registrar to be nil since broken URL in Get() method, got: %v", leadReg6)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: io.ReadAll() returns an error, with leading registrar from the beginning
	// -- -- -- -- -- -- -- -- -- -- //

	ctx7, cancel7 := context.WithCancel(context.Background())
	defer cancel7()
	testSys7 := createTestSystem(ctx7)

	respFunc7 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(errorReader{}),
		}
	}

	newMockTransport(respFunc7, 0, nil)
	resultCh7 := make(chan *components.CoreSystem, 1)
	defer close(resultCh7)
	go func() {
		res7 := DiscoverLeadingRegistrar(&testSys7, manualTicker, true)
		resultCh7 <- res7
	}()
	time.Sleep(5 * manualTicker)
	cancel7()
	leadReg7 := <-resultCh7
	if leadReg7 != nil {
		t.Errorf("Expected an error when reading the response body, got: %v", leadReg7)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case: The previous leading registrar has been lost. i.e. Prefix in bodyBytes string is not "lead Service Registrar since", with leading registrar from the beginning
	// -- -- -- -- -- -- -- -- -- -- //

	ctx8, cancel8 := context.WithCancel(context.Background())
	defer cancel8()
	testSys8 := createTestSystem(ctx8)

	respFunc8 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(""))),
		}
	}

	newMockTransport(respFunc8, 0, nil)
	resultCh8 := make(chan *components.CoreSystem, 1)
	defer close(resultCh8)
	go func() {
		res8 := DiscoverLeadingRegistrar(&testSys8, manualTicker, true)
		resultCh8 <- res8
	}()
	time.Sleep(5 * manualTicker)
	cancel8()
	leadReg8 := <-resultCh8
	if leadReg8 != nil {
		t.Errorf("Expected the lead registrar to be nil since it is lost, got: %s", leadReg8)
	}
}

func TestHandleLeadingRegistrar(t *testing.T) {

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case, everything works, the system has UnitAssets
	// -- -- -- -- -- -- -- -- -- -- //

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	testSys1 := createTestSystem(ctx1)
	mua1 := testSys1.UAssets["mockUnitAsset"]
	serv1 := (*testSys1.UAssets["mockUnitAsset"]).GetServices()["test"]

	payload1, err1 := serviceRegistrationForm(&testSys1, mua1, serv1, "ServiceRecord_v1")

	if err1 != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr1 forms.ServiceRecord_v1
	if err := json.Unmarshal(payload1, &sr1); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	sr1.EndOfValidity = time.Now().Format(time.RFC3339)

	fakeBody1, err := json.Marshal(sr1)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}

	respFunc1 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody1))),
		}
	}
	newMockTransport(respFunc1, 0, nil)

	manualTicker := 50 * time.Millisecond
	resultTest1 := make(chan time.Duration, 1)
	resultErr1 := make(chan error, 1)

	go func() {
		dur1, err1 := HandleLeadingRegistrar(&testSys1, manualTicker, false)
		resultTest1 <- dur1
		resultErr1 <- err1
	}()
	time.Sleep(100 * time.Millisecond)
	cancel1()
	res1 := <-resultTest1
	resErr1 := <-resultErr1
	if int(res1.Seconds()) != 15 {
		t.Errorf("Expected the delay to be 15 seconds since there is no leading registrar, got: %d", int(res1.Seconds()))
	}
	if resErr1 != nil {
		t.Errorf("Expected the error to be nil since there is no need to deregister a service that is not registered, got: %v", err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Good case, when leading registrar is not null in the beginning
	// -- -- -- -- -- -- -- -- -- -- //

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	testSys2 := createTestSystem(ctx2)
	mua2 := testSys2.UAssets["mockUnitAsset"]
	serv2 := (*testSys2.UAssets["mockUnitAsset"]).GetServices()["test"]

	payload2, err2 := serviceRegistrationForm(&testSys2, mua2, serv2, "ServiceRecord_v1")

	if err2 != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr2 forms.ServiceRecord_v1
	if err = json.Unmarshal(payload2, &sr2); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	sr2.EndOfValidity = time.Now().Format(time.RFC3339)

	fakeBody2, err := json.Marshal(sr2)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}

	respFunc2 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody2))),
		}
	}

	newMockTransport(respFunc2, 0, nil)
	resultTest2 := make(chan time.Duration, 1)
	resultErr2 := make(chan error, 1)
	go func() {
		dur2, err2 := HandleLeadingRegistrar(&testSys2, manualTicker, true)
		resultTest2 <- dur2
		resultErr2 <- err2
	}()
	time.Sleep(100 * time.Millisecond)
	cancel2()
	res2 := <-resultTest2
	resErr2 := <-resultErr2
	if int(res2.Seconds()) > 0 {
		t.Errorf("Expected the delay to be negative since the service should have been registered, got: %d", int(res2.Seconds()))
	}
	if resErr2 != nil {
		t.Errorf("Expected the service to be able to deregistered, got: %v", err)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Bad case, error when deregistring the service
	// -- -- -- -- -- -- -- -- -- -- //

	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	testSys3 := createTestSystem(ctx3)
	mua3 := testSys3.UAssets["mockUnitAsset"]
	serv3 := (*testSys3.UAssets["mockUnitAsset"]).GetServices()["test"]

	payload3, err3 := serviceRegistrationForm(&testSys3, mua3, serv3, "ServiceRecord_v1")

	if err3 != nil {
		t.Fatalf("The Service Record version was wrong.")
	}

	var sr3 forms.ServiceRecord_v1
	if err = json.Unmarshal(payload3, &sr3); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	sr3.EndOfValidity = time.Now().Format(time.RFC3339)

	fakeBody3, err := json.Marshal(sr3)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}

	respFunc3 := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody3))),
		}
	}

	newMockTransport(respFunc3, 1, errHTTP)
	resultTest3 := make(chan time.Duration, 1)
	resultErr3 := make(chan error, 1)
	go func() {
		dur3, err3 := HandleLeadingRegistrar(&testSys3, manualTicker, true)
		resultTest3 <- dur3
		resultErr3 <- err3
	}()
	time.Sleep(100 * time.Millisecond)
	cancel3()
	resErr3 := <-resultErr3
	if resErr3 == nil {
		t.Errorf("Expected an error when deregistring the service, got: %v", resErr3)
	}

	// -- -- -- -- -- -- -- -- -- -- //
	// Neutral case, the system has no UAssets so it won't either register or deregister anything
	// -- -- -- -- -- -- -- -- -- -- //

	ctx4, cancel4 := context.WithCancel(context.Background())
	defer cancel4()
	testSys4 := createTestSystem(ctx4)
	testSys4.UAssets = nil

	resultTest4 := make(chan time.Duration, 1)
	resultErr4 := make(chan error, 1)
	go func() {
		dur4, err4 := HandleLeadingRegistrar(&testSys4, manualTicker, true)
		resultTest4 <- dur4
		resultErr4 <- err4
	}()
	time.Sleep(100 * time.Millisecond)
	cancel4()
	res4 := <-resultTest4
	if res4 != 0 {
		t.Errorf("Expected the delay to be 0 (time.Duration zero value) since the system has no UAssets, got: %d", int(res4.Seconds()))
	}
}
*/

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
