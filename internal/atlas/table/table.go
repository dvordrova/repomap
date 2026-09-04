// Package table is the one request shape the atlas asks the model with: a
// keyed table. Rows go in with keys the code assigned; the same keys come
// back with one to three short cells each. The code checks that every key
// returned exactly once, that no cell is missing or extra, that closed
// choices are in their list, and rejects the whole window otherwise. Nothing
// here knows what a directory or a file is.
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

// Kind is what a cell may hold.
type Kind string

const (
	// Text is one short line; longer answers are cut at a word.
	Text Kind = "text"
	// Choice is one option from a closed list.
	Choice Kind = "choice"
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
	// Free is a prefix after which the model may write its own short text,
	// such as "new: " for a title the code has not seen. Empty means no.
	Free         string `json:"free,omitempty"`
	FreeMaxRunes int    `json:"free_max_runes,omitempty"`
	// Note is one sentence the model reads about this cell.
	Note string `json:"note,omitempty"`
}

// Definition is one table: its stage name, window size, prompt and columns.
type Definition struct {
	// Stage is the debugdump stage of every window of this table.
	Stage string
	// Contract versions the prompt and the columns; it is part of the cache
	// identity so a changed table never reads an old answer.
	Contract string
	Window   int
	System   string
	Columns  []Column
	// MaxOutputTokens bounds the answer; a window that overflows it is cut
	// in half by the caller, never retried as is.
	MaxOutputTokens int
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

// Window is one request: up to Definition.Window rows with keys r1..rN.
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

// Windows cuts rows into windows of the definition's size, keeping the
// caller's order. Boundaries are the caller's business: it sorts rows by path
// so a window rarely straddles a directory.
func Windows(def Definition, round int, rows []Row) ([]Window, error) {
	return WindowsWithContext(def, round, nil, rows)
}

// WindowsWithContext is Windows with fields every window carries.
func WindowsWithContext(def Definition, round int, context []Field, rows []Row) ([]Window, error) {
	if def.Window < 1 {
		return nil, fmt.Errorf("table %s: window size %d", def.Stage, def.Window)
	}
	var windows []Window
	for start := 0; start < len(rows); start += def.Window {
		end := min(start+def.Window, len(rows))
		window := Window{Stage: def.Stage, Round: round, Index: len(windows), Context: context, Rows: rows[start:end]}
		request, err := Request(def, window)
		if err != nil {
			return nil, err
		}
		window.Request = request
		windows = append(windows, window)
	}
	return windows, nil
}

// Request is the exact user prompt of a window: one JSON object with the
// table name, the columns to fill and the rows. Field order is the caller's.
func Request(def Definition, window Window) ([]byte, error) {
	var out bytes.Buffer
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
		if column.Free != "" {
			spec["free_prefix"] = column.Free
		}
		if column.Note != "" {
			spec["note"] = column.Note
		}
		writeJSON(&out, spec)
	}
	out.WriteString("]")
	if len(window.Context) > 0 {
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
	out.WriteString(",\n  \"rows\": [\n")
	for i, row := range window.Rows {
		if i > 0 {
			out.WriteString(",\n")
		}
		out.WriteString("    {\"key\": ")
		writeJSON(&out, Key(i))
		for _, field := range row.Fields {
			out.WriteString(", ")
			writeJSON(&out, field.Name)
			out.WriteString(": ")
			writeJSON(&out, field.Value)
		}
		out.WriteString("}")
	}
	out.WriteString("\n  ]\n}\n")
	return out.Bytes(), nil
}

func writeJSON(out *bytes.Buffer, value any) {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	var buffer bytes.Buffer
	encoder = json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		out.WriteString("null")
		return
	}
	out.Write(bytes.TrimRight(buffer.Bytes(), "\n"))
}

// Answer is what one row came back with: a value per column.
type Answer map[string]string

// Answers are the window's rows in request order.
type Answers []Answer

// Decode reads a window's response and checks it against the definition and
// the rows: every key exactly once, every column filled, nothing else.
func Decode(def Definition, window Window, raw []byte) (Answers, error) {
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
	text := collapse(cell)
	switch column.Kind {
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

// State is the cache identity of one window: the table contract, the prompt
// digest and the request digest. The run, the target and the clock are not
// in it, so an unchanged row is answered from the cache across runs.
func State(def Definition, window Window) ([]byte, error) {
	return json.Marshal(struct {
		Contract string `json:"contract"`
		Prompt   string `json:"prompt_sha256"`
		Request  string `json:"request_sha256"`
	}{
		Contract: def.Contract,
		Prompt:   sha256Hex([]byte(def.System)),
		Request:  sha256Hex(window.Request),
	})
}

// Call is the executable form of one window.
func Call(def Definition, window Window) (llm.Call[Answers], error) {
	state, err := State(def, window)
	if err != nil {
		return llm.Call[Answers]{}, err
	}
	maxOutput := def.MaxOutputTokens
	if maxOutput <= 0 {
		maxOutput = 8192
	}
	return llm.Call[Answers]{
		State: state,
		Prompt: llm.Prompt{
			System: def.System, User: string(window.Request), ResponseFormatJSON: true,
		},
		Limits: llm.Limits{
			MaxRequestBytes:  llm.SemanticRecordByteLimit,
			MaxResponseBytes: llm.ProviderResponseByteLimit,
			MaxOutputTokens:  maxOutput,
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
