package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func reorderedDisplayCatalog(entries []DisplayTextEntry) DisplayTextCatalog {
	catalog := DisplayTextCatalog{Version: DisplayTextVersion, Entries: append([]DisplayTextEntry{}, entries...)}
	for i := range catalog.Entries {
		catalog.Entries[i].Ref = fmt.Sprintf("t%d", i+1)
		catalog.Entries[i].Protected = append([]DisplayProtectedText(nil), catalog.Entries[i].Protected...)
	}
	catalog.SHA256 = displayCatalogDigest(catalog.Entries)
	return catalog
}

func TestDisplayTranslationsReuseExactEntriesInAnotherOrder(t *testing.T) {
	saved := reorderedDisplayCatalog([]DisplayTextEntry{
		{Role: "summary", Text: "Reads __REPOMAP_P1__.", Protected: []DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "`config.json`"}}},
		{Role: "label", Text: "Storage"},
	})
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: saved.SHA256, Entries: []DisplayTranslationEntry{
		{Ref: "t1", Text: "Читает __REPOMAP_P1__."}, {Ref: "t2", Text: "Хранилище"},
	}}
	current := reorderedDisplayCatalog([]DisplayTextEntry{saved.Entries[1], saved.Entries[0]})
	before, _ := json.Marshal([]any{saved, current, translations})
	got, err := rebindDisplayTranslations(saved, current, translations)
	if err != nil {
		t.Fatal(err)
	}
	want := []DisplayTranslationEntry{{Ref: "t1", Text: "Хранилище"}, {Ref: "t2", Text: "Читает __REPOMAP_P1__."}}
	if got.CatalogSHA256 != current.SHA256 || !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("translation moved to another text: %+v", got)
	}
	if err := got.Validate(current); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{saved, current, translations})
	if string(before) != string(after) {
		t.Fatal("reuse mutated its saved or current inputs")
	}
	for name, change := range map[string]func(*DisplayTextCatalog){
		"changed prose":            func(c *DisplayTextCatalog) { c.Entries[0].Text = "Different meaning" },
		"changed role":             func(c *DisplayTextCatalog) { c.Entries[0].Role = "reason" },
		"changed protected source": func(c *DisplayTextCatalog) { c.Entries[1].Protected[0].Text = "`secrets.json`" },
		"changed protected ref":    func(c *DisplayTextCatalog) { c.Entries[1].Protected[0].Ref = "__REPOMAP_P2__" },
		"new entry": func(c *DisplayTextCatalog) {
			c.Entries = append(c.Entries, DisplayTextEntry{Role: "label", Text: "Extra"})
		},
		"dropped entry":             func(c *DisplayTextCatalog) { c.Entries = c.Entries[:1] },
		"duplicate replacing entry": func(c *DisplayTextCatalog) { c.Entries[1] = c.Entries[0] },
	} {
		t.Run(name, func(t *testing.T) {
			changed := reorderedDisplayCatalog(current.Entries)
			change(&changed)
			changed = reorderedDisplayCatalog(changed.Entries)
			if _, err := rebindDisplayTranslations(saved, changed, translations); err == nil {
				t.Fatal("accepted more than a permutation of the complete saved catalogue")
			}
		})
	}
	invalid := translations
	invalid.CatalogSHA256 = current.SHA256
	if _, err := rebindDisplayTranslations(saved, current, invalid); err == nil {
		t.Fatal("unbound saved translation was reused")
	}
}

func TestIncomingComponentNameReusesOwnerUIComposition(t *testing.T) {
	owner := &pageSection{ID: "tests", programTargetID: "tests", Name: "test.example", Root: "test", ShortLabel: "test (executable)", Kind: "executable"}
	section := &pageSection{ID: "service", programTargetID: "service", Name: "service", ShortLabel: "service", Map: &pageMap{Nodes: []pageMapNode{
		{ID: "foreign-tests", Branch: "component", Component: "tests", FullTitle: owner.ShortLabel},
		{ID: "request-part", FullTitle: "Request handling", Summary: "Handles requests."},
	}}}
	page := &PreparedPage{view: &pageView{Sections: []*pageSection{section, owner}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	if len(page.catalog.Entries) != 2 || page.catalog.Entries[0].Text != "Request handling" || page.catalog.Entries[1].Text != "Handles requests." {
		t.Fatalf("native component name acquired a new model translation: %+v", page.catalog.Entries)
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256, Entries: []DisplayTranslationEntry{
		{Ref: "t1", Text: "Обработка запросов"}, {Ref: "t2", Text: "Обрабатывает запросы."},
	}}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
		t.Fatal(err)
	}
	if got := section.Map.Nodes[0].FullTitle; got != owner.ShortLabel || got != "test (исполняемый компонент)" {
		t.Fatalf("component container lost its owner's composed name: %q, owner %q", got, owner.ShortLabel)
	}
	if section.Map.Nodes[1].FullTitle != "Обработка запросов" || section.Map.Nodes[1].Summary != "Обрабатывает запросы." {
		t.Fatal("existing model descriptions no longer use their translation")
	}
}
