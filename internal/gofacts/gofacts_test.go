package gofacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func loadForHost(ctx context.Context, repoPath string, files []string) (*Facts, error) {
	return LoadWithOptions(ctx, repoPath, files, LoadOptions{GoTarget: runtime.GOOS + "/" + runtime.GOARCH})
}

func TestLoadWithOptionsRequiresResolvedGoTarget(t *testing.T) {
	_, err := LoadWithOptions(context.Background(), t.TempDir(), nil, LoadOptions{})
	if err == nil || !strings.Contains(err.Error(), "resolved Go target is required") {
		t.Fatalf("missing target error = %v", err)
	}
}

func TestLoadCollectsEveryModuleWithoutADegradedPackageCap(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"a-large/go.mod":          "module example.com/a\n\ngo 1.24\n",
		"a-large/cmd/a/main.go":   "package main\n\nimport _ \"example.com/a/lib\"\n\nfunc main() {}\n",
		"a-large/lib/lib.go":      "package lib\n",
		"z-service/go.mod":        "module example.com/z\n\ngo 1.24\n",
		"z-service/cmd/z/main.go": "package main\n\nimport _ \"example.com/z/lib\"\n\nfunc main() {}\n",
		"z-service/lib/lib.go":    "package lib\n",
	}
	fileList := make([]string, 0, len(files))
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}

	facts, err := loadForHost(context.Background(), repo, fileList)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Modules) != 2 || len(facts.EntrypointPackages) != 2 {
		t.Fatalf("complete module facts = modules %d entrypoints %d", len(facts.Modules), len(facts.EntrypointPackages))
	}
	if facts.PackagesCount != 4 || facts.RetainedPackagesCount != 4 || len(facts.Packages) != 4 {
		t.Fatalf("package counts = discovered %d retained %d/%d", facts.PackagesCount, facts.RetainedPackagesCount, len(facts.Packages))
	}
	retained := make(map[string]struct{}, len(facts.Packages))
	for _, pkg := range facts.Packages {
		retained[pkg.CanonicalPath] = struct{}{}
	}
	for _, want := range []string{"example.com/a/cmd/a", "example.com/a/lib", "example.com/z/cmd/z", "example.com/z/lib"} {
		if _, ok := retained[want]; !ok {
			t.Fatalf("fair selection omitted exact entry package %q: %#v", want, facts.Packages)
		}
	}
	if facts.Coverage.State != CoverageComplete || facts.Coverage.ModulesDiscovered != 2 ||
		facts.Coverage.ModulesAvailable != 2 || facts.Coverage.ModulesUnavailable != 0 ||
		facts.Coverage.PackagesDiscovered != 4 || facts.Coverage.PackagesRetained != 4 ||
		facts.Coverage.EdgesDiscovered != 2 || facts.Coverage.EdgesRetained != 2 ||
		len(facts.InternalEdges) != 2 {
		t.Fatalf("coverage = %#v", facts.Coverage)
	}
	for _, module := range facts.Modules {
		if module.PackagesCount != 2 || module.RetainedPackagesCount != 2 || module.Coverage.State != CoverageComplete {
			t.Fatalf("module coverage = %#v", module)
		}
	}

	reversed := append([]string(nil), fileList...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := loadForHost(context.Background(), repo, reversed)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("facts depend on tracked-file order:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestLoadWithOptionsUsesOneLinuxTargetForBuildSelectedFiles(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":         "module example.com/target\n\ngo 1.24\n",
		"main_linux.go":  "//go:build linux\n\npackage main\n\nfunc main() {}\nfunc linuxOnly() {}\n",
		"main_darwin.go": "//go:build darwin\n\npackage main\n\nfunc main() {}\nfunc darwinOnly() {}\n",
	}
	fileList := make([]string, 0, len(files))
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}
	facts, err := LoadWithOptions(
		context.Background(), repo, fileList,
		LoadOptions{GoTarget: "linux/amd64"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.EntrypointPackages) != 1 ||
		!containsString(facts.EntrypointPackages[0].GoFiles, "main_linux.go") ||
		containsString(facts.EntrypointPackages[0].GoFiles, "main_darwin.go") {
		t.Fatalf("linux entrypoint files = %#v", facts.EntrypointPackages)
	}
	if len(facts.Packages) != 1 || !facts.Packages[0].DeclarationsScanned ||
		!facts.Packages[0].LoadCompleteness.Complete() ||
		!hasPackageDeclaration(facts.Packages[0].Declarations, PackageDeclarationFunc, "linuxOnly", "") ||
		hasPackageDeclaration(facts.Packages[0].Declarations, PackageDeclarationFunc, "darwinOnly", "") {
		t.Fatalf("linux package declarations = %#v", facts.Packages)
	}
}

func TestLoadFailsClosedOnIncompletePackageGoListAuthority(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/incomplete\n\ngo 1.24\n",
		"api/api.go": `package api
import _ "embed"
//go:embed missing.txt
var Missing string
func Public() {}
`,
	}
	fileList := make([]string, 0, len(files))
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}

	_, err := loadForHost(context.Background(), repo, fileList)
	if err == nil || !strings.Contains(err.Error(), "incomplete go list authority") ||
		!strings.Contains(err.Error(), "example.com/incomplete/api") {
		t.Fatalf("incomplete package error = %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasPackageDeclaration(values []PackageDeclaration, kind PackageDeclarationKind, name, receiver string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Name == name && value.Receiver == receiver {
			return true
		}
	}
	return false
}

func TestLoadZeroCapsRetainAllFacts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/all\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"alpha", "beta", "gamma"} {
		dir := filepath.Join(repo, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		content := "package " + name + "\n"
		if index > 0 {
			content = "package " + name + "\n\nimport _ \"example.com/all/alpha\"\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name+".go"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	facts, err := loadForHost(context.Background(), repo, []string{
		"go.mod", "alpha/alpha.go", "beta/beta.go", "gamma/gamma.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if facts.PackagesCount != 3 || facts.RetainedPackagesCount != 3 || len(facts.InternalEdges) != 2 ||
		facts.Coverage.State != CoverageComplete || facts.Coverage.EdgesRetained != 2 {
		t.Fatalf("zero-cap facts = packages %d/%d edges %d coverage %#v", facts.RetainedPackagesCount, facts.PackagesCount, len(facts.InternalEdges), facts.Coverage)
	}
}

func TestParseGoListOutputMarksIncompleteAndDependencyErrors(t *testing.T) {
	t.Parallel()

	packages, warnings, err := parseGoListOutput(strings.NewReader(`
{"ImportPath":"example.com/app","Dir":"/repo","Name":"app","Incomplete":true,"DepsErrors":[{"Err":"dependency unavailable"}]}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || !packages[0].Incomplete || len(packages[0].DepsErrors) != 1 {
		t.Fatalf("decoded package diagnostics = %#v", packages)
	}
	if len(warnings) != 2 ||
		warnings[0] != "package example.com/app: go list facts are incomplete" ||
		warnings[1] != "package example.com/app dependency: dependency unavailable" {
		t.Fatalf("warnings = %q", warnings)
	}
}

func TestPackageLoadCompletenessFailsClosedOnEveryGoListDiagnostic(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		pkg  goListPackage
		want PackageLoadState
	}{
		{name: "complete", pkg: goListPackage{}, want: PackageLoadComplete},
		{name: "incomplete", pkg: goListPackage{Incomplete: true}, want: PackageLoadIncomplete},
		{name: "package error", pkg: goListPackage{Error: &goListError{Err: "broken"}}, want: PackageLoadIncomplete},
		{name: "dependency error", pkg: goListPackage{DepsErrors: []goListError{{Err: "missing"}}}, want: PackageLoadIncomplete},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := packageLoadCompleteness(test.pkg)
			if got == nil || got.Version != PackageLoadCompletenessVersion || got.State != test.want ||
				got.Complete() != (test.want == PackageLoadComplete) {
				t.Fatalf("package load completeness = %#v, want %q", got, test.want)
			}
		})
	}

	for _, untrusted := range []*PackageLoadCompleteness{
		nil,
		{Version: PackageLoadCompletenessVersion + 1, State: PackageLoadComplete},
		{Version: PackageLoadCompletenessVersion, State: "invented"},
	} {
		if untrusted.Complete() {
			t.Fatalf("unknown package load authority was accepted: %#v", untrusted)
		}
	}
}

func TestLoadCancellationTerminatesGoCommand(t *testing.T) {
	fakeBin := t.TempDir()
	fakeGo := filepath.Join(fakeBin, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/cancel\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := loadForHost(ctx, repo, []string{"go.mod"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Load error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled Go subprocess returned after %v", elapsed)
	}
}

func TestLoadFailsClosedWhenModuleGoModEscapesRepository(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "go.mod"), []byte("module example.com/outside\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside/go.mod", filepath.Join(repo, "go.mod")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := loadForHost(context.Background(), repo, []string{"go.mod", "main.go"})
	if err == nil || !strings.Contains(err.Error(), "load Go facts for module .") ||
		!strings.Contains(err.Error(), "validate go.mod") {
		t.Fatalf("unsafe module error = %v", err)
	}
}

func TestLoadPreservesVersionLookingModulePathsAndPackageDisplay(t *testing.T) {
	t.Parallel()

	for _, modulePath := range []string{"corp.example/platform/v0", "github.com/example/project/v2", "example.com/tool/v10"} {
		modulePath := modulePath
		t.Run(modulePath, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			if err := os.MkdirAll(filepath.Join(repo, "internal", "worker"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.24\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "root.go"), []byte("package platform\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "internal", "worker", "worker.go"), []byte("package worker\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			facts, err := loadForHost(context.Background(), repo, []string{"go.mod", "root.go", "internal/worker/worker.go"})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts.Modules) != 1 || facts.Modules[0].ModulePath != modulePath {
				t.Fatalf("module identity = %#v", facts.Modules)
			}
			byCanonical := make(map[string]PackageFact)
			for _, pkg := range facts.Packages {
				byCanonical[pkg.CanonicalPath] = pkg
			}
			if root := byCanonical[modulePath]; root.DisplayPath != "platform" || root.Locality != "local" {
				t.Fatalf("root package = %#v", root)
			}
			worker := byCanonical[modulePath+"/internal/worker"]
			if worker.DisplayPath != "internal/worker" || worker.ModulePath != modulePath || worker.PackageDir != "internal/worker" ||
				len(worker.Files) != 1 || worker.Files[0] != "internal/worker/worker.go" {
				t.Fatalf("worker package = %#v", worker)
			}
		})
	}
}

func TestLoadAssignsNestedPackagesToExactOwningModule(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "server", "internal", "worker"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.work":                        "go 1.24\n\nuse (\n\t.\n\t./server\n)\n",
		"go.mod":                         "module example.com/foo\n\ngo 1.24\n",
		"root.go":                        "package foo\n",
		"server/go.mod":                  "module example.com/foobar/v2\n\ngo 1.24\n",
		"server/server.go":               "package server\n",
		"server/internal/worker/work.go": "package worker\n",
	}
	var fileList []string
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}
	facts, err := loadForHost(context.Background(), repo, fileList)
	if err != nil {
		t.Fatal(err)
	}
	byDir := make(map[string]PackageFact)
	for _, pkg := range facts.Packages {
		byDir[pkg.PackageDir] = pkg
	}
	worker := byDir["server/internal/worker"]
	if worker.ModulePath != "example.com/foobar/v2" || worker.DisplayPath != "internal/worker" {
		t.Fatalf("nested package ownership = %#v", worker)
	}
}

func TestLoadRecordsOnlyAuthorizedLocalReplacementDirectories(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{filepath.Join(repo, "dep"), outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"go.mod":     "module example.com/root\n\ngo 1.24\n\nreplace example.com/dep => ./dep\nreplace example.com/outside => ../outside\n",
		"root.go":    "package root\n",
		"dep/go.mod": "module example.com/dep\n\ngo 1.24\n",
		"dep/dep.go": "package dep\n",
	}
	var fileList []string
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}
	if err := os.WriteFile(filepath.Join(outside, "go.mod"), []byte("module example.com/outside\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	facts, err := loadForHost(context.Background(), repo, fileList)
	if err != nil {
		t.Fatal(err)
	}
	var root ModuleFact
	for _, module := range facts.Modules {
		if module.ModulePath == "example.com/root" {
			root = module
		}
	}
	if len(root.Replacements) != 2 || !root.Replacements[0].Local || root.Replacements[0].Dir != "dep" ||
		root.Replacements[1].Local || root.Replacements[1].Dir != "" {
		t.Fatalf("replacement provenance = %#v", root.Replacements)
	}
}

func TestDiscoverGoModulesDoesNotEnterExcludedSubmoduleGitlink(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/root\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs := DiscoverGoModules([]string{"go.mod", "deps/platform"}, repo)
	if len(dirs) != 1 || dirs[0] != "." {
		t.Fatalf("discovered modules = %#v", dirs)
	}
}

func TestNormalizePackagePaths(t *testing.T) {
	cases := []struct {
		name           string
		repoRoot       string
		moduleRoot     string
		pkgDir         string
		wantModuleDir  string
		wantModRelDir  string
		wantPackageDir string
	}{
		{
			name:           "server module root package",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd/server",
			pkgDir:         "/tmp/etcd/server",
			wantModuleDir:  "server",
			wantModRelDir:  ".",
			wantPackageDir: "server",
		},
		{
			name:           "etcdctl nested package",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd/etcdctl",
			pkgDir:         "/tmp/etcd/etcdctl/ctlv3/command",
			wantModuleDir:  "etcdctl",
			wantModRelDir:  "ctlv3/command",
			wantPackageDir: "etcdctl/ctlv3/command",
		},
		{
			name:           "root module contrib example",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd",
			pkgDir:         "/tmp/etcd/contrib/raftexample",
			wantModuleDir:  ".",
			wantModRelDir:  "contrib/raftexample",
			wantPackageDir: "contrib/raftexample",
		},
		{
			name:           "root module tools benchmark",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd",
			pkgDir:         "/tmp/etcd/tools/benchmark",
			wantModuleDir:  ".",
			wantModRelDir:  "tools/benchmark",
			wantPackageDir: "tools/benchmark",
		},
		{
			name:           "root module root package",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd",
			pkgDir:         "/tmp/etcd",
			wantModuleDir:  ".",
			wantModRelDir:  ".",
			wantPackageDir: ".",
		},
		{
			name:           "etcdutl module root",
			repoRoot:       "/tmp/etcd",
			moduleRoot:     "/tmp/etcd/etcdutl",
			pkgDir:         "/tmp/etcd/etcdutl",
			wantModuleDir:  "etcdutl",
			wantModRelDir:  ".",
			wantPackageDir: "etcdutl",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md, mrd, pd, err := normalizePackagePaths(tc.repoRoot, tc.moduleRoot, tc.pkgDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if md != tc.wantModuleDir {
				t.Errorf("module_dir = %q, want %q", md, tc.wantModuleDir)
			}
			if mrd != tc.wantModRelDir {
				t.Errorf("module_relative_dir = %q, want %q", mrd, tc.wantModRelDir)
			}
			if pd != tc.wantPackageDir {
				t.Errorf("package_dir = %q, want %q", pd, tc.wantPackageDir)
			}
		})
	}
}

func TestNormalizePackagePathsOutsideRepo(t *testing.T) {
	_, _, _, err := normalizePackagePaths("/tmp/etcd", "/tmp/etcd", "/var/log")
	if err == nil {
		t.Fatal("expected error for package outside repo")
	}
}

func TestDiscoverGoModules(t *testing.T) {
	fileList := []string{
		"go.mod",
		"cmd/app/main.go",
		"internal/pkg/go.mod",
		"internal/pkg/foo.go",
		"tools/helper/go.mod",
		"tools/helper/main.go",
		"README.md",
	}
	mods := DiscoverGoModules(fileList, "/fake/repo")
	if len(mods) != 3 {
		t.Fatalf("got %d modules, want 3: %v", len(mods), mods)
	}
	if mods[0] != "." {
		t.Fatalf("first module = %q, want %q", mods[0], ".")
	}
	if mods[1] != "internal/pkg" {
		t.Fatalf("second module = %q, want %q", mods[1], "internal/pkg")
	}
	if mods[2] != "tools/helper" {
		t.Fatalf("third module = %q, want %q", mods[2], "tools/helper")
	}
}

func TestDiscoverGoModulesNoFiles(t *testing.T) {
	mods := DiscoverGoModules(nil, "/fake/repo")
	if len(mods) != 0 {
		t.Fatalf("got %d modules, want 0: %v", len(mods), mods)
	}
}

func TestLoadWithExactModuleDirDoesNotRequireUnselectedSiblingModuleAuthority(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":               "module example.com/root\n\ngo 1.24\n",
		"cmd/app/main.go":      "package main\n\nfunc main() {}\n",
		"hack/tools/go.mod":    "this is not a valid go.mod\n",
		"hack/tools/broken.go": "package tools\n",
	}
	fileList := make([]string, 0, len(files))
	for name, content := range files {
		filename := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}

	facts, err := LoadWithOptions(context.Background(), repo, fileList, LoadOptions{
		GoTarget:  runtime.GOOS + "/" + runtime.GOARCH,
		ModuleDir: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Modules) != 1 || facts.Modules[0].ModuleDir != "." ||
		facts.Modules[0].ModulePath != "example.com/root" {
		t.Fatalf("explicit module facts = %#v", facts.Modules)
	}
}

func TestSelectModuleDirsRequiresAnExactDiscoveredCanonicalDirectory(t *testing.T) {
	discovered := []string{".", "hack/tools", "staging/client"}
	got, err := selectModuleDirs(discovered, "staging/client")
	if err != nil || len(got) != 1 || got[0] != "staging/client" {
		t.Fatalf("select exact module = %#v, %v", got, err)
	}
	for _, invalid := range []string{"missing", "../escape", "hack/../hack/tools", " hack/tools"} {
		if _, err := selectModuleDirs(discovered, invalid); err == nil {
			t.Errorf("selectModuleDirs accepted %q", invalid)
		}
	}
}

func TestParseGoListOutput(t *testing.T) {
	input := `{"ImportPath": "example.com/foo", "Dir": "/foo", "Name": "foo", "GoFiles": ["foo.go"], "Imports": ["fmt", "example.com/bar"], "Module": {"Path": "example.com"}}
{"ImportPath": "example.com/bar", "Dir": "/bar", "Name": "bar", "GoFiles": ["bar.go"], "Imports": ["strings"]}
`
	pkgs, warnings, err := parseGoListOutput(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if len(warnings) != 0 {
		t.Fatalf("got %d warnings, want 0", len(warnings))
	}
	if pkgs[0].ImportPath != "example.com/foo" {
		t.Fatalf("first package = %q", pkgs[0].ImportPath)
	}
	if pkgs[1].Name != "bar" {
		t.Fatalf("second package Name = %q", pkgs[1].Name)
	}
}

func TestParseGoListOutputWithError(t *testing.T) {
	input := `{"ImportPath": "example.com/broken", "Name": "broken", "Error": {"Err": "build error"}}
{"ImportPath": "example.com/ok", "Dir": "/ok", "Name": "ok", "GoFiles": ["ok.go"], "Module": {"Path": "example.com"}}
`
	pkgs, warnings, err := parseGoListOutput(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if pkgs[0].ImportPath != "example.com/broken" || pkgs[1].ImportPath != "example.com/ok" {
		t.Fatalf("got packages %#v", pkgs)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
}

func TestBuildEntrypoints(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/cmd/app", Dir: "/repo/cmd/app", Name: "main", GoFiles: []string{"main.go"}, Module: &goListModule{Path: "example.com"}},
		{ImportPath: "example.com/lib", Dir: "/repo/lib", Name: "lib", GoFiles: []string{"lib.go"}, Module: &goListModule{Path: "example.com"}},
		{ImportPath: "example.com/internal/util", Dir: "/repo/internal/util", Name: "main", GoFiles: []string{"main.go"}, Module: &goListModule{Path: "example.com"}},
	}
	eps := buildEntrypointCandidates(pkgs, "/repo", "/repo", ".", "example.com")

	if len(eps) != 2 {
		t.Fatalf("entrypoints = %d, want 2", len(eps))
	}
	if eps[0].ModulePath != "example.com" {
		t.Fatalf("module path = %q", eps[0].ModulePath)
	}
	if eps[0].ImportPath != "example.com/cmd/app" {
		t.Fatalf("import path = %q", eps[0].ImportPath)
	}
	if eps[0].Dir != "cmd/app" {
		t.Fatalf("dir = %q, want cmd/app", eps[0].Dir)
	}
	if eps[0].PackageDir != "cmd/app" {
		t.Fatalf("package_dir = %q, want cmd/app", eps[0].PackageDir)
	}
	if eps[0].ModuleRelativeDir != "cmd/app" {
		t.Fatalf("module_relative_dir = %q, want cmd/app", eps[0].ModuleRelativeDir)
	}
	if eps[0].ModuleDir != "." {
		t.Fatalf("module_dir = %q, want .", eps[0].ModuleDir)
	}
}

func TestBuildEntrypointsSubModule(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "go.etcd.io/etcd/etcdctl/v3/ctlv3/command", Dir: "/repo/etcdctl/ctlv3/command", Name: "main", GoFiles: []string{"command.go"}, Module: &goListModule{Path: "go.etcd.io/etcd/etcdctl/v3"}},
	}
	eps := buildEntrypointCandidates(pkgs, "/repo", "/repo/etcdctl", "etcdctl", "go.etcd.io/etcd/etcdctl/v3")

	if len(eps) != 1 {
		t.Fatalf("entrypoints = %d, want 1", len(eps))
	}
	if eps[0].ModulePath != "go.etcd.io/etcd/etcdctl/v3" {
		t.Fatalf("module path = %q", eps[0].ModulePath)
	}
	if eps[0].PackageDir != "etcdctl/ctlv3/command" {
		t.Fatalf("package_dir = %q, want etcdctl/ctlv3/command", eps[0].PackageDir)
	}
	if eps[0].ModuleRelativeDir != "ctlv3/command" {
		t.Fatalf("module_relative_dir = %q, want ctlv3/command", eps[0].ModuleRelativeDir)
	}
	if eps[0].ModuleDir != "etcdctl" {
		t.Fatalf("module_dir = %q, want etcdctl", eps[0].ModuleDir)
	}
}

func TestBuildEntrypointsRootModule(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "go.etcd.io/etcd/v3/contrib/raftexample", Dir: "/repo/contrib/raftexample", Name: "main", GoFiles: []string{"main.go"}, Module: &goListModule{Path: "go.etcd.io/etcd/v3"}},
	}
	eps := buildEntrypointCandidates(pkgs, "/repo", "/repo", ".", "go.etcd.io/etcd/v3")

	if len(eps) != 1 {
		t.Fatalf("entrypoints = %d, want 1", len(eps))
	}
	if eps[0].PackageDir != "contrib/raftexample" {
		t.Fatalf("package_dir = %q, want contrib/raftexample", eps[0].PackageDir)
	}
	if eps[0].ModuleRelativeDir != "contrib/raftexample" {
		t.Fatalf("module_relative_dir = %q, want contrib/raftexample", eps[0].ModuleRelativeDir)
	}
}

func TestBuildEntrypointsServerModule(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "go.etcd.io/etcd/server/v3", Dir: "/repo/server", Name: "main", GoFiles: []string{"main.go"}, Module: &goListModule{Path: "go.etcd.io/etcd/server/v3"}},
	}
	eps := buildEntrypointCandidates(pkgs, "/repo", "/repo/server", "server", "go.etcd.io/etcd/server/v3")

	if len(eps) != 1 {
		t.Fatalf("entrypoints = %d, want 1", len(eps))
	}
	if eps[0].PackageDir != "server" {
		t.Fatalf("package_dir = %q, want server", eps[0].PackageDir)
	}
	if eps[0].ModuleRelativeDir != "." {
		t.Fatalf("module_relative_dir = %q, want .", eps[0].ModuleRelativeDir)
	}
	if eps[0].ModuleDir != "server" {
		t.Fatalf("module_dir = %q, want server", eps[0].ModuleDir)
	}
	if eps[0].Kind != EntrypointKindGoMain {
		t.Fatalf("entrypoint kind = %q, want %q", eps[0].Kind, EntrypointKindGoMain)
	}
}

func TestBuildEntrypointsNoModule(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "main", Dir: "/repo", Name: "main", GoFiles: []string{"main.go"}, Module: nil},
	}
	eps := buildEntrypointCandidates(pkgs, "/repo", "/repo", ".", "")
	if len(eps) != 1 {
		t.Fatalf("entrypoints = %d, want 1", len(eps))
	}
	if eps[0].ModulePath != "" {
		t.Fatalf("module path = %q, want empty", eps[0].ModulePath)
	}
	if eps[0].PackageDir != "." {
		t.Fatalf("package_dir = %q, want .", eps[0].PackageDir)
	}
}

func TestBuildInternalEdges(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/cmd/app", Name: "main", Imports: []string{"example.com/lib"}},
		{ImportPath: "example.com/lib", Name: "lib", Imports: []string{"fmt"}},
		{ImportPath: "example.com/util", Name: "util", Imports: []string{"example.com/lib"}},
	}
	known := buildKnownSet(pkgs)
	edges := buildInternalEdges(pkgs, known)

	if len(edges) != 2 {
		t.Fatalf("internal edges = %d, want 2: %+v", len(edges), edges)
	}

	edgeMap := make(map[string]struct{})
	for _, e := range edges {
		edgeMap[e.From+">"+e.To] = struct{}{}
	}

	if _, ok := edgeMap["example.com/cmd/app>example.com/lib"]; !ok {
		t.Fatalf("missing edge cmd/app -> lib")
	}
	if _, ok := edgeMap["example.com/util>example.com/lib"]; !ok {
		t.Fatalf("missing edge util -> lib")
	}
}

func TestBuildInternalEdgesNone(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/a", Name: "a", Imports: []string{"strings", "os"}},
		{ImportPath: "example.com/b", Name: "b", Imports: []string{"fmt"}},
	}
	known := buildKnownSet(pkgs)
	edges := buildInternalEdges(pkgs, known)
	if len(edges) != 0 {
		t.Fatalf("internal edges = %d, want 0", len(edges))
	}
}

func TestBuildExternalImports(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/a", Name: "a", Imports: []string{"fmt", "os", "example.com/b"}},
		{ImportPath: "example.com/b", Name: "b", Imports: []string{"fmt", "io"}},
		{ImportPath: "example.com/c", Name: "c", Imports: []string{"fmt"}},
	}
	known := buildKnownSet(pkgs)
	ext := buildExternalImports(pkgs, known)

	for _, x := range ext {
		if x.ImportPath == "example.com/b" {
			t.Fatalf("external imports should not contain internal package example.com/b")
		}
	}

	if len(ext) != 3 {
		t.Fatalf("external imports = %d, want 3: %+v", len(ext), ext)
	}

	if ext[0].ImportPath != "fmt" || ext[0].UsedByCount != 3 {
		t.Fatalf("top external import should be fmt with count 3, got: %+v", ext[0])
	}
}

func TestBuildExternalImportsDedup(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/a", Name: "a", Imports: []string{"fmt", "fmt"}},
		{ImportPath: "example.com/b", Name: "b", Imports: []string{"fmt"}},
	}
	known := buildKnownSet(pkgs)
	ext := buildExternalImports(pkgs, known)

	if len(ext) != 1 {
		t.Fatalf("external imports = %d, want 1", len(ext))
	}
	if ext[0].UsedByCount != 2 {
		t.Fatalf("fmt used_by_count = %d, want 2", ext[0].UsedByCount)
	}
}
