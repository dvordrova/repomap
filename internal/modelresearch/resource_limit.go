package modelresearch

import "fmt"

const (
	// ProviderResponseByteLimit is the one transport-owned response body
	// ceiling shared by provider adapters and semantic response decoders.
	// Individual stages must not impose a smaller response envelope.
	ProviderResponseByteLimit = 16 * 1024 * 1024

	// SemanticRecordByteLimit is the common safety ceiling for persisted
	// semantic records and artifacts. Schema and identity validation remain
	// independent of this resource boundary.
	SemanticRecordByteLimit = 32 * 1024 * 1024
)

// ResourceLimitKind identifies the configured resource boundary that stopped
// one semantic request. Values are closed and safe to include in diagnostics.
type ResourceLimitKind string

const (
	ResourceLimitRequestBytes  ResourceLimitKind = "request_bytes"
	ResourceLimitResponseBytes ResourceLimitKind = "response_bytes"
	ResourceLimitRecordBytes   ResourceLimitKind = "record_bytes"
	ResourceLimitOutputTokens  ResourceLimitKind = "output_tokens"
)

// ResourceLimitError is a terminal, non-retryable semantic resource outcome.
// Its Error text contains only bounded transport metadata. Provider prose is
// retained privately so callers can pass it only through the existing safe
// exchange recorder.
type ResourceLimitError struct {
	Stage               string
	Kind                ResourceLimitKind
	Limit               int
	Observed            int
	ObservedKnown       bool
	ObservedAtLeast     bool
	ConfiguredMaxTokens int
	InputTokens         int
	OutputTokens        int
	ReasoningTokens     int
	FinishReason        string
	HTTPStatus          int
	providerContent     []byte
}

func (err *ResourceLimitError) Error() string {
	if err == nil {
		return "llm resource limit reached"
	}
	stage := err.Stage
	if stage == "" {
		stage = "semantic_request"
	}
	message := fmt.Sprintf("llm resource limit reached: stage=%s limit=%s", stage, err.Kind)
	if err.Limit > 0 {
		message += fmt.Sprintf(" configured=%d", err.Limit)
	}
	if err.ObservedKnown {
		if err.ObservedAtLeast {
			message += fmt.Sprintf(" observed>=%d", err.Observed)
		} else {
			message += fmt.Sprintf(" observed=%d", err.Observed)
		}
	}
	if err.FinishReason != "" {
		message += " finish_reason=" + err.FinishReason
	}
	if err.HTTPStatus > 0 {
		message += fmt.Sprintf(" status=%d", err.HTTPStatus)
	}
	if err.InputTokens > 0 {
		message += fmt.Sprintf(" input_tokens=%d", err.InputTokens)
	}
	if err.OutputTokens > 0 {
		message += fmt.Sprintf(" output_tokens=%d", err.OutputTokens)
	}
	if err.ReasoningTokens > 0 {
		message += fmt.Sprintf(" reasoning_tokens=%d", err.ReasoningTokens)
	}
	return message
}

// NewResourceLimitError defensively retains provider content for the existing
// redacted exchange recorder without exposing a mutable field on the error.
func NewResourceLimitError(details ResourceLimitError, providerContent []byte) *ResourceLimitError {
	details.providerContent = append([]byte(nil), providerContent...)
	return &details
}

// Clone returns an independent copy, including privately retained diagnostic
// content. It is used when a provider adapter adds stage configuration.
func (err *ResourceLimitError) Clone() *ResourceLimitError {
	if err == nil {
		return nil
	}
	return NewResourceLimitError(*err, err.providerContent)
}

// ProviderContent returns a defensive copy of parsed provider content. It is
// diagnostic input only and must pass through the existing exchange recorder
// before persistence.
func (err *ResourceLimitError) ProviderContent() []byte {
	if err == nil {
		return nil
	}
	return append([]byte(nil), err.providerContent...)
}
