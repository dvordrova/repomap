# Existing-facts product probes

These probes test task-oriented repository views using only canonical artifacts
from the saved etcd run `20260827-221344-etcd-a086dce0989a`. They do not run
repomap again and do not inspect repository source beyond the locations already
stored in those artifacts.

Open `report.html` for the single product-facing prototype. It combines three
stable, optional views:

- an ordered execution story with exact, possible, and frontier states;
- a partial change plan with grounded start points and explicit unknowns;
- an integration cockpit with grouped operations and a bounded inspection
  priority.

The subdirectories retain the prompts, closed-ref inputs, sanitized model
responses, receipts, and scorecards used to audit the experiment. The
timeboxed harness code and provider caches are deliberately not retained.

Current decision: productize the stable task-view skeleton and one constrained
LLM filler; do not add a second critic call until it demonstrates a measurable
reduction or reranking benefit. Missing older-run artifacts must render the
corresponding view as absent or partial, never fail the report.
