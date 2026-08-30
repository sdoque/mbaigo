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
		t.Fatal("expected context to NOT be canceled")
	default:
		// pass
	}

	cancel()
	select {
	case <-sys.Ctx.Done():
		// pass
	default:
		t.Error("expected context to be canceled")
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
	status  int
	body    string
	err     error
	errBody error
	// routes answer particular URLs differently from the default, so one
	// transport can play a standby and the lead it refers to.
	routes map[string]mockAnswer
}

type mockAnswer struct {
	status int
	body   string
}

func (t *mockTrans) route(url string, status int, body string) {
	if t.routes == nil {
		t.routes = map[string]mockAnswer{}
	}
	t.routes[url] = mockAnswer{status, body}
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

// RoundTrip method is required to fulfill the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network.
func (t *mockTrans) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.err != nil {
		return nil, t.err
	}
	status, body := t.status, t.body
	if a, routed := t.routes[req.URL.String()]; routed {
		status, body = a.status, a.body
	}
	resp := &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body: errorReadCloser{
			strings.NewReader(body),
			t.errBody,
			nil,
		},
		ContentLength: int64(len(body)),
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
	// Case: unrelated system
	{"bad name", "", true, nil},
	// Case: url.Parse() error
	{coreFake.Name, "", true, func(m *mockTrans) { coreFake.Url = string(rune(0)) }},
	// Case: http.Get() no error
	{coreFake.Name, coreFake.Url, false, func(m *mockTrans) { m.setError() }},
	// Case: io.ReadAll() no error
	{coreFake.Name, coreFake.Url, false, func(m *mockTrans) { m.setBodyError() }},
	// Case: http < 200 no error
	{coreFake.Name, coreFake.Url, false, func(m *mockTrans) { m.setResponse(199, "") }},
	// Case: http > 299 no error
	{coreFake.Name, coreFake.Url, false, func(m *mockTrans) { m.setResponse(300, "") }},
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
	},
	},
}

func TestGetRunningCoreSystem(t *testing.T) {
	name := "testSystem"
	sys := NewSystem(name, context.Background())
	sys.Husk = &Husk{}

	// Case: return error for empty core system list (and should not match itself)
	if len(sys.Husk.CoreS) != 0 {
		t.Fatalf("expected no core systems, had %d in list", len(sys.Husk.CoreS))
	}
	_, err := GetRunningCoreSystemURL(&sys, name)
	if err == nil {
		t.Error("expected error, got nil")
	}
	sys.Husk.CoreS = []*CoreSystem{coreReg, coreFake}

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

// The generated configuration carries an empty authorizer entry so an operator
// can see where the URL goes. Until one is written the cloud has no authorizer,
// and every caller must reach that conclusion: a provider that read the slot as
// present would demand tokens nothing could issue, and would answer 503 to
// everything but its core services for as long as it ran.
func TestAnEmptySlotIsNotACoreSystem(t *testing.T) {
	sys := NewSystem("ds18b20", context.Background())
	sys.Husk = &Husk{CoreS: []*CoreSystem{
		{Name: "authorizer", Url: ""},
		{Name: "ca", Url: "http://192.168.1.10:20100/ca/certification"},
	}}

	if _, err := GetRunningCoreSystemURL(&sys, "authorizer"); err == nil {
		t.Error("an entry with no URL was treated as a running authorizer")
	}
	// Whitespace is the same thing written less obviously.
	sys.Husk.CoreS[0].Url = "   "
	if _, err := GetRunningCoreSystemURL(&sys, "authorizer"); err == nil {
		t.Error("an entry whose URL is only spaces was treated as a running authorizer")
	}
	// And a real one still resolves.
	sys.Husk.CoreS[0].Url = "https://192.168.1.10:30104/authorizer/authorization"
	got, err := GetRunningCoreSystemURL(&sys, "authorizer")
	if err != nil || got != "https://192.168.1.10:30104/authorizer/authorization" {
		t.Errorf("a configured authorizer resolved to %q (%v)", got, err)
	}
}

// A standby registrar is a referral. With a registrar on every host, the one a
// system is configured with is usually its own and usually not the lead; the
// client follows the standby's answer once and asks the destination to confirm.
func TestRegistrarReferral(t *testing.T) {
	sys := NewSystem("testSystem", context.Background())
	sys.Husk = &Husk{CoreS: []*CoreSystem{{ServiceRegistrarName, "http://standby/serviceregistrar/registry"}}}

	m := newMockTransport()
	m.route("http://standby/serviceregistrar/registry/status", http.StatusServiceUnavailable,
		ServiceRegistrarStandby+"http://lead/serviceregistrar/registry")
	m.route("http://lead/serviceregistrar/registry/status", http.StatusOK, ServiceRegistrarLeader+" now")

	got, err := GetRunningCoreSystemURL(&sys, ServiceRegistrarName)
	if err != nil || got != "http://lead/serviceregistrar/registry" {
		t.Fatalf("referral not followed: got %q, %v", got, err)
	}

	// The referral is taken only as far as the next question: a destination
	// that does not answer as the lead is not used on the standby's word.
	m.route("http://lead/serviceregistrar/registry/status", http.StatusServiceUnavailable, "Service Unavailable")
	if got, err := GetRunningCoreSystemURL(&sys, ServiceRegistrarName); err == nil {
		t.Fatalf("a referral to a non-leader was accepted: %q", got)
	}
}

// What was learned comes after what was configured, and is never added twice.
func TestLearnedCoreSystemsFollowConfiguredOnes(t *testing.T) {
	sys := NewSystem("testSystem", context.Background())
	sys.Husk = &Husk{CoreS: []*CoreSystem{{"orchestrator", "http://file/orchestrator/orchestration"}}}

	if !sys.AddLearnedCoreSystem(CoreSystem{"orchestrator", "http://learned/orchestrator/orchestration"}) {
		t.Fatal("a new core system was not added")
	}
	if sys.AddLearnedCoreSystem(CoreSystem{"orchestrator", "http://learned/orchestrator/orchestration"}) {
		t.Fatal("the same URL was added twice")
	}
	if sys.AddLearnedCoreSystem(CoreSystem{"orchestrator", "http://file/orchestrator/orchestration"}) {
		t.Fatal("a URL the file already names was learned as new")
	}
	all := sys.CoreSystems()
	if len(all) != 2 || all[0].Url != "http://file/orchestrator/orchestration" {
		t.Fatalf("the file's entry does not come first: %v", all)
	}
}
