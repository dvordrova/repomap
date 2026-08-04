# Decision: morning review cockpit and saved-artifact frontier

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can the saved Caddy and chi experiments be made understandable from one
dev-only review surface, while keeping deterministic facts, untrusted model
proposals, locally accepted canonical knowledge, and research hypotheses
visibly distinct?

## Approved implementation

1. Generate an untracked Morning Review Cockpit under `tmp/repomap-review`
   using only already saved Caddy and chi artifacts. It may use simple HTML,
   Markdown, JSON, collapsible sections, and links; it is not a production
   renderer or server.
2. Show the evidence-to-canonical pipeline, experiment funnel, facts, model
   proposals, claim support, coverage, unknowns, diagnostics, canonical
   Mechanisms, replay state, report/Search links, and raw-artifact links with a
   permanent fact/proposal/canonical/rejected/unknown grammar.
3. Reflect the completed local-verdict work from decision 105. Re-evaluate the
   fixed chi response without a model, publish it only if accepted, and show
   both its proposed and locally derived verdicts.
4. If both Caddy and chi remain canonical, add only deterministic
   presentation targets derived from existing statement evidence. Prefer an
   inspector-local trace strip or exact file/symbol lens; do not add
   architecture components, semantic relations, or hashed Mechanism fields.
5. Add a bounded Explore Possibilities projection from saved opportunities,
   selected candidates, leaf results, rejected proposals, capability gaps, and
   canonical Mechanisms. Hypotheses remain questions, never Start Here or
   repository truth, and are capped at 20 visible items.
6. Update one review artifact after every gated goal and finish with
   `tmp/repomap-review/MORNING.md`, an exact serving command, and a five-minute
   walkthrough.

## Required saved inputs

- accepted Caddy directory-listing Mechanism and its no-model replay/report;
- fixed chi request-dispatch fixture, projection, supplement, response,
  verdict-replay diagnostic, canonical Mechanism, and report;
- saved broad-run opportunities, selected leaves, rejections, capability
  contracts, and unavailable/frontier state.

## Focused checks

- review generation makes zero model, probe, analyzer, package-loading, SSA,
  call-graph, or runtime-surface calls;
- every displayed canonical claim originates in a saved canonical Mechanism;
- every displayed model claim is labelled untrusted proposal;
- Caddy and chi Mechanism logical/content hashes stay unchanged;
- at least three steps per Mechanism expose distinct existing-evidence targets
  without changing semantic identity;
- frontier cards include exact saved grounds, missing capabilities, and a
  question-shaped next probe, and never enter Start Here/Search truth;
- the review is reachable over an existing or standard-library static server,
  not `file://`.

## Hard exclusions

- model calls, repository-wide analysis, package loading, SSA, call graph,
  runtime-surface discovery, cache/provider work, embeddings, MCP, runtime Q&A,
  lexical ranking changes, or a universal knowledge-object framework;
- editing saved model prose or claims, inventing facts/relations/evidence, or
  turning rejected/speculative material into canonical knowledge;
- a new production server or renderer.

## Gate

If the fixed chi response fails claim-level validation, record the exact
blocker, skip chi publication/focus-dependent work, and continue only the
saved-artifact review and frontier. Verdict mismatch alone is not a blocker.
