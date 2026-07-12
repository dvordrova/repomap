package pyright

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
)

// These checks intentionally stay shallow and fixture-driven while the
// playground contract is disposable. Replace them instead of preserving their
// shape when the Python adapter graduates into the product pipeline.

type fakeClient struct {
	responses map[string][]any
	closed    bool
}

func (c *fakeClient) Call(_ context.Context, method string, _ any, result any) error {
	queue := c.responses[method]
	if len(queue) == 0 {
		return fmt.Errorf("unexpected LSP method %q", method)
	}
	c.responses[method] = queue[1:]
	data, err := json.Marshal(queue[0])
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func (c *fakeClient) Notify(string, any) error { return nil }

func (c *fakeClient) Close(context.Context) error {
	c.closed = true
	return nil
}

func TestExactLocationProducesBoundedEvidence(t *testing.T) {
	fixture, err := filepath.Abs("testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err = filepath.EvalSymlinks(fixture)
	if err != nil {
		t.Fatal(err)
	}
	serviceURI := pathURI(filepath.Join(fixture, "app/service.py"))
	mainURI := pathURI(filepath.Join(fixture, "main.py"))
	testURI := pathURI(filepath.Join(fixture, "tests/test_service.py"))
	repositoryURI := pathURI(filepath.Join(fixture, "app/repository.py"))
	typeshedURI := "file:///tool/pyright/typeshed-fallback/stdlib/builtins.pyi"

	process := documentSymbol{
		Name: "process", Kind: 12,
		Range:          sourceRange{Start: position{Line: 7}, End: position{Line: 9, Character: 41}},
		SelectionRange: sourceRange{Start: position{Line: 7, Character: 4}, End: position{Line: 7, Character: 11}},
	}
	legacyProcess := documentSymbol{
		Name: "process", Kind: 6,
		Range:          sourceRange{Start: position{Line: 13, Character: 4}, End: position{Line: 14, Character: 20}},
		SelectionRange: sourceRange{Start: position{Line: 13, Character: 8}, End: position{Line: 13, Character: 15}},
	}
	documents := []documentSymbol{
		process,
		{Name: "LegacyService", Kind: 5, Children: []documentSymbol{legacyProcess}},
	}
	root := hierarchyItem("process", 12, serviceURI, process.Range, process.SelectionRange)
	mainCaller := hierarchyItem("run", 12, mainURI, lineRange(3, 0, 4, 28), lineRange(3, 4, 3, 7))
	testCaller := hierarchyItem("test_process", 12, testURI, lineRange(3, 0, 4, 44), lineRange(3, 4, 3, 16))
	save := hierarchyItem("save", 6, repositoryURI, lineRange(1, 4, 2, 20), lineRange(1, 8, 1, 12))
	getattr := hierarchyItem("getattr", 12, typeshedURI, lineRange(100, 0, 101, 1), lineRange(100, 4, 100, 11))

	client := &fakeClient{responses: map[string][]any{
		"textDocument/documentSymbol":       {documents, documents},
		"textDocument/prepareCallHierarchy": {[]callHierarchyItem{root}},
		"callHierarchy/incomingCalls": {[]incomingCall{
			{From: mainCaller, FromRanges: []sourceRange{lineRange(4, 11, 4, 18)}},
			{From: testCaller, FromRanges: []sourceRange{lineRange(4, 11, 4, 18)}},
			{From: testCaller, FromRanges: []sourceRange{lineRange(4, 11, 4, 18)}},
		}},
		"callHierarchy/outgoingCalls": {[]outgoingCall{
			{To: save, FromRanges: []sourceRange{lineRange(9, 11, 9, 38)}},
			{To: getattr, FromRanges: []sourceRange{lineRange(18, 11, 18, 42)}},
			{To: getattr, FromRanges: []sourceRange{lineRange(18, 11, 18, 42)}},
		}},
		"textDocument/references": {[]lspLocation{
			{URI: serviceURI, Range: process.SelectionRange},
			{URI: mainURI, Range: lineRange(4, 11, 4, 18)},
			{URI: mainURI, Range: lineRange(4, 11, 4, 18)},
		}},
	}}
	analyzer := New(Options{MaxIncoming: 10, MaxOutgoing: 10, MaxReferences: 10})
	analyzer.client = client
	analyzer.repoPath = fixture
	analyzer.version = "1.1.fixture"
	analyzer.capabilities.Capabilities.DocumentSymbolProvider = true
	analyzer.capabilities.Capabilities.CallHierarchyProvider = true
	analyzer.capabilities.Capabilities.ReferencesProvider = true

	resolution, err := analyzer.ResolveLocation(context.Background(), analysis.LocationRequest{
		RepoPath: fixture,
		Location: evidence.Location{Path: "app/service.py", Line: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.Candidates) != 1 || resolution.Candidates[0].Entity.Name != "process" {
		t.Fatalf("candidates = %#v, want top-level process only", resolution.Candidates)
	}
	if resolution.Candidates[0].Entity.Location.Line != 8 {
		t.Fatalf("resolved line = %d, want one-based line 8", resolution.Candidates[0].Entity.Location.Line)
	}

	graph, err := analyzer.AnalyzeExactSymbol(context.Background(), analysis.ExactSymbolRequest{
		RepoPath: fixture,
		Symbol:   resolution.Candidates[0].Entity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	assertRepositoryRelativeLocations(t, graph)
	assertExternalGetattr(t, graph)

	callRelations := 0
	for _, relation := range graph.Relations {
		if relation.Kind == evidence.RelationCalls {
			callRelations++
		}
		for _, provenance := range relation.Provenance {
			if provenance.Provider != "pyright" || provenance.Version != "1.1.fixture" || provenance.Operation == "" {
				t.Fatalf("incomplete provenance: %#v", provenance)
			}
		}
	}
	if callRelations != 4 {
		t.Fatalf("call relations = %d, want four deduplicated calls", callRelations)
	}
	if !strings.Contains(strings.Join(graph.Warnings, "\n"), "dynamic dispatch through getattr remains unresolved") {
		t.Fatalf("warnings = %v, want unresolved dynamic boundary", graph.Warnings)
	}
	if err := analyzer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !client.closed {
		t.Fatal("client was not closed")
	}
}

func TestDiscoverBinaryIsActionable(t *testing.T) {
	_, err := discoverBinary(filepath.Join(t.TempDir(), "missing-pyright-langserver"))
	if err == nil || !strings.Contains(err.Error(), "npm install -g pyright") || !strings.Contains(err.Error(), "--pyright-langserver") {
		t.Fatalf("error = %v, want actionable setup instructions", err)
	}
}

func TestClassifyLocationSeparatesToolchainScopes(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(string(filepath.Separator), "repo")
	tests := []struct {
		name string
		uri  string
		want evidence.SourceScope
	}{
		{name: "repository", uri: pathURI(filepath.Join(repo, "app/service.py")), want: evidence.SourceScopeRepository},
		{name: "standard library", uri: "file:///tool/typeshed-fallback/stdlib/builtins.pyi", want: evidence.SourceScopeStandardLibrary},
		{name: "dependency", uri: "file:///venv/lib/python3.12/site-packages/pkg/api.py", want: evidence.SourceScopeDependency},
		{name: "outside workspace", uri: "file:///elsewhere/generated.py", want: evidence.SourceScopeOutsideWorkspace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope, location := classifyLocation(repo, test.uri, lineRange(0, 0, 1, 1))
			if scope != test.want {
				t.Fatalf("scope = %q, want %q", scope, test.want)
			}
			if (scope == evidence.SourceScopeRepository) != (location != nil) {
				t.Fatalf("location = %#v for scope %q", location, scope)
			}
		})
	}
}

func hierarchyItem(name string, kind int, uri string, full, selection sourceRange) callHierarchyItem {
	return callHierarchyItem{Name: name, Kind: kind, URI: uri, Range: full, SelectionRange: selection}
}

func lineRange(startLine, startColumn, endLine, endColumn int) sourceRange {
	return sourceRange{
		Start: position{Line: startLine, Character: startColumn},
		End:   position{Line: endLine, Character: endColumn},
	}
}

func assertRepositoryRelativeLocations(t *testing.T, graph evidence.Graph) {
	t.Helper()
	for _, entity := range graph.Entities {
		if entity.Location != nil && filepath.IsAbs(entity.Location.Path) {
			t.Fatalf("entity %q leaked absolute path %q", entity.Name, entity.Location.Path)
		}
	}
	for _, relation := range graph.Relations {
		for _, provenance := range relation.Provenance {
			if provenance.Location != nil && filepath.IsAbs(provenance.Location.Path) {
				t.Fatalf("provenance leaked absolute path %q", provenance.Location.Path)
			}
		}
	}
}

func assertExternalGetattr(t *testing.T, graph evidence.Graph) {
	t.Helper()
	for _, entity := range graph.Entities {
		if entity.Name == "getattr" {
			if entity.Scope != evidence.SourceScopeStandardLibrary || entity.Location != nil {
				t.Fatalf("getattr entity = %#v, want pathless stdlib entity", entity)
			}
			return
		}
	}
	t.Fatal("getattr evidence entity not found")
}
