package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const mechanismStudyExperimentTestRootSymbol = "example.com/fixture.Root"

type mechanismStudyExperimentTestCounts struct {
	analyze         int
	capture         int
	promptFactory   int
	liveFactory     int
	promptBuild     int
	providerSend    int
	semanticResolve int
}

type mechanismStudyExperimentStubProvider struct {
	counts         *mechanismStudyExperimentTestCounts
	envelope       []byte
	prompt         mechanismstudy.Prompt
	rawResponse    []byte
	unsafeResponse []byte
	failOnSend     bool
}

func (provider *mechanismStudyExperimentStubProvider) MechanismStudyPromptJSON(
	prompt mechanismstudy.Prompt,
) ([]byte, error) {
	provider.counts.promptBuild++
	provider.prompt = prompt
	return append([]byte(nil), provider.envelope...), nil
}

func (provider *mechanismStudyExperimentStubProvider) MechanismStudyBodyMeasured(
	_ context.Context,
	body []byte,
) (modelresearch.ProviderResult, error) {
	provider.counts.providerSend++
	if provider.failOnSend {
		return modelresearch.ProviderResult{}, fmt.Errorf("unexpected provider send")
	}
	if !bytes.Equal(body, provider.envelope) {
		return modelresearch.ProviderResult{}, fmt.Errorf("provider body changed")
	}
	if len(provider.unsafeResponse) > 0 {
		return modelresearch.ProviderResult{
			Content: append([]byte(nil), provider.unsafeResponse...), Attempts: 1,
			RequestBytes: len(body), ResponseBytes: len(provider.unsafeResponse),
			FinishReason: "stop", ChoiceCount: 1,
		}, nil
	}
	request, err := mechanismStudyExperimentRequestFromPrompt(provider.prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	if len(request.Cards) != 1 || len(request.Cards[0].Edges) < 2 || len(request.Cards[0].Readings) != 1 {
		return modelresearch.ProviderResult{}, fmt.Errorf("unexpected request graph")
	}
	card := request.Cards[0]
	response := mechanismstudy.Response{
		Version:    mechanismstudy.ResultVersion,
		CatalogRef: request.CatalogRef, CatalogSHA256: request.CatalogSHA256,
		RequestRef: request.RequestRef,
		Cards: []mechanismstudy.ResponseCard{{
			CardRef: card.Ref,
			Mechanisms: []mechanismstudy.Candidate{{EdgeRefs: []string{
				card.Edges[0].Ref, card.Edges[1].Ref,
			}}},
		}},
	}
	provider.rawResponse, err = json.Marshal(response)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	return modelresearch.ProviderResult{
		Content: append([]byte(nil), provider.rawResponse...), Attempts: 1,
		RequestBytes: len(body), ResponseBytes: len(provider.rawResponse) + 64,
		FinishReason: "stop", ChoiceCount: 1,
	}, nil
}

func TestMechanismStudyExperimentPersistsOneValidatedProviderCall(t *testing.T) {
	repo := t.TempDir()
	out := filepath.Join(t.TempDir(), "experiment")
	index := mechanismStudyExperimentSyntheticIndex(t, 2)
	counts := &mechanismStudyExperimentTestCounts{}
	provider := &mechanismStudyExperimentStubProvider{
		counts: counts,
		envelope: []byte(
			`{"model":"fixture-model","messages":[],"max_tokens":64000,"response_format":{"type":"json_object"}}`,
		),
	}
	deps := mechanismStudyExperimentTestDependencies(t, repo, &index, provider, counts)
	var stdout, stderr bytes.Buffer
	err := runMechanismStudyExperimentCLIWith(
		t.Context(), mechanismStudyExperimentArgs(repo, out, false), &stdout, &stderr, deps,
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts.analyze != 1 || counts.capture != 2 || counts.promptFactory != 0 ||
		counts.liveFactory != 1 || counts.promptBuild != 1 || counts.providerSend != 1 {
		t.Fatalf("dependency calls = %+v", counts)
	}
	if !strings.Contains(stdout.String(), "outcome: mechanism") ||
		!strings.Contains(stdout.String(), "provider_calls: 1") || stderr.Len() != 0 {
		t.Fatalf("stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}

	requireMechanismStudyExperimentFiles(t, out, []string{
		mechanismStudyExperimentBundleFile,
		mechanismStudyExperimentEnvelopeFile,
		mechanismStudyExperimentResponseFile,
		mechanismStudyExperimentSummaryFile,
		mechanismStudyExperimentValidatedFile,
	})
	if got := readMechanismStudyExperimentFile(t, out, mechanismStudyExperimentEnvelopeFile); !bytes.Equal(got, provider.envelope) {
		t.Fatalf("provider envelope changed\ngot:  %s\nwant: %s", got, provider.envelope)
	}
	if got := readMechanismStudyExperimentFile(t, out, mechanismStudyExperimentResponseFile); !bytes.Equal(got, provider.rawResponse) {
		t.Fatalf("raw response changed\ngot:  %s\nwant: %s", got, provider.rawResponse)
	}

	var summary mechanismStudyExperimentSummary
	decodeMechanismStudyExperimentFile(t, out, mechanismStudyExperimentSummaryFile, &summary)
	if summary.Mode != "provider" || summary.Outcome != mechanismstudy.OutcomeMechanism ||
		summary.RequestCount != 1 || summary.ProviderCalls != 1 || summary.ProviderAttempts != 1 ||
		summary.ResolvedReadings != 1 || summary.AdvertisedNodes != 3 ||
		summary.AdvertisedEdges != 2 || summary.MechanismCards != 1 || summary.PreparedCards != 0 ||
		summary.DirectCallIndexSHA256 != index.SHA256 || summary.RawResponseSHA256 == "" {
		t.Fatalf("summary = %+v", summary)
	}
	var validated mechanismStudyExperimentValidated
	decodeMechanismStudyExperimentFile(t, out, mechanismStudyExperimentValidatedFile, &validated)
	if len(validated.Cards) != 1 || validated.Cards[0].State != mechanismstudy.OutcomeMechanism ||
		len(validated.Cards[0].Mechanisms) != 1 || len(validated.RequestResults) != 1 {
		t.Fatalf("validated result = %+v", validated)
	}

	for _, name := range []string{
		mechanismStudyExperimentSummaryFile,
		mechanismStudyExperimentBundleFile,
		mechanismStudyExperimentEnvelopeFile,
		mechanismStudyExperimentResponseFile,
		mechanismStudyExperimentValidatedFile,
	} {
		data := readMechanismStudyExperimentFile(t, out, name)
		for _, forbidden := range []string{
			mechanismStudyExperimentTestRootSymbol, "fixture.go", "direct-node-",
			"Authorization", "api_key", "source_body", "direct_call_index\"",
		} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Fatalf("%s leaked forbidden %q: %s", name, forbidden, data)
			}
		}
	}
}

func TestMechanismStudyExperimentSkipsProviderWithoutTwoEdgePath(t *testing.T) {
	repo := t.TempDir()
	out := filepath.Join(t.TempDir(), "prepared")
	index := mechanismStudyExperimentSyntheticIndex(t, 1)
	counts := &mechanismStudyExperimentTestCounts{}
	provider := &mechanismStudyExperimentStubProvider{
		counts: counts, envelope: []byte(`{"unused":true}`), failOnSend: true,
	}
	deps := mechanismStudyExperimentTestDependencies(t, repo, &index, provider, counts)
	var stdout, stderr bytes.Buffer
	if err := runMechanismStudyExperimentCLIWith(
		t.Context(), mechanismStudyExperimentArgs(repo, out, false), &stdout, &stderr, deps,
	); err != nil {
		t.Fatal(err)
	}
	if counts.analyze != 1 || counts.capture != 2 || counts.promptFactory != 0 ||
		counts.liveFactory != 0 || counts.promptBuild != 0 || counts.providerSend != 0 {
		t.Fatalf("zero-call dependency counts = %+v", counts)
	}
	if !strings.Contains(stdout.String(), "outcome: prepared_investigation") ||
		!strings.Contains(stdout.String(), "request_count: 0") ||
		!strings.Contains(stdout.String(), "provider_calls: 0") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	requireMechanismStudyExperimentFiles(t, out, []string{
		mechanismStudyExperimentSummaryFile, mechanismStudyExperimentValidatedFile,
	})
	var summary mechanismStudyExperimentSummary
	decodeMechanismStudyExperimentFile(t, out, mechanismStudyExperimentSummaryFile, &summary)
	if summary.Mode != "provider_skipped" || summary.Outcome != mechanismstudy.OutcomePrepared ||
		summary.RequestCount != 0 || summary.ProviderCalls != 0 || summary.PreparedCards != 1 ||
		summary.MechanismCards != 0 || summary.AdvertisedEdges != 1 {
		t.Fatalf("prepared summary = %+v", summary)
	}
	var validated mechanismStudyExperimentValidated
	decodeMechanismStudyExperimentFile(t, out, mechanismStudyExperimentValidatedFile, &validated)
	if len(validated.Cards) != 1 || validated.Cards[0].State != mechanismstudy.OutcomePrepared ||
		len(validated.Cards[0].Mechanisms) != 0 || len(validated.RequestResults) != 0 {
		t.Fatalf("prepared result = %+v", validated)
	}
}

func TestMechanismStudyExperimentRequestOnlyBuildsButDoesNotSend(t *testing.T) {
	repo := t.TempDir()
	out := filepath.Join(t.TempDir(), "request-only")
	index := mechanismStudyExperimentSyntheticIndex(t, 2)
	counts := &mechanismStudyExperimentTestCounts{}
	provider := &mechanismStudyExperimentStubProvider{
		counts: counts, envelope: []byte(`{"exact":"request-only-envelope"}`), failOnSend: true,
	}
	deps := mechanismStudyExperimentTestDependencies(t, repo, &index, provider, counts)
	var stdout, stderr bytes.Buffer
	if err := runMechanismStudyExperimentCLIWith(
		t.Context(), mechanismStudyExperimentArgs(repo, out, true), &stdout, &stderr, deps,
	); err != nil {
		t.Fatal(err)
	}
	if counts.analyze != 1 || counts.capture != 2 || counts.promptFactory != 1 ||
		counts.liveFactory != 0 || counts.promptBuild != 1 || counts.providerSend != 0 {
		t.Fatalf("request-only dependency counts = %+v", counts)
	}
	requireMechanismStudyExperimentFiles(t, out, []string{
		mechanismStudyExperimentBundleFile,
		mechanismStudyExperimentEnvelopeFile,
		mechanismStudyExperimentSummaryFile,
		mechanismStudyExperimentValidatedFile,
	})
	var summary mechanismStudyExperimentSummary
	decodeMechanismStudyExperimentFile(t, out, mechanismStudyExperimentSummaryFile, &summary)
	if summary.Mode != "request_only" || summary.Outcome != mechanismstudy.OutcomePrepared ||
		summary.RequestCount != 1 || summary.ProviderCalls != 0 || summary.PreparedCards != 1 ||
		summary.RequestSHA256 == "" || summary.ProviderEnvelopeSHA256 == "" {
		t.Fatalf("request-only summary = %+v", summary)
	}
}

func TestMechanismStudyExperimentRejectsUnsafeProviderResponseBeforePersistence(t *testing.T) {
	repo := t.TempDir()
	out := filepath.Join(t.TempDir(), "unsafe")
	index := mechanismStudyExperimentSyntheticIndex(t, 2)
	counts := &mechanismStudyExperimentTestCounts{}
	provider := &mechanismStudyExperimentStubProvider{
		counts: counts, envelope: []byte(`{"exact":"safe"}`),
		unsafeResponse: []byte(`{"token":"sk-abcdefghijklmnop"}`),
	}
	deps := mechanismStudyExperimentTestDependencies(t, repo, &index, provider, counts)
	var stdout, stderr bytes.Buffer
	err := runMechanismStudyExperimentCLIWith(
		t.Context(), mechanismStudyExperimentArgs(repo, out, false), &stdout, &stderr, deps,
	)
	if err == nil || !strings.Contains(err.Error(), "credential scan") ||
		!strings.Contains(err.Error(), "secret_key") {
		t.Fatalf("unsafe response error = %v", err)
	}
	if counts.providerSend != 1 {
		t.Fatalf("provider sends = %d, want 1", counts.providerSend)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe response created output: %v", statErr)
	}
}

func TestMechanismStudyExperimentHelpAndDevSwitchAreWired(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runMechanismStudyExperimentCLIWith(
		t.Context(), []string{"--help"}, &stdout, &stderr, mechanismStudyExperimentDependencies{},
	); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{
		"--repo", "--root-path", "--root-line", "--root-symbol",
		"--label", "--question", "--out", "--request-only",
	} {
		if !strings.Contains(stdout.String(), flag) {
			t.Fatalf("help omitted %s: %q", flag, stdout.String())
		}
	}
	if err := runMechanismStudyExperimentCLIWith(
		t.Context(), nil, io.Discard, io.Discard, mechanismStudyExperimentDependencies{},
	); err == nil || !strings.Contains(err.Error(), mechanismStudyExperimentUsage) {
		t.Fatalf("missing-flag error = %v", err)
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(mainSource, []byte(`case "mechanism-study-experiment":`)) ||
		!bytes.Contains(mainSource, []byte("mechanism-study-experiment --repo <repo>")) {
		t.Fatal("main dev switch or dev usage is not wired")
	}
}

func mechanismStudyExperimentArgs(repo, out string, requestOnly bool) []string {
	args := []string{
		"--repo", repo,
		"--root-path", "fixture.go",
		"--root-line", "10",
		"--root-symbol", mechanismStudyExperimentTestRootSymbol,
		"--label", "Fixture path",
		"--question", "How does the exact root reach the leaf?",
		"--out", out,
	}
	if requestOnly {
		args = append(args, "--request-only")
	}
	return args
}

func mechanismStudyExperimentTestDependencies(
	t *testing.T,
	repo string,
	index *surfacediscovery.DirectCallIndex,
	provider mechanismStudyExperimentProvider,
	counts *mechanismStudyExperimentTestCounts,
) mechanismStudyExperimentDependencies {
	t.Helper()
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	absRepo, err := filepath.Abs(resolvedRepo)
	if err != nil {
		t.Fatal(err)
	}
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: filepath.Clean(absRepo), Head: strings.Repeat("a", 40),
		Dirty: []freshness.DirtyFile{}, Submodules: []freshness.SubmoduleState{},
	}
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	return mechanismStudyExperimentDependencies{
		analyzeContext: func(_ context.Context, options surfacediscovery.Options) (surfacediscovery.Result, error) {
			counts.analyze++
			if options.RepoPath != filepath.Clean(absRepo) {
				return surfacediscovery.Result{}, fmt.Errorf("analyzed repo = %q", options.RepoPath)
			}
			return surfacediscovery.Result{DirectCallIndex: index}, nil
		},
		captureRepository: func(_ context.Context, path string) (freshness.RepositoryState, error) {
			counts.capture++
			if path != filepath.Clean(absRepo) {
				return freshness.RepositoryState{}, fmt.Errorf("captured repo = %q", path)
			}
			return state, nil
		},
		newPromptProvider: func(io.Writer) (mechanismStudyExperimentProvider, error) {
			counts.promptFactory++
			return provider, nil
		},
		newLiveProvider: func(io.Writer) (mechanismStudyExperimentProvider, error) {
			counts.liveFactory++
			return provider, nil
		},
	}
}

func mechanismStudyExperimentRequestFromPrompt(prompt mechanismstudy.Prompt) (mechanismstudy.Request, error) {
	const marker = "Exact request bundle JSON:\n"
	position := strings.LastIndex(prompt.User, marker)
	if position < 0 {
		return mechanismstudy.Request{}, fmt.Errorf("prompt omitted exact request bundle")
	}
	var request mechanismstudy.Request
	if err := json.Unmarshal([]byte(prompt.User[position+len(marker):]), &request); err != nil {
		return mechanismstudy.Request{}, fmt.Errorf("decode request from prompt: %w", err)
	}
	return request, nil
}

func mechanismStudyExperimentSyntheticIndex(t *testing.T, edgeCount int) surfacediscovery.DirectCallIndex {
	t.Helper()
	if edgeCount < 0 || edgeCount > 2 {
		t.Fatalf("unsupported edge count %d", edgeCount)
	}
	scenario := surfacediscovery.Scenario{
		ID: "fixture-scenario", GOOS: "linux", GOARCH: "amd64", Tags: []string{},
	}
	module := surfacediscovery.DirectCallModule{Path: "example.com/fixture", Directory: "."}
	module.ID = mechanismStudyExperimentStableID("direct-module", module.Path, module.Directory)
	type nodeSpec struct {
		name string
		line int
	}
	specs := []nodeSpec{{name: "Root", line: 10}, {name: "Middle", line: 20}, {name: "Leaf", line: 30}}
	nodes := make([]surfacediscovery.DirectCallNode, 0, edgeCount+1)
	for _, spec := range specs[:edgeCount+1] {
		location := surfacediscovery.Location{Path: "fixture.go", Line: spec.line, Column: 1}
		symbol := surfacediscovery.Symbol{
			ID:      "example.com/fixture." + spec.name,
			Package: "example.com/fixture", Name: spec.name, Location: location,
			EquivalentIDs: []string{},
		}
		node := surfacediscovery.DirectCallNode{
			Symbol: symbol, Package: symbol.Package, ModuleID: module.ID, ScenarioID: scenario.ID,
			Declaration: location,
			Body: surfacediscovery.DirectCallBodyRange{
				Start: location,
				End:   surfacediscovery.Location{Path: location.Path, Line: location.Line + 2, Column: 1},
			},
		}
		node.ID = mechanismStudyExperimentStableID(
			"direct-node", node.ModuleID, node.ScenarioID, node.Symbol.ID,
			mechanismStudyExperimentLocationKey(node.Declaration),
		)
		nodes = append(nodes, node)
	}
	nodeByName := make(map[string]surfacediscovery.DirectCallNode, len(nodes))
	for _, node := range nodes {
		nodeByName[node.Symbol.Name] = node
	}
	sort.Slice(nodes, func(i, j int) bool {
		return mechanismStudyExperimentNodeKey(nodes[i]) < mechanismStudyExperimentNodeKey(nodes[j])
	})
	edges := make([]surfacediscovery.DirectCallEdge, 0, edgeCount)
	for position, pair := range [][2]string{{"Root", "Middle"}, {"Middle", "Leaf"}}[:edgeCount] {
		edge := surfacediscovery.DirectCallEdge{
			CallerID: nodeByName[pair[0]].ID, CalleeID: nodeByName[pair[1]].ID,
			ScenarioID: scenario.ID, Invocation: surfacediscovery.DirectCallSynchronous,
			RepresentativeCallsite: surfacediscovery.Location{
				Path: "fixture.go", Line: 11 + position*10, Column: 2,
			},
			WitnessCount: 1,
		}
		edge.ID = mechanismStudyExperimentStableID(
			"direct-edge", edge.ScenarioID, edge.CallerID, edge.CalleeID, string(edge.Invocation),
		)
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		return mechanismStudyExperimentEdgeKey(edges[i]) < mechanismStudyExperimentEdgeKey(edges[j])
	})
	index := surfacediscovery.DirectCallIndex{
		Version: surfacediscovery.DirectCallIndexVersion,
		State:   surfacediscovery.DirectCallIndexReady, Scenario: scenario,
		Modules: []surfacediscovery.DirectCallModule{module}, Nodes: nodes, Edges: edges,
		Frontiers: []surfacediscovery.DirectCallNodeFrontier{},
		Coverage: surfacediscovery.DirectCallIndexCoverage{
			FunctionsConsidered: len(nodes), CallInstructionsConsidered: len(edges),
			ModulesIndexed: 1, NodesConsidered: len(nodes), NodesIndexed: len(nodes),
			UniqueEdgesConsidered: len(edges), EdgesIndexed: len(edges),
			DirectStaticWitnessesIndexed: len(edges),
		},
	}
	index.SHA256 = mechanismStudyExperimentIndexSHA256(t, index)
	if err := index.Validate(); err != nil {
		t.Fatalf("synthetic DirectCallIndex: %v", err)
	}
	return index
}

func mechanismStudyExperimentStableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func mechanismStudyExperimentLocationKey(location surfacediscovery.Location) string {
	return fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column)
}

func mechanismStudyExperimentNodeKey(node surfacediscovery.DirectCallNode) string {
	return strings.Join([]string{
		node.ModuleID, node.Package, node.Symbol.ID,
		mechanismStudyExperimentLocationKey(node.Declaration), node.ID,
	}, "\x00")
}

func mechanismStudyExperimentEdgeKey(edge surfacediscovery.DirectCallEdge) string {
	return strings.Join([]string{
		edge.CallerID, edge.CalleeID, string(edge.Invocation),
		mechanismStudyExperimentLocationKey(edge.RepresentativeCallsite), edge.ID,
	}, "\x00")
}

func mechanismStudyExperimentIndexSHA256(t *testing.T, index surfacediscovery.DirectCallIndex) string {
	t.Helper()
	index.SHA256 = ""
	encoded, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func requireMechanismStudyExperimentFiles(t *testing.T, out string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected output directory %s", entry.Name())
		}
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("artifact files = %v, want %v", got, want)
	}
}

func readMechanismStudyExperimentFile(t *testing.T, out, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeMechanismStudyExperimentFile(t *testing.T, out, name string, target any) {
	t.Helper()
	if err := json.Unmarshal(readMechanismStudyExperimentFile(t, out, name), target); err != nil {
		t.Fatal(err)
	}
}
