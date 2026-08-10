package orient

import (
	"context"
	"sync"
	"time"
)

// ProgressStage identifies a stable, user-visible step in the orientation
// pipeline. Callers may ignore stages they do not want to present.
type ProgressStage string

const (
	ProgressSnapshotStarted  ProgressStage = "snapshot_started"
	ProgressSnapshotReady    ProgressStage = "snapshot_ready"
	ProgressBundleReady      ProgressStage = "bundle_ready"
	ProgressSurfaceStarted   ProgressStage = "surface_started"
	ProgressSurfaceReady     ProgressStage = "surface_ready"
	ProgressSurfaceFailed    ProgressStage = "surface_failed"
	ProgressSurfaceWaiting   ProgressStage = "surface_waiting"
	ProgressSurfacePhase     ProgressStage = "surface_phase"
	ProgressModelRequest     ProgressStage = "model_request"
	ProgressProviderWaiting  ProgressStage = "provider_waiting"
	ProgressOrientationDone  ProgressStage = "orientation_done"
	ProgressPlanningWaiting  ProgressStage = "planning_waiting"
	ProgressResearchPrepared ProgressStage = "research_prepared"
	ProgressResearchDone     ProgressStage = "research_done"
)

// ProgressEvent contains bounded metadata about work in progress. It never
// contains repository contents, prompts, credentials, or provider responses.
type ProgressEvent struct {
	Stage                 ProgressStage
	RepoPath              string
	RepoName              string
	Model                 string
	Activity              string
	BundleBytes           int
	RequestBytes          int
	ResponseBytes         int
	CandidateCount        int
	FileCount             int
	EvidenceCount         int
	FindingCount          int
	RejectedCount         int
	NewFactCount          int
	InputTokens           int
	OutputTokens          int
	Cached                bool
	SurfaceCount          int
	Phase                 string
	PhaseState            string
	CompletedCount        int
	TotalCount            int
	Warning               string
	GoTarget              string
	GoTargetProvenance    string
	SuggestedGoTarget     string
	GoTargetEvidenceCount int
	GoTargetEvidencePaths []string
	LatencyMillis         int64
}

func emitProgress(opts Options, event ProgressEvent) {
	if opts.Progress != nil {
		opts.Progress(event)
	}
}

func startProgressHeartbeat(ctx context.Context, opts Options, event ProgressEvent) func() {
	if opts.Progress == nil {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	var wait sync.WaitGroup
	wait.Add(1)
	started := time.Now()
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				event.LatencyMillis = time.Since(started).Milliseconds()
				emitProgress(opts, event)
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
		wait.Wait()
	}
}
