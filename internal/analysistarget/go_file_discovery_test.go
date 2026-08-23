package analysistarget

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestDiscoverGoTargetFilesProjectsExactMainsAndPublicDeclarations(t *testing.T) {
	fixture := newGoFileDiscoveryFixture(t)

	got, err := DiscoverGoTargetFiles(fixture.repository, fixture.facts, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileCandidate{
		{
			FileRef: fixture.id(t, "api/public.go"),
			Hypotheses: []string{
				goExportedFuncHypothesis,
				goExportedMethodHypothesis,
			},
		},
		{
			FileRef: fixture.id(t, "api/types.go"),
			Hypotheses: []string{
				goExportedConstHypothesis,
				goExportedTypeHypothesis,
				goExportedVarHypothesis,
			},
		},
		{
			FileRef:    fixture.id(t, "cmd/app/main.go"),
			Hypotheses: []string{goMainFileHypothesis},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}

	// The exact equality above also proves that two exported functions in
	// public.go and two duplicate main anchors do not duplicate file rows or
	// their plain hypotheses.
	for _, candidate := range got {
		if candidate.FileRef == fixture.id(t, "internal/hidden/hidden.go") {
			t.Fatalf("internal package became a public target candidate: %#v", candidate)
		}
	}
}

func TestGoFileTargetResolverRestoresPrivateCanonicalTargetRefs(t *testing.T) {
	fixture := newGoFileDiscoveryFixture(t)
	resolver, err := NewGoFileTargetResolver(fixture.repository, fixture.facts, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}

	libraryRef := fixture.targetRef(t, KindModuleLibrary)
	executableRef := fixture.targetRef(t, KindExecutablePackage)
	implementationID := fixture.id(t, "api/implementation.go")
	runID := fixture.id(t, "cmd/app/run.go")

	// Package membership is not target-entry authority. Files omitted by
	// GoTargetDiscovery must not let a README hypothesis restore a neighboring
	// exact target.
	for _, fileRef := range []corpus.FileID{runID, implementationID} {
		if resolver.ResolvesOne(fileRef) {
			t.Fatalf("neighboring implementation %q became target authority", fileRef)
		}
		if _, err := resolver.ResolveOne(fileRef); err == nil ||
			!strings.Contains(err.Error(), "has no exact Go target") {
			t.Fatalf("neighboring implementation error = %v", err)
		}
	}
	if got, err := resolver.ResolveOne(fixture.id(t, "cmd/app/main.go")); err != nil || got != executableRef {
		t.Fatalf("main target = %q, %v; want %q", got, err, executableRef)
	}
	if got, err := resolver.ResolveOne(fixture.id(t, "api/public.go")); err != nil || got != libraryRef {
		t.Fatalf("library target = %q, %v; want %q", got, err, libraryRef)
	}
	if !resolver.ResolvesOne(fixture.id(t, "api/types.go")) || resolver.ResolvesOne(fixture.id(t, "README.md")) ||
		(GoFileTargetResolver{}).ResolvesOne(implementationID) {
		t.Fatal("ResolvesOne did not preserve exact initialized target authority")
	}

	for _, fileRefs := range [][]corpus.FileID{
		nil,
		{"f999"},
		{fixture.id(t, "README.md")},
	} {
		if _, err := resolver.Resolve(fileRefs); err == nil {
			t.Fatalf("Resolve(%q) accepted an invalid selection", fileRefs)
		}
	}
	if _, err := (GoFileTargetResolver{}).ResolveOne(implementationID); err == nil {
		t.Fatal("zero-value resolver was accepted")
	}

	candidates, err := DiscoverGoTargetFiles(fixture.repository, fixture.facts, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}
	for _, targetRef := range fixture.catalogTargetRefs() {
		if strings.Contains(string(wire), targetRef) {
			t.Fatalf("provider candidate wire leaked target ref %q: %s", targetRef, wire)
		}
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(wire, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row) != 2 || row["file_ref"] == nil || row["hypotheses"] == nil {
			t.Fatalf("candidate wire has fields outside file_ref+hypotheses: %s", wire)
		}
	}
}

func TestDiscoverGoTargetFilesIsPermutationStable(t *testing.T) {
	fixture := newGoFileDiscoveryFixture(t)
	first, err := DiscoverGoTargetFiles(fixture.repository, fixture.facts, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}

	permuted := fixture.facts
	slices.Reverse(permuted.Packages)
	for index := range permuted.Packages {
		slices.Reverse(permuted.Packages[index].Declarations)
		slices.Reverse(permuted.Packages[index].Files)
	}
	slices.Reverse(permuted.EntrypointPackages)
	permutedCatalog, err := BuildCatalog(permuted)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DiscoverGoTargetFiles(fixture.repository, permuted, permutedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection changed under fact permutation: %#v != %#v", first, second)
	}
}

func TestDiscoverGoTargetFilesFailsClosedWithoutExactSourceAuthority(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*goFileDiscoveryFixture)
		want   string
	}{
		{
			name: "exported declaration has no location",
			mutate: func(fixture *goFileDiscoveryFixture) {
				pkg := fixture.packageFact("example.com/product/api")
				pkg.Declarations[0].Path = ""
				pkg.Declarations[0].Line = 0
				pkg.Declarations[0].Column = 0
			},
			want: "has no exact source location",
		},
		{
			name: "exported declaration is outside package",
			mutate: func(fixture *goFileDiscoveryFixture) {
				pkg := fixture.packageFact("example.com/product/api")
				pkg.Declarations[0].Path = "cmd/app/main.go"
			},
			want: "is outside package",
		},
		{
			name: "main root is absent from corpus",
			mutate: func(fixture *goFileDiscoveryFixture) {
				fixture.facts.EntrypointPackages[0].Anchors[0].Path = "cmd/app/missing.go"
				mainPackage := fixture.packageFact("example.com/product/cmd/app")
				mainPackage.Files = append(mainPackage.Files, "cmd/app/missing.go")
				fixture.catalog = mustGoFileDiscoveryCatalog(fixture.t, fixture.facts)
			},
			want: "is absent from the corpus",
		},
		{
			name: "exported declaration file is not build selected",
			mutate: func(fixture *goFileDiscoveryFixture) {
				fixture.packageFact("example.com/product/api").Declarations[0].Path = "api/ignored.go"
			},
			want: "is not a build-selected package file",
		},
		{
			name: "main root file is not build selected",
			mutate: func(fixture *goFileDiscoveryFixture) {
				fixture.facts.EntrypointPackages[0].Anchors[0].Path = "cmd/app/ignored.go"
				fixture.catalog = mustGoFileDiscoveryCatalog(fixture.t, fixture.facts)
			},
			want: "is not a build-selected package file",
		},
		{
			name: "main root is not Go source",
			mutate: func(fixture *goFileDiscoveryFixture) {
				fixture.facts.EntrypointPackages[0].Anchors[0].Path = "cmd/app/main.s"
				mainPackage := fixture.packageFact("example.com/product/cmd/app")
				mainPackage.Files = append(mainPackage.Files, "cmd/app/main.s")
				fixture.catalog = mustGoFileDiscoveryCatalog(fixture.t, fixture.facts)
			},
			want: "is not a Go source file",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGoFileDiscoveryFixture(t)
			test.mutate(fixture)
			if test.name != "main root is absent from corpus" &&
				test.name != "main root file is not build selected" &&
				test.name != "main root is not Go source" {
				fixture.catalog = mustGoFileDiscoveryCatalog(t, fixture.facts)
			}
			_, err := DiscoverGoTargetFiles(fixture.repository, fixture.facts, fixture.catalog)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGoFileTargetResolverRejectsPackageFileOwnershipDrift(t *testing.T) {
	fixture := newGoFileDiscoveryFixture(t)
	apiPackage := fixture.packageFact("example.com/product/api")
	apiPackage.Files = append(apiPackage.Files, "README.md")
	// Package file inventories are private producer authority and deliberately
	// do not alter the names-only target catalog seal.
	if _, err := NewGoFileTargetResolver(fixture.repository, fixture.facts, fixture.catalog); err == nil ||
		!strings.Contains(err.Error(), "is outside package directory") {
		t.Fatalf("package file ownership drift error = %v", err)
	}
}

func TestDiscoverGoTargetFilesRejectsCatalogFactDrift(t *testing.T) {
	fixture := newGoFileDiscoveryFixture(t)
	fixture.packageFact("example.com/product/api").Declarations[0].Name = "Renamed"
	if _, err := DiscoverGoTargetFiles(fixture.repository, fixture.facts, fixture.catalog); err == nil ||
		!strings.Contains(err.Error(), "catalog does not match Go facts") {
		t.Fatalf("catalog drift error = %v", err)
	}
}

type goFileDiscoveryFixture struct {
	t          *testing.T
	repository *corpus.Corpus
	facts      gofacts.Facts
	catalog    TargetCatalog
}

func newGoFileDiscoveryFixture(t *testing.T) *goFileDiscoveryFixture {
	t.Helper()
	paths := []string{
		"README.md",
		"api/ignored.go",
		"api/implementation.go",
		"api/public.go",
		"api/types.go",
		"cmd/app/main.go",
		"cmd/app/ignored.go",
		"cmd/app/run.go",
		"cmd/app/main.s",
		"go.mod",
		"internal/hidden/hidden.go",
	}
	repositoryRoot := t.TempDir()
	for _, filePath := range paths {
		absolutePath := filepath.Join(repositoryRoot, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolutePath, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{
		Paths: paths, RegularPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	complete := func() *gofacts.PackageLoadCompleteness {
		return &gofacts.PackageLoadCompleteness{
			Version: gofacts.PackageLoadCompletenessVersion,
			State:   gofacts.PackageLoadComplete,
		}
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/product", ModuleDir: ".", Main: true,
			PackagesCount: 3, RetainedPackagesCount: 3,
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: 3, PackagesRetained: 3,
			},
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/product/cmd/app", Name: "main",
				ModuleID: "module-root", ModulePath: "example.com/product",
				PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app", DisplayPath: "cmd/app", Locality: "local",
				Files:               []string{"cmd/app/main.go", "cmd/app/run.go"},
				DeclarationsScanned: true, LoadCompleteness: complete(),
			},
			{
				CanonicalPath: "example.com/product/api", Name: "api",
				ModuleID: "module-root", ModulePath: "example.com/product",
				PackageDir: "api", ModuleRelativeDir: "api", DisplayPath: "api", Locality: "local",
				Files:               []string{"api/implementation.go", "api/public.go", "api/types.go", "api/generated_untracked.go"},
				DeclarationsScanned: true, LoadCompleteness: complete(),
				Declarations: []gofacts.PackageDeclaration{
					{Kind: gofacts.PackageDeclarationFunc, Name: "Close", Path: "api/public.go", Line: 9, Column: 6, ExecutableBody: true},
					{Kind: gofacts.PackageDeclarationFunc, Name: "Open", Path: "api/public.go", Line: 3, Column: 6, ExecutableBody: true},
					{Kind: gofacts.PackageDeclarationMethod, Name: "Serve", Receiver: "Client", Path: "api/public.go", Line: 14, Column: 18, ExecutableBody: true},
					{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "api/types.go", Line: 3, Column: 6},
					{Kind: gofacts.PackageDeclarationVar, Name: "Default", Path: "api/types.go", Line: 7, Column: 5},
					{Kind: gofacts.PackageDeclarationConst, Name: "Mode", Path: "api/types.go", Line: 10, Column: 7},
					{Kind: gofacts.PackageDeclarationFunc, Name: "helper", Path: "api/implementation.go", Line: 3, Column: 6, ExecutableBody: true},
				},
			},
			{
				CanonicalPath: "example.com/product/internal/hidden", Name: "hidden",
				ModuleID: "module-root", ModulePath: "example.com/product",
				PackageDir: "internal/hidden", ModuleRelativeDir: "internal/hidden", DisplayPath: "internal/hidden", Locality: "local",
				Files: []string{"internal/hidden/hidden.go"}, DeclarationsScanned: true, LoadCompleteness: complete(),
				Declarations: []gofacts.PackageDeclaration{{
					Kind: gofacts.PackageDeclarationType, Name: "Hidden",
					Path: "internal/hidden/hidden.go", Line: 3, Column: 6,
				}},
			},
		},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: "example.com/product", ImportPath: "example.com/product/cmd/app",
			PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app", ModuleDir: ".",
			Kind: "primary_binary", GoFiles: []string{"main.go", "run.go"},
			Anchors: []gofacts.EntrypointAnchor{
				{Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain, Path: "cmd/app/main.go", Line: 3},
				{Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain, Path: "cmd/app/main.go", Line: 3},
			},
		}},
		PackagesCount: 3, RetainedPackagesCount: 3,
		Coverage: gofacts.Coverage{
			State: gofacts.CoverageComplete, ModulesDiscovered: 1, ModulesAvailable: 1,
			PackagesDiscovered: 3, PackagesRetained: 3,
		},
	}
	return &goFileDiscoveryFixture{
		t: t, repository: repository, facts: facts,
		catalog: mustGoFileDiscoveryCatalog(t, facts),
	}
}

func mustGoFileDiscoveryCatalog(t *testing.T, facts gofacts.Facts) TargetCatalog {
	t.Helper()
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func (fixture *goFileDiscoveryFixture) id(t *testing.T, filePath string) corpus.FileID {
	t.Helper()
	fileRef, ok := fixture.repository.ID(filePath)
	if !ok {
		t.Fatalf("missing fixture corpus path %q", filePath)
	}
	return fileRef
}

func (fixture *goFileDiscoveryFixture) packageFact(packagePath string) *gofacts.PackageFact {
	for index := range fixture.facts.Packages {
		if fixture.facts.Packages[index].CanonicalPath == packagePath {
			return &fixture.facts.Packages[index]
		}
	}
	fixture.t.Fatalf("missing fixture package %q", packagePath)
	return nil
}

func (fixture *goFileDiscoveryFixture) targetRef(t *testing.T, kind Kind) string {
	t.Helper()
	for _, entry := range fixture.catalog.Entries {
		if entry.Candidate.Target.Kind == kind {
			return entry.Candidate.Target.Ref
		}
	}
	t.Fatalf("missing fixture target kind %q", kind)
	return ""
}

func (fixture *goFileDiscoveryFixture) catalogTargetRefs() []string {
	result := make([]string, 0, len(fixture.catalog.Entries))
	for _, entry := range fixture.catalog.Entries {
		result = append(result, entry.Candidate.Target.Ref)
	}
	return result
}
