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

func TestExactMemberSearchLocationRequiresOneUniqueExactLocator(t *testing.T) {
	t.Parallel()

	first := evidence.Location{Path: "service.go", Line: 10, Column: 2, EndLine: 10, EndColumn: 8}
	second := evidence.Location{Path: "service.go", Line: 20, Column: 2, EndLine: 20, EndColumn: 8}
	member := componentmap.Candidate{
		ID: componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "service.Run"},
		Facts: []componentmap.LocalFact{
			{Kind: componentmap.FactDeclaration, Location: &first},
			{Kind: componentmap.FactContainment, Location: &first},
		},
	}
	if got := exactMemberSearchLocation(member); got == nil || *got != first {
		t.Fatalf("equivalent exact locations = %#v, want %#v", got, first)
	}

	member.Facts = append(member.Facts, componentmap.LocalFact{Kind: componentmap.FactContainment, Location: &second})
	if got := exactMemberSearchLocation(member); got != nil {
		t.Fatalf("multiple legitimate exact locations chose one target: %#v", got)
	}
	member.Facts[0], member.Facts[2] = member.Facts[2], member.Facts[0]
	if got := exactMemberSearchLocation(member); got != nil {
		t.Fatalf("reordered multiple exact locations chose one target: %#v", got)
	}
}

func TestCatalogExactSearchKeepsMultiplyLocatedMemberNeutral(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	data.OpenablePaths = append(data.OpenablePaths, "internal/report/other.go")
	sort.Strings(data.OpenablePaths)
	data.ArchitectureCanvas.Components[0].Members[0].Facts = []componentmap.LocalFact{
		{
			Kind: componentmap.FactDeclaration, Value: "CollectFacts", Certainty: evidence.CertaintyStatic,
			Location: &evidence.Location{Path: "internal/report/report.go", Line: 88},
		},
		{
			Kind: componentmap.FactContainment, Value: "CollectFacts", Certainty: evidence.CertaintyStatic,
			Location: &evidence.Location{Path: "internal/report/other.go", Line: 12},
		},
	}
	index, err := BuildSemanticSearchIndexWithCatalog(
		data,
		exactSearchTestCatalog(t, data.OpenablePaths),
	)
	if err != nil {
		t.Fatal(err)
	}
	member := findSemanticSearchItem(t, index.Items, "CollectFacts")
	if member.Kind != SemanticSearchKindMember || member.Target.Kind != SemanticSearchTargetMap ||
		member.Target.ComponentID != "" || member.Target.Location != nil {
		t.Fatalf("multiply located member target = %#v, want neutral map", member.Target)
	}
}

func TestCatalogExactSearchUsesStableSourceTargetedMemberIdentity(t *testing.T) {
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

	catalog := exactSearchTestCatalog(t, data.OpenablePaths)
	migrated, err := BuildSemanticSearchIndexWithCatalog(data, catalog)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndexWithCatalog: %v", err)
	}
	again, err := BuildSemanticSearchIndexWithCatalog(data, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(migrated, again) {
		t.Fatalf("exact search changed between identical builds:\nfirst:  %#v\nsecond: %#v", migrated, again)
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
		"member:e2550d242336b445e0d2bbdc7d64a939c831d5d78cd94534d1393be36085a811",
		SemanticSearchTargetLocation)
	if member.Target.ComponentID != "" || member.Target.Location == nil ||
		member.Target.Location.Path != "internal/report/report.go" ||
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
			Version: ArchitectureCanvasVersion,
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
	catalogEditorial, err := BuildSemanticSearchIndexWithCatalog(
		editorialData,
		exactSearchTestCatalog(t, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	findSemanticSearchItem(t, catalogEditorial.Items, "How facts become a report")
	emptyScopeMember := findSemanticSearchItem(t, catalogEditorial.Items, "CollectFacts")
	if emptyScopeMember.Target.Kind != SemanticSearchTargetMap || emptyScopeMember.Target.ComponentID != "" {
		t.Fatalf("empty exact scope inferred member ownership: %#v", emptyScopeMember)
	}

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
	legacy, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	data.SemanticSearch = &legacy
	catalog := exactSearchTestCatalog(t, []string{"cmd/repomap/main.go"})
	if _, err := BuildSemanticSearchIndexWithCatalog(data, catalog); err == nil ||
		!strings.Contains(err.Error(), "does not match openable paths") {
		t.Fatalf("mismatched catalog error = %v", err)
	}
	before, err := json.Marshal(data.SemanticSearch)
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachExactWorkspaceSearch(data, catalog); err == nil ||
		!strings.Contains(err.Error(), "does not match openable paths") {
		t.Fatalf("AttachExactWorkspaceSearch mismatch error = %v", err)
	}
	after, err := json.Marshal(data.SemanticSearch)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed exact adapter replaced legacy projection:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestCatalogExactSearchBoundsRawReportMembersBeforeProjection(t *testing.T) {
	t.Parallel()

	members := make([]componentmap.Candidate, maxReportExactSymbols+1)
	for index := range members {
		members[index].ID = componentmap.MemberID{
			Kind:  componentmap.MemberPackage,
			Value: fmt.Sprintf("package-%04d", index),
		}
	}
	data := &ReportData{
		RepoName:      "bounded",
		OpenablePaths: []string{"service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			Components: []ArchitectureComponent{{
				ID:      "service",
				Name:    "Service",
				Members: members,
			}},
		},
	}
	_, err := BuildSemanticSearchIndexWithCatalog(
		data,
		exactSearchTestCatalog(t, data.OpenablePaths),
	)
	if err == nil || !strings.Contains(err.Error(), "exact member input exceeds") {
		t.Fatalf("raw member bound error = %v", err)
	}
}

func TestCatalogExactSearchRejectsHistoricalCanvasBeforeExactInputBounds(t *testing.T) {
	t.Parallel()

	members := make([]componentmap.Candidate, maxReportExactSymbols+1)
	for index := range members {
		members[index].ID = componentmap.MemberID{
			Kind:  componentmap.MemberPackage,
			Value: fmt.Sprintf("package-%04d", index),
		}
	}
	data := &ReportData{
		RepoName:      "historical-before-bounds",
		OpenablePaths: []string{"service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion - 1,
			Components: []ArchitectureComponent{{
				ID: "service", Name: "Service", Members: members,
			}},
		},
	}
	_, err := BuildSemanticSearchIndexWithCatalog(
		data,
		exactSearchTestCatalog(t, data.OpenablePaths),
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported architecture canvas version") {
		t.Fatalf("historical canvas precedence error = %v", err)
	}
	if strings.Contains(err.Error(), "exact member input exceeds") {
		t.Fatalf("historical canvas reached current-shape member bounds: %v", err)
	}
}

func TestCatalogExactSearchCountsSharedMembersSeparatelyFromMembershipEdges(t *testing.T) {
	t.Parallel()

	shared := componentmap.Candidate{
		ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "shared.Run"},
		Name: "Run",
		Facts: []componentmap.LocalFact{{
			Kind: componentmap.FactDeclaration, Value: "Run",
			Location: &evidence.Location{Path: "service.go", Line: 3, Column: 1},
		}},
	}
	build := func(componentIDs ...componentmap.ComponentID) SemanticSearchIndex {
		components := make([]ArchitectureComponent, 0, len(componentIDs))
		for _, componentID := range componentIDs {
			components = append(components, ArchitectureComponent{
				ID: componentID, Name: string(componentID), Members: []componentmap.Candidate{shared},
			})
		}
		data := &ReportData{
			RepoName:           "shared",
			OpenablePaths:      []string{"service.go"},
			ArchitectureCanvas: &ArchitectureCanvas{Version: ArchitectureCanvasVersion, Components: components},
		}
		index, err := BuildSemanticSearchIndexWithCatalog(
			data,
			exactSearchTestCatalog(t, data.OpenablePaths),
		)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}

	first := build("component-a", "component-b")
	second := build("component-b", "component-a")
	firstMember := findSemanticSearchItem(t, first.Items, "Run")
	secondMember := findSemanticSearchItem(t, second.Items, "Run")
	if firstMember.ID != secondMember.ID || firstMember.Target.Kind != SemanticSearchTargetLocation ||
		firstMember.Target.ComponentID != "" || firstMember.Target.Location == nil ||
		firstMember.Target.Location.Path != "service.go" || !reflect.DeepEqual(firstMember, secondMember) {
		t.Fatalf("shared exact member search changed with membership order: first=%#v second=%#v", firstMember, secondMember)
	}
	count := 0
	for _, item := range first.Items {
		if item.Kind == SemanticSearchKindMember && item.Title == "Run" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("shared exact member search items = %d, want one", count)
	}
}

func TestCatalogExactSearchDoesNotSourceTargetOversizedMemberFacts(t *testing.T) {
	t.Parallel()

	facts := make([]componentmap.LocalFact, maxReportExactFactsPerMember+1)
	for index := range facts {
		facts[index] = componentmap.LocalFact{
			Kind:     componentmap.FactDeclaration,
			Value:    fmt.Sprintf("Run%d", index),
			Location: &evidence.Location{Path: "service.go", Line: index + 1},
		}
	}
	data := &ReportData{
		RepoName:      "bounded-member-facts",
		OpenablePaths: []string{"service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			Components: []ArchitectureComponent{{
				ID: "component", Name: "Component", Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "service.Run"},
					Name: "Run", Facts: facts,
				}},
			}},
		},
	}
	index, err := BuildSemanticSearchIndexWithCatalog(data, exactSearchTestCatalog(t, data.OpenablePaths))
	if err != nil {
		t.Fatal(err)
	}
	member := findSemanticSearchItem(t, index.Items, "Run")
	if member.Target.Kind != SemanticSearchTargetMap || member.Target.Location != nil || member.Target.ComponentID != "" {
		t.Fatalf("oversized member facts gained an exact/owner target: %#v", member)
	}
}

func TestCatalogExactSearchBoundsMembershipEdgesIndependently(t *testing.T) {
	t.Parallel()

	members := make([]componentmap.Candidate, maxReportExactSymbols)
	for index := range members {
		members[index].ID = componentmap.MemberID{
			Kind:  componentmap.MemberPackage,
			Value: fmt.Sprintf("package-%04d", index),
		}
	}
	components := make([]ArchitectureComponent, maxReportExactMemberships/maxReportExactSymbols+1)
	for index := range components {
		components[index] = ArchitectureComponent{
			ID:      componentmap.ComponentID(fmt.Sprintf("component-%02d", index)),
			Name:    fmt.Sprintf("Component %d", index),
			Members: members,
		}
	}
	data := &ReportData{
		RepoName:           "bounded-memberships",
		OpenablePaths:      []string{"service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{Version: ArchitectureCanvasVersion, Components: components},
	}
	_, err := BuildSemanticSearchIndexWithCatalog(
		data,
		exactSearchTestCatalog(t, data.OpenablePaths),
	)
	if err == nil || !strings.Contains(err.Error(), "exact membership input exceeds") {
		t.Fatalf("membership bound error = %v", err)
	}
}

func TestCatalogExactSearchRejectsDuplicateMembershipWithinComponent(t *testing.T) {
	t.Parallel()

	member := componentmap.Candidate{
		ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "service.Run"},
		Name: "Run",
		Facts: []componentmap.LocalFact{{
			Kind: componentmap.FactDeclaration, Value: "Run",
			Location: &evidence.Location{Path: "service.go", Line: 3},
		}},
	}
	data := &ReportData{
		RepoName:      "duplicate-membership",
		OpenablePaths: []string{"service.go"},
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{{
			ID: "component", Name: "Component", Members: []componentmap.Candidate{member, member},
		}}, Version: ArchitectureCanvasVersion},
	}
	_, err := BuildSemanticSearchIndexWithCatalog(
		data,
		exactSearchTestCatalog(t, data.OpenablePaths),
	)
	if err == nil || !strings.Contains(err.Error(), "repeats exact member") {
		t.Fatalf("duplicate membership error = %v", err)
	}
}

func TestCatalogExactSearchKeepsVersions(t *testing.T) {
	t.Parallel()

	if SemanticSearchIndexVersion != 6 || CurrentFormatVersion != 48 || CurrentRunManifestVersion != 18 {
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
