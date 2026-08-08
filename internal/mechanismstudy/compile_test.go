package mechanismstudy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

const fixtureRevision = "0123456789abcdef0123456789abcdef01234567"

func TestCompileStudyUsesOnlyExactPrimaryDirectReadings(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	study := themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: fixtureRevision,
		Cards: []themestudy.ThemeCard{{
			Ordinal: 1, CanonicalID: "private-theme-canonical-id",
			FinalTitle: "Startup path", FinalQuestion: "How does the startup path continue?",
			Readings: []themestudy.Reading{
				readingForNode(root, themestudy.FitDirect),
				{Label: "support", Symbol: "example.com/mechanism.side", Path: "main.go", Line: 99, Fit: themestudy.FitSupporting},
			},
			AlternateReadings: []themestudy.Reading{readingForNode(root, themestudy.FitDirect)},
		}},
	}
	binding := studyBinding()
	first, err := Compile(study, index, binding)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	second, err := Compile(study, index, binding)
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	if first.CatalogSHA256 != second.CatalogSHA256 || !reflect.DeepEqual(first.Cards, second.Cards) {
		t.Fatalf("compilation is not deterministic:\nfirst  %s\nsecond %s", first.CatalogSHA256, second.CatalogSHA256)
	}
	if len(first.Cards) != 1 || len(first.Cards[0].Readings) != 1 {
		t.Fatalf("cards/readings = %#v, want one primary direct reading", first.Cards)
	}
	if got := frontierCount(first.Cards[0], FrontierUnsupportedReading); got != 2 {
		t.Fatalf("unsupported reading frontier = %d, want supporting + alternate", got)
	}
	if len(first.Cards[0].Nodes) > MaxNodesPerCard || len(first.Cards[0].Edges) > MaxEdgesPerCard {
		t.Fatalf("card exceeded graph bounds: nodes=%d edges=%d", len(first.Cards[0].Nodes), len(first.Cards[0].Edges))
	}
	batches, err := BuildRequestBatches(first)
	if err != nil {
		t.Fatalf("BuildRequestBatches: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want one", len(batches))
	}
	wire, err := ProviderVisibleJSON(batches[0])
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	for _, forbidden := range []string{"main.go", "example.com/mechanism", "private-theme-canonical-id", root.ID, index.SHA256} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("provider wire leaked %q:\n%s", forbidden, wire)
		}
	}
	for _, required := range []string{`"ref":"t1"`, `"ref":"r1"`, `"ref":"n`, `"ref":"e`, `"catalog_sha256"`} {
		if !strings.Contains(string(wire), required) {
			t.Fatalf("provider wire misses %s:\n%s", required, wire)
		}
	}
}

func TestCompileContextsNeedsNoStudyAndDoesNotSelectARoot(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Selected entry", Question: "What exact work follows this entry?",
		Readings: []ExactReading{{
			Label: "Explicit entry", Path: root.Declaration.Path,
			Line: root.Declaration.Line, Symbol: root.Symbol.ID,
		}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	if compilation.Binding.ContextKind != ContextExplicit || compilation.Binding.StudyThemesSHA256 != "" ||
		compilation.Binding.AtlasStudyCatalogSHA256 != "" {
		t.Fatalf("explicit context fabricated Study authority: %+v", compilation.Binding)
	}
	if len(compilation.Cards) != 1 || len(compilation.Cards[0].Readings) != 1 ||
		compilation.Cards[0].Readings[0].RootNodeRef == "" {
		t.Fatalf("explicit root was not retained exactly: %+v", compilation.Cards)
	}

	wrong, err := CompileContexts([]ExactContext{{
		Label: "Wrong root", Question: "No fuzzy lookup",
		Readings: []ExactReading{{
			Label: "suffix only", Path: root.Declaration.Path,
			Line: root.Declaration.Line, Symbol: "entry",
		}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts wrong exact root: %v", err)
	}
	if wrong.Cards[0].Readings[0].RootNodeRef != "" || frontierCount(wrong.Cards[0], FrontierNoExactFunction) != 1 {
		t.Fatalf("suffix symbol was not closed exactly: %+v", wrong.Cards[0])
	}
	if requests, err := BuildRequestBatches(wrong); err != nil || len(requests) != 0 {
		t.Fatalf("unresolved context produced provider work: requests=%d err=%v", len(requests), err)
	}
}

func TestCompileAccountsDynamicAndExternalFrontierPerSelectedCaller(t *testing.T) {
	index := buildClosedFrontierIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/frontier.entry")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Entry", Question: "What exact work can be followed?",
		Readings: []ExactReading{{
			Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID,
		}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	card := compilation.Cards[0]
	if frontierCount(card, FrontierDynamicInvoke) < 2 {
		t.Fatalf("dynamic frontier = %+v, want interface + function-value calls", card.Frontier)
	}
	if frontierCount(card, FrontierExternalCallee) < 1 {
		t.Fatalf("external frontier = %+v, want selected-caller external call", card.Frontier)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil || len(batches) != 1 {
		t.Fatalf("BuildRequestBatches: batches=%d err=%v", len(batches), err)
	}
	for _, forbidden := range []string{"fmt.Println", "example.com/frontier", "main.go"} {
		if strings.Contains(batches[0].WireJSON, forbidden) {
			t.Fatalf("frontier wire leaked target detail %q: %s", forbidden, batches[0].WireJSON)
		}
	}
}

func TestCompileIsDepthTwoBalancedAndAccountsFrontier(t *testing.T) {
	index := buildFanoutIndex(t)
	contexts := make([]ExactContext, 0, 5)
	for number := 1; number <= 5; number++ {
		root := requireNodeBySymbol(t, index, fmt.Sprintf("example.com/fan.root%d", number))
		contexts = append(contexts, ExactContext{
			Label: fmt.Sprintf("Root %d", number), Question: "Find connected work",
			Readings: []ExactReading{{
				Label: "root", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID,
			}},
		})
	}
	compilation, err := CompileContexts(contexts, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	if len(compilation.Cards) != 5 {
		t.Fatalf("cards = %d, want five explicit contexts", len(compilation.Cards))
	}
	for _, card := range compilation.Cards {
		if len(card.Nodes) > MaxNodesPerCard || len(card.Edges) > MaxEdgesPerCard {
			t.Fatalf("card %s exceeds bounds: nodes=%d edges=%d", card.Ref, len(card.Nodes), len(card.Edges))
		}
		rootRef := card.Readings[0].RootNodeRef
		outgoing := 0
		for _, edge := range card.Edges {
			if edge.CallerRef == rootRef {
				outgoing++
			}
		}
		if outgoing != MaxRootNeighborsPerDirection {
			t.Fatalf("card %s root outgoing = %d, want balanced root cap %d", card.Ref, outgoing, MaxRootNeighborsPerDirection)
		}
		if frontierCount(card, FrontierShallowBound) == 0 || frontierCount(card, FrontierDepthBound) == 0 {
			t.Fatalf("card %s lost bounded frontier: %+v", card.Ref, card.Frontier)
		}
		assertEveryNodeWithinDepth(t, card, rootRef, MaxDepth)
	}
}

func TestCompileSeparatesRootAndContinuationCapsPermutationStably(t *testing.T) {
	firstOrder := []string{"debug", "writeDefaultConfig", "cleanup", "watchConfiguredDirs", "checkRunEnv", "start"}
	secondOrder := append([]string(nil), firstOrder...)
	for left, right := 0, len(secondOrder)-1; left < right; left, right = left+1, right-1 {
		secondOrder[left], secondOrder[right] = secondOrder[right], secondOrder[left]
	}
	firstIndex := buildRootBudgetIndex(t, firstOrder)
	secondIndex := buildRootBudgetIndex(t, secondOrder)
	first := compileRootBudget(t, firstIndex)
	second := compileRootBudget(t, secondIndex)
	if !reflect.DeepEqual(first.Cards, second.Cards) {
		t.Fatalf("source call permutation changed provider card:\nfirst  %+v\nsecond %+v", first.Cards, second.Cards)
	}

	card := first.Cards[0]
	rootRef := card.Readings[0].RootNodeRef
	rootOutgoing := 0
	for _, edge := range card.Edges {
		if edge.CallerRef == rootRef {
			rootOutgoing++
		}
	}
	if rootOutgoing != len(firstOrder) {
		t.Fatalf("root outgoing = %d, want all %d below root cap %d", rootOutgoing, len(firstOrder), MaxRootNeighborsPerDirection)
	}
	requireEdgeByLabels(t, card, "Run", "checkRunEnv")
	requireEdgeByLabels(t, card, "Run", "start")

	root := requireNodeBySymbol(t, firstIndex, "example.com/rootbudget.Run")
	exactOutgoing := firstIndex.Outgoing(root.ID)
	if len(exactOutgoing) != len(firstOrder) {
		t.Fatalf("exact root outgoing = %d, want %d", len(exactOutgoing), len(firstOrder))
	}
	selectedExact := make(map[string]struct{})
	for _, exact := range first.authority[card.Ref].edgeByRef {
		selectedExact[exact.ID] = struct{}{}
	}
	for _, exact := range exactOutgoing[4:6] {
		if _, ok := selectedExact[exact.ID]; !ok {
			t.Fatalf("fifth/sixth exact root edge %s was still trimmed by the former shared cap", exact.ID)
		}
	}

	startEdge := requireEdgeByLabels(t, card, "Run", "start")
	continuations := 0
	for _, edge := range card.Edges {
		if edge.CallerRef == startEdge.CalleeRef {
			continuations++
		}
	}
	if continuations != MaxContinuationNeighborsPerDirection {
		t.Fatalf("start continuations = %d, want cap %d", continuations, MaxContinuationNeighborsPerDirection)
	}
	start := requireNodeBySymbol(t, firstIndex, "example.com/rootbudget.start")
	exactContinuations := firstIndex.Outgoing(start.ID)
	if len(exactContinuations) != 10 {
		t.Fatalf("exact start continuations = %d, want ten", len(exactContinuations))
	}
	for _, exact := range exactContinuations[4:MaxContinuationNeighborsPerDirection] {
		if _, ok := selectedExact[exact.ID]; !ok {
			t.Fatalf("fifth-through-eighth exact continuation %s was trimmed by the former four-neighbor cap", exact.ID)
		}
	}
	for _, exact := range exactContinuations[MaxContinuationNeighborsPerDirection:] {
		if _, ok := selectedExact[exact.ID]; ok {
			t.Fatalf("continuation beyond cap was selected: %s", exact.ID)
		}
	}
	if frontierCount(card, FrontierShallowBound) < 2 {
		t.Fatalf("continuation omissions are not accounted: %+v", card.Frontier)
	}
	if len(card.Nodes) > MaxNodesPerCard || len(card.Edges) > MaxEdgesPerCard {
		t.Fatalf("separate caps exceeded card bounds: nodes=%d edges=%d", len(card.Nodes), len(card.Edges))
	}
	assertEveryNodeWithinDepth(t, card, rootRef, MaxDepth)
}

func TestBuildRequestBatchesEnforcesExactWireCeilings(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	contexts := make([]ExactContext, MaxCards)
	for position := range contexts {
		contexts[position] = ExactContext{
			Label: fmt.Sprintf("Context %d", position+1), Question: "Find the useful connected mechanisms",
			Readings: []ExactReading{{
				Label: "entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID,
			}},
		}
	}
	compilation, err := CompileContexts(contexts, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil {
		t.Fatalf("BuildRequestBatches: %v", err)
	}
	if len(batches) != 2 || len(batches) > MaxProviderCalls {
		t.Fatalf("batches = %d, want two bounded calls", len(batches))
	}
	totalCards := 0
	for _, batch := range batches {
		totalCards += len(batch.Request.Cards)
		if len(batch.Request.Cards) > MaxCardsPerRequest || len(batch.WireJSON) > MaxRequestBytes {
			t.Fatalf("batch exceeds card/byte bound: cards=%d bytes=%d", len(batch.Request.Cards), len(batch.WireJSON))
		}
		nodes, edges := 0, 0
		for _, card := range batch.Request.Cards {
			nodes += len(card.Nodes)
			edges += len(card.Edges)
		}
		if nodes > MaxNodesPerRequest || edges > MaxEdgesPerRequest {
			t.Fatalf("batch exceeds graph bounds: nodes=%d edges=%d", nodes, edges)
		}
	}
	if totalCards != MaxCards {
		t.Fatalf("batched cards = %d, want %d", totalCards, MaxCards)
	}
}

func TestNodeLabelsDisambiguateSameNamesWithoutLeakingImportPaths(t *testing.T) {
	index := buildSameNameIndex(t)
	aRun := requireNodeBySymbol(t, index, "example.com/same/a.Run")
	bRun := requireNodeBySymbol(t, index, "example.com/same/b.Run")
	compilation, err := CompileContexts([]ExactContext{
		{
			Label: "A", Question: "How does A continue?",
			Readings: []ExactReading{{Label: "A Run", Path: aRun.Declaration.Path, Line: aRun.Declaration.Line, Symbol: aRun.Symbol.ID}},
		},
		{
			Label: "B", Question: "How does B continue?",
			Readings: []ExactReading{{Label: "B Run", Path: bRun.Declaration.Path, Line: bRun.Declaration.Line, Symbol: bRun.Symbol.ID}},
		},
	}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil || len(batches) != 1 {
		t.Fatalf("BuildRequestBatches: batches=%d err=%v", len(batches), err)
	}
	wire := batches[0].WireJSON
	for _, expected := range []string{"a · Run", "b · Run"} {
		if !strings.Contains(wire, expected) {
			t.Fatalf("wire misses disambiguating package leaf %q: %s", expected, wire)
		}
	}
	for _, forbidden := range []string{"example.com/same/a", "example.com/same/b", "a/a.go", "b/b.go"} {
		if strings.Contains(wire, forbidden) {
			t.Fatalf("wire leaked import/path %q: %s", forbidden, wire)
		}
	}
}

func TestProviderVisibleRequestSampleIsExactAndContainsNoActionProtocol(t *testing.T) {
	request := Request{
		Version: RequestVersion, PromptVersion: PromptVersion,
		CatalogRef: "mc-aaaaaaaaaaaaaaaa", CatalogSHA256: strings.Repeat("a", 64), RequestRef: "q1",
		Cards: []Card{{
			Ref: "t1", Label: "Startup", Question: "What work follows?",
			Readings: []Reading{{Ref: "r1", Label: "Entry", RootNodeRef: "n1"}},
			Nodes:    []Node{{Ref: "n1", Label: "entry"}, {Ref: "n2", Label: "service"}, {Ref: "n3", Label: "persist"}},
			Edges: []Edge{
				{Ref: "e1", CallerRef: "n1", CalleeRef: "n2", Invocation: surfacediscovery.DirectCallSynchronous, WitnessCount: 1},
				{Ref: "e2", CallerRef: "n2", CalleeRef: "n3", Invocation: surfacediscovery.DirectCallSynchronous, WitnessCount: 1},
			},
		}},
	}
	batch, err := makeRequestBatch(request)
	if err != nil {
		t.Fatalf("makeRequestBatch: %v", err)
	}
	expected := fmt.Sprintf(`{"version":%d,"prompt_version":%q,"catalog_ref":"mc-aaaaaaaaaaaaaaaa","catalog_sha256":"%s","request_ref":"q1","cards":[{"ref":"t1","label":"Startup","question":"What work follows?","readings":[{"ref":"r1","label":"Entry","root_node_ref":"n1"}],"nodes":[{"ref":"n1","label":"entry"},{"ref":"n2","label":"service"},{"ref":"n3","label":"persist"}],"edges":[{"ref":"e1","caller_ref":"n1","callee_ref":"n2","invocation":"synchronous","witness_count":1},{"ref":"e2","caller_ref":"n2","callee_ref":"n3","invocation":"synchronous","witness_count":1}]}]}`, RequestVersion, PromptVersion, strings.Repeat("a", 64))
	if batch.WireJSON != expected {
		t.Fatalf("provider request changed:\n got %s\nwant %s", batch.WireJSON, expected)
	}
	t.Logf("provider-visible request sample: %s", batch.WireJSON)
	for _, forbidden := range []string{"selector", "next_action", "deepen", "root_choice", "canonical_id", "path", "symbol", "source"} {
		if strings.Contains(batch.WireJSON, forbidden) {
			t.Fatalf("provider request contains forbidden protocol/data %q: %s", forbidden, batch.WireJSON)
		}
	}
	prompt, err := BuildPrompt(batch)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(prompt.User, batch.WireJSON) || strings.Contains(prompt.System, "next_action") {
		t.Fatalf("prompt does not preserve direct plural contract: %+v", prompt)
	}
	tampered := batch
	tampered.WireJSON = `{"private":"not the exact request"}`
	tampered.WireSHA256 = sha256Hex([]byte(tampered.WireJSON))
	if _, err := BuildPrompt(tampered); err == nil {
		t.Fatal("prompt accepted wire bytes that do not restore the request")
	}
}

func TestCompilationCatalogFailsClosedAfterPublicGraphMutation(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Entry", Question: "What follows?",
		Readings: []ExactReading{{Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	compilation.Cards[0].Nodes[0].Label = "mutated"
	if err := compilation.Validate(); err == nil {
		t.Fatal("mutated public graph retained the old catalog identity")
	}
}

func TestSafeLabelsDropLocatorLikeAndCanonicalText(t *testing.T) {
	tests := []string{
		"Read internal/server/main.go next",
		"Start with main.go",
		"Open handler:42",
		"0123456789abcdef0123456789abcdef",
	}
	for _, value := range tests {
		if got := safeLabel(value, 80, "safe fallback"); got != "safe fallback" {
			t.Fatalf("safeLabel(%q) = %q, want fallback", value, got)
		}
	}
	if got := safeLabel("server · Run", 80, "fallback"); got != "server · Run" {
		t.Fatalf("safe semantic label changed: %q", got)
	}
}

func TestOneHopRemainsPreparedWithoutProviderRequest(t *testing.T) {
	index := buildOneHopIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/onehop.entry")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Entry", Question: "What follows?",
		Readings: []ExactReading{{Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil {
		t.Fatalf("BuildRequestBatches: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("one-hop evidence produced %d provider requests", len(batches))
	}
	prepared, err := PreparedCards(compilation)
	if err != nil {
		t.Fatalf("PreparedCards: %v", err)
	}
	if len(prepared) != 1 || prepared[0].State != OutcomePrepared || len(prepared[0].Mechanisms) != 0 {
		t.Fatalf("one-hop state = %+v", prepared)
	}
}

func readingForNode(node surfacediscovery.DirectCallNode, fit themestudy.FitClass) themestudy.Reading {
	return themestudy.Reading{
		Label: node.Symbol.Name, Symbol: node.Symbol.ID,
		Path: node.Declaration.Path, Line: node.Declaration.Line, Fit: fit,
	}
}

func studyBinding() Binding {
	return Binding{
		StudyThemesSHA256:         strings.Repeat("a", 64),
		AtlasStudyCatalogSHA256:   strings.Repeat("b", 64),
		RepositoryRevision:        fixtureRevision,
		RepositoryFreshnessSHA256: strings.Repeat("c", 64),
	}
}

func repositoryBinding() RepositoryBinding {
	return RepositoryBinding{
		RepositoryRevision:        fixtureRevision,
		RepositoryFreshnessSHA256: strings.Repeat("c", 64),
	}
}

func buildChainIndex(t *testing.T) *surfacediscovery.DirectCallIndex {
	t.Helper()
	return analyzeSource(t, "example.com/mechanism", `package main

func main() { entry() }
func entry() { service(); side() }
func service() { persist() }
func persist() { flush() }
func flush() {}
func side() { sideLeaf() }
func sideLeaf() {}
`)
}

func buildOneHopIndex(t *testing.T) *surfacediscovery.DirectCallIndex {
	t.Helper()
	return analyzeSource(t, "example.com/onehop", `package main

func main() {}
func entry() { leaf() }
func leaf() {}
`)
}

func buildClosedFrontierIndex(t *testing.T) *surfacediscovery.DirectCallIndex {
	t.Helper()
	return analyzeSource(t, "example.com/frontier", `package main

import "fmt"

type Runner interface { Run() }

func main() {}
func entry(runner Runner, callback func()) {
	runner.Run()
	callback()
	fmt.Println("frontier")
	service()
}
func service() { persist() }
func persist() {}
`)
}

func buildFanoutIndex(t *testing.T) *surfacediscovery.DirectCallIndex {
	t.Helper()
	var source strings.Builder
	source.WriteString("package main\n\nfunc main() {}\n")
	for root := 1; root <= 5; root++ {
		fmt.Fprintf(&source, "func root%d() {", root)
		for branch := 1; branch <= 10; branch++ {
			fmt.Fprintf(&source, " r%db%d();", root, branch)
		}
		source.WriteString(" }\n")
		for branch := 1; branch <= 10; branch++ {
			fmt.Fprintf(&source, "func r%db%d() { r%db%dleaf() }\n", root, branch, root, branch)
			fmt.Fprintf(&source, "func r%db%dleaf() { r%db%ddeep() }\n", root, branch, root, branch)
			fmt.Fprintf(&source, "func r%db%ddeep() {}\n", root, branch)
		}
	}
	return analyzeSource(t, "example.com/fan", source.String())
}

func buildRootBudgetIndex(t *testing.T, rootCalls []string) *surfacediscovery.DirectCallIndex {
	t.Helper()
	var source strings.Builder
	source.WriteString("package main\n\nfunc main() {}\nfunc Run() {")
	for _, name := range rootCalls {
		fmt.Fprintf(&source, " %s();", name)
	}
	source.WriteString(" }\n")
	source.WriteString("func debug() {}\nfunc writeDefaultConfig() {}\nfunc cleanup() {}\nfunc watchConfiguredDirs() {}\nfunc checkRunEnv() {}\n")
	source.WriteString("func start() { startOne(); startTwo(); startThree(); startFour(); startFive(); startSix(); startSeven(); startEight(); startNine(); startTen() }\n")
	source.WriteString("func startOne() {}\nfunc startTwo() {}\nfunc startThree() {}\n")
	source.WriteString("func startFour() {}\nfunc startFive() {}\nfunc startSix() {}\nfunc startSeven() {}\n")
	source.WriteString("func startEight() {}\nfunc startNine() {}\nfunc startTen() {}\n")
	return analyzeSource(t, "example.com/rootbudget", source.String())
}

func compileRootBudget(t *testing.T, index *surfacediscovery.DirectCallIndex) *Compilation {
	t.Helper()
	root := requireNodeBySymbol(t, index, "example.com/rootbudget.Run")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Run loop", Question: "How does Run reach its core loop?",
		Readings: []ExactReading{{
			Label: "Run", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID,
		}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts root budget: %v", err)
	}
	return compilation
}

func buildSameNameIndex(t *testing.T) *surfacediscovery.DirectCallIndex {
	t.Helper()
	repository := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/same\n\ngo 1.25\n",
		"main.go": `package main

import (
	"example.com/same/a"
	"example.com/same/b"
)

func main() { a.Run(); b.Run() }
`,
		"a/a.go": `package a

func Run() { next() }
func next() { leaf() }
func leaf() {}
`,
		"b/b.go": `package b

func Run() { next() }
func next() { leaf() }
func leaf() {}
`,
	}
	for name, contents := range files {
		path := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
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

func analyzeSource(t *testing.T, module, source string) *surfacediscovery.DirectCallIndex {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module "+module+"\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
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

func requireNodeBySymbol(t *testing.T, index *surfacediscovery.DirectCallIndex, symbol string) surfacediscovery.DirectCallNode {
	t.Helper()
	for _, node := range index.Nodes {
		if node.Symbol.ID == symbol {
			return node
		}
	}
	t.Fatalf("node %q not found", symbol)
	return surfacediscovery.DirectCallNode{}
}

func frontierCount(card Card, reason FrontierReason) int {
	for _, frontier := range card.Frontier {
		if frontier.Reason == reason {
			return frontier.Count
		}
	}
	return 0
}

func assertEveryNodeWithinDepth(t *testing.T, card Card, root string, maxDepth int) {
	t.Helper()
	outgoing := make(map[string][]string)
	incoming := make(map[string][]string)
	for _, edge := range card.Edges {
		outgoing[edge.CallerRef] = append(outgoing[edge.CallerRef], edge.CalleeRef)
		incoming[edge.CalleeRef] = append(incoming[edge.CalleeRef], edge.CallerRef)
	}
	outDistance := graphDistance(root, outgoing)
	inDistance := graphDistance(root, incoming)
	for _, node := range card.Nodes {
		out, outOK := outDistance[node.Ref]
		in, inOK := inDistance[node.Ref]
		if (!outOK || out > maxDepth) && (!inOK || in > maxDepth) {
			t.Fatalf("node %s has directed distances out=%d/%v in=%d/%v, want one <= %d", node.Ref, out, outOK, in, inOK, maxDepth)
		}
	}
}

func graphDistance(root string, adjacency map[string][]string) map[string]int {
	distance := map[string]int{root: 0}
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if _, seen := distance[next]; seen {
				continue
			}
			distance[next] = distance[current] + 1
			queue = append(queue, next)
		}
	}
	return distance
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return encoded
}
