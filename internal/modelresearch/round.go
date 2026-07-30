package modelresearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/secretscan"
)

type Prompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

type ProviderResult struct {
	Content               []byte
	Attempts              int
	RequestBytes          int
	InputTokens           int
	OutputTokens          int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
}

type Provider interface {
	BuildResearchRequest(Prompt) ([]byte, error)
	Research(context.Context, Prompt) (ProviderResult, error)
}

type ExecuteInput struct {
	Plan       PlannedRound
	Policy     Policy
	Usage      Usage
	Repository RepositoryContext
	RunsDir    string
	RunDir     string
	Profile    string
	Model      string
	Provider   Provider
}

type researchResponse struct {
	Findings            []RawFinding `json:"findings"`
	UnresolvedFrontiers []Frontier   `json:"unresolved_frontiers"`
	Summary             string       `json:"summary,omitempty"`
}

func BuildPrompt(bundle EvidenceBundle) (Prompt, error) {
	if bundle.Version != ContractVersion || bundle.PolicyVersion != PolicyVersion ||
		strings.TrimSpace(bundle.Question) == "" || len(bundle.Evidence) == 0 {
		return Prompt{}, fmt.Errorf("model research: incomplete targeted evidence bundle")
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return Prompt{}, fmt.Errorf("model research: encode targeted bundle: %w", err)
	}
	system := `You interpret one bounded, locally assembled repository evidence bundle. Structural and runtime facts remain under local authority. Use only supplied opaque evidence IDs. Do not create files, paths, symbols, surfaces, transitions, proof, runtime order, or evidence. Model prose is interpretation, never a verified fact. Return valid JSON only.`
	user := `Answer exactly one research question from the supplied evidence.

Return this JSON shape:
{
  "findings": [
    {
      "id": "short stable response-local id",
      "interpretation": "concise interpretation",
      "responsibility_name": "optional conceptual responsibility name",
      "hypothesis_assessment": "supported | unsupported | mixed",
      "evidence_ids": ["one or more exact supplied evidence ids"],
      "explanation": "short human-readable explanation"
    }
  ],
  "unresolved_frontiers": [
    {
      "question": "one next bounded frontier",
      "evidence_ids": ["supplied evidence ids that expose it"],
      "evidence_category": "optional local evidence category",
      "runtime_only": false,
      "reason": "why it remains unresolved"
    }
  ],
  "summary": "optional concise round summary"
}

Rules:
- Every finding and frontier must cite supplied evidence IDs.
- Do not emit paths, symbols, relations, or IDs absent from the bundle.
- Do not upgrade static evidence to observed execution or guaranteed runtime order.
- A source_window is a bounded lexical excerpt, not a complete file or call graph. Missing text cannot prove that an operation, call, or relation is absent. Describe only what the supplied windows show and record the rest as an unresolved frontier.
- Unsupported hypotheses are useful; say unsupported rather than inventing support.
- Suggest at most one distinct bounded frontier. Do not request a repeated wording pass.

Targeted evidence bundle JSON:
` + string(encoded)
	return Prompt{Version: PromptVersion, System: system, User: user}, nil
}

func ExecuteRound(ctx context.Context, input ExecuteInput) (ResearchRound, error) {
	round := ResearchRound{
		Version: ContractVersion, ID: input.Plan.Bundle.RoundID,
		Purpose: input.Plan.Bundle.Purpose, Question: input.Plan.Bundle.Question,
		SelectionReason: input.Plan.Gate.Reason, Status: RoundPlanned,
		PromptVersion: PromptVersion, Profile: input.Profile, Model: input.Model,
		Gate:                 input.Plan.Gate,
		LocalFilesInspected:  append([]string(nil), input.Plan.Scope.LocallyInspected...),
		ProviderVisiblePaths: append([]string(nil), input.Plan.Bundle.ProviderAllowedPaths...),
	}
	for _, item := range input.Plan.Bundle.Evidence {
		round.InputEvidenceIDs = append(round.InputEvidenceIDs, item.ID)
	}
	round.InputEvidenceIDs = sortedUnique(round.InputEvidenceIDs)
	if !input.Plan.Gate.Selected {
		round.Status = statusForGate(input.Plan.Gate.Reason)
		round.StopReason = input.Plan.Gate.Reason
		return round, nil
	}
	if input.Provider == nil {
		round.Status = RoundFailed
		round.StopReason = "provider_unavailable"
		return round, fmt.Errorf("model research: provider is required")
	}

	bundleSHA, _, err := BundleHash(input.Plan.Bundle)
	if err != nil {
		return round, err
	}
	round.LocalEvidenceBundleSHA256 = bundleSHA
	prompt, err := BuildPrompt(input.Plan.Bundle)
	if err != nil {
		return round, err
	}
	request, err := input.Provider.BuildResearchRequest(prompt)
	if err != nil {
		return round, fmt.Errorf("model research: build provider request: %w", err)
	}
	round.RequestBytes = len(request)
	round.ProviderRequestSHA256 = requestHash(request)
	if err := persistProviderArtifacts(input.RunDir, input.Plan.Bundle, request, nil); err != nil {
		return round, err
	}
	if allowed, reason := input.Policy.Allows(input.Policy.Targeted, input.Usage, len(request)); !allowed {
		round.Status = RoundBudgetExhausted
		round.StopReason = reason
		round.Gate.Selected = false
		round.Gate.Reason = reason
		return round, nil
	}

	cacheKey, err := CacheKey(FingerprintInput{
		Repository: input.Repository, Stage: "targeted_research", PromptVersion: PromptVersion,
		Profile: input.Profile, Model: input.Model, EvidenceBundleHash: bundleSHA,
		PolicyVersion: input.Policy.Version,
	})
	if err != nil {
		return round, err
	}
	round.CacheKey = cacheKey
	if input.RunsDir != "" {
		record, found, loadErr := loadCache(input.RunsDir, cacheKey, round.ProviderRequestSHA256, bundleSHA)
		if loadErr != nil {
			if !errors.Is(loadErr, ErrInvalidCachedRound) {
				round.Status = RoundRejected
				round.StopReason = "invalid_cached_record"
				return round, loadErr
			}
			found = false
		}
		if found {
			round.Cached = true
			round.Status = RoundCached
			round.ResponseBytes = record.ResponseBytes
			round.InputTokens = record.InputTokens
			round.OutputTokens = record.OutputTokens
			round.PromptCacheHitTokens = record.PromptCacheHitTokens
			round.PromptCacheMissTokens = record.PromptCacheMissTokens
			round.LatencyMillis = record.LatencyMillis
			round.RetryCount = record.RetryCount
			if err := persistProviderArtifacts(input.RunDir, input.Plan.Bundle, request, record.Response); err != nil {
				return round, err
			}
			if err := applyResponse(&round, input.Plan.Bundle, record.Response); err != nil {
				round.Status = RoundRejected
				round.StopReason = "invalid_cached_response"
				return round, err
			}
			round.CompletedAt = nowUTC()
			return round, nil
		}
	}

	started := time.Now()
	result, callErr := input.Provider.Research(ctx, prompt)
	round.LatencyMillis = time.Since(started).Milliseconds()
	round.ResponseBytes = len(result.Content)
	round.InputTokens = result.InputTokens
	round.OutputTokens = result.OutputTokens
	round.PromptCacheHitTokens = result.PromptCacheHitTokens
	round.PromptCacheMissTokens = result.PromptCacheMissTokens
	if result.Attempts > 1 {
		round.RetryCount = result.Attempts - 1
	}
	if callErr != nil {
		round.Status = RoundFailed
		round.StopReason = "provider_call_failed"
		return round, callErr
	}
	if err := persistProviderArtifacts(input.RunDir, input.Plan.Bundle, request, result.Content); err != nil {
		return round, err
	}
	if input.RunsDir != "" {
		record := cacheRecord{
			Version: cacheRecordVersion, CacheKey: cacheKey,
			RequestSHA256: round.ProviderRequestSHA256, BundleSHA256: bundleSHA,
			ResponseSHA256: requestHash(result.Content), Response: append([]byte(nil), result.Content...),
			RequestBytes: len(request), ResponseBytes: len(result.Content),
			InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
			PromptCacheHitTokens:  result.PromptCacheHitTokens,
			PromptCacheMissTokens: result.PromptCacheMissTokens,
			LatencyMillis:         round.LatencyMillis, RetryCount: round.RetryCount,
		}
		if err := saveCache(input.RunsDir, record); err != nil {
			return round, err
		}
	}
	if err := applyResponse(&round, input.Plan.Bundle, result.Content); err != nil {
		round.Status = RoundRejected
		round.StopReason = "invalid_response"
		return round, err
	}
	round.CompletedAt = nowUTC()
	return round, nil
}

func persistProviderArtifacts(runDir string, bundle EvidenceBundle, request, response []byte) error {
	if runDir == "" {
		return nil
	}
	if _, found := secretscan.Detect(string(request)); found {
		return fmt.Errorf("model research: targeted request contains an obvious credential")
	}
	if len(response) > 0 {
		if _, found := secretscan.Detect(string(response)); found {
			return fmt.Errorf("model research: targeted response contains an obvious credential")
		}
	}
	subdir := filepath.Join(runDir, "research", bundle.RoundID)
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("model research: encode run bundle: %w", err)
	}
	if err := writeProtected(filepath.Join(subdir, "evidence_bundle.json"), append(bundleJSON, '\n')); err != nil {
		return err
	}
	if err := writeProtected(filepath.Join(subdir, "request.redacted.json"), request); err != nil {
		return err
	}
	if len(response) > 0 {
		if err := writeProtected(filepath.Join(subdir, "response.raw.json"), response); err != nil {
			return err
		}
	}
	return nil
}

func applyResponse(round *ResearchRound, bundle EvidenceBundle, raw []byte) error {
	var response researchResponse
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("model research: invalid targeted JSON: %w", err)
	}
	known := make(map[string]EvidenceItem, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		known[item.ID] = item
	}
	seenFindings := make(map[string]struct{}, len(response.Findings))
	for _, finding := range response.Findings {
		finding.ID = strings.TrimSpace(finding.ID)
		finding.EvidenceIDs = sortedUnique(finding.EvidenceIDs)
		reason := validateFinding(finding, known, seenFindings)
		if reason != "" {
			round.RejectedFindings = append(round.RejectedFindings, RejectedFinding{Finding: finding, Reason: reason})
			continue
		}
		seenFindings[finding.ID] = struct{}{}
		round.ValidatedFindings = append(round.ValidatedFindings, ValidatedFinding(finding))
	}
	for _, frontier := range response.UnresolvedFrontiers {
		frontier.Question = strings.TrimSpace(frontier.Question)
		frontier.EvidenceIDs = sortedUnique(frontier.EvidenceIDs)
		if frontier.Question == "" || len(frontier.EvidenceIDs) == 0 || hasUnknownID(frontier.EvidenceIDs, known) {
			continue
		}
		round.UnresolvedFrontiers = append(round.UnresolvedFrontiers, frontier)
		break
	}
	round.NewGroundedFactsCount = round.Gate.NewExactEvidence
	if len(round.ValidatedFindings) == 0 && len(round.RejectedFindings) > 0 {
		round.Status = RoundRejected
		round.StopReason = "all_findings_rejected"
		return nil
	}
	if round.Cached {
		round.Status = RoundCached
	} else {
		round.Status = RoundCompleted
	}
	if len(round.ValidatedFindings) == 0 {
		round.StopReason = "no_supported_interpretation"
	} else if len(round.UnresolvedFrontiers) == 0 {
		round.StopReason = "local_evidence_sufficient"
	} else {
		round.StopReason = "bounded_frontier_recorded"
	}
	return nil
}

func validateFinding(finding RawFinding, known map[string]EvidenceItem, seen map[string]struct{}) string {
	if finding.ID == "" || strings.TrimSpace(finding.Interpretation) == "" {
		return "missing_finding_identity_or_interpretation"
	}
	if _, duplicate := seen[finding.ID]; duplicate {
		return "duplicate_finding_id"
	}
	switch finding.HypothesisAssessment {
	case "supported", "unsupported", "mixed":
	default:
		return "invalid_hypothesis_assessment"
	}
	if len(finding.EvidenceIDs) == 0 {
		return "missing_supporting_evidence"
	}
	if hasUnknownID(finding.EvidenceIDs, known) {
		return "unknown_evidence_id"
	}
	text := strings.ToLower(finding.Interpretation + " " + finding.Explanation)
	for _, certaintyUpgrade := range []string{"observed at runtime", "guaranteed", "always executes", "proves runtime order"} {
		if strings.Contains(text, certaintyUpgrade) {
			return "unsupported_certainty_upgrade"
		}
	}
	return ""
}

func hasUnknownID(ids []string, known map[string]EvidenceItem) bool {
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return true
		}
	}
	return false
}

func statusForGate(reason string) RoundStatus {
	switch reason {
	case "no_new_exact_evidence":
		return RoundNoNewEvidence
	case "call_budget_exhausted", "stage_byte_budget_exhausted", "total_byte_budget_exhausted":
		return RoundBudgetExhausted
	default:
		return RoundSkipped
	}
}

func ApplyRound(state *State, plan PlannedRound, round ResearchRound) {
	if state == nil {
		return
	}
	state.Rounds = append(state.Rounds, round)
	if !round.Cached && (round.Status == RoundCompleted || round.Status == RoundRejected || round.Status == RoundFailed) {
		state.Usage.SemanticCalls++
		state.Usage.RequestBytes += round.RequestBytes
	}
	localEvidence := plan.Scope.LocalEvidence
	if len(localEvidence) == 0 {
		localEvidence = plan.Bundle.Evidence
	}
	for _, item := range localEvidence {
		state.Theory.GroundedFacts = appendGroundedFact(state.Theory.GroundedFacts, item)
	}
	inspectedPaths := make(map[string]struct{})
	windowIDs := make(map[string]struct{})
	for _, fact := range state.Theory.GroundedFacts {
		if fact.Location != nil {
			inspectedPaths[fact.Location.Path] = struct{}{}
		}
		if fact.Kind == EvidenceSource {
			windowIDs[fact.ID] = struct{}{}
		}
	}
	state.Coverage.FocusedLocalEvidenceInspected = len(inspectedPaths)
	state.Coverage.TargetedModelEvidenceWindows = len(windowIDs)
	for _, finding := range round.ValidatedFindings {
		state.Theory.AcceptedModelInterpretations = append(state.Theory.AcceptedModelInterpretations, finding)
	}
	state.Theory.RejectedModelClaims = append(state.Theory.RejectedModelClaims, round.RejectedFindings...)
	state.Theory.UnresolvedFrontiers = append(state.Theory.UnresolvedFrontiers, round.UnresolvedFrontiers...)
	if round.Status == RoundCompleted || round.Status == RoundCached || round.Status == RoundRejected {
		state.Theory.ResearchedQuestions = append(state.Theory.ResearchedQuestions, round.Question)
	}
	state.Theory.ResearchedQuestions = sortedUnique(state.Theory.ResearchedQuestions)
	state.UpdatedAt = nowUTC()
}

func appendGroundedFact(facts []EvidenceItem, item EvidenceItem) []EvidenceItem {
	for _, existing := range facts {
		if existing.ID == item.ID {
			return facts
		}
	}
	return append(facts, item)
}

func SortTheory(theory *WorkingTheory) {
	if theory == nil {
		return
	}
	sort.Slice(theory.GroundedFacts, func(i, j int) bool { return theory.GroundedFacts[i].ID < theory.GroundedFacts[j].ID })
	sort.Slice(theory.AcceptedModelInterpretations, func(i, j int) bool {
		return theory.AcceptedModelInterpretations[i].ID < theory.AcceptedModelInterpretations[j].ID
	})
}
