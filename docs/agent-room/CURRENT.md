# Current product decision

Status: active living ADR. Updated: 2026-09-17.

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

One ordinary pipeline consumes the shared corpus. The canonical target plan
assigns `t1..tN` once; that same identity passes through selected outcomes,
ProgramIndex, facts, GroupsIndex and report joins, including failed targets.
Native adapters contribute to the same compact fact identity scheme. Adapter
`SourceRef` values are construction keys only and never persist. Within a target,
objects use `n*`, relations use `e*`, and nested facts are scoped below those IDs.
Facts own `a*`, human claims own `h*`, operations own `o*`, and the group overlay
owns `g*`, `k*` and `x*`. Provider requests reuse these IDs directly under a
closed allowlist; cross-target graph refs are merely qualified (`t1.g1`,
`t1.n22`), not renumbered. Only choices that do not exist in an artifact use
temporary `c*` refs. Natural ordinal order is mandatory, so `t10` follows `t9`.
The repository index SHA binds every derived decision; failed target outcomes
and model-row refusals remain explicit.

Atlas tables obey the same rule: provider row keys are their existing compact
place/target/joint/question IDs, never a second `r*` numbering. Question IDs are
assigned once from canonical question order and survive presentation reordering.
Symbol selection and operation review use the symbol's `s*` ID instead of
`selection:s*` / `operation:s*` pseudo-places. Atlas arrows receive persisted
compact `x*` IDs before model reading instead of joined box-pair keys.
Exact-input memo fingerprints omit owner identity entirely rather
than replacing it, so identical evidence can still share one accepted response
without exposing another name to the model.

The active simplification removes duplicated fact graphs from later artifacts.
ProgramIndex owns deterministic facts; the reading result owns model annotations
that reference those facts. Places, atlas and GroupsIndex are migration names,
not permission to persist partially self-contained copies of objects, relations,
paths or structural edges. The final repository artifact contains the fact graph
once and annotations once. `report.json` embeds the same ProgramIndexes directly,
not a presentation-specific graph, and stores GroupsIndex as the same thin
semantic overlays. Native group subjects and structural edges exist only as an
in-memory join while the HTML is built.

GroupsIndex v12 is now that thin overlay: `g*` groups, `k*` containers, `o*`
operations and `x*`
connections are deterministic target-local ordinals over canonical content;
subject rows contain only `n*`/`e*p*` refs, categories and interpretations.
Reading it requires the exact bound ProgramIndex and derives native subject facts
and structural edges in memory. No semantic ID is a content hash.

ProgramIndex v16 removes the unused symbol-link hash layer. An object has its
one compact `n*` identity inside a target; the shared scope refers to it as
`t*.n*`. Cross-target meaning exists only in explicit `x*` connections backed
by those existing facts. There is no adapter-specific hidden join key to mint,
persist, copy into GroupsIndex or expose in `report.json`.
Report hydration uses that same target-qualified identity; local `n*` values are
never entered into or read from a repository-wide unqualified map.

- **Shared fact vocabulary (ProgramIndex 17):** every fact is written once
  (no `contains` relations, no derivable counts or empty fields). All adapters use
  one closed `invocation`/`dispatch` vocabulary and one resolution rule: one known
  target is `exact`, several `alternatives`, none `unresolved`. Go signatures use
  short package names, types show only their form, and struct tags become object
  `aliases`. Go resolves interface values it observed, including external types
  such as `*sql.DB`.
- **Registrations (2026-09-17):** the fact layer names no framework. A
  `registration` is the shape of a non-owned call handing over a callable,
  address or named value; the reading stage classifies it (`http_server`,
  `queue_consumer`, `scheduled`, `interaction`, `extension`, `http_client`,
  `db`…) and operations, routes, client calls and portals follow from accepted
  decisions. `sql_query` is a fixed `db` boundary. Without a model there are
  candidates, not routes; the Echo preset test is the offline acceptance.
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
  pass with its own 32,768-token output allowance: legitimate windows need 1–16 thousand tokens, and two Watchtower windows looped to 128,000. Source/prose/fill context
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
This wave uses ProgramIndex 17, places graph 16, reading input 18, atlas 10,
GroupsIndex 12, dependency catalog 2, extraction artifact 2, facts 3, claims 2
and target outcomes 3 with compact artifact-local IDs and no saved adapter
SourceRefs. Dependency catalogs assign canonical `i*` importers and `d*`
dependencies. Accepted extractor nodes use `u*`; arbitrary public-protocol IDs
remain only in the exact recorded stdout.
The current reading input and atlas carry the current semantic result with
target-qualified native object refs and short graph/part/area/joint IDs, in
addition to heading context and original dispatch observations,
operations v19, boundaries v5, question-batch v3 and local cache record v3.
Question routes are v10 and knowledge is v3: both retain the same compact
question/place IDs used by provider rows.
Native adapter constants remain in their owning language packages; genuine
fixtures are regenerated through those adapters when their format changes.
There are no old-format readers or manually rewritten seals.

## Acceptance and open work

| Scope | Current status |
| --- | --- |
| Local corrected contracts | Full `go test ./...`, `go vet ./...` and `make build` pass. The standalone Echo/sqlc/Cobra module passes its own test/vet. A fresh ordinary no-model API run completed with browser-checked handler → converter → service → repository → sqlc navigation; its saved dependency/extraction refs and report DOM IDs are compact and unique. |
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
