package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	semanticDiscoveryBundleFile             = "semantic_discovery_bundle.json"
	semanticDiscoveryOpportunityFile        = "semantic_opportunities.json"
	semanticDiscoveryLeavesFile             = "semantic_discovery_leaves.json"
	semanticDiscoveryFanInFile              = "semantic_discovery_fan_in.json"
	semanticDiscoveryStatusFile             = "semantic_discovery_status.json"
	semanticDiscoveryFanoutConcurrency      = 3
	semanticDiscoveryMaxCalls               = 7
	semanticDiscoveryMaxRequestBytes        = 512 << 10
	semanticDiscoveryMaxAggregateBytes      = 2 << 20
	semanticDiscoveryMaxNormalizationIssues = 512
)

type semanticDiscoveryEditor interface {
	SemanticDiscoveryPromptJSON(semanticdiscovery.Prompt) ([]byte, error)
	DiscoverSemanticsMeasured(context.Context, semanticdiscovery.Prompt) (modelresearch.ProviderResult, error)
}

type semanticDiscoveryStageMetrics struct {
	Stage                 string `json:"stage"`
	PromptVersion         string `json:"prompt_version"`
	Status                string `json:"status"`
	ProviderCall          bool   `json:"provider_call,omitempty"`
	RequestBytes          int    `json:"request_bytes,omitempty"`
	ResponseBytes         int    `json:"response_bytes,omitempty"`
	InputTokens           int    `json:"input_tokens,omitempty"`
	OutputTokens          int    `json:"output_tokens,omitempty"`
	PromptCacheHitTokens  int    `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int    `json:"prompt_cache_miss_tokens,omitempty"`
	LatencyMillis         int64  `json:"latency_ms,omitempty"`
}

type semanticDiscoveryOutcome struct {
	Version               int                                    `json:"version"`
	Strategy              string                                 `json:"strategy"`
	BundleSHA256          string                                 `json:"bundle_sha256"`
	ValidationState       string                                 `json:"validation_state"`
	FailureCode           string                                 `json:"failure_code,omitempty"`
	OpportunityCandidates int                                    `json:"opportunity_candidates"`
	NormalizationIssues   int                                    `json:"normalization_issues"`
	SelectedCandidates    int                                    `json:"selected_candidates"`
	LeafTasks             int                                    `json:"leaf_tasks"`
	LeafSucceeded         int                                    `json:"leaf_succeeded"`
	LeafInsufficient      int                                    `json:"leaf_insufficient"`
	LeafFailed            int                                    `json:"leaf_failed"`
	LeafReductionIssues   int                                    `json:"leaf_reduction_issues"`
	FanInReductionIssues  int                                    `json:"fan_in_reduction_issues"`
	Artifacts             int                                    `json:"artifacts"`
	ArtifactKinds         map[semanticdiscovery.ArtifactKind]int `json:"artifact_kinds,omitempty"`
	Verdicts              map[semanticdiscovery.Verdict]int      `json:"verdicts,omitempty"`
	ClaimBasis            map[semanticdiscovery.ClaimBasis]int   `json:"claim_basis,omitempty"`
	UnsupportedClaims     int                                    `json:"unsupported_claims"`
	SemanticCalls         int                                    `json:"semantic_calls"`
	RequestBytes          int                                    `json:"request_bytes"`
	AttemptedRequestBytes int                                    `json:"attempted_request_bytes"`
	ResponseBytes         int                                    `json:"response_bytes"`
	InputTokens           int                                    `json:"input_tokens"`
	OutputTokens          int                                    `json:"output_tokens"`
	PromptCacheHitTokens  int                                    `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int                                    `json:"prompt_cache_miss_tokens"`
	ProviderLatencyMillis int64                                  `json:"provider_latency_ms"`
	SynthesisWallMillis   int64                                  `json:"synthesis_wall_ms"`
	WallMillis            int64                                  `json:"wall_ms"`
	Stages                []semanticDiscoveryStageMetrics        `json:"stages"`
}

type semanticDiscoveryRunResult struct {
	Outcome   semanticDiscoveryOutcome
	Bundle    semanticdiscovery.Bundle
	Proposal  semanticdiscovery.OpportunityProposal
	Selected  []semanticdiscovery.OpportunityCandidate
	Leaves    []semanticdiscovery.LeafResult
	FanIn     semanticdiscovery.FanInArtifact
	Artifacts []semanticdiscovery.Artifact
}

type semanticDiscoveryOpportunityArtifact struct {
	Version              int                                   `json:"version"`
	BundleSHA256         string                                `json:"bundle_sha256"`
	Proposal             semanticdiscovery.OpportunityProposal `json:"proposal"`
	SelectedCandidateIDs []string                              `json:"selected_candidate_ids"`
	Normalization        semanticdiscovery.NormalizationReport `json:"normalization"`
}

type semanticDiscoveryLeafFailure struct {
	TaskID      string                         `json:"task_id"`
	CandidateID string                         `json:"candidate_id"`
	Kind        semanticdiscovery.ArtifactKind `json:"kind"`
	Reason      string                         `json:"reason"`
}

type semanticDiscoveryLeavesArtifact struct {
	Version      int                              `json:"version"`
	BundleSHA256 string                           `json:"bundle_sha256"`
	Results      []semanticdiscovery.LeafResult   `json:"validated_results"`
	Reductions   []semanticDiscoveryLeafReduction `json:"reductions,omitempty"`
	Failures     []semanticDiscoveryLeafFailure   `json:"failures"`
}

type semanticDiscoveryLeafReduction struct {
	TaskID      string                                `json:"task_id"`
	CandidateID string                                `json:"candidate_id"`
	Report      semanticdiscovery.LeafReductionReport `json:"report"`
}

type semanticDiscoveryFanInArtifactFile struct {
	Version      int                                    `json:"version"`
	BundleSHA256 string                                 `json:"bundle_sha256"`
	Artifact     semanticdiscovery.FanInArtifact        `json:"artifact"`
	Reduction    semanticdiscovery.FanInReductionReport `json:"reduction"`
}

type semanticDiscoveryBudget struct {
	mu           sync.Mutex
	calls        int
	requestBytes int
}

func (budget *semanticDiscoveryBudget) reserve(requestBytes int) error {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if requestBytes <= 0 || requestBytes > semanticDiscoveryMaxRequestBytes {
		return fmt.Errorf("request_byte_budget_exhausted")
	}
	if budget.calls >= semanticDiscoveryMaxCalls {
		return fmt.Errorf("semantic_call_budget_exhausted")
	}
	if budget.requestBytes+requestBytes > semanticDiscoveryMaxAggregateBytes {
		return fmt.Errorf("aggregate_request_byte_budget_exhausted")
	}
	budget.calls++
	budget.requestBytes += requestBytes
	return nil
}

type semanticDiscoveryStagePlan struct {
	name    string
	prompt  semanticdiscovery.Prompt
	request []byte
}

type semanticDiscoveryValidator func([]byte) error

func editSemanticDiscoveryForRun(
	ctx context.Context,
	runDir string,
	stderr io.Writer,
) (semanticDiscoveryRunResult, error) {
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return semanticDiscoveryRunResult{}, fmt.Errorf("semantic discovery: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	return prepareSemanticDiscovery(ctx, runDir, client)
}

func prepareSemanticDiscovery(
	ctx context.Context,
	runDir string,
	provider semanticDiscoveryEditor,
) (semanticDiscoveryRunResult, error) {
	if err := removeSemanticDiscoveryOutputs(runDir); err != nil {
		return semanticDiscoveryRunResult{}, err
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return semanticDiscoveryRunResult{}, fmt.Errorf("semantic discovery: read saved run: %w", err)
	}
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return semanticDiscoveryRunResult{}, fmt.Errorf("semantic discovery: build bounded saved-fact bundle: %w", err)
	}
	return ensureSemanticDiscoveryFanout(ctx, bundle, runDir, provider)
}

func ensureSemanticDiscoveryFanout(
	ctx context.Context,
	bundle semanticdiscovery.Bundle,
	runDir string,
	provider semanticDiscoveryEditor,
) (result semanticDiscoveryRunResult, returnErr error) {
	started := time.Now()
	var synthesisStarted time.Time
	result.Bundle = bundle
	result.Outcome = semanticDiscoveryOutcome{
		Version:  1,
		Strategy: "fan_out_fan_in",
	}
	defer func() {
		result.Outcome.WallMillis = time.Since(started).Milliseconds()
		if !synthesisStarted.IsZero() {
			result.Outcome.SynthesisWallMillis = time.Since(synthesisStarted).Milliseconds()
		}
		result.Outcome.recomputeTotals()
		if returnErr != nil && result.Outcome.ValidationState == "" {
			result.Outcome.ValidationState = "failed"
			result.Outcome.FailureCode = semanticDiscoveryFailureCode(returnErr)
		}
		if err := writeSemanticDiscoveryJSON(
			runDir,
			semanticDiscoveryStatusFile,
			result.Outcome,
		); err != nil {
			if returnErr == nil {
				returnErr = err
			} else {
				returnErr = errors.Join(returnErr, err)
			}
		}
	}()

	if ctx == nil {
		return result, fmt.Errorf("semantic discovery: context is required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if provider == nil {
		return result, fmt.Errorf("semantic discovery: provider is required")
	}
	bundleSHA, canonicalBundle, err := semanticdiscovery.BundleHash(bundle)
	if err != nil {
		return result, err
	}
	result.Outcome.BundleSHA256 = bundleSHA
	if err := writeGuidedTourArtifact(
		filepath.Join(runDir, semanticDiscoveryBundleFile),
		append(canonicalBundle, '\n'),
	); err != nil {
		return result, fmt.Errorf("semantic discovery: save bounded bundle: %w", err)
	}

	budget := &semanticDiscoveryBudget{}
	proposal, _, normalization, scanStage, err := executeSemanticOpportunityScan(
		ctx, bundle, provider, budget,
	)
	result.Outcome.Stages = append(result.Outcome.Stages, scanStage)
	result.Outcome.NormalizationIssues = len(normalization.Issues)
	if err != nil {
		result.Outcome.ValidationState = scanStage.Status
		result.Outcome.FailureCode = semanticDiscoveryFailureCode(err)
		return result, err
	}
	result.Proposal = proposal
	result.Outcome.OpportunityCandidates = len(proposal.Candidates)
	selected, err := semanticdiscovery.SelectOpportunities(
		bundle,
		proposal,
		semanticdiscovery.MaxSelectedCandidates,
	)
	if err != nil {
		return result, err
	}
	result.Selected = selected
	result.Outcome.SelectedCandidates = len(selected)
	if err := writeSemanticDiscoveryJSON(
		runDir,
		semanticDiscoveryOpportunityFile,
		semanticDiscoveryOpportunityArtifact{
			Version: 1, BundleSHA256: bundleSHA, Proposal: proposal,
			SelectedCandidateIDs: semanticCandidateIDs(selected),
			Normalization:        normalization,
		},
	); err != nil {
		return result, err
	}
	synthesisStarted = time.Now()

	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, selected)
	if err != nil {
		return result, err
	}
	result.Outcome.LeafTasks = len(tasks)
	leaves, failures, reductions, leafStages, err := executeSemanticLeaves(
		ctx, tasks, provider, budget,
	)
	result.Outcome.Stages = append(result.Outcome.Stages, leafStages...)
	result.Leaves = leaves
	result.Outcome.LeafSucceeded = len(leaves)
	result.Outcome.LeafFailed = len(failures)
	for _, reduction := range reductions {
		result.Outcome.LeafReductionIssues += len(reduction.Report.Issues)
	}
	for _, leaf := range leaves {
		if leaf.Artifact.Status == semanticdiscovery.LeafStatusInsufficientEvidence {
			result.Outcome.LeafInsufficient++
		}
	}
	if writeErr := writeSemanticDiscoveryJSON(
		runDir,
		semanticDiscoveryLeavesFile,
		semanticDiscoveryLeavesArtifact{
			Version: 1, BundleSHA256: bundleSHA, Results: leaves,
			Reductions: reductions, Failures: failures,
		},
	); writeErr != nil {
		return result, writeErr
	}
	if err != nil {
		return result, err
	}
	if len(leaves) == 0 {
		result.Outcome.ValidationState = "rejected_no_valid_leaves"
		return result, fmt.Errorf("semantic discovery: no leaf passed local validation")
	}

	// Missing-only leaves are deliberately retained. Fan-in is the first stage
	// allowed to judge whether their combined mechanism is supported.
	fanIn, artifacts, fanInStage, unsupported, fanInReduction, err := executeSemanticFanIn(
		ctx,
		bundle,
		proposal,
		selected,
		leaves,
		runDir,
		provider,
		budget,
	)
	result.Outcome.Stages = append(result.Outcome.Stages, fanInStage)
	result.Outcome.UnsupportedClaims += unsupported
	result.Outcome.FanInReductionIssues += len(fanInReduction.Issues)
	if err != nil {
		result.Outcome.ValidationState = fanInStage.Status
		result.Outcome.FailureCode = semanticDiscoveryFailureCode(err)
		return result, err
	}
	result.FanIn = fanIn
	result.Artifacts = artifacts
	result.Outcome.addArtifactCoverage(artifacts)
	if len(failures) > 0 || fanInReduction.DroppedArtifacts > 0 {
		result.Outcome.ValidationState = "accepted_partial"
	} else {
		result.Outcome.ValidationState = "accepted"
	}
	return result, nil
}

func executeSemanticOpportunityScan(
	ctx context.Context,
	bundle semanticdiscovery.Bundle,
	provider semanticDiscoveryEditor,
	budget *semanticDiscoveryBudget,
) (
	semanticdiscovery.OpportunityProposal,
	semanticdiscovery.OpportunityProposal,
	semanticdiscovery.NormalizationReport,
	semanticDiscoveryStageMetrics,
	error,
) {
	prompt, err := semanticdiscovery.BuildOpportunityPrompt(bundle)
	if err != nil {
		return semanticdiscovery.OpportunityProposal{}, semanticdiscovery.OpportunityProposal{}, semanticdiscovery.NormalizationReport{}, semanticDiscoveryStageMetrics{}, err
	}
	plan, err := newSemanticDiscoveryStagePlan(
		provider,
		prompt,
		"semantic_discovery_opportunity",
	)
	if err != nil {
		return semanticdiscovery.OpportunityProposal{}, semanticdiscovery.OpportunityProposal{}, semanticdiscovery.NormalizationReport{}, semanticDiscoveryStageMetrics{}, err
	}
	var normalization semanticdiscovery.NormalizationReport
	var proposal semanticdiscovery.OpportunityProposal
	var modelProposal semanticdiscovery.OpportunityProposal
	stage, err := executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		budget,
		func(raw []byte) error {
			parsed, parseErr := semanticdiscovery.ParseOpportunityProposal(raw)
			if parseErr != nil {
				return parseErr
			}
			modelProposal = parsed
			proposal, normalization = semanticdiscovery.NormalizeOpportunityProposal(bundle, parsed)
			if validateErr := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); validateErr != nil {
				return validateErr
			}
			return validateSemanticOpportunityNormalizationReport(normalization)
		},
	)
	if err != nil {
		return proposal, modelProposal, normalization, stage, err
	}
	return proposal, modelProposal, normalization, stage, nil
}

func validateSemanticOpportunityNormalizationReport(
	report semanticdiscovery.NormalizationReport,
) error {
	if len(report.Issues) > semanticDiscoveryMaxNormalizationIssues {
		return fmt.Errorf("semantic discovery: opportunity normalization report is too large")
	}
	localCodes := map[string]struct{}{
		"invalid_candidate_enum": {}, "repository_bearing_or_malformed_prose": {},
		"unknown_support_id": {}, "candidate_without_known_support": {},
		"invalid_missing_information": {}, "candidate_kind_not_locally_grounded": {},
		"duplicate_enrichment_support_id": {}, "unknown_enrichment_support_id": {},
		"invalid_product_intent": {}, "capability_contract_derived": {},
		"expected_path_support_reduced": {}, "architecture_anchor_support_reduced": {},
	}
	globalCodes := map[string]struct{}{
		"duplicate_candidate_removed": {}, "candidate_limit_applied": {},
	}
	for _, issue := range report.Issues {
		_, local := localCodes[issue.Code]
		_, global := globalCodes[issue.Code]
		if !local && !global {
			return fmt.Errorf("semantic discovery: opportunity normalization issue code is invalid")
		}
		if (local && issue.CandidateIndex < 0) || (global && issue.CandidateIndex != -1) {
			return fmt.Errorf("semantic discovery: opportunity normalization issue index is invalid")
		}
		needsDetail := issue.Code == "unknown_support_id" ||
			issue.Code == "duplicate_candidate_removed" ||
			issue.Code == "duplicate_enrichment_support_id" ||
			issue.Code == "unknown_enrichment_support_id" ||
			issue.Code == "invalid_product_intent" ||
			issue.Code == "expected_path_support_reduced" ||
			issue.Code == "architecture_anchor_support_reduced"
		if needsDetail != (issue.Detail != "") || len(issue.Detail) > 256 ||
			strings.TrimSpace(issue.Detail) != issue.Detail {
			return fmt.Errorf("semantic discovery: opportunity normalization issue detail is invalid")
		}
		for _, char := range issue.Detail {
			if char < 0x20 || char == 0x7f {
				return fmt.Errorf("semantic discovery: opportunity normalization issue detail is invalid")
			}
		}
	}
	return nil
}

type semanticDiscoveryLeafCompletion struct {
	result    semanticdiscovery.LeafResult
	failure   *semanticDiscoveryLeafFailure
	reduction semanticDiscoveryLeafReduction
	stage     semanticDiscoveryStageMetrics
}

func executeSemanticLeaves(
	ctx context.Context,
	tasks []semanticdiscovery.LeafTask,
	provider semanticDiscoveryEditor,
	budget *semanticDiscoveryBudget,
) (
	[]semanticdiscovery.LeafResult,
	[]semanticDiscoveryLeafFailure,
	[]semanticDiscoveryLeafReduction,
	[]semanticDiscoveryStageMetrics,
	error,
) {
	completions := make(chan semanticDiscoveryLeafCompletion, len(tasks))
	semaphore := make(chan struct{}, semanticDiscoveryFanoutConcurrency)
	var wait sync.WaitGroup
	for _, task := range tasks {
		task := task
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				completions <- semanticDiscoveryLeafCompletion{
					failure: semanticLeafFailure(task, "leaf_canceled"),
				}
				return
			}
			completions <- executeSemanticLeaf(
				ctx,
				task,
				provider,
				budget,
			)
		}()
	}
	wait.Wait()
	close(completions)

	results := make([]semanticdiscovery.LeafResult, 0, len(tasks))
	failures := make([]semanticDiscoveryLeafFailure, 0)
	reductions := make([]semanticDiscoveryLeafReduction, 0, len(tasks))
	stages := make([]semanticDiscoveryStageMetrics, 0, len(tasks))
	for completion := range completions {
		if completion.stage.Stage != "" {
			stages = append(stages, completion.stage)
		}
		if completion.failure != nil {
			failures = append(failures, *completion.failure)
		} else {
			results = append(results, completion.result)
		}
		if completion.reduction.TaskID != "" {
			reductions = append(reductions, completion.reduction)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Task.ID < results[j].Task.ID })
	sort.Slice(failures, func(i, j int) bool { return failures[i].TaskID < failures[j].TaskID })
	sort.Slice(reductions, func(i, j int) bool { return reductions[i].TaskID < reductions[j].TaskID })
	sort.Slice(stages, func(i, j int) bool { return stages[i].Stage < stages[j].Stage })
	if err := ctx.Err(); err != nil {
		return results, failures, reductions, stages, err
	}
	return results, failures, reductions, stages, nil
}

func executeSemanticLeaf(
	ctx context.Context,
	task semanticdiscovery.LeafTask,
	provider semanticDiscoveryEditor,
	budget *semanticDiscoveryBudget,
) semanticDiscoveryLeafCompletion {
	prompt, err := semanticdiscovery.BuildLeafPrompt(task)
	if err != nil {
		return semanticDiscoveryLeafCompletion{failure: semanticLeafFailure(task, semanticDiscoveryFailureCode(err))}
	}
	plan, err := newSemanticDiscoveryStagePlan(
		provider,
		prompt,
		"semantic_discovery_leaf/"+task.ID,
	)
	if err != nil {
		return semanticDiscoveryLeafCompletion{failure: semanticLeafFailure(task, semanticDiscoveryFailureCode(err))}
	}
	var reductionReport semanticdiscovery.LeafReductionReport
	var reducedArtifact semanticdiscovery.LeafArtifact
	stage, err := executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		budget,
		func(raw []byte) error {
			artifact, parseErr := semanticdiscovery.ParseLeafArtifact(raw)
			if parseErr != nil {
				return parseErr
			}
			artifact, reductionReport, parseErr = semanticdiscovery.ReduceLeafArtifact(task, artifact)
			if parseErr != nil {
				return parseErr
			}
			reducedArtifact = artifact
			return validateSemanticLeafReductionReport(artifact, reductionReport)
		},
	)
	reduction := semanticDiscoveryLeafReduction{
		TaskID: task.ID, CandidateID: task.Candidate.ID, Report: reductionReport,
	}
	if err != nil {
		return semanticDiscoveryLeafCompletion{
			stage: stage, reduction: reduction,
			failure: semanticLeafFailure(task, err.Error()),
		}
	}
	return semanticDiscoveryLeafCompletion{
		stage: stage, reduction: reduction,
		result: semanticdiscovery.LeafResult{Task: task, Artifact: reducedArtifact},
	}
}

func validateSemanticLeafReductionReport(
	artifact semanticdiscovery.LeafArtifact,
	report semanticdiscovery.LeafReductionReport,
) error {
	if report.KeptObservations != len(artifact.Observations) ||
		report.KeptContradictions != len(artifact.Contradictions) ||
		report.KeptMissingEvidence != len(artifact.MissingEvidence) ||
		report.DroppedObservations < 0 || report.DroppedContradictions < 0 ||
		report.DroppedMissingEvidence < 0 ||
		len(report.Issues) != report.DroppedObservations+
			report.DroppedContradictions+report.DroppedMissingEvidence {
		return fmt.Errorf("semantic discovery: leaf reduction counts are inconsistent")
	}
	allowedCodes := map[string]struct{}{
		"item_limit": {}, "unknown_support_id": {}, "repository_reference": {},
		"duplicate_value": {}, "unsupported_semantics": {}, "invalid_item": {},
	}
	seen := make(map[string]struct{}, len(report.Issues))
	counts := map[string]int{}
	for _, issue := range report.Issues {
		if issue.Index < 0 {
			return fmt.Errorf("semantic discovery: leaf reduction issue index is invalid")
		}
		switch issue.Section {
		case "observations", "contradictions", "missing_evidence":
		default:
			return fmt.Errorf("semantic discovery: leaf reduction issue section is invalid")
		}
		if _, allowed := allowedCodes[issue.Code]; !allowed {
			return fmt.Errorf("semantic discovery: leaf reduction issue code is invalid")
		}
		key := fmt.Sprintf("%s:%d", issue.Section, issue.Index)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic discovery: leaf reduction repeats an issue")
		}
		seen[key] = struct{}{}
		counts[issue.Section]++
	}
	if counts["observations"] != report.DroppedObservations ||
		counts["contradictions"] != report.DroppedContradictions ||
		counts["missing_evidence"] != report.DroppedMissingEvidence {
		return fmt.Errorf("semantic discovery: leaf reduction issue counts are inconsistent")
	}
	return nil
}

func executeSemanticFanIn(
	ctx context.Context,
	bundle semanticdiscovery.Bundle,
	proposal semanticdiscovery.OpportunityProposal,
	selected []semanticdiscovery.OpportunityCandidate,
	leaves []semanticdiscovery.LeafResult,
	runDir string,
	provider semanticDiscoveryEditor,
	budget *semanticDiscoveryBudget,
) (
	semanticdiscovery.FanInArtifact,
	[]semanticdiscovery.Artifact,
	semanticDiscoveryStageMetrics,
	int,
	semanticdiscovery.FanInReductionReport,
	error,
) {
	prompt, err := semanticdiscovery.BuildFanInPrompt(bundle, leaves)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, semanticDiscoveryStageMetrics{}, 0, semanticdiscovery.FanInReductionReport{}, err
	}
	plan, err := newSemanticDiscoveryStagePlan(
		provider,
		prompt,
		"semantic_discovery_fan_in",
	)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, semanticDiscoveryStageMetrics{}, 0, semanticdiscovery.FanInReductionReport{}, err
	}
	unsupported := 0
	var reduction semanticdiscovery.FanInReductionReport
	var reducedArtifact semanticdiscovery.FanInArtifact
	stage, err := executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		budget,
		func(raw []byte) error {
			artifact, parseErr := semanticdiscovery.ParseFanInArtifact(raw)
			if parseErr != nil {
				return parseErr
			}
			artifact = semanticdiscovery.NormalizeFanInArtifact(artifact)
			unsupported = semanticdiscovery.UnsupportedClaimCount(bundle, leaves, artifact)
			artifact, reduction, parseErr = semanticdiscovery.ReduceFanInArtifact(bundle, leaves, artifact)
			if parseErr != nil {
				return parseErr
			}
			reducedArtifact = artifact
			return validateSemanticFanInReductionReport(artifact, reduction)
		},
	)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, err
	}
	artifact := reducedArtifact
	artifacts, err := semanticdiscovery.MaterializePartialArtifacts(bundle, leaves, artifact)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, err
	}
	record, err := semanticdiscovery.EncodeRecord(bundle, proposal, selected, leaves, artifact)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, err
	}
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, err
	}
	if err := writeSemanticDiscoveryJSON(runDir, semanticDiscoveryFanInFile, semanticDiscoveryFanInArtifactFile{
		Version: 1, BundleSHA256: bundleSHA, Artifact: artifact, Reduction: reduction,
	}); err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, err
	}
	if err := writeGuidedTourArtifact(
		filepath.Join(runDir, semanticdiscovery.RecordFile),
		append(record, '\n'),
	); err != nil {
		return semanticdiscovery.FanInArtifact{}, nil, stage, unsupported, reduction, fmt.Errorf("semantic discovery: save replay record: %w", err)
	}
	return artifact, artifacts, stage, unsupported, reduction, nil
}

func validateSemanticFanInReductionReport(
	artifact semanticdiscovery.FanInArtifact,
	report semanticdiscovery.FanInReductionReport,
) error {
	expectedIssues := report.DroppedArtifacts
	if expectedIssues > semanticdiscovery.MaxFanInReductionIssues {
		expectedIssues = semanticdiscovery.MaxFanInReductionIssues
	}
	if report.KeptArtifacts != len(artifact.Artifacts) || report.DroppedArtifacts < 0 ||
		len(report.Issues) != expectedIssues {
		return fmt.Errorf("semantic discovery: fan-in reduction counts are inconsistent")
	}
	allowedCodes := map[string]struct{}{
		"unknown_candidate": {}, "duplicate_candidate": {}, "invalid_proposal": {},
	}
	seen := make(map[int]struct{}, len(report.Issues))
	for _, issue := range report.Issues {
		if issue.ArtifactIndex < 0 {
			return fmt.Errorf("semantic discovery: fan-in reduction index is invalid")
		}
		if _, allowed := allowedCodes[issue.Code]; !allowed {
			return fmt.Errorf("semantic discovery: fan-in reduction code is invalid")
		}
		if _, duplicate := seen[issue.ArtifactIndex]; duplicate {
			return fmt.Errorf("semantic discovery: fan-in reduction repeats an artifact index")
		}
		seen[issue.ArtifactIndex] = struct{}{}
	}
	return nil
}

func newSemanticDiscoveryStagePlan(
	provider semanticDiscoveryEditor,
	prompt semanticdiscovery.Prompt,
	stage string,
) (semanticDiscoveryStagePlan, error) {
	request, err := provider.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		return semanticDiscoveryStagePlan{}, fmt.Errorf("semantic discovery: build %s request: %w", stage, err)
	}
	if len(request) == 0 || len(request) > semanticDiscoveryMaxRequestBytes {
		return semanticDiscoveryStagePlan{}, fmt.Errorf("semantic discovery: %s request exceeds bounded request size", stage)
	}
	return semanticDiscoveryStagePlan{
		name: stage, prompt: prompt, request: request,
	}, nil
}

func executeSemanticDiscoveryStage(
	ctx context.Context,
	provider semanticDiscoveryEditor,
	plan semanticDiscoveryStagePlan,
	budget *semanticDiscoveryBudget,
	validate semanticDiscoveryValidator,
) (semanticDiscoveryStageMetrics, error) {
	metrics := semanticDiscoveryStageMetrics{
		Stage: plan.name, PromptVersion: plan.prompt.Version,
		RequestBytes: len(plan.request),
	}
	if err := budget.reserve(len(plan.request)); err != nil {
		metrics.Status = "skipped_budget"
		return metrics, fmt.Errorf("semantic discovery: %s: %w", plan.name, err)
	}
	metrics.ProviderCall = true
	started := time.Now()
	providerResult, callErr := provider.DiscoverSemanticsMeasured(ctx, plan.prompt)
	metrics.addResponse(providerResult, time.Since(started))
	if err := ctx.Err(); err != nil {
		metrics.Status = "canceled"
		return metrics, err
	}
	if callErr != nil {
		metrics.Status = "failed_provider"
		return metrics, fmt.Errorf("semantic discovery: %s provider call: %w", plan.name, callErr)
	}
	if err := validate(providerResult.Content); err != nil {
		metrics.Status = "rejected"
		return metrics, fmt.Errorf("semantic discovery: %s response rejected: %w", plan.name, err)
	}
	metrics.Status = "accepted"
	return metrics, nil
}

func (metrics *semanticDiscoveryStageMetrics) addResponse(
	response modelresearch.ProviderResult,
	latency time.Duration,
) {
	metrics.ResponseBytes = providerResultResponseBytes(response)
	metrics.InputTokens = response.InputTokens
	metrics.OutputTokens = response.OutputTokens
	metrics.PromptCacheHitTokens = response.PromptCacheHitTokens
	metrics.PromptCacheMissTokens = response.PromptCacheMissTokens
	metrics.LatencyMillis = latency.Milliseconds()
}

func (outcome *semanticDiscoveryOutcome) recomputeTotals() {
	sort.Slice(outcome.Stages, func(i, j int) bool { return outcome.Stages[i].Stage < outcome.Stages[j].Stage })
	outcome.SemanticCalls = 0
	outcome.RequestBytes = 0
	outcome.AttemptedRequestBytes = 0
	outcome.ResponseBytes = 0
	outcome.InputTokens = 0
	outcome.OutputTokens = 0
	outcome.PromptCacheHitTokens = 0
	outcome.PromptCacheMissTokens = 0
	outcome.ProviderLatencyMillis = 0
	for _, stage := range outcome.Stages {
		outcome.RequestBytes += stage.RequestBytes
		outcome.ResponseBytes += stage.ResponseBytes
		outcome.InputTokens += stage.InputTokens
		outcome.OutputTokens += stage.OutputTokens
		outcome.PromptCacheHitTokens += stage.PromptCacheHitTokens
		outcome.PromptCacheMissTokens += stage.PromptCacheMissTokens
		outcome.ProviderLatencyMillis += stage.LatencyMillis
		if stage.ProviderCall {
			outcome.SemanticCalls++
			outcome.AttemptedRequestBytes += stage.RequestBytes
		}
	}
}

func (outcome *semanticDiscoveryOutcome) addArtifactCoverage(artifacts []semanticdiscovery.Artifact) {
	outcome.Artifacts = len(artifacts)
	outcome.ArtifactKinds = make(map[semanticdiscovery.ArtifactKind]int)
	outcome.Verdicts = make(map[semanticdiscovery.Verdict]int)
	outcome.ClaimBasis = make(map[semanticdiscovery.ClaimBasis]int)
	for _, artifact := range artifacts {
		outcome.ArtifactKinds[artifact.Kind]++
		outcome.Verdicts[artifact.Verdict]++
		for _, statement := range artifact.Statements {
			outcome.ClaimBasis[statement.Basis]++
		}
	}
}

func semanticCandidateIDs(candidates []semanticdiscovery.OpportunityCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	sort.Strings(ids)
	return ids
}

func semanticLeafFailure(
	task semanticdiscovery.LeafTask,
	reason string,
) *semanticDiscoveryLeafFailure {
	return &semanticDiscoveryLeafFailure{
		TaskID: task.ID, CandidateID: task.Candidate.ID, Kind: task.Candidate.Kind,
		Reason: semanticDiscoveryReason(reason),
	}
}

func semanticDiscoveryFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	message := strings.ToLower(err.Error())
	for _, candidate := range []string{
		"request_byte_budget_exhausted", "aggregate_request_byte_budget_exhausted",
		"semantic_call_budget_exhausted",
		"no leaf passed local validation", "provider call", "response rejected",
	} {
		if strings.Contains(message, strings.ReplaceAll(candidate, "_", " ")) || strings.Contains(message, candidate) {
			return strings.ReplaceAll(candidate, " ", "_")
		}
	}
	return "local_stage_failure"
}

func semanticDiscoveryReason(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}

func writeSemanticDiscoveryJSON(runDir string, name string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("semantic discovery: encode %s: %w", name, err)
	}
	if err := writeGuidedTourArtifact(filepath.Join(runDir, name), append(encoded, '\n')); err != nil {
		return fmt.Errorf("semantic discovery: save %s: %w", name, err)
	}
	return nil
}

func removeSemanticDiscoveryOutputs(runDir string) error {
	for _, name := range []string{
		semanticdiscovery.RecordFile,
		semanticDiscoveryBundleFile,
		semanticDiscoveryOpportunityFile,
		semanticDiscoveryLeavesFile,
		semanticDiscoveryFanInFile,
		semanticDiscoveryStatusFile,
	} {
		if err := os.Remove(filepath.Join(runDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("semantic discovery: remove stale %s: %w", name, err)
		}
	}
	return nil
}
