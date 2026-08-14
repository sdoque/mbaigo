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
//	si       = (value * from.Num + from.Add) / from.Den
//	value_to = (si * to.Den - to.Add) / to.Num
//
// The scale is a ratio rather than a decimal because the interesting one is not
// representable: Fahrenheit is five ninths of a kelvin, and 5.0/9.0 in binary
// floating point is not five ninths. Multiplying by it and dividing by it again
// leaves 100 C as 211.99999999999991 F, so a threshold at 212 never fires and
// every log line carries the noise. Kept as 5 and 9 and applied as one
// multiplication and one division, 100 C is 212.
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
	"math"
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

	// Num, Den and Add relate this unit to the SI coincident unit:
	// si = (value*Num + Add) / Den.
	//
	// A ratio rather than a decimal multiplier so that a scale like five ninths
	// stays exact until it is applied. Add is expressed in the same scaled
	// space, so degrees Fahrenheit carry 2298.35 over 9 rather than
	// 255.37222222222223: the repeating decimal is never written down.
	Num float64
	Den float64
	Add float64

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
	qudtUnit + "K":     {Symbol: "K", QuantityKind: KindTemperature, Dimension: DimTemperature, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "DEG_C": {Symbol: "°C", QuantityKind: KindTemperature, Dimension: DimTemperature, Num: 1, Den: 1, Add: 273.15, HasFactor: true},
	qudtUnit + "DEG_F": {Symbol: "°F", QuantityKind: KindTemperature, Dimension: DimTemperature, Num: 5, Den: 9, Add: 2298.35, HasFactor: true},
	qudtUnit + "DEG_R": {Symbol: "°R", QuantityKind: KindTemperature, Dimension: DimTemperature, Num: 5, Den: 9, Add: 0, HasFactor: true},

	// Pressure.
	qudtUnit + "PA":       {Symbol: "Pa", QuantityKind: KindPressure, Dimension: DimPressure, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "KiloPA":   {Symbol: "kPa", QuantityKind: KindPressure, Dimension: DimPressure, Num: 1000, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "BAR":      {Symbol: "bar", QuantityKind: KindPressure, Dimension: DimPressure, Num: 100000, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "MilliBAR": {Symbol: "mbar", QuantityKind: KindPressure, Dimension: DimPressure, Num: 100, Den: 1, Add: 0, HasFactor: true},

	// Time.
	qudtUnit + "SEC":      {Symbol: "s", QuantityKind: KindTime, Dimension: DimTime, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "MilliSEC": {Symbol: "ms", QuantityKind: KindTime, Dimension: DimTime, Num: 1, Den: 1000, Add: 0, HasFactor: true},
	qudtUnit + "MIN":      {Symbol: "min", QuantityKind: KindTime, Dimension: DimTime, Num: 60, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "HR":       {Symbol: "h", QuantityKind: KindTime, Dimension: DimTime, Num: 3600, Den: 1, Add: 0, HasFactor: true},

	// Length and mass, to show the pattern is general.
	qudtUnit + "M":  {Symbol: "m", QuantityKind: KindLength, Dimension: DimLength, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "FT": {Symbol: "ft", QuantityKind: KindLength, Dimension: DimLength, Num: 3048, Den: 10000, Add: 0, HasFactor: true},
	qudtUnit + "KG": {Symbol: "kg", QuantityKind: KindMass, Dimension: DimMass, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "LB": {Symbol: "lb", QuantityKind: KindMass, Dimension: DimMass, Num: 45359237, Den: 100000000, Add: 0, HasFactor: true},

	// Plane angle. Dimensionless in SI, exactly like a ratio — which is why the
	// quantity kind, not the dimension, is what keeps them apart.
	qudtUnit + "RAD": {Symbol: "rad", QuantityKind: KindAngle, Dimension: DimDimensionless, Num: 1, Den: 1, Add: 0, HasFactor: true},
	qudtUnit + "DEG": {Symbol: "°", QuantityKind: KindAngle, Dimension: DimDimensionless, Num: math.Pi, Den: 180, Add: 0, HasFactor: true},

	// Dimensionless ratios.
	qudtUnit + "PERCENT": {Symbol: "%", QuantityKind: KindRatio, Dimension: DimDimensionless, Num: 1, Den: 100, Add: 0, HasFactor: true},
	qudtUnit + "NUM":     {Symbol: "", QuantityKind: KindRatio, Dimension: DimDimensionless, Num: 1, Den: 1, Add: 0, HasFactor: true},

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

	if def, ok := units[trimmed]; ok {
		return def, true
	}
	if canonical, ok := legacyUnitNames[trimmed]; ok {
		// The ok matters: an alias pointing at a name the table does not hold
		// would otherwise resolve to a zero UnitDef reported as known — a unit
		// with an empty IRI, which Convert then refuses for a reason that names
		// neither the alias nor the typo behind it.
		if def, known := units[canonical]; known {
			return def, true
		}
	}
	return UnitDef{}, false
}

// legacyUnitNames are the plain unit names these systems used before QUDT.
//
// The framework promises in two places that a deployment naming "Celsius" keeps
// working. Without this it did not: every configuration written before the
// migration named a unit LookupUnit could not resolve, and a system that treats
// that as fatal stops booting on upgrade — which is not a promise kept.
//
// A migration aid, not a second vocabulary. Only names this repository actually
// wrote are here, and a new configuration should state the IRI: a name is
// ambiguous across domains in a way an IRI is not, which is the reason for
// adopting QUDT in the first place.
//
// Single letters are deliberately absent, apart from K. In SI, C is the coulomb,
// F the farad and m the metre — so aliasing them to degrees Celsius, degrees
// Fahrenheit and the metre reads as convenient only while the table holds no
// electrical units. The moment it grows toward the full vocabulary they become
// silently wrong conversions rather than lookup failures, which is the worse of
// the two. K is unambiguous: it is the kelvin in both readings.
var legacyUnitNames = map[string]string{
	"Celsius":     qudtUnit + "DEG_C",
	"celsius":     qudtUnit + "DEG_C",
	"°C":          qudtUnit + "DEG_C",
	"degC":        qudtUnit + "DEG_C",
	"Fahrenheit":  qudtUnit + "DEG_F",
	"fahrenheit":  qudtUnit + "DEG_F",
	"°F":          qudtUnit + "DEG_F",
	"degF":        qudtUnit + "DEG_F",
	"Kelvin":      qudtUnit + "K",
	"kelvin":      qudtUnit + "K",
	"K":           qudtUnit + "K",
	"Percent":     qudtUnit + "PERCENT",
	"percent":     qudtUnit + "PERCENT",
	"%":           qudtUnit + "PERCENT",
	"millisecond": qudtUnit + "MilliSEC",
	"ms":          qudtUnit + "MilliSEC",
	"second":      qudtUnit + "SEC",
	"s":           qudtUnit + "SEC",
	"minute":      qudtUnit + "MIN",
	"hour":        qudtUnit + "HR",
	"mbar":        qudtUnit + "MilliBAR",
	"bar":         qudtUnit + "BAR",
	"kPa":         qudtUnit + "KiloPA",
	"Pa":          qudtUnit + "PA",
	"dB":          qudtUnit + "DeciB",
	"degree":      qudtUnit + "DEG",
	"°":           qudtUnit + "DEG",
	"radian":      qudtUnit + "RAD",
	"meter":       qudtUnit + "M",
	"metre":       qudtUnit + "M",
	"foot":        qudtUnit + "FT",
	"ft":          qudtUnit + "FT",
	"kg":          qudtUnit + "KG",
	"lb":          qudtUnit + "LB",
}

// CanonicalUnit reports the QUDT IRI a legacy unit name stands for, so a system
// that refuses an unknown unit can name the replacement instead of only the
// problem.
func CanonicalUnit(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	canonical, ok := legacyUnitNames[trimmed]
	return canonical, ok
}

// Convert changes a value from one unit to another.
//
// interval says whether the value is a difference rather than a point on the
// scale. It matters only where the offset is non-zero, and there it matters a
// great deal: 5 °C of control error is 9 °F, not 41 °F. A controller's setpoint
// and its measurement are points; the error between them is an interval.
func Convert(value float64, from, to UnitDef, interval bool) (float64, error) {
	// Both resolved, and the same. Two *unresolved* units are also equal here —
	// a zero UnitDef has an empty IRI — and returning the value unchanged for
	// them was a silent identity conversion: a caller that dropped the ok from
	// two LookupUnits and mistyped both got its number back as though it had
	// been converted.
	if from.IRI == "" || to.IRI == "" {
		return 0, fmt.Errorf("cannot convert between units that were not resolved")
	}
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
	if to.Num == 0 || to.Den == 0 || from.Den == 0 {
		return 0, fmt.Errorf("%s has a degenerate scale and cannot be converted", describe(to))
	}

	if interval {
		// Offsets cancel across a difference, so only the scale applies.
		return tidy(value * from.Num * to.Den / (from.Den * to.Num)), nil
	}
	si := (value*from.Num + from.Add) / from.Den
	return tidy((si*to.Den - to.Add) / to.Num), nil
}

// significantDigits is where a converted value is rounded.
//
// Even with the scale kept as a ratio, a conversion through SI is two operations
// and the last bits do not always land: 0 C reaches 31.99999999999998 F. That
// residue is an artefact of the arithmetic, not a measurement — no sensor in
// this framework reports twelve significant figures — and leaving it in means a
// threshold at 32 does not fire and every log line carries the noise.
//
// Twelve is well inside float64's fifteen to seventeen, so nothing a caller
// could legitimately have measured is discarded.
const significantDigits = 12

// tidy rounds a converted value to significantDigits.
func tidy(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	// The scale factor has to be representable. For a value near the bottom of
	// float64's range the exponent exceeds 308, math.Pow returns +Inf, and
	// Round(v*Inf)/Inf is NaN — a value with no error beside it, which poisons
	// every comparison downstream. A number that small has no digits to trim
	// anyway.
	scale := math.Pow(10, float64(significantDigits)-math.Ceil(math.Log10(math.Abs(v))))
	if math.IsInf(scale, 0) || scale == 0 {
		return v
	}
	return math.Round(v*scale) / scale
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
// is what an operator recognizes, and falling back to the IRI.
func describe(u UnitDef) string {
	if u.Symbol != "" {
		return u.Symbol
	}
	if u.IRI != "" {
		return u.IRI
	}
	return "an unnamed unit"
}

// NormalizeUnits converts a returned value into the unit the consumer asked for.
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
func NormalizeUnits(cer *components.Cervice, f forms.Form) (forms.Form, error) {
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
		// No form alongside the error. Returning the unconverted one handed any
		// caller that logs and continues a valid-looking reading in the wrong
		// unit — the exact thing this function exists to prevent. Every caller
		// already returns on error, so nothing loses anything it was using.
		return nil, err
	}
	return f, nil
}

// AdoptUnit converts a value into the unit the caller works in, in place.
//
// It is the same judgment NormalizeUnits applies to a reading coming back from
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
