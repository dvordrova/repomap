# 260 — Target-rooted Study source trails

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-09)

**Preserves:** Decisions 243, 246, 256, 257 and 258; one private
`DirectCallIndex`; exact final Study-reading binding; refs-only mechanism
responses; existing 32-node/48-edge card, request and result ceilings; and
Study source-trace presentation without runtime-order or branch claims.

## Product defect

The depth-two mechanism compiler centered each card on a Study reading. In a
repository with several commands it could therefore publish a locally exact
path that had no connection to the selected product. In a sparse useful chain
it also stopped two edges around the reading even when the already-built exact
call index contained a short target-to-reading source trail.

Raising repository-wide graph limits does not fix either defect. The compiler
must consume the one D257 target before it admits edges, and it must preserve
the D258 reading identity rather than rediscovering either endpoint.

## Exact target input

One explicit `TargetCompileInput` carries the validated `AnalysisTarget`, the
producer-owned sealed `TargetRoots`, the D258 `StudyReadingRootBindings`, the
current private `DirectCallIndex`, final Study artifact and repository binding.
Compilation revalidates every envelope against the same target, index digest,
scenario and repository revision before collecting a graph.

Executable roots are only exact selected-package `main` declarations. Library
roots are only exact exported declarations recorded by the DirectCallIndex
producer and sealed by `analysistarget.BindExactRoots`; mechanismstudy does not
guess exported names, synthesize `main`, reload packages or build SSA.

The target trail authority is private version 1. The compilation digest binds
the complete target, sealed target-root SHA and omission count, D258 envelope,
per-card exact target-root node IDs, index and scenario. Facts restoration
rebuilds private target-root refs and rejects drift. No target identity, full
symbol ID, path or source body is added to provider JSON.

## Target-rooted collection

For each exact reading, the compiler finds the directed shortest distance from
all exact target roots. It retains the complete shortest-path DAG: equal
shortest alternatives either all fit the existing card bounds or the reading
stays prepared with `ambiguous_shortest_connector`. Input or opaque-ID order
never chooses one connector. An unreachable reading stays prepared with
`target_unreachable`; a connector beyond the existing eight-edge result limit
stays behind `depth_bound`.

After the reading, one deterministic source-ordered breadth-first continuation
uses at most eight total edges for that reading. A complete published simple
trail remains at most eight edges, and the existing 32-node/48-edge card bounds
remain final. Callsite path, line and column order first; exact declaration and
semantic fields break visible ties; opaque edge/node ID is the final tie only.
Dense fanout therefore cannot become eight continuations per parent.

Every provider candidate is revalidated as one simple ordered path. Under a
targeted compilation it must start at an exact target-root ref and cross at
least one exact Study-reading ref. A suffix path and a fork/merge-shaped set of
otherwise advertised edges close item-locally. The browser still receives one
linear source trace and no branch or runtime-order claim.

## Failure and runtime boundary

Target-rooted compilation is optional enrichment but has no legacy fallback.
An ordinary caller first binds D258 readings and exact target roots, then calls
`CompileTargeted`. A missing target or nil/unavailable index skips the optional
stage honestly and must not call legacy repository-wide `Compile` or construct
a provider client. A valid empty target-root set or zero target-reachable
readings produces prepared cards and zero provider batches. Identity drift,
unsafe persistence and artifact tamper remain terminal integrity failures.

The legacy context compiler retains its existing absolute depth-two graph.
The existing refs-only request and response schemas, prompt identity,
`MaxEdgesPerMechanism = 8`, report42, manifest14 and UI21 do not change.

## Acceptance

1. A repomap-shaped target starts at `cmd/repomap/main` and excludes the
   quality command even when both can reach the same reading.
2. A Moby-shaped `cmd/dockerd` target excludes proxy and plugin mains.
3. A Telebot-shaped library starts at exact exported `NewBot`, reaches an
   internal Study reading and never invents `main`.
4. A sparse chain publishes one exact eight-edge target trail while the legacy
   compiler remains depth two.
5. Dense fanout retains at most eight total post-reading edges and preserves
   the 32/48 bounds with truthful frontier accounting.
6. Small equal-shortest alternatives are all advertised; either linear
   alternative can be selected, while a fork-shaped candidate is rejected.
   An over-bound equal-shortest DAG stays prepared without a prefix winner.
7. Reading input permutation leaves the selected exact graph unchanged.
8. Facts round-trip restores private target-root authority; target digest and
   target-root-ref tampering fail closed.
9. Focused tests, full mechanismstudy tests, vet and diff checks pass before
   runtime integration.

Approved by:
    Repository owner after the repomap/Telebot/Moby report review requested one
    product target before Architecture, Study and mechanism budgets, then asked
    for a deeper useful source trail rather than a reading-local depth increase,
    2026-08-09.
