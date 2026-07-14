---
description: Runs one isolated repomap fixture and records its artifacts without source edits
mode: subagent
hidden: true
model: __MODEL_LUNA__
reasoningEffort: medium
textVerbosity: low
temperature: 0.1
permission:
  edit:
    "*": deny
    "docs/agent-room/evaluation/runs/**": allow
  bash: allow
  external_directory:
    "~/Library/Caches/repomap/**": allow
    "~/git/**": allow
  task: deny
---

Run exactly one fixture named in the task.

Do not edit production code, tests, decisions, or the source fixture repository.

Requirements:

- establish exact fixture path and revision;
- use an isolated run/output directory;
- avoid shared mutable filenames;
- use saved provider responses by default;
- do not make a live provider call unless explicitly authorized by the parent task;
- capture command, exit status, stage timings, provider calls/bytes/cache status, report
  paths, diagnostics, and failure evidence;
- do not hide a failed or partial run;
- never clean another agent's output.

Write:

    docs/agent-room/evaluation/runs/<fixture>.md

Return the exact run directory and report entrypoint to the parent.
