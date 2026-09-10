package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestOperationLabelsKeepEnglishNamesAndExactDeclarationAliases(t *testing.T) {
	const purpose = "Handles the user's input."
	const translated = "Обрабатывает ввод пользователя."
	cases := []struct{ name, native, alias, kind, source, want string }{
		{"키누름", "키누름", "Handle key press", "interaction", "model", "Handle key press (키누름)"},
		{"애니메이션", "애니메이션", "Animate simulation", "continuous", "model", "Animate simulation (애니메이션)"},
		{"예약작업", "예약작업", "Scheduled refresh", "scheduled", "model", "Scheduled refresh (예약작업)"},
		{"Submit selected order", "제출", "Order submission", "interaction", "model", "Submit selected order"},
		{"animate", "animate", "", "interaction", "model", "animate"},
		{"onSlownessChange", "onSlownessChange", "", "interaction", "model", "onSlownessChange"},
		{"Open", "Open", "", "interaction", "model", "Open"},
		{"앱 --start", "앱 --start", "Start application", "command", "model", "앱 --start"},
		{"POST /게임", "POST /게임", "Start game", "request", "fact", "POST /게임"},
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			section := &pageSection{ID: "app", ShortLabel: "app", programTargetID: "program"}
			other := &pageSection{ID: "other", ShortLabel: "other", programTargetID: "other"}
			index := groupindex.Index{Target: programindex.Target{ID: "program"}}
			peer := groupindex.Index{Target: programindex.Target{ID: "other"}, Groups: []groupindex.Group{{ID: "peer"}}}
			group := groupindex.Group{ID: "actions", Lane: groupindex.LaneTriggers}
			builder := pageBuilder{data: &ReportData{}, subjects: map[string]subjectRef{}, byProgram: map[string]*pageSection{"program": section, "other": other}}
			for i, value := range cases {
				id := fmt.Sprintf("subject-%d", i)
				location := programindex.Location{Path: "app.py", Line: 10 + i, Column: 1}
				subject := groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: value.native, Kind: programindex.ObjectFunction, Location: &location}, Interpretation: &groupindex.Interpretation{Alias: value.alias}}
				builder.subjects[id] = subjectRef{subject: subject, programTargetID: "program"}
				index.Subjects = append(index.Subjects, subject)
				group.MemberSubjectIDs = append(group.MemberSubjectIDs, id)
				index.Operations = append(index.Operations, groupindex.Operation{ID: id, SubjectID: id, GroupID: group.ID, Name: value.name, Kind: value.kind, Source: value.source, Summary: purpose, Location: location})
				peer.Connections = append(peer.Connections, groupindex.Connection{ID: id, SourceKind: "integration", From: groupindex.Endpoint{TargetID: "other", GroupID: "peer"}, To: groupindex.Endpoint{TargetID: "program", GroupID: group.ID}, ToLocation: &location})
			}
			index.Groups = []groupindex.Group{group}
			builder.indexes = []groupindex.Index{index, peer}
			section.Map = builder.buildOperationMap(section, &index)
			other.Map = builder.buildOperationMap(other, &peer)
			builder.fillSectionOperations(section)
			section.Triggers = []pageGroup{builder.groupCard(section.ID, index, group)}
			other.Core = []pageGroup{builder.groupCard(other.ID, peer, groupindex.Group{ID: "peer"})}
			question := &pageQuestion{ID: "q", Answers: []pageAnswerPart{{MapLinks: builder.questionStepMapLinks(atlas.QuestionStop{SubjectID: "subject-0"})}}}
			page := &PreparedPage{view: &pageView{Sections: []*pageSection{other, section}, Questions: []*pageQuestion{question}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion}}
			if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
				t.Fatal(err)
			}
			page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
			translations := DisplayTranslations{Version: DisplayTextVersion, Language: language, CatalogSHA256: page.catalog.SHA256}
			for _, entry := range page.catalog.Entries {
				if entry.Role != "summary" || entry.Text != purpose {
					t.Fatalf("operation name or source entered translation: %+v", entry)
				}
				translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated})
			}
			if len(translations.Entries) != 1 {
				t.Fatalf("operation descriptions were not collected once: %+v", page.catalog)
			}
			if err := page.applyDisplay(RenderOptions{Language: language, Translations: &translations}); err != nil {
				t.Fatal(err)
			}
			wantPurpose := purpose
			if language == Russian {
				wantPurpose = translated
			}
			for i, value := range cases {
				operation := section.Triggers[0].Operations[i]
				if operation.Name != value.want || operation.Summary != wantPurpose || other.Core[0].Connections[i].Title != value.want {
					t.Fatalf("card or cross-target action changed: %+v / %+v", operation, other.Core[0].Connections[i])
				}
				id := operationNodeID(section.ID, index.Operations[i].ID)
				found := false
				for _, node := range section.Map.Nodes {
					if node.ID == id {
						found = node.FullTitle == value.want && node.CanonicalTitle == value.want && node.Summary == wantPurpose
					}
				}
				if !found || index.Operations[i].Name != value.name || index.Subjects[i].Object.Name != value.native {
					t.Fatalf("map name or native authority changed for %q", value.name)
				}
				remoteFound := false
				for _, node := range other.Map.Nodes {
					if node.Href == "#"+id {
						remoteFound = node.Remote && node.Activation == "" && node.FullTitle == "app / "+value.want &&
							node.CanonicalTitle == "app / "+value.want && node.Summary == wantPurpose
					}
				}
				if !remoteFound {
					t.Fatalf("remote map peer lost its exact operation alias or description: %q", value.want)
				}
			}
			if got := question.Answers[0].MapLinks[0].Label; got != "app / "+cases[0].want {
				t.Fatalf("question link lost the accepted alias: %q", got)
			}
			parsed, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) { return uiText(language, key, args...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			section.InputsCount = len(cases)
			section.InboundCount = len(section.Requests)
			for _, templateName := range []string{"input-catalog", "map.html"} {
				var out bytes.Buffer
				var view any = section
				if templateName == "map.html" {
					view = section.Map
				}
				if err := parsed.ExecuteTemplate(&out, templateName, view); err != nil {
					t.Fatal(err)
				}
				html := stdhtml.UnescapeString(out.String())
				for _, value := range cases {
					if !strings.Contains(html, value.want) || !strings.Contains(html, wantPurpose) {
						t.Fatalf("%s lost an action name or description: %q", templateName, value.want)
					}
				}
			}
		})
	}
}

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
