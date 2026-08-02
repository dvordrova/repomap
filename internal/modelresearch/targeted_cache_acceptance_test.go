package modelresearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
)

func TestExecuteRoundRecordsLiveAndValidatedCachedExchange(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	provider := &savedProvider{response: targetedCacheValidResponse(t, plan)}
	repository := RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"}

	coldWriter, err := debugdump.NewWriter(runsDir, "cold-run", true)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan, Policy: policy, Repository: repository,
		RunsDir: runsDir, RunDir: coldWriter.RunDir(),
		Profile: "test", Model: "saved", Provider: provider,
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
		ExchangeWriter:         coldWriter, ExchangeOrdinal: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := coldWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if cold.Status != RoundCompleted || cold.Cached {
		t.Fatalf("cold round = %#v", cold)
	}
	coldRecords := readTargetedSemanticRecords(t, coldWriter.RunDir())
	if len(coldRecords) != 1 || coldRecords[0].Stage != debugdump.SemanticStageTargetedResearch ||
		coldRecords[0].State != debugdump.SemanticStateAccepted ||
		coldRecords[0].RequestProvenance != debugdump.SemanticRequestPrepared ||
		coldRecords[0].SemanticCalls != 1 || coldRecords[0].TransportAttempts != 1 {
		t.Fatalf("cold semantic exchange = %#v", coldRecords)
	}
	for _, removed := range []string{"request.redacted.json", "response.raw.json"} {
		if _, err := os.Stat(filepath.Join(coldWriter.RunDir(), "research", plan.Bundle.RoundID, removed)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("replaced targeted artifact %s still exists: %v", removed, err)
		}
	}
	if _, err := os.Stat(filepath.Join(
		coldWriter.RunDir(), "research", plan.Bundle.RoundID, "evidence_bundle.json",
	)); err != nil {
		t.Fatalf("targeted evidence bundle was not retained: %v", err)
	}

	warmWriter, err := debugdump.NewWriter(runsDir, "warm-run", true)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan, Policy: policy, Repository: repository,
		RunsDir: runsDir, RunDir: warmWriter.RunDir(),
		Profile: "test", Model: "saved", Provider: provider,
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
		ExchangeWriter:         warmWriter, ExchangeOrdinal: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := warmWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if warm.Status != RoundCached || !warm.Cached || provider.calls != 1 {
		t.Fatalf("warm round = %#v, provider calls = %d", warm, provider.calls)
	}
	warmRecords := readTargetedSemanticRecords(t, warmWriter.RunDir())
	if len(warmRecords) != 1 || warmRecords[0].State != debugdump.SemanticStateCacheHit ||
		warmRecords[0].ValidationCode != debugdump.SemanticValidationCache ||
		warmRecords[0].SemanticCalls != 0 || warmRecords[0].TransportAttempts != 0 ||
		warmRecords[0].Response.Storage != "raw_content" {
		t.Fatalf("warm semantic exchange = %#v", warmRecords)
	}
}

func TestExecuteRoundJournalFailureWarnsWithoutChangingAcceptedResult(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	provider := &savedProvider{response: targetedCacheValidResponse(t, plan)}
	runsDir := t.TempDir()
	writer, err := debugdump.NewWriter(runsDir, "closed-writer", true)
	if err != nil {
		t.Fatal(err)
	}
	var warnings bytes.Buffer
	writer.SetWarningWriter(&warnings)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunDir:     writer.RunDir(), Profile: "test", Model: "saved", Provider: provider,
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
		ExchangeWriter:         writer, ExchangeOrdinal: 1,
	})
	if err != nil || round.Status != RoundCompleted || provider.calls != 1 {
		t.Fatalf("accepted result changed by journal failure: round=%#v calls=%d err=%v", round, provider.calls, err)
	}
	if got := warnings.String(); got != "warning: semantic exchange journal unavailable: stage=targeted_research code=artifact_write_failed\n" {
		t.Fatalf("bounded semantic warning = %q", got)
	}
}

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
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
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

func TestExecuteRoundCacheBindsEndpointAndExactRequest(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	provider := &savedProvider{
		response: targetedCacheValidResponse(t, plan), requestMaxTokens: 8_000,
	}
	endpointA := targetedCacheEndpointSHA256(t)
	endpointB, err := ProviderEndpointSHA256("https://targeted-cache-b.test/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	input := ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
		ProviderEndpointSHA256: endpointA,
	}
	run := func() ResearchRound {
		t.Helper()
		round, err := ExecuteRound(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		return round
	}

	if cold := run(); cold.Cached {
		t.Fatalf("endpoint A cold round = %#v", cold)
	}
	if warm := run(); !warm.Cached {
		t.Fatalf("endpoint A warm round = %#v", warm)
	}
	input.ProviderEndpointSHA256 = endpointB
	if cold := run(); cold.Cached {
		t.Fatalf("endpoint B reused endpoint A response: %#v", cold)
	}
	input.ProviderEndpointSHA256 = endpointA
	provider.requestMaxTokens = 16_000
	if cold := run(); cold.Cached {
		t.Fatalf("changed exact request reused prior response: %#v", cold)
	}
	provider.requestMaxTokens = 8_000
	if warm := run(); !warm.Cached {
		t.Fatalf("original exact request did not retain its cache: %#v", warm)
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want endpoint A + endpoint B + request variant", provider.calls)
	}
	assertTargetedCacheFileCount(t, runsDir, 3)
}

func TestExecuteRoundDoesNotCacheMalformedTargetedResponse(t *testing.T) {
	plan, policy := targetedCacheTestPlan(t)
	runsDir := t.TempDir()
	provider := &savedProvider{response: []byte(`{"findings":`)}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan, Policy: policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
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
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
	}
	invalid := []byte(`{
  "findings":[{"id":"invalid","interpretation":"unsupported evidence","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`)
	cacheKey := seedTargetedCacheRecord(t, input, invalid)
	provider := &savedProvider{response: targetedCacheValidResponse(t, plan)}
	input.Provider = provider
	writer, err := debugdump.NewWriter(runsDir, "invalid-cache-recompute", true)
	if err != nil {
		t.Fatal(err)
	}
	input.RunDir = writer.RunDir()
	input.ExchangeWriter = writer
	input.ExchangeOrdinal = 1

	recomputed, err := ExecuteRound(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if recomputed.Status != RoundCompleted || recomputed.Cached || len(recomputed.RejectedFindings) != 0 {
		t.Fatalf("recomputed round = %#v", recomputed)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one recomputation", provider.calls)
	}
	records := readTargetedSemanticRecords(t, writer.RunDir())
	if len(records) != 1 || records[0].State != debugdump.SemanticStateAccepted ||
		records[0].SemanticCalls != 1 || records[0].TransportAttempts != 1 {
		t.Fatalf("invalid cache fabricated a provider exchange: %#v", records)
	}
	record := readTargetedCacheRecord(t, runsDir, cacheKey)
	if string(record.Response) != string(provider.response) {
		t.Fatalf("recomputed cache response = %s, want %s", record.Response, provider.response)
	}

	input.ExchangeWriter = nil
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
		ProviderEndpointSHA256: targetedCacheEndpointSHA256(t),
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
	request, err := (&savedProvider{}).BuildResearchRequest(Prompt{Version: "cache-contract"})
	if err != nil {
		t.Fatal(err)
	}
	input.ProviderEndpointSHA256 = targetedCacheEndpointSHA256(t)
	current := targetedResearchCacheFingerprint(input, bundleSHA, requestHash(request))
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
	if input.ProviderEndpointSHA256 == "" {
		input.ProviderEndpointSHA256 = targetedCacheEndpointSHA256(t)
	}
	fingerprint := targetedResearchCacheFingerprint(input, bundleSHA, requestHash(request))
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

func targetedCacheEndpointSHA256(t *testing.T) string {
	t.Helper()
	digest, err := ProviderEndpointSHA256("https://targeted-cache.test/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	return digest
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

func readTargetedSemanticRecords(
	t *testing.T,
	runDir string,
) []debugdump.SemanticExchangeRecord {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	records := make([]debugdump.SemanticExchangeRecord, 0, len(directories))
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(
			runDir,
			debugdump.SemanticExchangesDir,
			directory.Name(),
			debugdump.SemanticExchangeMetaFile,
		))
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}
