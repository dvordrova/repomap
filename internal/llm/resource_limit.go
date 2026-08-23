package llm

import "fmt"

const (
	// ProviderResponseByteLimit is the transport-owned response body ceiling.
	ProviderResponseByteLimit = 16 * 1024 * 1024

	// SemanticRecordByteLimit is the common safety ceiling for provider
	// requests and persisted semantic records. Domain validation is separate.
	SemanticRecordByteLimit = 32 * 1024 * 1024
)

// ResourceLimitKind identifies the closed resource boundary that stopped one
// model request.
type ResourceLimitKind string

const (
	ResourceLimitRequestBytes  ResourceLimitKind = "request_bytes"
	ResourceLimitResponseBytes ResourceLimitKind = "response_bytes"
	ResourceLimitRecordBytes   ResourceLimitKind = "record_bytes"
	ResourceLimitCatalogItems  ResourceLimitKind = "catalog_items"
	ResourceLimitOutputTokens  ResourceLimitKind = "output_tokens"
	ResourceLimitSemanticCalls ResourceLimitKind = "semantic_calls"
)

// ResourceLimitError is a terminal, non-retryable model resource outcome.
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

func NewResourceLimitError(details ResourceLimitError) *ResourceLimitError {
	return &details
}

func (err *ResourceLimitError) Clone() *ResourceLimitError {
	if err == nil {
		return nil
	}
	return NewResourceLimitError(*err)
}
