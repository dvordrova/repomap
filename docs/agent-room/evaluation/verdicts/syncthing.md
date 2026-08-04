FIXTURE VERDICT: PASS

## Scope and identity

Audited exactly the focused producer-semantics run
`20260714-211633-syncthing` against the independent source oracle. The manifest
binds `report.json` SHA-256
`90a34e3846a28c4543010a35dce969a46c814802e019e5ef31860e15a18462a3`
to Syncthing revision `d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a`
(`run_manifest.json:3-12`; `oracles/syncthing.md:9-17`).

## Blocking findings

None. In particular, the former primary-seed semantic blocker is fixed in both
serialized authorities, not merely in the catalog:

- `trigger-1866c3e977616ca13632e2ab` is the exact, build-selected
  `cmd/syncthing/main.go:212` declaration with `gofacts-go-main` evidence
  (`trigger_catalog.json:508-570`; `report.json:4946-5014`).
- Both artifacts classify it as `primary_application`, `entry_surface`, and
  `partial_trace_ready`, with the bounded and honest reason that an exact
  process entry supplies a one-anchor partial trace while typed downstream
  closure is unavailable (`trigger_catalog.json:573-590`;
  `report.json:5016-5033`). This agrees with the oracle's primary executable
  and CLI dispatch boundary (`oracles/syncthing.md:24-46,94-101`).

The unavailable closure is correctly a frontier, not a rejection of the
primary: the only reported cause is `auto.Assets` type failure in `lib/api`,
and the architecture anchor separately preserves the exact declaration
(`surface_coverage.json:216-243,259-295`;
`architecture_grounding.json:785-817`).

## Semantic checks

- **Executable classification and recall:** The 18 process entries consist of
  one primary, the oracle's six secondary build targets, ten `cmd/dev` tooling
  programs, and one test helper (`surface_coverage.json:194-196`; the oracle
  enumerates the primary/secondary/tooling boundary at
  `oracles/syncthing.md:24-67`). The important primary seed and all six
  secondary executable packages are retained. The two unavailable entries
  (`cmd/syncthing` and `cmd/infra/strelaypoolsrv`) remain classified and
  declaration-backed rather than silently omitted.
- **Ownership, evidence, and frontiers:** Process declarations have exact
  repository locations and owners. Route/server observations name their owning
  executable and static registration/start evidence; unresolved handler,
  dispatch, address, and dynamic-route facts are labeled as frontiers rather
  than execution claims (`surface_summary.md:7-9,57-69,83-95`;
  `surface_coverage.json:298-365`). This is appropriate for a static run and
  does not claim configured GUI/API, peers, or synchronization success—the
  oracle's stated runtime frontier (`oracles/syncthing.md:149-176`).
- **Dependency/noise precision:** The catalog reports no supporting or
  dependency-only surfaces and every displayed surface has an executable role
  (`report.json:4229-4250`). The nine HTTP-server starts are source-backed,
  owned operational producers, not unowned dependency noise; their partial
  readiness accurately limits them to operational traces. No source-oracle
  semantic blocker establishes that they must be discarded.
- **Counts:** Serialized totals reconcile: report run counts, discovered
  catalog totals, and metadata all say 45; the composition is 18 process + 18
  HTTP route + 9 HTTP server (`report.json:13-28,4224-4248`;
  `metadata.json:9-25`).
- **Suggestions, traces, evidence bundles, research windows, and components:**
  This was deliberately offline with `flows: 0`; `flows` and candidate flows
  are null and no orientation report exists (`metadata.json:15-25`;
  `report.json:5-12,10622-10624`). Therefore it presents no invented
  suggestions, saved traces, evidence bundles, focused research windows, or
  component-responsibility narrative. The static architecture artifact is
  limited to anchors/archetype and says what it cannot establish
  (`architecture_grounding.json:3-13,785-817`), so it does not impersonate the
  oracle's composition/model/filesystem/discovery/API responsibility map
  (`oracles/syncthing.md:129-145`).
- **Provider/local coverage:** Offline mode is explicit; model and endpoint are
  empty, and the run record reports no provider use (`metadata.json:6-25`;
  `runs/syncthing.md:51-55`). Claims are consequently local-static only.

## Non-blocking script failure

`./scripts/syncthing_surface_check.sh` exited 1 solely because its fixed
expectation is 36 while this producer-aware run serializes 45 records
(`runs/syncthing.md:57-70`). That is a stale fixed-count acceptance check, not
a source-oracle semantic failure: the report both enumerates and reconciles
the added nine server-start producers, and labels their partial operational
limits. It must not override the catalog/report agreement for the primary.

*Smallest generic correction:* make acceptance validate reconciled categories
and their ownership/readiness invariants, rather than a fixture-specific total.

## Advisory findings

- The primary's deeper launch-to-`App.Start`/GUI/API journey is not traceable
  in this run because the typed closure is unavailable. The retained exact
  partial anchor is useful and honest, but not a substitute for that journey.
- The rendered report was not a separate semantic authority in this offline,
  zero-flow audit; its machine report and trigger catalog reconcile. Future UI
  acceptance should expose the same 45/category totals and the primary's
  `partial_trace_ready` status directly.
