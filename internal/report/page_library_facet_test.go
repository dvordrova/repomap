package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// freqtrade-client is also installable as library freqtrade_client: the
// library the same manifest installs was folded into the program, and the
// program's page names it where the component's reading copies its entry
// lines from, instead of a second component holding nothing of its own.
func TestAProgramNamesTheLibraryFacetFoldedIntoIt(t *testing.T) {
	builder := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t1", Libraries: []string{"freqtrade_client"}}},
		{Target: programindex.Target{ID: "t2"}},
	}}}}
	section := &pageSection{ID: "t1", programTargetID: "t1", Libraries: builder.libraryFacet("t1")}
	if other := builder.libraryFacet("t2"); len(other) != 0 {
		t.Fatalf("a program with no folded library names one: %v", other)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var page bytes.Buffer
	if err := parsed.ExecuteTemplate(&page, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	intro := html[:strings.Index(html, "</header>")]
	start := strings.Index(intro, `class="component-entry component-library"`)
	if start < 0 {
		t.Fatalf("the program does not name its library facet:\n%s", intro)
	}
	line := intro[start:]
	line = line[:strings.Index(line, "</p>")]
	if !strings.Contains(line, "Also installable as library") || !strings.Contains(line, "<code>freqtrade_client</code>") {
		t.Fatalf("the facet line lacks its name: %s", line)
	}
}
