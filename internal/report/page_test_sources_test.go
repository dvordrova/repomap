package report

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestOverviewOmitsKnownTestsWithoutDeletingSourcesOrQuestions(t *testing.T) {
	const target = "application"
	location := func(path string) *programindex.Location {
		return &programindex.Location{Path: path, Line: 3, Column: 7}
	}
	index := groupindex.Index{Target: programindex.Target{ID: target, TestSources: []string{"checks/market.ts"}},
		Subjects: []groupindex.Subject{
			{ID: "serve", Object: &groupindex.ObjectFacts{Name: "serve", Location: location("service.ts")}},
			{ID: "check", Object: &groupindex.ObjectFacts{Name: "check", Location: location("checks/market.ts")}},
			{ID: "unknown", Object: &groupindex.ObjectFacts{Name: "load", Location: location("testing/real-service.test.ts")}},
		},
		Groups: []groupindex.Group{
			{ID: "core", Title: "Service", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"serve", "check"}, EvidenceSubjectIDs: []string{"serve", "check"}},
			{ID: "tests", Title: "Checks", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"check"}},
			{ID: "unknown", Title: "Loading", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"unknown"}},
		},
		Operations: []groupindex.Operation{
			{ID: "serve", SubjectID: "serve", GroupID: "core", Name: "serve", Kind: "command", Location: *location("service.ts")},
			{ID: "check", SubjectID: "check", GroupID: "tests", Name: "check", Kind: "scheduled", Location: *location("checks/market.ts")},
		},
		StructuralEdges: []groupindex.StructuralEdge{
			{FromSubjectID: "check", ToSubjectID: "serve", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls},
			{FromSubjectID: "serve", ToSubjectID: "unknown", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls},
		},
		Containers: []groupindex.Container{{ID: "all", Title: "All", GroupIDs: []string{"core", "tests", "unknown"}}},
	}
	for _, entry := range []struct {
		id       string
		evidence []string
		fromPath string
	}{
		{"test-evidence", []string{"check"}, ""}, {"mixed-evidence", []string{"check", "serve"}, ""}, {"unknown-evidence", nil, ""}, {"test-site", nil, "checks/market.ts"},
	} {
		connection := groupindex.Connection{ID: entry.id, From: groupindex.Endpoint{TargetID: target, GroupID: "core"}, To: groupindex.Endpoint{TargetID: target, GroupID: "unknown"}}
		for _, id := range entry.evidence {
			connection.Evidence = append(connection.Evidence, groupindex.SubjectEndpoint{TargetID: target, SubjectID: id})
		}
		if entry.fromPath != "" {
			connection.FromLocation = location(entry.fromPath)
		}
		index.Connections = append(index.Connections, connection)
	}
	data := &ReportData{Questions: []atlas.QuestionRoute{{Question: "How is the service tested?", Stops: []atlas.QuestionStop{{SubjectID: "check", Path: "checks/market.ts", Line: 3, Column: 7}}}}}
	section := &pageSection{ID: "app", programTargetID: target, ShortLabel: "App"}
	builder := &pageBuilder{data: data, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subject.ID] = subjectRef{subject: subject}
	}
	before, err := json.Marshal(struct {
		Data    *ReportData
		Indexes []groupindex.Index
	}{data, builder.indexes})
	if err != nil {
		t.Fatal(err)
	}
	overview := builder.overviewBuilder()
	projected := overview.indexes[0]
	if len(projected.Subjects) != 2 || len(projected.Groups) != 2 || len(projected.Operations) != 1 || len(projected.StructuralEdges) != 1 || len(projected.Connections) != 2 {
		t.Fatalf("wrong overview projection: %+v", projected)
	}
	if !reflect.DeepEqual(projected.Groups[0].MemberSubjectIDs, []string{"serve"}) || !reflect.DeepEqual(projected.Containers[0].GroupIDs, []string{"core", "unknown"}) || projected.Connections[0].ID != "mixed-evidence" || projected.Connections[1].ID != "unknown-evidence" {
		t.Fatalf("mixed/unknown evidence or frame changed: %+v", projected)
	}
	section.Map = overview.buildMap(section)
	if section.Map == nil || slices.ContainsFunc(section.Map.Nodes, func(node pageMapNode) bool {
		return node.ID == mapNodeID("tests") || node.ID == operationNodeID("app", "check")
	}) {
		t.Fatal("test node reached overview map")
	}
	overview.fillSectionOperations(section)
	if len(section.Activities) != 1 || section.Activities[0].Name != "serve" {
		t.Fatalf("test operation reached catalogue: %+v", section.Activities)
	}
	builder.overviewIndexes = overview.indexes
	if links := builder.questionStepMapLinks(data.Questions[0].Stops[0]); len(links) != 0 {
		t.Fatalf("source check links to a hidden test map node: %+v", links)
	}
	step, err := builder.questionStep(data.Questions[0], atlas.QuestionStep{Path: "checks/market.ts", Line: 3, Column: 7, StopIndexes: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if step.Source.Path != "checks/market.ts" || step.Source.Line != 3 || step.Column != 7 {
		t.Fatalf("test source check lost original anchor: %+v", step)
	}
	after, _ := json.Marshal(struct {
		Data    *ReportData
		Indexes []groupindex.Index
	}{data, builder.indexes})
	if string(before) != string(after) {
		t.Fatal("overview filtering mutated saved graph or original question evidence")
	}
}

func TestOverviewRetainsProductionUsesAndSchemaWithoutTestShelfRows(t *testing.T) {
	production := atlas.DestinationUse{Address: "https://prices.example", Steps: []atlas.DestinationStep{{Path: "service.ts", Line: 4}, {Path: "transport.ts", Line: 9}}}
	testingUse := atlas.DestinationUse{Address: "https://fixture.example", Steps: []atlas.DestinationStep{{Path: "checks.ts", Line: 4}, {Path: "transport.ts", Line: 9}}}
	unknown := atlas.DestinationUse{Frontier: "runtime address", Steps: []atlas.DestinationStep{{Path: "testing/production.test.ts", Line: 5}, {Path: "transport.ts", Line: 9}}}
	index := groupindex.Index{Target: programindex.Target{ID: "service", TestSources: []string{"checks.ts"}},
		Outbound: []groupindex.OutboundCall{
			{ID: "shared", Location: programindex.Location{Path: "transport.ts", Line: 9}, Uses: []atlas.DestinationUse{testingUse, production, unknown}},
			{ID: "test-only-uses", Location: programindex.Location{Path: "transport.ts", Line: 10}, Uses: []atlas.DestinationUse{testingUse}},
			{ID: "test-call", Location: programindex.Location{Path: "checks.ts", Line: 12}},
			{ID: "direct-unknown", Location: programindex.Location{Path: "transport.ts", Line: 13}},
		},
		Data: []groupindex.DataRecord{
			{DataRecord: atlas.DataRecord{ID: "live", Path: "models.py", Line: 8, References: []string{"test-owner"}, Data: &facts.DataObject{Kind: "table", Origin: "orm", Name: "trades"}}},
			{DataRecord: atlas.DataRecord{ID: "test", Path: "checks.ts", Line: 4, Data: &facts.DataObject{Kind: "query", Name: "seed"}}},
			{DataRecord: atlas.DataRecord{ID: "test-owner", Path: "generated.sql", Line: 1, Data: &facts.DataObject{Kind: "table", Name: "fixture", Owner: &facts.Anchor{Path: "checks.ts", Line: 4}}}},
		},
	}
	builder := pageBuilder{indexes: []groupindex.Index{index}}
	before, _ := json.Marshal(builder.indexes)
	overview := builder.overviewBuilder()
	got := overview.indexes[0]
	if len(got.Outbound) != 2 || got.Outbound[0].ID != "shared" || !reflect.DeepEqual(got.Outbound[0].Uses, []atlas.DestinationUse{production, unknown}) || got.Outbound[1].ID != "direct-unknown" || len(got.Data) != 1 || got.Data[0].ID != "live" {
		t.Fatalf("test shelf leaked or production/unknown uses were lost: %+v", got)
	}
	section := &pageSection{ID: "service", programTargetID: "service"}
	overview.fillSectionOutbound(section)
	overview.fillSectionData(section)
	if len(section.Outbound) != 2 || len(section.Outbound[0].Uses) != 2 || section.Data.Tables != 1 || section.Data.Queries != 0 {
		t.Fatalf("shelves ignored overview projection: %+v / %+v", section.Outbound, section.Data)
	}
	refs := section.Data.Rows[0].References
	if len(refs) != 1 || refs[0].Name != "fixture" || refs[0].Href != "" || refs[0].Source == nil || refs[0].Source.Path != "generated.sql" || refs[0].Source.Line != 1 {
		t.Fatalf("hidden table lost source or retained broken catalogue href: %+v", refs)
	}
	after, _ := json.Marshal(builder.indexes)
	if string(before) != string(after) {
		t.Fatal("overview changed original data or destination chains")
	}
}

func TestOverviewPortalCountsUseExactTestSites(t *testing.T) {
	call := facts.Fact{ID: "call", TargetID: "caller", Anchor: &facts.Anchor{Path: "checks.ts", Line: 4}}
	route := facts.Fact{ID: "route", TargetID: "server"}
	data := &ReportData{Facts: &facts.Result{Facts: []facts.Fact{{ID: "portal", Kind: facts.KindPortal, Refs: []string{"call", "route"}}}}}
	builder := pageBuilder{data: data, testPaths: map[string]bool{"checks.ts": true}, factsByID: map[string]facts.Fact{"call": call, "route": route}}
	nodes := map[string]*pageRepoNode{"caller": {ID: "caller", Width: 100, Height: 100}, "server": {ID: "server", X: 200, Width: 100, Height: 100}}
	if edges, _ := builder.repoEdges(nodes); len(edges) != 0 || len(builder.repoOutgoingCounts()) != 0 {
		t.Fatal("test portal returned to overview")
	}
	call.Anchor = &facts.Anchor{Path: "testing/production.test.ts", Line: 4}
	builder.factsByID[call.ID] = call
	if edges, _ := builder.repoEdges(nodes); len(edges) != 1 || builder.repoOutgoingCounts()["caller"] != 1 {
		t.Fatal("filename alone suppressed a production portal")
	}
}
