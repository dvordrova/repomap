package llm

import (
	"context"
	"sync"
	"testing"
)

// ownedProvider is a provider with its account's own controller, as the
// run's one Jev client is. It records the gate each attempt runs under.
type ownedProvider struct {
	*testProvider
	controller *BatchController
	mu         sync.Mutex
	gates      []*attemptGate
}

func (provider *ownedProvider) AttemptController() *BatchController { return provider.controller }

func (provider *ownedProvider) Complete(ctx context.Context, prepared Prepared) (Completion, error) {
	provider.mu.Lock()
	provider.gates = append(provider.gates, attemptGateForContext(ctx))
	provider.mu.Unlock()
	return provider.testProvider.Complete(ctx, prepared)
}

// Every call through a provider that owns its controller runs under that
// controller's one gate, whichever executor, batch form or gate already in
// the context the caller brings; its preset limit is the gate's, whatever
// concurrency the first batch asks. Without one, the executor's stays.
func TestAProviderOwnedControllerIsEveryCallersGate(t *testing.T) {
	owned := &ownedProvider{testProvider: baseTestProvider(), controller: NewBatchController(3)}
	executors := []Executor{
		{BatchConcurrency: 12, BatchController: &BatchController{}},
		{BatchConcurrency: 1, BatchController: &BatchController{}},
		{},
	}
	textModelGate := newAttemptGate(12)
	ctx := bindAttemptGate(t.Context(), textModelGate)
	if _, err := ExecuteJSON(ctx, executors[0], owned, baseTestCall("", "direct")); err != nil {
		t.Fatal(err)
	}
	for _, result := range ExecuteJSONEach(t.Context(), executors[1], owned, []Call[testValue]{baseTestCall("", "each a"), baseTestCall("", "each b")}) {
		if result.Err != nil {
			t.Fatal(result.Err)
		}
	}
	if _, err := ExecuteJSONBatch(t.Context(), executors[2], owned, []Call[testValue]{baseTestCall("", "batch a"), baseTestCall("", "batch b")}); err != nil {
		t.Fatal(err)
	}
	gate := owned.controller.gate
	if gate == nil || gate.configured != 3 || len(owned.gates) != 5 {
		t.Fatalf("owned gate %+v, attempts %d", gate, len(owned.gates))
	}
	for _, got := range owned.gates {
		if got != gate {
			t.Fatal("an attempt ran under another gate than its provider's own")
		}
	}
	for _, executor := range executors[:2] {
		if executor.BatchController.gate != nil {
			t.Fatal("an executor's own controller was bound for a provider that owns one")
		}
	}

	plain := &ownedProvider{testProvider: baseTestProvider()}
	executor := Executor{BatchConcurrency: 2, BatchController: &BatchController{}}
	if _, err := ExecuteJSON(t.Context(), executor, plain, baseTestCall("", "plain")); err != nil {
		t.Fatal(err)
	}
	if plain.gates[0] == nil || plain.gates[0] != executor.BatchController.gate {
		t.Fatal("a provider without its own controller left its executor's gate")
	}
}

// Recoverable and Notice draw one line for every caller of an outcome's
// issues: cache and metrics diagnostics go on, a failed cache read, write or
// eviction is printed, and an unwritten journal or an unknown kind stops.
func TestIssueRecoverableAndNotice(t *testing.T) {
	for kind, want := range map[IssueKind]struct {
		recoverable bool
		notice      string
	}{
		IssueCacheRead:     {true, "cache read failed"},
		IssueCacheWrite:    {true, "cache write failed"},
		IssueCacheEvict:    {true, "cache eviction failed"},
		IssueCacheValidate: {true, ""},
		IssueMetrics:       {true, ""},
		IssueObserver:      {false, ""},
		"unknown":          {false, ""},
	} {
		issue := Issue{Kind: kind}
		if issue.Recoverable() != want.recoverable || issue.Notice() != want.notice {
			t.Errorf("%s: recoverable %v notice %q", kind, issue.Recoverable(), issue.Notice())
		}
	}
}
