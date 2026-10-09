package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/dvordrova/repomap/internal/atlas"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOwnedReadingHandoffMatchesStrictSavedReading(t *testing.T) {
	original := testGraph(t)
	generic := readOptions(t, original, nil, "")
	want, err := Read(context.Background(), generic)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := atlas.SealGraph(original)
	if err != nil {
		t.Fatal(err)
	}
	opts := readOptions(t, atlas.Graph{}, nil, "")
	if err := sealed.WriteGraph(opts.OwnerRunDir); err != nil {
		t.Fatal(err)
	}
	// Generic run must use the same artifact reference rather than inline graph.
	encoded, err := atlas.EncodeGraph(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.WriteGraph(generic.OwnerRunDir, encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveInput(generic); err != nil {
		t.Fatal(err)
	}
	original.Places[0].Given = "mutated producer after seal"
	got, graph, _, err := ReadSealed(context.Background(), opts, sealed)
	if err != nil {
		t.Fatal(err)
	}
	want.TablesPath, got.TablesPath = "", ""
	if !reflect.DeepEqual(got, want) {
		t.Fatal("complete ordinary reading outcome changed")
	}
	for _, name := range []string{InputFilename, atlas.TablesFilename} {
		wb, err := os.ReadFile(filepath.Join(generic.OwnerRunDir, name))
		if err != nil {
			t.Fatal(err)
		}
		gb, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wb, gb) {
			t.Fatalf("complete %s bytes changed", name)
		}
	}
	loaded, err := LoadInput(filepath.Join(opts.OwnerRunDir, InputFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(graph, loaded.Graph) {
		t.Fatal("returned graph/source refs differ from strict saved input")
	}
	if _, _, _, err := ReadSealed(context.Background(), opts, sealed); err == nil {
		t.Fatal("read consumed same authority twice")
	}
	// External/mutable generic supplied bytes still pass strict DecodeGraph.
	bad := generic
	bad.SealedGraph = append([]byte(nil), encoded...)
	var obj map[string]any
	json.Unmarshal(bad.SealedGraph, &obj)
	obj["sha256"] = "bad"
	bad.SealedGraph, _ = json.Marshal(obj)
	if _, err := SaveInput(bad); err == nil {
		t.Fatal("tampered caller seal admitted")
	}
	sealed, err = atlas.SealGraph(loaded.Graph)
	if err != nil {
		t.Fatal(err)
	}
	opts.Graph = loaded.Graph
	if _, _, _, err := ReadSealed(context.Background(), opts, sealed); err == nil {
		t.Fatal("mutable caller graph mixed with opaque producer")
	}
}
