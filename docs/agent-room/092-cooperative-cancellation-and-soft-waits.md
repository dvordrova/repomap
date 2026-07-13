# Decision: Cooperative cancellation and soft provider waits

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Goal

Prefer soft request-size targets and patient long-running analysis over tight
premature cutoffs. Keep the CLI responsive to the user's Ctrl-C throughout
provider waits and expensive local analysis, and log periodic progress so a
user can decide when a run has taken too long.

## Approved scope

1. Increase the default provider HTTP timeout substantially while preserving
   the existing environment override.
2. Emit periodic, content-free wait messages for orientation, targeted
   research, and architecture synthesis, explicitly reminding the user that
   Ctrl-C cancels the run.
3. Propagate the normal CLI signal context into expensive local surface
   discovery and targeted local planning.
4. Treat context cancellation as cancellation of the run rather than a normal
   analyzer warning followed by additional work.
5. Preserve the call-count bounds, technical request safety ceilings, provider
   evidence validation, and all local authority boundaries from decisions 090
   and 091.

## Non-goals

Do not add background provider calls, detach work after cancellation, ignore
provider transport timeouts, introduce an unbounded loop, or log prompts,
responses, source, credentials, or authorization material in wait messages.
