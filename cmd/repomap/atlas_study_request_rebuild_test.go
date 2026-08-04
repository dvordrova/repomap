package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
)

// TestAtlasStudyRequestRebuildRefusesOriginalCanonicalRun verifies the seam
// never mutates the original canonical run: the copied-run metadata rule from
// atlas-study-response-replay is enforced identically.
func TestAtlasStudyRequestRebuildRefusesOriginalCanonicalRun(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "canonical-run", "canonical-run")
	var stdout bytes.Buffer
	err := runAtlasStudyRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted the original canonical run")
	}
	if !strings.Contains(err.Error(), "refusing to mutate the original canonical run") {
		t.Fatalf("rebuild error does not explain the refusal: %v", err)
	}
}

// TestAtlasStudyRequestRebuildRefusesWhenResultOrStatusPersisted verifies the
// seam only runs for a run that has not yet resolved a provider exchange.
func TestAtlasStudyRequestRebuildRefusesWhenResultOrStatusPersisted(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	stale := []byte("stale result must not be read or changed\n")
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, atlasstudy.ResultArtifactFilename), stale)
	var stdout bytes.Buffer
	err := runAtlasStudyRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted a run with a persisted result")
	}
}

// TestAtlasStudyRequestRebuildRefusesExistingRequest verifies the exclusive
// write rule: the caller must delete the stale request from the copy first.
func TestAtlasStudyRequestRebuildRefusesExistingRequest(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	var stdout bytes.Buffer
	err := runAtlasStudyRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild accepted a run with an existing request artifact")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("rebuild error does not explain the exclusive-write refusal: %v", err)
	}
}

// TestAtlasStudyRequestRebuildFailsCleanlyWithoutFullRunData verifies that a
// run dir without the canonical repository artifacts (snapshot, repository
// Atlas) fails with an error and publishes nothing: no partial request may
// ever be left behind.
func TestAtlasStudyRequestRebuildFailsCleanlyWithoutFullRunData(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	if err := os.Remove(filepath.Join(fixture.runDir, atlasstudy.RequestArtifactFilename)); err != nil {
		t.Fatalf("remove fixture request: %v", err)
	}
	var stdout bytes.Buffer
	err := runAtlasStudyRequestRebuildCLI([]string{"--run-dir", fixture.runDir}, &stdout)
	if err == nil {
		t.Fatalf("rebuild succeeded without full run data")
	}
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename, atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename,
	} {
		if _, statErr := os.Lstat(filepath.Join(fixture.runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("rebuild failure left artifact %q behind", name)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("rebuild failure printed partial stdout: %q", stdout.String())
	}
}
