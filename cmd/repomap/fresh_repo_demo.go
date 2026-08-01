package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

const (
	freshRepoDemoStatusFile     = "fresh_repo_demo_status.json"
	freshRepoDemoCandidatesFile = "fresh_repo_candidates.json"
	freshRepoOpportunityFile    = "fresh_repo_opportunity_attempt.json"
	freshRepoOpportunityReplay  = "fresh_repo_opportunity_replay.json"
	freshRepoDemoFactsFile      = "fresh_repo_source_facts.json"
	freshRepoDemoAttemptsDir    = "fresh_mechanisms"
	freshRepoDemoMaxFactOps     = 4
	freshRepoDemoMaxQuestions   = 3
	freshRepoDemoMaxCandidates  = 3
	freshRepoDemoMaxSourceFacts = 16
	freshRepoDemoMaxProbeFacts  = 5
	freshRepoDemoMaxSeedFuncs   = 4
	freshRepoDemoMaxFrontier    = 2
	freshRepoDemoMaxParsedBytes = 128 << 10
	// The existing generic probe core has a stricter six-file ceiling. The
	// candidate-specific planner may inspect at most eight bounded local files,
	// while every probe handed to the unchanged core remains within six.
	freshRepoDemoMaxProbeFiles = 6
)

var errFreshNoUsableSourceWindows = errors.New(
	"fresh repository mechanism: no usable non-truncated saved source windows",
)

var (
	errFreshPrimaryEvidenceInsufficient = errors.New(
		"fresh repository mechanism: insufficient primary evidence",
	)
	errFreshPrimaryMechanismPreserved = errors.New(
		"fresh repository mechanism: existing accepted candidate preserved",
	)
)

type freshRepoDemoResult struct {
	Status freshRepoDemoStatus
}

type freshRepoDemoStatus struct {
	Version              int                             `json:"version"`
	State                string                          `json:"state"`
	FailureReason        string                          `json:"failure_reason,omitempty"`
	SourceWindows        int                             `json:"source_windows"`
	SourceFunctions      int                             `json:"source_functions"`
	CentralFunctions     int                             `json:"central_source_functions,omitempty"`
	CentralParsedBytes   int                             `json:"central_parsed_bytes,omitempty"`
	SourceFacts          int                             `json:"source_facts"`
	QuestionsProposed    int                             `json:"questions_proposed"`
	CandidatesSelected   int                             `json:"candidates_selected"`
	Opportunity          *semanticDiscoveryStageMetrics  `json:"opportunity,omitempty"`
	OnboardingEditor     *semanticDiscoveryStageMetrics  `json:"onboarding_editor,omitempty"`
	OnboardingEditorFail string                          `json:"onboarding_editor_failure,omitempty"`
	Attempts             []freshRepoCandidateAttempt     `json:"attempts,omitempty"`
	PublishedCandidateID string                          `json:"published_candidate_id,omitempty"`
	PublishedMechanismID string                          `json:"published_mechanism_id,omitempty"`
	PublishedArtifact    *goldenMechanismArtifactSummary `json:"published_artifact,omitempty"`
	PublishedMechanisms  []freshPublishedMechanism       `json:"published_mechanisms,omitempty"`
	TotalModelCalls      int                             `json:"total_model_calls"`
	TotalInputTokens     int                             `json:"total_input_tokens"`
	TotalOutputTokens    int                             `json:"total_output_tokens"`
	CacheHitTokens       int                             `json:"prompt_cache_hit_tokens"`
	CacheMissTokens      int                             `json:"prompt_cache_miss_tokens"`
	ProviderLatencyMS    int64                           `json:"provider_latency_ms"`
	SourceExtractionMS   int64                           `json:"source_extraction_ms"`
	PlanningBundleMS     int64                           `json:"planning_bundle_ms"`
	OpportunityWallMS    int64                           `json:"opportunity_wall_ms"`
	CandidateSelectionMS int64                           `json:"candidate_selection_ms"`
	WallMillis           int64                           `json:"wall_ms"`
}

type freshRepoCandidatesArtifact struct {
	Version       int                                   `json:"version"`
	Proposal      semanticdiscovery.OpportunityProposal `json:"proposal"`
	Normalization semanticdiscovery.NormalizationReport `json:"normalization"`
	Selected      []freshRepoCandidatePlan              `json:"selected"`
}

type freshRepoOpportunityAttemptArtifact struct {
	Version            int                                   `json:"version"`
	PromptVersion      string                                `json:"prompt_version"`
	ValidationState    string                                `json:"validation_state"`
	FailureReason      string                                `json:"failure_reason,omitempty"`
	ModelProposal      semanticdiscovery.OpportunityProposal `json:"model_proposal"`
	NormalizedProposal semanticdiscovery.OpportunityProposal `json:"normalized_proposal"`
	Normalization      semanticdiscovery.NormalizationReport `json:"normalization"`
}

type freshRepoSourceFactsArtifact struct {
	Version   int                      `json:"version"`
	Windows   int                      `json:"windows"`
	Functions int                      `json:"functions"`
	Facts     []semanticdiscovery.Fact `json:"facts"`
}

type freshRepoCandidatePlan struct {
	CandidateID             string                                      `json:"candidate_id"`
	Question                string                                      `json:"question"`
	Kind                    semanticdiscovery.ArtifactKind              `json:"kind"`
	AnchorFactIDs           []string                                    `json:"anchor_fact_ids"`
	AnchorEvidenceIDs       []string                                    `json:"anchor_evidence_ids"`
	CentralityReason        string                                      `json:"centrality_reason"`
	RequiredCapabilities    []semanticdiscovery.Capability              `json:"required_capabilities"`
	AvailableCapabilities   []semanticdiscovery.Capability              `json:"available_capabilities"`
	MissingCapabilities     []semanticdiscovery.Capability              `json:"missing_capabilities"`
	ExpectedFrontier        []string                                    `json:"expected_frontier,omitempty"`
	ExpectedOnboardingValue semanticdiscovery.ExpectedValue             `json:"expected_onboarding_value"`
	LocalScore              int                                         `json:"local_score"`
	Centrality              freshCandidateCentrality                    `json:"centrality"`
	ProductIntent           *semanticdiscovery.OpportunityProductIntent `json:"product_intent,omitempty"`
	Identity                semanticdiscovery.MechanismIdentity         `json:"identity"`
	Aspects                 []semanticdiscovery.AnswerAspect            `json:"aspects,omitempty"`
	Probe                   goldenmechanism.Plan                        `json:"probe"`
	Primary                 *freshPrimaryProbePlan                      `json:"primary_path,omitempty"`
}

type freshRepoCandidateAttempt struct {
	CandidateID        string                                  `json:"candidate_id"`
	Question           string                                  `json:"question"`
	State              string                                  `json:"state"`
	FailureStage       string                                  `json:"failure_stage,omitempty"`
	FailureReason      string                                  `json:"failure_reason,omitempty"`
	IntentKey          string                                  `json:"intent_key,omitempty"`
	ProbeBudget        *goldenmechanism.BudgetStats            `json:"probe_budget,omitempty"`
	ProbePartial       bool                                    `json:"probe_partial,omitempty"`
	ProbeStopReason    goldenmechanism.StopReason              `json:"probe_stop_reason,omitempty"`
	ProbeSHA256        string                                  `json:"probe_sha256,omitempty"`
	ProbeFacts         int                                     `json:"probe_facts,omitempty"`
	Synthesis          *semanticDiscoveryStageMetrics          `json:"synthesis,omitempty"`
	Reduction          *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	Artifact           *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	VisibleSteps       int                                     `json:"visible_steps,omitempty"`
	OnboardingRole     report.OnboardingRole                   `json:"onboarding_role,omitempty"`
	PrimaryEligibility *freshPrimaryEligibility                `json:"primary_eligibility,omitempty"`
	WallMillis         int64                                   `json:"wall_ms"`
}

type freshPublishedMechanism struct {
	CandidateID string                `json:"candidate_id"`
	MechanismID string                `json:"mechanism_id"`
	ArtifactID  string                `json:"artifact_id"`
	Role        report.OnboardingRole `json:"role"`
}

type freshSourceFunction struct {
	Function sourcewindowfacts.Function
	Fact     semanticdiscovery.Fact
}

type freshCandidateWork struct {
	Candidate      semanticdiscovery.OpportunityCandidate
	Plan           freshRepoCandidatePlan
	Seeds          []freshSourceFunction
	InitialSources []freshSourceFunction
	Primary        *freshPrimaryCandidateWork
}

// editFreshRepoMechanismForRun is the production entrypoint for the bounded
// cold-repository experiment. All repository-wide facts and saved source
// windows already exist before this function runs; only candidate-scoped Go
// syntax probes are allowed below this boundary.
func editFreshRepoMechanismForRun(
	ctx context.Context,
	runDir string,
	repoRoot string,
	stderr io.Writer,
) (freshRepoDemoResult, error) {
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return freshRepoDemoResult{}, fmt.Errorf("fresh repository mechanism: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	return prepareFreshRepoMechanism(ctx, runDir, repoRoot, client)
}

func prepareFreshRepoMechanism(
	ctx context.Context,
	runDir string,
	repoRoot string,
	provider semanticDiscoveryEditor,
) (result freshRepoDemoResult, returnErr error) {
	started := time.Now()
	status := freshRepoDemoStatus{Version: 1, State: "started"}
	result.Status = status
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil && status.State != "published" {
			status.State = "failed"
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		result.Status = status
		if err := writeGoldenJSON(filepath.Join(runDir, freshRepoDemoStatusFile), status); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	if ctx == nil {
		return result, fmt.Errorf("fresh repository mechanism: context is required")
	}
	if provider == nil {
		return result, fmt.Errorf("fresh repository mechanism: provider is required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return result, fmt.Errorf("fresh repository mechanism: resolve run directory: %w", err)
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return result, fmt.Errorf("fresh repository mechanism: resolve repository root: %w", err)
	}
	data, err := report.ReadRunDir(absRunDir)
	if err != nil {
		return result, fmt.Errorf("fresh repository mechanism: read saved run: %w", err)
	}
	if err := preserveLegacyFreshMechanism(absRunDir); err != nil {
		return result, err
	}

	sourceStarted := time.Now()
	savedSources, windowCount, _, savedSourceErr := freshSourceFunctions(
		absRunDir,
		absRepoRoot,
	)
	centralSources, centralParsedBytes, err := freshCentralSourceFunctions(
		absRunDir,
		absRepoRoot,
		data,
	)
	status.SourceExtractionMS = time.Since(sourceStarted).Milliseconds()
	if err != nil {
		return result, err
	}
	sources := mergeFreshSourceFunctions(
		savedSources,
		centralSources,
		freshRepoOnboardingMaxPlanningFacts,
	)
	discoverySources := freshSavedDiscoverySources(absRunDir, data, sources)
	if savedSourceErr != nil && len(sources) == 0 && len(discoverySources) == 0 {
		if errors.Is(savedSourceErr, os.ErrNotExist) {
			status.State = "no_publishable_candidate"
			status.FailureReason = "no_exact_discovery_anchors"
			return result, nil
		}
		if errors.Is(savedSourceErr, errFreshNoUsableSourceWindows) {
			status.State = "no_publishable_candidate"
			status.FailureReason = "no_exact_discovery_anchors"
			return result, nil
		}
		return result, savedSourceErr
	}
	sources = appendFreshDiscoverySources(
		sources,
		discoverySources,
		freshRepoOnboardingMaxPlanningFacts,
	)
	status.SourceWindows = windowCount
	status.SourceFunctions = len(sources)
	status.CentralFunctions = len(centralSources)
	status.CentralParsedBytes = centralParsedBytes
	status.SourceFacts = len(sources)
	sourceFacts := freshFacts(sources)
	if err := writeGoldenJSON(filepath.Join(absRunDir, freshRepoDemoFactsFile), freshRepoSourceFactsArtifact{
		Version: 1, Windows: windowCount, Functions: len(sources), Facts: sourceFacts,
	}); err != nil {
		return result, err
	}
	if len(sources) == 0 {
		status.State = "no_publishable_candidate"
		status.FailureReason = "no_exact_discovery_anchors"
		return result, nil
	}

	bundleStarted := time.Now()
	data.SemanticSupplementalFacts = sourceFacts
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	status.PlanningBundleMS = time.Since(bundleStarted).Milliseconds()
	if err != nil {
		return result, fmt.Errorf("fresh repository mechanism: build planning bundle: %w", err)
	}
	opportunityStarted := time.Now()
	proposal, modelProposal, normalization, opportunityMetrics, err := executeSemanticOpportunityScan(
		ctx,
		bundle,
		provider,
		&semanticDiscoveryBudget{},
	)
	status.OpportunityWallMS = time.Since(opportunityStarted).Milliseconds()
	status.Opportunity = &opportunityMetrics
	status.addMetrics(opportunityMetrics)
	opportunityAttempt := freshRepoOpportunityAttemptArtifact{
		Version:            1,
		PromptVersion:      opportunityMetrics.PromptVersion,
		ValidationState:    opportunityMetrics.Status,
		ModelProposal:      modelProposal,
		NormalizedProposal: proposal,
		Normalization:      normalization,
	}
	if err != nil {
		opportunityAttempt.FailureReason = semanticDiscoveryReason(err.Error())
	}
	if writeErr := writeGoldenJSON(
		filepath.Join(absRunDir, freshRepoOpportunityFile),
		opportunityAttempt,
	); writeErr != nil {
		return result, writeErr
	}
	if err != nil {
		status.FailureReason = "opportunity_scan_rejected"
		return result, err
	}
	proposal = capFreshProposal(proposal)
	status.QuestionsProposed = len(proposal.Candidates)
	if len(proposal.Candidates) == 0 {
		status.State = "no_publishable_candidate"
		status.FailureReason = "no_mechanism_questions_proposed"
		return result, nil
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return result, fmt.Errorf("fresh repository mechanism: capped proposal: %w", err)
	}

	selectionStarted := time.Now()
	works := selectFreshDiscoveryCandidates(absRepoRoot, data, proposal, sources)
	status.CandidateSelectionMS = time.Since(selectionStarted).Milliseconds()
	status.CandidatesSelected = len(works)
	plans := make([]freshRepoCandidatePlan, 0, len(works))
	for _, work := range works {
		plans = append(plans, work.Plan)
	}
	if err := writeGoldenJSON(filepath.Join(absRunDir, freshRepoDemoCandidatesFile), freshRepoCandidatesArtifact{
		Version: 1, Proposal: proposal, Normalization: normalization, Selected: plans,
	}); err != nil {
		return result, err
	}
	if len(works) == 0 {
		status.State = "no_publishable_candidate"
		status.FailureReason = "no_candidate_has_candidate_specific_function_anchors"
		return result, nil
	}

	for _, work := range works {
		attempt, mechanism, summary, attemptErr := attemptFreshCandidate(
			ctx,
			absRunDir,
			absRepoRoot,
			data,
			work,
			provider,
		)
		if attempt.Synthesis != nil {
			status.addMetrics(*attempt.Synthesis)
		}
		if attemptErr != nil {
			status.Attempts = append(status.Attempts, attempt)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			continue
		}
		role, err := freshPublishedOnboardingRole(absRunDir, summary.ID)
		if err != nil {
			return result, err
		}
		attempt.OnboardingRole = role
		status.Attempts = append(status.Attempts, attempt)
		if err := writeGoldenJSON(
			filepath.Join(absRunDir, freshRepoDemoAttemptsDir, work.Candidate.ID, "attempt.json"),
			attempt,
		); err != nil {
			return result, err
		}
		status.PublishedMechanisms = append(status.PublishedMechanisms, freshPublishedMechanism{
			CandidateID: work.Candidate.ID,
			MechanismID: mechanism.ID,
			ArtifactID:  summary.ID,
			Role:        role,
		})
		status.State = "published"
		if status.PublishedArtifact == nil || role == report.OnboardingRolePrimaryBehavior {
			status.PublishedCandidateID = work.Candidate.ID
			status.PublishedMechanismID = mechanism.ID
			status.PublishedArtifact = &summary
		}
		if !freshContinueAfterPublication(role) {
			break
		}
	}
	editorialMetrics, editorialAttempted, editorialErr := editFreshRepositoryOnboarding(
		ctx,
		absRunDir,
		provider,
		freshPreferredArtifactID(status),
	)
	if editorialAttempted {
		status.OnboardingEditor = &editorialMetrics
		status.addMetrics(editorialMetrics)
	}
	if editorialErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		status.OnboardingEditorFail = semanticDiscoveryReason(editorialErr.Error())
	}
	if len(status.PublishedMechanisms) > 0 {
		return result, nil
	}

	status.State = "no_publishable_candidate"
	status.FailureReason = "all_bounded_candidates_rejected"
	return result, nil
}

func freshContinueAfterPublication(_ report.OnboardingRole) bool {
	return true
}

func freshPreferredArtifactID(status freshRepoDemoStatus) string {
	if status.PublishedArtifact == nil {
		return ""
	}
	return status.PublishedArtifact.ID
}

func (status *freshRepoDemoStatus) addMetrics(metrics semanticDiscoveryStageMetrics) {
	if metrics.ProviderCall {
		status.TotalModelCalls++
	}
	status.TotalInputTokens += metrics.InputTokens
	status.TotalOutputTokens += metrics.OutputTokens
	status.CacheHitTokens += metrics.PromptCacheHitTokens
	status.CacheMissTokens += metrics.PromptCacheMissTokens
	status.ProviderLatencyMS += metrics.LatencyMillis
}

func freshSourceFunctions(
	runDir string,
	repoRoot string,
) ([]freshSourceFunction, int, int, error) {
	windows, err := sourcewindowfacts.LoadRunForDiscovery(runDir, repoRoot)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("fresh repository mechanism: load saved source windows: %w", err)
	}
	if len(windows) == 0 {
		return nil, 0, 0, errFreshNoUsableSourceWindows
	}
	byFunction := make(map[string]sourcewindowfacts.Function)
	functionCount := 0
	for _, window := range windows {
		functions, extractErr := sourcewindowfacts.ExtractGoFunctions(window)
		if extractErr != nil {
			return nil, len(windows), functionCount, fmt.Errorf(
				"fresh repository mechanism: extract functions from %s: %w",
				window.Path,
				extractErr,
			)
		}
		functionCount += len(functions)
		for _, function := range functions {
			if len(freshSubstantiveWindowObservations(function.Observations)) == 0 {
				continue
			}
			key := function.Path + "\x00" + function.Symbol
			previous, exists := byFunction[key]
			if !exists || freshWindowFunctionBetter(function, previous) {
				byFunction[key] = function
			}
		}
	}
	functions := make([]sourcewindowfacts.Function, 0, len(byFunction))
	for _, function := range byFunction {
		functions = append(functions, function)
	}
	sort.Slice(functions, func(i, j int) bool {
		leftScore := freshWindowFunctionScore(functions[i])
		rightScore := freshWindowFunctionScore(functions[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if functions[i].Path != functions[j].Path {
			return functions[i].Path < functions[j].Path
		}
		if functions[i].StartLine != functions[j].StartLine {
			return functions[i].StartLine < functions[j].StartLine
		}
		return functions[i].Symbol < functions[j].Symbol
	})
	if len(functions) > freshRepoDemoMaxSourceFacts {
		functions = functions[:freshRepoDemoMaxSourceFacts]
	}
	result := make([]freshSourceFunction, 0, len(functions))
	for _, function := range functions {
		fact, buildErr := freshWindowFunctionFact(function)
		if buildErr != nil {
			return nil, len(windows), functionCount, buildErr
		}
		result = append(result, freshSourceFunction{Function: function, Fact: fact})
	}
	return result, len(windows), functionCount, nil
}

func freshSavedDiscoverySources(
	runDir string,
	data *report.ReportData,
	represented []freshSourceFunction,
) []freshSourceFunction {
	if data == nil {
		return nil
	}
	state, err := modelresearch.ReadState(runDir)
	if err != nil {
		return nil
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	representedAnchors := make(map[string]struct{}, len(represented))
	for _, source := range represented {
		if !freshSourceUsesGoProofAdapter(source) {
			continue
		}
		for _, symbol := range freshRepresentedGoSymbols(source.Function.Symbol) {
			representedAnchors[freshDiscoveryAnchorKey(
				source.Function.Path,
				"go",
				symbol,
				source.Function.StartLine,
			)] = struct{}{}
		}
	}
	result := make([]freshSourceFunction, 0, freshRepoDemoMaxSourceFacts)
	seen := make(map[string]struct{}, freshRepoDemoMaxSourceFacts)
	for _, item := range state.Theory.GroundedFacts {
		if item.Kind != modelresearch.EvidenceSource || item.Location == nil ||
			item.Window == nil || !item.Window.CodeBearing || item.Window.Truncated ||
			item.Location.Path == "" || item.Location.Line != item.Window.StartLine ||
			item.Window.StartLine <= 0 || item.Window.EndLine < item.Window.StartLine ||
			len(item.Window.Lines) != item.Window.EndLine-item.Window.StartLine+1 {
			continue
		}
		if _, allowed := openable[item.Location.Path]; !allowed {
			continue
		}
		if !freshSavedWindowHasMatchingProvenance(item) {
			continue
		}
		for _, anchor := range report.ExactDiscoveryAnchors(
			item.Location.Path,
			item.Window.StartLine,
			item.Window.Lines,
		) {
			key := freshDiscoveryAnchorKey(
				anchor.Path,
				anchor.Language,
				anchor.Symbol,
				anchor.Line,
			)
			// Preserve existing Go request bytes only when the exact
			// declaration is already represented by the established Go
			// collector. Declaration-only Go anchors must remain discoverable.
			if _, alreadyRepresented := representedAnchors[key]; alreadyRepresented {
				continue
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			lineIndex := anchor.Line - item.Window.StartLine
			lineText := item.Window.Lines[lineIndex]
			evidenceID := goldenStableID(
				"frde",
				item.ID,
				anchor.Path,
				anchor.Symbol,
				fmt.Sprint(anchor.Line),
			)
			factID := goldenStableID(
				"frdf",
				anchor.Path,
				anchor.Language,
				anchor.Symbol,
				fmt.Sprint(anchor.Line),
				anchor.ContentSHA256,
			)
			result = append(result, freshSourceFunction{
				Function: sourcewindowfacts.Function{
					Path:          anchor.Path,
					Symbol:        anchor.Symbol,
					StartLine:     anchor.Line,
					EndLine:       anchor.Line,
					Lines:         []string{lineText},
					ContentSHA256: anchor.ContentSHA256,
				},
				Fact: semanticdiscovery.Fact{
					ID:          factID,
					Kind:        semanticdiscovery.FactSourceSignal,
					Statement:   anchor.Statement,
					Keywords:    sortedGoldenStrings([]string{"exact declaration", anchor.Language, anchor.Symbol}),
					SourceGroup: goldenStableID("frdg", factID),
					// Behavior means the code-bearing saved window can ground
					// an investigation question. It does not assert a call,
					// effect, order, or completed mechanism; those remain
					// unavailable without the proof adapter below.
					Capabilities: []semanticdiscovery.Capability{
						semanticdiscovery.CapabilityStatic,
						semanticdiscovery.CapabilityBehavior,
					},
					Scope: semanticdiscovery.FactScopeLocal,
					Source: &semanticdiscovery.FactSource{
						Path:            anchor.Path,
						StartLine:       anchor.Line,
						EndLine:         anchor.Line,
						EnclosingSymbol: anchor.Symbol,
						ContentSHA256:   anchor.ContentSHA256,
					},
					Evidence: []semanticdiscovery.EvidenceRef{{
						ID: evidenceID, Kind: "exact_source_declaration",
						Label: anchor.Language + " declaration",
						Path:  anchor.Path, Line: anchor.Line,
					}},
				},
			})
			if len(result) == freshRepoDemoMaxSourceFacts {
				return result
			}
		}
	}
	return result
}

func freshDiscoveryAnchorKey(
	sourcePath string,
	language string,
	symbol string,
	line int,
) string {
	return sourcePath + "\x00" + language + "\x00" + symbol + "\x00" + fmt.Sprint(line)
}

func freshRepresentedGoSymbols(symbol string) []string {
	result := []string{symbol}
	if separator := strings.LastIndex(symbol, "."); separator >= 0 &&
		separator+1 < len(symbol) {
		result = append(result, symbol[separator+1:])
	}
	return result
}

func freshSavedWindowHasMatchingProvenance(item modelresearch.EvidenceItem) bool {
	if item.Location == nil {
		return false
	}
	for _, provenance := range item.Provenance {
		if provenance.Location != nil &&
			provenance.Location.Path == item.Location.Path &&
			provenance.Location.Line == item.Location.Line {
			return true
		}
	}
	return false
}

func appendFreshDiscoverySources(
	sources []freshSourceFunction,
	discovery []freshSourceFunction,
	limit int,
) []freshSourceFunction {
	if limit <= 0 {
		return nil
	}
	result := append([]freshSourceFunction(nil), sources...)
	if len(result) >= limit {
		return result[:limit]
	}
	seen := make(map[string]struct{}, len(result)+len(discovery))
	for _, source := range result {
		seen[source.Fact.ID] = struct{}{}
	}
	for _, source := range discovery {
		if _, duplicate := seen[source.Fact.ID]; duplicate {
			continue
		}
		seen[source.Fact.ID] = struct{}{}
		result = append(result, source)
		if len(result) == limit {
			break
		}
	}
	return result
}

func freshWindowFunctionBetter(left, right sourcewindowfacts.Function) bool {
	if left.Partial != right.Partial {
		return !left.Partial
	}
	if len(left.Observations) != len(right.Observations) {
		return len(left.Observations) > len(right.Observations)
	}
	return len(left.Lines) > len(right.Lines)
}

func freshWindowFunctionScore(function sourcewindowfacts.Function) int {
	score := len(freshSubstantiveWindowObservations(function.Observations)) * 5
	name := strings.ToLower(function.Symbol)
	for _, central := range []string{"main", "run", "serve", "handle", "dispatch", "execute", "open", "new", "start"} {
		if strings.Contains(name, central) {
			score += 6
		}
	}
	if function.Partial {
		score -= 2
	}
	return score
}

func freshSubstantiveWindowObservations(
	observations []sourcewindowfacts.Observation,
) []sourcewindowfacts.Observation {
	result := make([]sourcewindowfacts.Observation, 0, len(observations))
	for _, observation := range observations {
		if observation.Kind == sourcewindowfacts.ObservationDeclaration {
			continue
		}
		result = append(result, observation)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := freshWindowObservationPriority(result[i])
		right := freshWindowObservationPriority(result[j])
		if left != right {
			return left > right
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		if result[i].Column != result[j].Column {
			return result[i].Column < result[j].Column
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func freshWindowObservationPriority(observation sourcewindowfacts.Observation) int {
	switch observation.Kind {
	case sourcewindowfacts.ObservationDirectCall:
		return 6
	case sourcewindowfacts.ObservationReturn:
		return 5
	case sourcewindowfacts.ObservationAssignment:
		return 4
	case sourcewindowfacts.ObservationBranch:
		return 3
	case sourcewindowfacts.ObservationRead:
		return 2
	default:
		return 0
	}
}

type freshWindowObservationClass uint8

const (
	freshWindowObservationLocalCall freshWindowObservationClass = iota
	freshWindowObservationDomainCall
	freshWindowObservationOutput
	freshWindowObservationStateWrite
	freshWindowObservationBranch
	freshWindowObservationLocalWrite
	freshWindowObservationRead
	freshWindowObservationAuxiliaryCall
	freshWindowObservationClassCount
)

// freshRepresentativeWindowObservations keeps a small fact representative of
// the visible function instead of allowing the first four calls to consume the
// entire budget. Logging, formatting, built-in, and synchronization/context
// calls remain available as a fallback, but cannot displace local/domain calls
// or visible output and state effects.
func freshRepresentativeWindowObservations(
	observations []sourcewindowfacts.Observation,
	limit int,
) []sourcewindowfacts.Observation {
	if limit <= 0 {
		return nil
	}
	ordered := freshSubstantiveWindowObservations(observations)
	if len(ordered) <= limit {
		return ordered
	}

	buckets := make([][]sourcewindowfacts.Observation, freshWindowObservationClassCount)
	for _, observation := range ordered {
		class := freshWindowObservationCategory(observation)
		buckets[class] = append(buckets[class], observation)
	}

	selected := make([]sourcewindowfacts.Observation, 0, limit)
	appendRepresentativeRounds := func(classes []freshWindowObservationClass) {
		for round := 0; len(selected) < limit; round++ {
			appended := false
			for _, class := range classes {
				if round >= len(buckets[class]) {
					continue
				}
				selected = append(selected, buckets[class][round])
				appended = true
				if len(selected) == limit {
					return
				}
			}
			if !appended {
				return
			}
		}
	}

	appendRepresentativeRounds([]freshWindowObservationClass{
		freshWindowObservationLocalCall,
		freshWindowObservationDomainCall,
		freshWindowObservationOutput,
		freshWindowObservationStateWrite,
		freshWindowObservationBranch,
	})
	appendRepresentativeRounds([]freshWindowObservationClass{
		freshWindowObservationLocalWrite,
		freshWindowObservationRead,
	})
	appendRepresentativeRounds([]freshWindowObservationClass{
		freshWindowObservationAuxiliaryCall,
	})
	return selected
}

func freshWindowObservationCategory(
	observation sourcewindowfacts.Observation,
) freshWindowObservationClass {
	switch observation.Kind {
	case sourcewindowfacts.ObservationDirectCall:
		if freshAuxiliaryWindowCall(observation.Target) {
			return freshWindowObservationAuxiliaryCall
		}
		identifiers := freshIdentifiers(observation.Target)
		if len(identifiers) == 1 {
			return freshWindowObservationLocalCall
		}
		return freshWindowObservationDomainCall
	case sourcewindowfacts.ObservationReturn:
		return freshWindowObservationOutput
	case sourcewindowfacts.ObservationAssignment:
		if observation.Operator != ":=" && observation.Operator != "var" {
			return freshWindowObservationStateWrite
		}
		if strings.ContainsAny(observation.Object, ".[*") {
			return freshWindowObservationStateWrite
		}
		return freshWindowObservationLocalWrite
	case sourcewindowfacts.ObservationBranch:
		return freshWindowObservationBranch
	case sourcewindowfacts.ObservationRead:
		return freshWindowObservationRead
	default:
		return freshWindowObservationRead
	}
}

func freshAuxiliaryWindowCall(target string) bool {
	identifiers := freshIdentifiers(target)
	if len(identifiers) == 0 {
		return true
	}
	if len(identifiers) == 1 && freshBuiltinCall(identifiers[0]) {
		return true
	}

	receiverWords := make(map[string]struct{}, len(identifiers)*2)
	for _, identifier := range identifiers[:len(identifiers)-1] {
		for _, word := range freshIdentifierWords(identifier) {
			receiverWords[strings.ToLower(word)] = struct{}{}
		}
	}
	for _, auxiliary := range []string{
		"context", "ctx", "errors", "fmt", "formatter", "formatting", "log", "logger",
		"logging", "mu", "mutex", "rwmutex", "slog", "sync",
	} {
		if _, ok := receiverWords[auxiliary]; ok {
			return true
		}
	}

	last := strings.ToLower(identifiers[len(identifiers)-1])
	switch last {
	case "cancel", "debugf", "errorf", "fprintf", "fprint", "fprintln", "lock",
		"printf", "println", "rlock", "runlock", "sprintf", "sprint", "sprintln", "unlock":
		return true
	default:
		return false
	}
}

func freshWindowFunctionFact(
	function sourcewindowfacts.Function,
) (semanticdiscovery.Fact, error) {
	observations := freshRepresentativeWindowObservations(
		function.Observations,
		freshRepoDemoMaxFactOps,
	)
	if len(observations) == 0 {
		return semanticdiscovery.Fact{}, fmt.Errorf(
			"fresh repository mechanism: %s has no substantive bounded observation",
			function.Symbol,
		)
	}
	statements := make([]string, 0, len(observations))
	capabilities := []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic}
	evidenceRefs := make([]semanticdiscovery.EvidenceRef, 0, len(observations))
	keywords := []string{"bounded function", freshHumanLabel(function.Symbol)}
	for _, observation := range observations {
		statement, capability := freshWindowObservationStatement(function, observation)
		statements = append(statements, statement)
		capabilities = append(capabilities, capability)
		if capability != semanticdiscovery.CapabilityDataRead {
			capabilities = append(capabilities, semanticdiscovery.CapabilityBehavior)
		}
		evidenceRefs = append(evidenceRefs, semanticdiscovery.EvidenceRef{
			ID: observation.ID, Kind: "bounded_go_window", Label: string(observation.Kind),
			Path: function.Path, Line: observation.Line, Column: observation.Column,
		})
		keywords = append(keywords, freshObservationKeyword(observation))
	}
	fact := semanticdiscovery.Fact{
		ID: goldenStableID(
			"frf",
			function.Path,
			function.Symbol,
			function.ContentSHA256,
		),
		Kind:         semanticdiscovery.FactSourceSignal,
		Statement:    strings.Join(statements, " "),
		Keywords:     sortedGoldenStrings(keywords),
		SourceGroup:  goldenStableID("frg", function.Path, function.Symbol, function.ContentSHA256),
		Capabilities: freshCapabilities(capabilities...),
		Scope:        semanticdiscovery.FactScopeLocal,
		Source: &semanticdiscovery.FactSource{
			Path: function.Path, StartLine: function.StartLine, EndLine: function.EndLine,
			EnclosingSymbol: function.Symbol, ContentSHA256: function.ContentSHA256,
		},
		Evidence: evidenceRefs,
	}
	return fact, nil
}

func freshWindowObservationStatement(
	function sourcewindowfacts.Function,
	observation sourcewindowfacts.Observation,
) (string, semanticdiscovery.Capability) {
	subject := freshHumanLabel(function.Symbol)
	switch observation.Kind {
	case sourcewindowfacts.ObservationDirectCall:
		return fmt.Sprintf(
			"The %s function directly calls the %s operation.",
			subject,
			freshHumanLabel(observation.Target),
		), semanticdiscovery.CapabilityDirectCall
	case sourcewindowfacts.ObservationBranch:
		return fmt.Sprintf(
			"The %s function checks a branch condition involving %s.",
			subject,
			freshHumanLabel(observation.Object),
		), semanticdiscovery.CapabilityBranch
	case sourcewindowfacts.ObservationAssignment:
		return fmt.Sprintf(
			"The %s function writes the %s value.",
			subject,
			freshHumanLabel(observation.Object),
		), semanticdiscovery.CapabilityDataWrite
	case sourcewindowfacts.ObservationRead:
		return fmt.Sprintf(
			"The %s function reads the %s value.",
			subject,
			freshHumanLabel(observation.Object),
		), semanticdiscovery.CapabilityDataRead
	case sourcewindowfacts.ObservationReturn:
		return fmt.Sprintf(
			"The %s function returns %s.",
			subject,
			freshReturnLabel(observation.Value),
		), semanticdiscovery.CapabilityOutputEffect
	default:
		return fmt.Sprintf("The %s function contains a bounded source operation.", subject), semanticdiscovery.CapabilityStatic
	}
}

func freshReturnLabel(value string) string {
	label := freshHumanLabel(value)
	if label == "source operation" || label == "nil" || label == "" {
		return "its result"
	}
	return "the " + label + " result"
}

func freshObservationKeyword(observation sourcewindowfacts.Observation) string {
	switch observation.Kind {
	case sourcewindowfacts.ObservationDirectCall:
		return "direct call " + freshHumanLabel(observation.Target)
	case sourcewindowfacts.ObservationBranch:
		return "branch " + freshHumanLabel(observation.Object)
	case sourcewindowfacts.ObservationAssignment:
		return "write " + freshHumanLabel(observation.Object)
	case sourcewindowfacts.ObservationRead:
		return "read " + freshHumanLabel(observation.Object)
	case sourcewindowfacts.ObservationReturn:
		return "return " + freshHumanLabel(observation.Value)
	default:
		return "source operation"
	}
}

func freshFacts(sources []freshSourceFunction) []semanticdiscovery.Fact {
	result := make([]semanticdiscovery.Fact, 0, len(sources))
	for _, source := range sources {
		result = append(result, source.Fact)
	}
	return result
}

func capFreshProposal(
	proposal semanticdiscovery.OpportunityProposal,
) semanticdiscovery.OpportunityProposal {
	result := semanticdiscovery.OpportunityProposal{Version: proposal.Version}
	for _, candidate := range proposal.Candidates {
		if candidate.Kind != semanticdiscovery.ArtifactMechanism {
			continue
		}
		result.Candidates = append(result.Candidates, candidate)
		if len(result.Candidates) == freshRepoDemoMaxQuestions {
			break
		}
	}
	return result
}

func selectFreshDiscoveryCandidates(
	repoRoot string,
	data *report.ReportData,
	proposal semanticdiscovery.OpportunityProposal,
	sources []freshSourceFunction,
) []freshCandidateWork {
	candidates := proposal.Candidates
	if len(candidates) > freshRepoDemoMaxCandidates {
		candidates = candidates[:freshRepoDemoMaxCandidates]
	}
	works := make([]freshCandidateWork, 0, len(candidates))
	for _, candidate := range candidates {
		anchors := freshCandidateDiscoveryAnchors(candidate, sources)
		if len(anchors) > 0 && !freshSourcesUseGoProofAdapter(anchors) {
			works = append(works, freshUnsupportedProofCandidate(data, candidate, sources, anchors))
			continue
		}
		works = append(works, planFreshPrimaryCandidate(repoRoot, data, candidate, sources))
	}
	freshMarkPrimaryIntentCollisions(works)
	sort.SliceStable(works, func(i, j int) bool {
		leftCentral := freshCandidateIsEligibleFirstContact(works[i])
		rightCentral := freshCandidateIsEligibleFirstContact(works[j])
		if leftCentral != rightCentral {
			return leftCentral
		}
		leftReady := works[i].Plan.Primary != nil && works[i].Plan.Primary.Status == freshPrimaryPlanReady
		rightReady := works[j].Plan.Primary != nil && works[j].Plan.Primary.Status == freshPrimaryPlanReady
		if leftReady != rightReady {
			return leftReady
		}
		if comparison := compareFreshCandidateCentrality(
			works[i].Plan.Centrality,
			works[j].Plan.Centrality,
		); comparison != 0 {
			return comparison > 0
		}
		return works[i].Candidate.ID < works[j].Candidate.ID
	})
	if len(works) > freshRepoDemoMaxCandidates {
		works = works[:freshRepoDemoMaxCandidates]
	}
	return works
}

func freshCandidateDiscoveryAnchors(
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
) []freshSourceFunction {
	byFact := make(map[string]freshSourceFunction, len(sources))
	for _, source := range sources {
		byFact[source.Fact.ID] = source
	}
	result := make([]freshSourceFunction, 0, freshRepoDemoMaxSeedFuncs)
	seen := make(map[string]struct{}, freshRepoDemoMaxSeedFuncs)
	for _, factID := range freshCandidatePlanningAnchorIDs(candidate) {
		source, exists := byFact[factID]
		if !exists {
			continue
		}
		key := source.Function.Path + "\x00" + source.Function.Symbol
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, source)
		if len(result) == freshRepoDemoMaxSeedFuncs {
			break
		}
	}
	return result
}

func freshSourcesUseGoProofAdapter(sources []freshSourceFunction) bool {
	if len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		if !freshSourceUsesGoProofAdapter(source) {
			return false
		}
	}
	return true
}

func freshSourceUsesGoProofAdapter(source freshSourceFunction) bool {
	return strings.EqualFold(filepath.Ext(source.Function.Path), ".go")
}

func freshUnsupportedProofCandidate(
	data *report.ReportData,
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
	anchors []freshSourceFunction,
) freshCandidateWork {
	const reason = "proof_adapter_unavailable"
	rootAnchors := make([]freshPrimaryAnchor, 0, len(anchors))
	distinctSymbols := make([]string, 0, len(anchors))
	distinctFiles := make([]string, 0, len(anchors))
	seenFiles := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if len(anchor.Fact.Evidence) == 0 {
			continue
		}
		rootAnchors = append(rootAnchors, freshPrimaryAnchor{
			ID: goldenStableID(
				"fpa",
				anchor.Function.Path,
				anchor.Function.Symbol,
				anchor.Function.ContentSHA256,
			),
			OriginFactID:     anchor.Fact.ID,
			OriginEvidenceID: anchor.Fact.Evidence[0].ID,
			Path:             anchor.Function.Path,
			Symbol:           anchor.Function.Symbol,
			ContentSHA256:    anchor.Function.ContentSHA256,
		})
		distinctSymbols = append(
			distinctSymbols,
			anchor.Function.Path+"\x00"+anchor.Function.Symbol,
		)
		if _, duplicate := seenFiles[anchor.Function.Path]; !duplicate {
			seenFiles[anchor.Function.Path] = struct{}{}
			distinctFiles = append(distinctFiles, anchor.Function.Path)
		}
	}
	primary := &freshPrimaryProbePlan{
		Version:       freshPrimaryPlanVersion,
		CandidateID:   candidate.ID,
		Question:      candidate.QuestionAnswered,
		Status:        freshPrimaryPlanInsufficient,
		StatusReasons: []string{reason},
		RootAnchors:   rootAnchors,
		Aspects:       freshPrimaryAspects(candidate.QuestionAnswered),
		StopConditions: []string{
			reason,
		},
		EffectResolution: reason,
		Eligibility: freshPrimaryEligibility{
			Status:          freshPrimaryPlanInsufficient,
			Reasons:         []string{reason},
			DistinctSymbols: distinctSymbols,
			DistinctFiles:   distinctFiles,
		},
		AnchorFacts: freshFacts(anchors),
	}
	work := freshCandidateWork{
		Candidate:      candidate,
		Seeds:          anchors,
		InitialSources: sources,
	}
	work.Plan = freshPrimaryCandidatePlan(data, candidate, sources, anchors, primary)
	return work
}

func selectFreshCandidates(
	repoRoot string,
	data *report.ReportData,
	proposal semanticdiscovery.OpportunityProposal,
	sources []freshSourceFunction,
) []freshCandidateWork {
	candidates := proposal.Candidates
	if len(candidates) > freshRepoDemoMaxCandidates {
		candidates = candidates[:freshRepoDemoMaxCandidates]
	}
	works := make([]freshCandidateWork, 0, len(candidates))
	for _, candidate := range candidates {
		seeds := selectFreshSeeds(candidate, sources)
		if len(seeds) < 3 {
			continue
		}
		pkg, ok := freshOwningPackage(data, seeds)
		if !ok {
			continue
		}
		intentKey := freshIntentKey(pkg, seeds)
		frontier := freshExpansionFrontier(repoRoot, pkg, seeds, sources)
		expansionAllowlist := append([]string(nil), frontier.Symbols...)
		if len(expansionAllowlist) == 0 {
			// A non-empty allowlist closes the probe frontier. Reusing an exact
			// seed symbol adds no follow-up because it is already scheduled.
			expansionAllowlist = []string{seeds[0].Function.Symbol}
		}
		probeSeeds := append(freshProbeSeeds(seeds), frontier.Seeds...)
		parsedSourceBudget := freshRepoDemoMaxParsedBytes - frontier.ParsedBytes
		if parsedSourceBudget <= 0 {
			continue
		}
		plan := goldenmechanism.Plan{
			MechanismID:        intentKey,
			Seeds:              probeSeeds,
			ExpansionAllowlist: expansionAllowlist,
			Limits: goldenmechanism.Limits{
				MaxDepth: 3, MaxFiles: freshRepoDemoMaxProbeFiles, MaxFunctions: 15,
				MaxParsedSourceBytes: parsedSourceBudget, MaxSourceBytes: 128 << 10,
				MaxFunctionLines: 220, MaxFunctionBytes: 48 << 10, Timeout: 5 * time.Second,
			},
		}
		anchorFacts := make([]string, 0, len(seeds))
		anchorEvidence := make([]string, 0, len(seeds))
		for _, seed := range seeds {
			anchorFacts = append(anchorFacts, seed.Fact.ID)
			anchorEvidence = append(anchorEvidence, seed.Fact.Evidence[0].ID)
		}
		sort.Strings(anchorFacts)
		sort.Strings(anchorEvidence)
		purposeOverlap := freshPurposeOverlap(data, candidate)
		centrality := deriveFreshCandidateCentrality(
			data,
			candidate,
			sources,
			seeds,
			purposeOverlap,
		)
		localScore := centrality.localScore()
		work := freshCandidateWork{
			Candidate:      candidate,
			Seeds:          seeds,
			InitialSources: sources,
			Plan: freshRepoCandidatePlan{
				CandidateID: candidate.ID, Question: candidate.QuestionAnswered, Kind: candidate.Kind,
				AnchorFactIDs: anchorFacts, AnchorEvidenceIDs: anchorEvidence,
				CentralityReason: freshCentralityReason(candidate, seeds, centrality),
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityBehavior,
				},
				AvailableCapabilities:   freshSeedCapabilities(seeds),
				MissingCapabilities:     []semanticdiscovery.Capability{},
				ExpectedFrontier:        frontier.Symbols,
				ExpectedOnboardingValue: candidate.ExpectedValue,
				LocalScore:              localScore,
				Centrality:              centrality,
				ProductIntent:           candidate.ProductIntent,
				Identity: semanticdiscovery.MechanismIdentity{
					RepositoryNamespace: pkg.ModulePath,
					IntentKey:           intentKey,
					Scope: semanticdiscovery.MechanismScope{
						Kind:  semanticdiscovery.MechanismScopeGoPackage,
						Value: pkg.CanonicalPath,
					},
				},
				Probe: plan,
			},
		}
		works = append(works, work)
	}
	sort.Slice(works, func(i, j int) bool {
		if comparison := compareFreshCandidateCentrality(
			works[i].Plan.Centrality,
			works[j].Plan.Centrality,
		); comparison != 0 {
			return comparison > 0
		}
		return works[i].Candidate.ID < works[j].Candidate.ID
	})
	return diversifyFreshCandidates(works, freshRepoDemoMaxCandidates)
}

func selectFreshSeeds(
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
) []freshSourceFunction {
	anchorIDs := freshCandidateCentralAnchorIDs(candidate)
	supported := make(map[string]struct{}, len(anchorIDs))
	for _, id := range anchorIDs {
		supported[id] = struct{}{}
	}
	queryTerms := freshTerms(candidate.Title + " " + candidate.QuestionAnswered)
	ranked := append([]freshSourceFunction(nil), sources...)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := freshSeedScore(ranked[i], supported, queryTerms)
		right := freshSeedScore(ranked[j], supported, queryTerms)
		if left != right {
			return left > right
		}
		if ranked[i].Function.Path != ranked[j].Function.Path {
			return ranked[i].Function.Path < ranked[j].Function.Path
		}
		return ranked[i].Function.Symbol < ranked[j].Function.Symbol
	})
	result := make([]freshSourceFunction, 0, freshRepoDemoMaxSeedFuncs)
	paths := make(map[string]struct{})
	for _, source := range ranked {
		if _, exists := paths[source.Function.Path]; !exists && len(paths) >= freshRepoDemoMaxProbeFiles {
			continue
		}
		result = append(result, source)
		paths[source.Function.Path] = struct{}{}
		if len(result) == freshRepoDemoMaxSeedFuncs {
			break
		}
	}
	return result
}

func freshSeedScore(
	source freshSourceFunction,
	supported map[string]struct{},
	queryTerms map[string]struct{},
) int {
	score := freshWindowFunctionScore(source.Function)
	if _, exists := supported[source.Fact.ID]; exists {
		score += 100
	}
	for term := range freshTerms(source.Fact.Statement + " " + strings.Join(source.Fact.Keywords, " ")) {
		if _, exists := queryTerms[term]; exists {
			score += 4
		}
	}
	return score
}

func freshCentralityReason(
	candidate semanticdiscovery.OpportunityCandidate,
	seeds []freshSourceFunction,
	centrality freshCandidateCentrality,
) string {
	supported := make(map[string]struct{}, len(candidate.SupportIDs))
	for _, id := range candidate.SupportIDs {
		supported[id] = struct{}{}
	}
	direct := 0
	for _, seed := range seeds {
		if _, ok := supported[seed.Fact.ID]; ok {
			direct++
		}
	}
	return fmt.Sprintf(
		"Locally ranked from %s expected value, %s confidence, purpose=%d, explanation=%d, navigation=%d, evidence=%d, penalties=%d (core=%d, input=%d, effect=%d, boundary=%d, bounded-cost=%d, secondary-penalty=%d), %d directly selected fact(s), and %d exact function anchor(s).",
		candidate.ExpectedValue,
		candidate.Confidence,
		centrality.PurposeAlignment,
		centrality.ExplanatoryValue,
		centrality.NavigationValue,
		centrality.EvidenceReadiness,
		centrality.Penalties,
		centrality.CoreCoverage,
		centrality.InputCoverage,
		centrality.EffectCoverage,
		centrality.BoundaryCoverage,
		centrality.BoundedCost,
		centrality.SecondaryPenalty,
		direct,
		len(seeds),
	)
}

func freshPurposeOverlap(
	data *report.ReportData,
	candidate semanticdiscovery.OpportunityCandidate,
) int {
	if data == nil {
		return 0
	}
	purposeParts := []string{
		data.DocumentedPurpose,
		data.ProjectGuess,
		data.RecommendedFlow,
		data.RepoName,
	}
	for _, direction := range data.CandidateDirections {
		if direction.Disposition == "rejected" {
			continue
		}
		purposeParts = append(
			purposeParts,
			direction.Name,
			direction.Trigger,
			direction.WhyInteresting,
		)
	}
	purposeTerms := freshTerms(strings.Join(purposeParts, " "))
	overlap := 0
	for term := range freshTerms(candidate.Title + " " + candidate.QuestionAnswered) {
		if _, exists := purposeTerms[term]; exists {
			overlap++
		}
	}
	return overlap
}

func diversifyFreshCandidates(
	works []freshCandidateWork,
	limit int,
) []freshCandidateWork {
	if limit <= 0 || len(works) == 0 {
		return nil
	}
	remaining := append([]freshCandidateWork(nil), works...)
	selected := make([]freshCandidateWork, 0, min(limit, len(remaining)))
	for len(remaining) > 0 && len(selected) < limit {
		bestIndex := 0
		bestOverlap := freshAnchorOverlapCount(
			remaining[0].Plan.AnchorFactIDs,
			selected,
		)
		for index, work := range remaining {
			comparison := compareFreshCandidateCentrality(
				work.Plan.Centrality,
				remaining[bestIndex].Plan.Centrality,
			)
			overlap := freshAnchorOverlapCount(work.Plan.AnchorFactIDs, selected)
			if comparison > 0 ||
				(comparison == 0 && overlap < bestOverlap) ||
				(comparison == 0 && overlap == bestOverlap &&
					work.Candidate.ID < remaining[bestIndex].Candidate.ID) {
				bestIndex = index
				bestOverlap = overlap
			}
		}
		selected = append(selected, remaining[bestIndex])
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	return selected
}

func freshAnchorOverlapCount(
	left []string,
	selected []freshCandidateWork,
) int {
	leftSet := make(map[string]struct{}, len(left))
	for _, id := range left {
		leftSet[id] = struct{}{}
	}
	maximum := 0
	for _, previous := range selected {
		intersection := 0
		for _, id := range previous.Plan.AnchorFactIDs {
			if _, exists := leftSet[id]; exists {
				intersection++
			}
		}
		maximum = max(maximum, intersection)
	}
	return maximum
}

func freshOwningPackage(
	data *report.ReportData,
	seeds []freshSourceFunction,
) (report.PackageInfo, bool) {
	if data == nil || data.RepositoryGraph == nil {
		return report.PackageInfo{}, false
	}
	counts := make(map[string]int)
	byPath := make(map[string]report.PackageInfo)
	for _, pkg := range data.RepositoryGraph.Packages {
		if pkg.ModulePath == "" || pkg.CanonicalPath == "" ||
			(pkg.Locality != "" && pkg.Locality != "local") {
			continue
		}
		for _, file := range pkg.Files {
			for _, seed := range seeds {
				if file == seed.Function.Path {
					key := pkg.ModulePath + "\x00" + pkg.CanonicalPath
					counts[key]++
					byPath[key] = pkg
				}
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) == 0 {
		return report.PackageInfo{}, false
	}
	return byPath[keys[0]], true
}

func freshIntentKey(pkg report.PackageInfo, seeds []freshSourceFunction) string {
	anchors := make([]string, 0, len(seeds)+2)
	anchors = append(anchors, pkg.ModulePath, pkg.CanonicalPath)
	for _, seed := range seeds {
		anchors = append(anchors, seed.Function.Path+"\x00"+seed.Function.Symbol)
	}
	sort.Strings(anchors[2:])
	return goldenStableID("fresh", anchors...)
}

func freshProbeSeeds(seeds []freshSourceFunction) []goldenmechanism.Seed {
	result := make([]goldenmechanism.Seed, 0, len(seeds))
	for _, seed := range seeds {
		result = append(result, goldenmechanism.Seed{
			OriginFactID: seed.Fact.ID, OriginEvidenceID: seed.Fact.Evidence[0].ID,
			Path: seed.Function.Path, Symbol: seed.Function.Symbol,
		})
	}
	return result
}

type freshFrontierResult struct {
	Symbols     []string
	Seeds       []goldenmechanism.Seed
	ParsedBytes int
}

type freshFrontierCall struct {
	Name       string
	FactID     string
	EvidenceID string
	Path       string
	Line       int
}

// freshExpansionFrontier follows only exact, unqualified package-level calls.
// Candidate files come from the already-saved owning-package index, and at
// most two repository-local files are parsed through the confined reader.
func freshExpansionFrontier(
	repoRoot string,
	pkg report.PackageInfo,
	seeds []freshSourceFunction,
	all []freshSourceFunction,
) freshFrontierResult {
	calls := freshFrontierCalls(seeds)
	if len(calls) == 0 {
		return freshFrontierResult{}
	}
	seedKeys := make(map[string]struct{}, len(seeds))
	seedPaths := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		seedKeys[seed.Function.Path+"\x00"+seed.Function.Symbol] = struct{}{}
		seedPaths[seed.Function.Path] = struct{}{}
	}
	packageFiles := make(map[string]struct{}, len(pkg.Files))
	for _, path := range pkg.Files {
		packageFiles[path] = struct{}{}
	}

	result := freshFrontierResult{}
	resolvedNames := make(map[string]struct{})
	for _, call := range calls {
		var matches []freshSourceFunction
		for _, source := range all {
			if source.Function.Symbol != call.Name {
				continue
			}
			if _, owned := packageFiles[source.Function.Path]; !owned {
				continue
			}
			matches = append(matches, source)
		}
		if len(matches) != 1 {
			continue
		}
		match := matches[0]
		key := match.Function.Path + "\x00" + match.Function.Symbol
		if _, alreadySeeded := seedKeys[key]; alreadySeeded {
			continue
		}
		if _, newPath := seedPaths[match.Function.Path]; !newPath &&
			len(seedPaths) >= freshRepoDemoMaxProbeFiles {
			continue
		}
		result.Seeds = append(result.Seeds, goldenmechanism.Seed{
			OriginFactID: call.FactID, OriginEvidenceID: call.EvidenceID,
			Path: match.Function.Path, Symbol: match.Function.Symbol,
		})
		result.Symbols = append(result.Symbols, match.Function.Symbol)
		seedKeys[key] = struct{}{}
		seedPaths[match.Function.Path] = struct{}{}
		resolvedNames[call.Name] = struct{}{}
		if len(result.Seeds) == freshRepoDemoMaxFrontier {
			return normalizeFreshFrontier(result)
		}
	}

	reader, err := reporead.New(repoRoot)
	if err != nil {
		return normalizeFreshFrontier(result)
	}
	defer reader.Close()
	paths := freshFrontierCandidatePaths(pkg.Files, seedPaths, calls, resolvedNames)
	fileBudget := freshRepoDemoMaxProbeFiles - len(seedPaths)
	if fileBudget > freshRepoDemoMaxFrontier-len(result.Seeds) {
		fileBudget = freshRepoDemoMaxFrontier - len(result.Seeds)
	}
	for _, sourcePath := range paths {
		if fileBudget <= 0 || len(result.Seeds) == freshRepoDemoMaxFrontier {
			break
		}
		fileBudget--
		remaining := freshRepoDemoMaxParsedBytes - result.ParsedBytes
		if remaining <= 0 {
			break
		}
		content, readErr := reader.ReadFile(sourcePath, int64(remaining))
		if readErr != nil || content.Truncated {
			continue
		}
		fileSet := token.NewFileSet()
		file, parseErr := parser.ParseFile(fileSet, sourcePath, content.Bytes, parser.SkipObjectResolution)
		if parseErr != nil || (pkg.Name != "" && file.Name.Name != pkg.Name) {
			continue
		}
		result.ParsedBytes += len(content.Bytes)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Body == nil {
				continue
			}
			call, ok := freshFrontierCallByName(calls, resolvedNames, function.Name.Name)
			if !ok {
				continue
			}
			result.Seeds = append(result.Seeds, goldenmechanism.Seed{
				OriginFactID: call.FactID, OriginEvidenceID: call.EvidenceID,
				Path: sourcePath, Symbol: function.Name.Name,
			})
			result.Symbols = append(result.Symbols, function.Name.Name)
			resolvedNames[function.Name.Name] = struct{}{}
			seedPaths[sourcePath] = struct{}{}
			if len(result.Seeds) == freshRepoDemoMaxFrontier {
				break
			}
		}
	}
	return normalizeFreshFrontier(result)
}

func freshFrontierCalls(seeds []freshSourceFunction) []freshFrontierCall {
	var calls []freshFrontierCall
	for _, seed := range seeds {
		allowedEvidence := make(map[string]struct{}, len(seed.Fact.Evidence))
		for _, ref := range seed.Fact.Evidence {
			allowedEvidence[ref.ID] = struct{}{}
		}
		for _, observation := range seed.Function.Observations {
			if observation.Kind != sourcewindowfacts.ObservationDirectCall {
				continue
			}
			if _, supported := allowedEvidence[observation.ID]; !supported {
				continue
			}
			identifiers := freshIdentifiers(observation.Target)
			if len(identifiers) != 1 || freshBuiltinCall(identifiers[0]) {
				continue
			}
			calls = append(calls, freshFrontierCall{
				Name: identifiers[0], FactID: seed.Fact.ID, EvidenceID: observation.ID,
				Path: seed.Function.Path, Line: observation.Line,
			})
		}
	}
	sort.Slice(calls, func(i, j int) bool {
		if calls[i].Path != calls[j].Path {
			return calls[i].Path < calls[j].Path
		}
		if calls[i].Line != calls[j].Line {
			return calls[i].Line < calls[j].Line
		}
		return calls[i].Name < calls[j].Name
	})
	return calls
}

func freshFrontierCandidatePaths(
	files []string,
	seedPaths map[string]struct{},
	calls []freshFrontierCall,
	resolved map[string]struct{},
) []string {
	type rankedPath struct {
		path  string
		score int
	}
	var ranked []rankedPath
	for _, sourcePath := range files {
		if filepath.Ext(sourcePath) != ".go" || strings.HasSuffix(sourcePath, "_test.go") {
			continue
		}
		if _, seeded := seedPaths[sourcePath]; seeded {
			continue
		}
		score := 0
		fileTerms := freshTerms(filepath.Base(sourcePath))
		for _, call := range calls {
			if _, done := resolved[call.Name]; done {
				continue
			}
			for term := range freshTerms(call.Name) {
				if _, match := fileTerms[term]; match {
					score += 10
				}
			}
		}
		ranked = append(ranked, rankedPath{path: sourcePath, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].path < ranked[j].path
	})
	result := make([]string, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.path)
	}
	return result
}

func freshFrontierCallByName(
	calls []freshFrontierCall,
	resolved map[string]struct{},
	name string,
) (freshFrontierCall, bool) {
	if _, done := resolved[name]; done {
		return freshFrontierCall{}, false
	}
	for _, call := range calls {
		if call.Name == name {
			return call, true
		}
	}
	return freshFrontierCall{}, false
}

func normalizeFreshFrontier(result freshFrontierResult) freshFrontierResult {
	sort.Slice(result.Seeds, func(i, j int) bool {
		if result.Seeds[i].Path != result.Seeds[j].Path {
			return result.Seeds[i].Path < result.Seeds[j].Path
		}
		return result.Seeds[i].Symbol < result.Seeds[j].Symbol
	})
	result.Symbols = sortedGoldenStrings(result.Symbols)
	return result
}

func freshBuiltinCall(name string) bool {
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag",
		"len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	default:
		return false
	}
}

func freshSeedCapabilities(seeds []freshSourceFunction) []semanticdiscovery.Capability {
	var values []semanticdiscovery.Capability
	for _, seed := range seeds {
		values = append(values, seed.Fact.Capabilities...)
	}
	return freshCapabilities(values...)
}

func attemptFreshCandidate(
	ctx context.Context,
	runDir string,
	repoRoot string,
	data *report.ReportData,
	work freshCandidateWork,
	provider semanticDiscoveryEditor,
) (
	attempt freshRepoCandidateAttempt,
	mechanism semanticdiscovery.Mechanism,
	summary goldenMechanismArtifactSummary,
	returnErr error,
) {
	started := time.Now()
	attemptDir := filepath.Join(runDir, freshRepoDemoAttemptsDir, work.Candidate.ID)
	defer func() {
		attempt.WallMillis = time.Since(started).Milliseconds()
		if err := writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	attempt = freshRepoCandidateAttempt{
		CandidateID: work.Candidate.ID,
		Question:    work.Candidate.QuestionAnswered,
		State:       "started",
		IntentKey:   work.Plan.Identity.IntentKey,
	}
	if err := writeGoldenJSON(filepath.Join(attemptDir, "candidate_plan.json"), work.Plan); err != nil {
		attempt.reject("plan", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	if work.Plan.Primary != nil {
		eligibility := work.Plan.Primary.Eligibility
		attempt.PrimaryEligibility = &eligibility
		acceptedPath := filepath.Join(
			runDir,
			freshRepoDemoAttemptsDir,
			"accepted",
			work.Candidate.ID,
			"mechanism_v1.json",
		)
		if info, statErr := os.Lstat(acceptedPath); statErr == nil && info.Mode().IsRegular() {
			attempt.State = "preserved_existing_mechanism"
			attempt.FailureStage = "preservation"
			attempt.FailureReason = "existing accepted candidate retained without resynthesis"
			return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, errFreshPrimaryMechanismPreserved
		}
		if work.Plan.Primary.Status != freshPrimaryPlanReady {
			attempt.State = string(freshPrimaryPlanInsufficient)
			attempt.FailureStage = "eligibility"
			attempt.FailureReason = strings.Join(work.Plan.Primary.StatusReasons, ",")
			return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, errFreshPrimaryEvidenceInsufficient
		}
	}

	probe, err := goldenmechanism.Probe(ctx, repoRoot, work.Plan.Probe)
	if err != nil {
		attempt.reject("probe", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	probeRaw, err := marshalGoldenJSON(probe)
	if err != nil {
		attempt.reject("probe_encode", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	probeDigest := sha256.Sum256(probeRaw)
	probeSHA := hex.EncodeToString(probeDigest[:])
	attempt.ProbeBudget = &probe.Budget
	attempt.ProbePartial = probe.Partial
	attempt.ProbeStopReason = probe.StopReason
	attempt.ProbeSHA256 = probeSHA
	if err := writeAtomicFile(filepath.Join(attemptDir, "probe.json"), probeRaw, 0o600); err != nil {
		attempt.reject("probe_save", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}

	probeFacts, aspects, err := freshProbeFacts(work, probe)
	if err != nil {
		attempt.reject("fact_projection", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	attempt.ProbeFacts = len(probeFacts)
	work.Plan.Aspects = aspects
	if err := writeGoldenJSON(filepath.Join(attemptDir, "candidate_plan.json"), work.Plan); err != nil {
		attempt.reject("plan_save", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	facts, candidate, err := freshCandidateProjection(work, probeFacts, aspects)
	if err != nil {
		attempt.reject("candidate_projection", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	supplement, bundle, err := report.PrepareSemanticSupplement(
		data,
		candidate.ID,
		probeSHA,
		facts,
	)
	if err != nil {
		attempt.reject("supplement", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		attempt.reject("candidate_validation", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	leaf, err := freshValidatedLeaf(bundle, candidate, probeFacts)
	if err != nil {
		attempt.reject("leaf_validation", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}

	counted := &countingSemanticDiscoveryEditor{delegate: provider}
	synthesis, synthesisErr := executeGoldenMechanismSynthesis(
		ctx,
		bundle,
		proposal,
		leaf,
		counted,
	)
	attempt.Synthesis = &synthesis.Metrics
	attempt.Reduction = &synthesis.Reduction
	validationStatus := "accepted"
	failureClass := ""
	if synthesisErr != nil {
		validationStatus = "rejected"
		failureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
	}
	if err := writeGoldenJSON(filepath.Join(attemptDir, "response_attempt.json"), goldenMechanismResponseAttempt{
		Version: 1, CandidateID: candidate.ID,
		PromptVersion:    semanticdiscovery.GoldenMechanismPromptVersion,
		ValidationStatus: validationStatus, FailureClass: failureClass,
		Reduction: &synthesis.Reduction, Content: string(synthesis.RawResponse),
	}); err != nil {
		attempt.reject("response_save", err)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, errors.Join(synthesisErr, err)
	}
	if counted.calls != 1 {
		callErr := fmt.Errorf(
			"fresh repository mechanism: candidate synthesis used %d provider calls, want exactly one",
			counted.calls,
		)
		attempt.reject("synthesis_call_count", callErr)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, errors.Join(synthesisErr, callErr)
	}
	if synthesisErr != nil {
		attempt.reject("synthesis_validation", synthesisErr)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, synthesisErr
	}
	artifact := synthesis.Artifacts[0]
	if work.Plan.Primary != nil {
		if err := validateFreshPrimaryArtifact(artifact, work.Plan.Primary, probeFacts); err != nil {
			attempt.reject("primary_relevance", err)
			_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
			return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
		}
	}
	summary, err = summarizeGoldenMechanismArtifact(candidate, artifact)
	if err != nil {
		attempt.reject("publishability", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	attempt.Artifact = &summary

	mechanism, visibleSteps, err := publishFreshMechanism(
		runDir,
		work.Plan.Identity,
		candidate.ID,
		probeRaw,
		supplement,
		synthesis.RecordBytes,
		artifact,
	)
	if err != nil {
		attempt.reject("publication", err)
		_ = writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt)
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	attempt.State = "published"
	attempt.VisibleSteps = visibleSteps
	if err := writeGoldenJSON(filepath.Join(attemptDir, "attempt.json"), attempt); err != nil {
		return attempt, semanticdiscovery.Mechanism{}, goldenMechanismArtifactSummary{}, err
	}
	return attempt, mechanism, summary, nil
}

func (attempt *freshRepoCandidateAttempt) reject(stage string, err error) {
	attempt.State = "rejected"
	attempt.FailureStage = stage
	if err != nil {
		attempt.FailureReason = semanticDiscoveryReason(err.Error())
	}
}

func freshProbeFacts(
	work freshCandidateWork,
	probe goldenmechanism.Result,
) ([]semanticdiscovery.Fact, []semanticdiscovery.AnswerAspect, error) {
	if work.Plan.Primary != nil {
		if work.Primary == nil || len(work.Primary.ProbeFacts) == 0 {
			return nil, nil, fmt.Errorf(
				"fresh repository mechanism: primary planner produced no deterministic facts",
			)
		}
		if probe.MechanismID != work.Plan.Identity.IntentKey {
			return nil, nil, fmt.Errorf(
				"fresh repository mechanism: primary probe identity mismatch",
			)
		}
		facts := append([]semanticdiscovery.Fact(nil), work.Primary.ProbeFacts...)
		aspects := freshPrimaryAnswerAspects(work.Plan.Primary.Aspects)
		eligibility := deriveFreshPrimaryEligibility(work.Plan.Primary, facts, false)
		if eligibility.Status != freshPrimaryPlanReady {
			return nil, nil, fmt.Errorf(
				"fresh repository mechanism: primary facts became ineligible: %s",
				strings.Join(eligibility.Reasons, ","),
			)
		}
		return facts, aspects, nil
	}
	functions := make(map[string]goldenmechanism.Function, len(probe.Functions))
	for _, function := range probe.Functions {
		functions[function.ID] = function
	}
	supportedFactIDs, supportedScopes := freshCandidateSupportedScopes(work)
	relevant := make([]goldenmechanism.Observation, 0, len(probe.Observations))
	diversification := make([]goldenmechanism.Observation, 0, len(probe.Observations))
	for _, observation := range probe.Observations {
		if freshProbeObservationPriority(observation) <= 0 || len(observation.Evidence) == 0 {
			continue
		}
		function, exists := functions[observation.FunctionID]
		if !exists {
			continue
		}
		if freshProbeFunctionMatchesCandidate(function, supportedFactIDs, supportedScopes) {
			relevant = append(relevant, observation)
			continue
		}
		diversification = append(diversification, observation)
	}
	sortObservations := func(observations []goldenmechanism.Observation) {
		sort.SliceStable(observations, func(i, j int) bool {
			left := freshProbeObservationPriority(observations[i])
			right := freshProbeObservationPriority(observations[j])
			if left != right {
				return left > right
			}
			return observations[i].ID < observations[j].ID
		})
	}
	sortObservations(relevant)
	sortObservations(diversification)
	selected := make([]goldenmechanism.Observation, 0, freshRepoDemoMaxProbeFacts)
	seenFunctions := make(map[string]struct{})
	seenObservations := make(map[string]struct{})
	appendGroup := func(observations []goldenmechanism.Observation) {
		for _, observation := range observations {
			if len(selected) == freshRepoDemoMaxProbeFacts {
				return
			}
			if _, exists := seenFunctions[observation.FunctionID]; exists {
				continue
			}
			selected = append(selected, observation)
			seenFunctions[observation.FunctionID] = struct{}{}
			seenObservations[observation.ID] = struct{}{}
		}
		for _, observation := range observations {
			if len(selected) == freshRepoDemoMaxProbeFacts {
				return
			}
			if _, exists := seenObservations[observation.ID]; exists {
				continue
			}
			selected = append(selected, observation)
			seenObservations[observation.ID] = struct{}{}
		}
	}
	// Candidate-supported functions own the fixed answer contract. Only after
	// their useful operations are exhausted may unrelated seed functions add
	// bounded context for diversification.
	appendGroup(relevant)
	appendGroup(diversification)
	if len(selected) < 3 {
		return nil, nil, fmt.Errorf(
			"fresh repository mechanism: bounded probe retained %d useful operations, need at least three",
			len(selected),
		)
	}

	facts := make([]semanticdiscovery.Fact, 0, len(selected))
	aspects := make([]semanticdiscovery.AnswerAspect, 0, len(selected))
	for index, observation := range selected {
		function := functions[observation.FunctionID]
		aspectID := fmt.Sprintf("operation-%d", index+1)
		statement := freshProbeObservationStatement(function, observation)
		references := make([]semanticdiscovery.EvidenceRef, 0, len(observation.Evidence))
		for _, reference := range observation.Evidence {
			references = append(references, semanticdiscovery.EvidenceRef{
				ID: reference.ID, Kind: "bounded_go_syntax",
				Path: reference.Location.Path, Line: reference.Location.Line,
				Column: reference.Location.Column,
			})
		}
		capabilities := []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic, observation.Capability}
		if observation.Capability != semanticdiscovery.CapabilitySequence {
			capabilities = append(capabilities, semanticdiscovery.CapabilityBehavior)
		}
		fact := semanticdiscovery.Fact{
			ID:        goldenStableID("frp", work.Plan.Identity.IntentKey, observation.ID),
			Kind:      semanticdiscovery.FactSourceSignal,
			Statement: statement,
			Keywords: sortedGoldenStrings([]string{
				"answer_aspect:" + aspectID,
				freshOperationLabel(observation.Operation),
				freshHumanLabel(function.Symbol),
			}),
			SourceGroup:  function.ID,
			Capabilities: freshCapabilities(capabilities...),
			Scope:        semanticdiscovery.FactScopeLocal,
			Source:       freshProbeFunctionSource(function),
			Evidence:     references,
		}
		facts = append(facts, fact)
		aspects = append(aspects, semanticdiscovery.AnswerAspect{
			ID:                   aspectID,
			Label:                freshOperationLabel(observation.Operation),
			RequiredCapabilities: []semanticdiscovery.Capability{observation.Capability},
			Key:                  index < 3,
		})
	}
	return facts, aspects, nil
}

func freshCandidateSupportedScopes(
	work freshCandidateWork,
) (map[string]struct{}, map[string]struct{}) {
	supportedFactIDs := make(map[string]struct{}, len(work.Candidate.SupportIDs))
	for _, id := range work.Candidate.SupportIDs {
		supportedFactIDs[id] = struct{}{}
	}
	supportedScopes := make(map[string]struct{}, len(work.Candidate.SupportIDs))
	for _, source := range work.InitialSources {
		if _, supported := supportedFactIDs[source.Fact.ID]; !supported {
			continue
		}
		supportedScopes[freshFunctionScope(source.Function.Path, source.Function.Symbol)] = struct{}{}
	}
	return supportedFactIDs, supportedScopes
}

func freshProbeFunctionMatchesCandidate(
	function goldenmechanism.Function,
	supportedFactIDs map[string]struct{},
	supportedScopes map[string]struct{},
) bool {
	for _, id := range function.OriginFactIDs {
		if _, supported := supportedFactIDs[id]; supported {
			return true
		}
	}
	_, supported := supportedScopes[freshFunctionScope(function.Path, function.Symbol)]
	return supported
}

func freshFunctionScope(path string, symbol string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) + "\x00" + strings.TrimSpace(symbol)
}

func freshProbeObservationPriority(observation goldenmechanism.Observation) int {
	switch observation.Operation {
	case "http_handler_entry_signature":
		return 100
	case "read_directory", "url_query_get", "parse_parameter":
		return 95
	case "direct_local_call", "local_error_handoff":
		return 90
	case "json_encode", "template_execute", "plain_format", "write_header",
		"write_response", "write_to_response", "response_header":
		return 88
	case "sort", "slice", "append", "append_assignment", "direct_return":
		return 82
	case "assignment", "error_return":
		return 75
	case "branch_predicate":
		return 70
	case "lexical_order":
		return 60
	default:
		return 0
	}
}

func freshProbeObservationStatement(
	function goldenmechanism.Function,
	observation goldenmechanism.Observation,
) string {
	subject := freshHumanLabel(function.Symbol)
	target := freshHumanLabel(observation.TargetSymbol)
	if target == "source operation" {
		target = freshHumanLabel(observation.Object)
	}
	switch observation.Operation {
	case "http_handler_entry_signature":
		return fmt.Sprintf("The %s function accepts an HTTP request and response writer.", subject)
	case "read_directory":
		return fmt.Sprintf("The %s function reads directory entries.", subject)
	case "url_query_get":
		return fmt.Sprintf("The %s function reads a URL query value.", subject)
	case "parse_parameter":
		return fmt.Sprintf("The %s function parses a request parameter.", subject)
	case "direct_local_call":
		return fmt.Sprintf("The %s function directly calls the %s operation.", subject, target)
	case "local_error_handoff":
		return fmt.Sprintf("The %s function passes a local error to the %s operation.", subject, target)
	case "json_encode":
		return fmt.Sprintf("The %s function encodes JSON output.", subject)
	case "template_execute":
		return fmt.Sprintf("The %s function executes a template into an output destination.", subject)
	case "plain_format":
		return fmt.Sprintf("The %s function formats plain text into an output destination.", subject)
	case "write_header":
		return fmt.Sprintf("The %s function writes an HTTP response status.", subject)
	case "write_response":
		return fmt.Sprintf("The %s function writes bytes to an HTTP response.", subject)
	case "write_to_response":
		return fmt.Sprintf("The %s function writes buffered output to an HTTP response.", subject)
	case "response_header":
		return fmt.Sprintf("The %s function changes an HTTP response header.", subject)
	case "sort":
		return fmt.Sprintf("The %s function sorts a collection.", subject)
	case "slice":
		return fmt.Sprintf("The %s function slices a collection.", subject)
	case "append", "append_assignment":
		return fmt.Sprintf("The %s function appends data to a collection.", subject)
	case "assignment":
		return fmt.Sprintf("The %s function writes a local value.", subject)
	case "direct_return":
		return fmt.Sprintf("The %s function directly returns the %s operation result.", subject, target)
	case "error_return":
		return fmt.Sprintf("The %s function returns a non-nil error expression.", subject)
	case "branch_predicate":
		return fmt.Sprintf("The %s function checks a branch condition involving %s.", subject, freshHumanLabel(observation.Object))
	case "lexical_order":
		return fmt.Sprintf(
			"In the bounded source for the %s function, the %s operation appears before the %s operation.",
			subject,
			freshOperationLabel(observation.Subject),
			freshOperationLabel(observation.Object),
		)
	default:
		return fmt.Sprintf("The %s function contains a bounded source operation.", subject)
	}
}

func freshOperationLabel(operation string) string {
	labels := map[string]string{
		"http_handler_entry_signature": "HTTP request entry",
		"read_directory":               "Directory read",
		"url_query_get":                "Query read",
		"parse_parameter":              "Parameter parsing",
		"direct_local_call":            "Local handoff",
		"local_error_handoff":          "Error handoff",
		"json_encode":                  "JSON output",
		"template_execute":             "Template output",
		"plain_format":                 "Plain text output",
		"write_header":                 "Response status",
		"write_response":               "Response write",
		"write_to_response":            "Buffered response",
		"response_header":              "Response header",
		"sort":                         "Collection sorting",
		"slice":                        "Collection slicing",
		"append":                       "Collection append",
		"append_assignment":            "Collection append",
		"assignment":                   "Local value write",
		"direct_return":                "Direct return",
		"error_return":                 "Error return",
		"branch_predicate":             "Branch decision",
		"lexical_order":                "Local source order",
	}
	if label := labels[operation]; label != "" {
		return label
	}
	label := freshHumanLabel(operation)
	if label == "source operation" {
		return "Source operation"
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

func freshProbeFunctionSource(function goldenmechanism.Function) *semanticdiscovery.FactSource {
	if len(function.Source) == 0 {
		return nil
	}
	lines := make([]string, 0, len(function.Source))
	for _, line := range function.Source {
		lines = append(lines, line.Text)
	}
	raw, _ := json.Marshal(lines)
	digest := sha256.Sum256(raw)
	return &semanticdiscovery.FactSource{
		Path:            function.Path,
		StartLine:       function.Source[0].Location.Line,
		EndLine:         function.Source[len(function.Source)-1].Location.Line,
		EnclosingSymbol: function.Symbol,
		ContentSHA256:   hex.EncodeToString(digest[:]),
	}
}

func freshCandidateProjection(
	work freshCandidateWork,
	probeFacts []semanticdiscovery.Fact,
	aspects []semanticdiscovery.AnswerAspect,
) ([]semanticdiscovery.Fact, semanticdiscovery.OpportunityCandidate, error) {
	initialByID := make(map[string]semanticdiscovery.Fact, len(work.InitialSources))
	for _, source := range work.InitialSources {
		initialByID[source.Fact.ID] = source.Fact
	}
	selected := make(map[string]semanticdiscovery.Fact)
	for _, seed := range work.Seeds {
		selected[seed.Fact.ID] = seed.Fact
	}
	for _, fact := range probeFacts {
		selected[fact.ID] = fact
	}
	// Every model-selected supplemental seed remains available even if local
	// seed ranking chose a different bounded function.
	for _, id := range work.Candidate.SupportIDs {
		if fact, exists := initialByID[id]; exists {
			selected[id] = fact
		}
	}
	if len(selected) > freshRepoDemoMaxSourceFacts {
		return nil, semanticdiscovery.OpportunityCandidate{}, fmt.Errorf(
			"fresh repository mechanism: candidate needs %d supplemental facts, limit is %d",
			len(selected),
			freshRepoDemoMaxSourceFacts,
		)
	}
	facts := make([]semanticdiscovery.Fact, 0, len(selected))
	for _, fact := range selected {
		facts = append(facts, fact)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })

	candidate := work.Candidate
	initialSupport := make(map[string]struct{}, len(candidate.SupportIDs))
	for _, id := range candidate.SupportIDs {
		initialSupport[id] = struct{}{}
	}
	candidate.EnrichmentSupportIDs = nil
	for _, fact := range facts {
		if _, duplicate := initialSupport[fact.ID]; !duplicate {
			candidate.EnrichmentSupportIDs = append(candidate.EnrichmentSupportIDs, fact.ID)
		}
	}
	sort.Strings(candidate.EnrichmentSupportIDs)
	allCapabilities := make([]semanticdiscovery.Capability, 0)
	for _, fact := range facts {
		allCapabilities = append(allCapabilities, fact.Capabilities...)
	}
	allCapabilities = freshCapabilities(allCapabilities...)
	candidate.CapabilityContract = &semanticdiscovery.CapabilityContract{
		RequiredCapabilities:  allCapabilities,
		AvailableCapabilities: append([]semanticdiscovery.Capability(nil), allCapabilities...),
		MissingCapabilities:   []semanticdiscovery.Capability{},
		Resolution:            semanticdiscovery.CapabilityResolutionReady,
	}
	aliases := sortedGoldenStrings([]string{candidate.QuestionAnswered, candidate.Title})
	candidate.IntentContract = &semanticdiscovery.IntentContract{
		RequiredAnswerAspects: append([]semanticdiscovery.AnswerAspect(nil), aspects...),
		MinCovered:            3,
		MinKeyCovered:         3,
		LocalSearchAliases:    aliases,
	}
	return facts, candidate, nil
}

func freshValidatedLeaf(
	bundle semanticdiscovery.Bundle,
	candidate semanticdiscovery.OpportunityCandidate,
	probeFacts []semanticdiscovery.Fact,
) (semanticdiscovery.LeafResult, error) {
	tasks, err := semanticdiscovery.PlanLeafTasks(
		bundle,
		[]semanticdiscovery.OpportunityCandidate{candidate},
	)
	if err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	artifact := semanticdiscovery.LeafArtifact{
		Version:     semanticdiscovery.LeafArtifactVersion,
		TaskID:      tasks[0].ID,
		CandidateID: candidate.ID,
		Status:      semanticdiscovery.LeafStatusUsable,
	}
	known := make(map[string]semanticdiscovery.Fact, len(tasks[0].Facts))
	for _, fact := range tasks[0].Facts {
		known[fact.ID] = fact
	}
	used := make([]string, 0, len(candidate.SupportIDs)+len(probeFacts))
	seen := make(map[string]struct{}, len(candidate.SupportIDs)+len(probeFacts))
	appendObservation := func(fact semanticdiscovery.Fact) {
		if _, duplicate := seen[fact.ID]; duplicate {
			return
		}
		artifact.Observations = append(artifact.Observations, semanticdiscovery.LeafObservation{
			Text:       semanticdiscovery.ProjectTrustedFactStatement(fact.Statement),
			SupportIDs: []string{fact.ID},
		})
		used = append(used, fact.ID)
		seen[fact.ID] = struct{}{}
	}
	groundingSlots := 8 - len(probeFacts)
	grounding, err := freshGroundingObservations(known, candidate.SupportIDs, groundingSlots)
	if err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	for _, observation := range grounding {
		artifact.Observations = append(artifact.Observations, observation)
		for _, id := range observation.SupportIDs {
			used = append(used, id)
			seen[id] = struct{}{}
		}
	}
	for _, fact := range probeFacts {
		if _, exists := known[fact.ID]; !exists {
			return semanticdiscovery.LeafResult{}, fmt.Errorf(
				"fresh repository mechanism: probe fact %q is unavailable to the leaf",
				fact.ID,
			)
		}
		appendObservation(fact)
	}
	artifact.CandidateConnection = semanticdiscovery.LeafCandidateConnection{
		CandidateID: candidate.ID,
		Relation:    "needs_combination",
		Explanation: "The bounded source operations need one editorial synthesis to answer the selected mechanism question.",
		SupportIDs:  sortedGoldenStrings(used),
	}
	artifact = semanticdiscovery.NormalizeLeafArtifact(artifact)
	if err := semanticdiscovery.ValidateLeafArtifact(tasks[0], artifact); err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	return semanticdiscovery.LeafResult{Task: tasks[0], Artifact: artifact}, nil
}

// freshGroundingObservations packs exact planner-grounding statements into the
// remaining leaf slots. It preserves every fact and clause while keeping the
// five aspect-bearing probe observations atomic under the existing eight-item
// leaf contract.
func freshGroundingObservations(
	known map[string]semanticdiscovery.Fact,
	ids []string,
	maxItems int,
) ([]semanticdiscovery.LeafObservation, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("fresh repository mechanism: candidate has no support facts")
	}
	if maxItems <= 0 {
		return nil, fmt.Errorf("fresh repository mechanism: no leaf slots remain for candidate support")
	}
	itemCount := len(ids)
	if itemCount > maxItems {
		itemCount = maxItems
	}
	statements := make([][]string, itemCount)
	supportIDs := make([][]string, itemCount)
	for index, id := range ids {
		fact, exists := known[id]
		if !exists {
			return nil, fmt.Errorf(
				"fresh repository mechanism: candidate support fact %q is unavailable to the leaf",
				id,
			)
		}
		group := index % itemCount
		statements[group] = append(statements[group], fact.Statement)
		supportIDs[group] = append(supportIDs[group], fact.ID)
	}
	result := make([]semanticdiscovery.LeafObservation, 0, itemCount)
	for index := 0; index < itemCount; index++ {
		result = append(result, semanticdiscovery.LeafObservation{
			Text: semanticdiscovery.ProjectTrustedFactStatement(
				strings.Join(statements[index], " "),
			),
			SupportIDs: supportIDs[index],
		})
	}
	return result, nil
}

func publishFreshMechanism(
	runDir string,
	identity semanticdiscovery.MechanismIdentity,
	candidateID string,
	probeRaw []byte,
	supplement report.SemanticSupplement,
	recordRaw []byte,
	want semanticdiscovery.Artifact,
) (mechanism semanticdiscovery.Mechanism, visibleSteps int, returnErr error) {
	supplementRaw, err := marshalGoldenJSON(supplement)
	if err != nil {
		return mechanism, 0, err
	}
	recordRaw = append(append([]byte(nil), recordRaw...), '\n')
	paths := []string{
		filepath.Join(runDir, report.GoldenMechanismProbeFile),
		filepath.Join(runDir, report.GoldenMechanismFactsFile),
		filepath.Join(runDir, report.GoldenMechanismRecordFile),
		filepath.Join(runDir, semanticdiscovery.MechanismFile),
	}
	backups, err := backupGoldenFiles(paths)
	if err != nil {
		return mechanism, 0, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := restoreGoldenFiles(backups); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("fresh repository mechanism: rollback: %w", err))
		}
	}()
	// The saved Mechanism is derived from the other three files. Keeping the
	// previous Mechanism beside replacement facts while ExtractMechanismV1
	// re-reads the run makes ReadRunDir report a transient stale-input warning.
	// Warnings are planner context, so that warning changes the replay bundle
	// hash before the replacement can be extracted. Remove the backed-up
	// derived file for the duration of the transaction; rollback restores it.
	if err := os.Remove(paths[3]); err != nil && !os.IsNotExist(err) {
		return mechanism, 0, err
	}
	for index, raw := range [][]byte{probeRaw, supplementRaw, recordRaw} {
		if err := writeAtomicFile(paths[index], raw, 0o600); err != nil {
			return mechanism, 0, err
		}
	}
	extracted, artifact, err := report.ExtractMechanismV1(runDir, candidateID, identity)
	if err != nil {
		return mechanism, 0, err
	}
	if err := requireGoldenArtifact([]semanticdiscovery.Artifact{artifact}, want); err != nil {
		return mechanism, 0, err
	}
	mechanismRaw, err := semanticdiscovery.EncodeMechanism(extracted)
	if err != nil {
		return mechanism, 0, err
	}
	if err := writeAtomicFile(paths[3], mechanismRaw, 0o600); err != nil {
		return mechanism, 0, err
	}
	replayed, err := report.ReadRunDir(runDir)
	if err != nil {
		return mechanism, 0, err
	}
	if err := report.ApplySavedMechanismV1(
		replayed,
		paths[3],
		paths[1],
		paths[0],
	); err != nil {
		return mechanism, 0, err
	}
	for _, projected := range replayed.UserMechanisms {
		if projected.ArtifactID != artifact.ID {
			continue
		}
		if len(projected.Steps) < 3 {
			return mechanism, 0, fmt.Errorf(
				"fresh repository mechanism: accepted artifact projects only %d code-backed user steps",
				len(projected.Steps),
			)
		}
		if err := archiveFreshMechanismEntry(
			runDir,
			candidateID,
			probeRaw,
			supplementRaw,
			mechanismRaw,
		); err != nil {
			return mechanism, 0, err
		}
		committed = true
		return extracted, len(projected.Steps), nil
	}
	return mechanism, 0, fmt.Errorf("fresh repository mechanism: accepted artifact has no user projection")
}

func freshCapabilities(values ...semanticdiscovery.Capability) []semanticdiscovery.Capability {
	seen := make(map[semanticdiscovery.Capability]struct{}, len(values))
	result := make([]semanticdiscovery.Capability, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func freshHumanLabel(value string) string {
	identifiers := freshIdentifiers(value)
	if len(identifiers) == 0 {
		return "source operation"
	}
	if len(identifiers) > 3 {
		identifiers = identifiers[len(identifiers)-3:]
	}
	words := make([]string, 0, len(identifiers)*2)
	for _, identifier := range identifiers {
		words = append(words, freshIdentifierWords(identifier)...)
	}
	filtered := words[:0]
	for _, word := range words {
		lower := strings.ToLower(word)
		switch lower {
		case "ctx", "context", "mx", "x", "r", "w", "s", "c", "m", "p", "f", "nil", "true", "false":
			continue
		}
		filtered = append(filtered, lower)
	}
	if len(filtered) == 0 {
		return "source operation"
	}
	return strings.Join(filtered, " ")
}

func freshIdentifiers(value string) []string {
	result := make([]string, 0, 4)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		identifier := string(current)
		if identifier != "func" && identifier != "return" && identifier != "if" {
			result = append(result, identifier)
		}
		current = nil
	}
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' {
			current = append(current, char)
			continue
		}
		flush()
	}
	flush()
	return result
}

func freshIdentifierWords(identifier string) []string {
	identifier = strings.Trim(identifier, "_")
	if identifier == "" {
		return nil
	}
	var words []string
	start := 0
	runes := []rune(identifier)
	for index := 1; index < len(runes); index++ {
		previous, current := runes[index-1], runes[index]
		nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
		boundary := current == '_' || previous == '_' ||
			(unicode.IsLower(previous) && unicode.IsUpper(current)) ||
			(unicode.IsDigit(previous) != unicode.IsDigit(current)) ||
			(unicode.IsUpper(previous) && unicode.IsUpper(current) && nextLower)
		if !boundary {
			continue
		}
		part := strings.Trim(string(runes[start:index]), "_")
		if part != "" {
			words = append(words, part)
		}
		start = index
		if current == '_' {
			start++
		}
	}
	if start < len(runes) {
		part := strings.Trim(string(runes[start:]), "_")
		if part != "" {
			words = append(words, part)
		}
	}
	return words
}

func freshTerms(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, identifier := range freshIdentifiers(value) {
		for _, word := range freshIdentifierWords(identifier) {
			word = strings.ToLower(word)
			if len(word) >= 3 {
				result[word] = struct{}{}
			}
		}
	}
	return result
}

// Keep the interface assertion close to the production constructor. It also
// documents that the fresh-repository path uses the existing measured model
// client rather than a new provider abstraction.
var _ semanticDiscoveryEditor = (*deepseek.Client)(nil)
