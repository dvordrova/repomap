package atlas

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/facts"
)

// DataRecord preserves one extraction entity and its source relations through
// the same atlas target projection; no model selection can erase its schema.
type DataRecord struct {
	ID         string            `json:"id"`
	Path       string            `json:"path"`
	Line       int               `json:"line"`
	Data       *facts.DataObject `json:"data"`
	References []string          `json:"references,omitempty"`
}

func CloneDataRecords(rows []DataRecord) []DataRecord {
	out := append([]DataRecord(nil), rows...)
	for i := range out {
		out[i].Data = facts.CloneData(rows[i].Data)
		out[i].References = append([]string(nil), rows[i].References...)
	}
	return out
}

func ValidateDataRecords(rows []DataRecord) error {
	known := map[string]bool{}
	for i, row := range rows {
		if row.ID == "" || row.Path == "" || row.Line < 1 || row.Data == nil || known[row.ID] || i > 0 && rows[i-1].ID >= row.ID {
			return fmt.Errorf("atlas: invalid data record")
		}
		known[row.ID] = true
		if err := row.Data.Validate(); err != nil {
			return err
		}
	}
	for _, row := range rows {
		for _, ref := range row.References {
			if !known[ref] {
				return fmt.Errorf("atlas: data reference outside target")
			}
		}
	}
	return nil
}
