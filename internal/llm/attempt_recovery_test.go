package llm

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestRateLimitRecoveryRequiresCurrentEpochSuccessesAfterCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newAttemptGate(4)
		old := gate.recoveryEpoch()
		gate.backoff(time.Minute)
		epoch := gate.recoveryEpoch()
		for range 8 {
			gate.completed(old)
			gate.completed(epoch)
		}
		if gate.currentLimit() != 1 {
			t.Fatal("old in-flight completions or cooldown restored capacity")
		}
		time.Sleep(time.Minute)
		for range 3 {
			gate.completed(epoch)
		}
		if gate.currentLimit() != 1 {
			t.Fatal("capacity recovered before four successes")
		}
		gate.completed(epoch)
		if gate.currentLimit() != 2 {
			t.Fatal("capacity did not double")
		}
		gate.backoff(time.Minute)
		for range 8 {
			gate.completed(epoch)
		}
		if gate.currentLimit() != 1 {
			t.Fatal("later 429 did not reset recovery")
		}
		time.Sleep(time.Minute)
		epoch = gate.recoveryEpoch()
		for range 8 {
			gate.completed(epoch)
		}
		if gate.currentLimit() != 4 {
			t.Fatal("capacity failed to reach its configured ceiling")
		}
		for range 8 {
			gate.completed(epoch)
		}
		if gate.currentLimit() != 4 {
			t.Fatal("recovery exceeded configured parallelism")
		}
	})
}
