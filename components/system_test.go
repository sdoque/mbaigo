package components

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewSystem(t *testing.T) {
	name := "TestingSystem"
	ctx, cancel := context.WithCancel(context.Background())
	sys := NewSystem(name, ctx)

	if sys.Name != name {
		t.Errorf("expected system name %s, got %s", name, sys.Name)
	}

	// It's a bit of a silly test but the system context is an important dependency
	// for cancelling some background services (system registration and http servers).
	select {
	case <-sys.Ctx.Done():
		t.Fatal("expected context to NOT be cancelled")
	default:
		// pass
	}

	cancel()
	select {
	case <-sys.Ctx.Done():
		// pass
	default:
		t.Error("expected context to be cancelled")
	}
}

////////////////////////////////////////////////////////////////////////////////

type errorReadCloser struct {
	r        io.Reader
	errRead  error
	errClose error
}

func (ec errorReadCloser) Read(p []byte) (n int, err error) {
	if ec.errRead != nil {
		return 0, ec.errRead
	}
	return ec.r.Read(p)
}

func (ec errorReadCloser) Close() error {
	return ec.errClose
}

var errMockTrans = fmt.Errorf("mock error")

type mockTrans struct {
	status       int
	body         string
	err          error
	errBody      error
	errBodyClose error
}

func newMockTransport() *mockTrans {
	t := &mockTrans{
		status: http.StatusOK,
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = t
	return t
}

func (t *mockTrans) setResponse(status int, body string) {
	t.status = status
	t.body = body
}

func (t *mockTrans) setError() {
	t.err = errMockTrans
}

func (t *mockTrans) setBodyError() {
	t.errBody = errMockTrans
}

func (t *mockTrans) setBodyCloseError() {
	t.errBodyClose = errMockTrans
}

// RoundTrip method is required to fulfil the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network.
func (t *mockTrans) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.err != nil {
		return nil, t.err
	}
	resp := &http.Response{
		StatusCode: t.status,
		Status:     http.StatusText(t.status),
		Body: errorReadCloser{
			strings.NewReader(t.body),
			t.errBody,
			t.errBodyClose,
		},
		ContentLength: int64(len(t.body)),
		Request:       req,
	}
	return resp, nil
}

const coreRegURL = "http://registrar"
const coreFakeURL = "http://fake"

var coreReg = &CoreSystem{ServiceRegistrarName, coreRegURL}
var coreFake = &CoreSystem{"fakesystem", coreFakeURL}

type sampleGetRunningCoreSystem struct {
	name    string
	url     string
	wantErr bool
	setup   func(*mockTrans)
}

var tableGetRunningCoreSystem = []sampleGetRunningCoreSystem{
	// Tests for non-registrars
	// Case: url.Parse() error
	{coreFake.Name, "", true, func(m *mockTrans) { coreFake.Url = string(rune(0)) }},
	// Case: http.Get() error
	{coreFake.Name, "", true, func(m *mockTrans) { m.setError() }},
	// Case: io.ReadAll() error
	{coreFake.Name, "", true, func(m *mockTrans) { m.setBodyError() }},
	// Case: http < 200 error
	{coreFake.Name, "", true, func(m *mockTrans) { m.setResponse(199, "") }},
	// Case: http > 299 error
	{coreFake.Name, "", true, func(m *mockTrans) { m.setResponse(300, "") }},
	// Case: return url
	{coreFake.Name, coreFake.Url, false, nil},

	// Tests for registrars
	// Case: url.Parse() error
	{coreReg.Name, "", true, func(m *mockTrans) { coreReg.Url = string(rune(0)) }},
	// Case: http.Get() error
	{coreReg.Name, "", true, func(m *mockTrans) { m.setError() }},
	// Case: io.ReadAll() error
	{coreReg.Name, "", true, func(m *mockTrans) { m.setBodyError() }},
	// Case: http < 200 error
	{coreReg.Name, "", true, func(m *mockTrans) { m.setResponse(199, "") }},
	// Case: http > 299 error
	{coreReg.Name, "", true, func(m *mockTrans) { m.setResponse(300, "") }},
	// Case: return error when missing prefix string in body for registrar
	{coreReg.Name, "", true, nil},
	// Case: return url
	{coreReg.Name, coreReg.Url, false, func(m *mockTrans) {
		m.setResponse(200, ServiceRegistrarLeader)
	}},
}

func TestGetRunningCoreSystem(t *testing.T) {
	name := "testSystem"
	sys := NewSystem(name, context.Background())

	// Case: return error for empty core system list (and should not match itself)
	if len(sys.CoreS) != 0 {
		t.Fatalf("expected no core systems, had %d in list", len(sys.CoreS))
	}
	_, err := GetRunningCoreSystemURL(&sys, name)
	if err == nil {
		t.Error("expected error, got nil")
	}
	sys.CoreS = []*CoreSystem{coreReg, coreFake}

	for _, test := range tableGetRunningCoreSystem {
		coreReg.Url = coreRegURL // reset URLs after testing url.Parse() errors
		coreFake.Url = coreFakeURL
		m := newMockTransport()
		if test.setup != nil {
			test.setup(m)
		}

		gotURL, gotErr := GetRunningCoreSystemURL(&sys, test.name)
		switch {
		case test.wantErr == (gotErr == nil):
			t.Errorf("expected error = %v, got: %v", test.wantErr, gotErr)
		case gotURL != test.url:
			t.Errorf("expected core system URL '%s', got '%s'", test.url, gotURL)
		}
	}
}
