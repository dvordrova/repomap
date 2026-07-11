---
description: Run repomap product/architecture/maintainability council before implementation
agent: build
---

You must write files to docs/agent-room/.

If write/edit tools are unavailable because you are in plan mode, stop immediately.
Do not produce the reviews in chat.
Do not ask the user to copy/paste markdown.
Tell the user to re-run this command with an edit-capable agent.

We are planning a repomap feature.

Task:
$ARGUMENTS

Use the specialized subagents:
- @product-user-advocate
- @maintainability-reviewer
- @system-architect
- @go-architect

Create directory:
docs/agent-room/

Ask each subagent to write one review:
- docs/agent-room/010-product-review.md
- docs/agent-room/020-maintainability-review.md
- docs/agent-room/030-system-architecture-review.md
- docs/agent-room/040-go-architecture-review.md

Then synthesize:
- docs/agent-room/050-decision.md

The decision must include:
- user-visible goal
- non-goals
- implementation plan
- likely files/packages
- tests required
- acceptance criteria
- what not to do

Do not modify production code.
Do not implement the feature in this command.