package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

// TestThemeStudyScoutRequestRebuildRefusesOriginalCanonicalRun verifies the
// seam never mutates the original canonical run: the copied-run metadata rule
// from theme-study-response-replay is enforced identically.
func TestThemeStudyScoutRequestRebuildRefusesOriginalCanonicalRun(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "canonical-run", "canonical-run")
	var stdout bytes.Buffer
	err := runThemeStudyScoutRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted the original canonical run")
	}
	if !strings.Contains(err.Error(), "refusing to mutate the original canonical run") {
		t.Fatalf("rebuild error does not explain the refusal: %v", err)
	}
}

// TestThemeStudyScoutRequestRebuildRefusesWhenResultOrStatusPersisted
// verifies the seam only runs for a run that has not yet resolved a provider
// exchange.
func TestThemeStudyScoutRequestRebuildRefusesWhenResultOrStatusPersisted(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	stale := []byte("stale result must not be read or changed\n")
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, themestudy.ScoutResultArtifactFilename), stale)
	var stdout bytes.Buffer
	err := runThemeStudyScoutRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted a run with a persisted result")
	}
}

// TestThemeStudyScoutRequestRebuildRefusesExistingRequest verifies the
// exclusive write rule: the caller must delete the stale request from the
// copy first.
func TestThemeStudyScoutRequestRebuildRefusesExistingRequest(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	var stdout bytes.Buffer
	err := runThemeStudyScoutRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted a run with an existing request artifact")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("rebuild error does not explain the exclusive-write refusal: %v", err)
	}
}

// TestThemeStudyScoutRequestRebuildFailsCleanlyWithoutFullRunData verifies
// that a run dir without the canonical repository artifacts fails with an
// error and publishes nothing: no partial request may ever be left behind.
func TestThemeStudyScoutRequestRebuildFailsCleanlyWithoutFullRunData(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	if err := os.Remove(filepath.Join(fixture.runDir, themestudy.ScoutRequestArtifactFilename)); err != nil {
		t.Fatalf("remove fixture request: %v", err)
	}
	var stdout bytes.Buffer
	err := runThemeStudyScoutRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild succeeded without full run data")
	}
	for _, name := range []string{
		themestudy.ScoutRequestArtifactFilename, themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename,
	} {
		if _, statErr := os.Lstat(filepath.Join(fixture.runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("rebuild failure left artifact %q behind", name)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("rebuild failure printed partial stdout: %q", stdout.String())
	}
}

// TestThemeStudyScoutRequestRebuildAuthorityModeFailsCleanlyWithoutManifestSeed
// verifies the --repo authority mode refuses to compile a request when the
// copied run carries no run_manifest.json authority seed, and publishes
// nothing: no partial request may ever be left behind.
func TestThemeStudyScoutRequestRebuildAuthorityModeFailsCleanlyWithoutManifestSeed(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	if err := os.Remove(filepath.Join(fixture.runDir, themestudy.ScoutRequestArtifactFilename)); err != nil {
		t.Fatalf("remove fixture request: %v", err)
	}
	var stdout bytes.Buffer
	err := runThemeStudyScoutRequestRebuildCLI(
		[]string{"--run-dir", fixture.runDir, "--repo", fixture.runDir},
		&stdout,
	)
	if err == nil {
		t.Fatalf("rebuild --repo accepted a run without a manifest authority seed")
	}
	if !strings.Contains(err.Error(), "authority seed") {
		t.Fatalf("rebuild --repo error does not explain the missing seed: %v", err)
	}
	for _, name := range []string{
		themestudy.ScoutRequestArtifactFilename, themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename,
	} {
		if _, statErr := os.Lstat(filepath.Join(fixture.runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("rebuild --repo failure left artifact %q behind", name)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("rebuild --repo failure printed partial stdout: %q", stdout.String())
	}
}
