package goldenmechanism

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
)

type parsedFile struct {
	id          string
	path        string
	packageName string
	data        []byte
	file        *ast.File
	sha256      string
}

type functionDecl struct {
	id          string
	symbol      string
	receiver    string
	receiverVar string
	packageName string
	file        *parsedFile
	decl        *ast.FuncDecl
	returns     []typeInfo
	params      map[string]typeInfo
	errorResult []int
}

type typeInfo struct {
	base           string
	text           string
	responseWriter bool
}

type frontierItem struct {
	decl           *functionDecl
	depth          int
	seed           bool
	originFacts    []string
	originEvidence []string
	reachedFrom    []string
	plannedFrom    []string
}

// Probe runs one bounded, repository-local Go syntax probe. The only files it
// parses are exact seed paths. Invalid plans, paths, or symbols fail closed;
// exhausted file/source/function/depth/time budgets return a validated partial
// result.
func Probe(parent context.Context, repoPath string, requested Plan) (Result, error) {
	started := time.Now()
	plan, err := requested.normalized()
	if err != nil {
		return Result{}, err
	}
	root, err := resolveRepoRoot(repoPath)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(parent, plan.Limits.Timeout)
	defer cancel()

	result := Result{
		Version:      Version,
		MechanismID:  plan.MechanismID,
		Seeds:        make([]SeedResolution, len(plan.Seeds)),
		Files:        []File{},
		Functions:    []Function{},
		Observations: []Observation{},
		Budget:       BudgetStats{SeedCount: len(plan.Seeds)},
	}
	for index, seed := range plan.Seeds {
		result.Seeds[index] = SeedResolution{Seed: seed}
	}

	fset := token.NewFileSet()
	files, skipped, err := parseSeedFiles(ctx, root, plan, fset, &result)
	if err != nil {
		return Result{}, err
	}
	index, byPath, err := indexFunctions(plan.MechanismID, files)
	if err != nil {
		return Result{}, err
	}

	seedItems := make(map[string]*frontierItem)
	for seedIndex, seed := range plan.Seeds {
		if reason, wasSkipped := skipped[seed.Path]; wasSkipped {
			result.Seeds[seedIndex].Status = reason
			continue
		}
		decl := byPath[seed.Path+"\x00"+seed.Symbol]
		if decl == nil {
			return Result{}, fmt.Errorf("golden mechanism: seed symbol %q is not declared in %s", seed.Symbol, seed.Path)
		}
		item := seedItems[decl.id]
		if item == nil {
			item = &frontierItem{decl: decl, depth: seed.Depth, seed: true}
			seedItems[decl.id] = item
		} else if item.depth != seed.Depth {
			return Result{}, fmt.Errorf(
				"golden mechanism: duplicate symbol %q has conflicting planner depths",
				seed.Symbol,
			)
		}
		item.originFacts = append(item.originFacts, seed.OriginFactID)
		item.originEvidence = append(item.originEvidence, seed.OriginEvidenceID)
		item.plannedFrom = append(item.plannedFrom, seed.ReachedFromEvidenceID)
	}

	frontier := make([]frontierItem, 0, len(seedItems))
	for _, item := range seedItems {
		item.originFacts = sortedUnique(item.originFacts)
		item.originEvidence = sortedUnique(item.originEvidence)
		item.reachedFrom = sortedUnique(item.reachedFrom)
		item.plannedFrom = sortedUnique(item.plannedFrom)
		frontier = append(frontier, *item)
	}
	sort.Slice(frontier, func(i, j int) bool {
		left, right := frontier[i].decl, frontier[j].decl
		if left.file.path != right.file.path {
			return left.file.path < right.file.path
		}
		return left.decl.Pos() < right.decl.Pos()
	})

	allowed := make(map[string]struct{}, len(plan.ExpansionAllowlist))
	for _, symbol := range plan.ExpansionAllowlist {
		allowed[symbol] = struct{}{}
	}
	scheduled := make(map[string]int, len(frontier))
	for _, item := range frontier {
		scheduled[item.decl.id] = item.depth
	}

	for len(frontier) > 0 {
		if stopForContext(ctx, &result) {
			break
		}
		if len(result.Functions) >= plan.Limits.MaxFunctions {
			markPartial(&result, StopFunctionLimit)
			break
		}
		item := frontier[0]
		frontier = frontier[1:]
		function, window, err := buildFunction(root, fset, plan, item, result.Budget.IncludedSourceBytes)
		if err != nil {
			if err == errSourceBudget {
				markPartial(&result, StopSourceByteLimit)
				break
			}
			return Result{}, err
		}
		result.Budget.IncludedSourceBytes += window.includedBytes
		if function.SourceTruncated {
			switch function.SourceStopReason {
			case string(sourcecard.StopLineLimit):
				markPartial(&result, StopFunctionLineLimit)
			case string(sourcecard.StopByteLimit):
				markPartial(&result, StopFunctionByteLimit)
			}
		}

		observations, callees := analyzeFunction(ctx, fset, index, item.decl, function, window, allowed)
		result.Functions = append(result.Functions, function)
		result.Observations = append(result.Observations, observations...)
		if ctx.Err() != nil {
			markPartial(&result, StopTimeout)
		}
		if item.seed {
			for seedIndex := range result.Seeds {
				resolution := &result.Seeds[seedIndex]
				if resolution.Status == "" && resolution.Seed.Path == item.decl.file.path && resolution.Seed.Symbol == item.decl.symbol {
					resolution.Status = SeedResolved
					resolution.FunctionID = item.decl.id
					result.Budget.ResolvedSeedCount++
				}
			}
		}
		if item.depth > result.Budget.MaxDepthReached {
			result.Budget.MaxDepthReached = item.depth
		}

		for _, callee := range callees {
			if _, exists := scheduled[callee.decl.id]; exists {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[callee.decl.symbol]; !ok {
					continue
				}
			}
			if item.depth >= plan.Limits.MaxDepth {
				markPartial(&result, StopDepthLimit)
				continue
			}
			next := frontierItem{
				decl:        callee.decl,
				depth:       item.depth + 1,
				reachedFrom: []string{callee.observationID},
			}
			scheduled[callee.decl.id] = next.depth
			frontier = append(frontier, next)
		}
		sort.SliceStable(frontier, func(i, j int) bool {
			if frontier[i].depth != frontier[j].depth {
				return frontier[i].depth < frontier[j].depth
			}
			if frontier[i].decl.file.path != frontier[j].decl.file.path {
				return frontier[i].decl.file.path < frontier[j].decl.file.path
			}
			return frontier[i].decl.decl.Pos() < frontier[j].decl.decl.Pos()
		})
	}
	if err := finalizePendingSeeds(&result); err != nil {
		return Result{}, err
	}

	result.Budget.FunctionsIncluded = len(result.Functions)
	result.Budget.Observations = len(result.Observations)
	result.Budget.ElapsedMillis = time.Since(started).Milliseconds()
	sortResult(&result)
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func finalizePendingSeeds(result *Result) error {
	for index := range result.Seeds {
		resolution := &result.Seeds[index]
		if resolution.Status != "" {
			continue
		}
		switch result.StopReason {
		case StopTimeout:
			resolution.Status = SeedSkippedTimeout
		case StopSourceByteLimit, StopFunctionByteLimit:
			resolution.Status = SeedSkippedByteLimit
		case StopFileLimit:
			resolution.Status = SeedSkippedFileLimit
		case StopFunctionLimit:
			resolution.Status = SeedSkippedFunctionLimit
		default:
			return fmt.Errorf("golden mechanism: seed %q was not included without a matching budget stop", resolution.Seed.Symbol)
		}
	}
	return nil
}

func resolveRepoRoot(repoPath string) (string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return "", fmt.Errorf("golden mechanism: repository path is required")
	}
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: resolve repository path: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: resolve repository symlinks: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: stat repository: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("golden mechanism: repository path is not a directory")
	}
	return root, nil
}

func parseSeedFiles(
	ctx context.Context,
	root string,
	plan Plan,
	fset *token.FileSet,
	result *Result,
) (map[string]*parsedFile, map[string]SeedStatus, error) {
	unique := make(map[string]struct{}, len(plan.Seeds))
	paths := make([]string, 0, len(plan.Seeds))
	for _, seed := range plan.Seeds {
		if _, exists := unique[seed.Path]; exists {
			continue
		}
		unique[seed.Path] = struct{}{}
		paths = append(paths, seed.Path)
	}
	sort.Strings(paths)
	resolvedPaths := make(map[string]string, len(paths))
	fileSizes := make(map[string]int64, len(paths))
	// Containment, existence, and regular-file checks are plan validation, not
	// analysis work. Perform them for every path before a budget may skip one.
	for _, relativePath := range paths {
		absolutePath, err := resolveSeedPath(root, relativePath)
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Stat(absolutePath)
		if err != nil {
			return nil, nil, fmt.Errorf("golden mechanism: stat %s: %w", relativePath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("golden mechanism: seed path %s is not a regular file", relativePath)
		}
		resolvedPaths[relativePath] = absolutePath
		fileSizes[relativePath] = info.Size()
	}

	files := make(map[string]*parsedFile, len(paths))
	skipped := make(map[string]SeedStatus)
	for pathIndex, relativePath := range paths {
		if ctx.Err() != nil {
			markPartial(result, StopTimeout)
			for _, remaining := range paths[pathIndex:] {
				skipped[remaining] = SeedSkippedTimeout
			}
			break
		}
		if len(files) >= plan.Limits.MaxFiles {
			markPartial(result, StopFileLimit)
			for _, remaining := range paths[pathIndex:] {
				skipped[remaining] = SeedSkippedFileLimit
			}
			break
		}
		absolutePath := resolvedPaths[relativePath]
		if fileSizes[relativePath] > int64(plan.Limits.MaxParsedSourceBytes-result.Budget.ParsedSourceBytes) {
			markPartial(result, StopSourceByteLimit)
			for _, remaining := range paths[pathIndex:] {
				skipped[remaining] = SeedSkippedByteLimit
			}
			break
		}
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, nil, fmt.Errorf("golden mechanism: read %s: %w", relativePath, err)
		}
		parsed, err := parser.ParseFile(fset, relativePath, data, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("golden mechanism: parse %s: %w", relativePath, err)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		file := &parsedFile{
			id:          stableID("gm-file", plan.MechanismID, relativePath, digest),
			path:        relativePath,
			packageName: parsed.Name.Name,
			data:        data,
			file:        parsed,
			sha256:      digest,
		}
		files[relativePath] = file
		result.Files = append(result.Files, File{
			ID: file.id, Path: relativePath, SHA256: digest, Bytes: len(data), Package: parsed.Name.Name,
		})
		result.Budget.ParsedSourceBytes += len(data)
	}
	result.Budget.FilesParsed = len(result.Files)
	return files, skipped, nil
}

func resolveSeedPath(root, relativePath string) (string, error) {
	if !validGoPath(relativePath) {
		return "", fmt.Errorf("golden mechanism: invalid repository-relative Go path %q", relativePath)
	}
	candidate := filepath.Join(root, filepath.FromSlash(relativePath))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: resolve seed path %s: %w", relativePath, err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: verify seed path %s: %w", relativePath, err)
	}
	if !filepath.IsLocal(relative) || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("golden mechanism: seed path %s resolves outside repository", relativePath)
	}
	return resolved, nil
}

func indexFunctions(
	mechanismID string,
	files map[string]*parsedFile,
) (map[string]*functionDecl, map[string]*functionDecl, error) {
	index := make(map[string]*functionDecl)
	byPath := make(map[string]*functionDecl)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file := files[path]
		if file == nil || file.file == nil {
			return nil, nil, fmt.Errorf("golden mechanism: parsed file %s is unavailable", path)
		}
		for _, declaration := range file.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			receiver, receiverVar := receiverIdentity(function)
			symbol := function.Name.Name
			if receiver != "" {
				symbol = receiver + "." + function.Name.Name
			}
			decl := &functionDecl{
				id:          stableID("gm-fn", mechanismID, file.path, symbol),
				symbol:      symbol,
				receiver:    receiver,
				receiverVar: receiverVar,
				packageName: file.packageName,
				file:        file,
				decl:        function,
				returns:     resultTypes(function.Type.Results),
				params:      parameterTypes(function.Type.Params),
				errorResult: errorResultIndexes(function.Type.Results),
			}
			key := file.packageName + "\x00" + symbol
			if prior := index[key]; prior != nil {
				return nil, nil, fmt.Errorf("golden mechanism: ambiguous local declaration %s in %s and %s", symbol, prior.file.path, file.path)
			}
			index[key] = decl
			byPath[file.path+"\x00"+symbol] = decl
		}
	}
	return index, byPath, nil
}

var errSourceBudget = fmt.Errorf("source budget exhausted")

type functionWindow struct {
	lastLine      int
	lastColumn    int
	includedBytes int
}

func buildFunction(
	repoRoot string,
	fset *token.FileSet,
	plan Plan,
	item frontierItem,
	includedBytes int,
) (Function, functionWindow, error) {
	remaining := plan.Limits.MaxSourceBytes - includedBytes
	if remaining <= 0 {
		return Function{}, functionWindow{}, errSourceBudget
	}
	maxBytes := min(plan.Limits.MaxFunctionBytes, remaining)
	position := fset.Position(item.decl.decl.Pos())
	end := fset.Position(item.decl.decl.End())
	kind := evidence.EntityFunction
	if item.decl.receiver != "" {
		kind = evidence.EntityMethod
	}
	targetEvidenceID := item.decl.id
	if len(item.originEvidence) > 0 {
		targetEvidenceID = item.originEvidence[0]
	}
	card, err := sourcecard.Read(sourcecard.Request{
		RepoPath:         repoRoot,
		TargetEvidenceID: targetEvidenceID,
		Target: evidence.Entity{
			ID: item.decl.id, Kind: kind, Name: item.decl.symbol, Language: "go",
			Scope:    evidence.SourceScopeRepository,
			Location: &evidence.Location{Path: item.decl.file.path, Line: position.Line, Column: position.Column},
		},
	}, sourcecard.Limits{
		MaxFileBytes:   int64(len(item.decl.file.data)),
		MaxWindowLines: plan.Limits.MaxFunctionLines,
		MaxWindowBytes: maxBytes,
		MaxLineBytes:   maxBytes,
	})
	if err != nil {
		if strings.Contains(err.Error(), "source window is empty") {
			return Function{}, functionWindow{}, errSourceBudget
		}
		return Function{}, functionWindow{}, fmt.Errorf("golden mechanism: read bounded source for %s: %w", item.decl.symbol, err)
	}

	function := Function{
		ID: item.decl.id, Symbol: item.decl.symbol, Path: item.decl.file.path,
		Location: evidence.Location{
			Path: item.decl.file.path, Line: position.Line, Column: position.Column,
			EndLine: end.Line, EndColumn: end.Column,
		},
		Seed: item.seed, Depth: item.depth,
		OriginFactIDs: sortedUnique(item.originFacts), OriginEvidenceIDs: sortedUnique(item.originEvidence),
		ReachedFromIDs:         sortedUnique(item.reachedFrom),
		PlannedFromEvidenceIDs: sortedUnique(item.plannedFrom), Source: []SourceLine{},
		SourceStopReason: "function_end",
	}
	window := functionWindow{}
	for _, line := range card.Lines {
		if line.Line > end.Line {
			break
		}
		lineID := stableID("gm-src", plan.MechanismID, item.decl.file.path, fmt.Sprintf("%d", line.Line), line.Text)
		function.Source = append(function.Source, SourceLine{
			ID: lineID,
			Location: evidence.Location{
				Path: item.decl.file.path, Line: line.Line, Column: 1,
				EndLine: line.Line, EndColumn: len(line.Text) + 1,
			},
			Text: line.Text, Truncated: line.Truncated,
		})
		window.includedBytes += len(line.Text)
		if len(function.Source) > 1 {
			window.includedBytes++
		}
	}
	if len(function.Source) == 0 {
		return Function{}, functionWindow{}, errSourceBudget
	}
	last := function.Source[len(function.Source)-1]
	window.lastLine = last.Location.Line
	window.lastColumn = last.Location.EndColumn
	function.SourceTruncated = card.Window.Truncated || window.lastLine < end.Line || last.Truncated
	if function.SourceTruncated {
		if last.Truncated {
			function.SourceStopReason = string(sourcecard.StopByteLimit)
		} else {
			function.SourceStopReason = string(card.Window.StopReason)
		}
	}
	return function, window, nil
}

func stopForContext(ctx context.Context, result *Result) bool {
	if ctx.Err() == nil {
		return false
	}
	markPartial(result, StopTimeout)
	return true
}

func markPartial(result *Result, reason StopReason) {
	result.Partial = true
	if result.StopReason == "" {
		result.StopReason = reason
	}
}

func sortResult(result *Result) {
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	sort.Slice(result.Functions, func(i, j int) bool {
		if result.Functions[i].Depth != result.Functions[j].Depth {
			return result.Functions[i].Depth < result.Functions[j].Depth
		}
		if result.Functions[i].Path != result.Functions[j].Path {
			return result.Functions[i].Path < result.Functions[j].Path
		}
		return result.Functions[i].Location.Line < result.Functions[j].Location.Line
	})
	sort.Slice(result.Observations, func(i, j int) bool {
		left, right := result.Observations[i], result.Observations[j]
		if left.Evidence[0].Location.Path != right.Evidence[0].Location.Path {
			return left.Evidence[0].Location.Path < right.Evidence[0].Location.Path
		}
		if left.Evidence[0].Location.Line != right.Evidence[0].Location.Line {
			return left.Evidence[0].Location.Line < right.Evidence[0].Location.Line
		}
		if left.Evidence[0].Location.Column != right.Evidence[0].Location.Column {
			return left.Evidence[0].Location.Column < right.Evidence[0].Location.Column
		}
		return left.ID < right.ID
	})
}
