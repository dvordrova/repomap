# Architecture and flow UI audit

This ledger traces each visible symptom from the rendered report back through
the browser projection, report DTO, saved artifacts, flow construction, and
local analyzers. A visual symptom is not classified as a UI defect until the
saved evidence has been inspected.

| ID | User-visible symptom / reproduction | Expected behavior | First incorrect layer | Evidence supporting classification | Chosen fix boundary | Files or contracts affected | Regression test | Status | Deferred follow-up |
|---|---|---|---|---|---|---|---|---|---|
| AF-01 | Primary names such as `newBackupCommand` and `openWithAppendLock` ellipsize at narrower desktop widths. Reproduce at 1280×800 in any selected flow. | Primary symbols get up to two lines; metadata may truncate first. | Rendering / styling. | `architecture_canvas` JSON and `.rm-arch__focus-step-copy strong` contain the complete names; CSS forces `white-space: nowrap`. | CSS only. | `internal/report/templates/architecture_canvas.css` | Asset contract plus Playwright checks at 1600, 1440, and 1280. | Confirmed; fix pending. | None. |
| AF-02 | Header says “Verified execution trace”, “verified transitions”, and “execution lanes” although the report states execution was not observed. | Static evidence is described as evidence-backed/static, and partial proof is not presented as complete execution. | Rendering copy / frontend projection. | Saved `flow_edges` preserve `certainty: static`, `resolution`, `invocation`, and conditions. No DTO loss precedes the unsupported copy in `renderFocusedFlow`. | Derive supportable copy from existing edge certainty and proof-slot states; no schema change. | `internal/report/templates/architecture_canvas.js`; asset/projection tests. | Reject runtime-execution wording for static fixtures; require evidence-backed wording. | Confirmed; fix pending. | Scenario-qualified static traces remain a model gap because transitions do not carry scenario IDs. |
| AF-03 | Check, Restore, and Snapshots dispatch paths select `finalizeSnapshotFilter` as the application callback. Saved Check UI: `main → newRootCommand → newCheckCommand → finalizeSnapshotFilter`. | Cobra `Run`/`RunE` delegates to `runCheck`, `runRestore`, or `runSnapshots`; setup helpers are not substituted as the application callable. | Analyzer / backend discovery. | `snapshot.json` already records `finalizeSnapshotFilter`. `commandHandler` calls `firstLocalCall`, which chooses the first package-local call in the callback body. The report and flow builders preserve that input unchanged. | Fix Cobra callback target selection in command-trace extraction using direct return/result evidence. | `internal/gofacts/commandtrace.go`; focused analyzer fixtures. | Helper-before-`run*` callback fixtures for returned and assigned-result calls; direct `runInit` remains stable. | Confirmed analyzer defect; fix pending. | Callback bodies with no defensible delegated callable remain unresolved rather than guessed. |
| AF-04 | Command path copy says “in saved order”, which can be read as runtime order. | Show only the explicit dispatch chain supported by transition endpoints; do not imply total runtime order. | Frontend projection wording; explicit total order is a semantic-model gap. | FlowProof transitions explicitly chain entrypoint, root constructor, command constructor, and application callable. Handler operations are sibling fan-out edges. Neither transition DTO nor model has runtime ordering. | Build the compact path from explicit transition topology and label it “dispatch chain”; never use raw anchor array/source order as runtime order. | `internal/report/templates/architecture_canvas.js`; projection tests. | Shuffled anchor input still yields the same explicit transition chain; sibling operations remain fan-out. | Classification confirmed; fix pending. | Typed order constraints require a future producer and FlowProof version; do not add an integer order field. |
| AF-05 | Backup lifecycle wording can be hard to interpret: task start, callback, join, cancellation invocation, and cancellation-context use appear together. | Preserve exact direction and use neutral verbs that match each stored relation. | Frontend label projection unless artifact inspection finds inversion. | Saved edges retain distinct `starts_goroutine`, `callback`, `joins`, `cancels`, and `uses_cancellation` relations with source, target, evidence, certainty, and provider. | Map each typed relation to precise user-facing copy; retain raw evidence in details. | `internal/report/templates/architecture_canvas.js`; relation-label tests. | Every supported lifecycle relation has a distinct label; selected edge detail retains original source/target. | Data direction confirmed; copy fix pending. | Guard conditions are syntax context, not proof that lifecycle events co-occurred. |
| AF-06 | Flow overview is immediately followed by Branches, every Exact step, and Proof slots, repeating substantial content. | Keep the compact overview; expose complete evidence behind an explicit “Inspect full evidence” control. | Rendering composition / frontend projection. | Raw flow steps are not duplicated in saved JSON. `renderSelection` always renders `inspectFlow`, which repeats the same anchors already projected by `renderFocusedFlow`. | Collapse flow-only evidence detail by default without deleting data; retain step/edge detail selections. | `internal/report/templates/architecture_canvas.js` and CSS. | Flow-only selection renders one compact path and a closed full-evidence disclosure; opening it reveals all exact steps and slots. | Confirmed; fix pending. | None. |
| AF-07 | Proof completeness is below the fold; Check looks authoritative despite missing core operation, I/O boundary, and termination. | Compact header shows a derived proof-area count and the highest-priority missing/partial area. | Frontend projection. | All eight slot statuses and missing reasons already exist in `flow.slots`; no additional DTO field is needed. | Derive a single proof summary from slot states and link it to full evidence. | `internal/report/templates/architecture_canvas.js`; projection tests. | Verified, partial, missing, and not-applicable slots produce deterministic counts and warning copy. | Confirmed; fix pending. | `Proof.Satisfied` is not evidence-closed in v2; the UI will report slot status, not claim proof completeness. |
| AF-08 | Discovered surfaces reports `0 async tasks` while Backup FlowProof contains a Scan task. | Preserve both counts and explain their independent bounded scopes. | Intentional coverage difference, with a possible surface-analyzer coverage gap. | `surface_coverage.json` is repository-wide across three build-selected executables and counts final catalog kinds exclusively. FlowProof is selected-command-local. The surface artifact did not catalog the guarded Backup scanner start. | Clarify repository-wide/tooling scope and rename count to non-worker async tasks; do not synthesize agreement. | `surface_catalog.js`; existing scope docs; asset tests. | UI copy states independence from selected FlowProof and exclusive worker/task categories. | Confirmed coverage difference; UI/docs fix pending. | Add a focused scanner-start analyzer fixture; absence in this bounded run remains visible. |
| AF-09 | Surface inventory highlights two `buildTargets` workers from release tooling. | Keep tooling ownership visible; classify the channel consumer as a worker and the finite producer as an async task. | Analyzer classification for task 2; frontend prioritization for ownership prominence. | Both records correctly point to `helpers/build-release-binaries`. Task 1 has a `channel_receive_loop`; task 2 only has a finite `control_flow_loop`, but `recordAsyncTask` promotes any loop to worker. | Promote only persistent/event-driven loop kinds to worker; preserve executable ownership and explain repository-wide scope. | `internal/experiment/surfacediscovery/async.go`; analyzer fixtures; surface UI copy. | One channel consumer + one finite producer yields 1 worker and 1 non-worker async task. | Confirmed analyzer defect; fix pending. | Primary-application versus tooling grouping can be added later without deleting valid evidence. |
| AF-10 | Landscape default Fit renders the 14 components at scale ~0.36 with large unused margins. Baseline: 1206×720 viewport, 2370×1840 ELK surface, visible group bounds only y=158…1580. | Fit uses visible landscape bounds and reasonable padding. | Rendering / layout bounds. | `fit()` uses the full ELK root rectangle, including empty top/bottom space; visible group bounds would fit near scale 0.47. No component fact is wrong. | Add deterministic visible-landscape bounds for Fit; keep layout and graph library unchanged. | `internal/report/templates/architecture_canvas.js`; asset/Playwright geometry checks. | Initial and restored Landscape fit visible group bounds inside the viewport without using hidden flow nodes. | Confirmed; fix pending. | ELK compaction itself is out of scope. |
| AF-11 | A malformed/legacy proof that omits a required slot can lose that omission during architecture projection. | Missing required slots remain explicit frontiers/diagnostics. | Report projection / DTO completeness. | `validateArchitectureProof` validates only supplied slots. The current Restic proofs include all eight slots, so this does not explain the screenshots. | Add required-slot diagnostics/frontiers only if implemented independently with focused report tests. | `internal/report/architecture_canvas.go`; projection tests. | Missing required slot produces an `invalid_slot` diagnostic and `proof_slot` frontier. | Deferred; not a blocker for current saved run. | Strict evidence-closed `Proof.Satisfied` belongs to a future FlowProof version. |
| AF-12 | The current 32-component Restic Landscape forms a narrow 2610×4436 vertical spine, starts at the minimum 0.18 scale, and leaves most of a wide viewport unused. | Sparse and mixed Landscapes use a deterministic board or hybrid composition; a meaningful connected core may retain graph-aware placement. Initial view stays readable and Fit remains explicit. | Frontend Landscape projection, layout strategy, and presentation styling. | The saved report has 11 valid subsystem groups, 39 distinct structural component pairs, and 9 structural connected components sized 24 + eight singletons. Membership and parent references are valid. The frontend nevertheless sends every compound group, hidden structural/flow edge, and a hidden unassigned node through one root layered ELK layout; each subsystem also uses a one-column `DOWN` layout. Current browser bounds are 2610×4436, with UI alone 320×1536. | Classify structural connectivity in the frontend; graph-layout only a meaningful core, deterministically pack sparse/disconnected groups, exclude hidden-only nodes and flow overlays from Landscape geometry, and separate readable initial focus from Fit. Do not change saved facts or the selected-flow projection. | `internal/report/templates/architecture_canvas.js`; `architecture_canvas.css`; browser/asset tests. | Sparse board, connected graph, and mixed hybrid fixtures; Playwright at 1600×1000, 1440×900, and 1280×800. | Fixed and verified. | Component importance/category is absent in this run, so connectivity, group size, and stable IDs are the allowed ordering proxies. |
| AF-13 | The balanced board still opens on an unexplained crop; Fit falls to 0.391; selecting a component leaves it as a thumbnail; CLI Commands remains a six-card column; singleton groups duplicate one child; deterministic remainder evidence participates as an architecture group. | Internal child grids reduce tall groups; singleton projection is flat; deterministic remainder evidence remains inspectable outside primary bounds; initial, readable Fit, and component focus use separate transforms. | Frontend Landscape projection/layout/viewport, plus one missing producer classification for the deterministic remainder. | In run `20260712-071842-restic`, all 11 groups use one child column. CLI Commands is 300×804 at y=924 and defines maxY=1728, extending 432 px below the next architecture group. UI is 300×928. Four singleton wrappers each add a 50 px header and 26 px bottom gap. Aggregate visible group overhead is 836 px. The remainder group is produced deterministically by `proposal.omitted_members_preserved` but the saved subsystem has no explicit diagnostic category. Forty-one hidden flow chips and routed edges do not contribute to `landscapeBounds()`. | Add explicit diagnostic category only at deterministic remainder production/projection; otherwise fix frontend group shape, board packing, primary bounds, and interaction transforms. Do not modify component identities, memberships, structural facts, or selected-flow projection. | `internal/componentmap/landscape.go`; `internal/report/architecture_canvas.go`; Landscape JS/CSS and focused tests. | Child-grid/singleton/diagnostic/packing transform contracts plus the four-state Playwright matrix. | Classification confirmed; fix pending. | The current regenerated report has no component titled `Snapshots Command`; generic focus behavior is reproduced against `Backup Command` and must work for any saved component ID. |

## Baseline artifacts

The ignored local directory `tmp/architecture-flow-audit/` contains baseline
captures for Landscape, Backup, Check, Init, Discovered surfaces, and expanded
Coverage and limits at 1600×1000. The source run is
`20260712-042829-restic`; its `snapshot.json`, `orientation_report.json`,
`report.json`, `trigger_catalog.json`, and `surface_coverage.json` are the saved
evidence used for classification.

The AF-12 baseline uses regenerated run `20260712-071842-restic`. It contains
32 components in 11 valid groups. Structural edges form one 24-component core
and eight isolated components; the graph is neither wholly disconnected nor
malformed. The narrow spine is therefore not a discovery, grouping, parent,
stale-DOM, or backend-dimension defect. It is caused first by applying one
compound layered graph strategy to the mixed projection, laying every group's
children downward, and allowing hidden-only nodes and overlays to constrain the
same layout. Browser captures at 1600×1000, 1440×900, and 1280×800 are saved as
`landscape-before-*.png` in the ignored audit artifact directory.

## AF-12 outcome

The frontend classifies a projection as `graph` when one meaningful group
region contains every group, `hybrid` when that region has disconnected peers,
and `board` when no region has at least three groups and enough distinct edges
to connect them. A graph or hybrid core is first offered to flat group-level
ELK; a narrow or excessively wide result is rejected rather than shown as a
spine. The fallback orders the primary region by deterministic graph breadth,
then packs groups into centered shortest columns. Remaining regions follow the
core without changing membership or inventing edges.

For run `20260712-071842-restic`, the final mode is `hybrid-board`. Bounds fell
from 2610×4436 to 1352×1756. At 1600×1000 the initial scale rose from 0.18 to
0.887; explicit Fit is 0.391. At 1440×900 initial/Fit are 0.887/0.348, and at
1280×800 they are 0.881/0.306. Initial group bounds use about 95.4% of the
Landscape viewport width at all three sizes. All 11 groups and 32 components
remain present; browser checks found zero group overlaps and zero child/group
containment failures.

The current report has no component importance or semantic category field, so
its cards remain neutral rather than receiving guessed colors. The restrained
blue, teal, violet, and amber accents activate only for explicit primary,
boundary, supporting, or unresolved category values; selection remains blue.
Structural witnesses are quiet neutral lines and brighten only on interaction.
The selected-flow projection remains separate and unchanged.

Before/after screenshots are stored under ignored
`tmp/architecture-flow-audit/landscape-{before,after}-*.png`.

## AF-13 baseline

At 1600×1000 the Landscape viewport is 1204×718 and its visible group bounds
are x=28…1324, y=28…1728 (1296×1700). The initial transform is
`translate(3.15px, 3.15px) scale(0.887)`: readable, but simply top-aligns the
whole board and gives no primary-area rationale. Fit computes 0.391 from the
full 1700 px height and produces `translate(338.96px, 17.06px)`. Clicking a
component does not change that transform.

The height contributors are concrete frontend geometry rather than malformed
membership: CLI Commands is a one-column 804 px group and alone adds 432 px to
the board's bottom extent; UI is a one-column 928 px group; every group pays
76 px of header/bottom overhead; and four one-child groups render redundant
parent and child boxes. The deterministic remainder group currently occupies a
normal board slot and participates in graph ordering even though its producer
diagnostic says it is evidence preserved outside conceptual synthesis. Routed
edges are excluded from Fit, and 41 hidden legacy flow-chip nodes have no
layout position, so neither explains the bounds.

Four-state baseline captures are under ignored
`tmp/architecture-flow-audit/current2-before-{A,B,C,D}-*.png`.
