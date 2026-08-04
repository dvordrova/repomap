FIXTURE VERDICT: BLOCKED

## Scope and run integrity

Audited the final-local offline run at production checkpoint `f6ae3cf`, fixture
revision `873fac5fc094fe538d0c477509127bb321d51a32`. The manifest binds that
revision and `report.json` SHA-256
`48d865396039f7829530d430ad808ab149f22589748e99717ef39ef28635fab7`
(`run_manifest.json:3-12`). The run made no provider request: model and endpoint
are empty and `offline` is true (`metadata.json:7-25`; `runs/caddy.md:40-44`).

## Blocking findings

1. **Wrapped administrative registrations are missing.** The source oracle records
   `/config/`, `/id/`, and `/stop` as administrative HTTP surfaces
   (`oracles/caddy.md:28`; source `admin.go:262-265`). Each is registered through
   `addRoute`, which wraps and delegates to `addRouteWithMetrics`, whose terminal
   `ServeMux.Handle` is at `admin.go:234-257,240`. The generated catalog/report
   contains only the six debug routes from the direct `addRouteWithMetrics` calls;
   the canonical check reports exactly that set and exits 1
   (`canonical.stderr.log:1`; `canonical.exit_status`; `runs/caddy.md:15-22`).
   Thus primary Caddy administrative control/reload surfaces have been lost at a
   wrapper boundary.

   *Smallest generic correction:* retain constant registrations through local
   wrapper/closure delegation when the wrapper forwards its pattern and handler to
   a supported registration primitive.

2. **Important-entry recall, seed membership, and surface accounting fail the
   fixture contract.** The report serializes 18 records: 6 routes, 3 descriptors,
   4 route frontiers, 4 servers, and one process entry, all application-owned
   (`report.json:2672-2703`). The canonical product oracle requires 19 routes, 5
   descriptors, 2 route frontiers, 6 servers, 22 application records, and 11
   supporting-dependency behaviors (`scripts/caddy_surface_check.sh:212-252`).
   Its discovery oracle also requires 32 records split 21/11, five descriptors,
   two configuration frontiers, and eight exact seeds; the report lists only seven
   seeds, omitting two required provider seeds (`report.json:2716-2724`; script
   `150-174`). This is not a count-presentation issue: the report's 18 total does
   reconcile with its own kind counts, but not with the source-derived expected
   inventory.

3. **The report cannot provide the requested ownership and architecture journey.**
   It correctly identifies the sole `cmd/caddy.main` process entry and labels the
   retained records `primary_application` (`report.json:2704-2714,5036-5043`), in
   agreement with the oracle's one primary and no secondary executables
   (`oracles/caddy.md:21-25`). But it has no CLI command records, no secondary or
   tooling classification, zero supporting dependencies, and no orientation report
   (`report.json:2686-2697`; warning at `report.json:10-12`). Consequently it does
   not deliver component responsibilities for core, command/configuration, HTTP,
   and TLS ownership described by the oracle (`oracles/caddy.md:62-67`), nor a
   focused research window.

## Misleading behavior and trace assessment

- The 18 retained records are internally evidence-backed and candid: repository
  wrapper chains, exact registration/start locations, ownership, and partial
  readiness are serialized (for example `report.json:2762-2908`). Configuration
  route records are properly marked dynamic/unsupported rather than invented
  routes (`surface_coverage.json:51-85`; `report.json:3430-3486`). However, this
  honesty does not compensate for omission of the wrapped admin controls and the
  missing provider/dependency inventory.
- Partial-trace language is useful where present: route/server records state that
  reachability or dispatch remains unresolved, while dynamic frontiers cannot seed
  a trace (`surface_summary.md:9-36,63-73`). No flows or saved traces exist
  (`report.json:6-7`). Do not present partial candidates, descriptors, frontiers,
  or their source evidence as saved traces or evidence bundles.
- The report has no suggestions or model orientation, and the feedback/research
  template is empty (`onboarding-feedback.md:4-18`). In this offline run it makes
  no provider coverage claim, appropriately; local coverage is only the bounded
  `go:darwin/amd64:tags=` static scenario (`report.json:2673-2676`), not runtime
  or configuration-complete coverage.

## Advisory

Keep the existing distinction between repository-owned application surfaces and
supporting dependency behavior after recall is repaired. Reconcile serialized and
visible totals from one complete catalog, and preserve explicit partial/unsupported
frontiers rather than upgrading them to complete traces.
