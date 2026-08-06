# Decision 228: loosen over-strict Architecture caps (limit audit)

## Status

ACTIVE (bounded Architecture corrective; owner doctrine: challenge every
warning/error/limit with three questions — is it true, how much worse without
it, does it serve giving the user more facts — and loosen where the result
gets better).

## Evidence (real runs)

1. **etcd `20260805-205931` (and 3 more etcd runs)** — the model proposed 16
   honest components in ONE subsystem («Ядро сервера», «Хранилище»,
   «Сетевой транспорт», «Устойчивость и отказоустойчивость», …).
   `MaxComponentsPerSubsystem = 8` rejected the whole proposal
   (`response.invalid_proposal`), so the user saw no model architecture at
   all. The cap is an internal UX guess, not a truth limit.
2. **casdoor runs** — rejected by `duplicate_component_identity` (fixed in
   D227 as participation).
3. Prompt says «hypothesis is required wire syntax but only advisory model
   input: the backend derives the product hypothesis status exclusively from
   exact process_entry/call_target proof» — yet the validator rejects the
   whole proposal when the model's advisory hypothesis flag differs from the
   backend's derivation (`proposal.ungrounded_primary_component`). The model
   is punished for an advisory field the backend computes itself.

## Decisions

1. `MaxComponentsPerSubsystem` 8 → 24; `MaxTotalNestedComponents` 24 → 48;
   `MaxPrimarySubsystems` 8 → 12; `maxSubsystems` (validate) 16 → 24.
   Large modular servers (etcd-class) honestly need more than 8 nested
   components; the general caps remain as runaway protection.
2. `maxConceptualMembershipsPerMember` 8 → 32 (a package can genuinely
   participate in many roles; D227 makes this reachable).
3. `maxAnchorMembers` 16 → 64.
4. `proposal.ungrounded_primary_component` is no longer a hard rejection:
   the backend derives hypothesis exactly as the prompt promises and
   overwrites the model's advisory flag locally (deterministic), never
   rejecting the proposal for it.
5. Prompt guidance updated to match: «prefer one to six component records
   per subsystem and no more than forty-eight component records in total»;
   subsystems «never more than twelve».
6. Versions: SynthesisRequest 13 → 14, prompt v16 → v17.

## Non-goals

- No change to unknown/wrong-kind ref rejection (real errors stay fail-closed).
- No change to duplicate-member-within-component rejection.
- No new analysis or provider call.

## Acceptance

- etcd-class proposal (16 components in one subsystem) accepted and rendered.
- Advisory hypothesis mismatch accepted; backend hypothesis wins.
- Provider-free tests for both; request wire v14; full gates.
