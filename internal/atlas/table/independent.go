package table

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
// rows have nil cells; the exact response can still be cached and
// revalidated for its accepted neighbours.
type Result struct {
	Answers    Answers        `json:"answers"`
	Rejections []RowRejection `json:"rejections,omitempty"`
	rowKeys    []string
}

// AcceptedRowKeys limits optional response metadata to rows whose required
// cells were accepted. An empty, non-nil slice accepts no row metadata.
func (result Result) AcceptedRowKeys() []string {
	keys := make([]string, 0, len(result.Answers))
	for i, answer := range result.Answers {
		if answer != nil {
			keys = append(keys, result.rowKeys[i])
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

// DecodeResult validates rows one by one. Extra envelope and cell fields have
// no role in an answer; a missing or invalid required cell refuses only its
// own row.
func DecodeResult(def Definition, window Window, raw []byte) (Result, error) {
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
	indexes, err := rowIndexes(window.Rows)
	if err != nil {
		return Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	result := Result{Answers: make(Answers, len(window.Rows)), rowKeys: make([]string, len(window.Rows))}
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
	}
	byKey := make(map[string][]map[string]json.RawMessage)
	for _, rawRow := range envelope.Rows {
		var cells map[string]json.RawMessage
		var key string
		if json.Unmarshal(rawRow, &cells) != nil {
			result.Rejections = append(result.Rejections, RowRejection{Reason: "response row has no string key"})
			continue
		}
		if json.Unmarshal(cells["key"], &key) != nil || key == "" {
			result.Rejections = append(result.Rejections, RowRejection{Reason: "response row has no string key"})
			continue
		}
		if _, known := indexes[key]; !known {
			result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: "response key was not asked"})
			continue
		}
		byKey[key] = append(byKey[key], cells)
	}
	for i, row := range window.Rows {
		key := row.ID
		cells := byKey[key]
		if len(cells) == 0 {
			result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: "row was not answered"})
			continue
		}
		// A repeat of the same answer is one answer; two different answers
		// for one row leave no decision.
		answer, err := decodeIndependentCells(def, window.Context, row, cells[0])
		for _, repeat := range cells[1:] {
			if err != nil {
				break
			}
			if again, repeatErr := decodeIndependentCells(def, window.Context, row, repeat); repeatErr != nil || !maps.Equal(answer, again) {
				err = errors.New("row key was answered more than once, differently")
			}
		}
		if err == nil {
			result.Answers[i] = answer
			continue
		}
		result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: err.Error()})
	}
	if len(window.Rows) > 0 && len(result.AcceptedRowKeys()) == 0 {
		rejection := result.Rejections[0]
		return result, fmt.Errorf("table %s: no rows accepted; row %s: %s", def.Stage, rejection.Key, rejection.Reason)
	}
	return result, nil
}

func decodeIndependentCells(def Definition, context []Field, row Row, cells map[string]json.RawMessage) (Answer, error) {
	answer := make(Answer, len(def.Columns))
	for _, column := range def.Columns {
		if column.WhenOptionsFrom != "" && len(optionsFrom(context, row, column.WhenOptionsFrom)) == 0 {
			// Empty choices have no decision to request or validate.
			continue
		}
		raw, found := cells[column.Name]
		if !found || string(raw) == "null" {
			if column.Missing != "" {
				answer[column.Name] = column.Missing
				continue
			}
			if column.Optional {
				continue
			}
		}
		if column.Optional && found {
			// A small model says "no" where it should leave the cell out.
			var cell string
			if json.Unmarshal(raw, &cell) == nil && (strings.TrimSpace(cell) == "" || strings.EqualFold(strings.TrimSpace(cell), "no") || strings.EqualFold(strings.TrimSpace(cell), "none")) {
				continue
			}
		}
		if !found {
			return nil, fmt.Errorf("missing %q cell", column.Name)
		}
		var cell string
		if err := json.Unmarshal(raw, &cell); err != nil {
			return nil, fmt.Errorf("cell %q is not a string", column.Name)
		}
		value, err := normalizeCell(column, context, row, cell)
		if err != nil {
			return nil, err
		}
		answer[column.Name] = value
	}
	return answer, nil
}
