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
