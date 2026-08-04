# Decision: Optional Unverified-Path Normalization Corrective

Status: Active. Approved by the repository owner after a real ordinary run
failed on `orientation: unverified_paths[1] has invalid path "a/b/c/"`.

## Observed failure

`unverified_paths` is optional model uncertainty metadata. It cannot authorize
source navigation or establish evidence, but one safe directory-like value with
a trailing slash caused strict validation to reject the entire orientation and
prevent report publication.

## Approved corrective scope

1. Before final orientation validation, trim surrounding whitespace and trailing
   slashes from optional `unverified_paths`.
2. Retain only canonical repository-relative values, collapse duplicates, and
   record deterministic warnings for normalization or rejection.
3. Keep strict validation unchanged for every retained value and for all
   grounded/navigation-bearing orientation fields.

## Non-goals

- no prompt, provider, request, report, UI, Search, Study, Architecture, or
  analyzer change;
- no normalization of unsafe absolute or parent-traversing paths;
- no provider rerun required for the regression.

## Acceptance

- a provider-free regression preserves `a/b/c/` as canonical `a/b/c`, rejects
  `../secret`, and leaves a report that passes the existing strict validator;
- focused tests, `./scripts/check.sh`, `./scripts/etcd_check.sh ../etcd`, and
  `git diff --check` pass before rebuilding the stable PATH binary.
