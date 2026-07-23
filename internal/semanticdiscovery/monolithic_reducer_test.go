package semanticdiscovery

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestReduceMonolithicArtifactDerivesVerdict(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatal(err)
	}
	input := semanticTestMonolithic(t, selected)
	proposal := proposalBySelectedKind(t, &input, selected, ArtifactDependencyUsage)
	proposal.Verdict = VerdictSupported

	reduced, report, err := ReduceMonolithicArtifact(bundle, selected, input)
	if err != nil {
		t.Fatalf("ReduceMonolithicArtifact() error = %v", err)
	}
	canonical := proposalBySelectedKind(t, &reduced, selected, ArtifactDependencyUsage)
	if canonical.Verdict != VerdictMixed {
		t.Fatalf("canonical verdict = %q, want %q", canonical.Verdict, VerdictMixed)
	}
	if len(report.VerdictDiagnostics) != 1 ||
		report.VerdictDiagnostics[0].ModelVerdict != VerdictSupported ||
		report.VerdictDiagnostics[0].DerivedVerdict != VerdictMixed {
		t.Fatalf("verdict diagnostics = %#v", report.VerdictDiagnostics)
	}
}

func TestReduceMonolithicArtifactDropsWholeInvalidProposal(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	input := semanticTestMonolithic(t, selected)
	invalid := proposalBySelectedKind(t, &input, selected, ArtifactDependencyUsage)
	invalid.Claims[0].SupportIDs = []string{testFactStatic}
	before := cloneJSON(t, input)

	reduced, report, err := ReduceMonolithicArtifact(bundle, selected, input)
	if err != nil {
		t.Fatalf("ReduceMonolithicArtifact() error = %v", err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("ReduceMonolithicArtifact() mutated its input")
	}
	if len(reduced.Artifacts) != 2 || report.KeptArtifacts != 2 || report.DroppedArtifacts != 1 {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if !hasFanInReductionIssue(report, "invalid_proposal") {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
	for _, proposal := range reduced.Artifacts {
		if proposal.CandidateID == invalid.CandidateID {
			t.Fatal("reducer retained claims from an invalid proposal")
		}
	}
	if err := ValidatePartialMonolithicArtifact(bundle, selected, reduced); err != nil {
		t.Fatalf("ValidatePartialMonolithicArtifact() error = %v", err)
	}
	if err := ValidateMonolithicArtifact(bundle, selected, reduced); err == nil ||
		!strings.Contains(err.Error(), "one artifact per candidate") {
		t.Fatalf("ValidateMonolithicArtifact(partial) error = %v", err)
	}

	artifacts, err := MaterializePartialMonolithicArtifacts(bundle, selected, reduced)
	if err != nil {
		t.Fatalf("MaterializePartialMonolithicArtifacts() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("materialized artifacts = %d, want 2", len(artifacts))
	}
	mechanism := artifactByKind(t, artifacts, ArtifactMechanism)
	if len(mechanism.RelatedArtifactIDs) != 0 {
		t.Fatalf(
			"related artifact ids = %v, want absent subset relation filtered",
			mechanism.RelatedArtifactIDs,
		)
	}
}

func TestReduceMonolithicArtifactDropsUnknownAndDuplicateProposals(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	input := semanticTestMonolithic(t, selected)
	duplicate := cloneJSON(t, input.Artifacts[0])
	unknown := cloneJSON(t, input.Artifacts[0])
	unknown.CandidateID = "semantic-candidate-unknown"
	input.Artifacts = append(input.Artifacts, duplicate, unknown)

	reduced, report, err := ReduceMonolithicArtifact(bundle, selected, input)
	if err != nil {
		t.Fatalf("ReduceMonolithicArtifact() error = %v", err)
	}
	if len(reduced.Artifacts) != 3 || report.KeptArtifacts != 3 || report.DroppedArtifacts != 2 {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if !hasFanInReductionIssue(report, "duplicate_candidate") ||
		!hasFanInReductionIssue(report, "unknown_candidate") {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
	if err := ValidateMonolithicArtifact(bundle, selected, reduced); err != nil {
		t.Fatalf("ValidateMonolithicArtifact(reduced full set) error = %v", err)
	}
}

func TestReduceMonolithicArtifactRejectsNoValidProposal(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	input := semanticTestMonolithic(t, selected)
	for index := range input.Artifacts {
		input.Artifacts[index].Title = "internal/unsupported.go"
	}

	reduced, report, err := ReduceMonolithicArtifact(bundle, selected, input)
	if err == nil || !strings.Contains(err.Error(), "no valid artifacts") {
		t.Fatalf("ReduceMonolithicArtifact() error = %v", err)
	}
	if len(reduced.Artifacts) != 0 || report.KeptArtifacts != 0 ||
		report.DroppedArtifacts != len(input.Artifacts) {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if !hasFanInReductionIssue(report, "no_valid_artifacts") {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
}

func TestReduceMonolithicArtifactBoundsDiagnostics(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	valid := semanticTestMonolithic(t, selected).Artifacts[0]
	input := FanInArtifact{
		Version:   FanInArtifactVersion,
		Artifacts: []ArtifactProposal{valid},
	}
	const invalidCount = MaxFanInReductionIssues + 7
	for index := 0; index < invalidCount; index++ {
		unknown := cloneJSON(t, valid)
		unknown.CandidateID = fmt.Sprintf("semantic-candidate-unknown-%02d", index)
		input.Artifacts = append(input.Artifacts, unknown)
	}

	reduced, report, err := ReduceMonolithicArtifact(bundle, selected, input)
	if err != nil {
		t.Fatalf("ReduceMonolithicArtifact() error = %v", err)
	}
	if len(reduced.Artifacts) != 1 || report.KeptArtifacts != 1 ||
		report.DroppedArtifacts != invalidCount {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if len(report.Issues) != MaxFanInReductionIssues {
		t.Fatalf("reduction issues = %d, want bounded %d", len(report.Issues), MaxFanInReductionIssues)
	}
	for _, issue := range report.Issues {
		if issue.Code != "unknown_candidate" {
			t.Fatalf("unexpected bounded issue = %#v", issue)
		}
	}
	if err := ValidatePartialMonolithicArtifact(bundle, selected, reduced); err != nil {
		t.Fatalf("ValidatePartialMonolithicArtifact() error = %v", err)
	}
}
