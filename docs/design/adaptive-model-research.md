# Adaptive bounded model research

This document records the provider and context pipeline observed before the
adaptive-research implementation, the boundary failure that motivates the
change, and the bounded target policy approved in decisions 090, 091, and 092.
It describes the normal `repomap <repo>` product path separately from opt-in
playgrounds.

## Observed pre-change product pipeline

The normal online product path is:

1. build a complete deterministic tracked-file snapshot;
2. build one compact orientation bundle;
3. make one mandatory orientation call;
4. attach local FlowProof sessions to accepted orientation flows;
5. optionally make one prose call per `--flows N` selection;
6. make one best-effort architecture-synthesis call;
7. render saved artifacts without further provider calls.

The ordinary CLI hard-codes `MaxLLMFiles = 60` and `MaxLLMEdges = 60` at
`cmd/repomap/main.go:229-253`. The older `repomap orient` developer command uses
different limits. Internal non-positive fallbacks are 150 files and 120 edges
at `internal/llmbundle/llmbundle.go:81-103`.

### Normal provider-call sites

| Stage | Call site | Purpose and visible facts | Request/response accounting | Cache and necessity | Local effect |
| --- | --- | --- | --- | --- | --- |
| Broad orientation | `internal/orient/orient.go`, transport at `internal/deepseek/client.go` | Interpret one `llmbundle.Bundle`: compact README, directory/language summaries, bounded modules, entrypoints, orientation candidates, edges, source signals, candidate file summaries, and closed `allowed_paths`. A replayed legacy snapshot may also contribute at most eight persisted command traces; fresh ordinary runs produce none. No raw repository tree or broad source dump. | Exact request bytes, aggregate latency, and one logical call are saved in `metadata.json`. Response bytes and transport retry count are not currently typed metrics. | No cross-run cache. Mandatory online; failure aborts orientation. Offline and request-preview modes skip the call. | Supplies conceptual orientation and candidate flow seeds. Local path validation, confidence gates, and direction acceptance still apply. |
| Focused flow explanation | `internal/orient/orient.go:417-448` and `internal/orient/flow.go:310-376`, transport at `internal/deepseek/client.go:365-367` | Explain one deterministic `flowexplain.FlowBundle` containing selected tracked files/tests/docs/packages/edges/signals for a model-suggested flow. | Exact request bytes increase aggregate metadata. No per-call response bytes, latency, retry count, prompt version, or cache fingerprint. | Optional; zero calls when the default `--flows 0` is used. One logical call per selected flow otherwise. | Adds validated prose/read order only. It does not add local edges or proof facts. |
| Architecture synthesis | `cmd/repomap/architecture_synthesis.go:79-210`, adapter at `internal/deepseek/component_synthesis.go:29-46` | Group and name opaque exact members using repository archetype, behavior anchors, saved flows, local candidate facts, local supporting relations, and anchor bindings. The request contract is `internal/componentmap/synthesis.go:51-75`; the model cannot return paths, evidence, edges, certainty, or layout. | Input bytes and latency are saved in the singular call record; response byte count and validation outcome are also saved (`internal/componentmap/synthesis.go:87-121`). | Best-effort and persistently cached. The pre-change key includes a constant captured revision, component contract, prompt version, and exact request hash (`internal/componentmap/synthesis.go:258-277`), but not provider/model or a research-policy version. No retry. | May replace conceptual naming and membership only after local validation. Deterministic local fallback survives invalid output. |

The shared orientation/flow transport retries one logical call up to three times
after the initial attempt for network failures, 429, and 5xx responses. It does
not retry non-retryable 4xx, invalid JSON, empty/invalid envelopes, or local
validation failures (`internal/deepseek/client.go:395-460`). Backoff is bounded
and context-aware. Architecture synthesis currently makes one transport
attempt. Metadata counts logical provider stages, not retry attempts.

### Other provider-call sites

These are explicit developer or focused-investigation paths and are not called
by ordinary browser navigation:

- source assessment: `internal/sourceexplain/service.go:27-49` through
  `internal/deepseek/source.go:22-95`; bounded source lines and opaque evidence
  IDs; saved/replayed by the investigation playground;
- symbol explanation: `cmd/symbol-playground/main.go:296-303` through
  `internal/deepseek/symbol.go`; bounded symbol bundles;
- component-study planning: `internal/componentstudy/service.go:23-38` through
  `internal/deepseek/component_plan.go`; a standalone bounded question/ID
  selector;
- component teaching: `internal/componentteach/service.go:23-38` through
  `internal/deepseek/component_teach.go`; a standalone evidence-ID explanation;
- provider doctor: `cmd/repomap/main.go:580-613`; one tiny synthetic JSON call,
  no repository facts and no retry.

These paths retain their own contracts. Decision 090 changes the normal report
pipeline; it does not silently count standalone commands against another run.

## Observed context construction and coupling

`gofacts.Facts.CommandTraces` remains a legacy persisted field with bounded
readers so old `snapshot.json` artifacts still load. Decision 273 stopped the
ordinary Go-facts loader from producing new command traces.

When replaying a legacy snapshot, the orientation builder still:

1. ranks at most eight command traces for provider visibility
   (`internal/llmbundle/llmbundle.go:195-203,848-883`);
2. builds a candidate file index using local facts, including all command-trace
   paths (`internal/llmbundle/llmbundle.go:238-383`);
3. truncates that index to `MaxFiles`, with selected trace and entrypoint pins
   (`internal/llmbundle/llmbundle.go:218-222,416-535`);
4. assigns every selected index path to `AllowedPaths`
   (`internal/llmbundle/llmbundle.go:223-225,779-786`);
5. filters entrypoints, signals, candidates, and legacy provider-visible command traces
   against that closed set (`internal/llmbundle/llmbundle.go:225-233,906-942`).

`AllowedPaths` is therefore correctly a provider request allowlist, but its
name has also been allowed to imply local availability. A trace with a step
outside the selected set can be omitted from the provider bundle even though
the complete local snapshot still contains it.

The exact confirmed defect is at `internal/orient/proof.go:10-13`:

```text
selected candidate file index -> bundle.AllowedPaths
bundle.AllowedPaths -> filtered bundle.Go.CommandTraces
bundle.Go.CommandTraces -> local flowproof/assemble.Attach
```

Consequently, pre-change FlowProof receives the provider-filtered copy instead
of `snapshot.GoFacts.CommandTraces`. This is not required by the proof engine.
Its Go executor starts from an exact proof transition, confines paths to the
canonical repository root, and may resolve a declaration in another tracked
local file (`internal/analyzer/golang/gotypes/resolver.go:50-63,90-165,268-297`).
The report parser still reloads full legacy command traces from `snapshot.json`.

Focused direction retrieval also authorizes against the full tracked-file list,
not the orientation allowlist (`internal/orient/flow.go:156-166` and
`internal/flowexplain/flowexplain.go:230-355`). The coupling is therefore an
accidental initial-proof input choice, not a repository-security requirement.

## Explicit evidence scopes

The adaptive pipeline uses three non-interchangeable scopes:

### Local repository universe

All tracked, authorized, safe, build-selected inputs available to deterministic
analysis. Bounds come from repository and manifest authorization, build
scenario, ignored/secret policy, and explicit analyzer task/file/symbol/depth/
time budgets. The local universe is not serialized into provider requests.

### Provider-visible bundle

The exact evidence serialized into one provider request. Each stage has a
closed `provider_allowed_paths`/opaque-ID set, a request-byte target and hard
limit, and secondary count ceilings. Provider validation remains fail-closed.

### Focused investigation scope

The bounded local worklist selected for one question or trace frontier. Its
`focused_evidence_ids` can refer to tracked files that were absent from initial
orientation. Only a further selected subset is made provider-visible.

Evidence records distinguish `provider_visible_initially`,
`locally_retrieved_after_orientation`, `provider_visible_in_targeted_round`,
and `never_sent_to_provider`.

## Approved target pipeline and budgets

The target normal run is:

```text
deterministic repository facts
  -> broad byte-bounded orientation
  -> local question planning and value-of-information gate
  -> bounded local evidence expansion
  -> zero, one, or two targeted interpretation rounds
  -> accumulated working theory
  -> final bounded architecture synthesis
```

The selected policy is versioned with the implementation and calibrated on
saved Restic and Caddy fixtures. Targets guide measurement and calibration;
they are not shaping or rejection thresholds. Technical ceilings are
fail-closed safety rails:

| Budget | Soft target | Technical ceiling | Secondary ceiling |
| --- | ---: | ---: | ---: |
| Broad orientation request | 80 KiB | 1 MiB | 250 compact file summaries |
| One targeted request | 64 KiB | 1 MiB | 25 source windows / 160 provider evidence items |
| Architecture request | 160 KiB | 1 MiB | existing exact candidate ceilings |
| Normal-run total external requests | materially below ceiling | 4 MiB | four semantic calls |
| Targeted rounds | — | two | one question per round |

Bytes remain the primary provider boundary. File/window counts are secondary
safety ceilings, not the repository-understanding budget. A request may exceed
its soft target when retaining useful bounded evidence requires it, but a stage
is skipped rather than allowed to exceed the technical or aggregate ceiling.
Evidence omitted from a targeted provider bundle remains separately recorded as
local evidence with `never_sent_to_provider` visibility. Architecture input is
not reduced by repeatedly lowering an arbitrary candidate-count constant.
Transport retries remain attempts within one semantic stage and are recorded
separately.

The former optional per-flow prose calls are not part of the normal adaptive
allocation: targeted rounds replace their model role. Saved deterministic flow
bundles remain available without another provider call.

## Cache and failure policy

Each stage fingerprint binds repository identity/revision, relevant dirty input
hashes, build scenario, stage, prompt version, provider/model, canonical local
evidence bundle hash, and research-policy version. Report layout/template bytes
are deliberately absent. A changed evidence bundle invalidates that stage and
its dependents, not unrelated cached stages.

Provider failure never removes deterministic evidence or earlier accepted
findings. A failed/invalid targeted response is recorded as failed/rejected; it
does not trigger a differently worded semantic round. Architecture synthesis
continues with local facts and any prior validated findings. Budget exhaustion
persists the frontier and still finishes the report.

Context cancellation is different from provider or analyzer failure. The
normal CLI signal context reaches provider requests, targeted local planning,
and Go surface discovery. Cancellation ends the run rather than becoming a
warning followed by more work. The default provider timeout is ten minutes
(still configurable through the documented environment override), and
content-free wait heartbeats are emitted every ten seconds for long provider
stages and surface discovery.

## Measurement baseline

Pre-change saved normal runs showed orientation requests below the former
96-KiB cutoff only because the 60-file ceiling truncated candidate summaries
first:

- Caddy: 45,697 orientation request bytes; one orientation call; architecture
  commonly replayed from cache;
- Restic: 61,322 orientation request bytes; one orientation call; one
  architecture call in the audited run.

The final policy-v2 local budget check measured 250 compact file summaries for
each repository without a provider call:

- Restic: 1,344 authorized files, 186,442-byte local bundle, and 147,660-byte
  orientation request;
- Caddy: 612 authorized files, 169,097-byte local bundle, and 143,992-byte
  orientation request.

The final bounded product journeys measured:

- Restic: 100,345-byte compact context; 117,476-byte orientation request;
  34,587-byte and 21,271-byte targeted requests; 190,483-byte architecture
  request; four semantic calls and 363,817 aggregate request bytes. Two local
  runtime surfaces were discovered. The architecture provider response was
  captured but rejected for ungrounded components, so the report correctly used
  the local fallback rather than presenting the response as validated.
- Caddy: 88,481-byte compact context; 103,701-byte orientation request;
  6,449-byte and 4,278-byte targeted requests; and a 142,891-byte architecture
  request. All model stages replayed exact caches, the architecture proposal
  validated, and local discovery honestly retained zero runtime surfaces.

These measurements justify removing the former 96-KiB architecture rejection
threshold. They also demonstrate why a successful provider call is not an
acceptance criterion: exact-ID and grounding validation still controls whether
the model proposal or deterministic local fallback reaches the report.

The Restic journey visibly exercised ten-second content-free heartbeats for
surface discovery, orientation, both targeted rounds, and architecture
synthesis. A channel-coordinated default-journey test cancels an in-flight
orientation request and verifies that no targeted research, architecture,
report publication, or generic failure wording follows; the default CLI reports
`repomap: canceled` and uses exit status 130.

The implementation records exact request and response bytes, tokens when the
provider reports them, latency, retries, cache state, locally inspected files,
evidence sent, accepted/rejected findings, new exact facts, and resolved
frontiers per stage. Model prose is never counted as a grounded fact.
