package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
)

func TestQuestionExcerptHeadingsTranslateWithoutChangingOriginalText(t *testing.T) {
	const sourceText = "我来到北京清华大学 <cut> & test (executable)"
	anchor := pageAnchor{Path: "jieba/__init__.py", Text: "jieba/__init__.py:53", Href: "https://example.test/jieba.py#L53"}
	excerpts := []pageQuestionExcerpt{
		{Kind: "Declaration", Text: sourceText, Source: anchor, ShowSource: true},
		{Kind: "Author's documentation", Text: sourceText, Source: anchor},
		{Kind: "Observed entrypoint", Text: sourceText, Source: anchor},
		{Kind: "Manifest value", Text: sourceText, Source: anchor},
		{Kind: "Observed boundary", Text: sourceText, Source: anchor},
		{Kind: "Declared members", Source: anchor, ShowSource: true, Members: []pageQuestionMember{{Text: sourceText, Doc: sourceText, Source: anchor}}},
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := parsed.ExecuteTemplate(&out, "question-excerpts.html", excerpts); err != nil {
				t.Fatal(err)
			}
			html := out.String()
			for _, excerpt := range excerpts {
				if !strings.Contains(html, stdhtml.EscapeString(englishUI(language, excerpt.Kind))) {
					t.Fatalf("missing translated heading %q", excerpt.Kind)
				}
			}
			if strings.Count(html, "<pre>"+stdhtml.EscapeString(sourceText)+"</pre>") != 5 || !strings.Contains(html, "<code>"+stdhtml.EscapeString(sourceText)+"</code>") || !strings.Contains(html, `href="https://example.test/jieba.py#L53"`) {
				t.Fatal("translated headings changed an original quote, member or source destination")
			}
		})
	}
}

func TestReportRenderingIsEnglishOnly(t *testing.T) {
	t.Parallel()

	data := reportProgramShellDataFixture(t, "fixture")
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<html lang="en">`,
		`JavaScript is optional here`,
		`id="rm-report-app-js"`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("English HTML is missing %q", want)
		}
	}
	for _, unwanted := range []string{`"report_language"`, "rm-localization-status"} {
		if strings.Contains(string(html), unwanted) {
			t.Fatalf("English HTML retained removed presentation state %q", unwanted)
		}
	}

	t.Run("ordinary words matching declarations remain translatable", func(t *testing.T) {
		original := "Run the script to inspect the service mesh and its protocol. Use Run(service), pkg.Run, `service`, NewServer and app/server.go."
		masked, protected := protectedDisplayText(original, []string{"Run", "service", "protocol", "NewServer", "app/server.go"})
		if !strings.HasPrefix(masked, "Run the script to inspect the service mesh and its protocol.") {
			t.Fatalf("a same-named declaration froze ordinary prose: %q", masked)
		}
		var fragments []string
		var replacements []string
		for _, item := range protected {
			fragments = append(fragments, item.Text)
			replacements = append(replacements, item.Ref, item.Text)
		}
		want := []string{"Run(service)", "pkg.Run", "`service`", "NewServer", "app/server.go"}
		if !reflect.DeepEqual(fragments, want) || strings.NewReplacer(replacements...).Replace(masked) != original {
			t.Fatalf("explicit code/path fragments changed: %#v, want %#v", fragments, want)
		}
	})

	t.Run("literal placeholder names round trip beside code", func(t *testing.T) {
		const original = "Inspect `code` then literal __REPOMAP_P1__ and __REPOMAP_P2__."
		page := &PreparedPage{view: &pageView{Summary: &pageSentence{Text: original}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
		if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
			t.Fatal(err)
		}
		page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
		catalog := page.TextCatalog()
		if len(catalog.Entries) != 1 || len(catalog.Entries[0].Protected) != 3 {
			t.Fatalf("code and literal markers were not protected independently: %#v", catalog.Entries)
		}
		entry := catalog.Entries[0]
		translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: catalog.SHA256,
			Entries: []DisplayTranslationEntry{{Ref: entry.Ref, Text: strings.Replace(entry.Text, "Inspect", "Проверьте", 1)}}}
		if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
			t.Fatal(err)
		}
		if want := strings.Replace(original, "Inspect", "Проверьте", 1); page.view.Summary.Text != want {
			t.Fatalf("a restored literal was recursively replaced: got %q, want %q", page.view.Summary.Text, want)
		}
	})

	t.Run("boundary purposes translate beside exact route labels", func(t *testing.T) {
		purposes := []string{"Accepts a request to run a level from the frontend via HTTP POST.", "Serves level information to the frontend via HTTP GET."}
		translated := []string{"Принимает запрос на запуск уровня через HTTP POST.", "Возвращает сведения об уровнях через HTTP GET."}
		names := []string{"POST /api/levels/run", "GET /api/levels"}
		section := &pageSection{ID: "backend", Map: &pageMap{Operations: true}, Triggers: []pageGroup{{}}}
		for i, name := range names {
			source := pageAnchor{Text: fmt.Sprintf("backend/app/app.py:%d", i+20), Href: fmt.Sprintf("/source/app#L%d", i+20)}
			section.Map.Nodes = append(section.Map.Nodes, pageMapNode{ID: fmt.Sprintf("operation-%d", i), FullTitle: name, Summary: purposes[i], Activation: "request", SourceKind: "fact", Source: source, Href: source.Href})
			section.Triggers[0].Operations = append(section.Triggers[0].Operations, pageGroupOperation{Name: name, Kind: "request", Summary: purposes[i], Source: "fact", Anchor: source, Href: fmt.Sprintf("#operation-%d", i)})
		}
		beforeNodes := append([]pageMapNode(nil), section.Map.Nodes...)
		beforeOperations := append([]pageGroupOperation(nil), section.Triggers[0].Operations...)
		page := &PreparedPage{view: &pageView{Sections: []*pageSection{section}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
		if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
			t.Fatal(err)
		}
		page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
		catalog := page.TextCatalog()
		if len(catalog.Entries) != len(purposes) {
			t.Fatalf("expected two shared purposes and no extracted route labels: %#v", catalog.Entries)
		}
		translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: catalog.SHA256}
		for i, entry := range catalog.Entries {
			if entry.Text != purposes[i] {
				t.Fatalf("purpose or source route changed: %#v", entry)
			}
			translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated[i]})
		}
		if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
			t.Fatal(err)
		}
		for i, node := range section.Map.Nodes {
			operation := section.Triggers[0].Operations[i]
			if node.Summary != translated[i] || operation.Summary != translated[i] {
				t.Fatalf("operation map/card retained an English purpose: %#v / %#v", node, operation)
			}
			before := beforeNodes[i]
			if node.ID != before.ID || node.FullTitle != before.FullTitle || node.CanonicalTitle != before.FullTitle || node.Href != before.Href || node.Source != before.Source || node.SourceKind != before.SourceKind || node.Activation != before.Activation {
				t.Fatal("translating a boundary purpose changed map identity or exact source")
			}
			beforeOperation := beforeOperations[i]
			beforeOperation.Summary = translated[i]
			beforeOperation.SummaryRef = catalog.Entries[i].Ref
			if !reflect.DeepEqual(operation, beforeOperation) {
				t.Fatal("translating a boundary purpose changed its route label or source anchor")
			}
		}
		parsed, err := template.New("map").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/map.html")
		if err != nil {
			t.Fatal(err)
		}
		var rendered bytes.Buffer
		if err := parsed.ExecuteTemplate(&rendered, "map.html", section.Map); err != nil {
			t.Fatal(err)
		}
		for i, purpose := range purposes {
			if strings.Contains(rendered.String(), purpose) || !strings.Contains(rendered.String(), `data-summary="`+translated[i]+`"`) {
				t.Fatalf("operation inspector/menu data was not localized: %s", rendered.String())
			}
		}
	})

	t.Run("localized display keeps English authority and source text", func(t *testing.T) {
		data := reportProgramShellDataFixture(t, "fixture")
		sourcePath := data.OpenablePaths[0]
		const quote = "The author says this exact sentence."
		const signature = "func sample(value string) int"
		data.Claims = &claims.Result{Claims: []claims.Claim{{ID: "claim-source", Source: claims.SourceReadme, Path: sourcePath, Line: 1, Text: quote}}}
		data.ReadmeOverview = "This unused README overview is retained only in canonical data."
		data.Timing = &RunTiming{WallMS: 372_000, Stages: []StageTiming{{Stage: "atlas_answer", Live: 2, Cached: 1, ProviderMS: 61_000, SlowestMS: 32_000}}}
		step := atlas.QuestionStep{Path: sourcePath, Line: 1, StopIndexes: []int{0}}
		data.Questions = []atlas.QuestionRoute{{Version: atlas.QuestionRouteVersion, Revision: data.CapturedRevision, Question: "My original question?", UserQuestion: true,
			Stops:  []atlas.QuestionStop{{Path: sourcePath, Line: 1, Name: "sample", Why: "This declaration prepares a response.", Evidence: map[string]any{"evidence": []map[string]any{{"anchor_path": sourcePath, "anchor_line": 1, "signature": signature, "author_text": quote}}}}},
			Answer: &atlas.QuestionAnswer{State: "answered", Parts: []atlas.QuestionAnswerPart{{State: "answered", Text: "It calls `sample(value)` before returning a response.", Basis: "The declaration names its argument.", Source: atlas.SourceModel, Steps: []atlas.QuestionStep{step}}}},
		}}
		canonicalBefore, err := encodeReportJSON(&data, 0)
		if err != nil {
			t.Fatal(err)
		}
		options := reportSingleTargetRenderOptionsFixture(t, &data)
		prepared, err := PreparePage(&data, options)
		if err != nil {
			t.Fatal(err)
		}
		catalog := prepared.TextCatalog()
		if err := catalog.Validate(); err != nil {
			t.Fatal(err)
		}
		if len(catalog.Entries) == 0 {
			t.Fatal("model display catalogue is empty")
		}
		serialized, err := json.Marshal(catalog)
		if err != nil {
			t.Fatal(err)
		}
		for _, original := range []string{quote, signature, "My original question?", data.ReadmeOverview} {
			if bytes.Contains(serialized, []byte(original)) {
				t.Fatalf("source or user wording entered translator: %s", original)
			}
		}
		protected := false
		for _, entry := range catalog.Entries {
			for _, part := range entry.Protected {
				if part.Text == "`sample(value)`" {
					protected = true
				}
			}
		}
		if !protected {
			t.Fatal("inline code was not protected")
		}
		questionID := prepared.view.Questions[0].ID
		translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: catalog.SHA256, Entries: []DisplayTranslationEntry{}}
		for i, entry := range catalog.Entries {
			translated := fmt.Sprintf("Перевод %d", i+1)
			for _, part := range entry.Protected {
				translated += " " + part.Ref
			}
			translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated})
		}
		options.Language, options.Translations = Russian, &translations
		rendered, err := RenderHTMLWithOptions(&data, options)
		if err != nil {
			t.Fatal(err)
		}
		decoded := stdhtml.UnescapeString(string(rendered))
		for _, want := range []string{`<html lang="ru">`, "Перевод", quote, signature, "`sample(value)`", sourcePath, questionID, "My original question?", "Общее время запуска: 6 мин 12 с.", "Ответы на вопросы: запросов к модели — 2"} {
			if !strings.Contains(decoded, want) {
				t.Fatalf("localized report lost %q", want)
			}
		}
		canonicalAfter, err := encodeReportJSON(&data, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonicalBefore, canonicalAfter) {
			t.Fatal("display translation changed canonical report data")
		}

		standalone := data
		standalone.GitHubSourceLinks, err = newGitHubSourceLinks("https://github.com/example/fixture", data.CapturedRevision, "")
		if err != nil {
			t.Fatal(err)
		}
		staticPage, err := PreparePage(&standalone, options)
		if err != nil {
			t.Fatal(err)
		}
		served := data
		served.SourceIDs = make(map[string]string)
		for i, path := range data.OpenablePaths {
			served.SourceIDs[path] = fmt.Sprintf("%043d", i)
		}
		serverOptions := options
		serverOptions.LocalRoots = []string{"/tmp/report-run", "/tmp/repository"}
		serverOptions.ReportSHA256 = strings.Repeat("f", 64)
		servedPage, err := PreparePage(&served, serverOptions)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(catalog, staticPage.TextCatalog()) || !reflect.DeepEqual(catalog, servedPage.TextCatalog()) {
			t.Fatal("source transport or report stamp changed translation catalogue")
		}

		bad := translations
		bad.Entries = append([]DisplayTranslationEntry(nil), translations.Entries...)
		bad.Entries[0].Text += " __REPOMAP_P999999__"
		if err := bad.Validate(catalog); err == nil {
			t.Fatal("translation invented a protected source placeholder")
		}
		bad = translations
		bad.Entries = bad.Entries[:len(bad.Entries)-1]
		if err := bad.Validate(catalog); err == nil {
			t.Fatal("incomplete translation accepted")
		}
		for _, entry := range catalog.Entries {
			if len(entry.Protected) > 0 {
				if err := entry.ValidateTranslation("Перевод без исходного кода"); err == nil {
					t.Fatal("translation removed a source placeholder")
				}
				break
			}
		}

		noModel := RenderOptions{Language: Russian, NoModel: true}
		fallbackPage, err := PreparePage(&data, noModel)
		if err != nil {
			t.Fatal(err)
		}
		if len(fallbackPage.TextCatalog().Entries) != 0 {
			t.Fatal("no-model display requested model translations")
		}
		if _, err := RenderHTMLWithOptions(&data, noModel); err != nil {
			t.Fatalf("no-model Russian UI requires a translator: %v", err)
		}
	})
}

func TestRunMetadataCannotActivateRemovedReportLanguage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"repo_name":"fixture","effective_options":{"report_language":"ru"}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	data := reportProgramShellDataFixture(t, "fixture")
	if err := parseRunMetadata(path, &data); err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<html lang="en">`) ||
		strings.Contains(string(html), `"report_language"`) {
		t.Fatalf("metadata activated removed report language: %s", html[:min(len(html), 500)])
	}
}
