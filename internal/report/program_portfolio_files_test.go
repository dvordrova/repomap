package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestFileBoundSharedJoinKeepsCompleteValuesAndLatchesReadFailure(t *testing.T) {
	for _, bad := range []string{"missing", "different-bytes"} {
		t.Run(bad, func(t *testing.T) {
			data := reportTwoTargetDataFixture(t)
			newBuilder := func() *pageBuilder {
				builder := &pageBuilder{data: &data, indexes: data.GroupGraph.hydrated}
				for _, graph := range builder.indexes {
					builder.sections = append(builder.sections, &pageSection{programTargetID: graph.Target.ID})
				}
				return builder
			}
			want := newBuilder().sharedJoin()
			files := reportNativeFilesFixture(t, data.ProgramPortfolio.Entries)
			if err := BindProgramPortfolioFiles(&data, data.ProgramPortfolio.DefaultTargetID, files); err != nil {
				t.Fatal(err)
			}
			if got := newBuilder().sharedJoin(); !reflect.DeepEqual(got, want) {
				t.Fatal("file-backed shared join changed exact held files, identities, holders or unreachable declarations")
			}
			last := files[len(files)-1].Filename
			raw, err := os.ReadFile(last)
			if err != nil {
				t.Fatal(err)
			}
			if bad == "missing" {
				if err := os.Remove(last); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(last, append(raw, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			builder := newBuilder()
			join := builder.sharedJoin()
			originalErr := builder.nativeFailure().err
			if originalErr == nil {
				t.Fatal("failed later native file became a successful shared join")
			}
			if err := os.WriteFile(last, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if builder.sharedJoin() != join || builder.nativeFailure().err != originalErr {
				t.Fatal("shared join retried or repaired a failed original native read")
			}
		})
	}
}

func TestNativeReaderCatalogueKeepsOrderingOwnershipAndOverviewScope(t *testing.T) {
	at := programindex.Location{Path: "shared.go", Line: 2, Column: 1}
	callable := func(id string, end int, unreachable bool) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: "shared", Location: &at, EndLine: end, Unreachable: unreachable}
	}
	first := programindex.Index{Target: programindex.Target{ID: "t2", TestSources: []string{"native-target-only.go"}}, Objects: []programindex.Object{
		{ID: "n1", Kind: programindex.ObjectPackage, Name: "app"}, {ID: "n2", Kind: programindex.ObjectModule, Name: "app"}, callable("n3", 4, true),
	}, Relations: []programindex.Relation{{Witnesses: []programindex.Witness{
		{Kind: "go_test_declaration", Location: &programindex.Location{Path: "build-test.go"}},
		{Kind: "other", Location: &programindex.Location{Path: "not-a-test.go"}},
	}}}}
	second := programindex.Index{Target: programindex.Target{ID: "t10"}, Objects: []programindex.Object{
		callable("n1", 7, false), {ID: "n2", Kind: programindex.ObjectFunction, Name: "unrun", Unreachable: true},
		{ID: "n3", Kind: programindex.ObjectPackage, Name: "second"},
	}}
	last := programindex.Index{Target: programindex.Target{ID: "t11"}, Objects: []programindex.Object{callable("n1", 0, false)}}
	for _, overviewFirst := range []bool{false, true} {
		builder := &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{last, second, first}}},
			indexes:  []groupindex.Index{{Target: programindex.Target{ID: "t2", TestSources: []string{"author-test.go"}}}},
			sections: []*pageSection{{programTargetID: "t2", Name: "entry", Label: "entry label"}}}
		var overview *pageBuilder
		if overviewFirst {
			overview = builder.overviewBuilder()
		}
		if got := builder.declarationEnd(at); got != 7 {
			t.Fatalf("last eligible original EndLine = %d, want 7", got)
		}
		key := groupindex.DeclarationKey(first.Objects[2])
		if got := builder.runByJoin(); got.keys[subjectKey("t2", "n3")] != key || !slices.Equal(got.programs[key], []string{"t10"}) {
			t.Fatalf("native run ownership changed: %+v", got)
		}
		if got := builder.packageOwners(); len(got) != 1 || !slices.Equal(got[0].packages, []string{"app", "app", "entry", "entry label"}) || got[0].section != builder.sections[0] {
			t.Fatalf("package ordering or section ownership changed: %+v", got)
		}
		if overview == nil {
			overview = builder.overviewBuilder()
		}
		if !reflect.DeepEqual(overview.testPaths, map[string]bool{"author-test.go": true, "build-test.go": true}) {
			t.Fatalf("overview testing scope changed: %v", overview.testPaths)
		}
		if overview.nativeCatalogue() != builder.nativeCatalogue() {
			t.Fatal("overview compiled a second native reader catalogue")
		}
	}
}

func TestNativeReaderCatalogueLatchesTheLastBadFile(t *testing.T) {
	for _, bad := range []string{"missing", "different-bytes"} {
		t.Run(bad, func(t *testing.T) {
			data := reportTwoTargetDataFixture(t)
			files := reportNativeFilesFixture(t, data.ProgramPortfolio.Entries)
			if err := BindProgramPortfolioFiles(&data, data.ProgramPortfolio.DefaultTargetID, files); err != nil {
				t.Fatal(err)
			}
			last := files[len(files)-1].Filename
			raw, err := os.ReadFile(last)
			if err != nil {
				t.Fatal(err)
			}
			if bad == "missing" {
				if err := os.Remove(last); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(last, append(raw, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			builder := &pageBuilder{data: &data}
			catalogue := builder.nativeCatalogue()
			originalErr := builder.nativeFailure().err
			if originalErr == nil {
				t.Fatal("failed complete native visit became a successful catalogue")
			}
			if err := os.WriteFile(last, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if builder.nativeCatalogue() != catalogue || builder.nativeFailure().err != originalErr {
				t.Fatal("helper silently retried or repaired a failed native visit")
			}
		})
	}
}

func TestSavedFileRestoreKeepsNativeOnlyRelationAndWitnessPaths(t *testing.T) {
	location := func(path string) *programindex.Location {
		return &programindex.Location{Path: path, Line: 1, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "python", Kind: "executable", Name: "paths", Selector: "paths",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "main.py"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "main", Kind: programindex.SeedCallable, Location: location("main.py")}}},
		Objects: []programindex.ObjectInput{{SourceRef: "main", Kind: programindex.ObjectFunction, Name: "main", Visibility: programindex.VisibilityPublic, Location: location("main.py")}},
		Relations: []programindex.RelationInput{{SourceRef: "unresolved", Kind: programindex.RelationCalls, FromRef: "main",
			Resolution: programindex.ResolutionUnresolved, TargetsObserved: 1, Location: location("unresolved.py"),
			Witnesses: []programindex.Witness{{Kind: "python_call", Location: location("witness.py")}}, WitnessesObserved: 1}},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 1, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := programindex.EncodeValidated(index)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, programindex.ArtifactFilename), raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	data := ReportData{ProgramPortfolio: &ProgramPortfolio{Version: ProgramPortfolioVersion, DefaultTargetID: index.Target.ID}}
	if err := readSavedFiles(&data, []savedFile{{Section: savedSectionProgramIndex, Path: programindex.ArtifactFilename, SHA256: hex.EncodeToString(digest[:])}}, root); err != nil {
		t.Fatal(err)
	}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(data.OpenablePaths, []string{"main.py", "unresolved.py", "witness.py"}) {
		t.Fatalf("saved restoration lost native-only paths: %q", data.OpenablePaths)
	}
}

func TestNativeSourcePathsKeepSeedsUnresolvedRelationsAndWitnesses(t *testing.T) {
	index := programindex.Index{Target: programindex.Target{
		Sources: []programindex.TargetSource{{Path: "source.py"}},
		Seeds:   []programindex.TargetSeed{{Location: &programindex.Location{Path: "seed.py"}}},
	}, Objects: []programindex.Object{{Location: &programindex.Location{Path: "object.py"}}},
		Relations: []programindex.Relation{{Location: &programindex.Location{Path: "unresolved.py"},
			Witnesses: []programindex.Witness{{Location: &programindex.Location{Path: "witness.py"}}}}}}
	var paths []string
	if err := visitNativeSourcePaths(index, func(path string) error { paths = append(paths, path); return nil }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{"source.py", "seed.py", "object.py", "unresolved.py", "witness.py"}) {
		t.Fatalf("native path inventory omitted original evidence: %q", paths)
	}
}

func TestFileBoundPathInventoryMatchesInlineAndRefusesChangedOriginal(t *testing.T) {
	for _, bad := range []string{"missing", "different-bytes"} {
		t.Run(bad, func(t *testing.T) {
			data := reportTwoTargetDataFixture(t)
			if err := collectOpenablePaths(&data); err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(data.OpenablePaths)
			files := reportNativeFilesFixture(t, data.ProgramPortfolio.Entries)
			if err := BindProgramPortfolioFiles(&data, data.ProgramPortfolio.DefaultTargetID, files); err != nil {
				t.Fatal(err)
			}
			if err := collectOpenablePaths(&data); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(data.OpenablePaths, want) {
				t.Fatal("file binding changed complete native source paths")
			}
			last := files[len(files)-1].Filename
			if bad == "missing" {
				if err := os.Remove(last); err != nil {
					t.Fatal(err)
				}
			} else {
				stream, err := os.OpenFile(last, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := stream.WriteString("\n")
				closeErr := stream.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("write=%v close=%v", writeErr, closeErr)
				}
			}
			if err := collectOpenablePaths(&data); err == nil {
				t.Fatal("cached paths hid a changed or missing original")
			}
			if !slices.Equal(data.OpenablePaths, want) {
				t.Fatal("failed path read installed a partial inventory")
			}
		})
	}
}

func reportNativeFilesFixture(t *testing.T, indexes []programindex.Index) []ProgramIndexFile {
	t.Helper()
	root := t.TempDir()
	var files []ProgramIndexFile
	for _, index := range indexes {
		raw, err := programindex.EncodeValidated(index)
		if err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(root, index.Target.ID+".json")
		if err := os.WriteFile(filename, raw, 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, ProgramIndexFile{Filename: filename, Target: index.Target.Snapshot(), SHA256: index.SHA256})
	}
	return files
}

func TestProgramPortfolioFilesVisitCompleteNativeValuesWithoutRetainingBodies(t *testing.T) {
	data := reportTwoTargetDataFixture(t)
	want := append([]programindex.Index(nil), data.ProgramPortfolio.Entries...)
	files := reportNativeFilesFixture(t, want)
	if err := BindProgramPortfolioFiles(&data, want[1].Target.ID, files); err != nil {
		t.Fatal(err)
	}
	if data.ProgramPortfolio.Len() != len(want) || len(data.ProgramPortfolio.Entries) != 0 ||
		data.defaultProgramIndex != nil || len(data.programIndexes) != 0 {
		t.Fatal("file binding retained the complete native set")
	}
	// The caller's target metadata is mutable; the installed binding owns it.
	files[0].Target.Sources[0].Path = "changed-by-caller.py"
	var got []programindex.Index
	if err := data.ProgramPortfolio.ReadProgramIndexes(func(index programindex.Index) error {
		got = append(got, index)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatal("complete native target set changed")
	}
	for i := range want {
		assertSameNativeArtifact(t, got[i], want[i])
	}
	first, found, err := data.ProgramPortfolio.readTarget(want[0].Target.ID)
	if err != nil || !found {
		t.Fatalf("first read: found=%v error=%v", found, err)
	}
	first.Objects[0].Name = "changed-decoded-value"
	first.Target.Sources[0].Path = "changed-decoded-source.py"
	second, found, err := data.ProgramPortfolio.readTarget(want[0].Target.ID)
	if err != nil || !found {
		t.Fatal("later read reused a mutable decoded native value")
	}
	assertSameNativeArtifact(t, second, want[0])
}

func assertSameNativeArtifact(t *testing.T, got, want programindex.Index) {
	t.Helper()
	gotBytes, err := programindex.EncodeValidated(got)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := programindex.EncodeValidated(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatal("complete native artifact changed")
	}
}

func TestProgramPortfolioMixedValidationChecksMutableInlineAndOriginalFile(t *testing.T) {
	data := reportTwoTargetDataFixture(t)
	indexes := data.ProgramPortfolio.Entries
	files := reportNativeFilesFixture(t, indexes[:1])
	if err := BindProgramPortfolioFiles(&data, indexes[0].Target.ID, files); err != nil {
		t.Fatal(err)
	}
	data.ProgramPortfolio.Entries = []programindex.Index{indexes[1].Snapshot()}
	if err := data.ProgramPortfolio.Validate(); err != nil {
		t.Fatal(err)
	}
	data.ProgramPortfolio.Entries[0].Objects[0].Name += "changed"
	if err := data.ProgramPortfolio.Validate(); err == nil {
		t.Fatal("mixed portfolio accepted a changed public inline graph")
	}
	data.ProgramPortfolio.Entries[0] = indexes[1].Snapshot()
	if err := os.Remove(files[0].Filename); err != nil {
		t.Fatal(err)
	}
	if err := data.ProgramPortfolio.Validate(); err == nil {
		t.Fatal("mixed portfolio accepted a missing original file")
	}
}

func TestProgramPortfolioFilesRefuseTheLastBadArtifactWithoutReplacingAcceptedBinding(t *testing.T) {
	for _, bad := range []string{"missing", "changed-seal", "wrong-target", "duplicate", "missing-default"} {
		t.Run(bad, func(t *testing.T) {
			data := reportTwoTargetDataFixture(t)
			accepted := data.ProgramPortfolio
			files := reportNativeFilesFixture(t, accepted.Entries)
			defaultID := accepted.DefaultTargetID
			last := len(files) - 1
			switch bad {
			case "missing":
				if err := os.Remove(files[last].Filename); err != nil {
					t.Fatal(err)
				}
			case "changed-seal":
				raw, err := os.ReadFile(files[last].Filename)
				if err != nil {
					t.Fatal(err)
				}
				raw = bytes.Replace(raw, []byte(`"name":"`), []byte(`"name":"edited-`), 1)
				if err := os.WriteFile(files[last].Filename, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong-target":
				files[last].Target.Name += "other"
			case "duplicate":
				files[last] = files[0]
			case "missing-default":
				defaultID = "t999"
			}
			if err := BindProgramPortfolioFiles(&data, defaultID, files); err == nil || data.ProgramPortfolio != accepted {
				t.Fatal("invalid complete binding replaced the accepted native set")
			}
		})
	}
}

func TestProgramPortfolioFilesRenderTheSamePageAndRefuseLaterArtifactLoss(t *testing.T) {
	for _, bad := range []string{"missing", "different-bytes"} {
		t.Run(bad, func(t *testing.T) {
			data := reportProgramShellDataFixture(t, "file-backed-native")
			options := reportSingleTargetRenderOptionsFixture(t, &data)
			want, err := RenderHTMLWithOptions(&data, options)
			if err != nil {
				t.Fatal(err)
			}
			files := reportNativeFilesFixture(t, data.ProgramPortfolio.Entries)
			if err := BindProgramPortfolioFiles(&data, data.ProgramPortfolio.DefaultTargetID, files); err != nil {
				t.Fatal(err)
			}
			got, err := RenderHTMLWithOptions(&data, options)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("native file binding changed the complete rendered page")
			}
			switch bad {
			case "missing":
				if err := os.Remove(files[0].Filename); err != nil {
					t.Fatal(err)
				}
			case "different-bytes":
				raw, err := os.ReadFile(files[0].Filename)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(files[0].Filename, append(raw, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := RenderHTMLWithOptions(&data, options); err == nil {
				t.Fatal("native file loss became a partial page")
			}
			if _, err := encodeReportJSON(&data, 0); err == nil {
				t.Fatal("native file loss became a partial saved report")
			}
			// Non-error helpers must latch the same error, too, so a later
			// failure inside page construction cannot turn into absent evidence.
			builder := &pageBuilder{data: &data}
			if got := builder.nativeTargets(files[0].Target.ID); len(got) != 0 || builder.nativeFailure().err == nil {
				t.Fatal("native helper hid the read error")
			}
		})
	}
}

func TestProgramPortfolioFilesKeepTheCompleteTwoTargetPageAndReleaseTargetCaches(t *testing.T) {
	data := reportTwoTargetDataFixture(t)
	var pages []TargetNavigationPage
	for i, index := range data.ProgramPortfolio.Entries {
		runID := []string{"20260810-120000-page-a1b2c3", "20260810-120000-page-d4e5f6"}[i]
		pages = append(pages, TargetNavigationPage{RunID: runID, ProgramTarget: index.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename})
	}
	navigation, err := BuildTargetNavigation(pages, data.ProgramPortfolio.DefaultTargetID, data.ProgramPortfolio.DefaultTargetID)
	if err != nil {
		t.Fatal(err)
	}
	options := RenderOptions{TargetNavigation: navigation}
	want, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}
	files := reportNativeFilesFixture(t, data.ProgramPortfolio.Entries)
	if err := BindProgramPortfolioFiles(&data, data.ProgramPortfolio.DefaultTargetID, files); err != nil {
		t.Fatal(err)
	}
	got, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("file binding changed the complete cross-target page")
	}
	builder := &pageBuilder{data: &data}
	first := builder.nativeTargets(files[0].Target.ID)
	if len(first) != 1 {
		t.Fatal("first native target unavailable")
	}
	builder.callsFrom = map[string]map[string][]programindex.Relation{files[0].Target.ID: {"calls": first[0].Relations}}
	builder.dataByTarget = map[string]*pageDataFacts{files[0].Target.ID: {relations: map[string]programindex.Relation{first[0].Relations[0].ID: first[0].Relations[0]}}}
	builder.fieldsByTarget = map[string]*pageFieldFacts{files[0].Target.ID: {}}
	builder.ownUseReader = groupindex.NewOwnUseReader(first[0])
	second := builder.nativeTargets(files[1].Target.ID)
	if len(second) != 1 || len(builder.callsFrom) != 0 || len(builder.dataByTarget) != 0 || len(builder.fieldsByTarget) != 0 || builder.ownUseReader != nil {
		t.Fatal("switching target retained previous native relation caches")
	}
	back := builder.nativeTargets(files[0].Target.ID)
	if len(back) != 1 {
		t.Fatal("returning to the first target lost its native graph")
	}
	assertSameNativeArtifact(t, first[0], back[0])
	if err := os.Remove(files[1].Filename); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderHTMLWithOptions(&data, options); err == nil {
		t.Fatal("missing later target became a partial cross-target page")
	}
}
