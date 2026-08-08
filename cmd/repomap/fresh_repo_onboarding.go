package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

const (
	// The existing semantic-discovery bundle admits at most sixteen
	// supplemental facts. Central anchors are ranked ahead of saved-window
	// fallbacks inside that unchanged core boundary.
	freshRepoOnboardingMaxPlanningFacts = 16
	freshRepoOnboardingMaxAnchorFiles   = 8
	freshRepoOnboardingMaxAnchorFuncs   = 12
	freshRepoOnboardingMaxParsedBytes   = 256 << 10
	freshRepoOnboardingMaxRetainedBytes = 128 << 10
	freshRepoOnboardingMaxFunctionLines = 220
	freshRepoOnboardingMaxBundleBytes   = 2 << 20
)

type freshFlowBundleSource struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Category string `json:"category"`
	Snippet  string `json:"snippet"`
	Weight   int    `json:"weight"`
	Reason   string `json:"reason"`
}

type freshFlowBundle struct {
	SourceSignals []freshFlowBundleSource `json:"source_signals"`
}

type freshCentralAnchor struct {
	Path   string
	Line   int
	Score  int
	Reason string
}

type freshParsedAnchorFile struct {
	Path string
	Data []byte
	File *ast.File
	FSet *token.FileSet
}

type freshRankedFunction struct {
	Function sourcewindowfacts.Function
	Score    int
}

// freshCandidateCentrality is a local presentation/selection diagnostic. It
// never enters the Mechanism contract or semantic hash.
type freshCandidateCentrality struct {
	PurposeAlignment  int `json:"purpose_alignment"`
	ExplanatoryValue  int `json:"explanatory_value"`
	NavigationValue   int `json:"navigation_value"`
	CoreCoverage      int `json:"core_coverage"`
	InputCoverage     int `json:"input_coverage"`
	EffectCoverage    int `json:"effect_coverage"`
	BoundaryCoverage  int `json:"boundary_coverage"`
	EvidenceReadiness int `json:"evidence_readiness"`
	BoundedCost       int `json:"bounded_cost"`
	SecondaryPenalty  int `json:"secondary_penalty"`
	Penalties         int `json:"penalties"`
}

func deriveFreshCandidateCentrality(
	data *report.ReportData,
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
	seeds []freshSourceFunction,
	purposeOverlap int,
) freshCandidateCentrality {
	supported := make(map[string]struct{}, len(candidate.SupportIDs))
	for _, id := range candidate.SupportIDs {
		supported[id] = struct{}{}
	}
	var selected []freshSourceFunction
	groups := make(map[string]struct{})
	for _, source := range sources {
		if _, ok := supported[source.Fact.ID]; !ok {
			continue
		}
		selected = append(selected, source)
		groups[source.Fact.SourceGroup] = struct{}{}
	}
	textParts := []string{candidate.Title, candidate.QuestionAnswered}
	for _, source := range selected {
		textParts = append(
			textParts,
			source.Function.Path,
			source.Function.Symbol,
			source.Fact.Statement,
			strings.Join(source.Fact.Keywords, " "),
		)
	}
	terms := freshTerms(strings.Join(textParts, " "))
	centrality := freshCandidateCentrality{
		PurposeAlignment:  min(purposeOverlap, 8),
		CoreCoverage:      freshCandidateCoreCoverage(data, selected, terms),
		InputCoverage:     freshCandidateInputCoverage(selected, terms),
		EffectCoverage:    freshCandidateEffectCoverage(selected, terms),
		BoundaryCoverage:  freshCandidateBoundaryCoverage(data, selected),
		EvidenceReadiness: min(len(groups)+len(seeds), 8),
		BoundedCost:       len(seeds),
		SecondaryPenalty:  freshSecondaryMechanismPenalty(terms),
	}
	centrality.ExplanatoryValue = min(
		centrality.CoreCoverage+centrality.InputCoverage+centrality.EffectCoverage,
		12,
	)
	architectureAnchors := 0
	estimatedCostPenalty := 0
	if candidate.ProductIntent != nil {
		architectureAnchors = len(candidate.ProductIntent.ArchitectureAreaAnchorIDs)
		switch candidate.ProductIntent.EstimatedCost {
		case semanticdiscovery.OpportunityEstimatedCostHigh:
			estimatedCostPenalty = 2
		case semanticdiscovery.OpportunityEstimatedCostMedium:
			estimatedCostPenalty = 1
		}
	}
	centrality.NavigationValue = min(
		centrality.BoundaryCoverage+min(len(groups), 3)+min(architectureAnchors, 2),
		8,
	)
	centrality.Penalties = centrality.BoundedCost + centrality.SecondaryPenalty + estimatedCostPenalty
	return centrality
}

func freshCandidateBoundaryCoverage(
	data *report.ReportData,
	sources []freshSourceFunction,
) int {
	if data == nil || len(sources) == 0 {
		return 0
	}
	touchedComponents := make(map[string]struct{})
	touchedRoles := make(map[componentmap.Role]struct{})
	for _, component := range data.Components {
		matched := false
		for _, source := range sources {
			if freshComponentTouchesPath(component, source.Function.Path) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		touchedComponents[component.ID] = struct{}{}
		switch component.Role {
		case componentmap.RoleEntry, componentmap.RoleBoundary,
			componentmap.RoleCoordination, componentmap.RoleDomain, componentmap.RoleState:
			touchedRoles[component.Role] = struct{}{}
		}
	}
	coverage := 0
	if _, ok := touchedRoles[componentmap.RoleEntry]; ok {
		coverage++
	}
	if _, ok := touchedRoles[componentmap.RoleBoundary]; ok {
		coverage++
	}
	for _, role := range []componentmap.Role{
		componentmap.RoleCoordination, componentmap.RoleDomain, componentmap.RoleState,
	} {
		if _, ok := touchedRoles[role]; ok {
			coverage++
			break
		}
	}
	// Crossing two validated top-level components is still useful when their
	// coarse model roles happen to be the same.
	coverage = max(coverage, min(len(touchedComponents), 3))
	return min(coverage, 3)
}

func freshComponentTouchesPath(component report.Component, filePath string) bool {
	filePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(filePath)))
	for _, anchor := range component.AnchorGroups {
		anchorPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(anchor.Path)))
		if anchorPath == "." || anchorPath == "" {
			continue
		}
		if anchorPath == filePath {
			return true
		}
		directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(anchorPath)))
		if directory != "." && strings.HasPrefix(filePath, strings.TrimSuffix(directory, "/")+"/") {
			return true
		}
	}
	return false
}

func freshCandidateCoreCoverage(
	data *report.ReportData,
	sources []freshSourceFunction,
	terms map[string]struct{},
) int {
	if data == nil {
		return 0
	}
	coreTerms := make(map[string]struct{})
	coreEvidence := make(map[string]struct{})
	for _, area := range data.HighLevelMap {
		role := string(area.Role)
		if role != "domain" && role != "coordination" && role != "state" {
			continue
		}
		for term := range freshTerms(area.Name + " " + area.WhyItMatters) {
			coreTerms[term] = struct{}{}
		}
		for _, item := range area.Evidence {
			for _, token := range strings.FieldsFunc(item, func(value rune) bool {
				return value == ',' || value == ';' || unicode.IsSpace(value)
			}) {
				clean := strings.Trim(token, "`'\"().")
				if strings.Contains(clean, ".") || strings.Contains(clean, "/") {
					coreEvidence[filepath.ToSlash(clean)] = struct{}{}
				}
			}
		}
	}
	coverage := 0
	for term := range terms {
		if _, ok := coreTerms[term]; ok {
			coverage++
		}
	}
	for _, source := range sources {
		for evidencePath := range coreEvidence {
			if source.Function.Path == evidencePath || strings.HasSuffix(evidencePath, source.Function.Path) {
				coverage += 2
				break
			}
		}
	}
	return min(coverage, 6)
}

func freshCandidateInputCoverage(
	sources []freshSourceFunction,
	terms map[string]struct{},
) int {
	coverage := freshTermCoverage(terms, []string{
		"input", "entry", "request", "command", "receive", "read", "load", "open", "serve", "handle",
	})
	for _, source := range sources {
		if freshFactHasCapability(
			source.Fact,
			semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityDataRead,
		) {
			coverage += 2
		}
	}
	return min(coverage, 5)
}

func freshCandidateEffectCoverage(
	sources []freshSourceFunction,
	terms map[string]struct{},
) int {
	coverage := freshTermCoverage(terms, []string{
		"output", "write", "persist", "save", "store", "upload", "response", "sync", "replicate", "restore", "apply", "render",
	})
	for _, source := range sources {
		if freshFactHasCapability(
			source.Fact,
			semanticdiscovery.CapabilityDataWrite,
			semanticdiscovery.CapabilityDataTransformation,
			semanticdiscovery.CapabilityOutputEffect,
		) {
			coverage += 2
		}
	}
	return min(coverage, 5)
}

func freshTermCoverage(terms map[string]struct{}, vocabulary []string) int {
	coverage := 0
	for _, term := range vocabulary {
		if _, ok := terms[term]; ok {
			coverage++
		}
	}
	return coverage
}

func freshFactHasCapability(
	fact semanticdiscovery.Fact,
	capabilities ...semanticdiscovery.Capability,
) bool {
	wanted := make(map[semanticdiscovery.Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		wanted[capability] = struct{}{}
	}
	for _, capability := range fact.Capabilities {
		if _, ok := wanted[capability]; ok {
			return true
		}
	}
	return false
}

func freshSecondaryMechanismPenalty(terms map[string]struct{}) int {
	return freshTermCoverage(terms, []string{
		"registry", "register", "factory", "constructor", "plugin", "adapter", "helper", "utility", "error", "validate", "config",
	})
}

func (centrality freshCandidateCentrality) localScore() int {
	return centrality.PurposeAlignment + centrality.ExplanatoryValue +
		centrality.NavigationValue + centrality.EvidenceReadiness - centrality.Penalties
}

func compareFreshCandidateCentrality(left, right freshCandidateCentrality) int {
	leftValues := []int{
		left.PurposeAlignment,
		left.ExplanatoryValue,
		left.NavigationValue,
		left.EvidenceReadiness,
		-left.Penalties,
	}
	rightValues := []int{
		right.PurposeAlignment,
		right.ExplanatoryValue,
		right.NavigationValue,
		right.EvidenceReadiness,
		-right.Penalties,
	}
	for index := range leftValues {
		if leftValues[index] > rightValues[index] {
			return 1
		}
		if leftValues[index] < rightValues[index] {
			return -1
		}
	}
	return 0
}

// freshCentralSourceFunctions turns only previously selected saved-flow and
// orientation locations into bounded syntax facts. It does not enumerate the
// repository, resolve calls, or inspect files outside the existing report
// allowlist.
func freshCentralSourceFunctions(
	runDir string,
	repoRoot string,
	data *report.ReportData,
) ([]freshSourceFunction, int, error) {
	anchors, err := freshSavedFlowAnchors(runDir, data)
	if err != nil {
		return nil, 0, err
	}
	files, parsedBytes, err := freshParseAnchorFiles(repoRoot, data, anchors)
	if err != nil {
		return nil, 0, err
	}
	ranked, err := freshFunctionsAtAnchors(files, anchors, data)
	if err != nil {
		return nil, 0, err
	}
	if len(ranked) < 6 {
		ranked, parsedBytes, err = freshAddOrientationFunctions(
			repoRoot,
			data,
			files,
			ranked,
			parsedBytes,
		)
		if err != nil {
			return nil, 0, err
		}
	}
	ranked = selectFreshRankedFunctionsByRole(ranked, freshRepoOnboardingMaxAnchorFuncs)
	result := make([]freshSourceFunction, 0, len(ranked))
	retainedBytes := 0
	for _, item := range ranked {
		functionBytes := len(strings.Join(item.Function.Lines, "\n"))
		if retainedBytes+functionBytes > freshRepoOnboardingMaxRetainedBytes {
			continue
		}
		fact, buildErr := freshWindowFunctionFact(item.Function)
		if buildErr != nil {
			return nil, 0, buildErr
		}
		result = append(result, freshSourceFunction{Function: item.Function, Fact: fact})
		retainedBytes += functionBytes
	}
	return result, parsedBytes, nil
}

func selectFreshRankedFunctionsByRole(
	functions []freshRankedFunction,
	limit int,
) []freshRankedFunction {
	if limit <= 0 || limit > len(functions) {
		limit = len(functions)
	}
	buckets := make(map[artifactrole.Role][]freshRankedFunction)
	for _, function := range functions {
		role := artifactrole.Classify(function.Function.Path, artifactrole.Hints{})
		buckets[role] = append(buckets[role], function)
	}
	roles := []artifactrole.Role{
		artifactrole.RolePrimaryProductionEntry,
		artifactrole.RoleEffectBoundary,
		artifactrole.RolePublicAPI,
		artifactrole.RoleProductionCore,
		artifactrole.RoleCurrentDocumentation,
		artifactrole.RoleExperimental,
		artifactrole.RoleTooling,
		artifactrole.RoleExample,
		artifactrole.RoleHistoricalDocumentation,
		artifactrole.RolePlayground,
		artifactrole.RoleTest,
		artifactrole.RoleFixture,
		artifactrole.RoleGenerated,
	}
	for role := range buckets {
		sort.Slice(buckets[role], func(i, j int) bool {
			if buckets[role][i].Score != buckets[role][j].Score {
				return buckets[role][i].Score > buckets[role][j].Score
			}
			if buckets[role][i].Function.Path != buckets[role][j].Function.Path {
				return artifactrole.LessPath(
					buckets[role][i].Function.Path,
					buckets[role][j].Function.Path,
					role,
				)
			}
			return buckets[role][i].Function.StartLine < buckets[role][j].Function.StartLine
		})
	}
	selected := make([]freshRankedFunction, 0, limit)
	for _, roleGroup := range [][]artifactrole.Role{roles[:4], roles[4:]} {
		for index := 0; len(selected) < limit; index++ {
			added := false
			for _, role := range roleGroup {
				if index >= len(buckets[role]) {
					continue
				}
				selected = append(selected, buckets[role][index])
				added = true
				if len(selected) == limit {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	return selected
}

func freshSavedFlowAnchors(runDir string, data *report.ReportData) ([]freshCentralAnchor, error) {
	if data == nil {
		return nil, nil
	}
	allowed := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		allowed[path] = struct{}{}
	}
	purpose := freshOnboardingPurposeTerms(data)
	flowDir := filepath.Join(runDir, "flows")
	entries, err := os.ReadDir(flowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fresh repository onboarding: read saved flows: %w", err)
	}
	var anchors []freshCentralAnchor
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(flowDir, entry.Name(), "flow_bundle.json")
		info, statErr := os.Lstat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, fmt.Errorf("fresh repository onboarding: inspect %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > freshRepoOnboardingMaxBundleBytes {
			continue
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("fresh repository onboarding: read %s: %w", path, readErr)
		}
		var bundle freshFlowBundle
		if decodeErr := json.Unmarshal(raw, &bundle); decodeErr != nil {
			return nil, fmt.Errorf("fresh repository onboarding: decode %s: %w", path, decodeErr)
		}
		for _, signal := range bundle.SourceSignals {
			if signal.Line <= 0 || filepath.Ext(signal.Path) != ".go" || strings.HasSuffix(signal.Path, "_test.go") {
				continue
			}
			if _, ok := allowed[signal.Path]; !ok {
				continue
			}
			score := signal.Weight + freshCentralSignalCategoryScore(signal.Category)
			score += freshArtifactRoleScore(artifactrole.Classify(signal.Path, artifactrole.Hints{}))
			for term := range freshTerms(signal.Path + " " + signal.Snippet + " " + signal.Reason) {
				if _, matchesPurpose := purpose[term]; matchesPurpose {
					score += 6
				}
			}
			visible := strings.ToLower(signal.Snippet + " " + signal.Reason)
			if strings.Contains(visible, "error") || strings.Contains(visible, "invalid") {
				score -= 18
			}
			anchors = append(anchors, freshCentralAnchor{
				Path: signal.Path, Line: signal.Line, Score: score, Reason: signal.Reason,
			})
		}
	}
	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].Score != anchors[j].Score {
			return anchors[i].Score > anchors[j].Score
		}
		if anchors[i].Path != anchors[j].Path {
			leftRole := artifactrole.Classify(anchors[i].Path, artifactrole.Hints{})
			rightRole := artifactrole.Classify(anchors[j].Path, artifactrole.Hints{})
			if leftRole == rightRole {
				return artifactrole.LessPath(anchors[i].Path, anchors[j].Path, leftRole)
			}
			return anchors[i].Path < anchors[j].Path
		}
		return anchors[i].Line < anchors[j].Line
	})
	return anchors, nil
}

func freshCentralSignalCategoryScore(category string) int {
	switch category {
	case "storage_durability":
		return 35
	case "background_loop":
		return 30
	case "request_handler":
		return 24
	case "admin_maintenance":
		return 12
	case "threshold_limit", "observability":
		return -12
	default:
		return 0
	}
}

func freshParseAnchorFiles(
	repoRoot string,
	data *report.ReportData,
	anchors []freshCentralAnchor,
) (map[string]freshParsedAnchorFile, int, error) {
	paths := freshRoleAwareAnchorPaths(anchors, freshRepoOnboardingMaxAnchorFiles)
	return freshParseSelectedFiles(repoRoot, data, paths, nil, 0)
}

func freshRoleAwareAnchorPaths(anchors []freshCentralAnchor, limit int) []string {
	if limit <= 0 {
		return nil
	}
	best := make(map[string]freshCentralAnchor)
	for _, anchor := range anchors {
		if anchor.Path == "" {
			continue
		}
		previous, exists := best[anchor.Path]
		if !exists || anchor.Score > previous.Score ||
			anchor.Score == previous.Score && anchor.Line < previous.Line {
			best[anchor.Path] = anchor
		}
	}
	buckets := make(map[artifactrole.Role][]freshCentralAnchor)
	for _, anchor := range best {
		role := artifactrole.Classify(anchor.Path, artifactrole.Hints{})
		buckets[role] = append(buckets[role], anchor)
	}
	roles := []artifactrole.Role{
		artifactrole.RolePrimaryProductionEntry,
		artifactrole.RoleEffectBoundary,
		artifactrole.RolePublicAPI,
		artifactrole.RoleProductionCore,
		artifactrole.RoleCurrentDocumentation,
		artifactrole.RoleExperimental,
		artifactrole.RoleTooling,
		artifactrole.RoleExample,
		artifactrole.RoleHistoricalDocumentation,
		artifactrole.RolePlayground,
		artifactrole.RoleTest,
		artifactrole.RoleFixture,
		artifactrole.RoleGenerated,
	}
	for role := range buckets {
		sort.Slice(buckets[role], func(i, j int) bool {
			if buckets[role][i].Score != buckets[role][j].Score {
				return buckets[role][i].Score > buckets[role][j].Score
			}
			return artifactrole.LessPath(buckets[role][i].Path, buckets[role][j].Path, role)
		})
	}
	paths := make([]string, 0, min(limit, len(best)))
	for _, roleGroup := range [][]artifactrole.Role{roles[:4], roles[4:]} {
		for index := 0; len(paths) < limit; index++ {
			added := false
			for _, role := range roleGroup {
				if index >= len(buckets[role]) {
					continue
				}
				paths = append(paths, buckets[role][index].Path)
				added = true
				if len(paths) == limit {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	return paths
}

func freshParseSelectedFiles(
	repoRoot string,
	data *report.ReportData,
	paths []string,
	existing map[string]freshParsedAnchorFile,
	parsedBytes int,
) (map[string]freshParsedAnchorFile, int, error) {
	if existing == nil {
		existing = make(map[string]freshParsedAnchorFile)
	}
	allowed := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		allowed[path] = struct{}{}
	}
	for _, path := range paths {
		if len(existing) >= freshRepoOnboardingMaxAnchorFiles {
			break
		}
		if _, ok := allowed[path]; !ok || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			continue
		}
		if _, duplicate := existing[path]; duplicate {
			continue
		}
		resolved, err := resolveFreshRepoPath(repoRoot, path)
		if err != nil {
			return nil, parsedBytes, err
		}
		info, err := os.Lstat(resolved)
		if err != nil {
			return nil, parsedBytes, fmt.Errorf("fresh repository onboarding: inspect %s: %w", path, err)
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > int64(freshRepoOnboardingMaxParsedBytes-parsedBytes) {
			continue
		}
		raw, err := os.ReadFile(resolved)
		if err != nil {
			return nil, parsedBytes, fmt.Errorf("fresh repository onboarding: read %s: %w", path, err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, raw, 0)
		if err != nil {
			return nil, parsedBytes, fmt.Errorf("fresh repository onboarding: parse %s: %w", path, err)
		}
		existing[path] = freshParsedAnchorFile{Path: path, Data: raw, File: parsed, FSet: fset}
		parsedBytes += len(raw)
	}
	return existing, parsedBytes, nil
}

func freshFunctionsAtAnchors(
	files map[string]freshParsedAnchorFile,
	anchors []freshCentralAnchor,
	data *report.ReportData,
) ([]freshRankedFunction, error) {
	best := make(map[string]freshRankedFunction)
	for _, anchor := range anchors {
		file, ok := files[anchor.Path]
		if !ok {
			continue
		}
		declaration := freshEnclosingDeclaration(file, anchor.Line)
		if declaration == nil {
			continue
		}
		function, err := freshFunctionFromDeclaration(file, declaration)
		if err != nil {
			return nil, err
		}
		if len(freshSubstantiveWindowObservations(function.Observations)) == 0 {
			continue
		}
		key := function.Path + "\x00" + function.Symbol
		score := anchor.Score + freshOnboardingFunctionScore(function, data)
		if previous, exists := best[key]; !exists || score > previous.Score {
			best[key] = freshRankedFunction{Function: function, Score: score}
		}
	}
	result := make([]freshRankedFunction, 0, len(best))
	for _, function := range best {
		result = append(result, function)
	}
	return result, nil
}

func freshAddOrientationFunctions(
	repoRoot string,
	data *report.ReportData,
	files map[string]freshParsedAnchorFile,
	ranked []freshRankedFunction,
	parsedBytes int,
) ([]freshRankedFunction, int, error) {
	paths := make([]string, 0, len(data.FirstFilesToOpen))
	for _, item := range data.FirstFilesToOpen {
		paths = append(paths, item.Path)
	}
	paths = artifactrole.SortPaths(paths)
	var err error
	files, parsedBytes, err = freshParseSelectedFiles(repoRoot, data, paths, files, parsedBytes)
	if err != nil {
		return nil, parsedBytes, err
	}
	known := make(map[string]struct{}, len(ranked))
	for _, item := range ranked {
		known[item.Function.Path+"\x00"+item.Function.Symbol] = struct{}{}
	}
	for _, file := range files {
		for _, declaration := range file.File.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			function, buildErr := freshFunctionFromDeclaration(file, fn)
			if buildErr != nil || len(freshSubstantiveWindowObservations(function.Observations)) == 0 {
				continue
			}
			key := function.Path + "\x00" + function.Symbol
			if _, duplicate := known[key]; duplicate {
				continue
			}
			score := freshOnboardingFunctionScore(function, data)
			if score <= freshWindowFunctionScore(function) {
				continue
			}
			known[key] = struct{}{}
			ranked = append(ranked, freshRankedFunction{Function: function, Score: score})
		}
	}
	return ranked, parsedBytes, nil
}

func freshEnclosingDeclaration(file freshParsedAnchorFile, line int) *ast.FuncDecl {
	var best *ast.FuncDecl
	for _, declaration := range file.File.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := file.FSet.Position(function.Pos()).Line
		end := file.FSet.Position(function.End()).Line
		if line < start || line > end {
			continue
		}
		if best == nil || start > file.FSet.Position(best.Pos()).Line {
			best = function
		}
	}
	return best
}

func freshFunctionFromDeclaration(
	file freshParsedAnchorFile,
	declaration *ast.FuncDecl,
) (sourcewindowfacts.Function, error) {
	start := file.FSet.Position(declaration.Pos()).Line
	end := file.FSet.Position(declaration.End()).Line
	lines := strings.Split(strings.ReplaceAll(string(file.Data), "\r\n", "\n"), "\n")
	if end > len(lines) {
		end = len(lines)
	}
	if end-start+1 > freshRepoOnboardingMaxFunctionLines {
		end = start + freshRepoOnboardingMaxFunctionLines - 1
	}
	if start <= 0 || end < start || end > len(lines) {
		return sourcewindowfacts.Function{}, fmt.Errorf(
			"fresh repository onboarding: invalid function bounds in %s",
			file.Path,
		)
	}
	window, err := sourcewindowfacts.NewWindow(
		goldenStableID("fca", file.Path, fmt.Sprintf("%d", start)),
		file.Path,
		start,
		lines[start-1:end],
	)
	if err != nil {
		return sourcewindowfacts.Function{}, err
	}
	return sourcewindowfacts.ExtractGoFunction(window, freshASTSymbol(declaration))
}

func freshASTSymbol(declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return declaration.Name.Name
	}
	receiver := declaration.Recv.List[0].Type
	for {
		switch value := receiver.(type) {
		case *ast.StarExpr:
			receiver = value.X
		case *ast.IndexExpr:
			receiver = value.X
		case *ast.IndexListExpr:
			receiver = value.X
		default:
			if identifier, ok := receiver.(*ast.Ident); ok {
				return identifier.Name + "." + declaration.Name.Name
			}
			return declaration.Name.Name
		}
	}
}

func freshOnboardingFunctionScore(
	function sourcewindowfacts.Function,
	data *report.ReportData,
) int {
	score := freshWindowFunctionScore(function)
	score += freshArtifactRoleScore(artifactrole.Classify(function.Path, artifactrole.Hints{}))
	name := strings.ToLower(freshHumanLabel(function.Symbol))
	purpose := freshOnboardingPurposeTerms(data)
	for _, action := range []string{
		"run", "start", "open", "serve", "handle", "dispatch", "process",
		"sync", "replicate", "restore", "write", "upload", "persist", "apply", "render",
	} {
		if strings.Contains(name, action) {
			score += 18
		}
	}
	for term := range freshTerms(function.Path + " " + function.Symbol) {
		if _, matchesPurpose := purpose[term]; matchesPurpose {
			score += 7
		}
	}
	for _, helper := range []string{"new", "set", "get", "error", "validate", "default"} {
		if strings.Contains(name, helper) {
			score -= 10
		}
	}
	for _, peripheral := range []string{"usage", "help", "completion"} {
		if strings.Contains(name, peripheral) {
			score -= 40
		}
	}
	return score
}

func freshArtifactRoleScore(role artifactrole.Role) int {
	return artifactrole.SelectionPriority(role) -
		artifactrole.SelectionPriority(artifactrole.RoleProductionCore)
}

func resolveFreshRepoPath(repoRoot string, relativePath string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("fresh repository onboarding: resolve repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("fresh repository onboarding: inspect repository root: %w", err)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("fresh repository onboarding: source path is outside the repository")
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(clean)))
	if err != nil {
		return "", fmt.Errorf("fresh repository onboarding: resolve %s: %w", clean, err)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fresh repository onboarding: source path is outside the repository")
	}
	return candidate, nil
}

func freshOnboardingPurposeTerms(data *report.ReportData) map[string]struct{} {
	if data == nil {
		return nil
	}
	parts := []string{data.DocumentedPurpose, data.ProjectGuess, data.RecommendedFlow}
	for _, area := range data.HighLevelMap {
		parts = append(parts, area.Name, area.WhyItMatters)
	}
	for _, direction := range data.CandidateDirections {
		if direction.Disposition == "rejected" {
			continue
		}
		parts = append(parts, direction.Name, direction.Trigger, direction.WhyInteresting)
	}
	return freshTerms(strings.Join(parts, " "))
}

func mergeFreshSourceFunctions(
	saved []freshSourceFunction,
	central []freshSourceFunction,
	limit int,
) []freshSourceFunction {
	byKey := make(map[string]freshSourceFunction, len(saved)+len(central))
	for _, source := range append(append([]freshSourceFunction(nil), central...), saved...) {
		key := source.Function.Path + "\x00" + source.Function.Symbol
		if previous, exists := byKey[key]; exists &&
			freshWindowFunctionBetter(previous.Function, source.Function) {
			continue
		}
		byKey[key] = source
	}
	result := make([]freshSourceFunction, 0, len(byKey))
	for _, source := range byKey {
		result = append(result, source)
	}
	centralKeys := make(map[string]struct{}, len(central))
	for _, source := range central {
		centralKeys[source.Function.Path+"\x00"+source.Function.Symbol] = struct{}{}
	}
	return selectFreshSourceFunctionsByRole(result, centralKeys, limit)
}

func selectFreshSourceFunctionsByRole(
	sources []freshSourceFunction,
	centralKeys map[string]struct{},
	limit int,
) []freshSourceFunction {
	if limit <= 0 || limit > len(sources) {
		limit = len(sources)
	}
	buckets := make(map[artifactrole.Role][]freshSourceFunction)
	for _, source := range sources {
		role := artifactrole.Classify(source.Function.Path, artifactrole.Hints{})
		buckets[role] = append(buckets[role], source)
	}
	roles := []artifactrole.Role{
		artifactrole.RolePrimaryProductionEntry,
		artifactrole.RoleEffectBoundary,
		artifactrole.RolePublicAPI,
		artifactrole.RoleProductionCore,
		artifactrole.RoleCurrentDocumentation,
		artifactrole.RoleExperimental,
		artifactrole.RoleTooling,
		artifactrole.RoleExample,
		artifactrole.RoleHistoricalDocumentation,
		artifactrole.RolePlayground,
		artifactrole.RoleTest,
		artifactrole.RoleFixture,
		artifactrole.RoleGenerated,
	}
	for role := range buckets {
		sort.Slice(buckets[role], func(i, j int) bool {
			leftKey := buckets[role][i].Function.Path + "\x00" + buckets[role][i].Function.Symbol
			rightKey := buckets[role][j].Function.Path + "\x00" + buckets[role][j].Function.Symbol
			_, leftCentral := centralKeys[leftKey]
			_, rightCentral := centralKeys[rightKey]
			if leftCentral != rightCentral {
				return leftCentral
			}
			leftScore := freshWindowFunctionScore(buckets[role][i].Function)
			rightScore := freshWindowFunctionScore(buckets[role][j].Function)
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			if buckets[role][i].Function.Path != buckets[role][j].Function.Path {
				return artifactrole.LessPath(
					buckets[role][i].Function.Path,
					buckets[role][j].Function.Path,
					role,
				)
			}
			return leftKey < rightKey
		})
	}
	selected := make([]freshSourceFunction, 0, limit)
	for _, roleGroup := range [][]artifactrole.Role{roles[:4], roles[4:]} {
		for index := 0; len(selected) < limit; index++ {
			added := false
			for _, role := range roleGroup {
				if index >= len(buckets[role]) {
					continue
				}
				selected = append(selected, buckets[role][index])
				added = true
				if len(selected) == limit {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	return selected
}

func preserveLegacyFreshMechanism(runDir string) error {
	mechanismPath := filepath.Join(runDir, semanticdiscovery.MechanismFile)
	info, err := os.Lstat(mechanismPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("fresh repository onboarding: inspect legacy mechanism: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > freshRepoOnboardingMaxBundleBytes {
		return fmt.Errorf("fresh repository onboarding: legacy mechanism is not a bounded regular file")
	}
	mechanismRaw, err := os.ReadFile(mechanismPath)
	if err != nil {
		return fmt.Errorf("fresh repository onboarding: read legacy mechanism: %w", err)
	}
	mechanism, err := semanticdiscovery.DecodeMechanism(mechanismRaw)
	if err != nil {
		return fmt.Errorf("fresh repository onboarding: decode legacy mechanism: %w", err)
	}
	probeRaw, err := readFreshMechanismCompanion(
		filepath.Join(runDir, report.GoldenMechanismProbeFile),
	)
	if err != nil {
		return err
	}
	factsRaw, err := readFreshMechanismCompanion(
		filepath.Join(runDir, report.GoldenMechanismFactsFile),
	)
	if err != nil {
		return err
	}
	return archiveFreshMechanismEntry(
		runDir,
		mechanism.Payload.Candidate.ID,
		probeRaw,
		factsRaw,
		mechanismRaw,
	)
}

func readFreshMechanismCompanion(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("fresh repository onboarding: inspect mechanism companion: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > freshRepoOnboardingMaxBundleBytes {
		return nil, fmt.Errorf("fresh repository onboarding: mechanism companion is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fresh repository onboarding: read mechanism companion: %w", err)
	}
	return raw, nil
}

func archiveFreshMechanismEntry(
	runDir string,
	candidateID string,
	probeRaw []byte,
	factsRaw []byte,
	mechanismRaw []byte,
) (returnErr error) {
	mechanism, err := semanticdiscovery.DecodeMechanism(mechanismRaw)
	if err != nil {
		return fmt.Errorf("fresh repository onboarding: archive mechanism: %w", err)
	}
	if mechanism.Payload.Candidate.ID != candidateID {
		return fmt.Errorf("fresh repository onboarding: archive candidate identity mismatch")
	}
	entryDir := filepath.Join(report.MechanismV1CollectionPath(runDir), candidateID)
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		return fmt.Errorf("fresh repository onboarding: create mechanism collection entry: %w", err)
	}
	paths := []string{
		filepath.Join(entryDir, report.GoldenMechanismProbeFile),
		filepath.Join(entryDir, report.GoldenMechanismFactsFile),
		filepath.Join(entryDir, semanticdiscovery.MechanismFile),
	}
	backups, err := backupGoldenFiles(paths)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := restoreGoldenFiles(backups); err != nil {
			returnErr = errors.Join(
				returnErr,
				fmt.Errorf("fresh repository onboarding: rollback mechanism collection: %w", err),
			)
		}
	}()
	for index, raw := range [][]byte{probeRaw, factsRaw, mechanismRaw} {
		if len(raw) == 0 || len(raw) > freshRepoOnboardingMaxBundleBytes {
			return fmt.Errorf("fresh repository onboarding: archive payload is outside bounds")
		}
		if err := writeAtomicFile(paths[index], raw, 0o600); err != nil {
			return err
		}
	}
	committed = true
	return nil
}

func freshPublishedOnboardingRole(
	runDir string,
	artifactID string,
) (report.OnboardingRole, error) {
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return report.OnboardingRoleUnknown, err
	}
	for _, mechanism := range data.UserMechanisms {
		if mechanism.ArtifactID != artifactID {
			continue
		}
		return report.DeriveMechanismOnboardingRole(data, mechanism), nil
	}
	return report.OnboardingRoleUnknown, fmt.Errorf(
		"fresh repository onboarding: published artifact has no user mechanism projection",
	)
}
