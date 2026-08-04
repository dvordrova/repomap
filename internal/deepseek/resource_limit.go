package deepseek

import (
	"errors"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

type ResourceLimitKind = modelresearch.ResourceLimitKind

const (
	ResourceLimitRequestBytes  = modelresearch.ResourceLimitRequestBytes
	ResourceLimitResponseBytes = modelresearch.ResourceLimitResponseBytes
	ResourceLimitRecordBytes   = modelresearch.ResourceLimitRecordBytes
	ResourceLimitCatalogItems  = modelresearch.ResourceLimitCatalogItems
	ResourceLimitOutputTokens  = modelresearch.ResourceLimitOutputTokens
	ResourceLimitSemanticCalls = modelresearch.ResourceLimitSemanticCalls
)

type ResourceLimitError = modelresearch.ResourceLimitError

func annotateResourceLimit(err error, stage string, maxTokens int) error {
	var resourceErr *ResourceLimitError
	if !errors.As(err, &resourceErr) {
		return err
	}
	annotated := resourceErr.Clone()
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
