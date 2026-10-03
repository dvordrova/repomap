package report

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The files a program keeps are its data (owner, 2026-09-29; GroupsIndex
// data records of kind file, READING § What a call reaches): the Data
// shelf lists each by its path as written, or the field it is read from,
// and the component's reading reads each with the functions reaching it,
// by part (31-reading-column.js rmComponentFiles). No role is given, no
// line number printed: a name reads its function, a path links to where
// it is written.

// pageDataFile is one file as the Data shelf lists it: its path as written,
// or the field it is read from with the paths its writes store; neither is
// a path not established. The shelf prints no functions: the column reads
// them.
type pageDataFile struct {
	ID, Path, Field string
	Values          []string
}

// pageFiles is a component's files as its reading reads them: the
// declarations once, then each file.
type pageFiles struct {
	Decls []pageReadingDecl `json:"decls"`
	Files []pageFile        `json:"files"`
}

// pageFile is one file: its path as written, else the field it is read from
// with each write of that field (the path it stores, or who writes what is
// not established, or the setting whose branch writes it); neither is the
// one line of the paths not established. By is the functions whose calls
// reach it, by part.
type pageFile struct {
	Path   string             `json:"path,omitempty"`
	Field  string             `json:"field,omitempty"`
	Values []pageFileValue    `json:"values,omitempty"`
	By     []pageReadingNames `json:"by"`
}

// pageFileValue is one write of a file's field: the path it stores, linked
// to the write with its place on hover; when that is not established, the
// setting whose branch makes the write (its name and its input's tile), else
// the function writing it.
type pageFileValue struct {
	Value   string `json:"value,omitempty"`
	Href    string `json:"href,omitempty"`
	Open    string `json:"open,omitempty"`
	At      string `json:"at,omitempty"`
	Setting string `json:"setting,omitempty"`
	Input   string `json:"input,omitempty"`
	Decl    *int   `json:"decl,omitempty"`
}

// fillSectionFiles lists a program's files on its Data shelf and writes
// their reading for its component.
func (builder *pageBuilder) fillSectionFiles(section *pageSection, index *groupindex.Index) {
	targetID := index.Target.ID
	reading := pageFiles{Decls: []pageReadingDecl{}, Files: []pageFile{}}
	at := map[string]int{}
	// declare names a function once, in the part it stands in.
	declare := func(subjectID string) (int, string, string) {
		ref, known := builder.subject(targetID, subjectID)
		if !known || ref.subject.Object == nil {
			return -1, "", ""
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		key := declarationKey(anchor)
		if label == "" || key == "" {
			return -1, "", ""
		}
		part, title := "", ""
		if group := builder.edgesBetweenGroups(*index).groupOf[subjectID]; group != "" {
			part = "#" + groupAnchorID(section.ID, group)
			title = builder.groupTitles[groupindex.Endpoint{TargetID: targetID, GroupID: group}]
		}
		if position, seen := at[key]; seen {
			return position, part, title
		}
		kind := ""
		if object := ref.subject.Object; object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod || object.Kind == programindex.ObjectLambda {
			kind = "function"
		}
		at[key] = len(reading.Decls)
		reading.Decls = append(reading.Decls, pageReadingDecl{Name: builder.withType(targetID, ref.subject, label), Key: key, Anonymous: anchor.words, Href: anchor.Href, Open: anchor.Open,
			NoSource: anchor.NoSource, Code: anchor.Code, At: anchor.Text, File: anchor.Path, Kind: kind, Part: part})
		return at[key], part, title
	}
	// by groups the functions whose calls reach a file by part, the part
	// naming most first, each function once and by name.
	by := func(groups []pageReadingNames, subjects []string) []pageReadingNames {
		for _, subject := range subjects {
			position, part, title := declare(subject)
			if position < 0 {
				continue
			}
			found := slices.IndexFunc(groups, func(group pageReadingNames) bool { return group.Part == part && group.Title == title })
			if found < 0 {
				groups = append(groups, pageReadingNames{Part: part, Title: title})
				found = len(groups) - 1
			}
			if !slices.Contains(groups[found].Decls, position) {
				groups[found].Decls = append(groups[found].Decls, position)
			}
		}
		return groups
	}
	var unknown []string
	for _, record := range index.Data {
		data := record.Data
		if data == nil || data.Kind != "file" || data.File == nil || vendoredPath(record.Path) {
			continue
		}
		if data.Name == "" {
			unknown = append(unknown, record.CallSubjectIDs...)
			continue
		}
		row := pageDataFile{ID: dataRowID(section.ID, record.ID), Path: data.Name, Field: data.File.Field}
		file := pageFile{Path: data.Name, Field: data.File.Field, By: by(nil, record.CallSubjectIDs)}
		if file.Field != "" {
			row.Path, file.Path = "", ""
			// A setting or a function writing what is not established is
			// named once, however many of its writes there are.
			named := map[string]bool{}
			for position, value := range data.File.Values {
				written := pageFileValue{Value: value.Value}
				if value.Value != "" {
					anchor := builder.links.anchor(value.Anchor.Path, value.Anchor.Line, value.Anchor.Column)
					written.Href, written.Open, written.At = anchor.Href, anchor.Open, anchor.Text
					if !slices.Contains(row.Values, value.Value) {
						row.Values = append(row.Values, value.Value)
					}
				} else if position < len(record.ValueSubjectIDs) {
					writer := record.ValueSubjectIDs[position]
					if name, input := builder.settingWriting(section, index, writer, value.Anchor.Path, value.Anchor.Line); name != "" {
						if !named["setting\x00"+input+name] {
							named["setting\x00"+input+name] = true
							written.Setting, written.Input = name, input
						}
					} else if decl, _, _ := declare(writer); decl >= 0 && !named["decl\x00"+strconv.Itoa(decl)] {
						named["decl\x00"+strconv.Itoa(decl)] = true
						written.Decl = &decl
					}
				}
				if written.Value != "" || written.Setting != "" || written.Decl != nil {
					file.Values = append(file.Values, written)
				}
			}
		}
		section.Data.Files = append(section.Data.Files, row)
		reading.Files = append(reading.Files, file)
	}
	// The paths not established are one line: the functions reaching each.
	if len(unknown) > 0 {
		section.Data.Unknown = true
		reading.Files = append(reading.Files, pageFile{By: by(nil, unknown)})
	}
	byName := func(a, b int) int {
		left, right := reading.Decls[a].Name, reading.Decls[b].Name
		return cmp.Or(strings.Compare(strings.ToLower(left), strings.ToLower(right)), strings.Compare(left, right))
	}
	for i := range reading.Files {
		groups := reading.Files[i].By
		for j := range groups {
			slices.SortFunc(groups[j].Decls, byName)
		}
		slices.SortStableFunc(groups, func(a, b pageReadingNames) int {
			return cmp.Or(cmp.Compare(len(b.Decls), len(a.Decls)), strings.Compare(a.Title, b.Title), strings.Compare(a.Part, b.Part))
		})
	}
	if len(reading.Files) == 0 {
		return
	}
	raw, err := json.Marshal(reading)
	if err != nil {
		return
	}
	section.Data.FilesReading = string(raw)
}

// settingWriting is the setting whose branch a write stands in (the lines
// the comparison naming it guards in the function declaring it, as
// settingSets reads them), by name and by its input's tile; none when the
// write is in no setting's branch.
func (builder *pageBuilder) settingWriting(section *pageSection, index *groupindex.Index, writer, path string, line int) (string, string) {
	if writer == "" {
		return "", ""
	}
	for _, operation := range index.Operations {
		if operation.Kind != "setting" || operation.DeclaredBy != writer || operation.Location.Path != path {
			continue
		}
		if branch := builder.branchAt(index.Target.ID, operation.DeclaredBy, operation.Location); branch != nil && branch.Line <= line && line <= branch.EndLine {
			return builder.operationDisplayName(index.Target.ID, operation), operationNodeID(section.ID, operation.ID)
		}
	}
	return "", ""
}
