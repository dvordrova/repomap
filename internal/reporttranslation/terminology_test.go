package reporttranslation

import (
	"encoding/json"
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
	var request []requestEntry
	if err := json.Unmarshal([]byte(wire), &request); err != nil {
		t.Fatal(err)
	}
	if request[0].Role != "answer" || request[1].Role != "term-explanation" || len(request[0].Terms) != 1 ||
		request[0].Terms[0].Explanation != entry.Terms[0].Explanation || request[0].Terms[0].Spelling != "custom dictionary" {
		t.Fatal("typed request omitted the original name or definition context")
	}
	if request[0].Text != "Load custom dictionary with __REPOMAP_P1__." {
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
	var request []requestEntry
	if err := json.Unmarshal([]byte(provider.requests[0].Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request[0].Terms) != 2 || request[0].Terms[0].Spelling != "bank" || request[0].Terms[1].Spelling != "bank" || request[0].Terms[0].Explanation == request[0].Terms[1].Explanation {
		t.Fatalf("equal names merged distinct definitions: %+v", request[0].Terms)
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
