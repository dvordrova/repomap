package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orient"
)

func TestRunOutputTimestampsEveryEventAndAlignsContinuation(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { return clock }
	output.Artifacts("/tmp/run")
	clock = clock.Add(1250 * time.Millisecond)
	output.Stage("Facts", "collecting\r\n  declarations\n\n")
	clock = clock.Add(2500 * time.Millisecond)
	output.State("Facts", "complete", "12 sources")
	clock = clock.Add(125 * time.Millisecond)
	output.Warn("one warning", "first detail\nsecond detail")
	clock = clock.Add(5 * time.Second)
	output.Error("one error")
	clock = clock.Add(time.Second)
	output.Progress(orient.ProgressEvent{Stage: orient.ProgressSnapshotStarted, RepoPath: "/repo"})
	text := buffer.String()
	for _, want := range []string{
		"[   0.000 +0.000] Artifacts:\n" + strings.Repeat(" ", 20) + "/tmp/run\n",
		"[   1.250 +1.250] Facts:\n" + strings.Repeat(" ", 20) + "collecting\n" + strings.Repeat(" ", 20) + "declarations\n",
		"[   3.750 +2.500]   state: complete\n" + strings.Repeat(" ", 20) + "12 sources\n",
		"[   3.875 +0.125] WARN",
		"[   8.875 +5.000] ERROR",
		"[   9.875 +1.000] Repository snapshot:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing event %q:\n%s", want, text)
		}
	}
	if count := len(regexp.MustCompile(`(?m)^\[`).FindAllString(text, -1)); count != 6 {
		t.Fatalf("%d prefixes for six logical events:\n%s", count, text)
	}
	t.Log("console sample:\n" + text)
}

func TestRunOutputSuppressedBlankAndAccountingDoNotAdvanceDelta(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started.Add(time.Second)
	output.now = func() time.Time { return clock }
	output.Stage("Facts", "first")
	before := buffer.String()
	clock = clock.Add(2 * time.Second)
	output.Stage("Facts", " \r\n ")
	output.Stage(" \n ")
	output.ModelCall("answer", time.Second, false)
	output.Wall("facts", time.Second)
	output.TimingReport()
	if buffer.String() != before {
		t.Fatal("suppressed event printed text")
	}
	clock = clock.Add(3 * time.Second)
	output.Stage("Facts", "second")
	if !strings.Contains(buffer.String(), "[   6.000 +5.000]   second") {
		t.Fatalf("delta used an unprinted event:\n%s", buffer.String())
	}
}

func TestRunOutputSerializesConcurrentEventsWithMonotonicTimes(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { clock = clock.Add(time.Millisecond); return clock }
	var work sync.WaitGroup
	for i := 0; i < 40; i++ {
		work.Add(1)
		go func(i int) {
			defer work.Done()
			output.Stage(fmt.Sprintf("Task %d", i), fmt.Sprintf("first %d\nsecond %d", i, i))
		}(i)
	}
	work.Wait()
	lines := strings.Split(strings.TrimSuffix(buffer.String(), "\n"), "\n")
	if len(lines) != 120 {
		t.Fatalf("lost output: %d lines", len(lines))
	}
	header := regexp.MustCompile(`^\[\s*([0-9]+\.[0-9]{3}) \+([0-9]+\.[0-9]{3})\] Task ([0-9]+):$`)
	for event := 0; event < 40; event++ {
		match := header.FindStringSubmatch(lines[event*3])
		if len(match) != 4 {
			t.Fatalf("broken event header: %q", lines[event*3])
		}
		elapsed, _ := strconv.ParseFloat(match[1], 64)
		delta, _ := strconv.ParseFloat(match[2], 64)
		wantDelta := 0.001
		if event == 0 {
			wantDelta = 0
		}
		if elapsed != float64(event+1)/1000 || delta != wantDelta {
			t.Fatalf("nonmonotonic event: %q", lines[event*3])
		}
		if strings.TrimSpace(lines[event*3+1]) != "first "+match[3] || strings.TrimSpace(lines[event*3+2]) != "second "+match[3] {
			t.Fatal("concurrent multi-line events interleaved")
		}
	}
}

func TestRunOutputChildSharesOnlyConsoleClockAndErrorsKeepIt(t *testing.T) {
	var buffer bytes.Buffer
	parent := newRunOutput(&buffer)
	clock := parent.started
	parent.now = func() time.Time { return clock }
	parent.Stage("Root", "first")
	clock = clock.Add(20 * time.Second)
	child := newRunOutput(&buffer)
	child.consoleClock = parent
	child.ModelCall("child", 4*time.Second, false)
	child.Stage("Target", "working")
	clock = clock.Add(2310 * time.Millisecond)
	writeRunOutputError(parent, errors.New("stopped"))
	if !strings.Contains(buffer.String(), "[  20.000 +20.000] Target:") || !strings.Contains(buffer.String(), "[  22.310 +2.310] ERROR") {
		t.Fatalf("child/error reset the clock:\n%s", buffer.String())
	}
	if len(parent.TimingReport().Stages) != 0 || child.TimingReport().Stages[0].Live != 1 {
		t.Fatal("console clock sharing changed model accounting")
	}
	clock = clock.Add(time.Second)
	writeRunOutputError(parent, context.Canceled)
	if !strings.Contains(buffer.String(), "[  23.310 +1.000] Run:") {
		t.Fatal("cancellation reset the clock")
	}
}

func TestRunOutputDoesNotMoveBackwardsOrCountFailedWrites(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started.Add(time.Second)
	output.now = func() time.Time { return clock }
	output.Stage("First")
	clock = clock.Add(-time.Second)
	output.Stage("Backwards clock")
	if !strings.Contains(buffer.String(), "[   1.000 +0.000] Backwards clock:") {
		t.Fatal("console time moved backwards")
	}
	output.writer = failedConsoleWriter{}
	clock = clock.Add(3 * time.Second)
	output.Stage("Failed write")
	output.writer = &buffer
	clock = clock.Add(time.Second)
	output.Stage("Visible")
	if !strings.Contains(buffer.String(), "[   4.000 +3.000] Visible:") {
		t.Fatal("failed write advanced the visible-event delta")
	}
}

type failedConsoleWriter struct{}

func (failedConsoleWriter) Write([]byte) (int, error) { return 0, errors.New("not written") }

func TestModelWaitUsesRunClockAcrossRequestsAndConcurrentCallbacks(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started
	output.now = func() time.Time { return clock }
	wait := waitingOnTheModel(output)
	output.Stage("Analysis")
	clock = clock.Add(10 * time.Minute)
	wait(deepseek.WaitProgress{Stage: "model completion", Elapsed: 10 * time.Minute})
	before := buffer.String()
	clock = clock.Add(40 * time.Second)
	wait(deepseek.WaitProgress{Stage: "model completion", Elapsed: 30 * time.Second})
	if buffer.String() != before {
		t.Fatal("heartbeat ignored its wall-clock throttle")
	}
	clock = clock.Add(21 * time.Second)
	// A new 30-second request is eligible even after the previous 10-minute one.
	var work sync.WaitGroup
	for i := 0; i < 20; i++ {
		work.Add(1)
		go func() {
			defer work.Done()
			wait(deepseek.WaitProgress{Stage: "model completion", Elapsed: 30 * time.Second})
		}()
	}
	work.Wait()
	if strings.Count(buffer.String(), "still waiting") != 2 || !strings.Contains(buffer.String(), "[ 661.000 +61.000]   still waiting on the model: 30s") {
		t.Fatalf("new request remained silent or concurrent callbacks duplicated it:\n%s", buffer.String())
	}
	clock = clock.Add(time.Minute)
	wait(deepseek.WaitProgress{Stage: "other stage", Elapsed: 29999 * time.Millisecond})
	clock = clock.Add(time.Second)
	output.Stage("Analysis", "progress")
	if !strings.Contains(buffer.String(), "[ 722.000 +61.000]   progress") {
		t.Fatal("suppressed short heartbeat changed the event delta")
	}
}

func TestModelWaitFactoryBindsExistingClientWithoutPreparingARequest(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	clock := output.started.Add(42 * time.Second)
	output.now = func() time.Time { return clock }
	client := &deepseek.Client{}
	provider, err := providerFactoryWithOutput(func() (llm.Provider, error) { return client, nil }, output)()
	if err != nil || provider != client || client.OnWait == nil {
		t.Fatalf("client heartbeat was not bound: %v", err)
	}
	client.OnWait(deepseek.WaitProgress{Stage: "model completion", Elapsed: 30 * time.Second})
	if !strings.Contains(buffer.String(), "[  42.000 +0.000]   still waiting") {
		t.Fatal("factory installed a separate console clock")
	}
	cause := errors.New("configuration invalid")
	if _, err := providerFactoryWithOutput(func() (llm.Provider, error) { return (*deepseek.Client)(nil), cause }, output)(); !errors.Is(err, cause) {
		t.Fatal("factory error changed")
	}
}

func TestRunOutputTerminalErrorKeepsStreamAndClock(t *testing.T) {
	var progress, diagnostics bytes.Buffer
	output := newRunOutput(&progress)
	clock := output.started
	output.now = func() time.Time { return clock }
	output.Stage("Reading", "working")
	before := progress.String()
	clock = clock.Add(12500 * time.Millisecond)
	writeRunOutputErrorTo(output, &diagnostics, errors.New("failed"))
	if progress.String() != before || !strings.Contains(diagnostics.String(), "[  12.500 +12.500] ERROR") {
		t.Fatalf("error changed streams or reset clock: %q / %q", progress.String(), diagnostics.String())
	}
}
