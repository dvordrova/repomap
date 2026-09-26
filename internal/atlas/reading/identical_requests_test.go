package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// withSharedFile adds lib/z.go to both targets of twoTargetGraph, as Redis
// builds zmalloc.c into the server and into each tool. Each target draws it
// as its own part "lib", whose description request is then the same bytes.
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

// drawingProvider answers like a model: every description it writes is a new
// draw, and a description request waits briefly for an identical one so both
// are in the air together and finish in the reverse order.
type drawingProvider struct {
	*tableProvider
	mu      sync.Mutex
	waiting map[string]chan struct{}
}

func (provider *drawingProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	if strings.Contains(string(prepared.Bytes()), designDescribeTask) {
		key := string(prepared.Bytes())
		provider.mu.Lock()
		if twin, found := provider.waiting[key]; found {
			delete(provider.waiting, key)
			provider.mu.Unlock()
			close(twin)
		} else {
			twin := make(chan struct{})
			provider.waiting[key] = twin
			provider.mu.Unlock()
			select {
			case <-twin:
				time.Sleep(5 * time.Millisecond) // the twin lands first
			case <-time.After(150 * time.Millisecond):
			case <-ctx.Done():
				return llm.Completion{}, ctx.Err()
			}
		}
	}
	return provider.tableProvider.Complete(ctx, prepared)
}

// A part two targets share has one description request, byte for byte. It
// is asked once, both targets show that one answer, and the cache keeps the
// answer this reading used: a second reading over the same cache asks
// nothing and sends and draws exactly what the first did, whatever order the
// answers came back in.
func TestIdenticalRequestsOfTwoTargetsGetOneAnswer(t *testing.T) {
	var draws atomic.Int64
	asked := map[string]int{}
	var askedMu sync.Mutex
	provider := &drawingProvider{waiting: map[string]chan struct{}{}, tableProvider: &tableProvider{
		describe: func(name string) string {
			askedMu.Lock()
			asked[name]++
			askedMu.Unlock()
			return fmt.Sprintf("About %s, draw %d.", name, draws.Add(1))
		},
		// Areas and core read the descriptions, so a changed one changes them.
		areaFor: func(part map[string]any) string {
			if part["name"] == "svc/api" || part["name"] == "svc/core" || part["name"] == "lib" {
				return "Serving"
			}
			return ""
		},
	}}
	graph := withSharedFile(t, twoTargetGraph(t))
	cache := t.TempDir()
	read := func() (Options, Result) {
		t.Helper()
		opts := twoTargetOptions(t, graph, provider.tableProvider)
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
	lib := map[string]string{}
	for _, target := range cold.Atlas.Targets {
		for _, box := range target.Boxes {
			if box.Title == "lib" {
				lib[target.ID] = box.Line
			}
		}
	}
	if len(lib) != 2 || lib["svc"] == "" || lib["svc"] != lib["web"] {
		t.Fatalf("the shared part's descriptions per target: %v; want one answer for both", lib)
	}
	if asked["lib"] != 1 {
		t.Fatalf("the identical description request reached the provider %d times", asked["lib"])
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
