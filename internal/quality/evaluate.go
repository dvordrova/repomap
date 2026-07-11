package quality

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const (
	EvaluationVersion                    = 2
	orientationContractUnmeasuredWarning = "orientation.contract_unmeasured"
)

type Result struct {
	Version            int                     `json:"version"`
	TaskID             string                  `json:"task_id"`
	Passed             bool                    `json:"passed"`
	DirectionCoverage  DirectionCoverage       `json:"direction_coverage"`
	Grounding          GroundingResult         `json:"grounding"`
	ImportantEvidence  ImportantEvidenceResult `json:"important_evidence"`
	SemanticDrilldown  SemanticDrilldownResult `json:"semantic_drilldown"`
	ForbiddenTripwires ForbiddenTripwireResult `json:"forbidden_phrase_tripwires"`
	ContractAdherence  ContractAdherence       `json:"contract_adherence"`
	BytesAndLatency    BytesAndLatency         `json:"bytes_and_latency"`
}

type DirectionCoverage struct {
	Complete  bool             `json:"complete"`
	Checks    []DirectionCheck `json:"checks"`
	Missing   []string         `json:"missing"`
	Ambiguous []string         `json:"ambiguous"`
}

type DirectionCheck struct {
	DirectionID       string   `json:"direction_id"`
	Covered           bool     `json:"covered"`
	MatchedAlias      string   `json:"matched_alias,omitempty"`
	CandidateNames    []string `json:"candidate_names"`
	SelectedCandidate string   `json:"selected_candidate,omitempty"`
}

type GroundingResult struct {
	Valid                      bool                 `json:"valid"`
	RepositoryMatched          bool                 `json:"repository_matched"`
	AllowedPathCount           int                  `json:"allowed_path_count"`
	ReferencedPathCount        int                  `json:"referenced_path_count"`
	UnscoredProseEvidenceCount int                  `json:"unscored_prose_evidence_count"`
	InvalidReferences          []GroundingReference `json:"invalid_references"`
}

type GroundingReference struct {
	Field string `json:"field"`
	Path  string `json:"path"`
}

type ImportantEvidenceResult struct {
	Complete bool                     `json:"complete"`
	Checks   []ImportantEvidenceCheck `json:"checks"`
	Missing  []string                 `json:"missing"`
}

type ImportantEvidenceCheck struct {
	DirectionID string `json:"direction_id"`
	Path        string `json:"path"`
	Found       bool   `json:"found"`
}

type SemanticDrilldownResult struct {
	Complete          bool             `json:"complete"`
	Symbol            IdentityCheck    `json:"symbol"`
	Path              IdentityCheck    `json:"path"`
	TestTarget        IdentityCheck    `json:"test_evidence_target"`
	TargetsAgree      bool             `json:"source_and_test_targets_agree"`
	Predicates        []PredicateCheck `json:"predicates"`
	MissingPredicates []string         `json:"missing_predicates"`
	Tests             []PathCheck      `json:"tests"`
	MissingTests      []string         `json:"missing_tests"`
	IncompatibleTests []string         `json:"incompatible_tests"`
}

type IdentityCheck struct {
	Expected string `json:"expected"`
	Observed string `json:"observed,omitempty"`
	Matched  bool   `json:"matched"`
}

type PredicateCheck struct {
	Predicate sourceexplain.Predicate `json:"predicate"`
	Found     bool                    `json:"found"`
}

type PathCheck struct {
	Path                   string `json:"path"`
	Found                  bool   `json:"found"`
	ScenarioCompatible     bool   `json:"scenario_compatible"`
	GoplsVersionCompatible bool   `json:"gopls_version_compatible"`
	ContextCompatible      bool   `json:"context_compatible"`
}

type ForbiddenTripwireResult struct {
	Clear     bool             `json:"clear"`
	Checks    []TripwireCheck  `json:"checks"`
	Triggered []ForbiddenMatch `json:"triggered"`
}

type TripwireCheck struct {
	ID        string         `json:"id"`
	Scope     OverclaimScope `json:"scope"`
	Triggered bool           `json:"triggered"`
}

type ForbiddenMatch struct {
	ID          string         `json:"id"`
	Scope       OverclaimScope `json:"scope"`
	ContainsAll []string       `json:"contains_all"`
}

type ContractAdherence struct {
	OrientationContext  ContractCheck `json:"orientation_context"`
	OrientationResponse ContractCheck `json:"orientation_response"`
	SourceBundle        ContractCheck `json:"source_bundle"`
	SourceResponse      ContractCheck `json:"source_response"`
	TestEvidence        ContractCheck `json:"test_evidence"`
}

type ContractCheck struct {
	Decoded      bool                       `json:"decoded"`
	Valid        bool                       `json:"valid"`
	Measured     bool                       `json:"measured"`
	Clean        bool                       `json:"clean"`
	Error        string                     `json:"error,omitempty"`
	WarningCodes []string                   `json:"warning_codes"`
	Evaluation   *ContractEvaluationSummary `json:"evaluation,omitempty"`
}

type ContractEvaluationSummary struct {
	Version  int `json:"version"`
	Score    int `json:"score"`
	MaxScore int `json:"max_score"`
}

type BytesAndLatency struct {
	Orientation       StageObservation `json:"orientation"`
	Source            StageObservation `json:"source"`
	TestEvidenceBytes int              `json:"test_evidence_bytes"`
}

type StageObservation struct {
	ReplayInputBytes     int    `json:"replay_input_bytes"`
	ResponseBytes        int    `json:"response_bytes"`
	ModelContextBytes    int    `json:"model_context_bytes"`
	ProviderRequestBytes *int   `json:"provider_request_bytes"`
	LatencyMillis        *int64 `json:"latency_ms"`
}

type orientationResponse struct {
	ProjectGuess         string                      `json:"project_guess"`
	Confidence           float64                     `json:"confidence"`
	HighLevelMap         []orientationMapItem        `json:"high_level_map"`
	FirstFilesToOpen     []orientationPath           `json:"first_files_to_open"`
	CandidateFlows       []flowexplain.CandidateFlow `json:"candidate_flows"`
	ImportantDomainWords []orientationDomainWord     `json:"important_domain_words"`
	QuestionsForHuman    []string                    `json:"questions_for_human"`
	UnverifiedPaths      []orientationPath           `json:"unverified_paths"`
	Warnings             []string                    `json:"warnings"`
}

type orientationMapItem struct {
	Name         string   `json:"name"`
	Evidence     []string `json:"evidence"`
	WhyItMatters string   `json:"why_it_matters"`
}

type orientationPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type orientationDomainWord struct {
	Word     string   `json:"word"`
	Guess    string   `json:"guess"`
	Evidence []string `json:"evidence"`
}

// Evaluate replays already-loaded artifacts. Artifact failures are recorded in
// their own result slices so useful direction selection cannot hide malformed
// source evidence, invalid paths, or missing test navigation.
func Evaluate(loaded LoadedTask) (Result, error) {
	if err := loaded.Task.Validate(); err != nil {
		return Result{}, err
	}

	result := Result{
		Version:            EvaluationVersion,
		TaskID:             loaded.Task.ID,
		DirectionCoverage:  emptyDirectionCoverage(),
		Grounding:          GroundingResult{InvalidReferences: []GroundingReference{}},
		ImportantEvidence:  emptyImportantEvidence(),
		SemanticDrilldown:  emptySemanticDrilldown(loaded.Task.Expected.Drilldown),
		ForbiddenTripwires: emptyForbiddenTripwires(),
		ContractAdherence: ContractAdherence{
			OrientationContext:  emptyContractCheck(),
			OrientationResponse: emptyContractCheck(),
			SourceBundle:        emptyContractCheck(),
			SourceResponse:      emptyContractCheck(),
			TestEvidence:        emptyContractCheck(),
		},
		BytesAndLatency: observations(loaded),
	}

	context, contextCheck := decodeOrientationContext(loaded.OrientationContext)
	result.ContractAdherence.OrientationContext = contextCheck
	response, responseCheck := decodeOrientationResponse(
		loaded.OrientationResponse,
		loaded.Task.Captures.Orientation.ResponseForm,
	)
	result.ContractAdherence.OrientationResponse = responseCheck

	result.DirectionCoverage, result.ImportantEvidence = evaluateDirections(
		loaded.Task.Expected.Directions,
		response.CandidateFlows,
	)
	result.Grounding = evaluateGrounding(
		loaded.Task.Repository.Name,
		context,
		response,
		contextCheck.Valid,
	)

	sourceBundle, sourceBundleCheck := decodeSourceBundle(loaded.SourceBundle)
	if sourceBundleCheck.Valid && sourceBundle.RepoName != loaded.Task.Repository.Name {
		sourceBundleCheck.Valid = false
		sourceBundleCheck.Clean = false
		sourceBundleCheck.Error = fmt.Sprintf(
			"quality: source bundle repository %q does not match task repository %q",
			sourceBundle.RepoName,
			loaded.Task.Repository.Name,
		)
	}
	result.ContractAdherence.SourceBundle = sourceBundleCheck
	var sourceResult sourceexplain.ParseResult
	if sourceBundleCheck.Valid {
		sourceResult, result.ContractAdherence.SourceResponse = decodeSourceResponse(
			sourceBundle,
			loaded.SourceResponse,
		)
	} else {
		result.ContractAdherence.SourceResponse.Error = "source bundle is invalid"
	}

	testBundle, testCheck := decodeTestEvidence(loaded.TestEvidence)
	result.ContractAdherence.TestEvidence = testCheck
	result.SemanticDrilldown = evaluateDrilldown(
		loaded.Task.Expected.Drilldown,
		sourceBundle,
		sourceBundleCheck.Valid,
		sourceResult,
		result.ContractAdherence.SourceResponse.Valid,
		testBundle,
		testCheck.Valid,
		loaded.Task.Repository.Scenario,
	)
	result.ForbiddenTripwires = evaluateForbiddenTripwires(
		loaded.Task.Expected.ForbiddenOverclaims,
		loaded.OrientationResponse,
		loaded.SourceResponse,
		loaded.TestEvidence,
	)
	result.Passed = result.DirectionCoverage.Complete &&
		result.Grounding.Valid &&
		result.ImportantEvidence.Complete &&
		result.SemanticDrilldown.Complete &&
		result.ForbiddenTripwires.Clear &&
		result.ContractAdherence.clean()
	return result, nil
}

func decodeOrientationContext(data []byte) (OrientationGroundingContext, ContractCheck) {
	check := emptyContractCheck()
	check.Measured = true
	var context OrientationGroundingContext
	if err := decodeStrictJSON(data, &context); err != nil {
		check.Error = err.Error()
		return context, check
	}
	check.Decoded = true
	if err := context.Validate(); err != nil {
		check.Error = err.Error()
		return context, check
	}
	check.Valid = true
	check.Clean = true
	return context, check
}

func decodeOrientationResponse(data []byte, responseForm ResponseForm) (orientationResponse, ContractCheck) {
	check := emptyContractCheck()
	var response orientationResponse
	// Product orientation uses json.Unmarshal: unknown legacy/provider fields
	// are tolerated, while malformed JSON and trailing values are still errors.
	if err := json.Unmarshal(data, &response); err != nil {
		check.Error = err.Error()
		return response, check
	}
	check.Decoded = true
	if err := validateOrientationResponse(response); err != nil {
		check.Error = err.Error()
		return response, check
	}
	check.Valid = true
	if responseForm == ResponseFormNormalizedReport {
		check.WarningCodes = []string{orientationContractUnmeasuredWarning}
		return response, check
	}

	check.Measured = true
	if err := validateRawOrientationShape(data); err != nil {
		check.Error = err.Error()
		return response, check
	}
	var strict orientationResponse
	if err := decodeStrictJSON(data, &strict); err != nil {
		check.Error = fmt.Sprintf("quality: raw orientation response contract drift: %v", err)
		return response, check
	}
	check.Clean = true
	return response, check
}

func validateRawOrientationShape(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("quality: decode raw orientation fields: %w", err)
	}
	arrayFields := map[string]bool{
		"high_level_map":         true,
		"first_files_to_open":    true,
		"candidate_flows":        true,
		"important_domain_words": true,
		"questions_for_human":    true,
		"unverified_paths":       true,
		"warnings":               true,
	}
	for _, field := range []string{
		"project_guess",
		"confidence",
		"high_level_map",
		"first_files_to_open",
		"candidate_flows",
		"important_domain_words",
		"questions_for_human",
		"unverified_paths",
		"warnings",
	} {
		raw, exists := fields[field]
		trimmed := strings.TrimSpace(string(raw))
		if !exists || trimmed == "" || trimmed == "null" {
			return fmt.Errorf("quality: raw orientation required field %q is missing or null", field)
		}
		if arrayFields[field] && !strings.HasPrefix(trimmed, "[") {
			return fmt.Errorf("quality: raw orientation required field %q is not an array", field)
		}
	}
	return nil
}

func validateOrientationResponse(response orientationResponse) error {
	if strings.TrimSpace(response.ProjectGuess) == "" {
		return fmt.Errorf("quality: orientation response project_guess is required")
	}
	if response.Confidence < 0 || response.Confidence > 1 {
		return fmt.Errorf("quality: orientation response confidence is outside [0,1]")
	}
	if len(response.CandidateFlows) == 0 {
		return fmt.Errorf("quality: orientation response has no candidate flows")
	}
	for index, item := range response.HighLevelMap {
		if strings.TrimSpace(item.Name) == "" ||
			strings.TrimSpace(item.WhyItMatters) == "" ||
			!nonEmptyStrings(item.Evidence) {
			return fmt.Errorf("quality: orientation response high_level_map[%d] is incomplete", index)
		}
	}
	for index, item := range response.FirstFilesToOpen {
		if err := validateOrientationPath(item); err != nil {
			return fmt.Errorf("quality: orientation response first_files_to_open[%d]: %w", index, err)
		}
	}
	for index, candidate := range response.CandidateFlows {
		if strings.TrimSpace(candidate.Name) == "" ||
			strings.TrimSpace(candidate.Trigger) == "" ||
			strings.TrimSpace(candidate.LikelyEntrypoint) == "" ||
			strings.TrimSpace(candidate.WhyInteresting) == "" ||
			!nonEmptyStrings(candidate.LikelyFiles) || !nonEmptyStrings(candidate.Evidence) ||
			candidate.Confidence < 0 || candidate.Confidence > 1 {
			return fmt.Errorf("quality: orientation response candidate_flows[%d] is incomplete", index)
		}
		for _, path := range candidate.LikelyFiles {
			if !validRelativePath(path) {
				return fmt.Errorf("quality: orientation response candidate_flows[%d] has invalid likely file %q", index, path)
			}
		}
	}
	for index, item := range response.ImportantDomainWords {
		if strings.TrimSpace(item.Word) == "" ||
			strings.TrimSpace(item.Guess) == "" ||
			!nonEmptyStrings(item.Evidence) {
			return fmt.Errorf("quality: orientation response important_domain_words[%d] is incomplete", index)
		}
	}
	for index, item := range response.UnverifiedPaths {
		if err := validateOrientationPath(item); err != nil {
			return fmt.Errorf("quality: orientation response unverified_paths[%d]: %w", index, err)
		}
	}
	return nil
}

func validateOrientationPath(item orientationPath) error {
	if !validRelativePath(item.Path) || strings.TrimSpace(item.Reason) == "" {
		return fmt.Errorf("path and reason are required")
	}
	return nil
}

func nonEmptyStrings(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func decodeSourceBundle(data []byte) (sourceexplain.Bundle, ContractCheck) {
	check := emptyContractCheck()
	check.Measured = true
	var bundle sourceexplain.Bundle
	if err := decodeStrictJSON(data, &bundle); err != nil {
		check.Error = err.Error()
		return bundle, check
	}
	check.Decoded = true
	if err := bundle.Validate(); err != nil {
		check.Error = err.Error()
		return bundle, check
	}
	check.Valid = true
	check.Clean = true
	return bundle, check
}

func decodeSourceResponse(bundle sourceexplain.Bundle, data []byte) (sourceexplain.ParseResult, ContractCheck) {
	check := emptyContractCheck()
	check.Measured = true
	result, err := sourceexplain.ParseReport(bundle, data)
	if err != nil {
		check.Error = err.Error()
		return sourceexplain.ParseResult{}, check
	}
	check.Decoded = true
	check.Valid = true
	evaluation := sourceexplain.Evaluate(result)
	check.Evaluation = &ContractEvaluationSummary{
		Version:  evaluation.Version,
		Score:    evaluation.Score,
		MaxScore: evaluation.MaxScore,
	}
	check.WarningCodes = make([]string, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		check.WarningCodes = append(check.WarningCodes, warning.Code)
	}
	sort.Strings(check.WarningCodes)
	check.Clean = len(check.WarningCodes) == 0 && evaluation.Score == evaluation.MaxScore
	return result, check
}

func decodeTestEvidence(data []byte) (testevidence.Bundle, ContractCheck) {
	check := emptyContractCheck()
	check.Measured = true
	var bundle testevidence.Bundle
	if err := decodeStrictJSON(data, &bundle); err != nil {
		check.Error = err.Error()
		return bundle, check
	}
	check.Decoded = true
	if err := bundle.Validate(); err != nil {
		check.Error = err.Error()
		return bundle, check
	}
	check.Valid = true
	check.Clean = true
	return bundle, check
}

func evaluateDirections(
	expected []DirectionExpectation,
	candidates []flowexplain.CandidateFlow,
) (DirectionCoverage, ImportantEvidenceResult) {
	coverage := emptyDirectionCoverage()
	important := emptyImportantEvidence()
	for _, direction := range expected {
		check := DirectionCheck{
			DirectionID:    direction.ID,
			CandidateNames: []string{},
		}
		var selected flowexplain.CandidateFlow
		selectedPaths := map[string]struct{}{}
		selectedCoverage := -1
		selectedComplete := false
		selectedAlias := ""
		for _, candidate := range candidates {
			alias := matchedDirectionAlias(direction.Aliases, candidate.Name)
			if alias == "" {
				continue
			}
			check.CandidateNames = append(check.CandidateNames, candidate.Name)
			paths := candidateEvidencePaths(candidate)
			coverage := countExpectedPaths(direction.ImportantPaths, paths)
			complete := coverage == len(direction.ImportantPaths)
			if selectedCoverage < 0 ||
				(complete && !selectedComplete) ||
				(complete == selectedComplete && coverage > selectedCoverage) ||
				(complete == selectedComplete && coverage == selectedCoverage && candidate.Confidence > selected.Confidence) ||
				(complete == selectedComplete && coverage == selectedCoverage && candidate.Confidence == selected.Confidence && candidate.Name < selected.Name) {
				selected = candidate
				selectedPaths = paths
				selectedCoverage = coverage
				selectedComplete = complete
				selectedAlias = alias
			}
		}
		check.Covered = len(check.CandidateNames) > 0
		if check.Covered {
			check.MatchedAlias = selectedAlias
			check.SelectedCandidate = selected.Name
		}
		coverage.Checks = append(coverage.Checks, check)
		if !check.Covered {
			coverage.Missing = append(coverage.Missing, direction.ID)
		} else if len(check.CandidateNames) > 1 {
			coverage.Ambiguous = append(coverage.Ambiguous, direction.ID)
		}

		for _, path := range direction.ImportantPaths {
			_, found := selectedPaths[path]
			important.Checks = append(important.Checks, ImportantEvidenceCheck{
				DirectionID: direction.ID,
				Path:        path,
				Found:       found,
			})
			if !found {
				important.Missing = append(important.Missing, direction.ID+":"+path)
			}
		}
	}
	coverage.Complete = len(coverage.Missing) == 0 && len(coverage.Ambiguous) == 0
	important.Complete = len(important.Missing) == 0
	return coverage, important
}

func matchedDirectionAlias(aliases []string, candidateName string) string {
	haystack := strings.ToLower(candidateName)
	for _, alias := range aliases {
		if strings.Contains(haystack, strings.ToLower(alias)) {
			return alias
		}
	}
	return ""
}

func candidateEvidencePaths(candidate flowexplain.CandidateFlow) map[string]struct{} {
	paths := make(map[string]struct{}, len(candidate.LikelyFiles)+len(candidate.Evidence)+1)
	for _, path := range candidate.LikelyFiles {
		paths[path] = struct{}{}
	}
	for _, evidenceItem := range candidate.Evidence {
		if validRelativePath(evidenceItem) {
			paths[evidenceItem] = struct{}{}
		}
	}
	if strings.HasSuffix(strings.ToLower(candidate.LikelyEntrypoint), ".go") {
		paths[candidate.LikelyEntrypoint] = struct{}{}
	}
	return paths
}

func countExpectedPaths(expected []string, found map[string]struct{}) int {
	count := 0
	for _, path := range expected {
		if _, ok := found[path]; ok {
			count++
		}
	}
	return count
}

func evaluateGrounding(
	wantRepo string,
	context OrientationGroundingContext,
	response orientationResponse,
	contextValid bool,
) GroundingResult {
	result := GroundingResult{
		RepositoryMatched: contextValid && context.RepoName == wantRepo,
		AllowedPathCount:  len(context.AllowedPaths),
		InvalidReferences: []GroundingReference{},
	}
	allowed := make(map[string]struct{}, len(context.AllowedPaths))
	for _, path := range context.AllowedPaths {
		allowed[path] = struct{}{}
	}
	seen := make(map[string]struct{})
	check := func(field, path string) {
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		result.ReferencedPathCount++
		if _, exists := allowed[path]; !contextValid || !exists {
			result.InvalidReferences = append(result.InvalidReferences, GroundingReference{
				Field: field,
				Path:  path,
			})
		}
	}
	checkEvidence := func(field, value string) {
		_, explicitlyAllowed := allowed[value]
		if explicitlyAllowed || looksLikeStructuredEvidencePath(value) {
			check(field, value)
			return
		}
		result.UnscoredProseEvidenceCount++
	}
	for index, file := range response.FirstFilesToOpen {
		check(fmt.Sprintf("first_files_to_open[%d].path", index), file.Path)
	}
	for itemIndex, item := range response.HighLevelMap {
		for evidenceIndex, evidenceItem := range item.Evidence {
			checkEvidence(fmt.Sprintf("high_level_map[%d].evidence[%d]", itemIndex, evidenceIndex), evidenceItem)
		}
	}
	for candidateIndex, candidate := range response.CandidateFlows {
		for pathIndex, path := range candidate.LikelyFiles {
			check(fmt.Sprintf("candidate_flows[%d].likely_files[%d]", candidateIndex, pathIndex), path)
		}
		for evidenceIndex, evidenceItem := range candidate.Evidence {
			checkEvidence(fmt.Sprintf("candidate_flows[%d].evidence[%d]", candidateIndex, evidenceIndex), evidenceItem)
		}
		if strings.HasSuffix(strings.ToLower(candidate.LikelyEntrypoint), ".go") {
			check(fmt.Sprintf("candidate_flows[%d].likely_entrypoint", candidateIndex), candidate.LikelyEntrypoint)
		}
	}
	for itemIndex, item := range response.ImportantDomainWords {
		for evidenceIndex, evidenceItem := range item.Evidence {
			checkEvidence(fmt.Sprintf("important_domain_words[%d].evidence[%d]", itemIndex, evidenceIndex), evidenceItem)
		}
	}
	result.Valid = result.RepositoryMatched && len(result.InvalidReferences) == 0
	return result
}

func looksLikeStructuredEvidencePath(value string) bool {
	if !validRelativePath(value) || len(strings.Fields(value)) != 1 || strings.ContainsAny(value, "*?[]") {
		return false
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".go", ".md", ".yaml", ".yml", ".json", ".toml", ".proto",
		".mod", ".sum", ".sh", ".c", ".h", ".rs", ".py", ".js", ".ts":
		return true
	default:
		return false
	}
}

func evaluateDrilldown(
	expected DrilldownExpectation,
	bundle sourceexplain.Bundle,
	bundleValid bool,
	parsed sourceexplain.ParseResult,
	responseValid bool,
	tests testevidence.Bundle,
	testsValid bool,
	scenario BuildScenario,
) SemanticDrilldownResult {
	result := emptySemanticDrilldown(expected)
	if bundleValid {
		result.Symbol.Observed = bundle.Target.Name
		result.Symbol.Matched = bundle.Target.Name == expected.Symbol
		result.Path.Observed = bundle.Target.Path
		result.Path.Matched = bundle.Target.Path == expected.Path
	}
	if testsValid {
		result.TestTarget.Observed = tests.TargetName
		result.TestTarget.Matched = tests.TargetName == expected.Symbol
	}
	result.TargetsAgree = bundleValid && testsValid && bundle.Target.Name == tests.TargetName
	foundPredicates := make(map[sourceexplain.Predicate]struct{})
	if responseValid {
		for _, claim := range parsed.Report.Claims {
			foundPredicates[claim.Predicate] = struct{}{}
		}
	}
	for _, predicate := range expected.SourcePredicates {
		_, found := foundPredicates[predicate]
		result.Predicates = append(result.Predicates, PredicateCheck{
			Predicate: predicate,
			Found:     found,
		})
		if !found {
			result.MissingPredicates = append(result.MissingPredicates, string(predicate))
		}
	}
	for _, path := range expected.TestReferencePaths {
		check := PathCheck{Path: path}
		if testsValid {
			for _, reference := range tests.References {
				if reference.Path != path {
					continue
				}
				check.Found = true
				scenarioCompatible := referenceScenarioCompatible(reference, tests, scenario)
				goplsCompatible := referenceGoplsVersionCompatible(reference, scenario.GoplsVersion)
				check.ScenarioCompatible = check.ScenarioCompatible || scenarioCompatible
				check.GoplsVersionCompatible = check.GoplsVersionCompatible || goplsCompatible
				check.ContextCompatible = check.ContextCompatible || (scenarioCompatible && goplsCompatible)
			}
		}
		result.Tests = append(result.Tests, check)
		if !check.Found {
			result.MissingTests = append(result.MissingTests, path)
		} else if !check.ContextCompatible {
			result.IncompatibleTests = append(result.IncompatibleTests, path)
		}
	}
	result.Complete = result.Symbol.Matched &&
		result.Path.Matched &&
		result.TestTarget.Matched &&
		result.TargetsAgree &&
		len(result.MissingPredicates) == 0 &&
		len(result.MissingTests) == 0 &&
		len(result.IncompatibleTests) == 0
	return result
}

func referenceScenarioCompatible(
	reference testevidence.Reference,
	tests testevidence.Bundle,
	want BuildScenario,
) bool {
	for _, scenarioID := range reference.Scenarios {
		for _, candidate := range tests.Scenarios {
			if candidate.ID != scenarioID {
				continue
			}
			if candidate.Build.GOOS == want.GOOS &&
				candidate.Build.GOARCH == want.GOARCH &&
				equalStringSets(candidate.Build.BuildTags, want.BuildTags) {
				return true
			}
		}
	}
	return false
}

func referenceGoplsVersionCompatible(reference testevidence.Reference, wantVersion string) bool {
	for _, provenance := range reference.Provenance {
		if provenance.Provider == "gopls" && provenance.Version == wantVersion {
			return true
		}
	}
	return false
}

func equalStringSets(left, right []string) bool {
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if _, ok := rightSet[value]; !ok {
			return false
		}
	}
	return true
}

func evaluateForbiddenTripwires(
	expected []ForbiddenOverclaim,
	orientationResponse []byte,
	sourceResponse []byte,
	testEvidence []byte,
) ForbiddenTripwireResult {
	result := emptyForbiddenTripwires()
	for _, overclaim := range expected {
		artifact := orientationResponse
		if overclaim.Scope == OverclaimScopeDrilldown {
			artifact = make([]byte, 0, len(sourceResponse)+1+len(testEvidence))
			artifact = append(artifact, sourceResponse...)
			artifact = append(artifact, '\n')
			artifact = append(artifact, testEvidence...)
		}
		triggered := containsAllFold(string(artifact), overclaim.ContainsAll)
		result.Checks = append(result.Checks, TripwireCheck{
			ID:        overclaim.ID,
			Scope:     overclaim.Scope,
			Triggered: triggered,
		})
		if triggered {
			result.Triggered = append(result.Triggered, ForbiddenMatch{
				ID:          overclaim.ID,
				Scope:       overclaim.Scope,
				ContainsAll: append([]string{}, overclaim.ContainsAll...),
			})
		}
	}
	result.Clear = len(result.Triggered) == 0
	return result
}

func containsAllFold(text string, fragments []string) bool {
	text = strings.ToLower(text)
	for _, fragment := range fragments {
		if !strings.Contains(text, strings.ToLower(fragment)) {
			return false
		}
	}
	return true
}

func observations(loaded LoadedTask) BytesAndLatency {
	return BytesAndLatency{
		Orientation: StageObservation{
			ReplayInputBytes:     len(loaded.OrientationContext),
			ResponseBytes:        len(loaded.OrientationResponse),
			ModelContextBytes:    loaded.Task.Captures.Orientation.ModelContextBytes,
			ProviderRequestBytes: cloneInt(loaded.Task.Captures.Orientation.ProviderRequestBytes),
			LatencyMillis:        cloneInt64(loaded.Task.Captures.Orientation.LatencyMillis),
		},
		Source: StageObservation{
			ReplayInputBytes:     len(loaded.SourceBundle),
			ResponseBytes:        len(loaded.SourceResponse),
			ModelContextBytes:    loaded.Task.Captures.Source.ModelContextBytes,
			ProviderRequestBytes: cloneInt(loaded.Task.Captures.Source.ProviderRequestBytes),
			LatencyMillis:        cloneInt64(loaded.Task.Captures.Source.LatencyMillis),
		},
		TestEvidenceBytes: len(loaded.TestEvidence),
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func emptyDirectionCoverage() DirectionCoverage {
	return DirectionCoverage{
		Checks:    []DirectionCheck{},
		Missing:   []string{},
		Ambiguous: []string{},
	}
}

func emptyImportantEvidence() ImportantEvidenceResult {
	return ImportantEvidenceResult{Checks: []ImportantEvidenceCheck{}, Missing: []string{}}
}

func emptySemanticDrilldown(expected DrilldownExpectation) SemanticDrilldownResult {
	return SemanticDrilldownResult{
		Symbol:            IdentityCheck{Expected: expected.Symbol},
		Path:              IdentityCheck{Expected: expected.Path},
		TestTarget:        IdentityCheck{Expected: expected.Symbol},
		Predicates:        []PredicateCheck{},
		MissingPredicates: []string{},
		Tests:             []PathCheck{},
		MissingTests:      []string{},
		IncompatibleTests: []string{},
	}
}

func emptyForbiddenTripwires() ForbiddenTripwireResult {
	return ForbiddenTripwireResult{Checks: []TripwireCheck{}, Triggered: []ForbiddenMatch{}}
}

func emptyContractCheck() ContractCheck {
	return ContractCheck{WarningCodes: []string{}}
}

func (c ContractAdherence) clean() bool {
	legacyOrientationAccepted := c.OrientationResponse.Decoded &&
		c.OrientationResponse.Valid &&
		!c.OrientationResponse.Measured &&
		!c.OrientationResponse.Clean &&
		len(c.OrientationResponse.WarningCodes) == 1 &&
		c.OrientationResponse.WarningCodes[0] == orientationContractUnmeasuredWarning
	orientationAccepted := measuredClean(c.OrientationResponse) || legacyOrientationAccepted
	return measuredClean(c.OrientationContext) &&
		orientationAccepted &&
		measuredClean(c.SourceBundle) &&
		measuredClean(c.SourceResponse) &&
		measuredClean(c.TestEvidence)
}

func measuredClean(check ContractCheck) bool {
	return check.Decoded && check.Valid && check.Measured && check.Clean
}
