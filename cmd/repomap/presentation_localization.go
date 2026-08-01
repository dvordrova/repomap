package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	presentationLocalizationCacheVersion    = "presentation-localization-cache-v3"
	presentationLocalizationCacheDir        = ".localization-cache"
	presentationLocalizationCacheVersionDir = "v3"
	maxPresentationLocalizationCacheBytes   = 8 << 20
)

type presentationLocalizationProvider interface {
	BuildLocalizationRequest(
		localization.Prompt,
	) (deepseek.LocalizationRequestEvidence, error)
	ExecuteLocalizationRequest(
		context.Context,
		localization.Prompt,
		deepseek.LocalizationRequestEvidence,
	) (modelresearch.ProviderResult, error)
}

var errPresentationLocalizationProviderConfiguration = errors.New(
	"presentation localization provider configuration is unavailable",
)

type lazyPresentationLocalizationProvider struct {
	promptClient  *deepseek.Client
	newLiveClient func() (*deepseek.Client, error)
	onWait        func(deepseek.WaitProgress)
}

func (provider *lazyPresentationLocalizationProvider) BuildLocalizationRequest(
	prompt localization.Prompt,
) (deepseek.LocalizationRequestEvidence, error) {
	if provider == nil || provider.promptClient == nil {
		return deepseek.LocalizationRequestEvidence{}, fmt.Errorf(
			"%w: prompt client is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	return provider.promptClient.BuildLocalizationRequest(prompt)
}

func (provider *lazyPresentationLocalizationProvider) ExecuteLocalizationRequest(
	ctx context.Context,
	prompt localization.Prompt,
	evidence deepseek.LocalizationRequestEvidence,
) (modelresearch.ProviderResult, error) {
	if provider == nil || provider.newLiveClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live client factory is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	client, err := provider.newLiveClient()
	if err != nil {
		return modelresearch.ProviderResult{}, errors.Join(
			errPresentationLocalizationProviderConfiguration,
			fmt.Errorf("localization live provider: %w", err),
		)
	}
	if client == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live client is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	liveEvidence, err := client.BuildLocalizationRequest(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, errors.Join(
			errPresentationLocalizationProviderConfiguration,
			fmt.Errorf("rebuild request identity: %w", err),
		)
	}
	if !samePresentationLocalizationRequest(liveEvidence, evidence) {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live provider identity changed after cache lookup",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	client.OnWait = provider.onWait
	return client.ExecuteLocalizationRequest(ctx, prompt, evidence)
}

type presentationLocalizationCacheIdentity struct {
	Version                    string                               `json:"version"`
	ContractVersion            string                               `json:"contract_version"`
	TranslationContractVersion string                               `json:"translation_contract_version"`
	TargetLocale               string                               `json:"target_locale"`
	BatchContractVersion       int                                  `json:"batch_contract_version"`
	BatchContentSHA256         string                               `json:"batch_content_sha256"`
	Request                    deepseek.LocalizationRequestEvidence `json:"request"`
}

type presentationLocalizationCacheRecord struct {
	Version    string                                `json:"version"`
	Key        string                                `json:"key"`
	Identity   presentationLocalizationCacheIdentity `json:"identity"`
	Projection localization.Projection               `json:"projection"`
}

// presentationLocalizationCacheObservation binds a cache decision to the
// exact bounded regular file that was inspected. The file identity prevents a
// replacement inode from being removed, while data preserves the exact bytes
// used to classify the observed inode as corrupt.
type presentationLocalizationCacheObservation struct {
	info os.FileInfo
	data []byte
}

type presentationLocalizationOutcome struct {
	State            string
	ReasonCode       string
	FailureStage     string
	ValidationCode   string
	UnsafeKind       string
	TranslationIndex int
	BatchTotal       int
	BatchAttempted   int
	BatchCompleted   int
	FailedBatch      int
	CacheHit         bool
	CacheCorrupt     bool
	CacheWriteErr    bool
	RequestBytes     int
	ResponseBytes    int
	InputTokens      int
	OutputTokens     int
	Attempts         int
	ProviderCalls    int
	Batches          []presentationLocalizationBatchOutcome
}

type presentationLocalizationExecutionOptions struct {
	ExchangeWriter  *debugdump.Writer
	ApplyProjection func(
		localization.CanonicalArtifact,
		localization.Input,
		localization.Projection,
	) (localization.Result, error)
}

type presentationLocalizationStageError struct {
	Stage          string
	ValidationCode string
	ReasonCode     string
	Err            error
}

func (stageErr *presentationLocalizationStageError) Error() string {
	return stageErr.Err.Error()
}

func (stageErr *presentationLocalizationStageError) Unwrap() error {
	return stageErr.Err
}

func localizationStageError(
	stage,
	validationCode,
	reasonCode string,
	err error,
) error {
	if err == nil {
		return nil
	}
	return &presentationLocalizationStageError{
		Stage: stage, ValidationCode: validationCode, ReasonCode: reasonCode, Err: err,
	}
}

func presentationLocalizationErrorDetails(err error) (stage, validationCode, reasonCode string) {
	var stageErr *presentationLocalizationStageError
	if errors.As(err, &stageErr) {
		return stageErr.Stage, stageErr.ValidationCode, stageErr.ReasonCode
	}
	return report.LocalizationStageStatusWrite,
		report.LocalizationValidationStatus,
		report.LocalizationFailurePreparation
}

type presentationLocalizationBatchOutcome struct {
	Index                int
	Count                int
	FieldCount           int
	PredictedOutputBytes int
	CacheHit             bool
	RequestBytes         int
	ResponseBytes        int
	InputTokens          int
	OutputTokens         int
	Attempts             int
	ProviderCalls        int
}

type presentationLocalizationBatchPlan struct {
	Batch        localization.Batch
	Prompt       localization.Prompt
	Request      deepseek.LocalizationRequestEvidence
	Identity     presentationLocalizationCacheIdentity
	Key          string
	IdentityJSON []byte
	RequestSHA   string
}

func preparePresentationLocalizationForRun(
	runDir string,
	canonical *report.ReportData,
	sourceEpisodeJSON []byte,
) (*report.ReportData, report.PreparedPresentationLocalization, error) {
	if canonical == nil ||
		canonical.FormatVersion != report.CurrentFormatVersion ||
		canonical.ReportLanguage != "" ||
		canonical.GitLabSourceLinks != nil ||
		canonical.GitHubSourceLinks != nil {
		return nil, report.PreparedPresentationLocalization{}, localizationStageError(
			report.LocalizationStageCanonicalRead,
			report.LocalizationValidationCanonicalReport,
			report.LocalizationFailurePreparation,
			fmt.Errorf("localization: report data is not canonical English"),
		)
	}
	data, err := report.PrepareRunPresentation(runDir, canonical, sourceEpisodeJSON)
	if err != nil {
		return nil, report.PreparedPresentationLocalization{}, localizationStageError(
			report.LocalizationStagePresentationHydration,
			report.LocalizationValidationPresentationInventory,
			report.LocalizationFailurePreparation,
			fmt.Errorf("localization: prepare run presentation: %w", err),
		)
	}
	prepared, err := report.PreparePresentationLocalization(
		data,
		localization.LocaleRussian,
	)
	if err != nil {
		return nil, report.PreparedPresentationLocalization{}, localizationStageError(
			report.LocalizationStageInventoryBuild,
			report.LocalizationValidationPresentationInventory,
			report.LocalizationFailurePreparation,
			fmt.Errorf("localization: build presentation inventory: %w", err),
		)
	}
	return data, prepared, nil
}

func recordPresentationLocalizationPreparationFailure(
	runDir string,
	cause error,
) (presentationLocalizationOutcome, error) {
	stage, validationCode, _ := presentationLocalizationErrorDetails(cause)
	failure := report.PresentationLocalizationFailure{
		ReasonCode:     report.LocalizationFailurePreparation,
		FailureStage:   stage,
		ValidationCode: validationCode,
	}
	if writeErr := report.WritePresentationLocalizationFailure(runDir, failure); writeErr != nil {
		return presentationLocalizationOutcome{}, errors.Join(cause, writeErr)
	}
	return presentationLocalizationOutcome{
		State:          report.PresentationLocalizationFailed,
		ReasonCode:     failure.ReasonCode,
		FailureStage:   failure.FailureStage,
		ValidationCode: failure.ValidationCode,
	}, nil
}

func localizePreparedPresentationForRun(
	ctx context.Context,
	runDir,
	cacheRoot string,
	noCache bool,
	stderr io.Writer,
	data *report.ReportData,
	prepared report.PreparedPresentationLocalization,
) (presentationLocalizationOutcome, error) {
	exchangeWriter, writerErr := debugdump.OpenWriter(runDir, true)
	if writerErr == nil {
		defer exchangeWriter.Close()
		exchangeWriter.SetWarningWriter(stderr)
	} else {
		fmt.Fprintf(
			stderr,
			"warning: semantic exchange journal unavailable: stage=%s code=%s\n",
			debugdump.SemanticStageLocalization,
			debugdump.SemanticExchangeWarningCode,
		)
	}
	promptClient, err := deepseek.NewPromptFromEnv()
	if err != nil {
		failure := report.PresentationLocalizationFailure{
			ReasonCode:      report.LocalizationFailureProviderConfig,
			FailureStage:    report.LocalizationStageProviderConfiguration,
			ValidationCode:  report.LocalizationValidationRequestIdentity,
			CanonicalSHA256: prepared.Canonical.SHA256,
		}
		if writeErr := report.WritePresentationLocalizationFailure(
			runDir, failure,
		); writeErr != nil {
			return presentationLocalizationOutcome{}, errors.Join(err, writeErr)
		}
		return presentationLocalizationOutcome{
			State:          report.PresentationLocalizationFailed,
			ReasonCode:     report.LocalizationFailureProviderConfig,
			FailureStage:   failure.FailureStage,
			ValidationCode: failure.ValidationCode,
		}, nil
	}
	provider := &lazyPresentationLocalizationProvider{
		promptClient:  promptClient,
		newLiveClient: deepseek.NewFromEnv,
		onWait: func(progress deepseek.WaitProgress) {
			fmt.Fprintf(
				stderr,
				"repomap: %s still running after %s (Ctrl-C to cancel)\n",
				progress.Stage,
				progress.Elapsed.Round(time.Second),
			)
		},
	}
	return executePresentationLocalization(
		ctx,
		runDir,
		cacheRoot,
		noCache,
		data,
		prepared,
		provider,
		presentationLocalizationExecutionOptions{ExchangeWriter: exchangeWriter},
	)
}

func markPresentationLocalizationUnavailable(
	runDir,
	reasonCode string,
	prepared *report.PreparedPresentationLocalization,
) error {
	failure := report.PresentationLocalizationFailure{
		ReasonCode: reasonCode, FailureStage: report.LocalizationStageUnavailable,
		ValidationCode: localizationUnavailableValidationCode(reasonCode),
	}
	if prepared != nil {
		failure.CanonicalSHA256 = prepared.Canonical.SHA256
	}
	return report.WritePresentationLocalizationFailure(runDir, failure)
}

func executePresentationLocalization(
	ctx context.Context,
	runDir,
	cacheRoot string,
	noCache bool,
	data *report.ReportData,
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
	options ...presentationLocalizationExecutionOptions,
) (presentationLocalizationOutcome, error) {
	outcome := presentationLocalizationOutcome{}
	executionOptions := presentationLocalizationExecutionOptions{}
	if len(options) > 0 {
		executionOptions = options[0]
	}
	applyProjection := executionOptions.ApplyProjection
	if applyProjection == nil {
		applyProjection = localization.Apply
	}
	fail := func(reasonCode, failureStage, validationCode string) (presentationLocalizationOutcome, error) {
		outcome.State = report.PresentationLocalizationFailed
		outcome.ReasonCode = reasonCode
		outcome.FailureStage = failureStage
		outcome.ValidationCode = validationCode
		return outcome, report.WritePresentationLocalizationFailure(
			runDir,
			report.PresentationLocalizationFailure{
				ReasonCode: reasonCode, FailureStage: failureStage,
				ValidationCode: validationCode, CanonicalSHA256: prepared.Canonical.SHA256,
				Progress: report.PresentationLocalizationProgress{
					BatchTotal: outcome.BatchTotal, BatchAttempted: outcome.BatchAttempted,
					BatchCompleted: outcome.BatchCompleted, FailedBatch: outcome.FailedBatch,
				},
			},
		)
	}
	failTerminal := func(
		cause error,
		reasonCode,
		failureStage,
		validationCode string,
	) (presentationLocalizationOutcome, error) {
		failed, writeErr := fail(reasonCode, failureStage, validationCode)
		if writeErr != nil {
			return failed, errors.Join(cause, writeErr)
		}
		return failed, cause
	}

	plans, err := buildPresentationLocalizationBatchPlans(prepared, provider)
	if err != nil {
		stage, validationCode, reasonCode := presentationLocalizationErrorDetails(err)
		if isSemanticResourceLimit(err) {
			return failTerminal(err, reasonCode, stage, validationCode)
		}
		return fail(reasonCode, stage, validationCode)
	}
	outcome.BatchTotal = len(plans)
	projections := make([]localization.Projection, len(plans))
	requestSHAs := make([]string, len(plans))
	cacheKeys := make([]string, len(plans))
	outcome.Batches = make([]presentationLocalizationBatchOutcome, 0, len(plans))
	allCacheHits := true

	for index, plan := range plans {
		outcome.BatchAttempted++
		requestSHAs[index] = plan.RequestSHA
		cacheKeys[index] = plan.Key
		batchOutcome := presentationLocalizationBatchOutcome{
			Index: index, Count: len(plans),
			FieldCount:           plan.Batch.Manifest.FieldCount,
			PredictedOutputBytes: plan.Batch.Manifest.PredictedOutputBytes,
		}

		if !noCache {
			record, found, corrupt := loadPresentationLocalizationCache(
				cacheRoot,
				plan.Key,
				plan.IdentityJSON,
			)
			outcome.CacheCorrupt = outcome.CacheCorrupt || corrupt
			if found && presentationLocalizationBatchCacheProjectionValid(
				plan.Batch,
				record,
			) {
				recordPresentationLocalizationSemanticExchange(
					executionOptions.ExchangeWriter,
					plan,
					index,
					nil,
					&debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableProjection},
					debugdump.SemanticRequestPrepared,
					debugdump.SemanticStateCacheHit,
					debugdump.SemanticValidationCache,
					0,
					0,
				)
				projections[index] = record.Projection
				batchOutcome.CacheHit = true
				outcome.Batches = append(outcome.Batches, batchOutcome)
				outcome.BatchCompleted++
				continue
			}
			if found {
				outcome.CacheCorrupt = true
			}
		}

		allCacheHits = false
		batchOutcome.RequestBytes = len(plan.Request.Body)
		outcome.RequestBytes += batchOutcome.RequestBytes
		providerResult, executeErr := provider.ExecuteLocalizationRequest(
			ctx,
			plan.Prompt,
			plan.Request,
		)
		batchOutcome.ResponseBytes = providerResultResponseBytes(providerResult)
		batchOutcome.ProviderCalls++
		outcome.ProviderCalls++
		batchOutcome.InputTokens = providerResult.InputTokens
		batchOutcome.OutputTokens = providerResult.OutputTokens
		batchOutcome.Attempts = providerResult.Attempts
		outcome.ResponseBytes += batchOutcome.ResponseBytes
		outcome.InputTokens += batchOutcome.InputTokens
		outcome.OutputTokens += batchOutcome.OutputTokens
		outcome.Attempts += batchOutcome.Attempts
		outcome.Batches = append(outcome.Batches, batchOutcome)
		if executeErr != nil {
			reasonCode := report.LocalizationFailureProviderRequest
			failureStage := report.LocalizationStageProviderRequest
			validationCode := report.LocalizationValidationTransport
			if errors.Is(executeErr, errPresentationLocalizationProviderConfiguration) {
				reasonCode = report.LocalizationFailureProviderConfig
				failureStage = report.LocalizationStageProviderConfiguration
				validationCode = report.LocalizationValidationRequestIdentity
			}
			outcome.FailedBatch = index + 1
			state := debugdump.SemanticStateProviderFailed
			semanticValidation := debugdump.SemanticValidationProvider
			unavailableCode := debugdump.SemanticUnavailableNoContent
			if ctx.Err() != nil {
				state = debugdump.SemanticStateCanceled
				semanticValidation = debugdump.SemanticValidationCanceled
				unavailableCode = debugdump.SemanticUnavailableCanceled
			}
			provenance := debugdump.SemanticRequestPrepared
			if providerResult.Attempts > 0 {
				provenance = debugdump.SemanticRequestExactSent
			}
			recordedResponse := providerFailureContentForExchange(
				executeErr,
				providerResult.Content,
			)
			unavailable := localizationUnavailableResponse(recordedResponse, unavailableCode)
			if unavailable != nil {
				unavailable.OriginalBytes = batchOutcome.ResponseBytes
			}
			recordPresentationLocalizationSemanticExchange(
				executionOptions.ExchangeWriter,
				plan,
				index,
				recordedResponse,
				unavailable,
				provenance,
				state,
				semanticValidation,
				1,
				providerResult.Attempts,
			)
			if isSemanticResourceLimit(executeErr) {
				return failTerminal(executeErr, reasonCode, failureStage, validationCode)
			}
			return fail(reasonCode, failureStage, validationCode)
		}
		if unsafeKind, found := secretscan.DetectAlways(string(providerResult.Content)); found {
			outcome.UnsafeKind, outcome.TranslationIndex = presentationLocalizationUnsafeResponseAttribution(
				plan.Batch.Canonical,
				plan.Batch.Input,
				providerResult.Content,
				unsafeKind,
			)
			outcome.FailedBatch = index + 1
			recordPresentationLocalizationSemanticExchange(
				executionOptions.ExchangeWriter, plan, index, providerResult.Content,
				localizationUnavailableResponse(providerResult.Content, debugdump.SemanticUnavailableNoContent),
				debugdump.SemanticRequestExactSent,
				debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
				1, providerResult.Attempts,
			)
			return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageResponseSecretScan, report.LocalizationValidationUnsafeResponse)
		}
		projection, decodeErr := localization.DecodeRussianProviderResponse(
			plan.Batch.Canonical,
			plan.Batch.Input,
			[]byte(providerResult.Content),
		)
		if decodeErr != nil {
			outcome.FailedBatch = index + 1
			recordPresentationLocalizationSemanticExchange(
				executionOptions.ExchangeWriter, plan, index, providerResult.Content,
				localizationUnavailableResponse(providerResult.Content, debugdump.SemanticUnavailableNoContent),
				debugdump.SemanticRequestExactSent,
				debugdump.SemanticStateRejected, debugdump.SemanticValidationDecode,
				1, providerResult.Attempts,
			)
			return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageResponseDecode, report.LocalizationValidationResponseDecode)
		}
		validation, validationErr := applyProjection(
			plan.Batch.Canonical,
			plan.Batch.Input,
			projection,
		)
		if validationErr != nil {
			outcome.FailedBatch = index + 1
			recordPresentationLocalizationSemanticExchange(
				executionOptions.ExchangeWriter, plan, index, providerResult.Content,
				localizationUnavailableResponse(providerResult.Content, debugdump.SemanticUnavailableNoContent),
				debugdump.SemanticRequestExactSent,
				debugdump.SemanticStateRejected, debugdump.SemanticValidationApply,
				1, providerResult.Attempts,
			)
			return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageProjectionApply, report.LocalizationValidationProjectionApply)
		}
		if validation.Fallback || len(validation.Diagnostics) != 0 {
			outcome.FailedBatch = index + 1
			recordPresentationLocalizationSemanticExchange(
				executionOptions.ExchangeWriter, plan, index, providerResult.Content,
				localizationUnavailableResponse(providerResult.Content, debugdump.SemanticUnavailableNoContent),
				debugdump.SemanticRequestExactSent,
				debugdump.SemanticStateRejected, debugdump.SemanticValidationQuality,
				1, providerResult.Attempts,
			)
			return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageProjectionQuality, report.LocalizationValidationProjectionDiagnostics)
		}
		recordPresentationLocalizationSemanticExchange(
			executionOptions.ExchangeWriter, plan, index, providerResult.Content,
			localizationUnavailableResponse(providerResult.Content, debugdump.SemanticUnavailableNoContent),
			debugdump.SemanticRequestExactSent,
			debugdump.SemanticStateAccepted, debugdump.SemanticValidationAccepted,
			1, providerResult.Attempts,
		)
		projections[index] = projection
		outcome.BatchCompleted++

		if !noCache {
			record := presentationLocalizationCacheRecord{
				Version: presentationLocalizationCacheVersion,
				Key:     plan.Key, Identity: plan.Identity,
				Projection: projection,
			}
			cacheAlreadyValid := false
			cacheWriteBlocked := false
			var cacheWinner *localization.Projection
			existing, found, corrupt, observed := loadPresentationLocalizationCacheObserved(
				cacheRoot,
				plan.Key,
				plan.IdentityJSON,
			)
			if found && presentationLocalizationBatchCacheProjectionValid(
				plan.Batch,
				existing,
			) {
				cacheAlreadyValid = true
				cacheWinner = &existing.Projection
			} else if found {
				corrupt = true
			}
			if corrupt {
				outcome.CacheCorrupt = true
				removed, err := removeCorruptPresentationLocalizationCache(
					cacheRoot,
					plan.Key,
					observed,
				)
				if err != nil {
					outcome.CacheWriteErr = true
					cacheWriteBlocked = true
				} else if !removed {
					// A different inode or byte sequence won the race. Never
					// unlink it. A valid winner is first-valid-wins; an invalid
					// winner blocks this write and will be observed afresh by a
					// later run.
					winner, winnerFound, _ := loadPresentationLocalizationCache(
						cacheRoot,
						plan.Key,
						plan.IdentityJSON,
					)
					if winnerFound && presentationLocalizationBatchCacheProjectionValid(
						plan.Batch,
						winner,
					) {
						cacheAlreadyValid = true
						cacheWinner = &winner.Projection
					} else {
						outcome.CacheWriteErr = true
						cacheWriteBlocked = true
					}
				}
			}
			if !cacheAlreadyValid && !cacheWriteBlocked {
				if err := writePresentationLocalizationCache(
					cacheRoot,
					plan.Key,
					record,
				); err != nil {
					existing, found, _ := loadPresentationLocalizationCache(
						cacheRoot,
						plan.Key,
						plan.IdentityJSON,
					)
					if !found || !presentationLocalizationBatchCacheProjectionValid(
						plan.Batch,
						existing,
					) {
						outcome.CacheWriteErr = true
					} else {
						cacheAlreadyValid = true
						cacheWinner = &existing.Projection
					}
				}
			}
			if cacheAlreadyValid && cacheWinner != nil {
				projection = *cacheWinner
				projections[index] = projection
			}
		}
	}

	projection, err := localization.CombineBatchProjections(
		prepared.Canonical,
		prepared.Input,
		localizationBatches(plans),
		projections,
	)
	if err != nil {
		return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageProjectionApply, report.LocalizationValidationBatchCombination)
	}
	if _, result, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		projection,
	); err != nil || result.Fallback || len(result.Diagnostics) != 0 {
		return fail(report.LocalizationFailureInvalidProjection, report.LocalizationStageProjectionQuality, report.LocalizationValidationPresentationApply)
	}
	requestSetJSON, err := json.Marshal(requestSHAs)
	if err != nil {
		return fail(report.LocalizationFailurePreparation, report.LocalizationStageStatusWrite, report.LocalizationValidationStatus)
	}
	cacheSetJSON, err := json.Marshal(cacheKeys)
	if err != nil {
		return fail(report.LocalizationFailurePreparation, report.LocalizationStageStatusWrite, report.LocalizationValidationStatus)
	}
	requestSHA := sha256Hex(requestSetJSON)
	cacheKey := "translation-" + sha256Hex(cacheSetJSON)
	if err := report.WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		projection,
		allCacheHits,
		requestSHA,
		cacheKey,
		report.PresentationLocalizationProgress{
			BatchTotal: outcome.BatchTotal, BatchAttempted: outcome.BatchAttempted,
			BatchCompleted: outcome.BatchCompleted,
		},
	); err != nil {
		outcome.State = report.PresentationLocalizationFailed
		outcome.ReasonCode = report.LocalizationFailurePreparation
		outcome.FailureStage = report.LocalizationStageStatusWrite
		outcome.ValidationCode = report.LocalizationValidationStatus
		return outcome, err
	}
	outcome.State = report.PresentationLocalizationSucceeded
	outcome.CacheHit = allCacheHits
	return outcome, nil
}

func recordPresentationLocalizationSemanticExchange(
	writer *debugdump.Writer,
	plan presentationLocalizationBatchPlan,
	index int,
	response []byte,
	unavailable *debugdump.SemanticUnavailable,
	requestProvenance string,
	state string,
	validationCode string,
	semanticCalls int,
	transportAttempts int,
) {
	if writer == nil {
		return
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage:                  debugdump.SemanticStageLocalization,
		InstanceOrdinal:        index + 1,
		SemanticAttemptOrdinal: 1,
		RequestProvenance:      requestProvenance,
		State:                  state,
		ValidationCode:         validationCode,
		SemanticCalls:          semanticCalls,
		TransportAttempts:      transportAttempts,
		Request:                plan.Request.Body,
		Response:               response,
		ResponseUnavailable:    unavailable,
	})
}

func localizationUnavailableResponse(
	response []byte,
	code string,
) *debugdump.SemanticUnavailable {
	if len(response) > 0 {
		return nil
	}
	return &debugdump.SemanticUnavailable{Code: code}
}

// presentationLocalizationUnsafeResponseAttribution runs only after the
// mandatory raw response scan has already rejected the response. It decodes
// through the existing strict provider contract solely to identify which
// batch-local translation contained unsafe material. Provider text and decode
// errors never leave this function.
func presentationLocalizationUnsafeResponseAttribution(
	canonical localization.CanonicalArtifact,
	input localization.Input,
	response []byte,
	detectedKind string,
) (string, int) {
	unsafeKind := presentationLocalizationUnsafeKind(detectedKind)
	projection, err := localization.DecodeRussianProviderResponse(
		canonical,
		input,
		response,
	)
	if err != nil {
		return unsafeKind, 0
	}
	for index, field := range input.Fields {
		translation, ok := projection.Translations[field.ID]
		if !ok {
			return unsafeKind, 0
		}
		translationKind, found := secretscan.DetectAlways(translation)
		if found {
			return presentationLocalizationUnsafeKind(translationKind), index + 1
		}
	}
	return unsafeKind, 0
}

func presentationLocalizationUnsafeKind(kind string) string {
	return secretscan.ClosedKind(kind)
}

func buildPresentationLocalizationBatchPlans(
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
) ([]presentationLocalizationBatchPlan, error) {
	batches, err := localization.BuildBatches(prepared.Canonical, prepared.Input)
	if err != nil {
		return nil, localizationStageError(
			report.LocalizationStageBatchPartition,
			report.LocalizationValidationPayloadBudget,
			report.LocalizationFailurePayloadTooLarge,
			err,
		)
	}
	plans := make([]presentationLocalizationBatchPlan, 0, len(batches))
	for _, batch := range batches {
		prompt, err := localization.BuildRussianPrompt(batch.Canonical, batch.Input)
		if err != nil {
			return nil, localizationStageError(
				report.LocalizationStagePromptBuild,
				report.LocalizationValidationPayloadBudget,
				report.LocalizationFailurePayloadTooLarge,
				err,
			)
		}
		request, err := provider.BuildLocalizationRequest(prompt)
		if err != nil {
			return nil, localizationStageError(
				report.LocalizationStageProviderConfiguration,
				report.LocalizationValidationRequestIdentity,
				report.LocalizationFailureProviderConfig,
				errors.Join(errPresentationLocalizationProviderConfiguration, err),
			)
		}
		if err := request.Validate(prompt); err != nil {
			return nil, localizationStageError(
				report.LocalizationStageProviderConfiguration,
				report.LocalizationValidationRequestIdentity,
				report.LocalizationFailureProviderConfig,
				errors.Join(errPresentationLocalizationProviderConfiguration, err),
			)
		}
		identity := presentationLocalizationCacheIdentity{
			Version:                    presentationLocalizationCacheVersion,
			ContractVersion:            report.PresentationLocalizationContractVersion,
			TranslationContractVersion: localization.PromptVersion,
			TargetLocale:               localization.LocaleRussian,
			BatchContractVersion:       batch.Manifest.Version,
			BatchContentSHA256:         batch.Manifest.ContentSHA256,
			Request:                    request,
		}
		key, identityJSON, err := presentationLocalizationCacheKey(identity)
		if err != nil {
			return nil, localizationStageError(
				report.LocalizationStagePromptBuild,
				report.LocalizationValidationRequestIdentity,
				report.LocalizationFailurePreparation,
				err,
			)
		}
		plans = append(plans, presentationLocalizationBatchPlan{
			Batch: batch, Prompt: prompt, Request: request, Identity: identity,
			Key: key, IdentityJSON: identityJSON, RequestSHA: sha256Hex(request.Body),
		})
	}
	return plans, nil
}

func localizationBatches(plans []presentationLocalizationBatchPlan) []localization.Batch {
	batches := make([]localization.Batch, len(plans))
	for index, plan := range plans {
		batches[index] = plan.Batch
	}
	return batches
}

func samePresentationLocalizationRequest(
	left,
	right deepseek.LocalizationRequestEvidence,
) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func presentationLocalizationBatchProjectionValid(
	batch localization.Batch,
	projection localization.Projection,
) bool {
	encoded, err := json.Marshal(projection)
	if err != nil {
		return false
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return false
	}
	result, err := localization.Apply(batch.Canonical, batch.Input, projection)
	return err == nil && !result.Fallback && len(result.Diagnostics) == 0
}

func presentationLocalizationBatchCacheProjectionValid(
	batch localization.Batch,
	record presentationLocalizationCacheRecord,
) bool {
	return record.Version == presentationLocalizationCacheVersion &&
		presentationLocalizationBatchProjectionValid(batch, record.Projection)
}

func presentationLocalizationCacheProjectionValid(
	data *report.ReportData,
	prepared report.PreparedPresentationLocalization,
	record presentationLocalizationCacheRecord,
) bool {
	encoded, err := json.Marshal(record.Projection)
	if err != nil {
		return false
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return false
	}
	_, result, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		record.Projection,
	)
	return err == nil && !result.Fallback && len(result.Diagnostics) == 0
}

func savePresentationLocalizationFailure(
	runDir,
	reasonCode,
	canonicalSHA string,
) (presentationLocalizationOutcome, error) {
	err := report.WritePresentationLocalizationFailure(runDir, report.PresentationLocalizationFailure{
		ReasonCode: reasonCode, FailureStage: report.LocalizationStageUnavailable,
		ValidationCode:  localizationUnavailableValidationCode(reasonCode),
		CanonicalSHA256: canonicalSHA,
	})
	return presentationLocalizationOutcome{
		State:      report.PresentationLocalizationFailed,
		ReasonCode: reasonCode,
	}, err
}

func localizationUnavailableValidationCode(reasonCode string) string {
	switch reasonCode {
	case report.LocalizationFailureOfflineRequested:
		return report.LocalizationValidationOffline
	case report.LocalizationFailureCacheCorrupt, report.LocalizationFailureCacheUnavailable:
		return report.LocalizationValidationCache
	case report.LocalizationFailureSavedProjection:
		return report.LocalizationValidationSavedProjection
	default:
		return report.LocalizationValidationStatus
	}
}

func presentationLocalizationCacheKey(
	identity presentationLocalizationCacheIdentity,
) (string, []byte, error) {
	if strings.TrimSpace(identity.Version) == "" ||
		strings.TrimSpace(identity.ContractVersion) == "" ||
		strings.TrimSpace(identity.TranslationContractVersion) == "" ||
		strings.TrimSpace(identity.TargetLocale) == "" ||
		identity.BatchContractVersion != localization.BatchManifestVersion ||
		!validSHA256Hex(identity.BatchContentSHA256) {
		return "", nil, fmt.Errorf("localization cache: invalid identity")
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", nil, fmt.Errorf("localization cache: encode identity: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > deepseek.MaxLocalizationRequestBodyBytes*2 {
		return "", nil, fmt.Errorf("localization cache: identity exceeds its byte limit")
	}
	return "translation-" + sha256Hex(encoded), encoded, nil
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func loadPresentationLocalizationCache(
	cacheRoot,
	key string,
	identityJSON []byte,
) (presentationLocalizationCacheRecord, bool, bool) {
	record, found, corrupt, _ := loadPresentationLocalizationCacheObserved(
		cacheRoot,
		key,
		identityJSON,
	)
	return record, found, corrupt
}

func loadPresentationLocalizationCacheObserved(
	cacheRoot,
	key string,
	identityJSON []byte,
) (
	presentationLocalizationCacheRecord,
	bool,
	bool,
	presentationLocalizationCacheObservation,
) {
	if !validPresentationLocalizationCacheKey(key) {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return presentationLocalizationCacheRecord{}, false, false,
			presentationLocalizationCacheObservation{}
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxPresentationLocalizationCacheBytes {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	file, err := os.Open(path)
	if err != nil {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	data, err := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	if err != nil || len(data) == 0 ||
		len(data) > maxPresentationLocalizationCacheBytes {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	observed := presentationLocalizationCacheObservation{info: opened, data: data}
	if !utf8.Valid(data) {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	var record presentationLocalizationCacheRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	recordIdentityJSON, err := json.Marshal(record.Identity)
	if err != nil ||
		record.Version != presentationLocalizationCacheVersion ||
		record.Key != key ||
		!bytes.Equal(recordIdentityJSON, identityJSON) {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	return record, true, false, observed
}

func removeCorruptPresentationLocalizationCache(
	cacheRoot,
	key string,
	observed presentationLocalizationCacheObservation,
) (bool, error) {
	if !validPresentationLocalizationCacheKey(key) {
		return false, fmt.Errorf("localization cache: invalid corrupt entry key")
	}
	if observed.info == nil || len(observed.data) == 0 ||
		len(observed.data) > maxPresentationLocalizationCacheBytes {
		return false, fmt.Errorf(
			"localization cache: corrupt entry has no stable observation",
		)
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	// Serialize corrupt-entry cleanup for this content identity. Writers publish with a
	// no-replace hard link, so while the observed corrupt path exists they
	// cannot replace it. Holding this claim through the conditional unlink
	// closes the only writer/remover race: a second cleanup cannot remove a
	// valid entry published after this cleanup releases the path.
	claimPath := path + ".cleanup.lock"
	claim, err := os.OpenFile(
		claimPath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"localization cache: claim corrupt entry cleanup: %w",
			err,
		)
	}
	defer func() {
		_ = claim.Close()
		_ = os.Remove(claimPath)
	}()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || !os.SameFile(observed.info, info) ||
		info.Size() != int64(len(observed.data)) {
		return false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, nil
	}
	opened, statErr := file.Stat()
	data, readErr := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	closeErr := file.Close()
	if statErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(info, opened) || readErr != nil || closeErr != nil ||
		!bytes.Equal(data, observed.data) {
		return false, nil
	}
	// Recheck the directory entry after reading. This closes the practical
	// replacement window and, together with the inode and byte comparison,
	// ensures a different first-valid-wins entry is never intentionally
	// removed.
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil || !os.SameFile(opened, current) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("localization cache: remove corrupt entry: %w", err)
	}
	return true, nil
}

func writePresentationLocalizationCache(
	cacheRoot,
	key string,
	record presentationLocalizationCacheRecord,
) error {
	if !validPresentationLocalizationCacheKey(key) ||
		record.Key != key ||
		record.Version != presentationLocalizationCacheVersion {
		return fmt.Errorf("localization cache: invalid record")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("localization cache: encode record: %w", err)
	}
	data = append(data, '\n')
	if len(data) == 0 || len(data) > maxPresentationLocalizationCacheBytes {
		return fmt.Errorf("localization cache: record exceeds its byte limit")
	}
	versionDir := filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		return fmt.Errorf("localization cache: create directory: %w", err)
	}
	if info, err := os.Lstat(versionDir); err != nil ||
		info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("localization cache: directory is unavailable")
	}
	path := filepath.Join(versionDir, key+".json")
	temporary, err := os.CreateTemp(versionDir, ".translation-")
	if err != nil {
		return fmt.Errorf("localization cache: create temporary record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("localization cache: protect temporary record: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("localization cache: write record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("localization cache: sync record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("localization cache: close record: %w", err)
	}
	// Cache keys are content identities. Publish without replacement so two
	// concurrent misses can never turn one key into last-writer-wins state.
	if err := os.Link(temporaryPath, path); err != nil {
		if presentationLocalizationCacheFileMatches(path, data) {
			return nil
		}
		return fmt.Errorf("localization cache: publish immutable record: %w", err)
	}
	directory, err := os.Open(versionDir)
	if err != nil {
		return fmt.Errorf("localization cache: open directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("localization cache: sync directory: %w", err)
	}
	return nil
}

func presentationLocalizationCacheFileMatches(path string, want []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() != int64(len(want)) ||
		info.Size() <= 0 || info.Size() > maxPresentationLocalizationCacheBytes {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return false
	}
	got, err := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	return err == nil && bytes.Equal(got, want)
}

func validPresentationLocalizationCacheKey(value string) bool {
	const prefix = "translation-"
	if !strings.HasPrefix(value, prefix) ||
		len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
