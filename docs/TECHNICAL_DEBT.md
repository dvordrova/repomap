# Technical debt ledger

This file tracks concrete implementation debt discovered by experiments. Product
questions belong in [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md); planned investigation
architecture belongs in [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md).

Last reviewed: 2026-07-11.

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

The retired staged Ollama experiment recorded protocol, prompt, schema, reducer,
evaluator, model, options, and bundle-hash metadata. Historical captures remain
incomparable and the shell entrypoint is intentionally not maintained.

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
The retired capture helper removed the artifact-only terminal newline where
present and derived byte-identical context/request hashes without exposing
credentials. The k6 orientation/source latencies were not instrumented, so they
remain `null` rather than being reconstructed from filesystem timestamps. The
compact request/context bodies remain ignored local capture artifacts; offline
replay pins the recorded values in its baseline test but cannot independently
recompute them from the five committed replay artifacts.

Future orientation and source runs retain measured provider/source-call latency
in their ignored metadata. The preflight behavior remains a testable contract:
fail before network use when the exact symbol path is absent from bounded model
context, and never invent raw responses or a passing task manifest.

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

### TD-009: Browser-selected investigation is durable but deliberately single-track

**Evidence:** accepting an exact component symbol now reduces `EventSourceRead`,
saves the real repository and scoped Go `FactContext` below the authorized run,
and resumes after handler restart without the opaque candidate cache. A local
source-ready branch supports leaf functions. One explicit action collects up to
five direct `_test.go` references without a provider request.

The MVP intentionally keeps one current checkpoint per report run. Selecting a
new symbol replaces the current session. Returned test references remain
non-clickable because they are not part of the immutable report manifest's
editor-open authority, and they are labelled as navigation evidence rather than
test support. The browser does not automatically request model assessment.

**Consequence:** the friend can leave and resume one useful drill-down, but this
is not yet a multi-investigation workspace and it cannot claim what a related
test proves.

**Revisit when:** observed onboarding use actually needs multiple retained
investigations, or when a new signed/validated action authority can safely make
post-report test paths clickable. Do not add back/branch/session-manager UI only
to make the state machine look complete.

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

### TD-012: Debug artifact writes are not fully symlink-confined

**Evidence:** the debug writer uses predictable `name.tmp` files and ordinary
`MkdirAll`/`WriteFile`/`Rename` under a caller-selected `--debug-dir`. The
default OS user-cache directory is private and the README requires a trusted
custom location, but a pre-created symlink inside an attacker-controlled custom
directory can redirect a write outside the run directory.

**Consequence:** the default friend trial is not exposed, but placing
`--debug-dir` inside an untrusted checkout can overwrite another writable file.
M5 increases the number of local direction artifacts and therefore makes the
existing writer limitation more important to keep explicit.

**Done when:** the writer resolves a trusted base once, rejects symlink/special
ancestors, creates randomized exclusive temp files in confined directories, and
atomically renames them; adversarial tests cover the run directory, `flows/`,
and temp-file targets.

### TD-013: Repository survey selection is still file-first

**Evidence:** the 60-path etcd onboarding bundle retained many files from
`server/etcdserver/api/rafthttp` while omitting the real core
`server/etcdserver/server.go`. The model noticed the missing role but guessed
the nonexistent `server/etcdserver/etcdserver.go`. A bounded diversity pass,
one decaying third import hop, a generic package-role anchor, and an explicit
Raft component signal now retain `server/etcdserver/raft.go` and reduce
`rafthttp` to 11 paths at the product limit. The real `server.go` is retained
at the 150-path developer limit but is still not guaranteed at 60.

**Consequence:** file-level scores can spend a bounded request on many siblings
from one package before representing another central package. Raising another
magic score could repair etcd while moving the same failure elsewhere.

**Done when:** selection ranks a bounded set of packages using entrypoint
distance, import structure, and explicit local signals, then chooses a small
representative set of files per package. The five repository preflights must
retain their current useful paths, and the etcd 60-path fixture must contain a
real core etcdserver anchor without hard-coding that path. Current heuristic
regression tests are explicitly replaceable when this package-first selector
lands.

### TD-014: Focused flow retrieval is lexical before it is relational

**Evidence:** the captured `Raft Leader Election Flow` promoted `main` from
`server/main.go` into a global search term, selecting unrelated `etcdctl` and
example `main.go` files. Bare `election` and `leader` matches also mixed Raft
leadership with etcd's user-facing concurrency Election API. Package and edge
context was additionally computed from every positive lexical candidate rather
than only the retained bounded files.

The generic `main` term is now ignored and package/edge context is built only
from retained files. These fixes remove demonstrated mechanical pollution but
do not solve word-sense ambiguity.

**Consequence:** a valid model direction can still produce a plausible-looking
but semantically mixed reading list. Import edges shown beside that list are
not yet used as the primary retrieval constraint.

**Done when:** exact model seeds establish one or more package neighborhoods,
graph reachability is the primary inclusion signal, and lexical aliases only
rank files inside or near those neighborhoods. A fixture must distinguish Raft
leader election from the concurrency Election API without an etcd-specific
path blacklist.

### TD-015: Candidate direction choice has no local diversity or feasibility policy

**Evidence:** the orientation prompt asks the provider for runtime/event flows
but does not constrain their count, coverage, overlap, or whether the
implementation is inside the current repository. The visible three directions
exist because DeepSeek returned three. Confidence and ordering are also
provider-authored. One run proposed `Raft Leader Election Flow` even though the
election state machine lives in the external `go.etcd.io/raft/v3` module and
the current repository contains only the integration boundary.

**Consequence:** two valid provider responses can produce materially different
onboarding journeys, duplicate one subsystem, or invite a drill-down that the
available repository cannot complete.

**Done when:** local facts score direction feasibility and overlap, the product
has an explicit diversity policy, and external implementation boundaries are
visible before a direction is recommended. Keep the provider useful for naming
and interpretation; do not turn its self-reported confidence into static
evidence.

### TD-016: Unverified model paths are not reconciled with the full tracked inventory

**Evidence:** `allowed_paths` is intentionally only the compact model allowlist,
while the local snapshot knows every tracked path. The etcd report displayed a
nonexistent `server/etcdserver/etcdserver.go` as an unverified candidate and
repeated it inside a warning. The parser correctly prevented that path from
entering verified file fields, but the presentation could not distinguish a
real locally omitted file from a fabricated path.

**Consequence:** a fail-closed structured contract can still show a misleading
warning and make a selection failure look like a repository fact.

**Done when:** post-processing compares unverified paths and path-like warning
mentions with the full tracked inventory, labels real omitted files separately,
drops or clearly marks nonexistent guesses, and preserves a parser diagnostic
without promoting either category into verified flow evidence.

### TD-017: First-pass module discovery can spend time on tool-only modules

**Evidence:** Pebble has 1,416 tracked files and `git ls-files` completes in
0.07 seconds, but the snapshot runs `go list -e -json ./...` sequentially in
both the root module and `internal/devtools`. On a warm cache those calls took
2.31 and 1.11 seconds; the full snapshot took 3.99 seconds and bundle assembly
5.31 seconds. A cold `internal/devtools` graph includes CockroachDB, `x/tools`,
and static-analysis dependencies and can be much slower. The CLI previously
described this entire phase only as `scanning`.

The progress UI now names Go package collection explicitly and reports separate
repository-fact and compact-context durations. This makes the delay diagnosable
but does not remove it.

**Consequence:** the first onboarding impression can be dominated by a module
that does not participate in the product runtime. Blindly skipping nested
modules would break real multi-module repositories such as etcd.

**Done when:** the first pass classifies and prioritizes modules using local
facts, analyzes likely runtime/user-facing modules first, and defers tool,
example, or dev-only modules behind an explicit frontier. Cache identity must
include module files, Go environment, and repository revision; a user can still
request complete module coverage. Measure cold and warm Pebble runs before
adding concurrency or persistence.

### TD-018: Go 1.24 lacks a root-relative atomic rename API

**Evidence:** browser checkpoints use fixed internal names below an already
opened `os.Root`; reads, directory creation, and temporary writes remain rooted.
Go 1.24 does not provide `(*os.Root).Rename`, so the final atomic replacement
currently calls `os.Rename` on paths reconstructed from `root.Name()`. No
client-supplied path or session ID reaches this operation.

**Consequence:** the normal local threat model is bounded, but an adversary able
to swap the checkpoint directory concurrently retains a narrow TOCTOU window.
Raising the project's minimum Go version solely for this MVP would impose a
larger compatibility cost.

**Done when:** the supported Go baseline exposes a root-relative rename, or a
small platform-safe replacement preserves atomicity without resolving the
confined root back into ambient filesystem paths.

### TD-019: Teacher evidence selection is bounded but not question-ranked

**Evidence:** the Pebble `Batch.Commit` teacher bundle fit comfortably at about
27 KiB, but 25 of its 45 evidence items were static relations. Several were
unrelated fan-in from benchmarks, replay tooling, and tests. They survived
because the current compactor orders evidence by kind and deterministic source
order, not by the primary question or its onboarding/debugging/configuration
lens.

**Consequence:** the current DeepSeek case remained useful, but a larger or
weaker model request can spend attention and byte budget on callers that do not
reduce the selected unknown. Globally preferring "production" would also be
wrong for a testing or performance question.

**Done when:** the planner emits or the local pipeline derives one small
research-move enum, and the teacher selector ranks evidence for that move while
retaining a full inclusion/omission trace. Pebble lifecycle, Soft Serve startup,
and at least one test- or configuration-first case must demonstrate that the
same collector supports different rankings without separate engines.

### TD-020: Closed-world claim rejection is still an English lexical guard

**Evidence:** teacher prompt v2 explicitly forbade inferring absence from a
bounded static caller slice, but the model still wrote "does not call" and
"only used". The tolerant parser now drops that item while preserving eleven
grounded siblings, but it recognizes a small English phrase set plus evidence
scope qualifiers.

**Consequence:** a paraphrase or another language can evade the guard, while an
unusual but locally source-supported negative statement may require careful
wording. The rule is a useful fail-soft MVP boundary, not a semantic verifier.

**Done when:** normalized report items carry a locally derived claim scope and
support basis, the contract represents bounded absence/contradiction explicitly,
and evaluation rejects scope promotion structurally. Keep the lexical guard
replaceable and retain its real v2 replay fixture until that contract exists.

### TD-021: Flow proof cost omits compiler-internal package loading

**Evidence:** the restic proof reports the six tasks, four evidence files, seven
resolved symbols, and wall time that directly advanced its slots. The targeted
`go/packages` call necessarily reads and type-checks additional build-selected
files in `cmd/restic`, but those compiler-support files are not counted in the
saved `files` statistic. A cold local replay took about 25.5 seconds; a warm
replay took under a second.

**Consequence:** the worklist is bounded and the 90-second wall-time limit is
honest, but the file counter currently means "files admitted as proof evidence",
not "all files read by the Go toolchain". That distinction is not yet visible
in the report and could mislead performance work.

**Done when:** resolver results separately report evidence files, packages
loaded, build-selected syntax files, and measurable bytes/cache state without
adding source contents to artifacts. Keep wall time as the hard MVP guard;
avoid speculative parser optimization until those counters show a real need.

### TD-022: Canvas acceptance starts after realistic raw-run projection

**Evidence:** restic, Soft Serve, and Colima fixtures exercise the complete
saved canvas and browser interaction, while the raw
`BuildArchitectureCanvasInput -> ProjectArchitectureCanvas` path is protected
by a smaller synthetic run fixture. The showcase fixtures do not retain the
complete original repository facts and FlowProof session. Deterministic
no-model fallback is contract-tested but has no realistic screenshot. The
architecture synthesis record also owns request bytes/latency separately from
the older orientation request counter.

**Consequence:** topology and interaction regressions are well covered, but one
production wiring regression could escape the showcase fixtures and run-detail
request totals can be misread as including the later synthesis call.

**Done when:** one bounded realistic v2 run replays from raw saved facts through
canvas projection in a test, a no-model product artifact is inspected, and run
details either aggregate or clearly separate orientation and architecture-
synthesis request metrics. Do not check in a full repository dump to achieve
this.

### TD-023: Default surface discovery repeats Go package loading

**Evidence:** the default persisted Go run first loads bounded package facts for
the repository snapshot, then the isolated surface analyzer performs its own
`go/packages`/SSA load. The tiny worker fixture found two surfaces in about 139
ms on a warm machine. An etcd run built with repomap's Go 1.24 runtime initially
spent about seven extra seconds and printed dependency errors because the target
module requires Go 1.26. A root `go.mod` preflight now skips that incompatible
SSA load immediately with one report warning; the repeated run spent about 8.9
seconds on repository facts and 11.3 seconds total without stderr flooding.
Compatible cold/warm cost on Pebble and Soft Serve is still unmeasured. Progress
names the extra stage and reports its duration instead of hiding it inside
scanning.

**Consequence:** the useful local surface shelf may add noticeable duplicate
work on a large multi-module repository. Reusing an incompatible cache or
silently disabling analysis would be worse than the current explicit cost.

**Done when:** cold and warm compatible target-repository runs separate
package-fact and surface-discovery time, then demonstrate whether a shared
build-scenario cache or reuse boundary is worthwhile. Also decide whether a
released repomap binary must match the target module's Go language version or
can delegate this analyzer to the target toolchain. Keep
`--discover-surfaces=false` during measurement and do not merge analyzer
contracts merely to avoid a second load.

## Maintenance rules

- Add an item only when there is concrete evidence or a demonstrated gap.
- State the consequence and an observable completion condition.
- Do not use this file as a feature wishlist.
- Remove or move resolved items into the commit/decision that resolved them.
