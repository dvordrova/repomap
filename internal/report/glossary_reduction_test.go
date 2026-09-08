package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

// This provider merges compatible domain names. Native declarations never
// become choices in this request.
type glossaryReductionFixtureProvider struct{ requests [][]byte }

func (*glossaryReductionFixtureProvider) State() []byte {
	return []byte(`{"provider":"glossary-reduction-fixture"}`)
}

func (*glossaryReductionFixtureProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (provider *glossaryReductionFixtureProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.requests = append(provider.requests, prepared.Bytes())
	var input struct {
		Groups []struct {
			Ref      string `json:"ref"`
			Variants []struct {
				Ref string `json:"ref"`
				terminology.Candidate
			} `json:"variants"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &input); err != nil {
		return llm.Completion{}, err
	}
	type row struct {
		Members        []string `json:"members"`
		Representative string   `json:"representative"`
	}
	rows := make(map[string]*row)
	for _, group := range input.Groups {
		if len(group.Variants) != 1 {
			return llm.Completion{}, fmt.Errorf("native identity or unexpected fixture group entered meaning reduction")
		}
		variant := group.Variants[0]
		if rows[variant.Name] == nil {
			rows[variant.Name] = &row{Representative: variant.Ref}
		}
		rows[variant.Name].Members = append(rows[variant.Name].Members, group.Ref)
	}
	var names []string
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)
	output := struct {
		Groups []*row `json:"groups"`
	}{}
	for _, name := range names {
		output.Groups = append(output.Groups, rows[name])
	}
	raw, err := json.Marshal(output)
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

func TestReducedGlossaryAddsDomainDefinitionsBesideUnchangedNativeOwners(t *testing.T) {
	// Field/proxy explanations and policy disagreement reproduce the ordinary
	// python-tutorial-game reduction failure from 2026-09-08. Robot adds the
	// exact-owner case even when names and source lines coincide.
	native := []pageGlossaryTerm{
		{ID: "concept-field", Name: "Field", OriginalName: "Field", Code: true,
			Explanation: "Represents the game board and its state, including robots, awards, walls, and step counters.",
			Sources:     []pageAnchor{{Path: "backend/app/field.py", Line: 10, Text: "backend/app/field.py:10:1", Open: "backend/app/field.py:10:1"}}, Places: []pageLearnLink{{Title: "Simulation", Href: "#simulation"}}},
		{ID: "concept-robot-left", Name: "Robot", OriginalName: "Robot", Code: true,
			Explanation: "The simulation's moving robot.", Sources: []pageAnchor{{Path: "types.ts", Line: 4, Text: "types.ts:4:7", Open: "types.ts:4:7"}}, Places: []pageLearnLink{{Title: "Left", Href: "#left"}}},
		{ID: "concept-robot-right", Name: "Robot", OriginalName: "Robot", Code: true,
			Explanation: "The robot's transport representation.", Sources: []pageAnchor{{Path: "types.ts", Line: 4, Text: "types.ts:4:41", Open: "types.ts:4:41"}}, Places: []pageLearnLink{{Title: "Right", Href: "#right"}}},
	}
	domains := []terminology.Candidate{
		{Name: "Field", Explanation: "Backend game-field/simulation object that holds robots, walls, prepared user code and step counters while a level runs.", Sources: []terminology.Source{{Path: "backend/app/field.py", Line: 10}, {Path: "backend/app/field.py", Line: 23}}},
		{Name: "Field", Explanation: "Backend simulation model that holds run state, step counter and user code and advances or ends a run.", Sources: []terminology.Source{{Path: "backend/app/field.py", Line: 10}}},
		{Name: "proxy", Explanation: "A development server configuration in front/package.json that forwards requests from the frontend dev server to the backend at http://localhost:8080.", Sources: []terminology.Source{{Path: "front/package.json", Line: 52}}},
		{Name: "proxy", Explanation: "A development server setting in front/package.json that forwards API requests to the backend at http://localhost:8080.", Sources: []terminology.Source{{Path: "front/package.json", Line: 0}, {Path: "front/package.json", Line: 52}}},
		{Name: "proxy", Explanation: "A frontend development setting that forwards API requests to the backend at http://localhost:8080, avoiding cross-origin issues.", Sources: []terminology.Source{{Path: "front/package.json", Line: 0}}},
	}
	for i := range domains {
		domains[i].Origins = []terminology.Origin{{RequestSHA256: strings.Repeat(fmt.Sprint(i+1), 64), Row: "r1"}}
	}
	candidates := domains
	provider := &glossaryReductionFixtureProvider{}
	catalog, err := terminology.Reduce(t.Context(), llm.Executor{}, provider, candidates)
	if err != nil || len(catalog.Entries) != 2 || len(provider.requests) != 1 || catalog.Validate() != nil {
		t.Fatalf("ordinary glossary reduction failed: %+v, %v", catalog, err)
	}
	for _, original := range native {
		if bytes.Contains(provider.requests[0], []byte(original.Explanation)) || bytes.Contains(provider.requests[0], []byte(original.ID)) {
			t.Fatal("known native identity was delegated to domain grouping")
		}
	}
	for _, candidate := range candidates {
		count := 0
		for _, entry := range catalog.Entries {
			for _, variant := range entry.Variants {
				if reflect.DeepEqual(variant, candidate) {
					count++
				}
			}
		}
		if count != 1 {
			t.Fatalf("original candidate/provenance retained %d times: %+v", count, candidate)
		}
	}
	view := &pageView{Glossary: append([]pageGlossaryTerm(nil), native...), Questions: []*pageQuestion{{ID: "q-proxy", Question: "How does the proxy work?", Answers: []pageAnswerPart{{RequestSHA256: domains[2].Origins[0].RequestSHA256, OriginRow: "r1"}}}}}
	builder := &pageBuilder{data: &ReportData{Glossary: &catalog}, links: pageLinks{repositoryURL: "https://example.test/repo", blobPrefix: "/blob/", revision: "revision"}}
	if err := builder.reducedGlossary(view); err != nil {
		t.Fatal("valid reduction failed final Report.Glossary projection", err)
	}
	if len(view.Glossary) != 5 {
		t.Fatalf("projected glossary dropped entries: %+v", view.Glossary)
	}
	for _, original := range native {
		at := slices.IndexFunc(view.Glossary, func(term pageGlossaryTerm) bool { return term.ID == original.ID })
		if at < 0 || !reflect.DeepEqual(view.Glossary[at], original) {
			t.Fatal("final report changed a native owner, explanation or whole source anchor")
		}
	}
	proxy := view.Glossary[slices.IndexFunc(view.Glossary, func(term pageGlossaryTerm) bool { return term.Name == "proxy" })]
	if len(proxy.Sources) != 2 || len(proxy.Questions) != 1 || proxy.Questions[0].Href != "#q-proxy" {
		t.Fatalf("union sources or original question binding lost: %+v", proxy)
	}
	templates, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) { return uiText(English, key, args...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
		t.Fatal(err)
	}
	for _, term := range view.Glossary {
		if strings.Count(html.String(), `id="`+term.ID+`"`) != 1 {
			t.Fatal("published glossary lost a separately addressable entry")
		}
	}
}
