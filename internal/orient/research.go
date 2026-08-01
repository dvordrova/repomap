package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type orientationCall struct {
	Raw        []byte
	Prepared   *orientationPart
	Metrics    modelresearch.StageMetrics
	CacheInput modelresearch.StageCacheInput
	SaveCache  bool
}

func repositoryContext(opts Options, bundleJSON []byte) modelresearch.RepositoryContext {
	context := opts.RepositoryContext
	if context.Identity == "" {
		context.Identity, _ = filepath.Abs(opts.RepoPath)
	}
	if context.Revision == "" {
		context.Revision = "unknown"
	}
	if context.Scenario == "" {
		context.Scenario = "go-default"
	}
	if context.DirtySHA256 == "" {
		context.DirtySHA256 = modelresearch.SHA256(bundleJSON)
	}
	return context
}

func obtainOrientation(
	ctx context.Context,
	client *deepseek.Client,
	dw *debugdump.Writer,
	policy modelresearch.Policy,
	repository modelresearch.RepositoryContext,
	profile string,
	bundleJSON []byte,
	requestJSON []byte,
	useCache bool,
	prepareCached func([]byte) (orientationPart, error),
) (orientationCall, error) {
	call := orientationCall{Metrics: modelresearch.StageMetrics{
		Stage: "orientation", Status: "prepared", RequestBytes: len(requestJSON),
	}}
	bundleHash := modelresearch.SHA256(bundleJSON)
	if dw != nil && useCache {
		call.CacheInput = modelresearch.StageCacheInput{
			RunsDir: dw.BaseDir,
			Fingerprint: modelresearch.FingerprintInput{
				Repository: repository, Stage: "orientation",
				PromptVersion: deepseek.OrientationPromptVersionJSON,
				Profile:       profile, Model: client.Model,
				EvidenceBundleHash: bundleHash, PolicyVersion: policy.Version,
			},
			Request: requestJSON, EvidenceBundleHash: bundleHash,
		}
		cached, found, err := modelresearch.LoadStageResponse(call.CacheInput)
		if err != nil {
			return call, fmt.Errorf("orientation cache: %w", err)
		}
		if found {
			if prepareCached == nil {
				return call, fmt.Errorf("orientation cache: semantic validator is required")
			}
			prepared, prepareErr := prepareCached(cached.Content)
			if prepareErr != nil {
				if err := modelresearch.InvalidateStageResponse(call.CacheInput); err != nil {
					return call, fmt.Errorf("orientation cache: invalidate rejected hit: %w", err)
				}
				found = false
			} else {
				call.Prepared = &prepared
			}
		}
		if found {
			call.Raw = cached.Content
			call.Metrics.Status = "cached"
			call.Metrics.ResponseBytes = cached.ResponseBytes
			call.Metrics.InputTokens = cached.InputTokens
			call.Metrics.OutputTokens = cached.OutputTokens
			call.Metrics.LatencyMillis = cached.LatencyMillis
			call.Metrics.RetryCount = cached.RetryCount
			call.Metrics.CacheHit = true
			return call, nil
		}
		call.SaveCache = true
	}

	started := time.Now()
	result, err := client.OrientMeasured(ctx, bundleJSON)
	call.Metrics.LatencyMillis = time.Since(started).Milliseconds()
	call.Metrics.RequestBytes = result.RequestBytes
	call.Metrics.ResponseBytes = len(result.Content)
	call.Metrics.InputTokens = result.InputTokens
	call.Metrics.OutputTokens = result.OutputTokens
	call.Metrics.SemanticCalls = 1
	if result.Attempts > 1 {
		call.Metrics.RetryCount = result.Attempts - 1
	}
	// A completion recovery changes max_tokens for the accepted transport
	// envelope. The legacy orientation cache has one path per stage fingerprint,
	// so saving it under the original request identity would be dishonest.
	// Keep the valid per-run result and leave persistent reuse to a later exact
	// multi-envelope cache contract.
	if result.CompletionRetries > 0 {
		call.SaveCache = false
	}
	if err != nil {
		call.Metrics.Status = "failed"
		return call, err
	}
	call.Raw = result.Content
	call.Metrics.Status = "response_received"
	return call, nil
}

func saveOrientationResponse(call orientationCall) error {
	if !call.SaveCache {
		return nil
	}
	_, err := modelresearch.SaveStageResponse(call.CacheInput, modelresearch.StageResponse{
		Content: call.Raw, RequestBytes: call.Metrics.RequestBytes,
		ResponseBytes: call.Metrics.ResponseBytes,
		InputTokens:   call.Metrics.InputTokens, OutputTokens: call.Metrics.OutputTokens,
		LatencyMillis: call.Metrics.LatencyMillis, RetryCount: call.Metrics.RetryCount,
	})
	return err
}

func runTargetedResearch(
	ctx context.Context,
	opts Options,
	client *deepseek.Client,
	dw *debugdump.Writer,
	bundle llmbundle.Bundle,
	snapshot snapshot.Snapshot,
	report *orientationPart,
	state *modelresearch.State,
	runMeta *debugdump.RunMeta,
	surfaceResult *surfacediscovery.Result,
) ([]string, error) {
	if state == nil || report == nil || client == nil || dw == nil || len(report.ResearchQuestions) == 0 {
		return nil, nil
	}
	candidates := make([]modelresearch.FileCandidate, 0, len(bundle.CandidateFileIndex))
	for _, candidate := range bundle.CandidateFileIndex {
		candidates = append(candidates, modelresearch.FileCandidate{
			ID: candidate.ID, Path: candidate.Path, Kind: candidate.Kind, Score: candidate.Score,
		})
	}
	candidates = addResearchFocusLocations(candidates, report, surfaceResult, snapshot, bundle.SourceSignals)
	var traces []gofacts.CommandTrace
	if snapshot.GoFacts != nil {
		traces = snapshot.GoFacts.CommandTraces
	}
	stopPlanningHeartbeat := startProgressHeartbeat(ctx, opts, ProgressEvent{
		Stage: ProgressPlanningWaiting, RepoName: snapshot.RepoName,
		Activity: "refining repository understanding",
	})
	plan, err := modelresearch.PlanTargetedRounds(ctx, modelresearch.PlanningInput{
		RepoPath: opts.RepoPath, Questions: report.ResearchQuestions, Candidates: candidates,
		InitialProviderPaths: bundle.ProviderAllowedPaths,
		Universe: modelresearch.LocalRepositoryUniverse{
			AuthorizedPaths: snapshot.FilteredFiles, CommandTraces: traces, ScenarioID: state.Repository.Scenario,
		},
		Policy: state.Policy, Usage: state.Usage,
	})
	stopPlanningHeartbeat()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return []string{fmt.Sprintf("targeted model research planning failed: %v", err)}, nil
	}
	traceIDs := savedFlowProofIDs(report.CandidateFlows)
	state.Theory.RelatedTraceIDs = append([]string(nil), traceIDs...)
	warnings := make([]string, 0)
	for _, planned := range plan.Selected {
		if err := ctx.Err(); err != nil {
			return warnings, err
		}
		planned.Bundle.KnownTraceIDs = append([]string(nil), traceIDs...)
		emitProgress(opts, ProgressEvent{
			Stage: ProgressResearchPrepared, RepoName: snapshot.RepoName, Model: client.Model,
			Activity: "targeted research", FileCount: len(planned.Scope.LocallyInspected),
			EvidenceCount: len(planned.Bundle.Evidence),
		})
		runsDir := dw.BaseDir
		if opts.NoCache {
			runsDir = ""
		}
		round, callErr := modelresearch.ExecuteRound(ctx, modelresearch.ExecuteInput{
			Plan: planned, Policy: state.Policy, Usage: state.Usage, Repository: state.Repository,
			RunsDir: runsDir, RunDir: dw.RunDir(),
			Profile: "openai-compatible/" + client.Auth, Model: client.Model, Provider: client,
		})
		if err := ctx.Err(); err != nil {
			return warnings, err
		}
		modelresearch.ApplyRound(state, planned, round)
		if artifactErr := writeResearchBundleArtifact(dw, round, planned.Bundle); artifactErr != nil {
			warnings = append(warnings, fmt.Sprintf("persist targeted evidence bundle: %v", artifactErr))
		}
		attemptState := string(round.Status)
		runMeta.RequestAttempts = append(runMeta.RequestAttempts, debugdump.RequestAttempt{
			Stage: "targeted_research", State: attemptState, RequestBytes: round.RequestBytes,
			ProviderCallCount: boolInt(!round.Cached && round.RequestBytes > 0),
			LatencyMillis:     optionalMillis(round.LatencyMillis),
		})
		if callErr != nil {
			warnings = append(warnings, fmt.Sprintf("targeted model research %q failed; local evidence was preserved: %v", round.Question, callErr))
		}
		emitProgress(opts, ProgressEvent{
			Stage: ProgressResearchDone, RepoName: snapshot.RepoName, Model: client.Model,
			Activity: string(round.Status), RequestBytes: round.RequestBytes,
			ResponseBytes: round.ResponseBytes, FindingCount: len(round.ValidatedFindings),
			RejectedCount: len(round.RejectedFindings), NewFactCount: round.NewGroundedFactsCount,
			InputTokens: round.InputTokens, OutputTokens: round.OutputTokens,
			Cached: round.Cached, LatencyMillis: round.LatencyMillis,
		})
	}
	remainingSlots := state.Policy.MaxTargetedRounds - len(state.Rounds)
	for _, skipped := range plan.Skipped {
		if err := ctx.Err(); err != nil {
			return warnings, err
		}
		if remainingSlots <= 0 {
			break
		}
		round, _ := modelresearch.ExecuteRound(ctx, modelresearch.ExecuteInput{Plan: skipped, Policy: state.Policy})
		state.SkippedRounds = append(state.SkippedRounds, round)
		remainingSlots--
	}
	modelresearch.SortTheory(&state.Theory)
	if err := modelresearch.WriteState(dw.RunDir(), *state); err != nil {
		warnings = append(warnings, fmt.Sprintf("persist model research state: %v", err))
	}
	runMeta.ProviderRequestCount = state.Usage.SemanticCalls
	runMeta.ExternalRequestBytes = state.Usage.RequestBytes
	return warnings, nil
}

func addResearchFocusLocations(
	candidates []modelresearch.FileCandidate,
	report *orientationPart,
	surfaceResult *surfacediscovery.Result,
	snapshot snapshot.Snapshot,
	signals []sourcesignals.Signal,
) []modelresearch.FileCandidate {
	byPath := make(map[string][]evidence.Location)
	seen := make(map[string]struct{})
	addGroup := func(locations []evidence.Location) {
		sort.Slice(locations, func(i, j int) bool {
			if locations[i].Path != locations[j].Path {
				return locations[i].Path < locations[j].Path
			}
			if locations[i].Line != locations[j].Line {
				return locations[i].Line < locations[j].Line
			}
			return locations[i].Column < locations[j].Column
		})
		for _, location := range locations {
			if location.Path == "" || location.Line <= 0 {
				continue
			}
			key := fmt.Sprintf("%s\x00%d\x00%d", location.Path, location.Line, location.Column)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			byPath[location.Path] = append(byPath[location.Path], location)
		}
	}

	if surfaceResult != nil {
		registration := make([]evidence.Location, 0, len(surfaceResult.Catalog.Triggers))
		starts := make([]evidence.Location, 0, len(surfaceResult.Catalog.Triggers))
		descriptors := make([]evidence.Location, 0, len(surfaceResult.Catalog.Triggers))
		processes := make([]evidence.Location, 0, len(surfaceResult.Catalog.Triggers))
		for _, trigger := range surfaceResult.Catalog.Triggers {
			registration = appendSurfaceFocusLocation(registration, trigger.RegistrationSite)
			if trigger.ServerStartSite != nil {
				starts = appendSurfaceFocusLocation(starts, *trigger.ServerStartSite)
			}
			if trigger.DescriptorSite != nil {
				descriptors = appendSurfaceFocusLocation(descriptors, *trigger.DescriptorSite)
			}
			processes = appendSurfaceFocusLocation(processes, trigger.ProcessEntrypoint.Location)
		}
		addGroup(registration)
		addGroup(starts)
		addGroup(descriptors)
		addGroup(processes)
	}

	frontiers := make([]evidence.Location, 0)
	transitions := make([]evidence.Location, 0)
	anchors := make([]evidence.Location, 0)
	if report != nil {
		for _, flow := range report.CandidateFlows {
			if flow.LocalProof == nil {
				continue
			}
			proof := flow.LocalProof.Proof
			anchorByID := make(map[string]evidence.Location, len(proof.Anchors))
			transitionByID := make(map[string]evidence.Location, len(proof.Transitions))
			for _, anchor := range proof.Anchors {
				if anchor.Location != nil {
					anchorByID[anchor.ID] = *anchor.Location
					anchors = append(anchors, *anchor.Location)
				}
			}
			for _, transition := range proof.Transitions {
				transitionByID[transition.ID] = transition.Evidence
				transitions = append(transitions, transition.Evidence)
				if transition.Resolution == evidence.ResolutionUnresolved {
					frontiers = append(frontiers, transition.Evidence)
				}
			}
			for _, slot := range proof.Slots {
				if slot.Status != flowproof.SlotMissing && slot.Status != flowproof.SlotPartial &&
					slot.Status != flowproof.SlotUnresolved {
					continue
				}
				for _, evidenceID := range slot.EvidenceIDs {
					if location, ok := transitionByID[evidenceID]; ok {
						frontiers = append(frontiers, location)
					}
					if location, ok := anchorByID[evidenceID]; ok {
						frontiers = append(frontiers, location)
					}
				}
			}
		}
	}
	addGroup(frontiers)
	addGroup(transitions)
	addGroup(anchors)

	if surfaceResult != nil {
		behavior := make([]evidence.Location, 0, len(surfaceResult.Grounding.Anchors))
		for _, anchor := range surfaceResult.Grounding.Anchors {
			behavior = appendSurfaceFocusLocation(behavior, anchor.Location)
		}
		addGroup(behavior)
	}

	commandCallsites := make([]evidence.Location, 0)
	commandDeclarations := make([]evidence.Location, 0)
	if snapshot.GoFacts != nil {
		for _, trace := range snapshot.GoFacts.CommandTraces {
			for _, step := range trace.Steps {
				if step.CallsiteLocation != nil {
					commandCallsites = append(commandCallsites, *step.CallsiteLocation)
				}
				commandDeclarations = append(commandDeclarations, step.TargetLocation)
			}
			for _, call := range trace.HandlerCalls {
				commandCallsites = append(commandCallsites, evidence.Location{Path: call.Path, Line: call.Line})
				if call.Resolved && call.TargetPath != "" {
					commandDeclarations = append(commandDeclarations, evidence.Location{Path: call.TargetPath, Line: call.TargetLine})
				}
			}
		}
	}
	addGroup(commandCallsites)
	addGroup(commandDeclarations)

	sourceLocations := make([]evidence.Location, 0, len(signals))
	for _, signal := range signals {
		sourceLocations = append(sourceLocations, evidence.Location{Path: signal.Path, Line: signal.Line})
	}
	addGroup(sourceLocations)

	result := append([]modelresearch.FileCandidate(nil), candidates...)
	for index := range result {
		result[index].FocusLocations = append([]evidence.Location(nil), byPath[result[index].Path]...)
	}
	return result
}

func appendSurfaceFocusLocation(locations []evidence.Location, location surfacediscovery.Location) []evidence.Location {
	if location.Path == "" || location.Line <= 0 {
		return locations
	}
	return append(locations, evidence.Location{Path: location.Path, Line: location.Line, Column: location.Column})
}

func savedFlowProofIDs(flows []flowexplain.CandidateFlow) []string {
	ids := make([]string, 0, len(flows))
	for _, flow := range flows {
		if flow.LocalProof != nil && flow.LocalProof.Proof.ID != "" {
			ids = append(ids, flow.LocalProof.Proof.ID)
		}
	}
	sort.Strings(ids)
	result := ids[:0]
	for _, id := range ids {
		if len(result) == 0 || result[len(result)-1] != id {
			result = append(result, id)
		}
	}
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func optionalMillis(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func writeResearchBundleArtifact(dw *debugdump.Writer, round modelresearch.ResearchRound, bundle modelresearch.EvidenceBundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	subdir := filepath.Join("research", strings.TrimSpace(round.ID))
	return dw.WriteDirFile(subdir, "evidence_bundle.json", append(data, '\n'))
}
