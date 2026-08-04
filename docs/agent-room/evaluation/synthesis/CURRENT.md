# Final-local cross-fixture synthesis

**Decision:** `094-syncthing-surface-trace-product-fixture.md`  
**Production checkpoint:** `f6ae3cf71800a90c3fded480923b9f7ab092f6ca`  
**Product verdict:** **BLOCKED, but not by an external dependency.**

This synthesis uses the pinned Restic, Caddy, and Syncthing source oracles; the
three final-local run records and manifest-bound reports; all three fixture
verdicts; the semantic and performance audits; the active decision; and the
current diff. The manifests bind the reports to the oracle revisions and report
format 18. The worktree has no post-`f6ae3cf` production-code diff; its other
changes are OpenCode/workflow and evaluation material and do not alter the
product evidence.

## 1. Current product truth

1. **The old v6 producer/v5 consumer defect is fixed at `f6ae3cf`.** The current
   producer and report reader both accept surface artifact version 6. More
   importantly, the final-local reports actually contain v6 catalogs: Syncthing
   projects 45 records and Caddy projects 18. The semantic audit correctly
   recognizes this (`contracts/CURRENT.md:9-24`). Performance-audit statements
   that these reports rejected v6 or projected empty inventories are stale and
   must not drive another compatibility change.

2. **`f6ae3cf` made the analyzer-side Syncthing fallback materially better.** In
   `trigger_catalog.json`, exact primary ID
   `trigger-1866c3e977616ca13632e2ab` remains an `entry_surface` with
   `partial_trace_ready`, exact declaration/ownership quality, and the honest
   unavailable typed-closure reason. Focused analyzer, FlowProof, saved
   Syncthing replay, report-version, and persisted-canvas tests pass. The
   one-anchor proof behavior is therefore implemented below the report layer.

3. **The same primary record is corrupted at the report boundary.** The
   manifest-bound Syncthing `report.json` rewrites that exact ID to
   `surface_role: rejected`, `trace_readiness: rejected`, and rejects all quality
   dimensions. `internal/report` independently re-derives producer semantics,
   applies “unavailable means rejected” before considering the exact process
   declaration, and its coherence gate also excludes every unavailable record
   from traceable suggestions. This contradicts the catalog, the assembler, and
   decision 094. It is the immediate release blocker.

4. **Syncthing remains the severe fixture failure.** Its canonical command exits
   1 and the product has not yet demonstrated the required primary partial trace,
   suggestion/component membership, focused research, or browser journey. The
   report does usefully preserve 18 exact process declarations, primary versus
   six secondary-service versus tooling/helper roles, and scoped `auto.Assets`
   diagnostics. Those gains do not offset rejection of the primary.

5. **The Syncthing fixed aggregate oracle is not authoritative enough to explain
   the 45-record failure.** The script expects 36 records and preserves an old
   aggregate expectation of two rejected records without asserting that the
   exact primary is partial-ready. The source oracle does not
   establish exact totals of 12 routes and six server starts. The fixture verdict
   also says the nine-record excess is exactly the server category even though
   actual-versus-expected differs by six routes and three servers. The inventory
   therefore needs semantic membership review; changing 36 to 45, or deleting
   all nine server records, would be unsupported. The incompatible primary
   semantics remains a genuine blocker independently of this evaluation flaw.

6. **Caddy is a separate genuine analyzer failure.** The canonical check exits 1.
   Direct `addRouteWithMetrics` debug registrations survive, but constant
   `/config/`, `/id/`, and `/stop` registrations passed through `addRoute` into
   `addRouteWithMetrics` disappear. The report consequently has only six exact
   routes, 18 total records, and no supporting-dependency projection. Aggregate
   32/33-era totals are not sacred, but these three source-oracle registrations
   are exact and must survive.

7. **Restic has no final-local full-surface verdict.** Three full runs reached
   local discovery and were canceled by 180/600-second harness limits; the only
   completed report explicitly used `--discover-surfaces=false`. It proves that
   ordinary Cobra commands can still render, but cannot validate generic
   ownership, dependency exclusion, or full surface performance. Its missing
   build-selected `mount` command is a command-reader gap visible in the fallback,
   not evidence about the disabled generic analyzer.

8. **No genuine external blocker exists.** All fixture repositories and pinned
   revisions are locally available and clean; all final-local runs intentionally
   made zero provider calls; no provider outage, missing credential, missing
   source checkout, or inaccessible required service was encountered. The Restic
   timeout is a local harness/measurement and possibly product-boundedness issue,
   not an external blocker. The required fresh Syncthing provider/browser pass is
   intentionally deferred until deterministic truth is sound.

## 2. Shared root causes ranked by user impact

### 1. One cross-repository contract defect: two authorities derive surface semantics

**Impact: critical.** Surface discovery writes authoritative role, readiness,
reason, and quality, but report projection and product coherence recompute them
with a different rule set. This lets one persisted ID mean “exact partial entry”
in `trigger_catalog.json` and “rejected” in `report.json`, canvas, and suggestion
gates. The defect is generic to every current surface artifact even though the
unavailable Syncthing primary exposes it most clearly. It also explains why green
analyzer/assembler tests do not prove product behavior.

This is the one shared persisted-contract defect. The Caddy and count findings
below must not be folded into it.

### 2. Analyzer-specific gap: Caddy loses constants through a local wrapper boundary

**Impact: high.** The tightened target witness admits direct terminal calls but
does not preserve Caddy's path/handler values across `addRoute` delegation to
`addRouteWithMetrics`. The generic missing capability is bounded interprocedural
constant/callback propagation through an exact local wrapper, not knowledge of
Caddy route names. It explains the missing three admin routes, but not
Syncthing's report-semantic contradiction.

### 3. Presentation/model defect: listener starts are automatically promoted as entry surfaces

**Impact: high but not yet precisely counted.** Both analyzer and report semantic
switches classify every `http_server` with a start location as an independently
selectable `entry_surface`. In Syncthing this can make a listener lifecycle fact
appear beside the owning process and route as another user entry. A server start
should remain linked operational evidence or a frontier unless it establishes a
distinct selectable surface. This is a role/presentation defect; the current
fixed-count fixture does not prove which of the nine starts are duplicates.

### 4. Analyzer-specific command recall gaps

**Impact: medium.** Restic's separately registered, Darwin-selected `mount`
command is absent from the deterministic Cobra inventory. Syncthing's Kong
command tree is not covered by the Cobra reader. These reduce onboarding recall
but are independent of exact process survival and should not expand the immediate
batch.

### 5. Fixture/setup and measurement limitations

**Impact: acceptance evidence missing, not product truth established.** Offline
runs cannot exercise model directions, components, focused windows, saved
traces, or browser behavior. Restic's full run produced no durable report before
the harness canceled it. The missing owner-saved Restic path was correctly not
guessed. Browser/server latency and console behavior are unmeasured. These are
limitations, not permission to pass or to report an external blocker.

### 6. False positives and overstatements in the evaluation

- `performance/CURRENT.md` incorrectly says the final Syncthing/Caddy reports
  reject v6 and are empty. Their manifest-bound reports project v6 with 45 and 18
  records respectively. Performance conclusions based on empty reports are void.
- Its “external blocker: yes” conclusion is also incorrect under the workflow's
  blocker definition. A command-tool timeout is local and actionable.
- The Syncthing script's fixed 36/kind/readiness counts are stale semantic
  expectations, not a source oracle. In particular, its two-rejected aggregate
  does not verify the decision-required primary transition.
- The Syncthing verdict's assertion that all nine excess records are server
  starts is arithmetically unsupported: actual-versus-script differs by six
  routes and three servers.
- Caddy's exact missing `/config/`, `/id/`, and `/stop` routes are real, but old
  aggregate 32/33 and 21/11 totals should not be frozen after witness filtering.
- Unresolved process dispatch does not by itself make an exact repository-owned
  registration dependency-owned. It limits reachability/readiness authority;
  ownership and reachability must remain separate.
- Zero flows in an offline run is not a FlowProof regression. It leaves final
  acceptance untested. The concrete current defect is the report/coherence
  rejection of a seed that the catalog and assembler support.
- One slow or canceled Restic capture does not establish a performance
  regression. It does establish that the current evaluation has no valid full
  Restic comparison.

## 3. Single smallest generic implementation batch

### Preserve current producer semantics through report and coherence

Make the v6 producer's role, readiness, reason, and quality authoritative for a
generic surface record instead of unconditionally re-deriving them in report
projection, command-catalog merge, and product coherence. Legacy or report-built
records whose semantic fields are absent may still use one canonical fallback
derivation; malformed current fields must fail validation rather than be silently
rewritten.

Then narrow the suggestion traceability exception to exactly this case:

- `kind == process_entry`;
- exact, non-provisional, repository-local declaration location;
- application-owned `entry_surface` with `partial_trace_ready`;
- typed closure unavailable.

That record may seed only the existing one-anchor proof and must stop at the
“typed downstream closure unavailable” frontier. Unavailable routes, commands,
descriptors, activities, and typed transitions remain rejected/non-traceable.

This is one coherent contract correction: the analyzer, report JSON, canvas
suggestion, and already-implemented FlowProof assembler will agree on one ID. It
does not attempt the independent Caddy wrapper, server-role, Kong/Cobra,
performance, or UI work.

## 4. Direct tests and affected fixtures

### Direct tests

1. Add a producer-to-report v6 contract test with an exact unavailable process
   record and assert exact semantic preservation of ID, role, readiness,
   reason, and quality through projection and `ApplyProductCoherence`.
2. Change `TestSuggestionsKeepSourceAndTypedTraceAvailabilityDistinct` so the
   exact unavailable process is investigation-available **and** can start the
   bounded process trace; keep source-signal aggregates and runtime activities
   non-traceable.
3. Add negative cases proving unavailable routes/commands and malformed or
   provisional process records cannot use the exception.
4. Keep the existing analyzer partial-load, FlowProof unavailable-process, saved
   Syncthing replay, v2-v6 reader, and persisted-canvas tests.

Focused checks:

```text
go test ./internal/report -run 'Test(ParseDiscoveredSurfaces|ProjectDiscoveredSurfaces|SuggestionsKeepSourceAndTypedTraceAvailabilityDistinct|.*UnavailableProcess.*)' -count=1
go test ./internal/experiment/surfacediscovery -run 'TestAnalyzeIsolatesIllTypedExecutableAndKeepsExactProcessEntries' -count=1
go test ./internal/flowproof/assemble -run 'Test.*(Process|Unavailable)' -count=1
go test ./internal/orient -run TestReplaySavedSyncthingOrientationSeedsPartialTracesWithoutProvider -count=1
go vet ./internal/report/... ./internal/experiment/surfacediscovery/... ./internal/flowproof/assemble/... ./internal/orient/...
```

The focused report, analyzer, assembler, and saved-orientation tests were run
during this synthesis and pass at `f6ae3cf`; the new report-contract assertions
are what must turn the currently hidden contradiction red.

### Affected fixtures

1. **Syncthing — mandatory immediate rerun.** The same primary surface ID must be
   partial-ready in catalog, report, canvas/suggestion, and one-anchor proof while
   preserving `auto.Assets` diagnostics. Replace fixed aggregate acceptance with
   semantic membership assertions before treating its canonical script as an
   oracle.
2. **Core persisted canvas fixtures — direct regression.** Colima runtime v2,
   Restic backup v2, and Soft-Serve daemon v2 must retain exact surface/trace
   membership and branching.
3. **Caddy — affected by shared projection but still expected to fail its
   independent wrapper recall.** Do not make this contract batch recover its
   three routes accidentally or relax their oracle.
4. **Restic — no new full run is required to prove this semantic correction.**
   Run its full regression after the independent analyzer/boundedness work; the
   no-surface fallback cannot close it.

## 5. Superficial fixes to reject

- Do not make the analyzer reject the exact Syncthing primary merely to agree
  with the report.
- Do not copy the producer fix into a second report switch and leave two semantic
  authorities; preserve/validate the current contract or share one derivation.
- Do not permit every unavailable record to seed a trace.
- Do not suppress `auto.Assets`, mark the typed closure available, or invent a
  downstream transition.
- Do not patch JSON/HTML after projection, hide the record, or repair only the
  headline count.
- Do not change Syncthing's expected total from 36 to 45 and call it fixed; nor
  delete all server records to recover 36 without membership evidence.
- Do not restore Caddy's historical totals by weakening target witnesses or
  treating signature/CHA possibility as exact delegation.
- Do not increase Restic's harness timeout and call the fixture passed without a
  completed, manifest-bound full report.
- Do not add fixture-specific names, paths, route strings, or executable rules.
- Do not spend the fresh provider/browser run before deterministic artifacts
  agree.

## 6. Deferred independent findings

1. Add bounded exact-wrapper value propagation for Caddy's `addRoute` delegation,
   then assert the three admin registrations and honest wrapper evidence without
   freezing aggregate totals.
2. Define when an HTTP server start is a distinct selectable surface versus
   linked process/route activity; recompute Syncthing membership from that rule
   and rewrite the fixture assertions around semantic IDs and roles.
3. Complete a bounded/resumable full Restic surface run and investigate why local
   discovery exceeded 600 seconds before making performance claims.
4. Fix Restic's build-selected `mount` registration with a direct build-tagged
   Cobra fixture.
5. Consider generic Kong command evidence only if corrected Syncthing
   orientation still cannot present its CLI journey.
6. Revisit architecture archetype labels and absolute dependency provenance only
   after admitted surface evidence is correct.
7. Improve partial/offline presentation so disabled discovery and skipped model
   orientation cannot look like verified zero coverage.
8. After deterministic Syncthing, Caddy, Restic, and repository checks pass,
   perform the required one fresh bounded Syncthing provider run, served
   Playwright journey, console review, screenshots, and exact
   suggestion/surface/trace/component/focused-window audit.
9. Repeat same-host performance captures only after correctness; do not use the
   stale final-local performance comparisons as a baseline.
