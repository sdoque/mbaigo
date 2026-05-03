/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 ***************************************************************************SDG*/

package usecases

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// Helpers newKGTestSystem and addTestAsset are defined in kgraphing_test.go
// and are reused here because both files live in the usecases package.

// ── portDefName ───────────────────────────────────────────────────────────────

func TestPortDefName(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty string", "", ""},
		{"lowercase start is capitalised", "temperature", "Temperature"},
		{"already capitalised gets Port suffix", "OnOff", "OnOffPort"},
		{"single lowercase letter", "a", "A"},
		{"single uppercase letter", "A", "APort"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := portDefName(tc.in); got != tc.want {
				t.Errorf("portDefName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── sysmlName ─────────────────────────────────────────────────────────────────

func TestSysmlName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a-b", "a_b"},
		{"a b", "a_b"},
		{"a.b", "a_b"},
		{"a-b c.d", "a_b_c_d"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := sysmlName(tc.in); got != tc.want {
				t.Errorf("sysmlName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── systemBaseType ────────────────────────────────────────────────────────────

func TestSystemBaseType(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"serviceregistrar", "ServiceRegistrar"},
		{"orchestrator", "Orchestrator"},
		{"ca", "CertificateAuthority"},
		{"thermostat", "ArrowheadSystem"},
		{"", "ArrowheadSystem"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := systemBaseType(tc.in); got != tc.want {
				t.Errorf("systemBaseType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── assetTypeName / behaviorTypeName ──────────────────────────────────────────

func TestAssetTypeName(t *testing.T) {
	sys := newKGTestSystem()
	// Asset name contains a hyphen to exercise sysmlName sanitisation.
	if got, want := assetTypeName(sys, "sensor-1"), "mysys_sensor_1UnitAsset"; got != want {
		t.Errorf("assetTypeName = %q, want %q", got, want)
	}
}

func TestBehaviorTypeName(t *testing.T) {
	sys := newKGTestSystem()
	if got, want := behaviorTypeName(sys, "sensor-1"), "mysys_sensor_1Behavior"; got != want {
		t.Errorf("behaviorTypeName = %q, want %q", got, want)
	}
}

// ── assetHasBehavior ──────────────────────────────────────────────────────────

func TestAssetHasBehavior(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)

	// Default cervice Mode is empty ⇒ no behaviour.
	if assetHasBehavior(ua) {
		t.Error("expected false for default (empty Mode)")
	}

	ua.CervicesMap["humidity"].Mode = "get"
	if !assetHasBehavior(ua) {
		t.Error("expected true for Mode=get")
	}

	ua.CervicesMap["humidity"].Mode = "set"
	if !assetHasBehavior(ua) {
		t.Error("expected true for Mode=set")
	}

	// Modes other than get/set do not trigger behaviour generation.
	ua.CervicesMap["humidity"].Mode = "other"
	if assetHasBehavior(ua) {
		t.Error("expected false for Mode=other")
	}
}

// ── sysmlPortDefs ─────────────────────────────────────────────────────────────

func TestSysmlPortDefs(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	out := sysmlPortDefs(sys)

	// Service "temperature" and cervice "humidity" are both lowercase, so
	// portDefName capitalises their first letter.
	if !strings.Contains(out, "port def 'Temperature'") {
		t.Error("missing Temperature port def")
	}
	if !strings.Contains(out, "port def 'Humidity'") {
		t.Error("missing Humidity port def")
	}
}

func TestSysmlPortDefsDeduplicates(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)
	// A second cervice sharing the service's definition exercises the `seen`
	// map so the same port def is not emitted twice.
	ua.CervicesMap["also_temperature"] = &components.Cervice{
		Definition: "temperature",
		Details:    map[string][]string{},
		Nodes:      make(map[string][]components.NodeInfo),
	}

	out := sysmlPortDefs(sys)
	if got := strings.Count(out, "port def 'Temperature'"); got != 1 {
		t.Errorf("Temperature port def appeared %d times, want 1", got)
	}
}

// ── sysmlBlockDefs ────────────────────────────────────────────────────────────

func TestSysmlBlockDefs(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	out := sysmlBlockDefs(sys)

	for _, want := range []string{
		"part def 'mysysSystem' :> ArrowheadSystem",
		`attribute redefines name : String = "mysys"`,
		"attribute redefines host : String",
		"attribute httpPort : Integer",
		"part 'sensor1' : 'mysys_sensor1UnitAsset'",
		"part def 'mysys_sensor1UnitAsset' :> UnitAsset",
		`attribute redefines mission : String = "measure_things"`,
		"out port 'temperature' : 'Temperature'",
		"in port 'humidity' : 'Humidity'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSysmlBlockDefsCoreSystem(t *testing.T) {
	// A core Arrowhead system specialises from its role-specific base type,
	// not from the generic ArrowheadSystem.
	sys := newKGTestSystem()
	sys.Name = "orchestrator"
	if out := sysmlBlockDefs(sys); !strings.Contains(out, ":> Orchestrator") {
		t.Errorf("expected :> Orchestrator for core system; got:\n%s", out)
	}
}

func TestSysmlBlockDefsSkipsZeroPort(t *testing.T) {
	sys := newKGTestSystem()
	sys.Husk.ProtoPort["coap"] = 0 // not configured
	if strings.Contains(sysmlBlockDefs(sys), "coapPort") {
		t.Error("zero port must not produce an attribute")
	}
}

func TestSysmlBlockDefsPerformAction(t *testing.T) {
	// An asset with a get/set cervice gains a `perform action` stanza that
	// references its behaviour action def.
	sys := newKGTestSystem()
	ua := addTestAsset(sys)
	ua.CervicesMap["humidity"].Mode = "get"

	out := sysmlBlockDefs(sys)
	if !strings.Contains(out, "perform action behave : 'mysys_sensor1Behavior'") {
		t.Errorf("expected perform action stanza; got:\n%s", out)
	}
}

// ── sysmlIBD ──────────────────────────────────────────────────────────────────

func TestSysmlIBD(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	out := sysmlIBD(sys)

	for _, want := range []string{
		"part 'mysys' : 'mysysSystem'",
		`attribute host : String = "testhost"`,
		`attribute ipAddress : String = "192.0.2.1"`,
		"attribute httpPort : Integer = 8080",
		"// provides sensor1.temperature at http://192.0.2.1:8080/mysys/sensor1/temp",
		"// @connect sensor1.humidity → (no registered provider)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSysmlIBDWithProviderNode(t *testing.T) {
	// When a cervice has registered provider nodes, each node URL produces
	// a concrete @connect annotation instead of the placeholder.
	sys := newKGTestSystem()
	ua := addTestAsset(sys)
	ua.CervicesMap["humidity"].Nodes["providerA"] = []components.NodeInfo{
		{URL: "http://192.0.2.2:8080/humidity/access"},
	}

	out := sysmlIBD(sys)
	want := "// @connect sensor1.humidity → http://192.0.2.2:8080/humidity/access"
	if !strings.Contains(out, want) {
		t.Errorf("expected %q; got:\n%s", want, out)
	}
}

func TestSysmlIBDSkipsZeroPort(t *testing.T) {
	sys := newKGTestSystem()
	sys.Husk.ProtoPort["http"] = 0
	if strings.Contains(sysmlIBD(sys), "httpPort") {
		t.Error("zero port must not appear as attribute")
	}
}

// ── sysmlBehaviorDefs ─────────────────────────────────────────────────────────

func TestSysmlBehaviorDefsEmpty(t *testing.T) {
	// No cervices tagged get/set ⇒ nothing emitted.
	sys := newKGTestSystem()
	addTestAsset(sys)

	if got := sysmlBehaviorDefs(sys); got != "" {
		t.Errorf("expected empty output, got:\n%s", got)
	}
}

func TestSysmlBehaviorDefsGetOnly(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)
	ua.CervicesMap["humidity"].Mode = "get"

	out := sysmlBehaviorDefs(sys)

	if !strings.Contains(out, "action def 'mysys_sensor1Behavior'") {
		t.Error("missing action def")
	}
	if !strings.Contains(out, "action 'get_humidity' : GetState") {
		t.Error("missing get action")
	}
	// A compute step is only inserted when both gets and sets are present.
	if strings.Contains(out, "compute : Compute") {
		t.Error("compute step must not appear when sets are absent")
	}
}

func TestSysmlBehaviorDefsGetSetCompute(t *testing.T) {
	// With both a get and a set cervice, the emitted flow is
	// get → compute → set, wired with `first ... then ...` steps.
	sys := newKGTestSystem()
	ua := addTestAsset(sys)
	ua.CervicesMap["humidity"].Mode = "get"
	ua.CervicesMap["setpoint"] = &components.Cervice{
		Definition: "setpoint",
		Mode:       "set",
		Details:    map[string][]string{},
		Nodes:      make(map[string][]components.NodeInfo),
	}

	out := sysmlBehaviorDefs(sys)

	for _, want := range []string{
		"action 'get_humidity' : GetState",
		"action compute : Compute",
		"action 'set_setpoint' : SetState",
		"first 'get_humidity' then compute",
		"first compute then 'set_setpoint'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// ── sysmlPackage ──────────────────────────────────────────────────────────────

func TestSysmlPackage(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	out := sysmlPackage(sys)

	if !strings.HasPrefix(out, "package 'testhost_mysys' {\n") {
		t.Errorf("wrong package preamble; got:\n%s", out)
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Error("must end with closing brace")
	}
	// All three major sections must be present.
	for _, want := range []string{
		"// ── Port Definitions",
		"// ── Block Definitions (BDD)",
		"// ── Internal Block Diagram (IBD)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing section %q", want)
		}
	}
}

// ── SModeling (HTTP handler) ──────────────────────────────────────────────────

func TestSModeling(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	req := httptest.NewRequest("GET", "/smodel", nil)
	w := httptest.NewRecorder()

	SModeling(w, req, sys)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	if !strings.HasPrefix(w.Body.String(), "package 'testhost_mysys' {") {
		t.Error("body must begin with the SysML package declaration")
	}
}
