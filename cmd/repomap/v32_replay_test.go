package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestReplaySavedStudyMapV32FailsClosed(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), studyMapStatus{
		Version: studyMapStatusVersion,
		State:   "published",
	}); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(runDir, studymap.RecordFile)
	if err := os.WriteFile(recordPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := replaySavedStudyMapV32(runDir)
	if err == nil {
		t.Fatal("replay accepted missing split artifacts")
	}
	if _, statErr := os.Stat(recordPath); !os.IsNotExist(statErr) {
		t.Fatalf("stale Study Map survived failed replay: %v", statErr)
	}
	if status.State != "failed" || !status.LocalReplay || status.Selected != 0 {
		t.Fatalf("status = %#v", status)
	}
}

func TestReplaySavedPavedPathsV32FailsClosed(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.StatusFile), pavedPathStatus{
		Version: pavedPathStatusVersion,
		State:   "published",
		Paths:   3,
	}); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(runDir, pavedpath.RecordFile)
	if err := os.WriteFile(recordPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := replaySavedPavedPathsV32(runDir)
	if err == nil {
		t.Fatal("replay accepted missing operational artifacts")
	}
	if _, statErr := os.Stat(recordPath); !os.IsNotExist(statErr) {
		t.Fatalf("stale Paved Paths survived failed replay: %v", statErr)
	}
	if status.State != "failed" || !status.LocalReplay || status.Paths != 0 {
		t.Fatalf("status = %#v", status)
	}
}

func TestReplayPavedPathLandmarksForStaleStudyScope(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	recordPath := filepath.Join(runDir, pavedpath.RecordFile)
	if err := os.WriteFile(recordPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := replayPavedPathLandmarksForStaleStudyScope(
		runDir,
		pavedReplayTestBundle(),
		[]string{"study-current"},
		pavedPathStatus{Version: pavedPathStatusVersion, State: "published", Paths: 2},
		pavedPathAttempt{Version: pavedPathAttemptVersion, ValidationState: "accepted"},
		errors.New("v32 replay: Paved Path Study scope does not match the reviewed Study Map"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "landmarks" || !status.LocalReplay || status.Paths != 0 ||
		status.Landmarks == 0 || !strings.Contains(status.Failure, "Study scope") {
		t.Fatalf("status = %#v", status)
	}
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := pavedpath.DecodeRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 0 || len(record.Landmarks) == 0 {
		t.Fatalf("record = %#v", record)
	}
	var saved pavedPathStatus
	if err := readV32ReplayJSON(filepath.Join(runDir, pavedpath.StatusFile), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.State != status.State || saved.Landmarks != status.Landmarks || !saved.LocalReplay {
		t.Fatalf("saved status = %#v, returned = %#v", saved, status)
	}
}

func pavedReplayTestBundle() pavedpath.Bundle {
	return pavedpath.Bundle{
		Version:  pavedpath.BundleVersion,
		RepoName: "fixture",
		Evidence: []pavedpath.Evidence{
			{
				ID:        "ev-build",
				Role:      pavedpath.RoleBuildTarget,
				Path:      "Makefile",
				StartLine: 1,
				EndLine:   2,
				Label:     "build target",
				Excerpt:   []string{"build:", "\tgo build ./cmd/fixture"},
				Target:    "build",
				Commands: []pavedpath.Command{
					{
						Value:      "make build",
						Basis:      pavedpath.CommandStructural,
						SafeToCopy: true,
					},
				},
			},
		},
		AllowedPaths: []string{"Makefile"},
	}
}
