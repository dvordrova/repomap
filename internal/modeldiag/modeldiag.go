// Package modeldiag collects what the model-assisted stages did not accept.
// It is a log, never an authority: nothing here influences the graph, the
// report, or whether a run succeeds. It exists so a weak prompt, a badly
// shaped request, and an actual hallucination can be told apart from the run
// directory instead of by rerunning the stage.
package modeldiag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Filename is the append-only log every model stage writes into its run
// directory, one JSON object per line.
const Filename = "rejected.jsonl"

// MaxSamples bounds how many offending refs one row names. Counts stay exact.
const MaxSamples = 5

// Row is one discarded-response accounting entry. Count is exact; Samples is
// a bounded illustration. Raw carries the model's own output when a whole
// response section was refused.
type Row struct {
	Stage   string          `json:"stage"`
	Target  string          `json:"target,omitempty"`
	Kind    string          `json:"kind"`
	Count   int             `json:"count"`
	Samples []string        `json:"samples,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

// Append adds rows to the run's log without disturbing what earlier stages
// wrote. A logging failure is returned but is never a reason to fail a run;
// callers report it and continue.
func Append(runDir string, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	var buffer bytes.Buffer
	for _, row := range rows {
		if strings.TrimSpace(row.Stage) == "" || strings.TrimSpace(row.Kind) == "" {
			return fmt.Errorf("model diagnostics: stage and kind are required")
		}
		if len(row.Samples) > MaxSamples {
			row.Samples = row.Samples[:MaxSamples]
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("model diagnostics: encode row: %w", err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("model diagnostics: open writer: %w", err)
	}
	defer writer.Close()
	if err := writer.AppendFile(Filename, buffer.Bytes()); err != nil {
		return fmt.Errorf("model diagnostics: append rows: %w", err)
	}
	return nil
}

// Read restores the log. A missing file means no stage discarded anything.
func Read(runDir string) ([]Row, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, Filename))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("model diagnostics: read: %w", err)
	}
	rows := []Row{}
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var row Row
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("model diagnostics: decode row: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Summary renders the console breakdown: one line per kind, largest first.
func Summary(rows []Row) []string {
	if len(rows) == 0 {
		return nil
	}
	type total struct {
		count   int
		samples []string
	}
	byKind := map[string]*total{}
	order := []string{}
	for _, row := range rows {
		key := row.Stage + " " + row.Kind
		entry, seen := byKind[key]
		if !seen {
			entry = &total{}
			byKind[key] = entry
			order = append(order, key)
		}
		entry.count += row.Count
		for _, sample := range row.Samples {
			if len(entry.samples) < MaxSamples {
				entry.samples = append(entry.samples, sample)
			}
		}
	}
	lines := make([]string, 0, len(order))
	for _, key := range order {
		entry := byKind[key]
		line := fmt.Sprintf("%s: %d", key, entry.count)
		if len(entry.samples) > 0 {
			line += " (" + strings.Join(entry.samples, ", ") + ")"
		}
		lines = append(lines, line)
	}
	return lines
}
