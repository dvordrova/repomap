package snapshot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
)

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

		got, err := Build(Options{RepoPath: repo})
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

		got, err := Build(Options{RepoPath: repo})
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

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "@example/manifest-project" {
			t.Fatalf("repo_name = %q, want manifest identity", got.RepoName)
		}
	})

	t.Run("untracked manifest cannot replace neutral fallback", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "untracked-task-label")
		writeSnapshotFile(t, repo, "README.md", "# Project\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"leaked-task-name"}`)
		trackSnapshotFiles(t, repo, "README.md")

		got, err := Build(Options{RepoPath: repo})
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

func TestBuildResolvesAndScopesAnalysisTarget(t *testing.T) {
	t.Run("repomap shaped executable", func(t *testing.T) {
		repo := t.TempDir()
		writeSnapshotFile(t, repo, "go.mod", "module example.com/repomap\n\ngo 1.24\n")
		writeSnapshotFile(t, repo, "cmd/repomap/main.go", "package main\nimport _ \"example.com/repomap/internal/report\"\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "cmd/quality/main.go", "package main\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "internal/report/report.go", "package report\n")
		trackSnapshotFiles(t, repo, "go.mod", "cmd/repomap/main.go", "cmd/quality/main.go", "internal/report/report.go")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.AnalysisTarget == nil || got.AnalysisTarget.PackagePath != "example.com/repomap/cmd/repomap" {
			t.Fatalf("target = %#v", got.AnalysisTarget)
		}
		if len(got.GoFacts.Packages) != 2 || len(got.GoFacts.EntrypointPackages) != 1 {
			t.Fatalf("scoped facts = %d packages, %d entrypoints", len(got.GoFacts.Packages), len(got.GoFacts.EntrypointPackages))
		}
		if slices.Contains(got.FilteredFiles, "cmd/quality/main.go") || !slices.Contains(got.FilteredFiles, "cmd/repomap/main.go") {
			t.Fatalf("target source files = %#v", got.FilteredFiles)
		}
	})

	t.Run("moby shaped ambiguity and override", func(t *testing.T) {
		repo := t.TempDir()
		writeSnapshotFile(t, repo, "go.mod", "module example.com/moby\n\ngo 1.24\n")
		writeSnapshotFile(t, repo, "cmd/dockerd/main.go", "package main\nimport _ \"example.com/moby/daemon\"\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "cmd/docker-proxy/main.go", "package main\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "daemon/daemon.go", "package daemon\n")
		trackSnapshotFiles(t, repo, "go.mod", "cmd/dockerd/main.go", "cmd/docker-proxy/main.go", "daemon/daemon.go")

		if _, err := Build(Options{RepoPath: repo}); err == nil ||
			!strings.Contains(err.Error(), "analysis target is ambiguous") ||
			!strings.Contains(err.Error(), "cmd/dockerd") ||
			!strings.Contains(err.Error(), "cmd/docker-proxy") ||
			strings.Contains(err.Error(), "example.com/moby@") ||
			strings.Contains(err.Error(), "daemon") {
			t.Fatalf("ambiguity error = %v", err)
		}
		got, err := Build(Options{RepoPath: repo, AnalysisTargetOverride: "cmd/dockerd"})
		if err != nil {
			t.Fatal(err)
		}
		if got.AnalysisTarget == nil || got.AnalysisTarget.PackageDir != "cmd/dockerd" || len(got.GoFacts.Packages) != 2 {
			t.Fatalf("selected target/facts = %#v / %#v", got.AnalysisTarget, got.GoFacts.Packages)
		}
	})

	t.Run("telebot shaped root library", func(t *testing.T) {
		repo := t.TempDir()
		writeSnapshotFile(t, repo, "go.mod", "module example.com/telebot\n\ngo 1.24\n")
		writeSnapshotFile(t, repo, "bot.go", "package telebot\n\nfunc NewBot() {}\n")
		writeSnapshotFile(t, repo, "layout/layout.go", "package layout\n\nfunc Open() {}\n")
		trackSnapshotFiles(t, repo, "go.mod", "bot.go", "layout/layout.go")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.AnalysisTarget == nil || got.AnalysisTarget.Kind != analysistarget.KindModuleLibrary ||
			got.AnalysisTarget.PackagePath != "" || len(got.GoFacts.Packages) != 2 {
			t.Fatalf("library target/facts = %#v / %#v", got.AnalysisTarget, got.GoFacts.Packages)
		}
	})
}

func TestAnalysisTargetCandidateKeysDescribeModuleLibrariesWithoutEmptyPackageAliases(t *testing.T) {
	candidates := []analysistarget.Candidate{
		{Target: analysistarget.Target{
			Kind: analysistarget.KindModuleLibrary, ModulePath: "example.com/root", ModuleDir: ".",
		}},
		{Target: analysistarget.Target{
			Kind: analysistarget.KindModuleLibrary, ModulePath: "example.com/nested", ModuleDir: "nested",
		}},
	}
	if got := analysisTargetCandidateKeys(candidates); got != "example.com/root, nested" {
		t.Fatalf("module-library candidate keys = %q", got)
	}
	executable := analysistarget.Candidate{MainModule: true, Target: analysistarget.Target{
		Kind: analysistarget.KindExecutablePackage, PackagePath: "example.com/root/cmd/app", PackageDir: "cmd/app",
	}}
	if got := analysisTargetCandidateKeys(append(candidates, executable)); got != "cmd/app" {
		t.Fatalf("executable candidate key = %q", got)
	}
}

func TestBuildDefersAnalysisTargetResolutionWithFullCatalog(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)

	got, err := Build(Options{RepoPath: repo, DeferAnalysisTargetResolution: true})
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
	deferred, err := Build(Options{RepoPath: repo, DeferAnalysisTargetResolution: true})
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
	deferred, err := Build(Options{RepoPath: repo, DeferAnalysisTargetResolution: true})
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

	deferred, err := Build(Options{RepoPath: repo, DeferAnalysisTargetResolution: true})
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

func TestBuildRejectsDeferredResolutionWithExplicitOverride(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)

	_, err := Build(Options{
		RepoPath: repo, AnalysisTargetOverride: "cmd/app", DeferAnalysisTargetResolution: true,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be deferred with explicit override") {
		t.Fatalf("defer plus override error = %v", err)
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

	got, err := Build(Options{RepoPath: repo})
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

func TestBuildReadsBoundedTrackedReadme(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	trackSnapshotFiles(t, repo, "README.md")

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "abc\n...[truncated]" {
		t.Fatalf("readme = %q", got.Readme)
	}
}

func TestBuildDoesNotReadTrackedReadmeSymlinkEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("do not disclose"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "README.md")

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "" {
		t.Fatalf("readme = %q, want empty", got.Readme)
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

	got, err := Build(Options{RepoPath: repo, MaxTreeLines: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.FileTree, "linked.go") ||
		!slices.Contains(got.FileTree, "regular.go") {
		t.Fatalf("file tree = %#v, want visible regular and symlink paths", got.FileTree)
	}
	if !slices.Equal(got.FilteredFiles, []string{"regular.go"}) {
		t.Fatalf("analysis files = %#v, want only regular.go", got.FilteredFiles)
	}
}

func TestBuildDoesNotReadUntrackedReadme(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("local private notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	trackSnapshotFiles(t, repo)

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "" {
		t.Fatalf("readme = %q, want empty for untracked file", got.Readme)
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

	got, err := Build(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.Go.GoModExists || got.Go.ModuleName != "" {
		t.Fatalf("Go hints = %#v, want escaping go.mod ignored", got.Go)
	}
}

func TestTruncateUTF8Bytes(t *testing.T) {
	in := "ábc"
	got, truncated := truncateUTF8Bytes(in, 1)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if got != "" {
		t.Fatalf("got %q, want empty valid UTF-8 string", got)
	}

	got, truncated = truncateUTF8Bytes(in, 2)
	if !truncated {
		t.Fatal("expected truncation at 2")
	}
	if got != "á" {
		t.Fatalf("got %q, want %q", got, "á")
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
