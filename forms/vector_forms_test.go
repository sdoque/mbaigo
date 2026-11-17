/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this
 * repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 ***************************************************************************SDG*/

package forms

import (
	"encoding/json"
	"math"
	"testing"
)

func TestVectorChecksWhatCannotBeRead(t *testing.T) {
	good := VectorA_v1a{X: 0.1, Y: -0.2, Z: 9.81, Frame: VectorVehicle}
	if err := good.Check(); err != nil {
		t.Errorf("%+v refused: %v", good, err)
	}
	bad := map[string]VectorA_v1a{
		"NaN x":         {X: math.NaN(), Frame: VectorVehicle},
		"infinite z":    {Z: math.Inf(1), Frame: VectorVehicle},
		"no frame":      {X: 1},
		"unknown frame": {X: 1, Frame: "chip"},
	}
	for name, v := range bad {
		if err := v.Check(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestVectorDistance(t *testing.T) {
	a := &VectorA_v1a{X: 1, Y: 2, Z: 3, Unit: "u", Frame: VectorVehicle}
	b := &VectorA_v1a{X: 1, Y: 5, Z: 7, Unit: "u", Frame: VectorVehicle}
	if d, ok := b.DistanceFrom(a); !ok || d != 5 {
		t.Errorf("distance %v (%v), want 5", d, ok)
	}
	c := *b
	c.Unit = "w"
	if _, ok := c.DistanceFrom(a); ok {
		t.Error("vectors in different units were compared")
	}
	if _, ok := b.DistanceFrom(&SignalA_v1a{}); ok {
		t.Error("a vector was compared with a signal")
	}
	var _ Distancer = b
}

// What a person types with curl must unpack into the form.
func TestVectorFromCurl(t *testing.T) {
	body := `{"x": 0, "y": 0, "z": 9.81, "unit": "http://qudt.org/vocab/unit/M-PER-SEC2", "frame": "vehicle", "version": "VectorA_v1.0"}`
	typ, ok := FormTypeMap["VectorA_v1.0"]
	if !ok {
		t.Fatal("VectorA_v1.0 is not registered")
	}
	if typ.Name() != "VectorA_v1a" {
		t.Errorf("VectorA_v1.0 registers %s", typ.Name())
	}
	var v VectorA_v1a
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	if v.Z != 9.81 || v.Frame != VectorVehicle || v.Check() != nil {
		t.Errorf("unpacked %+v", v)
	}
}
