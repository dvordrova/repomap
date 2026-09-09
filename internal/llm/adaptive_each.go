package llm

import (
	"context"
	"fmt"
)

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
	type node struct {
		item    Item
		call    Call[Value]
		outcome Outcome[Value]
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
			return nil, nil, err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		// A whole-parent cached answer/replay still takes precedence over a
		// split memo. Successful in-memory siblings need neither lookup.
		for i := 0; i < len(plan); i++ {
			if plan[i].done {
				continue
			}
			found, err := loadAdaptiveSplit(executor, provider, plan[i].call)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				continue
			}
			parts, err := children(plan[i].item)
			if err != nil {
				return nil, nil, err
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
			final := make([]Item, len(plan))
			outcomes := make([]Outcome[Value], len(plan))
			for i, item := range plan {
				final[i], outcomes[i] = item.item, item.outcome
			}
			return final, outcomes, nil
		}
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(calls))
		}
		results := ExecuteJSONEach(ctx, executor, provider, calls)
		if err := ctx.Err(); err != nil {
			return nil, nil, err
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
					return nil, nil, err
				}
				if len(parts) > 0 {
					if err := saveAdaptiveSplit(executor, provider, result.Outcome.Request, item.call.Limits, memo); err != nil {
						return nil, nil, err
					}
					next = append(next, parts...)
					continue
				}
			}
			if terminal == nil {
				terminal = &BatchItemError{Index: i, Err: result.Err}
			}
		}
		if terminal != nil {
			return nil, nil, fmt.Errorf("llm: adaptive independent item: %w", terminal)
		}
		plan = next
	}
}
