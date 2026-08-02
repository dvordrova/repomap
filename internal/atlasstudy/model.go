// Package atlasstudy defines the Atlas-first repository Brief and Study
// contract. It keeps canonical repository identities private while exposing a
// bounded task-shaped reading catalog with exact paths, lines, optional
// qualified symbols and short typed refs to a model provider.
package atlasstudy

import (
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	Version       = 3
	PromptVersion = "atlas-study-prompt-v9"

	RequestArtifactFilename = "atlas_study_request.v3.json"
	ResultArtifactFilename  = "atlas_study_result.v3.json"
	StatusArtifactFilename  = "atlas_study_status.v3.json"

	MaxRequestArtifactBytes = 16 << 20
	MaxResultArtifactBytes  = 16 << 20
	MaxStatusArtifactBytes  = 64 << 10

	MaxDirections           = studymap.MaxCandidates
	MaxDirectionDiagnostics = 12
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
	MaxEvidence       int `json:"max_evidence"`
	MaxDocuments      int `json:"max_documents"`
}

func DefaultLimits() Limits {
	return Limits{
		MaxWireBytes: 1 << 20, MaxResponseBytes: 16 << 20, MaxTextBytes: 4096,
		MaxUnits: 512, MaxSubsystems: 64, MaxComponents: 256,
		MaxSurfaces: 128, MaxReadingTargets: 512, MaxEvidence: 512,
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
)

func (kind RefKind) Valid() bool {
	switch kind {
	case RefUnit, RefSubsystem, RefComponent, RefSurface, RefReadingTarget,
		RefEvidence, RefDocument:
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
	Atlas          repositoryatlas.Atlas `json:"atlas"`
	Architecture   ArchitectureInput     `json:"architecture"`
	Language       Language              `json:"language"`
	Surfaces       []Surface             `json:"surfaces"`
	ReadingTargets []ReadingTarget       `json:"reading_targets"`
	Evidence       []EvidenceFact        `json:"evidence"`
	Documents      []DocumentClaim       `json:"documents"`
	Limits         Limits                `json:"limits"`
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
	Question        string            `json:"question"`
	WhyItMatters    string            `json:"why_it_matters"`
	LearningOutcome string            `json:"learning_outcome"`
	TargetJob       TargetJob         `json:"target_job"`
	LearningStage   LearningStage     `json:"learning_stage"`
	PrincipalRefs   []CanonicalRef    `json:"principal_refs"`
	Reading         []ResolvedReading `json:"reading"`
}

type DirectionIssueCode string

const (
	IssueUnrequestedOutput       DirectionIssueCode = "unrequested_output"
	IssueDecodeCandidate         DirectionIssueCode = "decode_candidate"
	IssueInvalidQuestion         DirectionIssueCode = "invalid_question"
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

type Diagnostics struct {
	DirectionsReceived int              `json:"directions_received"`
	DirectionsAccepted int              `json:"directions_accepted"`
	DirectionsRejected int              `json:"directions_rejected"`
	Issues             []DirectionIssue `json:"issues,omitempty"`
}
