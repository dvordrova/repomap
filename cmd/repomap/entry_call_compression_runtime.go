package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
)

type entryCallCompressionClient interface {
	EntryCallCompressionPromptJSON(entrycall.Prompt) ([]byte, error)
	EntryCallCompressionBodyMeasured(context.Context, []byte) (modelresearch.ProviderResult, error)
}

type entryCallCompressionClientFactory func() (entryCallCompressionClient, error)

type entryCallCompressionRunOutcome struct {
	Status            entrycall.Status
	SemanticCalls     int
	TransportAttempts int
	RequestBytes      int
	ResponseBytes     int
	LatencyMillis     int64
	Canceled          bool
}

func defaultEntryCallCompressionClientFactory() (entryCallCompressionClient, error) {
	return deepseek.NewFromEnv()
}

// runEntryCallCompressionForRun is an optional bounded semantic stage. An
// accepted artifact pair is report material and must be written before final
// publication; every closed status remains diagnostic-only. Integrity and
// persistence failures are returned so the caller can remove any incomplete
// artifact prefix before generating the report.
func runEntryCallCompressionForRun(
	ctx context.Context,
	runDir string,
	substrate *entrycall.Substrate,
	repositoryState freshness.RepositoryState,
	output *runOutput,
	clients entryCallCompressionClientFactory,
) (entryCallCompressionRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if clients == nil {
		return entryCallCompressionRunOutcome{}, fmt.Errorf("entry call compression: client factory is required")
	}
	if err := resetEntryCallCompressionArtifacts(runDir); err != nil {
		return entryCallCompressionRunOutcome{}, err
	}
	repositoryStateSHA256, err := repositoryState.Digest()
	if err != nil {
		return entryCallCompressionRunOutcome{}, fmt.Errorf("entry call compression: bind repository state: %w", err)
	}

	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return entryCallCompressionRunOutcome{}, fmt.Errorf("entry call compression: open artifact writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Entry-call semantic exchange journal unavailable",
	})

	outcome := entryCallCompressionRunOutcome{}
	if substrate == nil || substrate.State != entrycall.StateReady {
		outcome.Status = entrycall.Status{
			Version: entrycall.StatusVersion, State: entrycall.StatusUnavailable,
			Reason: entrycall.ReasonSubstrateUnavailable, PromptVersion: entrycall.PromptVersion,
			RepositoryStateSHA256: repositoryStateSHA256,
		}
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}

	compilation, err := entrycall.Compile(substrate.Snapshot())
	if err != nil {
		return outcome, fmt.Errorf("entry call compression: compile exact call families: %w", err)
	}
	baseStatus := entrycall.Status{
		Version: entrycall.StatusVersion, PromptVersion: entrycall.PromptVersion,
		RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.RequestSHA256(),
		SubstrateSHA256:       compilation.SubstrateSHA256,
		RepositoryStateSHA256: repositoryStateSHA256,
		AdvertisedFamilies:    compilation.AdvertisedFamilyCount(),
	}
	if baseStatus.AdvertisedFamilies == 0 {
		baseStatus.State = entrycall.StatusSkipped
		baseStatus.Reason = entrycall.ReasonNoFamilies
		outcome.Status = baseStatus
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}

	prompt, err := entrycall.BuildPrompt(compilation)
	if err != nil {
		return outcome, fmt.Errorf("entry call compression: build prompt: %w", err)
	}
	bundle, err := entrycall.ProviderVisibleJSON(compilation)
	if err != nil {
		return outcome, fmt.Errorf("entry call compression: build provider-visible bundle: %w", err)
	}
	outcome.RequestBytes = len(bundle)
	client, clientErr := clients()
	if clientErr != nil || client == nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entrycall.ReasonConfigurationFailed
		outcome.Status = baseStatus
		recordEntryCallCompressionExchange(
			writer, bundle, nil, nil, debugdump.SemanticRequestPrepared,
			0, 0, debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
		)
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}
	envelope, envelopeErr := client.EntryCallCompressionPromptJSON(prompt)
	if envelopeErr != nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entryCallCompressionFailureReason(envelopeErr, false)
		outcome.Status = baseStatus
		recordEntryCallCompressionExchange(
			writer, bundle, nil, nil, debugdump.SemanticRequestPrepared,
			0, 0, debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
		)
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}
	outcome.RequestBytes = len(envelope)
	if err := scanEntryCallCompressionBytes("provider request", envelope); err != nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entrycall.ReasonConfigurationFailed
		outcome.Status = baseStatus
		recordEntryCallCompressionExchange(
			writer, envelope, nil, nil, debugdump.SemanticRequestPrepared,
			0, 0, debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
		)
		if persistErr := persistEntryCallCompressionStatus(writer, outcome.Status); persistErr != nil {
			return outcome, persistErr
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entrycall.ReasonCanceled
		outcome.Status = baseStatus
		outcome.Canceled = true
		recordEntryCallCompressionExchange(
			writer, envelope, nil, &debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableCanceled},
			debugdump.SemanticRequestPrepared, 0, 0,
			debugdump.SemanticStateCanceled, debugdump.SemanticValidationCanceled,
		)
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}

	output.Stage(
		"Generic entry-call experiment",
		fmt.Sprintf("advertised exact call families: %d", baseStatus.AdvertisedFamilies),
		"compressing one bounded refs-only call graph",
	)
	started := time.Now()
	providerResult, callErr := client.EntryCallCompressionBodyMeasured(ctx, envelope)
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.TransportAttempts = providerResult.Attempts
	outcome.ResponseBytes = providerResultResponseBytes(providerResult)
	if providerResult.Attempts > 0 {
		outcome.SemanticCalls = 1
	}
	if callErr != nil {
		reason := entryCallCompressionFailureReason(callErr, providerResult.Attempts > 0)
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = reason
		outcome.Status = baseStatus
		outcome.Canceled = reason == entrycall.ReasonCanceled
		response := providerFailureContentForExchange(callErr, providerResult.Content)
		responseUnavailable := entryCallCompressionUnavailableResponse(reason, response, outcome.ResponseBytes)
		if responseUnavailable == nil && len(response) > 0 {
			if scanErr := scanEntryCallCompressionBytes("provider response", response); scanErr != nil {
				baseStatus.Reason = entrycall.ReasonResponseRejected
				outcome.Status = baseStatus
				recordEntryCallCompressionExchange(
					writer, envelope, response, nil, debugdump.SemanticRequestExactSent,
					outcome.SemanticCalls, outcome.TransportAttempts,
					debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
				)
				if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
					return outcome, err
				}
				writeEntryCallCompressionOutcome(output, outcome)
				return outcome, nil
			}
		}
		recordEntryCallCompressionExchange(
			writer, envelope, response, responseUnavailable, debugdump.SemanticRequestExactSent,
			outcome.SemanticCalls, outcome.TransportAttempts,
			entryCallCompressionSemanticState(reason), entryCallCompressionValidationCode(reason),
		)
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}

	if err := scanEntryCallCompressionBytes("provider response", providerResult.Content); err != nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entrycall.ReasonResponseRejected
		outcome.Status = baseStatus
		recordEntryCallCompressionExchange(
			writer, envelope, providerResult.Content, nil, debugdump.SemanticRequestExactSent,
			outcome.SemanticCalls, outcome.TransportAttempts,
			debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret,
		)
		if persistErr := persistEntryCallCompressionStatus(writer, outcome.Status); persistErr != nil {
			return outcome, persistErr
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}
	result, validationErr := entrycall.Reduce(compilation, providerResult.Content)
	if validationErr != nil {
		baseStatus.State = entrycall.StatusRejected
		baseStatus.Reason = entrycall.ReasonResponseRejected
		outcome.Status = baseStatus
		recordEntryCallCompressionExchange(
			writer, envelope, providerResult.Content, nil, debugdump.SemanticRequestExactSent,
			outcome.SemanticCalls, outcome.TransportAttempts,
			debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse,
		)
		if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
			return outcome, err
		}
		writeEntryCallCompressionOutcome(output, outcome)
		return outcome, nil
	}
	result.RepositoryStateSHA256 = repositoryStateSHA256
	resultRaw, err := entrycall.EncodeResult(result)
	if err != nil {
		return outcome, fmt.Errorf("entry call compression: encode result: %w", err)
	}
	if err := scanEntryCallCompressionBytes("result artifact", resultRaw); err != nil {
		return outcome, err
	}
	if err := writer.WriteValidatedFile(
		entrycall.ResultArtifactFilename,
		resultRaw,
		func(saved []byte) error {
			_, decodeErr := entrycall.DecodeResult(saved)
			return decodeErr
		},
	); err != nil {
		return outcome, fmt.Errorf("entry call compression: persist result: %w", err)
	}
	baseStatus.State = entrycall.StatusAccepted
	baseStatus.SelectedFamilies = result.SelectedFamilyCount()
	baseStatus.RejectedFamilies = result.RejectedFamilyCount()
	if baseStatus.RejectedFamilies > 0 {
		baseStatus.State = entrycall.StatusAcceptedPartial
		baseStatus.Reason = entrycall.ReasonResponsePartial
	}
	baseStatus.ResultSHA256 = modelresearch.SHA256(resultRaw)
	outcome.Status = baseStatus
	recordEntryCallCompressionExchange(
		writer, envelope, providerResult.Content, nil, debugdump.SemanticRequestExactSent,
		outcome.SemanticCalls, outcome.TransportAttempts,
		debugdump.SemanticStateAccepted, debugdump.SemanticValidationAccepted,
	)
	if err := persistEntryCallCompressionStatus(writer, outcome.Status); err != nil {
		return outcome, err
	}
	writeEntryCallCompressionOutcome(output, outcome)
	return outcome, nil
}

func resetEntryCallCompressionArtifacts(runDir string) error {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("entry call compression: open run directory for reset: %w", err)
	}
	defer root.Close()
	for _, name := range entrycall.ArtifactFilenames {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("entry call compression: reset artifact %s: %w", name, err)
		}
	}
	return nil
}

func persistEntryCallCompressionStatus(writer *debugdump.Writer, status entrycall.Status) error {
	statusRaw, err := entrycall.EncodeStatus(status)
	if err != nil {
		return fmt.Errorf("entry call compression: encode status: %w", err)
	}
	if err := scanEntryCallCompressionBytes("status artifact", statusRaw); err != nil {
		return err
	}
	if err := writer.WriteValidatedFile(
		entrycall.StatusArtifactFilename,
		statusRaw,
		func(saved []byte) error {
			_, decodeErr := entrycall.DecodeStatus(saved)
			return decodeErr
		},
	); err != nil {
		return fmt.Errorf("entry call compression: persist status: %w", err)
	}
	return nil
}

func scanEntryCallCompressionBytes(label string, data []byte) error {
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf(
			"entry call compression: %s failed credential scan (%s)",
			label,
			secretscan.ClosedKind(kind),
		)
	}
	return nil
}

func entryCallCompressionFailureReason(err error, attempted bool) entrycall.StatusReason {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return entrycall.ReasonCanceled
	}
	var limit *modelresearch.ResourceLimitError
	if errors.As(err, &limit) {
		if !attempted && limit.Kind == modelresearch.ResourceLimitRequestBytes {
			return entrycall.ReasonConfigurationFailed
		}
		return entrycall.ReasonOutputLimit
	}
	if !attempted {
		return entrycall.ReasonConfigurationFailed
	}
	return entrycall.ReasonProviderFailed
}

func entryCallCompressionUnavailableResponse(
	reason entrycall.StatusReason,
	response []byte,
	responseBytes int,
) *debugdump.SemanticUnavailable {
	if len(response) > entrycall.MaxResponseBytes || reason == entrycall.ReasonOutputLimit && responseBytes > entrycall.MaxResponseBytes {
		return &debugdump.SemanticUnavailable{
			Code:           debugdump.SemanticUnavailableSize,
			OriginalSHA256: modelresearch.SHA256(response),
			OriginalBytes:  responseBytes,
		}
	}
	if len(response) == 0 {
		code := debugdump.SemanticUnavailableNoContent
		if reason == entrycall.ReasonCanceled {
			code = debugdump.SemanticUnavailableCanceled
		}
		return &debugdump.SemanticUnavailable{Code: code, OriginalBytes: responseBytes}
	}
	return nil
}

func entryCallCompressionSemanticState(reason entrycall.StatusReason) string {
	if reason == entrycall.ReasonCanceled {
		return debugdump.SemanticStateCanceled
	}
	return debugdump.SemanticStateProviderFailed
}

func entryCallCompressionValidationCode(reason entrycall.StatusReason) string {
	if reason == entrycall.ReasonCanceled {
		return debugdump.SemanticValidationCanceled
	}
	return debugdump.SemanticValidationProvider
}

func recordEntryCallCompressionExchange(
	writer *debugdump.Writer,
	request []byte,
	response []byte,
	unavailable *debugdump.SemanticUnavailable,
	provenance string,
	semanticCalls int,
	transportAttempts int,
	state string,
	validationCode string,
) {
	if writer == nil {
		return
	}
	if len(response) == 0 && unavailable == nil {
		unavailable = &debugdump.SemanticUnavailable{Code: debugdump.SemanticUnavailableNoContent}
	}
	if unavailable != nil {
		response = nil
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageEntryCallCompression,
		InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: provenance,
		State:             state, ValidationCode: validationCode,
		SemanticCalls: semanticCalls, TransportAttempts: transportAttempts,
		Request: request, Response: response, ResponseUnavailable: unavailable,
	})
}

func writeEntryCallCompressionOutcome(output *runOutput, outcome entryCallCompressionRunOutcome) {
	if output == nil || outcome.Status.PromptVersion == "" {
		return
	}
	details := []string{
		fmt.Sprintf("provider calls: %d", outcome.SemanticCalls),
		fmt.Sprintf("transport attempts: %d", outcome.TransportAttempts),
		fmt.Sprintf("advertised families: %d", outcome.Status.AdvertisedFamilies),
		fmt.Sprintf("selected families: %d", outcome.Status.SelectedFamilies),
		fmt.Sprintf("rejected unreachable families: %d", outcome.Status.RejectedFamilies),
	}
	if outcome.Status.Reason != entrycall.ReasonNone {
		details = append(details, "reason: "+string(outcome.Status.Reason))
	}
	output.State("Generic entry-call experiment", string(outcome.Status.State), details...)
}

func recordEntryCallCompressionDiagnostic(
	runDir string,
	outcome entryCallCompressionRunOutcome,
) error {
	state := "failed"
	if outcome.Status.PromptVersion != "" {
		switch outcome.Status.State {
		case entrycall.StatusAccepted:
			state = "accepted"
		case entrycall.StatusAcceptedPartial:
			state = "accepted_partial"
		case entrycall.StatusSkipped:
			state = "skipped"
		case entrycall.StatusUnavailable:
			state = "unavailable"
		case entrycall.StatusRejected:
			switch outcome.Status.Reason {
			case entrycall.ReasonCanceled:
				state = "canceled"
			case entrycall.ReasonOutputLimit:
				state = "resource_exhausted"
			case entrycall.ReasonResponseRejected:
				state = "response_validation_failed"
			case entrycall.ReasonProviderFailed:
				state = "provider_failed"
			default:
				state = "failed"
			}
		}
	}
	return recordAtlasFirstStageDiagnostic(runDir, atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageEntryCallCompression, State: state,
		RequestBytes: outcome.RequestBytes, SemanticCalls: outcome.SemanticCalls,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	})
}

// entryCallCompressionPublicationContext gives an already-started optional
// entry-call request enough bounded time to reconcile its durable closed status
// into an otherwise valid report after the caller cancels.
func entryCallCompressionPublicationContext(
	ctx context.Context,
	outcome entryCallCompressionRunOutcome,
) (context.Context, context.CancelFunc) {
	if outcome.Canceled && ctx.Err() != nil {
		return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	}
	return ctx, func() {}
}
