package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

////////////////////////////////////////////////////////////////////////////////
// The most simplest unit asset

const (
	unitName    string = "randomiser"
	unitService string = "random"
)

// Force type check (fulfilling the interface) at compile time
var _ components.UnitAsset = &uaRandomiser{}

type uaRandomiser struct {
	Name        string              `json:"-"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"-"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
}

// Add required functions to fulfil the UnitAsset interface
func (ua uaRandomiser) GetName() string                  { return ua.Name }
func (ua uaRandomiser) GetServices() components.Services { return ua.ServicesMap }
func (ua uaRandomiser) GetCervices() components.Cervices { return ua.CervicesMap }
func (ua uaRandomiser) GetDetails() map[string][]string  { return ua.Details }
func (ua uaRandomiser) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	if servicePath != unitService {
		http.Error(w, "unknown service path: "+servicePath, http.StatusBadRequest)
		return
	}
	f := forms.SignalA_v1a{
		Value: rand.Float64(),
	}
	b, err := usecases.Pack(f.NewForm(), "application/json")
	if err != nil {
		http.Error(w, "error from Pack: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(b); err != nil {
		http.Error(w, "error from Write: "+err.Error(), http.StatusInternalServerError)
	}
}

func addUATemplate(sys *components.System) {
	s := &components.Service{
		Definition: unitService, // The "name" of the service
		SubPath:    unitService, // Not "allowed" to be changed afterwards
		Details:    map[string][]string{"key1": {"value1"}},
		RegPeriod:  60,
		// NOTE: must start with lower-case, it gets embedded into another sentence in the web API
		Description: "returns a random float64",
	}
	ua := components.UnitAsset(&uaRandomiser{
		Name:    unitName, // WARN: don't use the system name!! this is an asset!
		Details: map[string][]string{"key2": {"value2"}},
		ServicesMap: components.Services{
			s.SubPath: s,
		},
	})
	sys.UAssets[ua.GetName()] = &ua
}

func loadUA(ca usecases.ConfigurableAsset, sys *components.System) (components.UnitAsset, func()) {
	s := ca.Services[0]
	ua := &uaRandomiser{
		Name:        ca.Name,
		Owner:       sys,
		Details:     ca.Details,
		ServicesMap: usecases.MakeServiceMap(ca.Services),
		// Let it consume its own service
		CervicesMap: components.Cervices{unitService: &components.Cervice{
			Definition: s.Definition,
			Details:    s.Details,
			// Nodes will be filled up by any discovered cervices
			Nodes: make(map[string][]string, 0),
		}},
	}
	return ua, func() {}
}

////////////////////////////////////////////////////////////////////////////////
// The most simplest system

const (
	systemName string = "test"
	systemPort int    = 29999
)

var serviceURL = "GET /" + path.Join(systemName, unitName, unitService)

func newSystem() (*components.System, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	// TODO: want this to return a pointer type instead!
	// easier to use and pointer is used all the time anyway down below
	sys := components.NewSystem(systemName, ctx)
	sys.Husk = &components.Husk{
		Description: " is the most simplest system possible",
		Details:     map[string][]string{"key3": {"value3"}},
		ProtoPort:   map[string]int{"http": systemPort},
	}

	// Setup default config with default unit asset and values
	addUATemplate(&sys)
	rawResources, err := usecases.Configure(&sys)

	// Extra check to work around "created config" error. Not required normally!
	if err != nil {
		// TODO: once configuration PR is merged, check for ErrCreatedConfig blah instead
		if !strings.Contains(err.Error(), "a new configuration file") {
			cancel()
			return nil, nil, err
		}
		// Since Configure() created the config file, it must be cleaned up when this test is done!
		defer os.Remove("systemconfig.json")
		// Default config file was created, redo the func call to load the file
		rawResources, err = usecases.Configure(&sys)
		if err != nil {
			cancel()
			return nil, nil, err
		}
	}
	// NOTE: if the config file already existed (thus the above error block didn't
	// get to run), then the config file should be left alone and not removed!

	// Load unit assets defined in the config file
	cleanups, err := LoadResources(&sys, rawResources, loadUA)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// TODO: this is not ready for production yet?
	// usecases.RequestCertificate(&sys)

	// TODO: prints logs?
	usecases.RegisterServices(&sys)

	// TODO: prints logs??
	usecases.SetoutServers(&sys)

	stop := func() {
		cancel()
		// TODO: a waitgroup or something should be used to make sure all goroutines have stopped
		// Not doing much in the mock cleanups so this works fine for now...?
		cleanups()
	}
	return &sys, stop, nil
}

////////////////////////////////////////////////////////////////////////////////
// PROPOSAL: new additions to usecases/configuration.go?

// TODO: this function really needs an error return too
type NewResourceFunc func(usecases.ConfigurableAsset, *components.System) (components.UnitAsset, func())

func LoadResources(sys *components.System, rawRes []json.RawMessage, newRes NewResourceFunc) (func(), error) {
	// Resets this map so it can be filled with loaded unit assets (rather than templates)
	sys.UAssets = make(map[string]*components.UnitAsset)

	var cleanups []func()
	for _, raw := range rawRes {
		var ca usecases.ConfigurableAsset
		if err := json.Unmarshal(raw, &ca); err != nil {
			return func() {}, err
		}

		ua, f := newRes(ca, sys)
		sys.UAssets[ua.GetName()] = &ua
		cleanups = append(cleanups, f)
	}

	doCleanups := func() {
		for _, f := range cleanups {
			f()
		}
		// Stops hijacking SIGINT and return signal control to user
		signal.Stop(sys.Sigs)
		close(sys.Sigs)
	}
	return doCleanups, nil
}

////////////////////////////////////////////////////////////////////////////////
// Mock simulating traffic between a system and registrars/orchestrators

type event struct {
	key  string
	hits int
	body []byte
}

type mockTrans struct {
	t      *testing.T
	hits   map[string]int // Used to track http requests
	mutex  sync.Mutex     // For protecting access to the above map
	events chan event
}

func newMockTransport(t *testing.T) *mockTrans {
	m := &mockTrans{
		t:      t,
		hits:   make(map[string]int),
		events: make(chan event),
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = m
	return m
}

func (m *mockTrans) waitFor(event string) (int, []byte, error) {
	select {
	case e := <-m.events:
		if e.key != event {
			return 0, nil, fmt.Errorf("got %s, expected %s", e.key, event)
		}
		return e.hits, e.body, nil
	case <-time.Tick(10 * time.Second):
		return 0, nil, fmt.Errorf("event timeout")
	}
}

func newServiceRecord() []byte {
	f := forms.ServiceRecord_v1{
		Id:            13, // NOTE: this should match with eventUnregister
		Created:       time.Now().Format(time.RFC3339),
		EndOfValidity: time.Now().Format(time.RFC3339),
		Version:       "ServiceRecord_v1",
	}
	b, err := usecases.Pack(&f, "application/json")
	if err != nil {
		panic(err) // Hard fail if Pack() can't handle the above form
	}
	return b
}

func newServicePoint() []byte {
	f := forms.ServicePoint_v1{
		// per usecases/registration.go:serviceRegistrationForm()
		ServNode: fmt.Sprintf("localhost_%s_%s_%s", systemName, unitName, unitService),
		// per orchestrator/thing.go:selectService()
		ServLocation: fmt.Sprintf("http://localhost:%d/%s/%s/%s",
			systemPort, systemName, unitName, unitService,
		),
		Version: "ServicePoint_v1",
	}
	b, err := usecases.Pack(&f, "application/json")
	if err != nil {
		panic(err) // Another hard fail if Pack() can't work with the above form
	}
	return b
}

const (
	eventRegistryStatus string = "GET /serviceregistrar/registry/status"
	eventRegister       string = "POST /serviceregistrar/registry/register"
	eventUnregister     string = "DELETE /serviceregistrar/registry/unregister/13"
	eventOrchestration  string = "GET /orchestrator/orchestration"
	eventOrchestrate    string = "POST /orchestrator/orchestration/squest"
)

var mockRequests = map[string]struct {
	sendEvent bool
	status    int
	body      []byte
}{
	eventRegistryStatus: {false, 200, []byte(components.ServiceRegistrarLeader)},
	eventRegister:       {true, 200, newServiceRecord()},
	eventUnregister:     {true, 200, nil},
	eventOrchestration:  {false, 200, nil},
	eventOrchestrate:    {true, 200, newServicePoint()},
}

func (m *mockTrans) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mutex.Lock() // This lock is mainly for guarding concurrent access to the hits map
	defer m.mutex.Unlock()
	key := req.Method + " " + req.URL.Path
	m.hits[key] += 1
	if key == serviceURL {
		// The example service will, through the system, return a proper response
		return http.DefaultTransport.RoundTrip(req)
	}

	// Any other requests needs to be mocked, simulating responses from the
	// service registrar and orchestrator.
	mock, found := mockRequests[key]
	if !found {
		m.t.Errorf("unknown request: %s", key)
		// Let's see how the system responds to this
		mock.status = http.StatusNotImplemented
		mock.body = []byte(http.StatusText(mock.status))
	}
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(mock.status)
	rec.Write(mock.body) // Safe to ignore the returned error, it's always nil

	// Allows for syncing up the test, with the request flow performed by the system
	if mock.sendEvent {
		var b []byte
		if req.Body != nil {
			var err error
			b, err = io.ReadAll(req.Body)
			if err != nil {
				m.t.Errorf("failed reading request body: %v", err)
			}
			defer req.Body.Close()
		}
		// Using a goroutine prevents thread locking
		go func(k string, h int, b []byte) {
			m.events <- event{k, h, b}
		}(key, m.hits[key], b)
	}
	return rec.Result(), nil
}

////////////////////////////////////////////////////////////////////////////////

func countGoroutines() (int, string) {
	c := runtime.NumGoroutine()
	buf := &bytes.Buffer{}
	// A write to this buffer will always return nil error, so safe to ignore here.
	// This call will spawn some goroutine too, so need to chill for a little while.
	_ = pprof.Lookup("goroutine").WriteTo(buf, 2)
	trace := buf.String()
	// Calling signal.Notify() will leave an extra goroutine that runs forever,
	// so it should be subtracted from the count. For more info, see:
	// https://github.com/golang/go/issues/52619
	// https://github.com/golang/go/issues/72803
	// https://github.com/golang/go/issues/21576
	if strings.Contains(trace, "os/signal.signal_recv") {
		c -= 1
	}
	return c, trace
}

func assertNotEq(t *testing.T, got, want any) {
	if got != want {
		t.Errorf("got %v, expected %v", got, want)
	}
}

func TestSimpleSystemIntegration(t *testing.T) {
	routinesStart, _ := countGoroutines()
	m := newMockTransport(t)
	sys, stopSystem, err := newSystem()
	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}

	// Validate service registration
	hits, body, err := m.waitFor(eventRegister)
	assertNotEq(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventRegister)
	}
	var sr forms.ServiceRecord_v1
	err = json.Unmarshal(body, &sr)
	assertNotEq(t, err, nil)
	assertNotEq(t, sr.SystemName, systemName)
	assertNotEq(t, sr.SubPath, path.Join(unitName, unitService))

	// Validate service usage
	ua := *sys.UAssets[unitName]
	if ua == nil {
		t.Fatalf("system missing unit asset: %s", unitName)
	}
	service := ua.GetCervices()[unitService]
	if service == nil {
		t.Fatalf("unit asset missing cervice: %s", unitService)
	}
	f, err := usecases.GetState(service, sys)
	assertNotEq(t, err, nil)
	fs, ok := f.(*forms.SignalA_v1a)
	if ok == false || fs == nil || fs.Value == 0.0 {
		t.Errorf("invalid form: %#v", f)
	}

	// Late validation for service discovery
	hits, body, err = m.waitFor(eventOrchestrate)
	assertNotEq(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventUnregister)
	}
	var sq forms.ServiceQuest_v1
	err = json.Unmarshal(body, &sq)
	assertNotEq(t, err, nil)
	assertNotEq(t, sq.ServiceDefinition, unitService)

	// Validate service unregister
	stopSystem()
	hits, _, err = m.waitFor(eventUnregister) // NOTE: doesn't receive a body
	assertNotEq(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventUnregister)
	}

	// Detect any leaking goroutines
	// Delay a short moment and let the goroutines finish. Not sure if there's
	// a better way to wait for an _unknown number_ of goroutines.
	time.Sleep(1 * time.Second)
	routinesStop, trace := countGoroutines()
	if (routinesStop - routinesStart) != 0 {
		t.Errorf("leaking goroutines: count at start=%d, stop=%d\n%s",
			routinesStart, routinesStop, trace,
		)
	}
}
