package contracttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
)

func assertGoMethodArgumentExpressions(t *testing.T, index programindex.Index, graph atlas.Graph) {
	t.Helper()
	const path = "internal/storefixture/http_registrations.go"
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "PassMethodArgumentExpressions", path)
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "go", filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	sourceLines := strings.Split(string(source), "\n")
	var sequences [][]string
	var sameLineColumns []int
	negative := 0
	for _, relation := range index.Relations {
		if relation.FromID != caller.ID {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != "receiveMethodArguments" {
				continue
			}
			var sequence []string
			for i, argument := range pattern.Arguments {
				if argument.Origin == nil {
					continue
				}
				origin := argument.Origin
				if argument.Position != i+1 || origin.Kind != "unknown" || origin.Anchor == nil || origin.Anchor.Path != path ||
					origin.Anchor.Line < 1 || origin.Anchor.Column < 1 {
					t.Fatalf("written method value lost its position/source: %+v", argument)
				}
				written := sourceLines[origin.Anchor.Line-1][origin.Anchor.Column-1:]
				if !strings.HasPrefix(written, origin.Text) || len(argument.ObjectIDs) != 0 || argument.Resolution != programindex.ResolutionUnresolved {
					t.Fatalf("source expression changed or promoted an unresolved wrapper: %+v", argument)
				}
				sequence = append(sequence, origin.Text)
			}
			if len(sequence) == 0 {
				negative++ // spread, literal body and literal receiver body
				continue
			}
			sequences = append(sequences, sequence)
			if strings.Contains(sourceLines[pattern.Location.Line-1], "; false;") {
				sameLineColumns = append(sameLineColumns, pattern.Location.Column)
			}
		}
	}
	want := [][]string{{"app.first", "app.second", "app.first", "(app.second)"}, {"app.second", "app.first"}, {"app.first"}, {"app.second"}}
	for _, expected := range want {
		found := false
		for _, actual := range sequences {
			found = found || reflect.DeepEqual(actual, expected)
		}
		if !found {
			t.Fatalf("native method argument sequence %v missing from %v", expected, sequences)
		}
	}
	if len(sequences) != 4 || negative != 3 || len(sameLineColumns) != 2 || sameLineColumns[0] == sameLineColumns[1] {
		t.Fatalf("source calls collapsed or literal/spread gained guessed arguments: sequences=%v negative=%d columns=%v", sequences, negative, sameLineColumns)
	}
	// This is the actual declaration evidence builder used by orientation and
	// questions. Its wire shape must retain positions without canonical IDs.
	evidence := lines.CallableEvidence(graph, map[string]bool{caller.ID: true})[caller.ID]
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Calls []atlas.SymbolCall `json:"calls"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	positive := 0
	for _, call := range wire.Calls {
		if call.Name != "receiveMethodArguments" || len(call.SourceArguments) != 4 {
			continue
		}
		for i, argument := range call.SourceArguments {
			if argument.Position != i+1 || argument.Origin.Text != want[0][i] || argument.Origin.Anchor == nil {
				t.Fatalf("consuming request lost the original argument positions: %s", raw)
			}
		}
		positive++
	}
	if positive != 1 || strings.Contains(string(raw), caller.ID) || strings.Contains(string(raw), "body-not-provider-evidence") {
		t.Fatalf("consuming request lost expressions or gained function bodies/native IDs: %s", raw)
	}
}
