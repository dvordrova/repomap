package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

const (
	architectureSynthesisCacheDirectory = ".component-synthesis"
	architectureSynthesisRevision       = "captured-bundle-v1"
)

type componentLandscapeSynthesizer interface {
	ComponentSynthesisPromptJSON(componentmap.SynthesisPrompt) ([]byte, error)
	SynthesizeComponentLandscapeMeasured(context.Context, componentmap.SynthesisPrompt) (modelresearch.ProviderResult, error)
}

type architectureSynthesisOutcome struct {
	Cached                bool
	InputBytes            int
	LatencyMillis         int64
	FallbackReason        componentmap.FallbackReason
	ResponseBytes         int
	Attempted             bool
	ProviderCallSucceeded bool
	ResponseParsed        bool
	ValidationOutcome     componentmap.ValidationOutcome
	ArchitectureSource    componentmap.ArchitectureSource
	ArchitectureLevel     int
	NormalizationCount    int
	FallbackSelected      bool
	InputTokens           int
	OutputTokens          int
}

func synthesizeArchitectureForRun(
	ctx context.Context,
	runDir string,
	stderr io.Writer,
	noCache bool,
) (architectureSynthesisOutcome, error) {
	exchangeWriter, writerErr := debugdump.OpenWriter(runDir, true)
	if writerErr == nil {
		defer exchangeWriter.Close()
		exchangeWriter.SetWarningWriter(stderr)
	} else {
		fmt.Fprintf(
			stderr,
			"warning: semantic exchange journal unavailable: stage=%s code=%s\n",
			debugdump.SemanticStageArchitecture,
			debugdump.SemanticExchangeWarningCode,
		)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	outcome, err := prepareArchitectureSynthesisWithOptions(
		ctx,
		runDir,
		architectureSynthesisRevision,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
		architectureSynthesisOptions{disableCache: noCache, exchangeWriter: exchangeWriter},
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return outcome, ctxErr
	}
	if statusErr := persistArchitectureSynthesisStatus(runDir, outcome, err); statusErr != nil {
		if err != nil {
			return outcome, errors.Join(err, statusErr)
		}
		return outcome, statusErr
	}
	if err != nil {
		return outcome, err
	}
	if outcome.Cached {
		fmt.Fprintf(
			stderr,
			"repomap: reused cached architecture response of %d bytes for a %d-byte request (original call: %d ms, %s)\n",
			outcome.ResponseBytes,
			outcome.InputBytes,
			outcome.LatencyMillis,
			formatTokenUsage(outcome.InputTokens, outcome.OutputTokens),
		)
	} else {
		fmt.Fprintf(
			stderr,
			"repomap: architecture synthesis received %d bytes from a %d-byte request in %d ms (%s)\n",
			outcome.ResponseBytes,
			outcome.InputBytes,
			outcome.LatencyMillis,
			formatTokenUsage(outcome.InputTokens, outcome.OutputTokens),
		)
	}
	if outcome.FallbackReason != "" {
		fmt.Fprintf(stderr, "repomap: architecture synthesis downgraded to local fallback: %s\n", outcome.FallbackReason)
	}
	return outcome, nil
}

func prepareArchitectureSynthesis(
	ctx context.Context,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
) (architectureSynthesisOutcome, error) {
	return prepareArchitectureSynthesisWithOptions(
		ctx, runDir, repositoryRevision, profile, model, provider, architectureSynthesisOptions{},
	)
}

type architectureSynthesisOptions struct {
	disableCache   bool
	exchangeWriter *debugdump.Writer
}

func prepareArchitectureSynthesisWithOptions(
	ctx context.Context,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
	options architectureSynthesisOptions,
) (architectureSynthesisOutcome, error) {
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: read saved run: %w", err)
	}
	if data.ModelResearch != nil {
		context := data.ModelResearch.Repository
		repositoryRevision = context.Revision + ":" + modelresearch.SHA256([]byte(
			context.Identity + "\x00" + context.DirtySHA256 + "\x00" + context.Scenario,
		))[:16]
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: build candidates: %w", err)
	}
	return ensureArchitectureSynthesisWithOptions(
		ctx,
		input.CandidateBundle,
		runDir,
		repositoryRevision,
		profile,
		model,
		provider,
		options,
	)
}

func ensureArchitectureSynthesis(
	ctx context.Context,
	bundle componentmap.CandidateBundle,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
) (architectureSynthesisOutcome, error) {
	return ensureArchitectureSynthesisWithOptions(
		ctx, bundle, runDir, repositoryRevision, profile, model, provider, architectureSynthesisOptions{},
	)
}

func ensureArchitectureSynthesisWithOptions(
	ctx context.Context,
	bundle componentmap.CandidateBundle,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
	options architectureSynthesisOptions,
) (architectureSynthesisOutcome, error) {
	if provider == nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider is required")
	}
	const outputLanguage = "en"
	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	requestJSON, err := provider.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: build provider request: %w", err)
	}
	policy := modelresearch.DefaultPolicy()
	usage := modelresearch.Usage{}
	if state, stateErr := modelresearch.ReadState(runDir); stateErr == nil {
		policy = state.Policy
		usage = state.Usage
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: read research budget: %w", stateErr)
	}
	outcome := architectureSynthesisOutcome{InputBytes: len(requestJSON)}
	if allowed, reason := policy.Allows(policy.Architecture, usage, len(requestJSON)); !allowed {
		outcome.FallbackReason = componentmap.FallbackReason(reason)
		budgetErr := fmt.Errorf("architecture synthesis: %s", reason)
		if recordErr := recordArchitectureResearch(runDir, outcome, "skipped", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(budgetErr, recordErr)
		}
		return outcome, budgetErr
	}
	baseCacheKey, err := componentmap.SynthesisCacheKeyForProviderAndLanguage(
		repositoryRevision,
		bundle,
		profile,
		model,
		outputLanguage,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	cacheKey := baseCacheKey + "-request-" + modelresearch.SHA256(requestJSON)
	runPath := filepath.Join(runDir, report.ArchitectureSynthesisFile)
	cachePath := filepath.Join(
		filepath.Dir(runDir),
		architectureSynthesisCacheDirectory,
		cacheKey+".json",
	)

	if !options.disableCache {
		for _, candidate := range []struct {
			path      string
			copyToRun bool
		}{{path: cachePath, copyToRun: true}} {
			saved, readErr := os.ReadFile(candidate.path)
			if readErr == nil {
				cachedOutcome, replayErr := replayArchitectureSynthesisOutcome(
					bundle,
					repositoryRevision,
					outputLanguage,
					saved,
				)
				if replayErr != nil {
					continue
				}
				var cachedRecord componentmap.SynthesisRecord
				if err := json.Unmarshal(saved, &cachedRecord); err != nil || cachedRecord.Call == nil {
					continue
				}
				cachedOutcome.Cached = true
				cachedOutcome.InputBytes = len(requestJSON)
				recordArchitectureSemanticExchange(
					options.exchangeWriter,
					requestJSON,
					cachedRecord.Call.Response,
					cachedRecord.Call.ResponseState,
					cachedRecord.Call.ResponseBytes,
					0,
					debugdump.SemanticStateCacheHit,
					debugdump.SemanticValidationCache,
				)
				if candidate.copyToRun {
					if err := writeArchitectureSynthesisRecord(runPath, saved); err != nil {
						return architectureSynthesisOutcome{}, err
					}
				}
				if err := recordArchitectureResearch(
					runDir,
					cachedOutcome,
					architectureResearchStatus(cachedOutcome),
					true,
					policy,
					usage,
				); err != nil {
					return architectureSynthesisOutcome{}, err
				}
				return cachedOutcome, nil
			}
			if !errors.Is(readErr, os.ErrNotExist) {
				return architectureSynthesisOutcome{}, fmt.Errorf(
					"architecture synthesis: read saved record %s: %w",
					candidate.path,
					readErr,
				)
			}
		}
	}

	outcome.Attempted = true
	started := time.Now()
	providerResult, err := provider.SynthesizeComponentLandscapeMeasured(ctx, prompt)
	raw := providerResult.Content
	responseBytes := providerResultResponseBytes(providerResult)
	latency := time.Since(started)
	outcome.LatencyMillis = latency.Milliseconds()
	outcome.ResponseBytes = responseBytes
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	if ctxErr := ctx.Err(); ctxErr != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON, raw, componentmap.ResponseCaptured,
			responseBytes, providerResult.Attempts,
			debugdump.SemanticStateCanceled, debugdump.SemanticValidationCanceled,
		)
		return outcome, ctxErr
	}
	if err != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON,
			providerFailureContentForExchange(err, raw), componentmap.ResponseCaptured,
			responseBytes, providerResult.Attempts,
			debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
		)
		callErr := fmt.Errorf("architecture synthesis: provider call: %w", err)
		if isSemanticResourceLimit(callErr) {
			return outcome, callErr
		}
		if recordErr := recordArchitectureResearch(runDir, outcome, "failed", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(callErr, recordErr)
		}
		return outcome, callErr
	}
	result, err := componentmap.RecordSynthesisResponseForLanguage(
		bundle,
		repositoryRevision,
		profile,
		model,
		outputLanguage,
		latency,
		raw,
	)
	if err != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON, raw, componentmap.ResponseCaptured,
			responseBytes, providerResult.Attempts,
			debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse,
		)
		validationErr := fmt.Errorf("architecture synthesis: validate response: %w", err)
		if isSemanticResourceLimit(validationErr) {
			return outcome, validationErr
		}
		if recordErr := recordArchitectureResearch(runDir, outcome, "rejected", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(validationErr, recordErr)
		}
		return outcome, validationErr
	}
	result.Record.Call.Metadata.InputTokens = providerResult.InputTokens
	result.Record.Call.Metadata.OutputTokens = providerResult.OutputTokens
	outcome = architectureSynthesisOutcome{
		InputBytes:            len(requestJSON),
		LatencyMillis:         result.Record.Call.Metadata.LatencyMillis,
		FallbackReason:        result.Landscape.FallbackReason,
		ResponseBytes:         responseBytes,
		Attempted:             true,
		ProviderCallSucceeded: true,
		ResponseParsed:        architectureResponseParsed(result.Landscape),
		ValidationOutcome:     result.Landscape.ValidationOutcome,
		ArchitectureSource:    result.Landscape.Source,
		ArchitectureLevel:     result.Landscape.Level,
		NormalizationCount:    len(result.Landscape.Normalizations),
		FallbackSelected:      result.Landscape.Fallback,
		InputTokens:           providerResult.InputTokens,
		OutputTokens:          providerResult.OutputTokens,
	}
	state := debugdump.SemanticStateAccepted
	validationCode := debugdump.SemanticValidationAccepted
	if result.Landscape.ValidationOutcome == componentmap.ValidationRejected {
		state = debugdump.SemanticStateRejected
		validationCode = debugdump.SemanticValidationResponse
	}
	recordArchitectureSemanticExchange(
		options.exchangeWriter,
		requestJSON,
		raw,
		result.Record.Call.ResponseState,
		result.Record.Call.ResponseBytes,
		providerResult.Attempts,
		state,
		validationCode,
	)
	saved, err := json.MarshalIndent(result.Record, "", "  ")
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: encode record: %w", err)
	}
	saved = append(saved, '\n')
	if !options.disableCache {
		if err := writeArchitectureSynthesisRecord(cachePath, saved); err != nil {
			return architectureSynthesisOutcome{}, err
		}
	}
	if err := writeArchitectureSynthesisRecord(runPath, saved); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	if err := recordArchitectureResearch(
		runDir,
		outcome,
		architectureResearchStatus(outcome),
		false,
		policy,
		usage,
	); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	return outcome, nil
}

func recordArchitectureSemanticExchange(
	writer *debugdump.Writer,
	request []byte,
	response []byte,
	responseState componentmap.ResponseState,
	responseBytes int,
	transportAttempts int,
	state string,
	validationCode string,
) {
	if writer == nil {
		return
	}
	semanticCalls := 1
	if state == debugdump.SemanticStateCacheHit {
		semanticCalls = 0
		transportAttempts = 0
	}
	exchange := debugdump.SemanticExchange{
		Stage:                  debugdump.SemanticStageArchitecture,
		InstanceOrdinal:        1,
		SemanticAttemptOrdinal: 1,
		RequestProvenance:      debugdump.SemanticRequestPrepared,
		State:                  state,
		ValidationCode:         validationCode,
		SemanticCalls:          semanticCalls,
		TransportAttempts:      transportAttempts,
		Request:                request,
		Response:               response,
	}
	if len(response) == 0 {
		unavailableCode := debugdump.SemanticUnavailableNoContent
		if state == debugdump.SemanticStateCanceled {
			unavailableCode = debugdump.SemanticUnavailableCanceled
		} else if state == debugdump.SemanticStateCacheHit {
			unavailableCode = debugdump.SemanticUnavailableCache
			if responseState == componentmap.ResponseOversize ||
				responseState == componentmap.ResponseSensitiveOmitted {
				unavailableCode = debugdump.SemanticUnavailableOmitted
			}
		}
		exchange.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code:          unavailableCode,
			OriginalBytes: responseBytes,
		}
	}
	writer.RecordSemanticExchange(exchange)
}

func recordArchitectureResearch(
	runDir string,
	outcome architectureSynthesisOutcome,
	status string,
	cached bool,
	policy modelresearch.Policy,
	usage modelresearch.Usage,
) error {
	state, err := modelresearch.ReadState(runDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	state.Architecture = modelresearch.StageMetrics{
		Stage: "architecture_synthesis", Status: status,
		RequestBytes: outcome.InputBytes, ResponseBytes: outcome.ResponseBytes,
		LatencyMillis: outcome.LatencyMillis, CacheHit: cached,
		InputTokens: outcome.InputTokens, OutputTokens: outcome.OutputTokens,
	}
	if outcome.Attempted && !cached {
		state.Architecture.SemanticCalls = 1
		state.Usage.SemanticCalls = usage.SemanticCalls + 1
		state.Usage.RequestBytes = usage.RequestBytes + outcome.InputBytes
	}
	state.Policy = policy
	return modelresearch.WriteState(runDir, state)
}

func architectureSynthesisDiagnosticCodes(diagnostics []componentmap.Diagnostic) []string {
	const maxCodes = 4

	codes := make([]string, 0, min(len(diagnostics), maxCodes))
	seen := make(map[string]struct{}, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "" {
			continue
		}
		if _, exists := seen[diagnostic.Code]; exists {
			continue
		}
		seen[diagnostic.Code] = struct{}{}
		codes = append(codes, diagnostic.Code)
		if len(codes) == maxCodes {
			break
		}
	}
	return codes
}

func architectureResearchStatus(outcome architectureSynthesisOutcome) string {
	switch outcome.ValidationOutcome {
	case componentmap.ValidationAcceptedNormalized:
		return "normalized"
	case componentmap.ValidationAccepted:
		return "accepted"
	case componentmap.ValidationRejected:
		return "rejected_fallback"
	default:
		return "completed"
	}
}

func architectureResponseParsed(landscape componentmap.Landscape) bool {
	for _, diagnostic := range landscape.Diagnostics {
		if diagnostic.Severity == componentmap.FindingFatal && strings.HasPrefix(diagnostic.Code, "response.") {
			return false
		}
	}
	return true
}

func replayArchitectureSynthesisOutcome(
	bundle componentmap.CandidateBundle,
	repositoryRevision string,
	outputLanguage string,
	saved []byte,
) (architectureSynthesisOutcome, error) {
	landscape, err := componentmap.ReplaySynthesis(bundle, repositoryRevision, saved)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(saved, &record); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	if record.Call == nil || record.Call.Metadata.OutputLanguage != outputLanguage {
		return architectureSynthesisOutcome{}, fmt.Errorf(
			"architecture synthesis: saved record output language does not match active request",
		)
	}
	return architectureSynthesisOutcome{
		InputBytes:            record.Call.Metadata.InputBytes,
		LatencyMillis:         record.Call.Metadata.LatencyMillis,
		FallbackReason:        landscape.FallbackReason,
		ResponseBytes:         record.Call.ResponseBytes,
		ProviderCallSucceeded: true,
		ResponseParsed:        architectureResponseParsed(landscape),
		ValidationOutcome:     landscape.ValidationOutcome,
		ArchitectureSource:    landscape.Source,
		ArchitectureLevel:     landscape.Level,
		NormalizationCount:    len(landscape.Normalizations),
		FallbackSelected:      landscape.Fallback,
		InputTokens:           record.Call.Metadata.InputTokens,
		OutputTokens:          record.Call.Metadata.OutputTokens,
	}, nil
}

func architectureSynthesisStatus(
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) report.ArchitectureSynthesisStatus {
	status := report.ArchitectureSynthesisStatus{
		Version:               report.ArchitectureSynthesisStatusVersion,
		PromptBytes:           outcome.InputBytes,
		LatencyMillis:         outcome.LatencyMillis,
		ProviderCallSucceeded: outcome.ProviderCallSucceeded,
		ResponseParsed:        outcome.ResponseParsed,
		ProposalAccepted:      outcome.ValidationOutcome == componentmap.ValidationAccepted || outcome.ValidationOutcome == componentmap.ValidationAcceptedNormalized,
		ProposalNormalized:    outcome.ValidationOutcome == componentmap.ValidationAcceptedNormalized,
		ProposalRejected:      outcome.ValidationOutcome == componentmap.ValidationRejected,
		FallbackSelected:      outcome.FallbackSelected,
		ArchitectureSource:    string(outcome.ArchitectureSource),
		ArchitectureLevel:     outcome.ArchitectureLevel,
		NormalizationCount:    outcome.NormalizationCount,
		FallbackReason:        string(outcome.FallbackReason),
	}
	if synthesisErr == nil {
		if outcome.Cached {
			status.State = report.ArchitectureSynthesisCached
		} else {
			status.State = report.ArchitectureSynthesisSucceeded
			status.ProviderRequestCount = 1
		}
		return status
	}

	status.State = report.ArchitectureSynthesisFailed
	if outcome.Attempted {
		status.ProviderRequestCount = 1
	}
	message := synthesisErr.Error()
	switch {
	case strings.Contains(message, "response content is empty"):
		status.ErrorCode = "empty_response"
	case strings.Contains(message, "unusable"), strings.Contains(message, "validate response"):
		status.ErrorCode = "invalid_response"
	default:
		status.ErrorCode = "provider_error"
	}
	return status
}

func persistArchitectureSynthesisStatus(
	runDir string,
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) error {
	if isSemanticResourceLimit(synthesisErr) {
		return nil
	}
	return writeArchitectureSynthesisStatus(
		runDir,
		architectureSynthesisStatus(outcome, synthesisErr),
	)
}

func writeArchitectureSynthesisStatus(
	runDir string,
	status report.ArchitectureSynthesisStatus,
) error {
	if err := status.Validate(); err != nil {
		return fmt.Errorf("architecture synthesis: status: %w", err)
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("architecture synthesis: encode status: %w", err)
	}
	data = append(data, '\n')
	return writeArchitectureSynthesisRecord(
		filepath.Join(runDir, report.ArchitectureSynthesisStatusFile),
		data,
	)
}

func writeArchitectureSynthesisRecord(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("architecture synthesis: create record directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".repomap-architecture-synthesis-")
	if err != nil {
		return fmt.Errorf("architecture synthesis: create temporary record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("architecture synthesis: protect temporary record: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("architecture synthesis: write temporary record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("architecture synthesis: close temporary record: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("architecture synthesis: replace saved record: %w", err)
	}
	return nil
}
