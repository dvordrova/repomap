// Package table is the one request shape the atlas asks the model with: a
// keyed table. Rows go in with keys the code assigned; the same keys come
// back with short cells. Independent rows are accepted or rejected separately;
// coupled tables require a complete valid window. Nothing here knows what a
// directory or a file is.
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
	// field that carries the row's own list instead.
	Options     []string `json:"options,omitempty"`
	OptionsFrom string   `json:"options_from,omitempty"`
	// LimitFrom names the row field bounding a Sequence's number of choices.
	LimitFrom string `json:"limit_from,omitempty"`
	// Free is a prefix after which the model may write its own short text,
	// such as "new: " for a title the code has not seen. Empty means no.
	Free         string `json:"free,omitempty"`
	FreeMaxRunes int    `json:"free_max_runes,omitempty"`
	// Note is one sentence the model reads about this cell.
	Note string `json:"note,omitempty"`
	// EmptyValue is an owner-defined spelling for absent prose. It stays in
	// the response contract but does not become text for the optional glossary.
	EmptyValue string `json:"empty_value,omitempty"`
	// When limits this cell to a previously validated choice in the same row.
	// Inactive cells have no authority and are not required or retained.
	When map[string]string `json:"when,omitempty"`
	// EmptyFrom names another text cell of the same row whose first sentence
	// stands in when this required text cell comes back empty, null or
	// missing; the answer records <name>_from with the source cell. It is a
	// decoder rule, not part of the request or the memo state: an operation
	// label taken from the model's own description keeps the row's accepted
	// decision instead of refusing the whole row.
	EmptyFrom string `json:"-"`
	// WhenOptionsFrom requires this cell only when the named input field has
	// advertised choices. Empty choices have no decision to request or validate.
	WhenOptionsFrom string `json:"when_options_from,omitempty"`
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
	// Independent validates each row separately from its neighbours.
	// The prompt must restrict each answer to that row and its context.
	Independent bool
	// Memoize reuses independent description rows through entity knowledge.
	Memoize bool
	// ContextAfterRows keeps repeated evidence ahead of changing context in
	// the request prefix. The default preserves context before rows.
	ContextAfterRows bool
}

// Field is one ordered input of a row.
type Field struct {
	Name  string
	Value any
}

// Row is one thing the model is asked about. ID is the code-side identity;
// the model sees only the window-local key.
type Row struct {
	ID     string
	Fields []Field
}

// Window is one request with keys r1..rN and optional Definition.Window row limit.
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

// Key is the window-local key of row i.
func Key(i int) string { return fmt.Sprintf("r%d", i+1) }

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
		writeRow(&encoded, position, row)
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
			writeRow(&encoded, 0, row)
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

func writeRow(out *jsonBuffer, index int, row Row) {
	out.WriteString("    {\"key\": ")
	writeJSON(out, Key(index))
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
		if len(column.When) != 0 {
			spec["when"] = column.When
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
		writeRow(&out, i, row)
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

// AcceptedRowKeys preserves whole-response metadata for complete tables and
// limits a partial independent result to the rows whose required cells passed.
func (answers Answers) AcceptedRowKeys() []string {
	keys := make([]string, 0, len(answers))
	for i, answer := range answers {
		if answer != nil {
			keys = append(keys, Key(i))
		}
	}
	if len(keys) == len(answers) {
		return nil
	}
	return keys
}

// Decode returns accepted cells in request order, with nil for an independently
// rejected row. DecodeResult additionally reports each rejection's reason.
func Decode(def Definition, window Window, raw []byte) (Answers, error) {
	result, err := DecodeResult(def, window, raw)
	return result.Answers, err
}

func decodeWindow(def Definition, window Window, raw []byte) (Answers, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("table %s: response is not {\"rows\": [...]}: %w", def.Stage, err)
	}
	if len(envelope.Rows) != len(window.Rows) {
		return nil, fmt.Errorf("table %s: %d rows answered, %d asked", def.Stage, len(envelope.Rows), len(window.Rows))
	}
	answers := make(Answers, len(window.Rows))
	seen := make(map[string]struct{}, len(window.Rows))
	for _, cells := range envelope.Rows {
		keyRaw, ok := cells["key"]
		if !ok {
			return nil, fmt.Errorf("table %s: a row has no key", def.Stage)
		}
		var key string
		if err := json.Unmarshal(keyRaw, &key); err != nil {
			return nil, fmt.Errorf("table %s: a row key is not a string", def.Stage)
		}
		index, ok := keyIndex(key, len(window.Rows))
		if !ok {
			return nil, fmt.Errorf("table %s: key %q was not asked", def.Stage, key)
		}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("table %s: key %q answered twice", def.Stage, key)
		}
		seen[key] = struct{}{}
		if len(cells) != len(def.Columns)+1 {
			return nil, fmt.Errorf("table %s: row %s has %d cells, %d columns asked", def.Stage, key, len(cells)-1, len(def.Columns))
		}
		answer := make(Answer, len(def.Columns))
		for _, column := range def.Columns {
			cellRaw, ok := cells[column.Name]
			if !ok {
				return nil, fmt.Errorf("table %s: row %s has no %q cell", def.Stage, key, column.Name)
			}
			var cell string
			if err := json.Unmarshal(cellRaw, &cell); err != nil {
				return nil, fmt.Errorf("table %s: row %s cell %q is not a string", def.Stage, key, column.Name)
			}
			value, err := normalizeCell(column, window.Rows[index], cell)
			if err != nil {
				return nil, fmt.Errorf("table %s: row %s: %w", def.Stage, key, err)
			}
			answer[column.Name] = value
		}
		answers[index] = answer
	}
	return answers, nil
}

func keyIndex(key string, count int) (int, bool) {
	if !strings.HasPrefix(key, "r") {
		return 0, false
	}
	var index int
	if _, err := fmt.Sscanf(key, "r%d", &index); err != nil || index < 1 || index > count {
		return 0, false
	}
	if Key(index-1) != key {
		return 0, false
	}
	return index - 1, true
}

func normalizeCell(column Column, row Row, cell string) (string, error) {
	if column.Kind == Prose {
		text := strings.TrimSpace(cell)
		if text == "" {
			return "", fmt.Errorf("cell %q is empty", column.Name)
		}
		return text, nil
	}
	text := collapse(cell)
	switch column.Kind {
	case Sequence:
		if text == "none" {
			return "", nil
		}
		options := make(map[string]bool)
		for _, option := range rowOptions(row, column.OptionsFrom) {
			options[option] = true
		}
		var selected []string
		seen := make(map[string]bool)
		for _, ref := range strings.Fields(text) {
			if options[ref] && !seen[ref] {
				selected = append(selected, ref)
				seen[ref] = true
			}
		}
		if len(selected) == 0 {
			return "", fmt.Errorf("cell %q has no known choices; use none for an empty selection", column.Name)
		}
		limit := 0
		for _, field := range row.Fields {
			if field.Name == column.LimitFrom {
				limit, _ = field.Value.(int)
			}
		}
		if column.LimitFrom != "" && (limit < 1 || len(selected) > limit) {
			return "", fmt.Errorf("cell %q chooses %d items, limit %d", column.Name, len(selected), limit)
		}
		return strings.Join(selected, " "), nil
	case Text:
		if text == "" {
			return "", fmt.Errorf("cell %q is empty", column.Name)
		}
		if column.MaxRunes > 0 {
			text = cutRunes(text, column.MaxRunes)
		}
		return text, nil
	case Choice:
		options := column.Options
		if column.OptionsFrom != "" {
			options = rowOptions(row, column.OptionsFrom)
		}
		for _, option := range options {
			if strings.EqualFold(text, option) {
				return option, nil
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
		if column.Free != "" && len(text) > len(column.Free) &&
			strings.EqualFold(text[:len(column.Free)], column.Free) {
			rest := collapse(text[len(column.Free):])
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

func rowOptions(row Row, name string) []string {
	for _, field := range row.Fields {
		if field.Name != name {
			continue
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

// labelFromProse takes the first sentence of a prose cell as a short label,
// or the prose cut to the limit when that sentence is still too long.
func labelFromProse(text string, limit int) string {
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
	row := map[string]string{"key": Key(0)}
	hasProse := false
	for _, column := range def.Columns {
		row[column.Name] = "<computed " + column.Name + ">"
		// A conditional prose branch is still eligible: the model decides which
		// branch applies, so row preparation must not erase its glossary support.
		hasProse = hasProse || column.Kind == Text || column.Kind == Prose
	}
	example, err := json.Marshal(struct {
		Rows []map[string]string `json:"rows"`
	}{[]map[string]string{row}})
	if err != nil {
		return llm.Call[Answers]{}, err
	}
	return llm.Call[Answers]{
		State: state,
		Prompt: llm.Prompt{
			System: def.System, User: string(window.Request), ResponseFormatJSON: true,
			Reasoning: def.Reasoning, ResponseExample: string(example), NoResponseAdjunct: !hasProse,
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

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// SortRows orders rows by ID so windows are stable across runs.
func SortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
}
