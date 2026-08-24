package coremap

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestCoreMapRunRestoresModelOwnedRefinedGroups(t *testing.T) {
	_, repository := coreMapTestCorpus(t, map[string][]byte{
		"pkg/main.py": []byte("def run():\n    persist()\n\ndef persist():\n    pass\n"),
	})
	fileRef, ok := repository.ID("pkg/main.py")
	if !ok {
		t.Fatal("fixture file is absent from corpus")
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "pkg.main", Selector: "python:pkg.main",
			Sources:       []programindex.TargetSource{{FileRef: string(fileRef), Path: "pkg/main.py"}},
			AnchorFileRef: string(fileRef),
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "run", Kind: programindex.SeedCallable,
				Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1},
			}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "pkg.main", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1}},
			{SourceRef: "run", Kind: programindex.ObjectFunction, Name: "run", Visibility: programindex.VisibilityPublic, OwnerRef: "module", ContainerRef: "module", Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1}},
			{SourceRef: "persist", Kind: programindex.ObjectFunction, Name: "persist", Visibility: programindex.VisibilityInternal, OwnerRef: "module", ContainerRef: "module", Location: &programindex.Location{Path: "pkg/main.py", Line: 4, Column: 1}},
		},
		Relations: []programindex.RelationInput{{
			SourceRef: "run-persist", Kind: programindex.RelationCalls, FromRef: "run", ToRefs: []string{"persist"},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1,
			Location:  &programindex.Location{Path: "pkg/main.py", Line: 2, Column: 5},
			Witnesses: []programindex.Witness{{Kind: "callsite", Detail: "persist"}}, WitnessesObserved: 1,
		}},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 3, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	compilation, err := CompileProgram("sample", repository, index, readmetargetscout.Result{})
	if err != nil {
		t.Fatal(err)
	}
	provider := &groupingSequenceProvider{responses: [][]byte{
		[]byte(`{"blocks":[{"name":"Request execution","purpose":"Accepts and coordinates work.","file_refs":[],"symbol_refs":["s1"]},{"name":"Persistence","purpose":"Persists accepted work.","file_refs":[],"symbol_refs":["s2"]}]}`),
		[]byte(`{"groups":[{"name":"Runtime","purpose":"Coordinates accepted work.","block_refs":["b1"]},{"name":"Storage","purpose":"Persists accepted work.","block_refs":["b2"]}]}`),
	}}
	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, compilation)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Refined) != 2 || len(result.RefinedGroups) != 2 || provider.completeCalls != 2 ||
		result.Coverage.RefinedGroupCalls != 1 || result.RequestSizes.Grouping.Calls != 1 ||
		result.RefinedGroups[0].BlockIDs[0] != result.Refined[0].ID {
		t.Fatalf("grouped CoreMap = %#v / calls=%d", result, provider.completeCalls)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateAgainst(compilation); err != nil {
		t.Fatal(err)
	}
}

func TestGroupingResponseRequiresCompleteDisjointPartition(t *testing.T) {
	allowed := map[string]Block{
		"b1": {ID: "core-1"},
		"b2": {ID: "core-2"},
		"b3": {ID: "core-3"},
	}
	valid := []byte(`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b2"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b3"]}]}`)
	response, err := decodeGroupingResponse(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGroupingResponse(response, allowed); err != nil {
		t.Fatal(err)
	}
	empty, err := decodeGroupingResponse([]byte(`{"groups":[]}`))
	if err != nil || validateGroupingResponse(empty, allowed) != nil {
		t.Fatalf("explicit empty grouping = %#v / %v", empty, err)
	}

	cases := []string{
		`{"groups":null}`,
		`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b3"]}]}`,
		`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b2"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b2","b3"]}]}`,
		`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b2"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b4"]}]}`,
		`{"groups":[{"name":"Everything","purpose":"Contains all work.","block_refs":["b1","b2","b3"]}]}`,
	}
	for _, raw := range cases {
		value, decodeErr := decodeGroupingResponse([]byte(raw))
		if decodeErr == nil {
			decodeErr = validateGroupingResponse(value, allowed)
		}
		if decodeErr == nil {
			t.Fatalf("invalid grouping accepted: %s", raw)
		}
	}
}

func TestGroupingRequestUsesBoundedBlockTopologyWithoutProgramIDs(t *testing.T) {
	locationA := surfacediscovery.Location{Path: "pkg/a.go", Line: 1, Column: 1}
	locationB := surfacediscovery.Location{Path: "pkg/b.go", Line: 2, Column: 1}
	locationC := surfacediscovery.Location{Path: "pkg/c.go", Line: 3, Column: 1}
	compilation := Compilation{
		repository: "sample",
		baselineRequest: baselineRequest{Target: targetRequest{
			Language: "go", Kind: "executable", Name: "sample", Selector: "sample",
		}},
		programCoverage: programindex.Coverage{},
		symbols: map[string]symbolAuthority{
			"s1": groupingTestSymbol("node-a", "program-secret-a", "A", locationA),
			"s2": groupingTestSymbol("node-b", "program-secret-b", "B", locationB),
			"s3": groupingTestSymbol("node-c", "program-secret-c", "C", locationC),
		},
		groupingEdges: []groupingEdgeAuthority{
			{FromObjectID: "program-secret-a", ToObjectID: "program-secret-b"},
			{FromObjectID: "program-secret-b", ToObjectID: "program-secret-c"},
		},
		targetSeedRows: []targetSeedRequest{},
	}
	blocks := []Block{
		groupingTestBlock("core-a", "Accept", compilation.symbols["s1"].fact),
		groupingTestBlock("core-b", "Process", compilation.symbols["s2"].fact),
		groupingTestBlock("core-c", "Persist", compilation.symbols["s3"].fact),
	}
	request, allowed, err := buildGroupingRequest(compilation, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 3 || len(request.Relations) != 3 {
		t.Fatalf("grouping request = %#v", request)
	}
	var aToC *int
	for _, relation := range request.Relations {
		if relation.LeftRef == "b1" && relation.RightRef == "b3" {
			aToC = relation.LeftReachesRightMinHops
		}
	}
	if aToC == nil || *aToC != 2 {
		t.Fatalf("b1 -> b3 minimum hops = %#v", aToC)
	}
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"program-secret-", "node-a", "node-b", "node-c", "pkg/a.go"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("grouping request leaked %q: %s", forbidden, wire)
		}
	}
}

func TestStableGroupIDDependsOnlyOnTargetAndMembership(t *testing.T) {
	left := stableGroupID("target", []string{"core-b", "core-a"})
	right := stableGroupID("target", []string{"core-a", "core-b"})
	changed := stableGroupID("target", []string{"core-a", "core-c"})
	if left != right || left == changed {
		t.Fatalf("stable group IDs = %q / %q / %q", left, right, changed)
	}
}

func groupingTestSymbol(nodeID, programID, name string, location surfacediscovery.Location) symbolAuthority {
	fact := SymbolFact{
		NodeID: nodeID, Kind: programindex.ObjectFunction,
		Symbol:  surfacediscovery.Symbol{ID: nodeID, Name: name, Package: "sample", Location: location},
		Package: "sample", Exported: true, Declaration: location,
	}
	return symbolAuthority{fact: fact, programObjectID: programID}
}

func groupingTestBlock(id, name string, symbol SymbolFact) Block {
	return Block{
		ID: id, Name: name, Purpose: name + " responsibility.",
		Files: []FileFact{}, Symbols: []SymbolFact{symbol}, Children: []Block{},
	}
}

type groupingSequenceProvider struct {
	responses     [][]byte
	completeCalls int
}

func (provider *groupingSequenceProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`)
}

func (provider *groupingSequenceProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.System + "\n" + prompt.User))
}

func (provider *groupingSequenceProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	if provider.completeCalls >= len(provider.responses) {
		return llm.Completion{}, context.Canceled
	}
	response := provider.responses[provider.completeCalls]
	provider.completeCalls++
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{
			InputTokens: 10, OutputTokens: 10, ProviderResponseBytes: len(response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}
