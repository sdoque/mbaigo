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
	"strings"
	"testing"
)

func TestPolarChecksWhatCannotBeRead(t *testing.T) {
	ok := func(p PolarA_v1a) PolarA_v1a { p.NewForm(); return p }
	good := []PolarA_v1a{
		ok(PolarA_v1a{Magnitude: 12, Direction: 30, Frame: PolarVehicle}),
		ok(PolarA_v1a{Magnitude: 4.2, Direction: 270, Frame: PolarCompassFrom}),
		ok(PolarA_v1a{Magnitude: 1, Direction: 90, Frame: PolarCompassTo}),
		ok(PolarA_v1a{Magnitude: 0, Direction: 0, Frame: "map@2026-09-27T10:00:00Z"}),
		ok(PolarA_v1a{Magnitude: 3, Direction: 0, Frame: PolarMap}),
	}
	for _, p := range good {
		if err := p.Check(); err != nil {
			t.Errorf("%+v refused: %v", p, err)
		}
	}
	bad := map[string]PolarA_v1a{
		"negative":       {Magnitude: -1, Frame: PolarVehicle},
		"NaN magnitude":  {Magnitude: math.NaN(), Frame: PolarVehicle},
		"NaN direction":  {Magnitude: 1, Direction: math.NaN(), Frame: PolarVehicle},
		"no frame":       {Magnitude: 1},
		"unknown frame":  {Magnitude: 1, Frame: "north-ish"},
		"map, no prefix": {Magnitude: 1, Frame: "mapping"},
	}
	for name, p := range bad {
		if err := p.Check(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := (&PolarA_v1a{Magnitude: -2, Frame: PolarVehicle}).Check(); !strings.Contains(err.Error(), "half round") {
		t.Errorf("a negative magnitude does not say how to fix it: %v", err)
	}
}

func TestPolarRadians(t *testing.T) {
	for _, c := range []struct {
		unit string
		dir  float64
		want float64
	}{
		{"", 90, math.Pi / 2},
		{unitDegree, -45, -math.Pi / 4},
		{unitRadian, 1.5, 1.5},
	} {
		p := PolarA_v1a{Direction: c.dir, DirectionUnit: c.unit}
		if got, err := p.Radians(); err != nil || math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%v %q = %v (%v), want %v", c.dir, c.unit, got, err, c.want)
		}
	}
	if _, err := (&PolarA_v1a{Direction: 1, DirectionUnit: "<http://qudt.org/vocab/unit/GON>"}).Radians(); err == nil {
		t.Error("gradians were taken for degrees or radians")
	}
}

// What a person types with curl must unpack into the form.
func TestPolarFromCurl(t *testing.T) {
	body := `{"magnitude": 10, "direction": 15, "frame": "vehicle", "version": "PolarA_v1.0"}`
	typ, ok := FormTypeMap["PolarA_v1.0"]
	if !ok {
		t.Fatal("PolarA_v1.0 is not registered")
	}
	if typ.Name() != "PolarA_v1a" {
		t.Errorf("PolarA_v1.0 registers %s", typ.Name())
	}
	var p PolarA_v1a
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p.Magnitude != 10 || p.Direction != 15 || p.Frame != PolarVehicle || p.Check() != nil {
		t.Errorf("unpacked %+v", p)
	}
}

func TestPathCheck(t *testing.T) {
	p := PathA_v1a{X: []float64{0, 1}, Y: []float64{0, 0}, Heading: []float64{0, 0}, Frame: "map@x"}
	if err := p.Check(); err != nil {
		t.Errorf("a well-formed path was refused: %v", err)
	}
	p.Heading = p.Heading[:1]
	if p.Check() == nil {
		t.Error("arrays out of step were accepted")
	}
	p = PathA_v1a{X: []float64{0}, Y: []float64{0}, Heading: []float64{0}}
	if p.Check() == nil {
		t.Error("a path with no frame was accepted")
	}
	if _, ok := FormTypeMap["PathA_v1.0"]; !ok {
		t.Error("PathA_v1.0 is not registered")
	}
}
