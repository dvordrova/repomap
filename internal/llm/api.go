// Package llm owns provider-neutral preparation, execution, validation, and
// accepted-response caching for model-assisted repository analysis.
package llm

import (
	"context"
	"errors"
	"time"
)

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
// must not contain credentials.
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

// ProviderError preserves a typed cause for errors.Is/errors.As while keeping
// Error content closed. Provider error bodies and authentication details are
// never interpolated into the user-facing string.
type ProviderError struct {
	Operation string
	cause     error
}

func (providerErr *ProviderError) Error() string {
	if providerErr == nil || providerErr.Operation == "" {
		return "llm: provider operation failed"
	}
	return "llm: provider " + providerErr.Operation + " failed"
}

func (providerErr *ProviderError) Unwrap() error {
	if providerErr == nil {
		return nil
	}
	return providerErr.cause
}

// Executor configures the one persistent accepted-response cache and optional
// semantic event observer. RootDir contains the fixed .llm-cache directory.
// Enabled=false bypasses all cache identity, reads, and writes.
type Executor struct {
	RootDir  string
	Enabled  bool
	Observer Observer
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
