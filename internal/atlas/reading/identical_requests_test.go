package reading

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// withSharedFile adds lib/z.go to both targets of twoTargetGraph, as Redis
// builds zmalloc.c into the server and into each tool.
func withSharedFile(t *testing.T, graph atlas.Graph) atlas.Graph {
	t.Helper()
	both := []string{"svc", "web"}
	decl := atlas.Decl{ObjectID: "n99", Name: "Alloc", Kind: "function", Signature: "func Alloc(n int) []byte", LineNo: 3, Exported: true}
	for i, place := range graph.Places {
		if place.ID == atlas.DirectoryID(".") {
			graph.Places[i].Directory.Dirs = append(graph.Places[i].Directory.Dirs, "lib")
			graph.Places[i].Directory.FileCount++
		}
	}
	file := atlas.FileID("lib/z.go")
	graph.Places = append(graph.Places,
		atlas.Place{ID: atlas.DirectoryID("lib"), Kind: atlas.PlaceDirectory, Path: "lib", Depth: 1, Parent: atlas.DirectoryID("."), TargetIDs: both, Given: "lib files",
			Directory: &atlas.DirectoryFacts{Files: []string{"z.go"}, FileCount: 1, TopBox: true}},
		atlas.Place{ID: file, Kind: atlas.PlaceFile, Path: "lib/z.go", Parent: atlas.DirectoryID("lib"), TargetIDs: both, Given: "given lib/z.go",
			File: &atlas.FileFacts{Decls: []atlas.Decl{decl}, Callers: []string{atlas.FileID("svc/core/c.go"), atlas.FileID("web/src/app.ts")}}},
		atlas.Place{ID: atlas.SymbolID("lib/z.go", 3, "Alloc"), Kind: atlas.PlaceSymbol, Path: "lib/z.go", LineNo: 3, Parent: file, TargetIDs: both, Given: "Alloc",
			Symbol: &atlas.SymbolFacts{Decl: decl, Rank: 1}},
	)
	for _, from := range []string{"svc/core/c.go", "web/src/app.ts"} {
		graph.Edges = append(graph.Edges, atlas.Edge{From: atlas.FileID(from), To: file, Kind: "calls", Count: 2,
			Witnesses: []atlas.Witness{{Caller: "F", Callee: "Alloc", Path: from, LineNo: 6}}})
	}
	atlas.SortPlaces(graph.Places)
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// Each target keeps its complete owned part. A warm run reuses exact
// requests without replacing the targets with a shared group.
func TestIdenticalRequestsOfTwoTargetsGetOneAnswer(t *testing.T) {
	provider := &tableProvider{}
	graph := withSharedFile(t, twoTargetGraph(t))
	cache := t.TempDir()
	read := func() (Options, Result) {
		t.Helper()
		opts := twoTargetOptions(t, graph, provider)
		opts.Provider = provider
		opts.Executor = llm.Executor{RootDir: cache, Enabled: true, BatchConcurrency: 4, BatchController: &llm.BatchController{}}
		result, err := Read(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		return opts, result
	}
	coldOpts, cold := read()
	calls := provider.calls
	// Both targets hold the shared declaration in a part of their own.
	holders := map[string]bool{}
	for _, target := range cold.Atlas.Targets {
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				if file.Path == "lib/z.go" {
					holders[target.ID] = true
				}
			}
		}
	}
	if !holders["svc"] || !holders["web"] {
		t.Fatalf("the shared file's holders: %v; want both targets", holders)
	}

	warmOpts, warm := read()
	if provider.calls != calls {
		t.Fatalf("the second reading asked the provider %d more times; want none", provider.calls-calls)
	}
	if coldAtlas, warmAtlas := drawn(t, cold.Atlas), drawn(t, warm.Atlas); coldAtlas != warmAtlas {
		t.Fatalf("the second reading drew another atlas:\n%s\n%s", coldAtlas, warmAtlas)
	}
	// Rows the second reading recalls from their memos make no window; every
	// window it did make repeats the first reading's request.
	coldRequests, warmRequests := windowRequests(t, coldOpts.OwnerRunDir), windowRequests(t, warmOpts.OwnerRunDir)
	if len(warmRequests) == 0 {
		t.Fatal("the second reading made no window")
	}
	for window, request := range warmRequests {
		if coldRequests[window] != request {
			t.Errorf("%s: the second reading sent %s, the first %s", window, request, coldRequests[window])
		}
	}
}

// drawn is the atlas without where its answers came from this time: the
// budget's live and cached counts and each line's model or cache source.
func drawn(t *testing.T, value atlas.Atlas) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	delete(tree, "budget")
	var strip func(any)
	strip = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			if source := node["source"]; source == atlas.SourceModel || source == atlas.SourceCache {
				delete(node, "source")
			}
			for _, child := range node {
				strip(child)
			}
		case []any:
			for _, child := range node {
				strip(child)
			}
		}
	}
	strip(tree)
	raw, err = json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// windowRequests maps every window of a reading to the content hash of the
// exact request it sent.
func windowRequests(t *testing.T, run string) map[string]string {
	t.Helper()
	refs, err := filepath.Glob(filepath.Join(run, atlas.TablesDir, "*.request.ref.json"))
	if err != nil {
		t.Fatal(err)
	}
	windows := map[string]string{}
	for _, name := range refs {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var ref struct {
			File string `json:"file"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil {
			t.Fatal(err)
		}
		windows[filepath.Base(name)] = filepath.Base(ref.File)
	}
	return windows
}
