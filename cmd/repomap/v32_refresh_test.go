package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestV32RefreshResourceLimitErrorPreservesTerminalType(t *testing.T) {
	t.Parallel()

	limitErr := &deepseek.ResourceLimitError{
		Stage: "semantic_discovery", Kind: deepseek.ResourceLimitOutputTokens,
		Limit: 64_000, Observed: 64_000, ObservedKnown: true,
		FinishReason: "length",
	}
	for _, stage := range []string{"Study", "Paved Paths"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			got := v32RefreshResourceLimitError(stage, limitErr)
			var typed *deepseek.ResourceLimitError
			if !errors.As(got, &typed) || typed != limitErr {
				t.Fatalf("v32RefreshResourceLimitError() = %v, want wrapped original ResourceLimitError", got)
			}
			if !strings.Contains(got.Error(), "v32 refresh: "+stage+" resource limit") {
				t.Fatalf("v32RefreshResourceLimitError() = %q", got)
			}
		})
	}
	if got := v32RefreshResourceLimitError("Study", errors.New("invalid response")); got != nil {
		t.Fatalf("non-resource error became terminal resource error: %v", got)
	}
}

func TestRunV32RefreshResourceLimitStopsBeforePavedPathsAndPublication(t *testing.T) {
	clearLLMEnv(t)

	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "batch.go"), "package example\n\nfunc Core() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "batch.go")
	commitTestRepository(t, repository)
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	runDir := writePresentationLocalizationServeRun(
		t,
		t.TempDir(),
		"20260802-000000-v32-refresh-resource",
		state,
		presentationLocalizationCoherenceFixture(),
		"en",
	)

	bundle, directions := studyMapV32ReviewFixture(t)
	for index := range bundle.Anchors {
		bundle.Anchors[index].Capabilities = []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
		}
	}
	bundle.Documents = []studymap.Document{{
		ID: "doc-readme", Path: "README.md", Label: "README", Excerpt: "Fixture documentation.",
	}}
	bundle.AllowedPaths = append(bundle.AllowedPaths, "README.md")
	for index := range directions.Directions {
		directions.Directions[index].DocumentIDs = []string{"doc-readme"}
	}
	record, _, _, err := prepareStudyMapV32(
		context.Background(),
		t.TempDir(),
		bundle,
		&studyMapV32TypedRoundTripProvider{t: t, bundle: bundle, directions: directions},
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceProposal := studyMapV32ProposalFromRecord(record)
	proposalRaw, err := json.Marshal(sourceProposal)
	if err != nil {
		t.Fatal(err)
	}
	decodedSourceProposal, err := studymap.DecodeProposal(proposalRaw)
	if err != nil {
		t.Fatal(err)
	}
	sourceRecord, err := studymap.BuildRecord(bundle, decodedSourceProposal)
	if err != nil {
		t.Fatal(err)
	}
	bundleSHA, err := studymap.BundleHash(sourceRecord.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.RecordFile), sourceRecord); err != nil {
		t.Fatal(err)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), studyMapAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyMapPromptVersion,
		BundleSHA256: bundleSHA, ValidationState: "accepted", Response: proposalRaw,
	}); err != nil {
		t.Fatal(err)
	}
	savedRaw, err := os.ReadFile(filepath.Join(runDir, studymap.RecordFile))
	if err != nil {
		t.Fatal(err)
	}
	savedRecord, err := studymap.DecodeRecord(savedRaw)
	if err != nil {
		t.Fatal(err)
	}
	rebuiltRecord, err := studymap.BuildRecord(savedRecord.Bundle, decodedSourceProposal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rebuiltRecord, savedRecord) {
		t.Fatalf("v3.1 source fixture is not replayable\nsaved=%#v\nrebuilt=%#v", savedRecord, rebuiltRecord)
	}

	publicationBefore := make(map[string][]byte)
	for _, name := range []string{"report.json", "report.html", report.RunManifestFilename} {
		publicationBefore[name], err = os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatal(err)
		}
	}

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{\"version\":1"}}],"usage":{"completion_tokens":64000}}`)
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "64000")

	var stdout, stderr bytes.Buffer
	err = runV32RefreshCLI(
		[]string{"--run-dir", runDir, "--repo", state.Identity, "--reuse-study"},
		&stdout,
		&stderr,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("runV32RefreshCLI() error = %v, want ResourceLimitError\nstderr:\n%s", err, stderr.String())
	}
	if calls.Load() == 0 {
		t.Fatal("resource fixture was not called")
	}
	if stdout.Len() != 0 || strings.Contains(stderr.String(), "collecting bounded operational evidence") {
		t.Fatalf("terminal refresh continued: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, name := range []string{pavedpath.BundleFile, pavedpath.AttemptFile, pavedpath.RecordFile, pavedpath.StatusFile} {
		if _, statErr := os.Lstat(filepath.Join(runDir, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("terminal refresh created Paved Path artifact %s: %v", name, statErr)
		}
	}
	for name, before := range publicationBefore {
		after, readErr := os.ReadFile(filepath.Join(runDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("terminal refresh changed publication artifact %s", name)
		}
	}
}
