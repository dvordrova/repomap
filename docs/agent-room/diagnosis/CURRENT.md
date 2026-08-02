# Diagnosis: target-specific witness admission in surface discovery

**Blocker:** post-`bec3d3e` CHA/dynamic callback candidates are still promoted to
static wrapper paths without a callsite-specific target witness.

**Scope:** only `internal/experiment/surfacediscovery` edge admission, the
retained Restic/Caddy artifacts, and the exact detached Caddy admin
registration case. No production code or tests were changed.

Retained artifact roots used below:

```text
post-edge Restic: /Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-restic/runs/20260714-164750-restic
post-edge Caddy:  /Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-caddy/caddy-20260714T-post-edge-873fac5f/product/20260714-165101-caddy
pre-edge Caddy:   /Users/dvordrova/Library/Caches/repomap/evaluation/initial-caddy/caddy-20260714T000000Z-873fac5f/product/20260714-162702-caddy
```

## Observed contradiction

`bec3d3e` added an executable import-closure gate and correctly stopped the
demonstrated cross-main Restic helper hop. The same implementation still admits
any same-signature function in that closure and every CHA interface candidate:

- `callTargetWitness` returns true for every `IsInvoke` target and otherwise
  uses `types.AssignableTo(target.Signature, call.Common().Value.Type())`
  (`internal/experiment/surfacediscovery/analyzer.go:874-886`).
- `callTargetEligible` then checks only that the target package is recursively
  imported by the selected main (`analyzer.go:859-872`). Importability scopes a
  candidate to an executable; it does not show that this instruction selected
  that target.
- The same weak predicate now controls reverse relevance propagation
  (`analyzer.go:392-425,918-920`) and forward wrapper traversal
  (`analyzer.go:729-835`). A frontier is added after traversal, but the weak edge
  has already entered the wrapper chain, semantic summary, ownership inputs,
  and task budget.

The retained artifacts expose both consequences:

1. **Restic false static wrappers.** In post-edge Restic
   `trigger_catalog.json:375-408,964-997,1409-1442`, the callsite
   `internal/backend/retry/backend_retry.go:234:2` is serialized as
   `retry.(*Backend).Stat -> Azure ... local.start$1` and
   `retry.(*Backend).Stat -> golang.org/x/net/trace.init#1`. Their actual
   terminal calls are in unrelated dependency source. The corresponding
   summaries still say `certainty: "static"`
   (`semantic_summaries.json:2-44,146-193`). Coverage reports 2,589 omitted
   targets at this one callsite, meaning 2,597 targets before the configured
   limit of eight (`surface_coverage.json:331-339`).
2. **Caddy shutdown-to-start wrapper.** In post-edge Caddy, a retained server
   record contains
   `TrapSignals -> ... -> unsyncedStop -> NewContextWithCause$1 ->
   reverseproxy.(*Handler).Cleanup -> ... -> closeConnections -> run$2 ->
   caddyhttp.(*App).Start` (`trigger_catalog.json:4020-4254`). The first false
   hop in that suffix is exactly
   `reverseproxy.(*Handler).closeConnections` at
   `streaming.go:413:35 -> caddy.run$2` (`trigger_catalog.json:4201-4235`).
   The artifact nevertheless labels the resulting `app.go:619` server start
   `certainty: "static"` and `partial_trace_ready`
   (`trigger_catalog.json:4255-4328`).
3. **Exact Caddy registrations disappear while false paths consume the
   budget.** The post-edge catalog has 18 records, inspects exactly the
   1,500-function limit, reaches `depth`, `targets`, and `tasks`, and no longer
   matches `net-http-servemux-handle`
   (`surface_coverage.json:25-39,621-628`). The retained pre-edge artifact
   inspected 161 functions, reached only the target budget, and retained the
   exact `/config/`, `/id/`, `/stop`, and debug registrations at `admin.go:240`
   (`initial-caddy/.../surface_coverage.json:25-44,228-285` and
   `surface_summary.md:24-88,207-271,351-429`). The source is unchanged at the
   fixture revision.

The focused current tests do not detect this contradiction. The following
passed unchanged during diagnosis:

```text
go test ./internal/experiment/surfacediscovery \
  -run 'TestAnalyze(DoesNotCrossExecutableRoots|InterfaceTargets|ImportReachableDetachedHTTPComposition|CaddyAdminRouteProviders|DynamicAndNegativeControls|CrossPackageWrapper)' \
  -count=1
```

## Smallest reproducible case

The false edge needs only one dynamic function call and one unrelated function
with the same signature in the same executable closure:

```go
func caller(ctx context.Context) {
    _, cancel := context.WithCancel(ctx)
    defer cancel() // dynamic func() value
}

func decoy() { terminalRegistration() } // also func(), but never assigned to cancel
```

CHA puts both the real returned cancel closure and `decoy` in the `func()`
candidate set. The current signature check admits `decoy`; an import-closure
check cannot distinguish them.

Two positive controls delimit the safe correction:

```go
func exactNestedClosure() {
    inner := func(path string) { terminalRegistration(path) }
    outer := func(path string) { inner(path) }
    outer("/config/")
}

type Runner interface{ Run() }
func invoke(r Runner) { r.Run() }
func main() { invoke(realRunner{}) }
```

The nested call has a target-specific `MakeClosure.Bindings` witness even though
the call inside `outer` is through an SSA `FreeVar`. The interface call has a
target-specific receiver witness when the current traversal environment binds
`r` to `realRunner`. Neither case should be discarded merely because
`StaticCallee` is nil.

## Exact control and data flow

### Analyzer/CHA flow

1. `load` builds all safe packages and a whole-program CHA graph
   (`analyzer.go:219-306`). x/tools documents that CHA assumes all functions are
   address-taken and all concrete types enter interfaces, so it may include
   uninstantiated or unreachable callees
   (`golang.org/x/tools/go/callgraph/cha/cha.go:12-22`).
2. x/tools indexes every non-method function by signature and returns the whole
   signature bucket for a non-static function call; interface invokes return all
   concrete methods whose receiver implements the interface
   (`go/callgraph/internal/chautil/lazy.go:23-26,53-65,68-93`).
3. `targets` combines `StaticCallee`, an immediate `MakeClosure`, and all CHA
   edges for that instruction (`analyzer.go:1803-1839`).
4. `prepare` stores those candidates and propagates terminal relevance backward
   through `propagationTargetEligible` (`analyzer.go:349-458`). In `bec3d3e`, that
   predicate changed from exact-or-small-bounded-target-set to the current
   signature/blanket-invoke predicate. Consequently large weak candidate sets
   can now make callers relevant and rank terminal-bearing decoys first.
5. `walk` limits the already-ranked candidates, calls `callTargetEligible`, and
   immediately appends each admitted target to the static wrapper chain before
   recursing (`analyzer.go:729-835`). `recordSummary` persists that chain with
   `certainty: "static"` (`analyzer.go:1891-1924`).

### Restic `retry.(*Backend).Stat`

The source is unambiguous: `context.WithCancel` returns `cancel`, followed by
`defer cancel()` at `backend_retry.go:233-234`. Direct SSA inspection during
this diagnosis produced `defer t10()` for that instruction and these properties:

| Property | Observed value |
|---|---|
| SSA instruction | `*ssa.Defer`, at `backend_retry.go:234:2` |
| `StaticCallee` | `nil` |
| call value | `*ssa.UnOp` loading the spilled local allocation named `cancel`; the preceding SSA value is `extract t7 #1` |
| immediate `MakeClosure` | no |
| `IsInvoke` | false (`CallCommon.Method == nil`) |
| static type | `context.CancelFunc`, underlying `func()` |
| CHA result | the `func()` bucket, 2,597 candidates at this site |
| executable import closure | true for the retained Azure and x/net targets; Azure is imported through Restic's build-selected backend registry, and the artifact proves both passed `callTargetEligible` |

The actual returned value is the closure returned at Go's
`context.WithCancel` implementation (`context/context.go:240-243` in the
recorded toolchain). No SSA value, store, argument, closure binding, or return
origin at the Restic callsite names Azure `start$1` or x/net `init#1`. Their only
connection is the CHA signature bucket. The Azure closure really serves an HTTP
server, but only from `local.(*Server).start` in its own dependency source
(`.../apps/internal/local/server.go:117-126`); x/net registers its debug routes
from its own `init` (`x/net/trace/trace.go:120-131`).

### Caddy `TrapSignals` path

The prefix is not the defect. `cmdRun -> TrapSignals ->
trapSignalsCrossPlatform -> trapSignalsCrossPlatform$1 ->
exitProcessFromSignal -> exitProcess -> Stop -> unsyncedStop` is supported by
direct calls or the immediately launched closure
(`cmd/commandfuncs.go:188-200`, `sigtrap.go:29-59`, `caddy.go:694-741,760-783`).

There are then three different dynamic-edge classes that the current boolean
collapses:

1. `ctx.cfg.cancelFunc(...)` at `caddy.go:741` is a non-invoke function-value
   call. `NewContextWithCause$1` is a plausible real target because
   `provisionContext` stores the returned `cancelCause` into that field at
   `caddy.go:504-521`. That store/return flow is a potential target-specific
   witness, but the analyzer does not collect or persist it; signature equality
   alone currently admits the target.
2. `cu.Cleanup()` at `context.go:85` and `a.Start()`/`a.Stop()` at
   `caddy.go:447,734` are interface invokes. The concrete Caddy methods are
   type-valid dynamic candidates, but CHA alone does not prove which runtime
   module value occupies the map/interface at that call.
3. `oc.gracefulClose()` at `reverseproxy/streaming.go:413` is a non-invoke
   function-field call of type `func() error`. It has nil `StaticCallee`, no
   immediate `MakeClosure`, and no callsite value origin naming `caddy.run$2`.
   The field is populated at `streaming.go:194-203` from the local
   websocket-close factory (or nil), not from Caddy's startup function.
   CHA includes `run$2` solely because the anonymous startup function at
   `caddy.go:444-463` also has signature `func() error`; both are in the same
   executable import closure. This is the first unsupported edge that turns the
   retained shutdown/cleanup path into a startup/server path.

### Exact detached Caddy registration

Caddy's standard admin registrations do not require the false shutdown edge.
The useful chain is independently source-specific:

```text
cmdRun (Cobra dispatch unresolved)
  -> Load -> changeConfig -> unsyncedDecodeAndRun -> run
  -> finishSettingUp -> replaceRemoteAdminServer
  -> (*AdminConfig).newAdminHandler
  -> addRoute / addRouteWithMetrics
  -> (*http.ServeMux).Handle at admin.go:240
```

The source anchors are `cmd/commandfuncs.go:252-283`,
`caddy.go:136,247,363,475,594-605`, `admin.go:510-540`, and
`admin.go:234-273`. The old `/config/` record preserves the same exact calls and
terminal evidence (`initial-caddy/.../trigger_catalog.json:5828-6097`). Its
frontier correctly says only that dispatch from the process entrypoint is
unresolved; that does not invalidate the registration literal and exact
`ServeMux.Handle` call.

One dynamic-looking step in this chain is important: `addRoute` is
`newAdminHandler$2`, and its call to captured `addRouteWithMetrics` is through an
SSA `FreeVar` at `admin.go:257`. x/tools defines the enclosing
`MakeClosure.Bindings` as the values corresponding to the callee's `FreeVars`
(`go/ssa/ssa.go:789-805`). That binding, followed through its captured
allocation/store when necessary, identifies `newAdminHandler$1` and is a real
target-specific witness. The current `bind` function maps only parameters, not
closure free variables (`analyzer.go:1622-1637`), so replacing the signature
fallback with `StaticCallee` alone would incorrectly lose this registration.

The evidence strongly supports weak-path budget starvation as the immediate
post-edge reason these exact routes disappear: the current run reaches exactly
1,500 inspected functions and emits long TrapSignals-derived chains, while the
pre-edge run reached the registration after 161 functions. A per-function task
trace was not retained, so the precise final instruction at budget exhaustion
is not proven.

## Root cause

The root cause is not CHA itself and not the executable boundary. It is the use
of one boolean for two different facts:

- **compatibility/scope:** CHA edge, assignable signature, interface
  implementation, and target package in an executable's import closure; versus
- **target identity at this source instruction:** the call value or invoke
  receiver resolves, through bounded SSA value flow, to this target.

`callTargetWitness` currently treats the first category as the second. Because
the predicate is used during reverse relevance as well as forward traversal,
the error both fabricates static paths and allows those paths to starve exact
work. A later `entrypoint_dispatch_unresolved` frontier describes uncertainty
but does not retract the already-persisted static wrapper.

## Smallest generic witness criterion

> A call edge may contribute an authoritative wrapper, static summary,
> executable handoff, ownership, or architecture relationship only when the
> callsite's SSA function value or invoke receiver, evaluated under the current
> bounded traversal environment, identifies that target (or a finite
> source-derived candidate set containing it). `StaticCallee` is the trivial
> case. Signature equality, CHA membership, address-taken status, target budget,
> and import closure are compatibility/scope facts only.

The smallest sufficient authoritative witness classes are:

1. **Exact static identity:** `call.Common().StaticCallee() == target`. In
   x/tools this already includes an immediately applied `MakeClosure`.
2. **Bound function-value identity:** bounded def-use evaluation of
   `call.Common().Value` reaches the target through identity-preserving SSA
   nodes, current parameter bindings, a witnessed store/load or return extract,
   or `MakeClosure.Bindings -> Function.FreeVars`. A unique origin is exact; a
   finite source-derived set is ambiguous/possible but still target-specific.
3. **Bound invoke receiver identity:** bounded evaluation of the invoke receiver
   reaches a concrete dynamic type (or finite source-derived type set), and
   method selection on that type yields the target. Merely implementing the
   interface is not enough.

An edge supported only by CHA remains useful but is **possible dispatch**, not
an authoritative wrapper. It may produce a bounded frontier or detached
possible inventory, but it must not enter static wrapper paths, process
ownership, trace readiness, summaries, or architecture relationships. This
preserves real type-valid callbacks without claiming that the source selected
them. Exact terminal registrations discovered independently inside such code
may remain inventory with unresolved reachability and no fabricated process
handoff.

Applied to the evidence:

- Restic's `defer cancel()` has no value origin for Azure/x-net, so those edges
  become one bounded unresolved dynamic-call frontier.
- Caddy's `closeConnections -> run$2` has no value origin and is removed from
  the static path.
- Caddy's field-stored cancel callback can be retained only if the
  `NewContextWithCause` return/store/load chain is actually resolved; otherwise
  it remains possible.
- Concrete interface arguments in the existing `interface_single` and
  `interface_multiple` fixtures can be retained through receiver bindings.
  Reflection/map-selected Caddy app callbacks remain possible unless a bounded
  concrete receiver origin is found.
- The detached Caddy admin registration survives through its exact direct calls
  and captured-closure binding, while the unresolved Cobra/process handoff
  remains a frontier.

## Rejected alternative explanations and fixes

- **Cross-executable contamination:** rejected as the explanation for these
  hops. Both false targets pass the new import closure; the Caddy decoy is in
  the same module, and Restic intentionally imports the Azure dependency.
- **The terminal registrations themselves are false:** rejected. Azure and
  x/net contain the cited terminal calls; the false claim is that the Restic
  defer reaches them. Caddy `app.go:619` is also a real start site; the false
  claim is the shutdown-to-start wrapper.
- **Raise `MaxTargets`, `MaxTasks`, or reorder candidates:** rejected. This
  increases or hides weak work and does not create a target witness.
- **Use signature equality plus an uncertainty frontier:** rejected. The
  persisted edge and summary remain static, and reverse relevance still
  consumes the budget before the frontier is attached.
- **Keep every CHA invoke as authoritative:** rejected. CHA proves interface
  compatibility across the whole program, not the dynamic type at this source
  value. Unresolved invokes should remain possible rather than disappear.
- **Require only `StaticCallee`:** rejected. It would lose valid captured
  closure calls such as Caddy `newAdminHandler$2 -> newAdminHandler$1`, plus
  source-resolved interface and stored callbacks.
- **Treat any function reference/address-taken function as a witness:**
  rejected. CHA already assumes all functions are address-taken; that fact is
  not callsite-specific.
- **Delete all detached or dependency records:** rejected. Exact Caddy admin
  registration inventory and independently exact dependency terminals remain
  useful when their reachability/ownership is honestly bounded.
- **Adopt whole-program RTA/VTA now:** not required for the demonstrated
  correction. A bounded value-origin classifier at the existing callsite is the
  smaller change; reflection/config-selected callbacks may still remain
  unresolved.
- **Filter only final JSON/summaries:** rejected. Weak edges already affect
  reverse relevance, traversal budget, ownership, and architecture inputs.

## Smallest safe correction

1. Replace the boolean witness with a small witness classification: exact
   static, bound function value, bound invoke receiver, CHA-only possible, or
   none. Keep the executable import closure as a second scope gate, never as
   identity evidence.
2. Pass the current traversal environment into forward edge admission. When
   entering a `MakeClosure`, bind each callee `FreeVar` from the corresponding
   `MakeClosure.Bindings` value in addition to binding parameters. Reuse only
   exact, bounded store/load and return origins; do not infer identity from a
   signature.
3. Allow only target-specific classes to propagate authoritative relevance and
   relevance distance. Process CHA-only candidates later or in a separate,
   smaller possible-inventory budget so they cannot starve exact paths.
4. Build wrappers, summaries, terminal ownership, and architecture
   relationships only from the admitted target-specific edge set. Persist the
   witness kind and source location (using existing evidence/provenance fields
   if sufficient) so generated artifacts can be checked.
5. For CHA-only candidates, emit one bounded source-call frontier. If an exact
   registration is recovered independently, leave its identity exact but its
   reachability unresolved, process handoff absent, and trace readiness
   unsupported.

This is a bounded def-use correction, not a new repository-wide analysis layer.

## Direct tests and fixture checks required

### Analyzer tests

1. Add `TestAnalyzeRejectsSameSignatureDecoy` in one executable: a real callback
   and an unrelated terminal-bearing callback have the same signature. Only a
   source-bound target may enter a wrapper; an unknown value produces a
   frontier. Assert the decoy is absent from triggers, summaries, and grounding.
2. Add a Restic-shaped `defer` case where the second result of a constructor is
   called and an unrelated `func()` contains a terminal. The unrelated target
   must not be admitted solely from CHA/signature compatibility.
3. Add `TestAnalyzeBindsCapturedClosureFreeVar` using nested
   `addRoute`/`addRouteWithMetrics` closures. Assert the exact route and terminal
   call survive, and the wrapper transition cites the closure binding rather
   than signature compatibility.
4. Strengthen `TestAnalyzeInterfaceTargets`:
   - a concrete argument-bound receiver retains its real method;
   - two concrete callsites retain their respective methods without admitting a
     third same-interface decoy;
   - an unresolved receiver remains a possible/frontier candidate and never a
     static wrapper or executable handoff.
5. Keep `TestAnalyzeDoesNotCrossExecutableRoots` and exact static/cross-package
   wrapper tests. Add an invariant that every persisted wrapper step, semantic
   summary path, and architecture relationship has a target-specific witness.
6. Add a budget-order regression: many CHA-compatible decoys before a later
   exact registration must not exhaust the authoritative task budget or hide the
   exact registration.

Focused command after implementation:

```text
go test ./internal/experiment/surfacediscovery \
  -run 'TestAnalyze(DoesNotCrossExecutableRoots|RejectsSameSignatureDecoy|RejectsUnboundDeferTarget|BindsCapturedClosureFreeVar|InterfaceTargets|CrossPackageWrapper|ImportReachableDetachedHTTPComposition)' \
  -count=1
```

### Restic retained-fixture check

Regenerate offline analyzer artifacts at the pinned Restic revision and assert:

- no wrapper, summary, or grounding path contains
  `retry.(*Backend).Stat -> Azure ... start$1` or
  `retry.(*Backend).Stat -> x/net/trace.init#1`;
- `backend_retry.go:234` has a bounded unresolved/possible dynamic-call frontier
  unless the actual context cancel closure is source-resolved;
- the helper executables remain tooling-owned and the existing cross-main
  negative control remains fixed;
- no assertion freezes total counts.

### Caddy retained-fixture check

Use the focused Caddy surface fixtures and a direct built-binary run at the
pinned revision. Assert:

- no static wrapper contains `streaming.go:413 -> caddy.run$2`, and no server
  start inherits a `TrapSignals -> ... -> closeConnections -> run$2` chain;
- type-valid unresolved app/module callbacks remain possible/frontier evidence,
  not fabricated static paths;
- exact `/config/`, `/id/`, `/stop`, and debug registrations at `admin.go:240`
  survive with `net-http-servemux-handle` and exact registration identities;
- their detached/process dispatch remains explicitly unresolved and they do not
  become trace-ready merely because the terminal call is exact;
- exact returned admin descriptors may remain exact inventory while runtime
  provider selection remains a `route_provider_dispatch_candidate`;
- real server start sites survive at no stronger reachability than their
  admitted dynamic receiver witness supports;
- exact registrations are reached before any possible-candidate budget.

Finally run the focused Go tests and `go vet` for changed packages. No provider
call or browser run is needed for this blocker diagnosis or its deterministic correction.

## Risks

- A resolver that follows arbitrary aliases, maps, reflection, or all stores
  would recreate whole-program imprecision. Stop at a bounded unsupported
  frontier instead.
- Assignment evidence must be scoped to the current traversal/executable.
  Merging global stores from independent mains or traversal orders would
  reintroduce the same bug under a different name.
- `Phi` nodes and multiple exact assignments need finite candidate semantics;
  they must not be upgraded from ambiguous to exact.
- Closure `FreeVars` and `MakeClosure.Bindings` are positional. A mismatched or
  partially resolved binding must fail closed.
- CHA-only possible traversal needs a separate/lower-priority budget or it can
  still starve exact discovery even if final labels are corrected.
- Existing artifacts cannot demonstrate the correction; Restic and Caddy must
  be regenerated and semantically inspected.

## Explicit non-goals

- Fixing the separate surface artifact v6/report-reader v5 mismatch.
- Changing Syncthing partial-load process-entry semantics.
- Adding Kong/general command discovery or Restic `mount` recall.
- Redesigning the UI, architecture archetype selection, or surface totals.
- Proving runtime execution, callback order, listener lifetime, or
  configuration-selected module identity.
- Preserving old Caddy/Restic counts or deleting all dependency inventory.
- Adding repository-specific function names, paths, or exceptions to production
  code.
- Introducing whole-repository pointer analysis, RTA, or VTA in this correction.
