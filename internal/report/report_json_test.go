package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/terminology"
)

// report.json is compact, names the files of its run directory that hold a
// section byte for byte instead of repeating them, keeps the other target's
// ProgramIndex in its artifact encoding, and reads back to exactly the data
// it was written from.
func TestReportJSONNamesItsRunDirectoryFilesAndReadsBackExactly(t *testing.T) {
	data := reportTwoTargetDataFixture(t)
	runDir := t.TempDir()
	data.ArtifactsDir = runDir
	owner, other := data.ProgramPortfolio.Entries[0], data.ProgramPortfolio.Entries[1]
	writeReportProgramIndexArtifacts(t, runDir, owner)
	data.Facts = &facts.Result{Version: 1, Revision: data.CapturedRevision, SHA256: strings.Repeat("f", 64)}
	data.Claims = &claims.Result{Version: 1, Revision: data.CapturedRevision, Claims: []claims.Claim{{
		ID: "h1", Source: claims.SourceReadme, Path: "README.md", Line: 1, Text: "Runs <the> fixture & more.",
	}}, SHA256: strings.Repeat("c", 64)}
	data.Orientation = &orientation.Result{Version: 1, Summary: "Runs the fixture."}
	data.Glossary = &terminology.Catalog{Version: terminology.CatalogVersion, Entries: []terminology.Entry{}, SHA256: strings.Repeat("g", 64)}
	writeReportJSONFile(t, runDir, facts.ArtifactFilename, data.Facts, "")
	writeReportJSONFile(t, runDir, claims.ArtifactFilename, data.Claims, "")
	writeReportJSONFile(t, runDir, terminology.CatalogFilename, data.Glossary, "\n")
	// A file that holds the section in other bytes is not the section's copy.
	pretty, err := json.MarshalIndent(data.Orientation, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, orientation.ArtifactFilename), pretty)

	encoded, err := encodeReportJSON(&data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(encoded, []byte("\n")) != 1 || !bytes.HasSuffix(encoded, []byte("}\n")) {
		t.Fatal("report.json is not one compact line")
	}
	var wire struct {
		Files            []savedFile    `json:"files"`
		ProgramPortfolio savedPortfolio `json:"program_portfolio"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range wire.Files {
		names = append(names, file.Section+":"+file.Path)
	}
	if want := []string{
		"program_index:" + programindex.ArtifactFilename, "facts:" + facts.ArtifactFilename,
		"claims:" + claims.ArtifactFilename, "glossary:" + terminology.CatalogFilename,
	}; !reflect.DeepEqual(names, want) {
		t.Fatalf("named files = %v, want %v", names, want)
	}
	otherArtifact, err := programindex.EncodeValidated(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.ProgramPortfolio.Entries) != 1 || !bytes.Equal(wire.ProgramPortfolio.Entries[0], otherArtifact) {
		t.Fatal("the other target's ProgramIndex is not written in its artifact encoding")
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &sections); err != nil {
		t.Fatal(err)
	}
	for _, named := range []string{"facts", "claims", "glossary"} {
		if sections[named] != nil {
			t.Fatalf("named section %s is written as well", named)
		}
	}
	if sections["orientation"] == nil {
		t.Fatal("orientation, whose file holds other bytes, is not written")
	}

	restored, err := decodeStrictReportJSON(encoded, runDir)
	if err != nil {
		t.Fatal(err)
	}
	assertSameReportData(t, restored, data)

	inlineData := data
	inlineData.ArtifactsDir = ""
	inline, err := encodeReportJSON(&inlineData, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inline, []byte(`"files"`)) || len(inline) <= len(encoded) {
		t.Fatal("without a run directory report.json named files")
	}
	restoredInline, err := decodeStrictReportJSON(inline, "")
	if err != nil {
		t.Fatal(err)
	}
	assertSameReportData(t, restoredInline, data)

	if _, err := decodeStrictReportJSON(encoded, ""); err == nil {
		t.Fatal("report.json naming files was read without its run directory")
	}
	writeReportJSONFile(t, runDir, claims.ArtifactFilename, data.Claims, "\n")
	if _, err := decodeStrictReportJSON(encoded, runDir); err == nil ||
		!strings.Contains(err.Error(), "is not the file report.json was written with") {
		t.Fatalf("changed named file = %v", err)
	}
}

// Another target's ProgramIndex that is byte for byte the program-index.json
// of its own run directory, one the run's program page portfolio names, is
// named by its path from the run directory instead of copied. The report
// reads back exactly and is refused once that file is changed or missing.
func TestReportJSONNamesTheOtherTargetsRunDirectoryFile(t *testing.T) {
	data := reportTwoTargetDataFixture(t)
	owner, other := data.ProgramPortfolio.Entries[0], data.ProgramPortfolio.Entries[1]
	runs := t.TempDir()
	runDir, otherRunID := filepath.Join(runs, "20260810-120000-page-a1b2c3"), "20260810-120000-page-d4e5f6"
	otherDir := filepath.Join(runs, otherRunID)
	for _, dir := range []string{runDir, otherDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	data.ArtifactsDir = runDir
	writeReportProgramIndexArtifacts(t, runDir, owner)
	writeReportProgramIndexArtifacts(t, otherDir, other)
	pages, err := programpage.Build(owner.Target.ID, []programpage.Page{
		{Target: owner.Target.Snapshot(), RunID: filepath.Base(runDir)},
		{Target: other.Target.Snapshot(), RunID: otherRunID},
	})
	if err != nil {
		t.Fatal(err)
	}
	pagesJSON, err := pages.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programpage.ArtifactFilename), pagesJSON)

	encoded, err := encodeReportJSON(&data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Files            []savedFile    `json:"files"`
		ProgramPortfolio savedPortfolio `json:"program_portfolio"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	otherPath := "../" + otherRunID + "/" + programindex.ArtifactFilename
	otherBytes, err := os.ReadFile(filepath.Join(otherDir, programindex.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	otherDigest := sha256.Sum256(otherBytes)
	if len(wire.Files) != 2 || wire.Files[0].Path != programindex.ArtifactFilename ||
		wire.Files[1] != (savedFile{Section: savedSectionProgramIndex, Path: otherPath, SHA256: hex.EncodeToString(otherDigest[:])}) {
		t.Fatalf("named files = %+v", wire.Files)
	}
	if len(wire.ProgramPortfolio.Entries) != 0 {
		t.Fatal("a ProgramIndex its target's run directory holds is written as well")
	}
	restored, err := decodeStrictReportJSON(encoded, runDir)
	if err != nil {
		t.Fatal(err)
	}
	assertSameReportData(t, restored, data)

	writeReportProgramFile(t, filepath.Join(otherDir, programindex.ArtifactFilename), append(otherBytes, '\n'))
	if _, err := decodeStrictReportJSON(encoded, runDir); err == nil ||
		!strings.Contains(err.Error(), otherPath+" is not the file report.json was written with") {
		t.Fatalf("changed file of the other target's run directory = %v", err)
	}
	if err := os.Remove(filepath.Join(otherDir, programindex.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStrictReportJSON(encoded, runDir); err == nil ||
		!strings.Contains(err.Error(), "read "+otherPath+" named by report.json") {
		t.Fatalf("missing file of the other target's run directory = %v", err)
	}
}

// assertSameReportData compares everything report.json persists, each
// ProgramIndex with every location spelled out.
func assertSameReportData(t *testing.T, got, want ReportData) {
	t.Helper()
	// Compare complete native values, independent of whether the existing
	// saved format names their original files or holds their bytes inline.
	for _, data := range []*ReportData{&got, &want} {
		portfolio := *data.ProgramPortfolio
		portfolio.Entries = nil
		if err := data.ProgramPortfolio.ReadProgramIndexes(func(index programindex.Index) error {
			portfolio.Entries = append(portfolio.Entries, index)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		portfolio.files = nil
		data.ProgramPortfolio = &portfolio
	}
	gotJSON, err := json.Marshal(reportDataForPersistence(&got))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(reportDataForPersistence(&want))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("read back report data differs:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	if len(got.GroupGraph.hydrated) != len(want.ProgramPortfolio.Entries) {
		t.Fatal("group overlays were not hydrated from the ProgramIndexes")
	}
}

func writeReportJSONFile(t *testing.T, runDir, name string, value any, suffix string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, name), append(encoded, suffix...))
}

func reportTwoTargetDataFixture(t *testing.T) ReportData {
	t.Helper()
	rebound, err := programindex.RebindTargetSet([]programindex.Index{
		reportProgramIndexFixture(t, "python", "executable"),
		reportProgramIndexFixture(t, "go", "library"),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, firstGroups, _ := reportFinalGraphFixture(t, rebound[0])
	second, secondGroups, _ := reportFinalGraphFixture(t, rebound[1])
	portfolio, err := NewProgramPortfolio(first.Target.ID, []programindex.Index{first, second})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView([]groupindex.Index{firstGroups, secondGroups}, first.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture", CapturedRevision: strings.Repeat("a", 40),
		ProgramPortfolio: portfolio, GroupGraph: graph,
		TargetOutcomePortfolio: reportTargetOutcomeViewFixture(t, []TargetNavigationPage{
			{RunID: "20260810-120000-page-a1b2c3", ProgramTarget: first.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename},
			{RunID: "20260810-120000-page-d4e5f6", ProgramTarget: second.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename},
		}, first.Target.ID),
	}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	return data
}
