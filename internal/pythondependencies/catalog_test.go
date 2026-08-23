package pythondependencies

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestBuildProjectsExactWorkspaceStdlibAndExternalImports(t *testing.T) {
	index := dependencyProgramIndex(t, false)
	catalog, err := Build(index)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete || len(catalog.Coverage.Omissions) != 0 {
		t.Fatalf("coverage = %#v", catalog.Coverage)
	}
	wantKinds := map[string]dependencies.Kind{
		"pkg.tasks": dependencies.KindWorkspace,
		"json":      dependencies.KindStdlib,
		"fastapi":   dependencies.KindExternal,
	}
	if len(catalog.Dependencies) != len(wantKinds) {
		t.Fatalf("dependencies = %#v", catalog.Dependencies)
	}
	for _, value := range catalog.Dependencies {
		if wantKinds[value.PackagePath] != value.Kind {
			t.Fatalf("dependency %q = %#v", value.PackagePath, value)
		}
		if len(value.ImporterRefs) != 1 {
			t.Fatalf("dependency importer binding = %#v", value)
		}
		delete(wantKinds, value.PackagePath)
	}
	if len(wantKinds) != 0 {
		t.Fatalf("missing dependencies = %#v", wantKinds)
	}
}

func TestBuildKeepsDynamicImportAsProgramFrontierNotMissingDependency(t *testing.T) {
	catalog, err := Build(dependencyProgramIndex(t, true))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete || len(catalog.Coverage.Omissions) != 0 {
		t.Fatalf("coverage = %#v", catalog.Coverage)
	}
}

func TestBuildKeepsUnnamedDirectImportAsPartialCoverage(t *testing.T) {
	index := dependencyProgramIndex(t, false)
	input := programindex.Input{
		ScenarioSHA256: index.ScenarioSHA256,
		SourceSHA256:   index.SourceSHA256,
		Target: programindex.TargetInput{
			Language: index.Target.Language, Kind: index.Target.Kind,
			Name: index.Target.Name, Selector: index.Target.Selector,
			Sources:       []programindex.TargetSource{{FileRef: "f-main", Path: "pkg/main.py"}},
			AnchorFileRef: "f-main",
		},
		Coverage: programindex.CoverageInput{Measured: true},
	}
	moduleRef := ""
	for _, object := range index.Objects {
		input.Objects = append(input.Objects, programindex.ObjectInput{
			SourceRef: object.ID, Kind: object.Kind, Name: object.Name,
			Visibility: object.Visibility, Signature: object.Signature,
			OwnerRef: object.OwnerID, ContainerRef: object.ContainerID,
			Location: object.Location,
		})
		if object.Kind == programindex.ObjectModule && object.Name == "pkg.main" {
			moduleRef = object.ID
		}
	}
	if moduleRef == "" {
		t.Fatal("fixture has no pkg.main module")
	}
	input.Relations = []programindex.RelationInput{{
		SourceRef: "unknown-import", Kind: programindex.RelationImports,
		FromRef: moduleRef, Resolution: programindex.ResolutionUnresolved,
		Location:          &programindex.Location{Path: "pkg/main.py", Line: 8, Column: 1},
		TargetsObserved:   1,
		Witnesses:         []programindex.Witness{{Kind: "import", Detail: "unknown_package"}},
		WitnessesObserved: 1,
	}}
	input.Coverage.ObjectsObserved = len(input.Objects)
	input.Coverage.RelationsObserved = len(input.Relations)
	index, err := programindex.New(input)
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	catalog, err := Build(index)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if catalog.Coverage.State != dependencies.CoveragePartial || len(catalog.Coverage.Omissions) != 1 ||
		catalog.Coverage.Omissions[0].Reason != dependencies.OmissionDependencyIdentityMissing ||
		!strings.Contains(catalog.Coverage.Omissions[0].PackagePath, "unknown_package") {
		t.Fatalf("coverage = %#v", catalog.Coverage)
	}
}

func dependencyProgramIndex(t *testing.T, unresolved bool) programindex.Index {
	t.Helper()
	location := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "module-main", Kind: programindex.ObjectModule, Name: "pkg.main", Visibility: programindex.VisibilityPublic, Location: location("pkg/main.py", 1)},
		{SourceRef: "module-tasks", Kind: programindex.ObjectModule, Name: "pkg.tasks", Visibility: programindex.VisibilityPublic, Location: location("pkg/tasks.py", 1)},
		{SourceRef: "callback", Kind: programindex.ObjectFunction, Name: "callback", Visibility: programindex.VisibilityPublic, ContainerRef: "module-tasks", OwnerRef: "module-tasks", Location: location("pkg/tasks.py", 3)},
		{SourceRef: "external-json", Kind: programindex.ObjectExternalSymbol, Name: "json", Visibility: programindex.VisibilityUnknown},
		{SourceRef: "external-fastapi", Kind: programindex.ObjectExternalSymbol, Name: "fastapi.FastAPI", Visibility: programindex.VisibilityUnknown},
	}
	relations := []programindex.RelationInput{
		{SourceRef: "import-tasks", Kind: programindex.RelationImports, FromRef: "module-main", ToRefs: []string{"callback"}, Resolution: programindex.ResolutionExact, Location: location("pkg/main.py", 1), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "from_import", Detail: "pkg.tasks.callback"}}, WitnessesObserved: 1},
		{SourceRef: "import-json", Kind: programindex.RelationImports, FromRef: "module-main", ToRefs: []string{"external-json"}, Resolution: programindex.ResolutionExact, Location: location("pkg/main.py", 2), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: WitnessStdlibImport, Detail: "json"}}, WitnessesObserved: 1},
		{SourceRef: "import-fastapi", Kind: programindex.RelationImports, FromRef: "module-main", ToRefs: []string{"external-fastapi"}, Resolution: programindex.ResolutionExact, Location: location("pkg/main.py", 3), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: WitnessExternalFromImport, Detail: "fastapi.FastAPI"}}, WitnessesObserved: 1},
	}
	if unresolved {
		relations = append(relations, programindex.RelationInput{
			SourceRef: "import-dynamic", Kind: programindex.RelationImports,
			FromRef: "module-main", Resolution: programindex.ResolutionUnresolved,
			Location: location("pkg/main.py", 4), TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: "dynamic_import", Detail: "importlib.import_module"}},
			WitnessesObserved: 1,
		})
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "library", Name: "pkg", Selector: "python:pkg",
			Sources:       []programindex.TargetSource{{FileRef: "f-main", Path: "pkg/main.py"}},
			AnchorFileRef: "f-main",
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}
