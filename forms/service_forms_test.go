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

package forms

import (
	"encoding/json"
	"testing"
)

// Unpack resolves a payload by its version string, so a form the map does not
// know about cannot cross the wire at all.
func TestRegistryEventIsRegistered(t *testing.T) {
	if _, ok := FormTypeMap["RegistryEvent_v1"]; !ok {
		t.Error("RegistryEvent_v1 is not in FormTypeMap, so Unpack cannot resolve it")
	}

	var f RegistryEvent_v1
	if got := f.NewForm().FormVersion(); got != "RegistryEvent_v1" {
		t.Errorf("version %q, want RegistryEvent_v1", got)
	}
}

// The event carries the registration record whole rather than a summary of it.
// This checks that everything a subscriber acts on survives the round trip —
// which system, which service, where to reach it, and the details the
// registration carried.
func TestRegistryEventCarriesTheWholeRecord(t *testing.T) {
	var rec ServiceRecord_v1
	rec.NewForm()
	rec.Id = 7
	rec.SystemName = "ds18b20"
	rec.ServiceDefinition = "temperature"
	rec.SubPath = "sensor_Id/temperature"
	rec.Mission = "measurement"
	rec.IPAddresses = []string{"10.0.0.33"}
	rec.ProtoPort = map[string]int{"http": 20150, "https": 30150}
	rec.Details = map[string][]string{
		"Unit":               {"<http://qudt.org/vocab/unit/DEG_C>"},
		"FunctionalLocation": {"Kitchen"},
	}

	var sent RegistryEvent_v1
	sent.NewForm()
	sent.Change = RegistryRegistered
	sent.Record = rec
	sent.Timestamp = "2026-08-09T09:00:00Z"

	raw, err := json.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// json.Unmarshal here rather than usecases.Unpack: forms is the lower
	// package and cannot import the one that resolves a version string. That
	// resolution is covered by TestRegistryEventIsRegistered above.
	var back RegistryEvent_v1
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.Change != RegistryRegistered {
		t.Errorf("change = %q, want %q", back.Change, RegistryRegistered)
	}
	if back.Record.SystemName != "ds18b20" || back.Record.ServiceDefinition != "temperature" {
		t.Errorf("the record lost its identity: %+v", back.Record)
	}
	// A subscriber builds the URL to call from these, exactly as /syslist does,
	// which is why the event does not restate one.
	if back.Record.ProtoPort["https"] != 30150 || len(back.Record.IPAddresses) != 1 {
		t.Errorf("the record lost its address: %v %v", back.Record.IPAddresses, back.Record.ProtoPort)
	}
	if got := back.Record.Details["Unit"]; len(got) != 1 || got[0] != "<http://qudt.org/vocab/unit/DEG_C>" {
		t.Errorf("the record lost its details: %v", back.Record.Details)
	}
	if back.Record.Mission != "measurement" {
		t.Errorf("mission = %q; an authorization-aware subscriber reads it", back.Record.Mission)
	}
}

// A deregistration carries the record as it last stood, so a subscriber can act
// on what left without having kept its own copy.
func TestRegistryEventReportsWhatLeft(t *testing.T) {
	var rec ServiceRecord_v1
	rec.NewForm()
	rec.SystemName = "parallax"
	rec.ServiceDefinition = "rotation"

	var sent RegistryEvent_v1
	sent.NewForm()
	sent.Change = RegistryDeregistered
	sent.Record = rec

	raw, err := json.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back RegistryEvent_v1
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.Change != RegistryDeregistered {
		t.Errorf("change = %q, want %q", back.Change, RegistryDeregistered)
	}
	if back.Record.SystemName != "parallax" {
		t.Errorf("a subscriber cannot tell what left: %+v", back.Record)
	}
}
