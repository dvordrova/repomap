package orient

import (
	"context"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

// Options contains only the deterministic repository-orientation inputs used
// by the ordinary repomap command. Exact producer snapshots are handed to the
// language-neutral ProgramIndex path after this package returns.
type Options struct {
	RepoPath string
	GoTarget string
	// BuildTags is the canonical run-wide Go build selection shared by initial
	// fact extraction and the later packages/types/SSA projection.
	BuildTags []string
	// RepositoryCorpus is built once by ordinary main and shared by snapshot
	// extraction and the parallel initial target-discovery cubes.
	RepositoryCorpus *corpus.Corpus
	// AutoGoTarget authorizes the snapshot's bounded platform preflight only
	// when the caller has no explicit CLI or Go environment target authority.
	AutoGoTarget bool

	DebugDir         string
	RunID            string
	DumpRedacted     bool
	RequireArtifacts bool

	// SkipGoFacts keeps this package a language-neutral artifact shell for a
	// page whose adapter has already crossed the ProgramIndex seam.
	SkipGoFacts bool

	Progress         func(ProgressEvent)
	EffectiveOptions debugdump.EffectiveOptions
}

// Run extracts one deterministic repository snapshot, discovers exact program
// facts for an already scoped target, and persists canonical artifacts.
func Run(ctx context.Context, opts Options) error {
	if opts.RequireArtifacts && opts.DebugDir == "" {
		return fmt.Errorf("required browser artifacts need a debug directory")
	}
	buildTags, err := gotarget.CanonicalBuildTags(opts.BuildTags)
	if err != nil {
		return fmt.Errorf("orientation build tags: %w", err)
	}
	opts.BuildTags = buildTags
	opts.EffectiveOptions.BuildTags = append([]string(nil), buildTags...)

	snapshotStarted := time.Now()
	emitProgress(opts, ProgressEvent{
		Stage:    ProgressSnapshotStarted,
		RepoPath: opts.RepoPath,
		GoTarget: opts.GoTarget,
	})

	repositorySnapshot, err := snapshot.BuildContext(ctx, snapshot.Options{
		RepoPath:         opts.RepoPath,
		GoTarget:         opts.GoTarget,
		BuildTags:        opts.BuildTags,
		RepositoryCorpus: opts.RepositoryCorpus,
		AutoGoTarget:     opts.AutoGoTarget,
		SkipGoFacts:      opts.SkipGoFacts,
	})
	if err != nil {
		return err
	}

	if repositorySnapshot.GoTargetSelection != nil {
		selection := *repositorySnapshot.GoTargetSelection
		if err := selection.ValidateAgainstAdvisory(repositorySnapshot.GoTargetAdvisory); err != nil {
			return fmt.Errorf("apply automatic Go target selection: %w", err)
		}
		if opts.GoTarget != selection.Baseline {
			return fmt.Errorf(
				"apply automatic Go target selection: baseline %s does not match resolved target %s",
				selection.Baseline,
				opts.GoTarget,
			)
		}
		selected, err := gotarget.Parse(selection.Target)
		if err != nil {
			return fmt.Errorf("apply automatic Go target selection: %w", err)
		}
		opts.GoTarget = selected.String()
		opts.EffectiveOptions.GoTarget = selected.String()
		opts.EffectiveOptions.GoTargetSource = selection.Source
		opts.EffectiveOptions.GoTargetBaseline = selection.Baseline
	}

	snapshotReady := ProgressEvent{
		Stage:         ProgressSnapshotReady,
		RepoName:      repositorySnapshot.RepoName,
		FileCount:     repositorySnapshot.FilesConsidered,
		GoTarget:      opts.GoTarget,
		LatencyMillis: time.Since(snapshotStarted).Milliseconds(),
	}
	if repositorySnapshot.GoTargetSelection != nil {
		snapshotReady.GoTargetProvenance = repositorySnapshot.GoTargetSelection.Display()
	}
	if repositorySnapshot.GoTargetAdvisory != nil {
		snapshotReady.SuggestedGoTarget = repositorySnapshot.GoTargetAdvisory.Suggested
		snapshotReady.GoTargetEvidenceCount = repositorySnapshot.GoTargetAdvisory.EvidenceFiles
		snapshotReady.GoTargetEvidencePaths = append(
			[]string(nil),
			repositorySnapshot.GoTargetAdvisory.Examples...,
		)
	}
	emitProgress(opts, snapshotReady)

	snapshotJSON, err := repositorySnapshot.JSON()
	if err != nil {
		return fmt.Errorf("encode repository snapshot: %w", err)
	}
	return persistArtifacts(ctx, opts, repositorySnapshot, snapshotJSON)
}

func persistArtifacts(
	ctx context.Context,
	opts Options,
	repositorySnapshot snapshot.Snapshot,
	snapshotJSON []byte,
) error {
	runID := opts.RunID
	if runID == "" {
		runID = debugdump.GenerateRunID(repositorySnapshot.RepoName)
	}
	runMeta := debugdump.RunMeta{
		RunID:            runID,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		RepoName:         repositorySnapshot.RepoName,
		RepoPath:         opts.RepoPath,
		Command:          "repomap",
		EffectiveOptions: opts.EffectiveOptions,
	}

	var writer *debugdump.Writer
	if opts.DebugDir != "" {
		var err error
		writer, err = debugdump.NewWriter(opts.DebugDir, runID, opts.DumpRedacted)
		if err != nil {
			if opts.RequireArtifacts {
				return fmt.Errorf("create required debug writer: %w", err)
			}
			return nil
		}
		defer writer.Close()
		if err := writer.WriteMetadata(runMeta); err != nil && opts.RequireArtifacts {
			return fmt.Errorf("write required debug metadata: %w", err)
		}
		if err := writer.WriteSnapshot(snapshotJSON); err != nil && opts.RequireArtifacts {
			return fmt.Errorf("write required debug snapshot: %w", err)
		}
	}

	return nil
}
