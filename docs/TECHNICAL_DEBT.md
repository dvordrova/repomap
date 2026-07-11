# Technical debt ledger

This file tracks concrete implementation debt discovered by experiments. Product
questions belong in [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md); planned investigation
architecture belongs in [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md).

Last reviewed: 2026-07-10.

## Active

### TD-001: Model capability boundary is still named and owned by DeepSeek

**Evidence:** the main CLI now has atomic provider-neutral `REPOMAP_LLM_*`
endpoint/model/auth/timeout configuration, explicit no-auth, DeepSeek aliases,
`doctor`, and request preview. However, `internal/orient` still constructs the
concrete `deepseek.Client`; prompt methods, response mode, retries, and several
artifact/experiment names remain provider-owned.

**Consequence:** a compatible company endpoint is a supported runtime choice,
but changing the model capability contract or using a non-compatible transport
still requires editing orchestration.

**Done when:** each consuming cube owns its small validated model contract and a
second client can implement it without importing prompt/domain state from the
DeepSeek package. Keep the now-working provider-neutral runtime configuration
and documented DeepSeek aliases.

### TD-002: HTTP timeout and cancellation do not fit local inference

**Evidence:** timeout is now configurable through `REPOMAP_LLM_TIMEOUT`; the
doctor uses one small request without retries. Normal orientation/source calls
still retry retryable failures, and prior Qwen 3B runs showed Ollama continuing
generation after client cancellation while a retry waited behind it.

**Consequence:** an engineer can choose an adequate timeout, but slow local
generation may still waste CPU after cancellation and retries may amplify it.

**Done when:** cancellation behavior has an integration test and retry policy can
avoid amplifying an already-running local generation. Configurable timeout and
the no-retry doctor probe are complete.

### TD-003: The full symbol prompt is unsuitable for CPU-local models

**Evidence:** the etcd `kvServer.Put` experiment produced a 16,962-byte bundle
and a 21,007-byte tagged request. Ollama counted 5,512 input tokens for Qwen 0.5B.

Local baselines on an Intel i7-9750H MacBook Pro:

| Model | Format | Time | Outcome |
| --- | --- | ---: | --- |
| Qwen2.5-Coder 3B Q4 | tagged | >120 s | interrupted; context fit, latency did not |
| Qwen2.5-Coder 0.5B Q4 | tagged | 81.91 s | parseable but mostly copied prompt placeholders |
| SmolLM2 135M F16 | tagged | 142.81 s | ignored contract and hallucinated unrelated code |

A compact static-facts prompt reduced Qwen 0.5B input from 5,512 to 634 tokens.
The tagged attempt completed in 18.85 seconds but misunderstood `KEY: VALUE`,
generated Markdown, hit the 320-token limit, and scored 40/100. A subsequent
JSON-Schema experiment with a 523-token input completed without truncation in
142.58 seconds and scored 45/100. The schema guaranteed the outer shape, but the
model copied an instruction as both interpretations and invented evidence IDs;
local normalization discarded or repaired those claims. Constrained decoding is
therefore reliable enough for contract testing but too slow and semantically weak
for this repository task on the 0.5B model.

The same 523-token JSON-Schema experiment on Qwen2.5-Coder 1.5B completed in
22.64 seconds (18.71 output tokens/second) and scored 60/100. It followed the
schema, used only known evidence IDs, and proposed a grounded reading order, but
invented concrete `exampleKey`/`exampleValue` request values and cited valid but
semantically irrelevant call edges for its claims. The model is fast enough for
iteration on this machine; the result is not yet trustworthy repository
understanding.

`local-symbol-v2` then replaced the monolithic prose request with deterministic
name preclassification plus two constrained model decisions: choose a role over
prioritized evidence, then choose an executable next action. Qwen 1.5B has no
free-text output fields in this protocol. Three consecutive `kvServer.Put` runs
were identical:

| Target | Model calls | Input/output tokens | Time | Protocol checks |
| --- | ---: | ---: | ---: | ---: |
| `kvServer.Put` | 2 | 380 / 63 | 3.98–4.08 s | 9/9 |
| `kvServer.DeleteRange` | 2 | 387 / 63 | 5.83 s | 9/9 |
| `WAL.Save` | 3 | 579 / 118 | 8.12 s | 8/8 |

All runs had zero parser warnings and the locally rendered report scored 100/100
on the existing contract evaluator. Put/DeleteRange selected validation, error
translation, and delegation evidence; WAL selected `sync`, `saveEntry`, and
`saveState`. Each result conservatively chose `read_target` because source
behavior was still absent. These scores validate the staged protocol and reducer,
not semantic truth about the source.

The same Qwen 0.5B model succeeds on genuinely small requests:

| Smoke test | Context | Time | Outcome |
| --- | ---: | ---: | --- |
| one-sentence Go explanation | 22 input tokens | 10.60 s cold | normal text response |
| code review with runtime JSON Schema | 47 input tokens | 8.16 s warm | valid required fields and enums |

The structured review was parseable but semantically generic: it suggested
general read error handling instead of naming `scanner.Err()`. This is sufficient
for provider/contract validation, not proof of useful repository understanding.

The machine is an x86 Intel Mac. Ollama correctly uses CPU-only inference on
this platform; the Radeon Pro 560X is not used by the macOS Ollama runtime.

**Done when:** the staged profile is fed by indexed context rather than a saved
bundle, executes its chosen `read_target` action into bounded source evidence,
and is measured against Qwen 3B. The full remote bundle remains available.

### TD-004: Prompt evaluator overestimates semantically useless responses

**Evidence:** the Qwen 0.5B response followed most tags but copied the placeholder
responsibility, cited a non-test target as `TEST`, claimed there were no unknowns,
and repeated an instruction as `NEXT_QUERY`. The current structural rubric would
still assign a relatively high score. Qwen 1.5B then scored 60/100 while inventing
concrete request values: its evidence IDs existed, but did not support the claims
that cited them.

**Consequence:** prompt experiments can appear successful while producing little
useful repository understanding.

The staged protocol avoids this failure mode by eliminating model prose and
scoring constrained decisions separately. It does not repair the generic prose
evaluator, which is still used by DeepSeek and monolithic experiments.

**Done when:** fixtures cover prompt/template echo, invalid test evidence, empty or
vacuous unknowns, malformed next queries, and claims that merely restate symbol
identity. They must also cover novel concrete literals and valid-but-irrelevant
citations. Contract score and semantic-usefulness score should be reported
separately.

### TD-005: Experiment artifacts lack explicit contract versions

**Evidence:** `cmd/symbol-evaluate` normalizes and scores a captured response
without another model call. The obsolete monolithic Ollama experiment that
produced unversioned request/envelope/report directories has been removed; its
measured failures remain recorded above. Any surviving older captured directory
still lacks stable prompt, schema, parser, or evaluator version identifiers.

`ollama_symbol_staged_experiment.sh` does record protocol, prompt, schema,
reducer, evaluator, model, Ollama, options, and bundle-hash metadata. The older
monolithic path was removed; any historical captures from it remain incomparable.

The M3 quality task/result pair now records provider, model, prompt version,
capture precision, model-context bytes/hash, nullable provider-request
bytes/hash, artifact hashes, repository revision/scenario, and evaluator
version. Applying it to the historical etcd orientation capture exposed
genuinely missing model, prompt-version, request, and latency metadata; the
fixture records those values as `unknown`/`null` instead of inventing them. It
also keeps the 3,536-byte source replay DTO distinct from the 3,001-byte model
context. The older 6,601-byte value described an indented preview rather than
the compact wire request, so task schema v2 records the legacy request hash and
byte count as `null` instead of preserving false precision. This closes the
quality-suite path but not the older experiment paths described above.

The k6 task uses task schema v2 to distinguish pre-parser `provider_content`
from a post-parser `normalized_report`. Evaluator v3 rejects both an ambiguous
many-candidate match and a drill-down path unrelated to every selected
orientation candidate. Request preview and the live
client now share the same compact serializer;
`scripts/quality_capture_meta.sh` removes the artifact-only terminal newline
where present and derives byte-identical context/request hashes without exposing
credentials. The k6 orientation/source latencies were not instrumented, so they
remain `null` rather than being reconstructed from filesystem timestamps. The
compact request/context bodies remain ignored local capture artifacts; offline
replay pins the recorded values in its baseline test but cannot independently
recompute them from the five committed replay artifacts.

Future orientation and source runs retain measured provider/source-call latency
in their ignored metadata. `scripts/quality_preflight.sh` also fails before
network use when the exact symbol path is absent from the bounded orientation
request, and records clean revision/toolchain plus exact context and request
hashes for the linked target. It deliberately does not invent raw model
responses or a passing task manifest.

**Done when:** experiment metadata identifies provider plus stable prompt, schema,
parser, and evaluator versions, so results remain comparable after any of those
contracts change. Artifacts must continue to exclude credentials and
authorization headers.

### TD-006: Large orchestration functions outgrow one source-assessment cube

**Evidence:** the k6 `Scheduler.Run` experiment resolves the exact method and 20
bounded test references, but the default source card stops after 80 lines at
`internal/execution/scheduler.go:498`. The method continues through executor
launch, result collection, and teardown. Within the retained window the current
name-based question seeder emitted only `maps_error`; DeepSeek correctly marked
it ambiguous, leaving no source-supported claim.

The same boundary originally blocked two otherwise linked M3 preflights before
a model call: NATS `client.processInboundMsg` and golangci-lint
`runCommand.runAnalysis` were present in their 60-path orientation bundles and
resolved exactly, but neither produced a bounded source question. Syntax-only
questions now cover the small visible shapes in those two symbols. The remaining
debt is the large-method continuation represented by `Scheduler.Run`, not those
two short functions.

**Consequence:** blindly selecting a central long method can produce excellent
navigation but a weak semantic step. Increasing every prompt globally would
spend context without ensuring that the relevant operation is included.

**Done when:** a bounded continuation or goal-aware source selection can request
the next relevant segment, and orchestration/delegation questions are seeded
from local syntax without claiming callee behavior. `Scheduler.Run` should then
cover setup, executor lifecycle, error selection, and teardown as separate
evidence-backed steps while preserving the 160-line/32 KiB provider ceiling.

### TD-007: Test references do not establish test support

**Evidence:** the M1 `find_tests` capability produces bounded gopls locations
with static provenance and build scenario, but deliberately marks them only as
`test_reference`. It does not read the test body or determine what is asserted.

**Consequence:** related tests are useful navigation, but no behavioral claim may
be promoted to `test_supported` from a reference location alone.

**Done when:** an investigation can select one reference, collect bounded test
source, identify the relevant test/case and assertion with cited evidence IDs,
and either promote a matching claim or preserve an explicit contradiction or
unknown. This must remain lazy rather than parsing every test eagerly.

### Resolved TD-008: Local memory now has mandatory freshness context

Resolved on 2026-07-10. `freshness.RepositoryState` records a stabilized
canonical identity, HEAD, non-ignored dirty-content hashes, and ignored Go build
inputs while excluding unrelated ignored files such as `.env`. `FactContext`
adds Go/gopls and collector versions, GOOS/GOARCH/tags, GOFLAGS/GOWORK/CGO, and
the normalized analyzer/collector options. `ClaimContext` binds claims to an
exact fact document plus provider/model, prompt, parser, and evaluator versions.

`internal/index` v2 requires a current `FactContext` on load and returns a typed
stale error before decoding stored symbols. The production investigation resume
path requires current repository/fact/claim contexts through `memory.Load` and
reduces repository, fact, or claim changes before returning an executable
action. Same-HEAD/different-dirty-content, tool/options changes, prompt changes,
tampering, and symlinked artifacts have direct tests.

**Residual optimization:** the current investigation session safely discards
all focused facts after any repository-content change, even when the changed
path is unrelated. `internal/index` still has path-level invalidation, but that
selective policy is not wired into session memory. Measure repetition and
latency in the friend onboarding trial before adding dependency-aware reuse;
this is an efficiency limitation, not a stale-evidence hole.

### TD-009: Candidate directions do not yet route to symbols

**Evidence:** the default `./repomap` pass now retains and displays every
validated orientation direction, but the cards are read-only. The isolated
handoff records a selected flow as provenance while `investigation.Runner`
still requires a separately supplied exact symbol; it does not consume the
selected flow when choosing evidence. Flow IDs are derived name slugs, and the
main CLI exposes neither named selection nor deterministic symbol candidates.

**Consequence:** the orientation baseline is useful for choosing where a human
wants to investigate, but clicking or auto-expanding a model-proposed
entrypoint would falsely promote a navigation hypothesis into a symbol fact.
The complete journey still needs a manual bridge.

**Done when:** one shared application step maps a user-selected direction's
verified files to a bounded deterministic set of exact analyzer symbols, asks
the user to confirm one, and enters the existing investigation reducer through
the main CLI/browser state. The selected direction must affect ranking and be
bound to repository identity and revision; model prose alone must never become
the selected symbol.

### TD-010: Name-seeded source questions mix hypotheses with observations

**Evidence:** `classifyCall` currently seeds semantic-looking question labels
such as `validates_input` from a callee name. The Prometheus `Labels.IsValid`
capture can locally prove that the value assigned from `Validate` is returned in
a nil comparison, but it still cannot prove what `Validate` does internally.
The reconstructed claim text preserves that unknown; the predicate label alone
does not.

This is partially paid down for calls that have no recognized name hint. They
now receive syntax-only predicates (`checks_call_result`,
`returns_call_result`, or `calls_from_branch`) only after the bounded scanner
proves the corresponding local shape. Historical name-seeded predicates remain
unchanged, so the semantic-hypothesis/observation split is not yet complete.

**Consequence:** a consumer that reads only `Claim.Predicate` may overstate a
syntax-grounded call-result observation as callee behavior. Quality fixtures and
UI copy must describe this as a validation-shaped/name-seeded question until a
callee or test source step adds stronger evidence.

**Done when:** the contract separates the seeded semantic hypothesis from the
locally proven syntax observation (for example `returned_nil_comparison` or
`guarded_call_result`), and a later evidence cube explicitly promotes or rejects
the semantic hypothesis without breaking historical replay fixtures.

### TD-011: Investigation sessions do not retain parser diagnostics

**Evidence:** `sourceexplain.Explanation` contains the normalized report, parser
warnings, and contract evaluation, but `investigation.Runner` emits only the
report in `EventSourceAssessed`; `Session` persists only that report. A response
reduced to `ambiguous` remains safe, but a resumed CLI/browser cannot explain
which model drift caused the downgrade or show its reduced contract score.

**Consequence:** raw/debug artifacts can diagnose the original run, while the
durable investigation state cannot present the same trust signal. Future safe
local repairs would be especially misleading if their provenance disappeared at
this boundary.

**Done when:** the assessed-source event/session stores a compact validated
envelope containing report, warning codes, and parser/evaluator/prompt versions;
resume tests prove those diagnostics survive without storing raw provider text.

## Maintenance rules

- Add an item only when there is concrete evidence or a demonstrated gap.
- State the consequence and an observable completion condition.
- Do not use this file as a feature wishlist.
- Remove or move resolved items into the commit/decision that resolved them.
