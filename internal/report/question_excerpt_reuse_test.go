package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

func questionExcerptDataFixture(t *testing.T) ReportData {
	t.Helper()
	data := reportProgramShellDataFixture(t, "fixture")
	stop := func(path string, line int, evidence map[string]any) atlas.QuestionStop {
		evidence["anchor_path"], evidence["anchor_line"] = path, line
		return atlas.QuestionStop{Path: path, Line: line, Name: path, Why: "Read this original source.", Evidence: map[string]any{"evidence": []map[string]any{evidence}}}
	}
	stops := []atlas.QuestionStop{
		stop("backend/field.py", 10, map[string]any{"signature": "class Field", "owned_declarations": []map[string]any{
			{"path": "backend/field.py", "line": 11, "signature": "width: int", "author_doc": "Width in cells."},
			{"path": "backend/field.py", "line": 12, "signature": "height: int"},
		}}),
		stop("README.md", 8, map[string]any{"author_text": "Run <server>."}),
		stop("front/client.ts", 12, map[string]any{"method": "GET", "values": []string{"/api/levels"}}),
		stop("backend/app.py", 18, map[string]any{"method": "GET", "values": []string{"/api/levels"}}),
		stop("backend/Pipfile", 10, map[string]any{"kind": "manifest", "manifest_key": "packages.fastapi", "value": "*"}),
	}
	step := func(i int) atlas.QuestionStep {
		return atlas.QuestionStep{Path: stops[i].Path, Line: stops[i].Line, StopIndexes: []int{i}}
	}
	origin := atlas.LearningOrigin{Title: "Run and try it", Question: "How does the game start?", Why: "A newcomer needs a runnable example.", Sources: []atlas.QuestionStop{stops[0], stops[1], stops[3], stops[4]}}
	data.Questions = []atlas.QuestionRoute{{Version: atlas.QuestionRouteVersion, Revision: data.CapturedRevision, Question: origin.Question,
		Stops: stops, Origins: []atlas.LearningOrigin{origin},
		Answer: &atlas.QuestionAnswer{State: "partial", Parts: []atlas.QuestionAnswerPart{
			{State: "unavailable", Source: atlas.SourceGiven},
			{State: "partial", Text: "The game uses a field.", Basis: "The declaration gives its dimensions.", Remaining: "Runtime behavior is not shown.", Source: atlas.SourceModel, Steps: []atlas.QuestionStep{step(0), step(2)}},
			{State: "partial", Text: "The README documents a startup command.", Source: atlas.SourceModel, Steps: []atlas.QuestionStep{step(1)}},
		}},
	}, {Version: atlas.QuestionRouteVersion, Revision: data.CapturedRevision, Question: "What else uses the field?", Stops: stops,
		Origins: []atlas.LearningOrigin{{Title: "Data", Why: "Explore the data model.", Sources: []atlas.QuestionStop{stops[0]}}},
		Answer:  &atlas.QuestionAnswer{State: "unavailable", Parts: []atlas.QuestionAnswerPart{{State: "unavailable", Source: atlas.SourceGiven}}},
	}}
	data.Learning = &atlas.LearningPlan{State: "ready", Reviews: []atlas.LearningReview{{Title: "Data", State: "questions", Reason: "The field is a central concept.", Sources: []atlas.QuestionStop{stops[0]}}}}
	return data
}

func TestQuestionOriginExcerptsReferenceOnlyTheirOwnAnswer(t *testing.T) {
	data := questionExcerptDataFixture(t)
	before, err := json.Marshal(data.Questions)
	if err != nil {
		t.Fatal(err)
	}
	page, err := PreparePage(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	question := page.view.Questions[0]
	if len(question.Answers) != 2 || len(question.Origins[0].Checks[0].Excerpts) != 2 {
		t.Fatal("accepted answer parts or original excerpt data were lost")
	}
	for _, excerpt := range question.Origins[0].Checks[0].Excerpts {
		if excerpt.AnswerCheckID != question.Answers[0].CheckID {
			t.Fatal("declaration and member inventory did not share their accepted answer check")
		}
	}
	if question.Origins[0].Checks[1].Excerpts[0].AnswerCheckID != question.Answers[1].CheckID {
		t.Fatal("the independent answer part lost its own destination")
	}
	for _, check := range question.Origins[0].Checks[2:] {
		for _, excerpt := range check.Excerpts {
			if excerpt.AnswerCheckID != "" {
				t.Fatal("different source or origin-only prerequisite was hidden")
			}
		}
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			parsed, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) { return uiText(language, key, args...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var rendered bytes.Buffer
			if err := parsed.ExecuteTemplate(&rendered, "question.html", question); err != nil {
				t.Fatal(err)
			}
			html := rendered.String()
			for text, count := range map[string]int{`<pre>class Field</pre>`: 1, `<table class="reading-members">`: 1, `<pre>Run &lt;server&gt;.</pre>`: 1, `<pre>GET /api/levels</pre>`: 2, `<pre>packages.fastapi = *</pre>`: 1} {
				if got := strings.Count(html, text); got != count {
					t.Fatalf("%q occurs %d times, want %d", text, got, count)
				}
			}
			label, _ := uiText(language, "Show excerpt")
			for _, answer := range question.Answers {
				if strings.Count(html, `<a href="#`+answer.CheckID+`">`+label+`</a>`) != 1 {
					t.Fatal("one source item must link once even when its declaration and member table are both repeated")
				}
				// The fragment target is the visible summary, so native HTML can
				// reach and open the existing disclosure without JavaScript.
				if strings.Count(html, `<details class="answer-check"><summary id="`+answer.CheckID+`">`) != 1 {
					t.Fatal("excerpt reference has no question-owned visible disclosure target")
				}
			}
			_, origins, found := strings.Cut(html, `<details class="question-origins">`)
			if !found || !strings.Contains(origins, `data-display-ref="`+question.Origins[0].WhyRef+`">A newcomer needs a runnable example.</span>`) || !strings.Contains(origins, "backend/field.py:10") {
				t.Fatal("the origin lost its own reason, display binding or source")
			}
			rendered.Reset()
			if err := parsed.ExecuteTemplate(&rendered, "question.html", page.view.Questions[1]); err != nil {
				t.Fatal(err)
			}
			if strings.Count(rendered.String(), `<pre>class Field</pre>`) != 1 || strings.Contains(rendered.String(), `href="#question-`) {
				t.Fatal("another question with an unavailable answer borrowed a sibling's excerpt")
			}
			rendered.Reset()
			if err := parsed.ExecuteTemplate(&rendered, "learning-review.html", page.view); err != nil {
				t.Fatal(err)
			}
			if strings.Count(rendered.String(), `<pre>class Field</pre>`) != 1 || strings.Contains(rendered.String(), `-answer-check-`) {
				t.Fatal("global learning review was deduplicated into an individual question")
			}
		})
	}
	after, err := json.Marshal(data.Questions)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("display reuse mutated the original question evidence: %v", err)
	}
}

func TestQuestionExcerptReuseRequiresExactContentAndSource(t *testing.T) {
	source := pageAnchor{Path: "field.py", Line: 10, Text: "field.py:10", Href: "https://example.test/field.py#L10"}
	for name, change := range map[string]func(*pageQuestionStep){
		"source path":          func(check *pageQuestionStep) { check.Source.Path = "other.py" },
		"source column":        func(check *pageQuestionStep) { check.Column++ },
		"source href":          func(check *pageQuestionStep) { check.Source.Href += "other" },
		"excerpt location":     func(check *pageQuestionStep) { check.Excerpts[0].Source.Line++ },
		"excerpt kind":         func(check *pageQuestionStep) { check.Excerpts[0].Kind = "Author's documentation" },
		"excerpt text":         func(check *pageQuestionStep) { check.Excerpts[0].Text += " changed" },
		"source visibility":    func(check *pageQuestionStep) { check.Excerpts[0].ShowSource = true },
		"member declaration":   func(check *pageQuestionStep) { check.Excerpts[0].Members[0].Text = "width: str" },
		"member documentation": func(check *pageQuestionStep) { check.Excerpts[0].Members[0].Doc = "Different documentation." },
		"member source":        func(check *pageQuestionStep) { check.Excerpts[0].Members[0].Source.Line++ },
		"member order": func(check *pageQuestionStep) {
			members := check.Excerpts[0].Members
			members[0], members[1] = members[1], members[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			original := pageQuestionStep{Source: source, Column: 3, Excerpts: []pageQuestionExcerpt{{Kind: "Declared members", Text: "Original text", Source: source,
				Members: []pageQuestionMember{{Text: "width: int", Source: source}, {Text: "height: int", Source: source}},
			}}}
			changed := original
			changed.Excerpts = append([]pageQuestionExcerpt(nil), original.Excerpts...)
			changed.Excerpts[0].Members = append([]pageQuestionMember(nil), original.Excerpts[0].Members...)
			change(&changed)
			question := &pageQuestion{ID: "question-one", Answers: []pageAnswerPart{{Checks: []pageQuestionStep{original}}}, Origins: []pageQuestionOrigin{{Checks: []pageQuestionStep{changed}}}}
			linkQuestionOriginExcerpts(question)
			if question.Origins[0].Checks[0].Excerpts[0].AnswerCheckID != "" {
				t.Fatal("different original content or source was treated as an exact copy")
			}
		})
	}
}

func TestQuestionExcerptReferencesPreserveSavedDisplayCatalogue(t *testing.T) {
	data := questionExcerptDataFixture(t)
	data.ArtifactsDir, data.defaultProgramIndexArtifactFilename = t.TempDir(), "program-index.json"
	page, err := PreparePage(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current := page.TextCatalog()
	// Restore the preceding view shape: all original bodies present, without
	// presentation references. Its complete translation input must be identical.
	for _, question := range page.view.Questions {
		for i := range question.Answers {
			question.Answers[i].CheckID = ""
		}
		for i := range question.Origins {
			for j := range question.Origins[i].Checks {
				for k := range question.Origins[i].Checks[j].Excerpts {
					question.Origins[i].Checks[j].Excerpts[k].AnswerCheckID = ""
				}
			}
		}
	}
	previous := &PreparedPage{view: page.view, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := previous.collectDisplayTexts(&data, false); err != nil {
		t.Fatal(err)
	}
	previous.catalog.SHA256 = displayCatalogDigest(previous.catalog.Entries)
	if !reflect.DeepEqual(previous.TextCatalog(), current) {
		t.Fatal("excerpt presentation changed the exact saved translation catalogue")
	}
	// The ordinary translation stage saves this input before publication.
	catalogJSON, err := json.Marshal(previous.TextCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data.ArtifactsDir, "report-display-texts.json"), catalogJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: current.SHA256}
	for i, entry := range current.Entries {
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: fmt.Sprintf("Перевод %d: %s", i+1, entry.Text)})
	}
	manifest := validRunManifestFixture(t)
	source, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := Generate(data.ArtifactsDir, source, GenerateOptions{Data: &data, PublishHTML: true, Render: RenderOptions{Language: Russian, Translations: &translations}})
	if err != nil {
		t.Fatal(err)
	}
	saved := map[string][]byte{}
	for _, name := range []string{receipt.HTMLFilename(), "report.json", RunManifestFilename, "report-display-texts.json", "report-translations.ru.json"} {
		saved[name], err = os.ReadFile(filepath.Join(data.ArtifactsDir, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	rendered, err := RenderSavedHTML(data.ArtifactsDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rendered, saved[receipt.HTMLFilename()]) || !bytes.Contains(rendered, []byte(">Показать фрагмент</a>")) {
		t.Fatal("saved rendering lost the unchanged translations or excerpt references")
	}
	for name, want := range saved {
		got, err := os.ReadFile(filepath.Join(data.ArtifactsDir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("saved rendering modified %s: %v", name, err)
		}
	}
}
