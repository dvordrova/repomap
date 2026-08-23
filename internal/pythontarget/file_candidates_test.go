package pythontarget

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
)

func TestFileCandidatesProjectsAndMergesExactExecutableAndLibraryFiles(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": `[project]
name = "acme"
[project.scripts]
acme = "acme.cli:main"
`,
		"acme/__init__.py": "",
		"acme/cli.py": `def main():
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
`,
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	got, err := FileCandidates(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	want := candidatePaths(t, repository, got)
	if !reflect.DeepEqual(want, map[string][]string{
		"pyproject.toml": {
			pythonDeclaredPackageHypothesis,
			pythonPublicPackageHypothesis,
		},
		"acme/cli.py": {
			pythonMainGuardHypothesis,
			pythonCallableHypothesis,
			pythonConsoleScriptHypothesis,
		},
	}) {
		t.Fatalf("candidates = %#v", want)
	}
	if len(got) != 2 {
		t.Fatalf("duplicate file refs were not merged: %#v", got)
	}
}

func TestFileCandidatesUsesNamespaceLibraryPackagingSource(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": `[project]
name = "plugins"
[tool.hatch.build.targets.wheel]
packages = ["beetsplug"]
`,
		"beetsplug/plugin.py": "def register():\n    return None\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	library := targetBySelector(t, catalog, "python:.:library:library")
	if len(library.Packages) != 1 || !library.Packages[0].Namespace || library.Packages[0].Path != "" {
		t.Fatalf("fixture did not produce a namespace package: %#v", library.Packages)
	}

	got, err := FileCandidates(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if paths := candidatePaths(t, repository, got); !reflect.DeepEqual(paths, map[string][]string{
		"pyproject.toml": {
			pythonDeclaredPackageHypothesis,
			pythonPublicPackageHypothesis,
		},
	}) {
		t.Fatalf("namespace package candidates = %#v", paths)
	}
}

func TestFileCandidatesProjectsOneExactLibrarySourceWithoutModuleFanout(t *testing.T) {
	files := map[string]string{
		"pyproject.toml": "[project]\nname = \"many-packages\"\n",
	}
	const packageCount = 96
	for index := 0; index < packageCount; index++ {
		files[fmt.Sprintf("pkg%03d/__init__.py", index)] = ""
	}
	repository := fixture(t, files)
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	got, err := FileCandidates(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want one exact packaging source", len(got))
	}
	manifestRef, ok := repository.ID("pyproject.toml")
	if !ok || got[0].FileRef != manifestRef || !reflect.DeepEqual(got[0].Hypotheses, []string{
		pythonDeclaredPackageHypothesis,
		pythonPublicPackageHypothesis,
	}) {
		t.Fatalf("candidate = %#v", got[0])
	}
}

func TestFileCandidatesDoNotAdvertiseResolverOnlyModuleViews(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": `[tool.poetry]
name = "web"
version = "0.1.0"
`,
		"src/__init__.py": "",
		"src/config.py":   "VALUE = 1\n",
		"src/main.py": `from runtime import Whatever

app = Whatever()
`,
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := FileCandidates(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	paths := candidatePaths(t, repository, candidates)
	if !reflect.DeepEqual(paths, map[string][]string{
		"pyproject.toml": {
			pythonDeclaredPackageHypothesis,
			pythonPublicPackageHypothesis,
		},
	}) {
		t.Fatalf("native candidates include resolver-only module views: %#v", paths)
	}
}

func TestExecutableRootAndBasisHypothesesArePlainAndExhaustive(t *testing.T) {
	rootCases := map[RootKind]string{
		RootCallable:    pythonCallableHypothesis,
		RootModule:      pythonModuleHypothesis,
		RootMainGuard:   pythonMainGuardHypothesis,
		RootScriptFile:  pythonExecutableScriptHypothesis,
		RootBoundObject: pythonBoundObjectHypothesis,
	}
	if _, err := executableRootHypothesis(RootModuleExecution); err == nil {
		t.Fatal("resolver-only module execution became a native root hypothesis")
	}
	for kind, want := range rootCases {
		got, err := executableRootHypothesis(kind)
		if err != nil || got != want {
			t.Errorf("root %q = %q, %v; want %q", kind, got, err, want)
		}
	}

	bases := []Basis{
		{Kind: BasisPEP621Script},
		{Kind: BasisPEP621GUIScript},
		{Kind: BasisPoetryScript},
		{Kind: BasisSetupCFGScript},
		{Kind: BasisSetupCFGGUIScript},
		{Kind: BasisSetupPYScript},
		{Kind: BasisSetupPYGUIScript},
		{Kind: BasisPackageMain},
		{Kind: BasisNameMainGuard},
		{Kind: BasisPythonShebang},
		{Kind: BasisModuleExecutionView},
		{Kind: BasisImportPackage},
	}
	got, err := executableBasisHypotheses(bases)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		pythonConsoleScriptHypothesis,
		pythonGUIScriptHypothesis,
		pythonPackageMainHypothesis,
		pythonMainGuardHypothesis,
		pythonExecutableScriptHypothesis,
		pythonDeclaredPackageHypothesis,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("basis hypotheses = %#v, want %#v", got, want)
	}
	for _, hypothesis := range append(append([]string(nil), got...), rootCaseValues(rootCases)...) {
		if strings.Contains(hypothesis, "pyt-") || strings.Contains(hypothesis, "anchor") ||
			strings.Contains(hypothesis, "claim") {
			t.Fatalf("internal identity leaked into hypothesis %q", hypothesis)
		}
	}
}

func TestFileCandidatesRejectsNilCorpus(t *testing.T) {
	_, err := FileCandidates(nil, Catalog{})
	if err == nil || !strings.Contains(err.Error(), "repository corpus is required") {
		t.Fatalf("error = %v", err)
	}
}

func candidatePaths(
	t *testing.T,
	repository *corpus.Corpus,
	candidates []analysistarget.FileCandidate,
) map[string][]string {
	t.Helper()
	result := make(map[string][]string, len(candidates))
	for _, candidate := range candidates {
		info, ok := repository.Info(candidate.FileRef)
		if !ok {
			t.Fatalf("unknown file ref %q", candidate.FileRef)
		}
		result[info.Entry.Path] = append([]string(nil), candidate.Hypotheses...)
	}
	return result
}

func rootCaseValues(values map[RootKind]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
