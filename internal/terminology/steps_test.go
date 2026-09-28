package terminology

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// glossaryAnswer answers one of the glossary's text-model requests in a
// test: a names request with names(the texts of its prose), an explanation
// request with explain(name) for each of its terms. Any other request is
// not a glossary request.
func glossaryAnswer(user string, names func([]string) []string, explain func(string) string) (string, bool) {
	var input struct {
		Prose []struct {
			Ref  string   `json:"ref"`
			Text []string `json:"text"`
		} `json:"prose"`
		Terms []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"terms"`
	}
	if json.Unmarshal([]byte(user), &input) != nil || input.Prose == nil {
		return "", false
	}
	if input.Terms == nil {
		var texts []string
		for _, row := range input.Prose {
			texts = append(texts, row.Text...)
		}
		raw, _ := json.Marshal(map[string]any{"names": names(texts)})
		return string(raw), true
	}
	rows := make([]map[string]string, 0, len(input.Terms))
	for _, term := range input.Terms {
		rows = append(rows, map[string]string{"ref": term.Ref, "explanation": explain(term.Name)})
	}
	raw, _ := json.Marshal(map[string]any{"terms": rows})
	return string(raw), true
}

// written lists those of these names that the texts write.
func written(candidates ...string) func([]string) []string {
	return func(texts []string) []string {
		var found []string
		for _, name := range candidates {
			for _, text := range texts {
				if mentionsTerm(text, name) {
					found = append(found, name)
					break
				}
			}
		}
		return found
	}
}

// explainedBy explains each name from a fixed dictionary.
func explainedBy(definitions map[string]string) func(string) string {
	return func(name string) string { return definitions[name] }
}

// everyConcept is a categorizer that decides every name is a domain concept.
func everyConcept() *typesafetest.Categorizer {
	return &typesafetest.Categorizer{Decide: typesafetest.ByColumn(map[string]llm.Verdict{"term": typesafetest.Choose(TermDomainConcept)})}
}

func TestTermOptionsCarryTheirCriteria(t *testing.T) {
	for _, name := range []string{TermDomainConcept, TermGeneralVocabulary, TermCodeElement} {
		criteria := termOptions[name]
		if criteria.What == "" || criteria.Includes == "" || criteria.NotFor == "" || len(criteria.Examples) == 0 {
			t.Fatalf("option %s lacks criteria: %+v", name, criteria)
		}
	}
	def := TermDefinition()
	if !def.Classifier || len(def.Columns) != 1 || len(def.Columns[0].Criteria) != 3 {
		t.Fatalf("the term decision is not one closed question with criteria: %+v", def)
	}
}

// Only a decided domain concept is explained. A near-tie is recorded as
// undecided and remembered, so a warm run asks neither model again.
func TestGlossaryExplainsOnlyDecidedDomainConcepts(t *testing.T) {
	c := NewCollector([]string{"a.go", "b.go"})
	c.pending["a"] = proseSource{Texts: []string{"Each LTX file is shipped by the logger."}, Sources: []Source{{Path: "a.go", Line: 3}}, Origin: Origin{RequestSHA256: strings.Repeat("a", 64), Row: "r1"}}
	c.pending["b"] = proseSource{Texts: []string{"The worker count bounds LTX files; the lock page stays empty."}, Sources: []Source{{Path: "b.go", Line: 9}}, Origin: Origin{RequestSHA256: strings.Repeat("b", 64), Row: "r2"}}
	verdicts := map[string]llm.Verdict{
		"LTX file":     typesafetest.Choose(TermDomainConcept),
		"logger":       typesafetest.Choose(TermGeneralVocabulary),
		"worker count": typesafetest.Choose(TermCodeElement),
		"lock page":    {Choice: TermDomainConcept, Probabilities: map[string]float64{TermDomainConcept: 0.45, TermCodeElement: 0.40, TermGeneralVocabulary: 0.15}},
	}
	categorizer := &typesafetest.Categorizer{Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
		name, _ := question.Item["name"].(string)
		verdict, known := verdicts[name]
		return verdict, known && question.Name == "candidate"
	}}
	var explained []string
	provider := &testProvider{}
	provider.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		answer, _ := glossaryAnswer(inputUser(t, prepared), written("LTX file", "logger", "worker count", "lock page"), func(name string) string {
			explained = append(explained, name)
			return "The meaning of " + name + "."
		})
		return completed(answer)
	}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	if err := c.Generate(t.Context(), executor, provider, categorizer, "A replication tool."); err != nil {
		t.Fatal(err)
	}
	got := c.Snapshot()
	if len(got) != 1 || got[0].Name != "LTX file" || !reflect.DeepEqual(explained, []string{"LTX file"}) || len(got[0].Origins) != 2 {
		t.Fatalf("published %+v, explained %v", got, explained)
	}
	want := []NameDecision{{"lock page", NameUndecided, 1}, {"logger", TermGeneralVocabulary, 1}, {"LTX file", TermDomainConcept, 2}, {"worker count", TermCodeElement, 1}}
	if !reflect.DeepEqual(c.Decisions(), want) {
		t.Fatalf("decisions: %+v", c.Decisions())
	}
	calls, asked := provider.calls, categorizer.Calls()
	warm := NewCollector([]string{"a.go", "b.go"})
	maps.Copy(warm.pending, c.pending)
	if err := warm.Generate(t.Context(), executor, provider, categorizer, "A replication tool."); err != nil {
		t.Fatal(err)
	}
	if provider.calls != calls || categorizer.Calls() != asked || !reflect.DeepEqual(warm.Snapshot(), got) || !reflect.DeepEqual(warm.Decisions(), want) {
		t.Fatalf("a warm run asked again or decided otherwise: calls %d->%d, categorizer %d->%d, %+v", calls, provider.calls, asked, categorizer.Calls(), warm.Decisions())
	}
}

// A name found in one window is one term for all prose: the term lookup's
// case and plural spellings fold into it, and every row that writes it is
// attached, keeping the spelling the prose writes most often.
func TestGatheredNameFoldsSpellingsAndAttachesEveryRow(t *testing.T) {
	items := []proseSource{
		{Texts: []string{"WAL segments are shipped."}},
		{Texts: []string{"One WAL segment per commit."}},
		{Texts: []string{"No wal segment is lost; more WAL segments follow."}},
		{Texts: []string{"Nothing here."}},
	}
	names := gatherNames(items, []string{"WAL segments", "wal segment", "WAL segment"})
	if len(names) != 1 || names[0].Name != "WAL segment" || !reflect.DeepEqual(names[0].Rows, []int{0, 1, 2}) {
		t.Fatalf("gathered: %+v", names)
	}
}

// The question about a name carries every text that writes it, however many.
func TestTermQuestionCarriesEveryWrittenText(t *testing.T) {
	var items []proseSource
	var rows []int
	for i := range 300 {
		items = append(items, proseSource{Texts: []string{fmt.Sprintf("Row %d writes the lock page %s.", i, strings.Repeat("x", 500))}})
		rows = append(rows, i)
	}
	fields := termFields(items, glossaryName{Name: "lock page", Rows: rows})
	if len(fields) != 2 || len(fields[1].Value.([]string)) != 300 {
		t.Fatalf("the question left texts out: %d fields", len(fields))
	}
}

// A name's decision is remembered by the name and the texts that write it,
// not by the window it was asked in: prose that adds a name asks that name
// alone, and the names whose texts did not change are not asked again.
func TestAGlossaryDecisionIsRememberedPerName(t *testing.T) {
	prose := map[string]proseSource{
		"a": {Texts: []string{"Each LTX file is shipped by the logger."}, Sources: []Source{{Path: "a.go", Line: 3}}, Origin: Origin{RequestSHA256: strings.Repeat("a", 64), Row: "r1"}},
	}
	var asked []string
	categorizer := &typesafetest.Categorizer{Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
		name, _ := question.Item["name"].(string)
		asked = append(asked, name)
		return typesafetest.Choose(TermDomainConcept), question.Name == "candidate"
	}}
	provider := &testProvider{}
	provider.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		answer, _ := glossaryAnswer(inputUser(t, prepared), written("LTX file", "logger", "lock page"), func(name string) string { return "The meaning of " + name + "." })
		return completed(answer)
	}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	run := func() {
		c := NewCollector([]string{"a.go", "b.go"})
		maps.Copy(c.pending, prose)
		if err := c.Generate(t.Context(), executor, provider, categorizer, "A replication tool."); err != nil {
			t.Fatal(err)
		}
	}
	run()
	slices.Sort(asked)
	if !slices.Equal(asked, []string{"LTX file", "logger"}) {
		t.Fatalf("the first run asked %v", asked)
	}
	asked = nil
	prose["b"] = proseSource{Texts: []string{"The lock page stays empty."}, Sources: []Source{{Path: "b.go", Line: 9}}, Origin: Origin{RequestSHA256: strings.Repeat("b", 64), Row: "r2"}}
	run()
	if !slices.Equal(asked, []string{"lock page"}) {
		t.Fatalf("the second run asked %v, want the new name alone", asked)
	}
}
