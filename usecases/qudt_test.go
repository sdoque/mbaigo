package usecases

import (
	"math"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

const (
	degC    = "http://qudt.org/vocab/unit/DEG_C"
	degF    = "http://qudt.org/vocab/unit/DEG_F"
	kelvin  = "http://qudt.org/vocab/unit/K"
	kiloPA  = "http://qudt.org/vocab/unit/KiloPA"
	bar     = "http://qudt.org/vocab/unit/BAR"
	metre   = "http://qudt.org/vocab/unit/M"
	foot    = "http://qudt.org/vocab/unit/FT"
	pound   = "http://qudt.org/vocab/unit/LB"
	kilo    = "http://qudt.org/vocab/unit/KG"
	milliS  = "http://qudt.org/vocab/unit/MilliSEC"
	second  = "http://qudt.org/vocab/unit/SEC"
	percent = "http://qudt.org/vocab/unit/PERCENT"
	decibel = "http://qudt.org/vocab/unit/DeciB"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v; want %v", got, want)
	}
}

// The conversions an engineer can check by hand, which is the point: if these
// are right, the affine-through-SI arithmetic is right.
func TestConvertKnownValues(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		from, to string
		interval bool
		want     float64
	}{
		{"freezing in Fahrenheit", 0, degC, degF, false, 32},
		{"boiling in Fahrenheit", 100, degC, degF, false, 212},
		{"body heat back to Celsius", 98.6, degF, degC, false, 37},
		{"the scales meet at forty below", -40, degC, degF, false, -40},
		{"absolute zero", 0, kelvin, degC, false, -273.15},
		{"a hundred kilopascals is a bar", 100, kiloPA, bar, false, 1},
		{"a foot in metres", 1, foot, metre, false, 0.3048},
		{"a pound in kilograms", 1, pound, kilo, false, 0.45359237},
		{"milliseconds to seconds", 1500, milliS, second, false, 1.5},
		{"a percentage as a ratio", 50, percent, "http://qudt.org/vocab/unit/NUM", false, 0.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertUnits(tc.value, tc.from, tc.to, tc.interval)
			if err != nil {
				t.Fatalf("ConvertUnits: %v", err)
			}
			closeTo(t, got, tc.want)
		})
	}
}

// The distinction that makes temperature harder than everything else: a point on
// the scale carries the offset, a difference does not. Getting this wrong turns
// a 5 degree control error into a 41 degree one, and the controller acts on it.
func TestIntervalsDoNotCarryTheOffset(t *testing.T) {
	point, err := ConvertUnits(5, degC, degF, false)
	if err != nil {
		t.Fatalf("ConvertUnits: %v", err)
	}
	closeTo(t, point, 41)

	difference, err := ConvertUnits(5, degC, degF, true)
	if err != nil {
		t.Fatalf("ConvertUnits: %v", err)
	}
	closeTo(t, difference, 9)

	// Where the offset is zero the distinction is invisible, which is why it is
	// easy to miss until temperature turns up.
	for _, unit := range []struct{ from, to string }{{kiloPA, bar}, {foot, metre}, {milliS, second}} {
		asPoint, err := ConvertUnits(2, unit.from, unit.to, false)
		if err != nil {
			t.Fatalf("ConvertUnits: %v", err)
		}
		asInterval, err := ConvertUnits(2, unit.from, unit.to, true)
		if err != nil {
			t.Fatalf("ConvertUnits: %v", err)
		}
		closeTo(t, asPoint, asInterval)
	}
}

// A temperature is not a length. Both guards would catch this; the quantity kind
// is the finer one and reports first, which is the more useful message.
func TestConvertRefusesDifferentDimensions(t *testing.T) {
	_, err := ConvertUnits(20, degC, metre, false)
	if err == nil {
		t.Fatal("a temperature was converted to a length")
	}
	if !strings.Contains(err.Error(), "quantity kind") {
		t.Errorf("error %q does not say why", err)
	}
}

// Decibels and pH have no affine relationship to SI, so their multiplier and
// offset are meaningless. Refusing is the only honest answer; producing a number
// would be worse than failing.
func TestConvertRefusesNonLinearUnits(t *testing.T) {
	_, err := ConvertUnits(3, decibel, percent, false)
	if err == nil {
		t.Fatal("a logarithmic unit was converted as though it were linear")
	}
	if !strings.Contains(err.Error(), "linear") {
		t.Errorf("error %q does not say why", err)
	}
}

func TestConvertIsIdentityForTheSameUnit(t *testing.T) {
	for _, unit := range []string{degC, decibel, percent} {
		got, err := ConvertUnits(21.5, unit, unit, false)
		if err != nil {
			t.Fatalf("converting %s to itself: %v", unit, err)
		}
		closeTo(t, got, 21.5)
	}
}

func TestConvertRoundTrips(t *testing.T) {
	pairs := []struct{ a, b string }{{degC, degF}, {degC, kelvin}, {kiloPA, bar}, {foot, metre}, {pound, kilo}}
	for _, p := range pairs {
		there, err := ConvertUnits(21.5, p.a, p.b, false)
		if err != nil {
			t.Fatalf("%s to %s: %v", p.a, p.b, err)
		}
		back, err := ConvertUnits(there, p.b, p.a, false)
		if err != nil {
			t.Fatalf("%s back to %s: %v", p.b, p.a, err)
		}
		closeTo(t, back, 21.5)
	}
}

// Configuration and Turtle write the IRI in angle brackets; a form carries it
// bare. Both have to resolve, or the same value would have to be written twice
// in two shapes.
func TestLookupUnitAcceptsBothWrittenForms(t *testing.T) {
	bare, ok := LookupUnit(degC)
	if !ok {
		t.Fatal("the bare IRI did not resolve")
	}
	bracketed, ok := LookupUnit("<" + degC + ">")
	if !ok {
		t.Fatal("the bracketed IRI did not resolve")
	}
	spaced, ok := LookupUnit("  <" + degC + ">  ")
	if !ok {
		t.Fatal("a padded IRI did not resolve")
	}
	if bare.IRI != bracketed.IRI || bare.IRI != spaced.IRI {
		t.Error("the three written forms resolved to different units")
	}
	if bare.IRI != degC {
		t.Errorf("IRI = %q; want %q — a definition must know its own identity", bare.IRI, degC)
	}
}

// An unknown unit is refused rather than assumed. The table is a curated subset
// today, so this is the path a deployment hits when it uses something not yet
// listed — it must fail loudly rather than convert by 1.0.
func TestUnknownUnitsAreRefused(t *testing.T) {
	if _, ok := LookupUnit("http://qudt.org/vocab/unit/FURLONG"); ok {
		t.Error("an absent unit resolved")
	}
	if _, err := ConvertUnits(1, "Celsius", degF, false); err == nil {
		t.Error("a pre-QUDT unit name was accepted")
	}
	if _, err := ConvertUnits(1, degC, "", false); err == nil {
		t.Error("an empty target unit was accepted")
	}
}

// Every entry must carry its own IRI and a dimension, or the guards above are
// silently inoperative for it.
func TestUnitTableIsWellFormed(t *testing.T) {
	for iri, def := range units {
		if def.IRI != iri {
			t.Errorf("%s: IRI field is %q", iri, def.IRI)
		}
		if def.Dimension == "" {
			t.Errorf("%s: no dimension, so the dimension guard cannot apply", iri)
		}
		if def.HasFactor && def.Multiplier == 0 {
			t.Errorf("%s: claims an affine relationship but its multiplier is zero", iri)
		}
	}
}

// cervice is a consumer's stated need.
func cervice(details map[string][]string) *components.Cervice {
	return &components.Cervice{Definition: "temperature", Details: details}
}

func reading(value float64, unit string) *forms.SignalA_v1a {
	var f forms.SignalA_v1a
	f.NewForm()
	f.Value = value
	f.Unit = unit
	return &f
}

// The case this exists for: a Fahrenheit sensor serving a Celsius consumer,
// with neither knowing about the other.
func TestNormaliseUnitsConvertsToWhatTheConsumerAsked(t *testing.T) {
	c := cervice(map[string][]string{"Unit": {"<" + degC + ">"}})
	got, err := NormaliseUnits(c, reading(70, degF))
	if err != nil {
		t.Fatalf("NormaliseUnits: %v", err)
	}

	sig, ok := got.(*forms.SignalA_v1a)
	if !ok {
		t.Fatalf("got %T; want a signal form", got)
	}
	closeTo(t, sig.Value, 21.11111111111)
	// Labelled as the consumer wrote it: the value lies unless the label follows,
	// and answering in a different notation would leave one unit spelled two
	// ways depending on whether a conversion happened.
	if sig.Unit != "<"+degC+">" {
		t.Errorf("unit = %q; want the consumer's %q", sig.Unit, "<"+degC+">")
	}
}

// A control error is a difference, so the offset must not be applied to it.
func TestNormaliseUnitsHonoursIntervals(t *testing.T) {
	point := cervice(map[string][]string{"Unit": {degC}})
	interval := cervice(map[string][]string{"Unit": {degC}, "Measure": {"interval"}})

	asPoint, err := NormaliseUnits(point, reading(41, degF))
	if err != nil {
		t.Fatalf("NormaliseUnits: %v", err)
	}
	closeTo(t, asPoint.(*forms.SignalA_v1a).Value, 5)

	asInterval, err := NormaliseUnits(interval, reading(9, degF))
	if err != nil {
		t.Fatalf("NormaliseUnits: %v", err)
	}
	closeTo(t, asInterval.(*forms.SignalA_v1a).Value, 5)
}

// A cloud that has not adopted QUDT identifiers must keep working exactly as it
// did: matching on the unit string was all the assurance there ever was.
func TestNormaliseUnitsLeavesPreQudtDeploymentsAlone(t *testing.T) {
	c := cervice(map[string][]string{"Unit": {"Celsius"}})
	got, err := NormaliseUnits(c, reading(21.5, "Celsius"))
	if err != nil {
		t.Fatalf("a pre-QUDT pairing was refused: %v", err)
	}
	closeTo(t, got.(*forms.SignalA_v1a).Value, 21.5)
}

// But a mismatch it cannot reason about is reported rather than passed on. A
// control loop must never receive a number in a unit it did not expect.
func TestNormaliseUnitsRefusesWhatItCannotConvert(t *testing.T) {
	c := cervice(map[string][]string{"Unit": {"Celsius"}})
	if _, err := NormaliseUnits(c, reading(70, "Fahrenheit")); err == nil {
		t.Error("an unconvertible mismatch was passed through as though it were correct")
	}

	// A QUDT consumer meeting a provider that sent nothing at all.
	c = cervice(map[string][]string{"Unit": {degC}})
	if _, err := NormaliseUnits(c, reading(70, "")); err == nil {
		t.Error("a reading with no unit was accepted by a consumer that named one")
	}
}

// Forms without a unit, and consumers without a preference, are untouched.
func TestNormaliseUnitsIsInertWhereItHasNothingToSay(t *testing.T) {
	silent := cervice(map[string][]string{"Forms": {"SignalA_v1a"}})
	got, err := NormaliseUnits(silent, reading(21.5, degF))
	if err != nil {
		t.Fatalf("NormaliseUnits: %v", err)
	}
	closeTo(t, got.(*forms.SignalA_v1a).Value, 21.5)

	var binary forms.SignalB_v1a
	binary.NewForm()
	binary.Value = true
	if _, err := NormaliseUnits(cervice(map[string][]string{"Unit": {degC}}), &binary); err != nil {
		t.Errorf("a form carrying no unit was refused: %v", err)
	}

	if _, err := NormaliseUnits(nil, reading(21.5, degC)); err != nil {
		t.Errorf("a nil cervice was refused: %v", err)
	}
}

// The registrar compares strings, so a consumer that named its unit could never
// be paired with a provider using another. Naming a quantity kind is what lets
// them meet; the unit stays behind as the conversion target.
func TestQuestDetailsRelaxTheUnit(t *testing.T) {
	details := map[string][]string{
		"QuantityKind":       {"<http://qudt.org/vocab/quantitykind/ThermodynamicTemperature>"},
		"Unit":               {"<" + degC + ">"},
		"Measure":            {"interval"},
		"FunctionalLocation": {"Kitchen"},
		"Forms":              {"SignalA_v1a"},
	}

	q := questDetails(details)
	if _, present := q["Unit"]; present {
		t.Error("the unit was sent to the registrar, so a Fahrenheit provider could never match")
	}
	if _, present := q["Measure"]; present {
		t.Error("Measure was sent to the registrar; it says nothing about which provider suits")
	}
	if len(q["QuantityKind"]) != 1 || len(q["FunctionalLocation"]) != 1 || len(q["Forms"]) != 1 {
		t.Errorf("a matching criterion was lost: %v", q)
	}

	// Without a quantity kind there is nothing to relax to, so the unit stays
	// the only thing keeping the pairing honest.
	q = questDetails(map[string][]string{"Unit": {"Celsius"}, "Forms": {"SignalA_v1a"}})
	if len(q["Unit"]) != 1 {
		t.Error("the unit was dropped even though no quantity kind was named")
	}
}

const (
	degree = "http://qudt.org/vocab/unit/DEG"
	radian = "http://qudt.org/vocab/unit/RAD"
	number = "http://qudt.org/vocab/unit/NUM"
)

// The servo case. A plane angle and a plain ratio are both dimensionless, so the
// dimension guard permits this conversion and the arithmetic produces 28.6 —
// while the servo, which has 180 degrees of travel, is at 90. No unit conversion
// can reach 90; that needs the actuator's range. Refusing is the only correct
// answer, and only the quantity kind can tell these two apart.
func TestConvertRefusesPercentToDegrees(t *testing.T) {
	from, _ := LookupUnit(percent)
	to, _ := LookupUnit(degree)
	if from.Dimension != to.Dimension {
		t.Fatal("this test is pointless unless the two share a dimension")
	}

	_, err := ConvertUnits(50, percent, degree, false)
	if err == nil {
		t.Fatal("50% was converted to an angle; a servo would have been driven to the wrong position")
	}
	if !strings.Contains(err.Error(), "quantity kind") {
		t.Errorf("error %q does not name the reason", err)
	}
}

// But angles convert among themselves, and ratios among themselves.
func TestConvertWithinAQuantityKind(t *testing.T) {
	halfTurn, err := ConvertUnits(180, degree, radian, false)
	if err != nil {
		t.Fatalf("degrees to radians: %v", err)
	}
	closeTo(t, halfTurn, math.Pi)

	ratio, err := ConvertUnits(50, percent, number, false)
	if err != nil {
		t.Fatalf("percent to a plain ratio: %v", err)
	}
	closeTo(t, ratio, 0.5)
}

// With a well-formed table the quantity kind catches everything first, so the
// dimension guard only earns its place when a kind is missing — which a
// generated table makes possible. This is that case.
func TestDimensionGuardCoversUnitsWithoutAQuantityKind(t *testing.T) {
	anonymousHeat := UnitDef{IRI: "urn:test:heat", Symbol: "h", Dimension: DimTemperature, Multiplier: 1, HasFactor: true}
	anonymousSpan := UnitDef{IRI: "urn:test:span", Symbol: "s", Dimension: DimLength, Multiplier: 1, HasFactor: true}

	_, err := Convert(20, anonymousHeat, anonymousSpan, false)
	if err == nil {
		t.Fatal("units with no quantity kind converted across dimensions")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Errorf("error %q does not name the dimension mismatch", err)
	}
}

// Every convertible unit must say what it measures, or the guard is inoperative
// for it and the servo case comes back.
func TestEveryConvertibleUnitDeclaresItsQuantityKind(t *testing.T) {
	for iri, def := range units {
		if def.HasFactor && def.QuantityKind == "" {
			t.Errorf("%s is convertible but declares no quantity kind", iri)
		}
	}
}
