package tests

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"path"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

const (
	unitName    string = "randomiser"
	unitService string = "random"
)

// The most simplest unit asset
type uaRandomiser struct {
	Name        string              `json:"-"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"-"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
}

// Force type check (fulfilling the interface) at compile time
var _ components.UnitAsset = &uaRandomiser{}

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

func createUATemplate(sys *components.System) {
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

func loadUAConfig(ca usecases.ConfigurableAsset, sys *components.System) (components.UnitAsset, func()) {
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

const (
	systemName string = "test"
	systemPort int    = 29999
)

var serviceURL = "GET /" + path.Join(systemName, unitName, unitService)

// The most simplest system
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
	createUATemplate(&sys)
	rawResources, err := usecases.Configure(&sys)

	// Extra check to work around "created config" error. Not required normally!
	if err != nil {
		// Return errors not related to config creation
		if errors.Is(err, usecases.ErrNewConfig) == false {
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
	cleanups, err := LoadResources(&sys, rawResources, loadUAConfig)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// TODO: this is not ready for production yet?
	// usecases.RequestCertificate(&sys)

	usecases.RegisterServices(&sys)

	// TODO: prints logs
	usecases.SetoutServers(&sys)

	stop := func() {
		cancel()
		// TODO: a waitgroup or something should be used to make sure all goroutines have stopped
		// Not doing much in the mock cleanups so this works fine for now...?
		cleanups()
	}
	return &sys, stop, nil
}
