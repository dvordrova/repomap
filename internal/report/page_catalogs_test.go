package report

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestPartsCatalogueCountsLocalMapGroupsAcrossLanes(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "backend"}, Groups: []groupindex.Group{
		{ID: "handler", Title: "Handle requests", Lane: groupindex.LaneTriggers},
		{ID: "storage", Title: "Store records", Lane: groupindex.LaneDependencies},
	}, Operations: []groupindex.Operation{{ID: "serve", Name: "Serve", Kind: "request", GroupID: "handler"}},
		Containers: []groupindex.Container{{ID: "area", Title: "Requests", GroupIDs: []string{"handler"}}},
		Connections: []groupindex.Connection{{
			From: groupindex.Endpoint{TargetID: "backend", GroupID: "storage"},
			To:   groupindex.Endpoint{TargetID: "other", GroupID: "remote"},
		}},
	}
	other := groupindex.Index{Target: programindex.Target{ID: "other"}, Groups: []groupindex.Group{
		{ID: "remote", Title: "Remote store", Lane: groupindex.LaneCore},
	}}
	section := &pageSection{ID: "backend-page", programTargetID: "backend", FactsAvailable: true}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index, other}, byProgram: map[string]*pageSection{
		"backend": section, "other": {ID: "other-page"},
	}}
	section.Map = builder.buildMap(section)
	if len(section.Map.Nodes) <= 2 {
		t.Fatal("fixture must also contain an operation, area and foreign component")
	}
	if got := section.PartsCount(); got != 2 {
		t.Fatalf("parts = %d, want both local groups, excluding operations, areas and remote nodes", got)
	}
	if missing := sectionCoverage(section); slices.Contains(missing, "Parts") || slices.Contains(missing, "Core") {
		t.Fatalf("a component with map parts was reported as missing them: %v", missing)
	}
	section.Map = nil
	if missing := sectionCoverage(section); !slices.Contains(missing, "Parts") {
		t.Fatalf("a component without a map lost its missing-parts observation: %v", missing)
	}
}

func TestInputActivityGroupsRetainOriginalKindsAndDisplayBindings(t *testing.T) {
	section := &pageSection{Activities: []pageGroupOperation{
		{Name: "worker", Kind: "continuous", SummaryRef: "worker-ref", Source: "model"},
		{Name: "scheduled", Kind: "scheduled", SummaryRef: "scheduled-ref", Source: "model"},
		{Name: "command", Kind: "command", SummaryRef: "command-ref", Source: "fact"},
		{Name: "click", Kind: "interaction", SummaryRef: "click-ref", Source: "model"},
		{Name: "port", Kind: "setting", SummaryRef: "port-ref", Source: "model"},
	}}
	groups := section.ActivityGroups()
	if len(groups[0].Rows) != 1 || len(groups[1].Rows) != 1 || groups[1].Title != "Settings" || len(groups[2].Rows) != 2 || len(groups[3].Rows) != 1 {
		t.Fatalf("activity kinds not grouped for reading: %+v", groups)
	}
	for _, group := range groups {
		for _, row := range group.Rows {
			if !slices.ContainsFunc(section.Activities, func(original pageGroupOperation) bool { return reflect.DeepEqual(original, row) }) {
				t.Fatalf("display grouping changed the operation or its translation binding: %+v", row)
			}
		}
	}
}

func TestRecipeNeverPromotesAnEntrypointToDocumentedCommand(t *testing.T) {
	if got := recipeBasis([]string{"entry"}, map[string]facts.Fact{"entry": {Kind: facts.KindEntrypoint}}); got != "Inferred from an entrypoint" {
		t.Fatalf("recipe basis = %q", got)
	}
}

// Without a model recipe the page lists the proved entrypoints as ways to
// run the program, but never a library's exports: liblua.a's 156 lua_*
// functions are its API, no way to run it.
func TestRecipeListsNoLibraryExport(t *testing.T) {
	layer := &facts.Result{Facts: []facts.Fact{
		{ID: "main", Kind: facts.KindEntrypoint, Key: "callable", Symbol: "main", Anchor: &facts.Anchor{Path: "lua.c", Line: 10}},
		{ID: "api", Kind: facts.KindEntrypoint, Key: facts.EntrypointExport, Symbol: "lua_gettop", Anchor: &facts.Anchor{Path: "lapi.c", Line: 20}},
	}}
	builder := &pageBuilder{data: &ReportData{Facts: layer}, links: pageLinks{}}
	var view pageView
	builder.recipe(&view)
	if len(view.Recipe) != 1 || view.Recipe[0].Command != "main" {
		t.Fatalf("recipe: %+v", view.Recipe)
	}
	// A library alone: its entries are no launch, and the page says so
	// rather than that no entrypoint was found.
	layer.Facts = layer.Facts[1:]
	view = pageView{}
	builder.recipe(&view)
	if len(view.Recipe) != 0 || !strings.Contains(view.RecipeMissing, "a library's exports are the API it offers") {
		t.Fatalf("library recipe: %+v %q", view.Recipe, view.RecipeMissing)
	}
}

// A word checked only inside another input's handler is that input's
// sub-argument, never an input of its own: redis-server listed "5 commands"
// named hashtable, int, limit, raw and zipmap, DEBUG OBJECT's encodings and
// SORT's LIMIT, and the Inputs collection counted them.
func TestASubArgumentIsNoInputOfItsOwn(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "t1"},
		Operations: []groupindex.Operation{{ID: "o1", Kind: "command", Name: "sort", Source: "model"}, {ID: "o2", Kind: "command", Name: "limit", Source: "model"}},
		Launch:     groupindex.Launch{Nested: map[string]bool{"o2": true}}}
	builder := &pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder.fillSectionOperations(section)
	var names []string
	for _, row := range append(section.Requests, section.Activities...) {
		names = append(names, row.Name)
	}
	if strings.Join(names, " ") != "sort" {
		t.Fatalf("inputs listed: %v", names)
	}
}
