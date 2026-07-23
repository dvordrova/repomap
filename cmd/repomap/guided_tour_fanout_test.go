package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

type guidedTourFanoutStub struct {
	mu                 sync.Mutex
	leafResponses      map[string][]byte
	monolithicResponse []byte
	finalResponse      []byte
	fanInRequestBytes  int
	callsByVersion     map[string]int
}

func (stub *guidedTourFanoutStub) GuidedTourPromptJSON(prompt guidedtour.Prompt) ([]byte, error) {
	if prompt.Version == guidedtour.FanInPromptVersion && stub.fanInRequestBytes > 0 {
		return []byte(strings.Repeat("x", stub.fanInRequestBytes)), nil
	}
	return json.Marshal(prompt)
}

func (stub *guidedTourFanoutStub) EditGuidedTourMeasured(
	_ context.Context,
	prompt guidedtour.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.callsByVersion == nil {
		stub.callsByVersion = make(map[string]int)
	}
	stub.callsByVersion[prompt.Version]++
	if prompt.Version == guidedtour.PromptVersion {
		response := stub.monolithicResponse
		if response == nil {
			response = stub.finalResponse
		}
		return modelresearch.ProviderResult{
			Content: response, InputTokens: 80, OutputTokens: 40, Attempts: 1,
			PromptCacheHitTokens: 20, PromptCacheMissTokens: 60,
		}, nil
	}
	if prompt.Version == guidedtour.FanInPromptVersion {
		return modelresearch.ProviderResult{
			Content: stub.finalResponse, InputTokens: 80, OutputTokens: 40, Attempts: 1,
			PromptCacheHitTokens: 20, PromptCacheMissTokens: 60,
		}, nil
	}
	for taskID, response := range stub.leafResponses {
		if strings.Contains(prompt.User, taskID) {
			return modelresearch.ProviderResult{
				Content: response, InputTokens: 40, OutputTokens: 20, Attempts: 1,
				PromptCacheHitTokens: 30, PromptCacheMissTokens: 10,
			}, nil
		}
	}
	return modelresearch.ProviderResult{}, fmt.Errorf("unexpected guided tour prompt")
}

func TestCompareGuidedTourStrategiesRecordsRejectedMonolithAndContinuesFanout(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := guidedTourFanoutTestProvider(t, bundle, "")
	invalid := guidedTourTestProposal(t, bundle, false)
	var proposal guidedtour.Proposal
	if err := json.Unmarshal(invalid, &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Steps[0].Explanation = "Read main.go first."
	provider.monolithicResponse = mustJSON(t, proposal)

	comparison, err := compareGuidedTourStrategies(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	monolithic := comparison.Variants[0]
	fanout := comparison.Variants[1]
	if monolithic.ValidationState != "rejected" || monolithic.FailureReason == "" ||
		monolithic.Coverage.CandidateID != "" {
		t.Fatalf("rejected monolithic metrics = %#v", monolithic)
	}
	if fanout.FailureReason != "" || fanout.Coverage.CandidateID == "" {
		t.Fatalf("fan-out metrics = %#v", fanout)
	}
	if _, err := os.Stat(filepath.Join(runDir, guidedTourFanoutFile)); err != nil {
		t.Fatalf("fan-out story missing after monolithic rejection: %v", err)
	}
}

func TestCompareGuidedTourStrategiesPersistsExactCoverageAndMetrics(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := guidedTourFanoutTestProvider(t, bundle, "")

	comparison, err := compareGuidedTourStrategies(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Variants) != 2 || comparison.SelectedStrategy != "" {
		t.Fatalf("comparison = %#v", comparison)
	}
	monolithic := comparison.Variants[0]
	fanout := comparison.Variants[1]
	if monolithic.Strategy != "monolithic" || monolithic.SemanticCalls != 1 ||
		monolithic.PromptCacheHitTokens != 20 || monolithic.PromptCacheMissTokens != 60 ||
		monolithic.Coverage.ReferencedBeats != 3 {
		t.Fatalf("monolithic metrics = %#v", monolithic)
	}
	if fanout.Strategy != "fan_out_fan_in" || fanout.SemanticCalls != 3 ||
		fanout.LeafTasks != 2 || fanout.Coverage.ReferencedBeats != 3 {
		t.Fatalf("fan-out metrics = %#v", fanout)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, guidedTourComparisonFile))
	if err != nil {
		t.Fatal(err)
	}
	var saved guidedtour.Comparison
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if err := saved.Validate(); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 4 {
		t.Fatalf("provider calls = %d, want 4", provider.callCount())
	}
}

func (stub *guidedTourFanoutStub) callCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	total := 0
	for _, count := range stub.callsByVersion {
		total += count
	}
	return total
}

func TestGuidedTourStrategyMetricsUsesCurrentInvocationWallTime(t *testing.T) {
	outcome := guidedTourOutcome{
		LatencyMillis:     9_876,
		WallMillis:        42,
		UnsupportedClaims: 2,
		ValidationState:   "accepted",
	}

	metrics := guidedTourStrategyMetrics(
		"monolithic",
		outcome,
		guidedtour.StoryCoverage{CandidateID: "candidate-1"},
		nil,
	)
	if metrics.WallMillis != outcome.WallMillis {
		t.Fatalf("comparison wall time = %d, want current invocation %d", metrics.WallMillis, outcome.WallMillis)
	}
	if metrics.WallMillis == outcome.LatencyMillis {
		t.Fatalf("comparison wall time reused provider latency: %#v", metrics)
	}
	if metrics.UnsupportedClaims != outcome.UnsupportedClaims {
		t.Fatalf("comparison unsupported claims = %d, want %d", metrics.UnsupportedClaims, outcome.UnsupportedClaims)
	}
}

func TestGuidedTourUnsupportedClaimCountersUseFinalProposal(t *testing.T) {
	bundle := guidedTourTestBundle()
	var proposal guidedtour.Proposal
	if err := json.Unmarshal(guidedTourTestProposal(t, bundle, false), &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Steps[0].Explanation = "The entry routes requests into analysis."
	monolithic := mustJSON(t, proposal)
	if got := countGuidedTourProposalUnsupportedClaims(bundle, monolithic); got != 1 {
		t.Fatalf("monolithic unsupported claims = %d, want 1", got)
	}
	fanIn := mustJSON(t, guidedtour.FanInArtifact{
		Version:     guidedtour.FanInArtifactVersion,
		Verdict:     guidedtour.FanInVerdictMixed,
		Proposal:    &proposal,
		StepSupport: []guidedtour.FanInStepSupport{},
	})
	if got := countGuidedTourFanInUnsupportedClaims(bundle, fanIn); got != 1 {
		t.Fatalf("fan-in unsupported claims = %d, want 1", got)
	}
}

func TestAddGuidedTourResponseMetricsKeepsProviderLatencySeparate(t *testing.T) {
	outcome := guidedTourOutcome{WallMillis: 42}
	addGuidedTourResponseMetrics(&outcome, modelresearch.StageResponse{LatencyMillis: 120})
	addGuidedTourResponseMetrics(&outcome, modelresearch.StageResponse{LatencyMillis: 80})

	if outcome.LatencyMillis != 200 {
		t.Fatalf("provider latency = %d, want aggregate 200", outcome.LatencyMillis)
	}
	if outcome.WallMillis != 42 {
		t.Fatalf("current invocation wall time changed to %d", outcome.WallMillis)
	}
}

func TestEnsureGuidedTourFanoutExperimentCachesValidatedLeavesAndFanIn(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := guidedTourFanoutTestProvider(t, bundle, "")

	first, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ValidationState != "accepted" || first.LeafTasks != 2 ||
		first.LeafSucceeded != 2 || first.LeafFailed != 0 || first.SemanticCalls != 3 ||
		first.CacheHits != 0 || provider.callCount() != 3 {
		t.Fatalf("first outcome = %#v, provider calls = %d", first, provider.callCount())
	}
	if first.InputTokens != 160 || first.OutputTokens != 80 {
		t.Fatalf("first token metrics = %d/%d", first.InputTokens, first.OutputTokens)
	}
	if first.PromptCacheHitTokens != 80 || first.PromptCacheMissTokens != 80 {
		t.Fatalf(
			"first prompt cache metrics = %d/%d",
			first.PromptCacheHitTokens,
			first.PromptCacheMissTokens,
		)
	}
	record, err := os.ReadFile(filepath.Join(runDir, guidedTourFanoutFile))
	if err != nil {
		t.Fatal(err)
	}
	story, err := guidedtour.ReplayRecord(bundle, record)
	if err != nil {
		t.Fatal(err)
	}
	if story.CandidateID != bundle.Candidates[0].ID || len(story.Steps) != 3 {
		t.Fatalf("fan-out story = %#v", story)
	}
	finalRaw, err := os.ReadFile(filepath.Join(runDir, guidedTourFanInFile))
	if err != nil {
		t.Fatal(err)
	}
	finalArtifact, err := guidedtour.ParseFanInArtifact(finalRaw)
	if err != nil {
		t.Fatal(err)
	}
	if finalArtifact.Verdict != guidedtour.FanInVerdictMixed || finalArtifact.Proposal == nil {
		t.Fatalf("persisted fan-in artifact = %#v", finalArtifact)
	}

	replayProvider := guidedTourFanoutTestProvider(t, bundle, "")
	second, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", replayProvider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.ValidationState != "cached" || second.SemanticCalls != 0 ||
		second.CacheHits != 3 || replayProvider.callCount() != 0 {
		t.Fatalf("second outcome = %#v, provider calls = %d", second, replayProvider.callCount())
	}
}

func TestEnsureGuidedTourFanoutCachesOnlyNormalizedLeafProse(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		t.Fatal(err)
	}
	provider := guidedTourFanoutTestProvider(t, bundle, "")
	artifact := guidedTourLeafTestArtifact(tasks[0])
	const rawPath = "cmd/repomap/main.go"
	artifact.Observations[0].Explanation =
		"The exact static fact from " + rawPath + " remains one bounded inspection fact."
	provider.leafResponses[tasks[0].ID] = mustJSON(t, artifact)

	first, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.InputTokens == 0 || first.OutputTokens == 0 ||
		first.PromptCacheHitTokens == 0 || first.PromptCacheMissTokens == 0 {
		t.Fatalf("provider metrics were not retained: %#v", first)
	}

	cacheFiles, err := filepath.Glob(
		filepath.Join(filepath.Dir(runDir), ".model-research", "*.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	foundNormalizedLeaf := false
	for _, cacheFile := range cacheFiles {
		raw, readErr := os.ReadFile(cacheFile)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var record struct {
			Response []byte `json:"response"`
		}
		if unmarshalErr := json.Unmarshal(raw, &record); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		if strings.Contains(string(record.Response), rawPath) {
			t.Fatalf("raw repository path persisted in cache %s", cacheFile)
		}
		if strings.Contains(string(record.Response), "the supplied repository reference") {
			foundNormalizedLeaf = true
		}
	}
	if !foundNormalizedLeaf {
		t.Fatal("normalized leaf artifact was not persisted")
	}

	replayProvider := guidedTourFanoutTestProvider(t, bundle, "")
	replayed, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", replayProvider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Cached || replayProvider.callCount() != 0 {
		t.Fatalf("cache replay = %#v, provider calls = %d", replayed, replayProvider.callCount())
	}
}

func TestEnsureGuidedTourFanoutExperimentDegradesAfterOneRejectedLeaf(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		t.Fatal(err)
	}
	provider := guidedTourFanoutTestProvider(t, bundle, tasks[0].ID)

	outcome, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ValidationState != "accepted_partial" || outcome.LeafSucceeded != 1 ||
		outcome.LeafFailed != 1 || outcome.SemanticCalls != 3 {
		t.Fatalf("partial outcome = %#v", outcome)
	}
	artifactRaw, err := os.ReadFile(filepath.Join(runDir, guidedTourFanoutLeavesFile))
	if err != nil {
		t.Fatal(err)
	}
	var artifact guidedTourFanoutArtifact
	if err := json.Unmarshal(artifactRaw, &artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact.Results) != 1 || len(artifact.Failures) != 1 ||
		artifact.Failures[0].TaskID != tasks[0].ID {
		t.Fatalf("partial leaf artifact = %#v", artifact)
	}
	if _, err := os.Stat(filepath.Join(runDir, guidedTourFanoutFile)); err != nil {
		t.Fatalf("partial fan-in record missing: %v", err)
	}
}

func TestEnsureGuidedTourFanoutExperimentRunsFanInForMissingOnlyLeaves(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := guidedTourFanoutTestProvider(t, bundle, "")
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		provider.leafResponses[task.ID] = mustJSON(
			t,
			guidedTourMissingOnlyLeafTestArtifact(task),
		)
	}
	provider.finalResponse = guidedTourFanInTestResponse(
		t,
		bundle,
		guidedtour.FanInVerdictInsufficientEvidence,
		guidedtour.LeafTask{},
	)
	if err := writeGuidedTourArtifact(
		filepath.Join(runDir, guidedTourFanoutFile),
		[]byte("stale story must be removed\n"),
	); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err == nil || !strings.Contains(err.Error(), "insufficient evidence") {
		t.Fatalf("ensureGuidedTourFanoutExperiment() error = %v", err)
	}
	if outcome.ValidationState != "insufficient_evidence" ||
		outcome.LeafInsufficient != len(tasks) || outcome.LeafSucceeded != len(tasks) ||
		outcome.SemanticCalls != len(tasks)+1 {
		t.Fatalf("insufficient outcome = %#v", outcome)
	}
	if provider.callsByVersion[guidedtour.FanInPromptVersion] != 1 {
		t.Fatalf("fan-in calls = %d, want 1", provider.callsByVersion[guidedtour.FanInPromptVersion])
	}
	finalRaw, err := os.ReadFile(filepath.Join(runDir, guidedTourFanInFile))
	if err != nil {
		t.Fatal(err)
	}
	finalArtifact, err := guidedtour.ParseFanInArtifact(finalRaw)
	if err != nil {
		t.Fatal(err)
	}
	if finalArtifact.Verdict != guidedtour.FanInVerdictInsufficientEvidence ||
		finalArtifact.Proposal != nil {
		t.Fatalf("persisted fan-in artifact = %#v", finalArtifact)
	}
	if _, err := os.Stat(filepath.Join(runDir, guidedTourFanoutFile)); !os.IsNotExist(err) {
		t.Fatalf("stale fan-in story exists or stat failed: %v", err)
	}

	replayProvider := guidedTourFanoutTestProvider(t, bundle, "")
	replayed, replayErr := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", replayProvider,
	)
	if replayErr == nil || !strings.Contains(replayErr.Error(), "insufficient evidence") {
		t.Fatalf("cached ensureGuidedTourFanoutExperiment() error = %v", replayErr)
	}
	if !replayed.Cached || replayed.ValidationState != "insufficient_evidence" ||
		replayed.SemanticCalls != 0 || replayed.CacheHits != len(tasks)+1 ||
		replayProvider.callCount() != 0 {
		t.Fatalf("cached insufficient outcome = %#v", replayed)
	}
}

func TestAllowsFanoutCallUsesAggregateAndPerRequestBounds(t *testing.T) {
	policy, err := modelresearch.DefaultPolicy().WithGuidedTourBudget(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		calls         int
		bytes         int
		request       int
		wantAllowed   bool
		wantReasonSub string
	}{
		{name: "allowed", calls: 3, bytes: 700, request: 300, wantAllowed: true},
		{name: "call bound", calls: 4, request: 1, wantReasonSub: "call"},
		{name: "aggregate bytes", calls: 1, bytes: policy.MaxGuidedTourBytes - 100, request: 101, wantReasonSub: "byte"},
		{name: "empty", request: 0, wantReasonSub: "empty"},
		{name: "per request", request: policy.GuidedTour.MaxRequestBytes + 1, wantReasonSub: "stage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := allowsFanoutCall(policy, tt.calls, tt.bytes, tt.request)
			if allowed != tt.wantAllowed || (!allowed && !strings.Contains(reason, tt.wantReasonSub)) {
				t.Fatalf("allowsFanoutCall() = %t, %q", allowed, reason)
			}
		})
	}
}

func TestEnsureGuidedTourFanoutRejectsOversizedSerializedFanInRequest(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		t.Fatal(err)
	}
	provider := guidedTourFanoutTestProvider(t, bundle, "")
	provider.fanInRequestBytes = modelresearch.DefaultPolicy().GuidedTour.MaxRequestBytes + 1

	outcome, err := ensureGuidedTourFanoutExperiment(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err == nil || !strings.Contains(err.Error(), "stage_byte_budget_exhausted") {
		t.Fatalf("ensureGuidedTourFanoutExperiment() error = %v", err)
	}
	if outcome.ValidationState != "skipped_fan_in_stage_byte_budget_exhausted" ||
		outcome.SemanticCalls != len(tasks) ||
		provider.callsByVersion[guidedtour.FanInPromptVersion] != 0 {
		t.Fatalf("oversized fan-in outcome = %#v, calls = %#v", outcome, provider.callsByVersion)
	}
}

func TestAllowsFanoutLeafCallReservesMaxFanInEnvelope(t *testing.T) {
	policy, err := modelresearch.DefaultPolicy().WithGuidedTourBudget(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	leafByteLimit := policy.MaxGuidedTourBytes - policy.GuidedTour.MaxRequestBytes
	if leafByteLimit <= 0 {
		t.Fatalf("leaf byte limit = %d, want positive", leafByteLimit)
	}

	allowed, reason := allowsFanoutLeafCall(
		policy,
		policy.MaxGuidedTourCalls-2,
		leafByteLimit-1,
		1,
	)
	if !allowed {
		t.Fatalf("exact leaf reservation rejected: %s", reason)
	}
	if allowed, reason := allowsFanoutCall(
		policy,
		policy.MaxGuidedTourCalls-1,
		leafByteLimit,
		policy.GuidedTour.MaxRequestBytes,
	); !allowed {
		t.Fatalf("reserved maximum fan-in request rejected: %s", reason)
	}

	if allowed, reason := allowsFanoutLeafCall(
		policy,
		policy.MaxGuidedTourCalls-2,
		leafByteLimit,
		1,
	); allowed || reason != "fan_in_byte_budget_reserved" {
		t.Fatalf("byte envelope reservation = %t, %q", allowed, reason)
	}
	if allowed, reason := allowsFanoutLeafCall(
		policy,
		policy.MaxGuidedTourCalls-1,
		0,
		1,
	); allowed || reason != "fan_in_call_reserved" {
		t.Fatalf("call slot reservation = %t, %q", allowed, reason)
	}
}

func TestGuidedTourLeafFailureReasonIsBoundedAndSingleLine(t *testing.T) {
	reason := guidedTourLeafFailureReason(
		"leaf response failed local validation",
		fmt.Errorf("first\nsecond %s", strings.Repeat("x", 1024)),
	)
	if len(reason) > 512 || strings.ContainsAny(reason, "\r\n") ||
		!strings.Contains(reason, "first second") {
		t.Fatalf("guidedTourLeafFailureReason() = %q", reason)
	}
}

func guidedTourFanoutTestProvider(
	t *testing.T,
	bundle guidedtour.Bundle,
	invalidTaskID string,
) *guidedTourFanoutStub {
	t.Helper()
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		t.Fatal(err)
	}
	responses := make(map[string][]byte, len(tasks))
	var supportTask guidedtour.LeafTask
	for _, task := range tasks {
		artifact := guidedTourLeafTestArtifact(task)
		if task.ID == invalidTaskID {
			artifact.Observations[0].SupportIDs[0] = "invented-beat"
			artifact.CandidateConnection.SupportIDs[0] = "invented-beat"
		}
		responses[task.ID] = mustJSON(t, artifact)
		if supportTask.ID == "" && task.ID != invalidTaskID {
			supportTask = task
		}
	}
	return &guidedTourFanoutStub{
		leafResponses:      responses,
		monolithicResponse: guidedTourTestProposal(t, bundle, false),
		finalResponse: guidedTourFanInTestResponse(
			t,
			bundle,
			guidedtour.FanInVerdictMixed,
			supportTask,
		),
	}
}

func guidedTourLeafTestArtifact(task guidedtour.LeafTask) guidedtour.LeafArtifact {
	beatIDs := make([]string, 0, len(task.Candidate.Beats))
	observations := make([]guidedtour.LeafObservation, 0, len(task.Candidate.Beats))
	for _, beat := range task.Candidate.Beats {
		beatIDs = append(beatIDs, beat.ID)
		observations = append(observations, guidedtour.LeafObservation{
			Explanation: "This exact beat remains one bounded static inspection fact.",
			SupportIDs:  []string{beat.ID},
		})
	}
	return guidedtour.LeafArtifact{
		Version:      guidedtour.LeafArtifactVersion,
		TaskID:       task.ID,
		CandidateID:  task.CandidateID,
		Observations: observations,
		CandidateConnection: guidedtour.LeafCandidateConnection{
			CandidateID: task.CandidateID,
			TargetID:    task.Candidate.Beats[0].ID,
			Relation:    guidedtour.LeafConnectionNeedsCombination,
			Explanation: "This local fragment does not establish a complete story alone.",
			SupportIDs:  beatIDs,
		},
		MissingEvidence: []guidedtour.LeafMissingEvidence{},
	}
}

func guidedTourMissingOnlyLeafTestArtifact(task guidedtour.LeafTask) guidedtour.LeafArtifact {
	missing := guidedtour.LeafMissingEvidence{
		Explanation: "The supplied exact facts do not establish the missing connection.",
	}
	if len(task.Candidate.Gaps) > 0 {
		missing.GapIDs = []string{task.Candidate.Gaps[0].ID}
	} else {
		missing.BeatIDs = []string{task.Candidate.Beats[0].ID}
	}
	return guidedtour.LeafArtifact{
		Version:      guidedtour.LeafArtifactVersion,
		TaskID:       task.ID,
		CandidateID:  task.CandidateID,
		Observations: []guidedtour.LeafObservation{},
		CandidateConnection: guidedtour.LeafCandidateConnection{
			CandidateID: task.CandidateID,
			TargetID:    task.Candidate.Beats[0].ID,
			Relation:    guidedtour.LeafConnectionNeedsCombination,
			Explanation: "This local fragment does not establish a complete story alone.",
			SupportIDs:  []string{},
		},
		MissingEvidence: []guidedtour.LeafMissingEvidence{missing},
	}
}

func guidedTourFanInTestResponse(
	t *testing.T,
	bundle guidedtour.Bundle,
	verdict guidedtour.FanInVerdict,
	supportTask guidedtour.LeafTask,
) []byte {
	t.Helper()
	artifact := guidedtour.FanInArtifact{
		Version:     guidedtour.FanInArtifactVersion,
		Verdict:     verdict,
		StepSupport: []guidedtour.FanInStepSupport{},
	}
	if verdict == guidedtour.FanInVerdictInsufficientEvidence {
		artifact.Explanation = "The validated leaves do not establish an honest guided story."
		return mustJSON(t, artifact)
	}
	var proposal guidedtour.Proposal
	if err := json.Unmarshal(guidedTourTestProposal(t, bundle, false), &proposal); err != nil {
		t.Fatal(err)
	}
	if supportTask.ID == "" || supportTask.CandidateID != proposal.CandidateID {
		t.Fatalf("fan-in test support task = %#v for proposal %q", supportTask, proposal.CandidateID)
	}
	leafArtifact := guidedTourLeafTestArtifact(supportTask)
	for stepIndex, step := range proposal.Steps {
		observationIndex := -1
		for index, observation := range leafArtifact.Observations {
			supportsStep := true
			for _, beatID := range step.BeatIDs {
				found := false
				for _, supportID := range observation.SupportIDs {
					found = found || supportID == beatID
				}
				if !found {
					supportsStep = false
					break
				}
			}
			if supportsStep {
				observationIndex = index
				break
			}
		}
		if observationIndex < 0 {
			t.Fatalf("proposal step %d has no fixture observation support", stepIndex)
		}
		artifact.StepSupport = append(artifact.StepSupport, guidedtour.FanInStepSupport{
			StepIndex: stepIndex,
			Refs: []guidedtour.FanInObservationRef{{
				TaskID: supportTask.ID, ObservationIndex: observationIndex,
			}},
		})
	}
	artifact.Explanation = "The combined exact observations support a bounded editorial story."
	artifact.Proposal = &proposal
	return mustJSON(t, artifact)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
