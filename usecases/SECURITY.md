# What protects a local cloud, and what does not

A local cloud runs at whatever level of protection its operator has deployed.
Every level below the last one is a legitimate deployment: a cloud with no
certificate authority is how everybody starts, and a framework that refused to
run without one would teach nothing.

So the framework never withholds function to enforce security. It states what it
is doing instead. Each system prints one line when it starts serving:

```
thermostat: security: identified — callers over TLS are identified; no authorizer
configured: any identified system may use any service; an HTTP port is open, so
this system is also reachable without TLS
```

and publishes the same facts in its knowledge graph, at `/<system>/kgraph`:

```turtle
alc:pihome_thermostat_Security a afo:SecurityPosture ;
    afo:hasSecurityLevel "identified" ;
    afo:namesCertificateAuthority "true"^^xsd:boolean ;
    afo:namesAuthorizer "false"^^xsd:boolean ;
    afo:isIdentified "true"^^xsd:boolean ;
    afo:canVerifyPeers "true"^^xsd:boolean ;
    afo:verifiesTokens "false"^^xsd:boolean ;
    afo:offersTLS "true"^^xsd:boolean ;
    afo:acceptsPlaintext "true"^^xsd:boolean .
```

The graph is where a *cloud's* posture can be read rather than one system's:
which systems are enrolled, which are still reachable in the clear, which name an
authorizer they cannot reach.

## The four levels

| Level | Meaning |
|---|---|
| `open` | No certificate authority configured. Nothing is identified, nothing is authorised, everything is in the clear. |
| `enrolling` | A CA is configured; this system has no certificate yet. Its HTTPS endpoint is not bound. |
| `identified` | This system holds a certificate from the cloud's CA. Callers over TLS are named and verified. What they may do is not restricted. |
| `authorised` | As `identified`, and every incoming request must also carry a token minted for that caller, that service and that action. |

`acceptsPlaintext` qualifies every level above `open`: a system still listening on
its HTTP port can be reached without any of the protection its level describes.

## Three deployments

### 1. No CA, no maitreD, no authorizer

**Works, completely.** The HTTP server binds immediately and does not wait for
enrolment. Every system reports `open`.

**Protection: none.** No identity, no attestation, no authorization. Anyone who
can reach the port can call any service. This is the right way to learn the
framework and the wrong way to run anything.

One nuisance: if a `ca` entry is left in `coreSystems` pointing at a CA that is
not running, every system retries enrolment once a minute, forever, and logs each
failure. Remove the entry to get a quiet `open` cloud.

### 2. CA and maitreD, no authorizer

**Works.** Systems enrol, receive certificates, bind their HTTPS endpoints, and
present their client certificate on outbound calls. Providers verify it
(`RequireAndVerifyClientCert`). Systems report `identified`.

**Protection: authentication, not authorization.** You know who is calling — a
verified common name chaining to your CA, whose binary maitreD attested against
the CA-mastered whitelist at enrolment. You do not restrict what they may do: any
enrolled system may call any service on any other.

Two things stay in the clear at this level, by construction:

- **Enrolment.** A system with no certificate cannot complete an mTLS handshake,
  so the CSR goes to the CA over plain HTTP. This is what maitreD's binary
  attestation is for: the hop is not authenticated, so the *executable* is.
- **The core hops.** Registration, orchestration and certification all use the
  `coreSystems` URLs, which are `http://` in the generated template. Point them
  at the HTTPS ports to close this; the CA must keep an HTTP port either way.

### 3. `http = 0` everywhere

**The cloud never starts, and this is not a bug you can configure around.**

Enrolment is the reason. A system with no certificate cannot complete the mTLS
handshake that the CA's HTTPS listener demands, and there is no plaintext port
left to send the CSR to. Nothing enrols, so nothing gets a certificate, so no
HTTPS listener ever binds. Every system retries once a minute forever, reporting
`enrolling`.

maitreD compounds it: the CA reaches it at a hardcoded `http://` URL, so a
maitreD with no HTTP port can never attest anyone and the CA signs nothing.

**What does work** is `http = 0` on everything *except* the CA and maitreD, with
`coreSystems` pointed at the HTTPS ports. Enrolment is then the only plaintext
hop — the ordinary bootstrap compromise — and everything after it is mTLS.

## Where enforcement actually happens

Authorization is opt-in **per system**, through that system's own configuration.
`AuthorizeRequest` permits immediately when the serving system's `coreSystems`
list has no `authorizer` entry. Adding one is what switches enforcement on, for
that system alone.

This means a cloud can be authorised in part. A system that names an authorizer
and cannot reach it refuses every request with 503 rather than serving them
unauthorised — visible as `namesAuthorizer true` with `verifiesTokens false`.

The authorize service itself has exactly one legitimate caller, the orchestrator,
and checks the peer's common name when the connection carries one. Over plain
HTTP there is nothing to check, and the authorizer says so in its log rather than
refusing: refusing would break every deployment using the default `http://` core
URLs, to close a gap that only exists in clouds which have not adopted TLS
anyway. A cloud that means to be protected should point its core URLs at HTTPS,
and can confirm it did by reading `acceptsPlaintext` off the graph.
