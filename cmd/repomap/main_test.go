package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDefaultCompletesOneRequestOrientationJourney(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/friend-trial\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")

	orientation := map[string]any{
		"project_guess": "tiny Go command",
		"confidence":    0.9,
		"high_level_map": []any{map[string]any{
			"name":           "command",
			"evidence":       []string{"main.go"},
			"why_it_matters": "it owns process startup",
		}},
		"first_files_to_open": []any{map[string]any{
			"path":   "main.go",
			"reason": "process entrypoint",
		}},
		"candidate_flows": []any{map[string]any{
			"name":              "Process startup",
			"trigger":           "the executable starts",
			"likely_entrypoint": "main.go",
			"likely_files":      []string{"main.go"},
			"why_interesting":   "shows the complete behavior of this tiny command",
			"evidence":          []string{"main.go"},
			"confidence":        0.9,
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []string{"Which behavior should we inspect next?"},
		"unverified_paths":       []any{},
		"warnings":               []string{},
	}
	orientationJSON, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		envelope, marshalErr := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": string(orientationJSON),
				},
			}},
		})
		if marshalErr != nil {
			t.Errorf("marshal envelope: %v", marshalErr)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(envelope)
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "deepseek-v4-flash")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	var preview bytes.Buffer
	previewOpened := false
	err = runDefaultWithDeps(repo, []string{"--preview-request"}, defaultRunDeps{
		stdout: &preview,
		stderr: io.Discard,
		openReport: func(string) error {
			previewOpened = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("preview request: %v", err)
	}
	if requestCount != 0 {
		t.Fatalf("preview made %d provider request(s), want 0", requestCount)
	}
	if previewOpened {
		t.Fatal("preview opened a report")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var openedReport string
	debugDir := t.TempDir()
	err = runDefaultWithDeps(repo, []string{"--debug-dir", debugDir}, defaultRunDeps{
		stdout: &stdout,
		stderr: &stderr,
		openReport: func(path string) error {
			openedReport = path
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("provider request count = %d, want 1", requestCount)
	}
	if !bytes.Equal(preview.Bytes(), requestBody) {
		t.Fatalf("preview differs from outbound request\npreview: %s\nrequest: %s", preview.Bytes(), requestBody)
	}
	for _, want := range []string{`"model":"deepseek-v4-flash"`, `"response_format":{"type":"json_object"}`} {
		if !bytes.Contains(requestBody, []byte(want)) {
			t.Fatalf("request body missing %q: %s", want, requestBody)
		}
	}
	for _, want := range []string{"Project: tiny Go command", "Candidate directions:", "Process startup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	for _, want := range []string{
		"repomap: scanning ",
		"repomap: compact local context ",
		fmt.Sprintf("repomap: asking deepseek-v4-flash with %d-byte request", len(requestBody)),
		"repomap: validated 1 candidate direction(s)",
		"Report: ",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
	if openedReport == "" {
		t.Fatal("generated report was not opened")
	}
	if _, err := os.Stat(openedReport); err != nil {
		t.Fatalf("opened report %q: %v", openedReport, err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(filepath.Dir(openedReport), "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !bytes.Contains(reportJSON, []byte("Process startup")) {
		t.Fatalf("report.json does not retain candidate direction: %s", reportJSON)
	}
}

func TestRunDefaultNoOpenSuppressesBrowser(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/offline\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")

	opened := false
	if err := runDefaultWithDeps(repo, []string{"--offline", "--no-open", "--debug-dir", t.TempDir()}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		openReport: func(string) error {
			opened = true
			return nil
		},
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if opened {
		t.Fatal("browser opener was called with --no-open")
	}
}

func TestRunDefaultAcceptsRepositoryAfterFlags(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/flags-first\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")

	var stdout bytes.Buffer
	if err := runDefaultWithDeps(".", []string{"--offline", "--no-debug", repo}, defaultRunDeps{
		stdout: &stdout,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "offline mode") {
		t.Fatalf("stdout does not describe offline run:\n%s", stdout.String())
	}
}

func TestReportOpenCommand(t *testing.T) {
	tests := []struct {
		goos     string
		wantName string
	}{
		{goos: "darwin", wantName: "open"},
		{goos: "linux", wantName: "xdg-open"},
		{goos: "windows", wantName: "rundll32"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			name, args, err := reportOpenCommand(test.goos, "/tmp/report.html")
			if err != nil {
				t.Fatal(err)
			}
			if name != test.wantName {
				t.Fatalf("command = %q, want %q", name, test.wantName)
			}
			if len(args) == 0 || args[len(args)-1] != "/tmp/report.html" {
				t.Fatalf("args = %v", args)
			}
		})
	}
	if _, _, err := reportOpenCommand("plan9", "/tmp/report.html"); err == nil {
		t.Fatal("unsupported OS did not return an error")
	}
}

func TestRepoRunLabelResolvesCurrentDirectory(t *testing.T) {
	label := repoRunLabel(".")
	if label == "" || label == "." || label == string(filepath.Separator) {
		t.Fatalf("repoRunLabel(.) = %q", label)
	}
}

func TestRunDoctorReportsConfigWithoutSecret(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("REPOMAP_LLM_ENDPOINT", "https://llm.company.example/v1/chat/completions")
	t.Setenv("REPOMAP_LLM_MODEL", "company-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "must-not-be-printed")

	var stdout bytes.Buffer
	if err := runDoctor([]string{"llm"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"LLM configuration OK",
		"endpoint: https://llm.company.example/v1/chat/completions",
		"model: company-model",
		"auth: bearer",
		"network_check: skipped",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "must-not-be-printed") {
		t.Fatal("doctor output contains API key")
	}
}

func TestRunDoctorChecksNoAuthProviderWithoutRepositoryContent(t *testing.T) {
	clearLLMEnv(t)
	var authorization string
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requestBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"status\":\"ok\"}"}}]}`)
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	var stdout bytes.Buffer
	if err := runDoctor([]string{"llm", "--check"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if authorization != "" {
		t.Fatalf("Authorization = %q, want none", authorization)
	}
	if strings.Contains(requestBody, "repo_name") || strings.Contains(requestBody, "allowed_paths") {
		t.Fatalf("doctor sent repository content: %s", requestBody)
	}
	if !strings.Contains(stdout.String(), "network_check: passed") {
		t.Fatalf("doctor output = %q", stdout.String())
	}
}

func TestRunDoctorRejectsUnknownTarget(t *testing.T) {
	t.Parallel()

	err := runDoctor([]string{"database"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("runDoctor() error = %v", err)
	}
}

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"REPOMAP_LLM_ENDPOINT",
		"REPOMAP_LLM_MODEL",
		"REPOMAP_LLM_API_KEY",
		"REPOMAP_LLM_MAX_TOKENS",
		"REPOMAP_LLM_TIMEOUT",
		"REPOMAP_LLM_AUTH",
		"DEEPSEEK_ENDPOINT",
		"DEEPSEEK_MODEL",
		"DEEPSEEK_API_KEY",
		"DEEPSEEK_MAX_TOKENS",
		"DEEPSEEK_TIMEOUT",
		"DEEPSEEK_AUTH",
	} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
}
