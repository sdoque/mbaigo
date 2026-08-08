# The security posture a system reports

A local cloud runs at whatever level of protection its operator has deployed, and
every level below the last one is a legitimate deployment. The framework never
withholds function to enforce security: it states what it is doing instead, so an
operator can see the difference between a cloud that is protected and one that
was meant to be.

**For how to deploy each level, which systems to run and what each one adds, see
[`systems/SECURITY.md`](https://github.com/sdoque/systems/blob/main/SECURITY.md).**
This note is the framework contract: what a system reports, and what each report
means.

## The levels

`Posture(sys)` returns a `SecurityPosture` whose `Level` is one of:

| Level | Meaning |
|---|---|
| `open` | No certificate authority is configured. Nothing is identified, nothing is authorised, everything is in the clear. |
| `enrolling` | A CA is configured; this system has no certificate yet. Its HTTPS endpoint is not bound. |
| `identified` | This system holds a certificate from the cloud's CA. Callers over TLS are named and verified. What they may do is not restricted. |
| `authorized` | As `identified`, and every incoming request must also carry a token minted for that caller, that service and that action. |

## The properties

Every property is an observation the system makes about itself, not a setting it
was given. That is what makes them worth querying:

| Property | Reports |
|---|---|
| `namesCertificateAuthority` | A CA is configured. |
| `namesAuthorizer` | An authorizer is configured. |
| `isIdentified` | A certificate issued by that CA is held. |
| `canVerifyPeers` | The CA certificate is held, so callers can be verified. |
| `verifiesTokens` | The authorizer's public key is held, so tokens are checked. |
| `offersTLS` | An HTTPS port is configured. The endpoint binds only once a certificate is obtained. |
| `acceptsPlaintext` | An HTTP port is configured. |

Two combinations carry more than their parts:

- **`namesAuthorizer` true with `verifiesTokens` false** is a system that intends
  to authorise and currently cannot. It is refusing every request with 503 rather
  than serving them unauthorised. Without the pair, that state is only visible as
  a log line on one machine.
- **`acceptsPlaintext` true above `open`** means the system can be reached
  without any of the protection its level names. Reporting the level alone would
  overstate it, so `String()` says so and the graph carries it.

## Where it surfaces

- **Once at startup**, from `SetoutServers`, as a single line. An adopter running
  a cloud for the first time should learn what protection is in force from the
  terminal rather than by reading the configuration back.
- **In the knowledge graph**, at `/<system>/kgraph`, as an `afo:SecurityPosture`
  linked from the system by `afo:hasSecurityPosture`. The graph is the right home
  because the question is about a cloud, not one system.

## Where enforcement happens

`AuthorizeRequest` permits immediately when the serving system's `coreSystems`
list has no `authorizer` entry, so authorization is adopted per system rather
than per cloud. Once one is declared, a provider that cannot yet verify refuses
with 503 rather than serving unauthorised.

The authorizer's own `authorize` service has exactly one legitimate caller, the
orchestrator, and checks the peer's common name when the connection carries one.
Over plain HTTP there is nothing to check, and it reports that rather than
refusing — refusing would break every deployment using the default `http://` core
URLs, to close a gap that only exists in clouds which have not adopted TLS
anyway.
