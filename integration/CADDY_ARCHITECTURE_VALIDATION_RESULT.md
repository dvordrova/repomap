# Caddy architecture validation replay

## Source run

Saved run:

`/Users/dvordrova/Library/Caches/repomap/runs/20260712-184001-caddy`

The replay uses the captured provider response from `architecture_synthesis.json`.
No provider request is required.

## Local grounding

- repository archetype: `modular_platform_server`;
- grounding mode: `mixed`;
- behavior-anchor families: 13;
- low-level architecture relationships: 135;
- runtime surfaces: 0.

Runtime-surface discovery and architecture grounding are separate bounded
analyses. The empty `trigger_catalog.json` does not mean that architecture
grounding found no anchors.

## Captured proposal

The decoded response contains six top-level subsystems and twelve nested
components:

| Primary subsystem | Nested components |
| --- | --- |
| Core | Module Registry, Lifecycle, Logging |
| Config | Config Ingress, Config Adapter, HTTP Caddyfile |
| Admin | Admin Handler |
| HTTP | HTTP App, Request Dispatch, Headers |
| Security | TLS/PKI |
| Entry | Main |

The proposal references 22 unique supplied member IDs and all 13 supplied
behavior-anchor IDs. Every nested component is grounded and none is marked as a
hypothesis.

The exact decoded response is committed at
`internal/componentmap/testdata/caddy_architecture_proposal.json`.

## Pre-fix validation result

The validator counted all twelve nested components as primary pillars. It
produced one warning:

`proposal.excess_primary_pillars: grounded architecture exceeds eight primary pillars`

That recoverable hierarchy-shape mismatch caused:

- validation result: rejected;
- fallback reason: `proposal_invalid_or_empty`;
- rendered architecture: deterministic Packages / Repository symbols /
  Repository files grouping.

The provider call itself succeeded and `architecture_synthesis_status.json`
correctly records one successful request. The product defect is local proposal
validation and fallback selection, not provider transport or response quality.

## Required corrected result

The six top-level subsystems are six primary pillars. The twelve children are
nested responsibilities. Replaying the captured response must select the
validated or locally normalized model architecture, preserve exact IDs and
evidence, and render the six conceptual groups instead of the raw candidate-kind
fallback.
