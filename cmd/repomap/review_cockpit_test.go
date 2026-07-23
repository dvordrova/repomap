package main

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestReviewCockpitArgs(t *testing.T) {
	t.Parallel()

	opts, err := parseReviewCockpitArgs([]string{
		"--chi-run", "chi", "--out", "review", "--caddy-run", "caddy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.CaddyRun != "caddy" || opts.ChiRun != "chi" || opts.OutDir != "review" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if _, err := parseReviewCockpitArgs([]string{"--out", "review"}); err == nil {
		t.Fatal("expected missing saved-run inputs to fail")
	}
}

func TestReviewCockpitTraceTargetsAreDistinctAndResolved(t *testing.T) {
	t.Parallel()

	artifact := semanticdiscovery.Artifact{
		Statements: []semanticdiscovery.Statement{
			{ID: "s1", Basis: semanticdiscovery.ClaimDirect},
			{ID: "s2", Basis: semanticdiscovery.ClaimCompositional},
			{ID: "s3", Basis: semanticdiscovery.ClaimDirect},
			{ID: "gap", Basis: semanticdiscovery.ClaimUnresolved},
		},
		Steps: []semanticdiscovery.Step{
			{ID: "one", StatementIDs: []string{"s1"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "a.go", Line: 10}}},
			{ID: "two", StatementIDs: []string{"s2"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "a.go", Line: 10}, {Path: "b.go", Line: 20}}},
			{ID: "three", StatementIDs: []string{"s3"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "c.go", Line: 30}}},
			{ID: "gap", StatementIDs: []string{"gap"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "gap.go", Line: 40}}},
		},
	}

	targets := deriveReviewTraceTargets(artifact)
	if len(targets) != 3 {
		t.Fatalf("got %d targets, want 3", len(targets))
	}
	want := []string{"a.go", "b.go", "c.go"}
	for index, target := range targets {
		if target.Target.Path != want[index] {
			t.Fatalf("target %d path = %q, want %q", index, target.Target.Path, want[index])
		}
		if strings.Contains(target.StepID, "gap") {
			t.Fatalf("unresolved gap became a trace target: %#v", target)
		}
	}
}

func TestReviewCockpitRejectedFrontierKeepsQuestionAndDiagnostics(t *testing.T) {
	t.Parallel()

	candidate := semanticdiscovery.OpportunityCandidate{
		ID: "candidate-1", QuestionAnswered: "Could this bounded behavior exist?",
		SupportIDs: []string{"fact-1"}, MissingInformation: []string{"runtime selection"},
	}
	facts := map[string]semanticdiscovery.Fact{
		"fact-1": {
			ID: "fact-1", Statement: "A deterministic anchor exists.",
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
			Evidence:     []semanticdiscovery.EvidenceRef{{Path: "source.go", Line: 7}},
		},
	}
	card := candidateFrontierCard(
		"repo", candidate, facts, semanticdiscovery.LeafResult{},
		semanticdiscovery.ArtifactProposal{}, []string{"invalid_proposal"}, true,
	)
	if card.Status != "rejected" {
		t.Fatalf("status = %q, want rejected", card.Status)
	}
	if card.Question != candidate.QuestionAnswered || card.SuggestedNextProbe != candidate.QuestionAnswered {
		t.Fatalf("candidate question was rewritten: %#v", card)
	}
	if len(card.Diagnostics) != 1 || card.Diagnostics[0] != "invalid_proposal" {
		t.Fatalf("diagnostics = %#v", card.Diagnostics)
	}
	if len(card.WhySuspected) != 1 || card.WhySuspected[0].Locations[0] != "source.go:7" {
		t.Fatalf("saved grounds were not retained: %#v", card.WhySuspected)
	}
}

func TestReviewCockpitHTMLContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"What are we testing?",
		"proposal-warning",
		"Presentation-only evidence lens",
		"Explore possibilities",
		"fetch('data.json'",
	} {
		if !strings.Contains(reviewCockpitHTML, token) {
			t.Fatalf("review HTML is missing %q", token)
		}
	}
}
