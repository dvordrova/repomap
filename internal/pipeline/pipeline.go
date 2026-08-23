// Package pipeline owns the language-neutral semantic cube chain over one
// sealed ProgramIndex and its exact dependency, declaration, and README
// authorities. Language adapters prepare those authorities; this package runs
// the cubes, persists their validated artifacts, and returns their results.
package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

// ArtifactWriter is the narrow persistence authority required by the
// pipeline. The ordinary run passes its already-open, run-confined writer.
type ArtifactWriter interface {
	WriteValidatedFile(string, []byte, func([]byte) error) error
}

// Runtime is the one execution context shared by every cube in a run. The
// executor carries cache configuration and the request observer used by the
// semantic journal; stage binding remains owned by this pipeline.
type Runtime struct {
	Provider   llm.Provider
	Executor   llm.Executor
	Artifacts  ArtifactWriter
	Progress   func(ProgressEvent)
	Accounting func(AccountingEvent)
	// StopAfter is an ordinary-run inspection checkpoint. A configured stage
	// still executes, validates, and persists its canonical artifact before Run
	// returns successfully; later stages are not prepared or invoked.
	StopAfter Stage
}

// Authorities are exact inputs prepared by language and repository owners.
// The pipeline does not re-extract, repair, or promote any of them.
type Authorities struct {
	RepositoryName string
	Repository     *corpus.Corpus
	ProgramIndex   programindex.Index
	Dependencies   dependencies.Catalog
	// Declarations is nil when a language adapter has no distinct
	// package-manager declaration authority. Observed dependency imports remain
	// exact in Dependencies; the pipeline must not synthesize a declaration
	// artifact merely to satisfy orchestration.
	Declarations *dependencydeclaration.Result
	ReadmeRoles  readmetargetscout.Result
}

// Result is the complete language-neutral semantic authority produced by one
// successful pipeline run.
type Result struct {
	ActivityEntrypoints     activityentrypoint.Result
	IntegrationDependencies integrationdependency.Result
	IntegrationUsage        integrationusage.Result
	ActivityPaths           activitypath.Result
	CoreMap                 coremap.Result
	Accounting              []AccountingEvent
	// StoppedAfter is execution metadata, not semantic artifact authority.
	StoppedAfter Stage
}

// AccountingState is the provider-neutral outcome of one prepared semantic
// request. A cache hit performs no live provider call.
type AccountingState string

const (
	AccountingAccepted       AccountingState = "accepted"
	AccountingCacheHit       AccountingState = "cache_hit"
	AccountingRejected       AccountingState = "rejected"
	AccountingProviderFailed AccountingState = "provider_failed"
)

// AccountingEvent contains the complete non-payload measurements needed by
// ordinary-run metadata. Ordinal is scoped to Stage. Metrics retain the
// provider's token and latency report, while SemanticCalls distinguishes a
// current live call from measurements restored with a cache hit.
type AccountingEvent struct {
	Stage             string
	Ordinal           int
	State             AccountingState
	RequestBytes      int
	SemanticCalls     int
	TransportAttempts int
	Metrics           llm.Metrics
}

// Stage is the closed, presentation-neutral pipeline stage identity.
type Stage string

const (
	StageActivityEntrypoints     Stage = "activity_entrypoints"
	StageIntegrationDependencies Stage = "integration_dependencies"
	StageIntegrationUsage        Stage = "integration_usage"
	StageActivityPaths           Stage = "activity_paths"
	StageCoreMap                 Stage = "core_map"
)

// ProgressState distinguishes a stage announcement from a fully persisted
// result. Progress is observational and cannot alter semantic execution.
type ProgressState string

const (
	ProgressStarted ProgressState = "started"
	ProgressReady   ProgressState = "ready"
)

// ProgressEvent gives the command layer neutral stage identity, elapsed time,
// artifact identity, and the already-produced result set. Consumers should
// inspect only the result corresponding to Stage when State is ProgressReady.
type ProgressEvent struct {
	Stage            Stage
	State            ProgressState
	Elapsed          time.Duration
	ArtifactFilename string
	Result           Result
}

// Run executes ActivityEntrypoint -> IntegrationDependency ->
// IntegrationUsage -> ActivityPath -> CoreMap. Every stage either returns and
// persists one fully validated result or terminates the chain with an error.
func Run(ctx context.Context, runtime Runtime, authorities Authorities) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("semantic pipeline: context is required")
	}
	if runtime.Provider == nil {
		return Result{}, fmt.Errorf("semantic pipeline: model provider is unavailable")
	}
	if runtime.Artifacts == nil {
		return Result{}, fmt.Errorf("semantic pipeline: artifact writer is unavailable")
	}
	if runtime.StopAfter != "" && runtime.StopAfter != StageActivityEntrypoints {
		return Result{}, fmt.Errorf(
			"semantic pipeline: unsupported stop checkpoint %q", runtime.StopAfter,
		)
	}
	if authorities.Repository == nil {
		return Result{}, fmt.Errorf("semantic pipeline: repository corpus is unavailable")
	}
	if err := authorities.ProgramIndex.Validate(); err != nil {
		return Result{}, fmt.Errorf("semantic pipeline: ProgramIndex authority: %w", err)
	}
	if err := authorities.Dependencies.Validate(); err != nil {
		return Result{}, fmt.Errorf("semantic pipeline: dependency authority: %w", err)
	}
	if authorities.Dependencies.Coverage.State != dependencies.CoverageComplete {
		return Result{}, fmt.Errorf("semantic pipeline: dependency authority is incomplete")
	}
	if authorities.Declarations != nil {
		if err := authorities.Declarations.ValidateAgainst(
			authorities.Repository.Snapshot(), authorities.ProgramIndex,
		); err != nil {
			return Result{}, fmt.Errorf("semantic pipeline: declaration authority: %w", err)
		}
	}
	accounting := newAccountingObserver(runtime.Executor.Observer, runtime.Accounting)
	runtime.Executor.Observer = accounting

	var result Result
	var err error

	started := beginStage(runtime.Progress, StageActivityEntrypoints, result)
	result.ActivityEntrypoints, err = runActivityEntrypoints(ctx, runtime, authorities.ProgramIndex)
	if err != nil {
		return Result{}, err
	}
	result.Accounting = accounting.snapshot()
	completeStage(runtime.Progress, StageActivityEntrypoints, started, activityentrypoint.ArtifactFilename, result)
	if runtime.StopAfter == StageActivityEntrypoints {
		result.StoppedAfter = StageActivityEntrypoints
		return result, nil
	}

	started = beginStage(runtime.Progress, StageIntegrationDependencies, result)
	result.IntegrationDependencies, err = runIntegrationDependencies(
		ctx, runtime, authorities.Dependencies, authorities.Declarations, authorities.ProgramIndex.Target,
	)
	if err != nil {
		return Result{}, err
	}
	result.Accounting = accounting.snapshot()
	completeStage(runtime.Progress, StageIntegrationDependencies, started, integrationdependency.ArtifactFilename, result)

	started = beginStage(runtime.Progress, StageIntegrationUsage, result)
	result.IntegrationUsage, err = runIntegrationUsage(
		ctx, runtime, authorities.ProgramIndex, result.IntegrationDependencies,
	)
	if err != nil {
		return Result{}, err
	}
	result.Accounting = accounting.snapshot()
	completeStage(runtime.Progress, StageIntegrationUsage, started, integrationusage.ArtifactFilename, result)

	started = beginStage(runtime.Progress, StageActivityPaths, result)
	result.ActivityPaths, err = runActivityPaths(
		runtime, authorities.ProgramIndex, result.ActivityEntrypoints,
		result.IntegrationDependencies, result.IntegrationUsage,
	)
	if err != nil {
		return Result{}, err
	}
	result.Accounting = accounting.snapshot()
	completeStage(runtime.Progress, StageActivityPaths, started, activitypath.ArtifactFilename, result)

	started = beginStage(runtime.Progress, StageCoreMap, result)
	result.CoreMap, err = runCoreMap(ctx, runtime, authorities, result.IntegrationDependencies, result.IntegrationUsage)
	if err != nil {
		return Result{}, err
	}
	result.Accounting = accounting.snapshot()
	completeStage(runtime.Progress, StageCoreMap, started, coremap.ArtifactFilename, result)

	return result, nil
}

func runActivityEntrypoints(
	ctx context.Context,
	runtime Runtime,
	index programindex.Index,
) (activityentrypoint.Result, error) {
	executor := debugdump.BindStage(runtime.Executor, debugdump.SemanticStageActivityEntrypoints)
	result, err := activityentrypoint.Run(ctx, executor, runtime.Provider, index)
	if err != nil {
		return activityentrypoint.Result{}, fmt.Errorf("semantic pipeline: activity entrypoints: %w", err)
	}
	if err := result.ValidateAgainst(index); err != nil {
		return activityentrypoint.Result{}, fmt.Errorf(
			"semantic pipeline: activity entrypoints: validate result authority: %w", err,
		)
	}
	encoded, err := activityentrypoint.Encode(result)
	if err != nil {
		return activityentrypoint.Result{}, fmt.Errorf("semantic pipeline: activity entrypoints: encode result: %w", err)
	}
	wantArtifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return activityentrypoint.Result{}, fmt.Errorf("semantic pipeline: activity entrypoints: digest result: %w", err)
	}
	if err := runtime.Artifacts.WriteValidatedFile(
		activityentrypoint.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := activityentrypoint.Decode(saved, index)
			if decodeErr != nil {
				return decodeErr
			}
			if validateErr := decoded.ValidateAgainst(index); validateErr != nil {
				return validateErr
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("semantic pipeline: activity entrypoints: persisted authority mismatch")
			}
			savedSHA256, digestErr := decoded.ArtifactSHA256()
			if digestErr != nil {
				return digestErr
			}
			if savedSHA256 != wantArtifactSHA256 {
				return fmt.Errorf("semantic pipeline: activity entrypoints: persisted artifact digest mismatch")
			}
			return nil
		},
	); err != nil {
		return activityentrypoint.Result{}, fmt.Errorf("semantic pipeline: activity entrypoints: persist result: %w", err)
	}
	return result, nil
}

func runIntegrationDependencies(
	ctx context.Context,
	runtime Runtime,
	catalog dependencies.Catalog,
	declarations *dependencydeclaration.Result,
	target programindex.Target,
) (integrationdependency.Result, error) {
	executor := debugdump.BindStage(runtime.Executor, debugdump.SemanticStageIntegrationDependencies)
	var result integrationdependency.Result
	var err error
	if declarations == nil {
		result, err = integrationdependency.Run(ctx, executor, runtime.Provider, catalog)
	} else {
		result, err = integrationdependency.RunWithDeclarations(
			ctx, executor, runtime.Provider, catalog, *declarations, target,
		)
	}
	if err != nil {
		return integrationdependency.Result{}, fmt.Errorf("semantic pipeline: integration dependencies: %w", err)
	}
	if declarations == nil {
		if err := result.ValidateAgainst(catalog); err != nil {
			return integrationdependency.Result{}, fmt.Errorf(
				"semantic pipeline: integration dependencies: validate result authority: %w", err,
			)
		}
	} else if err := result.ValidateAgainstDeclarations(catalog, *declarations, target); err != nil {
		return integrationdependency.Result{}, fmt.Errorf(
			"semantic pipeline: integration dependencies: validate result authority: %w", err,
		)
	}
	encoded, err := integrationdependency.Encode(result)
	if err != nil {
		return integrationdependency.Result{}, fmt.Errorf("semantic pipeline: integration dependencies: encode result: %w", err)
	}
	wantArtifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return integrationdependency.Result{}, fmt.Errorf("semantic pipeline: integration dependencies: digest result: %w", err)
	}
	if err := runtime.Artifacts.WriteValidatedFile(
		integrationdependency.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := integrationdependency.Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if declarations == nil {
				if validateErr := decoded.ValidateAgainst(catalog); validateErr != nil {
					return validateErr
				}
			} else if validateErr := decoded.ValidateAgainstDeclarations(catalog, *declarations, target); validateErr != nil {
				return validateErr
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("semantic pipeline: integration dependencies: persisted authority mismatch")
			}
			savedSHA256, digestErr := decoded.ArtifactSHA256()
			if digestErr != nil {
				return digestErr
			}
			if savedSHA256 != wantArtifactSHA256 {
				return fmt.Errorf("semantic pipeline: integration dependencies: persisted artifact digest mismatch")
			}
			return nil
		},
	); err != nil {
		return integrationdependency.Result{}, fmt.Errorf("semantic pipeline: integration dependencies: persist result: %w", err)
	}
	return result, nil
}

func runIntegrationUsage(
	ctx context.Context,
	runtime Runtime,
	index programindex.Index,
	selected integrationdependency.Result,
) (integrationusage.Result, error) {
	executor := debugdump.BindStage(runtime.Executor, debugdump.SemanticStageIntegrationUsage)
	result, err := integrationusage.Run(ctx, executor, runtime.Provider, index, selected)
	if err != nil {
		return integrationusage.Result{}, fmt.Errorf("semantic pipeline: integration usage: %w", err)
	}
	if err := result.ValidateAgainst(index, selected); err != nil {
		return integrationusage.Result{}, fmt.Errorf(
			"semantic pipeline: integration usage: validate result authority: %w", err,
		)
	}
	encoded, err := integrationusage.Encode(result)
	if err != nil {
		return integrationusage.Result{}, fmt.Errorf("semantic pipeline: integration usage: encode result: %w", err)
	}
	wantArtifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return integrationusage.Result{}, fmt.Errorf("semantic pipeline: integration usage: digest result: %w", err)
	}
	if err := runtime.Artifacts.WriteValidatedFile(
		integrationusage.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := integrationusage.Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if validateErr := decoded.ValidateAgainst(index, selected); validateErr != nil {
				return validateErr
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("semantic pipeline: integration usage: persisted authority mismatch")
			}
			savedSHA256, digestErr := decoded.ArtifactSHA256()
			if digestErr != nil {
				return digestErr
			}
			if savedSHA256 != wantArtifactSHA256 {
				return fmt.Errorf("semantic pipeline: integration usage: persisted artifact digest mismatch")
			}
			return nil
		},
	); err != nil {
		return integrationusage.Result{}, fmt.Errorf("semantic pipeline: integration usage: persist result: %w", err)
	}
	return result, nil
}

func runActivityPaths(
	runtime Runtime,
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	uses integrationusage.Result,
) (activitypath.Result, error) {
	result, err := activitypath.Build(index, activities, integrations, uses)
	if err != nil {
		return activitypath.Result{}, fmt.Errorf("semantic pipeline: activity paths: %w", err)
	}
	if err := result.ValidateAgainst(index, activities, integrations, uses); err != nil {
		return activitypath.Result{}, fmt.Errorf(
			"semantic pipeline: activity paths: validate result authority: %w", err,
		)
	}
	encoded, err := activitypath.Encode(result)
	if err != nil {
		return activitypath.Result{}, fmt.Errorf("semantic pipeline: activity paths: encode result: %w", err)
	}
	wantArtifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return activitypath.Result{}, fmt.Errorf("semantic pipeline: activity paths: digest result: %w", err)
	}
	if err := runtime.Artifacts.WriteValidatedFile(
		activitypath.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := activitypath.Decode(saved, index, activities, integrations, uses)
			if decodeErr != nil {
				return decodeErr
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("semantic pipeline: activity paths: persisted authority mismatch")
			}
			savedSHA256, digestErr := decoded.ArtifactSHA256()
			if digestErr != nil {
				return digestErr
			}
			if savedSHA256 != wantArtifactSHA256 {
				return fmt.Errorf("semantic pipeline: activity paths: persisted artifact digest mismatch")
			}
			return nil
		},
	); err != nil {
		return activitypath.Result{}, fmt.Errorf("semantic pipeline: activity paths: persist result: %w", err)
	}
	return result, nil
}

func runCoreMap(
	ctx context.Context,
	runtime Runtime,
	authorities Authorities,
	selected integrationdependency.Result,
	uses integrationusage.Result,
) (coremap.Result, error) {
	compilation, err := coremap.CompileProgramWithIntegrationUsage(
		authorities.RepositoryName,
		authorities.Repository,
		authorities.ProgramIndex,
		authorities.ReadmeRoles,
		selected,
		uses,
	)
	if err != nil {
		return coremap.Result{}, fmt.Errorf("semantic pipeline: core map: compile evidence: %w", err)
	}
	result, err := coremap.Run(
		ctx, executorForCoreMap(runtime.Executor), runtime.Provider, compilation,
	)
	if err != nil {
		return coremap.Result{}, fmt.Errorf("semantic pipeline: core map: %w", err)
	}
	if err := result.ValidateAgainst(compilation); err != nil {
		return coremap.Result{}, fmt.Errorf("semantic pipeline: core map: validate result authority: %w", err)
	}
	encoded, err := coremap.Encode(result)
	if err != nil {
		return coremap.Result{}, fmt.Errorf("semantic pipeline: core map: encode result: %w", err)
	}
	if err := runtime.Artifacts.WriteValidatedFile(
		coremap.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := coremap.Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if validateErr := decoded.ValidateAgainst(compilation); validateErr != nil {
				return validateErr
			}
			canonical, encodeErr := coremap.Encode(decoded)
			if encodeErr != nil {
				return encodeErr
			}
			if !bytes.Equal(canonical, saved) {
				return fmt.Errorf("semantic pipeline: core map: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return coremap.Result{}, fmt.Errorf("semantic pipeline: core map: persist result: %w", err)
	}
	return result, nil
}

func beginStage(progress func(ProgressEvent), stage Stage, result Result) time.Time {
	started := time.Now()
	if progress != nil {
		progress(ProgressEvent{Stage: stage, State: ProgressStarted, Result: result})
	}
	return started
}

func completeStage(
	progress func(ProgressEvent),
	stage Stage,
	started time.Time,
	artifactFilename string,
	result Result,
) {
	if progress == nil {
		return
	}
	progress(ProgressEvent{
		Stage: stage, State: ProgressReady, Elapsed: time.Since(started),
		ArtifactFilename: artifactFilename, Result: result,
	})
}

type diagnosticStageObserver interface {
	ObserveStage(string, llm.Event) error
}

type accountingObserver struct {
	forward  llm.Observer
	callback func(AccountingEvent)

	mu       sync.Mutex
	ordinals map[string]int
	events   []AccountingEvent
}

func newAccountingObserver(
	forward llm.Observer,
	callback func(AccountingEvent),
) *accountingObserver {
	return &accountingObserver{
		forward: forward, callback: callback,
		ordinals: make(map[string]int), events: []AccountingEvent{},
	}
}

func (observer *accountingObserver) Observe(event llm.Event) error {
	if observer == nil || observer.forward == nil {
		return nil
	}
	return observer.forward.Observe(event)
}

func (observer *accountingObserver) ObserveStage(stage string, event llm.Event) error {
	if observer == nil {
		return nil
	}
	if accounting, ok := accountingEventFor(stage, event); ok {
		observer.mu.Lock()
		accounting.Ordinal = observer.ordinals[stage] + 1
		observer.ordinals[stage] = accounting.Ordinal
		observer.events = append(observer.events, accounting)
		callback := observer.callback
		observer.mu.Unlock()
		if callback != nil {
			callback(accounting)
		}
	}
	if stages, ok := observer.forward.(diagnosticStageObserver); ok {
		return stages.ObserveStage(stage, event)
	}
	if observer.forward != nil {
		return observer.forward.Observe(event)
	}
	return nil
}

func (observer *accountingObserver) snapshot() []AccountingEvent {
	if observer == nil {
		return []AccountingEvent{}
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return append([]AccountingEvent(nil), observer.events...)
}

func accountingEventFor(stage string, event llm.Event) (AccountingEvent, bool) {
	if stage == "" {
		return AccountingEvent{}, false
	}
	result := AccountingEvent{
		Stage: stage, RequestBytes: event.RequestBytes, Metrics: event.Metrics,
	}
	switch event.Kind {
	case llm.EventLive:
		if event.Source != llm.SourceLive || event.Failure != llm.FailureNone {
			return AccountingEvent{}, false
		}
		result.State = AccountingAccepted
		result.SemanticCalls = 1
	case llm.EventCacheHit:
		if event.Source != llm.SourceCache || event.Failure != llm.FailureNone {
			return AccountingEvent{}, false
		}
		result.State = AccountingCacheHit
	case llm.EventFailure:
		if event.Source != llm.SourceLive {
			return AccountingEvent{}, false
		}
		switch event.Failure {
		case llm.FailureProvider:
			result.State = AccountingProviderFailed
			result.SemanticCalls = 1
		case llm.FailureResponse, llm.FailureValidation:
			result.State = AccountingRejected
			result.SemanticCalls = 1
		case llm.FailurePrepare:
			result.State = AccountingRejected
		default:
			return AccountingEvent{}, false
		}
	default:
		return AccountingEvent{}, false
	}
	if result.SemanticCalls > 0 {
		result.TransportAttempts = event.Metrics.Attempts
	}
	return result, true
}

type coreMapStageObserver struct {
	observer llm.Observer
	stages   diagnosticStageObserver
}

func (observer coreMapStageObserver) Observe(event llm.Event) error {
	return observer.observer.Observe(event)
}

func (observer coreMapStageObserver) ObserveCoreMap(stage coremap.Stage, event llm.Event) error {
	var diagnosticStage string
	switch stage {
	case coremap.StageBaseline:
		diagnosticStage = debugdump.SemanticStageCoreMapBaseline
	case coremap.StageRefined:
		diagnosticStage = debugdump.SemanticStageCoreMapRefined
	default:
		return nil
	}
	return observer.stages.ObserveStage(diagnosticStage, event)
}

func executorForCoreMap(executor llm.Executor) llm.Executor {
	if _, ok := executor.Observer.(coremap.StageObserver); ok {
		return executor
	}
	stages, ok := executor.Observer.(diagnosticStageObserver)
	if !ok || executor.Observer == nil {
		return executor
	}
	executor.Observer = coreMapStageObserver{observer: executor.Observer, stages: stages}
	return executor
}
