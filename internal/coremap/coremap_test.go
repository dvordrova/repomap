package coremap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestCompileProgramRunsCoreMapFromLanguageNeutralObjects(t *testing.T) {
	_, repository := coreMapTestCorpus(t, map[string][]byte{
		"pkg/main.py": []byte("def run():\n    callback()\n"),
	})
	fileRef, ok := repository.ID("pkg/main.py")
	if !ok {
		t.Fatal("fixture file is absent from corpus")
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "module-main", Kind: programindex.ObjectModule, Name: "pkg.main", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1}},
		{SourceRef: "run", Kind: programindex.ObjectFunction, Name: "run", Visibility: programindex.VisibilityPublic, Signature: "run()", OwnerRef: "module-main", ContainerRef: "module-main", Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1}},
		{SourceRef: "app-type", Kind: programindex.ObjectType, Name: "App", Visibility: programindex.VisibilityPublic, OwnerRef: "module-main", ContainerRef: "module-main", Location: &programindex.Location{Path: "pkg/main.py", Line: 4, Column: 1}},
		{SourceRef: "app-variable", Kind: programindex.ObjectVariable, Name: "app", Visibility: programindex.VisibilityPublic, OwnerRef: "module-main", ContainerRef: "module-main", Location: &programindex.Location{Path: "pkg/main.py", Line: 5, Column: 1}},
		{SourceRef: "private-variable", Kind: programindex.ObjectVariable, Name: "_secret", Visibility: programindex.VisibilityInternal, OwnerRef: "module-main", ContainerRef: "module-main", Location: &programindex.Location{Path: "pkg/main.py", Line: 6, Column: 1}},
		{SourceRef: "class-variable", Kind: programindex.ObjectVariable, Name: "Setting", Visibility: programindex.VisibilityPublic, OwnerRef: "app-type", ContainerRef: "app-type", Location: &programindex.Location{Path: "pkg/main.py", Line: 7, Column: 5}},
	}
	relations := []programindex.RelationInput{
		{
			SourceRef: "call-callback", Kind: programindex.RelationCalls,
			FromRef: "run", Resolution: programindex.ResolutionUnresolved,
			Location:        &programindex.Location{Path: "pkg/main.py", Line: 2, Column: 5},
			TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "callsite", Detail: "callback"}},
			WitnessesObserved: 1,
		},
		{
			SourceRef: "call-alternatives", Kind: programindex.RelationCalls,
			FromRef: "run", ToRefs: []string{"app-type", "app-variable"},
			Resolution:      programindex.ResolutionAlternatives,
			Location:        &programindex.Location{Path: "pkg/main.py", Line: 3, Column: 5},
			TargetsObserved: 2, Witnesses: []programindex.Witness{{Kind: "callsite_candidates", Detail: "App or app"}},
			WitnessesObserved: 1,
		},
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64), SourceSHA256: strings.Repeat("d", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "pkg.main", Selector: "python:pkg.main",
			Sources:       []programindex.TargetSource{{FileRef: string(fileRef), Path: "pkg/main.py"}},
			AnchorFileRef: string(fileRef),
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "run", Kind: programindex.SeedCallable,
				Location: &programindex.Location{Path: "pkg/main.py", Line: 1, Column: 1},
			}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	compilation, err := CompileProgram("sample", repository, index, readmetargetscout.Result{})
	if err != nil {
		t.Fatal(err)
	}
	if compilation.programTarget == nil || compilation.programTarget.ID != index.Target.ID ||
		compilation.programIndexSHA256 != index.SHA256 || len(compilation.symbolRows) != 3 ||
		len(compilation.dynamicRelationRows) != 4 ||
		compilation.symbolRows[0].OutgoingCalls != 2 || compilation.symbolRows[0].UnresolvedOutgoing != 2 ||
		compilation.symbolRows[0].Ref != "s1" ||
		!slices.Equal(compilation.symbolRows[0].TargetSeedKinds, []programindex.SeedKind{programindex.SeedCallable}) ||
		len(compilation.targetSeedRows) != 1 || compilation.targetSeedRows[0].SymbolRef != "s1" {
		t.Fatalf("program compilation = %#v / %#v", compilation, compilation.symbolRows)
	}
	dynamicTargets := make([]string, 0, 2)
	for position := 0; position < len(compilation.dynamicRelationRows); position += 2 {
		fromView := compilation.dynamicRelationRows[position]
		toView := compilation.dynamicRelationRows[position+1]
		wantRef := fmt.Sprintf("r%d", position/2+1)
		if fromView.Ref != wantRef || toView.Ref != wantRef ||
			fromView.JointRef != "j1" || toView.JointRef != "j1" ||
			fromView.Perspective != "from" || toView.Perspective != "to" ||
			fromView.Resolution != programindex.ResolutionAlternatives ||
			fromView.From.SymbolRef != "s1" || fromView.To == nil || toView.To == nil ||
			fromView.To.SymbolRef != toView.To.SymbolRef ||
			fromView.TargetOrdinal != position/2+1 || toView.TargetOrdinal != position/2+1 {
			t.Fatalf("dynamic relation projection = %#v", compilation.dynamicRelationRows)
		}
		dynamicTargets = append(dynamicTargets, fromView.To.SymbolRef)
	}
	slices.Sort(dynamicTargets)
	if !slices.Equal(dynamicTargets, []string{"s2", "s3"}) {
		t.Fatalf("dynamic relation targets = %#v", dynamicTargets)
	}
	dynamicWire, err := json.Marshal(compilation.dynamicRelationRows)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dynamicWire, []byte("call-alternatives")) || bytes.Contains(dynamicWire, []byte(index.Relations[1].ID)) {
		t.Fatalf("dynamic relation projection leaked internal relation identities: %s", dynamicWire)
	}
	if compilation.symbolRows[0].Kind != programindex.ObjectFunction ||
		compilation.symbolRows[1].Kind != programindex.ObjectType ||
		compilation.symbolRows[2].Kind != programindex.ObjectVariable ||
		compilation.symbolRows[2].Name != "app" ||
		compilation.symbolRows[1].IncomingCalls != 0 || compilation.symbolRows[2].IncomingCalls != 0 {
		t.Fatalf("program core object catalog = %#v", compilation.symbolRows)
	}
	if compilation.symbols["s1"].fact.Symbol.ID != compilation.symbols["s1"].fact.NodeID {
		t.Fatalf("program core object identity = %#v", compilation.symbols["s1"].fact)
	}
	provider := &coreMapTestProvider{response: []byte(
		`{"blocks":[{"name":"Execution","purpose":"Runs the selected Python target.","file_refs":[],"symbol_refs":["s1"]}]}`,
	)}
	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, compilation)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProgramTarget == nil || result.ProgramTarget.ID != index.Target.ID ||
		result.ProgramIndexSHA256 != index.SHA256 || result.DirectCallSHA256 != "" || result.CoreObjectSHA256 != "" {
		t.Fatalf("program result authority = %#v", result)
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

func TestRefinedMapPackingKeepsEveryFactWithoutSlicing(t *testing.T) {
	compilation := Compilation{
		repository: "sample",
		baselineRequest: baselineRequest{Target: targetRequest{
			Language: "python", Kind: "library", Name: "sample", Selector: "sample",
		}},
	}
	facts := make([]semanticFactRequest, 96)
	for position := range facts {
		row := symbolRequest{
			Ref: fmt.Sprintf("s%d", position+1), FileRef: corpus.FileID(fmt.Sprintf("f%d", position+1)),
			Path: fmt.Sprintf("pkg/module_%03d.py", position+1), Line: 1, Package: "pkg",
			Name: fmt.Sprintf("operation_%03d", position+1), Signature: strings.Repeat("x", 12<<10),
		}
		facts[position] = semanticFactRequest{Ref: fmt.Sprintf("q%d", position+1), Symbol: &row}
	}
	requests, err := packMapRequests(compilation, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) < 2 {
		t.Fatalf("map requests = %d, want multiple bounded shards", len(requests))
	}
	seen := make(map[string]int, len(facts))
	for _, request := range requests {
		wire, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if len(wire)+len(refinedPrompt) > maxRefinedPayloadBytes {
			t.Fatalf("map request bytes = %d plus prompt", len(wire))
		}
		for _, fact := range request.Facts {
			seen[fact.Ref]++
		}
	}
	for _, fact := range facts {
		if seen[fact.Ref] != 1 {
			t.Fatalf("fact %s transmission count = %d", fact.Ref, seen[fact.Ref])
		}
	}
}

func TestRefinedReducePackingKeepsEveryCandidateWithoutSlicing(t *testing.T) {
	fileRef := corpus.FileID("f1")
	file := FileFact{FileRef: fileRef, Path: "pkg/main.py"}
	row := symbolRequest{Ref: "s1", FileRef: fileRef, Path: file.Path, Line: 1, Package: "pkg", Name: "run"}
	compilation := Compilation{
		repository: "sample",
		baselineRequest: baselineRequest{Target: targetRequest{
			Language: "python", Kind: "library", Name: "sample", Selector: "sample",
		}},
		files: map[corpus.FileID]FileFact{fileRef: file},
		symbols: map[string]symbolAuthority{
			"s1": {request: row, fact: SymbolFact{NodeID: "node-1"}},
		},
	}
	proposals := make([]proposal, 1_200)
	for position := range proposals {
		proposals[position] = proposal{
			Name: fmt.Sprintf("Candidate %d", position+1), Purpose: strings.Repeat("p", maxPurposeBytes),
			FileRefs: []corpus.FileID{fileRef}, SymbolRefs: []string{"s1"},
		}
	}
	requests, err := packReduceRequests(compilation, 1, proposals)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) < 2 {
		t.Fatalf("reduce requests = %d, want multiple bounded batches", len(requests))
	}
	total := 0
	for _, request := range requests {
		wire, err := marshalReduceRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		if len(wire)+len(reducePrompt) > maxRefinedPayloadBytes {
			t.Fatalf("reduce request bytes = %d plus prompt", len(wire))
		}
		total += len(request.Candidates)
	}
	if total != len(proposals) {
		t.Fatalf("transmitted candidates = %d, want %d", total, len(proposals))
	}
}

func TestDynamicRelationAdvertisesEndpointSymbolButNotUnsentFileRef(t *testing.T) {
	fileRef := corpus.FileID("f1")
	file := FileFact{FileRef: fileRef, Path: "pkg/main.py"}
	row := symbolRequest{Ref: "s1", FileRef: fileRef, Path: file.Path, Line: 1, Package: "pkg", Name: "run"}
	compilation := Compilation{
		files: map[corpus.FileID]FileFact{fileRef: file},
		symbols: map[string]symbolAuthority{
			"s1": {request: row, fact: SymbolFact{NodeID: "node-1"}},
		},
	}
	relation := dynamicRelationRequest{
		Ref: "r1", JointRef: "j1", Perspective: "from",
		Kind: programindex.RelationPassesCallback, Resolution: programindex.ResolutionUnresolved,
		From: relationEndpointRequest{
			SymbolRef: "s1", Kind: programindex.ObjectFunction, Name: "run",
			Visibility: programindex.VisibilityPublic,
		},
		TargetsObserved: 1, TargetsOmitted: 1,
	}
	authority, err := authorityForFacts(compilation, []semanticFactRequest{{Ref: "q1", DynamicRelation: &relation}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := authority.symbols["s1"]; !ok || len(authority.files) != 0 {
		t.Fatalf("dynamic relation authority = %#v", authority)
	}
	blocks := []proposal{{
		Name: "Dispatch", Purpose: "Runs supplied behavior.",
		SymbolRefs: []string{"s1", "s999", "s1"},
		FileRefs:   []corpus.FileID{fileRef, "f999"},
	}}
	blocks = normalizeProposalRefs(blocks, authority.files, authority.symbols)
	if err := validateRefinedBatchProposals(blocks, authority.files, authority.symbols, true); err != nil {
		t.Fatalf("known authority was lost while discarding unknown refs: %v", err)
	}
	if len(blocks[0].FileRefs) != 0 || len(blocks[0].SymbolRefs) != 1 || blocks[0].SymbolRefs[0] != "s1" {
		t.Fatalf("normalized proposal refs = %#v", blocks[0])
	}

	ungrounded := []proposal{{
		Name: "Invented", Purpose: "Has no request-local authority.",
		SymbolRefs: []string{"s999"}, FileRefs: []corpus.FileID{"f999"},
	}}
	ungrounded = normalizeProposalRefs(ungrounded, authority.files, authority.symbols)
	if err := validateRefinedBatchProposals(ungrounded, authority.files, authority.symbols, true); err == nil {
		t.Fatal("proposal grounded only by invented refs was accepted")
	}
}

func TestRefinedProposalNormalizationDeduplicatesExactRecordsButPreservesDistinctClaims(t *testing.T) {
	fileRef := corpus.FileID("f1")
	file := FileFact{FileRef: fileRef, Path: "pkg/server.go"}
	firstFact := SymbolFact{
		NodeID: "node-1", Kind: programindex.ObjectFunction,
		Declaration: surfacediscovery.Location{Path: file.Path, Line: 10},
	}
	secondFact := SymbolFact{
		NodeID: "node-2", Kind: programindex.ObjectFunction,
		Declaration: surfacediscovery.Location{Path: file.Path, Line: 20},
	}
	authority := map[string]symbolAuthority{
		"s1": {fact: firstFact},
		"s2": {fact: secondFact},
	}
	blocks := []proposal{
		{
			Name: "Trainer server construction", Purpose: "Builds the trainer gRPC server.",
			FileRefs:   []corpus.FileID{fileRef, "f999", fileRef},
			SymbolRefs: []string{"s1", "s999", "s2", "s1"},
		},
		{
			Name: "Trainer server construction", Purpose: "Builds the trainer gRPC server.",
			FileRefs: []corpus.FileID{fileRef}, SymbolRefs: []string{"s2", "s1"},
		},
		{
			Name: "gRPC server construction", Purpose: "Builds shared gRPC servers.",
			FileRefs: []corpus.FileID{fileRef}, SymbolRefs: []string{"s1", "s2"},
		},
	}

	blocks = normalizeProposalRefs(blocks, map[corpus.FileID]FileFact{fileRef: file}, authority)
	if len(blocks) != 2 {
		t.Fatalf("normalized blocks = %#v, want one exact duplicate removed and both distinct claims", blocks)
	}
	if err := validateRefinedBatchProposals(blocks, map[corpus.FileID]FileFact{fileRef: file}, authority, true); err != nil {
		t.Fatal(err)
	}
	restored := restoreBlocks(StageRefined, "target-1", blocks, map[corpus.FileID]FileFact{fileRef: file}, authority)
	if restored[0].ID == restored[1].ID {
		t.Fatalf("distinct semantic claims share stable ID %q", restored[0].ID)
	}
	if err := validateRestoredBlocks(StageRefined, "target-1", restored, true); err != nil {
		t.Fatal(err)
	}
}

type coreMapTestProvider struct {
	response      []byte
	completeCalls int
}

func (provider *coreMapTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`)
}

func (provider *coreMapTestProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.System + "\n" + prompt.User))
}

func (provider *coreMapTestProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.completeCalls++
	return llm.Completion{
		Response: provider.response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{
			InputTokens: 10, OutputTokens: 10, ProviderResponseBytes: len(provider.response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}

func coreMapTestCorpus(t *testing.T, files map[string][]byte) (string, *corpus.Corpus) {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, len(files))
	for name, content := range files {
		absolute := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, content, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	slices.Sort(paths)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{
		Paths: append([]string(nil), paths...), RegularPaths: append([]string(nil), paths...),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return root, repository
}
