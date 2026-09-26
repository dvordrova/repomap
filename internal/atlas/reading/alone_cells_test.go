package reading

import (
	"context"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

// cellReader is a reader over testGraph in exploration mode, as Read builds
// it, for driving one stage at a time.
func cellReader(t *testing.T, provider llm.Provider) *reader {
	t.Helper()
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.budget = true
	r.places = make(map[string]atlas.Place)
	r.directoriesByPath = make(map[string]string)
	r.lines, r.titles, r.symbolLine = make(map[string]cell), make(map[string]cell), make(map[string]cell)
	r.openDirs, r.openFiles = make(map[string]bool), make(map[string]bool)
	r.knowledge, r.knowledgeSubjects = make(map[string]*Knowledge), make(map[string]*Knowledge)
	r.knowledgeRecords = make(map[knowledgeRecordKey]*Knowledge)
	r.responseTables = make(map[string]rememberedTable)
	r.boxOf = make(map[string]string)
	for _, place := range r.opts.Graph.Places {
		r.places[place.ID] = place
		if place.Kind == atlas.PlaceDirectory {
			r.directoriesByPath[place.Path] = place.ID
		}
	}
	return r
}

// A caption or open cell refused alone takes the fallback the whole row
// takes: the given title or line, and an open that closes nothing, so the
// directory's descendants are still asked. Its row's text is not accepted as
// glossary prose. A row with no surviving cell is still refused.
func TestDirectoryAndFileCellsFailAlone(t *testing.T) {
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		paths := make(map[string]string)
		for _, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			paths[row["key"].(string)], _ = row["path"].(string)
		}
		for _, row := range rows {
			switch paths[row["key"].(string)] {
			case "pkg":
				delete(row, "open")
			case "pkg/a":
				row["title"] = ""
			case "pkg/b":
				row["title"], row["line"], row["open"] = "", " ", "maybe"
			case "pkg/a/x.go":
				row["open"] = "maybe"
			case "pkg/a/y.go":
				row["line"] = ""
			}
		}
	}
	adapter := &parsedRowAdapter{Provider: provider}
	r := cellReader(t, adapter)
	if err := r.readDirectories(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := r.readFiles(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := func(path string) string {
		for _, place := range r.places {
			if place.Path == path && place.Kind != atlas.PlaceSymbol {
				return place.ID
			}
		}
		t.Fatalf("no place %s", path)
		return ""
	}
	if r.titles[id("pkg")].value != "Title pkg" || r.lines[id("pkg")].source != atlas.SourceModel || !r.openDirs[id("pkg")] {
		t.Fatalf("a missing open refused its captions or closed the directory: %+v %+v %v", r.titles[id("pkg")], r.lines[id("pkg")], r.openDirs[id("pkg")])
	}
	if got := r.titles[id("pkg/a")]; got.value != directoryTitle("pkg/a") || got.source != atlas.SourceGiven || r.lines[id("pkg/a")].value != "Directory pkg/a does things." {
		t.Fatalf("an empty title did not fall back alone: %+v / %+v", got, r.lines[id("pkg/a")])
	}
	if got := r.lines[id("pkg/b")]; got.value != r.places[id("pkg/b")].Given || got.source != atlas.SourceGiven || !r.openDirs[id("pkg/b")] {
		t.Fatalf("a row with no valid cell kept a model value or closed: %+v", got)
	}
	x, y := id("pkg/a/x.go"), id("pkg/a/y.go")
	if r.lines[x].value != "File pkg/a/x.go does things." || !r.openFiles[x] {
		t.Fatalf("an unlisted open refused the file line or closed the file: %+v %v", r.lines[x], r.openFiles[x])
	}
	if got := r.lines[y]; got.value != r.places[y].Given || got.source != atlas.SourceGiven || !r.openFiles[y] {
		t.Fatalf("an empty file line did not fall back alone: %+v", got)
	}
	if provider.answers["pkg/b/z.go"] == 0 {
		t.Fatal("a refused open closed its directory's files")
	}
	kinds := make(map[string][]string)
	for _, row := range r.rejected {
		kinds[row.Kind] = append(kinds[row.Kind], row.Samples[0])
	}
	if !slices.Equal(kinds["cell_rejected"], []string{id("pkg"), id("pkg/a"), x, y}) || !slices.Equal(kinds["row_rejected"], []string{id("pkg/b")}) {
		t.Fatalf("refused cells and rows were not journaled apart: %+v", r.rejected)
	}
	for _, accepted := range adapter.accepted {
		for _, key := range []string{id("pkg"), id("pkg/a"), id("pkg/b"), x, y} {
			if slices.Contains(accepted, key) {
				t.Fatalf("a row with a refused cell authorized glossary prose: %v", adapter.accepted)
			}
		}
	}
}

// A target whose role is refused keeps its native fallback role, and one
// whose line is refused keeps its accepted role.
func TestTargetCellsKeepTheirFallback(t *testing.T) {
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		for _, row := range rows {
			switch row["key"] {
			case "t1":
				row["role"] = "maybe"
			case "t2":
				row["line"] = 42
			}
		}
	}
	r := cellReader(t, provider)
	r.opts.Targets = []TargetMeta{{ID: "t1", Name: "cli", Kind: "library", Root: "pkg/a"}, {ID: "t2", Name: "lib", Kind: "library", Root: "pkg/b"}}
	r.boundaries = map[string]*boundaryState{}
	if err := r.readTargets(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.targets["t1"]; got.role != atlas.RoleLibrary || got.line != "Text for t1" {
		t.Fatalf("a refused role replaced the fallback or lost the line: %+v", got)
	}
	if got := r.targets["t2"]; got.line != "" || got.role != atlas.Roles()[0] {
		t.Fatalf("a refused line invented text or lost the role: %+v", got)
	}
}

// A joint the model confirms without a label is a joint with no label; a
// joint without its same decision is refused.
func TestJointLabelMayBeEmptyButSameIsRequired(t *testing.T) {
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		for _, row := range rows {
			switch row["key"] {
			case "j1":
				row["same"], row["label"] = "yes", ""
			case "j2":
				delete(row, "same")
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Targets = []TargetMeta{{ID: "client"}, {ID: "service"}}
	r.targets = map[string]*targetState{"client": {role: atlas.RoleProduct}, "service": {role: atlas.RoleProduct}}
	r.boundaries = make(map[string]*boundaryState)
	for _, id := range []string{"out1", "out2", "in"} {
		target, direction, kind := "client", atlas.DirectionOut, atlas.BoundaryClientRequest
		if id == "in" {
			target, direction, kind = "service", atlas.DirectionIn, atlas.BoundaryRequest
		}
		r.boundaries[id] = &boundaryState{line: id + " behavior", kind: kind, place: atlas.Place{ID: id, TargetIDs: []string{target}, Boundary: &atlas.BoundaryFacts{Direction: direction, Values: []string{"shared-value"}}}}
	}
	if err := r.readJoints(t.Context()); err != nil {
		t.Fatal(err)
	}
	var confirmed []string
	for _, joint := range r.joints {
		if !joint.Blind {
			confirmed = append(confirmed, joint.From.BoundaryID+":"+joint.Label)
		}
	}
	if !slices.Equal(confirmed, []string{"out1:"}) || len(r.rejected) != 1 || r.rejected[0].Samples[0] != "j2" || !strings.Contains(r.rejected[0].Reason, `"same"`) {
		t.Fatalf("an unlabelled joint was dropped or a joint without its decision was kept: %v / %+v", confirmed, r.rejected)
	}
}

// A type's prose line may hold a line break or a tab; the atlas shows it as
// one line, which passes atlas validation and the group index text rule.
func TestTypeLineWhitespaceIsOneAtlasLine(t *testing.T) {
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		fill, _ := input["fill"].([]any)
		if input["table"] != lines.StageSymbols || len(fill) == 0 || fill[0].(map[string]any)["kind"] != "prose" {
			return
		}
		for _, row := range rows {
			row["line"] = "Z holds a thing.\n\n\tIt expires. "
		}
	}
	result, err := Read(context.Background(), readOptions(t, testGraph(t), provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatalf("a type line with whitespace failed the atlas: %v", err)
	}
	found := false
	for _, target := range result.Atlas.Targets {
		var files []atlas.File
		for _, box := range target.Boxes {
			files = append(files, box.Files...)
		}
		for _, entry := range target.OffMap {
			files = append(files, entry.File)
		}
		for _, file := range files {
			for i, symbol := range file.Symbols {
				if symbol.Name != "Z" {
					continue
				}
				found = true
				// The group index text rule: trimmed, no control character.
				if symbol.Line != "Z holds a thing. It expires." || strings.IndexFunc(symbol.Line, unicode.IsControl) >= 0 {
					t.Fatalf("type line was not one line: %q", symbol.Line)
				}
				// The symbols share the atlas's storage: a nameless one is
				// still refused.
				file.Symbols[i].Name = ""
				if err := atlas.Validate(result.Atlas); err == nil {
					t.Fatal("a symbol without a name passed validation")
				}
				file.Symbols[i].Name = "Z"
			}
		}
	}
	if !found {
		t.Fatalf("the type Z is not in the atlas: %+v", result.Atlas.Targets)
	}
}
