package gofacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkipsModuleWhoseGoModEscapesRepository(t *testing.T) {
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

	facts, err := Load(context.Background(), repo, []string{"go.mod", "main.go"}, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if facts.PackagesCount != 0 {
		t.Fatalf("packages count = %d, want 0; unsafe module was analyzed", facts.PackagesCount)
	}
	if len(facts.Modules) != 1 {
		t.Fatalf("modules = %d, want 1", len(facts.Modules))
	}
	if len(facts.Modules[0].Warnings) != 1 {
		t.Fatalf("module warnings = %q, want one", facts.Modules[0].Warnings)
	}
	if warning := facts.Modules[0].Warnings[0]; !strings.Contains(warning, "unsafe go.mod") || !strings.Contains(warning, "skipping go list") {
		t.Fatalf("module warning = %q, want unsafe go.mod skip warning", warning)
	}
	if len(facts.Warnings) != 1 || !strings.Contains(facts.Warnings[0], "module .: unsafe go.mod") {
		t.Fatalf("top-level warnings = %q, want unsafe root-module warning", facts.Warnings)
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
			facts, err := Load(context.Background(), repo, []string{"go.mod", "root.go", "internal/worker/worker.go"}, 20, 20)
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
	facts, err := Load(context.Background(), repo, fileList, 20, 20)
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
	facts, err := Load(context.Background(), repo, fileList, 20, 20)
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

func TestExtractModulePath(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "example.com/lib", Module: &goListModule{Path: "example.com"}},
		{ImportPath: "example.com/cmd", Module: &goListModule{Path: "example.com"}},
	}
	if got := extractModulePath(pkgs); got != "example.com" {
		t.Fatalf("module path = %q, want %q", got, "example.com")
	}
}

func TestExtractModulePathEmpty(t *testing.T) {
	pkgs := []goListPackage{
		{ImportPath: "stdlib", Module: nil},
	}
	if got := extractModulePath(pkgs); got != "" {
		t.Fatalf("module path = %q, want empty", got)
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

func TestClassifyEntrypoint(t *testing.T) {
	cases := []struct {
		importPath string
		packageDir string
		moduleDir  string
		want       string
	}{
		{"go.etcd.io/etcd/server/v3", "server", "server", "primary_binary"},
		{"go.etcd.io/etcd/etcdctl/v3", "etcdctl", "etcdctl", "cli"},
		{"go.etcd.io/etcd/etcdutl/v3", "etcdutl", "etcdutl", "tool"},
		{"go.etcd.io/etcd/v3/tools/benchmark", "tools/benchmark", ".", "tool"},
		{"go.etcd.io/etcd/v3/contrib/raftexample", "contrib/raftexample", ".", "example"},
		{"go.etcd.io/etcd/v3/tests/functional", "tests/functional", ".", "test_binary"},
		{"example.com/unknown/module", "unknown/module", "unknown", "unknown"},
		{"go.etcd.io/etcd/v3/tools/benchmark", "tools/benchmark", "tools/mod", "tool"},
	}
	for _, tc := range cases {
		got := classifyEntrypoint(tc.importPath, tc.packageDir, tc.moduleDir)
		if got != tc.want {
			t.Errorf("classifyEntrypoint(%q, %q, %q) = %q, want %q", tc.importPath, tc.packageDir, tc.moduleDir, got, tc.want)
		}
	}
}

func TestGuessModuleRole(t *testing.T) {
	cases := []struct {
		moduleDir string
		want      string
	}{
		{"server", "server_runtime"},
		{"api", "api_definitions"},
		{"tests", "tests"},
		{"tools/mod", "tools"},
		{"pkg", "shared_library"},
		{".", "repository_root"},
		{"clientv3", "client_library"},
		{"etcdctl", "unknown"},
		{"mystery", "unknown"},
	}
	for _, tc := range cases {
		got := guessModuleRole(tc.moduleDir)
		if got != tc.want {
			t.Errorf("guessModuleRole(%q) = %q, want %q", tc.moduleDir, got, tc.want)
		}
	}
}

func TestOpenFilesGeneration(t *testing.T) {
	eps := []Entrypoint{
		{ImportPath: "go.etcd.io/etcd/server/v3", PackageDir: "server", GoFiles: []string{"main.go"}, Kind: "primary_binary"},
		{ImportPath: "go.etcd.io/etcd/etcdctl/v3", PackageDir: "etcdctl", GoFiles: []string{"main.go"}, Kind: "cli"},
		{ImportPath: "go.etcd.io/etcd/etcdutl/v3", PackageDir: "etcdutl", GoFiles: []string{"main.go"}, Kind: "tool"},
		{ImportPath: "go.etcd.io/etcd/v3/contrib/raftexample", PackageDir: "contrib/raftexample", GoFiles: []string{"main.go"}, Kind: "example"},
		{ImportPath: "go.etcd.io/etcd/v3/tools/benchmark", PackageDir: "tools/benchmark", GoFiles: []string{"main.go"}, Kind: "tool"},
	}
	candidates := buildOrientationCandidates(eps)

	if len(candidates) != 5 {
		t.Fatalf("candidates = %d, want 5", len(candidates))
	}

	wantFiles := map[string]string{
		"go.etcd.io/etcd/server/v3":              "server/main.go",
		"go.etcd.io/etcd/etcdctl/v3":             "etcdctl/main.go",
		"go.etcd.io/etcd/etcdutl/v3":             "etcdutl/main.go",
		"go.etcd.io/etcd/v3/contrib/raftexample": "contrib/raftexample/main.go",
		"go.etcd.io/etcd/v3/tools/benchmark":     "tools/benchmark/main.go",
	}

	for _, c := range candidates {
		want, ok := wantFiles[c.Name]
		if !ok {
			t.Fatalf("unexpected candidate: %s", c.Name)
		}
		if len(c.OpenFiles) != 1 || c.OpenFiles[0] != want {
			t.Errorf("candidate %s: open_files = %v, want [%s]", c.Name, c.OpenFiles, want)
		}
	}
}

func TestPriorityForKind(t *testing.T) {
	if priorityForKind("primary_binary") <= priorityForKind("cli") {
		t.Fatal("primary_binary should have higher priority than cli")
	}
	if priorityForKind("cli") <= priorityForKind("tool") {
		t.Fatal("cli should have higher priority than tool")
	}
	if priorityForKind("tool") <= priorityForKind("example") {
		t.Fatal("tool should have higher priority than example")
	}
	if priorityForKind("example") <= priorityForKind("test_binary") {
		t.Fatal("example should have higher priority than test_binary")
	}
	if priorityForKind("test_binary") <= priorityForKind("unknown") {
		t.Fatal("test_binary should have higher priority than unknown")
	}
}

func TestSignalFlowKindUsesOperationalDefaults(t *testing.T) {
	t.Parallel()

	if priority := priorityForKind("signal_flow"); priority != 2 {
		t.Fatalf("priority = %d, want 2", priority)
	}
	if why := whyForKind("signal_flow"); why != "operational flow discovered from source signals" {
		t.Fatalf("why = %q", why)
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
