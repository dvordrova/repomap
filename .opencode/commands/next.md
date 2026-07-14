---
description: Propose and, after one explicit yes, activate the next product decision
agent: workflow-manager
---

Optional direction from the repository owner:

$ARGUMENTS

Manage the next-decision flow with minimal owner interaction.

## Existing proposal

When `docs/agent-room/NEXT.md` already points to a Proposed decision:

1. read it;
2. show a compact summary:
   - problem;
   - proposed outcome;
   - why now;
   - non-goals;
   - acceptance journey;
   - expected size/risk;
3. ask exactly:

       Activate this decision as CURRENT? yes / no

Do not create another proposal.

When the owner replies with an unambiguous yes/да in this conversation, activate it using
the workflow-manager contract, create the governance commit, and stop with:

    Next command: /go

## No existing proposal

When no pending NEXT proposal exists:

1. require the current decision to have fresh acceptance PASS;
2. invoke `@decision-planner` in a fresh context, passing the optional owner direction;
3. read the created proposal and NEXT.md;
4. show the compact summary;
5. ask exactly:

       Activate this decision as CURRENT? yes / no

Do not activate it until the owner explicitly replies.

`CURRENT.md` is always written by the workflow. The owner never edits it manually.
