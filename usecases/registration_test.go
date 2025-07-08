package usecases

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("forced read error")
}

func manualEqualityCheck(map1 map[string][]string, map2 map[string][]string) error {
	if len(map1) != len(map2) {
		return fmt.Errorf("Expected map length %d, got %d", len(map2), len(map1))
	}
	for key, value := range map2 {
		mv, ok := map1[key]
		if !ok {
			return fmt.Errorf("Expected key %q not found in merged map", key)
		}
		if len(mv) != len(value) {
			return fmt.Errorf("For key %q, expected slice length %d, got %d", key, len(value), len(mv))
		}
		for i := range value {
			if mv[i] != value[i] {
				return fmt.Errorf("For key %q, at index %d, expected %q, got %q", key, i, value[i], mv[i])
			}
		}
	}
	for key := range map1 {
		if _, ok := map2[key]; !ok {
			return fmt.Errorf("Unexpected key %q found in merged map", key)
		}
	}
	return nil
}

type deepCopyMapTestStruct struct {
	mockSystem components.System
	testName   string
}

var deepCopyMapTestParams = []deepCopyMapTestStruct{
	{createTestSystem(false), "Good case, the copy works as a deep copy"},
}

func TestDeepCopyMap(t *testing.T) {
	for _, testCase := range deepCopyMapTestParams {
		mua := testCase.mockSystem.UAssets["testUnitAsset"]
		original := (*mua).GetDetails()

		test := deepCopyMap((*mua).GetDetails())

		// If they are not equal from the beginning then the copy was not successful
		err := manualEqualityCheck(original, test)
		if err != nil {
			t.Errorf("In test case: %s: Expected deep copied map to be equal to original, Expected: %v, got: %v", testCase.testName, original, test)
		}

		// When we change something in the original, the deep copied map should not change
		original["Test"][0] = "changed original"
		err = manualEqualityCheck(original, test)
		if err == nil {
			t.Errorf("In test case: %s: Deep copy failed, changes in original affected the deep copied map. Expected: %v, got %v", testCase.testName, original, test)
		}
		original["Test"][0] = "test"

		// When we change something in the deep copied map, the original should not change
		test["Test"][0] = "changed deep copy"
		err = manualEqualityCheck(original, test)
		if err == nil {
			t.Errorf("In test case: %s: Deep copy failed, changes in deep copied map affected the original. Expected: %v, got %v", testCase.testName, original, test)
		}
	}
}

type serviceRegistrationFormTestStruct struct {
	version     string
	expectedErr bool
	testName    string
}

var serviceRegistrationFormTestParams = []serviceRegistrationFormTestStruct{
	{"ServiceRecord_v1", false, "Good case, everything works"},
	{"Wrong version", true, "Bad case, the wrong version string is sent in"},
}

func TestServiceRegistrationForm(t *testing.T) {
	for _, testCase := range serviceRegistrationFormTestParams {
		testSys := createTestSystem(false)
		mua := testSys.UAssets["testUnitAsset"]
		serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]

		payload, err := serviceRegistrationForm(&testSys, mua, serv, testCase.version)
		if (testCase.expectedErr == true && err == nil) || (testCase.expectedErr == false && err != nil) {
			t.Errorf("In test case: %s: Expected %t error, got: %v", testCase.testName, testCase.expectedErr, err)
		}

		if testCase.expectedErr == false {
			var sr forms.ServiceRecord_v1
			if err = json.Unmarshal(payload, &sr); err != nil {
				t.Fatalf("Invalid JSON: %v", err)
			}

			// Check that the ServiceNode is created correctly
			expectedNode := testSys.Host.Name + "_" + testSys.Name + "_" +
				(*testSys.UAssets["testUnitAsset"]).GetName() + "_" +
				(*testSys.UAssets["testUnitAsset"]).GetServices()["test"].Definition
			if sr.ServiceNode != expectedNode {
				t.Errorf("Expected ServiceNode %q, got: %q", expectedNode, sr.ServiceNode)
			}

			// Check that the ProtoPorts that are equal to 0 gets removed
			if len(sr.ProtoPort) != 1 {
				t.Errorf("Expected: one proto port (excluding 0s), got: %v", sr.ProtoPort)
			}

			// Check that the unit asset details exists and are ok
			if v, ok := sr.Details["Test"]; !ok || len(v) != 1 {
				t.Errorf("Missing or incorrect unit asset details. Expected: %v, got: %v", (*mua).GetDetails(), v)
			}

			// Check that the service forms exists and are ok
			if v, ok := sr.Details["Forms"]; !ok || len(v) != 1 {
				t.Errorf("Missing or incorrect service forms. Expected: %v, got: %v", (*serv).Details, v)
			}
		}
	}

	// Special case
	// Check that when the Service RegPeriod equals 0, ServiceRegistrationForm defaults to its RegLife default value of 30
	testSys := createTestSystem(false)
	mua := testSys.UAssets["testUnitAsset"]
	serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]
	(*testSys.UAssets["testUnitAsset"]).GetServices()["test"].RegPeriod = 0
	version := "ServiceRecord_v1"
	payload, err := serviceRegistrationForm(&testSys, mua, serv, version)
	if err != nil {
		t.Fatalf("The Service Record version was wrong.")
	}
	var sr forms.ServiceRecord_v1
	if err := json.Unmarshal(payload, &sr); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if sr.RegLife != 30 {
		t.Errorf("Expected RegLife: 30, got: %d", sr.RegLife)
	}
}

type unregisterServiceTestStruct struct {
	registrarUrl     string
	expectedErr      bool
	mockTransportErr int
	errHTTP          error
	testName         string
}

var unregisterServiceTestParams = []unregisterServiceTestStruct{
	{"https://leadingregistrar", false, 0, nil, "Good case, an unregistered service tries to unregister"},
	{"https://leadingregistrar", false, 0, nil, "Good case, an registered service tries to unregister"},
	{"https://leadingregistrar", true, 1, errHTTP, "Bad case, error in response body"},
	{"", false, 0, nil, "Good case, no leading registrar URL was sent in"},
	{brokenUrl, true, 0, nil, "Bad case, broken URL"},
}

func TestUnregisterService(t *testing.T) {
	for _, testCase := range unregisterServiceTestParams {
		testSys := createTestSystem(false)
		serv := (*testSys.UAssets["testUnitAsset"]).GetServices()["test"]

		newMockTransport(createWorkingHttpResp(), testCase.mockTransportErr, testCase.errHTTP)
		err := unregisterService(testCase.registrarUrl, serv)
		if (testCase.expectedErr == true && err == nil) || (testCase.expectedErr == false && err != nil) {
			t.Errorf("In test case: %s: We expected %t error, got: %v", testCase.testName, testCase.expectedErr, err)
		}
	}
}

func TestServiceRegistrationFormList(t *testing.T) {
	list := []string{
		"ServiceRecord_v1",
	}
	// Check that the return value of ServiceRegistrationFormsList is equal to the expected list of ServiceRegistrationForms
	test := ServiceRegistrationFormsList()
	for i := range list {
		if list[i] != test[i] {
			t.Errorf("Expected lists to be equal. Expected: %v, got: %v", list, test)
			break
		}
	}
}

type registerServiceTestStruct struct {
	registrarUrl     string
	contentType      string
	mockServID       int
	correctTime      bool
	brokenBody       bool
	expectedErr      bool
	mockTransportErr int
	errHTTP          error
	testName         string
}

func createWorkingRegisterServiceBody(mua *components.UnitAsset, serv *components.Service, correctTime bool, contentType string, brokenBody bool) func() *http.Response {
	payload, err := serviceRegistrationForm(&testSys, mua, serv, "ServiceRecord_v1")
	if err != nil {
		log.Fatalf("The service Record version was wrong")
	}
	var sr forms.ServiceRecord_v1
	if err = json.Unmarshal(payload, &sr); err != nil {
		log.Fatalf("Invalid JSON: %v", err)
	}
	if correctTime == true {
		sr.EndOfValidity = time.Now().Format(time.RFC3339)
	} else {
		sr.EndOfValidity = ""
	}

	fakebody, err := json.Marshal(sr)
	if err != nil {
		log.Fatalf("Fail marshal at start of test: %v", err)
	}

	if brokenBody == false {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(string(fakebody))),
			}
		}
		return respFunc
	} else {
		respFunc := func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(errorReader{}),
			}
		}
		return respFunc
	}
}

func createMockSysMockUnitAssetandMockService(id int) (mockSys components.System, mua *components.UnitAsset, mockServ *components.Service) {
	mockSys = createTestSystem(false)
	mua = mockSys.UAssets["testUnitAsset"]
	mockServ = (*mockSys.UAssets["testUnitAsset"]).GetServices()["test"]
	mockServ.ID = id
	return
}

var registerServiceTestParams = []registerServiceTestStruct{
	{"https://leadingregistrar", "application/json", 1, true, false, false, 0, nil, "Good case, with PUT method"},
	{"https://leadingregistrar", "application/json", 0, true, false, false, 0, nil, "Good case, with POST method"},
	{"https://leadingregistrar", "application/json", 1, true, false, true, 1, timeoutError{}, "Bad case, timeout error"},
	{"https://leadingregistrar", "application/json", 1, true, false, true, 1, errHTTP, "Bad case, error in defaultClint"},
	{"https://leadingregistrar", "application/json", 1, true, true, true, 0, nil, "Bad case, error in ReadAll"},
	{"https://leadingregistrar", "", 1, true, false, true, 0, nil, "Bad case, error in Unpack"},
	{"https://leadingregistrar", "application/json", 1, false, false, true, 0, nil, "Bad case, error parsing time"},
	{"", "application/json", 1, true, false, false, 0, nil, "Good case, no leading registrar URL sent in"},
	{brokenUrl, "application/json", 1, true, false, true, 0, nil, "Bad case, broken URL with PUT method"},
	{brokenUrl, "application/json", 0, true, false, true, 0, nil, "Bad case, broken URL with POST method"},
}

var delay = time.Duration(15) * time.Second

func TestRegisterService(t *testing.T) {
	for _, testCase := range registerServiceTestParams {
		mockSys, mua, mockServ := createMockSysMockUnitAssetandMockService(testCase.mockServID)
		respFunc := createWorkingRegisterServiceBody(mua, mockServ, testCase.correctTime, testCase.contentType, testCase.brokenBody)
		newMockTransport(respFunc, testCase.mockTransportErr, testCase.errHTTP)

		test, err := registerService(&mockSys, testCase.registrarUrl, mua, mockServ)

		// Special case
		if testCase.registrarUrl == "" {
			if err != nil || test != delay {
				t.Errorf("In test case: %s: Did we expect error? %t, got: %v and %d delay.", testCase.testName, testCase.expectedErr, err, test)
			}
			continue
		}

		if testCase.expectedErr == false {
			if err != nil || test == delay {
				t.Errorf("In test case: %s: Did we expect error? %t, got: %v and %d delay.", testCase.testName, testCase.expectedErr, err, test)
			}
		} else {
			if err == nil || test != delay {
				t.Errorf("In test case: %s: Did we expect error? %t, got: %v and %d delay.", testCase.testName, testCase.expectedErr, err, test)
			}
		}
	}
}
