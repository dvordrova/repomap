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
	"os/exec"
	"path/filepath"
	"runtime"
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

var _ analysis.Provider = (*Analyzer)(nil)

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
		rootEntity := symbolEntity(hierarchyRoot)
		graph.AddEntity(rootEntity)
		for _, edge := range edges {
			target, targetOK := normalizeSymbol(repoPath, edge.Symbol, a.opts.IncludeExternal)
			if !targetOK {
				continue
			}
			if samePosition(target.Location, hierarchyRoot.Location) {
				continue
			}
			callsite, callsiteOK := normalizeLocation(repoPath, edge.Callsite, a.opts.IncludeExternal)
			if !callsiteOK {
				continue
			}
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

	if a.opts.IncludeImplementations {
		a.addImplementations(ctx, repoPath, version, localSymbols, &graph)
	}

	graph.Sort()
	if err := graph.Validate(); err != nil {
		return evidence.Graph{}, fmt.Errorf("gopls: invalid evidence graph: %w", err)
	}
	return graph, nil
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

func entityKind(kind string) evidence.EntityKind {
	switch strings.ToLower(kind) {
	case "function":
		return evidence.EntityFunction
	case "method":
		return evidence.EntityMethod
	case "interface":
		return evidence.EntityInterface
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
