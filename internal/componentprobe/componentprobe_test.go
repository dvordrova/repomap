package componentprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
)

type fakeProvider struct {
	graphs  map[string]evidence.Graph
	fail    map[string]error
	queries []string
}

func (f *fakeProvider) AnalyzeExactSymbol(_ context.Context, request analyzer.ExactSymbolRequest) (evidence.Graph, error) {
	f.queries = append(f.queries, request.Symbol.Name)
	if err := f.fail[request.Symbol.Name]; err != nil {
		return evidence.Graph{}, err
	}
	graph, ok := f.graphs[request.Symbol.Name]
	if !ok {
		return evidence.Graph{}, fmt.Errorf("no graph for %s", request.Symbol.Name)
	}
	return graph, nil
}

func (*fakeProvider) References(_ context.Context, _ string, _ evidence.Location) (evidence.LocationSet, error) {
	return evidence.LocationSet{
		Locations: []evidence.Location{{Path: "sample_test.go", Line: 3, Column: 6}},
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider: "fake", Version: "1", Operation: "references",
		}},
		Scenarios: []evidence.Scenario{{ID: "test-build", Name: "test build"}},
	}, nil
}

// Experiment contract: this test may be replaced when the deeper-research
// cube graduates, but it protects namespaced evidence and honest connectivity.
func TestCollectBuildsConnectedNamespacedEvidence(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeSource(t, repo, "sample.go", "package sample\n\nfunc A() {\n\tB()\n}\n\nfunc B() {}\n")
	writeSource(t, repo, "external.go", "package sample\n\nfunc External() { A() }\n")
	writeSource(t, repo, "sample_test.go", "package sample\n\nfunc TestA() {}\n")
	a := studySymbol("symbol-a", "A", 3)
	b := studySymbol("symbol-b", "B", 7)
	external := studySymbol("symbol-external", "External", 3)
	external.Path = "external.go"
	study, plan := studyPlan([]componentstudy.SymbolCandidate{a, b}, a.ID, b.ID)
	provider := &fakeProvider{graphs: map[string]evidence.Graph{
		"A": exactGraph(repo, a, []callSpec{
			{caller: a, callee: b, callsiteLine: 4},
			{caller: external, callee: a, callsitePath: "external.go", callsiteLine: 3},
		}),
		"B":        exactGraph(repo, b, []callSpec{{caller: a, callee: b, callsiteLine: 4}}),
		"External": exactGraph(repo, external, []callSpec{{caller: external, callee: a, callsitePath: "external.go", callsiteLine: 3}}),
	}}

	bundle, err := Collect(context.Background(), provider, repo, study, plan, Options{})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if bundle.Status != StatusConnected {
		t.Fatalf("Collect().Status = %q, want %q", bundle.Status, StatusConnected)
	}
	if bundle.Round != RoundInitial || bundle.Parent != nil {
		t.Fatal("initial collection did not produce an unparented round 1 bundle")
	}
	if len(bundle.SymbolProbes) != 2 {
		t.Fatalf("Collect().SymbolProbes = %d, want 2", len(bundle.SymbolProbes))
	}
	if bundle.SymbolProbes[0].Structural.Target.EvidenceID != "resolution-001" ||
		bundle.SymbolProbes[1].Structural.Target.EvidenceID != "resolution-001" {
		t.Fatal("raw structural artifacts did not preserve local ids")
	}
	if bundle.SymbolProbes[0].EvidenceIndex[0].ID == bundle.SymbolProbes[1].EvidenceIndex[0].ID {
		t.Fatal("namespaced resolution evidence ids collide across probes")
	}
	if hasWindowAt(bundle.CallsiteWindows, "sample.go", 4) {
		t.Fatal("callsite already covered by a selected symbol source card was emitted again")
	}
	if !hasWindowAt(bundle.CallsiteWindows, "external.go", 3) {
		t.Fatal("external caller source window was not retained")
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Collect().Validate() error = %v", err)
	}
	accepted, ok := frontierNamed(bundle.Frontier, "External")
	if !ok {
		t.Fatal("initial collection has no external callable frontier")
	}
	next, err := CollectFrontier(context.Background(), provider, repo, bundle, accepted.ID, Options{})
	if err != nil {
		t.Fatalf("CollectFrontier() error = %v", err)
	}
	digest, err := SHA256(bundle)
	if err != nil {
		t.Fatalf("SHA256() error = %v", err)
	}
	if next.Round != RoundFrontier || next.Parent == nil ||
		next.Parent.BundleSHA256 != digest || next.Parent.AcceptedFrontierID != accepted.ID {
		t.Fatal("frontier collection did not bind round 2 to round 1")
	}
	if len(next.SymbolProbes) != 1 || next.SymbolProbes[0].Source.Target.Path != "external.go" {
		t.Fatal("frontier collection merged prior source artifacts into round 2")
	}
	if next.Focus.PrimaryQuestion.ID != bundle.Focus.PrimaryQuestion.ID ||
		next.Focus.PrimaryQuestion.Question != bundle.Focus.PrimaryQuestion.Question ||
		next.Focus.PrimaryQuestion.Why != bundle.Focus.PrimaryQuestion.Why ||
		len(next.Focus.PrimaryQuestion.EvidenceIDs) != 1 ||
		next.Focus.PrimaryQuestion.EvidenceIDs[0] != next.SymbolProbes[0].SelectedSymbol.ID {
		t.Fatal("frontier collection changed the research question instead of refining its evidence")
	}
	if err := next.ValidateAgainst(bundle); err != nil {
		t.Fatalf("CollectFrontier().ValidateAgainst() error = %v", err)
	}
	if _, err := CollectFrontier(context.Background(), provider, repo, next, accepted.ID, Options{}); err == nil {
		t.Fatal("CollectFrontier() allowed a third round")
	}
}

// Experiment contract: partial evidence must survive and bounded fan-in must
// not starve the outgoing next hop. This test is intentionally disposable.
func TestCollectRetainsPartialProbeAndFairOutgoingFrontier(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeSource(t, repo, "sample.go", partialSource())
	writeSource(t, repo, "aaa_test.go", "package sample\n\nfunc TestCaller() { A() }\n")
	writeSource(t, repo, "sample_test.go", "package sample\n\nfunc TestA() {}\n")
	a := studySymbol("symbol-a", "A", 3)
	missing := studySymbol("symbol-missing", "Missing", 20)
	z := studySymbol("symbol-z", "Z", 7)
	calls := []callSpec{{caller: a, callee: z, callsiteLine: 4}}
	for i := 1; i <= 5; i++ {
		caller := studySymbol(fmt.Sprintf("caller-%d", i), fmt.Sprintf("Caller%d", i), 8+i)
		calls = append(calls, callSpec{caller: caller, callee: a, callsiteLine: 8 + i})
	}
	testCaller := studySymbol("test-caller", "TestCaller", 3)
	testCaller.Path = "aaa_test.go"
	calls = append(calls, callSpec{caller: testCaller, callee: a, callsitePath: "aaa_test.go", callsiteLine: 3})
	study, plan := studyPlan([]componentstudy.SymbolCandidate{a, missing}, a.ID, missing.ID)
	provider := &fakeProvider{
		graphs: map[string]evidence.Graph{"A": exactGraph(repo, a, calls)},
		fail:   map[string]error{"Missing": errors.New("selected declaration unavailable")},
	}

	bundle, err := Collect(context.Background(), provider, repo, study, plan, Options{MaxCallsiteWindows: 1})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if bundle.Status != StatusFrontier || len(bundle.SymbolProbes) != 1 {
		t.Fatalf("Collect() status/probes = %q/%d, want frontier/1", bundle.Status, len(bundle.SymbolProbes))
	}
	if !hasDirection(bundle.Frontier, DirectionOutgoing) {
		t.Fatal("bounded frontier starved the outgoing next hop")
	}
	if hasTestCallEndpoint(bundle.Frontier) {
		t.Fatal("test caller displaced a production call endpoint in the bounded frontier")
	}
	if !hasWindowDirection(bundle.CallsiteWindows, DirectionIncoming) {
		t.Fatal("external incoming callsite window was not retained")
	}
	if len(bundle.CallsiteWindows) != 1 || strings.HasSuffix(bundle.CallsiteWindows[0].Callsite.Path, "_test.go") {
		t.Fatal("test caller displaced a production callsite window")
	}
	if len(bundle.Frontier) > hardMaxFrontier || len(bundle.CallsiteWindows) > hardMaxCallsiteWindows {
		t.Fatal("partial probe exceeded hard bounds")
	}
	if len(bundle.Warnings) == 0 || bundle.Warnings[0].SymbolID != missing.ID {
		t.Fatal("failed selected symbol was not retained as a warning")
	}
}

type callSpec struct {
	caller       componentstudy.SymbolCandidate
	callee       componentstudy.SymbolCandidate
	callsitePath string
	callsiteLine int
}

func exactGraph(repo string, target componentstudy.SymbolCandidate, calls []callSpec) evidence.Graph {
	graph := evidence.NewGraph(repo, target.Name)
	graph.Scenarios = []evidence.Scenario{{ID: "active-build", Name: "active build"}}
	query := evidence.Entity{ID: "query-" + target.ID, Kind: evidence.EntityQuery, Name: target.Name}
	targetEntity := graphEntity(target)
	graph.AddEntity(query)
	graph.AddEntity(targetEntity)
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: targetEntity.ID, Kind: evidence.RelationResolvesTo, Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "fake", Version: "1", Operation: "exact_symbol", Location: targetEntity.Location}},
		Scenarios:  []string{"active-build"},
	})
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: targetEntity.ID, Kind: evidence.RelationMatchesQuery, Certainty: evidence.CertaintyPossible,
		Provenance: []evidence.Provenance{{Provider: "fake", Version: "1", Operation: "document_symbols", Location: targetEntity.Location}},
		Scenarios:  []string{"active-build"},
	})
	for _, call := range calls {
		caller := graphEntity(call.caller)
		callee := graphEntity(call.callee)
		graph.AddEntity(caller)
		graph.AddEntity(callee)
		callsitePath := call.callsitePath
		if callsitePath == "" {
			callsitePath = "sample.go"
		}
		callsite := evidence.Location{Path: callsitePath, Line: call.callsiteLine, Column: 2}
		graph.AddRelation(evidence.Relation{
			From: caller.ID, To: callee.ID, Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{Provider: "fake", Version: "1", Operation: "call_hierarchy", Location: &callsite}},
			Scenarios:  []string{"active-build"},
		})
	}
	graph.Sort()
	return graph
}

func graphEntity(candidate componentstudy.SymbolCandidate) evidence.Entity {
	location := evidence.Location{Path: candidate.Path, Line: candidate.Line, Column: candidate.Column}
	return evidence.Entity{
		ID: candidate.ID, Kind: evidence.EntityKind(candidate.Kind), Name: candidate.Name, Language: "go", Location: &location,
	}
}

func studyPlan(
	symbols []componentstudy.SymbolCandidate,
	primaryIDs ...string,
) (componentstudy.Bundle, componentstudy.Plan) {
	file := componentstudy.FileCandidate{
		ID: "file-sample", Rank: 1, Path: "sample.go", Reason: "selected package file",
		Provenance: componentstudy.Provenance{Source: "test", Operation: "fixture"}, Certainty: componentstudy.CertaintyStatic,
	}
	study := componentstudy.Bundle{
		Version: componentstudy.BundleVersion, RepoName: "sample",
		Goal:      componentstudy.Goal{ID: "goal-onboarding", Kind: componentstudy.GoalOnboarding, Objective: "understand startup"},
		Component: componentstudy.Component{ID: "component-startup", Name: "Startup", Purpose: "starts services"},
		Anchors:   []componentstudy.AnchorCandidate{}, Files: []componentstudy.FileCandidate{file},
		Symbols: symbols, Evidence: []componentstudy.EvidenceCandidate{},
	}
	plan := componentstudy.Plan{
		Version: componentstudy.PlanVersion, Framing: "Follow the selected lifecycle symbols.",
		Questions: []componentstudy.Question{{
			ID: "question-primary", Question: "How does the lifecycle proceed?", Why: "It grounds onboarding.", EvidenceIDs: primaryIDs,
		}},
		PrimaryQuestionID: "question-primary", SelectedFiles: []componentstudy.FileCandidate{file},
		SelectedSymbols: symbols, Unknowns: []string{}, Warnings: []string{},
	}
	return study, plan
}

func studySymbol(id, name string, line int) componentstudy.SymbolCandidate {
	return componentstudy.SymbolCandidate{
		ID: id, Rank: line, Name: name, Kind: string(evidence.EntityFunction), Path: "sample.go", Line: line, Column: 6,
		Reason: "selected declaration", Provenance: componentstudy.Provenance{Source: "gopls", Operation: "document_symbols"},
		Certainty: componentstudy.CertaintyStatic,
	}
}

func writeSource(t *testing.T, repo, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func partialSource() string {
	return "package sample\n\nfunc A() {\n\tZ()\n}\n\nfunc Z() {}\n\n" +
		"func Caller1() { A() }\n" +
		"func Caller2() { A() }\n" +
		"func Caller3() { A() }\n" +
		"func Caller4() { A() }\n" +
		"func Caller5() { A() }\n" +
		"func Caller6() { A() }\n"
}

func hasDirection(frontier []Frontier, direction Direction) bool {
	for _, candidate := range frontier {
		if candidate.Direction == direction {
			return true
		}
	}
	return false
}

func hasWindowDirection(windows []CallsiteWindow, direction Direction) bool {
	for _, window := range windows {
		if window.Direction == direction {
			return true
		}
	}
	return false
}

func hasWindowAt(windows []CallsiteWindow, path string, line int) bool {
	for _, window := range windows {
		if window.Callsite.Path == path && window.Callsite.Line == line {
			return true
		}
	}
	return false
}

func hasTestCallEndpoint(frontier []Frontier) bool {
	for _, candidate := range frontier {
		if candidate.Kind == FrontierCallEndpoint && strings.HasSuffix(candidate.Location.Path, "_test.go") {
			return true
		}
	}
	return false
}

func frontierNamed(frontier []Frontier, name string) (Frontier, bool) {
	for _, candidate := range frontier {
		if candidate.Kind == FrontierCallEndpoint && candidate.Name == name {
			return candidate, true
		}
	}
	return Frontier{}, false
}
