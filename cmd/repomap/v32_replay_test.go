package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
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

func TestLoadBoundStudyMapReviewsRevalidatesLegacyV1Attempt(t *testing.T) {
	t.Parallel()

	bundle, directions := studyReviewCacheIndependentFixture(t)
	directions.Directions = directions.Directions[:1]
	direction := directions.Directions[0]
	reviewBundle, err := studymap.BuildReviewBundle(bundle, direction)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(studyReviewCacheProposal(reviewBundle))
	if err != nil {
		t.Fatal(err)
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runDir, studyMapReviewAttemptsDir), 0o700); err != nil {
		t.Fatal(err)
	}
	attempt := studyMapReviewAttempt{
		Version: 1, PromptVersion: legacyReadingPackReviewPromptVersion,
		BundleSHA256: bundleSHA, DirectionID: direction.DirectionID,
		ValidationState: "accepted", Bundle: &reviewBundle, Response: response,
	}
	if err := writeGoldenJSON(
		filepath.Join(runDir, studyMapReviewAttemptsDir, direction.DirectionID+".json"),
		attempt,
	); err != nil {
		t.Fatal(err)
	}
	reviews, summaries, issues, err := loadBoundStudyMapReviews(
		runDir, bundle, directions, bundleSHA,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || len(summaries) != 1 || len(issues) != 0 {
		t.Fatalf("legacy replay reviews/summaries/issues = %d/%d/%d", len(reviews), len(summaries), len(issues))
	}
}

func TestLoadBoundStudyMapInputsRevalidatesTypedBriefShapeAttempt(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	bundle.Documents = []studymap.Document{{
		ID: "doc-shape-invalid", Path: "README.md", Label: "README",
		Excerpt: "Repository documentation.",
	}}
	bundle.AllowedPaths = append(bundle.AllowedPaths, "README.md")
	catalog, prompt, err := buildStudyMapBriefShapeStage(bundle)
	if err != nil {
		t.Fatal(err)
	}
	typedBrief, err := studyMapTypedBriefShapeResponse(
		t, prompt.User, bundle, "document_in_shape",
	)
	if err != nil {
		t.Fatal(err)
	}
	brief, diagnostics, err := studymap.DecodeAndResolveBriefShapeProposal(typedBrief, catalog)
	if err != nil {
		t.Fatal(err)
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	providerDirections := directions
	providerDirections.Directions = append([]studymap.DirectionCandidate(nil), directions.Directions...)
	for index := range providerDirections.Directions {
		providerDirections.Directions[index].DirectionID = ""
	}
	directionResponse, err := json.Marshal(providerDirections)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeAttempt), studyMapV32StageAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyBriefPromptVersion,
		BundleSHA256: bundleSHA, ValidationState: "accepted",
		BriefDiagnostics: &diagnostics, Response: typedBrief,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), studyMapV32StageAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyCandidatesPromptVersion,
		BundleSHA256: bundleSHA, ValidationState: "accepted", Response: directionResponse,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeFile), brief); err != nil {
		t.Fatal(err)
	}
	if err := writeNormalizedDirectionProposal(
		filepath.Join(runDir, studyMapDirectionsFile), directions,
	); err != nil {
		t.Fatal(err)
	}

	loadedBrief, loadedDirections, err := loadBoundStudyMapInputs(runDir, bundle, bundleSHA)
	if err != nil {
		t.Fatal(err)
	}
	if !equalV32Projection(loadedBrief, brief) || !equalV32Projection(loadedDirections, directions) {
		t.Fatalf("replayed Brief/Directions changed: %#v / %#v", loadedBrief, loadedDirections)
	}

	diagnostics.ShapeRejected++
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeAttempt), studyMapV32StageAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyBriefPromptVersion,
		BundleSHA256: bundleSHA, ValidationState: "accepted",
		BriefDiagnostics: &diagnostics, Response: typedBrief,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadBoundStudyMapInputs(runDir, bundle, bundleSHA); err == nil ||
		!strings.Contains(err.Error(), "Brief diagnostics do not match") {
		t.Fatalf("tampered Brief diagnostics error = %v", err)
	}
}

func TestLoadBoundStudyMapInputsRejectsMonolithicSourceAttempt(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapSourceAttemptFile), studyMapAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyMapPromptVersion,
		BundleSHA256: bundleSHA, ValidationState: "accepted",
		Response: json.RawMessage(`{"version":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadBoundStudyMapInputs(runDir, bundle, bundleSHA); err == nil ||
		!strings.Contains(err.Error(), "typed split Study input attempts are required") {
		t.Fatalf("monolithic source replay error = %v", err)
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
