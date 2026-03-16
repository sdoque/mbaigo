package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

type requestEvent struct {
	event string
	hits  int
	body  []byte
}

// Mock simulating traffic between a system and registrars/orchestrators
type mockTrans struct {
	t      *testing.T
	hits   map[string]int    // Used to track http requests
	mutex  sync.Mutex        // For protecting access to the above map
	events chan requestEvent // Tracks service "events" and requests to the cloud services
}

func newMockTransport(t *testing.T) *mockTrans {
	m := &mockTrans{
		t:      t,
		hits:   make(map[string]int),
		events: make(chan requestEvent),
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = m
	return m
}

func (m *mockTrans) waitFor(event string) (int, []byte, error) {
	select {
	case e := <-m.events:
		if e.event != event {
			return 0, nil, fmt.Errorf("got %s, expected %s", e.event, event)
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
	event := req.Method + " " + req.URL.Path
	m.hits[event] += 1
	if event == serviceURL {
		// The example service will, through the system, return a proper response
		return http.DefaultTransport.RoundTrip(req)
	}

	// Any other requests needs to be mocked, simulating responses from the
	// service registrar and orchestrator.
	mock, found := mockRequests[event]
	if !found {
		m.t.Errorf("unknown request: %s", event)
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
		go func(e string, h int, b []byte) {
			m.events <- requestEvent{e, h, b}
		}(event, m.hits[event], b)
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

func assertEqual(t *testing.T, got, want any) {
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
	assertEqual(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventRegister)
	}
	var sr forms.ServiceRecord_v1
	err = json.Unmarshal(body, &sr)
	assertEqual(t, err, nil)
	assertEqual(t, sr.SystemName, systemName)
	assertEqual(t, sr.SubPath, path.Join(unitName, unitService))

	// Validate service usage
	ua, ok := sys.UAssets[unitName]
	if !ok || ua == nil {
		t.Fatalf("system missing unit asset: %s", unitName)
	}
	service := ua.GetCervices()[unitService]
	if service == nil {
		t.Fatalf("unit asset missing cervice: %s", unitService)
	}
	f, err := usecases.GetState(service, sys)
	assertEqual(t, err, nil)
	fs, ok := f.(*forms.SignalA_v1a)
	if ok == false || fs == nil || fs.Value == 0.0 {
		t.Errorf("invalid form: %#v", f)
	}

	// Late validation for service discovery
	hits, body, err = m.waitFor(eventOrchestrate)
	assertEqual(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventUnregister)
	}
	var sq forms.ServiceQuest_v1
	err = json.Unmarshal(body, &sq)
	assertEqual(t, err, nil)
	assertEqual(t, sq.ServiceDefinition, unitService)

	// Validate service unregister
	stopSystem()
	hits, _, err = m.waitFor(eventUnregister) // NOTE: doesn't receive a body
	assertEqual(t, err, nil)
	if hits != 1 {
		t.Errorf("system skipped: %s", eventUnregister)
	}

	// Detect any leaking goroutines
	// Delay a short moment and let the goroutines finish. Not sure if there's
	// a better way to wait for an _unknown number_ of goroutines.
	// This might give flaky test results in slower environments!
	time.Sleep(1 * time.Second)
	routinesStop, trace := countGoroutines()
	if (routinesStop - routinesStart) != 0 {
		t.Errorf("leaking goroutines: count at start=%d, stop=%d\n%s",
			routinesStart, routinesStop, trace,
		)
	}
}
