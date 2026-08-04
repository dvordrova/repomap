# Current handoff

Last updated: 2026-07-11.

## Product direction

repomap is a local-first repository onboarding and investigation tool for an
engineer using an OpenAI-compatible company model. It builds bounded local
facts, uses a model only for bounded interpretation and conceptual grouping,
then lets the engineer challenge flows and claims through exact saved evidence.

The active product sequence remains:

1. onboard on a project the engineer already knows;
2. explore an unfamiliar repository progressively;
3. investigate one bug, ticket, or feature from a bounded starting point.

The browser is a visualization of that process, not a package dependency dump
or a general coding agent.

## Current milestone

Architecture Canvas v2 is implemented as the primary onboarding surface.

- ELK.js performs one compound layout with routed structural edges.
- Conceptual subsystem/component naming may use one revision-cached provider
  request over opaque local candidate IDs.
- Membership, relations, FlowProof, evidence, frontiers, and layout inputs remain
  local.
- Selecting one saved flow stays on the canvas and preserves main, task, shared,
  callback, cancellation, join, condition, and frontier semantics.
- Component, flow, step, and edge selection are hash-persisted and keyboard
  accessible.
- Exact saved locations open through the existing loopback report server.
- The old role-lane renderer, inline import labels, manual graph geometry, and
  rejected React spike have been removed.
- A compact non-graph orientation card remains only when a saved v2 canvas is
  unavailable.
- A separate `Discovered surfaces · Local static analysis` shelf now renders
  bounded HTTP/worker/async registrations below the canvas. It never promotes
  registration evidence into a component edge or completed runtime flow.

Provider-free preview:

```bash
make canvas-preview CANVAS_FIXTURE=internal/report/testdata/canvas/restic-backup-v2.json
make canvas-preview CANVAS_FIXTURE=internal/report/testdata/canvas/soft-serve-daemon-v2.json
make canvas-preview CANVAS_FIXTURE=internal/report/testdata/canvas/colima-runtime-v2.json
```

## Acceptance evidence

| Fixture | Components | Structural edges | Flow shape |
| --- | ---: | ---: | --- |
| restic Backup | 9 | 7 | main + scanner task + shared context |
| Soft Serve daemon | 10 | 10 | main + shared + 11 tasks |
| Colima runtime | 10 | 10 | synchronous main + 9 conditional dispatch edges |
| etcd production | 6 | 40 | landscape only; no compatible saved proof |

Restic does not duplicate `Scanner.Scan`, cancel does not join Wait, and Wait
joins the scanner task. Colima alternatives remain conditional synchronous
dispatch instead of invented goroutine branches.

Screenshots from the final browser pass:

- `/tmp/repomap-canvas-restic-flow-edge.png`
- `/tmp/repomap-canvas-soft-serve-flow.png`
- `/tmp/repomap-canvas-colima-flow.png`

## Honest boundary

- The three showcase fixtures begin at the final presentation projection. A
  smaller run fixture protects raw-facts-to-canvas wiring; a realistic checked-
  in restic raw run remains useful debt.
- Deterministic no-model fallback is contract-tested and production-usable, but
  does not yet have a separate realistic screenshot.
- Restic fixture acceptance currently focuses on Backup; Init and Restore are
  not additional selectable flows in that fixture.
- Flow transitions that retain only a provider name cannot expose provenance or
  scenario that was never saved.
- Arrow-key spatial traversal is deferred; essential selection is still
  keyboard accessible.
- Architecture synthesis request bytes/latency live in
  `architecture_synthesis.json`; old orientation request totals do not include
  that later cached call.
- Legacy manifest component IDs authorize lazy symbol investigation. v2
  conceptual component IDs are different and must not be treated as the same
  authority.
- Go facts and FlowProof are static, build-scenario evidence, not absolute
  runtime truth.
- Python remains experimental and explicitly weaker where static resolution is
  uncertain.
- Surface discovery is default-on only for persisted Go artifact runs and keeps
  `--discover-surfaces=false`. Non-Go/no-debug/preview runs skip it, failures are
  warnings, and its configured-terminal scope is not whole-repository truth.
- A landscape no longer depends on FlowProof. When a run has no compatible
  proof, the canvas remains visible, shows no flow buttons, and explicitly says
  that its structural edges do not imply runtime sequence.

## What to do next

Run the complete friend onboarding journey on one real known repository using
the production command, not the preview fixture. Challenge:

1. whether the conceptual map names the responsibilities the engineer expects;
2. whether the first selected flow is useful;
3. whether exact evidence explains each transition;
4. whether locally discovered surfaces reveal a better entrypoint than the
   model recommendations without pretending registration was execution;
5. which missing flow or frontier blocks understanding first.

Then improve the smallest demonstrated gap. Do not restart broad local-model
prompt tuning, add per-click model calls, or introduce a graph database.

## Read in this order

1. [CORE_IDEA.md](CORE_IDEA.md)
2. [MILESTONES.md](MILESTONES.md)
3. [SYSTEM_MAP.md](SYSTEM_MAP.md)
4. [design/architecture-canvas-v2.md](design/architecture-canvas-v2.md)
5. [design/surface-discovery-ui-handoff.md](design/surface-discovery-ui-handoff.md)
6. [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md)
7. [ENGINEER_TRIAL.md](ENGINEER_TRIAL.md)

## Verification

```bash
node --check internal/report/templates/architecture_canvas.js
node --check internal/report/templates/surface_catalog.js
go test ./internal/report ./cmd/canvas-preview ./internal/componentmap ./internal/deepseek ./cmd/repomap
make surface-check
go vet ./...
go build -trimpath -o /tmp/repomap ./cmd/repomap
/tmp/repomap ../etcd --offline --no-open --no-serve --debug-dir /tmp/repomap-etcd
```

No live provider request is required for normal verification.

## Workspace caution

The worktree contains other ongoing backend, Python, experiment, report-server,
and documentation work. Stage exact files or hunks only. Do not rewrite commits
created by the parallel backend task. Generated reports, provider artifacts, and
debug dumps remain outside version control.

The surface shelf is implemented and verified in the working tree, but its
report integration depends on the still-uncommitted authorized interactive-
report foundation (report format v11). Commit that foundation first; then commit
the surface projection and UI as the coherent v11-to-v12 increment. Do not stage
the surface files alone over the current `HEAD`, where their integration points
would otherwise be absent or dead.
