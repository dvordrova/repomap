package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// Equal names in separate sources remain separately addressable without JS.
// Explicit answer terms must work even when no code concept was selected.
func TestGlossaryHTMLKeepsAnswerTermsAndSourceDistinctDefinitions(t *testing.T) {
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			view := &pageView{Glossary: []pageGlossaryTerm{
				{ID: "term-first", Name: "Разбор", OriginalName: "Parsing", Explanation: "First explanation with <source> intact.",
					Sources:   []pageAnchor{{Text: "first.py:12", Href: "https://example.test/first.py#L12"}, {Text: "first.py:18", Href: "https://example.test/first.py#L18"}},
					Questions: []pageLearnLink{{Title: "How does the first parser work?", Href: "#question-first"}},
					Places:    []pageLearnLink{{Title: "First parser", Href: "#part-first"}}},
				{ID: "term-second", Name: "Разбор", OriginalName: "Parsing", Explanation: "A separate definition from another source.",
					Sources:   []pageAnchor{{Text: "second.py:30", Href: "https://example.test/second.py#L30"}},
					Questions: []pageLearnLink{{Title: "How does the second parser work?", Href: "#question-second"}},
					Places:    []pageLearnLink{{Title: "Second parser", Href: "#part-second"}}},
			}}
			templates, err := template.New("report").Funcs(template.FuncMap{
				"t": func(key string, params ...any) (string, error) { return uiText(language, key, params...) },
			}).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var html bytes.Buffer
			if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
				t.Fatal(err)
			}
			for _, term := range view.Glossary {
				if strings.Count(html.String(), `id="`+term.ID+`"`) != 1 {
					t.Fatalf("term %s lost its separate address", term.ID)
				}
				for _, source := range term.Sources {
					if strings.Count(html.String(), `href="`+source.Href+`"`) != 1 || !strings.Contains(html.String(), source.Text) {
						t.Fatalf("term %s lost source %+v", term.ID, source)
					}
				}
				for _, link := range append(term.Questions, term.Places...) {
					if strings.Count(html.String(), `href="`+link.Href+`"`) != 1 {
						t.Fatalf("term %s lost destination %s", term.ID, link.Href)
					}
				}
			}
			for _, preserved := range []string{"First explanation with &lt;source&gt; intact.", "A separate definition from another source.", `data-term-original="Parsing"`, `class="learn-concept"`, `class="concept-search"`, `class="concept-pages"`} {
				if !strings.Contains(html.String(), preserved) {
					t.Fatalf("static glossary lost %q", preserved)
				}
			}
			if strings.Contains(html.String(), `class="term-mention"`) {
				t.Fatal("glossary definitions contain nested inline term controls")
			}
			if strings.Contains(html.String(), "glossary-comparison-note") {
				t.Fatal("complete glossary carries an incomplete-comparison notice")
			}
			view.GlossaryPartialComparison = true
			html.Reset()
			if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
				t.Fatal(err)
			}
			notice, err := uiText(language, "Some explanations were not compared together; similar entries may remain separate.")
			if err != nil || !strings.Contains(html.String(), notice) || strings.Index(html.String(), notice) > strings.Index(html.String(), `class="concept-results"`) {
				t.Fatal("partial glossary hides its localized notice behind term expansion")
			}
		})
	}
}
