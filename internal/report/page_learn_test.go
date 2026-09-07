package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// The two entrances must keep exact links to the same sections and nodes,
// including fallback reports without a cross-component repository map.
func TestLearnWorkKeepOneReport(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`data-mode="learn"`, `data-mode="work"`, `id="learn-parts"`, `id="repository-map"`, `id="how-to-run"`, `data-component-name=`} {
		if !bytes.Contains(html, []byte(expected)) {
			t.Fatalf("missing %s", expected)
		}
	}
	if bytes.Count(html, []byte(`data-component-name=`)) != 1 {
		t.Fatal("reading modes duplicated the component")
	}
}

func TestQuestionAnswerRendersItsOwnSourcesAndKeepsTheReadingRoute(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	step := atlas.QuestionStep{Path: "README.md", Line: 14, StopIndexes: []int{0}}
	data.Questions = []atlas.QuestionRoute{{Version: 7, Revision: data.CapturedRevision, Question: "How do I run it?",
		Stops: []atlas.QuestionStop{{Path: "README.md", Line: 14, Name: "Running", Why: "Read the documented command.", Evidence: map[string]any{
			"context": map[string]any{"directory_author_doc": "Unrelated directory context"},
			"evidence": []map[string]any{
				{"anchor_path": "README.md", "anchor_line": 14, "author_text": "Run <example> to start.", "prior_model_hypothesis": "Prior speculation is not a source excerpt."},
				{"anchor_path": "backend/main.py", "anchor_line": 14, "kind": "entrypoint", "name": "main", "entrypoint_kind": "main_guard", "language": "python", "component_kind": "executable", "root": "backend"},
				{"anchor_path": "front/package.json", "anchor_line": 12, "kind": "manifest", "manifest_key": "scripts.start", "value": "run <dev>"},
				{"anchor_path": "backend/models.py", "anchor_line": 3, "kind": "type", "signature": "class Options", "owned_declarations": []map[string]any{
					{"path": "backend/models.py", "line": 4, "name": "labels", "kind": "variable", "signature": "labels: list[str] = ['<run>']"},
					{"path": "backend/methods.py", "line": 12, "name": "start", "kind": "method", "signature": "start(self) -> bool", "author_doc": "Starts a run."},
				}},
			},
		}}},
		Guide:  &atlas.QuestionGuide{State: "ready", Steps: []atlas.QuestionStep{step}},
		Answer: &atlas.QuestionAnswer{State: "partial", Parts: []atlas.QuestionAnswerPart{{State: "partial", Text: "The README documents the local startup command.\n\nRun <example> with 'two  spaces' preserved.", Basis: "The Running section gives the command; this was not executed.", Remaining: "The required environment is not described.", Source: atlas.SourceModel, Steps: []atlas.QuestionStep{step}}}},
	}}
	data.Questions[0].UserQuestion = true
	data.Questions[0].Origins = []atlas.LearningOrigin{{Title: "Run and try it", Question: "Which command starts the app?", Why: "A newcomer needs a runnable starting point.", Sources: data.Questions[0].Stops}}
	data.Learning = &atlas.LearningPlan{State: "ready", Reviews: []atlas.LearningReview{{Title: "Data", State: "unknown", Reason: "Storage is not explained in this excerpt.", PartialContext: true}}}
	data.Learning.Selections = []atlas.LearningSelection{{LearningQuestion: atlas.LearningQuestion{Question: "How does a helper format debug output?", Origins: data.Questions[0].Origins}, Audience: "not_selected", Reason: "The selected menu explains the primary user workflow.", Source: atlas.SourceModel, PartialContext: true}}
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="questions"`, "The README documents the local startup command.", "The required environment is not described.", "README.md:14", "Read the documented command.", "Read the supporting code and documentation", "Check this interpretation", "The Running section gives the command; this was not executed.", "Run &lt;example&gt; to start.", "Why this question?", "Included because you asked it.", "Run and try it", "A newcomer needs a runnable starting point.", "Which command starts the app?", "Data · Still open · part of the repository", "Storage is not explained in this excerpt."} {
		if !bytes.Contains(html, []byte(want)) {
			t.Fatalf("answer missing %s", want)
		}
	}
	for _, want := range []string{"Observed entrypoint", "main · main guard", "python executable in backend", "backend/main.py:14", "Manifest value", "scripts.start = run &lt;dev&gt;", "front/package.json:12", "Declared member", "labels: list[str] = [&#39;&lt;run&gt;&#39;]", "backend/models.py:4", "backend/methods.py:12", "Starts a run."} {
		if !bytes.Contains(html, []byte(want)) {
			t.Fatalf("observed launch/manifest source missing: %s", want)
		}
	}
	for _, want := range []string{"The README documents the local startup command.\n\nRun &lt;example&gt; with &#39;two  spaces&#39; preserved.", `<table class="reading-members">`, `<code>labels: list[str] = [&#39;&lt;run&gt;&#39;]</code>`, `<span>Author's documentation</span>Starts a run.`} {
		if !bytes.Contains(html, []byte(want)) {
			t.Fatalf("answer formatting or original member evidence lost: %s", want)
		}
	}
	for _, misleading := range []string{"Unrelated directory context", "Prior speculation is not a source excerpt."} {
		if bytes.Contains(html, []byte(misleading)) {
			t.Fatalf("source excerpt includes context or interpretation: %s", misleading)
		}
	}
	_, check, _ := strings.Cut(string(html), `<details class="answer-check">`)
	_, check, _ = strings.Cut(check, `<ul class="reading-stops">`)
	check, _, _ = strings.Cut(check, `</ul></details>`)
	for _, source := range []string{"README.md:14", "backend/models.py:3", "backend/models.py:4", "backend/methods.py:12"} {
		if strings.Count(check, source) != 1 {
			t.Fatalf("source check must keep one exact link per declaration: %s", source)
		}
	}
	for _, want := range []string{"How does a helper format debug output? · Not selected for this introduction · compared within part of the question set", "Menu rationale: The selected menu explains the primary user workflow.", "Which questions belong in the introduction?"} {
		if !bytes.Contains(html, []byte(want)) {
			t.Fatalf("selection reason hidden or changed: %s", want)
		}
	}
	data.Questions[0].Answer.Parts[0].Steps[0].Line = 99
	if _, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data)); err == nil {
		t.Fatal("answer accepted another source location")
	}
}

func TestIndependentReadingsDoNotInventAGlobalOrder(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	steps := []atlas.QuestionStep{{Path: "README.md", Line: 14, StopIndexes: []int{0}}, {Path: "README.md", Line: 22, StopIndexes: []int{1}}}
	data.Questions = []atlas.QuestionRoute{{Version: 7, Revision: data.CapturedRevision, Question: "How do I run and test it?",
		Stops: []atlas.QuestionStop{{Path: "README.md", Line: 14, Name: "Running", Why: "Start the application."}, {Path: "README.md", Line: 22, Name: "Testing", Why: "Run the tests."}},
		Guide: &atlas.QuestionGuide{State: "partitioned", Steps: steps, Parts: []atlas.QuestionGuidePart{
			{Source: atlas.SourceModel, Steps: steps[:1], OpenQuestion: "Required environment is not described."},
			{Source: atlas.SourceCache, Steps: steps[1:], OpenQuestion: "none"},
		}},
	}}
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(html, []byte(`<div class="reading-part"><ol`)) != 2 || !bytes.Contains(html, []byte("separate reading orders")) || !bytes.Contains(html, []byte("Required environment is not described.")) || bytes.Contains(html, []byte("some model answers were unavailable")) {
		t.Fatal("separate accepted readings became one order or a provider failure")
	}
	data.Questions[0].Guide.Parts[1].Steps = steps[:1]
	if _, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data)); err == nil {
		t.Fatal("part membership disagrees with the sources used by the answer")
	}
}

func TestLearnKeepsConceptMembershipsAndSkipsRemoteCopies(t *testing.T) {
	concept := `[{"name":"Lease","explanation":"Owns items for a limited time.","source":{"Path":"lease.go","Line":12,"Href":"https://example.test/lease.go#L12"}}]`
	view := &pageView{Sections: []*pageSection{
		{ID: "app", ShortLabel: "server (executable)", Kind: "executable", Map: &pageMap{Nodes: []pageMapNode{
			{ID: "area", FullTitle: "Storage", Children: "lease"},
			{ID: "lease", FullTitle: "Lease management", Concepts: concept},
			{ID: "remote", FullTitle: "Remote copy", Concepts: concept, Remote: true},
			{ID: "op", FullTitle: "Start", Activation: "command"},
		}}},
		{ID: "lib", ShortLabel: "server (library)", Kind: "library", Map: &pageMap{Nodes: []pageMapNode{{ID: "lib-lease", FullTitle: "Lease management", Concepts: concept}}}},
	}}
	builder := &pageBuilder{data: &ReportData{}}
	builder.learn(view)
	if len(view.LearnConcepts) != 1 || len(view.LearnConcepts[0].Places) != 2 {
		t.Fatalf("concepts = %+v", view.LearnConcepts)
	}
	if view.LearnConcepts[0].Source.Line != 12 {
		t.Fatal("lost declaration source")
	}
	if !strings.HasPrefix(view.LearnConcepts[0].ID, "concept-") {
		t.Fatal("term has no individual address")
	}
	if view.LearnConcepts[0].Places[1].Href != "#lib-lease" || !strings.Contains(view.LearnConcepts[0].Places[1].Title, "library") {
		t.Fatal("lost distinct library owner")
	}
	if len(view.LearnBands) != 1 || len(view.LearnBands[0].Parts[0].Areas) != 1 || view.LearnBands[0].Parts[0].Areas[0].Title != "Storage" {
		t.Fatalf("did not preserve root hierarchy: %+v", view.LearnBands)
	}
}

func TestQuestionTermsUseSelectedDeclarationsNotNameGuesses(t *testing.T) {
	lease := pageLearnConcept{ID: "concept-lease", pageMapConcept: pageMapConcept{Name: "Lease", Explanation: "An existing model explanation, with <data> intact.", Source: pageAnchor{Path: "lease.go", Line: 12, Text: "lease.go:12", Href: "https://example.test/lease.go#L12"}},
		Places: []pageLearnLink{{Title: "server / Leases", Href: "#server-leases"}, {Title: "library / Leases", Href: "#library-leases"}}}
	other := lease
	other.ID = "concept-other-lease"
	other.Source.Path = "other/lease.go"
	other.Explanation = "An unrelated meaning of the same word."
	neighbour := lease
	neighbour.ID = "concept-neighbour"
	neighbour.Name = "Lessor"
	neighbour.Explanation = "A neighbouring declaration is not the selected declaration."
	question := &pageQuestion{ID: "question-lease", Question: "What does Lease mean?", Answers: []pageAnswerPart{{Text: "A lease controls data lifetime.", Checks: []pageQuestionStep{
		{Name: "Lease", Source: lease.Source}, {Name: "Lease", Source: lease.Source},
	}}}}
	view := &pageView{LearnConcepts: []pageLearnConcept{lease, other, neighbour}, Questions: []*pageQuestion{question}}
	questionConcepts(view)
	terms := question.Answers[0].Terms
	if len(terms) != 1 || terms[0].ID != lease.ID || len(terms[0].Places) != 2 || terms[0].Explanation != lease.Explanation {
		t.Fatalf("wrong or altered explanations: %+v", terms)
	}
	templates, err := template.New("report").ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := templates.ExecuteTemplate(&html, "question.html", question); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Terms in these sources", "<summary>Lease</summary>", "with &lt;data&gt; intact.", `href="#concept-lease"`, "lease.go:12"} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(html.String(), other.Explanation) || strings.Contains(html.String(), neighbour.Explanation) {
		t.Fatal("unrelated term displayed beside answer")
	}
	if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
		t.Fatal(err)
	}
	if strings.Count(html.String(), `id="concept-lease"`) != 1 {
		t.Fatal("inline explanation duplicated the catalogue's destination")
	}
}
