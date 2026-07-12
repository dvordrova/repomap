package orient

// ProgressStage identifies a stable, user-visible step in the orientation
// pipeline. Callers may ignore stages they do not want to present.
type ProgressStage string

const (
	ProgressSnapshotStarted ProgressStage = "snapshot_started"
	ProgressSnapshotReady   ProgressStage = "snapshot_ready"
	ProgressBundleReady     ProgressStage = "bundle_ready"
	ProgressSurfaceStarted  ProgressStage = "surface_started"
	ProgressSurfaceReady    ProgressStage = "surface_ready"
	ProgressSurfaceFailed   ProgressStage = "surface_failed"
	ProgressModelRequest    ProgressStage = "model_request"
	ProgressOrientationDone ProgressStage = "orientation_done"
)

// ProgressEvent contains bounded metadata about work in progress. It never
// contains repository contents, prompts, credentials, or provider responses.
type ProgressEvent struct {
	Stage          ProgressStage
	RepoPath       string
	RepoName       string
	Model          string
	BundleBytes    int
	RequestBytes   int
	CandidateCount int
	SurfaceCount   int
	Warning        string
	LatencyMillis  int64
}

func emitProgress(opts Options, event ProgressEvent) {
	if opts.Progress != nil {
		opts.Progress(event)
	}
}
