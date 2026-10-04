package dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

func TestPersistWritesExactValidatedCatalog(t *testing.T) {
	t.Parallel()

	catalog := Empty()
	runDir := t.TempDir()
	if err := Persist(runDir, catalog); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, catalog) {
		t.Fatalf("persisted catalog = %#v, want %#v", decoded, catalog)
	}
}

func TestBuildAssignsCanonicalCompactDependencyAndImporterRefs(t *testing.T) {
	t.Parallel()

	app := Importer{
		Language: "go", Name: "app", ModulePath: "example.com/root",
		PackagePath: "example.com/root/cmd/app", RepositoryPath: "cmd/app",
	}
	worker := Importer{
		Language: "go", Name: "worker", ModulePath: "example.com/root",
		PackagePath: "example.com/root/internal/worker", RepositoryPath: "internal/worker",
	}
	sealed, err := BuildWithOmissions([]Importer{worker, app, app}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := importerRefsByPackage(sealed)
	rows := []Dependency{
		{
			Language: "go", Kind: KindExternal, Name: "kafka", ModulePath: "corp/kafka",
			ModuleVersion: "v1.2.3", PackagePath: "corp/kafka/client",
			ImporterRefs: []string{refs[worker.PackagePath], refs[app.PackagePath], refs[worker.PackagePath]},
		},
		{
			Language: "go", Kind: KindWorkspace, Name: "state", ModulePath: "example.com/root",
			PackagePath: "example.com/root/internal/state", RepositoryPath: "internal/state",
			ImporterRefs: []string{refs[app.PackagePath]},
		},
		{
			Language: "go", Kind: KindStdlib, Name: "http", PackagePath: "net/http",
			ImporterRefs: []string{refs[worker.PackagePath]},
		},
		{
			Language: "go", Kind: KindExternal, Name: "kafka", ModulePath: "corp/kafka",
			ModuleVersion: "v1.2.3", PackagePath: "corp/kafka/client",
			ImporterRefs: []string{refs[app.PackagePath]},
		},
	}

	got, err := BuildWithOmissions(sealed.Importers, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(got.Importers) != 2 || len(got.Dependencies) != 3 {
		t.Fatalf("catalog cardinality = %d importers, %d dependencies", len(got.Importers), len(got.Dependencies))
	}
	if got.Dependencies[0].Kind != KindWorkspace || got.Dependencies[1].Kind != KindStdlib ||
		got.Dependencies[2].Kind != KindExternal {
		t.Fatalf("dependency order = %#v", got.Dependencies)
	}
	wantImporterRefs := []string{refs[app.PackagePath], refs[worker.PackagePath]}
	slices.Sort(wantImporterRefs)
	if !slices.Equal(got.Dependencies[2].ImporterRefs, wantImporterRefs) {
		t.Fatalf("merged importer refs = %#v", got.Dependencies[2].ImporterRefs)
	}
	for index, value := range got.Dependencies {
		if value.ID != "d"+strconv.Itoa(index+1) {
			t.Fatalf("dependency has noncanonical compact id: %#v", value)
		}
	}
	for index, importer := range got.Importers {
		if importer.Ref != "i"+strconv.Itoa(index+1) {
			t.Fatalf("importer has noncanonical compact ref: %#v", importer)
		}
	}

	reversedRows := append([]Dependency(nil), rows...)
	slices.Reverse(reversedRows)
	reversedImporters := append([]Importer(nil), sealed.Importers...)
	slices.Reverse(reversedImporters)
	again, err := BuildWithOmissions(reversedImporters, reversedRows, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(got)
	secondJSON, _ := json.Marshal(again)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("catalog depends on input order:\n%s\n%s", firstJSON, secondJSON)
	}
	var restored Catalog
	if err := json.Unmarshal(firstJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("persisted catalog failed validation: %v", err)
	}
	if !reflect.DeepEqual(got, restored) {
		t.Fatalf("persisted catalog drifted:\n%#v\n%#v", got, restored)
	}
}

func TestCatalogSubsetResealsCompactLocalIdentity(t *testing.T) {
	t.Parallel()

	first := Importer{
		Language: "go", Name: "one", ModulePath: "example.com/root",
		PackagePath: "example.com/root/one", RepositoryPath: "one",
	}
	second := Importer{
		Language: "go", Name: "two", ModulePath: "example.com/root",
		PackagePath: "example.com/root/two", RepositoryPath: "two",
	}
	sealed, err := BuildWithOmissions([]Importer{first, second}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := importerRefsByPackage(sealed)
	catalog, err := BuildWithOmissions(sealed.Importers, []Dependency{
		{
			Language: "go", Kind: KindStdlib, Name: "fmt", PackagePath: "fmt",
			ImporterRefs: []string{refs[first.PackagePath], refs[second.PackagePath]},
		},
		{
			Language: "go", Kind: KindStdlib, Name: "os", PackagePath: "os",
			ImporterRefs: []string{refs[second.PackagePath]},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmtID := catalog.Dependencies[0].ID
	subset, err := catalog.Subset(map[string]struct{}{refs[first.PackagePath]: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(subset.Importers) != 1 || len(subset.Dependencies) != 1 ||
		subset.Dependencies[0].PackagePath != "fmt" || subset.Dependencies[0].ID != fmtID ||
		!reflect.DeepEqual(subset.Dependencies[0].ImporterRefs, []string{refs[first.PackagePath]}) {
		t.Fatalf("subset = %#v", subset)
	}
}

func TestCatalogRejectsUnknownImporterAndIdentityDrift(t *testing.T) {
	t.Parallel()

	if _, err := BuildWithOmissions(nil, []Dependency{{
		Language: "go", Kind: KindStdlib, Name: "fmt", PackagePath: "fmt",
		ImporterRefs: []string{"importer-unknown"},
	}}, nil); err == nil {
		t.Fatal("unknown importer ref was accepted")
	}

	importer := Importer{
		Language: "go", Name: "app", ModulePath: "example.com/root",
		PackagePath: "example.com/root/app", RepositoryPath: "app",
	}
	catalog, err := BuildWithOmissions([]Importer{importer}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	catalog.Importers[0].Ref = "i2"
	if err := catalog.Validate(); err == nil {
		t.Fatal("noncanonical importer ordinal was accepted")
	}
}

func TestBuildWithOmissionsPersistsHonestPartialCoverage(t *testing.T) {
	t.Parallel()

	importer, err := SealImporter(Importer{
		Language: "go", Name: "app", ModulePath: "example.com/root",
		PackagePath: "example.com/root/app", RepositoryPath: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := BuildWithOmissions([]Importer{importer}, []Dependency{{
		Language: "go", Kind: KindStdlib, Name: "fmt", PackagePath: "fmt",
		ImporterRefs: []string{importer.Ref},
	}}, []Omission{
		{ImporterRef: importer.Ref, ImporterPackagePath: importer.PackagePath, PackagePath: "missing/module", Reason: OmissionDependencyMetadataMissing},
		{ImporterRef: importer.Ref, ImporterPackagePath: importer.PackagePath, PackagePath: "broken/module", Reason: OmissionDependencyLoadUnavailable},
		{ImporterRef: importer.Ref, ImporterPackagePath: importer.PackagePath, PackagePath: "missing/module", Reason: OmissionDependencyMetadataMissing},
	})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Coverage.State != CoveragePartial || catalog.Coverage.ImportsObserved != 3 ||
		catalog.Coverage.ImportsRetained != 1 || len(catalog.Coverage.Omissions) != 2 {
		t.Fatalf("coverage = %#v", catalog.Coverage)
	}
	compactRef := catalog.Importers[0].Ref
	subset, err := catalog.Subset(map[string]struct{}{compactRef: {}})
	if err != nil || !reflect.DeepEqual(subset.Coverage, catalog.Coverage) {
		t.Fatalf("retained importer coverage = %#v, error %v", subset.Coverage, err)
	}
	empty, err := catalog.Subset(map[string]struct{}{})
	if err != nil || empty.Coverage.State != CoverageComplete || empty.Coverage.ImportsObserved != 0 {
		t.Fatalf("excluded importer coverage = %#v, error %v", empty.Coverage, err)
	}
	tampered := catalog
	tampered.Coverage.State = CoverageComplete
	if err := tampered.Validate(); err == nil {
		t.Fatal("partial catalog claimed complete coverage")
	}
}

func importerRefsByPackage(catalog Catalog) map[string]string {
	result := make(map[string]string, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		result[importer.PackagePath] = importer.Ref
	}
	return result
}

// An importer that imports a package only for its effect is recorded among
// its importers and in EffectImporterRefs; rows of one package merge both
// lists, a subset keeps the kept importers' effect refs, and an effect ref
// that is no importer of the package is refused.
func TestEffectImportersAreASubsetOfImporters(t *testing.T) {
	t.Parallel()
	driverUser := Importer{Language: "go", Name: "store", ModulePath: "example.com/root", PackagePath: "example.com/root/store", RepositoryPath: "store"}
	caller := Importer{Language: "go", Name: "sync", ModulePath: "example.com/root", PackagePath: "example.com/root/sync", RepositoryPath: "sync"}
	sealed, err := BuildWithOmissions([]Importer{driverUser, caller}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := importerRefsByPackage(sealed)
	driver := func(importer string, effect bool) Dependency {
		value := Dependency{Language: "go", Kind: KindExternal, Name: "pq", ModulePath: "github.com/lib/pq", ModuleVersion: "v1.10.9", PackagePath: "github.com/lib/pq", ImporterRefs: []string{importer}}
		if effect {
			value.EffectImporterRefs = []string{importer}
		}
		return value
	}
	catalog, err := BuildWithOmissions(sealed.Importers, []Dependency{driver(refs[driverUser.PackagePath], true), driver(refs[caller.PackagePath], false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pq := catalog.Dependencies[0]
	if len(pq.ImporterRefs) != 2 || !reflect.DeepEqual(pq.EffectImporterRefs, []string{refs[driverUser.PackagePath]}) {
		t.Fatalf("merged driver = %#v", pq)
	}
	subset, err := catalog.Subset(map[string]struct{}{refs[caller.PackagePath]: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(subset.Dependencies) != 1 || len(subset.Dependencies[0].EffectImporterRefs) != 0 {
		t.Fatalf("subset kept another importer's effect import: %#v", subset.Dependencies)
	}
	broken := catalog
	broken.Dependencies = []Dependency{snapshotDependency(pq)}
	broken.Dependencies[0].EffectImporterRefs = []string{"i9"}
	if err := broken.Validate(); err == nil {
		t.Fatal("an effect importer that imports nothing was accepted")
	}
}
