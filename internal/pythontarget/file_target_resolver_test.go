package pythontarget

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestFileTargetResolverUsesOnlySealedTargetSourceAuthority(t *testing.T) {
	repository := fixture(t, map[string]string{
		"README.md": "# Example\n",
		"pyproject.toml": `[project]
name = "example"
[project.scripts]
example = "pkg.cli:main"
`,
		"pkg/__init__.py": "",
		"pkg/cli.py":      "def main():\n    return 0\n",
		"pkg/support.py":  "VALUE = 1\n",
	})
	discovered, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewFileTargetResolver(repository, discovered)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		selector string
		paths    []string
		wantKind Kind
		wantRoot RootKind
	}{
		{
			name:     "declared script root",
			selector: "python:.:script:example",
			paths:    []string{"pkg/cli.py"},
			wantKind: KindExecutable,
			wantRoot: RootCallable,
		},
		{
			name:     "library packaging source",
			selector: "python:.:library:library",
			paths:    []string{"pyproject.toml"},
			wantKind: KindLibrary,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := targetBySelector(t, discovered, test.selector)
			for _, filePath := range test.paths {
				fileRef := resolverFileID(t, repository, filePath)
				if !resolver.ResolvesOne(fileRef) {
					t.Fatalf("ResolvesOne(%q) = false", filePath)
				}
				got, err := resolver.ResolveOne(fileRef)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("resolved target = %#v, want exact catalog entry %#v", got, want)
				}
				if err := got.Validate(); err != nil {
					t.Fatalf("resolved target lost its seal: %v", err)
				}
				if got.Kind != test.wantKind {
					t.Fatalf("kind = %q, want %q", got.Kind, test.wantKind)
				}
				if test.wantRoot != "" && (len(got.Roots) != 1 || got.Roots[0].Kind != test.wantRoot) {
					t.Fatalf("roots = %#v, want one %q root", got.Roots, test.wantRoot)
				}
			}
		})
	}

	// Module inventories do not become native hypotheses, but a different cube
	// may select any exact module file and receive a sealed module-execution
	// view instead of being rejected for lack of a framework-specific marker.
	packageRef := resolverFileID(t, repository, "pkg/__init__.py")
	packageView, err := resolver.ResolveOne(packageRef)
	if err != nil || len(packageView.Roots) != 1 || packageView.Roots[0].Kind != RootModuleExecution {
		t.Fatalf("package module view = %#v, %v", packageView, err)
	}

	// support.py is not advertised by native discovery either, but its closed
	// file ref is sufficient resolver authority after README classification.
	supportRef := resolverFileID(t, repository, "pkg/support.py")
	supportView, err := resolver.ResolveOne(supportRef)
	if err != nil || supportView.ScopeRef == "" ||
		len(supportView.Roots) != 1 || supportView.Roots[0].Kind != RootModuleExecution ||
		supportView.Roots[0].Path != "pkg/support.py" {
		t.Fatalf("support module view = %#v, %v", supportView, err)
	}
}

func TestFileTargetResolverProjectsArbitrarySelectedModuleWithoutFrameworkAuthority(t *testing.T) {
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
	wantLibrary := targetBySelector(t, catalog, "python:.:library:library")
	resolver, err := NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}

	appRef := resolverFileID(t, repository, "src/main.py")
	got, err := resolver.ResolveOne(appRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roots) != 1 || got.Roots[0] != (Root{
		Kind: RootModuleExecution, Module: "main", Path: "src/main.py", Line: 1,
	}) || got.ScopeRef == "" || got.Basis[0].Kind != BasisModuleExecutionView {
		t.Fatalf("resolved framework-neutral module target = %#v", got)
	}
	if restored, ok, err := resolver.ResolveSelector(got.Selector); err != nil || !ok || !reflect.DeepEqual(restored, got) {
		t.Fatalf("exact selector did not restore module view: %#v, %v, %v", restored, ok, err)
	}
	wire, err := catalog.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(wire)
	if err != nil {
		t.Fatal(err)
	}
	if restored, ok, err := decoded.ResolveSelector(got.Selector); err != nil || !ok || !reflect.DeepEqual(restored, got) {
		t.Fatalf("catalog selector did not restore persisted module view: %#v, %v, %v", restored, ok, err)
	}
	restoredResolver, err := NewFileTargetResolver(repository, decoded.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	restoredAfterPersistence, err := restoredResolver.ResolveOne(appRef)
	if err != nil || !reflect.DeepEqual(restoredAfterPersistence, got) || !decoded.OwnsTarget(restoredAfterPersistence) {
		t.Fatalf("persisted module-scope authority changed target: %#v, %v", restoredAfterPersistence, err)
	}
	moduleView := got
	manifestRef := resolverFileID(t, repository, "pyproject.toml")
	got, err = resolver.ResolveOne(manifestRef)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantLibrary) {
		t.Fatalf("resolved manifest target = %#v, want exact library target %#v", got, wantLibrary)
	}
	configRef := resolverFileID(t, repository, "src/config.py")
	configView, err := resolver.ResolveOne(configRef)
	if err != nil || len(configView.Roots) != 1 || configView.Roots[0].Path != "src/config.py" {
		t.Fatalf("unadvertised neighboring module view = %#v, %v", configView, err)
	}
	if !reflect.DeepEqual(moduleView.SourceRefs, []corpus.FileID{appRef}) ||
		!reflect.DeepEqual(wantLibrary.SourceRefs, []corpus.FileID{manifestRef}) {
		t.Fatalf(
			"fixture source authority = module %#v, library %#v",
			moduleView.SourceRefs, wantLibrary.SourceRefs,
		)
	}
}

func TestFileTargetResolverSupportsReadmeOnlyProjectWithoutNativeTargets(t *testing.T) {
	repository := fixture(t, map[string]string{
		"README.md":    "Run src/main.py to start the service.\n",
		"src/main.py":  "from runtime import Whatever\napp = Whatever()\n",
		"src/other.py": "VALUE = 1\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 0 || len(catalog.ModuleScopes) != 1 {
		t.Fatalf("plain source discovery = entries %#v, scopes %#v", catalog.Entries, catalog.ModuleScopes)
	}
	candidates, resolver, err := FileCandidatesWithResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("plain modules flooded native hypotheses: %#v", candidates)
	}
	mainRef := resolverFileID(t, repository, "src/main.py")
	mainTarget, err := resolver.ResolveOne(mainRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainTarget.Roots) != 1 || mainTarget.Roots[0].Kind != RootModuleExecution ||
		mainTarget.Roots[0].Path != "src/main.py" || !catalog.OwnsTarget(mainTarget) {
		t.Fatalf("README-addressable module target = %#v", mainTarget)
	}
}

func TestFileTargetResolverOwnsCatalogAndReturnedTargets(t *testing.T) {
	repository := fixture(t, map[string]string{
		"tool.py": "if __name__ == '__main__':\n    pass\n",
	})
	discovered, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	want := targetBySelector(t, discovered, "python:.:guard:tool")
	catalog := discovered.Snapshot()
	resolver, err := NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the caller's catalog after construction must not rewrite the
	// resolver's sealed authority.
	catalog.Entries[0].Modules[0].Path = "invented.py"
	catalog.Entries[0].Roots[0].Path = "invented.py"
	fileRef := resolverFileID(t, repository, "tool.py")
	first, err := resolver.ResolveOne(fileRef)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("caller mutation changed resolver target: %#v", first)
	}

	// The result itself is independently owned as well.
	first.Modules[0].Path = "also-invented.py"
	first.Roots[0].Path = "also-invented.py"
	second, err := resolver.ResolveOne(fileRef)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("returned mutation changed resolver target: %#v", second)
	}
}

func TestFileTargetResolverRejectsSharedFileAmbiguity(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": `[project]
name = "commands"
[project.scripts]
first = "pkg.cli:main"
second = "pkg.cli:main"
`,
		"pkg/__init__.py": "",
		"pkg/cli.py":      "def main():\n    return 0\n",
	})
	discovered, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewFileTargetResolver(repository, discovered)
	if err != nil {
		t.Fatal(err)
	}

	fileRef := resolverFileID(t, repository, "pkg/cli.py")
	all, err := resolver.Resolve([]corpus.FileID{fileRef, fileRef})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Selector != "python:.:script:first" ||
		all[1].Selector != "python:.:script:second" {
		t.Fatalf("shared source targets = %#v", all)
	}
	if resolver.ResolvesOne(fileRef) {
		t.Fatal("shared source was advertised as an unambiguous target")
	}
	if _, err := resolver.ResolveOne(fileRef); err == nil ||
		!strings.Contains(err.Error(), "maps to 2 Python targets") {
		t.Fatalf("ambiguity error = %v", err)
	}
}

func TestFileTargetResolverDistinguishesUnknownAndUnmatchedCorpusRefs(t *testing.T) {
	repository := fixture(t, map[string]string{
		"README.md": "# Tool\n",
		"tool.py":   "if __name__ == '__main__':\n    pass\n",
	})
	discovered, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewFileTargetResolver(
		repository,
		discovered,
	)
	if err != nil {
		t.Fatal(err)
	}

	readmeRef := resolverFileID(t, repository, "README.md")
	if _, err := resolver.ResolveOne(readmeRef); err == nil ||
		!strings.Contains(err.Error(), "has no exact Python target") {
		t.Fatalf("unmatched corpus ref error = %v", err)
	}
	if _, err := resolver.ResolveOne("f999"); err == nil ||
		!strings.Contains(err.Error(), "unknown file_ref") {
		t.Fatalf("unknown corpus ref error = %v", err)
	}
	if resolver.ResolvesOne(readmeRef) || resolver.ResolvesOne("f999") ||
		(FileTargetResolver{}).ResolvesOne(resolverFileID(t, repository, "tool.py")) {
		t.Fatal("ResolvesOne accepted unmatched, unknown, or uninitialized authority")
	}
	if _, err := (FileTargetResolver{}).ResolveOne(resolverFileID(t, repository, "tool.py")); err == nil ||
		!strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("zero-value resolver error = %v", err)
	}
}

func TestFileTargetResolverRejectsCatalogFromDifferentCorpusIdentity(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pkg/__init__.py": "",
		"pyproject.toml":  "[project]\nname = \"pkg\"\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	shiftedCorpus := fixture(t, map[string]string{
		"000-before.txt":  "changes every following FileID\n",
		"pkg/__init__.py": "",
		"pyproject.toml":  "[project]\nname = \"pkg\"\n",
	})

	if _, err := NewFileTargetResolver(shiftedCorpus, catalog); err == nil ||
		!strings.Contains(err.Error(), "corpus binding") {
		t.Fatalf("different corpus identity error = %v", err)
	}
	if _, err := NewFileTargetResolver(nil, catalog); err == nil ||
		!strings.Contains(err.Error(), "repository corpus is required") {
		t.Fatalf("nil corpus error = %v", err)
	}
}

func resolverFileID(t *testing.T, repository *corpus.Corpus, filePath string) corpus.FileID {
	t.Helper()
	fileRef, ok := repository.ID(filePath)
	if !ok {
		t.Fatalf("fixture has no corpus ref for %q", filePath)
	}
	return fileRef
}
