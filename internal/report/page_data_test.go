package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestDataCataloguePreservesScopeSourceAndActualModelCallAssociations(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"}}
	for i := 0; i < 6; i++ {
		index.Data = append(index.Data, groupindex.DataRecord{DataRecord: atlas.DataRecord{ID: fmt.Sprintf("table-%d", i), Path: "models.py", Line: 10 + i,
			Data: &facts.DataObject{Kind: "table", Origin: "orm", Scope: fmt.Sprintf("orm:Base%d", i), Name: "trades", Owner: &facts.Anchor{Path: "models.py", Line: 9 + i}, Columns: []facts.DataColumn{{Name: "id", PrimaryKey: true, Anchor: facts.Anchor{Path: "models.py", Line: 11 + i}}}}}, OwnerSubjectID: fmt.Sprintf("model-%d", i)})
	}
	index.Data = append(index.Data, groupindex.DataRecord{DataRecord: atlas.DataRecord{ID: "query", Path: "queries.py", Line: 3, References: []string{"table-0"},
		Data: &facts.DataObject{Kind: "query", Origin: "query", Scope: "orm:Base0", Name: "ReadTrades", SQL: "SELECT * FROM trades WHERE note='<script>'", Statement: "SELECT", Partial: true}}})
	index.Operations = []groupindex.Operation{{ID: "get-trades", SubjectID: "handler", Name: "GET /trades", Kind: "request"}, {ID: "retry", SubjectID: "unknown", Name: "Retry", Kind: "scheduled"}}
	index.Subjects = []groupindex.Subject{{ID: "method", Object: &groupindex.ObjectFacts{Name: "Trade.get_trades", OwnerID: "model-0", Location: &programindex.Location{Path: "models.py", Line: 30, Column: 5}}}, {ID: "other-method", Object: &groupindex.ObjectFacts{Name: "Archive.get_trades", OwnerID: "model-1", Location: &programindex.Location{Path: "models.py", Line: 50, Column: 5}}}}
	index.StructuralEdges = []groupindex.StructuralEdge{
		{FromSubjectID: "handler", ToSubjectID: "helper", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "helper", ToSubjectID: "method", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives},
		{FromSubjectID: "method", ToSubjectID: "handler", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "handler", ToSubjectID: "other-method", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationReads, Resolution: programindex.ResolutionExact},
	}
	section := &pageSection{ID: "service", programTargetID: "service", Map: &pageMap{}}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, links: pageLinks{sourceIDs: map[string]string{"models.py": "models", "queries.py": "queries"}}}
	builder.fillSectionData(section)
	if section.Data.Tables != 6 || section.Data.Queries != 1 {
		t.Fatal("same table names in independent scopes collapsed")
	}
	for _, row := range section.Data.Rows {
		if row.Scope == "orm:Base0" && row.SQL == "" {
			if len(row.Operations) != 1 || !row.Operations[0].Possible || row.Operations[0].Via != "Trade.get_trades" || row.Operations[0].Anchor.Open != "models.py:30:5" {
				t.Fatalf("native call association lost: %+v", row.Operations)
			}
		} else if len(row.Operations) != 0 {
			t.Fatal("name or non-call relation invented a database association")
		}
	}
	page := &PreparedPage{view: &pageView{Sections: []*pageSection{section}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	if len(page.catalog.Entries) != 0 {
		t.Fatal("native SQL/schema text entered translation")
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := parsed.ExecuteTemplate(&rendered, "data.html", section); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	for _, want := range []string{"trades", "GET /trades", "Trade.get_trades", "models.py:30:5", "ReadTrades", "#service-data-table-0", "&lt;script&gt;", "все 6 →"} {
		if !strings.Contains(html, want) {
			t.Fatalf("data overview lost %q", want)
		}
	}
	if strings.Contains(html, "<script>") || strings.Count(html, `id="service-data-table-`) != 6 {
		t.Fatal("SQL not escaped or rows lost/duplicated")
	}
	if strings.Contains(html, "orm:Base") {
		t.Fatal("internal source scope leaked into reader details")
	}
}

func TestDataCatalogueLinksQueryOperationsAndScopedTablesBothWays(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"}}
	location := &programindex.Location{Path: "queries.py", Line: 10, Column: 1}
	index.Subjects = []groupindex.Subject{
		{ID: "query-owner", Object: &groupindex.ObjectFacts{Kind: programindex.ObjectFunction, Name: "readTrades", Location: location}},
		{ID: "unrelated", Object: &groupindex.ObjectFacts{Kind: programindex.ObjectMethod, Name: "other", OwnerID: "query-class", Location: location}},
		{ID: "query-class", Object: &groupindex.ObjectFacts{Kind: programindex.ObjectType, Name: "LegacyQueryClass", Location: location}},
	}
	index.Data = []groupindex.DataRecord{
		{DataRecord: atlas.DataRecord{ID: "table", Path: "schema.sql", Line: 2, Data: &facts.DataObject{Kind: "table", Origin: "ddl", Scope: "schema:main", Name: "trades"}}},
		{DataRecord: atlas.DataRecord{ID: "other-table", Path: "archive.sql", Line: 2, Data: &facts.DataObject{Kind: "table", Origin: "ddl", Scope: "schema:archive", Name: "trades"}}},
		{DataRecord: atlas.DataRecord{ID: "query", Path: "queries.py", Line: 11, References: []string{"table"}, Data: &facts.DataObject{Kind: "query", Origin: "query", Scope: "schema:main", Name: "ReadTrades", SQL: "SELECT id FROM trades", Statement: "SELECT"}}, OwnerSubjectID: "query-owner"},
		{DataRecord: atlas.DataRecord{ID: "legacy-query", Path: "queries.py", Line: 12, Data: &facts.DataObject{Kind: "query", Origin: "query", Scope: "unknown", Name: "DifferentQuery", SQL: "SELECT id FROM trades"}}, OwnerSubjectID: "query-class"},
	}
	index.Operations = []groupindex.Operation{{ID: "get", Name: "GET /trades", SubjectID: "handler", Kind: "request"}}
	index.StructuralEdges = []groupindex.StructuralEdge{
		{FromSubjectID: "handler", ToSubjectID: "query-owner", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives},
		{FromSubjectID: "handler", ToSubjectID: "unrelated", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
	}
	href := "#" + operationNodeID("service", "get")
	section := &pageSection{ID: "service", programTargetID: "service", Requests: []pageGroupOperation{{Name: "GET /trades", Kind: "request", Href: href}}, RouteGroups: []pageRouteGroup{{Rows: []pageRouteRow{{Paths: []pageRoutePath{{Path: "/trades", OperationHrefs: []string{href}}}}}}}}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, links: pageLinks{sourceIDs: map[string]string{"queries.py": "queries", "schema.sql": "schema"}}}
	builder.fillSectionData(section)
	rows := map[string]pageDataRow{}
	for _, row := range section.Data.Rows {
		rows[row.ID] = row
	}
	query, table := rows["service-data-query"], rows["service-data-table"]
	if len(query.Operations) != 1 || !query.Operations[0].Possible || len(query.References) != 1 || query.References[0].Href != "#service-data-table" {
		t.Fatalf("query links lost: %+v", query)
	}
	if len(table.Queries) != 1 || table.Queries[0].Href != "#service-data-query" || len(table.Operations) != 1 || table.Operations[0].ViaHref != "#service-data-query" || !table.Operations[0].Possible {
		t.Fatalf("table inverse/source links lost: %+v", table)
	}
	if len(rows["service-data-other-table"].Operations) != 0 || len(rows["service-data-other-table"].Queries) != 0 || len(rows["service-data-legacy-query"].Operations) != 0 {
		t.Fatal("name or class membership invented query ownership")
	}
	if len(section.Requests[0].Data) != 2 || len(section.RouteGroups[0].Rows[0].Paths[0].Data) != 2 {
		t.Fatal("first-screen operations lost query/table links")
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := parsed.ExecuteTemplate(&html, "operation-row", section.Requests[0]); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Данные", `href="#service-data-query"`, `href="#service-data-table"`} {
		if !strings.Contains(html.String(), expected) {
			t.Fatalf("input row lacks %q", expected)
		}
	}
}

// Record ids carry kind prefixes with a colon ("query:…"). Inside a fragment
// href html/template reads the text before the colon as a URL scheme and
// replaces the whole link with #ZgotmplZ; the meetup report shipped 82 such
// dead "SQL texts" links. Page ids therefore never contain a colon.
func TestDataRowIDsAreSafeFragmentTargets(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"}, Data: []groupindex.DataRecord{
		{DataRecord: atlas.DataRecord{ID: "entity:f-1", Path: "schema.sql", Line: 2, Data: &facts.DataObject{Kind: "table", Origin: "ddl", Scope: "schema:main", Name: "events"}}},
		{DataRecord: atlas.DataRecord{ID: "query:q-1", Path: "queries.sql", Line: 5, References: []string{"entity:f-1"},
			Data: &facts.DataObject{Kind: "query", Origin: "query", Scope: "schema:main", Name: "CreateEvent", SQL: "INSERT INTO events VALUES (1)", Statement: "INSERT"}}},
	}}
	section := &pageSection{ID: "service", programTargetID: "service"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionData(section)
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "data.html", section); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Contains(html, "ZgotmplZ") {
		t.Fatal("a data link was rejected by html/template")
	}
	for _, text := range []string{`id="service-data-entity-f-1"`, `id="service-data-query-q-1"`, `href="#service-data-query-q-1"`, `href="#service-data-entity-f-1"`} {
		if !strings.Contains(html, text) {
			t.Fatalf("data catalog lost %q", text)
		}
	}
}
