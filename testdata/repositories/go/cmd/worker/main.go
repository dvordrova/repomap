package main

import (
	"example.com/repomap/cumulative-go-fixture/internal/storefixture"
	"example.com/repomap/cumulative-go-fixture/pkg/servicecfg"
	"log"
	"time"
)

func processPendingJobs() { log.Print("checking pending jobs") }
func main() {
	storefixture.StartBackground(make(chan struct{}))
	processPendingJobs() // startup: once before the loop
	for range time.Tick(time.Duration(servicecfg.PollIntervalSeconds()) * time.Second) {
		processPendingJobs()
	}
}

func processOncePerItem(items []string) {
	for range items {
		processPendingJobs()
	}
}

func waitForJobs(ticks <-chan time.Time, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-ticks:
			processPendingJobs()
		}
	}
}

func registerCallbacks(items []string) []func() {
	var callbacks []func()
	for range items {
		callbacks = append(callbacks, func() {
			processPendingJobs() // callback body does not inherit the registration loop
		})
	}
	return callbacks
}

// The worker's own hook, which the shared storefixture code runs in the
// worker alone.
type workerHook struct{}

func (workerHook) Run() { processPendingJobs() }

func runWorkerHook() { storefixture.RunLinkedHook(workerHook{}) }
