// Package testbudget sizes a test's waits by the test binary's own deadline
// instead of a wall-clock guess: a process that is slow to start under load
// is not a hung process, and a wait that does end fails inside the test,
// with its message, before the binary's timeout kills everything.
package testbudget

import (
	"testing"
	"time"
)

// Until is how long a test may still wait: the time to its deadline less a
// margin for reporting, or ten minutes when it has no deadline.
func Until(t testing.TB) time.Duration {
	t.Helper()
	deadline, ok := t.(interface{ Deadline() (time.Time, bool) })
	if !ok {
		return 10 * time.Minute
	}
	at, has := deadline.Deadline()
	if !has {
		return 10 * time.Minute
	}
	remaining := time.Until(at) - 2*time.Second
	if remaining < time.Second {
		return time.Second
	}
	return remaining
}
