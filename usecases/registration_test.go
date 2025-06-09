package usecases

import (
	//"bytes"
	"context"
	//"encoding/json"
	//"errors"
	//"fmt"
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

func createTestSystem(ctx context.Context) components.System {
	sys := components.NewSystem("testSystem", ctx)

	sys.Husk = &components.Husk{
		Description: "A test system",
		Details:     map[string][]string{"Developer": {"Test dev"}},
		ProtoPort:   map[string]int{"https": 0, "http": 1234, "coap": 0},
		InfoLink:    "https://for.testing.purposes",
	}

	orchestrator := &components.CoreSystem{
		Name:        "orchestrator",
		Url:         "https://orchestrator",
		Certificate: "",
	}
	leadingRegistrar := &components.CoreSystem{
		Name:        "serviceregistrar",
		Url:         "https://leadingregistrar",
		Certificate: "",
	}
	test := &components.CoreSystem{
		Name:        "test",
		Url:         "https://test",
		Certificate: "",
	}
	sys.CoreS = []*components.CoreSystem{
		orchestrator,
		leadingRegistrar,
		test,
	}
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

	result := DiscoverLeadingRegistrar(&testSys, testURL, false)
	if result != true {
		t.Errorf("Expected %t, got: %t", true, result)
	}

	/*
		statusCode = http.StatusBadRequest
		responseBody = "lead Service Registrar since"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

	statusCode = http.StatusOK
	responseBody = "wrong response"
	testURL = ts.URL
	result = DiscoverLeadingRegistrar(&testSys, testURL, false)
	if result != false {
		t.Errorf("Expected %t, got: %t", false, result)
	}

	/*
		statusCode = http.StatusBadRequest
		responseBody = "wrong response"
		result = DiscoverLeadingRegistrar(&testSys, ts.URL, false)
		if result != false {
			t.Errorf("Expected %t, got: %t", false, result)
		}
	*/

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
