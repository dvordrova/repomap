# Decision 227: cross-cutting member participation instead of partition rejection

## Status

ACTIVE (bounded Architecture corrective; owner-approved direction: the model
must not be penalized for honest participation, and the user benefits from
more knowledge — they will verify).

## Problem (evidence)

Fresh casdoor run `20260806-041149-casdoor-504783179b19`:

- The model proposed four components: «Запуск приложения» (u1), «Сервер»
  (u2), «Сертификаты» (u2), «Провайдеры жизненного цикла» (u2).
- One unit `u2` (the server package) participates in three conceptual
  components. The validation rejected the whole proposal with
  `proposal.duplicate_component_identity` because the three components share
  the same exact member set `{u2}`.
- Result: the user saw no model architecture at all (local fallback), even
  though the proposal carried honest knowledge: one package serves several
  roles (server, certificates, lifecycle providers).

This is not a model defect: it is the current contract treating exclusive
ownership (partition) as mandatory. The same class was already resolved in
the core-refactor line (codex, D200 era): cross-cutting membership is
participation, not ownership.

## Decision

1. A unit may appear in several proposed components. Membership expresses
   **participation**, never exclusive ownership.
2. The prompt no longer demands «assign each member ref at most once».
   Instead it instructs: never repeat a member ref within one component; a
   genuinely cross-cutting member may appear in several components.
3. Validation change: `proposal.duplicate_component_identity` is removed as
   a hard rejection for components that merely share members. It remains a
   hard rejection only for an **exact twin** — two components with identical
   name, description, member set and anchor set (a literal copy, no added
   knowledge).
4. The user-facing report keeps the full proposal; a component whose member
   participates elsewhere shows an honest «also participates in …» note so
   the shared membership is visible, not hidden.
5. No new analysis stage, no new provider call, no runtime claims.

## Bounds

- Same scope as D216/D223: units, anchors, relations unchanged.
- The local fallback (failed synthesis) still exists for genuine validation
  failures (unknown refs, malformed components, exact twins).
- No change to: D225 association join, D226 fragment, D224 study.

## Acceptance

- Fresh casdoor-class proposal (shared member across components, distinct
  names/descriptions) is accepted and renders with participation notes.
- Exact twin proposal (same name+description+members+anchors) still fails
  closed with `proposal.duplicate_component_identity`.
- Provider-free tests: both cases replay without a model call.
- Request wire versions bumped (request 13, prompt v16).
- Full gates: build, vet, tests, golden, browser EN/RU.
