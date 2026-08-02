package orient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/repositoryatlas/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

type Options struct {
	RepoPath                  string
	SnapshotOnly              bool
	LLMBundleOnly             bool
	LLMRequestOnly            bool
	OutputJSON                bool
	Offline                   bool
	NoCache                   bool
	FlowCount                 int
	FlowBundlesOnly           bool
	MaxReadmeBytes            int
	MaxReadmeLLMBytes         int
	MaxTreeLines              int
	MaxInterestingFiles       int
	MaxGoPkgs                 int
	MaxGoEdges                int
	MaxLLMEntrypoints         int
	MaxLLMModules             int
	MaxLLMFiles               int
	MaxOrientationBundleBytes int
	MaxLocalDirectionFiles    int
	MaxLLMEdges               int
	MaxLLMSignals             int
	MaxLLMSignalsPerFile      int
	DebugDir                  string
	RunID                     string
	DumpRedacted              bool
	RequireArtifacts          bool
	DiscoverSurfaces          bool
	ExplainFlows              int
	// Progress callbacks may run from heartbeat goroutines. They must be
	// concurrency-safe and return promptly.
	Progress          func(ProgressEvent)
	EffectiveOptions  debugdump.EffectiveOptions
	ResearchPolicy    modelresearch.Policy
	RepositoryContext modelresearch.RepositoryContext
}

type combinedReport struct {
	RepoName       string           `json:"repo_name"`
	Orientation    *orientationPart `json:"orientation,omitempty"`
	ExplainedFlows []explainedFlow  `json:"explained_flows"`
	Warnings       []string         `json:"warnings,omitempty"`
}

func Run(ctx context.Context, opts Options) ([]byte, error) {
	policy := opts.ResearchPolicy
	if policy.Version == "" {
		policy = modelresearch.DefaultPolicy()
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	requireArtifacts := opts.RequireArtifacts
	if opts.RequireArtifacts && !opts.SnapshotOnly && !opts.LLMBundleOnly && !opts.LLMRequestOnly && opts.DebugDir == "" {
		return nil, fmt.Errorf("required browser artifacts need a debug directory")
	}
	snapshotStarted := time.Now()
	emitProgress(opts, ProgressEvent{
		Stage:    ProgressSnapshotStarted,
		RepoPath: opts.RepoPath,
	})

	s, err := snapshot.Build(snapshot.Options{
		RepoPath:            opts.RepoPath,
		MaxReadmeBytes:      opts.MaxReadmeBytes,
		MaxTreeLines:        opts.MaxTreeLines,
		MaxInterestingFiles: opts.MaxInterestingFiles,
		MaxGoPkgs:           opts.MaxGoPkgs,
		MaxGoEdges:          opts.MaxGoEdges,
	})
	if err != nil {
		return nil, err
	}
	emitProgress(opts, ProgressEvent{
		Stage:         ProgressSnapshotReady,
		RepoName:      s.RepoName,
		FileCount:     s.FilesConsidered,
		LatencyMillis: time.Since(snapshotStarted).Milliseconds(),
	})

	snapshotJSON, _ := s.JSON()

	if opts.SnapshotOnly {
		if opts.OutputJSON || opts.SnapshotOnly {
			return append(snapshotJSON, '\n'), nil
		}
		return snapshotJSON, nil
	}
	orientationSignals, orientationSignalTrace := collectOrientationSignals(s, opts)
	operationalWarnings := discoverOperationalCandidates(&s, orientationSignals)
	snapshotJSON, _ = s.JSON()

	bundleStarted := time.Now()
	maxOrientationFiles := orientationFileLimit(opts.MaxLLMFiles, len(s.FilteredFiles))
	bundle, bundleSelectionTrace := llmbundle.BuildWithTrace(s, s.FilteredFiles, llmbundle.Options{
		MaxReadmeBytes:   opts.MaxReadmeLLMBytes,
		MaxModules:       opts.MaxLLMModules,
		MaxEntrypoints:   opts.MaxLLMEntrypoints,
		MaxFiles:         maxOrientationFiles,
		MaxEdges:         opts.MaxLLMEdges,
		MaxSignalTotal:   opts.MaxLLMSignals,
		MaxSignalPerFile: opts.MaxLLMSignalsPerFile,
		RepoPath:         opts.RepoPath,
		SourceSignals:    orientationSignals,
		MaxBytes:         opts.MaxOrientationBundleBytes,
		PolicyVersion:    policy.Version,
	})
	bundle.Warnings = append(bundle.Warnings, operationalWarnings...)
	bundleJSON, _ := json.MarshalIndent(bundle, "", "  ")
	modelBundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal compact model bundle: %w", err)
	}
	orientationCatalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		return nil, err
	}
	orientationWireJSON, err := buildOrientationWireBundle(bundle, orientationCatalog)
	if err != nil {
		return nil, err
	}
	emitProgress(opts, ProgressEvent{
		Stage:          ProgressBundleReady,
		RepoName:       s.RepoName,
		BundleBytes:    len(modelBundleJSON),
		CandidateCount: len(bundle.CandidateFileIndex),
		LatencyMillis:  time.Since(bundleStarted).Milliseconds(),
	})

	if opts.LLMBundleOnly {
		out := append(bundleJSON, '\n')
		return out, nil
	}
	if opts.LLMRequestOnly || !opts.Offline {
		if err := validateOrientationBundleForRemote(bundle); err != nil {
			return nil, err
		}
	}
	if opts.LLMRequestOnly {
		client, err := deepseek.NewPromptFromEnv()
		if err != nil {
			return nil, err
		}
		requestJSON, err := client.OrientPromptJSON(orientationWireJSON)
		if err != nil {
			return nil, err
		}
		if allowed, reason := policy.Allows(policy.Orientation, modelresearch.Usage{}, len(requestJSON)); !allowed {
			return nil, fmt.Errorf("orientation request rejected by %s: %d bytes", reason, len(requestJSON))
		}
		return requestJSON, nil
	}

	runID := opts.RunID
	if runID == "" {
		runID = debugdump.GenerateRunID(s.RepoName)
	}

	var dw *debugdump.Writer
	runMeta := debugdump.RunMeta{
		RunID:               runID,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		RepoName:            s.RepoName,
		RepoPath:            opts.RepoPath,
		Command:             "orient",
		CompactContextBytes: len(orientationWireJSON),
		LLMBundleOnly:       opts.LLMBundleOnly,
		EffectiveOptions:    opts.EffectiveOptions,
	}
	if opts.DebugDir != "" {
		dw, err = debugdump.NewWriter(opts.DebugDir, runID, opts.DumpRedacted)
		if err != nil {
			if requireArtifacts {
				return nil, fmt.Errorf("create required debug writer: %w", err)
			}
			dw = nil
		}
		if dw != nil {
			defer dw.Close()
			if err := dw.WriteMetadata(runMeta); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required debug metadata: %w", err)
			}
			if err := dw.WriteSnapshot(snapshotJSON); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required debug snapshot: %w", err)
			}
			if err := dw.WriteLLMBundleWithSidecar(
				modelBundleJSON,
				llmbundle.OrientationContextSelectionFilename,
				func(savedBundle []byte) ([]byte, error) {
					contextSelection, selectionErr := llmbundle.FinalizeOrientationContextSelection(
						bundleSelectionTrace,
						bundle,
						modelBundleJSON,
						savedBundle,
						orientationWireJSON,
						orientationSignalTrace,
					)
					if selectionErr != nil {
						return nil, selectionErr
					}
					return llmbundle.EncodeOrientationContextSelection(contextSelection)
				},
			); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required model bundle and orientation context selection: %w", err)
			}
		}
	}
	report := combinedReport{
		RepoName: s.RepoName,
		Warnings: append([]string(nil), operationalWarnings...),
	}
	var successfulSurfaceResult *surfacediscovery.Result
	if opts.DiscoverSurfaces && dw != nil && s.GoFacts != nil {
		surfaceStarted := time.Now()
		emitProgress(opts, ProgressEvent{
			Stage:    ProgressSurfaceStarted,
			RepoName: s.RepoName,
		})
		surfaceOptions := surfacediscovery.DefaultOptions(opts.RepoPath)
		surfaceOptions.Progress = func(progress surfacediscovery.PhaseProgress) {
			emitProgress(opts, ProgressEvent{
				Stage: ProgressSurfacePhase, RepoName: s.RepoName,
				Phase: progress.Phase, PhaseState: progress.State, Activity: progress.Detail,
				CompletedCount: progress.Completed, TotalCount: progress.Total,
				LatencyMillis: progress.ElapsedMillis,
			})
		}
		surfaceResult, surfaceErr := surfacediscovery.AnalyzeContextWithInput(
			ctx,
			surfaceOptions,
			surfaceDiscoveryInput(s.RepoName, s.GoFacts),
		)
		if errors.Is(surfaceErr, context.Canceled) || errors.Is(surfaceErr, context.DeadlineExceeded) {
			return nil, surfaceErr
		}
		if surfaceErr == nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			surfaceErr = surfacediscovery.WriteArtifacts(dw.RunDir(), surfaceResult)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		surfaceLatency := time.Since(surfaceStarted).Milliseconds()
		runMeta.SurfaceDiscoveryRan = true
		runMeta.SurfaceDiscoveryMillis = &surfaceLatency
		runMeta.SurfaceDiscoveryCount = len(surfaceResult.Catalog.Triggers)
		if surfaceErr != nil {
			warning := formatSurfaceDiscoveryWarning(surfaceErr)
			report.Warnings = append(report.Warnings, warning)
			runMeta.Warnings = append(runMeta.Warnings, warning)
			emitProgress(opts, ProgressEvent{
				Stage:         ProgressSurfaceFailed,
				RepoName:      s.RepoName,
				Warning:       warning,
				LatencyMillis: surfaceLatency,
			})
		} else {
			successfulSurfaceResult = &surfaceResult
			emitProgress(opts, ProgressEvent{
				Stage:         ProgressSurfaceReady,
				RepoName:      s.RepoName,
				SurfaceCount:  len(surfaceResult.Catalog.Triggers),
				LatencyMillis: surfaceLatency,
			})
		}
		if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
			return nil, fmt.Errorf("write surface discovery metadata: %w", metadataErr)
		}
	}
	if dw != nil && s.GoFacts != nil {
		catalog := surfacediscovery.TriggerCatalog{}
		if successfulSurfaceResult != nil {
			catalog = successfulSurfaceResult.Catalog
		}
		atlas, err := goadapter.Project(goadapter.Input{
			RepositoryName: s.RepoName,
			Facts:          *s.GoFacts,
			Catalog:        catalog,
		})
		if err != nil {
			return nil, fmt.Errorf("project repository Atlas: %w", err)
		}
		encoded, err := repositoryatlas.CanonicalJSON(atlas)
		if err != nil {
			return nil, fmt.Errorf("encode repository Atlas: %w", err)
		}
		if err := dw.WriteValidatedFile(
			repositoryatlas.ArtifactFilename,
			encoded,
			func(saved []byte) error {
				_, validateErr := repositoryatlas.DecodeCanonicalJSON(saved)
				return validateErr
			},
		); err != nil {
			return nil, fmt.Errorf("write repository Atlas: %w", err)
		}
	}

	flowCount := opts.FlowCount
	if opts.ExplainFlows > 0 {
		flowCount = opts.ExplainFlows
	}
	if flowCount < 0 {
		flowCount = 0
	}

	if opts.Offline {
		report.Warnings = append(report.Warnings, "offline mode: skipping all LLM calls")
		flows, err := buildFlowBundlesFromSnapshot(s, flowCount, dw, opts)
		if err != nil {
			return nil, err
		}
		report.ExplainedFlows = flows
		report.Warnings = append(report.Warnings, fmt.Sprintf("run %s to get LLM orientation", "repomap "+opts.RepoPath))
	} else {
		client, err := deepseek.NewFromEnv()
		if err != nil {
			if dw != nil {
				runMeta.RequestAttempts = append(runMeta.RequestAttempts, debugdump.RequestAttempt{
					Stage: "configuration", State: "failed",
				})
				_ = dw.WriteMetadata(runMeta)
				dw.WriteError(err)
			}
			return nil, fmt.Errorf(
				"configure REPOMAP_LLM_* for an OpenAI-compatible provider: %w\nOr run local facts only:\n  repomap %s --offline",
				err,
				opts.RepoPath,
			)
		}
		config := client.EffectiveConfig()
		client.OnWait = func(progress deepseek.WaitProgress) {
			emitProgress(opts, ProgressEvent{
				Stage: ProgressProviderWaiting, RepoName: s.RepoName, Model: client.Model,
				Activity: progress.Stage, LatencyMillis: progress.Elapsed.Milliseconds(),
			})
		}
		repository := repositoryContext(opts, modelBundleJSON)
		researchState := modelresearch.NewState(policy, repository)
		researchState.Coverage.LocalAuthorizedFiles = len(s.FilteredFiles)
		researchState.Coverage.InitialModelSummaries = len(bundle.CandidateFileIndex)
		if dw != nil {
			runMeta.Model = config.Model
			runMeta.Endpoint = config.Endpoint
			runMeta.AuthMode = config.AuthMode
			runMeta.TimeoutMillis = config.Timeout.Milliseconds()
			runMeta.MaxTokens = config.MaxTokens
			runMeta.PromptVersion = deepseek.OrientationPromptVersionJSON
			if err := dw.WriteMetadata(runMeta); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required provider metadata: %w", err)
			}
		}

		requestJSON, err := client.OrientPromptJSON(orientationWireJSON)
		if err != nil {
			if dw != nil {
				runMeta.RequestAttempts = append(runMeta.RequestAttempts, debugdump.RequestAttempt{
					Stage: "orientation", State: "request_build_failed",
				})
				_ = dw.WriteMetadata(runMeta)
				dw.WriteError(err)
			}
			return nil, err
		}
		if allowed, reason := policy.Allows(policy.Orientation, modelresearch.Usage{}, len(requestJSON)); !allowed {
			return nil, fmt.Errorf("orientation request rejected by %s: %d bytes", reason, len(requestJSON))
		}
		runMeta.ExternalRequestBytes = 0
		runMeta.ProviderRequestCount = 0
		runMeta.RequestAttempts = append(runMeta.RequestAttempts, debugdump.RequestAttempt{
			Stage: "orientation", State: "prepared", RequestBytes: len(requestJSON),
		})
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write request attempt metadata: %w", metadataErr)
			}
		}
		emitProgress(opts, ProgressEvent{
			Stage:        ProgressModelRequest,
			RepoName:     s.RepoName,
			Model:        client.Model,
			BundleBytes:  len(orientationWireJSON),
			RequestBytes: len(requestJSON),
		})

		prepareOrientation := func(raw []byte) (orientationPart, string, error) {
			if err := validateProviderOutputForStorage("orientation", raw); err != nil {
				return orientationPart{}, "response_rejected", err
			}
			or, err := parseAndResolveOrientationResponse(raw, orientationCatalog)
			if err != nil {
				return orientationPart{}, "response_parse_failed", err
			}
			mergeOperationalCandidateFlows(&or, bundle.Go.OrientationCandidates, bundle.SourceSignals)

			localProofInput := localFlowProofInput(s, successfulSurfaceResult)
			attachLocalFlowProofs(ctx, opts.RepoPath, &or, localProofInput)
			reconcileResolvedUnknownPaths(&or)
			applyOrientationConfidenceGate(&or, bundle)
			for index := range or.CandidateFlows {
				flowexplain.ClassifyCandidateFlow(&or.CandidateFlows[index])
			}
			if err := validateResolvedOrientation(or); err != nil {
				return orientationPart{}, "response_validation_failed", err
			}
			return or, "", nil
		}

		call, err := obtainOrientation(
			ctx, client, dw, policy, repository, "openai-compatible/"+client.Auth,
			orientationWireJSON, orientationCatalog.digest, requestJSON, !opts.NoCache,
			func(raw []byte) (orientationPart, error) {
				prepared, _, err := prepareOrientation(raw)
				return prepared, err
			},
		)
		raw := call.Raw
		providerLatency := call.Metrics.LatencyMillis
		researchState.Orientation = call.Metrics
		researchState.Usage.SemanticCalls += call.Metrics.SemanticCalls
		if call.Metrics.SemanticCalls > 0 {
			researchState.Usage.RequestBytes += call.Metrics.RequestBytes
		}
		runMeta.ExternalRequestBytes = researchState.Usage.RequestBytes
		runMeta.ProviderRequestCount = researchState.Usage.SemanticCalls
		runMeta.ProviderLatencyMillis = &providerLatency
		attempt := &runMeta.RequestAttempts[len(runMeta.RequestAttempts)-1]
		attempt.LatencyMillis = &providerLatency
		attempt.RequestBytes = call.Metrics.RequestBytes
		attempt.ProviderCallCount = call.Metrics.SemanticCalls
		if call.Metrics.SemanticCalls > 0 {
			attempt.TransportAttemptCount = call.Metrics.RetryCount + 1
		}
		contextErr := ctx.Err()
		if contextErr != nil {
			attempt.State = "canceled"
		} else if err != nil {
			attempt.State = "failed"
		} else if call.Metrics.CacheHit {
			attempt.State = "cached"
		} else {
			attempt.State = "response_received"
		}
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write provider latency metadata: %w", metadataErr)
			}
		}
		if contextErr != nil {
			recordOrientationSemanticExchange(
				dw, requestJSON, raw, call.Metrics,
				debugdump.SemanticStateCanceled,
				debugdump.SemanticValidationCanceled,
				debugdump.SemanticUnavailableCanceled,
			)
			return nil, contextErr
		}
		if err != nil {
			recordOrientationSemanticExchange(
				dw, requestJSON, providerFailureContentForExchange(err, raw), call.Metrics,
				debugdump.SemanticStateProviderFailed,
				debugdump.SemanticValidationProvider,
				debugdump.SemanticUnavailableNoContent,
			)
			if dw != nil {
				writeOrientationFailureArtifacts(dw, "provider_request_failed", err)
			}
			return nil, err
		}

		or, responseFailureState, responseErr := resolvePreparedOrientation(call, prepareOrientation)
		if responseErr != nil {
			attempt.State = responseFailureState
			validationCode := debugdump.SemanticValidationResponse
			if responseFailureState == "response_rejected" {
				validationCode = debugdump.SemanticValidationSecret
			} else if responseFailureState == "response_parse_failed" {
				validationCode = debugdump.SemanticValidationDecode
			}
			recordOrientationSemanticExchange(
				dw, requestJSON, raw, call.Metrics,
				debugdump.SemanticStateRejected,
				validationCode,
				debugdump.SemanticUnavailableNoContent,
			)
			if dw != nil {
				_ = dw.WriteMetadata(runMeta)
				writeOrientationFailureArtifacts(
					dw,
					responseFailureState,
					responseErr,
				)
			}
			if responseFailureState == "response_parse_failed" {
				return nil, fmt.Errorf("llm provider returned invalid JSON for orientation")
			}
			return nil, responseErr
		}
		if call.Metrics.CacheHit {
			attempt.State = "cached"
			researchState.Orientation.Status = "cached"
			recordOrientationSemanticExchange(
				dw, requestJSON, raw, call.Metrics,
				debugdump.SemanticStateCacheHit,
				debugdump.SemanticValidationCache,
				debugdump.SemanticUnavailableCache,
			)
		} else {
			attempt.State = "succeeded"
			researchState.Orientation.Status = "completed"
			recordOrientationSemanticExchange(
				dw, requestJSON, raw, call.Metrics,
				debugdump.SemanticStateAccepted,
				debugdump.SemanticValidationAccepted,
				debugdump.SemanticUnavailableNoContent,
			)
			if err := saveOrientationResponse(call); err != nil {
				return nil, fmt.Errorf("persist validated orientation cache: %w", err)
			}
		}
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write successful request metadata: %w", metadataErr)
			}
		}
		report.Orientation = &or
		runMeta.CandidateDirectionCount = len(or.CandidateFlows)
		acceptedFlows := acceptedCandidateFlows(or.CandidateFlows)
		runMeta.AcceptedDirectionCount = len(acceptedFlows)
		runMeta.RejectedDirectionCount = len(or.CandidateFlows) - len(acceptedFlows)
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write orientation metadata: %w", metadataErr)
			}
		}
		emitProgress(opts, ProgressEvent{
			Stage:          ProgressOrientationDone,
			RepoName:       s.RepoName,
			Model:          client.Model,
			CandidateCount: len(acceptedFlows),
			RejectedCount:  len(or.CandidateFlows) - len(acceptedFlows),
			LatencyMillis:  providerLatency,
			ResponseBytes:  call.Metrics.ResponseBytes,
			InputTokens:    call.Metrics.InputTokens,
			OutputTokens:   call.Metrics.OutputTokens,
			Cached:         call.Metrics.CacheHit,
		})
		researchWarnings, researchErr := runTargetedResearch(
			ctx, opts, client, dw, bundle, s, &or, &researchState, &runMeta, successfulSurfaceResult,
		)
		if researchErr != nil {
			return nil, researchErr
		}
		report.Warnings = append(report.Warnings, researchWarnings...)
		if dw != nil && len(or.ResearchQuestions) == 0 {
			if err := modelresearch.WriteState(dw.RunDir(), researchState); err != nil {
				return nil, fmt.Errorf("persist model research state: %w", err)
			}
		}
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write targeted research metadata: %w", metadataErr)
			}
		}
		out, _ := json.MarshalIndent(or, "", "  ")
		if dw != nil {
			writeErr := dw.WriteOrientationReportWithSidecar(
				out,
				ConfidenceWarningDiagnosticsFile,
				func(savedOrientation []byte) ([]byte, error) {
					return EncodeConfidenceWarningDiagnostics(
						savedOrientation,
						or.confidenceWarningDiagnostics,
					)
				},
			)
			if writeErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write required orientation report: %w", writeErr)
			}
		}

		cfs := selectTopFlows(acceptedFlows, flowCount)
		expandedIDs := make(map[string]struct{}, len(cfs))
		for _, candidate := range cfs {
			expandedIDs[flowexplain.GenerateFlowID(candidate.Name)] = struct{}{}
		}
		if err := writeLocalFlowBundles(
			acceptedFlows,
			expandedIDs,
			s.FilteredFiles,
			s.GoFacts,
			dw,
			opts,
		); err != nil {
			return nil, err
		}
		for _, cf := range cfs {
			ef := explainOneFlow(cf, s.FilteredFiles, s.GoFacts, opts.MaxLLMFiles, dw, opts)
			if ef.ArtifactError != "" {
				return nil, fmt.Errorf("persist flow %q: %s", cf.Name, ef.ArtifactError)
			}
			report.ExplainedFlows = append(report.ExplainedFlows, ef)
		}
	}

	if opts.OutputJSON {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}
		out = append(out, '\n')
		return out, nil
	}

	text := formatHumanReadable(report, opts.DebugDir, runID)
	return []byte(text), nil
}

func orientationFileLimit(explicitLimit, inputCount int) int {
	if explicitLimit > 0 {
		return explicitLimit
	}
	// The ordinary Orientation path is byte-bounded, not count-bounded. Every
	// eligible candidate comes from FilteredFiles, so this input count is an
	// exact upper bound without rerunning candidate selection here.
	return max(1, inputCount)
}

func resolvePreparedOrientation(
	call orientationCall,
	prepare func([]byte) (orientationPart, string, error),
) (orientationPart, string, error) {
	if call.Prepared != nil {
		return *call.Prepared, "", nil
	}
	return prepare(call.Raw)
}

func recordOrientationSemanticExchange(
	dw *debugdump.Writer,
	request,
	response []byte,
	metrics modelresearch.StageMetrics,
	state,
	validationCode,
	unavailableCode string,
) {
	if dw == nil {
		return
	}
	if metrics.SemanticCalls == 0 && state != debugdump.SemanticStateCacheHit {
		return
	}
	transportAttempts := 0
	if metrics.SemanticCalls > 0 {
		transportAttempts = metrics.RetryCount + 1
	}
	exchange := debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageOrientation,
		InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: debugdump.SemanticRequestPrepared,
		State:             state, ValidationCode: validationCode,
		SemanticCalls: metrics.SemanticCalls, TransportAttempts: transportAttempts,
		Request: request, Response: response,
	}
	if len(response) == 0 {
		exchange.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: unavailableCode, OriginalBytes: metrics.ResponseBytes,
		}
	}
	dw.RecordSemanticExchange(exchange)
}

// providerFailureContentForExchange is only for the existing redacting
// semantic-exchange recorder.
func providerFailureContentForExchange(err error, fallback []byte) []byte {
	var limitErr *deepseek.ResourceLimitError
	if errors.As(err, &limitErr) {
		return limitErr.ProviderContent()
	}
	return fallback
}

func writeOrientationFailureArtifacts(
	dw *debugdump.Writer,
	stage string,
	err error,
) {
	validation, marshalErr := json.MarshalIndent(map[string]string{
		"status": "failed",
		"stage":  stage,
		"error":  err.Error(),
	}, "", "  ")
	if marshalErr == nil {
		_ = dw.WriteOrientationValidation(append(validation, '\n'))
	}
	dw.WriteError(err)
}

func acceptedCandidateFlows(flows []flowexplain.CandidateFlow) []flowexplain.CandidateFlow {
	accepted := make([]flowexplain.CandidateFlow, 0, len(flows))
	for _, flow := range flows {
		if flow.Disposition == flowexplain.DirectionAccepted {
			accepted = append(accepted, flow)
		}
	}
	return accepted
}

func formatSurfaceDiscoveryWarning(err error) string {
	const maxRunes = 500
	message := strings.Join(strings.Fields(err.Error()), " ")
	message = strings.TrimPrefix(message, "surface discovery: ")
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "…"
	}
	return "surface discovery unavailable: " + message
}
