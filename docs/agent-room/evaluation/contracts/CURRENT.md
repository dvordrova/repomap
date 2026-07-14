# Semantic-contract audit — producer-semantics Syncthing

**Decision:** `094-syncthing-surface-trace-product-fixture.md`  
**Checkpoint audited:** `21fe3b956134233925fc47cf4ab139444b7d616a`  
**Current artifact:** manifest-bound Syncthing run
`20260714-211633-syncthing` under
`/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/`.  
**Verdict:** **BLOCKED for decision-094 acceptance; the prior primary-record
semantic contradiction is fixed.**

## Verified producer-to-report contract

The primary ID `trigger-1866c3e977616ca13632e2ab` now agrees across the
producer catalog and current `report.json`:

| Field | Catalog | Report |
| --- | --- | --- |
| `kind` / entrypoint | `process_entry`; `cmd/syncthing/main.go:212` | same |
| ownership | `cmd/syncthing`; `primary_application` | same |
| closure state | `availability: unavailable` with the ill-typed closure reason | same |
| role/readiness | `entry_surface`; `partial_trace_ready` | same |
| quality | `identity: exact`, `registration_start: not_applicable`, `handler_callback: not_applicable`, `reachability: partial`, `ownership: exact`, `traceability: partial_trace_ready` | same |

Exact evidence: `trigger_catalog.json:508-591` and
`report.json:4946-5034`. The report also projects the catalog as v6 with 45
records (`report.json:4224-4255`), while its run metadata records the same 45
surface discovery result (`metadata.json:10-18`). Thus the old claim that this
advisory `lib/api` `undefined: auto.Assets` diagnostic fatally rewrites the
exact primary seed is obsolete. The scoped diagnostic remains visible in
`surface_summary.md:557-569`; it does not erase the seed.

The implementation matches the artifact: the producer gives an exact
unavailable process entry the one-anchor partial semantics
(`internal/experiment/surfacediscovery/semantics.go:52-67`); report projection
copies complete producer semantics and only derives legacy/incomplete records
(`internal/report/surface_semantics.go:14-45`,
`internal/report/surfaces.go:733-778`); coherence permits only that exact,
non-provisional, application-owned unavailable process exception
(`internal/report/coherence.go:913-932`). This prevents an advisory diagnostic
from becoming fatal while retaining rejection for other unavailable records.

## What this capture cannot establish

This was an offline run with `flows: 0` (`metadata.json:15-25`):
`candidate_flows` and `flows` are null (`report.json:5-7`) and `flow_count` is
zero (`report.json:10624`). No provider request, response, or cache was read or
written (`runs/syncthing.md:51-55`). Therefore there is no current artifact in
which a suggestion, evidence bundle, component relation, exact location, or
static reachability could have been counted as a saved trace. Likewise, there
is no trace evidence set on which to verify seed membership, anchor/transition
requirements, canvas membership, rendered trace header, focused-window policy,
or stale cache replay. These remain unexercised acceptance conditions, not
failures demonstrated by this run.

## Remaining blockers, ranked by cross-repository impact

### 1. Decision-094 trace/product contract remains unexercised

**Impact: critical.** Decision 094 requires an evidence-backed partial trace,
exact suggestion/component membership, focused research or a correct skip, and
a served-browser review (`094-syncthing-surface-trace-product-fixture.md:51-59`).
The sole current Syncthing capture has no orientation or saved flow, so it
cannot verify that the now-valid seed becomes exactly one one-anchor trace,
appears in its trace evidence with an anchor and allowed frontier/transition,
or remains distinct from suggestions, components, and evidence bundles.

Do not infer runtime execution from the exact declaration or static
reachability. After deterministic fixture truth is sound, make the required
bounded run and reconcile one ID through report JSON, rendered header, canvas,
saved trace, and fixture verdict.

### 2. Syncthing fixture acceptance still has an unresolved inventory contract

**Impact: high.** The current product capture reports 45 surfaces and succeeds,
but the canonical script exits 1 because it still expects 36
(`runs/syncthing.md:21-26,57-70`). The report's own counts are mechanically
consistent—18 process entries, 18 routes, and 9 HTTP servers
(`report.json:4229-4244`)—but an old aggregate target is not evidence that the
nine listener/start records are all duplicates. Conversely, listener lifecycle
facts are currently independently promoted as `entry_surface` with
`partial_trace_ready` (for example
`trigger-0333606dca3a65ac3c8d5ecb`, `trigger_catalog.json:120-211`; and
`surface_summary.md:23-33`).

Define and test semantic membership: when a server start is a distinct
selectable entry versus linked route/process operational evidence. Then replace
the fixed aggregate assertion with source-backed ID/role assertions. Do not
change 36 to 45, or delete starts merely to make the script green.

### 3. Caddy still loses exact wrapped administrative registrations

**Impact: high.** The latest retained Caddy evidence is still the final-local
`f6ae3cf` report, not a post-`21fe3b9` rerun. Its canonical check exits 1:
`/config/`, `/id/`, and `/stop` are absent after local `addRoute` delegation to
`addRouteWithMetrics` (`verdicts/caddy.md:14-27`; `runs/caddy.md:15-22`). This
is a genuine bounded wrapper value/callback propagation gap, independent of the
now-fixed Syncthing producer/report semantic preservation.

Do not call import reachability runtime execution or use unresolved dispatch to
reclassify a repository-owned registration as dependency-only. Keep ownership,
static reachability, and trace readiness separate; recover the three exact
registrations with wrapper evidence, then rerun Caddy.

### 4. Restic has no completed full-surface artifact

**Impact: high for cross-fixture release acceptance.** The only successful
final-local Restic report explicitly used `--discover-surfaces=false`
(`runs/restic.md:108-128`); its three full attempts timed out before a report
was produced (`runs/restic.md:130-145`). It cannot validate executable
ownership, dependency-only exclusion, generic seed policy, or a full catalog.
The fallback also omits Darwin-selected `mount` while reporting 28 commands
(`verdicts/restic.md:35-50`).

Produce a bounded or resumable, manifest-bound full artifact before treating
Restic as a regression pass. A successful partial report is not evidence of a
verified zero generic/dependency surface set.

## Closure rule

The shared producer/catalog-to-report semantic blocker is closed for the
verified Syncthing primary ID. Decision 094 remains blocked until the
Syncthing inventory rule and deterministic fixtures are sound, Caddy wrapper
recall is restored, Restic has a full artifact, and the required fresh bounded
Syncthing/provider/browser trace audit proves the remaining cross-artifact
membership contracts.
