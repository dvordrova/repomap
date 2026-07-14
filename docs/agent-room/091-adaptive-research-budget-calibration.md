# Decision: Adaptive research budget calibration

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Clarification of decision 090

The approximate 64–96 KiB stage sizes and 256–320 KiB normal-run total are
measurement targets, not semantic hard cutoffs. Do not discard useful exact
architecture evidence merely to satisfy those early estimates.

Keep bounded technical safety rails:

- at most four semantic provider calls in a normal run;
- at most two targeted research rounds;
- at most 1 MiB in any one provider request;
- existing stage-specific secondary safety ceilings for summaries, source
  windows, evidence items, and exact architecture candidates;
- no full repository source dump and no autonomous loop.

The aggregate request budget may be as large as the four bounded calls permit.
Normal runs should still report and target materially smaller request totals.
Tighten a byte limit only after fixture and live measurements demonstrate that
the tighter boundary preserves useful evidence and product quality.

## Explicitly rejected policy

Do not introduce a small fixed architecture-candidate count by repeatedly
lowering a magic number until one fixture happens to fit a byte estimate.
Architecture input remains behavior-first and exact; package support is still
secondary, but omission must result from a stated relevance policy or a real
technical safety boundary.
