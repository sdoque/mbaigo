/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
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
	"sync"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newKGTestSystem returns a minimal *components.System suitable for kgraphing
// tests. It avoids NewDevice() (which calls OS functions) by constructing the
// HostingDevice manually.
func newKGTestSystem() *components.System {
	host := &components.HostingDevice{
		Name:        "testhost",
		IPAddresses: []string{"192.0.2.1"},
	}
	sys := &components.System{
		Name: "mysys",
		Husk: &components.Husk{
			Host:      host,
			ProtoPort: map[string]int{"http": 8080},
			Details:   map[string][]string{},
		},
		UAssets: make(map[string]*components.UnitAsset),
		Mutex:   &sync.Mutex{},
	}
	return sys
}

// addTestAsset populates sys.UAssets with a single UnitAsset carrying one
// service and one cervice. Callers can further modify the returned asset.
func addTestAsset(sys *components.System) *components.UnitAsset {
	svc := &components.Service{
		Definition: "temperature",
		SubPath:    "temp",
		Details:    map[string][]string{"Forms": {"application/json"}},
		RegPeriod:  30,
	}
	cerv := &components.Cervice{
		Definition: "humidity",
		Details:    map[string][]string{},
		Nodes:      make(map[string][]components.NodeInfo),
	}
	ua := &components.UnitAsset{
		Name:        "sensor1",
		Mission:     "measure_things",
		Details:     map[string][]string{},
		ServicesMap: components.Services{"temp": svc},
		CervicesMap: components.Cervices{"humidity": cerv},
	}
	sys.UAssets["sensor1"] = ua
	return ua
}

// ── prefixes ──────────────────────────────────────────────────────────────────

func TestPrefixes(t *testing.T) {
	p := prefixes()

	for _, want := range []string{
		"@prefix alc:",
		"@prefix afo:",
		"@prefix owl:",
		"@prefix rdf:",
		"@prefix rdfs:",
		"@prefix xsd:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prefixes() missing %q", want)
		}
	}

	// Must end with a blank line (two newlines after the last declaration).
	if !strings.HasSuffix(p, "\n\n") {
		t.Error("prefixes() must end with a blank line")
	}
}

// ── finalizeBlock ─────────────────────────────────────────────────────────────

func TestFinalizeBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trailing semicolon is removed",
			input: "alc:foo a afo:Bar ;\n    afo:hasName \"foo\" ;",
			want:  "alc:foo a afo:Bar ;\n    afo:hasName \"foo\" .\n\n",
		},
		{
			name:  "no trailing semicolon left unchanged (dot appended)",
			input: "alc:foo a afo:Bar ;\n    afo:hasName \"foo\"",
			want:  "alc:foo a afo:Bar ;\n    afo:hasName \"foo\" .\n\n",
		},
		{
			name:  "trailing whitespace after semicolon is cleaned",
			input: "alc:foo a afo:Bar ;   \n",
			want:  "alc:foo a afo:Bar .\n\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := finalizeBlock(tc.input)
			if got != tc.want {
				t.Errorf("finalizeBlock(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── endpointLocalName ─────────────────────────────────────────────────────────

func TestEndpointLocalName(t *testing.T) {
	sys := newKGTestSystem()
	got := endpointLocalName(sys, "http", 8080)
	want := "testhost_mysys_http_8080_Endpoint"
	if got != want {
		t.Errorf("endpointLocalName = %q, want %q", got, want)
	}
}

// ── modelSystem ───────────────────────────────────────────────────────────────

func TestModelSystem(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	out := modelSystem(sys)

	// Subject IRI
	if !strings.Contains(out, "alc:testhost_mysys a afo:System") {
		t.Error("modelSystem: missing System IRI")
	}
	// System name literal
	if !strings.Contains(out, `afo:hasName "mysys"`) {
		t.Error("modelSystem: missing hasName")
	}
	// Husk link
	if !strings.Contains(out, "afo:hasHusk alc:testhost_mysys_Husk") {
		t.Error("modelSystem: missing hasHusk")
	}
	// UnitAsset link
	if !strings.Contains(out, "afo:hasUnitAsset alc:testhost_mysys_sensor1") {
		t.Error("modelSystem: missing hasUnitAsset")
	}
	// Must be a valid finalised block
	if !strings.HasSuffix(out, ".\n\n") {
		t.Error("modelSystem: output must end with ' .\\n\\n'")
	}
}

func TestModelSystemLocalCloud(t *testing.T) {
	sys := newKGTestSystem()
	sys.Husk.Details["LocalCloud"] = []string{"MyCloud"}

	out := modelSystem(sys)

	if !strings.Contains(out, "afo:isContainedIn alc:MyCloud") {
		t.Errorf("modelSystem: expected isContainedIn; got:\n%s", out)
	}
}

// ── modelHusk ─────────────────────────────────────────────────────────────────

func TestModelHusk(t *testing.T) {
	sys := newKGTestSystem()

	out := modelHusk(sys)

	if !strings.Contains(out, "alc:testhost_mysys_Husk a afo:Husk") {
		t.Error("modelHusk: missing Husk IRI")
	}
	if !strings.Contains(out, "afo:runsOnHost alc:testhost") {
		t.Error("modelHusk: missing runsOnHost")
	}
	// Endpoint link (port 8080 ≠ 0, so it must appear)
	if !strings.Contains(out, "afo:communicatesOver alc:testhost_mysys_http_8080_Endpoint") {
		t.Error("modelHusk: missing communicatesOver link")
	}
	if !strings.HasSuffix(out, ".\n\n") {
		t.Error("modelHusk: output must end with ' .\\n\\n'")
	}
}

func TestModelHuskSkipsZeroPort(t *testing.T) {
	sys := newKGTestSystem()
	sys.Husk.ProtoPort["coap"] = 0

	out := modelHusk(sys)

	if strings.Contains(out, "coap") {
		t.Error("modelHusk: zero-port protocol must not appear in output")
	}
}

func TestModelHuskDetails(t *testing.T) {
	sys := newKGTestSystem()
	// LocalCloud must be skipped at Husk level (handled by modelSystem)
	sys.Husk.Details["LocalCloud"] = []string{"MyCloud"}
	sys.Husk.Details["Role"] = []string{"producer"}

	out := modelHusk(sys)

	if strings.Contains(out, "LocalCloud") {
		t.Error("modelHusk: LocalCloud must not appear in Husk block")
	}
	if !strings.Contains(out, "afo:hasRole alc:producer") {
		t.Error("modelHusk: expected hasRole detail")
	}
}

// ── modelHost ─────────────────────────────────────────────────────────────────

func TestModelHost(t *testing.T) {
	sys := newKGTestSystem()

	out := modelHost(sys)

	if !strings.Contains(out, "alc:testhost a afo:Host") {
		t.Error("modelHost: missing Host IRI")
	}
	if !strings.Contains(out, `afo:hasName "testhost"`) {
		t.Error("modelHost: missing hasName")
	}
	if !strings.Contains(out, `afo:hasIPaddress "192.0.2.1"`) {
		t.Error("modelHost: missing IP address")
	}
	if !strings.HasSuffix(out, ".\n\n") {
		t.Error("modelHost: output must end with ' .\\n\\n'")
	}
}

// ── modelEndpoints ────────────────────────────────────────────────────────────

func TestModelEndpoints(t *testing.T) {
	sys := newKGTestSystem()

	out := modelEndpoints(sys)

	if !strings.Contains(out, "alc:testhost_mysys_http_8080_Endpoint a afo:Endpoint") {
		t.Error("modelEndpoints: missing Endpoint IRI")
	}
	if !strings.Contains(out, `afo:usesProtocol "http"`) {
		t.Error("modelEndpoints: missing usesProtocol")
	}
	if !strings.Contains(out, "afo:usesPort 8080") {
		t.Error("modelEndpoints: missing usesPort")
	}
	if !strings.Contains(out, "afo:onHost alc:testhost") {
		t.Error("modelEndpoints: missing onHost")
	}
}

func TestModelEndpointsSkipsZeroPort(t *testing.T) {
	sys := newKGTestSystem()
	sys.Husk.ProtoPort["http"] = 0 // disable the only port

	out := modelEndpoints(sys)

	if out != "" {
		t.Errorf("modelEndpoints: expected empty output for zero port, got %q", out)
	}
}

// ── modelCervices ─────────────────────────────────────────────────────────────

func TestModelCervicesEmpty(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)

	out := modelCervices("testhost_mysys", ua)

	if !strings.Contains(out, "alc:testhost_mysys_sensor1_humidity a afo:ConsumedService") {
		t.Error("modelCervices: missing ConsumedService IRI")
	}
	if !strings.Contains(out, `afo:consumes "humidity"`) {
		t.Error("modelCervices: missing consumes literal")
	}
}

func TestModelCervicesWithNodes(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)

	// Populate the cervice's Nodes with a NodeInfo so the URL path is exercised.
	cerv := ua.CervicesMap["humidity"]
	cerv.Nodes["providerA"] = []components.NodeInfo{
		{URL: "http://192.0.2.2:8080/humidity/access"},
	}

	out := modelCervices("testhost_mysys", ua)

	if !strings.Contains(out, "afo:consumes alc:providerA") {
		t.Errorf("modelCervices: expected provider IRI; got:\n%s", out)
	}
	if !strings.Contains(out, "<http://192.0.2.2:8080/humidity/access>") {
		t.Errorf("modelCervices: expected fromUrl IRI; got:\n%s", out)
	}
}

// ── modelServices ─────────────────────────────────────────────────────────────

func TestModelServices(t *testing.T) {
	sys := newKGTestSystem()
	ua := addTestAsset(sys)

	out := modelServices("testhost_mysys", ua, sys)

	if !strings.Contains(out, "alc:testhost_mysys_sensor1_temperature a afo:Service") {
		t.Error("modelServices: missing Service IRI")
	}
	if !strings.Contains(out, `afo:hasServiceDefinition "temperature"`) {
		t.Error("modelServices: missing hasServiceDefinition")
	}
	// URL must include the system name, asset name, and sub-path
	if !strings.Contains(out, "http://192.0.2.1:8080/mysys/sensor1/temp") {
		t.Error("modelServices: missing or wrong hasUrl")
	}
	if !strings.Contains(out, "afo:hostedOnEndpoint alc:testhost_mysys_http_8080_Endpoint") {
		t.Error("modelServices: missing hostedOnEndpoint")
	}
}

// ── KGraphing (HTTP handler) ──────────────────────────────────────────────────

func TestKGraphing(t *testing.T) {
	sys := newKGTestSystem()
	addTestAsset(sys)

	req := httptest.NewRequest("GET", "/kgraph", nil)
	w := httptest.NewRecorder()

	KGraphing(w, req, sys)

	if w.Code != 200 {
		t.Fatalf("KGraphing status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/turtle" {
		t.Errorf("Content-Type = %q, want text/turtle", ct)
	}

	body := w.Body.String()

	// Spot-check that all major sections appear.
	for _, want := range []string{
		"@prefix alc:",
		"a afo:System",
		"a afo:Husk",
		"a afo:Host",
		"a afo:Endpoint",
		"a afo:UnitAsset",
		"a afo:ConsumedService",
		"a afo:Service",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("KGraphing response missing %q", want)
		}
	}
}
