package usecases

import (
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

// SetActivitiesCost(serv *components.Service, bodyBytes []byte) (err error)
func TestSetActivitiesCost(t *testing.T) {
	// Forms, with and without version
	// SwitchCase: formtype, ActivityCostForm_v1 or not
	// Check if serv.Definition and ActivityCostForm.Activity is equal
}

// ACServices(w http.ResponseWriter, r *http.Request, ua *components.UnitAsset, serviceP string)
func TestACServices(t *testing.T) {

}
