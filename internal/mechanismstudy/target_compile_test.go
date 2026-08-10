package mechanismstudy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestCompileTargetedRepomapStartsAtSelectedMainAndCrossesStudyReading(t *testing.T) {
	const module = "example.com/repomap"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"cmd/repomap/main.go": `package main
import "example.com/repomap/internal/app"
func main() { app.RunDevUI() }
`,
		"cmd/quality/main.go": `package main
import "example.com/repomap/internal/quality"
func main() { quality.Evaluate() }
`,
		"internal/app/app.go": `package app
import "example.com/repomap/internal/report"
func RunDevUI() { report.ReadRunDir() }
`,
		"internal/quality/quality.go": `package quality
import "example.com/repomap/internal/report"
func Evaluate() { report.ReadRunDir() }
`,
		"internal/report/report.go": `package report
func ReadRunDir() { readRunDir() }
func readRunDir() { decode() }
func decode() { render() }
func render() { write() }
func write() {}
`,
	})
	target := exactTargetForPackage(t, index, module, module+"/cmd/repomap")
	reading := requireNodeBySymbol(t, index, module+"/internal/report.ReadRunDir")
	compilation := compileTargetedReading(t, index, target, reading)
	card := compilation.Cards[0]
	authority := compilation.authority[card.Ref]

	if compilation.TargetTrailVersion != TargetTrailVersion || compilation.AnalysisTargetRef != target.Ref ||
		compilation.TargetRootsSHA256 == "" {
		t.Fatalf("target identity missing from compilation: %+v", compilation)
	}
	if len(card.TargetRootRefs) != 1 {
		t.Fatalf("provider-visible target roots = %v, want one exact main ref", card.TargetRootRefs)
	}
	if _, exact := authority.targetRootRefs[card.TargetRootRefs[0]]; !exact {
		t.Fatalf("provider-visible target root is not exact private authority: card=%v authority=%v", card.TargetRootRefs, authority.targetRootRefs)
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 1 {
		t.Fatalf("PlanRequestBatches: batches=%d err=%v", len(plan.Batches), err)
	}
	wire := plan.Batches[0].WireJSON
	if !strings.Contains(wire, `"target_root_refs":["`+card.TargetRootRefs[0]+`"]`) {
		t.Fatalf("target roots are absent from provider wire: %s", wire)
	}
	for _, private := range []string{module, "cmd/repomap/main.go", module + "/cmd/repomap.main"} {
		if strings.Contains(wire, private) {
			t.Fatalf("target-root wire leaked private identity %q: %s", private, wire)
		}
	}
	prompt, err := BuildPrompt(plan.Batches[0])
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	for _, requirement := range []string{
		"target_root_refs",
		"include every advertised connector edge",
		"local suffix",
		"exactly one branch",
		"Never union alternative continuations",
	} {
		if !strings.Contains(prompt.System, requirement) {
			t.Fatalf("target-root prompt misses %q: %s", requirement, prompt.System)
		}
	}
	for name, roots := range map[string][]string{
		"private identity": {module + "/cmd/repomap.main"},
		"unknown node":     {"n999"},
		"duplicate node":   {card.TargetRootRefs[0], card.TargetRootRefs[0]},
	} {
		t.Run("reject target roots "+name, func(t *testing.T) {
			request := plan.Batches[0].Request
			request.Cards = append([]Card(nil), request.Cards...)
			request.Cards[0] = copyCard(request.Cards[0])
			request.Cards[0].TargetRootRefs = append([]string(nil), roots...)
			if err := request.Validate(); err == nil {
				t.Fatalf("Request.Validate accepted target roots %v", roots)
			}
		})
	}
	for _, node := range authority.nodeByRef {
		if node.Symbol.ID == module+"/cmd/quality.main" || node.Symbol.ID == module+"/internal/quality.Evaluate" {
			t.Fatalf("off-target quality executable entered selected product card: %s", node.Symbol.ID)
		}
	}

	mainSymbol := module + "/cmd/repomap.main"
	path := []string{
		exactEdgeRef(t, authority, mainSymbol, module+"/internal/app.RunDevUI"),
		exactEdgeRef(t, authority, module+"/internal/app.RunDevUI", reading.Symbol.ID),
		exactEdgeRef(t, authority, reading.Symbol.ID, module+"/internal/report.readRunDir"),
	}
	accepted := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: path}})
	if accepted.State != OutcomeMechanism || len(accepted.Mechanisms) != 1 {
		t.Fatalf("target-rooted candidate = %+v", accepted)
	}
	mechanism := accepted.Mechanisms[0]
	if _, targetRoot := authority.targetRootRefs[mechanism.NodeRefs[0]]; !targetRoot ||
		len(mechanism.ReadingRefs) != 1 {
		t.Fatalf("restored path is not target-rooted through a Study reading: %+v", mechanism)
	}

	notRooted := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: path[1:]}})
	if notRooted.State != OutcomePrepared || len(notRooted.Issues) != 1 ||
		notRooted.Issues[0].Code != IssueNotTargetRooted {
		t.Fatalf("suffix path escaped target-root validation: %+v", notRooted)
	}
}

func TestCompileTargetedMobyDockerdExcludesProxyAndPluginMains(t *testing.T) {
	const module = "example.com/moby"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"cmd/dockerd/main.go": `package main
import "example.com/moby/daemon"
func main() { daemon.Start() }
`,
		"cmd/docker-proxy/main.go": `package main
import "example.com/moby/proxy"
func main() { proxy.Run() }
`,
		"cmd/plugin/main.go": `package main
import "example.com/moby/plugin"
func main() { plugin.Run() }
`,
		"daemon/daemon.go": `package daemon
func Start() { boot() }
func boot() { serve() }
func serve() {}
`,
		"proxy/proxy.go": `package proxy
func Run() { forward() }
func forward() {}
`,
		"plugin/plugin.go": `package plugin
func Run() { load() }
func load() {}
`,
	})
	target := exactTargetForPackage(t, index, module, module+"/cmd/dockerd")
	reading := requireNodeBySymbol(t, index, module+"/daemon.Start")
	compilation := compileTargetedReading(t, index, target, reading)
	for _, node := range compilation.authority["t1"].nodeByRef {
		if strings.Contains(node.Symbol.ID, "/cmd/docker-proxy.") ||
			strings.Contains(node.Symbol.ID, "/cmd/plugin.") ||
			strings.Contains(node.Symbol.ID, "/proxy.") || strings.Contains(node.Symbol.ID, "/plugin.") {
			t.Fatalf("off-target Moby executable entered dockerd card: %s", node.Symbol.ID)
		}
	}
}

func TestCompileTargetedLibraryUsesExactExportedAPIRootWithoutSyntheticMain(t *testing.T) {
	const module = "example.com/telebot"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"bot.go": `package telebot
func NewBot() { configure() }
func configure() { connect() }
func connect() {}
func privateHelper() { connect() }
`,
	})
	target := exactTargetForPackage(t, index, module, module)
	if target.Kind != analysistarget.KindLibraryPackage {
		t.Fatalf("target kind = %s, want library", target.Kind)
	}
	reading := requireNodeBySymbol(t, index, module+".configure")
	compilation := compileTargetedReading(t, index, target, reading)
	authority := compilation.authority["t1"]
	if len(authority.targetRootRefs) != 1 {
		t.Fatalf("library target roots = %#v, want exact exported NewBot only", authority.targetRootRefs)
	}
	for ref := range authority.targetRootRefs {
		if got := authority.nodeByRef[ref].Symbol.ID; got != module+".NewBot" {
			t.Fatalf("library root = %s, want NewBot", got)
		}
	}
	for _, node := range authority.nodeByRef {
		if node.Symbol.Name == "main" {
			t.Fatalf("library trail invented main: %+v", node)
		}
	}
	accepted := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: []string{
		exactEdgeRef(t, authority, module+".NewBot", module+".configure"),
		exactEdgeRef(t, authority, module+".configure", module+".connect"),
	}}})
	if accepted.State != OutcomeMechanism {
		t.Fatalf("exact library API trail = %+v", accepted)
	}
}

func TestCompileTargetedSparseChainUsesEightEdgeTrail(t *testing.T) {
	const module = "example.com/sparse"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"main.go": `package main
func main() { a() }
func a() { b() }
func b() { c() }
func c() { d() }
func d() { e() }
func e() { f() }
func f() { g() }
func g() { h() }
func h() { beyond() }
func beyond() {}
`,
	})
	target := exactTargetForPackage(t, index, module, module)
	reading := requireNodeBySymbol(t, index, module+".b")
	compilation := compileTargetedReading(t, index, target, reading)
	card := compilation.Cards[0]
	authority := compilation.authority["t1"]
	edges := []string{
		exactEdgeRef(t, authority, module+".main", module+".a"),
		exactEdgeRef(t, authority, module+".a", module+".b"),
		exactEdgeRef(t, authority, module+".b", module+".c"),
		exactEdgeRef(t, authority, module+".c", module+".d"),
		exactEdgeRef(t, authority, module+".d", module+".e"),
		exactEdgeRef(t, authority, module+".e", module+".f"),
		exactEdgeRef(t, authority, module+".f", module+".g"),
		exactEdgeRef(t, authority, module+".g", module+".h"),
	}
	if len(card.Edges) != MaxEdgesPerMechanism || frontierCount(card, FrontierDepthBound) == 0 {
		t.Fatalf("sparse target trail edges=%d frontier=%+v, want eight plus depth frontier", len(card.Edges), card.Frontier)
	}
	result := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: edges}})
	if result.State != OutcomeMechanism || len(result.Mechanisms[0].EdgeRefs) != MaxEdgesPerMechanism {
		t.Fatalf("eight-edge target trail was not restorable: %+v", result)
	}
	legacy, err := CompileContexts([]ExactContext{{
		Label: "Legacy reading", Question: "What surrounds this reading?",
		Readings: []ExactReading{{
			Label: reading.Symbol.Name, Path: reading.Declaration.Path,
			Line: reading.Declaration.Line, Symbol: reading.Symbol.ID,
		}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts legacy depth: %v", err)
	}
	assertEveryNodeWithinDepth(t, legacy.Cards[0], legacy.Cards[0].Readings[0].RootNodeRef, MaxDepth)
	if len(legacy.Cards[0].Edges) >= len(card.Edges) {
		t.Fatalf("legacy depth-two graph silently inherited target depth: legacy=%d target=%d",
			len(legacy.Cards[0].Edges), len(card.Edges))
	}
}

func TestCompileTargetedAdvertisesEverySmallEqualShortestAlternativeButPublishesOnePath(t *testing.T) {
	const module = "example.com/equal"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"main.go": `package main
func main() { left(); right() }
func left() { reading() }
func right() { reading() }
func reading() { leaf() }
func leaf() {}
`,
	})
	target := exactTargetForPackage(t, index, module, module)
	reading := requireNodeBySymbol(t, index, module+".reading")
	compilation := compileTargetedReading(t, index, target, reading)
	authority := compilation.authority["t1"]
	leftPath := []string{
		exactEdgeRef(t, authority, module+".main", module+".left"),
		exactEdgeRef(t, authority, module+".left", module+".reading"),
	}
	rightPath := []string{
		exactEdgeRef(t, authority, module+".main", module+".right"),
		exactEdgeRef(t, authority, module+".right", module+".reading"),
	}
	for _, ref := range append(append([]string{}, leftPath...), rightPath...) {
		if _, present := authority.edgeByRef[ref]; !present {
			t.Fatalf("equal shortest alternative edge %s was choose-first omitted", ref)
		}
	}
	selected := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: rightPath}})
	if selected.State != OutcomeMechanism || len(selected.Mechanisms) != 1 ||
		len(selected.Mechanisms[0].EdgeRefs) != 2 {
		t.Fatalf("one exact sequential alternative was not accepted: %+v", selected)
	}
	fork := append(append([]string{}, leftPath...), rightPath...)
	rejected := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: fork}})
	if rejected.State != OutcomePrepared || len(rejected.Issues) != 1 ||
		rejected.Issues[0].Code != IssueDisconnected {
		t.Fatalf("branch-shaped candidate became a mechanism: %+v", rejected)
	}
}

func TestCompileTargetedDenseFanoutKeepsTotalContinuationBudget(t *testing.T) {
	const module = "example.com/dense"
	var source strings.Builder
	source.WriteString("package main\nfunc main() { fan() }\nfunc fan() {")
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&source, " branch%d();", i)
	}
	source.WriteString(" }\n")
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&source, "func branch%d() {}\n", i)
	}
	index := analyzeRepositoryFiles(t, module, map[string]string{"main.go": source.String()})
	target := exactTargetForPackage(t, index, module, module)
	reading := requireNodeBySymbol(t, index, module+".fan")
	compilation := compileTargetedReading(t, index, target, reading)
	card := compilation.Cards[0]
	if len(card.Edges) != 1+MaxContinuationEdgesPerReading ||
		len(card.Nodes) > MaxNodesPerCard || len(card.Edges) > MaxEdgesPerCard ||
		frontierCount(card, FrontierShallowBound) < 12 {
		t.Fatalf("dense fanout card nodes=%d edges=%d frontier=%+v", len(card.Nodes), len(card.Edges), card.Frontier)
	}
	for _, mechanism := range allTargetRootedPaths(card, compilation.authority["t1"]) {
		if len(mechanism) > MaxEdgesPerMechanism {
			t.Fatalf("advertised trail exceeds max path depth: %v", mechanism)
		}
	}
}

func TestCompileTargetedWideReadingRootReservesSourceOrderedQualifyingSpine(t *testing.T) {
	const module = "example.com/wide-root"
	var source strings.Builder
	source.WriteString("package main\nfunc main() {")
	for leaf := 1; leaf <= 9; leaf++ {
		fmt.Fprintf(&source, " leaf%d();", leaf)
	}
	// Both late branches are deep enough. The earlier callsite must win even
	// though its declaration is deliberately later in the file.
	source.WriteString(" firstTrail(); secondTrail() }\n")
	for leaf := 1; leaf <= 9; leaf++ {
		fmt.Fprintf(&source, "func leaf%d() {}\n", leaf)
	}
	source.WriteString(`func secondTrail() { secondEnd() }
func secondEnd() {}
func firstTrail() { firstEnd() }
func firstEnd() {}
`)
	index := analyzeRepositoryFiles(t, module, map[string]string{"main.go": source.String()})
	target := exactTargetForPackage(t, index, module, module)
	main := requireNodeBySymbol(t, index, module+".main")
	compilation := compileTargetedReading(t, index, target, main)
	card := compilation.Cards[0]
	authority := compilation.authority[card.Ref]

	if len(card.Edges) != MaxContinuationEdgesPerReading {
		t.Fatalf("wide root retained %d edges, want existing continuation bound %d",
			len(card.Edges), MaxContinuationEdgesPerReading)
	}
	firstPath := []string{
		exactEdgeRef(t, authority, module+".main", module+".firstTrail"),
		exactEdgeRef(t, authority, module+".firstTrail", module+".firstEnd"),
	}
	for _, edge := range authority.edgeByRef {
		caller := authority.nodeByRef[authority.nodeRefByID[edge.CallerID]].Symbol.ID
		callee := authority.nodeByRef[authority.nodeRefByID[edge.CalleeID]].Symbol.ID
		if caller == module+".main" && callee == module+".secondTrail" ||
			caller == module+".secondTrail" && callee == module+".secondEnd" {
			t.Fatalf("later qualifying spine displaced source-order winner: %s -> %s", caller, callee)
		}
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 1 || len(plan.Batches[0].Request.Cards) != 1 {
		t.Fatalf("qualifying two-edge spine produced plan=%+v err=%v", plan, err)
	}
	accepted := resolveTargetedCandidates(t, compilation, []Candidate{{EdgeRefs: firstPath}})
	if accepted.State != OutcomeMechanism || len(accepted.Mechanisms) != 1 ||
		len(accepted.Mechanisms[0].EdgeRefs) != 2 {
		t.Fatalf("reserved two-edge candidate was not exact mechanism: %+v", accepted)
	}

	canonical := collectTargetRootedGraph(index, []string{main.ID}, 0, []string{main.ID})
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal index permutation fixture: %v", err)
	}
	var permuted surfacediscovery.DirectCallIndex
	if err := json.Unmarshal(raw, &permuted); err != nil {
		t.Fatalf("unmarshal index permutation fixture: %v", err)
	}
	for left, right := 0, len(permuted.Nodes)-1; left < right; left, right = left+1, right-1 {
		permuted.Nodes[left], permuted.Nodes[right] = permuted.Nodes[right], permuted.Nodes[left]
	}
	for left, right := 0, len(permuted.Edges)-1; left < right; left, right = left+1, right-1 {
		permuted.Edges[left], permuted.Edges[right] = permuted.Edges[right], permuted.Edges[left]
	}
	permutedSelection := collectTargetRootedGraph(&permuted, []string{main.ID}, 0, []string{main.ID})
	if !reflect.DeepEqual(canonical, permutedSelection) {
		t.Fatalf("raw node/edge permutation changed source-ordered selection:\ncanonical %+v\npermuted %+v",
			canonical, permutedSelection)
	}
}

func TestCompileTargetedWideReadingRootWithoutContinuationDepthPlansNoProviderCall(t *testing.T) {
	const module = "example.com/wide-root-shallow"
	var source strings.Builder
	source.WriteString("package main\nfunc main() {")
	for leaf := 1; leaf <= 12; leaf++ {
		fmt.Fprintf(&source, " leaf%d();", leaf)
	}
	source.WriteString(" }\n")
	for leaf := 1; leaf <= 12; leaf++ {
		fmt.Fprintf(&source, "func leaf%d() {}\n", leaf)
	}
	index := analyzeRepositoryFiles(t, module, map[string]string{"main.go": source.String()})
	target := exactTargetForPackage(t, index, module, module)
	main := requireNodeBySymbol(t, index, module+".main")
	compilation := compileTargetedReading(t, index, target, main)
	card := compilation.Cards[0]
	if len(card.Edges) != MaxContinuationEdgesPerReading {
		t.Fatalf("shallow wide root retained %d edges, want %d", len(card.Edges), MaxContinuationEdgesPerReading)
	}
	for _, path := range allTargetRootedPaths(card, compilation.authority[card.Ref]) {
		if len(path) != 1 {
			t.Fatalf("shallow fixture unexpectedly contains qualifying path: %v", path)
		}
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 0 {
		t.Fatalf("no-depth card planned provider work: plan=%+v err=%v", plan, err)
	}
}

func TestCompileTargetedEqualShortestConnectorsAreAllOrPrepared(t *testing.T) {
	const module = "example.com/ambiguous"
	var source strings.Builder
	source.WriteString("package main\nfunc main() {")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&source, " via%d();", i)
	}
	source.WriteString(" }\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&source, "func via%d() { reading() }\n", i)
	}
	source.WriteString("func reading() { leaf() }\nfunc leaf() {}\n")
	index := analyzeRepositoryFiles(t, module, map[string]string{"main.go": source.String()})
	target := exactTargetForPackage(t, index, module, module)
	reading := requireNodeBySymbol(t, index, module+".reading")
	compilation := compileTargetedReading(t, index, target, reading)
	card := compilation.Cards[0]
	if card.Readings[0].RootNodeRef != "" || len(card.Edges) != 0 ||
		frontierCount(card, FrontierAmbiguousConnector) != 1 {
		t.Fatalf("over-bound equal shortest connectors chose a prefix: %+v", card)
	}
	if batches, err := BuildRequestBatches(compilation); err != nil || len(batches) != 0 {
		t.Fatalf("ambiguous prepared card produced provider request: batches=%d err=%v", len(batches), err)
	}
}

func TestTargetGraphSelectionIsReadingPermutationStable(t *testing.T) {
	const module = "example.com/permutation"
	var source strings.Builder
	source.WriteString("package main\nfunc main() { start() }\nfunc start() { r1(); r2(); r3(); r4() }\n")
	for reading := 1; reading <= 4; reading++ {
		fmt.Fprintf(&source, "func r%d() {", reading)
		for leaf := 1; leaf <= MaxContinuationEdgesPerReading; leaf++ {
			fmt.Fprintf(&source, " r%dLeaf%d();", reading, leaf)
		}
		source.WriteString(" }\n")
		for leaf := 1; leaf <= MaxContinuationEdgesPerReading; leaf++ {
			fmt.Fprintf(&source, "func r%dLeaf%d() {}\n", reading, leaf)
		}
	}
	index := analyzeRepositoryFiles(t, module, map[string]string{"main.go": source.String()})
	main := requireNodeBySymbol(t, index, module+".main")
	readings := make([]string, 0, 4)
	for reading := 1; reading <= 4; reading++ {
		readings = append(readings, requireNodeBySymbol(t, index, fmt.Sprintf("%s.r%d", module, reading)).ID)
	}
	first := collectTargetRootedGraph(index, []string{main.ID}, 0, readings)
	second := collectTargetRootedGraph(index, []string{main.ID}, 0, []string{
		readings[3], readings[1], readings[0], readings[2],
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reading permutation changed target graph:\nfirst  %+v\nsecond %+v", first, second)
	}
	if len(first.admittedReadings) == len(readings) || first.frontier[FrontierShallowBound] == 0 {
		t.Fatalf("fixture did not exercise deterministic bound contention: %+v", first)
	}
}

func TestMissingTargetConnectorReportsConfiguredDepthFrontier(t *testing.T) {
	limited := &surfacediscovery.DirectCallIndex{
		Scope: surfacediscovery.DirectCallIndexScope{
			TargetKind:    surfacediscovery.AnalysisTargetExecutablePackage,
			TargetPackage: "example.com/app", MaxDepth: 10, EdgeLimit: 10_000,
		},
		Coverage: surfacediscovery.DirectCallIndexCoverage{
			DepthBoundRepositoryCallsExcluded: 1,
		},
	}
	if got := missingTargetConnectorReason(limited); got != FrontierDepthBound {
		t.Fatalf("limited missing connector = %q, want %q", got, FrontierDepthBound)
	}
	limited.Coverage.DepthBoundRepositoryCallsExcluded = 0
	if got := missingTargetConnectorReason(limited); got != FrontierTargetUnreachable {
		t.Fatalf("exhausted target graph missing connector = %q, want %q", got, FrontierTargetUnreachable)
	}
	if got := missingTargetConnectorReason(nil); got != FrontierTargetUnreachable {
		t.Fatalf("legacy missing connector = %q, want %q", got, FrontierTargetUnreachable)
	}
}

func TestTargetCompilationFactsRoundTripRestoresPrivateRootAuthority(t *testing.T) {
	const module = "example.com/artifact-target"
	index := analyzeRepositoryFiles(t, module, map[string]string{
		"main.go": `package main
func main() { start() }
func start() { reading() }
func reading() { continueWork() }
func continueWork() {}
`,
	})
	target := exactTargetForPackage(t, index, module, module)
	reading := requireNodeBySymbol(t, index, module+".reading")
	compilation := compileTargetedReading(t, index, target, reading)
	plan, err := PlanRequestBatches(compilation)
	if err != nil {
		t.Fatalf("PlanRequestBatches: %v", err)
	}
	facts, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	restored, err := DecodeFacts(facts)
	if err != nil {
		t.Fatalf("DecodeFacts: %v", err)
	}
	if restored.Compilation.TargetTrailVersion != TargetTrailVersion ||
		restored.Compilation.AnalysisTargetRef != target.Ref ||
		restored.Compilation.TargetRootsSHA256 != compilation.TargetRootsSHA256 ||
		!reflect.DeepEqual(restored.Compilation.Cards, compilation.Cards) ||
		!reflect.DeepEqual(
			restored.Compilation.authority["t1"].targetRootRefs,
			compilation.authority["t1"].targetRootRefs,
		) {
		t.Fatalf("target facts lost private root authority: %+v", restored.Compilation)
	}
	if err := restored.Compilation.Validate(); err != nil {
		t.Fatalf("restored target compilation Validate: %v", err)
	}

	tamperedDigest := *compilation
	tamperedDigest.TargetRootsSHA256 = strings.Repeat("f", 64)
	if err := tamperedDigest.Validate(); err == nil {
		t.Fatal("target roots digest tamper retained catalog authority")
	}
	tamperedRoots := *compilation
	tamperedRoots.authority = make(map[string]cardAuthority, len(compilation.authority))
	for ref, authority := range compilation.authority {
		tamperedRoots.authority[ref] = authority
	}
	authority := tamperedRoots.authority["t1"]
	authority.targetRootRefs = map[string]struct{}{}
	tamperedRoots.authority["t1"] = authority
	if err := tamperedRoots.Validate(); err == nil {
		t.Fatal("target root restoration tamper retained catalog authority")
	}
	tamperedProjection := *compilation
	tamperedProjection.Cards = append([]Card(nil), compilation.Cards...)
	tamperedProjection.Cards[0] = copyCard(compilation.Cards[0])
	tamperedProjection.Cards[0].TargetRootRefs = nil
	if err := tamperedProjection.Validate(); err == nil {
		t.Fatal("target root provider projection tamper retained catalog authority")
	}
}

func compileTargetedReading(
	t *testing.T,
	index *surfacediscovery.DirectCallIndex,
	target analysistarget.Target,
	reading surfacediscovery.DirectCallNode,
) *Compilation {
	t.Helper()
	study := themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: fixtureRevision,
		Cards: []themestudy.ThemeCard{{
			Ordinal: 1, CanonicalID: "targeted-card", FinalTitle: "Targeted trail",
			FinalQuestion: "How does the selected product reach and continue through this reading?",
			Readings: []themestudy.Reading{{
				CanonicalSpanID: "targeted-reading", Label: reading.Symbol.Name,
				Symbol: reading.Symbol.ID, Path: reading.Declaration.Path,
				Line: reading.Declaration.Line, Fit: themestudy.FitDirect,
			}},
		}},
	}
	readingRoots, err := BindStudyReadingRoots(study, index)
	if err != nil {
		t.Fatalf("BindStudyReadingRoots: %v", err)
	}
	targetRoots, err := analysistarget.BindExactRoots(target, index)
	if err != nil {
		t.Fatalf("BindExactRoots: %v", err)
	}
	compilation, err := CompileTargeted(TargetCompileInput{
		Study: study, Index: index, Binding: studyBinding(), ReadingRoots: readingRoots,
		AnalysisTarget: target, TargetRoots: targetRoots,
	})
	if err != nil {
		t.Fatalf("CompileTargeted: %v", err)
	}
	if err := compilation.Validate(); err != nil {
		t.Fatalf("target compilation Validate: %v", err)
	}
	return compilation
}

func exactTargetForPackage(
	t *testing.T,
	index *surfacediscovery.DirectCallIndex,
	modulePath, packagePath string,
) analysistarget.Target {
	t.Helper()
	const moduleID = "test-module"
	packageDirs := make(map[string]string)
	for _, node := range index.Nodes {
		dir := strings.TrimPrefix(node.Package, modulePath)
		dir = strings.TrimPrefix(dir, "/")
		if dir == "" {
			dir = "."
		}
		packageDirs[node.Package] = dir
	}
	facts := gofacts.Facts{Modules: []gofacts.ModuleFact{{
		ID: moduleID, ModulePath: modulePath, ModuleDir: ".", Main: true,
	}}}
	paths := make([]string, 0, len(packageDirs))
	for path := range packageDirs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		facts.Packages = append(facts.Packages, gofacts.PackageFact{
			CanonicalPath: path, Name: filepath.Base(packageDirs[path]), ModuleID: moduleID,
			ModulePath: modulePath, PackageDir: packageDirs[path], Locality: "local",
		})
	}
	for _, node := range index.Nodes {
		if node.Symbol.Name != "main" {
			continue
		}
		facts.EntrypointPackages = append(facts.EntrypointPackages, gofacts.Entrypoint{
			ModulePath: modulePath, ImportPath: node.Package, ModuleDir: ".",
			PackageDir: packageDirs[node.Package], Kind: "binary",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: node.Declaration.Path, Line: node.Declaration.Line,
			}},
		})
	}
	resolution, err := analysistarget.Resolve(facts, analysistarget.Options{Override: packagePath})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("Resolve target %s: resolution=%+v err=%v", packagePath, resolution, err)
	}
	return resolution.Selected.Snapshot()
}

func analyzeRepositoryFiles(t *testing.T, module string, files map[string]string) *surfacediscovery.DirectCallIndex {
	t.Helper()
	repository := t.TempDir()
	files = cloneStringMap(files)
	files["go.mod"] = "module " + module + "\n\ngo 1.25\n"
	for name, contents := range files {
		filename := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	result, err := surfacediscovery.Analyze(surfacediscovery.DefaultOptions(repository))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.DirectCallIndex == nil || result.DirectCallIndex.State != surfacediscovery.DirectCallIndexReady {
		t.Fatalf("direct call index = %+v", result.DirectCallIndex)
	}
	return result.DirectCallIndex
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func exactEdgeRef(t *testing.T, authority cardAuthority, callerSymbol, calleeSymbol string) string {
	t.Helper()
	for ref, edge := range authority.edgeByRef {
		caller := authority.nodeByRef[authority.nodeRefByID[edge.CallerID]]
		callee := authority.nodeByRef[authority.nodeRefByID[edge.CalleeID]]
		if caller.Symbol.ID == callerSymbol && callee.Symbol.ID == calleeSymbol {
			return ref
		}
	}
	t.Fatalf("edge %s -> %s not found", callerSymbol, calleeSymbol)
	return ""
}

func resolveTargetedCandidates(t *testing.T, compilation *Compilation, candidates []Candidate) CardResult {
	t.Helper()
	batches, err := BuildRequestBatches(compilation)
	if err != nil || len(batches) != 1 {
		t.Fatalf("BuildRequestBatches: batches=%d err=%v", len(batches), err)
	}
	batch := batches[0]
	raw, err := json.Marshal(Response{
		Version: ResultVersion, CatalogRef: batch.Request.CatalogRef,
		CatalogSHA256: batch.Request.CatalogSHA256, RequestRef: batch.Request.RequestRef,
		Cards: []ResponseCard{{CardRef: batch.Request.Cards[0].Ref, Mechanisms: candidates}},
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	result, err := ResolveResponse(compilation, batch, raw)
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	return result.Cards[0]
}

func allTargetRootedPaths(card Card, authority cardAuthority) [][]string {
	adjacency := make(map[string][]Edge)
	for _, edge := range card.Edges {
		adjacency[edge.CallerRef] = append(adjacency[edge.CallerRef], edge)
	}
	paths := [][]string{}
	var walk func(string, []string, map[string]struct{})
	walk = func(node string, edges []string, seen map[string]struct{}) {
		if len(adjacency[node]) == 0 {
			paths = append(paths, append([]string(nil), edges...))
			return
		}
		for _, edge := range adjacency[node] {
			if _, cycle := seen[edge.CalleeRef]; cycle {
				continue
			}
			nextSeen := make(map[string]struct{}, len(seen)+1)
			for ref := range seen {
				nextSeen[ref] = struct{}{}
			}
			nextSeen[edge.CalleeRef] = struct{}{}
			walk(edge.CalleeRef, append(edges, edge.Ref), nextSeen)
		}
	}
	for root := range authority.targetRootRefs {
		walk(root, nil, map[string]struct{}{root: {}})
	}
	return paths
}
