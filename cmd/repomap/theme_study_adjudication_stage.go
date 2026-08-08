package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// themeAdjStageOutcome is the run-wiring summary of the Theme Adjudication
// stage.
type themeAdjStageOutcome struct {
	State             atlasstudy.ProductState
	FailureCode       atlasstudy.FailureCode
	Cached            bool
	AdjAccepted       int
	AdjRejected       int
	RequestBytes      int
	ResponseBytes     int
	InputTokens       int
	OutputTokens      int
	SemanticCalls     int
	TransportAttempts int
	LatencyMillis     int64
}

// themeAdjStageResult is the full Adjudication stage return.
type themeAdjStageResult struct {
	outcome themeAdjStageOutcome
	request themestudy.AdjudicationRequest
	result  themestudy.AdjudicationResult
	err     error
}

func themeAdjErr(outcome themeAdjStageOutcome, request themestudy.AdjudicationRequest, err error) themeAdjStageResult {
	return themeAdjStageResult{outcome: outcome, request: request, err: err}
}

func themeAdjOK(outcome themeAdjStageOutcome, request themestudy.AdjudicationRequest, result themestudy.AdjudicationResult) themeAdjStageResult {
	return themeAdjStageResult{outcome: outcome, request: request, result: result}
}

// runThemeAdjudicationStage executes the Source Review / Theme Adjudication
// semantic stage (contract E) over the Scout-accepted candidates and the
// locally expanded f* sources. The same accepted-only cache discipline
// applies; item-local rejection keeps valid siblings while zero valid themes
// is a semantic failure (honest, never a fabricated shelf).
func runThemeAdjudicationStage(
	ctx context.Context,
	runDir string,
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	scoutRequest themestudy.ScoutRequest,
	scoutResult themestudy.ScoutResult,
	expansion themestudy.SourceExpansion,
	anchors map[string]themestudy.AnchorInfo,
	language themestudy.Language,
	writer *debugdump.Writer,
	output *runOutput,
	clients themeStudyClientFactory,
) themeAdjStageResult {
	outcome := themeAdjStageOutcome{State: atlasstudy.ProductStatePrepared}
	output.Stage("Study", "Theme Adjudication: reviewing themes against exact sources")
	// Archive 12 P0 (owner directive): carry the exact a* seed evidence the
	// Scout already received into the Adjudication wire — f* expansion is
	// additional context, not a replacement for anchor evidence.
	request, err := themestudy.CompileAdjudication(language, scoutResult.Candidates, expansion, anchors, scoutRequest.SeedPacks.Packs)
	if err != nil {
		return themeAdjErr(outcome, themestudy.AdjudicationRequest{},
			fmt.Errorf("theme adjudication run: compile request: %w", err))
	}
	requestBytes, err := themestudy.EncodeAdjudicationRequest(request)
	if err != nil {
		return themeAdjErr(outcome, request, themeTerminalResource(err, 0))
	}
	if unsafeErr := themeUnsafeSourcePayload("adjudication_request", requestBytes); unsafeErr != nil {
		return themeAdjErr(outcome, request,
			themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	prompt := themestudy.BuildAdjudicationPrompt(request)
	promptClient, err := clients(true)
	if err != nil {
		if persistErr := persistThemeAdjRequest(writer, request, requestBytes); persistErr != nil {
			return themeAdjErr(outcome, request,
				errors.Join(err, themeTerminalResource(persistErr, 0)))
		}
		return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	maxRequestBytes := policy.Orientation.MaxRequestBytes
	providerRequest, err := promptClient.ThemeAdjudicationPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		if persistErr := persistThemeAdjRequest(writer, request, requestBytes); persistErr != nil {
			return themeAdjErr(outcome, request,
				errors.Join(err, themeTerminalResource(persistErr, 0)))
		}
		err = themeTerminalResource(err, promptClient.EffectiveConfig().MaxTokens)
		if isSemanticResourceLimit(err) {
			return themeAdjErr(outcome, request,
				themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
		}
		return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	if unsafeErr := themeUnsafeSourcePayload("adjudication_provider_request", providerRequest); unsafeErr != nil {
		return themeAdjErr(outcome, request,
			themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	if err := persistThemeAdjRequest(writer, request, requestBytes); err != nil {
		return themeAdjErr(outcome, request,
			themeTerminalResource(err, promptClient.EffectiveConfig().MaxTokens))
	}
	outcome.RequestBytes = len(providerRequest)
	config := promptClient.EffectiveConfig()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(config.Endpoint)
	if err != nil {
		return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	cacheInput := themeStageCacheInput(
		runsDir, repository, policy, config, endpointSHA,
		themeAdjudicationCacheStage, themeAdjudicationCacheContract,
		request.CatalogSHA256, language, providerRequest,
	)

	if !noCache {
		cached, found, loadErr := modelresearch.LoadStageResponse(cacheInput)
		if loadErr != nil {
			return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, loadErr, output)
		}
		if found {
			result, status, validation, validationErr := themeValidateAdjudication(request, cached.Content)
			if validationErr == nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return themeAdjErr(outcome, request, ctxErr)
				}
				outcome.State = atlasstudy.ProductState(result.State)
				outcome.Cached = true
				outcome.AdjAccepted = result.Status.Accepted
				outcome.AdjRejected = result.Status.Rejected
				outcome.ResponseBytes = len(cached.Content)
				outcome.InputTokens = cached.InputTokens
				outcome.OutputTokens = cached.OutputTokens
				outcome.LatencyMillis = cached.LatencyMillis
				writer.RecordSemanticExchange(debugdump.SemanticExchange{
					Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 2, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateCacheHit,
					ValidationCode:    debugdump.SemanticValidationCache,
					RequestProvenance: debugdump.SemanticRequestPrepared,
					Request:           requestBytes, Response: cached.Content,
				})
				if err := persistThemeAdjAccepted(writer, request, result, status); err != nil {
					err = themeTerminalResource(err, config.MaxTokens)
					if isSemanticResourceLimit(err) {
						return themeAdjErr(outcome, request,
							themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
					}
					return themeAdjErr(outcome, request, err)
				}
				return themeAdjOK(outcome, request, result)
			}
			validationErr = themeTerminalResource(validationErr, config.MaxTokens)
			if isSemanticResourceLimit(validationErr) {
				return themeAdjErr(outcome, request,
					themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, validationErr))
			}
			if err := modelresearch.InvalidateStageResponse(cacheInput); err != nil {
				return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
			}
			if validation == debugdump.SemanticValidationSecret {
				return themeAdjErr(outcome, request,
					themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, validationErr))
			}
			// A rejected cached response falls through to a fresh call.
		}
	}

	started := time.Now()
	outcome.SemanticCalls = 1
	providerResult, err := promptClient.ThemeAdjudicationMeasured(ctx, prompt, maxRequestBytes)
	// Archive 12 P0 (owner directive): preserve every known failure
	// telemetry metric BEFORE branching on error — attempts, latency,
	// input/output tokens when the transport measured them, response bytes
	// when safely known. A provider failure must be investigable from the
	// outcome alone, not only after success.
	outcome.TransportAttempts = providerResult.Attempts
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.ResponseBytes = providerResultResponseBytes(providerResult)
	if err != nil {
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 2, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateProviderFailed,
			ValidationCode:      debugdump.SemanticValidationProvider,
			ResponseUnavailable: themeResponseUnavailable(err, outcome.ResponseBytes, providerResult.Content),
			SemanticCalls:       1, RequestProvenance: debugdump.SemanticRequestExactSent,
			TransportAttempts: providerResult.Attempts,
			Request:           providerRequest, Response: providerResult.Content,
		})
		return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	if unsafeErr := themeUnsafePayload("adjudication_provider_response", providerResult.Content); unsafeErr != nil {
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 2, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateRejected,
			ValidationCode: debugdump.SemanticValidationSecret,
			SemanticCalls:  1, TransportAttempts: providerResult.Attempts,
			RequestProvenance: debugdump.SemanticRequestExactSent,
			Request:           providerRequest, Response: providerResult.Content,
		})
		return themeAdjErr(outcome, request,
			themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	result, status, validation, validationErr := themeValidateAdjudication(request, providerResult.Content)
	if validationErr != nil {
		validationErr = themeTerminalResource(validationErr, config.MaxTokens)
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 2, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateRejected,
			ValidationCode: validation, SemanticCalls: 1,
			TransportAttempts: providerResult.Attempts,
			RequestProvenance: debugdump.SemanticRequestExactSent,
			Request:           providerRequest, Response: providerResult.Content,
		})
		if isSemanticResourceLimit(validationErr) || validation == debugdump.SemanticValidationSecret {
			return themeAdjErr(outcome, request,
				themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, validationErr))
		}
		return themeAdjOrdinaryFailure(writer, request, outcome, atlasstudy.FailureValidation, validationErr, output)
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 2, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateAccepted,
		ValidationCode: debugdump.SemanticValidationAccepted,
		SemanticCalls:  1, TransportAttempts: providerResult.Attempts,
		RequestProvenance: debugdump.SemanticRequestExactSent,
		Request:           providerRequest, Response: providerResult.Content,
	})
	outcome.State = atlasstudy.ProductState(result.State)
	outcome.AdjAccepted = result.Status.Accepted
	outcome.AdjRejected = result.Status.Rejected
	if err := persistThemeAdjAccepted(writer, request, result, status); err != nil {
		err = themeTerminalResource(err, config.MaxTokens)
		if isSemanticResourceLimit(err) {
			return themeAdjErr(outcome, request,
				themeAdjTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
		}
		return themeAdjErr(outcome, request, err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			return themeAdjErr(outcome, request, errors.Join(ctxErr, cleanupErr))
		}
		return themeAdjErr(outcome, request, ctxErr)
	}
	if !noCache {
		if _, err := modelresearch.SaveStageResponse(cacheInput, modelresearch.StageResponse{
			Content:     providerResult.Content,
			InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
			PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
			PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
			LatencyMillis:         outcome.LatencyMillis,
			RetryCount:            max(0, providerResult.Attempts-1),
		}); err != nil {
			output.Warn("Theme Adjudication cache write failed", "accepted per-run result remains authoritative")
		}
	}
	return themeAdjOK(outcome, request, result)
}

// themeValidateAdjudication validates one raw Adjudication response against
// its exact request identity with item-local rejection. Zero valid themes is
// an honest semantic-empty result (Decision 232): the failed status and the
// empty result publish so the report renders the complete local question
// browse — never fabricated cards, never hidden information.
func themeValidateAdjudication(
	request themestudy.AdjudicationRequest,
	raw []byte,
) (themestudy.AdjudicationResult, themestudy.AdjudicationStatusRecord, string, error) {
	if kind, found := secretscan.DetectAlways(string(raw)); found {
		return themestudy.AdjudicationResult{}, themestudy.AdjudicationStatusRecord{},
			debugdump.SemanticValidationSecret, fmt.Errorf(
				"theme adjudication run: provider response rejected by mandatory secret scan: kind=%s",
				secretscan.ClosedKind(kind),
			)
	}
	candidateByRef := make(map[string]*themestudy.ScoutCandidate, len(request.Candidates))
	for index := range request.Candidates {
		candidateByRef[request.Candidates[index].Ref] = &request.Candidates[index]
	}
	themes, status, err := themestudy.ValidateAdjudication(raw, candidateByRef)
	if err != nil {
		return themestudy.AdjudicationResult{}, themestudy.AdjudicationStatusRecord{},
			debugdump.SemanticValidationResponse, err
	}
	// Decision 232: zero valid themes is a legitimate semantic-empty
	// outcome, published as a failed state with the empty result retained.
	// The artifact contract serializes Unknowns with omitempty, so an empty
	// non-nil slice would not survive the encode→decode round-trip. Normalize
	// to nil so the persisted result always equals the in-memory record.
	for index := range themes {
		if len(themes[index].Unknowns) == 0 {
			themes[index].Unknowns = nil
		}
	}
	result := themestudy.AdjudicationResult{
		Version: themestudy.AdjudicationResultVersion, State: status.State,
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, WireSHA256: request.WireSHA256,
		Themes: themes, Status: status,
	}
	record := themestudy.AdjudicationStatusRecord{
		Version: themestudy.AdjudicationResultVersion, State: status.State,
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, Status: status,
	}
	return result, record, debugdump.SemanticValidationAccepted, nil
}

func persistThemeAdjRequest(
	writer *debugdump.Writer,
	request themestudy.AdjudicationRequest,
	encoded []byte,
) error {
	return writer.WriteValidatedFile(themestudy.AdjudicationRequestArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := themestudy.DecodeAdjudicationRequest(saved)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(decoded, request) {
			return fmt.Errorf("theme adjudication request changed before publication")
		}
		return nil
	})
}

func persistThemeAdjAccepted(
	writer *debugdump.Writer,
	request themestudy.AdjudicationRequest,
	result themestudy.AdjudicationResult,
	status themestudy.AdjudicationStatusRecord,
) error {
	resultBytes, err := themestudy.EncodeAdjudicationResult(result)
	if err != nil {
		return err
	}
	if err := writer.WriteValidatedFile(themestudy.AdjudicationResultArtifactFilename, resultBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeAdjudicationResult(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(decoded, result) {
			return fmt.Errorf("theme adjudication result changed before publication")
		}
		return nil
	}); err != nil {
		return err
	}
	statusBytes, err := themestudy.EncodeAdjudicationStatus(status)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(themestudy.AdjudicationStatusArtifactFilename, statusBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeAdjudicationStatus(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if decoded.Status.Accepted != result.Status.Accepted {
			return fmt.Errorf("theme adjudication status changed before publication")
		}
		return nil
	})
}

func themeAdjOrdinaryFailure(
	writer *debugdump.Writer,
	request themestudy.AdjudicationRequest,
	outcome themeAdjStageOutcome,
	code atlasstudy.FailureCode,
	cause error,
	output *runOutput,
) themeAdjStageResult {
	status := themestudy.AdjudicationStatusRecord{
		Version: themestudy.AdjudicationResultVersion, State: string(atlasstudy.ProductStateFailed),
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, FailureCode: string(code),
		Status: themestudy.AdjudicationStatus{State: string(atlasstudy.ProductStateFailed)},
	}
	statusBytes, err := themestudy.EncodeAdjudicationStatus(status)
	if err != nil {
		return themeAdjErr(outcome, request, errors.Join(cause, err))
	}
	if err := writer.WriteValidatedFile(themestudy.AdjudicationStatusArtifactFilename, statusBytes, func(saved []byte) error {
		_, decodeErr := themestudy.DecodeAdjudicationStatus(saved)
		return decodeErr
	}); err != nil {
		return themeAdjErr(outcome, request,
			errors.Join(cause, themeTerminalResource(err, 0)))
	}
	outcome.State = atlasstudy.ProductStateFailed
	outcome.FailureCode = code
	if output != nil {
		output.Warn(
			"Study themes unavailable",
			"Theme Adjudication failed: "+string(code),
			fmt.Sprintf("provider calls: %d · transport attempts: %d", outcome.SemanticCalls, outcome.TransportAttempts),
			fmt.Sprintf("request/response bytes: %d/%d", outcome.RequestBytes, outcome.ResponseBytes),
			formatRunOutputTokens(outcome.InputTokens, outcome.OutputTokens),
			formatRunOutputDuration(outcome.LatencyMillis),
			"local Atlas and Architecture remain available",
		)
	}
	return themeAdjErr(outcome, request, nil)
}

func themeAdjTerminalFailure(
	writer *debugdump.Writer,
	request themestudy.AdjudicationRequest,
	outcome themeAdjStageOutcome,
	code atlasstudy.FailureCode,
	cause error,
) error {
	status := themestudy.AdjudicationStatusRecord{
		Version: themestudy.AdjudicationResultVersion, State: string(atlasstudy.ProductStateFailed),
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, FailureCode: string(code),
		Status: themestudy.AdjudicationStatus{State: string(atlasstudy.ProductStateFailed)},
	}
	statusBytes, err := themestudy.EncodeAdjudicationStatus(status)
	if err != nil {
		return errors.Join(cause, err)
	}
	if err := writer.WriteValidatedFile(themestudy.AdjudicationStatusArtifactFilename, statusBytes, func(saved []byte) error {
		_, decodeErr := themestudy.DecodeAdjudicationStatus(saved)
		return decodeErr
	}); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
