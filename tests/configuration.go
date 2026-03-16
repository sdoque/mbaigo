package tests

import (
	"encoding/json"
	"os/signal"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/usecases"
)

// PROPOSAL: new additions to usecases/configuration.go

// NewResourceFunc is the function type used for loading unit assets that were
// defined in "systemconfig.json".
// A new, custom instance of [Components.UnitAsset] should be created and populated
// with fields from the provided [usecases.ConfigurableAsset].
// Any services or consumed services should be added too.
// The function should then return the UnitAsset and an optional cleanup function.
//
// TODO: this function really needs an error return
// TODO: feels unnecessarily confusing to provide system instance.
type NewResourceFunc func(usecases.ConfigurableAsset, *components.System) (*components.UnitAsset, func())

// LoadResources loads all unit assets from rawRes (which was loaded from "systemconfig.json" file)
// and calls newResFunc repeatedly for each loaded asset.
// The fully loaded unit asset and an optional cleanup function are collected from
// newResFunc and are then attached to the sys system.
// LoadResources then returns a system cleanup function and an optional error.
// The error always originate from [json.Unmarshal].
func LoadResources(sys *components.System, rawRes []json.RawMessage, newResFunc NewResourceFunc) (func(), error) {
	// Resets this map so it can be filled with loaded unit assets (rather than templates)
	sys.UAssets = make(map[string]*components.UnitAsset)

	var cleanups []func()
	for _, raw := range rawRes {
		var ca usecases.ConfigurableAsset
		if err := json.Unmarshal(raw, &ca); err != nil {
			return func() {}, err
		}

		ua, f := newResFunc(ca, sys)
		sys.UAssets[ua.GetName()] = ua
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
