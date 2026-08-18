# Service Subscription

**Status:** Publisher implemented; consumer side not yet. See "What exists" below. The corresponding Go implementation will live in this
directory (`subscribe.go`); this document defines the contract.

**What exists today is a different thing, and this document does not describe
it.** The Service Registrar carries a *registry* subscription: `GET /syslist`
with `Accept: text/event-stream` returns a snapshot of the registered systems
followed by one event per registration and deregistration, so a consumer such as
the KGrapher learns when the cloud's shape changes. It has no thresholds, no
baselines and no per-service values, because it is not about a service's value —
there are no `/subscribe` endpoints on services, and looking for them is the
mistake this note exists to prevent.

Two properties of the registry stream are worth stating where a reader of this
document will look for them:

- **It heartbeats.** The registrar writes an SSE comment line every 20 seconds
  on an idle stream, and a subscriber treats silence past three intervals as a
  dead connection rather than a quiet cloud. The reasoning is the one given
  below for service values: a stream that is idle by design cannot otherwise be
  told from one that has stopped existing.
- **Its authorization is checked once, at connect.** The access token is
  verified when the stream opens and not again, so a subscription outlives the
  token that opened it and revoking a policy takes effect only when the
  connection next drops. This is the lenient of the two models named below, and
  it is a deliberate choice for a stream whose content is the registry's public
  shape rather than a plant's measurements. A service value subscription should
  take the strict model instead: keep the claims, and close the stream when they
  expire, letting the reconnection re-authorize.

## Purpose

A subscription mechanism for mbaigo services that lets one or more consumer
systems register interest in a provider's service and receive notifications
when the service's value changes meaningfully — *plus* a periodic heartbeat
so the consumer can distinguish "value is unchanged" from "publisher is
gone." The intended use case is the small, slow-changing measurement
streams that dominate operational technology (OT) plants: a `ds18b20` temperature sensor consumed by
a `Thermostat`, an `eThermostat`, and a `Collector` system simultaneously.

The mechanism is *subscription* in semantics (registration-based
notification with bounded staleness) and *streaming* in transport (a
persistent HTTP response carrying server-sent events). The two terms are
not interchangeable — operators and policy think in subscriptions, the wire
carries a stream.

## Concepts

- **Publisher** — the service-providing system whose service is marked
  `Subscribable`. Holds the current value, computes change deltas, emits
  events on heartbeat or change.
- **Subscriber** — a consumer system that opens a subscription stream and
  receives events.
- **Baseline** — the value last broadcast to subscribers. Used to compute
  whether a new sample should trigger a change event.
- **Threshold** — the minimum change in value (in the service's natural
  units) that triggers a change event. Per-service, configurable.
- **Heartbeat interval** — the maximum time between events on a
  subscription, regardless of value change. Per-service, configurable.

## Subscription semantics — the contract

The contract a publisher offers to every subscriber:

1. **On subscribe**, the publisher emits the current value as the first
   event. The subscriber never has to wait for a heartbeat to know the
   present state.
2. **A change event is emitted** when the difference between the current
   sample and the *baseline* exceeds the configured threshold, in the
   service's natural units. After the event is emitted, the baseline is
   updated to the current sample.
3. **A heartbeat event is emitted** when no event of any kind has been
   sent in the configured heartbeat interval. The heartbeat carries the
   current value (which may equal the baseline) so subscribers always see
   the *current* state on every event, regardless of cause.
4. **The heartbeat timer resets on every event**, change or heartbeat.
   This means *"30 s since last event"* is the operative rule, not
   *"every 30 s of wall clock."* Subscribers depend on this for liveness
   detection: missing two consecutive heartbeat windows is a strong
   signal the publisher is gone.
5. **Baseline is shared across subscribers**, not per-subscription. A new
   subscriber's first event is the on-subscribe current value; subsequent
   change events fire when the cloud-wide baseline shifts. This trades a
   small amount of per-subscriber semantic correctness for substantial
   simplification of publisher state.

## Service-side declaration

In `systemconfig.json`, a service marks itself subscribable via two new
optional fields on each `Service` entry:

```json
{
  "definition": "temperature",
  "subpath":    "temp",
  "subscribable": true,
  "heartbeat":   "30s",
  "threshold":   0.5,
  "details":     {"Unit": ["celsius"]},
  "registrationPeriod": 30
}
```

Defaults when absent:

| Field | Default | Notes |
|-------|---------|-------|
| `subscribable` | `false` | Backwards-compatible: existing services are unchanged. |
| `heartbeat`    | `"30s"` | Applied only if `subscribable: true`. |
| `threshold`    | `0`     | Zero means *"any change emits"*. |

If `subscribable` is `true` and the publisher framework adds the
subscription endpoint at `GET /system/asset/service/subscribe` automatically.

## Wire format — Server-Sent Events

A subscriber opens the subscription endpoint with a request in the
server-sent events (SSE) style:

```
GET /<system>/<asset>/<service>/subscribe HTTP/1.1
Accept: text/event-stream
```

The publisher responds with `Content-Type: text/event-stream` and writes
events as they fire:

```
event: value
data: {"value": 20.5, "timestamp": "2026-04-30T14:23:00Z", "cause": "subscribe"}

event: value
data: {"value": 21.0, "timestamp": "2026-04-30T14:24:12Z", "cause": "change"}

event: value
data: {"value": 21.0, "timestamp": "2026-04-30T14:24:42Z", "cause": "heartbeat"}
```

`cause` is one of `subscribe`, `change`, or `heartbeat`. Subscribers
should not branch on `cause` for normal operation — the value is
authoritative regardless — but it is useful for diagnostics, logging,
and audit.

The `event: value` line allows future extension to other event types
(e.g. `event: error`, `event: shutdown`) without breaking the data
channel.

## Subscriber lifecycle

1. **Subscribe**: open the SSE connection. The first event arrives
   within a single network round-trip; the publisher emits the current
   value before any timer fires.
2. **Receive events**: handle each `event: value` as it arrives. Update
   the local view of the value, reset any liveness timers.
3. **Detect publisher loss**: if the time since the last event exceeds
   `heartbeat × N` (a small multiplier, 2 or 3 to allow for jitter), the
   subscriber treats the publisher as offline and may alarm, fall back,
   or attempt to re-subscribe.
4. **Reconnect**: the SSE protocol's standard reconnect mechanism
   applies — subscriber retries on disconnect with exponential backoff.
   On reconnect the publisher emits a fresh on-subscribe event, restoring
   the contract.
5. **Unsubscribe**: close the HTTP connection. The publisher's writer
   fails on its next event emission; the goroutine cleans up.

## Composition with authorization

A subscription is a continuous form of `read`. The authorizer's existing
`read` action gates it; no new verb is required at the policy layer.

What does need deliberate handling is **token time to live (TTL) versus subscription
lifetime**. A subscription may run for hours; an authorization token is
valid for minutes. Two acceptable resolutions, to be settled in the
authorizer's specification (see `security/authorizer/POLICY.md`):

- **Strict**: when the token expires, the publisher disconnects the
  subscriber. The subscriber must re-authorize and reconnect.
  Revocation latency stays bounded by the token TTL.
- **Lenient**: the publisher checks the token only at the
  `/subscribe` request; the subscription continues until the
  connection drops, regardless of token expiry. Simpler, but
  revocation latency degrades to "until the subscriber reconnects."

The strict model is the more correct choice and is straightforward to
implement (the publisher tracks token expiry alongside the subscription
goroutine).

## Open questions

- **Per-subscriber threshold override.** A subscriber may want a finer
  threshold than the service's default (a `Collector` doing analytics
  may care about 0.1 °C; a `Thermostat` is fine with 0.5 °C). Could be
  expressed as a query parameter (`?threshold=0.1`). Defer to v2 unless
  a use case forces it earlier.
- **`subscribe` as its own action verb in POLICY.md.** Currently treated
  as `read`. If the authorizer ever needs to authorize *one-shot reads*
  differently from *continuous subscriptions* (e.g. policies that say
  "this consumer may poll but not subscribe"), a separate verb is the
  cleanest extension. Not needed today.
- **Multi-publisher failover.** If the same logical signal can be served
  by more than one provider (redundant sensors), how does a subscriber
  pick one and fail over if it dies? Belongs in the orchestrator's
  domain, not in this spec.

## What exists

The publisher half is implemented in `publishing.go`. A service declares itself
in `systemconfig.json` and its system hands each sample to `usecases.Publish`;
everything else — baseline, threshold, heartbeat, subscribers — is the
framework's.

Three decisions were taken while building it that this document did not settle:

- **The stream is the service's own path with `Accept: text/event-stream`**, not
  a `/subscribe` beneath it. The same resource in two representations, as the
  registry's system list already does. There is then no second path to declare,
  discover and authorize, and following a value is authorized as the read it
  already is — this cloud has been bitten once by a path the framework served
  without declaring.
- **Terms are negotiated, not deferred to v2.** A subscriber proposes a
  heartbeat and a threshold as query parameters, the publisher clamps them to
  what it can honour (`fastestHeartbeat`, `finestThreshold`), and the agreed
  terms are the first event on the stream. A control loop that believes it will
  hear about a change of 0.1 and will not is worse off than one that knows.
  Retrofitting a negotiation onto a deployed protocol is much harder than
  starting with one.
- **A slow subscriber is skipped, not disconnected and not waited for.** A
  value is a state rather than a sequence, so a subscriber that misses one
  learns the truth from the next event or the next heartbeat. This is why the
  registry stream needs resynchronisation and this does not: there, a dropped
  event is a change nobody will mention again.

Still to build: the consumer side. A cervice for a subscribable service should
follow it and keep the last value, so `GetState` answers from the cache without
a network call and every existing consumer is unchanged. A service that is not
subscribable is polled exactly as now, and a subscription that dies falls back to
polling rather than failing — a controller should degrade to slower data, not to
no data.

## Versioning

| Date | Change |
|------|--------|
| 2026-04-30 | Initial specification: SSE transport, shared baseline, heartbeat-resets-on-send, on-subscribe-emits-current. |
| 2026-08-18 | Publisher implemented. Stream moved onto the service's own path via Accept; terms negotiated rather than dictated; slow subscribers skipped rather than resynchronised. |
