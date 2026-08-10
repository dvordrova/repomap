# 276 — Target-portfolio eligibility and zero-primary status truth

**Status:** ACTIVE (owner-authorized ordinary-run corrective, 2026-08-10)

**Preserves:** Decisions 262–263, 269, 274 and 275; the complete exact local
target catalog; one semantic portfolio call; refs-only response restoration;
explicit `--target`; exhaustive `--all-targets`; exact Architecture membership;
and strict local validation without backend-authored conceptual grouping.

## Fresh etcd failures

An ordinary current-binary etcd run asked the portfolio model to choose from
183 exact Go packages. The request exposed the root target as
`{"ref":"t1","display_path":".","kind":"library","symbols":[]}`. That exact
package contains only the `main_test` BOM/tidy placeholder and exposes no
build-selected exported API. The response nevertheless copied the literal
`t1` used by the response example, selected it as the default, and the strict
ref reducer accepted it. The weak repository label was also `v3`, derived from
the final semantic-import-path segment rather than the preceding repository
name.

The selected empty root then produced an Architecture request with zero
production-primary candidates and only exact tooling/test supporting evidence.
The model returned a structurally valid partial grouping over that supplied
scope. `componentmap` accepted it correctly, but the persisted Architecture
status validator rejected every successful zero-primary status even when its
supporting membership partition was exact. The ordinary run therefore failed
with `status.invalid_evidence` after a valid provider response.

## Portfolio correction

- The complete local catalog stays unchanged. Empty-API library packages remain
  available to explicit local selection and exhaustive catalog inclusion.
- The ordinary semantic portfolio may not select a library whose complete
  build-selected exported declaration inventory is empty. The prompt states
  that such a target exposes no advertised public API; local response reduction
  enforces the same exact fact for both the default and every selected sibling.
- The response-shape reminder contains no literal valid `t1` choice. It explains
  the fields without teaching the first catalog entry as a default.
- A standard trailing Go semantic-import major segment (`/v2`, `/v3`, …) is
  removed only for the provider-visible repository label, so
  `go.etcd.io/etcd/v3` presents `etcd`. Canonical module/package identity and
  every local ref remain unchanged.
- A failed/invalid provider result may use the existing strong local fallback
  only when that default satisfies the same exact executable-or-public-API
  eligibility. It never falls back to the known-empty root.

The response semantic identity advances from v2 to v3. Catalog, compilation and
request remain v2 because their exact JSON shape and complete local authority do
not change. Prompt identity advances from its exact content. Earlier responses
miss closed; there is no retry, judge, target quota, path-purpose role, ranking,
or second call.

## Zero-primary Architecture status

When membership accounting is present and the request contains zero primary
scope, a successful or cached status is valid exactly when:

- requested, covered and uncovered primary counts are all zero;
- covered supporting evidence equals the distinct accepted member count;
- that supporting count does not exceed the requested conceptual count; and
- the existing covered/uncovered conceptual partition and diagnostics remain
  internally consistent.

This case is not the production `empty_primary_scope` or
`supporting_only_unit_coverage` failure: those gates apply only when production
primary scope was actually requested. Count mismatches still fail closed. No
Architecture member is inserted, promoted or repaired. Architecture synthesis
status advances from v15 to v16 so older successful statuses are not silently
reinterpreted; its field schema, report/manifest identities and UI remain
unchanged.

## Verification

Provider-free tests cover an etcd-shaped empty root plus client/server/CLI
targets, literal-first-ref resistance, current-v3 response restoration,
old-v2 rejection, empty-library default/sibling rejection, eligible-only local
fallback, standard `/vN` repository labels, and explicit-target preservation.
Architecture tests cover accepted and accepted-partial zero-primary supporting
partitions plus negative count drift and status/exchange parity. Final
acceptance is one ordinary etcd command with no `--target` or `--all-targets`,
followed by the existing manifest/report/portfolio checks.

Approved by:
    Repository owner through the persistent instruction that the ordinary
    `repomap REPO --github-url ... --lang ru` command must work without manual
    target selection, cleanup, or recovery rituals, 2026-08-10.
