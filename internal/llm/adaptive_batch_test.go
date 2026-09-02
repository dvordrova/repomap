package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type adaptiveBatchTestProvider struct {
	kind     ResourceLimitKind
	atomic   bool
	attempts int
}

func (*adaptiveBatchTestProvider) State() []byte {
	return []byte(`{"provider":"adaptive-batch-test"}`)
}

func (*adaptiveBatchTestProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	return NewPrepared([]byte(prompt.User))
}

func (provider *adaptiveBatchTestProvider) Complete(
	_ context.Context,
	prepared Prepared,
) (Completion, error) {
	provider.attempts++
	var values []string
	if err := json.Unmarshal(prepared.Bytes(), &values); err != nil {
		return Completion{}, err
	}
	if provider.atomic || len(values) > 1 {
		return Completion{}, NewResourceLimitError(ResourceLimitError{
			Kind: provider.kind, Limit: 1, Observed: len(values), ObservedKnown: true,
		})
	}
	response, err := json.Marshal(struct {
		Value string `json:"value"`
	}{Value: values[0]})
	if err != nil {
		return Completion{}, err
	}
	return Completion{
		Response: response, FinishReason: FinishStop, ChoiceCount: 1,
		Metrics: Metrics{Attempts: 1},
	}, nil
}

func adaptiveBatchTestBuild(items [][]string) ([]Call[testValue], error) {
	calls := make([]Call[testValue], len(items))
	for position, item := range items {
		wire, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		calls[position] = Call[testValue]{
			State:  []byte(`{"item":"` + strings.Join(item, ",") + `"}`),
			Prompt: Prompt{User: string(wire), ResponseFormatJSON: true},
			Limits: Limits{
				MaxRequestBytes: 1024, MaxResponseBytes: ProviderResponseByteLimit,
				MaxOutputTokens: 128_000,
			},
		}
	}
	return calls, nil
}

func adaptiveBatchTestSplit(item []string) ([]string, []string, bool) {
	if len(item) <= 1 {
		return nil, nil, false
	}
	middle := len(item) / 2
	return append([]string(nil), item[:middle]...), append([]string(nil), item[middle:]...), true
}

func TestExecuteAdaptiveJSONBatchRetriesCompletePlanUntilFullCover(t *testing.T) {
	provider := &adaptiveBatchTestProvider{kind: ResourceLimitResponseBytes}
	plan, outcomes, err := ExecuteAdaptiveJSONBatch(
		t.Context(), Executor{}, provider,
		[][]string{{"a", "b", "c", "d"}},
		adaptiveBatchTestBuild, adaptiveBatchTestSplit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 4 || len(outcomes) != 4 {
		t.Fatalf("final plan/outcomes = %d/%d, want 4/4", len(plan), len(outcomes))
	}
	for position, want := range []string{"a", "b", "c", "d"} {
		if len(plan[position]) != 1 || plan[position][0] != want ||
			outcomes[position].Value.Value != want {
			t.Fatalf("final item %d = %#v / %#v", position, plan[position], outcomes[position])
		}
	}
	if provider.attempts <= len(outcomes) {
		t.Fatalf("provider attempts = %d, want failed envelope attempts plus full cover", provider.attempts)
	}
}

func TestExecuteAdaptiveJSONBatchWithAccountingKeepsDiscardedRoundsOperationalOnly(t *testing.T) {
	provider := &adaptiveBatchTestProvider{kind: ResourceLimitResponseBytes}
	plan, outcomes, accounting, err := ExecuteAdaptiveJSONBatchWithAccounting(
		t.Context(), Executor{}, provider,
		[][]string{{"a", "b", "c", "d"}},
		adaptiveBatchTestBuild, adaptiveBatchTestSplit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 4 || len(outcomes) != 4 {
		t.Fatalf("final plan/outcomes = %d/%d, want 4/4", len(plan), len(outcomes))
	}
	if accounting.DiscardedLiveCalls != 5 || accounting.DiscardedAttempts != 2 ||
		accounting.DiscardedLatency != 0 {
		t.Fatalf("discarded accounting = %#v", accounting)
	}
	for position, outcome := range outcomes {
		if outcome.Value.Value == "" || outcome.Metrics.Attempts != 1 {
			t.Fatalf("final semantic outcome %d = %#v", position, outcome)
		}
	}
}

func TestExecuteAdaptiveJSONBatchKeepsAtomicResourceFailureTerminal(t *testing.T) {
	provider := &adaptiveBatchTestProvider{kind: ResourceLimitOutputTokens, atomic: true}
	_, _, err := ExecuteAdaptiveJSONBatch(
		t.Context(), Executor{}, provider,
		[][]string{{"a"}},
		adaptiveBatchTestBuild, adaptiveBatchTestSplit,
	)
	if err == nil {
		t.Fatal("atomic provider resource failure was accepted")
	}
	var itemErr *BatchItemError
	var resourceErr *ResourceLimitError
	if !errors.As(err, &itemErr) || itemErr.Index != 0 ||
		!errors.As(err, &resourceErr) || resourceErr.Kind != ResourceLimitOutputTokens {
		t.Fatalf("atomic failure = %v", err)
	}
	if provider.attempts != 1 {
		t.Fatalf("provider attempts = %d, want 1", provider.attempts)
	}
}
