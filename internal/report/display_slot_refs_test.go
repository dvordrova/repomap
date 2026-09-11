package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestDisplaySlotsKeepDistinctMeaningsForIdenticalTranslatedText(t *testing.T) {
	page := &PreparedPage{view: &pageView{
		Summary:    &pageSentence{Text: "A bank."},
		LearnBands: []pageLearnBand{{Parts: []pageLearnPart{{Summary: "A bank."}}}},
		Glossary: []pageGlossaryTerm{
			glossaryFixture("finance", "bank", "A financial institution.", "q-finance"),
			glossaryFixture("river", "bank", "The land beside a river.", "q-river"),
		},
		Questions: []*pageQuestion{
			{ID: "q-finance", Question: "Finance?", Answers: []pageAnswerPart{{Text: "A bank."}}},
			{ID: "q-river", Question: "Landscape?", Answers: []pageAnswerPart{{Text: "A bank."}}},
		},
	}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	finance, river := &page.view.Questions[0].Answers[0], &page.view.Questions[1].Answers[0]
	if finance.TextRef == "" || river.TextRef == "" || finance.TextRef == river.TextRef || page.view.Summary.TextRef == finance.TextRef {
		t.Fatal("separate text slots lost their contextual catalogue refs")
	}
	if page.view.LearnBands[0].Parts[0].SummaryRef != page.view.Summary.TextRef {
		t.Fatal("identical text and bindings stopped sharing their existing catalogue entry")
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256}
	for _, entry := range page.catalog.Entries {
		value := DisplayTranslationEntry{Ref: entry.Ref, Text: entry.Text}
		if entry.Ref == finance.TextRef || entry.Ref == river.TextRef || entry.Ref == page.view.Summary.TextRef {
			value.Text = "Это bank."
		}
		if err := entry.ValidateTranslation(value.Text); err != nil {
			t.Fatal(err)
		}
		translations.Entries = append(translations.Entries, value)
	}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
		t.Fatal(err)
	}
	if finance.Text != "Это bank." || river.Text != finance.Text || page.view.Summary.Text != finance.Text {
		t.Fatal("fixture did not produce identical final text in three separate contexts")
	}
	var plans []displayTermPlan
	if err := json.Unmarshal([]byte(page.view.TermMentionsJSON), &plans); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		finance.TextRef:           {"finance"},
		river.TextRef:             {"river"},
		page.view.Summary.TextRef: {"finance", "river"},
	}
	if len(plans) != len(want) {
		t.Fatalf("a source-scoped slot lost its dictionary or global alternatives: %+v", plans)
	}
	for _, plan := range plans {
		if plan.Text != finance.Text || len(plan.Spans) != 1 || plan.Spans[0].Start != 4 || plan.Spans[0].End != 8 || !reflect.DeepEqual(plan.Spans[0].IDs, want[plan.Ref]) {
			t.Fatalf("one text slot acquired another slot's definition: %+v", plan)
		}
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, question := range page.view.Questions {
		var html bytes.Buffer
		if err := parsed.ExecuteTemplate(&html, "question.html", question); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.String(), `data-display-ref="`+question.Answers[0].TextRef+`">Это bank.</p>`) {
			t.Fatalf("the visible answer lost its exact text ref: %s", html.String())
		}
	}
}

func TestMapConceptCarriesItsExplanationRefThroughDisplayAssembly(t *testing.T) {
	original := pageMapConcept{Name: "Parser", Explanation: "A bank.", Source: pageAnchor{Text: "parser.go:12", Href: "/source/parser.go#L12"}}
	raw, err := json.Marshal([]pageMapConcept{original})
	if err != nil {
		t.Fatal(err)
	}
	page := &PreparedPage{view: &pageView{Sections: []*pageSection{{Map: &pageMap{Nodes: []pageMapNode{{Concepts: string(raw)}}}}}},
		catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	if err := page.applyDisplay(RenderOptions{Language: English}); err != nil {
		t.Fatal(err)
	}
	var concepts []pageMapConcept
	if err := json.Unmarshal([]byte(page.view.Sections[0].Map.Nodes[0].Concepts), &concepts); err != nil {
		t.Fatal(err)
	}
	if len(concepts) != 1 || len(page.catalog.Entries) != 1 {
		t.Fatal("concept explanation was not collected once")
	}
	original.ExplanationRef = page.catalog.Entries[0].Ref
	if concepts[0] != original {
		t.Fatalf("display assembly lost the ref or changed native concept data: %+v", concepts[0])
	}
}

func TestModelProseRefsExcludeTheSourcePreviewHost(t *testing.T) {
	group := pageGroup{ID: "part", Summary: "A <bank>.", SummaryRef: "t1", Highlights: []pageChipRow{{Path: "parser.go", Members: []pageChip{{
		Name: "Parser", Line: 12, Summary: "Reads a <bank>.", SummaryRef: "t2", Anchor: pageAnchor{Text: "parser.go:12", Href: "/source/parser.go#L12"},
	}}}}}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := parsed.ExecuteTemplate(&html, "group", group); err != nil {
		t.Fatal(err)
	}
	// This fragment is also valid XML. Inspect its element boundaries, rather
	// than requiring a particular layout around the prose and its citations.
	decoder := xml.NewDecoder(bytes.NewReader(html.Bytes()))
	depth, refDepth := 0, 0
	var ref string
	var prose strings.Builder
	got := make(map[string]string)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			var nextRef, class string
			for _, attribute := range token.Attr {
				switch attribute.Name.Local {
				case "data-display-ref":
					nextRef = attribute.Value
				case "class":
					class = attribute.Value
				}
			}
			if nextRef != "" {
				if strings.Contains(" "+class+" ", " model ") || ref != "" {
					t.Fatal("a prose ref includes the model host where the source-preview script appends controls")
				}
				ref, refDepth = nextRef, depth
			}
		case xml.CharData:
			if ref != "" {
				prose.Write(token)
			}
		case xml.EndElement:
			if depth == refDepth && ref != "" {
				got[ref] = prose.String()
				ref = ""
				prose.Reset()
			}
			depth--
		}
	}
	if !reflect.DeepEqual(got, map[string]string{"t1": "A <bank>.", "t2": "Reads a <bank>."}) {
		t.Fatalf("prose slots include source labels or lost their original text: %+v", got)
	}
}
