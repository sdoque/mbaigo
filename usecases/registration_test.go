package usecases

import (
	//"bytes"
	"context"
	//"sync"
	"time"

	//"encoding/json"
	//"errors"
	"fmt"
	//"io"
	//"log"
	//"net"
	"net/http"
	"net/http/httptest"

	//"strconv"
	//"strings"
	"testing"
	//"time"

	"github.com/sdoque/mbaigo/components"
	//"github.com/sdoque/mbaigo/forms"
)

type mockTransport struct {
	resp *http.Response
}

func newMockTransport(resp *http.Response) mockTransport {
	t := mockTransport{
		resp: resp,
	}
	http.DefaultClient.Transport = t
	return t
}

func (t mockTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	t.resp.Request = req
	return t.resp, nil
}

type UnitAsset struct {
	Name        string              `json:"name"`    // Must be a unique name, ie. a sensor ID
	Owner       *components.System  `json:"-"`       // The parent system this UA is part of
	Details     map[string][]string `json:"details"` // Metadata or details about this UA
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	test int `json:"-"`
}

func (mua *UnitAsset) GetName() string {
	return mua.Name
}

func (mua *UnitAsset) GetServices() components.Services {
	return mua.ServicesMap
}

func (mua *UnitAsset) GetCervices() components.Cervices {
	return mua.CervicesMap
}

func (mua *UnitAsset) GetDetails() map[string][]string {
	return mua.Details
}

func (mua *UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	return
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("forced read error")
}

func createTestSystem(ctx context.Context) components.System {
	sys := components.NewSystem("testSystem", ctx)

	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
	}

	orchestrator := &components.CoreSystem{
		Name: "orchestrator",
		Url:  "https://orchestrator",
	}
	leadingRegistrar := &components.CoreSystem{
		Name: "serviceregistrar",
		Url:  "https://leadingregistrar",
	}
	test := &components.CoreSystem{
		Name: "test",
		Url:  "https://test",
	}
	sys.CoreS = []*components.CoreSystem{
		orchestrator,
		leadingRegistrar,
		test,
	}

	setTest := &components.Service{
		Definition:  "test",
		SubPath:     "test",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Description: "A test service",
	}
	ServicesMap := &components.Services{
		setTest.SubPath: setTest,
	}
	mua := &UnitAsset{
		Name:        "mockUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	var muaInterface components.UnitAsset = mua
	sys.UAssets[mua.GetName()] = &muaInterface

	return sys
}

func TestDiscoverLeadingRegistrar(t *testing.T) {
	statusCode := http.StatusOK
	responseBody := "lead Service Registrar since"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
	defer ts.Close()
	testURL := ts.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testSys.CoreS[1].Url = testURL

	manualTicker := 10 * time.Millisecond

	resultCh := make(chan *components.CoreSystem, 1)
	defer close(resultCh)
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res.Name != "serviceregistrar" {
			t.Errorf("Expected %s, got: %s", "serviceregistrar", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	statusCode = http.StatusOK
	responseBody = "wrong response"
	testURL = ts.URL
	testSys.CoreS[1].Url = testURL
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res != nil {
			t.Errorf("Expected %s, got: %s", "leadingRegistrar be nil", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	statusCode = http.StatusOK
	responseBody = "lead Service Registrar since"
	testURL = ts.URL
	testSys.CoreS[1].Url = testURL
	ts.Close()
	go func() {
		resultCh <- DiscoverLeadingRegistrar(&testSys, manualTicker)
	}()
	time.Sleep(5 * manualTicker)
	cancel()
	select {
	case res := <-resultCh:
		if res != nil {
			t.Errorf("Expected %s, got: %s", "leadingRegistrar be nil since Get() error", res.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for DiscoverLeadingRegistrar result")
	}

	/*
		result := DiscoverLeadingRegistrar(&testSys, testURL, false)
		if result != true {
			t.Errorf("Expected %t, got: %t", true, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "lead Service Registrar since"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusOK
		responseBody = "wrong response"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "wrong response"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusOK
		responseBody = "lead Service Registrar since"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, true)
		if result != true {
			t.Errorf("Expected %t, got: %t", true, result)
		}

		statusCode = http.StatusOK
		responseBody = "wrong response"
		testURL = ts.URL
		result = DiscoverLeadingRegistrar(&testSys, testURL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	/*
		statusCode = http.StatusBadRequest
		responseBody = "lead Service Registrar since"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}

		statusCode = http.StatusBadRequest
		responseBody = "wrong response"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, true)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/
}

func TestHandleLeadingRegistrar(t *testing.T) {
	statusCode := http.StatusOK
	responseBody := "lead Service Registrar since"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
	defer ts.Close()
	testURL := ts.URL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSys := createTestSystem(ctx)

	testSys.CoreS[1].Url = testURL

	manualTicker := 10 * time.Millisecond

	go func() {
		HandleLeadingRegistrar(&testSys, manualTicker, false)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	go func() {
		HandleLeadingRegistrar(&testSys, manualTicker, true)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
}
