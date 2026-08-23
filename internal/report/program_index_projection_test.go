package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythondeclareddependencies"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestReadRunDirRestoresDefaultProgramPortfolioAndOpenablePaths(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	if data.ProgramPortfolio == nil || len(data.ProgramPortfolio.Entries) != 1 ||
		data.ProgramPortfolio.DefaultTargetID != index.Target.ID {
		t.Fatalf("program portfolio = %#v", data.ProgramPortfolio)
	}
	if data.CoreMapView == nil || data.CoreMapView.ProgramTargetID != index.Target.ID ||
		data.CoreMapView.ProgramIndexSHA256 != index.SHA256 {
		t.Fatalf("core map view = %#v", data.CoreMapView)
	}
	if data.IntegrationUsageView == nil ||
		data.IntegrationUsageView.ProgramTargetID != index.Target.ID ||
		data.IntegrationUsageView.ProgramIndexSHA256 != index.SHA256 ||
		len(data.IntegrationUsageView.Dependencies) != 0 ||
		len(data.IntegrationUsageView.DeclaredCandidates) != 0 ||
		data.IntegrationUsageView.DeclarationCoverage == nil ||
		data.IntegrationUsageView.DeclarationCoverage.Input.State != dependencydeclaration.CoverageComplete {
		t.Fatalf("integration usage view = %#v", data.IntegrationUsageView)
	}
	if data.ActivityEntrypointView == nil ||
		data.ActivityEntrypointView.ProgramTargetID != index.Target.ID ||
		data.ActivityEntrypointView.ProgramIndexSHA256 != index.SHA256 {
		t.Fatalf("activity entrypoint view = %#v", data.ActivityEntrypointView)
	}
	if data.ActivityPathView == nil ||
		data.ActivityPathView.ProgramTargetID != index.Target.ID ||
		data.ActivityPathView.ProgramIndexSHA256 != index.SHA256 ||
		len(data.ActivityPathView.Outcomes) != 0 || len(data.ActivityPathView.Routes) != 0 {
		t.Fatalf("activity path view = %#v", data.ActivityPathView)
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil || !reflect.DeepEqual(defaultEntry.Target, index.Target) {
		t.Fatalf("default program entry = %#v / %v, want %#v", defaultEntry, err, index.Target)
	}
	for _, sourcePath := range []string{
		"app/__init__.py", "app/main.py", "storage/__init__.py", "storage/db.py", "scripts/clean.py",
	} {
		if !slices.Contains(data.OpenablePaths, sourcePath) {
			t.Errorf("projected source path %q is not openable: %#v", sourcePath, data.OpenablePaths)
		}
	}
}

func TestReadRunDirKeepsSnapshotGoFactsOnlyAsMaterialInputs(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	snapshot := map[string]any{
		"repo_name": "go-fixture",
		"go_facts": map[string]any{
			"modules": []map[string]any{{
				"id": "go-module", "module_path": "example.com/repo", "module_dir": ".",
			}},
			"packages": []map[string]any{{"files": []string{"cmd/app/main.go"}}},
		},
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), encoded)
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"go-fixture"}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil || defaultEntry.Target.ID != index.Target.ID {
		t.Fatalf("default program entry = %#v / %v", defaultEntry, err)
	}
	wantMaterial := []string{"cmd/app/main.go", "go.mod", "go.sum"}
	if !reflect.DeepEqual(data.materialInputPaths, wantMaterial) {
		t.Fatalf("material input paths = %#v, want %#v", data.materialInputPaths, wantMaterial)
	}
}

func TestReadRunDirDoesNotRecoverMissingGoAnalysisTarget(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "go", "executable_package")
	writeReportProgramIndexArtifacts(t, runDir, index)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"go-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"go-fixture"}`))

	if _, err := ReadRunDir(runDir); err == nil ||
		!strings.Contains(err.Error(), "Go ProgramIndex target page requires its exact outer analysis target") {
		t.Fatalf("missing exact Go semantic authority error = %v", err)
	}
}

func TestCollectOpenablePathsRejectsInvalidProgramPath(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "executable")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	portfolio.Entries[0].Target.Sources[0].Path = "../escape.py"
	data := &ReportData{ProgramPortfolio: portfolio}
	if err := collectOpenablePaths(data); err == nil || !strings.Contains(err.Error(), "invalid openable path") {
		t.Fatalf("invalid program path error = %v", err)
	}
}

func TestCollectOpenablePathsDoesNotPromotePackageDirectories(t *testing.T) {
	index := cubeMapProgramIndexFixture(t)
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	cubeView, err := NewCubeMapView(cubeMapViewFixture(t), index.Target, index.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	analysisTarget := cubeView.Target.Snapshot()
	data := &ReportData{
		ProgramPortfolio: portfolio,
		AnalysisTarget:   &analysisTarget,
		CubeMapView:      cubeView,
	}
	if err := collectOpenablePaths(data); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(data.OpenablePaths, "internal/client") {
		t.Fatalf("package directory was promoted to source authority: %#v", data.OpenablePaths)
	}
	if !slices.Contains(data.OpenablePaths, "internal/client/send.go") {
		t.Fatalf("exact integration source is not openable: %#v", data.OpenablePaths)
	}
}

func TestReadRunDirRejectsMismatchedProgramIndexSetBinding(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "library")
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programindex.ArtifactFilename), encoded)
	set, err := programindex.NewArtifactSet("different-target", []programindex.ArtifactSetEntry{{
		TargetID: "different-target", Filename: programindex.ArtifactFilename, IndexSHA256: index.SHA256,
	}})
	if err != nil {
		t.Fatal(err)
	}
	setBytes, err := programindex.EncodeArtifactSet(set)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programindex.ArtifactSetFilename), setBytes)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))

	if _, err := ReadRunDir(runDir); err == nil || !strings.Contains(err.Error(), "artifact-set binding") {
		t.Fatalf("mismatched binding error = %v", err)
	}
}

func TestReadRunDirRequiresExactSnapshotAndMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, string){
		"missing snapshot": func(t *testing.T, runDir string) {
			writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"fixture"}`))
		},
		"malformed snapshot": func(t *testing.T, runDir string) {
			writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":`))
			writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"fixture"}`))
		},
		"missing metadata": func(t *testing.T, runDir string) {
			writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"fixture"}`))
		},
		"malformed metadata": func(t *testing.T, runDir string) {
			writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"fixture"}`))
			writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":`))
		},
		"mismatched metadata": func(t *testing.T, runDir string) {
			writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"fixture"}`))
			writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"other"}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			runDir := t.TempDir()
			writeReportProgramIndexArtifacts(t, runDir, reportProgramIndexFixture(t, "python", "library"))
			mutate(t, runDir)
			if _, err := ReadRunDir(runDir); err == nil {
				t.Fatal("ReadRunDir accepted incomplete or invalid run identity")
			}
		})
	}
}

func reportProgramIndexFixture(t *testing.T, language, kind string) programindex.Index {
	t.Helper()
	location := func(sourcePath string) *programindex.Location {
		return &programindex.Location{Path: sourcePath, Line: 1, Column: 1}
	}
	targetInput := programindex.TargetInput{
		Language: language, Kind: kind, Name: "fixture", Selector: "fixture",
		Sources: []programindex.TargetSource{
			{FileRef: "f1", Path: "app/__init__.py"},
			{FileRef: "f2", Path: "app/main.py"},
			{FileRef: "f3", Path: "scripts/clean.py"},
			{FileRef: "f4", Path: "storage/__init__.py"},
			{FileRef: "f5", Path: "storage/db.py"},
		},
		AnchorFileRef: "f2",
	}
	if language == "python" && kind == "library" {
		targetInput.AnchorFileRef = "f1"
	}
	if strings.Contains(kind, "executable") {
		targetInput.Seeds = []programindex.TargetSeedInput{{
			ObjectRef: "fn-start", Kind: programindex.SeedCallable, Location: location("app/main.py"),
		}}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target:         targetInput,
		Objects: []programindex.ObjectInput{
			{SourceRef: "pkg-app", Kind: programindex.ObjectPackage, Name: "app", Visibility: programindex.VisibilityPublic, Location: location("app/__init__.py")},
			{SourceRef: "mod-main", Kind: programindex.ObjectModule, Name: "app.main", Visibility: programindex.VisibilityPublic, ContainerRef: "pkg-app", Location: location("app/main.py")},
			{SourceRef: "fn-start", Kind: programindex.ObjectFunction, Name: "start", Visibility: programindex.VisibilityPublic, ContainerRef: "mod-main", OwnerRef: "mod-main", Location: location("app/main.py")},
			{SourceRef: "pkg-storage", Kind: programindex.ObjectPackage, Name: "storage", Visibility: programindex.VisibilityPublic, Location: location("storage/__init__.py")},
			{SourceRef: "mod-db", Kind: programindex.ObjectModule, Name: "storage.db", Visibility: programindex.VisibilityPublic, ContainerRef: "pkg-storage", Location: location("storage/db.py")},
			{SourceRef: "fn-load", Kind: programindex.ObjectFunction, Name: "load", Visibility: programindex.VisibilityPublic, ContainerRef: "mod-db", OwnerRef: "mod-db", Location: location("storage/db.py")},
			{SourceRef: "mod-clean", Kind: programindex.ObjectModule, Name: "scripts.clean", Visibility: programindex.VisibilityPublic, Location: location("scripts/clean.py")},
			{SourceRef: "fn-clean", Kind: programindex.ObjectFunction, Name: "clean", Visibility: programindex.VisibilityPublic, ContainerRef: "mod-clean", OwnerRef: "mod-clean", Location: location("scripts/clean.py")},
		},
		Relations: []programindex.RelationInput{
			{SourceRef: "rel-import", Kind: programindex.RelationImports, FromRef: "mod-main", ToRefs: []string{"fn-load"}, Resolution: programindex.ResolutionExact, TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "python_import"}}, WitnessesObserved: 1},
			{SourceRef: "rel-call", Kind: programindex.RelationCalls, FromRef: "fn-clean", ToRefs: []string{"fn-load"}, Resolution: programindex.ResolutionExact, TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "python_call"}}, WitnessesObserved: 1},
		},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 8, RelationsObserved: 2,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func writeReportProgramIndexArtifacts(t *testing.T, runDir string, index programindex.Index) {
	t.Helper()
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programindex.ArtifactFilename), encoded)
	set, err := programindex.NewArtifactSet(index.Target.ID, []programindex.ArtifactSetEntry{{
		TargetID: index.Target.ID, Filename: programindex.ArtifactFilename, IndexSHA256: index.SHA256,
	}})
	if err != nil {
		t.Fatal(err)
	}
	setBytes, err := programindex.EncodeArtifactSet(set)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programindex.ArtifactSetFilename), setBytes)
	if index.Target.Language == "python" {
		_, _, targetBytes, declarationBytes := reportPythonDeclarationArtifactsFixture(t, index)
		writeReportProgramFile(t, filepath.Join(runDir, pythontarget.ArtifactFilename), targetBytes)
		writeReportProgramFile(t, filepath.Join(runDir, dependencydeclaration.ArtifactFilename), declarationBytes)
	}
	if index.Target.Language == "python" || index.Target.Language == "go" {
		_, activityBytes := reportActivityEntrypointFixture(t, index)
		writeReportProgramFile(t, filepath.Join(runDir, activityentrypoint.ArtifactFilename), activityBytes)
		_, coreBytes := reportCoreMapFixture(t, index)
		writeReportProgramFile(t, filepath.Join(runDir, coremap.ArtifactFilename), coreBytes)
		_, catalogBytes, selectedBytes, usageBytes := reportIntegrationUsageFixture(t, index)
		writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), catalogBytes)
		writeReportProgramFile(t, filepath.Join(runDir, integrationdependency.ArtifactFilename), selectedBytes)
		writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageBytes)
		_, pathBytes := reportActivityPathFixture(
			t, index, activityBytes, selectedBytes, usageBytes,
		)
		writeReportProgramFile(t, filepath.Join(runDir, activitypath.ArtifactFilename), pathBytes)
	}
}

func reportActivityEntrypointFixture(
	t *testing.T,
	index programindex.Index,
) (*ActivityEntrypointView, []byte) {
	t.Helper()
	provider := &reportCoreMapProvider{response: []byte(`{"activity_refs":["a1"]}`)}
	result, err := activityentrypoint.Run(
		t.Context(), llm.Executor{Enabled: false}, provider, index,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := activityentrypoint.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewActivityEntrypointView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	return view, encoded
}

func reportIntegrationUsageFixture(
	t *testing.T,
	index programindex.Index,
) (*IntegrationUsageView, []byte, []byte, []byte) {
	t.Helper()
	catalog := dependencies.Empty()
	var selected integrationdependency.Result
	var err error
	if index.Target.Language == "python" {
		_, declarations, _, _ := reportPythonDeclarationArtifactsFixture(t, index)
		selected, err = integrationdependency.RunWithDeclarations(
			t.Context(), llm.Executor{}, nil, catalog, declarations, index.Target,
		)
	} else {
		selected, err = integrationdependency.Run(
			t.Context(), llm.Executor{}, nil, catalog,
		)
	}
	if err != nil {
		t.Fatal(err)
	}
	usage, err := integrationusage.Run(t.Context(), llm.Executor{}, nil, index, selected)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewIntegrationUsageView(usage, index, selected)
	if err != nil {
		t.Fatal(err)
	}
	catalogBytes, err := dependencies.Encode(catalog)
	if err != nil {
		t.Fatal(err)
	}
	selectedBytes, err := integrationdependency.Encode(selected)
	if err != nil {
		t.Fatal(err)
	}
	usageBytes, err := integrationusage.Encode(usage)
	if err != nil {
		t.Fatal(err)
	}
	return view, catalogBytes, selectedBytes, usageBytes
}

func reportPythonDeclarationArtifactsFixture(
	t *testing.T,
	index programindex.Index,
) (pythontarget.Catalog, dependencydeclaration.Result, []byte, []byte) {
	t.Helper()
	if index.Target.Language != "python" {
		t.Fatalf("Python declaration fixture received %q target", index.Target.Language)
	}
	modules := make([]pythontarget.Module, 0, len(index.Target.Sources))
	basis := make([]pythontarget.Basis, 0, len(index.Target.Sources))
	packagesByName := make(map[string]pythontarget.Package)
	for _, source := range index.Target.Sources {
		moduleName, packageModule := reportPythonModuleName(t, source.Path)
		modules = append(modules, pythontarget.Module{
			FileID: corpus.FileID(source.FileRef), Name: moduleName, Path: source.Path,
			Importable: true, Package: packageModule,
		})
		basis = append(basis, pythontarget.Basis{
			FileID: corpus.FileID(source.FileRef), Kind: pythontarget.BasisImportPackage,
			Path: source.Path, Label: strings.Split(moduleName, ".")[0],
		})
		topLevel := strings.Split(moduleName, ".")[0]
		if _, exists := packagesByName[topLevel]; !exists {
			packagePath := ""
			directory := topLevel
			namespace := true
			if packageModule && moduleName == topLevel {
				packagePath = source.Path
				directory = filepath.ToSlash(filepath.Dir(source.Path))
				namespace = false
			}
			packagesByName[topLevel] = pythontarget.Package{
				Name: topLevel, Dir: directory, Path: packagePath, Namespace: namespace,
			}
		}
	}

	target := pythontarget.Target{
		Version: pythontarget.TargetVersion, Selector: index.Target.Selector,
		DisplayName: index.Target.Name, ProjectDir: ".", SourceRoots: []string{"."},
		Modules: modules, Basis: basis,
	}
	if index.Target.Kind == "library" {
		target.Kind = pythontarget.KindLibrary
		target.Packages = make([]pythontarget.Package, 0, len(packagesByName))
		for _, value := range packagesByName {
			target.Packages = append(target.Packages, value)
		}
	} else {
		target.Kind = pythontarget.KindExecutable
		if len(index.Target.Seeds) == 0 || index.Target.Seeds[0].Location == nil {
			t.Fatal("executable Python fixture has no exact seed")
		}
		seed := index.Target.Seeds[0]
		moduleName, _ := reportPythonModuleName(t, seed.Location.Path)
		qualname := "start"
		for _, object := range index.Objects {
			if object.ID == seed.ObjectID {
				qualname = object.Name
				break
			}
		}
		target.Roots = []pythontarget.Root{{
			Kind: pythontarget.RootCallable, Module: moduleName, Qualname: qualname,
			Path: seed.Location.Path, Line: seed.Location.Line,
		}}
	}
	targets, err := pythontarget.NewCatalog([]pythontarget.Target{target}, nil)
	if err != nil {
		t.Fatalf("pythontarget.NewCatalog: %v", err)
	}
	scope, _, err := pythondeclareddependencies.ScopeForTarget(targets, targets.Entries[0], index)
	if err != nil {
		t.Fatalf("pythondeclareddependencies.ScopeForTarget: %v", err)
	}
	declarations, err := dependencydeclaration.Build(dependencydeclaration.Input{
		CorpusSHA256: index.SourceSHA256, ProgramIndexSHA256: index.SHA256,
		TargetID: index.Target.ID, Scope: scope,
		Sources: []dependencydeclaration.SourceInput{}, Statements: []dependencydeclaration.StatementInput{},
		Includes: []dependencydeclaration.IncludeInput{}, Frontiers: []dependencydeclaration.FrontierInput{},
	})
	if err != nil {
		t.Fatalf("dependencydeclaration.Build: %v", err)
	}
	if err := pythondeclareddependencies.ValidateTargetAuthority(declarations, targets, index); err != nil {
		t.Fatalf("declaration target authority: %v", err)
	}
	targetBytes, err := targets.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	declarationBytes, err := dependencydeclaration.Encode(declarations)
	if err != nil {
		t.Fatal(err)
	}
	return targets, declarations, targetBytes, declarationBytes
}

func reportPythonModuleName(t *testing.T, sourcePath string) (string, bool) {
	t.Helper()
	if !strings.HasSuffix(sourcePath, ".py") {
		t.Fatalf("Python fixture source %q is not a module", sourcePath)
	}
	withoutExtension := strings.TrimSuffix(sourcePath, ".py")
	packageModule := strings.HasSuffix(withoutExtension, "/__init__") || withoutExtension == "__init__"
	withoutExtension = strings.TrimSuffix(withoutExtension, "/__init__")
	if withoutExtension == "__init__" {
		withoutExtension = "fixture"
	}
	return strings.ReplaceAll(withoutExtension, "/", "."), packageModule
}

func reportActivityPathFixture(
	t *testing.T,
	index programindex.Index,
	activityBytes []byte,
	selectedBytes []byte,
	usageBytes []byte,
) (*ActivityPathView, []byte) {
	t.Helper()
	activities, err := activityentrypoint.Decode(activityBytes, index)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := integrationdependency.Decode(selectedBytes)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := integrationusage.Decode(usageBytes)
	if err != nil {
		t.Fatal(err)
	}
	result, err := activitypath.Build(index, activities, selected, usage)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewActivityPathView(result, index, activities, selected, usage)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := activitypath.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	return view, encoded
}

func reportCoreMapFixture(t *testing.T, index programindex.Index) (*CoreMapView, []byte) {
	t.Helper()
	repositoryRoot := t.TempDir()
	pathSet := make(map[string]struct{})
	for _, source := range index.Target.Sources {
		pathSet[source.Path] = struct{}{}
	}
	for _, object := range index.Objects {
		if object.Location != nil {
			pathSet[object.Location.Path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(pathSet))
	for sourcePath := range pathSet {
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	for _, sourcePath := range paths {
		fullPath := filepath.Join(repositoryRoot, filepath.FromSlash(sourcePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeReportProgramFile(t, fullPath, []byte("# exact test source\n"))
	}
	repository, err := corpus.New(t.Context(), repositoryRoot, gitfiles.Listing{
		Paths: append([]string(nil), paths...), RegularPaths: append([]string(nil), paths...),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, _, selectedBytes, usageBytes := reportIntegrationUsageFixture(t, index)
	selected, err := integrationdependency.Decode(selectedBytes)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := integrationusage.Decode(usageBytes)
	if err != nil {
		t.Fatal(err)
	}
	compilation, err := coremap.CompileProgramWithIntegrationUsage(
		index.Target.Language+"-fixture", repository, index, readmetargetscout.Result{}, selected, usage,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := &reportCoreMapProvider{response: []byte(
		`{"blocks":[{"name":"Execution core","purpose":"Groups exact declarations that may explain execution.","file_refs":[],"symbol_refs":["s1"]}]}`,
	)}
	result, err := coremap.Run(t.Context(), llm.Executor{Enabled: false}, provider, compilation)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := coremap.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewCoreMapView(result, index, nil)
	if err != nil {
		t.Fatal(err)
	}
	return view, encoded
}

type reportCoreMapProvider struct {
	response []byte
}

func (provider *reportCoreMapProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`)
}

func (provider *reportCoreMapProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.System + "\n" + prompt.User))
}

func (provider *reportCoreMapProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{
		Response: provider.response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{
			InputTokens: 10, OutputTokens: 10, ProviderResponseBytes: len(provider.response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}

func writeReportProgramFile(t *testing.T, filePath string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
