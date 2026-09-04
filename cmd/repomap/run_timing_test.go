package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

// A run of chi wrote a hundred and forty-six exchanges and not one number of
// seconds. Every stage line now says how far into the run it is, every model
// call is accounted to its stage, and the run closes with where the time
// went.
func TestRunOutputSaysWhereTheTimeWent(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { return clock }

	output.Stage("Facts", "extracting")
	clock = clock.Add(90 * time.Second)
	observer := timed(output, nil).(timedObserver)
	for _, latency := range []time.Duration{4 * time.Second, 9 * time.Second} {
		observer.output.ModelCall("program_grouping", latency, false)
	}
	observer.output.ModelCall("program_grouping", 0, true)
	observer.output.ModelCall("orientation", 2*time.Second, false)
	_ = llm.EventLive
	output.Stage("Report", "path: x")
	output.Timing()

	text := buffer.String()
	for _, want := range []string{
		"Facts: (t+0s)",
		"Report: (t+1m30s)",
		"wall clock: 1m30s",
		"program_grouping: 2 live calls, 1 cached, provider time 13s, slowest 9s",
		"orientation: 1 live calls, 0 cached, provider time 2s, slowest 2s",
		"provider time in all: 15s",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}
