package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	semanticDiscoveryMonolithicFile = "semantic_discovery.monolithic.json"
	semanticDiscoveryComparisonFile = "semantic_discovery_comparison.json"
)

type semanticDiscoveryMonolithicResult struct {
	Outcome   semanticDiscoveryOutcome
	FanIn     semanticdiscovery.FanInArtifact
	Reduction semanticdiscovery.FanInReductionReport
	Artifacts []semanticdiscovery.Artifact
}

type semanticDiscoveryMonolithicArtifact struct {
	Version              int                                    `json:"version"`
	BundleSHA256         string                                 `json:"bundle_sha256"`
	SelectedCandidateIDs []string                               `json:"selected_candidate_ids"`
	FanIn                semanticdiscovery.FanInArtifact        `json:"proposal,omitempty"`
	Reduction            semanticdiscovery.FanInReductionReport `json:"reduction"`
	Artifacts            []semanticdiscovery.Artifact           `json:"artifacts,omitempty"`
	Metrics              semanticDiscoveryOutcome               `json:"metrics"`
}

type semanticDiscoveryVariantMetrics struct {
	Strategy              string                                 `json:"strategy"`
	ValidationState       string                                 `json:"validation_state"`
	FailureCode           string                                 `json:"failure_code,omitempty"`
	SelectedCandidates    int                                    `json:"selected_candidates"`
	Artifacts             int                                    `json:"artifacts"`
	CompletenessPercent   int                                    `json:"completeness_percent"`
	ArtifactKinds         map[semanticdiscovery.ArtifactKind]int `json:"artifact_kinds,omitempty"`
	Verdicts              map[semanticdiscovery.Verdict]int      `json:"verdicts,omitempty"`
	ClaimBasis            map[semanticdiscovery.ClaimBasis]int   `json:"claim_basis,omitempty"`
	UnsupportedClaims     int                                    `json:"unsupported_claims"`
	LeafReductionIssues   int                                    `json:"leaf_reduction_issues,omitempty"`
	FanInReductionIssues  int                                    `json:"fan_in_reduction_issues,omitempty"`
	ProviderCalls         int                                    `json:"provider_calls"`
	RequestBytes          int                                    `json:"request_bytes"`
	InputTokens           int                                    `json:"input_tokens"`
	OutputTokens          int                                    `json:"output_tokens"`
	PromptCacheHitTokens  int                                    `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int                                    `json:"prompt_cache_miss_tokens"`
	ProviderLatencyMillis int64                                  `json:"provider_latency_ms"`
	WallMillis            int64                                  `json:"wall_ms"`
}

type semanticDiscoveryComparison struct {
	Version           int                               `json:"version"`
	BundleSHA256      string                            `json:"bundle_sha256"`
	Model             string                            `json:"model"`
	Profile           string                            `json:"profile"`
	SharedOpportunity semanticDiscoveryStageMetrics     `json:"shared_opportunity_scan"`
	Variants          []semanticDiscoveryVariantMetrics `json:"variants"`
	TokenComparison   semanticDiscoveryTokenComparison  `json:"token_comparison"`
}

type semanticDiscoveryTokenComparison struct {
	Basis                        string `json:"basis"`
	SameBundleAndCandidates      bool   `json:"same_bundle_and_candidates"`
	FanOutTotalTokens            int    `json:"fan_out_total_tokens"`
	MonolithicTotalTokens        int    `json:"monolithic_total_tokens"`
	LargerToSmallerRatioPermille int    `json:"larger_to_smaller_ratio_permille"`
	ComparableWithin25Percent    bool   `json:"comparable_within_25_percent"`
}

func runSemanticDiscovery(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, runErr := editSemanticDiscoveryForRun(ctx, absDir, stderr)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err := report.Generate(absDir); err != nil {
		return errors.Join(runErr, err)
	}
	writeSemanticDiscoveryProgress(stderr, result.Outcome)
	fmt.Printf("Report: %s/report.html\n", absDir)
	return runErr
}

func runSemanticDiscoveryExperiment(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	for _, name := range []string{semanticDiscoveryMonolithicFile, semanticDiscoveryComparisonFile} {
		if err := os.Remove(filepath.Join(absDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("semantic discovery experiment: remove stale %s: %w", name, err)
		}
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("semantic discovery experiment: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	profile := "openai-compatible/" + client.Auth

	fanout, fanoutErr := prepareSemanticDiscovery(ctx, absDir, client)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	monolithic := semanticDiscoveryMonolithicResult{
		Outcome: semanticDiscoveryOutcome{
			Version: 1, Strategy: "monolithic", BundleSHA256: fanout.Outcome.BundleSHA256,
			ValidationState: "skipped_no_opportunity_plan",
		},
	}
	var monolithicErr error
	if len(fanout.Selected) > 0 {
		monolithic, monolithicErr = executeSemanticMonolithic(
			ctx,
			fanout.Bundle,
			fanout.Selected,
			client,
		)
	}
	if err := writeSemanticDiscoveryJSON(
		absDir,
		semanticDiscoveryMonolithicFile,
		semanticDiscoveryMonolithicArtifact{
			Version: 1, BundleSHA256: monolithic.Outcome.BundleSHA256,
			SelectedCandidateIDs: semanticCandidateIDs(fanout.Selected),
			FanIn:                monolithic.FanIn, Reduction: monolithic.Reduction,
			Artifacts: monolithic.Artifacts,
			Metrics:   monolithic.Outcome,
		},
	); err != nil {
		monolithicErr = errors.Join(monolithicErr, err)
	}

	comparison := semanticDiscoveryComparison{
		Version:      2,
		BundleSHA256: fanout.Outcome.BundleSHA256,
		Model:        client.Model,
		Profile:      profile,
		Variants: []semanticDiscoveryVariantMetrics{
			semanticDiscoveryVariant("fan_out_fan_in", fanout.Outcome, true),
			semanticDiscoveryVariant("monolithic", monolithic.Outcome, false),
		},
	}
	comparison.TokenComparison = compareSemanticDiscoveryTokens(comparison.Variants)
	for _, stage := range fanout.Outcome.Stages {
		if stage.Stage == "semantic_discovery_opportunity" {
			comparison.SharedOpportunity = stage
			break
		}
	}
	if err := writeSemanticDiscoveryJSON(
		absDir,
		semanticDiscoveryComparisonFile,
		comparison,
	); err != nil {
		return errors.Join(fanoutErr, monolithicErr, err)
	}
	if err := report.Generate(absDir); err != nil {
		return errors.Join(fanoutErr, monolithicErr, err)
	}
	writeSemanticDiscoveryProgress(stderr, fanout.Outcome)
	writeSemanticDiscoveryProgress(stderr, monolithic.Outcome)
	if fanoutErr != nil {
		fmt.Fprintf(stderr, "warning: fan-out/fan-in variant: %s\n", semanticDiscoveryReason(fanoutErr.Error()))
	}
	if monolithicErr != nil {
		fmt.Fprintf(stderr, "warning: monolithic variant: %s\n", semanticDiscoveryReason(monolithicErr.Error()))
	}
	fmt.Printf("Comparison: %s/%s\n", absDir, semanticDiscoveryComparisonFile)
	fmt.Printf("Report: %s/report.html\n", absDir)
	if fanoutErr != nil && monolithicErr != nil {
		return errors.Join(
			fmt.Errorf("fan-out/fan-in variant: %w", fanoutErr),
			fmt.Errorf("monolithic variant: %w", monolithicErr),
		)
	}
	return nil
}

func compareSemanticDiscoveryTokens(
	variants []semanticDiscoveryVariantMetrics,
) semanticDiscoveryTokenComparison {
	comparison := semanticDiscoveryTokenComparison{
		Basis: "recorded_input_plus_output_tokens",
		SameBundleAndCandidates: len(variants) == 2 &&
			variants[0].SelectedCandidates == variants[1].SelectedCandidates,
	}
	if len(variants) != 2 {
		return comparison
	}
	comparison.FanOutTotalTokens = variants[0].InputTokens + variants[0].OutputTokens
	comparison.MonolithicTotalTokens = variants[1].InputTokens + variants[1].OutputTokens
	smaller := min(comparison.FanOutTotalTokens, comparison.MonolithicTotalTokens)
	larger := max(comparison.FanOutTotalTokens, comparison.MonolithicTotalTokens)
	if smaller <= 0 {
		return comparison
	}
	comparison.LargerToSmallerRatioPermille = larger * 1000 / smaller
	comparison.ComparableWithin25Percent = comparison.LargerToSmallerRatioPermille <= 1250
	return comparison
}

func executeSemanticMonolithic(
	ctx context.Context,
	bundle semanticdiscovery.Bundle,
	selected []semanticdiscovery.OpportunityCandidate,
	provider semanticDiscoveryEditor,
) (result semanticDiscoveryMonolithicResult, returnErr error) {
	started := time.Now()
	result.Outcome = semanticDiscoveryOutcome{
		Version: 1, Strategy: "monolithic", SelectedCandidates: len(selected),
	}
	defer func() {
		result.Outcome.WallMillis = time.Since(started).Milliseconds()
		result.Outcome.SynthesisWallMillis = result.Outcome.WallMillis
		result.Outcome.recomputeTotals()
		if returnErr != nil && result.Outcome.FailureCode == "" {
			result.Outcome.FailureCode = semanticDiscoveryFailureCode(returnErr)
		}
	}()
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil {
		return result, err
	}
	result.Outcome.BundleSHA256 = bundleSHA
	prompt, err := semanticdiscovery.BuildMonolithicPrompt(bundle, selected)
	if err != nil {
		return result, err
	}
	plan, err := newSemanticDiscoveryStagePlan(
		provider,
		prompt,
		"semantic_discovery_monolithic",
	)
	if err != nil {
		return result, err
	}
	unsupported := 0
	var reduction semanticdiscovery.FanInReductionReport
	var reducedArtifact semanticdiscovery.FanInArtifact
	stage, err := executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		&semanticDiscoveryBudget{},
		func(raw []byte) error {
			artifact, parseErr := semanticdiscovery.ParseMonolithicArtifact(raw)
			if parseErr != nil {
				return parseErr
			}
			artifact = semanticdiscovery.NormalizeMonolithicArtifact(artifact)
			// Preserve the parsed response in the experiment artifact even when
			// proposal reduction rejects every item. This is model output only;
			// it is never materialized or used as repository evidence.
			result.FanIn = artifact
			unsupported = semanticdiscovery.UnsupportedMonolithicClaimCount(bundle, selected, artifact)
			artifact, reduction, parseErr = semanticdiscovery.ReduceMonolithicArtifact(
				bundle,
				selected,
				artifact,
			)
			if parseErr != nil {
				return parseErr
			}
			reducedArtifact = artifact
			return validateSemanticFanInReductionReport(artifact, reduction)
		},
	)
	result.Outcome.Stages = append(result.Outcome.Stages, stage)
	result.Outcome.UnsupportedClaims = unsupported
	result.Outcome.FanInReductionIssues = len(reduction.Issues)
	result.Reduction = reduction
	if err != nil {
		result.Outcome.ValidationState = stage.Status
		return result, err
	}
	result.FanIn = reducedArtifact
	result.Artifacts, err = semanticdiscovery.MaterializePartialMonolithicArtifacts(
		bundle,
		selected,
		result.FanIn,
	)
	if err != nil {
		return result, err
	}
	result.Outcome.addArtifactCoverage(result.Artifacts)
	result.Outcome.ValidationState = "accepted"
	if reduction.DroppedArtifacts > 0 {
		result.Outcome.ValidationState = "accepted_partial"
	}
	return result, nil
}

func semanticDiscoveryVariant(
	strategy string,
	outcome semanticDiscoveryOutcome,
	excludeOpportunity bool,
) semanticDiscoveryVariantMetrics {
	variantOutcome := outcome
	variantOutcome.Strategy = strategy
	if excludeOpportunity {
		variantOutcome.Stages = make([]semanticDiscoveryStageMetrics, 0, len(outcome.Stages))
		for _, stage := range outcome.Stages {
			if stage.Stage != "semantic_discovery_opportunity" {
				variantOutcome.Stages = append(variantOutcome.Stages, stage)
			}
		}
		variantOutcome.recomputeTotals()
	}
	wallMillis := variantOutcome.WallMillis
	if excludeOpportunity && variantOutcome.SynthesisWallMillis > 0 {
		wallMillis = variantOutcome.SynthesisWallMillis
	}
	completeness := 0
	if variantOutcome.SelectedCandidates > 0 {
		completeness = variantOutcome.Artifacts * 100 / variantOutcome.SelectedCandidates
	}
	return semanticDiscoveryVariantMetrics{
		Strategy: strategy, ValidationState: variantOutcome.ValidationState,
		FailureCode:        variantOutcome.FailureCode,
		SelectedCandidates: variantOutcome.SelectedCandidates,
		Artifacts:          variantOutcome.Artifacts, CompletenessPercent: completeness,
		ArtifactKinds: variantOutcome.ArtifactKinds, Verdicts: variantOutcome.Verdicts,
		ClaimBasis: variantOutcome.ClaimBasis, UnsupportedClaims: variantOutcome.UnsupportedClaims,
		LeafReductionIssues:  variantOutcome.LeafReductionIssues,
		FanInReductionIssues: variantOutcome.FanInReductionIssues,
		ProviderCalls:        variantOutcome.SemanticCalls,
		RequestBytes:         variantOutcome.RequestBytes,
		InputTokens:          variantOutcome.InputTokens, OutputTokens: variantOutcome.OutputTokens,
		PromptCacheHitTokens:  variantOutcome.PromptCacheHitTokens,
		PromptCacheMissTokens: variantOutcome.PromptCacheMissTokens,
		ProviderLatencyMillis: variantOutcome.ProviderLatencyMillis,
		WallMillis:            wallMillis,
	}
}

func writeSemanticDiscoveryProgress(writer io.Writer, outcome semanticDiscoveryOutcome) {
	fmt.Fprintf(
		writer,
		"repomap: semantic discovery %s state=%s artifacts=%d leaves=%d/%d calls=%d tokens=%d/%d prompt-cache=%d/%d wall=%s\n",
		outcome.Strategy,
		outcome.ValidationState,
		outcome.Artifacts,
		outcome.LeafSucceeded,
		outcome.LeafTasks,
		outcome.SemanticCalls,
		outcome.InputTokens,
		outcome.OutputTokens,
		outcome.PromptCacheHitTokens,
		outcome.PromptCacheMissTokens,
		time.Duration(outcome.WallMillis)*time.Millisecond,
	)
}
