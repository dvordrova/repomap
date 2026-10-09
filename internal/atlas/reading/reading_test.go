package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

func graphPlaceID(t *testing.T, graph atlas.Graph, kind atlas.PlaceKind, path string, line int, name string) string {
	t.Helper()
	for _, place := range graph.Places {
		if place.Kind != kind || place.Path != path || line != 0 && place.LineNo != line {
			continue
		}
		if name != "" && (place.Symbol == nil || place.Symbol.Decl.Name != name) {
			continue
		}
		return place.ID
	}
	t.Fatalf("missing %s place at %s:%d %s", kind, path, line, name)
	return ""
}

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
	nextNode := 0
	file := func(path string, depth int, decls []atlas.Decl, callers, callees []string, generated bool) atlas.Place {
		for position := range decls {
			if decls[position].ObjectID == "" {
				nextNode++
				decls[position].ObjectID = fmt.Sprintf("n%d", nextNode)
			}
		}
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
	var symbols []atlas.Place
	for _, file := range graph.Places {
		if file.File == nil {
			continue
		}
		for rank, decl := range file.File.Decls {
			symbols = append(symbols, atlas.Place{
				ID: atlas.SymbolID(file.Path, decl.LineNo, decl.Name), Kind: atlas.PlaceSymbol,
				Path: file.Path, LineNo: decl.LineNo, Parent: file.ID, TargetIDs: append([]string(nil), file.TargetIDs...),
				Given: decl.Name, Symbol: &atlas.SymbolFacts{Decl: decl, Candidate: !file.File.Generated, Rank: rank + 1},
			})
		}
	}
	graph.Places = append(graph.Places, symbols...)
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
// back with a duplicate table key or a missing shared-question decision.
type tableProvider struct {
	mu                 sync.Mutex
	calls              int
	refuse             map[string]bool
	refuseStage        string
	boxFor             map[string]string
	fileLineFor        map[string]string
	openFor            map[string]string
	sameFor            map[string]string
	answers            map[string]int
	questionFor        map[string]table.Answer
	questionBatchFor   func(questionBatchRequest, questionbatch.Response) questionbatch.Response
	questionRequests   [][]byte
	maxQuestionBytes   int
	answerFor          func(map[string]any) table.Answer
	learningFor        func(learningRequest) learningResponse
	learningSelectNone bool
	// groupFor names the smaller box a file goes in when the grouping
	// divides the box named box; nil names it by the file's directory. A
	// proposal names each distinct box once, holding its files' paths, so
	// a box whose files share one name is read as it is. describe writes a
	// description; nil writes "About <name>.", and a description it returns
	// empty is refused.
	groupFor func(box, file string) string
	describe func(name string) string
	// designRequests counts the proposal and description requests;
	// proposed keeps each proposal request and described each description
	// request by part name.
	designRequests map[string]int
	proposed       [][]byte
	described      map[string][]byte
}

type questionBatchRequest struct {
	Task      string           `json:"task"`
	Evidence  []map[string]any `json:"evidence"`
	Questions []struct {
		Key      string `json:"key"`
		Question string `json:"question"`
	} `json:"questions"`
}

func (*tableProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"table"}`)
}

func (provider *tableProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	request["_system"], _ = json.Marshal(prompt.System)
	raw, err := json.Marshal(request)
	if err != nil {
		return llm.Prepared{}, err
	}
	if provider.maxQuestionBytes > 0 && string(request["task"]) == `"`+questionbatch.Contract+`"` && len(raw) > provider.maxQuestionBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Stage: lines.StageQuestion, Kind: llm.ResourceLimitRequestBytes,
			Limit: provider.maxQuestionBytes, Observed: len(raw), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(raw)
}

func (provider *tableProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	provider.calls++
	provider.mu.Unlock()
	var batch questionBatchRequest
	if err := json.Unmarshal(prepared.Bytes(), &batch); err != nil {
		return llm.Completion{}, err
	}
	if raw, ok, err := provider.design(batch.Task, prepared.Bytes()); ok {
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
	}
	if batch.Task == questionbatch.Contract {
		provider.mu.Lock()
		provider.questionRequests = append(provider.questionRequests, append([]byte(nil), prepared.Bytes()...))
		provider.mu.Unlock()
		response := questionbatch.Response{Questions: []questionbatch.Decision{}}
		refused := false
		for _, question := range batch.Questions {
			decision := questionbatch.Decision{Key: question.Key, Selections: []questionbatch.Selection{}}
			for _, row := range batch.Evidence {
				path, _ := row["path"].(string)
				refused = refused || provider.refuse[path]
				answer, found := provider.questionFor[path]
				if !found || answer["relevance"] == "none" {
					continue
				}
				decision.Selections = append(decision.Selections, questionbatch.Selection{
					Row: row["key"].(string), Anchors: strings.Fields(answer["anchors"]),
					Relevance: answer["relevance"], Why: answer["why"],
				})
			}
			response.Questions = append(response.Questions, decision)
		}
		if refused && len(response.Questions) > 0 {
			response.Questions = response.Questions[:len(response.Questions)-1]
		}
		if provider.questionBatchFor != nil {
			response = provider.questionBatchFor(batch, response)
		}
		raw, err := json.Marshal(response)
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1,
			Metrics: llm.Metrics{Attempts: 1, UsageReported: true, InputTokens: 10, OutputTokens: 5}}, err
	}
	if batch.Task == learningMergeContract {
		// Every question is a group of one.
		var merge struct {
			Questions []struct {
				Ref string `json:"ref"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(prepared.Bytes(), &merge); err != nil {
			return llm.Completion{}, err
		}
		var groups []map[string]any
		for _, question := range merge.Questions {
			groups = append(groups, map[string]any{"representative": question.Ref, "members": []string{question.Ref}})
		}
		raw, err := json.Marshal(map[string]any{"groups": groups})
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
	}
	var learning learningRequest
	if json.Unmarshal(prepared.Bytes(), &learning) == nil && learning.Evidence != nil && provider.learningFor != nil {
		raw, err := json.Marshal(provider.learningFor(learning))
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
	}
	var request struct {
		Table string `json:"table"`
		Fill  []struct {
			Name        string   `json:"name"`
			Kind        string   `json:"kind"`
			Options     []string `json:"options"`
			OptionsFrom string   `json:"options_from"`
			Free        string   `json:"free_prefix"`
		} `json:"fill"`
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var rows []map[string]string
	refused := false
	for _, row := range request.Rows {
		key, _ := row["key"].(string)
		path, _ := row["path"].(string)
		if provider.refuse[path] && (provider.refuseStage == "" || provider.refuseStage == request.Table) {
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
			case "sequence":
				answer[column.Name] = "none"
			case "text", "prose":
				answer[column.Name] = "Text for " + key
			case "choice":
				options := column.Options
				if column.OptionsFrom != "" {
					// A row's own list, else the window's shared one.
					list, ok := row[column.OptionsFrom].([]any)
					if !ok {
						list, _ = request.Context[column.OptionsFrom].([]any)
					}
					for _, item := range list {
						options = append(options, fmt.Sprint(item))
					}
				}
				if len(options) > 0 {
					answer[column.Name] = options[0]
				} else if column.Free != "" {
					// Nothing listed: the free value names its own.
					answer[column.Name] = column.Free + "Text for " + key
				}
				// A peer choice takes the first listed ref, not "none".
				if column.Name == "peer" && len(options) > 1 {
					answer[column.Name] = options[1]
				}
			}
		}
		switch request.Table {
		case stageLearn:
			if candidates, ok := row["candidate_options"].([]any); ok {
				var selected []string
				if !provider.learningSelectNone {
					for _, candidate := range candidates {
						selected = append(selected, candidate.(string))
					}
				}
				if limit, ok := row["limit"].(float64); ok && len(selected) > int(limit) {
					selected = selected[:int(limit)]
				}
				answer["questions"] = "none"
				if len(selected) > 0 {
					answer["questions"] = strings.Join(selected, " ")
				}
			}
		case lines.StageAnswer:
			answer["basis"] = "The selected declarations and their signatures suggest this role."
			answer["state"], answer["answer"], answer["sources"], answer["remaining"] = "partial", "The declarations describe the available functions.", row["candidate_options"].([]any)[0].(string), "Their implementation was not inspected."
			if provider.answerFor != nil {
				for key, value := range provider.answerFor(row) {
					answer[key] = value
				}
			}

		case lines.StageDirectories:
			answer["title"] = "Title " + filepath.Base(path)
			answer["line"] = "Directory " + path + " does things."
		case lines.StageFiles:
			answer["line"] = "File " + path + " does things."
			if line, ok := provider.fileLineFor[path]; ok {
				answer["line"] = line
			}
			if chosen, ok := provider.boxFor[path]; ok {
				answer["box"] = chosen
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
		if request.Table == lines.StageDirectories || request.Table == lines.StageFiles {
			if _, asked := answer["open"]; asked {
				if open, specified := provider.openFor[path]; specified {
					answer["open"] = open
					if open == "" {
						delete(answer, "open")
					}
				}
			}
		}
		rows = append(rows, answer)
	}
	if refused && len(rows) > 0 {
		// A second, different answer for the first row leaves it undecided.
		repeat := map[string]string{}
		for name, value := range rows[0] {
			repeat[name] = value
			if name != "key" {
				repeat[name] = value + " (answered again differently)"
			}
		}
		rows = append(rows, repeat)
	}
	response, _ := json.Marshal(map[string]any{"rows": rows})
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, UsageReported: true, InputTokens: 10, OutputTokens: 5},
	}, nil
}

// readOptions reads the graph with the provider; a live reading's closed
// tables go to closedDecisions.
func readOptions(t *testing.T, graph atlas.Graph, provider llm.Provider, cacheRoot string) Options {
	t.Helper()
	opts := Options{
		Graph: graph, Targets: []TargetMeta{{ID: "t1", Language: "go", Kind: "executable", Name: "example.com/x", Root: "pkg/a"}},
		Repository: "x", Revision: "abc",
		Executor: llm.Executor{RootDir: cacheRoot, Enabled: cacheRoot != "", BatchConcurrency: 2, BatchController: &llm.BatchController{}},
		Provider: provider, OwnerRunDir: t.TempDir(),
	}
	if provider != nil {
		opts.Categorizer = closedDecisions()
	}
	return opts
}

// closedDecisions answers the closed tables as the tests decide them: every
// candidate explains its part, every part is the domain, every declaration
// is a key, a handed callable runs around the handlers and a call that hands
// nothing over serves; no declaration is a helper, every box asked about
// needs smaller boxes and a declaration goes in the proposed box that holds
// its file. Any other question fails its request.
func closedDecisions() *typesafetest.Categorizer {
	return closedDecisionsWith(nil)
}

// closedDecisionsWith is closedDecisions with some columns decided otherwise.
func closedDecisionsWith(verdicts map[string]llm.Verdict) *typesafetest.Categorizer {
	decided := map[string]llm.Verdict{
		"explains": typesafetest.Yes(0.9), "role": typesafetest.Choose(lines.PartDomain), "key_symbol": typesafetest.Choose("yes"),
		"helper": typesafetest.Choose(lines.RoleHelperOwnJob), "grouping": typesafetest.Choose(lines.GroupNeedsSmaller),
		"binds": typesafetest.Choose(lines.APIMiddleware), "talks": typesafetest.Choose(lines.APIServes),
		"enters": typesafetest.Choose(lines.APINone),
	}
	maps.Copy(decided, verdicts)
	byColumn := typesafetest.ByColumn(decided)
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if _, decided := verdicts["box"]; !decided && strings.HasSuffix(key, "|box") {
			// A declaration goes in the proposed box that holds its file.
			file, _ := question.Item["file"].(string)
			if box := heldBy(question.Options, file); box != "" {
				return typesafetest.Choose(box), true
			}
			return llm.Verdict{}, false
		}
		return byColumn(key, question)
	}}
}

// jevCalls is how many requests the reading's categorizer answered.
func jevCalls(opts Options) int { return opts.Categorizer.(*typesafetest.Categorizer).Calls() }

func TestCompactIDsUseNumericOrder(t *testing.T) {
	ids := []string{"p10", "p2", "p1", "p11"}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	if !reflect.DeepEqual(ids, []string{"p1", "p2", "p10", "p11"}) {
		t.Fatalf("compact IDs sorted lexically: %v", ids)
	}
}

func readWindowPayload(filename string) ([]byte, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var ref struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(filepath.Dir(filename), ref.File))
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
	for _, want := range []string{"## atlas_directories · round 1", "## atlas_files · round 1", "- given: given pkg/a/x.go"} {
		if !strings.Contains(text, want) {
			t.Errorf("tables.md lacks %q", want)
		}
	}
	// pkg/a and pkg/b are one window whose context names their parent once,
	// by what it holds; no row repeats it, and the root has no parent.
	if strings.Count(text, `context parent: {"line":"4 files: a, b","path":"pkg"}`) != 1 || strings.Contains(text, "  - parent:") || strings.Count(text, "context parent:") != 2 {
		t.Errorf("directory parent is not the window's shared context:\n%s", text)
	}
	if strings.Contains(text, "pkg/b/gen.go\"") {
		t.Error("a generated file was asked about")
	}
	// Without a model there is no map of parts: every file stays readable off
	// the map, with its fallback line.
	target := result.Atlas.Targets[0]
	if len(target.Boxes) != 0 || len(target.OffMap) != 4 || target.Files != 4 || target.MapFailure == "" {
		t.Fatalf("boxes %d off-map %d files %d failure %q", len(target.Boxes), len(target.OffMap), target.Files, target.MapFailure)
	}
	for _, entry := range target.OffMap {
		file := entry.File
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
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.input.ref.json"))
	// The directory, file, symbol and target windows are printed as before;
	// without parts no arrow window is.
	if len(requests) != 3+1+2+1 {
		t.Fatalf("request files: %d", len(requests))
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestLiveReadingKeepsFileLinesWithoutDirectoryPlacement(t *testing.T) {
	graph := testGraph(t)
	provider := &tableProvider{boxFor: map[string]string{"pkg/a/y.go": "pkg/b"}}
	result, err := Read(context.Background(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	wireRequests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.request.ref.json"))
	if len(wireRequests) == 0 {
		t.Fatal("no exact provider request references")
	}
	for _, request := range wireRequests {
		var wire map[string]json.RawMessage
		raw, err := readWindowPayload(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		// The text model's preparation adds the system prompt, Jev's the model.
		if len(wire["_system"]) == 0 && len(wire["model"]) == 0 {
			t.Fatal("request reference points to table input without provider preparation")
		}
	}
	// Four declarations are read as one box, the target's own; the file
	// table's box answer moves no file out of it.
	target := result.Atlas.Targets[0]
	if len(target.Boxes) != 1 || len(target.Boxes[0].Files) != 4 {
		t.Fatalf("parts: %+v", target.Boxes)
	}
	for _, file := range target.Boxes[0].Files {
		if file.Path == "pkg/a/y.go" && (file.Source != atlas.SourceModel || file.Line != "File pkg/a/y.go does things.") {
			t.Fatalf("moved file: %+v", file)
		}
	}
	if target.Line != "Text for "+target.ID {
		t.Fatalf("target line: %q", target.Line)
	}
	tables, _ := os.ReadFile(result.TablesPath)
	if !strings.Contains(string(tables), "line → File pkg/a/x.go does things.") {
		t.Fatal("tables.md does not print the model's cell beside the row")
	}
	// All files can be read together. Caller evidence is available even when
	// that caller has not been described, and contains no generated sentence.
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "atlas_files-*.input.ref.json"))
	if len(requests) != 1 {
		t.Fatalf("file windows: %d", len(requests))
	}
	request, _ := readWindowPayload(requests[0])
	if !strings.Contains(string(request), `"declarations":["Main"],"path":"pkg/a/x.go"`) || strings.Contains(string(request), "File pkg/a/x.go does things.") {
		t.Fatalf("file request does not isolate deterministic caller evidence:\n%s", request)
	}
}

func TestRejectedWindowFallsBackAndIsNotCached(t *testing.T) {
	graph := testGraph(t)
	cacheRoot := t.TempDir()
	provider := &tableProvider{refuse: map[string]bool{"pkg/b/z.go": true}, refuseStage: lines.StageFiles}
	opts := readOptions(t, graph, provider, cacheRoot)
	opts.WindowRows = 2
	result, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	// The refused window is journaled, and so is each of its rows' own reason.
	if len(result.Rejected) != 2 || result.Rejected[0].Kind != "window_rejected" || result.Rejected[0].Stage != lines.StageFiles ||
		result.Rejected[1].Kind != "row_rejected" || !strings.Contains(result.Rejected[1].Reason, "answered more than once, differently") {
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
	opts = readOptions(t, graph, again, cacheRoot)
	opts.WindowRows = 2
	second, err := Read(context.Background(), opts)
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
		if use.Stage == lines.StageFiles && (use.Reused != 2 || use.Live != 1) {
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
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.input.ref.json"))
	if len(requests) == 0 {
		t.Fatal("no request references")
	}
	for _, name := range requests {
		raw, _ := readWindowPayload(name)
		if hexID.Match(raw) || absolutePath.Match(raw) || strings.Contains(string(raw), "dir:") || strings.Contains(string(raw), "file:") {
			t.Errorf("%s carries an identity or an absolute path", filepath.Base(name))
		}
	}
}

// design answers the grouping's proposals and the parts' descriptions; ok
// is false for any other request.
func (provider *tableProvider) design(task string, body []byte) ([]byte, bool, error) {
	if task != groupProposeTask && task != designDescribeTask {
		return nil, false, nil
	}
	provider.mu.Lock()
	if provider.designRequests == nil {
		provider.designRequests, provider.described = map[string]int{}, map[string][]byte{}
	}
	provider.designRequests[task]++
	provider.mu.Unlock()
	if task == designDescribeTask {
		var request describeInput
		if err := json.Unmarshal(body, &request); err != nil {
			return nil, true, err
		}
		provider.mu.Lock()
		provider.described[request.Part] = append([]byte(nil), body...)
		provider.mu.Unlock()
		text := "About " + request.Part + "."
		if provider.describe != nil {
			text = provider.describe(request.Part)
		}
		raw, err := json.Marshal(map[string]string{"description": text})
		return raw, true, err
	}
	var request groupProposeInput
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, true, err
	}
	provider.mu.Lock()
	provider.proposed = append(provider.proposed, append([]byte(nil), body...))
	provider.mu.Unlock()
	var paths []string
	for _, file := range request.Files {
		paths = append(paths, file.Path)
	}
	for _, dir := range request.Counts {
		for _, file := range dir.Files {
			paths = append(paths, path.Join(dir.Dir, file.Name))
		}
	}
	var names []string
	files := map[string][]string{}
	for _, file := range paths {
		name := path.Dir(file)
		if provider.groupFor != nil {
			name = provider.groupFor(request.Box.Name, file)
		}
		if files[name] == nil {
			names = append(names, name)
		}
		files[name] = append(files[name], file)
	}
	boxes := []map[string]string{}
	for _, name := range names {
		boxes = append(boxes, map[string]string{"name": name, "holds": "Holds " + strings.Join(files[name], ", ") + "."})
	}
	raw, err := json.Marshal(map[string]any{"boxes": boxes})
	return raw, true, err
}

// heldBy is the proposed box whose holds lists the file, as the preset
// proposal writes it; "" when none does.
func heldBy(options []llm.Option, file string) string {
	for _, option := range options {
		for _, word := range strings.Fields(option.Meaning) {
			if strings.TrimRight(word, ",.") == file {
				return option.Name
			}
		}
	}
	return ""
}
