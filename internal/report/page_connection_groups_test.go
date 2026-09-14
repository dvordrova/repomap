package report

import (
	"bytes"
	"html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestInternalCallRemainsReadableAfterItsDeclarationsShareAPart(t *testing.T) {
	part := groupindex.Group{ID: "bootstrap", Title: "Application bootstrap", MemberSubjectIDs: []string{"entry", "metrics"}}
	index := groupindex.Index{Groups: []groupindex.Group{part}}
	b := pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"index.tsx": "entry-file", "metrics.ts": "metrics-file"}}}
	for _, item := range []struct{ id, name, path string }{{"entry", "src/index", "index.tsx"}, {"metrics", "reportWebVitals", "metrics.ts"}, {"outside", "other", "metrics.ts"}} {
		b.subjects[item.id] = subjectRef{subject: groupindex.Subject{ID: item.id, Object: &groupindex.ObjectFacts{Name: item.name, Location: &programindex.Location{Path: item.path, Line: 3, Column: 1}}}}
	}
	call := groupindex.StructuralEdge{Role: groupindex.EdgeRelationTarget, RelationID: "call", RelationKind: programindex.RelationCalls, FromSubjectID: "entry", ToSubjectID: "metrics", Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "index.tsx", Line: 19, Column: 1}}
	second := call
	second.RelationID, second.Resolution = "second-call", programindex.ResolutionAlternatives
	second.Location = &programindex.Location{Path: "index.tsx", Line: 19, Column: 20}
	contains := call
	contains.RelationID, contains.RelationKind = "contains", programindex.RelationContains
	outside := call
	outside.RelationID, outside.ToSubjectID = "outside-call", "outside"
	index.StructuralEdges = []groupindex.StructuralEdge{call, second, contains, outside}
	rows := b.internalGroupConnections(index, part)
	if len(rows) != 2 || rows[0].Label != "src/index calls reportWebVitals" || !rows[0].Native || rows[0].Possible || !rows[1].Possible {
		t.Fatalf("internal call disappeared, acquired membership edges, or lost resolution: %+v", rows)
	}
	if rows[0].FromSource.Path != "index.tsx" || rows[0].FromSource.Line != 19 || rows[0].EvidenceID == rows[1].EvidenceID || rows[0].ToSource.Path != "metrics.ts" || rows[0].ToSource.Line != 3 {
		t.Fatalf("original call sites and destination declaration changed: %+v", rows)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "group", pageGroup{ID: part.ID, Title: part.Title, InternalConnections: rows}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Connections within this part · 2", "src/index calls reportWebVitals", "index.tsx:19", "metrics.ts:3"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("static reading lost %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Connected to") || strings.Contains(out.String(), `class="model"><span`) {
		t.Fatal("native internal calls became a self-navigation or model statement")
	}
}

func TestExternalCallerReadingUsesExactMethodNotSharedClientGroup(t *testing.T) {
	index := groupindex.Index{Groups: []groupindex.Group{
		{ID: "client", Title: "HTTP client", MemberSubjectIDs: []string{"get", "post"}},
		{ID: "page", Title: "Level page", MemberSubjectIDs: []string{"load"}},
		{ID: "play", Title: "Playground", MemberSubjectIDs: []string{"run"}},
	}, StructuralEdges: []groupindex.StructuralEdge{
		{RelationID: "get-call", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, FromSubjectID: "load", ToSubjectID: "get", Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "page.ts", Line: 20}},
		{RelationID: "post-call", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, FromSubjectID: "run", ToSubjectID: "post", Resolution: programindex.ResolutionAlternatives, Location: &programindex.Location{Path: "play.ts", Line: 72}},
		{RelationID: "read-only", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationReads, FromSubjectID: "run", ToSubjectID: "get"},
	}}
	b := pageBuilder{subjects: map[string]subjectRef{}}
	for _, id := range []string{"get", "post", "load", "run"} {
		b.subjects[id] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Location: &programindex.Location{Path: id + ".ts", Line: 3}}}}
	}
	rows := b.outboundCallers(&index, "front", "get")
	if len(rows) != 1 || rows[0].Title != "Level page" || rows[0].Label != "load → get" || rows[0].Possible || rows[0].FromSource.Line != 20 {
		t.Fatalf("GET acquired another client method's caller: %+v", rows)
	}
	rows = b.outboundCallers(&index, "front", "post")
	if len(rows) != 1 || rows[0].Href != "#"+groupAnchorID("front", "play") || !rows[0].Possible || rows[0].FromSource.Line != 72 {
		t.Fatalf("POST lost its exact caller/source/status: %+v", rows)
	}
	if len(b.outboundCallers(&index, "front", "")) != 0 {
		t.Fatal("unknown caller was inferred from client membership")
	}
}

func TestConnectionReadingGroupsPreserveIdentityDirectionAndEvidence(t *testing.T) {
	rows := []pageConnection{
		{Href: "#rules", Title: "Rules", Arrow: "→", Label: "call 1", Summary: "Checks moves", SummaryRef: "summary1", FromSource: &pageAnchor{Text: "eval.clj:32"}},
		{Href: "#rules", Title: "Rules", Arrow: "→", Label: "call 2", Summary: "Checks moves", SummaryRef: "summary2", FromSource: &pageAnchor{Text: "eval.clj:33"}, Possible: true},
		{Href: "#rules", Title: "Rules", Arrow: "←", Label: "callback"},
		{Href: "#other-rules", Title: "Rules", Arrow: "→", Label: "call 1"},
		{Title: "Unresolved", Arrow: "→", Label: "unknown 1"},
		{Title: "Unresolved", Arrow: "→", Label: "unknown 2"},
	}
	groups := (pageGroup{Connections: rows}).ConnectionGroups()
	if len(groups) != 5 || len(groups[0].Summaries) != 1 || len(groups[0].Rows) != 2 {
		t.Fatalf("unexpected reading groups: %#v", groups)
	}
	var restored []pageConnection
	for _, group := range groups {
		restored = append(restored, group.Rows...)
	}
	if !reflect.DeepEqual(restored, []pageConnection{rows[0], rows[1], rows[3], rows[4], rows[5], rows[2]}) {
		t.Fatal("reading lost original rows, provenance or endpoints")
	}
	if got := collapseConnections([]pageConnection{rows[0], {Href: "#other", Title: rows[0].Title, Arrow: rows[0].Arrow, Label: rows[0].Label, Summary: rows[0].Summary, FromSource: rows[0].FromSource}}); len(got) != 2 {
		t.Fatal("equal labels merged different participants before reading")
	}
}

func TestNativeConnectionReadingWithoutModelSentenceKeepsEveryCallAndReverseLink(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "target"}, Groups: []groupindex.Group{
		{ID: "field", Title: "Field simulation", MemberSubjectIDs: []string{"check"}},
		{ID: "robot", Title: "Robot movement", MemberSubjectIDs: []string{"move"}},
	}}
	b := pageBuilder{data: &ReportData{}, byProgram: map[string]*pageSection{"target": {ID: "backend"}}, subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"field.py": "f", "robot.py": "r"}}}
	for id, path := range map[string]string{"check": "field.py", "move": "robot.py"} {
		b.subjects[id] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectMethod, Location: &programindex.Location{Path: path, Line: 3, Column: 1}}}}
	}
	for i, id := range []string{"one", "two"} {
		index.StructuralEdges = append(index.StructuralEdges, groupindex.StructuralEdge{RelationID: id, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, FromSubjectID: "check", ToSubjectID: "move", Resolution: programindex.ResolutionAlternatives, Location: &programindex.Location{Path: "field.py", Line: 17, Column: 4 + i*10}})
	}
	forward := b.groupConnections(index, index.Groups[0])
	reverse := b.groupConnections(index, index.Groups[1])
	if len(forward) != 2 || len(reverse) != 2 {
		t.Fatalf("native calls absent or same-line calls collapsed: %+v / %+v", forward, reverse)
	}
	for _, row := range forward {
		if !row.Native || !row.Possible || row.Title != "Robot movement" || row.Label != "check calls move" || row.FromSource == nil || row.ToSource == nil || row.Href != "#"+groupAnchorID("backend", "robot") {
			t.Fatalf("native connection lost code/source/navigation: %+v", row)
		}
	}
	for _, row := range reverse {
		if row.Arrow != "←" || row.Title != "Field simulation" {
			t.Fatalf("reverse connection missing: %+v", row)
		}
	}
	index.Connections = []groupindex.Connection{{SourceKind: "native_calls", SourceID: "one", ToSubjectID: "move"}}
	if len(b.nativeGroupConnections(index, index.Groups[0])) != 1 {
		t.Fatal("existing native connection duplicated")
	}
}
