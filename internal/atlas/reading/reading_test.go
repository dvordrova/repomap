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
	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/atlas/table"
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
// back with a duplicate table key or a missing shared-question decision.
type tableProvider struct {
	mu                 sync.Mutex
	calls              int
	refuse             map[string]bool
	boxFor             map[string]string
	fileLineFor        map[string]string
	openFor            map[string]string
	partFor            map[string]string
	partNames          []string
	sameFor            map[string]string
	answers            map[string]int
	questionFor        map[string]table.Answer
	questionBatchFor   func(questionBatchRequest, questionbatch.Response) questionbatch.Response
	questionRequests   [][]byte
	maxQuestionBytes   int
	answerFor          func(map[string]any) table.Answer
	learningFor        func(learningRequest) learningResponse
	learningSelectNone bool
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
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "*.input.ref.json"))
	// three directory rounds, one independent file round; the one arrow has
	// no witness call and takes its fallback sentence without a window
	if len(requests) != 3+1 {
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
		if len(wire["_system"]) == 0 {
			t.Fatal("request reference points to table input without provider preparation")
		}
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
	opts := readOptions(t, graph, provider, cacheRoot)
	opts.WindowRows = 2
	result, err := Read(context.Background(), opts)
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
