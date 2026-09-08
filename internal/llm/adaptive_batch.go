package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AdaptiveBatchAccounting records live transport work from complete-plan
// rounds that were discarded before a later adaptive split succeeded. Final
// accepted outcomes are deliberately excluded so callers can keep using their
// ordinary outcome accounting without double counting semantic authority.
type AdaptiveBatchAccounting struct {
	DiscardedLiveCalls int
	DiscardedAttempts  int
	DiscardedLatency   time.Duration
}

// ExecuteAdaptiveJSONBatch executes a complete caller-ordered plan. When the
// real provider context, response or output-token envelope rejects one non-atomic item,
// split deterministically replaces that item and the complete plan is retried.
// Accepted sibling requests may be served from their identity-bound cache;
// no partial outcomes are returned as semantic authority.
func ExecuteAdaptiveJSONBatch[Item any, Value any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	items []Item,
	build func([]Item) ([]Call[Value], error),
	split func(Item) (Item, Item, bool),
) ([]Item, []Outcome[Value], error) {
	plan, outcomes, _, err := ExecuteAdaptiveJSONBatchWithAccounting(
		ctx, executor, provider, items, build, split,
	)
	return plan, outcomes, err
}

// ExecuteAdaptiveJSONBatchWithAccounting is ExecuteAdaptiveJSONBatch plus an
// operational ledger for discarded rounds. The ledger never turns a failed or
// superseded outcome into semantic authority.
func ExecuteAdaptiveJSONBatchWithAccounting[Item any, Value any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	items []Item,
	build func([]Item) ([]Call[Value], error),
	split func(Item) (Item, Item, bool),
) ([]Item, []Outcome[Value], AdaptiveBatchAccounting, error) {
	plan := append([]Item(nil), items...)
	var accounting AdaptiveBatchAccounting
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, accounting, err
		}
		calls, err := build(plan)
		if err != nil {
			return nil, nil, accounting, err
		}
		if len(calls) != len(plan) {
			return nil, nil, accounting, fmt.Errorf(
				"llm: adaptive batch builder returned %d calls for %d items",
				len(calls), len(plan),
			)
		}
		replanned := false
		for index, call := range calls {
			found, err := loadAdaptiveSplit(executor, provider, call)
			if err != nil {
				return nil, nil, accounting, err
			}
			if !found {
				continue
			}
			left, right, ok := split(plan[index])
			if ok {
				plan = replaceAdaptiveItem(plan, index, left, right)
				replanned = true
				break // Rebuild all calls from the current owner's complete plan.
			}
		}
		if replanned {
			continue
		}
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(plan))
		}
		outcomes, err := ExecuteJSONBatch(ctx, executor, provider, calls)
		if err == nil {
			return plan, outcomes, accounting, nil
		}
		addDiscardedRound(&accounting, outcomes)
		var itemErr *BatchItemError
		var resourceErr *ResourceLimitError
		if !errors.As(err, &itemErr) || itemErr.Index < 0 || itemErr.Index >= len(plan) ||
			!errors.As(err, &resourceErr) ||
			!adaptiveSplitKind(resourceErr.Kind) {
			return nil, nil, accounting, err
		}
		// Only a transport resource refusal authorizes a persistent split.
		// A semantic validator returning a typed resource error is not one.
		providerErr, providerFailure := itemErr.Err.(*ProviderError)
		if !providerFailure || providerErr.Operation != "complete" {
			return nil, nil, accounting, err
		}
		left, right, ok := split(plan[itemErr.Index])
		if !ok {
			return nil, nil, accounting, err
		}
		if err := saveAdaptiveSplit(executor, provider, outcomes[itemErr.Index].Request,
			calls[itemErr.Index].Limits, resourceErr.Kind); err != nil {
			return nil, nil, accounting, err
		}
		plan = replaceAdaptiveItem(plan, itemErr.Index, left, right)
	}
}

func replaceAdaptiveItem[Item any](plan []Item, index int, left, right Item) []Item {
	next := make([]Item, 0, len(plan)+1)
	next = append(next, plan[:index]...)
	next = append(next, left, right)
	return append(next, plan[index+1:]...)
}

func addDiscardedRound[Value any](accounting *AdaptiveBatchAccounting, outcomes []Outcome[Value]) {
	if accounting == nil {
		return
	}
	for _, outcome := range outcomes {
		// RequestBytes proves Prepare completed and executeLive was entered. A
		// canceled or queued zero slot is not a live provider call, while a
		// provider resource failure may legitimately report zero Attempts.
		if outcome.Cached || outcome.RequestBytes <= 0 {
			continue
		}
		accounting.DiscardedLiveCalls++
		accounting.DiscardedAttempts += outcome.Metrics.Attempts
		accounting.DiscardedLatency += outcome.Metrics.Latency
	}
}
