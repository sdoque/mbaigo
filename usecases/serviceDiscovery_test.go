package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// mockTransport is used for replacing the default network Transport (used by
// http.DefaultClient) and it will intercept network requests.
type mockTransport struct {
	resp *http.Response
}

func newMockTransport(resp *http.Response) mockTransport {
	t := mockTransport{
		resp: resp,
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = t
	return t
}

// RoundTrip method is required to fulfil the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network and count how many times
// a domain was requested.
func (t mockTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	t.resp.Request = req
	return t.resp, nil
}

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

func createTestSystem(ctx context.Context) components.System {
	// instantiate the System
	sys := components.NewSystem("testSystem", ctx)

	// Instatiate the Capusle
	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
	}
	return sys
}

func TestFillQuestForm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)
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

// Create a error reader to break json.unmarshal
type errReader int

var errBodyRead error = fmt.Errorf("bad body read")

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errBodyRead
}
func (errReader) Close() error {
	return nil
}

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
