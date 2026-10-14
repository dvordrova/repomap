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

// A paced gate starts attempts no closer than a minute over the rate, even
// with free leases; a 429 still collapses it.
func TestAPacedGateSpacesAttemptStartsEvenly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		controller := NewBatchController(8)
		controller.Pace(120)
		gate := controller.bind(8)
		var starts []time.Time
		for range 4 {
			release, err := gate.acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			starts = append(starts, time.Now())
			release()
		}
		for i := 1; i < len(starts); i++ {
			if gap := starts[i].Sub(starts[i-1]); gap != 500*time.Millisecond {
				t.Fatalf("start %d came %v after the previous one, want 500ms", i, gap)
			}
		}
		unpaced := NewBatchController(8).bind(8)
		before := time.Now()
		for range 4 {
			release, _ := unpaced.acquire(t.Context())
			release()
		}
		if time.Since(before) != 0 {
			t.Fatal("an unpaced gate waited")
		}
		gate.backoff(time.Minute)
		if gate.currentLimit() != 1 {
			t.Fatal("a 429 did not collapse the paced gate")
		}
	})
}
