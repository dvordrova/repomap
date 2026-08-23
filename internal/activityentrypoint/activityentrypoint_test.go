package activityentrypoint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

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

func TestCompilationAdvertisesTopologyWithoutSemanticFiltering(t *testing.T) {
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
	if err := validateResponse(response{ActivityRefs: []string{"missing"}}, map[string]struct{}{"a1": {}}); err == nil {
		t.Fatal("unknown ref was accepted")
	}
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
