package groupindex

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
	"sort"
)

// DataRecord carries the original atlas extraction plus an optional native
// owner resolved by exact source location, never by matching a class name.
type DataRecord struct {
	atlas.DataRecord
	OwnerSubjectID string `json:"owner_subject_id,omitempty"`
}

func projectData(program programindex.Index, rows []atlas.DataRecord) []DataRecord {
	type sourceKey struct {
		path string
		line int
	}
	owners := make(map[sourceKey][]programindex.Object)
	for _, object := range program.Objects {
		if (object.Kind == programindex.ObjectType || object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod || object.Kind == programindex.ObjectLambda) && object.Location != nil {
			key := sourceKey{object.Location.Path, object.Location.Line}
			owners[key] = append(owners[key], object)
		}
	}
	out := make([]DataRecord, 0, len(rows))
	for _, row := range atlas.CloneDataRecords(rows) {
		record := DataRecord{DataRecord: row}
		if row.Data != nil && row.Data.Owner != nil {
			anchor := row.Data.Owner
			var matches []string
			for _, object := range owners[sourceKey{anchor.Path, anchor.Line}] {
				if anchor.Column > 0 && object.Location.Column != anchor.Column {
					continue
				}
				if row.Data.Kind == "table" && object.Kind != programindex.ObjectType {
					continue
				}
				matches = append(matches, object.ID)
			}
			if len(matches) == 1 {
				record.OwnerSubjectID = matches[0]
			}
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func cloneData(rows []DataRecord) []DataRecord {
	out := append([]DataRecord(nil), rows...)
	for i := range out {
		out[i].DataRecord = atlas.CloneDataRecords([]atlas.DataRecord{rows[i].DataRecord})[0]
	}
	return out
}
func (index Index) validateData(subjects map[string]Subject) error {
	known := map[string]bool{}
	for i, row := range index.Data {
		if row.ID == "" || row.Path == "" || row.Line < 1 || row.Data == nil || known[row.ID] || i > 0 && index.Data[i-1].ID >= row.ID {
			return fmt.Errorf("group index: invalid data source record")
		}
		if err := row.Data.Validate(); err != nil {
			return err
		}
		known[row.ID] = true
		if row.OwnerSubjectID != "" {
			subject, ok := subjects[row.OwnerSubjectID]
			if !ok || subject.Object == nil || subject.Object.Location == nil || row.Data.Owner == nil || subject.Object.Location.Path != row.Data.Owner.Path || subject.Object.Location.Line != row.Data.Owner.Line || row.Data.Owner.Column > 0 && subject.Object.Location.Column != row.Data.Owner.Column {
				return fmt.Errorf("group index: data owner is not its exact source declaration")
			}
		}
	}
	for _, row := range index.Data {
		for _, ref := range row.References {
			if !known[ref] {
				return fmt.Errorf("group index: data relation outside target")
			}
		}
	}
	return nil
}
