// Package partstest checks a language fixture's map of parts: it reads the
// fixture's graph with a preset provider that draws every listed file as its
// own part, then checks what the parts request carries and that every
// declaration has exactly one part or an entry off the map. Language fixture
// tests of every adapter share it.
package partstest

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/llm"
)

const partsTask = "repomap.atlas.parts.v1"

// Map is one target's checked map of parts: its atlas target and, by symbol
// place ID, the part each declaration takes ("" off the map).
type Map struct {
	Target atlas.Target
	PartOf map[string]string
	// Symbols finds a declaration's symbol place by its file and name.
	Symbols map[[2]string]string
}

// Check reads the graph for one target and checks the parts request and
// the membership of every declaration. root is the fixture's directory on
// disk, which must not reach the request.
func Check(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string) Map {
	t.Helper()
	// The reading works on the sealed graph and its compact place IDs.
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if graph, err = atlas.DecodeGraph(encoded); err != nil {
		t.Fatal(err)
	}
	provider := &preset{}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Targets: []reading.TargetMeta{target}, Repository: "fixture", Revision: "test",
		Executor: llm.Executor{BatchConcurrency: 4, BatchController: &llm.BatchController{}},
		Provider: provider, OwnerRunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	checked := Map{PartOf: map[string]string{}, Symbols: map[[2]string]string{}}
	for _, candidate := range result.Atlas.Targets {
		if candidate.ID == target.ID {
			checked.Target = candidate
		}
	}
	if checked.Target.MapFailure != "" {
		t.Fatalf("map failure: %s", checked.Target.MapFailure)
	}
	checkRequest(t, graph, target.ID, root, provider.requests)
	checkMembership(t, graph, target.ID, checked)
	return checked
}

// lexicalChildren are the declarations of a file inside the source range of
// another function or method of that file: they are not units.
func lexicalChildren(decls []atlas.Decl) map[int]bool {
	children := map[int]bool{}
	for i, child := range decls {
		for j, parent := range decls {
			if i == j || parent.EndLine <= 0 || parent.Kind != "function" && parent.Kind != "method" {
				continue
			}
			starts := parent.LineNo < child.LineNo || parent.LineNo == child.LineNo && parent.Column < child.Column
			if starts && child.LineNo <= parent.EndLine {
				children[i] = true
			}
		}
	}
	return children
}

var pairPattern = regexp.MustCompile(`^(f[0-9]+) -> (f[0-9]+)( \([0-9]+\))?$`)

func checkRequest(t testing.TB, graph atlas.Graph, targetID, root string, requests [][]byte) {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("parts requests: %d, want one", len(requests))
	}
	raw := requests[0]
	var request map[string]json.RawMessage
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	for key := range request {
		if !slices.Contains([]string{"task", "files", "calls", "imports"}, key) {
			t.Fatalf("the parts request carries %q", key)
		}
	}
	var body struct {
		Files   []map[string]any `json:"files"`
		Calls   []string         `json:"calls"`
		Imports []string         `json:"imports"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if root != "" && strings.Contains(string(raw), root) {
		t.Fatal("the parts request carries the host path of the repository")
	}
	files := map[string]atlas.Place{}
	for _, place := range graph.Places {
		if place.File != nil && slices.Contains(place.TargetIDs, targetID) {
			files[place.ID] = place
		}
		// Author prose never reaches the request: no README or AGENTS
		// document and no declaration documentation.
		if place.Document != nil {
			for _, line := range strings.Split(place.Document.Text, "\n") {
				if line = strings.TrimSpace(line); len(line) >= 24 && strings.Contains(string(raw), line) {
					t.Fatalf("the parts request carries %s text %q", place.Path, line)
				}
			}
		}
	}
	listed := map[string]bool{}
	for _, file := range body.Files {
		for key := range file {
			if !slices.Contains([]string{"ref", "path", "units", "types", "functions", "variables"}, key) {
				t.Fatalf("a file row carries %q", key)
			}
		}
		ref := fmt.Sprint(file["ref"])
		place, ok := files[ref]
		if !ok || listed[ref] || fmt.Sprint(file["path"]) != place.Path || strings.HasPrefix(place.Path, "/") {
			t.Fatalf("file row %v is unknown, repeated or not its own path", file)
		}
		listed[ref] = true
		children := lexicalChildren(place.File.Decls)
		var names []string
		for _, kind := range []string{"types", "functions", "variables"} {
			values, _ := file[kind].([]any)
			for _, value := range values {
				name, _, _ := strings.Cut(fmt.Sprint(value), " ")
				names = append(names, name)
			}
		}
		for i, decl := range place.File.Decls {
			if decl.Doc != "" && len(decl.Doc) >= 24 && strings.Contains(string(raw), decl.Doc) {
				t.Fatalf("the parts request carries the documentation of %s", decl.Name)
			}
			if children[i] && slices.Contains(names, decl.Name) {
				t.Fatalf("%s lists the lexical child %s", place.Path, decl.Name)
			}
		}
		for _, name := range names {
			if strings.Contains(name, "$") {
				t.Fatalf("%s lists the closure %s", place.Path, name)
			}
		}
	}
	// Every file with a declaration of its own (not a lexical child, not a
	// method that goes with its type) is listed once.
	methods := map[string]bool{}
	for _, place := range graph.Places {
		if place.Symbol == nil || !slices.Contains(place.TargetIDs, targetID) || place.Symbol.Decl.Kind != "type" {
			continue
		}
		for _, member := range place.Symbol.Members {
			methods[member.Path+"\x00"+member.Decl.Name] = true
		}
	}
	for id, place := range files {
		children := lexicalChildren(place.File.Decls)
		own := false
		for i, decl := range place.File.Decls {
			if !children[i] && !methods[place.Path+"\x00"+decl.Name] {
				own = true
			}
		}
		if own && !listed[id] {
			t.Fatalf("%s holds code but is not listed", place.Path)
		}
	}
	for _, pair := range append(append([]string(nil), body.Calls...), body.Imports...) {
		match := pairPattern.FindStringSubmatch(pair)
		if match == nil || !listed[match[1]] || !listed[match[2]] || match[1] == match[2] {
			t.Fatalf("aggregate %q names a ref the request does not list", pair)
		}
	}
}

// checkMembership: every declaration of the target's files is in exactly one
// part or off the map, by symbol place and by native object in member_ids.
func checkMembership(t testing.TB, graph atlas.Graph, targetID string, checked Map) {
	t.Helper()
	seen := map[string]int{}
	byObject := map[string]string{}
	for _, box := range checked.Target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				seen[symbol.ID]++
				checked.PartOf[symbol.ID] = box.ID
			}
		}
		for _, id := range box.MemberIDs {
			if previous := byObject[id]; previous != "" && previous != box.ID {
				t.Fatalf("declaration %s is a member of %s and %s", id, previous, box.ID)
			}
			byObject[id] = box.ID
		}
	}
	for _, entry := range checked.Target.OffMap {
		for _, symbol := range entry.File.Symbols {
			seen[symbol.ID]++
			checked.PartOf[symbol.ID] = ""
			if byObject[symbol.ObjectID] != "" {
				t.Fatalf("declaration %s is both a member and off the map", symbol.Name)
			}
		}
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		file := place.Path
		decl := place.Symbol.Decl
		checked.Symbols[[2]string{file, decl.Name}] = place.ID
		if seen[place.ID] != 1 {
			t.Fatalf("%s %s is in %d parts or off-map entries, want one", file, decl.Name, seen[place.ID])
		}
		if decl.ObjectID != "" && checked.PartOf[place.ID] != "" && byObject[decl.ObjectID] != checked.PartOf[place.ID] {
			t.Fatalf("%s %s: member_ids name part %q, its part is %q", file, decl.Name, byObject[decl.ObjectID], checked.PartOf[place.ID])
		}
	}
}

// preset answers every request of the reading: every listed file is its own
// part, every description is a sentence, no areas are drawn, and a table
// cell takes its first option or a short text.
type preset struct {
	mu       sync.Mutex
	requests [][]byte
}

func (*preset) State() []byte { return []byte(`{"provider":"parts-preset"}`) }

func (*preset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (p *preset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task    string           `json:"task"`
		Table   string           `json:"table"`
		Fill    []map[string]any `json:"fill"`
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
		Files   []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var response any
	switch request.Task {
	case partsTask:
		p.mu.Lock()
		p.requests = append(p.requests, prepared.Bytes())
		p.mu.Unlock()
		var groups []map[string]any
		for _, file := range request.Files {
			groups = append(groups, map[string]any{"name": file.Path, "files": []string{file.Ref}})
		}
		response = map[string]any{"groups": groups}
	case "repomap.atlas.describe.v1":
		response = map[string]string{"description": "Preset description."}
	case "repomap.atlas.areas.v1":
		response = map[string]any{"areas": []any{}}
	default:
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			answer := map[string]any{"key": row["key"]}
			for _, column := range request.Fill {
				if when, ok := column["when"].(map[string]any); ok {
					active := true
					for cell, wanted := range when {
						active = active && answer[cell] == wanted
					}
					if !active {
						continue
					}
				}
				name := fmt.Sprint(column["name"])
				switch column["kind"] {
				case "text", "prose":
					answer[name] = "Preset text."
				case "sequence":
					answer[name] = "none"
				case "choice":
					var options []any
					options, _ = column["options"].([]any)
					if from, ok := column["options_from"].(string); ok {
						if list, ok := row[from].([]any); ok {
							options = list
						} else if list, ok := request.Context[from].([]any); ok {
							options = list
						}
					}
					if len(options) > 0 {
						answer[name] = options[0]
					}
				}
			}
			rows = append(rows, answer)
		}
		response = map[string]any{"rows": rows}
	}
	raw, err := json.Marshal(response)
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}
