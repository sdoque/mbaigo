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
