package table

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// A closed table asks only closed choices: every column picks one listed
// option. Such a table can be answered by a decision model instead of a
// text model. The rows, context and options are the same; only the wire
// form and the validation differ.
const (
	// ClassifierQuestions and ClassifierInputBytes pack a closed table's
	// windows for a decision model: every row and column becomes its own
	// question, and the shared context is sent once as state. A request of
	// 150 rows of seven columns (1,050 questions) exceeded the model's
	// 64k-token request.
	ClassifierQuestions  = 150
	ClassifierInputBytes = 90_000
	// ClassifierConcurrency is how many decision requests run at once. A
	// 150-question request takes ~3 s whatever the load: 26 of them took 23 s
	// four at a time and 5.7 s all at once, far inside 1,200 requests/min.
	ClassifierConcurrency = 24
	// ClassifierMinProbability is the probability of the chosen option
	// below which a choice is not taken: the cell is left unanswered instead
	// of guessed. It is not the model's confidence, which measures how
	// concentrated the whole distribution is: a yes at 0.69 has confidence
	// 0.39.
	ClassifierMinProbability = 0.5
)

// Closed reports whether every column is an unconditional closed choice.
func Closed(def Definition) bool {
	if len(def.Columns) == 0 {
		return false
	}
	for _, column := range def.Columns {
		if column.Kind != Choice || column.Free != "" || len(column.When) > 0 || column.WhenOptionsFrom != "" {
			return false
		}
	}
	return true
}

// MinProbabilityOf is the table's acceptance floor for a decision model.
func MinProbabilityOf(def Definition) float64 {
	if def.MinProbability > 0 {
		return def.MinProbability
	}
	return ClassifierMinProbability
}

// ForClassifier packs a closed table for a decision model.
func ForClassifier(def Definition) Definition {
	def.Window = max(1, ClassifierQuestions/max(1, len(def.Columns)))
	def.MaxInputBytes = ClassifierInputBytes
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
// be taken either way; in between the row stays explicitly unanswered.
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
	label, purpose map[string]string // ref -> shown name, ref -> purpose
	ref            map[string]string // shown name -> ref
	list           string            // the catalogue's field name
}

func namesFor(column Column, context []Field, row Row, options []string) optionNames {
	names := optionNames{label: map[string]string{}, purpose: map[string]string{}, ref: map[string]string{}}
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
						if purpose, _ := entry["purpose"].(string); purpose != "" {
							names.purpose[ref] = purpose
						}
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
func ClassifierCall(def Definition, window Window, minProbability float64) (llm.Call[Result], error) {
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
			for _, option := range options {
				if purpose := names.purpose[option]; purpose != "" {
					criteria[names.label[option]] = purpose
				} else {
					criteria[names.label[option]] = nil
				}
			}
			if column.Optional {
				criteria[classifierAbsent] = "No listed option applies to this row."
			}
			questions[questionKey(row, column)] = map[string]any{"type": "choice", "instructions": instructions, "criteria": criteria}
		}
	}
	body, err := json.Marshal(map[string]any{
		"state":     map[string]any{"task": def.System, "context": fieldsMap(window.Context)},
		"questions": questions,
	})
	if err != nil {
		return llm.Call[Result]{}, err
	}
	state, err := json.Marshal(struct {
		Contract       string  `json:"contract"`
		Prompt         string  `json:"prompt_sha256"`
		Request        string  `json:"request_sha256"`
		MinProbability float64 `json:"min_probability"`
		YesAt          float64 `json:"yes_at,omitempty"`
	}{def.Contract + ".classifier.v2", sha256Hex([]byte(def.System)), sha256Hex(body), minProbability, def.YesAt})
	if err != nil {
		return llm.Call[Result]{}, err
	}
	return llm.Call[Result]{
		State:  state,
		Prompt: llm.Prompt{User: string(body), NoResponseAdjunct: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1},
		DecodeValidate: func(raw []byte) (Result, error) {
			return DecodeClassifier(def, window, raw, minProbability)
		},
	}, nil
}

// DecodeClassifier accepts a row when every column is decided: a listed
// option above minProbability, an explicit "none of these" for an optional
// column, or a yes/no clear of the uncertain band. Anything else leaves the
// row explicitly unanswered, never silently absent; other rows stand alone.
func DecodeClassifier(def Definition, window Window, raw []byte, minProbability float64) (Result, error) {
	var envelope struct {
		Answers map[string]struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Probabilities map[string]float64 `json:"probabilities"`
			Noul          *float64           `json:"noul"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Answers == nil {
		return Result{}, fmt.Errorf("table %s: response has no answers", def.Stage)
	}
	yesAt := max(minProbability, 0.5+ClassifierNoulMargin)
	result := Result{Answers: make(Answers, len(window.Rows)), independent: true, rowKeys: make([]string, len(window.Rows))}
	accepted := 0
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
		answer := make(Answer, len(def.Columns))
		reason := ""
		for _, column := range def.Columns {
			got, ok := envelope.Answers[questionKey(row, column)]
			options := columnOptions(column, window.Context, row)
			if yesOnly(column, options) {
				switch {
				case !ok || got.Type != "noul" || got.Noul == nil:
					reason = fmt.Sprintf("column %s was not answered", column.Name)
				case *got.Noul >= yesAt:
					answer[column.Name] = "yes"
					continue
				case *got.Noul <= 1-yesAt:
					continue
				default:
					reason = fmt.Sprintf("column %s is uncertain: yes at %.2f", column.Name, *got.Noul)
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
			probability := got.Probabilities[got.Choice]
			switch {
			case !ok || got.Type != "choice":
				reason = fmt.Sprintf("column %s was not answered", column.Name)
			case column.Optional && got.Choice == classifierAbsent && probability > minProbability:
				continue
			case got.Choice != classifierAbsent && names.ref[got.Choice] == "":
				reason = fmt.Sprintf("column %s chose %q, not one of the options", column.Name, got.Choice)
			case probability <= minProbability:
				reason = fmt.Sprintf("column %s is uncertain: %q at %.2f, not above %.2f", column.Name, got.Choice, probability, minProbability)
			default:
				answer[column.Name] = names.ref[got.Choice]
				continue
			}
			break
		}
		if reason != "" {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Reason: reason})
			continue
		}
		result.Answers[i] = answer
		accepted++
	}
	if accepted == 0 {
		return Result{}, fmt.Errorf("table %s: no rows accepted; %s", def.Stage, result.Rejections[0].Reason)
	}
	return result, nil
}
