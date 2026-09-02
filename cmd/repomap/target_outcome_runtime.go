package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// repositorySelectedTarget projects one adapter-native selection into the
// closed identity that can be shown even when page analysis never completes.
func repositorySelectedTarget(target repositoryTypedTarget) (targetoutcome.SelectedTarget, error) {
	if err := target.Validate(); err != nil {
		return targetoutcome.SelectedTarget{}, err
	}
	return targetoutcome.NewSelectedTargetWithLanguages(
		targetoutcome.LanguageGroup(target.Key.Adapter),
		target.AllowedLanguages,
		target.Scope,
		target.Display,
		target.Selector,
	)
}

func classifyRepositoryTargetFailure(
	stage targetoutcome.Stage,
	err error,
) (targetoutcome.Stage, targetoutcome.Reason) {
	if !stage.Valid() {
		stage = targetoutcome.StageProgramAnalysis
	}
	if errors.Is(err, jstsproject.ErrTypeScriptCompilerUnavailable) {
		return targetoutcome.StageTargetPreparation, targetoutcome.ReasonRequiredToolUnavailable
	}
	var resourceErr *llm.ResourceLimitError
	var reportResourceErr *report.ReportResourceLimitError
	var bundleResourceErr *report.StandaloneTargetBundleResourceLimitError
	if errors.As(err, &resourceErr) || errors.As(err, &reportResourceErr) ||
		errors.As(err, &bundleResourceErr) {
		return stage, targetoutcome.ReasonResourceLimit
	}
	var goSourceErr *surfacediscovery.AnalysisTargetSSAUnavailableError
	if errors.As(err, &goSourceErr) {
		return targetoutcome.StageProgramAnalysis, targetoutcome.ReasonSourceNotAnalyzable
	}
	var providerFailure llm.ProviderFailureSource
	if errors.As(err, &providerFailure) &&
		providerFailure.ProviderFailure().Kind == llm.ProviderFailureResponse {
		return targetoutcome.StageSemanticAnalysis, targetoutcome.ReasonModelResultRejected
	}
	return stage, targetoutcome.ReasonAnalysisFailed
}

func persistTargetOutcomePortfolioForRuns(
	portfolio targetoutcome.Portfolio,
	runs []targetPublishedRun,
) error {
	runDirs := make([]string, 0, len(runs))
	for _, run := range runs {
		runDirs = append(runDirs, run.RunDir)
	}
	return persistTargetOutcomePortfolioForRunDirs(portfolio, runDirs)
}

func persistTargetOutcomePortfolioForRunDirs(
	portfolio targetoutcome.Portfolio,
	runDirs []string,
) error {
	encoded, err := portfolio.CanonicalJSON()
	if err != nil {
		return err
	}
	for _, runDir := range runDirs {
		if runDir == "" {
			return fmt.Errorf("target outcome portfolio: run directory is empty")
		}
		if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("target outcome portfolio: create diagnostic run: %w", err)
		}
		writer, writerErr := debugdump.OpenWriter(runDir, true)
		if writerErr != nil {
			return fmt.Errorf("target outcome portfolio: open run: %w", writerErr)
		}
		writeErr := writer.WriteValidatedFile(
			targetoutcome.ArtifactFilename,
			encoded,
			func(saved []byte) error {
				if !bytes.Equal(saved, encoded) {
					return fmt.Errorf("target outcome portfolio: persisted bytes changed")
				}
				decoded, decodeErr := targetoutcome.Decode(saved)
				if decodeErr != nil {
					return decodeErr
				}
				if decoded.SHA256 != portfolio.SHA256 {
					return fmt.Errorf("target outcome portfolio: persisted authority changed")
				}
				return nil
			},
		)
		closeErr := writer.Close()
		if writeErr != nil {
			return fmt.Errorf("target outcome portfolio: persist run: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("target outcome portfolio: close run: %w", closeErr)
		}
	}
	return nil
}
