package deepseek

import (
	"errors"

	"github.com/dvordrova/repomap/internal/llm"
)

const (
	ResourceLimitRequestBytes  = llm.ResourceLimitRequestBytes
	ResourceLimitResponseBytes = llm.ResourceLimitResponseBytes
	ResourceLimitOutputTokens  = llm.ResourceLimitOutputTokens
)

type ResourceLimitError = llm.ResourceLimitError

func newResourceLimitError(details ResourceLimitError) *ResourceLimitError {
	return llm.NewResourceLimitError(details)
}

func annotateResourceLimit(err error, stage string, maxTokens int) error {
	var resourceErr *ResourceLimitError
	if !errors.As(err, &resourceErr) {
		return err
	}
	annotated := resourceErr.Clone()
	if annotated == nil {
		return err
	}
	if annotated.Stage == "" {
		annotated.Stage = stage
	}
	if annotated.ConfiguredMaxTokens == 0 {
		annotated.ConfiguredMaxTokens = maxTokens
	}
	if annotated.Kind == ResourceLimitOutputTokens && annotated.Limit == 0 {
		annotated.Limit = maxTokens
	}
	return annotated
}
