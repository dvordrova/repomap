package llm

import (
	"context"
	"fmt"
)

// AdaptiveEachResult is one leaf of ExecuteAdaptiveJSONEachResults: the
// complete item it covers, that item's outcome and, when the item could
// neither be completed nor split further, its terminal error. Whether such a
// leaf is acceptable is the owning stage's decision, as with EachResult.
type AdaptiveEachResult[Item any, Value any] struct {
	Item    Item
	Outcome Outcome[Value]
	Err     error
}

// ExecuteAdaptiveJSONEach completes independent items without canceling or
// repeating successful siblings when an item must split. Build owns one item,
// not the changing plan: accepted calls and outcomes stay unchanged in memory,
// including when the persistent cache is disabled. Each round uses the shared
// pool, and all splittable failures from that round become its next children.
// A terminal failure still prevents returning a partial successful cover.
func ExecuteAdaptiveJSONEach[Item any, Value any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	items []Item,
	build func(Item) (Call[Value], error),
	split func(Item) (Item, Item, bool),
) ([]Item, []Outcome[Value], error) {
	results, err := executeAdaptiveJSONEach(ctx, executor, provider, items, build, split, false)
	if err != nil {
		return nil, nil, err
	}
	final := make([]Item, len(results))
	outcomes := make([]Outcome[Value], len(results))
	for i, result := range results {
		final[i], outcomes[i] = result.Item, result.Outcome
	}
	return final, outcomes, nil
}

// ExecuteAdaptiveJSONEachResults runs the same rounds, but an item that can
// neither be completed nor split further stays as its own terminal leaf beside
// the completed cover while the remaining items continue. Cancellation, build,
// split-memo and executor errors still fail the whole call.
func ExecuteAdaptiveJSONEachResults[Item any, Value any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	items []Item,
	build func(Item) (Call[Value], error),
	split func(Item) (Item, Item, bool),
) ([]AdaptiveEachResult[Item, Value], error) {
	return executeAdaptiveJSONEach(ctx, executor, provider, items, build, split, true)
}

func executeAdaptiveJSONEach[Item any, Value any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	items []Item,
	build func(Item) (Call[Value], error),
	split func(Item) (Item, Item, bool),
	keepTerminal bool,
) ([]AdaptiveEachResult[Item, Value], error) {
	type node struct {
		item    Item
		call    Call[Value]
		outcome Outcome[Value]
		err     error
		done    bool
	}
	makeNode := func(item Item) (node, error) {
		call, err := build(item)
		return node{item: item, call: call}, err
	}
	children := func(item Item) ([]node, error) {
		left, right, ok := split(item)
		if !ok {
			return nil, nil
		}
		a, err := makeNode(left)
		if err != nil {
			return nil, err
		}
		b, err := makeNode(right)
		if err != nil {
			return nil, err
		}
		return []node{a, b}, nil
	}
	plan := make([]node, len(items))
	for i, item := range items {
		var err error
		plan[i], err = makeNode(item)
		if err != nil {
			return nil, err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// A whole-parent cached answer/replay still takes precedence over a
		// split memo. Successful in-memory siblings need neither lookup.
		for i := 0; i < len(plan); i++ {
			if plan[i].done {
				continue
			}
			found, err := loadAdaptiveSplit(executor, provider, plan[i].call)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			parts, err := children(plan[i].item)
			if err != nil {
				return nil, err
			}
			if len(parts) > 0 {
				remaining := append(parts, plan[i+1:]...)
				plan = append(plan[:i], remaining...)
				i-- // Expand any previously refused children before execution.
			}
		}
		var calls []Call[Value]
		for _, item := range plan {
			if !item.done {
				calls = append(calls, item.call)
			}
		}
		if len(calls) == 0 {
			results := make([]AdaptiveEachResult[Item, Value], len(plan))
			for i, item := range plan {
				results[i] = AdaptiveEachResult[Item, Value]{Item: item.item, Outcome: item.outcome, Err: item.err}
			}
			return results, nil
		}
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(calls))
		}
		results := ExecuteJSONEach(ctx, executor, provider, calls)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var next []node
		var terminal error
		at := 0
		for i, item := range plan {
			if item.done {
				next = append(next, item)
				continue
			}
			result := results[at]
			at++
			if result.Err == nil {
				item.outcome, item.done = result.Outcome, true
				next = append(next, item)
				continue
			}
			memo, eligible := adaptiveFailureMemo(item.call, result.Outcome, result.Err)
			if eligible {
				parts, err := children(item.item)
				if err != nil {
					return nil, err
				}
				if len(parts) > 0 {
					if err := saveAdaptiveSplit(executor, provider, result.Outcome.Request, item.call.Limits, memo); err != nil {
						return nil, err
					}
					next = append(next, parts...)
					continue
				}
			}
			if keepTerminal {
				item.outcome, item.err, item.done = result.Outcome, result.Err, true
				next = append(next, item)
				continue
			}
			if terminal == nil {
				terminal = &BatchItemError{Index: i, Err: result.Err}
			}
		}
		if terminal != nil {
			return nil, fmt.Errorf("llm: adaptive independent item: %w", terminal)
		}
		plan = next
	}
}
