# Decision 226: Mechanism Evidence Contract and one honest vertical fragment

## Status

ACTIVE (Phase 4 of the overnight program
`hermes-repomap-overnight-goal-v3.txt`, approved by the repository owner's
standing goal authorization 2026-08-05). Provider-free; no new semantic
stage; no repository-wide call graph; no invented edges.

## Question asked before any semantic stage

"Does one additional model call create more product value than local
retrieval, source evidence, or UI work?"

Answer: NO evidence-backed yes. The existing local producers (gofacts,
SSA behavior-handoff projection, Atlas boundary/resource observations,
exact anchor retrieval) already support the vertical fragment below with
exact saved evidence. No new model call is added.

## Per-transition claim contract (closed set)

Every displayed mechanism transition carries exactly:

- `claim_kind` — closed set:
  `process_entry | exact_registration | direct_static_call |
  resolved_static_dispatch | structural_dependency | publish_callsite |
  consume_registration | storage_boundary_callsite |
  outbound_client_callsite | runtime_observed_transition |
  interpreted_role | unresolved_continuation`
- `support_mode` — closed set:
  `observed_local | resolved_static | runtime_observed | corroborated |
  interpreted | unknown`
- exact evidence refs and a source action (path:line:column, provenance
  provider/version/operation, scenario);
- build/runtime scenario label;
- limitations (always visible, never hover-only);
- `ordering` — closed set:
  `exact_local_order | resolved_path_order | partial_order | not_established`

Mappings from current local data (Archive 6 verified):
- behavior_handoff structural edge (go_ssa, certainty static,
  scenario "Recorded Go build scenario") →
  claim_kind=`direct_static_call`, support_mode=`resolved_static`,
  ordering=`resolved_path_order`;
- the process entry anchor (kind `process_entry`, proof_mode
  `process_entry` or `call_target`, exact location) →
  claim_kind=`process_entry`, support_mode=`resolved_static`,
  ordering=`exact_local_order`, evidence derived from the anchor's actual
  proof mode — a process entry is a declaration/entry anchor, never a
  runtime dependency or registration claim;
- behavior anchor with proof_mode `call_target` →
  support_mode=`resolved_static`; `declaration_family` →
  support_mode=`observed_local`;
- Atlas boundary/resource observation (evidence location + provenance
  detail = imported path) →
  claim_kind=`storage_boundary_callsite` / `outbound_client_callsite`,
  support_mode=`observed_local`;
- a transition whose target has no further locally observed handoff →
  `unresolved_continuation` with support_mode=`unknown`, ordering=
  `not_established`, and the honest frontier copy.

## Representation rules

- A single-repository Overview is an analyzed repository perimeter, NOT a
  C4 System Context diagram. Repository scope, deployable/container scope
  and software-system scope are separate and never conflated.
- An imported package/client constructor is an observed external
  touchpoint, not automatically an external software system.
- Architecture remains a conceptual/static list plus optional canvas.
- A selected mechanism prefers a process-data/DFD-like fragment: entry →
  locally supported transitions → observed boundary/resource → explicit
  unresolved frontier. A partial or disconnected mechanism with an
  explicit unknown frontier is a successful honest result.
- Swimlanes only when participant ownership AND ordering are supported;
  SIPOC may be a compact optional summary, never the main graph; BPMN
  gateways/events/message flows/compensation and FFBD sequencing are
  forbidden unless those exact semantics are locally supported; concept-map
  ideas organize editorial understanding but grant no runtime authority.

## Vertical fragment (one, honest, casdoor)

Surface/process entry → locally supported operation transitions → observed
boundary/resource → exact evidence and explicit unresolved frontier.

1. `process entry` — `github.com/casdoor/casdoor.main`, main.go:36,
   anchor kind process_entry (proof_mode process_entry).
2. `direct_static_call` — main.go:150:16 → `service.Start`
   (behavior_handoff, go_ssa, scenario "Recorded Go build scenario"),
   ordering `resolved_path_order`.
3. `direct_static_call` — ldap/server.go:61:30 → `ldap.getTLSconfig`
   (behavior_handoff, go_ssa), boundary anchor kind
   tls_or_security_boundary (proof_mode call_target).
4. `storage_boundary_callsite` / `outbound_client_callsite` rows from
   Atlas observations (Decision 225 associations) with exact witnesses.
5. `unresolved_continuation` — beyond the observed handoffs, no further
   locally saved transition exists; frontier is explicit and empty, never
   invented.

Rules honored: no invented edges to make a continuous path; no execution
order inferred from package imports (ordering comes only from SSA handoff
order and call_target anchors); no model prose turned into transitions; no
repository-wide call graph; no new semantic call.

## Provider-free acceptance

1. Contract unit tests: every transition row carries claim_kind,
   support_mode, evidence, scenario, limitations, ordering from the closed
   sets; frontier transitions are unresolved_continuation with
   not_established ordering.
2. Fragment tests: the casdoor vertical fragment (main → service.Start →
   ldap.getTLSconfig) resolves entirely from saved report data with exact
   locations; no synthesized edge.
3. UI/DOM tests: fragment renders as a compact DFD-like list; limitations
   and frontier always visible; EN/RU parity; no BPMN/SIPOC-main/swimlane
   claims.
4. Full gates: gofmt, `go vet ./...`, `go test -count=1 ./...`, `make
   build` → `.bin/repomap`, node --check on touched assets, report/manifest
   round trip, golden regen.

## Non-goals

- No complete business-process proof; no new semantic call; no repository-
  wide call graph; no C4/System Context claims; no BPMN gateways/events/
  message flows/compensation; no FFBD sequencing; no swimlanes without
  ownership+ordering; no SIPOC as main graph; no push.
