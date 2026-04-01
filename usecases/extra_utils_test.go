package usecases

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// mockTransport is used for replacing the default network Transport (used by
// http.DefaultClient) and it will intercept network requests.
type mockTransport struct {
	respFunc func() *http.Response
	hits     int
	err      error
}

func newMockTransport(respFunc func() *http.Response, v int, err error) *mockTransport {
	t := &mockTransport{
		respFunc: respFunc,
		hits:     v,
		err:      err,
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
	if t.hits == 0 {
		return resp, t.err
	}
	resp = t.respFunc()
	resp.Request = req
	return resp, nil
}

// A mocked form used for testing
type mockForm struct {
	XMLName xml.Name `json:"-" xml:"testName"`
	Value   any      `json:"value" xml:"value"`
	Unit    string   `json:"unit" xml:"unit"`
	Version string   `json:"version" xml:"version"`
}

// NewForm creates a new form
func (f mockForm) NewForm() forms.Form {
	f.Version = "testVersion"
	return f
}

// FormVersion returns the version of the form
func (f mockForm) FormVersion() string {
	return f.Version
}

// Create a error reader to break json.Unmarshal()
type errReader int

var errBodyRead error = fmt.Errorf("bad body read")

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errBodyRead
}
func (errReader) Close() error {
	return nil
}

// Variables used in testing
var brokenUrl = string(rune(0))
var errHTTP error = fmt.Errorf("bad http request")

// Help function to create a test system
func createTestSystem(broken bool) (sys components.System) {
	// instantiate the System
	ctx := context.Background()
	sys = components.NewSystem("testSystem", ctx)

	// Instantiate the Capsule
	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
		Host:        components.NewDevice(),
		Messengers:  make(map[string]int),
	}

	// create fake services and cervices for a mocked unit asset
	testCerv := &components.Cervice{
		Definition: "testCerv",
		Details:    map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:      map[string][]components.NodeInfo{},
	}

	CervicesMap := &components.Cervices{
		testCerv.Definition: testCerv,
	}
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
	mua := &components.UnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
		CervicesMap: *CervicesMap,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	sys.UAssets[mua.GetName()] = mua

	leadingRegistrar := &components.CoreSystem{
		Name: components.ServiceRegistrarName,
		Url:  "https://leadingregistrar",
	}
	test := &components.CoreSystem{
		Name: "test",
		Url:  "https://test",
	}
	if broken == false {
		orchestrator := &components.CoreSystem{
			Name: "orchestrator",
			Url:  "https://orchestator",
		}
		sys.Husk.CoreS = []*components.CoreSystem{
			leadingRegistrar,
			orchestrator,
			test,
		}
	} else {
		orchestrator := &components.CoreSystem{
			Name: "orchestrator",
			Url:  brokenUrl,
		}
		sys.Husk.CoreS = []*components.CoreSystem{
			leadingRegistrar,
			orchestrator,
			test,
		}
	}
	return
}
