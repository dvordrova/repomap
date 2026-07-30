package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

func TestTrackedSymlinkDefaultPublishesRegularSourceSubset(t *testing.T) {
	result := runTrackedSymlinkPublication(t)
	assertTrackedSymlinkRegularSubset(t, result)
}

func TestEarlyCatalogRegularSubsetReachesOptionalModelStages(t *testing.T) {
	result := runTrackedSymlinkPublication(t)
	if result.providerRequests < 2 {
		t.Fatalf(
			"provider requests = %d, want orientation plus an optional stage",
			result.providerRequests,
		)
	}
	if !strings.Contains(result.stderr, "repomap: synthesizing bounded architecture grouping") {
		t.Fatalf("regular source subset did not reach an optional stage:\n%s", result.stderr)
	}
	if strings.Contains(
		result.stderr,
		"authorized source catalog is unavailable; skipping optional model stages and publishing a view-only report",
	) {
		t.Fatalf("tracked symlink disabled the regular source subset:\n%s", result.stderr)
	}
}

func TestSourceCatalogPathsAreRegularBoundedAndNoFollow(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "regular.go"), "package fixture\n")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "nested", "regular.go"), "package fixture\n")
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "--", "regular.go", "nested/regular.go")
	commitTestRepository(t, root)
	if err := os.Symlink("nested", filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	state, err := freshness.CaptureRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	if !sourceCatalogPathsAreRegular(
		context.Background(),
		state,
		state.Identity,
		[]string{"nested/regular.go", "regular.go"},
	) {
		t.Fatal("bounded regular-file scope was rejected")
	}
	if sourceCatalogPathsAreRegular(context.Background(), state, state.Identity, []string{"linked/regular.go"}) {
		t.Fatal("preflight followed a symlinked directory")
	}
	if sourceCatalogPathsAreRegular(
		context.Background(),
		state,
		state.Identity,
		[]string{strings.Repeat("a", maxEarlySourceCatalogPathBytes+1)},
	) {
		t.Fatal("preflight accepted an oversized path")
	}
	if sourceCatalogPathsAreRegular(
		context.Background(),
		state,
		state.Identity,
		make([]string, maxEarlySourceCatalogPaths+1),
	) {
		t.Fatal("preflight accepted an oversized path collection")
	}
}

func TestSourceCatalogPathsAreRegularRejectsTrackedSymlinkMaterializedAsFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "target.go"), "package fixture\n")
	if err := os.Symlink("target.go", filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "--", "target.go", "linked.go")
	commitTestRepository(t, root)
	runGit(t, root, "config", "core.symlinks", "false")
	if err := os.Remove(filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "linked.go"), "target.go")

	state, err := freshness.CaptureRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Dirty) != 0 {
		t.Fatalf("core.symlinks=false materialization is unexpectedly dirty: %#v", state.Dirty)
	}
	if sourceCatalogPathsAreRegular(
		context.Background(),
		state,
		state.Identity,
		[]string{"linked.go"},
	) {
		t.Fatal("preflight accepted tracked mode 120000 materialized as a regular file")
	}
}

func TestSourceCatalogGitEnvironmentDropsAmbientAuthority(t *testing.T) {
	environment := strings.Join(sourceCatalogGitEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_DIR=/tmp/other.git",
		"GIT_WORK_TREE=/tmp/other",
		"GIT_OBJECT_DIRECTORY=/tmp/objects",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=/tmp/hooks",
		"PAGER=less",
	}), "\n")
	for _, unsafe := range []string{
		"GIT_DIR=",
		"GIT_WORK_TREE=",
		"GIT_OBJECT_DIRECTORY=",
		"GIT_CONFIG_COUNT=",
		"GIT_CONFIG_KEY_",
		"GIT_CONFIG_VALUE_",
		"PAGER=less",
	} {
		if strings.Contains(environment, unsafe) {
			t.Fatalf("preflight Git environment retained %q:\n%s", unsafe, environment)
		}
	}
	for _, required := range []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	} {
		if !strings.Contains(environment, required) {
			t.Fatalf("preflight Git environment omitted %q:\n%s", required, environment)
		}
	}
}

type trackedSymlinkPublication struct {
	runDir           string
	stderr           string
	providerRequests int
	requestBody      []byte
}

func runTrackedSymlinkPublication(t *testing.T) trackedSymlinkPublication {
	t.Helper()
	clearLLMEnv(t)

	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/tracked-link\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	linkDir := filepath.Join(repository, "client", "v3")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(linkDir, "example_lease_test.go")
	if err := os.Symlink("../../main.go", linkPath); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	runGit(
		t,
		repository,
		"add",
		"--",
		"go.mod",
		"main.go",
		"client/v3/example_lease_test.go",
	)
	commitTestRepository(t, repository)
	if stage := gitOutputForTest(
		t,
		repository,
		"ls-files",
		"--stage",
		"--",
		"client/v3/example_lease_test.go",
	); !strings.HasPrefix(stage, "120000 ") {
		t.Fatalf("tracked symlink mode = %q, want 120000", stage)
	}

	orientationJSON, err := json.Marshal(map[string]any{
		"project_guess": "tracked-link fixture",
		"confidence":    0.9,
		"high_level_map": []any{map[string]any{
			"name":           "command",
			"evidence":       []string{"main.go"},
			"why_it_matters": "owns process startup",
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
			"why_interesting":   "shows startup",
			"evidence":          []string{"main.go"},
			"confidence":        0.9,
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []any{},
		"unverified_paths":       []any{},
		"warnings":               []any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var providerRequests atomic.Int32
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := providerRequests.Add(1)
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Errorf("read provider request: %v", readErr)
			return
		}
		if call == 1 {
			requestBody = append([]byte(nil), body...)
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": string(orientationJSON),
			}}},
		}); err != nil {
			t.Errorf("write provider response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	debugDir := t.TempDir()
	args := []string{
		"--debug-dir", debugDir,
		"--discover-surfaces=false",
		"--no-open",
		"--no-serve",
	}
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repository, args, defaultRunDeps{
		ctx:    context.Background(),
		stdout: io.Discard,
		stderr: &stderr,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps: %v\nstderr:\n%s", err, stderr.String())
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	return trackedSymlinkPublication{
		runDir:           runDir,
		stderr:           stderr.String(),
		providerRequests: int(providerRequests.Load()),
		requestBody:      requestBody,
	}
}

func assertTrackedSymlinkRegularSubset(
	t *testing.T,
	result trackedSymlinkPublication,
) {
	t.Helper()
	const linkPath = "client/v3/example_lease_test.go"

	if result.providerRequests < 2 {
		t.Fatalf("provider requests = %d, want at least 2", result.providerRequests)
	}
	if bytes.Contains(result.requestBody, []byte(linkPath)) {
		t.Fatalf("tracked symlink reached the initial bounded allowed scope: %s", result.requestBody)
	}
	if !strings.Contains(result.stderr, "Report: ") {
		t.Fatalf("regular-subset report was not published:\n%s", result.stderr)
	}
	for _, name := range []string{"report.json", "report.html", report.RunManifestFilename} {
		info, err := os.Stat(filepath.Join(result.runDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("%s was not published as a non-empty regular file: info=%v err=%v", name, info, err)
		}
	}

	reportJSON, err := os.ReadFile(filepath.Join(result.runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated report.ReportData
	if err := json.Unmarshal(reportJSON, &generated); err != nil {
		t.Fatal(err)
	}
	if generated.FormatVersion != report.CurrentFormatVersion {
		t.Fatalf("report format version = %d", generated.FormatVersion)
	}
	if trackedSymlinkContains(generated.OpenablePaths, linkPath) {
		t.Fatalf("tracked symlink reached openable paths: %#v", generated.OpenablePaths)
	}
	if generated.SemanticSearch != nil {
		t.Fatalf("default view-only report retained semantic search: %#v", generated.SemanticSearch)
	}

	manifest, err := report.ReadRunManifest(result.runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	if manifest.Version != report.CurrentRunManifestVersion ||
		manifest.ReportFormatVersion != report.CurrentFormatVersion {
		t.Fatalf(
			"view-only manifest/report versions = %d/%d",
			manifest.Version,
			manifest.ReportFormatVersion,
		)
	}
	catalog, err := manifest.SourceCatalog()
	if err != nil {
		t.Fatalf("regular source catalog is unavailable: %v", err)
	}
	if _, ok := catalog.Lookup(linkPath); ok {
		t.Fatal("tracked symlink reached the source catalog")
	}
	if _, err := manifest.WorkspaceSnapshot(); err != nil {
		t.Fatalf("regular workspace snapshot is unavailable: %v", err)
	}
}

func trackedSymlinkContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
