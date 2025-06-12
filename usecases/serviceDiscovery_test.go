package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// mockTransport is used for replacing the default network Transport (used by
// http.DefaultClient) and it will intercept network requests.
type mockTransport struct {
	resp *http.Response
	hits int
	err  error
}

func newMockTransport(resp *http.Response, v int, err error) *mockTransport {
	t := &mockTransport{
		resp: resp,
		hits: v,
		err:  err,
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = t
	return t
}

// RoundTrip method is required to fulfil the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network, and count how many times
// a http request was sent
func (t *mockTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	t.hits -= 1
	//log.Printf("hits: %d", t.hits)
	if t.hits == 0 {
		return nil, t.err
	}
	t.resp.Request = req
	return t.resp, nil
}

var errHTTP error = fmt.Errorf("bad http request")

// Tests the output from ServQuestForms() to ensure expected outcome

func TestServQuestForms(t *testing.T) {
	expectedForms := []string{"ServiceQuest_v1", "ServicePoint_v1"}
	lst := ServQuestForms()
	// Loop through the forms from ServQuestForms() and compare them to expected forms
	for i, form := range lst {
		if form != expectedForms[i] {
			t.Errorf("Expected %s, got %s", form, expectedForms[i])
		}
	}
}

type UnitAsset struct {
	Name        string              `json:"name"`    // Must be a unique name, ie. a sensor ID
	Owner       *components.System  `json:"-"`       // The parent system this UA is part of
	Details     map[string][]string `json:"details"` // Metadata or details about this UA
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
}

func (mua UnitAsset) GetName() string {
	return mua.Name
}

func (mua UnitAsset) GetServices() components.Services {
	return mua.ServicesMap
}

func (mua UnitAsset) GetCervices() components.Cervices {
	return mua.CervicesMap
}

func (mua UnitAsset) GetDetails() map[string][]string {
	return mua.Details
}

func (mua UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {}

func createTestSystem(ctx context.Context, broken bool) components.System {
	// instantiate the System
	sys := components.NewSystem("testSystem", ctx)

	// Instantiate the Capsule
	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
	}

	testCerv := &components.Cervice{
		Definition: "testCerv",
		Details:    map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:      map[string][]string{},
	}

	CervicesMap := &components.Cervices{
		testCerv.Definition: testCerv,
	}

	mua := &UnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		CervicesMap: *CervicesMap,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	var muaInterface components.UnitAsset = mua
	sys.UAssets[mua.GetName()] = &muaInterface

	if broken == false {
		orchestrator := &components.CoreSystem{
			Name: "orchestrator",
			Url:  "https://orchestator",
		}
		sys.CoreS = []*components.CoreSystem{
			orchestrator,
		}
	} else {
		orchestrator := &components.CoreSystem{
			Name: "orchestrator",
			Url:  brokenUrl,
		}
		sys.CoreS = []*components.CoreSystem{
			orchestrator,
		}
	}
	return sys
}

func TestFillQuestForm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx, false)
	mua := UnitAsset{}
	questForm := FillQuestForm(&testSys, mua, "TestDef", "TestProtocol")
	// Loop through the details in questForm and mua (mockUnitAsset), error if they're not the same
	for i, detail := range questForm.Details["Details"] {
		if detail != mua.GetDetails()["Details"][i] {
			t.Errorf("Expected %s, got: %s", mua.GetDetails()["Details"][i], detail)
		}
	}
}

type testBodyHasProtocol struct {
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

type testBodyHasVersion struct {
	Version string `json:"version"`
}

type testBodyNoVersion struct{}

func createTestData(bodyType string, proto int, version string, errRead bool) (data []byte, err error) {
	if errRead == true {
		return json.Marshal(errReader(0))
	}
	switch bodyType {
	case "testBodyHasProtocol":
		body := testBodyHasProtocol{
			Protocol: proto,
			Version:  version,
		}
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "testBodyHasVersion":
		body := testBodyHasVersion{
			Version: version,
		}
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "testBodyNoVersion":
		body := testBodyNoVersion{}
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, errors.New("Body type not supported")
	}
}

type params struct {
	testCase      string
	bodyType      string
	protocol      int
	version       string
	errRead       bool
	expectedError bool
}

func TestExtractQuestForm(t *testing.T) {
	// A list holding structs containing the parameters used for the test
	testParams := []params{
		// {"testCase", "bodyType", "protocol", "version", "errRead", "expectedError"}
		{"No errors", "testBodyHasVersion", -1, "ServiceQuest_v1", false, false},
		{"Error during Unmarshal", "testBodyHasVersion", -1, "ServiceQuest_v1", true, true},
		{"Missing version", "testBodyNoVersion", -1, "", false, false},
		{"Error while writing to correct form", "testBodyHasProtocol", 123, "ServiceQuest_v1", false, true},
		{"Error Unsupported version", "testBodyHasVersion", -1, "", false, true},
	}
	for _, x := range testParams {
		// Create the data []byte that will be sent into the function
		data, err := createTestData(x.bodyType, x.protocol, x.version, x.errRead)
		if err != nil {
			t.Errorf("---\tError occured while creating test data")
		}
		// Do the test
		rec, err := ExtractQuestForm(data)
		if x.testCase == "No errors" || x.testCase == "Missing version" {
			if err != nil {
				t.Errorf("Test case: '%s' got error: %e", x.testCase, err)
			}
			if x.testCase == "Missing version" && rec.Version != "" {
				t.Errorf("---\tExpected no version, got %s", rec.Version)
			}
		} else {
			if err == nil {
				t.Errorf("---\tTest case: Expected errors in '%s', got none", x.testCase)
			}
		}
	}
}

func createServicePointTestForm() forms.ServicePoint_v1 {
	var f forms.ServicePoint_v1
	f.NewForm()
	f.Version = "ServicePoint_v1"
	f.ServLocation = "TestService"
	f.ServiceDefinition = "TestService"
	f.Details = map[string][]string{
		"Details": {"detail_1", "detail_2"},
	}
	return f
}

// Create a error reader to break json.unmarshal
type errReader int

var errBodyRead error = fmt.Errorf("bad body read")

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errBodyRead
}
func (errReader) Close() error {
	return nil
}

var brokenUrl = string([]byte{0x7f})

// sendHttpReq(method string, url string, jsonQF []byte, ctx context.Context) (resp *http.Response, err error)
func TestSendHttpReq(t *testing.T) {
	// Good case: everything passes
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string("test body"))),
	}
	newMockTransport(resp, 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var qForm forms.ServiceQuest_v1
	qForm.NewForm()
	jsonQF, err := json.MarshalIndent(qForm, "", "  ")
	if err != nil {
		t.Errorf("Error occurred while Marshalling in test: %v", err)
	}
	_, err = sendHttpReq(http.MethodPost, "https://test", jsonQF, ctx)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
	cancel()

	// Bad case: url broken, cant make request
	ctx, cancel = context.WithCancel(context.Background())
	qForm.NewForm()
	_, err = sendHttpReq(http.MethodPost, brokenUrl, jsonQF, ctx)
	if err == nil {
		t.Errorf("Expected errors while sending http request")
	}
	cancel()

	// Bad case: response returns error
	newMockTransport(resp, 1, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	qForm.NewForm()
	_, err = sendHttpReq(http.MethodPost, "https://test", jsonQF, ctx)
	if err == nil {
		t.Errorf("Expected errors while sending http request")
	}
	cancel()
}

func TestSearch4Service(t *testing.T) {
	// Best case, everything pass
	f := createServicePointTestForm()
	// Create mock response from orchestrator
	fakeBody, err := json.Marshal(f)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
	}
	newMockTransport(resp, 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	testSys := createTestSystem(ctx, false)
	var qForm forms.ServiceQuest_v1

	serviceForm, err := Search4Service(qForm, &testSys)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
	if serviceForm.ServLocation != f.ServLocation {
		t.Errorf("Expected %s, got: %s", f.ServLocation, serviceForm.ServLocation)
	}
	cancel()

	// Error at "prepare the payload to perform a service quest"
	// Untested because I found no way of breaking json.Marshal, without making big changes to the form

	// Error while getting core system url
	newMockTransport(resp, 1, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at GetRunningCoreSystemURL()")
	}
	cancel()

	// Error at sendHttpRequest
	newMockTransport(resp, 2, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at GetRunningCoreSystemURL()")
	}
	cancel()

	// Error at "Read the response", io.ReadAll()
	f = createServicePointTestForm()
	resp.Body = errReader(0)
	newMockTransport(resp, 0, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	serviceForm, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error")
	}
	cancel()

	// Error at "Read the response", ExtractDiscoveryForm()
	f = createServicePointTestForm()
	resp.Body = io.NopCloser(strings.NewReader(string("test")))
	newMockTransport(resp, 0, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	serviceForm, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error")
	}
	cancel()

}

// Search4Services(cer *components.Cervice, sys *components.System) (err error)
// *forms.ServicePoint_v1
/*
   ServiceID         int                 `json:"serviceId"`
   ProviderName      string              `json:"providerName"`
   ServiceDefinition string              `json:"definition"`
   Details           map[string][]string `json:"details"`
   ServLocation      string              `json:"serviceURL"`
   ServNode          string              `json:"serviceNode"`
   Token             string              `json:"token"`
   Version           string              `json:"version"`
*/

func createTestServicePoint() (f forms.ServicePoint_v1) {
	f.ProviderName = "testProvider"
	f.ServiceDefinition = "testDef"
	f.Details = map[string][]string{
		"Details": {"detail1", "detail2"},
	}
	f.Version = "ServicePoint_v1"
	return
}

func TestSearch4Services(t *testing.T) {
	// Best case: everything passes
	fakeBody := createTestServicePoint()
	data, err := json.Marshal(fakeBody)
	if err != nil {
		t.Error("Error in test during json.Marshal()")
	}
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
	resp.Header = make(http.Header)
	resp.Header.Set("Content-Type", "application/json")
	newMockTransport(resp, 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	testSys := createTestSystem(ctx, false)
	cer := (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err != nil {
		t.Errorf("Expected no errors, got %v", err)
	}
	cancel()

	// Bad case: GetRunningCoreSystemURL() returns error
	newMockTransport(resp, 1, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, true) // true sets orchestrator url to a brokenURL
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()

	// Bad case: Orchestrator url is ""
	newMockTransport(resp, 0, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	(*testSys.CoreS[0]).Url = ""
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()

	// Bad case: sendHttpReq() returns an error
	// TODO: Fix this, maybe change the mockTransport to count number of times it's been called
	// and then change the retError to true and it should fail.
	newMockTransport(resp, 2, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()

	// Bad case: Response status code is < 200 or >= 300
	resp.StatusCode = 199
	newMockTransport(resp, 4, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()

	// Bad case: io.ReadAll() return an error
	resp.StatusCode = 200
	resp.Body = errReader(0)
	newMockTransport(resp, 0, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()

	// Bad case: Unpack() returns an error
	resp.Body = io.NopCloser(strings.NewReader(string(data)))
	resp.Header.Set("Content-Type", "Error")
	newMockTransport(resp, 0, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
	cancel()
}

func createTestServiceRecord(number int) (f forms.ServiceRecord_v1) {
	f.Id = number
	f.ServiceDefinition = fmt.Sprintf("testDefinition%d", number)
	f.SystemName = fmt.Sprintf("testSystem%d", number)
	f.ServiceNode = fmt.Sprintf("test%d", number)
	f.IPAddresses = []string{fmt.Sprintf("test%d", number), fmt.Sprintf("test%d", number+1)}
	f.ProtoPort = map[string]int{"test": 1}
	f.Details = map[string][]string{"Details": {fmt.Sprintf("Detail%d", number), fmt.Sprintf("Detail%d", number+1)}}
	f.Certificate = fmt.Sprintf("Certificate%d", number)
	f.SubPath = fmt.Sprintf("Subpath%d", number)
	f.RegLife = number
	f.Version = "ServiceRecord_v1"
	f.Created = fmt.Sprintf("Created%d", number)
	f.Updated = fmt.Sprintf("Updated%d", number)
	f.EndOfValidity = fmt.Sprintf("EoV%d", number)
	f.SubscribeAble = true
	f.ACost = float64(number)
	f.CUnit = fmt.Sprintf("CUnit%d", number)
	return
}

// FillDiscoveredServices(dsList []forms.ServiceRecord_v1, version string) (f forms.Form, err error)
func TestFillDiscoveredServices(t *testing.T) {
	// Create a bunch of service records contained in a list
	dsList := []forms.ServiceRecord_v1{}
	for i := range 10 {
		record := createTestServiceRecord(i)
		dsList = append(dsList, record)
	}
	versionList := []string{"ServiceRecordList_v1", "default"}
	for _, version := range versionList {
		_, err := FillDiscoveredServices(dsList, version)
		if version != "ServiceRecordList_v1" {
			if err == nil {
				t.Errorf("Expected error in default case")
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error during testing: %v", err)
			}
		}
	}
}

// ExtractDiscoveryForm(bodyBytes []byte) (sLoc forms.ServicePoint_v1, err error)
func TestExtractDiscoveryForm(t *testing.T) {
	// Best case: everything passes
	spForm := createServicePointTestForm()
	data, err := json.Marshal(spForm)
	if err != nil {
		t.Errorf("Error occurred while marshaling the test form")
	}
	//form version: forms.ServicePoint_v1 expected
	form, err := ExtractDiscoveryForm(data)
	if err != nil {
		t.Errorf("Expected no errors")
	}
	if form.ServLocation != "TestService" {
		t.Errorf("Expected service location: %s, got %s", "TestService", form.ServLocation)
	}

	// Bad case: Default switch case, wrong form version
	spForm.Version = ""
	data, err = json.Marshal(spForm)
	if err != nil {
		t.Errorf("Error occurred while marshaling the test form")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected error because of wrong form version")
	}

	// Bad case: version key not found
	data, err = json.Marshal(nil)
	if err != nil {
		t.Errorf("Error when marshalling in test")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected errors for missing form")
	}

	// Bad case: Unmarshalling body bytes to forms.ServicePoint_v1
	// Needed to create my own map, with the correct version but a field that had a different type
	// than the target field in order to break unmarshal
	wrongForm := make(map[string]any)
	wrongForm["version"] = "ServicePoint_v1"
	wrongForm["serviceId"] = false // Target field is an int
	data, err = json.Marshal(wrongForm)
	if err != nil {
		t.Errorf("Error when marshalling in test")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected errors for wrong form")
	}
}
