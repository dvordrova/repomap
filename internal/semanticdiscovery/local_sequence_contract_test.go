package semanticdiscovery

import (
	"errors"
	"testing"
)

func TestValidateLocalSequenceClaims(t *testing.T) {
	t.Parallel()

	entry := Fact{
		ID: "fact-entry", Kind: FactSourceSignal,
		Statement:   "A browse branch contains a direct call to the browse handler",
		SourceGroup: "group-entry",
		Capabilities: []Capability{
			CapabilityStatic, CapabilityEntry, CapabilityBranch, CapabilityDirectCall,
		},
		Scope: FactScopeLocal,
	}
	sequence := Fact{
		ID: "fact-sequence", Kind: FactSourceSignal,
		Statement:   "Within one same-function browse branch, when the browse predicate succeeds, that branch directly returns a call to the browse handler",
		SourceGroup: "group-entry",
		Capabilities: []Capability{
			CapabilityStatic, CapabilityBranch, CapabilityDirectCall, CapabilitySequence,
		},
		Scope: FactScopeLocal,
	}
	bundle := Bundle{
		Version: BundleVersion, RepoName: "fixture",
		Facts: []Fact{entry, sequence},
	}

	tests := []struct {
		name     string
		claim    ProposedClaim
		wantCode string
	}{
		{
			name: "conditional branch claim cites separate sequence fact",
			claim: ProposedClaim{
				Title: "Browse branch",
				Text:  "Within this branch, when the browse predicate succeeds, the handler then calls the browse implementation",
				Basis: ClaimDirect,
				SupportIDs: []string{
					entry.ID, sequence.ID,
				},
			},
		},
		{
			name: "entry temporal prose lacks sequence fact",
			claim: ProposedClaim{
				Title: "Browse branch",
				Text:  "Within this branch, the handler then calls the browse implementation",
				Basis: ClaimDirect, SupportIDs: []string{entry.ID},
			},
			wantCode: LocalSequenceSupportMiss,
		},
		{
			name: "unused sequence fact without a temporal claim",
			claim: ProposedClaim{
				Title: "Browse branch",
				Text:  "The browse branch contains a direct call to the browse implementation",
				Basis: ClaimDirect, SupportIDs: []string{entry.ID},
			},
		},
		{
			name: "local fact is widened into runtime guarantee",
			claim: ProposedClaim{
				Title: "Browse branch",
				Text:  "Within this branch, the runtime always then calls the browse implementation",
				Basis: ClaimDirect, SupportIDs: []string{entry.ID, sequence.ID},
			},
			wantCode: LocalSequenceScopeWidened,
		},
		{
			name: "local fact loses conditional branch scope",
			claim: ProposedClaim{
				Title: "Browse call",
				Text:  "The handler then calls the browse implementation",
				Basis: ClaimDirect, SupportIDs: []string{entry.ID, sequence.ID},
			},
			wantCode: LocalSequenceScopeWidened,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			proposal := ArtifactProposal{
				CandidateID: "candidate-fixture",
				Verdict:     VerdictSupported,
				Title:       "Directory listing",
				Summary:     "Directory listing mechanism",
				Claims:      []ProposedClaim{test.claim},
			}
			err := ValidateLocalSequenceClaims(bundle, proposal, entry.ID, sequence.ID)
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateLocalSequenceClaims() error = %v", err)
				}
				return
			}
			var scopeErr *LocalSequenceClaimError
			if !errors.As(err, &scopeErr) || scopeErr.Code != test.wantCode {
				t.Fatalf("ValidateLocalSequenceClaims() error = %#v, want %q", err, test.wantCode)
			}
		})
	}
}
