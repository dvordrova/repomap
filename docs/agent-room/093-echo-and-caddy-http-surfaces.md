# Decision: Echo and Caddy HTTP surface discovery

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

`repomap <repo>` should retain the existing bounded `net/http` and Gin surface
discovery while adding verified Echo route registration support and recovering
statically provable Caddy admin/server dispatch surfaces. Every result remains a
deterministic local record in the existing `All surfaces` catalog.

## Approved scope

1. Add catalog entries for verified Echo route-registration terminal APIs and
   deterministic direct/convenience-wrapper fixtures.
2. Reuse the generic catalog-driven SSA propagation engine; do not add
   framework-name branches to traversal.
3. Recover Caddy-relevant admin/control-plane and HTTP server dispatch surfaces
   only when exact build-selected calls, registrations, handlers, or server
   roots support them.
4. Represent configuration-derived or dynamically assembled served-site routes
   as explicit unresolved frontiers rather than invented concrete routes.
5. Project all accepted records through the existing unified typed catalog and
   `All surfaces` navigation, preserving exact evidence and coverage accounting.
6. Add focused deterministic Echo and Caddy regressions. Automated tests do not
   require a provider or network call.

## Non-goals

Do not build a universal route inventory, run runtime tracing, add a whole-
program pointer analysis, infer routes from architecture anchors or model prose,
hard-code Caddy repository paths, redesign the report, or add another surface
catalog. Do not claim that route registration proves handler execution.

## Acceptance

The decision is implemented when a verified Echo fixture emits exact method,
path, handler, registration, and wrapper evidence; Caddy emits useful static
admin/server surfaces where exact evidence exists while dynamic site routes
remain honest frontiers; direct `net/http` and Gin behavior remains unchanged;
the resulting records appear in `All surfaces`; and focused plus repository-wide
checks pass.

## Verified Caddy baseline

The implementation audit used the clean Caddy checkout at commit
`873fac5fc094fe538d0c477509127bb321d51a32`. Its standard build has nine core
admin `ServeMux` patterns, five returned `AdminRouter` descriptors, local and
remote admin `http.Server.Serve` roots, a configured HTTP/1/2 server root, and
an HTTP/3 `ServeListener` root. Ordinary served-site routes remain assembled
from runtime configuration and module registries, so the accepted result keeps
that inventory unresolved instead of promoting all registered handler modules
to active routes.
