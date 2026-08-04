package semanticdiscovery

import (
	"strings"
	"testing"
)

func TestReduceFanInArtifactMaterializesAndReplaysValidSubset(t *testing.T) {
	bundle := semanticTestBundle()
	opportunity := semanticTestOpportunity(bundle)
	selected, err := SelectOpportunities(bundle, opportunity, 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	invalid := proposalByCandidateKind(
		t,
		&input,
		leaves,
		ArtifactDependencyUsage,
	)
	invalid.Title = "internal/unsupported.go"

	reduced, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err != nil {
		t.Fatalf("ReduceFanInArtifact() error = %v", err)
	}
	if report.KeptArtifacts != 2 || report.DroppedArtifacts != 1 {
		t.Fatalf("reduction counts = %#v", report)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "invalid_proposal" {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
	if err := ValidatePartialFanInArtifact(bundle, leaves, reduced); err != nil {
		t.Fatalf("ValidatePartialFanInArtifact() error = %v", err)
	}
	if err := ValidateFanInArtifact(bundle, leaves, reduced); err == nil ||
		!strings.Contains(err.Error(), "exactly one artifact per candidate") {
		t.Fatalf("ValidateFanInArtifact(partial) error = %v", err)
	}

	artifacts, err := MaterializePartialArtifacts(bundle, leaves, reduced)
	if err != nil {
		t.Fatalf("MaterializePartialArtifacts() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("materialized artifacts = %d, want 2", len(artifacts))
	}
	mechanism := artifactByKind(t, artifacts, ArtifactMechanism)
	if len(mechanism.RelatedArtifactIDs) != 0 {
		t.Fatalf("related artifact ids = %v, want absent subset relation filtered", mechanism.RelatedArtifactIDs)
	}

	raw, err := EncodeRecord(bundle, opportunity, selected, leaves, reduced)
	if err != nil {
		t.Fatalf("EncodeRecord(partial fan-in) error = %v", err)
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		t.Fatalf("DecodeRecord() error = %v", err)
	}
	if record.Version != RecordVersion || len(record.SelectedCandidateIDs) != 3 ||
		len(record.Leaves) != 3 || len(record.FanIn.Artifacts) != 2 {
		t.Fatalf("partial record lost original context: %#v", record)
	}
	replayed, err := ReplayRecord(bundle, raw)
	if err != nil {
		t.Fatalf("ReplayRecord(partial fan-in) error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("replayed artifacts = %d, want 2", len(replayed))
	}
	stale := cloneJSON(t, bundle)
	stale.Facts[0].Statement += " changed"
	if _, err := ReplayRecord(stale, raw); err == nil || !strings.Contains(err.Error(), "bundle hash") {
		t.Fatalf("ReplayRecord(stale partial fan-in) error = %v", err)
	}
}

func TestReduceFanInArtifactDropsUnknownAndDuplicateProposals(t *testing.T) {
	bundle := semanticTestBundle()
	opportunity := semanticTestOpportunity(bundle)
	selected, err := SelectOpportunities(bundle, opportunity, 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	duplicate := cloneJSON(t, input.Artifacts[0])
	unknown := cloneJSON(t, input.Artifacts[0])
	unknown.CandidateID = "semantic-candidate-unknown"
	input.Artifacts = append(input.Artifacts, duplicate, unknown)

	reduced, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err != nil {
		t.Fatalf("ReduceFanInArtifact() error = %v", err)
	}
	if len(reduced.Artifacts) != 3 || report.KeptArtifacts != 3 || report.DroppedArtifacts != 2 {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if !hasFanInReductionIssue(report, "duplicate_candidate") ||
		!hasFanInReductionIssue(report, "unknown_candidate") {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
	if err := ValidateFanInArtifact(bundle, leaves, reduced); err != nil {
		t.Fatalf("ValidateFanInArtifact(reduced full set) error = %v", err)
	}
}

func TestReduceFanInArtifactRejectsNoValidProposal(t *testing.T) {
	bundle := semanticTestBundle()
	opportunity := semanticTestOpportunity(bundle)
	selected, err := SelectOpportunities(bundle, opportunity, 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	for index := range input.Artifacts {
		input.Artifacts[index].Title = "internal/unsupported.go"
	}

	reduced, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err == nil || !strings.Contains(err.Error(), "no valid artifacts") {
		t.Fatalf("ReduceFanInArtifact() error = %v", err)
	}
	if len(reduced.Artifacts) != 0 || report.KeptArtifacts != 0 ||
		report.DroppedArtifacts != len(input.Artifacts) {
		t.Fatalf("reduction result = %#v, report = %#v", reduced, report)
	}
	if !hasFanInReductionIssue(report, "no_valid_artifacts") {
		t.Fatalf("reduction issues = %#v", report.Issues)
	}
}

func TestReduceFanInArtifactReportsIndependentValidationReasons(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	input := cloneJSON(t, fixture.artifact)
	proposal := &input.Artifacts[0]
	proposal.Summary = "Directory listing uses sort/page controls and retains an evidence gap"
	proposal.Claims[1].Text = "The listing handler then reads directory entries from local storage"
	last := len(proposal.Claims) - 1
	proposal.Claims[last].Text = "The prepared directory listing's error path remains uninspected"

	reduced, report, err := ReduceFanInArtifact(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		input,
	)
	if err == nil || !strings.Contains(err.Error(), "no valid artifacts") {
		t.Fatalf("ReduceFanInArtifact() error = %v, want rejected proposal", err)
	}
	if len(reduced.Artifacts) != 0 || len(report.Issues) != 2 {
		t.Fatalf("reduction = %#v, report = %#v", reduced, report)
	}
	if len(report.VerdictDiagnostics) != 0 {
		t.Fatalf("verdict derived before semantic rejection: %#v", report.VerdictDiagnostics)
	}
	issue := report.Issues[0]
	if issue.Code != "invalid_proposal" {
		t.Fatalf("reduction issue = %#v", issue)
	}
	for _, code := range []string{
		FanInReasonUnknownRepositoryReference,
		FanInReasonUnsupportedSequence,
		FanInReasonLimitationNotExplicit,
	} {
		if !hasFanInReductionReason(issue, code) {
			t.Fatalf("reduction reasons = %#v, want %q", issue.Reasons, code)
		}
	}
	if got := fanInReductionReason(issue, FanInReasonUnsupportedSequence); got == nil ||
		got.ClaimIndex == nil || *got.ClaimIndex != 1 ||
		!equalOrderedStrings(got.SupportIDs, []string{goldenFactCollection}) {
		t.Fatalf("sequence diagnostic = %#v", got)
	}
	if got := fanInReductionReason(issue, FanInReasonLimitationNotExplicit); got == nil ||
		got.ClaimIndex == nil || *got.ClaimIndex != last {
		t.Fatalf("limitation diagnostic = %#v", got)
	}
}

func TestFanInDiagnosticHonorsExplicitMissingSequenceCapability(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatal(err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	proposal := proposalByCandidateKind(t, &input, leaves, ArtifactDependencyUsage)
	proposal.Title = "internal/unsupported.go"
	proposal.Claims[1].Text = "Package metadata loading lifecycle remains unresolved"

	_, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err != nil {
		t.Fatalf("ReduceFanInArtifact() error = %v", err)
	}
	var dependencyIssue *FanInReductionIssue
	for index := range report.Issues {
		if report.Issues[index].ArtifactIndex == 1 {
			dependencyIssue = &report.Issues[index]
			break
		}
	}
	if dependencyIssue == nil {
		t.Fatalf("dependency reduction issue missing: %#v", report)
	}
	if hasFanInReductionReason(*dependencyIssue, FanInReasonUnsupportedSequence) {
		t.Fatalf("explicit missing sequence capability reported as unsupported: %#v", dependencyIssue)
	}
}

func TestReduceFanInArtifactBoundsDetailedReasons(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	input := cloneJSON(t, fixture.artifact)
	proposal := &input.Artifacts[0]
	proposal.Summary = "directory/listing"
	for len(proposal.Claims) < maxClaimsPerArtifact {
		clone := cloneJSON(t, proposal.Claims[0])
		clone.Title = "step/" + string(rune('a'+len(proposal.Claims)))
		clone.Text = "The request handler calls browse/" + string(rune('a'+len(proposal.Claims)))
		proposal.Claims = append(proposal.Claims, clone)
	}
	for index := range proposal.Claims {
		proposal.Claims[index].Title = "claim/" + string(rune('a'+index))
		proposal.Claims[index].Text += " through/path"
	}

	_, report, err := ReduceFanInArtifact(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		input,
	)
	if err == nil {
		t.Fatal("ReduceFanInArtifact() unexpectedly accepted invalid proposal")
	}
	if len(report.Issues) == 0 || len(report.Issues[0].Reasons) != MaxFanInReductionReasons {
		t.Fatalf("bounded reasons = %#v", report.Issues)
	}
}

func hasFanInReductionIssue(report FanInReductionReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasFanInReductionReason(issue FanInReductionIssue, code string) bool {
	return fanInReductionReason(issue, code) != nil
}

func fanInReductionReason(issue FanInReductionIssue, code string) *FanInReductionReason {
	for index := range issue.Reasons {
		if issue.Reasons[index].Code == code {
			return &issue.Reasons[index]
		}
	}
	return nil
}
