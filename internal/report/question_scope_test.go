package report

import (
	"bytes"
	stdhtml "html"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/reading"
)

func TestQuestionScopeUsesStaticVocabularyWithoutChangingSavedRoute(t *testing.T) {
	t.Parallel()
	// Obtain the ordinary scope list without a provider, rather than keeping a
	// second English copy that could drift away from the reader's saved route.
	runDir := t.TempDir()
	result, err := reading.Read(t.Context(), reading.Options{
		Graph: atlas.Graph{Revision: "abc", Places: []atlas.Place{{
			ID: atlas.DirectoryID("."), Kind: atlas.PlaceDirectory, Path: ".", Directory: &atlas.DirectoryFacts{},
		}}},
		Repository: "fixture", Revision: "abc", OwnerRunDir: runDir,
		Through: lines.StageQuestion, Questions: []string{"What is included?"},
	})
	if err != nil || len(result.Questions) != 1 {
		t.Fatalf("ordinary question scope: %+v / %v", result.Questions, err)
	}
	route := &result.Questions[0]
	originalScope := slices.Clone(route.Scope)
	if len(originalScope) != 8 {
		t.Fatalf("review vocabulary coverage for the changed scope list: %v", originalScope)
	}
	routePath := filepath.Join(runDir, atlas.QuestionFilename)
	saved, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatal(err)
	}
	question := &pageQuestion{Question: route.Question, Scope: route.Scope}
	page := &PreparedPage{view: &pageView{Questions: []*pageQuestion{question}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range page.catalog.Entries {
		if slices.Contains(originalScope, entry.Text) {
			t.Fatal("static scope was added to the model's display catalogue")
		}
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := parsed.ExecuteTemplate(&out, "question.html", question); err != nil {
				t.Fatal(err)
			}
			html := out.String()
			if strings.Count(html, "<li>") != len(originalScope) {
				t.Fatal("rendered scope lost or duplicated an item")
			}
			for _, source := range originalScope {
				want := source
				if language == Russian {
					want = russianUI[source]
					if want == "" || want == source || strings.Contains(html, stdhtml.EscapeString(source)) {
						t.Fatalf("Russian scope kept its English source: %q", source)
					}
				}
				if !strings.Contains(html, "<li>"+stdhtml.EscapeString(want)+"</li>") {
					t.Fatalf("scope translation missing from static HTML: %q", want)
				}
			}
			if !slices.Equal(route.Scope, originalScope) || !slices.Equal(question.Scope, originalScope) {
				t.Fatal("rendering translated the original source scope in place")
			}
		})
	}
	after, err := os.ReadFile(routePath)
	if err != nil || !bytes.Equal(saved, after) {
		t.Fatalf("rendering changed the saved English route: %v", err)
	}
}
