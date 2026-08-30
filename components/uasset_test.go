package components

import (
	"encoding/json"
	"strings"
	"testing"
)

// A mission outside the taxonomy can no longer be written in Go at all — the
// type has an unexported field, so `Mission{"whatever"}` does not compile
// outside this package. What remains testable at run time is the boundary:
// missions that arrive as text, from a configuration file or a registration
// record.
func TestAMissionOutsideTheTaxonomyIsRefusedAtTheBoundary(t *testing.T) {
	for _, m := range Missions {
		got, err := MissionFromString(m.String())
		if err != nil {
			t.Errorf("MissionFromString(%q): %v; every documented mission must be accepted", m, err)
		}
		if got != m {
			t.Errorf("MissionFromString(%q) = %q", m, got)
		}
	}

	// The near misses, which are what an operator actually types.
	for _, name := range []string{"Measurement", "measure_temperature", "sensor",
		"measurement ", "web_dashboard", "provide_weather_data", "expose_zigbee_devices"} {
		if _, err := MissionFromString(name); err == nil {
			t.Errorf("MissionFromString(%q) was accepted", name)
		}
	}
}

// The three phrases above are not hypothetical: they were written as missions in
// four systems, and read like missions without being any of them. This is the
// message an operator gets now, at the point the file is read.
func TestAnUnknownMissionSaysWhatWasExpected(t *testing.T) {
	_, err := MissionFromString("web_dashboard")
	if err == nil {
		t.Fatal("an invented mission was accepted")
	}
	if !strings.Contains(err.Error(), "web_dashboard") {
		t.Errorf("error %q does not quote what was written", err)
	}
	for _, m := range Missions {
		if !strings.Contains(err.Error(), m.String()) {
			t.Errorf("error %q omits the permitted value %q", err, m)
		}
	}
}

// JSON is how a mission reaches a system: a configuration file it wrote itself,
// or a record another system registered. It goes out as the same plain string it
// always was, so nothing reading the wire can tell the type changed.
func TestAMissionRoundTripsAsPlainText(t *testing.T) {
	type asset struct {
		Mission Mission `json:"mission,omitempty"`
	}

	encoded, err := json.Marshal(asset{Mission: MissionControl})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if string(encoded) != `{"mission":"control"}` {
		t.Errorf("a mission is written as %s; the wire form must not change", encoded)
	}

	var decoded asset
	if err := json.Unmarshal([]byte(`{"mission":"actuation"}`), &decoded); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if decoded.Mission != MissionActuation {
		t.Errorf("read back %q, want %q", decoded.Mission, MissionActuation)
	}

	// A configuration naming something that is not a mission is refused as the
	// file is read, rather than carried inward.
	if err := json.Unmarshal([]byte(`{"mission":"web_dashboard"}`), &decoded); err == nil {
		t.Error("a configuration declaring an invented mission was accepted")
	}

	// An absent one is a different mistake, reported by ValidateMission with the
	// asset's name to hand.
	var absent asset
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil {
		t.Fatalf("an asset with no mission could not be read at all: %v", err)
	}
	if !absent.Mission.IsZero() {
		t.Error("an absent mission did not come back as none declared")
	}
}

func TestValidateMissionRejectsAnAssetThatDeclaresNone(t *testing.T) {
	if err := ValidateMission("Servo_1", MissionActuation); err != nil {
		t.Errorf("a documented mission was refused: %v", err)
	}

	err := ValidateMission("Servo_1", Mission{})
	if err == nil {
		t.Fatal("an asset declaring no mission was accepted")
	}
	if !strings.Contains(err.Error(), "declares no mission") {
		t.Errorf("error %q does not describe the problem", err)
	}
	// The operator must be able to fix the configuration from the message alone,
	// without going to the specification.
	if !strings.Contains(err.Error(), "Servo_1") {
		t.Errorf("error %q does not name the offending asset", err)
	}
	for _, m := range Missions {
		if !strings.Contains(err.Error(), m.String()) {
			t.Errorf("error %q omits the permitted value %q", err, m)
		}
	}
}

// Mobility is what a load balancer needs and the graph cannot derive. The graph
// says which host each system runs on, so what is where is known; what is
// missing is whether any of it could be somewhere else.
func TestMobilityVocabulary(t *testing.T) {
	if len(Mobilities) != 3 {
		t.Fatalf("%d mobilities; the middle case is the common one and must not be collapsed away", len(Mobilities))
	}
	seen := map[string]bool{}
	for _, m := range Mobilities {
		if m.IsZero() {
			t.Error("an empty mobility would read as a declaration rather than as silence")
		}
		if seen[m.String()] {
			t.Errorf("%q appears twice", m)
		}
		seen[m.String()] = true
	}
	for _, want := range Mobilities {
		if !seen[want.String()] {
			t.Errorf("%q is not in Mobilities, so an error cannot name it as permitted", want)
		}
	}
}

// The vocabulary is enforced where the text arrives, as it is for a mission, so
// a value outside it cannot be held at all rather than merely being discouraged.
func TestMobilityRefusedOutsideTheVocabulary(t *testing.T) {
	var m Mobility
	if err := json.Unmarshal([]byte(`"portable"`), &m); err == nil {
		t.Fatal("an unknown mobility was accepted; a configuration typo would reach a load balancer")
	} else if !strings.Contains(err.Error(), "fixed, tethered, movable") {
		t.Errorf("the error does not list what is permitted: %v", err)
	}

	if err := json.Unmarshal([]byte(`"movable"`), &m); err != nil || m != MobilityMovable {
		t.Errorf("a good mobility was refused: %v %v", m, err)
	}

	// Silence is not an error: an asset that says nothing is one nothing may
	// propose to move, which is the conservative reading.
	if err := json.Unmarshal([]byte(`""`), &m); err != nil || !m.IsZero() {
		t.Errorf("an absent mobility should be silence, got %v %v", m, err)
	}
}

// A field is only worth having if it survives the round trip an operator's file
// makes: written by the template, read back on the next start.
func TestMobilityRoundTripsThroughJSON(t *testing.T) {
	out, err := json.Marshal(UnitAsset{Name: "sensor", Mobility: MobilityFixed})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"mobility":"fixed"`) {
		t.Fatalf("mobility is not written as the plain word: %s", out)
	}
	var back UnitAsset
	if err := json.Unmarshal(out, &back); err != nil || back.Mobility != MobilityFixed {
		t.Errorf("mobility did not survive the round trip: %v %v", back.Mobility, err)
	}
}

// Tethered means "movable to any host that can still reach what I talk to", and
// a balancer cannot check reachability against a dependency nobody named.
func TestValidateMobilityTether(t *testing.T) {
	if err := ValidateMobility("gateway", MobilityTethered, nil); err == nil {
		t.Error("a tethered asset naming no tether was accepted")
	}
	if err := ValidateMobility("gateway", MobilityTethered, []string{"192.168.1.50:502"}); err != nil {
		t.Errorf("a properly tethered asset was refused: %v", err)
	}
	// The other direction is a mistake too, and a quieter one: a tether nothing
	// reads looks like a constraint being honoured.
	if err := ValidateMobility("sensor", MobilityFixed, []string{"somewhere"}); err == nil {
		t.Error("tetheredTo was accepted on a fixed asset, where nothing will ever read it")
	}
	if err := ValidateMobility("sensor", MobilityFixed, nil); err != nil {
		t.Errorf("a plain fixed asset was refused: %v", err)
	}
	// Silence stays legal.
	if err := ValidateMobility("anything", Mobility{}, nil); err != nil {
		t.Errorf("an undeclared mobility was refused: %v", err)
	}
}
