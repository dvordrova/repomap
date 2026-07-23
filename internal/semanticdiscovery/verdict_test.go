package semanticdiscovery

import (
	"reflect"
	"testing"
)

func TestDeriveVerdict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input VerdictInput
		want  Verdict
	}{
		{
			name: "resolved claims and complete required coverage",
			input: VerdictInput{
				ClaimBases:        []ClaimBasis{ClaimDirect, ClaimCompositional},
				RequiredAspectIDs: []string{"entry", "result"},
				CoveredAspectIDs:  []string{"result", "entry"},
			},
			want: VerdictSupported,
		},
		{
			name: "resolved and unresolved claims",
			input: VerdictInput{
				ClaimBases: []ClaimBasis{ClaimDirect, ClaimUnresolved},
			},
			want: VerdictMixed,
		},
		{
			name: "resolved claim and retained missing evidence",
			input: VerdictInput{
				ClaimBases:              []ClaimBasis{ClaimDirect},
				RetainedMissingEvidence: 1,
			},
			want: VerdictMixed,
		},
		{
			name: "resolved claim and uncovered required aspect",
			input: VerdictInput{
				ClaimBases:        []ClaimBasis{ClaimInterpretive},
				RequiredAspectIDs: []string{"entry", "result"},
				CoveredAspectIDs:  []string{"entry"},
			},
			want: VerdictMixed,
		},
		{
			name: "only gaps",
			input: VerdictInput{
				ClaimBases:              []ClaimBasis{ClaimUnresolved},
				RetainedMissingEvidence: 1,
			},
			want: VerdictInsufficientEvidence,
		},
		{
			name: "central claim contradicted by validated evidence",
			input: VerdictInput{
				ClaimBases:               []ClaimBasis{ClaimDirect},
				CentralClaimContradicted: true,
			},
			want: VerdictUnsupported,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			before := append([]ClaimBasis(nil), test.input.ClaimBases...)
			if got := DeriveVerdict(test.input); got != test.want {
				t.Fatalf("DeriveVerdict() = %q, want %q", got, test.want)
			}
			if !reflect.DeepEqual(test.input.ClaimBases, before) {
				t.Fatal("DeriveVerdict() mutated its input")
			}
		})
	}
}

func TestReduceFanInArtifactDerivesVerdictAndRecordsMismatch(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	proposal := proposalByCandidateKind(t, &input, leaves, ArtifactDependencyUsage)
	proposal.Verdict = VerdictSupported

	reduced, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err != nil {
		t.Fatalf("ReduceFanInArtifact() error = %v", err)
	}
	got := proposalByCandidateKind(t, &reduced, leaves, ArtifactDependencyUsage)
	if got.Verdict != VerdictMixed {
		t.Fatalf("canonical verdict = %q, want %q", got.Verdict, VerdictMixed)
	}
	if report.DroppedArtifacts != 0 || len(report.VerdictDiagnostics) != 1 {
		t.Fatalf("reduction report = %#v", report)
	}
	diagnostic := report.VerdictDiagnostics[0]
	if diagnostic.Code != "model_verdict_mismatch" ||
		diagnostic.ModelVerdict != VerdictSupported ||
		diagnostic.DerivedVerdict != VerdictMixed ||
		!reflect.DeepEqual(diagnostic.Reasons, []VerdictReason{
			VerdictReasonUnresolvedClaimPresent,
			VerdictReasonMissingEvidenceRetained,
		}) {
		t.Fatalf("verdict diagnostic = %#v", diagnostic)
	}
}

func TestMaterializeArtifactsNeverTrustsRawVerdict(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatal(err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	proposalByCandidateKind(t, &input, leaves, ArtifactDependencyUsage).Verdict = VerdictSupported

	if err := ValidateFanInArtifact(bundle, leaves, input); err != nil {
		t.Fatalf("content validation rejected model mismatch: %v", err)
	}
	artifacts, err := MaterializeArtifacts(bundle, leaves, input)
	if err != nil {
		t.Fatalf("MaterializeArtifacts() error = %v", err)
	}
	if got := artifactByKind(t, artifacts, ArtifactDependencyUsage).Verdict; got != VerdictMixed {
		t.Fatalf("materialized verdict = %q, want locally derived %q", got, VerdictMixed)
	}
}

func TestReduceFanInArtifactRejectsClaimBeforeVerdictDerivation(t *testing.T) {
	bundle := semanticTestBundle()
	selected, err := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	leaves := semanticTestLeaves(t, bundle, selected)
	input := semanticTestFanIn(t, leaves)
	for index := range input.Artifacts {
		input.Artifacts[index].Claims[0].SupportIDs = []string{"unknown-fact-id"}
		input.Artifacts[index].Verdict = VerdictSupported
	}

	_, report, err := ReduceFanInArtifact(bundle, leaves, input)
	if err == nil {
		t.Fatal("ReduceFanInArtifact() accepted unknown support IDs")
	}
	if len(report.VerdictDiagnostics) != 0 {
		t.Fatalf("verdict was derived before claim rejection: %#v", report.VerdictDiagnostics)
	}
	if !hasFanInReductionIssue(report, "no_valid_artifacts") {
		t.Fatalf("reduction report = %#v", report)
	}
}
