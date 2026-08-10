package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/report"
)

const (
	runOutputPhaseInterval   = 10 * time.Second
	runOutputWaitingInterval = 30 * time.Second
)

// runOutput is the deliberately small stderr presentation boundary for an
// ordinary run. It serializes concurrent progress callbacks and keeps human
// output separate from machine-readable stdout and report artifacts.
type runOutput struct {
	mu sync.Mutex

	writer       io.Writer
	currentStage string
	now          func() time.Time
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

func newRunOutput(writer io.Writer) *runOutput {
	if writer == nil {
		writer = io.Discard
	}
	return &runOutput{
		writer:       writer,
		now:          time.Now,
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

// MapConnectivity explains the local fact-to-edge projection in the console,
// not in the report. A report is user-facing documentation; operational
// suppression counts belong beside the run that produced it.
func (output *runOutput) MapConnectivity(counts report.ArchitectureStructuralConnectivity) {
	if counts.PackageImportFactCount == 0 {
		return
	}
	details := []string{
		fmt.Sprintf("exact package-import facts: %d", counts.PackageImportFactCount),
		fmt.Sprintf(
			"projected: %d witness(es) across %d component-pair edge(s)",
			counts.ProjectedWitnessCount,
			counts.ProjectedPairEdgeCount,
		),
		fmt.Sprintf("retained inside one component (no Map edge): %d", counts.SuppressedIntraComponentCount),
		fmt.Sprintf("suppressed because an endpoint has no final component: %d", counts.SuppressedUnjoinedEndpointCount),
		fmt.Sprintf("suppressed because final ownership is plural: %d", counts.SuppressedPluralOwnershipCount),
	}
	if counts.SuppressedUnjoinedEndpointCount > 0 || counts.SuppressedPluralOwnershipCount > 0 {
		output.Warn("Map package connectivity is partial", details...)
		return
	}
	output.State("Map connectivity", "complete", details...)
}

// ArchitectureScope explains deterministic model-input omissions in the
// console. The complete saved RepositoryGraph and the user report remain free
// of operational selector diagnostics.
func (output *runOutput) ArchitectureScope(scope report.ArchitectureProductScope) {
	if scope.OmittedModules == 0 {
		return
	}
	output.Warn(
		"Architecture model scope omits non-production modules",
		fmt.Sprintf("modules retained: %d/%d", scope.RetainedModules, scope.ObservedModules),
		fmt.Sprintf("packages retained: %d/%d", scope.RetainedPackages, scope.ObservedPackages),
		fmt.Sprintf("exact package edges retained: %d/%d", scope.RetainedEdges, scope.ObservedEdges),
		fmt.Sprintf("whole non-production modules omitted: %d", scope.OmittedModules),
		"the complete local repository graph remains available",
	)
}

// ThemeInputClosure makes deterministic pre-provider Atlas shaping visible in
// the ordinary console. The authoritative saved Atlas and the user report stay
// unchanged; these counts explain why a large repository can still produce a
// bounded Theme request without repeating a forensic artifact join.
func (output *runOutput) ThemeInputClosure(closure themeStudyAtlasClosure) {
	if closure.ObservedUnits == closure.RetainedUnits &&
		closure.ObservedEntities == closure.RetainedEntities &&
		closure.ObservedEvidence == closure.RetainedEvidence &&
		closure.ObservedObservations == closure.RetainedObservations &&
		closure.ObservedRelations == closure.RetainedRelations {
		return
	}
	output.State(
		"Study request preparation", "bounded",
		fmt.Sprintf(
			"records needed to prepare this Study request: %d (%d local Atlas records remain unchanged)",
			closure.RetainedUnits,
			closure.ObservedUnits,
		),
		"repository scope and Architecture groups: unchanged",
		"provider-visible Study surfaces and evidence: unchanged",
		"the complete local Repository Atlas remains authoritative and unchanged",
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
			details = append(details, "Go target: "+event.GoTarget, "choose another: --go-target GOOS/GOARCH")
		}
		output.writeDetailsLocked(details...)
	case orient.ProgressSnapshotReady:
		output.stageLocked("Repository snapshot")
		details := []string{
			"state: complete",
			fmt.Sprintf("tracked files: %d", event.FileCount),
			formatRunOutputDuration(event.LatencyMillis),
		}
		if event.SuggestedGoTarget != "" {
			details = append(details,
				fmt.Sprintf("platform hint: %s has %d target-specific production Go file(s)", event.SuggestedGoTarget, event.GoTargetEvidenceCount),
				"try: --go-target "+event.SuggestedGoTarget,
			)
			if len(event.GoTargetEvidencePaths) > 0 {
				details = append(details, "evidence: "+strings.Join(event.GoTargetEvidencePaths, ", "))
			}
		}
		output.writeDetailsLocked(details...)
	case orient.ProgressBundleReady:
		output.stageLocked("Model context")
		output.writeDetailsLocked(
			"state: prepared",
			fmt.Sprintf("request context bytes: %d", event.BundleBytes),
			fmt.Sprintf("candidate files: %d", event.CandidateCount),
			formatRunOutputDuration(event.LatencyMillis),
		)
	case orient.ProgressSurfaceStarted:
		output.stageLocked("Runtime surfaces")
		output.writeDetailsLocked("discovering local Go runtime surfaces")
	case orient.ProgressSurfacePhase:
		output.surfacePhaseLocked(event)
	case orient.ProgressSurfaceWaiting, orient.ProgressPlanningWaiting:
		key := "waiting/" + string(event.Stage) + "/" + event.Activity
		if !output.allowProgressLocked(key, 0, runOutputWaitingInterval, false) {
			return
		}
		stage := "Runtime surfaces"
		if event.Stage == orient.ProgressPlanningWaiting {
			stage = "Research"
		}
		output.stageLocked(stage)
		output.writeDetailsLocked(
			singleRunOutputLine(event.Activity)+" is still running",
			formatRunOutputElapsed(event.LatencyMillis),
			"Ctrl-C to cancel",
		)
	case orient.ProgressSurfaceReady:
		output.stageLocked("Runtime surfaces")
		output.writeDetailsLocked(
			"state: complete",
			fmt.Sprintf("surfaces: %d", event.SurfaceCount),
			formatRunOutputDuration(event.LatencyMillis),
		)
	case orient.ProgressSurfaceFailed:
		output.currentStage = ""
		fmt.Fprintln(output.writer, "WARN")
		output.writeDetailsLocked("runtime surface discovery failed", event.Warning)
	case orient.ProgressModelRequest:
		output.stageLocked("Orientation")
		output.writeDetailsLocked(
			"state: request prepared",
			fmt.Sprintf("request bytes: %d", event.RequestBytes),
			"model: "+event.Model,
		)
	case orient.ProgressProviderWaiting:
		key := "waiting/provider/" + event.Activity + "/" + event.Model
		if !output.allowProgressLocked(key, 0, runOutputWaitingInterval, false) {
			return
		}
		output.stageLocked("Orientation")
		output.writeDetailsLocked(
			singleRunOutputLine(event.Activity)+" is still running",
			"model: "+event.Model,
			formatRunOutputElapsed(event.LatencyMillis),
			"Ctrl-C to cancel",
		)
	case orient.ProgressOrientationDone:
		state := "accepted"
		if event.Cached {
			state = "cached"
		}
		output.stageLocked("Orientation")
		output.writeDetailsLocked(
			"state: "+state,
			fmt.Sprintf("response bytes: %d", event.ResponseBytes),
			fmt.Sprintf("accepted directions: %d", event.CandidateCount),
			fmt.Sprintf("rejected directions: %d", event.RejectedCount),
			formatRunOutputTokens(event.InputTokens, event.OutputTokens),
			formatRunOutputDuration(event.LatencyMillis),
		)
	case orient.ProgressResearchPrepared:
		output.stageLocked("Research")
		output.writeDetailsLocked(
			"state: prepared",
			fmt.Sprintf("evidence items: %d", event.EvidenceCount),
			fmt.Sprintf("locally inspected files: %d", event.FileCount),
		)
	case orient.ProgressResearchDone:
		state := singleRunOutputLine(event.Activity)
		if state == "" {
			state = "complete"
		}
		if event.Cached {
			state = "cached"
		}
		output.stageLocked("Research")
		output.writeDetailsLocked(
			"state: "+state,
			fmt.Sprintf("request bytes: %d", event.RequestBytes),
			fmt.Sprintf("response bytes: %d", event.ResponseBytes),
			fmt.Sprintf("validated findings: %d", event.FindingCount),
			fmt.Sprintf("rejected findings: %d", event.RejectedCount),
			fmt.Sprintf("new grounded facts: %d", event.NewFactCount),
			formatRunOutputTokens(event.InputTokens, event.OutputTokens),
			formatRunOutputDuration(event.LatencyMillis),
		)
	}
}

func (output *runOutput) surfacePhaseLocked(event orient.ProgressEvent) {
	key := "surface/" + event.Phase
	switch event.PhaseState {
	case "started":
		output.lastProgress[key] = runOutputProgress{at: output.now()}
		output.stageLocked("Runtime surfaces")
		output.writeDetailsLocked(singleRunOutputLine(event.Activity))
	case "completed":
		delete(output.lastProgress, key)
		output.stageLocked("Runtime surfaces")
		output.writeDetailsLocked(
			fmt.Sprintf("%s: complete", singleRunOutputLine(event.Phase)),
			formatRunOutputCount(event.CompletedCount, event.TotalCount),
			formatRunOutputDuration(event.LatencyMillis),
		)
	default:
		if !output.allowProgressLocked(
			key,
			event.CompletedCount,
			runOutputPhaseInterval,
			event.TotalCount > 0 && event.CompletedCount >= event.TotalCount,
		) {
			return
		}
		output.stageLocked("Runtime surfaces")
		output.writeDetailsLocked(
			singleRunOutputLine(event.Phase)+": "+formatRunOutputCount(event.CompletedCount, event.TotalCount),
			formatRunOutputElapsed(event.LatencyMillis),
		)
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

func formatRunOutputElapsed(milliseconds int64) string {
	return "elapsed: " + (time.Duration(milliseconds) * time.Millisecond).Round(time.Second).String()
}

func formatRunOutputTokens(input, output int) string {
	if input == 0 && output == 0 {
		return "tokens: unavailable"
	}
	return fmt.Sprintf("input tokens: %d\noutput tokens: %d", input, output)
}
