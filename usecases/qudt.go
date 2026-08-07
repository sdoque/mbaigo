/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
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

// Unit conversion follows QUDT's own pattern: every unit carries a multiplier
// and an offset relative to the SI coincident unit of its dimension, so a
// conversion is two affine steps through SI rather than a pairwise table. That
// is N entries instead of N squared, and it is why the same code converts
// temperature, pressure, length and mass without special cases.
//
//	si       = value * from.Multiplier + from.Offset
//	value_to = (si - to.Offset) / to.Multiplier
//
// The offset is zero for everything except temperature and gauge pressure, so
// most conversions collapse to a scaling. Temperature is the case that exercises
// the term the others leave at zero — and the one where the difference between a
// reading and an interval matters.
//
// The rationale is written up in systems/authorizer/QUDT.md.

package usecases

import (
	"fmt"
	"strings"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// UnitDef is one unit's relationship to the SI unit of its dimension.
type UnitDef struct {
	// IRI is the QUDT unit, e.g. http://qudt.org/vocab/unit/DEG_C.
	IRI string

	// Symbol is for display and log messages only. Nothing decides on it: two
	// units can print the same symbol and mean different things.
	Symbol string

	// Dimension is QUDT's dimension vector local name. Two units can be
	// converted only if these match, which is what stops a temperature being
	// read as a length.
	Dimension string

	// QuantityKind is what the unit measures. It is a finer guard than the
	// dimension, and the only one that helps where the dimension is empty: a
	// plane angle and a plain ratio are both dimensionless in SI, so nothing in
	// the arithmetic objects to turning 50% into 28.6 degrees. On a servo with
	// 180 degrees of travel the answer is 90, and no unit conversion can reach
	// it — that needs the actuator's range, which is calibration rather than
	// units. Refusing is the only correct response.
	QuantityKind string

	// Multiplier and Offset relate this unit to the SI coincident unit:
	// si = value*Multiplier + Offset.
	Multiplier float64
	Offset     float64

	// HasFactor is false for units whose relationship to SI is not affine —
	// decibels, pH, Richter. Their multiplier and offset are meaningless, so
	// conversion must refuse rather than produce a plausible number.
	HasFactor bool
}

// QUDT dimension vectors, in QUDT's own letter order: amount of substance,
// electric current, length, luminous intensity, mass, thermodynamic
// temperature, time, dimensionless.
//
// These are written out for the units below rather than derived. When the table
// is generated from a QUDT release they should come from qudt:hasDimensionVector
// verbatim; until then, treat them as this file's own convention and check them
// against the release before relying on the dimension guard for anything new.
const (
	DimTemperature   = "A0E0L0I0M0H1T0D0"
	DimPressure      = "A0E0L-1I0M1H0T-2D0"
	DimTime          = "A0E0L0I0M0H0T1D0"
	DimLength        = "A0E0L1I0M0H0T0D0"
	DimMass          = "A0E0L0I0M1H0T0D0"
	DimDimensionless = "A0E0L0I0M0H0T0D0"
)

// qudtUnit and qudtKind are the QUDT vocabulary namespaces.
const (
	qudtUnit = "http://qudt.org/vocab/unit/"
	qudtKind = "http://qudt.org/vocab/quantitykind/"
)

// The quantity kinds the units below measure. Like the dimension vectors these
// are written out rather than derived, and should be checked against the QUDT
// release when the table is generated — quantitykind:Angle in particular has a
// close neighbour in PlaneAngle.
const (
	KindTemperature = qudtKind + "ThermodynamicTemperature"
	KindPressure    = qudtKind + "Pressure"
	KindTime        = qudtKind + "Time"
	KindLength      = qudtKind + "Length"
	KindMass        = qudtKind + "Mass"
	KindRatio       = qudtKind + "DimensionlessRatio"
	KindAngle       = qudtKind + "Angle"
)

// units is a curated subset covering what the systems in this repository
// actually register, plus enough of length and mass to demonstrate that the
// pattern is not temperature-specific.
//
// This is deliberately not the whole vocabulary. QUDT has some 1900 units, and
// the intent recorded in QUDT.md is to generate the full table from a pinned
// release so that an unfamiliar unit in a configuration is impossible rather
// than merely unlikely. Until that generator exists, a unit outside this table
// is refused — which is the safe direction, but it does mean the table has to
// grow with the deployment.
//
// Figures are from the QUDT vocabulary and should be re-checked against the
// release the generator pins.
var units = map[string]UnitDef{
	// Temperature. The only entries here with a non-zero offset.
	qudtUnit + "K":     {Symbol: "K", QuantityKind: KindTemperature, Dimension: DimTemperature, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "DEG_C": {Symbol: "°C", QuantityKind: KindTemperature, Dimension: DimTemperature, Multiplier: 1, Offset: 273.15, HasFactor: true},
	qudtUnit + "DEG_F": {Symbol: "°F", QuantityKind: KindTemperature, Dimension: DimTemperature, Multiplier: 5.0 / 9.0, Offset: 255.37222222222223, HasFactor: true},
	qudtUnit + "DEG_R": {Symbol: "°R", QuantityKind: KindTemperature, Dimension: DimTemperature, Multiplier: 5.0 / 9.0, Offset: 0, HasFactor: true},

	// Pressure.
	qudtUnit + "PA":       {Symbol: "Pa", QuantityKind: KindPressure, Dimension: DimPressure, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "KiloPA":   {Symbol: "kPa", QuantityKind: KindPressure, Dimension: DimPressure, Multiplier: 1000, Offset: 0, HasFactor: true},
	qudtUnit + "BAR":      {Symbol: "bar", QuantityKind: KindPressure, Dimension: DimPressure, Multiplier: 100000, Offset: 0, HasFactor: true},
	qudtUnit + "MilliBAR": {Symbol: "mbar", QuantityKind: KindPressure, Dimension: DimPressure, Multiplier: 100, Offset: 0, HasFactor: true},

	// Time.
	qudtUnit + "SEC":      {Symbol: "s", QuantityKind: KindTime, Dimension: DimTime, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "MilliSEC": {Symbol: "ms", QuantityKind: KindTime, Dimension: DimTime, Multiplier: 0.001, Offset: 0, HasFactor: true},
	qudtUnit + "MIN":      {Symbol: "min", QuantityKind: KindTime, Dimension: DimTime, Multiplier: 60, Offset: 0, HasFactor: true},
	qudtUnit + "HR":       {Symbol: "h", QuantityKind: KindTime, Dimension: DimTime, Multiplier: 3600, Offset: 0, HasFactor: true},

	// Length and mass, to show the pattern is general.
	qudtUnit + "M":  {Symbol: "m", QuantityKind: KindLength, Dimension: DimLength, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "FT": {Symbol: "ft", QuantityKind: KindLength, Dimension: DimLength, Multiplier: 0.3048, Offset: 0, HasFactor: true},
	qudtUnit + "KG": {Symbol: "kg", QuantityKind: KindMass, Dimension: DimMass, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "LB": {Symbol: "lb", QuantityKind: KindMass, Dimension: DimMass, Multiplier: 0.45359237, Offset: 0, HasFactor: true},

	// Plane angle. Dimensionless in SI, exactly like a ratio — which is why the
	// quantity kind, not the dimension, is what keeps them apart.
	qudtUnit + "RAD": {Symbol: "rad", QuantityKind: KindAngle, Dimension: DimDimensionless, Multiplier: 1, Offset: 0, HasFactor: true},
	qudtUnit + "DEG": {Symbol: "°", QuantityKind: KindAngle, Dimension: DimDimensionless, Multiplier: 0.017453292519943295, Offset: 0, HasFactor: true},

	// Dimensionless ratios.
	qudtUnit + "PERCENT": {Symbol: "%", QuantityKind: KindRatio, Dimension: DimDimensionless, Multiplier: 0.01, Offset: 0, HasFactor: true},
	qudtUnit + "NUM":     {Symbol: "", QuantityKind: KindRatio, Dimension: DimDimensionless, Multiplier: 1, Offset: 0, HasFactor: true},

	// Logarithmic: present so that a request to convert one is refused with a
	// reason rather than failing to resolve.
	qudtUnit + "DeciB": {Symbol: "dB", Dimension: DimDimensionless, HasFactor: false},
	qudtUnit + "PH":    {Symbol: "pH", Dimension: DimDimensionless, HasFactor: false},
}

func init() {
	// The map key is the unit's identity; carrying it in the value too means a
	// UnitDef passed around on its own can still say what it is.
	for iri, def := range units {
		def.IRI = iri
		units[iri] = def
	}
}

// LookupUnit resolves a QUDT unit IRI.
//
// It accepts the angle-bracketed form that appears in configuration and in
// Turtle — "<http://qudt.org/vocab/unit/DEG_C>" — as well as the bare IRI, so a
// value can be written once in a way that is valid in both places.
func LookupUnit(iri string) (UnitDef, bool) {
	trimmed := strings.TrimSpace(iri)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")

	def, ok := units[trimmed]
	return def, ok
}

// Convert changes a value from one unit to another.
//
// interval says whether the value is a difference rather than a point on the
// scale. It matters only where the offset is non-zero, and there it matters a
// great deal: 5 °C of control error is 9 °F, not 41 °F. A controller's setpoint
// and its measurement are points; the error between them is an interval.
func Convert(value float64, from, to UnitDef, interval bool) (float64, error) {
	if from.IRI == to.IRI {
		return value, nil
	}
	if !from.HasFactor || !to.HasFactor {
		return 0, fmt.Errorf("no affine conversion between %s and %s: one of them is not linear in SI",
			describe(from), describe(to))
	}
	// Checked before the dimension, because it is the finer test and the only
	// one that catches units sharing an empty dimension vector.
	if from.QuantityKind != "" && to.QuantityKind != "" && from.QuantityKind != to.QuantityKind {
		return 0, fmt.Errorf("cannot convert %s to %s: different quantity kinds (%s vs %s)",
			describe(from), describe(to), local(from.QuantityKind), local(to.QuantityKind))
	}
	if from.Dimension != to.Dimension {
		return 0, fmt.Errorf("cannot convert %s to %s: different dimensions (%s vs %s)",
			describe(from), describe(to), from.Dimension, to.Dimension)
	}
	if to.Multiplier == 0 {
		return 0, fmt.Errorf("%s has a zero multiplier and cannot be converted to", describe(to))
	}

	if interval {
		// Offsets cancel across a difference, so only the scale applies.
		return value * from.Multiplier / to.Multiplier, nil
	}
	si := value*from.Multiplier + from.Offset
	return (si - to.Offset) / to.Multiplier, nil
}

// ConvertUnits is Convert by IRI, for callers holding what a form or a
// configuration gave them rather than a resolved definition.
func ConvertUnits(value float64, fromIRI, toIRI string, interval bool) (float64, error) {
	from, ok := LookupUnit(fromIRI)
	if !ok {
		return 0, fmt.Errorf("unknown unit %q", fromIRI)
	}
	to, ok := LookupUnit(toIRI)
	if !ok {
		return 0, fmt.Errorf("unknown unit %q", toIRI)
	}
	return Convert(value, from, to, interval)
}

// describe names a unit for an error message, preferring the symbol because it
// is what an operator recognises, and falling back to the IRI.
func describe(u UnitDef) string {
	if u.Symbol != "" {
		return u.Symbol
	}
	if u.IRI != "" {
		return u.IRI
	}
	return "an unnamed unit"
}

// NormaliseUnits converts a returned value into the unit the consumer asked for.
//
// This is the only place in the framework that can do it: the cervice states
// what the consumer wants and the payload states what the provider sent, and
// nothing else holds both. Doing it here also means no consuming system carries
// conversion code — the thermostat asks for a temperature and reads degrees
// Celsius whether the sensor speaks Celsius or Fahrenheit.
//
// It is deliberately conservative. A form that carries no unit, a cervice that
// states no preference, or a unit outside the QUDT table is left exactly as it
// was: a cloud that has not adopted QUDT identifiers keeps working unchanged.
// Once both sides are QUDT units, a conversion that cannot be made is an error
// rather than a raw number, because a control loop must never receive a value in
// a unit it did not expect.
func NormaliseUnits(cer *components.Cervice, f forms.Form) (forms.Form, error) {
	if cer == nil {
		return f, nil
	}
	want := firstDetail(cer.Details, "Unit")
	if want == "" {
		return f, nil // the consumer expressed no preference
	}

	bearer, ok := f.(forms.UnitBearer)
	if !ok {
		return f, nil // not a form whose value carries a unit
	}
	if err := AdoptUnit(bearer, want, isInterval(cer.Details)); err != nil {
		return f, err
	}
	return f, nil
}

// AdoptUnit converts a value into the unit the caller works in, in place.
//
// It is the same judgement NormaliseUnits applies to a reading coming back from
// a provider, exposed for the other direction: a setpoint arriving in a PUT is a
// number in someone else's unit, and writing it into a control loop without
// asking is how a Fahrenheit target becomes a Celsius one.
//
// Conservative on purpose. A unit outside the QUDT table is accepted when the
// two strings already agree, so a deployment that has not migrated keeps
// working; when they disagree and cannot be reconciled it is an error, because
// the alternative is a wrong number that looks entirely reasonable.
func AdoptUnit(bearer forms.UnitBearer, want string, interval bool) error {
	if want == "" {
		return nil
	}
	got := bearer.GetUnit()

	target, known := LookupUnit(want)
	source, sent := LookupUnit(got)
	if !known || !sent {
		if got != want {
			return fmt.Errorf("the value is in %q but %q was expected, and neither is a QUDT unit that can be converted", got, want)
		}
		return nil
	}

	converted, err := Convert(bearer.GetValue(), source, target, interval)
	if err != nil {
		return err
	}
	bearer.SetValue(converted)
	// Label it with the unit as the caller wrote it, not the canonical IRI. The
	// caller asked in a particular notation — configuration and Turtle use the
	// angle-bracketed form — and answering in another leaves the same unit
	// spelled two ways depending on whether a conversion happened.
	bearer.SetUnit(strings.TrimSpace(want))
	return nil
}

// isInterval reports whether a cervice consumes differences rather than points
// on a scale. It matters only where a unit has an offset, and there it decides
// whether 5 degrees of control error becomes 9 or 41.
func isInterval(details map[string][]string) bool {
	for _, v := range details["Measure"] {
		if strings.EqualFold(strings.TrimSpace(v), "interval") {
			return true
		}
	}
	return false
}

// firstDetail returns the first value recorded under a detail key.
func firstDetail(details map[string][]string, key string) string {
	if values := details[key]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

// local shortens an IRI to its last segment for an error message an operator has
// to read at three in the morning.
func local(iri string) string {
	if i := strings.LastIndexAny(iri, "/#"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}
