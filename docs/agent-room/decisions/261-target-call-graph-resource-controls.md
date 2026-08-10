# 261 — Target call-graph resource controls

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-09)

**Preserves:** Decisions 250, 257, 258 and 260; one exact Go build scenario;
one selected package target; complete build-selected local declaration
identity; private DirectCallIndex authority; bounded refs-only Study requests;
and the existing absolute node/edge safety ceilings.

## Product defect

The DirectCallIndex previously had only an internal all-or-nothing resource
ceiling. A large target could fail after the local package load and SSA build
without telling the user how to make the exact same product analysis smaller
or explicitly authorize a larger graph. A blind `depth-1` suggestion is also
not acceptable: it can repeat the expensive local work and hit the same BFS
layer again.

Depth must affect the target call graph it names. It must not silently change
the separate runtime-surface/value walk or the final eight-edge Study trace.

## Approved controls

The ordinary command exposes:

- `--depth N`, default `10`, for exact repository-local call edges outward
  from the selected AnalysisTarget roots; and
- `--edges-limit E`, default `10000`, for the target-rooted exact edge set.

`N` must be positive. It has no invented semantic maximum: the finite complete
declaration set and the existing absolute edge ceiling already bound the work.
`E` must be positive and cannot exceed the existing absolute
`MaxDirectCallIndexEdges` safety ceiling.

Executable roots are the selected package's exact `main` declarations.
Library roots are every producer-owned exported function or method in the
selected package. All build-selected repository function declarations remain
indexed so public API identity and exact source actions do not disappear when
the edge neighborhood is reduced. Only edges are target-rooted and
depth-bounded.

The producer records the configured target kind/package, depth and edge limit
inside DirectCallIndex v2 and therefore inside its SHA-256. It counts
repository-local calls omitted at the depth frontier both globally and on the
exact boundary caller. A selected Study trace that lands on that caller retains
`depth_bound`; a missing target-to-reading connector is also `depth_bound`, not
`target_unreachable`, whenever the global frontier proves the graph was not
exhausted.

Readiness belongs to the selected target, not to any safe package in its import
closure. If the selected package itself is not SSA-safe, or an executable's
advertised exact `main` roots do not all resolve in SSA, surface discovery
returns a typed fatal target-admission error. The run tells the user to choose
another `--target` or correct `--go-target`; it cannot continue to provider
work on a safe sibling package.

## Closed edge-limit outcome

An edge-limit breach remains terminal before Architecture or Study provider
calls and retains no partial graph. Its local coverage records the exact
positive BFS depth known to fit: the overflowing caller layer is excluded at
that depth and all shallower layers were already complete. The CLI offers that
guaranteed lower-depth retry. When overflow occurs in the root layer, no
positive depth can help and the depth suggestion is omitted. A larger explicit
edge-limit alternative is also shown, without claiming that a guessed doubled
ceiling is guaranteed to exhaust the graph.

A retry rebuilds snapshot/package facts, `go list`, SSA and DirectCallIndex;
repomap has no persisted local-analysis resume artifact. Standard Go module,
build and export caches remain reusable. The failed ceiling path made no new
provider call. `--no-cache` controls model-response caches, not Go's caches.

## Evidence and acceptance

Provider-free measurement on the selected `cmd/repomap` target retained 6,988
declarations and produced these edge counts for depths 1 through 10:

`35, 213, 781, 2079, 3901, 5817, 7178, 7979, 8392, 8578`.

The default 10,000-edge ceiling therefore retains depth 10 with 1,422 edges of
headroom. Selected Linux `cmd/dockerd` retained 7,614 declarations and reached
only 37 edges by depth 7; its remaining frontier is static/dynamic resolution,
not this resource ceiling.

Permanent gates prove:

1. an edge-limit failure at depth two becomes a ready exact graph at the
   producer-recorded safe depth one;
2. every exported library function/method contributes a root while private
   functions remain declaration-only;
3. target/depth/edge identity changes the DirectCallIndex digest even after the
   reachable graph has exhausted;
4. a depth-truncated selected boundary function and a missing connector both
   retain `depth_bound` instead of looking exhausted or unreachable;
5. an unsafe selected package and a missing exact executable root fail even
   when another admitted package can build SSA; and
6. invalid flags fail before analysis while error copy names both real retry
   controls and exact cache/provider behavior.

No raw graph, canonical node ID, source body, new provider request, report
field, browser control or second SSA build is added.

Approved by:
    Repository owner after the target-rooted Study integration exposed the
    DirectCallIndex safety ceiling and explicitly requested actionable
    `--depth` / `--edges-limit` recovery plus honest retry-cache behavior,
    2026-08-09.
