# Caddy architecture fixture

This fixture records why saved run `20260712-101953-caddy` is a useful package
landscape but not yet a behavior-grounded architecture. Diagnosis uses only the
saved artifacts under the local repomap run cache; no repository analysis or
provider call was repeated.

## Saved evidence

| Artifact | Observed fact |
|---|---|
| `trigger_catalog.json` | Zero triggers. |
| `surface_coverage.json` | 47 packages loaded, one process entrypoint considered, one function walked, zero dispatch roots, zero semantic seeds, zero frontiers, and no exhausted budget. |
| `orientation_report.json` | Four model-oriented directions, all partial after local confidence caps. The high-level map names Core runtime and lifecycle, Admin API server, HTTP server and middleware, Caddyfile configuration parsing, and TLS certificate management. |
| `flows/*/flow_status.json` | All four bundles are `local_only`; none is a completed FlowProof. |
| `flows/*/flow_bundle.json` | Useful exact files and source signals were retrieved, but package selection is zero for HTTP/Caddyfile and only `cmd/caddy` for CLI/startup. Basename matching also admits unrelated `command.go` and `caddyfile.go` files. |
| `architecture_synthesis.json` | Prompt `component-landscape-v2` succeeded with nine subsystems and eighteen components. Every returned member is a package ID. |
| `report.json` | The canvas promotes 57 structural edges, all `package_import`. The five orientation concepts and eighteen synthesized component names have no overlap. |

## Failure classification

| Problem | Classification | Evidence and boundary |
|---|---|---|
| The board is visually balanced but explains packages rather than runtime responsibilities. | Intentional package landscape plus missing grounding-mode presentation. | Rendering preserves the saved synthesis accurately; coordinates and colors cannot recover absent behavioral evidence. |
| CLI and startup bundles select only the binary package; HTTP and Caddyfile select no package. | Architecture candidate selection. | Correct high-score files exist, but package attachment and lexical neighborhood scoring do not promote them into exact architecture members. |
| A successful provider response can contain only package IDs and still be accepted as the primary architecture. | Architecture synthesis prompt/contract. | The request is dominated by package candidates/imports and the response contract requires neither anchors nor behavior-backed primary pillars. |
| `main → cmd.Main`, command registration, config adaptation/application, admin startup, request dispatch, and lifecycle calls are absent. | Missing behavioral anchors. | The source bundles contain relevant files and lines, but no saved contract identifies these roles or links them into a bounded backbone. |
| Surface discovery reports one walked function after loading all packages. | Surface-discovery traversal. | Traversal starts at `main` and follows only functions in the reverse closure of configured terminal seeds. `cmd.Main` is not relevant when no seed is matched, so composition traversal stops immediately. |
| Fifty-seven package imports are shown as the conceptual architecture's connective tissue. | Package-edge promotion. | Promotion is mechanically exact but semantically wrong for the primary behavioral view. Package imports belong in details or an explicit dependency view. |
| A modular server/plugin platform is treated like a directory taxonomy. | Repository-archetype mismatch. | Saved evidence contains one executable, config adapters, an admin server, HTTP middleware, TLS/PKI packages, and many module families, but no repository-level archetype is recorded. |
| The four directions remain useful retrieval bundles even though they are not proved flows. | Intentional fallback behavior. | `local_only` is honest and should remain available. It must not be reinterpreted as a FlowProof or runtime trace. |
| Architecture synthesis itself reports success and `fallback=false`. | Not a fallback defect. | JSON/schema validation succeeded; the missing gate is onboarding usefulness and behavioral grounding. |

## Exact contradictions

- `PackagesInspected=47` describes SSA load scope, while
  `FunctionsInspected=1` describes actual walk scope. The current product does
  not make that distinction obvious.
- The HTTP bundle contains `Server.ServeHTTP` and a primary handler-chain call,
  yet has zero selected packages, zero related edges, and zero chain steps.
- The Caddyfile bundle contains exact parser functions, yet has zero selected
  packages, zero tests, and zero related edges.
- The startup bundle contains `admin.go` and `caddy.go`, but selects only the
  binary package.
- Architecture synthesis technically succeeds while replacing the more useful
  high-level orientation concepts with Core, Internal Utilities, Filesystem,
  Standard Modules, and Testing package groups.

## Working hypothesis

The bounded archetype hypothesis is `modular_platform_server`, not because the
repository is named Caddy, but because the saved evidence indicates:

- one executable process entry;
- server/admin and HTTP handler boundaries;
- configuration parser/adapter packages;
- registry/module vocabulary and broad module families;
- TLS/PKI responsibilities;
- substantially more library/module packages than executables.

This remains a hypothesis until exact registry, lifecycle, config-apply, and
request-dispatch anchors are collected.

## Smallest correction boundary

The first implementation slice must preserve the existing package landscape
and add a separate, presentation-neutral grounding envelope:

1. a saved grounding mode (`behavior_grounded`, `mixed`, or
   `package_landscape`);
2. a bounded repository-archetype assessment with evidence and alternatives;
3. exact behavior anchors with stable IDs, locations, certainty, scenario,
   producer, role, related members, and limitations;
4. a backbone projection that separates primary pillars, extension families,
   support/tooling, and unresolved hypotheses;
5. local gates that downgrade to `package_landscape` instead of inventing a
   behavioral map;
6. package imports hidden from the primary behavioral edge layer and retained
   in component details/dependency mode.

Entrypoint composition traversal may then add anchors by following bounded,
direct repository-local calls before seed pruning. It must retain existing
depth/task/target budgets and stop before ordinary request implementation.

## Baseline capture

The current balanced package taxonomy is captured at
`tmp/architecture-flow-audit/caddy-package-landscape-before-1600x1000.png` and
`tmp/architecture-flow-audit/caddy-package-landscape-card-before.png`.
