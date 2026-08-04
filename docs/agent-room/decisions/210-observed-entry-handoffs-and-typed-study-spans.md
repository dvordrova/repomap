# Decision 210: Observed entry handoffs and typed Study route spans

## Status

Owner-authorized after final acceptance of Decision 209. The repository owner
delegated selection of the next product direction after the working Atlas-first
UX to this review thread. This decision is that bounded direction.

Feature implementation remains held until the decision handoff is delivered.
After that handoff the parallel lanes below are authorized. D209 remains the
accepted product baseline and must stay usable throughout implementation.

D210 changes a local producer contract and the Atlas Study provider request.
It is not a prompt-only calibration. Provider-free fixtures and replay gates
come first. One fresh Casdoor provider run is authorized only as the final
installed-binary acceptance after every provider-free gate is green; it is not
an iterative prompt-tuning loop.

## Observed problem

The final D209 Casdoor Study request contains ten exact reading targets: the
process entry, `service.Start`, and eight TLS/certificate functions. This was
not caused by `MaxReadingTargets`: ten is far below the active limit of 512.
The loss happens before Atlas Study compilation.

`report.atlasStudyReadingTargets` currently starts from exact saved source
snippets and admits a locator only when `atlasStudyReadingAssociation` finds an
exact Surface, Navigator evidence, saved flow, process entry or behavior
`call_target`. Package declarations and declaration-family motifs are correctly
drawer-only. On Casdoor there are no saved flows, Navigator points at `main`,
most non-entry server Surfaces are partial, and the behavior call-target
producer is reached only after a name/path classifier recognized generic
`Start` and TLS/certificate declarations. The model therefore never sees the
rest of the repository-shaped startup handoffs. Rephrasing the prompt cannot
recover facts absent from the request.

There is a second contract hole. A provider-authored free-form question can
promise a system path while selecting one focused source. Exact ref validation
can prove which targets were selected, but cannot prove that arbitrary prose is
supported without language-dependent lexical rules, NLP inference or silently
weakening the promise.

## Outcome A: exact first-hop entry handoffs

The existing surface-discovery analysis publishes a separate producer-owned
`entry_handoffs` collection in Architecture Grounding v5. An entry handoff is
not a BehaviorAnchor, does not receive a semantic architecture kind and is not
added to Architecture synthesis candidates or relations.

Each handoff records:

- the exact build-selected production process-entry declaration;
- one exact repository-local static callee declaration reached directly from
  the process-entry function body;
- an exact representative callsite and the complete bounded witness count;
- the target package, build scenario, certainty and producer provenance; and
- the closed limitation that this is a direct static source edge, not proof of
  runtime order, successful execution, ownership or transitive reachability.

The producer uses the SSA program already built by surface discovery. It does
not load packages again, rebuild SSA or CHA, run a new depth traversal, or add
keywords. Dynamic/interface calls, external callees, recursion back to the
same entry, `init`, tests, examples and helper executables are not promoted to
production handoffs. Multiple exact
calls for the same typed `(entrypoint, callee)` edge merge locally; order never
chooses an owner or representative semantic role.

Grounding coverage records considered and published handoff counts, a
candidate-set SHA-256 and explicit collection/persistence-limit reasons. A
bounded prefix may never claim complete coverage. Older grounding can still
render its self-contained historical report, but it cannot be reinterpreted as
a complete D210 handoff producer.

The new collection is deliberately narrower than a general symbol taxonomy.
Classifying all symbols as utility, client, domain, storage and similar roles,
deeper entrypoint traversal, and library-first analysis remain later product
decisions. D210 targets ordinary executable Go repositories; a repository with
no production process entry may still use independently observed Surface or
saved-flow spans, otherwise Study is honestly unavailable without padding.

## Outcome B: producer-owned reading support

Atlas Study separates an exact reading locator from the reason it is eligible.
A target has zero or more exact private support identities. Support has a
closed producer-owned role and authority. The initial roles are evidence
shapes, not application-domain labels:

- `process_entry`;
- `entry_handoff`;
- `surface` for an exact producer Surface;
- `surface_candidate` for an exact locator with explicitly partial Surface
  authority;
- `observed_call_boundary` for the existing exact static call-target proof;
  and
- `saved_flow` for an existing exact locally saved flow step.

Navigator selection and model Architecture membership may add exact principal
or related-navigation context, but they cannot create a support role or make a
locator eligible by themselves. Declaration-family motifs and bare package
declarations remain drawer-only. A partial Surface never becomes an exact
Surface, owner or runtime path.

All evidence attached to the same typed locator is merged. One locator may
support several roles and several conceptual components. No source, component,
owner, role or support is selected by sorted-first, choose-first or fuzzy
matching.

Before compilation the backend computes the complete eligible target and span
candidate sets, then performs deterministic breadth selection across observed
support-role lanes and, within each role, round-robins exact target-package
buckets. Selection order is a request budget mechanism, never semantic
importance or authority. A repeated family from one package cannot consume
every slot while another observed package bucket remains available.

The private request artifact and closed status record the complete candidate
SHA-256, considered/selected counts, per-role counts and per-package bucket
counts. Permutation of producer input is byte-stable. Omitted candidates are
therefore explicit partial selection, not hidden first-N loss. If the
configured target or direction budget cannot represent every observed support
role at least once, Atlas Study becomes provider-free unavailable with a closed
reason instead of silently dropping a role or increasing a limit. More package
buckets than direction slots produce explicit partial candidate coverage; they
do not make an otherwise useful large-repository Study unavailable.

## Outcome C: backend-owned typed route spans

The backend compiles request-local `route_support` and `route_span` objects.
Each span contains:

- a private canonical identity and short request-local `span_ref`;
- a closed structural kind (`focused` or `system_path`);
- a locally generated English or Russian question;
- exact required support refs; and
- the exact reading-target refs allowed to answer it.

A focused span promises only inspection of one observed evidence boundary. A
system-path span contains at least two distinct exact locators and exists only
when an exact producer relation already joins its
clauses. In D210 that means a production process entry joined to its exact
first-hop handoff, or an already saved exact flow whose selected portion fits
the existing one-through-five reading contract. The backend does not combine
unrelated roles because their labels sound compatible. A partial Surface may
form a focused candidate span but never a system path.

The question is no longer provider-authored. The provider returns exactly one
`span_ref` per direction plus `why_it_matters`, `learning_outcome`, job/stage,
principals and the ordered reading list. The resolver restores the localized
question from the exact span. Question text, limitation prose, labels, paths
and symbols never participate in a support join.

For each direction the resolver requires:

- an exact request-local span ref of the correct kind;
- one through five distinct reading targets, preserving D207;
- every selected target to be in that span's exact allowed set and to cover at
  least one required support identity;
- every required support identity to be covered by at least one selected
  target, plus at least two distinct targets for a system-path span; and
- all existing exact principal, component, surface, locator, privacy,
  language and response-size checks.

An unrelated target is padding and rejects that direction. Unknown,
wrong-kind, raw-canonical, cross-request, duplicate-span and incomplete-span
responses reject item-locally. Valid siblings and the validated Brief remain
intact. There is no fuzzy repair, semantic retry, locally invented route or
choose-first fallback.

The request asks for one direction for every advertised span, up to the
existing explicit maximum. A response covering every advertised span is
`accepted`; one with at least one valid direction and explicit uncovered spans
is `accepted_partial`. Zero valid directions remains a validation failure and
does not publish a synthetic local Study. Accepted-partial results are eligible
for the accepted-only cache only after the same exact replay validation as
complete results.

Canonical direction identity is derived from the canonical span identity,
principals and ordered exact targets, not localized or provider prose. Exact
duplicate suppression uses `(span identity, reading set)`; different spans
with the same locator do not collapse accidentally. Report route titles come
only from the backend-owned span question. Lexical question-coverage remains a
diagnostic and cannot accept, reject, hide or rename a route.

## Identity and artifact contract

- Architecture Grounding advances to v5 and includes typed handoffs plus
  truthful handoff coverage. Handoffs are explicitly excluded from the
  Architecture synthesis request, so this decision does not grant new model
  Architecture authority.
- Atlas Study request/catalog version advances to v6 and prompt to v12.
- Atlas Study result/status advance to v7; request/result/status artifact names
  advance with them.
- the accepted cache contract advances to `atlas-study-accepted-v6`;
- the Atlas Study report projection advances to v5; and
- request, catalog, result, status, replay, semantic exchange and manifest
  identities bind the exact support/span projection, candidate-set digest,
  language, limits and final provider request bytes.

D209 responses without a `span_ref` miss closed. There is no compatibility
reader, active-artifact migration or reinterpretation of an old accepted
result. Historical static HTML remains self-contained.

## Acceptance

### Producer and shelf

- a neutral-name fixture with `main` directly calling declarations in two
  repository packages publishes two exact entry handoffs without TLS, auth,
  server or Casdoor keywords;
- repeated callsites for one typed edge merge with the exact witness count;
- dynamic/interface, external, test/example/helper and `init` calls do not
  become production handoffs;
- handoff collection reuses the existing package load, SSA and CHA; a D210 run
  performs no second build and no deeper call-graph traversal;
- a partial Surface remains `surface_candidate` with partial authority and
  cannot grant owner, exact-surface or system-path semantics;
- declaration-family and package-drawer locations remain in source navigation
  but are not Study support;
- equal locators merge plural supports and principals without choosing one;
- input permutation produces byte-identical grounding and Atlas Study request;
- Chinese identifiers and paths round-trip as exact local locators without an
  English token floor or lexical classification; and
- selection covers every observed support role once or becomes explicitly
  unavailable; within each role it rotates across exact package buckets and
  records omitted buckets, so one repeated package family cannot silently
  monopolize the shelf.

### Span and replay

- a process-entry-only focused span can ask only its local backend question; it
  cannot publish a provider question promising request handling;
- an entry-handoff system span requires both the entry and handoff clauses;
  selecting only one rejects item-locally, while exact coverage accepts;
- identical labels, paths or symbols with different support identities never
  join;
- one locator carrying two exact supports remains one locator and cannot make a
  one-file route masquerade as a system path; an unrelated fifth target rejects
  as padding;
- unknown, wrong-kind, raw-canonical and cross-request span refs fail closed;
- one and five readings remain valid, while zero and six remain invalid;
- tampered question, support, allowed-target or span identity fails result and
  cache replay validation;
- EN/RU requests share typed topology but bind different exact localized
  questions and request identities;
- complete and partial span coverage are distinguished consistently in result,
  status, cache, report and manifest; and
- the new saved-response acceptance fixture reaches a reviewable HTML report
  with zero provider calls. Old D209 response artifacts are not rewritten.

### Product verification

- `go test ./...` and `go vet ./...` pass;
- the normal binary is built and exercised provider-free on the product
  fixture and a large nearby Go repository, with exit state, artifacts,
  manifest and report checked directly;
- the large-repository run records handoff/target/span coverage and does not
  repeat package loading or SSA solely for D210; and
- only after those gates and exact binary installation, one fresh Casdoor run
  may verify that the offered spans and accepted routes are no longer
  TLS/certificate-dominated. Any semantic failure stops the checkpoint; it
  does not authorize prompt v13 or a retry loop.

## Parallel implementation lanes

After this handoff, two lanes may start independently against the fixed types:

1. **Producer lane** — `internal/surfacediscovery/analyzer.go`,
   `internal/surfacediscovery/model.go`, grounding artifact normalization and
   coverage/version tests; no Atlas Study or UI edits.
2. **Typed contract lane** — `internal/atlasstudy/model.go`, `compile.go`,
   `prompt.go`, `response.go`, artifact/replay validation and contract identity
   tests using constructed support/span inputs; no producer inference.

After both converge:

3. **Report projection lane** — `internal/report/architecture_grounding.go`,
   `source_snippet.go` and `atlas_study.go`: exact handoff source locators,
   support merge, observed-lane selection, span construction and truthful
   coverage. This lane may make only the minimal UI projection needed to show
   backend-owned questions and complete/partial status; no IA redesign.
4. **Runtime acceptance lane** — `cmd/repomap/atlas_study_runtime.go`, cache,
   saved-response replay, report/manifest binding, direct binary fixtures and
   the final installed-binary acceptance.

Producer and typed-contract lanes are parallel. Report integration depends on
their types. Runtime/cache/manifest acceptance starts after report integration
has one provider-free end-to-end fixture.

## Explicitly out of scope

D210 does not add a Casdoor/auth/TLS role checklist, a utility/client/domain
symbol taxonomy, all-symbol model classification, another model call, deeper
SSA or DFS, library-first routes, raw Orientation, source bytes on the model
wire, prompt-only breadth tuning, fixed minimum route padding, fuzzy repair,
semantic retry, choose-first ownership, hidden remainder, legacy reader,
migration or a broad UI rewrite.
