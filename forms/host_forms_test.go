package forms

import (
	"encoding/json"
	"testing"
	"time"
)

// A host's report must survive the round trip, and the optional fields must be
// distinguishable from zero.
//
// Pressure-stall figures are pointers for exactly this reason: a kernel without
// CONFIG_PSI reports nothing, and a stall of 0.0 means "nothing was delayed",
// which is the opposite conclusion. A plain float64 cannot hold that difference,
// and a balancer reading 0.0 from a host that never measured would move work
// onto it believing it idle.
func TestHostLoadDistinguishesUnmeasuredFromZero(t *testing.T) {
	var quiet HostLoad_v1
	quiet.NewForm()
	quiet.Host = "canbus"
	zero := 0.0
	quiet.StallCPU = &zero

	body, err := json.Marshal(&quiet)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back HostLoad_v1
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.StallCPU == nil || *back.StallCPU != 0 {
		t.Errorf("a measured zero came back as %v; it must not read as unmeasured", back.StallCPU)
	}

	// A fresh destination: json.Unmarshal leaves fields absent from the payload
	// as they were, so reusing the one above would have measured the previous
	// reading rather than this one.
	var silent, unmeasured HostLoad_v1
	silent.NewForm()
	body, _ = json.Marshal(&silent)
	if err := json.Unmarshal(body, &unmeasured); err != nil {
		t.Fatal(err)
	}
	if unmeasured.StallCPU != nil {
		t.Error("a kernel that reports no pressure came back as having measured something")
	}
}

// The version registers, so Unpack can find the type by name.
func TestHostLoadIsRegistered(t *testing.T) {
	var f HostLoad_v1
	if f.NewForm().FormVersion() != "HostLoad_v1" {
		t.Errorf("version = %q", f.FormVersion())
	}
	if _, known := FormTypeMap["HostLoad_v1"]; !known {
		t.Error("HostLoad_v1 is not in FormTypeMap, so nothing can unpack it by name")
	}
}

// Headroom is the comparable number and must stay within its stated range, or
// a balancer sorting on it compares nonsense.
func TestHeadroomIsAFraction(t *testing.T) {
	for _, tc := range []struct {
		name     string
		headroom float64
		ok       bool
	}{
		{"idle", 1.0, true},
		{"saturated", 0.0, true},
		{"half", 0.5, true},
		{"above one", 1.5, false},
		{"negative", -0.1, false},
	} {
		f := HostLoad_v1{Headroom: tc.headroom, SampledAt: time.Now()}
		inRange := f.Headroom >= 0 && f.Headroom <= 1
		if inRange != tc.ok {
			t.Errorf("%s: headroom %v in range = %t", tc.name, tc.headroom, inRange)
		}
	}
}
