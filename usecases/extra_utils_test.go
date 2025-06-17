package usecases

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sdoque/mbaigo/components"
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

// A mocked UnitAsset used for testing
type mockUnitAsset struct {
	Name        string              `json:"name"`    // Must be a unique name, ie. a sensor ID
	Owner       *components.System  `json:"-"`       // The parent system this UA is part of
	Details     map[string][]string `json:"details"` // Metadata or details about this UA
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
}

func (mua mockUnitAsset) GetName() string {
	return mua.Name
}

func (mua mockUnitAsset) GetServices() components.Services {
	return mua.ServicesMap
}

func (mua mockUnitAsset) GetCervices() components.Cervices {
	return mua.CervicesMap
}

func (mua mockUnitAsset) GetDetails() map[string][]string {
	return mua.Details
}

func (mua mockUnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {}

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
var brokenUrl = string([]byte{0x7f})
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
	}

	// create fake services and cervices for a mocked unit asset
	testCerv := &components.Cervice{
		Definition: "testCerv",
		Details:    map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:      map[string][]string{},
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
	mua := &mockUnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
		CervicesMap: *CervicesMap,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	var muaInterface components.UnitAsset = mua
	sys.UAssets[mua.GetName()] = &muaInterface

	leadingRegistrar := &components.CoreSystem{
		Name: "serviceregistrar",
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
		sys.CoreS = []*components.CoreSystem{
			leadingRegistrar,
			orchestrator,
			test,
		}
	} else {
		orchestrator := &components.CoreSystem{
			Name: "orchestrator",
			Url:  brokenUrl,
		}
		sys.CoreS = []*components.CoreSystem{
			leadingRegistrar,
			orchestrator,
			test,
		}
	}
	return
}
