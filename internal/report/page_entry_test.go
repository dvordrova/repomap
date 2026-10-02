package report

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Redis's main was a near-tie of the parts answer, so no part holds it: the
// map draws no entry part and the code never picks one. The component's
// reading and its "Not on the map" list name main as the program's entry,
// with why it is off the map; the reading's line names it with its file,
// linking to its line ("main redis.c"), as a card's source reads (etcd's
// RootCmd had read "tools/proto-annotations/cmd/root.go:28" in its row);
// with no source link its place stays its path and line.
func TestALaunchPointOffTheMapIsNamedWithItsReason(t *testing.T) {
	object := func(id, name string, line int) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction,
			Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}
	}
	index := groupindex.Index{
		Target:   programindex.Target{ID: "t1", Seeds: []programindex.TargetSeed{{ObjectID: "n1"}}},
		Subjects: []groupindex.Subject{object("n1", "main", 9124), object("n2", "aeMain", 300), object("n3", "oom", 1112)},
		Groups:   []groupindex.Group{{ID: "g1", Title: "Event loop", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"n2"}}},
		OffMap:   []groupindex.OffMapFile{{Path: "redis.c", Reason: groupindex.OffMapUndecided, SubjectIDs: []string{"n1", "n3"}}},
	}
	groupindex.Derive(&index)
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subjectKey("t1", subject.ID)] = subjectRef{subject: subject}
	}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder.fillSectionOffMap(section)
	if len(section.OffMapEntries) != 1 || section.OffMapEntries[0].Chip.Name != "main" || section.OffMapEntries[0].Reason != "In no part of its file" {
		t.Fatalf("the entry off the map: %+v", section.OffMapEntries)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var list, page bytes.Buffer
	if err := parsed.ExecuteTemplate(&list, "off-map", section); err != nil {
		t.Fatal(err)
	}
	if strings.Count(list.String(), "the program&#39;s entry") != 1 {
		t.Fatalf("the Not on the map list does not name main as the entry once:\n%s", list.String())
	}
	if err := parsed.ExecuteTemplate(&page, "target.html", section); err != nil {
		t.Fatal(err)
	}
	// The component names its entry with why it is off the map, and with its
	// file and line, as the page's other source anchors are written.
	start := strings.Index(page.String(), `class="component-entry"`)
	if start < 0 {
		t.Fatalf("the component does not name its entry:\n%s", page.String()[:min(len(page.String()), 2000)])
	}
	line := page.String()[start:]
	line = line[:strings.Index(line, "</p>")]
	for _, want := range []string{"main", "redis.c:9124", "In no part of its file"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the entry line lacks %q: %s", want, line)
		}
	}
	// Linked, it reads as its file, its link to its line.
	builder.links = newPageLinks(&ReportData{GitHubSourceLinks: &GitHubSourceLinks{RepositoryURL: "https://github.com/redis/redis", Revision: "abc"}})
	linked := &pageSection{ID: "t1", programTargetID: "t1"}
	builder.fillSectionOffMap(linked)
	page.Reset()
	if err := parsed.ExecuteTemplate(&page, "target.html", linked); err != nil {
		t.Fatal(err)
	}
	line = page.String()[strings.Index(page.String(), `class="component-entry"`):]
	line = line[:strings.Index(line, "</p>")]
	if !strings.Contains(line, `href="https://github.com/redis/redis/blob/abc/redis.c#L9124" target="_blank">redis.c</a>`) || strings.Contains(line, "redis.c:9124") {
		t.Fatalf("the linked entry line does not read as its file linking to its line: %s", line)
	}
	for _, group := range index.Groups {
		if group.Lane == groupindex.LaneTriggers {
			t.Fatalf("a part was made the entry: %+v", group)
		}
	}
}

// The component's "Entrypoints" link had landed on its inputs, where a
// reader looking for main found none. It lands on the program's entry: the
// part holding the seed, with the seed read there; a seed no part holds
// leaves no part, and the link reads the component at its entry line.
func TestTheEntrypointsLinkLandsOnTheProgramsEntry(t *testing.T) {
	object := func(id, name string, line int) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction,
			Location: &programindex.Location{Path: "kvd.c", Line: line, Column: 1}}}
	}
	build := func(groups []groupindex.Group, seeds ...string) (*pageSection, string) {
		index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: groups,
			Subjects: []groupindex.Subject{object("n1", "main", 302), object("n2", "loopMain", 40), object("n3", "setupSignals", 273)}}
		for _, seed := range seeds {
			index.Target.Seeds = append(index.Target.Seeds, programindex.TargetSeed{ObjectID: seed})
		}
		groupindex.Derive(&index)
		builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{},
			links: pageLinks{repositoryURL: "https://example.test/kvd", blobPrefix: "/blob/", revision: "r"}}
		for _, subject := range index.Subjects {
			builder.subjects[subjectKey("t1", subject.ID)] = subjectRef{subject: subject}
		}
		section := &pageSection{ID: "t1", programTargetID: "t1", FactsAvailable: true, Entrypoints: []pageEntrypoint{{Symbol: "main"}}}
		builder.fillSectionOffMap(section)
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var page bytes.Buffer
		if err := parsed.ExecuteTemplate(&page, "target.html", section); err != nil {
			t.Fatal(err)
		}
		link := page.String()[strings.Index(page.String(), `<a href="#t1-entrypoints"`):]
		return section, link[:strings.Index(link, ">")+1]
	}
	server := groupindex.Group{ID: "g1", Title: "Server", MemberSubjectIDs: []string{"n1", "n3"}}
	loop := groupindex.Group{ID: "g2", Title: "Event loop", MemberSubjectIDs: []string{"n2"}}
	section, link := build([]groupindex.Group{server, loop}, "n1")
	if section.EntryPart != groupAnchorID("t1", "g1") || section.EntrySource != "https://example.test/kvd/blob/r/kvd.c#L302" {
		t.Fatalf("the entry on the map lands at %q %q", section.EntryPart, section.EntrySource)
	}
	if link != `<a href="#t1-entrypoints" data-entry-landing data-entry-part="t1-g1" data-entry-source="https://example.test/kvd/blob/r/kvd.c#L302">` {
		t.Fatalf("the Entrypoints link: %s", link)
	}
	// Two seeds in one part land on the part; in two parts, on no part.
	if section, _ := build([]groupindex.Group{server, loop}, "n1", "n3"); section.EntryPart != "t1-g1" || section.EntrySource != "" {
		t.Fatalf("two seeds in one part land at %q %q", section.EntryPart, section.EntrySource)
	}
	if section, _ := build([]groupindex.Group{server, loop}, "n1", "n2"); section.EntryPart != "" {
		t.Fatalf("seeds in two parts land on %q", section.EntryPart)
	}
	// A seed no part holds leaves the link to the component's entry line.
	section, link = build([]groupindex.Group{loop}, "n1")
	if section.EntryPart != "" || section.EntrySource != "" || link != `<a href="#t1-entrypoints" data-entry-landing>` {
		t.Fatalf("an entry off the map lands at %q %q: %s", section.EntryPart, section.EntrySource, link)
	}
}

// liblua.a's Entry list named 156 exports in one flat list, each a link to
// its source: GroupsIndex stands each in its part, and the page says which,
// so the list folds by part and an entry reads its function there. The
// export is joined to its part by its subject, never by a link's key: Lua
// 5.1.5 has no remote, so offline every key is empty, and served its
// export fact's anchor stands without the column its declaration's has
// (etc/noparser.c:21:0 against :21:16); its 159 exports had stood flat.
func TestALibrarysExportsNameTheirParts(t *testing.T) {
	object := func(id, name string, line int) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction,
			Location: &programindex.Location{Path: "lapi.c", Line: line, Column: 16}}}
	}
	for _, links := range []struct {
		name  string
		links pageLinks
	}{
		{"linked to its remote", pageLinks{repositoryURL: "https://example.test/lua", blobPrefix: "/blob/", revision: "r"}},
		{"offline with no remote", pageLinks{}},
		{"served", pageLinks{sourceIDs: map[string]string{"lapi.c": "s1"}}},
	} {
		index := groupindex.Index{
			Target: programindex.Target{ID: "t1", Seeds: []programindex.TargetSeed{{ObjectID: "n4"}},
				Exports: []programindex.TargetExport{{ObjectID: "n1"}, {ObjectID: "n2"}, {ObjectID: "n3"}}},
			Subjects: []groupindex.Subject{object("n1", "lua_pushnil", 10), object("n2", "luaL_checkint", 20), object("n3", "lua_settop", 30), object("n4", "lapi", 1)},
			Groups:   []groupindex.Group{{ID: "g1", Title: "Core API", MemberSubjectIDs: []string{"n1", "n3", "n4"}}, {ID: "g2", Title: "Auxiliary library", MemberSubjectIDs: []string{"n2"}}},
		}
		groupindex.Derive(&index)
		builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{}, links: links.links}
		for _, subject := range index.Subjects {
			builder.subjects[subjectKey("t1", subject.ID)] = subjectRef{subject: subject}
		}
		// The facts' anchors carry no column.
		export := func(name, id string, line int) pageEntrypoint {
			return pageEntrypoint{Symbol: name, Kind: "export", Anchor: builder.links.anchorPointer("lapi.c", line, 0), ObjectID: id}
		}
		// A module run as a script: its fact stands at its __main__ block
		// (line 82), its subject at the module's first line.
		script := export("lapi", "n4", 82)
		script.Kind = "callable"
		section := &pageSection{ID: "t1", programTargetID: "t1", FactsAvailable: true, Entrypoints: []pageEntrypoint{
			export("lua_pushnil", "n1", 10), export("luaL_checkint", "n2", 20), export("lua_settop", "n3", 30), script}}
		builder.fillSectionOffMap(section)
		var entries []pageEntry
		if err := json.Unmarshal([]byte(componentEntries(section)), &entries); err != nil {
			t.Fatal(err)
		}
		parts, keys := map[string]string{}, map[string]string{}
		for _, entry := range entries {
			parts[entry.Name], keys[entry.Name] = entry.Part, entry.Key
		}
		if parts["lua_pushnil"] != "#t1-g1" || parts["lua_settop"] != "#t1-g1" || parts["luaL_checkint"] != "#t1-g2" {
			t.Fatalf("%s: the exports' parts: %v", links.name, parts)
		}
		// Read in its part by the key the part lists it under.
		if want := declarationKeyOf(&builder, "t1", "n1"); keys["lua_pushnil"] != want {
			t.Fatalf("%s: lua_pushnil read by %q, its part lists %q", links.name, keys["lua_pushnil"], want)
		}
		// The script keeps its own place, its __main__ block, never moved to
		// the module's first line (control review, 2026-10-02: freqtrade.main
		// had moved from main.py:82 to :1).
		if want := declarationKey(builder.links.anchorPointer("lapi.c", 82, 0)); parts["lapi"] != "" || keys["lapi"] != want {
			t.Fatalf("%s: the script read in %q by %q, want its own place %q", links.name, parts["lapi"], keys["lapi"], want)
		}
	}
}
