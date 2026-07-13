# Adaptive bounded model research

This document records the provider and context pipeline observed before the
adaptive-research implementation, the boundary failure that motivates the
change, and the bounded target policy approved in decision 090. It describes
the normal `repomap <repo>` product path separately from opt-in playgrounds.

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
| Broad orientation | `internal/orient/orient.go:273-385`, transport at `internal/deepseek/client.go:433-460` | Interpret one `llmbundle.Bundle`: compact README, directory/language summaries, bounded modules, entrypoints, at most eight command traces, orientation candidates, edges, source signals, candidate file summaries, and closed `allowed_paths`. No raw repository tree or broad source dump. Prompt is built at `internal/deepseek/client.go:248-337`. | Exact request bytes, aggregate latency, and one logical call are saved in `metadata.json` (`internal/orient/orient.go:284-321`). Response bytes and transport retry count are not currently typed metrics. Current saved runs measured 45,697 request bytes for Caddy and 61,322 for Restic. | No cross-run cache. Mandatory online; failure aborts orientation. Offline and request-preview modes skip the call. | Supplies conceptual orientation and candidate flow seeds. Local path validation, confidence gates, and direction acceptance still apply. |
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

The complete local command catalog is collected in `gofacts.Facts.CommandTraces`
with its own 40-trace/16-call-per-command analyzer ceilings
(`internal/gofacts/commandtrace.go:16-20,98-112`). It remains in `snapshot.json`.

The orientation builder then:

1. ranks at most eight command traces for provider visibility
   (`internal/llmbundle/llmbundle.go:195-203,848-883`);
2. builds a candidate file index using local facts, including all command-trace
   paths (`internal/llmbundle/llmbundle.go:238-383`);
3. truncates that index to `MaxFiles`, with selected trace and entrypoint pins
   (`internal/llmbundle/llmbundle.go:218-222,416-535`);
4. assigns every selected index path to `AllowedPaths`
   (`internal/llmbundle/llmbundle.go:223-225,779-786`);
5. filters entrypoints, signals, candidates, and provider-visible command traces
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
The report parser already reloads full command traces from `snapshot.json` at
`internal/report/parse.go:636-648`.

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

The selected policy is versioned with the implementation and is calibrated on
saved Restic and Caddy fixtures. The intended defaults are:

| Budget | Target | Hard limit | Secondary ceiling |
| --- | ---: | ---: | ---: |
| Broad orientation request | 80 KiB | 96 KiB | 250 compact file summaries |
| One targeted request | 64 KiB | 80 KiB | 25 source windows / 160 evidence items |
| Architecture request | 80 KiB | 96 KiB | existing exact candidate ceilings |
| Normal-run total external requests | — | 320 KiB | four semantic calls |
| Targeted rounds | — | two | one question per round |

The byte limit is primary. File/window counts are safety ceilings, not the
repository-understanding budget. A stage is skipped rather than allowed to
exceed the remaining total. Transport retries remain attempts within one
semantic stage and are recorded separately.

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

## Measurement baseline

Pre-change saved normal runs show the orientation request below the selected
96-KiB hard limit but only because the 60-file ceiling truncates candidate
summaries first:

- Caddy: 45,697 orientation request bytes; one orientation call; architecture
  commonly replayed from cache;
- Restic: 61,322 orientation request bytes; one orientation call; one
  architecture call in the audited run.

The implementation records exact request and response bytes, tokens when the
provider reports them, latency, retries, cache state, locally inspected files,
evidence sent, accepted/rejected findings, new exact facts, and resolved
frontiers per stage. Model prose is never counted as a grounded fact.
