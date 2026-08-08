package surfacediscovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestDirectCallIndexBuildsExactConnectedChainAndWitnesses(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/chain\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

func main() { boot() }

func boot() { load() }

func load() {

	persist()
	persist()
}

func schedule() {
	go persist()
	defer persist()
}

func persist() {}
`)

	result, err := Analyze(DefaultOptions(repository))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	index := requireReadyDirectCallIndex(t, result)
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(index.Modules) != 1 || index.Modules[0].Path != "example.com/chain" || index.Modules[0].Directory != "." {
		t.Fatalf("modules = %+v, want exact root module", index.Modules)
	}
	if index.Scenario.ID != scenarioID(runtime.GOOS, runtime.GOARCH, nil) {
		t.Fatalf("scenario = %q", index.Scenario.ID)
	}

	boot := requireDirectCallNode(t, index, "example.com/chain.boot")
	if boot.Package != "example.com/chain" || boot.Declaration.Path != "main.go" || boot.Declaration.Line != 5 {
		t.Fatalf("boot node = %+v", boot)
	}
	if boot.Body.Start.Path != "main.go" || boot.Body.Start.Line != 5 ||
		boot.Body.End.Path != "main.go" || boot.Body.End.Line != 5 {
		t.Fatalf("boot body = %+v, want exact declaration range", boot.Body)
	}
	root := index.ResolveRoot("main.go", 5, "example.com/chain.boot")
	if root.State != DirectCallRootResolved || root.Binding != DirectCallRootDeclaration || root.Node.ID != boot.ID {
		t.Fatalf("declaration root = %+v", root)
	}
	containing := index.ResolveRoot("main.go", 9, "example.com/chain.load")
	if containing.State != DirectCallRootResolved || containing.Binding != DirectCallRootContainingFunction {
		t.Fatalf("containing root = %+v", containing)
	}
	if unresolved := index.ResolveRoot("main.go", 9, "example.com/chain.persist"); unresolved.State != DirectCallRootUnresolved {
		t.Fatalf("unrelated symbol root = %+v", unresolved)
	}
	if module, ok := index.Module(boot.ModuleID); !ok || module.Path != "example.com/chain" {
		t.Fatalf("module lookup = %+v, %v", module, ok)
	}

	requireDirectCallEdge(t, index, "example.com/chain.main", "example.com/chain.boot", DirectCallSynchronous, 1)
	requireDirectCallEdge(t, index, "example.com/chain.boot", "example.com/chain.load", DirectCallSynchronous, 1)
	persistEdge := requireDirectCallEdge(
		t, index, "example.com/chain.load", "example.com/chain.persist", DirectCallSynchronous, 2,
	)
	if persistEdge.RepresentativeCallsite.Path != "main.go" || persistEdge.RepresentativeCallsite.Line != 9 {
		t.Fatalf("representative callsite = %+v, want first exact witness", persistEdge.RepresentativeCallsite)
	}
	load := requireDirectCallNode(t, index, "example.com/chain.load")
	if incoming, outgoing := index.Incoming(load.ID), index.Outgoing(load.ID); len(incoming) != 1 || len(outgoing) != 1 {
		t.Fatalf("load adjacency incoming=%+v outgoing=%+v", incoming, outgoing)
	}
	requireDirectCallEdge(t, index, "example.com/chain.schedule", "example.com/chain.persist", DirectCallGoroutine, 1)
	requireDirectCallEdge(t, index, "example.com/chain.schedule", "example.com/chain.persist", DirectCallDeferred, 1)

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal Result: %v", err)
	}
	if strings.Contains(string(encoded), "direct_call_index") || strings.Contains(string(encoded), index.SHA256) {
		t.Fatal("Result JSON persisted the in-memory direct-call index")
	}
}

func TestDirectCallIndexSnapshotOwnsAllMutableStorage(t *testing.T) {
	source := DirectCallIndex{
		Version: DirectCallIndexVersion,
		State:   DirectCallIndexReady,
		Scenario: Scenario{
			ID: "scenario", GOOS: "test-os", GOARCH: "test-arch", Tags: []string{"tag"},
		},
		Modules: []DirectCallModule{{ID: "module", Path: "example.com/module", Directory: "."}},
		Nodes: []DirectCallNode{
			{ID: "caller", Symbol: Symbol{ID: "example.com/module.caller", EquivalentIDs: []string{"caller-alias"}}},
			{ID: "callee", Symbol: Symbol{ID: "example.com/module.callee", EquivalentIDs: []string{"callee-alias"}}},
		},
		Edges: []DirectCallEdge{{
			ID: "edge", CallerID: "caller", CalleeID: "callee", Invocation: DirectCallSynchronous,
		}},
		Frontiers: []DirectCallNodeFrontier{{CallerID: "caller", ExternalCalleesExcluded: 1}},
	}
	source.initializeLookups()
	snapshot := source.Snapshot()
	if !reflect.DeepEqual(snapshot.Scenario, source.Scenario) ||
		!reflect.DeepEqual(snapshot.Modules, source.Modules) ||
		!reflect.DeepEqual(snapshot.Nodes, source.Nodes) ||
		!reflect.DeepEqual(snapshot.Edges, source.Edges) ||
		!reflect.DeepEqual(snapshot.Frontiers, source.Frontiers) {
		t.Fatalf("Snapshot() = %#v, want exact value copy of %#v", snapshot, source)
	}

	snapshot.Scenario.Tags[0] = "changed-tag"
	snapshot.Modules[0].Path = "changed/module"
	snapshot.Nodes[0].Symbol.EquivalentIDs[0] = "changed-alias"
	snapshot.Edges[0].CallerID = "changed-caller"
	snapshot.Frontiers[0].ExternalCalleesExcluded = 99
	snapshot.nodeLookup["caller"] = 1
	snapshot.moduleLookup["module"] = 99
	snapshot.incomingLookup["callee"][0] = 99
	snapshot.outgoingLookup["caller"][0] = 99
	snapshot.frontierLookup["caller"] = 99

	if source.Scenario.Tags[0] != "tag" || source.Modules[0].Path != "example.com/module" ||
		source.Nodes[0].Symbol.EquivalentIDs[0] != "caller-alias" ||
		source.Edges[0].CallerID != "caller" || source.Frontiers[0].ExternalCalleesExcluded != 1 {
		t.Fatalf("Snapshot mutation changed source public storage: %#v", source)
	}
	if source.nodeLookup["caller"] != 0 || source.moduleLookup["module"] != 0 ||
		source.incomingLookup["callee"][0] != 0 || source.outgoingLookup["caller"][0] != 0 ||
		source.frontierLookup["caller"] != 0 {
		t.Fatalf("Snapshot mutation changed source lookup storage: %#v", source)
	}
}

func TestDirectCallIndexExcludesInterfaceDispatchCandidates(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/dynamic\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import "fmt"

type Runner interface { Run() }
type implementation struct{}
func (implementation) Run() {}

func main() { start(func() {}) }

func start(callback func()) {
	var runner Runner = implementation{}
	runner.Run()
	callback()
	fmt.Println("external")
}
`)

	result, err := Analyze(DefaultOptions(repository))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	index := requireReadyDirectCallIndex(t, result)
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	requireDirectCallEdge(t, index, "example.com/dynamic.main", "example.com/dynamic.start", DirectCallSynchronous, 1)
	if index.Coverage.DynamicInvokesExcluded == 0 {
		t.Fatalf("coverage = %+v, want the interface invoke accounted as excluded", index.Coverage)
	}
	for _, edge := range index.Edges {
		callee := requireDirectCallNodeByID(t, index, edge.CalleeID)
		if strings.HasSuffix(callee.Symbol.ID, ").Run") || strings.HasSuffix(callee.Symbol.ID, ".Run") {
			t.Fatalf("interface implementation candidate entered exact index: %+v", edge)
		}
	}
	start := requireDirectCallNode(t, index, "example.com/dynamic.start")
	frontier, ok := index.Frontier(start.ID)
	if !ok {
		t.Fatal("start frontier is absent")
	}
	if frontier.CallerID != start.ID || frontier.DynamicInvokesExcluded != 1 ||
		frontier.NonStaticCallsExcluded != 1 || frontier.ExternalCalleesExcluded != 1 {
		t.Fatalf("start frontier = %+v, want one closed call in every class", frontier)
	}
	if _, ok := index.Frontier("unknown"); ok {
		t.Fatal("unknown caller returned a frontier")
	}
	encoded, err := json.Marshal(frontier)
	if err != nil {
		t.Fatalf("marshal frontier: %v", err)
	}
	if strings.Contains(string(encoded), "fmt") || strings.Contains(string(encoded), "main.go") {
		t.Fatalf("frontier leaked target or source detail: %s", encoded)
	}

	t.Run("validation rejects unknown zero and duplicate callers", func(t *testing.T) {
		unknown := index
		unknown.Frontiers = append(append([]DirectCallNodeFrontier(nil), index.Frontiers...), DirectCallNodeFrontier{
			CallerID: "unknown", DynamicInvokesExcluded: 1,
		})
		sort.Slice(unknown.Frontiers, func(i, j int) bool {
			return unknown.Frontiers[i].CallerID < unknown.Frontiers[j].CallerID
		})
		unknown.SHA256, _ = directCallIndexSHA256(unknown)
		if err := unknown.Validate(); err == nil || !strings.Contains(err.Error(), "unknown caller") {
			t.Fatalf("unknown caller validation = %v", err)
		}

		zero := index
		zero.Frontiers = append([]DirectCallNodeFrontier(nil), index.Frontiers...)
		for position := range zero.Frontiers {
			if zero.Frontiers[position].CallerID == start.ID {
				zero.Frontiers[position] = DirectCallNodeFrontier{CallerID: start.ID}
			}
		}
		zero.SHA256, _ = directCallIndexSHA256(zero)
		if err := zero.Validate(); err == nil || !strings.Contains(err.Error(), "invalid frontier") {
			t.Fatalf("zero frontier validation = %v", err)
		}

		duplicate := index
		duplicate.Frontiers = append(append([]DirectCallNodeFrontier(nil), index.Frontiers...), frontier)
		sort.Slice(duplicate.Frontiers, func(i, j int) bool {
			return duplicate.Frontiers[i].CallerID < duplicate.Frontiers[j].CallerID
		})
		duplicate.SHA256, _ = directCallIndexSHA256(duplicate)
		if err := duplicate.Validate(); err == nil || !strings.Contains(err.Error(), "not unique canonical order") {
			t.Fatalf("duplicate frontier validation = %v", err)
		}
	})
}

func TestDirectCallIndexSameNamePackagesScenarioAndDigestAreStable(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/same\n\ngo 1.25\n")
	for _, directory := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}
	writeFixtureFile(t, filepath.Join(repository, "a", "a.go"), `package a

func Run() { finish() }
func finish() {}
`)
	writeFixtureFile(t, filepath.Join(repository, "b", "b.go"), `package b

func Run() { finish() }
func finish() {}
`)
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import (
	"example.com/same/a"
	"example.com/same/b"
	"fmt"
)

type runner interface { Run() }
type localRunner struct{}
func (localRunner) Run() {}

func main() {
	a.Run()
	b.Run()
	var current runner = localRunner{}
	current.Run()
	invoke(func() {})
}

func invoke(callback func()) {
	callback()
	fmt.Println("external")
}
`)

	firstOptions := DefaultOptions(repository)
	firstOptions.BuildTags = []string{"zeta", "alpha"}
	firstResult, err := Analyze(firstOptions)
	if err != nil {
		t.Fatalf("first Analyze: %v", err)
	}
	first := requireReadyDirectCallIndex(t, firstResult)

	secondOptions := DefaultOptions(repository)
	secondOptions.BuildTags = []string{"alpha", "zeta"}
	secondResult, err := Analyze(secondOptions)
	if err != nil {
		t.Fatalf("second Analyze: %v", err)
	}
	second := requireReadyDirectCallIndex(t, secondResult)

	if err := first.Validate(); err != nil {
		t.Fatalf("first Validate: %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second Validate: %v", err)
	}
	if first.SHA256 != second.SHA256 || !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical index changed under tag permutation:\nfirst  %s\nsecond %s", first.SHA256, second.SHA256)
	}
	if len(first.Frontiers) == 0 {
		t.Fatal("permutation fixture did not retain caller frontier accounting")
	}
	if !reflect.DeepEqual(first.Scenario.Tags, []string{"alpha", "zeta"}) {
		t.Fatalf("scenario tags = %v", first.Scenario.Tags)
	}

	aRun := requireDirectCallNode(t, first, "example.com/same/a.Run")
	bRun := requireDirectCallNode(t, first, "example.com/same/b.Run")
	if aRun.ID == bRun.ID || aRun.Package == bRun.Package {
		t.Fatalf("same-name package nodes collapsed: a=%+v b=%+v", aRun, bRun)
	}
	requireDirectCallEdge(t, first, "example.com/same/a.Run", "example.com/same/a.finish", DirectCallSynchronous, 1)
	requireDirectCallEdge(t, first, "example.com/same/b.Run", "example.com/same/b.finish", DirectCallSynchronous, 1)
}

func TestDirectCallIndexUnavailableStateRetainsNoPartialGraph(t *testing.T) {
	builder := newDirectCallIndexBuilderWithLimits(Scenario{
		ID: scenarioID(runtime.GOOS, runtime.GOARCH, nil), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
	}, 1, 1)
	builder.modules["partial"] = DirectCallModule{ID: "partial"}
	builder.nodes["partial"] = DirectCallNode{ID: "partial"}
	builder.edges["partial"] = DirectCallEdge{ID: "partial"}
	builder.frontiers["partial"] = DirectCallNodeFrontier{CallerID: "partial", DynamicInvokesExcluded: 1}
	builder.close(DirectCallIndexClosedNodeLimit)
	index := builder.finish()
	if index.State != DirectCallIndexUnavailable || index.ClosedReason != DirectCallIndexClosedNodeLimit {
		t.Fatalf("closed index = %+v", index)
	}
	if len(index.Modules) != 0 || len(index.Nodes) != 0 || len(index.Edges) != 0 || len(index.Frontiers) != 0 {
		t.Fatalf("closed index retained partial graph: %+v", index)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate closed index: %v", err)
	}
}

func TestUnavailableDirectCallIndexIsCanonicalAndEmpty(t *testing.T) {
	t.Parallel()

	first := UnavailableDirectCallIndex()
	second := UnavailableDirectCallIndex()
	if err := first.Validate(); err != nil {
		t.Fatalf("UnavailableDirectCallIndex: %v", err)
	}
	if first.State != DirectCallIndexUnavailable ||
		first.ClosedReason != DirectCallIndexClosedSSAUnavailable ||
		len(first.Nodes) != 0 || len(first.Edges) != 0 || first.SHA256 == "" {
		t.Fatalf("closed index = %#v", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("closed index is not deterministic:\nfirst  %#v\nsecond %#v", first, second)
	}
}

func TestDirectCallIndexResolveRootUsesOnlyExactAliasesAndLookupsStayImmutable(t *testing.T) {
	scenario := Scenario{
		ID: scenarioID(runtime.GOOS, runtime.GOARCH, nil), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Tags: []string{},
	}
	module := DirectCallModule{Path: "example.com/alias", Directory: "."}
	module.ID = stableDirectCallID("direct-module", module.Path, module.Directory)
	declaration := Location{Path: "worker.go", Line: 10, Column: 6}
	node := DirectCallNode{
		Symbol: Symbol{
			ID: "example.com/alias.Boot", EquivalentIDs: []string{"producer-owned/Boot"},
			Package: "example.com/alias", Name: "Boot", Location: declaration,
		},
		Package: "example.com/alias", ModuleID: module.ID, ScenarioID: scenario.ID,
		Declaration: declaration,
		Body: DirectCallBodyRange{
			Start: Location{Path: "worker.go", Line: 10, Column: 1},
			End:   Location{Path: "worker.go", Line: 20, Column: 2},
		},
	}
	node.ID = stableDirectCallNodeID(node)
	index := DirectCallIndex{
		Version: DirectCallIndexVersion, State: DirectCallIndexReady, Scenario: scenario,
		Modules: []DirectCallModule{module}, Nodes: []DirectCallNode{node}, Edges: []DirectCallEdge{},
		Coverage: DirectCallIndexCoverage{ModulesIndexed: 1, NodesConsidered: 1, NodesIndexed: 1},
	}
	index.SHA256, _ = directCallIndexSHA256(index)
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate alias fixture: %v", err)
	}

	alias := index.ResolveRoot("worker.go", 10, "producer-owned/Boot")
	if alias.State != DirectCallRootResolved || alias.Binding != DirectCallRootDeclaration || alias.Node.ID != node.ID {
		t.Fatalf("exact alias resolution = %+v", alias)
	}
	for _, fuzzy := range []string{"Boot", "owned/Boot", "example.com/alias"} {
		if resolution := index.ResolveRoot("worker.go", 10, fuzzy); resolution.State != DirectCallRootUnresolved {
			t.Fatalf("non-exact alias %q resolved as %+v", fuzzy, resolution)
		}
	}

	// A cache-less copy exercises the scan fallback. Lookup methods must not
	// mutate value equality or the canonical JSON/SHA material.
	cacheless := index
	cacheless.nodeLookup = nil
	cacheless.moduleLookup = nil
	cacheless.incomingLookup = nil
	cacheless.outgoingLookup = nil
	cacheless.frontierLookup = nil
	before := cacheless
	beforeJSON, err := json.Marshal(cacheless)
	if err != nil {
		t.Fatalf("marshal before lookups: %v", err)
	}
	if _, ok := cacheless.Node(node.ID); !ok {
		t.Fatal("cache-less node lookup failed")
	}
	if _, ok := cacheless.Module(module.ID); !ok {
		t.Fatal("cache-less module lookup failed")
	}
	if incoming, outgoing := cacheless.Incoming(node.ID), cacheless.Outgoing(node.ID); len(incoming) != 0 || len(outgoing) != 0 {
		t.Fatalf("cache-less empty adjacency changed: incoming=%v outgoing=%v", incoming, outgoing)
	}
	if _, ok := cacheless.Frontier(node.ID); ok {
		t.Fatal("cache-less node with no exclusions returned a frontier")
	}
	afterJSON, err := json.Marshal(cacheless)
	if err != nil {
		t.Fatalf("marshal after lookups: %v", err)
	}
	if !reflect.DeepEqual(before, cacheless) || !reflect.DeepEqual(beforeJSON, afterJSON) || cacheless.SHA256 != before.SHA256 {
		t.Fatal("lookup calls mutated value or digest material")
	}
	if err := cacheless.Validate(); err != nil {
		t.Fatalf("Validate after lookups: %v", err)
	}

	// The same exact producer alias on two exact nodes is ambiguous; the backend
	// never chooses one by name, suffix, or slice order.
	ambiguous := index
	second := node
	second.Symbol.ID = "example.com/alias.AlternateBoot"
	second.ID = stableDirectCallNodeID(second)
	ambiguous.Nodes = append(append([]DirectCallNode(nil), ambiguous.Nodes...), second)
	sort.Slice(ambiguous.Nodes, func(i, j int) bool {
		return directCallNodeKey(ambiguous.Nodes[i]) < directCallNodeKey(ambiguous.Nodes[j])
	})
	ambiguous.Coverage.NodesConsidered = 2
	ambiguous.Coverage.NodesIndexed = 2
	ambiguous.nodeLookup = nil
	ambiguous.moduleLookup = nil
	ambiguous.incomingLookup = nil
	ambiguous.outgoingLookup = nil
	ambiguous.SHA256, _ = directCallIndexSHA256(ambiguous)
	if err := ambiguous.Validate(); err != nil {
		t.Fatalf("Validate ambiguous fixture: %v", err)
	}
	if resolution := ambiguous.ResolveRoot("worker.go", 10, "producer-owned/Boot"); resolution.State != DirectCallRootAmbiguous {
		t.Fatalf("duplicate exact alias resolution = %+v, want ambiguous", resolution)
	}
}

func requireReadyDirectCallIndex(t *testing.T, result Result) DirectCallIndex {
	t.Helper()
	if result.DirectCallIndex == nil {
		t.Fatal("DirectCallIndex is nil")
	}
	index := *result.DirectCallIndex
	if index.State != DirectCallIndexReady || index.ClosedReason != "" {
		t.Fatalf("DirectCallIndex = %+v, want ready", index)
	}
	return index
}

func requireDirectCallNode(t *testing.T, index DirectCallIndex, symbol string) DirectCallNode {
	t.Helper()
	for _, node := range index.Nodes {
		if node.Symbol.ID == symbol {
			return node
		}
	}
	t.Fatalf("direct-call node %q not found in %+v", symbol, index.Nodes)
	return DirectCallNode{}
}

func requireDirectCallNodeByID(t *testing.T, index DirectCallIndex, id string) DirectCallNode {
	t.Helper()
	for _, node := range index.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("direct-call node id %q not found", id)
	return DirectCallNode{}
}

func requireDirectCallEdge(
	t *testing.T,
	index DirectCallIndex,
	callerSymbol, calleeSymbol string,
	invocation DirectCallInvocation,
	witnessCount int,
) DirectCallEdge {
	t.Helper()
	caller := requireDirectCallNode(t, index, callerSymbol)
	callee := requireDirectCallNode(t, index, calleeSymbol)
	for _, edge := range index.Edges {
		if edge.CallerID == caller.ID && edge.CalleeID == callee.ID && edge.Invocation == invocation {
			if edge.WitnessCount != witnessCount {
				t.Fatalf("edge %s -> %s witness count = %d, want %d", callerSymbol, calleeSymbol, edge.WitnessCount, witnessCount)
			}
			return edge
		}
	}
	t.Fatalf("direct-call edge %s -> %s (%s) not found in %+v", callerSymbol, calleeSymbol, invocation, index.Edges)
	return DirectCallEdge{}
}
