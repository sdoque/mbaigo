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

	"github.com/sdoque/mbaigo/forms"
)

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

func TestFillQuestForm(t *testing.T) {
	testSys, _ := createTestSystem(false)
	mua := mockUnitAsset{}
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

type extractQuestFormParams struct {
	testCase      string
	bodyType      string
	protocol      int
	version       string
	errRead       bool
	expectedError bool
}

func TestExtractQuestForm(t *testing.T) {
	// A list holding structs containing the parameters used for the test
	testParams := []extractQuestFormParams{
		// {testCase, bodyType, protocol, version, errRead, expectedError}
		// Always start with the "Best case, no errors"
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
			t.Errorf("---\tError occurred while creating test data")
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

// Creates a ServicePoint_v1 form with test values
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

type sendHttpReqParams struct {
	testCase    string
	method      string
	url         string
	data        []byte
	ctx         context.Context
	respError   bool
	expectError bool
}

func testSystemSetup() (resp func() *http.Response, data []byte, ctx context.Context, err error) {
	ctx = context.Background()
	var form forms.ServiceQuest_v1
	form.NewForm()
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("test body"))),
		}
	}
	data, err = json.MarshalIndent(form, "", "  ")
	if err != nil {
		return nil, nil, ctx, errors.New("---\tError occurred while marshalling in test system setup")
	}
	return
}

func TestSendHttpReq(t *testing.T) {
	resp, data, ctx, err := testSystemSetup()
	newMockTransport(resp, 0, nil)
	if err != nil {
		t.Errorf("Error occurred while starting test system: %e", err)
	}
	params := []sendHttpReqParams{
		// {testCase, method, url, data, ctx, respError, expectError}
		{"No errors", http.MethodPost, "http://test", data, ctx, false, false},
		{"Error creating new request", http.MethodPost, brokenUrl, data, ctx, false, true},
		{"DefaultClient returns error", http.MethodPost, "http://test", data, ctx, true, true},
	}
	var lastLoopErr bool
	for _, c := range params {
		// Make sure the the mockTransport doesn't return an error unless needed by the test
		if (lastLoopErr == true) && (c.respError == false) {
			newMockTransport(resp, 0, nil)
			lastLoopErr = false
		}
		// Make a new mockTransport with an error response if the test needs it
		if (lastLoopErr == false) && (c.respError == true) {
			newMockTransport(resp, 1, errHTTP)
			lastLoopErr = true
		}
		// Run the test
		_, err = sendHttpReq(c.method, c.url, c.data)
		if c.expectError == false {
			if err != nil {
				t.Errorf("Unexpected error in '%s' test case: %e", c.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("Expected error in '%s' test case, got none", c.testCase)
			}
		}
	}
}

func TestSearch4Service(t *testing.T) {
	// Best case, everything pass
	f := createServicePointTestForm()
	// Create mock response from orchestrator
	fakeBody, err := json.Marshal(f)
	if err != nil {
		t.Errorf("Fail Marshal at start of test")
	}
	resp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}
	newMockTransport(resp, 0, nil)
	testSys, _ := createTestSystem(false)
	var qForm forms.ServiceQuest_v1
	serviceForm, err := Search4Service(qForm, &testSys)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
	if serviceForm.ServLocation != f.ServLocation {
		t.Errorf("Expected %s, got: %s", f.ServLocation, serviceForm.ServLocation)
	}

	// Error at "prepare the payload to perform a service quest"
	// Untested because I found no way of breaking json.Marshal, without making big changes to the form

	// Error while getting core system url
	newMockTransport(resp, 1, errHTTP)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at GetRunningCoreSystemURL()")
	}

	// Error at sendHttpRequest
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 ?",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}
	newMockTransport(resp, 2, errHTTP)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at sendHttpRequest()")
	}

	// Non-2xx status code of response from sendHttpRequest()
	resp = func() *http.Response {
		return &http.Response{
			Status:     "300 ?",
			StatusCode: 300,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}
	newMockTransport(resp, 0, nil)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at sendHttpRequest")
	}

	// Error at "Read the response", io.ReadAll()
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       errReader(0),
		}
	}
	f = createServicePointTestForm()
	newMockTransport(resp, 0, nil)
	qForm.NewForm()
	serviceForm, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error")
	}

	// Error at "Read the response", ExtractDiscoveryForm()
	f = createServicePointTestForm()
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("test"))),
		}
	}
	newMockTransport(resp, 0, nil)
	qForm.NewForm()
	serviceForm, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error")
	}
}

// Used to create a ServicePoint_v1 form for testing purposes
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
	resp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(data))),
		}
	}
	newMockTransport(resp, 0, nil)
	testSys, _ := createTestSystem(false)
	cer := (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err != nil {
		t.Errorf("Expected no errors, got %v", err)
	}

	// Bad case: GetRunningCoreSystemURL() returns error
	newMockTransport(resp, 1, errHTTP)
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}

	// Bad case: Orchestrator url is ""
	newMockTransport(resp, 0, nil)
	for i, cs := range testSys.CoreS {
		if cs.Name == "orchestrator" {
			(*testSys.CoreS[i]).Url = ""
		}
	}
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}

	// Bad case: sendHttpReq() returns an error
	newMockTransport(resp, 2, nil)
	testSys, _ = createTestSystem(false) // Needed otherwise we don't get past the orchestrator error handlers
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}

	// Bad case: io.ReadAll() return an error
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       errReader(0),
		}
	}
	newMockTransport(resp, 0, nil)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}

	// Bad case: Unpack() returns an error and type assertion/conversion fails
	resp = func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"Error"}},
			Body:       io.NopCloser(strings.NewReader(string(data))),
		}
	}
	newMockTransport(resp, 0, nil)
	cer = (*testSys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
	err = Search4Services(cer, &testSys)
	if err == nil {
		t.Errorf("Expected errors")
	}
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
