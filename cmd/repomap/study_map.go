package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	studyMapMaxSourceFunctions      = 24
	studyMapMaxDocumentExcerptBytes = 1200
	studyMapMaxDocumentExcerptTotal = 6000
	studyMapMaxDocumentReadBytes    = 16 << 10
	studyMapStatusVersion           = 1
)

const studyMapSystemPrompt = `You are an editorial onboarding planner for one bounded repository model. The supplied objects and opaque IDs are the complete authority. Produce a Repository Brief and useful source-backed study directions. A Study Direction is a recommendation about what to read, not a runtime claim. Return valid JSON only. Never invent or alter a file, symbol, component, document, mechanism, relation, fact, or ID.`

const studyMapUserPrompt = `Return exactly this JSON shape:
{
  "version": 1,
  "repository_type": "service_application | library_framework | cli_tool | monorepo | mixed",
  "brief": {
    "what_it_is": {"text": "short answer", "support_ids": ["exact supplied ids"]},
    "problem": {"text": "problem it addresses", "support_ids": ["exact supplied ids"]},
    "main_input": {"text": "main input or trigger, or empty when unavailable", "support_ids": ["exact supplied ids"]},
    "central_responsibility": {"text": "central responsibility", "support_ids": ["exact supplied ids"]},
    "observable_result": {"text": "observable result, or empty when unavailable", "support_ids": ["exact supplied ids"]},
    "domain_terms": [{"term": "term", "meaning": "short meaning", "support_ids": ["exact supplied ids"]}]
  },
  "shape_area_ids": ["one to seven exact supplied area ids"],
  "candidates": [
    {
      "question": "natural developer question ending in ?",
      "why_it_matters": "one sentence",
      "learning_outcome": "what the reader will understand",
      "target_user_job": "first_contact | use_or_operate | extend_or_integrate | contribute | debug_or_maintain",
      "learning_stage": "orientation | central_operation | core_model | integration | operations | contribution",
      "anchor_ids": ["three to five exact supplied code anchor ids"],
      "document_ids": ["zero or more exact supplied document ids"],
      "area_ids": ["one or more exact supplied area ids"],
      "mechanism_id": "exact supplied canonical mechanism id or empty",
      "reading_anchors": [
        {"anchor_id": "one selected anchor id", "label": "Start here | Then inspect | Related implementation | Public boundary | Core data type", "what_to_look_for": "bounded editorial reading instruction"}
      ],
      "confidence": "low | medium | high",
      "search_queries": ["natural search wording"]
    }
  ]
}

Rules:
- Return eight to twelve diverse candidates when the supplied repository objects support them; never exceed twelve.
- Every support, area, document, anchor, and mechanism ID must be copied exactly from the supplied bundle.
- Every candidate must use three to five production-relevant code anchors and list those anchors exactly once in reading_anchors.
- reading_anchors.label is a closed schema value. Copy one of the five listed English literals exactly; the report localizes it later.
- Favor central repository responsibilities over easy helpers, examples, fixtures, previews, evaluators, generated code, and isolated declarations.
- Use repository_type only to diversify coverage. Do not force a generic checklist unsupported by the repository.
- Brief documentation support describes intended purpose, not proven runtime behavior. Leave a brief field empty instead of guessing.
- Questions may ask about behavior. Explanations and reading instructions must not assert an execution sequence unless the attached canonical Mechanism already establishes it.
- Reading-anchor order is editorial. Never write “then the system”, “next it executes”, “runtime step”, “proven sequence”, or equivalent wording.
- Attach a canonical mechanism only when its supplied anchors genuinely overlap the direction. Otherwise leave mechanism_id empty.
- Use concise human language. Do not mention facts, candidates, validation, confidence, gaps, model metadata, or internal IDs in prose.

Variable bounded repository bundle JSON:
`

type studyMapStatus struct {
	Version               int                             `json:"version"`
	State                 string                          `json:"state"`
	FailureReason         string                          `json:"failure_reason,omitempty"`
	RepositoryType        studymap.RepositoryType         `json:"repository_type,omitempty"`
	Anchors               int                             `json:"code_anchors"`
	Areas                 int                             `json:"areas"`
	Documents             int                             `json:"documents"`
	Mechanisms            int                             `json:"canonical_mechanisms"`
	Candidates            int                             `json:"candidates"`
	Validated             int                             `json:"validated_candidates"`
	Selected              int                             `json:"selected_directions"`
	Metrics               semanticDiscoveryStageMetrics   `json:"model_stage"`
	Stages                []semanticDiscoveryStageMetrics `json:"model_stages,omitempty"`
	ProviderLatencyMillis int64                           `json:"summed_provider_latency_ms,omitempty"`
	WallMillis            int64                           `json:"wall_ms"`
	LocalReplay           bool                            `json:"local_replay,omitempty"`
}

type studyMapSourceOutcome string

const (
	studyMapNoSupportedSourceAdapter  studyMapSourceOutcome = "no_supported_source_adapter"
	studyMapNoEligibleSourceFunctions studyMapSourceOutcome = "no_eligible_source_functions"
)

// studyMapSourceOutcomeError is a typed local availability result. It carries
// no filesystem cause so an absent optional research directory cannot leak
// into the product-facing diagnostic.
type studyMapSourceOutcomeError struct {
	Outcome studyMapSourceOutcome
}

func (err *studyMapSourceOutcomeError) Error() string {
	return "study map: " + string(err.Outcome)
}

func studyMapSourceOutcomeCode(err error) (studyMapSourceOutcome, bool) {
	var outcomeErr *studyMapSourceOutcomeError
	if !errors.As(err, &outcomeErr) {
		return "", false
	}
	return outcomeErr.Outcome, true
}

func unavailableStudyMapSourceOutcome(data *report.ReportData) error {
	hasGeneralSource := false
	hasGoAdapterSource := false
	if data != nil {
		for _, filePath := range data.OpenablePaths {
			if !artifactrole.IsSourcePath(filePath) {
				continue
			}
			hasGeneralSource = true
			if strings.EqualFold(path.Ext(filePath), ".go") {
				hasGoAdapterSource = true
			}
		}
	}
	outcome := studyMapNoEligibleSourceFunctions
	if hasGeneralSource && !hasGoAdapterSource {
		outcome = studyMapNoSupportedSourceAdapter
	}
	return &studyMapSourceOutcomeError{Outcome: outcome}
}

type studyMapAttempt struct {
	Version         int                           `json:"version"`
	PromptVersion   string                        `json:"prompt_version"`
	BundleSHA256    string                        `json:"bundle_sha256"`
	ValidationState string                        `json:"validation_state"`
	FailureReason   string                        `json:"failure_reason,omitempty"`
	Metrics         semanticDiscoveryStageMetrics `json:"metrics"`
	Response        json.RawMessage               `json:"response,omitempty"`
	RawResponse     string                        `json:"raw_response,omitempty"`
}

func editStudyMapForRun(
	ctx context.Context,
	runDir string,
	repoRoot string,
	stderr io.Writer,
) (studyMapStatus, error) {
	return prepareStudyMapWithProviderFactory(ctx, runDir, repoRoot, func() (semanticDiscoveryEditor, error) {
		client, err := deepseek.NewFromEnv()
		if err != nil {
			return nil, fmt.Errorf("study map: provider configuration: %w", err)
		}
		var progressMu sync.Mutex
		client.OnWait = func(progress deepseek.WaitProgress) {
			progressMu.Lock()
			defer progressMu.Unlock()
			fmt.Fprintf(
				stderr,
				"repomap: %s still running after %s (Ctrl-C to cancel)\n",
				progress.Stage,
				progress.Elapsed.Round(time.Second),
			)
		}
		return client, nil
	})
}

// prepareStudyMapMonolithic is retained only so saved v3.1 attempt fixtures
// remain understandable. Production calls use prepareStudyMap in
// study_map_v32.go and never execute this response shape.
func prepareStudyMapMonolithic(
	ctx context.Context,
	runDir string,
	repoRoot string,
	provider semanticDiscoveryEditor,
) (status studyMapStatus, returnErr error) {
	started := time.Now()
	status = studyMapStatus{Version: studyMapStatusVersion, State: "started"}
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil && status.State == "started" {
			status.State = "failed"
			status.FailureReason = semanticDiscoveryReason(returnErr.Error())
		}
		if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status); err != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w; save study map status: %v", returnErr, err)
			} else {
				returnErr = fmt.Errorf("study map: save status: %w", err)
			}
		}
	}()
	if ctx == nil || provider == nil {
		return status, fmt.Errorf("study map: context and provider are required")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if err := os.Remove(filepath.Join(runDir, studymap.RecordFile)); err != nil && !os.IsNotExist(err) {
		return status, fmt.Errorf("study map: remove stale record: %w", err)
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return status, fmt.Errorf("study map: read saved run: %w", err)
	}
	bundle, err := buildStudyMapBundle(runDir, repoRoot, data)
	if err != nil {
		return status, err
	}
	status.Anchors = len(bundle.Anchors)
	status.Areas = len(bundle.Areas)
	status.Documents = len(bundle.Documents)
	status.Mechanisms = len(bundle.Mechanisms)
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.BundleFile), bundle); err != nil {
		return status, fmt.Errorf("study map: save bundle: %w", err)
	}
	promptBundle, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		return status, fmt.Errorf("study map: encode provider bundle: %w", err)
	}
	prompt := semanticdiscovery.Prompt{
		Version:         semanticdiscovery.StudyMapPromptVersion,
		System:          studyMapSystemPrompt,
		User:            studyMapUserPrompt + string(promptBundle),
		ThinkingProfile: semanticdiscovery.ThinkingMax,
		ProgressLabel:   "repository study map editing",
	}
	plan, err := newSemanticDiscoveryStagePlan(provider, prompt, "repository_study_map")
	if err != nil {
		return status, err
	}
	metrics := semanticDiscoveryStageMetrics{
		Stage: plan.name, PromptVersion: plan.prompt.Version, RequestBytes: len(plan.request),
		ProviderCall: true,
	}
	attempt := studyMapAttempt{
		Version: 1, PromptVersion: prompt.Version, BundleSHA256: bundleSHA,
		ValidationState: "started", Metrics: metrics,
	}
	callStarted := time.Now()
	providerResult, callErr := provider.DiscoverSemanticsMeasured(ctx, prompt)
	metrics.addResponse(providerResult, time.Since(callStarted))
	status.Metrics = metrics
	attempt.Metrics = metrics
	if ctxErr := ctx.Err(); ctxErr != nil {
		attempt.ValidationState = "canceled"
		attempt.FailureReason = semanticDiscoveryReason(ctxErr.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		return status, ctxErr
	}
	if callErr != nil {
		metrics.Status = "failed_provider"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(callErr.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		return status, fmt.Errorf("study map: provider call: %w", callErr)
	}
	if json.Valid(providerResult.Content) {
		attempt.Response = append(json.RawMessage(nil), providerResult.Content...)
	} else {
		attempt.RawResponse = string(providerResult.Content)
	}
	proposal, err := studymap.DecodeProposal(providerResult.Content)
	if err != nil {
		metrics.Status = "rejected"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(err.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		return status, err
	}
	record, err := studymap.BuildRecord(bundle, proposal)
	if err != nil {
		metrics.Status = "rejected"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(err.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		return status, err
	}
	metrics.Status = "accepted"
	status.Metrics = metrics
	status.RepositoryType = record.RepositoryType
	status.Candidates = record.Reduction.Proposed
	status.Validated = record.Reduction.Validated
	status.Selected = len(record.Directions)
	status.State = "published"
	attempt.Metrics = metrics
	attempt.ValidationState = metrics.Status
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt); err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.RecordFile), record); err != nil {
		return status, fmt.Errorf("study map: save canonical record: %w", err)
	}
	return status, nil
}

func buildStudyMapBundle(
	runDir string,
	repoRoot string,
	data *report.ReportData,
) (studymap.Bundle, error) {
	if data == nil {
		return studymap.Bundle{}, fmt.Errorf("study map: report data is required")
	}
	savedSources, _, _, savedErr := freshSourceFunctions(runDir, repoRoot)
	if savedErr != nil {
		savedSources = studyMapRecoverSavedSourceFunctions(runDir, repoRoot)
	}
	centralSources, _, centralErr := freshCentralSourceFunctions(runDir, repoRoot, data)
	var exactSources []freshSourceFunction
	if len(savedSources) == 0 && len(centralSources) == 0 {
		exactSources = freshSavedDiscoverySources(runDir, data, nil)
	}
	diverseSources := studyMapDiverseSourceFunctions(repoRoot, data, append(savedSources, centralSources...))
	sources := mergeFreshSourceFunctions(
		savedSources,
		append(centralSources, diverseSources...),
		studyMapMaxSourceFunctions,
	)
	sources = studyMapReserveMechanismSources(
		sources,
		append(append([]freshSourceFunction(nil), savedSources...), append(centralSources, diverseSources...)...),
		data.UserMechanisms,
		studyMapMaxSourceFunctions,
	)
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		if filePath = strings.TrimSpace(filePath); filePath != "" {
			openable[filePath] = struct{}{}
		}
	}
	areas, areaPaths := studyMapAreas(data, openable)
	anchors := make([]studymap.Anchor, 0, len(sources)+len(exactSources))
	for _, source := range sources {
		if _, ok := openable[source.Function.Path]; !ok {
			continue
		}
		line := source.Function.StartLine
		if len(source.Fact.Evidence) > 0 && source.Fact.Evidence[0].Line > 0 {
			line = source.Fact.Evidence[0].Line
		}
		areaIDs := studyMapAreasForPath(source.Function.Path, areas, areaPaths)
		anchors = append(anchors, studymap.Anchor{
			ID: source.Fact.ID, Path: source.Function.Path, Symbol: source.Function.Symbol,
			Line: line, Role: artifactrole.Classify(source.Function.Path, artifactrole.Hints{}),
			Statement:    source.Fact.Statement,
			Capabilities: append([]semanticdiscovery.Capability(nil), source.Fact.Capabilities...),
			AreaIDs:      areaIDs, Function: source.Function,
		})
	}
	if len(sources) == 0 {
		for _, source := range exactSources {
			language := studyMapExactSourceLanguage(source.Function.Path)
			if language == "" {
				continue
			}
			if _, ok := openable[source.Function.Path]; !ok {
				continue
			}
			line := source.Function.StartLine
			if len(source.Fact.Evidence) > 0 && source.Fact.Evidence[0].Line > 0 {
				line = source.Fact.Evidence[0].Line
			}
			exact := &studymap.ExactSource{
				Path: source.Function.Path, Language: language, Symbol: source.Function.Symbol,
				Line: line, StartLine: source.Function.StartLine, EndLine: source.Function.EndLine,
				Lines: append([]string(nil), source.Function.Lines...), ContentSHA256: source.Function.ContentSHA256,
			}
			anchors = append(anchors, studymap.Anchor{
				ID: source.Fact.ID, Path: exact.Path, Symbol: exact.Symbol, Line: exact.Line,
				Role:      artifactrole.Classify(exact.Path, artifactrole.Hints{}),
				Statement: source.Fact.Statement, Capabilities: append(
					[]semanticdiscovery.Capability(nil), source.Fact.Capabilities...,
				),
				AreaIDs: studyMapAreasForPath(exact.Path, areas, areaPaths), ExactSource: exact,
			})
		}
	}
	if len(anchors) == 0 {
		if centralErr != nil && len(sources) == 0 && len(exactSources) == 0 {
			return studymap.Bundle{}, fmt.Errorf("study map: collect central source: %w", centralErr)
		}
		savedSourceUnavailable := errors.Is(savedErr, os.ErrNotExist) ||
			errors.Is(savedErr, errFreshNoUsableSourceWindows)
		if savedErr != nil && !savedSourceUnavailable &&
			len(sources) == 0 && len(exactSources) == 0 {
			return studymap.Bundle{}, fmt.Errorf("study map: no bounded source functions: %w", savedErr)
		}
		return studymap.Bundle{}, unavailableStudyMapSourceOutcome(data)
	}
	areas, areaPaths = ensureStudyMapAreas(areas, areaPaths, anchors)
	for index := range anchors {
		anchors[index].AreaIDs = studyMapAreasForPath(anchors[index].Path, areas, areaPaths)
	}
	documents := studyMapDocuments(repoRoot, data, openable)
	mechanisms := studyMapMechanisms(data, anchors, openable)
	allowedPaths := studyMapAllowedPaths(anchors, areas, documents, mechanisms)
	bundle := studymap.Bundle{
		Version: studymap.BundleVersion, RepoName: data.RepoName,
		DocumentedPurpose:  strings.TrimSpace(data.DocumentedPurpose),
		OrientationSummary: strings.TrimSpace(data.ProjectGuess),
		RepositoryTypeHint: studyMapRepositoryType(data),
		Areas:              areas, Anchors: anchors, Documents: documents, Mechanisms: mechanisms,
		AllowedPaths: allowedPaths,
	}
	for _, term := range data.ImportantDomainWords {
		if strings.TrimSpace(term.Word) == "" || strings.TrimSpace(term.Guess) == "" {
			continue
		}
		bundle.DomainTerms = append(bundle.DomainTerms, studymap.DomainTerm{
			Term: strings.TrimSpace(term.Word), Explanation: strings.TrimSpace(term.Guess),
		})
		if len(bundle.DomainTerms) == 12 {
			break
		}
	}
	if _, err := studymap.BundleHash(bundle); err != nil {
		return studymap.Bundle{}, err
	}
	return bundle, nil
}

func studyMapExactSourceLanguage(sourcePath string) string {
	switch strings.ToLower(path.Ext(sourcePath)) {
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return ""
	}
}

func studyMapAllowedPaths(
	anchors []studymap.Anchor,
	areas []studymap.Area,
	documents []studymap.Document,
	mechanisms []studymap.Mechanism,
) []string {
	allowedPaths := make([]string, 0, len(anchors)+len(areas)+len(documents))
	for _, anchor := range anchors {
		allowedPaths = append(allowedPaths, anchor.Path)
	}
	for _, area := range areas {
		if area.Path != "" {
			allowedPaths = append(allowedPaths, area.Path)
		}
	}
	for _, document := range documents {
		allowedPaths = append(allowedPaths, document.Path)
	}
	for _, mechanism := range mechanisms {
		allowedPaths = append(allowedPaths, mechanism.Paths...)
	}
	return uniqueStudyStrings(allowedPaths)
}

// studyMapRecoverSavedSourceFunctions keeps one syntactically incomplete
// bounded window from canceling otherwise independent exact windows. It never
// repairs or extends a window: only functions that still pass the existing
// sourcewindowfacts and local fact validators are retained.
func studyMapRecoverSavedSourceFunctions(runDir, repoRoot string) []freshSourceFunction {
	windows, err := sourcewindowfacts.LoadRunForDiscovery(runDir, repoRoot)
	if err != nil {
		return nil
	}
	best := make(map[string]sourcewindowfacts.Function)
	for _, window := range windows {
		functions, extractErr := sourcewindowfacts.ExtractGoFunctions(window)
		if extractErr != nil {
			continue
		}
		for _, function := range functions {
			if len(freshSubstantiveWindowObservations(function.Observations)) == 0 {
				continue
			}
			key := function.Path + "\x00" + function.Symbol
			previous, exists := best[key]
			if !exists || freshWindowFunctionBetter(function, previous) {
				best[key] = function
			}
		}
	}
	ranked := make([]sourcewindowfacts.Function, 0, len(best))
	for _, function := range best {
		ranked = append(ranked, function)
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := freshWindowFunctionScore(ranked[i]), freshWindowFunctionScore(ranked[j])
		if left != right {
			return left > right
		}
		if ranked[i].Path != ranked[j].Path {
			return ranked[i].Path < ranked[j].Path
		}
		return ranked[i].Symbol < ranked[j].Symbol
	})
	result := make([]freshSourceFunction, 0, min(studyMapMaxSourceFunctions, len(ranked)))
	for _, function := range ranked {
		fact, buildErr := freshWindowFunctionFact(function)
		if buildErr != nil {
			continue
		}
		result = append(result, freshSourceFunction{Function: function, Fact: fact})
		if len(result) == studyMapMaxSourceFunctions {
			break
		}
	}
	return result
}

// studyMapDiverseSourceFunctions fills uncovered production packages from the
// already bounded report package/file landscape. It reads at most the existing
// eight-file/256 KiB onboarding budget and performs no call graph or package
// enumeration.
func studyMapDiverseSourceFunctions(
	repoRoot string,
	data *report.ReportData,
	existing []freshSourceFunction,
) []freshSourceFunction {
	paths := studyMapDiversePackagePaths(data, existing, freshRepoOnboardingMaxAnchorFiles)
	if len(paths) == 0 {
		return nil
	}
	files, _, err := freshParseSelectedFiles(repoRoot, data, paths, nil, 0)
	if err != nil {
		return nil
	}
	known := make(map[string]struct{}, len(existing))
	for _, source := range existing {
		known[source.Function.Path+"\x00"+source.Function.Symbol] = struct{}{}
	}
	byPath := make(map[string][]freshRankedFunction, len(files))
	mechanismLines := studyMapMechanismLines(data.UserMechanisms)
	for _, file := range files {
		for _, declaration := range file.File.Decls {
			functionDeclaration, ok := declaration.(*ast.FuncDecl)
			if !ok || functionDeclaration.Body == nil {
				continue
			}
			function, buildErr := freshFunctionFromDeclaration(file, functionDeclaration)
			if buildErr != nil || len(freshSubstantiveWindowObservations(function.Observations)) == 0 {
				continue
			}
			key := function.Path + "\x00" + function.Symbol
			if _, duplicate := known[key]; duplicate {
				continue
			}
			score := freshOnboardingFunctionScore(function, data)
			if studyMapFunctionContainsAnyLine(function, mechanismLines[function.Path]) {
				score += 10_000
			}
			byPath[file.Path] = append(byPath[file.Path], freshRankedFunction{
				Function: function,
				Score:    score,
			})
		}
	}
	for filePath := range byPath {
		sort.Slice(byPath[filePath], func(i, j int) bool {
			if byPath[filePath][i].Score != byPath[filePath][j].Score {
				return byPath[filePath][i].Score > byPath[filePath][j].Score
			}
			return byPath[filePath][i].Function.Symbol < byPath[filePath][j].Function.Symbol
		})
	}
	var ranked []freshRankedFunction
	for depth := 0; len(ranked) < freshRepoOnboardingMaxAnchorFuncs; depth++ {
		added := false
		for _, filePath := range paths {
			if depth >= len(byPath[filePath]) {
				continue
			}
			ranked = append(ranked, byPath[filePath][depth])
			added = true
			if len(ranked) == freshRepoOnboardingMaxAnchorFuncs {
				break
			}
		}
		if !added {
			break
		}
	}
	result := make([]freshSourceFunction, 0, len(ranked))
	for _, item := range ranked {
		fact, buildErr := freshWindowFunctionFact(item.Function)
		if buildErr == nil {
			result = append(result, freshSourceFunction{Function: item.Function, Fact: fact})
		}
	}
	return result
}

func studyMapDiversePackagePaths(
	data *report.ReportData,
	existing []freshSourceFunction,
	limit int,
) []string {
	if data == nil || limit <= 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		allowed[filePath] = struct{}{}
	}
	seen := make(map[string]struct{}, len(existing))
	mechanismLines := studyMapMechanismLines(data.UserMechanisms)
	for _, source := range existing {
		if !studyMapFunctionContainsAnyLine(source.Function, mechanismLines[source.Function.Path]) {
			seen[source.Function.Path] = struct{}{}
		}
	}
	result := make([]string, 0, limit)
	for _, mechanism := range data.UserMechanisms {
		for _, location := range mechanism.Files {
			if len(result) == limit || len(result) == 4 {
				break
			}
			filePath := strings.TrimSpace(location.Path)
			if _, ok := allowed[filePath]; !ok || filepath.Ext(filePath) != ".go" ||
				strings.HasSuffix(filePath, "_test.go") ||
				!artifactrole.IsProduction(artifactrole.Classify(filePath, artifactrole.Hints{})) {
				continue
			}
			if _, duplicate := seen[filePath]; duplicate {
				continue
			}
			result = append(result, filePath)
			seen[filePath] = struct{}{}
		}
		if len(result) == limit || len(result) == 4 {
			break
		}
	}
	type packageFiles struct {
		directory string
		paths     []string
	}
	packageCount := 0
	if data.RepositoryGraph != nil {
		packageCount = len(data.RepositoryGraph.Packages)
	}
	packages := make([]packageFiles, 0, packageCount)
	var repositoryPackages []report.PackageInfo
	if data.RepositoryGraph != nil {
		repositoryPackages = data.RepositoryGraph.Packages
	}
	for _, repositoryPackage := range repositoryPackages {
		var paths []string
		for _, filePath := range repositoryPackage.Files {
			if _, ok := allowed[filePath]; !ok || filepath.Ext(filePath) != ".go" ||
				strings.HasSuffix(filePath, "_test.go") {
				continue
			}
			if _, duplicate := seen[filePath]; duplicate {
				continue
			}
			role := artifactrole.Classify(filePath, artifactrole.Hints{})
			if !artifactrole.IsProduction(role) {
				continue
			}
			paths = append(paths, filePath)
		}
		if len(paths) == 0 {
			continue
		}
		packages = append(packages, packageFiles{
			directory: repositoryPackage.Dir,
			paths:     artifactrole.SortPaths(paths),
		})
	}
	sort.SliceStable(packages, func(i, j int) bool {
		leftDepth := strings.Count(strings.Trim(packages[i].directory, "/"), "/")
		rightDepth := strings.Count(strings.Trim(packages[j].directory, "/"), "/")
		if packages[i].directory == "." {
			leftDepth = -1
		}
		if packages[j].directory == "." {
			rightDepth = -1
		}
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return packages[i].directory < packages[j].directory
	})
	for index := 0; len(result) < limit; index++ {
		added := false
		for packageIndex := range packages {
			if index >= len(packages[packageIndex].paths) {
				continue
			}
			filePath := packages[packageIndex].paths[index]
			result = append(result, filePath)
			seen[filePath] = struct{}{}
			added = true
			if len(result) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return result
}

func studyMapMechanismLines(mechanisms []report.UserMechanism) map[string][]int {
	result := make(map[string][]int)
	for _, mechanism := range mechanisms {
		for _, location := range mechanism.Files {
			if location.Path != "" && location.Line > 0 {
				result[location.Path] = append(result[location.Path], location.Line)
			}
		}
		for _, step := range mechanism.Steps {
			for _, location := range step.Locations {
				if location.Path != "" && location.Line > 0 {
					result[location.Path] = append(result[location.Path], location.Line)
				}
			}
		}
	}
	return result
}

func studyMapFunctionContainsAnyLine(function sourcewindowfacts.Function, lines []int) bool {
	for _, line := range lines {
		if line >= function.StartLine && line <= function.EndLine {
			return true
		}
	}
	return false
}

func studyMapReserveMechanismSources(
	selected []freshSourceFunction,
	available []freshSourceFunction,
	mechanisms []report.UserMechanism,
	limit int,
) []freshSourceFunction {
	if len(mechanisms) == 0 || limit <= 0 {
		return selected
	}
	required := make([]freshSourceFunction, 0, len(mechanisms))
	requiredKeys := make(map[string]struct{})
	for _, mechanism := range mechanisms {
		mechanismLines := studyMapMechanismLines([]report.UserMechanism{mechanism})
		for _, source := range available {
			if !studyMapFunctionContainsAnyLine(source.Function, mechanismLines[source.Function.Path]) {
				continue
			}
			key := source.Function.Path + "\x00" + source.Function.Symbol
			if _, duplicate := requiredKeys[key]; !duplicate {
				required = append(required, source)
				requiredKeys[key] = struct{}{}
			}
			break
		}
	}
	if len(required) == 0 {
		return selected
	}
	result := make([]freshSourceFunction, 0, min(limit, len(selected)+len(required)))
	seen := make(map[string]struct{}, limit)
	appendSource := func(source freshSourceFunction) {
		if len(result) == limit {
			return
		}
		key := source.Function.Path + "\x00" + source.Function.Symbol
		if _, duplicate := seen[key]; duplicate {
			return
		}
		result = append(result, source)
		seen[key] = struct{}{}
	}
	for _, source := range required {
		appendSource(source)
	}
	for _, source := range selected {
		appendSource(source)
	}
	return result
}

func studyMapAreas(
	data *report.ReportData,
	openable map[string]struct{},
) ([]studymap.Area, map[string][]string) {
	areas := make([]studymap.Area, 0, studymap.MaxAreas)
	paths := make(map[string][]string)
	seenIDs := make(map[string]struct{})
	seenPaths := make(map[string]struct{})
	seenNames := make(map[string]struct{})
	addArea := func(area studymap.Area, areaPaths []string) {
		if len(areas) == studymap.MaxAreas || area.ID == "" || strings.TrimSpace(area.Name) == "" {
			return
		}
		nameKey := strings.ToLower(strings.TrimSpace(area.Name))
		if _, duplicate := seenIDs[area.ID]; duplicate {
			return
		}
		if _, duplicate := seenNames[nameKey]; duplicate {
			return
		}
		areaPaths = uniqueStudyStrings(areaPaths)
		if area.Path != "" {
			if _, duplicate := seenPaths[area.Path]; duplicate {
				return
			}
		}
		seenIDs[area.ID] = struct{}{}
		seenNames[nameKey] = struct{}{}
		if area.Path != "" {
			seenPaths[area.Path] = struct{}{}
		}
		areas = append(areas, area)
		paths[area.ID] = areaPaths
	}

	components := append([]report.Component(nil), data.Components...)
	sort.SliceStable(components, func(i, j int) bool {
		left := studyMapReportComponentPaths(components[i], openable)
		right := studyMapReportComponentPaths(components[j], openable)
		if len(left) == 0 || len(right) == 0 {
			return len(left) > len(right)
		}
		leftProduction := studyMapPathProduction(left[0])
		rightProduction := studyMapPathProduction(right[0])
		if leftProduction != rightProduction {
			return leftProduction
		}
		return components[i].ID < components[j].ID
	})
	for _, component := range components {
		componentPaths := studyMapReportComponentPaths(component, openable)
		if len(componentPaths) == 0 || !studyMapPathProduction(componentPaths[0]) {
			continue
		}
		responsibility := strings.TrimSpace(component.ModelPurpose)
		if responsibility == "" {
			responsibility = "Production code grouped around " + component.Name + "."
		}
		addArea(studymap.Area{
			ID: component.ID, Name: component.Name, Responsibility: responsibility,
			Path: componentPaths[0],
		}, componentPaths)
	}

	if data.RepositoryGraph != nil {
		packages := append([]report.PackageInfo(nil), data.RepositoryGraph.Packages...)
		sort.SliceStable(packages, func(i, j int) bool {
			leftDepth := strings.Count(strings.Trim(packages[i].Dir, "/"), "/")
			rightDepth := strings.Count(strings.Trim(packages[j].Dir, "/"), "/")
			if packages[i].Dir == "." {
				leftDepth = -1
			}
			if packages[j].Dir == "." {
				rightDepth = -1
			}
			if leftDepth != rightDepth {
				return leftDepth < rightDepth
			}
			return packages[i].CanonicalPath < packages[j].CanonicalPath
		})
		for _, repositoryPackage := range packages {
			var packagePaths []string
			for _, filePath := range repositoryPackage.Files {
				if _, ok := openable[filePath]; !ok || filepath.Ext(filePath) != ".go" ||
					strings.HasSuffix(filePath, "_test.go") || !studyMapPathProduction(filePath) {
					continue
				}
				packagePaths = append(packagePaths, filePath)
			}
			packagePaths = artifactrole.SortPaths(packagePaths)
			selectedPath := ""
			for _, filePath := range packagePaths {
				if _, duplicate := seenPaths[filePath]; !duplicate {
					selectedPath = filePath
					break
				}
			}
			if selectedPath == "" {
				continue
			}
			name := repositoryPackage.DisplayPath
			if name == "" || name == "." {
				name = repositoryPackage.Name
			}
			name = studyMapHumanName(name)
			responsibility := "Production package " + repositoryPackage.CanonicalPath + "."
			addArea(studymap.Area{
				ID:   studyMapOpaqueID("package-area", repositoryPackage.CanonicalPath),
				Name: name, Responsibility: responsibility,
				Path: selectedPath,
			}, packagePaths)
		}
	}

	if data.ArchitectureCanvas != nil {
		components := append([]report.ArchitectureComponent(nil), data.ArchitectureCanvas.Components...)
		sort.SliceStable(components, func(i, j int) bool {
			left := studyMapComponentPath(components[i], openable)
			right := studyMapComponentPath(components[j], openable)
			leftProduction := studyMapPathProduction(left)
			rightProduction := studyMapPathProduction(right)
			if leftProduction != rightProduction {
				return leftProduction
			}
			if components[i].Hypothesis != components[j].Hypothesis {
				return !components[i].Hypothesis
			}
			return components[i].ID < components[j].ID
		})
		for _, component := range components {
			if len(areas) == studymap.MaxAreas {
				break
			}
			if studyMapPeripheralArea(component.Name + " " + component.Description) {
				continue
			}
			filePath := studyMapComponentPath(component, openable)
			if filePath == "" && !studyMapCanvasComponentRelated(component, data.Components) {
				continue
			}
			area := studymap.Area{
				ID: string(component.ID), Name: strings.TrimSpace(component.Name),
				Responsibility: strings.TrimSpace(component.Description),
				ComponentID:    string(component.ID), Path: filePath,
			}
			if area.Name == "" {
				continue
			}
			if area.Responsibility == "" {
				area.Responsibility = "Code grouped around " + area.Name + "."
			}
			if filePath != "" {
				area.Line = studyMapComponentLine(component, filePath)
			}
			addArea(area, studyMapComponentPaths(component, openable))
		}
	}
	return areas, paths
}

func studyMapReportComponentPaths(
	component report.Component,
	openable map[string]struct{},
) []string {
	var result []string
	for _, group := range component.AnchorGroups {
		if _, ok := openable[group.Path]; ok && studyMapPathProduction(group.Path) {
			result = append(result, group.Path)
		}
	}
	return artifactrole.SortPaths(uniqueStudyStrings(result))
}

func studyMapPeripheralArea(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{
		"example", "demo", "fixture", "playground", "preview", "evaluator", "benchmark", "test harness",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func studyMapCanvasComponentRelated(
	component report.ArchitectureComponent,
	components []report.Component,
) bool {
	canvasTerms := freshTerms(component.Name + " " + component.Description)
	for _, candidate := range components {
		overlap := 0
		for term := range freshTerms(candidate.Name + " " + candidate.ModelPurpose) {
			if _, ok := canvasTerms[term]; ok {
				overlap++
			}
		}
		if overlap > 0 {
			return true
		}
	}
	return false
}

func studyMapComponentPath(
	component report.ArchitectureComponent,
	openable map[string]struct{},
) string {
	paths := studyMapComponentPaths(component, openable)
	if len(paths) == 0 {
		return ""
	}
	return artifactrole.SortPaths(paths)[0]
}

func studyMapComponentPaths(
	component report.ArchitectureComponent,
	openable map[string]struct{},
) []string {
	var result []string
	for _, member := range component.Members {
		for _, fact := range member.Facts {
			if fact.Location != nil {
				if _, ok := openable[fact.Location.Path]; ok {
					result = append(result, fact.Location.Path)
				}
			}
		}
		if member.ID.Kind == componentmap.MemberFile || member.ID.Kind == componentmap.MemberEntrypoint {
			if _, ok := openable[member.ID.Value]; ok {
				result = append(result, member.ID.Value)
			}
		}
	}
	return uniqueStudyStrings(result)
}

func studyMapComponentLine(component report.ArchitectureComponent, filePath string) int {
	for _, member := range component.Members {
		for _, fact := range member.Facts {
			if fact.Location != nil && fact.Location.Path == filePath && fact.Location.Line > 0 {
				return fact.Location.Line
			}
		}
	}
	return 0
}

func ensureStudyMapAreas(
	areas []studymap.Area,
	areaPaths map[string][]string,
	anchors []studymap.Anchor,
) ([]studymap.Area, map[string][]string) {
	seen := make(map[string]struct{}, len(areas))
	for _, area := range areas {
		seen[area.ID] = struct{}{}
	}
	for _, anchor := range anchors {
		if len(areas) >= 1 {
			break
		}
		directory := path.Dir(anchor.Path)
		if directory == "." {
			directory = path.Base(anchor.Path)
		}
		id := studyMapOpaqueID("area", directory)
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(anchor.Symbol)
		if name == "" {
			name = studyMapHumanName(directory)
		}
		responsibility := "Exact code area for inspecting behavior around " + name + "."
		areas = append(areas, studymap.Area{
			ID: id, Name: name,
			Responsibility: responsibility,
			Path:           anchor.Path, Line: anchor.Line,
		})
		areaPaths[id] = []string{anchor.Path}
	}
	return areas, areaPaths
}

func studyMapAreasForPath(
	filePath string,
	areas []studymap.Area,
	areaPaths map[string][]string,
) []string {
	var result []string
	for _, area := range areas {
		for _, areaPath := range areaPaths[area.ID] {
			areaDir := path.Dir(areaPath)
			if filePath == areaPath || (areaDir != "." && strings.HasPrefix(filePath, areaDir+"/")) {
				result = append(result, area.ID)
				break
			}
		}
	}
	if len(result) == 0 && len(areas) > 0 {
		best := -1
		bestPrefix := -1
		for index, area := range areas {
			for _, areaPath := range areaPaths[area.ID] {
				prefix := commonStudyPathPrefix(filePath, areaPath)
				if prefix > bestPrefix {
					best, bestPrefix = index, prefix
				}
			}
		}
		if best >= 0 {
			result = append(result, areas[best].ID)
		}
	}
	return uniqueStudyStrings(result)
}

func studyMapDocuments(
	repoRoot string,
	data *report.ReportData,
	openable map[string]struct{},
) []studymap.Document {
	excerpts := make(map[string]string)
	if data.ModelResearch != nil {
		for _, item := range data.ModelResearch.Theory.GroundedFacts {
			if item.Kind != modelresearch.EvidenceSource || item.Location == nil || item.Window == nil ||
				!studyMapDocumentPath(item.Location.Path) {
				continue
			}
			excerpt := truncateStudyText(
				strings.TrimSpace(strings.Join(item.Window.Lines, "\n")),
				studyMapMaxDocumentExcerptBytes,
			)
			if excerpt != "" && excerpts[item.Location.Path] == "" {
				excerpts[item.Location.Path] = excerpt
			}
		}
	}
	var paths []string
	for filePath := range openable {
		if studyMapDocumentPath(filePath) {
			paths = append(paths, filePath)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		leftREADME := strings.HasPrefix(strings.ToLower(path.Base(paths[i])), "readme")
		rightREADME := strings.HasPrefix(strings.ToLower(path.Base(paths[j])), "readme")
		if leftREADME != rightREADME {
			return leftREADME
		}
		leftRole := artifactrole.Classify(paths[i], artifactrole.Hints{Documentation: true})
		rightRole := artifactrole.Classify(paths[j], artifactrole.Hints{Documentation: true})
		if artifactrole.SelectionPriority(leftRole) != artifactrole.SelectionPriority(rightRole) {
			return artifactrole.SelectionPriority(leftRole) > artifactrole.SelectionPriority(rightRole)
		}
		return paths[i] < paths[j]
	})
	if len(paths) > studymap.MaxDocuments {
		paths = paths[:studymap.MaxDocuments]
	}
	result := make([]studymap.Document, 0, len(paths))
	excerptBytes := 0
	excerptLimit := studyMapDocumentExcerptLimit(len(paths))
	for _, filePath := range paths {
		excerpt := excerpts[filePath]
		remaining := studyMapMaxDocumentExcerptTotal - excerptBytes
		if remaining > 0 {
			if excerpt == "" {
				excerpt = readStudyMapDocumentExcerpt(repoRoot, filePath)
			}
			excerpt = truncateStudyText(excerpt, min(excerptLimit, remaining))
			excerptBytes += len(excerpt)
		} else {
			excerpt = ""
		}
		result = append(result, studymap.Document{
			ID: studyMapOpaqueID("doc", filePath), Path: filePath,
			Label: studyMapDocumentLabel(filePath), Excerpt: excerpt,
		})
	}
	return result
}

func studyMapDocumentExcerptLimit(documentCount int) int {
	if documentCount <= 0 {
		return 0
	}
	fairShare := studyMapMaxDocumentExcerptTotal / documentCount
	if fairShare <= 0 {
		fairShare = 1
	}
	return min(studyMapMaxDocumentExcerptBytes, fairShare)
}

func readStudyMapDocumentExcerpt(repoRoot, filePath string) string {
	resolved, err := resolveFreshRepoPath(repoRoot, filePath)
	if err != nil {
		return ""
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return ""
	}
	file, err := os.Open(resolved)
	if err != nil {
		return ""
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, studyMapMaxDocumentReadBytes))
	if err != nil {
		return ""
	}
	return truncateStudyText(strings.TrimSpace(string(raw)), studyMapMaxDocumentExcerptBytes)
}

func studyMapMechanisms(
	data *report.ReportData,
	anchors []studymap.Anchor,
	openable map[string]struct{},
) []studymap.Mechanism {
	result := make([]studymap.Mechanism, 0, min(studymap.MaxMechanisms, len(data.UserMechanisms)))
	for _, mechanism := range data.UserMechanisms {
		mechanismLines := studyMapMechanismLines([]report.UserMechanism{mechanism})
		paths := make([]string, 0, len(mechanism.Files))
		for _, location := range mechanism.Files {
			if _, ok := openable[location.Path]; ok {
				paths = append(paths, location.Path)
			}
		}
		paths = uniqueStudyStrings(paths)
		var anchorIDs []string
		for _, anchor := range anchors {
			if studyMapFunctionContainsAnyLine(anchor.Function, mechanismLines[anchor.Path]) {
				anchorIDs = append(anchorIDs, anchor.ID)
			}
		}
		if len(anchorIDs) == 0 {
			continue
		}
		result = append(result, studymap.Mechanism{
			ID: mechanism.ArtifactID, Question: mechanism.Question,
			Title: mechanism.Title, AnchorIDs: uniqueStudyStrings(anchorIDs), Paths: paths,
		})
		if len(result) == studymap.MaxMechanisms {
			break
		}
	}
	return result
}

func studyMapRepositoryType(data *report.ReportData) studymap.RepositoryType {
	if data.ArchitectureCanvas != nil {
		switch data.ArchitectureCanvas.RepositoryArchetype {
		case componentmap.ArchetypeLibraryFramework:
			return studymap.RepositoryLibrary
		case componentmap.ArchetypeCLITool:
			return studymap.RepositoryCLI
		case componentmap.ArchetypeMonorepoMixed:
			return studymap.RepositoryMonorepo
		case componentmap.ArchetypeApplication, componentmap.ArchetypeDaemonWorkerSystem:
			return studymap.RepositoryService
		case componentmap.ArchetypeModularPlatformServer:
			return studymap.RepositoryMixed
		}
	}
	mainFiles := 0
	for _, filePath := range data.OpenablePaths {
		if path.Base(filePath) == "main.go" {
			mainFiles++
		}
	}
	if mainFiles == 1 {
		return studymap.RepositoryCLI
	}
	if mainFiles > 1 {
		return studymap.RepositoryMonorepo
	}
	return studymap.RepositoryLibrary
}

func studyMapDocumentPath(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	ext := strings.ToLower(path.Ext(base))
	return strings.HasPrefix(base, "readme") || ext == ".md" || ext == ".mdx" || ext == ".rst"
}

func studyMapDocumentLabel(filePath string) string {
	base := path.Base(filePath)
	if strings.HasPrefix(strings.ToLower(base), "readme") {
		return "README"
	}
	return studyMapHumanName(strings.TrimSuffix(base, path.Ext(base)))
}

func studyMapHumanName(value string) string {
	value = strings.Trim(value, "/._-")
	words := strings.FieldsFunc(value, func(char rune) bool { return char == '/' || char == '_' || char == '-' || char == '.' })
	for index, word := range words {
		if word == "" {
			continue
		}
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[index] = string(runes)
	}
	if len(words) == 0 {
		return "Repository code"
	}
	return strings.Join(words, " ")
}

func studyMapPathProduction(filePath string) bool {
	if filePath == "" {
		return false
	}
	return artifactrole.IsProduction(artifactrole.Classify(filePath, artifactrole.Hints{}))
}

func commonStudyPathPrefix(left, right string) int {
	leftParts, rightParts := strings.Split(left, "/"), strings.Split(right, "/")
	count := 0
	for count < len(leftParts) && count < len(rightParts) && leftParts[count] == rightParts[count] {
		count++
	}
	return count
}

func studyMapOpaqueID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func uniqueStudyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func truncateStudyText(value string, byteLimit int) string {
	if len(value) <= byteLimit {
		return value
	}
	for byteLimit > 0 && !utf8.RuneStart(value[byteLimit]) {
		byteLimit--
	}
	return strings.TrimSpace(value[:byteLimit])
}
