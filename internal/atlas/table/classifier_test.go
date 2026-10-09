package table

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// A categorizer may own an envelope different from Jev's. Tables must ask
// its preparation, and preserve every original row when it requires a split.
type twoRowCategorizer struct{ typesafe.Client }

func (c *twoRowCategorizer) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var body struct{ Questions map[string]json.RawMessage }
	if err := json.Unmarshal([]byte(prompt.User), &body); err != nil {
		return llm.Prepared{}, err
	}
	if len(body.Questions) > 2 {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens, Limit: 2, Observed: len(body.Questions), ObservedKnown: true})
	}
	return c.Client.Prepare(prompt, limits)
}

func TestClassifierPartitionUsesTheActualProviderPreparation(t *testing.T) {
	c := &twoRowCategorizer{}
	window := closedWindow()
	window.Rows = append(window.Rows, Row{ID: "s3", Fields: []Field{{Name: "name", Value: "Third"}}})
	needs, err := ClassifierNeedsPartition(c, closedDefinition(), window)
	if err != nil || !needs {
		t.Fatalf("preflight: needs=%t err=%v", needs, err)
	}
	pieces, err := FitClassifierWindows(c, closedDefinition(), []Window{window})
	if err != nil || len(pieces) != 2 {
		t.Fatalf("partition: pieces=%d err=%v", len(pieces), err)
	}
	var rows []Row
	for _, piece := range pieces {
		if piece.Refused != "" {
			t.Fatalf("accepted neighbour refused: %s", piece.Refused)
		}
		if err := prepareClassifier(c, closedDefinition(), piece); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, piece.Rows...)
	}
	if fmt.Sprint(rows) != fmt.Sprint(window.Rows) {
		t.Fatal("partition changed row evidence or order")
	}
}

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

func TestRequiredProbabilitiesRefuseOnlyTheirCellAndKeepExplicitZero(t *testing.T) {
	def := Definition{Stage: "scores", Columns: []Column{
		{Name: "part", Kind: Choice, Options: []string{"A", "B"}, Alone: true},
		{Name: "other", Kind: Choice, Options: []string{"A", "B"}},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	for name, invalid := range map[string]llm.Verdict{
		"missing rival":        chose("A", map[string]float64{"A": 0.9}),
		"missing chosen":       chose("A", map[string]float64{"B": 0}),
		"negative":             chose("A", map[string]float64{"A": 1, "B": -0.1}),
		"over one":             chose("A", map[string]float64{"A": 2, "B": 0}),
		"NaN":                  chose("A", map[string]float64{"A": math.NaN(), "B": 0}),
		"infinite rival":       chose("A", map[string]float64{"A": 1, "B": math.Inf(1)}),
		"invalid known marker": {Choice: "A", Probabilities: map[string]float64{"A": 1, "B": 0}, InvalidProbabilities: []string{" B "}},
	} {
		t.Run(name, func(t *testing.T) {
			valid := chose("A", map[string]float64{"A": 1, "B": 0, "unknown": math.NaN()})
			result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
				"s1|part": invalid, "s1|other": valid, "s2|part": valid, "s2|other": valid,
			})
			if err != nil || result.Answers[0]["part"] != "" || result.Answers[0]["other"] != "A" || result.Answers[1]["part"] != "A" || result.Uncertain(0) || len(result.Rejections) != 1 || result.Rejections[0].Cell != "part" {
				t.Fatalf("invalid score repaired or refused accepted neighbour: %+v / %v", result, err)
			}
		})
	}
}

func TestRawScoreOccurrencesKeepKnownRefusalsAndDiscardUnknownExtras(t *testing.T) {
	def := Definition{Stage: "scores", Columns: []Column{{Name: "pick", Kind: Choice, Options: []string{"A", "B"}}}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	good := `{"type":"choice","choice":"A","probabilities":{"A":1,"B":0}}`
	for _, bad := range []string{
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":null}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":"NaN"}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":1e309}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":0," B ":0.5}}`,
		`{"type":"choice","choice":"A","choice":"B","probabilities":{"A":1,"B":0}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":0},"probabilities":{"A":1}}`,
	} {
		for _, copies := range []string{bad, bad + `,"s1|pick":` + good, good + `,"s1|pick":` + bad} {
			verdicts, err := (&typesafe.Client{}).Verdicts([]byte(`{"answers":{"s1|pick":` + copies + `,"s2|pick":` + good + `}}`))
			if err != nil {
				t.Fatal(err)
			}
			result, err := DecodeClassifierAnswers(def, window, verdicts)
			if err != nil || result.Answers[0] != nil || result.Uncertain(0) || result.Answers[1]["pick"] != "A" {
				t.Fatalf("known invalid occurrence repaired: %s / %+v / %v", copies, result, err)
			}
		}
	}
	for _, extra := range []string{
		`{"TYPE":" choice ","choice":" A ","probabilities":{" A ":1,"b":0,"unknown":null,"another":"NaN"}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":0,"unknown":1e309}}`,
		`{"type":"choice","choice":"A","probabilities":{"A":1,"B":0,"unknown":1,"unknown":0}}`,
	} {
		verdicts, err := (&typesafe.Client{}).Verdicts([]byte(`{"answers":{"s1|pick":` + extra + `,"s1|pick":` + good + `,"s2|pick":` + good + `}}`))
		if err != nil {
			t.Fatal(err)
		}
		result, err := DecodeClassifierAnswers(def, window, verdicts)
		if err != nil || result.Answers[0]["pick"] != "A" || len(result.Rejections) != 0 {
			t.Fatalf("unknown extra poisoned known complete scores: %s / %+v / %v", extra, result, err)
		}
	}
}

func TestRequiredProbabilityValidationPrecedesCutoffAndRankedNoul(t *testing.T) {
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	def := Definition{Stage: "cutoff", YesAt: 0.8, Columns: []Column{{Name: "open", Kind: Choice, Options: []string{"yes", "no"}}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|open": chose("no", map[string]float64{"no": 1}),
		"s2|open": chose("no", map[string]float64{"no": 1, "yes": 0}),
	})
	if err != nil || result.Answers[0] != nil || result.Answers[1]["open"] != "no" || result.Uncertain(0) {
		t.Fatalf("missing yes repaired as cutoff no: %+v / %v", result, err)
	}
	def = Definition{Stage: "ranked", Ranked: true, Columns: []Column{{Name: "key", Kind: Choice, Options: []string{"yes"}, Optional: true}}}
	for _, invalid := range []llm.Verdict{yes(math.NaN()), yes(math.Inf(1)), yes(-0.1), yes(1.1), {InvalidYes: true}} {
		result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"s1|key": invalid, "s2|key": yes(0)})
		if err != nil || result.Answers[0] != nil || result.Answers[1] == nil || result.Answers[1][ProbabilityCell("key")] != "0.0000" || result.Uncertain(0) {
			t.Fatalf("invalid noul published rank: %+v / %v", result, err)
		}
	}
}

func TestYesAtRequiresAClosedChoiceButStillDecidesFromScores(t *testing.T) {
	def := Definition{Stage: "cutoff", YesAt: 0.8, Columns: []Column{
		{Name: "open", Kind: Choice, Options: []string{"yes", "no"}, Alone: true},
		{Name: "other", Kind: Choice, Options: []string{"A", "B"}},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	for _, choice := range []string{"", "outside"} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("choice=%q/raw=%t", choice, raw), func(t *testing.T) {
				verdicts := map[string]llm.Verdict{
					"s1|open":  chose(choice, map[string]float64{"yes": 0.9, "no": 0.1}),
					"s1|other": chose("A", map[string]float64{"A": 1, "B": 0}),
					"s2|open":  chose(" YES ", map[string]float64{"yes": 0.7, "no": 0.3}),
					"s2|other": chose("A", map[string]float64{"A": 1, "B": 0}),
				}
				if raw {
					encodedChoice, _ := json.Marshal(choice)
					choiceField := `"choice":` + string(encodedChoice) + `,`
					if choice == "" {
						choiceField = ""
					}
					var err error
					verdicts, err = (&typesafe.Client{}).Verdicts([]byte(`{"answers":{"s1|open":{"type":"choice",` + choiceField + `"probabilities":{"yes":0.9,"no":0.1}},"s1|other":{"type":"choice","choice":"A","probabilities":{"A":1,"B":0}},"s2|open":{"type":"choice","choice":" YES ","probabilities":{"yes":0.7,"no":0.3}},"s2|other":{"type":"choice","choice":"A","probabilities":{"A":1,"B":0}}}}`))
					if err != nil {
						t.Fatal(err)
					}
				}
				result, err := DecodeClassifierAnswers(def, window, verdicts)
				if err != nil || result.Answers[0]["open"] != "" || result.Answers[0]["other"] != "A" || result.Answers[1]["open"] != "no" || result.Answers[1]["other"] != "A" || result.Uncertain(0) || len(result.Rejections) != 1 || result.Rejections[0].Cell != "open" {
					t.Fatalf("invalid closed choice accepted or cutoff semantics/neighbours changed: %+v / %v", result, err)
				}
			})
		}
	}
}

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
		"s2|part": chose("none", map[string]float64{"none": 1, "Serving": 0}),
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
	result, _ := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{"p1|role": chose("domain", map[string]float64{"domain": 0.51, "interface": 0.49, "wiring": 0, "support": 0})})
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
		"s1|explains": yes(0.8), "s1|talks": chose("db", map[string]float64{"db": 0.9, "sdk": 0.1, "none of these": 0}),
		"s2|explains": yes(0.2), "s2|talks": chose("none of these", map[string]float64{"none of these": 0.9, "db": 0.1, "sdk": 0}),
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
		if len(call.Prompt.User) > (typesafe.RequestTokenLimit - 1) {
			t.Fatalf("piece body %d bytes", len(call.Prompt.User))
		}
		rows += len(piece.Rows)
	}
	if rows != 150 {
		t.Fatalf("rows %d, want 150", rows)
	}
}

// Provider preparation decides whether each complete row fits. Only an
// oversized row is packed; its neighbours keep their bytes and order. A
// row that still cannot fit is refused locally without reaching transport,
// including when no packed form exists.
func TestFitClassifierWindowsPacksInPlaceAndRefusesOnlyWhatCannotFit(t *testing.T) {
	jev := &typesafe.Client{}
	field := func(id string, bytes int) Row {
		return Row{ID: id, Fields: []Field{{Name: "name", Value: id}, {Name: "uses", Value: strings.Repeat("u", bytes)}}}
	}
	def := closedDefinition()
	def.Pack = func(row Row) Row {
		// A lossless form a quarter the size, recognisable in the request;
		// "mid" has nothing to pack.
		if row.ID == "mid" {
			return row
		}
		return Row{ID: row.ID, Fields: []Field{{Name: "name", Value: row.ID}, {Name: "packed", Value: strings.Repeat("p", len(row.Fields[1].Value.(string))/4)}}}
	}
	window := Window{Context: closedWindow().Context, Rows: []Row{
		field("s1", 100), field("big", (typesafe.QuestionTokenLimit-1)+1000), field("s2", 100),
		field("mid", (typesafe.QuestionTokenLimit-1)*3/2), field("huge", (typesafe.QuestionTokenLimit-1)*5), field("s3", 100),
	}}
	shape := func(fitted []Window) ([]string, []string, []string) {
		var windows, order, refused []string
		for _, piece := range fitted {
			var ids []string
			for _, row := range piece.Rows {
				ids = append(ids, row.ID)
			}
			order = append(order, ids...)
			windows = append(windows, strings.Join(ids, ","))
			if piece.Refused != "" {
				refused = append(refused, piece.Refused)
			}
		}
		return windows, order, refused
	}
	fitted, err := FitClassifierWindows(jev, def, []Window{window})
	if err != nil {
		t.Fatal(err)
	}
	windows, order, refused := shape(fitted)
	if !slices.Equal(order, []string{"s1", "big", "s2", "mid", "huge", "s3"}) || !slices.Equal(windows, []string{"s1,big,s2", "mid", "huge", "s3"}) {
		t.Fatalf("windows %v: rows keep their order, and only a row still over the bound leaves its window", windows)
	}
	if len(refused) != 2 || fitted[2].Refused == "" || !strings.Contains(refused[1], "row huge was not sent: even packed") ||
		!strings.Contains(refused[0], fmt.Sprintf("configured=%d", typesafe.QuestionTokenLimit)) {
		t.Fatalf("refusals %q", refused)
	}
	first := fitted[0]
	if first.Rows[1].Fields[1].Name != "packed" || first.Rows[0].Fields[1].Name != "uses" || first.Rows[2].Fields[1].Name != "uses" {
		t.Fatal("only the row over the bound is packed, in its place")
	}
	if !strings.Contains(string(first.Request), `"packed"`) {
		t.Fatal("the packed window's request was not rebuilt")
	}

	def.Pack = nil
	fitted, err = FitClassifierWindows(jev, def, []Window{window})
	if err != nil {
		t.Fatal(err)
	}
	windows, _, refused = shape(fitted)
	if !slices.Equal(windows, []string{"s1", "big", "s2", "mid", "huge", "s3"}) || len(refused) != 3 || !strings.Contains(refused[2], "row huge was not sent: as built") {
		t.Fatalf("without a packed form: windows %v, refusals %q", windows, refused)
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

func TestRowCatalogueCriteriaAreFactoredWithoutChangingEvidenceOrClosedDecisions(t *testing.T) {
	for _, untyped := range []bool{false, true} {
		t.Run(fmt.Sprintf("untyped=%t", untyped), func(t *testing.T) {
			entries := []map[string]any{
				{"ref": "c1", "title": "Read", "criteria": strings.Repeat("read-source ", 600), "anchor": "storage.c:11"},
				{"ref": "c2", "title": "Write", "criteria": strings.Repeat("write-source ", 600), "anchor": "storage.c:22"},
				{"ref": "c3", "title": "write", "criteria": strings.Repeat("commit-source ", 600), "anchor": "storage.c:33"},
			}
			var catalogue any = entries
			if untyped {
				catalogue = []any{entries[0], entries[1], entries[2]}
			}
			def := Definition{Stage: "orientation_flow", Contract: "flow", System: "Follow the program's work", Classifier: true,
				Columns: []Column{{Name: "next", Kind: Choice, OptionsFrom: "candidates", CriteriaFrom: "criteria", Item: "step"}}}
			window := Window{Rows: []Row{{ID: "t1.n1", Fields: []Field{{Name: "step", Value: "main"}, {Name: "candidates", Value: catalogue}}}}}
			original, _ := json.Marshal(window)
			// The zero-value client supports the real encoder and Prepare, but
			// cannot make an HTTP request.
			client := &typesafe.Client{Model: "jev-1.13.0"}
			call, err := ClassifierCall(client, def, window)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Questions map[string]struct {
					Criteria     map[string]string `json:"criteria"`
					Instructions struct {
						Step struct {
							Name       string           `json:"step"`
							Candidates []map[string]any `json:"candidates"`
						} `json:"step"`
					} `json:"instructions"`
				} `json:"questions"`
			}
			if err := json.Unmarshal([]byte(call.Prompt.User), &body); err != nil {
				t.Fatal(err)
			}
			q := body.Questions["t1.n1|next"]
			if q.Instructions.Step.Name != "main" || len(q.Instructions.Step.Candidates) != 3 || len(q.Criteria) != 3 {
				t.Fatalf("lost original question/closed choices: %+v", q)
			}
			for i, label := range []string{"Read", "c2", "c3"} {
				entry := q.Instructions.Step.Candidates[i]
				if entry["ref"] != entries[i]["ref"] || entry["title"] != entries[i]["title"] || entry["anchor"] != entries[i]["anchor"] || entry["criteria"] != nil {
					t.Fatalf("changed catalogue identity/source fields: %+v", entry)
				}
				text := entries[i]["criteria"].(string)
				if q.Criteria[label] != text || strings.Count(call.Prompt.User, text) != 1 {
					t.Fatalf("criteria not preserved exactly once for %s", label)
				}
			}
			if _, err := client.Prepare(call.Prompt, call.Limits); err != nil {
				t.Fatalf("factored complete question does not fit: %v", err)
			}
			// Reconstruct the former duplicate representation with exactly the
			// same evidence, and verify the actual single-question envelope.
			var duplicated map[string]any
			_ = json.Unmarshal([]byte(call.Prompt.User), &duplicated)
			questions := duplicated["questions"].(map[string]any)
			instructions := questions["t1.n1|next"].(map[string]any)["instructions"].(map[string]any)
			instructions["step"] = fieldsMap(window.Rows[0].Fields)
			raw, _ := json.Marshal(duplicated)
			oldPrompt := call.Prompt
			oldPrompt.User = string(raw)
			if _, err := client.Prepare(oldPrompt, call.Limits); !classifierInputRefused(err) {
				t.Fatalf("duplicate form should exceed the real envelope: %v", err)
			}
			result, err := call.DecodeValidate([]byte(`{"answers":{"t1.n1|next":{"type":"choice","choice":"c3","probabilities":{"Read":0.1,"c2":0.1,"c3":0.8}}}}`))
			if err != nil || result.Answers[0]["next"] != "c3" {
				t.Fatalf("original closed ref was not decoded: %+v %v", result, err)
			}
			after, _ := json.Marshal(window)
			if string(original) != string(after) {
				t.Fatal("request factoring mutated the original window")
			}
		})
	}
}

func TestRowCatalogueKeepsCriteriaNotActuallyCarriedByTheOptions(t *testing.T) {
	def := Definition{Stage: "classifier", Contract: "c", System: "s", Columns: []Column{{Name: "part", Kind: Choice,
		OptionsFrom: "candidates", CriteriaFrom: "criteria", Criteria: map[string]llm.Criteria{"c1": {What: "Explicit decision criterion"}}}}}
	window := Window{Rows: []Row{{ID: "n1", Fields: []Field{{Name: "candidates", Value: []map[string]any{
		{"ref": "c1", "title": "Serving", "criteria": "Original source evidence", "anchor": "server.c:8"},
	}}}}}}
	call, err := ClassifierCall(&typesafe.Client{}, def, window)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(call.Prompt.User, "Original source evidence") || !strings.Contains(call.Prompt.User, "Explicit decision criterion") || !strings.Contains(call.Prompt.User, "server.c:8") {
		t.Fatalf("factoring erased independent source or explicit criterion: %s", call.Prompt.User)
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

// Two independent decisions about one row: a near-tie on an Alone column
// leaves that cell unanswered and recorded, and the row keeps the decision
// that was clear. A row with no decision at all is still unanswered.
func TestAnAloneClassifierColumnFailsByItself(t *testing.T) {
	def := Definition{Stage: "atlas_alone", Contract: "c", System: "s", Columns: []Column{
		{Name: "binds", Kind: Choice, Options: []string{"request", "none"}, Alone: true},
		{Name: "publishes", Kind: Choice, Options: []string{"serves", "none"}, Alone: true},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|binds":     chose("request", map[string]float64{"request": 0.95, "none": 0.05}),
		"s1|publishes": chose("none", map[string]float64{"none": 0.52, "serves": 0.48}),
		"s2|binds":     chose("none", map[string]float64{"none": 0.5, "request": 0.5}),
		"s2|publishes": chose("none", map[string]float64{"none": 0.51, "serves": 0.49}),
	})
	if err != nil || result.Answers[0]["binds"] != "request" || len(result.Answers[0]) != 1 || result.Answers[1] != nil {
		t.Fatalf("a near-tie on one decision cost the row its other: %+v %v", result, err)
	}
	if len(result.Rejections) != 2 || result.Rejections[0].Cell != "publishes" || result.Rejections[1].Cell != "" || slices.Contains(result.AcceptedRowKeys(), "s1") {
		t.Fatalf("the refused cell or row was not recorded: %+v", result.Rejections)
	}
}

// A remembered answer is the answer to one question: a classifier row's
// memo basis pins each question's wording, its item and every option's
// criteria, which the request serializes only to the categorizer. A table
// that asks none of them keeps the basis it had.
func TestMemoBasisChangesWithAnOptionsCriteria(t *testing.T) {
	criteria := map[string]llm.Criteria{
		"none":    {What: "no entry", NotFor: "a way in", Includes: "printed text", Examples: []string{"a format"}},
		"command": {What: "what a person gives", NotFor: "a request", Includes: "an option", Examples: []string{"--verbose"}},
	}
	def := Definition{Stage: "atlas_api", Contract: "repomap.atlas.api.v7", System: "task", Classifier: true, Memoize: true,
		Columns: []Column{{Name: "enters", Kind: Choice, Options: []string{"command", "none"}, Criteria: criteria, Item: "outside_symbol", Ask: "What do the words become?"}}}
	window := Window{Rows: []Row{{ID: "sym1", Fields: []Field{{Name: "symbol", Value: "flag.Bool"}}}}}
	basis := func(def Definition) string {
		t.Helper()
		id, err := MemoIdentity(&typesafe.Client{Model: "jev-1.13.0"}, def, window)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	original := basis(def)
	reworded := def
	reworded.Columns = []Column{def.Columns[0]}
	changed := map[string]llm.Criteria{"none": criteria["none"], "command": {What: "what a person types", NotFor: "a request", Includes: "an option", Examples: []string{"--verbose"}}}
	reworded.Columns[0].Criteria = changed
	if basis(reworded) == original {
		t.Fatal("a changed option's criteria kept the remembered answer")
	}
	reworded.Columns[0].Criteria, reworded.Columns[0].Ask = criteria, "What does the call become?"
	if basis(reworded) == original {
		t.Fatal("a changed question kept the remembered answer")
	}
	plain := Definition{Stage: "atlas_symbols", Contract: "repomap.atlas.symbol-selection.v9", System: "task", Classifier: true, Memoize: true,
		Columns: []Column{{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}, Note: "a declaration to look at first"}}}
	if questions, err := questionsDigest(plain); err != nil || questions != "" {
		t.Fatalf("a table that asks no worded question pins %q: %v", questions, err)
	}
}
