package components

import (
	"strings"
	"testing"
)

func TestValidMission(t *testing.T) {
	for _, m := range Missions {
		if !ValidMission(m) {
			t.Errorf("ValidMission(%q) = false; every documented mission must be accepted", m)
		}
	}

	for _, m := range []string{"", "Measurement", "measure_temperature", "sensor", "measurement "} {
		if ValidMission(m) {
			t.Errorf("ValidMission(%q) = true; want false", m)
		}
	}
}

func TestValidateMissionRejectsAbsentAndUnknown(t *testing.T) {
	tests := []struct {
		name        string
		mission     string
		wantErr     bool
		wantInError string
	}{
		{"a documented mission is accepted", MissionActuation, false, ""},
		{"an absent mission is refused", "", true, "declares no mission"},
		{"pre-taxonomy free text is refused", "control_heater", true, "unknown mission"},
		{"case matters", "Actuation", true, "unknown mission"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMission("Servo_1", tc.mission)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ValidateMission(%q) error = %v; wantErr %v", tc.mission, err, tc.wantErr)
			}
			if err == nil {
				return
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("error %q does not describe the problem (%q)", err, tc.wantInError)
			}
			// The operator must be able to fix the configuration from the message
			// alone, without going to the specification.
			if !strings.Contains(err.Error(), "Servo_1") {
				t.Errorf("error %q does not name the offending asset", err)
			}
			for _, m := range Missions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error %q omits the permitted value %q", err, m)
				}
			}
		})
	}
}
