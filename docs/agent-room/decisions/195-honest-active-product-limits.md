# Decision 195: Honest active-product limits

## Status

Approved by the repository owner in the current supervisory session as the
reliability work immediately after Decision 194. The work lands as sequential,
reviewable, uncommitted checkpoints on top of the corrected Overview anatomy UI.

## Problem

The active product still has multiple independent `len` and cardinality limits
which silently change the semantic result. Overview source projection keeps only
eight ranked snippets even when more saved Surface and Architecture Component
locations are visible. Individual semantic stages impose body and record limits
below the configured provider output envelope and then omit, retry, or fall back.
Orientation builds its compact allowed-facts request from many unrelated magic
prefixes and may silently fit only a maximal prefix. Other production analysis
collectors use a mixture of true safety ceilings, validation invariants, and
arbitrary product-loss cutoffs without consistently distinguishing them.

Those behaviors make a successful report look complete when exact eligible facts
were discarded. They also make the configured model limits an inaccurate account
of the resources that the product will actually use.

## Decision

Replace silent active-product truncation with explicit, globally understandable
resource contracts. Preserve fail-closed schema, namespace, duplicate,
wrong-kind, path, manifest, report, secret, and semantic validation. Full compact
facts never means repository source contents, the raw file tree, or raw internal
edges: the provider boundary in `AGENTS.md` remains unchanged.

Implementation is split into four sequential review checkpoints.

### 1. Complete saved source coverage for visible Overview objects

The backend projects a deterministic exact saved source excerpt for every
persisted eligible visible entry Surface location and every exact Architecture
Component location. Eligibility is the same closed contract used by the
Overview anatomy: available application entry surfaces only, no provisional,
test, or helper surface; and component member locations, falling back to exact
behavior-anchor locations only when the component has no precise member
location. Saved object order and producer-owned component source order are
preserved; this order expresses identity, not importance.

Existing `user_sources`, captured-input hashes, repository authority,
source-catalog/workspace-content reads, redaction rules, report bytes, and run
manifest remain the authority. Exact excerpts are deduplicated without dropping
an owning visible object. Conflicting saved source identity, unreadable current
authority, or unsafe persisted content fails closed before publication. This
adds no DTO, version, provider input, artifact family, or source framework.

The eight-snippet cutoff is removed. Existing editorial excerpts remain in
their deterministic order and exact coverage excerpts are appended only when no
existing verified excerpt already contains the required location. If the full
mandatory report exceeds the existing global report artifact ceiling, report
generation returns a typed terminal resource error. It never silently omits a
card or publishes a partial report, manifest, or `latest` result.

### 2. One honest semantic output ceiling

Decision 194's configured `REPOMAP_LLM_MAX_TOKENS`, default 64,000, is the only
semantic output allowance. Remove the Architecture 256 KiB local fallback,
Guided Tour 64 KiB omission, Study review 64 KiB fallback, Localization 2 MiB
English fallback, and any other ordinary semantic stage-owned lower body or
record ceiling found by the production `len` audit. A shared transport safety
body ceiling may remain only when technically necessary and must be large enough
not to undercut the configured token envelope.

`finish_reason=length`, exact output-envelope exhaustion, and transport body
overflow are typed terminal resource outcomes. The existing safe exchange owner
may record the one failed exchange after mandatory redaction. No later semantic
call, cache/apply, report/manifest/`latest` publication, or semantic fallback is
allowed. Non-resource semantic validation and mandatory secret scans remain
separate and unchanged.

### 3. One honest compact-input byte budget

Replace ordinary Orientation magic prefixes—including edge, module, entrypoint,
source-signal, README, tree, interesting-file, Go package/edge, merged candidate,
operational/open-file/evidence, and equivalent active-product cutoffs—with the
complete deterministic eligible compact facts under one request byte budget.
The default is 1 MiB and one user override may raise or lower it.

The exact canonical request is built atomically. If it does not fit, a typed
terminal resource error is returned before the provider call and no later
product publication occurs. There is no byte-fit trimming, maximal-prefix
selection, retry, or fallback. Existing provider-visible allowed-path binding,
typed references, local source-content prohibition, and compact-bundle boundary
remain strict.

### 4. Classify remaining production analysis ceilings

Classify every active non-development/non-replay cap identified by
`/private/tmp/repomap-d194-len-audit-prod.txt`. Remove arbitrary cardinality
limits where bounded facts can be processed completely. Retain genuine CPU,
memory, file, and attack-surface ceilings only as explicit typed terminal errors,
or as first-class bounded-coverage status where partial output is semantically
valid and visibly labeled. A successful result must not silently claim
completeness after exhaustion.

Manifest/report size and hash authority, path and scalar validation, cardinality
and schema invariants, typed namespace and kind checks, duplicate rejection,
index constraints, empty-value checks, complete localization batching, and
secret rejection remain fail closed. Development, replay, and golden-only paths
are inventoried but not changed by this checkpoint without a separate cleanup
decision.

## Publication and review contract

Each checkpoint is reviewed as an uncommitted diff. No checkpoint uses a live
LLM, installs or replaces the PATH binary, or changes legacy readers or migration
behavior. A binary build, commit, or push occurs only after explicit supervisor
authorization. The corrected Overview anatomy keeps its drawer-first exact
source navigation, saved-order Components, explicit empty sections, Study
publication notice, and prohibition on fabricated joins, arrows, or importance.

## Proof

Provider-free proof includes:

- exact fresh Casdoor and etcd visible Surface/Component coverage counts and
  deterministic report/HTML byte deltas;
- source authority, exact line inclusion, deduplication, conflict, unsafe
  content, and report-resource-exhaustion tests with no partial publication;
- output-limit tests for every semantic stage, including length, body overflow,
  cache/apply/publication absence, cancellation, and non-resource separation;
- exact under/at/over compact-input budget tests showing complete facts or one
  pre-call typed terminal outcome and no provider request; and
- a classified production cap inventory with focused exhaustion tests plus the
  complete provider-free repository check.
