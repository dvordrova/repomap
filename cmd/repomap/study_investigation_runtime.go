package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

type studyInvestigationClient interface {
	MechanismStudyPromptJSON(mechanismstudy.Prompt) ([]byte, error)
	MechanismStudyBodyMeasured(context.Context, []byte) (modelresearch.ProviderResult, error)
}

type studyInvestigationClientFactory func() (studyInvestigationClient, error)

type studyInvestigationRunOutcome struct {
	Status            mechanismstudy.Status
	ReportInput       report.StudyInvestigationInput
	SemanticCalls     int
	TransportAttempts int
	RequestBytes      int
	ResponseBytes     int
	LatencyMillis     int64
}

const studyInvestigationPublicationTimeout = 30 * time.Second

func defaultStudyInvestigationClientFactory() (studyInvestigationClient, error) {
	return deepseek.NewFromEnv()
}

// runStudyInvestigationForRun turns the final, already-persisted Study cards
// into a bounded direct-call investigation family. It consumes only the one
// selected AnalysisTarget and live DirectCallIndex handoff from the existing
// surface SSA pass. Provider failures close one planned prefix item and retain
// all earlier exact results; integrity, credential, and persistence failures
// remain terminal.
func runStudyInvestigationForRun(
	ctx context.Context,
	runDir string,
	index *surfacediscovery.DirectCallIndex,
	target analysistarget.Target,
	repositoryRevision string,
	repositoryFreshnessSHA256 string,
	output *runOutput,
	clients studyInvestigationClientFactory,
) (studyInvestigationRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if clients == nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: client factory is required")
	}
	if err := resetStudyInvestigationArtifacts(runDir); err != nil {
		return studyInvestigationRunOutcome{}, err
	}
	themesRaw, err := readStudyInvestigationArtifact(
		runDir,
		themestudy.StudyThemesArtifactFilename,
		themestudy.MaxStudyThemesArtifactBytes,
	)
	if err != nil {
		return studyInvestigationRunOutcome{}, err
	}
	themes, err := themestudy.DecodeStudyThemes(themesRaw)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: decode Study themes: %w", err)
	}
	if themes.Revision != repositoryRevision {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: Study revision does not match repository")
	}
	readingRoots, err := mechanismstudy.BindStudyReadingRoots(themes, index)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf(
			"study investigation: bind exact Study reading roots: %w", err,
		)
	}
	targetRoots, err := analysistarget.BindExactRoots(target, index)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf(
			"study investigation: bind exact analysis target roots: %w", err,
		)
	}
	compilation, err := mechanismstudy.CompileTargeted(mechanismstudy.TargetCompileInput{
		Study: themes,
		Index: index,
		Binding: mechanismstudy.Binding{
			StudyThemesSHA256:         modelresearch.SHA256(themesRaw),
			AtlasStudyCatalogSHA256:   themes.ScoutSHA256,
			RepositoryRevision:        repositoryRevision,
			RepositoryFreshnessSHA256: repositoryFreshnessSHA256,
		},
		ReadingRoots:   readingRoots,
		AnalysisTarget: target,
		TargetRoots:    targetRoots,
	})
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: compile exact Study context: %w", err)
	}
	plan, err := mechanismstudy.PlanRequestBatches(compilation)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: plan requests: %w", err)
	}
	factsRaw, err := mechanismstudy.EncodeFacts(compilation, plan)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: encode facts: %w", err)
	}

	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: open artifact writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Study investigation semantic exchange journal unavailable",
	})
	if err := writer.WriteValidatedFile(
		mechanismstudy.FactsArtifactFilename,
		factsRaw,
		func(saved []byte) error {
			_, decodeErr := mechanismstudy.DecodeFacts(saved)
			return decodeErr
		},
	); err != nil {
		return studyInvestigationRunOutcome{}, fmt.Errorf("study investigation: persist facts: %w", err)
	}

	output.Stage(
		"Study investigation",
		fmt.Sprintf("prepared cards: %d", len(compilation.Cards)),
		fmt.Sprintf("planned provider batches: %d", len(plan.Batches)),
	)
	outcome := studyInvestigationRunOutcome{}
	accepted := make([]mechanismstudy.BatchCandidate, 0, len(plan.Batches))
	executions := make([]mechanismstudy.BatchExecution, 0, len(plan.Batches))
	var client studyInvestigationClient
	for position, batch := range plan.Batches {
		execution := mechanismstudy.BatchExecution{
			RequestRef: batch.Request.RequestRef, RequestSHA256: batch.WireSHA256,
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			execution.State = mechanismstudy.BatchCanceled
			executions = append(executions, execution)
			break
		}
		prompt, buildErr := mechanismstudy.BuildPrompt(batch)
		if buildErr != nil {
			return outcome, fmt.Errorf("study investigation: build prompt: %w", buildErr)
		}
		if client == nil {
			client, err = clients()
			if err != nil {
				execution.State = mechanismstudy.BatchConfigurationFailed
				executions = append(executions, execution)
				break
			}
		}
		envelope, requestErr := client.MechanismStudyPromptJSON(prompt)
		if requestErr != nil {
			execution.State = mechanismstudy.BatchConfigurationFailed
			executions = append(executions, execution)
			break
		}
		if err := scanStudyInvestigationBytes("provider request", envelope); err != nil {
			return outcome, err
		}

		started := time.Now()
		outcome.SemanticCalls++
		outcome.RequestBytes += len(envelope)
		providerResult, callErr := client.MechanismStudyBodyMeasured(ctx, envelope)
		outcome.LatencyMillis += time.Since(started).Milliseconds()
		outcome.TransportAttempts += providerResult.Attempts
		outcome.ResponseBytes += providerResultResponseBytes(providerResult)
		execution.ProviderCalls = 1
		execution.TransportAttempts = providerResult.Attempts
		if callErr != nil {
			response := providerFailureContentForExchange(callErr, providerResult.Content)
			if len(response) > 0 {
				if err := scanStudyInvestigationBytes("provider response", response); err != nil {
					recordStudyInvestigationSecretExchange(
						writer, position+1, envelope, response, providerResult.Attempts,
					)
					return outcome, err
				}
			}
			execution.State = studyInvestigationBatchFailure(callErr)
			recordStudyInvestigationExchange(
				writer, position+1, envelope, response, providerResultResponseBytes(providerResult),
				providerResult.Attempts, execution.State,
			)
			executions = append(executions, execution)
			break
		}
		if err := scanStudyInvestigationBytes("provider response", providerResult.Content); err != nil {
			recordStudyInvestigationSecretExchange(
				writer, position+1, envelope, providerResult.Content, providerResult.Attempts,
			)
			return outcome, err
		}
		candidate, validationErr := mechanismstudy.ParseBatchCandidate(
			compilation,
			batch,
			providerResult.Content,
		)
		if validationErr != nil {
			execution.State = mechanismstudy.BatchResponseInvalid
			recordStudyInvestigationExchange(
				writer, position+1, envelope, providerResult.Content,
				providerResultResponseBytes(providerResult), providerResult.Attempts,
				execution.State,
			)
			executions = append(executions, execution)
			break
		}
		execution.State = mechanismstudy.BatchAccepted
		accepted = append(accepted, candidate)
		executions = append(executions, execution)
		recordStudyInvestigationExchange(
			writer, position+1, envelope, providerResult.Content,
			providerResultResponseBytes(providerResult), providerResult.Attempts,
			execution.State,
		)
	}

	candidatesRaw, err := mechanismstudy.EncodeCandidates(factsRaw, accepted)
	if err != nil {
		return outcome, fmt.Errorf("study investigation: encode candidates: %w", err)
	}
	if err := writer.WriteValidatedFile(
		mechanismstudy.CandidatesArtifactFilename,
		candidatesRaw,
		func(saved []byte) error {
			_, decodeErr := mechanismstudy.DecodeCandidates(factsRaw, saved)
			return decodeErr
		},
	); err != nil {
		return outcome, fmt.Errorf("study investigation: persist candidates: %w", err)
	}
	resultRaw, err := mechanismstudy.EncodeResult(factsRaw, candidatesRaw)
	if err != nil {
		return outcome, fmt.Errorf("study investigation: encode result: %w", err)
	}
	if err := writer.WriteValidatedFile(
		mechanismstudy.ResultArtifactFilename,
		resultRaw,
		func(saved []byte) error {
			_, decodeErr := mechanismstudy.DecodeResult(factsRaw, candidatesRaw, saved)
			return decodeErr
		},
	); err != nil {
		return outcome, fmt.Errorf("study investigation: persist result: %w", err)
	}
	statusRaw, err := mechanismstudy.EncodeStatus(
		factsRaw,
		candidatesRaw,
		resultRaw,
		mechanismstudy.StatusExecution{Batches: executions},
	)
	if err != nil {
		return outcome, fmt.Errorf("study investigation: encode status: %w", err)
	}
	if err := writer.WriteValidatedFile(
		mechanismstudy.StatusArtifactFilename,
		statusRaw,
		func(saved []byte) error {
			_, decodeErr := mechanismstudy.DecodeStatus(factsRaw, candidatesRaw, resultRaw, saved)
			return decodeErr
		},
	); err != nil {
		return outcome, fmt.Errorf("study investigation: persist status: %w", err)
	}

	restoredFacts, err := mechanismstudy.DecodeFacts(factsRaw)
	if err != nil {
		return outcome, err
	}
	restoredResult, err := mechanismstudy.DecodeResult(factsRaw, candidatesRaw, resultRaw)
	if err != nil {
		return outcome, err
	}
	outcome.Status, err = mechanismstudy.DecodeStatus(
		factsRaw,
		candidatesRaw,
		resultRaw,
		statusRaw,
	)
	if err != nil {
		return outcome, err
	}
	publication, err := mechanismstudy.PublicationCards(
		restoredFacts.Compilation,
		restoredResult.Cards,
	)
	if err != nil {
		return outcome, fmt.Errorf("study investigation: restore report projection: %w", err)
	}
	outcome.ReportInput, err = report.StudyInvestigationInputFromPublicationCards(publication)
	if err != nil {
		return outcome, err
	}
	output.State(
		"Study investigation",
		string(outcome.Status.State),
		fmt.Sprintf("provider calls: %d", outcome.Status.ProviderCallCount),
		fmt.Sprintf("mechanism cards: %d", outcome.Status.MechanismCardCount),
		fmt.Sprintf("prepared cards: %d", outcome.Status.PreparedCardCount),
	)
	return outcome, nil
}

func recordStudyInvestigationSecretExchange(
	writer *debugdump.Writer,
	ordinal int,
	request []byte,
	response []byte,
	transportAttempts int,
) {
	if writer == nil {
		return
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageMechanismStudy,
		InstanceOrdinal: ordinal, SemanticAttemptOrdinal: 1,
		RequestProvenance: debugdump.SemanticRequestExactSent,
		State:             debugdump.SemanticStateRejected,
		ValidationCode:    debugdump.SemanticValidationSecret,
		SemanticCalls:     1, TransportAttempts: transportAttempts,
		Request: request, Response: response,
	})
}

func resetStudyInvestigationArtifacts(runDir string) error {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("study investigation: open run directory for reset: %w", err)
	}
	defer root.Close()
	for _, name := range mechanismstudy.ArtifactFilenames {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("study investigation: reset artifact %s: %w", name, err)
		}
	}
	return nil
}

func readStudyInvestigationArtifact(runDir, name string, limit int) ([]byte, error) {
	path := filepath.Join(runDir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("study investigation: inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > int64(limit) {
		return nil, fmt.Errorf("study investigation: %s is not a bounded regular file", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("study investigation: read %s: %w", name, err)
	}
	return data, nil
}

func scanStudyInvestigationBytes(label string, data []byte) error {
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf(
			"study investigation: %s failed credential scan (%s)",
			label,
			secretscan.ClosedKind(kind),
		)
	}
	return nil
}

func studyInvestigationBatchFailure(err error) mechanismstudy.BatchExecutionState {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mechanismstudy.BatchCanceled
	}
	var limit *modelresearch.ResourceLimitError
	if errors.As(err, &limit) && (limit.Kind == modelresearch.ResourceLimitOutputTokens ||
		limit.Kind == modelresearch.ResourceLimitResponseBytes) {
		return mechanismstudy.BatchOutputLimit
	}
	return mechanismstudy.BatchProviderFailed
}

// studyInvestigationPublicationContext gives an already-started optional
// mechanism call enough bounded time to publish its durable canceled status
// and the otherwise valid report. Other stages keep the caller context.
func studyInvestigationPublicationContext(
	ctx context.Context,
	status mechanismstudy.Status,
) (context.Context, context.CancelFunc) {
	if studyInvestigationCanceled(status) {
		return context.WithTimeout(context.WithoutCancel(ctx), studyInvestigationPublicationTimeout)
	}
	return ctx, func() {}
}

func studyInvestigationCanceled(status mechanismstudy.Status) bool {
	for _, batch := range status.Batches {
		if batch.State == mechanismstudy.BatchCanceled {
			return true
		}
	}
	return false
}

// validateStudyInvestigationRepositoryFreshness keeps the exact SSA/index and
// Study evidence bound to one repository state. This stage is intentionally
// stricter than the ordinary report's optional --strict-snapshot policy:
// accepting any repository drift would publish mechanism paths from stale
// local authority.
func validateStudyInvestigationRepositoryFreshness(
	initial freshness.RepositoryState,
	current freshness.RepositoryState,
) error {
	initialDigest, err := initial.Digest()
	if err != nil {
		return fmt.Errorf("bind Study investigation initial repository state: %w", err)
	}
	currentDigest, err := current.Digest()
	if err != nil {
		return fmt.Errorf("bind Study investigation current repository state: %w", err)
	}
	if initialDigest != currentDigest {
		return fmt.Errorf("study investigation: repository changed while exact mechanism authority was active")
	}
	return nil
}

func recordStudyInvestigationExchange(
	writer *debugdump.Writer,
	ordinal int,
	request []byte,
	response []byte,
	responseBytes int,
	transportAttempts int,
	state mechanismstudy.BatchExecutionState,
) {
	if writer == nil {
		return
	}
	exchange := debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageMechanismStudy,
		InstanceOrdinal: ordinal, SemanticAttemptOrdinal: 1,
		RequestProvenance: debugdump.SemanticRequestExactSent,
		SemanticCalls:     1, TransportAttempts: transportAttempts,
		Request: request, Response: response,
	}
	switch state {
	case mechanismstudy.BatchAccepted:
		exchange.State = debugdump.SemanticStateAccepted
		exchange.ValidationCode = debugdump.SemanticValidationAccepted
	case mechanismstudy.BatchResponseInvalid:
		exchange.State = debugdump.SemanticStateRejected
		exchange.ValidationCode = debugdump.SemanticValidationResponse
	case mechanismstudy.BatchCanceled:
		exchange.State = debugdump.SemanticStateCanceled
		exchange.ValidationCode = debugdump.SemanticValidationCanceled
	default:
		exchange.State = debugdump.SemanticStateProviderFailed
		exchange.ValidationCode = debugdump.SemanticValidationProvider
	}
	if len(response) == 0 {
		code := debugdump.SemanticUnavailableNoContent
		if state == mechanismstudy.BatchCanceled {
			code = debugdump.SemanticUnavailableCanceled
		}
		exchange.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: code, OriginalBytes: responseBytes,
		}
	}
	writer.RecordSemanticExchange(exchange)
}
