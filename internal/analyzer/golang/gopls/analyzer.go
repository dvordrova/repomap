// Package gopls adapts the experimental gopls command-line interface to the
// language-neutral evidence graph. It is intentionally isolated so the CLI
// transport can later be replaced by a long-lived LSP client.
package gopls

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
)

type Options struct {
	Binary                 string
	MaxSymbols             int
	MaxCallRoots           int
	MaxCallers             int
	MaxCallees             int
	MaxImplementationRoots int
	CommandTimeout         time.Duration
	IncludeExternal        bool
	IncludeImplementations bool
}

type Analyzer struct {
	opts   Options
	runner commandRunner
}

type commandRunner interface {
	Run(ctx context.Context, dir, binary string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, dir, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", binary, strings.Join(args, " "), err)
	}
	return output, nil
}

type symbol struct {
	Name     string
	Kind     evidence.EntityKind
	Location evidence.Location
}

type callEdge struct {
	Direction string
	Symbol    symbol
	Callsite  evidence.Location
}

const activeBuildScenarioID = "gopls-active-build"

// CollectorVersion identifies the adapter semantics used to turn gopls CLI
// output into evidence. Bump it when parsing or evidence construction changes.
const CollectorVersion = 2

var _ analysis.Provider = (*Analyzer)(nil)
var _ analysis.LocationResolver = (*Analyzer)(nil)
var _ analysis.ExactSymbolAnalyzer = (*Analyzer)(nil)

func New(opts Options) *Analyzer {
	return newWithRunner(opts, execRunner{})
}

func newWithRunner(opts Options, runner commandRunner) *Analyzer {
	if opts.Binary == "" {
		opts.Binary = "gopls"
	}
	if opts.MaxSymbols <= 0 {
		opts.MaxSymbols = 40
	}
	if opts.MaxCallRoots <= 0 {
		opts.MaxCallRoots = 3
	}
	if opts.MaxCallers <= 0 {
		opts.MaxCallers = 10
	}
	if opts.MaxCallees <= 0 {
		opts.MaxCallees = 10
	}
	if opts.MaxImplementationRoots <= 0 {
		opts.MaxImplementationRoots = 2
	}
	if opts.CommandTimeout <= 0 {
		opts.CommandTimeout = 30 * time.Second
	}
	return &Analyzer{opts: opts, runner: runner}
}

func (a *Analyzer) Analyze(ctx context.Context, req analysis.Request) (evidence.Graph, error) {
	if strings.TrimSpace(req.Query) == "" {
		return evidence.Graph{}, fmt.Errorf("gopls: query is required")
	}
	repoPath, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: resolve repo path: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(repoPath); resolveErr == nil {
		repoPath = resolved
	}

	graph := evidence.NewGraph(repoPath, req.Query)
	graph.Build = evidence.BuildContext{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	graph.Scenarios = append(graph.Scenarios, evidence.Scenario{
		ID:         activeBuildScenarioID,
		Name:       "gopls active build configuration",
		WorkingDir: repoPath,
		Build:      graph.Build,
	})
	graph.Warnings = append(graph.Warnings, "gopls CLI adapter is experimental; evidence is scoped to the active build configuration")

	version := a.version(ctx, repoPath)
	queryEntity := evidence.Entity{
		ID:   queryEntityID(req.Query),
		Kind: evidence.EntityQuery,
		Name: req.Query,
	}
	graph.AddEntity(queryEntity)

	output, err := a.run(ctx, repoPath, "workspace_symbol", "-matcher", "fuzzy", req.Query)
	if err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: workspace symbol query: %w", err)
	}
	symbols := parseWorkspaceSymbols(output)
	localSymbols := make([]symbol, 0, len(symbols))
	for _, item := range symbols {
		item, ok := normalizeSymbol(repoPath, item, a.opts.IncludeExternal)
		if !ok {
			continue
		}
		localSymbols = append(localSymbols, item)
	}
	resolved, hasResolution := resolveExactSymbol(req.Query, localSymbols)
	if hasResolution {
		localSymbols = prioritizeSymbol(localSymbols, resolved)
	}
	if len(localSymbols) > a.opts.MaxSymbols {
		localSymbols = localSymbols[:a.opts.MaxSymbols]
		graph.Warnings = append(graph.Warnings, fmt.Sprintf("workspace symbols truncated to %d", a.opts.MaxSymbols))
	}

	for _, item := range localSymbols {
		entity := symbolEntity(item)
		graph.AddEntity(entity)
		graph.AddRelation(evidence.Relation{
			From:      queryEntity.ID,
			To:        entity.ID,
			Kind:      evidence.RelationMatchesQuery,
			Certainty: evidence.CertaintyPossible,
			Provenance: []evidence.Provenance{{
				Provider:  "gopls",
				Version:   version,
				Operation: "workspace_symbol",
				Detail:    "fuzzy workspace symbol match",
				Location:  cloneLocation(entity.Location),
			}},
			Scenarios: []string{activeBuildScenarioID},
		})
	}
	if hasResolution {
		resolvedEntity := symbolEntity(resolved)
		graph.AddRelation(evidence.Relation{
			From:      queryEntity.ID,
			To:        resolvedEntity.ID,
			Kind:      evidence.RelationResolvesTo,
			Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider:  "gopls",
				Version:   version,
				Operation: "workspace_symbol",
				Detail:    "unique exact symbol name match",
				Location:  cloneLocation(resolvedEntity.Location),
			}},
			Scenarios: []string{activeBuildScenarioID},
		})
	} else {
		graph.Warnings = append(graph.Warnings, fmt.Sprintf("query %q did not resolve to one unique exact symbol", req.Query))
	}

	callRoots := 0
	for _, root := range localSymbols {
		if root.Kind != evidence.EntityFunction && root.Kind != evidence.EntityMethod {
			continue
		}
		if callRoots >= a.opts.MaxCallRoots {
			break
		}
		callRoots++
		callOutput, callErr := a.run(ctx, repoPath, "call_hierarchy", positionArg(repoPath, root.Location))
		if callErr != nil {
			graph.Warnings = append(graph.Warnings, fmt.Sprintf("call hierarchy for %s: %v", root.Name, callErr))
			continue
		}
		hierarchyRoot, edges := parseCallHierarchy(callOutput)
		hierarchyRoot, ok := normalizeSymbol(repoPath, hierarchyRoot, a.opts.IncludeExternal)
		if !ok {
			continue
		}
		hierarchyRoot = canonicalizeHierarchyRoot(root, hierarchyRoot)
		a.addCallEdges(repoPath, version, hierarchyRoot, edges, &graph)
	}

	if a.opts.IncludeImplementations {
		a.addImplementations(ctx, repoPath, version, localSymbols, &graph)
	}

	graph.Sort()
	if err := graph.Validate(); err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: invalid evidence graph: %w", err)
	}
	return graph, nil
}

// AnalyzeExactSymbol confirms and analyzes one declaration by its exact source
// position. It deliberately does not run workspace_symbol: a selected Run in
// one file must not be re-resolved by name to another Run elsewhere.
func (a *Analyzer) AnalyzeExactSymbol(ctx context.Context, req analysis.ExactSymbolRequest) (evidence.Graph, error) {
	selected, repoPath, err := normalizeExactSymbolRequest(req)
	if err != nil {
		return evidence.Graph{}, err
	}

	output, err := a.run(ctx, repoPath, "call_hierarchy", positionArg(repoPath, selected.Location))
	if err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: call hierarchy for selected declaration: %w", err)
	}
	reported, edges := parseCallHierarchy(output)
	reported, ok := normalizeSymbol(repoPath, reported, false)
	if !ok || (reported.Kind != evidence.EntityFunction && reported.Kind != evidence.EntityMethod) {
		return evidence.Graph{}, fmt.Errorf("gopls: call hierarchy did not identify the selected declaration")
	}
	if !samePosition(selected.Location, reported.Location) {
		return evidence.Graph{}, fmt.Errorf(
			"gopls: call hierarchy root %s:%d:%d does not match selected declaration %s:%d:%d",
			reported.Location.Path,
			reported.Location.Line,
			reported.Location.Column,
			selected.Location.Path,
			selected.Location.Line,
			selected.Location.Column,
		)
	}
	if callableName(selected.Name) != callableName(reported.Name) {
		return evidence.Graph{}, fmt.Errorf(
			"gopls: call hierarchy root %q does not match selected declaration %q",
			reported.Name,
			selected.Name,
		)
	}

	version := a.version(ctx, repoPath)
	graph := evidence.NewGraph(repoPath, selected.Name)
	graph.Build = evidence.BuildContext{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	graph.Scenarios = append(graph.Scenarios, evidence.Scenario{
		ID:         activeBuildScenarioID,
		Name:       "gopls active build configuration",
		WorkingDir: repoPath,
		Build:      graph.Build,
	})
	graph.Warnings = append(graph.Warnings, "gopls CLI adapter is experimental; evidence is scoped to the active build configuration")

	queryEntity := evidence.Entity{ID: queryEntityID(selected.Name), Kind: evidence.EntityQuery, Name: selected.Name}
	targetEntity := symbolEntity(selected)
	graph.AddEntity(queryEntity)
	graph.AddEntity(targetEntity)
	resolutionProvenance := []evidence.Provenance{{
		Provider:  "gopls",
		Version:   version,
		Operation: "call_hierarchy",
		Detail:    "selected declaration confirmed at its exact source position",
		Location:  cloneLocation(targetEntity.Location),
	}}
	graph.AddRelation(evidence.Relation{
		From:       queryEntity.ID,
		To:         targetEntity.ID,
		Kind:       evidence.RelationMatchesQuery,
		Certainty:  evidence.CertaintyStatic,
		Provenance: resolutionProvenance,
		Scenarios:  []string{activeBuildScenarioID},
	})
	graph.AddRelation(evidence.Relation{
		From:       queryEntity.ID,
		To:         targetEntity.ID,
		Kind:       evidence.RelationResolvesTo,
		Certainty:  evidence.CertaintyStatic,
		Provenance: resolutionProvenance,
		Scenarios:  []string{activeBuildScenarioID},
	})

	// gopls may spell a method name differently between document symbols and
	// call hierarchy. The position and callable name were confirmed above, so
	// preserve the selected document-symbol identity in the resulting graph.
	a.addCallEdges(repoPath, version, selected, edges, &graph)
	graph.Sort()
	if err := graph.Validate(); err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: invalid exact-symbol evidence graph: %w", err)
	}
	return graph, nil
}

func normalizeExactSymbolRequest(req analysis.ExactSymbolRequest) (symbol, string, error) {
	if strings.TrimSpace(req.RepoPath) == "" {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol repository is required")
	}
	if strings.TrimSpace(req.Symbol.Name) == "" || req.Symbol.Location == nil {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol name and declaration location are required")
	}
	if req.Symbol.Kind != evidence.EntityFunction && req.Symbol.Kind != evidence.EntityMethod {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol must be a function or method")
	}
	if req.Symbol.Language != "" && req.Symbol.Language != "go" {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol language must be Go")
	}
	if req.Symbol.Location.Line <= 0 || req.Symbol.Location.Column <= 0 {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol declaration requires line and column")
	}

	repoPath, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return symbol{}, "", fmt.Errorf("gopls: resolve repo path: %w", err)
	}
	repoPath, err = filepath.EvalSymlinks(repoPath)
	if err != nil {
		return symbol{}, "", fmt.Errorf("gopls: resolve repo symlinks: %w", err)
	}
	location, ok, err := normalizeExistingRepoLocation(repoPath, *req.Symbol.Location)
	if err != nil {
		return symbol{}, "", fmt.Errorf("gopls: resolve exact symbol location: %w", err)
	}
	if !ok {
		return symbol{}, "", fmt.Errorf("gopls: exact symbol location is outside repository")
	}
	return symbol{Name: req.Symbol.Name, Kind: req.Symbol.Kind, Location: location}, repoPath, nil
}

func callableName(name string) string {
	name = strings.TrimSpace(name)
	if separator := strings.LastIndexByte(name, '.'); separator >= 0 {
		return name[separator+1:]
	}
	return name
}

func (a *Analyzer) addCallEdges(repoPath, version string, root symbol, edges []callEdge, graph *evidence.Graph) {
	rootEntity := symbolEntity(root)
	graph.AddEntity(rootEntity)
	var callers []callEdge
	var callees []callEdge
	for _, edge := range edges {
		target, targetOK := normalizeSymbol(repoPath, edge.Symbol, a.opts.IncludeExternal)
		if !targetOK || samePosition(target.Location, root.Location) {
			continue
		}
		callsite, callsiteOK := normalizeLocation(repoPath, edge.Callsite, a.opts.IncludeExternal)
		if !callsiteOK {
			continue
		}
		edge.Symbol = target
		edge.Callsite = callsite
		switch edge.Direction {
		case "caller":
			callers = append(callers, edge)
		case "callee":
			callees = append(callees, edge)
		}
	}

	sortCallEdges(callers, true)
	sortCallEdges(callees, false)
	omittedCallers := max(0, len(callers)-a.opts.MaxCallers)
	omittedCallees := max(0, len(callees)-a.opts.MaxCallees)
	callers = callers[:min(len(callers), a.opts.MaxCallers)]
	callees = callees[:min(len(callees), a.opts.MaxCallees)]
	appendCallLimitWarning(graph, "incoming", omittedCallers, a.opts.MaxCallers)
	appendCallLimitWarning(graph, "outgoing", omittedCallees, a.opts.MaxCallees)

	for _, edge := range append(callers, callees...) {
		target := edge.Symbol
		callsite := edge.Callsite
		targetEntity := symbolEntity(target)
		graph.AddEntity(targetEntity)
		from, to := rootEntity.ID, targetEntity.ID
		if edge.Direction == "caller" {
			from, to = targetEntity.ID, rootEntity.ID
		}
		graph.AddRelation(evidence.Relation{
			From:      from,
			To:        to,
			Kind:      evidence.RelationCalls,
			Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider:  "gopls",
				Version:   version,
				Operation: "call_hierarchy",
				Detail:    "direct call reported by gopls",
				Location:  &callsite,
			}},
			Scenarios: []string{activeBuildScenarioID},
		})
	}
}

func sortCallEdges(edges []callEdge, rankLowSignal bool) {
	sort.SliceStable(edges, func(i, j int) bool {
		if rankLowSignal {
			leftTier := callPathTier(edges[i].Symbol.Location.Path)
			rightTier := callPathTier(edges[j].Symbol.Location.Path)
			if leftTier != rightTier {
				return leftTier < rightTier
			}
		}
		left := edges[i].Symbol
		right := edges[j].Symbol
		if left.Location.Path != right.Location.Path {
			return left.Location.Path < right.Location.Path
		}
		if left.Location.Line != right.Location.Line {
			return left.Location.Line < right.Location.Line
		}
		if left.Location.Column != right.Location.Column {
			return left.Location.Column < right.Location.Column
		}
		return left.Name < right.Name
	})
}

func callPathTier(filePath string) int {
	lower := strings.ToLower(filepath.ToSlash(filePath))
	segments := strings.Split(lower, "/")
	base := segments[len(segments)-1]
	if strings.HasSuffix(base, "_test.go") {
		return 2
	}
	for _, segment := range segments[:len(segments)-1] {
		switch segment {
		case "bench", "benchmark", "benchmarks", "test", "testdata", "tests":
			return 2
		}
	}
	return 0
}

func appendCallLimitWarning(graph *evidence.Graph, direction string, omitted, limit int) {
	if omitted <= 0 {
		return
	}
	warning := fmt.Sprintf(
		"gopls bounded static call hierarchy: omitted %d %s calls at analyzer limit %d",
		omitted,
		direction,
		limit,
	)
	for _, existing := range graph.Warnings {
		if existing == warning {
			return
		}
	}
	graph.Warnings = append(graph.Warnings, warning)
}

// References returns deterministic repository-relative locations together
// with the provider provenance and active build scenario that made them
// visible. It does not infer how a reference is exercised at runtime or what a
// test asserts.
func (a *Analyzer) References(ctx context.Context, repoPath string, location evidence.Location) (evidence.LocationSet, error) {
	if location.Path == "" || location.Line <= 0 || location.Column <= 0 {
		return evidence.LocationSet{}, fmt.Errorf("gopls: reference location requires path, line, and column")
	}
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return evidence.LocationSet{}, fmt.Errorf("gopls: resolve repo path: %w", err)
	}
	repoPath, err = filepath.EvalSymlinks(repoPath)
	if err != nil {
		return evidence.LocationSet{}, fmt.Errorf("gopls: resolve repo symlinks: %w", err)
	}
	location, ok, err := normalizeExistingRepoLocation(repoPath, location)
	if err != nil {
		return evidence.LocationSet{}, fmt.Errorf("gopls: resolve reference location: %w", err)
	}
	if !ok {
		return evidence.LocationSet{}, fmt.Errorf("gopls: reference location is outside repository")
	}
	output, err := a.run(ctx, repoPath, "references", positionArg(repoPath, location))
	if err != nil {
		return evidence.LocationSet{}, fmt.Errorf("gopls: references: %w", err)
	}

	seen := make(map[string]struct{})
	result := make([]evidence.Location, 0)
	for _, candidate := range parseLocations(output) {
		candidate, ok, resolveErr := normalizeExistingRepoLocation(repoPath, candidate)
		if resolveErr != nil || !ok {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d:%d:%d", candidate.Path, candidate.Line, candidate.Column, candidate.EndLine, candidate.EndColumn)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	set := evidence.LocationSet{
		Locations: result,
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider:  "gopls",
			Version:   a.version(ctx, repoPath),
			Operation: "references",
			Detail:    "source references reported by gopls",
			Location:  cloneLocation(&location),
		}},
		Scenarios: []evidence.Scenario{{
			ID:         activeBuildScenarioID,
			Name:       "gopls active build configuration",
			WorkingDir: repoPath,
			Build:      evidence.BuildContext{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
		}},
	}
	if err := set.Validate(); err != nil {
		return evidence.LocationSet{}, fmt.Errorf("gopls: invalid reference evidence: %w", err)
	}
	return set, nil
}

type documentSymbol struct {
	Name     string
	Kind     evidence.EntityKind
	Location evidence.Location
	Depth    int
}

func (a *Analyzer) ResolveLocation(ctx context.Context, req analysis.LocationRequest) (analysis.LocationResolution, error) {
	if strings.TrimSpace(req.RepoPath) == "" || req.Location.Path == "" || req.Location.Line <= 0 {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: location resolution requires repository, path, and line")
	}
	repoPath, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: resolve repo path: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(repoPath); resolveErr == nil {
		repoPath = resolved
	}
	location, ok, err := normalizeExistingRepoLocation(repoPath, req.Location)
	if err != nil {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: resolve source location: %w", err)
	}
	if !ok {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: source location is outside repository")
	}
	absPath := filepath.Join(repoPath, filepath.FromSlash(location.Path))
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: source file is unavailable")
	}
	if info.Size() > 4*1024*1024 {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: source file exceeds 4 MiB")
	}
	source, err := os.ReadFile(absPath)
	if err != nil {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: read source file: %w", err)
	}
	lines := strings.Split(string(source), "\n")
	if location.Line > len(lines) {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: source line %d exceeds file length %d", location.Line, len(lines))
	}
	output, err := a.run(ctx, repoPath, "symbols", absPath)
	if err != nil {
		return analysis.LocationResolution{}, fmt.Errorf("gopls: document symbols: %w", err)
	}
	symbols := parseDocumentSymbols(output, location.Path)
	maxCandidates := req.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 5
	}
	if maxCandidates > 20 {
		maxCandidates = 20
	}
	candidates := selectLocationCandidates(symbols, lines, location.Line, maxCandidates, req.RankTerms)
	version := a.version(ctx, repoPath)
	result := analysis.LocationResolution{
		Location:   location,
		Candidates: candidates,
		Certainty:  evidence.CertaintyStatic,
		Provenance: evidence.Provenance{
			Provider:  "gopls",
			Version:   version,
			Operation: "document_symbols",
			Detail:    "bounded declarations around a repository source location",
			Location:  cloneLocation(&location),
		},
		Scenario: evidence.Scenario{
			ID:         activeBuildScenarioID,
			Name:       "gopls active build configuration",
			WorkingDir: repoPath,
			Build:      evidence.BuildContext{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
		},
	}
	if len(candidates) == 0 {
		result.Warnings = append(result.Warnings, "gopls returned no declaration candidate for this location")
	}
	return result, nil
}

func (a *Analyzer) addImplementations(ctx context.Context, repoPath, version string, symbols []symbol, graph *evidence.Graph) {
	roots := 0
	for _, root := range symbols {
		if root.Kind != evidence.EntityInterface && root.Kind != evidence.EntityType {
			continue
		}
		if roots >= a.opts.MaxImplementationRoots {
			return
		}
		roots++
		output, err := a.run(ctx, repoPath, "implementation", positionArg(repoPath, root.Location))
		if err != nil {
			graph.Warnings = append(graph.Warnings, fmt.Sprintf("implementations for %s: %v", root.Name, err))
			continue
		}
		rootEntity := symbolEntity(root)
		graph.AddEntity(rootEntity)
		for _, location := range parseLocations(output) {
			location, ok := normalizeLocation(repoPath, location, a.opts.IncludeExternal)
			if !ok || samePosition(root.Location, location) {
				continue
			}
			implementation := evidence.Entity{
				ID:       entityID("implementation", location, ""),
				Kind:     evidence.EntityType,
				Name:     fmt.Sprintf("%s:%d", filepath.Base(location.Path), location.Line),
				Language: "go",
				Location: &location,
			}
			graph.AddEntity(implementation)
			from, to := rootEntity.ID, implementation.ID
			if root.Kind == evidence.EntityInterface {
				from, to = implementation.ID, rootEntity.ID
			}
			graph.AddRelation(evidence.Relation{
				From:      from,
				To:        to,
				Kind:      evidence.RelationImplements,
				Certainty: evidence.CertaintyPossible,
				Provenance: []evidence.Provenance{{
					Provider:  "gopls",
					Version:   version,
					Operation: "implementation",
					Detail:    "implementation candidate reported by gopls",
					Location:  &location,
				}},
				Scenarios: []string{activeBuildScenarioID},
			})
		}
	}
}

func (a *Analyzer) version(ctx context.Context, repoPath string) string {
	output, err := a.run(ctx, repoPath, "version")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func (a *Analyzer) run(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, a.opts.CommandTimeout)
	defer cancel()
	output, err := a.runner.Run(commandCtx, repoPath, a.opts.Binary, args...)
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("command timed out after %s", a.opts.CommandTimeout)
	}
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return output, fmt.Errorf("%w: %s", err, trimmed)
		}
		return output, err
	}
	return output, nil
}

func parseWorkspaceSymbols(output []byte) []symbol {
	var symbols []symbol
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lastSpace := strings.LastIndexByte(line, ' ')
		if lastSpace <= 0 {
			continue
		}
		kindText := line[lastSpace+1:]
		rest := line[:lastSpace]
		location, name, ok := parseLocatedName(rest)
		if !ok {
			continue
		}
		symbols = append(symbols, symbol{
			Name:     name,
			Kind:     entityKind(kindText),
			Location: location,
		})
	}
	return symbols
}

func parseDocumentSymbols(output []byte, path string) []documentSymbol {
	var symbols []documentSymbol
	var parents []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		raw := scanner.Text()
		depth := 0
		for depth < len(raw) && raw[depth] == '\t' {
			depth++
		}
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) < 3 {
			continue
		}
		location, ok := parseDocumentRange(fields[len(fields)-1], path)
		if !ok {
			continue
		}
		name := strings.Join(fields[:len(fields)-2], " ")
		kind := entityKind(fields[len(fields)-2])
		if name == "" {
			continue
		}
		if depth > len(parents) {
			depth = len(parents)
		}
		if depth > 0 {
			name = parents[depth-1] + "." + name
		}
		if len(parents) <= depth {
			parents = append(parents, name)
		} else {
			parents[depth] = name
			parents = parents[:depth+1]
		}
		symbols = append(symbols, documentSymbol{Name: name, Kind: kind, Location: location, Depth: depth})
	}
	return symbols
}

func parseDocumentRange(value, path string) (evidence.Location, bool) {
	parts := strings.SplitN(value, "-", 2)
	start := strings.Split(parts[0], ":")
	if len(start) != 2 {
		return evidence.Location{}, false
	}
	line, err := strconv.Atoi(start[0])
	if err != nil || line <= 0 {
		return evidence.Location{}, false
	}
	column, err := strconv.Atoi(start[1])
	if err != nil || column <= 0 {
		return evidence.Location{}, false
	}
	location := evidence.Location{Path: path, Line: line, Column: column, EndLine: line, EndColumn: column}
	if len(parts) == 2 {
		end := strings.Split(parts[1], ":")
		if len(end) == 2 {
			location.EndLine, _ = strconv.Atoi(end[0])
			location.EndColumn, _ = strconv.Atoi(end[1])
		}
	}
	return location, true
}

func selectLocationCandidates(symbols []documentSymbol, lines []string, targetLine, limit int, rankTerms []string) []analysis.LocationCandidate {
	type ranked struct {
		symbol    documentSymbol
		match     string
		rank      int
		hintScore int
		reasons   []string
	}
	var rankedCandidates []ranked
	seen := make(map[string]struct{})
	add := func(item documentSymbol, match string, rank int) {
		key := entityID(string(item.Kind), item.Location, item.Name)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		score, reasons := symbolHintScore(item.Name, rankTerms)
		rankedCandidates = append(rankedCandidates, ranked{
			symbol:    item,
			match:     match,
			rank:      rank,
			hintScore: score,
			reasons:   reasons,
		})
	}

	for _, item := range symbols {
		if item.Location.Line == targetLine {
			add(item, "declaration", 0)
		}
	}
	for _, item := range symbols {
		if item.Location.Line > targetLine && item.Location.Line-targetLine <= 80 && commentGap(lines, targetLine, item.Location.Line) {
			add(item, "leading_comment", 1)
		}
	}
	for index, item := range symbols {
		if item.Depth != 0 || item.Location.Line > targetLine || !scopeKind(item.Kind) {
			continue
		}
		end := len(lines)
		for next := index + 1; next < len(symbols); next++ {
			if symbols[next].Depth == 0 {
				end = symbols[next].Location.Line - 1
				break
			}
		}
		if targetLine <= end {
			add(item, "preceding_declaration", 2)
		}
	}
	for _, item := range symbols {
		if item.Kind == evidence.EntityFunction || item.Kind == evidence.EntityMethod {
			add(item, "file_declaration", 3)
		}
	}

	sort.SliceStable(rankedCandidates, func(i, j int) bool {
		if rankedCandidates[i].rank != rankedCandidates[j].rank {
			return rankedCandidates[i].rank < rankedCandidates[j].rank
		}
		if rankedCandidates[i].hintScore != rankedCandidates[j].hintScore {
			return rankedCandidates[i].hintScore > rankedCandidates[j].hintScore
		}
		leftDistance := abs(rankedCandidates[i].symbol.Location.Line - targetLine)
		rightDistance := abs(rankedCandidates[j].symbol.Location.Line - targetLine)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if rankedCandidates[i].symbol.Depth != rankedCandidates[j].symbol.Depth {
			return rankedCandidates[i].symbol.Depth > rankedCandidates[j].symbol.Depth
		}
		return rankedCandidates[i].symbol.Name < rankedCandidates[j].symbol.Name
	})
	if len(rankedCandidates) > limit {
		rankedCandidates = rankedCandidates[:limit]
	}
	result := make([]analysis.LocationCandidate, 0, len(rankedCandidates))
	for _, candidate := range rankedCandidates {
		entity := symbolEntity(symbol{
			Name:     candidate.symbol.Name,
			Kind:     candidate.symbol.Kind,
			Location: candidate.symbol.Location,
		})
		result = append(result, analysis.LocationCandidate{
			Entity:       entity,
			Match:        candidate.match,
			Certainty:    locationMatchCertainty(candidate.match),
			Distance:     abs(candidate.symbol.Location.Line - targetLine),
			Investigable: candidate.symbol.Kind == evidence.EntityFunction || candidate.symbol.Kind == evidence.EntityMethod,
			RankReasons:  candidate.reasons,
		})
	}
	return result
}

func symbolHintScore(name string, terms []string) (int, []string) {
	name = strings.ToLower(name)
	seen := make(map[string]struct{})
	score := 0
	var reasons []string
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if len(term) < 3 || len(term) > 64 {
			continue
		}
		if _, ok := seen[term]; ok || !strings.Contains(name, term) {
			continue
		}
		seen[term] = struct{}{}
		score++
		reasons = append(reasons, "name matches component term '"+term+"'")
		if len(reasons) == 3 {
			break
		}
	}
	return score, reasons
}

func locationMatchCertainty(match string) evidence.Certainty {
	if match == "declaration" {
		return evidence.CertaintyStatic
	}
	return evidence.CertaintyPossible
}

func commentGap(lines []string, targetLine, declarationLine int) bool {
	if targetLine <= 0 || declarationLine <= targetLine || declarationLine > len(lines) {
		return false
	}
	inBlockComment := false
	for lineNumber := targetLine; lineNumber < declarationLine; lineNumber++ {
		line := strings.TrimSpace(lines[lineNumber-1])
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "/*") {
			inBlockComment = true
		}
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}
		return false
	}
	return true
}

func scopeKind(kind evidence.EntityKind) bool {
	return kind == evidence.EntityFunction || kind == evidence.EntityMethod ||
		kind == evidence.EntityType || kind == evidence.EntityInterface
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func resolveExactSymbol(query string, symbols []symbol) (symbol, bool) {
	var resolved symbol
	matches := 0
	for _, item := range symbols {
		if item.Name != query {
			continue
		}
		resolved = item
		matches++
	}
	return resolved, matches == 1
}

func prioritizeSymbol(symbols []symbol, target symbol) []symbol {
	prioritized := make([]symbol, 0, len(symbols))
	prioritized = append(prioritized, target)
	for _, item := range symbols {
		if sameSymbol(item, target) {
			continue
		}
		prioritized = append(prioritized, item)
	}
	return prioritized
}

func sameSymbol(left, right symbol) bool {
	return left.Name == right.Name &&
		left.Kind == right.Kind &&
		samePosition(left.Location, right.Location)
}

func canonicalizeHierarchyRoot(requested, reported symbol) symbol {
	if samePosition(requested.Location, reported.Location) {
		return requested
	}
	return reported
}

func parseCallHierarchy(output []byte) (symbol, []callEdge) {
	var root symbol
	var edges []callEdge
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "identifier: ") {
			parsed, ok := parseDescriptor(strings.TrimPrefix(line, "identifier: "))
			if ok {
				root = parsed
			}
			continue
		}
		direction := ""
		switch {
		case strings.HasPrefix(line, "caller["):
			direction = "caller"
		case strings.HasPrefix(line, "callee["):
			direction = "callee"
		default:
			continue
		}
		separator := " from/to "
		separatorIndex := strings.Index(line, separator)
		if separatorIndex < 0 {
			continue
		}
		target, ok := parseDescriptor(line[separatorIndex+len(separator):])
		if !ok {
			continue
		}
		callsiteText := line[:separatorIndex]
		rangesIndex := strings.Index(callsiteText, ": ranges ")
		if rangesIndex < 0 {
			continue
		}
		callsite, ok := parseRangeLocation(callsiteText[rangesIndex+len(": ranges "):])
		if !ok {
			continue
		}
		edges = append(edges, callEdge{Direction: direction, Symbol: target, Callsite: callsite})
	}
	return root, edges
}

func parseDescriptor(value string) (symbol, bool) {
	separatorIndex := strings.LastIndex(value, " in ")
	if separatorIndex < 0 {
		return symbol{}, false
	}
	descriptor := strings.TrimSpace(value[:separatorIndex])
	location, ok := parseLocation(strings.TrimSpace(value[separatorIndex+len(" in "):]))
	if !ok {
		return symbol{}, false
	}
	firstSpace := strings.IndexByte(descriptor, ' ')
	if firstSpace < 0 {
		return symbol{}, false
	}
	return symbol{
		Name:     strings.TrimSpace(descriptor[firstSpace+1:]),
		Kind:     entityKind(descriptor[:firstSpace]),
		Location: location,
	}, true
}

func parseLocatedName(value string) (evidence.Location, string, bool) {
	for index := 0; index < len(value); index++ {
		if value[index] != ' ' {
			continue
		}
		location, ok := parseLocation(value[:index])
		if !ok {
			continue
		}
		name := strings.TrimSpace(value[index+1:])
		if name == "" {
			return evidence.Location{}, "", false
		}
		return location, name, true
	}
	return evidence.Location{}, "", false
}

func parseRangeLocation(value string) (evidence.Location, bool) {
	separatorIndex := strings.LastIndex(value, " in ")
	if separatorIndex < 0 {
		return evidence.Location{}, false
	}
	rangeText := strings.TrimSpace(value[:separatorIndex])
	path := strings.TrimSpace(value[separatorIndex+len(" in "):])
	parts := strings.Split(rangeText, ":")
	if len(parts) != 2 {
		return evidence.Location{}, false
	}
	line, err := strconv.Atoi(parts[0])
	if err != nil {
		return evidence.Location{}, false
	}
	columns := strings.SplitN(parts[1], "-", 2)
	column, err := strconv.Atoi(columns[0])
	if err != nil {
		return evidence.Location{}, false
	}
	location := evidence.Location{Path: path, Line: line, Column: column, EndLine: line}
	if len(columns) == 2 {
		location.EndColumn, _ = strconv.Atoi(columns[1])
	}
	return location, true
}

func parseLocations(output []byte) []evidence.Location {
	var locations []evidence.Location
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		location, ok := parseLocation(strings.TrimSpace(scanner.Text()))
		if ok {
			locations = append(locations, location)
		}
	}
	return locations
}

func parseLocation(value string) (evidence.Location, bool) {
	lastColon := strings.LastIndexByte(value, ':')
	if lastColon < 0 {
		return evidence.Location{}, false
	}
	columnText := value[lastColon+1:]
	pathAndLine := value[:lastColon]
	lineColon := strings.LastIndexByte(pathAndLine, ':')
	if lineColon < 0 {
		return evidence.Location{}, false
	}
	lineText := pathAndLine[lineColon+1:]
	path := pathAndLine[:lineColon]

	columnParts := strings.SplitN(columnText, "-", 2)
	column, err := strconv.Atoi(columnParts[0])
	if err != nil {
		return evidence.Location{}, false
	}
	line, err := strconv.Atoi(lineText)
	if err != nil || path == "" {
		return evidence.Location{}, false
	}
	location := evidence.Location{Path: path, Line: line, Column: column, EndLine: line}
	if len(columnParts) == 2 {
		location.EndColumn, _ = strconv.Atoi(columnParts[1])
	}
	return location, true
}

func normalizeSymbol(repoPath string, item symbol, includeExternal bool) (symbol, bool) {
	location, ok := normalizeLocation(repoPath, item.Location, includeExternal)
	if !ok {
		return symbol{}, false
	}
	item.Location = location
	return item, true
}

func normalizeLocation(repoPath string, location evidence.Location, includeExternal bool) (evidence.Location, bool) {
	path := location.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoPath, path)
	}
	relative, err := filepath.Rel(repoPath, path)
	if err != nil {
		return evidence.Location{}, false
	}
	isExternal := relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if isExternal && !includeExternal {
		return evidence.Location{}, false
	}
	if isExternal {
		location.Path = filepath.ToSlash(filepath.Clean(path))
	} else {
		location.Path = filepath.ToSlash(filepath.Clean(relative))
	}
	return location, true
}

// normalizeExistingRepoLocation resolves every symlink in an existing path
// before checking containment. filepath.Rel alone is insufficient here: a path
// can be lexically inside repoPath while its final symlink target is outside.
func normalizeExistingRepoLocation(repoPath string, location evidence.Location) (evidence.Location, bool, error) {
	path := filepath.FromSlash(location.Path)
	if !filepath.IsAbs(path) {
		if !filepath.IsLocal(path) {
			return evidence.Location{}, false, nil
		}
		path = filepath.Join(repoPath, path)
	}

	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return evidence.Location{}, false, err
	}
	relative, err := filepath.Rel(repoPath, resolvedPath)
	if err != nil {
		return evidence.Location{}, false, err
	}
	if relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return evidence.Location{}, false, nil
	}

	location.Path = filepath.ToSlash(filepath.Clean(relative))
	return location, true, nil
}

func entityKind(kind string) evidence.EntityKind {
	switch strings.ToLower(kind) {
	case "function":
		return evidence.EntityFunction
	case "method":
		return evidence.EntityMethod
	case "interface":
		return evidence.EntityInterface
	case "field":
		return evidence.EntityField
	case "variable":
		return evidence.EntityVariable
	case "constant":
		return evidence.EntityConstant
	case "struct", "class", "type":
		return evidence.EntityType
	case "package":
		return evidence.EntityPackage
	case "module":
		return evidence.EntityModule
	default:
		return evidence.EntityUnknown
	}
}

func symbolEntity(item symbol) evidence.Entity {
	location := item.Location
	return evidence.Entity{
		ID:       entityID(string(item.Kind), location, item.Name),
		Kind:     item.Kind,
		Name:     item.Name,
		Language: "go",
		Location: &location,
	}
}

func entityID(kind string, location evidence.Location, name string) string {
	return fmt.Sprintf("%s:%s:%d:%d:%s", kind, location.Path, location.Line, location.Column, name)
}

func queryEntityID(query string) string {
	return "query:" + strings.ToLower(strings.Join(strings.Fields(query), "-"))
}

func positionArg(repoPath string, location evidence.Location) string {
	path := location.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoPath, filepath.FromSlash(path))
	}
	return fmt.Sprintf("%s:%d:%d", path, location.Line, location.Column)
}

func cloneLocation(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	copy := *location
	return &copy
}

func samePosition(left, right evidence.Location) bool {
	return left.Path == right.Path && left.Line == right.Line && left.Column == right.Column
}
