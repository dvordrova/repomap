package main

import (
	"bytes"
	"github.com/dvordrova/repomap/internal/debugdump"
	"os"
	"path/filepath"
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

// The owner page's account is the whole run's: every target run's stages
// add up under the driving run's wall clock.
func TestWholeRunTimingMergesTargetRuns(t *testing.T) {
	runsDir := t.TempDir()
	write := func(name string, timing debugdump.RunTiming) string {
		dir := filepath.Join(runsDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"repo_name":"x"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeRunTiming(dir, timing); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	a := write("a", debugdump.RunTiming{WallMS: 20000, Stages: []debugdump.StageTiming{{Stage: "program_grouping", Live: 3, ProviderMS: 9000, SlowestMS: 4000}}})
	b := write("b", debugdump.RunTiming{WallMS: 30000, Stages: []debugdump.StageTiming{{Stage: "program_grouping", Live: 2, Cached: 1, ProviderMS: 5000, SlowestMS: 5000}, {Stage: "program_categorization", Live: 4, ProviderMS: 8000, SlowestMS: 3000}}})
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { return clock.Add(5 * time.Minute) }
	output.ModelCall("orientation", 2*time.Second, false)
	total := wholeRunTiming(output, []targetPublishedRun{{RunDir: a}, {RunDir: b}})
	if total.WallMS != 300000 || len(total.Stages) != 3 {
		t.Fatalf("total = %#v", total)
	}
	byStage := map[string]debugdump.StageTiming{}
	for _, stage := range total.Stages {
		byStage[stage.Stage] = stage
	}
	if g := byStage["program_grouping"]; g.Live != 5 || g.Cached != 1 || g.ProviderMS != 14000 || g.SlowestMS != 5000 {
		t.Errorf("grouping = %#v", g)
	}
	if byStage["orientation"].ProviderMS != 2000 || byStage["program_categorization"].Live != 4 {
		t.Errorf("stages = %#v", total.Stages)
	}
}
