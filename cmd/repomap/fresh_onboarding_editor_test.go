package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestFreshOnboardingEditorPersistsPreferredBaselineWithoutModelWork(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeFile(t, filepath.Join(runDir, "snapshot.json"), `{"repo_name":"fixture"}`)

	metrics, attempted, err := editFreshRepositoryOnboarding(
		context.Background(),
		runDir,
		nil,
		"semantic-artifact-central",
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempted {
		t.Fatal("onboarding editor attempted a model call without accepted mechanisms")
	}
	if metrics.ProviderCall {
		t.Fatalf("baseline metrics = %#v", metrics)
	}

	raw, err := os.ReadFile(filepath.Join(runDir, report.RepositoryOnboardingFile))
	if err != nil {
		t.Fatal(err)
	}
	editorial, err := report.DecodeRepositoryOnboardingEditorial(raw)
	if err != nil {
		t.Fatal(err)
	}
	if editorial.PreferredArtifactID != "semantic-artifact-central" {
		t.Fatalf("preferred artifact ID = %q", editorial.PreferredArtifactID)
	}
	if editorial.Compressions == nil || len(editorial.Compressions) != 0 {
		t.Fatalf("baseline compressions = %#v", editorial.Compressions)
	}
}
