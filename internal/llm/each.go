package llm

import (
	"context"
	"sync"
)

// EachResult is one item of ExecuteJSONEach: its outcome and, when the item
// failed, why. A failed item leaves its neighbours untouched.
type EachResult[T any] struct {
	Outcome Outcome[T]
	Err     error
}

// ExecuteJSONEach runs independent calls through the same pool, attempt gate
// and observer as ExecuteJSONBatch, but never cancels a sibling when one item
// fails: each item's result stays in its caller-indexed slot with its own
// error. A table window that the model answered badly is one rejected window,
// not a rejected stage. Only the caller's context ends the run early.
func ExecuteJSONEach[T any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	calls []Call[T],
) []EachResult[T] {
	results := make([]EachResult[T], len(calls))
	if len(calls) == 0 {
		return results
	}
	concurrency := executor.BatchConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	gate := executor.BatchController.bind(concurrency)
	eachCtx := bindAttemptGate(ctx, gate)
	workerCount := min(concurrency, gate.currentLimit(), len(calls))
	if workerCount < 1 {
		workerCount = 1
	}
	events := make([][]Event, len(calls))
	jobs := make(chan int)
	var workers sync.WaitGroup
	var mu sync.Mutex
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				itemExecutor := executor
				var buffer *batchEventBuffer
				if executor.Observer != nil {
					buffer = &batchEventBuffer{}
					itemExecutor.Observer = buffer
				}
				outcome, err := ExecuteJSON(eachCtx, itemExecutor, provider, calls[index])
				if err != nil && isProviderOverload(err) {
					gate.collapse()
				}
				mu.Lock()
				results[index] = EachResult[T]{Outcome: outcome, Err: err}
				if buffer != nil {
					events[index] = buffer.events
				}
				mu.Unlock()
			}
		}()
	}
	for index := range calls {
		if ctx.Err() != nil {
			mu.Lock()
			results[index] = EachResult[T]{Err: ctx.Err()}
			mu.Unlock()
			continue
		}
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	for index := range events {
		for _, event := range events[index] {
			results[index].Outcome.Issues = observe(executor.Observer, event, results[index].Outcome.Issues)
		}
	}
	return results
}
