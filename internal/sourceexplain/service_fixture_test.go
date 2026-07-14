package sourceexplain_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestServiceUsesFixedSourceFixture(t *testing.T) {
	t.Parallel()

	var bundle sourceexplain.Bundle
	if err := json.Unmarshal(deepseektest.SourceBundleJSON, &bundle); err != nil {
		t.Fatalf("unmarshal fixture bundle: %v", err)
	}
	explainer := deepseektest.NewExplainer()
	result, err := sourceexplain.NewService(explainer).Explain(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if result.Evaluation.Score != 100 {
		t.Fatalf("evaluation score = %d, want 100; warnings = %#v", result.Evaluation.Score, result.Parsed.Warnings)
	}
	if len(result.Parsed.Report.Claims) != 4 {
		t.Fatalf("claims = %#v", result.Parsed.Report.Claims)
	}
	requests := explainer.SourceRequests()
	if len(requests) != 1 || !json.Valid(requests[0]) {
		t.Fatalf("requests = %d, want one valid JSON bundle", len(requests))
	}
}
