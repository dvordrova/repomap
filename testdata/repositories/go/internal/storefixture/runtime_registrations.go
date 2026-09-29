package storefixture

import (
	"sync"
	"time"
)

func commitPendingBatches() {}

// RunCommitWorker periodically commits pending local batches until shutdown.
func RunCommitWorker(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			commitPendingBatches()
		case <-stop:
			return
		}
	}
}

func StartCommitWorker(stop <-chan struct{}) { go RunCommitWorker(stop) }

func ScheduleCommit() { time.AfterFunc(time.Minute, commitPendingBatches) }

func RetryCommit() {
	for attempt := 0; attempt < 3; attempt++ {
		commitPendingBatches()
	}
}

// RunCompactor compacts committed batches every hour until shutdown.
func RunCompactor(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			commitPendingBatches()
		case <-stop:
			return
		}
	}
}

// StartCompactor starts the compactor the way litestream's Store starts its
// monitors: a closure marking a wait group done, named and handled by the
// one function it calls itself.
func StartCompactor(stop <-chan struct{}, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunCompactor(stop)
	}()
}

func loadCache() error { return nil }

// WarmCache loads the cache once beside the startup; its goroutine ends
// when the load does.
func WarmCache(done chan<- error) {
	go func() { done <- loadCache() }()
}

// StartBackground is what the worker service starts beside its polling
// loop: two workers for as long as it runs, a one-shot load, a one-shot
// delay and a finite retry.
func StartBackground(stop <-chan struct{}) {
	var wg sync.WaitGroup
	StartCommitWorker(stop)
	StartCompactor(stop, &wg)
	WarmCache(make(chan error, 1))
	ScheduleCommit()
	RetryCommit()
}
