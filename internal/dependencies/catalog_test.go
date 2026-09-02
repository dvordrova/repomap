package dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestScaleWarningsTreatFormerArtifactCeilingAsDiagnosticOnly(t *testing.T) {
	warnings := scaleWarningsForEncodedBytes(AdvisoryArtifactBytes + 1)
	if len(warnings) != 1 || warnings[0].Kind != ScaleWarningArtifactBytes ||
		warnings[0].Retained != AdvisoryArtifactBytes+1 ||
		warnings[0].AdvisorySize != AdvisoryArtifactBytes {
		t.Fatalf("warnings = %#v", warnings)
	}
	if warnings := scaleWarningsForEncodedBytes(AdvisoryArtifactBytes); len(warnings) != 0 {
		t.Fatalf("threshold warning = %#v", warnings)
	}
}

func TestBuildCanonicalizesStableDependenciesAndImporterRefs(t *testing.T) {
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

	got, err := BuildWithOmissions([]Importer{worker, app, app}, rows, nil)
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
	for _, value := range got.Dependencies {
		if value.ID == "" {
			t.Fatalf("dependency has no stable local id: %#v", value)
		}
	}
	for _, importer := range got.Importers {
		if importer.Ref == "" {
			t.Fatalf("importer has no stable local ref: %#v", importer)
		}
	}

	reversedRows := append([]Dependency(nil), rows...)
	slices.Reverse(reversedRows)
	reversedImporters := []Importer{app, worker}
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

func TestCatalogSubsetRetainsStableDependencyIdentity(t *testing.T) {
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
	catalog.Importers[0].PackagePath = "example.com/root/drifted"
	if err := catalog.Validate(); err == nil {
		t.Fatal("drifted importer ref binding was accepted")
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
	subset, err := catalog.Subset(map[string]struct{}{importer.Ref: {}})
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
