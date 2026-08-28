# Existing-facts execution story

Fast hypothesis experiment: project a useful, honest execution story from an existing repomap run, then compare a deterministic baseline with two constrained DeepSeek edits. It does not scan repository source and does not launch repomap.

The stable slots are start, ordered edges, exact/possible certainty, external boundary, known gaps, and two code jumps. DeepSeek rewrote only the prose inside those slots. The experiment rejected unknown refs, reordered or invented bridges, promoted certainty, changed endpoints, missing frontiers, and host-absolute paths.

`artifacts/` retains the closed input catalog, sanitized DeepSeek responses, and scorecard. The reusable product decision is represented in the single parent `report.html`; the one-shot harness and provider cache are not retained.

The follow-up `Why this matters` probe adds a deliberately smaller copy task:
explain which developer question this particular route can help investigate,
name the useful code anchors, and keep its most important uncertainty visible.
`artifacts/why-this-matters-input.json` is the complete six-card fact catalog;
`prompts/why-system.md` and `prompts/why-this-matters.md` define the bounded JSON
request. The parent report currently renders a locally audited preview of this
slot. No new DeepSeek response is recorded, so the preview must not be cited as
provider output.

Closed refs establish where model wording came from; they do not by themselves
prove that free prose is semantically faithful. A production cube would still
need local schema/ref validation plus a stricter factual-language contract (or
closed selection with locally rendered copy).
