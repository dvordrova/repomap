// Package semanticdiscovery defines the bounded, replayable semantic layer
// built from already saved repository facts. It performs no repository I/O and
// contains no model or network client.
package semanticdiscovery

const (
	BundleVersion              = 1
	OpportunityProposalVersion = 1
	LeafTaskVersion            = 1
	LeafArtifactVersion        = 1
	FanInArtifactVersion       = 1
	ArtifactVersion            = 1
	RecordVersion              = 2

	OpportunityPromptVersion       = "semantic-opportunity-json-v10"
	LeafPromptVersion              = "semantic-leaf-json-v3"
	FanInPromptVersion             = "semantic-fan-in-json-v7"
	MonolithicPromptVersion        = "semantic-monolithic-json-v6"
	GoldenMechanismPromptVersionV3 = "semantic-golden-mechanism-json-v3"
	GoldenMechanismPromptVersion   = "semantic-golden-mechanism-json-v4"
	OnboardingEditorPromptVersion  = "repository-onboarding-editor-json-v1"
	StudyMapPromptVersion          = "repository-study-map-json-v1"
	StudyBriefPromptVersion        = "repository-brief-shape-json-v3"
	StudyCandidatesPromptVersion   = "repository-study-candidates-json-v5"
	ReadingPackReviewPromptVersion = "repository-reading-pack-review-json-v2"

	RecordFile               = "semantic_artifacts.json"
	MaxSelectedCandidates    = 5
	MaxOpportunityCandidates = 20
)

type ArtifactKind string

const (
	ArtifactMechanism         ArtifactKind = "mechanism"
	ArtifactDependencyUsage   ArtifactKind = "dependency_usage"
	ArtifactRepositoryPattern ArtifactKind = "repository_pattern"
	ArtifactContributionGuide ArtifactKind = "contribution_guide"
	ArtifactGoLearning        ArtifactKind = "go_learning"
	ArtifactRepositoryStory   ArtifactKind = "repository_story"
)

type ExpectedValue string

const (
	ExpectedValueLow    ExpectedValue = "low"
	ExpectedValueMedium ExpectedValue = "medium"
	ExpectedValueHigh   ExpectedValue = "high"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type FactKind string

const (
	FactRepositoryPurpose FactKind = "repository_purpose"
	FactComponent         FactKind = "component"
	FactFlow              FactKind = "flow"
	FactFlowStep          FactKind = "flow_step"
	FactRuntimeSurface    FactKind = "runtime_surface"
	FactPackageImport     FactKind = "package_import"
	FactDependency        FactKind = "dependency"
	FactREADMEClaim       FactKind = "readme_claim"
	FactDomainTerm        FactKind = "domain_term"
	FactTestReference     FactKind = "test_reference"
	FactSourceSignal      FactKind = "source_signal"
	FactGuidedStep        FactKind = "guided_step"
	FactWarning           FactKind = "warning"
	FactUnknown           FactKind = "unknown"
)

type Capability string

const (
	CapabilityStatic             Capability = "static"
	CapabilityBehavior           Capability = "behavior"
	CapabilitySequence           Capability = "sequence"
	CapabilityLimitation         Capability = "limitation"
	CapabilityEntry              Capability = "entry"
	CapabilityDirectCall         Capability = "direct_call"
	CapabilityBranch             Capability = "branch"
	CapabilityDataRead           Capability = "data_read"
	CapabilityDataWrite          Capability = "data_write"
	CapabilityDataTransformation Capability = "data_transformation"
	CapabilityOutputEffect       Capability = "output_effect"
	CapabilityErrorPath          Capability = "error_path"
	CapabilityTestEvidence       Capability = "test_evidence"
	CapabilityOwnership          Capability = "ownership"
	CapabilityLifecycle          Capability = "lifecycle"
)

type CapabilityResolution string

const (
	CapabilityResolutionRequiresProbe        CapabilityResolution = "requires_probe"
	CapabilityResolutionReady                CapabilityResolution = "ready"
	CapabilityResolutionPartial              CapabilityResolution = "partial"
	CapabilityResolutionInsufficientEvidence CapabilityResolution = "insufficient_evidence"
)

type CapabilityContract struct {
	RequiredCapabilities  []Capability         `json:"required_capabilities"`
	AvailableCapabilities []Capability         `json:"available_capabilities"`
	MissingCapabilities   []Capability         `json:"missing_capabilities"`
	Resolution            CapabilityResolution `json:"resolution"`
}

type AnswerAspect struct {
	ID                   string       `json:"id"`
	Label                string       `json:"label"`
	RequiredCapabilities []Capability `json:"required_capabilities"`
	Key                  bool         `json:"key,omitempty"`
}

type IntentContract struct {
	RequiredAnswerAspects []AnswerAspect `json:"required_answer_aspects"`
	MinCovered            int            `json:"min_covered"`
	MinKeyCovered         int            `json:"min_key_covered"`
	LocalSearchAliases    []string       `json:"local_search_aliases"`
}

type FactScope string

const (
	FactScopeLocal      FactScope = "local"
	FactScopeComponent  FactScope = "component"
	FactScopeFlow       FactScope = "flow"
	FactScopeRepository FactScope = "repository"
)

// PlannerContextKind identifies saved editorial artifacts that may help the
// opportunity scan choose a useful question. Planner context is deliberately
// separate from Facts: it has no opaque evidence ID and can never be cited by
// a leaf, fan-in claim, or replayed semantic artifact.
type PlannerContextKind string

const (
	PlannerContextOrientation PlannerContextKind = "orientation"
	PlannerContextComponent   PlannerContextKind = "component"
	PlannerContextFlow        PlannerContextKind = "flow"
	PlannerContextGuidedTour  PlannerContextKind = "guided_tour"
	PlannerContextVocabulary  PlannerContextKind = "vocabulary"
	PlannerContextResearch    PlannerContextKind = "research"
	PlannerContextLimitation  PlannerContextKind = "limitation"
)

type PlannerContext struct {
	Kind PlannerContextKind `json:"kind"`
	Text string             `json:"text"`
}

// Focus contains only IDs copied from the saved report model.
type Focus struct {
	ComponentIDs []string `json:"component_ids,omitempty"`
	FlowIDs      []string `json:"flow_ids,omitempty"`
	SurfaceIDs   []string `json:"surface_ids,omitempty"`
}

// EvidenceRef is local navigation evidence. Model responses never contain
// this type; materialization copies it from supporting Facts.
type EvidenceRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// FactSource binds a fact to one exact, bounded repository source window.
// It is local provenance and is never included in provider requests.
type FactSource struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	EnclosingSymbol string `json:"enclosing_symbol"`
	ContentSHA256   string `json:"content_sha256"`
}

// Fact is one locally assembled, model-visible statement derived from
// deterministic extraction or an exact local proof. SourceGroup is a stable
// opaque identity for an independently collected evidence family. Source is
// canonical local provenance and is stripped before a model sees the Fact.
type Fact struct {
	ID           string        `json:"id"`
	Kind         FactKind      `json:"kind"`
	Statement    string        `json:"statement"`
	Keywords     []string      `json:"keywords,omitempty"`
	SourceGroup  string        `json:"source_group"`
	Capabilities []Capability  `json:"capabilities"`
	Scope        FactScope     `json:"scope"`
	Source       *FactSource   `json:"source,omitempty"`
	Focus        Focus         `json:"focus,omitempty"`
	Evidence     []EvidenceRef `json:"evidence,omitempty"`
}

type Bundle struct {
	Version        int              `json:"version"`
	RepoName       string           `json:"repo_name"`
	PlannerContext []PlannerContext `json:"planner_context,omitempty"`
	Facts          []Fact           `json:"facts"`
}

type OpportunityProposal struct {
	Version    int                    `json:"version"`
	Candidates []OpportunityCandidate `json:"candidates"`
}

// OpportunityKind describes the product use of a mechanism candidate. It is
// editorial planning metadata, not a new semantic artifact kind.
type OpportunityKind string

const (
	OpportunityKindCentralBehavior     OpportunityKind = "central_behavior"
	OpportunityKindQuestionPath        OpportunityKind = "question_path"
	OpportunityKindExtensionPath       OpportunityKind = "extension_path"
	OpportunityKindMaintenanceBoundary OpportunityKind = "maintenance_boundary"
)

// OpportunityUserJob binds a candidate to one concrete unfamiliar-repository
// task without changing the candidate's semantic truth contract.
type OpportunityUserJob string

const (
	OpportunityUserJobFirstContact OpportunityUserJob = "first_contact_onboarding"
	OpportunityUserJobExploration  OpportunityUserJob = "question_driven_exploration"
	OpportunityUserJobContribution OpportunityUserJob = "contribution_extension"
	OpportunityUserJobMaintenance  OpportunityUserJob = "maintenance_rewrite"
)

type OpportunityEstimatedCost string

const (
	OpportunityEstimatedCostLow    OpportunityEstimatedCost = "low"
	OpportunityEstimatedCostMedium OpportunityEstimatedCost = "medium"
	OpportunityEstimatedCostHigh   OpportunityEstimatedCost = "high"
)

// OpportunityExpectation is a bounded answer-shape proposal. Description is
// editorial; SupportIDs and RequiredCapabilities are validated against local
// facts before the candidate can be selected.
type OpportunityExpectation struct {
	Description          string       `json:"description"`
	SupportIDs           []string     `json:"support_ids"`
	RequiredCapabilities []Capability `json:"required_capabilities"`
}

type OpportunityExpectedPath struct {
	InputTrigger     OpportunityExpectation `json:"input_trigger"`
	CoreWork         OpportunityExpectation `json:"core_work"`
	ObservableEffect OpportunityExpectation `json:"observable_effect"`
}

// OpportunityFrontier is a model-proposed bounded local investigation need.
// It can only point back to supplied fact IDs and known capability values.
type OpportunityFrontier struct {
	FromAnchorIDs       []string     `json:"from_anchor_ids"`
	DesiredCapabilities []Capability `json:"desired_capabilities"`
	Rationale           string       `json:"rationale"`
}

// OpportunityProductIntent carries locally checkable product-planning
// metadata. It is optional for backward-compatible saved candidates; the
// normal fresh-repository v3 path requires it before local selection.
type OpportunityProductIntent struct {
	OpportunityKind           OpportunityKind          `json:"opportunity_kind"`
	TargetUserJob             OpportunityUserJob       `json:"target_user_job"`
	CentralAnchorIDs          []string                 `json:"central_anchor_ids"`
	ExpectedPath              OpportunityExpectedPath  `json:"expected_path"`
	ArchitectureAreaAnchorIDs []string                 `json:"architecture_area_anchor_ids"`
	BoundedFrontier           []OpportunityFrontier    `json:"bounded_frontier"`
	OnboardingRationale       string                   `json:"onboarding_rationale"`
	InvestigationRationale    string                   `json:"investigation_rationale"`
	EstimatedCost             OpportunityEstimatedCost `json:"estimated_cost"`
	SearchQueries             []string                 `json:"search_queries,omitempty"`
}

// ID is assigned locally during normalization. The opportunity model leaves
// it empty and can only select supplied Fact IDs.
type OpportunityCandidate struct {
	ID                   string                    `json:"id,omitempty"`
	Kind                 ArtifactKind              `json:"kind"`
	Title                string                    `json:"title"`
	QuestionAnswered     string                    `json:"question_answered"`
	SupportIDs           []string                  `json:"support_ids"`
	EnrichmentSupportIDs []string                  `json:"enrichment_support_ids,omitempty"`
	MissingInformation   []string                  `json:"missing_information"`
	ExpectedValue        ExpectedValue             `json:"expected_value"`
	Confidence           Confidence                `json:"confidence"`
	CapabilityContract   *CapabilityContract       `json:"capability_contract,omitempty"`
	IntentContract       *IntentContract           `json:"intent_contract,omitempty"`
	ProductIntent        *OpportunityProductIntent `json:"product_intent,omitempty"`
}

// promptCandidateScope is the only opportunity output forwarded to later
// model stages. Editorial title, question, and missing-information prose stay
// local so a leaf can never treat a previous model's wording as evidence.
type promptCandidateScope struct {
	ID   string       `json:"id"`
	Kind ArtifactKind `json:"artifact_kind"`
}

type NormalizationIssue struct {
	CandidateIndex int    `json:"candidate_index"`
	Code           string `json:"code"`
	Detail         string `json:"detail,omitempty"`
}

type NormalizationReport struct {
	Issues []NormalizationIssue `json:"issues,omitempty"`
}

type LeafTask struct {
	Version   int                  `json:"version"`
	ID        string               `json:"id"`
	Candidate OpportunityCandidate `json:"candidate"`
	Facts     []Fact               `json:"facts"`
}

type LeafStatus string

const (
	LeafStatusUsable               LeafStatus = "usable"
	LeafStatusInsufficientEvidence LeafStatus = "insufficient_evidence"
)

type LeafArtifact struct {
	Version             int                     `json:"version"`
	TaskID              string                  `json:"task_id"`
	CandidateID         string                  `json:"candidate_id"`
	Status              LeafStatus              `json:"status"`
	Observations        []LeafObservation       `json:"observations"`
	CandidateConnection LeafCandidateConnection `json:"candidate_connection"`
	Contradictions      []LeafContradiction     `json:"contradictions"`
	MissingEvidence     []LeafMissingEvidence   `json:"missing_evidence"`
}

type LeafObservation struct {
	Text       string   `json:"text"`
	SupportIDs []string `json:"support_ids"`
}

type LeafCandidateConnection struct {
	CandidateID string   `json:"candidate_id"`
	Relation    string   `json:"relation"`
	Explanation string   `json:"explanation"`
	SupportIDs  []string `json:"support_ids"`
}

type LeafContradiction struct {
	Explanation string   `json:"explanation"`
	SupportIDs  []string `json:"support_ids"`
}

type LeafMissingEvidence struct {
	Explanation         string       `json:"explanation"`
	SupportIDs          []string     `json:"support_ids"`
	MissingCapabilities []Capability `json:"missing_capabilities,omitempty"`
}

type LeafResult struct {
	Task     LeafTask     `json:"task"`
	Artifact LeafArtifact `json:"artifact"`
}

type LeafReductionIssue struct {
	Section string `json:"section"`
	Index   int    `json:"index"`
	Code    string `json:"code"`
}

type LeafReductionReport struct {
	Issues                 []LeafReductionIssue `json:"issues,omitempty"`
	KeptObservations       int                  `json:"kept_observations"`
	KeptContradictions     int                  `json:"kept_contradictions"`
	KeptMissingEvidence    int                  `json:"kept_missing_evidence"`
	DroppedObservations    int                  `json:"dropped_observations"`
	DroppedContradictions  int                  `json:"dropped_contradictions"`
	DroppedMissingEvidence int                  `json:"dropped_missing_evidence"`
}

type Verdict string

const (
	VerdictSupported            Verdict = "supported"
	VerdictMixed                Verdict = "mixed"
	VerdictInsufficientEvidence Verdict = "insufficient_evidence"
	VerdictUnsupported          Verdict = "unsupported"
)

type ClaimBasis string

const (
	ClaimDirect        ClaimBasis = "direct"
	ClaimCompositional ClaimBasis = "compositional"
	ClaimInterpretive  ClaimBasis = "interpretive"
	ClaimUnresolved    ClaimBasis = "unresolved"
)

type ObservationRef struct {
	TaskID           string `json:"task_id"`
	ObservationIndex int    `json:"observation_index"`
}

type MissingEvidenceRef struct {
	TaskID       string `json:"task_id"`
	MissingIndex int    `json:"missing_index"`
}

type ProposedClaim struct {
	Title           string               `json:"title"`
	Text            string               `json:"text"`
	Basis           ClaimBasis           `json:"basis"`
	SupportIDs      []string             `json:"support_ids"`
	ObservationRefs []ObservationRef     `json:"observation_refs,omitempty"`
	MissingRefs     []MissingEvidenceRef `json:"missing_refs,omitempty"`
}

type ArtifactProposal struct {
	CandidateID         string          `json:"candidate_id"`
	Verdict             Verdict         `json:"verdict"`
	Title               string          `json:"title"`
	Summary             string          `json:"summary"`
	Claims              []ProposedClaim `json:"claims"`
	Aliases             []string        `json:"aliases,omitempty"`
	LikelyQuestions     []string        `json:"likely_questions,omitempty"`
	RelatedCandidateIDs []string        `json:"related_candidate_ids,omitempty"`
}

type FanInArtifact struct {
	Version   int                `json:"version"`
	Artifacts []ArtifactProposal `json:"artifacts"`
}

type FanInReductionIssue struct {
	ArtifactIndex int                    `json:"artifact_index"`
	Code          string                 `json:"code"`
	Reasons       []FanInReductionReason `json:"reasons,omitempty"`
}

// FanInReductionReason is a bounded, non-prose diagnostic for one rejected
// proposal. It intentionally omits model text and repository values so debug
// artifacts can explain validation without echoing secrets or unsafe paths.
type FanInReductionReason struct {
	Code       string   `json:"code"`
	Field      string   `json:"field,omitempty"`
	ClaimIndex *int     `json:"claim_index,omitempty"`
	SupportIDs []string `json:"support_ids,omitempty"`
}

type FanInReductionReport struct {
	Issues             []FanInReductionIssue `json:"issues,omitempty"`
	VerdictDiagnostics []VerdictDiagnostic   `json:"verdict_diagnostics,omitempty"`
	KeptArtifacts      int                   `json:"kept_artifacts"`
	DroppedArtifacts   int                   `json:"dropped_artifacts"`
}

type VerdictDiagnostic struct {
	Code           string          `json:"code"`
	ArtifactIndex  int             `json:"artifact_index"`
	CandidateID    string          `json:"candidate_id"`
	ModelVerdict   Verdict         `json:"model_verdict"`
	DerivedVerdict Verdict         `json:"derived_verdict"`
	Reasons        []VerdictReason `json:"reasons"`
}

// Artifact is the locally materialized report/search object. Repository IDs,
// focus, evidence, statement IDs, and step IDs are all derived locally.
type Artifact struct {
	Version                int           `json:"version"`
	ID                     string        `json:"id"`
	CandidateID            string        `json:"candidate_id"`
	Kind                   ArtifactKind  `json:"kind"`
	Title                  string        `json:"title"`
	Summary                string        `json:"summary"`
	Question               string        `json:"question"`
	Verdict                Verdict       `json:"verdict"`
	Statements             []Statement   `json:"statements"`
	Steps                  []Step        `json:"steps"`
	Focus                  Focus         `json:"focus,omitempty"`
	Evidence               []EvidenceRef `json:"evidence,omitempty"`
	Aliases                []string      `json:"aliases,omitempty"`
	LikelyQuestions        []string      `json:"likely_questions,omitempty"`
	Unknowns               []string      `json:"unknowns,omitempty"`
	Confidence             Confidence    `json:"confidence"`
	RequiredAspectIDs      []string      `json:"required_answer_aspects,omitempty"`
	CoveredAspectIDs       []string      `json:"covered_answer_aspects,omitempty"`
	UncoveredAspectIDs     []string      `json:"uncovered_answer_aspects,omitempty"`
	UsedFactIDs            []string      `json:"used_fact_ids,omitempty"`
	UnusedAvailableFactIDs []string      `json:"unused_available_fact_ids,omitempty"`
	RelatedArtifactIDs     []string      `json:"related_artifact_ids,omitempty"`
}

type Statement struct {
	ID           string     `json:"id"`
	Text         string     `json:"text"`
	Basis        ClaimBasis `json:"basis"`
	SupportIDs   []string   `json:"support_ids,omitempty"`
	SourceGroups []string   `json:"source_groups,omitempty"`
	AspectIDs    []string   `json:"answer_aspect_ids,omitempty"`
}

type Step struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Explanation  string        `json:"explanation"`
	StatementIDs []string      `json:"statement_ids"`
	Focus        Focus         `json:"focus,omitempty"`
	Evidence     []EvidenceRef `json:"evidence,omitempty"`
}

type ThinkingProfile string

const (
	ThinkingDisabled ThinkingProfile = "disabled"
	ThinkingHigh     ThinkingProfile = "high"
	ThinkingMax      ThinkingProfile = "max"
)

type Prompt struct {
	Version         string          `json:"version"`
	System          string          `json:"system"`
	User            string          `json:"user"`
	ThinkingProfile ThinkingProfile `json:"thinking_profile"`
	ProgressLabel   string          `json:"-"`
}

type Record struct {
	Version              int                 `json:"version"`
	BundleSHA256         string              `json:"bundle_sha256"`
	Opportunity          OpportunityProposal `json:"opportunity"`
	SelectedCandidateIDs []string            `json:"selected_candidate_ids"`
	Leaves               []LeafResult        `json:"leaves"`
	FanIn                FanInArtifact       `json:"fan_in"`
}
