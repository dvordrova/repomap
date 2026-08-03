package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	atlasStudyCacheContract = "atlas-study-accepted-v5"
	atlasStudyCacheStage    = "atlas_study"
)

type atlasStudyRunOutcome struct {
	State              atlasstudy.ProductState
	FailureCode        atlasstudy.FailureCode
	ProviderSkipped    bool
	Cached             bool
	SemanticCalls      int
	DirectionCount     int
	RejectedDirections int
	RequestBytes       int
	ResponseBytes      int
	InputTokens        int
	OutputTokens       int
	TransportAttempts  int
	LatencyMillis      int64
}

type atlasStudyClient interface {
	AtlasStudyPromptJSON(atlasstudy.Prompt, int) ([]byte, error)
	AtlasStudyMeasured(context.Context, atlasstudy.Prompt, int) (modelresearch.ProviderResult, error)
	EffectiveConfig() deepseek.EffectiveConfig
}

type atlasStudyClientFactory func(requireCredentials bool) (atlasStudyClient, error)

func defaultAtlasStudyClientFactory(requireCredentials bool) (atlasStudyClient, error) {
	if requireCredentials {
		return deepseek.NewFromEnv()
	}
	return deepseek.NewPromptFromEnv()
}

// runAtlasStudyForRun owns the single Atlas-backed Repository Brief and Study
// question. The caller supplies the same authority-confirmed, source-covered
// ReportData later used by final Generate. BuildAtlasStudyInput then reads only
// its Atlas, usable canonical visible Architecture Canvas (D177 local or
// accepted enrichment) and exact saved sources; Navigator is neither an input
// nor a prerequisite.
func runAtlasStudyForRun(
	ctx context.Context,
	preparedData *report.ReportData,
	runDir string,
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	providerEnabled bool,
	language atlasstudy.Language,
	output *runOutput,
) (atlasStudyRunOutcome, error) {
	if preparedData == nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: authorized prepared report data is required")
	}
	input, err := report.BuildAtlasStudyInput(preparedData, language)
	if err != nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: build exact input: %w", err)
	}
	product, err := atlasstudy.Compile(input)
	if err != nil {
		return atlasStudyRunOutcome{}, atlasStudyTerminalResource(err, 0)
	}
	return runAtlasStudyProductForRun(
		ctx, runDir, runsDir, repository, policy, noCache, providerEnabled,
		product, output, defaultAtlasStudyClientFactory,
	)
}

func runAtlasStudyProductForRun(
	ctx context.Context,
	runDir string,
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	providerEnabled bool,
	product atlasstudy.Product,
	output *runOutput,
	clients atlasStudyClientFactory,
) (atlasStudyRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if clients == nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: client factory is required")
	}
	if err := policy.Validate(); err != nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: model policy: %w", err)
	}
	output.Stage("Study", "compiling exact Atlas-backed brief and reading routes")
	if err := resetAtlasStudyArtifacts(runDir); err != nil {
		return atlasStudyRunOutcome{}, err
	}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: open confined artifact writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Atlas Study semantic exchange journal unavailable",
	})
	if !providerEnabled {
		outcome := atlasStudyRunOutcome{
			State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
		}
		output.State("Study", "unavailable", "provider calls: 0", "reason: offline requested")
		return outcome, nil
	}

	requestRecord, err := product.RequestRecord()
	if err != nil {
		return atlasStudyRunOutcome{}, fmt.Errorf("atlas study run: build request record: %w", err)
	}
	requestArtifact, err := atlasstudy.EncodeRequestRecord(requestRecord)
	if err != nil {
		return atlasStudyRunOutcome{}, atlasStudyTerminalResource(err, 0)
	}
	outcome := atlasStudyRunOutcome{State: atlasstudy.ProductStatePrepared}
	if unsafeErr := atlasStudyUnsafePayload("request_artifact", requestArtifact); unsafeErr != nil {
		recordAtlasStudyExchange(
			writer, requestArtifact, nil,
			&debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableOmitted},
			debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
			0, 0, debugdump.SemanticRequestPrepared,
		)
		return atlasStudyTerminalFailure(
			writer, product, outcome, atlasstudy.FailureProvider, unsafeErr,
		)
	}
	prompt := product.BuildPrompt()
	promptClient, err := clients(false)
	if err != nil {
		if persistErr := persistAtlasStudyRequest(writer, product, requestRecord, requestArtifact); persistErr != nil {
			return outcome, errors.Join(err, atlasStudyTerminalResource(persistErr, 0))
		}
		recordAtlasStudyPreparedFailure(writer, requestArtifact)
		return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, err, output)
	}
	maxRequestBytes := policy.Orientation.MaxRequestBytes
	providerRequest, err := promptClient.AtlasStudyPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		if persistErr := persistAtlasStudyRequest(writer, product, requestRecord, requestArtifact); persistErr != nil {
			return outcome, errors.Join(err, atlasStudyTerminalResource(persistErr, 0))
		}
		err = atlasStudyTerminalResource(err, promptClient.EffectiveConfig().MaxTokens)
		if isSemanticResourceLimit(err) {
			recordAtlasStudyExchange(
				writer, requestArtifact, nil,
				&debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableSize},
				debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
				0, 0, debugdump.SemanticRequestPrepared,
			)
			return atlasStudyTerminalFailure(
				writer, product, outcome, atlasstudy.FailureResource, err,
			)
		}
		recordAtlasStudyPreparedFailure(writer, requestArtifact)
		return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, err, output)
	}
	if unsafeErr := atlasStudyUnsafePayload("provider_request", providerRequest); unsafeErr != nil {
		recordAtlasStudyExchange(
			writer, providerRequest, nil,
			&debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableOmitted},
			debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
			0, 0, debugdump.SemanticRequestPrepared,
		)
		return atlasStudyTerminalFailure(
			writer, product, outcome, atlasstudy.FailureProvider, unsafeErr,
		)
	}
	if err := persistAtlasStudyRequest(writer, product, requestRecord, requestArtifact); err != nil {
		return outcome, atlasStudyTerminalResource(err, promptClient.EffectiveConfig().MaxTokens)
	}
	outcome.RequestBytes = len(providerRequest)
	config := promptClient.EffectiveConfig()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(config.Endpoint)
	if err != nil {
		recordAtlasStudyPreparedFailure(writer, providerRequest)
		return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, err, output)
	}
	cacheInput := atlasStudyStageCacheInput(
		runsDir, repository, policy, config, endpointSHA, product, providerRequest,
	)

	if !noCache {
		cached, found, loadErr := modelresearch.LoadStageResponse(cacheInput)
		if loadErr != nil {
			recordAtlasStudyPreparedFailure(writer, providerRequest)
			return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, loadErr, output)
		}
		if found {
			result, diagnostics, failure, validation, validationErr :=
				validateAtlasStudyResponse(product, cached.Content)
			if validationErr == nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return outcome, ctxErr
				}
				outcome.State = atlasstudy.ProductStateAccepted
				outcome.Cached = true
				outcome.DirectionCount = len(result.Directions)
				outcome.RejectedDirections = diagnostics.DirectionsRejected
				outcome.ResponseBytes = len(cached.Content)
				outcome.InputTokens = cached.InputTokens
				outcome.OutputTokens = cached.OutputTokens
				outcome.LatencyMillis = cached.LatencyMillis
				recordAtlasStudyExchange(
					writer, providerRequest, cached.Content, nil,
					debugdump.SemanticStateCacheHit, debugdump.SemanticValidationCache,
					0, 0, debugdump.SemanticRequestPrepared,
				)
				if err := persistAcceptedAtlasStudy(writer, product, result); err != nil {
					err = atlasStudyTerminalResource(err, config.MaxTokens)
					if isSemanticResourceLimit(err) {
						return atlasStudyTerminalFailure(
							writer, product, outcome, atlasstudy.FailureResource, err,
						)
					}
					return outcome, err
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					if cleanupErr := resetAtlasStudyArtifacts(runDir); cleanupErr != nil {
						return outcome, errors.Join(ctxErr, cleanupErr)
					}
					return outcome, ctxErr
				}
				atlasStudyAcceptedOutput(output, outcome)
				return outcome, nil
			}
			validationErr = atlasStudyTerminalResource(validationErr, config.MaxTokens)
			if isSemanticResourceLimit(validationErr) {
				recordAtlasStudyExchange(
					writer, providerRequest, nil,
					&debugdump.SemanticUnavailable{
						Code: debugdump.SemanticUnavailableSize, OriginalBytes: len(cached.Content),
					},
					debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse,
					0, 0, debugdump.SemanticRequestPrepared,
				)
				return atlasStudyTerminalFailure(
					writer, product, outcome, atlasstudy.FailureResource, validationErr,
				)
			}
			if err := modelresearch.InvalidateStageResponse(cacheInput); err != nil {
				return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, err, output)
			}
			if validation == debugdump.SemanticValidationSecret {
				recordAtlasStudyExchange(
					writer, providerRequest, cached.Content, nil,
					debugdump.SemanticStateRejected, validation,
					0, 0, debugdump.SemanticRequestPrepared,
				)
				return atlasStudyTerminalFailure(writer, product, outcome, failure, validationErr)
			}
		}
	}

	liveClient, err := clients(true)
	if err != nil {
		recordAtlasStudyPreparedFailure(writer, providerRequest)
		return atlasStudyOrdinaryFailure(writer, product, outcome, atlasstudy.FailureProvider, err, output)
	}
	if liveClient.EffectiveConfig() != config {
		recordAtlasStudyPreparedFailure(writer, providerRequest)
		return atlasStudyOrdinaryFailure(
			writer, product, outcome, atlasstudy.FailureProvider,
			fmt.Errorf("atlas study run: provider configuration changed after exact request preparation"),
			output,
		)
	}
	if concrete, ok := liveClient.(*deepseek.Client); ok {
		concrete.OnWait = func(progress deepseek.WaitProgress) {
			output.Stage(
				"Study", strings.TrimSpace(progress.Stage)+" is still running",
				"elapsed: "+progress.Elapsed.Round(time.Second).String(), "Ctrl-C to cancel",
			)
		}
	}
	output.State(
		"Study", "request prepared",
		fmt.Sprintf("request bytes: %d", len(providerRequest)), "model: "+config.Model,
	)
	started := time.Now()
	outcome.SemanticCalls = 1
	providerResult, callErr := liveClient.AtlasStudyMeasured(ctx, prompt, maxRequestBytes)
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.ResponseBytes = providerResultResponseBytes(providerResult)
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.TransportAttempts = providerResult.Attempts
	if ctxErr := ctx.Err(); ctxErr != nil {
		recordAtlasStudyExchange(
			writer, providerRequest, nil,
			&debugdump.SemanticUnavailable{
				Code: debugdump.SemanticUnavailableCanceled, OriginalBytes: outcome.ResponseBytes,
			},
			debugdump.SemanticStateCanceled, debugdump.SemanticValidationCanceled,
			1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
		)
		return atlasStudyTerminalFailure(
			writer, product, outcome, atlasstudy.FailureCanceled, ctxErr,
		)
	}
	if callErr != nil {
		callErr = atlasStudyTerminalResource(callErr, config.MaxTokens)
		response := providerFailureContentForExchange(callErr, providerResult.Content)
		unavailable := atlasStudyResponseUnavailable(callErr, outcome.ResponseBytes, response)
		state := debugdump.SemanticStateProviderFailed
		validation := debugdump.SemanticValidationProvider
		code := atlasstudy.FailureProvider
		terminal := false
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			state, validation, code, terminal = debugdump.SemanticStateCanceled,
				debugdump.SemanticValidationCanceled, atlasstudy.FailureCanceled, true
		} else if isSemanticResourceLimit(callErr) {
			code, terminal = atlasstudy.FailureResource, true
		}
		recordAtlasStudyExchange(
			writer, providerRequest, response, unavailable, state, validation,
			1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
		)
		if terminal {
			return atlasStudyTerminalFailure(writer, product, outcome, code, callErr)
		}
		return atlasStudyOrdinaryFailure(writer, product, outcome, code, callErr, output)
	}

	result, diagnostics, failure, validation, validationErr :=
		validateAtlasStudyResponse(product, providerResult.Content)
	if validationErr != nil {
		validationErr = atlasStudyTerminalResource(validationErr, config.MaxTokens)
		recordAtlasStudyExchange(
			writer, providerRequest, providerResult.Content, nil,
			debugdump.SemanticStateRejected, validation,
			1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
		)
		if isSemanticResourceLimit(validationErr) || validation == debugdump.SemanticValidationSecret {
			return atlasStudyTerminalFailure(
				writer, product, outcome,
				map[bool]atlasstudy.FailureCode{
					true: atlasstudy.FailureResource, false: failure,
				}[isSemanticResourceLimit(validationErr)],
				validationErr,
			)
		}
		return atlasStudyOrdinaryFailure(writer, product, outcome, failure, validationErr, output)
	}
	recordAtlasStudyExchange(
		writer, providerRequest, providerResult.Content, nil,
		debugdump.SemanticStateAccepted, debugdump.SemanticValidationAccepted,
		1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
	)
	outcome.State = atlasstudy.ProductStateAccepted
	outcome.DirectionCount = len(result.Directions)
	outcome.RejectedDirections = diagnostics.DirectionsRejected
	if err := persistAcceptedAtlasStudy(writer, product, result); err != nil {
		err = atlasStudyTerminalResource(err, config.MaxTokens)
		if isSemanticResourceLimit(err) {
			return atlasStudyTerminalFailure(
				writer, product, outcome, atlasstudy.FailureResource, err,
			)
		}
		return outcome, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if cleanupErr := resetAtlasStudyArtifacts(runDir); cleanupErr != nil {
			return outcome, errors.Join(ctxErr, cleanupErr)
		}
		return outcome, ctxErr
	}
	cacheSaved := false
	if !noCache {
		if _, err := modelresearch.SaveStageResponse(cacheInput, modelresearch.StageResponse{
			Content:     providerResult.Content,
			InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
			PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
			PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
			LatencyMillis:         outcome.LatencyMillis, RetryCount: max(0, providerResult.Attempts-1),
		}); err != nil {
			output.Warn("Atlas Study cache write failed", "accepted per-run result remains authoritative")
		} else {
			cacheSaved = true
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		var cleanupErr error
		if cacheSaved {
			cleanupErr = modelresearch.InvalidateStageResponse(cacheInput)
		}
		cleanupErr = errors.Join(cleanupErr, resetAtlasStudyArtifacts(runDir))
		if cleanupErr != nil {
			return outcome, errors.Join(ctxErr, cleanupErr)
		}
		return outcome, ctxErr
	}
	atlasStudyAcceptedOutput(output, outcome)
	return outcome, nil
}

func atlasStudyStageCacheInput(
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	config deepseek.EffectiveConfig,
	endpointSHA string,
	product atlasstudy.Product,
	request []byte,
) modelresearch.StageCacheInput {
	return modelresearch.StageCacheInput{
		RunsDir: runsDir,
		Fingerprint: modelresearch.FingerprintInput{
			Repository: repository, Stage: atlasStudyCacheStage,
			PromptVersion: atlasstudy.PromptVersion, CacheContract: atlasStudyCacheContract,
			Profile: "openai-compatible/" + config.AuthMode,
			Model:   config.Model, ProviderEndpointSHA256: endpointSHA,
			RequestSHA256:      modelresearch.SHA256(request),
			EvidenceBundleHash: product.CatalogSHA256(), PolicyVersion: policy.Version,
			OutputLanguage: string(product.Language()),
		},
		Request: request, EvidenceBundleHash: product.CatalogSHA256(),
	}
}

func validateAtlasStudyResponse(
	product atlasstudy.Product,
	raw []byte,
) (
	atlasstudy.ResultRecord,
	atlasstudy.Diagnostics,
	atlasstudy.FailureCode,
	string,
	error,
) {
	if kind, found := secretscan.DetectAlways(string(raw)); found {
		return atlasstudy.ResultRecord{}, atlasstudy.Diagnostics{}, atlasstudy.FailureDecode,
			debugdump.SemanticValidationSecret, fmt.Errorf(
				"atlas study run: provider response rejected by mandatory secret scan: kind=%s",
				secretscan.ClosedKind(kind),
			)
	}
	result, diagnostics, err := product.ResolveResponseJSON(raw)
	if err == nil {
		if err = product.ValidateResultRecord(result); err == nil {
			return result, diagnostics, "", debugdump.SemanticValidationAccepted, nil
		}
	}
	failure := atlasstudy.FailureValidation
	var decodeErr *atlasstudy.ResponseDecodeError
	if errors.As(err, &decodeErr) {
		failure = atlasstudy.FailureDecode
	}
	var refErr *atlasstudy.ReferenceError
	if errors.As(err, &refErr) {
		failure = atlasstudy.FailureReference
	}
	return atlasstudy.ResultRecord{}, diagnostics, failure,
		debugdump.SemanticValidationResponse, err
}

func persistAtlasStudyRequest(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	record atlasstudy.RequestRecord,
	encoded []byte,
) error {
	return writer.WriteValidatedFile(atlasstudy.RequestArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := atlasstudy.DecodeRequestRecord(saved)
		if err != nil {
			return err
		}
		return product.ValidateRequestRecord(decoded)
	})
}

func persistAtlasStudyResult(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	record atlasstudy.ResultRecord,
) error {
	encoded, err := atlasstudy.EncodeResultRecord(record)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(atlasstudy.ResultArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := atlasstudy.DecodeResultRecord(saved)
		if err != nil {
			return err
		}
		return product.ValidateResultRecord(decoded)
	})
}

func persistAtlasStudyStatus(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	status atlasstudy.Status,
) error {
	encoded, err := atlasstudy.EncodeStatus(status)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(atlasstudy.StatusArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := atlasstudy.DecodeStatus(saved)
		if err != nil {
			return err
		}
		if err := product.ValidateStatus(decoded); err != nil {
			return err
		}
		if decoded != status {
			return fmt.Errorf("atlas study status changed before publication")
		}
		return nil
	})
}

func persistAcceptedAtlasStudy(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	result atlasstudy.ResultRecord,
) error {
	if err := persistAtlasStudyResult(writer, product, result); err != nil {
		return err
	}
	status, err := product.AcceptedStatus(result)
	if err != nil {
		return err
	}
	return persistAtlasStudyStatus(writer, product, status)
}

func atlasStudyOrdinaryFailure(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	outcome atlasStudyRunOutcome,
	code atlasstudy.FailureCode,
	cause error,
	output *runOutput,
) (atlasStudyRunOutcome, error) {
	status, err := product.FailureStatus(code)
	if err != nil {
		return outcome, errors.Join(cause, err)
	}
	if err := persistAtlasStudyStatus(writer, product, status); err != nil {
		return outcome, errors.Join(cause, atlasStudyTerminalResource(err, 0))
	}
	outcome.State = atlasstudy.ProductStateFailed
	outcome.FailureCode = code
	if output != nil {
		output.Warn("Atlas Study failed", "failure: "+string(code), "canonical local products remain available")
	}
	return outcome, nil
}

func atlasStudyTerminalFailure(
	writer *debugdump.Writer,
	product atlasstudy.Product,
	outcome atlasStudyRunOutcome,
	code atlasstudy.FailureCode,
	cause error,
) (atlasStudyRunOutcome, error) {
	status, err := product.FailureStatus(code)
	if err != nil {
		return outcome, errors.Join(cause, err)
	}
	if err := persistAtlasStudyStatus(writer, product, status); err != nil {
		return outcome, errors.Join(cause, err)
	}
	outcome.State = atlasstudy.ProductStateFailed
	outcome.FailureCode = code
	return outcome, cause
}

func atlasStudyTerminalResource(err error, maxTokens int) error {
	var local *atlasstudy.ResourceLimitError
	if !errors.As(err, &local) {
		return err
	}
	details := modelresearch.ResourceLimitError{
		Stage: "atlas_study", Kind: modelresearch.ResourceLimitRequestBytes,
		ConfiguredMaxTokens: maxTokens,
	}
	switch {
	case local.Section == "response_bytes":
		details.Kind = modelresearch.ResourceLimitResponseBytes
		details.Limit = local.Limit
		details.Observed = local.Actual
		details.ObservedKnown = true
	case strings.HasSuffix(local.Section, "_artifact_bytes"):
		details.Kind = modelresearch.ResourceLimitRecordBytes
		details.Limit = local.Limit
		details.Observed = local.Actual
		details.ObservedKnown = true
	case strings.HasSuffix(local.Section, "_bytes"):
		details.Limit = local.Limit
		details.Observed = local.Actual
		details.ObservedKnown = true
	case atlasStudyCatalogCardinalitySection(local.Section):
		details.Kind = modelresearch.ResourceLimitCatalogItems
		details.Limit = local.Limit
		details.Observed = local.Actual
		details.ObservedKnown = true
	}
	return modelresearch.NewResourceLimitError(details, nil)
}

func atlasStudyCatalogCardinalitySection(section string) bool {
	switch section {
	case "units", "subsystems", "components", "surfaces", "reading_targets", "evidence", "documents":
		return true
	default:
		return false
	}
}

func atlasStudyUnsafePayload(label string, data []byte) error {
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf(
			"atlas study run: unsafe %s rejected by mandatory secret scan: kind=%s",
			label, secretscan.ClosedKind(kind),
		)
	}
	return nil
}

func atlasStudyResponseUnavailable(
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

func recordAtlasStudyExchange(
	writer *debugdump.Writer,
	request []byte,
	response []byte,
	unavailable *debugdump.SemanticUnavailable,
	state string,
	validation string,
	semanticCalls int,
	transportAttempts int,
	provenance string,
) {
	if len(response) > 0 {
		unavailable = nil
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageAtlasStudy,
		InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: provenance, State: state, ValidationCode: validation,
		SemanticCalls: semanticCalls, TransportAttempts: transportAttempts,
		Request: request, Response: response, ResponseUnavailable: unavailable,
	})
}

func recordAtlasStudyPreparedFailure(writer *debugdump.Writer, request []byte) {
	recordAtlasStudyExchange(
		writer, request, nil,
		&debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableNoContent},
		debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
		0, 0, debugdump.SemanticRequestPrepared,
	)
}

func resetAtlasStudyArtifacts(runDir string) error {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("atlas study run: open run root: %w", err)
	}
	defer root.Close()
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
	} {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("atlas study run: remove stale %s: %w", name, err)
		}
	}
	return nil
}

func atlasStudyAcceptedOutput(output *runOutput, outcome atlasStudyRunOutcome) {
	state := "accepted"
	providerCalls := 1
	if outcome.Cached {
		state = "cached"
		providerCalls = 0
	}
	details := []string{
		fmt.Sprintf("directions: %d", outcome.DirectionCount),
		fmt.Sprintf("response bytes: %d", outcome.ResponseBytes),
		formatRunOutputTokens(outcome.InputTokens, outcome.OutputTokens),
		fmt.Sprintf("provider calls: %d", providerCalls),
	}
	if !outcome.Cached {
		details = append(details,
			fmt.Sprintf("transport attempts: %d", outcome.TransportAttempts),
			formatRunOutputDuration(outcome.LatencyMillis),
		)
	}
	output.State("Study", state, details...)
	if outcome.RejectedDirections > 0 {
		output.Warn(
			"Atlas Study omitted invalid model routes",
			fmt.Sprintf("rejected: %d", outcome.RejectedDirections),
			fmt.Sprintf("accepted: %d", outcome.DirectionCount),
		)
	}
}
