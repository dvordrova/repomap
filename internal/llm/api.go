// Package llm owns provider-neutral preparation, execution, validation, and
// accepted-response caching for model-assisted repository analysis.
package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultBatchConcurrency is the conservative product concurrency for
// independent model calls. Executor's zero value remains sequential so tests,
// fixtures, and callers that have not opted in retain their original behavior.
const DefaultBatchConcurrency = 4

// Prompt is provider-neutral model input. A Provider is responsible for
// turning it into its exact transport request.
type Prompt struct {
	System             string
	User               string
	ResponseFormatJSON bool
}

// Limits are part of request preparation and response acceptance. Providers
// should encode MaxOutputTokens into the exact request where their protocol
// supports it.
type Limits struct {
	MaxRequestBytes  int
	MaxResponseBytes int
	MaxOutputTokens  int
}

// Prepared holds one exact immutable provider request. Bytes returns a copy so
// neither an adapter nor an observer can mutate the cache identity.
type Prepared struct {
	exact []byte
}

func NewPrepared(exact []byte) (Prepared, error) {
	if len(exact) == 0 {
		return Prepared{}, errors.New("llm: prepared request is empty")
	}
	return Prepared{exact: cloneBytes(exact)}, nil
}

func (prepared Prepared) Bytes() []byte {
	return cloneBytes(prepared.exact)
}

func (prepared Prepared) Len() int {
	return len(prepared.exact)
}

// Metrics are transport measurements for one live completion. Cache hits
// preserve the measurements of the accepted call that populated the entry.
// ProviderResponseBytes is cumulative across all transport Attempts.
type Metrics struct {
	InputTokens           int           `json:"input_tokens"`
	OutputTokens          int           `json:"output_tokens"`
	ReasoningTokens       int           `json:"reasoning_tokens,omitempty"`
	PromptCacheHitTokens  int           `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int           `json:"prompt_cache_miss_tokens,omitempty"`
	ProviderResponseBytes int           `json:"provider_response_bytes,omitempty"`
	UsageReported         bool          `json:"usage_reported,omitempty"`
	Latency               time.Duration `json:"latency"`
	Attempts              int           `json:"attempts"`
}

type FinishReason string

const (
	FinishStop                       FinishReason = "stop"
	FinishLength                     FinishReason = "length"
	FinishContentFilter              FinishReason = "content_filter"
	FinishToolCalls                  FinishReason = "tool_calls"
	FinishInsufficientSystemResource FinishReason = "insufficient_system_resource"
	FinishUnknown                    FinishReason = "unknown"
)

// Completion contains exact assistant content after the Provider has decoded
// its own transport envelope. ExecuteJSON separately decodes and validates the
// domain JSON contained in Response.
type Completion struct {
	Response     []byte
	FinishReason FinishReason
	ChoiceCount  int
	Metrics      Metrics
}

// Provider separates stable non-secret configuration, deterministic request
// preparation, and the live network effect. State must be a JSON object and
// must not contain credentials. When Executor.BatchConcurrency is larger than
// one, all three methods may be called concurrently on the same Provider and
// the implementation must be concurrency-safe.
type Provider interface {
	State() []byte
	Prepare(Prompt, Limits) (Prepared, error)
	Complete(context.Context, Prepared) (Completion, error)
}

// DecodeValidate supports domains whose accepted response is not a direct Go
// JSON shape, for example reducers over opaque references.
type DecodeValidate[T any] func([]byte) (T, error)

// DecodeJSON builds the default JSON decoder used by simple cubes. Validation
// runs only after decoding succeeds; a nil validator accepts any decoded T.
func DecodeJSON[T any](validate func(T) error) DecodeValidate[T] {
	return func(raw []byte) (T, error) { return decodeJSONValue(raw, validate) }
}

// Call is one cube-owned model operation. State should contain preparation,
// prompt, and semantic-contract versions. It is hashed but never persisted.
//
// Set DecodeValidate for a custom reducer, or set Validate to use the default
// JSON decoder. Setting both is an error.
type Call[T any] struct {
	State          []byte
	Prompt         Prompt
	Limits         Limits
	DecodeValidate DecodeValidate[T]
	Validate       func(T) error
}

// BatchItemError identifies the exact caller-order item that failed in one
// batch execution. Domain planners may use the index to deterministically
// re-shard only response/output-resource failures; every other item error
// remains terminal to the complete domain result.
type BatchItemError struct {
	Index int
	Err   error
}

func (err *BatchItemError) Error() string {
	if err == nil {
		return "llm: batch item failed"
	}
	return fmt.Sprintf("llm: batch item %d: %v", err.Index, err.Err)
}

func (err *BatchItemError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

type EventKind string

const (
	EventCacheHit EventKind = "cache_hit"
	EventLive     EventKind = "live"
	EventFailure  EventKind = "failure"
)

type EventSource string

const (
	SourceCache EventSource = "cache"
	SourceLive  EventSource = "live"
)

// FailureKind is deliberately closed and content-free so event sinks never
// need a provider error that might contain headers or credentials.
type FailureKind string

const (
	FailureNone       FailureKind = ""
	FailurePrepare    FailureKind = "prepare"
	FailureProvider   FailureKind = "provider"
	FailureResponse   FailureKind = "response"
	FailureValidation FailureKind = "validation"
	FailureCache      FailureKind = "cache"
)

// Event contains only exact semantic request/response bytes, measurements,
// and SHA-256 cache identity. Provider and cube state bytes are excluded.
type Event struct {
	Kind             EventKind
	Source           EventSource
	Failure          FailureKind
	CacheKey         string
	Request          []byte
	RequestSHA256    string
	RequestBytes     int
	RequestRedacted  bool
	Response         []byte
	ResponseSHA256   string
	ResponseBytes    int
	ResponseRedacted bool
	FinishReason     FinishReason
	ChoiceCount      int
	Metrics          Metrics
	Cached           bool
}

type Observer interface {
	Observe(Event) error
}

type ObserverFunc func(Event) error

func (observe ObserverFunc) Observe(event Event) error {
	return observe(event)
}

type IssueKind string

const (
	IssueCacheRead     IssueKind = "cache_read"
	IssueCacheValidate IssueKind = "cache_validate"
	IssueCacheEvict    IssueKind = "cache_evict"
	IssueCacheWrite    IssueKind = "cache_write"
	IssueObserver      IssueKind = "observer"
)

// Issue is non-fatal when ExecuteJSON has an accepted output. Callers can
// report operational cache or observer failures without discarding that value.
type Issue struct {
	Kind IssueKind
	Err  error
}

func (issue Issue) Error() string {
	if issue.Err == nil {
		return string(issue.Kind)
	}
	return string(issue.Kind) + ": " + issue.Err.Error()
}

func (issue Issue) Unwrap() error {
	return issue.Err
}

// Outcome is returned for both accepted calls and failures. A nil error from
// ExecuteJSON means Value has passed the cube's decoder and validation.
type Outcome[T any] struct {
	Value            T
	CacheKey         string
	Cached           bool
	Request          []byte
	RequestSHA256    string
	RequestBytes     int
	RequestRedacted  bool
	Response         []byte
	ResponseSHA256   string
	ResponseBytes    int
	ResponseRedacted bool
	FinishReason     FinishReason
	ChoiceCount      int
	Metrics          Metrics
	Issues           []Issue
}

// ProviderFailureKind is a closed, provider-neutral failure classification.
// It deliberately excludes arbitrary provider text.
type ProviderFailureKind string

const (
	ProviderFailureUnknown    ProviderFailureKind = "unknown"
	ProviderFailureHTTPStatus ProviderFailureKind = "http_status"
	ProviderFailureTimeout    ProviderFailureKind = "timeout"
	ProviderFailureNetwork    ProviderFailureKind = "network"
	ProviderFailureResource   ProviderFailureKind = "resource_limit"
	ProviderFailureResponse   ProviderFailureKind = "response"
	ProviderFailureCanceled   ProviderFailureKind = "canceled"

	httpStatusCodeFirst      = 100
	httpStatusCodeLast       = 599
	httpServerErrorCodeFirst = 500
)

// ProviderFailure contains only closed classifications and scalar transport
// facts that are safe to render. Provider error text and response bodies never
// enter this value.
type ProviderFailure struct {
	Kind           ProviderFailureKind
	HTTPStatus     int
	Attempts       int
	RetryExhausted bool
	ResourceKind   ResourceLimitKind
}

// ProviderFailureSource lets a provider expose closed transport facts without
// granting its arbitrary error text authority in shared diagnostics.
type ProviderFailureSource interface {
	ProviderFailure() ProviderFailure
}

// ProviderError preserves a typed cause for errors.Is/errors.As while rendering
// only a normalized ProviderFailure. Provider error bodies and authentication
// details are never interpolated into the user-facing string.
type ProviderError struct {
	Operation string
	failure   ProviderFailure
	cause     error
}

func (providerErr *ProviderError) Error() string {
	if providerErr == nil || providerErr.Operation == "" {
		return "llm: provider operation failed"
	}
	failure := normalizeProviderFailure(providerErr.failure)
	details := "class=" + string(failure.Kind)
	if failure.HTTPStatus != 0 {
		details += fmt.Sprintf(" status=%d", failure.HTTPStatus)
	}
	if failure.Attempts > 0 {
		details += fmt.Sprintf(" attempts=%d", failure.Attempts)
	}
	if failure.RetryExhausted {
		details += " retries_exhausted=true"
	}
	if failure.ResourceKind != "" {
		details += " resource=" + string(failure.ResourceKind)
	}
	message := "llm: provider " + providerErr.Operation + " failed (" + details + ")"
	if guidance := providerFailureGuidance(failure); guidance != "" {
		message += "; " + guidance
	}
	return message
}

func (providerErr *ProviderError) Unwrap() error {
	if providerErr == nil {
		return nil
	}
	return providerErr.cause
}

func (providerErr *ProviderError) ProviderFailure() ProviderFailure {
	if providerErr == nil {
		return ProviderFailure{Kind: ProviderFailureUnknown}
	}
	return normalizeProviderFailure(providerErr.failure)
}

func newProviderError(operation string, cause error, attempts int) *ProviderError {
	failure := ProviderFailure{Kind: ProviderFailureUnknown}
	var source ProviderFailureSource
	if errors.As(cause, &source) {
		failure = source.ProviderFailure()
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		failure.Kind = ProviderFailureTimeout
	} else if errors.Is(cause, context.Canceled) {
		failure.Kind = ProviderFailureCanceled
	}
	var resourceErr *ResourceLimitError
	if errors.As(cause, &resourceErr) {
		failure.Kind = ProviderFailureResource
		failure.ResourceKind = resourceErr.Kind
		failure.HTTPStatus = resourceErr.HTTPStatus
	}
	if attempts > 0 {
		failure.Attempts = attempts
	}
	return &ProviderError{
		Operation: operation,
		failure:   normalizeProviderFailure(failure),
		cause:     cause,
	}
}

func normalizeProviderFailure(failure ProviderFailure) ProviderFailure {
	switch failure.Kind {
	case ProviderFailureHTTPStatus, ProviderFailureTimeout, ProviderFailureNetwork,
		ProviderFailureResource, ProviderFailureResponse, ProviderFailureCanceled:
	default:
		failure.Kind = ProviderFailureUnknown
	}
	if failure.HTTPStatus < httpStatusCodeFirst || failure.HTTPStatus > httpStatusCodeLast ||
		(failure.Kind != ProviderFailureHTTPStatus && failure.Kind != ProviderFailureResource) {
		failure.HTTPStatus = 0
	}
	if failure.Attempts < 0 {
		failure.Attempts = 0
	}
	if failure.Attempts < 2 ||
		(failure.Kind != ProviderFailureHTTPStatus && failure.Kind != ProviderFailureNetwork &&
			failure.Kind != ProviderFailureTimeout) {
		failure.RetryExhausted = false
	}
	if failure.Kind != ProviderFailureResource || !validResourceLimitKind(failure.ResourceKind) {
		failure.ResourceKind = ""
	}
	return failure
}

func validResourceLimitKind(kind ResourceLimitKind) bool {
	switch kind {
	case ResourceLimitRequestBytes, ResourceLimitResponseBytes, ResourceLimitRecordBytes,
		ResourceLimitCatalogItems, ResourceLimitOutputTokens, ResourceLimitSemanticCalls:
		return true
	default:
		return false
	}
}

func providerFailureGuidance(failure ProviderFailure) string {
	switch failure.Kind {
	case ProviderFailureHTTPStatus:
		switch failure.HTTPStatus {
		case 401, 403:
			return "check provider credentials and model access"
		case 429:
			return "check provider rate limits or quota, then retry"
		default:
			if failure.HTTPStatus >= httpServerErrorCodeFirst {
				return "retry later or check provider service health"
			}
			return "check provider endpoint, request compatibility, and account access"
		}
	case ProviderFailureTimeout:
		return "check provider latency or increase the configured timeout"
	case ProviderFailureNetwork:
		return "check network access and the configured provider endpoint"
	case ProviderFailureResource:
		switch failure.ResourceKind {
		case ResourceLimitOutputTokens:
			return "reduce the requested result, or raise the output-token limit where configurable"
		case ResourceLimitResponseBytes:
			return "reduce the requested result, or raise the response limit where configurable"
		case ResourceLimitSemanticCalls:
			return "reduce the requested work, or raise the model-call limit where configurable"
		default:
			return "reduce the model request, or raise the resource limit where configurable"
		}
	case ProviderFailureResponse:
		return "check provider response compatibility and retry"
	case ProviderFailureCanceled:
		return "rerun after clearing the cancellation"
	default:
		return "check provider configuration, credentials, and connectivity"
	}
}

// Executor configures the one persistent accepted-response cache and optional
// semantic event observer. RootDir contains the fixed .llm-cache directory.
// Enabled=false bypasses all cache identity, reads, and writes.
//
// BatchConcurrency bounds the number of independent ExecuteJSONBatch items
// that may execute at once. Values below two preserve the sequential default.
// Providers used with a larger value must permit concurrent calls. Batch
// observers are still invoked serially in caller order.
//
// BatchController should be shared by executors that use the same provider so
// the first transport HTTP 429 makes later provider attempts sequential.
type Executor struct {
	RootDir          string
	Enabled          bool
	Observer         Observer
	BatchConcurrency int
	BatchController  *BatchController
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
