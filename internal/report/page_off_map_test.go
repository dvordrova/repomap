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
