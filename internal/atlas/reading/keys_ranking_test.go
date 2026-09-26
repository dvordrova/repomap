package reading

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// The keys prompt says declarations "lists everything the part holds", and
// the core rows carry the declarations a part holds. A part of thirteen
// declarations was sent twelve of them: the list is names only and goes
// whole to both tables. The tables are prepared without a model, so the
// saved inputs are what any provider would be sent.
func TestKeysAndCoreListEveryDeclarationOfAPart(t *testing.T) {
	var declarations []any
	r := answerTestReader(t, nil, nil)
	r.dry, r.opts.Through = true, ""
	r.opts.Targets = []TargetMeta{{ID: "t", Name: "service"}}
	r.opts.Graph = atlas.Graph{}
	r.places = map[string]atlas.Place{}
	r.boxes = map[string]*boxState{}
	r.boundaries = map[string]*boundaryState{}
	r.selectedKeys, r.partKeys, r.keysDecided = map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, part := range []struct {
		id, file string
		count    int
	}{{"p1", "big.go", 13}, {"p2", "small.go", 1}} {
		fileID := atlas.FileID(part.file)
		r.places[fileID] = atlas.Place{ID: fileID, Kind: atlas.PlaceFile, Path: part.file, TargetIDs: []string{"t"}, File: &atlas.FileFacts{}}
		box := &boxState{id: part.id, targetID: "t", title: part.file + " part", files: []string{fileID}, symbols: map[string]bool{}}
		for i := 1; i <= part.count; i++ {
			id := fmt.Sprintf("s%s%02d", part.id, i)
			name := fmt.Sprintf("Declaration%02d", i)
			r.places[id] = atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: part.file, LineNo: i, Parent: fileID, TargetIDs: []string{"t"},
				Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object-" + id, Name: name, Kind: "function"}, Rank: i}}
			box.symbols[id] = true
			r.selectedKeys[id] = true
			if part.id == "p1" {
				declarations = append(declarations, name)
			}
		}
		r.boxes[part.id] = box
	}
	if err := r.readKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := r.readCore(t.Context()); err != nil {
		t.Fatal(err)
	}
	input := func(stage string) (request struct {
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
	}) {
		raw, err := readWindowPayload(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, stage+"-r1-w0.input.ref.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		return request
	}
	keys, _ := input(lines.StageKeys).Context["declarations"].([]any)
	var core []any
	for _, row := range input(lines.StageCore).Rows {
		if row["key"] == "p1" {
			core, _ = row["declarations"].([]any)
		}
	}
	if !slices.Equal(keys, declarations) || !slices.Equal(core, declarations) {
		t.Fatalf("a part's declarations were cut: keys %d, core %d of %d", len(keys), len(core), len(declarations))
	}
}

// A refused candidate is no key, and the others are still ranked by their
// probability. An accepted answer without a probability, or a flat ranking,
// still keeps the selection's own order.
func TestKeyRankingSkipsARefusedCandidate(t *testing.T) {
	cell := table.ProbabilityCell("explains")
	var ids []string
	var answers []rowAnswer
	for i, p := range []string{"0.10", "0.95", "0.20", "0.90", "0.85", "0.80", "0.75"} {
		ids = append(ids, fmt.Sprintf("s%d", i+1))
		answers = append(answers, rowAnswer{answer: table.Answer{cell: p}})
	}
	answers[2] = rowAnswer{}
	ranked, ok := rankedByProbability(ids, answers)
	if !ok || !slices.Equal(ranked, []string{"s2", "s4", "s5", "s6", "s7"}) || len(ranked) != lines.MaxKeysPerPart {
		t.Fatalf("one refused candidate declined the ranking or became a key: %v / %t", ranked, ok)
	}
	withoutProbability := slices.Clone(answers)
	withoutProbability[0] = rowAnswer{answer: table.Answer{"explains": "yes"}}
	if _, ok := rankedByProbability(ids, withoutProbability); ok {
		t.Fatal("an answer without a probability was ranked")
	}
	flat := slices.Clone(answers)
	for i := range flat {
		if flat[i].answer != nil {
			flat[i] = rowAnswer{answer: table.Answer{cell: "0.5"}}
		}
	}
	if _, ok := rankedByProbability(ids, flat); ok {
		t.Fatal("a flat ranking decided the keys")
	}
}
