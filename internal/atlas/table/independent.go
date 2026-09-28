package table

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// RowRejection names the response key that could not supply a valid answer.
// Key is empty when a response row could not be associated with any request
// row. Cell names the one cell refused when the row itself was kept.
type RowRejection struct {
	Key    string `json:"key,omitempty"`
	Cell   string `json:"cell,omitempty"`
	Reason string `json:"reason"`
}

// Result retains accepted cells at their original request positions. Refused
// rows have nil cells; the exact response can still be cached and
// revalidated for its accepted neighbours.
type Result struct {
	Answers    Answers        `json:"answers"`
	Rejections []RowRejection `json:"rejections,omitempty"`
	rowKeys    []string
	// partial marks accepted rows that lost a cell: their written text is not
	// accepted as glossary prose.
	partial []bool
	// uncertain marks rows a decision model answered, but not clearly
	// enough to take: an explicit answer that the row stays undecided.
	uncertain []bool
}

// Uncertain reports a row without an answer because every decision it
// missed was a near-tie: the response answered it, uncertainly.
func (result Result) Uncertain(row int) bool {
	return row < len(result.uncertain) && result.uncertain[row]
}

// AcceptedRowKeys limits optional response metadata to rows accepted with
// every cell they answered. An empty, non-nil slice accepts no row metadata.
func (result Result) AcceptedRowKeys() []string {
	keys := make([]string, 0, len(result.Answers))
	for i, answer := range result.Answers {
		if answer != nil && (i >= len(result.partial) || !result.partial[i]) {
			keys = append(keys, result.rowKeys[i])
		}
	}
	return keys
}

// RefuseCell discards one cell of an accepted row that the owner's own check
// found without authority, recording why. The row keeps its other cells and
// authorizes no glossary prose.
func (result *Result) RefuseCell(row int, column, reason string) {
	delete(result.Answers[row], column)
	if len(result.partial) != len(result.Answers) {
		result.partial = make([]bool, len(result.Answers))
	}
	result.partial[row] = true
	result.Rejections = append(result.Rejections, RowRejection{Key: result.rowKeys[row], Cell: column, Reason: fmt.Sprintf("cell %q: %s", column, reason)})
}

// Accepted counts the rows the result accepted, whole or without a cell.
func (result Result) Accepted() int {
	accepted := 0
	for _, answer := range result.Answers {
		if answer != nil {
			accepted++
		}
	}
	return accepted
}

func (result Result) ResponseRejections() []llm.ResponseRejection {
	var rejections []llm.ResponseRejection
	for _, row := range result.Rejections {
		kind := "row_rejected"
		if row.Cell != "" {
			kind = "cell_rejected"
		}
		rejections = append(rejections, llm.ResponseRejection{Kind: kind, Count: 1, Reason: row.Reason, Samples: []string{row.Key}})
	}
	return rejections
}

// NoRowsAccepted refuses a response in which no row could be accepted, so it
// is never cached as an answer. It keeps every row's own reason for the
// journal.
type NoRowsAccepted struct {
	Stage      string
	Rejections []RowRejection
}

func (refusal *NoRowsAccepted) Error() string {
	if len(refusal.Rejections) == 0 {
		return fmt.Sprintf("table %s: no rows accepted", refusal.Stage)
	}
	first := refusal.Rejections[0]
	return fmt.Sprintf("table %s: no rows accepted; row %s: %s", refusal.Stage, first.Key, first.Reason)
}

// ResponseRows reads the rows of a table response. The contract's form is
// {"rows": [...]}; a bare array of rows and a root whose only field is an
// object holding rows are the same rows in a wrapper. Rows stay matched by
// key alone. No rows array, or a null one, is refused.
func ResponseRows(normalized []byte) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	if json.Unmarshal(normalized, &rows) == nil && rows != nil {
		return rows, nil
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(normalized, &root) != nil {
		return nil, errors.New(`response is not {"rows": [...]}`)
	}
	value, found := rowsField(root)
	if !found && len(root) == 1 {
		for _, inner := range root {
			var wrapped map[string]json.RawMessage
			if json.Unmarshal(inner, &wrapped) == nil {
				value, found = rowsField(wrapped)
			}
		}
	}
	if !found || json.Unmarshal(value, &rows) != nil || rows == nil {
		return nil, errors.New(`response is not {"rows": [...]}`)
	}
	return rows, nil
}

// rowsField finds the rows field the way encoding/json finds a struct field:
// the exact name first, then one spelled in another case.
func rowsField(object map[string]json.RawMessage) (json.RawMessage, bool) {
	if value, found := object["rows"]; found {
		return value, true
	}
	for name, value := range object {
		if strings.EqualFold(name, "rows") {
			return value, true
		}
	}
	return nil, false
}

// DecodeResult validates rows one by one. Extra envelope and cell fields have
// no role in an answer; a missing or invalid required cell refuses only its
// own row, and an Alone or Optional cell only itself.
func DecodeResult(def Definition, window Window, raw []byte) (Result, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return Result{}, err
	}
	rows, err := ResponseRows(normalized)
	if err != nil {
		return Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	indexes, err := rowIndexes(window.Rows)
	if err != nil {
		return Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
	}
	result := Result{Answers: make(Answers, len(window.Rows)), rowKeys: make([]string, len(window.Rows)), partial: make([]bool, len(window.Rows))}
	for i, row := range window.Rows {
		result.rowKeys[i] = row.ID
	}
	byKey := make(map[string][]map[string]json.RawMessage)
	for _, rawRow := range rows {
		var cells map[string]json.RawMessage
		var key string
		if json.Unmarshal(rawRow, &cells) != nil || json.Unmarshal(cells["key"], &key) != nil || strings.TrimSpace(key) == "" {
			result.Rejections = append(result.Rejections, RowRejection{Reason: "response row has no string key"})
			continue
		}
		key = strings.TrimSpace(key)
		if _, known := indexes[key]; !known {
			result.Rejections = append(result.Rejections, RowRejection{Key: key, Reason: "response key was not asked"})
			continue
		}
		byKey[key] = append(byKey[key], cells)
	}
	accepted := 0
	for i, row := range window.Rows {
		copies := byKey[row.ID]
		if len(copies) == 0 {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Reason: "row was not answered"})
			continue
		}
		answer, refused, err := decodeCopies(def, window.Context, row, copies)
		if err != nil {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Reason: err.Error()})
			continue
		}
		result.Answers[i] = answer
		result.partial[i] = len(refused) > 0
		accepted++
		for _, cell := range refused {
			result.Rejections = append(result.Rejections, RowRejection{Key: row.ID, Cell: cell.column, Reason: fmt.Sprintf("cell %q: %s", cell.column, cell.reason)})
		}
	}
	if len(window.Rows) > 0 && accepted == 0 {
		return result, &NoRowsAccepted{Stage: def.Stage, Rejections: result.Rejections}
	}
	return result, nil
}

// cellRefusal is one refused Alone or Optional cell of a kept row.
type cellRefusal struct {
	column, reason string
}

// decodeCopies reads every copy of one row's answer. A repeat of the same
// answer is one answer. Copies that differ only in Alone cells lose those
// cells; a difference in any other cell leaves no decision for the row.
func decodeCopies(def Definition, context []Field, row Row, copies []map[string]json.RawMessage) (Answer, []cellRefusal, error) {
	answers := make([]Answer, len(copies))
	var refused []cellRefusal
	written := make(map[string]bool)
	for i, cells := range copies {
		answer, cellRefused, wrote, err := decodeCells(def, context, row, cells)
		if err != nil {
			if len(copies) > 1 {
				return nil, nil, fmt.Errorf("row key was answered more than once, differently: %w", err)
			}
			return nil, nil, err
		}
		answers[i] = answer
		for name := range wrote {
			written[name] = true
		}
		for _, cell := range cellRefused {
			if !slices.ContainsFunc(refused, func(known cellRefusal) bool { return known.column == cell.column }) {
				refused = append(refused, cell)
			}
		}
	}
	answer := answers[0]
	for _, again := range answers[1:] {
		for _, column := range def.Columns {
			value, present := answer[column.Name]
			other, otherPresent := again[column.Name]
			if present == otherPresent && value == other {
				continue
			}
			if !column.Alone {
				return nil, nil, errors.New("row key was answered more than once, differently")
			}
			delete(answer, column.Name)
			if !slices.ContainsFunc(refused, func(known cellRefusal) bool { return known.column == column.Name }) {
				refused = append(refused, cellRefusal{column.Name, "the row's copies differ"})
			}
		}
	}
	// A value the column fills in for an absent cell, such as an address's
	// unknown, is no answer: a row whose every written cell was refused has
	// none, and is not remembered as answered.
	decided := false
	for name := range answer {
		decided = decided || written[name]
	}
	if len(refused) > 0 && !decided {
		return nil, nil, fmt.Errorf("every cell was refused; cell %q: %s", refused[0].column, refused[0].reason)
	}
	return answer, refused, nil
}

// decodeCells reads one copy of a row. A cell without Alone or Optional that
// fails refuses the copy; an Alone or Optional one is refused by itself. The
// written set names the kept cells the model wrote a value for.
func decodeCells(def Definition, context []Field, row Row, cells map[string]json.RawMessage) (Answer, []cellRefusal, map[string]bool, error) {
	answer := make(Answer, len(def.Columns))
	written := make(map[string]bool, len(def.Columns))
	var refused []cellRefusal
	for _, column := range def.Columns {
		if column.WhenOptionsFrom != "" && len(optionsFrom(context, row, column.WhenOptionsFrom)) == 0 {
			// Empty choices have no decision to request or validate.
			continue
		}
		value, present, wrote, err := decodeCell(column, context, row, cells)
		if err != nil {
			if column.Alone || column.Optional {
				refused = append(refused, cellRefusal{column.Name, err.Error()})
				continue
			}
			return nil, nil, nil, err
		}
		if present {
			answer[column.Name] = value
		}
		if present && wrote {
			written[column.Name] = true
		}
	}
	return answer, refused, written, nil
}

// decodeCell reads one cell: its value, whether the row has one, and whether
// the model wrote it rather than the column filling in its own absence value.
func decodeCell(column Column, context []Field, row Row, cells map[string]json.RawMessage) (string, bool, bool, error) {
	raw, found := cells[column.Name]
	null := found && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
	if !found || null {
		switch {
		case column.Missing != "":
			return column.Missing, true, false, nil
		case column.Optional:
			return "", false, false, nil
		case column.EmptyValue != "":
			// The column's own spelling of absent prose.
			return column.EmptyValue, true, false, nil
		case !found:
			return "", false, false, fmt.Errorf("missing %q cell", column.Name)
		}
		raw = json.RawMessage(`""`)
	}
	cell, absent, err := cellText(column, context, row, raw)
	if err != nil || absent {
		return "", false, false, err
	}
	if column.Optional {
		// A small model says "no" where it should leave the cell out.
		if trimmed := strings.TrimSpace(cell); trimmed == "" || strings.EqualFold(trimmed, "no") || strings.EqualFold(trimmed, "none") {
			return "", false, false, nil
		}
	}
	value, err := normalizeCell(column, context, row, cell)
	if err != nil {
		return "", false, false, err
	}
	// An empty list is a written empty selection; an empty string is not.
	written := strings.TrimSpace(cell) != "" || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("["))
	return value, true, written, nil
}

// cellText reads a cell's JSON value as the text the column validates. A
// string is itself; a list of strings is a Sequence's selection; on a choice,
// true is yes, and false is no where no is an option or no value on an
// Optional column. Any other value is not an answer for the cell.
func cellText(column Column, context []Field, row Row, raw json.RawMessage) (string, bool, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, false, nil
	}
	var list []string
	if column.Kind == Sequence && json.Unmarshal(raw, &list) == nil {
		return strings.Join(list, " "), false, nil
	}
	var flag bool
	if column.Kind == Choice && json.Unmarshal(raw, &flag) == nil {
		options := column.Options
		if column.OptionsFrom != "" {
			options = optionsFrom(context, row, column.OptionsFrom)
		}
		switch {
		case flag:
			return "yes", false, nil
		case slices.Contains(options, "no"):
			return "no", false, nil
		case column.Optional:
			return "", true, nil
		}
	}
	return "", false, fmt.Errorf("cell %q is not a string", column.Name)
}
