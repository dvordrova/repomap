package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type orientationCall struct {
	Raw        []byte
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
) (orientationCall, error) {
	call := orientationCall{Metrics: modelresearch.StageMetrics{
		Stage: "orientation", Status: "prepared", RequestBytes: len(requestJSON),
	}}
	bundleHash := modelresearch.SHA256(bundleJSON)
	if dw != nil {
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
	call.Metrics.ResponseBytes = len(result.Content)
	call.Metrics.InputTokens = result.InputTokens
	call.Metrics.OutputTokens = result.OutputTokens
	call.Metrics.SemanticCalls = 1
	if result.Attempts > 1 {
		call.Metrics.RetryCount = result.Attempts - 1
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
	traceIDs := acceptedFlowIDs(report.CandidateFlows)
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
		round, callErr := modelresearch.ExecuteRound(ctx, modelresearch.ExecuteInput{
			Plan: planned, Policy: state.Policy, Usage: state.Usage, Repository: state.Repository,
			RunsDir: dw.BaseDir, RunDir: dw.RunDir(),
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

func acceptedFlowIDs(flows []flowexplain.CandidateFlow) []string {
	ids := make([]string, 0, len(flows))
	for _, flow := range flows {
		if flow.Disposition == flowexplain.DirectionAccepted {
			ids = append(ids, flowexplain.GenerateFlowID(flow.Name))
		}
	}
	return ids
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
