package orient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
)

func TestRunDumpsInspectableRequestBeforeProviderFailure(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/trial\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "trial"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cmd", "trial", "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "cmd/trial/main.go")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported response_format"}`))
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "company-test-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "128")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	debugDir := t.TempDir()
	runID := "failed-provider"
	_, err := Run(context.Background(), Options{
		RepoPath:            repo,
		OutputJSON:          true,
		RunID:               runID,
		DebugDir:            debugDir,
		DumpLLM:             true,
		DumpRedacted:        true,
		MaxReadmeBytes:      1024,
		MaxReadmeLLMBytes:   512,
		MaxTreeLines:        50,
		MaxInterestingFiles: 50,
		MaxGoPkgs:           50,
		MaxGoEdges:          50,
		MaxLLMEntrypoints:   10,
		MaxLLMModules:       10,
		MaxLLMFiles:         50,
		MaxLLMEdges:         50,
	})
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("Run() error = %v", err)
	}

	runDir := filepath.Join(debugDir, runID)
	request, err := os.ReadFile(filepath.Join(runDir, "llm_request.redacted.json"))
	if err != nil {
		t.Fatalf("read request artifact: %v", err)
	}
	if !strings.Contains(string(request), `"model":"company-test-model"`) || !strings.Contains(string(request), `"json_object"`) {
		t.Fatalf("request artifact does not describe the attempted request: %s", request)
	}

	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata artifact: %v", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Model != "company-test-model" || metadata.Endpoint != server.URL ||
		metadata.PromptVersion != deepseek.OrientationPromptVersionJSON {
		t.Fatalf("metadata model/endpoint/prompt = %q / %q / %q", metadata.Model, metadata.Endpoint, metadata.PromptVersion)
	}
	if metadata.ProviderLatencyMillis == nil || *metadata.ProviderLatencyMillis < 0 {
		t.Fatalf("metadata provider latency = %v", metadata.ProviderLatencyMillis)
	}
}

func TestRunDumpLLMAbortsBeforeNetworkWhenDebugWriterFails(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/dump-failure\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "company-test-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	blockedDebugDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedDebugDir, []byte("file blocks mkdir"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		RepoPath:     repo,
		OutputJSON:   true,
		RunID:        "must-not-call-provider",
		DebugDir:     blockedDebugDir,
		DumpLLM:      true,
		DumpRedacted: true,
	})
	if err == nil || !strings.Contains(err.Error(), "create required debug writer") {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("provider requests = %d, want 0", requests)
	}
}

func TestRunOfflineRespectsZeroFlowExpansion(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/offline\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	output, err := Run(context.Background(), Options{
		RepoPath:   repo,
		Offline:    true,
		OutputJSON: true,
		FlowCount:  0,
	})
	if err != nil {
		t.Fatal(err)
	}
	var report combinedReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.ExplainedFlows) != 0 {
		t.Fatalf("explained flows = %d, want 0", len(report.ExplainedFlows))
	}
}

func runOrientGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
}
