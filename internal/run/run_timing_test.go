package run

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
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
		"[   0.000 +0.000] Facts:",
		"[  90.000 +90.000] Report:",
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
	a := debugdump.RunTiming{WallMS: 20000, Stages: []debugdump.StageTiming{{Stage: "program_grouping", Live: 3, ProviderMS: 9000, SlowestMS: 4000}}}
	b := debugdump.RunTiming{WallMS: 30000, Stages: []debugdump.StageTiming{{Stage: "program_grouping", Live: 2, Cached: 1, ProviderMS: 5000, SlowestMS: 5000}, {Stage: "program_categorization", Live: 4, ProviderMS: 8000, SlowestMS: 3000}}}
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { return clock.Add(5 * time.Minute) }
	output.ModelCall("orientation", 2*time.Second, false)
	total := wholeRunTiming(output, []targetPublishedRun{{Timing: a}, {Timing: b}})
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

type timingProviderStub struct {
	completion llm.Completion
	calls      int
}

func (*timingProviderStub) State() []byte { return []byte(`{"provider":"timing-test"}`) }
func (*timingProviderStub) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (provider *timingProviderStub) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.calls++
	return provider.completion, nil
}

func TestRunTimingIncludesRejectedLiveResponseWithoutRecountingCacheMetrics(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	executor := debugdump.BindStage(llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: timed(output, debugdump.NewSemanticObserver(nil))}, "atlas_learn")
	provider := &timingProviderStub{completion: llm.Completion{
		Response: []byte(`{"value":"refused"}`), FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: 3083 * time.Millisecond, InputTokens: 3919, OutputTokens: 314, UsageReported: true},
	}}
	type response struct {
		Value string `json:"value"`
	}
	expected := "accepted"
	call := llm.Call[response]{State: []byte(`{"contract":"timing-test"}`), Prompt: llm.Prompt{User: `{"question":"learning"}`},
		Limits: llm.Limits{MaxRequestBytes: 1024, MaxResponseBytes: 1024, MaxOutputTokens: 128},
		Validate: func(value response) error {
			if value.Value != expected {
				return errors.New("refused result")
			}
			return nil
		},
	}
	if _, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err == nil {
		t.Fatal("the first response was not rejected")
	}
	first := output.TimingReport()
	if len(first.Stages) != 1 || first.Stages[0].Live != 1 || first.Stages[0].Cached != 0 || first.Stages[0].ProviderMS != 3083 {
		t.Fatalf("refused live response disappeared from closing accounting: %+v", first)
	}
	provider.completion.Response = []byte(`{"value":"accepted"}`)
	provider.completion.Metrics.Latency = 2 * time.Second
	if _, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	if outcome, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil || !outcome.Cached || provider.calls != 2 {
		t.Fatalf("warm request did not use the accepted cache: %+v, %v", outcome, err)
	}
	// The cached answer now fails a changed local validator. Its old latency
	// must not count again; the new live replacement must count exactly once.
	expected = "replacement"
	provider.completion.Response = []byte(`{"value":"replacement"}`)
	provider.completion.Metrics.Latency = time.Second
	if outcome, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil || outcome.Cached || provider.calls != 3 {
		t.Fatalf("cache refusal did not produce one live replacement: %+v, %v", outcome, err)
	}
	result := output.TimingReport()
	if len(result.Stages) != 1 || result.Stages[0].Live != 3 || result.Stages[0].Cached != 1 || result.Stages[0].ProviderMS != 6083 || result.Stages[0].SlowestMS != 3083 {
		t.Fatalf("failure/cache timing was omitted or double-counted: %+v", result)
	}
	output.Timing()
	if !strings.Contains(buffer.String(), "atlas_learn: 3 live calls, 1 cached, provider time 6s, slowest 3s") || !strings.Contains(buffer.String(), "provider time in all: 6s") {
		t.Fatalf("human closing report lost refused live work: %s", buffer.String())
	}
}

func TestRunTimingCountsOnlyFailureEventsWithActualLiveAttempts(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	observer := timed(output, debugdump.NewSemanticObserver(nil)).(timedObserver)
	for _, failure := range []llm.FailureKind{llm.FailureProvider, llm.FailureResponse, llm.FailureValidation} {
		if err := observer.ObserveStage("live", llm.Event{Kind: llm.EventFailure, Source: llm.SourceLive, Failure: failure, Metrics: llm.Metrics{Attempts: 2, Latency: time.Second}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []llm.Event{
		{Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailurePrepare},
		{Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailureProvider, Metrics: llm.Metrics{Latency: time.Minute}},
		{Kind: llm.EventFailure, Source: llm.SourceCache, Failure: llm.FailureValidation, Metrics: llm.Metrics{Attempts: 1, Latency: time.Hour}},
		{Kind: llm.EventFailure, Source: llm.SourceCache, Failure: llm.FailureCache},
	} {
		if err := observer.ObserveStage("not-a-current-call", event); err != nil {
			t.Fatal(err)
		}
	}
	result := output.TimingReport()
	if len(result.Stages) != 1 || result.Stages[0].Stage != "live" || result.Stages[0].Live != 3 || result.Stages[0].Cached != 0 || result.Stages[0].ProviderMS != 3000 {
		t.Fatalf("non-transport failure or inherited cache metrics became live work: %+v", result)
	}
}
