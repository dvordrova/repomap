package report

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/terminology"
)

func pageGlossaryCatalog(t *testing.T, groups ...[]terminology.Candidate) *terminology.Catalog {
	t.Helper()
	catalog := &terminology.Catalog{Version: terminology.CatalogVersion}
	for _, variants := range groups {
		catalog.Entries = append(catalog.Entries, terminology.Entry{Explanation: variants[0].Explanation, Variants: variants})
	}
	if err := catalog.Seal(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestReducedGlossaryKeepsNativeOwnerAndWholeAnchor(t *testing.T) {
	left := glossaryFixture("concept-left", "Value", "The first declaration's value.", "q-left")
	left.Code = true
	left.Sources = []pageAnchor{{Path: "types.ts", Line: 4, Text: "types.ts:4", Open: "types.ts:4:7"}}
	left.Places = []pageLearnLink{{Title: "Left", Href: "#left"}}
	right := glossaryFixture("concept-right", "Value", "The second declaration's value.", "q-right")
	right.Code = true
	right.Sources = []pageAnchor{{Path: "types.ts", Line: 4, Text: "types.ts:4", Open: "types.ts:4:41"}}
	right.Places = []pageLearnLink{{Title: "Right", Href: "#right"}}
	catalog := pageGlossaryCatalog(t)
	before, _ := json.Marshal(catalog)
	builder := &pageBuilder{data: &ReportData{Glossary: catalog}, links: pageLinks{sourceIDs: map[string]string{"types.ts": "source-id"}}}
	view := &pageView{Glossary: []pageGlossaryTerm{right, left}}
	if err := builder.reducedGlossary(view); err != nil {
		t.Fatal(err)
	}
	if len(view.Glossary) != 2 || view.Glossary[0].ID != left.ID || view.Glossary[1].ID != right.ID {
		t.Fatalf("same-line declarations lost one of their native destinations: %+v", view.Glossary)
	}
	for i, original := range []pageGlossaryTerm{left, right} {
		term := view.Glossary[i]
		if !term.Code || !reflect.DeepEqual(term.Sources, original.Sources) || !reflect.DeepEqual(term.Places, original.Places) || !reflect.DeepEqual(term.Questions, original.Questions) {
			t.Fatalf("native owner links or exact source column changed: %+v", term)
		}
	}
	after, _ := json.Marshal(catalog)
	if string(before) != string(after) {
		t.Fatal("display projection changed the canonical reduced glossary")
	}

}

func TestReducedGlossaryUsesExactAnswerRowAndKeepsEarlierTermsAvailable(t *testing.T) {
	financeRequest, landRequest, earlierRequest := strings.Repeat("a", 64), strings.Repeat("a", 64), strings.Repeat("c", 64)
	commonSource := []terminology.Source{{Path: "domain.py", Line: 10}}
	finance := terminology.Candidate{Name: "bank", Explanation: "A financial institution.", Sources: commonSource, Origins: []terminology.Origin{{RequestSHA256: financeRequest, Row: "r1"}}}
	land := terminology.Candidate{Name: "bank", Explanation: "Land beside a river.", Sources: commonSource, Origins: []terminology.Origin{{RequestSHA256: landRequest, Row: "r2"}}}
	earlier := terminology.Candidate{Name: "HMM", Explanation: "The hidden Markov model used for segmentation.", Sources: []terminology.Source{{Path: "README.md", Line: 0}}, Origins: []terminology.Origin{{RequestSHA256: earlierRequest}}}
	alias := earlier
	alias.Name = "hidden Markov model"
	alias.Explanation = "The segmentation model, also called HMM."
	catalog := pageGlossaryCatalog(t, []terminology.Candidate{finance}, []terminology.Candidate{land}, []terminology.Candidate{earlier, alias})
	view := &pageView{Questions: []*pageQuestion{
		{ID: "q-finance", Question: "What does bank do?", Answers: []pageAnswerPart{{Text: "The bank uses HMM.", RequestSHA256: financeRequest, OriginRow: "r1"}, {RequestSHA256: financeRequest, OriginRow: "r1"}}},
		{ID: "q-land", Question: "What is beside the bank?", Answers: []pageAnswerPart{{RequestSHA256: landRequest, OriginRow: "r2"}}},
		{ID: "q-later", Question: "Explain HMM and bank.", Answers: []pageAnswerPart{{RequestSHA256: strings.Repeat("d", 64), Checks: []pageQuestionStep{{Source: pageAnchor{Path: "domain.py", Line: 10}}}}}},
		{ID: "q-no-origin", Question: "What happens?", Answers: []pageAnswerPart{{RequestSHA256: financeRequest}}},
	}}
	builder := &pageBuilder{data: &ReportData{Glossary: catalog}, links: pageLinks{repositoryURL: "https://example.test/repo", blobPrefix: "/blob/", revision: "revision"}}
	if err := builder.reducedGlossary(view); err != nil {
		t.Fatal(err)
	}
	if len(view.Glossary) != 4 {
		t.Fatalf("aliases or homonyms disappeared: %+v", view.Glossary)
	}
	var financeID, landID, hmmID string
	for _, term := range view.Glossary {
		switch term.Explanation {
		case finance.Explanation:
			financeID = term.ID
			if len(term.Questions) != 1 || term.Questions[0].Href != "#q-finance" {
				t.Fatal("question sense was inferred from a shared source or repeated per answer part")
			}
		case land.Explanation:
			landID = term.ID
			if len(term.Questions) != 1 || term.Questions[0].Href != "#q-land" {
				t.Fatal("same-spelled sense acquired another answer's origin")
			}
		case earlier.Explanation:
			if len(term.Questions) != 0 || len(term.Sources) != 1 || term.Sources[0].Line != 0 || term.Sources[0].Href != "https://example.test/repo/blob/revision/README.md" {
				t.Fatal("input-only terminology acquired an invented question or source line")
			}
			if term.OriginalName == "HMM" {
				hmmID = term.ID
			}
		}
	}
	page := &PreparedPage{view: view}
	known := page.prepareTerminology("answer", "The bank uses HMM.", "q-finance", nil, nil)
	unknown := page.prepareTerminology("answer", "The bank uses HMM.", "q-later", nil, nil)
	for _, test := range []struct {
		name    string
		entry   DisplayTextEntry
		bankIDs []string
	}{
		{"exact answer scope", known, []string{financeID}},
		{"later answer retains both alternatives", unknown, []string{financeID, landID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.entry.validateTermBindings(); err != nil {
				t.Fatal(err)
			}
			if err := test.entry.ValidateTranslation(test.entry.Text); err != nil {
				t.Fatal(err)
			}
			slices.Sort(test.bankIDs)
			wantTerms := append(append([]string{}, test.bankIDs...), hmmID)
			slices.Sort(wantTerms)
			var gotTerms []string
			for _, term := range test.entry.Terms {
				gotTerms = append(gotTerms, term.ID)
			}
			slices.Sort(gotTerms)
			if !slices.Equal(gotTerms, wantTerms) || len(test.entry.Protected) != 0 {
				t.Fatalf("glossary context changed or plain names became placeholders: %+v", test.entry)
			}
			plain, spans, err := test.entry.finishDisplayText(test.entry.Text)
			wantSpans := []displayTermSpan{{Start: 4, End: 8, IDs: test.bankIDs}, {Start: 14, End: 17, IDs: []string{hmmID}}}
			if err != nil || plain != "The bank uses HMM." || !reflect.DeepEqual(spans, wantSpans) {
				t.Fatalf("exact matches lost source-scoped meanings or earlier terms: %q, %+v, %v", plain, spans, err)
			}
		})
	}
}

func TestGlossaryProjectionIsStableAndDoesNotDuplicateMemberships(t *testing.T) {
	concept := pageLearnConcept{ID: "concept-one", pageMapConcept: pageMapConcept{Name: "Value", Explanation: "An accepted declaration.", Source: pageAnchor{Path: "value.go", Line: 4}},
		Places: []pageLearnLink{{Title: "Area", Href: "#area"}}}
	view := &pageView{LearnConcepts: []pageLearnConcept{concept, concept}, Questions: []*pageQuestion{{ID: "q-one", Question: "What is Value?", Answers: []pageAnswerPart{{Terms: []pageLearnConcept{concept, concept}}}}}}
	collectGlossary(view)
	before, _ := json.Marshal(view.Glossary)
	collectGlossary(view)
	after, _ := json.Marshal(view.Glossary)
	if string(before) != string(after) || len(view.Glossary) != 1 || len(view.Glossary[0].Places) != 1 || len(view.Glossary[0].Questions) != 1 {
		t.Fatal("collecting the same native memberships duplicated glossary destinations")
	}
	a := terminology.Candidate{Name: "zeta", Explanation: "The last name.", Sources: []terminology.Source{{Path: "z.py", Line: 0}}}
	b := terminology.Candidate{Name: "alpha", Explanation: "The first name.", Sources: []terminology.Source{{Path: "a.py", Line: 1}}}
	catalog := pageGlossaryCatalog(t, []terminology.Candidate{a}, []terminology.Candidate{b})
	builder := &pageBuilder{data: &ReportData{Glossary: catalog}}
	view = &pageView{}
	if err := builder.reducedGlossary(view); err != nil {
		t.Fatal(err)
	}
	if view.Glossary[0].OriginalName != "alpha" || view.Glossary[1].OriginalName != "zeta" {
		t.Fatal("dictionary order depends on opaque reduction IDs")
	}
}

func TestQuestionProjectionRetainsOriginalAnswerRequestForGlossaryScope(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	origin := strings.Repeat("e", 64)
	step := atlas.QuestionStep{Path: "README.md", Line: 12, StopIndexes: []int{0}}
	data.Questions = []atlas.QuestionRoute{{Version: atlas.QuestionRouteVersion, Revision: data.CapturedRevision, Question: "What is HMM?",
		Stops:  []atlas.QuestionStop{{Path: "README.md", Line: 12, Name: "HMM"}},
		Answer: &atlas.QuestionAnswer{State: "answered", Parts: []atlas.QuestionAnswerPart{{State: "answered", Text: "A model used by the tokenizer.", Source: atlas.SourceCache, OriginRequest: origin, OriginRow: "r7", Steps: []atlas.QuestionStep{step}}}},
	}}
	view, err := buildPageView(&data, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Questions) != 1 || len(view.Questions[0].Answers) != 1 || view.Questions[0].Answers[0].RequestSHA256 != origin || view.Questions[0].Answers[0].OriginRow != "r7" {
		t.Fatal("cached answer projection lost the exact original request context")
	}
}
