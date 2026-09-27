package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// kvd is read end to end, without captions as an ordinary run reads it, with
// a preset in place of the model that answers each registrar and each entry
// from its row alone. The command table's rows reach the registrar table as
// one field of their record type, and the entries they make are named by the
// word each row wrote. Nothing in the pipeline names kvd, Redis or a command
// table; the same two decisions name an HTTP route in the Echo preset.
func TestCFixturePresetReadingTurnsTableRowsIntoNamedRequests(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvd")
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	preset := &kvdPreset{}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true,
		Targets:  []reading.TargetMeta{{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: kvdCategorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("indexes = %d", len(indexes))
	}
	// Every registrar the program hands a callable to was asked once, the
	// command table's field among them; fopen and the socket calls, which
	// are handed none, were not asked what a callable becomes.
	preset.mu.Lock()
	handed := slices.Clone(preset.handed)
	named := slices.Clone(preset.named)
	unnamed, entryWindows := preset.unnamed, preset.entryWindows
	preset.mu.Unlock()
	if unnamed != 0 {
		t.Fatalf("%d incoming rows with nothing to decide were sent without captions", unnamed)
	}
	// Seven entries of seven handlers share one request: a handler's calls
	// say nothing about the words that name its entry.
	if entryWindows != 1 {
		t.Fatalf("entries were named in %d requests, want one", entryWindows)
	}
	sort.Strings(handed)
	if want := []string{"kvd.h.kvCommand.proc", "pthread.h.pthread_create", "signal.h.struct sigaction", "stdlib.h.qsort"}; !slices.Equal(handed, want) {
		t.Fatalf("registrars asked what a callable becomes = %v, want %v", handed, want)
	}
	handlers := map[string]string{}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectFunction && object.Location != nil && object.Location.Path == "kvd.c" {
			handlers[object.ID] = object.Name
		}
	}
	type input struct{ kind, name, handler, source string }
	var got []input
	for _, operation := range indexes[0].Operations {
		got = append(got, input{kind: operation.Kind, name: operation.Name, handler: handlers[operation.SubjectID], source: operation.Source})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].kind+got[i].name < got[j].kind+got[j].name })
	want := []input{{kind: "continuous", name: "statsWorker", handler: "statsWorker", source: "fact"}}
	for _, row := range cCommandRows {
		want = append(want, input{kind: "request", name: row.name, handler: row.function, source: "fact"})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].kind+want[i].name < want[j].kind+want[j].name })
	if !slices.Equal(got, want) {
		t.Fatalf("inputs = %+v\nwant %+v", got, want)
	}
	// Each command's entry was offered the words its row wrote and chose the
	// command's own. The thread's registration wrote only its call word: no
	// word named it, and it keeps its handler's name.
	// The event loop's calls through fe->rfileProc and fe->wfileProc stay
	// unresolved, and the handlers their stores name reach the map as the
	// possible arrows of the loop's part, as several alternatives would.
	processEvents := cObject(t, index, programindex.ObjectFunction, "loopProcessEvents", "loop.c")
	possible := map[string]bool{}
	for _, connection := range indexes[0].Connections {
		if connection.FromSubjectID != processEvents.ID || connection.SupportResolution != programindex.PatternValuePossible {
			continue
		}
		for _, relation := range index.Relations {
			if relation.ID == connection.SourceID && (relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0) {
				t.Fatalf("a witnessed call became resolved: %+v", relation)
			}
		}
		possible[handlers[connection.ToSubjectID]] = true
	}
	for _, handler := range []string{"acceptHandler", "readQueryFromClient", "sendReplyToClient"} {
		if !possible[handler] {
			t.Fatalf("the event loop draws no possible arrow to %s: %v", handler, possible)
		}
	}
	sort.Strings(named)
	wantNamed := []string{"pthread_create"}
	for _, row := range cCommandRows {
		wantNamed = append(wantNamed, "kvCommand "+row.name)
	}
	sort.Strings(wantNamed)
	if !slices.Equal(named, wantNamed) {
		t.Fatalf("entries asked for a name = %v, want %v", named, wantNamed)
	}
}

// kvdPreset answers the text-model tables of a caption-less reading of kvd
// the way a reader would from each row, and fails any other request, so a
// table the reading starts to ask is noticed rather than answered by chance.
type kvdPreset struct {
	// roles are what a symbol does, by its api row and column; a symbol
	// not here middlewares, publishes and talks to nothing.
	roles         map[string]map[string]string
	mu            sync.Mutex
	handed, named []string
	// unnamed counts rows of the incoming boundaries table that had no words
	// to name them by: without captions such a row has nothing to decide.
	unnamed int
	// entryWindows counts the requests that named entries.
	entryWindows int
}

func (*kvdPreset) State() []byte { return []byte(`{"provider":"kvd-preset"}`) }

func (*kvdPreset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (preset *kvdPreset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task  string           `json:"task"`
		Table string           `json:"table"`
		Fill  []map[string]any `json:"fill"`
		Rows  []map[string]any `json:"rows"`
		Files []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var answer any
	switch {
	case request.Task == "repomap.atlas.parts.v1":
		// One part per source file is as good a map as any for this test.
		var groups []map[string]any
		for _, file := range request.Files {
			groups = append(groups, map[string]any{"name": file.Path, "files": []string{file.Ref}})
		}
		answer = map[string]any{"groups": groups}
	case request.Task == "repomap.atlas.describe.v1":
		answer = map[string]any{"description": "Preset description."}
	case request.Task == "repomap.atlas.areas.v1":
		answer = map[string]any{"areas": []any{}}
	case request.Table == "atlas_api" || request.Table == "atlas_boundaries" || request.Table == "atlas_layers":
		outgoing := false
		for _, column := range request.Fill {
			outgoing = outgoing || column["name"] == "destination"
		}
		if request.Table == "atlas_boundaries" && !outgoing {
			preset.mu.Lock()
			preset.entryWindows++
			preset.mu.Unlock()
		}
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			if request.Table == "atlas_boundaries" && !outgoing && row["word_options"] == nil {
				preset.mu.Lock()
				preset.unnamed++
				preset.mu.Unlock()
			}
			cells, err := preset.answer(request.Table, request.Fill, row)
			if err != nil {
				return llm.Completion{}, err
			}
			rows = append(rows, cells)
		}
		answer = map[string]any{"rows": rows}
	default:
		return llm.Completion{}, fmt.Errorf("kvd preset: no answer for task %q table %q", request.Task, request.Table)
	}
	response, err := json.Marshal(answer)
	if err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (preset *kvdPreset) answer(table string, fill []map[string]any, row map[string]any) (map[string]any, error) {
	answer := map[string]any{"key": row["key"]}
	symbol, _ := row["symbol"].(string)
	usage, _ := row["usage"].(string)
	for _, column := range fill {
		name, _ := column["name"].(string)
		if !conditionHolds(column, answer, row) {
			continue
		}
		switch {
		case table == "atlas_api" && name == "binds":
			preset.mu.Lock()
			preset.handed = append(preset.handed, symbol)
			preset.mu.Unlock()
			switch {
			case strings.Contains(usage, `{"get", getCommand`), strings.Contains(usage, `{"ping", pingCommand`):
				// A row of the table a client's first word is looked up in.
				answer["binds"] = "request"
			case strings.Contains(usage, "pthread_create"):
				// The thread's body loops for as long as the server runs.
				answer["binds"] = "continuous"
			}
		case table == "atlas_api":
			if value := preset.roles[symbol][name]; value != "" {
				answer[name] = value
			}
		case table == "atlas_boundaries" && name == "name":
			words, _ := row["words"].([]any)
			var values []string
			for _, item := range words {
				word, _ := item.(map[string]any)
				value, _ := word["value"].(string)
				values = append(values, value)
			}
			preset.mu.Lock()
			preset.named = append(preset.named, strings.Join(values, " "))
			preset.mu.Unlock()
			// The command a client sends; the record type names nothing.
			var refs []string
			for _, command := range cCommandRows {
				refs = append(refs, wordRefs(row, command.name)...)
			}
			answer["name"] = refs
		case table == "atlas_layers" && name == "role":
			answer["role"] = "logic"
		default:
			return nil, fmt.Errorf("kvd preset: no answer for %s.%s", table, name)
		}
	}
	return answer, nil
}

// kvdCategorizer answers the closed tables: every declaration and candidate
// is a key, and the part of the server's commands is what kvd exists for.
func kvdCategorizer() *typesafetest.Categorizer {
	decide := typesafetest.ByColumn(map[string]llm.Verdict{"explains": typesafetest.Yes(0.9), "key_symbol": typesafetest.Choose("yes")})
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if strings.HasSuffix(key, "|role") {
			return typesafetest.Choose("domain"), true
		}
		return decide(key, question)
	}}
}
