package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

func testGraph(t *testing.T) atlas.Graph {
	t.Helper()
	dir := func(path string, depth int, parent string, dirs, files []string, count int, top bool) atlas.Place {
		place := atlas.Place{
			ID: atlas.DirectoryID(path), Kind: atlas.PlaceDirectory, Path: path, Depth: depth, Parent: parent,
			TargetIDs: []string{"t1"},
			Directory: &atlas.DirectoryFacts{Dirs: dirs, Files: files, FileCount: count, TopBox: top},
		}
		place.Given = fmt.Sprintf("%d files", count)
		return place
	}
	file := func(path string, depth int, decls []atlas.Decl, callers, callees []string, generated bool) atlas.Place {
		return atlas.Place{
			ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, Depth: depth,
			Parent: atlas.DirectoryID(filepath.Dir(path)), TargetIDs: []string{"t1"},
			Given: "given " + path,
			File:  &atlas.FileFacts{Decls: decls, Callers: callers, Callees: callees, Generated: generated},
		}
	}
	graph := atlas.Graph{
		Version: atlas.GraphVersion, Revision: "abc",
		Places: []atlas.Place{
			dir(".", 0, "", []string{"pkg"}, nil, 4, false),
			dir("pkg", 1, atlas.DirectoryID("."), []string{"a", "b"}, nil, 4, false),
			dir("pkg/a", 2, atlas.DirectoryID("pkg"), nil, []string{"x.go", "y.go"}, 2, true),
			dir("pkg/b", 2, atlas.DirectoryID("pkg"), nil, []string{"gen.go", "z.go"}, 2, true),
			file("pkg/a/x.go", 0, []atlas.Decl{{Name: "Main", Kind: "function", Signature: "func()", Doc: "Main runs.", LineNo: 3, Exported: true}}, nil, []string{atlas.FileID("pkg/a/y.go"), atlas.FileID("pkg/b/z.go")}, false),
			file("pkg/a/y.go", 1, []atlas.Decl{{Name: "help", Kind: "function", LineNo: 3}}, []string{atlas.FileID("pkg/a/x.go")}, nil, false),
			file("pkg/b/gen.go", 2, []atlas.Decl{{Name: "Gen", Kind: "type", LineNo: 9}}, nil, nil, true),
			file("pkg/b/z.go", 1, []atlas.Decl{{Name: "Z", Kind: "type", LineNo: 5, Exported: true, Doc: "Z is a thing."}}, []string{atlas.FileID("pkg/a/x.go")}, nil, false),
		},
		Edges: []atlas.Edge{
			{From: atlas.FileID("pkg/a/x.go"), To: atlas.FileID("pkg/a/y.go"), Kind: "calls", Count: 1, Witnesses: []atlas.Witness{{Caller: "Main", Callee: "help", Path: "pkg/a/x.go", LineNo: 4}}},
			{From: atlas.FileID("pkg/a/x.go"), To: atlas.FileID("pkg/b/z.go"), Kind: "calls", Count: 2, Witnesses: []atlas.Witness{}},
		},
		Seeds: []string{atlas.FileID("pkg/a/x.go")},
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

// tableProvider answers every window from its request: rows get a line made
// of their path, directories get a title, files keep their box unless the
// test says otherwise. A window whose rows include a path in `refuse` comes
// back with a duplicate key, which the table refuses.
type tableProvider struct {
	mu        sync.Mutex
	calls     int
	refuse    map[string]bool
	boxFor    map[string]string
	partFor   map[string]string
	partNames []string
	sameFor   map[string]string
	answers   map[string]int
}

func (*tableProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"table"}`)
}

func (*tableProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (provider *tableProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	provider.calls++
	provider.mu.Unlock()
	var request struct {
		Table string `json:"table"`
		Fill  []struct {
			Name        string   `json:"name"`
			Kind        string   `json:"kind"`
			Options     []string `json:"options"`
			OptionsFrom string   `json:"options_from"`
		} `json:"fill"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var rows []map[string]string
	refused := false
	for _, row := range request.Rows {
		key, _ := row["key"].(string)
		path, _ := row["path"].(string)
		if provider.refuse[path] {
			refused = true
		}
		provider.mu.Lock()
		if provider.answers == nil {
			provider.answers = make(map[string]int)
		}
		provider.answers[path]++
		provider.mu.Unlock()
		// Every column gets a plausible cell: text from the key, a choice
		// from the first option; the directory and file tables get the
		// cells the tests look for.
		answer := map[string]string{"key": key}
		for _, column := range request.Fill {
			switch column.Kind {
			case "text":
				answer[column.Name] = "Text for " + key
			case "choice":
				options := column.Options
				if column.OptionsFrom != "" {
					if list, ok := row[column.OptionsFrom].([]any); ok {
						for _, item := range list {
							options = append(options, fmt.Sprint(item))
						}
					}
				}
				if len(options) > 0 {
					answer[column.Name] = options[0]
				}
				// A peer choice takes the first listed ref, not "none".
				if column.Name == "peer" && len(options) > 1 {
					answer[column.Name] = options[1]
				}
			}
		}
		switch request.Table {
		case lines.StageDirectories:
			answer["title"] = "Title " + filepath.Base(path)
			answer["line"] = "Directory " + path + " does things."
		case lines.StageFiles:
			answer["line"] = "File " + path + " does things."
			if chosen, ok := provider.boxFor[path]; ok {
				answer["box"] = chosen
			}
		case lines.StageZones:
			if part, ok := row["title"].(string); ok && provider.partFor != nil {
				if chosen, ok := provider.partFor[part]; ok {
					answer["part"] = chosen
				}
			}
			for i, column := range request.Fill {
				if strings.HasPrefix(column.Name, "part_") {
					if i < len(provider.partNames) {
						answer[column.Name] = provider.partNames[i]
					} else {
						answer[column.Name] = fmt.Sprintf("Part %d", i+1)
					}
				}
			}
		case lines.StageJoints:
			if provider.sameFor != nil {
				if value, ok := row["value"].(string); ok {
					if same, ok := provider.sameFor[value]; ok {
						answer["same"] = same
						answer["label"] = "reads " + value
					}
				}
			}
		}
		rows = append(rows, answer)
	}
	if refused && len(rows) > 0 {
		rows = append(rows, rows[0])
	}
	response, _ := json.Marshal(map[string]any{"rows": rows})
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, UsageReported: true, InputTokens: 10, OutputTokens: 5},
	}, nil
}

func readOptions(t *testing.T, graph atlas.Graph, provider llm.Provider, cacheRoot string) Options {
	t.Helper()
	return Options{
		Graph: graph, Targets: []TargetMeta{{ID: "t1", Language: "go", Kind: "executable", Name: "example.com/x", Root: "pkg/a"}},
		Repository: "x", Revision: "abc",
		Executor: llm.Executor{RootDir: cacheRoot, Enabled: cacheRoot != "", BatchConcurrency: 2, BatchController: &llm.BatchController{}},
		Provider: provider, OwnerRunDir: t.TempDir(),
	}
}

func TestDryReadingPrintsTablesAndFallsBack(t *testing.T) {
	graph := testGraph(t)
	result, err := Read(context.Background(), readOptions(t, graph, nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := os.ReadFile(result.TablesPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(tables)
	for _, want := range []string{"## atlas_directories · round 1", "## atlas_files · round 1", "- given: given pkg/a/x.go", `box_options: ["here","pkg/b"]`} {
		if !strings.Contains(text, want) {
			t.Errorf("tables.md lacks %q", want)
		}
	}
	if strings.Contains(text, "pkg/b/gen.go\"") {
		t.Error("a generated file was asked about")
	}
	target := result.Atlas.Targets[0]
	if len(target.Boxes) != 2 || target.Files != 4 {
		t.Fatalf("boxes %d files %d", len(target.Boxes), target.Files)
	}
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			if file.Path == "pkg/b/gen.go" {
				if file.Source != atlas.SourceUnused || file.Asked {
					t.Errorf("generated file: source %q asked %v", file.Source, file.Asked)
				}
				continue
			}
			if file.Source != atlas.SourceGiven || file.Line != "given "+file.Path {
				t.Errorf("%s: source %q line %q", file.Path, file.Source, file.Line)
			}
		}
	}
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.request.json"))
	// three directory rounds, two file rounds, one arrow window
	if len(requests) != 3+2+1 {
		t.Fatalf("request files: %d", len(requests))
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestLiveReadingKeepsLinesAndMovesFiles(t *testing.T) {
	graph := testGraph(t)
	provider := &tableProvider{boxFor: map[string]string{"pkg/a/y.go": "pkg/b"}}
	result, err := Read(context.Background(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	target := result.Atlas.Targets[0]
	boxes := make(map[string]atlas.Box)
	for _, box := range target.Boxes {
		boxes[box.ID] = box
	}
	if boxes["pkg/a"].Title != "Title a" || boxes["pkg/a"].Line != "Directory pkg/a does things." {
		t.Fatalf("box pkg/a: %+v", boxes["pkg/a"])
	}
	if len(boxes["pkg/a"].Files) != 1 || len(boxes["pkg/b"].Files) != 3 {
		t.Fatalf("y.go did not move: a=%d b=%d", len(boxes["pkg/a"].Files), len(boxes["pkg/b"].Files))
	}
	for _, file := range boxes["pkg/b"].Files {
		if file.Path == "pkg/a/y.go" && (file.Source != atlas.SourceModel || file.Line != "File pkg/a/y.go does things.") {
			t.Fatalf("moved file: %+v", file)
		}
	}
	if target.Line != "Directory pkg/a does things." {
		t.Fatalf("target line: %q", target.Line)
	}
	tables, _ := os.ReadFile(result.TablesPath)
	if !strings.Contains(string(tables), "line → File pkg/a/x.go does things.") {
		t.Fatal("tables.md does not print the model's cell beside the row")
	}
	// The caller's line reaches the callee's row in the next round.
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "atlas_files-r2-*.request.json"))
	if len(requests) != 1 {
		t.Fatalf("second file round windows: %d", len(requests))
	}
	second, _ := os.ReadFile(requests[0])
	if !strings.Contains(string(second), `"callers": ["x.go: File pkg/a/x.go does things."]`) {
		t.Fatalf("second round lacks the caller's line:\n%s", second)
	}
}

func TestLonelyNewBoxIsCancelled(t *testing.T) {
	graph := testGraph(t)
	provider := &tableProvider{boxFor: map[string]string{"pkg/a/y.go": "new: Helpers"}}
	result, err := Read(context.Background(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, box := range result.Atlas.Targets[0].Boxes {
		if strings.Contains(box.ID, "#") {
			t.Fatalf("a box of one file survived: %s", box.ID)
		}
	}
}

func TestRejectedWindowFallsBackAndIsNotCached(t *testing.T) {
	graph := testGraph(t)
	cacheRoot := t.TempDir()
	provider := &tableProvider{refuse: map[string]bool{"pkg/b/z.go": true}}
	result, err := Read(context.Background(), readOptions(t, graph, provider, cacheRoot))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].Kind != "window_rejected" || result.Rejected[0].Stage != lines.StageFiles {
		t.Fatalf("rejected rows: %+v", result.Rejected)
	}
	for _, box := range result.Atlas.Targets[0].Boxes {
		for _, file := range box.Files {
			if file.Path == "pkg/b/z.go" && file.Source != atlas.SourceGiven {
				t.Fatalf("refused row kept a model line: %+v", file)
			}
			if file.Path == "pkg/a/x.go" && file.Source != atlas.SourceModel {
				t.Fatalf("a good window was dragged down: %+v", file)
			}
		}
	}
	var files atlas.StageUse
	for _, use := range result.Uses {
		if use.Stage == lines.StageFiles {
			files = use
		}
	}
	if files.Rejected != 1 || files.Given == 0 {
		t.Fatalf("file stage use: %+v", files)
	}
	// Ask again with a provider that answers well: the refused window is
	// asked live, the accepted ones come from the cache.
	again := &tableProvider{}
	second, err := Read(context.Background(), readOptions(t, graph, again, cacheRoot))
	if err != nil {
		t.Fatal(err)
	}
	if again.answers["pkg/b/z.go"] != 1 {
		t.Fatalf("the refused window was not asked again: %v", again.answers)
	}
	if again.answers["pkg/a/x.go"] != 0 {
		t.Fatalf("an accepted window was asked again: %v", again.answers)
	}
	for _, use := range second.Uses {
		if use.Stage == lines.StageFiles && (use.Cached == 0 || use.Live != 1) {
			t.Fatalf("second run file use: %+v", use)
		}
	}
}

var (
	hexID        = regexp.MustCompile(`[0-9a-f]{64}`)
	absolutePath = regexp.MustCompile(`"/(Users|home|private|tmp)/`)
)

func TestRequestBytesCarryNoIdentities(t *testing.T) {
	graph := testGraph(t)
	result, err := Read(context.Background(), readOptions(t, graph, nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.request.json"))
	for _, name := range requests {
		raw, _ := os.ReadFile(name)
		if hexID.Match(raw) || absolutePath.Match(raw) || strings.Contains(string(raw), "dir:") || strings.Contains(string(raw), "file:") {
			t.Errorf("%s carries an identity or an absolute path", filepath.Base(name))
		}
	}
}
