package activityentrypoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCompileStateBindsLargeBatchByDigestWithoutObjectInventory(t *testing.T) {
	canonicalID := "object-" + strings.Repeat("a", 64)
	wire := []byte(`{"candidates":["` + strings.Repeat(canonicalID+`","`, MaxCandidatesPerBatch) + `end"]}`)
	state, err := compileState(strings.Repeat("b", 64), 1, 2, wire)
	if err != nil {
		t.Fatalf("compileState: %v", err)
	}
	if len(state) > 1024 || bytes.Contains(state, []byte(canonicalID)) || bytes.Contains(state, []byte("object_ids")) {
		t.Fatalf("cube state retained the request inventory: %d bytes: %s", len(state), state)
	}
	changed, err := compileState(strings.Repeat("b", 64), 1, 2, append(wire, ' '))
	if err != nil {
		t.Fatalf("compile changed state: %v", err)
	}
	if bytes.Equal(state, changed) {
		t.Fatal("request mutation did not change cube state identity")
	}
}

type fixedProvider struct {
	response []byte
	prompts  []llm.Prompt
}

func (provider *fixedProvider) State() []byte {
	return []byte(`{"provider":"activity-entrypoint-test"}`)
}

func (provider *fixedProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	provider.prompts = append(provider.prompts, prompt)
	exact, err := json.Marshal(prompt)
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(exact)
}

func (provider *fixedProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{
		Response: provider.response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}

func TestRunRestoresExactActivityObjectAndSealsArtifact(t *testing.T) {
	index := activityTestIndex(t)
	provider := &fixedProvider{response: []byte(`{"activity_refs":["a1"]}`)}
	result, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, index)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Objects) != 1 || result.Objects[0].Name != "Start" {
		t.Fatalf("selected objects = %#v", result.Objects)
	}
	if result.Coverage.CallablesIndexed != 3 || result.Coverage.CallablesWithoutLocation != 1 ||
		result.Coverage.SeededModulesIndexed != 0 ||
		result.Coverage.SeededModulesWithoutLocation != 0 ||
		result.Coverage.CandidatesAdvertised != 2 || result.Coverage.CandidatesOmitted != 0 ||
		result.Coverage.Selected != 1 {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	if err := result.ValidateAgainst(index); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded, index)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.SHA256 != result.SHA256 {
		t.Fatalf("decoded sha = %q, want %q", decoded.SHA256, result.SHA256)
	}
	if len(provider.prompts) != 1 || strings.Contains(provider.prompts[0].User, index.Objects[0].ID) ||
		strings.Contains(provider.prompts[0].User, index.SHA256) {
		t.Fatalf("provider prompt leaked canonical identity: %s", provider.prompts[0].User)
	}
}

func TestCompilationAdvertisesSeededModuleLaunchAnchor(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "tools/export.py", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64), SourceSHA256: strings.Repeat("d", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "command", Name: "export", Selector: "script:tools/export.py",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "tools/export.py"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "module", Kind: programindex.SeedMainGuard, Location: location(12),
			}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "tools.export", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "helper", Kind: programindex.ObjectFunction, Name: "load_rows", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(3)},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 2},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	compiled, err := compile(index)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(compiled.candidates) != 2 || compiled.candidates[0].object.Kind != programindex.ObjectModule ||
		compiled.candidates[0].row.SeedKinds[0] != programindex.SeedMainGuard ||
		compiled.coverage.SeededModulesIndexed != 1 || compiled.coverage.CandidatesAdvertised != 2 {
		t.Fatalf("seeded module activity catalog = %#v / %#v", compiled.candidates, compiled.coverage)
	}
}

func TestCompilationAdvertisesEligibleTopologyWithoutSemanticPromotion(t *testing.T) {
	compiled, err := compile(activityTestIndex(t))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(compiled.candidates) != 2 || len(compiled.batches) != 1 {
		t.Fatalf("candidate partition = %d/%d", len(compiled.candidates), len(compiled.batches))
	}
	if compiled.candidates[0].row.Name != "Start" || compiled.candidates[0].row.Topology.OutgoingCalls != 1 {
		t.Fatalf("start row = %#v", compiled.candidates[0].row)
	}
	if compiled.candidates[1].row.Name != "helper" || compiled.candidates[1].row.Topology.IncomingCalls != 1 {
		t.Fatalf("helper row = %#v", compiled.candidates[1].row)
	}
}

func TestRunFiltersUnknownAndDuplicateActivityRefs(t *testing.T) {
	index := activityTestIndex(t)
	refs := []string{"a1", "a1"}
	for ordinal := 0; ordinal < 8; ordinal++ {
		refs = append(refs, fmt.Sprintf("unknown-%04d", ordinal))
	}
	raw, err := json.Marshal(response{ActivityRefs: refs})
	if err != nil {
		t.Fatal(err)
	}
	mixedProvider := &fixedProvider{response: raw}
	result, err := Run(
		context.Background(), llm.Executor{Enabled: false},
		mixedProvider, index,
	)
	if err != nil {
		t.Fatalf("mixed known/unknown response: %v", err)
	}
	if len(result.Objects) != 1 || result.Objects[0].Name != "Start" || result.Coverage.Selected != 1 {
		t.Fatalf("filtered mixed selection = %#v", result)
	}
	if len(mixedProvider.prompts) != 1 {
		t.Fatalf("mixed response provider requests = %d, want 1 without retry", len(mixedProvider.prompts))
	}

	unknownProvider := &fixedProvider{
		response: []byte(`{"activity_refs":["unknown","foreign","unknown"]}`),
	}
	result, err = Run(
		context.Background(), llm.Executor{Enabled: false},
		unknownProvider, index,
	)
	if err != nil {
		t.Fatalf("all-unknown response: %v", err)
	}
	if len(result.Objects) != 0 || result.Coverage.Selected != 0 {
		t.Fatalf("all-unknown selection = %#v", result)
	}
	if len(unknownProvider.prompts) != 1 {
		t.Fatalf("all-unknown provider requests = %d, want 1 without retry", len(unknownProvider.prompts))
	}
}

func TestBatchRequestsExposeSeedCandidateRefOnlyInOwningBatch(t *testing.T) {
	compiled, err := compile(activityBatchedSeedTestIndex(t))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(compiled.candidates) != MaxCandidatesPerBatch+1 || len(compiled.batches) != 2 {
		t.Fatalf("candidate partition = %d candidates / %d batches", len(compiled.candidates), len(compiled.batches))
	}
	if len(compiled.batches[0]) != MaxCandidatesPerBatch || len(compiled.batches[1]) != 1 {
		t.Fatalf("batch sizes = %d / %d", len(compiled.batches[0]), len(compiled.batches[1]))
	}

	first := requestForBatch(compiled, 0, compiled.batches[0])
	second := requestForBatch(compiled, 1, compiled.batches[1])
	if len(first.Seeds) != 1 || len(second.Seeds) != 1 {
		t.Fatalf("complete seed context was not repeated: first=%#v second=%#v", first.Seeds, second.Seeds)
	}
	wantRef := compiled.candidates[len(compiled.candidates)-1].ref
	if first.Seeds[0].CandidateRef != "" || second.Seeds[0].CandidateRef != wantRef {
		t.Fatalf("batch-local seed refs: first=%#v second=%#v", first.Seeds[0], second.Seeds[0])
	}
	firstSeed, secondSeed := first.Seeds[0], second.Seeds[0]
	firstSeed.CandidateRef = secondSeed.CandidateRef
	if firstSeed != secondSeed {
		t.Fatalf("non-selectable seed context changed across batches: first=%#v second=%#v", first.Seeds[0], second.Seeds[0])
	}
	firstWire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondWire, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(firstWire, []byte(`"candidate_ref"`)) ||
		!bytes.Contains(secondWire, []byte(`"candidate_ref":"`+wantRef+`"`)) {
		t.Fatalf("batch-local seed wire authority: first_has_ref=%t second_has_%s=%t",
			bytes.Contains(firstWire, []byte(`"candidate_ref"`)), wantRef,
			bytes.Contains(secondWire, []byte(`"candidate_ref":"`+wantRef+`"`)))
	}

	advertised := make(map[string]int, len(compiled.candidates))
	for _, payload := range []request{first, second} {
		if payload.CandidatesObserved != len(compiled.candidates) ||
			payload.CandidatesAdvertised != len(compiled.candidates) || payload.CandidatesOmitted != 0 {
			t.Fatalf("batch coverage drifted: %#v", payload)
		}
		for _, candidate := range payload.Candidates {
			advertised[candidate.Ref]++
		}
	}
	for _, candidate := range compiled.candidates {
		if advertised[candidate.ref] != 1 {
			t.Errorf("candidate %q advertised %d times", candidate.ref, advertised[candidate.ref])
		}
	}
}

func activityBatchedSeedTestIndex(t *testing.T) programindex.Index {
	t.Helper()
	const candidateCount = MaxCandidatesPerBatch + 1
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "cmd/app/main.go", Line: line, Column: 1}
	}
	objects := make([]programindex.ObjectInput, candidateCount)
	for position := range objects {
		objects[position] = programindex.ObjectInput{
			SourceRef:  fmt.Sprintf("callable-%04d", position+1),
			Kind:       programindex.ObjectFunction,
			Name:       fmt.Sprintf("callable%04d", position+1),
			Visibility: programindex.VisibilityInternal,
			Location:   location(position + 1),
		}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("7", 64), SourceSHA256: strings.Repeat("8", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: "app", Selector: "./cmd/app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "cmd/app/main.go"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: objects[candidateCount-1].SourceRef,
				Kind:      programindex.SeedCallable,
				Location:  location(candidateCount),
			}},
		},
		Objects: objects,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: candidateCount,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func TestCompilationEligibilityPreservesDynamicJointsAndSeedHandoffs(t *testing.T) {
	compiled, err := compile(activityEligibilityTestIndex(t, "executable"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got := make(map[string]bool, len(compiled.candidates))
	for _, value := range compiled.candidates {
		got[value.object.Name] = true
	}
	for _, want := range []string{
		"seed", "seedTarget", "rootCaller", "alternativeCaller", "alternativeTarget",
		"callbackTarget", "decoratorSource", "decoratedTarget", "implementationSource", "implementationTarget",
	} {
		if !got[want] {
			t.Errorf("eligible catalog omitted %q: %#v", want, got)
		}
	}
	if got["ordinaryExactCallee"] {
		t.Fatalf("ordinary internal exact callee was advertised: %#v", got)
	}
	if compiled.coverage.CallablesIndexed != 11 || compiled.coverage.CallablesIneligible != 1 ||
		compiled.coverage.CandidatesAdvertised != 10 || compiled.coverage.CandidatesOmitted != 0 {
		t.Fatalf("eligibility coverage = %#v", compiled.coverage)
	}
}

func TestCompilationEligibilityRetainsPublicLibraryOperation(t *testing.T) {
	compiled, err := compile(activityEligibilityTestIndex(t, "library"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, value := range compiled.candidates {
		if value.object.Name == "ordinaryExactCallee" {
			if value.object.Visibility != programindex.VisibilityPublic {
				t.Fatalf("library operation visibility = %q", value.object.Visibility)
			}
			return
		}
	}
	t.Fatal("public library operation with an exact incoming call was omitted")
}

func activityEligibilityTestIndex(t *testing.T, targetKind string) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/program.go", Line: line, Column: 1}
	}
	object := func(ref, name string, line int) programindex.ObjectInput {
		visibility := programindex.VisibilityInternal
		if name == "ordinaryExactCallee" {
			visibility = programindex.VisibilityPublic
		}
		return programindex.ObjectInput{
			SourceRef: ref, Kind: programindex.ObjectFunction, Name: name,
			Visibility: visibility, Location: location(line),
		}
	}
	relation := func(ref string, kind programindex.RelationKind, from, to string, resolution programindex.Resolution, line int) programindex.RelationInput {
		return programindex.RelationInput{
			SourceRef: ref, Kind: kind, FromRef: from, ToRefs: []string{to},
			Resolution: resolution, Location: location(line), TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: "eligibility_test", Location: location(line)}},
			WitnessesObserved: 1,
		}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("e", 64), SourceSHA256: strings.Repeat("f", 64),
		Target: programindex.TargetInput{
			Language: "generic", Kind: targetKind, Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "app/program.go"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "seed", Kind: programindex.SeedCallable, Location: location(1),
			}},
		},
		Objects: []programindex.ObjectInput{
			object("seed", "seed", 1),
			object("seed-target", "seedTarget", 2),
			object("root-caller", "rootCaller", 3),
			object("ordinary", "ordinaryExactCallee", 4),
			object("alternative-caller", "alternativeCaller", 5),
			object("alternative-target", "alternativeTarget", 6),
			object("callback-target", "callbackTarget", 7),
			object("decorator-source", "decoratorSource", 8),
			object("decorated-target", "decoratedTarget", 9),
			object("implementation-source", "implementationSource", 10),
			object("implementation-target", "implementationTarget", 11),
		},
		Relations: []programindex.RelationInput{
			relation("seed-call", programindex.RelationCalls, "seed", "seed-target", programindex.ResolutionExact, 20),
			relation("ordinary-call", programindex.RelationCalls, "root-caller", "ordinary", programindex.ResolutionExact, 21),
			relation("callback-call", programindex.RelationCalls, "root-caller", "callback-target", programindex.ResolutionExact, 22),
			relation("decorated-call", programindex.RelationCalls, "root-caller", "decorated-target", programindex.ResolutionExact, 23),
			relation("implementation-call", programindex.RelationCalls, "root-caller", "implementation-target", programindex.ResolutionExact, 24),
			relation("alternative-call", programindex.RelationCalls, "alternative-caller", "alternative-target", programindex.ResolutionAlternatives, 25),
			relation("callback-joint", programindex.RelationPassesCallback, "seed", "callback-target", programindex.ResolutionExact, 26),
			relation("decorator-joint", programindex.RelationDecorates, "decorator-source", "decorated-target", programindex.ResolutionExact, 27),
			relation("implementation-joint", programindex.RelationImplements, "implementation-source", "implementation-target", programindex.ResolutionExact, 28),
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 11, RelationsObserved: 9},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func activityTestIndex(t *testing.T) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "cmd/app/main.go", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "command", Name: "app", Selector: "./cmd/app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "cmd/app/main.go"}},
			AnchorFileRef: "f1",
			Seeds:         []programindex.TargetSeedInput{{ObjectRef: "start", Kind: programindex.SeedCallable, Location: location(2)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectPackage, Name: "main", Visibility: programindex.VisibilityInternal, Location: location(1)},
			{SourceRef: "start", Kind: programindex.ObjectFunction, Name: "Start", Visibility: programindex.VisibilityPublic, Signature: "func Start()", ContainerRef: "module", Location: location(2)},
			{SourceRef: "helper", Kind: programindex.ObjectFunction, Name: "helper", Visibility: programindex.VisibilityInternal, Signature: "func helper()", ContainerRef: "module", Location: location(6)},
			{SourceRef: "generated", Kind: programindex.ObjectFunction, Name: "generated", Visibility: programindex.VisibilityUnknown},
		},
		Relations: []programindex.RelationInput{{
			SourceRef: "start-helper", Kind: programindex.RelationCalls, FromRef: "start", ToRefs: []string{"helper"},
			Resolution: programindex.ResolutionExact, Location: location(3), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "test_call", Location: location(3)}}, WitnessesObserved: 1,
		}},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}
