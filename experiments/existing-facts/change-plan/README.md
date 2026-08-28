# Existing-facts change-plan hypothesis

This timeboxed experiment asks one realistic etcd task: change KV Put quota-exhaustion handling, find the first edit anchors, and state what must be verified.

It reads only six canonical JSON artifacts from one completed run. It does not read repository source, execute Go/C++ targets, or run `repomap`.

Treatments:

- D0: deterministic retrieval into a fixed sparse plan skeleton.
- P1: evidence-first DeepSeek plan over closed short refs.
- P2: DeepSeek planner followed by a critic that can only remove/reorder steps or increase uncertainty.

All accepted plans have 3–6 steps, grounded closed refs, a source jump in the first two actions, and explicit unknowns. A recorded frontier remains partial evidence. Missing tests and commands remain unknown.

`artifacts/` retains the compact catalog, sanitized requests/responses, receipts,
and scorecard. The one-shot harness and provider cache are not retained. Its
main result is negative: the second critic call did not remove or reorder a
step and reduced paraphrase stability, so it should not enter the product.
