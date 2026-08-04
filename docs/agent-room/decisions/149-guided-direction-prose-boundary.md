# Decision 149: Guided direction prose boundary

## Status

Approved by the repository owner after a real Chatto run produced valid Guided
Tour JSON twice but rejected both responses on heuristic prose classification.

## Observed failure

The selected `suggested_direction` carried exact local beat, gap, component,
and evidence IDs. Its editorial explanation was nevertheless treated as a
runtime proof claim by a regular-expression classifier, so the whole Guided
Tour was discarded even though this candidate kind is explicitly a hypothesis
for investigation.

The exact candidate name `Administrative backup/restore` was also rejected as
path-like prose solely because the repository-provided title contains a natural
slash.

## Corrective contract

- A `suggested_direction` remains explicitly hypothetical and editorial.
  Its prose is not publication authority for runtime behavior.
- Exact candidate, beat, gap, component, evidence, and location references
  remain locally derived or fail closed.
- Repository-reference detection remains strict for every model-authored prose
  field. The exact locally supplied candidate name may be reused as the title,
  including a natural slash; arbitrary slash/path prose remains rejected.
- The behavioral clause classifier remains diagnostic only. It must not decide
  whether a structurally valid editorial direction is published.
- The prompt describes this boundary directly and does not add a new parser,
  analyzer, provider request shape, report format, or presentation layer.

## Acceptance

- focused tests cover hypothetical prose, exact natural-slash title replay,
  and rejection of a tampered path-like title;
- the saved Chatto Guided Tour bundle publishes a story using only supplied
  IDs and locations;
- full repository checks and nearby etcd validation pass.
