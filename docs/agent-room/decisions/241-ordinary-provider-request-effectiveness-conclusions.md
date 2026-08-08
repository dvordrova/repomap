# 241 — Ordinary provider-request effectiveness conclusions

**Status:** ACTIVE (owner-authorized, 2026-08-08; implementation and final
acceptance pending)
**Concludes:** the bounded experiment and candidate-selection phase authorized
by Decision 239.
**Preserves:** Decisions 235–240, the one ordinary product command, local Atlas
authority, exact local remainder, request-local opaque provider identities,
offline behavior, the current privacy boundary, and Decision 239's complete
final verification gate. Historical Theme artifact compatibility is explicitly
not a product requirement; old formats fail closed and remain available in git.

## 1. Evidence standard

The D239 baseline used fresh ordinary-command runs on sqlc, Gotify, Maddy,
PocketBase, Dive, and goargs and retained all reached Architecture, Theme Scout,
and Theme Adjudication exchanges. Candidate changes were then exercised through
isolated fresh provider A/B observations and exact saved-response replay where
the disputed behavior was deterministic.

Provider output varied materially even when the provider request SHA-256 was
identical. For example, the two anchor-restoration experiments sent identical
provider requests for Maddy, Gotify, and sqlc while Maddy primary coverage moved
51→70 and sqlc moved 49→33. A one-run coverage increase is therefore not causal
evidence by itself. This decision accepts changes only where the local input,
wire, validation, or assembly correction is independently justified and the
real responses demonstrate the affected failure class.

## 2. Accepted corrections

### 2.1 Theme T1 — production-aware generic roles and ordering

Theme Scout adopts the existing generic `artifactrole` vocabulary for both its
names-only file catalog and exact seed ordering. Producer-owned entrypoint and
boundary evidence may refine a role; repository words and repo-specific tables
do not participate. Higher-value production entries, core code, effect
boundaries, and public APIs are advertised before examples, tests, fixtures,
generated files, playgrounds, experiments, and documentation.

The final ordinary gate exposed two remaining generic precedence errors: a
`main.go` under an exact `testing` segment was promoted to a production entry,
and a `main.go` under an exact `scripts` segment was promoted ahead of product
code. Exact `testing` segments are now test/fixture scope and exact `scripts`
segments are the closed `tooling` role; near-miss names such as
`testingtools`/`scriptsupport` remain production paths. These roles are consumed
by every closed local role list, so tooling is deferred rather than silently
dropped. A generic observed static call is also no longer upgraded to
`effect_integration_boundary`: only an effect-specific producer fact or the
path classifier may make that claim.

The sqlc baseline considered 1,007 paths, advertised 503 in lexical order, and
mislabelled all 503 as `production_source`. The first T1 observation advertised
574 within the same closed byte budget and moved core compiler/analyzer files
ahead of fixtures. The final gate then exposed the `scripts` and generic-call
misclassifications above; the contract correction, rather than the now-stale
intermediate role histogram, is authoritative. Gotify and Dive remained
complete while gaining the same role distinctions. Scout request identity is
v5, vocabulary identity v3, and accepted-cache contract v3.

### 2.2 Theme T3 — align prose bounds and keep local truncation readable

Scout is told the backend's already-enforced limits: at most 80 Unicode
characters for a title, 200 for a question, and 240 each for `why_it_matters`
and `expected_learning`. Adjudication is told the existing 240-character
observation limit, 120-character unknown limit, and four-unknown ceiling. The
instruction asks for short complete sentences and explicitly rejects padding
or unfinished prose.

This does not add a limit, truncate another field, relax validation, or ask for
another provider call. Fresh Maddy and Gotify observations accepted 12 and 17
themes respectively; their longest Scout fields stayed within the advertised
bounds and their longest observations were 118 and 236 characters. The value
of T3 is the truthful contract alignment, not those stochastic theme counts.

The first final PocketBase gate still exceeded the disclosed observation limit
once. The former raw-rune truncation published a final card ending mid-sentence
(`OnTerminate и `). Therefore the existing bounded normalization now keeps
the largest complete-word prefix that fits when whitespace supplies a boundary
(otherwise the largest rune-safe prefix) and appends `…`. It preserves the
useful bounded evidence instead of rolling back to a much earlier sentence.
The typed normalization count remains in status, no model-authored fact is
invented, and no useful theme is rejected.
Scout result identity advances 4→5 and Adjudication result identity 3→4; request
schemas and provider prompts do not change for this local correction.

### 2.3 Explicit-empty nested Architecture components are item-local

In the nested response grammar, an explicitly present empty `member_refs: []`
or historical `unit_refs: []` is an unambiguous component with no usable
membership. It reaches the existing `proposal.empty_component` item-local
salvage: that component is dropped, its valid siblings publish, and its exact
members remain in the deterministic local remainder.

The real sqlc response that exposed this problem was valid JSON with three
subsystems, 16 components, and 84 member refs; only the `MySQL` component had
`member_refs: []`. The former decoder rejected the complete Architecture.
Exact replay with item-local salvage retained the other 15 components as
`accepted_partial`.

This is deliberately narrow. An omitted membership field, `null`, a non-array,
both `member_refs` and `unit_refs`, an unknown ref, or another malformed shape
continues through its existing strict rejection path. An anchor never substitutes
for the required member or shared-unit identity.

### 2.4 Supporting-only production participation is item-local

A package-owned supporting symbol does not establish the missing primary
package scope of its production unit. However, one such participation does not
justify discarding unrelated valid model grouping. Supporting members from a
production unit with no selected primary member return to the deterministic
local remainder; an emptied component is dropped and valid siblings remain.
All-supporting test, tooling, and documentation units remain intentional and
are not affected.

The real PocketBase response that exposed the scope error retained 13 of 15
primary members and 72 supporting members across 11 components, but the former
whole-proposal gate discarded it for supporting-only production-unit coverage.
The accepted salvage never invents the absent primary placement and includes
four protections:

- shared-unit participation participates in effective membership;
- a primary ref removed by the existing member-ref ceiling cannot manufacture
  a supporting-only salvage condition;
- exact anchor associations that no longer intersect retained membership are
  pruned;
- the pre-salvage resolved provider proposal remains bound by its durable
  digest, while final coverage and remainder describe the published shape.

The closed recoverable diagnostic is
`proposal.supporting_only_unit_coverage_salvaged`. If no useful component
survives, the existing local fallback remains authoritative.

### 2.5 Partial Architecture is a console warning

An accepted partial Architecture is publishable but has lost model-authored
grouping coverage. The ordinary console therefore emits `WARN`, the exact
state, request/response shape, primary/supporting coverage and local remainder,
and closed validation diagnostic codes. Cached partial results use the same
warning semantics. The status contract retains only closed safe values.

No provider prose, raw source, repository path, endpoint, credential, header,
or arbitrary error string is added to console/status metadata. `report.json`
and `report.html` remain user-facing product documentation and receive no
debug-only archaeology.

### 2.6 Equivalent Theme reductions are explained outside the report

When two accepted Theme candidates reduce to one card because they have the
same exact reading set or the same normalized title/question, the reducer
retains alternate titles, questions, and readings and increments the exact
`co_projected` count. `study_themes.v1.json` v4 persists that count and validates
the partition `adjudicated = cards + omitted + co_projected`.

The ordinary console explains the backend-owned reduction in product language:
`equivalent themes merged into existing cards: N · alternate readings retained`.
The HTML keeps the resulting card and alternate readings but does not show
operational accounting jargon.

## 3. Rejected experiments

### 3.1 Prompt-only exhaustive and cohesive coverage

Both Architecture prompt variants are rejected. The exhaustive-primary prompt
raised coverage on some repositories but made Dive mirror the package tree with
22 components and produced the sqlc empty-component whole failure. The follow-up
5–15/cohesive-unit wording repaired Dive to eight components but produced a
150-ref sqlc catch-all and PocketBase catch-alls with 24 and 18 refs. These are
prompt-shaped trade-offs, not a stable root correction. The D239 production-aware
baseline prompt remains.

### 3.2 Blind equivalent-anchor deduplication

Blind anchor coalescing is rejected in both restoration forms. It reduced
Maddy/Gotify/sqlc requests from 81,253/28,318/48,639 bytes to
75,484/26,358/46,039 and reduced model-visible anchors from 65/25/31 to
36/15/18. But expanding one returned alias to every hidden exact anchor
invented component participation, while selecting one canonical-ID-sorted
callsite made a hidden representative choice the model could not express.
Multiplicity was also left ambiguously represented by unit anchor counts.
Exact anchors therefore remain distinct in the production request.

### 3.3 Caller-aware anchor context in its current form

The caller-aware follow-up is also rejected. It split only the call-target
aliases, exposed caller package/unit plus `witness_count`, and produced
49 visible Maddy anchors for 65 witnesses, 15/25 on Gotify, and 18/31 on sqlc.
Requests were 82,979, 27,240, and 46,883 bytes. Maddy then failed with empty
primary and supporting-only coverage; Gotify covered 17/18 primary and sqlc
52/73, with no stable cross-repository improvement that justifies the new
weighting and caller abstraction. This rejection does not prohibit a separately
approved future typed-handoff experiment.

### 3.4 Full unit package-import adjacency

The complete directed unit-pair import matrix is rejected. It was bounded and
did not expose paths or canonical IDs, but it serialized 226 pairs for Maddy,
98 for sqlc, 11 for Dive, and 52 for PocketBase. Request bytes increased
81,253→96,634, 48,639→55,405, 14,642→15,589, and 46,492→50,129 respectively.

The product effect was inconsistent: locally derived Canvas structural edges
moved 45→18 on Maddy, 13→33 on sqlc, and 23→15 on Dive, while PocketBase's
provider proposal failed the former supporting-only gate. Maddy collapsed into
large overlapping components. Static import topology changed grouping
granularity but did not establish a generally better conceptual map. Decision
223's bounded `relation_out_count` remains; no raw or pair-expanded internal
graph is added to the provider request.

### 3.5 Architecture-backed Theme seeds

Theme Scout does not gain source seeds selected from Architecture components.
The experiment sometimes improved Gotify and Dive, but amplified stochastic
Architecture boundaries and left sqlc dominated by test-database, setup, and
script themes; its model-visible component seeds represented Generators, Test
Utilities, and Scripts rather than the core compiler. The first form also
seeded components already represented by exact Study targets, and the corrected
form still introduced component-binding caps and replay identity not justified
by stable product improvement. Architecture labels remain bounded editorial
context; they do not become new source authority.

### 3.6 Adjudication whole-file source deduplication

Dropping an expanded file whenever the same path appeared in anchor evidence is
rejected. It reduced adjudication wire bytes by 57% on Maddy (281,277→121,077),
46% on PocketBase (215,671→116,286), and 34% on Gotify (81,933→54,342), but
removed broader context rather than duplicate spans. Unknowns increased
12→24, 10→20, and 0→6 respectively, and supported observations became less
specific. Adjudication keeps the complete existing bounded source expansion.

## 4. Product and privacy boundary

D241 adds no product flag, semantic stage, retry, shell entrypoint, repository-
specific classifier, presentation layer, or model-authored connectivity. The
ordinary cold path remains at most Architecture → Theme Scout → Theme
Adjudication. No full file tree, raw internal edges, canonical Atlas IDs,
unadvertised paths, credentials, or Authorization headers enter provider
requests or diagnostics. Offline runs remain provider-free.

Contract changes use explicit current identities and clean cache invalidation.
Only the current Theme request format has a production reader and provider-free
replay path. Pre-D241 and experimental intermediate request/result/status
formats fail closed; git is their history. Rejected experiment branches and
payloads are not production compatibility contracts.

## 5. Required final gate

D239 is not complete when implementation compiles. Before completion:

1. run focused tests for every changed Architecture decode/salvage/status and
   current Theme request/replay contract;
2. run `go test -count=1 ./...`, `go vet ./...`, `git diff --check`, and build
   one exact clean candidate with `go build -trimpath`;
3. clear semantic caches, then run the exact ordinary command on sqlc, Gotify,
   Maddy, PocketBase, Dive, and goargs, using the remote-specific URL flag and
   no experimental flag or alternate entrypoint;
4. verify exit status, manifest, Atlas, report JSON/HTML, every reached stage
   status and semantic exchange, request/response hashes and bytes, partial
   warnings, final groupings, themes, source associations, and local remainder;
5. clear caches again and run fresh ordinary-command acceptance on etcd and
   casdoor with the same artifact and product inspection.

The six corpus runs and the final two regression runs may use the owner's
authorized concurrency, but each is one fresh observation rather than a retry
until a preferred response appears. Debug artifacts, provider payloads,
binaries, credentials, and corpus-specific paths are never committed.
