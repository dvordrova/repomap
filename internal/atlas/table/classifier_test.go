package table

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

func closedDefinition() Definition {
	return Definition{
		Stage: "atlas_closed", Contract: "repomap.atlas.closed.v1", System: "choose a part",
		Columns: []Column{{Name: "part", Kind: Choice, OptionsFrom: "part_options"}},
	}
}

func closedWindow() Window {
	return Window{
		Context: []Field{{Name: "parts", Value: []map[string]any{{"ref": "c1", "title": "Serving"}}}, {Name: "part_options", Value: []string{"c1", "none"}}},
		Rows:    []Row{{ID: "s1", Fields: []Field{{Name: "name", Value: "Serve"}}}, {ID: "s2", Fields: []Field{{Name: "name", Value: "helper"}}}},
	}
}

func chose(choice string, probabilities map[string]float64) llm.Verdict {
	return llm.Verdict{Choice: choice, Probabilities: probabilities}
}

func yes(p float64) llm.Verdict { return llm.Verdict{Yes: &p} }

// A chosen option that does not lead its runner-up by the margin refuses
// its own row, not its neighbour; an unlisted choice is never taken.
func TestDecodeClassifierRefusesUncertainRowsAlone(t *testing.T) {
	window := closedWindow()
	result, err := DecodeClassifierAnswers(closedDefinition(), window, map[string]llm.Verdict{
		"s1|part": chose("Serving", map[string]float64{"Serving": 0.9, "none": 0.1}),
		"s2|part": chose("Serving", map[string]float64{"Serving": 0.4, "none": 0.35}),
	})
	if err != nil || result.Answers[0]["part"] != "c1" || result.Answers[1] != nil || len(result.Rejections) != 1 || result.Rejections[0].Key != "s2" || strings.Join(result.AcceptedRowKeys(), " ") != "s1" {
		t.Fatalf("result %+v %v", result, err)
	}
	result, err = DecodeClassifierAnswers(closedDefinition(), window, map[string]llm.Verdict{
		"s1|part": chose("c9", map[string]float64{"c9": 1}),
		"s2|part": chose("none", map[string]float64{"none": 1}),
	})
	if err != nil || result.Answers[0] != nil || result.Answers[1]["part"] != "none" {
		t.Fatalf("unlisted choice: %+v %v", result, err)
	}
}

// A choice is taken when it leads its runner-up by the margin, whatever its
// own probability: "support" at 0.49 against 0.32 is decided, and 0.51
// against 0.49, which the old 0.50 floor took, is the explicit unknown. A
// lead of exactly the margin counts; a choice below its rival never does.
func TestAClassifierChoiceMustLeadItsRunnerUp(t *testing.T) {
	def := Definition{Stage: "atlas_core", Contract: "c", System: "s", Columns: []Column{
		{Name: "role", Kind: Choice, Options: []string{"domain", "interface", "wiring", "support"}},
	}}
	for name, tc := range map[string]struct {
		verdict llm.Verdict
		want    string
	}{
		"clear lead under one half":  {chose("support", map[string]float64{"support": 0.49, "domain": 0.32, "interface": 0.12, "wiring": 0.07}), "support"},
		"near-tie over one half":     {chose("domain", map[string]float64{"domain": 0.51, "interface": 0.49, "wiring": 0, "support": 0}), ""},
		"lead of exactly the margin": {chose("wiring", map[string]float64{"wiring": 0.3, "domain": 0.2, "interface": 0.2, "support": 0.2}), "wiring"},
		"choice below its rival":     {chose("domain", map[string]float64{"domain": 0.33, "interface": 0.34, "wiring": 0.2, "support": 0.13}), ""},
	} {
		window := Window{Rows: []Row{{ID: "p1"}}}
		result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"p1|role": tc.verdict})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tc.want != "" && result.Answers[0]["role"] != tc.want {
			t.Fatalf("%s: not decided: %+v", name, result)
		}
		if tc.want == "" && (result.Answers[0] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, "is uncertain")) {
			t.Fatalf("%s: decided without a lead: %+v", name, result)
		}
	}
	window := Window{Rows: []Row{{ID: "p1"}}}
	result, _ := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"p1|role": chose("domain", map[string]float64{"domain": 0.51, "interface": 0.49})})
	if len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, `against "interface" at 0.49`) {
		t.Fatalf("the journal does not name the runner-up: %+v", result.Rejections)
	}
}

// An optional column's "none of these" is one more option: it leaves the
// cell empty when it leads by the margin and is uncertain when it does not,
// and a listed option close behind it is no more decided than one close
// behind another listed option.
func TestNoneOfTheseMustLeadLikeAnyOption(t *testing.T) {
	def := Definition{Stage: "atlas_optional", Contract: "c", System: "s", Columns: []Column{
		{Name: "talks", Kind: Choice, Options: []string{"db", "sdk"}, Optional: true},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|talks": chose("none of these", map[string]float64{"none of these": 0.45, "db": 0.3, "sdk": 0.25}),
		"s2|talks": chose("none of these", map[string]float64{"none of these": 0.4, "db": 0.35, "sdk": 0.25}),
		"s3|talks": chose("db", map[string]float64{"db": 0.45, "none of these": 0.4, "sdk": 0.15}),
	})
	if err != nil || result.Answers[0] == nil || len(result.Answers[0]) != 0 || result.Answers[1] != nil || result.Answers[2] != nil {
		t.Fatalf("result %+v %v", result, err)
	}
	if len(result.Rejections) != 2 || !strings.Contains(result.Rejections[1].Reason, `against "none of these" at 0.40`) {
		t.Fatalf("a listed option close behind none of these was decided: %+v", result.Rejections)
	}
}

// A yes/no asked as a choice without a cutoff follows the same margin: yes
// at 0.54 against 0.46 is uncertain, not the yes the old floor took. A yes/no
// asked as a noul is one probability and keeps its band around one half.
func TestYesNoChoicesFollowTheMarginAndNoulsTheirBand(t *testing.T) {
	def := Definition{Stage: "atlas_yes_no", Contract: "c", System: "s", Columns: []Column{
		{Name: "entry", Kind: Choice, Options: []string{"yes", "no"}},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|entry": chose("yes", map[string]float64{"yes": 0.54, "no": 0.46}),
		"s2|entry": chose("no", map[string]float64{"yes": 0.44, "no": 0.56}),
	})
	if err != nil || result.Answers[0] != nil || result.Answers[1]["entry"] != "no" {
		t.Fatalf("yes/no choice: %+v %v", result, err)
	}
	def.Columns = []Column{{Name: "entry", Kind: Choice, Options: []string{"yes"}, Optional: true}}
	result, err = DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"s1|entry": yes(0.58), "s2|entry": yes(0.62)})
	if err != nil || result.Answers[0] != nil || result.Answers[1]["entry"] != "yes" {
		t.Fatalf("noul: %+v %v", result, err)
	}
}

func TestOnlyUnconditionalChoicesAreClosed(t *testing.T) {
	def := closedDefinition()
	if !Closed(def) {
		t.Fatal("a closed choice table is not closed")
	}
	def.Columns = append(def.Columns, Column{Name: "line", Kind: Text})
	if Closed(def) {
		t.Fatal("a table with text is closed")
	}
	def.Columns = []Column{{Name: "box", Kind: Choice, OptionsFrom: "box_options", Free: "new: "}}
	if Closed(def) {
		t.Fatal("a choice that allows free text is closed")
	}
}

// An optional yes-only column is a yes/no question, and an optional choice
// has an explicit way to say that nothing applies: a decision model that
// must pick a listed value otherwise answers yes to every row. The request
// side of both is in the Jev request golden.
func TestOptionalColumnsCanBeLeftEmpty(t *testing.T) {
	def := Definition{Stage: "atlas_optional", Contract: "c", System: "s", Columns: []Column{
		{Name: "explains", Kind: Choice, Options: []string{"yes"}, Optional: true},
		{Name: "talks", Kind: Choice, Options: []string{"db", "sdk"}, Optional: true},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|explains": yes(0.8), "s1|talks": chose("db", map[string]float64{"db": 0.9}),
		"s2|explains": yes(0.2), "s2|talks": chose("none of these", map[string]float64{"none of these": 0.9}),
	})
	if err != nil || result.Answers[0]["explains"] != "yes" || result.Answers[0]["talks"] != "db" || len(result.Answers[1]) != 0 || result.Answers[1] == nil {
		t.Fatalf("result %+v %v", result, err)
	}
}

// A yes/no cutoff decides every row: yes at the cutoff or above, no below.
func TestYesAtIsACutoffNotAnUncertainBand(t *testing.T) {
	def := Definition{Stage: "atlas_cutoff", Contract: "c", System: "s", YesAt: 0.8,
		Columns: []Column{{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}}}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|key_symbol": chose("yes", map[string]float64{"yes": 0.85, "no": 0.15}),
		"s2|key_symbol": chose("yes", map[string]float64{"yes": 0.7, "no": 0.3}),
	})
	if err != nil || result.Answers[0]["key_symbol"] != "yes" || result.Answers[1]["key_symbol"] != "no" {
		t.Fatalf("result %+v %v", result, err)
	}
}

// A window whose categorizer body is too large is halved, keeping every
// row: options repeated per question do not count in byte packing.
func TestFitClassifierWindowsHalvesOversizedBodiesAndKeepsRows(t *testing.T) {
	def := closedDefinition()
	var titles []map[string]any
	var options []string
	for i := 1; i <= 18; i++ {
		ref := fmt.Sprintf("c%d", i)
		titles = append(titles, map[string]any{"ref": ref, "title": strings.Repeat("Part title number ", 3) + ref})
		options = append(options, ref)
	}
	window := Window{Context: []Field{{Name: "parts", Value: titles}, {Name: "part_options", Value: append(options, "none")}}}
	for i := 0; i < 150; i++ {
		window.Rows = append(window.Rows, Row{ID: fmt.Sprintf("s%d", i+1), Fields: []Field{{Name: "name", Value: strings.Repeat("x", 300)}}})
	}
	jev := &typesafe.Client{}
	fitted, err := FitClassifierWindows(jev, def, []Window{window})
	if err != nil || len(fitted) < 2 {
		t.Fatalf("fitted %d windows, %v", len(fitted), err)
	}
	rows := 0
	for _, piece := range fitted {
		call, _ := ClassifierCall(jev, def, piece)
		if len(call.Prompt.User) > ClassifierBodyBytes {
			t.Fatalf("piece body %d bytes", len(call.Prompt.User))
		}
		rows += len(piece.Rows)
	}
	if rows != 150 {
		t.Fatalf("rows %d, want 150", rows)
	}
}

// A column that reads its options and their criteria from a catalogue of
// objects sends each option under its title with that entry's text as its
// criteria, and the catalogue itself only that way: the shared context
// keeps the other fields. Two entries sharing a title stay two options,
// each shown by its ref with its own criteria, and a choice of either is
// read back as its ref.
func TestCriteriaFromACatalogueAreTheOptionsOwnTerms(t *testing.T) {
	def := Definition{
		Stage: "atlas_catalogue", Contract: "repomap.atlas.catalogue.v1", System: "choose a box", Classifier: true,
		Columns: []Column{{Name: "box", Kind: Choice, OptionsFrom: "boxes", CriteriaFrom: "holds", Item: "declaration", Ask: "Which box does `declaration` go in?"}},
	}
	window := Window{
		Context: []Field{{Name: "file", Value: "a.c"}, {Name: "boxes", Value: []map[string]any{
			{"ref": "b1", "title": "Replies", "holds": "The reply buffers."},
			{"ref": "b2", "title": "Keys", "holds": "The key lookups."},
			{"ref": "b3", "title": "keys", "holds": "The key commands."},
		}}},
		Rows: []Row{{ID: "d1", Fields: []Field{{Name: "declaration", Value: "addReply"}}}},
	}
	client := &typesafe.Client{Model: "jev-test"}
	call, err := ClassifierCall(client, def, window)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"questions":{"d1|box":{"criteria":{"Replies":"The reply buffers.","b2":"The key lookups.","b3":"The key commands."},"instructions":{"declaration":{"declaration":"addReply"},"question":"Which box does ` + "`declaration`" + ` go in?"},"type":"choice"}},"state":{"context":{"file":"a.c"},"task":"choose a box"}}`
	if call.Prompt.User != want {
		t.Fatalf("request:\n%s\nwant:\n%s", call.Prompt.User, want)
	}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"d1|box": chose("b3", map[string]float64{"b3": 0.7, "b2": 0.2, "Replies": 0.1})})
	if err != nil || result.Answers[0]["box"] != "b3" {
		t.Fatalf("answer %v, %v", result.Answers, err)
	}
}

// Static options carry their own criteria object.
func TestStaticOptionsCarryTheirCriteria(t *testing.T) {
	def := Definition{
		Stage: "atlas_gate", Contract: "repomap.atlas.gate.v1", System: "gate", Classifier: true,
		Columns: []Column{{Name: "boxes", Kind: Choice, Options: []string{"one box", "several boxes"}, Item: "file", Ask: "One or several?",
			Criteria: map[string]llm.Criteria{"one box": {What: "one job"}, "several boxes": {What: "several jobs", Examples: []string{"a web file"}}}}},
	}
	window := Window{Rows: []Row{{ID: "f1", Fields: []Field{{Name: "path", Value: "a.c"}}}}}
	call, err := ClassifierCall(&typesafe.Client{Model: "jev-test"}, def, window)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"questions":{"f1|boxes":{"criteria":{"one box":{"what":"one job"},"several boxes":{"examples":["a web file"],"what":"several jobs"}},"instructions":{"file":{"path":"a.c"},"question":"One or several?"},"type":"choice"}},"state":{"context":{},"task":"gate"}}`
	if call.Prompt.User != want {
		t.Fatalf("request:\n%s\nwant:\n%s", call.Prompt.User, want)
	}
}
