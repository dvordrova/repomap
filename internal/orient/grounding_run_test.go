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

func TestRunDropsUngroundedFirstFileWithoutFailing(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/grounding\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	orientation := `{
  "project_guess":"tiny command",
  "confidence":0.7,
  "high_level_map":[],
  "first_files_to_open":[
    {"path":"main.go","reason":"entrypoint"},
    {"path":"internal/deepseek/deepseek.go","reason":"invented path"}
  ],
  "candidate_flows":[{
    "name":"Process startup",
    "trigger":"the executable starts",
    "likely_entrypoint":"main.go",
    "likely_files":["main.go"],
    "why_interesting":"shows startup",
    "evidence":["main.go"],
    "confidence":0.7
  }],
  "important_domain_words":[],
  "questions_for_human":[],
  "unverified_paths":[],
  "warnings":[]
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": orientation},
			}},
		})
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	output, err := Run(context.Background(), Options{
		RepoPath:          repo,
		OutputJSON:        true,
		MaxLLMFiles:       10,
		MaxLLMEdges:       10,
		MaxLLMEntrypoints: 10,
		MaxLLMModules:     10,
	})
	if err != nil {
		t.Fatalf("Run() should retain grounded orientation: %v", err)
	}
	var report combinedReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if report.Orientation == nil || len(report.Orientation.FirstFilesToOpen) != 1 ||
		report.Orientation.FirstFilesToOpen[0].Path != "main.go" {
		t.Fatalf("first files = %#v", report.Orientation)
	}
	warnings := strings.Join(report.Orientation.Warnings, "\n")
	if !strings.Contains(warnings, `dropped first_files_to_open[1] outside allowed_paths: "internal/deepseek/deepseek.go"`) {
		t.Fatalf("warnings = %q", report.Orientation.Warnings)
	}
}
