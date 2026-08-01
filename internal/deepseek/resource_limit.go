package deepseek

import (
	"errors"
	"fmt"
)

// ResourceLimitKind identifies the configured resource boundary that stopped
// one semantic request. Values are closed and safe to include in diagnostics.
type ResourceLimitKind string

const (
	ResourceLimitRequestBytes  ResourceLimitKind = "request_bytes"
	ResourceLimitResponseBytes ResourceLimitKind = "response_bytes"
	ResourceLimitOutputTokens  ResourceLimitKind = "output_tokens"
)

// ResourceLimitError is a terminal, non-retryable semantic resource outcome.
// Its Error text contains only bounded transport metadata. Provider prose is
// retained privately, when the response envelope was parsed, so the existing
// semantic exchange recorder can apply its mandatory redaction and secret
// handling without exposing that prose through an error string.
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
	responseContent     []byte
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

// ProviderContent returns a defensive copy of parsed provider content. It is
// diagnostic input only and must pass through the existing exchange recorder
// before persistence.
func (err *ResourceLimitError) ProviderContent() []byte {
	if err == nil {
		return nil
	}
	return append([]byte(nil), err.responseContent...)
}

func annotateResourceLimit(err error, stage string, maxTokens int) error {
	var resourceErr *ResourceLimitError
	if !errors.As(err, &resourceErr) {
		return err
	}
	annotated := *resourceErr
	annotated.responseContent = append([]byte(nil), resourceErr.responseContent...)
	if annotated.Stage == "" {
		annotated.Stage = stage
	}
	if annotated.ConfiguredMaxTokens == 0 {
		annotated.ConfiguredMaxTokens = maxTokens
	}
	if annotated.Kind == ResourceLimitOutputTokens && annotated.Limit == 0 {
		annotated.Limit = maxTokens
	}
	return &annotated
}
