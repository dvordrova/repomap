package programindex

import (
	"encoding/json"
	"strings"
	"testing"
)

// sameValueInput is one declaration reading one setting in two spellings:
// in Go one relation holds both calls; in C, JS/TS and Clojure each call is
// its own relation. Both shapes are one fact.
func sameValueInput(oneRelation bool) Input {
	input := shapeInput()
	call := func(ref, word string, line int) RelationPatternInput {
		return RelationPatternInput{
			SourceRef: ref, Form: PatternCall, Selector: "get", Location: &Location{Path: "root.lang", Line: line, Column: 9},
			Arguments: []PatternArgumentInput{{Position: 1, Kind: PatternLiteralString, Value: word}}, ArgumentsObserved: 1,
		}
	}
	first, second := call("first-call", "storageClass", 3), call("second-call", "storage-class", 5)
	relation := func(ref string, patterns ...RelationPatternInput) RelationInput {
		return RelationInput{
			SourceRef: ref, Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
			Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
			Location: patterns[0].Location, Patterns: patterns, PatternsObserved: len(patterns),
		}
	}
	if oneRelation {
		second.SameValueAs = &PatternRefInput{RelationSourceRef: "reads", PatternSourceRef: "first-call"}
		input.Relations = []RelationInput{relation("reads", first, second)}
		return input
	}
	second.SameValueAs = &PatternRefInput{RelationSourceRef: "read-first", PatternSourceRef: "first-call"}
	input.Relations = []RelationInput{relation("read-first", first), relation("read-second", second)}
	return input
}

// A call reading the same value as an earlier call names that call's
// sealed pattern, in one relation or across two, survives the artifact and
// is refused when it names no pattern, a later one, one of another callee
// or one that names another itself.
func TestSameValueAsNamesTheEarlierCallsPattern(t *testing.T) {
	for _, oneRelation := range []bool{true, false} {
		index, err := newMeasuredProgramIndex(sameValueInput(oneRelation))
		if err != nil {
			t.Fatalf("New (one relation %v): %v", oneRelation, err)
		}
		var first, second RelationPattern
		for _, relation := range index.Relations {
			for _, pattern := range relation.Patterns {
				switch pattern.SourceRef {
				case "first-call":
					first = pattern
				case "second-call":
					second = pattern
				}
			}
		}
		if first.SameValueAs != "" || second.SameValueAs != first.ID || first.ID == "" {
			t.Fatalf("one relation %v: first %q names %q, second %q names %q", oneRelation, first.ID, first.SameValueAs, second.ID, second.SameValueAs)
		}
		encoded, err := Encode(index)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"same_value_as":"`+first.ID+`"`) {
			t.Fatalf("the artifact does not write the fact: %s", encoded)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := json.Marshal(decoded)
		want, _ := json.Marshal(index)
		if string(again) != string(want) {
			t.Fatal("the fact does not read back")
		}
	}

	unknown := sameValueInput(false)
	unknown.Relations[1].Patterns[0].SameValueAs.PatternSourceRef = "no-such-call"
	if _, err := newMeasuredProgramIndex(unknown); err == nil || !strings.Contains(err.Error(), "same value") {
		t.Fatalf("a reference to no pattern was accepted: %v", err)
	}

	later := sameValueInput(false)
	later.Relations[0].Patterns[0].Location.Line = 9
	if _, err := newMeasuredProgramIndex(later); err == nil || !strings.Contains(err.Error(), "same value") {
		t.Fatalf("a call naming a later call was accepted: %v", err)
	}

	otherCallee := sameValueInput(false)
	otherCallee.Relations[1].ToRefs = []string{"target-b"}
	if _, err := newMeasuredProgramIndex(otherCallee); err == nil || !strings.Contains(err.Error(), "same value") {
		t.Fatalf("a call of another callee was accepted: %v", err)
	}

	chained := sameValueInput(true)
	third := chained.Relations[0].Patterns[1]
	third.SourceRef, third.Location = "third-call", &Location{Path: "root.lang", Line: 7, Column: 9}
	third.SameValueAs = &PatternRefInput{RelationSourceRef: "reads", PatternSourceRef: "second-call"}
	chained.Relations[0].Patterns = append(chained.Relations[0].Patterns, third)
	chained.Relations[0].PatternsObserved = 3
	if _, err := newMeasuredProgramIndex(chained); err == nil || !strings.Contains(err.Error(), "same value") {
		t.Fatalf("a call naming a call that names another was accepted: %v", err)
	}
}
