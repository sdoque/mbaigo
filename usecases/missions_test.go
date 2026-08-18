package usecases

import (
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// assetWith builds a unit asset carrying the given asset-level mission and one
// service per entry in serviceMissions (keyed by definition, empty value meaning
// the service declares none of its own).
func assetWith(name string, assetMission components.Mission, serviceMissions map[string]components.Mission) *components.UnitAsset {
	services := components.Services{}
	for definition, mission := range serviceMissions {
		services[definition] = &components.Service{
			Definition: definition,
			SubPath:    definition,
			Mission:    mission,
		}
	}
	return &components.UnitAsset{
		Name:        name,
		Mission:     assetMission,
		ServicesMap: services,
	}
}

func systemWith(assets ...*components.UnitAsset) *components.System {
	sys := &components.System{Name: "testsystem"}
	sys.UAssets = make(map[string]*components.UnitAsset)
	for _, ua := range assets {
		sys.UAssets[ua.Name] = ua
	}
	return sys
}

func TestValidateMissionsAcceptsAssetLevelMission(t *testing.T) {
	sys := systemWith(assetWith("sensor_Id", components.MissionMeasurement,
		map[string]components.Mission{"temperature": {}}))

	if err := ValidateMissions(sys); err != nil {
		t.Errorf("ValidateMissions = %v; a service inherits its asset's mission", err)
	}
}

// A PLC or broker front end is too coarse to authorize against: the mission
// belongs to what is behind each service.
func TestValidateMissionsAcceptsServiceLevelOverride(t *testing.T) {
	plc := assetWith("PLC with Modbus slave", components.MissionMeasurement, map[string]components.Mission{
		"Slider1_Front_PB":      {},                          // ro register, inherits measurement
		"Slider1_Motor_Forward": components.MissionActuation, // rw register, overrides
	})

	if err := ValidateMissions(systemWith(plc)); err != nil {
		t.Errorf("ValidateMissions = %v; a service may override its asset's mission", err)
	}

	if got := components.EffectiveMission(plc, plc.ServicesMap["Slider1_Motor_Forward"]); got != components.MissionActuation {
		t.Errorf("EffectiveMission = %q; want %q", got, components.MissionActuation)
	}
	if got := components.EffectiveMission(plc, plc.ServicesMap["Slider1_Front_PB"]); got != components.MissionMeasurement {
		t.Errorf("EffectiveMission = %q; want %q", got, components.MissionMeasurement)
	}
}

func TestValidateMissionsRejectsUndeclaredAndUnknown(t *testing.T) {
	tests := []struct {
		name         string
		asset        *components.UnitAsset
		wantInError  string
		wantsService string
	}{
		{
			name:         "neither asset nor service declares one",
			asset:        assetWith("Bathroom/temperature", components.Mission{}, map[string]components.Mission{"temperature": {}}),
			wantInError:  "declares no mission",
			wantsService: "temperature",
		},
		// The two cases that used to sit here — free text at asset level and an
		// unknown mission at service level — can no longer be written in Go.
		// components.Mission has an unexported field, so a value outside the
		// taxonomy cannot be constructed outside that package, and text is only
		// turned into one by MissionFromString, which refuses it. Those two are
		// tested there, at the boundary the text arrives by.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMissions(systemWith(tc.asset))
			if err == nil {
				t.Fatal("ValidateMissions = nil; want an error")
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("error %q does not describe the problem (%q)", err, tc.wantInError)
			}
			// The operator has to find the offending service, not just the system.
			if !strings.Contains(err.Error(), tc.wantsService) {
				t.Errorf("error %q does not name the offending service %q", err, tc.wantsService)
			}
			if !strings.Contains(err.Error(), tc.asset.Name) {
				t.Errorf("error %q does not name the offending asset", err)
			}
		})
	}
}
