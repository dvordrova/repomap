// Package table is the one request shape the atlas asks the model with: a
// keyed table. Rows go in with keys the code assigned; the same keys come
// back with short cells. Every row is accepted or rejected separately from
// its neighbours. Nothing here knows what a directory or a file is.
package table

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/llm"
)

// DefaultInputBytes is the packing target for consecutive complete rows.
// A row with its context may exceed it in a request of its own; the shared
// provider envelope still applies. Isolated readings may set a hard budget.
const DefaultInputBytes = 64 * 1024

// Kind is what a cell may hold.
type Kind string

const (
	// Text is one short line; longer answers are cut at a word.
	Text Kind = "text"
	// Prose preserves the complete explanation and its whitespace. The
	// provider response envelope bounds it; qualifiers must not be cut off.
	Prose Kind = "prose"
	// Choice is one option from a closed list.
	Choice Kind = "choice"
	// Sequence is a space-separated ordered selection of exact closed refs.
	Sequence Kind = "sequence"
)

// Column is one cell the model fills for every row.
type Column struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	// MaxRunes bounds a text cell.
	MaxRunes int `json:"max_runes,omitempty"`
	// Options is a closed list shared by every row; OptionsFrom names the row
	// field that carries the row's own list instead, or the window context
	// field when the row has none: a catalogue every row chooses from is
	// sent once, not once per row.
	Options     []string `json:"options,omitempty"`
	OptionsFrom string   `json:"options_from,omitempty"`
	// LimitFrom names the row or context field that tells the model how many
	// of a Sequence's options to choose, such as a learning menu of five
	// questions. It is guidance: every distinct advertised ref the model
	// chose is kept, and the owner journals a selection past it.
	LimitFrom string `json:"limit_from,omitempty"`
	// Free is a prefix after which the model may write its own short text,
	// such as "new: " for a title the code has not seen. Empty means no.
	Free         string `json:"free,omitempty"`
	FreeMaxRunes int    `json:"free_max_runes,omitempty"`
	// Note is one sentence the model reads about this cell.
	Note string `json:"note,omitempty"`
	// EmptyValue is an owner-defined spelling for absent prose. It stays in
	// the response contract but does not become text for the optional glossary.
	// An empty, null or missing cell reads as it, and so does its spelling in
	// another case or with a final period ("None.").
	EmptyValue string `json:"empty_value,omitempty"`
	// Missing is the value a cell takes when the model omits it or sends
	// null, and a text cell when it comes back empty: a value that already
	// means "no decision", such as unassessed or a label's "-". A decoder
	// rule, not part of the request or the memo state; a written choice is
	// still validated as before.
	Missing string `json:"-"`
	// Optional lets the model leave the cell out or send null: the row then
	// has no value for it, which is the answer "not this". A written value
	// is still validated; one that fails is discarded and recorded, and the
	// row keeps its other cells. A decoder rule like Missing.
	Optional bool `json:"-"`
	// Alone lets this cell fail by itself: a missing, null, mistyped, empty
	// or unlisted value, or repeated copies that differ, refuse only this
	// cell, recorded as a rejection. The row keeps its other decisions unless
	// a cell without Alone failed or no cell the model wrote survived: a
	// Missing or EmptyValue filled in for an absent cell is no answer. A
	// decoder rule like Missing; the owner reads an absent cell with its own
	// fallback.
	Alone bool `json:"-"`
	// WhenOptionsFrom requires this cell only when the named input field, in
	// the row or the window context, has advertised choices. A row without
	// the field has no decision to request or validate: an address cell is
	// asked only where the code has address candidates to choose from.
	WhenOptionsFrom string `json:"when_options_from,omitempty"`
	// Ask is the question a decision model is asked for this column, stated
	// directly; empty derives one from the column's name and note.
	Ask string `json:"-"`
	// Item is what a decision model's question calls the row, such as
	// "file"; empty is "row". Ask names it.
	Item string `json:"-"`
	// Criteria are a decision model's explicit terms for each of Options.
	Criteria map[string]llm.Criteria `json:"-"`
	// CriteriaFrom names the field of each entry of the OptionsFrom
	// catalogue whose text is that option's criteria for a decision model.
	// The catalogue then reaches the model only as the options and their
	// criteria, never again in the shared context.
	CriteriaFrom string `json:"-"`
}

// Definition is one table: its stage name, window size, prompt and columns.
type Definition struct {
	// Stage is the debugdump stage of every window of this table.
	Stage string
	// Contract versions the prompt and columns for independent row memos.
	// Whole responses are keyed by exact provider bytes and always revalidated
	// against the current table, including after a contract-only change.
	Contract string
	// Window optionally limits rows per request. Zero uses only the byte budget.
	Window  int
	System  string
	Columns []Column
	// Reasoning opts this table into provider-supported deliberate reasoning.
	Reasoning bool
	// MaxInputBytes optionally bounds system + user UTF-8 bytes before provider
	// encoding for isolated readings. Zero uses DefaultInputBytes as a packing
	// target and keeps an oversized row whole in its own request.
	MaxInputBytes int
	// Memoize reuses independent description rows through entity knowledge.
	Memoize bool
	// ContextAfterRows keeps repeated evidence ahead of changing context in
	// the request prefix. The default preserves context before rows.
	ContextAfterRows bool
	// Classifier sends this closed table to the run's categorizer
	// (llm.Categorizer), never to the text model. It is an explicit,
	// measured opt-in, not a consequence of the table's shape.
	Classifier bool
	// YesAt turns a yes/no column into a cutoff for the categorizer: yes at
	// this probability of yes or above, no below it. Zero keeps the ordinary
	// choice rule: a side that does not lead the other by ClassifierMargin
	// leaves the row unanswered.
	YesAt float64
	// Ranked makes the categorizer's yes/no answers a ranking: every row
	// keeps the probability of yes (ProbabilityCell) and none is refused as
	// uncertain, because the owner orders rows instead of thresholding them.
	Ranked bool
}

// ProbabilityCell names the cell holding a ranked column's probability of yes.
func ProbabilityCell(column string) string { return column + "@p" }

// Field is one ordered input of a row.
type Field struct {
	Name  string
	Value any
}

// Row is one thing the model is asked about. ID is its existing artifact
// identity and is also the closed key copied by the model. The table layer
// never invents a second, window-local identity.
type Row struct {
	ID     string
	Fields []Field
}

// Window is one request with the rows' existing IDs and an optional
// Definition.Window row limit.
// Context is what every row of the window shares: a closed list of names to
// choose from, a count the answer is measured against.
type Window struct {
	Stage   string
	Round   int
	Index   int
	Context []Field
	Rows    []Row
	Request []byte
}

// Windows packs rows into windows of the definition's budgets, keeping the
// caller's order. Boundaries are the caller's business: it sorts rows by path
// so a window rarely straddles a directory.
func Windows(def Definition, round int, rows []Row) ([]Window, error) {
	return WindowsWithContext(def, round, nil, rows)
}

// WindowsWithContext is Windows with fields every window carries.
func WindowsWithContext(def Definition, round int, context []Field, rows []Row) ([]Window, error) {
	if def.Window < 0 {
		return nil, fmt.Errorf("table %s: window size %d", def.Stage, def.Window)
	}
	limit := def.MaxInputBytes
	if limit == 0 {
		limit = DefaultInputBytes
	}
	if limit < 1 {
		return nil, fmt.Errorf("table %s: input budget must be positive", def.Stage)
	}
	var windows []Window
	if len(rows) == 0 {
		return windows, nil
	}
	empty, err := Request(def, Window{Context: context})
	if err != nil {
		return nil, err
	}
	baseSize := len(def.System) + len(empty)
	appendWindow := func(part []Row) error {
		window := Window{Stage: def.Stage, Round: round, Index: len(windows), Context: context, Rows: part}
		request, err := Request(def, window)
		if err != nil {
			return err
		}
		window.Request = request
		windows = append(windows, window)
		return nil
	}
	// Measure with the same encoder as Request, including each window-local
	// key. Fill consecutive complete rows until the next one no longer fits.
	start, size := 0, baseSize
	for i, row := range rows {
		position := i - start
		if def.Window > 0 && position == def.Window {
			if err := appendWindow(rows[start:i]); err != nil {
				return nil, err
			}
			start, position, size = i, 0, baseSize
		}
		var encoded jsonBuffer
		writeRow(&encoded, row)
		if encoded.err != nil {
			return nil, fmt.Errorf("table %s: encode evidence: %w", def.Stage, encoded.err)
		}
		additional := encoded.Len()
		if position > 0 {
			additional += len(",\n")
		}
		if position > 0 && size+additional > limit {
			if err := appendWindow(rows[start:i]); err != nil {
				return nil, err
			}
			start, size = i, baseSize
			encoded.Reset()
			writeRow(&encoded, row)
			if encoded.err != nil {
				return nil, fmt.Errorf("table %s: encode evidence: %w", def.Stage, encoded.err)
			}
			additional = encoded.Len()
		}
		if def.MaxInputBytes > 0 && size+additional > limit {
			return nil, fmt.Errorf("table %s: row %s needs %d input bytes, budget %d; reduce this row's evidence or shared context", def.Stage, row.ID, size+additional, limit)
		}
		size += additional
	}
	if err := appendWindow(rows[start:]); err != nil {
		return nil, err
	}
	return windows, nil
}

func writeRow(out *jsonBuffer, row Row) {
	out.WriteString("    {\"key\": ")
	writeJSON(out, row.ID)
	for _, field := range row.Fields {
		out.WriteString(", ")
		writeJSON(out, field.Name)
		out.WriteString(": ")
		writeJSON(out, field.Value)
	}
	out.WriteString("}")
}

// Request is the exact user prompt of a window: one JSON object with the
// table name, the columns to fill and the rows. Field order is the caller's.
func Request(def Definition, window Window) ([]byte, error) {
	if _, err := rowIndexes(window.Rows); err != nil {
		return nil, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	for _, column := range def.Columns {
		if column.Name == "key" {
			// "key" is every row's own identity in the request and the answer.
			return nil, fmt.Errorf("table %s: column name %q is reserved for the row identity", def.Stage, column.Name)
		}
		if column.LimitFrom == "" {
			continue
		}
		for _, row := range window.Rows {
			// The limit the model is told is the owner's to supply: a row
			// without it is a preparation error, never a refused answer.
			field, found := fieldFrom(window.Context, row, column.LimitFrom)
			if limit, isInt := field.Value.(int); !found || !isInt || limit < 1 {
				return nil, fmt.Errorf("table %s: row %s has no positive integer %q for column %q", def.Stage, row.ID, column.LimitFrom, column.Name)
			}
		}
	}
	var out jsonBuffer
	out.WriteString("{\n  \"table\": ")
	writeJSON(&out, def.Stage)
	out.WriteString(",\n  \"fill\": [")
	for i, column := range def.Columns {
		if i > 0 {
			out.WriteString(", ")
		}
		spec := map[string]any{"name": column.Name, "kind": string(column.Kind)}
		if column.MaxRunes > 0 {
			spec["max_runes"] = column.MaxRunes
		}
		if len(column.Options) > 0 {
			spec["options"] = column.Options
		}
		if column.OptionsFrom != "" {
			spec["options_from"] = column.OptionsFrom
		}
		if column.LimitFrom != "" {
			spec["limit_from"] = column.LimitFrom
		}
		if column.Free != "" {
			spec["free_prefix"] = column.Free
		}
		if column.Note != "" {
			spec["note"] = column.Note
		}
		if column.EmptyValue != "" {
			spec["empty_value"] = column.EmptyValue
		}
		if column.WhenOptionsFrom != "" {
			spec["when_options_nonempty"] = column.WhenOptionsFrom
		}
		writeJSON(&out, spec)
	}
	out.WriteString("]")
	writeContext := func() {
		if len(window.Context) == 0 {
			return
		}
		out.WriteString(",\n  \"context\": {")
		for i, field := range window.Context {
			if i > 0 {
				out.WriteString(", ")
			}
			writeJSON(&out, field.Name)
			out.WriteString(": ")
			writeJSON(&out, field.Value)
		}
		out.WriteString("}")
	}
	if !def.ContextAfterRows {
		writeContext()
	}
	out.WriteString(",\n  \"rows\": [\n")
	for i, row := range window.Rows {
		if i > 0 {
			out.WriteString(",\n")
		}
		writeRow(&out, row)
	}
	out.WriteString("\n  ]")
	if def.ContextAfterRows {
		writeContext()
	}
	out.WriteString("\n}\n")
	if out.err != nil {
		return nil, fmt.Errorf("table %s: encode evidence: %w", def.Stage, out.err)
	}
	return out.Bytes(), nil
}

type jsonBuffer struct {
	bytes.Buffer
	err error
}

func writeJSON(out *jsonBuffer, value any) {
	if out.err != nil {
		return
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		out.err = err
		return
	}
	out.Write(bytes.TrimRight(buffer.Bytes(), "\n"))
}

// Answer is what one row came back with: a value per column.
type Answer map[string]string

// Answers are the window's rows in request order.
type Answers []Answer

// Decode returns accepted cells in request order, with nil for an independently
// rejected row. DecodeResult additionally reports each rejection's reason.
func Decode(def Definition, window Window, raw []byte) (Answers, error) {
	result, err := DecodeResult(def, window, raw)
	return result.Answers, err
}

func rowIndexes(rows []Row) (map[string]int, error) {
	result := make(map[string]int, len(rows))
	for i, row := range rows {
		if row.ID == "" {
			return nil, fmt.Errorf("row %d has no ID", i+1)
		}
		if _, exists := result[row.ID]; exists {
			return nil, fmt.Errorf("row ID %q is duplicated", row.ID)
		}
		result[row.ID] = i
	}
	return result, nil
}

func normalizeCell(column Column, context []Field, row Row, cell string) (string, error) {
	if column.Kind == Prose || column.Kind == Text {
		text := strings.TrimSpace(cell)
		if column.Kind == Text {
			text = collapse(cell)
		}
		// The column's own spelling of absence, in any case or with a final
		// period, and an empty cell read as that absence; without one, an
		// empty cell has no answer.
		switch {
		case column.EmptyValue != "" && (text == "" || strings.EqualFold(strings.TrimSuffix(text, "."), column.EmptyValue)):
			return column.EmptyValue, nil
		case text == "" && column.Missing != "":
			return column.Missing, nil
		case text == "":
			return "", fmt.Errorf("cell %q is empty", column.Name)
		case column.Kind == Text && column.MaxRunes > 0:
			return cutRunes(text, column.MaxRunes), nil
		}
		return text, nil
	}
	text := collapse(cell)
	switch column.Kind {
	case Sequence:
		// none is the contract's empty selection. A provider that fills every
		// schema key sends null, which arrives here as an empty string; an
		// empty selection is the only reading of an empty sequence, so it
		// is not a refused row.
		if text == "none" || text == "" {
			return "", nil
		}
		options := make(map[string]bool)
		for _, option := range optionsFrom(context, row, column.OptionsFrom) {
			options[option] = true
		}
		var selected []string
		seen := make(map[string]bool)
		for _, ref := range strings.Fields(strings.ReplaceAll(text, ",", " ")) {
			if options[ref] && !seen[ref] {
				selected = append(selected, ref)
				seen[ref] = true
			}
		}
		// Refs outside the row's options were never selectable: a row citing
		// only such refs (calls listed as context beside call_options, or
		// invented ones) selects nothing, and its other cells keep their
		// decisions. The raw response keeps what was written. Every distinct
		// advertised ref stays, in the written order, whatever LimitFrom
		// asked: keeping the first N would be the code choosing.
		return strings.Join(selected, " "), nil
	case Choice:
		options := column.Options
		if column.OptionsFrom != "" {
			options = optionsFrom(context, row, column.OptionsFrom)
		}
		for _, option := range options {
			if strings.EqualFold(text, option) {
				return option, nil
			}
		}
		// Surrounding quotes or backticks and one final . , ; or ! are
		// formatting: "yes.", "\"p3\"" and "`d2`" are the listed choice. A ref
		// followed by anything else, such as "p3: Storage", is not.
		if bare := choiceText(text); bare != text {
			text = bare
			for _, option := range options {
				if strings.EqualFold(text, option) {
					return option, nil
				}
			}
		}
		// A closed choice whose only option is unknown has nothing else to
		// choose: any other answer establishes nothing and is unknown. An
		// outgoing candidate without an a* address catalogue is the usual
		// case; the model copies the observed path into the address cell.
		if column.Free == "" && len(options) == 1 && options[0] == "unknown" {
			return "unknown", nil
		}
		// A cut answer that begins exactly one option is that option: the
		// model wrote "Utilities and" for "Utilities and configuration".
		if len(text) >= 4 {
			matched := ""
			for _, option := range options {
				if len(option) > len(text) && strings.EqualFold(option[:len(text)], text) {
					if matched != "" {
						matched = ""
						break
					}
					matched = option
				}
			}
			if matched != "" {
				return matched, nil
			}
		}
		if rest, free := freeChoiceText(column.Free, text); free {
			if rest == "" {
				return "", fmt.Errorf("cell %q has an empty %q value", column.Name, strings.TrimSpace(column.Free))
			}
			limit := column.FreeMaxRunes
			if limit == 0 {
				limit = 40
			}
			return column.Free + cutRunes(rest, limit), nil
		}
		return "", fmt.Errorf("cell %q is %q, not one of the options", column.Name, text)
	default:
		return "", fmt.Errorf("column %q has kind %q", column.Name, column.Kind)
	}
}

// choiceText strips the formatting a written choice may carry: surrounding
// quotes or backticks, and one final . , ; or !.
func choiceText(text string) string {
	punctuated := false
	for {
		n := len(text)
		switch {
		case n >= 2 && strings.IndexByte("\"'`", text[0]) >= 0 && text[n-1] == text[0]:
			text = strings.TrimSpace(text[1 : n-1])
		case n >= 1 && !punctuated && strings.IndexByte(".,;!", text[n-1]) >= 0:
			text, punctuated = strings.TrimSpace(text[:n-1]), true
		default:
			return text
		}
	}
}

// Tagged free choices keep their written name even when the model varies
// whitespace around the colon. The tag itself must still match exactly.
func freeChoiceText(prefix, text string) (string, bool) {
	if prefix == "" {
		return "", false
	}
	if tag := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(prefix), ":")); strings.HasSuffix(strings.TrimSpace(prefix), ":") && tag != "" {
		colon := strings.IndexByte(text, ':')
		if colon >= 0 && strings.EqualFold(strings.TrimSpace(text[:colon]), tag) {
			return collapse(text[colon+1:]), true
		}
		return "", false
	}
	if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
		return collapse(text[len(prefix):]), true
	}
	return "", false
}

// fieldFrom finds a named input: the row's own field first, then the window
// context's. A row field shadows a context field of the same name.
func fieldFrom(context []Field, row Row, name string) (Field, bool) {
	if name == "" {
		return Field{}, false
	}
	for _, field := range row.Fields {
		if field.Name == name {
			return field, true
		}
	}
	for _, field := range context {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

// optionsFrom reads the closed list a named row or context field carries.
func optionsFrom(context []Field, row Row, name string) []string {
	field, ok := fieldFrom(context, row, name)
	if !ok {
		return nil
	}
	switch value := field.Value.(type) {
	case []string:
		return value
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}

// IsFree reports whether a choice cell used the column's free prefix and
// returns the text after it.
func IsFree(column Column, value string) (string, bool) {
	if column.Free == "" || len(value) <= len(column.Free) || !strings.HasPrefix(value, column.Free) {
		return "", false
	}
	return value[len(column.Free):], true
}

func collapse(text string) string {
	fields := strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r < 0x20 || r == 0x7f })
	return strings.Join(fields, " ")
}

// OneLine collapses whitespace and control characters to single spaces: a
// prose cell shown where a line is required.
func OneLine(text string) string { return collapse(text) }

// LabelFromProse takes the first sentence of a prose cell as a short label,
// or the prose cut to the limit when that sentence is still too long. A
// nonpositive limit keeps the whole first sentence.
func LabelFromProse(text string, limit int) string {
	text = strings.TrimSpace(text)
	if end := strings.Index(text, ". "); end > 0 {
		text = text[:end]
	} else if strings.HasSuffix(text, ".") {
		text = strings.TrimSuffix(text, ".")
	}
	return cutRunes(text, limit)
}

func cutRunes(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := limit - 1
	for cut > limit/2 && !unicode.IsSpace(runes[cut]) {
		cut--
	}
	return strings.TrimRight(string(runes[:cut]), " ,;:") + "…"
}

// State binds independent row memos to the table contract, prompt, request and
// reasoning preference. Exact-response reuse follows actual provider bytes;
// a State-only change does not invalidate that accepted response.
func State(def Definition, window Window) ([]byte, error) {
	return json.Marshal(struct {
		Contract  string `json:"contract"`
		Prompt    string `json:"prompt_sha256"`
		Request   string `json:"request_sha256"`
		Reasoning bool   `json:"reasoning,omitempty"`
	}{
		Contract:  def.Contract,
		Prompt:    sha256Hex([]byte(def.System)),
		Request:   sha256Hex(window.Request),
		Reasoning: def.Reasoning,
	})
}

// Call is the executable form of one window.
func Call(def Definition, window Window) (llm.Call[Answers], error) {
	state, err := State(def, window)
	if err != nil {
		return llm.Call[Answers]{}, err
	}
	hasProse := false
	for _, column := range def.Columns {
		// A conditional prose branch is still eligible: the model decides which
		// branch applies, so row preparation must not erase its glossary support.
		hasProse = hasProse || column.Kind == Text || column.Kind == Prose
	}
	return llm.Call[Answers]{
		State: state,
		Prompt: llm.Prompt{
			System: def.System, User: string(window.Request), ResponseFormatJSON: true,
			Reasoning: def.Reasoning, ResponseExample: ResponseExample(def, window), NoResponseAdjunct: !hasProse,
		},
		Limits: llm.Limits{
			MaxRequestBytes:  llm.SemanticRecordByteLimit,
			MaxResponseBytes: llm.ProviderResponseByteLimit,
			MaxOutputTokens:  llm.DefaultMaxOutputTokens,
		},
		DecodeValidate: func(raw []byte) (Answers, error) {
			return Decode(def, window, raw)
		},
	}, nil
}

// MemoIdentity identifies the interpretation input without its owning row ID.
// Equal evidence may be interpreted once for multiple artifact owners, while
// every actual provider request and response still uses each artifact's ID.
// No substitute row key is minted for memoization.
func MemoIdentity(provider llm.Provider, def Definition, window Window) (string, error) {
	type memoRow struct {
		Fields []Field `json:"fields"`
	}
	rows := make([]memoRow, len(window.Rows))
	for i, row := range window.Rows {
		rows[i] = memoRow{Fields: row.Fields}
	}
	semantic, err := json.Marshal(struct {
		Table   string    `json:"table"`
		Fill    []Column  `json:"fill"`
		Context []Field   `json:"context,omitempty"`
		Rows    []memoRow `json:"rows"`
	}{def.Stage, def.Columns, window.Context, rows})
	if err != nil {
		return "", err
	}
	questions, err := questionsDigest(def)
	if err != nil {
		return "", err
	}
	state, err := json.Marshal(struct {
		Contract  string `json:"contract"`
		Prompt    string `json:"prompt_sha256"`
		Reasoning bool   `json:"reasoning,omitempty"`
		Questions string `json:"questions,omitempty"`
	}{def.Contract, sha256Hex([]byte(def.System)), def.Reasoning, questions})
	if err != nil {
		return "", err
	}
	return llm.MemoIdentityBytes(provider, state, semantic)
}

// questionsDigest pins what a decision model is asked beyond the columns
// the request serializes: each question's wording, its item's name, its
// options and their criteria. A remembered answer to a question worded
// otherwise is not this question's answer. A table that asks none of them
// keeps its basis ("" leaves the field out).
func questionsDigest(def Definition) (string, error) {
	type question struct {
		Column       string                  `json:"column"`
		Ask          string                  `json:"ask,omitempty"`
		Item         string                  `json:"item,omitempty"`
		Options      []string                `json:"options,omitempty"`
		Criteria     map[string]llm.Criteria `json:"criteria,omitempty"`
		CriteriaFrom string                  `json:"criteria_from,omitempty"`
	}
	var questions []question
	for _, column := range def.Columns {
		if column.Ask == "" && column.Item == "" && len(column.Criteria) == 0 && column.CriteriaFrom == "" {
			continue
		}
		questions = append(questions, question{Column: column.Name, Ask: column.Ask, Item: column.Item, Options: column.Options, Criteria: column.Criteria, CriteriaFrom: column.CriteriaFrom})
	}
	if len(questions) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(questions)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}

// ResponseExample is the one-line answer shape appended to every system
// prompt: the row key first, then the cells in fill order. A map would
// marshal its keys alphabetically and show the key third; the model copies
// the shape it sees, and the decoder reads the key before anything else.
func ResponseExample(def Definition, window Window) string {
	var example strings.Builder
	example.WriteString(`{"rows":[{"key":`)
	key := "<same row ID>"
	if len(window.Rows) > 0 {
		key = window.Rows[0].ID
	}
	example.Write(jsonString(key))
	for _, column := range def.Columns {
		example.WriteString(",")
		example.Write(jsonString(column.Name))
		example.WriteString(":")
		example.Write(jsonString("<computed " + column.Name + ">"))
	}
	example.WriteString("}]}")
	return example.String()
}

func jsonString(value string) []byte {
	var out jsonBuffer
	writeJSON(&out, value)
	return out.Bytes()
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// SortRows orders rows by ID so windows are stable across runs.
func SortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
}
