package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	navigatorCacheContract = "atlas-first-navigator-accepted-v1"
	navigatorCacheStage    = "navigator"
	canonicalOutputEnglish = "en"

	maxNavigatorSeeds         = 512
	maxNavigatorDirectTrails  = 512
	maxNavigatorIntersections = 512
	maxNavigatorEvidence      = 1024
	maxNavigatorActions       = 512
)

type navigatorRunOutcome struct {
	Empty             bool
	ProviderSkipped   bool
	Cached            bool
	ActionCount       int
	Selected          *navigator.RecommendationAction
	RequestBytes      int
	ResponseBytes     int
	InputTokens       int
	OutputTokens      int
	TransportAttempts int
	LatencyMillis     int64
}

type navigatorAcceptedResponse struct {
	raw    []byte
	record navigator.RecommendationRecord
}

type navigatorResponseFailure string

const (
	navigatorFailureSecret    navigatorResponseFailure = "secret"
	navigatorFailureDecode    navigatorResponseFailure = "decode"
	navigatorFailureReference navigatorResponseFailure = "reference"
)

// runNavigatorForRun owns the one ordinary Atlas-first semantic decision.
// The local Atlas already exists under the confined run root before this
// function starts; every terminal error returns before report authority,
// latest, serving, or browser opening can occur.
func runNavigatorForRun(
	ctx context.Context,
	runDir string,
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	providerEnabled bool,
	output *runOutput,
) (navigatorRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	output.Stage("Navigator", "compiling exact application startup actions")
	atlas, err := readNavigatorAtlas(runDir)
	if err != nil {
		return navigatorRunOutcome{}, fmt.Errorf("navigator run: %w", err)
	}
	product, err := navigator.CompileProduct(navigator.ProductInput{
		Atlas: atlas, Limits: ordinaryNavigatorLimits(policy),
	})
	if err != nil {
		return navigatorRunOutcome{}, fmt.Errorf("navigator run: compile product: %w", err)
	}
	outcome := navigatorRunOutcome{ActionCount: len(product.Actions())}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return outcome, fmt.Errorf("navigator run: open confined artifact writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(navigatorJournalWarningWriter{output: output})

	if product.Empty() {
		record, err := product.EmptyRecord()
		if err != nil {
			return outcome, err
		}
		if err := persistNavigatorResult(writer, product, atlas, record); err != nil {
			return outcome, err
		}
		if err := persistNavigatorStatus(writer, product.PreparedStatus()); err != nil {
			return outcome, err
		}
		outcome.Empty = true
		output.State("Navigator", "empty", "eligible startup actions: 0", "provider calls: 0")
		return outcome, nil
	}
	requestRecord, err := product.RequestRecord()
	if err != nil {
		return outcome, err
	}
	if err := persistNavigatorRequest(writer, product, requestRecord); err != nil {
		return outcome, err
	}
	if !providerEnabled {
		status, err := product.UnavailableStatus(navigator.UnavailableOffline)
		if err != nil {
			return outcome, err
		}
		if err := persistNavigatorStatus(writer, status); err != nil {
			return outcome, err
		}
		outcome.ProviderSkipped = true
		output.State(
			"Navigator", "unavailable",
			fmt.Sprintf("eligible startup actions: %d", outcome.ActionCount),
			"provider calls: 0", "reason: offline requested",
		)
		return outcome, nil
	}
	if err := persistNavigatorStatus(writer, product.PreparedStatus()); err != nil {
		return outcome, err
	}
	compiled, ok := product.CompiledRequest()
	if !ok {
		return outcome, fmt.Errorf("navigator run: compiled request is unavailable")
	}
	promptClient, err := deepseek.NewPromptFromEnv()
	if err != nil {
		return outcome, failNavigatorRun(writer, product, navigator.FailureProvider, err)
	}
	providerRequest, err := promptClient.NavigatorPromptJSON(
		compiled.WireJSON(), compiled.MaxWireBytes(),
	)
	if err != nil {
		code := navigator.FailureProvider
		if isSemanticResourceLimit(err) {
			code = navigator.FailureResource
		}
		return outcome, failNavigatorRun(writer, product, code, err)
	}
	outcome.RequestBytes = len(providerRequest)
	config := promptClient.EffectiveConfig()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(config.Endpoint)
	if err != nil {
		return outcome, failNavigatorRun(writer, product, navigator.FailureProvider, err)
	}
	cacheInput := navigatorStageCacheInput(
		runsDir, repository, policy, config, endpointSHA,
		product.AtlasSHA256(), providerRequest,
	)

	if !noCache {
		cached, found, loadErr := modelresearch.LoadStageResponse(cacheInput)
		if loadErr != nil {
			return outcome, failNavigatorRun(writer, product, navigator.FailureProvider, loadErr)
		}
		if found {
			accepted, failure, validationErr := validateNavigatorResponse(product, cached.Content)
			if validationErr == nil {
				outcome.Cached = true
				outcome.ResponseBytes = len(cached.Content)
				outcome.InputTokens = cached.InputTokens
				outcome.OutputTokens = cached.OutputTokens
				outcome.LatencyMillis = cached.LatencyMillis
				selected := *accepted.record.Selected
				outcome.Selected = &selected
				recordNavigatorExchange(
					writer, providerRequest, accepted.raw, nil,
					debugdump.SemanticStateCacheHit, debugdump.SemanticValidationCache,
					0, 0, debugdump.SemanticRequestPrepared,
				)
				if err := persistNavigatorResult(writer, product, atlas, accepted.record); err != nil {
					return outcome, err
				}
				status, err := product.SelectedStatus(accepted.record)
				if err != nil {
					return outcome, err
				}
				if err := persistNavigatorStatus(writer, status); err != nil {
					return outcome, err
				}
				output.State(
					"Navigator", "cached",
					fmt.Sprintf("eligible startup actions: %d", outcome.ActionCount),
					"provider calls: 0",
				)
				return outcome, nil
			}
			if err := modelresearch.InvalidateStageResponse(cacheInput); err != nil {
				return outcome, failNavigatorRun(
					writer, product, navigator.FailureProvider,
					fmt.Errorf("evict invalid Navigator cache entry after %s validation: %w", failure, err),
				)
			}
		}
	}

	liveClient, err := deepseek.NewFromEnv()
	if err != nil {
		return outcome, failNavigatorRun(writer, product, navigator.FailureProvider, err)
	}
	if liveClient.EffectiveConfig() != config {
		return outcome, failNavigatorRun(
			writer, product, navigator.FailureProvider,
			fmt.Errorf("navigator run: provider configuration changed after exact request preparation"),
		)
	}
	liveClient.OnWait = func(progress deepseek.WaitProgress) {
		output.Stage(
			"Navigator",
			strings.TrimSpace(progress.Stage)+" is still running",
			"elapsed: "+progress.Elapsed.Round(time.Second).String(),
			"Ctrl-C to cancel",
		)
	}
	output.State(
		"Navigator", "request prepared",
		fmt.Sprintf("eligible startup actions: %d", outcome.ActionCount),
		fmt.Sprintf("request bytes: %d", len(providerRequest)),
		"model: "+config.Model,
	)
	started := time.Now()
	providerResult, callErr := liveClient.NavigateMeasured(
		ctx, compiled.WireJSON(), compiled.MaxWireBytes(),
	)
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.ResponseBytes = providerResultResponseBytes(providerResult)
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.TransportAttempts = providerResult.Attempts
	if callErr != nil {
		code := navigator.FailureProvider
		state := debugdump.SemanticStateProviderFailed
		validation := debugdump.SemanticValidationProvider
		unavailable := debugdump.SemanticUnavailableNoContent
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			code = navigator.FailureCanceled
			state = debugdump.SemanticStateCanceled
			validation = debugdump.SemanticValidationCanceled
			unavailable = debugdump.SemanticUnavailableCanceled
		} else if isSemanticResourceLimit(callErr) {
			code = navigator.FailureResource
		}
		recordNavigatorExchange(
			writer, providerRequest,
			providerFailureContentForExchange(callErr, providerResult.Content),
			&debugdump.SemanticUnavailable{Code: unavailable, OriginalBytes: outcome.ResponseBytes},
			state, validation, 1, providerResult.Attempts,
			debugdump.SemanticRequestExactSent,
		)
		return outcome, failNavigatorRun(writer, product, code, callErr)
	}
	accepted, failure, validationErr := validateNavigatorResponse(product, providerResult.Content)
	if validationErr != nil {
		code := navigator.FailureReference
		validation := debugdump.SemanticValidationResponse
		if failure == navigatorFailureSecret {
			code = navigator.FailureDecode
			validation = debugdump.SemanticValidationSecret
		} else if failure == navigatorFailureDecode {
			code = navigator.FailureDecode
			validation = debugdump.SemanticValidationDecode
		}
		recordNavigatorExchange(
			writer, providerRequest, providerResult.Content, nil,
			debugdump.SemanticStateRejected, validation,
			1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
		)
		return outcome, failNavigatorRun(writer, product, code, validationErr)
	}
	recordNavigatorExchange(
		writer, providerRequest, accepted.raw, nil,
		debugdump.SemanticStateAccepted, debugdump.SemanticValidationAccepted,
		1, providerResult.Attempts, debugdump.SemanticRequestExactSent,
	)
	if err := persistNavigatorResult(writer, product, atlas, accepted.record); err != nil {
		return outcome, err
	}
	status, err := product.SelectedStatus(accepted.record)
	if err != nil {
		return outcome, err
	}
	if err := persistNavigatorStatus(writer, status); err != nil {
		return outcome, err
	}
	selected := *accepted.record.Selected
	outcome.Selected = &selected
	if !noCache {
		_, cacheErr := modelresearch.SaveStageResponse(cacheInput, modelresearch.StageResponse{
			Content:     accepted.raw,
			InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
			PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
			PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
			LatencyMillis:         outcome.LatencyMillis, RetryCount: max(0, providerResult.Attempts-1),
		})
		if cacheErr != nil {
			output.Warn("Navigator cache write failed", "accepted per-run result remains authoritative")
		}
	}
	output.State(
		"Navigator", "accepted",
		fmt.Sprintf("eligible startup actions: %d", outcome.ActionCount),
		fmt.Sprintf("response bytes: %d", outcome.ResponseBytes),
		formatRunOutputTokens(outcome.InputTokens, outcome.OutputTokens),
		fmt.Sprintf("transport attempts: %d", outcome.TransportAttempts),
		formatRunOutputDuration(outcome.LatencyMillis),
	)
	return outcome, nil
}

func ordinaryNavigatorLimits(policy modelresearch.Policy) navigator.Limits {
	return navigator.Limits{
		MaxWireBytes:      policy.Orientation.MaxRequestBytes,
		MaxResponseBytes:  modelresearch.ProviderResponseByteLimit,
		MaxUnitLabelBytes: policy.Orientation.MaxRequestBytes,
		MaxSeeds:          maxNavigatorSeeds, MaxDirectTrails: maxNavigatorDirectTrails,
		MaxIntersections: maxNavigatorIntersections, MaxEvidence: maxNavigatorEvidence,
		MaxGaps: 0, MaxActions: maxNavigatorActions,
	}
}

func navigatorStageCacheInput(
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	config deepseek.EffectiveConfig,
	endpointSHA string,
	atlasSHA string,
	request []byte,
) modelresearch.StageCacheInput {
	return modelresearch.StageCacheInput{
		RunsDir: runsDir,
		Fingerprint: modelresearch.FingerprintInput{
			Repository: repository, Stage: navigatorCacheStage,
			PromptVersion: deepseek.NavigatorPromptVersionJSON,
			CacheContract: navigatorCacheContract,
			Profile:       "openai-compatible/" + config.AuthMode,
			Model:         config.Model, ProviderEndpointSHA256: endpointSHA,
			RequestSHA256:      modelresearch.SHA256(request),
			EvidenceBundleHash: atlasSHA, PolicyVersion: policy.Version,
			OutputLanguage: canonicalOutputEnglish,
		},
		Request: request, EvidenceBundleHash: atlasSHA,
	}
}

func validateNavigatorResponse(
	product navigator.Product,
	raw []byte,
) (navigatorAcceptedResponse, navigatorResponseFailure, error) {
	if kind, found := secretscan.DetectAlways(string(raw)); found {
		return navigatorAcceptedResponse{}, navigatorFailureSecret, fmt.Errorf(
			"navigator run: provider response rejected by mandatory secret scan: kind=%s",
			secretscan.ClosedKind(kind),
		)
	}
	decoded, err := deepseek.DecodeNavigatorResponse(raw)
	if err != nil {
		return navigatorAcceptedResponse{}, navigatorFailureDecode, err
	}
	record, err := product.ResolveRecommendation(decoded)
	if err != nil {
		return navigatorAcceptedResponse{}, navigatorFailureReference, err
	}
	if err := product.ValidateRecommendationRecord(record); err != nil {
		return navigatorAcceptedResponse{}, navigatorFailureReference, err
	}
	return navigatorAcceptedResponse{raw: slices.Clone(raw), record: record}, "", nil
}

func persistNavigatorRequest(
	writer *debugdump.Writer,
	product navigator.Product,
	record navigator.RequestRecord,
) error {
	encoded, err := navigator.EncodeRequestRecord(record)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(navigator.RequestArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := navigator.DecodeRequestRecord(saved)
		if err != nil {
			return err
		}
		return product.ValidateRequestRecord(decoded)
	})
}

func persistNavigatorResult(
	writer *debugdump.Writer,
	product navigator.Product,
	atlas repositoryatlas.Atlas,
	record navigator.RecommendationRecord,
) error {
	encoded, err := navigator.EncodeRecommendationRecord(record)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(navigator.RecordArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := navigator.DecodeRecommendationRecord(saved)
		if err != nil {
			return err
		}
		if err := product.ValidateRecommendationRecord(decoded); err != nil {
			return err
		}
		return navigator.ValidateRecommendationRecordAgainstAtlas(decoded, atlas)
	})
}

func persistNavigatorStatus(writer *debugdump.Writer, status navigator.Status) error {
	encoded, err := navigator.EncodeStatus(status)
	if err != nil {
		return err
	}
	return writer.WriteValidatedFile(navigator.StatusArtifactFilename, encoded, func(saved []byte) error {
		decoded, err := navigator.DecodeStatus(saved)
		if err != nil {
			return err
		}
		if decoded != status {
			return fmt.Errorf("navigator status changed before publication")
		}
		return nil
	})
}

func failNavigatorRun(
	writer *debugdump.Writer,
	product navigator.Product,
	code navigator.FailureCode,
	cause error,
) error {
	status, err := product.FailureStatus(code)
	if err != nil {
		return errors.Join(cause, err)
	}
	if err := persistNavigatorStatus(writer, status); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func recordNavigatorExchange(
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
		Stage:           debugdump.SemanticStageNavigator,
		InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: provenance,
		State:             state, ValidationCode: validation,
		SemanticCalls: semanticCalls, TransportAttempts: transportAttempts,
		Request: request, Response: response, ResponseUnavailable: unavailable,
	})
}

func readNavigatorAtlas(runDir string) (repositoryatlas.Atlas, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("open run root: %w", err)
	}
	defer root.Close()
	info, err := root.Lstat(repositoryatlas.ArtifactFilename)
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("inspect Repository Atlas artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > repositoryatlas.MaxArtifactBytes {
		return repositoryatlas.Atlas{}, fmt.Errorf("Repository Atlas artifact is not a bounded regular file")
	}
	file, err := root.Open(repositoryatlas.ArtifactFilename)
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("open Repository Atlas artifact: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return repositoryatlas.Atlas{}, fmt.Errorf("Repository Atlas artifact changed before reading")
	}
	encoded, err := io.ReadAll(io.LimitReader(file, repositoryatlas.MaxArtifactBytes+1))
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("read Repository Atlas artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > repositoryatlas.MaxArtifactBytes {
		return repositoryatlas.Atlas{}, fmt.Errorf("Repository Atlas artifact exceeds its canonical boundary")
	}
	atlas, err := repositoryatlas.DecodeCanonicalJSON(encoded)
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("decode Repository Atlas artifact: %w", err)
	}
	return atlas, nil
}

type navigatorJournalWarningWriter struct{ output *runOutput }

func (writer navigatorJournalWarningWriter) Write(data []byte) (int, error) {
	if writer.output != nil {
		writer.output.Warn("Navigator semantic exchange journal unavailable")
	}
	return len(data), nil
}

type runOutputWarningSink struct {
	output  *runOutput
	summary string
}

func (writer runOutputWarningSink) Write(data []byte) (int, error) {
	if writer.output != nil {
		detail := strings.TrimSpace(string(data))
		detail = strings.TrimPrefix(detail, "warning: ")
		writer.output.Warn(writer.summary, detail)
	}
	return len(data), nil
}
