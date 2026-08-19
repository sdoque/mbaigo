# To do

Things found in the framework and not yet done. See `systems/TODO.md` for the
systems' own list.

## Decisions taken that somebody else should look at

- **The authorizer answers unverified callers while it is enrolling.** Its
  refusal of plaintext quests fires only once TLS has bound, and binding waits
  for the certificate — so for the length of one CSR round trip an unverified
  caller can be issued a signed token, which then outlives the window by its own
  lifetime. Left open deliberately: there is no better channel for the
  orchestrator to have used, and refusing would stop orchestration cloud-wide
  for a gap the cloud has not yet had the means to close. A test asserts the
  current behavior. Worth revisiting, not worth changing quietly.
- **Single-letter unit aliases are not coming back.** A code review asked for
  `"C"`, `"F"` and `"m"` to be restored to `legacyUnitNames` for migration. They
  are absent on purpose: in SI, C is the coulomb and F the farad, so restoring
  them buys convenience today and a silent miscalculation the day electrical
  units enter the table. The error message names which side could not be
  resolved instead.

## Known not to work

- **The painter's arrival and departure flashes do not display.** The elements
  are emitted and expire correctly — `painter/page_check.js` proves both — and
  nothing appears in a browser. The picture is rebuilt on every frame, so either
  those frames are not running or the elements are replaced faster than they can
  be painted. Undiagnosed.

## Worth doing

- **Nothing exercises a system from an empty directory to a running state.**
  Every one of a day's worth of faults lived in that gap — nine systems that
  could not start, two TLS regressions, a mission that was not a mission — and
  the test suites passed throughout. A smoke test that builds each system, runs
  it twice in an empty directory and asserts it reaches "up" would have caught
  all of them.
- **`Service.Merge` is called from nowhere.** It exists to stop a configuration
  changing a service's subpath or description, and the seam it was written for
  is now occupied by `fillServicesFromTemplates`. Either wire it in or delete
  it; a method that documents an intention nothing enforces is worse than
  neither.

- **`hasMethods` belongs in the ontology.** The framework writes it as
  `alc:hasMethods` because AFO 1.2.0 does not define it, which is what the
  `afoDefined` comment calls the agenda. It is the one thing a service now says
  that no shared vocabulary names, it is what democrat's Asset Interfaces
  Description reads, and promoting it costs one line in `kgraphing.go` once the
  ontology lands. Worth putting to Oskar alongside the 1.2.0 proposal.

- **Every failed request marks the provider stale.** `askOneProvider` wraps any
  error from `sendHTTPReqWithToken` in `staleProvider`, so a 503 from a provider
  that has not yet obtained the authorizer's key costs its token exactly as a
  403 does, and so does a 400 caused by the consumer's own payload. The doc
  comment on `staleProvider` says it is for "could not be reached, or it refused
  the credential", which neither of those is. Distinguishing them needs the
  status carried on the error. Left alone for now: the retry backoff cut the
  window to seconds, and this has been the behavior since it was written, so
  changing it without a fault to point at is speculation.
  `TestAProviderThatIsNotReadySaysSo` records what it currently does.
