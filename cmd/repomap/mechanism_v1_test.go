package main

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestRequirePublishedUserMechanismAllowsSourceBackedSecondary(t *testing.T) {
	t.Parallel()

	const artifactID = "semantic-artifact-secondary"
	data := &report.ReportData{
		SemanticArtifacts: []semanticdiscovery.Artifact{{
			ID:          artifactID,
			CandidateID: "semantic-candidate-secondary",
		}},
		UserMechanisms: []report.UserMechanism{{
			ArtifactID: artifactID,
			Role:       report.OnboardingRoleSecondaryBehavior,
			Steps: []report.UserMechanismStep{
				testPublishedMechanismStep("first.go", "evidence-first"),
				testPublishedMechanismStep("second.go", "evidence-second"),
			},
		}},
		SemanticSearch: &report.SemanticSearchIndex{
			Items: []report.SemanticSearchItem{{
				Target: report.SemanticSearchTarget{
					Kind:       report.SemanticSearchTargetArtifact,
					ArtifactID: artifactID,
				},
			}},
		},
	}

	published, err := requirePublishedUserMechanism(data, artifactID, false)
	if err != nil {
		t.Fatalf("secondary mechanism publication error = %v", err)
	}
	if published.ID != artifactID {
		t.Fatalf("published artifact = %q, want %q", published.ID, artifactID)
	}

	if _, err := requirePublishedUserMechanism(data, artifactID, true); err == nil ||
		!strings.Contains(err.Error(), "not Start Here") {
		t.Fatalf("primary requirement error = %v", err)
	}
	data.StartHereArtifactID = artifactID
	if _, err := requirePublishedUserMechanism(data, artifactID, true); err != nil {
		t.Fatalf("primary publication error = %v", err)
	}

	data.UserMechanisms[0].Steps[1].Sources = nil
	if _, err := requirePublishedUserMechanism(data, artifactID, false); err == nil ||
		!strings.Contains(err.Error(), "source-backed") {
		t.Fatalf("missing source error = %v", err)
	}
}

func testPublishedMechanismStep(path, evidenceID string) report.UserMechanismStep {
	return report.UserMechanismStep{
		Locations: []report.UserCodeLocation{{Path: path, Line: 1}},
		Sources: []report.SourceSnippet{{
			Path:               path,
			StartLine:          1,
			EndLine:            1,
			Content:            "func example() {}",
			Lines:              []report.SourceSnippetLine{{Line: 1, Text: "func example() {}"}},
			RelatedEvidenceIDs: []string{evidenceID},
		}},
	}
}
