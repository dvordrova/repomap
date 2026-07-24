package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

func TestCatalogExactSearchPreservesCurrentSerializedIndex(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	data.OpenablePaths = append(data.OpenablePaths, "README.md")
	sort.Strings(data.OpenablePaths)
	data.ArchitectureCanvas.Components[0].Members[0].Facts = []componentmap.LocalFact{{
		Kind:      componentmap.FactDeclaration,
		Value:     "CollectFacts",
		Certainty: evidence.CertaintyStatic,
		Location: &evidence.Location{
			Path: "internal/report/report.go", Line: 88, Column: 1,
		},
	}}
	data.ArchitectureCanvas.BehaviorAnchors = []componentmap.BehaviorAnchor{{
		ID:       "anchor-collect",
		Kind:     componentmap.AnchorCommandDispatch,
		Label:    "CollectFacts dispatch",
		Location: evidence.Location{Path: "cmd/repomap/main.go", Line: 42, Column: 1},
	}}

	legacy, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("legacy BuildSemanticSearchIndex: %v", err)
	}
	catalog := exactSearchTestCatalog(t, data.OpenablePaths)
	migrated, err := BuildSemanticSearchIndexWithCatalog(data, catalog)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndexWithCatalog: %v", err)
	}
	if !reflect.DeepEqual(migrated, legacy) {
		t.Fatalf("catalog adapter changed search index:\nlegacy:   %#v\nmigrated: %#v", legacy, migrated)
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	migratedJSON, err := json.Marshal(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if string(migratedJSON) != string(legacyJSON) {
		t.Fatalf("serialized search index changed:\nlegacy:   %s\nmigrated: %s", legacyJSON, migratedJSON)
	}

	assertExactSearchItem(t, migrated.Items, SemanticSearchKindLocation, "internal/report/report.go",
		"location:194065e66aa65f4c4b3f2862eb55e16cffbd9fd4550a740bd47558babdc373cf",
		SemanticSearchTargetLocation)
	document := assertExactSearchItem(t, migrated.Items, SemanticSearchKindLocation, "README.md",
		"location:e9e4ad82596db5a0b90a70d0108687a861487726eb56cf4f51d7dfd503b03edf",
		SemanticSearchTargetLocation)
	if len(document.Aliases) != 0 || document.Summary != "" || document.Question != "" ||
		document.Target.Location == nil || document.Target.Location.Path != "README.md" {
		t.Fatalf("document wire item changed: %#v", document)
	}
	member := assertExactSearchItem(t, migrated.Items, SemanticSearchKindMember, "CollectFacts",
		"member:990bcba4591b7afa328819ef6ec102d0aee7f616dcf530204208d578b02cd80c",
		SemanticSearchTargetComponent)
	if member.Target.ComponentID != "component-analysis" ||
		!reflect.DeepEqual(member.Aliases, []string{"symbol"}) {
		t.Fatalf("member target/aliases changed: %#v", member)
	}
	anchor := findSemanticSearchItem(t, migrated.Items, "CollectFacts dispatch")
	if anchor.Kind != SemanticSearchKindAnchor || anchor.Target.Kind != SemanticSearchTargetLocation ||
		anchor.Target.Location == nil || anchor.Target.Location.Path != "cmd/repomap/main.go" {
		t.Fatalf("exact anchor target changed: %#v", anchor)
	}
}

func TestCatalogExactSearchKeepsExactAndEditorialInputsSeparated(t *testing.T) {
	t.Parallel()

	exactData := &ReportData{
		RepoName:      "exact-only",
		OpenablePaths: []string{"README.md", "service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{
				ID:   "service",
				Name: "Service",
				Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "run"},
					Name: "Run",
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactDeclaration, Value: "Run",
						Location: &evidence.Location{Path: "service.go", Line: 3, Column: 1},
					}},
				}},
			}},
		},
	}
	exactIndex, err := BuildSemanticSearchIndexWithCatalog(
		exactData,
		exactSearchTestCatalog(t, exactData.OpenablePaths),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"README.md", "service.go", "Run"} {
		findSemanticSearchItem(t, exactIndex.Items, title)
	}
	for _, item := range exactIndex.Items {
		if item.Kind == SemanticSearchKindLocation &&
			(len(item.Aliases) != 0 || item.Summary != "" || item.Question != "") {
			t.Fatalf("exact path gained editorial fields: %#v", item)
		}
	}

	editorialData := semanticSearchTestReport()
	editorialData.OpenablePaths = nil
	legacyEditorial, err := BuildSemanticSearchIndex(editorialData)
	if err != nil {
		t.Fatal(err)
	}
	catalogEditorial, err := BuildSemanticSearchIndexWithCatalog(
		editorialData,
		exactSearchTestCatalog(t, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalogEditorial, legacyEditorial) {
		t.Fatalf("empty exact scope changed editorial search:\nlegacy: %#v\ncatalog: %#v", legacyEditorial, catalogEditorial)
	}
	findSemanticSearchItem(t, catalogEditorial.Items, "How facts become a report")

	hiddenArchitecture := semanticSearchTestReport()
	hiddenArchitecture.RepositoryGuide = &RepositoryGuide{ArchitectureUseful: false}
	legacyHidden, err := BuildSemanticSearchIndex(hiddenArchitecture)
	if err != nil {
		t.Fatal(err)
	}
	catalogHidden, err := BuildSemanticSearchIndexWithCatalog(
		hiddenArchitecture,
		exactSearchTestCatalog(t, hiddenArchitecture.OpenablePaths),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalogHidden, legacyHidden) {
		t.Fatalf("catalog exact search exposed hidden Architecture items: %#v", catalogHidden)
	}
}

func TestCatalogExactSearchRejectsMismatchedPathAuthority(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	catalog := exactSearchTestCatalog(t, []string{"cmd/repomap/main.go"})
	if _, err := BuildSemanticSearchIndexWithCatalog(data, catalog); err == nil ||
		!strings.Contains(err.Error(), "does not match openable paths") {
		t.Fatalf("mismatched catalog error = %v", err)
	}
}

func TestCatalogExactSearchKeepsVersions(t *testing.T) {
	t.Parallel()

	if SemanticSearchIndexVersion != 5 || CurrentFormatVersion != 26 || CurrentRunManifestVersion != 4 {
		t.Fatalf(
			"versions changed: search=%d report=%d manifest=%d",
			SemanticSearchIndexVersion,
			CurrentFormatVersion,
			CurrentRunManifestVersion,
		)
	}
}

func assertExactSearchItem(
	t *testing.T,
	items []SemanticSearchItem,
	kind SemanticSearchKind,
	title string,
	id string,
	target SemanticSearchTargetKind,
) SemanticSearchItem {
	t.Helper()
	item := findSemanticSearchItem(t, items, title)
	if item.Kind != kind || item.ID != id || item.Stability != SemanticSearchStabilityExact ||
		item.Target.Kind != target {
		t.Fatalf("exact item %q = %#v", title, item)
	}
	return item
}

func exactSearchTestCatalog(t *testing.T, paths []string) sourcecatalog.Catalog {
	t.Helper()
	root := t.TempDir()
	inputs := make([]freshness.CapturedInput, 0, len(paths))
	for _, sourcePath := range paths {
		id := sha256.Sum256([]byte("report-exact-search-test\x00" + sourcePath))
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id[:]),
			Path:          sourcePath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", id[:]),
			Stages:        []string{"report_evidence"},
		})
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   paths,
		CapturedInputs: inputs,
	})
	if err != nil {
		t.Fatalf("sourcecatalog.New: %v", err)
	}
	return catalog
}
