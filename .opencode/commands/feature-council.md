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

Treat existing numbered reviews and decisions as historical records. Do not overwrite
them. Require $ARGUMENTS to name an unused proposed decision filename and corresponding
unused review filenames before writing. If those filenames are absent, stop and ask for
them.

Ask each subagent to write its review to the explicitly supplied review filename, then
synthesize the explicitly supplied numbered decision file with status `Proposed`.

Do not select a decision by numeric order. Do not update `docs/agent-room/CURRENT.md`
until the repository owner explicitly approves the proposed scope.

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
