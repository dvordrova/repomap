# Decision 196: Atlas-first navigation and truthful workspace

## Status

Approved by the repository owner in the current supervisory session. This
decision supersedes only Decision 195's raw-complete-facts Orientation lane.
The reviewed Decision 195 source coverage, UI, and semantic-output work remain
valid prerequisites and are integrated without changing their accepted scope.

## Problem

Orientation currently mixes three responsibilities in one prompt-shaped
bundle: raw local observations (`source_signals` and related facts), selection
of what might be worth investigating, and early semantic adjudication. Making
that bundle "complete" produced a multi-megabyte pre-security etcd payload
dominated by repetitive local signal evidence. It is not a useful model
representation, and it is unsafe to solve by raising a byte default, silently
dropping facts, or treating byte counts as provider-token counts.

At the same time Repository Atlas v1 exists only as a provider-free core. The
normal run and the product workspace cannot yet show its exact Units, evidence,
authority, or relations. The UI must not turn static or partial facts into
invented runtime mechanisms.

## Decision

Build the product in this order:

1. complete the reviewed Decision 195 source-coverage/UI and terminal-output
   checkpoints, including their provider-free regressions;
2. persist the smallest useful exact Atlas vertical in an ordinary successful
   local run;
3. compile bounded task-shaped Navigator projections from that Atlas and local
   derived motifs; and
4. expose those facts in the existing workspace with an explicit epistemic
   label, exact evidence drawer, and honest empty/partial states.

One owner-approved live-provider A/B may occur only after the provider-free
projection and UI matrix are complete. It is not a development dependency.

### Atlas is the local source of truth

The canonical `repositoryatlas` v1 model remains language-neutral. The first
ordinary-run vertical persists only what the existing Go adapter proves:

- repository/module/app/package Units;
- available, exact application Surface facts;
- exact `process_entry` Operations and their proven Surface-to-Operation
  relation; and
- Observation, Evidence, Relation, Phase and Authority values required to
  explain each visible fact.

Boundary, Resource and Contract are intentionally absent until a language
adapter proves them. File paths and symbols remain Evidence locators, never
Atlas entities. Every relation/ref remains unit-scoped and validated. There is
no fake reachability, inferred runtime execution, migration reader, second
canonical motif store, or UI-only join.

The persisted Atlas must be manifest-authorized, deterministic, redacted by the
existing artifact authority, and available to the report without a provider
call. Its exact artifact name/version and report projection are selected by the
first implementation plan and reviewed before code changes.

### Navigator receives a task-shaped projection, not raw observations

Raw source signals, file candidates, local edges, and evidence occurrences stay
local diagnostic material. They may form Atlas Observations/Evidence and
derived opportunity motifs, but are not copied as an all-hits model payload.

The backend compiles each model request from a bounded semantic task slice:

1. a concrete question and request-local scope;
2. locally selected seed Units/Surfaces/Operations;
3. a compact map summary;
4. bounded trails, intersections and explicitly named gaps where proven;
5. representative exact evidence; and
6. a closed catalog of backend-owned local operations.

The model receives short **request-local** references and a concise meaning for
each reference. It never creates canonical IDs, performs graph traversal,
chooses an arbitrary budget/depth, receives full source, raw `file_tree`, raw
`internal_edges`, or a Neo4j-like graph dump. The backend validates every
returned reference against that request-local catalog.

`source_signal` remains valid local Observation data. Repetitions may be
normalized into a derived motif/projection only with exact contributing evidence
and deterministic provenance. A motif is not a second canonical store and is
not automatically a Flow or a mechanism.

The 1 MiB Orientation setting remains an honest serialized-request resource
limit for a compiled Navigator projection, not a claim about the provider's
token context. If an approved projection cannot fit, the current terminal
resource contract remains in force; this decision adds no silent trimming,
fallback, request splitting, or automatic continuation. A future change to
publish a local Atlas when an optional Navigator call fails needs its own
explicit decision.

### Product workspace

The workspace is one anatomy, not a visual assertion that every card is a
runtime path:

- entry surfaces at the top;
- exact proven components/operations in the centre;
- integrations/resources/contracts only where Atlas facts prove them;
- Study directions, when published, as reading routes through the same exact
  IDs; and
- an evidence/source drawer for every clickable object.

Every shown object/edge carries a clear Authority state: observed, resolved,
inferred, partial, conflicted, or unknown as applicable. Inferred and partial
hypotheses are allowed product data, but never look equivalent to resolved
facts. No invented coverage percentage, timing, anchor count, probe, search
result, mechanism, action, or arrow is permitted. Empty and failed states are
first-class.

The reviewed Decision 195 source projection is the immediate enabler for a
clickable Overview. It remains drawer-first and may use current
SurfaceCatalog/Architecture IDs until the persisted Atlas vertical supplies the
same facts. The Architecture view must share IDs with Overview, not present a
competing truth.

### Study Brief/Shape references are typed at the producer boundary

The saved Casdoor run showed that Brief/Shape accepts an opaque canonical string
in `shape_area_ids` and discovers a document-vs-area wrong-kind error only
after the candidate and independent review calls have succeeded. Replace that
stage's raw canonical IDs with a request-local typed catalog: support references
may name permitted Anchor/Document/Area values, and Shape references may name
Areas only. A catalog reference binds the exact bundle, order, prompt, and
validator inputs.

Resolve and validate these refs immediately, before marking Brief/Shape
accepted, saving a canonical artifact, or making the next candidate call.
Unknown, raw-canonical, prefix, duplicate, wrong-kind, and cross-request refs
receive closed field-position diagnostics; there is no retry, fuzzy repair,
substitution, or old response reader.

An invalid Shape item does not erase valid sibling Areas. With zero valid Areas,
Shape is an explicit empty editorial selection and does not gate independently
source-reviewed Directions; it never uses a default-area fallback. Brief's
required supported statements retain their current gate: zero valid required
support fails before later Study calls. Canonical IDs are restored only after
successful immediate resolution.

### Work and review boundaries

The implementation is staged as separately reviewable, provider-free
checkpoints:

1. repair and integrate the reviewed Decision 195 UI/source coverage and
   semantic resource-limit changes;
2. persist + validate the minimum exact Go Atlas vertical in ordinary run
   artifacts, with no provider/UI semantic expansion;
3. add the pure task-shaped Navigator projection compiler with raw-vs-derived
   comparison fixtures;
4. repair Study Brief/Shape request-local typed references and immediate
   validation; and
5. connect the first Atlas-backed workspace shelves and Authority labels.

Each checkpoint names exact files, artifact size/count deltas, and failure
behavior before editing. It must pass focused provider-free tests and the full
repository checks before commit. No live LLM call, PATH installation, legacy
reader, migration, provider/cache protocol change, or broad framework rewrite
is implicit.

## Proof

- deterministic Atlas JSON/order, scoped typed-ref/evidence validation, and
  saved artifact/manifest authorization;
- Casdoor and etcd provider-free projections showing raw observations stay
  local while representative evidence and request-local refs remain valid;
- size/count attribution for all projection sections, including explicit
  terminal over-budget behavior;
- exact source drawer, Authority-labelled object, empty/partial/conflict,
  and Study-route UI matrix using saved fixtures; and
- one owner-approved live A/B at the end, comparing the same repository state
  and exact input/output/call journal deltas.
