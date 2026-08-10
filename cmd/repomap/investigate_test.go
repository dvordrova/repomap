package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/tasklens"
)

type offlineTaskProviderStub struct{}

func (offlineTaskProviderStub) TaskInvestigationPromptJSON(tasklens.Bundle) ([]byte, error) {
	return []byte(`{"request":"bounded-task-fixture"}`), nil
}

func (offlineTaskProviderStub) InvestigateTaskMeasured(context.Context, tasklens.Bundle) (modelProviderResult, error) {
	panic("offline investigation must not call the provider")
}

type failedTaskProviderStub struct {
	calls *int
}

func (failedTaskProviderStub) TaskInvestigationPromptJSON(tasklens.Bundle) ([]byte, error) {
	return []byte(`{"request":"bounded-task-fixture"}`), nil
}

func (provider failedTaskProviderStub) InvestigateTaskMeasured(
	context.Context,
	tasklens.Bundle,
) (modelProviderResult, error) {
	(*provider.calls)++
	return modelProviderResult{Attempts: 1}, errors.New("intentional provider failure")
}

func TestRunInvestigateOfflineWritesOnlyTaskPathAndAuthorizedReport(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "issue-labelled-checkout")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	investigateWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/investigatefixture\n\ngo 1.24\n")
	investigateWriteFile(t, filepath.Join(repo, "config.go"), `package fixture

type Config struct { Enabled bool }
type Engine struct { Config Config }

func CopyConfig(engine *Engine, config Config) {
	engine.Config.Enabled = config.Enabled
}

func Enabled(engine *Engine) bool { return engine.Config.Enabled }
`)
	investigateWriteFile(t, filepath.Join(repo, "config_test.go"), `package fixture

import "testing"

func TestCopyConfig(t *testing.T) {
	engine := &Engine{}
	CopyConfig(engine, Config{Enabled: true})
	if !Enabled(engine) { t.Fatal("not copied") }
}
`)
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "config.go", "config_test.go")
	commitTestRepository(t, repo)
	taskFile := filepath.Join(t.TempDir(), "task.md")
	investigateWriteFile(t, taskFile, "## Prompt-safe task\n\nThe Enabled configuration is ignored. Locate CopyConfig and the nearest verification test.\n")
	debugDir := filepath.Join(t.TempDir(), "runs")
	t.Setenv("REPOMAP_LLM_MODEL", "offline-must-ignore-provider-environment")
	var stdout, stderr bytes.Buffer
	err := runInvestigate([]string{
		repo, "--task-file", taskFile, "--debug-dir", debugDir,
		"--offline", "--no-open", "--no-serve", "--strict-snapshot",
	}, investigateDependencies{
		ctx: context.Background(), stdout: &stdout, stderr: &stderr,
		newProvider: func(bool) (taskInvestigationProvider, deepseek.EffectiveConfig, error) {
			t.Fatal("offline investigation unexpectedly configured a provider")
			return nil, deepseek.EffectiveConfig{}, nil
		},
		openReport: func(string) error { t.Fatal("report unexpectedly opened"); return nil },
	})
	if err != nil {
		t.Fatalf("runInvestigate() error = %v\nstderr:\n%s", err, stderr.String())
	}
	runDir := strings.TrimSpace(stdout.String())
	if runDir == "" {
		t.Fatal("run directory was not printed")
	}
	var metadata struct {
		Model                string `json:"model"`
		Endpoint             string `json:"endpoint"`
		ProviderRequestCount int    `json:"provider_request_count"`
	}
	if err := json.Unmarshal(investigateReadFile(t, filepath.Join(runDir, "metadata.json")), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Model != "deepseek-v4-flash" ||
		metadata.Endpoint != "https://api.deepseek.com/chat/completions" ||
		metadata.ProviderRequestCount != 0 {
		t.Fatalf("offline metadata provider identity = %#v", metadata)
	}
	for _, name := range []string{
		tasklens.BundleFile, tasklens.AttemptFile, tasklens.PackFile,
		tasklens.StatusFile, tasklens.TraceJSONFile, tasklens.TraceMarkdownFile,
		"snapshot.json", "report.json", "report.html", report.RunManifestFilename,
	} {
		if info, err := os.Stat(filepath.Join(runDir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("artifact %s: %v", name, err)
		}
	}
	for _, absent := range []string{"orientation_report.json", "llm_bundle.json", "architecture_synthesis.json"} {
		if _, err := os.Stat(filepath.Join(runDir, absent)); !os.IsNotExist(err) {
			t.Fatalf("generic stage artifact %s unexpectedly exists", absent)
		}
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.TaskInvestigation == nil || data.TaskInvestigation.State != "accepted_local_complete" ||
		data.TaskInvestigation.Provider.Calls != 0 || !data.TaskInvestigation.Sufficient {
		t.Fatalf("task workspace = %#v", data.TaskInvestigation)
	}
	if data.RepoName != "example.com/investigatefixture" {
		t.Fatalf("repository name = %q", data.RepoName)
	}
	if strings.Contains(string(investigateReadFile(t, filepath.Join(runDir, "report.json"))), "anchor-") ||
		strings.Contains(string(investigateReadFile(t, filepath.Join(runDir, "report.json"))), "evidence-") {
		t.Fatal("user-facing report exposed opaque Task Lens IDs")
	}
	var status tasklens.Status
	if err := json.Unmarshal(investigateReadFile(t, filepath.Join(runDir, tasklens.StatusFile)), &status); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitOutputForTest(t, repo, "rev-parse", "HEAD"))
	if status.CapturedRevision != head || status.Provider.Calls != 0 || !status.Sufficient ||
		!status.CheapExit.Eligible || !containsString(status.StagesSkipped, "generic_orientation") {
		t.Fatalf("status = %#v", status)
	}
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.CurrentRunManifestVersion != 18 {
		t.Fatalf("current manifest version = %d, want 18", report.CurrentRunManifestVersion)
	}
	if manifest.Version != report.CurrentRunManifestVersion {
		t.Fatalf("manifest version = %d", manifest.Version)
	}
	artifactHashes := map[string]string{
		tasklens.BundleFile:        manifest.MaterialInputs.TaskBundleSHA256,
		tasklens.AttemptFile:       manifest.MaterialInputs.TaskAttemptSHA256,
		tasklens.PackFile:          manifest.MaterialInputs.TaskPackSHA256,
		tasklens.StatusFile:        manifest.MaterialInputs.TaskStatusSHA256,
		tasklens.TraceJSONFile:     manifest.MaterialInputs.TaskRetrievalTraceSHA256,
		tasklens.TraceMarkdownFile: manifest.MaterialInputs.TaskRetrievalTraceMarkdownSHA256,
	}
	for name, want := range artifactHashes {
		if got := tasklens.SHA256(investigateReadFile(t, filepath.Join(runDir, name))); got != want {
			t.Fatalf("manifest hash for %s = %q, want %q", name, want, got)
		}
	}
	var bundle tasklens.Bundle
	if err := json.Unmarshal(investigateReadFile(t, filepath.Join(runDir, tasklens.BundleFile)), &bundle); err != nil {
		t.Fatal(err)
	}
	snapshotRaw := investigateReadFile(t, filepath.Join(runDir, "snapshot.json"))
	var savedSnapshot snapshot.Snapshot
	if err := json.Unmarshal(snapshotRaw, &savedSnapshot); err != nil {
		t.Fatalf("decode bounded task snapshot: %v", err)
	}
	wantSnapshotPaths := []string{"config.go", "config_test.go", "go.mod"}
	if !slices.Equal(savedSnapshot.FileTree, wantSnapshotPaths) ||
		!slices.Equal(savedSnapshot.FileTree, bundle.AllowedPaths) {
		t.Fatalf("bounded task snapshot paths = %v, bundle paths = %v", savedSnapshot.FileTree, bundle.AllowedPaths)
	}
	if slices.Contains(savedSnapshot.FileTree, filepath.Base(taskFile)) {
		t.Fatalf("bounded task snapshot included external task file: %v", savedSnapshot.FileTree)
	}
	if savedSnapshot.RepoName != bundle.Repository.Identity ||
		savedSnapshot.FilesConsidered != len(wantSnapshotPaths) ||
		savedSnapshot.FilesSkipped != bundle.Metrics.TrackedFiles-len(wantSnapshotPaths) {
		t.Fatalf("bounded task snapshot accounting = %#v", savedSnapshot)
	}
	if manifest.SnapshotSHA256 != tasklens.SHA256(snapshotRaw) {
		t.Fatalf("manifest snapshot sha256 = %q, want exact saved bytes", manifest.SnapshotSHA256)
	}
	captured := make(map[string]struct{}, len(manifest.CapturedInputs))
	for _, input := range manifest.CapturedInputs {
		captured[input.Path] = struct{}{}
	}
	for _, materialPath := range bundle.AllowedPaths {
		if _, ok := captured[materialPath]; !ok {
			t.Fatalf("model-visible source %q is missing from captured inputs", materialPath)
		}
	}
	packPath := filepath.Join(runDir, tasklens.PackFile)
	packRaw := investigateReadFile(t, packPath)
	if err := os.WriteFile(packPath, append(packRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := report.ReadRunManifest(runDir); err == nil ||
		!strings.Contains(err.Error(), "Task Lens artifact") {
		t.Fatalf("ReadRunManifest after Task Lens artifact tamper error = %v", err)
	}
}

func TestRunInvestigateDoesNotAcceptCheapExitBeforePackSufficiency(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "cheap-exit-preview")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	investigateWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/cheapexitpreview\n\ngo 1.24\n")
	investigateWriteFile(t, filepath.Join(repo, "config.go"), `package preview

type Config struct { Enabled bool }
type Engine struct { Config Config }

func CopyConfig(engine *Engine, config Config) {
	engine.Config.Enabled = config.Enabled
}

func Enabled(engine *Engine) bool { return engine.Config.Enabled }
`)
	investigateWriteFile(t, filepath.Join(repo, "config_test.go"), `package preview

import "testing"

func TestCopyConfig(t *testing.T) {
	engine := &Engine{}
	CopyConfig(engine, Config{Enabled: true})
	if !Enabled(engine) { t.Fatal("not copied") }
}
`)
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "config.go", "config_test.go")
	commitTestRepository(t, repo)
	taskFile := filepath.Join(t.TempDir(), "task.md")
	investigateWriteFile(t, taskFile, "## Prompt-safe task\n\nThe Enabled configuration is ignored. Locate CopyConfig and the nearest verification test.\n")
	debugDir := filepath.Join(t.TempDir(), "runs")
	var stdout, stderr bytes.Buffer
	providerCalls := 0
	previewFinalizations := 0
	err := runInvestigate([]string{
		repo, "--task-file", taskFile, "--debug-dir", debugDir,
		"--no-open", "--no-serve", "--strict-snapshot",
	}, investigateDependencies{
		ctx: context.Background(), stdout: &stdout, stderr: &stderr,
		newProvider: func(bool) (taskInvestigationProvider, deepseek.EffectiveConfig, error) {
			return failedTaskProviderStub{calls: &providerCalls}, deepseek.EffectiveConfig{}, nil
		},
		finalizePack: func(pack tasklens.Pack, attemptState string) (tasklens.Pack, bool) {
			if attemptState == "skipped_local_complete" {
				previewFinalizations++
				pack.Locality = tasklens.LocalityBroadDynamic
			}
			return report.FinalizeTaskInvestigationPack(pack, attemptState)
		},
	})
	if err != nil {
		t.Fatalf("runInvestigate() error = %v\nstderr:\n%s", err, stderr.String())
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1 after insufficient local preview", providerCalls)
	}
	if previewFinalizations != 1 {
		t.Fatalf("preview finalizations = %d, want 1", previewFinalizations)
	}
	runDir := strings.TrimSpace(stdout.String())
	var bundle tasklens.Bundle
	if err := json.Unmarshal(investigateReadFile(t, filepath.Join(runDir, tasklens.BundleFile)), &bundle); err != nil {
		t.Fatal(err)
	}
	if !bundle.CheapExit.Eligible {
		t.Fatalf("fixture did not exercise an eligible collector cheap exit: %#v", bundle.CheapExit)
	}
	var status tasklens.Status
	if err := json.Unmarshal(investigateReadFile(t, filepath.Join(runDir, tasklens.StatusFile)), &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "partial_local" || status.Provider.Calls != 1 {
		t.Fatalf("status accepted an insufficient local preview: %#v", status)
	}
}

func TestMarshalTaskArtifactRejectsOversizedEscapedResponse(t *testing.T) {
	_, err := marshalTaskArtifact(tasklens.Attempt{
		Version:     tasklens.AttemptVersion,
		RawResponse: strings.Repeat("\x01", tasklens.MaxArtifactBytes),
	})
	if err == nil || !strings.Contains(err.Error(), "bounded saved size") {
		t.Fatalf("marshalTaskArtifact oversized error = %v", err)
	}
}

func TestRequireTaskInvestigationReportFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        *report.ReportData
		want        bool
		wantMessage string
	}{
		{name: "nil report", want: true},
		{name: "missing task projection", data: &report.ReportData{}, want: true},
		{
			name: "projection validation warning",
			data: &report.ReportData{Warnings: []string{
				"unrelated warning",
				"task investigation unavailable: saved pack failed reducer replay",
			}},
			want:        true,
			wantMessage: "saved pack failed reducer replay",
		},
		{
			name: "validated task projection",
			data: &report.ReportData{TaskInvestigation: &report.TaskInvestigationWorkspace{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireTaskInvestigationReport(test.data)
			if (err != nil) != test.want {
				t.Fatalf("requireTaskInvestigationReport() error = %v, want error %t", err, test.want)
			}
			if test.wantMessage != "" && (err == nil || !strings.Contains(err.Error(), test.wantMessage)) {
				t.Fatalf("requireTaskInvestigationReport() error = %v, want %q", err, test.wantMessage)
			}
		})
	}
}

func investigateWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func investigateReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func gitOutputForTest(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := append([]string{"-C", repo}, args...)
	var output bytes.Buffer
	if err := runCommandForTest(command, &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func runCommandForTest(args []string, output *bytes.Buffer) error {
	command := exec.Command("git", args...)
	command.Stdout = output
	command.Stderr = output
	return command.Run()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
