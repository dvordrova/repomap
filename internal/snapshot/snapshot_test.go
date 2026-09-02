package snapshot

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
)

func buildSnapshotForTest(opts Options) (Snapshot, error) {
	if strings.TrimSpace(opts.GoTarget) == "" {
		opts.GoTarget = runtime.GOOS + "/" + runtime.GOARCH
	}
	repository, err := corpus.Open(context.Background(), opts.RepoPath)
	if err != nil {
		return Snapshot{}, err
	}
	defer repository.Close()
	opts.RepositoryCorpus = repository
	return BuildContext(context.Background(), opts)
}

func TestBuildContextRequiresSharedCorpusAndExactGoTarget(t *testing.T) {
	_, err := BuildContext(context.Background(), Options{
		RepoPath: t.TempDir(), GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err == nil || !strings.Contains(err.Error(), "repository corpus is required") {
		t.Fatalf("missing corpus error = %v", err)
	}

	repositoryPath := newDeferredAnalysisTargetFixture(t)
	repository, err := corpus.Open(context.Background(), repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = BuildContext(context.Background(), Options{
		RepoPath: repositoryPath, RepositoryCorpus: repository, GoTarget: "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "restore resolved Go target") {
		t.Fatalf("invalid target error = %v", err)
	}
}

func TestBuildUsesSemanticRepositoryIdentity(t *testing.T) {
	t.Parallel()

	t.Run("root module precedes remote and manifest", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "task-labelled-checkout")
		writeSnapshotFile(t, repo, "go.mod", "module example.com/owner/module\n\ngo 1.26\n")
		writeSnapshotFile(t, repo, "main.go", "package main\n\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"manifest-name"}`)
		trackSnapshotFiles(t, repo, "go.mod", "main.go", "package.json")
		runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "git@github.com:owner/remote-name.git")

		got, err := buildSnapshotForTest(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "example.com/owner/module" {
			t.Fatalf("repo_name = %q, want root module path", got.RepoName)
		}
		if got.DisplayName != "task-labelled-checkout" {
			t.Fatalf("display_name = %q, want local checkout label", got.DisplayName)
		}
	})

	t.Run("normalized remote precedes manifest", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "another-task-label")
		writeSnapshotFile(t, repo, "README.md", "# Project\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"manifest-name"}`)
		trackSnapshotFiles(t, repo, "README.md", "package.json")
		runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "https://token@GitHub.com/owner/remote-name.git")

		got, err := buildSnapshotForTest(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "github.com/owner/remote-name" {
			t.Fatalf("repo_name = %q, want normalized remote identity", got.RepoName)
		}
	})

	t.Run("tracked manifest precedes neutral fallback", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "manifest-task-label")
		writeSnapshotFile(t, repo, "package.json", `{"name":"@example/manifest-project"}`)
		trackSnapshotFiles(t, repo, "package.json")

		got, err := buildSnapshotForTest(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "@example/manifest-project" {
			t.Fatalf("repo_name = %q, want manifest identity", got.RepoName)
		}
	})

	t.Run("large tracked manifest keeps its identity", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "large-manifest")
		manifest := strings.Repeat(" ", AdvisoryManifestBytes+1) + `{"name":"complete-manifest-project"}`
		writeSnapshotFile(t, repo, "package.json", manifest)
		trackSnapshotFiles(t, repo, "package.json")

		got, err := buildSnapshotForTest(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "complete-manifest-project" {
			t.Fatalf("repo_name = %q", got.RepoName)
		}
		warnings := RepositoryScaleWarnings(got)
		if !slices.ContainsFunc(warnings, func(warning RepositoryScaleWarning) bool {
			return warning.Kind == RepositoryScaleWarningManifestBytes
		}) {
			t.Fatalf("manifest warnings = %#v", warnings)
		}
	})

	t.Run("untracked manifest cannot replace neutral fallback", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "untracked-task-label")
		writeSnapshotFile(t, repo, "README.md", "# Project\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"leaked-task-name"}`)
		trackSnapshotFiles(t, repo, "README.md")

		got, err := buildSnapshotForTest(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != neutralRepositoryName {
			t.Fatalf("repo_name = %q, want neutral fallback", got.RepoName)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "leaked-task-name") {
			t.Fatalf("snapshot included untracked manifest identity: %s", encoded)
		}
	})
}

func TestGoModuleMetadataReadsBeyondFormerOneMiBLimit(t *testing.T) {
	repo := t.TempDir()
	goMod := strings.Repeat("// retained metadata\n", AdvisoryGoModuleBytes/20+2) +
		"module example.com/complete/module\n\ngo 1.24\n"
	writeSnapshotFile(t, repo, "go.mod", goMod)
	trackSnapshotFiles(t, repo, "go.mod")
	repository, err := corpus.Open(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	exists, name, byteCount := goModuleMetadata(repository, []string{"go.mod"})
	if !exists || name != "example.com/complete/module" || byteCount != len(goMod) {
		t.Fatalf("go module metadata = %t, %q, %d", exists, name, byteCount)
	}
}

func TestBuildAutoGoTargetUsesUniqueProductionPlatformBeforeGoFacts(t *testing.T) {
	repo := t.TempDir()
	writeSnapshotFile(t, repo, "go.mod", "module example.com/moby\n\ngo 1.24\n")
	writeSnapshotFile(t, repo, "cmd/dockerd/main.go", "package main\nimport \"example.com/moby/daemon\"\nfunc main() { daemon.Run() }\n")
	writeSnapshotFile(t, repo, "daemon/config_linux.go", "package daemon\nfunc Run() {}\n")
	writeSnapshotFile(t, repo, "daemon/network_linux.go", "package daemon\nconst network = true\n")
	writeSnapshotFile(t, repo, "daemon/storage_linux.go", "package daemon\nconst storage = true\n")
	trackSnapshotFiles(t, repo,
		"go.mod", "cmd/dockerd/main.go", "daemon/config_linux.go",
		"daemon/network_linux.go", "daemon/storage_linux.go",
	)

	got, err := buildSnapshotForTest(Options{
		RepoPath: repo, GoTarget: "darwin/amd64", AutoGoTarget: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GoTargetSelection == nil || got.GoTargetSelection.Source != GoTargetSelectionAuto ||
		got.GoTargetSelection.Target != "linux/amd64" ||
		got.GoTargetSelection.Baseline != "darwin/amd64" ||
		got.GoTargetSelection.Display() != "auto: linux/amd64 (host darwin)" {
		t.Fatalf("automatic selection = %#v", got.GoTargetSelection)
	}
	if got.AnalysisTarget != nil || got.TargetCatalog == nil || len(got.TargetCatalog.Entries) != 2 {
		t.Fatalf("fresh snapshot must expose an unselected exact catalog: target=%#v catalog=%#v", got.AnalysisTarget, got.TargetCatalog)
	}
	if got.GoFacts == nil || got.GoFacts.Coverage.State == "unavailable" {
		t.Fatalf("final Linux Go facts = %#v", got.GoFacts)
	}
	if !slices.Contains(got.FilteredFiles, "daemon/config_linux.go") {
		t.Fatalf("final target files = %#v", got.FilteredFiles)
	}
}

func TestBuildAutoGoTargetLeavesExplicitAndAmbiguousBaselinesUnchanged(t *testing.T) {
	t.Run("caller disables automatic selection for an explicit target", func(t *testing.T) {
		repo := t.TempDir()
		writeSnapshotFile(t, repo, "go.mod", "module example.com/explicit\n\ngo 1.24\n")
		writeSnapshotFile(t, repo, "main.go", "package main\nfunc main() {}\n")
		for _, path := range []string{"a_linux.go", "b_linux.go", "c_linux.go"} {
			writeSnapshotFile(t, repo, path, "package main\n")
		}
		trackSnapshotFiles(t, repo, "go.mod", "main.go", "a_linux.go", "b_linux.go", "c_linux.go")

		got, err := buildSnapshotForTest(Options{RepoPath: repo, GoTarget: "darwin/amd64", AutoGoTarget: false})
		if err != nil {
			t.Fatal(err)
		}
		if got.GoTargetSelection != nil || got.GoTargetAdvisory == nil ||
			got.GoTargetAdvisory.Suggested != "linux/amd64" {
			t.Fatalf("explicit baseline authority = selection %#v, advisory %#v", got.GoTargetSelection, got.GoTargetAdvisory)
		}
	})

	t.Run("tied platform evidence does not select", func(t *testing.T) {
		repo := t.TempDir()
		writeSnapshotFile(t, repo, "go.mod", "module example.com/tied\n\ngo 1.24\n")
		writeSnapshotFile(t, repo, "main.go", "package main\nfunc main() {}\n")
		paths := []string{"go.mod", "main.go"}
		for _, goos := range []string{"linux", "windows"} {
			for _, stem := range []string{"a", "b", "c"} {
				path := stem + "_" + goos + ".go"
				writeSnapshotFile(t, repo, path, "package main\n")
				paths = append(paths, path)
			}
		}
		trackSnapshotFiles(t, repo, paths...)

		got, err := buildSnapshotForTest(Options{RepoPath: repo, GoTarget: "darwin/amd64", AutoGoTarget: true})
		if err != nil {
			t.Fatal(err)
		}
		if got.GoTargetSelection != nil || got.GoTargetAdvisory != nil {
			t.Fatalf("tied evidence selected a target: %#v / %#v", got.GoTargetSelection, got.GoTargetAdvisory)
		}
	})
}

func TestBuildProducesFullUnselectedAnalysisTargetCatalog(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)

	got, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalysisTarget != nil {
		t.Fatalf("deferred target = %#v, want nil", got.AnalysisTarget)
	}
	if got.TargetCatalog == nil {
		t.Fatal("deferred target catalog is nil")
	}
	if err := got.TargetCatalog.Validate(); err != nil {
		t.Fatalf("deferred target catalog: %v", err)
	}
	if got.GoFacts == nil || len(got.GoFacts.Packages) != 3 || len(got.TargetCatalog.Entries) != 3 {
		t.Fatalf("full facts/catalog = %#v / %#v", got.GoFacts, got.TargetCatalog)
	}
	for _, packageDir := range []string{"cmd/app", "cmd/helper"} {
		if deferredTargetRef(t, got, packageDir) == "" {
			t.Fatalf("catalog omitted %q", packageDir)
		}
	}
	if deferredModuleTargetRef(t, got) == "" {
		t.Fatal("catalog omitted module-library target")
	}

	wire, err := got.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, leaked := encoded["target_catalog"]; leaked {
		t.Fatalf("live target catalog leaked into snapshot JSON: %s", wire)
	}
}

func TestScopeAnalysisTargetUsesExactCatalogRefAndExcludesHelper(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)
	deferred, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	targetRef := deferredTargetRef(t, deferred, "cmd/app")

	scoped, err := ScopeAnalysisTarget(deferred, targetRef)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.AnalysisTarget == nil || scoped.AnalysisTarget.Ref != targetRef || scoped.AnalysisTarget.PackageDir != "cmd/app" {
		t.Fatalf("scoped target = %#v", scoped.AnalysisTarget)
	}
	if scoped.TargetCatalog != nil {
		t.Fatalf("scoped snapshot retained live catalog: %#v", scoped.TargetCatalog)
	}
	if scoped.GoFacts == nil || len(scoped.GoFacts.Packages) != 2 {
		t.Fatalf("scoped facts = %#v", scoped.GoFacts)
	}
	for _, pkg := range scoped.GoFacts.Packages {
		if pkg.PackageDir == "cmd/helper" || pkg.CanonicalPath == "example.com/workspace/cmd/helper" {
			t.Fatalf("helper package survived exact scope: %#v", pkg)
		}
	}
	if slices.Contains(scoped.FilteredFiles, "cmd/helper/main.go") ||
		!slices.Contains(scoped.FilteredFiles, "cmd/app/main.go") ||
		!slices.Contains(scoped.FilteredFiles, "core/core.go") {
		t.Fatalf("scoped files = %#v", scoped.FilteredFiles)
	}
	if deferred.TargetCatalog == nil || deferred.GoFacts == nil || len(deferred.GoFacts.Packages) != 3 {
		t.Fatalf("scope mutated deferred snapshot: catalog=%#v facts=%#v", deferred.TargetCatalog, deferred.GoFacts)
	}

	wire, err := scoped.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, leaked := encoded["target_catalog"]; leaked {
		t.Fatalf("scoped snapshot JSON included target catalog: %s", wire)
	}
}

func TestScopeAnalysisTargetRejectsUnknownEmptyAndDriftedRefs(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)
	deferred, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ScopeAnalysisTarget(deferred, "at-unknown"); err == nil || !strings.Contains(err.Error(), "unknown target ref") {
		t.Fatalf("unknown ref error = %v", err)
	}
	if _, err := ScopeAnalysisTarget(deferred, ""); err == nil || !strings.Contains(err.Error(), "exact and non-empty") {
		t.Fatalf("empty ref error = %v", err)
	}

	tampered := deferred
	catalog := deferred.TargetCatalog.Snapshot()
	catalog.Entries[0].Candidate.Target.Ref += "-drift"
	tampered.TargetCatalog = &catalog
	if _, err := ScopeAnalysisTarget(tampered, deferred.TargetCatalog.Entries[0].Candidate.Target.Ref); err == nil ||
		!strings.Contains(err.Error(), "validate live target catalog") {
		t.Fatalf("drifted catalog error = %v", err)
	}
}

func TestScopeAnalysisTargetModuleLibraryExcludesMainAndRetainsModulePackages(t *testing.T) {
	repo := t.TempDir()
	writeSnapshotFile(t, repo, "go.mod", "module example.com/mixed\n\ngo 1.26\n")
	writeSnapshotFile(t, repo, "main.go", "package main\n\nfunc main() {}\n")
	writeSnapshotFile(t, repo, "api/api.go", "package api\n\nfunc Open() {}\n")
	writeSnapshotFile(t, repo, "internal/store/store.go", "package store\n\nfunc Save() {}\n")
	trackSnapshotFiles(t, repo, "go.mod", "main.go", "api/api.go", "internal/store/store.go")

	deferred, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil {
		t.Fatal("deferred catalog is nil")
	}
	moduleRef := ""
	for _, entry := range deferred.TargetCatalog.Entries {
		if entry.Candidate.Target.Kind == analysistarget.KindModuleLibrary {
			moduleRef = entry.Candidate.Target.Ref
		}
	}
	if moduleRef == "" {
		t.Fatalf("module library missing from %#v", deferred.TargetCatalog.Entries)
	}

	scoped, err := ScopeAnalysisTarget(deferred, moduleRef)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.AnalysisTarget == nil || scoped.AnalysisTarget.Kind != analysistarget.KindModuleLibrary ||
		scoped.AnalysisTarget.PackagePath != "" || scoped.AnalysisTarget.PackageDir != "" {
		t.Fatalf("module target = %#v", scoped.AnalysisTarget)
	}
	if scoped.GoFacts == nil || len(scoped.GoFacts.Packages) != 2 {
		t.Fatalf("module facts = %#v", scoped.GoFacts)
	}
	for _, pkg := range scoped.GoFacts.Packages {
		if pkg.Name == "main" || pkg.CanonicalPath == "example.com/mixed" {
			t.Fatalf("main package survived module-library scope: %#v", pkg)
		}
	}
	if slices.Contains(scoped.FilteredFiles, "main.go") ||
		!slices.Contains(scoped.FilteredFiles, "api/api.go") ||
		!slices.Contains(scoped.FilteredFiles, "internal/store/store.go") ||
		!slices.Contains(scoped.FilteredFiles, "go.mod") {
		t.Fatalf("module-library files = %#v", scoped.FilteredFiles)
	}
}

func TestNormalizeRemoteIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		remote   string
		expected string
	}{
		{name: "https", remote: "https://github.com/go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "https credentials", remote: "https://secret@example.com/owner/repo.git", expected: "example.com/owner/repo"},
		{name: "ssh url", remote: "ssh://git@GitHub.com/go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "scp syntax", remote: "git@github.com:go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "local absolute path", remote: "/tmp/task-labelled-checkout", expected: ""},
		{name: "file url", remote: "file:///tmp/task-labelled-checkout", expected: ""},
		{name: "local relative path", remote: "../task-labelled-checkout", expected: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeRemoteIdentity(test.remote); got != test.expected {
				t.Fatalf("normalizeRemoteIdentity(%q) = %q, want %q", test.remote, got, test.expected)
			}
		})
	}
}

func TestRepositoryOriginIdentityDoesNotGuessAmongOtherRemotes(t *testing.T) {
	t.Parallel()

	repo := filepath.Join(t.TempDir(), "multiple-remotes")
	writeSnapshotFile(t, repo, "README.md", "# Repository\n")
	trackSnapshotFiles(t, repo, "README.md")
	runSnapshotGit(t, "-C", repo, "remote", "add", "alpha", "git@gitlab.example.test:alpha/project.git")
	runSnapshotGit(t, "-C", repo, "remote", "add", "beta", "git@gitlab.example.test:beta/project.git")

	if got := RepositoryOriginIdentity(repo); got != "" {
		t.Fatalf("RepositoryOriginIdentity() guessed %q without origin", got)
	}
	runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "https://token@gitlab.example.test/team/project.git")
	if got := RepositoryOriginIdentity(repo); got != "gitlab.example.test/team/project" {
		t.Fatalf("RepositoryOriginIdentity() = %q", got)
	}
}

func TestBuildIdentityIgnoresAmbientGitConfigOverrides(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "ambient-config-checkout")
	writeSnapshotFile(t, repo, "README.md", "# Repository\n")
	trackSnapshotFiles(t, repo, "README.md")
	runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "https://github.com/wanted/repository.git")

	ambientConfig := filepath.Join(t.TempDir(), "ambient.gitconfig")
	if err := os.WriteFile(
		ambientConfig,
		[]byte("[remote \"origin\"]\n\turl = https://example.invalid/ambient/repository.git\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG", ambientConfig)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "remote.origin.url")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://example.invalid/injected/repository.git")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'remote.origin.url'='https://example.invalid/parameters/repository.git'")
	t.Setenv("GIT_CONFIG_GLOBAL", ambientConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", ambientConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	got, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoName != "github.com/wanted/repository" {
		t.Fatalf("repo_name = %q, want repository-local origin", got.RepoName)
	}
}

func TestRepositoryGitEnvironmentDropsAmbientConfigOverrides(t *testing.T) {
	t.Parallel()

	environment := []string{
		"PATH=/usr/bin", "GIT_DIR=/tmp/other.git", "GIT_CONFIG=/tmp/config",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.origin.url",
		"GIT_CONFIG_VALUE_0=https://example.invalid/repository.git",
		"GIT_CONFIG_PARAMETERS='remote.origin.url'='https://example.invalid/parameters.git'",
		"GIT_CONFIG_SYSTEM=/tmp/system", "GIT_CONFIG_GLOBAL=/tmp/global", "GIT_CONFIG_NOSYSTEM=1",
	}
	joined := strings.Join(repositoryGitEnvironment(environment), "\n")
	for _, forbidden := range []string{
		"GIT_DIR=", "GIT_CONFIG=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_",
		"GIT_CONFIG_VALUE_", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_SYSTEM=",
		"GIT_CONFIG_GLOBAL=", "GIT_CONFIG_NOSYSTEM=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("repository environment retained %q: %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("repository environment lost PATH: %q", joined)
	}
}

func TestParseRepositoryManifestName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parse    func([]byte) string
		manifest string
		expected string
	}{
		{name: "package json", parse: parseJSONManifestName, manifest: `{"name":"web-project"}`, expected: "web-project"},
		{name: "python project", parse: parsePythonManifestName, manifest: "[project]\nname = \"python-project\"\n", expected: "python-project"},
		{name: "poetry project", parse: parsePythonManifestName, manifest: "[tool.poetry]\nname = 'poetry-project'\n", expected: "poetry-project"},
		{name: "cargo package", parse: parseCargoManifestName, manifest: "[package]\nname = \"rust-project\"\n", expected: "rust-project"},
		{name: "setup metadata", parse: parseSetupConfigName, manifest: "[metadata]\nname = python-legacy\n", expected: "python-legacy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.parse([]byte(test.manifest)); got != test.expected {
				t.Fatalf("manifest name = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestBuildKeepsTrackedSymlinkVisibleButNotAnalyzable(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "regular.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular.go", filepath.Join(repo, "linked.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "regular.go", "linked.go")

	got, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.FilteredFiles, []string{"regular.go"}) {
		t.Fatalf("analysis files = %#v, want only regular.go", got.FilteredFiles)
	}
}

func TestShouldSkipPath(t *testing.T) {
	cases := []struct {
		path string
		skip bool
	}{
		{"vendor/a.go", true},
		{"node_modules/pkg/index.js", true},
		{".github/workflows/ci.yml", true},
		{"configs/.env", true},
		{"certs/tls.key", true},
		{"images/logo.png", true},
		{"cmd/repomap/main.go", false},
	}

	for _, tc := range cases {
		if got := shouldSkipPath(tc.path); got != tc.skip {
			t.Fatalf("shouldSkipPath(%q)=%v, want %v", tc.path, got, tc.skip)
		}
	}
}

func TestParseModuleName(t *testing.T) {
	got := parseModuleName([]byte("module github.com/example/demo\n\ngo 1.22\n"))
	if got != "github.com/example/demo" {
		t.Fatalf("parseModuleName()=%q, want %q", got, "github.com/example/demo")
	}
}

func TestBuildDoesNotReadTrackedGoModSymlinkEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(outside, []byte("module private.example/outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "go.mod")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "go.mod")

	got, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoName == "private.example/outside" {
		t.Fatalf("repository identity escaped through tracked go.mod symlink: %q", got.RepoName)
	}
}

func TestBuildFailsWhenRequestedGoFactsCannotLoad(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeSnapshotFile(t, repo, "go.mod", "this is not a Go module\n")
	writeSnapshotFile(t, repo, "main.go", "package main\nfunc main() {}\n")
	trackSnapshotFiles(t, repo, "go.mod", "main.go")

	_, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err == nil || !strings.Contains(err.Error(), "load exact Go facts") {
		t.Fatalf("error = %v, want fail-closed Go facts error", err)
	}
}

func TestBuildCanExplicitlySkipIncidentalGoFilesForAnotherLanguage(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeSnapshotFile(t, repo, "app.py", "print('ready')\n")
	writeSnapshotFile(t, repo, "example.go", "not valid Go source\n")
	trackSnapshotFiles(t, repo, "app.py", "example.go")

	got, err := buildSnapshotForTest(Options{RepoPath: repo, SkipGoFacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.GoFacts != nil || got.AnalysisTarget != nil || got.TargetCatalog != nil {
		t.Fatalf("explicit non-Go snapshot activated Go authority: %#v", got)
	}
}

func newDeferredAnalysisTargetFixture(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	writeSnapshotFile(t, repo, "go.mod", "module example.com/workspace\n\ngo 1.24\n")
	writeSnapshotFile(t, repo, "cmd/app/main.go", "package main\nimport _ \"example.com/workspace/core\"\nfunc main() {}\n")
	writeSnapshotFile(t, repo, "cmd/helper/main.go", "package main\nfunc main() {}\n")
	writeSnapshotFile(t, repo, "core/core.go", "package core\n\nfunc Open() {}\n")
	trackSnapshotFiles(t, repo, "go.mod", "cmd/app/main.go", "cmd/helper/main.go", "core/core.go")
	return repo
}

func deferredTargetRef(t *testing.T, snapshot Snapshot, packageDir string) string {
	t.Helper()

	if snapshot.TargetCatalog == nil {
		t.Fatal("target catalog is nil")
	}
	for _, entry := range snapshot.TargetCatalog.Entries {
		if entry.Candidate.Target.PackageDir == packageDir {
			return entry.Candidate.Target.Ref
		}
	}
	t.Fatalf("target catalog omitted package directory %q", packageDir)
	return ""
}

func deferredModuleTargetRef(t *testing.T, snapshot Snapshot) string {
	t.Helper()
	if snapshot.TargetCatalog == nil {
		t.Fatal("target catalog is nil")
	}
	for _, entry := range snapshot.TargetCatalog.Entries {
		if entry.Candidate.Target.Kind == analysistarget.KindModuleLibrary {
			return entry.Candidate.Target.Ref
		}
	}
	t.Fatal("target catalog omitted module-library target")
	return ""
}

func trackSnapshotFiles(t *testing.T, repo string, paths ...string) {
	t.Helper()

	runSnapshotGit(t, "init", "--quiet", repo)
	args := []string{"-C", repo, "add", "--"}
	args = append(args, paths...)
	runSnapshotGit(t, args...)
}

func writeSnapshotFile(t *testing.T, repo, relativePath, contents string) {
	t.Helper()

	filePath := filepath.Join(repo, relativePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runSnapshotGit(t *testing.T, args ...string) {
	t.Helper()

	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
