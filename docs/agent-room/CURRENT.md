# Current product decision

Status: active living ADR. Updated: 2026-09-11.

[CONSTITUTION](../CONSTITUTION.md) takes precedence. This page records the
current decision and acceptance state; [AGENTS](../../AGENTS.md) routes work to
one authoritative contract per area. [CHANGELOG](CHANGELOG.md) records concise
implementation/check evidence. The former long ADR and agent instructions are
preserved in the [non-normative archive](../archive/2026-09-10/README.md), not
required reading or another source of current rules.

## Reader outcomes before optimization

A newcomer should quickly see what a component does, which requests and
commands enter it, which responsibilities run in the background, and which
other systems it contacts. They can then follow a useful scenario with
checkable source links and explicit gaps. Count source-backed observations;
several call sites do not necessarily mean several external systems.

| Outcome | Required evidence |
| --- | --- |
| Understand the repository and its parts | Complete selected component inventory, responsibility and ownership; unavailable analysis stays visible. |
| Run or use it | Native launch/registration identities, operations and attributed prerequisites/instructions. A documented command is not a verified run. |
| Follow a scenario | Trigger, participating parts, observed connections, result and unresolved steps. Reading order is not call order. |
| Understand data and concepts | Original declarations, owned fields/interfaces, storage/query observations and supported lifecycle claims. |
| Understand external communication | Participants/roles, direction, purpose, interface and original call/address evidence; unresolved configuration stays unresolved. |
| Configure behavior | Source-linked settings and their observed reads/defaults/conditions, distinguished from installed dependency documentation. |
| Understand failure and recovery | Supported error, cancellation, retry, cleanup and observable-result evidence; inferred behavior remains qualified. |
| Change and check it | Relevant implementation, tests/examples, checking instructions and missing evidence. Test discovery is not coverage. |

The source inventory and the initial explanation have separate completeness
requirements. A declaration can remain available without its own caption;
that alone does not establish a useful map or useful Learn questions. Evaluate
optimizations against preserved responsibilities, scenarios, operations,
integrations, concepts, anchors and gaps, then elapsed time, requests/tokens,
local work and disk. No percentage of closed code is an acceptance rule.

## Current decision

One ordinary pipeline consumes the shared corpus: native scouts and the target
portfolio select typed targets; real adapters produce sealed ProgramIndex
views; facts/claims and places feed atlas reading; GroupsIndex projects accepted
results; one owner report publishes all successful targets. Shared parsing and
stored facts preserve each target's native scope. Places materializes one target
at a time and reuses a consecutive shared input; it does not retain every child
index. Failed target outcomes and model-row refusals remain explicit.

- **Reading:** role/activation/outgoing selection is independent of captions and
  directory closure. Native boundaries and accepted operations survive missing
  prose. Questions retain original evidence, explicit coverage and independent
  results. Learn proposes from eight intents and uses the same answer path.
- **Operations:** v19 asks whether evidence supports this declaration's task and
  its independent activation (`self`/`none`). Process entry or asynchronous launch
  alone does not establish a responsibility; a task may delegate work to helpers.
  Caller/setup context is evidence, never a handoff choice. Persistent consumers,
  cron and supported one-shot scheduled work remain distinguishable from
  listeners, lifecycle hooks and middleware within the same request. Original
  own-call receiver/arguments/API distinguish effects on the request and response.
- **Boundaries:** v5 asks fixed native facts only for explanation and applicable
  closed address/destination fields. Candidate communication needs an accepted
  runtime interpretation; a remote client instance is distinct from an option
  passed to its later constructor. Source chains preserve correlated uses and explicit
  unknowns; imports, timers and internal delegation do not become integrations.
  Native listener addresses stay separate from HTTP routes. Shared source
  boundaries retain each target's original fact/declaration identity through
  atlas and GroupsIndex, including distinct same-line methods and paths.
- **Questions and orientation:** question-batch v3 assigns relevance per original
  anchor, allowing different roles inside one evidence chunk. Question and
  boundary evidence preserve safe native receiver/arguments/API and exact call
  sites; declared-interface identity retains unresolved dispatch observations.
  Orientation receives original member evidence too. Document heading
  ancestry preserves author scope without treating a model file-role hint as a
  native exclusion rule. Retrieval/answers distinguish task responsibility from
  HTTP hosting and preserve differing call arguments; launch recipes retain
  supported required arguments. Implementation-body retrieval is not added.
- **Execution:** shared reasoning/output allowance is 128,000 tokens, subject to
  the provider ceiling. Output refusals partition independent questions first;
  input/context refusals preserve and repartition complete evidence. A new
  limit/packing policy is tested on one saved complete window before a full run.
- **Glossary:** only accepted prose enters a separate p-ref generation/reduction
  pass, now using that same shared output allowance. Source/prose/fill context
  stays local in Prepared and the accepted cache record; no deferred g-ref
  appendix is sent. Original scope survives current warm execution, entity and
  question memos, and exact replay. Reduction v5 factors repeated source anchors
  and complete source sets into request-local catalogues. Every original
  definition and scope survives, including each independently prepared child.
  Both owners use the shared exact-request split memo. Exhausted provider-local
  timeouts remain optional refusals while the run itself has not been cancelled.
- **Report:** compact named inventories precede maps, with five initial rows and
  All N retaining the remainder. Calls, workers and source-linked data must be
  easy to find. Code names/paths/addresses remain original; English aliases and
  operation labels coexist with localized descriptions. Saved rendering uses
  ordinary templates and no provider or cache. Detailed display/data changes
  retain their consuming contract and require fresh ordinary acceptance. Data
  links use existing exact model/callable ownership; an unresolved endpoint or
  unowned SQL literal cannot acquire a database flow in the report.

## Contracts and formats

Commands/orchestration: [Surface](../contracts/SURFACE.md). Corpus, author scope
and targets: [Discovery](../contracts/DISCOVERY.md). Native materialization and
source expressions: [ProgramIndex](../contracts/PROGRAM_INDEX.md), with native
[Go](../contracts/GO.md), [Python](../contracts/PYTHON.md) and
[JSTS](../contracts/JSTS.md) contracts. Semantic stages:
[Reading](../contracts/READING.md). Provider/cache:
[Execution](../contracts/EXECUTION.md). Definitions:
[Terminology](../contracts/TERMINOLOGY.md). Display:
[Report](../contracts/REPORT.md). Facts/SQL: [EXTRACTORS](../EXTRACTORS.md).
Required checks: [Development](../contracts/DEVELOPMENT.md).

### Formats

Current shapes are defined by their owning code, not historical run headers:
[ProgramIndex](../../internal/programindex/index.go),
[graph/atlas](../../internal/atlas/atlas.go),
[reading input](../../internal/atlas/reading/input.go),
[GroupsIndex](../../internal/groupindex/index.go),
[report](../../internal/report/report.go),
[manifest](../../internal/report/manifest.go), and
[accepted cache](../../internal/llm/cache.go).
This wave uses graph 14 / saved reading input 15 / atlas 7 for native boundary origins and distinct listener addresses, in addition to heading context and original dispatch observations,
operations v19, boundaries v5, question-batch v3 and local cache record v3.
Native adapter constants remain in their owning language packages; genuine
fixtures are regenerated through those adapters when their format changes.
There are no old-format readers or manually rewritten seals.

## Acceptance and open work

| Scope | Current status |
| --- | --- |
| Local corrected contracts | Focused tests/vet pass for operations/boundaries, local glossary provenance and consuming memo paths. Full combined check/build belongs to the current integration checkpoint. |
| Saved Watchtower glossary window | One controlled output-budget probe completed: same 433 rows/727 texts, 21,040 input, 8,903 output tokens, 28.803 s, finish=stop. 216 term rows, 211 exact occurrences; five unsupported optional names discarded. Only structure/refs/text checked; original-source provenance still needs ordinary acceptance. |
| Saved Watchtower reducer window | The complete 458-variant input shrank from 4,733,303 to 502,076 provider bytes by factoring source scopes. One replay completed in 15.503 s (155,669 input / 5,502 output tokens); current validation accepted 412 groups retaining all 458 original variants and their exact sources/origins. This checks packing and restoration, not every semantic merge or a new ordinary report. |
| Saved boundary contract windows | One issue-bot replay retained its client constructor and five GitLab dispatches while rejecting the option factory; one complete service window retained OTLP construction and HTTP dispatch while rejecting local delegation. Current v5 decoder accepted all rows. This checks those source distinctions only; destinations, answer quality and full ordinary reports remain separate acceptance. |
| Saved issue-bot retrieval response | Current per-anchor validation accepts 21/21 original rows (previously 18/21); offline response validation only. |
| Syn / issue-bot / Watchtower ordinary reports | Third series completed in 225 / 575 / 307 seconds, with 30 / 20 / 27 questions and no unavailable answers. Native routes, operation activation, remote client/option distinctions and glossary provenance were checked. Watchtower's worker answer and required launch argument, issue-bot's per-call argument exception, and a mixed route counter still needed correction; current follow-up checks remain pending. The issue-bot glossary exhausted 128k output before lossless splitting. |
| Freqtrade | Latest larger run stopped after repeated 16k retrieval refusals; no accepted final report for this wave. A corrected ordinary run with worker, destination, question and data checks is required. |
| Airflow | Full current ordinary acceptance remains pending. Do not restart before prerequisite fixes, saved-window checks and Freqtrade acceptance. Old elapsed time is not a measurement of the new builder. |

Artifact consistency, a green fixture, a saved reading and a successful single
window each establish their own limited evidence. None alone establishes model
answer quality, full repository completeness or end-to-end performance. Update
this table after inspecting the corresponding ordinary result; preserve receipts
and limitations in the journal/archive rather than accumulating them here.
