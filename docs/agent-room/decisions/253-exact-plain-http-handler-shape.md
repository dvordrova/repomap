# 253 — Exact plain HTTP handler shape

**Status:** ACTIVE (owner-authorized, 2026-08-09)

**Preserves:** D220's generic typed-registration detector, D248's
framework-free experiment, D250's build scenario and all existing Surface,
report, provider and budget contracts.

## Product failure

On Linux Moby, the generic registration detector treated every unnamed plain
function type as an HTTP handler. `rootless.RunInNetNS(string, func() error)`
therefore became an `http_route`, reverse-marked a large daemon call cone,
exhausted the 1,500-task walk budget and published fifteen junk routes while
real Serve and metrics surfaces were absent. Raising the walk limit would spend
more work on the same false premise.

## Approved contract

- An unnamed plain function is a supported typed HTTP handler only when its
  unaliased signature is exactly non-variadic
  `func(net/http.ResponseWriter, *net/http.Request)` with no results.
- Parameter aliases are unwrapped before exact package/type identity checks.
- Named `net/http.Handler` and `net/http.HandlerFunc` retain their existing
  behavior. Existing cataloged framework context-handler types retain their
  existing behavior and remain subject to `--no-frameworks`.
- No function name, package path, repository path or framework-specific
  blacklist is added. No walk, target, task or output limit changes.
- No artifact, report, provider, cache or UI identity changes: previously valid
  HTTP registrations are byte-equivalent; false registrations disappear.

## Acceptance

1. A repository-local `(string, func() error)` call produces no typed route.
2. A repository-local method accepting
   `(string, func(http.ResponseWriter, *http.Request))` still produces its one
   exact typed route.
3. Existing named standard-library and framework handler fixtures remain
   accepted under their prior framework policy.
4. A provider-free Linux Moby run does not publish the RunInNetNS junk routes;
   detached work no longer reaches 1,500 because of that false registration
   cone, and real Surface results can be assessed independently.

## Owner-risk check

This is the smallest product correction because one over-broad type predicate
created the false roots. It remains necessary next week for any repository
with `(string, callback)` APIs, fixes the cause rather than its task-limit
symptom, and adds only exact signature checks. Skipping it makes every larger
limit and downstream audit operate on knowingly false HTTP authority.
