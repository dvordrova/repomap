package reporttranslation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

func termEntry() report.DisplayTextEntry {
	return report.DisplayTextEntry{
		Role: "answer", Scope: "LOCAL-QUESTION-ID", Context: "LOCAL-CONTEXT-ID",
		Text:  "Load custom dictionary with __REPOMAP_P1__.",
		Terms: []report.DisplayTextTerm{{ID: "LOCAL-TERM-ID", Spelling: "custom dictionary", Explanation: "A user-supplied vocabulary."}},
		Protected: []report.DisplayProtectedText{
			{Ref: "__REPOMAP_P1__", Text: "SOURCE-CODE-NEVER-SENT"},
		},
	}
}

func TestTranslationKeepsTermNamesAndTranslatesDefinitionsInTheExistingWindow(t *testing.T) {
	entry := termEntry()
	catalog := testCatalog(t, []report.DisplayTextEntry{entry, {
		Role: "term-explanation", Text: entry.Terms[0].Explanation, Context: "LOCAL-TERM-ID", Terms: entry.Terms,
	}})
	provider := &testProvider{respond: func(request modelRequest) modelResponse {
		if len(request.Entries) != 2 {
			t.Fatal("dictionary context created a separate request")
		}
		return modelResponse{Translations: []responseEntry{
			{Ref: "t1", Text: "Через __REPOMAP_P1__ загружается custom dictionary."},
			{Ref: "t2", Text: "Словарь, добавленный пользователем."},
		}}
	}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	translated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || len(translated.Entries) != 2 || translated.Entries[0].Text != "Через __REPOMAP_P1__ загружается custom dictionary." || translated.Entries[1].Text != "Словарь, добавленный пользователем." {
		t.Fatalf("literal term names or full definitions changed: %+v", translated)
	}
	wire := provider.requests[0].Prompt.User
	for _, forbidden := range []string{"LOCAL-QUESTION-ID", "LOCAL-CONTEXT-ID", "LOCAL-TERM-ID", "SOURCE-CODE-NEVER-SENT", catalog.SHA256, `"mentions"`, "__REPOMAP_M", `"g1"`} {
		if strings.Contains(wire, forbidden) {
			t.Fatalf("local identity, source bytes or occurrence bookkeeping entered translation: %s", forbidden)
		}
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire), &request); err != nil {
		t.Fatal(err)
	}
	if request.Entries[0].Role != "answer" || request.Entries[1].Role != "term-explanation" || len(request.Terms) != 1 ||
		request.Terms[0].Explanation != entry.Terms[0].Explanation || request.Terms[0].Spelling != "custom dictionary" {
		t.Fatal("typed request omitted the original name or definition context")
	}
	assertRequestTerms(t, request, catalog.Entries)
	if request.Entries[0].Text != "Load custom dictionary with __REPOMAP_P1__." {
		t.Fatal("an ordinary term acquired placeholder bookkeeping")
	}
	if _, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil || len(provider.requests) != 1 {
		t.Fatal("exact request was not reused through the shared cache")
	}
	entry.Terms = append([]report.DisplayTextTerm(nil), entry.Terms...)
	entry.Terms[0].Explanation = "A replacement vocabulary for a particular domain."
	changed := testCatalog(t, []report.DisplayTextEntry{entry, catalog.Entries[1]})
	if _, err := Translate(t.Context(), executor, provider, changed, report.Russian); err != nil || len(provider.requests) != 2 {
		t.Fatal("changed definition context reused the previous prepared request")
	}
}

func TestTranslationRetainsHomonymDefinitionsWithoutRequestingAChoice(t *testing.T) {
	entry := report.DisplayTextEntry{
		Role: "answer", Text: "The bank is described.",
		Terms: []report.DisplayTextTerm{
			{ID: "bank-finance", Spelling: "bank", Explanation: "A financial institution."},
			{ID: "bank-land", Spelling: "bank", Explanation: "The land beside a river."},
		},
	}
	catalog := testCatalog(t, []report.DisplayTextEntry{entry})
	provider := &testProvider{rawResponse: []byte(`{"t1":{"text":"Описывается bank."}}`)}
	result, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil || len(result.Entries) != 1 || result.Entries[0].Text != "Описывается bank." {
		t.Fatalf("a plain translation required choosing a meaning: %+v, %v", result, err)
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(provider.requests[0].Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Terms) != 2 || request.Terms[0].Spelling != "bank" || request.Terms[1].Spelling != "bank" || request.Terms[0].Explanation == request.Terms[1].Explanation {
		t.Fatalf("equal names merged distinct definitions: %+v", request.Terms)
	}
	assertRequestTerms(t, request, catalog.Entries)
}

// Resolve the actual wire refs and compare the exact set of definitions for
// each original text, including homonyms. No sibling's context may leak in.
func assertRequestTerms(t *testing.T, request modelRequest, entries []report.DisplayTextEntry) {
	t.Helper()
	known := make(map[string]report.DisplayTextEntry)
	for _, entry := range entries {
		known[entry.Ref] = entry
	}
	definitions := make(map[string][2]string)
	pairs := make(map[[2]string]bool)
	for _, term := range request.Terms {
		pair := [2]string{term.Spelling, term.Explanation}
		if term.Ref == "" || pairs[pair] || definitions[term.Ref] != ([2]string{}) {
			t.Fatalf("duplicate or missing definition identity: %+v", term)
		}
		pairs[pair], definitions[term.Ref] = true, pair
	}
	used := make(map[string]bool)
	for _, entry := range request.Entries {
		original, exists := known[entry.Ref]
		if !exists || entry.Text != original.Text || entry.Role != original.Role {
			t.Fatalf("window changed an original text: %+v", entry)
		}
		want, got := make(map[[2]string]bool), make(map[[2]string]bool)
		for _, term := range original.Terms {
			want[[2]string{term.Spelling, term.Explanation}] = true
		}
		for _, ref := range entry.Terms {
			pair, exists := definitions[ref]
			if !exists || got[pair] {
				t.Fatalf("unknown or duplicate term ref in %s: %s", entry.Ref, ref)
			}
			got[pair], used[ref] = true, true
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s changed applicable definitions: got %v, want %v", entry.Ref, got, want)
		}
	}
	if len(used) != len(definitions) {
		t.Fatal("request carries definitions not used by its own entries")
	}
}

func TestTranslationSharesExactDefinitionsWithoutChangingEntryScopeOrLocalBinding(t *testing.T) {
	finance := report.DisplayTextTerm{ID: "local-finance", Spelling: "bank", Explanation: "A financial institution."}
	land := report.DisplayTextTerm{ID: "local-land", Spelling: "bank", Explanation: "The land beside a river."}
	duplicate := finance
	duplicate.ID = "another-local-finance"
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use the bank.", Terms: []report.DisplayTextTerm{finance, duplicate}},
		{Role: "answer", Text: "Walk along the bank.", Terms: []report.DisplayTextTerm{land}},
		{Role: "answer", Text: "The bank has two meanings.", Terms: []report.DisplayTextTerm{finance, land}},
		{Role: "label", Text: "Start here"},
	})
	provider := &testProvider{}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
	if err != nil || result.Validate(catalog) != nil || len(provider.requests) != 1 {
		t.Fatalf("shared context changed translation execution: %+v, %v", result, err)
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(provider.requests[0].Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Terms) != 2 || len(request.Entries[0].Terms) != 1 || len(request.Entries[2].Terms) != 2 || len(request.Entries[3].Terms) != 0 {
		t.Fatalf("definitions were not shared by exact pair: %+v", request)
	}
	assertRequestTerms(t, request, catalog.Entries)
	// Local term identity changes rebind the same exact provider answer; they
	// must not add context or alter an otherwise identical model request.
	for i := range catalog.Entries {
		for j := range catalog.Entries[i].Terms {
			catalog.Entries[i].Terms[j].ID += "-new-owner"
		}
	}
	rebound := testCatalog(t, catalog.Entries)
	cached, err := Translate(t.Context(), executor, provider, rebound, report.Russian)
	if err != nil || len(provider.requests) != 1 || cached.CatalogSHA256 != rebound.SHA256 || cached.CatalogSHA256 == result.CatalogSHA256 {
		t.Fatalf("local binding changed request identity: %+v, %v", cached, err)
	}
}

func TestTranslationDoesNotRepairAChangedOrdinaryTermSpelling(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{termEntry()})
	provider := &testProvider{rawResponse: []byte(`{"t1":{"text":"Загрузите пользовательский словарь через __REPOMAP_P1__."}}`)}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
	if err != nil || len(result.Entries) != 1 || result.Entries[0].Text != "Загрузите пользовательский словарь через __REPOMAP_P1__." {
		t.Fatalf("term spelling was repaired or rejected instead of leaving exact matching to the report: %+v, %v", result, err)
	}
	provider.rawResponse = nil
	if _, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil || len(provider.requests) != 1 {
		t.Fatalf("plain translated text lost its existing cache contract: %v", err)
	}
}
