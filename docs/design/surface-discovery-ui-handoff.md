# Surface discovery UI handoff

Status: backend discovery experiment is implemented and available through an
opt-in normal repomap run. Default enablement and report UI rendering are **not
implemented yet**. This document is the handoff for that product/UI increment.

## Product intent

The report needs three visibly distinct collections:

1. **Discovered surfaces** — deterministic registrations and starts found by
   local Go analysis. These establish that a route, async task, or worker is
   statically registered under the recorded build scenario.
2. **Recommended flows** — the existing bounded model-assisted onboarding
   choices. A missing recommendation must never hide a discovered surface.
3. **Expanded flows** — the existing selected `FlowProof` investigations.

The immediate UI task is to expose collection 1 without changing collections 2
or 3. Do not blend a discovered trigger into the architecture canvas as if it
were already a complete runtime flow.

## What exists now

The analyzer lives under:

- `internal/experiment/surfacediscovery`
- `internal/semantics/catalog`
- `cmd/surface-discovery-playground`
- `internal/surfacebridge`

Normal repomap integration is currently opt-in:

```bash
go run ./cmd/repomap /path/to/repo \
  --offline \
  --discover-surfaces \
  --no-open \
  --no-serve
```

When an artifact run is enabled, `internal/orient` writes these files beside
`snapshot.json`, `report.json`, and `report.html`:

- `trigger_catalog.json`
- `surface_coverage.json`
- `semantic_summaries.json`
- `surface_summary.md`

The browser report does not currently read or embed them. Specifically:

- `internal/report.ReadRunDir` does not parse the surface artifacts;
- `report.ReportData` has no discovered-surface DTO;
- `templates/script.js` has no surface renderer;
- surface evidence paths are not yet added to `openable_paths`/run authority;
- the `--discover-surfaces` flag currently defaults to `false`.

## Discovery basis

The versioned semantic catalog describes terminal operations, not public helper
methods or repository wrappers.

Implemented seeds:

- `net/http` route registration and server/dispatcher roots;
- `github.com/gin-gonic/gin.(*RouterGroup).Handle`;
- `golang.org/x/sync/errgroup.(*Group).Go`.

SSA propagation derives library and repository wrappers. For example:

```text
registerJSON
  -> (*gin.RouterGroup).GET
  -> (*gin.RouterGroup).Handle      configured terminal
```

and:

```text
registerTasks
  -> (*errgroup.Group).Go(runWorker) configured terminal
  -> runWorker contains channel receive loop
  -> worker registration
```

Framework/application wrapper names are not configured.

## Artifact versions

Current values:

```text
TriggerCatalog.version: 2
TriggerCatalog.analyzer_version: surface-ssa-v2
TriggerCatalog.catalog_version: 1
SurfaceCoverage.version: 2
```

The UI should treat a missing catalog as a valid legacy run. Unsupported future
versions should produce a bounded report warning and omit the section rather
than breaking the whole report.

## `trigger_catalog.json`

Top-level shape:

```json
{
  "version": 2,
  "analyzer_version": "surface-ssa-v2",
  "catalog_version": 1,
  "repository": {
    "root": "/absolute/repository/path",
    "module_path": "example.com/app"
  },
  "scenario": {
    "id": "go:darwin/amd64:tags=",
    "goos": "darwin",
    "goarch": "amd64",
    "tags": []
  },
  "triggers": []
}
```

Relevant `TriggerRecord` fields:

| Field | Meaning |
| --- | --- |
| `id` | Stable local opaque ID. Never derive UI order or display name from it. |
| `provisional_id` | Identity includes unresolved/dynamic values and may change after grounding improves. |
| `kind` | Currently `http_route`, `worker`, or `async_task`. |
| `identity` | HTTP `method`/`path`, or worker/task `name`. |
| `transport` | `http`, `https`, or `in_process`. |
| `framework` | Currently `net/http`, `gin`, or `errgroup`. |
| `process_entrypoint` | Exact composition entrypoint symbol and location. |
| `dispatcher` | Mux/router/group identity when statically available. |
| `registration_site` | Exact terminal registration/start callsite. |
| `server_start_site` | Optional exact HTTP server start site. |
| `handler` | Registered handler/callback identity. |
| `middleware` | Separate middleware identities; never flatten into handler. |
| `wrapper_chain` | Derived library/repository wrappers with declaration and callsite locations. |
| `final_seed` | Configured terminal semantic seed ID. |
| `discovery_basis` | `catalog_static` or `wrapper_static`. |
| `certainty` | Currently `static`; this is not observed runtime behavior. |
| `resolution` | `exact`, `ambiguous`, or `dynamic`. |
| `scenario_id` | Build scenario under which the static result was produced. |
| `evidence` / `provenance` | Local operation/location and analyzer identity. |
| `dynamic_frontier` | Values or targets intentionally left unresolved. |
| `status` | Precise local classification described below. |

### HTTP example

Abbreviated:

```json
{
  "id": "trigger-68bcf830e521d0e67d5a5532",
  "provisional_id": false,
  "kind": "http_route",
  "identity": {
    "path": {"kind": "constant", "text": "/health", "known": true, "candidates": []}
  },
  "framework": "net/http",
  "registration_site": {"path": "main.go", "line": 9, "column": 16},
  "server_start_site": {"path": "main.go", "line": 10, "column": 25},
  "handler": {
    "kind": "function",
    "text": "example.com/app.healthHandler",
    "known": true,
    "candidates": []
  },
  "wrapper_chain": [],
  "final_seed": "net-http-servemux-handlefunc",
  "discovery_basis": "catalog_static",
  "certainty": "static",
  "resolution": "exact",
  "status": "confirmed_direct_registration"
}
```

### Worker example

Abbreviated:

```json
{
  "kind": "worker",
  "identity": {"name": "example.com/workers.runWorker"},
  "transport": "in_process",
  "framework": "errgroup",
  "handler": {
    "kind": "function",
    "text": "example.com/workers.runWorker",
    "known": true,
    "candidates": []
  },
  "final_seed": "x-sync-errgroup-go",
  "certainty": "static",
  "resolution": "exact",
  "status": "confirmed_worker_registration",
  "evidence": [
    {"kind": "async_task_start"},
    {"kind": "channel_receive_loop"}
  ]
}
```

`confirmed_worker_registration` means both facts are statically supported:

1. the callback is passed to a configured async-start terminal;
2. the resolved callback contains an event-loop shape.

It does **not** mean the callback executed, the loop is infinite, cancellation
is correct, or a particular event/branch was observed.

### Current statuses

- `confirmed_direct_registration`
- `confirmed_through_library_wrapper`
- `confirmed_through_repository_wrapper`
- `dynamic_unknown`
- `confirmed_async_task_start`
- `possible_worker_loop`
- `confirmed_worker_registration`

Suggested user-facing labels:

| Status | UI label |
| --- | --- |
| confirmed direct/wrapper registration | Confirmed registration |
| dynamic unknown | Dynamic registration |
| confirmed async task start | Async task |
| possible worker loop | Possible worker |
| confirmed worker registration | Worker registration |

Always show certainty/resolution separately from the friendly status.

## `surface_coverage.json`

Coverage explains the bounds of the catalog. Relevant fields:

```json
{
  "version": 2,
  "entrypoints_considered": [],
  "dispatch_roots_found": 0,
  "configured_seeds_matched": [],
  "packages_inspected": 0,
  "functions_inspected": 0,
  "direct_triggers": 0,
  "wrapper_derived_triggers": 0,
  "unresolved_handlers": 0,
  "possible_registrations": 0,
  "workers": 0,
  "async_tasks": 0,
  "loop_signals": [],
  "dynamic_frontiers": [],
  "budgets_reached": [],
  "scope_statement": "runtime registrations and starts found through configured terminal seeds and bounded wrapper propagation under the recorded build scenario, subject to listed frontiers"
}
```

Never replace `scope_statement` with “all routes/workers found.”

### Loop signals

Current kinds:

| Kind | Meaning |
| --- | --- |
| `registration_loop` | A configured registration sink occurs inside a control-flow loop; cardinality may be dynamic. |
| `channel_receive_loop` | A callback control-flow loop contains a channel receive. |
| `select_event_loop` | A callback control-flow loop contains a `select`. Runtime branch/cancellation remain unproven. |
| `control_flow_loop` | A bounded SSA cycle without a stronger event shape. |

A loop alone never creates a trigger. Loop signals should appear as evidence or
frontier context, not as standalone workers.

## Suggested report DTO

Do not expose the full experimental structs directly to presentation code.
`internal/report` can parse a bounded projection similar to:

```go
type DiscoveredSurfaces struct {
    Version         int                 `json:"version"`
    AnalyzerVersion string              `json:"analyzer_version"`
    ScenarioID      string              `json:"scenario_id"`
    ScopeStatement  string              `json:"scope_statement"`
    DirectCount     int                 `json:"direct_count"`
    WrapperCount    int                 `json:"wrapper_count"`
    WorkerCount     int                 `json:"worker_count"`
    AsyncTaskCount  int                 `json:"async_task_count"`
    Triggers        []DiscoveredTrigger `json:"triggers"`
    LoopSignals     []SurfaceLoopSignal `json:"loop_signals"`
}
```

The projection should keep only display/evidence fields, enforce artifact size
and trigger-count bounds, and sort by stable trigger ID. Missing artifacts are
not warnings for older reports.

## Suggested UI

Add a separate overview card immediately after “Architecture & flows”:

```text
Discovered surfaces                         Local static analysis

[ 12 HTTP routes ] [ 3 workers ] [ 2 async tasks ] [ 4 dynamic frontiers ]

Filter: All | HTTP | Workers | Async tasks | Dynamic

GET /health                         net/http · exact
  healthHandler
  registered main.go:42

runQueueConsumer                    errgroup · static
  Worker registration
  channel receive loop · worker.go:81

Scope: runtime registrations and starts found through configured terminal...
```

Minimum card behavior:

- collapsed summary remains useful with many triggers;
- filters operate locally and do not remove data from `report.json`;
- HTTP primary label is `METHOD path` or `<dynamic route>`;
- worker/task primary label is `identity.name`, shortened visually but retained
  in full through title/details;
- show framework, friendly status, certainty, and resolution;
- show handler/callback and exact registration location;
- show middleware separately;
- show wrapper chain inside expandable details;
- show dynamic frontiers and loop signals without alarming “error” styling;
- exact paths become editor links only when included in `openable_paths` and
  authorized by `run_manifest.json`;
- no model-confidence badge: these are local static facts, not model claims;
- no arrows/edges should be invented from wrapper order or shared components.

The section must render safely with zero triggers. If analysis ran and found
none, show “No surfaces matched the configured terminal catalog under this
build scenario,” not “This repository has no routes or workers.”

## Backend/report integration required for the UI

1. Parse `trigger_catalog.json` and `surface_coverage.json` in
   `internal/report.ReadRunDir`.
2. Add the bounded presentation DTO to `report.ReportData` and bump report
   format version.
3. Add these paths to `openable_paths` before run-manifest generation:
   process entrypoint, registration, optional server start, wrapper declaration,
   wrapper callsite, evidence locations, and loop-signal locations.
4. Render the separate overview card in `templates/script.js` and add scoped CSS.
5. Keep legacy reports valid when surface artifacts are missing.
6. Add parser limits and tests for missing, malformed, unsupported-version, and
   oversized catalogs.
7. Update the HTML golden only after the frontend layout is intentional.

## Default repomap behavior requested

The desired next behavior is:

- ordinary Go artifact runs enable surface discovery by default;
- non-Go repositories do not invoke the Go analyzer and continue normally;
- `--no-debug` / request-preview modes do not fail because there is nowhere to
  persist/render the catalog;
- keep an explicit opt-out such as `--discover-surfaces=false` while measuring
  latency;
- a surface-analysis failure should be visible as a precise warning and should
  not discard an otherwise valid orientation report unless artifacts are
  explicitly required by a strict mode.

Current code does not yet satisfy these bullets: the flag defaults off and
`orient` currently treats an enabled discovery failure as fatal.

## No-invention and wording constraints

The UI must preserve these distinctions:

- registration/start is not callback execution;
- static is not observed;
- middleware registration is not middleware execution order;
- ambiguous interface targets are not unique targets;
- dynamic route identity must not become a concrete path;
- `select` does not prove which arm ran or that cancellation is wired correctly;
- missing configured-seed matches do not prove absence;
- conceptual components and model recommendations remain hypotheses;
- trigger existence, certainty, and resolution come from local analysis only.

## Fixtures and verification

Useful fixtures:

- `internal/experiment/surfacediscovery/testdata/direct`
- `internal/experiment/surfacediscovery/testdata/dynamic`
- `internal/experiment/surfacediscovery/testdata/wrappers`
- `internal/experiment/surfacediscovery/testdata/gin`
- `internal/experiment/surfacediscovery/testdata/workers`

Focused backend check:

```bash
make surface-check
```

Standalone artifacts:

```bash
make surface-playground \
  SURFACE_REPO=internal/experiment/surfacediscovery/testdata/workers \
  SURFACE_OUT=tmp/surface-workers
```

End-to-end report acceptance:

1. Run normal repomap on a temporary git copy of the worker fixture.
2. Confirm `report.json` embeds one `worker`, one `async_task`, and one
   `channel_receive_loop`.
3. Confirm the overview displays both triggers in “Discovered surfaces.”
4. Confirm the worker registration and loop locations open only through the
   manifest-authorized local server.
5. Confirm a legacy saved report without surface artifacts still renders.
6. Run `./scripts/check.sh` and `./scripts/etcd_check.sh ../etcd`.

## Relevant commits

- `1a7e4d5` — HTTP SSA discovery and deterministic artifacts
- `f76ccb5` — grouping replay and `FlowSeed` bridge
- `00ee2a3` — opt-in persistence beside report runs
- `0bb4501` — loop signals and errgroup worker classification
