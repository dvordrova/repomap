package main

// Chatto regression (commit 7054b5f): when the reducer rejects every theme
// (0 cards or reducer error), the whole theme artifact set is reset so the
// report reads as an honest neutral browse instead of hard-failing with
// "accepted Scout status requires all theme stage artifacts". This test
// reproduces the exact chatto 20260805-205935 artifact shape: scout and
// adjudication recorded as accepted_partial, no themes artifact.
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestResetThemeArtifactsOnReducerRejection(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	// Write a scout status and an adjudication status as if the run had
	// reached the reducer stage (chatto artifact shape: accepted_partial).
	for name, payload := range map[string]string{
		themestudy.ScoutStatusArtifactFilename:        `{"version":2,"state":"accepted_partial"}`,
		themestudy.AdjudicationStatusArtifactFilename: `{"version":2,"state":"accepted_partial"}`,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate the reducer rejecting every theme: the runtime calls
	// resetThemeStudyArtifacts, which must remove the whole set.
	if err := resetThemeStudyArtifacts(runDir); err != nil {
		t.Fatalf("resetThemeStudyArtifacts: %v", err)
	}
	for _, name := range themestudy.ThemeArtifactFilenames {
		if _, err := os.Stat(filepath.Join(runDir, name)); err == nil {
			t.Fatalf("artifact %s survived the reducer-rejection reset", name)
		}
	}
}

func TestResetThemeArtifactsLeavesOtherArtifactsIntact(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, themestudy.ScoutStatusArtifactFilename), []byte(`{"version":2,"state":"accepted"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "repository_atlas.v1.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetThemeStudyArtifacts(runDir); err != nil {
		t.Fatalf("resetThemeStudyArtifacts: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, themestudy.ScoutStatusArtifactFilename)); !os.IsNotExist(err) {
		t.Fatalf("theme artifact not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "repository_atlas.v1.json")); err != nil {
		t.Fatalf("non-theme artifact was removed: %v", err)
	}
}

func TestResetThemeArtifactsReportsRealErrors(t *testing.T) {
	t.Parallel()
	// Point at a path that cannot be opened as a directory root.
	if err := resetThemeStudyArtifacts(filepath.Join(t.TempDir(), "missing", "dir")); err == nil ||
		!strings.Contains(err.Error(), "open run directory") {
		t.Fatalf("resetThemeStudyArtifacts on missing dir = %v, want open-run-directory error", err)
	}
}
