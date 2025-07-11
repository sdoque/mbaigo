package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type testBodyHasProtocol struct {
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

type testBodyHasVersion struct {
	Version string `json:"version"`
}

type testBodyNoVersion struct{}

func createTestBodyHasProtocol(proto int, version string, errRead bool) ([]byte, error) {
	if errRead == true {
		return json.Marshal(errReader(0))
	}
	body := testBodyHasProtocol{
		Protocol: proto,
		Version:  version,
	}
	return json.Marshal(body)
}

func createTestBodyHasVersion(proto int, version string, errRead bool) ([]byte, error) {
	if errRead == true {
		return json.Marshal(errReader(0))
	}
	body := testBodyHasVersion{
		Version: version,
	}
	return json.Marshal(body)
}

func createTestBodyHasNoVersion(proto int, version string, errRead bool) ([]byte, error) {
	if errRead == true {
		return json.Marshal(errReader(0))
	}
	body := testBodyNoVersion{}
	return json.Marshal(body)
}

type extractQuestFormParams struct {
	expectedError bool
	errRead       bool
	proto         int
	version       string
	f             func(int, string, bool) ([]byte, error)
	testCase      string
}

func TestExtractQuestForm(t *testing.T) {
	testParams := []extractQuestFormParams{
		{false, false, -1, "ServiceQuest_v1", createTestBodyHasVersion, "No errors"},
		{true, true, -1, "ServiceQuest_v1", createTestBodyHasVersion, "Error during Unmarshal"},
		{true, false, -1, "", createTestBodyHasNoVersion, "Missing version"},
		{true, false, 123, "ServiceQuest_v1", createTestBodyHasProtocol, "Error while writing to correct form"},
		{true, false, -1, "", createTestBodyHasVersion, "Error Unsupported version"},
	}
	for _, x := range testParams {
		data, err := x.f(x.proto, x.version, x.errRead)
		if err != nil {
			t.Errorf("---\tError occurred while creating test data")
		}
		// Do the test
		_, err = ExtractQuestForm(data)
		if x.expectedError == false && err != nil {
			t.Errorf("Expected no errors in '%s', got: %v ", x.testCase, err)
		}
		if x.expectedError == true && err == nil {
			t.Errorf("Expected errors in '%s'", x.testCase)

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
		_, err = sendHTTPReq(c.method, c.url, c.data)
		if c.expectError == false && err != nil {
			t.Errorf("Unexpected error in '%s' test case: %e", c.testCase, err)
		}
		if c.expectError == true && err == nil {
			t.Errorf("Expected error in '%s' test case, got none", c.testCase)
		}
	}
}

// --------------------------------------------------------- //
// Helper functions and structs for testing Search4Service()
// --------------------------------------------------------- //

type search4ServiceParams struct {
	expectError bool
	response    func() *http.Response
	transport   func(func() *http.Response) *mockTransport
	testCase    string
}

// This function returns different http responses depending on the number of times it's read
// allowedReads takes a positive number, and will count back from that number until it reaches 0, then return a
// http.Response with errReader() in body, given 0 or negative number it'll always return a functioning http.Response
func createMultiHttpResp(statusCode int, broken bool, allowedReads int) func() *http.Response {
	f := createServicePointTestForm()
	// Create mock response from orchestrator
	fakeBody, err := json.Marshal(f)
	if err != nil {
		log.Println("Fail Marshal at start of test")
	}
	count := allowedReads
	return func() *http.Response {
		count--
		if broken == true && count == 0 {
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       errReader(0),
			}
		}
		return &http.Response{
			StatusCode: statusCode,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
		}
	}
}

func TestSearch4Service(t *testing.T) {
	// Test parameters
	params := []search4ServiceParams{
		{
			false,
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Best case",
		},
		{
			true,
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 1, errHTTP) },
			"Bad case, error getting core system url",
		},
		{
			true,
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 2, errHTTP) },
			"Bad case, error sending http request",
		},
		{
			true,
			createMultiHttpResp(200, true, 2),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Bad case, error reading response body",
		},
	}
	testSys := createTestSystem(false)
	for _, c := range params {
		// Setup
		c.transport(c.response)
		var qForm forms.ServiceQuest_v1
		qForm.NewForm()

		// Test
		_, err := Search4Service(qForm, &testSys)
		if c.expectError == false && err != nil {
			t.Errorf("Expected no errors in testcase '%s', got: %v", c.testCase, err)
		}
		if c.expectError == true && err == nil {
			t.Errorf("Expected errors in testcase '%s'", c.testCase)
		}
	}
}

// --------------------------------------------------------- //
// Helper functions and structs for testing Search4Services()
// --------------------------------------------------------- //

type search4ServicesParams struct {
	expectError bool
	setup       func() (*components.Cervice, components.System)
	response    func() *http.Response
	transport   func(func() *http.Response) *mockTransport
	testCase    string
}

func TestSearch4Services(t *testing.T) {
	params := []search4ServicesParams{
		{
			false,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Best case, no errors",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 1, errHTTP) },
			"Bad case, GetRunningCoreSystemURL() returns error",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				for i, cs := range sys.CoreS {
					if cs.Name == "orchestrator" {
						(*sys.CoreS[i]).Url = ""
					}
				}
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Bad case, Orchestrator url is empty",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			createMultiHttpResp(200, false, 0),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 2, errHTTP) },
			"Bad case, sendHttpReq() returns an error",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			createMultiHttpResp(200, true, 2),
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Bad case, error while reading body",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			func() *http.Response {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"Error"}},
					Body:       io.NopCloser(strings.NewReader(string(""))),
				}
			},
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Bad case, error during Unpack",
		},
		{
			true,
			func() (cer *components.Cervice, sys components.System) {
				sys = createTestSystem(false)
				cer = (*sys.UAssets["testUnitAsset"]).GetCervices()["testCerv"]
				return
			},
			func() *http.Response {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(string(`{"version":"SignalA_v1.0"}`))),
				}
			},
			func(resp func() *http.Response) *mockTransport { return newMockTransport(resp, 0, nil) },
			"Bad case, error during type conversion",
		},
	}

	for _, c := range params {
		// Setup
		c.transport(c.response)
		cer, sys := c.setup()

		// Test
		err := Search4Services(cer, &sys)
		if (c.expectError == false) && (err != nil) {
			t.Errorf("Expected no errors in '%s', got: %v", c.testCase, err)
		}
		if (c.expectError == true) && (err == nil) {
			t.Errorf("Expected errors in '%s'", c.testCase)
		}
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
		if version != "ServiceRecordList_v1" && err == nil {
			t.Errorf("Expected error in default case")
		}
		if version == "ServiceRecordList_v1" && err != nil {
			t.Errorf("Unexpected error during testing: %v", err)
		}
	}
}

// --------------------------------------------------------------- //
// Helper functions and structs for testing ExtractDiscoveryForm()
// --------------------------------------------------------------- //

type extractDiscoveryFormParams struct {
	expectError bool
	data        func() any
	testCase    string
}

// ExtractDiscoveryForm(bodyBytes []byte) (sLoc forms.ServicePoint_v1, err error)
func TestExtractDiscoveryForm(t *testing.T) {
	params := []extractDiscoveryFormParams{
		{
			false,
			func() any { return createServicePointTestForm() },
			"Best case",
		},
		{
			true,
			func() any {
				return ""
			},
			"Bad case, Unmarshal breaks",
		},
		{
			true,
			func() any {
				form := createServicePointTestForm()
				form.Version = ""
				return form
			},
			"Bad case, wrong form version",
		},
		{
			true,
			func() any { return nil },
			"Bad case, version key missing",
		},
		{
			true,
			func() any {
				wrongForm := make(map[string]any)
				wrongForm["version"] = "ServicePoint_v1"
				wrongForm["serviceId"] = false // Target field is an int
				return wrongForm
			},
			"Bad case, can't unmarshal to ServicePoint_v1 (field type mismatch)",
		},
	}

	for _, c := range params {
		// Setup
		data, err := json.Marshal(c.data())
		if err != nil {
			t.Errorf("couldn't marshal data in '%s'", c.testCase)
		}
		// Test
		_, err = ExtractDiscoveryForm(data)
		if (c.expectError == false) && (err != nil) {
			t.Errorf("Expected no errors in '%s', got: %v", c.testCase, err)
		}
		if (c.expectError == true) && (err == nil) {
			t.Errorf("Expected errors in '%s'", c.testCase)
		}
	}
}
