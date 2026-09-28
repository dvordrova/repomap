package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Redis's main was a near-tie of the parts answer, so no part holds it: the
// map draws no entry part and the code never picks one. The component's
// reading and its "Not on the map" list name main as the program's entry,
// with why it is off the map; the reading's line names it with its file
// and line ("main redis.c:9124"), as the page's source anchors are written.
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
	if !strings.Contains(list.String(), `main<span class="ln">:9124</span></span> <span class="meta">(the program&#39;s entry)</span>`) || strings.Count(list.String(), "the program&#39;s entry") != 1 {
		t.Fatalf("the Not on the map list does not name main as the entry:\n%s", list.String())
	}
	if err := parsed.ExecuteTemplate(&page, "target.html", section); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.String(), `<p class="component-entry">The program&#39;s entry is not on the map: `) || !strings.Contains(page.String(), `In no part of its file`) {
		t.Fatalf("the component does not name its entry:\n%s", page.String()[:min(len(page.String()), 2000)])
	}
	// The entry line names the file, as the page's other source anchors do:
	// "main :9124" had left the reader without it.
	line := page.String()[strings.Index(page.String(), `<p class="component-entry">`):]
	line = line[:strings.Index(line, "</p>")]
	if !strings.Contains(line, `<code>main</code> <span class="anchor">redis.c:9124</span>`) || strings.Contains(line, `class="ln"`) {
		t.Fatalf("the entry line does not name main with its file and line: %s", line)
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
