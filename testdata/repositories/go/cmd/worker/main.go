package main

import (
	"example.com/repomap/cumulative-go-fixture/pkg/servicecfg"
	"log"
	"time"
)

func processPendingJobs() { log.Print("checking pending jobs") }
func main() {
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
