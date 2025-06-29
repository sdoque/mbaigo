package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

////////////////////////////////////////////////////////////////////////////////
// The most simplest unit asset

// Force type check (fulfilling the interface) at compile time
var _ components.UnitAsset = &uaGreeter{}

type uaGreeter struct {
	Name        string              `json:"-"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"-"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	greeting    string
}

// Add required functions to fulfil the UnitAsset interface
func (ua uaGreeter) GetName() string                  { return ua.Name }
func (ua uaGreeter) GetServices() components.Services { return ua.ServicesMap }
func (ua uaGreeter) GetCervices() components.Cervices { return ua.CervicesMap }
func (ua uaGreeter) GetDetails() map[string][]string  { return ua.Details }
func (ua uaGreeter) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	if servicePath != "greet" {
		http.Error(w, "unknown service path: "+servicePath, http.StatusBadRequest)
		return
	}
	if _, err := fmt.Fprintln(w, ua.greeting); err != nil {
		http.Error(w, "error while writing greeting: "+err.Error(), http.StatusInternalServerError)
	}
}

func addGreeterTemplate(sys *components.System) {
	greetService := &components.Service{
		Definition: "greet", // The "name" of the service
		SubPath:    "greet", // Not "allowed" to be changed afterwards
		Details:    map[string][]string{"key1": {"value1"}},
		RegPeriod:  60,
		// NOTE: must start with lower-case, it gets embedded into another sentence in the web API
		Description: "greets you with a message",
	}
	ua := components.UnitAsset(&uaGreeter{
		Name:    "greeter", // WARN: don't use the system name!! this is an asset!
		Details: map[string][]string{"key2": {"value2"}},
		ServicesMap: components.Services{
			greetService.SubPath: greetService,
		},
	})
	sys.UAssets[ua.GetName()] = &ua
}

func loadGreeter(ca usecases.ConfigurableAsset, sys *components.System) (components.UnitAsset, func()) {
	service := ca.Services[0]
	ua := &uaGreeter{
		Name:        ca.Name,
		Owner:       sys,
		Details:     ca.Details,
		ServicesMap: usecases.MakeServiceMap(ca.Services),
		// Let it consume its own service
		CervicesMap: components.Cervices{ca.Name: &components.Cervice{
			Definition: service.Definition,
			Details:    service.Details,
			// TODO: need nodes map?? doesn't look like it so far
		}},
	}
	return ua, func() {}
}

////////////////////////////////////////////////////////////////////////////////
// The most simplest system

func newSystem() (*components.System, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	// TODO: want this to return a pointer type instead! easier to use and pointer is used all the time anyway down below
	sys := components.NewSystem("test", ctx)
	sys.Husk = &components.Husk{
		Description: " is the most simplest system possible, used for performing integration tests",
		Details:     map[string][]string{"key3": {"value3"}},
		ProtoPort:   map[string]int{"http": 29999},
	}

	addGreeterTemplate(&sys)
	rawResources, err := usecases.Configure(&sys)
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

	// TODO: this could had been done already in Configure()?
	// But that would need a change in the function signature
	cleanups, err := LoadResources(&sys, rawResources, loadGreeter)
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

type mockTrans struct {
	t      *testing.T
	hits   map[string]int // Used to track http requests
	events chan string    // Allows waiting for requests
}

func newMockTransport(t *testing.T) *mockTrans {
	m := &mockTrans{
		t:      t,
		hits:   make(map[string]int),
		events: make(chan string),
	}
	// Hijack the default http client so no actual http requests are sent over the network
	http.DefaultClient.Transport = m
	return m
}

func (m *mockTrans) waitFor(event string) error {
	select {
	case e := <-m.events:
		if e != event {
			return fmt.Errorf("got %s, expected %s", e, event)
		}
		return nil
	case <-time.Tick(10 * time.Second):
		return fmt.Errorf("event timeout")
	}
}

func newServiceRecord() (b []byte, err error) {
	f := forms.ServiceRecord_v1{
		Id:            13,
		Created:       time.Now().Format(time.RFC3339),
		EndOfValidity: time.Now().Format(time.RFC3339),
		Version:       "ServiceRecord_v1",
	}
	return usecases.Pack(&f, "application/json")
}

const eventRegistryStatus string = "GET /serviceregistrar/registry/status"
const eventRegister string = "POST /serviceregistrar/registry/register"
const eventUnregister string = "DELETE /serviceregistrar/registry/unregister/13"

func (m *mockTrans) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	status, body := 200, ""
	key := req.Method + " " + req.URL.Path
	m.hits[key] += 1
	switch key {

	// Find leading registrar
	case eventRegistryStatus:
		status, body = 200, components.ServiceRegistrarLeader

	// Register services with registrar
	case eventRegister:
		// TODO: validate body
		// b, err := io.ReadAll(req.Body)
		// if err != nil {
		// 	return nil, err
		// }
		// defer req.Body.Close()
		// fmt.Println(string(b))
		f, err := newServiceRecord()
		if err != nil {
			m.t.Fatalf("newServiceRecord: %s", err)
		}
		m.events <- key
		status, body = 200, string(f)

	// Unregister services
	case eventUnregister:
		// TODO: validate the id matches with id in the form sent above
		m.events <- key

	// TODO handle orchestrator requests

	default:
		m.t.Errorf("unknown request: %s", key)
	}

	resp = &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Header: map[string][]string{
			"Content-Type": {"application/json"},
		},
	}
	return resp, nil
}

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

func TestSimpleSystemIntegration(t *testing.T) {
	routinesStart, _ := countGoroutines()
	m := newMockTransport(t)
	sys, stop, err := newSystem()
	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
	// TODO: try grabbing the ua in some cleaner way
	var ua components.UnitAsset
	for _, v := range sys.UAssets {
		ua = *v
		break
	}
	if ua == nil {
		t.Fatal("missing unit asset in service")
	}

	// Validate service registration
	if err = m.waitFor(eventRegister); err != nil {
		t.Fatal(err)
	}
	// This status check occurs so many times so can't assume we only hit it once
	if m.hits[eventRegistryStatus] < 1 {
		t.Errorf("system skipped: %s", eventRegistryStatus)
	}
	if m.hits[eventRegister] != 1 {
		t.Errorf("system skipped: %s", eventRegister)
	}

	// Validate service use
	service := ua.GetCervices()[ua.GetName()]
	if service == nil {
		t.Fatalf("unit asset missing cervice: %s", ua.GetName())
	}
	f, err := usecases.GetState(service, sys)
	if err != nil {
		t.Errorf("%s", err)
	}
	// TODO: validate return form
	fmt.Println(f)

	// Validate service unregister
	stop()
	m.waitFor(eventUnregister)
	if m.hits[eventUnregister] != 1 {
		t.Errorf("system skipped: %s", eventUnregister)
	}

	// Detect any leaking goroutines
	// Delay a short moment and let the goroutines finish. Not sure if there's
	// a better way to wait for an unknown number of goroutines.
	time.Sleep(1 * time.Second)
	routinesStop, trace := countGoroutines()
	if (routinesStop - routinesStart) != 0 {
		t.Errorf("leaking goroutines, count at start=%d, stop=%d\n%s",
			routinesStart, routinesStop, trace,
		)
	}
}
