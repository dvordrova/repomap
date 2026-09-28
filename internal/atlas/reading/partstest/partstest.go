// Package partstest checks a language fixture's map of parts: it reads the
// fixture's graph with a preset provider that draws every listed unit (a
// whole file, or one box of a split file) as its own part, then checks what
// the parts request carries and that every
// declaration has exactly one part or an entry off the map. Language fixture
// tests of every adapter share it. CheckSplit reads the same graph with the
// role split of every candidate file: two boxes per file, the units that are
// no helpers put in them alternately and every fifth left open for the code
// to place by its users, so the split's rules are checked on each adapter's
// real facts. Its helper question takes a declaration for a helper when
// other code calls, reads or hands it over, the language does not export
// it and no registration names it.
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
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

const (
	partsTask = "repomap.atlas.parts.v2"
	boxesTask = "repomap.atlas.file_boxes.v1"
)

// Map is one target's checked map of parts: its atlas target and, by symbol
// place ID, the part each declaration takes ("" off the map).
type Map struct {
	// Atlas is the whole reading; Target is the checked target in it.
	Atlas  atlas.Atlas
	Target atlas.Target
	PartOf map[string]string
	// Symbols finds a declaration's symbol place by its file and name.
	Symbols map[[2]string]string
	// Split are the paths of the files the role split split, and RoleParts
	// the IDs of the parts their boxes became.
	Split     map[string]bool
	RoleParts map[string]bool
	// Registered are, by file path, the words of the registrations the
	// assignment showed with that file's declarations ("HandleFunc /x").
	Registered map[string][]string
	// HelperItems are the helper question's items by file and name, and
	// Helpers the declarations the check's helper rule took for helpers.
	HelperItems map[[2]string]map[string]any
	Helpers     map[[2]string]bool
	// Rejected is the reading's journal of what it refused or recorded.
	Rejected []modeldiag.Row
}

// Check reads the graph for one target with the gate answering "one box"
// for every candidate file, and checks the parts request and the
// membership of every declaration. root is the fixture's directory on disk,
// which must not reach the request.
func Check(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string) Map {
	t.Helper()
	return check(t, graph, target, root, false)
}

// CheckSplit reads the graph with every candidate file split in two boxes
// and checks, besides what Check does, that a split happened, that every
// declaration of a split file is in a role part or off the map as
// undecided, that no undecided unit is one the code should have placed by
// its users or what it uses, that a repeated name stays one unit, that a
// module body is a row of the assignment, that no import-only arrow touches
// a role part, and that a seed declaration in a split file keeps the entry:
// the part holding it stands in the "in" column.
func CheckSplit(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string) Map {
	t.Helper()
	return check(t, graph, target, root, true)
}

func check(t testing.TB, graph atlas.Graph, target reading.TargetMeta, root string, split bool) Map {
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
	// Every candidate explains its part and is a key; every part is the
	// domain; the gate keeps every file whole unless the check splits.
	gate := typesafetest.Choose("one box")
	if split {
		gate = typesafetest.Choose("several boxes")
	}
	byColumn := typesafetest.ByColumn(map[string]llm.Verdict{
		"explains": typesafetest.Yes(0.9), "role": typesafetest.Choose("domain"), "key_symbol": typesafetest.Choose("yes"), "boxes": gate,
		"helper": typesafetest.Choose("responsibility"),
	})
	exported := map[[2]string]bool{}
	for _, place := range graph.Places {
		if place.Symbol != nil {
			exported[[2]string{place.Path, place.Symbol.Decl.Name}] = place.Symbol.Decl.Exported
		}
	}
	var helpersMu sync.Mutex
	helpers := map[[2]string]bool{}
	categorizer := &recording{Categorizer: typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if strings.HasSuffix(key, "|helper") && split {
			// A helper has users, is not exported and is not registered.
			file, _ := question.Item["file"].(string)
			name, _ := question.Item["name"].(string)
			used := question.Item["called_by"] != nil || question.Item["read_by"] != nil || question.Item["handed_over_by"] != nil
			if used && question.Item["registered"] == nil && !exported[[2]string{file, name}] {
				helpersMu.Lock()
				helpers[[2]string{file, name}] = true
				helpersMu.Unlock()
				return typesafetest.Choose("helper"), true
			}
			return typesafetest.Choose("responsibility"), true
		}
		if !strings.HasSuffix(key, "|box") {
			return byColumn(key, question)
		}
		// dN goes in the first box when N is odd and in the second when it
		// is even; every fifth declaration is a near-tie, left undecided.
		var n int
		fmt.Sscanf(key, "d%d|box", &n)
		first, second := question.Options[0].Name, question.Options[1].Name
		if n%5 == 0 {
			return llm.Verdict{Choice: first, Probabilities: map[string]float64{first: 0.5, second: 0.5}}, true
		}
		if n%2 == 0 {
			first = second
		}
		return typesafetest.Choose(first), true
	}}}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Targets: []reading.TargetMeta{target}, Repository: "fixture", Revision: "test",
		Executor: llm.Executor{BatchConcurrency: 4, BatchController: &llm.BatchController{}},
		Provider: provider, Categorizer: categorizer, OwnerRunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	checked := Map{Atlas: result.Atlas, PartOf: map[string]string{}, Symbols: map[[2]string]string{}, Helpers: helpers, Rejected: result.Rejected}
	for _, candidate := range result.Atlas.Targets {
		if candidate.ID == target.ID {
			checked.Target = candidate
		}
	}
	if checked.Target.MapFailure != "" {
		t.Fatalf("map failure: %s", checked.Target.MapFailure)
	}
	// A whole file of helpers used from one box joins that box and is no
	// row of its own.
	joined := map[string]bool{}
	for _, row := range result.Rejected {
		if row.Kind == "role_attached" {
			for _, sample := range row.Samples {
				joined[sample] = true
			}
		}
	}
	checkRequest(t, graph, target.ID, root, provider.requests, split, joined)
	checkMembership(t, graph, target.ID, checked)
	checked.Split, checked.RoleParts = map[string]bool{}, map[string]bool{}
	checked.Registered = categorizer.registered
	checked.HelperItems = categorizer.helperItems
	if split {
		checkSplit(t, graph, target.ID, checked, categorizer, provider)
	} else if categorizer.assigned > 0 {
		t.Fatalf("the gate answered one box, yet %d declarations were assigned", categorizer.assigned)
	}
	return checked
}

// recording is the categorizer of a check: it keeps what the gate and the
// assignment were asked.
type recording struct {
	typesafetest.Categorizer
	mu       sync.Mutex
	gated    int
	assigned int
	// items are, by file path, the declarations the assignment asked about;
	// registered the words of their registrations.
	items, registered map[string][]string
	// helperItems are the helper question's items by file and name, and
	// helperAsked how often each was asked.
	helperItems map[[2]string]map[string]any
	helperAsked map[[2]string]int
}

func (c *recording) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var body struct {
		State struct {
			Context map[string]any `json:"context"`
		} `json:"state"`
		Questions map[string]struct {
			Instructions map[string]any `json:"instructions"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &body); err == nil {
		c.mu.Lock()
		if c.items == nil {
			c.items, c.registered = map[string][]string{}, map[string][]string{}
			c.helperItems, c.helperAsked = map[[2]string]map[string]any{}, map[[2]string]int{}
		}
		for key, question := range body.Questions {
			switch {
			case strings.HasSuffix(key, "|helper"):
				item, _ := question.Instructions["declaration"].(map[string]any)
				file, _ := item["file"].(string)
				name, _ := item["name"].(string)
				c.helperItems[[2]string{file, name}] = item
				c.helperAsked[[2]string{file, name}]++
			case strings.HasSuffix(key, "|boxes"):
				c.gated++
			case strings.HasSuffix(key, "|box"):
				file, _ := body.State.Context["file"].(string)
				declaration, _ := question.Instructions["declaration"].(map[string]any)
				name, _ := declaration["declaration"].(string)
				c.assigned++
				if name != "" {
					c.items[file] = append(c.items[file], name)
				}
				words, _ := declaration["registered"].([]any)
				for _, word := range words {
					c.registered[file] = append(c.registered[file], fmt.Sprint(word))
				}
			}
		}
		c.mu.Unlock()
	}
	return c.Categorizer.Complete(ctx, prepared)
}

// checkSplit checks what the role split must keep on real facts.
func checkSplit(t testing.TB, graph atlas.Graph, targetID string, checked Map, categorizer *recording, provider *preset) {
	t.Helper()
	if categorizer.gated == 0 || categorizer.assigned == 0 {
		t.Fatalf("no split happened: %d gate questions, %d assignments", categorizer.gated, categorizer.assigned)
	}
	checkHelpers(t, graph, targetID, checked, categorizer, provider)
	// Every registration handing over a declaration of an assigned file
	// shows its words with that file's declarations.
	for _, place := range graph.Places {
		boundary := place.Boundary
		if boundary == nil || boundary.Direction != atlas.DirectionIn || len(boundary.Words) == 0 || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		subject := placeByID(graph, boundary.SubjectID)
		if len(categorizer.items[subject.Path]) == 0 {
			continue
		}
		if words := strings.Join(boundary.Words, " "); !slices.Contains(categorizer.registered[subject.Path], words) {
			t.Fatalf("the registration %q of %s is not shown with the declarations of %s: %v", words, subject.Given, subject.Path, categorizer.registered[subject.Path])
		}
	}
	// The preset names each file's boxes after its path.
	for _, box := range checked.Target.Boxes {
		if path, ok := strings.CutSuffix(box.Title, ": first"); ok {
			checked.Split[path], checked.RoleParts[box.ID] = true, true
		} else if path, ok := strings.CutSuffix(box.Title, ": second"); ok {
			checked.Split[path], checked.RoleParts[box.ID] = true, true
		}
	}
	if len(checked.RoleParts) < 2 {
		t.Fatalf("the split drew %d role parts", len(checked.RoleParts))
	}
	undecided := map[string]bool{}
	for _, entry := range checked.Target.OffMap {
		if entry.Reason != atlas.OffMapUndecided {
			continue
		}
		if !checked.Split[entry.File.Path] {
			t.Fatalf("undecided declarations of %s, which is not split", entry.File.Path)
		}
		for _, symbol := range entry.File.Symbols {
			undecided[symbol.ID] = true
		}
	}
	checkUndecided(t, graph, targetID, checked, undecided)
	for _, place := range graph.Places {
		if place.File == nil || !checked.Split[place.Path] || !slices.Contains(place.TargetIDs, targetID) {
			continue
		}
		byName := map[string]string{}
		for _, decl := range place.File.Decls {
			id := checked.Symbols[[2]string{place.Path, decl.Name}]
			for _, candidate := range graph.Places {
				if candidate.Symbol != nil && candidate.Path == place.Path && candidate.LineNo == decl.LineNo && candidate.Symbol.Decl.Name == decl.Name {
					id = candidate.ID
				}
			}
			part := checked.PartOf[id]
			if part != "" && !checked.RoleParts[part] && !methodElsewhere(graph, id) {
				t.Fatalf("%s %s of a split file is in part %s, not a role part", place.Path, decl.Name, part)
			}
			if part == "" && !undecided[id] && !methodElsewhere(graph, id) {
				t.Fatalf("%s %s of a split file is off the map but not undecided", place.Path, decl.Name)
			}
			// A repeated name is one unit: every declaration of it is where
			// the first one is.
			if first, repeated := byName[decl.Name]; repeated && checked.PartOf[first] != part {
				t.Fatalf("%s repeats %s in parts %q and %q", place.Path, decl.Name, checked.PartOf[first], part)
			} else if !repeated {
				byName[decl.Name] = id
			}
			if decl.Kind == "module" && id != "" && !slices.Contains(categorizer.items[place.Path], decl.Name) {
				t.Fatalf("the module body of %s is no row of the assignment: %v", place.Path, categorizer.items[place.Path])
			}
		}
	}
	// An input whose handler is undecided names no part.
	for _, boundary := range checked.Target.Boundaries {
		for _, place := range graph.Places {
			if place.Boundary == nil || place.Path != boundary.Path || place.LineNo != boundary.LineNo || place.Column != boundary.Column {
				continue
			}
			if place.Boundary.Direction == atlas.DirectionIn && undecided[place.Boundary.SubjectID] && boundary.BoxID != "" {
				t.Fatalf("the input at %s:%d of an undecided handler stands in part %s", boundary.Path, boundary.LineNo, boundary.BoxID)
			}
		}
	}
	for _, arrow := range checked.Target.Arrows {
		if len(arrow.Witnesses) == 0 && (checked.RoleParts[arrow.From] || checked.RoleParts[arrow.To]) {
			t.Fatalf("an import-only arrow %s -> %s touches a role part: a split file has no endpoint", arrow.From, arrow.To)
		}
	}
	for _, seed := range graph.SeedDecls {
		place := placeByID(graph, seed)
		if !slices.Contains(place.TargetIDs, targetID) || !checked.Split[place.Path] {
			continue
		}
		part := checked.PartOf[seed]
		if part == "" {
			continue // an undecided seed declaration enters through no part
		}
		for _, box := range checked.Target.Boxes {
			if box.ID == part && box.Side != atlas.SideIn {
				t.Fatalf("the part %s holding the seed %s stands %q, not in", part, place.Symbol.Decl.Name, box.Side)
			}
		}
	}
}

// checkUndecided holds every undecided unit of a split file to the code
// rule on real facts: the declarations of its file that use it (call it,
// are decorated by it, read it when it does not run; never a hand-over) are
// not all in one part, and when none uses it, what it uses in its file is
// not all in one part either. The preset gives each box a part of its own.
func checkUndecided(t testing.TB, graph atlas.Graph, targetID string, checked Map, undecided map[string]bool) {
	t.Helper()
	byID := map[string]atlas.Place{}
	test := map[string]bool{}
	for _, place := range graph.Places {
		byID[place.ID] = place
		if place.File != nil {
			test[place.Path] = place.File.Test || place.File.Generated
		}
	}
	// users and uses by declaration, between declarations of one file that
	// is not test or generated code; a follower's are its own.
	users, uses := map[string]map[string]bool{}, map[string]map[string]bool{}
	link := func(from, to string) {
		source, target := byID[from], byID[to]
		if from == to || target.Symbol == nil || source.Path != target.Path || test[source.Path] || !slices.Contains(target.TargetIDs, targetID) {
			return
		}
		if users[to] == nil {
			users[to] = map[string]bool{}
		}
		if uses[from] == nil {
			uses[from] = map[string]bool{}
		}
		users[to][from], uses[from][to] = true, true
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
			used := byID[use.PlaceID]
			runs := used.Symbol != nil && slices.Contains([]string{"function", "method", "lambda"}, used.Symbol.Decl.Kind)
			if use.Resolution == "exact" && (use.Kind == "decorates" || use.Kind == "reads" && !runs) {
				link(place.ID, use.PlaceID)
			}
		}
	}
	// A declaration that follows a type of another file is its type's.
	inFile := func(ids map[string]bool) []string {
		var result []string
		for id := range ids {
			if !undecided[id] && !methodElsewhere(graph, id) {
				result = append(result, id)
			}
		}
		return result
	}
	onePart := func(ids []string) bool {
		part := ""
		for _, id := range ids {
			if checked.PartOf[id] == "" || part != "" && checked.PartOf[id] != part {
				return false
			}
			part = checked.PartOf[id]
		}
		return part != ""
	}
	// An undecided entry lists a unit with its followers; their users and
	// uses are the unit's.
	for _, entry := range checked.Target.OffMap {
		if entry.Reason != atlas.OffMapUndecided {
			continue
		}
		unitUsers, unitUses := map[string]bool{}, map[string]bool{}
		for _, symbol := range entry.File.Symbols {
			for id := range users[symbol.ID] {
				unitUsers[id] = true
			}
			for id := range uses[symbol.ID] {
				unitUses[id] = true
			}
		}
		if placed := inFile(unitUsers); len(unitUsers) > 0 {
			if len(placed) == len(unitUsers) && onePart(placed) {
				t.Fatalf("%s %s is undecided, yet every declaration of its file that uses it is in part %s", entry.File.Path, entry.File.Symbols[0].Name, checked.PartOf[placed[0]])
			}
			continue
		}
		if placed := inFile(unitUses); len(placed) > 0 && len(placed) == len(unitUses) && onePart(placed) {
			t.Fatalf("%s %s is undecided, no declaration of its file uses it and all it uses is in part %s", entry.File.Path, entry.File.Symbols[0].Name, checked.PartOf[placed[0]])
		}
	}
}

// checkHelpers holds the helper question and the code rules for helpers to
// real facts: no declaration of test or generated code is asked, none is
// asked twice, no helper is named, no unit is assigned twice, and a helper
// of a split file whose users (the declarations of the program that call
// it, are decorated by it or read it when it does not run; never a
// hand-over) all stand in one part is in that part.
func checkHelpers(t testing.TB, graph atlas.Graph, targetID string, checked Map, categorizer *recording, provider *preset) {
	t.Helper()
	hidden := map[string]bool{}
	for _, place := range graph.Places {
		if place.File != nil {
			hidden[place.Path] = place.File.Test || place.File.Generated
		}
	}
	if len(categorizer.helperAsked) == 0 {
		t.Fatal("the helper question asked nothing")
	}
	for key, count := range categorizer.helperAsked {
		if count != 1 || hidden[key[0]] {
			t.Fatalf("%s %s was asked the helper question %d times (test or generated: %v)", key[0], key[1], count, hidden[key[0]])
		}
	}
	for path, names := range provider.named {
		for _, name := range names {
			if checked.Helpers[[2]string{path, name}] {
				t.Fatalf("the naming of %s lists the helper %s", path, name)
			}
		}
	}
	for path, names := range categorizer.items {
		for i, name := range names {
			if slices.Contains(names[i+1:], name) {
				t.Fatalf("%s %s was assigned twice", path, name)
			}
		}
	}
	byID := map[string]atlas.Place{}
	for _, place := range graph.Places {
		byID[place.ID] = place
	}
	// users by declaration, between declarations of files that are neither
	// test nor generated code, in any file.
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
			used := byID[use.PlaceID]
			runs := used.Symbol != nil && slices.Contains([]string{"function", "method", "lambda"}, used.Symbol.Decl.Kind)
			if use.Resolution == "exact" && (use.Kind == "decorates" || use.Kind == "reads" && !runs) {
				link(place.ID, use.PlaceID)
			}
		}
	}
	for key := range checked.Helpers {
		id := checked.Symbols[key]
		place := byID[id]
		// A function or variable is its unit alone, but for what its source
		// range holds, which only it can call.
		if !checked.Split[key[0]] || place.Symbol == nil || place.Symbol.Decl.Kind != "function" && place.Symbol.Decl.Kind != "variable" {
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

var (
	pairPattern   = regexp.MustCompile(`^([fc][0-9]+) -> ([fc][0-9]+)( \([0-9]+\))?$`)
	importPattern = regexp.MustCompile(`^(f[0-9]+) -> (f[0-9]+)$`)
)

// checkRequest checks the parts request: code structure only, one row per
// whole file (its f* ref) or per box of a split file (a c* ref with the
// box's name), a split file never a whole row, every file with a
// declaration of its own listed unless it joined a box (joined), and calls
// and imports over listed refs only, imports between whole files alone.
func checkRequest(t testing.TB, graph atlas.Graph, targetID, root string, requests [][]byte, split bool, joined map[string]bool) {
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
		if !slices.Contains([]string{"task", "units", "calls", "imports"}, key) {
			t.Fatalf("the parts request carries %q", key)
		}
	}
	var body struct {
		Units   []map[string]any `json:"units"`
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
	listed, whole, boxed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	byPath := map[string]atlas.Place{}
	for _, place := range files {
		byPath[place.Path] = place
	}
	for _, file := range body.Units {
		for key := range file {
			if !slices.Contains([]string{"ref", "path", "box", "units", "types", "functions", "variables"}, key) {
				t.Fatalf("a unit row carries %q", key)
			}
		}
		ref := fmt.Sprint(file["ref"])
		if listed[ref] {
			t.Fatalf("unit row %v is repeated", file)
		}
		listed[ref] = true
		if box, isBox := file["box"]; isBox {
			place, ok := byPath[fmt.Sprint(file["path"])]
			if !ok || !strings.HasPrefix(ref, "c") || fmt.Sprint(box) == "" || !split {
				t.Fatalf("box row %v is unknown, not a c* ref, unnamed or drawn without a split", file)
			}
			boxed[place.ID] = true
			continue
		}
		place, ok := files[ref]
		if !ok || fmt.Sprint(file["path"]) != place.Path || strings.HasPrefix(place.Path, "/") {
			t.Fatalf("file row %v is unknown or not its own path", file)
		}
		whole[ref] = true
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
		// A name the file declares twice (a second Go init, overload stubs,
		// a Clojure declare) is one unit: listed once and counted once, with
		// the module body the only unit no list names.
		modules := 0
		for i, decl := range place.File.Decls {
			if decl.Kind == "module" && !children[i] {
				modules++
			}
		}
		unique := slices.Clone(names)
		slices.Sort(unique)
		if len(slices.Compact(unique)) != len(names) {
			t.Fatalf("%s lists a repeated name more than once: %v", place.Path, names)
		}
		if units, _ := file["units"].(float64); int(units) != len(names)+modules {
			t.Fatalf("%s counts %v units for %d names and %d module bodies", place.Path, file["units"], len(names), modules)
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
		if own && !whole[id] && !boxed[id] && !joined[place.Path] {
			t.Fatalf("%s holds code but is not listed", place.Path)
		}
		if joined[place.Path] && (whole[id] || boxed[id]) {
			t.Fatalf("%s joined a box yet is listed", place.Path)
		}
		if whole[id] && boxed[id] {
			t.Fatalf("%s is split yet listed as a whole row", place.Path)
		}
	}
	if split && len(boxed) == 0 {
		t.Fatal("no split file's boxes are rows of the parts request")
	}
	for _, pair := range body.Calls {
		match := pairPattern.FindStringSubmatch(pair)
		if match == nil || !listed[match[1]] || !listed[match[2]] || match[1] == match[2] {
			t.Fatalf("call aggregate %q names a ref the request does not list", pair)
		}
	}
	for _, pair := range body.Imports {
		match := importPattern.FindStringSubmatch(pair)
		if match == nil || !whole[match[1]] || !whole[match[2]] || match[1] == match[2] {
			t.Fatalf("import %q names a ref that is not a listed whole file", pair)
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
	// A file is off the map as a whole only when it declares nothing; one
	// whose declarations follow units of other files is on the map through
	// them. Declarations off the map in a file a part holds name that part.
	declares := map[string]bool{}
	for _, place := range graph.Places {
		if place.Symbol != nil && slices.Contains(place.TargetIDs, targetID) {
			declares[place.Path] = true
		}
	}
	holds := map[[2]string]bool{}
	for _, box := range checked.Target.Boxes {
		for _, file := range box.Files {
			holds[[2]string{box.ID, file.Path}] = true
		}
	}
	for _, entry := range checked.Target.OffMap {
		if len(entry.File.Symbols) == 0 && declares[entry.File.Path] {
			t.Fatalf("%s declares code yet is off the map as a file without declarations (%s)", entry.File.Path, entry.Reason)
		}
		if entry.BoxID != "" && !holds[[2]string{entry.BoxID, entry.File.Path}] {
			t.Fatalf("off-map declarations of %s name part %s, which does not hold the file", entry.File.Path, entry.BoxID)
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

// preset answers every text-model request of the reading: every listed unit
// is its own part, every description is a sentence, no areas are drawn, and
// a table cell takes its first option or a short text.
type preset struct {
	mu       sync.Mutex
	requests [][]byte
	// named are, by file path, the declarations each naming request lists.
	named map[string][]string
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
		Units   []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
			Box  string `json:"box"`
		} `json:"units"`
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
		// Each unit is a part of its own, named after its file or its box
		// (the naming names a box after its file).
		var groups []map[string]any
		for _, unit := range request.Units {
			name := unit.Path
			if unit.Box != "" {
				name = unit.Box
			}
			groups = append(groups, map[string]any{"name": name, "units": []string{unit.Ref}})
		}
		response = map[string]any{"groups": groups}
	case "repomap.atlas.describe.v1":
		response = map[string]string{"description": "Preset description."}
	case boxesTask:
		var named struct {
			File struct {
				Path         string `json:"path"`
				Declarations []struct {
					Name string `json:"name"`
				} `json:"declarations"`
			} `json:"file"`
		}
		if err := json.Unmarshal(prepared.Bytes(), &named); err != nil {
			return llm.Completion{}, err
		}
		p.mu.Lock()
		if p.named == nil {
			p.named = map[string][]string{}
		}
		for _, decl := range named.File.Declarations {
			p.named[named.File.Path] = append(p.named[named.File.Path], decl.Name)
		}
		p.mu.Unlock()
		response = map[string]any{"boxes": []map[string]string{
			{"name": named.File.Path + ": first", "holds": "The odd declarations."},
			{"name": named.File.Path + ": second", "holds": "The even declarations."},
		}}
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
