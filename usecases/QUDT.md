# QUDT unit handling — design note

QUDT is the *Quantities, Units, Dimensions and Data Types* vocabulary: a public
ontology that gives every unit a stable identifier, states what it measures, and
records how it relates to the SI unit of the same dimension.

Status: **largely implemented.** The conversion machinery, the discovery
relaxation and the consumption hook are in `usecases/qudt.go`; `ds18b20`,
`ds18b20F`, `thermostat`, `parallax`, `leveler`, `ethermostat`, `revolutionary`,
`meteorologue` and `flattener` declare QUDT identifiers. What is not done is
listed at the end.

Two decisions differ from the plan below, and the plan is left as written so the
change of mind is visible:

- **No new signal form.** Section 9 argued for `SignalA_v1b` because putting an
  Internationalized Resource Identifier (IRI) in `Unit` changes the meaning of a
  field. In the end the IRI went into
  `SignalA_v1a.Unit` and normalization was made inert wherever either side is not
  a QUDT unit, so a pre-QUDT deployment is untouched and no form had to be
  versioned. The compatibility the version bump was meant to buy is bought by the
  conversion being conditional.
- **Discovery relaxes at the consumer, not the orchestrator.** Section 2 put the
  quantity-kind widening in the orchestrator. It turned out to belong in
  `Search4Services`, which builds the quest from the cervice's own details: a
  consumer that names a quantity kind is asking for a temperature, so the unit is
  dropped there and the orchestrator needed no change at all.

One thing the plan did not anticipate: the dimension guard is not enough. A plane
angle and a plain ratio are both dimensionless, so nothing in the arithmetic
objects to converting 50% into 28.6 degrees — while a servo with 180 degrees of
travel is at 90. `Convert` refuses across quantity kinds for that reason, and
`parallax` states its range so a consumer can do what unit conversion cannot.

Motivating question: if the `ds18b20` system provides its temperature service in
°F while the `thermostat` system consumes °C, where does that get resolved — in
the Orchestrator, or in `GetState()`?

Answer: **both, doing different jobs.** The Orchestrator decides *whether* a °F
provider is an acceptable substitute; only `GetState()` ever sees a number and
can convert it.

---

## 1. What the code does today

The unit appears in three unconnected places:

| Where | ds18b20 | thermostat |
|---|---|---|
| Registered detail | `thing.go:67` — `{"Unit": {"Celsius"}}` | `thing.go:139` — cervice wants `{"Unit": {"Celsius"}}` |
| Payload field | ~~`f.Unit = "Celsius"` hardcoded~~ — now stamped from the configured unit | — |
| Consumption | — | `thing.go:252-259` asserts `*SignalA_v1a`, reads `.Value`, **never reads `.Unit`** |

The filter in the ESR, the ephemeral service registrar
(`systems/esr/thing.go:251-280`), is a plain string-set membership
test. So `unit:DEG_F` vs `unit:DEG_C` yields *no match at all*: the thermostat
falls to `updateValvePosition(50)` and logs "unable to obtain a temperature
reading" forever. Not silent corruption — but not a resolution either.

The real hazard is the near miss: if a °F provider ever *did* get matched (a
mistyped detail, a provider registering both units), `thermostat/thing.go:258`
feeds Fahrenheit numbers straight into the P controller with no check.

## 2. Layering

**ESR — leave unchanged.** It compares strings and nothing more, deliberately. Teaching it QUDT
couples the service registry to a physics library and to lookups during a query.

**Orchestrator — compatibility, not conversion.** It never sees a payload; it
returns a `ServicePoint_v1` URL (`orchestrator/thing.go:239-248`). What it *can*
do is relax the quest: the consumer asks for `unit:DEG_C`, the Orchestrator
resolves the quantity kind and queries ESR on
`QuantityKind: quantitykind:ThermodynamicTemperature` instead. Providers register
**both** keys, so ESR's exact-match filter stays untouched and compatibility
becomes a coarser string match. The provider's actual unit already rides back to
the consumer through existing machinery:

    orchestrator/thing.go:244  →  service_discovery.go:150  →  cer.Nodes[node].Details

**`GetState()` — the conversion.** It is the only place holding both sides:
`cer.Details["Unit"]` (what the consumer declared it wants) and `f.Unit` from the
unpacked payload (what it got). One insertion point, right after `Unpack` at
`consumption.go:85`. Gate it on a small interface so the core stays generic:

```go
type UnitBearer interface {
    GetUnit() string; SetUnit(string)
    GetValue() float64; SetValue(float64)
}
```

Normalize only when the returned form implements it. When the conversion is
unknown, **return an error** — never pass the raw value through. A control loop
must not receive a number in an unexpected unit.

**Rejected alternatives.** Converting in each consumer's `thing.go` repeats the
same code across 30+ systems, and `thermostat/thing.go:252` already demonstrates
that the author forgets. Converting in the *provider* (serve °F on request) makes
the GET stateful per consumer and breaks the shared cached reading.

## 3. The general pattern (length, mass, pressure, … not just temperature)

QUDT gives every unit a `qudt:conversionMultiplier` and `qudt:conversionOffset`
relative to the coincident unit of its dimension in the International System of
Units (SI). Conversion is therefore two
affine steps *through SI*, never a pairwise table — **N entries, not N²**:

    si       = value_from * from.Multiplier + from.Offset
    value_to = (si - to.Offset) / to.Multiplier

| Unit | Multiplier | Offset |
|---|---|---|
| `unit:DEG_C` | 1.0 | 273.15 |
| `unit:DEG_F` | 0.555555… | 255.372222… |
| `unit:FT` | 0.3048 | 0 |
| `unit:LB` | 0.45359237 | 0 |
| `unit:KiloPA` | 1000 | 0 |
| `unit:BAR` | 100000 | 0 |
| `unit:PERCENT` | 0.01 | 0 |

(Figures from memory — verify against the pinned QUDT release.)

The offset is zero for essentially everything except temperature and gauge
pressure. Length and mass collapse to pure scaling, which is why they never raise
the question of points against intervals below. The pattern extends to every
dimension without additional cases;
temperature merely exercises the one term the others leave at zero. Composite
units come along too — `unit:MilliM-PER-HR` → `unit:M-PER-SEC` is the same two
numbers, which matters given `rdfObject`'s doc comment already cites `mm/h` and
`W/m²`.

```go
type UnitDef struct {
    IRI        string  // http://qudt.org/vocab/unit/DEG_F
    Dimension  string  // qudt:hasDimensionVector, e.g. A0E0L0I0M0H1T0D0
    Multiplier float64
    Offset     float64
    HasFactor  bool    // false for logarithmic/ordinal units — refuse to convert
}

func Convert(v float64, from, to *UnitDef, interval bool) (float64, error) {
    if from.IRI == to.IRI { return v, nil }
    if !from.HasFactor || !to.HasFactor {
        return 0, fmt.Errorf("no affine conversion for %s → %s", from.IRI, to.IRI)
    }
    if from.Dimension != to.Dimension {
        return 0, fmt.Errorf("dimension mismatch: %s vs %s", from.Dimension, to.Dimension)
    }
    if interval {                                   // a delta: offsets cancel
        return v * from.Multiplier / to.Multiplier, nil
    }
    return (v*from.Multiplier + from.Offset) / to.Multiplier - to.Offset/to.Multiplier, nil
}
```

## 4. Two keys, two layers

- **`qudt:hasDimensionVector`** makes a conversion *arithmetically* legal. It is
  the guard inside `Convert` and comes from the unit table — nobody configures it.
- **`qudt:hasQuantityKind`** makes the substitution *semantically* legal, and is
  what the Orchestrator matches on. Required because dimension alone is not
  sufficient: torque and energy are both `L2 M1 T-2`, so a dimension-only check
  will happily hand a torque sensor to something asking for energy. Likewise
  frequency against becquerel.

## 5. The one thing QUDT will not tell you

Whether a *particular reading* is an absolute point or an interval.
`quantitykind:ThermodynamicTemperature` describes the quantity; it does not know
that `getError()` returns a difference. Applying the offset is correct for the
setpoint and the measurement; it is **wrong** for `thermostat/thing.go:210-217` —
5 °C of error is 9 °F, not 41 °F. Both are labeled `"Celsius"` today.

This flag is per-*service*, not per-unit-asset: the thermostat's `temperature`
and `setpoint` are absolute while `error` is a delta on the same unit. Cheapest
correct encoding is `Details["Measure"]: {"interval"}` on the services that emit
deltas, defaulting to absolute when absent. In the whole codebase that is exactly
one service. Everything else is either absolute or has a zero offset where the
distinction is invisible.

## 6. Where the table comes from

Generate offline into a `qudt_units_gen.go` — a small program over QUDT's
`VOCAB_QUDT-UNITS` Turtle emitting the four fields per unit. Take the **whole**
vocabulary (~1900 units, a few hundred KB of Go), not a curated subset: a subset
means an unfamiliar unit in someone's config fails at runtime, whereas the full
table makes "unknown unit" impossible, and the data is static so it costs nothing
to carry. Regenerate only when bumping the QUDT release.

**No GraphDB, no KGrapher, no network.** A control loop with a 2-second period
must not depend on the KGrapher being deployed to interpret a temperature.

## 7. Configuration shape

Full IRIs go in each unit asset's configuration, angle brackets included:

```json
"details": {
  "Unit":         ["<http://qudt.org/vocab/unit/DEG_F>"],
  "QuantityKind": ["<http://qudt.org/vocab/quantitykind/ThermodynamicTemperature>"]
}
```

`rdfObject` passes `<...>` straight through (`kgraphing.go:76-78`), so the
knowledge graph gets real IRIs with **zero code change** — only the predicate
mapping and the prefix lines are needed. The conversion lookup strips the
brackets; one `strings.Trim`.

## 8. Knowledge-graph side

- Add `@prefix qudt:`, `unit:`, `quantitykind:` to `prefixes()` (`kgraphing.go:57`).
- Give the `Unit` detail key a real predicate — `qudt:hasUnit`, not
  `alc:hasUnit` — the way `FunctionalLocation` was handled at
  `kgraphing.go:265-272`. That mapping is the existing extension point.
- Without the angle brackets, `rdfObject` emits `qudt:unit/DEG_C` as a **string
  literal** (the slash fails `isValidPNLocal`), which is semantically inert.

## 9. Wire format

Putting an IRI in `SignalA_v1a.Unit` changes the meaning of a field in a form
registered as `"SignalA_v1.0"` (`forms/signal_forms.go:35-57`). Every deployed
system reading `"Celsius"` breaks. Given `FormTypeMap` versioning already exists,
`SignalA_v1b` with a QUDT-IRI `Unit` is the honest path.

## 10. Current unit inventory

Across all systems today:

    9× Celsius   7× kPa   6× Percent   3× millisecond   3× spec.unit
    1× bar       1× SEK/kWh            1× {"Percent", "Rotational"}

- `{"Percent", "Rotational"}` already crams a unit and a qualifier into one key.
  These separate cleanly: `Unit: unit:PERCENT` plus a `QuantityKind`.
- `SEK/kWh` does not fit and should not be forced to. QUDT covers ISO 4217
  currencies but not currency-per-energy composites, and exchange rates are not
  static multipliers. `Service.CUnit` (`components/service.go:37`) already exists
  — keep cost there, out of the QUDT path entirely.
- The three `spec.unit` sites (`weatherman/thing.go:172`,
  `beekeeper/thing.go:308`, `meteorologue/thing.go:453`) are the easiest
  migration: the unit already comes from a spec table, so the table changes, not
  the code.

## 11. Caveats to settle before building

- **Logarithmic and ordinal units** (dB, pH, Richter) have no meaningful
  multiplier. QUDT flags them; `HasFactor` must refuse rather than silently
  produce nonsense.
- **Gauge against absolute pressure** is the other real-world offset trap, and QUDT
  does *not* model it — `unit:KiloPA` says nothing about the reference. With 7
  kPa services and 1 bar service in the cloud, check this before assuming
  pressure is pure scaling.
- **Counts and dimensionless ratios** all share the empty dimension vector, so
  the dimension check permits converting "percent humidity" into "percent valve
  opening". Only the quantity-kind match at the Orchestrator prevents that —
  which is the argument for treating it as mandatory, not optional.

## 12. What remains

- **The generated table.** `units` in `usecases/qudt.go` is a curated subset
  covering what these systems register. Generating the whole QUDT vocabulary from
  a pinned release would make an unfamiliar unit impossible rather than merely
  refused, and would replace the hand-written dimension vectors and quantity
  kinds with the release's own.
- **The remaining systems.** `emulator`, `modboss` and the other weather systems
  still declare pre-QUDT unit strings. They work — normalization is inert for
  them — but they cannot be paired across units.
- **The units QUDT does not cover here.** `meteorologue` still states `ppm`,
  `km/h`, `mm/h` and `mm` as plain symbols, because `units` in `qudt.go` has no
  entry for them. An IRI that resolves to nothing would read as a promise the
  conversion cannot keep, so the symbol stays until the table grows.
- **Gauge versus absolute pressure**, which QUDT does not model, and which the
  seven kPa services make a live question.
- **The knowledge-graph side**, section 8: the `qudt:` prefixes and a real
  predicate for the unit. Note that `kgrapher/files/alc-ontology-local.ttl`
  already declares `qudt:` and `qudt-unit:`.

## 13. The original touch list

- `mbaigo/usecases/qudt_units_gen.go` — generated table (new)
- `mbaigo/usecases/qudt.go` — `UnitDef`, `Convert`, IRI lookup (new)
- `mbaigo/usecases/consumption.go:85` — normalize after `Unpack`
- `mbaigo/forms/signal_forms.go` — `SignalA_v1b`
- `mbaigo/usecases/kgraphing.go:57,265-272` — prefixes and the `Unit` predicate
- `systems/orchestrator/thing.go` — relax quest to `QuantityKind`
- each system's `systemconfig.json` — IRIs in `details`
- `systems/thermostat/thing.go:210-217` — mark `error` as an interval
