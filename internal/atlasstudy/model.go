// Package atlasstudy defines the Atlas-first repository Brief and Study
// contract. It keeps canonical repository identities private while exposing a
// bounded task-shaped reading catalog with exact paths, lines, optional
// qualified symbols and short typed refs to a model provider.
package atlasstudy

import (
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

const (
	// Version and PromptVersion own the byte-identical provider request and
	// private request catalog contract.
	Version       = 7
	PromptVersion = "atlas-study-prompt-v13"

	// ResultVersion owns local response validation plus result/status replay.
	// It advances independently so the unchanged v6 provider request cannot
	// reinterpret a result accepted by the earlier question validator.
	ResultVersion = 8

	RequestArtifactFilename = "atlas_study_request.v7.json"
	ResultArtifactFilename  = "atlas_study_result.v8.json"
	StatusArtifactFilename  = "atlas_study_status.v8.json"

	MaxRequestArtifactBytes = 16 << 20
	MaxResultArtifactBytes  = 16 << 20
	// Status repeats the complete exact CandidateCoverage from the request.
	// Its bound must therefore be able to represent every request-encodable
	// compiled product without trimming package buckets after a provider call.
	MaxStatusArtifactBytes = MaxRequestArtifactBytes

	// MaxDirections is the model-output ceiling: the maximum number of accepted
	// directions and the ceiling for the returned directions array. The desired
	// direction count is MinPortfolioDirections..MaxDirections; the valid
	// production cardinality is 1..MaxDirections and zero valid directions is a
	// failure. The old advertised-span meaning of MaxRouteSpans is removed; the
	// request frontier has exactly one budget, MaxAdvertisedSpans.
	MaxDirections = 10

	// MinPortfolioDirections is the desired lower bound of the ranked Study
	// portfolio. A valid response with fewer directions is accepted as-is: the
	// backend never forces filler or padding.
	MinPortfolioDirections = 6

	MinDirectionReadingCount = 1
	MaxDirectionReadingCount = 5
	MaxDirectionDiagnostics  = 12
	MaxDomainTerms           = 8
	MaxDomainTermDiagnostics = 12

	// MaxOmissionRepresentatives bounds the representative typed refs recorded
	// for each closed omission reason. The complete candidate digest already
	// binds the full considered set, so an unbounded omission list is never
	// persisted.
	MaxOmissionRepresentatives = 12
)

type Language string

const (
	LanguageEnglish Language = "en"
	LanguageRussian Language = "ru"
)

func (language Language) Valid() bool {
	return language == LanguageEnglish || language == LanguageRussian
}

// Limits are explicit producer-owned ceilings. Compile never trims a valid
// section to make it fit.
type Limits struct {
	MaxWireBytes      int `json:"max_wire_bytes"`
	MaxResponseBytes  int `json:"max_response_bytes"`
	MaxTextBytes      int `json:"max_text_bytes"`
	MaxUnits          int `json:"max_units"`
	MaxSubsystems     int `json:"max_subsystems"`
	MaxComponents     int `json:"max_components"`
	MaxSurfaces       int `json:"max_surfaces"`
	MaxReadingTargets int `json:"max_reading_targets"`
	// MaxAdvertisedSpans is the one unambiguous advertised-span limit: the
	// request frontier budget. The complete considered span set is never
	// trimmed to it; selection order is a request-budget mechanism, never
	// semantic importance.
	MaxAdvertisedSpans int `json:"max_advertised_spans"`
	MaxEvidence        int `json:"max_evidence"`
	MaxDocuments       int `json:"max_documents"`
}

func DefaultLimits() Limits {
	return Limits{
		MaxWireBytes: 1 << 20, MaxResponseBytes: 16 << 20, MaxTextBytes: 4096,
		MaxUnits: 512, MaxSubsystems: 64, MaxComponents: 256,
		MaxSurfaces: 128, MaxReadingTargets: 512, MaxAdvertisedSpans: 32, MaxEvidence: 512,
		MaxDocuments: 64,
	}
}

type RefKind string

const (
	RefUnit          RefKind = "unit"
	RefSubsystem     RefKind = "subsystem"
	RefComponent     RefKind = "component"
	RefSurface       RefKind = "surface"
	RefReadingTarget RefKind = "reading_target"
	RefEvidence      RefKind = "evidence"
	RefDocument      RefKind = "document"
	RefRouteSupport  RefKind = "route_support"
	RefRouteRelation RefKind = "route_relation"
	RefRouteSpan     RefKind = "route_span"
)

func (kind RefKind) Valid() bool {
	switch kind {
	case RefUnit, RefSubsystem, RefComponent, RefSurface, RefReadingTarget,
		RefEvidence, RefDocument, RefRouteSupport, RefRouteRelation, RefRouteSpan:
		return true
	default:
		return false
	}
}

type CanonicalRef struct {
	Kind RefKind `json:"kind"`
	ID   string  `json:"id"`
}

// CatalogObject is persisted only in the local request artifact. It restores
// a request-local ref to one canonical identity and, for reading targets, the
// exact source locator needed by the report source drawer.
type CatalogObject struct {
	Ref                  string                    `json:"ref"`
	Kind                 RefKind                   `json:"kind"`
	CanonicalID          string                    `json:"canonical_id"`
	Label                string                    `json:"label,omitempty"`
	Fact                 string                    `json:"fact,omitempty"`
	Authority            repositoryatlas.Authority `json:"authority"`
	Owner                *CanonicalRef             `json:"owner,omitempty"`
	RelatedComponentRefs []CanonicalRef            `json:"related_component_refs,omitempty"`
	PrincipalRefs        []CanonicalRef            `json:"principal_refs,omitempty"`
	Location             *evidence.Location        `json:"location,omitempty"`
	Symbol               string                    `json:"symbol,omitempty"`
	SupportRole          SupportRole               `json:"support_role,omitempty"`
	SupportTarget        *CanonicalRef             `json:"support_target,omitempty"`
	PackageBucket        string                    `json:"package_bucket,omitempty"`
	SpanKind             RouteSpanKind             `json:"span_kind,omitempty"`
	Question             string                    `json:"question,omitempty"`
	TargetJob            TargetJob                 `json:"target_job,omitempty"`
	LearningStage        LearningStage             `json:"learning_stage,omitempty"`
	RequiredSupportRefs  []CanonicalRef            `json:"required_support_refs,omitempty"`
	AllowedTargetRefs    []CanonicalRef            `json:"allowed_target_refs,omitempty"`
	SpanJoins            []CanonicalSpanJoin       `json:"span_joins,omitempty"`
	RelationKind         RouteProducerRelationKind `json:"relation_kind,omitempty"`
	ProducerID           string                    `json:"producer_id,omitempty"`
	FromSupport          *CanonicalRef             `json:"from_support,omitempty"`
	ToSupport            *CanonicalRef             `json:"to_support,omitempty"`
	FromTarget           *CanonicalRef             `json:"from_target,omitempty"`
	ToTarget             *CanonicalRef             `json:"to_target,omitempty"`
	SavedFlowID          string                    `json:"saved_flow_id,omitempty"`
	FromStepID           string                    `json:"from_step_id,omitempty"`
	ToStepID             string                    `json:"to_step_id,omitempty"`
	FromStepOrdinal      int                       `json:"from_step_ordinal,omitempty"`
	ToStepOrdinal        int                       `json:"to_step_ordinal,omitempty"`
}

type ArchitectureInput struct {
	Version    int         `json:"version"`
	Source     string      `json:"source"`
	Title      string      `json:"title,omitempty"`
	Subtitle   string      `json:"subtitle,omitempty"`
	Subsystems []Subsystem `json:"subsystems"`
	Components []Component `json:"components"`
}

type Subsystem struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	Authority    repositoryatlas.Authority `json:"authority"`
	ComponentIDs []string                  `json:"component_ids"`
}

type Component struct {
	ID               string                    `json:"id"`
	SubsystemID      string                    `json:"subsystem_id"`
	Name             string                    `json:"name"`
	Description      string                    `json:"description,omitempty"`
	Authority        repositoryatlas.Authority `json:"authority"`
	ReadingTargetIDs []string                  `json:"reading_target_ids"`
}

type Surface struct {
	ID               string                    `json:"id"`
	UnitID           string                    `json:"unit_id"`
	Name             string                    `json:"name"`
	Kind             string                    `json:"kind"`
	Authority        repositoryatlas.Authority `json:"authority"`
	ReadingTargetIDs []string                  `json:"reading_target_ids"`
}

type ReadingTargetKind string

const (
	ReadingTargetEntrypoint ReadingTargetKind = "entrypoint"
	ReadingTargetFunction   ReadingTargetKind = "function"
	ReadingTargetMethod     ReadingTargetKind = "method"
	ReadingTargetType       ReadingTargetKind = "type"
	ReadingTargetFile       ReadingTargetKind = "file"
	ReadingTargetDocument   ReadingTargetKind = "document"
)

func (kind ReadingTargetKind) Valid() bool {
	switch kind {
	case ReadingTargetEntrypoint, ReadingTargetFunction, ReadingTargetMethod,
		ReadingTargetType, ReadingTargetFile, ReadingTargetDocument:
		return true
	default:
		return false
	}
}

// ReadingTarget retains the canonical identity and complete locator in the
// private request artifact. The model-visible wire receives only the bounded
// repository-relative path, focus line and optional exact symbol beside a
// short request-local ref; responses may return only that ref.
type ReadingTarget struct {
	ID                  string                    `json:"id"`
	Owner               CanonicalRef              `json:"owner,omitempty"`
	RelatedComponentIDs []string                  `json:"related_component_ids,omitempty"`
	PrincipalRefs       []CanonicalRef            `json:"principal_refs"`
	Kind                ReadingTargetKind         `json:"kind"`
	Label               string                    `json:"label"`
	Fact                string                    `json:"fact"`
	Authority           repositoryatlas.Authority `json:"authority"`
	Location            evidence.Location         `json:"location"`
	Symbol              string                    `json:"symbol,omitempty"`
}

// SupportRole is a producer-owned evidence shape. It is deliberately not an
// application-domain label and never implies runtime order or ownership.
type SupportRole string

const (
	SupportProcessEntry         SupportRole = "process_entry"
	SupportEntryHandoff         SupportRole = "entry_handoff"
	SupportSurface              SupportRole = "surface"
	SupportSurfaceCandidate     SupportRole = "surface_candidate"
	SupportObservedCallBoundary SupportRole = "observed_call_boundary"
	SupportSavedFlow            SupportRole = "saved_flow"
)

func (role SupportRole) Valid() bool {
	switch role {
	case SupportProcessEntry, SupportEntryHandoff, SupportSurface,
		SupportSurfaceCandidate, SupportObservedCallBoundary, SupportSavedFlow:
		return true
	default:
		return false
	}
}

// ReadingSupport binds one exact producer proof to one exact reading target.
// PackageBucket is a canonical private package identity used only for bounded
// breadth selection; it is never model-authored authority.
type ReadingSupport struct {
	ID            string                    `json:"id"`
	TargetID      string                    `json:"target_id"`
	PackageBucket string                    `json:"package_bucket"`
	Role          SupportRole               `json:"role"`
	Authority     repositoryatlas.Authority `json:"authority"`
}

type RouteSpanKind string

const (
	RouteSpanFocused    RouteSpanKind = "focused"
	RouteSpanSystemPath RouteSpanKind = "system_path"
)

func (kind RouteSpanKind) Valid() bool {
	return kind == RouteSpanFocused || kind == RouteSpanSystemPath
}

type RouteProducerRelationKind string

const (
	RouteRelationEntryHandoff  RouteProducerRelationKind = "entry_handoff"
	RouteRelationSavedFlowEdge RouteProducerRelationKind = "saved_flow_edge"
)

func (kind RouteProducerRelationKind) Valid() bool {
	return kind == RouteRelationEntryHandoff || kind == RouteRelationSavedFlowEdge
}

type RouteSpanJoin struct {
	RelationID string `json:"relation_id"`
}

type CanonicalSpanJoin struct {
	Relation CanonicalRef `json:"relation"`
}

// RouteProducerRelation is a private exact producer edge. It never appears on
// the provider wire and is the only authority for a system-path join.
type RouteProducerRelation struct {
	ID              string                    `json:"id"`
	Kind            RouteProducerRelationKind `json:"kind"`
	ProducerID      string                    `json:"producer_id"`
	FromSupportID   string                    `json:"from_support_id"`
	ToSupportID     string                    `json:"to_support_id"`
	FromTargetID    string                    `json:"from_target_id"`
	ToTargetID      string                    `json:"to_target_id"`
	SavedFlowID     string                    `json:"saved_flow_id,omitempty"`
	FromStepID      string                    `json:"from_step_id,omitempty"`
	ToStepID        string                    `json:"to_step_id,omitempty"`
	FromStepOrdinal int                       `json:"from_step_ordinal,omitempty"`
	ToStepOrdinal   int                       `json:"to_step_ordinal,omitempty"`
}

// RouteSpan is a backend-owned promise. Questions are localized presentation;
// canonical direction identity is derived only from the span ID and exact
// canonical refs.
type RouteSpan struct {
	ID                 string          `json:"id"`
	Kind               RouteSpanKind   `json:"kind"`
	QuestionEnglish    string          `json:"question_en"`
	QuestionRussian    string          `json:"question_ru"`
	TargetJob          TargetJob       `json:"target_job"`
	LearningStage      LearningStage   `json:"learning_stage"`
	RequiredSupportIDs []string        `json:"required_support_ids"`
	AllowedTargetIDs   []string        `json:"allowed_target_ids"`
	Joins              []RouteSpanJoin `json:"joins,omitempty"`
}

func (span RouteSpan) question(language Language) string {
	if language == LanguageRussian {
		return span.QuestionRussian
	}
	return span.QuestionEnglish
}

type CandidateCoverageCount struct {
	Key        string `json:"key"`
	Considered int    `json:"considered"`
	Selected   int    `json:"selected"`
}

// CoverageOmissionReason is a closed reason for an advertised-frontier
// omission. It is never semantic importance: selection order is a
// request-budget mechanism only.
type CoverageOmissionReason string

const (
	// OmissionAdvertisedBudget records considered spans omitted because the
	// advertised frontier is capped at MaxAdvertisedSpans.
	OmissionAdvertisedBudget CoverageOmissionReason = "advertised_budget"
)

func (reason CoverageOmissionReason) Valid() bool {
	return reason == OmissionAdvertisedBudget
}

// CoverageOmission aggregates one closed omission reason with a bounded
// representative list of typed refs. The complete candidate digest
// (CandidateSHA256) already binds the full considered set, so an unbounded
// omission list is never persisted.
type CoverageOmission struct {
	Reason          CoverageOmissionReason `json:"reason"`
	Count           int                    `json:"count"`
	Representatives []CanonicalRef         `json:"representatives,omitempty"`
}

// CandidateCoverage makes bounded shelf loss explicit and request-bound. The
// considered set is the complete locally supported D210 span set; the selected
// set is the advertised request frontier. Complete is full-selection equality
// over targets and spans, which under the D211 producer invariant (every
// support target has one focused span) equals the advertised frontier matching
// the complete considered span set.
type CandidateCoverage struct {
	CandidateSHA256   string                   `json:"candidate_sha256"`
	TargetsConsidered int                      `json:"targets_considered"`
	TargetsSelected   int                      `json:"targets_selected"`
	SpansConsidered   int                      `json:"spans_considered"`
	SpansSelected     int                      `json:"spans_selected"`
	Complete          bool                     `json:"complete"`
	PerRole           []CandidateCoverageCount `json:"per_role"`
	PerPackage        []CandidateCoverageCount `json:"per_package"`
	Omissions         []CoverageOmission       `json:"omissions,omitempty"`
}

type EvidenceFact struct {
	ID          string                    `json:"id"`
	SubjectRefs []CanonicalRef            `json:"subject_refs"`
	Authority   repositoryatlas.Authority `json:"authority"`
	Fact        string                    `json:"fact"`
}

type DocumentClaim struct {
	ID        string                    `json:"id"`
	Label     string                    `json:"label"`
	Claim     string                    `json:"claim"`
	Authority repositoryatlas.Authority `json:"authority"`
}

type Input struct {
	Atlas             repositoryatlas.Atlas   `json:"atlas"`
	Architecture      ArchitectureInput       `json:"architecture"`
	Language          Language                `json:"language"`
	Surfaces          []Surface               `json:"surfaces"`
	ReadingTargets    []ReadingTarget         `json:"reading_targets"`
	ReadingSupports   []ReadingSupport        `json:"reading_supports"`
	ProducerRelations []RouteProducerRelation `json:"producer_relations"`
	RouteSpans        []RouteSpan             `json:"route_spans"`
	Evidence          []EvidenceFact          `json:"evidence"`
	Documents         []DocumentClaim         `json:"documents"`
	Limits            Limits                  `json:"limits"`
}

type Prompt struct {
	Version  string   `json:"version"`
	Language Language `json:"language"`
	System   string   `json:"system"`
	User     string   `json:"user"`
}

type RepositoryType string

const (
	RepositoryService  RepositoryType = "service_application"
	RepositoryLibrary  RepositoryType = "library_framework"
	RepositoryCLI      RepositoryType = "cli_tool"
	RepositoryMonorepo RepositoryType = "monorepo"
	RepositoryMixed    RepositoryType = "mixed"
)

func (kind RepositoryType) Valid() bool {
	switch kind {
	case RepositoryService, RepositoryLibrary, RepositoryCLI,
		RepositoryMonorepo, RepositoryMixed:
		return true
	default:
		return false
	}
}

type LearningStage string

const (
	StageOrientation      LearningStage = "orientation"
	StageCentralOperation LearningStage = "central_operation"
	StageCoreModel        LearningStage = "core_model"
	StageIntegration      LearningStage = "integration"
	StageOperations       LearningStage = "operations"
	StageContribution     LearningStage = "contribution"
)

func (stage LearningStage) Valid() bool {
	switch stage {
	case StageOrientation, StageCentralOperation, StageCoreModel,
		StageIntegration, StageOperations, StageContribution:
		return true
	default:
		return false
	}
}

type TargetJob string

const (
	JobFirstContact TargetJob = "first_contact"
	JobUseOperate   TargetJob = "use_or_operate"
	JobIntegrate    TargetJob = "extend_or_integrate"
	JobContribute   TargetJob = "contribute"
	JobMaintain     TargetJob = "debug_or_maintain"
)

func (job TargetJob) Valid() bool {
	switch job {
	case JobFirstContact, JobUseOperate, JobIntegrate, JobContribute, JobMaintain:
		return true
	default:
		return false
	}
}

type ReadingLabel string

const (
	ReadingStart    ReadingLabel = "start"
	ReadingContinue ReadingLabel = "continue"
	ReadingConnect  ReadingLabel = "connect"
	ReadingVerify   ReadingLabel = "verify"
	ReadingContrast ReadingLabel = "contrast"
)

func (label ReadingLabel) Valid() bool {
	switch label {
	case ReadingStart, ReadingContinue, ReadingConnect, ReadingVerify, ReadingContrast:
		return true
	default:
		return false
	}
}

type SupportedStatement struct {
	Text        string         `json:"text"`
	SupportRefs []CanonicalRef `json:"support_refs"`
}

type DomainTerm struct {
	Term        string         `json:"term"`
	Meaning     string         `json:"meaning"`
	SupportRefs []CanonicalRef `json:"support_refs"`
}

type Brief struct {
	WhatItIs              SupportedStatement `json:"what_it_is"`
	Problem               SupportedStatement `json:"problem"`
	MainInput             SupportedStatement `json:"main_input"`
	CentralResponsibility SupportedStatement `json:"central_responsibility"`
	ObservableResult      SupportedStatement `json:"observable_result"`
	DomainTerms           []DomainTerm       `json:"domain_terms,omitempty"`
}

type ResolvedReading struct {
	Target        CanonicalRef `json:"target"`
	Label         ReadingLabel `json:"label"`
	WhatToLookFor string       `json:"what_to_look_for"`
}

type Direction struct {
	ID              string            `json:"id"`
	Span            CanonicalRef      `json:"span"`
	Question        string            `json:"question"`
	WhyItMatters    string            `json:"why_it_matters"`
	LearningOutcome string            `json:"learning_outcome"`
	TargetJob       TargetJob         `json:"target_job"`
	LearningStage   LearningStage     `json:"learning_stage"`
	PrincipalRefs   []CanonicalRef    `json:"principal_refs"`
	Reading         []ResolvedReading `json:"reading"`
}

// SpanCoverage carries the four distinct D211 span stages and the four
// independent coverage flags. The old Requested/Covered/Uncovered
// advertised-coverage semantics are removed: an advertised span with no
// returned direction is normal not_selected, never uncovered, and never turns
// a result into accepted_partial.
type SpanCoverage struct {
	// ConsideredSpanCount is the complete locally supported D210 span set.
	ConsideredSpanCount int `json:"considered_span_count"`
	// AdvertisedSpanCount is the request frontier (MaxAdvertisedSpans).
	AdvertisedSpanCount int `json:"advertised_span_count"`
	// ModelSelectedSpanCount is the distinct spans referenced by the returned
	// directions, including locally rejected siblings.
	ModelSelectedSpanCount int `json:"model_selected_span_count"`
	// AcceptedSpanCount is the distinct spans of directions that pass exact
	// item-local validation; it equals the accepted direction count because
	// duplicate-span directions are rejected.
	AcceptedSpanCount int `json:"accepted_span_count"`

	// FrontierComplete is true when the advertised frontier equals the complete
	// considered set (zero omissions).
	FrontierComplete bool `json:"frontier_complete"`
	// SelectedItemsComplete is true when every returned selected item is
	// locally valid and at least one direction was returned (no rejected
	// sibling).
	SelectedItemsComplete bool `json:"selected_items_complete"`
	// SupportCoverageComplete is recorded independently: item-local validation
	// keeps the invariant that every accepted direction covers all required
	// support identities of its exact span, so the flag is true whenever at
	// least one direction is accepted.
	SupportCoverageComplete bool `json:"support_coverage_complete"`
	// PortfolioTargetMet is true when the accepted direction count is within
	// the desired MinPortfolioDirections..MaxDirections band. It is
	// independent of status and does not by itself invalidate an otherwise
	// exact result.
	PortfolioTargetMet bool `json:"portfolio_target_met"`
}

type DirectionIssueCode string

const (
	IssueUnrequestedOutput       DirectionIssueCode = "unrequested_output"
	IssueDecodeCandidate         DirectionIssueCode = "decode_candidate"
	IssueInvalidQuestion         DirectionIssueCode = "invalid_question"
	IssueInvalidSpanRef          DirectionIssueCode = "invalid_span_ref"
	IssueWrongKindSpanRef        DirectionIssueCode = "wrong_kind_span_ref"
	IssueDuplicateSpanRef        DirectionIssueCode = "duplicate_span_ref"
	IssueSpanTargetNotAllowed    DirectionIssueCode = "span_target_not_allowed"
	IssueSpanSupportIncomplete   DirectionIssueCode = "span_support_incomplete"
	IssueSystemPathTooShort      DirectionIssueCode = "system_path_too_short"
	IssueInvalidWhy              DirectionIssueCode = "invalid_why"
	IssueInvalidOutcome          DirectionIssueCode = "invalid_outcome"
	IssueInvalidTargetJob        DirectionIssueCode = "invalid_target_job"
	IssueInvalidLearningStage    DirectionIssueCode = "invalid_learning_stage"
	IssueInvalidPrincipalCount   DirectionIssueCode = "invalid_principal_count"
	IssueDuplicatePrincipalRef   DirectionIssueCode = "duplicate_principal_ref"
	IssuePrincipalNotAdvertised  DirectionIssueCode = "principal_not_advertised"
	IssueRawCanonicalRef         DirectionIssueCode = "raw_canonical_ref"
	IssueUnknownRef              DirectionIssueCode = "unknown_ref"
	IssueWrongKindPrincipalRef   DirectionIssueCode = "wrong_kind_principal_ref"
	IssueComponentMissing        DirectionIssueCode = "component_principal_missing"
	IssueInvalidReadingCount     DirectionIssueCode = "invalid_reading_count"
	IssueDuplicateReadingTarget  DirectionIssueCode = "duplicate_reading_target"
	IssueWrongKindReadingRef     DirectionIssueCode = "wrong_kind_reading_ref"
	IssueReadingPrincipalMissing DirectionIssueCode = "reading_principal_not_selected"
	IssueInvalidReadingLabel     DirectionIssueCode = "invalid_reading_label"
	IssueInvalidReadingCopy      DirectionIssueCode = "invalid_reading_copy"
	IssueDuplicateDirection      DirectionIssueCode = "duplicate_direction"
	IssueInvalidRef              DirectionIssueCode = "invalid_ref"
)

func (code DirectionIssueCode) Valid() bool {
	switch code {
	case IssueUnrequestedOutput, IssueDecodeCandidate, IssueInvalidQuestion,
		IssueInvalidSpanRef, IssueWrongKindSpanRef, IssueDuplicateSpanRef,
		IssueSpanTargetNotAllowed, IssueSpanSupportIncomplete, IssueSystemPathTooShort,
		IssueInvalidWhy, IssueInvalidOutcome, IssueInvalidTargetJob,
		IssueInvalidLearningStage, IssueInvalidPrincipalCount,
		IssueDuplicatePrincipalRef, IssuePrincipalNotAdvertised,
		IssueRawCanonicalRef, IssueUnknownRef,
		IssueWrongKindPrincipalRef, IssueComponentMissing,
		IssueInvalidReadingCount, IssueDuplicateReadingTarget,
		IssueWrongKindReadingRef, IssueReadingPrincipalMissing,
		IssueInvalidReadingLabel, IssueInvalidReadingCopy,
		IssueDuplicateDirection, IssueInvalidRef:
		return true
	default:
		return false
	}
}

type DirectionIssue struct {
	Position int                `json:"position"`
	Code     DirectionIssueCode `json:"code"`
}

type DomainTermIssueCode string

const (
	DomainTermIssueUnrequestedOutput DomainTermIssueCode = "unrequested_output"
	DomainTermIssueDecodeCandidate   DomainTermIssueCode = "decode_candidate"
	DomainTermIssueInvalidSupport    DomainTermIssueCode = "invalid_support"
	DomainTermIssueInvalidTerm       DomainTermIssueCode = "invalid_term"
	DomainTermIssueInvalidMeaning    DomainTermIssueCode = "invalid_meaning"
)

func (code DomainTermIssueCode) Valid() bool {
	switch code {
	case DomainTermIssueUnrequestedOutput, DomainTermIssueDecodeCandidate,
		DomainTermIssueInvalidSupport,
		DomainTermIssueInvalidTerm, DomainTermIssueInvalidMeaning:
		return true
	default:
		return false
	}
}

type DomainTermIssue struct {
	Position int                 `json:"position"`
	Code     DomainTermIssueCode `json:"code"`
}

type Diagnostics struct {
	DirectionsReceived  int               `json:"directions_received"`
	DirectionsAccepted  int               `json:"directions_accepted"`
	DirectionsRejected  int               `json:"directions_rejected"`
	Issues              []DirectionIssue  `json:"issues,omitempty"`
	DomainTermsReceived int               `json:"domain_terms_received"`
	DomainTermsAccepted int               `json:"domain_terms_accepted"`
	DomainTermsRejected int               `json:"domain_terms_rejected"`
	DomainTermIssues    []DomainTermIssue `json:"domain_term_issues,omitempty"`
}
