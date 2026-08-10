package modelresearch

import (
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	ContractVersion = 1
	PromptVersion   = "targeted-research-json-v2"
	StateFile       = "model_research.json"
)

type RoundStatus string

const (
	RoundPlanned         RoundStatus = "planned"
	RoundSkipped         RoundStatus = "skipped"
	RoundCompleted       RoundStatus = "completed"
	RoundRejected        RoundStatus = "rejected"
	RoundCached          RoundStatus = "cached"
	RoundNoNewEvidence   RoundStatus = "no_new_evidence"
	RoundBudgetExhausted RoundStatus = "budget_exhausted"
	RoundFailed          RoundStatus = "failed"
)

type EvidenceVisibility string

const (
	VisibilityProviderInitial EvidenceVisibility = "provider_visible_initially"
	VisibilityLocalAfter      EvidenceVisibility = "locally_retrieved_after_orientation"
	VisibilityProviderTarget  EvidenceVisibility = "provider_visible_in_targeted_round"
	VisibilityNeverProvider   EvidenceVisibility = "never_sent_to_provider"
)

type EvidenceKind string

const (
	EvidenceFileSummary EvidenceKind = "file_summary"
	EvidenceDeclaration EvidenceKind = "declaration"
	EvidenceCallsite    EvidenceKind = "callsite"
	EvidenceTransition  EvidenceKind = "transition"
	EvidenceSource      EvidenceKind = "source_window"
	EvidenceTest        EvidenceKind = "test_reference"
	EvidenceFrontier    EvidenceKind = "frontier"
)

type RepositoryContext struct {
	Identity          string `json:"identity"`
	Revision          string `json:"revision"`
	DirtySHA256       string `json:"dirty_sha256,omitempty"`
	Scenario          string `json:"scenario"`
	AnalysisTargetRef string `json:"analysis_target_ref,omitempty"`
}

type ProposedQuestion struct {
	ID                 string   `json:"id"`
	Purpose            string   `json:"purpose"`
	Question           string   `json:"question"`
	CandidateIDs       []string `json:"candidate_ids,omitempty"`
	EvidenceCategories []string `json:"evidence_categories,omitempty"`
}

type FileCandidate struct {
	ID             string              `json:"id"`
	Path           string              `json:"path"`
	Kind           string              `json:"kind,omitempty"`
	Score          int                 `json:"score,omitempty"`
	FocusLocations []evidence.Location `json:"focus_locations,omitempty"`
}

type SourceWindow struct {
	StartLine   int      `json:"start_line"`
	EndLine     int      `json:"end_line"`
	Lines       []string `json:"lines"`
	CodeBearing bool     `json:"code_bearing"`
	Truncated   bool     `json:"truncated,omitempty"`
}

// EvidenceItem is exact local evidence. Interpretation fields do not live in
// this type, so model prose cannot be mistaken for a grounded fact.
type EvidenceItem struct {
	ID         string                `json:"id"`
	Kind       EvidenceKind          `json:"kind"`
	Statement  string                `json:"statement"`
	Location   *evidence.Location    `json:"location,omitempty"`
	Symbol     string                `json:"symbol,omitempty"`
	Relation   string                `json:"relation,omitempty"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
	Window     *SourceWindow         `json:"source_window,omitempty"`
	Visibility []EvidenceVisibility  `json:"visibility"`
}

type EvidenceBundle struct {
	Version              int            `json:"version"`
	PolicyVersion        string         `json:"policy_version"`
	RoundID              string         `json:"round_id"`
	Purpose              string         `json:"purpose"`
	Question             string         `json:"question"`
	ProviderAllowedPaths []string       `json:"provider_allowed_paths"`
	Evidence             []EvidenceItem `json:"evidence"`
	KnownComponentIDs    []string       `json:"known_component_ids,omitempty"`
	KnownSurfaceIDs      []string       `json:"known_surface_ids,omitempty"`
	KnownTraceIDs        []string       `json:"known_trace_ids,omitempty"`
	KnownFrontiers       []Frontier     `json:"known_frontiers,omitempty"`
	WorkingHypothesis    string         `json:"working_hypothesis,omitempty"`
}

type GateDecision struct {
	Selected              bool     `json:"selected"`
	Reason                string   `json:"reason"`
	NewExactEvidence      int      `json:"new_exact_evidence"`
	UnresolvedHighValue   bool     `json:"unresolved_high_value"`
	RuntimeOnly           bool     `json:"runtime_only"`
	RemainingCalls        int      `json:"remaining_calls"`
	RemainingRequestBytes int      `json:"remaining_request_bytes"`
	Signals               []string `json:"signals,omitempty"`
}

type RawFinding struct {
	ID                   string   `json:"id"`
	Interpretation       string   `json:"interpretation"`
	ResponsibilityName   string   `json:"responsibility_name,omitempty"`
	HypothesisAssessment string   `json:"hypothesis_assessment"`
	EvidenceIDs          []string `json:"evidence_ids"`
	Explanation          string   `json:"explanation,omitempty"`
}

type ValidatedFinding struct {
	ID                   string   `json:"id"`
	Interpretation       string   `json:"interpretation"`
	ResponsibilityName   string   `json:"responsibility_name,omitempty"`
	HypothesisAssessment string   `json:"hypothesis_assessment"`
	EvidenceIDs          []string `json:"evidence_ids"`
	Explanation          string   `json:"explanation,omitempty"`
}

type RejectedFinding struct {
	Finding RawFinding `json:"finding"`
	Reason  string     `json:"reason"`
}

type Frontier struct {
	Question         string   `json:"question"`
	EvidenceIDs      []string `json:"evidence_ids,omitempty"`
	EvidenceCategory string   `json:"evidence_category,omitempty"`
	RuntimeOnly      bool     `json:"runtime_only,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

type StageMetrics struct {
	Stage                 string `json:"stage"`
	Status                string `json:"status"`
	RequestBytes          int    `json:"request_bytes,omitempty"`
	ResponseBytes         int    `json:"response_bytes,omitempty"`
	InputTokens           int    `json:"input_tokens,omitempty"`
	OutputTokens          int    `json:"output_tokens,omitempty"`
	PromptCacheHitTokens  int    `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int    `json:"prompt_cache_miss_tokens,omitempty"`
	LatencyMillis         int64  `json:"latency_ms,omitempty"`
	SemanticCalls         int    `json:"semantic_calls,omitempty"`
	RetryCount            int    `json:"retry_count,omitempty"`
	CacheHit              bool   `json:"cache_hit,omitempty"`
	LocalFilesInspected   int    `json:"local_files_inspected,omitempty"`
	EvidenceItemsSent     int    `json:"evidence_items_sent,omitempty"`
	NewGroundedFacts      int    `json:"new_grounded_facts,omitempty"`
	RejectedClaims        int    `json:"rejected_claims,omitempty"`
}

type ResearchRound struct {
	Version                   int                `json:"version"`
	ID                        string             `json:"round_id"`
	Purpose                   string             `json:"purpose"`
	Question                  string             `json:"question"`
	SelectionReason           string             `json:"selection_reason"`
	Status                    RoundStatus        `json:"status"`
	InputEvidenceIDs          []string           `json:"input_evidence_ids"`
	LocalEvidenceBundleSHA256 string             `json:"local_evidence_bundle_sha256"`
	ProviderRequestSHA256     string             `json:"provider_request_sha256,omitempty"`
	CacheKey                  string             `json:"cache_key,omitempty"`
	PromptVersion             string             `json:"prompt_version"`
	Profile                   string             `json:"profile,omitempty"`
	Model                     string             `json:"model,omitempty"`
	RequestBytes              int                `json:"request_bytes,omitempty"`
	ResponseBytes             int                `json:"response_bytes,omitempty"`
	InputTokens               int                `json:"input_tokens,omitempty"`
	OutputTokens              int                `json:"output_tokens,omitempty"`
	PromptCacheHitTokens      int                `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens     int                `json:"prompt_cache_miss_tokens,omitempty"`
	LatencyMillis             int64              `json:"latency_ms,omitempty"`
	RetryCount                int                `json:"retry_count,omitempty"`
	Cached                    bool               `json:"cached"`
	ValidatedFindings         []ValidatedFinding `json:"validated_findings"`
	RejectedFindings          []RejectedFinding  `json:"rejected_findings"`
	NewGroundedFactsCount     int                `json:"new_grounded_facts_count"`
	UnresolvedFrontiers       []Frontier         `json:"unresolved_frontiers"`
	StopReason                string             `json:"stop_reason"`
	Gate                      GateDecision       `json:"value_of_information_gate"`
	LocalFilesInspected       []string           `json:"local_files_inspected"`
	ProviderVisiblePaths      []string           `json:"provider_visible_paths"`
	CompletedAt               string             `json:"completed_at,omitempty"`
}

type WorkingTheory struct {
	Version                      int                `json:"version"`
	Repository                   RepositoryContext  `json:"repository"`
	GroundedFacts                []EvidenceItem     `json:"grounded_facts"`
	AcceptedModelInterpretations []ValidatedFinding `json:"accepted_model_interpretations"`
	RejectedModelClaims          []RejectedFinding  `json:"rejected_model_claims"`
	ResearchedQuestions          []string           `json:"researched_questions"`
	UnresolvedFrontiers          []Frontier         `json:"unresolved_frontiers"`
	RelatedComponentIDs          []string           `json:"related_component_ids,omitempty"`
	RelatedSurfaceIDs            []string           `json:"related_surface_ids,omitempty"`
	RelatedTraceIDs              []string           `json:"related_trace_ids,omitempty"`
}

type Coverage struct {
	LocalAuthorizedFiles          int `json:"local_authorized_files"`
	InitialModelSummaries         int `json:"initial_model_summaries"`
	FocusedLocalEvidenceInspected int `json:"focused_local_evidence_inspected"`
	TargetedModelEvidenceWindows  int `json:"targeted_model_evidence_windows"`
}

type State struct {
	Version       int               `json:"version"`
	Policy        Policy            `json:"policy"`
	Repository    RepositoryContext `json:"repository"`
	Orientation   StageMetrics      `json:"orientation"`
	Rounds        []ResearchRound   `json:"targeted_rounds"`
	SkippedRounds []ResearchRound   `json:"skipped_targeted_rounds,omitempty"`
	Architecture  StageMetrics      `json:"architecture_synthesis"`
	GuidedTour    StageMetrics      `json:"guided_tour"`
	Theory        WorkingTheory     `json:"working_theory"`
	Coverage      Coverage          `json:"coverage"`
	Usage         Usage             `json:"usage"`
	UpdatedAt     string            `json:"updated_at"`
}

func NewState(policy Policy, repository RepositoryContext) State {
	return State{
		Version:    ContractVersion,
		Policy:     policy,
		Repository: repository,
		Theory:     WorkingTheory{Version: ContractVersion, Repository: repository},
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

func (s State) Validate() error {
	if s.Version != ContractVersion || s.Theory.Version != ContractVersion {
		return fmt.Errorf("model research: unsupported state version")
	}
	if err := s.Policy.Validate(); err != nil {
		return err
	}
	if len(s.Rounds) > s.Policy.MaxTargetedRounds {
		return fmt.Errorf("model research: %d targeted rounds exceeds limit %d", len(s.Rounds), s.Policy.MaxTargetedRounds)
	}
	if s.Usage.SemanticCalls > s.Policy.MaxSemanticCalls {
		return fmt.Errorf("model research: semantic call count exceeds policy")
	}
	if s.Usage.RequestBytes > s.Policy.MaxTotalRequestBytes {
		return fmt.Errorf("model research: request bytes exceed policy")
	}
	return nil
}
