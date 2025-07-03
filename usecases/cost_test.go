package usecases

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

func TestGetActivitiesCost(t *testing.T) {
	testServ := &components.Service{
		Definition: "testDef",
		ACost:      123,
		CUnit:      "testCUnit",
	}
	data, err := GetActivitiesCost(testServ)
	if err != nil {
		t.Errorf("no error expected, got: %v", err)
	}

	// Check that correct data is present
	if strings.Contains(string(data), `"activity": "testDef"`) == false {
		t.Errorf("Definition/activity doesn't match")
	}
	if (strings.Contains(string(data), `"cost": 123`)) == false {
		t.Errorf("ACost/cost doesn't match")
	}
}

// ------------------------------------------------------ //
// Helper functions and structs for TestSetActivitiesCost()
// ------------------------------------------------------ //

type setACparams struct {
	dataString  string
	expectError bool
	testCase    string
}

func createTestService() (serv *components.Service) {
	testServ := &components.Service{
		ID:            0,
		Definition:    "testDefinition",
		SubPath:       "testService",
		Details:       map[string][]string{"Details": {"detail1", "detail2"}},
		RegPeriod:     45,
		RegTimestamp:  "Now",
		RegExpiration: "Later",
		Description:   "A service for testing purposes",
		SubscribeAble: false,
		ACost:         123,
		CUnit:         "testCUnit",
	}
	return testServ
}

func TestSetActivitiesCost(t *testing.T) {
	testParams := []setACparams{
		// Best case: No errors
		{
			`{"activity":"testDefinition","cost":321,"unit":"",
			"timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`,
			false, "Best case, no errors",
		},
		// Bad case: Fail @ unmarshal
		{"", true, "Bad case, break first unmarshal"},
		// Bad case: No version field in byte array
		{
			`{"activity":"testDefinition","cost":321,"unit":"","timestamp":"0001-01-01T00:00:00Z"}`,
			true, "Bad case, version missing",
		},
		// Bad case: Unsupported version
		{
			`{"activity":"testDefinition","cost":321,"unit":"",
			"timestamp":"0001-01-01T00:00:00Z","version":"WrongVersion"}`,
			true, "Bad case, unsupported version",
		},
		// Bad case: mismatch between 'serv.Definition' and 'acForm.Activity'
		{
			`{"activity":"WrongDef","cost":321,"unit":"",
			"timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`,
			true, "Bad case, serv.Definition != acForm.Activity",
		},
		// Bad case: Fail @ 2nd unmarshal
		{
			`{"activity":"testDefinition","cost":"321","unit":"",
			"timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`,
			true, "Bad case, break first unmarshal",
		},
	}
	testServ := createTestService()

	for _, c := range testParams {
		err := SetActivitiesCost(testServ, []byte(c.dataString))

		if (c.expectError == true && err == nil) || (c.expectError == false && err != nil) {
			t.Errorf("Testcase '%s' failed, expectError was %v error was: %v", c.testCase, c.expectError, err)
		}
	}
}

// ------------------------------------------------------ //
// Helper functions and structs for TestACServices()
// ------------------------------------------------------ //

// Creates a unitasset with values used for testing
func createUnitAsset(cost float64) components.UnitAsset {
	setTest := &components.Service{
		ID:            1,
		Definition:    "test",
		SubPath:       "test",
		Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
		Description:   "A test service",
		RegPeriod:     45,
		RegTimestamp:  "now",
		RegExpiration: "45",
		ACost:         cost,
	}
	ServicesMap := &components.Services{
		setTest.SubPath: setTest,
	}
	var ua components.UnitAsset = &mockUnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
		CervicesMap: nil,
	}
	return ua
}

type acServicesParams struct {
	httpMethod     string
	responseWriter *httptest.ResponseRecorder
	expectError    bool
	request        *http.Request
	unitAsset      components.UnitAsset
	testCase       string
}

func TestACServices(t *testing.T) {
	testParams := []acServicesParams{
		// Good case: no errors in GET/PUT
		{
			"GET", httptest.NewRecorder(), false,
			httptest.NewRequest(
				http.MethodGet,
				"http://localhost",
				io.NopCloser(strings.NewReader(``)),
			),
			createUnitAsset(0), "GET, Best case: no errors in GET",
		},
		{
			"PUT", httptest.NewRecorder(), false,
			httptest.NewRequest(
				http.MethodPut,
				"http://localhost",
				io.NopCloser(strings.NewReader(
					`{"activity":"test", "cost": 321, "version":"ActivityCostForm_v1"}`,
				)),
			),
			createUnitAsset(0), "PUT, Best case: no errors in PUT",
		},
		// GET, Bad case: GetActivitiesCost() returns error
		{
			"GET", httptest.NewRecorder(), true,
			httptest.NewRequest(http.MethodGet, "http://localhost", io.NopCloser(strings.NewReader(``))),
			createUnitAsset(math.NaN()), "GET, Bad case: error from GetActivitiesCost()"},
		// PUT, Bad case: Reading response body returns an error
		{
			"PUT", httptest.NewRecorder(), true,
			httptest.NewRequest(http.MethodPut, "http://localhost", io.NopCloser(errReader(0))),
			createUnitAsset(0), "PUT, Bad case: reading response body",
		},
		// PUT, Bad case: SetActivitiesCost() returns error
		{
			"PUT", httptest.NewRecorder(), true,
			httptest.NewRequest(http.MethodPut, "http://localhost", io.NopCloser(strings.NewReader(``))),
			createUnitAsset(0), "PUT, Bad case: error updating activities cost",
		},
		// DEFAULT: Method not supported (POST),
		{
			"POST", httptest.NewRecorder(), true,
			httptest.NewRequest(http.MethodPost, "http://localhost", io.NopCloser(strings.NewReader(``))),
			createUnitAsset(0), "POST, Bad case: Method not supported",
		},
		// TODO: GET, Bad case: Couldn't write to responsewriter
	}

	for _, c := range testParams {
		// Setup
		ua := c.unitAsset
		w := c.responseWriter
		r := c.request
		// Test
		ACServices(w, r, &ua, "test")

		if c.expectError == false && w.Result().StatusCode != 200 {
			t.Errorf("Expected statuscode 200 in testcase '%s' got: %d", c.testCase, w.Result().StatusCode)
		}
		if c.expectError == true && w.Result().StatusCode == 200 {
			t.Errorf("Expected statuscode not to be 200 in testcase '%s'", c.testCase)
		}
	}
}
