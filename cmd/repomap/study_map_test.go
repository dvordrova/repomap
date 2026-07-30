package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestStudyMapDiversePackagePathsPrefersUncoveredProductionFiles(t *testing.T) {
	t.Parallel()

	data := &report.ReportData{
		OpenablePaths: []string{
			"a.go", "b.go", "c.go", "middleware/one.go", "middleware/two.go",
			"_examples/demo/main.go", "core_test.go",
		},
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{
			{CanonicalPath: "example/core", Dir: ".", Files: []string{"a.go", "b.go", "c.go", "core_test.go"}},
			{CanonicalPath: "example/middleware", Dir: "middleware", Files: []string{"middleware/one.go", "middleware/two.go"}},
			{CanonicalPath: "demo", Dir: "_examples/demo", Files: []string{"_examples/demo/main.go"}},
		}},
	}
	existing := []freshSourceFunction{{Function: sourcewindowFunctionPath("a.go")}}
	got := studyMapDiversePackagePaths(data, existing, 4)
	want := []string{"b.go", "middleware/one.go", "c.go", "middleware/two.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("diverse package paths = %v, want %v", got, want)
	}
	for _, filePath := range got {
		if !artifactrole.IsProduction(artifactrole.Classify(filePath, artifactrole.Hints{})) {
			t.Fatalf("non-production file selected: %q", filePath)
		}
	}
}

func TestStudyMapMechanismPathsRemainAuthorizedAndAttachable(t *testing.T) {
	t.Parallel()

	anchors := []studymap.Anchor{{
		ID: "fact-browse", Path: "modules/caddyhttp/fileserver/browse.go",
		Function: sourcewindowfacts.Function{
			Path: "modules/caddyhttp/fileserver/browse.go", StartLine: 1, EndLine: 20,
		},
	}}
	data := &report.ReportData{UserMechanisms: []report.UserMechanism{{
		ArtifactID: "mechanism-browse", Title: "Browse", Question: "How are listings built?",
		Files: []report.UserCodeLocation{
			{Path: "modules/caddyhttp/fileserver/browse.go", Line: 10},
			{Path: "modules/caddyhttp/fileserver/template.go", Line: 20},
			{Path: "private/not-openable.go", Line: 1},
		},
	}}}
	openable := map[string]struct{}{
		"modules/caddyhttp/fileserver/browse.go":   {},
		"modules/caddyhttp/fileserver/template.go": {},
	}
	mechanisms := studyMapMechanisms(data, anchors, openable)
	if len(mechanisms) != 1 || !slices.Equal(mechanisms[0].AnchorIDs, []string{"fact-browse"}) ||
		!slices.Equal(mechanisms[0].Paths, []string{
			"modules/caddyhttp/fileserver/browse.go",
			"modules/caddyhttp/fileserver/template.go",
		}) {
		t.Fatalf("mechanisms = %#v", mechanisms)
	}
	allowed := studyMapAllowedPaths(anchors, nil, nil, mechanisms)
	if !slices.Contains(allowed, "modules/caddyhttp/fileserver/template.go") ||
		slices.Contains(allowed, "private/not-openable.go") {
		t.Fatalf("allowed paths = %v", allowed)
	}
}

func TestStudyMapAreasPreferExactProductionShape(t *testing.T) {
	t.Parallel()

	openable := map[string]struct{}{
		"core.go": {}, "tree.go": {}, "middleware/chain.go": {}, "_examples/demo/main.go": {},
	}
	data := &report.ReportData{
		ReportLanguage: "ru",
		Components: []report.Component{{
			ID: "component-core", Name: "Core Router", ModelPurpose: "Matches requests.",
			AnchorGroups: []report.AnchorGroup{{ID: "anchor-core", Path: "core.go"}},
		}},
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{
			{CanonicalPath: "example/router", Name: "router", Dir: ".", Files: []string{"core.go", "tree.go"}},
			{CanonicalPath: "example/router/middleware", Name: "middleware", Dir: "middleware", Files: []string{"middleware/chain.go"}},
			{CanonicalPath: "demo", Name: "main", Dir: "_examples/demo", Files: []string{"_examples/demo/main.go"}},
		}},
	}
	areas, areaPaths := studyMapAreas(data, openable)
	if len(areas) != 3 {
		t.Fatalf("production areas = %#v", areas)
	}
	for _, area := range areas {
		if studyMapPeripheralArea(area.Name) || len(areaPaths[area.ID]) == 0 || !studyMapPathProduction(area.Path) {
			t.Fatalf("non-production or non-navigable area = %#v paths=%v", area, areaPaths[area.ID])
		}
		if strings.HasPrefix(area.ID, "package-area-") &&
			!strings.HasPrefix(area.Responsibility, "Основной пакет ") {
			t.Fatalf("Russian package area was not localized: %#v", area)
		}
	}
}

func TestEnsureStudyMapAreasUsesOneLocalizedExactSymbol(t *testing.T) {
	t.Parallel()

	areas, paths := ensureStudyMapAreas(
		nil,
		make(map[string][]string),
		[]studymap.Anchor{
			{ID: "write-batch", Path: "wal.go", Symbol: "Log.writeBatch", Line: 458},
			{ID: "sync", Path: "wal.go", Symbol: "Log.Sync", Line: 319},
		},
		"ru",
	)
	if len(areas) != 1 {
		t.Fatalf("areas = %#v, want one honest area without three-area padding", areas)
	}
	if areas[0].Name != "Log.writeBatch" ||
		areas[0].Responsibility != "Точная область кода для изучения поведения вокруг Log.writeBatch." ||
		areas[0].Path != "wal.go" || areas[0].Line != 458 {
		t.Fatalf("localized exact area = %#v", areas[0])
	}
	if !slices.Equal(paths[areas[0].ID], []string{"wal.go"}) {
		t.Fatalf("area paths = %#v", paths)
	}
}

func TestStudyMapDiverseSourceFunctionsBuildsExactLocalFacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	files := map[string]string{
		"api.go":   "package fixture\n\nfunc Serve() int {\n\tvalue := prepare()\n\treturn value\n}\n\nfunc prepare() int { return 1 }\n",
		"tree.go":  "package fixture\n\nfunc FindRoute() bool {\n\tfound := lookup()\n\treturn found\n}\n\nfunc lookup() bool { return true }\n",
		"write.go": "package fixture\n\nfunc WriteResult() error {\n\treturn persist()\n}\n\nfunc persist() error { return nil }\n",
	}
	var openable []string
	for filePath, source := range files {
		writeFile(t, filepath.Join(repoRoot, filePath), source)
		openable = append(openable, filePath)
	}
	data := &report.ReportData{
		DocumentedPurpose: "Serve requests and write results.",
		OpenablePaths:     openable,
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{{
			CanonicalPath: "example/fixture", Dir: ".", Files: openable,
		}}},
	}
	got := studyMapDiverseSourceFunctions(repoRoot, data, nil)
	if len(got) < 3 {
		t.Fatalf("diverse source functions = %#v", got)
	}
	paths := make(map[string]struct{})
	for _, source := range got {
		if err := source.Function.Validate(); err != nil {
			t.Fatalf("function %s: %v", source.Function.Path, err)
		}
		if source.Fact.ID == "" || len(source.Fact.Evidence) == 0 || source.Fact.Source == nil {
			t.Fatalf("fact is not locally grounded: %#v", source.Fact)
		}
		paths[source.Function.Path] = struct{}{}
	}
	if len(paths) != 3 {
		t.Fatalf("diverse source paths = %v", paths)
	}
}

func TestStudyMapDiverseSourceFunctionsReservesExactMechanismAnchor(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filePath := "mechanism.go"
	writeFile(t, filepath.Join(repoRoot, filePath), "package fixture\n\nfunc Generic() int { return 1 }\n\nfunc Desired() int {\n\treturn 2\n}\n")
	data := &report.ReportData{
		OpenablePaths: []string{filePath},
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{{
			CanonicalPath: "example/fixture", Dir: ".", Files: []string{filePath},
		}}},
		UserMechanisms: []report.UserMechanism{{
			ArtifactID: "mechanism-desired", Title: "Desired", Question: "How does desired work?",
			Files: []report.UserCodeLocation{{Path: filePath, Line: 6}},
		}},
	}
	got := studyMapDiverseSourceFunctions(repoRoot, data, nil)
	found := false
	for _, source := range got {
		if source.Function.Path == filePath && source.Function.Symbol == "Desired" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("exact mechanism function was not reserved: %#v", got)
	}
}

func TestStudyMapDocumentsReadsBoundedAuthorizedExcerpt(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "README.md"), "# Fixture\n\nA concise current overview.\n")
	documents := studyMapDocuments(repoRoot, &report.ReportData{}, map[string]struct{}{"README.md": {}})
	if len(documents) != 1 || documents[0].Excerpt == "" ||
		len(documents[0].Excerpt) > studyMapMaxDocumentExcerptBytes {
		t.Fatalf("documents = %#v", documents)
	}
}

func TestStudyMapDocumentsDistributesExcerptBudgetAcrossSelectedDocs(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	openable := make(map[string]struct{}, studymap.MaxDocuments)
	for index := range studymap.MaxDocuments {
		filePath := "docs/guide-" + string(rune('a'+index)) + ".md"
		if index == 0 {
			filePath = "README.md"
		}
		writeFile(
			t,
			filepath.Join(repoRoot, filePath),
			"# Guide\n\n"+strings.Repeat("This document has enough saved text for fair allocation.\n", 40),
		)
		openable[filePath] = struct{}{}
	}

	documents := studyMapDocuments(repoRoot, &report.ReportData{}, openable)
	if len(documents) != studymap.MaxDocuments {
		t.Fatalf("documents = %d, want %d", len(documents), studymap.MaxDocuments)
	}
	total := 0
	perDocumentLimit := studyMapMaxDocumentExcerptTotal / studymap.MaxDocuments
	for _, document := range documents {
		if document.Excerpt == "" {
			t.Fatalf("document %q did not receive a reserved excerpt", document.Path)
		}
		if len(document.Excerpt) > perDocumentLimit {
			t.Fatalf("document %q excerpt = %d, want <= %d", document.Path, len(document.Excerpt), perDocumentLimit)
		}
		total += len(document.Excerpt)
	}
	if total > studyMapMaxDocumentExcerptTotal {
		t.Fatalf("total excerpt bytes = %d, want <= %d", total, studyMapMaxDocumentExcerptTotal)
	}
}

func TestStudyMapRecoverySkipsOnlySyntacticallyIncompleteWindow(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runDir := t.TempDir()
	goodSource := "package fixture\n\nfunc Good() int {\n\tvalue := helper()\n\treturn value\n}\n\nfunc helper() int { return 1 }\n"
	badSource := "package fixture\n\nfunc Bad() string {\n\tvalue := `one\ntwo\nthree`\n\treturn value\n}\n"
	writeFile(t, filepath.Join(repoRoot, "good.go"), goodSource)
	writeFile(t, filepath.Join(repoRoot, "bad.go"), badSource)
	goodLocation := evidence.Location{Path: "good.go", Line: 3}
	badLocation := evidence.Location{Path: "bad.go", Line: 3}
	bundle := modelresearch.EvidenceBundle{
		Version: modelresearch.ContractVersion,
		RoundID: "research-study-recovery",
		Evidence: []modelresearch.EvidenceItem{
			{
				ID: "evidence-good", Kind: modelresearch.EvidenceSource,
				Statement: "bounded source window selected locally for the research question",
				Location:  &goodLocation, Certainty: evidence.CertaintyStatic,
				Window: &modelresearch.SourceWindow{
					StartLine: 3, EndLine: 6,
					Lines:       []string{"func Good() int {", "\tvalue := helper()", "\treturn value", "}"},
					CodeBearing: true,
				},
			},
			{
				ID: "evidence-bad", Kind: modelresearch.EvidenceSource,
				Statement: "bounded source window selected locally for the research question",
				Location:  &badLocation, Certainty: evidence.CertaintyStatic,
				Window: &modelresearch.SourceWindow{
					StartLine: 3, EndLine: 4,
					Lines:       []string{"func Bad() string {", "\tvalue := `one"},
					CodeBearing: true,
				},
			},
		},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundleDir := filepath.Join(runDir, "research", bundle.RoundID)
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "evidence_bundle.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	got := studyMapRecoverSavedSourceFunctions(runDir, repoRoot)
	if len(got) != 1 || got[0].Function.Path != "good.go" || got[0].Function.Symbol != "Good" {
		t.Fatalf("recovered source functions = %#v", got)
	}
}

func TestBuildStudyMapBundleUsesExactPythonDeclarationFallback(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runDir := t.TempDir()
	filePath := "beets/ui/commands/import_/__init__.py"
	lines := []string{
		"# bounded saved source",
		"def import_files(session) -> None:",
		"    session.run()",
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoRoot, filePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repoRoot, filePath), strings.Join(lines, "\n")+"\n")
	location := evidence.Location{Path: filePath, Line: 40}
	state := modelresearch.NewState(
		modelresearch.DefaultPolicy(),
		modelresearch.RepositoryContext{Identity: "beets-fixture", Revision: "fixture", Scenario: "default"},
	)
	state.Theory.GroundedFacts = []modelresearch.EvidenceItem{{
		ID: "python-import-window", Kind: modelresearch.EvidenceSource,
		Statement: "bounded source window selected locally for the research question",
		Location:  &location, Certainty: evidence.CertaintyStatic,
		Window: &modelresearch.SourceWindow{
			StartLine: 40, EndLine: 42, Lines: lines, CodeBearing: true,
		},
		Provenance: []evidence.Provenance{{
			Provider: "reporead", Operation: "read_bounded_source_window", Location: &location,
		}},
	}}
	if err := modelresearch.WriteState(runDir, state); err != nil {
		t.Fatal(err)
	}
	data := &report.ReportData{
		RepoName: "beets-fixture", OpenablePaths: []string{filePath},
	}
	bundle, err := buildStudyMapBundle(runDir, repoRoot, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Anchors) != 1 {
		t.Fatalf("anchors = %#v", bundle.Anchors)
	}
	anchor := bundle.Anchors[0]
	if anchor.Path != filePath || anchor.Symbol != "import_files" || anchor.Line != 41 ||
		anchor.ExactSource == nil || anchor.ExactSource.Language != "python" ||
		anchor.ExactSource.Path != filePath || anchor.ExactSource.Symbol != "import_files" ||
		anchor.ExactSource.Line != 41 || !slices.Equal(anchor.ExactSource.Lines, []string{lines[1]}) ||
		anchor.Function.Path != "" {
		t.Fatalf("Python anchor = %#v", anchor)
	}
	if _, err := studymap.BundleHash(bundle); err != nil {
		t.Fatalf("BundleHash() error = %v", err)
	}
	promptRaw, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(promptRaw), lines[1]) || len(bundle.Mechanisms) != 0 {
		t.Fatalf("Python fallback invented source or a complete mechanism: %s %#v", promptRaw, bundle.Mechanisms)
	}
}

func TestBuildStudyMapBundleFromSavedRun(t *testing.T) {
	runDir := os.Getenv("REPOMAP_STUDY_MAP_TEST_RUN")
	repoRoot := os.Getenv("REPOMAP_STUDY_MAP_TEST_REPO")
	if runDir == "" || repoRoot == "" {
		t.Skip("set REPOMAP_STUDY_MAP_TEST_RUN and REPOMAP_STUDY_MAP_TEST_REPO for an offline saved-run check")
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := buildStudyMapBundle(runDir, repoRoot, data)
	if err != nil {
		t.Fatal(err)
	}
	production := 0
	for _, anchor := range bundle.Anchors {
		if artifactrole.IsProduction(anchor.Role) {
			production++
		}
	}
	if len(bundle.Anchors) < 3 || len(bundle.Areas) < 3 || production == 0 {
		t.Fatalf("saved-run Study Map bundle = %d anchors, %d areas, %d production anchors", len(bundle.Anchors), len(bundle.Areas), production)
	}
	if len(bundle.Anchors) > studyMapMaxSourceFunctions || len(bundle.Areas) > studymap.MaxAreas {
		t.Fatalf("saved-run Study Map bundle exceeded bounds: %#v", bundle)
	}
	t.Logf("saved-run Study Map bundle: %d anchors, %d areas, %d production anchors", len(bundle.Anchors), len(bundle.Areas), production)
}

func TestReplayStudyMapAttemptFromSavedRun(t *testing.T) {
	runDir := os.Getenv("REPOMAP_STUDY_MAP_TEST_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_STUDY_MAP_TEST_RUN for an offline saved-response replay")
	}
	bundleRaw, err := os.ReadFile(filepath.Join(runDir, studymap.BundleFile))
	if err != nil {
		t.Fatal(err)
	}
	var bundle studymap.Bundle
	if err := json.Unmarshal(bundleRaw, &bundle); err != nil {
		t.Fatal(err)
	}
	attemptRaw, err := os.ReadFile(filepath.Join(runDir, studymap.AttemptFile))
	if err != nil {
		t.Fatal(err)
	}
	var attempt studyMapAttempt
	if err := json.Unmarshal(attemptRaw, &attempt); err != nil {
		t.Fatal(err)
	}
	proposal, err := studymap.DecodeProposal(attempt.Response)
	if err != nil {
		t.Fatal(err)
	}
	record, err := studymap.BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Directions) == 0 || len(record.Directions) > studymap.MaxDirections {
		t.Fatalf("offline replay selected %d directions", len(record.Directions))
	}
	questions := make([]string, 0, len(record.Directions))
	for _, direction := range record.Directions {
		questions = append(questions, direction.Question)
	}
	t.Logf("offline replay selected %d direction(s): %v", len(record.Directions), questions)
}

func sourcewindowFunctionPath(filePath string) sourcewindowfacts.Function {
	return sourcewindowfacts.Function{Path: filePath, Symbol: "existing"}
}
