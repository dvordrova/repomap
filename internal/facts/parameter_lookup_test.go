package facts

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// This is the original scan, retained only as a complete behavioral oracle.
// In particular its traversal state belongs to one walk, not a cached slot.
func scannedParameterValue(target *targetContext, value *sourcevalue.Value, reached map[parameterSlot]bool) (passed *sourcevalue.Anchor, known bool) {
	if value == nil || value.Kind != "parameter" || value.Owner == nil || value.Position == 0 {
		return nil, false
	}
	slot := parameterSlot{owner: *value.Owner, position: value.Position}
	if reached[slot] {
		return nil, true
	}
	reached[slot] = true
	var owner string
	for _, object := range target.input.Index.Objects {
		if isCallable(object) && object.Location != nil && object.Location.Path == value.Owner.Path && object.Location.Line == value.Owner.Line {
			owner = object.ID
			break
		}
	}
	if owner == "" {
		return nil, false
	}
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || len(relation.ToIDs) != 1 || relation.ToIDs[0] != owner {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.Position != value.Position {
					continue
				}
				known = true
				anchor := producedAt(argument.Origin)
				if anchor == nil {
					handed, handedKnown := scannedParameterValue(target, argument.Origin, reached)
					if !handedKnown {
						return nil, false
					}
					if anchor = handed; anchor == nil {
						continue
					}
				}
				if passed != nil && *passed != *anchor {
					return nil, false
				}
				passed = anchor
			}
		}
	}
	return passed, known
}

func TestParameterLookupPreservesCompleteOriginalWalk(t *testing.T) {
	parameter := func(line, column int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "parameter", Position: 1, Owner: &sourcevalue.Anchor{Path: "routes.py", Line: line, Column: column}}
	}
	produced := func(line int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "routes.py", Line: line, Column: 9}}
	}
	call := func(to string, values ...*sourcevalue.Value) programindex.Relation {
		relation := programindex.Relation{Kind: programindex.RelationCalls, ToIDs: []string{to}}
		for _, value := range values {
			relation.Patterns = append(relation.Patterns, programindex.RelationPattern{Arguments: []programindex.PatternArgument{{Position: 1, Origin: value}}})
		}
		return relation
	}
	objects := []programindex.Object{
		{ID: "first", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "routes.py", Line: 10, Column: 5}},
		{ID: "second", Kind: programindex.ObjectLambda, Location: &programindex.Location{Path: "routes.py", Line: 10, Column: 25}},
		{ID: "branch", Kind: programindex.ObjectMethod, Location: &programindex.Location{Path: "routes.py", Line: 20, Column: 5}},
		{ID: "leaf", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "routes.py", Line: 30}},
	}
	ignoredAlternative := call("first", produced(99))
	ignoredAlternative.ToIDs = []string{"first", "second"}
	ignoredExternal := call("first", produced(98))
	ignoredExternal.Kind = programindex.RelationInvokesExternal
	omitted := call("first", produced(70))
	omitted.TargetsOmitted = 1 // Preserve the current scan's exact condition.
	for _, tc := range []struct {
		name      string
		relations []programindex.Relation
		value     *sourcevalue.Value
		visited   bool
	}{
		{"nil", nil, nil, false},
		{"no caller", nil, parameter(10, 5), false},
		{"unknown owner", []programindex.Relation{call("first", produced(70))}, parameter(11, 5), false},
		{"same line first native owner", []programindex.Relation{call("second", produced(71)), call("first", produced(70))}, parameter(10, 25), false},
		{"different columns ignored", []programindex.Relation{call("first", produced(70))}, parameter(10, 99), false},
		{"multiple patterns same value", []programindex.Relation{call("first", produced(70), produced(70))}, parameter(10, 5), false},
		{"unknown argument after known", []programindex.Relation{call("first", produced(70), nil)}, parameter(10, 5), false},
		{"different callers", []programindex.Relation{call("first", produced(70)), call("first", produced(71))}, parameter(10, 5), false},
		{"self recursion", []programindex.Relation{call("first", parameter(10, 5)), call("first", produced(70))}, parameter(10, 5), false},
		{"mutual recursion", []programindex.Relation{call("first", parameter(20, 5)), call("branch", parameter(10, 5)), call("branch", produced(70))}, parameter(10, 5), false},
		{"converging paths", []programindex.Relation{call("leaf", parameter(10, 5), parameter(20, 5)), call("first", parameter(20, 5)), call("branch", produced(70))}, parameter(30, 0), false},
		{"inconsistent cycle", []programindex.Relation{call("first", parameter(20, 5), produced(71)), call("branch", parameter(10, 5), produced(70))}, parameter(10, 5), false},
		{"previsited slot", []programindex.Relation{call("first", produced(70))}, parameter(10, 5), true},
		{"relation filters and omissions", []programindex.Relation{ignoredAlternative, ignoredExternal, omitted}, parameter(10, 5), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := TargetInput{Root: ".", Index: programindex.Index{Objects: objects, Relations: tc.relations}}
			before, _ := json.Marshal(input.Index)
			target, err := newTargetContext(input)
			if err != nil {
				t.Fatal(err)
			}
			scanReached, lookupReached := map[parameterSlot]bool{}, map[parameterSlot]bool{}
			if tc.visited {
				slot := parameterSlot{owner: *tc.value.Owner, position: tc.value.Position}
				scanReached[slot], lookupReached[slot] = true, true
			}
			want, wantKnown := scannedParameterValue(target, tc.value, scanReached)
			got, gotKnown := target.parameterValue(tc.value, lookupReached)
			if !reflect.DeepEqual(got, want) || gotKnown != wantKnown || !reflect.DeepEqual(scanReached, lookupReached) {
				t.Fatalf("lookup %v/%t/%v differs from original %v/%t/%v", got, gotKnown, lookupReached, want, wantKnown, scanReached)
			}
			after, _ := json.Marshal(input.Index)
			if string(before) != string(after) {
				t.Fatal("lookup changed original native evidence")
			}
		})
	}
}

func BenchmarkParameterOwnerLookup(b *testing.B) {
	var objects []programindex.Object
	for line := 1; line <= 10000; line++ {
		objects = append(objects, programindex.Object{ID: fmt.Sprint(line), Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "routes.py", Line: line}})
	}
	value := &sourcevalue.Value{Kind: "parameter", Position: 1, Owner: &sourcevalue.Anchor{Path: "routes.py", Line: 10000}}
	target, _ := newTargetContext(TargetInput{Root: ".", Index: programindex.Index{Objects: objects}})
	target.values()
	for _, tc := range []struct {
		name string
		read func(*targetContext, *sourcevalue.Value, map[parameterSlot]bool) (*sourcevalue.Anchor, bool)
	}{
		{"original scan", scannedParameterValue},
		{"existing complete index", func(t *targetContext, v *sourcevalue.Value, r map[parameterSlot]bool) (*sourcevalue.Anchor, bool) {
			return t.parameterValue(v, r)
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				tc.read(target, value, map[parameterSlot]bool{})
			}
		})
	}
}

func TestParameterLookupMatchesCompleteNativePythonFixture(t *testing.T) {
	// Copy exactly the cumulative inventory, then use the ordinary native
	// producer. No fabricated native origin substitutes for these expressions.
	var inventory struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	bytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "python.files.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &inventory); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, entry := range inventory.Entries {
		original := filepath.Join("..", "..", "testdata", "repositories", "python", filepath.FromSlash(entry.Path))
		value, err := os.ReadFile(original)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(original)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, value, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}} {
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %s: %v", output, err)
		}
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var selected pythontarget.Target
	for _, target := range catalog.Entries {
		if target.Selector == "python:.:script:repomap-fixture" {
			selected = target
		}
	}
	if selected.Selector == "" {
		t.Fatal("complete native fixture lost its selected target")
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, selected)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(index)
	count, known, unknown, cyclic := 0, 0, 0, 0
	var visit func(*sourcevalue.Value)
	visit = func(value *sourcevalue.Value) {
		if value == nil {
			return
		}
		if value.Kind == "parameter" {
			oldReached, newReached := map[parameterSlot]bool{}, map[parameterSlot]bool{}
			want, wantKnown := scannedParameterValue(target, value, oldReached)
			got, gotKnown := target.parameterValue(value, newReached)
			if !reflect.DeepEqual(want, got) || wantKnown != gotKnown || !reflect.DeepEqual(oldReached, newReached) {
				t.Fatalf("native parameter %+v: indexed %v/%t differs from scan %v/%t", value, got, gotKnown, want, wantKnown)
			}
			count++
			if got != nil && gotKnown {
				known++
			} else {
				unknown++
			}
			if len(oldReached) > 1 {
				cyclic++
			}
		}
		visit(value.Initializer)
		for i := range value.Parts {
			visit(&value.Parts[i])
		}
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			visit(pattern.ReceiverValue)
			visit(pattern.ResultValue)
			for _, argument := range pattern.Arguments {
				visit(argument.Origin)
			}
		}
	}
	if count == 0 || known == 0 || unknown == 0 || cyclic == 0 {
		t.Fatalf("native contrasts absent: parameters%d known%d unknown%d multi-slot%d", count, known, unknown, cyclic)
	}
	after, _ := json.Marshal(index)
	if string(before) != string(after) {
		t.Fatal("indexed walk changed complete native input")
	}
	t.Logf("full cumulative native inventory%d objects%d relations%d: all%d native pattern parameter expressions identical; known%d unknown%d multi-slot%d", len(inventory.Entries), len(index.Objects), len(index.Relations), count, known, unknown, cyclic)
}
