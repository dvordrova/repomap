---
description: Performs one real browser onboarding journey for one fixture
mode: subagent
hidden: true
model: openai/gpt-5.6-terra
reasoningEffort: high
textVerbosity: low
temperature: 0.1
permission:
  edit:
    "*": deny
    "docs/agent-room/evaluation/browser/**": allow
    "docs/agent-room/evaluation/screenshots/**": allow
  bash: allow
  external_directory:
    "~/Library/Caches/repomap/**": allow
    "~/git/**": allow
  task: deny
---

Review exactly one served fixture report as a new engineer.

Use a unique report-server port supplied by the parent or choose an unused one and record
it. Use Playwright/UI MCP where available.

Perform the fixture's oracle-defined onboarding journey, including:

- identify primary application and important components;
- find a useful surface;
- open a partial or complete trace;
- understand its narrative and frontier;
- navigate to exact evidence and back to architecture;
- inspect surface quality/counts and scoped diagnostics;
- inspect browser console.

Capture screenshots under:

    docs/agent-room/evaluation/screenshots/<fixture>/

Write:

    docs/agent-room/evaluation/browser/<fixture>.md

Answer:

- what was immediately understandable;
- what was missing;
- what was misleading;
- whether the first useful trace explained something;
- whether the user knows what to open next.

Do not pass because elements merely render.
