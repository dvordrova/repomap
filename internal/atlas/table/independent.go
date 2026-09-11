package table

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// RowRejection names the response key that could not supply a valid answer.
// Key is empty when a response row could not be associated with any request row.
type RowRejection struct {
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
}

// Result retains accepted cells at their original request positions. Refused
// independent rows have nil cells; the exact response can still be cached and
// revalidated for its accepted neighbours.
type Result struct {
	Answers     Answers        `json:"answers"`
	Rejections  []RowRejection `json:"rejections,omitempty"`
	independent bool
}

// AcceptedRowKeys limits optional response metadata to rows whose required
// cells were accepted. An empty, non-nil slice accepts no row metadata.
func (result Result) AcceptedRowKeys() []string {
	if !result.independent {
		return nil
	}
	keys := make([]string, 0, len(result.Answers))
	for i, answer := range result.Answers {
		if answer != nil {
			keys = append(keys, Key(i))
		}
	}
	return keys
}

func (result Result) ResponseRejections() []llm.ResponseRejection {
	var rejections []llm.ResponseRejection
	for _, row := range result.Rejections {
		rejections = append(rejections, llm.ResponseRejection{Kind: "row_rejected", Count: 1, Reason: row.Reason, Samples: []string{row.Key}})
	}
	return rejections
}

// DecodeResult keeps coupled decisions atomic and validates independent rows
// one by one. Extra envelope and cell fields have no role in an independent
// answer; a missing or invalid required cell refuses only its own row.
func DecodeResult(def Definition, window Window, raw []byte) (Result, error) {
	if !def.Independent {
		answers, err := decodeWindow(def, window, raw)
		return Result{Answers: answers}, err
	}
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return Result{}, err
	}
	var envelope struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(normalized, &envelope); err != nil || envelope.Rows == nil {
		return Result{}, fmt.Errorf("table %s: response is not {\"rows\": [...]}", def.Stage)
	}
	result := Result{Answers: make(Answers, len(window.Rows)), independent: true}
	byKey := make(map[string][]map[string]json.RawMessage)
	positional := keylessInAskedOrder(envelope.Rows, len(window.Rows))
	for i, rawRow := range envelope.Rows {
		var cells map[string]json.RawMessage
		var key string
		if json.Unmarshal(rawRow, &cells) != nil {
			result.Rejections = append(result.Rejections, RowRejection{Reason: "response row has no string key"})
			continue
		}
		if positional {
			key = Key(i)
		} else if json.Unmarshal(cells["key"], &key) != nil || key == "" {
			result.Rejections = append(result.Rejections, RowRejection{Reason: "response row has no string key"})
			continue
		}
		if _, known := keyIndex(key, len(window.Rows)); !known {
			result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: "response key was not asked"})
			continue
		}
		byKey[key] = append(byKey[key], cells)
	}
	for i, row := range window.Rows {
		key := Key(i)
		cells := byKey[key]
		var reason string
		switch len(cells) {
		case 0:
			reason = "row was not answered"
		case 1:
			answer, err := decodeIndependentCells(def, row, cells[0])
			if err == nil {
				result.Answers[i] = answer
				continue
			}
			reason = err.Error()
		default:
			reason = "row key was answered more than once"
		}
		result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: reason})
	}
	if len(window.Rows) > 0 && len(result.AcceptedRowKeys()) == 0 {
		rejection := result.Rejections[0]
		return result, fmt.Errorf("table %s: no rows accepted; row %s: %s", def.Stage, rejection.Key, rejection.Reason)
	}
	return result, nil
}

// keylessInAskedOrder reports a response that answers exactly one row per
// asked row and carries no key on any of them. The model drops the key it
// was told to copy in small answer windows (ten of 242 Freqtrade windows,
// one to three questions each); with one row per asked row and no key
// anywhere, the asked order is the only reading. A partial or partly keyed
// response keeps the strict rule: every row names its key or is refused.
func keylessInAskedOrder(rows []json.RawMessage, asked int) bool {
	if asked == 0 || len(rows) != asked {
		return false
	}
	for _, rawRow := range rows {
		var cells map[string]json.RawMessage
		if json.Unmarshal(rawRow, &cells) != nil {
			return false
		}
		if _, keyed := cells["key"]; keyed {
			return false
		}
	}
	return true
}

func decodeIndependentCells(def Definition, row Row, cells map[string]json.RawMessage) (Answer, error) {
	answer := make(Answer, len(def.Columns))
	processed := make(map[string]bool, len(def.Columns))
	var deferred []Column
	for _, column := range def.Columns {
		active := true
		if column.WhenOptionsFrom != "" {
			active = len(rowOptions(row, column.WhenOptionsFrom)) > 0
		}
		for name, expected := range column.When {
			value, known := answer[name]
			if !processed[name] {
				return nil, fmt.Errorf("cell %q depends on unvalidated %q", column.Name, name)
			}
			// An earlier inactive cell cannot activate a dependent branch.
			active = active && known && value == expected
		}
		processed[column.Name] = true
		if !active {
			continue
		}
		raw, found := cells[column.Name]
		if (!found || string(raw) == "null") && column.Missing != "" {
			answer[column.Name] = column.Missing
			continue
		}
		if !found {
			if column.EmptyFrom != "" {
				deferred = append(deferred, column)
				continue
			}
			return nil, fmt.Errorf("missing %q cell", column.Name)
		}
		var cell string
		if err := json.Unmarshal(raw, &cell); err != nil {
			return nil, fmt.Errorf("cell %q is not a string", column.Name)
		}
		value, err := normalizeCell(column, row, cell)
		if err != nil {
			if column.EmptyFrom != "" && strings.TrimSpace(cell) == "" {
				deferred = append(deferred, column)
				continue
			}
			return nil, err
		}
		answer[column.Name] = value
	}
	// A provider that fills every schema key returns null for a label the
	// model skipped; the model's own description of the same row is the
	// nearest honest label. Nothing is invented: an empty source still
	// refuses the row.
	for _, column := range deferred {
		source := answer[column.EmptyFrom]
		if source == "" {
			return nil, fmt.Errorf("cell %q is empty", column.Name)
		}
		answer[column.Name] = LabelFromProse(source, column.MaxRunes)
		answer[column.Name+"_from"] = column.EmptyFrom
	}
	return answer, nil
}
