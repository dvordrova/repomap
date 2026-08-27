package observability

import "sync"

type Metrics struct {
	mu       sync.Mutex
	attempts map[string]int
	failures map[string]int
}

func NewMetrics() *Metrics {
	return &Metrics{attempts: map[string]int{}, failures: map[string]int{}}
}

func (metrics *Metrics) RecordAttempt(client string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.attempts[client]++
}

func (metrics *Metrics) RecordFailure(client string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.failures[client]++
}
