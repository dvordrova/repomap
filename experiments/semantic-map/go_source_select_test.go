package semanticmap

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	caddyGoSelectionRevision = "873fac5fc094fe538d0c477509127bb321d51a32"
	caddyGoSelectionQuestion = "How does Caddy replace a running configuration without discarding the old one before the new applications are ready, and where can the handoff fail?"
)

func TestGoSelectionQueriesAreNeutralAndBounded(t *testing.T) {
	queries, content, err := goSelectionQueries(caddyGoSelectionQuestion)
	if err != nil {
		t.Fatal(err)
	}
	wantQueries := []GoSelectionQuery{
		{ID: "q1", Text: "caddy replac"},
		{ID: "q2", Text: "caddy run"},
		{ID: "q3", Text: "caddy config"},
	}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatalf("queries = %#v, want %#v", queries, wantQueries)
	}
	wantContent := []string{
		"caddy", "replac", "run", "config", "discar", "applic", "ready", "handof", "fail",
	}
	if !reflect.DeepEqual(content, wantContent) {
		t.Fatalf("content = %#v, want %#v", content, wantContent)
	}
	if len(queries) > goSelectionMaxQueryTerms {
		t.Fatalf("query count = %d, limit %d", len(queries), goSelectionMaxQueryTerms)
	}
}

func TestGoSourceSelectionIsDeterministicWithoutCuratedInputs(t *testing.T) {
	repoPath, revision := makeGoSelectionFixture(t)
	adapter := newFakeGoSelectionAnalyzer()
	opts := GoSourceSelectionOptions{
		RepositoryPath:   repoPath,
		ExpectedRevision: revision,
		Question:         "How does sample keep running and replace the prepared state?",
	}

	firstTrace, firstPacket, err := selectGoQuestionSources(
		context.Background(),
		opts,
		adapter,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondTrace, secondPacket, err := selectGoQuestionSources(
		context.Background(),
		opts,
		newFakeGoSelectionAnalyzer(),
	)
	if err != nil {
		t.Fatal(err)
	}
	firstTraceJSON := mustEncodeGoSelection(t, firstTrace)
	secondTraceJSON := mustEncodeGoSelection(t, secondTrace)
	firstPacketJSON := mustEncodeGoSelection(t, firstPacket)
	secondPacketJSON := mustEncodeGoSelection(t, secondPacket)
	if !bytes.Equal(firstTraceJSON, secondTraceJSON) ||
		!bytes.Equal(firstPacketJSON, secondPacketJSON) {
		t.Fatal("selector output is not byte-identical across equivalent runs")
	}
	if bytes.Contains(firstTraceJSON, []byte(repoPath)) ||
		bytes.Contains(firstPacketJSON, []byte(repoPath)) {
		t.Fatal("encoded output contains an absolute repository path")
	}
	assertGoSelectionNames(t, firstTrace, "run", "prepare", "swap", "stopOld")
	assertGoSelectionEdge(t, firstTrace, "run", "prepare")
	assertGoSelectionEdge(t, firstTrace, "prepare", "swap")
	assertGoSelectionEdge(t, firstTrace, "prepare", "stopOld")
	validateGoSelectionArtifacts(t, firstTraceJSON, firstPacketJSON)
}

func TestGoAnchoredSelectionPreservesSeedsWithoutWorkspaceSearch(t *testing.T) {
	repoPath, revision := makeGoSelectionFixture(t)
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]GoTopicDeclaration, len(inventory.Declarations))
	for _, declaration := range inventory.Declarations {
		byName[declaration.Name] = declaration
	}
	anchors := []GoTopicDeclaration{byName["prepare"], byName["run"]}
	adapter := newFakeGoSelectionAnalyzer()
	adapter.mislabelCallTargets = true
	trace, packet, err := selectGoAnchoredQuestionSources(
		context.Background(),
		GoSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "How does the prepared state become the running state?",
		},
		anchors,
		adapter,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSeedIDs := []string{anchors[0].ID, anchors[1].ID}
	if !reflect.DeepEqual(trace.SeedDeclarationIDs, wantSeedIDs) ||
		!reflect.DeepEqual(packet.SeedDeclarationIDs, wantSeedIDs) {
		t.Fatalf(
			"seed IDs = %#v / %#v, want %#v",
			trace.SeedDeclarationIDs,
			packet.SeedDeclarationIDs,
			wantSeedIDs,
		)
	}
	if trace.Coverage != "anchor_seeded_non_exhaustive" ||
		packet.Coverage != trace.Coverage ||
		len(trace.Candidates) != 0 ||
		adapter.analyzeCalls != 0 {
		t.Fatalf(
			"anchored contract = coverage %q/%q, candidates %d, workspace calls %d",
			trace.Coverage,
			packet.Coverage,
			len(trace.Candidates),
			adapter.analyzeCalls,
		)
	}
	if strings.Contains(strings.Join(trace.Provenance.Operations, ","), "workspace_symbol") {
		t.Fatalf("anchored provenance = %#v", trace.Provenance.Operations)
	}
	assertGoSelectionNames(t, trace, "prepare", "run")
	physicalSymbols := make(map[string]struct{}, len(trace.SelectedSymbols))
	for _, symbol := range trace.SelectedSymbols {
		key := fmt.Sprintf(
			"%s:%d:%d:%s",
			symbol.Path,
			symbol.StartLine,
			symbol.StartColumn,
			goSelectionCallableName(symbol.Name),
		)
		if _, duplicate := physicalSymbols[key]; duplicate {
			t.Fatalf("anchored selection duplicated physical declaration %q", key)
		}
		physicalSymbols[key] = struct{}{}
	}
	seedReasons := make(map[string]struct{}, len(wantSeedIDs))
	for _, sourceSlice := range packet.SourceSlices {
		for _, reason := range sourceSlice.SelectionReasonIDs {
			if strings.HasPrefix(reason, "anchor:") {
				seedReasons[strings.TrimPrefix(reason, "anchor:")] = struct{}{}
			}
		}
	}
	for _, id := range wantSeedIDs {
		if _, ok := seedReasons[id]; !ok {
			t.Errorf("packet does not retain seed reason %q", id)
		}
	}
	if _, _, err := selectGoAnchoredQuestionSources(
		context.Background(),
		GoSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "How does the prepared state become the running state?",
		},
		[]GoTopicDeclaration{anchors[0], anchors[0]},
		newFakeGoSelectionAnalyzer(),
	); err == nil || !strings.Contains(err.Error(), "duplicate anchor") {
		t.Fatalf("duplicate anchor error = %v", err)
	}
}

func TestGoSourceSelectionRejectsUntrackedCheckout(t *testing.T) {
	repoPath, revision := makeGoSelectionFixture(t)
	if err := os.WriteFile(
		filepath.Join(repoPath, "untracked.go"),
		[]byte("package sample\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	_, _, err := selectGoQuestionSources(
		context.Background(),
		GoSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "How does sample replace its running state?",
		},
		newFakeGoSelectionAnalyzer(),
	)
	if err == nil || !strings.Contains(err.Error(), "untracked repository state") {
		t.Fatalf("untracked checkout error = %v", err)
	}
}

func TestGoSelectionTruncatesAnalyzerHitsPerQuery(t *testing.T) {
	adapter := manyHitGoSelectionAnalyzer{fakeGoSelectionAnalyzer: newFakeGoSelectionAnalyzer()}
	candidates, _, err := discoverGoSelectionCandidates(
		context.Background(),
		adapter,
		"/bounded/repository",
		[]GoSelectionQuery{{ID: "q1", Text: "sample run"}},
		[]string{"sample", "run"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != goSelectionMaxHitsPerTerm {
		t.Fatalf(
			"candidates = %d, want deterministic per-query cap %d",
			len(candidates),
			goSelectionMaxHitsPerTerm,
		)
	}
}

func TestGoSelectionRejectsNestedAnalyzerOverflow(t *testing.T) {
	graph := evidence.NewGraph("/bounded/repository", "sample run")
	graph.AddEntity(evidence.Entity{
		ID:   "query:sample-run",
		Kind: evidence.EntityQuery,
		Name: "sample run",
	})
	provenance := make([]evidence.Provenance, goSelectionMaxProvenance+1)
	for index := range provenance {
		provenance[index] = evidence.Provenance{
			Provider:  "gopls",
			Version:   "v0.test",
			Operation: "workspace_symbol",
		}
	}
	graph.Relations = append(graph.Relations, evidence.Relation{
		From:       "query:sample-run",
		To:         "query:sample-run",
		Kind:       evidence.RelationMatchesQuery,
		Certainty:  evidence.CertaintyStatic,
		Provenance: provenance,
	})
	if err := validateGoSelectionAnalyzerGraph(
		graph,
		goSelectionMaxDiscoveryItems,
	); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("provenance overflow error = %v", err)
	}

	result := analysis.LocationResolution{
		Candidates: make(
			[]analysis.LocationCandidate,
			goSelectionMaxResolveResults+1,
		),
	}
	if err := validateGoSelectionResolution(
		result,
	); err == nil || !strings.Contains(err.Error(), "candidates") {
		t.Fatalf("resolution overflow error = %v", err)
	}
}

func TestRecordedCaddyGoSelection(t *testing.T) {
	traceJSON := readBoundedFile(t, "caddy.go-selection.json", 64<<10)
	packetJSON := readBoundedFile(t, "caddy.auto-source-slices.json", 32<<10)
	trace := decodeStrict[GoSourceSelectionTrace](t, traceJSON)
	packet := decodeStrict[GoSourceSelectionPacket](t, packetJSON)

	if got := mustEncodeGoSelection(t, trace); !bytes.Equal(got, traceJSON) {
		t.Fatal("recorded Caddy trace is not canonically encoded")
	}
	if got := mustEncodeGoSelection(t, packet); !bytes.Equal(got, packetJSON) {
		t.Fatal("recorded Caddy packet is not canonically encoded")
	}
	validateGoSelectionArtifacts(t, traceJSON, packetJSON)
	if trace.Repository.Revision != caddyGoSelectionRevision ||
		trace.Question != caddyGoSelectionQuestion {
		t.Fatal("recorded Caddy selector input changed")
	}

	assertGoSelectionNames(
		t,
		trace,
		"changeConfig",
		"unsyncedDecodeAndRun",
		"run",
		"provisionContext",
		"unsyncedStop",
	)
	assertGoSelectionEdge(t, trace, "changeConfig", "unsyncedDecodeAndRun")
	assertGoSelectionEdge(t, trace, "unsyncedDecodeAndRun", "run")
	assertGoSelectionEdge(t, trace, "run", "provisionContext")
	assertGoSelectionEdge(t, trace, "unsyncedDecodeAndRun", "unsyncedStop")

	packetText := ""
	for _, sourceSlice := range packet.SourceSlices {
		packetText += sourceSlice.Text
	}
	for _, snippet := range []string{
		"currentCtx = ctx",
		"unsyncedStop(oldCtx)",
		"a.Start()",
		"ctx.App(appName)",
	} {
		if !strings.Contains(packetText, snippet) {
			t.Errorf("packet does not preserve mechanism evidence %q", snippet)
		}
	}
}

func TestGoSelectorImplementationHasNoCaddyOracle(t *testing.T) {
	source, err := os.ReadFile("go_source_select.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"changeConfig",
		"unsyncedDecodeAndRun",
		"unsyncedStop",
		"provisionContext",
		"caddy.source-slices.json",
		"adapter-structure",
		"/Users/",
	} {
		if bytes.Contains(source, []byte(forbidden)) {
			t.Errorf("selector implementation contains curated oracle text %q", forbidden)
		}
	}
}

func TestGoSourcePacketReprojectsEdgesAfterSourceBudgetPruning(t *testing.T) {
	repoPath, revision, selected, edges := makeGoSelectionBudgetFixture(t)
	packet, retained, retainedEdges, err := buildGoSelectionPacket(
		repoPath,
		GoSelectionRepository{Name: "budget", Revision: revision},
		"How does the running state move through the pipeline?",
		selected,
		edges,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 2 || len(packet.SourceSlices) != 2 {
		t.Fatalf(
			"retained symbols/slices = %d/%d, want 2/2 after byte pruning",
			len(retained),
			len(packet.SourceSlices),
		)
	}
	if len(retainedEdges) != 1 ||
		retainedEdges[0].from != goSelectionEntityKey(retained[0].entity) ||
		retainedEdges[0].to != goSelectionEntityKey(retained[1].entity) {
		t.Fatalf("retained edges = %#v, want only root -> child", retainedEdges)
	}
	if got := packet.SourceSlices[1].SelectionReasonIDs; !reflect.DeepEqual(got, []string{"e1"}) {
		t.Fatalf("child reasons = %#v, want reprojected e1", got)
	}
	for _, edge := range retainedEdges {
		if strings.Contains(edge.from, "leaf") || strings.Contains(edge.to, "leaf") {
			t.Fatal("budget-pruned leaf remains in an exact call")
		}
	}
}

func TestLiveCaddyGoSelection(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_CADDY_REPO")
	if repoPath == "" {
		t.Skip("set REPOMAP_CADDY_REPO to replay the pinned live selector")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	opts := GoSourceSelectionOptions{
		RepositoryPath:   repoPath,
		ExpectedRevision: caddyGoSelectionRevision,
		Question:         caddyGoSelectionQuestion,
		GoplsBinary:      os.Getenv("REPOMAP_GOPLS_BINARY"),
	}

	trace, packet, err := SelectGoQuestionSources(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	traceJSON := mustEncodeGoSelection(t, trace)
	packetJSON := mustEncodeGoSelection(t, packet)
	validateGoSelectionArtifacts(t, traceJSON, packetJSON)
	assertGoSelectionNames(
		t,
		trace,
		"changeConfig",
		"unsyncedDecodeAndRun",
		"run",
		"provisionContext",
		"unsyncedStop",
	)

	if outputDir := os.Getenv("REPOMAP_GO_SELECTION_OUTPUT_DIR"); outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(outputDir, "caddy.go-selection.json"),
			traceJSON,
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(outputDir, "caddy.auto-source-slices.json"),
			packetJSON,
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func validateGoSelectionArtifacts(t *testing.T, traceJSON, packetJSON []byte) {
	t.Helper()
	trace := decodeStrict[GoSourceSelectionTrace](t, traceJSON)
	packet := decodeStrict[GoSourceSelectionPacket](t, packetJSON)
	if trace.Version != goSelectionVersion || packet.Version != goSelectionVersion {
		t.Fatal("selector artifact version changed")
	}
	if trace.Repository != packet.Repository ||
		trace.Question != packet.Question ||
		trace.Coverage != packet.Coverage {
		t.Fatal("trace and packet inputs or coverage differ")
	}
	if trace.Coverage != "selected_symbol_targets_only_non_exhaustive" {
		t.Fatalf("coverage = %q", trace.Coverage)
	}
	if len(trace.QueryTerms) == 0 || len(trace.QueryTerms) > goSelectionMaxQueryTerms {
		t.Fatalf("query terms = %d", len(trace.QueryTerms))
	}
	if len(trace.Candidates) == 0 || len(trace.Candidates) > goSelectionMaxCandidates {
		t.Fatalf("candidates = %d", len(trace.Candidates))
	}
	if len(trace.SelectedSymbols) == 0 || len(trace.SelectedSymbols) > goSelectionMaxSlices {
		t.Fatalf("selected symbols = %d", len(trace.SelectedSymbols))
	}
	if len(trace.ExactCalls) > goSelectionMaxCallEndpoints {
		t.Fatalf("exact calls = %d", len(trace.ExactCalls))
	}
	if len(packet.SourceSlices) == 0 || len(packet.SourceSlices) > goSelectionMaxSlices {
		t.Fatalf("source slices = %d", len(packet.SourceSlices))
	}
	if bytes.Contains(traceJSON, []byte("/Users/")) ||
		bytes.Contains(packetJSON, []byte("/Users/")) {
		t.Fatal("artifact contains an absolute home path")
	}

	queryIDs := make(map[string]struct{}, goSelectionMaxQueryTerms)
	for index, query := range trace.QueryTerms {
		wantID := fmt.Sprintf("q%d", index+1)
		if query.ID != wantID || query.Text == "" || len(query.Text) > goSelectionMaxQuestionBytes {
			t.Fatalf("query_terms[%d] = %#v", index, query)
		}
		queryIDs[query.ID] = struct{}{}
	}
	previousCandidate := 1 << 30
	hitsByQuery := make(map[string]int, goSelectionMaxQueryTerms)
	for _, candidate := range trace.Candidates {
		if !goSelectionCanonicalPath(candidate.Path) ||
			candidate.Line <= 0 ||
			candidate.Column <= 0 ||
			len(candidate.QueryTermIDs) == 0 {
			t.Fatalf("invalid candidate %#v", candidate)
		}
		if candidate.Score > previousCandidate {
			t.Fatal("candidates are not sorted by descending score")
		}
		previousCandidate = candidate.Score
		for _, queryID := range candidate.QueryTermIDs {
			if _, ok := queryIDs[queryID]; !ok {
				t.Fatalf("candidate references unknown query %q", queryID)
			}
			hitsByQuery[queryID]++
			if hitsByQuery[queryID] > goSelectionMaxHitsPerTerm {
				t.Fatalf(
					"query %q has %d hits, limit %d",
					queryID,
					hitsByQuery[queryID],
					goSelectionMaxHitsPerTerm,
				)
			}
		}
	}

	symbols := make(map[string]GoSelectionSymbol, goSelectionMaxSlices)
	for _, symbol := range trace.SelectedSymbols {
		if symbol.ID == "" ||
			!goSelectionCanonicalPath(symbol.Path) ||
			symbol.StartLine <= 0 ||
			symbol.EndLine < symbol.StartLine ||
			symbol.EndLine-symbol.StartLine+1 > goSelectionMaxSliceLines ||
			len(symbol.SelectionReasonIDs) == 0 {
			t.Fatalf("invalid selected symbol %#v", symbol)
		}
		if symbol.Distance > trace.Limits.ExpansionHops {
			t.Fatalf(
				"selected symbol %q distance = %d, hop limit %d",
				symbol.ID,
				symbol.Distance,
				trace.Limits.ExpansionHops,
			)
		}
		if _, duplicate := symbols[symbol.ID]; duplicate {
			t.Fatalf("duplicate selected symbol %q", symbol.ID)
		}
		symbols[symbol.ID] = symbol
	}
	callIDs := make(map[string]struct{}, goSelectionMaxCallEndpoints)
	for index, call := range trace.ExactCalls {
		wantID := fmt.Sprintf("e%d", index+1)
		if call.ID != wantID ||
			!goSelectionCanonicalPath(call.Path) ||
			call.StartLine <= 0 ||
			call.StartColumn <= 0 {
			t.Fatalf("invalid exact call %#v", call)
		}
		if _, ok := symbols[call.CallerSymbolID]; !ok {
			t.Fatalf("call %s has unknown caller", call.ID)
		}
		if _, ok := symbols[call.CalleeSymbolID]; !ok {
			t.Fatalf("call %s has unknown callee", call.ID)
		}
		callIDs[call.ID] = struct{}{}
	}

	totalBytes := 0
	sliceSymbols := make(map[string]struct{}, goSelectionMaxSlices)
	for _, sourceSlice := range packet.SourceSlices {
		if !goSelectionCanonicalPath(sourceSlice.Path) ||
			sourceSlice.StartLine <= 0 ||
			sourceSlice.EndLine < sourceSlice.StartLine ||
			sourceSlice.EndLine-sourceSlice.StartLine+1 > goSelectionMaxSliceLines ||
			sourceSlice.Text == "" ||
			len(sourceSlice.SelectionReasonIDs) == 0 {
			t.Fatalf("invalid source slice %#v", sourceSlice)
		}
		symbol, ok := symbols[sourceSlice.EnclosingSymbolID]
		if !ok ||
			symbol.Path != sourceSlice.Path ||
			symbol.StartLine != sourceSlice.StartLine ||
			symbol.EndLine != sourceSlice.EndLine {
			t.Fatalf("source slice does not match selected symbol %q", sourceSlice.EnclosingSymbolID)
		}
		lineCount := len(strings.Split(strings.TrimSuffix(sourceSlice.Text, "\n"), "\n"))
		if lineCount != sourceSlice.EndLine-sourceSlice.StartLine+1 {
			t.Fatalf("slice %s text line count = %d", sourceSlice.Path, lineCount)
		}
		for _, reasonID := range sourceSlice.SelectionReasonIDs {
			if reasonID != "seed" {
				if _, ok := callIDs[reasonID]; !ok {
					t.Fatalf("slice references unknown reason %q", reasonID)
				}
			}
		}
		totalBytes += len(sourceSlice.Text)
		sliceSymbols[sourceSlice.EnclosingSymbolID] = struct{}{}
	}
	if totalBytes > goSelectionMaxSourceBytes {
		t.Fatalf("source bytes = %d, limit %d", totalBytes, goSelectionMaxSourceBytes)
	}
	if len(sliceSymbols) != len(symbols) {
		t.Fatal("trace selected symbols and packet slices differ")
	}
}

func assertGoSelectionNames(t *testing.T, trace GoSourceSelectionTrace, names ...string) {
	t.Helper()
	known := make(map[string]struct{}, goSelectionMaxSlices)
	for _, symbol := range trace.SelectedSymbols {
		known[goSelectionCallableName(symbol.Name)] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			t.Errorf("selected symbols omit %q; got %v", name, sortedGoSelectionNames(trace))
		}
	}
}

func assertGoSelectionEdge(
	t *testing.T,
	trace GoSourceSelectionTrace,
	callerName string,
	calleeName string,
) {
	t.Helper()
	names := make(map[string]string, goSelectionMaxSlices)
	for _, symbol := range trace.SelectedSymbols {
		names[symbol.ID] = goSelectionCallableName(symbol.Name)
	}
	for _, call := range trace.ExactCalls {
		if names[call.CallerSymbolID] == callerName && names[call.CalleeSymbolID] == calleeName {
			return
		}
	}
	t.Errorf("exact calls omit %s -> %s", callerName, calleeName)
}

func mustEncodeGoSelection(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := EncodeGoSourceSelection(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func makeGoSelectionFixture(t *testing.T) (string, string) {
	t.Helper()
	repoPath := t.TempDir()
	source := strings.Join([]string{
		"package sample",
		"",
		"func run() {",
		"\tprepare()",
		"}",
		"",
		"func prepare() {",
		"\tswap()",
		"\tstopOld()",
		"}",
		"",
		"func swap() {}",
		"",
		"func stopOld() {}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoPath, "flow.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "selector@example.invalid"},
		{"config", "user.name", "Selector Test"},
		{"add", "flow.go"},
		{"commit", "-qm", "fixture"},
	} {
		command := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	command := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return repoPath, strings.TrimSpace(string(output))
}

func makeGoSelectionBudgetFixture(
	t *testing.T,
) (string, string, []goSelectionNode, []goSelectionEdge) {
	t.Helper()
	repoPath := t.TempDir()
	var source strings.Builder
	line := 0
	writeLine := func(value string) {
		source.WriteString(value)
		source.WriteByte('\n')
		line++
	}
	writeLine("package budget")
	writeLine("")
	entities := make(map[string]evidence.Entity, 3)
	for _, name := range []string{"root", "child", "leaf"} {
		start := line + 1
		writeLine("func " + name + "() {")
		for index := 0; index < 110; index++ {
			writeLine("\t// " + strings.Repeat(name+"-", 18))
		}
		writeLine("}")
		writeLine("")
		location := evidence.Location{
			Path:      "flow.go",
			Line:      start,
			Column:    6,
			EndLine:   line - 1,
			EndColumn: 2,
		}
		entities[name] = evidence.Entity{
			ID:       fmt.Sprintf("function:flow.go:%d:6:%s", start, name),
			Kind:     evidence.EntityFunction,
			Name:     name,
			Language: "go",
			Scope:    evidence.SourceScopeRepository,
			Location: &location,
		}
	}
	if err := os.WriteFile(
		filepath.Join(repoPath, "flow.go"),
		[]byte(source.String()),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	revision := commitGoSelectionFixture(t, repoPath, "flow.go")
	root := entities["root"]
	child := entities["child"]
	leaf := entities["leaf"]
	firstEdge := goSelectionEdge{
		from: goSelectionEntityKey(root),
		to:   goSelectionEntityKey(child),
		location: evidence.Location{
			Path: "flow.go", Line: root.Location.Line + 1, Column: 2,
			EndLine: root.Location.Line + 1, EndColumn: 7,
		},
	}
	secondEdge := goSelectionEdge{
		from: goSelectionEntityKey(child),
		to:   goSelectionEntityKey(leaf),
		location: evidence.Location{
			Path: "flow.go", Line: child.Location.Line + 1, Column: 2,
			EndLine: child.Location.Line + 1, EndColumn: 6,
		},
	}
	edges := []goSelectionEdge{firstEdge, secondEdge}
	sortGoSelectionEdges(edges)
	return repoPath, revision, []goSelectionNode{
		{entity: root, root: true},
		{
			entity:        child,
			distance:      1,
			parentKey:     goSelectionEntityKey(root),
			parentEdgeKey: goSelectionEdgeKey(firstEdge),
		},
		{
			entity:        leaf,
			distance:      2,
			parentKey:     goSelectionEntityKey(child),
			parentEdgeKey: goSelectionEdgeKey(secondEdge),
		},
	}, edges
}

func commitGoSelectionFixture(t *testing.T, repoPath string, files ...string) string {
	t.Helper()
	commands := [][]string{
		{"init", "-q"},
		{"config", "user.email", "selector@example.invalid"},
		{"config", "user.name", "Selector Test"},
		append([]string{"add"}, files...),
		{"commit", "-qm", "fixture"},
	}
	for _, args := range commands {
		command := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	command := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

type fakeGoSelectionAnalyzer struct {
	entities            map[string]evidence.Entity
	analyzeCalls        int
	exactCalls          int
	resolveCalls        int
	mislabelCallTargets bool
}

type manyHitGoSelectionAnalyzer struct {
	*fakeGoSelectionAnalyzer
}

func newFakeGoSelectionAnalyzer() *fakeGoSelectionAnalyzer {
	location := func(name string, line, endLine int) evidence.Entity {
		sourceLocation := evidence.Location{
			Path:      "flow.go",
			Line:      line,
			Column:    6,
			EndLine:   endLine,
			EndColumn: 2,
		}
		return evidence.Entity{
			ID:       fmt.Sprintf("function:flow.go:%d:6:%s", line, name),
			Kind:     evidence.EntityFunction,
			Name:     name,
			Language: "go",
			Scope:    evidence.SourceScopeRepository,
			Location: &sourceLocation,
		}
	}
	return &fakeGoSelectionAnalyzer{entities: map[string]evidence.Entity{
		"run":     location("run", 3, 5),
		"prepare": location("prepare", 7, 10),
		"swap":    location("swap", 12, 12),
		"stopOld": location("stopOld", 14, 14),
	}}
}

func (fake *fakeGoSelectionAnalyzer) Analyze(
	_ context.Context,
	request analysis.Request,
) (evidence.Graph, error) {
	fake.analyzeCalls++
	graph := evidence.NewGraph(request.RepoPath, request.Query)
	query := evidence.Entity{ID: "query:" + request.Query, Kind: evidence.EntityQuery, Name: request.Query}
	graph.AddEntity(query)
	names := []string{"prepare", "run"}
	if strings.Contains(request.Query, "run") {
		names = []string{"run", "prepare"}
	}
	for _, name := range names {
		entity := fake.entities[name]
		graph.AddEntity(entity)
		graph.AddRelation(fakeGoSelectionRelation(query.ID, entity.ID, evidence.RelationMatchesQuery, nil))
	}
	graph.Sort()
	return graph, nil
}

func (fake manyHitGoSelectionAnalyzer) Analyze(
	_ context.Context,
	request analysis.Request,
) (evidence.Graph, error) {
	graph := evidence.NewGraph(request.RepoPath, request.Query)
	query := evidence.Entity{
		ID:   "query:" + request.Query,
		Kind: evidence.EntityQuery,
		Name: request.Query,
	}
	graph.AddEntity(query)
	for index := 0; index < goSelectionMaxHitsPerTerm+1; index++ {
		name := fmt.Sprintf("runCandidate%02d", index)
		location := evidence.Location{
			Path:   "flow.go",
			Line:   index + 1,
			Column: 1,
		}
		entity := evidence.Entity{
			ID:       fmt.Sprintf("function:flow.go:%d:1:%s", index+1, name),
			Kind:     evidence.EntityFunction,
			Name:     name,
			Language: "go",
			Scope:    evidence.SourceScopeRepository,
			Location: &location,
		}
		graph.AddEntity(entity)
		graph.AddRelation(fakeGoSelectionRelation(
			query.ID,
			entity.ID,
			evidence.RelationMatchesQuery,
			nil,
		))
	}
	graph.Sort()
	return graph, nil
}

func (fake *fakeGoSelectionAnalyzer) AnalyzeExactSymbol(
	_ context.Context,
	request analysis.ExactSymbolRequest,
) (evidence.Graph, error) {
	fake.exactCalls++
	root := fake.entities[goSelectionCallableName(request.Symbol.Name)]
	graph := evidence.NewGraph(request.RepoPath, root.Name)
	graph.AddEntity(root)
	switch root.Name {
	case "run":
		fake.addCall(&graph, "run", "prepare", 4)
	case "prepare":
		fake.addCall(&graph, "prepare", "swap", 8)
		fake.addCall(&graph, "prepare", "stopOld", 9)
	}
	graph.Sort()
	return graph, nil
}

func (fake *fakeGoSelectionAnalyzer) ResolveLocation(
	_ context.Context,
	request analysis.LocationRequest,
) (analysis.LocationResolution, error) {
	fake.resolveCalls++
	for _, entity := range fake.entities {
		if entity.Location.Path == request.Location.Path &&
			entity.Location.Line == request.Location.Line &&
			entity.Location.Column == request.Location.Column {
			return analysis.LocationResolution{
				Location: request.Location,
				Candidates: []analysis.LocationCandidate{{
					Entity:       entity,
					Match:        "declaration",
					Certainty:    evidence.CertaintyStatic,
					Investigable: true,
				}},
			}, nil
		}
	}
	return analysis.LocationResolution{}, fmt.Errorf("unknown location")
}

func (fake *fakeGoSelectionAnalyzer) addCall(
	graph *evidence.Graph,
	fromName string,
	toName string,
	line int,
) {
	from := fake.entities[fromName]
	to := fake.entities[toName]
	if fake.mislabelCallTargets {
		to.Kind = evidence.EntityMethod
	}
	graph.AddEntity(from)
	graph.AddEntity(to)
	callsite := evidence.Location{
		Path:      "flow.go",
		Line:      line,
		Column:    2,
		EndLine:   line,
		EndColumn: 8,
	}
	graph.AddRelation(fakeGoSelectionRelation(from.ID, to.ID, evidence.RelationCalls, &callsite))
}

func fakeGoSelectionRelation(
	from string,
	to string,
	kind evidence.RelationKind,
	location *evidence.Location,
) evidence.Relation {
	operation := "workspace_symbol"
	if kind == evidence.RelationCalls {
		operation = "call_hierarchy"
	}
	return evidence.Relation{
		From:      from,
		To:        to,
		Kind:      kind,
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider:  "gopls",
			Version:   "v0.test",
			Operation: operation,
			Location:  location,
		}},
	}
}

func sortedGoSelectionNames(trace GoSourceSelectionTrace) []string {
	names := make([]string, 0, goSelectionMaxSlices)
	for _, symbol := range trace.SelectedSymbols {
		names = append(names, goSelectionCallableName(symbol.Name))
	}
	sort.Strings(names)
	return names
}
