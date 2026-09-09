package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

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
	// Model-only reading must not suppress requests or borrow all repository facts.
	section = &pageSection{ID: "section", programTargetID: "program"}
	builder.fillSectionOperations(section)
	if len(section.RouteGroups) != 0 || len(section.Requests) != 3 {
		t.Fatalf("model-only catalogue fabricated facts: %+v", section)
	}
}

func TestRecipeNeverPromotesAnEntrypointToDocumentedCommand(t *testing.T) {
	if got := recipeBasis([]string{"entry"}, map[string]facts.Fact{"entry": {Kind: facts.KindEntrypoint}}); got != "Inferred from an entrypoint" {
		t.Fatalf("recipe basis = %q", got)
	}
}
