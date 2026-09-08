package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func glossaryFixture(id, name, explanation, question string) pageGlossaryTerm {
	term := pageGlossaryTerm{ID: id, Name: name, OriginalName: name, Explanation: explanation}
	if question != "" {
		term.Questions = []pageLearnLink{{Href: "#" + question}}
	}
	return term
}

func matchedWords(text string, spans []displayTermSpan) []string {
	units := utf16.Encode([]rune(text))
	var result []string
	for _, span := range spans {
		result = append(result, string(utf16.Decode(units[span.Start:span.End])))
	}
	return result
}

func TestLiteralTermLookupFollowsFinalWordsAfterReordering(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{
		glossaryFixture("simulation-id", "simulation", "The simulation being run.", "q"),
		glossaryFixture("field-id", "Field", "The simulation field object.", "q"),
		glossaryFixture("ticker-id", "ticker", "An instrument code.", "q"),
		glossaryFixture("frame-id", "DataFrame", "A table of observations.", "q"),
	}}}
	for _, test := range []struct {
		original, translated string
		words, ids           []string
	}{
		{"The simulation Field object.", "😀 Объект Field для simulation.", []string{"Field", "simulation"}, []string{"field-id", "simulation-id"}},
		{"A ticker-indexed DataFrame.", "DataFrame с индексом по ticker.", []string{"DataFrame", "ticker"}, []string{"frame-id", "ticker-id"}},
	} {
		entry := page.prepareTerminology("answer", test.original, "q", nil, nil)
		entry.Ref = "t1"
		if err := entry.validateTermBindings(); err != nil {
			t.Fatal(err)
		}
		if entry.Text != test.original || strings.Contains(entry.Text, "__REPOMAP_M") {
			t.Fatal("glossary added model-facing occurrence bookkeeping")
		}
		if err := entry.ValidateTranslation(test.translated); err != nil {
			t.Fatal(err)
		}
		plain, spans, err := entry.finishDisplayText(test.translated)
		if err != nil || plain != test.translated || !reflect.DeepEqual(matchedWords(plain, spans), test.words) {
			t.Fatalf("final words changed: %q %+v %v", plain, spans, err)
		}
		for i, span := range spans {
			if !reflect.DeepEqual(span.IDs, []string{test.ids[i]}) {
				t.Fatalf("hint followed source word order: %+v", spans)
			}
		}
	}
}

func TestLiteralTermBoundariesOverlapsAndSourceSyntax(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{
		glossaryFixture("hmm", "HMM", "Hidden Markov model.", ""),
		glossaryFixture("custom", "custom dictionary", "A supplied vocabulary.", ""),
		glossaryFixture("dictionary", "dictionary", "A vocabulary.", ""),
	}}}
	original := "😀 HMM, HMMish XHMM _HMM HMM2 HMM\u0301; HMM과 custom dictionary, dictionary; `HMM` /api/HMM https://x.test/HMM model.HMM HMM(). HMM."
	entry := page.prepareTerminology("answer", original, "", nil, nil)
	plain, spans, err := entry.finishDisplayText(entry.Text)
	want := []string{"HMM", "HMM", "custom dictionary", "dictionary", "HMM"}
	if err != nil || plain != original || !reflect.DeepEqual(matchedWords(plain, spans), want) {
		t.Fatalf("boundaries/source exclusion: %q %+v %v", plain, matchedWords(plain, spans), err)
	}
	for _, span := range spans {
		if len(span.IDs) != 1 {
			t.Fatal("overlapping dictionary created nested definitions")
		}
	}
}

func TestTermScopeKeepsHomonymsSeparateWithoutInferringAnOccurrenceSense(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{
		glossaryFixture("finance", "bank", "A financial institution.", "q-finance"),
		glossaryFixture("river", "bank", "The land beside a river.", "q-river"),
	}}}
	for _, test := range []struct {
		scope string
		ids   []string
	}{{"q-finance", []string{"finance"}}, {"q-river", []string{"river"}}, {"", []string{"finance", "river"}}, {"q-later", []string{"finance", "river"}}} {
		entry := page.prepareTerminology("answer", "A bank beside another bank.", test.scope, nil, nil)
		plain, spans, err := entry.finishDisplayText("bank рядом с другим bank.")
		if err != nil || len(spans) != 2 {
			t.Fatalf("repeated names lost: %q %+v", plain, spans)
		}
		for _, span := range spans {
			if !reflect.DeepEqual(span.IDs, test.ids) {
				t.Fatalf("scope %q lost alternatives: %+v", test.scope, spans)
			}
		}
	}
	own := page.view.Glossary[1]
	entry := page.prepareTerminology("term-explanation", "A bank.", "", nil, &own)
	_, spans, _ := entry.finishDisplayText(entry.Text)
	if len(spans) != 1 || !reflect.DeepEqual(spans[0].IDs, []string{"river"}) {
		t.Fatal("the glossary entry lost its own definition")
	}
}

func TestPreparedGlossaryKeepsNamesAndTranslatesDefinitions(t *testing.T) {
	makePage := func() *PreparedPage {
		return &PreparedPage{view: &pageView{
			Glossary:  []pageGlossaryTerm{glossaryFixture("dictionary", "custom dictionary", "A user-supplied vocabulary.", "q")},
			Questions: []*pageQuestion{{ID: "q", Question: "How is the custom dictionary used?", Answers: []pageAnswerPart{{Text: "The custom dictionary is loaded."}}}},
		}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	}
	page := makePage()
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256}
	for _, entry := range page.catalog.Entries {
		var text string
		switch entry.Role {
		case "question":
			text = "Как используется custom dictionary?"
		case "answer":
			text = "Загружается custom dictionary."
		case "term-explanation":
			text = "Словарь, добавленный пользователем."
		default:
			t.Fatalf("unexpected translation slot %s", entry.Role)
		}
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: text})
	}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
		t.Fatal(err)
	}
	term := page.view.Glossary[0]
	if term.Name != "custom dictionary" || term.OriginalName != term.Name || term.Explanation != "Словарь, добавленный пользователем." {
		t.Fatalf("dictionary name or definition: %+v", term)
	}
	var plans []displayTermPlan
	if err := json.Unmarshal([]byte(page.view.TermMentionsJSON), &plans); err != nil || len(plans) != 2 {
		t.Fatalf("lookup plans: %s %v", page.view.TermMentionsJSON, err)
	}
	for _, plan := range plans {
		if plan.Ref == "" || !reflect.DeepEqual(matchedWords(plan.Text, plan.Spans), []string{"custom dictionary"}) {
			t.Fatalf("bad final lookup %+v", plan)
		}
	}
	english := makePage()
	if err := english.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	if err := english.applyDisplay(RenderOptions{Language: English}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(english.view.TermMentionsJSON), &plans); err != nil || len(plans) != 2 {
		t.Fatal("English lookup now also works without a model call")
	}
}

func TestSavedTranslationsRequireExactTermIdentity(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{glossaryFixture("finance", "bank", "A financial institution.", "q")}}}
	entry := page.prepareTerminology("answer", "A bank.", "q", nil, nil)
	saved := reorderedDisplayCatalog([]DisplayTextEntry{entry})
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: saved.SHA256, Entries: []DisplayTranslationEntry{{Ref: "t1", Text: "Это bank."}}}
	if _, err := rebindDisplayTranslations(saved, saved, translations); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*DisplayTextEntry){
		"definition": func(e *DisplayTextEntry) { e.Terms[0].Explanation = "The land beside a river." },
		"owner":      func(e *DisplayTextEntry) { e.Terms[0].ID = "river" },
		"scope":      func(e *DisplayTextEntry) { e.Scope = "q-other" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := entry
			copy.Terms = append([]DisplayTextTerm(nil), entry.Terms...)
			change(&copy)
			if _, err := rebindDisplayTranslations(saved, reorderedDisplayCatalog([]DisplayTextEntry{copy}), translations); err == nil {
				t.Fatal("rebound translation to a different context")
			}
		})
	}
}

func TestLiteralNamesDoNotAddSemanticRepairOrModelDecisions(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{glossaryFixture("main", "main", "The executable.", "")}}}
	entry := page.prepareTerminology("answer", "The main executable handles the main flow.", "", nil, nil)
	entry.Ref = "t1"
	if entry.Text != "The main executable handles the main flow." {
		t.Fatal("a dictionary lookup introduced annotation markup")
	}
	// A dictionary lookup deliberately does not classify the sense of each use.
	_, spans, _ := entry.finishDisplayText("main обрабатывает main flow.")
	if len(spans) != 2 {
		t.Fatal("literal lookup depends on a model classification")
	}
	// If translation changes a term despite the instruction, keep the actual
	// prose. No guessed alias, lemmatization or invented occurrence repairs it.
	changed := "Основной поток."
	if err := entry.ValidateTranslation(changed); err != nil {
		t.Fatal(err)
	}
	plain, spans, err := entry.finishDisplayText(changed)
	if err != nil || plain != changed || len(spans) != 0 {
		t.Fatal("translated prose was repaired to fit a dictionary")
	}
}

func TestBareAPIPathsStayLiteralBesideAnUntranslatedTerm(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{glossaryFixture("level", "level", "A game stage.", "")}}}
	entry := page.prepareTerminology("answer", "A level uses GET /api/level/{level_id}, then POST /api/level/run.", "", nil, nil)
	if len(entry.Protected) != 2 || entry.Protected[0].Text != "/api/level/{level_id}" || entry.Protected[1].Text != "/api/level/run" {
		t.Fatalf("path protection changed: %+v", entry)
	}
	translated := "Для level используется GET __REPOMAP_P1__, затем POST __REPOMAP_P2__."
	if err := entry.ValidateTranslation(translated); err != nil {
		t.Fatal(err)
	}
	plain, spans, err := entry.finishDisplayText(translated)
	if err != nil || plain != "Для level используется GET /api/level/{level_id}, затем POST /api/level/run." || !reflect.DeepEqual(matchedWords(plain, spans), []string{"level"}) {
		t.Fatalf("API path became a term occurrence: %q %+v %v", plain, spans, err)
	}
	if err := entry.ValidateTranslation(strings.ReplaceAll(translated, "__REPOMAP_P1__", "/api/уровень/{level_id}")); err == nil {
		t.Fatal("changed source path accepted")
	}
}

func TestSourcePlaceholdersRestoreOnceAndExactNativeNameRemainsLookable(t *testing.T) {
	page := &PreparedPage{view: &pageView{Glossary: []pageGlossaryTerm{glossaryFixture("frame", "DataFrame", "A table.", "")}}}
	original := "DataFrame follows __REPOMAP_P7__ and `__REPOMAP_P1__`."
	entry := page.prepareTerminology("answer", original, "", []string{"DataFrame"}, nil)
	if err := entry.ValidateTranslation(entry.Text); err != nil {
		t.Fatal(err)
	}
	plain, spans, err := entry.finishDisplayText(entry.Text)
	if err != nil || plain != original || !reflect.DeepEqual(matchedWords(plain, spans), []string{"DataFrame"}) {
		t.Fatalf("literal source was recursively parsed: %q %+v %v", plain, spans, err)
	}
}

func TestLiteralDisplaySlotsHaveStableLocalLookupRefs(t *testing.T) {
	page := &PreparedPage{view: &pageView{
		Summary:  &pageSentence{Text: "HMM"},
		Glossary: []pageGlossaryTerm{glossaryFixture("hmm", "HMM", "A segmentation model.", "")},
	}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, true); err != nil {
		t.Fatal(err)
	}
	if len(page.catalog.Entries) != 0 || page.view.Summary.TextRef == "" {
		t.Fatal("local literal lookup needs a translation request")
	}
	if err := page.applyDisplay(RenderOptions{Language: English}); err != nil {
		t.Fatal(err)
	}
	var plans []displayTermPlan
	if err := json.Unmarshal([]byte(page.view.TermMentionsJSON), &plans); err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Ref != page.view.Summary.TextRef {
		t.Fatalf("literal slot lost its lookup: %+v", plans)
	}
}
