// Package lines defines the tables that give places their one line: the
// directory table and the file table. Each definition says what a row
// carries, what the model fills, and how the answer is applied. The row
// context is one step up: a directory row carries its parent's fallback
// line; a file row carries its directory's model line and the lines of a few
// files that call it. Nothing transitive, nothing filled to a window.
package lines

import (
	_ "embed"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	// StageDirectories and StageFiles are the debugdump stage names.
	StageDirectories = "atlas_directories"
	StageFiles       = "atlas_files"

	directoriesContract = "repomap.atlas.directories.v1"
	filesContract       = "repomap.atlas.files.v1"

	// WindowRows is how many rows one request carries.
	WindowRows = 40

	// LineRunes bounds a model line; TitleRunes a box title.
	LineRunes  = 160
	TitleRunes = 40

	maxChildren  = 40
	maxDecls     = 20
	maxSignature = 160
	maxDoc       = 200
	maxCallers   = 3
	maxSiblings  = 8

	// BoxHere is the box choice that keeps a file in its own directory;
	// BoxNew is the prefix that starts a new box.
	BoxHere = "here"
	BoxNew  = "new: "
)

//go:embed prompts/directories.md
var directoriesPrompt string

//go:embed prompts/files.md
var filesPrompt string

// Directories is the directory table.
func Directories() table.Definition {
	return table.Definition{
		Stage: StageDirectories, Contract: directoriesContract, Window: WindowRows,
		System: directoriesPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{
			{Name: "title", Kind: table.Text, MaxRunes: TitleRunes, Note: "two to four words for the box"},
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what the directory's code does"},
		},
	}
}

// Files is the file table.
func Files() table.Definition {
	return table.Definition{
		Stage: StageFiles, Contract: filesContract, Window: WindowRows,
		System: filesPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what the file does"},
			{
				Name: "box", Kind: table.Choice, OptionsFrom: "box_options",
				Free: BoxNew, FreeMaxRunes: TitleRunes,
				Note: "one of box_options, or new: followed by a title",
			},
		},
	}
}

// Lines is what the reading knows so far: the model's line per place ID,
// consulted when a row needs its parent's or its callers' lines.
type Lines interface {
	Line(placeID string) (string, bool)
}

// DirectoryRow builds the row of one directory. The parent's line is its
// fallback, not its model line, so a reworded root does not cold-start the
// whole tree.
func DirectoryRow(place atlas.Place, parent *atlas.Place) table.Row {
	facts := place.Directory
	fields := []table.Field{
		{Name: "path", Value: place.Path},
		{Name: "name", Value: displayName(place.Path)},
	}
	if facts.Readme != "" {
		fields = append(fields, table.Field{Name: "readme", Value: facts.Readme})
	}
	if facts.Doc != "" {
		fields = append(fields, table.Field{Name: "doc", Value: facts.Doc})
	}
	fields = append(fields,
		table.Field{Name: "dirs", Value: bounded(facts.Dirs, maxChildren)},
		table.Field{Name: "files", Value: bounded(facts.Files, maxChildren)},
		table.Field{Name: "file_count", Value: facts.FileCount},
	)
	if parent != nil {
		fields = append(fields, table.Field{Name: "parent", Value: parent.Given})
	}
	return table.Row{ID: place.ID, Fields: fields}
}

// FileRow builds the row of one file. Its directory's model line and its
// callers' model lines come from earlier rounds; a caller without a line
// yet is left out rather than replaced by its fallback.
func FileRow(
	place atlas.Place, directory atlas.Place, siblings []string,
	lines Lines, places map[string]atlas.Place,
) table.Row {
	facts := place.File
	fields := []table.Field{{Name: "path", Value: place.Path}}
	if facts.Doc != "" {
		fields = append(fields, table.Field{Name: "doc", Value: facts.Doc})
	}
	if line, ok := lines.Line(directory.ID); ok {
		fields = append(fields, table.Field{Name: "directory", Value: line})
	} else {
		fields = append(fields, table.Field{Name: "directory", Value: directory.Given})
	}
	callers := make([]string, 0, maxCallers)
	for _, callerID := range facts.Callers {
		line, ok := lines.Line(callerID)
		if !ok {
			continue
		}
		caller, known := places[callerID]
		if !known {
			continue
		}
		callers = append(callers, path.Base(caller.Path)+": "+line)
		if len(callers) == maxCallers {
			break
		}
	}
	if len(callers) > 0 {
		fields = append(fields, table.Field{Name: "callers", Value: callers})
	}
	decls := make([]map[string]any, 0, min(len(facts.Decls), maxDecls))
	for _, decl := range rankedDecls(facts.Decls) {
		entry := map[string]any{"name": decl.Name, "kind": decl.Kind}
		if decl.Signature != "" {
			entry["signature"] = cut(decl.Signature, maxSignature)
		}
		if decl.Doc != "" {
			entry["doc"] = cut(decl.Doc, maxDoc)
		}
		decls = append(decls, entry)
		if len(decls) == maxDecls {
			break
		}
	}
	fields = append(fields, table.Field{Name: "declarations", Value: decls})
	options := []string{BoxHere}
	options = append(options, bounded(siblings, maxSiblings)...)
	fields = append(fields, table.Field{Name: "box_options", Value: options})
	return table.Row{ID: place.ID, Fields: fields}
}

// rankedDecls puts exported and documented declarations first, then the
// rest by fan-in, so a file with more declarations than the row can carry
// shows its most telling ones.
func rankedDecls(decls []atlas.Decl) []atlas.Decl {
	ranked := append([]atlas.Decl(nil), decls...)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if (a.Exported && a.Doc != "") != (b.Exported && b.Doc != "") {
			return a.Exported && a.Doc != ""
		}
		if a.Exported != b.Exported {
			return a.Exported
		}
		if (a.Doc != "") != (b.Doc != "") {
			return a.Doc != ""
		}
		if a.FanIn != b.FanIn {
			return a.FanIn > b.FanIn
		}
		return a.LineNo < b.LineNo
	})
	return ranked
}

func displayName(dirPath string) string {
	if dirPath == "." {
		return "(repository root)"
	}
	return path.Base(dirPath)
}

func bounded(values []string, limit int) []string {
	if values == nil {
		return []string{}
	}
	if len(values) > limit {
		return values[:limit]
	}
	return values
}

func cut(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
