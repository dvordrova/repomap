# Existing-facts execution story

Fast hypothesis experiment: project a useful, honest execution story from an existing repomap run, then compare a deterministic baseline with two constrained DeepSeek edits. It does not scan repository source and does not launch repomap.

The stable slots are start, ordered edges, exact/possible certainty, external boundary, known gaps, and two code jumps. DeepSeek rewrote only the prose inside those slots. The experiment rejected unknown refs, reordered or invented bridges, promoted certainty, changed endpoints, missing frontiers, and host-absolute paths.

`artifacts/` retains the closed input catalog, sanitized DeepSeek responses, and scorecard. The reusable product decision is represented in the single parent `report.html`; the one-shot harness and provider cache are not retained.
