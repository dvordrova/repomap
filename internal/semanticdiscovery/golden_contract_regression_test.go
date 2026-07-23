package semanticdiscovery

import (
	"strings"
	"testing"
)

func TestGoldenContractDistinguishesHarmlessProseFromRepositoryReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		wantError bool
	}{
		{
			name: "ordinary prose",
			text: "Sorting and pagination are applied to the listing",
		},
		{
			name:      "slash shorthand remains forbidden",
			text:      "Sorting uses sort/page controls",
			wantError: true,
		},
		{
			name:      "unknown repository path",
			text:      "The implementation lives in unknown/package/file.go",
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateModelText("golden prose", test.text, maxModelTextBytes, true)
			if test.wantError && (err == nil || !strings.Contains(err.Error(), "repository-bearing")) {
				t.Fatalf("validateModelText() error = %v, want repository-reference rejection", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateModelText() error = %v", err)
			}
		})
	}
}

func TestGoldenContractRequiresSequenceSupportForOrderingLanguage(t *testing.T) {
	t.Parallel()

	directCall := Fact{
		ID:           "fact-entry-call",
		Kind:         FactSourceSignal,
		Statement:    "The request handler calls the browse handler",
		SourceGroup:  "group-entry-call",
		Capabilities: []Capability{CapabilityEntry, CapabilityDirectCall},
		Scope:        FactScopeLocal,
	}
	withSequence := directCall
	withSequence.Capabilities = append(withSequence.Capabilities, CapabilitySequence)

	tests := []struct {
		name      string
		text      string
		fact      Fact
		wantError bool
	}{
		{
			name: "direct call without temporal language",
			text: "The request handler calls the browse handler",
			fact: directCall,
		},
		{
			name:      "temporal language without sequence support",
			text:      "The request handler then calls the browse handler",
			fact:      directCall,
			wantError: true,
		},
		{
			name: "temporal language with sequence support",
			text: "The request handler then calls the browse handler",
			fact: withSequence,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateSemanticSupport("golden claim", test.text, []Fact{test.fact})
			if test.wantError && (err == nil || !strings.Contains(err.Error(), "sequence-capable")) {
				t.Fatalf("validateSemanticSupport() error = %v, want sequence rejection", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateSemanticSupport() error = %v", err)
			}
		})
	}
}

func TestGoldenContractRequiresExplicitUnresolvedMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		wantError bool
	}{
		{
			name: "canonical evidence gap",
			text: "Evidence gap: The prepared directory listing does not establish its error path",
		},
		{
			name:      "confident unresolved prose",
			text:      "The prepared directory listing handles its error path",
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newGoldenMechanismFixture(t)
			claimIndex := len(fixture.artifact.Artifacts[0].Claims) - 1
			fixture.artifact.Artifacts[0].Claims[claimIndex].Text = test.text
			err := ValidateFanInArtifact(
				fixture.bundle,
				[]LeafResult{fixture.leaf},
				fixture.artifact,
			)
			if test.wantError && (err == nil || !strings.Contains(err.Error(), "explicit limitation")) {
				t.Fatalf("ValidateFanInArtifact() error = %v, want explicit limitation rejection", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("ValidateFanInArtifact() error = %v", err)
			}
		})
	}
}
