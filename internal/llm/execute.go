package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	executorContract      = "repomap.llm.execute-json.v1"
	maxProviderStateBytes = 64 * 1024
	maxCubeStateBytes     = 64 * 1024
	hardMaxResponseBytes  = 16 * 1024 * 1024
)

// ExecuteJSON prepares one exact request, attempts a fully revalidated cache
// hit, and otherwise makes exactly one live Provider call. Only a successfully
// decoded and validated live response is eligible for persistence.
func ExecuteJSON[T any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	call Call[T],
) (Outcome[T], error) {
	var outcome Outcome[T]
	decodeValidate, err := decoderForCall(call)
	if err != nil {
		return outcome, err
	}
	if err := validateLimits(call.Limits); err != nil {
		return outcome, err
	}
	if provider == nil {
		return outcome, errors.New("llm: provider is nil")
	}
	if err := ctx.Err(); err != nil {
		return outcome, err
	}

	prepared, err := provider.Prepare(call.Prompt, call.Limits)
	if err != nil {
		outcome.Issues = observe(executor.Observer, Event{
			Kind: EventFailure, Source: SourceLive, Failure: FailurePrepare,
		}, outcome.Issues)
		return outcome, &ProviderError{Operation: "prepare", cause: err}
	}
	request := prepared.Bytes()
	requestSensitivity := setOutcomeRequest(&outcome, request)
	if len(request) == 0 {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, errors.New("llm: provider returned an empty prepared request")
	}
	if len(request) > call.Limits.MaxRequestBytes {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, fmt.Errorf(
			"llm: prepared request is %d bytes, limit is %d",
			len(request), call.Limits.MaxRequestBytes,
		)
	}
	if requestSensitivity.structured {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, ErrSensitivePreparedRequest
	}

	if !executor.Enabled {
		return executeLive(ctx, executor, provider, prepared, decodeValidate, call.Limits, outcome)
	}
	providerState, err := canonicalProviderState(provider.State())
	if err != nil {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, err
	}
	if len(call.State) == 0 {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, errors.New("llm: cube state is empty while cache is enabled")
	}
	if len(call.State) > maxCubeStateBytes {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailurePrepare, outcome.Issues)
		return outcome, fmt.Errorf("llm: cube state exceeds %d bytes", maxCubeStateBytes)
	}
	cacheKey := executionCacheKey(providerState, call.State, request)
	outcome.CacheKey = cacheKey

	record, found, loadErr := loadAcceptedCache(executor.RootDir, cacheKey, request, call.Limits)
	if loadErr != nil {
		outcome.Issues = append(outcome.Issues, Issue{Kind: IssueCacheRead, Err: loadErr})
		outcome.Issues = observe(executor.Observer, eventForOutcome(
			EventFailure, SourceCache, FailureCache, outcome,
		), outcome.Issues)
		if evictErr := removeAcceptedCache(executor.RootDir, cacheKey); evictErr != nil {
			outcome.Issues = append(outcome.Issues, Issue{Kind: IssueCacheEvict, Err: evictErr})
		}
	} else if found {
		value, validateErr := decodeAcceptedJSON(decodeValidate, record.Response)
		if validateErr == nil {
			outcome.Value = value
			outcome.Cached = true
			_ = setOutcomeResponse(&outcome, record.Response)
			outcome.FinishReason = record.FinishReason
			outcome.ChoiceCount = record.ChoiceCount
			outcome.Metrics = record.Metrics
			outcome.Issues = observe(executor.Observer, eventForOutcome(
				EventCacheHit, SourceCache, FailureNone, outcome,
			), outcome.Issues)
			return outcome, nil
		}
		outcome.Issues = append(outcome.Issues, Issue{Kind: IssueCacheValidate, Err: validateErr})
		_ = setOutcomeResponse(&outcome, record.Response)
		outcome.FinishReason = record.FinishReason
		outcome.ChoiceCount = record.ChoiceCount
		outcome.Metrics = record.Metrics
		outcome.Cached = true
		outcome.Issues = observe(executor.Observer, eventForOutcome(
			EventFailure, SourceCache, FailureValidation, outcome,
		), outcome.Issues)
		outcome.Cached = false
		if evictErr := removeAcceptedCache(executor.RootDir, cacheKey); evictErr != nil {
			outcome.Issues = append(outcome.Issues, Issue{Kind: IssueCacheEvict, Err: evictErr})
		}
	}

	return executeLive(ctx, executor, provider, prepared, decodeValidate, call.Limits, outcome)
}

// ExecuteJSONBatch runs calls sequentially in caller-provided order. On the
// first failure it returns the ordered prefix including that failed outcome;
// later calls are not prepared or executed.
func ExecuteJSONBatch[T any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	calls []Call[T],
) ([]Outcome[T], error) {
	outcomes := make([]Outcome[T], 0, len(calls))
	for index, call := range calls {
		outcome, err := ExecuteJSON(ctx, executor, provider, call)
		outcomes = append(outcomes, outcome)
		if err != nil {
			return outcomes, fmt.Errorf("llm: batch item %d: %w", index, err)
		}
	}
	return outcomes, nil
}

func executeLive[T any](
	ctx context.Context,
	executor Executor,
	provider Provider,
	prepared Prepared,
	decodeValidate DecodeValidate[T],
	limits Limits,
	outcome Outcome[T],
) (Outcome[T], error) {
	completion, err := provider.Complete(ctx, prepared)
	responseSensitivity := setOutcomeResponse(&outcome, completion.Response)
	outcome.FinishReason = completion.FinishReason
	outcome.ChoiceCount = completion.ChoiceCount
	outcome.Metrics = completion.Metrics
	if err != nil {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailureProvider, outcome.Issues)
		return outcome, &ProviderError{Operation: "complete", cause: err}
	}
	if responseSensitivity.found {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailureResponse, outcome.Issues)
		return outcome, ErrSensitiveResponse
	}
	if err := validateLiveCompletion(completion, limits); err != nil {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailureResponse, outcome.Issues)
		return outcome, err
	}
	value, err := decodeAcceptedJSON(decodeValidate, completion.Response)
	if err != nil {
		outcome.Issues = observeFailure(executor.Observer, outcome, FailureValidation, outcome.Issues)
		return outcome, fmt.Errorf("llm: reject response: %w", err)
	}
	outcome.Value = value

	if executor.Enabled {
		exactRequest := prepared.Bytes()
		record := acceptedCacheRecord{
			Version:        cacheRecordVersion,
			Contract:       executorContract,
			CacheKey:       outcome.CacheKey,
			Accepted:       true,
			RequestSHA256:  sha256Hex(exactRequest),
			ResponseSHA256: sha256Hex(completion.Response),
			RequestBytes:   len(exactRequest),
			ResponseBytes:  len(completion.Response),
			Response:       cloneBytes(completion.Response),
			FinishReason:   outcome.FinishReason,
			ChoiceCount:    outcome.ChoiceCount,
			Metrics:        outcome.Metrics,
		}
		if err := saveAcceptedCache(executor.RootDir, record); err != nil {
			outcome.Issues = append(outcome.Issues, Issue{Kind: IssueCacheWrite, Err: err})
		}
	}
	outcome.Issues = observe(executor.Observer, eventForOutcome(
		EventLive, SourceLive, FailureNone, outcome,
	), outcome.Issues)
	return outcome, nil
}

func decoderForCall[T any](call Call[T]) (DecodeValidate[T], error) {
	if call.DecodeValidate != nil && call.Validate != nil {
		return nil, errors.New("llm: set either DecodeValidate or Validate, not both")
	}
	if call.DecodeValidate != nil {
		return call.DecodeValidate, nil
	}
	return DecodeJSON(call.Validate), nil
}

func validateLimits(limits Limits) error {
	if limits.MaxRequestBytes <= 0 {
		return errors.New("llm: MaxRequestBytes must be positive")
	}
	if limits.MaxResponseBytes <= 0 || limits.MaxResponseBytes > hardMaxResponseBytes {
		return fmt.Errorf(
			"llm: MaxResponseBytes must be between 1 and %d",
			hardMaxResponseBytes,
		)
	}
	if limits.MaxOutputTokens <= 0 {
		return errors.New("llm: MaxOutputTokens must be positive")
	}
	return nil
}

func validateLiveCompletion(completion Completion, limits Limits) error {
	if completion.ChoiceCount != 1 {
		return fmt.Errorf("llm: completion has %d choices, want exactly one", completion.ChoiceCount)
	}
	if completion.FinishReason != FinishStop {
		return fmt.Errorf("llm: completion did not stop normally (%s)", closedFinishReason(completion.FinishReason))
	}
	if len(completion.Response) > limits.MaxResponseBytes {
		return fmt.Errorf(
			"llm: response is %d bytes, limit is %d",
			len(completion.Response), limits.MaxResponseBytes,
		)
	}
	if err := validateMetrics(completion.Metrics); err != nil {
		return err
	}
	return nil
}

func validateMetrics(metrics Metrics) error {
	if metrics.InputTokens < 0 || metrics.OutputTokens < 0 ||
		metrics.ReasoningTokens < 0 || metrics.ProviderResponseBytes < 0 ||
		metrics.PromptCacheHitTokens < 0 || metrics.PromptCacheMissTokens < 0 ||
		metrics.Latency < 0 || metrics.Attempts < 1 {
		return errors.New("llm: completion metrics are invalid or report no transport attempt")
	}
	return nil
}

func closedFinishReason(reason FinishReason) FinishReason {
	switch reason {
	case FinishStop, FinishLength, FinishContentFilter, FinishToolCalls,
		FinishInsufficientSystemResource:
		return reason
	default:
		return FinishUnknown
	}
}

func decodeAcceptedJSON[T any](decodeValidate DecodeValidate[T], raw []byte) (T, error) {
	var zero T
	normalized, err := NormalizeJSON(raw)
	if err != nil {
		return zero, err
	}
	value, err := decodeValidate(cloneBytes(normalized))
	if err != nil {
		return zero, err
	}
	return value, nil
}

func canonicalProviderState(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("llm: provider state is empty")
	}
	if len(raw) > maxProviderStateBytes {
		return nil, fmt.Errorf("llm: provider state exceeds %d bytes", maxProviderStateBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var state map[string]any
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("llm: provider state must be a JSON object: %w", err)
	}
	if state == nil {
		return nil, errors.New("llm: provider state must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("llm: provider state contains multiple JSON values")
		}
		return nil, fmt.Errorf("llm: decode provider state: %w", err)
	}
	if assessment := assessSensitiveMaterial(raw); assessment.found {
		return nil, errors.New("llm: provider state contains explicit credential material")
	}
	canonical, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("llm: canonicalize provider state: %w", err)
	}
	return canonical, nil
}

func executionCacheKey(providerState, cubeState, request []byte) string {
	hash := sha256.New()
	writeHashFrame(hash, []byte(executorContract))
	writeHashFrame(hash, providerState)
	writeHashFrame(hash, cubeState)
	writeHashFrame(hash, request)
	return hex.EncodeToString(hash.Sum(nil))
}

type hashWriter interface {
	Write([]byte) (int, error)
}

func writeHashFrame(writer hashWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(value)
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func observeFailure[T any](
	observer Observer,
	outcome Outcome[T],
	failure FailureKind,
	issues []Issue,
) []Issue {
	return observe(observer, eventForOutcome(EventFailure, SourceLive, failure, outcome), issues)
}

func eventForOutcome[T any](
	kind EventKind,
	source EventSource,
	failure FailureKind,
	outcome Outcome[T],
) Event {
	return Event{
		Kind: kind, Source: source, Failure: failure,
		CacheKey: outcome.CacheKey,
		Request:  cloneBytes(outcome.Request), RequestSHA256: outcome.RequestSHA256,
		RequestBytes: outcome.RequestBytes, RequestRedacted: outcome.RequestRedacted,
		Response: cloneBytes(outcome.Response), ResponseSHA256: outcome.ResponseSHA256,
		ResponseBytes: outcome.ResponseBytes, ResponseRedacted: outcome.ResponseRedacted,
		FinishReason: outcome.FinishReason,
		ChoiceCount:  outcome.ChoiceCount, Metrics: outcome.Metrics, Cached: outcome.Cached,
	}
}

func setOutcomeRequest[T any](outcome *Outcome[T], request []byte) sensitiveAssessment {
	assessment := assessSensitiveMaterial(request)
	outcome.RequestSHA256 = sha256Hex(request)
	outcome.RequestBytes = len(request)
	outcome.RequestRedacted = assessment.found
	if assessment.found {
		outcome.Request = nil
	} else {
		outcome.Request = cloneBytes(request)
	}
	return assessment
}

func setOutcomeResponse[T any](outcome *Outcome[T], response []byte) sensitiveAssessment {
	assessment := assessSensitiveMaterial(response)
	outcome.ResponseSHA256 = sha256Hex(response)
	outcome.ResponseBytes = len(response)
	outcome.ResponseRedacted = assessment.found
	if assessment.found {
		outcome.Response = nil
	} else {
		outcome.Response = cloneBytes(response)
	}
	return assessment
}

func observe(observer Observer, event Event, issues []Issue) []Issue {
	if observer == nil {
		return issues
	}
	event.Request = cloneBytes(event.Request)
	event.Response = cloneBytes(event.Response)
	if err := observer.Observe(event); err != nil {
		return append(issues, Issue{Kind: IssueObserver, Err: err})
	}
	return issues
}
