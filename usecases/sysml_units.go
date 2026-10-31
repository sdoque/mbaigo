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

import "strings"

// sysmlQuantity is what a service's value is in SysML v2: the quantity value
// type it is typed by, and the measurement reference its numbers are on.
type sysmlQuantity struct {
	ValueType string // e.g. "ISQ::LengthValue"
	Ref       string // e.g. "SI::m"
}

// sysmlUnits maps a QUDT unit to the SysML v2 standard library (release
// 2026-08, "Quantities and Units"). A unit the library lacks — percent, RPM,
// the millisecond, the hectopascal — is declared in mAF and referenced as
// mAF::<name>; the declarations are in systems/modeler/mAF.sysml, and the
// modeler's tests fail if one referenced here is missing there.
//
// The SysML model and the knowledge graph then name a service's unit the same
// way, which is what lets the two be compared on it. Before this the model
// carried no units at all, and a temperature service could not say °C.
var sysmlUnits = map[string]sysmlQuantity{
	// Absolute temperature. Kelvin and Rankine are units of thermodynamic
	// temperature. The library has no such unit for degrees Celsius or
	// Fahrenheit: '°C' is a unit of temperature *difference*, and an absolute
	// Celsius reading is on the interval scale '°C_abs'. ThermodynamicTemperature
	// Value accepts only a unit, so a reading on '°C_abs' is typed by the general
	// ScalarQuantityValue, and the quantity kind is carried by qudtQuantityKind.
	qudtUnit + "K":     {"ISQ::ThermodynamicTemperatureValue", "SI::K"},
	qudtUnit + "DEG_R": {"ISQ::ThermodynamicTemperatureValue", "USCustomaryUnits::'°R'"},
	qudtUnit + "DEG_C": {"Quantities::ScalarQuantityValue", "SI::'°C_abs'"},
	qudtUnit + "DEG_F": {"Quantities::ScalarQuantityValue", "USCustomaryUnits::'°F_abs'"},

	qudtUnit + "PA":       {"ISQ::PressureValue", "SI::Pa"},
	qudtUnit + "HectoPA":  {"ISQ::PressureValue", "mAF::hPa"},
	qudtUnit + "KiloPA":   {"ISQ::PressureValue", "mAF::kPa"},
	qudtUnit + "BAR":      {"ISQ::PressureValue", "mAF::bar"},
	qudtUnit + "MilliBAR": {"ISQ::PressureValue", "mAF::mbar"},

	qudtUnit + "SEC":      {"ISQ::DurationValue", "SI::s"},
	qudtUnit + "MilliSEC": {"ISQ::DurationValue", "mAF::ms"},
	qudtUnit + "MIN":      {"ISQ::DurationValue", "SI::min"},
	qudtUnit + "HR":       {"ISQ::DurationValue", "SI::h"},

	qudtUnit + "M":       {"ISQ::LengthValue", "SI::m"},
	qudtUnit + "MilliM":  {"ISQ::LengthValue", "SI::mm"},
	qudtUnit + "FT":      {"ISQ::LengthValue", "USCustomaryUnits::ft"},
	qudtUnit + "FURLONG": {"ISQ::LengthValue", "mAF::fur"},

	qudtUnit + "KG": {"ISQ::MassValue", "SI::kg"},
	qudtUnit + "LB": {"ISQ::MassValue", "USCustomaryUnits::lb"},

	qudtUnit + "RAD": {"ISQ::AngularMeasureValue", "SI::rad"},
	qudtUnit + "DEG": {"ISQ::AngularMeasureValue", "SI::'°'"},
	qudtUnit + "GON": {"ISQ::AngularMeasureValue", "mAF::gon"},
	qudtUnit + "REV": {"ISQ::AngularMeasureValue", "mAF::r"},

	qudtUnit + "REV-PER-MIN": {"ISQ::AngularVelocityValue", "mAF::'r/min'"},
	qudtUnit + "PER-M":       {"ISQ::CurvatureValue", "SI::'m⁻¹'"},

	qudtUnit + "M-PER-SEC":             {"ISQ::SpeedValue", "SI::'m/s'"},
	qudtUnit + "KiloM-PER-HR":          {"ISQ::SpeedValue", "SI::'km/h'"},
	qudtUnit + "MilliM-PER-HR":         {"ISQ::SpeedValue", "mAF::'mm/h'"},
	qudtUnit + "FURLONG_PER_FORTNIGHT": {"ISQ::SpeedValue", "mAF::'fur/ftn'"},

	qudtUnit + "W":    {"ISQ::PowerValue", "SI::W"},
	qudtUnit + "W-HR": {"ISQ::EnergyValue", "SI::'W⋅h'"},
	qudtUnit + "LUX":  {"ISQ::IlluminanceValue", "SI::lx"},

	qudtUnit + "PERCENT": {"MeasurementReferences::DimensionOneValue", "mAF::'%'"},
	qudtUnit + "PPM":     {"MeasurementReferences::DimensionOneValue", "mAF::ppm"},
	qudtUnit + "NUM":     {"MeasurementReferences::DimensionOneValue", "MeasurementReferences::one"},

	// Sound level. The library types the decibel as a sound pressure level
	// unit, which is what the systems here measure with it.
	qudtUnit + "DeciB": {"ISQ::SoundPressureLevelValue", "SI::dB"},
}

// sysmlIntervals are the units whose difference is on another reference than
// their reading: a temperature deviation of 2 °C is a TemperatureDifferenceValue
// in '°C', not a point on '°C_abs'. The same distinction the framework makes
// when converting (Measure: interval), so 5 °C of error is 9 °F and not 41.
var sysmlIntervals = map[string]sysmlQuantity{
	qudtUnit + "K":     {"ISQ::TemperatureDifferenceValue", "SI::K"},
	qudtUnit + "DEG_R": {"ISQ::TemperatureDifferenceValue", "USCustomaryUnits::'°R'"},
	qudtUnit + "DEG_C": {"ISQ::TemperatureDifferenceValue", "SI::'°C'"},
	qudtUnit + "DEG_F": {"ISQ::TemperatureDifferenceValue", "USCustomaryUnits::'°F'"},
}

// sysmlKinds narrows the value type where the quantity kind says more than the
// unit: a percentage that is a relative humidity is a RelativeHumidityValue.
var sysmlKinds = map[string]string{
	qudtKind + "RelativeHumidity": "ISQ::RelativeHumidityValue",
}

// sysmlQuantityFor reads a service's (or a cervice's) declared unit, quantity
// kind and measure, and says what its value is in SysML v2. ok is false when no
// unit is declared or the unit has no mapping; the QUDT identifiers are
// returned either way, so the model can still name what it cannot type.
func sysmlQuantityFor(details map[string][]string) (q sysmlQuantity, unitIRI, kindIRI string, ok bool) {
	unitIRI = bareIRI(firstDetail(details, "Unit"))
	if def, known := LookupUnit(unitIRI); known {
		unitIRI = def.IRI // a legacy name, "Celsius", becomes its QUDT unit
	}
	kindIRI = bareIRI(firstDetail(details, "QuantityKind"))
	if unitIRI == "" {
		return sysmlQuantity{}, unitIRI, kindIRI, false
	}
	if isInterval(details) {
		if q, ok = sysmlIntervals[unitIRI]; ok {
			return q, unitIRI, kindIRI, true
		}
	}
	if q, ok = sysmlUnits[unitIRI]; !ok {
		return sysmlQuantity{}, unitIRI, kindIRI, false
	}
	if vt, narrower := sysmlKinds[kindIRI]; narrower {
		q.ValueType = vt
	}
	return q, unitIRI, kindIRI, true
}

// SysMLUnitRefs lists every measurement reference the mapping uses, so the
// modeler can check that each one it expects mAF to declare is declared.
func SysMLUnitRefs() []string {
	var refs []string
	for _, table := range []map[string]sysmlQuantity{sysmlUnits, sysmlIntervals} {
		for _, q := range table {
			refs = append(refs, q.Ref)
		}
	}
	return refs
}

func bareIRI(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "<"), ">")
}
