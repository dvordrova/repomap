---
description: Implement an approved repomap feature decision
agent: build
---

You are implementing an approved repomap feature decision.

Decision file or task:
$ARGUMENTS

Default decision file:
docs/agent-room/CURRENT.md

If $ARGUMENTS is empty, use:
docs/agent-room/CURRENT.md

`CURRENT.md` is a pointer. Read it, then read the referenced numbered decision.
Do not select the numerically latest decision automatically. If $ARGUMENTS names a
different decision, use it only when the repository owner explicitly approves changing
the active scope and update `CURRENT.md` as part of that governance change.

## Core project rules

The main product UX is sacred:

  repomap <repo>

The default command should produce a useful visual report:

  .repomap-runs/latest/report.html

Do not expose internal pipeline details as the primary user workflow.

DeepSeek is not the scanner.
Local deterministic extraction gathers facts and focused bundles first.
DeepSeek only interprets bounded facts.

## Hard non-goals unless explicitly present in the decision

Do NOT add:
- caching
- session model
- VS Code extension
- AST parsing
- LSP/gopls
- embeddings
- diagrams/UI frameworks
- third-party dependencies
- a new forest of flags
- user-facing commands for every internal pipeline stage

## Before editing

1. Read AGENTS.md.
2. Read docs/CORE_IDEA.md if it exists.
3. Read docs/DEEPSEEK_API_NOTES.md if it exists.
4. Read the decision file.
5. Summarize the implementation plan in 5-10 bullets.
6. Identify likely packages/files to edit.
7. Identify tests that must be added or updated.

Do not start editing until you have a concrete plan.

## Implementation rules

Implement only the accepted decision.

Verify that the accepted decision is the one referenced by `docs/agent-room/CURRENT.md`.

Do not follow conflicting ideas from older review files unless they are included in the decision file.

Prefer small, boring, testable changes.

Keep package boundaries clean:
- CLI parsing should stay near cmd/repomap.
- Pipeline orchestration should not be mixed with HTML rendering details.
