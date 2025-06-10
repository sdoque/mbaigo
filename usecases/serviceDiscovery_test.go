package usecases

import (
	"context"
	"encoding/json"
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
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = t
	return t
}

// RoundTrip method is required to fulfil the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network and count how many times
// a domain was requested.
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

var errHTTP error = fmt.Errorf("bad http request")

type mockUnitAsset struct {
}

func (mua mockUnitAsset) GetName() string {
	return "Test UnitAsset"
}

func (mua mockUnitAsset) GetServices() components.Services {
	return nil
}
func (mua mockUnitAsset) GetCervices() components.Cervices {
	return nil
}

func (mua mockUnitAsset) GetDetails() map[string][]string {
	return map[string][]string{
		"Details": []string{"test1", "test2"},
	}
}

func (mua mockUnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	return
}

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
	mua := mockUnitAsset{}
	questForm := FillQuestForm(&testSys, mua, "TestDef", "TestProtocol")
	// Loop through the details in questForm and mua (mockUnitAsset), error if they're same
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

func TestExtractQuestForm(t *testing.T) {
	body := testBodyHasVersion{
		Version: "ServiceQuest_v1",
	}
	data, _ := json.Marshal(body)

	// Everything passes, best outcome
	rec, _ := ExtractQuestForm(data)
	if rec.Version != body.Version {
		t.Errorf("Expected version: %s, got: %s", rec.Version, body.Version)
	}

	// Can't unmarshal data
	data, _ = json.Marshal(errReader(0))
	rec, err := ExtractQuestForm(data)
	if err == nil {
		t.Errorf("Expected error during unmarshal")
	}
	// Missing version
	noVersionBody := testBodyNoVersion{}
	data, _ = json.Marshal(noVersionBody)
	rec, err = ExtractQuestForm(data)
	if rec.Version != "" {
		t.Errorf("Expected no version, got %s", rec.Version)
	}
	// Error while writing to correct form
	protocolBody := testBodyHasProtocol{
		Version:  "ServiceQuest_v1",
		Protocol: 123,
	}
	data, _ = json.Marshal(protocolBody)
	rec, err = ExtractQuestForm(data)
	if err == nil {
		t.Errorf("Expected Error during unmarshal in switch case")
	}

	// Switch case: Unsupported service registration form
	body = testBodyHasVersion{
		Version: "",
	}
	data, _ = json.Marshal(body)
	rec, err = ExtractQuestForm(data)
	if err == nil {
		t.Errorf("Expected error in switch case (Unsupported form version)")
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
	newMockTransport(resp, false, nil)
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
	newMockTransport(resp, true, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	qForm.NewForm()
	_, err = sendHttpReq(http.MethodPost, "https://test", jsonQF, ctx)
	if err == nil {
		t.Errorf("Expected errors while sending http request")
	}
	cancel()
}

/*
	type ServiceQuest_v1 struct {
		SysId             int                 `json:"systemId"`
		RequesterName     string              `json:"requesterName"`
		ServiceDefinition string              `json:"serrviceDefinition"`
		Protocol          string              `json:"protocol"`
		Details           map[string][]string `json:"details"`
		Version           string              `json:"version"`
		Break             any                 `json:"break"`
	}
*/
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
	newMockTransport(resp, false, nil)
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
	newMockTransport(resp, true, errHTTP)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	_, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error at GetRunningCoreSystemURL()")
	}
	cancel()

	// Error at sendHttpRequest
	// Can't be tested

	// Error at "Read the response", io.ReadAll()
	f = createServicePointTestForm()
	resp.Body = errReader(0)
	newMockTransport(resp, false, nil)
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
	newMockTransport(resp, false, nil)
	ctx, cancel = context.WithCancel(context.Background())
	testSys = createTestSystem(ctx, false)
	qForm.NewForm()
	serviceForm, err = Search4Service(qForm, &testSys)
	if err == nil {
		t.Errorf("Expected error")
	}
	cancel()

}

func TestSearch4Services(t *testing.T) {
	return
}

/*
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
*/

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
	// Bad case: wrong form version
	spForm.Version = ""
	data, err = json.Marshal(spForm)
	if err != nil {
		t.Errorf("Error occurred while marshaling the test form")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected error because of wrong form version")
	}

	// Bad case: error when unmarshalling body
	data, err = json.Marshal("Test")
	if err != nil {
		t.Errorf("Error when marshalling in test")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected errors for broken unmarshal")
	}

	// Bad case: error when unmarshalling body
	var emptyForm forms.Form
	data, err = json.Marshal(emptyForm)
	if err != nil {
		t.Errorf("Error when marshalling in test")
	}
	form, err = ExtractDiscoveryForm(data)
	if err == nil {
		t.Errorf("Expected errors for missing form")
	}
}
