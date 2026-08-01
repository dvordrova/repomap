package orient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
)

func TestRunPersistsRequestAndResponseOnValidationFailure(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/invalid-response\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		wire := orientationWireFromHTTPRequest(t, request)
		fileRef, _ := orientationWireFileRefs(t, wire, "main.go")
		orientation, err := json.Marshal(orientationProviderResponse{
			ProjectGuess: "tiny command", Confidence: 0.7,
			FirstFilesToOpen: []orientationProviderFileToOpen{{FileRef: fileRef, Reason: "entrypoint"}},
			CandidateFlows:   []orientationProviderCandidateFlow{}, Warnings: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": string(orientation)},
			}},
		})
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	debugDir := t.TempDir()
	runID := "invalid-orientation"
	_, err := Run(context.Background(), Options{
		RepoPath:          repo,
		OutputJSON:        true,
		RunID:             runID,
		DebugDir:          debugDir,
		DumpRedacted:      true,
		RequireArtifacts:  true,
		MaxLLMFiles:       10,
		MaxLLMEdges:       10,
		MaxLLMEntrypoints: 10,
		MaxLLMModules:     10,
	})
	if err == nil || !strings.Contains(err.Error(), "at least one candidate flow") {
		t.Fatalf("Run() error = %v", err)
	}
	runDir := filepath.Join(debugDir, runID)
	for _, name := range []string{
		"llm_request.redacted.json",
		"llm_response.raw.json",
		"orientation_validation.json",
		"error.txt",
	} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); statErr != nil {
			t.Errorf("missing %s after validation failure: %v", name, statErr)
		}
	}
	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.RequestAttempts) != 1 || metadata.RequestAttempts[0].State != "response_validation_failed" {
		t.Fatalf("request attempts = %#v", metadata.RequestAttempts)
	}
	semanticRecords := readOrientationSemanticRecords(t, runDir)
	if len(semanticRecords) != 1 ||
		semanticRecords[0].State != debugdump.SemanticStateRejected ||
		semanticRecords[0].ValidationCode != debugdump.SemanticValidationResponse ||
		semanticRecords[0].SemanticCalls != 1 ||
		semanticRecords[0].TransportAttempts != 1 ||
		semanticRecords[0].Response.Storage != "raw_content" {
		t.Fatalf("rejected orientation semantic exchange = %#v", semanticRecords)
	}
	validation, err := os.ReadFile(filepath.Join(runDir, "orientation_validation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(validation), `"stage": "response_validation_failed"`) {
		t.Fatalf("validation diagnostics = %s", validation)
	}
}
