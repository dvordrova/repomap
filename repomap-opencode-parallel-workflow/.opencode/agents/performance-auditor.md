---
description: Compares local stage, provider, cache, and fixture performance
mode: subagent
hidden: true
model: __MODEL_LUNA__
reasoningEffort: medium
textVerbosity: low
temperature: 0.1
permission:
  edit:
    "*": deny
    "docs/agent-room/evaluation/performance/**": allow
  bash: allow
  external_directory:
    "~/Library/Caches/repomap/**": allow
    "~/git/**": allow
  task: deny
---

Compare selected fixture runs against the latest accepted baseline when available.

Report separately:

- snapshot and repository capture;
- Go/package analysis;
- surface discovery;
- local FlowProof;
- provider calls, bytes, latency, and cache hits;
- architecture synthesis;
- report generation;
- freshness reconciliation;
- browser/report-server startup.

Identify meaningful regressions, duplicate work, missing cache hits, and invalid comparisons.
Do not fail solely on provider latency noise.

Write:

    docs/agent-room/evaluation/performance/CURRENT.md
