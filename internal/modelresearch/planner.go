package modelresearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	maxSourceReadBytes  = 48 << 10
	maxSourceLineBytes  = 512
	minWindowLines      = 40
	maxWindowLines      = 80
	requestFramingBytes = 16 << 10
)

const noCodeBearingBoundedWindow = "no_code_bearing_bounded_window"

type PlanningInput struct {
	RepoPath             string
	Questions            []ProposedQuestion
	Candidates           []FileCandidate
	InitialProviderPaths []string
	Universe             LocalRepositoryUniverse
	Policy               Policy
	Usage                Usage
	PreviousEvidenceIDs  []string
}

type PlannedRound struct {
	Question ProposedQuestion
	Bundle   EvidenceBundle
	Scope    FocusedInvestigationScope
	Gate     GateDecision
	Score    int
}

type PlanResult struct {
	Selected []PlannedRound
	Skipped  []PlannedRound
}

// PlanTargetedRounds applies a deterministic local value-of-information policy
// and returns at most two bounded questions. Model ordering is only a hint;
// exact retrievable evidence and user-facing impact determine selection.
func PlanTargetedRounds(ctx context.Context, input PlanningInput) (PlanResult, error) {
	if ctx == nil {
		return PlanResult{}, fmt.Errorf("model research: context is required")
	}
	if err := ctx.Err(); err != nil {
		return PlanResult{}, err
	}
	if err := input.Policy.Validate(); err != nil {
		return PlanResult{}, err
	}
	if input.Policy.MaxTargetedRounds == 0 || len(input.Questions) == 0 {
		return PlanResult{}, nil
	}
	reader, err := reporead.New(input.RepoPath)
	if err != nil {
		return PlanResult{}, err
	}
	defer reader.Close()

	previous := makeStringSet(input.PreviousEvidenceIDs)
	initialPaths := makeStringSet(input.InitialProviderPaths)
	authorized := makeStringSet(input.Universe.AuthorizedPaths)
	candidates := make(map[string]FileCandidate, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.ID != "" && candidate.Path != "" {
			candidates[candidate.ID] = candidate
		}
	}

	plans := make([]PlannedRound, 0, len(input.Questions))
	seenQuestions := make(map[string]struct{}, len(input.Questions))
	for index, question := range input.Questions {
		if err := ctx.Err(); err != nil {
			return PlanResult{}, err
		}
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" {
			question.ID = stableID("question", question.Question)
		}
		identity := strings.ToLower(question.Question)
		if question.Question == "" {
			continue
		}
		if _, duplicate := seenQuestions[identity]; duplicate {
			continue
		}
		seenQuestions[identity] = struct{}{}

		plan := PlannedRound{Question: question, Score: questionImpactScore(question)}
		plan.Gate = GateDecision{
			Reason:                "planned",
			UnresolvedHighValue:   plan.Score > 0,
			RuntimeOnly:           isRuntimeOnlyQuestion(question.Question),
			RemainingCalls:        input.Policy.MaxSemanticCalls - input.Usage.SemanticCalls,
			RemainingRequestBytes: input.Policy.MaxTotalRequestBytes - input.Usage.RequestBytes,
		}
		if plan.Gate.RuntimeOnly {
			plan.Gate.Reason = "runtime_only_frontier"
			plans = append(plans, plan)
			continue
		}

		bundle, scope, unknownCandidates, err := assembleEvidenceBundle(
			ctx, reader, index+1, question, candidates, authorized, initialPaths,
			input.Universe.CommandTraces, input.Policy.Targeted,
		)
		if err != nil {
			return PlanResult{}, err
		}
		plan.Bundle = bundle
		plan.Scope = scope
		if unknownCandidates > 0 && len(bundle.Evidence) == 0 {
			plan.Gate.Reason = "unknown_candidate_ids"
			plans = append(plans, plan)
			continue
		}
		if !hasCodeBearingWindow(bundle.Evidence) {
			plan.Gate.Reason = noCodeBearingBoundedWindow
			plans = append(plans, plan)
			continue
		}
		newEvidence := 0
		for _, item := range bundle.Evidence {
			if _, alreadyUsed := previous[item.ID]; !alreadyUsed {
				newEvidence++
			}
		}
		plan.Gate.NewExactEvidence = newEvidence
		if newEvidence == 0 {
			plan.Gate.Reason = "no_new_exact_evidence"
			plans = append(plans, plan)
			continue
		}
		if len(bundle.Evidence) == 0 || len(scope.LocallyInspected) == 0 {
			plan.Gate.Reason = "no_bounded_local_evidence"
			plans = append(plans, plan)
			continue
		}
		plan.Gate.Selected = true
		plan.Gate.Reason = "new_exact_evidence_and_high_value_frontier"
		plan.Gate.Signals = localGateSignals(bundle.Evidence, initialPaths)
		plan.Score += newEvidence
		plans = append(plans, plan)
	}

	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].Gate.Selected != plans[j].Gate.Selected {
			return plans[i].Gate.Selected
		}
		if plans[i].Score != plans[j].Score {
			return plans[i].Score > plans[j].Score
		}
		return plans[i].Question.ID < plans[j].Question.ID
	})
	result := PlanResult{Selected: make([]PlannedRound, 0, input.Policy.MaxTargetedRounds)}
	for _, plan := range plans {
		if !plan.Gate.Selected {
			result.Skipped = append(result.Skipped, plan)
			continue
		}
		if len(result.Selected) == input.Policy.MaxTargetedRounds {
			plan.Gate.Selected = false
			plan.Gate.Reason = "targeted_round_limit"
			result.Skipped = append(result.Skipped, plan)
			continue
		}
		newEvidence := 0
		for _, item := range plan.Bundle.Evidence {
			if _, alreadySelected := previous[item.ID]; !alreadySelected {
				newEvidence++
			}
		}
		if newEvidence == 0 {
			plan.Gate.Selected = false
			plan.Gate.NewExactEvidence = 0
			plan.Gate.Reason = "no_new_exact_evidence"
			result.Skipped = append(result.Skipped, plan)
			continue
		}
		result.Selected = append(result.Selected, plan)
		for _, item := range plan.Bundle.Evidence {
			previous[item.ID] = struct{}{}
		}
	}
	return result, nil
}

func assembleEvidenceBundle(
	ctx context.Context,
	reader *reporead.Reader,
	roundNumber int,
	question ProposedQuestion,
	candidates map[string]FileCandidate,
	authorized map[string]struct{},
	initialPaths map[string]struct{},
	traces []gofacts.CommandTrace,
	budget StageBudget,
) (EvidenceBundle, FocusedInvestigationScope, int, error) {
	roundID := fmt.Sprintf("research-%d-%s", roundNumber, strings.TrimPrefix(stableID("round", question.Question), "round-"))
	bundle := EvidenceBundle{
		Version: ContractVersion, PolicyVersion: PolicyVersion, RoundID: roundID,
		Purpose: question.Purpose, Question: question.Question,
	}
	selectedPaths := make(map[string]int)
	focusLines := make(map[string]int)
	unknownCandidates := 0
	for _, candidateID := range sortedUnique(question.CandidateIDs) {
		candidate, ok := candidates[candidateID]
		if !ok {
			unknownCandidates++
			continue
		}
		if _, allowed := authorized[candidate.Path]; !allowed {
			unknownCandidates++
			continue
		}
		selectedPaths[candidate.Path] = 0
		focusLines[candidate.Path] = candidateFocusLine(candidate)
		bundle.Evidence = append(bundle.Evidence, EvidenceItem{
			ID: stableID("evidence", "file\x00"+candidate.Path), Kind: EvidenceFileSummary,
			Statement: "provider file summary selected for focused local expansion",
			Location:  &evidence.Location{Path: candidate.Path}, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{Provider: "local_research_planner", Version: PolicyVersion, Operation: "select_candidate_file"}},
			Visibility: []EvidenceVisibility{VisibilityProviderInitial},
		})
	}
	if len(selectedPaths) == 0 {
		for _, candidate := range bestQuestionCandidates(question, candidates, authorized, 3) {
			selectedPaths[candidate.Path] = 0
			focusLines[candidate.Path] = candidateFocusLine(candidate)
		}
	}

	for _, trace := range matchingCommandTraces(question.Question, traces) {
		if err := ctx.Err(); err != nil {
			return EvidenceBundle{}, FocusedInvestigationScope{}, 0, err
		}
		for _, step := range trace.Steps {
			addTraceLocation(selectedPaths, focusLines, authorized, step.TargetLocation)
			if step.CallsiteLocation != nil {
				addTraceLocation(selectedPaths, focusLines, authorized, *step.CallsiteLocation)
			}
			bundle.Evidence = append(bundle.Evidence, exactEvidenceItem(
				EvidenceDeclaration, step.Symbol, step.Relation, step.TargetLocation,
				"exact command trace declaration", initialPaths,
			))
		}
		for _, call := range trace.HandlerCalls {
			location := evidence.Location{Path: call.Path, Line: call.Line}
			addTraceLocation(selectedPaths, focusLines, authorized, location)
			bundle.Evidence = append(bundle.Evidence, exactEvidenceItem(
				EvidenceCallsite, call.Symbol, call.Relation, location,
				"exact command handler callsite", initialPaths,
			))
			if call.Resolved && call.TargetPath != "" {
				target := evidence.Location{Path: call.TargetPath, Line: call.TargetLine}
				addTraceLocation(selectedPaths, focusLines, authorized, target)
				bundle.Evidence = append(bundle.Evidence, exactEvidenceItem(
					EvidenceTransition, call.Symbol, call.Relation, target,
					"locally resolved command call target", initialPaths,
				))
			}
		}
	}

	paths := rankedPaths(selectedPaths)
	if len(paths) > budget.MaxFiles {
		paths = paths[:budget.MaxFiles]
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return EvidenceBundle{}, FocusedInvestigationScope{}, 0, err
		}
		line := focusLines[path]
		window, ok := readSourceWindow(reader, path, line)
		if !ok {
			continue
		}
		location := evidence.Location{Path: path, Line: window.StartLine}
		item := EvidenceItem{
			ID:   stableID("evidence", strings.Join([]string{"source", path, strconv.Itoa(window.StartLine), strconv.Itoa(window.EndLine)}, "\x00")),
			Kind: EvidenceSource, Statement: "bounded source window selected locally for the research question",
			Location: &location, Certainty: evidence.CertaintyStatic, Window: &window,
			Provenance: []evidence.Provenance{{Provider: "reporead", Version: PolicyVersion, Operation: "read_bounded_source_window", Location: &location}},
			Visibility: []EvidenceVisibility{VisibilityLocalAfter},
		}
		bundle.Evidence = append(bundle.Evidence, item)
	}
	localEvidence := deduplicateEvidence(bundle.Evidence)
	providerEvidence := prioritizedProviderEvidence(localEvidence)
	if len(providerEvidence) > budget.MaxEvidenceItems {
		providerEvidence = providerEvidence[:budget.MaxEvidenceItems]
	}
	bundle.Evidence = providerEvidence
	bundle.Evidence = fitEvidenceBytes(bundle, budget.MaxRequestBytes-requestFramingBytes)
	providerIDs := makeStringSet(nil)
	for index := range bundle.Evidence {
		bundle.Evidence[index].Visibility = appendVisibility(bundle.Evidence[index].Visibility, VisibilityProviderTarget)
		providerIDs[bundle.Evidence[index].ID] = struct{}{}
	}
	for index := range localEvidence {
		if _, sent := providerIDs[localEvidence[index].ID]; !sent {
			localEvidence[index].Visibility = appendVisibility(localEvidence[index].Visibility, VisibilityNeverProvider)
		}
	}
	for _, item := range bundle.Evidence {
		if item.Location != nil {
			bundle.ProviderAllowedPaths = append(bundle.ProviderAllowedPaths, item.Location.Path)
		}
	}
	bundle.ProviderAllowedPaths = sortedUnique(bundle.ProviderAllowedPaths)

	scope := FocusedInvestigationScope{QuestionID: question.ID, LocalEvidence: localEvidence}
	for _, item := range localEvidence {
		scope.FocusedEvidenceIDs = append(scope.FocusedEvidenceIDs, item.ID)
		if item.Location != nil {
			scope.LocallyInspected = append(scope.LocallyInspected, item.Location.Path)
		}
	}
	for _, item := range bundle.Evidence {
		scope.ProviderEvidenceIDs = append(scope.ProviderEvidenceIDs, item.ID)
	}
	scope.FocusedEvidenceIDs = sortedUnique(scope.FocusedEvidenceIDs)
	scope.ProviderEvidenceIDs = sortedUnique(scope.ProviderEvidenceIDs)
	scope.LocallyInspected = sortedUnique(scope.LocallyInspected)
	return bundle, scope, unknownCandidates, nil
}

func exactEvidenceItem(kind EvidenceKind, symbol, relation string, location evidence.Location, statement string, initial map[string]struct{}) EvidenceItem {
	visibility := []EvidenceVisibility{VisibilityLocalAfter}
	if _, wasInitial := initial[location.Path]; wasInitial {
		visibility = append([]EvidenceVisibility{VisibilityProviderInitial}, visibility...)
	}
	return EvidenceItem{
		ID:   stableID("evidence", strings.Join([]string{string(kind), location.Path, strconv.Itoa(location.Line), symbol, relation}, "\x00")),
		Kind: kind, Statement: statement, Location: &location, Symbol: symbol, Relation: relation,
		Certainty:  evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "go_command_facts", Version: fmt.Sprint(gofacts.CommandTraceVersion), Operation: "focused_research_expansion", Location: &location}},
		Visibility: visibility,
	}
}

func readSourceWindow(reader *reporead.Reader, path string, focusLine int) (SourceWindow, bool) {
	content, err := reader.ReadFile(path, maxSourceReadBytes)
	if err != nil || len(content.Bytes) == 0 || !utf8.Valid(content.Bytes) {
		return SourceWindow{}, false
	}
	if _, sensitive := secretscan.Detect(string(content.Bytes)); sensitive {
		return SourceWindow{}, false
	}
	lines := strings.Split(string(content.Bytes), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return SourceWindow{}, false
	}
	start, end, codeBearing := sourceWindowBounds(path, content.Bytes, lines, focusLine)
	if !codeBearing {
		return SourceWindow{}, false
	}
	selected := make([]string, 0, end-start+1)
	for _, line := range lines[start-1 : end] {
		selected = append(selected, truncateUTF8(line, maxSourceLineBytes))
	}
	return SourceWindow{
		StartLine: start, EndLine: end, Lines: selected,
		CodeBearing: true, Truncated: content.Truncated,
	}, true
}

type codeRange struct {
	start int
	end   int
}

func sourceWindowBounds(path string, content []byte, lines []string, focusLine int) (int, int, bool) {
	if strings.EqualFold(filepath.Ext(path), ".go") {
		selected, ok := goCodeRange(path, content, focusLine, len(lines))
		if !ok {
			return 0, 0, false
		}
		start, end, ok := boundedCodeRange(len(lines), focusLine, selected)
		return start, end, ok
	}

	codeLine := genericCodeLine(lines, focusLine)
	if codeLine == 0 {
		return 0, 0, false
	}
	center := codeLine
	if focusLine > 0 && focusLine <= len(lines) {
		center = focusLine
	}
	start, end := centeredWindow(len(lines), center, maxWindowLines)
	return start, end, codeLine >= start && codeLine <= end
}

func goCodeRange(path string, content []byte, focusLine, lineCount int) (codeRange, bool) {
	fileSet := token.NewFileSet()
	file, _ := parser.ParseFile(fileSet, path, content, parser.SkipObjectResolution)
	if file == nil {
		return codeRange{}, false
	}
	ranges := make([]codeRange, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			start := fileSet.Position(node.Pos()).Line
			end := fileSet.Position(node.End()).Line
			if start > 0 && end >= start {
				ranges = append(ranges, codeRange{start: start, end: end})
			}
		}
		return true
	})
	if len(ranges) == 0 {
		return codeRange{}, false
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].start != ranges[j].start {
			return ranges[i].start < ranges[j].start
		}
		return ranges[i].end < ranges[j].end
	})
	if focusLine <= 0 || focusLine > lineCount {
		return ranges[0], true
	}
	var containing []codeRange
	for _, candidate := range ranges {
		if candidate.start <= focusLine && focusLine <= candidate.end {
			containing = append(containing, candidate)
		}
	}
	if len(containing) > 0 {
		sort.SliceStable(containing, func(i, j int) bool {
			leftLength := containing[i].end - containing[i].start
			rightLength := containing[j].end - containing[j].start
			if leftLength != rightLength {
				return leftLength < rightLength
			}
			return containing[i].start < containing[j].start
		})
		return containing[0], true
	}
	nearest := ranges[0]
	nearestDistance := codeRangeDistance(nearest, focusLine)
	for _, candidate := range ranges[1:] {
		distance := codeRangeDistance(candidate, focusLine)
		if distance < nearestDistance {
			nearest = candidate
			nearestDistance = distance
		}
	}
	spanStart := min(nearest.start, focusLine)
	spanEnd := max(nearest.end, focusLine)
	if spanEnd-spanStart+1 > maxWindowLines {
		return codeRange{}, false
	}
	return nearest, true
}

func codeRangeDistance(candidate codeRange, line int) int {
	if line < candidate.start {
		return candidate.start - line
	}
	if line > candidate.end {
		return line - candidate.end
	}
	return 0
}

func boundedCodeRange(lineCount, focusLine int, selected codeRange) (int, int, bool) {
	if lineCount <= 0 || selected.start <= 0 || selected.end < selected.start {
		return 0, 0, false
	}
	validFocus := focusLine > 0 && focusLine <= lineCount
	if !validFocus {
		focusLine = selected.start
	}
	spanStart := selected.start
	spanEnd := selected.end
	if validFocus {
		spanStart = min(spanStart, focusLine)
		spanEnd = max(spanEnd, focusLine)
	}
	if spanEnd-spanStart+1 <= maxWindowLines {
		start, end := centeredWindow(lineCount, focusLine, maxWindowLines)
		if start > spanStart {
			start = spanStart
			end = min(lineCount, start+maxWindowLines-1)
		}
		if end < spanEnd {
			end = spanEnd
			start = max(1, end-maxWindowLines+1)
		}
		start, end = padWindow(lineCount, start, end, minWindowLines)
		return start, end, start <= focusLine && focusLine <= end && start <= selected.end && end >= selected.start
	}
	if validFocus && selected.start <= focusLine && focusLine <= selected.end {
		start, end := centeredWindow(lineCount, focusLine, maxWindowLines)
		start = max(start, selected.start)
		end = min(end, selected.end)
		if end-start+1 < maxWindowLines {
			start = max(selected.start, end-maxWindowLines+1)
			end = min(selected.end, start+maxWindowLines-1)
		}
		return start, end, start <= focusLine && focusLine <= end
	}
	start := selected.start
	end := min(selected.end, start+maxWindowLines-1)
	start, end = padWindow(lineCount, start, end, minWindowLines)
	return start, end, true
}

func centeredWindow(lineCount, center, size int) (int, int) {
	if size <= 0 || size > lineCount {
		size = lineCount
	}
	center = min(max(center, 1), lineCount)
	start := max(1, center-size/2)
	end := min(lineCount, start+size-1)
	start = max(1, end-size+1)
	return start, end
}

func padWindow(lineCount, start, end, minimum int) (int, int) {
	if end-start+1 >= minimum || lineCount <= end-start+1 {
		return start, end
	}
	missing := minimum - (end - start + 1)
	before := min(start-1, missing/2)
	start -= before
	missing -= before
	end = min(lineCount, end+missing)
	if end-start+1 < minimum {
		start = max(1, end-minimum+1)
	}
	return start, end
}

func genericCodeLine(lines []string, focusLine int) int {
	isCode := func(line string) bool {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
			strings.HasPrefix(line, "*") || strings.HasPrefix(line, "#") {
			return false
		}
		lower := strings.ToLower(line)
		return !strings.HasPrefix(lower, "import ") && !strings.HasPrefix(lower, "from ") &&
			!strings.HasPrefix(lower, "package ") && !strings.HasPrefix(lower, "use ")
	}
	if focusLine > 0 && focusLine <= len(lines) {
		start, end := centeredWindow(len(lines), focusLine, maxWindowLines)
		for line := start; line <= end; line++ {
			if isCode(lines[line-1]) {
				return line
			}
		}
		return 0
	}
	for index, line := range lines {
		if isCode(line) {
			return index + 1
		}
	}
	return 0
}

func hasCodeBearingWindow(items []EvidenceItem) bool {
	for _, item := range items {
		if item.Kind == EvidenceSource && item.Window != nil && item.Window.CodeBearing {
			return true
		}
	}
	return false
}

func prioritizedProviderEvidence(items []EvidenceItem) []EvidenceItem {
	result := append([]EvidenceItem(nil), items...)
	priority := func(kind EvidenceKind) int {
		switch kind {
		case EvidenceSource:
			return 0
		case EvidenceTransition:
			return 1
		case EvidenceCallsite:
			return 2
		case EvidenceDeclaration:
			return 3
		case EvidenceFrontier:
			return 4
		default:
			return 5
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := priority(result[i].Kind)
		right := priority(result[j].Kind)
		if left != right {
			return left < right
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func candidateFocusLine(candidate FileCandidate) int {
	for _, location := range candidate.FocusLocations {
		if location.Path == candidate.Path && location.Line > 0 {
			return location.Line
		}
	}
	return 0
}

func matchingCommandTraces(question string, traces []gofacts.CommandTrace) []gofacts.CommandTrace {
	terms := textTerms(question)
	var matched []gofacts.CommandTrace
	for _, trace := range traces {
		command := strings.ToLower(strings.TrimSpace(trace.Command))
		if command != "" {
			if _, ok := terms[command]; ok {
				matched = append(matched, trace)
				continue
			}
		}
		for _, step := range trace.Steps {
			if pathMatchesTerms(step.TargetLocation.Path, terms) {
				matched = append(matched, trace)
				break
			}
		}
	}
	return matched
}

func bestQuestionCandidates(question ProposedQuestion, candidates map[string]FileCandidate, authorized map[string]struct{}, limit int) []FileCandidate {
	terms := textTerms(question.Question + " " + strings.Join(question.EvidenceCategories, " "))
	var ranked []FileCandidate
	for _, candidate := range candidates {
		if _, ok := authorized[candidate.Path]; !ok {
			continue
		}
		if pathMatchesTerms(candidate.Path, terms) {
			candidate.Score += 100
		}
		ranked = append(ranked, candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].ID < ranked[j].ID
		}
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func fitEvidenceBytes(bundle EvidenceBundle, maxBytes int) []EvidenceItem {
	if maxBytes <= 0 {
		return nil
	}
	items := append([]EvidenceItem(nil), bundle.Evidence...)
	for len(items) > 0 {
		bundle.Evidence = items
		encoded, err := json.Marshal(bundle)
		if err == nil && len(encoded) <= maxBytes {
			return items
		}
		items = items[:len(items)-1]
	}
	return nil
}

func deduplicateEvidence(items []EvidenceItem) []EvidenceItem {
	seen := make(map[string]struct{}, len(items))
	result := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func appendVisibility(values []EvidenceVisibility, value EvidenceVisibility) []EvidenceVisibility {
	result := append([]EvidenceVisibility(nil), values...)
	for _, existing := range result {
		if existing == value {
			return result
		}
	}
	return append(result, value)
}

func rankedPaths(paths map[string]int) []string {
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Slice(result, func(i, j int) bool {
		if paths[result[i]] == paths[result[j]] {
			return result[i] < result[j]
		}
		return paths[result[i]] > paths[result[j]]
	})
	return result
}

func addTraceLocation(
	paths map[string]int,
	focusLines map[string]int,
	authorized map[string]struct{},
	location evidence.Location,
) {
	if location.Path == "" {
		return
	}
	if _, ok := authorized[location.Path]; !ok {
		return
	}
	if current, exists := paths[location.Path]; !exists || current == 0 {
		paths[location.Path] = location.Line
	}
	if current, exists := focusLines[location.Path]; !exists || current == 0 {
		focusLines[location.Path] = location.Line
	}
}

func localGateSignals(items []EvidenceItem, initialPaths map[string]struct{}) []string {
	signals := make(map[string]struct{})
	for _, item := range items {
		if item.Location != nil {
			if _, initial := initialPaths[item.Location.Path]; !initial {
				signals["new_authorized_file"] = struct{}{}
			}
		}
		switch item.Kind {
		case EvidenceCallsite:
			signals["new_exact_callsite"] = struct{}{}
		case EvidenceTransition:
			signals["new_resolved_target"] = struct{}{}
		case EvidenceSource:
			signals["new_source_window"] = struct{}{}
		}
	}
	result := make([]string, 0, len(signals))
	for signal := range signals {
		result = append(result, signal)
	}
	sort.Strings(result)
	return result
}

func questionImpactScore(question ProposedQuestion) int {
	text := strings.ToLower(question.Purpose + " " + question.Question)
	score := len(question.CandidateIDs) * 2
	for _, term := range []string{"backup", "config", "admin", "request", "runtime", "server", "repository", "security", "lifecycle", "user"} {
		if strings.Contains(text, term) {
			score += 3
		}
	}
	return score
}

func isRuntimeOnlyQuestion(question string) bool {
	text := strings.ToLower(question)
	for _, phrase := range []string{"runtime observation only", "requires runtime observation", "dynamic runtime value", "production traffic only"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func textTerms(value string) map[string]struct{} {
	terms := make(map[string]struct{})
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(term) >= 3 {
			terms[term] = struct{}{}
		}
	}
	return terms
}

func pathMatchesTerms(path string, terms map[string]struct{}) bool {
	for _, part := range strings.FieldsFunc(strings.ToLower(filepath.ToSlash(path)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if _, ok := terms[part]; ok {
			return true
		}
	}
	return false
}

func makeStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func stableID(prefix, value string) string {
	digest := sha256.Sum256([]byte(prefix + "\x00" + value))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut]
}
