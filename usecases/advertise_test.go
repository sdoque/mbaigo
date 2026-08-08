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
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package usecases

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// TestOnlyBoundPortsAreAdvertised is the defect this test was written for, seen
// on a live cloud: a system registered every configured port, including the
// HTTPS one, while its HTTPS listener was still waiting for a certificate.
// ConvertToServicePoint prefers HTTPS, so every consumer was handed a port
// nothing was listening on — for the eight minutes enrollment took — while the
// system's HTTP port served correctly the whole time.
func TestOnlyBoundPortsAreAdvertised(t *testing.T) {
	sys := createTestSystem(false)
	sys.Husk.ProtoPort = map[string]int{"http": 20150, "https": 30150}

	// The state between startup and enrollment: configured for both, serving one.
	sys.Husk.Bound.Bind("http", 20150)

	var ua *components.UnitAsset
	for _, asset := range sys.UAssets {
		ua = asset
		break
	}
	if ua == nil {
		t.Fatal("no unit asset in the test system")
	}
	var serv *components.Service
	for _, s := range (*ua).GetServices() {
		serv = s
		break
	}

	record := func() forms.ServiceRecord_v1 {
		payload, err := serviceRegistrationForm(&sys, ua, serv, "ServiceRecord_v1")
		if err != nil {
			t.Fatalf("building the registration: %v", err)
		}
		var sr forms.ServiceRecord_v1
		if err := json.Unmarshal(payload, &sr); err != nil {
			t.Fatalf("unpacking the registration: %v", err)
		}
		return sr
	}

	sr := record()
	if _, advertised := sr.ProtoPort["https"]; advertised {
		t.Errorf("an unbound HTTPS port was advertised: %v", sr.ProtoPort)
	}
	if sr.ProtoPort["http"] != 20150 {
		t.Errorf("the bound HTTP port was not advertised: %v", sr.ProtoPort)
	}

	// What a consumer is actually given must be reachable.
	sr.IPAddresses = []string{"10.0.0.33"}
	sr.SystemName, sr.SubPath = "ds18b20", "sensor/temperature"
	if sp := ConvertToServicePoint(sr); !strings.HasPrefix(sp.ServLocation, "http://10.0.0.33:20150/") {
		t.Errorf("a consumer is sent to %q, which nothing is listening on", sp.ServLocation)
	}

	// Enrollment completes and the listener binds. The next registration says so.
	sys.Husk.Bound.Bind("https", 30150)
	sr = record()
	if sr.ProtoPort["https"] != 30150 {
		t.Errorf("a bound HTTPS port was not advertised: %v", sr.ProtoPort)
	}

	sr.IPAddresses = []string{"10.0.0.33"}
	sr.SystemName, sr.SubPath = "ds18b20", "sensor/temperature"
	if sp := ConvertToServicePoint(sr); !strings.HasPrefix(sp.ServLocation, "https://10.0.0.33:30150/") {
		t.Errorf("HTTPS is bound but a consumer is sent to %q", sp.ServLocation)
	}
}

// TestRegistrationWaitsForAListener: registration starts before the servers do,
// and a record with no ports would send a consumer to port 0 — worse than no
// record at all.
func TestRegistrationWaitsForAListener(t *testing.T) {
	sys := createTestSystem(false)
	sys.Husk.Bound.Release("http")

	var ua *components.UnitAsset
	for _, asset := range sys.UAssets {
		ua = asset
		break
	}
	var serv *components.Service
	for _, s := range (*ua).GetServices() {
		serv = s
		break
	}

	// The fixture presets an ID, so clear it: what matters is that registration
	// does not go out, and a service that has never registered has no ID.
	serv.ID = 0

	delay, err := registerService(&sys, "http://registrar/registry", ua, serv)
	if err != nil {
		t.Fatalf("registering with nothing bound: %v", err)
	}
	if delay <= 0 {
		t.Errorf("delay %v; a system with no listener should retry rather than give up", delay)
	}
	if serv.ID != 0 {
		t.Errorf("a service with no listener was registered as ID %d", serv.ID)
	}
}
