package main

import (
	"fmt"
	"io"
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

	writer                       io.Writer
	currentStage                 string
	now                          func() time.Time
	lastProgress                 map[string]runOutputProgress
	reportedTargetReportWarnings map[targetReportScaleWarningOutputKey]struct{}
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
		writer:                       writer,
		now:                          time.Now,
		lastProgress:                 make(map[string]runOutputProgress),
		reportedTargetReportWarnings: make(map[targetReportScaleWarningOutputKey]struct{}),
	}
}

// Artifacts should be called as soon as the run directory is known. Keeping
// it outside a later completion summary makes failed and canceled runs
// inspectable without searching through progress output.
func (output *runOutput) Artifacts(path string) {
	output.mu.Lock()
	defer output.mu.Unlock()

	output.currentStage = ""
	fmt.Fprintln(output.writer, "Artifacts:")
	output.writeDetailsLocked(path)
}

// Stage writes a stage header once for adjacent updates and then writes each
// fact on its own indented line.
func (output *runOutput) Stage(name string, details ...string) {
	output.mu.Lock()
	defer output.mu.Unlock()

	output.stageLocked(name)
	output.writeDetailsLocked(details...)
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
	output.stageLocked("Target page")
	output.writeDetailsLocked(
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
	fmt.Fprintln(output.writer, level)
	output.writeDetailsLocked(append([]string{summary}, details...)...)
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
		output.stageLocked("Repository snapshot")
		details := []string{"collecting tracked repository facts", "repository: " + event.RepoPath}
		if event.GoTarget != "" {
			details = append(details, "Go target: "+event.GoTarget, "override: --force-platform GOOS/GOARCH")
		}
		output.writeDetailsLocked(details...)
	case orient.ProgressSnapshotReady:
		output.stageLocked("Repository snapshot")
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
		output.writeDetailsLocked(details...)
	}
}

func (output *runOutput) allowProgressLocked(
	key string,
	completed int,
	interval time.Duration,
	terminal bool,
) bool {
	now := output.now()
	previous, found := output.lastProgress[key]
	if !terminal && found && now.Sub(previous.at) < interval {
		return false
	}
	output.lastProgress[key] = runOutputProgress{at: now, completed: completed}
	return true
}

func (output *runOutput) stageLocked(name string) {
	name = singleRunOutputLine(name)
	if name == "" || name == output.currentStage {
		return
	}
	output.currentStage = name
	fmt.Fprintf(output.writer, "%s:\n", name)
}

func (output *runOutput) writeDetailsLocked(details ...string) {
	for _, detail := range details {
		detail = strings.ReplaceAll(detail, "\r\n", "\n")
		detail = strings.ReplaceAll(detail, "\r", "\n")
		for _, line := range strings.Split(detail, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Fprintf(output.writer, "  %s\n", line)
			}
		}
	}
}

func singleRunOutputLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func formatRunOutputCount(completed, total int) string {
	if total <= 0 {
		return fmt.Sprintf("completed: %d", completed)
	}
	return fmt.Sprintf("progress: %d/%d", completed, total)
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

func formatRunOutputElapsed(milliseconds int64) string {
	return "elapsed: " + (time.Duration(milliseconds) * time.Millisecond).Round(time.Second).String()
}
