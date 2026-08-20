# forms

A form is what one system puts on the wire for another to read. This package
holds the schemas; nothing here does anything with them.

That makes it the smallest package in the framework and the one with the widest
blast radius. A change to `components` affects systems when they are rebuilt. A
change to a form affects systems that were built years apart and are talking to
each other right now.

---

## Reading this package

The Go source is the catalogue and always will be — a list of every form in a
README is a list that goes stale, which this package has already demonstrated:
the same package comment was pasted into eight files and had begun to drift by
the time anyone noticed. Each file now says what it holds in one line, and the
package comment lives once, in `forms_definition.go`.

What follows is the part that is *not* in the source: the conventions, and the
reasoning behind them.

| File | Holds |
|---|---|
| `service_forms.go` | Registration and query — what a provider tells the registrar |
| `servicequest_forms.go` | Discovery — what a consumer asks the orchestrator |
| `signal_forms.go` | A single value, its unit, and when it was taken |
| `authorization_forms.go` | What the orchestrator asks the authorizer, and the grants back |
| `certificate_forms.go` | The PEM a system receives from the CA |
| `lifecycle_forms.go` | What an activity costs, in money and in carbon |
| `host_forms.go` | What a machine reports about its own spare capacity |
| `file_forms.go`, `message_forms.go`, `system_forms.go` | Documents, text, and how a cloud describes its systems |

---

## The four rules

### 1. The version is inside the payload

Every form carries a `Version` string naming itself, and `NewForm()` sets it:

```go
func (f *HostLoad_v1) NewForm() Form {
    f.Version = "HostLoad_v1"
    return f
}
```

Not a header, not a content type, not a path — a field. A receiver that has the
bytes has everything it needs to know what they are, whether they arrived over
HTTP, out of a cache, from a file, or from a subscription that began before the
sender was upgraded.

### 2. A form must register itself

```go
func init() {
    FormTypeMap["HostLoad_v1"] = reflect.TypeOf(HostLoad_v1{})
}
```

`Unpack` resolves the version string through this map. **A form that omits the
`init` compiles, marshals, transmits and cannot be unpacked** — and the failure
appears at the receiver, as a form version nobody knows, not at the author's
desk. If you add a form, add the three lines.

### 3. Add a field; do not change one

Adding an optional field is safe: an older receiver ignores what it does not
know, and a newer one reading an older payload gets the zero value.

Changing a field's meaning, type or units is not safe, and there is no way to
make it safe. Make a new version — `_v2` — and leave the old one in place for as
long as anything speaks it. The suffix convention is `_v1`, with a trailing
letter (`SignalA_v1a`) where a family of related forms shares a version.

### 4. Distinguish "absent" from "zero"

The hardest bug in this package to see, and it has a shape:

```go
StallCPU *float64 `json:"stallCPU,omitempty"`
```

A pointer, because a pressure-stall figure of `0.0` means *nothing was delayed*
and a missing one means *this kernel does not measure* — opposite conclusions,
and a plain `float64` cannot hold the difference. A balancer reading `0` from a
host that never looked would move work onto it believing it idle.

Use a pointer whenever the zero value is a legitimate reading. Use a plain value
when it is not. `Load1` is a plain `float64` because a load average of zero is a
real and unambiguous answer.

---

## What does not belong in a form

**A decision.** `HostLoad_v1` reports headroom and does not recommend
anything. A `ShouldMigrate` field would have put the balancing policy into every
maitreD in the cloud instead of the one system that balances.

**What another source already knows.** The same form carries no list of running
processes: the knowledge graph already says which systems run where. Load comes
from the host, placement from the graph, mobility from the asset — three
sources, each saying only what it is the authority on. A form that repeats
another's facts is a form that can contradict it.

**Anything the sender cannot actually determine.** A field nobody can fill
honestly gets filled dishonestly.

---

## Testing a form

Round-trip it, and test the absent case explicitly:

```go
body, _ := json.Marshal(&sent)
var back HostLoad_v1
json.Unmarshal(body, &back)
```

One trap worth knowing, because it cost a confusing failure while `HostLoad_v1`
was being written: `json.Unmarshal` leaves fields **absent from the payload
untouched**, so reusing a destination struct across two unmarshals measures the
previous payload rather than the current one. Use a fresh variable per case.
