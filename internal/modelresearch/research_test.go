package modelresearch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type savedProvider struct {
	response []byte
	err      error
	calls    int
}

func (p *savedProvider) BuildResearchRequest(prompt Prompt) ([]byte, error) {
	return json.Marshal(prompt)
}

func (p *savedProvider) Research(context.Context, Prompt) (ProviderResult, error) {
	p.calls++
	return ProviderResult{Content: append([]byte(nil), p.response...), Attempts: 1}, p.err
}

func TestPlanTargetedRoundsSelectsOnlyFocusedExactEvidence(t *testing.T) {
	repo := researchFixtureRepo(t)
	policy := DefaultPolicy()
	result, err := PlanTargetedRounds(PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "q-backup", Purpose: "understand backup responsibility",
			Question: "How does backup reach repository behavior?", CandidateIDs: []string{"candidate-backup"},
		}},
		Candidates: []FileCandidate{
			{ID: "candidate-backup", Path: "cmd/backup.go", Score: 100},
			{ID: "candidate-unrelated", Path: "internal/unrelated.go", Score: 99},
		},
		InitialProviderPaths: []string{"cmd/backup.go"},
		Universe: LocalRepositoryUniverse{AuthorizedPaths: []string{
			"cmd/backup.go", "internal/repository/repository.go", "internal/unrelated.go",
		}},
		Policy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 1 {
		t.Fatalf("selected rounds = %d, want 1; skipped=%#v", len(result.Selected), result.Skipped)
	}
	bundle := result.Selected[0].Bundle
	if containsString(bundle.ProviderAllowedPaths, "internal/unrelated.go") {
		t.Fatalf("targeted provider paths include unselected unrelated file: %v", bundle.ProviderAllowedPaths)
	}
	if !containsString(bundle.ProviderAllowedPaths, "cmd/backup.go") {
		t.Fatalf("targeted provider paths = %v, want selected backup file", bundle.ProviderAllowedPaths)
	}
	for _, item := range bundle.Evidence {
		if item.Location != nil && item.Location.Path == "internal/unrelated.go" {
			t.Fatalf("targeted evidence includes unselected file: %#v", item)
		}
	}
}

func TestPlanTargetedRoundsSkipsWithoutNewExactEvidence(t *testing.T) {
	input := basicPlanningInput(t)
	first, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Selected) != 1 {
		t.Fatalf("first selected rounds = %d, want 1", len(first.Selected))
	}
	for _, item := range first.Selected[0].Bundle.Evidence {
		input.PreviousEvidenceIDs = append(input.PreviousEvidenceIDs, item.ID)
	}
	second, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Selected) != 0 || len(second.Skipped) != 1 || second.Skipped[0].Gate.Reason != "no_new_exact_evidence" {
		t.Fatalf("second plan = %#v, want no-new-evidence skip", second)
	}
}

func TestPlanTargetedRoundsSkipsRuntimeOnlyFrontier(t *testing.T) {
	input := basicPlanningInput(t)
	input.Questions[0].Question = "This requires runtime observation only: which production backend is chosen?"
	result, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Gate.Reason != "runtime_only_frontier" {
		t.Fatalf("runtime-only plan = %#v", result)
	}
}

func TestPlanTargetedRoundsHardCapsTwoRounds(t *testing.T) {
	input := basicPlanningInput(t)
	input.Questions = []ProposedQuestion{
		{ID: "q1", Purpose: "backup", Question: "How does backup run?", CandidateIDs: []string{"candidate-backup"}},
		{ID: "q2", Purpose: "config", Question: "How does config load?", CandidateIDs: []string{"candidate-config"}},
		{ID: "q3", Purpose: "admin", Question: "How does admin control work?", CandidateIDs: []string{"candidate-admin"}},
	}
	input.Candidates = append(input.Candidates,
		FileCandidate{ID: "candidate-config", Path: "internal/config/config.go", Score: 90},
		FileCandidate{ID: "candidate-admin", Path: "internal/admin/admin.go", Score: 80},
	)
	input.Universe.AuthorizedPaths = append(input.Universe.AuthorizedPaths,
		"internal/config/config.go", "internal/admin/admin.go",
	)
	result, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 2 {
		t.Fatalf("selected rounds = %d, want hard cap 2", len(result.Selected))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Gate.Reason != "targeted_round_limit" {
		t.Fatalf("skipped rounds = %#v, want targeted round limit", result.Skipped)
	}
}

func TestExecuteRoundRejectsUnknownModelEvidenceID(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{response: []byte(`{
  "findings":[{"id":"invented","interpretation":"repository responsibility","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`)}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: planResult.Selected[0], Policy: input.Policy, Repository: RepositoryContext{
			Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default",
		}, RunsDir: t.TempDir(), Profile: "test", Model: "saved", Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != RoundRejected || len(round.ValidatedFindings) != 0 ||
		len(round.RejectedFindings) != 1 || round.RejectedFindings[0].Reason != "unknown_evidence_id" {
		t.Fatalf("round validation = %#v", round)
	}
}

func TestExecuteRoundReplaysIdenticalCachedInput(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := planResult.Selected[0].Bundle.Evidence[0].ID
	response, err := json.Marshal(researchResponse{Findings: []RawFinding{{
		ID: "finding-1", Interpretation: "backup is coordinated here",
		HypothesisAssessment: "supported", EvidenceIDs: []string{evidenceID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{response: response}
	runsDir := t.TempDir()
	repository := RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"}
	execute := func() ResearchRound {
		round, executeErr := ExecuteRound(context.Background(), ExecuteInput{
			Plan: planResult.Selected[0], Policy: input.Policy,
			Repository: repository,
			RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
		})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		return round
	}
	first := execute()
	second := execute() // A report-template/layout change is intentionally absent from the fingerprint.
	if first.Status != RoundCompleted || second.Status != RoundCached || !second.Cached {
		t.Fatalf("round statuses = %q/%q cached=%t", first.Status, second.Status, second.Cached)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one call plus cache replay", provider.calls)
	}
}

func TestFailedRoundPreservesGroundedLocalEvidence(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(input)
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{err: errors.New("saved provider failure")}
	round, callErr := ExecuteRound(context.Background(), ExecuteInput{
		Plan: planResult.Selected[0], Policy: input.Policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		Model:      "saved", Provider: provider,
	})
	if callErr == nil || round.Status != RoundFailed {
		t.Fatalf("failed round = %#v, err=%v", round, callErr)
	}
	state := NewState(input.Policy, RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"})
	ApplyRound(&state, planResult.Selected[0].Bundle, round)
	if len(state.Theory.GroundedFacts) == 0 {
		t.Fatal("failed targeted call discarded local evidence")
	}
}

func basicPlanningInput(t *testing.T) PlanningInput {
	t.Helper()
	repo := researchFixtureRepo(t)
	return PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "q-backup", Purpose: "backup architecture", Question: "How does backup run?",
			CandidateIDs: []string{"candidate-backup"},
		}},
		Candidates:           []FileCandidate{{ID: "candidate-backup", Path: "cmd/backup.go", Score: 100}},
		InitialProviderPaths: []string{"cmd/backup.go"},
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: []string{"cmd/backup.go", "internal/repository/repository.go"}},
		Policy:               DefaultPolicy(),
	}
}

func researchFixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeResearchFile(t, repo, "cmd/backup.go", "package cmd\n\nfunc runBackup() { executeBackup() }\n")
	writeResearchFile(t, repo, "internal/repository/repository.go", "package repository\n\nfunc Save() error { return nil }\n")
	writeResearchFile(t, repo, "internal/unrelated.go", "package internal\n\nfunc Unrelated() {}\n")
	writeResearchFile(t, repo, "internal/config/config.go", "package config\n\nfunc Load() {}\n")
	writeResearchFile(t, repo, "internal/admin/admin.go", "package admin\n\nfunc Apply() {}\n")
	return repo
}

func writeResearchFile(t *testing.T, repo, path, content string) {
	t.Helper()
	absolute := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func repoIdentity(t *testing.T) string {
	t.Helper()
	identity, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
