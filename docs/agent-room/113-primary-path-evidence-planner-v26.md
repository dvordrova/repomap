# Decision: Primary Path Evidence Planner v2.6

Status: Approved by the repository owner in the current implementation
session.

## Product outcome

Continue the frozen Litestream experiment at revision
`d26cb54ec43b5937a0c8b3bd875696c1375d8cb3`. Turn each already proposed
central candidate into its own bounded evidence-planning problem, and require
an input or trigger, candidate-specific core work, and an observable effect or
typed boundary before model synthesis. A primary mechanism is published only
when those locally validated facts support a meaningful multi-phase path.

## Approved implementation scope

1. Diagnose the saved v2.5 candidate artifacts before making another
   opportunity-planning call. Preserve exact questions, candidate IDs, model
   prose, and the already accepted secondary and extension mechanisms.
2. Derive intent identity from repository namespace, normalized question,
   candidate kind, bounded scope, and sorted candidate-specific anchors.
   Mechanism v1 identity is unchanged.
3. Derive a candidate-specific answer contract with three mandatory key roles:
   input/trigger, core work, and observable effect. Logging, errors, branches,
   configuration, cleanup, and local handoffs remain supporting evidence only.
4. Build one deterministic `ProbePlan` per candidate from exact local anchors,
   locally enumerated named frontiers, desired boundary kinds, explicit budgets,
   and stop conditions. An unresolved candidate anchor yields
   `requires_better_anchor` rather than borrowing another candidate's root.
5. Expand at most two exact repository-local named frontiers, to depth two,
   with at most two additional files, five additional functions, 64 KiB of
   retained source, and three seconds of local work. Stop at the first validated
   file, database, network, backend/interface, persistence, or public-output
   boundary. No dynamic implementation resolution is implied.
6. Project inspected syntax into ordinary deterministic facts with opaque IDs,
   exact source, content hashes, narrow scope, and candidate association.
7. Skip synthesis unless the candidate has input, core-work, and effect facts
   across at least two exact enclosing symbols and is not an all-logging or
   all-error set. Store `insufficient_primary_evidence` on failure.
8. Use at most one synthesis call for each eligible candidate and no semantic
   retry. Existing validators plus stricter local primary-path relevance own
   acceptance and onboarding role.
9. Reproduce the saved restore preflight rejection. Fix it only if it is a
   generic false-positive or serialization defect, with a general regression;
   otherwise preserve the rejection.
10. Replay the fixed Litestream checkout and save the requested diagnostics,
    plans, expansions, boundaries, accepted/rejected results, report, and
    conditional screenshots under `tmp/primary-path-planner-v26/`.

## Explicit non-goals

- No UI, Search, architecture-canvas, provider, model, thesis-copy, or
  Mechanism v1 truth-contract changes.
- No runtime-surface discovery, global SSA, call graph, pointer analysis,
  repository-wide traversal, or implementation resolution.
- No Litestream-specific production terms, paths, symbols, questions,
  candidates, or validator exceptions.
- No manual post-result frontier selection, semantic retry, weakened validator,
  or promotion of a secondary mechanism to satisfy the demo.

## Acceptance

Distinct candidates have distinct question-and-anchor identities. Every primary
contract and eligibility decision requires input, core work, and observable
effect. At most two bounded named-frontier expansions are attempted per
candidate. The frozen Litestream run either produces a source-backed primary
path scoring at least 3/4 or records `bounded automatic evidence planning
insufficient` before synthesis. Caddy and chi semantic identities/hashes and
all existing Litestream secondary/extension identities remain stable.
