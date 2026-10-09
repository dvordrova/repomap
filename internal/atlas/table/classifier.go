package table

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// A closed table asks only closed choices: every column picks one listed
// option. The run's categorizer (llm.Categorizer, Jev today) answers such a
// table instead of a text model. The rows, context and options are the same;
// the categorizer writes the request and reads the verdicts, and the
// decision rule below turns them into answers.
const (
	// ClassifierQuestions packs a closed table's windows for the
	// categorizer: every row and column becomes its own question, and the shared
	// context is sent once as state. A request of 150 rows of seven columns
	// (1,050 questions) exceeded the model's 64k-token request. Preparation packs
	// complete rows before transport; an indivisible oversized row stays
	// explicitly unanswered without taking its independent neighbours.
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

// ClassifierNeedsPartition asks the actual provider to prepare the complete
// request. Provider limits belong to that client, not to table-specific guesses.
func ClassifierNeedsPartition(c llm.Categorizer, def Definition, window Window) (bool, error) {
	err := prepareClassifier(c, def, window)
	if classifierInputRefused(err) {
		return true, nil
	}
	return false, err
}

func prepareClassifier(c llm.Categorizer, def Definition, window Window) error {
	call, err := ClassifierCall(c, def, window)
	if err != nil {
		return err
	}
	prepared, err := llm.Prepare(c, call.Prompt, call.Limits)
	if err != nil {
		return err
	}
	if call.Limits.MaxRequestBytes > 0 && prepared.Len() > call.Limits.MaxRequestBytes {
		return llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes,
			Limit: call.Limits.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true})
	}
	return nil
}

func classifierInputRefused(err error) bool {
	var resource *llm.ResourceLimitError
	return errors.As(err, &resource) && (resource.Kind == llm.ResourceLimitContextTokens || resource.Kind == llm.ResourceLimitRequestBytes)
}

// FitClassifierWindows preserves every row and its complete shared context.
// Preparation is the provider's. The owner may pack an oversized row losslessly;
// an indivisible refusal stays beside accepted neighbours without transport.
func FitClassifierWindows(c llm.Categorizer, def Definition, windows []Window) ([]Window, error) {
	var fitted []Window
	packed := map[string]bool{}
	piece := func(window Window, rows []Row) (Window, error) {
		window.Rows = rows
		request, err := Request(def, window)
		window.Request = request
		return window, err
	}
	var fit func(Window) error
	fit = func(window Window) error {
		err := prepareClassifier(c, def, window)
		if err == nil {
			fitted = append(fitted, window)
			return nil
		}
		if !classifierInputRefused(err) {
			return err
		}
		if len(window.Rows) <= 1 {
			form := "as built"
			if len(window.Rows) == 1 && packed[window.Rows[0].ID] {
				form = "even packed"
			}
			id := ""
			if len(window.Rows) == 1 {
				id = window.Rows[0].ID
			}
			window.Refused = fmt.Sprintf("row %s was not sent: %s, provider preparation refused the complete request: %v; partition the complete owning task", id, form, err)
			fitted = append(fitted, window)
			return nil
		}
		half := len(window.Rows) / 2
		for _, rows := range [][]Row{window.Rows[:half], window.Rows[half:]} {
			next, err := piece(window, rows)
			if err != nil {
				return err
			}
			if err := fit(next); err != nil {
				return err
			}
		}
		return nil
	}
	for _, window := range windows {
		err := prepareClassifier(c, def, window)
		if err == nil {
			fitted = append(fitted, window)
			continue
		}
		if !classifierInputRefused(err) {
			return nil, err
		}
		rows := slices.Clone(window.Rows)
		refused := make([]bool, len(rows))
		for i, row := range rows {
			alone, err := piece(window, []Row{row})
			if err != nil {
				return nil, err
			}
			err = prepareClassifier(c, def, alone)
			if err != nil && !classifierInputRefused(err) {
				return nil, err
			}
			if classifierInputRefused(err) && def.Pack != nil {
				rows[i], packed[row.ID] = def.Pack(row), true
				alone, err = piece(window, []Row{rows[i]})
				if err != nil {
					return nil, err
				}
				err = prepareClassifier(c, def, alone)
				if err != nil && !classifierInputRefused(err) {
					return nil, err
				}
			}
			refused[i] = classifierInputRefused(err)
		}
		// Separate only genuinely indivisible rows; contiguous accepted
		// neighbours retain their order and their complete evidence.
		start := 0
		emit := func(end int) error {
			if start == end {
				return nil
			}
			next, err := piece(window, rows[start:end])
			if err != nil {
				return err
			}
			return fit(next)
		}
		for i, over := range refused {
			if !over {
				continue
			}
			if err := emit(i); err != nil {
				return nil, err
			}
			start = i
			if err := emit(i + 1); err != nil {
				return nil, err
			}
			start = i + 1
		}
		if err := emit(len(rows)); err != nil {
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

// classifierItem factors only text carried verbatim by the closed options.
// Keep catalogue identities and any other source fields beside the question;
// the original row remains unchanged for decoding and persistence.
func classifierItem(row Row, column Column, names optionNames, options []llm.Option) map[string]any {
	values := fieldsMap(row.Fields)
	if column.OptionsFrom == "" || column.CriteriaFrom == "" || column.CriteriaFrom == "ref" || column.CriteriaFrom == "title" || names.list != column.OptionsFrom {
		return values
	}
	meanings := map[string]string{}
	for _, option := range options {
		if option.Criteria == nil && option.Meaning != "" {
			meanings[option.Name] = option.Meaning
		}
	}
	entry := func(original map[string]any) map[string]any {
		ref, _ := original["ref"].(string)
		text, _ := original[column.CriteriaFrom].(string)
		label := names.label[ref]
		if text == "" || names.ref[label] != ref || meanings[label] != text {
			return original
		}
		copy := make(map[string]any, len(original)-1)
		for key, value := range original {
			if key != column.CriteriaFrom {
				copy[key] = value
			}
		}
		return copy
	}
	switch list := values[column.OptionsFrom].(type) {
	case []map[string]any:
		copy := make([]map[string]any, len(list))
		for i, value := range list {
			copy[i] = entry(value)
		}
		values[column.OptionsFrom] = copy
	case []any:
		copy := slices.Clone(list)
		for i, value := range list {
			if object, ok := value.(map[string]any); ok {
				copy[i] = entry(object)
			}
		}
		values[column.OptionsFrom] = copy
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
			case []map[string]any:
				// A catalogue of objects offers its entries' refs.
				for _, entry := range list {
					if ref, _ := entry["ref"].(string); ref != "" {
						options = append(options, ref)
					}
				}
				return options
			case []any:
				for _, item := range list {
					if entry, ok := item.(map[string]any); ok {
						if ref, _ := entry["ref"].(string); ref != "" {
							options = append(options, ref)
						}
						continue
					}
					options = append(options, fmt.Sprint(item))
				}
				return options
			}
		}
	}
	return options
}

// classifierContext is the window context a decision model shares across
// its questions: every field but a catalogue a column reads its options and
// their criteria from, which each question already carries.
func classifierContext(def Definition, context []Field) map[string]any {
	values := fieldsMap(context)
	for _, column := range def.Columns {
		if column.CriteriaFrom != "" && column.OptionsFrom != "" {
			delete(values, column.OptionsFrom)
		}
	}
	return values
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
	// criteria is, by ref, the text of the catalogue entry's CriteriaFrom
	// field.
	criteria map[string]string
}

func namesFor(column Column, context []Field, row Row, options []string) optionNames {
	names := optionNames{label: map[string]string{}, ref: map[string]string{}, criteria: map[string]string{}}
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
					if text, _ := entry[column.CriteriaFrom].(string); ref != "" && column.CriteriaFrom != "" && text != "" {
						names.criteria[ref] = text
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

// ClassifierCall is Call for the categorizer: the system prompt is the task,
// the window context is shared, and each row and column is one question.
func ClassifierCall(c llm.Categorizer, def Definition, window Window) (llm.Call[Result], error) {
	if !Closed(def) {
		return llm.Call[Result]{}, fmt.Errorf("table %s: not a closed table", def.Stage)
	}
	questions := make(map[string]llm.Question, len(window.Rows)*len(def.Columns))
	for _, row := range window.Rows {
		for _, column := range def.Columns {
			options := columnOptions(column, window.Context, row)
			if len(options) == 0 {
				return llm.Call[Result]{}, fmt.Errorf("table %s: row %s column %s has no options", def.Stage, row.ID, column.Name)
			}
			names := namesFor(column, window.Context, row, options)
			question := llm.Question{Name: column.Item, Ask: columnQuestion(column, names)}
			if !yesOnly(column, options) {
				// A catalogue's purposes are already in the context; repeating
				// them in every question multiplied a request past the budget.
				// Only a column that takes its criteria from the catalogue
				// sends them, and then not the catalogue itself.
				for _, option := range options {
					choice := llm.Option{Name: names.label[option], Meaning: names.criteria[option]}
					if criteria, ok := column.Criteria[option]; ok {
						choice.Criteria = &criteria
					} else if column.EachCriteria != nil && !slices.Contains(column.Options, option) {
						choice.Criteria = column.EachCriteria
					}
					question.Options = append(question.Options, choice)
				}
				if column.Optional {
					question.Options = append(question.Options, llm.Option{Name: classifierAbsent, Meaning: "No listed option applies to this row."})
				}
			}
			question.Item = classifierItem(row, column, names, question.Options)
			questions[questionKey(row, column)] = question
		}
	}
	prompt, err := c.Prompt(def.System, classifierContext(def, window.Context), questions)
	if err != nil {
		return llm.Call[Result]{}, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	state, err := json.Marshal(struct {
		Contract string  `json:"contract"`
		Prompt   string  `json:"prompt_sha256"`
		Request  string  `json:"request_sha256"`
		Margin   float64 `json:"margin"`
		YesAt    float64 `json:"yes_at,omitempty"`
		Top      bool    `json:"top_choice,omitempty"`
	}{def.Contract + ".classifier.v2", sha256Hex([]byte(def.System)), sha256Hex([]byte(prompt.User)), ClassifierMargin, def.YesAt, def.TopChoice})
	if err != nil {
		return llm.Call[Result]{}, err
	}
	return llm.Call[Result]{
		State:  state,
		Prompt: prompt,
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1},
		DecodeValidate: func(raw []byte) (Result, error) {
			verdicts, err := c.Verdicts(raw)
			if err != nil {
				return Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
			}
			return DecodeClassifierAnswers(def, window, verdicts)
		},
	}, nil
}

// DecodeClassifierAnswers accepts a row when every column is decided: a
// listed option, or an optional column's explicit "none of these", leading
// every other listed option by ClassifierMargin, or a yes/no clear of the
// uncertain band. Anything else leaves the row explicitly unanswered, never
// silently absent; other rows stand alone. An Alone column that is not
// decided leaves only itself unanswered, recorded as a refused cell, and
// the row keeps its other decisions unless none of them was decided. A
// window whose every row was answered, even uncertainly, is an explicit
// answer: its uncertain rows stay unanswered and it is not asked again. A
// window where no row was accepted and some row was not answered, or was
// answered outside its options, is refused.
func DecodeClassifierAnswers(def Definition, window Window, verdicts map[string]llm.Verdict) (Result, error) {
	yesAt := 0.5 + ClassifierNoulMargin
	result := Result{Answers: make(Answers, len(window.Rows)), rowKeys: make([]string, len(window.Rows)), partial: make([]bool, len(window.Rows)), uncertain: make([]bool, len(window.Rows))}
	accepted, uncertain := 0, 0
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
		answer := make(Answer, len(def.Columns))
		reason := ""
		// A refused row is uncertain only when every failure was a near-tie:
		// one column not answered or answered outside its options makes it
		// unanswered.
		unsure := true
		var refused []RowRejection
		for _, column := range def.Columns {
			failed, failedUnsure := decideClassifierColumn(def, window, row, column, verdicts, yesAt, answer)
			if failed == "" {
				continue
			}
			unsure = unsure && failedUnsure
			if column.Alone {
				refused = append(refused, RowRejection{Key: row.ID, Cell: column.Name, Reason: failed})
				continue
			}
			reason = failed
			break
		}
		if reason == "" && len(refused) == len(def.Columns) {
			// No column was decided: the row has no answer at all.
			reason = refused[0].Reason
			refused = nil
		}
		if reason != "" {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Reason: reason})
			if unsure {
				uncertain++
				result.uncertain[i] = true
			}
			continue
		}
		result.Answers[i] = answer
		result.partial[i] = len(refused) > 0
		result.Rejections = append(result.Rejections, refused...)
		accepted++
	}
	if accepted == 0 && uncertain < len(window.Rows) {
		return Result{}, fmt.Errorf("table %s: no rows accepted; %s", def.Stage, result.Rejections[0].Reason)
	}
	return result, nil
}

// decideClassifierColumn writes one column's decision into the answer, or
// says why it has none and whether that is because the answer was uncertain.
// An optional column's "none of these" and a yes/no question's clear no are
// decisions that write no cell.
func decideClassifierColumn(def Definition, window Window, row Row, column Column, verdicts map[string]llm.Verdict, yesAt float64, answer Answer) (string, bool) {
	got, ok := verdicts[questionKey(row, column)]
	if ok && got.Conflict {
		return fmt.Sprintf("column %s was answered twice differently", column.Name), false
	}
	options := columnOptions(column, window.Context, row)
	if yesOnly(column, options) {
		if ok && (got.InvalidYes || got.Yes != nil && !validProbability(*got.Yes)) {
			return fmt.Sprintf("column %s has an invalid required probability of yes", column.Name), false
		}
		if def.Ranked && ok && got.Yes != nil {
			answer[ProbabilityCell(column.Name)] = strconv.FormatFloat(*got.Yes, 'f', 4, 64)
			if *got.Yes >= yesAt {
				answer[column.Name] = "yes"
			}
			return "", false
		}
		switch {
		case !ok || got.Yes == nil:
			return fmt.Sprintf("column %s was not answered", column.Name), false
		case *got.Yes >= yesAt:
			answer[column.Name] = "yes"
			return "", false
		case *got.Yes <= 1-yesAt:
			return "", false
		default:
			return fmt.Sprintf("column %s is uncertain: yes at %.2f", column.Name, *got.Yes), true
		}
	}
	if def.YesAt > 0 && len(options) == 2 && slices.Contains(options, "yes") && slices.Contains(options, "no") {
		got = LabelForm(got, options)
		if !ok || got.Choice == "" || got.Probabilities == nil {
			return fmt.Sprintf("column %s was not answered", column.Name), false
		}
		if !slices.Contains(options, got.Choice) {
			return fmt.Sprintf("column %s chose %q, not one of the options", column.Name, got.Choice), false
		}
		if reason := requiredProbabilities(got, options); reason != "" {
			return fmt.Sprintf("column %s %s", column.Name, reason), false
		}
		answer[column.Name] = "no"
		if got.Probabilities["yes"] >= def.YesAt {
			answer[column.Name] = "yes"
		}
		return "", false
	}
	names := namesFor(column, window.Context, row, options)
	labels := make([]string, 0, len(options)+1)
	for _, option := range options {
		labels = append(labels, names.label[option])
	}
	if column.Optional {
		labels = append(labels, classifierAbsent)
	}
	got = LabelForm(got, labels)
	if ok && slices.Contains(labels, got.Choice) {
		if reason := requiredProbabilities(got, labels); reason != "" {
			return fmt.Sprintf("column %s %s", column.Name, reason), false
		}
	}
	probability := got.Probabilities[got.Choice]
	rival, rivalAt := runnerUp(got, labels)
	switch {
	case !ok || got.Choice == "":
		return fmt.Sprintf("column %s was not answered", column.Name), false
	case !slices.Contains(labels, got.Choice):
		return fmt.Sprintf("column %s chose %q, not one of the options", column.Name, got.Choice), false
	// Jev's probabilities are hundredths carried as floats: 0.30
	// against 0.20 leads by 0.0999…, which is the margin.
	case probability-rivalAt < ClassifierMargin-1e-9 && !def.TopChoice:
		return fmt.Sprintf("column %s is uncertain: %q at %.2f against %q at %.2f, a lead under %.2f", column.Name, got.Choice, probability, rival, rivalAt, ClassifierMargin), true
	case got.Choice == classifierAbsent:
		return "", false
	default:
		answer[column.Name] = names.ref[got.Choice]
		return "", false
	}
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func requiredProbabilities(got llm.Verdict, labels []string) string {
	for _, label := range labels {
		value, present := got.Probabilities[label]
		if !present {
			return fmt.Sprintf("is missing required probability for %q", label)
		}
		if slices.Contains(got.InvalidProbabilities, label) || !validProbability(value) {
			return fmt.Sprintf("has an invalid required probability for %q", label)
		}
	}
	return ""
}

// LabelForm writes a verdict's choice and probability keys as the listed
// labels they spell in another letter case or with surrounding whitespace:
// " YES " is the label yes. That is the answer's form, not another decision
// (review B2's residual). A key that spells no label stays as written, and
// an exact key wins over one that only spells the same label.
func LabelForm(got llm.Verdict, labels []string) llm.Verdict {
	label := func(written string) string {
		trimmed := strings.TrimSpace(written)
		for _, candidate := range labels {
			if candidate == trimmed || strings.EqualFold(candidate, trimmed) {
				return candidate
			}
		}
		return written
	}
	got.Choice = label(got.Choice)
	got.InvalidProbabilities = slices.Clone(got.InvalidProbabilities)
	for i, written := range got.InvalidProbabilities {
		got.InvalidProbabilities[i] = label(written)
	}
	if got.Probabilities != nil {
		probabilities := make(map[string]float64, len(got.Probabilities))
		for written, at := range got.Probabilities {
			key := label(written)
			if _, exact := got.Probabilities[key]; exact && key != written {
				continue
			}
			probabilities[key] = at
		}
		got.Probabilities = probabilities
	}
	return got
}

// runnerUp is the listed option, other than the chosen one, that the answer
// gives the highest probability; the first listed wins a tie. A choice that
// is not the top option therefore has a negative lead.
func runnerUp(got llm.Verdict, labels []string) (string, float64) {
	rival, rivalAt := "", 0.0
	for _, label := range labels {
		if at := got.Probabilities[label]; label != got.Choice && (rival == "" || at > rivalAt) {
			rival, rivalAt = label, at
		}
	}
	return rival, rivalAt
}
