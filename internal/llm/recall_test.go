package llm

import (
	"errors"
	"testing"
)

func TestRecallJSONNeverCallsProviderAndRevalidatesCurrentContract(t *testing.T) {
	provider, call := baseTestProvider(), baseTestCall("one", "request")
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	miss, err := RecallJSON(t.Context(), executor, provider, call)
	if err != nil || miss.Cached || provider.completeCalls != 0 {
		t.Fatalf("cache miss made a live request: %+v / %v", miss, err)
	}
	if _, err := ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	hit, err := RecallJSON(t.Context(), executor, provider, call)
	if err != nil || !hit.Cached || hit.Value.Value != "ok" || provider.completeCalls != 1 {
		t.Fatal("complete cached response was not restored")
	}
	call.Validate = func(testValue) error { return errors.New("current contract refuses this answer") }
	refused, err := RecallJSON(t.Context(), executor, provider, call)
	if err != nil || refused.Cached || len(refused.Issues) != 1 || refused.Issues[0].Kind != IssueCacheValidate || provider.completeCalls != 1 {
		t.Fatalf("invalid cached answer acquired authority or triggered a request: %+v / %v", refused, err)
	}
	executor.Enabled = false
	if value, err := RecallJSON(t.Context(), executor, provider, call); err != nil || value.Cached || provider.completeCalls != 1 {
		t.Fatal("NoCache recall called the provider")
	}
}
