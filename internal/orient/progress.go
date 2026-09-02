package orient

// ProgressStage identifies a stable, user-visible step in the orientation
// pipeline. Callers may ignore stages they do not want to present.
type ProgressStage string

const (
	ProgressSnapshotStarted ProgressStage = "snapshot_started"
	ProgressSnapshotReady   ProgressStage = "snapshot_ready"
)

// ProgressEvent contains bounded metadata about work in progress. It never
// contains repository contents, prompts, credentials, or provider responses.
type ProgressEvent struct {
	Stage                 ProgressStage
	RepoPath              string
	RepoName              string
	FileCount             int
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
