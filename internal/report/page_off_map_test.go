package report

import (
	"bytes"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The component card lists the files off the map: the test-only parts' files
// under Tests, the rest under Not on the map with why. A route in a file off
// the map stays in the component's inputs without a part to stand in.
func TestOffMapFilesAndTheirRoutesReachTheCard(t *testing.T) {
	const target = "application"
	location := func(path string) *programindex.Location {
		return &programindex.Location{Path: path, Line: 3, Column: 7}
	}
	index := groupindex.Index{Target: programindex.Target{ID: target, TestSources: []string{"checks/market_test.go"}},
		Subjects: []groupindex.Subject{
			{ID: "serve", Object: &groupindex.ObjectFacts{Name: "serve", Location: location("service.go")}},
			{ID: "loose", Object: &groupindex.ObjectFacts{Name: "loose", Location: location("loose/handler.go")}},
			{ID: "check", Object: &groupindex.ObjectFacts{Name: "check", Location: location("checks/market_test.go")}},
		},
		Groups: []groupindex.Group{
			{ID: "core", Title: "Service", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"serve"}, EvidenceSubjectIDs: []string{}},
		},
		Operations: []groupindex.Operation{
			{ID: "o1", SubjectID: "loose", Name: "GET /loose", Kind: "request", Source: "model", Location: *location("loose/handler.go")},
		},
		OffMap: []groupindex.OffMapFile{
			{Path: "checks/market_test.go", Reason: groupindex.OffMapTests, Part: "Market checks"},
			{Path: "loose/handler.go", Reason: "left_out"},
		},
	}
	section := &pageSection{ID: "app", programTargetID: target, ShortLabel: "App"}
	builder := &pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subject.ID] = subjectRef{subject: subject}
	}
	builder.fillSectionOffMap(section)
	if len(section.TestFiles) != 1 || section.TestFiles[0].Anchor.Path != "checks/market_test.go" || section.TestFiles[0].Part != "Market checks" {
		t.Fatalf("tests: %+v", section.TestFiles)
	}
	if len(section.OffMap) != 1 || section.OffMap[0].Anchor.Path != "loose/handler.go" || section.OffMap[0].Reason != "Left out of the parts" {
		t.Fatalf("not on the map: %+v", section.OffMap)
	}
	overview := builder.overviewBuilder()
	section.Map = overview.buildMap(section)
	overview.fillSectionOperations(section)
	if len(section.Requests) != 1 || section.Requests[0].Name != "GET /loose" {
		t.Fatalf("the route off the map left the inputs: %+v", section.Requests)
	}
	for _, node := range section.Map.Nodes {
		if node.Activation != "" && node.InputOwner != "" {
			t.Fatalf("a route off the map stands in a part: %+v", node)
		}
	}
	for _, edge := range section.Map.Edges {
		if edge.To == mapNodeID("") {
			t.Fatalf("an edge leads to no part: %+v", edge)
		}
	}
}

// A split file's declarations no box took are listed with their source
// links, by their subjects, so Find can list them as code.
func TestUndecidedDeclarationsKeepTheirSourceLinks(t *testing.T) {
	const target = "server"
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "redis.c", Line: line, Column: 6}
	}
	index := groupindex.Index{Target: programindex.Target{ID: target},
		Subjects: []groupindex.Subject{
			{ID: "n1", Object: &groupindex.ObjectFacts{Name: "setCommand", Location: at(3753)}},
			{ID: "n2", Object: &groupindex.ObjectFacts{Name: "saveparam", Location: at(332)}},
			{ID: "n3", Object: &groupindex.ObjectFacts{Name: "getCommand", Location: at(3776)}},
		},
		OffMap: []groupindex.OffMapFile{{Path: "redis.c", Reason: groupindex.OffMapUndecided, SubjectIDs: []string{"n2", "n1"}}},
	}
	section := &pageSection{ID: "server", programTargetID: target, ShortLabel: "Server"}
	builder := &pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subjectKey(target, subject.ID)] = subjectRef{subject: subject, programTargetID: target}
	}
	builder.fillSectionOffMap(section)
	if len(section.OffMap) != 1 {
		t.Fatalf("not on the map: %+v", section.OffMap)
	}
	var got []string
	for _, chip := range section.OffMap[0].Members {
		got = append(got, fmt.Sprintf("%s:%d %s", chip.Name, chip.Line, chip.Anchor.Path))
	}
	if want := []string{"saveparam:332 redis.c", "setCommand:3753 redis.c"}; !slices.Equal(got, want) {
		t.Fatalf("undecided declarations %v, want %v", got, want)
	}
}

// A part its program never runs is listed where the program's unreachable
// code is, with its name and its declarations, and those declarations are
// not listed again among the unreachable symbols. redis-cli's "Linked list"
// stood on its map; its thirteen functions were listed twice over.
func TestAPartTheProgramNeverRunsIsListedWithItsUnreachableCode(t *testing.T) {
	const target = "cli"
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	objects := []programindex.Object{
		{ID: "n1", Kind: programindex.ObjectFunction, Name: "listCreate", Location: at("adlist.c", 41), Unreachable: true},
		{ID: "n2", Kind: programindex.ObjectFunction, Name: "listRelease", Location: at("adlist.c", 58), Unreachable: true},
		{ID: "n3", Kind: programindex.ObjectType, Name: "list", Location: at("adlist.h", 50)},
		{ID: "n4", Kind: programindex.ObjectFunction, Name: "sdsdup", Location: at("sds.c", 79), Unreachable: true},
		{ID: "n5", Kind: programindex.ObjectFunction, Name: "sdsnew", Location: at("sds.c", 69)},
	}
	index := groupindex.Index{Target: programindex.Target{ID: target},
		Groups: []groupindex.Group{{ID: "g1", Title: "Dynamic strings", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"n4", "n5"}, EvidenceSubjectIDs: []string{}}},
		OffMap: []groupindex.OffMapFile{
			{Path: "adlist.c", Reason: groupindex.OffMapUnreachable, Part: "Linked list", SubjectIDs: []string{"n1", "n2"}},
			{Path: "adlist.h", Reason: groupindex.OffMapUnreachable, Part: "Linked list", SubjectIDs: []string{"n3"}},
		},
	}
	for _, object := range objects {
		index.Subjects = append(index.Subjects, groupindex.Subject{ID: object.ID, Object: &groupindex.ObjectFacts{Name: object.Name, Kind: object.Kind, Location: object.Location}})
	}
	section := &pageSection{ID: "cli", programTargetID: target, ShortLabel: "redis-cli"}
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Target: programindex.Target{ID: target}, Objects: objects}}}},
		indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subjectKey(target, subject.ID)] = subjectRef{subject: subject, programTargetID: target}
	}
	builder.fillSectionOffMap(section)
	if len(section.TestFiles) != 0 || len(section.OffMap) != 0 {
		t.Fatalf("the part is listed as tests or off the map: %+v %+v", section.TestFiles, section.OffMap)
	}
	var parts []string
	for _, row := range section.UnreachedParts {
		line := row.Anchor.Path + " · " + row.Part
		for _, chip := range row.Members {
			line += fmt.Sprintf(" %s:%d", chip.Name, chip.Line)
		}
		parts = append(parts, line)
	}
	if want := []string{"adlist.c · Linked list listCreate:41 listRelease:58", "adlist.h · Linked list list:50"}; !slices.Equal(parts, want) {
		t.Fatalf("parts never run = %q, want %q", parts, want)
	}
	var symbols []string
	for _, row := range builder.unreachedRows(target) {
		for _, chip := range row.Members {
			symbols = append(symbols, row.Path+" "+chip.Name)
		}
	}
	if want := []string{"sds.c sdsdup"}; !slices.Equal(symbols, want) {
		t.Fatalf("unreachable symbols = %q, want %q", symbols, want)
	}
	// On the page the part stands under "Not reachable from the
	// entrypoints", before the symbols, and not under Tests or Not on the
	// map.
	section.FactsAvailable, section.Entrypoints, section.Map = true, []pageEntrypoint{{Symbol: "main"}}, &pageMap{}
	section.Unreached = builder.unreachedRows(target)
	section.UnreachedCount = 1
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	heading, part, symbolsAt := strings.Index(html, "Not reachable from the entrypoints"), strings.Index(html, " · Linked list · "), strings.Index(html, "unreached-symbols")
	if heading < 0 || part < heading || symbolsAt < part || strings.Contains(html, "off-map-catalog") {
		t.Fatalf("the part is not listed under what is not reachable (heading %d, part %d, symbols %d):\n%s", heading, part, symbolsAt, html)
	}
}

// A declaration one program never runs names the other programs of the
// report that run it: the same file, line, column, kind and name in an
// index that proves what its program runs and does not prove this one
// unreachable. A same-named function elsewhere is another declaration, and
// an index that proves nothing (a library, any adapter but C) runs nothing
// here. redis-server's list told a newcomer to skip aeStop, which
// redis-benchmark runs.
func TestAnUnreachedDeclarationNamesTheProgramsThatRunIt(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 6}
	}
	program := func(id string, objects ...programindex.Object) programindex.Index {
		return programindex.Index{Target: programindex.Target{ID: id}, Objects: objects}
	}
	portfolio := &ProgramPortfolio{Entries: []programindex.Index{
		program("t1",
			programindex.Object{ID: "n1", Kind: programindex.ObjectFunction, Name: "main", Location: at("redis.c", 9124)},
			programindex.Object{ID: "n2", Kind: programindex.ObjectFunction, Name: "aeStop", Location: at("ae.c", 82), Unreachable: true},
			programindex.Object{ID: "n3", Kind: programindex.ObjectFunction, Name: "anetRead", Location: at("anet.c", 182), Unreachable: true},
			programindex.Object{ID: "n4", Kind: programindex.ObjectFunction, Name: "zipmapRepr", Location: at("zipmap.c", 400), Unreachable: true}),
		// redis-benchmark runs aeStop and proves anetRead unreachable.
		program("t2",
			programindex.Object{ID: "n1", Kind: programindex.ObjectFunction, Name: "main", Location: at("redis-benchmark.c", 483)},
			programindex.Object{ID: "n7", Kind: programindex.ObjectFunction, Name: "aeStop", Location: at("ae.c", 82)},
			programindex.Object{ID: "n8", Kind: programindex.ObjectFunction, Name: "anetRead", Location: at("anet.c", 182), Unreachable: true}),
		// redis-cli runs anetRead, and its own zipmapRepr is another function.
		program("t3",
			programindex.Object{ID: "n1", Kind: programindex.ObjectFunction, Name: "main", Location: at("redis-cli.c", 501)},
			programindex.Object{ID: "n2", Kind: programindex.ObjectFunction, Name: "anetRead", Location: at("anet.c", 182)},
			programindex.Object{ID: "n3", Kind: programindex.ObjectFunction, Name: "zipmapRepr", Location: at("redis-cli.c", 90)},
			programindex.Object{ID: "n4", Kind: programindex.ObjectFunction, Name: "cliUnused", Location: at("redis-cli.c", 120), Unreachable: true}),
		// A library proves nothing: holding zipmap.c does not run it.
		program("t4",
			programindex.Object{ID: "n1", Kind: programindex.ObjectFunction, Name: "zipmapRepr", Location: at("zipmap.c", 400)}),
	}}
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: portfolio}, subjects: map[string]subjectRef{}, byProgram: map[string]*pageSection{}}
	for i, name := range []string{"redis-server", "redis-benchmark", "redis-cli", "libzipmap"} {
		section := &pageSection{ID: fmt.Sprintf("t%d", i+1), programTargetID: fmt.Sprintf("t%d", i+1), Label: name}
		builder.sections = append(builder.sections, section)
		builder.byProgram[section.programTargetID] = section
	}
	for _, index := range portfolio.Entries {
		for _, object := range index.Objects {
			builder.subjects[subjectKey(index.Target.ID, object.ID)] = subjectRef{programTargetID: index.Target.ID,
				subject: groupindex.Subject{ID: object.ID, Object: &groupindex.ObjectFacts{Name: object.Name, Kind: object.Kind, Location: object.Location}}}
		}
	}
	var listed []string
	for _, row := range builder.unreachedRows("t1") {
		for _, chip := range row.Members {
			line := chip.Name
			for _, program := range chip.RunBy {
				line += " · " + program.Name + " " + program.Href
			}
			listed = append(listed, line)
		}
	}
	if want := []string{"aeStop · redis-benchmark #t2", "anetRead · redis-cli #t3", "zipmapRepr"}; !slices.Equal(listed, want) {
		t.Fatalf("redis-server's unreached declarations = %q, want %q", listed, want)
	}
}

// A declaration two programs hold is one declaration: its "Called by" names
// the calls every other program's own code makes into it, by program, and a
// program that never runs it says so. A caller its program never runs is no
// caller, and a same-named function elsewhere is another declaration.
// anetTcpConnect's "Called by" had listed only redis-server's callers.
func TestASharedDeclarationListsTheCallsEveryProgramMakesIntoIt(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 5}
	}
	function := func(id, name, path string, line int, unreachable bool) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: name, Location: at(path, line), Unreachable: unreachable}
	}
	call := func(from, to string, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget,
			RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, Location: at("x.c", line)}
	}
	portfolio := &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t1"}, Objects: []programindex.Object{
			function("n1", "syncWithMaster", "replication.c", 300, false), function("n2", "anetTcpConnect", "anet.c", 129, false)}},
		{Target: programindex.Target{ID: "t2"}, Objects: []programindex.Object{
			function("n5", "cliConnect", "redis-cli.c", 60, false), function("n6", "anetTcpConnect", "anet.c", 129, false),
			function("n7", "cliDead", "redis-cli.c", 90, true), function("n8", "anetTcpConnect", "other.c", 129, false)}},
		{Target: programindex.Target{ID: "t3"}, Objects: []programindex.Object{
			function("n2", "anetTcpConnect", "anet.c", 129, true)}},
	}}
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: portfolio}, byProgram: map[string]*pageSection{}, indexes: []groupindex.Index{
		{Target: programindex.Target{ID: "t1"}, StructuralEdges: []groupindex.StructuralEdge{call("n1", "n2", 310)}},
		{Target: programindex.Target{ID: "t2"}, StructuralEdges: []groupindex.StructuralEdge{call("n5", "n6", 70), call("n7", "n6", 95), call("n5", "n8", 71)}},
		{Target: programindex.Target{ID: "t3"}},
	}}
	for i, name := range []string{"redis-server", "redis-cli", "redis-benchmark"} {
		section := &pageSection{ID: fmt.Sprintf("t%d", i+1), programTargetID: fmt.Sprintf("t%d", i+1), Label: name}
		builder.sections = append(builder.sections, section)
		builder.byProgram[section.programTargetID] = section
	}
	callers := func(targetID, subjectID string) []string {
		var listed []string
		for _, call := range builder.callersElsewhere(targetID, subjectID) {
			listed = append(listed, call.targetID+" "+call.caller)
		}
		return listed
	}
	if got := callers("t1", "n2"); !slices.Equal(got, []string{"t2 n5"}) {
		t.Fatalf("redis-server's anetTcpConnect is called elsewhere by %q, want redis-cli's cliConnect", got)
	}
	if got := callers("t3", "n2"); !slices.Equal(got, []string{"t1 n1", "t2 n5"}) {
		t.Fatalf("redis-benchmark's anetTcpConnect is called elsewhere by %q, want both programs' callers", got)
	}
	if got := callers("t2", "n8"); len(got) != 0 {
		t.Fatalf("a same-named function elsewhere is called by %q", got)
	}
	if !builder.neverRun("t3", "n2") || builder.neverRun("t1", "n2") {
		t.Fatal("a program's unreachable proof is not read per program")
	}
}

// A call leaving its program is read from the nearest function of that
// program's own code: shared anet.c is held by both programs, so redis-cli's
// connect reads from cliConnect and redis-server's accept from
// acceptHandler. A caller its program never runs is on no path, and a
// declaration of the program's own code is its own path.
func TestACallLeavingItsProgramReadsFromItsOwnCode(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 5}
	}
	function := func(id, name, path string, line int, unreachable bool) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: name, Location: at(path, line), Unreachable: unreachable}
	}
	call := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget,
			RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	anet := []programindex.Object{function("a1", "anetTcpConnect", "anet.c", 170, false), function("a2", "anetTcpGenericConnect", "anet.c", 128, false), function("a3", "anetAccept", "anet.c", 248, false)}
	portfolio := &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t1"}, Objects: append(slices.Clone(anet), function("s1", "acceptHandler", "redis.c", 2551, false), function("s2", "syncWithMaster", "redis.c", 5000, false))},
		{Target: programindex.Target{ID: "t2"}, Objects: append(slices.Clone(anet), function("c1", "cliConnect", "redis-cli.c", 174, false), function("c2", "cliDead", "redis-cli.c", 90, true))},
	}}
	builder := &pageBuilder{links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}, data: &ReportData{ProgramPortfolio: portfolio}, byProgram: map[string]*pageSection{}, subjects: map[string]subjectRef{}, groupEdges: map[string]*groupEdges{}, indexes: []groupindex.Index{
		{Target: programindex.Target{ID: "t1"}, StructuralEdges: []groupindex.StructuralEdge{call("s1", "a3"), call("s2", "a1"), call("a1", "a2")},
			Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"s1", "s2"}}, {ID: "g2", MemberSubjectIDs: []string{"a1", "a2", "a3"}}}},
		{Target: programindex.Target{ID: "t2"}, StructuralEdges: []groupindex.StructuralEdge{call("c2", "a2"), call("a1", "a2"), call("c1", "a1")}},
	}}
	for i, name := range []string{"redis-server", "redis-cli"} {
		section := &pageSection{ID: fmt.Sprintf("t%d", i+1), programTargetID: fmt.Sprintf("t%d", i+1), ShortLabel: name}
		builder.sections = append(builder.sections, section)
		builder.byProgram[section.programTargetID] = section
	}
	for _, index := range portfolio.Entries {
		for _, object := range index.Objects {
			builder.subjects[subjectKey(index.Target.ID, object.ID)] = subjectRef{programTargetID: index.Target.ID,
				subject: groupindex.Subject{ID: object.ID, Object: &groupindex.ObjectFacts{Name: object.Name, Kind: object.Kind, Location: object.Location}}}
		}
	}
	said := func(side *pageCallSide) string {
		if side == nil {
			return "<none>"
		}
		var names []string
		for _, step := range side.Path {
			names = append(names, step.Name)
		}
		return side.Program + ": " + strings.Join(names, " → ")
	}
	for _, check := range []struct{ target, subject, want string }{
		{"t2", "a2", "redis-cli: cliConnect → anetTcpConnect → anetTcpGenericConnect"},
		{"t1", "a3", "redis-server: acceptHandler → anetAccept"},
		{"t1", "a2", "redis-server: syncWithMaster → anetTcpConnect → anetTcpGenericConnect"},
		{"t1", "s1", "redis-server: acceptHandler"},
		// Nothing of redis-cli's own code calls its anetAccept.
		{"t2", "a3", "redis-cli: anetAccept"},
	} {
		if got := said(builder.callSide(check.target, check.subject)); got != check.want {
			t.Errorf("side of %s %s = %q, want %q", check.target, check.subject, got, check.want)
		}
	}
	if side := builder.callSide("t1", "a2"); side.Path[0].Part != "n-t1-g1" || side.Path[2].Part != "n-t1-g2" || side.Path[0].Key != "https://github.com/o/r/blob/abc/redis.c#L5000" {
		t.Fatalf("a side's steps do not name the part each is read in: %+v", side.Path)
	}
}
