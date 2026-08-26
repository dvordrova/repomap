package coremap

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
		result.RefinedGroups[0].BlockIDs[0] != result.Refined[0].ID ||
		result.RefinedGroups[0].Authority != GroupAuthorityModel ||
		result.RefinedGroups[1].Authority != GroupAuthorityModel {
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

func TestCoreMapResourceEnvelope(t *testing.T) {
	if MaxOutputTokens != 128_000 || MaxResponseBytes != 2<<20 {
		t.Fatalf("CoreMap resource envelope = %d tokens / %d bytes", MaxOutputTokens, MaxResponseBytes)
	}
}

func TestGroupingResponseNormalizesClosedMembershipSets(t *testing.T) {
	allowed := map[string]Block{
		"b1": {ID: "core-1"},
		"b2": {ID: "core-2"},
		"b3": {ID: "core-3"},
	}
	valid := []byte(`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b1","b2"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b2","b3"]}]}`)
	response, err := decodeGroupingResponse(valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Groups[0].BlockRefs) != 2 {
		t.Fatalf("same-group set membership was not canonicalized: %#v", response.Groups[0].BlockRefs)
	}
	if len(response.Groups[1].BlockRefs) != 2 || response.Groups[1].BlockRefs[0] != "b2" {
		t.Fatalf("cross-group overlap was not retained: %#v", response.Groups)
	}
	if err := validateGroupingResponse(response, allowed); err != nil {
		t.Fatal(err)
	}
	oneFull, err := decodeGroupingResponse([]byte(`{"groups":[{"name":"Everything","purpose":"Provides one supported survey area.","block_refs":["b1","b2","b3"]}]}`))
	if err != nil || validateGroupingResponse(oneFull, allowed) != nil {
		t.Fatalf("one complete model group = %#v / %v", oneFull, err)
	}
	mixed, err := decodeGroupingResponse([]byte(`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b999","b2"]},{"name":"Storage","purpose":"Persists accepted state.","block_refs":["b3"]},{"name":"Invented","purpose":"Has no local authority.","block_refs":["b998"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mixed = normalizeGroupingResponseRefs(mixed, allowed)
	if err := validateGroupingResponse(mixed, allowed); err != nil {
		t.Fatalf("mixed known/unknown grouping = %#v / %v", mixed, err)
	}
	if len(mixed.Groups) != 2 || len(mixed.Groups[0].BlockRefs) != 2 {
		t.Fatalf("unknown group memberships were not discarded: %#v", mixed)
	}
	exactDuplicates, err := decodeGroupingResponse([]byte(`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b2","b1"]},{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1","b2"]},{"name":"Execution lens","purpose":"Explains the same members from another supported perspective.","block_refs":["b1","b2"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	exactDuplicates = normalizeGroupingResponseRefs(exactDuplicates, allowed)
	if len(exactDuplicates.Groups) != 2 ||
		exactDuplicates.Groups[0].Name != "Runtime" || exactDuplicates.Groups[1].Name != "Execution lens" {
		t.Fatalf("group set normalization = %#v", exactDuplicates.Groups)
	}
	if err := validateGroupingResponse(exactDuplicates, allowed); err != nil {
		t.Fatal(err)
	}
	unknownOnly, err := decodeGroupingResponse([]byte(`{"groups":[{"name":"Invented A","purpose":"Has no local authority.","block_refs":["b998"]},{"name":"Invented B","purpose":"Also has no authority.","block_refs":["b999"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	unknownOnly = normalizeGroupingResponseRefs(unknownOnly, allowed)
	if len(unknownOnly.Groups) != 0 || validateGroupingResponse(unknownOnly, allowed) != nil {
		t.Fatalf("unknown-only grouping did not reduce to the legitimate empty set: %#v", unknownOnly)
	}
	empty, err := decodeGroupingResponse([]byte(`{"groups":[]}`))
	if err != nil || validateGroupingResponse(empty, allowed) != nil {
		t.Fatalf("explicit empty grouping = %#v / %v", empty, err)
	}

	cases := []string{
		`{"groups":null}`,
		`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":[]}]}`,
		`{"groups":[{"name":"Runtime","purpose":"Runs accepted work.","block_refs":["b1"]},{"name":"Runtime","purpose":"Persists accepted state.","block_refs":["b3"]}]}`,
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

func TestGroupingRestoresOverlappingMemberships(t *testing.T) {
	blocks := []Block{{ID: "core-1"}, {ID: "core-2"}}
	allowed := map[string]Block{"b1": blocks[0], "b2": blocks[1]}
	response := groupingResponse{Groups: []groupProposal{
		{Name: "Runtime", Purpose: "Coordinates accepted work.", BlockRefs: []string{"b1", "b2"}},
		{Name: "Storage", Purpose: "Persists accepted state.", BlockRefs: []string{"b2"}},
	}}
	groups, err := restoreRefinedGroups(response, allowed, blocks, "target")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRefinedGroups(groups, blocks, "target"); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || !reflect.DeepEqual(groups[0].BlockIDs, []string{"core-1", "core-2"}) ||
		!reflect.DeepEqual(groups[1].BlockIDs, []string{"core-2"}) {
		t.Fatalf("restored overlapping groups = %#v", groups)
	}
}

func TestGroupingAllowsOneCompleteAndAccountsOnePartialGroup(t *testing.T) {
	blocks := []Block{{ID: "core-1"}, {ID: "core-2"}, {ID: "core-3"}}
	allowed := map[string]Block{"b1": blocks[0], "b2": blocks[1], "b3": blocks[2]}
	complete := groupingResponse{Groups: []groupProposal{{
		Name: "Runtime", Purpose: "Coordinates all supported work.", BlockRefs: []string{"b1", "b2", "b3"},
	}}}
	groups, err := restoreRefinedGroups(complete, allowed, blocks, "target")
	if err != nil || len(groups) != 1 || groups[0].Authority != GroupAuthorityModel ||
		validateRefinedGroups(groups, blocks, "target") != nil {
		t.Fatalf("one complete group = %#v / %v", groups, err)
	}
	partial := groupingResponse{Groups: []groupProposal{{
		Name: "Runtime", Purpose: "Coordinates supported runtime work.", BlockRefs: []string{"b1", "b3"},
	}}}
	groups, err = restoreRefinedGroups(partial, allowed, blocks, "target")
	if err != nil || len(groups) != 2 || groups[1].Authority != GroupAuthorityLocalUnassigned ||
		!reflect.DeepEqual(groups[1].BlockIDs, []string{"core-2"}) ||
		validateRefinedGroups(groups, blocks, "target") != nil {
		t.Fatalf("one partial group = %#v / %v", groups, err)
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

func TestGroupingRequestSparseRelationsRoundTripCompleteLargeTopology(t *testing.T) {
	const blockCount = 160
	compilation := Compilation{
		repository: "large-sample",
		baselineRequest: baselineRequest{Target: targetRequest{
			Language: "go", Kind: "executable", Name: "large-sample", Selector: "large-sample",
		}},
		programCoverage: programindex.Coverage{},
		symbols:         make(map[string]symbolAuthority, blockCount),
		groupingEdges: []groupingEdgeAuthority{
			{FromObjectID: "program-001", ToObjectID: "program-002"},
			{FromObjectID: "program-006", ToObjectID: "program-005"},
		},
		targetSeedRows: []targetSeedRequest{},
	}
	blocks := make([]Block, blockCount)
	for position := 0; position < blockCount; position++ {
		ordinal := position + 1
		ref := fmt.Sprintf("s%d", ordinal)
		nodeID := fmt.Sprintf("node-%03d", ordinal)
		programID := fmt.Sprintf("program-%03d", ordinal)
		authority := groupingTestSymbol(nodeID, programID, fmt.Sprintf("Operation%03d", ordinal), surfacediscovery.Location{
			Path: fmt.Sprintf("pkg/%03d.go", ordinal), Line: ordinal, Column: 1,
		})
		compilation.symbols[ref] = authority
		blocks[position] = groupingTestBlock(fmt.Sprintf("core-%03d", ordinal), fmt.Sprintf("Responsibility %03d", ordinal), authority.fact)
	}
	// One exact representative appears in two responsibilities. This is positive
	// shared-member evidence independent of the two directional edge examples.
	blocks[3].Symbols = append(blocks[3].Symbols, compilation.symbols["s3"].fact)

	request, allowed, err := buildGroupingRequest(compilation, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != blockCount || request.RelationEncoding != groupingRelationEncoding {
		t.Fatalf("large grouping authority = %d / %q", len(allowed), request.RelationEncoding)
	}

	dense := denseGroupingRelationsForTest(compilation, blocks)
	if len(dense) != blockCount*(blockCount-1)/2 {
		t.Fatalf("complete relation domain = %d", len(dense))
	}
	sparseByPair := make(map[string]groupingRelationRequest, len(request.Relations))
	for _, relation := range request.Relations {
		if !groupingRelationHasEvidence(relation) {
			t.Fatalf("sparse request retained an empty relation: %#v", relation)
		}
		sparseByPair[relation.LeftRef+"\x00"+relation.RightRef] = relation
	}
	positive := 0
	for _, want := range dense {
		key := want.LeftRef + "\x00" + want.RightRef
		got, retained := sparseByPair[key]
		if groupingRelationHasEvidence(want) {
			positive++
			if !retained || !reflect.DeepEqual(got, want) {
				t.Fatalf("positive relation %q was not retained exactly: %#v / %#v", key, got, want)
			}
			continue
		}
		if retained {
			t.Fatalf("empty relation %q was retained: %#v", key, got)
		}
		// Absence expands to the exact former dense defaults.
		if want.SharedRepresentatives != 0 || want.LeftReachesRightMinHops != nil || want.RightReachesLeftMinHops != nil {
			t.Fatalf("absent relation %q does not have the lossless sparse default: %#v", key, want)
		}
	}
	if len(request.Relations) != positive || positive != 3 {
		t.Fatalf("positive sparse relations = %d / complete positives %d", len(request.Relations), positive)
	}

	sparseWire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	historicDense := request
	historicDense.Relations = dense
	denseWire, err := json.Marshal(historicDense)
	if err != nil {
		t.Fatal(err)
	}
	if len(denseWire)+len(groupPrompt) <= maxRefinedPayloadBytes {
		t.Fatalf("regression does not cross the former dense bound: %d + prompt", len(denseWire))
	}
	if len(sparseWire)+len(groupPrompt) > maxRefinedPayloadBytes {
		t.Fatalf("lossless sparse request exceeds the bound: %d + prompt", len(sparseWire))
	}
}

func denseGroupingRelationsForTest(compilation Compilation, blocks []Block) []groupingRelationRequest {
	symbolRefByNodeID := make(map[string]string, len(compilation.symbols))
	for ref, authority := range compilation.symbols {
		symbolRefByNodeID[authority.fact.NodeID] = ref
	}
	programObjectsByBlock := make(map[string]map[string]struct{}, len(blocks))
	for position, block := range blocks {
		blockRef := fmt.Sprintf("b%d", position+1)
		programObjectsByBlock[blockRef] = make(map[string]struct{}, len(block.Symbols))
		for _, symbol := range block.Symbols {
			authority := compilation.symbols[symbolRefByNodeID[symbol.NodeID]]
			programObjectsByBlock[blockRef][authority.programObjectID] = struct{}{}
		}
	}
	adjacency := groupingAdjacency(compilation.groupingEdges)
	result := make([]groupingRelationRequest, 0, len(blocks)*(len(blocks)-1)/2)
	for left := 0; left < len(blocks); left++ {
		for right := left + 1; right < len(blocks); right++ {
			leftRef := fmt.Sprintf("b%d", left+1)
			rightRef := fmt.Sprintf("b%d", right+1)
			result = append(result, groupingRelationRequest{
				LeftRef: leftRef, RightRef: rightRef,
				SharedRepresentatives:   sharedGroupingObjects(programObjectsByBlock[leftRef], programObjectsByBlock[rightRef]),
				LeftReachesRightMinHops: minGroupingHops(adjacency, programObjectsByBlock[leftRef], programObjectsByBlock[rightRef]),
				RightReachesLeftMinHops: minGroupingHops(adjacency, programObjectsByBlock[rightRef], programObjectsByBlock[leftRef]),
			})
		}
	}
	return result
}

func TestStableModelGroupIDIncludesClaimAndMembership(t *testing.T) {
	left := stableGroupID("target", "Runtime", "Runs accepted work.", []string{"core-b", "core-a"})
	right := stableGroupID("target", "Runtime", "Runs accepted work.", []string{"core-a", "core-b"})
	changedMembers := stableGroupID("target", "Runtime", "Runs accepted work.", []string{"core-a", "core-c"})
	changedClaim := stableGroupID("target", "Execution", "Explains accepted work.", []string{"core-a", "core-b"})
	if left != right || left == changedMembers || left == changedClaim {
		t.Fatalf("stable group IDs = %q / %q / %q / %q", left, right, changedMembers, changedClaim)
	}
	localLeft := stableLocalUnassignedGroupID("target", []string{"core-b", "core-a"})
	localRight := stableLocalUnassignedGroupID("target", []string{"core-a", "core-b"})
	if localLeft != localRight || !strings.HasPrefix(localLeft, "core-group-local-") {
		t.Fatalf("stable local unassigned IDs = %q / %q", localLeft, localRight)
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
