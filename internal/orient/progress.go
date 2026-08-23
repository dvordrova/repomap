package orient

// ProgressStage identifies a stable, user-visible step in the orientation
// pipeline. Callers may ignore stages they do not want to present.
type ProgressStage string

const (
	ProgressSnapshotStarted ProgressStage = "snapshot_started"
	ProgressSnapshotReady   ProgressStage = "snapshot_ready"
	ProgressProgramStarted  ProgressStage = "program_started"
	ProgressProgramReady    ProgressStage = "program_ready"
	ProgressProgramFailed   ProgressStage = "program_failed"
	ProgressProgramPhase    ProgressStage = "program_phase"
)

// ProgressEvent contains bounded metadata about work in progress. It never
// contains repository contents, prompts, credentials, or provider responses.
type ProgressEvent struct {
	Stage                 ProgressStage
	RepoPath              string
	RepoName              string
	Activity              string
	FileCount             int
	GraphNodeCount        int
	GraphEdgeCount        int
	ExternalCallFamilies  int
	ActivityCandidates    int
	CoreDeclarations      int
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
