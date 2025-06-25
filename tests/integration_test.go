package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/usecases"
)

// Force type check (fulfilling the interface) at compile time
var _ components.UnitAsset = &uaGreeter{}

// The most simplistic UnitAsset possible
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
		Details:    map[string][]string{"key": []string{"value"}},
		RegPeriod:  60,
		// NOTE: must start with lower-case, it gets embedded into another sentence in the web API
		Description: "greets you with a message",
	}
	var ua components.UnitAsset
	ua = &uaGreeter{
		Name:    "greeter", // WARN: don't use the system name!! this is an asset!
		Details: map[string][]string{"key": {"value"}},
		ServicesMap: components.Services{
			greetService.SubPath: greetService,
		},
	}
	sys.UAssets[ua.GetName()] = &ua
}

func loadGreeter(ca usecases.ConfigurableAsset, sys *components.System) (components.UnitAsset, func()) {
	ua := &uaGreeter{
		Name:        ca.Name,
		Owner:       sys,
		Details:     ca.Details,
		ServicesMap: usecases.MakeServiceMap(ca.Services),
	}
	return ua, func() {}
}

// Creates the most simplest system possible
func newSystem() (*components.System, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	// TODO: want this to return a pointer type instead! easier to use and pointer is used all the time anyway down below
	sys := components.NewSystem("test", ctx)
	sys.Husk = &components.Husk{
		Description: " is the most simplest system possible, used for performing integration tests",
		Details:     map[string][]string{"key": {"value"}},
		ProtoPort:   map[string]int{"http": 29999},
	}

	addGreeterTemplate(&sys)
	rawResources, err := usecases.Configure(&sys)
	if err != nil {
		// TODO: check for ErrCreatedConfig blah, if so continue
		cancel()
		return nil, nil, err
	}
	// Don't leave any leftovers, no matter what
	defer os.Remove("systemconfig.json")

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
	go usecases.SetoutServers(&sys)

	stop := func() {
		cancel()
		// TODO: this should be replaced with a wait group instead
		time.Sleep(2 * time.Second)
		cleanups()
	}
	return &sys, stop, nil
}

////////////////////////////////////////////////////////////////////////////////

// TODO: move to usecases/configure
// TODO: this function really needs an error return too
type NewResourceFunc func(usecases.ConfigurableAsset, *components.System) (components.UnitAsset, func())

// TODO: move to usecases/configure
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
	}
	return doCleanups, nil
}

////////////////////////////////////////////////////////////////////////////////

func TestSimpleSystemIntegration(t *testing.T) {
	sys, stop, err := newSystem()
	if err != nil {
		panic(err) // TODO
	}
}
