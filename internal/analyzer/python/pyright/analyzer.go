// Package pyright adapts one bounded Pyright LSP workspace session to the
// language-neutral evidence graph. It is intentionally focused on exact
// symbols and is not part of the default repository survey.
package pyright

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/lspclient"
)

const (
	activeScenarioID = "pyright-workspace"
	maxSourceBytes   = 4 * 1024 * 1024
	collectorVersion = 1
)

type Options struct {
	Binary         string
	MaxIncoming    int
	MaxOutgoing    int
	MaxReferences  int
	RequestTimeout time.Duration
}

type Analyzer struct {
	opts Options

	client       rpcClient
	repoPath     string
	version      string
	capabilities initializeResult
	opened       map[string]struct{}
}

type rpcClient interface {
	Call(ctx context.Context, method string, params, result any) error
	Notify(method string, params any) error
	Close(ctx context.Context) error
}

var _ analysis.LocationResolver = (*Analyzer)(nil)
var _ analysis.ExactSymbolAnalyzer = (*Analyzer)(nil)

func New(opts Options) *Analyzer {
	if opts.MaxIncoming <= 0 {
		opts.MaxIncoming = 12
	}
	if opts.MaxOutgoing <= 0 {
		opts.MaxOutgoing = 12
	}
	if opts.MaxReferences <= 0 {
		opts.MaxReferences = 40
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	return &Analyzer{opts: opts, opened: map[string]struct{}{}}
}

func (a *Analyzer) ResolveLocation(ctx context.Context, req analysis.LocationRequest) (analysis.LocationResolution, error) {
	repoPath, location, err := normalizeRequestLocation(req.RepoPath, req.Location)
	if err != nil {
		return analysis.LocationResolution{}, err
	}
	if err := a.ensureSession(ctx, repoPath); err != nil {
		return analysis.LocationResolution{}, err
	}
	symbols, err := a.documentSymbols(ctx, repoPath, location.Path)
	if err != nil {
		return analysis.LocationResolution{}, fmt.Errorf("pyright: document symbols: %w", err)
	}
	candidates := selectSymbols(symbols, location)
	limit := req.MaxCandidates
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := analysis.LocationResolution{
		Location:   location,
		Candidates: make([]analysis.LocationCandidate, 0, len(candidates)),
		Certainty:  evidence.CertaintyStatic,
		Provenance: a.provenance("textDocument/documentSymbol", "callable declaration selected by exact source location", &location),
		Scenario:   a.scenario(),
	}
	for _, candidate := range candidates {
		entity := documentEntity(location.Path, candidate)
		result.Candidates = append(result.Candidates, analysis.LocationCandidate{
			Entity:       entity,
			Match:        "exact declaration line",
			Certainty:    evidence.CertaintyStatic,
			Investigable: isCallableKind(entity.Kind),
		})
	}
	if len(result.Candidates) == 0 {
		result.Warnings = append(result.Warnings, "Pyright returned no callable declaration at this exact location")
	}
	return result, nil
}

func (a *Analyzer) AnalyzeExactSymbol(ctx context.Context, req analysis.ExactSymbolRequest) (evidence.Graph, error) {
	repoPath, selected, err := normalizeExactSymbolRequest(req)
	if err != nil {
		return evidence.Graph{}, err
	}
	if err := a.ensureSession(ctx, repoPath); err != nil {
		return evidence.Graph{}, err
	}
	symbols, err := a.documentSymbols(ctx, repoPath, selected.Location.Path)
	if err != nil {
		return evidence.Graph{}, fmt.Errorf("pyright: confirm document symbol: %w", err)
	}
	confirmed, ok := confirmDocumentSymbol(symbols, selected)
	if !ok {
		return evidence.Graph{}, fmt.Errorf("pyright: selected declaration no longer matches document symbols at %s:%d:%d", selected.Location.Path, selected.Location.Line, selected.Location.Column)
	}

	graph := evidence.NewGraph(repoPath, fmt.Sprintf("%s:%d:%d", selected.Location.Path, selected.Location.Line, selected.Location.Column))
	graph.Scenarios = append(graph.Scenarios, a.scenario())
	graph.Warnings = append(graph.Warnings, "Pyright provides bounded static evidence, not a complete runtime trace")
	query := evidence.Entity{ID: "query:" + graph.Query, Kind: evidence.EntityQuery, Name: graph.Query}
	target := documentEntity(selected.Location.Path, confirmed)
	graph.AddEntity(query)
	graph.AddEntity(target)
	resolutionEvidence := []evidence.Provenance{a.provenance(
		"textDocument/documentSymbol",
		"selected declaration confirmed by repository-relative range and symbol identity",
		target.Location,
	)}
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: target.ID, Kind: evidence.RelationMatchesQuery,
		Certainty: evidence.CertaintyStatic, Provenance: resolutionEvidence, Scenarios: []string{activeScenarioID},
	})
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: target.ID, Kind: evidence.RelationResolvesTo,
		Certainty: evidence.CertaintyStatic, Provenance: resolutionEvidence, Scenarios: []string{activeScenarioID},
	})

	root, hierarchyOK := a.prepareHierarchy(ctx, repoPath, selected.Location.Path, confirmed, &graph)
	if !capabilityAdvertised(a.capabilities.Capabilities.CallHierarchyProvider) {
		graph.Warnings = append(graph.Warnings, "Pyright did not advertise call hierarchy support; call evidence is partial")
		hierarchyOK = false
	}
	if hierarchyOK {
		a.collectIncoming(ctx, repoPath, root, target, &graph)
		a.collectOutgoing(ctx, repoPath, root, target, &graph)
	}
	if capabilityAdvertised(a.capabilities.Capabilities.ReferencesProvider) {
		a.collectReferences(ctx, repoPath, confirmed, target, &graph)
	} else {
		graph.Warnings = append(graph.Warnings, "Pyright did not advertise references support; reference evidence is partial")
	}
	graph.Sort()
	if err := graph.Validate(); err != nil {
		return evidence.Graph{}, fmt.Errorf("pyright: invalid evidence graph: %w", err)
	}
	return graph, nil
}

func (a *Analyzer) Close(ctx context.Context) error {
	if a.client == nil {
		return nil
	}
	client := a.client
	a.client = nil
	return client.Close(ctx)
}

func (a *Analyzer) ensureSession(ctx context.Context, repoPath string) error {
	if a.client != nil {
		if a.repoPath != repoPath {
			return fmt.Errorf("pyright: analyzer session is already bound to %q", a.repoPath)
		}
		return nil
	}
	binary, err := discoverBinary(a.opts.Binary)
	if err != nil {
		return err
	}
	client, err := lspclient.Start(ctx, lspclient.Options{
		Binary: binary,
		Args:   []string{"--stdio"},
		Dir:    repoPath,
		Configuration: map[string]any{
			"python": map[string]any{
				"analysis": map[string]string{"diagnosticMode": "workspace"},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("pyright: start language server: %w", err)
	}
	a.client = client
	a.repoPath = repoPath
	a.version = binaryVersion(ctx, repoPath, binary, a.opts.RequestTimeout)

	rootURI := pathURI(repoPath)
	params := map[string]any{
		"processId":        nil,
		"rootUri":          rootURI,
		"workspaceFolders": []map[string]string{{"uri": rootURI, "name": filepath.Base(repoPath)}},
		"capabilities": map[string]any{
			"workspace": map[string]any{"configuration": true, "workspaceFolders": true},
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"callHierarchy":  map[string]any{"dynamicRegistration": false},
				"references":     map[string]any{"dynamicRegistration": false},
			},
		},
		"clientInfo": map[string]string{"name": "repomap-pyright-playground", "version": strconv.Itoa(collectorVersion)},
	}
	if err := a.call(ctx, "initialize", params, &a.capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
		return fmt.Errorf("pyright: initialize workspace: %w", err)
	}
	if a.capabilities.ServerInfo.Version != "" {
		a.version = a.capabilities.ServerInfo.Version
	}
	if err := a.client.Notify("initialized", map[string]any{}); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
		return fmt.Errorf("pyright: send initialized: %w", err)
	}
	// Pyright applies workspace settings asynchronously. A workspace-symbol
	// query with an intentionally impossible term is a response-sized index
	// barrier: the result is ignored, but the server finishes enumerating the
	// workspace before incoming-call and reference queries run.
	var ignoredWorkspaceSymbols []any
	if err := a.call(ctx, "workspace/symbol", map[string]string{
		"query": "__repomap_workspace_index_barrier_7d6a9f__",
	}, &ignoredWorkspaceSymbols); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
		return fmt.Errorf("pyright: prepare workspace index: %w", err)
	}
	if !capabilityAdvertised(a.capabilities.Capabilities.DocumentSymbolProvider) {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
		return fmt.Errorf("pyright: language server did not advertise document symbol support")
	}
	return nil
}

func (a *Analyzer) documentSymbols(ctx context.Context, repoPath, relativePath string) ([]documentSymbol, error) {
	uri, err := a.openDocument(repoPath, relativePath)
	if err != nil {
		return nil, err
	}
	var symbols []documentSymbol
	if err := a.call(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": uri},
	}, &symbols); err != nil {
		return nil, err
	}
	return flattenSymbols(symbols), nil
}

func (a *Analyzer) openDocument(repoPath, relativePath string) (string, error) {
	absPath := filepath.Join(repoPath, filepath.FromSlash(relativePath))
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("pyright: inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourceBytes {
		return "", fmt.Errorf("pyright: source file must be regular and at most %d bytes", maxSourceBytes)
	}
	uri := pathURI(absPath)
	if _, exists := a.opened[uri]; exists {
		return uri, nil
	}
	source, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("pyright: read source file: %w", err)
	}
	if err := a.client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "python", "version": 1, "text": string(source),
		},
	}); err != nil {
		return "", fmt.Errorf("pyright: open source document: %w", err)
	}
	a.opened[uri] = struct{}{}
	return uri, nil
}

func (a *Analyzer) prepareHierarchy(ctx context.Context, repoPath, relativePath string, symbol documentSymbol, graph *evidence.Graph) (callHierarchyItem, bool) {
	uri := pathURI(filepath.Join(repoPath, filepath.FromSlash(relativePath)))
	var items []callHierarchyItem
	err := a.call(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     symbol.SelectionRange.Start,
	}, &items)
	if err != nil {
		graph.Warnings = append(graph.Warnings, "Pyright call hierarchy is unavailable: "+err.Error())
		return callHierarchyItem{}, false
	}
	if len(items) == 0 {
		graph.Warnings = append(graph.Warnings, "Pyright returned no call hierarchy item; call evidence is partial")
		return callHierarchyItem{}, false
	}
	root := items[0]
	if root.URI != uri || root.Name != symbol.Name || root.SelectionRange.Start != symbol.SelectionRange.Start {
		graph.Warnings = append(graph.Warnings, "Pyright call hierarchy root did not confirm the selected declaration; call evidence omitted")
		return callHierarchyItem{}, false
	}
	return root, true
}

func (a *Analyzer) collectIncoming(ctx context.Context, repoPath string, root callHierarchyItem, target evidence.Entity, graph *evidence.Graph) {
	var calls []incomingCall
	if err := a.call(ctx, "callHierarchy/incomingCalls", map[string]any{"item": root}, &calls); err != nil {
		graph.Warnings = append(graph.Warnings, "Pyright incoming calls are unavailable: "+err.Error())
		return
	}
	calls = dedupeIncomingCalls(repoPath, calls)
	if len(calls) > a.opts.MaxIncoming {
		graph.Warnings = append(graph.Warnings, fmt.Sprintf("Pyright incoming calls truncated from %d to %d", len(calls), a.opts.MaxIncoming))
		calls = calls[:a.opts.MaxIncoming]
	}
	for _, call := range calls {
		caller, scope := hierarchyEntity(repoPath, call.From)
		graph.AddEntity(caller)
		locations := callsiteLocations(repoPath, call.From.URI, call.FromRanges)
		if len(locations) == 0 {
			locations = []evidence.Location{{}}
		}
		for _, location := range locations {
			graph.AddRelation(callRelation(caller.ID, target.ID, "callHierarchy/incomingCalls", scope, location, a))
		}
	}
}

func (a *Analyzer) collectOutgoing(ctx context.Context, repoPath string, root callHierarchyItem, target evidence.Entity, graph *evidence.Graph) {
	var calls []outgoingCall
	if err := a.call(ctx, "callHierarchy/outgoingCalls", map[string]any{"item": root}, &calls); err != nil {
		graph.Warnings = append(graph.Warnings, "Pyright outgoing calls are unavailable: "+err.Error())
		return
	}
	calls = dedupeOutgoingCalls(repoPath, root.URI, calls)
	if len(calls) > a.opts.MaxOutgoing {
		graph.Warnings = append(graph.Warnings, fmt.Sprintf("Pyright outgoing calls truncated from %d to %d", len(calls), a.opts.MaxOutgoing))
		calls = calls[:a.opts.MaxOutgoing]
	}
	for _, call := range calls {
		callee, scope := hierarchyEntity(repoPath, call.To)
		graph.AddEntity(callee)
		locations := callsiteLocations(repoPath, root.URI, call.FromRanges)
		if len(locations) == 0 {
			locations = []evidence.Location{{}}
		}
		for _, location := range locations {
			graph.AddRelation(callRelation(target.ID, callee.ID, "callHierarchy/outgoingCalls", scope, location, a))
		}
		if isDynamicBoundary(call.To.Name) {
			graph.Warnings = appendUniqueWarning(graph.Warnings, fmt.Sprintf("dynamic dispatch through %s remains unresolved; no runtime target was invented", call.To.Name))
		}
	}
}

func (a *Analyzer) collectReferences(ctx context.Context, repoPath string, symbol documentSymbol, target evidence.Entity, graph *evidence.Graph) {
	uri := pathURI(filepath.Join(repoPath, filepath.FromSlash(target.Location.Path)))
	var locations []lspLocation
	if err := a.call(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     symbol.SelectionRange.Start,
		"context":      map[string]bool{"includeDeclaration": true},
	}, &locations); err != nil {
		graph.Warnings = append(graph.Warnings, "Pyright references are unavailable: "+err.Error())
		return
	}
	references := repositoryLocations(repoPath, locations)
	if len(references) == 0 {
		graph.Warnings = append(graph.Warnings, "Pyright returned no repository references; reference evidence is partial")
		return
	}
	if len(references) > a.opts.MaxReferences {
		graph.Warnings = append(graph.Warnings, fmt.Sprintf("Pyright references truncated from %d to %d", len(references), a.opts.MaxReferences))
		references = references[:a.opts.MaxReferences]
	}
	for _, location := range references {
		reference := evidence.Entity{
			ID:       entityID("reference", location, ""),
			Kind:     evidence.EntityReference,
			Name:     fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column),
			Language: "python",
			Scope:    evidence.SourceScopeRepository,
			Location: cloneLocation(location),
		}
		graph.AddEntity(reference)
		graph.AddRelation(evidence.Relation{
			From: reference.ID, To: target.ID, Kind: evidence.RelationReferences,
			Certainty:  evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{a.provenance("textDocument/references", "reference reported by Pyright", &location)},
			Scenarios:  []string{activeScenarioID},
		})
	}
}

func (a *Analyzer) call(ctx context.Context, method string, params, result any) error {
	requestCtx, cancel := context.WithTimeout(ctx, a.opts.RequestTimeout)
	defer cancel()
	return a.client.Call(requestCtx, method, params, result)
}

func (a *Analyzer) provenance(operation, detail string, location *evidence.Location) evidence.Provenance {
	return evidence.Provenance{Provider: "pyright", Version: a.version, Operation: operation, Detail: detail, Location: cloneLocationPointer(location)}
}

func (a *Analyzer) scenario() evidence.Scenario {
	return evidence.Scenario{
		ID:         activeScenarioID,
		Name:       "Pyright workspace configuration",
		WorkingDir: ".",
		Env: map[string]string{
			"diagnostic_mode":         "workspace",
			"workspace_index_barrier": "workspace/symbol",
		},
	}
}

func discoverBinary(configured string) (string, error) {
	binary := strings.TrimSpace(configured)
	if binary == "" {
		binary = "pyright-langserver"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("pyright: %q not found; install the official tool with `npm install -g pyright` or pass --pyright-langserver (https://github.com/microsoft/pyright)", binary)
	}
	return path, nil
}

func binaryVersion(ctx context.Context, repoPath, binary string, timeout time.Duration) string {
	candidates := []string{binary}
	if filepath.Base(binary) == "pyright-langserver" {
		candidates = append([]string{filepath.Join(filepath.Dir(binary), "pyright")}, candidates...)
	}
	for _, candidate := range candidates {
		versionCtx, cancel := context.WithTimeout(ctx, timeout)
		command := exec.CommandContext(versionCtx, candidate, "--version")
		command.Dir = repoPath
		output, err := command.CombinedOutput()
		cancel()
		if err == nil {
			return strings.TrimSpace(string(output))
		}
	}
	return "unknown"
}

func normalizeRequestLocation(repoPath string, location evidence.Location) (string, evidence.Location, error) {
	if strings.TrimSpace(repoPath) == "" || strings.TrimSpace(location.Path) == "" || location.Line <= 0 || location.Column < 0 {
		return "", evidence.Location{}, fmt.Errorf("pyright: repository, path, and positive line are required")
	}
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", evidence.Location{}, fmt.Errorf("pyright: resolve repository: %w", err)
	}
	repoPath, err = filepath.EvalSymlinks(repoPath)
	if err != nil {
		return "", evidence.Location{}, fmt.Errorf("pyright: resolve repository symlinks: %w", err)
	}
	absPath := filepath.Join(repoPath, filepath.FromSlash(location.Path))
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", evidence.Location{}, fmt.Errorf("pyright: resolve source path: %w", err)
	}
	relativePath, ok := repositoryRelative(repoPath, absPath)
	if !ok || !strings.HasSuffix(strings.ToLower(relativePath), ".py") {
		return "", evidence.Location{}, fmt.Errorf("pyright: source path must be a Python file inside the repository")
	}
	location.Path = relativePath
	return repoPath, location, nil
}

func normalizeExactSymbolRequest(req analysis.ExactSymbolRequest) (string, evidence.Entity, error) {
	if req.Symbol.Location == nil || strings.TrimSpace(req.Symbol.Name) == "" {
		return "", evidence.Entity{}, fmt.Errorf("pyright: exact symbol name and location are required")
	}
	if req.Symbol.Language != "" && req.Symbol.Language != "python" {
		return "", evidence.Entity{}, fmt.Errorf("pyright: exact symbol language must be Python")
	}
	if !isCallableKind(req.Symbol.Kind) {
		return "", evidence.Entity{}, fmt.Errorf("pyright: exact symbol must be callable")
	}
	repoPath, location, err := normalizeRequestLocation(req.RepoPath, *req.Symbol.Location)
	if err != nil {
		return "", evidence.Entity{}, err
	}
	req.Symbol.Location = &location
	return repoPath, req.Symbol, nil
}

func flattenSymbols(symbols []documentSymbol) []documentSymbol {
	result := make([]documentSymbol, 0, len(symbols))
	var visit func([]documentSymbol)
	visit = func(items []documentSymbol) {
		for _, item := range items {
			children := item.Children
			item.Children = nil
			result = append(result, item)
			visit(children)
		}
	}
	visit(symbols)
	return result
}

func selectSymbols(symbols []documentSymbol, location evidence.Location) []documentSymbol {
	line := location.Line - 1
	columnSpecified := location.Column > 0
	column := max(0, location.Column-1)
	result := make([]documentSymbol, 0)
	for _, symbol := range symbols {
		if !isCallableLSPKind(symbol.Kind) || symbol.SelectionRange.Start.Line != line {
			continue
		}
		if columnSpecified && !containsPosition(symbol.SelectionRange, position{Line: line, Character: column}) {
			continue
		}
		result = append(result, symbol)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SelectionRange.Start.Character < result[j].SelectionRange.Start.Character
	})
	return result
}

func confirmDocumentSymbol(symbols []documentSymbol, selected evidence.Entity) (documentSymbol, bool) {
	for _, symbol := range symbols {
		location := lspRangeLocation(selected.Location.Path, symbol.Range)
		if symbol.Name == selected.Name && location == *selected.Location && entityKind(symbol.Kind) == selected.Kind {
			return symbol, true
		}
	}
	return documentSymbol{}, false
}

func documentEntity(path string, symbol documentSymbol) evidence.Entity {
	location := lspRangeLocation(path, symbol.Range)
	return evidence.Entity{
		ID: entityID("symbol", location, symbol.Name), Kind: entityKind(symbol.Kind), Name: symbol.Name,
		Language: "python", Scope: evidence.SourceScopeRepository, Location: &location,
	}
}

func hierarchyEntity(repoPath string, item callHierarchyItem) (evidence.Entity, evidence.SourceScope) {
	scope, location := classifyLocation(repoPath, item.URI, item.Range)
	kind := entityKind(item.Kind)
	if scope == evidence.SourceScopeRepository && location != nil {
		return evidence.Entity{
			ID: entityID("symbol", *location, item.Name), Kind: kind, Name: item.Name,
			Language: "python", Scope: scope, Location: location,
		}, scope
	}
	idParts := []string{"pyright", "external", string(scope), string(kind), item.Name}
	return evidence.Entity{ID: strings.Join(idParts, ":"), Kind: kind, Name: item.Name, Language: "python", Scope: scope}, scope
}

func classifyLocation(repoPath, uri string, source sourceRange) (evidence.SourceScope, *evidence.Location) {
	path, ok := uriPath(uri)
	if ok {
		if relativePath, inside := repositoryRelative(repoPath, path); inside {
			location := lspRangeLocation(relativePath, source)
			return evidence.SourceScopeRepository, &location
		}
	}
	lower := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(lower, "/typeshed-fallback/stdlib/"), strings.Contains(lower, "/typeshed/stdlib/"):
		return evidence.SourceScopeStandardLibrary, nil
	case strings.Contains(lower, "/typeshed-fallback/stubs/"), strings.Contains(lower, "/typeshed/stubs/"), strings.Contains(lower, "/site-packages/"), strings.Contains(lower, "/dist-packages/"):
		return evidence.SourceScopeDependency, nil
	default:
		return evidence.SourceScopeOutsideWorkspace, nil
	}
}

func callsiteLocations(repoPath, uri string, ranges []sourceRange) []evidence.Location {
	path, ok := uriPath(uri)
	if !ok {
		return nil
	}
	relativePath, ok := repositoryRelative(repoPath, path)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]evidence.Location, 0, len(ranges))
	for _, source := range ranges {
		location := lspRangeLocation(relativePath, source)
		key := locationKey(location)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, location)
	}
	return result
}

func repositoryLocations(repoPath string, locations []lspLocation) []evidence.Location {
	seen := map[string]struct{}{}
	result := make([]evidence.Location, 0, len(locations))
	for _, item := range locations {
		path, ok := uriPath(item.URI)
		if !ok {
			continue
		}
		relativePath, ok := repositoryRelative(repoPath, path)
		if !ok {
			continue
		}
		location := lspRangeLocation(relativePath, item.Range)
		key := locationKey(location)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, location)
	}
	sort.Slice(result, func(i, j int) bool { return locationKey(result[i]) < locationKey(result[j]) })
	return result
}

func callRelation(from, to, operation string, scope evidence.SourceScope, location evidence.Location, analyzer *Analyzer) evidence.Relation {
	detail := "direct static call reported by Pyright; target scope=" + string(scope)
	var pointer *evidence.Location
	if location.Path != "" {
		pointer = &location
	}
	return evidence.Relation{
		From: from, To: to, Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{analyzer.provenance(operation, detail, pointer)},
		Scenarios:  []string{activeScenarioID},
	}
}

func entityKind(kind int) evidence.EntityKind {
	switch kind {
	case 5:
		return evidence.EntityType
	case 6:
		return evidence.EntityMethod
	case 9, 12:
		return evidence.EntityFunction
	default:
		return evidence.EntityUnknown
	}
}

func isCallableLSPKind(kind int) bool { return kind == 6 || kind == 9 || kind == 12 }

func isCallableKind(kind evidence.EntityKind) bool {
	return kind == evidence.EntityFunction || kind == evidence.EntityMethod
}

func isDynamicBoundary(name string) bool {
	switch name {
	case "getattr", "setattr", "__import__", "import_module":
		return true
	default:
		return false
	}
}

func containsPosition(source sourceRange, point position) bool {
	if point.Line < source.Start.Line || point.Line > source.End.Line {
		return false
	}
	if point.Line == source.Start.Line && point.Character < source.Start.Character {
		return false
	}
	if point.Line == source.End.Line && point.Character > source.End.Character {
		return false
	}
	return true
}

func lspRangeLocation(path string, source sourceRange) evidence.Location {
	return evidence.Location{
		Path: filepath.ToSlash(path), Line: source.Start.Line + 1, Column: source.Start.Character + 1,
		EndLine: source.End.Line + 1, EndColumn: source.End.Character + 1,
	}
}

func pathURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func uriPath(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(filepath.FromSlash(path)), true
}

func repositoryRelative(repoPath, path string) (string, bool) {
	relativePath, err := filepath.Rel(repoPath, path)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relativePath), true
}

func entityID(prefix string, location evidence.Location, name string) string {
	return fmt.Sprintf("pyright:%s:%s:%d:%d:%s", prefix, location.Path, location.Line, location.Column, name)
}

func locationKey(location evidence.Location) string {
	return fmt.Sprintf("%s:%09d:%09d:%09d:%09d", location.Path, location.Line, location.Column, location.EndLine, location.EndColumn)
}

func cloneLocation(location evidence.Location) *evidence.Location {
	copy := location
	return &copy
}

func cloneLocationPointer(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	return cloneLocation(*location)
}

func appendUniqueWarning(warnings []string, value string) []string {
	for _, warning := range warnings {
		if warning == value {
			return warnings
		}
	}
	return append(warnings, value)
}

func capabilityAdvertised(value any) bool {
	if value == nil {
		return false
	}
	if enabled, ok := value.(bool); ok {
		return enabled
	}
	return true
}

func dedupeIncomingCalls(repoPath string, calls []incomingCall) []incomingCall {
	seen := map[string]struct{}{}
	result := make([]incomingCall, 0, len(calls))
	for _, call := range calls {
		entity, _ := hierarchyEntity(repoPath, call.From)
		key := entity.ID + ":" + rangesKey(callsiteLocations(repoPath, call.From.URI, call.FromRanges))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, call)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := hierarchyEntity(repoPath, result[i].From)
		right, _ := hierarchyEntity(repoPath, result[j].From)
		return left.ID+rangesKey(callsiteLocations(repoPath, result[i].From.URI, result[i].FromRanges)) <
			right.ID+rangesKey(callsiteLocations(repoPath, result[j].From.URI, result[j].FromRanges))
	})
	return result
}

func dedupeOutgoingCalls(repoPath, callerURI string, calls []outgoingCall) []outgoingCall {
	seen := map[string]struct{}{}
	result := make([]outgoingCall, 0, len(calls))
	for _, call := range calls {
		entity, _ := hierarchyEntity(repoPath, call.To)
		key := entity.ID + ":" + rangesKey(callsiteLocations(repoPath, callerURI, call.FromRanges))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, call)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := hierarchyEntity(repoPath, result[i].To)
		right, _ := hierarchyEntity(repoPath, result[j].To)
		return left.ID+rangesKey(callsiteLocations(repoPath, callerURI, result[i].FromRanges)) <
			right.ID+rangesKey(callsiteLocations(repoPath, callerURI, result[j].FromRanges))
	})
	return result
}

func rangesKey(locations []evidence.Location) string {
	parts := make([]string, 0, len(locations))
	for _, location := range locations {
		parts = append(parts, locationKey(location))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
