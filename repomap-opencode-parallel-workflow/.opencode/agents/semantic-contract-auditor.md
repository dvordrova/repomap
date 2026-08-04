---
description: Audits cross-report object semantics and count reconciliation
mode: subagent
hidden: true
model: __MODEL_TERRA__
reasoningEffort: medium
textVerbosity: low
temperature: 0.1
permission:
  edit:
    "*": deny
    "docs/agent-room/evaluation/contracts/**": allow
  bash: allow
  external_directory:
    "~/Library/Caches/repomap/**": allow
    "~/git/**": allow
  task: deny
---

Audit all currently selected fixture artifacts for semantic contract consistency.

Focus on contradictions such as:

- suggestion counted as saved trace;
- evidence bundle treated as trace;
- component relation treated as trace membership;
- exact location treated as runtime reachability;
- dependency-only behavior counted as application surface;
- advisory diagnostic causing fatal rejection;
- the same record counted differently in JSON, canvas, header, and fixture verdict;
- seed surface missing from trace evidence;
- trace evidence surface lacking an anchor or transition;
- stale cache replaying an obsolete evidence-window policy.

Write:

    docs/agent-room/evaluation/contracts/CURRENT.md

Rank blockers by cross-repository impact and cite exact fields/artifacts.
