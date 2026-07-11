# Increment 2 result: shared evidence and applicability rules

## Focused regressions added

Two root-cause tests protect this contract change:

1. `TestEvidenceGraphRejectsUnknownSemanticEnums`
2. `TestCoreOwnsApplicabilityAndSlotVerdicts`

The evidence test uses valid Go and Python fact fixtures, accepts a pathless
external entity, and rejects unknown entity, scope, relation, resolution and
invocation values. The applicability test feeds the same normalized
synchronous-handler fact from Go- and Python-labelled fixtures and proves that
only the core derives `not_applicable` while retaining provider provenance.

## Behavior corrected

- evidence graph version 2 closes the semantic enum vocabulary at validation;
- FlowProof and session version 2 reject provenance-less version 1 proof state
  instead of silently reinterpreting it;
- empty scope, resolution and invocation remain accepted where those fields
  are optional, preserving existing Go graphs;
- repository-scoped entities require repository-relative locations while
  external dependency/toolchain entities may remain pathless;
- adapters now report `unknown`, `present` or `absent` concurrent lifecycle
  presence with provenance instead of supplying final slot statuses and
  reasons;
- the FlowProof core derives the only currently approved optional-slot outcome:
  CLI concurrency is `not_applicable` when a bounded handler scope is proven
  synchronous;
- an adapter-provided verified concurrency status is recomputed from facts and
  cannot survive without a concrete lifecycle;
- a reasonless `not_applicable` slot does not satisfy a proof.
- an absence fact without valid provider/operation provenance cannot satisfy an
  optional slot.

## Verification

Focused shared and adapter consumers passed:

```text
go test -count=1 ./internal/evidence ./internal/flowproof \
  ./internal/flowproof/assemble
go test -count=1 ./internal/analyzer/... ./internal/symbol \
  ./internal/investigation
```

The second command exercised the existing deterministic Pyright tests without
promoting that adapter to production FlowProof parity.

Fresh restic v2 facts replayed with unchanged honest outcomes:

- backup concurrency: `partial`, missing the selected branch scenario;
- init concurrency: `not_applicable` with
  `no_concurrent_lifecycle_in_scope`;
- both flows stop at `no_task`, not `complete`.

## Remaining assumptions and deferrals

- No generic cross-language async algebra was added. The shared evidence/core
  still exposes the Go-shaped `starts_goroutine` / `goroutine` vocabulary; this
  is a known vocabulary leak deferred until a real non-Go async fixture proves
  the replacement contract.
- The real Pyright fixture is not accepted as the cross-language proof fixture
  yet because its selected function and outgoing `getattr` fact do not match.
- Python dynamic uncertainty, scenario identity and shared DTO round trips
  remain assigned to Increment 4.
- Existing graph/proof artifacts are unreleased; version 1 state now fails
  clearly rather than being silently interpreted with version 2 semantics.
