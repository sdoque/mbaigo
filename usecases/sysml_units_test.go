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
	"strings"
	"testing"
)

func details(kv ...string) map[string][]string {
	d := make(map[string][]string)
	for i := 0; i+1 < len(kv); i += 2 {
		d[kv[i]] = append(d[kv[i]], kv[i+1])
	}
	return d
}

// A thermometer reads on the Celsius scale; a controller's deviation is a
// difference of two temperatures. SysML v2 keeps them apart ('°C_abs' against
// '°C') exactly as the framework does when it converts (5 °C of error is 9 °F,
// not 41), so the model must not collapse them.
func TestATemperatureAndItsDeviationAreDifferentQuantities(t *testing.T) {
	reading, _, _, ok := sysmlQuantityFor(details("Unit", "<http://qudt.org/vocab/unit/DEG_C>"))
	if !ok || reading.Ref != "SI::'°C_abs'" {
		t.Errorf("a reading in °C: %+v, want the absolute scale SI::'°C_abs'", reading)
	}
	deviation, _, _, ok := sysmlQuantityFor(details("Unit", "<http://qudt.org/vocab/unit/DEG_C>", "Measure", "interval"))
	if !ok || deviation.ValueType != "ISQ::TemperatureDifferenceValue" || deviation.Ref != "SI::'°C'" {
		t.Errorf("a deviation in °C: %+v, want a TemperatureDifferenceValue in SI::'°C'", deviation)
	}
}

// The unit is found however the configuration spelled it.
func TestAUnitIsFoundInEitherSpelling(t *testing.T) {
	for _, spelled := range []string{"<http://qudt.org/vocab/unit/DEG_C>", "http://qudt.org/vocab/unit/DEG_C", "Celsius"} {
		_, iri, _, ok := sysmlQuantityFor(details("Unit", spelled))
		if !ok || iri != "http://qudt.org/vocab/unit/DEG_C" {
			t.Errorf("%q: %q ok=%v", spelled, iri, ok)
		}
	}
}

// A percentage that is a relative humidity is typed as one.
func TestTheQuantityKindNarrowsTheType(t *testing.T) {
	q, _, _, _ := sysmlQuantityFor(details(
		"Unit", "<http://qudt.org/vocab/unit/PERCENT>",
		"QuantityKind", "<http://qudt.org/vocab/quantitykind/RelativeHumidity>"))
	if q.ValueType != "ISQ::RelativeHumidityValue" {
		t.Errorf("relative humidity: %+v", q)
	}
	valve, _, _, _ := sysmlQuantityFor(details("Unit", "<http://qudt.org/vocab/unit/PERCENT>"))
	if valve.ValueType != "MeasurementReferences::DimensionOneValue" {
		t.Errorf("a valve opening in %%: %+v", valve)
	}
}

// A unit with no mapping is still named, so the model says what it cannot type.
func TestAnUnmappedUnitIsNamedNotTyped(t *testing.T) {
	out := sysmlPort("out", "acidity", "provided service", details("Unit", "<http://qudt.org/vocab/unit/PH>"), portFacts{})
	if strings.Contains(out, ":>> value") {
		t.Errorf("an unmapped unit was typed:\n%s", out)
	}
	if !strings.Contains(out, `qudtUnit = "http://qudt.org/vocab/unit/PH"`) || !strings.Contains(out, "no SysML v2 unit is mapped for PH") {
		t.Errorf("an unmapped unit was not named:\n%s", out)
	}
}

// Every unit the framework can convert has a SysML v2 counterpart, so a
// deployment the framework accepts can be modeled with its units.
func TestEveryConvertibleUnitIsMapped(t *testing.T) {
	for iri, def := range units {
		if !def.HasFactor {
			continue
		}
		if _, ok := sysmlUnits[iri]; !ok {
			t.Errorf("%s converts at runtime but has no SysML v2 mapping", local(iri))
		}
	}
}

func TestAPortCarriesItsUnit(t *testing.T) {
	out := sysmlPort("out", "temperature", "provided service", details(
		"Unit", "<http://qudt.org/vocab/unit/DEG_C>",
		"QuantityKind", "<http://qudt.org/vocab/quantitykind/ThermodynamicTemperature>"), portFacts{})
	for _, want := range []string{
		"out port 'temperature' : 'Temperature' {  // provided service",
		"attribute :>> value : Quantities::ScalarQuantityValue { :>> mRef = SI::'°C_abs'; }",
		`attribute :>> qudtUnit = "http://qudt.org/vocab/unit/DEG_C";`,
		`attribute :>> qudtQuantityKind = "http://qudt.org/vocab/quantitykind/ThermodynamicTemperature";`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if bare := sysmlPort("in", "OnOff", "consumed service", nil, portFacts{}); bare != "        in port 'OnOff' : 'OnOffPort';  // consumed service\n" {
		t.Errorf("a port with no unit: %q", bare)
	}
}
