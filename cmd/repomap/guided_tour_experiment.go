package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/report"
)

const guidedTourComparisonFile = "guided_tour_comparison.json"

const guidedTourPreviousComparisonFile = "guided_tour_comparison.previous.json"

func runGuidedTourExperiment(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("guided tour experiment: provider configuration: %w", err)
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
	comparison, err := prepareGuidedTourExperiment(
		ctx,
		absDir,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
	)
	if err != nil {
		return err
	}
	for _, variant := range comparison.Variants {
		if variant.FailureReason != "" {
			fmt.Fprintf(stderr, "warning: %s variant: %s\n", variant.Strategy, variant.FailureReason)
		}
	}
	fmt.Fprintf(
		stderr,
		"repomap: guided tour experiment saved %d validated variants; human selection remains explicit\n",
		len(comparison.Variants),
	)
	fmt.Printf("Comparison: %s/%s\n", absDir, guidedTourComparisonFile)
	return nil
}

func runGuidedTourFanout(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("guided tour fan-out: provider configuration: %w", err)
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
	data, err := report.ReadRunDir(absDir)
	if err != nil {
		return fmt.Errorf("guided tour fan-out: read saved run: %w", err)
	}
	bundle, err := report.BuildGuidedTourBundle(data)
	if err != nil {
		return fmt.Errorf("guided tour fan-out: build bounded candidates: %w", err)
	}
	outcome, runErr := ensureGuidedTourFanoutExperiment(
		ctx,
		bundle,
		absDir,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
	)
	fmt.Fprintf(
		stderr,
		"repomap: guided fan-out leaves=%d valid=%d missing-only=%d failed=%d calls=%d cache-hits=%d wall=%s\n",
		outcome.LeafTasks,
		outcome.LeafSucceeded,
		outcome.LeafInsufficient,
		outcome.LeafFailed,
		outcome.SemanticCalls,
		outcome.CacheHits,
		time.Duration(outcome.WallMillis)*time.Millisecond,
	)
	return runErr
}

func prepareGuidedTourExperiment(
	ctx context.Context,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
) (guidedtour.Comparison, error) {
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return guidedtour.Comparison{}, fmt.Errorf("guided tour experiment: read saved run: %w", err)
	}
	bundle, err := report.BuildGuidedTourBundle(data)
	if err != nil {
		return guidedtour.Comparison{}, fmt.Errorf("guided tour experiment: build bounded candidates: %w", err)
	}
	return compareGuidedTourStrategies(ctx, bundle, runDir, profile, model, provider)
}

func compareGuidedTourStrategies(
	ctx context.Context,
	bundle guidedtour.Bundle,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
) (guidedtour.Comparison, error) {
	bundleSHA, _, err := guidedtour.BundleHash(bundle)
	if err != nil {
		return guidedtour.Comparison{}, err
	}

	monolithic, monolithicErr := ensureGuidedTourWithOptions(
		ctx,
		bundle,
		runDir,
		profile,
		model,
		provider,
		guidedTourRunOptions{
			independentExperiment: true,
			outputFile:            guidedTourMonolithicFile,
		},
	)
	fanout, fanoutErr := ensureGuidedTourFanoutExperiment(
		ctx,
		bundle,
		runDir,
		profile,
		model,
		provider,
	)
	var monolithicCoverage guidedtour.StoryCoverage
	if monolithicErr == nil {
		monolithicCoverage, monolithicErr = readGuidedTourCoverage(
			bundle,
			filepath.Join(runDir, guidedTourMonolithicFile),
		)
	}
	var fanoutCoverage guidedtour.StoryCoverage
	if fanoutErr == nil {
		fanoutCoverage, fanoutErr = readGuidedTourCoverage(
			bundle,
			filepath.Join(runDir, guidedTourFanoutFile),
		)
	}
	comparison := guidedtour.Comparison{
		Version:      guidedtour.ComparisonVersion,
		BundleSHA256: bundleSHA,
		Model:        model,
		Profile:      profile,
		Variants: []guidedtour.StrategyMetrics{
			guidedTourStrategyMetrics("monolithic", monolithic, monolithicCoverage, monolithicErr),
			guidedTourStrategyMetrics("fan_out_fan_in", fanout, fanoutCoverage, fanoutErr),
		},
	}
	if err := comparison.Validate(); err != nil {
		return guidedtour.Comparison{}, err
	}
	encoded, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return guidedtour.Comparison{}, fmt.Errorf("guided tour experiment: encode comparison: %w", err)
	}
	comparisonBytes := append(encoded, '\n')
	comparisonPath := filepath.Join(runDir, guidedTourComparisonFile)
	if previous, readErr := os.ReadFile(comparisonPath); readErr == nil &&
		!bytes.Equal(previous, comparisonBytes) {
		if err := writeGuidedTourArtifact(
			filepath.Join(runDir, guidedTourPreviousComparisonFile),
			previous,
		); err != nil {
			return guidedtour.Comparison{}, err
		}
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return guidedtour.Comparison{}, fmt.Errorf("guided tour experiment: archive comparison: %w", readErr)
	}
	if err := writeGuidedTourArtifact(comparisonPath, comparisonBytes); err != nil {
		return guidedtour.Comparison{}, err
	}
	if monolithicErr != nil && fanoutErr != nil {
		return comparison, errors.Join(
			fmt.Errorf("monolithic variant: %w", monolithicErr),
			fmt.Errorf("fan-out variant: %w", fanoutErr),
		)
	}
	return comparison, nil
}

func readGuidedTourCoverage(
	bundle guidedtour.Bundle,
	path string,
) (guidedtour.StoryCoverage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return guidedtour.StoryCoverage{}, fmt.Errorf("guided tour experiment: read %s: %w", filepath.Base(path), err)
	}
	story, err := guidedtour.ReplayRecord(bundle, raw)
	if err != nil {
		return guidedtour.StoryCoverage{}, fmt.Errorf("guided tour experiment: replay %s: %w", filepath.Base(path), err)
	}
	coverage, err := guidedtour.EvaluateStoryCoverage(bundle, story)
	if err != nil {
		return guidedtour.StoryCoverage{}, fmt.Errorf("guided tour experiment: cover %s: %w", filepath.Base(path), err)
	}
	return coverage, nil
}

func guidedTourStrategyMetrics(
	strategy string,
	outcome guidedTourOutcome,
	coverage guidedtour.StoryCoverage,
	variantErr error,
) guidedtour.StrategyMetrics {
	validationState := outcome.ValidationState
	if validationState == "" {
		validationState = "failed_before_validation"
	}
	failureReason := ""
	if variantErr != nil {
		failureReason = variantErr.Error()
	}
	return guidedtour.StrategyMetrics{
		Strategy: strategy,
		// Cache hits retain the provider's original token and latency metrics.
		// Calls + hits therefore describes the cold semantic topology, while
		// CacheHits separately exposes work avoided in this replay.
		SemanticCalls:         outcome.SemanticCalls + outcome.CacheHits,
		CacheHits:             outcome.CacheHits,
		RequestBytes:          outcome.RequestBytes,
		ResponseBytes:         outcome.ResponseBytes,
		InputTokens:           outcome.InputTokens,
		OutputTokens:          outcome.OutputTokens,
		PromptCacheHitTokens:  outcome.PromptCacheHitTokens,
		PromptCacheMissTokens: outcome.PromptCacheMissTokens,
		UnsupportedClaims:     outcome.UnsupportedClaims,
		WallMillis:            outcome.WallMillis,
		LeafTasks:             outcome.LeafTasks,
		LeafSucceeded:         outcome.LeafSucceeded,
		LeafInsufficient:      outcome.LeafInsufficient,
		LeafFailed:            outcome.LeafFailed,
		ValidationState:       validationState,
		FailureReason:         failureReason,
		Coverage:              coverage,
	}
}
