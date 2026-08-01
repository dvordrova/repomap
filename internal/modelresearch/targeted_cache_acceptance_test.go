package modelresearch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteRoundCachesOnlyAcceptedTargetedResponse(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	provider := &savedProvider{response: []byte(`{
  "findings":[{"id":"invalid","interpretation":"unsupported evidence","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`)}
	input := ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
	}

	rejected, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != RoundRejected || rejected.StopReason != "all_findings_rejected" {
		t.Fatalf("rejected round = %#v", rejected)
	}
	assertTargetedCacheFileCount(t, runsDir, 0)

	provider.response = targetedCacheValidResponse(t, plan)
	cold, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if cold.Status != RoundCompleted || cold.Cached {
		t.Fatalf("cold accepted round = %#v", cold)
	}
	assertTargetedCacheFileCount(t, runsDir, 1)
	record := readTargetedCacheRecord(t, runsDir, cold.CacheKey)
	if record.Version != cacheRecordVersion || record.CacheContract != targetedResearchCacheContractVersion ||
		string(record.Response) != string(provider.response) {
		t.Fatalf("accepted cache record = %#v", record)
	}

	warm, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if warm.Status != RoundCached || !warm.Cached {
		t.Fatalf("warm accepted round = %#v", warm)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want rejected live + accepted live and zero warm calls", provider.calls)
	}
	if got, want := normalizedTargetedRound(t, warm), normalizedTargetedRound(t, cold); got != want {
		t.Fatalf("normalized warm round differs from cold\nwarm: %s\ncold: %s", got, want)
	}
}

func TestExecuteRoundDoesNotCacheMalformedTargetedResponse(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	provider := &savedProvider{response: []byte(`{"findings":`)}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
	})
	if err == nil || round.Status != RoundRejected || round.StopReason != "invalid_response" {
		t.Fatalf("malformed round = %#v, err = %v", round, err)
	}
	assertTargetedCacheFileCount(t, runsDir, 0)
}

func TestExecuteRoundEvictsSemanticRejectedCacheBeforeRecompute(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	repository := RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"}
	input := ExecuteInput{
		Plan: plan, Policy: policy, Repository: repository,
		RunsDir: runsDir, Profile: "test", Model: "saved",
	}
	invalid := []byte(`{
  "findings":[{"id":"invalid","interpretation":"unsupported evidence","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`)
	cacheKey := seedTargetedCacheRecord(t, input, invalid)
	provider := &savedProvider{response: targetedCacheValidResponse(t, plan)}
	input.Provider = provider

	recomputed, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if recomputed.Status != RoundCompleted || recomputed.Cached || len(recomputed.RejectedFindings) != 0 {
		t.Fatalf("recomputed round = %#v", recomputed)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one recomputation", provider.calls)
	}
	record := readTargetedCacheRecord(t, runsDir, cacheKey)
	if string(record.Response) != string(provider.response) {
		t.Fatalf("recomputed cache response = %s, want %s", record.Response, provider.response)
	}

	warm, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if warm.Status != RoundCached || provider.calls != 1 {
		t.Fatalf("warm round = %#v, provider calls = %d", warm, provider.calls)
	}
	if got, want := normalizedTargetedRound(t, warm), normalizedTargetedRound(t, recomputed); got != want {
		t.Fatalf("normalized warm round differs from recomputed\nwarm: %s\nrecomputed: %s", got, want)
	}
}

func TestExecuteRoundRemovesSemanticRejectedCacheWhenRecomputeFails(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	input := ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunsDir:    runsDir, Profile: "test", Model: "saved",
	}
	cacheKey := seedTargetedCacheRecord(t, input, []byte(`{
  "findings":[{"id":"invalid","interpretation":"unsupported evidence","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`))
	provider := &savedProvider{err: errors.New("provider unavailable")}
	input.Provider = provider

	round, err := ExecuteRound(context.Background(), input)
	if err == nil || round.Status != RoundFailed || round.StopReason != "provider_call_failed" {
		t.Fatalf("failed recomputation = %#v, err = %v", round, err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one explicit recomputation attempt", provider.calls)
	}
	if _, statErr := os.Stat(cachePath(runsDir, cacheKey)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected exact cache entry still exists, stat error = %v", statErr)
	}
}

func TestTargetedResearchCacheIdentityIncludesCurrentContract(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	input := ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		Profile:    "test", Model: "saved",
	}
	bundleSHA, _, err := BundleHash(plan.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	current := targetedResearchCacheFingerprint(input, bundleSHA)
	currentKey, err := CacheKey(current)
	if err != nil {
		t.Fatal(err)
	}
	old := current
	old.CacheContract = "targeted-research-cache-v2"
	oldKey, err := CacheKey(old)
	if err != nil {
		t.Fatal(err)
	}
	if currentKey == oldKey {
		t.Fatalf("targeted cache contract drift kept key %q", currentKey)
	}
}

func targetedCacheTestPlan(t *testing.T) (PlannedRound, Policy) {
	t.Helper()
	input := basicPlanningInput(t)
	result, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 1 {
		t.Fatalf("selected rounds = %d, want one", len(result.Selected))
	}
	return result.Selected[0], input.Policy
}

func targetedCacheValidResponse(t *testing.T, plan PlannedRound) []byte {
	t.Helper()
	response, err := json.Marshal(researchResponse{Findings: []RawFinding{{
		ID: "finding-1", Interpretation: "backup is coordinated here",
		HypothesisAssessment: "supported", EvidenceIDs: []string{plan.Bundle.Evidence[0].ID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func seedTargetedCacheRecord(t *testing.T, input ExecuteInput, response []byte) string {
	t.Helper()
	bundleSHA, _, err := BundleHash(input.Plan.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(input.Plan.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{}
	request, err := provider.BuildResearchRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := targetedResearchCacheFingerprint(input, bundleSHA)
	cacheKey, err := CacheKey(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCache(input.RunsDir, cacheRecord{
		Version: cacheRecordVersion, CacheKey: cacheKey,
		CacheContract: fingerprint.CacheContract,
		RequestSHA256: requestHash(request), BundleSHA256: bundleSHA,
		ResponseSHA256: requestHash(response), Response: append([]byte(nil), response...),
		RequestBytes: len(request), ResponseBytes: len(response),
	}); err != nil {
		t.Fatal(err)
	}
	return cacheKey
}

func assertTargetedCacheFileCount(t *testing.T, runsDir string, want int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(runsDir, cacheDirectory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != want {
		t.Fatalf("targeted cache files = %v, want %d", files, want)
	}
}

func readTargetedCacheRecord(t *testing.T, runsDir, cacheKey string) cacheRecord {
	t.Helper()
	data, err := os.ReadFile(cachePath(runsDir, cacheKey))
	if err != nil {
		t.Fatal(err)
	}
	var record cacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func normalizedTargetedRound(t *testing.T, round ResearchRound) string {
	t.Helper()
	round.Status = RoundCompleted
	round.Cached = false
	round.CompletedAt = ""
	encoded, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
