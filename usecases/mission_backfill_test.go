package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// TestAConfigurationWrittenBeforeMissionsStillStarts is the upgrade path.
//
// The framework refuses to start a system whose assets declare no mission, and
// a systemconfig.json is written only when it does not exist — never merged
// into. So a system that gained a mission after its file was written could not
// start, and no edit to the release could fix it: the file on the machine is
// the one that has to change. That is how flattener stopped, reporting that
// "ComfortController" declared no mission, on a machine where nothing about
// that asset had changed.
//
// Filled from the system author's own template, which is what the file would
// have been seeded with today. Nothing is guessed.
func TestAConfigurationWrittenBeforeMissionsStillStarts(t *testing.T) {
	sys := components.NewSystem("flattener", context.Background())
	sys.UAssets["ComfortController"] = &components.UnitAsset{
		Name:    "ComfortController",
		Mission: components.MissionControl,
	}

	// As the file has it: a name, traits, and no mission at all.
	before := json.RawMessage(`{"name":"ComfortController","traits":[{"region":"SE2"}]}`)
	after := fillMissionsFromTemplates(&sys, []json.RawMessage{before})

	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatalf("the patched asset is not readable: %v", err)
	}
	if got.Mission != components.MissionControl {
		t.Errorf("the asset declares %q, so the system still refuses to start", got.Mission)
	}
	if len(got.Traits) != 1 {
		t.Errorf("the asset kept %d traits, so filling the mission dropped the "+
			"operator's configuration", len(got.Traits))
	}
}

// TestARenamedAssetStillGetsItsMission is the deployment the backfill was
// written for and originally missed.
//
// Renaming the asset is the documented commissioning step for whole families of
// systems — a ds18b20 asset is named after its 1-wire identifier, a telegrapher
// asset after its MQTT topic — and the templates are keyed by the template's
// name. So matching on name alone skipped precisely the configurations that
// predate missions and are renamed, which is most of the ones in the field, and
// they fail on upgrade with a fatal rather than a warning.
func TestARenamedAssetStillGetsItsMission(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	sys.UAssets["sensor_Id"] = &components.UnitAsset{
		Name:    "sensor_Id",
		Mission: components.MissionMeasurement,
	}

	// As the commissioned file has it: the sensor's own identifier.
	after := fillMissionsFromTemplates(&sys,
		[]json.RawMessage{json.RawMessage(`{"name":"28-0516d0bfd5ff"}`)})

	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatalf("the patched asset is not readable: %v", err)
	}
	if got.Mission != components.MissionMeasurement {
		t.Errorf("the renamed asset declares %q, so the system refuses to start "+
			"until someone hand-edits the file", got.Mission)
	}
}

// A system with more than one template gets no guess. Which template a renamed
// asset came from is not knowable from the name, and a mission is what the
// authorizer classifies an asset by — so refusing to start is better than
// inventing one.
func TestARenamedAssetGetsNoGuessWhenTemplatesDiffer(t *testing.T) {
	sys := components.NewSystem("mixed", context.Background())
	sys.UAssets["reader"] = &components.UnitAsset{Name: "reader", Mission: components.MissionMeasurement}
	sys.UAssets["driver"] = &components.UnitAsset{Name: "driver", Mission: components.MissionActuation}

	after := fillMissionsFromTemplates(&sys,
		[]json.RawMessage{json.RawMessage(`{"name":"something_renamed"}`)})

	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatalf("the asset is not readable: %v", err)
	}
	if !got.Mission.IsZero() {
		t.Errorf("an asset was given the mission %q, which no template named it for", got.Mission)
	}
}

// The four deployments this has cost, as four tests.
//
// A systemconfig.json is written once, when there is none, and never merged into
// afterwards. Every field added to a service since a deployment was commissioned
// is therefore missing from that deployment's file, and every service added since
// is missing altogether — and the failure is always silent, because a missing
// field reads as a deliberate zero.
func TestAConfigurationWrittenBeforeAFieldExistedGetsIt(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	sys.UAssets["sensor_Id"] = &components.UnitAsset{
		Name:    "sensor_Id",
		Mission: components.MissionMeasurement,
		ServicesMap: components.Services{"temperature": {
			Definition:       "temperature",
			SubPath:          "temperature",
			SubscribeAble:    true,
			Heartbeat:        30,
			Threshold:        0.1,
			FastestHeartbeat: 2,
			FinestThreshold:  0.0625,
		}},
	}

	// The file as it was written before any of that existed.
	before := json.RawMessage(`{"name":"28-00000f030344","mission":"measurement","services":[
		{"definition":"temperature","subpath":"temperature","registrationPeriod":30}]}`)

	after := fillServicesFromTemplates(&sys, []json.RawMessage{before})

	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatalf("the patched asset is not readable: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(got.Services))
	}
	serv := got.Services[0]

	// A capability: whether the code publishes, which no file can decide.
	if !serv.SubscribeAble {
		t.Error("a service this build publishes registers as unfollowable, so every " +
			"consumer polls a provider that was willing to push")
	}
	// Terms: the file said nothing, so the template's stand.
	if serv.Heartbeat != 30 || serv.Threshold != 0.1 {
		t.Errorf("terms came out as %ds / %v, want the template's 30s / 0.1",
			serv.Heartbeat, serv.Threshold)
	}
	if serv.FastestHeartbeat != 2 || serv.FinestThreshold != 0.0625 {
		t.Error("the bounds a subscriber is clamped to were not carried over")
	}
	// And what the file did say is untouched.
	if serv.RegPeriod != 30 {
		t.Errorf("the configured registration period became %d", serv.RegPeriod)
	}
}

// What an operator tuned is theirs. A release supplies what is missing and
// overrules nothing that the file states.
func TestWhatTheFileSaysIsNotOverruled(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	sys.UAssets["sensor_Id"] = &components.UnitAsset{
		Name:    "sensor_Id",
		Mission: components.MissionMeasurement,
		ServicesMap: components.Services{"temperature": {
			Definition: "temperature", SubPath: "temperature",
			SubscribeAble: true, Heartbeat: 30, Threshold: 0.1,
		}},
	}

	tuned := json.RawMessage(`{"name":"sensor_Id","mission":"measurement","services":[
		{"definition":"temperature","subpath":"temperature","heartbeat":120,"threshold":2.5}]}`)

	after := fillServicesFromTemplates(&sys, []json.RawMessage{tuned})
	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.Services[0].Heartbeat != 120 || got.Services[0].Threshold != 2.5 {
		t.Errorf("an operator's terms were overwritten by the release: %ds / %v",
			got.Services[0].Heartbeat, got.Services[0].Threshold)
	}
	if !got.Services[0].SubscribeAble {
		t.Error("the capability was not supplied alongside the tuning that was kept")
	}
}

// A service the build serves and the file has never heard of. Left out, it is a
// path the system answers on that the cloud cannot discover, authorize or reason
// about — which is what happened to the registrar's system list.
func TestAServiceTheFileNeverHeardOfIsAdded(t *testing.T) {
	sys := components.NewSystem("serviceregistrar", context.Background())
	sys.UAssets["registry"] = &components.UnitAsset{
		Name:    "registry",
		Mission: components.MissionCore,
		ServicesMap: components.Services{
			"query":   {Definition: "query", SubPath: "query"},
			"syslist": {Definition: "syslist", SubPath: "syslist"},
		},
	}

	old := json.RawMessage(`{"name":"registry","mission":"core","services":[
		{"definition":"query","subpath":"query","registrationPeriod":30}]}`)

	after := fillServicesFromTemplates(&sys, []json.RawMessage{old})
	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, s := range got.Services {
		paths = append(paths, s.SubPath)
	}
	if len(got.Services) != 2 {
		t.Fatalf("services are %v; a service this build serves is missing from the "+
			"configuration and therefore from the registry", paths)
	}
}
