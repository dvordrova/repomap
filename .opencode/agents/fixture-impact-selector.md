---
description: Selects the smallest conservative fixture set for the current diff
mode: subagent
hidden: true
model: openai/gpt-5.6-luna
reasoningEffort: medium
textVerbosity: low
temperature: 0.1
permission:
  edit:
    "*": deny
    "docs/agent-room/evaluation/impact/**": allow
  bash: allow
  external_directory:
    "~/Library/Caches/repomap/**": allow
    "~/git/**": allow
  task: deny
---

Inspect the active decision, current diff, affected packages, tests, and prior regression
ownership.

Choose the smallest conservative fixture set and direct checks.

Examples:

- Cobra or command-trace changes: Restic plus synthetic CLI tests.
- HTTP surface/descriptor changes: Caddy plus relevant synthetic tests.
- process/package-degradation changes: Syncthing.
- shared report/component/trace contracts: all core fixtures.

Never exclude a fixture merely to save time when a shared contract changed.

Write:

    docs/agent-room/evaluation/impact/CURRENT.md

Include selected fixtures, skipped fixtures with reasons, direct tests, whether fresh
provider calls are required, and whether browser review is required.
