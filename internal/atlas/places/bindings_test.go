package places

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func bindingObservationTarget(suffix string, reverse bool) TargetInput {
	location := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 4}
	}
	index := programindex.Index{Target: programindex.Target{ID: suffix, Language: "go"}, Objects: []programindex.Object{
		{ID: "factory-" + suffix, Name: "Install", Kind: programindex.ObjectFunction, Location: location("app/install.go", 1)},
		{ID: "callback-" + suffix, Name: "Run", Kind: programindex.ObjectFunction, Location: location("app/worker.go", 10)},
		{ID: "registrar-" + suffix, Name: "Register", Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: "company/runtime", Name: "Register"}},
	}, Relations: []programindex.Relation{{
		FromID: "factory-" + suffix, ToIDs: []string{"callback-" + suffix}, Kind: programindex.RelationPassesCallback,
		SourceArgumentID: "argument-" + suffix, Resolution: programindex.ResolutionAlternatives, Invocation: "callable_binding:field",
		Witnesses: []programindex.Witness{
			{Kind: "callback", Detail: "runtime.Register -> Run", Location: location("app/install.go", 7)},
			{Kind: "callable_receiver_field", Detail: "Name = \"거래\"", Location: location("app/install.go", 5)},
			{Kind: "interface_field_assignment", Detail: "observed receiver assignment", Location: location("app/install.go", 6)},
		},
	}, {
		FromID: "factory-" + suffix, ToIDs: []string{"registrar-" + suffix}, Kind: programindex.RelationInvokesExternal,
		Patterns: []programindex.RelationPattern{{Selector: "Register", Location: location("app/install.go", 7), ResultID: "registered-" + suffix,
			Arguments: []programindex.PatternArgument{
				{ID: "argument-" + suffix, Keyword: "callback", Kind: programindex.PatternDynamic, ObjectIDs: []string{"callback-" + suffix}},
				{Keyword: "name", Kind: programindex.PatternLiteralString, Value: "market-feed"},
			},
		}},
	}}}
	for i, selector := range []string{"start", "join", "is_alive"} {
		index.Relations = append(index.Relations, programindex.Relation{FromID: "factory-" + suffix, Kind: programindex.RelationCalls,
			Patterns: []programindex.RelationPattern{{Selector: selector, ReceiverID: "registered-" + suffix, Location: location("app/install.go", 20+i)}},
		})
	}
	if reverse {
		slices.Reverse(index.Relations[0].Witnesses)
		slices.Reverse(index.Relations)
	}
	return TargetInput{Index: index, Root: "app"}
}

func collectBindingObservations(t *testing.T, targets ...TargetInput) []atlas.SymbolBinding {
	t.Helper()
	native := make([]programindex.Index, len(targets))
	for i, target := range targets {
		native[i] = target.Index
	}
	before, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	b := builder{input: Input{Targets: targets}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	for _, target := range targets {
		b.collectObjects(target)
	}
	got := b.symbolBindings()[atlas.SymbolID("app/install.go", 1, "Install")]
	after, err := json.Marshal(native)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("binding projection mutated native indexes: %v", err)
	}
	return got
}

func TestBindingIdentityIgnoresNativeEvidenceSetOrder(t *testing.T) {
	a, b := bindingObservationTarget("a", false), bindingObservationTarget("b", true)
	// Repeated exact witnesses are the same observation, not another field.
	b.Index.Relations[len(b.Index.Relations)-1].Witnesses = append(b.Index.Relations[len(b.Index.Relations)-1].Witnesses,
		b.Index.Relations[len(b.Index.Relations)-1].Witnesses[0])
	forward := collectBindingObservations(t, a, b)
	backward := collectBindingObservations(t, b, a)
	if len(forward) != 1 || len(backward) != 1 {
		t.Fatalf("target order created duplicate bindings: %d / %d", len(forward), len(backward))
	}
	first, _ := json.Marshal(forward)
	second, _ := json.Marshal(backward)
	if !bytes.Equal(first, second) {
		t.Fatalf("same observation set has unstable identity bytes:\n%s\n%s", first, second)
	}
	if len(forward[0].Evidence) != 6 || len(forward[0].Arguments) != 1 || forward[0].Arguments[0].Value != "market-feed" {
		t.Fatalf("source evidence or literal argument was lost: %+v", forward[0])
	}
	want := map[atlas.EdgeEvidence]bool{
		{Extractor: "callable_receiver_field", Label: "Name = \"거래\"", Path: "app/install.go", LineNo: 5}:                              true,
		{Extractor: "interface_field_assignment", Label: "observed receiver assignment", Path: "app/install.go", LineNo: 6}:            true,
		{Extractor: "callback_registration", Label: "receiving call: Register", Path: "app/install.go", LineNo: 7}:                     true,
		{Extractor: "registration_result_use", Label: "call on the registration result: start", Path: "app/install.go", LineNo: 20}:    true,
		{Extractor: "registration_result_use", Label: "call on the registration result: join", Path: "app/install.go", LineNo: 21}:     true,
		{Extractor: "registration_result_use", Label: "call on the registration result: is_alive", Path: "app/install.go", LineNo: 22}: true,
	}
	for _, evidence := range forward[0].Evidence {
		if !want[evidence] {
			t.Fatalf("invented or altered native observation: %+v", evidence)
		}
		delete(want, evidence)
	}
	if len(want) != 0 {
		t.Fatalf("missing native observations: %+v", want)
	}
}

func TestBindingEvidenceNormalizationKeepsDistinctNativeObservations(t *testing.T) {
	for name, change := range map[string]func(*programindex.Index){
		"field value":         func(index *programindex.Index) { index.Relations[0].Witnesses[1].Detail = "Name = \"other\"" },
		"field path":          func(index *programindex.Index) { index.Relations[0].Witnesses[1].Location.Path = "app/other.go" },
		"field line":          func(index *programindex.Index) { index.Relations[0].Witnesses[1].Location.Line++ },
		"registration anchor": func(index *programindex.Index) { index.Relations[0].Witnesses[0].Location.Line++ },
		"literal argument":    func(index *programindex.Index) { index.Relations[1].Patterns[0].Arguments[1].Value = "another-feed" },
		"callback target": func(index *programindex.Index) {
			index.Objects[1].Name = "RunOther"
			index.Objects[1].Location.Path = "app/other.go"
		},
		"invocation": func(index *programindex.Index) { index.Relations[0].Invocation = "callable_binding:constructor" },
		"resolution": func(index *programindex.Index) { index.Relations[0].Resolution = programindex.ResolutionExact },
	} {
		t.Run(name, func(t *testing.T) {
			a, b := bindingObservationTarget("a", false), bindingObservationTarget("b", false)
			change(&b.Index)
			if got := collectBindingObservations(t, a, b); len(got) != 2 {
				t.Fatalf("distinct %s was collapsed: %+v", name, got)
			}
		})
	}
}
