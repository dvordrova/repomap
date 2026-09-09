package main

import (
	"example.com/repomap/cumulative-go-fixture/pkg/servicecfg"
	"log"
	"time"
)

func processPendingJobs() { log.Print("checking pending jobs") }
func main() {
	for range time.Tick(time.Duration(servicecfg.PollIntervalSeconds()) * time.Second) {
		processPendingJobs()
	}
}
