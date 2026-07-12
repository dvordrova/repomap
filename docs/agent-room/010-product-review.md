# Product Review: Operational Flows

**Role**: Product / User Advocate
**Date**: 2026-05-24
**Verdict**: Needs changes before implementation

## User impact

Today, `repomap <repo>` gives the user 4 candidate flows, all request-driven: "gRPC Put
request", "Watch stream", "Lease lifecycle", "Raft write path". The user opens the HTML
report, sees a flat list with confidence badges, clicks a flow, and gets a read-order of
files to inspect. This UX works because every flow tells the same kind of story: "here is
what happens when someone calls this endpoint".

Operational flows tell a different story: "here is what happens when a timer fires, when
the quota fills up, when a leader steps down". The user still needs to understand these
to get a complete mental model of the repo. But mixing them into the same flat list
without distinction will confuse. The user will wonder: "Is this triggered by a request
or does it just... happen?"

If done well, operational flows make `report.html` significantly more useful for repos
like etcd, databases, or any system with background maintenance. If done poorly, they
dilute the report with hand-wavy guesses that erode trust.

## Concrete issues

### 1. No visible distinction between request flows and operational flows

The current candidate flow schema has no `flow_kind` field. The trigger string
("gRPC Put request" vs "Periodic compaction timer") is the only differentiator. The HTML
report renders all flows identically — same cards, same sections. A first-time user
cannot tell at a glance which flows are request-driven and which are operational.

### 2. No minimum evidence threshold for operational flows

Source signals already have weights. `time.NewTicker` gets weight 40 (strong),
`go func()` gets weight 20 (weak), bare `ticker` gets weight 10 (noise). A single weak
signal should not produce a candidate flow. The prompt currently has no rule about what
counts as credible evidence for an operational flow.

### 3. Added flows could crowd out request flows

The default `FlowCount` is 4. If the LLM returns 6 operational flows and 4 request
flows, `selectTopFlows` sorts by confidence and may show only operational flows. The user
who ran `repomap etcd` to understand the gRPC API would see zero request flows and think
the tool is broken.

### 4. Offline mode has no operational flow path

`buildFlowBundlesFromSnapshot` uses only `OrientationCandidates` (main-package
entrypoints). In offline mode, the user gets no operational flow hints, even though the
source signals for background loops, compaction, and quota alarms were already extracted
locally.

### 5. Guessing vs. evidence is not communicated

The existing `confidence` field and `warnings` array can carry this, but the prompt says
nothing about distinguishing "signal-backed operational flows" from "LLM fills in the
gaps assuming this is etcd". A user who sees "backend quota exceeded -> NOSPACE alarm"
with no evidence cited will not know if repomap found this or guessed it.

## Recommendations

### A. Add `flow_type` to candidate flows (must-fix)

Add a `flow_type` field to the candidate flow schema: `"request"` or `"operational"`.
Keep both kinds in the same `candidate_flows` list — do NOT create a separate section.
In the HTML report, render a small pill badge next to each flow name: "Request" or
"Operational". This preserves the flat mental model ("here are all the flows") while
giving instant visual context.

### B. Require evidence in the prompt (must-fix)

Update the orientation prompt to say:

> Operational flows (background timers, compaction, quota enforcement, consensus
> transitions) must cite at least one source_signal with its file path and line.
> Do not invent operational flows that lack signal evidence in the bundle.
> If the evidence is weak, set confidence <= 0.3 and add a warning.

This lets the existing confidence badge do the work. Strong-signal operational
flows (weight >= 30, specific file, specific function pattern) get green. Weak
hand-waving gets amber or red.

### C. Cap operational flows implicitly, not with a hard limit

Do not add a `--max-operational-flows` flag. Instead, tell the LLM to produce
6-8 candidate flows total, preferring the strongest evidence regardless of kind.
`selectTopFlows` already sorts by confidence. The result is a natural mix:
if the repo has 5 strong request signals and 1 strong operational signal, the
top 4 will likely be 3 request + 1 operational. No flag needed.

### D. Add operational path to offline mode (should-fix)

In offline mode, scan source signals for operational categories
(`background_loop`, `admin_maintenance`, `threshold_limit`, `consensus_state`,
`storage_durability`). For each category with >= 2 signals, produce a stub
candidate flow with name like "Background lease expiry loop (offline hint)" and
a note: "Re-run with DEEPSEEK_API_KEY for AI analysis".

### E. Surface signal evidence in the flow detail page (should-fix)

When source signals back a flow, include them in the bundle summary visible on
the HTML flow detail page. A collapsible section "Source signal evidence" showing
the matched snippets with file paths and line numbers.

### F. Keep the trigger field as-is

Do NOT encode the kind into the trigger (e.g., "[OPERATIONAL] compaction"). The
trigger field should remain human-readable prose. `flow_type` makes the kind
programmatically available.

## What a first-time user should experience

```
$ repomap ../etcd
Project: etcd — distributed reliable key-value store
Confidence: 80%

4 candidate flow(s) explained:

━ gRPC Put request              [Request] ━
  Client sends Put via gRPC, server validates quota, proposes to Raft,
  applies to MVCC store, returns response.

━ Raft log write path           [Request] ━
  Proposal enters Raft log, replicates to followers, commits on quorum,
  applies to state machine.

━ Lease expiry background loop  [Operational] ━
  Periodic timer checks lease TTLs, expires dead leases, notifies watchers.

━ Backend quota enforcement     [Operational] ━
  Write exceeds storage quota, NOSPACE alarm triggers, cluster blocks writes
  until compaction frees space.
```

## Must-fix before implementation

1. Add `flow_type: "request" | "operational"` to `CandidateFlow` and the
   orientation prompt output schema.
2. Update the orientation prompt to require source_signal evidence for
   operational flows with a minimum confidence threshold for weak evidence.
3. Render the `flow_type` badge in the HTML report (pill next to flow name).
