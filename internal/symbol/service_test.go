package symbol_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/symbol"
)

func TestServiceUsesFixedExplainerFixture(t *testing.T) {
	t.Parallel()

	var bundle symbol.Bundle
	if err := json.Unmarshal(deepseektest.SymbolBundleJSON, &bundle); err != nil {
		t.Fatalf("unmarshal fixture bundle: %v", err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("fixture bundle Validate() error = %v", err)
	}
	explainer := deepseektest.NewExplainer()
	result, err := symbol.NewService(explainer).Explain(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if result.Evaluation.Score != 100 {
		t.Fatalf("evaluation score = %d, want 100; warnings = %#v", result.Evaluation.Score, result.Parsed.Warnings)
	}
	if result.Parsed.Report.Target.Name != "kvServer.Put" {
		t.Fatalf("target = %q", result.Parsed.Report.Target.Name)
	}
	requests := explainer.Requests()
	if len(requests) != 1 || !json.Valid(requests[0]) {
		t.Fatalf("requests = %d, want one valid JSON bundle", len(requests))
	}
}

func TestServiceRejectsInvalidBundleBeforeProviderCall(t *testing.T) {
	t.Parallel()

	var bundle symbol.Bundle
	if err := json.Unmarshal(deepseektest.SymbolBundleJSON, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Warnings = append(bundle.Warnings, "raw analyzer warning at /private/repo")
	explainer := deepseektest.NewExplainer()
	if _, err := symbol.NewService(explainer).Explain(context.Background(), bundle); err == nil {
		t.Fatal("Explain() accepted invalid provider bundle")
	}
	if requests := explainer.Requests(); len(requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(requests))
	}
}
