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

// TestAnAssetTheTemplateDoesNotKnowIsLeftToBeRefused is the limit of the above.
//
// Filling a blank from a template the author wrote is not the same as defaulting
// the field, and the difference matters: a mission is what the authorizer and
// the knowledge graph classify an asset by, so one nobody declared is worth
// refusing. An asset with no template to speak for it keeps its blank and is
// refused at startup as before.
func TestAnAssetTheTemplateDoesNotKnowIsLeftToBeRefused(t *testing.T) {
	sys := components.NewSystem("flattener", context.Background())
	sys.UAssets["ComfortController"] = &components.UnitAsset{
		Name:    "ComfortController",
		Mission: components.MissionControl,
	}

	after := fillMissionsFromTemplates(&sys,
		[]json.RawMessage{json.RawMessage(`{"name":"SomethingElse"}`)})

	var got ConfigurableAsset
	if err := json.Unmarshal(after[0], &got); err != nil {
		t.Fatalf("the asset is not readable: %v", err)
	}
	if got.Mission != "" {
		t.Errorf("an asset no template describes was given the mission %q", got.Mission)
	}
	if err := components.ValidateMission(got.Name, got.Mission); err == nil {
		t.Error("an asset with no declared mission was accepted")
	}
}
