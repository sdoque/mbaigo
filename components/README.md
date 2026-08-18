# components

The blocks an Arrowhead system is made of, and nothing else.

This package is meant to be readable as a block definition diagram: one file per
block, each block's own attributes declared with it, and no behaviour that a
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

## What does not belong here

A type earns its place by appearing in a diagram of the cloud. That rules out
two kinds of thing which drift in easily, because this is the package everything
imports:

**Mechanisms.** `CachedURL` — a mutex around a remembered string, so a core
system is not asked the same question twice — lived here for a while. It appears
in no diagram and no ontology, because it is not something a cloud contains. It
is now in `usecases`, owned by the behaviour that does the asking.

**Behaviour.** `GetRunningCoreSystemURL` is in `system.go` and is a use case
wearing a block's clothes: it polls registrars to find which one leads. Too much
depends on it to move today, and it is noted here rather than defended.

## The ontology

These blocks are the same things the [Arrowhead Framework Ontology](https://doi.org/10.1109/OJIES.2026.3693084) names —
`afo:System`, `afo:Husk`, `afo:UnitAsset`, `afo:Service`, `afo:Host` — which is
what lets a system describe itself at `/kgraph` without a translation layer. A
block added here that the ontology does not know about is a question to settle
with the ontology, not a term to invent in its namespace. See
`usecases/kgraphing.go`.
