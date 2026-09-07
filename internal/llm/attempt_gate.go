package llm

import (
	"context"
	"sync"
	"time"
)

// BatchController carries one adaptive provider-attempt gate across batches
// that use the same Provider. Its zero value is ready for use. The gate starts
// at the concurrency of the first bound batch and permanently collapses to one
// lease when a provider reports an explicit transport rate limit.
type BatchController struct {
	mu   sync.Mutex
	gate *attemptGate
}

func (controller *BatchController) bind(configured int) *attemptGate {
	if configured < 1 {
		configured = 1
	}
	if controller == nil {
		return newAttemptGate(configured)
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.gate == nil {
		controller.gate = newAttemptGate(configured)
	}
	return controller.gate
}

type attemptGate struct {
	mu      sync.Mutex
	limit   int
	active  int
	changed chan struct{}
	retryAt time.Time
}

func newAttemptGate(limit int) *attemptGate {
	if limit < 1 {
		limit = 1
	}
	return &attemptGate{limit: limit, changed: make(chan struct{})}
}

func (gate *attemptGate) acquire(ctx context.Context) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		gate.mu.Lock()
		if err := ctx.Err(); err != nil {
			gate.mu.Unlock()
			return nil, err
		}
		remaining := time.Until(gate.retryAt)
		if gate.active < gate.limit && remaining <= 0 {
			gate.active++
			gate.mu.Unlock()
			if err := ctx.Err(); err != nil {
				gate.release()
				return nil, err
			}
			var once sync.Once
			return func() {
				once.Do(func() { gate.release() })
			}, nil
		}
		changed := gate.changed
		gate.mu.Unlock()
		var timer *time.Timer
		var ready <-chan time.Time
		if remaining > 0 {
			timer = time.NewTimer(remaining)
			ready = timer.C
		}
		select {
		case <-changed:
		case <-ready:
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return nil, ctx.Err()
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (gate *attemptGate) release() {
	gate.mu.Lock()
	if gate.active > 0 {
		gate.active--
	}
	gate.signalLocked()
	gate.mu.Unlock()
}

func (gate *attemptGate) collapse() {
	gate.backoff(0)
}

func (gate *attemptGate) backoff(delay time.Duration) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	changed := false
	if gate.limit > 1 {
		gate.limit = 1
		changed = true
	}
	if next := time.Now().Add(delay); delay > 0 && next.After(gate.retryAt) {
		gate.retryAt = next
		changed = true
	}
	if changed {
		gate.signalLocked()
	}
}

func (gate *attemptGate) currentLimit() int {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.limit
}

func (gate *attemptGate) signalLocked() {
	close(gate.changed)
	gate.changed = make(chan struct{})
}

type attemptGateContextKey struct{}

func bindAttemptGate(ctx context.Context, gate *attemptGate) context.Context {
	if gate == nil || attemptGateForContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, attemptGateContextKey{}, gate)
}

func attemptGateForContext(ctx context.Context) *attemptGate {
	gate, _ := ctx.Value(attemptGateContextKey{}).(*attemptGate)
	return gate
}

func bindExecutorAttemptGate(ctx context.Context, executor Executor) context.Context {
	if attemptGateForContext(ctx) != nil {
		return ctx
	}
	concurrency := executor.BatchConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	return bindAttemptGate(ctx, executor.BatchController.bind(concurrency))
}

// AcquireProviderAttempt acquires the request-local transport-attempt lease.
// Providers that do not use this optional seam retain Provider compatibility;
// the returned release function is then a no-op.
func AcquireProviderAttempt(ctx context.Context) (func(), error) {
	gate := attemptGateForContext(ctx)
	if gate == nil {
		return func() {}, nil
	}
	return gate.acquire(ctx)
}

// CollapseProviderAttempts permanently reduces the request's shared attempt
// gate to one lease. A provider should call this only for an explicit transport
// overload such as HTTP 429, before releasing the attempt lease.
func CollapseProviderAttempts(ctx context.Context) {
	gate := attemptGateForContext(ctx)
	if gate != nil {
		gate.collapse()
	}
}

// BackoffProviderAttempts atomically serializes the shared gate and postpones
// every new attempt. Later overloads may extend, but never shorten, the wait.
// Already-started requests may finish; cancellation interrupts queued waits.
func BackoffProviderAttempts(ctx context.Context, delay time.Duration) {
	if gate := attemptGateForContext(ctx); gate != nil {
		gate.backoff(delay)
	}
}
