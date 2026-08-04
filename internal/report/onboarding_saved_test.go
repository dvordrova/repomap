package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func TestDecodeRepositoryOnboardingEditorialPreservesPreferredArtifact(t *testing.T) {
	t.Parallel()

	editorial, err := DecodeRepositoryOnboardingEditorial([]byte(`{
  "version": 1,
  "preferred_artifact_id": " semantic-artifact-central ",
  "compressions": []
}`))
	if err != nil {
		t.Fatal(err)
	}
	if editorial.PreferredArtifactID != "semantic-artifact-central" {
		t.Fatalf("preferred artifact ID = %q", editorial.PreferredArtifactID)
	}
	if editorial.Compressions == nil || len(editorial.Compressions) != 0 {
		t.Fatalf("compressions = %#v, want an empty saved collection", editorial.Compressions)
	}
}

func TestDecodeRepositoryOnboardingEditorialRejectsUnboundedPreferredArtifact(t *testing.T) {
	t.Parallel()

	raw := `{"version":1,"preferred_artifact_id":"` + strings.Repeat("x", 257) + `","compressions":[]}`
	if _, err := DecodeRepositoryOnboardingEditorial([]byte(raw)); err == nil {
		t.Fatal("expected an out-of-bounds preferred artifact ID to be rejected")
	}
}

func TestApplyRepositoryOnboardingEditorialPrefersSavedStartHere(t *testing.T) {
	t.Parallel()

	first := onboardingMechanism(
		"artifact-a",
		"How does the router dispatch a request?",
		"The request enters the router, which finds a route and invokes an endpoint.",
		[]string{"mux.go", "tree.go"},
		"Receive request", "Find route", "Invoke endpoint",
	)
	preferred := first
	preferred.ArtifactID = "artifact-z"
	for _, mechanism := range []*UserMechanism{&first, &preferred} {
		for stepIndex := range mechanism.Steps {
			source := &mechanism.Steps[stepIndex].Sources[0]
			source.HighlightRanges = []SourceHighlight{{
				StartLine: source.StartLine,
				EndLine:   source.EndLine,
			}}
		}
	}
	data := &ReportData{
		RepoName:          "router",
		DocumentedPurpose: "A router dispatches HTTP requests to endpoints.",
		OpenablePaths:     []string{"mux.go", "tree.go"},
		Components: []Component{{
			Name: "Router", Role: componentmap.RoleDomain,
			ModelPurpose: "Dispatches requests.",
			AnchorGroups: []AnchorGroup{{Path: "mux.go"}, {Path: "tree.go"}},
		}},
		UserMechanisms: []UserMechanism{first, preferred},
	}

	applyRepositoryOnboardingEditorial(data, RepositoryOnboardingEditorial{
		Version:             RepositoryOnboardingVersion,
		PreferredArtifactID: "artifact-z",
		Compressions:        []NarrativeCompression{},
	})
	if data.StartHereArtifactID != "artifact-z" {
		t.Fatalf("Start Here = %q, want saved preference", data.StartHereArtifactID)
	}
}
