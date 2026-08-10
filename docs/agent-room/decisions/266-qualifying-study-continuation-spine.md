# 266 — Reserve one qualifying Study continuation spine

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-10)

**Preserves:** Decisions 243, 260, 264 and 265; exact target and reading
authority; source-ordered deterministic collection; the existing eight-edge
continuation budget; refs-only requests; item-local reduction; and honest
zero-call plans.

## Product defect

In the fresh repomap run, one Study reading was the selected `main` itself.
Its target connector therefore had distance zero. More than eight direct
children exhausted the continuation breadth budget at the first call layer,
so the advertised card contained no two-edge path even though the exact local
index contained one behind a later child. The planner correctly made no call
for a card that could not satisfy the two-edge mechanism minimum.

This is a bounded collection-order defect. Raising a limit, asking the model to
invent missing depth, or weakening the reducer would spend more resources or
discard exactness without fixing the qualifying shape.

## Exact corrective

For a targeted reading whose shortest target connector has distance `d < 2`,
the compiler first seeks one complete source-ordered simple continuation spine
of exactly `2-d` edges. The search excludes connector ancestors and cycles and
uses only exact DirectCallIndex nodes and edges. It retains no partial spine.

When the spine exists, its edges are reserved before the existing breadth-first
continuation collection. They count against, rather than enlarge, the existing
`MaxContinuationEdgesPerReading` budget; ordinary breadth fills only the
remaining slots. When no complete spine exists, ordinary bounded breadth stays
unchanged and the exact eligibility check continues to produce a zero-call
plan.

No prompt, provider, request, response, reducer, report, manifest, graph depth,
edge ceiling or schema identity changes. The existing catalog digest binds the
exact selected graph bytes.

## Acceptance

1. A reading equal to a `main` with more than eight children retains exactly
   eight continuation edges while exposing one exact two-edge candidate and a
   provider batch.
2. When two qualifying late children exist, the earlier exact callsite wins
   even if declaration or raw index storage order differs.
3. A wide root with only one-edge leaf calls retains the same bound and plans
   zero provider calls.
4. Full mechanismstudy tests, vet and diff checks pass without a provider or
   real-repository run.

Approved by:
    Repository owner through the fresh zero-mechanism causal audit, 2026-08-10.
