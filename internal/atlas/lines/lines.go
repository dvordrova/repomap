// Package lines defines the tables that give places their one line: the
// directory table and the file table. Each definition says what a row
// carries, what the model fills, and how the answer is applied. The row
// context is one step up: a directory row carries its parent's fallback
// line; a file row carries its directory's model line and deterministic
// evidence from a few direct callers. Nothing transitive, nothing filled to a window.
package lines

import (
	_ "embed"
	"fmt"
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

	directoriesContract = "repomap.atlas.directories.v4"
	filesContract       = "repomap.atlas.files.v6"

	// WindowRows is the row budget of the dependent table definitions.
	WindowRows = 40

	// LineRunes bounds a model line; TitleRunes a box title.
	LineRunes  = 160
	TitleRunes = 40

	maxChildren  = 40
	maxDecls     = 20
	maxSignature = 160
	maxDoc       = 200
	maxCallers   = 3
	// maxCallerDecls bounds the calling declarations named per caller file.
	maxCallerDecls = 3
)

//go:embed prompts/directories.md
var directoriesPrompt string

//go:embed prompts/files.md
var filesPrompt string

// Directories is the directory table.
func Directories() table.Definition {
	return table.Definition{
		Stage: StageDirectories, Contract: directoriesContract,
		System: directoriesPrompt, Memoize: true,
		Columns: []table.Column{
			// A refused caption loses only itself: the directory keeps its
			// given title or line (Alone is decoder-only).
			{Name: "title", Kind: table.Text, MaxRunes: TitleRunes, Note: "two to four words for the box", Alone: true},
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what the directory's code does", Alone: true},
		},
	}
}

// Files describes source files. Architecture reading owns membership.
func Files() table.Definition {
	return table.Definition{
		Stage: StageFiles, Contract: filesContract,
		System: filesPrompt, Memoize: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what the file does"},
		},
	}
}

// WithOpen adds the budget cell: whether the model would open this place to
// read what is beneath it. Each cell then fails alone: a refused open closes
// nothing, and a refused caption keeps the place's given one.
func WithOpen(def table.Definition) table.Definition {
	def.Contract += ".open"
	def.Columns = append([]table.Column{}, def.Columns...)
	for i := range def.Columns {
		def.Columns[i].Alone = true
	}
	def.Columns = append(def.Columns, table.Column{
		Name: "open", Kind: table.Choice, Options: []string{"yes", "no"}, Alone: true,
		Note: "yes when a reader of the architecture should look inside; no for vendored, generated, test or trivial code",
	})
	return def
}

// Lines is what the reading knows so far: the model's line per place ID,
// consulted when a row needs its directory's line.
type Lines interface {
	Line(placeID string) (string, bool)
}

// DirectoryRow builds the row of one directory. Its parent is the window's
// shared context (DirectoryContext), not a row field: the children of one
// parent are asked together, and the same README line no longer repeats in
// every row.
func DirectoryRow(place atlas.Place) table.Row {
	facts := place.Directory
	fields := []table.Field{
		{Name: "path", Value: place.Path},
		{Name: "name", Value: displayName(place.Path)},
	}
	if facts.Readme != "" {
		fields = append(fields, table.Field{Name: "readme", Value: facts.Readme})
	}
	fields = append(fields,
		table.Field{Name: "dirs", Value: bounded(facts.Dirs, maxChildren)},
		table.Field{Name: "files", Value: bounded(facts.Files, maxChildren)},
		table.Field{Name: "file_count", Value: facts.FileCount},
	)
	return table.Row{ID: place.ID, Fields: fields}
}

// DirectoryContext is what the rows of one parent share: the parent's path
// and its deterministic line. The fallback, not the model line, so a
// reworded root does not cold-start the whole tree. A root row has none.
func DirectoryContext(parent *atlas.Place) []table.Field {
	if parent == nil {
		return nil
	}
	return []table.Field{{Name: "parent", Value: map[string]any{"path": parent.Path, "line": directoryFacts(*parent)}}}
}

// directoryFacts is a directory's fallback line as a model reads it: its
// README's first line, else what it holds. Never its package doc comment,
// which the place's Given falls back to for the reader: model inputs are
// code structure, not the authors' prose (owner, 2026-09-25).
func directoryFacts(place atlas.Place) string {
	facts := place.Directory
	if facts == nil || facts.Readme != "" || facts.Doc == "" {
		return place.Given
	}
	// The Given fell back to the package doc: say what it holds instead.
	names := append(append([]string{}, facts.Dirs...), facts.Files...)
	if len(names) > 3 {
		names = names[:3]
	}
	unit := "files"
	if facts.FileCount == 1 {
		unit = "file"
	}
	if len(names) == 0 {
		return fmt.Sprintf("%d %s", facts.FileCount, unit)
	}
	return fmt.Sprintf("%d %s: %s", facts.FileCount, unit, strings.Join(names, ", "))
}

// FileCallers names, per calling file, the declarations the graph saw
// calling into a file: the witnesses of the file-to-file edges, in edge
// order. A caller without witnesses is named by its path alone; the file's
// leading declarations said nothing about the call (Morfeu's rows named the
// same three declarations for eight of nine callers).
type FileCallers map[string][]string

// FileRow uses the directory's model line and direct caller facts. Caller
// evidence never contains another file's model output, so file requests are
// independent and a reworded file does not propagate through the call graph.
func FileRow(
	place atlas.Place, directory atlas.Place,
	lines Lines, places map[string]atlas.Place, calling FileCallers,
) table.Row {
	facts := place.File
	fields := []table.Field{{Name: "path", Value: place.Path}}
	if line, ok := lines.Line(directory.ID); ok {
		fields = append(fields, table.Field{Name: "directory_hypothesis", Value: line})
	} else {
		fields = append(fields, table.Field{Name: "directory_facts", Value: directoryFacts(directory)})
	}
	callers := make([]map[string]any, 0, maxCallers)
	for _, callerID := range facts.Callers {
		caller, known := places[callerID]
		if !known || caller.File == nil {
			continue
		}
		entry := map[string]any{"path": caller.Path}
		if names := bounded(calling[callerID], maxCallerDecls); len(names) > 0 {
			entry["declarations"] = names
		}
		callers = append(callers, entry)
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
		decls = append(decls, entry)
		if len(decls) == maxDecls {
			break
		}
	}
	fields = append(fields, table.Field{Name: "declaration_count", Value: len(facts.Decls)}, table.Field{Name: "declarations", Value: decls})
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
