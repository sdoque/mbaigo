# components

The blocks an Arrowhead system is made of, and nothing else.

This package is meant to be readable as a block definition diagram: one file per
block, each block's own attributes declared with it, and no behavior that a
diagram would not show. `usecases` holds the verbs.

| File | Block | What it is |
|------|-------|------------|
| `system.go` | **System** | the participant in a local cloud: a husk and one or more unit assets |
| `husk.go` | **Husk** | the runtime shell — servers, ports, certificate, core systems it knows of |
| `uasset.go` | **UnitAsset** | the functional part: what the system actually does something with |
| `service.go` | **Service** / **Cervice** | what an asset offers, and what it consumes |
| `host.go` | **HostingDevice** | the machine a system runs on |

`Mission` is in `uasset.go` because it classifies an asset. `BoundPorts` is in
`husk.go` because only a husk has one — it records which protocols a system is
*actually* serving, as against the ones its configuration names, which is not
the same question and has caught this project out before.

## Two vocabularies an asset declares

Both are small, closed and reasoned about by other systems, so they are
constants rather than free strings — a typo should be a compile error.

**`Mission`** says what the asset is *for*: measurement, actuation, control,
state, aggregation, logging, transaction, core. The authorizer writes policy
against it, and one further thing follows from it — a service whose effective
mission is `core` is served without an access token, because the plane that
makes tokens possible cannot itself require one. A service may override its
asset's mission where the asset's is too coarse, and `EffectiveMission` is what
resolves the two.

**`Mobility`** says whether the asset could run on a different host: `fixed`,
`tethered` or `movable`. It is a `Details` key rather than a field, following
the same convention as a service's `Methods` — no migration, and it reaches the
knowledge graph as a predicate on its own.

The graph already knows which host each system runs on, so *what is where* is
not the question. What was missing is whether any of it could be somewhere
else, and nothing in the model said. Without it, the first thing anything
balancing load would propose is to relocate the temperature sensor.

Three values rather than two, because the middle case is the common one: most
assets in this project talk to a device over the network — Modbus TCP, OPC UA,
MQTT, a Zigbee bridge, a triple store — and can move to any host that can still
reach it. A `tethered` asset owes the reader a `TetheredTo` detail naming what
it must still reach; one that names nothing is read as `fixed`, because a move
nobody can verify is a move nobody should make.

## What does not belong here

A type earns its place by appearing in a diagram of the cloud. That rules out
two kinds of thing which drift in easily, because this is the package everything
imports:

**Mechanisms.** `CachedURL` — a mutex around a remembered string, so a core
system is not asked the same question twice — lived here for a while. It appears
in no diagram and no ontology, because it is not something a cloud contains. It
is now in `usecases`, owned by the behavior that does the asking.

**Behavior.** `GetRunningCoreSystemURL` is in `system.go` and is a use case
wearing a block's clothes: it polls registrars to find which one leads. Too much
depends on it to move today, and it is noted here rather than defended.

## The ontology

These blocks are the same things the [Arrowhead Framework Ontology](https://doi.org/10.1109/OJIES.2026.3693084) names —
`afo:System`, `afo:Husk`, `afo:UnitAsset`, `afo:Service`, `afo:Host` — which is
what lets a system describe itself at `/kgraph` without a translation layer. A
block added here that the ontology does not know about is a question to settle
with the ontology, not a term to invent in its namespace. See
`usecases/kgraphing.go`.
