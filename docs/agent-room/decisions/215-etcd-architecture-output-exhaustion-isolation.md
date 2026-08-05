# Decision 215: Isolate attempted Architecture output exhaustion without losing the local product

## Status

APPROVED by the repository owner through the active supervisory goal
(`repomap-hermes-d215-etcd-failure-isolation-goal.txt`, 2026-08-05). This
corrective is scoped to attempted Architecture output/response resource
exhaustion only; it supersedes exactly one narrow clause of Decision 194 (see
below). D216 remains a recorded proposal, not an implementation.

## Evidence (exact incident)

- Run: `20260805-064730-etcd-4d99f0f8a558` (archive SHA-256
  `9a8238a9331d8a7f8f97c2d0f617155ec147d4de97a9fcec5a13cf2f85c1beea`).
- Architecture request SHA-256
  `00cb222adab0076fb8bdd346beb742f0be7e776f4a0118e673831ec427c905ab`;
  retained response SHA-256
  `be4dc6ccaf09dfa82904797301a16680e92d27a119a2348603b76cf0f6b38475`.
- Model `deepseek-v4-flash`; JSON-object mode; `thinking: disabled`;
  input 42,197 tokens; output 64,000 tokens; `finish_reason=length`;
  retained visible response 201,396 bytes.
- The response emitted one subsystem and one component, then remained inside
  that component's `member_refs`: 6,390 package-ref objects, 90 unique refs,
  one exact 56-ref / 1,761-byte block repeated 113 full times plus a partial
  repetition, no symbol refs, no `anchor_refs`, no `hypothesis`, no second
  component, no closing JSON. The first duplicate already made the proposal
  semantically invalid. This is a visible generation repetition loop, not
  hidden reasoning. D214 boundary/resource entities were not members of this
  request.

## D194 delta (exact narrow supersession)

Decision 194 states: "A typed resource-limit error terminates the complete
ordinary run with a non-zero exit. Later stages are not called and no
authorized report, run manifest, `latest` link, server, or opened report is
produced."

D215 supersedes that whole-run termination clause **only for an attempted
Architecture output/response resource exhaustion** — a typed
`ResourceLimitError` whose `Kind` is output tokens or provider response bytes,
raised after the Architecture provider call was attempted. Every other
semantic stage (Navigator, Theme Scout, Theme Adjudication, localization),
every pre-call candidate/catalog/request limit, cancellation, repository-
authority failure, artifact corruption, and status/accounting write failure
keeps its existing D194 behavior: typed resource-limit errors there remain
terminal. The global 64,000-token ceiling value is unchanged by this decision
(the earlier proposal to force an 8,192-token stage cap is superseded; the
current many-to-many membership contract has not yet been compacted enough to
prove that every legal answer fits a smaller envelope). The no-parse,
no-cache, no-apply, no-retry, no-fallback safety rules of D194 for the failed
response are fully preserved.

## Product invariant

Architecture synthesis is optional conceptual enrichment over the already
complete canonical local Architecture Canvas (D177). A length-ended response
must never be parsed, applied, cached, or presented as Architecture. But an
attempted Architecture output resource failure must no longer erase the
independently valid local product, Study, and report.

## Decision

### A. Dedicated attempted Architecture resource-failure type

Introduce an Architecture-owned typed wrapper (an extension of the existing
publishable-failure classification) that preserves the underlying
`modelresearch.ResourceLimitError` via `Unwrap`. The attempted Architecture
output/response exhaustion is publishable **only when all** are true:

- the Architecture provider call was attempted;
- the resource kind is output tokens (`ResourceLimitOutputTokens`) or provider
  response bytes (`ResourceLimitResponseBytes`);
- cancellation is not active;
- no accepted Architecture record/cache was written;
- the failed status is durable;
- the failed model-research accounting is durable.

A pre-call resource error (candidate/request/catalog limit) is never this
optional failure and stays terminal.

### B. Durable failed Architecture status

Advance `ArchitectureSynthesisStatusVersion` 8 → 9. Persist a bounded failed
status with:

- `state=failed`;
- closed error code `provider_output_limit` for `finish_reason=length`
  (distinct from the existing `empty_response`, `invalid_response`,
  `provider_error` codes);
- exact request bytes;
- known response bytes / content bytes;
- `provider_request_count=1`;
- exact transport attempts;
- `usage_reported`;
- input/output tokens when reported;
- configured/observed output limit (`configured_max_tokens`,
  `observed_output_tokens`);
- `finish_reason=length`;
- `response_complete=false`;
- local/requested/structural/anchor input counts.

The failed status must contain: no accepted proposal, no partial membership
count, no model Architecture source or level, no fallback represented as model
output, no provider prose or raw response. A failed status write remains
terminal (a status-write failure is not swallowed).

### C. Correct failed-call accounting

Record the attempted Architecture call exactly once in model-research state:
semantic calls +1, request bytes, response bytes, latency, input/output tokens
when reported, failed/resource-limited status. A pre-call rejection is not
counted. An accounting-write failure remains terminal. The existing run
metadata attempt is not a substitute for model-research accounting; both
remain truthful.

### D. Decision 192 exchange behavior preserved

Keep the one exact failed semantic exchange (request identity, partial
response diagnostic-only, existing redaction/secret handling). No second
exchange for the same semantic attempt; no execution read-back from the
diagnostic journal. Provider-free assertions use a generated compact loop
fixture, not the 201 KB owner run.

### E. Publishable continuation

Only after B and C succeed, allow `main` to continue: repository authority
reconciliation, Theme Scout, local source expansion, Theme Adjudication,
report/manifest generation, configured static export or serving/opening
behavior. The visible Architecture is the canonical local Canvas.

### F. Localized product disclosure

Use the typed EN/RU catalog.

- EN: "Conceptual grouping is unavailable because the model exceeded its
  response budget. The partial response was not used; exact local Architecture
  remains available."
- RU: «Концептуальная группировка недоступна: модель исчерпала лимит
  ответа. Частичный ответ не использован; точная локальная архитектура
  остаётся доступна.»

The warning is compact and secondary, never exposes the raw provider error,
and does not dominate Overview.

### G. Hard safety preserved

Prove: no `architecture_synthesis.json`; no accepted cross-run cache write; no
parsing of the response prefix; no membership recovery; no semantic retry; no
later model Architecture claim; local Canvas unchanged.

## Provider-free exact-failure replay

Add the smallest dev/test seam needed to replay the saved failure outcome
without another provider call: it accepts the prepared exact Architecture
request, a response body, and measured usage/finish evidence; it exercises the
real stage owner, status, accounting, continuation, report, and manifest
paths; it refuses unsafe/incomplete identities and does not treat replay as a
live provider call in transport accounting. The generated test response uses
one subsystem, one component, an open `member_refs`, a repeated 56-ref block,
and `finish_reason=length` — generated deterministically, not 201 KB in
testdata.

## Tests

1. attempted `finish_reason=length` with partial bytes;
2. status version/validation and closed error code;
3. model-research accounting exactly once;
4. no synthesis record/cache;
5. one redacted exchange only;
6. publication classification only after status + accounting;
7. status-write failure terminal;
8. accounting-write failure terminal;
9. cancellation terminal;
10. pre-call candidate/request/catalog resource limit remains terminal and
    provider-free;
11. non-resource Architecture provider failure retains existing behavior;
12. accepted, cached, accepted-partial, normalized, and semantically rejected
    Architecture paths remain green;
13. full CLI path continues through both D213 semantic stages and report;
14. report/manifest truthfully bind the failed Architecture status and local
    Canvas;
15. EN/RU catalog parity and rendered copy;
16. no raw provider error in primary UI.

Then: `gofmt`, `go test ./...`, `go vet ./...`, `make build`,
`node --check` on touched JS assets, all existing report/manifest/provider-free
acceptance gates.

## etcd acceptance

First replay the exact failure provider-free and generate a complete etcd
report from the saved local artifacts plus deterministic semantic fixtures.
After all provider-free gates are green, at most one fresh live etcd run is
authorized if credentials are available. PASS requires either a complete model
result or the truthful optional fallback (resource limit reached; partial
bytes diagnostic-only; failed status and accounting exist; no synthesis
record/cache; Study executes; report and manifest publish; command exits
successfully; exact local Architecture opens; warning visible and secondary).
No second live tuning call.

## Follow-up record (not implementation)

D216 — replace raw-member global partitioning with locally compiled bounded
Architecture units (24–64) before one model call that groups unit refs only,
then local expansion to exact members. The etcd request proves why: 251
conceptual candidates, 635 relations, 613 package-import edges (~74.6 KB), and
the model looped while enumerating raw package refs. Recorded, not
implemented, in D215.

## Non-goals

No 128k output ceiling; no 8,192 stage cap yet; no hidden-reasoning/
provider-profile work; no prompt-only "do not repeat" fix; no retry; no prefix
repair; no D214 rollback; no full UI redesign; no streaming/provider
framework; no Architecture ontology rewrite; no push.
