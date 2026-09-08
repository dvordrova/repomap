package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestDeclarationAliasKeepsNativeCodeAndOneGlossaryDefinition(t *testing.T) {
	const native = "개별종목_시세_추이"
	const alias = "Stock Price History"
	location := programindex.Location{Path: "items/core.py", Line: 12, Column: 1}
	builder := pageBuilder{subjects: map[string]subjectRef{
		"subject": {subject: groupindex.Subject{Object: &groupindex.ObjectFacts{Name: native, Kind: programindex.ObjectType, Location: &location},
			Interpretation: &groupindex.Interpretation{Key: true, Alias: alias, Line: "Reads historical prices for one stock."}}},
	}}
	var concepts []pageMapConcept
	if err := json.Unmarshal([]byte(builder.groupConcepts(groupindex.Group{MemberSubjectIDs: []string{"subject"}})), &concepts); err != nil {
		t.Fatal(err)
	}
	if len(concepts) != 1 || concepts[0].Name != native || concepts[0].Alias != alias || concepts[0].Source.Path != location.Path || concepts[0].Source.Line != location.Line {
		t.Fatalf("alias changed source identity: %+v", concepts)
	}
	chips, _ := builder.memberChips([]string{"subject"})
	if len(chips) != 1 || chips[0].Members[0].Name != native || chips[0].Members[0].Alias != alias {
		t.Fatal("key code lost its original name or accepted alias")
	}
	view := &pageView{LearnConcepts: []pageLearnConcept{{pageMapConcept: concepts[0], ID: "concept"}}}
	collectGlossary(view)
	if len(view.Glossary) != 1 || view.Glossary[0].Name != alias || view.Glossary[0].OriginalName != native {
		t.Fatalf("alias created another definition: %+v", view.Glossary)
	}
	page := &PreparedPage{view: view, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	entry := page.prepareTerminology("answer", alias+" wraps "+native+".", "", nil, nil)
	entry.Ref = "t1"
	if err := entry.validateTermBindings(); err != nil {
		t.Fatal(err)
	}
	plain, spans, err := entry.finishDisplayText(native + " — это " + alias + ".")
	if err != nil || !reflect.DeepEqual(matchedWords(plain, spans), []string{native, alias}) {
		t.Fatalf("both names must find the same definition: %q %+v %v", plain, spans, err)
	}
	for _, span := range spans {
		if !reflect.DeepEqual(span.IDs, []string{"concept"}) {
			t.Fatalf("an alias changed the glossary identity: %+v", span)
		}
	}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range page.catalog.Entries {
		if entry.Text == alias || entry.Text == native {
			t.Fatal("an English alias or native code name became a translation slot")
		}
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	translation := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256}
	for _, entry := range page.catalog.Entries {
		translation.Entries = append(translation.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: "Читает историю цен одной акции."})
	}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translation}); err != nil {
		t.Fatal(err)
	}
	if view.Glossary[0].Name != alias || view.Glossary[0].OriginalName != native {
		t.Fatal("changing report language renamed the code or its alias")
	}
}

func TestSameDirectoryComponentsKeepDistinctNamesAfterLocalization(t *testing.T) {
	sections := []*pageSection{
		{ID: "core", Name: "pykrx.website.krx.etx.core", Kind: "executable", Root: "pykrx/website/krx/etx"},
		{ID: "ticker", Name: "pykrx.website.krx.etx.ticker", Kind: "executable", Root: "pykrx/website/krx/etx"},
		{ID: "wrap", Name: "pykrx.website.krx.etx.wrap", Kind: "executable", Root: "pykrx/website/krx/etx"},
	}
	labelSections(sections)
	page := &PreparedPage{view: &pageView{Sections: sections}}
	page.rebuildDisplayLabels(Russian)
	seen := map[string]bool{}
	for _, section := range sections {
		if seen[section.ShortLabel] || !strings.Contains(section.ShortLabel, section.Name) || strings.Contains(section.ShortLabel, "executable") {
			t.Fatalf("component identities collapsed during translation: %+v", sections)
		}
		seen[section.ShortLabel] = true
	}
}

func TestEqualAliasesDoNotMergeDifferentDeclarations(t *testing.T) {
	view := &pageView{Glossary: []pageGlossaryTerm{
		{ID: "daily", Name: "Price History", OriginalName: "일별가격", Explanation: "Daily prices."},
		{ID: "monthly", Name: "Price History", OriginalName: "월별가격", Explanation: "Monthly prices."},
	}}
	page := &PreparedPage{view: view}
	entry := page.prepareTerminology("answer", "Price History", "", nil, nil)
	if err := entry.validateTermBindings(); err != nil {
		t.Fatal(err)
	}
	_, spans, err := entry.finishDisplayText("Price History")
	if err != nil || len(spans) != 1 || !reflect.DeepEqual(spans[0].IDs, []string{"daily", "monthly"}) {
		t.Fatalf("equal labels collapsed distinct definitions: %+v %v", spans, err)
	}
	invalid := DisplayTextEntry{Terms: []DisplayTextTerm{
		{ID: "one", Spelling: "Original", Explanation: "First meaning."},
		{ID: "one", Spelling: "Alias", Explanation: "Different meaning."},
	}}
	if invalid.validateTermBindings() == nil {
		t.Fatal("one alias changed the definition of its original")
	}
}
