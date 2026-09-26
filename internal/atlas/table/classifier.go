package table

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// A closed table asks only closed choices: every column picks one listed
// option. Such a table can be answered by a decision model instead of a
// text model. The rows, context and options are the same; only the wire
// form and the validation differ.
const (
	// ClassifierQuestions packs a closed table's windows for a decision
	// model: every row and column becomes its own question, and the shared
	// context is sent once as state. A request of 150 rows of seven columns
	// (1,050 questions) exceeded the model's 64k-token request. Bytes pack
	// by the ordinary target, and an oversized row still goes whole in its
	// own request: a model refusal leaves it explicitly unanswered instead of
	// failing the run.
	ClassifierQuestions = 150
	// ClassifierConcurrency is how many decision requests run at once. A
	// 150-question request takes ~3 s whatever the load: 26 of them took 23 s
	// four at a time and 5.7 s all at once; 57 key-selection requests took
	// 20.8 s 24 at a time and 12 s all at once, with no refusal. 64 stays far
	// inside 1,200 requests a minute; a 429 still backs off.
	ClassifierConcurrency = 64
	// ClassifierMargin is how far the chosen option's probability must lead
	// every other listed option's for a choice to be taken; a closer answer
	// is left explicitly uncertain instead of guessed. It replaced an
	// absolute 0.50 floor (owner, 2026-09-26), which refused "support" at
	// 0.49 against 0.32 yet took 0.51 against 0.49. In the saved Jev role
	// answers (60 over five roles, plus that 0.49) the leads run 0.00,
	// 0.02, then 0.17, 0.21 and up: 0.10 sits in the middle of that gap,
	// clear of both the near-ties and the owner's example. On 1,072 saved
	// eleven-option part choices it takes 101 the floor refused and leaves
	// uncertain the 9 the floor took with leads of 0.02 to 0.09.
	// Jev's confidence, which TypeSafe's guidance gates on, cannot serve:
	// for two options it is this lead, for more (n*top-1)/(n-1), blind to
	// the runner-up.
	ClassifierMargin = 0.1
)

// Closed reports whether every column is an unconditional closed choice.
func Closed(def Definition) bool {
	if len(def.Columns) == 0 {
		return false
	}
	for _, column := range def.Columns {
		if column.Kind != Choice || column.Free != "" || column.WhenOptionsFrom != "" {
			return false
		}
	}
	return true
}

// ClassifierBodyBytes bounds one decision-model request body: at about 0.38
// tokens a byte it stays well inside the model's 64k-token request. Byte
// packing measures the text-model request, which lacks each question's
// options; 150 questions naming 18 part titles each exceeded the budget.
const ClassifierBodyBytes = 120_000

// FitClassifierWindows halves any window whose decision-model body exceeds
// ClassifierBodyBytes until it fits or holds one row; nothing is dropped.
func FitClassifierWindows(def Definition, windows []Window) ([]Window, error) {
	var fitted []Window
	var fit func(Window) error
	fit = func(window Window) error {
		call, err := ClassifierCall(def, window)
		if err != nil {
			return err
		}
		if len(call.Prompt.User) <= ClassifierBodyBytes || len(window.Rows) < 2 {
			fitted = append(fitted, window)
			return nil
		}
		half := len(window.Rows) / 2
		for _, rows := range [][]Row{window.Rows[:half], window.Rows[half:]} {
			piece := window
			piece.Rows = append([]Row(nil), rows...)
			request, err := Request(def, piece)
			if err != nil {
				return err
			}
			piece.Request = request
			if err := fit(piece); err != nil {
				return err
			}
		}
		return nil
	}
	for _, window := range windows {
		if err := fit(window); err != nil {
			return nil, err
		}
	}
	return fitted, nil
}

// ForClassifier packs a closed table for a decision model.
func ForClassifier(def Definition) Definition {
	def.Window = max(1, ClassifierQuestions/max(1, len(def.Columns)))
	def.MaxInputBytes = 0
	return def
}

func fieldsMap(fields []Field) map[string]any {
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		values[field.Name] = field.Value
	}
	return values
}

func columnOptions(column Column, context []Field, row Row) []string {
	options := append([]string(nil), column.Options...)
	if column.OptionsFrom == "" {
		return options
	}
	for _, fields := range [][]Field{row.Fields, context} {
		for _, field := range fields {
			if field.Name != column.OptionsFrom {
				continue
			}
			switch list := field.Value.(type) {
			case []string:
				return append(options, list...)
			case []any:
				for _, item := range list {
					options = append(options, fmt.Sprint(item))
				}
				return options
			}
		}
	}
	return options
}

func questionKey(row Row, column Column) string {
	return row.ID + "|" + column.Name
}

// classifierAbsent is the explicit option of an optional column: the row
// has no value for it. Without it a decision model must pick a listed value.
const classifierAbsent = "none of these"

// ClassifierNoulMargin is how far from 0.5 a yes/no probability must be to
// be taken either way; in between the row stays explicitly unanswered. A
// noul is one probability, not a choice, so ClassifierMargin does not apply.
const ClassifierNoulMargin = 0.1

// yesOnly is an optional column whose only value is yes: a yes/no question.
func yesOnly(column Column, options []string) bool {
	return column.Optional && len(options) == 1 && options[0] == "yes"
}

// optionNames gives each closed ref the title its catalogue shows, so the
// decision model chooses among meanings rather than opaque refs. A
// catalogue is a context or row list of objects with a ref; refs without a
// unique title keep their ref.
type optionNames struct {
	label map[string]string // ref -> shown name
	ref   map[string]string // shown name -> ref
	list  string            // the catalogue's field name
}

func namesFor(column Column, context []Field, row Row, options []string) optionNames {
	names := optionNames{label: map[string]string{}, ref: map[string]string{}}
	catalogue := func(value any) []map[string]any {
		switch list := value.(type) {
		case []map[string]any:
			return list
		case []any:
			var entries []map[string]any
			for _, item := range list {
				if entry, ok := item.(map[string]any); ok {
					entries = append(entries, entry)
				}
			}
			return entries
		}
		return nil
	}
	if column.OptionsFrom != "" {
		for _, fields := range [][]Field{row.Fields, context} {
			for _, field := range fields {
				entries := catalogue(field.Value)
				if len(entries) == 0 || entries[0]["ref"] == nil {
					continue
				}
				titles := map[string]string{}
				seen := map[string]int{}
				for _, entry := range entries {
					ref, _ := entry["ref"].(string)
					title, _ := entry["title"].(string)
					if ref != "" && title != "" {
						titles[ref] = title
						seen[strings.ToLower(title)]++
					}
				}
				for ref, title := range titles {
					if seen[strings.ToLower(title)] == 1 && !slices.Contains(options, title) {
						names.label[ref] = title
					}
				}
				names.list = field.Name
				break
			}
			if names.list != "" {
				break
			}
		}
	}
	for _, option := range options {
		if names.label[option] == "" {
			names.label[option] = option
		}
		names.ref[names.label[option]] = option
	}
	return names
}

func columnQuestion(column Column, names optionNames) string {
	if column.Ask != "" {
		return column.Ask
	}
	if names.list != "" {
		return fmt.Sprintf("Which of `context.%s` does `row` belong to, as `task` defines `%s`?", names.list, column.Name)
	}
	if column.Note != "" {
		return fmt.Sprintf("`%s` for `row`, as `task` defines it: %s.", column.Name, column.Note)
	}
	return fmt.Sprintf("`%s` for `row`, as `task` defines it.", column.Name)
}

// ClassifierCall is Call for a decision model: the system prompt and the
// window context become the state, each row and column one question.
func ClassifierCall(def Definition, window Window) (llm.Call[Result], error) {
	if !Closed(def) {
		return llm.Call[Result]{}, fmt.Errorf("table %s: not a closed table", def.Stage)
	}
	questions := make(map[string]any, len(window.Rows)*len(def.Columns))
	for _, row := range window.Rows {
		for _, column := range def.Columns {
			options := columnOptions(column, window.Context, row)
			if len(options) == 0 {
				return llm.Call[Result]{}, fmt.Errorf("table %s: row %s column %s has no options", def.Stage, row.ID, column.Name)
			}
			names := namesFor(column, window.Context, row, options)
			instructions := map[string]any{"row": fieldsMap(row.Fields), "question": columnQuestion(column, names)}
			if yesOnly(column, options) {
				questions[questionKey(row, column)] = map[string]any{"type": "noul", "instructions": instructions}
				continue
			}
			criteria := make(map[string]any, len(options)+1)
			// A catalogue's purposes are already in the state; repeating them
			// in every question multiplied a request past the model's budget.
			for _, option := range options {
				criteria[names.label[option]] = nil
			}
			if column.Optional {
				criteria[classifierAbsent] = "No listed option applies to this row."
			}
			questions[questionKey(row, column)] = map[string]any{"type": "choice", "instructions": instructions, "criteria": criteria}
		}
	}
	evaluated := map[string]any{"task": def.System, "context": fieldsMap(window.Context)}
	if def.ClassifierOmitTask {
		evaluated = map[string]any{"context": fieldsMap(window.Context)}
	}
	body, err := json.Marshal(map[string]any{
		"state":     evaluated,
		"questions": questions,
	})
	if err != nil {
		return llm.Call[Result]{}, err
	}
	state, err := json.Marshal(struct {
		Contract string  `json:"contract"`
		Prompt   string  `json:"prompt_sha256"`
		Request  string  `json:"request_sha256"`
		Margin   float64 `json:"margin"`
		YesAt    float64 `json:"yes_at,omitempty"`
	}{def.Contract + ".classifier.v2", sha256Hex([]byte(def.System)), sha256Hex(body), ClassifierMargin, def.YesAt})
	if err != nil {
		return llm.Call[Result]{}, err
	}
	return llm.Call[Result]{
		State:  state,
		Prompt: llm.Prompt{User: string(body), NoResponseAdjunct: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1},
		DecodeValidate: func(raw []byte) (Result, error) {
			return DecodeClassifier(def, window, raw)
		},
	}, nil
}

// DecodeClassifier accepts a row when every column is decided: a listed
// option, or an optional column's explicit "none of these", leading every
// other listed option by ClassifierMargin, or a yes/no clear of the
// uncertain band. Anything else leaves the row explicitly unanswered, never
// silently absent; other rows stand alone.
func DecodeClassifier(def Definition, window Window, raw []byte) (Result, error) {
	answers, err := ParseClassifierAnswers(raw)
	if err != nil {
		return Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	return DecodeClassifierAnswers(def, window, answers)
}

// ClassifierAnswer is one decision-model answer as the response carries it.
type ClassifierAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          *float64           `json:"noul"`
}

// ParseClassifierAnswers reads a decision-model response once, so rows
// recalled from one remembered response need not parse it again each. Each
// answer is read on its own: a malformed one leaves only its question not
// answered. A response without answers decided nothing.
func ParseClassifierAnswers(raw []byte) (map[string]ClassifierAnswer, error) {
	var envelope struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Answers == nil {
		return nil, fmt.Errorf("response has no answers")
	}
	answers := make(map[string]ClassifierAnswer, len(envelope.Answers))
	for key, rawAnswer := range envelope.Answers {
		var answer ClassifierAnswer
		if json.Unmarshal(rawAnswer, &answer) == nil {
			answers[key] = answer
		}
	}
	return answers, nil
}

// DecodeClassifierAnswers is DecodeClassifier over already parsed answers.
// A window whose every row was answered, even uncertainly, is an explicit
// answer: its uncertain rows stay unanswered and it is not asked again. A
// window where no row was accepted and some row was not answered, or was
// answered outside its options, is refused.
func DecodeClassifierAnswers(def Definition, window Window, answers map[string]ClassifierAnswer) (Result, error) {
	envelope := struct{ Answers map[string]ClassifierAnswer }{answers}
	yesAt := 0.5 + ClassifierNoulMargin
	result := Result{Answers: make(Answers, len(window.Rows)), rowKeys: make([]string, len(window.Rows))}
	accepted, uncertain := 0, 0
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
		answer := make(Answer, len(def.Columns))
		reason := ""
		unsure := false
		for _, column := range def.Columns {
			got, ok := envelope.Answers[questionKey(row, column)]
			options := columnOptions(column, window.Context, row)
			if yesOnly(column, options) {
				if def.Ranked && ok && got.Type == "noul" && got.Noul != nil {
					answer[ProbabilityCell(column.Name)] = strconv.FormatFloat(*got.Noul, 'f', 4, 64)
					if *got.Noul >= yesAt {
						answer[column.Name] = "yes"
					}
					continue
				}
				switch {
				case !ok || got.Type != "noul" || got.Noul == nil:
					reason = fmt.Sprintf("column %s was not answered", column.Name)
				case *got.Noul >= yesAt:
					answer[column.Name] = "yes"
					continue
				case *got.Noul <= 1-yesAt:
					continue
				default:
					reason, unsure = fmt.Sprintf("column %s is uncertain: yes at %.2f", column.Name, *got.Noul), true
				}
				break
			}
			if def.YesAt > 0 && len(options) == 2 && slices.Contains(options, "yes") && slices.Contains(options, "no") {
				if !ok || got.Type != "choice" || got.Probabilities == nil {
					reason = fmt.Sprintf("column %s was not answered", column.Name)
					break
				}
				answer[column.Name] = "no"
				if got.Probabilities["yes"] >= def.YesAt {
					answer[column.Name] = "yes"
				}
				continue
			}
			names := namesFor(column, window.Context, row, options)
			labels := make([]string, 0, len(options)+1)
			for _, option := range options {
				labels = append(labels, names.label[option])
			}
			if column.Optional {
				labels = append(labels, classifierAbsent)
			}
			probability := got.Probabilities[got.Choice]
			rival, rivalAt := runnerUp(got, labels)
			switch {
			case !ok || got.Type != "choice":
				reason = fmt.Sprintf("column %s was not answered", column.Name)
			case !slices.Contains(labels, got.Choice):
				reason = fmt.Sprintf("column %s chose %q, not one of the options", column.Name, got.Choice)
			// Jev's probabilities are hundredths carried as floats: 0.30
			// against 0.20 leads by 0.0999…, which is the margin.
			case probability-rivalAt < ClassifierMargin-1e-9:
				reason, unsure = fmt.Sprintf("column %s is uncertain: %q at %.2f against %q at %.2f, a lead under %.2f", column.Name, got.Choice, probability, rival, rivalAt, ClassifierMargin), true
			case got.Choice == classifierAbsent:
				continue
			default:
				answer[column.Name] = names.ref[got.Choice]
				continue
			}
			break
		}
		if reason != "" {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Reason: reason})
			if unsure {
				uncertain++
			}
			continue
		}
		result.Answers[i] = answer
		accepted++
	}
	if accepted == 0 && uncertain < len(window.Rows) {
		return Result{}, fmt.Errorf("table %s: no rows accepted; %s", def.Stage, result.Rejections[0].Reason)
	}
	return result, nil
}

// runnerUp is the listed option, other than the chosen one, that the answer
// gives the highest probability; the first listed wins a tie. A choice that
// is not the top option therefore has a negative lead.
func runnerUp(got ClassifierAnswer, labels []string) (string, float64) {
	rival, rivalAt := "", 0.0
	for _, label := range labels {
		if at := got.Probabilities[label]; label != got.Choice && (rival == "" || at > rivalAt) {
			rival, rivalAt = label, at
		}
	}
	return rival, rivalAt
}
