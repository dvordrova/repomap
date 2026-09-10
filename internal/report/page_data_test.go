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
	parsed, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) { return uiText(Russian, key, args...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := parsed.ExecuteTemplate(&rendered, "data.html", section); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	for _, want := range []string{"trades", "GET /trades", "Trade.get_trades", "models.py:30:5", "ReadTrades", "#service-data-table-0", "&lt;script&gt;", "все 7 →"} {
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
