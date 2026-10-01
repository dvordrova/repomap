package report

import (
	"bytes"
	"encoding/json"
	"html/template"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A program's files (owner, 2026-09-29) are its Data shelf's and its
// component's: the shelf lists each by its path as written, or the paths
// its field's writes store and the field itself, and one line for the paths
// not established; the component carries their reading, each file with
// what else sets its field (the setting whose branch writes it, else the
// function writing what is not established) and the functions whose calls
// reach it, by part. No role is given and no line number written: a name
// reads its function, a path links to where it is written.
func TestAProgramsFilesAreItsDataWithTheFunctionsReachingThemByPart(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "kvd.c", Line: line, Column: 1}
	}
	builder := &pageBuilder{subjects: map[string]subjectRef{}, groupTitles: map[groupindex.Endpoint]string{},
		links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc", sourceIDs: map[string]string{"kvd.c": "f1"}},
		data:  &ReportData{ProgramPortfolio: &ProgramPortfolio{}}}
	function := func(id, name string, line int) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: at(line)}}}
	}
	function("n1", "saveSnapshot", 135)
	function("n2", "rdbLoad", 200)
	function("n3", "loadConfig", 320)
	function("n4", "main", 339)
	function("n5", "rewriteConfig", 400)
	function("n6", "dumpKeys", 500)
	builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: "g1"}] = "Persistence"
	builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: "g2"}] = "Configuration"
	call := func(symbol string, line int) facts.DataCall {
		return facts.DataCall{Symbol: symbol, Anchor: facts.Anchor{Path: "kvd.c", Line: line, Column: 5}}
	}
	record := func(id, name, field string, calls []facts.DataCall, callers []string, values []facts.DataValue, writers []string) groupindex.DataRecord {
		return groupindex.DataRecord{DataRecord: atlas.DataRecord{ID: id, Path: "kvd.c", Line: calls[0].Anchor.Line, Data: &facts.DataObject{Kind: "file", Origin: "call", Scope: "t1", Name: name,
			File: &facts.DataFile{Field: field, Values: values, Calls: calls}}}, CallSubjectIDs: callers, ValueSubjectIDs: writers}
	}
	value := func(written string, line int) facts.DataValue {
		return facts.DataValue{Value: written, Anchor: facts.Anchor{Path: "kvd.c", Line: line, Column: 12}}
	}
	setting := at(328)
	index := groupindex.Index{Target: programindex.Target{ID: "t1"},
		Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n1", "n2"}}, {ID: "g2", MemberSubjectIDs: []string{"n3", "n4", "n5", "n6"}}},
		// dbfilename is the setting the configuration reader compares on
		// line 328, whose branch writes server.dbfile.
		Operations: []groupindex.Operation{{ID: "o1", Kind: "setting", Name: "dbfilename", Source: "model", DeclaredBy: "n3", Location: *setting}},
		Data: []groupindex.DataRecord{
			record("w1", "{server.dbfile}", "server.dbfile", []facts.DataCall{call("stdio.h.fopen", 136), call("stdio.h.fopen", 210)}, []string{"n1", "n2"},
				[]facts.DataValue{value("", 328), value("", 410), value("", 420), value("dump.kv", 351)}, []string{"n3", "n5", "n5", "n4"}),
			record("w2", "{env:KVD_CONFIG}", "", []facts.DataCall{call("stdio.h.fopen", 321)}, []string{"n3"}, nil, nil),
			record("w3", "", "", []facts.DataCall{call("stdio.h.fopen", 505)}, []string{"n6"}, nil, nil),
		}}
	// The branch the setting's comparison guards, as the GroupsIndex saves it.
	index.Branches = []groupindex.InputBranch{{SubjectID: "n3", Location: *setting, Branch: programindex.LineRange{Line: 328, EndLine: 328}}}
	builder.indexes = []groupindex.Index{index}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder.fillSectionFiles(section, &index)

	// The shelf: each file by its path, or its field's paths and the field;
	// the paths not established are one line.
	var shelf []string
	for _, row := range section.Data.Files {
		shelf = append(shelf, row.ID+" "+row.Path+strings.Join(row.Values, "|")+" "+row.Field)
	}
	if want := []string{"t1-w1 dump.kv server.dbfile", "t1-w2 {env:KVD_CONFIG} "}; !slices.Equal(shelf, want) || !section.Data.Unknown {
		t.Fatalf("the shelf lists %q (unknown %v), want %q", shelf, section.Data.Unknown, want)
	}
	tmpl := template.Must(template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html"))
	var html bytes.Buffer
	if err := tmpl.ExecuteTemplate(&html, "data.html", section); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Files 2", `<li id="t1-w1"><code>dump.kv</code> from <code>server.dbfile</code></li>`, `<li id="t1-w2"><code>{env:KVD_CONFIG}</code></li>`, "Path not established"} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("the shelf omits %q:\n%s", want, html.String())
		}
	}
	if strings.Contains(html.String(), "saveSnapshot") || regexp.MustCompile(`kvd\.c:\d`).MatchString(html.String()) {
		t.Fatalf("the shelf prints the functions or a place:\n%s", html.String())
	}

	// The reading: each file with its functions by part, the part naming
	// most first; the field's writes that store no path it names by the
	// setting whose branch makes them, else by the function, once
	// (rewriteConfig writes it twice).
	var reading pageFiles
	if err := json.Unmarshal([]byte(section.Data.FilesReading), &reading); err != nil {
		t.Fatalf("the component carries no reading of its files: %q, %v", section.Data.FilesReading, err)
	}
	if regexp.MustCompile(`"(line|column)"`).MatchString(section.Data.FilesReading) {
		t.Fatalf("the reading writes a line: %s", section.Data.FilesReading)
	}
	var read []string
	for _, file := range reading.Files {
		line := file.Path + file.Field
		for _, written := range file.Values {
			switch {
			case written.Value != "":
				line += " =" + written.Value + "@" + written.At
			case written.Setting != "":
				line += " setting " + written.Setting + " " + written.Input
			case written.Decl != nil:
				line += " in " + reading.Decls[*written.Decl].Name
			}
		}
		for _, group := range file.By {
			var names []string
			for _, position := range group.Decls {
				names = append(names, reading.Decls[position].Name)
			}
			line += " [" + group.Title + "] " + strings.Join(names, ",")
		}
		read = append(read, line)
	}
	want := []string{
		"server.dbfile setting dbfilename t1-o1 in rewriteConfig =dump.kv@kvd.c:351 [Persistence] rdbLoad,saveSnapshot",
		"{env:KVD_CONFIG} [Configuration] loadConfig",
		" [Configuration] dumpKeys",
	}
	if !slices.Equal(read, want) {
		t.Fatalf("the component reads its files as %q\nwant %q", read, want)
	}
	raw := section.Data.FilesReading

	// The column: the path, linked to where it is written, from its field,
	// what else sets it, each opening to who reads or writes it by part box,
	// one to a line; a name reads its function, the setting its input. A
	// path not established and every count are left out (owner, 2026-09-29).
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading") +
		systemJSPiece(t, "31-reading-column.js", "var rmPendingKind=", "function rmComponentReading(")
	runSystemJS(t, readingViewElements+code+`
nodes['#t1-g1']={dataset:{title:'Persistence',lane:''},getAttribute:()=>'#t1-g1'};
nodes['#t1-g2']={dataset:{title:'Configuration',lane:''},getAttribute:()=>'#t1-g2'};
const inputs={'t1-o1':{dataset:{title:'dbfilename'}}};
ctx.nodeById=id=>inputs[id]||null;
const files=rmComponentFiles(ctx,`+raw+`);
assert.equal(files.tagName,'DETAILS');assert.ok(files.open,'a short list stands open');
assert.equal(files.children[0].textContent,'Files','its heading counts nothing');
const entries=files.children.filter(c=>c.has('map-file'));
assert.deepEqual(entries.map(e=>e.children[0].textContent),['dump.kv from server.dbfile; set by the setting dbfilename; set in rewriteConfig()','{env:KVD_CONFIG}'],'each file by its path; no line for a path not established');
assert.ok(entries.every(e=>e.tagName==='DETAILS'&&!e.open),'each folds its functions under its path');
assert.equal(entries[0].children[0].all(c=>c.has('map-reading-name'))[0].href,'https://github.com/o/r/blob/abc/kvd.c#L351','a path links to where it is written');
assert.deepEqual(entries.map(e=>[e.all(c=>c.has('map-part-box')).map(c=>c.textContent),e.all(c=>c.tagName==='LI').map(c=>c.textContent)]),
 [[['Persistence'],['rdbLoad()','saveSnapshot()']],[['Configuration'],['loadConfig()']]],'who reads or writes it, by part, one to a line');
assert.ok(!/\.c:\d/.test(files.textContent)&&!/Read or written by|Path not established/.test(files.textContent),'no line numbers and no labels: '+files.textContent);
assert.ok(!/\d/.test(files.textContent.replace(/L\d+/g,'')),'no counts: '+files.textContent);
entries[0].all(c=>c.tagName==='LI')[1].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
entries[0].children[0].all(c=>c.tagName==='BUTTON'&&c.textContent==='dbfilename')[0].listeners.click({stopPropagation(){}});
assert.deepEqual(read.slice(-2),['saveSnapshot','dbfilename'],'a name reads its function, a setting its input');
assert.equal(rmComponentFiles(ctx,null),null,'no files, no section');
`)
}
