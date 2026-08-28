# Existing-facts execution story

Fast hypothesis experiment: project a useful, honest execution story from an existing repomap run, then compare a deterministic baseline with two constrained DeepSeek edits. It does not scan repository source and does not launch repomap.

The stable slots are start, ordered edges, exact/possible certainty, external boundary, known gaps, and two code jumps. DeepSeek rewrote only the prose inside those slots. The experiment rejected unknown refs, reordered or invented bridges, promoted certainty, changed endpoints, missing frontiers, and host-absolute paths.

`artifacts/` retains the closed input catalog, sanitized DeepSeek responses, and scorecard. The reusable product decision is represented in the single parent `report.html`; the one-shot harness and provider cache are not retained.

The follow-up `Why this matters` probe adds a deliberately smaller copy task:
explain which developer question this particular route can help investigate,
keep its most important uncertainty visible, and leave code-anchor selection to
the local application. `artifacts/why-this-matters-input.json` is the complete
provider input: six fact cards and no repository identity, revision, path, or
canonical ID;
`prompts/why-system.md` and `prompts/why-this-matters.md` define the bounded JSON
request. `artifacts/why-this-matters-response.json` now records one live,
structurally accepted DeepSeek result, and its receipt records the exact hashes,
token counts, latency, and editorial review. The parent report renders only the
accepted `value` and `limit` fields; route facts and navigation remain local.

Closed refs establish where model wording came from; they do not by themselves
prove that free prose is semantically faithful. A production cube would still
need local schema/ref validation plus a stricter factual-language contract (or
closed selection with locally rendered copy).

The live result passed JSON shape, word bounds, and closed-ref membership. The
second `use_when` question was not promoted into the UI because its wording
connects the external callsite to startup while citing only the boundary card;
the receipt keeps that limitation explicit.
