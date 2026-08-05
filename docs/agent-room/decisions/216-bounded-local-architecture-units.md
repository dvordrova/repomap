# Decision 216: Bounded local Architecture units before semantic grouping

## Status

ACTIVE (goal 03 of the Archive 5 roadmap, approved by the repository owner
2026-08-05; implementation landed in the same session). Supersedes the
flat raw-member Architecture grouping for new runs while the legacy member
contract remains wire-valid for saved records.

## Problem proven by etcd

The current Architecture call asks one model to globally partition:

- 251 raw conceptual members;
- 57 behavior anchors;
- 53 structural locators;
- 635 relations, including 613 package imports (~74.6 KB of the provider
  payload).

The model began one very broad component and entered an exact 56-package-ref
cycle until the 64k output ceiling (see D215 evidence). The current flat
member task is not robust for a large modular server.

## Proposed authority transition

```text
exact raw members and relations
        ↓ local deterministic compiler
bounded ArchitectureUnit catalog
        ↓ one semantic grouping call
unit-level conceptual hierarchy
        ↓ local exact expansion
Landscape + exact member participation + local remainder
```

The model never returns raw package/symbol member refs.

## ArchitectureUnit

One unit contains backend-owned exact membership plus a compact wire
projection:

```text
private:
- canonical unit ID
- exact member IDs
- exact locations/evidence
- exact package graph membership
- production/test/tool role

wire:
- request-local u* ref
- short semantic label
- role: production | test | tooling | documentation
- member count by kind
- representative labels (bounded)
- anchor refs (bounded)
- boundary/resource summary (bounded)
- relation summary counts
```

No path, canonical ID, raw source, or unbounded edge list enters the wire.

## Deterministic compiler

1. Separate production, tests/integration, contrib/tooling, and examples.
2. Build package graph using exact local package identities.
3. Seed units from process entries and exact behavior-anchor neighborhoods.
4. Attach symbol candidates to exact owning package units.
5. Cluster remaining packages using deterministic graph + module/top-level
   structure.
6. Split any oversized unit deterministically.
7. Preserve every raw member in exactly one primary unit.
8. Record complete coverage and explicit omissions; no first-N truncation.
9. Compile D214 call sites as summaries/overlays, not additional raw model
   members.
10. Bound the advertised catalog, target 24–64 units.

The exploratory etcd graph produced five large coherent regions plus small
islands, so a bounded local unit layer is feasible. The production algorithm
must be stable, tested, and repository-generic.

## Model response

The response groups unit refs only:

```json
{
  "subsystems": [
    {"ref":"g1","name":"...","description":"..."}
  ],
  "components": [
    {
      "ref":"c1",
      "subsystem_ref":"g1",
      "name":"...",
      "description":"...",
      "unit_refs":["u1","u2"],
      "anchor_refs":["a1"],
      "hypothesis":true
    }
  ]
}
```

Rules:

- at most 8 subsystems;
- at most 24 components;
- every unit appears in at most one primary component;
- separately bounded cross-cutting unit participation only if the product still
  requires it;
- duplicate unit refs are fatal;
- exact partial grouping remains valid;
- omitted units become one local unclassified remainder;
- model prose cannot create evidence or operational authority.

Backend expansion maps `u*` to exact members and locally derives all
hypothesis/evidence tiers.

## Why this is better

- removes 613 raw package-import edges from the wire in favor of aggregated
  unit edges;
- removes 251 raw member assignments from output;
- makes output size calculable;
- preserves exact source authority;
- reduces model authority rather than increasing it;
- retains one Architecture semantic call;
- makes a smaller stage output ceiling justifiable after measuring the legal
  maximum;
- gives UI a meaningful evidence tier and production/test/tooling role.

## Acceptance

Provider-free:

- deterministic unit identity under input reordering;
- complete exact member coverage;
- stable etcd unit count and digest;
- no cross-role accidental merge;
- bounded wire bytes;
- maximum legal response serialized and proven below its chosen envelope;
- unknown/duplicate/wrong-kind unit refs fail closed;
- exact partial grouping publishes local remainder;
- accepted result expands to exact member identities;
- old response/cache identities miss closed;
- Casdoor, etcd, a library, and a small service all produce usable units.

Then exactly one live etcd A/B after all local gates.

No second semantic call, embeddings, Tree-sitter, fuzzy repair, raw source
upload, or UI redesign in this decision.

## Implementation record (2026-08-05, goal 03)

### Delivered (working tree, not committed)

- **Deterministic unit compiler** (`internal/componentmap/units.go`):
  `CompileUnitCatalog` separates roles (production/test/tooling/
  documentation), seeds around exact process entries and behavior-anchor
  packages, attaches symbols/files via exact ParentID, clusters remaining
  packages by top-level module structure (display name path or exact
  declaration-fact package path), splits oversized units, preserves every
  raw member in exactly one primary unit, and records complete coverage
  with explicit omissions. Canonical IDs are sanitized out of every wire
  label.
- **Wire**: `SynthesisRequest.Units` carries the bounded unit projection
  (u* refs, roles, member-kind counts, bounded representative labels,
  anchor-ref counts); `SynthesisRequestVersion=11`,
  `SynthesisRecordVersion=11`, `SynthesisPromptVersion=v14`.
- **Response**: component records accept either `unit_refs` (u*, bounded
  unit contract) or `member_refs` (legacy flat contract) — never both;
  unknown/duplicate/wrong-kind unit refs fail closed. Unit refs expand
  locally to exact members before Apply.
- **Prompt**: the model groups unit refs; no raw member enumeration needed
  for unit-catalog requests.
- **Tests**: unit compiler determinism (shuffled input), complete member
  coverage, role separation, canonical-free wire, exact expansion parity;
  etcd fixture wire measurement; maximum-legal unit proposal round-trip
  (75,597 bytes ≪ 16MiB response byte ceiling, well under the 64k output
  token ceiling).

### Wire win on the saved etcd fixture

- raw conceptual members: 284; unit catalog: 22 units, 1,967 unit-wire
  bytes (vs 88,109-byte legacy request JSON / 42,197 input tokens).
- etcd no longer asks the model to enumerate 284 raw members; it groups
  22 bounded unit refs.

### Gate results

- `go test ./internal/componentmap/ -count=1`: ok
- Full suite `go test ./... -count=1`: 74/74 ok, EXIT 0
- `go vet`, gofmt on touched files: clean
- Provider-free: all gates run without credentials
