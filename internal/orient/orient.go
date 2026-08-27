package orient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// Options contains only the deterministic repository-orientation inputs used
// by the ordinary repomap command. Exact producer snapshots are handed to the
// language-neutral ProgramIndex path after this package returns.
type Options struct {
	RepoPath string
	GoTarget string
	// GoModuleDir is present only for an exact typed --target key and narrows
	// deterministic fact loading before the complete key is resolved locally.
	GoModuleDir string
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

	AnalyzeGoProgram bool
	// SkipGoFacts is set only by the ordinary Python-only route. It prevents
	// incidental non-module Go files from activating the Go adapter.
	SkipGoFacts         bool
	DirectCallDepth     int
	DirectCallEdgeLimit int

	// PrecomputedSnapshot is an independently validated target projection from
	// a TargetRunContainer. It avoids rescanning the repository for sibling
	// target pages while preserving the ordinary ProgramIndex pipeline.
	PrecomputedSnapshot *snapshot.Snapshot
	// PreparedGoWorkspace is the live packages/types/SSA authority prepared by
	// the default target run. Sibling target pages must project from this exact
	// object rather than repeat package loading or SSA construction.
	PreparedGoWorkspace *surfacediscovery.PreparedWorkspace
	// PreparedGoSnapshots is present only when an outer language-neutral target
	// plan starts with a precomputed scoped Go page before any Go workspace
	// exists. The first Go page prepares one union workspace from these exact
	// selected Go projections; later pages receive that workspace through
	// PreparedGoWorkspace. It is live-run-only and never persisted.
	PreparedGoSnapshots []snapshot.Snapshot
	// PreparedGoWorkspaceSink hands the default run's live workspace to ordinary
	// portfolio orchestration. The value is intentionally not serializable.
	PreparedGoWorkspaceSink func(*surfacediscovery.PreparedWorkspace)
	// PreparedGoWorkspaceUnionFailureSink reports that preparation of a shared
	// selected-target union failed before any workspace became reusable. The
	// current exact target is retried locally; the outer dispatcher uses this
	// signal only to stop offering the same poisoned union to later targets.
	PreparedGoWorkspaceUnionFailureSink func(error)

	// DirectCallIndexSink receives an independently owned snapshot produced by
	// the successful surface SSA pass. It is a live-run handoff and is not
	// serialized as a snapshot or report artifact.
	DirectCallIndexSink func(surfacediscovery.DirectCallIndex)
	// DependencyCatalogSink receives the target-scoped, language-neutral
	// dependency authority produced by the ordinary snapshot load.
	DependencyCatalogSink func(dependencies.Catalog)
	// EntryCallSubstrateSink receives the independently owned exact substrate
	// produced by ordinary generic Go call discovery.
	EntryCallSubstrateSink func(entrycall.Substrate)
	// ExternalCallIndexSink receives the independently owned, root-independent
	// exact external-call facts produced by the ordinary Go package load. The
	// index intentionally carries no integration or importance classification.
	ExternalCallIndexSink func(surfacediscovery.ExternalCallIndex)
	// CoreObjectIndexSink receives the independently owned target-scoped Go
	// declaration index produced from the ordinary package/type/SSA lifetime.
	CoreObjectIndexSink func(gocoreobject.Index)
	// DynamicHandoffIndexSink receives exact SSA structural joints that the
	// direct-call graph deliberately cannot claim as one static call.
	DynamicHandoffIndexSink func(godynamichandoff.Index)
	// AnalysisTargetSink receives the resolved target before Go program analysis.
	AnalysisTargetSink func(analysistarget.Target)
	// TargetRunContainerSink receives the complete selected-target authority.
	TargetRunContainerSink func(snapshot.TargetRunContainer)
	// GoTargetSelectionSink receives the final automatic Go platform selection
	// before target selection or any later provider call.
	GoTargetSelectionSink func(snapshot.GoTargetSelection)
	// AnalysisTargetSelector receives the complete deterministic target catalog
	// and chooses the default target plus the complete set to publish.
	AnalysisTargetSelector func(
		context.Context,
		string,
		analysistarget.TargetCatalog,
		gofacts.Facts,
	) (snapshot.TargetRunSelection, error)

	Progress         func(ProgressEvent)
	EffectiveOptions debugdump.EffectiveOptions
}

// Run extracts one deterministic repository snapshot, resolves the selected
// target portfolio, discovers runtime surfaces, and persists the canonical
// snapshot and target-container artifacts.
func Run(ctx context.Context, opts Options) error {
	if opts.RequireArtifacts && opts.DebugDir == "" {
		return fmt.Errorf("required browser artifacts need a debug directory")
	}

	snapshotStarted := time.Now()
	emitProgress(opts, ProgressEvent{
		Stage:    ProgressSnapshotStarted,
		RepoPath: opts.RepoPath,
		GoTarget: opts.GoTarget,
	})

	var repositorySnapshot snapshot.Snapshot
	var err error
	if opts.PrecomputedSnapshot != nil {
		if opts.AnalysisTargetSelector != nil {
			return fmt.Errorf("precomputed target snapshot cannot be combined with target selection")
		}
		repositorySnapshot, err = snapshot.OwnSnapshot(*opts.PrecomputedSnapshot)
		if err != nil {
			return fmt.Errorf("own precomputed target snapshot: %w", err)
		}
		if repositorySnapshot.AnalysisTarget == nil || repositorySnapshot.TargetCatalog != nil {
			return fmt.Errorf("precomputed target snapshot must be an ordinary scoped target projection")
		}
	} else {
		repositorySnapshot, err = snapshot.BuildContext(ctx, snapshot.Options{
			RepoPath:         opts.RepoPath,
			GoTarget:         opts.GoTarget,
			GoModuleDir:      opts.GoModuleDir,
			RepositoryCorpus: opts.RepositoryCorpus,
			AutoGoTarget:     opts.AutoGoTarget,
			SkipGoFacts:      opts.SkipGoFacts,
		})
		if err != nil {
			return err
		}
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
		if opts.GoTargetSelectionSink != nil {
			opts.GoTargetSelectionSink(selection)
		}
	}

	var targetRunContainer *snapshot.TargetRunContainer
	if repositorySnapshot.TargetCatalog != nil {
		if opts.AnalysisTargetSelector == nil {
			return fmt.Errorf("exact Go target catalog requires the ordinary analysis target selector")
		}
		selection, err := opts.AnalysisTargetSelector(
			ctx,
			repositorySnapshot.RepoName,
			repositorySnapshot.TargetCatalog.Snapshot(),
			*repositorySnapshot.GoFacts,
		)
		if err != nil {
			return err
		}
		container, err := snapshot.BuildTargetRunContainer(repositorySnapshot, selection)
		if err != nil {
			return fmt.Errorf("build selected target run container: %w", err)
		}
		deliverTargetRunContainer(opts, container)
		targetRunContainer = &container
		repositorySnapshot, err = container.ScopedSnapshot(selection.DefaultTargetRef)
		if err != nil {
			return fmt.Errorf("apply selected analysis target: %w", err)
		}
	}

	deliverAnalysisTarget(opts, repositorySnapshot.AnalysisTarget)
	if err := deliverDependencyCatalog(opts, repositorySnapshot.GoFacts); err != nil {
		return err
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
	return persistArtifacts(ctx, opts, repositorySnapshot, snapshotJSON, targetRunContainer)
}

func persistArtifacts(
	ctx context.Context,
	opts Options,
	repositorySnapshot snapshot.Snapshot,
	snapshotJSON []byte,
	targetRunContainer *snapshot.TargetRunContainer,
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
	bindRunMetaAnalysisTarget(&runMeta, repositorySnapshot.AnalysisTarget)

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
		if targetRunContainer != nil {
			encoded, err := targetRunContainer.CanonicalJSON()
			if err != nil {
				return fmt.Errorf("encode target run container: %w", err)
			}
			if err := writer.WriteValidatedFile(
				snapshot.TargetRunContainerArtifactFilename,
				encoded,
				targetRunContainer.ValidateArtifact,
			); err != nil {
				return fmt.Errorf("write target run container: %w", err)
			}
		}
	}

	if opts.AnalyzeGoProgram && writer == nil {
		return fmt.Errorf("Go program analysis requires an artifact directory")
	}
	// A caller may request Go analysis for a repository that contains no Go
	// program at all. That is a legitimate empty language adapter outcome: the
	// ordinary coordinator will either run another language adapter or report
	// that no target exists. Once exact Go facts do exist, missing target/SSA
	// authority remains terminal below.
	if opts.AnalyzeGoProgram && repositorySnapshot.GoFacts != nil {
		if repositorySnapshot.GoFacts == nil {
			return fmt.Errorf("Go program analysis requires exact Go facts")
		}
		if repositorySnapshot.AnalysisTarget == nil {
			return fmt.Errorf("Go program analysis requires one exact analysis target")
		}
		started := time.Now()
		emitProgress(opts, ProgressEvent{
			Stage:    ProgressProgramStarted,
			RepoName: repositorySnapshot.RepoName,
		})
		surfaceOptions := surfacediscovery.DefaultOptions(opts.RepoPath, opts.GoTarget)
		if opts.DirectCallDepth > 0 {
			surfaceOptions.DirectCallDepth = opts.DirectCallDepth
		}
		if opts.DirectCallEdgeLimit > 0 {
			surfaceOptions.DirectCallEdgeLimit = opts.DirectCallEdgeLimit
		}
		surfaceOptions.CaptureEntryCallSubstrate = opts.EntryCallSubstrateSink != nil
		surfaceOptions.CaptureExternalCallIndex = opts.ExternalCallIndexSink != nil
		surfaceOptions.CaptureCoreObjectIndex = opts.CoreObjectIndexSink != nil
		surfaceOptions.CaptureDynamicHandoffIndex = opts.DynamicHandoffIndexSink != nil
		surfaceOptions.Progress = func(progress surfacediscovery.PhaseProgress) {
			emitProgress(opts, ProgressEvent{
				Stage:          ProgressProgramPhase,
				RepoName:       repositorySnapshot.RepoName,
				Phase:          progress.Phase,
				PhaseState:     progress.State,
				Activity:       progress.Detail,
				CompletedCount: progress.Completed,
				TotalCount:     progress.Total,
				LatencyMillis:  progress.ElapsedMillis,
			})
		}
		programInput := surfaceDiscoveryInput(
			repositorySnapshot.RepoName,
			repositorySnapshot.GoFacts,
			repositorySnapshot.AnalysisTarget,
		)
		workspace := opts.PreparedGoWorkspace
		var programErr error
		if workspace == nil {
			workspaceInputs := []surfacediscovery.Input{programInput}
			switch {
			case opts.PrecomputedSnapshot != nil && len(opts.PreparedGoSnapshots) == 0:
				return fmt.Errorf("precomputed Go target analysis requires the default run's prepared workspace or exact selected Go projections")
			case len(opts.PreparedGoSnapshots) > 0:
				workspaceInputs = make([]surfacediscovery.Input, 0, len(opts.PreparedGoSnapshots))
				seenTargets := make(map[string]struct{}, len(opts.PreparedGoSnapshots))
				currentIncluded := false
				for index := range opts.PreparedGoSnapshots {
					scoped, ownErr := snapshot.OwnSnapshot(opts.PreparedGoSnapshots[index])
					if ownErr != nil {
						return fmt.Errorf("prepare selected Go target workspace: own projection %d: %w", index, ownErr)
					}
					if scoped.AnalysisTarget == nil || scoped.TargetCatalog != nil || scoped.GoFacts == nil {
						return fmt.Errorf("prepare selected Go target workspace: projection %d is not an exact scoped Go target", index)
					}
					ref := scoped.AnalysisTarget.Ref
					if _, duplicate := seenTargets[ref]; duplicate {
						return fmt.Errorf("prepare selected Go target workspace: duplicate target projection %q", ref)
					}
					seenTargets[ref] = struct{}{}
					if repositorySnapshot.AnalysisTarget != nil && ref == repositorySnapshot.AnalysisTarget.Ref {
						currentIncluded = true
					}
					workspaceInputs = append(workspaceInputs, surfaceDiscoveryInput(
						scoped.RepoName, scoped.GoFacts, scoped.AnalysisTarget,
					))
				}
				if !currentIncluded {
					return fmt.Errorf("prepare selected Go target workspace: current target projection is absent")
				}
			case targetRunContainer != nil:
				workspaceInputs = make([]surfacediscovery.Input, 0, len(targetRunContainer.Targets))
				for _, projection := range targetRunContainer.Targets {
					scoped, scopeErr := targetRunContainer.ScopedSnapshot(projection.Target.Ref)
					if scopeErr != nil {
						return fmt.Errorf("prepare selected Go target workspace: %w", scopeErr)
					}
					workspaceInputs = append(workspaceInputs, surfaceDiscoveryInput(
						scoped.RepoName, scoped.GoFacts, scoped.AnalysisTarget,
					))
				}
			}
			workspace, programErr = surfacediscovery.PrepareWorkspace(ctx, surfaceOptions, workspaceInputs)
			shareableWorkspace := programErr == nil
			if programErr != nil && len(workspaceInputs) > 1 &&
				opts.PreparedGoWorkspaceUnionFailureSink != nil &&
				!errors.Is(programErr, context.Canceled) &&
				!errors.Is(programErr, context.DeadlineExceeded) {
				opts.PreparedGoWorkspaceUnionFailureSink(programErr)
				workspace, programErr = surfacediscovery.PrepareWorkspace(
					ctx, surfaceOptions, []surfacediscovery.Input{programInput},
				)
				shareableWorkspace = false
			}
			if programErr == nil && shareableWorkspace && opts.PreparedGoWorkspaceSink != nil {
				opts.PreparedGoWorkspaceSink(workspace)
			}
		}
		var result surfacediscovery.Result
		if programErr == nil {
			result, programErr = workspace.Analyze(ctx, surfaceOptions, programInput)
		}
		if errors.Is(programErr, context.Canceled) || errors.Is(programErr, context.DeadlineExceeded) {
			return programErr
		}
		if programErr == nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		latency := time.Since(started).Milliseconds()
		runMeta.GoProgramAnalysisRan = true
		runMeta.GoProgramAnalysisMillis = &latency
		if result.DirectCallIndex != nil {
			runMeta.GoProgramGraphNodes = len(result.DirectCallIndex.Nodes)
			runMeta.GoProgramGraphEdges = len(result.DirectCallIndex.Edges)
		}
		if programErr != nil {
			warning := formatGoProgramAnalysisError(programErr)
			runMeta.Warnings = append(runMeta.Warnings, warning)
			emitProgress(opts, ProgressEvent{
				Stage: ProgressProgramFailed, RepoName: repositorySnapshot.RepoName,
				Warning: warning, LatencyMillis: latency,
			})
			if err := writer.WriteMetadata(runMeta); err != nil && opts.RequireArtifacts {
				return errors.Join(programErr, fmt.Errorf("write failed Go program-analysis metadata: %w", err))
			}
			return programErr
		}
		deliverDirectCallIndex(opts, result.DirectCallIndex)
		deliverEntryCallSubstrate(opts, result.EntryCallSubstrate)
		deliverExternalCallIndex(opts, result.ExternalCallIndex)
		deliverCoreObjectIndex(opts, result.CoreObjectIndex)
		deliverDynamicHandoffIndex(opts, result.DynamicHandoffIndex)
		ready := ProgressEvent{
			Stage: ProgressProgramReady, RepoName: repositorySnapshot.RepoName,
			LatencyMillis: latency,
		}
		if result.DirectCallIndex != nil {
			ready.GraphNodeCount = len(result.DirectCallIndex.Nodes)
			ready.GraphEdgeCount = len(result.DirectCallIndex.Edges)
		}
		if result.ExternalCallIndex != nil {
			ready.ExternalCallFamilies = len(result.ExternalCallIndex.Families)
		}
		if result.EntryCallSubstrate != nil {
			ready.ActivityCandidates = len(result.EntryCallSubstrate.SurfaceCandidates)
		}
		if result.CoreObjectIndex != nil {
			ready.CoreDeclarations = len(result.CoreObjectIndex.Types) + len(result.CoreObjectIndex.Callables)
		}
		emitProgress(opts, ready)
		if err := writer.WriteMetadata(runMeta); err != nil && opts.RequireArtifacts {
			return fmt.Errorf("write Go program-analysis metadata: %w", err)
		}
	}

	return nil
}

func deliverDirectCallIndex(opts Options, index *surfacediscovery.DirectCallIndex) {
	if opts.DirectCallIndexSink != nil && index != nil {
		opts.DirectCallIndexSink(index.Snapshot())
	}
}

func deliverDependencyCatalog(opts Options, facts *gofacts.Facts) error {
	if opts.DependencyCatalogSink == nil || facts == nil || facts.Dependencies == nil {
		return nil
	}
	refs := make(map[string]struct{}, len(facts.Dependencies.Importers))
	for _, importer := range facts.Dependencies.Importers {
		refs[importer.Ref] = struct{}{}
	}
	catalog, err := facts.Dependencies.Subset(refs)
	if err != nil {
		return fmt.Errorf("deliver dependency catalog: %w", err)
	}
	opts.DependencyCatalogSink(catalog)
	return nil
}

func deliverAnalysisTarget(opts Options, target *analysistarget.Target) {
	if opts.AnalysisTargetSink != nil && target != nil {
		opts.AnalysisTargetSink(target.Snapshot())
	}
}

func deliverTargetRunContainer(opts Options, container snapshot.TargetRunContainer) {
	if opts.TargetRunContainerSink != nil {
		opts.TargetRunContainerSink(container.Snapshot())
	}
}

func deliverEntryCallSubstrate(opts Options, substrate *entrycall.Substrate) {
	if opts.EntryCallSubstrateSink != nil && substrate != nil {
		opts.EntryCallSubstrateSink(substrate.Snapshot())
	}
}

func deliverExternalCallIndex(opts Options, index *surfacediscovery.ExternalCallIndex) {
	if opts.ExternalCallIndexSink != nil && index != nil {
		opts.ExternalCallIndexSink(index.Snapshot())
	}
}

func deliverCoreObjectIndex(opts Options, index *gocoreobject.Index) {
	if opts.CoreObjectIndexSink != nil && index != nil {
		opts.CoreObjectIndexSink(index.Snapshot())
	}
}

func deliverDynamicHandoffIndex(opts Options, index *godynamichandoff.Index) {
	if opts.DynamicHandoffIndexSink != nil && index != nil {
		opts.DynamicHandoffIndexSink(index.Snapshot())
	}
}

func bindRunMetaAnalysisTarget(meta *debugdump.RunMeta, target *analysistarget.Target) {
	if meta == nil || target == nil {
		return
	}
	meta.AnalysisTargetRef = target.Ref
	meta.AnalysisTargetKind = string(target.Kind)
	meta.AnalysisTargetModule = target.ModulePath
	meta.AnalysisTargetDisplayPath = target.DisplayPath()
	meta.AnalysisTargetPackage = target.PackagePath
}

func formatGoProgramAnalysisError(err error) string {
	const maxRunes = 500
	message := strings.Join(strings.Fields(err.Error()), " ")
	message = strings.TrimPrefix(message, "surface discovery: ")
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "…"
	}
	return "Go program analysis failed: " + message
}
