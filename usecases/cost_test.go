package usecases

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// GetActivitiesCost(serv *components.Service) (payload []byte, err error)
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

// SetActivitiesCost(serv *components.Service, bodyBytes []byte) (err error)
func TestSetActivitiesCost(t *testing.T) {
	testParams := []setACparams{
		// Best case: No errors
		{`{"activity":"testDefinition","cost":321,"unit":"","timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`, false, "Best case, no errors"},
		// Bad case: Fail @ unmarshal
		{"", true, "Bad case, break first unmarshal"},
		// Bad case: No version field in byte array
		{`{"activity":"testDefinition","cost":321,"unit":"","timestamp":"0001-01-01T00:00:00Z"}`, true, "Bad case, version missing"},
		// Bad case: Unsupported version
		{`{"activity":"testDefinition","cost":321,"unit":"","timestamp":"0001-01-01T00:00:00Z","version":"WrongVersion"}`, true, "Bad case, unsupported version"},
		// Bad case: mismatch between 'serv.Definition' and 'acForm.Activity'
		{`{"activity":"WrongDef","cost":321,"unit":"","timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`, true, "Bad case, serv.Definition != acForm.Activity"},
		// Bad case: Fail @ 2nd unmarshal
		{`{"activity":"testDefinition","cost":"321","unit":"","timestamp":"0001-01-01T00:00:00Z","version":"ActivityCostForm_v1"}`, true, "Bad case, break first unmarshal"},
	}
	testServ := createTestService()

	for _, c := range testParams {
		// Test
		err := SetActivitiesCost(testServ, []byte(c.dataString))
		if c.expectError != true {
			if err != nil {
				t.Errorf("Expected no errors in testcase '%s', got: %v", c.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("Expected errors in testcase '%s'", c.testCase)
			}
		}
	}
}

// Creates a unitasset with values used for testing
func createUnitAsset() components.UnitAsset {
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
	var ua components.UnitAsset
	ua = &mockUnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
		CervicesMap: nil,
	}
	return ua
}

type acServicesParams struct {
	httpMethod  string
	expectError bool
	body        string
	testCase    string
}

// ACServices(w http.ResponseWriter, r *http.Request, ua *components.UnitAsset, serviceP string)
func TestACServices(t *testing.T) {
	testParams := []acServicesParams{
		// Good case: no errors in GET/PUT
		{"GET", false, "", "Best case: no errors in GET"},
		{"PUT", false, `{"activity":"test", "cost": 321, "version":"ActivityCostForm_v1"}`, "Best case: no errors in PUT"},
		// GET, Bad case: GetActivitiesCost() returns error
		// GET, Bad case: Couldn't write to responsewriter
		// PUT, Bad case: Reading response body returns an error
		// PUT, Bad case: SetActivitiesCost() returns error
		// DEFAULT: Method not supported (POST)
	}

	for _, c := range testParams {
		// Setup
		ua := createUnitAsset()
		w := httptest.NewRecorder()
		body := io.NopCloser(strings.NewReader(c.body))
		r := httptest.NewRequest(c.httpMethod, "http://localhost", body)
		// Test
		ACServices(w, r, &ua, "test")
		if c.expectError == false {
			if w.Result().StatusCode != 200 {
				t.Errorf("Expected no errors in '%s'", c.testCase)
			}
		} else {
			if r.Response.StatusCode == 200 {
				t.Errorf("Expected errors in testcase '%s'", c.testCase)
			}
		}
	}
}
