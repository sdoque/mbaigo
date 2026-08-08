# mbaigo

A Go realisation of the Arrowhead framework's local-cloud architecture, opinionated
toward operational simplicity. mbaigo provides the small set of types and
behaviors every Arrowhead system needs — bootstrap, configuration, certificate
enrollment, registration, discovery, and request handling — so that a working
system is a few hundred lines of application code plus this module.

The design target: **a technician deploys a system of systems in half a day; an
engineer builds a new application system in one day.** Whether the framework
delivers on that target is for you to verify by reading the source and the
companion [systems repository](https://github.com/sdoque/systems).

## Status

Early-stage research and demonstration code, MIT-licensed.

- The runtime is functional and is used in active demonstrations (industrial
  pulp-mill condition monitoring, building climate control, vehicle telemetry).
- Not production-ready. APIs evolve between alpha tags.
- The security systems (`ca`, `maitreD`) are released; additional security
  tooling is staged in as it stabilises.

## What's in the module

Three packages, each with a narrow purpose.

### `components` — the shapes a system instantiates

The static structure of an mbaigo system.

- **`System`** holds the runtime context, the unit-asset graph, and the husk.
- **`Husk`** is the framework boilerplate: system name, host metadata, protocol
  ports, certificate state, the CA-ready channel, the registrar channel. Every
  system fills one in.
- **`UnitAsset`** is your application — a sensor, a control loop, an actuator,
  a database adapter — bundled with the services it provides and the services
  it consumes.
- **`Service`** describes a service this system *provides*; **`Cervice`** (the
  deliberate spelling difference) describes a service this system *consumes*.
  The asymmetry is load-bearing: the producer/consumer roles drive registration,
  discovery, and the per-asset behavioral model the framework emits.

### `usecases` — the behavior the framework gives you for free

You import the use cases you need; everything else stays out.

- `Configure` reads `systemconfig.json` (generating one from a template if it's
  missing) and returns the unit-asset configurations.
- `RequestCertificate` enrolls with the CA non-blocking; the HTTPS server binds
  when the cert lands, the HTTP control plane is up immediately.
- `RegisterServices` advertises the system's services to the Service Registrar
  on the configured registration period.
- `SetoutServers` binds the HTTP and HTTPS servers and wires the per-system
  request dispatcher.
- `Search4Services`, `GetState`, `SetState` discover and invoke services on
  other systems through the Orchestrator.
- `SModeling`, `KGraphing`, `SysHateoas` emit the per-system meta-views
  described below.

### `forms` — the wire vocabulary

Versioned forms for what systems send each other.

- `SignalA_v1a` — a numeric value with unit and timestamp.
- `SignalB_v1a` — a boolean value with timestamp.
- `ServicePoint_v1`, `SystemRecordList_v1` — service-registry payloads.
- `Certificate` — certificate-distribution payload.

Each form is a struct plus marshal/unmarshal helpers for JSON and XML.
Versioning is in the type name so consumers pinned to `v1a` are not silently
broken when `v2` arrives.

## A minimum viable system

Every mbaigo system follows the same shape. A stripped version of
[`drafter`](https://github.com/sdoque/systems/tree/main/drafter) — the
template system maintained for newcomers:

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sys := components.NewSystem("drafter", ctx)
    usecases.WatchShutdown(&sys, cancel)

    sys.Husk = &components.Husk{
        Description: "skeleton system for learning the mbaigo architecture",
        Host:        components.NewDevice(),
        ProtoPort:   map[string]int{"http": 20192, "https": 0},
        // … name distinguisher, registrar channel, etc.
    }

    sys.UAssets[initTemplate().GetName()] = initTemplate()
    rawResources, _ := usecases.Configure(&sys)
    // … instantiate the configured unit assets …

    usecases.RequestCertificate(&sys)
    usecases.RegisterServices(&sys)
    go usecases.SetoutServers(&sys)

    <-sys.Ctx.Done()
}
```

Application logic lives in a sibling `thing.go` that defines the unit asset's
traits and serves its services. The framework owns the boilerplate; the
application owns the behavior.

## What's distinctive

- **Universal mTLS, attested at startup.** Every system enrolls with the CA via
  executable-hash attestation performed by a host-local sentinel (`maitreD`).
  The CA signs a CSR only after `maitreD` confirms the requesting process's
  binary hash is on a cloud-wide whitelist. Application keys are memory-only —
  identity is the running binary, not a long-lived on-disk credential.
- **Self-describing systems.** Every system serves three meta-views at runtime:
  a browsable HATEOAS index (`/doc`), a SysML v2 BDD/IBD fragment (`/smodel`),
  and an OWL/RDF knowledge-graph block (`/kgraph`). The `modeler` and
  `kgrapher` systems aggregate these per-system fragments into one cloud-wide
  model.
- **Operational simplicity as a design target.** Adding a system is one binary,
  one auto-generated `systemconfig.json`, and `go run .`. Adding a new asset
  type is one directory, one `thing.go`, and the existing Husk pattern.

## Try it

The runnable examples live in the companion systems repository:

> https://github.com/sdoque/systems

Quickest path:

```sh
git clone https://github.com/sdoque/systems
cd systems/orchestrator && go run .   # core: matches consumers to providers
# in another terminal:
cd systems/esr && go run .            # core: ephemeral service registrar
# in another terminal:
cd systems/drafter && go run .        # example application system
```

Browse `http://localhost:20192/drafter/doc` to see the running system describe
itself.

## Background

The design philosophy — how the Husk, the Unit Asset, the cervice/service
asymmetry, and the channel-tray concurrency pattern fit together — is described
in:

> van Deventer, J. A. (2025). *A Model Based Implementation of an IoT
> Framework*. Zenodo. https://doi.org/10.5281/zenodo.18504110

## Contributing

- Code must pass `gofmt -l .` (CI enforces).
- Tests live next to the code they cover and run under `go test ./...`.
- Local-build artefacts use the `_amac` suffix (caught by the repo's
  `.gitignore`) so binaries stay out of commits.

## License

MIT. See [LICENSE](LICENSE).
