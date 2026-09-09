package run

import (
	"encoding/json"
	"fmt"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/orient"
)

const runOutputPhaseInterval = 10 * time.Second

// runOutput is the deliberately small stderr presentation boundary for an
// ordinary run. It serializes concurrent progress callbacks and keeps human
// output separate from machine-readable stdout and report artifacts.
type runOutput struct {
	mu sync.Mutex

	writer       io.Writer
	currentStage string
	now          func() time.Time
	// started and lastPrinted time the visible events. Child target outputs
	// share only this console clock; their semantic accounting stays local.
	started      time.Time
	lastPrinted  time.Time
	consoleClock *runOutput
	modelTime    map[string]*stageModelTime
	wallTime     map[string]time.Duration
	lastProgress map[string]runOutputProgress
}

// runOutputWarningSink adapts bounded warning writers to the ordinary console
// without coupling them to any semantic stage.
type runOutputWarningSink struct {
	output  *runOutput
	summary string
}

func (writer runOutputWarningSink) Write(data []byte) (int, error) {
	if writer.output != nil {
		detail := strings.TrimSpace(string(data))
		detail = strings.TrimPrefix(detail, "warning: ")
		writer.output.Warn(writer.summary, detail)
	}
	return len(data), nil
}

type runOutputProgress struct {
	at        time.Time
	completed int
}

// targetPageConsoleContext is live console-only context. It keeps repeated
// Architecture and Study stage blocks attributable when one invocation
// publishes several ordinary target pages.
type targetPageConsoleContext struct {
	DisplayPath string
	Scope       string
	RunID       string
	Role        string
}

func analysisTargetSubject(target analysistarget.Target) string {
	if target.Kind == analysistarget.KindModuleLibrary {
		return target.ModulePath + " library API"
	}
	return target.PackagePath
}

func newRunOutput(writer io.Writer) *runOutput {
	if writer == nil {
		writer = io.Discard
	}
	return &runOutput{
		writer:       writer,
		now:          time.Now,
		started:      time.Now(),
		modelTime:    make(map[string]*stageModelTime),
		lastProgress: make(map[string]runOutputProgress),
	}
}

// Artifacts should be called as soon as the run directory is known. Keeping
// it outside a later completion summary makes failed and canceled runs
// inspectable without searching through progress output.
func (output *runOutput) Artifacts(path string) {
	output.mu.Lock()
	defer output.mu.Unlock()

	output.currentStage = ""
	output.writeEventLocked(output.writer, "Artifacts:", path)
}

// Stage writes a stage header once for adjacent updates and then writes each
// fact on its own indented line.
func (output *runOutput) Stage(name string, details ...string) {
	output.mu.Lock()
	defer output.mu.Unlock()

	output.writeEventLocked(output.writer, output.stageLocked(name), details...)
}

// State keeps a terminal or intermediate state visible below its stage rather
// than burying it in a dense progress sentence.
func (output *runOutput) State(stage, state string, details ...string) {
	lines := make([]string, 0, len(details)+1)
	lines = append(lines, "state: "+singleRunOutputLine(state))
	lines = append(lines, details...)
	output.Stage(stage, lines...)
}

func (output *runOutput) Warn(summary string, details ...string) {
	output.level("WARN", summary, details...)
}

func (output *runOutput) Error(summary string, details ...string) {
	output.level("ERROR", summary, details...)
}

// TargetPage marks the beginning or end of one ordinary target pipeline. The
// stages between matching markers retain their existing wording and accounting
// while gaining an exact target and run context.
func (output *runOutput) TargetPage(state string, target targetPageConsoleContext) {
	if output == nil {
		return
	}
	output.mu.Lock()
	defer output.mu.Unlock()

	// A completed default immediately followed by a started sibling must still
	// produce two visible boundaries; ordinary adjacent-stage coalescing would
	// otherwise merge their details under one header.
	output.currentStage = ""
	output.writeEventLocked(output.writer, output.stageLocked("Target page"),
		"state: "+singleRunOutputLine(state),
		"target: "+target.DisplayPath,
		"scope: "+target.Scope,
		"run: "+target.RunID,
		"role: "+target.Role,
	)
}

func (output *runOutput) level(level, summary string, details ...string) {
	output.mu.Lock()
	defer output.mu.Unlock()

	output.currentStage = ""
	output.writeEventLocked(output.writer, level, append([]string{summary}, details...)...)
}

// Progress is the adapter for the existing bounded orient.ProgressEvent
// contract. Started/completed/state events are always printed. Repeated phase
// counters and wait heartbeats are throttled; the underlying work and saved
// metrics are untouched.
func (output *runOutput) Progress(event orient.ProgressEvent) {
	output.mu.Lock()
	defer output.mu.Unlock()

	switch event.Stage {
	case orient.ProgressSnapshotStarted:
		header := output.stageLocked("Repository snapshot")
		details := []string{"collecting tracked repository facts", "repository: " + event.RepoPath}
		if event.GoTarget != "" {
			details = append(details, "Go target: "+event.GoTarget, "override: --force-platform GOOS/GOARCH")
		}
		output.writeEventLocked(output.writer, header, details...)
	case orient.ProgressSnapshotReady:
		header := output.stageLocked("Repository snapshot")
		details := []string{
			"state: complete",
			fmt.Sprintf("tracked files: %d", event.FileCount),
			formatRunOutputDuration(event.LatencyMillis),
		}
		if event.GoTargetProvenance != "" {
			details = append(details,
				"Go target: "+event.GoTargetProvenance,
				fmt.Sprintf("platform evidence: %d target-specific production Go file(s)", event.GoTargetEvidenceCount),
			)
			if len(event.GoTargetEvidencePaths) > 0 {
				details = append(details, "evidence: "+strings.Join(event.GoTargetEvidencePaths, ", "))
			}
		} else if event.SuggestedGoTarget != "" {
			details = append(details,
				fmt.Sprintf("platform hint: %s has %d target-specific production Go file(s)", event.SuggestedGoTarget, event.GoTargetEvidenceCount),
				"try: --force-platform "+event.SuggestedGoTarget,
			)
			if len(event.GoTargetEvidencePaths) > 0 {
				details = append(details, "evidence: "+strings.Join(event.GoTargetEvidencePaths, ", "))
			}
		}
		output.writeEventLocked(output.writer, header, details...)
	}
}

func (output *runOutput) stageLocked(name string) string {
	name = singleRunOutputLine(name)
	if name == "" || name == output.currentStage {
		return ""
	}
	output.currentStage = name
	return name + ":"
}

func sinceStart(output *runOutput) string {
	if output.started.IsZero() {
		return "0s"
	}
	return output.now().Sub(output.started).Round(time.Second).String()
}

// stageModelTime is what the provider took for one stage: how many calls
// were live and how many the cache answered, and the sum and the longest of
// the live ones. Live calls in one stage run several at a time, so the sum
// is provider effort and the wall clock is what the reader waited.
type stageModelTime struct {
	live, cached int
	sum, longest time.Duration
}

// Wall accounts a stretch of non-model work, such as building the program
// index, so the Time block can say what the model was not responsible for.
func (output *runOutput) Wall(name string, duration time.Duration) {
	if output == nil {
		return
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.wallTime == nil {
		output.wallTime = make(map[string]time.Duration)
	}
	output.wallTime[name] += duration
}

// ModelCall records one provider call under its stage. It waits for nothing.
func (output *runOutput) ModelCall(stage string, latency time.Duration, cached bool) {
	if output == nil {
		return
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	at := output.modelTime[stage]
	if at == nil {
		at = &stageModelTime{}
		output.modelTime[stage] = at
	}
	if cached {
		at.cached++
		return
	}
	at.live++
	at.sum += latency
	if latency > at.longest {
		at.longest = latency
	}
}

// Timing closes the run with where the time went: the wall clock, and for
// every stage that asked the model, how many calls, how much provider time
// and how long the slowest call took. kubernetes in an hour starts with
// knowing which of these numbers is the hour.
func (output *runOutput) Timing() {
	if output == nil {
		return
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	header := output.stageLocked("Time")
	lines := []string{"wall clock: " + sinceStart(output)}
	walls := make([]string, 0, len(output.wallTime))
	for name := range output.wallTime {
		walls = append(walls, name)
	}
	sort.Strings(walls)
	for _, name := range walls {
		lines = append(lines, fmt.Sprintf("%s: %s, no model", name, output.wallTime[name].Round(time.Millisecond)))
	}
	stages := make([]string, 0, len(output.modelTime))
	for stage := range output.modelTime {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	var total time.Duration
	for _, stage := range stages {
		at := output.modelTime[stage]
		total += at.sum
		lines = append(lines, fmt.Sprintf(
			"%s: %d live calls, %d cached, provider time %s, slowest %s",
			stage, at.live, at.cached, at.sum.Round(time.Second), at.longest.Round(time.Second),
		))
	}
	if len(stages) > 0 {
		lines = append(lines, "provider time in all: "+total.Round(time.Second).String())
	}
	output.writeEventLocked(output.writer, header, lines...)
}

// modelCallSummary uses the same accounting as the closing Time stage.
func (output *runOutput) modelCallSummary(stage string) string {
	if output == nil {
		return ""
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	at := output.modelTime[stage]
	if at == nil {
		return "provider requests: 0 new, 0 reused from cache"
	}
	return fmt.Sprintf("provider requests: %d new, %d reused from cache", at.live, at.cached)
}

// TimingReport is the Time stage as data, for the run's metadata.
func (output *runOutput) TimingReport() debugdump.RunTiming {
	if output == nil {
		return debugdump.RunTiming{}
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	report := debugdump.RunTiming{WallMS: output.now().Sub(output.started).Milliseconds()}
	stages := make([]string, 0, len(output.modelTime))
	for stage := range output.modelTime {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	for _, stage := range stages {
		at := output.modelTime[stage]
		report.Stages = append(report.Stages, debugdump.StageTiming{
			Stage: stage, Live: at.live, Cached: at.cached,
			ProviderMS: at.sum.Milliseconds(), SlowestMS: at.longest.Milliseconds(),
		})
	}
	return report
}

// writeRunTiming records the run's account in its metadata, so the page
// generated next can say how long the run took and where.
func writeRunTiming(runDir string, timing debugdump.RunTiming) error {
	path := filepath.Join(runDir, "metadata.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("run timing: read metadata: %w", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return fmt.Errorf("run timing: decode metadata: %w", err)
	}
	metadata["timing"] = timing
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("run timing: encode metadata: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("run timing: write metadata: %w", err)
	}
	return nil
}

// timedObserver hands every model call to the run output's clock on its way
// to the exchange journal.
type timedObserver struct {
	output *runOutput
	inner  *debugdump.SemanticObserver
}

func timed(output *runOutput, inner *debugdump.SemanticObserver) llm.Observer {
	if output == nil {
		return inner
	}
	inner.SetFailureNotice(func(receipt debugdump.SemanticFailureReceipt) {
		headline := "Model request failed"
		if receipt.State == debugdump.SemanticStateRejected {
			headline = "Model response rejected"
		}
		details := []string{"stage: " + receipt.Stage}
		if receipt.Stage == "report_translation" {
			details = append(details, "completed translations are kept; this failure does not cancel neighbouring requests")
		} else if receipt.State == debugdump.SemanticStateRejected {
			switch {
			case receipt.Stage == "orientation":
				details = append(details, "this response adds no overview; repository facts and maps remain available")
			case strings.HasPrefix(receipt.Stage, "atlas_"):
				details = append(details, "this request has no accepted model answer; accepted results from other requests are kept")
			}
		}
		details = append(details, "reason: "+receipt.Reason,
			fmt.Sprintf("transport attempts: %d", receipt.TransportAttempts), formatRunOutputDuration(receipt.LatencyMS),
			"request: "+receipt.RequestPath)
		if response := receipt.HTTPResponse; response != nil {
			details = append(details, fmt.Sprintf("last HTTP response: %d", response.StatusCode))
			var names []string
			for name := range response.Headers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				for _, value := range response.Headers[name] {
					details = append(details, fmt.Sprintf("response header %s: %q", name, value))
				}
			}
		}
		if receipt.ResponseUnavailable != "" {
			details = append(details, "raw response (last attempt): unavailable ("+receipt.ResponseUnavailable+")")
		} else {
			details = append(details, "raw response (last attempt): "+receipt.ResponsePath)
		}
		details = append(details, "journal: "+receipt.JournalPath)
		output.Warn(headline, details...)
	})
	return timedObserver{output: output, inner: inner}
}

func (observer timedObserver) Observe(event llm.Event) error {
	return observer.inner.Observe(event)
}

func (observer timedObserver) ObserveStage(stage string, event llm.Event) error {
	if event.Kind != llm.EventFailure {
		observer.output.ModelCall(stage, event.Metrics.Latency, event.Source == llm.SourceCache)
	} else if event.Source == llm.SourceLive && event.Metrics.Attempts > 0 {
		switch event.Failure {
		case llm.FailureProvider, llm.FailureResponse, llm.FailureValidation:
			// A refused response still consumed a live provider call. Cache
			// rejection carries the old call's metrics, while preparation or
			// cancellation before transport has no attempt to account here.
			observer.output.ModelCall(stage, max(0, event.Metrics.Latency), false)
		}
	}
	return observer.inner.ObserveStage(stage, event)
}

// One public progress event has one timestamp. Continuation lines align with
// its body; blank or suppressed events neither sample nor advance the clock.
// The caller holds output.mu, and a child additionally serializes its write
// through the root console's mutex without changing either timing account.
func (output *runOutput) writeEventLocked(writer io.Writer, header string, details ...string) {
	var lines []string
	if header != "" {
		lines = append(lines, header)
	}
	for _, detail := range details {
		detail = strings.ReplaceAll(strings.ReplaceAll(detail, "\r\n", "\n"), "\r", "\n")
		for _, line := range strings.Split(detail, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, "  "+line)
			}
		}
	}
	if len(lines) == 0 {
		return
	}
	clock := output
	if output.consoleClock != nil {
		clock = output.consoleClock
		clock.mu.Lock()
		defer clock.mu.Unlock()
	}
	now := clock.now()
	if now.Before(clock.started) {
		now = clock.started
	}
	if now.Before(clock.lastPrinted) {
		now = clock.lastPrinted
	}
	elapsed := now.Sub(clock.started)
	delta := time.Duration(0)
	if !clock.lastPrinted.IsZero() {
		delta = now.Sub(clock.lastPrinted)
	}
	prefix := fmt.Sprintf("[%8.3f +%.3f] ", elapsed.Seconds(), delta.Seconds())
	body := prefix + strings.Join(lines, "\n"+strings.Repeat(" ", len(prefix))) + "\n"
	if n, _ := io.WriteString(writer, body); n > 0 {
		clock.lastPrinted = now
	}
}

func (output *runOutput) consoleTime() time.Time {
	clock := output
	if output.consoleClock != nil {
		clock = output.consoleClock
	}
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now()
}

func singleRunOutputLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func formatRunOutputDuration(milliseconds int64) string {
	return "duration: " + (time.Duration(milliseconds) * time.Millisecond).Round(time.Millisecond).String()
}

// formatRunOutputWallDuration preserves the sub-millisecond truth for cheap
// local cubes instead of presenting completed work as a fabricated zero.
func formatRunOutputWallDuration(duration time.Duration) string {
	if duration > 0 && duration < time.Millisecond {
		return "duration: <1ms"
	}
	return "duration: " + duration.Round(time.Millisecond).String()
}

// wholeRunTiming merges every target run's account under the driving run's
// wall clock: per stage the calls add up and the slowest stays the slowest.
func wholeRunTiming(output *runOutput, runs []targetPublishedRun) debugdump.RunTiming {
	total := debugdump.RunTiming{}
	if output != nil {
		total = output.TimingReport()
	}
	byStage := make(map[string]*debugdump.StageTiming)
	for position := range total.Stages {
		byStage[total.Stages[position].Stage] = &total.Stages[position]
	}
	for _, run := range runs {
		for _, stage := range run.Timing.Stages {
			at, known := byStage[stage.Stage]
			if !known {
				total.Stages = append(total.Stages, stage)
				byStage = make(map[string]*debugdump.StageTiming)
				for position := range total.Stages {
					byStage[total.Stages[position].Stage] = &total.Stages[position]
				}
				continue
			}
			at.Live += stage.Live
			at.Cached += stage.Cached
			at.ProviderMS += stage.ProviderMS
			if stage.SlowestMS > at.SlowestMS {
				at.SlowestMS = stage.SlowestMS
			}
		}
	}
	sort.Slice(total.Stages, func(left, right int) bool {
		return total.Stages[left].Stage < total.Stages[right].Stage
	})
	return total
}
