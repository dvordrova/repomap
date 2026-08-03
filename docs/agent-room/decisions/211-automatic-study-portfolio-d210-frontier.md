# Decision 211: Automatic Study Portfolio over the D210 Frontier

## Status

Approved in direction by the repository owner. This is the final numbered
decision after the D210 Unicode entry-handoff regression gate (committed as
`test: cover Unicode entry handoff locators`) and every recorded provider-free
gate. The decision is active after this checkpoint. Production implementation
is not performed in this checkpoint turn and begins only as the separately
listed implementation lanes below.

One final fresh Casdoor semantic acceptance run is owner-authorized now, only
after every provider-free gate in this decision is green. It is one product
acceptance run, not a prompt-tuning loop.

## Observed problem

D210 fixed the upstream cause of the narrow TLS-heavy shelf and produced exact
typed backend-owned spans and first-hop entry handoffs. The default product is
still wrong in three ways:

1. **The default ask produces a startup-heavy shelf.** The D210 request asks for
   one direction for every advertised span up to a 12-item ceiling, and the
   advertised span set is seeded by role, so on an ordinary executable
   repository the entry/handoff/call-boundary focused spans dominate and the
   default output remains startup/TLS/certificate-dominated. Rephrasing the
   prompt cannot fix an ask that forces coverage of every advertised span.
2. **Questions are role-generic, not target-specific.** A focused span question
   reads "Where does this application process start?" regardless of which exact
   symbol the source card carries, so route titles are not distinguishable.
3. **The earlier D211 draft misused evidence shapes and response structure.** A
   separate `--study-focus-lane` flag treated internal evidence-shape roles as
   user intentions and left the default output intentionally narrow, and a
   separate portfolio array added a redundant response shape. Both are
   rejected.

## Outcome A: the ordered directions array is the portfolio

There is no separate portfolio array, field or object in the request, response,
catalog or artifact.

- The ordered `directions` array returned by the provider is the portfolio.
- **Array order is rank.** The first returned direction is the highest-ranked
  selection.
- **`why_it_matters` is the bounded rationale** for that selection, under the
  existing bounded-text and exact-locator prose rules.
- The backend derives the authoritative selected span refs **only from locally
  valid directions**. A direction that fails item-local validation never
  contributes a selected span ref, even though it counts as a rejected sibling
  for status.
- The model chooses, ranks and explains supplied spans by which advertised
  spans it returns directions for, in what order, with bounded rationale prose.
  It does not create paths, relations, support, authority, ownership, canonical
  IDs or questions.

**Cardinality.** The desired direction count is 6–10 and is stated to the model
as the target. The valid production cardinality is 1–10 accepted directions.
Zero valid directions is a failure. The backend never forces filler or padding:
a valid response with fewer than six directions is not rejected and receives no
synthetic additions.

## Outcome B: exactly three cardinality levels

1. **Considered spans — the complete locally supported D210 span set.** Every
   focused span compiled from an exact support and every system-path span
   compiled from one exact producer relation, exactly as D210 constructs them.
   This set is bounded only by the existing hard producer/resource limits
   (for example request/status artifact bytes, `MaxReadingTargets` and the
   evidence/document bounds). No advertised-span cap is applied to it.
2. **Advertised spans — the request frontier.** One unambiguous limit
   `MaxAdvertisedSpans`, initially **32**, unless exact byte measurement of the
   real provider request requires a lower documented value. The frontier is
   compiled from the complete considered set by deterministic breadth selection:
   every observed support role is represented or Atlas Study is provider-free
   unavailable with a closed reason; within each role exact target-package
   buckets round-robin so one repeated package family cannot monopolize the
   frontier; system-path spans remain eligible only through their exact producer
   join; selection order is a request budget mechanism, never semantic
   importance.
3. **Model output — `MaxDirections`, initially 10.** This is the maximum number
   of accepted directions and the ceiling for the returned directions array.

The old `MaxRouteSpans` meaning is removed. There is exactly one advertised-span
limit (`MaxAdvertisedSpans`) and one model-output limit (`MaxDirections`); two
ambiguous advertised-span limits are not retained.

## Outcome C: four distinct stages and independent coverage flags

The report, result and status keep four separate stages:

- **considered spans** — the complete locally supported D210 span set;
- **advertised spans** — the request frontier (`MaxAdvertisedSpans`);
- **model-selected spans** — the distinct spans referenced by the returned
  directions;
- **locally accepted spans** — the distinct spans referenced by directions that
  pass exact item-local validation.

An advertised span with no returned direction is normal **`not_selected`**. It
is never reported as uncovered and never turns a result into `accepted_partial`.

`coverage_complete` is not overloaded. Four flags are recorded independently:

- **`frontier_complete`** — the advertised frontier equals the complete
  considered set (zero omissions);
- **`selected_items_complete`** — every model-selected direction is locally
  valid and at least one direction was returned (no returned sibling rejected);
- **`support_coverage_complete`** — every locally accepted direction covers all
  required support identities of its exact span, with no padding and no
  partial-support acceptance; item-local validation keeps this invariant for
  every accepted direction, and the flag is recorded independently;
- **`portfolio_target_met`** — the number of locally accepted directions is
  within the desired 6–10 band; this is independent of status and does not by
  itself invalidate an otherwise exact result.

## Outcome D: status semantics

- **`accepted`** — every returned selected item is locally valid
  (`selected_items_complete` true). This holds regardless of how many advertised
  spans were `not_selected` and regardless of `portfolio_target_met`.
- **`accepted_partial`** — at least one selected item is valid and at least one
  returned sibling is locally rejected. Rejected siblings are recorded as
  bounded item-local diagnostics with closed codes; `not_selected` spans never
  contribute here.
- **`failure`** — zero valid selected items (zero valid directions), including
  an empty or fully-rejected directions array. No synthetic local Study
  portfolio is created.

`portfolio_target_met` is recorded separately. Fewer than six valid directions
must not, by itself, turn an otherwise exact result into `accepted_partial` or
`failure`.

## Outcome E: backend-owned target-specific questions

Questions remain backend-owned and are restored from the exact span; the
provider returns only the span ref and never the question.

- A focused question may use its exact source-card symbol/label for its one
  allowed reading target.
- A system-path question may use both exact endpoint symbols/labels.
- No other repository value (role name, path, line, package, component) enters
  the question, and no model-authored question, path, relation, authority or
  canonical ID is accepted.
- Both localized questions are generated together under the existing
  bounded-text and terminal-`?` rules and bind the exact request identity, so
  route titles are target-specific and distinguishable.

## Outcome F: bounded omissions and reporting

- Omission diagnostics are bounded: aggregate counts by closed omission reason
  plus a bounded set of representative typed refs and the complete candidate
  SHA-256. An unbounded omission list is never persisted.
- The report distinguishes considered, advertised, model-selected and locally
  accepted spans; the candidate SHA; role/package concentration (considered/
  selected counts per support role and per exact target-package bucket); the
  four independent coverage flags; and the bounded omission aggregates.

## Outcome G: semantic failure

On any semantic failure (provider failure, decode, reference, validation or
resource):

- the local Architecture remains visible and untouched;
- no synthetic Study portfolio is created;
- the failed new Study run is honestly unavailable with a closed code;
- historical self-contained reports remain untouched.

## Outcome H: provider-free honesty and the Casdoor acceptance run

- Provider-free fixtures prove the contracts and deterministic selection. They
  do **not** prove semantic usefulness.
- After every provider-free gate below is green, one fresh Casdoor semantic run
  is authorized. It is one product acceptance run, not a prompt-tuning loop, and
  any semantic failure stops the checkpoint without authorizing prompt iteration
  or a retry loop.
- The run must review the actual default Study shelf for:
  - target-specific distinguishable titles;
  - useful comparative selection (ranked, bounded rationale);
  - no forced filler;
  - exact source navigation;
  - considered / advertised / selected / accepted diagnostics;
  - material improvement over the accepted D210 shelf.

## Identity and artifact contract

- **No `focus_lane` field** is added to the provider wire, the catalog, the
  request, the response or any artifact. `process_entry`, `entry_handoff` and
  `observed_call_boundary` remain internal producer-owned `SupportRole` evidence
  shapes.
- **No separate portfolio array** is added to the response envelope; the ordered
  directions array carries the portfolio.
- Contracts actually changed advance:
  - Atlas Study request/catalog version v6 → v7 and prompt v12 → v13 (the
    `MaxAdvertisedSpans` frontier, the removed `MaxRouteSpans` meaning, the
    desired-6–10 ranked-ask and target-specific question text change the request
    and prompt bytes);
  - Atlas Study result/status v7 → v8, with `atlas_study_request.v7.json`,
    `atlas_study_result.v8.json` and `atlas_study_status.v8.json`;
  - the accepted cache contract to `atlas-study-accepted-v7`;
  - the Atlas Study report projection to v6 (four-stage diagnostics, independent
    coverage flags, bounded omissions, target-specific questions).
  - Request, catalog, result, status, replay, semantic exchange and manifest
    identities bind the exact frontier/selection projection, candidate-set
    digest, language, limits and final provider request bytes.
- Contracts unchanged by D211 (not bumped): Architecture Grounding stays v5
  (no producer change), the Repository Atlas canonical core, the D210 span
  construction and `SupportRole` enum, localization, and Overview/Architecture
  contracts.
- D209/D210 responses miss closed under the new cardinality and status
  semantics. No compatibility reader, active-artifact migration or
  reinterpretation; historical static HTML remains self-contained.

## Acceptance

- `go test ./...` and `go vet ./...` pass.
- The recorded D210 Unicode entry-handoff regression gate remains green.
- Provider-free gates: the complete considered set compiles to an advertised
  frontier with role seed and package round-robin; permutation produces
  byte-identical considered set, frontier, digest, coverage flags and bounded
  omissions; exact saved fixtures replay ordered directions under the new
  status/cardinality semantics with zero provider calls; item-local rejection of
  unknown, wrong-kind, raw-canonical, cross-request, duplicate-span and
  padded directions is preserved; `not_selected` never becomes uncovered or
  `accepted_partial`; 6–10 is desired, 1–10 is valid, zero is failure; the four
  coverage flags are recorded independently.
- The built binary is exercised provider-free on the product fixture and a
  large nearby Go repository with exit state, artifacts, manifest and report
  checked directly.
- Only after all of the above, the one authorized fresh Casdoor acceptance run
  reviews the actual default shelf per Outcome H.

## Implementation lanes

1. **Frontier lane** — `internal/atlasstudy/selection.go`, `model.go`,
   `compile.go`: `MaxAdvertisedSpans` (32), `MaxDirections` (10), removal of the
   `MaxRouteSpans` advertised meaning, coverage-aware frontier with bounded
   omission aggregates, target-specific question compilation; coverage tests.
2. **Response/status lane** — `prompt.go`, `response.go`, `artifact.go`,
   `replay.go`: ranked directions as portfolio, why-it-matters rationale,
   1–10 cardinality, the three status outcomes, the four independent flags,
   accepted-cache contract; contract identity tests on constructed fixtures.
3. **Runtime/report lane** — `cmd/repomap/atlas_study_runtime.go`,
   `internal/report/atlas_study.go`: four-stage diagnostics, flags, bounded
   omissions, target-specific question projection, manifest binding.
4. **Fixture/acceptance lane** — provider-free saved-response fixtures, direct
   built-binary gates, then the authorized default-shelf Casdoor acceptance run.

The D210 producer (handoffs, supports, relations, span construction) is not
reimplemented. No production code changes are part of this checkpoint.

## Explicitly out of scope

A focus flag or any user-facing lane; a separate portfolio array or second
semantic call; promoting `process_entry`, `entry_handoff` or
`observed_call_boundary` to user-facing concepts; model-authored questions,
paths, relations, support, authority, ownership or canonical IDs; GoSurvey;
Boundary/Resource roots or producers; Tree-sitter; deeper SSA/DFS; library-first
routes; raw Orientation; source bytes on the model wire; prompt-only breadth
tuning; forced filler or padding; fuzzy repair; semantic retry; choose-first
ownership; hidden remainder; a compatibility reader or migration; a broad UI
redesign; a second live run or any prompt-tuning loop.
