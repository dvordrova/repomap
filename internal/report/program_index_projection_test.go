package report

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestReadRunDirRestoresDefaultProgramPortfolioAndOpenablePaths(t *testing.T) {
	runDir := t.TempDir()
	index, groups, _ := writeReportFinalGraphArtifacts(
		t, runDir, reportProgramIndexFixture(t, "python", "executable"),
	)
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
	if data.GroupGraph == nil || data.GroupGraph.SelectedTargetID != index.Target.ID ||
		len(data.GroupGraph.Indexes) != 1 || data.GroupGraph.Indexes[0].SHA256 != groups.SHA256 {
		t.Fatalf("group graph = %#v", data.GroupGraph)
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
	index, groups, _ := reportFinalGraphFixture(t, reportProgramIndexFixture(t, "python", "executable"))
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView([]groupindex.Index{groups}, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		ProgramPortfolio: portfolio,
		GroupGraph:       graph,
	}
	if err := collectOpenablePaths(data); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(data.OpenablePaths, "app") {
		t.Fatalf("package directory was promoted to source authority: %#v", data.OpenablePaths)
	}
	if !slices.Contains(data.OpenablePaths, "app/main.py") {
		t.Fatalf("exact program source is not openable: %#v", data.OpenablePaths)
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
	return reportProgramIndexFixtureWithRelationOmissions(t, language, kind, 0)
}

func reportProgramIndexFixtureWithRelationOmissions(
	t *testing.T,
	language, kind string,
	relationOmissions int,
) programindex.Index {
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
			Measured: true, ObjectsObserved: 8, RelationsObserved: 2 + relationOmissions,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func writeReportProgramIndexArtifacts(t *testing.T, runDir string, index programindex.Index) {
	t.Helper()
	if err := dependencies.Persist(runDir, dependencies.Empty()); err != nil {
		t.Fatal(err)
	}
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
}

func writeReportProgramFile(t *testing.T, filePath string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
