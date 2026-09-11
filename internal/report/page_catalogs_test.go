package report

import (
	"bytes"
	"html/template"
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

func TestInputCatalogueJoinsExactFactsAndRetainsUngroupedRoutes(t *testing.T) {
	location := programindex.Location{Path: "server.go", Line: 10, Column: 1}
	index := groupindex.Index{Target: programindex.Target{ID: "program"}, Operations: []groupindex.Operation{
		{ID: "bound", FactID: "route-a", Kind: "request", Source: "fact", Name: "GET /a", Location: location},
		// Another membership is the same observation, not a second route.
		{ID: "bound-also", FactID: "route-a", Kind: "request", Source: "fact", Name: "GET /a", Location: location},
		{ID: "model", Kind: "request", Source: "model", Name: "GET /a", Location: location},
		{ID: "worker", Kind: "continuous", Source: "model", Name: "Process messages", Location: location},
	}}
	layer := &facts.Result{Facts: []facts.Fact{
		{ID: "route-a", TargetID: "facts", Kind: facts.KindHTTPRoute, Method: "GET", Path: "/a", Anchor: &facts.Anchor{Path: "server.go", Line: 10}},
		{ID: "orphan", TargetID: "facts", Kind: facts.KindHTTPRoute, Method: "POST", Path: "/outside-map", Anchor: &facts.Anchor{Path: "server.go", Line: 22}},
		{ID: "other-target", TargetID: "other", Kind: facts.KindHTTPRoute, Method: "GET", Path: "/a"},
	}}
	builder := pageBuilder{data: &ReportData{Facts: layer}, indexes: []groupindex.Index{index}, links: pageLinks{sourceIDs: map[string]string{"server.go": "source"}}}
	section := &pageSection{ID: "section", programTargetID: "program", factsTargetID: "facts", FactsAvailable: true}
	builder.fillSectionOperations(section)
	if len(section.RouteGroups) != 2 || section.RouteGroups[0].Paths+section.RouteGroups[1].Paths != 2 {
		t.Fatalf("route lost, duplicated, or borrowed from another target: %+v", section.RouteGroups)
	}
	if len(section.Requests) != 1 || section.Requests[0].Source != "model" || section.Requests[0].Href != "#"+operationNodeID("section", "model") {
		t.Fatalf("name or source position replaced exact fact identity: %+v", section.Requests)
	}
	if len(section.Activities) != 1 || section.Activities[0].Kind != "continuous" {
		t.Fatalf("background activity lost: %+v", section.Activities)
	}
	if got := section.RouteGroups[1].Rows[0].Paths[0].Anchor; got == nil || got.Line != 22 {
		t.Fatalf("ungrouped route lost its original anchor: %+v", got)
	}
	if got := section.RouteGroups[0].Rows[0].Paths[0].OperationHrefs; !slices.Equal(got, []string{"#" + operationNodeID("section", "bound"), "#" + operationNodeID("section", "bound-also")}) {
		t.Fatalf("route did not retain every exact operation membership: %v", got)
	}
	if got := section.RouteGroups[1].Rows[0].Paths[0].OperationHrefs; len(got) != 0 {
		t.Fatalf("ungrouped route acquired an invented operation link: %v", got)
	}
	// Two original registrations and one unmatched interpretation produce three
	// request records, even though the interpretation has the same name/site.
	// The factual HTTP summaries must still count exactly two registrations.
	section.InboundCount, section.InputsCount = 3, 4
	builder.sections = []*pageSection{section}
	builder.byFacts = map[string]*pageSection{"facts": section}
	card := builder.factsCard(facts.Target{ID: "facts"})
	boundaries := builder.boundaryCounts()
	if section.NativeRouteCount() != 2 || card.Routes != 2 || card.Counts != "2 routes" || len(boundaries) != 1 || boundaries[0].Routes != 2 {
		t.Fatalf("request interpretations inflated native HTTP counts: section=%d card=%+v boundaries=%+v", section.NativeRouteCount(), card, boundaries)
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) {
			return uiText(language, key, args...)
		}}).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var rendered bytes.Buffer
		if err := parsed.ExecuteTemplate(&rendered, "input-catalog", section); err != nil {
			t.Fatal(err)
		}
		for _, counted := range []struct {
			label string
			count string
		}{{"Incoming request records", "3"}, {"HTTP registrations in source", "2"}, {"Handlers without a matched route", "1"}} {
			label, err := uiText(language, counted.label)
			if err != nil || !strings.Contains(rendered.String(), label+" · "+counted.count) {
				t.Fatalf("%s catalogue lost the distinction %s/%s: %s", language, counted.label, counted.count, rendered.String())
			}
		}
		for _, kept := range []string{"/a", "/outside-map", "#" + operationNodeID("section", "bound"), "#" + operationNodeID("section", "bound-also"), "#" + operationNodeID("section", "model"), `data-source-kind="model"`} {
			if !strings.Contains(rendered.String(), kept) {
				t.Fatalf("display count correction discarded original record/link %q", kept)
			}
		}
	}
	// Model-only reading must not suppress requests or borrow all repository facts.
	section = &pageSection{ID: "section", programTargetID: "program"}
	builder.fillSectionOperations(section)
	if section.NativeRouteCount() != 0 || len(section.RouteGroups) != 0 || len(section.Requests) != 3 {
		t.Fatalf("model-only catalogue fabricated facts: %+v", section)
	}
}

func TestInputActivityGroupsRetainOriginalKindsAndDisplayBindings(t *testing.T) {
	section := &pageSection{Activities: []pageGroupOperation{
		{Name: "worker", Kind: "continuous", SummaryRef: "worker-ref", Source: "model"},
		{Name: "scheduled", Kind: "scheduled", SummaryRef: "scheduled-ref", Source: "model"},
		{Name: "command", Kind: "command", SummaryRef: "command-ref", Source: "fact"},
		{Name: "click", Kind: "interaction", SummaryRef: "click-ref", Source: "model"},
	}}
	groups := section.ActivityGroups()
	if len(groups[0].Rows) != 1 || len(groups[1].Rows) != 2 || len(groups[2].Rows) != 1 {
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
