package groupindex

import (
	"fmt"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// DataRecord carries the original atlas extraction plus an optional native
// owner resolved by exact source location, never by matching a class name.
// A file (kind file) names, for each of its calls and each of its field's
// values in their order, the subject making that call or write: its
// declaration, else the function holding the site ("" when none does).
type DataRecord struct {
	atlas.DataRecord
	OwnerSubjectID  string   `json:"owner_subject_id,omitempty"`
	CallSubjectIDs  []string `json:"call_subject_ids,omitempty"`
	ValueSubjectIDs []string `json:"value_subject_ids,omitempty"`
}

// projectData projects a program's data records; subjectAt names the
// subject of a declaration an atlas object reference names, else the one
// holding a site (nil names none).
func projectData(program programindex.Index, rows []atlas.DataRecord, subjectAt func(ref string, at facts.Anchor) string) []DataRecord {
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
		if row.Data != nil && row.Data.File != nil {
			at := func(ref string, anchor facts.Anchor) string {
				if subjectAt == nil {
					return ""
				}
				return subjectAt(ref, anchor)
			}
			for _, call := range row.Data.File.Calls {
				record.CallSubjectIDs = append(record.CallSubjectIDs, at(call.ObjectID, call.Anchor))
			}
			for _, value := range row.Data.File.Values {
				record.ValueSubjectIDs = append(record.ValueSubjectIDs, at(value.ObjectID, value.Anchor))
			}
		}
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
	sort.Slice(out, func(i, j int) bool { return dataIDLess(out[i].ID, out[j].ID) })
	return out
}

// dataIDLess orders data records as the atlas does: an extraction's y*
// records and the files' w* records each by ordinal, the files first.
func dataIDLess(left, right string) bool {
	if left == "" || right == "" || left[0] != right[0] {
		return left < right
	}
	return compactIDLess(left, right, left[:1])
}
func cloneData(rows []DataRecord) []DataRecord {
	out := append([]DataRecord(nil), rows...)
	for i := range out {
		out[i].DataRecord = atlas.CloneDataRecords([]atlas.DataRecord{rows[i].DataRecord})[0]
		out[i].CallSubjectIDs = append([]string(nil), rows[i].CallSubjectIDs...)
		out[i].ValueSubjectIDs = append([]string(nil), rows[i].ValueSubjectIDs...)
	}
	return out
}
func (index Index) validateData(subjects map[string]Subject) error {
	known := map[string]bool{}
	for i, row := range index.Data {
		if row.ID == "" || row.Path == "" || row.Line < 1 || row.Data == nil || known[row.ID] || i > 0 && !dataIDLess(index.Data[i-1].ID, row.ID) {
			return fmt.Errorf("group index: invalid data source record")
		}
		if err := row.Data.Validate(); err != nil {
			return err
		}
		known[row.ID] = true
		if file := row.Data.File; file != nil || len(row.CallSubjectIDs)+len(row.ValueSubjectIDs) > 0 {
			if file == nil || len(row.CallSubjectIDs) != len(file.Calls) || len(row.ValueSubjectIDs) != len(file.Values) {
				return fmt.Errorf("group index: a data file names no subject for each of its sites")
			}
			for _, id := range append(slices.Clone(row.CallSubjectIDs), row.ValueSubjectIDs...) {
				if _, ok := subjects[id]; id != "" && !ok {
					return fmt.Errorf("group index: a data file names an unknown subject")
				}
			}
		}
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
