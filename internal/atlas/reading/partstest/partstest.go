// Package partstest checks a language fixture's map of parts on its real
// graph. It reads the graph with a preset text model that divides every box
// it is asked about by the next path segment of its files (so the grouping
// ends at one box per file) and a preset Jev that asks for smaller boxes
// whenever it is asked and puts each declaration in the box named by its
// file. Check answers no declaration a helper; CheckHelpers takes one for a
// helper when other code calls, reads or hands it over, the language does
// not export it and no registration names it.
//
// The checks catch what a reader of the map would see go wrong: a
// declaration in no part or in two, a method away from its type, a lexical
// child away from its parent, test code on the canvas, the repository's
// host path or author prose in a provider request, a helper away from the
// part of its users, a registration missing from the assignment, a mutated
// native graph, an atlas that does not validate.
package partstest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

const (
	proposeTask  = "repomap.atlas.group_propose.v1"
	describeTask = "repomap.atlas.describe.v1"
)

// Map is one target's checked map of parts.
type Map struct {
	// Atlas is the whole reading; Target is the checked target in it.
	Atlas  atlas.Atlas
	Target atlas.Target
	// PartOf is, by symbol place ID, the part each declaration takes ("" off
	// the map); Symbols finds a declaration's place by its file and name.
	PartOf  map[string]string
	Symbols map[[2]string]string
	// Registered are, by file path, the words of the registrations the
	// assignment showed with that file's declarations ("HandleFunc /x").
	Registered map[string][]string
	// HelperItems are the helper question's items by file and name, and
	// Helpers the declarations the preset decided are helpers.
	HelperItems map[[2]string]map[string]any
	Helpers     map[[2]string]bool
	// Rejected is the reading's journal of what it refused or recorded.
	Rejected []modeldiag.Row
}

// Check reads the graph for one target with no helper and checks the map.
// root is the fixture's directory on disk, which must reach no request.
func Check(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string) Map {
	t.Helper()
	return check(t, graph, target, root, false)
}

// CheckHelpers is Check with the helper rule, and checks besides that every
// decided helper whose users all sit in one part is in that part.
func CheckHelpers(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string) Map {
	t.Helper()
	return check(t, graph, target, root, true)
}

func check(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string, helpers bool) Map {
	t.Helper()
	// The reading works on the sealed graph and its compact place IDs.
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if graph, err = atlas.DecodeGraph(encoded); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	provider := &preset{}
	jev := newJev(t, graph, helpers)
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Targets: []reading.TargetMeta{target}, Repository: "fixture", Revision: "test",
		Executor: llm.Executor{BatchConcurrency: 4, BatchController: &llm.BatchController{}},
		Provider: provider, Categorizer: jev, OwnerRunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the reading changed the native graph")
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	checked := Map{Atlas: result.Atlas, PartOf: map[string]string{}, Symbols: map[[2]string]string{}, Rejected: result.Rejected,
		Registered: jev.registered, HelperItems: jev.helperItems, Helpers: jev.helpers}
	for _, candidate := range result.Atlas.Targets {
		if candidate.ID == target.ID {
			checked.Target = candidate
		}
	}
	if checked.Target.MapFailure != "" {
		t.Fatalf("map failure: %s", checked.Target.MapFailure)
	}
	if provider.proposals == 0 || jev.grouping == 0 || jev.assigned == 0 {
		t.Fatalf("the grouping did not run: %d proposals, %d grouping questions, %d assignments", provider.proposals, jev.grouping, jev.assigned)
	}
	checkRequests(t, graph, root, append(provider.bodies(), jev.Requests()...))
	checkMembership(t, graph, target.ID, checked)
	checkFollowers(t, graph, target.ID, checked)
	checkTests(t, graph, checked)
	checkRegistrations(t, graph, target.ID, jev)
	for key, count := range jev.helperAsked {
		if count != 1 {
			t.Fatalf("%s %s was asked the helper question %d times", key[0], key[1], count)
		}
	}
	if helpers {
		checkHelpers(t, graph, target.ID, checked)
	}
	return checked
}

// jev is the preset decision model of a check. It keeps what the helper
// question and the assignment were asked.
type jev struct {
	typesafetest.Categorizer
	mu                 sync.Mutex
	grouping, assigned int
	registered         map[string][]string
	helperItems        map[[2]string]map[string]any
	helperAsked        map[[2]string]int
	helpers            map[[2]string]bool
}

func newJev(t testing.TB, graph atlas.Graph, helpers bool) *jev {
	exported := map[[2]string]bool{}
	for _, place := range graph.Places {
		if place.Symbol != nil {
			exported[[2]string{place.Path, place.Symbol.Decl.Name}] = place.Symbol.Decl.Exported
		}
	}
	decisions := &jev{registered: map[string][]string{}, helperItems: map[[2]string]map[string]any{}, helperAsked: map[[2]string]int{}, helpers: map[[2]string]bool{}}
	other := typesafetest.ByColumn(map[string]llm.Verdict{
		"explains": typesafetest.Yes(0.9), "role": typesafetest.Choose("domain"), "key_symbol": typesafetest.Choose("yes"),
	})
	decisions.Decide = func(key string, question llm.Question) (llm.Verdict, bool) {
		decisions.mu.Lock()
		defer decisions.mu.Unlock()
		file, _ := question.Item["file"].(string)
		name, _ := question.Item["name"].(string)
		switch key[strings.LastIndex(key, "|")+1:] {
		case "helper":
			decisions.helperItems[[2]string{file, name}] = question.Item
			decisions.helperAsked[[2]string{file, name}]++
			// A helper has users, is not exported and is not registered.
			used := question.Item["called_by"] != nil || question.Item["read_by"] != nil || question.Item["handed_over_by"] != nil
			if helpers && used && question.Item["registered"] == nil && !exported[[2]string{file, name}] {
				decisions.helpers[[2]string{file, name}] = true
				return typesafetest.Choose(lines.RoleHelperHelper), true
			}
			return typesafetest.Choose(lines.RoleHelperOwnJob), true
		case "grouping":
			decisions.grouping++
			return typesafetest.Choose(lines.GroupNeedsSmaller), true
		case "box":
			decisions.assigned++
			for _, word := range stringList(question.Item["registered"]) {
				if !slices.Contains(decisions.registered[file], word) {
					decisions.registered[file] = append(decisions.registered[file], word)
				}
			}
			// The preset names each smaller box by a path; the declaration
			// goes in the one its file lies under.
			best := ""
			for _, option := range question.Options {
				if (file == option.Name || strings.HasPrefix(file, option.Name+"/")) && len(option.Name) > len(best) {
					best = option.Name
				}
			}
			if best == "" {
				t.Errorf("no offered box holds %s %s: %v", file, name, question.Options)
				return llm.Verdict{}, false
			}
			return typesafetest.Choose(best), true
		}
		return other(key, question)
	}
	return decisions
}

// preset answers the text model's requests of the grouping: a proposal
// divides a box by the next path segment of its files, a description is a
// sentence and a table request gets a plausible cell per column. Any other
// task fails its request.
type preset struct {
	mu        sync.Mutex
	requests  [][]byte
	proposals int
}

func (p *preset) bodies() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.requests)
}

func (*preset) State() []byte { return []byte(`{"provider":"parts-preset"}`) }

func (*preset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (p *preset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task  string `json:"task"`
		Files []struct {
			Path  string   `json:"path"`
			Names []string `json:"names"`
		} `json:"files"`
		Directories []struct {
			Dir   string `json:"dir"`
			Files []struct {
				Name string `json:"name"`
			} `json:"files"`
		} `json:"directories"`
		Fill    []map[string]any `json:"fill"`
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	p.mu.Lock()
	p.requests = append(p.requests, slices.Clone(prepared.Bytes()))
	p.mu.Unlock()
	var response any
	switch request.Task {
	case proposeTask:
		var paths []string
		for _, file := range request.Files {
			paths = append(paths, file.Path)
		}
		for _, dir := range request.Directories {
			for _, file := range dir.Files {
				paths = append(paths, path.Join(dir.Dir, file.Name))
			}
		}
		if len(paths) == 0 {
			return llm.Completion{}, fmt.Errorf("parts preset: a proposal without files")
		}
		p.mu.Lock()
		p.proposals++
		p.mu.Unlock()
		var boxes []map[string]string
		for _, name := range nextSegments(paths) {
			boxes = append(boxes, map[string]string{"name": name, "holds": "The code under " + name + "."})
		}
		response = map[string]any{"boxes": boxes}
	case describeTask:
		response = map[string]string{"description": "Preset description."}
	case "":
		if len(request.Fill) == 0 {
			return llm.Completion{}, fmt.Errorf("parts preset: a request with no task and no table")
		}
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
					options, _ := column["options"].([]any)
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
	default:
		return llm.Completion{}, fmt.Errorf("parts preset: unknown task %q", request.Task)
	}
	raw, err := json.Marshal(response)
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

// nextSegments are the distinct paths one segment below the directory all
// paths share, in first-seen order: the directories and files a box's code
// falls into at the next level. One file gives one segment, itself.
func nextSegments(paths []string) []string {
	split := make([][]string, len(paths))
	common := -1
	for i, file := range paths {
		split[i] = strings.Split(file, "/")
		dirs := split[i][:len(split[i])-1]
		if common < 0 {
			common = len(dirs)
		}
		common = min(common, len(dirs))
		for j := 0; j < common; j++ {
			if dirs[j] != split[0][j] {
				common = j
				break
			}
		}
	}
	var names []string
	for _, segments := range split {
		name := strings.Join(segments[:common+1], "/")
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// checkRequests holds every provider request to trusted inputs: no host
// path of the repository, no README or AGENTS text, no declaration
// documentation.
func checkRequests(t testing.TB, graph atlas.Graph, root string, requests [][]byte) {
	t.Helper()
	for _, raw := range requests {
		if root != "" && bytes.Contains(raw, []byte(root)) {
			t.Fatalf("a provider request carries the host path of the repository: %.300s", raw)
		}
	}
	for _, place := range graph.Places {
		var prose []string
		if place.Document != nil {
			prose = strings.Split(place.Document.Text, "\n")
		}
		if place.File != nil {
			for _, decl := range place.File.Decls {
				prose = append(prose, decl.Doc)
			}
		}
		for _, line := range prose {
			line = strings.TrimSpace(line)
			if len(line) < 24 {
				continue
			}
			for _, raw := range requests {
				if bytes.Contains(raw, []byte(line)) {
					t.Fatalf("a provider request carries %s prose %q", place.Path, line)
				}
			}
		}
	}
}

// checkMembership: every declaration of the target is in exactly one part
// or off the map, by symbol place and by native object in member_ids.
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
	// A file is off the map as a whole only when it declares nothing.
	declares := map[string]bool{}
	for _, place := range graph.Places {
		if place.Symbol != nil && slices.Contains(place.TargetIDs, targetID) {
			declares[place.Path] = true
		}
	}
	for _, entry := range checked.Target.OffMap {
		if len(entry.File.Symbols) == 0 && declares[entry.File.Path] {
			t.Fatalf("%s declares code yet is off the map as a file without declarations (%s)", entry.File.Path, entry.Reason)
		}
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
		decl := place.Symbol.Decl
		checked.Symbols[[2]string{place.Path, decl.Name}] = place.ID
		if seen[place.ID] != 1 {
			t.Fatalf("%s %s is in %d parts or off-map entries, want one", place.Path, decl.Name, seen[place.ID])
		}
		if decl.ObjectID != "" && checked.PartOf[place.ID] != "" && byObject[decl.ObjectID] != checked.PartOf[place.ID] {
			t.Fatalf("%s %s: member_ids name part %q, its part is %q", place.Path, decl.Name, byObject[decl.ObjectID], checked.PartOf[place.ID])
		}
	}
}

// checkFollowers: a method goes with its type, wherever it is declared, and
// a lexical child with the function or method it is declared in.
func checkFollowers(t testing.TB, graph atlas.Graph, targetID string, checked Map) {
	t.Helper()
	at := map[string]string{} // path, line, name → place
	for _, place := range graph.Places {
		if place.Symbol != nil && slices.Contains(place.TargetIDs, targetID) {
			at[fmt.Sprintf("%s\x00%d\x00%s", place.Path, place.Symbol.Decl.LineNo, place.Symbol.Decl.Name)] = place.ID
		}
	}
	same := func(child, parent, what string) {
		if child == "" || parent == "" {
			return
		}
		if checked.PartOf[child] != checked.PartOf[parent] {
			t.Fatalf("%s %s is in part %q, its %s in %q", placeByID(graph, child).Path, placeByID(graph, child).Given, checked.PartOf[child], what, checked.PartOf[parent])
		}
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		for _, member := range place.Symbol.Members {
			same(at[fmt.Sprintf("%s\x00%d\x00%s", member.Path, member.Decl.LineNo, member.Decl.Name)], place.ID, "type")
		}
	}
	for _, place := range graph.Places {
		if place.File == nil || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		decls := place.File.Decls
		for i, parent := range lexicalParents(decls) {
			if parent < 0 {
				continue
			}
			same(at[fmt.Sprintf("%s\x00%d\x00%s", place.Path, decls[i].LineNo, decls[i].Name)],
				at[fmt.Sprintf("%s\x00%d\x00%s", place.Path, decls[parent].LineNo, decls[parent].Name)], "parent")
		}
	}
}

// lexicalParents is, for each declaration of a file, the outermost function
// or method of that file whose source range holds it, or -1.
func lexicalParents(decls []atlas.Decl) []int {
	parents := make([]int, len(decls))
	for i, child := range decls {
		parents[i] = -1
		for j, parent := range decls {
			if i == j || parent.EndLine <= 0 || parent.Kind != "function" && parent.Kind != "method" {
				continue
			}
			starts := parent.LineNo < child.LineNo || parent.LineNo == child.LineNo && parent.Column < child.Column
			if !starts || child.LineNo > parent.EndLine {
				continue
			}
			if parents[i] < 0 || parent.LineNo < decls[parents[i]].LineNo {
				parents[i] = j
			}
		}
	}
	return parents
}

// checkTests: test code stands in the off-canvas part of tests alone; a
// test file's declaration elsewhere follows a type of another file.
func checkTests(t testing.TB, graph atlas.Graph, checked Map) {
	t.Helper()
	test := map[string]bool{}
	for _, place := range graph.Places {
		if place.File != nil {
			test[place.Path] = place.File.Test
		}
	}
	for _, box := range checked.Target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				if box.ForTests && !test[file.Path] {
					t.Fatalf("the test part %q holds %s %s, which is no test code", box.Title, file.Path, symbol.Name)
				}
				if !box.ForTests && test[file.Path] && !methodElsewhere(graph, symbol.ID) {
					t.Fatalf("the part %q on the canvas holds the test code %s %s", box.Title, file.Path, symbol.Name)
				}
			}
		}
	}
}

// checkRegistrations: every registration handing over a declaration of a
// file whose declarations were assigned shows its words with them.
func checkRegistrations(t testing.TB, graph atlas.Graph, targetID string, decisions *jev) {
	t.Helper()
	for _, place := range graph.Places {
		boundary := place.Boundary
		if boundary == nil || boundary.Registrar != nil || boundary.Direction != atlas.DirectionIn || len(boundary.Words) == 0 || boundary.SubjectID == "" || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		subject := placeByID(graph, boundary.SubjectID)
		registered, asked := decisions.registered[subject.Path]
		if !asked || subject.Symbol == nil {
			continue
		}
		if words := strings.Join(boundary.Words, " "); !slices.Contains(registered, words) {
			t.Fatalf("the registration %q of %s is not shown with the declarations of %s: %v", words, subject.Given, subject.Path, registered)
		}
	}
}

// checkHelpers: a decided function or variable helper whose users (the
// declarations of non-test, non-generated code that call it, are decorated
// by it, read it, take it or hand it over, outside its own source range)
// all stand in one part is in that part.
func checkHelpers(t testing.TB, graph atlas.Graph, targetID string, checked Map) {
	t.Helper()
	byID := map[string]atlas.Place{}
	hidden := map[string]bool{}
	for _, place := range graph.Places {
		byID[place.ID] = place
		if place.File != nil {
			hidden[place.Path] = place.File.Test || place.File.Generated
		}
	}
	users := map[string]map[string]bool{}
	link := func(from, to string) {
		source, target := byID[from], byID[to]
		if from == to || target.Symbol == nil || hidden[source.Path] || hidden[target.Path] || !slices.Contains(target.TargetIDs, targetID) {
			return
		}
		if users[to] == nil {
			users[to] = map[string]bool{}
		}
		users[to][from] = true
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Resolution == "exact" && (call.Kind == "calls" || call.Kind == "decorates") {
				for _, callee := range call.CalleeIDs {
					link(place.ID, callee)
				}
			}
		}
		for _, use := range place.Symbol.Uses {
			if use.Resolution == "exact" && slices.Contains([]string{"decorates", atlas.UseTakes, "reads", "passes_callback"}, use.Kind) {
				link(place.ID, use.PlaceID)
			}
		}
	}
	for key := range checked.Helpers {
		id := checked.Symbols[key]
		place := byID[id]
		if place.Symbol == nil || place.Symbol.Decl.Kind != "function" && place.Symbol.Decl.Kind != "variable" {
			continue
		}
		decl := place.Symbol.Decl
		part, one := "", false
		for user := range users[id] {
			if other := byID[user]; other.Path == place.Path && decl.EndLine > 0 && other.LineNo >= decl.LineNo && other.LineNo <= decl.EndLine {
				continue
			}
			at := checked.PartOf[user]
			if at == "" || part != "" && at != part {
				part, one = "", false
				break
			}
			part, one = at, true
		}
		if one && checked.PartOf[id] != part {
			t.Fatalf("the helper %s %s is in part %q, yet every declaration that uses it is in part %s", key[0], key[1], checked.PartOf[id], part)
		}
	}
}

// methodElsewhere says whether a declaration follows a type declared in
// another file: its part is its type's, whatever its own file.
func methodElsewhere(graph atlas.Graph, id string) bool {
	place := placeByID(graph, id)
	for _, candidate := range graph.Places {
		if candidate.Symbol == nil || candidate.Path == place.Path {
			continue
		}
		for _, member := range candidate.Symbol.Members {
			if member.Path == place.Path && place.Symbol != nil && member.Decl.Name == place.Symbol.Decl.Name {
				return true
			}
		}
	}
	return false
}

func placeByID(graph atlas.Graph, id string) atlas.Place {
	for _, place := range graph.Places {
		if place.ID == id {
			return place
		}
	}
	return atlas.Place{}
}

func stringList(value any) []string {
	var result []string
	items, _ := value.([]any)
	for _, item := range items {
		result = append(result, fmt.Sprint(item))
	}
	return result
}
