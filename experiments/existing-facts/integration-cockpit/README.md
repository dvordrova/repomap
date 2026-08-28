# Existing-facts integration cockpit

This experiment asks one narrow question: can a stable integration UI be
filled usefully from canonical artifacts that already exist, without another
repository analysis run?

It reads a saved run, validates every artifact that is present, builds a
deterministic closed-ref catalog for one exact dependency, and compares three
lenses:

- **D0** is a local, exhaustive grouping of the saved operations.
- **P1** asks DeepSeek for concise operational notes inside the D0 slots.
- **P2** asks DeepSeek for change-oriented notes and a sortable 1–5 priority.

DeepSeek can fill text and priority fields, but it cannot add slots, facts,
paths, dependencies, operations, callers, or core areas. Missing reachability
is rendered exactly as `not represented in available facts`.

`input.json`, `result.json`, and `scorecard.json` retain the closed input,
sanitized responses, and evaluation. The one-shot harness and provider cache
are not retained. No source analyzer, language toolchain, or ordinary
`repomap` run was invoked.
