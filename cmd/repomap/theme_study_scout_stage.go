package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// themeScoutStageOutcome is the run-wiring summary of the Theme Scout stage.
type themeScoutStageOutcome struct {
	State             atlasstudy.ProductState
	FailureCode       atlasstudy.FailureCode
	Cached            bool
	ScoutAccepted     int
	ScoutRejected     int
	RequestBytes      int
	ResponseBytes     int
	InputTokens       int
	OutputTokens      int
	SemanticCalls     int
	TransportAttempts int
	LatencyMillis     int64
}

// themeScoutStageResult is the full stage return: outcome plus the exact
// accepted artifacts the caller persists and passes to the next stage.
type themeScoutStageResult struct {
	outcome themeScoutStageOutcome
	request themestudy.ScoutRequest
	result  themestudy.ScoutResult
	status  themestudy.ScoutStatusRecord
	err     error
}

func themeScoutErr(
	outcome themeScoutStageOutcome,
	request themestudy.ScoutRequest,
	err error,
) themeScoutStageResult {
	return themeScoutStageResult{outcome: outcome, request: request, err: err}
}

func themeScoutOK(
	outcome themeScoutStageOutcome,
	request themestudy.ScoutRequest,
	result themestudy.ScoutResult,
	status themestudy.ScoutStatusRecord,
) themeScoutStageResult {
	return themeScoutStageResult{
		outcome: outcome, request: request, result: result, status: status,
	}
}

// runThemeScoutStage executes the Theme Scout semantic stage (contract C):
// compile the bounded request, check the accepted-only stage cache, call the
// provider with transport-only retries, item-locally validate the response
// (zero valid candidates = semantic failure), and persist + cache-write only
// the accepted result. Rejected responses never enter the cache.
func runThemeScoutStage(
	ctx context.Context,
	runDir string,
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	product atlasstudy.Product,
	vocabulary themestudy.Vocabulary,
	packs themestudy.SeedPackResult,
	language themestudy.Language,
	repoName string,
	writer *debugdump.Writer,
	output *runOutput,
	clients themeStudyClientFactory,
) themeScoutStageResult {
	outcome := themeScoutStageOutcome{State: atlasstudy.ProductStatePrepared}
	output.Stage("Study", "Theme Scout: proposing source-grounded themes")
	contextBlock := themeScoutContext(product, repoName, themeSpanAnchorRefsFromPacks(packs))
	request, err := themestudy.CompileScout(language, vocabulary, packs, contextBlock, "")
	if err != nil {
		return themeScoutErr(outcome, themestudy.ScoutRequest{},
			fmt.Errorf("theme scout run: compile request: %w", err))
	}
	requestBytes, err := themestudy.EncodeScoutRequest(request)
	if err != nil {
		return themeScoutErr(outcome, request, themeTerminalResource(err, 0))
	}
	if unsafeErr := themeUnsafeSourcePayload("scout_request", requestBytes); unsafeErr != nil {
		themeRecordUnsafe(writer, requestBytes)
		return themeScoutErr(outcome, request,
			themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	prompt := themestudy.BuildScoutPrompt(request)
	promptClient, err := clients(true)
	if err != nil {
		if persistErr := persistThemeScoutRequest(writer, request, requestBytes); persistErr != nil {
			return themeScoutErr(outcome, request,
				errors.Join(err, themeTerminalResource(persistErr, 0)))
		}
		return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	maxRequestBytes := policy.Orientation.MaxRequestBytes
	providerRequest, err := promptClient.ThemeScoutPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		if persistErr := persistThemeScoutRequest(writer, request, requestBytes); persistErr != nil {
			return themeScoutErr(outcome, request,
				errors.Join(err, themeTerminalResource(persistErr, 0)))
		}
		err = themeTerminalResource(err, promptClient.EffectiveConfig().MaxTokens)
		if isSemanticResourceLimit(err) {
			return themeScoutErr(outcome, request,
				themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
		}
		return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	if unsafeErr := themeUnsafeSourcePayload("scout_provider_request", providerRequest); unsafeErr != nil {
		return themeScoutErr(outcome, request,
			themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	if err := persistThemeScoutRequest(writer, request, requestBytes); err != nil {
		return themeScoutErr(outcome, request,
			themeTerminalResource(err, promptClient.EffectiveConfig().MaxTokens))
	}
	outcome.RequestBytes = len(providerRequest)
	config := promptClient.EffectiveConfig()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(config.Endpoint)
	if err != nil {
		return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	cacheInput := themeStageCacheInput(
		runsDir, repository, policy, config, endpointSHA,
		themeScoutCacheStage, themeScoutCacheContract,
		request.CatalogSHA256, language, providerRequest,
	)

	if !noCache {
		cached, found, loadErr := modelresearch.LoadStageResponse(cacheInput)
		if loadErr != nil {
			return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, loadErr, output)
		}
		if found {
			// Cache hits are re-validated with the exact same item-local
			// semantics as fresh responses (D178/D182): a semantically
			// invalid cached response is invalidated, never trusted.
			result, status, validation, validationErr := themeValidateScout(request, cached.Content)
			if validationErr == nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return themeScoutErr(outcome, request, ctxErr)
				}
				outcome.State = atlasstudy.ProductState(result.State)
				outcome.Cached = true
				outcome.ScoutAccepted = result.Status.Accepted
				outcome.ScoutRejected = result.Status.Rejected
				outcome.ResponseBytes = len(cached.Content)
				outcome.InputTokens = cached.InputTokens
				outcome.OutputTokens = cached.OutputTokens
				outcome.LatencyMillis = cached.LatencyMillis
				writer.RecordSemanticExchange(debugdump.SemanticExchange{
					Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateCacheHit,
					ValidationCode:    debugdump.SemanticValidationCache,
					RequestProvenance: debugdump.SemanticRequestPrepared,
					Request:           requestBytes, Response: cached.Content,
				})
				if err := persistThemeScoutAccepted(writer, request, result); err != nil {
					err = themeTerminalResource(err, config.MaxTokens)
					if isSemanticResourceLimit(err) {
						return themeScoutErr(outcome, request,
							themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
					}
					return themeScoutErr(outcome, request, err)
				}
				return themeScoutOK(outcome, request, result, status)
			}
			validationErr = themeTerminalResource(validationErr, config.MaxTokens)
			if isSemanticResourceLimit(validationErr) {
				return themeScoutErr(outcome, request,
					themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, validationErr))
			}
			if err := modelresearch.InvalidateStageResponse(cacheInput); err != nil {
				return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
			}
			if validation == debugdump.SemanticValidationSecret {
				return themeScoutErr(outcome, request,
					themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, validationErr))
			}
			// A rejected cached response falls through to a fresh call.
		}
	}

	started := time.Now()
	providerResult, err := promptClient.ThemeScoutMeasured(ctx, prompt, maxRequestBytes)
	if err != nil {
		outcome.SemanticCalls = 1
		outcome.TransportAttempts = providerResult.Attempts
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateProviderFailed,
			ValidationCode:      debugdump.SemanticValidationProvider,
			ResponseUnavailable: themeResponseUnavailable(err, 0, nil),
			SemanticCalls:       1, RequestProvenance: debugdump.SemanticRequestExactSent,
			Request: providerRequest,
		})
		return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureProvider, err, output)
	}
	outcome.TransportAttempts = providerResult.Attempts
	outcome.SemanticCalls = 1
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	if unsafeErr := themeUnsafePayload("scout_provider_response", providerResult.Content); unsafeErr != nil {
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateRejected,
			ValidationCode: debugdump.SemanticValidationSecret,
			SemanticCalls:  1, TransportAttempts: providerResult.Attempts,
			RequestProvenance: debugdump.SemanticRequestExactSent,
			Request:           providerRequest, Response: providerResult.Content,
		})
		return themeScoutErr(outcome, request,
			themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureProvider, unsafeErr))
	}
	result, status, validation, validationErr := themeValidateScout(request, providerResult.Content)
	if validationErr != nil {
		validationErr = themeTerminalResource(validationErr, config.MaxTokens)
		writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateRejected,
			ValidationCode: validation, SemanticCalls: 1,
			TransportAttempts: providerResult.Attempts,
			RequestProvenance: debugdump.SemanticRequestExactSent,
			Request:           providerRequest, Response: providerResult.Content,
		})
		if isSemanticResourceLimit(validationErr) || validation == debugdump.SemanticValidationSecret {
			return themeScoutErr(outcome, request,
				themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, validationErr))
		}
		return themeScoutOrdinaryFailure(writer, request, outcome, atlasstudy.FailureValidation, validationErr, output)
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateAccepted,
		ValidationCode: debugdump.SemanticValidationAccepted,
		SemanticCalls:  1, TransportAttempts: providerResult.Attempts,
		RequestProvenance: debugdump.SemanticRequestExactSent,
		Request:           providerRequest, Response: providerResult.Content,
	})
	outcome.State = atlasstudy.ProductState(result.State)
	outcome.ScoutAccepted = result.Status.Accepted
	outcome.ScoutRejected = result.Status.Rejected
	outcome.ResponseBytes = len(providerResult.Content)
	if err := persistThemeScoutAccepted(writer, request, result); err != nil {
		err = themeTerminalResource(err, config.MaxTokens)
		if isSemanticResourceLimit(err) {
			return themeScoutErr(outcome, request,
				themeScoutTerminalFailure(writer, request, outcome, atlasstudy.FailureResource, err))
		}
		return themeScoutErr(outcome, request, err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			return themeScoutErr(outcome, request, errors.Join(ctxErr, cleanupErr))
		}
		return themeScoutErr(outcome, request, ctxErr)
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
			output.Warn("Theme Scout cache write failed", "accepted per-run result remains authoritative")
		}
	}
	return themeScoutOK(outcome, request, result, status)
}

// themeValidateScout validates one raw Scout response against its exact
// request identity with item-local rejection. Zero valid candidates is a
// semantic failure (honest unavailable Study, never a fabricated shelf).
func themeValidateScout(
	request themestudy.ScoutRequest,
	raw []byte,
) (themestudy.ScoutResult, themestudy.ScoutStatusRecord, string, error) {
	if kind, found := secretscan.DetectAlways(string(raw)); found {
		return themestudy.ScoutResult{}, themestudy.ScoutStatusRecord{},
			debugdump.SemanticValidationSecret, fmt.Errorf(
				"theme scout run: provider response rejected by mandatory secret scan: kind=%s",
				secretscan.ClosedKind(kind),
			)
	}
	candidates, status, err := themestudy.ValidateScout(
		raw, request.AnchorRefs(), request.FileRefs(), request.CatalogSHA256,
	)
	if err != nil {
		return themestudy.ScoutResult{}, themestudy.ScoutStatusRecord{},
			debugdump.SemanticValidationResponse, err
	}
	// Item-local acceptance: surviving siblings carry t* refs; zero survivors
	// is the honest semantic failure (never a locally fabricated shelf).
	if len(candidates) == 0 {
		return themestudy.ScoutResult{}, themestudy.ScoutStatusRecord{},
			debugdump.SemanticValidationResponse, fmt.Errorf("theme scout run: zero valid candidates")
	}
	themestudy.AssignCandidateRefs(candidates)
	result := themestudy.ScoutResult{
		Version: themestudy.ScoutResultVersion, State: status.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, WireSHA256: request.WireSHA256,
		Candidates: candidates, Status: status,
	}
	record := themestudy.ScoutStatusRecord{
		Version: themestudy.ScoutResultVersion, State: status.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, Status: status,
	}
	return result, record, debugdump.SemanticValidationAccepted, nil
}

func persistThemeScoutRequest(
	writer *debugdump.Writer,
	request themestudy.ScoutRequest,
	encoded []byte,
) error {
	return writer.WriteValidatedFile(themestudy.ScoutRequestArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := themestudy.DecodeScoutRequest(saved)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(decoded, request) {
			return fmt.Errorf("theme scout request changed before publication")
		}
		return nil
	})
}

func persistThemeScoutAccepted(
	writer *debugdump.Writer,
	request themestudy.ScoutRequest,
	result themestudy.ScoutResult,
) error {
	resultBytes, err := themestudy.EncodeScoutResult(result)
	if err != nil {
		return err
	}
	if err := writer.WriteValidatedFile(themestudy.ScoutResultArtifactFilename, resultBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeScoutResult(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(decoded, result) {
			return fmt.Errorf("theme scout result changed before publication")
		}
		return nil
	}); err != nil {
		return err
	}
	statusBytes, err := themestudy.EncodeScoutStatus(themestudy.ScoutStatusRecord{
		Version: themestudy.ScoutResultVersion, State: result.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: result.Language,
		CatalogSHA256: result.CatalogSHA256, Status: result.Status,
	})
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(themestudy.ScoutStatusArtifactFilename, statusBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeScoutStatus(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if decoded.Status.Accepted != result.Status.Accepted {
			return fmt.Errorf("theme scout status changed before publication")
		}
		return nil
	})
}

func themeScoutOrdinaryFailure(
	writer *debugdump.Writer,
	request themestudy.ScoutRequest,
	outcome themeScoutStageOutcome,
	code atlasstudy.FailureCode,
	cause error,
	output *runOutput,
) themeScoutStageResult {
	status := themestudy.ScoutStatusRecord{
		Version: themestudy.ScoutResultVersion, State: string(atlasstudy.ProductStateFailed),
		PromptVersion: themestudy.ScoutPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, FailureCode: string(code),
		Status: themestudy.ScoutStatus{State: string(atlasstudy.ProductStateFailed)},
	}
	statusBytes, err := themestudy.EncodeScoutStatus(status)
	if err != nil {
		return themeScoutErr(outcome, request, errors.Join(cause, err))
	}
	if err := writer.WriteValidatedFile(themestudy.ScoutStatusArtifactFilename, statusBytes, func(saved []byte) error {
		_, decodeErr := themestudy.DecodeScoutStatus(saved)
		return decodeErr
	}); err != nil {
		return themeScoutErr(outcome, request,
			errors.Join(cause, themeTerminalResource(err, 0)))
	}
	outcome.State = atlasstudy.ProductStateFailed
	outcome.FailureCode = code
	if output != nil {
		output.Warn("Study themes unavailable", "Theme Scout failed: "+string(code), "local Atlas and Architecture remain available")
	}
	return themeScoutErr(outcome, request, nil)
}

func themeScoutTerminalFailure(
	writer *debugdump.Writer,
	request themestudy.ScoutRequest,
	outcome themeScoutStageOutcome,
	code atlasstudy.FailureCode,
	cause error,
) error {
	status := themestudy.ScoutStatusRecord{
		Version: themestudy.ScoutResultVersion, State: string(atlasstudy.ProductStateFailed),
		PromptVersion: themestudy.ScoutPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, FailureCode: string(code),
		Status: themestudy.ScoutStatus{State: string(atlasstudy.ProductStateFailed)},
	}
	statusBytes, err := themestudy.EncodeScoutStatus(status)
	if err != nil {
		return errors.Join(cause, err)
	}
	if err := writer.WriteValidatedFile(themestudy.ScoutStatusArtifactFilename, statusBytes, func(saved []byte) error {
		_, decodeErr := themestudy.DecodeScoutStatus(saved)
		return decodeErr
	}); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func themeUnsafePayload(label string, data []byte) error {
	kind, found := secretscan.DetectAlways(string(data))
	if !found {
		return nil
	}
	return fmt.Errorf(
		"theme study run: unsafe %s rejected by mandatory secret scan: kind=%s",
		label, secretscan.ClosedKind(kind),
	)
}

// themeUnsafeSourcePayload scans locally expanded repository source before it
// is sent to a model provider. Real credential material still fails closed;
// credential-shaped assignments in production code are legitimate (per the
// owner doctrine, a repo that mentions credentials is not our reason to make
// Study unavailable).
func themeUnsafeSourcePayload(label string, data []byte) error {
	kind, found := secretscan.DetectSourceMaterial(string(data))
	if !found {
		return nil
	}
	return fmt.Errorf(
		"theme study run: unsafe %s rejected by mandatory secret scan: kind=%s",
		label, secretscan.ClosedKind(kind),
	)
}

func themeRecordUnsafe(writer *debugdump.Writer, request []byte) {
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage: debugdump.SemanticStageAtlasStudy, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, State: debugdump.SemanticStateRejected,
		ValidationCode:    debugdump.SemanticValidationSecret,
		RequestProvenance: debugdump.SemanticRequestPrepared,
		Request:           request,
	})
}

func themeResponseUnavailable(
	err error,
	originalBytes int,
	response []byte,
) *debugdump.SemanticUnavailable {
	if len(response) > 0 {
		return nil
	}
	code := debugdump.SemanticUnavailableNoContent
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = debugdump.SemanticUnavailableCanceled
	} else if isSemanticResourceLimit(err) && originalBytes > 0 {
		code = debugdump.SemanticUnavailableSize
	}
	return &debugdump.SemanticUnavailable{Code: code, OriginalBytes: originalBytes}
}

var _ = deepseek.EffectiveConfig{}
