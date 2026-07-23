package main

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestRequireGoldenArtifactsChecksEveryExpectedArtifact(t *testing.T) {
	t.Parallel()

	first := semanticdiscovery.Artifact{
		Version:     semanticdiscovery.ArtifactVersion,
		ID:          "semantic-artifact-first",
		CandidateID: "semantic-candidate-first",
		Kind:        semanticdiscovery.ArtifactMechanism,
		Title:       "First mechanism",
		Summary:     "The first accepted mechanism remains exact.",
	}
	second := semanticdiscovery.Artifact{
		Version:     semanticdiscovery.ArtifactVersion,
		ID:          "semantic-artifact-second",
		CandidateID: "semantic-candidate-second",
		Kind:        semanticdiscovery.ArtifactMechanism,
		Title:       "Second mechanism",
		Summary:     "The second accepted mechanism remains exact.",
	}
	unrelated := semanticdiscovery.Artifact{
		Version:     semanticdiscovery.ArtifactVersion,
		ID:          "semantic-artifact-unrelated",
		CandidateID: "semantic-candidate-unrelated",
		Kind:        semanticdiscovery.ArtifactRepositoryPattern,
		Title:       "Unrelated artifact",
	}
	artifacts := []semanticdiscovery.Artifact{unrelated, second, first}
	wants := []semanticdiscovery.Artifact{first, second}

	if err := requireGoldenArtifacts(artifacts, wants); err != nil {
		t.Fatalf("requireGoldenArtifacts() error = %v", err)
	}
	if err := requireGoldenArtifact(artifacts, first); err != nil {
		t.Fatalf("requireGoldenArtifact() compatibility error = %v", err)
	}

	changed := append([]semanticdiscovery.Artifact(nil), artifacts...)
	changed[1].Summary = "changed"
	if err := requireGoldenArtifacts(changed, wants); err == nil ||
		!strings.Contains(err.Error(), "summary") {
		t.Fatalf("changed artifact error = %v", err)
	}

	if err := requireGoldenArtifacts(artifacts[:2], wants); err == nil ||
		!strings.Contains(err.Error(), "omitted") {
		t.Fatalf("omitted artifact error = %v", err)
	}

	duplicated := append(append([]semanticdiscovery.Artifact(nil), artifacts...), first)
	if err := requireGoldenArtifacts(duplicated, wants); err == nil ||
		!strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicated candidate error = %v", err)
	}

	if err := requireGoldenArtifacts(artifacts, []semanticdiscovery.Artifact{first, first}); err == nil ||
		!strings.Contains(err.Error(), "duplicate expected candidate") {
		t.Fatalf("duplicate expectation error = %v", err)
	}
}
