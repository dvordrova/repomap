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
)

func TestRunRejectsUnknownFileRefWithoutRepair(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/grounding\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		wire := orientationWireFromHTTPRequest(t, request)
		fileRef, evidenceRef := orientationWireFileRefs(t, wire, "main.go")
		response := orientationProviderResponse{
			ProjectGuess: "tiny command", Confidence: 0.7,
			FirstFilesToOpen: []orientationProviderFileToOpen{
				{FileRef: fileRef, Reason: "entrypoint"},
				{FileRef: "f9999", Reason: "invented path"},
			},
			CandidateFlows: []orientationProviderCandidateFlow{{
				Name: "Process startup", FlowType: "request", Trigger: "the executable starts",
				LikelyEntrypointRef: fileRef, LikelyFileRefs: []string{fileRef},
				WhyInteresting: "shows startup", EvidenceRefs: []string{evidenceRef}, Confidence: 0.7,
			}},
			Warnings: []string{},
		}
		orientation, err := json.Marshal(response)
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

	_, err := Run(context.Background(), Options{
		RepoPath:          repo,
		OutputJSON:        true,
		MaxLLMFiles:       10,
		MaxLLMEdges:       10,
		MaxLLMEntrypoints: 10,
		MaxLLMModules:     10,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON for orientation") {
		t.Fatalf("unknown file ref must fail closed, got %v", err)
	}
}
