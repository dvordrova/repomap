package storefixture

import "time"

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
