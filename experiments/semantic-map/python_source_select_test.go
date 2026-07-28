package semanticmap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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
	beetsPythonSelectionRevision = "9acb1ecff6c7ee0a1e83e3b983c94792345712c5"
	beetsPythonSelectionQuestion = "How does a Beets plugin named in configuration become an executable CLI command?"
)

func TestPythonSelectionQueriesRemoveRepositoryAndRelationalFiller(t *testing.T) {
	queries, content, err := pythonSelectionQueries("beets", beetsPythonSelectionQuestion)
	if err != nil {
		t.Fatal(err)
	}
	want := []PythonSelectionQuery{
		{ID: "q1", Text: "plugin"},
		{ID: "q2", Text: "config"},
		{ID: "q3", Text: "comman"},
	}
	if !reflect.DeepEqual(queries, want) {
		t.Fatalf("queries = %#v, want %#v", queries, want)
	}
	if !reflect.DeepEqual(content, []string{"plugin", "config", "comman"}) {
		t.Fatalf("content terms = %#v", content)
	}
}

func TestPythonSelectionIsInvariantToWorkspaceResultOrder(t *testing.T) {
	repoPath, revision, entities := makePythonSelectionFixture(t)
	hits := []pyrightWorkspaceHit{
		workspaceHit("q1", entities["load_plugins"]),
		workspaceHit("q2", entities["configured_names"]),
		workspaceHit("q3", entities["commands"]),
	}
	reversed := append([]pyrightWorkspaceHit(nil), hits...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	opts := PythonSourceSelectionOptions{
		RepositoryPath:   repoPath,
		ExpectedRevision: revision,
		Question:         "How does a sample plugin configuration become a command?",
	}
	firstTrace, firstPacket, err := selectPythonQuestionSources(
		context.Background(),
		opts,
		fakePythonWorkspaceFinder{hits: hits},
		newFakePythonSelectionAnalyzer(entities),
	)
	if err != nil {
		t.Fatal(err)
	}
	secondTrace, secondPacket, err := selectPythonQuestionSources(
		context.Background(),
		opts,
		fakePythonWorkspaceFinder{hits: reversed},
		newFakePythonSelectionAnalyzer(entities),
	)
	if err != nil {
		t.Fatal(err)
	}
	firstTraceJSON := mustEncodePythonSelection(t, firstTrace)
	secondTraceJSON := mustEncodePythonSelection(t, secondTrace)
	firstPacketJSON := mustEncodePythonSelection(t, firstPacket)
	secondPacketJSON := mustEncodePythonSelection(t, secondPacket)
	if !bytes.Equal(firstTraceJSON, secondTraceJSON) ||
		!bytes.Equal(firstPacketJSON, secondPacketJSON) {
		t.Fatal("permuted workspace results changed the normalized selection")
	}
	for _, name := range []string{"configured_names", "load_plugins", "commands", "setup"} {
		assertPythonSelectionName(t, firstTrace, name)
	}
	validatePythonSelectionArtifacts(t, firstTraceJSON, firstPacketJSON)
}

func TestPythonSelectionWorkspaceProcessingBudgetFailsClosed(t *testing.T) {
	hits := make([]pyrightWorkspaceHit, pythonSelectionMaxHitUnion+1)
	for index := range hits {
		hits[index] = pyrightWorkspaceHit{
			QueryID: "q1",
			Name:    fmt.Sprintf("function_%d", index),
			Path:    "sample.py",
			Kind:    evidence.EntityFunction,
			Line:    index + 1,
			Column:  1,
		}
	}
	_, _, err := buildPythonSelectionCandidates(
		hits,
		[]PythonSelectionQuery{{ID: "q1", Text: "sample"}},
		[]string{"sample"},
	)
	if err == nil || !strings.Contains(err.Error(), "processing budget") {
		t.Fatalf("workspace overflow error = %v", err)
	}
}

func TestPythonSelectionRejectsUnsafeWorkspacePathBeforeExactAnalysis(t *testing.T) {
	_, _, err := buildPythonSelectionCandidates(
		[]pyrightWorkspaceHit{{
			QueryID: "q1",
			Name:    "load",
			Path:    "/private/repository.py",
			Kind:    evidence.EntityFunction,
			Line:    1,
			Column:  1,
		}},
		[]PythonSelectionQuery{{ID: "q1", Text: "load"}},
		[]string{"load"},
	)
	if err == nil || !strings.Contains(err.Error(), "scalar or path budget") {
		t.Fatalf("unsafe path error = %v", err)
	}
}

func TestPyrightWorkspaceReadinessUsesFirstNonemptyBoundedSample(t *testing.T) {
	hitA := []pyrightWorkspaceHit{{QueryID: "q1", Name: "alpha", Path: "a.py"}}
	hitB := []pyrightWorkspaceHit{{QueryID: "q1", Name: "beta", Path: "b.py"}}
	samples := [][]pyrightWorkspaceHit{nil, hitA, hitB}
	calls := 0
	hits, err := awaitPyrightWorkspaceCandidates(
		context.Background(),
		pyrightWorkspaceQuery{ID: "q1", Text: "plugin"},
		len(samples),
		0,
		func(context.Context) ([]pyrightWorkspaceHit, error) {
			sample := samples[calls]
			calls++
			return sample, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hits, hitA) || calls != 2 {
		t.Fatalf("first nonempty readiness = %#v after %d calls", hits, calls)
	}
}

func TestPyrightWorkspaceReadinessFailsClosed(t *testing.T) {
	t.Run("request error", func(t *testing.T) {
		sampleErr := errors.New("sample failed")
		hits, err := awaitPyrightWorkspaceCandidates(
			context.Background(),
			pyrightWorkspaceQuery{ID: "q1", Text: "plugin"},
			3,
			0,
			func(context.Context) ([]pyrightWorkspaceHit, error) {
				return nil, sampleErr
			},
		)
		if !errors.Is(err, sampleErr) || hits != nil {
			t.Fatalf("request failure returned hits=%#v err=%v", hits, err)
		}
	})
	t.Run("all empty", func(t *testing.T) {
		calls := 0
		hits, err := awaitPyrightWorkspaceCandidates(
			context.Background(),
			pyrightWorkspaceQuery{ID: "q1", Text: "plugin"},
			3,
			0,
			func(context.Context) ([]pyrightWorkspaceHit, error) {
				calls++
				return nil, nil
			},
		)
		if err == nil ||
			!strings.Contains(err.Error(), "nonempty candidate sample") ||
			hits != nil ||
			calls != 3 {
			t.Fatalf("empty readiness returned hits=%#v err=%v calls=%d", hits, err, calls)
		}
	})
}

func TestPyrightWorkspaceTruncationKeepsRelevantLateHit(t *testing.T) {
	hits := make([]pyrightWorkspaceHit, 0, pyrightWorkspaceMaxHitsPerQuery+1)
	for index := 0; index < pyrightWorkspaceMaxHitsPerQuery; index++ {
		hits = append(hits, pyrightWorkspaceHit{
			QueryID: "q1",
			Name:    fmt.Sprintf("aardvark_%02d", index),
			Path:    "a.py",
			Kind:    evidence.EntityFunction,
			Line:    index + 1,
			Column:  1,
		})
	}
	hits = append(hits, pyrightWorkspaceHit{
		QueryID: "q1",
		Name:    "plugin",
		Path:    "z.py",
		Kind:    evidence.EntityFunction,
		Line:    1,
		Column:  1,
	})
	cappedKeys := func(input []pyrightWorkspaceHit) []string {
		input = append([]pyrightWorkspaceHit(nil), input...)
		sortPyrightWorkspaceHitsForQuery(input, "plugin")
		input = input[:pyrightWorkspaceMaxHitsPerQuery]
		keys := make([]string, len(input))
		for index, hit := range input {
			keys[index] = pyrightWorkspaceHitKey(hit)
		}
		return keys
	}
	first := cappedKeys(hits)
	reversed := append([]pyrightWorkspaceHit(nil), hits...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	second := cappedKeys(reversed)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("relevant workspace truncation depends on analyzer result order")
	}
	if !strings.Contains(strings.Join(first, "\n"), "\x00plugin") {
		t.Fatal("relevant 65th lexicographic hit was lost before the per-query cap")
	}
}

func TestPythonSelectionEndpointCapacityCountsBothUnknowns(t *testing.T) {
	nodes := make(map[string]pythonSelectionNode, pythonSelectionMaxCallEndpoints)
	for index := 0; index < pythonSelectionMaxCallEndpoints-1; index++ {
		nodes[fmt.Sprintf("known-%02d", index)] = pythonSelectionNode{}
	}
	if pythonSelectionEndpointsFit(nodes, "unknown-a", "unknown-b") {
		t.Fatal("63 existing endpoints accepted two new endpoints")
	}
	if !pythonSelectionEndpointsFit(nodes, "known-00", "unknown-a") {
		t.Fatal("63 existing endpoints rejected one new endpoint")
	}
	if !pythonSelectionEndpointsFit(nodes, "unknown-a", "unknown-a") {
		t.Fatal("63 existing endpoints rejected one shared new endpoint")
	}
}

func TestPythonSelectionMandatorySpineFailsClosedAtSliceCap(t *testing.T) {
	nodes := make([]pythonSelectionNode, pythonSelectionMaxSlices+1)
	edges := make([]pythonSelectionEdge, 0, pythonSelectionMaxSlices)
	for index := range nodes {
		location := evidence.Location{
			Path:      "sample.py",
			Line:      index + 1,
			Column:    1,
			EndLine:   index + 1,
			EndColumn: 2,
		}
		entity := evidence.Entity{
			ID:       fmt.Sprintf("function:sample.py:%d:step_%02d", index+1, index),
			Kind:     evidence.EntityFunction,
			Name:     fmt.Sprintf("step_%02d", index),
			Language: "python",
			Scope:    evidence.SourceScopeRepository,
			Location: &location,
		}
		nodes[index] = pythonSelectionNode{entity: entity}
		if index == 0 {
			nodes[index].anchor = true
			nodes[index].queryIDs = []string{"q1"}
		}
		if index == len(nodes)-1 {
			nodes[index].anchor = true
			nodes[index].queryIDs = []string{"q2"}
		}
		if index > 0 {
			edges = append(edges, pythonSelectionEdge{
				from: goSelectionEntityKey(nodes[index-1].entity),
				to:   goSelectionEntityKey(entity),
				location: evidence.Location{
					Path:   "sample.py",
					Line:   index + 1,
					Column: 1,
				},
			})
		}
	}
	_, err := selectPythonSelectionNodes(nodes, edges, []string{"step"})
	if err == nil || !strings.Contains(err.Error(), "mandatory connected spine") {
		t.Fatalf("mandatory slice-cap error = %v", err)
	}
}

func TestPythonSelectionMandatorySpineFailsClosedAtSourceBudget(t *testing.T) {
	repoPath := t.TempDir()
	line := "#" + strings.Repeat("x", 9<<10) + "\n"
	if err := os.WriteFile(
		filepath.Join(repoPath, "sample.py"),
		[]byte(line+line+line),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	revision := commitGoSelectionFixture(t, repoPath, "sample.py")
	nodes := make([]pythonSelectionNode, 3)
	for index, name := range []string{"begin", "middle", "end"} {
		location := evidence.Location{
			Path:      "sample.py",
			Line:      index + 1,
			Column:    1,
			EndLine:   index + 1,
			EndColumn: 2,
		}
		nodes[index] = pythonSelectionNode{
			entity: evidence.Entity{
				ID:       fmt.Sprintf("function:sample.py:%d:%s", index+1, name),
				Kind:     evidence.EntityFunction,
				Name:     name,
				Language: "python",
				Scope:    evidence.SourceScopeRepository,
				Location: &location,
			},
			mandatory: true,
		}
	}
	nodes[0].anchor = true
	nodes[0].queryIDs = []string{"q1"}
	nodes[2].anchor = true
	nodes[2].queryIDs = []string{"q2"}
	edges := []pythonSelectionEdge{
		{
			from: goSelectionEntityKey(nodes[0].entity),
			to:   goSelectionEntityKey(nodes[1].entity),
		},
		{
			from: goSelectionEntityKey(nodes[1].entity),
			to:   goSelectionEntityKey(nodes[2].entity),
		},
	}
	_, _, _, err := buildPythonSelectionPacket(
		repoPath,
		GoSelectionRepository{Name: "fixture", Revision: revision},
		"How does the flow complete?",
		nodes,
		edges,
	)
	if err == nil || !strings.Contains(err.Error(), "source budget") {
		t.Fatalf("mandatory source-budget error = %v", err)
	}
}

func TestPythonSelectionMandatorySpineFailsClosedAtRangeResolution(t *testing.T) {
	repoPath, _, entities := makePythonSelectionFixture(t)
	node := pythonSelectionNode{
		entity:    entities["edge_free_plugin"],
		queryIDs:  []string{"q1"},
		anchor:    true,
		mandatory: true,
	}
	_, _, _, err := enrichPythonSelectionRanges(
		context.Background(),
		newFakePythonSelectionAnalyzer(nil),
		repoPath,
		[]pythonSelectionNode{node},
		nil,
		[]string{"plugin"},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "mandatory connected spine symbol") {
		t.Fatalf("mandatory range-resolution error = %v", err)
	}
}

func TestPythonSelectionFiltersUntrackedRootBeforeAnalyzer(t *testing.T) {
	repoPath, revision, entities := makePythonSelectionFixture(t)
	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("ignored.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	revision = commitGoSelectionFixture(t, repoPath, ".gitignore")
	if err := os.WriteFile(
		filepath.Join(repoPath, "ignored.py"),
		[]byte("def ignored_plugin():\n    pass\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	analyzer := newFakePythonSelectionAnalyzer(entities)
	_, _, err := selectPythonQuestionSources(
		context.Background(),
		PythonSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "ignored",
		},
		fakePythonWorkspaceFinder{hits: []pyrightWorkspaceHit{{
			QueryID: "q1",
			Name:    "ignored_plugin",
			Path:    "ignored.py",
			Kind:    evidence.EntityFunction,
			Line:    1,
			Column:  5,
		}}},
		analyzer,
	)
	if err == nil || !strings.Contains(err.Error(), "no edge-backed exact root") {
		t.Fatalf("untracked root error = %v", err)
	}
	if analyzer.resolveCalls != 0 || len(analyzer.exactCalls) != 0 {
		t.Fatalf(
			"untracked root reached analyzer: resolve=%d exact=%v",
			analyzer.resolveCalls,
			analyzer.exactCalls,
		)
	}
}

func TestPythonSelectionRejectsTrackedSymlinkBeforeAnalyzer(t *testing.T) {
	repoPath := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(outsideDir, "outside.py"),
		[]byte("def linked_plugin():\n    pass\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join(outsideDir, "outside.py"),
		filepath.Join(repoPath, "link.py"),
	); err != nil {
		t.Fatal(err)
	}
	revision := commitGoSelectionFixture(t, repoPath, "link.py")
	analyzer := newFakePythonSelectionAnalyzer(nil)
	_, _, err := selectPythonQuestionSources(
		context.Background(),
		PythonSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "linked",
		},
		fakePythonWorkspaceFinder{hits: []pyrightWorkspaceHit{{
			QueryID: "q1",
			Name:    "linked_plugin",
			Path:    "link.py",
			Kind:    evidence.EntityFunction,
			Line:    1,
			Column:  5,
		}}},
		analyzer,
	)
	if err == nil || !strings.Contains(err.Error(), "no edge-backed exact root") {
		t.Fatalf("tracked symlink error = %v", err)
	}
	if analyzer.resolveCalls != 0 || len(analyzer.exactCalls) != 0 {
		t.Fatalf(
			"tracked symlink reached analyzer: resolve=%d exact=%v",
			analyzer.resolveCalls,
			analyzer.exactCalls,
		)
	}
}

func TestPythonSelectionRejectsUntrackedGraphEndpointBeforeProjection(t *testing.T) {
	repoPath, revision, entities := makePythonSelectionFixture(t)
	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("ignored.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	revision = commitGoSelectionFixture(t, repoPath, ".gitignore")
	if err := os.WriteFile(
		filepath.Join(repoPath, "ignored.py"),
		[]byte("def ignored_target():\n    pass\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	ignoredLocation := evidence.Location{
		Path:      "ignored.py",
		Line:      1,
		Column:    1,
		EndLine:   2,
		EndColumn: 1,
	}
	ignored := evidence.Entity{
		ID:       "pyright:symbol:ignored.py:1:1:ignored_target",
		Kind:     evidence.EntityFunction,
		Name:     "ignored_target",
		Language: "python",
		Scope:    evidence.SourceScopeRepository,
		Location: &ignoredLocation,
	}
	analyzer := newFakePythonSelectionAnalyzer(entities)
	analyzer.graphFactory = func(request analysis.ExactSymbolRequest) evidence.Graph {
		graph := evidence.NewGraph(request.RepoPath, request.Symbol.Name)
		addFakePythonCall(&graph, request.Symbol, ignored)
		return graph
	}
	_, _, err := selectPythonQuestionSources(
		context.Background(),
		PythonSourceSelectionOptions{
			RepositoryPath:   repoPath,
			ExpectedRevision: revision,
			Question:         "plugin",
		},
		fakePythonWorkspaceFinder{hits: []pyrightWorkspaceHit{
			workspaceHit("q1", entities["load_plugins"]),
		}},
		analyzer,
	)
	if err == nil || !strings.Contains(err.Error(), "callee source preflight") {
		t.Fatalf("unsafe graph endpoint error = %v", err)
	}
	if analyzer.resolveCalls != 1 || !reflect.DeepEqual(analyzer.exactCalls, []string{"load_plugins"}) {
		t.Fatalf(
			"unsafe graph endpoint processing = resolve %d, exact %v",
			analyzer.resolveCalls,
			analyzer.exactCalls,
		)
	}
}

func TestPythonSelectionStopsAfterEdgeBackedRootsAndJoinsThem(t *testing.T) {
	repoPath, _, entities := makePythonSelectionFixture(t)
	analyzer := newFakePythonSelectionAnalyzer(entities)
	candidates := []pythonSelectionCandidateState{
		{hit: workspaceHit("q1", entities["edge_free_plugin"]), queryIDs: []string{"q1"}},
		{hit: workspaceHit("q1", entities["load_plugins"]), queryIDs: []string{"q1"}},
		{hit: workspaceHit("q2", entities["configured_names"]), queryIDs: []string{"q2"}},
		{hit: workspaceHit("q3", entities["commands"]), queryIDs: []string{"q3"}},
	}
	nodes, edges, _, _, err := inspectPythonSelectionRoots(
		context.Background(),
		analyzer,
		repoPath,
		[]PythonSelectionQuery{
			{ID: "q1", Text: "plugin"},
			{ID: "q2", Text: "config"},
			{ID: "q3", Text: "command"},
		},
		[]string{"plugin", "config", "command"},
		candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"edge_free_plugin",
		"load_plugins",
		"configured_names",
		"commands",
		"setup",
	}
	if !reflect.DeepEqual(analyzer.exactCalls, wantCalls) {
		t.Fatalf("exact analysis order = %v, want %v", analyzer.exactCalls, wantCalls)
	}
	if !pythonSelectionHasNamedCall(nodes, edges, "setup", "load_plugins") ||
		!pythonSelectionHasNamedCall(nodes, edges, "setup", "commands") {
		t.Fatal("bounded bridge analysis did not connect all three query roots")
	}
}

func TestPythonSelectionEdgeFreeGraphDoesNotAssertVersionMismatch(t *testing.T) {
	repoPath, _, entities := makePythonSelectionFixture(t)
	analyzer := newFakePythonSelectionAnalyzer(entities)
	candidates := []pythonSelectionCandidateState{
		{hit: workspaceHit("q1", entities["load_plugins"]), queryIDs: []string{"q1"}},
		{hit: workspaceHit("q2", entities["edge_free_plugin"]), queryIDs: []string{"q2"}},
		{hit: workspaceHit("q2", entities["configured_names"]), queryIDs: []string{"q2"}},
		{hit: workspaceHit("q3", entities["commands"]), queryIDs: []string{"q3"}},
	}
	_, _, version, _, err := inspectPythonSelectionRoots(
		context.Background(),
		analyzer,
		repoPath,
		[]PythonSelectionQuery{
			{ID: "q1", Text: "plugin"},
			{ID: "q2", Text: "config"},
			{ID: "q3", Text: "command"},
		},
		[]string{"plugin", "config", "command"},
		candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.1.test" {
		t.Fatalf("exact version = %q", version)
	}
	wantCalls := []string{
		"load_plugins",
		"edge_free_plugin",
		"configured_names",
		"commands",
		"setup",
	}
	if !reflect.DeepEqual(analyzer.exactCalls, wantCalls) {
		t.Fatalf("exact analysis order = %v, want %v", analyzer.exactCalls, wantCalls)
	}
}

func TestRecordedBeetsPythonSelection(t *testing.T) {
	traceJSON := readBoundedFile(t, "beets.python-selection.json", 96<<10)
	packetJSON := readBoundedFile(t, "beets.python-auto-source-slices.json", 48<<10)
	trace := decodeStrict[PythonSourceSelectionTrace](t, traceJSON)
	packet := decodeStrict[PythonSourceSelectionPacket](t, packetJSON)
	if got := mustEncodePythonSelection(t, trace); !bytes.Equal(got, traceJSON) {
		t.Fatal("recorded Beets trace is not canonically encoded")
	}
	if got := mustEncodePythonSelection(t, packet); !bytes.Equal(got, packetJSON) {
		t.Fatal("recorded Beets packet is not canonically encoded")
	}
	validatePythonSelectionArtifacts(t, traceJSON, packetJSON)
	if trace.Repository.Revision != beetsPythonSelectionRevision ||
		trace.Question != beetsPythonSelectionQuestion {
		t.Fatal("recorded Beets selector input changed")
	}
	for _, name := range []string{
		"_get_plugin",
		"load_plugins",
		"get_plugin_names",
		"commands",
		"_setup",
		"_bootstrap_config",
		"_raw_main",
	} {
		assertPythonSelectionName(t, trace, name)
	}
	packetText := pythonSelectionPacketText(packet)
	for _, source := range []string{
		`config["plugins"]`,
		"map(_get_plugin, names)",
		"plugin.commands()",
		"parser.add_subcommand",
		"parse_subcommand",
		"subcommand.func",
	} {
		if !strings.Contains(packetText, source) {
			t.Errorf("packet omits exact source evidence %q", source)
		}
	}
	for _, call := range trace.ExactCalls {
		if strings.Contains(call.CalleeSymbolID, "subcommand.func") ||
			strings.HasPrefix(call.CalleeSymbolID, "method:") {
			t.Fatalf("selector invented a concrete runtime dispatch target: %#v", call)
		}
	}
	for _, pair := range [][2]string{
		{"_raw_main", "_bootstrap_config"},
		{"_raw_main", "_setup"},
		{"_setup", "load_plugins"},
		{"load_plugins", "get_plugin_names"},
		{"_setup", "commands"},
		{"commands", "find_plugins"},
	} {
		if !pythonSelectionTraceHasNamedCall(trace, pair[0], pair[1]) {
			t.Errorf("recorded exact calls omit %s -> %s", pair[0], pair[1])
		}
	}
	if len(packet.SourceSlices) != 8 {
		t.Fatalf("recorded packet has %d slices, want 8", len(packet.SourceSlices))
	}
	for _, noise := range []string{
		"config_edit",
		"config_func",
		"editor_command",
		"command_output",
		"print_completion",
		"send",
	} {
		for _, name := range pythonSelectionNames(trace) {
			if name == noise {
				t.Errorf("recorded packet retains question-irrelevant neighbor %q", noise)
			}
		}
	}
}

func TestPythonSelectorImplementationHasNoBeetsOracle(t *testing.T) {
	var source []byte
	for _, name := range []string{"python_source_select.go", "pyright_workspace_symbol.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source = append(source, data...)
	}
	for _, forbidden := range []string{
		"beets",
		"_raw_main",
		"_get_plugin",
		"load_plugins",
		beetsPythonSelectionRevision,
		"/Users/",
	} {
		if bytes.Contains(bytes.ToLower(source), bytes.ToLower([]byte(forbidden))) {
			t.Errorf("selector implementation contains curated oracle text %q", forbidden)
		}
	}
}

func TestLiveBeetsPythonSelection(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_BEETS_REPO")
	if repoPath == "" {
		t.Skip("set REPOMAP_BEETS_REPO to replay the pinned live selector")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	trace, packet, err := SelectPythonQuestionSources(ctx, PythonSourceSelectionOptions{
		RepositoryPath:   repoPath,
		ExpectedRevision: beetsPythonSelectionRevision,
		Question:         beetsPythonSelectionQuestion,
		PyrightBinary:    os.Getenv("REPOMAP_PYRIGHT_BINARY"),
	})
	if err != nil {
		t.Fatal(err)
	}
	traceJSON := mustEncodePythonSelection(t, trace)
	packetJSON := mustEncodePythonSelection(t, packet)
	validatePythonSelectionArtifacts(t, traceJSON, packetJSON)
	if outputDir := os.Getenv("REPOMAP_PYTHON_SELECTION_OUTPUT_DIR"); outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(outputDir, "beets.python-selection.json"),
			traceJSON,
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(outputDir, "beets.python-auto-source-slices.json"),
			packetJSON,
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func validatePythonSelectionArtifacts(t *testing.T, traceJSON, packetJSON []byte) {
	t.Helper()
	trace := decodeStrict[PythonSourceSelectionTrace](t, traceJSON)
	packet := decodeStrict[PythonSourceSelectionPacket](t, packetJSON)
	if trace.Version != pythonSelectionVersion || packet.Version != pythonSelectionVersion {
		t.Fatal("Python selector artifact version changed")
	}
	if trace.Repository != packet.Repository ||
		trace.Question != packet.Question ||
		trace.Coverage != packet.Coverage {
		t.Fatal("trace and packet inputs or coverage differ")
	}
	if trace.Coverage != "bounded_pyright_static_call_neighborhood_non_exhaustive" {
		t.Fatalf("coverage = %q", trace.Coverage)
	}
	if len(trace.QueryTerms) == 0 || len(trace.QueryTerms) > pythonSelectionMaxQueryTerms {
		t.Fatalf("query terms = %d", len(trace.QueryTerms))
	}
	if len(trace.Candidates) == 0 || len(trace.Candidates) > pythonSelectionMaxCandidates {
		t.Fatalf("candidates = %d", len(trace.Candidates))
	}
	if len(trace.SelectedSymbols) == 0 || len(trace.SelectedSymbols) > pythonSelectionMaxSlices {
		t.Fatalf("selected symbols = %d", len(trace.SelectedSymbols))
	}
	if len(trace.ExactCalls) > pythonSelectionMaxCallEndpoints {
		t.Fatalf("exact calls = %d", len(trace.ExactCalls))
	}
	if len(packet.SourceSlices) == 0 || len(packet.SourceSlices) > pythonSelectionMaxSlices {
		t.Fatalf("source slices = %d", len(packet.SourceSlices))
	}
	if len(trace.UnresolvedFrontiers) == 0 {
		t.Fatal("selector does not expose its unresolved runtime frontier")
	}
	if bytes.Contains(traceJSON, []byte("/Users/")) ||
		bytes.Contains(packetJSON, []byte("/Users/")) {
		t.Fatal("encoded selector artifacts contain an absolute home path")
	}
	previousScore := int(^uint(0) >> 1)
	for _, candidate := range trace.Candidates {
		if !goSelectionCanonicalPath(candidate.Path) ||
			!strings.HasSuffix(candidate.Path, ".py") ||
			candidate.Line <= 0 ||
			candidate.Column <= 0 ||
			len(candidate.QueryTermIDs) == 0 ||
			candidate.Score > previousScore {
			t.Fatalf("invalid or unsorted candidate %#v", candidate)
		}
		previousScore = candidate.Score
	}
	symbols := make(map[string]PythonSelectionSymbol, pythonSelectionMaxSlices)
	for _, symbol := range trace.SelectedSymbols {
		if symbol.ID == "" ||
			!goSelectionCanonicalPath(symbol.Path) ||
			!strings.HasSuffix(symbol.Path, ".py") ||
			symbol.StartLine <= 0 ||
			symbol.EndLine < symbol.StartLine ||
			symbol.EndLine-symbol.StartLine+1 > pythonSelectionMaxSliceLines ||
			len(symbol.SelectionReasonIDs) == 0 {
			t.Fatalf("invalid selected symbol %#v", symbol)
		}
		symbols[symbol.ID] = symbol
	}
	callIDs := make(map[string]struct{}, pythonSelectionMaxCallEndpoints)
	for index, call := range trace.ExactCalls {
		if call.ID != fmt.Sprintf("e%d", index+1) ||
			!goSelectionCanonicalPath(call.Path) ||
			call.StartLine <= 0 ||
			call.StartColumn <= 0 {
			t.Fatalf("invalid exact call %#v", call)
		}
		if _, ok := symbols[call.CallerSymbolID]; !ok {
			t.Fatalf("unknown call caller %q", call.CallerSymbolID)
		}
		if _, ok := symbols[call.CalleeSymbolID]; !ok {
			t.Fatalf("unknown call callee %q", call.CalleeSymbolID)
		}
		callIDs[call.ID] = struct{}{}
	}
	totalBytes := 0
	sliceIDs := make(map[string]struct{}, pythonSelectionMaxSlices)
	for _, sourceSlice := range packet.SourceSlices {
		symbol, ok := symbols[sourceSlice.EnclosingSymbolID]
		if !ok ||
			symbol.Path != sourceSlice.Path ||
			symbol.StartLine != sourceSlice.StartLine ||
			symbol.EndLine != sourceSlice.EndLine ||
			sourceSlice.Text == "" {
			t.Fatalf("source slice does not match selected symbol %#v", sourceSlice)
		}
		for _, reason := range sourceSlice.SelectionReasonIDs {
			if strings.HasPrefix(reason, "e") {
				if _, ok := callIDs[reason]; !ok {
					t.Fatalf("source slice references unknown edge %q", reason)
				}
			}
		}
		totalBytes += len(sourceSlice.Text)
		sliceIDs[sourceSlice.EnclosingSymbolID] = struct{}{}
	}
	if totalBytes > pythonSelectionMaxSourceBytes {
		t.Fatalf("source bytes = %d, limit %d", totalBytes, pythonSelectionMaxSourceBytes)
	}
	if len(sliceIDs) != len(symbols) {
		t.Fatal("trace selected symbols and packet source slices differ")
	}
}

func mustEncodePythonSelection(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := EncodePythonSourceSelection(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertPythonSelectionName(
	t *testing.T,
	trace PythonSourceSelectionTrace,
	name string,
) {
	t.Helper()
	for _, symbol := range trace.SelectedSymbols {
		if symbol.Name == name {
			return
		}
	}
	t.Errorf("selected symbols omit %q; got %v", name, pythonSelectionNames(trace))
}

func pythonSelectionNames(trace PythonSourceSelectionTrace) []string {
	names := make([]string, 0, len(trace.SelectedSymbols))
	for _, symbol := range trace.SelectedSymbols {
		names = append(names, symbol.Name)
	}
	sort.Strings(names)
	return names
}

func pythonSelectionPacketText(packet PythonSourceSelectionPacket) string {
	var text strings.Builder
	for _, sourceSlice := range packet.SourceSlices {
		text.WriteString(sourceSlice.Text)
	}
	return text.String()
}

func pythonSelectionTraceHasNamedCall(
	trace PythonSourceSelectionTrace,
	caller string,
	callee string,
) bool {
	for _, call := range trace.ExactCalls {
		if strings.HasSuffix(call.CallerSymbolID, ":"+caller) &&
			strings.HasSuffix(call.CalleeSymbolID, ":"+callee) {
			return true
		}
	}
	return false
}

func pythonSelectionHasNamedCall(
	nodes []pythonSelectionNode,
	edges []pythonSelectionEdge,
	caller string,
	callee string,
) bool {
	names := make(map[string]string, pythonSelectionMaxCallEndpoints)
	for _, node := range nodes {
		names[goSelectionEntityKey(node.entity)] = node.entity.Name
	}
	for _, edge := range edges {
		if names[edge.from] == caller && names[edge.to] == callee {
			return true
		}
	}
	return false
}

type fakePythonWorkspaceFinder struct {
	hits []pyrightWorkspaceHit
}

func (finder fakePythonWorkspaceFinder) Find(
	_ context.Context,
	_ string,
	_ []pyrightWorkspaceQuery,
) (pyrightWorkspaceResult, error) {
	return pyrightWorkspaceResult{
		Version: "1.1.test",
		Hits:    append([]pyrightWorkspaceHit(nil), finder.hits...),
	}, nil
}

type fakePythonSelectionAnalyzer struct {
	entities     map[string]evidence.Entity
	resolveCalls int
	exactCalls   []string
	graphFactory func(analysis.ExactSymbolRequest) evidence.Graph
}

func newFakePythonSelectionAnalyzer(
	entities map[string]evidence.Entity,
) *fakePythonSelectionAnalyzer {
	return &fakePythonSelectionAnalyzer{
		entities:   entities,
		exactCalls: make([]string, 0, pythonSelectionMaxExactAnalyses),
	}
}

func (fake *fakePythonSelectionAnalyzer) ResolveLocation(
	_ context.Context,
	request analysis.LocationRequest,
) (analysis.LocationResolution, error) {
	fake.resolveCalls++
	for _, entity := range fake.entities {
		if entity.Location.Path == request.Location.Path &&
			entity.Location.Line == request.Location.Line {
			return analysis.LocationResolution{
				Location: request.Location,
				Candidates: []analysis.LocationCandidate{{
					Entity:       cloneGoSelectionEntity(entity),
					Match:        "exact declaration line",
					Certainty:    evidence.CertaintyStatic,
					Investigable: true,
				}},
			}, nil
		}
	}
	return analysis.LocationResolution{Location: request.Location}, nil
}

func (fake *fakePythonSelectionAnalyzer) AnalyzeExactSymbol(
	_ context.Context,
	request analysis.ExactSymbolRequest,
) (evidence.Graph, error) {
	fake.exactCalls = append(fake.exactCalls, request.Symbol.Name)
	if fake.graphFactory != nil {
		graph := fake.graphFactory(request)
		graph.Sort()
		return graph, nil
	}
	graph := evidence.NewGraph(request.RepoPath, request.Symbol.Name)
	graph.AddEntity(cloneGoSelectionEntity(request.Symbol))
	switch request.Symbol.Name {
	case "configured_names":
		addFakePythonCall(&graph, fake.entities["load_plugins"], fake.entities["configured_names"])
	case "load_plugins":
		addFakePythonCall(&graph, fake.entities["load_plugins"], fake.entities["configured_names"])
	case "commands":
		addFakePythonCall(&graph, fake.entities["setup"], fake.entities["commands"])
	case "setup":
		addFakePythonCall(&graph, fake.entities["setup"], fake.entities["load_plugins"])
		addFakePythonCall(&graph, fake.entities["setup"], fake.entities["commands"])
	}
	graph.Sort()
	return graph, nil
}

func addFakePythonCall(
	graph *evidence.Graph,
	from evidence.Entity,
	to evidence.Entity,
) {
	graph.AddEntity(cloneGoSelectionEntity(from))
	graph.AddEntity(cloneGoSelectionEntity(to))
	location := evidence.Location{
		Path:      from.Location.Path,
		Line:      from.Location.Line + 1,
		Column:    5,
		EndLine:   from.Location.Line + 1,
		EndColumn: 12,
	}
	graph.AddRelation(evidence.Relation{
		From:      from.ID,
		To:        to.ID,
		Kind:      evidence.RelationCalls,
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider:  "pyright",
			Version:   "1.1.test",
			Operation: "callHierarchy/incomingCalls",
			Location:  &location,
		}},
	})
}

func workspaceHit(queryID string, entity evidence.Entity) pyrightWorkspaceHit {
	return pyrightWorkspaceHit{
		QueryID: queryID,
		Name:    entity.Name,
		Path:    entity.Location.Path,
		Kind:    entity.Kind,
		Line:    entity.Location.Line,
		Column:  entity.Location.Column + 4,
	}
}

func makePythonSelectionFixture(
	t *testing.T,
) (string, string, map[string]evidence.Entity) {
	t.Helper()
	repoPath := t.TempDir()
	source := strings.Join([]string{
		"def configured_names():",
		"    return [\"sample\"]",
		"",
		"def load_plugins():",
		"    return configured_names()",
		"",
		"def commands():",
		"    return []",
		"",
		"def setup():",
		"    load_plugins()",
		"    return commands()",
		"",
		"def edge_free_plugin():",
		"    return None",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoPath, "sample.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision := commitGoSelectionFixture(t, repoPath, "sample.py")
	entities := map[string]evidence.Entity{}
	for _, declaration := range []struct {
		name      string
		startLine int
		endLine   int
	}{
		{name: "configured_names", startLine: 1, endLine: 2},
		{name: "load_plugins", startLine: 4, endLine: 5},
		{name: "commands", startLine: 7, endLine: 8},
		{name: "setup", startLine: 10, endLine: 12},
		{name: "edge_free_plugin", startLine: 14, endLine: 15},
	} {
		location := evidence.Location{
			Path:      "sample.py",
			Line:      declaration.startLine,
			Column:    1,
			EndLine:   declaration.endLine,
			EndColumn: 1,
		}
		entity := evidence.Entity{
			ID:       fmt.Sprintf("pyright:symbol:sample.py:%d:1:%s", declaration.startLine, declaration.name),
			Kind:     evidence.EntityFunction,
			Name:     declaration.name,
			Language: "python",
			Scope:    evidence.SourceScopeRepository,
			Location: &location,
		}
		entities[declaration.name] = entity
	}
	return repoPath, revision, entities
}
