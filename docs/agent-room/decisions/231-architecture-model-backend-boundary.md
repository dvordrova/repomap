# 231 — Architecture model/backend boundary: shared participation over type-blind slicing

**Status:** ACTIVE (owner-authorized decision A of the Archive 9 semantic-product program)
**Supersedes:** the D230 D9.7 shared-unit slice semantics in
`internal/componentmap/synthesis.go` (repeated broad unit → anchor-specific
package∩symbol slice). D229 item-local salvage, D4 equivalence coalescing,
D9.5 association scope and the connected-mechanism contract remain binding.

## Problem (owner pack, Archive 9 hard review v10, P0)

The D230 shared-unit safety net deletes useful semantic distinctions:

- Telebot `20260806-163318`: model proposes `Запуск бота` (u2 + lifecycle_start),
  `Обработка вебхуков` (u2 + request_dispatch_root), Middleware/Reactive/Layout
  (u1) → whole proposal rejected → local package fallback. Diagnostic chain:
  `shared_unit_slice` → `empty_anchor_slice` → `empty_member_coverage`.
- Chatto `20260806-163320`: 7 components all on u2 (42 internal packages) with
  distinct anchor families (admin 15, config ingress 1, config apply 5,
  TLS 6, process 1, lifecycle 1) → every slice empty → whole reject → local
  anchors fallback.
- Restic `20260806-163323`: «Диспетчер команд» (u2 + 16 anchors) survived in
  Archive 9 as «Основной демон» (its 16-anchor slice) while
  «Внутренние помощники» (u2 + 0 anchors) was removed; the type-blind slice
  still misclassified shared participation and dumped 48 members into the
  remainder instead of keeping the dispatcher/helpers distinction honest.

Root cause: the slice intersects unit members (packages) with anchor-owned
members (symbols). The intersection is empty by type, and the anchor symbols
are often not ConceptualMember-role candidates at all, so even the
symbol-fallback (D230 review B1 repair) cannot publish them as members.
The backend converts *participation* into *exclusive ownership* and then
deletes components that legitimately have no exclusive member slice.

Contract violations: salvage v6 §Unit use (repeated units → shared/conflicted
scope while anchor-specific slices survive); charter v5 monotonic law
(publish(valid+bad) ⊇ publish(valid)).

## Decision

**A (more prompt prose)** — REJECTED: the model cannot fix a backend type
mismatch with more self-check text; owner audit v10 explicitly rejects
"another prompt full of never/count/self-check".

**B (current wire + more salvage patches)** — REJECTED: patching
empty-anchor-slice case-by-case keeps the type-blind intersection as the
core mechanic and will regress on the next package/symbol shape.

**C (chosen) — shared participation model, two bounded internal lanes:**

### Lane 1 — shared participation (backend semantic model)

A component is valid when it has ANY of:

- exclusive owned member IDs (unit referenced exactly once → full expansion); or
- shared unit participation + distinct exact anchor family + distinct
  responsibility (zero exclusive members is VALID); or
- shared unit participation + distinct responsibility + no anchors
  (only when the responsibility is materially distinct from siblings;
  otherwise D4 equivalence coalesces it).

Store independently on the accepted component:

- editorial identity (name/description/alternates);
- participating unit refs (shared vs owned classification);
- exact supporting anchor IDs;
- exclusive owned member IDs (may be empty);
- shared member IDs (the shared unit's expansion, display-only);
- coverage/limitations.

### Lane 2 — backend normalization of flat wire mechanics

Keep the flat tagged-record wire TEMPORARILY (nested wire migration is
disproportionate this program; owner program v10 permits preserving it while
moving mechanics to backend). The backend:

- accepts unit/anchor refs without requiring the model to self-check kinds
  (catalog owns kinds);
- treats `hypothesis` as optional (absent → backend-derived false);
- does NOT require "at least one unit overall" (zero useful semantic
  components → local-only product state, honest);
- normalizes exact duplicate refs within a component (count, keep item);
- keeps shared-unit components ALIVE: `shared_unit_slice` becomes an
  informational recoverable finding, `empty_anchor_slice` fires only when a
  component has NO unit participation AND NO anchors AND NO distinct
  responsibility.

### Prompt changes (architecture synthesis, prompt v18)

Remove/soften fatal bookkeeping clauses:

- remove "never split one unit across components" (replaced by: a unit may
  participate in several components only when responsibilities are genuinely
  distinct; explain the distinguishing feature);
- remove "at least one supplied unit ref must be returned";
- remove "collect the distinct unit_refs... self-check" (backend validates);
- remove "silently validate the complete JSON syntax, every record kind,
  every unique subsystem ref, every exact subsystem_ref" (backend validates);
- remove "Every component must contain at least one supplied unit ref"
  (a component may be anchor-backed shared participation);
- remove `hypothesis` from the response grammar (backend derives it);
- unit/anchor refs may omit the backend-owned `kind` (`{"ref":"u1"}`);
- keep: choose supplied refs, partial grouping valid, no model remainder,
  bounded prose, honest authority (no runtime order from static facts),
  language contract.

### Zero useful semantic components

A structurally valid proposal that covers no requested conceptual members is
an honest **zero-useful-semantic result**, not a malformed schema: the exact
local landscape publishes with the recoverable finding
`proposal.zero_useful_semantic_components` and `FallbackRejectedUnknownMember`
as its explicit reason. This supersedes the D230 repair-log note that
`empty_member_coverage` is FATAL by design; `empty_member_coverage` remains in
the closed vocabulary for legacy records but is no longer emitted by active
code. No new status *state* is needed — `failed` + local source +
`ProposalRejected=true` + the closed code suffices.

### Report projection (fresh review D2)

`ArchitectureComponent` projects `SharedUnitRefs` and `SharedMembers`
(exact local expansion of the shared units) so the product renders shared
package scope and exact anchors without cloned ownership; the card shows
«N shared unit scope», the inspector shows a «Shared participation» section.
`ArchitectureCanvasVersion` advances with the landscape contract.

### Monotonic law (fresh review D3)

Availability monotonicity holds at the component/anchor/witness level with
stable IDs (derived from exclusive + shared members). Ownership classification
may reduce when a bad sibling's claim directly conflicts with an exclusive
claim (charter v5 conflict exception) — encoded in the metamorphic tests.

### Residual flat-wire mechanics (fresh review D4)

The flat g1/g2 grammar, `subsystem_ref` copying and count bounds remain
whole-response requirements in this program (the nested-wire migration is
explicitly deferred by the program escape). No Archive 9 run failed on these;
they are bounded response-local mechanics with closed validation.

### Adaptive units

PHASE 2 decides bounded adaptive splitting only if shared participation alone
still leaves undifferentiated broad units. Not part of this decision.

## Version/cache/replay identities

- `SynthesisPromptVersion` v17 → v18 (prompt bytes change).
- `SynthesisRequestVersion` 14 → 15 (schema: optional hypothesis, kind-less
  refs, shared participation).
- `SynthesisRecordVersion` 11 → 12 (record contract changes).
- `ProposalVersion` 10 → 11 (component may be anchor-backed shared without
  exclusive members).
- `ContractVersion` 10 → 11 (accepted landscape semantics change).
- `ArchitectureSynthesisStatusVersion` 9 → 10 (zero-useful code).
- `ArchitectureCanvasVersion` advances with the landscape contract.
- Replay: old v17 **records** fail closed (cache key mismatch) — required by
  the invariant "a saved response must never change acceptance semantics
  under unchanged identity". Raw saved response *bytes* replayed through
  `RecordSynthesisResponse` are re-evaluated under v18 semantics — the
  deliberate acceptance-replay path (same as D230's etcd replay).

## Acceptance (provider-free, Archive 9 replay)

- Telebot: Startup + Webhook publish as distinct anchor-backed shared
  participation; Middleware/Reactive/Layout without distinguishing support
  coalesce/omitted with complete accounting; no whole fallback.
- Chatto: admin/config/TLS/process/lifecycle roles survive as shared
  anchor-backed responsibilities; no cloned exclusive ownership.
- Restic: dispatcher + helpers represented honestly; no broad-unit clone and
  no silent role loss.
- etcd: stays accepted_partial (one bad optional anchor never erases
  siblings).
- Casdoor: startup/certificate/lifecycle roles survive; shared vs owned
  visible; no exact anchor-backed symbol silently lost.
- Metamorphic: permutation, duplicate injection, sibling poisoning,
  wrong-kind canonicalization, package-unit + symbol-anchor participation,
  cross-cutting broad unit, equivalent components, zero-valid → local-only,
  evidence monotonicity.

## Docs

- `PROMPT_VALIDATOR_MATRIX.md` in the D231 run dir records the full
  sentence-by-sentence audit this decision implements.
