# usecases

What a system *does*, as opposed to what it *is*.

`components` holds the nouns — a System, its Husk, its unit assets, the services
it provides and the cervices it consumes. This package holds the verbs: reading a
configuration file, enrolling with a certificate authority, registering services,
finding a provider, asking it for a value, answering somebody who asks, and
describing itself to a knowledge graph.

Almost none of it is called by a system directly. A system's `main` calls five
functions, in this order, and the rest of the package is reached through them.

## The five calls every system makes

```go
usecases.WatchShutdown(&sys, cancel)   // Ctrl+C cancels the context, once
rawResources, err := usecases.Configure(&sys)
                                       // read systemconfig.json, or write one and stop
//   ... the system builds its unit assets from rawResources ...
usecases.RequestCertificate(&sys)      // enroll with the CA, in the background
usecases.RegisterServices(&sys)        // tell the registrar, and keep telling it
go usecases.SetoutServers(&sys)        // bind HTTP now, HTTPS when the certificate lands
```

Three of those return before the work is finished. Enrollment, registration and
the HTTPS server all continue in goroutines, because a cloud starts in no
particular order: a system must serve plain HTTP while it waits for a certificate
authority that may not be running yet, and register with a registrar that may
appear later.

## Configuration — `configuration.go`

`Configure` reads `systemconfig.json` and hands back one raw entry per unit asset
for the system to instantiate.

If there is no file, it writes one from the system's template and returns
`ErrNewConfig`, which every system treats as fatal — so the first run of a system
writes a file and stops, and the second run uses it.

**A configuration file is never merged into.** Once it exists, it is the
operator's. That is what makes this package fill in a service's missing fields
from the template: a file written before a field existed says nothing about it,
and silence is indistinguishable from a deliberate zero. A *capability* — whether
a service can be followed, which depends on what the code does — is taken from
the template; a *setting* is taken from the file wherever the file says anything
at all. A service the build serves and the file has never heard of is added,
because a service absent from the configuration is one the cloud cannot
discover, authorize or reason about.

The same reasoning fills a missing mission from the template, and refuses to
guess when a system has several and the asset has been renamed.

## Identity — `authentication.go`, `identity.go`, `posture.go`

Every system enrolls, whether or not it serves HTTPS: the certificate is what
lets it *call* other systems over mTLS, not only be called.

Keys are generated in memory on every start and never written to disk. A
system's identity in the cloud is therefore the running binary — attested by the
maitreD against a hash the certificate authority holds — rather than a file that
outlives it. The exception is the certificate authority itself, whose root key
must survive a restart.

`Posture` reports what a system observes about its own security: whether it names
a certificate authority, whether it holds a certificate, whether it can verify
peers, whether it checks access tokens, whether it still answers plaintext. Each
is a fact rather than a setting, and a cloud where they disagree — a system that
means to authorize and currently cannot — is exactly the state worth being able
to query for.

## Authorization — `authorization.go`, `token.go`, `missions.go`

A consumer never presents credentials it chose. The Orchestrator asks the
Authorizer, and hands back a *service point*: where to go and a token to present
when it gets there. The provider recomputes the action from the HTTP method it
receives, so a token minted for a read is refused on a write.

A token is bound to the connection's common name, so it cannot be replayed by
another system that obtains it.

`missions.go` refuses to start a system whose assets do not classify themselves.
A mission is the axis policy is written along; an optional one is one that gets
left blank, and a permissive default is a hole.

## Discovery and consumption — `service_discovery.go`, `consumption.go`

`GetState` and `SetState` are what a control loop calls. Between them and the
network sit discovery, tokens, unit conversion and — since a service may be
followed rather than asked — a cached value.

Discovery is per *action*: a cervice used for both a GET and a PUT is discovered
twice and holds one token for each. Pruning is per action too, so a write
discovery does not delete a provider that was only ever readable.

A reading arrives in the provider's unit and is converted into the one the
consumer asked for (`qudt.go`). An unknown unit on either side is refused rather
than passed through: a number relabelled with a unit nobody could convert is a
wrong number that looks entirely reasonable, and these drive heaters and valves.

## Subscription — `publishing.go`

A service may declare itself followable. A consumer then opens a stream instead
of asking repeatedly, and `GetState` answers from what the stream last delivered.

**No consuming system changes to gain this.** The control loop calls `GetState`
on its own clock exactly as before; what changes is that the answer no longer
costs a request. A service that is not followable is polled as it always was, and
a subscription that lapses falls back to polling — slower data, never no data.

The terms are negotiated: a subscriber proposes a heartbeat and a threshold, the
publisher clamps them to what it can honour, and the agreed terms are the first
event on the stream. See `SUBSCRIBE.md`.

## Provision — `provision.go`, `servers_handlers.go`

The inbound half. `SetoutServers` binds the ports and routes a request to the
unit asset and service its path names, after `permitted` has decided whether the
caller may.

A stream is the same resource in another representation: `Accept:
text/event-stream` on a service's own path, rather than a `/subscribe` beside it.
A path the framework answers on without declaring is invisible to the authorizer,
to the Orchestrator and to the knowledge graph — which has cost this cloud more
than once.

## Description — `kgraphing.go`, `smodeling.go`, `docs.go`

Each system describes itself in Turtle at `/kgraph` and in SysML v2 at `/smodel`,
and renders a browsable page at `/doc`. No system knows the shape of the cloud
around it; the KGrapher and the painter are what put the separate accounts side
by side.

Terms are written in the Arrowhead Framework Ontology's namespace only where
that ontology defines them; everything this project mints goes to the local
cloud's own namespace. `afoDefined` in `kgraphing.go` is the list, and the list
is also the agenda for what to propose upstream.

## Odds and ends

| File | What it is for |
|------|----------------|
| `utilities.go` | the framework's HTTP client, form packing, name-case helpers |
| `registry_reading.go` | reading the registrar's list of systems, in one place |
| `cost.go`, `footprint.go` | what a service call costs, in money and in carbon |
| `shutdown.go` | one signal handler, so Ctrl+C interrupts a blocking startup |

## Reading order

To follow one request end to end, read in this order: `configuration.go` for
what a system knows about itself, `registration.go` for how it says so,
`service_discovery.go` for how a consumer finds it, `consumption.go` for the
call, and `servers_handlers.go` for the other end of it.
