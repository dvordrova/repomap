package adaptertest

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// ObjectAuthority is the complete retained object authority for one neutral
// pattern field. IDs are ProgramIndex object IDs, not language-adapter refs.
// Empty authority is represented by the zero value.
type ObjectAuthority struct {
	IDs        []string
	Resolution programindex.Resolution
	Observed   int
	Omitted    int
}

// Argument is one complete positional or keyword pattern argument. Tests name
// the fixture-specific meaning; this common contract knows only syntax and
// object authority.
type Argument struct {
	Position int
	Keyword  string
	Kind     programindex.PatternValueKind
	Value    string
	Parts    []programindex.PatternPart
	Objects  ObjectAuthority
}

// Pattern is the complete neutral pattern payload expected on one relation.
type Pattern struct {
	Form            programindex.PatternForm
	Selector        string
	ResultID        string
	RequireResult   bool
	ReceiverID      string
	ReceiverOrigins ObjectAuthority
	Arguments       []Argument
	Observed        int
	Omitted         int
	Path            string
	Line            int
	Column          int
}

// Relation is one directed ProgramIndex relation and its complete coverage
// ledger. Path and Line locate the source-bound fact without depending on an
// adapter-private SourceRef; an empty Path deliberately requires no location.
// Patterns is the complete retained pattern set, not one representative row.
type Relation struct {
	Kind       programindex.RelationKind
	FromID     string
	ToIDs      []string
	Resolution programindex.Resolution
	Invocation string
	Path       string
	Line       int

	TargetsObserved   int
	TargetsOmitted    int
	WitnessesObserved int
	WitnessesOmitted  int
	PatternsObserved  int
	PatternsOmitted   int
	Patterns          []Pattern
	SourceArgumentID  string
}

// Registration is a data-only registration/callback contract. Registration
// describes the call or decorator syntax. Callback, when present, is the
// separately directed passes_callback fact. No protocol or framework name is
// part of this contract.
type Registration struct {
	Name                string
	Registration        Relation
	Callbacks           []Callback
	Continuation        *Relation
	ResultPattern       int
	ContinuationPattern int
	// RequireComplete is an oracle assertion for one fixture family. It does
	// not turn bounded omissions in an ordinary target into a production error:
	// retained siblings remain valid while omitted rows stay a frontier.
	RequireComplete bool
}

// Callback binds one separately directed callback relation to the exact
// argument authority on the registration pattern that produced it.
type Callback struct {
	Relation            Relation
	RegistrationPattern int
	ArgumentPosition    int
	ArgumentKeyword     string
}

// AssertRegistration proves that an already sealed ProgramIndex contains one
// unambiguous, source-bound registration fact and, when expected, its separate
// callback edge. It compares every authority and observed/omitted count instead
// of inferring completeness from retained slice lengths.
func AssertRegistration(t testing.TB, index programindex.Index, want Registration) {
	t.Helper()
	if err := index.Validate(); err != nil {
		t.Fatalf("%s: invalid ProgramIndex: %v", registrationName(want), err)
	}
	objects := make(map[string]struct{}, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = struct{}{}
	}
	assertExpectedObjectIDs(t, registrationName(want)+" registration", objects, want.Registration)
	registration := assertRelation(t, index, registrationName(want)+" registration", want.Registration)
	callbacks := make([]programindex.Relation, 0, len(want.Callbacks))
	for position, callbackWant := range want.Callbacks {
		label := fmt.Sprintf("%s callback %d", registrationName(want), position)
		if callbackWant.RegistrationPattern < 0 ||
			callbackWant.RegistrationPattern >= len(want.Registration.Patterns) {
			t.Fatalf("%s: invalid registration pattern selector %d", label, callbackWant.RegistrationPattern)
		}
		actualPattern := matchingPatternIndex(
			registration.Patterns,
			want.Registration.Patterns[callbackWant.RegistrationPattern],
		)
		if actualPattern < 0 {
			t.Fatalf("%s: registration pattern is missing or ambiguous", label)
		}
		argument := patternArgument(t, label, registration.Patterns[actualPattern],
			callbackWant.ArgumentPosition, callbackWant.ArgumentKeyword)
		relationWant := callbackWant.Relation
		relationWant.SourceArgumentID = argument.ID
		assertExpectedObjectIDs(t, label, objects, relationWant)
		value := assertRelation(t, index, label, relationWant)
		callbacks = append(callbacks, value)
		assertCallbackArgument(t, label, registration.FromID, argument, value)
	}
	if want.RequireComplete {
		assertCompleteRegistration(t, registrationName(want), registration, callbacks)
	}
	if want.Continuation != nil {
		assertResultContinuation(t, index, registrationName(want), objects, registration, want)
	}
}

// AssertRelation proves one arbitrary directed ProgramIndex fact without
// assigning it product semantics. Cumulative-fixture tests use this for the
// calls that must feed a registration handler after dispatch.
func AssertRelation(t testing.TB, index programindex.Index, name string, want Relation) {
	t.Helper()
	if err := index.Validate(); err != nil {
		t.Fatalf("%s: invalid ProgramIndex: %v", name, err)
	}
	objects := make(map[string]struct{}, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = struct{}{}
	}
	assertExpectedObjectIDs(t, name, objects, want)
	assertRelation(t, index, name, want)
}

func assertRelation(t testing.TB, index programindex.Index, label string, want Relation) programindex.Relation {
	t.Helper()
	candidates := make([]programindex.Relation, 0, 1)
	for _, relation := range index.Relations {
		if relation.Kind != want.Kind || relation.FromID != want.FromID ||
			relation.Resolution != want.Resolution || !reflect.DeepEqual(relation.ToIDs, canonicalIDs(want.ToIDs)) ||
			!relationAt(relation, want.Path, want.Line) ||
			want.SourceArgumentID != "" && relation.SourceArgumentID != want.SourceArgumentID {
			continue
		}
		if len(relation.Patterns) != len(want.Patterns) {
			continue
		}
		candidates = append(candidates, relation)
	}
	if len(candidates) != 1 {
		t.Fatalf("%s: matching relations = %d, want exactly one; expectation=%#v", label, len(candidates), want)
	}
	got := candidates[0]
	if got.Invocation != want.Invocation ||
		got.TargetsObserved != want.TargetsObserved || got.TargetsOmitted != want.TargetsOmitted ||
		got.WitnessesObserved != want.WitnessesObserved || got.WitnessesOmitted != want.WitnessesOmitted ||
		got.PatternsObserved != want.PatternsObserved || got.PatternsOmitted != want.PatternsOmitted ||
		want.SourceArgumentID != "" && got.SourceArgumentID != want.SourceArgumentID {
		t.Fatalf("%s: relation coverage/delivery = %#v, want %#v", label, got, want)
	}
	assertPatterns(t, label, got.Patterns, want.Patterns)
	return got
}

func assertPatterns(t testing.TB, label string, got []programindex.RelationPattern, want []Pattern) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: retained patterns = %d, want %d", label, len(got), len(want))
	}
	used := make([]bool, len(got))
	for wantPosition, patternWant := range want {
		match := -1
		for gotPosition, patternGot := range got {
			if used[gotPosition] || patternGot.Form != patternWant.Form || patternGot.Selector != patternWant.Selector ||
				!patternAt(patternGot, patternWant.Path, patternWant.Line, patternWant.Column) {
				continue
			}
			if match >= 0 {
				t.Fatalf("%s: pattern expectation %d is ambiguous", label, wantPosition)
			}
			match = gotPosition
		}
		if match < 0 {
			t.Fatalf("%s: pattern expectation %d has no match: %#v", label, wantPosition, patternWant)
		}
		used[match] = true
		assertPattern(t, fmt.Sprintf("%s pattern %d", label, wantPosition), got[match], patternWant)
	}
}

func assertPattern(t testing.TB, label string, got programindex.RelationPattern, want Pattern) {
	t.Helper()
	wantOrigins := canonicalIDs(want.ReceiverOrigins.IDs)
	if got.Form != want.Form || got.Selector != want.Selector ||
		(want.RequireResult && got.ResultID == "") || (want.ResultID != "" && got.ResultID != want.ResultID) ||
		(!want.RequireResult && want.ResultID == "" && got.ResultID != "") ||
		got.ReceiverID != want.ReceiverID || !patternAt(got, want.Path, want.Line, want.Column) ||
		!reflect.DeepEqual(got.ReceiverOriginIDs, wantOrigins) ||
		got.ReceiverOriginResolution != want.ReceiverOrigins.Resolution ||
		got.ReceiverOriginsObserved != want.ReceiverOrigins.Observed ||
		got.ReceiverOriginsOmitted != want.ReceiverOrigins.Omitted ||
		got.ArgumentsObserved != want.Observed || got.ArgumentsOmitted != want.Omitted ||
		len(got.Arguments) != len(want.Arguments) {
		t.Fatalf("%s: pattern = %#v, want %#v", label, got, want)
	}
	for position, argumentWant := range want.Arguments {
		argumentGot := got.Arguments[position]
		if argumentGot.Position != argumentWant.Position || argumentGot.Keyword != argumentWant.Keyword ||
			argumentGot.Kind != argumentWant.Kind || argumentGot.Value != argumentWant.Value ||
			!reflect.DeepEqual(argumentGot.Parts, nonNilParts(argumentWant.Parts)) ||
			!reflect.DeepEqual(argumentGot.ObjectIDs, canonicalIDs(argumentWant.Objects.IDs)) ||
			argumentGot.Resolution != argumentWant.Objects.Resolution ||
			argumentGot.ObjectsObserved != argumentWant.Objects.Observed ||
			argumentGot.ObjectsOmitted != argumentWant.Objects.Omitted {
			t.Fatalf("%s: argument %d = %#v, want %#v", label, position, argumentGot, argumentWant)
		}
	}
}

func assertExpectedObjectIDs(t testing.TB, label string, objects map[string]struct{}, want Relation) {
	t.Helper()
	ids := append([]string{want.FromID}, want.ToIDs...)
	for _, pattern := range want.Patterns {
		if pattern.ResultID != "" {
			ids = append(ids, pattern.ResultID)
		}
		if pattern.ReceiverID != "" {
			ids = append(ids, pattern.ReceiverID)
		}
		ids = append(ids, pattern.ReceiverOrigins.IDs...)
		for _, argument := range pattern.Arguments {
			ids = append(ids, argument.Objects.IDs...)
		}
	}
	for _, id := range ids {
		if _, ok := objects[id]; !ok {
			t.Fatalf("%s: expectation cites unknown object ID %q", label, id)
		}
	}
}

func assertCallbackArgument(
	t testing.TB,
	label string,
	registrationFromID string,
	argument programindex.PatternArgument,
	callback programindex.Relation,
) {
	t.Helper()
	if registrationFromID != callback.FromID || callback.SourceArgumentID != argument.ID ||
		!reflect.DeepEqual(argument.ObjectIDs, callback.ToIDs) ||
		argument.Resolution != callback.Resolution || argument.ObjectsObserved != callback.TargetsObserved ||
		argument.ObjectsOmitted != callback.TargetsOmitted {
		t.Fatalf("%s: callback relation %#v is not the authority of argument %#v", label, callback, argument)
	}
}

func patternArgument(
	t testing.TB,
	label string,
	pattern programindex.RelationPattern,
	position int,
	keyword string,
) programindex.PatternArgument {
	t.Helper()
	if position == 0 && keyword == "" {
		t.Fatalf("%s: callback argument key is missing", label)
	}
	var matches []programindex.PatternArgument
	for _, argument := range pattern.Arguments {
		if argument.Position == position && argument.Keyword == keyword {
			matches = append(matches, argument)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s: callback argument matches = %d, want exactly one", label, len(matches))
	}
	return matches[0]
}

func assertResultContinuation(
	t testing.TB,
	index programindex.Index,
	label string,
	objects map[string]struct{},
	registration programindex.Relation,
	want Registration,
) {
	t.Helper()
	if want.ResultPattern < 0 || want.ResultPattern >= len(want.Registration.Patterns) ||
		want.ContinuationPattern < 0 || want.ContinuationPattern >= len(want.Continuation.Patterns) {
		t.Fatalf("%s: invalid continuation pattern selector", label)
	}
	resultIndex := matchingPatternIndex(registration.Patterns, want.Registration.Patterns[want.ResultPattern])
	if resultIndex < 0 {
		t.Fatalf("%s: result pattern expectation is missing or ambiguous", label)
	}
	resultID := registration.Patterns[resultIndex].ResultID
	if resultID == "" {
		t.Fatalf("%s: source pattern has no exact call-result object", label)
	}
	if _, ok := objects[resultID]; !ok {
		t.Fatalf("%s: source pattern result %q is not a ProgramIndex object", label, resultID)
	}
	continuationWant := *want.Continuation
	continuationWant.Patterns = append([]Pattern(nil), want.Continuation.Patterns...)
	continuationWant.Patterns[want.ContinuationPattern].ReceiverID = resultID
	assertExpectedObjectIDs(t, label+" continuation", objects, continuationWant)
	continuation := assertRelation(t, index, label+" continuation", continuationWant)
	matched := false
	for _, pattern := range continuation.Patterns {
		if pattern.ReceiverID == resultID {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("%s: result %q is not the next pattern receiver", label, resultID)
	}
}

func matchingPatternIndex(values []programindex.RelationPattern, want Pattern) int {
	match := -1
	for position, value := range values {
		if value.Form != want.Form || value.Selector != want.Selector ||
			!patternAt(value, want.Path, want.Line, want.Column) {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = position
	}
	return match
}

func assertCompleteRegistration(
	t testing.TB,
	label string,
	registration programindex.Relation,
	callbacks []programindex.Relation,
) {
	t.Helper()
	if registration.PatternsOmitted != 0 || registration.PatternsObserved != len(registration.Patterns) ||
		registration.WitnessesOmitted != 0 {
		t.Fatalf("%s: registration syntax/evidence is incomplete: %#v", label, registration)
	}
	for _, pattern := range registration.Patterns {
		if pattern.ArgumentsOmitted != 0 || pattern.ArgumentsObserved != len(pattern.Arguments) ||
			pattern.ReceiverOriginsOmitted != 0 {
			t.Fatalf("%s: retained pattern is incomplete: %#v", label, pattern)
		}
		for _, argument := range pattern.Arguments {
			if argument.ObjectsOmitted != 0 {
				t.Fatalf("%s: retained argument object authority is incomplete: %#v", label, argument)
			}
		}
	}
	for _, callback := range callbacks {
		if callback.TargetsOmitted != 0 || callback.WitnessesOmitted != 0 || callback.PatternsOmitted != 0 {
			t.Fatalf("%s: callback relation is incomplete: %#v", label, callback)
		}
	}
}

func relationAt(value programindex.Relation, path string, line int) bool {
	if path == "" {
		return value.Location == nil
	}
	return value.Location != nil && value.Location.Path == path && value.Location.Line == line
}

func patternAt(value programindex.RelationPattern, path string, line, column int) bool {
	if path == "" {
		return value.Location == nil
	}
	return value.Location != nil && value.Location.Path == path && value.Location.Line == line &&
		(column == 0 || value.Location.Column == column)
}

func canonicalIDs(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if result == nil {
		return []string{}
	}
	return result
}

func nonNilParts(values []programindex.PatternPart) []programindex.PatternPart {
	if values == nil {
		return []programindex.PatternPart{}
	}
	return values
}

func registrationName(value Registration) string {
	if value.Name != "" {
		return value.Name
	}
	return fmt.Sprintf("%s registration", value.Registration.Kind)
}
