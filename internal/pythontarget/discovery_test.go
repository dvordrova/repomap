package pythontarget

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestDiscoverPackagingRootsCorpusRefsAndDeterminism(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": `[project]
name = "acme"
[project.scripts]
api = "acme:main"
worker = "acme.cli:worker"
[tool.setuptools.package-dir]
"" = "src"
`,
		"src/acme/__init__.py": "from .cli import main\n",
		"src/acme/cli.py":      "def main():\n    return 0\n\ndef worker():\n    return 1\n",
		"src/acme/__main__.py": "from .cli import main\nmain()\n",
		"tests/test_cli.py":    "if __name__ == '__main__':\n    raise SystemExit(1)\n",
	})

	first, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	left, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("discovery is not deterministic:\n%s\n%s", left, right)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("catalog validation: %v", err)
	}
	if first.Coverage != CoverageComplete {
		t.Fatalf("coverage = %q, omissions = %#v", first.Coverage, first.Omissions)
	}

	api := targetBySelector(t, first, "python:.:script:api")
	if len(api.Roots) != 1 || api.Roots[0].Module != "acme.cli" || api.Roots[0].Qualname != "main" ||
		api.Roots[0].Path != "src/acme/cli.py" {
		t.Fatalf("re-export root was not restored exactly: %#v", api.Roots)
	}
	sourceID, ok := repository.ID("src/acme/cli.py")
	if !ok || len(api.SourceRefs) != 1 || api.SourceRefs[0] != sourceID || api.AnchorFileRef != sourceID {
		t.Fatalf("API source/anchor refs = %#v/%q, want %q", api.SourceRefs, api.AnchorFileRef, sourceID)
	}
	manifestID, ok := repository.ID("pyproject.toml")
	if !ok || len(api.Basis) != 1 || api.Basis[0].FileID != manifestID {
		t.Fatalf("API basis is not bound to the manifest corpus file: %#v", api.Basis)
	}
	if targetBySelector(t, first, "python:.:script:worker").Roots[0].Qualname != "worker" {
		t.Fatal("worker callable was not restored")
	}
	module := targetBySelector(t, first, "python:.:module:acme")
	if module.Roots[0].Kind != RootModule || module.Roots[0].Path != "src/acme/__main__.py" {
		t.Fatalf("module root = %#v", module.Roots)
	}
	library := targetBySelector(t, first, "python:.:library:library")
	if library.AnchorFileRef != manifestID || len(library.SourceRefs) != 1 || library.SourceRefs[0] != manifestID {
		t.Fatalf("library manifest authority = %q/%#v", library.AnchorFileRef, library.SourceRefs)
	}

	var restored Catalog
	if err := json.Unmarshal(left, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("canonical JSON did not restore a valid catalog: %v", err)
	}
}

func TestDiscoverKeepsArbitraryObjectsInSealedModuleScopeWithoutAdvertisingTargets(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"web\"\n",
		"web/__init__.py": "",
		"web/main.py": `from runtime import Whatever

app = Whatever()
not_app = object()
`,
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.ModuleScopes) != 1 || len(catalog.ModuleScopes[0].Modules) != 2 {
		t.Fatalf("module scopes = %#v", catalog.ModuleScopes)
	}
	for _, target := range catalog.Entries {
		if strings.Contains(target.Selector, "application") || target.AnchorFileRef == resolverFileID(t, repository, "web/main.py") {
			t.Fatalf("arbitrary object became native target authority: %#v", target)
		}
	}
}

func TestDiscoverRequirementsOnlyScopesIndependentGuards(t *testing.T) {
	repository := fixture(t, map[string]string{
		"services/api/requirements.txt": "runtime==1.0\n",
		"services/api/main.py":          "if __name__ == '__main__':\n    pass\n",
		"tools/check.py":                "if __name__ == '__main__':\n    pass\n",
		"web/package.json":              "{}\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	api := targetBySelector(t, catalog, "python:services/api:guard:main")
	root := targetBySelector(t, catalog, "python:.:guard:tools/check")
	if api.ProjectDir != "services/api" || len(api.SourceRoots) != 1 || api.SourceRoots[0] != "services/api" {
		t.Fatalf("requirements scope = %#v", api)
	}
	if root.ProjectDir != "." || len(root.Modules) != 1 || root.Modules[0].Path != "tools/check.py" {
		t.Fatalf("synthetic mixed-repository scope = %#v", root)
	}
}

func TestDiscoverPythonShebangUsesCorpusExecutableModeAndAnchor(t *testing.T) {
	repository := fixtureWithExecutables(t, map[string]string{
		"bin/plain.py":     "#!/usr/bin/env python3\nprint('ready')\n",
		"bin/guarded.py":   "#!/opt/venv/bin/python3.12\nif __name__ == '__main__':\n    pass\n",
		"bin/tool":         "#!/usr/bin/env -S python3 -u\nprint('extensionless')\n",
		"bin/not-python":   "#!/bin/sh\nexit 0\n",
		"bin/lookalike.py": "#!/usr/bin/env python-tool\nprint('not exact')\n",
	}, []string{"bin/not-python", "bin/tool"})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	plain := targetBySelector(t, catalog, "python:.:script-file:bin/plain")
	if plain.Roots[0].Kind != RootScriptFile ||
		!hasBasisKind(plain.Basis, BasisPythonShebang) {
		t.Fatalf("plain shebang target = %#v", plain)
	}
	guarded := targetBySelector(t, catalog, "python:.:guard:bin/guarded")
	if !hasBasisKind(guarded.Basis, BasisPythonShebang) || !hasBasisKind(guarded.Basis, BasisNameMainGuard) {
		t.Fatalf("guard/shebang basis did not merge: %#v", guarded.Basis)
	}
	tool := targetBySelector(t, catalog, "python:.:script-file:bin/tool")
	toolID, ok := repository.ID("bin/tool")
	if !ok || len(tool.SourceRefs) != 1 || tool.SourceRefs[0] != toolID || tool.AnchorFileRef != toolID ||
		len(tool.Basis) != 1 || tool.Basis[0].FileID != toolID {
		t.Fatalf("extensionless executable provenance = refs %#v anchor %q basis %#v", tool.SourceRefs, tool.AnchorFileRef, tool.Basis)
	}
	for _, target := range catalog.Entries {
		if strings.Contains(target.Selector, "not-python") || strings.Contains(target.Selector, "lookalike") ||
			target.Selector == "python:.:script-file:bin/guarded" {
			t.Fatalf("invented executable target: %#v", target)
		}
	}
}

func TestDiscoverOpenedCorpusRetainsGitExecutableMode(t *testing.T) {
	repo := t.TempDir()
	toolPath := filepath.Join(repo, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolPath, []byte("#!/usr/bin/env python3\nprint('ready')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--quiet", repo},
		{"-C", repo, "add", "--", "bin/tool"},
		{"-C", repo, "update-index", "--chmod=+x", "--", "bin/tool"},
	} {
		command := exec.Command("git", args...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	repository, err := corpus.Open(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	target := targetBySelector(t, catalog, "python:.:script-file:bin/tool")
	wantID, ok := repository.ID("bin/tool")
	if !ok || target.AnchorFileRef != wantID || target.Basis[0].FileID != wantID {
		t.Fatalf("opened corpus executable provenance = %#v", target)
	}
}

func TestTargetRejectsTamperedAnchorAndDynamicFactsRemainOmissions(t *testing.T) {
	repository := fixture(t, map[string]string{
		"setup.py": `from setuptools import setup
POINTS = {"console_scripts": ["stale = pkg.cli:main"]}
POINTS = load_points()
DELETED = {"console_scripts": ["deleted = pkg.cli:main"]}
del DELETED
setup(name="demo", packages=["pkg"], entry_points=POINTS)
setup(name="demo", packages=["pkg"], entry_points=DELETED)
`,
		"pkg/__init__.py": "",
		"app.py":          "if __name__ == '__main__':\n    pass\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Coverage != CoveragePartial || !hasOmissionKind(catalog.Omissions, OmissionDynamicSetup) {
		t.Fatalf("dynamic setup.py did not remain an explicit omission: %#v", catalog)
	}
	application := targetBySelector(t, catalog, "python:.:guard:app")
	otherID, ok := repository.ID("pkg/__init__.py")
	if !ok {
		t.Fatal("package file missing from corpus")
	}
	tampered := application
	tampered.AnchorFileRef = otherID
	tampered.Ref, err = targetRef(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("tampered anchor validated: %v", err)
	}
}

func fixture(t *testing.T, files map[string]string) *corpus.Corpus {
	return fixtureWithExecutables(t, files, nil)
}

func fixtureWithExecutables(t *testing.T, files map[string]string, executablePaths []string) *corpus.Corpus {
	t.Helper()
	repo := t.TempDir()
	paths := make([]string, 0, len(files))
	for filePath, content := range files {
		absolute := filepath.Join(repo, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	repository, err := corpus.New(context.Background(), repo, gitfiles.Listing{
		Paths: paths, RegularPaths: paths, ExecutablePaths: executablePaths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}

func hasBasisKind(values []Basis, kind BasisKind) bool {
	for _, value := range values {
		if value.Kind == kind {
			return true
		}
	}
	return false
}

func hasOmissionKind(values []Omission, kind OmissionKind) bool {
	for _, value := range values {
		if value.Kind == kind {
			return true
		}
	}
	return false
}

func targetBySelector(t *testing.T, catalog Catalog, selector string) Target {
	t.Helper()
	for _, target := range catalog.Entries {
		if target.Selector == selector {
			return target
		}
	}
	t.Fatalf("missing %q in %#v", selector, selectors(catalog))
	return Target{}
}

func selectors(catalog Catalog) []string {
	values := make([]string, 0, len(catalog.Entries))
	for _, target := range catalog.Entries {
		values = append(values, target.Selector)
	}
	return values
}
