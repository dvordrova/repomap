package table

import (
	"encoding/json"
	"fmt"
	"slices"

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

// yesOnly is an optional column whose only value is yes: a yes/no question.
func yesOnly(column Column, options []string) bool {
	return column.Optional && len(options) == 1 && options[0] == "yes"
}

func columnQuestion(column Column) string {
	if column.Note != "" {
		return fmt.Sprintf("`%s` for `row`, as `task` defines it: %s.", column.Name, column.Note)
	}
	return fmt.Sprintf("`%s` for `row`, as `task` defines it.", column.Name)
}

// ClassifierCall is Call for a decision model: the system prompt and the
// window context become the state, each row and column one choice question.
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
			instructions := map[string]any{"row": fieldsMap(row.Fields), "question": columnQuestion(column)}
			if yesOnly(column, options) {
				questions[questionKey(row, column)] = map[string]any{"type": "noul", "instructions": instructions}
				continue
			}
			criteria := make(map[string]any, len(options)+1)
			for _, option := range options {
				criteria[option] = nil
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
	}{def.Contract + ".classifier", sha256Hex([]byte(def.System)), sha256Hex(body), minProbability})
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

// DecodeClassifier accepts a row when every column's choice is a listed
// option at or above minProbability; other rows are refused on their own.
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
	result := Result{Answers: make(Answers, len(window.Rows)), independent: true, rowKeys: make([]string, len(window.Rows))}
	accepted := 0
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
		answer := make(Answer, len(def.Columns))
		reason := ""
		for _, column := range def.Columns {
			got, ok := envelope.Answers[questionKey(row, column)]
			options := columnOptions(column, window.Context, row)
			if ok && yesOnly(column, options) {
				if got.Type != "noul" || got.Noul == nil {
					reason = fmt.Sprintf("column %s was not answered", column.Name)
					break
				}
				if *got.Noul > minProbability {
					answer[column.Name] = "yes"
				}
				continue
			}
			switch {
			case ok && column.Optional && got.Choice == classifierAbsent:
				continue
			case !ok || got.Type != "choice":
				reason = fmt.Sprintf("column %s was not answered", column.Name)
			case !slices.Contains(options, got.Choice):
				reason = fmt.Sprintf("column %s chose %q, not one of the options", column.Name, got.Choice)
			case got.Probabilities[got.Choice] <= minProbability:
				if column.Optional {
					continue
				}
				reason = fmt.Sprintf("column %s chose %q with probability %.2f, not above %.2f", column.Name, got.Choice, got.Probabilities[got.Choice], minProbability)
			default:
				answer[column.Name] = got.Choice
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
