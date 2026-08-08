package forms

import (
	"encoding/json"
	"testing"
	"time"
)

// Unpack resolves a payload by its version string, so a form the map does not
// know about cannot cross the wire at all.
func TestAuthorizationFormsAreRegistered(t *testing.T) {
	for _, version := range []string{"AuthorizationQuest_v1", "AuthorizationGrantList_v1"} {
		if _, ok := FormTypeMap[version]; !ok {
			t.Errorf("FormTypeMap has no entry for %q", version)
		}
	}
}

func TestAuthorizationFormVersions(t *testing.T) {
	var quest AuthorizationQuest_v1
	quest.NewForm()
	if quest.FormVersion() != "AuthorizationQuest_v1" {
		t.Errorf("quest version = %q", quest.FormVersion())
	}

	var grants AuthorizationGrantList_v1
	grants.NewForm()
	if grants.FormVersion() != "AuthorizationGrantList_v1" {
		t.Errorf("grant list version = %q", grants.FormVersion())
	}
}

// The candidate records must survive the round trip intact: the orchestrator
// builds its service point from what comes back, so a dropped field there is a
// provider the consumer cannot reach.
func TestAuthorizationRoundTrip(t *testing.T) {
	var rec ServiceRecord_v1
	rec.NewForm()
	rec.SystemName = "ds18b20"
	rec.SubPath = "sensor_Id/temperature"
	rec.ServiceDefinition = "temperature"
	rec.Mission = "measurement"
	rec.IPAddresses = []string{"192.168.1.4"}
	rec.ProtoPort = map[string]int{"https": 30150}
	rec.Details = map[string][]string{"FunctionalLocation": {"Kitchen"}}

	var sent AuthorizationGrantList_v1
	sent.NewForm()
	sent.Grants = []AuthorizationGrant_v1{{Record: rec, TTL: "5m", Reason: "policy 0 permits read"}}
	sent.Refusals = []AuthorizationRefusal_v1{{
		ProviderName: "telegrapher",
		ServiceNode:  "pi_telegrapher_Bathroom/temperature_temperature",
		Reason:       "locations do not match",
	}}

	raw, err := json.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AuthorizationGrantList_v1
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Grants) != 1 || len(got.Refusals) != 1 {
		t.Fatalf("got %d grants and %d refusals; want 1 and 1", len(got.Grants), len(got.Refusals))
	}
	back := got.Grants[0].Record
	if back.Mission != "measurement" {
		t.Errorf("mission did not survive: %q", back.Mission)
	}
	if back.ProtoPort["https"] != 30150 {
		t.Errorf("https port did not survive: %v", back.ProtoPort)
	}
	if loc := back.Details["FunctionalLocation"]; len(loc) != 1 || loc[0] != "Kitchen" {
		t.Errorf("details did not survive: %v", back.Details)
	}
}

// A grant nobody can date is one nobody should trust, so an unusable TTL reads
// as expired rather than as unlimited.
func TestGrantLifetime(t *testing.T) {
	tests := []struct {
		ttl  string
		want time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"90s", 90 * time.Second},
		{"", 0},
		{"soon", 0},
		{"-1m", 0},
	}

	for _, tc := range tests {
		if got := (AuthorizationGrant_v1{TTL: tc.ttl}).Lifetime(); got != tc.want {
			t.Errorf("Lifetime(%q) = %v; want %v", tc.ttl, got, tc.want)
		}
	}
}

// An empty grant list is a complete answer meaning "none of these", not a
// failure. The orchestrator has to be able to tell it apart from an error, so
// it must marshal and unmarshal cleanly.
func TestEmptyGrantListIsValid(t *testing.T) {
	var empty AuthorizationGrantList_v1
	empty.NewForm()

	raw, err := json.Marshal(&empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AuthorizationGrantList_v1
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Grants) != 0 {
		t.Errorf("got %d grants; want none", len(got.Grants))
	}
	if got.FormVersion() != "AuthorizationGrantList_v1" {
		t.Errorf("version did not survive: %q", got.FormVersion())
	}
}
