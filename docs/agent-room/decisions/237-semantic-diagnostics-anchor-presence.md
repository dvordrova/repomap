# 237 — Semantic diagnostics and Architecture anchor presence

**Status:** ACTIVE (owner-authorized, 2026-08-08)
**Supersedes:** only the sparse provider-stage failure diagnostics and the
incorrect nested-response implementation that conflates omitted
`anchor_refs` with an explicit empty array.
**Preserves:** Decisions 235 and 236, the bounded provider payloads, the exact
local Canvas fallback, Study continuation, and every privacy boundary.

## Incident

The uncached ghz Architecture response in run
`20260808-053558-ghz-f00ecb26b0af` was valid, complete, and covered all 28
requested conceptual refs. Eight components explicitly returned
`"anchor_refs": []`. The nested decoder treated those present empty arrays as
missing fields, emitted `proposal.normalized_missing_anchor_refs`, converted a
full accepted landscape to `partial_model`, and then failed the partial-model
state invariant because there was neither a local remainder nor item-local
salvage.

The durable status collapsed the exact local error to `invalid_response`, and
the console printed only `validation: response`. The provider call, JSON parse,
exact failed phase, response shape, and exchange path were therefore not
self-describing.

## 1. Cross-stage semantic diagnostics

Every semantic exchange journal record carries one closed outcome object in
addition to the existing state and validation class:

- exact bounded phase and stable code;
- an optional backend-owned safe detail (never raw provider/body/error text);
- a bounded list of non-negative named integer metrics;
- request/response SHA, byte counts, payload filenames, semantic calls, and
  transport attempts remain authoritative in the existing exchange record.

The recorder supplies a truthful generic outcome for every provider-assisted
stage. Stage owners may refine it with a narrower stable code and metrics.
The stage-owned validation class takes precedence over broad lifecycle state,
so a received response that fails decode is never mislabeled as a transport
failure. Request-attempt outcomes are recomputed from the latest state without
mutating caller-owned metadata.
Semantic exchange metadata advances to v2 and `exchange.v2.json`; historical
`exchange.v1.json` artifacts remain immutable and readable by existing replay
fixtures.

Run metadata records the exact local binary identity from Go build info:
module path/version, Go version, VCS revision/time, and modified bit. No endpoint
text, credentials, Authorization header, repository source bytes, stack trace,
or raw error string is added. Reopened writers preserve the validated identity
already bound to that run and never rebind a historical run to the current
process.

## 2. Architecture failed-status observability

`ArchitectureSynthesisStatusVersion` advances 11 → 12. An attempted response
failure retains bounded evidence that existed before the failure:

- provider-call success, response capture/parse state, finish reason and usage;
- failure phase, stable code, safe detail, and bounded metrics;
- semantic exchange metadata path;
- nested response shape counts: subsystems, components, member refs, anchor
  refs, omitted anchor fields, explicit empty arrays, and null fields when the
  response is a bounded decodable nested object.

Failed status still cannot publish accepted Architecture membership, source,
level, or coverage. Empty completed responses remain response-decode failures;
output-token and response-byte exhaustion use one stable output-limit code in
status, exchange, and run metadata. The console prints the same stable
phase/code, essential counts, and exact status/exchange paths. The canonical
local Canvas remains visible and Study continues.

Successful and cached Architecture console output also prints its selected
source, exact conceptual coverage/remainder counts, bounded response shape, and
the status/exchange paths. In particular, `accepted_partial` must explain what
is partial without requiring an artifact search.

## 3. Nested `anchor_refs` presence contract

The D235 model-facing grammar is unchanged:

- omitted `anchor_refs` → backend-owned `[]` plus exactly one counted
  `proposal.normalized_missing_anchor_refs` finding per affected component;
- explicit `"anchor_refs": []` → valid empty participation, no missing-field
  normalization;
- explicit `"anchor_refs": null` or a non-array value → invalid bounded wire
  response;
- non-empty arrays retain exact request-local ref validation;
- historical flat `unit_refs` replay remains supported but is not reintroduced
  into the live prompt.

This is a correction to the already-approved grammar, not a new model-facing
shape. Synthesis prompt/request/proposal/landscape identities therefore stay
unchanged. Rejected responses were never cached as accepted records.

## 4. Verification

- provider-free unit regression for omitted / explicit empty / null fields;
- full-coverage nested response with explicit empty arrays remains accepted and
  never enters an inconsistent partial state;
- status and exchange tests prove exact safe diagnostics and reject unsafe,
  malformed, duplicate, or unbounded metadata;
- build the exact candidate with `go build -trimpath`;
- run one fresh ghz acceptance with an instrumented pre-fix binary and caches
  disabled to capture the incident directly if the new stochastic response
  exercises it;
- run one fresh ghz acceptance with the fixed binary and caches disabled;
- verify exit status, metadata, semantic exchanges, Architecture status,
  synthesis record when accepted, Atlas, report JSON, report HTML, and manifest;
- focused tests, full Go tests, and `go vet` for changed packages.

Live provider calls for these two acceptance runs are explicitly authorized by
the repository owner. They are verification, not a retry/tuning loop.
