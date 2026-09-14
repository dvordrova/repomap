# Implementation and acceptance journal

## 2026-09-14 — Compare boundary prompts and preserve free-choice formatting

- Replayed ten prompt/request variants on four complete saved Airflow windows
  through official `deepseek-v4-flash`, with thinking explicitly disabled.
  Additional windows, combined variants and three fresh repeats brought the
  comparison to 106 live replay requests. No source evidence was sampled or
  shortened; all requests and responses use the existing system cache. Qwen
  on the owner's other machine was not tested locally.
- The selected prompt is 70 lines / 593 words instead of 155 / 1,438. It puts
  `decision` and `basis` in separate table columns and distinguishes source
  indexing from communication with another process. Moving selected rows before
  their complete shared context was necessary in this comparison; shortening
  the prompt alone did not fix the SQLite examples. The response schema,
  destination catalogue and reasoning setting are unchanged.
- Comparing the selected ordering on 15 complete windows / 19 rows removed
  eight false external relationships involving SQLite or an in-process HTTP
  transport. Actual HTTP and SMTP relationships survived. One SQLite row and
  one unsupported PostgreSQL destination remain semantic errors in this
  diagnostic set, not schema refusals. This selected set is not an estimate of
  accuracy across repositories. Three fresh repeats of the four seed windows
  preserved the selected variant's decisions.
- One minimal-prompt response wrote `other:Airflow executor`. The old shared
  decoder refused the missing space. Free-choice tags now accept whitespace
  around the colon while preserving the model's written value; unknown tags
  and empty values remain unresolved. All 106 saved responses pass the owning
  decoder after this normalization, including that semantically unsupported
  executor relationship. Syntax acceptance and interpretation quality are
  recorded separately; no decision or destination was inferred by the decoder.
- The free-choice regression failed before the change and passes afterward.
  Focused lines, table and reading tests/vet and `make build` pass. Ordinary
  online Python acceptance at `78714d34ee` completed with exit 0 in 287.7s:
  both targets, 16 answers, one common manifest/report JSON/HTML and the full
  artifact chain. Both live boundary requests were accepted. Browser inspection
  followed Backend API → POST `/api/level/run` → the exact frontend source
  and caller, then Back. The older Airflow process remains in progress and
  predates these changes.

## 2026-09-14 — Audit recent refusals and remove design-only strictness

- Inspected 23 `rejected.jsonl` files from September 13–14 under the existing
  system run root and task run roots, plus their saved responses and stage
  accounting. Their 142 diagnostic records are not 142 failed model calls:
  cached responses recur and omission summaries contain multiple items.
- The removed area minimum produced 62 group refusals across 12 runs. In the
  latest complete Python report it removed 7 of 13 proposed areas; in Othello,
  2 of 8. Revalidating all six complete saved design windows with the current
  decoder accepts all 21 areas, recovering those nine without provider calls.
- Another 13 `group_rejected` records were merely omitted declaration refs,
  allowed by the prompt. Design diagnostics now distinguish `ungrouped_input`,
  `unknown_member` and `group_rejected`; the stage counts affected windows once.
  Removed two further new restrictions: explicit empty groups are accepted in
  every mode, and caption whitespace is normalized like the preexisting table
  decoder. Empty required fields, malformed rows, groups without known members,
  and conflicting known ownership remain refused; independent groups survive.
- Compared production refusal additions against `1ecad4db`. The remaining
  additions bind native Clojure/project inputs, JS/TS value-read refs and source
  locations, structural-edge locations, and unique declaration membership.
  No new boundary-choice validator or boundary prompt was introduced. The
  reported `remote_client_instance` decision failure was not present in these
  local journals; this does not establish what the other machine was sent.
- Existing glossary diagnostics dominate item counts. The last Othello glossary
  response has 684 entries: 639 identifiers, 44 domain terms and one malformed
  row. It omits 482 valid identifiers and refuses 158 terms without an exact
  supported occurrence (157 identifiers and one capitalization mismatch), plus
  the malformed row. There is also one earlier whole glossary output-limit
  refusal. These predate this change; their diagnostics are not evidence of
  hundreds of failed primary-analysis calls. The old documentation reducer
  also drops concepts after twelve per document (79 counted across repeated
  runs); its rule is unchanged here.
- The in-progress Airflow run has one malformed JSON response among 863 symbol
  windows: `r25` lacks its closing quote, losing 30 of 4,090 descriptions.
  All 137 operation windows were accepted. Final Airflow and browser acceptance
  remain pending; the running process predates these design fixes.
- Regressions reproduced empty-response, wrapped-caption and diagnostic-count
  failures before the change. Reading, GroupsIndex and report tests, reading
  vet, saved-response revalidation and `make build` pass. No cache was added or
  cleared, and no model answer was rewritten.

## 2026-09-14 — Remove the new two-part area rejection

- The owner reported `atlas_zones: an area must contain distinct collaborating
  parts` on another computer. The newly added design decoder was imposing a
  two-member minimum, not checking collaboration evidence. Removed only that
  new restriction; title/shape, known-reference and conflicting-membership
  validation remain intact.
- A single known part now survives area decoding and the ordinary reader.
  Repeated refs do not add members, unknown refs are still discarded, and
  singleton areas participate in the same conflict check as larger areas.
  Regression cases reproduced both the refusal and the missing saved area
  before the fix. The prompt leaves useful area boundaries to the model.
- Reading, GroupsIndex and report tests pass; reading vet and the owner binary
  build pass. The Airflow online run remains in progress.

## 2026-09-14 — Airflow exposed annotated underscore parser failure

- The ordinary Apache Airflow run at revision
  `1f5d7691663016c898718768991a2dae522b4475` selected 453 targets. Several
  Python preparations failed before publication: `_remove_kwargs(_: object)`
  in `airflow-core/src/airflow/api_fastapi/core_api/datamodels/trigger.py:27`
  exposed a declaration skipped as `_`, then accessed by its annotation.
- The cumulative Python fixture reproduced the numeric KeyError before the
  fix. Underscore now retains its ordinary Python declaration; annotated
  parameters and call results keep possible methods, while untyped parameters
  remain unresolved. No Airflow-specific filter or invented receiver was added.
- The complete Python parser suite, cumulative Python contract, corresponding
  TypeScript/JavaScript parameter cases, Clojure local callback and Go blank
  identifier checks pass; changed packages pass vet and the binary is rebuilt.
- The interrupted run exited 130 and is not publication acceptance. Its nine
  cache entries were moved into the existing default system response cache;
  the old cache directory is only a link for saved diagnostic references.
  The restarted full run used the default cache and official DeepSeek. It
  parsed Airflow core successfully (105,059 objects and 354,875 relations).
  The owner then explicitly narrowed acceptance to the Airflow CLI/core,
  browser UI and Task SDK library. The full run was stopped with exit 130;
  the ordinary three-target run is in progress, not yet browser acceptance.
- Before the requested push, canonical `make test`, `make vet`, `make ui-test`
  and `git diff --check` pass. Native Clojure `.cpcache` directories are ignored
  alongside other generated build outputs.

## 2026-09-14 — Visible zoom entrances on closed frames

- Replaced the faint expand corners with a small magnifier-plus control on
  every closed component, area and external collection. It follows the actual
  frame's top-right corner, not the limited text column, and retains a 28px
  hit area and 18px symbol while zooming. Open frames and leaves have no icon.
- Area summaries reserve title space beside the icon. Narrow external titles
  wrap instead of losing the destination suffix to an ellipsis.
- Browser inspection confirmed 12px corner insets and a 28px control on all
  three root summaries. The front icon opens its contents; the Application
  shell and navigation icon opens the group's parts and reading panel. Back
  restores the identical component camera. Existing smooth focus is reused.
- Saved Python rendering makes zero provider requests. All 23 web tests,
  generated-asset check, report tests/vet and owner binary build pass.

## 2026-09-14 — Group names in component summaries

- Distant component cards list every saved area name instead of a bare group
  count. Each name opens that exact area through the existing smooth focus and
  history path. Parts and inputs remain a secondary count when space permits.
- Text measurement reserves the group list before optional role, counts and
  purpose; short cards omit prose rather than cutting off a line. Dense lists
  retain all names in a scrollable region. World coordinates stay unchanged.
- Saved Python rendering used zero provider requests. At the normal 934×992
  browser size, all four front and both backend names fit without scrolling;
  one zoom step also retained both complete lists. Canvas rendering opened its
  three parts and reading panel; Back restored the identical camera transform.
- All 23 web tests, generated-asset check, report tests/vet and make build pass.

## 2026-09-14 — Final small-repository navigation audit

- Questions remain one vertical list of open topic headings and all their
  questions. Python question 3 opens directly; Back restores the list. There
  is no duplicate question grid or nested topic chooser.
- A 934 px browser exposed narrow root frames and a clipped first child on
  component entrance. Root summary width is now reserved after the actual
  area-summary heights are placed. Layout selection considers both root
  dimensions; short summaries retain a three-line purpose. Entering front
  focuses its first content at readable scale, with the component title kept
  in the visible horizontal part of its frame. Pan/zoom never rearranges nodes
  or routes. Browser All → front → Back restores identical viewport text.
- Home/All clears the visible Find query after recording the new visit. Empty
  group readings now say no connections to other parts were found. Python's
  remaining isolated Import retry, Sleep utility and Instruction labels have
  no observed external consumers; the retry self-call is internal. We do not
  invent links or infer dead code from an absent mapped relation. Browser
  Sleep utility exposes both the missing connection and missing input path.
- The ordinary online Othello audit completed in 325.546 s. The final owner
  binary completed its ordinary repeat with exit 0 in 5.527 s, reusing exact
  requests: `/private/tmp/repomap-goal-audit-20260914/20260913-223634-othello-2a9cc894b339`.
  Its complete manifest/report, native, places, atlas and GroupsIndex chain
  exists; the graph digest matches the first audit. It has 25 parts (24 core),
  8 inputs and 16 questions. Native declarations include 173 functions and 32
  vars, with no nominal types: board.cljc:27 returns a vector; game.cljc:5
  returns a map. These values are explained by Board model/Game state, not
  synthesized as type declarations. External symbols/imports are not runtime
  communication records, so External communication is absent from its legend.
- Othello browser: Board model is purple, has exact code and incoming sources;
  Handle board click selects its six reached parts. Python's previously
  inspected Run → backend → Robot write paths, GET exclusion and exact external
  POST caller remain unchanged. No native or model topology changed in this
  layout pass. The original audit records seven refusals (documentation,
  single-part areas, an unsupported orientation ref and optional glossary).
- All 23 real UI tests, generated-asset check, report tests/vet and make build
  pass. Python and Othello previews use ordinary saved rendering with zero
  provider requests. The question list remains open for the owner.

## 2026-09-14 — JSX and value uses retain their original connections

- JS/TS now publishes compiler-bound value and JSX tag references as original
  reads. Aliases, reexports, shorthand values and local shadowing retain their
  declaration identity. Pure writes, type-only syntax, unknown bindings and
  unindexed callback bodies do not borrow a runtime reader. JSX does not
  fabricate a synchronous call. Property and receiver uses sharing a source
  column retain separate relations.
- Real native testing exposed an older per-file rank cutoff: module activity
  and values after ten other declarations disappeared, and later types could
  not receive descriptions. Removed that cutoff; ranking only orders eligible
  authored declarations. The saved complete Python graph adds nine original
  declarations and admits seven existing types, losing none. These include
  robot direction values, drawing coordinates, Step, Wall and RunResult.
- Correction to the preceding native-test notes: the default Node 24 lacks a
  TypeScript compiler, so JSTS tests had silently skipped. The actual suite now
  runs with the prepared Node 21 compiler. The cumulative mixed fixture enables
  JavaScript explicitly, and its iteration check correctly expects possible
  JS dispatch. Full JSTS, places, contract, GroupsIndex, reading, report, Python
  and Clojure checks pass; native value-read/shadowing equivalents were also
  run explicitly for Python and Clojure. Focused vet and make build pass.
  Native TypeScript 7 is not prepared; general Go variable reads still have
  no producer equivalent. No fallback relationship is invented for either.
- Final ordinary online run exits 0 in 27s:
  `/private/tmp/repomap-iteration-20260914/20260913-220335-python-tutorial-game-0c7c8ba4ff97`.
  Both target chains are complete: Python 219 objects/738 relations, frontend
  259 objects/730 relations. The common GroupsIndex report has 27 parts, all
  16 questions answered, three original outgoing HTTP observations, one owner
  manifest/report JSON/physical HTML, and the complete saved reading chain.
  Seven design and one glossary refusals remain explicit. Exact accepted
  requests were reused; this was an ordinary run, not replay.
- Browser: Find Editor opens Code editor and its Playground JSX use at
  playground.tsx:110 → editor.tsx:19. The peer visit and Back retain the camera
  and expanded source evidence. Find fieldColor exposes draw.ts:117 →
  constants.ts:1, plus its reaching animation/resize inputs. The final report
  preserves the question → Run → backend/robot-write reading → Back journey.
  Its question menu is eight open topic headings with sixteen visible links;
  no question grid or collapsed topic choices. Updated python.html through
  saved render with zero provider requests and left Questions open.

## 2026-09-14 — Module activity and internal code connections stay readable

- Observed module-body calls, executions, external invocations, reads and
  writes now give the normal architecture request a source-located module
  declaration choice. Import/containment-only modules do not. Selecting that
  body does not assign other declarations in its file; their accepted choices
  and refusals remain independent. The prompt explains the distinction.
- Real cumulative Python module registration, JS/TS module calls, Clojure
  namespace evaluation and Go's callable main equivalent pass through native
  evidence and places. Places, GroupsIndex, reading, contract, JSTS, Clojure
  and report tests pass; focused vet and the owner build pass.
- The ordinary online two-target run finished with exit 0 in 4m5s:
  `/private/tmp/repomap-iteration-20260914/20260913-211836-python-tutorial-game-af4f98e3c6f9`.
  Both targets are analyzed and retain their complete native/GroupsIndex
  chains; the common atlas, report JSON, manifest and one physical HTML exist.
  All 16 questions have answers and all three integrations survive. Sixteen
  design refusals retain thirteen single-part-area decisions and three
  unassigned styled declarations; no local membership is invented for them.
- Browser: Server launch exposes the four original settings reads at
  main.py:17–20 and settings.py:15. Visiting Application configuration and Back
  restores the identical camera and expanded evidence. The normal model puts
  index.tsx and reportWebVitals together in Application bootstrap.
- That last journey exposed a presentation omission: same-part relations were
  kept in GroupsIndex but absent from the source reading. The ordinary group
  card and sidebar now expose those exact relations under Connections within
  this part. No self-arrow or new relation is added. Browser Find reportWebVitals
  now opens the original index.tsx:19 → reportWebVitals.ts:3 call, with both
  revision links. A report test preserves same-line occurrences, resolution,
  original sites and static reading, excluding containment and other members.
  Long source links wrap within the sidebar.
- The owner's question-list request remains one vertical series of open topic
  headings and visible questions. Consolidated its styles in the question
  stylesheet; Python question 7 opens with one click and Back returns to the
  same list. No duplicate question grid or topic disclosure remains there.
- Remaining native gaps: JSX component uses and general JS/TS variable reads
  still do not supply their ordinary dependency relations. Unused helpers and
  refused memberships remain distinct from those missing observations.

## 2026-09-14 — Typed iteration restores the normal robot step

- Python lost the element receiver in `for robot in sorted(self.robots)`, so
  the report showed rollback writes but omitted normal movement. Direct known
  collection annotations now retain a possible element class inside a
  synchronous loop. Calls and field writes preserve their native source sites.
  Unknown/replaced collections and receivers, shadowed sorted, heterogeneous
  tuples, unions, asynchronous iteration and post-loop uses stay unresolved.
- Cumulative Python native tests, real Go range and TS/JS for-of equivalents,
  full contract/JSTS packages and focused vet pass. Clojure's corresponding
  Java instance dispatch has no current native equivalent. The owner binary
  completed the ordinary online two-target Python run with exit 0 in 4m29s:
  `/private/tmp/repomap-iteration-20260914/20260913-205812-python-tutorial-game-5838b558b297`.
  Both native/GroupsIndex chains and the common atlas, manifest/report JSON
  and one physical HTML exist. Three integrations survived. Seven design
  refusals concern single-part areas/missing membership; an identifier-only
  glossary entry was also refused. These do not invent replacement membership.
- Browser: Robot movement lists forward prev_x/prev_y/x/y writes at robot.py
  40–43 for backend POST and front Run. The x path opens as handleClick →
  runLevel → possible HTTP integration → run_level → Field.make_step →
  Robot.make_step, with original links. GET /api/levels reaches HTTP endpoints
  and Level data, with no Robot writes. The exact external POST identifies
  handleClick at playground.tsx:72 as its caller and Run as its reaching input.
  Back restores the identical Run camera and reading.
- That journey exposed a separate focus bug: a hidden child's world bounds
  fitted the overview, so search opened its sidebar without revealing it.
  Camera preservation now requires an actually visible, legible destination.
  Browser Find Robot movement now opens the detailed purple card; all 22 UI
  tests, report tests/vet, bundle and owner build pass. Saved render makes zero
  provider calls.
- Remaining isolation audit: native module-level calls/reads can have no
  selected architectural member (index.tsx → reportWebVitals; main.py →
  settings). JSX component uses and general JS/TS variable reads have additional
  native gaps. These are distinct from unused helpers; no missing edge is
  inferred from a shared path or group name. The broader isolation work remains
  open.

## 2026-09-14 — One question list, header navigation and whole-map entrance

- Questions are one vertical list: saved topic headings and immediately visible
  questions, with a fallback for answers absent from the topic plan. Removed
  the duplicate grid, nested topic choices and input catalogue All disclosures.
- Header: reviewed repository link and house icon on the left, repomap/GitHub
  link on the right, one Find field and a component list. All shares the
  whole-map action. Nested repository links retain their directory and revision.
- Removed changing hover prose above the canvas and the large Zoom in buttons.
  A stable zoom/drag hint and small expand marks expose the action. Input
  context and close-details chevron live in the reading card. Search excerpts
  are short plain text, without model/source popovers; exact code names rank
  before matches in answer bodies.
- Whole-map entrance fits every root frame and refits its available space;
  manual cameras restore exactly. Short-window checking exposed the old .15
  lower zoom clamp: the minimum now permits the actual root bounds. The fixed
  world reserves external-summary space and chooses an orientation that also
  accounts for readable root headers. No layout runs during wheel zoom.
- Outbound reading exposes native callers of the exact sending callable,
  preserving original parts, possible-call status and source pairs. It does
  not attribute every method in a shared client group to the same caller.
- Saved Python browser journeys: grouped questions → question 7 → Field
  simulation → Back restores the answer; front → All → Back restores the exact
  camera; Find config returns Config first, a map visit returns to the same six
  compact results; POST → Playground orchestration → Back restores its source
  reading. Checked ordinary 934×992 and temporary 1280×720 browser layouts.
  Othello also opens its component from the list and exposes all question topics.
- Report tests/vet, 21 frontend tests, owner build and saved rendering pass.
  UI rendering used existing accepted report/translations with zero provider
  calls; the ordinary online variable-read run is recorded below. Broad
  pointer-hover journey acceptance is not claimed from click-only browser QA.

## 2026-09-13 — Connection reading, core colour and source-backed data reads

- Core uses its saved lane for purple cards and overview rows; both legends
  include only present categories. Reading and hover use a single contour.
  Removed the outline of the whole scrolling inspector and renamed Key code
  to Code in this part. Outgoing connections precede incoming connections and
  code; native relations remain inspectable without a model sentence.
- Browser on the saved Python report: Field simulation exposes all four
  Robot movement calls with exact sources. Peer navigation and Back restored
  the identical viewport and expanded evidence. Core fills are purple; node
  outlines/shadows and the inspector outline are absent.
- Python now emits declared-variable reads with lexical shadowing, import and
  receiver controls. Input paths include terminal data reads with their native
  site; reading a value, callback or class declaration does not activate effects.
  Cumulative Python + GroupsIndex checks and a native Clojure var-read
  equivalent pass. General Go/JS variable reads remain unavailable; JS/TS's
  existing type/contract reads are narrower evidence.
- Ordinary online owner-binary run exited 0 with both targets and three
  integrations: `/private/tmp/repomap-variable-reads/20260913-193510-python-tutorial-game-727901bff0c4`.
  Both target chains exist with one common manifest/report JSON/physical HTML.
  Reads bind all three backend requests to levels and POST to the source-size
  constant. Browser: GET /api/levels → Level data exposes get_levels_info →
  possible read levels, its declaration and the original read at app.py:21.
- Removed the introductory model-source popover; citations and attribution
  remain below the canvas through Repository summary sources. Othello's
  expanded legend contains no external category.
- Othello core investigation: its accepted Board representation and Game state
  groups exist, but the type-reading stage describes nominal declarations.
  Functional vectors/maps returned by functions supply no nominal type card.
  Recorded this general limitation; no Othello-specific inferred entity or
  mutation was added.
- Focused report/Python/Clojure/contract tests, scoped vet, 17 frontend tests,
  owner build and saved rendering passed. Multi-component top-level zoom and
  general functional-value entity interpretation remain open.

## 2026-09-13 — Reuse external catalogue groups on the canvas

- The system map now consumes the same outbound display grouping as the
  external catalogue. Each destination has one frame and keeps every original
  selectable call, source and input path. The frame is a display collection,
  not a newly inferred component or target.
- Matched peer links bind by integration identity and the exact caller and
  call location, including column. Duplicate structural/path projections lead
  to the same backend input; matching labels never establish an integration.
- Saved Python browser journey: backend API contains all three HTTP calls;
  POST opens the backend POST input, and Back restores the exact call camera.
  Parameterized GET retains its native method/path. The frame sidebar lists
  every record and its originating inputs; empty code/explanation actions
  were removed.

## 2026-09-13 — Python field writes and input evidence

- Source-ordered Python receiver origins resolve attribute writes to existing
  declared fields as alternatives, retaining exact write locations through
  GroupsIndex. Aliases, declared receivers and constructor results are covered;
  rebound, unknown, nested and dynamic writes stay unresolved.
- Input reading lists reached writes with their entity, field, source site
  and shortest call witness. Entity/part reading links back to those inputs.
  Exact matched peer inputs carry the originating frontend witness and its
  uncertainty; unrelated sibling inputs do not inherit effects.
- Cumulative Python fixture: 14 distinct write observations, nine candidate
  bindings, negative ownership/reassignment controls. No equivalent bound
  field-write emission exists in the current Go, JS/TS or Clojure adapters;
  those remain missing, not inferred from part reachability.
- Ordinary online acceptance, owner binary: `python-tutorial-game` completed
  with exit 0, two analyzed targets and three cross-target integrations.
  Run: `/private/tmp/repomap-entity-writes/20260913-190514-python-tutorial-game-40fdbc785aac`.
  Both native/program/group chains exist, with one common manifest, report JSON
  and physical HTML. All 13 bound writes retain their source locations.
- Browser: backend POST and frontend Run simulation expose the same eight
  reachable writes; Robot reading lists both inputs and only its three writes.
  Expanded frontend witness reaches `robot.py:46` through the HTTP call,
  possible integration and backend call chain. This does not establish that
  every write executes on every run. Functional Clojure value entities remain
  unresolved acceptance work.
- Focused report, Python, GroupsIndex and contract suites, 17 frontend tests,
  scoped vet and the owner binary build passed.

## 2026-09-13 — Align the report shell with the canvas

- Removed the independent centred widths of the sticky toolbar and question
  reading. Header, toolbar, canvas and answers now share the page gutter;
  prose retains its reading measure. Removed the redundant canvas-only width
  override.
- Python browser check at 2250 CSS px: toolbar moved from x=405 and questions
  from x=581 to the canvas gutter x=20; no horizontal document overflow.
  At the ordinary 934 px viewport, questions → answer → Back → Back to map
  kept one mounted canvas and placed the destination below the toolbar.
- Report tests/vet and `make build` passed. Both saved Python and Othello
  reports rendered successfully with zero provider requests.

## 2026-09-13 — Fixed zoom geography and repository reading entrance

- Automatic detail now changes contents inside fixed area rectangles. The
  second overview graph/layout was removed. Original directed routes are
  clipped only at closed area boundaries and continue to their actual parts
  on zooming in. Entry/exit scales differ to avoid boundary flicker.
- Live checks exposed and fixed swapped asymmetric minimum dimensions in ELK's
  vertical layout, initial cameras over empty frame corners, and diagonal
  clipping caused by tiny fractional differences on vertical segments.
- An unselected sidebar now links to questions, run material, terms, missing
  observations and author claims, with component purposes and original sources.
  Long reading stays below the mounted canvas. Closing details restores this
  entrance at its top instead of leaving a mostly empty column.
- Othello: View model → zoom out through automatic summary → retained all world
  coordinates and selected reading; no diagonal routes after the fix. All
  summary content fits. Run topic → desktop answer with 20 source links → Back
  twice restored the exact camera and home reading.
- Python/TS: external POST → Run → Source code validation retained the four
  source steps and their uncertainty. Back twice restored the exact external
  camera and reading. No summary overflow; present-type legend unchanged.
- This is small-repository browser evidence, not acceptance of the remaining
  multi-component top level or value-entity mutation analysis. No new provider
  requests were made. CURRENT was not read or changed.
- Final checks: 17 frontend tests, embedded-asset consistency, report tests,
  vet and owner binary build passed. Both saved renders exited successfully;
  final Othello browser had no console warnings/errors or summary overflow,
  and every home-reading link resolved to its existing report section.

## 2026-09-13 — Cross-component input paths and reverse reading

- The common map now continues an input through exact matched peer inputs,
  keeping original edge authority, cycle protection and source witnesses.
  A reached part never activates its other operations.
- Outbound records list inputs that reach their original caller subject;
  sharing a part, importing or reading the callable is insufficient.
- Part/outbound sidebars show grouped reverse input links. Redundant related
  operation and raw evidence disclosures were removed from these cards;
  the saved call-path explanation stays visible. The toolbar drops the
  type/style switches, and the legend contains only present categories.
- Clojure investigation: saved Othello has 173 functions, 32 variables and no
  type declarations. Board is a vector and Game a map returned by functions.
  The existing native-type concept stage cannot name those value entities.
  Python records unresolved attribute writes without target objects; these
  reports have no resolved write edges. Reaching a part must not claim mutation.
- Saved online Python/TS render: Run reaches 10 participants including the
  backend validation/simulation and its POST communication. The validator's
  four-step witness starts at PlayGround.handleClick, continues through
  runLevel and a possible integration to run_level, then possible validate.
  External POST lists only Run; external → Run → backend → Back twice restores
  exactly translate(-1410.5px,-1214.5px) scale(1) and the external reading.
  Othello shows Parts/Inputs only. Automatic semantic zoom and value-entity
  effects remain unaccepted; this probe does not close the full goal.
- Validation: focused report tests and vet, 12 frontend tests with embedded
  asset consistency, owner binary build, both saved renders and browser console
  checks passed. No new provider requests were made for this report change.

## 2026-09-13 — Named inputs, complete reading and return journeys

- Compact areas now show every named input, grouped once by its saved activation
  type and exact owner. Saved core/entry/dependency labels remain visible.
  Overview and detailed cards share their content and premeasured heights.
- Zoom no longer swaps layouts mid-gesture. Explicit area opening shows the
  area's title; reset keeps the current level. Programmatic camera changes
  commit history after completion. Back's second hash event cannot reopen and
  recenter a restored visit; opening the same part from overview retains a visit
  even with the same URL. Inspector expansions/scroll survive reconstruction,
  separately for the pinned input context. The initial hashless visit now saves
  its completed overview camera; Back clears the later reading and restores
  that initial overview instead of retaining a stale part in the inspector.
- Group readings consolidate repeated connections by exact peer and direction:
  one saved summary per relationship, all original evidence beneath it. Position
  evaluation's 10 evidence rows are three peer groups (2/3/5), with no loss of
  source pairs. Key code is visible immediately, duplicate full-group actions
  and vague All disclosures removed. Input participants and connected inputs
  are navigable; their original call-path explanation remains available.
- The common canvas stays mounted above answers and reference sections. The
  component sidebar links to existing flow, configuration, data, core,
  dependencies, dynamic execution, coverage and TODO material. Removed the
  two introductory labels and the duplicate catalogue inserted below the map.
  All operation and outbound catalogue rows remain visible within their groups.
- Small-repository browser journeys: Othello overview -> zoom/pan -> part ->
  Back/reload retains exact camera; detail -> overview -> same part -> two Backs
  restores both visits. Pinned Track pointer position -> View model -> hover its
  area highlights 11 edges/6 labels; moving into the sidebar restores the empty
  input path without replacing View model's reading. Python/TypeScript Run ->
  HTTP service -> backend POST -> validation -> question/map return preserves
  source-backed explanations and camera. Component -> dynamic execution exposes
  backend/app/field.py:98 in the reading below the same map.
- Focused report tests/vet, 12 frontend tests, generated-asset check and owner
  binary build pass. Saved online reports 20260913-113850-othello-d98e15694e4a
  and 20260913-113853-python-tutorial-game-7e7249a73524 render with zero provider
  requests. These are small-repository UI checks, not acceptance of a new
  multi-component semantic zoom or of the large-repository layout.

## 2026-09-13 — Separate reading, hover connections and zoom overview

- Node hover no longer replaces the selected inspector with a smaller duplicate.
  Clicked details and links stay mounted; connection evidence uses its own section
  and survives moving from its label into the reading column.
- One emphasis reason drives nodes and edges together. Area hover temporarily
  replaces a pinned path and clearly names/frames the area; leaving restores the
  path. An off-path inspected part stays outside that path. Search results do not
  inherit old highlighted edges. Removed the Selected colour legend and purple
  state recolouring; input/external type colours remain stable.
- Added a compact ELK overview using only saved areas, complete part membership,
  input counts, original directed relations and component ownership. Detailed
  card positions remain fixed. Zoom/click moves between display levels in the
  same canvas; history includes the level and both geometry identities. Overview
  keeps readable type and starts at the top/left in a narrow window.
- Browser journeys: input Track pointer position → Test helpers (explicitly outside
  its path) → hover area (4 edges) → right panel (0 path edges restored) → To code
  → Back to map. View model → hover UI presentation logic (6 grouped labels) →
  connection sources (14 source links) → move into inspector → To code → Back.
  Full details remain present throughout hover. Clear selection clears the heading.
  Detailed inventory remains Othello 14 parts/8 inputs and Python+TypeScript
  25 displayed cards/12 inputs/3 outbound records; all 50 detailed routes remain
  orthogonal and do not pass through leaf cards.
- UI vocabulary verification now checks authored frontend sources as well as
  templates; it catches a missing message before runtime. Eleven frontend tests,
  generated asset check, focused report tests, vet and owner binary build pass.
  Both existing saved online runs were rendered with zero provider calls.

## 2026-09-13 — React Flow + ELK system canvas

- Replaced the system SVG viewport and custom boundary route planner with React
  Flow and ELK's compound part-to-part routing. The adapter consumes existing
  report IDs, owners, operation paths and original relations. It adds no analysis
  stage. Legacy small diagrams reuse the same bundled ELK code.
- All bound inputs are named blue cards inside their exact implementing part;
  unbound inputs remain nodes. External observations are amber cards with native
  labels/addresses and their original sources. Component frames show language,
  kind, role and purpose. Neutral structure and purple selection replace the
  indistinguishable green tones. Input inventory: Othello 8, Python/TypeScript 12;
  the latter retains 3 independent external observations. No card text overflow
  was found in those rendered inventories.
- A reserved reading column replaces floating node previews. Numbered connection
  labels group outside identity + direction and every inner endpoint; hover
  reads original sources, number click pans to the outside part. Labels persist
  across their gaps and clear on workspace exit. Lines are noninteractive and
  have no native tooltip. Selection keeps node positions/sizes and camera;
  explicit search/destination navigation moves to the exact card.
- Browser verification found stationary-pointer previews after clicks/navigation,
  initial fit racing saved camera restoration, and stale ELK bends when comparing
  orientations. Fixed these on the ordinary path. Each ELK candidate now receives
  a fresh graph; regression exercises the real mutating library. Final rendered
  geometry: 32 Othello + 18 Python/TypeScript edges, no diagonal segments or paths
  through leaf cards. Input click, grouped destination/Back and question 5 → Run
  → backend validation → code → reload → map → question were walked in the browser.
- Developer-only `make ui-build` compiles checked-in JS/CSS; `make ui-test` runs
  6 tests and verifies generated assets. All pass, as do focused report tests,
  vet and `make build`. A local `go install` with Node absent from PATH completed
  and rendered Othello. Go/npm use their ordinary shared caches. No external
  script assets are requested by either generated report.
- Final UI renders reuse completed online runs 113850/113853 and their saved
  translations, both exit 0 with no provider calls. Preview is the actual generated
  HTML at loopback 8774. Details and input-method limits are in the UI review.



## 2026-09-13 — Boundary grouping and transient hover

- Owner screenshots exposed crowded Domain core labels and annotations that
  survived pointer exit. Previous verification of one convenient area did not
  cover these cases. Boundary groups now use outside identity plus direction,
  displaying every inner endpoint number once beside a single outside name.
  All original calls and source pairs remain in the group's inspection.
- Measured labels, marker hit areas and area titles reserve space; routes avoid
  them and replace the corresponding overview bundle. Extra annotation space is
  part of the initial drawing bounds. SVG edge paint order no longer changes
  which saved relations are highlighted. Selection cannot revive hover labels;
  empty canvas, exit and scroll clear them. Prose width no longer limits the canvas.
- Browser checks covered Othello's dense Domain core (9 groups), UI presentation
  logic (6 groups), frame/part/marker/empty-canvas/outside transitions, scroll,
  grouped source inspection and the Python/TypeScript validation area while a
  Run operation remained selected. A 1600 CSS-pixel window has a 1535-pixel
  canvas and drawing. Final exit leaves no temporary numbers, ports or tooltip.
- Focused report tests (including dense collision and pointer regressions),
  vet and ordinary build passed. Both 113850/113853 saved runs were rendered
  through the ordinary command with zero provider requests. This is a UI-only
  iteration on those previously completed online runs, not another online run.
  Detailed input-method coverage is recorded in repomap-ui-ux-review.md.

## 2026-09-13 — Common canvas and boundary labels

- The ordinary home report now assembles all existing component maps into one
  canvas after translation. Exact destination links join remote copies; saved
  membership, input ownership, native HTTP registrations, outbound records and
  unread components remain represented. Selection changes emphasis and the
  reading below the map, without expanding code lists or relaying out the scene.
- Operations highlight their saved path on the common canvas, including other
  components. A single-part operation stays in that part. Boundary markers
  match local inner-part numbers and name outside participants, retaining the
  earlier directed layout and full-width canvas. Marker click opens
  original relation evidence. Internal arrows remain ordinary arrows.
- Browser checks covered Othello View model → Quil painting boundary marker
  → exact source evidence, plus Track pointer position.
  The owner rejected the intermediate grid/number-strip design with an annotated
  screenshot; that display was removed, and the original directed layout restored. The canonical question 5 → Run simulation on click
  → backend HTTP API endpoints preserved identical node geometry and question
  context. To code → reload → Back to map → Back to question returned to question
  5 with the operation retained. Connection style survives reload too.
- Focused `internal/report` tests and vet, `make build`, and both saved renders
  passed. Ordinary Othello `20260913-113850-othello-d98e15694e4a` exited 0 in
  4.928 s; canonical Python/TypeScript `20260913-113853-python-tutorial-game-7e7249a73524`
  exited 0 in 10.330 s. All model responses reused the shared cache. The complete
  common artifact chain and each target's native/group artifacts were inspected;
  native facts, semantic atlas and common report graph match the prior accepted
  inputs (run-local facts references and cache accounting differ). The two-target
  publication has one manifest, report JSON and physical HTML.
- This records the working display experiment and its verification, not final
  owner approval of the visual design. No analysis cache or provider contract
  changed; CURRENT was not consulted or modified.

This is a concise living log, not another ADR. [CURRENT](CURRENT.md) owns the
current decision/status; [topical contracts](../../AGENTS.md#start-here) own
implementation details. Record a changed decision there in place. Append only
useful implementation/acceptance evidence here; archive superseded detailed
runs instead of growing the entry pages again.

## 2026-09-13 — Responsibilities instead of directory boxes

- Replaced file `here`/sibling/`new:` placement and fixed-count zone naming
  with aggregate declaration membership and areas in the ordinary atlas stage.
  The owner receives declarations, docs, native calls, values and activation
  context. Parts can cross directories and split one file; native relation
  endpoints and lexical ownership survive projection. Atlas v8 records exact
  members. No parallel analysis path, browser semantic validation or new cache
  was added. Conflicting memberships are refused together; independent groups
  survive and missing choices remain explicit inventory.
- Regressions exercise same-file separation, cross-directory collaboration,
  singleton collaborators, conflicting/unknown refs, retained lexical children
  and native cross-part source anchors. The saved Othello complete-window probe
  covered all 205 declarations and six author contexts (121,734 prepared bytes
  before the final behavioral-responsibility prompt clarification).
  Full `make test`, `make vet` and `make build` passed with ordinary Go caches
  and the existing nvm Node 21 TypeScript installation.
- Ordinary online final Othello run
  `20260913-102848-othello-055f2b24716e` exited 0 in 4.279 s, all model requests
  reused. Relative to the accepted 084650 run: 5 → 16 parts and 10 → 169
  cross-part native connections; all 354 subjects and 3,213 structural edges
  and the native index hash are unchanged. The visible product map has 14
  parts and 157 connections; two test-only groups remain in the complete data.
- Canonical ordinary online baseline
  `20260913-094041-python-tutorial-game-152990d369d2` exited 0. Final
  `20260913-102843-python-tutorial-game-ea9674967cc8` exited 0 in 35.989 s.
  Backend: 2 → 9 parts, including source validation and sensor initialization;
  frontend: 6 → 13 parts, including HTTP service, editor and simulation UI.
  Both native hashes, all subjects (219/259) and structural edges (567/679)
  are unchanged. Three native HTTP operations remain. The refreshed model
  selection marks Rootpage and LevelPage `operation_candidate=no` rather than
  separate interactions; both remain key code and participate in the flow.
  Othello adds the interpreted button-click interaction (7 → 8).
- Inspected complete owner/sibling artifact chains and matching published
  GroupsIndex hashes. Each run has one common manifest, report JSON and physical
  HTML; all targets retain their native index and dependency catalogue.
  Browser walkthroughs used loopback reports on 8772/8773: validation question
  → backend validation part → `app.py:90` / `utils.py:18`; Othello click → six
  collaborating parts, retaining eight scenario connections after opening a
  part and reloading. Root areas show their parts and internal arrows directly;
  the layout uses full page width and keeps all folded source relationships.
  A final UI-only correction routes full-code links through reading history:
  question → map → code → map → the original question now retains its named
  return. Focused report tests/vet passed; both final saved reports were
  rendered again with zero provider calls and the journey passed in the browser.

## 2026-09-13 — Clojure JVM and glossary reading

- Added the registered Clojure JVM adapter through native clj-kondo EDN,
  ProgramIndex and ordinary reading/reporting. The cumulative Clojure fixture
  and exact inventory cover imports, direct and shadowed calls, callback
  argument binding, reader syntax, Java uses and exact author docstrings.
  Focused corpus, claims, places, adapter, run, contract and report tests and
  vet passed. Existing native JSTS equivalents passed using the owner's
  TypeScript installation in nvm Node 21; Go and Python cumulative coverage
  passed in the contract suite. ClojureScript execution and Scala are not added.
- Installed the normal Clojure CLI and clj-kondo environment; retained ordinary
  caches and Java 21, and installed Java 25 required by current Metabase.
  Repositories live in `~/git`. Othello's actual specs passed: 143 examples,
  507 assertions, zero failures. Its desktop dependencies and Metabase's run
  dependencies resolved; Metabase's bootstrap namespace loaded on Java 25.
  This is not a complete Metabase application build. The optional complete
  Metabase native probe reached its root's 4,035 files and exceeded the
  five-minute test timeout; full Metabase analysis remains unaccepted.
- Ordinary online Othello run `20260913-083717-othello-56735348db67` exited 0
  in 260.652 s. Final EDN repeat `20260913-084650-othello-82aa91ac38fe`
  exited 0 in 4.427 s through the shared accepted-request cache, with zero
  live provider calls. Inspected the manifest, shared program facts and index
  set, dependencies, reduced documentation, places, atlas, tables, GroupsIndex,
  analyzed target outcome, report JSON and the single HTML. Browser reading
  followed desktop/browser launch instructions, original sources and UI flow.
- Othello exposed glossary context rendered as hundreds of apparently direct
  citations: AI had 834 locations across 28 files; rules had 796 across 28.
  Domain entries now label this saved provenance `Analysis context`, collapsed
  by default; file paths appear once, locations require opening the file.
  Questions are a separate collapsed reading choice. `(via model)` sits in
  the preview heading, outside the definition. Original destinations remain
  in static HTML. Report tests and vet passed; saved rendering updated both
  accepted Othello HTML files with zero provider calls. Browser walkthrough
  verified AI context, README permalinks and a linked question; details are in
  `repomap-ui-ux-review.md`.

## 2026-09-11 — current correction wave, ordinary acceptance pending

- The exchange journal records `cached_input_tokens` (DeepSeek's
  `prompt_cache_hit_tokens`) and `reasoning_tokens` beside the token
  counts, so a run shows whether lead-first windows over shared evidence
  were served from the provider's prefix cache and how much of the output
  was reasoning; the probes on the saved Freqtrade window had to infer both.
- Learn re-asks the intents a response left out and lists the intents
  after the evidence (learn v5). The owner's run (524,288-token provider)
  split Learn into four windows: two answered only the first intent
  (`purpose`) and stopped, two answered nothing, so seven intents were
  "missing intent review" with no other entries in the response and the
  report had two questions. Like retrieval, an intent with no entry in an
  accepted response is now `intent_omitted` and re-asked over the same
  evidence, first together with the other omitted intents, then one per
  window, before it is unavailable; a refused first window goes straight
  to single-intent windows. The intent catalogue moved from the system
  prompt to `"intents":[…]` after `evidence`, so a re-ask shares the
  request prefix with its parent window and the provider's prefix cache
  serves it; the prompt says `sources` are `e*` evidence refs only (the
  owner's model cited `h1 h2 h4` context refs and lost a question).
- Symbol and operation requests carry one decision per cell and define
  every value they render (schema review, cold fixes; symbol-selection v5,
  symbols v8, operations v20). `activation` (seven options consumed as a
  yes/no gate) is now `operation_candidate` yes/no; `call_count`/
  `limit_from` (an unreachable check) are gone; `invocation: synchronous`
  and `resolution: exact` are left out as defaults and exact local callees
  collapse to one `local_calls` line; origin trees (`source_arguments`,
  `receiver_value`, `result_value`) render as `kind`/`text`/`parts` two
  levels deep without anchors or owners (they were 32 % of the Morfeu
  orientation request and 90–95 % of an operations row); `name_kind` is
  asked only when a registered name exists (`[label]` was the single
  option in 16 of 18 rows), null fields are not rendered, the `u*`-era
  paragraph is gone. A shared `evidence-vocabulary.md` defines every
  rendered `invocation`, `resolution`, origin kind, witness kind,
  extractor and control label and is attached to the symbols, operations
  and boundaries prompts; a contract test renders every Go fixture row and
  fails on a value the prompt does not name. The response example puts
  `key` first, then the cells in fill order. `Column.Unasked` names the
  value an unasked conditional cell takes, apart from `Missing`.
- Boundary, file, directory, zone, target and portfolio requests state
  each fact once and offer only real choices (schema review, cold fixes;
  boundaries v7, files v5, directories v4, zones v2, targets v3, portfolio
  preparation 8). Boundaries: the address catalogue no longer lists format
  strings or literals from `fmt`/`errors`/`log`/`time`/`strings`/`strconv`
  calls (Morfeu: `unknown` in 25 of 29 answers, the catalogue being error
  templates) and is sent only when the code does not know the address; the
  out-kind options are the kinds the group index keeps (no `http_server`,
  `config`); `destination` is a closed `d*` choice from the shared known
  systems list annotated with the target's dependencies, with `other: `
  for a system outside it, so the page no longer normalises four spellings
  of one broker; the outgoing `line` is a 120-rune text; the owner's calls
  (line ±3 and client constructors) and source context are sent once per
  window in `context.owners`, rows referencing `owner_ref` (51 % of a
  Morfeu window was that repetition); fixed native facts get their own
  24-line prompt. Files: `box` is asked only when there is more than one
  option (`Missing: "here"`), callers name the declarations that witness
  the file edge. Directories share the parent's line in the window context;
  a zone part may hold one box. Targets define `shared_code`, target kinds
  and boundary labels; portfolio observations carry named fields instead
  of positional values.
- Three small stages ask one decision each (schema review, cold fixes).
  The README classifier asked ten classes over 14 KB of rules while only
  `target_entry` was consumed, and on Morfeu classified exactly the README
  files whose text it was given; it now asks which files the guidance names
  as entries, sends candidate files only (no prose, nothing under
  `.claude`/`.github`/`.vscode`), and answers `{"files":[…]}` in JSON mode
  (system prompt 13.6 → 4.8 KB). `documentation_reduce` drops `claims`
  (read by nothing but the glossary collector, where governance-template
  phrases entered the report) and caps `concepts` at twelve per document.
  The glossary labels each term with a closed `kind`; `identifier` terms
  (54 of Morfeu's 122: env names, file names, `RF01`, skill names) are
  accepted but not published. Contracts of all three stages bumped.
- The learning menu is bounded and questions are merged as groups (learn
  select v4, merge v2). Morfeu with the owner's 524,288-token window split
  Learn into 8 windows, each proposing "without quota"; select kept 11–14
  questions per intent and the row-form merge (each question choosing its
  own representative) merged none of 100, so a run's question count
  depended on how many windows Learn needed (40 vs 100, tokens ×2). Each
  intent's menu now chooses at most five questions (`limit: 5`, an
  over-limit choice refuses that intent's row); the merge is one call per
  pool that partitions the catalogue into groups of one information need
  (`{"groups":[{"representative","members"}]}`): on the saved 100-question
  catalogue the row form merged nothing twice while the group form gave 78
  groups with 16 sensible merges. Unplaced refs stay their own group, a
  refused window keeps every question.
- Question evidence sends each row fact once (question-batch v4, answer
  v9, learn v4). On the Morfeu retrieval window (2.07 MB for 24
  questions) `heading_path` cost 191 KB, its last element repeating the
  unit's own `section_title`/`section_line` and the parents repeating per
  unit; `anchor_path` (98 KB) equalled the row's `path` for 1,807 of 1,834
  units; `owned_declarations` copied type members that were already units
  of the same chunk (274 KB on the Freqtrade code window). A unit now
  carries `heading_path` as its parent titles only, `anchor_path` only
  when it differs from the row, and a member that is a unit of the same
  chunk as `{"ref":"aN"}`; `AnchorEvidence` restores the full record for
  routes, Learn and answers. Morfeu window −10 % (2,074,710 → 1,864,461
  bytes); memos and exact cache of these stages go cold once.
- Six places where the model was asked without a decision to make, or asked
  twice (schema review, warm fixes; request bytes of the remaining rows
  unchanged). A column-less native outbound fact now claims every selected
  call on its line (Morfeu `152759`: 4 of 29 outbound records were the same
  call twice, `bnd:…:sdk` beside `out:…:166:42`); the claim matches every
  native source, since an SDK observation arrives as `external_call`, not
  `fact` (run `171727` still showed the four duplicates while the claim
  matched `fact` alone). Orientation numbers groups
  and members in an order the graph fixes (sorted member ids, then lane,
  title, summary) so one group's changed lane no longer renumbers every
  `g*`/`s*`. An arrow without witnesses (an import-only edge) gets its
  fallback sentence "A uses B." without a model row (Morfeu arrows r7/r10
  had invented sentences). A declaration whose native route is its
  operation is not reviewed by the operations table (its model row was
  dropped by the group index anyway). The glossary comparison round is
  skipped when every group is a singleton whose lowercased names never meet
  (Morfeu: 121 self-assignments, 36 KB). Files under `.claude/`, `.github/`
  and `.vscode/` are never portfolio candidates (Morfeu asked about a hook
  script every run and warned "Target not analyzed").
- Retrieval asks at most eight questions per window and re-asks the ones
  the model omitted. Freqtrade `20260910-144751`, window w1: 64 questions
  over 460 code rows (4,661 anchors, 2.1 MB) came back with four keys and
  `finish=stop`; 60 questions became "missing question" and 27,600 cells
  unavailable with no second request, while the same 64 questions over
  1,241 document rows came back complete. Probes on that saved window:
  eight questions over the same 460 rows → 8/8 in 48 s; with thinking off
  the 64-question window degenerated into counting anchors up to the
  128,000-token cap, so reasoning is not the lever. `plan()` now cuts a
  window's questions into groups of eight over the same rows; the first
  window of each row set runs before its siblings so DeepSeek's prefix
  cache serves the rest (parallel probes hit 1,408 of 516,842 prompt
  tokens); after the first round the questions absent from an accepted
  response are re-asked once over the same rows (`question_omitted`
  journal rows, `recovered` when the second round answers, one console
  line naming the counts). Prompt and contract unchanged; per-question
  memos survive; whole-window cache entries of the old packing go cold.
- A refused answer window divides by questions before giving up. Morfeu
  `20260911-112125`: one empty provider response (`provider_no_content`)
  made all 22 questions of a window unavailable; only resource refusals
  divided a window. A failed call, an unusable envelope or a response with
  no accepted row on a shared window now divides its questions like a
  resource refusal until a request holds one question; the journal keeps
  the refused attempt as superseded and the console says why the window
  continues in smaller requests. Request bytes unchanged.
- A text the translator refuses stays in the source language instead of
  failing the run. The owner's run lost `t38` three times in single-entry
  windows, and by the previous code a terminal single-entry refusal ended
  the whole run after every stage had finished. `Translate` now keeps such
  an entry untranslated, `rejected.jsonl` gets an `entry_untranslated` row
  per text and the console names them once; a failure before any provider
  answer still fails the stage. `ExecuteAdaptiveJSONEachResults` exposes a
  terminal leaf beside the completed cover; the old entry point behaves as
  before.
- Learn reads a review without a reason and drops bad questions singly.
  Freqtrade run 20260910-144751, Learn window w3: eight `questions` reviews
  with 26 questions and `"reason": ""` on every one were all refused
  ("learn: a review needs a reason") and the window was lost as "no intent
  reviews accepted" with eight unavailable intents; the owner's provider
  produced the same message. A questions review with a blank reason and at
  least one accepted question now takes the first sentence of that
  question's `why` as its reason and records `reason_from: why` in
  learning-plan.json and the tables journal; `not_applicable`/`unknown`
  reviews still need their own reason. A proposed question that fails a
  rule (blank wording or why, no or only unadvertised sources) is dropped
  alone with a `question_rejected` journal row naming the intent, the
  question's beginning and the rule; the review keeps its other questions,
  and is refused only when none survive. Prompt, request bytes and cache
  keys unchanged.
- A missing question or Learn intent review says what the response held
  instead of only the gap: the rejection reason lists the field names of
  the entries that named no asked question or intent and the values under
  key-like fields ("3 unmatched entries with fields [question_ref rows],
  named [question_ref=q1]"). The key-synonym tolerance added for a day
  (`question`, `question_id`, intent titles, keyed objects, keyless order)
  is withdrawn: no saved response showed a provider naming keys another
  way, while the Freqtrade run that asked 64 questions in one window got
  q1, q2, q3 and q64 back with the right keys and nothing else. The gap
  is an oversized window, not a naming mismatch. Contracts unchanged.
- The Learn plan is read per intent, not per window. After a context
  refusal the evidence continues in several windows, each asked all eight
  intents; a window that skipped an intent another window reviewed used to
  add an "unavailable" review and make the whole plan "partial", so the
  questions page said "some topics could not be reviewed" although every
  topic had a review somewhere (the owner's run: six windows, 42 per-window
  misses). `consolidateLearningReviews` drops an intent's unavailable
  reviews when any window reviewed it and sets partial only for an intent
  no window reviewed; the journal keeps every per-window rejection. Two
  tests that encoded the per-window shape now assert the per-intent one.
- Editor and tool state directories stay out of the corpus: `.history`
  (VS Code Local History keeps copies of edited files, README included,
  which the owner's run then read as documentation), `.terraform`
  (downloaded providers with their READMEs), `.idea`, `.vs`. Corpus test
  covers `.history/README.md` and `.terraform/providers/x/README.md`.
- Learn selection reasons no longer show request-local keys: a model that
  wrote "q1 and q5 cover the same idea" put "q1", "q5" on the page. The keys
  are spelled out as the questions they stand for (`spellCandidateRefs`);
  keys outside the window's pool stay as written.
- A rejected response's row-level reasons reach the console. "learn: no
  intent reviews accepted" said only that every intent review failed; the
  WARN now lists up to four "rejected: intent: reason" lines from the same
  journal record (`SemanticFailureReceipt.Rejections`), for every stage.
- Selecting a code member no longer sets the part's summary vertically.
  With a member chosen the docked card gained a second column for the
  member panel (`.map-card-has-concepts`, 1fr + 1.35fr), which in the
  384 px side panel left the summary about 160 px wide; the member panel
  now stacks under the summary in the explorer. Verified: summary 367 px,
  member panel 367 px beneath it.
- Call lines keep their type when it tells records apart, and carry the
  note. The member-only line read "Patch, Patch, Patch" on a Kubernetes
  client: a member used by several types within one destination now keeps
  its type ("PodInterface.Patch", "DeploymentInterface.Patch"), a member
  used by one type reads alone ("ExchangeDeclare"), and the generic-name
  list gained Patch, Watch, Apply, Scan, Push, Pull, Insert, Select,
  Subscribe, Emit. With the telegraphic boundaries note the purpose fits
  the line itself: "ExchangeDeclare объявляет topic exchange morfeu.events
  и DLX morfeu.events.dlx · client.go:322"; the disclosure repeats the
  purpose only when it is longer than the note.
- The operation view says what it shows. After the first jump into
  "Операции › criar-filme" the owner liked the picture and asked what he
  was looking at. One muted line under the crumbs now reads "criar-filme:
  the dark box is the operation; below it the part with its handler and
  the parts that handler reaches. A solid arrow is a call in code, a dashed
  one an interpretation." (`.explorer-caption`, hidden in structure mode).
- "Where this service connects" on a real broker/database service (Morfeu:
  Echo + sqlc + PostgreSQL + Redis + RabbitMQ). Destination groups are
  keyed by the known system a destination names: one run called one broker
  "RabbitMQ broker", "AMQP broker (RabbitMQ)", "RabbitMQ broker (queue
  topology)" and four more ways, thirteen groups for three systems; now
  RabbitMQ · 17, PostgreSQL · 9, Redis · 5 (`canonicalDestination`;
  records keep their wording, an unresolved "Cache store" stays apart).
  A call line drops the package the group already implies and keeps the
  type only for a generic member: "Confirm", "ExchangeDeclare", "Pool.Ping",
  "Migrate.Up" instead of "amqp091-go.Channel.Confirm"; the full callable
  stays in the record body. An unresolved interface call takes its calling
  function's name for the line instead of its first sentence of prose. The
  body no longer prints "address not determined" or a one-step chain that
  repeats the record's own anchor. The source location sits on its own
  line under the call; the continuation rows live inside the same list.
  On the first screen the sixth and later rows of a long group open in
  place ("Развернуть · ещё N") instead of sending the reader to the
  component page ("all 13 → flung me upward").
- Boundaries prompt v6: the `line` cell is a telegraphic note of at most
  ten words without a subject ("stores events in PostgreSQL"), no narration
  of the call, an unknown appended as " · not established: …" only when it
  matters. Two probes on Morfeu (same code): a one-sentence wording gave
  average 28 → 11 words, the telegraphic wording 6 words (max 8):
  "publishes catalog events to morfeu.events with confirmation", "applies
  pending schema migrations to PostgreSQL", "checks Redis connectivity at
  startup". With the first wording "declareTopology issues an AMQP QueueDeclare on
  the broker channel to idempotently declare the DLQ … a broker management
  exchange, not a business publish" became "Declares the dead-letter queue
  for failed movie-created events on the broker"; "createDBPool builds the
  PostgreSQL connection pool from the parsed config … actual query
  exchanges happen through its library" became "Creates the PostgreSQL
  connection pool used to store and read application data". Three of 30
  candidate records were not accepted under the new wording; the boundary
  cache for v5 windows is cold.
- The page follows an opened map block only after the reader's own click
  on the map. `revealInWindow` (755383f6) ran on every layout, so a
  component page opened through "All 6 →" or a route link scrolled past
  its inventory to the auto-opened root area: the owner's "everything jumps
  somewhere on click" on the first page. `open()` now marks the map before
  it re-lays out; other layouts leave the page where the link put it.
  Verified: "All 6 →" lands on the inventory (target at 119 px), a node
  click still brings the opened block and the panel into view.
- Interface words a newcomer can read, from the audit's list of seventeen
  unexplained labels: "Входящие запросы" (was "Записи входящих обращений"),
  "Внешние вызовы", "вызов в коде" / "настройка клиента" (was "Вызов
  взаимодействия" / "Настройка взаимодействия"), "Кто обрабатывает входы",
  "Выполняет переданный код (eval/exec)", "Обработчики, для которых маршрут
  не найден в коде", "Адрес не установлен"; an address that passes through
  a frontier reads "Адрес приходит через `v5.Connect`" instead of "не
  определён · v5.Connect". The program index's platform notation stays off
  the page (`platform:javascript.WebSocket` reads WebSocket;
  `displayCallable`). The part search re-lays the map 200 ms after typing
  pauses instead of on every keystroke.
- Long lists read in groups the data already had. Operation lists longer
  than seven rows from more than one file are grouped by source file
  (gop: 17 user actions → six files), each file compacting to five rows on
  its own; the complete key-code list of a large part (`All N →`) carries a
  heading per source file; the data catalog shows tables first and SQL
  texts after, each with its own disclosure; the configuration table is
  sorted by source file so environment reads and manifest keys group
  themselves. `operationsByFile` template function; the first-screen copy
  still takes the first five rows of a group.
- Less noise on the component page. Configuration, manifest and TODO facts
  from vendored trees (`vendor/`, `node_modules/`, `third_party/`,
  `.terraform/`, virtualenvs) and data records from those trees or naming a
  database's own catalog (`pg_*`, `information_schema`) stay out of the
  reader's lists: gop's TODO list was 43 of 43 vendor files and its one "SQL
  text" was a Go error string from vendor/. The data catalog no longer
  prints "Connection not determined" under every row. The legend defines
  area, part and key code in words. Fit, zoom and reset sit in the explorer
  bar instead of a row of their own. Secondary counters and source
  locations in catalog rows are 12 px under 14 px titles instead of larger
  than them. The CSS "model" badge is localized through a `--model-badge`
  variable set from the vocabulary.
- The map opens on its parts, and hovering a node answers "what is this"
  in a readable card. A root view whose only own node was one area showed a
  single box reading "Open parts · N" (gop, meetup, python-dotenv): that
  area is now open from the start, its crumbs still name it. Explorer nodes
  carried their sentence only in the native SVG `<title>` tooltip (delayed,
  vanishing, unselectable); the title is gone and `repomapPreview.bind`
  shows a floating card beside the pointer with the node's kind, name,
  sentence and "click — explore"; the panel still changes only on a click.
  A code cube click reads in the panel instead of also following its link
  (a new tab in static reports, the overview in served ones). Verified at
  1280×900 on meetup: parts visible on load, no `<title>` left, card shown
  on a real hover and hidden on the next pointer move away.
- The map's explanation stands beside the map. The component map explorer
  put its inspector under a stage capped at 58vh, fixed the inspector at
  16rem and scrolled the page back to the map top after every click, so an
  explanation appeared 210–334 px below the clicked part, outside the
  window, and the wheel went to whichever of three nested scrollers was
  under the pointer (the owner's "awful scroller"). Now the workspace is two
  columns above 1100 px: the stage grows with its content (no vertical
  cap) and the inspector is a sticky side panel with auto height, bounded
  by the window; below 1100 px it follows the map with no fixed height.
  `orient()` scrolls only when the map is entirely out of view; an opened
  block low on a tall map is brought into the window by the page
  (`revealInWindow`). The layout width is the stage's, measured one
  microtask after the bundle has run (the first render used the figure's
  full width and produced a 1100 px map inside an 816 px stage). Breadcrumbs
  wrap instead of scrolling horizontally; code cube names clamp to two lines
  instead of scrolling inside the node; viewport history entries are
  written every 250 ms instead of every frame. Verified at 1280×900 on
  meetup and jumping-circle: part click leaves the page in place, the
  explanation is in view beside the map, the stage has no inner scroll.
- The component page leads with what the component is and ends the map
  with what happens: the orientation role and purpose (the overview card's
  sentence, same display refs) sit under the heading with the entrypoints
  on one line, and the main flow and the configuration table follow the map
  instead of hiding inside the "Code, entrypoints and sources" reference
  block. A component without a flow or a start list shows no flow section
  at all ("the model did not include this target" described the tool, not
  the code). Found by the owner's audit: five visible words below the map on
  every one of seven pages, no purpose sentence anywhere on the page.
  `TestComponentPageLeadsWithPurposeAndKeepsFlowBelowTheMap`.
- Data catalog links are live again. Record ids carry a kind prefix with a
  colon (`entity:f-…`, `query:q-…`); inside a fragment href html/template
  read the text before the colon as a URL scheme and replaced the link with
  `#ZgotmplZ` (meetup: 82 dead "SQL texts" links found by the static DOM
  audit). Page ids for data rows now come from `dataRowID`, which swaps the
  colon for a hyphen in both `id` and `href`; regression
  `TestDataRowIDsAreSafeFragmentTargets`.
- "Where this service connects" records are compact lines beneath their
  destination instead of full cards under a "Records · N" disclosure. The
  owner opened the meetup report and expected the destination title, then
  visually nested call blocks, brief, each opening its description on click,
  and an expansion after three. Each line now shows the native method and
  address, else the callable, else the first sentence of the purpose, with
  the source location; its purpose, address, basis, source anchor and
  destination chain open under it. Three lines stay in view, the rest wait
  under "Развернуть · ещё N" (`First`/`Rest` on the group, preview constant
  3). The group-level lead sentence is gone; the destination is named once.
  The first-screen copy clones the group's own children, which also stops a
  record's purpose and "address not determined" from standing in for the
  group's (the copy used `querySelector`, which descended into the first
  record). Counts and locations no longer break onto their own line under
  the catalog's block-level `.meta`. Contract sentence in REPORT.md; tests:
  five records render as three lines and one expansion between the third
  and fourth record, the destination named once.
- A "Where this service connects" section with more than five destinations
  is compacted to five rows and an "All N" disclosure. The compaction removed
  every `ul.operation-catalog` inside the section, which included the record
  list nested inside each destination row, so every "Records · N" disclosure
  opened to nothing (the owner's service: "Kubernetes API server · 156"). Only
  the group's own lists and disclosure move now; a destination row keeps its
  records, and the first-screen copy of the row inherits them. Reproduced on
  the real script with a synthetic six-destination section in the browser
  (six disclosures, zero record lists before; six lists and twelve records
  after). Node regression: `TestCatalogDisclosureKeepsDestinationRecords`
  fails on the previous script. Reports with at most five destinations were
  never affected, which is why the four small-repository reports checked
  earlier today did not show it.
- A symbol row whose `activation` cell is missing or null settles as
  `unassessed` instead of refusing the row: python-dotenv's ordinary run
  lost four rows to `missing "activation" cell` while their `key_symbol`
  and `outbound` were valid. Only a choice whose options already contain a
  no-decision value declares such a default.
- A run stops at the first provider refusal by credentials or balance
  (HTTP 401, 402, 403) with the cause in the console and as the final error,
  instead of walking every remaining stage with a refused window each:
  Freqtrade `20260911-070651` ran into a 402 at translation, and a later
  service run failed every request for its first minutes. Accepted work
  stays cached for the rerun.
- The finder script referenced the removed second search field after the
  Learn/Work switch was taken out (`proxy is not defined` at load), which
  stopped every script bundled after it: reading navigation, glossary and
  question links. The references are gone; a JavaScript smoke check of the
  bundle is part of the report tests now.
- A single-component report gets the same first-screen product card as a
  repository map would give it: name, purpose, counts and the inputs,
  commands and communication entrance, placed before the question list. The
  owner's one-service report showed the summary, then all questions, and
  the three inventory columns only on the component page.
- Glossary reduction windows list at most six sample observations per
  variant beside the real `count`; the catalog entry keeps every source.
  Freqtrade `20260911-053911` sent 137 reduction windows of 1.9–3.1 MB,
  114 million input tokens for 90 thousand output tokens, because every
  anchor of a common term rode into every window; the fourth run paid
  102 million more for the same stage. Reduction state version 6.
- Orientation walks a packing ladder after a size or context refusal, local
  or remote: 40 members per group with 6+6 observations, then 20 with 3+3,
  then 12 without observations (Freqtrade saved input: 1.74 → 1.18 → 0.92 MB). A 524,288-token
  provider with the 128,000 output reservation holds about 1.2 MB, so the
  owner's reports had no written summary; the console said `ready`. It now
  says `unavailable` with the refusal when no rung fits.
- A third context-refusal wording is recognised: "Requested token count
  exceeds the model's maximum context length of L tokens. You requested a
  total of R tokens: I tokens from the input messages and O tokens for the
  completion" (a gateway in front of DeepSeek, `code` "400"). It carries the
  same four counts as DeepSeek's own wording and is parsed with the same
  consistency checks; before this it was a generic failure and the window
  was lost instead of partitioned.
- `REPOMAP_LLM_CONTEXT_TOKENS` / `DEEPSEEK_CONTEXT_TOKENS` declare the
  provider's context window. A request whose estimated prompt tokens (bytes
  ÷ 3, below DeepSeek's measured 3.5) plus the output reservation exceed it
  is refused at preparation with the usual context refusal and partitioned
  by its stage, before any transport attempt; run 20260911-053911 spent
  36 s per remote refusal, 52 times at the answer stage alone. Unset keeps
  the check remote. No request bytes or cache keys change.
- The console now says when a refused request is partitioned: after the
  WARN for a provider resource refusal, the answer, question, learn and
  glossary stages
  print a `partitioned` state with the number of questions or evidence groups
  and the number of smaller requests that follow. The owner could not tell a
  split from a lost window; `tables.md` and `rejected.jsonl` recorded it, the
  console did not.
- A Go module library whose only consumer is one standalone executable is
  folded into that executable. An owner's service with `cmd/app` and
  `internal/app` (plus exported packages that make the module a library
  candidate) showed a "shared code" component holding every handler, worker
  and outbound call, while the product page listed only the routes: file
  ownership goes to the deepest root, and the library's root `.` owned all
  of `internal/`. The executable now claims the module root; the model's
  `shared_code` decision stays journaled with the fold beside it.
- Destination groups on the first screen carry their records disclosure
  (ids stripped from the copy), so a group expands there too; a group of
  several records shows no single record's purpose as its lead. Both were
  reported by the owner on the first grouped report.
- The provider refusal "The input (N tokens) is longer than the model's
  context length (M tokens)" is recognised as a context refusal: an
  OpenAI-compatible server in front of a 524,288-token DeepSeek worded it
  that way, and without recognition the window would be lost instead of
  split.
- The Learn/Work switch is removed from the report toolbar: the owner could
  not tell the two entrances apart, and the switch only hid the intro and
  question list or moved the search field. Summary, questions, map and the
  toolbar search are always present; the per-component entrance keeps its
  question links and the component search button; an old `?mode=` link is
  ignored. Print and JavaScript slice tests re-anchored.
- Go handlers registered as method values or method expressions
  (`mux.HandleFunc("/items", s.handleList)`, `http.HandlerFunc(s.handleCreate)`)
  now resolve to their methods: the callable resolver follows a synthetic
  bound-method wrapper or thunk to the method it delegates to. Before, the
  route fact had no handler and the handler no route, so a page listed
  `GET /test` under registrations and a separately labelled operation for
  the same handler. A wrapper around an interface method keeps resolving to
  nothing static; a middleware's returned closure still resolves to that
  closure.
- A sequence cell citing only refs outside its row's options, or nothing
  at all, is an empty selection and no longer refuses the row: an owner's
  symbol window answered `outbound: "c9 c12"` where those refs were context
  calls, not options, and lost its key-symbol and activation decisions with
  the row. Commas now separate refs like spaces. Exceeding the limit still
  refuses the cell.
- The "Where this service connects" catalogue groups communication records by
  destination: one row per destination text (case-insensitive; native label
  or kind when the model named none) with the record count, the shared kind,
  basis and address, and the first sentence of one purpose; every record keeps
  its own row beneath the group. An owner's Go service showed ten
  "Kubernetes API server" paragraphs and twenty "Postgres" ones as separate
  rows. Rendering only: no saved data changes; the first screen and the
  section count show groups.
- An operation label the model left empty or null takes the first sentence of
  the row's own description (`name_from: description`). An owner's Go service
  on another DeepSeek provider received rows with every schema key present and
  `"name": null` beside a complete description, and each such row was refused
  as `cell "name" is empty`; the official API writes the label. An empty
  description still refuses the row. Decoder rule only: requests and memo
  state are unchanged.
- Orientation is bounded by construction again: at most 40 members per group
  (`member_count` keeps the real size), 6 calls and 6 callers per listed
  member's evidence with omitted counts. After `956e5992` removed the
  12-member cap and added evidence for every listed member, the Freqtrade
  orientation request grew from 1.3 MB (accepted at 330,422 tokens on
  20260910-144751) to 22.9 MB, and the provider's context refusal failed
  publication of a 35-minute run (20260911-045158). Bounded on the same saved
  input: 1.82 MB (2,550 listed members, 258 evidence entries, largest 16.6 KB).
  A provider or preparation refusal by size or context now leaves an empty
  orientation with the refusal in `rejected.jsonl`; the report is published.
- Native boundary scopes stay within the targets holding the file, and the
  target projection omits a boundary whose file the target does not hold.
  After `956e5992` widened native scopes to every observing index view,
  Freqtrade `20260911-040254` (one route fact seen from seven views, file held
  by one product) and an owner's Go service failed at publication with
  `atlas: boundary … names unknown box`. Regression tests in `places`,
  `reading`.
- A complete keyless response is read in asked order: the model drops the
  `key` it was told to copy in small answer windows (seven one-question
  windows of `20260911-040254`, then two- and three-question windows of
  `20260911-045158`, ten of 242 windows), and each such window lost its
  answers as `response row has no string key`. One row per asked row and no
  key anywhere leaves the asked order as the only reading; a partial or
  partly keyed response is still refused row by row.
- A closed choice whose only option is `unknown` accepts any answer as
  `unknown`. Freqtrade `20260911-040254` refused 27 outgoing boundary rows
  (7 whole windows) because the model copied the observed path into
  `address` where `address_options` was `["unknown"]`; their kind, line and
  destination were lost with them. An offered `a*` ref still has to be chosen.
- Restoring an empty selected file set no longer fails a run: a Go repository
  whose only Python file is a test script (air) has a Python catalog but no
  required Python target. Both adapter restorers return nothing for an empty
  selection; a non-empty one still fails closed. Regression test in `run`.
- Glossary generation/reduction output allowance is 32,768 tokens instead of
  the shared 128,000. Measured legitimate windows: 1,276–15,278 output tokens
  (Syn, issue-bot, Watchtower); measured loops: two Watchtower windows at
  128,000/128,003 tokens, 318 s and 519 s, 15 of the run's 21 provider
  minutes. The existing resource-refusal split still halves a refused window.
  Saved-window probe before the change: `scratchpad/glossary-cap-probe`.

- Third ordinary series (binary `664e34c`): Syn 224.602 s, issue-bot 574.736 s,
  Watchtower 306.593 s; 30/20/27 questions, all available. Source/prompt and
  provenance receipts are in external `work/small-repo-audit-20260910/next-*`.
  Native and operation controls passed; answer acceptance still exposed the
  listener/worker distinction, a differing GitLab comment argument and a missing
  required template argument. Their current prompts now state those requirements;
  controlled full-window checks and ordinary follow-up remain pending.
- Glossary generation/reduction now use the existing exact-request resource
  refusal memo; current cached/replayed whole answers keep priority. Exhausted
  provider-local deadlines take optional refusal while actual run cancellation
  remains terminal. Focused terminology/LLM/translation tests and vet pass;
  ordinary warm acceptance remains pending. No timeout, output cap, cache type
  or old-journal migration was added. The observed issue-bot delay was a
  309.809 s, 128k-output repetition followed by complete 31/53-row children.
- The third Syn report exposed a display error: two native registrations plus
  an unmatched proxy interpretation were labelled three routes. Route summaries
  now count native registrations; the combined catalogue identifies its records
  and unmatched handlers separately. Existing rows/anchors are unchanged. Focused
  report tests and vet pass; saved rendering/browser verification follows the
  current frozen-binary series.
- Operation v19 states the positive task-and-activation requirement in the
  existing entry decision. Watchtower's actual v18 bytes already prohibited
  listener and dispatcher promotion, but two model rows still did so. The
  clarification adds no new evidence, judge or local role repair; its effect
  needs the next ordinary run.
- Watchtower listener/route projection: graph 14 / saved input 15 / atlas 7
  retain native target origins and the fixed `listen_address` kind. Exact
  paths/methods/columns no longer collapse into one fact. Shared-source context
  follows the compiler-located subject; each target restores its own FactID and
  ObjectID. Focused sealing, owner-context, method/path, fixed-fact and complete
  reading→GroupsIndex→catalogue tests pass. A local projection of compatible
  saved Watchtower native inputs retained its two route observations plus one
  listener, with both targets' exact route identities (0.50 s, no provider).
  Genuine sealed game adapters through current facts/places produced graph
  SHA-256 `ded06f4a626388e0a80db4b12d788d570057c2197802ef702a8e800d4995d19a`.
  This is local validation; existing model wrapper operations and a fresh
  ordinary Watchtower report remain separate acceptance work.
- Boundary v5 distinguishes `remote_client_instance` from an option for a
  later constructor. Two complete saved windows were prepared through the
  current owner and replayed once each: issue-bot retained NewClient and five
  dispatches, WithBaseURL became none (2.733 s); service retained OTLP New and
  HTTP Do, while the local wrapper became none (2.030 s). Current validation
  accepted 7/7 and 3/3 rows without rejection; full original row contexts were
  equal. This is a contract check, not ordinary report acceptance.
- Watchtower's glossary reducer repeated 87,780 source records in 4.73 MB and
  exceeded provider context twice. All 458 accepted variants survived, but the
  catalogue remained partially compared. Reducer v5 factors exact anchors and
  source sets within each independent request. The same full saved window is
  502,076 bytes; one supported replay passed in 15.503 s with 155,669 input and
  5,502 output tokens. Current validation accepted all 458 assignments into 412
  groups, retaining every original variant/source/origin. Full ordinary acceptance
  of the changed reducer remains separate from this saved-window check.
- Syn's actual operation request omitted native receiver/argument context and
  classified middleware as additional incoming work. Operation v18 retains its
  original own-call observations and separates same-request middleware from an
  independently activated endpoint/consumer. Actual cumulative Go contrasts,
  consuming request/decision tests, full reading/lines tests and scoped vet pass;
  corrected model decisions await the ordinary rerun.
- Operation v17 uses own `self`/`none`; fixed-boundary v4 no longer asks a model
  to decide the existence/kind of a native fact. Exact routes and accepted
  worker rows remain independent of captions/setup callers. Focused
  lines/reading tests and vet pass; ordinary classification needs reruns.
- Question-batch v3 validates relevance per selected anchor. Native calls retain
  receiver/source arguments, API and same-line sites in question and boundary
  evidence. Orientation includes native member evidence. Document ancestry is
  carried with author scope (graph 13 / reading input 14), not used as a hard
  file-role exclusion. Integrated ordinary answer quality remains pending.
- Native declared-interface calls retain their original unresolved dispatch
  observations in graph 13 / input 14. Source-ordered Python parameter types
  can supply declared method alternatives, with reassignment/unknown controls.
  Data query/table links are reversible and operation links require existing
  exact model or callable ownership; embedded literals gain no guessed owner.
  Ordinary source-chain quality remains under acceptance.
- Deferred prose source appendices were removed from provider messages.
  Prepared/existing accepted cache v3 retain compact local context once; current
  warm execution, entity memos, original-window question memos and replay keep
  original provenance. Seven consuming package suites/vet pass; race-enabled
  parallel/warm/replay regressions pass. Receipt:
  `work/small-repo-audit-20260910/local-prose-context-fix.md` in the external
  project work directory.
- Glossary generation/reduction now use the shared 128,000-token allowance.
  The saved Watchtower window that stopped at 8,000 completed with 8,903 output
  tokens in 28.803 s (21,040 input; finish=stop). Model/user evidence and p refs
  were unchanged. Of 216 term rows, 211 have exact occurrences; five unsupported
  optional names are discarded. Structure/closed refs/text are checked; original
  source provenance still requires ordinary acceptance.
- The saved issue-bot response accepts 21/21 question rows under per-anchor
  validation (previously 18/21), without a provider call. This is an offline
  decoder check, not a corrected ordinary report.
- The real Python16 Freqtrade native check retains `/trades` →
  `RPC._rpc_trade_history` → `Trade.get_trades_query` as alternatives, including
  the original Trade owner. This closes that observed native frontier, without
  claiming runtime execution or a completed model/report acceptance.
- Orientation preparation retains complete author claims and all validated group
  members with native evidence; the prepared provider envelope remains the actual
  limit. Fresh ordinary orientation quality remains pending.
- Short AGENTS/CURRENT entry pages now route to topical contracts. Full prior
  text is preserved in the [non-normative archive](../archive/2026-09-10/README.md).
  Required native fixtures, executable expectations, package bounds, ordinary
  runs, browser QA, warm/cache-clear checks and constitution supremacy remain.

Next: complete the combined build/check checkpoint, verify the corrected small
ordinary reports, then complete Freqtrade acceptance before full Airflow. Do not
mark those outcomes complete from probes or local tests.
