package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	architectureSynthesisCacheContract  = "architecture-external-cache-v3"
)

var errArchitectureSynthesisRejected = errors.New(
	"architecture synthesis: provider proposal rejected by local validation",
)

// architectureResponseRejected marks a provider response that failed only the
// closed semantic Architecture contract. It is intentionally not the marker
// consumed by main: publication may continue only after the failed status has
// itself been durably written.
type architectureResponseRejected struct {
	cause error
}

func (rejected *architectureResponseRejected) Error() string {
	if rejected == nil || rejected.cause == nil {
		return errArchitectureSynthesisRejected.Error()
	}
	return rejected.cause.Error()
}

func (rejected *architectureResponseRejected) Unwrap() error {
	if rejected == nil {
		return nil
	}
	return rejected.cause
}

func (rejected *architectureResponseRejected) Is(target error) bool {
	return target == errArchitectureSynthesisRejected
}

type architectureProviderCallFailed struct {
	cause error
}

func (failure *architectureProviderCallFailed) Error() string {
	if failure == nil || failure.cause == nil {
		return "architecture synthesis: provider call failed"
	}
	return failure.cause.Error()
}

func (failure *architectureProviderCallFailed) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

type publishableArchitectureFailure struct {
	cause error
}

func (failure *publishableArchitectureFailure) Error() string {
	if failure == nil || failure.cause == nil {
		return errArchitectureSynthesisRejected.Error()
	}
	return failure.cause.Error()
}

func (failure *publishableArchitectureFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func isPublishableArchitectureFailure(err error) bool {
	var failure *publishableArchitectureFailure
	return errors.As(err, &failure)
}

// architectureOutputResourceExhausted marks an attempted Architecture provider
// output/response resource exhaustion (Decision 215). It preserves the
// underlying ResourceLimitError via Unwrap so classification can extract exact
// bounded evidence, and it becomes publishable only after the failed status
// and model-research accounting are durable (see
// persistAndClassifyArchitectureSynthesisStatus). Pre-call resource limits
// never take this type.
type architectureOutputResourceExhausted struct {
	cause error
}

func (failure *architectureOutputResourceExhausted) Error() string {
	if failure == nil || failure.cause == nil {
		return "architecture synthesis: provider output resource exhausted"
	}
	return failure.cause.Error()
}

func (failure *architectureOutputResourceExhausted) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func isArchitectureOutputResourceExhausted(err error) bool {
	var failure *architectureOutputResourceExhausted
	return errors.As(err, &failure)
}

// classifyArchitectureOutputResourceExhaustion records the attempted
// Architecture call exactly once in model-research state (Decision 215 C) and
// returns the typed publishable failure. An accounting-write failure joins the
// original resource error and remains terminal. The caller has already
// attempted the provider call, so this never classifies a pre-call rejection.
func classifyArchitectureOutputResourceExhaustion(
	runDir string,
	outcome architectureSynthesisOutcome,
	policy modelresearch.Policy,
	usage modelresearch.Usage,
	cause error,
) error {
	if recordErr := recordArchitectureResearch(runDir, outcome, "resource_limited", false, policy, usage); recordErr != nil {
		return errors.Join(cause, recordErr)
	}
	return &architectureOutputResourceExhausted{cause: cause}
}

type componentLandscapeSynthesizer interface {
	ComponentSynthesisPromptJSON(componentmap.SynthesisPrompt) ([]byte, error)
	SynthesizeComponentLandscapeBodyMeasured(context.Context, []byte) (modelresearch.ProviderResult, error)
}

type architectureSynthesisOutcome struct {
	Cached                   bool
	InputBytes               int
	LatencyMillis            int64
	FallbackReason           componentmap.FallbackReason
	ResponseBytes            int
	ResponseContentBytes     int
	Attempted                bool
	TransportAttempts        int
	ProviderCallSucceeded    bool
	ResponseParsed           bool
	ValidationOutcome        componentmap.ValidationOutcome
	ArchitectureSource       componentmap.ArchitectureSource
	ArchitectureLevel        int
	NormalizationCount       int
	FallbackSelected         bool
	InputTokens              int
	OutputTokens             int
	UsageReported            bool
	FinishReason             string
	ResponseComplete         bool
	ResponseState            componentmap.ResponseState
	LocalCandidateCount      int
	RequestedConceptualCount int
	StructuralLocatorCount   int
	AnchorCount              int
	MembershipCounted        bool
	MemberOccurrences        int
	DistinctMembers          int
	CoveredConceptualCount   int
	UncoveredConceptualCount int
	UncoveredConceptualIDs   []componentmap.MemberID
	ValidationCodes          []string
}

func synthesizeArchitectureForRun(
	ctx context.Context,
	runDir string,
	authority report.RunAuthority,
	output *runOutput,
	noCache bool,
	outputLanguage string,
) (architectureSynthesisOutcome, error) {
	if output == nil {
		output = newRunOutput(nil)
	}
	output.Stage("Architecture", "synthesizing bounded conceptual grouping")
	bundle, repositoryRevision, err := prepareArchitectureSynthesisInput(
		runDir,
		architectureSynthesisRevision,
		&authority,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, persistAndClassifyArchitectureSynthesisStatus(
			runDir,
			architectureSynthesisOutcome{},
			err,
		)
	}
	exchangeWriter, writerErr := debugdump.OpenWriter(runDir, true)
	if writerErr == nil {
		defer exchangeWriter.Close()
		exchangeWriter.SetWarningWriter(runOutputWarningSink{
			output: output, summary: "Architecture semantic exchange journal unavailable",
		})
	} else {
		output.Warn(
			"semantic exchange journal unavailable",
			"stage: "+debugdump.SemanticStageArchitecture,
			"code: "+debugdump.SemanticExchangeWarningCode,
		)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		stageErr := fmt.Errorf("architecture synthesis: provider configuration: %w", err)
		if statusErr := persistArchitectureSynthesisStatus(
			runDir, architectureSynthesisOutcome{}, stageErr,
		); statusErr != nil {
			return architectureSynthesisOutcome{}, errors.Join(stageErr, statusErr)
		}
		return architectureSynthesisOutcome{}, stageErr
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		output.Stage(
			"Architecture",
			progress.Stage+" is still running",
			"elapsed: "+progress.Elapsed.Round(time.Second).String(),
			"Ctrl-C to cancel",
		)
	}
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(client.Endpoint)
	if err != nil {
		stageErr := fmt.Errorf("architecture synthesis: provider cache identity: %w", err)
		if statusErr := persistArchitectureSynthesisStatus(
			runDir, architectureSynthesisOutcome{}, stageErr,
		); statusErr != nil {
			return architectureSynthesisOutcome{}, errors.Join(stageErr, statusErr)
		}
		return architectureSynthesisOutcome{}, stageErr
	}
	outcome, err := ensureArchitectureSynthesisWithOptions(
		ctx,
		bundle,
		runDir,
		repositoryRevision,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
		architectureSynthesisOptions{
			disableCache: noCache, exchangeWriter: exchangeWriter,
			providerEndpointSHA256: endpointSHA, outputLanguage: outputLanguage,
			runAuthority: &authority,
		},
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return outcome, ctxErr
	}
	err = persistAndClassifyArchitectureSynthesisStatus(runDir, outcome, err)
	if err != nil {
		if errors.Is(err, errArchitectureSynthesisRejected) {
			output.Warn(
				"Architecture proposal rejected",
				"state: failed",
				"validation: response",
				"the exact local Architecture Canvas remains available",
			)
		}
		return outcome, err
	}
	state := "accepted"
	if outcome.ValidationOutcome == componentmap.ValidationAcceptedPartial {
		state = "accepted_partial"
	}
	if outcome.Cached {
		state = "cached"
		if outcome.ValidationOutcome == componentmap.ValidationAcceptedPartial {
			state = "cached_partial"
		}
	}
	output.State(
		"Architecture",
		state,
		fmt.Sprintf("request bytes: %d", outcome.InputBytes),
		fmt.Sprintf("response bytes: %d", outcome.ResponseBytes),
		formatRunOutputTokens(outcome.InputTokens, outcome.OutputTokens),
		formatRunOutputDuration(outcome.LatencyMillis),
	)
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
	endpointSHA := architectureSynthesisProviderEndpointSHA256(provider)
	return prepareArchitectureSynthesisWithOptions(
		ctx, runDir, repositoryRevision, profile, model, provider,
		architectureSynthesisOptions{providerEndpointSHA256: endpointSHA},
	)
}

type architectureSynthesisOptions struct {
	disableCache           bool
	exchangeWriter         *debugdump.Writer
	providerEndpointSHA256 string
	outputLanguage         string
	runAuthority           *report.RunAuthority
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
	bundle, repositoryRevision, err := prepareArchitectureSynthesisInput(
		runDir,
		repositoryRevision,
		options.runAuthority,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	return ensureArchitectureSynthesisWithOptions(
		ctx,
		bundle,
		runDir,
		repositoryRevision,
		profile,
		model,
		provider,
		options,
	)
}

func prepareArchitectureSynthesisInput(
	runDir string,
	repositoryRevision string,
	runAuthority *report.RunAuthority,
) (componentmap.CandidateBundle, string, error) {
	var (
		data *report.ReportData
		err  error
	)
	if runAuthority == nil {
		data, err = report.ReadRunDir(runDir)
	} else {
		data, err = report.ReadRunDirForAuthorizedArchitecture(
			runDir,
			*runAuthority,
		)
	}
	if err != nil {
		return componentmap.CandidateBundle{}, "", fmt.Errorf(
			"architecture synthesis: read saved run: %w",
			err,
		)
	}
	if data.ModelResearch != nil {
		context := data.ModelResearch.Repository
		repositoryRevision = context.Revision + ":" + modelresearch.SHA256([]byte(
			context.Identity + "\x00" + context.DirtySHA256 + "\x00" + context.Scenario,
		))[:16]
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		var limitErr *componentmap.CandidateBundleLimitError
		if errors.As(err, &limitErr) {
			return componentmap.CandidateBundle{}, "", fmt.Errorf(
				"architecture synthesis: build candidates: %w",
				modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
					Stage:         "architecture_input_" + string(limitErr.Kind),
					Kind:          modelresearch.ResourceLimitCatalogItems,
					Limit:         limitErr.Limit,
					Observed:      limitErr.Observed,
					ObservedKnown: true,
				}, nil),
			)
		}
		return componentmap.CandidateBundle{}, "", fmt.Errorf(
			"architecture synthesis: build candidates: %w",
			err,
		)
	}
	return input.CandidateBundle, repositoryRevision, nil
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
	endpointSHA := architectureSynthesisProviderEndpointSHA256(provider)
	return ensureArchitectureSynthesisWithOptions(
		ctx, bundle, runDir, repositoryRevision, profile, model, provider,
		architectureSynthesisOptions{providerEndpointSHA256: endpointSHA},
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
	if !modelresearch.IsSHA256(options.providerEndpointSHA256) {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider endpoint identity is required")
	}
	outputLanguage, err := normalizeArchitectureSynthesisOutputLanguage(options.outputLanguage)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	runPath := filepath.Join(runDir, report.ArchitectureSynthesisFile)
	if err := removeArchitectureSynthesisRunRecord(runPath); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	prompt, err := componentmap.BuildSynthesisPromptForLanguage(bundle, outputLanguage)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	requestJSON, err := provider.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: build provider request: %w", err)
	}
	providerIdentity := componentmap.SynthesisProviderIdentity{
		RequestSHA256:  modelresearch.SHA256(requestJSON),
		EndpointSHA256: options.providerEndpointSHA256,
	}
	policy := modelresearch.DefaultPolicy()
	usage := modelresearch.Usage{}
	if state, stateErr := modelresearch.ReadState(runDir); stateErr == nil {
		policy = state.Policy
		usage = state.Usage
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: read research budget: %w", stateErr)
	}
	conceptualCount, structuralLocatorCount := bundle.CandidateRoleCounts()
	outcome := architectureSynthesisOutcome{
		InputBytes:               len(requestJSON),
		LocalCandidateCount:      len(bundle.Candidates),
		RequestedConceptualCount: conceptualCount,
		StructuralLocatorCount:   structuralLocatorCount,
		AnchorCount:              len(bundle.BehaviorAnchors),
	}
	if allowed, reason := policy.Allows(policy.Architecture, usage, len(requestJSON)); !allowed {
		outcome.FallbackReason = componentmap.FallbackReason(reason)
		budgetErr := architectureSynthesisBudgetError(reason, policy, usage, len(requestJSON))
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
	cacheKey, err := architectureSynthesisExternalCacheKey(
		baseCacheKey,
		providerIdentity.EndpointSHA256,
		providerIdentity.RequestSHA256,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
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
					providerIdentity,
					saved,
				)
				if replayErr != nil {
					if isSemanticResourceLimit(replayErr) {
						return outcome, fmt.Errorf(
							"architecture synthesis: cached response resource limit: %w",
							replayErr,
						)
					}
					if removeErr := os.Remove(candidate.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: remove rejected cache: %w", removeErr)
					}
					continue
				}
				if cachedOutcome.FallbackSelected ||
					(cachedOutcome.ValidationOutcome != componentmap.ValidationAccepted &&
						cachedOutcome.ValidationOutcome != componentmap.ValidationAcceptedNormalized &&
						cachedOutcome.ValidationOutcome != componentmap.ValidationAcceptedPartial) {
					if removeErr := os.Remove(candidate.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: remove rejected cache: %w", removeErr)
					}
					continue
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return outcome, ctxErr
				}
				var cachedRecord componentmap.SynthesisRecord
				if err := json.Unmarshal(saved, &cachedRecord); err != nil || cachedRecord.Call == nil {
					continue
				}
				cachedOutcome.Cached = true
				cachedOutcome.InputBytes = len(requestJSON)
				cachedOutcome.LocalCandidateCount = len(bundle.Candidates)
				cachedOutcome.RequestedConceptualCount = conceptualCount
				cachedOutcome.StructuralLocatorCount = structuralLocatorCount
				cachedOutcome.AnchorCount = len(bundle.BehaviorAnchors)
				if architectureSynthesisAcceptedEvidenceCode(cachedOutcome) != "" {
					if removeErr := os.Remove(candidate.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: remove incomplete cache evidence: %w", removeErr)
					}
					continue
				}
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
					if candidate.copyToRun {
						_ = removeArchitectureSynthesisRunRecord(runPath)
					}
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
	providerResult, err := provider.SynthesizeComponentLandscapeBodyMeasured(ctx, requestJSON)
	raw := providerResult.Content
	providerResponseBytes := providerResultResponseBytes(providerResult)
	latency := time.Since(started)
	outcome.LatencyMillis = latency.Milliseconds()
	outcome.ResponseBytes = len(raw)
	outcome.ResponseContentBytes = len(raw)
	outcome.TransportAttempts = providerResult.Attempts
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.UsageReported = providerResult.UsageReported
	outcome.FinishReason = providerResult.FinishReason
	outcome.ResponseComplete = providerResult.FinishReason == "stop"
	if ctxErr := ctx.Err(); ctxErr != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON, raw, componentmap.ResponseCaptured,
			providerResponseBytes, providerResult.Attempts,
			debugdump.SemanticStateCanceled, debugdump.SemanticValidationCanceled,
		)
		return outcome, ctxErr
	}
	if err != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON,
			providerFailureContentForExchange(err, raw), componentmap.ResponseCaptured,
			providerResponseBytes, providerResult.Attempts,
			debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
		)
		callErr := fmt.Errorf("architecture synthesis: provider call: %w", err)
		if isSemanticResourceLimit(callErr) {
			return outcome, classifyArchitectureOutputResourceExhaustion(
				runDir, outcome, policy, usage, callErr,
			)
		}
		if recordErr := recordArchitectureResearch(runDir, outcome, "failed", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(callErr, recordErr)
		}
		return outcome, &architectureProviderCallFailed{cause: callErr}
	}
	result, err := componentmap.RecordSynthesisResponseForLanguageAndProvider(
		bundle,
		repositoryRevision,
		profile,
		model,
		outputLanguage,
		providerIdentity,
		latency,
		raw,
	)
	if err != nil {
		recordArchitectureSemanticExchange(
			options.exchangeWriter, requestJSON, raw, componentmap.ResponseCaptured,
			providerResponseBytes, providerResult.Attempts,
			debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse,
		)
		validationErr := fmt.Errorf("architecture synthesis: validate response: %w", err)
		if isSemanticResourceLimit(validationErr) {
			return outcome, classifyArchitectureOutputResourceExhaustion(
				runDir, outcome, policy, usage, validationErr,
			)
		}
		if recordErr := recordArchitectureResearch(runDir, outcome, "rejected", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(validationErr, recordErr)
		}
		return outcome, &architectureResponseRejected{cause: validationErr}
	}
	result.Record.Call.Metadata.InputTokens = providerResult.InputTokens
	result.Record.Call.Metadata.OutputTokens = providerResult.OutputTokens
	result.Record.Call.Metadata.UsageReported = providerResult.UsageReported
	result.Record.Call.Metadata.FinishReason = providerResult.FinishReason
	result.Record.Call.Metadata.TransportAttempts = providerResult.Attempts
	result.Record.Call.Metadata.ResponseComplete = providerResult.FinishReason == "stop"
	outcome = architectureSynthesisOutcome{
		InputBytes:               len(requestJSON),
		LatencyMillis:            result.Record.Call.Metadata.LatencyMillis,
		FallbackReason:           result.Landscape.FallbackReason,
		ResponseBytes:            result.Record.Call.ResponseBytes,
		ResponseContentBytes:     len(result.Record.Call.Response),
		Attempted:                true,
		TransportAttempts:        providerResult.Attempts,
		ProviderCallSucceeded:    true,
		ResponseParsed:           architectureResponseParsed(result.Landscape),
		ValidationOutcome:        result.Landscape.ValidationOutcome,
		ArchitectureSource:       result.Landscape.Source,
		ArchitectureLevel:        result.Landscape.Level,
		NormalizationCount:       len(result.Landscape.Normalizations),
		FallbackSelected:         result.Landscape.Fallback,
		InputTokens:              providerResult.InputTokens,
		OutputTokens:             providerResult.OutputTokens,
		UsageReported:            providerResult.UsageReported,
		FinishReason:             providerResult.FinishReason,
		ResponseComplete:         providerResult.FinishReason == "stop",
		ResponseState:            result.Record.Call.ResponseState,
		LocalCandidateCount:      len(bundle.Candidates),
		RequestedConceptualCount: conceptualCount,
		StructuralLocatorCount:   structuralLocatorCount,
		AnchorCount:              len(bundle.BehaviorAnchors),
		ValidationCodes:          architectureSynthesisDiagnosticCodes(result.Landscape.Diagnostics),
		MembershipCounted:        result.Membership.Counted,
		MemberOccurrences:        result.Membership.MemberOccurrences,
		DistinctMembers:          result.Membership.DistinctMembers,
		CoveredConceptualCount:   len(result.Membership.CoveredMemberIDs),
		UncoveredConceptualCount: len(result.Membership.UncoveredMemberIDs),
		UncoveredConceptualIDs:   append([]componentmap.MemberID(nil), result.Membership.UncoveredMemberIDs...),
	}
	accepted := !result.Landscape.Fallback &&
		(result.Landscape.ValidationOutcome == componentmap.ValidationAccepted ||
			result.Landscape.ValidationOutcome == componentmap.ValidationAcceptedNormalized ||
			result.Landscape.ValidationOutcome == componentmap.ValidationAcceptedPartial)
	if accepted {
		if evidenceCode := architectureSynthesisAcceptedEvidenceCode(outcome); evidenceCode != "" {
			accepted = false
			outcome.ValidationOutcome = componentmap.ValidationRejected
			outcome.FallbackSelected = false
			outcome.FallbackReason = ""
			outcome.ValidationCodes = prependArchitectureSynthesisDiagnosticCode(
				outcome.ValidationCodes,
				evidenceCode,
			)
		}
	}
	state := debugdump.SemanticStateRejected
	validationCode := debugdump.SemanticValidationResponse
	if accepted {
		state = debugdump.SemanticStateAccepted
		validationCode = debugdump.SemanticValidationAccepted
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
	if !accepted {
		if recordErr := recordArchitectureResearch(runDir, outcome, "rejected", false, policy, usage); recordErr != nil {
			return outcome, errors.Join(
				fmt.Errorf("architecture synthesis: provider proposal rejected"),
				recordErr,
			)
		}
		return outcome, &architectureResponseRejected{cause: errArchitectureSynthesisRejected}
	}
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
		_ = removeArchitectureSynthesisRunRecord(runPath)
		return architectureSynthesisOutcome{}, err
	}
	return outcome, nil
}

func architectureSynthesisBudgetError(
	reason string,
	policy modelresearch.Policy,
	usage modelresearch.Usage,
	requestBytes int,
) error {
	details := modelresearch.ResourceLimitError{Stage: "architecture_synthesis"}
	switch reason {
	case "stage_byte_budget_exhausted":
		details.Kind = modelresearch.ResourceLimitRequestBytes
		details.Limit = policy.Architecture.MaxRequestBytes
		details.Observed = requestBytes
		details.ObservedKnown = true
	case "total_byte_budget_exhausted":
		details.Kind = modelresearch.ResourceLimitRequestBytes
		details.Limit = policy.MaxTotalRequestBytes
		details.Observed = usage.RequestBytes + requestBytes
		details.ObservedKnown = true
	case "call_budget_exhausted":
		details.Kind = modelresearch.ResourceLimitSemanticCalls
		details.Limit = policy.MaxSemanticCalls
		details.Observed = usage.SemanticCalls
		details.ObservedKnown = true
	default:
		return fmt.Errorf("architecture synthesis: unsupported budget outcome %q", reason)
	}
	return modelresearch.NewResourceLimitError(details, nil)
}

type architectureSynthesisCacheIdentity interface {
	ArchitectureProviderEndpointSHA256() string
}

func architectureSynthesisProviderEndpointSHA256(provider componentLandscapeSynthesizer) string {
	owner, ok := provider.(architectureSynthesisCacheIdentity)
	if !ok {
		return ""
	}
	return owner.ArchitectureProviderEndpointSHA256()
}

func architectureSynthesisExternalCacheKey(baseKey, endpointSHA, requestSHA string) (string, error) {
	if strings.TrimSpace(baseKey) == "" || !modelresearch.IsSHA256(endpointSHA) || !modelresearch.IsSHA256(requestSHA) {
		return "", fmt.Errorf("architecture synthesis: incomplete external cache identity")
	}
	identity, err := json.Marshal(struct {
		Contract       string `json:"contract"`
		BaseKey        string `json:"base_key"`
		EndpointSHA256 string `json:"endpoint_sha256"`
		RequestSHA256  string `json:"request_sha256"`
	}{
		Contract: architectureSynthesisCacheContract, BaseKey: baseKey,
		EndpointSHA256: endpointSHA, RequestSHA256: requestSHA,
	})
	if err != nil {
		return "", fmt.Errorf("architecture synthesis: encode external cache identity: %w", err)
	}
	return "architecture-cache-" + modelresearch.SHA256(identity), nil
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
		RequestProvenance:      debugdump.SemanticRequestExactSent,
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
	if state == debugdump.SemanticStateCacheHit {
		exchange.RequestProvenance = debugdump.SemanticRequestPrepared
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
	codes := make([]string, 0, len(diagnostics))
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
	}
	return codes
}

func architectureSynthesisAcceptedEvidenceCode(outcome architectureSynthesisOutcome) string {
	switch {
	case outcome.ResponseState != componentmap.ResponseCaptured:
		return "response.not_captured"
	case !outcome.ResponseComplete:
		return "response.incomplete"
	case !outcome.MembershipCounted || outcome.MemberOccurrences == 0:
		return "response.membership_unavailable"
	}
	if err := architectureSynthesisStatus(outcome, nil).Validate(); err != nil {
		return "status.invalid_evidence"
	}
	return ""
}

func prependArchitectureSynthesisDiagnosticCode(codes []string, code string) []string {
	result := make([]string, 0, len(codes)+1)
	result = append(result, code)
	for _, candidate := range codes {
		if candidate == "" || candidate == code {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func architectureResearchStatus(outcome architectureSynthesisOutcome) string {
	switch outcome.ValidationOutcome {
	case componentmap.ValidationAcceptedPartial:
		return "accepted_partial"
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
	providerIdentity componentmap.SynthesisProviderIdentity,
	saved []byte,
) (architectureSynthesisOutcome, error) {
	result, err := componentmap.ReplaySynthesisResultForProvider(
		bundle,
		repositoryRevision,
		providerIdentity,
		saved,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	landscape := result.Landscape
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(saved, &record); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	if record.Call == nil || record.Call.Metadata.OutputLanguage != outputLanguage {
		return architectureSynthesisOutcome{}, fmt.Errorf(
			"architecture synthesis: saved record output language does not match active request",
		)
	}
	conceptualCount, structuralLocatorCount := bundle.CandidateRoleCounts()
	return architectureSynthesisOutcome{
		InputBytes:               record.Call.Metadata.InputBytes,
		LatencyMillis:            record.Call.Metadata.LatencyMillis,
		FallbackReason:           landscape.FallbackReason,
		ProviderCallSucceeded:    true,
		ResponseParsed:           architectureResponseParsed(landscape),
		ValidationOutcome:        landscape.ValidationOutcome,
		ArchitectureSource:       landscape.Source,
		ArchitectureLevel:        landscape.Level,
		NormalizationCount:       len(landscape.Normalizations),
		FallbackSelected:         landscape.Fallback,
		UsageReported:            record.Call.Metadata.UsageReported,
		InputTokens:              record.Call.Metadata.InputTokens,
		OutputTokens:             record.Call.Metadata.OutputTokens,
		FinishReason:             record.Call.Metadata.FinishReason,
		ResponseComplete:         record.Call.Metadata.ResponseComplete,
		TransportAttempts:        0,
		ResponseState:            record.Call.ResponseState,
		ResponseBytes:            record.Call.ResponseBytes,
		ResponseContentBytes:     len(record.Call.Response),
		ValidationCodes:          architectureSynthesisDiagnosticCodes(landscape.Diagnostics),
		MembershipCounted:        result.Membership.Counted,
		MemberOccurrences:        result.Membership.MemberOccurrences,
		DistinctMembers:          result.Membership.DistinctMembers,
		CoveredConceptualCount:   len(result.Membership.CoveredMemberIDs),
		UncoveredConceptualCount: len(result.Membership.UncoveredMemberIDs),
		UncoveredConceptualIDs:   append([]componentmap.MemberID(nil), result.Membership.UncoveredMemberIDs...),
		LocalCandidateCount:      len(bundle.Candidates),
		RequestedConceptualCount: conceptualCount,
		StructuralLocatorCount:   structuralLocatorCount,
	}, nil
}

func architectureSynthesisStatus(
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) report.ArchitectureSynthesisStatus {
	status := report.ArchitectureSynthesisStatus{
		Version:                  report.ArchitectureSynthesisStatusVersion,
		RequestBytes:             outcome.InputBytes,
		ResponseBytes:            outcome.ResponseBytes,
		ResponseContentBytes:     outcome.ResponseContentBytes,
		LatencyMillis:            outcome.LatencyMillis,
		TransportAttempts:        outcome.TransportAttempts,
		LocalCandidateCount:      outcome.LocalCandidateCount,
		RequestedConceptualCount: outcome.RequestedConceptualCount,
		StructuralLocatorCount:   outcome.StructuralLocatorCount,
		AnchorCount:              outcome.AnchorCount,
		MembershipCounted:        outcome.MembershipCounted,
		MemberOccurrences:        outcome.MemberOccurrences,
		DistinctMembers:          outcome.DistinctMembers,
		CoveredConceptualCount:   outcome.CoveredConceptualCount,
		UncoveredConceptualCount: outcome.UncoveredConceptualCount,
		UncoveredConceptualIDs:   append([]componentmap.MemberID(nil), outcome.UncoveredConceptualIDs...),
		UsageReported:            outcome.UsageReported,
		InputTokens:              outcome.InputTokens,
		OutputTokens:             outcome.OutputTokens,
		FinishReason:             outcome.FinishReason,
		ResponseComplete:         outcome.ResponseComplete,
		ResponseState:            string(outcome.ResponseState),
		ValidationCodes:          append([]string(nil), outcome.ValidationCodes...),
		ProviderCallSucceeded:    outcome.ProviderCallSucceeded,
		ResponseParsed:           outcome.ResponseParsed,
		ProposalAccepted: outcome.ValidationOutcome == componentmap.ValidationAccepted ||
			outcome.ValidationOutcome == componentmap.ValidationAcceptedNormalized ||
			outcome.ValidationOutcome == componentmap.ValidationAcceptedPartial,
		ProposalPartial:    outcome.ValidationOutcome == componentmap.ValidationAcceptedPartial,
		ProposalNormalized: outcome.NormalizationCount > 0,
		ProposalRejected:   outcome.ValidationOutcome == componentmap.ValidationRejected,
		FallbackSelected:   outcome.FallbackSelected,
		ArchitectureSource: string(outcome.ArchitectureSource),
		ArchitectureLevel:  outcome.ArchitectureLevel,
		NormalizationCount: outcome.NormalizationCount,
		FallbackReason:     string(outcome.FallbackReason),
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
	if errors.Is(synthesisErr, errArchitectureSynthesisRejected) {
		status.FallbackSelected = false
		status.FallbackReason = ""
	}
	// A failed optional enrichment never owns the visible Architecture. The
	// canonical local Canvas is published separately under Decision 177, so a
	// failed status must not carry an apparent selected architecture source.
	status.ProposalAccepted = false
	status.ProposalPartial = false
	status.ProposalNormalized = false
	status.CoveredConceptualCount = 0
	status.UncoveredConceptualCount = 0
	status.UncoveredConceptualIDs = nil
	status.ArchitectureSource = ""
	status.ArchitectureLevel = 0
	status.NormalizationCount = 0
	status.FallbackSelected = false
	status.FallbackReason = ""
	message := synthesisErr.Error()
	switch {
	case strings.Contains(message, "response content is empty"):
		status.ErrorCode = "empty_response"
	case errors.Is(synthesisErr, errArchitectureSynthesisRejected),
		strings.Contains(message, "unusable"), strings.Contains(message, "validate response"):
		status.ErrorCode = "invalid_response"
	default:
		status.ErrorCode = "provider_error"
	}
	return status
}

func normalizeArchitectureSynthesisOutputLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return "en", nil
	case "ru":
		return "ru", nil
	default:
		return "", fmt.Errorf("architecture synthesis: output language must be \"en\" or \"ru\"")
	}
}

// architectureSynthesisOutputLimitStatus builds the Decision 215 v9 failed
// status for an attempted Architecture output/response resource exhaustion.
// It carries only bounded operational evidence: exact request bytes, known
// partial response bytes, transport attempts, reported usage, the configured
// output ceiling and the observed completion tokens, finish_reason=length,
// response_complete=false, and the local/requested/structural/anchor input
// counts. It never carries an accepted proposal, partial membership counts, a
// model Architecture source or level, a fallback, provider prose, or the raw
// response.
func architectureSynthesisOutputLimitStatus(
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) report.ArchitectureSynthesisStatus {
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(synthesisErr, &limitErr) {
		limitErr = &modelresearch.ResourceLimitError{}
	}
	status := report.ArchitectureSynthesisStatus{
		Version:                  report.ArchitectureSynthesisStatusVersion,
		State:                    report.ArchitectureSynthesisFailed,
		ErrorCode:                report.ArchitectureSynthesisErrorProviderOutputLimit,
		RequestBytes:             outcome.InputBytes,
		ResponseBytes:            outcome.ResponseBytes,
		ResponseContentBytes:     outcome.ResponseContentBytes,
		LatencyMillis:            outcome.LatencyMillis,
		ProviderRequestCount:     1,
		TransportAttempts:        outcome.TransportAttempts,
		LocalCandidateCount:      outcome.LocalCandidateCount,
		RequestedConceptualCount: outcome.RequestedConceptualCount,
		StructuralLocatorCount:   outcome.StructuralLocatorCount,
		AnchorCount:              outcome.AnchorCount,
		UsageReported:            outcome.UsageReported,
		InputTokens:              outcome.InputTokens,
		OutputTokens:             outcome.OutputTokens,
		ConfiguredMaxTokens:      limitErr.ConfiguredMaxTokens,
		ObservedOutputTokens:     outcome.OutputTokens,
		FinishReason:             outcome.FinishReason,
		ResponseComplete:         outcome.ResponseComplete,
	}
	if status.ConfiguredMaxTokens == 0 {
		status.ConfiguredMaxTokens = limitErr.Limit
	}
	if status.ConfiguredMaxTokens == 0 {
		// The response-byte overflow limit error carries no token ceiling; the
		// exact global output ceiling is the configured budget for the
		// attempted call.
		status.ConfiguredMaxTokens = 64_000
	}
	if status.ObservedOutputTokens == 0 {
		status.ObservedOutputTokens = limitErr.OutputTokens
	}
	if status.FinishReason == "" {
		status.FinishReason = limitErr.FinishReason
	}
	return status
}

func removeArchitectureSynthesisRunRecord(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("architecture synthesis: remove unaccepted run record: %w", err)
}

func persistArchitectureSynthesisStatus(
	runDir string,
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) error {
	if isArchitectureOutputResourceExhausted(synthesisErr) {
		return writeArchitectureSynthesisStatus(
			runDir,
			architectureSynthesisOutputLimitStatus(outcome, synthesisErr),
		)
	}
	if isSemanticResourceLimit(synthesisErr) {
		return nil
	}
	if report.IsExactWorkspaceGraphUnavailable(synthesisErr) {
		return persistArchitectureSynthesisUnavailableWithCode(
			runDir,
			report.ArchitectureSynthesisUnavailableExactWorkspaceGraphCode,
		)
	}
	return writeArchitectureSynthesisStatus(
		runDir,
		architectureSynthesisStatus(outcome, synthesisErr),
	)
}

func persistAndClassifyArchitectureSynthesisStatus(
	runDir string,
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) error {
	statusErr := persistArchitectureSynthesisStatus(runDir, outcome, synthesisErr)
	if statusErr != nil {
		if synthesisErr != nil {
			return errors.Join(synthesisErr, statusErr)
		}
		return statusErr
	}
	if isArchitectureOutputResourceExhausted(synthesisErr) {
		// Decision 215: the failed status and model-research accounting are
		// durable, so the attempted Architecture output exhaustion is
		// publishable. main continues to Study and the report with the
		// canonical local Canvas.
		return &publishableArchitectureFailure{cause: synthesisErr}
	}
	if errors.Is(synthesisErr, errArchitectureSynthesisRejected) {
		return &publishableArchitectureFailure{cause: synthesisErr}
	}
	if report.IsExactWorkspaceGraphUnavailable(synthesisErr) {
		return &publishableArchitectureFailure{cause: synthesisErr}
	}
	// Decision 235 (v11) 1D sqlc/syn/bench: no canonical candidates is a
	// publishable local-only Architecture — the run continues to Study and
	// the minimal local report instead of terminating.
	if report.IsNoCanonicalArchitectureCandidates(synthesisErr) {
		return &publishableArchitectureFailure{cause: synthesisErr}
	}
	var providerFailure *architectureProviderCallFailed
	if errors.As(synthesisErr, &providerFailure) {
		return &publishableArchitectureFailure{cause: synthesisErr}
	}
	return synthesisErr
}

func persistArchitectureSynthesisUnavailable(runDir string) error {
	return persistArchitectureSynthesisUnavailableWithCode(
		runDir,
		report.ArchitectureSynthesisUnavailableOfflineCode,
	)
}

func persistArchitectureSynthesisUnavailableWithCode(runDir, code string) error {
	return writeArchitectureSynthesisStatus(runDir, report.ArchitectureSynthesisStatus{
		Version:         report.ArchitectureSynthesisStatusVersion,
		State:           report.ArchitectureSynthesisUnavailable,
		UnavailableCode: code,
	})
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
