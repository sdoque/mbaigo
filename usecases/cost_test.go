package usecases

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
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
	createData  func() (data []byte, err error)
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

func createACFormBytes(definition string, version string, errRead bool) (data []byte, err error) {
	var body interface{}
	if errRead == true {
		return json.Marshal(errReader(0))
	}
	if len(version) == 0 {
		body = testBodyNoVersion{}
	} else {
		body = forms.ActivityCostForm_v1{Activity: definition, Cost: 321, Version: version}
	}

	data, err = json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SetActivitiesCost(serv *components.Service, bodyBytes []byte) (err error)
func TestSetActivitiesCost(t *testing.T) {
	testParams := []setACparams{
		// Best case: No errors
		{func() (data []byte, err error) {
			return createACFormBytes("testDefinition", "ActivityCostForm_v1", false)
		}, false, "Best case, no errors"},
		// Bad case: Fail @ unmarshal
		{func() (data []byte, err error) {
			return createACFormBytes("testDefinition", "ActivityCostForm_v1", true)
		}, true, "Bad case, break first unmarshal"},
		// Bad case: No version field in byte array
		{func() (data []byte, err error) {
			return createACFormBytes("testDefinition", "", false)
		}, true, "Bad case, version missing"},
		// Bad case: Unsupported version
		{func() (data []byte, err error) {
			return createACFormBytes("testDefinition", "wrongVersion", false)
		}, true, "Bad case, unsupported version"},
		// Bad case: mismatch between 'serv.Definition' and 'acForm.Activity'
		{func() (data []byte, err error) {
			return createACFormBytes("", "ActivityCostForm_v1", false)
		}, true, "Bad case, serv.Definition != acForm.Activity"},
		// TODO: Add testcase to test 2nd unmarshal 'Bad case: Fail @ 2nd unmarshal'
	}
	testServ := createTestService()

	for _, c := range testParams {
		// Setup
		byteArr, err := c.createData()
		if err != nil {
			t.Errorf("failed while creating byte array in setup of testcase '%s'", c.testCase)
		}

		// Test
		err = SetActivitiesCost(testServ, byteArr)
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

// ACServices(w http.ResponseWriter, r *http.Request, ua *components.UnitAsset, serviceP string)
func TestACServices(t *testing.T) {

}
