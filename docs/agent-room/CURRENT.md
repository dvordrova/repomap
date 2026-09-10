# Current product decision

Status: active living ADR

Last updated: 2026-09-10

Historical provenance: pre-cleanup commit `4e54ab3`

This is the only current architectural decision record. Change the affected
section here when the ordinary product path changes. Older decisions and
planning notes are history in Git, not current requirements.

## Required analysis results before optimization (2026-09-10)

The owner requires defining what information repomap needs before choosing how
to obtain it more cheaply. This working specification makes the existing
constitution and eight Learn intents concrete. It does not implement a new
selection policy, authorize excluding target roles, or claim that the current
pipeline satisfies every outcome below.

The result lets a newcomer locate relevant code, understand its responsibility,
and follow a useful scenario with checkable sources. Completeness concerns
those outcomes and the explicitly selected repository scope. It does not
require a separate model-written sentence for every indexed declaration.

### What information the result needs

| Reader outcome | Required information | Evidence and sufficiency check |
| --- | --- | --- |
| Understand the repository and its parts | Component identities, purpose, role, ownership, the division of responsibilities, and where to start for a concrete task. | Account for every eligible selected target, including tools, examples and shared code. Bind interpreted responsibilities to declarations, manifests or attributed author documentation. Unavailable analysis remains visible. |
| Run or use it | Exposed commands, requests, interactions and background activities; their implementations, prerequisites and documented ways to try them. | Preserve observed launch and registration identities and exact anchors. Explain an operation once with its source; a thin entry wrapper need not have another caption if that explanation already covers it. A documented command is not a verified successful run. |
| Follow a useful scenario | Its trigger, participating parts, their responsibilities, observed connections, result and unresolved steps. | Every asserted connection needs its own supporting observation or explicitly attributed documentation. An ordered reading route is not proof of runtime execution; a missing connection remains a gap. |
| Understand data and essential concepts | Meaning of the concepts needed for the scenario, relevant fields/interfaces, storage and consequential lifecycle rules. | Declarations and owned members establish structure. Author prose can describe rules with attribution. Names or signatures alone do not prove mutations, persistence or runtime state transitions. |
| Understand integrations | Both sides when known, direction, protocol or interface, transported values, and possible or unresolved matches. | Retain original call sites, literals and endpoint observations. An imported library alone does not establish remote communication. Do not lose an observed boundary because its containing file lacks a prose description. |
| Configure or vary behavior | Settings and build choices that affect use, where they are supplied/read, and supported defaults or conditions. | Source-linked manifest values, reads, declarations and author instructions. Distinguish a declared setting from evidence that an operation uses it. |
| Understand failure and recovery | Evidence of errors, cancellation, timeouts, retries, cleanup and what the user can observe along the selected scenario. | Describe only supported behavior. A suggestive name is a lead for investigation; absent body/runtime evidence leaves a specific gap. |
| Change and check behavior | Relevant implementation locations, associated tests/examples, instructions for checking a change, dynamic execution and known missing evidence. | Keep test/configuration/example sources discoverable. Test discovery establishes an inventory, not tested behavior or coverage. State the scope of negative findings; lack of a model answer cannot establish absence. |

Each explanation keeps its subject, exact source locations and component
context. Facts, author claims and model interpretations remain distinguishable;
interpretations retain qualifications and gaps. Shared source identity may have
several owners without creating several copies of the declaration or erasing
its distinct uses. These are information requirements within the existing
ProgramIndex, atlas, knowledge and report, not new storage schemas.

There are two different completeness questions. The source inventory must keep
the eligible evidence required for later investigation. The initial explanation
must cover the responsibilities and scenarios it presents. A source can remain
available without a dedicated caption; availability alone does not prove that
the initial map or automatically proposed questions introduce it adequately.

### What the current implementation actually obtains

The ordinary symbol stage now separates discovery from display prose:

- `SymbolSelection` reviews every eligible candidate's key-symbol choice,
  activation and outgoing calls. A types-only row asks for the key choice.
  Accepted directory/file `open=no` decisions do not suppress these rows.
  `unassessed` preserves insufficient activation evidence; it is not `none`.
- `Symbols` and `Types` then explain only the model-selected keys that the
  existing overview displays: at most five ranked keys per file and three per
  box, with each selected source described once across owners. Selection uses
  original documentation before captions can affect the display order.
  Refused selection never supplies a caption through the native ranking fallback.
- Selection and caption have separate exact-input memos and knowledge records,
  both bound to the original subject and file context. A refused caption cannot
  delete accepted activation, outgoing-call or key choices. `readOperations`
  still independently reviews proposed and observed callback/async activations
  and writes operation names and descriptions. Native boundaries stay independent.
- `learningEvidence` uses accepted key-selection knowledge even when the symbol
  has no caption. Explicit question retrieval retains the full original graph;
  final answers already read original evidence without buying a description
  first. Question-only runs recall both kinds of knowledge without new selection
  or caption requests. The static report never calls a model.

This supersedes `ec981a11`'s broad closure of symbol rows. Its 52,393 skipped
rows were an impact count, not proof that the omitted roles were unnecessary.
The replacement regression checks persisted selection records directly,
including closed files and ancestors, rather than the empty atlas of a partial
reading. It also checks refusal isolation, non-key operations, original question
sources, replay and exact-input reuse.

The saved 438-target Airflow graph has 76,776 eligible selection rows, 2,405
boxes and 1,823 distinct box/target candidate pools after restoring the saved
file placements. The existing three-key display rule bounds captions by 4,601
for these pools; this is an upper bound before model selection, not an observed
caption count. Only 1,722 candidates have the existing callback/async hint, which
is not evidence that all other activations are absent. Broad role selection
therefore remains in this first change. These counts do not promise a runtime
ratio or acceptance of the newer 457-target graph.

Python HTTP facts now follow an observed single base-class chain to an external
framework method, stopping at a local override, missing base or multiple bases.
No native call edge is invented. TypeScript uses the compiler-resolved original
external class method; a local override keeps its local origin. The cumulative
Go fixture checks its distinct language equivalent, promoted methods through
embedding, with no extra Go production rule. All three retain source anchors
and contrasting local methods. The saved Airflow core index yields 120 production
`api_fastapi` route facts after this change (148 including 28 test routes); completeness and composed prefixes
still need separate acceptance. The earlier claim that multiline decorators
were lost was false: the Python adapter already reads their AST.

The obsolete full Airflow attempt was terminated before atlas reading; its
saved indexes, facts and model cache remain. Full acceptance is pending.
These local changes do not
reduce the measured 33-minute native preparation or 17-minute facts stage.

The ordinary online check on `python-tutorial-game` at `78714d34ee` completed
both targets with 121 symbol/type candidates and 21 captions, 24 questions and
419 translated display texts. Its 285.594-second wall time was dominated by
question answering; this is evidence of successful publication, not a measured
speedup over the prior run. The generated Russian report was inspected through
an original question, its concept explanation and its linked operation map.
Warm inspection also exposed a terminology reuse defect: a single explanation
with several sources became several variants when restored through individual
row memos. The collector now accumulates accepted fragments under that original
term identity. Refused row sources remain excluded; different original meanings
remain separate. This preserves the same glossary input for batch acceptance
and row-by-row reuse. The subsequent ordinary run completed in 7.388 seconds
with no new provider calls, including glossary and all eight translation
windows. Its saved artifact consistency checks passed. The current binary's
`cache clear` also removed a temporary copy of real response, payload and memo
files while retaining the publication and the original cache. Full tests, vet
and the ordinary binary build passed; the warm time is not a cold Airflow
estimate.

A12 corrected that acceptance: the saved atlas held eight frontend operations
(seven interactions and one continuous activity), but GroupsIndex projected
only the one whose symbol had a caption. Checking the atlas and clicking the
surviving operation missed this downstream loss. The projection now retains
every nonempty interpretation independently of its caption, and its validator
allows an absent caption. The regression test checks uncaptioned non-key
actions, background work, keys and aliases, retaining original operation names,
summaries and source anchors without inventing operations for other fields.
The correcting ordinary run at `ac15b95a` completed in 18.182 seconds. All eight
frontend operations retain their exact names, summaries and locations in the
atlas, saved GroupsIndex, common report and HTML anchors. Browser inspection
confirmed seven interactions plus one background activity on the first screen
and the previously missing non-key resize operation's map and source at
`simulation_field.tsx:114`. Analysis reused saved responses; four translation
windows were regenerated for the restored visible text. Full tests and vet
passed. This replaces the earlier operation-preservation acceptance, without
changing the unresolved full-Airflow performance or route-coverage limits.

### How to judge an optimization against these results

For each expensive computation or request family, first identify the required
information it produces and its downstream reader outcome. Then evaluate
removal, reuse or batching against that dependency. Share identical evidence
and exact accepted answers while preserving source and ownership context. A
separate caption is justified by the explanation it adds; selecting an
operation, boundary or learning concept has its own information requirement,
even when the implementation currently obtains all of them in one request.

A structural observation is not an importance decision. A project-configured
test inventory helps distinguish tests from production declarations; it does
not justify discarding the evidence needed to explain how to check a change.
An exact compatibility redirect can explain where an API moved, whereas a
short file, `internal` path or an Airflow-specific helper name is not a general
exclusion rule. The existing requirement to analyze tools/examples remains.
In the reviewed Airflow snapshot, `pyproject.toml` configures `python_files`
as `test_*.py` and `example_*.py`; copying pytest's default filename patterns
would not reproduce even that declared selection. Pytest explicitly supports
[configured discovery patterns](https://docs.pytest.org/en/stable/example/pythoncollection.html#changing-naming-conventions).
This configuration observation is not an executed test collection or a proof
that every declaration in those files is irrelevant to orientation.
Any later eligibility rule must specify its evidence and effect before request
preparation. A response violating that declared rule is recorded as refused,
without rewriting the original answer into a model-approved positive decision.

Compare the same snapshot by preserved launches, operations, integrations,
concepts, useful question topics, original anchors and explicit unresolved
coverage. Counts alone are insufficient: inspect which subjects disappeared
and whether their responsibilities remain explained elsewhere. Then compare
elapsed time, provider tokens/requests, native preparation work and disk use.
No percentage of closed code or file-length cutoff is an acceptance rule.

Saved Airflow input permits inspection without another AST build. It does not
make two complete model readings free: the verified saved run ends after files,
and complete symbol, operation and Learn answers have not been established in
the cache. Changed Learn evidence also changes its exact request. Establish
available baseline answers and missing work before calling a comparison cheap;
an offline comparison can establish structural impact, not the quality of model
answers that were never obtained.

## Removed on 2026-09-04

The owner's rule: limits are not wanted and the tool is not a security
boundary. Gone, with their tests: the scale-warning system in eleven
packages, the manifest verification suite and run receipts, RunAuthority
and the captured-input allow-list, the publication assessment and the
quarantine of failed runs, the workspace snapshot / source catalogue /
workspace-open chain behind the report server, the page-size limit, the
credential scanner, the test/limit/prompt inventory contracts, the
clientrecipe experiment and its fixtures. The run manifest is a record of
where a run came from; one `report.Generate` replaces the twenty named
generation variants; the server reads a run directory and serves it. Where
a section below still describes verification, receipts, thresholds or
allow-lists, it describes history.

Added the same day: console events carry timing, every exchange record
carries `latency_ms` and token counts, and a run closes with a `Time`
stage per target; the page shows the authors' docstrings on cards, the
README's own overview and quotes from every README, and gains a find box
that exists only with scripting; the map's hover card names a group's key
symbols and its arrows as sentences and the main path through a target is
a trace; "What is missing" names LICENSE, CONTRIBUTING, CHANGELOG and a
linter configuration; the page says how long the whole run took and
where. Group matching reads incidence from indexes built once — repomap
on itself compiled its 233,047 cross-target pairs in 42 s where it had
spent seventy CPU-minutes without finishing.

## Bottom-up iteration (2026-09-05)

### Current goal: a comprehensible whole page (2026-09-08)

The owner explicitly narrowed the current goal to the composition and
navigation of the whole report: the finite repository must stop feeling like
an endless succession of canvases and explanations. This continues the prior
Learn/Work work below; it does not reopen the truth audit of all model prose.
The chosen implementation stays in the ordinary HTML templates and reads the
same existing report. No provider requests, semantic graph or analysis path
are added for these UI changes.

The repository map opens as a complete set of component cards. Each keeps its
native name, role, path, full existing purpose, manifest/entrypoint anchors and
all incident connections. The same SVG and exact connections remain under an
explicit Connections control. Role controls filter that one level; a role is
not another parent. Source context makes a generic Python library's setup.py
boundary visible without locally rewriting its accepted description. Long
source paths wrap inside their card. No component is removed or reprioritized.

The component card's Parts count and missing-parts coverage use the same
local leaf groups as the map, across all lanes. Operations, containing areas
and foreign component nodes do not count as additional parts. Its Parts link
opens the complete component map. The separate core-lane reference catalogue
is labelled Core; an empty core lane cannot imply an empty component map.

The 2026-09-10 entrance correction makes every existing request, command,
scheduled/continuous activity and interaction visible by name on that row and
before the component map. Static grouping uses the existing activation kinds;
it neither classifies new workers nor adds a translation request. A route keeps
all operation links joined by its exact FactID and its original code link;
an ungrouped route gains no invented explanation. The operation picker is a
complete searchable grid. Selecting an operation opens all its original leaf
parts and connections at once. Opening a part keeps that operation diagram,
its node order and viewport above the code reading, rather than replacing it
with immediate neighbours. Entering a component does not select main or the
first part automatically.

Saved-render verification used the complete current-format Python/TypeScript
run `20260909-200202-python-tutorial-game-a06614d5d228` and chi run
`20260909-144532-chi-8da44377446a`, with zero provider calls and unchanged
translation catalogues. Browser checks retained all 12 and 35 input rows,
respectively, exact route destinations, the full operation graph and viewport
when opening a part, selected code across Learn/Work, and Back navigation.
The complete operation picker filters without horizontal paging. A regression
uses a longer chain and two source-distinct relations between the same parts
to prevent both one-hop collapse and duplicate entry relations. `make test`
with the installed TypeScript compiler, `make vet`, `make build` and the final
report-package checks passed. Receipts: `work/ui-entrance-20260910/checks.json`.
This UI check does not claim completion of the separate full Airflow run.

Learn exposes the complete saved question menu, one open answer, its position
in that menu, a next question and an explicit return to all questions. The
current question remains visible while scrolling or taking a map side trip.
Work instead offers the existing repository search and an exact component
search entrance. Mode changes retain map scope, operation, zoom, selected code,
search and reading. Component Learn entrances use only the saved answer's
exact map links. Inline terms now link directly to their existing map
memberships and select that declaration by its source, without a catalogue hop.

Opening a part reveals its key code as cubes inside its original group frame.
These read existing interpreted concepts and group highlights; a part without
either can show its original declaration index with no invented explanation.
Equal names with different source locations stay separate and show those
locations. There is no automatic first term or operation selection. Clicking
a cube keeps its explanation below the map; hover only emphasizes neighbours
or exposes the original brief title. Group arrows remain group arrows. The
path explicitly names component, area, part and selected code; root/back
actions address the corresponding scope. Existing full target evidence stays
in a native disclosure in its original order. No-script source links and
sections remain available.

Ordinary final runs `20260908-052654` completed on Jieba, chi, the small Go server
and Python/TypeScript: 30 of 30 targets and 44 saved questions, with zero live
provider calls and zero transport attempts, including translation. Browser
checks cover finite question returns, direct term-to-map selection, retained
code and zoom across Learn/Work, component sources and exact connections,
root/area/part Back/Forward, and the named map return from full details.
Re-entering a component from Home resets to its component entrance; a named
map return retains its precise scope and selected code.
Long source links fit their cards; duplicate code names retain their source
in the toolbar. Static HTML checks preserve 490 source excerpts and all
external source links. Tests, vet and the owner build pass. Continuous pointer
hover was not measured by the available browser automation API.
Final receipts and report links are in `work/ui-whole-page-acceptance.json`
and `work/repomap-review-current.md`. The owner still reviews the resulting
composition; implementation and browser checks do not prove human
comprehension. UX32–UX38 and all prior importance columns remain in the review
documents. Member arrows are still an alternative, not a shipped inference;
very narrow windows remain deferred by the owner.

### Prior goal and acceptance history: finish Learn/Work

On 2026-09-07 the owner asked task
`01a07a8c-53dd-7f33-8614-d726b350680f` to inherit the previous task's still-active
goal, whose final wording was “доводим систему learn/work до ума”. Continue
from the existing implementation and acceptance evidence. Learn should provide
a useful introduction through the system map, questions, source-anchored answers
and terms. Work should support investigation from either a subsystem or an
operation, retaining context while drilling into its connections and sources.
The operations/matching milestone below remains part of this work.

The owner is reviewing `repomap-questions-learn-work.md` and
`repomap-ui-ux-review.md`. Their 109 questions are a prioritization catalogue,
not a promise to implement every distant capability. Keep the recorded UI/UX
remarks until their acceptance journeys pass, and incorporate the owner's
subsequent importance ratings. The small-report browser check has verified the
continuation button for long inspector explanations (UX17) and preservation of
the selected explanation and its scroll position when switching to Work. Source
checks now avoid repeating the enclosing declaration's link while preserving
each member's exact source (UX29). Current full etcd Learn/Work acceptance,
reliable matching and the remaining navigation journeys are still open. The
ordinary 27-target run `20260907-083529-etcd-9d31f0a919d8` completed in
1 hour 42 minutes 29 seconds. All 27 targets were analyzed, with one common
publication and matching native/group and saved graph/question bindings.
Its 24 questions have 13 answered, seven partial and four unavailable results.
It predates the output allowance, answer/type contracts and later UI changes,
so it remains a baseline. Current-binary ordinary run
`20260907-101952-etcd-720ad79ed333` terminated after 55 minutes 1 second
with all 27 target pages analyzed but no common report or manifest. The
provider began returning HTTP 402, "Insufficient Balance"; the final orientation
request failed, so publication failed. Its saved 17 questions contain four
answered, one partial and 12 unavailable results. Successful requests remain
cached. The process ended by itself; it was not interrupted or restarted.
Online acceptance resumes only after provider access is restored. The log is
`work/learn-work-etcd-current-binary.log`. The incomplete-run receipt
`work/learn-work-etcd-current-binary-incomplete.json` verifies all 27 native,
group and index-set bindings and the saved graph. It records 3,029 accepted
table results and 1,873 refusals, including 1,865 HTTP 402 refusals; the shared
page/outcome portfolios and publication are absent. This is a terminal
incomplete run, not a passed large-report acceptance.
Ordinary runs and direct browser/source checks determine completion;
passing tests or a historical small-repository screenshot alone do not.

The owner's UI review method, clarified on 2026-09-07, follows a pair of real
questions through the visible interface. Choose actions by their visual
affordances, search first among prominent elements, then explore beside the
relevant item. An unfamiliar term starts a side question. After navigation,
assume the reader remembers learned terms but has lost their place; the page
must help them orient again and start the next question. Record these journeys
in `repomap-ui-ux-review.md`; DOM existence checks do not substitute for them.

The owner subsequently clarified that the UI/UX council should consider
overlooked ways of displaying and navigating the report, not challenge the
truth of all generated information. Compare composition, detail placement,
term side trips and visible context after a break. Preserve the completed
content audit separately; it is not a mandate to expand that audit. The owner
explicitly kept the current TypeScript field-loss fix in scope and requested
comparable TypeScript, Python and Go examples in the cumulative testdata.
The owner also made this a standing development rule: each language-cube
behavior change must be captured by an understandable source example in its
one cumulative language repository and an executable expectation over the
real language path. Existing exact coverage can be reused; unrelated green
tests or documentation alone are insufficient. Go interface declarations are
already represented by generic TicketContract.Cancel/Status plus embedding
and alias counterexamples in the cumulative Go fixture.
This rule applies across languages: a case discovered in one language requires
checking and recording its equivalents in every other supported language
without another owner prompt. Respect the native semantics; record a missing
equivalent instead of inventing one. An already-correct adapter needs the
regression example, not an unnecessary production change. The count-field
case demonstrates this: TypeScript and Go required fixes; Python required
only the comparable fixture and executable expectation.

The next display review has three comparison candidates: map with one reading
pane, wider reading with a compact coordinated map and inline term expansion,
and the same map with named returns through explicitly opened details. These
are local mockups over frozen report text, not a second product presentation
path. The owner's additional ideas are recorded as UX32–UX36: member cubes
inside groups with hover preview, text below the map, member-level arrows,
misleading grab/grabbing cursors, and the composition of small diagrams.
The existing report's cursor mismatch was reproduced when a hidden map first
initialized with zero width and later fit entirely. Pan eligibility now uses
the visible stage's actual horizontal or vertical overflow, recomputed on
stage/SVG resize and zoom. The same check controls the cursor, drag hint and
background pointer capture; controls remain clickable. Pointer identity and
up/cancel/lost-capture cleanup prevent an obsolete grabbing state. An independent
Chrome walkthrough of a UI-only copy of ordinary report 121841 verified
Home-to-backend, zoom, a real 220px drag and release, reset/fit and node/control
clicks. Report tests and the owner build passed. This is not a new ordinary
online acceptance run; narrow/vertical/canceled-drag cases and the held cursor
appearance remain unverified. Priorities and display choices remain for the
owner's review.

The second display mockup compares reading below, beside or above the map,
compact small-map composition, member previews and optional inner arrows.
Wide desktop walkthroughs covered below/side reading, an inline Settings side
trip and field.py → imports → source with named returns to the question.
The member arrow uses the frozen report's exact field.py-to-Robot import at
field.py:6; the group-level model connection keeps its own endpoints.
Preview preserves the current reading but expands the group and can move
below-map reading down. These are comparison results, not product acceptance.
The owner subsequently deferred very narrow windows (“ну прям узкие окна меня
не интересуют пока”): prioritize ordinary wide desktop composition and
navigation; keep earlier narrow-window observations as history.

The owner added a separate return-after-a-break check: after leaving and
returning to the tab, can the reader tell both where they are and what they
were doing? Evaluate the current screenshot with the click history forgotten.
Retained UI state alone is insufficient. UX31 records this for both Learn
side questions and Work investigations; its importance is still for the owner
to assess. Do not infer an unrecorded user intention from the selected object.

A later wide desktop check over the UI-only 121841 copy confirmed the actual
Learn source round trip: launch question → Settings → GitHub settings.py:4 →
the unchanged report → map → Work → the expanded original Learn answer.
The question, term and named returns remained visible at the relevant stops.
An independent Work check selected frontend animate and Area/Part Interactive
components, zoomed four times and dragged the map 260px. Reading the full
Interactive components section, following Utility helpers and using Back to map
preserved the exact selected operation, scope, SVG dimensions and scrollLeft=260.
The return label named that destination before the click. These close previously
unmeasured source/pan branches, not human comprehension, vertical pan, browser
history restoration or current-binary ordinary acceptance. Notes are
`work/ui-ux-wide-source-return.md` and `work/ui-ux-wide-pan-return.md`.

A wide desktop pass over the frozen 083529 etcd report with current CSS/JS
confirmed server → EtcdServer.run → Storage engine → Compaction logic →
API and networking. Operation paging preserves that selection; All uses
leaves the operation while retaining the area, then Back returns to Component.
At a deeper page scroll, however, the operation and area/part trail disappear
above the viewport while the map and sticky inspector remain visible. The
existing sticky reading-location now also links to the map's current Structure
or Operations view, with its selected/preview operation and area/part trail.
It updates on that visible map's committed layout, separately from a hover
subject or an explicitly opened Full details section. Following the link
restores the current map explanation and scrolls to its controls without
redrawing the map. A wide 1280×720 browser pass confirmed the operation/path
above a scrolled map, and an exact round trip with a nonempty Compaction filter,
four zoom steps and scrollLeft=260. All uses removes the operation from that
line while retaining the area/part. This addresses the specific disappearing
path, not the broader question of understandable levels. The original pass is
recorded in `work/ui-ux-etcd-wide-operation.md` and the follow-up in
`work/ui-ux-etcd-location-root.md`; their
Go-rendered HTML and analysis remain frozen, so it is not current-binary
ordinary acceptance. Apparent duplicate Revision term labels in that old HTML
are already disambiguated by current Go rendering and do not justify another
product fix. Current ordinary chi/server/Python+TypeScript checks and their
remaining owner review items are recorded in the report section below.

The owner subsequently clarified that the levels themselves are confusing:
“Root library”, and even the Python report's “front → backend” overview,
do not explain a useful system structure. Evaluate actual cube clicks on
different repository types, including a library and a small server from
`~/git`, before treating a preserved breadcrumb as success. In the frozen
Python report, front → Area Page views → Part Page views repeats the same
name and graph before exposing the code link. In frozen etcd, Root library
opens an empty application-shaped sequence (no HTTP routes, no entrypoints,
no core groups) rather than a useful library introduction. Root comes from
the renderer's substitution for the directory `.`; Kind is separate technical
metadata. A title-only rename does not resolve these display-level problems.
UX37–UX38 retain the owner's feedback and the actual screenshots. These are
presentation observations, not a reopened truth audit of all generated text.

The owner raised rapid DeepSeek spending and asked to pack questions and
categorization decisions into the largest useful requests, including comparing
thinking for a whole batch. The completed batching changes and measured
checks are recorded below; UI findings remain in the owner checklist. Paid
calls were paused during the initial audit.
The interrupted Python run is described below. The full WORK audit found
17,317 live calls, 143.22M input and 7.54M output tokens, excluding 9,443 local
cache events. All saved model names were deepseek-v4-flash. Etcd accounts for
92.9% of input. This diagnostic scope is not an account billing export and
cannot establish the owner's full ~$100 spend.

The concrete amplification is repeated retrieval: ordinary etcd 083529 used
24 final questions (681 original proposals), each rereading 179 source
windows: 4,296 calls and 45.45M input tokens. All 179 evidence-row payloads
were identical across those 24 questions; the complete request differed in
the question. There were no duplicate live exact requests inside any audited
run. A count of hundreds of Learn proposals is not a count of completed
answers. Detailed evidence is in `work/deepseek-usage-independent-audit.md`,
`work/deepseek-question-prefix-audit.json` and
`work/deepseek-question-batch-sizing.json`.

The initial prefix-only change has been superseded by the shared question cube
v1. It serializes the complete evidence catalogue once before all independent
questions, retaining question-specific selections and inspection coverage. The
old per-question string-cell retrieval table and its prompt are removed. The
new typed response requires every question, carries only positively selected
rows and their original anchors, and leaves refused windows unavailable. Its
per-question memos refer to current cached response bytes and revalidate the
complete original shared request before reuse. Adding or reordering questions
does not resend unchanged decisions. Replay changes are observed on the next
read; original runs remain snapshots.
Question-selection console events name the original question and distinguish
accepted empty selection from incomplete or unavailable retrieval. Each shows
selected sources, inspected/unavailable evidence groups and model/cache result
provenance. Rejected responses list their affected questions and reasons; the
closing summary separates questions with sources, empty selections, incomplete
selections and unavailable questions. A zero-source failure never prints ready.
Within one recall, questions backed by the same original window now share its
successful preparation and full-response validation. Reuse also requires exact
ordered equality of that memo's original row refs and question metadata; a
request key alone cannot authorize altered sibling evidence. Invalid metadata
does not poison a valid neighbour. Cancellation and one-time adjunct acceptance
remain unchanged. The regression's eight questions over three original windows
require three full preparations instead of 24, with identical current source
bindings. This is local work saved on a cache hit, not fewer provider calls.

The owner's proposal is broader than this prefix fix: one shared evidence
catalogue with all applicable questions/independent categorization rows,
partitioning only where actual input/output envelopes or genuine prior-result
dependencies require it. Merely batching each of the existing 179 windows
would not be the full investigation. Offline sizing with the official V4
tokenizer counts 1,655,254 tokens for all 2,739 compact source rows of etcd
083529 with unique row keys, before questions/output/reasoning. Thus today's
representation alone exceeds the published 1M context, but 179 is not a
provider requirement. The receipt is `work/deepseek-whole-corpus-token-count.json`.
A larger request must preserve question-specific source selections without
repeating empty decisions for every question/source pair. Compare larger
batches with thinking disabled/enabled on the same source set, measuring
missed sources, complete question coverage, tokens and latency; do not assume
that filling the context or enabling thinking improves quality. Shared retrieval is now integrated as the owning question cube, with
provider-supported thinking and unchanged compatible-endpoint preferences.
Ordinary quality acceptance is still outstanding. Independent
directory/file/callable/type/boundary/operation tables now use byte-only greedy
packing instead of the artificial 8/40-row caps. Explicit read-stage row limits
and the current 64KiB planning budget remain; other tables keep their own
contexts and dependent rounds. Entity memo inputs retain their singleton
representation independently of changed batch boundaries. Exact offline packing
of the original etcd 083529 rows under the same
64KiB input budget reduces Symbols from 1,075 to 148 calls and Operations from
180 to 53 when the 8-row cap is removed. These are preparation measurements,
not a live quality/cost comparison. Focused table/lines/reading checks and vet
pass; the ordinary 20260907-202410 small-server run completed with this packing.
A large-repository quality/cost comparison is still outstanding.
The consolidated measurement and next experiment are recorded in
`work/repomap-request-batching-review.md`, with the micro-request audit in
`work/deepseek-microrequest-audit.md`. Offline shared-question drafts now retain
the complete original evidence once and all questions with separate closed refs.
The complete chi retrieval input is about 81k tokenized content for eight
questions, versus 80 historical retrieval calls. The etcd input is about 1.656M
tokens for 24 questions; its 61,502 negative question/chunk decisions previously
included individual explanations. Re-encoding only the 3,994 accepted positive
decisions with original reasons uses about 168k tokens. Historical unavailable
rows stay separately unavailable; neither these sizes nor old selections are
new model-quality evidence. Exact local drafts and checks are under
`work/question-batch-design`. A batch implementation must keep per-question,
per-evidence-window coverage and per-question memo reuse, and requires a typed
selection response instead of weakening the shared string-cell table decoder.
Two authorized live chi draft replays have now completed with one attempt each.
Fast mode used 81,064 input / 3,414 output tokens in 17.54s, selecting 508 anchors.
Thinking used 81,143 input / 13,745 output tokens (12,082 reasoning) in 105.73s,
selecting 59 anchors. Both covered all eight questions with valid closed refs.
The independent source review nevertheless found a central missed README flow
example for question 2 and an unsupported test-specific reason for question 7.
Neither draft is accepted as the ordinary retrieval contract or its default
thinking policy. `work/question-batch-design/chi/quality-review.md` records exact
sources and required follow-up checks; fewer selected anchors is not itself
proof of better coverage. The second thinking draft completed in 138.98s with
81,336 input and 19,098 output tokens, including 17,134 reasoning tokens. It
restored the missing context-flow example but still promoted some test hints
into confirmed behavior and omitted the original excerpt for a named library's
role. `work/question-batch-design/chi/quality-review-v2.md` records all eight
questions. These failures informed the new cube's prompt, not local semantic
repair. The shared cube and its ordinary reader integration pass focused tests;
final question-answer quality remains an acceptance item.

The new ordinary small-server retrieval inspected all 28 original chunks for
eight questions in one call: 6,880 input and 25,026 output tokens, 189.262s.
Chi inspected 151 chunks for 57 questions in one call: 83,049 input and 26,945
output tokens, including 19,399 reasoning tokens, 203.668s. Jieba inspected 99
chunks for 16 questions in one call: 50,351 input and 31,718 output tokens,
260.931s. These results establish complete request/coverage handling, not that
every useful source was selected. In chi the new selection correctly retains
the separate ClientIP advisory tests previously missed by the draft. Other
questions still miss useful original excerpts, and a later route can omit a
requested example. The detailed audit is `work/shared-question-chi-live-review.md`.
The 57-question chi run was deliberately interrupted after 23 completed answers
because its menu repeats several introductory tasks. No report was published;
accepted responses remain cached. The selector and merger had both seen all
57 candidates together and retained them all. `work/chi-menu-selection-diagnosis.md`
records the actual responses; this was not a partition or UI duplication bug.
Learn proposal preparation now starts with complete evidence instead of the
artificial 64KiB fragments that generated five introductions. Actual prepared
request size and explicit development budgets still govern preparation; real
resource refusals split complete evidence by encoded byte weight. Accepted
sibling reviews survive, while child reviews retain their partial-context
scope. Selection/merge prompts and the absence of a question quota are unchanged.
When every child of a refused Learn request succeeds, a request-identity memo
retains only the accepted complete partition boundaries and child request keys.
Warm readings validate and reuse those exact child responses without resending
the refused parent. Original review order and current source bindings survive;
partial or semantically refused subtrees receive no positive partition memo.
Replay of an accepted whole parent takes precedence, and child replay is checked
by the usual decoder. Resource/replay/no-cache/cache-clear regressions pass;
the nested split regression now makes zero warm calls instead of two repeated
failed attempts. This specific branch has local provider-test evidence.
The ordinary chi run `20260907-212616` produced 15 final questions from one
proposal context, covering all eight goals, in 16.42s across proposal, selection
and merge. It preserves separate useful tasks such as REST flow and versioned
data. Its final ordinary publication and warm reuse passed in the v4 display
checks below; a smaller menu alone does not establish answer quality. Jieba likewise moved from three proposal contexts
and 16 questions to one context and eight questions in `20260907-215305`.
All 140 original context items, their order, the eight goals and all three
proposal/selection/merge prompts are identical across that comparison.

A real full-etcd envelope probe returned HTTP 400 before generation: its
1,656,470 input plus 128,000 reserved output tokens exceeded the provider's
reported 1,048,576 context. The adapter now exposes only an explicit context
refusal as `context_tokens`, retaining the exact body. Shared adaptive execution
allows the owning cube to split complete inputs; generic 400/quotas do not
trigger splitting. No guessed tokenizer ratio or new fixed token cap was added.
No new semantic graph, source truncation or question quota is approved by this
cost discussion.

The current cube also completed a real saved-input etcd reading with all 2,739
chunks and the original 24 questions. The initial request received an explicit
context refusal (1,462,018 input plus 128,000 reserved output tokens). Two
complete child catalogues of 692 and 2,047 chunks then succeeded in parallel,
using 1,464,151 input and 43,617 output tokens in total. All 65,736 question/chunk
slots were inspected, with zero refused final windows and 335 selected anchors;
wall time was 227.192s. A second `read --through question` reused both accepted
responses in 4.681s with zero live calls and identical selections. This reading
recalls no old model descriptions because those memos predate the shared
English policy, so its input differs from the earlier draft/baseline beyond
batching. It is a transport/partition/cache experiment, not an ordinary etcd
report or a matched answer-quality comparison. The receipt is
`work/question-batch-design/etcd/current-cube-verification.json`.

The owner replenished official DeepSeek access after the two 18:30 UTC
attempts failed with 402. Fresh ordinary runs at 18:56 UTC completed with exit
0: `20260907-185650-chi-68356c6bdb1b` (4/4 targets, eight answers) and
`20260907-185650-go-http-server-c4b4bcd5328b` (3/3 targets, ten answers).
Their common manifests/report bundles, target-local indexes/dependency
catalogues/GroupsIndexes and shared atlas artifacts were checked; the receipt
is `work/ui-ux-fresh-reports-verification.json`. The server checkout's existing
Terraform edits remain recorded in its manifest and were not changed.

Actual fresh browser clicks reinforce UX37–UX38. Chi hides its useful HTTP
middleware → HTTP routing pair behind Home → Root → Area HTTP routing. Part
HTTP routing then repeats the same picture and description before exposing
code. In the small server, Root (executable) → Cluster setup hides Server
handlers and Utility functions. Singleton role lanes all remain expanded even
when Applications or Libraries appears selected; an example/tool can be above
the intended component. These are display observations, not a general audit of
answer truth. Notes and screenshots are `work/ui-ux-fresh-chi-levels.md` and
`work/ui-ux-fresh-server-levels.md`. The later native headings, component cards
and actual role filters are checked below; broader visual alternatives remain
explicit owner-review items.

The explorer now bypasses a local area only when it has exactly one actual
part with the same title. It retains the original area node and description,
opens the part directly, and preserves the old area's URL as an alias without
adding a second history entry. Global search indexes one part and still matches
the area's description. Remote areas may be incomplete subsets and retain
their level. Multi-part and differently named areas also retain it. Python
frozen-data browser checks covered the direct click/code-detail path, area-only
search, Frontend UI's three parts, the remote Backend logic area, animate and
All uses, plus old area links and actual browser Back. Notes are
`work/ui-ux-direct-parts-check.md` and `work/ui-ux-direct-parts-history.md`.
The fresh chi/server runs include the initial direct-part change, but predate
the final history/search correction. The new Python ordinary run
`20260907-185833-python-tutorial-game-0b9d396aba81` was interrupted with SIGINT
(exit 130) during answers when the owner raised provider cost. It did not publish
a report. Do not treat these checks as final Learn/Work acceptance.

That walkthrough exposed loss of the originating question when opening the
term library or a map. The existing reading navigator now keeps a visible
“Back to question” link with its title during these side trips, including
Learn/Work switches and subsequent map navigation. It returns to the existing
expanded answer. Home and the general Questions entrance start a new journey.
This is in-page navigation state, not persistent investigation history.
An open area's hidden parent may still supply the inspector description, but
cannot dim its visible children as if it were a node on their current map.
Both changes passed focused report tests and browser checks on ordinary small
reports; the return link also wrapped and worked at a 600px viewport. The
latest small run is `20260907-085807-python-tutorial-game-88f6b9dbc0f2`, with
no added explicit questions and no live provider calls. The ongoing etcd run
predates these presentation changes. Missing explanations of external terms
such as uvicorn, the incomplete backend startup answer and the large-report
journeys remain open.

The owner subsequently rejected arbitrary small generation ceilings. The common
LLM allowance now supplies all stages, and table definitions cannot override it
with a smaller output cutoff. Focused atlas, provider, cache, guidance, portfolio,
documentation, orientation and run tests plus vet pass. Ordinary small run
`20260907-092436-python-tutorial-game-0d882d08ea25` completed in 8 minutes
27 seconds; all 92 saved table requests confirm `max_tokens: 128000`, including
reasoning-enabled answers. Its 16 questions have nine answered, four partial
and three unavailable results. The earlier etcd process continues as a baseline with its original
binary and ceilings; it cannot validate this change.

At the owner's request three agents independently walked launch/verification,
subsystem investigation and operation-to-source journeys. Their logs are saved
in the desktop project's `work/ui-ux-agent-*.md` and summarized in the UI review.
The shared HTML search now includes existing questions, complete answer text and
term explanations, with links back to their actual sections and sources. A
search preview is an excerpt; opening it preserves the full answer. Existing
source links identify the exact concept to explain when opening a code result.
The inspector remembers its selected concept per existing map node. Choosing
one of that part's using operations retains this selection, while a deliberate
drill-down brings the updated map path into view if it was above the viewport.
Hover alone does not scroll the page. The Learn/Work switch stays at the top
when search results open. These are presentation changes over the same HTML and
map, not another knowledge graph.

Ordinary run `20260907-094401-python-tutorial-game-c24b5800d13a` completed in
44 seconds with two targets, 16 questions, 11 answered and five partial, no
rejected windows. It preserves the prior run's question menu and group graph;
the three formerly rejected answers completed on the normal retry. Native
index/group bindings, common publication, graph/question persistence and HTML
anchors passed inspection. All 92 table requests use the shared 128,000-token
allowance. Agent repetitions confirmed search/navigation improvements and Robot retention
through Used by/All uses and exact Code search. A longer path also clicked the
already-selected operation-picker button; that still reset scope. The picker
now retains scope when selecting that same operation, and duplicate concept
names gain their existing exact source labels.

Type-v5 ordinary run `20260907-094938-python-tutorial-game-770ae08a8048`
completed in 4 minutes 59 seconds, with 14 questions: seven answered, three
partial and four unavailable. The subsequent current-binary run
`20260907-095737-python-tutorial-game-7b65a187424d` completed in 47 seconds,
preserving that menu and group graph. It has ten answered, three partial and
one unavailable question: "How do the backend and frontend divide the work in
this project?" received no provider content. The three prior source-ref
rejections resolved on normal retry. Both runs passed native index/group,
single-publication, saved graph/question and HTML-anchor inspection. All their
saved table requests retain the shared allowance. Development check records
are `work/learn-work-type-prose-acceptance.json` and
`work/learn-work-agent-final-acceptance.json`.

The agent's final browser repetition on 095737 confirmed Robot retention
through Used by POST, clicking the already-selected upper POST card, and
All uses. Field and Robot now end in whole sentences and retain their source
links, including Robot in the term library. Duplicate-label presentation
remains unverified: this reading selected only one Robot concept. An API Robot
still generated an unnecessary lifecycle-absence sentence in its type row;
prompt intent does not establish complete editorial acceptance. Dependency
installation and current full etcd acceptance remain open.

Two agents then evaluated the return-after-a-break scenario on 095737. In
Learn, an open Settings article visibly retains the original launch question;
its map link opens Application core with Field selected, losing the side
question about Settings. In Work, POST and its Area/Part path remain visible,
but the preceding Robot inspection is no longer shown. All uses restores Robot
while removing the trace of the just-read POST. The repository name is above
the viewport in these scrolled states. These are open UX31 findings: current
location is clearer than the previous activity. No persistent history or
guessed user-intention layer has been introduced.

The next ordinary small run, 101216, completed in 31 seconds and resolved the
remaining provider refusal: 14 questions, 11 marked answered and three partial.
The declared states are not editorial acceptance: the frontend/backend
responsibility answer still describes the HTTP boundary while excluding other
frontend work from its selection. That broad answer needs further review.
The map's existing term link now passes its exact source into the inspector,
and the reading toolbar preserves both the originating question and the term
article as named return links. The repository name stays in the desktop toolbar.
Choosing a group's operation from a concept records the observed navigation
step: "Opened [operation] from [concept in part]". Both names are actions that
reopen the corresponding view; the step remains after All uses. This records
the user's click provenance, not a new claim that the operation uses that type.
Component or a new independent scope clears that local operation-return step.
The agents verified both Learn and Work loops on 101216.

Resizing had separately hidden the docked inspector. Only floating previews
now close on resize. Ordinary run 101721 completed in seven seconds with no
provider calls; it preserved the graph and answer contents, changing only
the newly answered question's provenance from live model to cache. Settings
and its inspector remained visible when narrowing to 600 pixels and restoring
the viewport. At that point the narrow toolbar still scrolled out of view
and the inspector had limited reading space. The later 103546 run fixes
the Concept label's mid-word wrapping and increases the narrow inspector height.
Development checks are recorded in `work/learn-work-return-context-acceptance.json`
and `work/learn-work-return-resize-acceptance.json`.

The owner requested a three-agent UI/UX design council to investigate whether
the interaction model itself is missing something fundamental. Independent
Learn, Work/navigation and visual-critique reviews exchange objections before
a combined recommendation. These proposals are review hypotheses, not new
product requirements or permission for a separate semantic model. They examine
the difference between user intent, current object, relationship scope and
navigation history, and whether a correct-looking answer actually satisfies
the original task. Keep the owner's central visible map, optional entry by
question or object, and direct source access in view when comparing alternatives.
The combined owner-facing review is `repomap-ui-ux-design-review.md`, with six
testable hypotheses and blank importance fields. The strongest concrete finding
is relation attribution: a reviewer read Used by beside Robot as operations
using that type, but the list belongs to Application core. All three reviews
distinguish current object, map scope/filter and observed navigation history;
hover preview must not silently become the recorded activity. The proposed
single reading composition remains a comparison candidate, not an approved
large redesign. Test it on task understanding and correct relation attribution,
not just working links or the model's answered state.

Ordinary small run `20260907-103546-python-tutorial-game-e2aab0f7b611`
completed in seven seconds without live provider calls. Part actions now sit
inside that part's description, with "Related operations for [part]" before
secondary source details and a separate Concept column. The list comes from
the existing operation/group neighbourhood, including possible connections;
it does not assert ownership of the operation or use of the selected type.
The first layout attempt hid this action below the inspector fold and failed
visual review. The revised layout passed 1600px and 600px checks, including
Robot → POST → source → All uses and a More details click without page movement.
These checks resolve one concrete attribution defect, not the council's full
interaction-model review. The incomplete current-binary etcd run predates these
UI edits and cannot validate them.

D03 diagnosis on the older etcd baseline found that Applications expands a
role area; it is not an exclusive filter. All three applications were present.
ELK's placement, subsequent readable scaling and the initial top-left camera
put them below Tools/Examples and outside the first screen. The current bounded
UI change frames the opened area's existing components in a scrollable map
window without changing their layout or relations. Oversized areas frame their
first component at readable scale. A map initially hidden by a question URL
waits until it is visible to apply this initial frame; later resizing does not
reset the reader's pan. Fit remains the explicit whole-map view, while Reset
restores the opened area's readable frame.
Ordinary small run `20260907-104821-python-tutorial-game-34463d691ba7`
completed in seven seconds with no live provider calls. Exact native/group,
question, saved input and publication checks passed; graph and answer contents
are unchanged from 103546. Browser checks covered the initial map, hidden
Home → visible Home, and zoom/reset at 600px. Its frontend is readable while
the backend partly extends past the horizontal edge. The separate etcd UI
preview uses baseline HTML with current exact CSS/JS solely for layout testing;
it is not a new ordinary report or current-analysis acceptance. At 600px its
server is fully visible, Fit reveals neighbours, and Reset restores the same
readable frame without moving the page. Broader first-screen composition and
fresh large-report acceptance remain open. Development records are
`work/learn-work-relation-actions-acceptance.json` and
`work/learn-work-map-focus-acceptance.json`.

Ordinary small run `20260907-112735-python-tutorial-game-e70a52ffed75`
completed in six seconds with no live calls. The reading toolbar now remains
sticky at narrow widths. Map inspectors reserve its measured height on wider
screens; at 650 pixels and below they scroll with the page, so they do not
form a second permanently fixed block. Navigation measures the toolbar after
updating return links, location and the picker, before scrolling. Waiting only
for ResizeObserver had hidden the Level article heading beneath a newly taller
toolbar and misplaced search results after closing the search panel.
The 600-pixel walkthrough verifies the long domain question, its Level article,
the Application core map and named return to Level. At 16-pixel root text the
map toolbar is 288 pixels tall; the graph remains readable after scrolling its
inspector away. A separate 600-by-900 check with 20-pixel root text verifies
both named returns and the Learn/Work switch: the Level article begins below
its 346-pixel toolbar, and both map nodes can be read below the 379-pixel
toolbar after page scrolling. At 934 pixels the sticky inspector begins exactly below the
184-pixel toolbar. The narrow map breadcrumb and inspector can still leave
the viewport: visible return links do not fully solve D02/UX31 context or hover
inspection. No persistent history or inferred intention was added. Focused
report tests, vet and script syntax checks passed; all 55 structural checks in
`work/learn-work-narrow-context-acceptance.json` passed, including unchanged
graph, learning plan and 14 questions compared with 104821.

Ordinary small run `20260907-114411-python-tutorial-game-6dc028f6bb31`
completed in six seconds without live provider calls. A map node's existing
source link now sits beside its explanation, outside Code and connections.
Operation kind remains visible with its title; an operation with no further
keys, call witness, connections or main-path step has no empty disclosure.
Real supporting material stays collapsed. Root opened the POST source directly
at the exact captured GitHub revision, app.py line 74, and verified the route
highlight. An independent frontend check found animate's exact line 138 without
opening either disclosure or inspector continuation; on a 600-by-900 screen
with 20-pixel text it required normal page scrolling. All 69 structural checks
passed, with graph, questions and source bindings unchanged. Focused report
tests passed in 0.750 seconds; vet and script syntax passed. The receipt is
`work/learn-work-operation-source-acceptance.json`.

Ordinary small run `20260907-115943-python-tutorial-game-9ea84cf7a647`
completed in six seconds with no live calls. The operation strip now shows its
count and previous/more controls when it overflows. Explicit selection, clearing
the search and width changes reveal the selected card by scrolling only the
strip. Typing and resizing preserve the current search; an explicit external
selection clears an incompatible operation search to expose the new choice.
Map controls retain their own inspector instead of dismissing it on pointerdown.
A node inspected outside the exact selected scope or pinned operation is labelled
Preview; this does not change the selected scope or record a new intention.
Root verified global selection, strip paging, filtering and clearing at 934px.
An independent Work walkthrough verified Related operations selection and
wide-to-600-to-wide resizing with 20px text, plus keyboard Preview of Frontend UI
while Utility helpers remained selected. On the narrow screen the strip may
require ordinary page scrolling; an external selection can scroll the page back
to the map. A long breadcrumb still partly exceeds its visible width. The full
group detail section at this checkpoint also retained map state without showing
the prior operation; the later detail-return change below addresses that path.
These observations are not state loss or completed UX31 acceptance.
All 45 structural delta checks passed: graph, learning plan, 14 questions and
source bindings are unchanged. Focused report tests passed in 0.808 seconds;
vet and script syntax checks passed. Records are
`work/learn-work-operation-context-acceptance.json`,
`work/ui-ux-operation-context-root.md` and `work/ui-ux-work-current-context.md`.

Ordinary small run `20260907-121841-python-tutorial-game-b36eaf41cfe6`
adds an explicit Back to map action to the existing toolbar return block while
reading a full group section. Its label describes the live map's view, exact
operation with selected/preview state, and Area/Part path. The reading-location
line separately names Full details and the actual group. Source-hint buttons
are removed only from a copy of that title. Full group content remains general;
an operation in the return label never filters or claims ownership of its facts.
The new action scrolls to the existing map and shows its selected scope or
operation without changing the map's selection, searches, layout, zoom or pan.
The original on the map action still locates the named group and may change
scope. Inspector contents are shown again; exact disclosure restoration and
serialization of map state into browser history are not added.
Root verified POST → Area/Part → full section → Back at 934px on 121436.
An independent frontend path opened a neighbouring full group, changed
Learn/Work and returned to the original animate/Area/Part rather than that
neighbour. Final 121841 also passed a cold full-section URL and Learn → Back
to Structure without inventing a previous operation. Its 600px walkthrough
could not receive visual acceptance: after entering full details the browser
capture was displaced relative to measured DOM coordinates. The link's full
wrap is DOM-checked only; screenshots are not used to claim narrow acceptance.
Both temporary viewport overrides were restored. A later root check reproduced
capture failure on the ordinary Structure map before entering full details:
the viewport override changed measured DPR and both supported screenshot APIs
failed. Reset restored the normal 934-by-992 capture. This confirms an emulator
limitation without proving the absence of a separate narrow-layout defect;
repeat attempts with that unchanged mechanism are not acceptance progress.
The final ordinary run completed in eight seconds with no live calls. Its 24
delta checks preserve the 41 structural checks of 121436 against 115943: graph,
14 questions, both page bindings and source links are unchanged. Report tests
passed in 0.801 seconds; vet and script syntax passed. Records are
`work/learn-work-full-details-context-acceptance.json`,
`work/learn-work-full-details-label-acceptance.json` and
`work/ui-ux-full-details-context-root.md`. Large-report and human-comprehension
acceptance, D04 and the other open Learn/Work paths remain incomplete.

D04 source audit on 104821 isolated two semantic failures. Question retrieval
retained 23 original candidates, including UI and simulation declarations.
The single complete route pool selected only three backend endpoints and
three corresponding client HTTP functions. The final provider response itself
returned answered/none while explicitly excluding other frontend work. The
reader and renderer did not promote that state; both current prompts already
require completeness against the original question. The next bounded quality
checks are separate: an honest partial answer on the same six sources, a
complementary role selection from the unchanged 23, and a narrow HTTP-only
question as a control against over-demanding implementation details. Do not
repair the state or reinsert sources locally. The saved diagnosis is
`work/ui-ux-answer-task-completeness.md`.

The full 14-question static source audit of ordinary 121841 found three further
wrong answered results. Both the stored-level format and add-level answers
substitute the API Level model for the catalogue's actual dictionaries. The
add-level answer wrongly requires id (added by handlers) and omits the
wincondition_check used unconditionally to run the level. The selected levels
anchor carries a name and prior hypothesis, not its initializer structure;
route selection did not lose that anchor. The HTTP endpoint answer also calls
/api/levels a list response although it returns count. GetLevelsInfoResponse
with count was present in the original question input but was not selected.
The frontend IGetLevelsResponse reached the candidates but had no count field
in its original evidence; its later route exclusion is not the loss of that
field. Separately, the error-state answer is honestly partial against its
final input, while route selection discarded the already available State enum
with error/success. A final-answer prompt alone cannot repair all these paths.
The startup commands are factually correct but do not explain dependency setup
or state a prepared-environment assumption; they are not accepted as a complete
first-run recipe. The purpose answer's partial state is honest, but retrieval
retained only the README title. The other reviewed answers did not yield a
confirmed factual defect at their requested level; this is static audit, not
runtime success or human comprehension. The report and cached responses were
not patched. `work/learn-work-answer-quality-review.md` links all 14 exact
questions, original inputs and sources, with importance left for the owner.
`work/learn-work-remaining-acceptance.md` covers all 31 UX and six design items;
API balance blocks new semantic responses and the fresh large ordinary run,
while source/relationship, editor-failure and exact-name checks remain local
work rather than grounds to mark the full goal blocked.

Five isolated prompt-only development readings kept all nine tested stage
inputs byte-identical to the original small run. The first answer clarification
still called the HTTP-only overview answered. One broad route response contained
invalid JSON; the narrow route kept all three client/server pairs but asked for
unrequested payload details. The next route attempt and both second variants
were refused with HTTP 402, not evaluated semantically. None of these prompts
has entered the ordinary path. A reviewed next answer variant preserves useful
qualified deductions from names/signatures rather than demanding explicit job
documentation. `work/d04-prompt-experiments.json` records the outcomes; the next
variants are `work/d04-answer-completeness-prompt-3.md` and
`work/d04-route-responsibilities-prompt-2.md`.
Source inspection separately confirmed that proposal Title/Question/Why survive
in question origins but are absent from provider question/route/answer inputs.
Their causal role in D04 is unproven. No projection has been added: explicit
questions must not acquire invented intent, complete base goals must not add
unasked requirements, and standalone saved-input readings lack those origins.

### Active acceptance: operations and matching

The owner narrowed the next milestone to universal operation discovery and
matching, with Cobra and HTTP as examples. Preserve model descriptions and
roles on internal entities, plus the kinds, endpoints and evidence of their
connections through publication. A command, route, scheduled task or permanent
process uses the same graph interaction. Framework meaning belongs to model
interpretation over language-neutral evidence; no parallel Cobra analyzer.

Acceptance starts on the overview graph: find etcdutl, hover for a short
description, click the same node without moving the pointer, then see its
operations on the component graph. Hovering an operation highlights the paths
and components it touches; cross-component edges lead to the relevant code or
operation. Imports, calls and inferred integrations remain distinguishable.
The equivalent server view exposes routes and background work through the same
mechanism. The map work is part of this milestone: grouped, short overview
names; distinct executable/library colors and language icons; compact model
descriptions with source links; reachable previews with pointer-intent handling;
stable layout without overlapping boxes or a permanent tangle of arrows.
Performance fixes preserve the existing evidence and rendered information.
Existing symbol descriptions, knowledge records, native relations and matching
stages must be used before adding stages.

The owner's morning review separates the product's base questions from optional
agent-style investigation. The ordinary report should guide a reader through
useful questions chosen by the product: what is here, which component to open,
which operations it exposes, and what code those operations touch. A developer's
`--question` experiment is an additional reading aid, not the report's default
opening or a replacement for those base questions. Acceptance must independently
check the relevant source and actual navigation, not grade an answer by another
generated answer. The base-question coverage remains unfinished.

The normal Go call-index pass now visits every loaded repository function,
including callbacks outside the launch call tree, and canonicalizes generic
origins. Anonymous functions retain their compiler signatures through the
Go adapter as well (DirectCallIndex v9). Explicit depth/edge narrowing remains
explicit. Declaration candidates
include anonymous functions passed as callbacks; incidental closures are not
automatically added. Neutral callable bindings retain the source and destination
names, field/argument detail, invocation, resolution and source location. Calls
to external code retain qualified names, so `context.WithTimeout` does not lose
its identity before interpretation. DynamicHandoffIndex v6 also retains source
assignments to interface fields, keyed by the compiler's field declaration.
Local factory return values can resolve a stored implementation. These are
possible alternatives with an open frontier, never exact instance bindings:
all observed stores to one field do not prove the value of a particular receiver.
Fields with the same name on unrelated types do not share candidates. Both the
call site and the constructor assignment survive projection; an assignment is
support for the call, not a second call at the constructor line. This recovers
`quotaKVServer.Put -> kvServer.Put -> EtcdServer.Put` in etcd without any
framework-specific rule.

Dynamic value traversal reuses immutable summaries within one root and exact
interface method. The function key also retains `throughFlow`; factory result
indices remain attached to their own SSA values. Only subtrees that completed
without an active-path cycle enter this local memo. Cyclic results propagate
their dependency on the current path and continue to use the original traversal.
Merging retains child-order evidence, every exact assignment location and the
number of unresolved paths per incoming edge. An integer representation overflow
is a terminal extraction error, including when the same callable resolver feeds
external-call argument facts. This adds no persistent cache, interface-method
cap or inferred candidate, and does not promise linear traversal of cyclic graphs.

Interface-valued arguments now retain the concrete methods of the declared
interface when their implementation is resolved from the actual value, local
factory return, or observed alternatives. The existing transfer slot records
the declared interface and method. An unrelated compatible type cannot supply
an implementation; extra concrete methods outside the interface are excluded.
These are object-registration observations, not callback executions. On etcd,
quotaKVServer.Put retains the RegisterKVServer argument at grpc.go:80 and the
separate local KvServerToKvClient adapter binding at v3client.go:33.

Callable bindings now retain literal assignments to other fields of the same
SSA receiver in that function. Referrer identity keeps two command/worker
objects of the same type separate; conditional or later stores remain separate
anchored observations, not final runtime values. Named and anonymous callbacks
share this mechanism. No field names or framework types drive extraction.
ProgramIndex carries these as `callable_receiver_field` witnesses, and the
atlas attaches them as binding evidence rather than additional registrations.
The own callback sees its object's fields; a neighbouring caller's registration
retains just the binding shape and source, so a shared error helper does not
inherit every command's help text. Canonical sealing, independent copies,
source anchors and a two-object fixture verify the underlying facts.

Symbol rows propose activation and selected outgoing calls using local refs.
The independent `atlas_operations` v12 table reviews the candidate declarations
against registration evidence and their immediate native callers. Caller object
identities remain in Graph/reading-input v6 for local retrieval, then disappear
from provider rows. Each caller is represented once with its source signature,
documentation, registrations and every distinct incoming call site. The review
does not include unrelated same-named methods or recursively expand a caller's
own callers and outgoing calls. That expansion exceeded one row's context on
etcd's shared ExitWithError helper. It does not repeat previous
activation/name/description hypotheses in its input:
on etcd those encouraged the model to confirm internal helpers as operations.
Its compact calls also omit unrelated literal and log-message payloads. It keeps an
action name and short result description on the original symbol, with a separate
knowledge record for this decision. It can reject a helper without deleting its
function description. A closed entry choice now distinguishes this declaration,
an advertised immediate caller, and no supported entry. Only a self choice can
publish this declaration as an operation; choosing a caller does not promote
that caller without its own review. A registered callback or an observed asynchronous entry
is reviewed even if the first symbol pass did not propose an operation. Native
patternless calls retain qualified dispatch detail and resolution; previously
these observations vanished before reaching the model. Commands, requests, user
interactions, scheduled work and continuous work
share this contract. An observed HTTP route replaces the duplicate declaration
operation; different route aliases remain separate. These interpretations still
need evaluation: a source anchor proves the declaration exists, not that the
model's classification or explanation is correct.

Symbol table v6 and operation table v13 admit `interaction` for user-facing
handlers. A callable JSX attribute is evidence for review, not an automatic
interaction classification: render props and internal callbacks use the same
neutral binding observations. The model reads the exact element/attribute,
declaration and existing calls. Labels come from that evidence, not invented
button text. The original declaration and its native paths remain the map's
operation identity. Interaction labels are short English names grounded in the
supplied evidence. Literal commands and paths retain their exact spelling.

Repeated source observations use a row-local evidence dictionary. Callable
bindings use shared columns plus association rows. An operation row receives
only the registrations whose recipient is that declaration. Callbacks it
supplies are listed by name; their command metadata belongs to their own rows. This is lossless input
preparation: original order, multiplicity, alternatives and source locations
remain recoverable. Immediate caller context includes how that caller is bound,
not every callback it registers. Provider symbol windows contain eight rows;
already accepted entity descriptions do not change identity with batch size.

Path-presence questions use the corpus's existing VisiblePaths inventory,
including non-readable configuration types, rather than only source entries.
Filesystem inventory retains those paths before selecting readable content and
still respects explicit exclusions and skipped dependency directories; it does
not depend on Git. This fixes false "no CI" claims for YAML workflows. Changelog
directories and nested-project linter configurations also count. Absence wording
is restricted to recognized files in the inspected paths; it is not an assertion
that no alternative configuration exists. Content selection remains unchanged.

Matching retains native imports, calls and callback transfers separately from
model-confirmed integration hypotheses. Every peer window is considered rather
than silently selecting a first candidate subset. Rows share a peer dictionary
only when they have identical eligible counterparts: the same source object,
same-component-only counterparts and incompatible fixtures never appear as
choices in a request that forbids them. This removed six rejected peer windows
on etcd; shared dictionaries remain exhaustive over admissible peers. Inferred connections retain
their original joint identity, source/destination subjects and exact anchors,
including distinct calls at one source line. The ordinary report projection
keeps this information in GroupsIndex v5 (atlas v2, report v78), with no legacy
reader. Blind matching now compares each peer window's winners again against
the original evidence until one counterpart or none remains per outgoing
boundary. It no longer publishes one independent winner from every window.
Peer eligibility is partitioned inside each bounded window to keep unrelated
outgoing rows batched. Candidate declarations retain exact signatures and
comments, including streaming result types; protocol similarity alone is not
the same operation. No Cobra-specific detector or parallel graph has been introduced.

The operation map follows native calls and executions from the selected
subject, with cycle protection, and projects the visited subjects to groups.
Imports and the act of supplying a callback or service object do not become
execution paths. This currently also leaves an open path where an external
framework invokes a supplied callback without a separately observed call;
the binding stays in the underlying graph. A model-matched endpoint is included in
an operation's path only if its caller is reached; otherwise it remains visible
on the caller's group. Cross-component endpoints link to the matching operation
where its anchor is known. These are possible static paths, not a runtime trace;
unresolved dispatch and model integrations use dashed lines.

The operation view now has search, a group filter, and eight operations per
view. This is display pagination: every operation and its precomputed paths
remains in the static HTML. Hovering keeps the operation stationary and brings
its related groups alongside it; labels include their group to distinguish
same-named methods. Links to operations on another page reveal that page first.
Links back from code cards also reveal a hidden group. The existing pan/zoom,
source links and pointer-intent preview remain shared across these views.

An ordinary `--question` run now carries the atlas question result in memory
through publication into the common report JSON/HTML. It renders a short reading
guide, original source links and the unresolved question; incomplete/unavailable
selection remains explicit. The separate development `read` command still does
not render HTML. Six stops is a preference for the final reading order, not an
evidence-validity ceiling: seven or eight known anchors no longer cause the
whole guide to be discarded. Intermediate reduction rounds remain bounded and
unknown references do not become source anchors. No source bodies have been
added to question evidence yet.

Last checkpoint with the sampled command-to-handler transitions intact:
`20260906-020207-etcd-0249ec32f700`,
88.037 s with no new provider calls, 27/27 targets and one common HTML/report
JSON/manifest. All 27 local
ProgramIndex sets, dependency catalogs, reduced documents and GroupsIndex hashes
were inspected against the common report; each local set has one entry and all
native/set/groups/report hashes agree. The browser opened etcdutl from the
overview and followed etcdctl put to quotaKVServer.Put at quota.go:60, revealing
the destination page. It found no duplicate IDs, missing internal anchors,
visible node overlaps or console errors. No pointer-intent code changed, and
diagonal pointer transfer was not repeated; its prior real mouse acceptance
remains the 211826 checkpoint. The full receipt is workspace
`work/etcd-operation-map-views-acceptance.json`.

This is NOT universal matching acceptance. All 5,563 symbol descriptions,
564 file descriptions and 1,248 operation rows were reused. Matching reused all
2,651 rows in 129 windows, with no rejected rows. Current sampled choices are
kv.Get -> kvServer.Range, kv.GetStream -> kvServer.RangeStream and kv.Put ->
quotaKVServer.Put or kvProxy.Put. In the earlier 012907 diagnostic checkpoint,
changing the operation peer pool instead selected mockKVServer.RangeStream and
leasingKV.Get. The current correct samples do not establish general precision.
The global one-peer reduction applies to blind matches, not separately confirmed
equal-value pairs.
Exact gRPC full-method literals already exist in the generated client boundary
facts. An SDK wrapper's short method name and a model operation label are weaker
candidates; do not claim a runtime destination from their agreement. A mock or
client adapter is not the deployed server simply because its signature fits.

The callable context gap is now repaired locally. Graph/reading-input v6 retains
CalleeIDs on SymbolCall and PlaceID on SymbolCaller, resolved from compiler
locations to existing symbol places. The provider projection removes these
keys. Generated callables retain their calls and bindings with Candidate=false;
neither description nor operation review asks new rows for them. Multiple
target-native IDs at the same declaration now contribute observations to that
one place. Previously only the first target copy supplied its calls/bindings,
silently losing observations from siblings. Source-distinct calls survive,
duplicate copies meet, and same-named declarations in other files stay separate.
Graph validation rejects dangling caller/callee place links. The v6 increment
also keeps outgoing source columns locally. Inspection of getCommandFunc showed
one exact call site twice: its library index has no implementation in scope,
whereas the executable index has a possible kv.Get receiver plus an explicitly
open frontier. Both share compiler position, invocation and dispatch detail.
The atlas now removes the empty unresolved copy only when a possible receiver
at that exact source position carries otherwise identical call facts. It keeps
every possible receiver, different columns/details and independent evidence;
an exact view cannot erase an uncertain observation. Native indexes remain
unchanged. Source columns are stripped from provider rows, as callee IDs are.
This is a local representation correction, not the blocked callee-context
expansion. The concrete native observations are recorded in workspace
`work/etcd-interface-call-observations.json`.

Ordinary v6 run `20260906-033749-etcd-cbfec998d314` completed in 373.531 s.
It removed 221 empty repeated observations and preserved 156 source-distinct
calls formerly merged by their shared line number. Get matched kvServer.Range
again; GetStream did not. Comparing its provider facts also exposed 163 symbols
whose facts were unchanged but reordered by adding the local column. The key
now sorts existing call evidence first and uses the column only to distinguish
otherwise identical call sites. A new ordinary run
`20260906-035246-etcd-ed01de1c72e8` completed in 261.564 s, with zero order-only
changes across 6,790 unchanged symbols and 306 symbols with changed call facts.
Put matched quotaKVServer.Put and kvProxy.Put; Get matched kvServer.Range;
GetStream remains unresolved. Receipts: `work/etcd-dispatch-projection-diff.json`
and `work/etcd-dispatch-stable-order-diff.json`. These are possible matches,
not runtime endpoint resolution. Native target indexes are unchanged.

The first ordinary v5 run `20260906-022225-etcd-71e0df180a44` completed in
472.557 s. It keeps 1,533 generated context callables and 5,563 description
candidates. Aggregation changed observations for 825 symbols; 4,738 descriptions
were reused. Eight malformed symbol rows were recovered through the same read
stage in two four-row windows, 6.828 s total, reusing the other 5,555 descriptions.
All 1,263 operation rows answered (849 reused); three matching rows were
rejected for a missing peer cell. The second ordinary full run
`20260906-023410-etcd-11ea3457fab6` finished in 125.826 s, with 27/27 complete
targets and zero rejected/unanswered rows. Every symbol and operation row was
reused; only one matching window, two question windows, four route windows and
orientation made live calls. Native/set/groups/report hashes agree for all
27 pages, and the common HTML/JSON/manifest exist once. Its structural receipt
is `work/etcd-callable-identity-acceptance.json`.

The v5 report is diagnostic, NOT matching acceptance: browser search finds
etcdctl put but no remote destination, because all candidate pairs were rejected.
Get/GetStream also lost their previous matched endpoints; unnamed outgoing
rows of the enclosing get callback instead matched EtcdServer.Range. The
etcdutl map still exposes seven callbacks plus main; keyboard focus shows a
short defrag description and highlights its neighbouring groups. Pointer
transfer was not repeated in this check. Keep 020207 as the last sampled
working map. The 023410 owner HTML remains, but its backing cohort has been
replaced by newer diagnostic cohorts below.
Logs, metrics and exact call-path examples are in workspace
`work/etcd-callable-identity*`.

The restored native path is kv.Get -> kv.Do -> retryKVClient.Range ->
kVClient.Range -> the external Invoke carrying /etcdserverpb.KV/Range.
Reachability from kv.Get also reaches Put/DeleteRange/Txn through the shared
Do switch. Therefore this graph alone cannot assert that Get writes or deletes:
branch conditions and operation arguments are not interpreted by this pass.
GetStream has a separate path through retryKVClient.RangeStream. Do not replace
this remaining semantic gap with a gRPC/Cobra allowlist or guessed receivers.

Pending user approval: automatic approval review twice rejected a proposed
matching-context patch as a general source-data egress expansion, despite
inspection showing the same signature/doc/registration fields already present
in accepted symbol and operation requests. That patch is NOT applied. It would
give matching the selected call's immediate callee declarations and an incoming
callable's own registrations, without bodies or recursive expansion. An async
approval question is pending in this task. Do not retry the patch, change tool
to bypass review, or send the proposed new matching context until the user
answers. A separate safer correction was accepted: matching v3 renames existing
`signature`/`author_doc` fields to `caller_signature`/`caller_doc` and explains
that an outgoing boundary's signature belongs to its enclosing caller, not
the invoked SDK method. It adds no source data. The prior v2 input misleadingly
placed putCommandFunc's Cobra callback signature beside v3.KV.Put and compared
it with a protobuf handler signature. The ordinary full v3 run
`20260906-024726-etcd-c52238fbb537` completed in 325.416 s: 27/27 targets,
4,082 matching rows in 199 live windows, no rejected/unanswered rows. The
symbol/operation/question/route stages reused all their results; orientation
made one live call. All 27 native/set/groups/report hashes agree; the common
HTML/JSON/manifest exist once. The initial reference count covered only 8,306
outgoing callee links; a later audit also checked the 7,156 incoming called_by
place links. All 15,462 resolve. The earlier audit incorrectly looked for the
incoming JSON key callers instead of called_by; its receipt is corrected.
Put now matches quotaKVServer.Put (four source/target-view pairs for one
destination anchor); Get and GetStream still have no matched destination.
This is a partial recovery, NOT universal semantic acceptance. The receipt is
`work/etcd-matching-caller-fields-acceptance.json`, with semantic_acceptance=false.
Additional callee context still awaits approval and is not present in v3.

Browser verification of 024726 was not completed: a mistaken file-URL navigation
was denied by the browser URL policy, which explicitly forbade an alternate
route to the same blocked outcome. No HTTP/browser workaround was attempted.
The preceding keyboard and real pointer checks remain historical evidence,
not browser acceptance of this new report. Follow the prescribed loopback QA
entry point from the start in future independently permitted browser work.
Use `--no-open` for subsequent unattended runs so the ordinary CLI does not
attempt its own OS-level browser launch while browser inspection is unavailable.

Operation input v11 gives each declaration only its own registration evidence,
with a shorter prompt. Registrations of other callables keep their names, not
their help text. This removed the v10 regressions NewSnapshotRestoreCommand,
Start, the JWT parser callback and inner kvServer.Put from external actions;
quotaKVServer.Put remains a request. All seven executing etcdutl callbacks
remain, plus main as a launcher node. Server has 148 library / 149 executable
operation nodes; leasingKV.Get is still misclassified as a request. These counts
are not a verified public API inventory. The first v11 read rejected 16 rows
with invalid caller refs; isolated four-row windows recovered 12 and single-row
windows recovered the last four. The production window remains eight. Accepted
entity results were reused by the ordinary full run. Compact source examples
are in workspace `work/etcd-operation-owner-checked-examples.json`; discarded
development run directories are not required to replay accepted cache entries.

The operation map now projects one destination for the same source anchor and
component name across overlapping library/executable views, preferring the
executable destination when present. It preserves all source connections and
keeps different anchors/components separate: quota Put and proxy Put are still
two alternatives, not duplicate views of one endpoint.

The last field-only accepted report `20260906-001259-etcd-7947eb7ba5f8` remains
available: 97.709 s warm, seven source-registered etcdutl callbacks, 27 targets.
The command-parent relationship remains absent: names alone do not constitute a
complete CLI grammar. Library and executable views of server are overlapping
uses of the same implementation, not two independently deployed services.
Etcdutl uses server storage code locally; test client/server protocol matching
with etcdctl, not by pretending snapshot restore sends a request to a server.

The 024726 question guide reused the ready 023410 result: all 725 chunks,
93 candidate locations and
six selected stops, including WAL, EtcdServer.Put and applierV3backend.Put. It
still asks which bbolt buckets/MVCC index updates a Put writes and how WAL
relates to backend commit. That is a reading guide, not an
answer about storage semantics or durability. Source signatures/comments locate
code but cannot establish those details. The source-checked question review is
workspace `work/repomap-question-review.md`, including a 50-question catalogue
and a manually source-checked storage answer to use as an acceptance example.
That answer is not yet generated by repomap. Its next useful increment is reading
selected function bodies and necessary neighbours from the same snapshot,
attaching the result to existing entities instead of summarizing summaries.

Repeated source evidence previously overflowed individual rows: updateMax at
92,884 bytes and newGRPCProxyServer after first compaction at 73,433. Lossless
row-local evidence and binding tables resolved those envelopes. Two symbol
windows still produced invalid responses; eight-row windows recovered the 24
missing symbol descriptions without reanalyzing accepted entities. This is a
context/quality correction, not a new repository size limit.

Disk cleanup removed only our failed or superseded development cohorts and
retained shared response/entity caches and user-facing reports. The latest
receipts include `work/etcd-superseded-cohorts-cleanup.json`,
`etcd-entry-ownership-failed-retention.json`,
`etcd-operation-owner-probes-retention.json`,
`etcd-operation-owner-partial-retention.json` and
`etcd-operation-map-views-retention.json`,
`etcd-callable-identity-retention.json` and
`etcd-callable-identity-complete-retention.json` and
`etcd-matching-caller-fields-retention.json`,
`etcd-historical-native-retention.json`,
`etcd-cancellation-guide-retention.json` and
`etcd-storage-checkpoint-retention.json` and
`etcd-question-v4-retention.json` and `etcd-storage-v4-retention.json`.
Superseded successful owner HTML files
and compact source examples remain; their full native cohorts were removed only
after the replacement passed structural checks. The 020207 working-map and
042339 current cohorts remain complete. The earlier replacement of 022225
and its symbol-recovery directory freed 737,557,865 bytes. The 023410 backing
cohort has now also been removed, preserving its HTML/manifest/page portfolio.
An overly narrow timestamp assertion interrupted that cleanup after its first
portion; the receipt records the remaining exact portfolio paths and the
unavailable first-portion byte count. Cleanup then completed, leaving about
958 MB free. That was insufficient for the next publication's temporary JSON
peak. Inspection then found 78 obsolete child report.json copies in three older
27-target runs. Removing those copies and native indexes in six exact historical
cohorts freed 6,440,944,296 bytes, preserving every HTML and each complete common
historical report JSON, all manifests and model journals/caches. The failed
030952 backing cohort and duplicate development input/knowledge snapshots were
also removed after replacement, freeing 605,523,602 bytes. Finally, 024726's
native graph was compared with 031547: places agree exactly and the atlas differs
only in matching live/cache counters and its resulting hash. Its redundant
backing cohort was removed (458,679,362 bytes), preserving HTML/common JSON,
question guide and model journals. After 041257 passed the full artifact checks,
the 031547, 033749, 035246 and 040344 native backing cohorts were removed, along
with three replaced development input/knowledge copies. Their 27 native index
identities were compared with the replacement before any removal. All common
report JSON/HTML, manifests, portfolios, question results and model journals
remain. This freed another 2,060,161,816 bytes. After 042339 passed the same
full structural audit, its 27 native index identities were compared with
041257 before removing that redundant backing cohort and the route-only
comparison's duplicate input/knowledge. This freed another 533,826,012 bytes;
the compaction report, common JSON, question evidence and response journals
remain available. Neither model nor Go caches were touched.
Do not create per-run Go caches or retain every full failed analysis indefinitely.

The route table is now v5 and question-route output is v4 (report format 79).
It requests only the closed reading order and an open question; the redundant
summary is removed from provider output, persisted guide and stage logging.
The page already showed original candidate reasons instead of this summary.
There is no old-format adapter. Candidate pools receive aggregate file/entity
connections with their kinds and counts, while the original detailed call
witnesses remain in `QuestionRoute.Connections`. Previously every representative
anchor pair repeated all calls between its files, making even eighteen candidate
locations require another split and misleadingly suggesting function-level
precision. The compact context fits eighteen locations in one final pool.
Earlier isolated route reads took 8.319 s and 5.434 s, and both were partial:
one of the four initial pools received invalid JSON, even with provider JSON mode.
The failed response was not cached as accepted; the other four requests reused
accepted answers. Repeated full runs are not a remedy for this model failure.
The report shows the selected source locations and their original reasons.
This is a reading aid, not an established explanation of etcd persistence.

A second question was checked on the current saved input: how cancellation or
timeout reaches the etcdctl user. `work/question-cancellation-review` read all
725 chunks in 60.518 s including reduction, selecting 74 candidates, but its
final route had invalid JSON. One exact `replay` in 2.249 s reproduced the
extra closing brace; neither answer was accepted or repaired. With route v5,
the same retrieval was reused and five route windows succeeded in 8.915 s,
selecting commandCtx, ContextError, isContextError, EtcdError and ExitWithError.
This is development evidence. The first ordinary run 030952 completed all 27
analyses but failed to stage report.json when the disk filled, after 97.199 s;
the failure and rejected publication are recorded, not counted as acceptance.
After the cleanup above, ordinary run `20260906-031547-etcd-b051ef9237d9`
published in 89.827 s with zero new provider calls: all 27 target-local artifacts
agree with one common report JSON/HTML/manifest, all 15,462 callable refs resolve,
and the five-step cancellation guide is carried into the common report. Static
HTML checks found no duplicate IDs or missing internal anchors. Browser QA was
not repeated. Put still matches quotaKVServer.Put and Get/GetStream still lack
destinations; the route itself is reading guidance, not a behavioral answer.
Receipt: `work/etcd-cancellation-guide-acceptance.json` (structural acceptance
true, semantic acceptance false). Log/metrics are `work/etcd-cancellation-guide*`.
Tests for atlas/report/run/contracts and vet for changed Go packages passed,
and `make build` rebuilt the ordinary binary.

A later acceptance check found question.graph_sha256 was empty on the ordinary
path, although saved-input reading filled it. PersistGraph and SaveInput sealed
copies, while the reader kept the unsealed input value. SaveInput now returns
the same sealed graph it saves, and the reader uses that value in memory before
constructing its state. No artifact reread or provider context was added.
A regression begins with an unsealed graph, then reads its saved input and
checks both result identities and response reuse. Ordinary run
`20260906-040344-etcd-87110e6ac23a` completed in 89.690 s with zero live model
calls after the fix. This corrects a gap in earlier structural acceptance; their
question hash was not actually checked. The current receipt also checks that
question, places and saved input name the identical nonempty graph hash.

The storage question exposed two different losses. In the v6 run its final
six-stop guide selected forwarding wrappers and dropped the actual path and
write locations. A bounded generic facet-preservation prompt comparison on
033749 took 8.260 s and retained newBackend, but discarded both ToBackendFileName
and BackendPath even though they reached the final candidate pool. That prompt
was NOT adopted. See `work/question-storage-facets-evaluation.json`. Selecting
original evidence instead of repeated summaries is necessary but does not by
itself preserve every part of a compound question.

The next real question asks what starts/stops automatic compaction and what it
affects. The existing reader took 48.413 s; two rejected windows left 36 of 725
chunks unanswered. Its guide is explicitly partial. Run and Stop were present
in the same chunks, but the question table permitted only one selected anchor;
both Stop methods were discarded before route selection. It also chose Start
instead of NewServer, and Raft-log compaction instead of Cleanup. These are
different mechanisms, not interchangeable background work. The exact input and
output selections are recorded in `work/question-compaction-anchor-audit.json`.
Question table v4 now selects multiple closed anchors per chunk. Each anchor
keeps its own subject, source location and original evidence; the short row
reason explains their shared relevance and is not a behavioral proof. This uses
the existing sequence cell and the same provider input, with no framework rules
or new stage. Question-route remains v4 and report format 79. Tests cover two
complementary declarations in one chunk, duplicate/unknown ref filtering, exact
source identities and independent evidence restoration.

The v4 comparison took 50.804 s and retained all eight source-checked lifecycle
anchors (Run/Stop/Pause/Resume for Periodic and Revision), versus two before.
This is a narrow anchor-recall check, not a general precision/recall benchmark.
One six-row window was refused as malformed JSON. Ordinary online acceptance
`20260906-041257-etcd-f1b5669ecc73` then completed in 95.530 s: that one window
was the only live provider call, all 725 chunks were answered, all map stages
and the five route windows reused accepted answers. All 27 target-local graphs
agree with the one common report/manifest, all 15,509 caller/callee place refs
resolve, and question/places/reading-input share the same nonempty hash. Static
HTML checks verify its JSON digest and internal anchors; fresh mouse QA remains
blocked. Receipts: `work/etcd-question-graph-binding-acceptance.json`,
`work/etcd-compaction-question-acceptance.json`, and
`work/question-compaction-evaluation.json`.

The six-stop compaction guide now retains Periodic.Stop, but remains semantically
incomplete: it misses NewServer's actual startup and Cleanup's shutdown, and
incorrectly brings Raft-log compaction into the automatic MVCC compaction path.
The row's declarations do not expose the calls connecting those lifecycle sites.
No source bodies or additional matching context were sent. A ready reading
route is not proof that the question has been answered. Do not rerun blindly,
add framework-specific rules, or hide this loss with a longer summary.

The same storage/write question was next evaluated through the ordinary path
with question v4: `20260906-042339-etcd-d83a95e8daf6`, 141.150 s, all 27 targets,
725 answered chunks, 58 question windows and 14 route windows, no refusals. All
map stages reused answers. The 236 candidates retain the complete datadir
helper family and BackendPath, and the final guide now reaches MVCC Put; it
still discards every path helper. BackendPath survived to the final pool, so
this is not a retrieval miss. Structural checks pass, including all 15,509
callable refs, question graph identity and HTML digest/internal anchors.
Receipts: `work/etcd-storage-multi-anchor-acceptance.json` and
`work/question-storage-multi-anchor-evaluation.json`. This is the latest ordinary
HTML; it is not a complete storage answer or renewed browser acceptance.

A bounded route-only comparison clarified the six-stop rule in the prompt:
intermediate pools retain their limit, while the final length is a preference,
as already implemented by validation. It took 11.433 s and produced seven
forwarding/write stops, still dropping BackendPath from the final pool. A
different partial pool returned all 24 refs against its six-ref limit and was
refused. This variant was NOT adopted. The result refutes a six-stop limit as
the sole cause. No further prompt lottery was run. The saved question reservoir
retains the omitted evidence; the current HTML displays only the selected guide.

Source inspection confirms that the local datadir call graph already has
ToBackendFileName -> ToSnapDir -> ToMemberDir and the literal fragments db,
snap and member, with a separate WAL helper and wal literal. Those calls are
not supplied in the question declaration rows. Future question work must
distinguish missing evidence, discarded anchors and unsupported behavioral
claims before changing another prompt. Existing native identities and entity
knowledge are the starting point; no second graph or framework-specific path
interpretation was introduced.

A final local source review distinguishes transaction completion from physical
storage work. storeTxnWrite.End calls backend Unlock; batchTxBuffered.Unlock
writes back the read buffer and commits conditionally (batch limit or pending
deletes), while backend.run also commits on its timer/shutdown. WAL.sync flushes
and calls Fdatasync unless unsafeNoSync. Raft Ready sends committed entries to
applyc before its local storage.Save, and applyAll waits for notifyc after
applyEntries and before snapshot work. Consequently a single serial
Put -> WAL -> apply -> commit trace is not established by call reachability.
The ordinary storage candidate reservoir retains seven of nine manually checked
declarations in this slice, but only two survive the final guide; buffered
Unlock and backend.run are not candidates. Window 43 nevertheless contains
both declarations, including backend.run's existing hypothesis about periodic
commit. Thus this omission is selection despite an available relevant hint,
not a missing saved description or a cache miss. This is a narrow source audit, not
an end-to-end durability claim or product answer. It is recorded in the existing
question review and question-storage-multi-anchor-evaluation.json, without a
new provider request, graph, code change or browser-policy workaround.

The earlier question corpus had 582 code files and no `_test.go` files,
versus 401 regular test sources in the etcd checkout. Go discovery now retains
TestGoFiles and XTestGoFiles from its existing build-selected go-list result,
including rows for test-only directories. It parses declarations once through
the corpus reader, then passes them in memory through scoped Go facts into the
same ProgramIndex, places and question rows. No second compiler/SSA load or
test execution is introduced. Ordinary module packages remain distinct from
test-source containers; external test packages do not become new products.
Module components own tests of all their non-main packages, including private
packages, plus test-only directories. An executable owns tests of its exact
package, not tests of dependencies it imports. Parsed declarations are internal,
have no public callable identity or launch seed, and carry only declared
containment with a go_test_declaration witness. Declared imports remain in
the producer facts and are not inferred call edges. Parse failures preserve
the known source with unavailable declarations and a warning. Build tags and
the supplied corpus determine selection; excluded files are not reread.
The cumulative Go fixture checks same-package tests, external tests, a separate
test-only directory, a private production package, build-tag selection,
methods whose receiver is declared in production or later in test source,
independent mutable ownership, unavailable syntax and exact question anchors.
This supplies possible examples to inspect; setup, assertions, test-to-operation
calls and behavioral coverage still need analysis. A separate test-only
directory in a module with executables but no module-library component still
needs explicit ownership in the product; it must not be assigned arbitrarily
to every executable. Repositories made entirely of test-only packages are also
not yet product targets.

Ordinary online etcd acceptance 20260906-065354-etcd-8a1b15ba9192 completed
27/27 targets and published one common report in 17m20s (the report's timing
snapshot before final publication is 1032.657s). All 398 build-selected regular
test files reach places, including 82 under tests/integration and 58 under
tests/e2e. Their indexes contain 3,131 parsed objects and their question rows
2,978 declarations, with no test launch seeds or inferred test calls.
The guide for finding an MVCC write-test example now selects TestStorePut at
server/storage/mvcc/kvstore_test.go:61, TestTxnPut at :714, and the test commands
under CONTRIBUTING.md:132. Manual source inspection confirms these are useful
write-test and run-instruction pointers; this does not establish assertion or
execution understanding. One invalid JSON question window left 12 of 2,656
chunks unresolved, and the visible guide correctly remains partial. Other
refusals affected eight symbol descriptions and twelve zone assignments.
Structural checks verified all target indexes/dependencies/groups, the saved
graph binding, physical common artifacts and source links. Browser inspection
found no console errors or broken internal anchors. The Python acceptance
fixture 20260906-064626-python-tutorial-game-8ec9b687878b completed in 5.471s
with zero live provider calls and unchanged accepted facts/answers.
The test-source navigation foundation is accepted within these limits; a
complete behavioral testing guide and build-variant support are not delivered.

The next Go-specific design is a small build/run-variant catalog, not a
union of every tag. This is a proposal, not implemented behavior. Today
GOTAGS selects one canonical run-wide tag set and the Go platform is selected
separately; inactive files and alternative contexts are not exposed in the
report. The owner's follow-up sketch is a repository-local .repomap.conf
where the person running repomap can supply additional conditions per target
and questions they want the report to address. Repository-wide questions
are now accepted as a `questions` string list; named target build variants remain
a sketch. The local command is repomap conf [repository]: create
.repomap.conf once and open it, preserving existing bytes and comments.
It uses YAML with an editor argument vector and optional reading questions, initially
`editor: [code, --goto, '{{ .File }}:{{ .Line }}:{{ .Column }}']`.
Each argument is a Go text/template over absolute File and one-based Line and
Column; expansion never resplits the file path. The command runs directly in
the selected repository directory with inherited terminal streams. The same
opener handles the configuration and served source links. The ordinary run reads
settings once and passes the Config value to child targets and the report server;
source clicks do not reread the file. A separately constructed report server
loads settings only when no in-memory Config was supplied.
An editor change takes effect on the next repomap launch. A missing editor
shows a short corner error pointing to the editor setting in .repomap.conf;
it does not stop report serving or replace an existing file. Unknown
configuration fields fail explicitly. The root configuration is outside the
analysis corpus and provider inputs, so editor edits do not change analysis
hashes. There is no parent-directory search, global home config, Git requirement,
format migration or alternate editor fallback. Global configuration remains
deferred. Owner questions supplement useful product defaults and use the existing
graph and accumulated descriptions. The reader prepares question evidence once,
then retrieves and selects independently for each question. Configured questions
come first; repeated --question flags append distinct, trimmed texts. At that earlier per-question stage, reordering or adding questions did not alter
another question's provider request. The shared final-answer batch described
below now invalidates its whole window when a question is added. Each
question has hash-prefixed table references, so its journals cannot overwrite
those of a neighbour. The common report contains all routes in memory and in
JSON, with collapsed guides below the map. No automatic question collection is
selected for the owner, and no query triggers another compiler or atlas pass.
Ordinary fixture runs 20260906-080755-python-tutorial-game-9373acf26f25 and
20260906-080807-python-tutorial-game-b17ccbe6b088 read two configured questions,
then those same two plus a third CLI question. Both completed with two targets,
identical graph hashes, and ready guides. In the second run the existing four
retrieval windows and two route windows were cached; only the new question made
two retrieval calls and one route call. The temporary fixture configuration
was removed. Receipt: work/questions-map-acceptance.json.
Final ordinary run 20260906-081323-python-tutorial-game-19b48e2b215e
completed in about five seconds with all three questions and zero live model
calls. Its HTML includes the moving safe triangle and collapsed question list.
The preview event-handler regression checks immediate docked node switching,
transfer through blank space, Escape, and releasing a floating preview when the
pointer leaves the corridor advanced by its previous movement. It does not
claim a continuous mouse test in the browser, whose API still lacks pointer move.
The focused configuration, source-opening and corpus tests pass, as do the
changed packages' vet checks and make build. The built repomap conf created
and opened the working repository's local file successfully. Ordinary online
fixture run 20260906-074859-python-tutorial-game-af3833c6de0e published both
targets in about six seconds, with zero live provider calls despite adding
the local editor config. Its temporary config was removed after verification.
Explicit owner knowledge supplies missing context; automatic
discovery can expose conditions and suggest anchored contexts without having
to infer every script. Several named variants remain alternatives, never a
union of their tags. Configuration must resolve to existing repository parts;
unknown or ambiguous targets are visible errors, not guessed assignments.
A variant would retain its package scope, effective Go build context
and anchored source command. Labels such as "Integration tests" describe the
command; they never assign semantics to a tag merely named integration.
First retain corpus-file conditions (build expressions and implicit platform
suffixes) and go-list selection/ignored-file evidence so an inactive source
can be found without pretending it was analyzed. Then discover concrete
command contexts in build scripts, CI and documentation. Model explanations
remain hypotheses; Go's own file selection and, when performed, type checking
are separate facts. Listing selected files is not proof that a test builds or
runs. Unknown script expansion remains unresolved rather than inventing tags.
Do not enumerate the powerset of tags. Analyze requested or repository-defined
contexts separately, preserving context on their calls and symbols within the
existing pipeline. Reuse content-identical evidence and model rows; a context
label alone must not force another full repository analysis. The static report
can switch among already analyzed variants, never launch analysis on hover.

etcd at 58f45a9ff1c0 gives the first acceptance example: scripts/test.sh's
integration_pass runs tests/integration normally and tests/common with
-tags=integration; e2e_pass runs tests/e2e normally and tests/common with
-tags=e2e. The two common/*_test.go setup files assign different testRunner
and clusterTestCases implementations and declare overlapping helper functions.
The same TestKVPut has two cluster-run setup contexts plus the default
compilation context: unit_test.go has !(e2e || integration), returns no cluster
test cases, and selects UnitTestRunner. That runner fails outside short mode
and does not call m.Run in short mode. Seeing this test typecheck in the default
context therefore does not establish a cluster test execution. A development
packages.Load probe checked fixture tests, MVCC tests, and common tests in the
default, integration and e2e contexts; all five loads typechecked. Tests:true
introduces distinct compiler package IDs with the same PkgPath, including the
augmented package and generated test main. It cannot simply replace Tests:false
in the production loader, whose universe requires unique PkgPaths. Test calls
also include conversions, callable variables, interfaces and nested callbacks;
typed declarations alone cannot turn these into concrete execution edges.
This is investigation evidence, not shipped test-body or variant analysis.
Enabling both tags is not a valid combined context. A first useful answer
should connect a test to these source commands and selected setup files, without
claiming that the test was executed or that its assertions were analyzed.

The documentation gap is now addressed in the same places graph: every corpus
Markdown file contributes source sections, with complete text including fenced
examples, later paragraphs and original link targets. ATX headings outside
fences split sections; other Markdown syntax stays as text. These are author
claims with exact source extents, not code-file groups, call edges or inferred
target ownership. Question v5 reads each section in lossless 4 KiB excerpts;
long lines split at UTF-8 boundaries and retain exact line/column anchors.
No initial model reduction chooses which documentation survives. Graph/input
are v7; question-routes.json v1 collects v5 routes, report v81; old formats have no adapters. Commands
are evidence to inspect, not executed or asserted to work. This change does
not extend matching's separately blocked caller/callee context.
Ordinary etcd run 20260906-055542-etcd-12319d77cce7 finished in 228.231s,
27/27 targets, with 120 live question windows and 16 live route windows;
the base atlas stages reused their cache. All 67 Markdown documents became
1,485 sections, preserving 669,176 bytes and exact source line extents.
All 2,227 question chunks were inspected. The first guide stop is
CONTRIBUTING.md:132, whose original excerpt retains make verify, make test-unit,
make test-integration and make test-e2e, plus the distinction between tests
required for every change and integration or e2e tests for new features.
Route v5 nevertheless asked which test suites to run; the fixture likewise
asked for the exact dev-server command after selecting the npm start section.
Both were selection errors despite the original commands reaching the route
input. Route v6 checks selected excerpts before declaring an open question.
The same evidence now leaves the specific new test file/package unresolved
for etcd and no open question for the fixture. This narrow before/after check
is in docs-route-comparison.json; redundant parent README stops still remain.
Final ordinary etcd run 20260906-060335-etcd-af4c4a0663ef finished in 93.835s,
27/27 targets, with zero live LLM requests, including all question/route windows.
The fixture run 20260906-060335-python-tutorial-game-d0643b96d36f finished in
5.238s, 2/2 targets, also with zero live requests. The common report/artifact
hashes, current formats, callable references, HTML IDs and internal anchors
passed checks. Browser inspection confirmed the guide, revision-pinned
CONTRIBUTING source link and remaining question, with no console errors.
Receipts are etcd-documentation-v6-acceptance.json and
etcd-documentation-evaluation.json under the project work directory.
This validates documentation retrieval for these two questions, not test
implementation selection, default orientation recipes or universal matching.
Do not spend repeated model calls trying to recover sources that are not
supplied. The question review manually checks the concrete cancellation path,
including command-timeout, ContextError, retry cancellation and stderr/exit.
Those manual answers are acceptance examples, not generated product output.

Superseded artifacts from the 21:35 full experiment (27 directories, 617 MB)
and the 22:03, 22:09, 22:15, 22:33 and 22:39 experiments were removed after their
replacement passed. The shared cache and earlier user reports were preserved.
The superseded 20260906-051922-etcd-a2bf12cd82a5 cohort (27 directories,
638 MiB) was also removed after checking its exact portfolio membership and
replacement; docs-obsolete-cohort-cleanup.json records the removed paths.
The shared LLM cache and current complete report cohorts were retained.
Focused tests and vet passed
for atlas, native Go discovery/adapter, GroupsIndex, report, run, debug journal
and report server; `make build` and `git diff --check` passed.


The etcd report exposed a shared-root ownership defect: the first target at a
root took every file from its library/executable sibling. Places now retains
all indexed owners tied at the deepest root; a nested target still owns its
own subtree. Input order does not decide ownership. Configuration reads no
longer put an entire directory in the outbound integration lane, and entry
seeds are checked against the current target's file membership.

Go TODO extraction scans comment tokens, preserving physical source lines;
`context.TODO()` and string literals are not comment markers. Other languages
still use their existing line matching. Every readable text file is scanned;
the former whole-file 1 MiB cutoff silently lost all markers, even at the start
of an otherwise ordinary source file. Cumulative Go, Python and TypeScript
examples now retain source-distinct markers on both sides of that former size
boundary, including CRLF and Go physical-line anchors. Dependency facts retain the complete
imported package path instead of only the short package name.
Claims likewise read each eligible README/source file completely through the
existing corpus reader; a whole file larger than 1 MiB no longer silently
loses all its quotes. The existing quote selection, UTF-8 policy and physical
anchors are unchanged. Cumulative README/Go/Python/TypeScript regressions keep
their complete original claim sets, dates, ownership and seals after padding
moves the same text past that former byte boundary.

The overview graph groups parts by their interpreted role, uses short
repository-relative labels, and distinguishes executable/library nodes by color
and languages by small badges. The detailed catalogue remains collapsed below
it, and a compact chooser also handles navigation. Edges remain visible at low
contrast; hover emphasizes connected paths and reveals their labels. TODOs are grouped by file, unreached
files by directory, and the raw dependency inventory is subordinate and
collapsed. Open evidence lists keep their collapse control visible while
scrolling. A file link has no synthetic line-one label. Missing entrypoints
mean reachability was not assessed, rather than that every file is reachable.

The static zone layout reserves header and padding before the next zone. Both
interactive maps reserve a description strip above the graph, so text never
covers nodes or paths. Explorer descriptions lead with the retained summary
and navigation; code, witnesses and connection details expand separately.
Initial/reset scale is determined by label size alone; a single node does not
stretch to fill the stage. Explorer names are 16 px and auxiliary labels 14 px
at native scale; nested code and source links cannot compound font reductions.
The earlier typography-only checks missed the clipped overview and oversized
single-node map. Current acceptance includes actual screenshots of both sizes
and the component-to-backend journey, not just font measurements.
Hover emphasis softens unrelated outlines and arrows without fading node text.
Overview and explorer use the same ELK.js 0.12.0 layered layout and orthogonal
router. Its unmodified bundle and EPL-2.0 license are embedded in the report;
there is no CDN request or new analysis service. Nodes are placed from their
connections, rather than alphabetically before routing. Existing overview role
categories become expandable areas: the first category is expanded initially,
and All components deliberately opens the complete graph. A downward layout is preferred when it reduces
horizontal overflow; label size and graph membership do not change. Both
orientations start from clean inputs because ELK mutates its graph. White
casings distinguish a crossing from a junction. This does not assert that an
arbitrary dense graph has a planar layout. Folding the retained hierarchy and
restricting a deliberate scope are the primary density controls. Layout requests
carry a generation counter, so a slow previous selection cannot overwrite a
newer scope. Source observations and model interpretations remain unchanged.
Docked details follow a new hovered node immediately and retain the last
selection across empty space, allowing transfer to the panel without a triangle.
Leaving a node clears its graph emphasis independently of those retained details.
Structure is the initial view, independent of whether operations were found.
Operations is an explicit mode with a stable searchable chooser and a filled
selected operation. Hover previews before a selection; click pins it without
navigating to code. An explicit matched endpoint also pins its destination
operation. Source links open separately or use the configured editor. Opening
an area keeps the operation and filters its children to native reachability.
After a scope layout, a motionless pointer cannot select the node that happens
to move beneath it. Real pointer movement or keyboard focus resumes inspection;
this rule uses coordinates, not a delay.
An inspected group can expand one shortest native call path from the selected
operation, with source anchors and possible-call markers. It explains static
reachability, not a recorded execution or all behaviour of the group.
Group cards put navigable connections first, then already interpreted key
subjects with descriptions and operations. The full symbol inventory is
collapsed and grouped by file. No first-three-source-order representative
selection remains. Matched connections link to the exact peer operation and
show both source endpoints; a local group can link onward to a neighbour's
integrations without claiming that all of them execute on its own path.
The active navigation goal includes two equally supported starting points:
structure (a responsibility such as storage, without knowing an operation)
and operations (a command, endpoint or background activity). Structure is the
default. Existing GroupsIndex containers supply the areas; no framework names
or repository-specific classification is introduced by the renderer. The old
operation renderer bypassed these containers and flattened all reached groups;
the retained etcd graph still has Storage engine, Consensus and replication,
API and networking, and the other model-proposed areas.

Drilling into an area or group must keep the selected operation, if any, with
visible breadcrumbs, return navigation and an explicit Clear selection switch. A
group's general source card remains available separately. Search must find
areas and groups by their retained names and descriptions, independently of
operation names. The storage journey must expose its actual interpreted parts,
their code and the operations using them without starting from a command.
Clear selection keeps a short label; its hover help names the selected operation.
Cross-component operation links use the common explorer navigation, preserving
the original question, reading mode and the source visit for browser Back.
The selected code explanation aligns with its selector and uses a small
provenance information mark beside its source, without a repeated heading or
an inset bordered panel.
In a selected part, the short Back to part action clears only the selected
code explanation through the existing picker event. It preserves the part,
operation and question, and returns keyboard focus to that exact source's code
button. It is not a browser-history step or a reset of the map.

Breadcrumbs follow the existing containment hierarchy, not click history. They
distinguish Area and Part when the model gave both the same name; opening the
current node is idempotent. A boundary peer never contains an already displayed
part: shared ancestors are expanded just far enough to show disjoint peers.
Changing map scope does not scroll the document. Explicit links into the map
reserve the measured sticky toolbar height, and the breadcrumb row scrolls
horizontally rather than changing the map's vertical position. Initial layout
uses the map's available width, independent of when the shared inspector mounts.
Explicit map destinations set their intended scope, operation and search state
before recording the browser address. Search and question links use that same
transition. The destination page is revealed before asynchronous layout measures
it, so Back and Forward retain the intended map state while layout is pending.

The reading canvas is now a light mineral grey-green with white content and map
surfaces. The toolbar has one background; Home, Questions and the part picker
share an aligned control row, with a separate count and disclosure chevron.
Key code has one shaded file header per file and aligned symbol/description
rows; narrow cards stack each name above its explanation. Operations have their
own rows and kind labels. Existing model styling and source controls remain.
Browser acceptance on `20260906-193401-python-tutorial-game-00a4aab7514c`
checked native pointer clicks between both backend groups, repeated clicks,
parent navigation and the Field continuation/source. Document scroll stayed at
154.5 px across those map clicks; the inspector stayed 192 px high and revealed
the source when scrolled. The earlier 192800 build was also inspected at 900 px
and 600 px widths: the picker was visible, the page had no horizontal overflow,
and key-code rows stacked at the narrow width. Final ordinary run: six seconds,
zero live model calls. Reading/report/run tests, vet and build pass.

A later fresh-page check caught a specificity regression: the generic
non-operation inspector rule overrode the fixed overview and explorer heights.
On `20260906-210546-python-tutorial-game-71fe31d217bd`, the first hover increased
the overview inspector to 156 px and shifted the clicked node by about 61 px,
so a native pointer click missed. The generic auto-height override is removed.
The ordinary `20260906-211442-python-tutorial-game-858745b4d0fb` run finished
successfully in six seconds using the existing model cache. The overview
inspector now stays 112 px high and its first coordinate click opens front.
The explorer stays 192 px high; local clicks through Frontend components,
Interactive components, Routing and services, HTTP service and the backend
boundary keep document scroll at 96.875 px, with containment breadcrumbs.
Clicking the matched POST /api/level/run then opens that backend operation
inside the same report, visibly selected. The mineral canvas was also checked
in the browser as rgb(238, 242, 239).

Ordinary report `20260906-215453-python-tutorial-game-31acae9b84c8` includes
the JSX interaction review: run simulation, change slider, change slowness and
toggle play, alongside the continuous animation. The source chain is
StyledButton.onClick at playground.tsx:117, PlayGround.handleClick:60,
runLevel called at playground.tsx:72, and axios.post at service/http.ts:34.
In the browser, selecting run simulation preserves the operation context
through Routing and services / HTTP service and the backend boundary; clicking
POST /api/level/run selects the backend operation in the same HTML.
The predecessor ordinary run with the changed native graph took 2m39s; this
unchanged analysis rerun took eight seconds with zero live calls. The real
TypeScript 5.9.3 suite, atlas/group/report/contract tests and focused vet passed.

That browser check also exposed width-only `fit`: a vertical four-node map
grew beyond its viewport with 231 px high nodes. Fit now considers the stage's
height limit and width together and never enlarges past the reading scale.
The same map fits in a 645 px stage with 78 px nodes and no internal overflow.
Reset still restores reading scale. This verifies the small fixture; the
larger etcd routing/layout acceptance remains separate.

Repository search now finds the names and descriptions of existing components,
areas and operations, plus code entries already present in the HTML. It opens
the selected part on the existing map rather than hiding cards far below the
current viewport. Parts and operations come before the source inventory, with
exact names first within each category. Type and component filters, twelve-row
display pages and the source links preserve access to every match. This is text
search over current descriptions, not a natural-language answer engine. Opening
a search result starts in the complete structure, or pins that exact operation;
an earlier unrelated operation cannot silently restrict the result.

The owner approved Learn/Work and clarified that Learn must keep the map at
the centre of the page. The first implementation uses the existing repository
role hierarchy and connections, already folded to the primary applications
and expandable areas. Learn adds the short repository explanation and routes
to area responsibilities, running the project, and unfamiliar terms. Work
opens search and the same map. Maps remain mounted when changing modes;
selection, operation scope, filters, zoom and inspector must stay intact.
Only the active report section is shown with scripting; ordinary anchors,
search and question links reveal their exact existing destination. The URL
stores the mode and destination. Every section remains in static HTML.
Term search exposes the existing key-type explanations, eight collapsed
entries per display page, with source links and every owning map destination.
It deduplicates identical explanation/source pairs without merging distinct
executable/library owners. It adds no model call or second semantic authority.
This is the initial navigation layer, not completion of the Learn experience:
useful guided questions and system-level terminology still depend on the
quality and completeness of the existing analysis. Ordinary/browser acceptance
is being checked on both repositories before delivery.

The owner refined the next Learn step: maintain a curated base of general
learning intents, not a fixed list of questions copied into every report.
Existing terms and key types, core areas, integrations, README and document
sections give the model the context to propose useful repository-specific
questions. One base intent may produce zero, one or several questions;
inapplicable intents are omitted without a minimum count. For example, a
storage intent may split into questions about primary state, a log and
temporary data when those distinctions are supported here. Overlapping
questions from different intents should share one answer while retaining the
parent intents. No etcd-specific taxonomy belongs in the renderer.

Applicability and answer availability are separate decisions. A failed model
request or missing evidence does not justify declaring a topic inapplicable.
Keep the adaptation decision and its grounding inspectable; useful unanswered
questions remain visible with the narrower missing information. Explicit user
questions from `.repomap.conf` or the command line must receive a visible result,
including an explanation when inapplicable, rather than silently disappearing
under the automatic-question filter.

Automatic question generation runs after the ordinary atlas through the configured
client. `reading/learning.go` and embedded `prompts/learning.md` prepare the
eight curated intents, losslessly partition existing documentation, file purposes,
key declarations and boundaries, and accept zero/one/many proposals with original
anchors and reasons. Consolidation compares every proposal pair when context
requires multiple windows and retains all parent intents on the shared question.
Unknown context and positively supported inapplicability are separate. Generated
questions use the existing retrieval, route and answer stages. Explicit questions
are preserved; exact duplicate wording shares a result with both origins. The
report exposes each question's reason and original source excerpts, plus the
intent reviews. `read --through learn` stops before answers. The stage is registered
with the shared semantic journal; request/response payloads use the shared cache.

On 2026-09-06 the owner explicitly resolved the earlier automatic approval
rejection: automatic Learn generation is authorized in ordinary repomap for
user-selected repositories through the configured LLM client ("да, все что
потребуется"). No further confirmation is required for that integration. This
does not change the separate earlier matching-context expansion decision.

Term explanations are reused beside answers when an answer selects the exact
named declaration and source location. They are collapsed by default and retain
the original explanation, source and every map membership. The shared term
catalog has addressable entries; following a term reveals its page even through
an existing search filter. Name-only guesses and another type at the same source
line cannot supply an answer's term definition.

On 2026-09-08 the owner extended this into one shared glossary: collect terms
alongside analytical cube results, reduce them, bind them in the backend report,
then translate and decorate visible text. `internal/terminology` owns the
embedded adjunct prompt and closed source contract. Source-bearing analytical
calls return `{result, terms}` in the same request. Each owning stage supplies
its actual JSON `ResponseExample`; the adjunct wraps that shape rather than
inferring it from prose. Table examples depend only on their column contract,
so batch neighbours cannot change a row's memo identity. Calls with no usable
source refs pass through without a terminology prompt or response protocol.

A term is a name and explanation, with its closed source refs restored locally
and the accepted request plus result row retained. It must occur in the computed answer and
have a valid source. Go finds exact occurrences and row scopes; the model does
not enumerate input declarations or construct JSON Pointers. Unicode script
boundaries include names followed by Korean particles without splitting Latin
identifiers or combining marks. Domain answers and optional terminology are
validated independently. A bad or absent term never rejects a correct answer;
rejected metadata is recorded separately in the existing journal and never
enters the glossary. One parsed response also serves memo-row restoration,
which only collects metadata for accepted rows and their original source scope.
Exact raw exchanges remain the cache/replay authority, including a domain-accepted
response whose optional terms were refused. Provider state remains the base
transport state so exact saved-byte replay refreshes the same cache.

Two real PyKrx Types responses placed optional `terms` inside `result` despite
the explicit outer-sibling instruction. The adapter discards that one misplaced
metadata field only when the owning answer example does not declare it, records
`terminology_metadata_rejected`, and validates the remaining domain normally.
Those definitions never enter the glossary. Legitimate owning `terms` fields,
unknown domain fields and coupled assignments keep their owning contract.
Independent atlas rows now isolate missing or invalid cells to their own row;
unused extra fields do not reject valid neighbours. Only accepted row keys
reach the terminology collector, including on cache reuse and memo recall.
Raw responses, request bytes and cache identities are unchanged; this is the
same optional-metadata boundary on live responses, warm reuse and replay.

The ordinary run shares one collector across analytical stages. After analysis,
an aggregate closed-ref reducer joins compatible domain definitions and chooses
one original explanation. Go unions original spellings and sources and retains
all variants and request provenance. Reduction request v4 asks for one
`{ref, representative}` assignment per input group. The representative is a
closed original variant ref; equal choices identify one output group. The group
owning that variant must make the same choice. Missing inputs, conflicting
assignments, chains and cycles refuse the whole window; no transitive repair or
local insertion supplies an omitted choice. Identical repeated assignments are
idempotent and unknown input refs are discarded. This replaces the model's
redundant output-members list after two PyKrx responses omitted its first
singleton group. Earlier accepted groups stay indivisible and all original
variants remain visible to later comparisons. It does not classify translation policies.
Equal names alone never establish equal meanings. Complete groups partition only
when the provider envelope requires it; a nonshrinking round records partial
comparison. A refused model window leaves its already accepted input definitions
separate and stops retrying them in that reduction. Cancellation, invalid local
inputs/configuration and persistence failures remain terminal. Existing native
code concepts enter the final glossary directly, with whole source anchors and
map/question destinations. There is no native-to-candidate-to-native conversion.
Distinct declarations on one line keep their columns and identities; the same
exact declaration can retain memberships in several components.
`terminology.json` holds domain candidates; `glossary.json` and
`ReportData.Glossary` hold the sealed domain catalogue. Reduction and translation
use the base provider without recursively collecting another glossary. Saved
`read` collects candidates without stages beyond the requested stop.

The ordinary run output and static glossary now expose the saved partial
comparison state. The glossary uses one quiet localized explanation before its
list; successful complete comparisons show no notice. This projects the existing
catalogue flag and introduces no new stored report format or provider request.
Live validation, provider and completion-envelope failures also write a rejection
record with an exact exchange link. Domain validation retains its actual reason;
provider failures use the existing closed error description, never raw transport
error text. Refused responses still cannot enter the accepted response cache.

Report format is v85; question readings v9 retain the accepted final-answer
request digest and exact result row for contextual term bindings. Domain glossary
catalogue v3 preserves those origins through reduction without sending them to
the provider or making them part of a semantic candidate identity. Display text
catalogue v5 uses translation contract v9. Each actual translation request
contains one local catalogue of exact spelling/explanation pairs and ordered
text entries with their applicable dictionary refs. Equal spellings with
different explanations remain separate choices in that context. No occurrence
or sense selection is requested. Partitioning rebuilds the dictionary for each
complete child window; accepted translation binding and source protection are
unchanged. This removes repeated definitions without dropping their meanings.

The owner's clarified 2026-09-08 design keeps every glossary name in its original
spelling. A later clarification adds an optional English Alias beside a native
declaration name, including in Russian reports. Symbols v6 and Types v6 request
that short label together with the existing explanation. The accepted cell flows
through Knowledge, atlas v3 and GroupsIndex v6 into the ordinary report. Native
names, IDs, locations and source links are unchanged. The alias is display prose,
not a new observation or another graph. The renderer does not infer a name from
its alphabet or shorten a description into one. Cards show the short alias with
the native code name; the full translated explanation belongs to the selected
detail. Both saved names can lead to one glossary definition by literal lookup.

The 2026-09-10 correction extends the same alias binding to an operation that
repeats its native declaration name: cards, map nodes and operation links show
the accepted English alias beside that original name. This includes continuous
and scheduled work. A distinct action label keeps its own meaning; commands and
paths retain their literal spelling. Operation names are excluded from translation, including
remote map references; descriptions still change language. The regression uses
a Korean declaration and checks both English and Russian rendered reports with
the original source and operation links. An ordinary run rebuilds the changed
display catalogue; old saved translations are not silently adapted.

Ordinary acceptance `20260910-052059` on `33c29c5c` published both fixture
components and one Russian HTML in 236.618 s, with 31 live exchanges and three
cache hits. All 416 display entries have translations from eight initial
windows, with no rejected exchanges. Browser checks retained the English
operation labels on the entrance and selected map, Russian descriptions and
exact source/navigation links. The Korean alias binding is separately covered
by the report regression; this English-named fixture is not positive Korean
model evidence. Receipts are under
`work/airflow-validation-20260909/followup-20260910/ordinary-alias-*`.

Translation receives names and definitions as context and returns only
translated text: definitions and surrounding prose change language, names do not.
Go looks up exact complete names in the final text and binds UTF-16 spans to the
existing display slots. Source-owned question context narrows the available
entries; an otherwise ambiguous spelling offers separate definitions. This is a
dictionary lookup, not a claim that code determined the meaning of an occurrence.
The same lookup works in English, translated prose and literal display slots.
Longest complete names win at a shared start, without overlapping highlights;
identifier and script boundaries reject HMMish while allowing Korean HMM과.
Code, commands, paths and links remain outside inline lookup. Their original
source placeholders are restored once before matching, without recursive parsing.

The preceding M-marker experiment and its per-occurrence model decisions were
removed. Ordinary fixture translation had swapped simulation/Field hints when
reordering words; PyKrx likewise attached ticker to DataFrame. These failures
were caused by the translator carrying source occurrence numbers into a new word
order. There are now no source occurrence numbers or translation-side meaning
choices to synchronize. A translated name that violates the spelling instruction
simply has no exact hint; the report never guesses an additional spelling or repairs prose.
No native-name guard, morphological matcher or additional provider call remains.

The static glossary supplies original names, translated definitions, source links
and question/map destinations. Browser hints show those same definitions beside
locally matched words. Display refs travel with prose slots and dynamic map
content; identical text in separate slots does not cause their definitions to
merge. Formal saved rendering uses the report and bound translations with zero
provider calls. Ordinary PyKrx and fixture acceptance results follow below.

The same-directory component collision is fixed in the shared label composition:
when both directory and component kind coincide, navigation retains the native
module name (for example, `pykrx.website.krx.etx.core`, `.ticker` and `.wrap`).
Localization changes only the existing kind suffix and reuses those exact
destinations for glossary, question and map links. It no longer reconstructs a
short label from the shared directory and loses the distinguishing module.
Part inspector actions stretch across their container with a transparent
background; the earlier squeezed white strip and mismatched action baselines
are gone. Existing SVG titles retain their font and prepared lines: the explorer
measures those lines and expands each node to keep its horizontal padding.
It does not add another wrapping algorithm or clip the title.

Literal-name acceptance before aliases completed on PyKrx `130937` and fixture
`132612`: exact bindings, source syntax, dynamic hints, Escape and named returns
passed independent browser checks. Name preservation by the translator remains
a prompt requirement, not a hard semantic validator: 14/77 inspected PyKrx name
occurrences and 35/73 fixture occurrences became translated ordinary prose.
They correctly have no literal hint; code and paths retained their exact bytes.
Alias fixture `135436` subsequently completed a warm ordinary run in 7.7 seconds
with zero provider calls, two complete target pages and 14 questions (ten answered,
four partial). Its native English names needed no aliases; this is not positive
evidence for the Korean-name case. Directed browser checks retained native Field,
sources, compact cards, hints and named return.

Final PyKrx `20260908-140335-pykrx-98a43799df92` completed in 618.4 seconds
with all 22 component pages and all 56 Types rows accepted. Its 19 final answers
shared one provider request: seventeen answered, two partial, no refused answer
rows. The five Korean declarations in `pykrx/website/krx/items/core.py` now have
accepted English aliases, including `Individual stock price trend` beside
`개별종목_시세_추이`. Native names and exact declaration anchors remain intact.
The fresh glossary reduction produced 40 domain entries; native code concepts
still enter the report separately through their existing interpretations.
The warm ordinary run `20260908-141435-pykrx-fdcc9785ebd9` completed in 62.1
seconds with the same question states and zero live provider requests.
Both runs retain every required owner and target-local publication artifact.

Directed desktop browser acceptance on final PyKrx checked all five original
Korean-name cards, their English aliases and original source lines, the selected
Russian explanation, and Browser Back restoring the same member. The footer
now spans its full container; the stock title fits with 11px horizontal insets.
The glossary uses the Russian model-explanation badge, and the answer's map
link aligns below its full-row label. No browser errors were observed on these
paths. Screenshots and the exact part URL are recorded in
`work/pykrx-final-alias-visual-receipt.md`. This is a directed check of the changed
presentation, not a new exhaustive review of every answer or glossary page.
An independent binding audit checked 510 literal spans and 875 rendered hint
buttons across the final report, with no wrong spellings, excluded-code hints or
broken question destinations. All five Korean/English name pairs were found
through both report and glossary search, sharing their original identities.
Selecting each of the five map cards retained both names, its Russian
explanation and the exact source links. The binding receipt is
`work/positive-alias-pykrx-bindings.json`.

Formal saved render checks on final PyKrx and fixture `135436` used isolated
copies of the saved report and response cache. Rendering before and after the
ordinary `cache clear` command produced byte-identical HTML to the corresponding
ordinary run, made zero provider requests and left the saved inputs unchanged.
The original response caches were preserved. Receipts are
`work/alias-acceptance-20260908-140335-pykrx-98a43799df92.json`,
`work/alias-acceptance-20260908-141435-pykrx-fdcc9785ebd9.json`, and
`work/alias-render-cache-clear-acceptance-runs-glossary-{pykrx,fixture}/acceptance.json`.

The shared final-answer batch also has an explicit failure scope: exploratory
fixture `130455` returned fifteen rows, but one `unanswered` row supplied no
required remaining gap. The table rejected the complete window and exposed its
fifteen answers as unavailable. No answer text was repaired or retained from
that rejected window. That report is not acceptance evidence.

Real answer exchanges also exposed ambiguity between the answer's string-valued
`sources` and the terminology array-valued `sources`, plus an owning top-level
shape competing with the terminology wrapper. The shared adjunct now explicitly
places the owning shape under `result` and declares term sources as a JSON array.
Malformed JSON is still refused unchanged. These prompt changes invalidate exact
requests; comparisons after them must not claim unchanged retrieval or evidence
unless those artifacts were explicitly checked. The final ordinary and warm
acceptance runs above use the current prompt contracts.

Closing provider timing includes live attempts whose transport, response, or
validation failed. A cached response's historical latency and failures before
any transport attempt do not add live work. The same shared observer supplies
these counts and the existing semantic journal; rejected requests do not vanish
from the summary merely because no answer was accepted.

Console events now use `[123.456 +2.310]`: seconds since the invocation began,
then seconds since its previous printed event. One logical event has one prefix;
continuation lines align under its body. Child targets share the console clock
while retaining separate provider accounting. Suppressed progress and accounting
updates do not advance the visible delta. Ordinary, read and replay errors and
long-call heartbeats use that same clock, preserving their existing output
streams. Heartbeat throttling compares wall-clock instants, not elapsed durations
from different requests: a shorter new request can no longer disappear behind
the preceding long request's duration. No request bytes or saved metric schemas
changed for this console presentation.

Ordinary acceptance on public python-tutorial-game: `20260906-182915-…`
completed in 146 seconds with 16 automatic questions plus two explicit ones;
the resulting answers were 14 answered and four partial. Its ordinary warm
repeat `20260906-183305-…` took ten seconds with zero live calls. Browser
inspection followed a question topic to an answer, its exact Field definition,
the source and the existing map membership. The same map context survived
Learn/Work switching; a term link cleared a stale catalog search and revealed
the exact term. Questions are grouped by their base intents, with explicit
questions separately discoverable. Focused reading/report/run/debugdump tests,
vet and the build pass. Tests cover zero/one/many proposals, original origins,
unknown refs, exhaustive merge comparisons, rejected consolidation, ordinary
answer handoff and warm reuse. Answer completeness still needs review: a
local-run answer can admit a missing launch command while saying answered.

Full etcd learning-plan checks exposed a separate consolidation failure:
66 source windows produced 634 proposals (630 distinct wordings), but two consolidation
responses omitted mandatory rows. Those results were rejected and no automatic
question set was published. Consolidation now advertises shared choice refs
once instead of repeating the entire list in each row, and its prompt explicitly
requires every input row even when representatives coincide. Learn timing starts
before proposal generation, including both proposal and consolidation work.
The revised plan-only run `reading-2242729849` completed in 51 seconds:
66 cached proposal calls, 25 live consolidation calls and no rejected windows.
It retained 630 questions, so technical validation has not yet produced a useful
large-repository learning menu. No full answer run for those 630 questions was
started. Question granularity and full etcd answer acceptance remain open.

A subsequent independent audience review (`reading-32349442`) kept 541 of 630
candidates, yielding 536 merged questions: still not a useful introductory menu.
That iteration compared candidates within each base intent, retaining
first-day/specialist reasons and original sources. Input-sized pools reduced until
they fit together or reach a fixed point, with no question quota; explicit user
questions bypass this selection. Consolidation requires its shared catalogue
and every assignment to fit one window, avoiding hundreds of nearly empty
windows. These stages keep their own prompts even with a proposal prompt
override. The small ordinary 192800 run retained 16 automatic questions plus
two explicit ones. The full saved etcd plan `reading-2480982289` completed in
2m48.65s: 66 cached proposal calls, eight live audience comparisons and six live
consolidation calls. It retained 370 of 630 candidates, and consolidation left
all 370 separate. This is not a usable introduction and no full answer run for
that menu was started.

Follow-up checks on the same input isolated the failure. A more explicit merge
instruction (`reading-3753884770`) and an additional learning-need cell
(`reading-1647307051`, 2m9.452s) again retained 370 questions. Replaying eight
real purpose questions through the existing client did merge some candidates;
a grouped-response experiment also merged that small sample but copied every
question when given the full 370-question catalogue. Replaying an unchanged
185-row original merge request with thinking enabled took 100.365 seconds and
13,316 reasoning tokens, again returning 185 representatives. Request options,
responses and timings are preserved under `work/learn-etcd-*.{json,log}` and the
shared payload cache. These are isolated experiments, not ordinary acceptance.
The extra cell and prompt changes were removed; provider defaults are unchanged.
The unresolved work is repository-level question formation and granularity,
not another unverified instruction to merge a long list.

Selection decisions now record whether comparison covered only part of the
candidate set. A reduction replaces a previous partial decision when it reaches
the complete surviving menu; a nonshrinking set terminates with its independent
decisions labelled as such. Focused tests cover both cases, complete catalogues
with all their assignment rows, retained original reasons and anchors, and warm
cache reuse without live calls. The report exposes the partial comparison beside
the selection reason.

Selection v3 replaces per-question audience ratings with one set-valued menu
decision for each original learning goal. Every goal row reads its full curated
goal and closed candidate refs, while all rows share one original catalogue.
The model composes the topic menus together, then the next round compares the
union of the chosen original questions. The same byte-bound partitions, fixed
point, executor and exact cache apply; there is no numeric question quota or
local semantic ranking. A missing or invalid goal row leaves that intent
unavailable while accepted menu decisions survive. Questions
shared across goals preserve all origins without duplicating the answer.
The report labels menu rationales as such; an omitted candidate is not declared
inapplicable or given an invented individual reason for exclusion.

The saved full etcd plan `reading-2628323038` finished in 1m3.825s, with
54.408s in Learn: 66 cached proposals and 15 live menu/consolidation calls,
zero rejected windows. It chose 23 original wordings and consolidated them to
22 questions from 630 candidates. All eight base goals remain represented.
This follows a 59-question per-intent set-valued experiment: a single flat
global menu omitted whole goals despite claiming coverage, whereas mandatory
per-goal rows keep the goals reviewable together. Some retained robustness-test
questions and overlapping introductions still need editorial acceptance.
The ordinary etcd run `20260906-203245-etcd-658b6f3a7346` completed in 57m23s
with 27 targets and 22 questions, using the older graph v8 binary. It is not
accepted as Learn quality: the answer marked `answered` for concepts and bbolt
only explains keyIndex/generation, omits leases and bbolt storage, and leaves
the remaining gap empty. Its retrieval also left 12 chunks unresolved. The
menu still repeats launch/concept/networking topics and contains four
specialized robustness questions. Completion of HTML and syntactically valid
answer rows do not establish coverage of the user's question. Exact findings
and rejection diagnostics are in `work/learn-etcd-full-acceptance-findings.json`.

The ordinary small run `20260906-203251-python-tutorial-game-d2f0ba59f350`
completed in 10 seconds with two targets, one common HTML/JSON, 16 automatic
questions and two explicit questions. All 18 answers have preserved evidence;
eight are answered and ten partial. The menu comparison was the only live call;
the unchanged retrieval, routes and answers came from the shared cache. The
grey-green background, aligned navigation, expanded run answer and answer-to-map
transition were inspected in the browser. Focused reading/report/run tests and
vet passed. Artifact checks are in `work/learn-joint-small-acceptance.json`.
That run answer exposed a concrete evidence gap: `backend/main.py:14`
is a known main-guard seed in the ProgramIndex target and facts, but the shared
places graph's file declarations contain only App and App.app. Question
retrieval therefore missed the existing launch observation, so the orientation
run recipe and question answer differed.

Graph and saved reading input v9 carry the existing facts result's observed
entrypoint seeds and manifest values as source facts in the same graph. They
retain the original location, target context and native launch identity;
manifest files need not be code files. No extra source reads, launch-command
heuristics, semantic groups or edges are introduced. References outside the
corpus, including excluded environment files, do not enter this evidence.
Learn can use these observations without a key-symbol model decision, and
question retrieval appends them after its previous reservoir. Earlier complete
request windows remain reusable. Answer verification displays the exact
entrypoint or manifest value at its original source link. Focused regressions
cover the real acceptance fixture's main guard at line 14, frontend start
script and Python requirement; persistence, native identities and provider
identity isolation are also checked.

The first ordinary v9 run (`20260906-205642-python-tutorial-game-9a0ba26f9c81`,
2m2s) retained 13 launch/manifest observations, but the run answer still treated
the observed main guard as an unknown launch. Two isolated answer-only reads
even guessed an unsupported framework import command. The answer prompt now
illustrates the difference between invoking an observed script entrypoint and
inventing an application object for a framework runner. The route prompt keeps
complementary launch sources for the requested components before repeated
documentation. No command is inserted by Go or copied from the experiment.
The third isolated read derived `python main.py` from the actual seed; its
unchanged retrieval and route were cached, with one 2.149s answer call.

Ordinary acceptance `20260906-210410-python-tutorial-game-51ddff6ae36d`
completed in 44s. A malformed provider JSON response left one route unavailable,
so the unchanged ordinary rerun `20260906-210546-python-tutorial-game-71fe31d217bd`
reused the accepted results and completed in eight seconds, with no rejected
windows. It has two native targets, one common HTML/JSON, and 13 questions:
ten answered and three partial. Launch prerequisites include the exact Python
requirement, frontend script and backend main guard. The explicit run answer
now offers the derived backend invocation but still labels undocumented
configuration as an open question; that qualification needs further editorial
review rather than automatic promotion. The generated prerequisites answer
gives both launches. All question and Learn records bind to the same v9 graph;
native/group bindings, source locations and HTML/report digest passed checks
recorded in `work/learn-launch-acceptance.json`. Browser inspection covered the
grey-green overview, the native click into that answer, and the main-guard and
manifest excerpts with their exact source links. Focused atlas/report/run and
contract tests, vet and diff whitespace checks passed.

The full ordinary etcd run `20260906-203245-etcd-658b6f3a7346` completed in
57m23s (27 targets, 22 questions), as recorded above. It used graph v8 and the
earlier answer prompt, so it does not validate the v9 launch changes. Its
technically completed answers failed the Learn coverage review; it is not a
live process.

The earlier answer stage also received the prior reader's unresolved question
as a labelled tentative gap. In the current small report that guess overruled
the observed backend entrypoint and left a supported launch command unresolved.
Answer v6 removes that extra task while retaining original evidence and labelled
model descriptions. Its prompt requires a partial status when an essential
requested step remains missing, and grounds runnable commands in documentation
or observed launch configuration rather than a plausible framework example.
An isolated v6 retry gave `python main.py` and explained npm and uvicorn, with
dependency installation still unresolved. The first attempt exhausted the old
8192-token ceiling; neither that rejection nor the successful development read
constitutes ordinary report acceptance. The new ordinary check is pending.

Learn proposal request v3 factors only repeated component names and area-model
hypotheses into a request-local context catalogue. Every evidence ref keeps its
complete original observations and source binding; local graph/source values
are unchanged. Each actual child request rebuilds its own context refs, and
preparation measures the same encoded form that is sent. The PyKrx review's
343 evidence items are distinct: 38 shared headers do not replace those items.
On that saved input the owner JSON shrank from 297,580 to 262,300 bytes with
all original contexts reconstructing exactly. Selection and merge are unchanged.

The question pass retrieves anchored evidence for all questions together, then
answers them in one shared reasoning batch. The 2026-09-08 AI/ML, prompt and
simplicity review removed the separate route selector: it read the same original
evidence, while final answers already returned source refs. The supporting guide
now projects those refs in their accepted reading order. It has no independent
open question or reduction-round authority. Question readings are v9 and the
answer contract is v8. `read --through question` isolates retrieval;
`--through answer` runs the combined final stage. `--through route` fails with
explicit migration guidance rather than silently selecting another operation.
The semantic journal accepts only active stages; `atlas_route` is no longer
a producer stage or an accepted new exchange. Existing saved journals remain
unchanged.

The batch carries exact original source records once, each question's own allowed
refs, scope and retrieval coverage. It does not borrow another question's
sources or reinterpret existing file connections as symbol calls. Canonical
question/source ordering preserves exact-request reuse on reordering; output
returns in the user's order. Exact shared-request caching remains authoritative:
adding a question regenerates the changed answer window. Per-question answer
memos are deliberately absent until their independence and cost justify them.
This differs from retrieval's existing per-question memo.

Ordinary preparation uses the actual provider envelope with no question quota
or 64 KiB planning cap. Resource refusals split independent questions first and
recompute each child's complete source union. Only an oversized singleton
question partitions its original evidence into separately anchored answer parts.
Every successful sibling survives. Each question's complete answer validates
independently: a malformed, missing or duplicate row leaves only that question
unavailable. Retrieval applies the same isolation to each complete question
decision within a chunk; missing decisions never become inspected negatives.
An unparseable model response refuses its whole window; it is not repaired or
used as a reason for speculative retries. Exact raw responses remain the cache
authority, and retrieval memos revalidate their original complete window before
restoring accepted questions. Local
preparation, configuration, cancellation and persistence failures remain errors.
Optional response-cache read, write or eviction failures are reported without
discarding an accepted answer or its source bindings; mandatory run artifacts
and observer failures remain terminal. Prose is not classified as an internal
reference by its spelling: source names such as `c1` and `c99` remain valid in
answer, basis and gap text. The prompt keeps candidate refs out of prose, while
the structured sources column alone resolves and validates those refs. No prose
repair, extra name catalogue or changed request/cache identity is introduced.
The same distinction applies to Learn proposal and review prose: native names
such as `e1` and `q1` are valid wording. Nonempty reasons and questions, complete
intent decisions and closed structured source refs remain required.
Missing evidence is unanswered; a refused model window is unavailable. A
substantive answer or not-applicable explanation needs its own source refs;
incomplete retrieval or partial evidence cannot establish inapplicability.

The exact baseline on PyKrx `20260908-113211-pykrx-04f78dfccdec` had 17 route
calls and 17 answer calls: 139,343 input and 137,638 output tokens, with summed
provider latency 19m04.929s. Fifteen final answers were accepted and two had
malformed JSON; 93.9% of accepted answer output tokens were reasoning. Shared
retrieval was one additional call (136,470 input, 15,220 output, 126.764s).
The run failed later on malformed orientation JSON, so these are stage baselines,
not a successfully published report. Combined-batch acceptance is in progress.

The final answer table requests provider-supported deliberate reasoning. On
2026-09-07 the owner rejected the arbitrary small generation ceilings after a
valid answer request exhausted its 8192-token reasoning/output allowance.
Table definitions no longer carry individual 2048/4096/8192-token ceilings.
All stages use the shared 128,000-token request envelope, subject to the configured
client ceiling; the guidance classifier, target portfolio, documentation and
orientation stages also no longer impose their former 32,768-token cutoffs.
Short answers remain a prompt requirement rather than a smaller
generation cutoff. The provider adapter encodes this reasoning preference
with native `thinking` on official DeepSeek and default `chat_template_kwargs`
on compatible endpoints. Shared question retrieval also opts into reasoning;
other tables and final display translation retain fast mode. Exact request and memo identities include the
effective output allowance and reasoning preference.
The answer explains supported parts, names any missing central part, then chooses
its status. The original question determines both coverage and depth: an overview
does not require exact payload fields or internal branches, while a question
explicitly asking for those details does. Silent omission of a requested part
cannot become a complete answer merely because the remaining prose is useful.
Prior model uncertainty must not overrule positive source evidence, such as an
observed Python script entrypoint. The answer briefly explains unfamiliar names needed
for the requested action. No second review stage, new graph or local semantic
status repair is added.

The 2026-09-08 PyKrx review found an overview presenting documentation's stated
network responsibilities as implemented retry/timeout behavior, while the
failure answer qualified the same source. Answer v8 requires author-only
behavioral claims to be attributed in the answer itself, in overviews as well
as detailed answers. A separate basis sentence does not qualify an unconditional
claim. A documented overview can still be useful and complete without invented
demands for runtime verification. No body retrieval or local prose repair is
introduced by this prompt change.

Acceptance does not establish that v8 enforces those semantic requirements.
The same-evidence DeepSeek comparison returned 19 rows with thinking enabled
in 170.132 seconds (23,235 input, 21,803 output including 18,359 reasoning
tokens), but the overview still made unconditional documentation-only claims
and the failure answer borrowed pagination from another question's evidence.
With thinking disabled, the response reached the shared 128,000-token output
envelope after 490.107 seconds: its optional glossary expanded to 3,866 complete
term objects before truncation. That response was refused, not salvaged.
Retrieval without thinking took 48.009 seconds versus the saved enabled
baseline's 148.354 seconds, but omitted the failure question's only retry-policy
document. These are individual development comparisons, not a latency benchmark
or grounds to change either ordinary reasoning preference.

The existing answer basis is now rendered directly below its prose, outside
the collapsed original-source checks. Its display ref and translated text are
unchanged, and one source control serves both paragraphs. This exposes the
qualification; it does not repair an unsupported statement in the answer.
Saved PyKrx and mixed-language fixture renders preserved all 19 and 14 bound
basis paragraphs respectively without provider calls.

Ordinary PyKrx acceptance `20260908-161339-pykrx-2f1079ca9048` analyzed all 22
targets and published 15 questions (13 answered, two partial), using seven live
calls in 502.699 seconds. New proposal and translation formats were accepted.
The failure answer attributed its documented policy and retained an explicit
implementation gap. The layers answer's basis still overstated its evidence
as corroborating declarations although all four selected sources were author
documentation; ordinary answer quality therefore remains under review.
On the exact prior 745-text translation catalogue, prepared request bytes fell
from 262,401 to 135,247 (48.46%); all texts and applicable definition pairs were
preserved. This is a byte comparison, not a token or price estimate.
The final-template ordinary rerun `20260908-162337-pykrx-ecc82f67a93c` took
56.031 seconds and reused Learn, answers and translations. It still made one
glossary call: both runs' reduction responses omitted the first group and were
refused, preserving all 81 original entries with partial-comparison scope.
Rejected answers were not cached, leaving repeated similar explanations and a
stage summary that incorrectly reported ready for a partial comparison. The
v4 assignment reduction and explicit partial-comparison display above address
those two defects. Ordinary acceptance `20260908-171343-pykrx-38058edc7fdd`
then retained all 81 original variants in 34 domain entries, including the
previously omitted BLD singleton and all seven KRX variants in one entry.
The complete 22-target report published in 185.003 seconds with two live
requests: reduction (4.962s) and translation of 746 display texts (113.497s).
Other analysis reused its accepted cache. Saved presentation rendering remains
provider-free.
The subsequent ordinary warm run `20260908-173859-pykrx-9a46f8f35679`
published all 22 targets in 69.591 seconds with zero live provider calls across
their journals (17 accepted exchange cache hits). Its 746-text translation took
110ms; the complete saved translation and 34-entry/81-variant glossary are
identical to the preceding acceptance.

A separate single-decision Learn menu experiment retained the same 19 candidate
questions and all eight curated goals without a quota. It selected 14 in
3.803 seconds, omitting development setup, HTTP sessions/authentication and
non-business-day behavior among other topics. A shorter list did not establish
better coverage; this experiment was not adopted. Ordinary intent selection
and merge remain unchanged.

Final-answer prose keeps its paragraph breaks and complete qualifications through
normalization, caching and publication. The answer, basis and remaining-gap cells
use the table's prose kind; only short label cells collapse whitespace or trim
length. The existing provider response envelope and output-token budget still
bound an answer. The prompt asks for short paragraphs, with distinct steps and
payload shapes separated. In Learn, the explanation has a separate model/source
control, so reading or selecting the text does not trigger a source preview.
Source checks group a selected type's original fields and methods in a compact
table. Each declaration and its author documentation keep their own source link,
including methods declared in other files.

Readability verification on 2026-09-07 used ordinary run
`20260907-062913-python-tutorial-game-564ff42337d5`: two complete targets,
sixteen cached current-contract answers, seven marked answered and nine partial,
about eight seconds and no live provider calls. Focused table/lines/reading/report/
run tests and vet pass. The common artifacts, native/group bindings, saved
question data, original answer prose and HTML anchors agree. Browser inspection
covered topic-to-answer navigation, separate endpoint paragraphs and compact
member tables with exact links. This is not full Learn or current etcd acceptance.
Receipt: `work/learn-readability-acceptance.json` in the desktop project.
The owner's prioritization review is kept in workspace
`repomap-questions-learn-work.md` (the original 100 IDs plus nine new scenarios)
and `repomap-ui-ux-review.md` (29 UI/UX items). Historical fixes remain visible
there until their stated user journey is accepted; code changes alone do not
close the owner's remarks.

The current small report also passed the Field-continuation browser journey:
the inspector shows More details and a lower visual boundary before the text
ends; its button reveals the final qualification and field.py:10 source link,
then becomes Back to top. The map does not shift, and switching to Work keeps
the selected Application core and inspector scroll. This verifies the existing
implementation rather than adding another scrolling mechanism. UX17 remains
available for the owner's usability review; continuous pointer transfer and the
large report are separate checks. Receipt:
`work/learn-work-field-continuation-acceptance.json` in the desktop project.

Source-check presentation now shares one location link between a declaration's
signature, author documentation and member-table heading, using the stop's link
when the original location is identical. A different original declaration still
gets its own anchor, and every field/method keeps its exact individual link.
The ordinary small run `20260907-083823-python-tutorial-game-fbf7b65fc9cd`
completed in seven seconds with no live model calls. Browser inspection confirms
the class path appears once instead of three times and the member links remain.
Report tests cover both the selected location and other original declarations,
including a cross-file method; report vet and whitespace checks pass. This
addresses UX29 without changing provider evidence or stored answers.

Coverage acceptance on 2026-09-06 compared the original six-source etcd
keyIndex/generation answer and four saved small-repository questions. Prompt-only
variants either still omitted part of the question or demanded unasked details;
the ordinary `20260906-220956-…` run exposed a regression to a guessed framework
launch command and was not accepted. Reasoning-mode replays returned a partial
etcd storage answer and the observed Python script invocation, at 6.953s and
24.619s respectively. The current route prompt independently selected ten original
etcd anchors instead of six, preserving Lease, Lessor, Backend and BatchTx.
These replays do not replace full current-version etcd acceptance.

The ordinary answer-v4 run `20260906-222116-python-tutorial-game-3c5fdacb140c`
completed in 5m33s, with all retrieval/route/map work cached. Fourteen final
answers were accepted; one provider response error left an answer unavailable.
The unchanged `222724-…` run completed in 25s with one live answer and fourteen
cached answers, no rejected windows. The final warm ordinary report is
`20260906-222822-python-tutorial-game-6ac1cc158644`: 6.656s, no live calls,
all fifteen answers cached, nine answered and six partial. Initial plus recovery
answer requests used 48,558 input and 43,687 output tokens (including reasoning),
with sixteen transport attempts. This cost increase is confined to final answers;
it is not a speed improvement. The unchanged v9 graph, both native/group bindings,
original answer anchors, single common HTML/JSON, internal links and warm answer
identity passed inspection. Browser checks covered the two launch commands,
their original main-guard/manifest/README evidence, a native jump to the backend
map, and the visible unanswered backend-testing part. No browser errors were
reported. Focused llm/deepseek/table/reading/report/contract tests and vet passed.
Receipt: `work/learn-answer-reasoning-acceptance.json`. Full etcd menu/answer
coverage, missing type fields/defaults, and some literal Markdown/meta wording
in model prose remain open.
Learn links questions to one shared question section; a question opens the
answer before the supporting reading route, with links to existing map nodes.
“Check this interpretation” exposes the model's brief basis and original selected
declarations or author excerpts with their own exact source links. It does not
relabel directory context or earlier model prose as a declaration's documentation.
No implementation bodies or additional evidence category is sent by this step.
When a question's selection reason contains an exact source excerpt already
shown in that same question's answer checks, it links to that existing check
instead of printing the body twice. Source location, column, excerpt kind,
text and ordered member declarations must all match. The reason, source link
and unique prerequisites remain beside the question origin. Other questions
and the global Learn review keep their own excerpts. The destination is the
existing check's visible summary, so native HTML can reach it without scripts;
the ordinary navigator opens the disclosure while retaining the question.
Summary anchors reserve the same measured toolbar inset as other destinations.
This changes only page metadata and markup, not saved analysis or the exact
translation catalogue.
The owner explicitly reaffirmed that useful model interpretations are welcome:
the operator must be able to check them quickly. Names, signatures and argument
names can support an interpretation without bodies; do not automatically turn
that into “unanswered” or demand runtime proof for every explanation. Recalled
model descriptions can contribute, with that reliance stated; they do not become
independent proof for the answer. Concepts should
be explained where an answer needs them and link back to their owning areas.
Learn presents the system map and a manageable set of useful questions, with
further questions inside the selected topic. Its separate term search remains
an alternative entrance, not the learning sequence.

Review 2 acceptance gap (2026-09-09): the ordinary fixture report
`20260909-054542-python-tutorial-game-4e37ca2333e7` calls the source-length
constant an active guard. At fixture revision `78714d34`, the corresponding
branch in `backend/app/app.py:76` contains only `TODO` and `pass`. The original
answer evidence includes the named constant and a prior model description,
but not that implementation. This is a confirmed overstatement in one answer;
successful publication, cache reuse and local validation do not resolve it.
Improving that evidence/attribution remains open; it does not authorize a
browser-side correction or a general rejection of useful name-based deductions.

Answer-stage acceptance on 2026-09-06: the ordinary two-component report
`20260906-170233-python-tutorial-game-83677a41a877` completed in 8 seconds with
zero live provider calls after an answer-only reading took 4.175 seconds.
The browser showed the short answer, its one-sentence basis, original excerpts
and exact map links. Earlier output leaked request-local candidate numbers in
prose, prompting a lexical rejection that was later removed because legitimate
source names could have the same spelling. The current boundary is described
in the combined answer-stage contract above.
Focused reading/report/run tests and vet passed. The ordinary 27-component
etcd report `20260906-170227-etcd-5cd6bdae2281` completed in 2m51s with an
accepted Lease explanation and inspectable sources, also checked in the browser.
It is not complete question-route acceptance: one previously refused retrieval
window was recomputed, changing the subsequent route pools; five intermediate
route windows were refused for choosing 7–8 anchors against a six-item limit.
The route is explicitly partial. The answer does not establish exact expiry
timing or a complete runtime trace. The v7 correction removes that selection
limit and preserves termination without cutting accepted sources. All current report memberships and HTML links passed the artifact
audit in `work/learn-answer-final-acceptance.json`.

Route v7 acceptance on 2026-09-06: the saved full etcd reading
`reading-2859291439` reused all 150 retrieval windows, inspected all 2,685
chunks and selected from 492 original locations. Its four rounds kept
103 / 38 / 12 / 6 locations across 13 / 4 / 2 / 1 pools, with no refused
window. The 20 route calls took 10.095 s; the complete saved reading took
21.227 s. The selected README now includes lease grant/TTL, attaching keys,
expiry and revoke, together with the Lease and Lessor declarations. Its short
answer explains TTL, attached keys and deletion on expiry/revocation; it still
does not prove precise scheduling or a runtime deletion trace.

The ordinary full report `20260906-172703-etcd-eab91f0f7a54` completed 27/27
components in 2 minutes with zero live model requests. The ordinary fixture
`20260906-173149-python-tutorial-game-f81404157eb1` completed in 13 seconds;
its changed route/answer inputs made four calls, while retrieval and atlas
descriptions reused their cached results. Both have one common report,
matching report digest, current question/places/input graph hashes, and exact
source locations and map memberships. Receipts are
`work/learn-route-artifact-acceptance.json` and
`work/learn-route-question-acceptance.json`.

Browser checks opened the short answers and their original evidence, followed
the Lease answer into the correct existing group, preserved its inspector and
viewport on switching Work/Learn, and returned using the new shared Questions
navigation link. Source links retain the captured revision and open separately.
Focused tests exercise seven/eight-source intermediate selections, byte-bound
complete partitions, a nonshrinking selection terminating with all 83 sources,
cache reuse of those independent parts, legitimate empty selections, refusal,
and refusal to render parts that disagree with the answer's source selection.
Reading/lines/report/run/debugdump tests, vet and diff checks passed.

The saved reading also exposed a missing journal-stage registration for
`atlas_answer`. It is now registered. The later ordinary fixture persisted
both answer exchanges with verified exact request/response cache payloads.
The full 172703 report predates that journal-only correction; its table
request/response references remain available. This closes that diagnostic
gap without claiming automatic Learn question generation is implemented.

The first Learn/Work browser checks passed on the two-component fixture,
including mode changes with an operation and inspector selected. The full
27-component ordinary etcd run `20260906-155417-etcd-309ddf67939c` completed,
but visual acceptance is still pending: the overview's graph layout placed
tests above the primary applications and made the initial map too tall.
Folding role areas alone therefore does not yet satisfy the newcomer journey.

Question reading stops now link to exact existing operations or all groups
containing their subject, preserving separate executable/library owners. A
boundary's existing operation ID is also a direct join. Where a boundary has
no corresponding ProgramIndex subject, its file can link to groups containing
declarations in that file, explicitly labelled "file in". That fallback locates
the source file; it does not claim that all those groups execute the boundary.
Documentation without such membership remains a source link. No new provider
evidence, model classification or graph is added by these navigation changes.

The owner's Lease example adds a concrete terminology acceptance requirement:
an unfamiliar concept needs a short explanation of what it means here, which
objects it acts on and its important lifecycle, with source pointers. For etcd,
the manual reference is api/etcdserverpb/rpc.proto:107-117: a lease has a TTL,
keep-alives renew it, and expiry/revocation deletes its attached keys. Calling
the area "Lease management" does not explain this, and Lease must not be
presented as a generic mutex. The existing search can locate the area; this
complete lifecycle explanation is not yet a generated product answer.
Hard-coding an etcd glossary in the renderer would not satisfy the goal.

The terminology improvement stays on the existing symbol knowledge:
types receive their native-owned declarations, including methods in other
files and explicit Go interface methods, with exact ownership and signatures.
Interface methods have no invented direct-call nodes or implementation edges.
Claims now capture documentation attached to those explicit declarations, and
type context keeps the existing bounded author quote rather than reducing it
again to its first sentence. Ordinary file/callable context remains unchanged.
Atlas graph is version 9 and saved reading input is version 10. The independent type table
`repomap.atlas.types.v6` returns an explanation, key flag and optional English
alias; callable Symbols v6 shares the alias cell in its existing review.
A type's file hypothesis
is not supplied as factual evidence. Bare names without owned declarations or
author documentation remain source entries without a generated definition.
An initial ordinary run demonstrated why: the model invented active/completed
states for an undocumented enum. That intermediate report was discarded; the
final ordinary run does not ask for those unsupported definitions.

The three-agent UI walkthrough on 2026-09-07 exposed another loss: the complete
type response was normalized as a 240-character label, so Field and Robot still
ended in literal ellipses after the inspector was fully scrolled. Type v5 uses
prose normalization and asks for short complete sentences. It also omits an
undocumented lifecycle topic instead of appending an irrelevant "no deletion
rule is documented" caveat. Original declaration evidence remains unchanged.
The preservation regression covers a complete qualification beyond the old
cutoff. Ordinary runs 094938 and 095737 use type v5; the latter's browser
repetition confirms complete Field/Robot explanations and source links.
Editorial quality remains under review: one API Robot row still includes an
irrelevant lifecycle-absence sentence despite the prompt instruction.

Map inspectors now offer one concept at a time from the existing group's key
type subjects. A type named in the group's title is selected first; all other
retained concepts are available through the selector. Selecting it changes
both the explanation and the declaration link. The text is labelled as model
interpretation, not an author quotation. The explanation, source and actions
have separate columns in a fixed-height inspector, avoiding a list of type
definitions or a layout shift on hover. Source links open separately.

Longer explanations can overflow this fixed strip. Its inner content now has
a reserved continuation control outside the scrolling area: a shadowed lower
edge and “More details ↓” appear when content remains below, and clicking moves
down to the explanation's source. At the bottom it offers “↑ Back to top”.
The control disappears when all content fits, and the strip's height stays
fixed. Browser acceptance on `20260906-184502-…` checked Field's initially hidden
source, one-click scrolling to its exact link, returning to the top and changing
the selected concept. The ordinary run took seven seconds with no live calls.

Ordinary acceptance on the final UI:
`20260906-143809-etcd-63d416dc47ad` explicitly selects only the server library
(26 s in the CLI log; 194 type descriptions; zero live calls, zero refused
windows). `20260906-143918-python-tutorial-game-89f14476c0a9` covers both small
targets (16 s; four type descriptions; zero live calls or refused windows).
Browser screenshots at 1322x992 checked Lease search → Lease management with
Lease selected, changing to Lessor with its own source, and Application core →
Robot. Text, source and actions fit without scrolling the initial inspector.
Native/group bindings, exact reduced overview, single shared report artifacts,
question-map destinations and concept source paths were checked in
`work/type-concepts-acceptance.json`. Focused atlas/group/report tests, vet and
JavaScript syntax checks passed. This is not full 27-target etcd acceptance.

The subsequent ordinary check `20260906-151708-etcd-b512db32e076` selects the
server library only (63 s in the CLI, 60.991 s recorded before publication;
206 type rows, 102 marked key, 26 live type windows). Its Lessor row now has
16 explicitly owned interface methods and the second sentences documenting
deletion. The model line states that revoking a lease removes its attached
items, and the browser inspector shows it with the exact declaration link.
`20260906-151728-python-tutorial-game-6f5f41c54187` covers both small targets
(16 s; four type rows; one live type window). Native/groups/set hashes,
single shared report artifacts, exact reduced overview and input-to-HTML
Lessor text were checked in `work/interface-contract-acceptance.json`.
Focused claims/atlas/discovery/adapter/contract/report tests and vet passed.
These reports are diagnostic evidence, not completed semantic acceptance.

Two material gaps remain. The Lease row still lacks the separate service
contract connecting expiration to key deletion. Without implementation
evidence, type summaries can also overinfer: Robot.collides became collision
prevention, although its implementation only compares identity and position.
Explicit Go interface methods additionally expose a pre-existing review
weakness: operation classification can treat a method contract as an actual
request handler. Native edges remain correct, but declaration-only syntax
must reach operation eligibility/review before this change is fully accepted.
Do not use missing direct-call-node membership as a proxy for an absent body:
some implemented native declarations legitimately have no such node.

The owner clarified that names, signatures, argument names and implementation
are successive useful evidence, as for a human reader. Names can support a
model hypothesis; comments are optional. Code should refine what is queried,
changed or returned, and following the caller is needed to explain what it
does with a result. Retain declared fields/enum values where native adapters
currently expose only a type name. Do not count these improvements as
completion of terminology question 13.

An owner-authorized incremental experiment now compares the same four types
(Robot, Field, Lease, Lessor) from the pinned public acceptance revisions:
names only, existing normalized signatures/arguments, own code without
comments, then the same code with documentation. Each variant uses the same
prompt and actual configured request parameters, through `repomap replay`,
twice. It adds no alternative product pipeline and is not report acceptance.
Requests and responses live in the shared `.llm-cache/payloads`; measurements
and exact references are in `work/concept-evidence-experiment/manifest.json`
and `measurements.json`. No key or Authorization header is recorded.
Input tokens per four-type request were respectively 835 / 1277 / 3727 /
4390. Provider latency was 2.02–2.52 s; output was 155–185 tokens. Replays
remain live generations even when the provider reports prefix-cache hits.
This small set does not establish repository-wide latency. Code made Robot
and Field descriptions more faithful, but Lessor still became a method list;
more input by itself does not guarantee a useful lifecycle explanation.
The saved 27-component atlas contains 627 lifted type candidates, not 627
distinct domain concepts. The fresh server-library count above is narrower.

Search acceptance is currently complete on the small ordinary report
`20260906-140300-python-tutorial-game-e2d2e24bb8d2` (6 s, zero live calls).
Browser inspection checked readable topic results, parts before the source
inventory, and operation selection followed by an unrelated part search opening
its complete Structure scope. Reopening results resets their scroll position so
the first title stays visible. The preceding run with identical question-link
code also checked the guide's exec source-file link opening Application core.
Report tests and vet passed; current JavaScript syntax and whitespace checks
pass. Artifact checks are recorded in `work/topic-search-acceptance.json`.

Large-repository acceptance remains pending. The final etcd attempt
`20260906-134909-etcd-9951db1e0fe1` failed after 5m45s because the disk filled.
Earlier persistence failures excluded the server library and etcdutl executable;
that changed directory/file inputs, causing 247 live semantic calls rather than
the expected reuse. This is an observed changed input after target failures,
not evidence of nondeterministic request construction. No final report was
published. Its 27 unpublished run directories were removed after preserving
the log, changed directory inputs and accounting in
`work/topic-search-etcd-failure.json`; shared caches and delivered reports remain.
Do not retry the large run until sufficient disk space is available. The older
complete etcd report below remains the delivered map, without this search change.

Connections follow the displayed level: aggregate between areas/components,
expand to their members on deliberate drill-down, retain direction, provenance
and the constituent relations. At a focused scope, boundary-to-boundary edges
do not belong to that view; opening a boundary explores those connections.
One shared geometry mechanism routes visible edges around nodes, with
separate directional ports and readable labels, instead of separate patches
for overview and operation curves. Folding is a view operation and never
deletes graph evidence or silently promotes a possible call to an exact one.
Acceptance includes storage → parts → users, an operation → area → group with
context/back/All uses, migrate, the Raft call witness, frontend → backend,
and both single-node and dense graphs. Actual browser screenshots and journeys
are required; font measurements alone did not catch the previous defects.
Navigation and routing acceptance: ordinary runs
`20260906-130947-python-tutorial-game-d719b156fce1` (5 s) and
`20260906-131200-etcd-06f70d7533b7` (1m51s), both with zero live provider calls.
On these final files, browser checks followed Interactive components → Routing
and services → HTTP service → backend → POST /api/level/run and reached the
exact destination operation without opening source code. On etcd, opening
etcdutl from the overview and pinning migrate retained that operation while
opening Admin utility. Structure → Storage engine → Backend storage exposed
its users; choosing backend.run retained the scope, and All uses restored all
five neighbouring parts. Back restored the previous area. The seven-call native
witness from main to Raft Transport.AddRemote retained its original source
anchors. Stationary-pointer layout changes no longer replace that selection
with another operation. Continuous physical mouse transfer was not retested;
the available browser API exposes clicks and keyboard focus, not pointer motion.

Screenshots of the final overview, API area and storage views were inspected.
The folded etcd overview fits horizontally at the inspected viewport; expanded
dense areas still need scrolling. This is not a promise of a planar graph or
complete matching acceptance. Geometry checks on the retained root, API,
storage and backend scopes preserve every displayed relation, produce repeatable
coordinates and find no edge segments crossing card interiors. The same checks
pass for a complete directed 12-node/132-edge graph. Workspace record:
`work/structure-geometry-acceptance.json`. Both final reports have one common
HTML/JSON/manifest, all 2/27 native-to-group hash bindings match, and every
internal HTML anchor resolves without duplicate IDs; 4/476 native call-path
payloads parse. Record: `work/structure-final-artifact-audit.json`. No browser
errors were reported. Focused report tests, report vet and whitespace checks
pass. Base-question coverage and general matching quality remain active work.
Floating source cards test each pointer movement against the previous safe
triangle, then advance its apex to the accepted point. Leaving that narrowing
corridor releases a pending preview immediately. There is no timed expiry for
pointer intent; a stationary pointer preserves its current selection.
Keyboard selection survives scrolling. Scripted map nodes remove their duplicate native tooltip.
The ordinary morning UI checkpoint is
`20260906-051922-etcd-a2bf12cd82a5`: 91.884 s, 27/27 targets, no live provider
calls and no experimental question. All native/group/report hashes, 15,509
callable references and HTML anchors were inspected. Focused report/contract
tests, report vet and diff checks passed. Fresh loopback browser inspection
opened etcdutl from the overview and checked completion and migrate with the
description outside the graph. Command positions relative to the stage changed
by less than 0.002 CSS px; keyboard transfer to the source link retained the
description and path, and Escape dismissed it. No console errors were captured.
The compact layout's DOM measurements put the description above the stage;
viewport-emulated screenshots were blank, so visual acceptance of that layout
is still pending. Continuous mouse transfer was not retested: the current
browser API exposes focus/click but no pointer-move operation. That checkpoint
fixed same-column routing only. Workspace receipt:
`work/etcd-map-inspector-final-acceptance.json`.
The subsequent overview routing checkpoint is
`20260906-053535-etcd-e812a0e34892`: 94.230 s, 27/27 targets, no live provider
calls. Sampling the generated paths against all node rectangles found 88
edge/card intersections in 051922 and zero in this report, with the same 27
nodes and 84 edges, all inside the canvas. A dense-grid regression checks both
directions, skipped rows/columns, narrow gutters, and arrowhead arrival.
Report/contract tests, report vet, build and artifact checks passed. Fresh
loopback browser inspection checked the server-library overview, the etcdutl
description and click-through, and migrate's highlighted path with the
description beside it; no console errors were captured. Shared line segments
and crossings between edges remain, and this geometry change does not establish
matching precision or complete mouse-interaction acceptance. Workspace receipts:
`work/etcd-map-gutters-acceptance.json`, `work/etcd-map-gutters-geometry.json`.
Model prose is dotted and inspectable: the whole passage highlights, and a
hover/focus/click preview exposes its saved citations. These are the model's
citations, not a reconstruction of every file in the prompt or a guarantee
that the cited line supports the explanation. Text without saved citations
says so. Static HTML retains inline citations for readers without scripting.

Development storage: use one `.bin/repomap` and the ambient shared Go caches.
The model cache is shared within a debug root, so creating many experimental
debug roots duplicates caches. Cleanup removed 797 generated global runs older
than 2026-09-05 and 46 archived historical binaries (10.71 GiB), preserving all
model caches, the shared Go cache, current reports and experiment inputs.

An urgent owner-requested disk cleanup on 2026-09-06 found only 282 MiB free.
The Go compiler cache occupied 17.55 GiB and the etcd run root 12.30 GiB
(including its 0.76 GiB model cache). These are measured totals, not a claim
that all this data was written by the last run. Removed only 54,109 compiler
cache entries whose modification time was older than 72 hours, validating
their paths, inode, size and timestamp before deletion. This reclaimed
7.68 GiB; available disk space rose to 7.98 GiB. Recent compiler entries,
module downloads, all model caches, source and delivered reports remain.
Both final terminology report URLs returned HTTP 200 after cleanup. The exact
receipt is `work/emergency-go-build-cleanup-20260906.json`. No retention
mechanism was implemented by this emergency action. Before further heavy
acceptance runs, measure free space and clean only this task's superseded
unpublished cohorts; do not let automatic continuation repeatedly consume
the recovered headroom or clear model answers to obtain space.

The owner subsequently explicitly requested old etcd reports be removed.
Cleanup deleted 739 dated run directories, preserving the latest completed
cohort, the active cohort and the open in-app report, and excluding all model
caches. Free space rose from 4.40 to 16.62 GiB (12.22 GiB recovered). Exact
paths and the keep set are recorded in `work/etcd-cleanup-20260906-1705.json`.
Many historical run paths mentioned above no longer exist; their measurements
are historical records, not promises to retain their full artifacts. The latest
small and full report cohorts passed their bindings/link audit after deletion.

Publication retains the small target/dependency catalogue and releases each
completed child ProgramIndex after persistence. Facts, places and group
projection restore one target at a time; restored indexes are not attached to
the portfolio. Places retains current-target native lookups, then only the
deduplicated graph observations and exact declaration owners across targets.
This supersedes the earlier load-once retention rule after the 2026-09-09
Airflow run exhausted memory and disk. The owner projects the shared report once and
writes one `report.json`, `run_manifest.json` and `report.html`; facts, claims,
orientation, atlas and both portfolios are also repository-wide owner artifacts.
Target directories retain thin bindings to shared project facts (see
ProgramIndex below). The server receives the
in-memory report, while a new process restores only the common JSON and manifest.
Report format is 77, without an old-format adapter. The former per-target report
generation and subsequent owner JSON reload/render step have been removed.

Acceptance on etcd revision `58f45a9ff1c0`: the two ordinary warm runs
`20260905-170032` and `20260905-170910` completed in 74.399 s and 74.606 s,
down from 148.383 s. Each analyzed 27/27 targets with zero live provider calls.
Publication took 3.705 s and 4.064 s. One 88,179,503-byte report JSON replaces
27 reports totaling 1,718,748,491 bytes. Compared with `20260905-160210`, the
final report differs only in format version and timing; graph, facts, claims,
orientation, target inventory and source paths are identical. HTML source and
navigation links and section IDs are identical too. Browser QA covered the
overview, target chooser and jump to the server map. Eleven unresolved
container anchors exist in both reports and remain a separate UI defect.

The detailed repeat attributes 21.543 s to target artifact preparation
(including copies, validation, serialization and persistence), 16.701 s to
native analysis and 8.474 s to common ProgramIndex/dependency projection.
These are measured wall spans, not a CPU profile separating their internals.
Native analysis reuses a Go workspace across targets within the run; completed
native indexes are not yet reused across runs by the model-response cache.
The public mixed-language fixture completed in 5.582 s with 2/2 targets and
zero live model calls. Focused run/report/server tests and vet pass, including
27-target publication without saved index inputs and serving after the saved
publication is deleted. No compatibility reader was added.

Acceptance: the 20260905-135910 etcd run analyzed 27/27 targets in 5m14s;
Go TODO facts fell from 472 to 288 and the testgrid library now has its own
four files and a group. Focused places, reading, facts, report, groupindex
and run tests plus vet passed. Browser checks covered pointer transfer onto
the preview, Escape, source-link navigation, grouped TODO/file disclosures,
collapse from the middle of a list, compact navigation and all 51 zone frames
across 27 maps (no intersecting frames). Rendering the saved report with the
latest preview handler took 1.70s, including JSON decoding and writing HTML.

Cache audit: three readings of the identical saved etcd input each reused all
4,374 entity descriptions and 41 whole windows, attempted zero provider calls
and took 1.64–2.75s. Between the preceding and current full runs, 2,223 entity
bases changed: all are explained by changed ownership and parent knowledge.
Of these, 106 changed only because ownership participates in the memo key;
another 129 retained identical model input but changed parent knowledge IDs.
This is excessive invalidation, not evidence of nondeterministic request
ordering in the repeated reading. The subsequent cache change below separates
exact model input reuse from current entity/provenance bindings. The full
program-index rebuild was not part of this isolated repeatability check.

The owner asked to implement and verify from extracted evidence upward;
HTML prototypes are for discussing possible reader experiences. The first
new development boundary is `reading-input.json` (current version 9): the complete
places graph and target metadata used by the ordinary atlas reading. The
normal run persists it before any atlas model call. `repomap read INPUT`
runs those same stages without compilers, corpus collection, orientation or
HTML. `--through` stops at a named table and saves only the work actually
done; partial readings never publish an atlas. `--prompt`, `--window-rows`
and `--input-bytes` control the selected table; unchanged preceding requests
use the shared exact-response cache. The input has one current format, with
explicit errors instead of compatibility readers.

File rows now use deterministic direct-caller evidence (path, documentation,
leading declaration names), alongside the directory's model description.
They no longer consume other files' generated lines, so all file windows
can run independently. The file prompt contract is v2. Tables also pack
complete rows toward a 64 KiB system+user UTF-8 planning size. This is
independent of provider envelope limits and does not claim to measure tokens
or comprehension. A larger singleton retains its complete evidence and shared
context in a request of its own; ordinary packing cannot refuse it merely for
exceeding 64 KiB. Explicit development `read --input-bytes` budgets retain their
hard bound for stage-planning experiments. The actual prepared request still
passes the shared provider envelope checks. Per-window prompts and
normalized results with source bindings accompany the raw exchanges.

Validation proceeds from deterministic fixture expectations, to frozen-input
readings and real provider calls, to the ordinary end-to-end run. The baseline
fixture completed 2/2 targets in 49 s; 29 file descriptions took 9 sequential
requests and 12.3 s. A concrete downstream quality defect remains: the last
main-flow sentence described frontend animation but cited backend exec.
This needs better evidence selection and semantic anchoring in orientation,
not a change to the renderer. See README for the executable iteration loop.

The small-window experiment exposed a deterministic bug: module-level Python
and TypeScript variables have `ContainerID`, while the places builder checked
`OwnerID`. It therefore dropped the exported `instructions` object from
`front/src/utils/instructions.ts` and sent an empty declarations list. The
builder now uses lexical containment and its fixture test preserves the
constant's exact source binding without promoting function locals. File rows
also distinguish `directory_hypothesis` from `directory_facts` and disclose
the total declaration count. With the repaired input the model described the
file as instructional text, including in the smaller-window experiment.

Measured locally during this change (single runs, not a performance guarantee):

| workload | result |
|---|---|
| fixture, original cold model cache | 49 s end to end; 9 file requests, 12.3 s |
| fixture, independent files, cold model cache | 41 s end to end; 1 file request, about 6 s |
| fixture, same evidence, 8 rows / 16 KiB file budget | 4 live file requests in about 3 s; directory requests cached |
| fixture, complete saved reading after the declaration fix | 53 ms inside reader; 15 cached requests, no live calls |
| etcd, ordinary cold model cache run | 8m05s; 27/27 targets, 582 files, one physical HTML; one invalid JSON window recorded |
| etcd, saved input with final file prompt | 32.2 s; 20 live file windows, zero rejected; directory requests cached |
| etcd, repeat of that saved reading | under 1 s; all 29 windows cached |

The etcd end-to-end timing predates the final file-prompt wording and the
Python/TypeScript declaration fix. The final prompt was exercised on its
saved evidence separately, not claimed as another complete cold run. The
fixture's final ordinary run analyzed both targets and preserved facts.json
byte for byte. All product tests and vet passed; the updated places, reader,
table and CLI tests also passed after the last code changes.

The command and file-round descriptions below predate this boundary where
they conflict with this section.

### Question candidates and generation evidence (2026-09-05)

The next cube is the atlas question table, using the same executor, request
cache and frozen reading input. `read --question TEXT` runs it followed by
route selection; the ordinary command appends both after the atlas tables. Every graph declaration
and boundary enters exactly one chunk, including generated code, with up to
24 anchors per chunk. The model chooses relevance, a closed local anchor and
a short reason. Paths, positions and target identity are restored locally.
`question-route.json` records coverage and candidate stops. Connections keep
compiler witnesses, producer declarations and corpus membership distinct;
they do not claim that chosen representative declarations call each other.
Failed chunks remain unresolved.
There is no implementation/body inspection yet. The ordinary report now renders
the selected guide; the development `read` command still only saves artifacts.

Route selection now compares up to 24 distinct source locations in each pool,
then compares their selections until one pool remains. Pools split further by
the actual prepared input byte budget. A partial pool must reduce its members;
the final pool aims for six anchors in reading order, preserving extra valid
anchors instead of refusing the guide. The model returns
an ordered closed ref sequence, a short summary and one open question. Original
evidence, not summaries of earlier pools, is carried into every round. The full
candidate reservoir and all existing connections remain in question-route.json
v3, alongside the guide and per-round coverage. Unknown refs do not gain
authority, failed pools are recorded and no deterministic route is invented.
`--through question` isolates retrieval; `--through route` changes selection
while the shared exact-request cache reuses unchanged retrieval.

The live sqlc example reused retrieval in 5 ms and selected its six stops in
2.142 s: query.sql, sqlc.yaml, generated ListAuthors, handwritten ListAuthorNames,
schema.sql and generated Author. It left caller/test dependence as an open
question. The ordinary python-tutorial-game run completed 2/2 targets in 7 s;
only the route table called the provider. It chose the frontend HTTP sender,
backend receiving route, exec site, playground and level page. A deterministic
83-candidate test verifies that the entire first-round reservoir reaches the
model and that the tail survives successive selections. This is not yet a fresh
etcd routing measurement; its older saved input needs a new native preparation.

### Internal entity knowledge (owner clarification, 2026-09-05)

The owner wants descriptions and discoveries to accumulate on internal
entities and serve both later model cubes and people. A function's general
behavior can be reused across callers; the purpose and arguments of one call
belong to that call's context. Do not re-read a shared lower-level subtree just
because another higher-level caller reaches it. Keep source facts and model
interpretations distinct and remember which evidence and earlier interpretations
each conclusion depends on, so the affected knowledge can be invalidated when
its basis changes. This should remain a small record boundary, not a large
taxonomy or another plugin system.

SubjectID on a question stop is distinct from its context PlaceID, with original
selected evidence retained. Native declarations use their ProgramIndex object
IDs; boundaries and observed entities use existing graph IDs. Context fields
retain their actual context path, so generated-file attributes cannot silently
describe a configuration anchor. Internal IDs are restored locally, never sent
to the model.

Independent directory, file, symbol, operation and boundary interpretations persist
as knowledge.json v2. Each record binds the internal subject, current owners, context, exact
single-row evidence, cells and dependencies on earlier model interpretations.
The model still receives batches; single-row preparation only computes identity.
Only these independent tables opt into reuse and their prompts explicitly require
row independence. Comparative stages (groups, routes, portfolio) keep complete
request identity. Existing provider exchanges retain real transport accounting;
reused entities are counted as reused_rows, not fictional provider cache hits.

The shared executor stores entity-to-response-row indexes in its existing
.llm-cache directory. Each memo contains only the request key and original row
key; the answer comes from the current shared response and is revalidated by
the owning table. Response tables are loaded once per request per reading. Answer basis identity includes provider configuration, prompt/table
contract and the exact single-row model request. Repository labels, native
subject IDs, ownership and parent knowledge IDs do not affect answer reuse.
They instead contribute to the current knowledge ID together with the answer
basis and accepted cells. Every reuse rebuilds that binding: dependencies point
to this run's parent knowledge, not obsolete records. A parent's changed source
evidence can therefore produce a new provenance chain without model calls for
children receiving identical text. If its supplied model line changes, the
child's request and answer basis change too. Missing or changed inputs are
batched together. Missing rows with the same exact answer basis now share one
provider row within the reading. Every original subject still receives its own
current knowledge/owner/location binding and the same validated response-row
reference. This removes cold-run disagreements between identical inputs that
could otherwise change a warm run's recalled hint and invalidate question
retrieval. The regression covers distinct parent interpretations, rejected
aliases, changed batching, and replay updating all matching subjects. Exact
accepted request caching still applies to those batches.
No-cache bypasses reusable answer reads and pointer/index writes. Diagnostic
payloads still go to the shared cache directory. Cache clear removes payloads,
answer pointers and memo indexes; run snapshots remain, but raw-payload links
then stop resolving. Recalled rows are revalidated and are not rewritten.
An optional entity-memo write failure reports its stage and exact path without
discarding accepted cells or their current provenance. Each identical basis
gets one write attempt per reading; mandatory knowledge and window artifacts
still fail their owning persistence operation.

`repomap replay --file REQUEST.json [--debug-dir DIR]` sends the exact prepared
provider payload through the existing configured client, always live. It keeps
all saved request options, including unknown provider fields; the configured
client supplies endpoint, authentication, timeout and retry policy. Only a
successful provider response containing valid JSON replaces the answer pointer.
The owning stage checks its domain schema on later reuse. Replaying a batch
updates the rows recalled by entity indexes even if a later reading uses a
different batch size. Old run results and HTML remain snapshots.

The cache has one current format: v2 accepted records reference request and
response payloads stored by content hash. Semantic journals v3 retain per-run
accounting and relative links into this same store. Atlas tables likewise write
prompt/request/response ref JSON, plus run-local normalized results. Replay
prints the shared request and response paths, duration, attempts and usage.
Cache reads distinguish proven corruption from operational failures. Invalid
JSON, identity/accounting, unsafe entries and missing referenced payloads may
evict an accepted pointer; an I/O failure, an observed inode replacement during
opening, or a stricter current response-byte limit is a diagnosed miss and
does not remove it. Domain validation still rejects and evicts incompatible
answers. A subsequent provider failure therefore leaves a previously usable
answer available for another run. This classification does not make pathname
eviction atomic against a later concurrent writer.

The shared JSON normalizer separates one complete leading
`<think>...</think>` block before inspecting the final answer. Code fences or
draft JSON inside that block cannot become the answer. An incomplete block,
nested/repeated leading blocks, multiple final values or malformed final JSON
remain rejected. The live outcome, observer and reused response retain all
original bytes; normalization changes only the input to semantic validation.
This compatibility handling does not itself turn thinking off at the provider.
The owner clarified on 2026-09-08 that the earlier default-off preference must
still honor a stage's explicit reasoning opt-in. Compatible endpoints now send
`chat_template_kwargs: {enable_thinking: true}` for shared question retrieval
and final answers, and `false` for ordinary fast stages and display translation.
`REPOMAP_LLM_CHAT_TEMPLATE_KWARGS` or its legacy
`DEEPSEEK_CHAT_TEMPLATE_KWARGS` alias explicitly replaces that object in full;
a forced `false` therefore overrides stage reasoning, and an empty object omits
the extension. No silent retry removes the control. The actual endpoint host
selects native DeepSeek handling: `api.deepseek.com` encodes the same stage
preference with its existing `thinking` control and no default kwargs.
The configured variable prefix does not classify the endpoint. `DEEPSEEK_*`
and `REPOMAP_LLM_*` configure the same client, with the generic namespace
authoritative whenever any of its settings is set. These request options take
part in exact cache identity, and replay does not reapply environment options.
The provider regressions cover both environment families, endpoint-host routing,
explicit on/off/omission overrides and unchanged client configuration. An HTTP
executor regression alternates fast/reasoning requests, parses a Qwen-style
leading think block containing a non-JSON code fence, retains the original
response bytes and reuses each mode only from its own exact-response cache.
Parser rules remain unchanged: a complete `</think>` terminator is required;
a literal `<think/>`, truncation or ambiguous final JSON is refused.
Full `make test`, `make vet` and `make build` pass. Ordinary publication on the
Python/TypeScript fixture at `20260908-042325` retains both complete targets,
unchanged canonical content apart from timing and all 40 cached exchanges,
with zero provider attempts. This checks the unchanged official-DeepSeek path;
the remote owner's custom endpoint was not available for a live validation.

Question-only readings use the same independent row builders in recall-only mode:
they restore current descriptions but make no description requests. Source rows
receive explicitly labelled prior model hypotheses; stops retain used knowledge
IDs. This is reuse of extracted-evidence descriptions, not implementation
analysis. Functions still cover selected declarations using names, signatures
and docs. An uninspected body change does not invalidate those descriptions.
Changed target inventories may reuse byte-identical row inputs while producing
new entity bindings. This is exact input reuse, not fuzzy identity matching;
there are no old-format readers or cache migration paths.

Deterministic tests verify rebatching without provider calls, reanalysis of a
changed file while its unchanged function input stays cached, updated parent
links, invalidation when the parent's supplied wording changes, preservation of
unrelated knowledge, ownership/native-subject rebinding without provider calls,
question reuse, model/repository bindings and no-cache behavior.

Shared-payload/replay acceptance on 2026-09-05 used the same configured
DeepSeek client and current binary. The first ordinary python-tutorial-game
run took 41.69 s; its full repeat took 4.528 s with zero live calls and all 159
entity rows reused. Both targets retained their backing artifacts and one HTML;
browser inspection confirmed rendering and an openable source card. Shared
journal references and payload hashes were checked across both runs. Cache
clear was exercised on a copy: the payloads and indexes disappeared while
report.html and knowledge.json stayed byte-identical; the working cache stayed.

A real replay of one 50-symbol batch took 6.765 s and preserved the saved
request bytes. The subsequent reading through symbols, with a one-row budget,
took 72 ms wall / 43 ms inside the reader and made no provider calls. All 50
rows referenced the new response; 17 descriptions changed. This checks that
replay warms the cache used by entity references rather than an unused copy.

On the saved etcd input at revision 58f45a9ff1c0, first filling the new cache
format took 251.086 s. One file window was refused for an extra closing brace
after JSON, leaving 40 file descriptions missing. The next pass took 31.475 s,
reused 4039 rows and made 11 live calls to fill that gap and its changed
context. With all responses accepted, the third pass took 2.818 s: 4374 entity
rows reused, 37 complete windows cached, zero live calls and identical
knowledge IDs/cells. These are saved-input reading measurements, excluding
compiler extraction and report rendering. The old cache files were preserved;
the new cache format/key contract does not read old entries or migrate them.

Full ordinary etcd runs measured separately: 189.117 s with the atlas cache
warm and four live discovery/documentation/orientation calls (47 s of provider
time), then 148.383 s with every stage cached and zero live calls. The latter
completed 27/27 targets and published the single owner HTML. All 4374 knowledge
IDs and cells stayed identical across the two full runs. Coarse stage markers
place 71 s before the places graph, another 15 s through atlas projection and
cache reading, and 62 s after that through report publication. These intervals
are wall-clock regions, not a CPU profile. The next performance work belongs to
local preparation and publication; a 2.8 s saved read does not measure those.
The intermediate full run is removed after the final run's artifacts are
checked, retaining the newest complete report and the shared cache.

Live validation on the clean public python-tutorial-game revision 78714d34ee:
the first normal run with the new description prompts completed 2/2 targets
in 45 s (earlier discovery/documentation requests were cached). It produced
159 descriptions: 10 directories, 29 files, 109 symbols and 11 boundaries,
with 149 model dependency links. Every subject and dependency resolved; all
109 native symbol IDs matched their exact file and line. A second normal run
completed in 4 s, reused all 159 records with unchanged IDs, and made no live
provider calls. Both targets retained their required artifacts, facts.json was
unchanged, and the owner had the only physical HTML for that run portfolio.

Changing the symbol batch from 50 rows to 7 and the input budget to 8 KiB in
an isolated reading reused 148 descriptions through symbols in 34 ms with zero
provider windows. A new question about frontend animation recalled all 159
descriptions and ran only retrieval and selection: 7.685 s, no rejected windows.
Its route reached runLevel, PlayGround, SimulationField, drawRobotsBetweenSteps
and ISimulationStep. It also speculated about an unused sleep helper; source
inspection shows animation uses requestAnimationFrame. This remains a quality
gap: remembered model text is a hint, not verification of implementation.

Browser inspection of the ordinary repeat report confirmed the repository
purpose, run commands, HTTP crossings/port, exec site and missing/dead files.
The main flow currently ends at backend exec rather than frontend animation,
and a group titled Core application logic still appears in Inbound. These are
outstanding semantic/flow issues, not fixed by knowledge reuse. The new animation
question supplies a useful route separately; the HTML layout was not changed.
Full product tests and vet passed; focused tests/vet passed after the final
cache-write optimization. The real cache-clear command was exercised on a copy
of this run's cache (159 memo and 33 request entries): both were removed while
copied knowledge.json and report.html remained byte-for-byte intact. The working
experiment cache is retained for further development.

### Earlier question and extractor measurements

The saved etcd state-storage question processed 582 files / 713 chunks in
33 live windows, 36.8 s, with no rejected windows. It selected 19 direct and
64 context stops, with 112 witnessed file connections. BackendPath, WAL.Save
and batchTx.Commit were checked in source. This is candidate retrieval
evidence, not a completed user-facing route. An earlier run was rejected by
the API because the prompt omitted the word JSON; the prompt and semantic
journal registration were repaired before this measurement.

The owner asked for company-specific code to supply evidence, with sqlc as
the first example and the same rules for built-in and external producers.
The public interface has just nodes (local id, path or name, optional line)
and links (from, to, label, optional more precise source). Producers do not
assign architecture roles or copy internal IDs. A `.repomap.json` command
entry receives JSON on stdin and writes JSON on stdout. Repomap records the
exchange, restores identities and target membership, and lists corpus members
under a referenced path. See docs/EXTRACTORS.md and the Python example.

The built-in sqlc extractor crosses that same response decoder and fact
normalizer. It reads config v2 and emits config-block/path nodes and labeled
schema/query/output links with exact config lines. Missing output paths stay
visible; nonliteral and out-of-repository references stay named but unresolved.
Membership does not prove generation or migration execution. The common fact
format is v2, without old-format readers. Its `entity` and `relation` rows now
enter the same places graph. Entities retain the producer, declared path and
complete corpus membership. Observation edges retain the exact declaration;
inventory edges join entities to indexed code files without asserting that
those files were generated. Question rows include these declarations and
all member anchors in complete bounded chunks. Related code rows receive
the original producer context, not another model's summary. A selected source
anchor can be in a configuration file different from the row's declared path.
Missing outputs never become invented file anchors. Graph and saved reading
input are v2; question route is now v3 and plugin protocol remains v1. Ordinary HTML and
architecture classification tables are unchanged; there is no separate
HTTP/OpenAPI interpretation path or generated change recipe.

The live sqlc exercise now uses actual sqlc-generated Go files, ignored by
Git, plus a handwritten caller and the external Python producer. The first
ordinary run took 17 s; later full runs reused unchanged stages (10 s and
4 s). It read 4 code files plus 7 entities in 11 chunks. Compiler witnesses
connect ListAuthorNames to New and Queries.ListAuthors at main.go:21; producer
declarations connect query/schema/output paths and inventory connects the
output directory to indexed files. These are distinct connections.

Online inspection found two question defects: advice to edit generated code,
and a reason about query.sql paired with an anchor opening sqlc.yaml. The
question prompt now asks only what to inspect, and each closed anchor's actual
path and line are explicit in its evidence. The isolated final replay took
3.276 s, one live window, all 11 chunks accepted. Its query/config/caller
pointers matched their reasons on inspection. This is one observed fixture,
not a correctness guarantee or complete change plan. Duplicate stops from
independent producers remain in the reservoir; the new guide compares distinct
source locations and retains each location's original producer observations.

The final ordinary public python-tutorial-game run completed 2/2 targets in
12 s, with one physical HTML and all 30 question chunks accepted. The report
was opened again: run commands, three HTTP crossings, exec at field.py:98 and
the main flow ending at SimulationField.animate:138 remained available. The
question selected the HTTP sender, receiving route, request model and execution
site. Full tests passed, then affected atlas tests, final vet and build passed.
One build ran out of disk space; only three reproducible, untracked test
executables in .codex-tmp were removed. Source, reports and shared Go cache
were preserved.

The question-enabled ordinary fixture run completed 2/2 targets in 51 s,
with one physical HTML. All 30 question chunks were processed; it selected
the frontend HTTP call, backend endpoint and exec call with exact anchors.
The report was opened in the browser and its run instructions, protocol
crossings and execution pointer checked. Its final animation step now points
to frontend SimulationField.animate; this is one observed model result,
not proof the earlier semantic anchoring weakness is generally solved.

The simplified extension contract was tested with the built-in sqlc response
and an external process returning the same nodes and links. The ordinary
command also ran the 16-line Python example alongside sqlc on an isolated
project: 1/1 target in about 1 s, 7 entities, 5 relations, and both exact
exchanges saved. The output folder was absent. The complete test suite passed
with package concurrency 1 (parallel build/test/vet exhausted the remaining
disk space); vet and the final build then passed separately, without clearing
the shared Go cache. Tests for the final path lookup also passed.

## Atlas (the model path, from 2026-09-04)

The sections "Sparse overlapping categorization", "GroupsIndex" as built by
grouping, and "Cross-target group matching" below describe history: those
three stages and their packages were deleted on 2026-09-04. The GroupsIndex
remains as the projection of the atlas the page reads
(`groupindex.ProjectAtlas`). The atlas is one reading layer,
`internal/atlas`, along the plan in ATLAS_PLAN.md: the repository as places
(directories, files, boundaries; symbols next) asked about in keyed tables,
one line per place, context one step up, boxes by directory, arrows and
joints derived by code. `places` builds `places.json` from the program
indexes, claims, facts and corpus; `table` is the keyed request shape with
owning validation; `lines` holds the tables and their prompts; `reading`
walks them and prints every row to `tables.md` with the request bytes under
`tables/`. `--atlas` runs it after facts and claims and stops before the
report; `--no-model` walks it dry. Under `--atlas` the child run persists
the base index, the reduced documentation and an empty groups index and asks
no categorization or grouping.

The owner's 2026-09-09 validation correction preserves independent atlas rows
when a neighbour has an invalid closed choice, a missing cell or a duplicate
key. Each rejected row keeps its deterministic line and a recorded reason;
accepted rows retain their exact original response and independent row memos.
Unknown row keys are recorded without acquiring an identity, and unused extra
fields are ignored. Unparseable JSON, an entirely refused answer and incomplete
coupled assignments still reject their request. Cached response and replay
validation use the same row rules; optional terms follow only accepted rows.

The same audit extended independent acceptance to final answers, retrieval
questions, Learn intent reviews and menu decisions, arrow explanations, target
lines, zone choices and descriptions, joint confirmations and peer choices.
Independent validation and entity memoization are separate table flags: only
the existing description and operation families use entity memos. A refused
zone choice does not acquire a guessed assignment from its title or parent.
Coupled zone-name proposals still need their complete closed catalogue.
Learn consolidation preserves accepted original questions after a refused
comparison, retains valid comparisons and marks the plan partial. It does not
rewrite the scope of earlier menu decisions. Identical repeated native target
placements are idempotent; conflicting placements still refuse that target.
README/documentation and orientation corrections are recorded in their owning
sections below. Optional terms follow accepted output scopes in every changed
family, including live, cached and replayed responses.

Ordinary online acceptance used the real `python-tutorial-game` checkout on
2026-09-09. Owner `20260909-154859-python-tutorial-game-cf229063d519` published
both targets despite an HTTP 200 answer response with no content: its 12 answers
remained unavailable and that response was not cached. The next owner
`20260909-155409-python-tutorial-game-2a335e2d360f` reused the accepted analysis,
obtained eight answered and four partial answers, and translated the resulting
374 display texts in eight requests. Only that previously failed answer and its
dependent glossary/translation work made new calls. Owner
`20260909-155840-python-tutorial-game-5c85f175c915` then completed in 6.18 seconds
with zero live requests and byte-identical saved translations. Target-local
indexes, reduced documentation, dependency catalogues and GroupsIndexes, the
common graph/atlas/portfolios/manifest and sole HTML all passed artifact checks.
Cache clear removed an isolated copy of 205 cache records while preserving the
original acceptance cache. Full `make test`, `make vet`, build and focused race
checks passed. Independent regressions cover partially rejected responses,
multiple simultaneous resource refusals, NoCache, replay and refused merge
originals. These checks establish failure isolation and reuse, not universal
model answer quality. Receipts are `work/validation-final-*-receipt.json` and
`work/validation-final-cache-clear.json` in the external project work directory.

Steps 1 and 2 are in. The tables, in order: directories by depth; files by
call-graph round (a file may move to a sibling box or start one, a box of
one file is cancelled); boundaries from facts and from SDK calls with
literal arguments (kind from the code where it knows, else the model);
zones per target as two cubes, one row asking for exactly `want` part names
(the square root of the top boxes, four to eight) and then every top box
choosing from that closed list, boxes beneath inheriting; arrows folded by
code from file-to-file edges (six outgoing per box drawn), the model
writing only the hover sentence; the portfolio table (one line and a role
per target when there are several); joints between compatible targets by
matched method and path or equal literal, confirmed yes or no, and a blind
peer choice for outgoing calls nothing matched. Side and trace are code.
Above 2,000 files the directory and file rows carry an `open` cell and what
the model closes keeps its fallback line. Step 3 adds the symbol table: the
code ranks declarations per file (exported and documented first, then by
callers). Every callable and the first ten ranked declarations are description
candidates; additional type places remain available to questions. The model
first selects each candidate's key role, activation and outgoing calls without
writing a caption. The code keeps at most five keys per file by rank and a box
shows three. Only those displayed selected keys receive symbol/type prose.
Without the model the keys keep their original native ranking and source text.

Directory/file `open=no` controls presentation exploration, never symbol role
selection. Accepted selections have their own knowledge and exact-input memo;
caption refusal does not erase them. All original symbol/type places remain
available to questions, including types beyond the initial candidate ranking.
Question-only reading recalls the same selections and captions without new
requests. See "Required analysis results before optimization" above for the
current Airflow scope measurement and its limits.

Directories v3 and files v4 use the complete request-local `fill` catalogue
as their response contract. Both owner prompts show the base and optional-open
modes; an advertised `open` requires the existing `yes`/`no` choice. The earlier
system instructions incorrectly required only two base cells and forbade other
fields even when the request required `open`. The ordinary Airflow run exposed
this contradiction in all 18 directory windows: 17 refused missing `open`, one
returned invalid JSON, and all 2,706 rows remained without model answers.
The same contradiction affected the file prompt. The run was canceled with its
original input, cache and journals preserved, before spending further calls on
that invalid contract; no HTML was published. Both prompt examples now pass the
ordinary table preparation and validation in each mode. Missing `open` remains
a refused row while a complete neighbouring row survives. Full product tests,
vet and build pass. The full saved Airflow input then ran through directories
and files on `f0c5ee5c`, without rebuilding native facts or the map. All 1,472
asked directory rows in 13 windows were accepted; 1,234 descendants under closed
directories remained unasked. Files accepted 3,122 of 3,160 asked rows in 79 of
80 windows, with no missing-open refusals. One 38-row response completed its
table and then repeated optional terminology until the 128,000-token provider
ceiling: its incomplete envelope was refused, while all accepted neighbours
and their exact-response caches survived. That request took 460.401 seconds;
the saved reading took 861.406 seconds and exited zero with one rejected
window, not with complete model coverage. The copied input is byte-identical
to the original. Payload hashes, normalized cells, journal outcomes and cache
bindings pass the artifact audit in
`airflow-validation-20260909/followup-2142/reading-open-final-audit.json` in the
external project work directory. This focused check creates no HTML and does
not replace full Airflow publication acceptance.

Measured on 2026-09-04, repomap on itself, seven targets: 30 windows plus
33 symbol windows for 1,613 candidates (55 s), of which cmd/repomap's 47
directories, 199 files, 10 boundaries, 7 zones and 106 arrows; the three fixture joints on `/api/levels` confirmed and the
cross-fixture false candidate refused; 24 s wall with everything cached,
16 s on a repeat. One docstring's first sentence changed in one file cost
11 live windows and 55 s: the file's line is carried into the rows of the
files it calls, round after round, and a reworded line cold-starts them.
A change past the first sentence costs nothing, since only the first
sentence is sent. Step 4 is in: the atlas is the only path, `--atlas` is gone, the child run
writes an empty groups index that the projection replaces once every target
is read, the orientation is asked over the projected groups, and the page,
served or static, is built from them unchanged. The old stages'
packages (`programcategorization`, `programgrouping`, `groupmatching`) and
their run glue are deleted; `groupindex.Build` and the proposal types are
dead code awaiting removal. An in-process test drives the whole path over a
two-file Go repository with `--no-model` in about a second. repomap on
itself, seven targets, cold cache: 3m02s wall against 29m34s before.
Step 5, measured the same day on the owner's Mac (4 provider threads):

| repository | targets | files | windows | wall, cold cache |
|---|---|---|---|---|
| repomap on itself | 7 | 245 | 63 | 2m43s |
| etcd | 27 | 582 | 157 | 9m25s |
| kubernetes, cmd/kube-apiserver | 1 | 820 | 154 | about 10 min: facts 3 min, claims 1 min, tables 4m30s, orientation 6 s |

kubernetes' Go index takes seconds with a warm build cache; cold it is
the build of kube-apiserver. Three rules came out of the big runs: SDK
boundaries are a closed list of client libraries (the open "any package
with a literal" rule gave etcd 1,055 zap log calls and kubernetes 844
field paths); a call or import into a box of another target is a joint
the code derives (a workspace package of the same target is no boundary
at all); the orientation request is bounded by the model's context
(2 MB), not by the 32 MB record limit that let kubernetes' request draw
an HTTP 400. Not measured: chi (a minute by the numbers), etcd with a
warm cache. The disk was full during these runs (see the memory note);
an "unavailable for SSA" error with no diagnostic was the disk.

## Product surface

`repomap` has one supported product path:

```text
repository
  -> safe repository corpus and guidance documents
  -> repository-wide target selection
  -> one typed language-adapter path per selected target
  -> one sealed ProgramIndex per target
  -> one reduced documentation handoff
  -> sparse overlapping categorization of the same ProgramIndex
  -> one GroupsIndex per target
  -> cross-target group matching
  -> deterministic facts and claims
  -> model orientation over both
  -> report.json + one static report.html
```

The supported commands are deliberately small:

- `repomap [repository] [flags]` runs the ordinary online analysis and
  publishes the report;
- `repomap replay --file REQUEST.json [--debug-dir DIR]` repeats a prepared
  provider request and refreshes its shared answer.
- `repomap cache clear [--debug-dir DIR]` clears persistent model-response
  caches.

Report serving is part of the ordinary run and is controlled by `--no-serve`
and `--port`; `--no-open` controls only automatic opening. The `read` development command above reuses the atlas reader. There is no
second analysis implementation, developer server, or sidecar entrypoint.
`read --output` prepares its shared cache root even when the output directory
is elsewhere. This also applies to `--no-cache`: accepted-response reuse is
disabled, but the required exact request/response journal payloads still need
their shared directory on a first run.

The graph has one semantic data path. Language adapters provide deterministic
program facts. Repository documentation is reduced once and supplies context
to categorization. Categorization enriches the same ProgramIndex type rather
than creating another element inventory. Grouping produces the target-local
group graph. Matching adds validated connections across complete target-local
graphs. A deterministic stage then derives the anchored fact layer and the
quoted claim layer, and one model stage writes the orientation over both. Go
renders the finished page; the browser receives no analysis payload and
discovers nothing of its own.

Every target reads in one order:

```text
Inbound -> Entrypoints -> Core -> External calls and dependencies
```

`Triggers` is one presentation lane containing both `inbound` and
`background_activity`. The original category remains attached to every
subject, so an HTTP handler, CLI command, scheduler, consumer, worker, startup
hook, watcher, polling loop, or controller is not flattened into an
indistinguishable generic entry.

## Repository corpus and file identity

Every later stage derives authority from one immutable repository corpus.
Corpus collection now inventories current source files, recognized project
manifests and documentation on disk, including new and Git-ignored generated
sources. File refs and executable permissions come from that working directory,
independently of Git index membership. Later stages select from the same corpus.
The root `.repomapignore` lists literal repository-relative files or directories
to exclude; it is separate from `.gitignore`. Installed dependency trees,
virtual environments and known caches are excluded. A directory named `build`,
`dist` or `coverage` alone is not grounds to omit its sources. The current run's
output directory is excluded when it is inside the analyzed directory.

Remaining integration work: ordinary repository-state capture still requires
a Git HEAD, and claims extraction and report validation still require a
revision. A filesystem corpus works without Git; the complete ordinary command
does not yet. Those stages need optional Git metadata and content identities
for untracked inputs before Git-free execution is complete. Standalone GitHub
and GitLab publication checks every readable corpus path against the captured
revision once, using a NUL-delimited Git tree inventory and the analysis root's
exact repository-relative prefix. Absent paths, including submodule descendants,
and paths in the captured tracked changes retain their report content but have
no remote link. The owner requested plain path text with a `No source` hover
explanation, without interrupting publication or requiring extra actions.
Existing code cubes and their selection use source identity independently of
link availability. Unchanged sources keep their original permalinks. Child
targets reuse the outer check; the standalone manifest stores the unavailable
path list, so saved HTML rendering never reopens the repository or a provider.
This is publication metadata, not model input or an analysis exclusion. Local
serving still opens the analyzed working tree, including these paths. Corpus
tests cover identical inventory before Git init, before the
first commit and after a commit; they do not stand in for that integration.

Before any short file identity exists, collection excludes `.npmrc`, every
`.env*`, installed dependency subtrees `node_modules`, and every
`*.tsbuildinfo`. Excluded files never enter freshness
state, model input, debug output, or publication.

The selected repository is trusted by default. Heuristic credential scanning
is enabled only by `--scan-secrets`. API keys and Authorization headers are
never persisted. Provider requests never contain absolute host paths,
credentials, full repository source files, raw internal graph IDs, or
unadvertised repository paths.

Repository scale is retained completely. Former local file-count, text-byte,
graph-depth, collection-size, and artifact-size thresholds are warning-only.
They cannot sample, truncate, omit, reject, or partially publish otherwise
valid repository authority. Only representation overflow, security or
canonical identity/path/format validation, an actual provider envelope, or an
explicit user narrowing option may remain a correctness boundary.

Repository changes during a run do not fail publication. There is no
freshness gate or strict-snapshot mode.

## Repository guidance and documentation reduction

The initial guidance classifier may send the complete names-only safe-corpus
dictionary as a lossless prefix-compressed path tree with compact `f*` leaves,
the complete closed set of prose-file refs derived from that dictionary, and
complete textual README and AGENTS.md documents. It sends no other source-file
contents.

The classifier restores accepted rows only to advertised file refs. Its
repository-guidance result supplies exact documentation inputs and optional
target evidence. It does not create program objects, program edges, or target
identity.

`documentation_reduce` consumes the complete validated guidance snapshot and
produces one sealed `reduced-documentation.json` handoff:

- an optional repository overview;
- source-bound claims and concepts;
- the exact guidance digest;
- the exact reduction digest.

Sparse reduced documentation is legitimate, including the canonical empty
result when there is no guidance authority. Every retained source must restore
to the exact guidance snapshot. The reduction is repository context for every
selected target's categorization request; it is not copied into adapter facts
or treated as source-code authority.

The 2026-09-09 validation correction accepts guidance files, classifications
and hypotheses independently. Invalid members retain a recorded reason without
erasing valid neighbours or the native target inventory. Documentation likewise
keeps valid source claims and concepts beside malformed ones. Independent source
and merge requests use the shared worker pool without cancelling accepted
siblings; real resource refusals split every affected request in the round.
Successful input snapshots stay unchanged for exact warm-cache reuse, including
after children are added. A failed merge preserves its already accepted input
claims, and an incomplete merge supplies no invented whole-repository overview.
Optional terms are collected only from fully accepted file/source scopes.

Source and merge packing find the largest complete request prefix by probing
exponentially growing windows, then searching within the last fit/refusal
bracket. This avoids re-encoding every growing prefix, without repeatedly
encoding the entire remaining reservoir when only a small window fits. Exact
provider preparation still decides fit; UTF-8 splitting, worst-case ordinal
reservations, materialization, request bytes and adaptive execution are unchanged.
An indivisible merge candidate is rejected before materialization even when it
follows a valid window. Other preparation errors retain their original cause.
The 128-row regressions preserve all materialized bytes while reducing prepared
source bytes from 6,524,096 to 563,303 and merge bytes from 10,296,407 to 326,170.
These are local preparation counts, not provider-token or end-to-end speedup
claims; tiny windows can use a few more probes while remaining linear in the
reservoir size.

## Target discovery and selection

Every active Go and Python target scout and the JavaScript/TypeScript package
scout runs over the same corpus. Their exact native candidates and resolvable
guidance candidates are merged into one repository-wide `TargetPortfolio`
request. Discovering one supported language never suppresses another.

Each exact native target contributes one canonical required file
representative. A shared representative is deduplicated; alternatives for the
same target are not all made mandatory. Required now means that every native
target must receive a decision, not that every candidate must survive as an
independent product. The selection request assigns a closed target ref to each
native target, separately from its file address; one file can represent several
decisions. File refs still restore exact candidates and positively supported
guidance-only starts. The same portfolio chooses a retained repository default.

Stage B, authorized on 2026-09-09, sends adapter observations before selection:
Python launch bases and declared distribution membership; Go own-main counts,
own-main consumers and imports from another repository module; and exact fenced
README command/import quotations with their source path, line and heading.
These quotations are author claims, not runtime observations. Repeated evidence
is encoded once behind local refs; every request partition rebuilds its complete
evidence catalogue. Native identities and compiler edges stay local.

Python catalog/target v5 records static package lists in setup.py, pyproject.toml
and setup.cfg and package discovery where/include/exclude rules. Full package
names are matched, and namespace settings are respected. A project-name match
requires a corresponding real package. Dynamic setup calls are not evaluated
and supply no package declaration. This authority is separate from the complete
importable module inventory and does not itself classify a guard as an example.
Only a guard inside a declared library distribution is advertised as eligible
for a seed owner. Explicit package CLIs, console scripts and shebang launches
are not folded by this rule. JS/TS source-owning package targets retain their
existing independent compiler boundaries, including packages without start/bin.
Exact Python launch files also supply their corpus executable bit and direct
module-level relative imports, including wildcard imports, with original source
anchors. These observations do not change the native candidate's identity or
remove an author-written shebang candidate; native identity remains v3.

Each native target chooses `standalone`, `seed_of:<ref>`, `shared_code`, `tool`
or `example`. A seed owner must be an advertised mandatory target with its own
explicit `standalone` decision. Missing, duplicate, invalid or chained decisions
retain a standalone candidate and a `decision not received` reason. Unknown refs
have no authority. `target-placements.json` records every native decision with
its original columns, source quotations and any rejected answer. It is a journal
of this selection, not another analysis graph. A complete response-envelope or
transport failure keeps the existing portfolio error behavior.

Tools, examples and shared code all execute their complete ordinary target path.
Shared code keeps its library API and a counted catalogue, with exact target
links from observed consumers. Only positively accepted seeds lose their
separate run: their original launch identities and source locations enter the
owner's ProgramIndex without narrowing its objects or library API. Selected
roles pass through atlas v4 and GroupsIndex v8 into report v87. The later atlas
target table describes an already selected role rather than making a second
role decision. Saved reading input v10 retains that role and shared-code links;
the places graph remains v9. The code validates that every shared-code link
resolves to an actual shared-code target in the complete portfolio.

Ordinary online checks on 2026-09-09 completed all five repositories and both
cumulative counterexamples. The final four review reports are owner runs
`20260909-140836-go-http-server-7f09fe1266a4`,
`20260909-140836-chi-6a3ecb6770b7`,
`20260909-140839-python-tutorial-game-df98276c38d2`, and
`20260909-140841-jieba-c81bacdf4ba7`; pykrx is
`20260909-140441-pykrx-188c04b5299c`. Their complete target artifacts and the
single owner publication were checked. Products mean application and library
roles, with other roles shown in their counted catalogues:

| Repository | Expected products | Observed products | Other full analyses |
| --- | ---: | ---: | --- |
| go-server-template | 1 | 1 | 1 tool, 1 shared-code library |
| chi | 1 | 1 | 2 examples, 1 shared-code library |
| python-tutorial-game | 2 | 2 | none |
| jieba | under review | 3, including a refused decision's fallback | 18 examples |
| pykrx | 1 | 1 | 4 tools; 17 guards enter the library |

The explanation of jieba's three products was corrected on 2026-09-09.
The owner still accepts author-written shebangs as native launch candidates. The library and documented CLI had accepted roles;
`jieba/analyse/textrank.py` remained standalone after an invalid `shared_code`
answer for an executable row. The first model draft had called it a tool.
The later standalone placement is the recorded fallback, not a model decision
that textrank is a third product. Its author-written shebang preserves the native candidate; it does not
replace the missing model role decision. The file lacks execute permission
and has module-level relative imports that require package context. A fresh role
decision must read those neutral facts and the original launch mode; they
do not authorize a local exclusion of the candidate. The prompt must
make the existing library-only `shared_code` contract explicit. No path-specific
role, target removal or HTML suppression is used to force a count. Jieba's
product count remains under review; the other four counts and the two required
service-separation checks retain their recorded acceptance. The fresh ordinary
online selection `20260909-185307-jieba-43f79a64a266` accepts textrank as `tool`,
with no rejected decision or fallback. It selects two products (library and
documented CLI) and 19 tools while preserving all 21 analyses. The ordinary
run completed in 3m31s with one common Russian HTML and eight translation
requests. This records the accepted model role decision and completed run;
the useful role of each test utility remains a quality-review question.

The Go counterexample keeps both documented services separate and gives each
an exact link to the common library; its existing cmd/app exercise also remains
standalone (3 products, 5 full analyses). Python keeps acme.api and acme.worker
separate, folds only acme.demo into the library with its original line-5 anchor,
and leaves acme.excluded.check outside that ownership. Its library and existing
fixture console remain standalone too (4 products, 5 full analyses). The two
required service-separation checks pass; fixture/tool role quality remains a
review item. The cumulative JS/TS check retains all three source-owning packages.

On pykrx, report.json fell from 120,741,024 to 35,150,500 bytes (70.89%) while the
library retained the same 2,833 native objects with identical names, kinds and
source locations. Every one of the 17 accepted guards retains its exact launch
anchor. Native decisions, current decoder validation, the full test/vet suite,
and browser navigation were checked; no defaultRunDeps field was added.

The closing Stage B check on 2026-09-09 reran the ordinary path after removing
one duplicate seed-eligibility check and two redundant native-catalogue copies.
The artificial consumer-mutation test was removed; the existing Snapshot API
continues to copy its newly added package-declaration field. All 35 targets
across the five repositories completed with zero live provider calls, retaining
the observed product counts 1, 1, 2, 3 and 1. The jieba count includes the
refused-decision fallback described above. Final owner runs are
`20260909-144526-go-http-server-a90f19009296`,
`20260909-144532-chi-8da44377446a`,
`20260909-144544-python-tutorial-game-0278399f361d`,
`20260909-144600-jieba-b3e42e7feb7c`, and
`20260909-145104-pykrx-c754c7b04fce`. Full tests, vet and the owner build passed.
Focused desktop browser checks covered the final count links, role catalogues,
mode-preserving navigation and the frontend/backend connection inspector.
The before/Stage A/after reports, receipts and independent simplicity review
are in `work/overview-implementation-20260909/final-review` in the desktop
project. This closes the implementation and count checks, not editorial
acceptance of every generated answer.

Every positive file ref restores through exactly one language adapter into a
typed target plan. An exact `--target` bypasses model portfolio selection but
still resolves unambiguously through that same typed adapter boundary.
`--target` narrows selection; `--force-platform GOOS/GOARCH` overrides the
normal Go platform choice.

Scouts are corpus-only. They do not execute a selected target's compiler,
ProgramIndex, dependency, categorization, grouping, or matching path. A
compiler projection belongs only to the selected typed target, so an
unselected language prerequisite cannot block another language's target.

## Language-adapter boundary

Each language adapter owns syntax, compiler/type-system mechanics, native
target interpretation, and exact local fact restoration. It returns one
atomic, language-neutral `programindex.Input` for its selected target. The
shared `programindex.New` boundary validates and seals that snapshot.

The adapter input contains:

- the exact `ProgramTarget`, its source refs, anchor, selector, and structural
  execution seeds;
- program objects with kind, display name, visibility, ownership,
  containment, exact source location, and optional signature;
- external symbols with their exact raw package origins and required,
  language-neutral `package` or `platform` authority kind;
- language-neutral relations with resolution, targets, witnesses, neutral
  patterns, and complete observed/omitted coverage;
- target-scoped deterministic dependency authority.

Adapter-private indexes may retain richer language facts, but shared semantic
and report code depends only on the sealed ProgramIndex and target-scoped
dependency authority. Adapters do not contain framework, protocol, or product
role allowlists.

Every eligible selected target runs its real adapter. Fixture or preset paths
may replace only provider responses in focused tests; they never replace
deterministic adapter execution.

## ProgramIndex

ProgramIndex is the single typed program graph passed from language adapters
to shared stages. The sealed in-memory graph is version 12, with unchanged
target, object, relation and nested provenance identities. Each target retains
its sealed `program-index-set.json` binding.

The owner approved shared project storage on 2026-09-09. Ordinary persistence
stores one complete common-builder input per exact parser view under the initial
run's `program-facts/<digest>.json`. A target's `program-index.json` is a storage-v1
reference containing its original TargetInput (including seeds), relative facts
path, facts digest and expected sealed index digest. Reading this reference
uses the existing `programindex.New` and checks its original seal; it invokes
no parser, repository read or provider. The standalone Encode/Decode format is
still the same complete version-12 Index. A complete cohort may move together;
missing shared facts fail without reconstruction from source. No automatic
target merging or alternate graph is introduced.

ProgramIndex and GroupsIndex hashing use a local value copy to clear the seal;
JSON serialization reads their nested collections without copying them first.
Categorization's base-seal check follows the same rule. Validation computes the
target object scope once per invocation and still rechecks every object and
the complete seal. Public snapshots and handoff isolation are unchanged; no
past validation is memoized for these publicly mutable structs. Paired saved
PyKrx microbenchmarks reduced ProgramIndex validation allocation from 19.55 to
14.58 MB/op and GroupsIndex from 9.64 to 7.69 MB/op with identical original
hashes. This is not an end-to-end latency claim.

ProgramIndex version 12 retains:

- exact target scope and seeds;
- objects and their stable local identities;
- adapter-observed package/module directories, independent of source locations;
- exact, alternatives, and unresolved relation authority as distinct states;
- structural relation kinds such as calls, contains, imports, implements,
  decorates, passes-callback, sources, executes, reads, writes, and
  invokes-external;
- complete witnesses and coverage counts;
- every source-distinct neutral relation pattern;
- call/decorator form, selector, invocation text, and exact source location;
- call-result and receiver identity;
- receiver-origin provenance and its resolution;
- positional and keyword arguments;
- literal, template, dynamic, and object-backed values;
- reconstructed value candidates and their source-object/source-argument
  provenance;
- exact symbol-link identities suitable for deterministic cross-shard joins.

Every external symbol has an explicit `authority_kind`. `package` means an
ordinary external package that may support dependency categorization and a
cross-target integration boundary. `platform` means standard-library or
runtime authority. Its raw `package_path` is still retained exactly, but it
cannot receive `dependency`, support a dependencies-lane group, or become a
cross-target boundary. Other positively supported categories remain allowed.
Adapters derive this distinction from language-owned deterministic authority;
shared stages never infer it from a path prefix or dependency-name heuristic.

ProgramIndex IDs are canonical local identities. Provider-facing stages assign
deterministic request-local refs and restore accepted rows locally. A model is
never asked to copy a UUID, canonical path, canonical ID, or source location.

Categorization is an optional sealed section of the same ProgramIndex type.
It binds its sparse assignments to the base ProgramIndex digest and the
reduced-documentation digest, then reseals the index. Enrichment cannot change
target, objects, relations, patterns, witnesses, coverage, or value
provenance. The base structural index is recoverable and independently
validatable.

ProgramIndex aggregate semantic-text and canonical-JSON size thresholds are
warnings only. Structural JSON overhead does not consume evidence authority,
and crossing a warning threshold cannot truncate or reject the index.

## Language-specific deterministic authority

### Go

The Go fact inventory excludes non-`DepOnly` `go list` roots with no
build-selected `GoFiles` or `CgoFiles`. A directory containing only external
`*_test.go` files is not an ordinary package when `Tests=false`; its raw row
may inform dependency metadata but cannot enter package counts, target
identity, or ProgramIndex scope. A source-bearing package that fails type
checking remains selected and fails its owning target closed.

The direct-call traversal is complete by default. `--depth 0` and
`--edges-limit 0` mean retain all reachable exact calls and edges; positive
values are explicit user narrowing. Large depth, node, or edge counts emit
aggregate warnings without truncation. Dynamic and unresolved frontiers remain
separate from exact edges.

The Go fact inventory also retains the complete build-selected package-origin
universe from every `go list -deps` row, including `DepOnly` rows. The Go
tool's exact `Standard` bit maps standard packages to `platform`; every other
known external package maps to `package`, and generated cgo `C` authority maps
to `platform`. An external target absent from that universe fails the adapter
closed rather than being guessed from its import path.

### Python

The checked Python catalogue validates once when built or decoded and retains
exact native-target, selector and scoped-module lookups. Explicit Validate
still checks a supplied edited catalogue completely; lookups do not rescan it.
Selected targets with the same root/module inventory enter the existing
BuildMany parser core together. It parses each source AST once, then produces
each distinct package/alias view independently. Identical views share one
immutable common input and one stored facts payload. Each target retains its
original scope, launch seeds, complete dependencies and model analysis.
A bad launch projection refuses only that target while its neighbours keep
their already parsed inputs. A failed shared parser preparation retains the
existing exact-target fallback; cancellation stops dispatch immediately.

The sharing regression for the root Python project has seven targets and fifteen source files:
one parser process, one AST per source, and two separate package contexts.
Every restored sealed index matches the separate build byte for byte. Shared
storage is 323,238 bytes versus 1,481,442 bytes for seven complete indexes.
This is fixture evidence; full Airflow completion time and peak memory remain
unmeasured.
Final GroupsIndex subjects and the final report still scale with the complete
selected portfolio; these changes do not make the entire run constant-memory.

The Airflow sample also exposed an avoidable restore during atlas
construction. Places now collects each target's declarations, relations and
seeds together, then makes one later pass for external boundaries after source
documentation and fact boundaries are ready. Each saved target loads twice
instead of three times, retaining only one target's native lookup maps.
Seed locations are resolved against the complete file inventory before depths
are assigned. The saved mixed fixture's canonical graph bytes are unchanged.
The stopped Airflow process predates this change, so its elapsed time cannot
measure this additional reduction.

Ordinary mixed Python/TypeScript acceptance at `20260909-200202` completed
in 5.689 s, publishing both selected targets and one Russian HTML. All 23
exchanges reused their exact cache entries. Compared with `20260909-195105`,
both native index seals, both GroupsIndexes, places, facts, claims, atlas,
orientation, Learn plan, question routes and terminology are identical.
All 374 display entries have saved translations; artifact/link checks and
browser opening passed. Full product tests with the installed real TypeScript
compiler and vet passed. Two older callback tests were corrected to assert
the compiler's qualified local alias declarations and native owners; compiler
resolution was unchanged. Receipts are in the desktop work directory's
`airflow-performance-review-20260909/fixture-after.json` and
`fixture-comparison.json`. The interrupted Airflow run has no published HTML
and is not accepted. Combining tool/example model analyses remains a separate,
unapproved A5 proposal; sharing project facts does not combine those decisions.

The Python adapter owns package/module scope, import restoration, call and
registration facts, decorators, arguments, target seeds, external origins,
and complete observed/omitted coverage. Dynamic dispatch remains alternatives
or unresolved authority; shared stages never repair it by matching names.
It maps an exact top-level import root in `sys.stdlib_module_names` to
`platform` and every other external root to `package`; an invalid or missing
authority kind fails the adapter boundary.

Repeated aliases in one import retain one witness for the same declaration
at the same source site; the observed count includes that witness once.
Distinct import statements and calls through each alias retain their own
locations. The 2026-09-09 ordinary Airflow check exposed a false omission:
`BaseFacet`, `BaseFacet as DatasetFacet`, and `BaseFacet as RunFacet` produced
one witness but an observed count of three, refusing the Python dependency
catalog and target. Counting only distinct witnesses fixes the producer while
keeping the complete-coverage check. Cumulative Python, Go, TypeScript and
JavaScript examples exercise native imports, distinct calls and complete
dependency coverage. The interrupted Airflow run is not acceptance; its
successful requests remain cached for the corrected ordinary run.

The Python adapter also keeps an existing callable candidate consistent
between an argument and the callback transfer that cites that exact argument.
Aliases assigned to a function or lambda remain alternatives; an inline lambda
retains exact authority, while unknown or overwritten aliases gain no callback.
The Airflow Edge3, Azure and Vertica libraries exposed the earlier mismatch:
the argument named the assignment variable while the transfer named its callable.
All three now pass the real local extractor and ProgramIndex construction;
ordinary full-Airflow publication still needs completion. Cumulative Python,
Go, TypeScript and JavaScript examples retain their native authority rules.

Nested Python calls now use their complete native AST span in local relation
and argument-pattern identities. A chain such as `push().map(first).map(second)`
shares its starting position but retains two distinct calls, each with its own
arguments and callback candidates. Original source locations are unchanged;
the source-argument consistency check is unchanged. This fixes the three
Airflow task-sdk targets that previously failed with an argument-authority
mismatch. All three pass the real local extractor and sealed ProgramIndex
construction; the running older full-Airflow attempt remains a partial check.
Cumulative Python, Go, TypeScript and JavaScript regressions preserve each
language's existing resolution strength and both callbacks. Go and JS/TS
already distinguished the calls and needed no production change.

Python adapter v11 retains each declared package's native directory in
ProgramIndex v12. A namespace package without `__init__.py` keeps no source
location; its exact directory supplies the existing workspace dependency row.
Relative named and wildcard imports into another explicitly named portion of
an advertised namespace retain their original external import boundary. A
nearer ordinary package/module does not authorize an unknown child. Importing
an unknown member directly from a known namespace keeps only that known
boundary; it does not invent a declaration or a callable.

All six previously refused Airflow shared libraries and the three task-sdk
targets now pass the real local extractor, sealed index construction and
complete dependency coverage, with no omissions. The focused nine-target check
took 45.36 s; it is not a new full online acceptance run. The cumulative Python
fixture covers namespace directories without source anchors, named/wildcard
imports, their original call resolution and the contrasting unresolved child
of an ordinary package. Ordinary shared storage and standalone index round trips
preserve these facts. Go and JS/TS have no direct PEP420 namespace equivalent;
their package resolution is unchanged. The two saved acceptance-fixture indexes
are resealed under v12 with their old observations intact, without a compatibility
reader or regenerated model results.

The ordinary mixed Python/TypeScript run `20260909-212223` published both targets
and one Russian HTML in 7.350 s. All 23 exchanges, including eight translation
windows, reused their exact accepted cache entries; all 374 display entries
have translations. Artifact, target-outcome and publication checks passed.
The earlier saved report also renders with the current templates without
analysis or provider requests. Full product tests with the real TypeScript
compiler, vet and the canonical build passed. That older full Airflow process
was stopped with nineteen failed target rows; this focused acceptance does not
turn that historical run into a successful full check.

On 2026-09-10 the owner explicitly authorized a new full current-main check.
All ten previously refused JS/TS targets passed their real compiler/index and
complete dependency checks after helper v18, alongside the nine Python targets
above. Full tests with the installed TypeScript 6.0.3, vet and build passed.
The new full ordinary run `20260910-052204-airflow-28821b3e997f` started on
`33c29c5c` at 05:22:04 UTC. Its live state and log are under
`work/airflow-validation-20260909/followup-20260910/`; full elapsed time,
coverage and HTML remain unaccepted until that run finishes and is audited.

The current declaration experiment uses adapter v11 and question table v6.
Native class ownership feeds the existing atlas type-member representation;
question selection now keeps those members beside the selected type, and
the report displays each member at its own source location. Types beyond
the description-candidate budget remain readable without raising that budget.
The ordinary public python-tutorial-game run
`20260906-224927-python-tutorial-game-8bbd9681f1f0` completed in 7m34s.
Two answer responses failed validation; a second ordinary run,
`20260906-225804-python-tutorial-game-37aaa0710132`, completed in 1m03s
with two live answer calls and no rejected responses. The sealed native and
atlas graphs are identical between those runs. The final report has sixteen
questions: six answered and ten partial. This is not whole-Learn acceptance.
The API answer now names RunLevelRequest's level_id/code fields and the
response fields, while remaining explicit that its selected evidence lacks
nested model definitions. The settings answer includes port/debug/reload
defaults. Native extraction, shared reading, publication tests, all product
tests and changed-package vet passed. Browser QA opened the field excerpts,
clicked models.py:52 into a separate captured-revision GitHub tab, and followed
the answer into Application core without leaving the report. The selected
class retains its members, but referenced types are not recursively expanded
into answer evidence. The acceptance receipt is
`work/learn-python-declarations-acceptance.json` in the desktop project.

Instruction clarification resolved on 2026-09-07: the owner explicitly approved
editing AGENTS.md. Its obsolete signature/source-expression omission rule was
removed, and its credential restriction now names the LLM client's authentication
credentials. The repository-trust rule remains as stated in the constitution.

### JavaScript and TypeScript

Every eligible `package.json` project is considered. Source ownership belongs
to the deepest containing manifest. Every manifest with owned tracked
JavaScript/TypeScript source contributes one required target representative;
a source-less manifest is tooling and cannot suppress another target.

`package.json#name` is optional. An exact top-level lockfile name is the
secondary identity; otherwise the root uses `root-package` and a nested
project uses its repository-relative directory. These fallbacks never
authorize an implicit string-form `package.json#bin` command.

An explicit `jsts:<manifest>` narrows the typed plan before compiler
execution. Each retained package target receives its own compiler projection.
The selected TypeScript Compiler API honors
`tsconfig.json` or `jsconfig.json`, repository-confined solution project
references, aliases, and module resolution. An in-package project reference
extends the page graph; an exact reference into another selected package is a
cross-target boundary. Missing references and references outside the analyzed
repository fail closed.

Repomap never installs npm, yarn, pnpm, or other packages. Local compiler
candidates come from `typescript` or exact npm aliases declared by the selected
manifest. A nested package inherits root candidates only when it declares none.
Each candidate resolves from its declaring package scope; without declarations,
ordinary local `typescript` resolution still applies. A usable local compiler
takes priority. Otherwise helper v18 checks the selected Node installation and
the first `tsc` on `PATH` for an existing TypeScript package, without searching
other Node installations. Both sources must identify themselves as TypeScript
and expose a supported Compiler API. Distinct compatible local candidates in
the selected API tier fail closed; one stable legacy Compiler API candidate is
preferred when a native-preview candidate is also deliberately declared.
The cumulative fixture has no TypeScript declaration and exercises both
environment discovery paths, unusable-local fallback and local priority through
the real extractor. Identical compilers produce identical results, indexes and
dependency catalogues from either location; compiler availability does not
supply missing project imports or create calls. Result v14 retains its shape;
helper v18 records the discovery-contract change, without storing host paths.

Exact owned manifest script inputs and supported tool-config files retain their
additional compiler program even if the written config selects only sibling
sources. With no owned configured program, it uses the existing inferred
defaults and admits the explicitly listed JavaScript inputs. Other excluded
files and the sibling's source inventory stay outside that target. The
cumulative documentation-tools package checks this with JavaScript and
TypeScript scripts, a sibling-only config and an unlisted script.

Browser, Node server, and canonical safe package-owned `package.json#bin`
command/path pairs are product surfaces inside the owning target. A CLI entry
never invents a wrapper-to-source relation. An exact canonical `dev` or
`start` script with one helper-selected source may seed that source only after
CLI product authority exists.

Compiler/type-resolved declarations and exact external imports are the only
call-target authority. TypeScript default-library declarations and exact Node
standard-library origins map to `authority_kind: platform`; npm origins map to
`authority_kind: package`. Their raw origins, including the closed
`platform:javascript` namespace, remain unchanged. The shared platform policy
then excludes only dependency and integration-boundary authority while
retaining structural evidence and any other positively supported category.
Calls and constructions remain distinct. An unresolved property name stays
unresolved and is never joined to a repository declaration by name alone.

A compiler-resolved sibling package retains `repository_path` on each import,
call and ProgramIndex external symbol. This is an optional origin fact, not a
new graph or framework classification; an empty value means that no repository
origin was established. Result v14 / helper v18 replaces the old lossy handoff.
ProgramIndex v12 retains the optional field in its existing seal, as do
GroupsIndex and the report view. Dependency rows distinguish the exact package
name plus directory: two same-name siblings and an installed package coexist.
`jsts_package_export_v2` identities include the sibling directory; installed
packages have no repository export identity. Receiver provenance keeps the same
scope and original call resolution. Local package names such as `got` or `fs`
do not establish HTTP, SDK or standard-runtime behavior. The report links a
sibling only to its exact analyzed directory, and cannot substitute another
same-name component. Cumulative JS/TS examples retain two `got` packages and
an installed `got`; Go and Python examples likewise keep local HTTP-looking
names separate from actual external HTTP calls.

Method/path-shaped calls retain their selector, receiver and external origin,
positional arguments, callback refs, reconstructed values, and unresolved
frontiers only as neutral ProgramIndex patterns. The adapter does not build a
parallel route catalog, HTTP-use catalog, resource catalog, product path, or
deterministic HTTP join. Target-local meaning comes from categorization and
grouping; cross-target connections come only from repository matching over the
complete GroupsIndex set.

JSTS result v14 / helper v19 retains callable JSX attributes as anchored
`passes_callback` relations with `callable_binding:jsx_attribute` invocation.
Element and attribute names remain source observations; no event-name or
framework allowlist assigns meaning. Compiler-confirmed function-valued
variables remain callable when initialized through a wrapper. Inline anonymous
JSX functions and unindexed callable factory results currently keep an
unresolved binding; a factory result is never a binding to the factory itself.
Bindings are rebased with the rest of the helper output for nested packages.
Calls in non-callable local value initializers belong to the enclosing callable,
while those values retain separate receiver/argument identities. An imported
factory does not give exact import authority to a method on its result.

Direct interface property declarations now enter the same native declaration
catalogue. Their written signatures preserve optional/readonly modifiers and
nested field types; original names, locations and owners remain exact. A nested
type-literal member is not lifted into the outer interface, and inherited
members do not acquire a declaration at the derived interface. These are
type-owned variables, not callable implementations. The existing atlas member
projection carries them into question evidence. This repairs the deterministic
loss that left IGetLevelsResponse without count before any question selection;
it does not establish that a later model answer selects or explains the field.

Go core objects v3 similarly retain explicitly declared struct fields with
their native locations and signatures, including tags and embedded fields.
The Go adapter projects them into the same type-owned variable objects already
used by Python class declarations; no shared graph kind or body retrieval is
added. The cumulative language repositories have comparable count-field
examples linked from testdata/repositories/README.md. Their checks follow the
real adapters through the atlas and question evidence and distinguish
same-named fields in separate owners. This is local regression evidence;
new ordinary answer acceptance remains open while the provider is unavailable.

The field regression completed locally on 2026-09-07. All eight direct
TypeScript example fields reach question evidence with their source anchors;
the fixture covers optional/readonly fields, comments in multiline object
types, literal strings containing node_modules and significant spaces, and
multiline/interpolated template literal types. Source-derived signatures are
distinguished from compiler-generated type text before metadata normalization;
literal values survive the single-line representation and host-absolute import
paths remain excluded in both forms. The full JSTS suite ran with the existing
TypeScript 5.9.2 compiler (23.232s), not a skipped compiler check. Go producer,
adapter and discovery suites, cumulative contracts for all three languages,
atlas places/lines/reading tests, changed-package vet and final make build all
passed. No provider request or new ordinary report was made for this change.
Continue next with the owner's display/navigation comparison, not a broader
audit of every generated answer.

The cumulative JSTS fixture verifies wrapped callbacks, same-name callbacks in
different owners, internal render props, unresolved factory results and source
identity after sealing. Real TypeScript 5.9.3 checks use a temporary prepared
compiler prefix when the developer machine has no global compiler; a skipped
compiler suite is not acceptance. The ordinary product still installs nothing.

Optional signatures and source-expression display text matching the
always-on persistence guard are removed before initial ProgramIndex sealing;
identity, source location, resolution, targets, and witness counts remain.
Sensitive required identity or semantic fields still fail closed.

## Dependency authority

Dependencies are deterministic target-scoped facts, not model-authored
inventories. They bind exact package/import origins and applicable metadata to
the owning ProgramIndex target. Shared categorization and grouping can use
those facts through the ProgramIndex's external objects and relations, but a
model cannot invent an unobserved dependency.

Platform namespaces are not dependencies. Shared contracts are supporting
code, build and migration scripts are tools, and a runtime script, library, or
tool-only root does not promote itself into an application.

## Sparse overlapping categorization

`program_categorization` receives one base ProgramIndex and the exact reduced
documentation handoff. It advertises request-local refs for ProgramIndex
objects and relation patterns and returns zero or more assignments from this
closed vocabulary:

- `inbound` — externally delivered work such as HTTP/RPC requests, event or
  message subscriptions, webhooks, and equivalent request surfaces;
- `background_activity` — scheduled, startup, consumer, worker, watcher,
  polling, reconciliation, CLI invocation, and other independently initiated
  activity;
- `core` — domain responsibilities and coordinating application behavior;
- `dependency` — external systems, storage, services, packages, and boundary
  use.

The result is a sparse cover, not a partition. Each disjoint request is sparse
in rows but complete for its positively supported assignments: it inspects
every owned ref, is neither top-k nor a sample, and may return empty only when
none of those refs has a positively supported category. This does not require
an acknowledgement row for every ref. One subject may carry several
categories; an uncategorized subject remains in ProgramIndex without a
synthetic fallback. Unknown refs, malformed rows, empty category sets, and
invalid categories are discarded and diagnosed locally. A model-selected
`dependency` category that contradicts explicit `authority_kind: platform` is an
unsupported set member: only that category pair is discarded and diagnosed,
other valid categories on the same row remain, and the sealed ProgramIndex
rejects any reintroduced copy. Known accepted rows are canonicalized and
applied directly to the same ProgramIndex, which is resealed with both input
digests.

Categorization plans a deterministic disjoint cover of at most 32 owned
subjects per request and repeats the complete reduced documentation while it
fits. This is a semantic-focus partition, not a count limit: every subject is
owned by exactly one request for each required documentation shard, incident
graph context remains closed, and accepted positive rows are unioned without
sampling or truncation. If complete documentation cannot fit beside every
indivisible subject, only the documentation reservoir is losslessly sharded
and the exhaustive subject cover is repeated for every shard.

Categorization does not require every object or pattern to receive a class,
does not create groups, and does not construct the presentation layout.

## GroupsIndex

`program_grouping` consumes one complete enriched ProgramIndex. GroupsIndex
version 4 carries the required external-symbol authority kind and exact
per-member lane compatibility through the
self-contained downstream graph. Grouping proposes a
sparse set of developer-facing responsibilities and directed semantic
connections within that target. Go restores every request-local member and
evidence ref before `groupindex.Build` assigns stable identities and seals
`groups-index.json`.

Every group has:

- stable ID, title, and short summary;
- exactly one lane: `triggers`, `core`, or `dependencies`;
- one or more exact ProgramIndex object/pattern members;
- zero or more exact evidence subjects.

Lane selection is locally constrained for every direct member by its accepted
subject categories: `inbound` and `background_activity` support `triggers`,
`core` supports `core`, and `dependency` supports `dependencies`. One
compatible member cannot promote an incompatible peer into its lane. The two
trigger categories share the `triggers` lane. Membership is sparse and
overlapping: a multiply categorized exact subject may support several useful
groups, and uncategorized or ungrouped subjects are not deleted. Evidence may
cite advertised context, but it never becomes group membership. For a
`dependencies` group, explicit platform objects and exact invocation patterns
whose complete targets are platform authorities are unsupported evidence and
are discarded individually; standard-runtime APIs cannot substantiate an
outbound dependency group. Local callers and other advertised context remain
valid evidence, and connection evidence is unaffected by this group-lane rule.

GroupsIndex is self-contained for matching and report projection. It retains:

- the exact target and enriched ProgramIndex digest;
- every ProgramIndex object and relation pattern as a typed subject, including
  subjects with no group membership;
- accepted categories on each subject;
- object, pattern, argument, receiver, result, external-origin, value, and
  source-location facts needed downstream;
- deterministic structural edges derived from exact ProgramIndex structure;
- model-authored semantic group connections with exact evidence endpoints;
- its canonical seal.

Grouping and matching return canonical diagnostics for rejected proposals
alongside the graph; diagnostics are not graph nodes or connection authority.

Structural edges and semantic group connections remain distinguishable. The
former restate adapter authority; the latter are validated, evidence-bound
explanatory relations. A report projection cannot turn either kind into a
stronger claim.

Grouping may use provider-sized disjoint batches and a deterministic merge
round. Initial shards remain sparse positive selections. A merge is lossless
consolidation of their validated membership: for every candidate group, one
returned group in the same lane must contain that candidate's complete member
set. A single returned group can cover arbitrarily many candidates; no
candidate `g*` acknowledgement rows are required. The check runs after closed
ref filtering and canonicalization. Every candidate member has already been
validated individually against its lane; merge cannot use another member to
promote an incompatible ref. A member may additionally appear in another group
only when its own categories support that lane, and that duplication does not
replace the candidate's same-lane containment. An incomplete merge rejects the target's
complete grouping result instead of locally restoring or promoting omitted
members. Every accepted batch is restored against the same complete input.
Unknown members, unselectable categories, unknown evidence, conflicting group
keys, lane-invalid groups, or invalid connections are discarded rather than
guessed. A terminal batch failure rejects the target's complete grouping
result.

## Cross-target group matching

`group_matching` consumes the complete validated GroupsIndex set and no
separate program or documentation authority. It assigns request-local refs to
targets, groups, subjects, arguments, reconstructed values, structural edges,
and existing target-local connections.

Every unordered pair of groups belonging to different targets is considered
exactly once to derive its complete deterministic witness set `J(pair)`. For a
candidate-bearing pair, the request is an exact pair-local dossier rather than
a repeated copy of both complete target graphs:
the two endpoint groups retain their complete model-selected member/evidence
sets and separate deterministic `boundary_edge_refs`. Every boundary ref names
an existing GroupsIndex structural edge with role `pattern_target`,
`pattern_receiver`, or `pattern_receiver_origin`; its source is a pattern and
its destination is an exact non-platform external package symbol. The pattern
is group-owned only when its local source object is itself an object member or
reaches an object member through a finite, cycle-safe chain of exact
OwnerID/ContainerID facts. Evidence subjects are never roots; calls, structural
adjacency, local semantic connections, paths, frameworks, names, and selectors
never enter the ownership closure. This allows an outer service function to
remain the readable card member while a boundary in its nested callback or
call-result object is available to matching without being promoted into
presentation membership.

An eligible boundary edge normally has exact structural resolution. One
narrow language-neutral exception represents dynamic inbound or background
registration without a framework allowlist: an `alternatives`
`pattern_receiver_origin` edge is admitted only when its endpoint group is in
the `triggers` lane, the pattern itself has an accepted `inbound` or
`background_activity` category, and its retained receiver-origin set contains
exactly that one external symbol. This is possible boundary support, not an
exact framework identity, binding, runtime call, or request occurrence.

Subjects retain the deterministic closure of every member/evidence ref and
every advertised boundary edge's pattern and external endpoint, plus incident
local-connection evidence. Structural context retains the advertised boundary
edges and every one-hop edge incident to the endpoint
member/evidence/boundary-pattern subjects, together with the counterpart and
nested provenance owners required to close those refs. Target-local semantic
context retains only connections incident to an endpoint, with compact
neighboring-group facts.

Before any provider dispatch, Go exhaustively combines every eligible left
boundary edge and source-pattern-owned argument with every eligible right
boundary edge and source-pattern-owned argument. It keeps exactly the pairs
whose direct or locally reconstructed literal/template values are equal and
whose boundary/value resolutions can support a connection. Each surviving
pair becomes one canonical request-local witness candidate with a closed `j*`
ref. The `witness_candidates` catalog exposes the candidate's two boundary
edge refs, source-pattern refs, argument refs, and locally derived
`support_resolution`; the model never has to reproduce paths, values, or the
expanded witness tuple. Group evidence can explain a boundary but cannot lend
its boundary authority to an endpoint or become an ownership root.

Two possible boundary edges cannot form a candidate. One possible
semantic-trigger edge must be opposite an exact edge and the shared argument
value must be exact; its candidate remains possible. Two exact boundary edges
may still produce possible support when their equal value has only possible
reconstruction authority. Candidate construction is exhaustive and
deterministic, with no count cap or model-free semantic promotion: a `j*` row
is only admissible evidence for a possible model-authored connection, not a
connection in the graph.

Only a group pair with a non-empty `J(pair)` becomes an indivisible provider
item. A zero-candidate pair contributes no connection locally and makes no
provider request, cache lookup or write, or observer call. This absence is not
a negative semantic fact, a model answer, or a semantic fallback; it means
only that the closed local authority contains no admissible witness that a
model could select for that pair.

The model may return a directed semantic connection only between the advertised
endpoint groups, and only when those groups themselves retain opposite sides of
one direct integration boundary. For each proposal it authors the direction,
open snake-case semantic kind, label, and summary, then selects one or more
advertised refs through `witness_joint_refs`. It cannot invent an expanded
witness, supplementary subject evidence, a third endpoint, or a stronger
resolution. Pair ordering has no direction authority: `from` must be the actor
named by the semantic kind and the grammatical subject of the label/summary,
while `to` is the acted-on endpoint. A backend listed on the left does not
become the caller of a frontend merely because of request order. In the narrow
case where one `j*` joins a positive inbound delivery pattern in a
triggers group to a positive dependency-category exact outbound
`invokes_external` call, Go includes closed `required_from_group_ref` and
`required_to_group_ref` fields on that candidate. Normalization rederives this
orientation and discards a contradictory row; it never flips or rewrites the
model edge. A dual-role subject, arbitrary background activity, or bare
trigger-lane membership has no direction authority. Sparse empty output is a valid result: omission of a candidate is
not a negative fact, and no candidate is promoted without model selection.

Go resolves every selected `j*` ref inside that exact pair, revalidates its
boundary ownership, argument ownership, equal-value authority, and resolution
against the same GroupsIndex facts, and discards unknown or invalid refs. It
automatically restores both source-pattern subjects of every selected
candidate as bilateral connection evidence. Compatible selected candidates
are merged, and the strongest surviving resolution is persisted on the
connection as `support_resolution: exact|possible`. A row with no surviving
selected candidate is discarded before compatible accepted rows are merged.
Possible support is never promoted to an exact runtime call, binding, framework
identity, or occurrence. The underlying value candidates and their resolutions
remain in the automatically restored pattern evidence.

This closed selection contract prevents separate incomplete rows, local
call-chain adjacency, a handler's upstream domain logic, downstream UI
consumers, similar model/type names, and other indirect relationships from
becoming cross-target edges. Unknown pairs, invalid endpoints, malformed rows,
unknown witness refs, and legacy expanded-witness rows are discarded with
diagnostics. Candidate-bearing items execute through the shared bounded worker
pool. Their complete eligible batch remains atomic: a terminal item or
indivisible request/response envelope failure rejects the complete matching
result instead of publishing accepted siblings as partial graph authority.
Matching never creates new targets, subjects, groups, structural program
edges, arguments, reconstructed values, or locally unsupported value equality.

The matched GroupsIndex set is the full graph consumed by publication. There
is no second graph, report-only matching pass, or browser-side semantic
reconstruction.

## Facts, claims, and orientation

`docs/CONSTITUTION.md` is the product constitution and defines three labeled
layers of truth. The group graph is the model layer; it sits on top of the
deterministic fact layer and never replaces it.

`facts` is a deterministic stage over the repository corpus, the sealed
ProgramIndex set, the dependency catalogs, and the manifests. It runs after
index extraction and writes one repository-wide `facts.json` into the owner
run. Its closed row kinds are entrypoints from adapter seeds, HTTP
server routes and client calls with their method and path literals, cross-target
portals, environment-key config reads, risk calls, manifest rows, TODO markers,
file-level imports, dead modules, negatives, and dependencies. Every row carries
an exact anchor and a stable id derived from its root, kind, anchored path, the
anchored line content, and its principal literal. A template path keeps its
holes as parameters and is `possible`, never exact. A portal exists only when
one call matches exactly one route of another target; zero or several
candidates produce a diagnostic instead of a fact. Environment files are never
read: only the tracked path may be recorded.

Listen address facts parse bracketed IPv6 host/port pairs, including scoped
addresses such as `[fe80::1%eth0]:8080`, with the same existing port rules.
Unix socket paths keep their existing handling. Native Go `net.Listen`
examples preserve distinct IPv4/IPv6/Unix call sites and exact source anchors.
The Python tuple/host-port and JS numeric-listen APIs do not use this string
address parser; this fix adds no new API recognition.

`claims` quotes human-written text with its source path and, when known, its
date and age: README paragraphs and headings, docstrings and doc comments,
marker comments, and recent commit subjects. Only the shallowest README speaks
for the repository on the overview. A quote whose text matches a credential
shape is withheld and counted.

`orientation` is one model-assisted stage over facts, claims, and the complete
matched GroupsIndex set. It returns one repository summary, one role per
target, a run recipe, and one main flow. The model selects request-local refs
that Go restores to exact fact, claim, and subject ids. Validation is a pure
function over the response and the advertised catalog. Set refs filter unknown
or incompatible members and deduplicate repeats, recording ignored refs; a row
with no required evidence, a recipe step with no manifest or entrypoint evidence,
or a flow step naming another target's member is rejected with its raw JSON and
a reason into `rejected.jsonl`. Summary, roles, recipe and flow validate separately,
so a wrong field type does not discard accepted sections. Equivalent target
roles combine their evidence; conflicting roles leave only that target's role
unavailable. Complete prose and qualifications survive; only short labels collapse
whitespace. Unparseable responses yield the legitimate empty orientation and are
not cached. Optional terms follow accepted sections and rows on live and cached
responses. Rejection never aborts the run and no replacement is invented.
Preparation keeps the original context against the actual provider envelope;
the former 2 MiB limit and its size-triggered claim/member/fact removal are gone.
The stage caches on its stage identity, prompt version, and input digests through
the shared executor.

## Multi-target orchestration and failure isolation

Every selected typed target runs its complete target-local path: preparation,
adapter projection, base ProgramIndex validation, categorization, enriched
ProgramIndex validation, GroupsIndex construction, and page validation. A
non-default target is not a structural substitute.
The page-local `ProgramPortfolio` is only the deterministic target plus
ProgramIndex presentation projection. Its entries carry no semantic
availability or `structural_only` discriminator: the enriched ProgramIndex and
GroupsIndex are mandatory, separately sealed authorities for every successful
page.

A selected target may fail at preparation, program analysis, dependency
analysis, semantic analysis, or page validation without erasing completed
sibling pages. `target-outcome-portfolio.json` contains one exhaustive,
adapter-neutral row for every selected target. A row either binds one complete
validated ProgramTarget/page/run or carries only a closed public failure stage
and reason. It never persists raw errors or adapter-native refs and never turns
a failed target into a partial page.

`program-page-portfolio.json` is the complete language-neutral binding from
every successfully analyzed ProgramTarget to its safe child run. It has an
explicit logical default and does not infer one from slice order. Both
portfolios are sealed and persisted once in the successful owner run.
The same artifacts are mandatory when the repository selects only one target:
that run contains a one-page ProgramPagePortfolio and one exhaustive outcome
row, with no direct-page or browser-synthesized fallback.

JavaScript/TypeScript compiler materialization occurs inside each selected
package boundary so a missing compiler cannot preflight-fail unrelated
targets. A shared Go workspace is only an optimization: if union preparation
fails, the current exact target is retried locally and later Go targets remain
isolated. A target-local fallback workspace never becomes sibling authority.

The embedded JS/TS helper reserves an exit status for compiler-load failures;
stderr remains diagnostic text and cannot choose the public failure reason.
Missing Node retains the same prerequisite cause. Both Python parsers retain
the process exit error even with empty stderr, and enforce their existing
stderr bound through Write without an inherited bytes.Buffer.ReadFrom bypass.
Each helper accepts one complete JSON response; trailing values are refused.
Context cancellation retains its original cause. These transport-error fixes
do not change successful native results, their versions or model cache keys.

Context cancellation and complete-portfolio, persistence, manifest,
repository-overview, graph-set, or publication failures remain terminal. If at
least one target succeeds, the successful page portfolio remains valid even
when it has one page. The first deterministic successful page owns the one
physical HTML; the originally selected default remains the logical default in
the outcome portfolio. If every selected target fails, diagnostics are
retained but no targetless report is invented.

The final publication-failure block preserves the successfully analyzed target
count and shows the original wrapped failure reason. It links the owner run's
existing `rejected.jsonl` and semantic exchange directory when available.
These are console diagnostics, not raw-error fields in TargetOutcomePortfolio;
the original error still propagates unchanged.

After a failed model exchange is committed, the same journal recorder supplies
direct absolute request, raw-response and journal paths to the run console.
An unavailable body is identified explicitly rather than linking its marker as
raw content. Buffered first-layer failures notify only after their journal is
flushed; accepted answers with discarded optional terminology do not emit a
failure notice. Payloads are not copied or reformatted for the console.
The provider carries the last transport attempt's HTTP status and selected
diagnostic response headers through the shared executor into this journal and
notice. Request/trace/correlation IDs, retry/rate-limit headers, date and server
are retained; request headers, authorization, cookies and arbitrary token/key
headers are not collected. Total attempts and elapsed time still describe the
whole completion. HTTP metadata is diagnostic only and changes neither request
identity nor accepted-cache records; a warm hit makes no claim about a new HTTP
response. Raw HTTP error bodies remain exact, including non-JSON 500 responses.

## Persistence and publication

Manifest version 40 records the repository and publication source. Across a
successful repository run the following artifacts are persisted, as applicable:

- repository corpus and repository-guidance authority;
- `reduced-documentation.json`;
- the target ProgramIndex reference, shared `program-facts` input and `program-index-set.json`;
- the target-scoped `dependency-catalog.json` artifact;
- the matched `groups-index.json` artifact;
- `program-page-portfolio.json`;
- `target-outcome-portfolio.json`;
- `facts.json`, `claims.json`, `orientation.json`, and `rejected.jsonl`;
- manifest-bound `report.json`;
- the single owner `report.html`.

Multi-target publication retains each target's ProgramIndex, dependency
catalogue and GroupsIndex. Shared artifacts, the manifest, report JSON and HTML
are published once in the owner run from values already held in memory. A
served report has the same complete set of target sections; it needs no sibling
report files. Saved report restoration reads the common JSON and manifest.

UI iteration uses `repomap render RUN_DIR --output FILE.html`. It restores the
current common report and completed display translations with `ReadRunReceipt`,
restores captured remote source links, and calls the same `RenderHTMLWithOptions`
as ordinary publication. No provider or cache is initialized and no analysis is
run. Missing or incompatible saved data fails without regeneration. The original
analysis and translations stay unchanged; the chosen HTML is atomically replaced
only after successful rendering. This is the owner-requested 2026-09-08 remedy
for hand-assembled UI prototypes and cache-dependent analysis reruns. Editing the
ordinary templates, building and rendering saved data is the UI acceptance loop;
analysis changes still require ordinary pipeline acceptance.
Adding destination-side views of existing cross-component connections exposed
an ordering dependency in display translation refs. Saved rendering now rebinds
only a complete bijection of the same catalogue entries (role, text and protected
spans), using the original saved display catalogue. Both catalogues and both
translation bindings are validated. It never translates, drops or invents an
entry, and leaves saved files unchanged. Missing or changed prose still fails.

The report is one static page rendered in Go from the run's own verified
report data. Its overview answers what the repository is, which targets exist,
how they talk to each other, what is missing, and how to run it; each target
then reads inbound routes, entrypoints, core responsibilities, external calls
and dependencies, the main flow, risks, configuration, dead code, and TODOs.
Provenance is visible: facts are plain, model sentences have a distinct colour
and an explicit source button when sources exist, and claims are
quoted with their source and age. Anchors are `path:line` links
to the captured revision on GitHub or GitLab, or to the local editor opener in
served mode. The page embeds no analysis payload; it is stamped with the digest
of the report.json bytes it was rendered from. The Canvas renderer, the browser
payload projection, and the per-target chunk transport were removed.

The target picker keeps failed rows visible, red, disabled, and linkless. The
repository overview reports analyzed versus selected coverage.

The desktop overview presents each product as a row with its existing purpose,
the complete named request/command/activity list, and links to Parts and
Integrations catalogues (all N), without selecting a top-N subset. Tools, examples and fixtures retain complete expandable
role inventories with counts. Rows retain the repository map's original nodes
and connections; a product name opens its component directly, while a separate
action shows its complete source context and grouped neighbours. Selected
parts use the existing map stage for incoming neighbours, a centre with exact
key-code declarations and the existing full-code inspector, and outgoing
neighbours. Remote peers are grouped by native component with full component
and relation counts; their complete lists expand beside the selected reading.
An empty incoming or outgoing side does not reserve a blank column: the centre
uses that space and retains a compact zero count with the original explanation
that no such connections were recorded. Both nonempty sides retain their
original left/right positions and all original relations.
Reciprocal relations remain separate directed evidence and highlight the same
peer together. Leaf focus uses exact neighbouring parts instead of introducing
an extra folded area. Destination pages also project incoming cross-component
connections from the same source-owned indexes; they remain interpreted stubs
with original anchors, never inferred native calls. Learn's full question menu
and Work's single shared search enter this space. The compact reading address
names the original question/search and the selected component, part and source.
The existing browser history stores reading context and exact map state per
visit, so visiting the same part for another question does not replace the
earlier visit's selection. Hover never changes selected reading.

Incoming requests project request operations from the sealed GroupsIndex v7.
An operation's exact FactID links it to the observed HTTP route; each such fact
appears once, and observed routes without a map group retain their own anchors.
Unbound model requests remain explicitly labelled interpretations. Other
operations keep their activation kinds, including scheduled and continuous work.
Missing categories occupy one component coverage disclosure; no missing HTTP
fact is presented as absence of model-observed requests. Report v86 names the
analysis directory rather than the surrounding repository/module identity,
including the localized HTML filename.
The current run recipe cites entrypoint or manifest facts and is explicitly
labelled inferred; a citation to an entrypoint is not a documented command.

Local explanation/code actions retain the current question, part or operation.
Code names link directly to exact original sources, with a separate explanation
button. Learn/Work switches preserve the current reading. Hash navigation aligns
the containing reading surface after layout, accounting for the actual toolbar
height; it does not depend on scroll margins on SVG anchors.

Stage A acceptance on 2026-09-09 used new ordinary online runs, with the final
template fixes included: `20260909-124955-go-http-server-a1bac5da50e7`,
`20260909-124955-chi-ce7667722810`,
`20260909-124955-python-tutorial-game-4c879511b3f3`, and
`20260909-124955-jieba-63a23a9229ae`. All four exited zero. Their common
manifest/report/HTML, complete shared places and atlas, and each target's sealed
ProgramIndex, dependency catalogue, reduced documentation and GroupsIndex were
checked. The targets remain 3, 4, 2 and 21 respectively: this stage changes
presentation, not target independence. The final ordinary runs reused provider
responses; the initial Stage A runs also exercised live analysis and translation.
Desktop browser checks at 1280 by 720 confirmed the complete input catalogue,
direct code links, question/part/code return paths, and mode changes retaining
the selected place. A fresh operation link placed the map at 118.49 px below a
toolbar ending at 102.52 px. Jieba's 19 tool rows all expand; connections to
closed rows are drawn only after expansion, while their original evidence stays
available in the component's connections. The full product test/vet check and
focused publication/navigation checks passed. Native-target decisions and
documented fenced commands are recorded under Target discovery and selection.

The owner explicitly approved final localization on 2026-09-07/08 and supplied
two diagrams clarifying its boundary. Analytical LLM cubes receive one shared
English response instruction. The completed English frontend structure then
feeds two presentation steps: a deterministic dictionary translates our own UI
vocabulary, and an LLM cube translates explicitly selected generated display
prose. These values are assembled before rendering. This is not translation of
analysis artifacts, source quotations, code names, paths, IDs or graph topology.
The implementation passed ordinary publication on the small Go server and its
two sibling library/script components. The first run `20260907-202410` finished
in 5m56s and translated 160 collected display texts in one 37s call. After UI
catalogue corrections, `20260907-203247` repeated all analysis and translation
from cache in 2s with zero live calls. `20260907-203910` then translated the
revised 152-entry catalogue after ordinary words were removed from the protected
code-name set. Source names, paths and explicit code syntax stay original;
ordinary prose words such as Run/service/protocol remain translatable.

Focused publication/server regressions prove that canonical report.json is
unchanged by localization, only the selected HTML is installed, a failed
installation removes its translation and final artifacts, and a restored server
uses the same saved translation while retaining exact source opening. The
legacy report.html redirect preserves mode query parameters. Dictionary-only
no-model publication never creates a provider. Clearing an isolated copy of the
real cache removed its persistent entries while preserving JSON, translated
HTML and the saved display translation. Full product tests and vet passed;
follow-up UI and smart-question changes continue through focused acceptance.
The early online translation runs exposed a missing closed journal-stage
registration: their translation and cache succeeded, but a diagnostic exchange
warning was emitted. `report_translation` is now registered and covered by the
journal round-trip regression. The ordinary `20260907-211253` run records its
accepted exact translation exchange without that warning. Its predecessor
`20260907-205650` was correctly left unpublished when the translation echoed an
extra `protected` field. The first wire format subsequently tolerated that
irrelevant echo while retaining text/placeholder/completeness checks. Larger
chi and jieba responses then exposed malformed JSON in repeated `ref` fields;
these are not envelope refusals and remain rejected without local repair.
The historical translation contract v4 sent and received one flat JSON object,
`{"t1":"text","t2":"text"}`. Input preserved the catalogue's original order;
role labels and original protected-source metadata remained local. Every text,
closed ref and placeholder still crossed the wire. This avoided redundant
wrappers and repeated entry fields without trimming the catalogue or imposing a
row count. The decoder preserved duplicate keys to reject conflicting values,
discarded unknown refs, and rejected missing or non-string known values,
altered placeholders and trailing JSON. A 600-entry regression covered one
complete request without a row-count cap. Current v9, described in the shared
glossary section above, supplies ordered entries with role and applicable refs
into one request-local terminology dictionary, and accepts one text object per ref.
Under the owner's 2026-09-09 rule, repeated JSON object keys keep their last
value, including nested `text` keys. Translation validates only that final raw
value: a shadowed invalid value cannot reject it, and an earlier valid value
cannot repair an invalid final one. Original response/cache/journal bytes remain
unchanged. This does not change completeness or repeated-row checks on arrays.
Source-placeholder validation preserves every original occurrence and rejects
unknown refs, while allowing extra occurrences of the same known placeholder
for natural phrasing. Every occurrence restores the same original source bytes.
The prompt continues to prefer the original multiplicity; accepting a harmless
repeat does not change request bytes or invalidate accepted translations.

Earlier v2/v3 live calls succeeded on several complete catalogues but sometimes
returned an extra closing delimiter. These malformed responses were rejected,
never repaired or cached, and their runs published no HTML. The transport path
was checked for local suffix insertion; the extra delimiter was already in the
extracted provider message. At that time there was no automatic malformed-JSON retry. A
bounded v4 control on the problematic server catalogue accepted all 148 texts
and every protected occurrence on its first attempt: 5,424 input and 7,110 output
tokens in 32.073s. Its prepared request shrank from 33,070 to 25,556 bytes. This
is evidence for the simpler wire format, not a guarantee that a provider will
always return valid JSON.

The display collector distinguishes an observed HTTP route name from its
model-written purpose: the former stays original and the latter is translated
in both the menu and map inspector. An unused README view field is omitted
from the translation catalogue; its canonical report field remains. Literal
text resembling a generated protection marker is itself protected, so restoring
code snippets cannot accidentally substitute a source author's literal marker.
Executable regressions cover these boundaries and the rendered operation text.
Final browser reading of Jieba also distinguishes our evidence-kind headings
from the quoted text beneath them: headings use the UI dictionary while the
original excerpt stays literal. Cross-component link labels rebuild their
component disambiguators from their already resolved destinations; native names,
source anchors and model-written descriptions retain their separate ownership.

The accepted v4 translations and complete ordinary publications were verified
on four real repositories:

| Repository | Owner run | Complete targets | Questions: answered / partial | Display texts |
| --- | --- | --- | --- | --- |
| go-http-server | `20260907-223921-go-http-server-92a6e716e1e1` | 3/3 | 1 / 7 | 147 |
| python-tutorial-game | `20260907-223922-python-tutorial-game-94f3805eb917` | 2/2 | 10 / 3 | 317 |
| chi | `20260907-223911-chi-0ff23ffb96e0` | 4/4 | 13 / 2 | 607 |
| jieba | `20260907-223904-jieba-e4569fcdc91e` | 21/21 | 6 / 2 | 412 |

Each used its accepted analytical cache and one successful v4 translation
request. These four translation calls used 37,845 input and 50,749 output tokens
in total; these figures are not the cost of their preceding analysis or earlier
format experiments. Full catalogues and all protected source occurrences were
validated. The 412 restored Jieba values appear in its HTML, with 20 Chinese
source excerpts preserved exactly. Each publication has one English canonical
report JSON, one selected Russian HTML and the original per-target indexsets,
dependency catalogues, reduced documentation and GroupsIndexes. Ordinary warm
repeats for all four made zero live calls or transport attempts. Canonical
report content except timing and saved translations remain stable; Jieba's
warm owner is `20260907-224255-jieba-03a710366393`. Full `make test`, `make vet`
and `make build` passed on this final code. After the final dictionary-only
corrections, ordinary display refreshes at `20260907-225451` (server, fixture
and chi) and `20260907-225452` (Jieba) reuse these same accepted catalogues and
translations with zero provider calls; the owner checklist links those latest
HTML files. `work/final-{server,fixture,chi,jieba}-display-refresh.json` binds
these refreshed publications to the original v4 verification.

These are ordinary-path and display checks, not a guarantee about every answer.
For example, chi's run answer still leaves the examples directory implicit,
Jieba's parallel-processing answer omits the Windows limitation, and the small
server has seven explicitly partial answers. The earlier chi test-first order
inversion is fixed. The owner can review all current questions and the retained
visual alternatives in `work/repomap-review-current.md`. Exact verification
receipts are `work/final-{server,fixture,chi,jieba}-v4-acceptance.json`.

The ordinary server `20260907-211253` finished with all three targets, one Russian
HTML, English report JSON and saved translation. One formerly rejected answer
and its updated display translation were the only live calls. The next ordinary
run `20260907-211622` completed in 2s with zero provider calls, including reuse of
all 224 question/evidence decisions and the complete translation. Exact target
indexes, dependency catalogues, GroupsIndexes, common portfolios and publication
artifacts were verified. The untranslated source names and source-opening links
remain visible beside translated prose.

`llm.Prepare` owns the reusable response-language prompt fragment independently
of DeepSeek. Execution, memo identity and the existing provider-sized preparation
checks all use that boundary; raw replay still sends saved bytes unchanged.
The default is English and the final translation cube supplies its target
language. This intentionally changes exact request and memo identities once.
Focused llm/deepseek/targetportfolio/documentationreduce/orientation/reading
checks pass after their presets account for the shared fragment.

`--lang ru` selects one physical `report.<repo>.ru.html`, using the selected
repository checkout's directory name. A Go module suffix such as `chi/v5`
does not produce `report.v5.ru.html`. Default English publication retains
`report.html`. Canonical `report.json` remains English.
A separately saved display translation retains its language, original text
catalogue hash and closed text refs; the manifest names that presentation and
the receipt carries it in memory. The server restores the same values when it
re-renders source links. UI translation is an embedded dictionary, not another
model call. `--no-model` must still make zero provider calls and uses only that
dictionary. Initial supported display languages are `en` and `ru`.
The question's eight fixed scope explanations also use this dictionary at
rendering time. Their canonical English route values remain unchanged and
never enter the model's display-translation catalogue. Saved rendering applies
the same vocabulary without analysis or provider access.

Translation reports each adaptive request-plan size and its closing count of
new logical model calls versus accepted cache hits. Before new calls it plans
at least eight windows (fewer when there are fewer texts), using the existing
four-worker pool and balancing complete entries by original UTF-8
text bytes. This is work distribution, with no row quota or smaller provider
envelope. Every child rebuilds its complete term dictionary. A validated cached
whole-window answer takes precedence, so an upgrade does not retranslate it.
Failed live calls count as new; HTTP retries remain separate exchange metrics.
A 1,585-text regression verifies eight initial windows, four requests starting before any response,
complete ordered output, protected spans, dictionaries and zero-call warm reuse.

Independent translation windows use the shared adaptive `Each` executor. One
failed request no longer cancels its running neighbours or restarts the complete
plan. All divisible failures in a round split together; successful windows
remain in memory even with the persistent cache disabled. Each child builds
only from its own complete original entries. Progress reports the unfinished
requests, and original exchange observers run once per executed request.
Extra translated-entry fields such as echoed `terms` are ignored. The required
`text` and original protected spans still validate; unused metadata neither
authorizes a changed translation nor triggers another provider call.

Ordinary fixture check `20260909-151910-python-tutorial-game-2e3397597390`
published both targets and 380 translated texts in eight live translation
requests; the slowest took 10 seconds and the full run took 40.6 seconds.
The immediate repeat `20260909-152101-python-tutorial-game-7b1d13ea15c5`
made zero live calls, reused all eight translation windows and published
identical saved translations in 6.9 seconds. Both target index/dependency/group
sets and the common graph, atlas, manifest and sole HTML were checked. Ordinary
cache clear removed an isolated copy of those eight exact exchanges and their
payloads. Concurrent regression tests additionally exercise two refused parents
alongside a slow successful sibling, all refusals split in one round, no repeated
successes with caching disabled, replay and cancellation. Independent operation
regressions retain good neighbours through row rejection, reordered memo reuse
and replay; their optional terms follow only the accepted rows.

The owner confirmed from their server logs on 2026-09-09 that a long generation
ends in HTTP 500 after four minutes; the response has no reliable timeout marker.
The four-minute local attempt deadline starts after the shared gate opens.
Expiry returns directly to the complete-entry splitter as `attempt_time_ms`,
without an identical retry.
The exact split memo includes the deadline and retains no failed display text.
Accepted siblings and children keep their usual cache entries; a timed-out
singleton remains an error. Parent cancellation stays terminal. A shorter
configured client timeout retains ordinary transport retries.

The server subsequently returned HTTP 500 before that local deadline. The owner
explicitly chose to split on HTTP 500 itself instead of estimating an earlier
deadline. Divisible translation windows now opt into `SplitHTTP500`: transport
returns the first 500 without an identical retry, then the shared adaptive owner
halves the complete input. It does not infer that every 500 proves a timeout.
The exact-request memo records `http_500`, distinct from a resource limit, and
is used only while the owning call keeps this opt-in. Accepted whole-parent
cache/replay takes precedence; NoCache bypasses the observation and cache clear
removes it. Singleton HTTP 500 responses retain ordinary retries. Other stages
and other HTTP statuses keep their existing policies. Virtual-clock tests cover
immediate 500, 500 at 3m45s, local expiry at 4m, accepted siblings, warm reuse,
cancellation, and ordinary singleton-500/429/503 retries.
Ordinary fixture acceptance on 2026-09-09 translated the same 364 display texts
in four live calls, with the slowest at 19 seconds (the preceding single call
took 61 seconds). Owner `20260909-085757-python-tutorial-game-ee546a49429f`
published both targets. The next owner `20260909-085902-python-tutorial-game-34b0b7ab268b`
made no live calls and retained identical saved translations. Graph, facts,
orientation and glossary matched the baseline. Full tests, vet and changed-package
race checks passed; cache clear succeeded on the isolated acceptance cache.
The private endpoint's four-minute behavior is covered by the local transport
test, not established by this successful online run.

The shared adaptive executor persists an actual context/output/response
resource-refusal observation for an exact request that its owner can divide.
Previously each run retried that oversized parent even when all child responses
were already cached. The memo includes exact provider bytes, canonical provider
state and all envelope limits in its identity; its value contains only version
and the refusal kind. Current stage code always rebuilds the full child plan and
each child still needs its own valid answer. Accepted whole-parent cache/replay takes precedence. NoCache
bypasses the memo and ordinary cache clear removes it. This is separate from
Learn's existing positive partition memo and does not alter that stage.
The owning Translate regression processes all 1,310 original texts in two
complete children after one real-shaped fake-provider resource refusal; a new
executor/provider over the same cache makes zero calls and returns an identical
translation. Nested shared-executor tests cover all three resource kinds,
ordering, absent child answers, changed limits, replay, NoCache and cache clear.
There is no migration of earlier journals: an old oversized request can still
need one refusal after upgrading before this observation exists.

The owner's 2026-09-09 request for automatic halving also applies to refused
translation responses: translation alone opts into splitting a whole window
after JSON or translation validation fails. Such an observation is recorded as
`response_validation`, never as a provider token limit. The original refused
response remains refused and uncached; none of its fragments supply display
text. Every smaller request uses the same original entries and must pass the
unchanged owning validation. Halving stops at one entry, whose actual error
remains terminal. Successful windows and exact split observations survive a
new run through the same cache. Other stages keep their existing refusal rules;
apart from the explicit translation attempt deadline and HTTP 500 policy, transport, configuration,
cancellation and persistence failures do not gain this behavior. Progress output
explicitly announces automatic continuation.

Real browser checks found and corrected a filtered-search empty state that hid
matches in other categories, indistinguishable operation choices with the same
title, and a selected term/header that scrolled out of the inspector. Search
now offers the other matches while preserving query and component; duplicate
operations retain their distinct exact source anchors in the menu; the selected
header and term picker remain above the panel's scrolling body. Role controls
now really filter existing components, with All components as the default and
the selection retained in the URL. Ordinary browser checks on `20260907-211253`
and `20260907-211622` covered library/tool filtering, component navigation,
Back/Home, Learn/Work, reload and returning from a real GitHub source link.
The three existing reading entrances now appear immediately after the summary;
the fixed-height repository description sits below its canvas without moving
nodes during hover or expansion. Disconnected components now use a compact
width-aware grid. Connected maps pack their actual connected components while
retaining every real edge, direction, label and operation reference. Native
component names replace the deterministic root-directory label; language,
application/library/tool role and path remain distinct. Real native or model
names that happen to say Root are not rewritten.

The run entrance uses the existing learning intent: one matching question opens
its answer, several open their topic menu. Long question links fill their whole
list row, removing the non-clickable gap between wrapped lines. Browser checks
on chi and the Python/TypeScript fixture followed questions through a term,
component, operation, exact GitHub source and return to Learn/Work, preserving
URL context without runtime errors. The fixture's real frontend/backend edge
remains; the current chi overview has no confirmed cross-component edge, while
a saved connected chi case checks placement without manufacturing a connection.
The owner checklist is `work/repomap-review-current.md`, retaining all 38 prior
UX IDs and separate Learn/Work importance review. Nested-cube hover previews,
additional arrow treatments and the remaining visual alternatives stay visible
for owner review; this acceptance does not declare every generated answer or
possible presentation choice final.

## Source links and report server

`--no-serve` requires resolvable GitHub or GitLab source links and fails in
preflight with corrective flag guidance otherwise. A served report may add
only manifest-authorized local editor opening, using the repository's
.repomap.conf (VS Code by default). It does not expose browser APIs
for workspace reads, symbol lookup, source context, analysis control, or run
selection.

The local server validates manifest identity and every requested source path.
Static reports use hosted source URLs derived from the same exact locations.
No successful report contains deliberately inert source links.

## Model execution contract

Transport retry progress is independent of wait-heartbeat throttling. Each
retryable failure immediately prints its request digest, attempt number, closed
reason or HTTP status and planned minimum delay. A second event marks the actual
retry start after the shared gate permits it. Both use the ordinary run clock;
request, response and credential contents never enter these messages.

The owner's 2026-09-07 transport rule requires at least one minute before a
retry after HTTP 429. The provider now uses that floor and honors a longer
Retry-After delay in seconds or HTTP-date form. Its run-shared attempt gate
pauses new calls from sibling batches for the same interval and collapses to
one concurrent attempt; later rate limits may extend but never shorten the
pause. In-flight requests may finish and cancellation interrupts waiting.
Other retryable failures keep the existing short exponential backoff. This
transport-only change preserves prepared bytes, memo identities and the
three-retry allowance. Virtual-clock tests cover the floor, both header forms,
exhaustion, cancellation, and cooldown sharing/extension across batches.
A three-request simultaneous 429 burst with Retry-After values of 30, 90 and
120 seconds waits for the longest shared deadline, then retries all three
serially. That scenario passed ten repetitions under the race detector.

The owner subsequently supplied a compatible server's actual 429 message:
`rate limit exceeded: retry after 9.636307001s, reset after 45.636307001s`.
The adapter now reads both labelled relative durations from a plain error body
or its JSON `message`, string `error`, or `error.message`. The longest valid
body hint and Retry-After header feed that same minute-minimum shared cooldown.
The supplied example still waits one minute; a longer reset can extend it.
Negative, unitless or invalid values are ignored, the original error bytes
remain available unchanged, and non-429 responses keep their existing backoff.
The owner's reported 3,000 requests/minute, 300,000 tokens/minute and million-token
context describe their custom endpoint, not global product defaults. The current
gate controls concurrent attempts and server-requested cooldowns; it does not
preallocate a rolling token quota or equate context capacity with output length.
Focused DeepSeek, LLM and run tests, provider/LLM vet and the owner-facing build
passed. Ten race-detector repetitions of three simultaneous 429 responses
confirmed serial retries after the longest mixed header/body wait of 120.25s.
These are local HTTP-contract checks, not a live check of the owner's endpoint.

Each model-assisted stage owns its state, complete input authority,
provider-sized request preparation, prompt, response schema, restoration, and
semantic validation. Static prompt prose lives in readable Markdown beside
the owning stage and is compiled with `go:embed`. Go owns dynamic reservoirs
and deterministic partitions; the provider layer does not own domain prompts
or schemas.

The shared LLM executor owns exact provider requests, transport, retries,
provider-envelope and JSON decoding, adaptive batch execution, cache,
accounting, and journal events. Current stages share a 32 MiB request envelope,
a 16 MiB decoded-response ceiling, and request up to 128,000 output tokens;
the configured provider ceiling remains authoritative when lower.

The owner's 2026-09-09 syntax rule permits balancing JSON brackets before the
ordinary decoder: outside quoted strings, a closing bracket with no matching
open bracket anywhere in the active stack becomes whitespace; missing closing
brackets are appended at EOF. Whitespace prevents separate tokens from merging
(`1}2` must not become `12`). An unfinished quoted string may receive its
closing quote at EOF before those brackets, preserving its original contents,
including trailing spaces and literal bracket characters. An unfinished escape,
invalid string character, missing value, crossed nesting, multiple roots or
trailing prose is still refused; interior quotes and commas are never inserted.
The same rule applies
inside the existing JSON fence and after the existing complete thinking block.
The entire resulting object or array must pass JSON decoding and the owning
stage's unchanged completeness, schema and closed-ref validation. This does not
override provider-reported truncation or resource limits. Raw responses in
cache and journals remain original; normalization needs no model call and does
not change request identity.

Complete reservoirs are processed through as many deterministic disjoint
batches and convergent closed-ref merge rounds as needed. Composite input is
exhaustively repartitioned when a prepared request does not fit. An indivisible
oversize item or real provider response/output envelope failure is terminal
unless the owning stage defines a lossless adaptive split. It never authorizes
truncation or partial publication.

Independent planned items execute through a bounded worker pool of four. The
caller's item index is the only in-memory result slot and observer events replay
in that order. Every provider attempt acquires the run-shared adaptive gate. An
HTTP 429 collapses the gate to one before existing backoff/retry, so started
attempts finish while that retry and all new attempts become serial. A terminal
item error cancels queued siblings; the owning stage rejects the complete
batch. Already accepted sibling calls may retain exact identity-bound cache
entries but never become a partial semantic result.

A stage returns a fully validated result, a contractually legitimate empty
result, or an error. Orchestration, reporting, and the browser cannot provide
semantic fallback, repair, promotion, or partial success.

## Request-local refs and validation

Models select only closed request-local short refs. Provider catalogs may show
exact repository-relative paths, file names, symbol names and signatures,
dependency names, neutral pattern shapes, and reconstructed values when that
context has semantic value. The model never supplies canonical identities.

Unknown refs have no authority and are discarded rather than guessed,
repaired, or clarified. Set-valued outputs are filtered to advertised refs and
deduplicated locally; a model is not required to echo each selected ref exactly
once. Unknown keyed rows are likewise discarded. If filtering leaves a
mandatory scalar or complete required assignment unresolved, the stage rejects
the result instead of inventing a replacement.

Persistent caches remain on the ordinary path. A transport response is bound to
exact prepared request bytes and provider transport identity (endpoint/auth
mode), then decoded and validated by its current owning stage before use.
Effective model/options are already in the request bytes; local schema/state
changes alone do not alter what the provider was asked. Entity reuse separately
includes the table contract and exact single-row preparation. `--no-cache` is the
explicit live-provider bypass. Debug artifacts never contain API keys or
Authorization headers.

## Development contracts

Focused discovery and ProgramIndex regressions use exactly one cumulative real
repository fixture per active language under `testdata/repositories/<language>`.
New scenarios extend that fixture and its exact tracked-file inventory.
Deterministic adapter stages run for real; provider stages use exact
request-bound, fail-closed local presets with no network. Fixture success is
focused evidence, not product acceptance.

The acceptance fixture `testdata/acceptance/python-tutorial-game` is a copy of
that repository at revision `78714d34ee` with its `expected.json` and the
sealed ProgramIndex and dependency artifacts of one real run. A focused test
rebuilds the fact layer from those inputs and asserts every expected row with
its anchor. It is the constitution's acceptance list; a real online run and a
browser read of the published page remain the product acceptance.

`testdata/contracts` inventories every static prompt and its owning
`go:embed`, every production hard-bound symbol, and every product-root Go or
repository-owned JavaScript/TypeScript test with non-ordinary requirements.
The inventory and its contract test change together. These inventories do not
create another product path.

Canonical `make test` and `make vet` use ambient Go build and module caches,
cap package-level parallelism at four, and limit each test binary to five
minutes. Focused commands keep the same or tighter bounds. Owner-facing builds
use `make build` and produce `.bin/repomap`.

## Acceptance

A product change is accepted only when:

1. `make build` produces `.bin/repomap`;
2. that binary completes a normal online provider run on a real repository;
3. the process exit status is verified;
4. repository guidance, `reduced-documentation.json`, every enriched
   ProgramIndex, `program-index-set.json`, every matched `groups-index.json`,
   both target/page portfolios, the common manifest and report JSON, and
   the owner report HTML are inspected directly;
5. a multi-target run preserves every successful target in the shared graph
   and has exactly one report JSON and one physical report HTML;
6. focused tests and vet for changed packages pass.

For cache changes, acceptance also verifies a real second run and
`repomap cache clear`. Offline runs, fixtures, model-response replay, and
helper probes do not replace ordinary online acceptance.

Contributor-only browser QA serves the narrow owner run root and its backing
sibling directories through a temporary loopback-only HTTP server, then opens
the owner report URL. This is an inspection procedure, not a product command
or checked-in sidecar.
