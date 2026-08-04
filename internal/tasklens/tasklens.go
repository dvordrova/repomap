// Package tasklens defines the bounded, replayable contract for one
// task-conditioned repository investigation. A Task Investigation Pack is an
// ephemeral presentation/research artifact, not canonical repository truth.
package tasklens

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	BundleVersion   = 2
	ProposalVersion = 1
	PackVersion     = 2
	AttemptVersion  = 2
	StatusVersion   = 2

	PromptVersion = "task-investigation-pack-json-v2"

	BundleFile        = "task_investigation_bundle.json"
	AttemptFile       = "task_investigation_attempt.json"
	PackFile          = "task_investigation.json"
	StatusFile        = "task_investigation_status.json"
	TraceJSONFile     = "retrieval_trace.json"
	TraceMarkdownFile = "retrieval_trace.md"

	MaxInitialCandidates       = 40
	MaxRetainedAnchors         = 16
	MinVisibleAnchors          = 0
	PreferredMinVisibleAnchors = 3
	MaxVisibleAnchors          = 8
	MaxReadFiles               = 12
	MaxReadBytes               = 128 << 10
	MaxSourceScanBytes         = 4 << 20
	MaxRetainedSourceBytes     = 128 << 10
	// Manifest indexing shares the repository read budget with task evidence.
	// Reserve most slots for task-conditioned retrieval on multi-module repos.
	MaxManifestFiles      = 4
	MaxManifestBytes      = 32 << 10
	MaxGoplsQueries       = 12
	MaxFrontierExpansions = 2
	MaxModelCalls         = 4
	MaxNextProbes         = 3
	MaxModules            = 12
	MaxEvidenceJoins      = 6
	MaxHypothesisClauses  = 3
	MaxGuidanceSteps      = 4
	MaxLocalWallMillis    = 10_000

	MaxArtifactBytes         = 4 << 20
	MaxSavedRawResponseBytes = 512 << 10
	MaxTaskBytes             = 32 << 10

	maxTaskBytes = MaxTaskBytes
	maxTextBytes = 4096
)

const (
	RawResponseOmittedSize   = "size_limit"
	RawResponseOmittedSecret = "secret_like_content"

	AttemptWarningResponseSize     = "The raw response was omitted because it exceeded the bounded diagnostic size."
	AttemptWarningResponseSecret   = "The raw response was omitted because secret-like content was detected."
	AttemptWarningProviderFailed   = "The semantic synthesis call failed; no semantic retry was attempted."
	AttemptWarningResponseRejected = "The substantive response was rejected locally; no semantic retry was attempted."
	AttemptWarningSparseEvidence   = "Semantic synthesis was skipped because bounded retrieval retained fewer than three source anchors."

	ReductionErrorProviderFailed = "provider call failed; response was not used"
	ReductionErrorOmittedSize    = "response was rejected because it exceeded the replayable response size bound"
	ReductionErrorOmittedSecret  = "response was rejected because secret-like content prevented replay-safe retention"

	PackWarningLocalPartial = "This deterministic local pack remains partial because the bounded evidence does not support a complete, actionable task lens."
	PackWarningModelPartial = "This model-edited pack remains partial because the bounded evidence does not support a complete, actionable task lens."
)

// WarningCode is a semantic-neutral identity for one deterministic Task Lens
// reducer warning. Raw warning strings remain the canonical artifact
// diagnostics; presentation layers may map these codes to locale-specific
// product copy without interpreting the raw prose.
type WarningCode string

const (
	WarningAnchorOmittedIrrelevant        WarningCode = "anchor_omitted_irrelevant"
	WarningAnchorRoleReplaced             WarningCode = "anchor_role_replaced"
	WarningAnchorExplanationReplaced      WarningCode = "anchor_explanation_replaced"
	WarningAreaTargetsFiltered            WarningCode = "area_targets_filtered"
	WarningAreaOmittedWithoutAnchor       WarningCode = "area_omitted_without_anchor"
	WarningAreasBounded                   WarningCode = "areas_bounded"
	WarningAreaCopyReplaced               WarningCode = "area_copy_replaced"
	WarningAreaFallbackAdded              WarningCode = "area_fallback_added"
	WarningJoinsBounded                   WarningCode = "joins_bounded"
	WarningJoinRejected                   WarningCode = "join_rejected"
	WarningHypothesesBounded              WarningCode = "hypotheses_bounded"
	WarningHypothesisRejected             WarningCode = "hypothesis_rejected"
	WarningHypothesisSupportCompleted     WarningCode = "hypothesis_support_completed"
	WarningHypothesisCopyReplaced         WarningCode = "hypothesis_copy_replaced"
	WarningHypothesisFallbackAdded        WarningCode = "hypothesis_fallback_added"
	WarningReproductionBounded            WarningCode = "reproduction_bounded"
	WarningReproductionRejected           WarningCode = "reproduction_rejected"
	WarningReproductionDuplicate          WarningCode = "reproduction_duplicate"
	WarningReproductionFallbackAdded      WarningCode = "reproduction_fallback_added"
	WarningVerificationBounded            WarningCode = "verification_bounded"
	WarningVerificationOutsideFrontier    WarningCode = "verification_outside_frontier"
	WarningVerificationRejected           WarningCode = "verification_rejected"
	WarningVerificationDuplicate          WarningCode = "verification_duplicate"
	WarningVerificationFallbackAdded      WarningCode = "verification_fallback_added"
	WarningVerificationAuthorityAdded     WarningCode = "verification_authority_added"
	WarningVerificationTestAuthorityAdded WarningCode = "verification_test_authority_added"
	WarningNextProbesBounded              WarningCode = "next_probes_bounded"
	WarningNextProbeRejected              WarningCode = "next_probe_rejected"
	WarningNextProbeFallbackAdded         WarningCode = "next_probe_fallback_added"
	WarningAttemptResponseSize            WarningCode = "attempt_response_size"
	WarningAttemptResponseSecret          WarningCode = "attempt_response_secret"
	WarningAttemptProviderFailed          WarningCode = "attempt_provider_failed"
	WarningAttemptResponseRejected        WarningCode = "attempt_response_rejected"
	WarningAttemptSparseEvidence          WarningCode = "attempt_sparse_evidence"
	WarningPackLocalPartial               WarningCode = "pack_local_partial"
	WarningPackModelPartial               WarningCode = "pack_model_partial"
)

// WarningDiagnostic is a bounded terminal-presentation projection emitted at
// the same point as its canonical raw warning. Index is one-based and is the
// only dynamic product-copy parameter used by the reducer warning catalog.
type WarningDiagnostic struct {
	Code  WarningCode `json:"code"`
	Index int         `json:"index,omitempty"`
}

// WarningEmission keeps legacy artifact prose and terminal presentation
// identity together at a typed producer boundary.
type WarningEmission struct {
	Raw        string
	Diagnostic WarningDiagnostic
}

// RawResponseOmissionEmission selects the fixed warning from the typed
// omission reason recorded by the producer, never from warning prose.
func RawResponseOmissionEmission(reason string) (WarningEmission, bool) {
	switch reason {
	case RawResponseOmittedSize:
		return WarningEmission{
			Raw: AttemptWarningResponseSize,
			Diagnostic: WarningDiagnostic{
				Code: WarningAttemptResponseSize,
			},
		}, true
	case RawResponseOmittedSecret:
		return WarningEmission{
			Raw: AttemptWarningResponseSecret,
			Diagnostic: WarningDiagnostic{
				Code: WarningAttemptResponseSecret,
			},
		}, true
	default:
		return WarningEmission{}, false
	}
}

// AttemptStateWarningEmission selects the one fixed warning implied by a
// producer attempt state. Accepted and cleanly skipped states have no fixed
// attempt warning; reducer warnings are emitted independently.
func AttemptStateWarningEmission(state string) (WarningEmission, bool) {
	var raw string
	var code WarningCode
	switch state {
	case "provider_failed":
		raw = AttemptWarningProviderFailed
		code = WarningAttemptProviderFailed
	case "rejected":
		raw = AttemptWarningResponseRejected
		code = WarningAttemptResponseRejected
	case "skipped_insufficient_evidence":
		raw = AttemptWarningSparseEvidence
		code = WarningAttemptSparseEvidence
	default:
		return WarningEmission{}, false
	}
	return WarningEmission{
		Raw:        raw,
		Diagnostic: WarningDiagnostic{Code: code},
	}, true
}

// PartialPackWarningEmission selects the fixed partial-pack warning from the
// structural attempt state used by FinalizeTaskInvestigationPack.
func PartialPackWarningEmission(attemptState string) WarningEmission {
	if attemptState == "accepted" || attemptState == "accepted_with_rejections" {
		return WarningEmission{
			Raw: PackWarningModelPartial,
			Diagnostic: WarningDiagnostic{
				Code: WarningPackModelPartial,
			},
		}
	}
	return WarningEmission{
		Raw: PackWarningLocalPartial,
		Diagnostic: WarningDiagnostic{
			Code: WarningPackLocalPartial,
		},
	}
}

type reductionWarningCollector struct {
	raw         *[]string
	diagnostics *[]WarningDiagnostic
}

func (collector reductionWarningCollector) add(
	code WarningCode,
	index int,
	message string,
) {
	*collector.raw = append(*collector.raw, message)
	if collector.diagnostics != nil {
		*collector.diagnostics = append(*collector.diagnostics, WarningDiagnostic{
			Code:  code,
			Index: index,
		})
	}
}

func RawResponseOmissionWarning(reason string) string {
	emission, ok := RawResponseOmissionEmission(reason)
	if !ok {
		return ""
	}
	return emission.Raw
}

func RawResponseOmissionReductionError(reason string) string {
	switch reason {
	case RawResponseOmittedSize:
		return ReductionErrorOmittedSize
	case RawResponseOmittedSecret:
		return ReductionErrorOmittedSecret
	default:
		return ""
	}
}

type TaskKind string

const (
	TaskBug           TaskKind = "bug"
	TaskFeature       TaskKind = "feature"
	TaskExtension     TaskKind = "extension"
	TaskConfiguration TaskKind = "configuration"
	TaskOperational   TaskKind = "operational"
	TaskCompatibility TaskKind = "compatibility"
	TaskUnknown       TaskKind = "unknown"
)

type Locality string

const (
	LocalityLocalExact       Locality = "local_exact"
	LocalityBoundedCrossFile Locality = "bounded_cross_file"
	LocalityExtension        Locality = "extension_contribution"
	LocalityBroadDynamic     Locality = "broad_dynamic"
)

type EvidenceKind string

const (
	EvidenceRepositoryFact EvidenceKind = "repository_fact"
	EvidenceDocumentClaim  EvidenceKind = "document_claim"
	EvidenceTaskProvided   EvidenceKind = "task_provided"
)

type AnchorRole string

const (
	RoleSymptomSite                  AnchorRole = "symptom_site"
	RolePublicOrCLIEntry             AnchorRole = "public_or_cli_entry"
	RoleStateOwner                   AnchorRole = "state_owner"
	RoleStateMutation                AnchorRole = "state_mutation"
	RoleConfigurationSource          AnchorRole = "configuration_source"
	RoleConfigurationCopy            AnchorRole = "configuration_copy"
	RoleErrorCreation                AnchorRole = "error_creation"
	RoleErrorMapping                 AnchorRole = "error_mapping"
	RoleIntegrationBoundary          AnchorRole = "integration_boundary"
	RoleRepresentativeImplementation AnchorRole = "representative_implementation"
	RoleGeneratedOutput              AnchorRole = "generated_output"
	RoleReproductionAnchor           AnchorRole = "reproduction_anchor"
	RoleVerificationAnchor           AnchorRole = "verification_anchor"
	RoleDocumentationContract        AnchorRole = "documentation_contract"
)

type SupportType string

const (
	SupportLocallyObserved SupportType = "locally_observed"
	SupportDocument        SupportType = "document_supported"
	SupportModelHypothesis SupportType = "model_hypothesis"
	SupportUnresolved      SupportType = "unresolved"
)

type HypothesisStatus string

const (
	HypothesisSupported  HypothesisStatus = "supported"
	HypothesisPlausible  HypothesisStatus = "plausible"
	HypothesisUnresolved HypothesisStatus = "unresolved"
)

type GuidanceAuthority string

const (
	AuthorityTaskProvided          GuidanceAuthority = "task_provided"
	AuthorityRepositoryDocument    GuidanceAuthority = "repository_document"
	AuthorityRepositoryTest        GuidanceAuthority = "repository_test_or_example"
	AuthorityRepositoryObservation GuidanceAuthority = "repository_observation"
	AuthorityMissing               GuidanceAuthority = "missing_evidence"
)

type ProbeAction string

const (
	ProbeInspectSymbol       ProbeAction = "inspect_symbol"
	ProbeResolveReference    ProbeAction = "resolve_reference"
	ProbeCompareConfigCopies ProbeAction = "compare_config_copies"
	ProbeInspectFixture      ProbeAction = "inspect_fixture"
	ProbeInspectSibling      ProbeAction = "inspect_sibling_implementation"
	ProbeSearchTaskTerms     ProbeAction = "search_task_terms"
)

type Repository struct {
	Identity           string `json:"identity"`
	DisplayName        string `json:"display_name,omitempty"`
	Revision           string `json:"revision"`
	TreeHash           string `json:"tree_hash"`
	StateSHA256        string `json:"state_sha256"`
	IdentitySource     string `json:"identity_source"`
	IdentitySourcePath string `json:"identity_source_path,omitempty"`
}

type Task struct {
	Text       string `json:"text"`
	EvidenceID string `json:"evidence_id"`
}

type Term struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Normalized  string   `json:"normalized"`
	Found       bool     `json:"found"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Weight      int      `json:"weight,omitempty"`
}

type Module struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Dir        string `json:"dir"`
	SourcePath string `json:"source_path"`
}

type SourceLine struct {
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Highlight bool   `json:"highlight,omitempty"`
}

type Evidence struct {
	ID        string       `json:"id"`
	Kind      EvidenceKind `json:"kind"`
	Path      string       `json:"path,omitempty"`
	StartLine int          `json:"start_line,omitempty"`
	EndLine   int          `json:"end_line,omitempty"`
	AnchorID  string       `json:"anchor_id,omitempty"`
	Summary   string       `json:"summary"`
}

type Anchor struct {
	ID          string       `json:"id"`
	Path        string       `json:"path"`
	Symbol      string       `json:"symbol"`
	Section     string       `json:"section,omitempty"`
	Package     string       `json:"package,omitempty"`
	StartLine   int          `json:"start_line"`
	EndLine     int          `json:"end_line"`
	Excerpt     []SourceLine `json:"excerpt"`
	Scope       SourceScope  `json:"source_scope"`
	RoleHints   []AnchorRole `json:"role_hints,omitempty"`
	EvidenceIDs []string     `json:"evidence_ids"`
	Score       int          `json:"score,omitempty"`
}

type Relation struct {
	ID          string      `json:"id"`
	LeftID      string      `json:"left_anchor_id"`
	RightID     string      `json:"right_anchor_id"`
	Kind        string      `json:"kind"`
	SupportType SupportType `json:"support_type"`
	EvidenceIDs []string    `json:"evidence_ids"`
	Scope       string      `json:"scope"`
}

const (
	relationKindDirectCall      = "direct_call"
	relationKindExactIdentifier = "scope_unknown"
	relationKindSharedTaskTerm  = "shared_state_alias"
	relationKindTestReference   = "test_exercises"
)

type Budgets struct {
	InitialCandidates       int   `json:"initial_candidates"`
	CandidateItemsFound     int   `json:"candidate_items_found"`
	RetainedAnchors         int   `json:"retained_anchors"`
	AnchorItemsFound        int   `json:"anchor_items_found"`
	EvidenceFilesConsidered int   `json:"evidence_files_considered"`
	ReadFiles               int   `json:"read_files"`
	ReadBytes               int   `json:"read_bytes"`
	SourceScanBytes         int   `json:"source_scan_bytes,omitempty"`
	RetainedSourceBytes     int   `json:"retained_source_bytes"`
	GoplsQueries            int   `json:"gopls_queries"`
	FrontierExpansions      int   `json:"frontier_expansions"`
	LocalWallMillis         int64 `json:"local_wall_millis"`

	CandidateLimitBound    bool `json:"candidate_limit_bound,omitempty"`
	AnchorLimitBound       bool `json:"anchor_limit_bound,omitempty"`
	FileLimitBound         bool `json:"file_limit_bound,omitempty"`
	ByteLimitBound         bool `json:"byte_limit_bound,omitempty"`
	SourceScanLimitBound   bool `json:"source_scan_limit_bound,omitempty"`
	RetainedByteLimitBound bool `json:"retained_byte_limit_bound,omitempty"`
	TimeLimitBound         bool `json:"time_limit_bound,omitempty"`
}

type RetrievalMetrics struct {
	TrackedFiles      int `json:"tracked_files"`
	GitGrepQueries    int `json:"git_grep_queries"`
	ASTParses         int `json:"ast_parses"`
	RelationsRetained int `json:"relations_retained"`
	EvidenceFilesRead int `json:"evidence_files_read"`
	ModuleFilesFound  int `json:"module_files_found"`
	ModuleFilesRead   int `json:"module_files_read"`
	ModuleBytesRead   int `json:"module_bytes_read"`
	ManifestFilesRead int `json:"manifest_files_read"`
	ManifestBytesRead int `json:"manifest_bytes_read"`
}

// Bundle is the full local authority for one synthesis attempt. It contains a
// bounded source subset, never a raw repository tree or global edge list.
type Bundle struct {
	Version            int                  `json:"version"`
	ID                 string               `json:"id"`
	Repository         Repository           `json:"repository"`
	Task               Task                 `json:"task"`
	KindHint           TaskKind             `json:"task_kind_hint"`
	Profile            TaskProfile          `json:"task_profile"`
	ObservableHint     string               `json:"observable_hint"`
	Locality           Locality             `json:"locality"`
	Terms              []Term               `json:"terms"`
	Modules            []Module             `json:"modules,omitempty"`
	Anchors            []Anchor             `json:"anchors"`
	Evidence           []Evidence           `json:"evidence"`
	Relations          []Relation           `json:"relations,omitempty"`
	RoleContract       RoleContract         `json:"role_contract"`
	RoleCoverage       RoleCoverage         `json:"role_coverage"`
	Verification       VerificationFrontier `json:"verification_frontier"`
	DecisiveRelationID string               `json:"decisive_relation_id,omitempty"`
	CheapExit          CheapExitDecision    `json:"cheap_exit"`
	AllowedPaths       []string             `json:"allowed_paths"`
	StagesSkipped      []string             `json:"stages_skipped"`
	Budgets            Budgets              `json:"budgets"`
	Metrics            RetrievalMetrics     `json:"metrics"`
	// LocalTrace is an in-memory collection product. The canonical trace is
	// saved as its own hashed artifact and is intentionally not duplicated in
	// the model bundle JSON.
	LocalTrace RetrievalTrace `json:"-"`
}

// PromptBundle deliberately omits display-only checkout naming and local
// scores. It is the only Bundle projection allowed across the model boundary.
type PromptBundle struct {
	Version            int                  `json:"version"`
	ID                 string               `json:"id"`
	Repository         Repository           `json:"repository"`
	Task               Task                 `json:"task"`
	KindHint           TaskKind             `json:"task_kind_hint"`
	Profile            TaskProfile          `json:"task_profile"`
	ObservableHint     string               `json:"observable_hint"`
	Locality           Locality             `json:"locality"`
	Terms              []Term               `json:"terms"`
	Modules            []Module             `json:"modules,omitempty"`
	Anchors            []Anchor             `json:"anchors"`
	Evidence           []Evidence           `json:"evidence"`
	Relations          []Relation           `json:"relations,omitempty"`
	RoleContract       RoleContract         `json:"role_contract"`
	RoleCoverage       RoleCoverage         `json:"role_coverage"`
	Verification       VerificationFrontier `json:"verification_frontier"`
	DecisiveRelationID string               `json:"decisive_relation_id,omitempty"`
	AllowedPaths       []string             `json:"allowed_paths"`
	Budgets            Budgets              `json:"budgets"`
	Metrics            RetrievalMetrics     `json:"metrics"`
}

func (bundle Bundle) PromptBundle() PromptBundle {
	repository := bundle.Repository
	repository.DisplayName = ""
	budgets := bundle.Budgets
	// Wall-clock variation is operational accounting, not semantic evidence.
	// Keeping it out of the provider projection also makes identical checkouts
	// under different display names produce identical requests.
	budgets.LocalWallMillis = 0
	anchors := append([]Anchor(nil), bundle.Anchors...)
	for index := range anchors {
		anchors[index].Score = 0
	}
	return PromptBundle{
		Version: bundle.Version, ID: bundle.ID, Repository: repository,
		Task: bundle.Task, KindHint: bundle.KindHint, Profile: bundle.Profile,
		ObservableHint: bundle.ObservableHint, Locality: bundle.Locality,
		Terms:   append([]Term(nil), bundle.Terms...),
		Modules: append([]Module(nil), bundle.Modules...), Anchors: anchors,
		Evidence:     append([]Evidence(nil), bundle.Evidence...),
		Relations:    append([]Relation(nil), bundle.Relations...),
		RoleContract: bundle.RoleContract, RoleCoverage: bundle.RoleCoverage,
		Verification: bundle.Verification, DecisiveRelationID: bundle.DecisiveRelationID,
		AllowedPaths: append([]string(nil), bundle.AllowedPaths...), Budgets: budgets,
		Metrics: bundle.Metrics,
	}
}

type Proposal struct {
	Version            int                    `json:"version"`
	Interpretation     ProposedInterpretation `json:"task_interpretation"`
	Areas              []ProposedArea         `json:"likely_areas"`
	Anchors            []ProposedAnchor       `json:"anchors"`
	Joins              []ProposedJoin         `json:"evidence_joins,omitempty"`
	Hypothesis         []ProposedClause       `json:"working_hypothesis"`
	ReproduceOrObserve []ProposedGuidance     `json:"reproduce_or_observe"`
	Verify             ProposedVerification   `json:"verify"`
	NextProbes         []ProposedProbe        `json:"next_probes"`
}

type ProposedInterpretation struct {
	Restatement string   `json:"restatement"`
	Kind        TaskKind `json:"task_kind"`
	Observable  string   `json:"observable_or_outcome"`
}

type ProposedArea struct {
	Label     string   `json:"label"`
	Why       string   `json:"why"`
	TargetIDs []string `json:"target_ids"`
}

type ProposedAnchor struct {
	AnchorID string     `json:"anchor_id"`
	Role     AnchorRole `json:"role"`
	Why      string     `json:"why"`
}

type ProposedJoin struct {
	LeftID      string      `json:"left_anchor_id"`
	RightID     string      `json:"right_anchor_id"`
	RelationID  string      `json:"relation_id,omitempty"`
	Kind        string      `json:"relation_kind"`
	SupportType SupportType `json:"support_type"`
	SupportIDs  []string    `json:"support_ids"`
	Explanation string      `json:"explanation"`
	Scope       string      `json:"scope_non_guarantees"`
}

type ProposedClause struct {
	Status      HypothesisStatus `json:"status"`
	Text        string           `json:"text"`
	SupportIDs  []string         `json:"support_ids,omitempty"`
	RelationIDs []string         `json:"relation_ids,omitempty"`
}

type ProposedGuidance struct {
	Text        string            `json:"text"`
	Authority   GuidanceAuthority `json:"authority"`
	EvidenceIDs []string          `json:"evidence_ids,omitempty"`
}

type ProposedVerification struct {
	Effect string             `json:"effect_to_observe"`
	Steps  []ProposedGuidance `json:"steps"`
}

type ProposedProbe struct {
	Action    ProbeAction `json:"action"`
	AnchorIDs []string    `json:"anchor_ids"`
	Text      string      `json:"text"`
}

type Interpretation struct {
	Restatement      string   `json:"restatement"`
	Kind             TaskKind `json:"task_kind"`
	Observable       string   `json:"observable_or_outcome"`
	FoundTerms       []string `json:"repository_terms_found,omitempty"`
	UserProvidedOnly []string `json:"user_provided_only_terms,omitempty"`
}

type Area struct {
	Label     string   `json:"label"`
	Why       string   `json:"why"`
	TargetIDs []string `json:"target_ids"`
}

type InvestigationAnchor struct {
	ID          string       `json:"id"`
	Path        string       `json:"path"`
	Symbol      string       `json:"symbol"`
	Section     string       `json:"section,omitempty"`
	Package     string       `json:"package,omitempty"`
	Role        AnchorRole   `json:"role"`
	StartLine   int          `json:"start_line"`
	EndLine     int          `json:"end_line"`
	Excerpt     []SourceLine `json:"excerpt"`
	Scope       SourceScope  `json:"source_scope"`
	Why         string       `json:"why"`
	EvidenceIDs []string     `json:"evidence_ids"`
}

type EvidenceJoin struct {
	ID          string      `json:"id"`
	LeftID      string      `json:"left_anchor_id"`
	RightID     string      `json:"right_anchor_id"`
	RelationID  string      `json:"relation_id,omitempty"`
	Kind        string      `json:"relation_kind"`
	SupportType SupportType `json:"support_type"`
	SupportIDs  []string    `json:"support_ids"`
	Explanation string      `json:"explanation"`
	Scope       string      `json:"scope_non_guarantees"`
}

type HypothesisClause struct {
	Status      HypothesisStatus `json:"status"`
	Text        string           `json:"text"`
	SupportIDs  []string         `json:"support_ids,omitempty"`
	RelationIDs []string         `json:"relation_ids,omitempty"`
}

type Guidance struct {
	Text        string            `json:"text"`
	Authority   GuidanceAuthority `json:"authority"`
	EvidenceIDs []string          `json:"evidence_ids,omitempty"`
}

type Verification struct {
	Effect string     `json:"effect_to_observe"`
	Steps  []Guidance `json:"steps"`
}

type Probe struct {
	Action    ProbeAction `json:"action"`
	AnchorIDs []string    `json:"anchor_ids"`
	Text      string      `json:"text"`
}

type Pack struct {
	Version                 int                   `json:"version"`
	ID                      string                `json:"id"`
	BundleSHA256            string                `json:"bundle_sha256"`
	Repository              Repository            `json:"repository"`
	Locality                Locality              `json:"locality"`
	Profile                 TaskProfile           `json:"task_profile"`
	RoleContract            RoleContract          `json:"role_contract"`
	RoleCoverage            RoleCoverage          `json:"role_coverage"`
	VerificationFrontier    VerificationFrontier  `json:"verification_frontier"`
	DecisiveRelationID      string                `json:"decisive_relation_id,omitempty"`
	CheapExit               CheapExitDecision     `json:"cheap_exit"`
	StagesSkipped           []string              `json:"stages_skipped"`
	Interpretation          Interpretation        `json:"task_interpretation"`
	TaskObservationConcrete bool                  `json:"task_observation_concrete"`
	LikelyAreas             []Area                `json:"likely_areas"`
	Anchors                 []InvestigationAnchor `json:"investigation_anchors"`
	EvidenceJoins           []EvidenceJoin        `json:"evidence_joins"`
	WorkingHypothesis       []HypothesisClause    `json:"working_hypothesis"`
	ReproduceOrObserve      []Guidance            `json:"reproduce_or_observe"`
	Verify                  Verification          `json:"verify"`
	NextProbes              []Probe               `json:"next_probes"`
	Warnings                []string              `json:"warnings,omitempty"`
	Budgets                 Budgets               `json:"budgets"`
}

type ProviderMetrics struct {
	Calls                 int   `json:"calls"`
	TransportAttempts     int   `json:"transport_attempts"`
	RequestBytes          int   `json:"request_bytes"`
	ResponseBytes         int   `json:"response_bytes"`
	InputTokens           int   `json:"input_tokens"`
	OutputTokens          int   `json:"output_tokens"`
	PromptCacheHitTokens  int   `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int   `json:"prompt_cache_miss_tokens"`
	LatencyMillis         int64 `json:"latency_millis"`
}

type Attempt struct {
	Version                  int             `json:"version"`
	BundleSHA256             string          `json:"bundle_sha256"`
	PromptVersion            string          `json:"prompt_version"`
	PromptSHA256             string          `json:"prompt_sha256,omitempty"`
	State                    string          `json:"state"`
	ResponseSHA256           string          `json:"response_sha256,omitempty"`
	RawResponse              string          `json:"raw_response,omitempty"`
	RawResponseOmittedReason string          `json:"raw_response_omitted_reason,omitempty"`
	ReductionError           string          `json:"reduction_error,omitempty"`
	Warnings                 []string        `json:"warnings,omitempty"`
	Provider                 ProviderMetrics `json:"provider"`
}

type Status struct {
	Version                      int               `json:"version"`
	State                        string            `json:"state"`
	Sufficient                   bool              `json:"sufficient"`
	TaskID                       string            `json:"task_id"`
	BundleSHA256                 string            `json:"bundle_sha256"`
	AttemptSHA256                string            `json:"attempt_sha256"`
	PackSHA256                   string            `json:"pack_sha256"`
	RetrievalTraceSHA256         string            `json:"retrieval_trace_sha256"`
	RetrievalTraceMarkdownSHA256 string            `json:"retrieval_trace_markdown_sha256"`
	CapturedRevision             string            `json:"captured_revision"`
	TreeHash                     string            `json:"tree_hash"`
	Locality                     Locality          `json:"locality"`
	StagesSkipped                []string          `json:"stages_skipped"`
	Provider                     ProviderMetrics   `json:"provider"`
	CheapExit                    CheapExitDecision `json:"cheap_exit"`
	Budgets                      Budgets           `json:"budgets"`
	Warnings                     []string          `json:"warnings,omitempty"`
}

func OpaqueID(kind string, parts ...string) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, "repomap-task-lens-v1\x00"+kind)
	for _, part := range parts {
		_, _ = io.WriteString(hash, "\x00"+part)
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))[:20]
}

func BundleHash(bundle Bundle) (string, error) {
	if err := bundle.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("task lens: marshal bundle: %w", err)
	}
	return SHA256(raw), nil
}

func SHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func DecodeProposal(raw []byte) (Proposal, error) {
	if len(raw) == 0 || len(raw) > MaxArtifactBytes {
		return Proposal{}, fmt.Errorf("task lens: proposal is outside bounds")
	}
	if kind, found := secretscan.Detect(string(raw)); found {
		return Proposal{}, fmt.Errorf("task lens: proposal rejected because %s was detected", kind)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, fmt.Errorf("task lens: decode proposal: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Proposal{}, err
	}
	if proposal.Version != ProposalVersion {
		return Proposal{}, fmt.Errorf("task lens: unsupported proposal version %d", proposal.Version)
	}
	return proposal, nil
}

func DecodePack(raw []byte) (Pack, error) {
	if len(raw) == 0 || len(raw) > MaxArtifactBytes {
		return Pack{}, fmt.Errorf("task lens: pack is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var pack Pack
	if err := decoder.Decode(&pack); err != nil {
		return Pack{}, fmt.Errorf("task lens: decode pack: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Pack{}, err
	}
	if err := pack.Validate(); err != nil {
		return Pack{}, err
	}
	return pack, nil
}

func BuildPack(bundle Bundle, proposal Proposal) (Pack, error) {
	if err := bundle.Validate(); err != nil {
		return Pack{}, err
	}
	if proposal.Version != ProposalVersion {
		return Pack{}, fmt.Errorf("task lens: unsupported proposal version %d", proposal.Version)
	}
	bundleHash, err := BundleHash(bundle)
	if err != nil {
		return Pack{}, err
	}
	index := newBundleIndex(bundle)
	if err := validateProposalHeader(proposal, index); err != nil {
		return Pack{}, err
	}

	pack := Pack{
		Version: PackVersion, ID: bundle.ID, BundleSHA256: bundleHash,
		Repository: bundle.Repository, Locality: bundle.Locality, Profile: bundle.Profile,
		RoleContract: bundle.RoleContract, RoleCoverage: bundle.RoleCoverage,
		VerificationFrontier: bundle.Verification,
		DecisiveRelationID:   bundle.DecisiveRelationID, CheapExit: bundle.CheapExit,
		StagesSkipped: append([]string(nil), bundle.StagesSkipped...), Budgets: bundle.Budgets,
		LikelyAreas: []Area{}, Anchors: []InvestigationAnchor{}, EvidenceJoins: []EvidenceJoin{},
		WorkingHypothesis: []HypothesisClause{}, ReproduceOrObserve: []Guidance{},
		NextProbes: []Probe{},
		Interpretation: Interpretation{
			Restatement: localRestatement(bundle.Task.Text),
			Kind:        bundle.KindHint, Observable: bundle.ObservableHint,
		},
		TaskObservationConcrete: TaskProvidesConcreteReproductionOrObservation(bundle.Task.Text),
		Verify:                  Verification{Effect: localVerificationEffectForBundle(bundle), Steps: []Guidance{}},
	}
	for _, term := range bundle.Terms {
		if term.Found {
			pack.Interpretation.FoundTerms = append(pack.Interpretation.FoundTerms, term.Text)
		} else {
			pack.Interpretation.UserProvidedOnly = append(pack.Interpretation.UserProvidedOnly, term.Text)
		}
	}

	selected := make(map[string]struct{}, len(proposal.Anchors))
	for _, proposed := range proposal.Anchors {
		anchor, exists := index.anchors[proposed.AnchorID]
		if !exists {
			return Pack{}, fmt.Errorf("task lens: proposal references unknown anchor id")
		}
		if _, duplicate := selected[proposed.AnchorID]; duplicate {
			return Pack{}, fmt.Errorf("task lens: proposal repeats an anchor id")
		}
		if !locallyAllowedAnchorRole(proposed.Role, anchor) || !validText(proposed.Why, 1024, true) ||
			unknownPathInText(proposed.Why, index.paths) {
			return Pack{}, fmt.Errorf("task lens: proposed anchor is invalid")
		}
		selected[proposed.AnchorID] = struct{}{}
		pack.Anchors = append(pack.Anchors, InvestigationAnchor{
			ID: anchor.ID, Path: anchor.Path, Symbol: anchor.Symbol, Section: anchor.Section,
			Package: anchor.Package, Role: proposed.Role, StartLine: anchor.StartLine,
			EndLine: anchor.EndLine, Excerpt: append([]SourceLine(nil), anchor.Excerpt...), Scope: anchor.Scope,
			Why: localAnchorWhy(), EvidenceIDs: append([]string(nil), anchor.EvidenceIDs...),
		})
	}
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(pack.Anchors) < minimumVisible || len(pack.Anchors) > MaxVisibleAnchors {
		return Pack{}, fmt.Errorf("task lens: visible anchor count must be between %d and %d", minimumVisible, MaxVisibleAnchors)
	}

	for _, proposed := range proposal.Areas {
		if !validText(proposed.Label, 256, true) || !validText(proposed.Why, 1024, true) ||
			unknownPathInText(proposed.Label, index.paths) || unknownPathInText(proposed.Why, index.paths) ||
			len(proposed.TargetIDs) == 0 || len(proposed.TargetIDs) > MaxVisibleAnchors {
			return Pack{}, fmt.Errorf("task lens: proposed area is invalid")
		}
		for _, id := range proposed.TargetIDs {
			if _, ok := selected[id]; !ok {
				return Pack{}, fmt.Errorf("task lens: area target is not a visible anchor")
			}
		}
		pack.LikelyAreas = append(pack.LikelyAreas, Area{
			Label: localAreaLabel(proposed.TargetIDs, index), Why: localAreaWhy(),
			TargetIDs: uniqueSorted(proposed.TargetIDs),
		})
	}
	if len(pack.LikelyAreas) > 3 ||
		(len(pack.Anchors) > 0 && len(pack.LikelyAreas) == 0) ||
		(len(pack.Anchors) == 0 && len(pack.LikelyAreas) != 0) {
		return Pack{}, fmt.Errorf("task lens: likely area count must be between 1 and 3")
	}

	if len(proposal.Joins) > MaxEvidenceJoins {
		return Pack{}, fmt.Errorf("task lens: more than %d evidence joins", MaxEvidenceJoins)
	}
	for joinIndex, proposed := range proposal.Joins {
		join, err := buildJoin(proposed, selected, index)
		if err != nil {
			return Pack{}, fmt.Errorf("task lens: join %d: %w", joinIndex, err)
		}
		pack.EvidenceJoins = append(pack.EvidenceJoins, join)
	}
	for clauseIndex, proposed := range proposal.Hypothesis {
		clause, err := buildClause(proposed, selected, index)
		if err != nil {
			return Pack{}, fmt.Errorf("task lens: hypothesis clause %d: %w", clauseIndex, err)
		}
		pack.WorkingHypothesis = append(pack.WorkingHypothesis, clause)
	}
	if len(pack.WorkingHypothesis) == 0 || len(pack.WorkingHypothesis) > MaxHypothesisClauses {
		return Pack{}, fmt.Errorf("task lens: working hypothesis must contain one to %d clauses", MaxHypothesisClauses)
	}
	for stepIndex, proposed := range proposal.ReproduceOrObserve {
		step, err := buildGuidance(proposed, selected, index)
		if err != nil {
			return Pack{}, fmt.Errorf("task lens: reproduce step %d: %w", stepIndex, err)
		}
		pack.ReproduceOrObserve = append(pack.ReproduceOrObserve, step)
	}
	if len(pack.ReproduceOrObserve) == 0 || len(pack.ReproduceOrObserve) > MaxGuidanceSteps {
		return Pack{}, fmt.Errorf("task lens: reproduce or observe guidance must contain one to %d steps", MaxGuidanceSteps)
	}
	for stepIndex, proposed := range proposal.Verify.Steps {
		step, err := buildGuidance(proposed, selected, index)
		if err != nil {
			return Pack{}, fmt.Errorf("task lens: verify step %d: %w", stepIndex, err)
		}
		pack.Verify.Steps = append(pack.Verify.Steps, step)
	}
	if !validText(pack.Verify.Effect, 1024, true) ||
		!pathReferencesGrounded(pack.Verify.Effect, index.paths, index.taskText) ||
		len(pack.Verify.Steps) == 0 || len(pack.Verify.Steps) > MaxGuidanceSteps {
		return Pack{}, fmt.Errorf("task lens: verification guidance is invalid")
	}
	for probeIndex, proposed := range proposal.NextProbes {
		probe, err := buildProbe(proposed, selected, index.paths)
		if err != nil {
			return Pack{}, fmt.Errorf("task lens: next probe %d: %w", probeIndex, err)
		}
		pack.NextProbes = append(pack.NextProbes, probe)
	}
	if len(pack.NextProbes) == 0 || len(pack.NextProbes) > MaxNextProbes {
		return Pack{}, fmt.Errorf("task lens: next probes must contain one to %d items", MaxNextProbes)
	}
	if err := pack.Validate(); err != nil {
		return Pack{}, err
	}
	return pack, nil
}

// ValidatePackAgainstBundle replays the reducer contract over a saved pack.
// It prevents a pack whose own hashes were recomputed after modification from
// weakening anchor, evidence, relation, or guidance authority when rendered.
func ValidatePackAgainstBundle(bundle Bundle, pack Pack) error {
	if err := pack.Validate(); err != nil {
		return err
	}
	if len(pack.Warnings) > 1 {
		return fmt.Errorf("task lens: invalid pack warning count")
	}
	for _, warning := range pack.Warnings {
		if warning != PackWarningLocalPartial && warning != PackWarningModelPartial {
			return fmt.Errorf("task lens: invalid pack warning")
		}
	}
	proposal := Proposal{
		Version: ProposalVersion,
		Interpretation: ProposedInterpretation{
			Restatement: pack.Interpretation.Restatement,
			Kind:        pack.Interpretation.Kind,
			Observable:  pack.Interpretation.Observable,
		},
		Verify: ProposedVerification{Effect: pack.Verify.Effect},
	}
	for _, area := range pack.LikelyAreas {
		proposal.Areas = append(proposal.Areas, ProposedArea{
			Label: area.Label, Why: area.Why,
			TargetIDs: append([]string(nil), area.TargetIDs...),
		})
	}
	for _, anchor := range pack.Anchors {
		proposal.Anchors = append(proposal.Anchors, ProposedAnchor{
			AnchorID: anchor.ID, Role: anchor.Role, Why: anchor.Why,
		})
	}
	for _, join := range pack.EvidenceJoins {
		proposal.Joins = append(proposal.Joins, ProposedJoin{
			LeftID: join.LeftID, RightID: join.RightID, RelationID: join.RelationID,
			Kind: join.Kind, SupportType: join.SupportType,
			SupportIDs:  append([]string(nil), join.SupportIDs...),
			Explanation: join.Explanation, Scope: join.Scope,
		})
	}
	for _, clause := range pack.WorkingHypothesis {
		proposal.Hypothesis = append(proposal.Hypothesis, ProposedClause{
			Status: clause.Status, Text: clause.Text,
			SupportIDs:  append([]string(nil), clause.SupportIDs...),
			RelationIDs: append([]string(nil), clause.RelationIDs...),
		})
	}
	for _, guidance := range pack.ReproduceOrObserve {
		proposal.ReproduceOrObserve = append(proposal.ReproduceOrObserve, ProposedGuidance{
			Text: guidance.Text, Authority: guidance.Authority,
			EvidenceIDs: append([]string(nil), guidance.EvidenceIDs...),
		})
	}
	for _, guidance := range pack.Verify.Steps {
		proposal.Verify.Steps = append(proposal.Verify.Steps, ProposedGuidance{
			Text: guidance.Text, Authority: guidance.Authority,
			EvidenceIDs: append([]string(nil), guidance.EvidenceIDs...),
		})
	}
	for _, probe := range pack.NextProbes {
		proposal.NextProbes = append(proposal.NextProbes, ProposedProbe{
			Action: probe.Action, AnchorIDs: append([]string(nil), probe.AnchorIDs...), Text: probe.Text,
		})
	}
	rebuilt, err := BuildPack(bundle, proposal)
	if err != nil {
		return fmt.Errorf("task lens: replay saved pack: %w", err)
	}
	warnings := pack.Warnings
	pack.Warnings = nil
	rebuilt.Warnings = nil
	if !reflect.DeepEqual(rebuilt, pack) {
		return fmt.Errorf("task lens: saved pack does not match its bounded bundle")
	}
	pack.Warnings = warnings
	return nil
}

// ReduceProposal preserves the valid portion of a substantive model response
// while dropping individual joins, clauses, guidance steps, or probes that do
// not pass the same strict local checks used by BuildPack. Structural failures
// in interpretation or anchor selection still reject the whole response because
// later references cannot be interpreted safely. Presentation fields on known
// selected anchors and their likely-area grouping may be replaced from exact
// local authority.
func ReduceProposal(bundle Bundle, proposal Proposal) (Pack, []string, error) {
	return reduceProposal(bundle, proposal, nil)
}

// ReduceProposalWithDiagnostics runs the same reducer as ReduceProposal and
// returns the fixed product-message identities emitted alongside its raw
// canonical warnings. It is used when reconstructing terminal presentation
// from replayable Task Lens artifacts; it does not change those artifacts.
func ReduceProposalWithDiagnostics(
	bundle Bundle,
	proposal Proposal,
) (Pack, []string, []WarningDiagnostic, error) {
	var diagnostics []WarningDiagnostic
	pack, warnings, err := reduceProposal(bundle, proposal, &diagnostics)
	return pack, warnings, diagnostics, err
}

func reduceProposal(
	bundle Bundle,
	proposal Proposal,
	diagnostics *[]WarningDiagnostic,
) (Pack, []string, error) {
	if err := bundle.Validate(); err != nil {
		return Pack{}, nil, err
	}
	if proposal.Version != ProposalVersion {
		return Pack{}, nil, fmt.Errorf("task lens: unsupported proposal version %d", proposal.Version)
	}
	index := newBundleIndex(bundle)
	if err := validateProposalHeader(proposal, index); err != nil {
		return Pack{}, nil, err
	}
	reduced := proposal
	var warnings []string
	warning := reductionWarningCollector{raw: &warnings, diagnostics: diagnostics}
	selected := make(map[string]struct{}, len(proposal.Anchors))
	seenProposed := make(map[string]struct{}, len(proposal.Anchors))
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(proposal.Anchors) < minimumVisible || len(proposal.Anchors) > MaxVisibleAnchors {
		return Pack{}, nil, fmt.Errorf("task lens: visible anchor count must be between %d and %d", minimumVisible, MaxVisibleAnchors)
	}
	relevantIDs := taskRelevantBundleAnchorIDs(bundle)
	relevantSelected := 0
	for _, proposed := range proposal.Anchors {
		if _, relevant := relevantIDs[proposed.AnchorID]; relevant {
			relevantSelected++
		}
	}
	canOmitIrrelevant := relevantSelected >= minimumVisible
	reduced.Anchors = make([]ProposedAnchor, 0, len(proposal.Anchors))
	for itemIndex, original := range proposal.Anchors {
		proposed := original
		anchor, exists := index.anchors[proposed.AnchorID]
		if !exists {
			return Pack{}, nil, fmt.Errorf("task lens: proposal references unknown anchor id")
		}
		if _, duplicate := seenProposed[proposed.AnchorID]; duplicate {
			return Pack{}, nil, fmt.Errorf("task lens: proposal repeats an anchor id")
		}
		seenProposed[proposed.AnchorID] = struct{}{}
		if _, relevant := relevantIDs[proposed.AnchorID]; !relevant && canOmitIrrelevant {
			warning.add(WarningAnchorOmittedIrrelevant, itemIndex+1, fmt.Sprintf(
				"Anchor %d was omitted because local evidence did not ground it in the task, a required or supporting role, the decisive component, or exact verification.",
				itemIndex+1,
			))
			continue
		}
		if !locallyAllowedAnchorRole(proposed.Role, anchor) {
			proposed.Role = anchor.RoleHints[0]
			proposed.Why = localAnchorWhy()
			warning.add(WarningAnchorRoleReplaced, itemIndex+1, fmt.Sprintf(
				"Anchor %d was assigned an exact locally allowed role and its explanation was replaced with local wording.",
				itemIndex+1,
			))
		} else if !validText(proposed.Why, 1024, true) || unknownPathInText(proposed.Why, index.paths) {
			proposed.Why = localAnchorWhy()
			warning.add(WarningAnchorExplanationReplaced, itemIndex+1, fmt.Sprintf(
				"Anchor %d explanation was replaced with local wording because its presentation text was not grounded.",
				itemIndex+1,
			))
		}
		selected[proposed.AnchorID] = struct{}{}
		reduced.Anchors = append(reduced.Anchors, proposed)
	}
	reduced.Areas = nil
	for itemIndex, area := range proposal.Areas {
		targetIDs := make([]string, 0, len(area.TargetIDs))
		seenTargets := make(map[string]struct{}, len(area.TargetIDs))
		for _, id := range area.TargetIDs {
			if _, visible := selected[id]; !visible {
				continue
			}
			if _, duplicate := seenTargets[id]; duplicate {
				continue
			}
			seenTargets[id] = struct{}{}
			targetIDs = append(targetIDs, id)
		}
		if !slices.Equal(targetIDs, area.TargetIDs) {
			warning.add(WarningAreaTargetsFiltered, itemIndex+1, fmt.Sprintf(
				"Likely area %d target IDs were filtered to unique selected anchors.",
				itemIndex+1,
			))
		}
		if len(targetIDs) == 0 {
			warning.add(WarningAreaOmittedWithoutAnchor, itemIndex+1, fmt.Sprintf(
				"Likely area %d was omitted because it did not retain a selected anchor.",
				itemIndex+1,
			))
			continue
		}
		if len(reduced.Areas) == 3 {
			warning.add(WarningAreasBounded, 0, "Additional likely areas were omitted at the local presentation bound.")
			break
		}
		area.TargetIDs = targetIDs
		if !validText(area.Label, 256, true) || !validText(area.Why, 1024, true) ||
			unknownPathInText(area.Label, index.paths) || unknownPathInText(area.Why, index.paths) {
			area.Label = localAreaLabel(targetIDs, index)
			area.Why = localAreaWhy()
			warning.add(WarningAreaCopyReplaced, itemIndex+1, fmt.Sprintf(
				"Likely area %d label or explanation was replaced with local wording.",
				itemIndex+1,
			))
		}
		reduced.Areas = append(reduced.Areas, area)
	}
	if len(reduced.Areas) == 0 && len(selected) > 0 {
		reduced.Areas = localFallbackAreas(bundle, selected, index)
		warning.add(WarningAreaFallbackAdded, 0, "A deterministic local likely area was added because no model area retained a selected anchor.")
	}

	reduced.Joins = nil
	reduced.Hypothesis = nil
	reduced.ReproduceOrObserve = nil
	reduced.Verify.Steps = nil
	reduced.NextProbes = nil
	for itemIndex, item := range proposal.Joins {
		if itemIndex >= MaxEvidenceJoins {
			warning.add(WarningJoinsBounded, 0, "Additional evidence joins were omitted at the local presentation bound.")
			break
		}
		if _, err := buildJoin(item, selected, index); err != nil {
			warning.add(WarningJoinRejected, itemIndex+1, fmt.Sprintf("Evidence join %d was rejected locally: %v.", itemIndex+1, err))
			continue
		}
		reduced.Joins = append(reduced.Joins, item)
	}
	hasSubstantiveHypothesis := false
	for itemIndex, item := range proposal.Hypothesis {
		if itemIndex >= MaxHypothesisClauses {
			warning.add(WarningHypothesesBounded, 0, "Additional hypothesis clauses were omitted at the local presentation bound.")
			break
		}
		normalized, completedRelationEvidence := completeHypothesisRelationEvidence(item, selected, index)
		clause, err := buildClause(normalized, selected, index)
		if err != nil {
			warning.add(WarningHypothesisRejected, itemIndex+1, fmt.Sprintf("Hypothesis clause %d was rejected locally: %v.", itemIndex+1, err))
			continue
		}
		if completedRelationEvidence {
			warning.add(WarningHypothesisSupportCompleted, itemIndex+1, fmt.Sprintf(
				"Hypothesis clause %d support was completed from exact local relation evidence.",
				itemIndex+1,
			))
		}
		if clause.Text != normalized.Text {
			warning.add(WarningHypothesisCopyReplaced, itemIndex+1, fmt.Sprintf(
				"Hypothesis clause %d prose was replaced with calibrated local wording because its references were not fully grounded.",
				itemIndex+1,
			))
		}
		reduced.Hypothesis = append(reduced.Hypothesis, normalized)
		if clause.Status != HypothesisUnresolved {
			hasSubstantiveHypothesis = true
		}
	}
	if !hasSubstantiveHypothesis {
		fallback, relationBound := localFallbackHypothesis(bundle, selected, index)
		if len(reduced.Hypothesis) == 0 ||
			(relationBound && len(reduced.Hypothesis) < MaxHypothesisClauses) {
			if _, err := buildClause(fallback, selected, index); err != nil {
				return Pack{}, warnings, fmt.Errorf("task lens: build bounded local hypothesis fallback: %w", err)
			}
			reduced.Hypothesis = append(reduced.Hypothesis, fallback)
			warning.add(WarningHypothesisFallbackAdded, 0, "A bounded exact local hypothesis fallback was added because no substantive model clause passed local reduction.")
		}
	}
	var retainedReproduction []Guidance
	for itemIndex, item := range proposal.ReproduceOrObserve {
		if itemIndex >= MaxGuidanceSteps {
			warning.add(WarningReproductionBounded, 0, "Additional reproduction steps were omitted at the local presentation bound.")
			break
		}
		guidance, err := buildGuidance(item, selected, index)
		if err != nil {
			warning.add(WarningReproductionRejected, itemIndex+1, fmt.Sprintf("Reproduction step %d was rejected locally: %v.", itemIndex+1, err))
			continue
		}
		if containsGuidance(retainedReproduction, guidance) {
			warning.add(WarningReproductionDuplicate, itemIndex+1, fmt.Sprintf(
				"Reproduction step %d was omitted because it duplicates the same locally authoritative guidance.",
				itemIndex+1,
			))
			continue
		}
		retainedReproduction = append(retainedReproduction, guidance)
		reduced.ReproduceOrObserve = append(reduced.ReproduceOrObserve, item)
	}
	if len(reduced.ReproduceOrObserve) == 0 {
		fallback := localFallbackReproduction(bundle, selected)
		if _, err := buildGuidance(fallback, selected, index); err != nil {
			return Pack{}, warnings, fmt.Errorf("task lens: build bounded local reproduction fallback: %w", err)
		}
		reduced.ReproduceOrObserve = []ProposedGuidance{fallback}
		warning.add(WarningReproductionFallbackAdded, 0, "No model reproduction or observation step passed local reduction; a bounded local reproduction or observation fallback was used.")
	}
	// effect_to_observe is presentation text derived from the task-provided
	// observable. The model may organize verification steps, but it cannot
	// establish a new expected effect.
	reduced.Verify.Effect = localVerificationEffectForBundle(bundle)
	var retainedVerification []Guidance
	for itemIndex, item := range proposal.Verify.Steps {
		if itemIndex >= MaxGuidanceSteps {
			warning.add(WarningVerificationBounded, 0, "Additional verification steps were omitted at the local presentation bound.")
			break
		}
		if !verificationGuidanceUsesExactFrontier(item, bundle, selected) {
			warning.add(WarningVerificationOutsideFrontier, itemIndex+1, fmt.Sprintf(
				"Verification step %d was rejected locally because its evidence is outside the exact verification frontier.",
				itemIndex+1,
			))
			continue
		}
		guidance, err := buildGuidance(item, selected, index)
		if err != nil {
			warning.add(WarningVerificationRejected, itemIndex+1, fmt.Sprintf("Verification step %d was rejected locally: %v.", itemIndex+1, err))
			continue
		}
		if containsGuidance(retainedVerification, guidance) {
			warning.add(WarningVerificationDuplicate, itemIndex+1, fmt.Sprintf(
				"Verification step %d was omitted because it duplicates the same locally authoritative guidance.",
				itemIndex+1,
			))
			continue
		}
		retainedVerification = append(retainedVerification, guidance)
		reduced.Verify.Steps = append(reduced.Verify.Steps, item)
	}
	if len(reduced.Verify.Steps) == 0 {
		fallback := localFallbackVerification(bundle, selected)
		if _, err := buildGuidance(fallback, selected, index); err != nil {
			return Pack{}, warnings, fmt.Errorf("task lens: build bounded local verification fallback: %w", err)
		}
		reduced.Verify.Steps = []ProposedGuidance{fallback}
		warning.add(WarningVerificationFallbackAdded, 0, "No model verification step passed local reduction; a bounded repository-owned verification or missing-evidence fallback was used.")
	}
	if !hasRepositoryBackedGuidance(reduced.Verify.Steps) {
		fallback := localFallbackVerification(bundle, selected)
		if hasRepositoryBackedGuidance([]ProposedGuidance{fallback}) {
			if _, err := buildGuidance(fallback, selected, index); err != nil {
				return Pack{}, warnings, fmt.Errorf("task lens: build exact-frontier verification fallback: %w", err)
			}
			if len(reduced.Verify.Steps) < MaxGuidanceSteps {
				reduced.Verify.Steps = append(reduced.Verify.Steps, fallback)
			} else {
				replaceIndex := len(reduced.Verify.Steps) - 1
				for index := len(reduced.Verify.Steps) - 1; index >= 0; index-- {
					if reduced.Verify.Steps[index].Authority == AuthorityMissing {
						replaceIndex = index
						break
					}
				}
				reduced.Verify.Steps[replaceIndex] = fallback
			}
			code := WarningVerificationAuthorityAdded
			message := "An exact-frontier repository verification item was added because the surviving model steps had no bound repository authority."
			if fallback.Authority == AuthorityRepositoryTest {
				code = WarningVerificationTestAuthorityAdded
				message = "A selected repository test or example from the exact verification frontier was added because the surviving model steps had no bound repository authority."
			}
			warning.add(code, 0, message)
		}
	}
	for itemIndex, item := range proposal.NextProbes {
		if itemIndex >= MaxNextProbes {
			warning.add(WarningNextProbesBounded, 0, "Additional next probes were omitted at the local presentation bound.")
			break
		}
		if _, err := buildProbe(item, selected, index.paths); err != nil {
			warning.add(WarningNextProbeRejected, itemIndex+1, fmt.Sprintf("Next probe %d was rejected locally: %v.", itemIndex+1, err))
			continue
		}
		reduced.NextProbes = append(reduced.NextProbes, item)
	}
	if len(reduced.NextProbes) == 0 {
		fallback := localFallbackProbe(bundle, selected)
		if _, err := buildProbe(fallback, selected, index.paths); err != nil {
			return Pack{}, warnings, fmt.Errorf("task lens: build bounded local next probe fallback: %w", err)
		}
		reduced.NextProbes = []ProposedProbe{fallback}
		warning.add(WarningNextProbeFallbackAdded, 0, "No model next probe passed local reduction; a bounded local next probe fallback was used.")
	}
	pack, err := BuildPack(bundle, reduced)
	if err != nil {
		return Pack{}, warnings, err
	}
	pack.Warnings = append(pack.Warnings, warnings...)
	return pack, warnings, nil
}

func completeHypothesisRelationEvidence(
	proposed ProposedClause,
	selected map[string]struct{},
	index bundleIndex,
) (ProposedClause, bool) {
	normalized := proposed
	normalized.SupportIDs = uniqueSorted(proposed.SupportIDs)
	provided := append([]string(nil), normalized.SupportIDs...)
	for _, relationID := range proposed.RelationIDs {
		relation, ok := index.relations[relationID]
		if !ok {
			continue
		}
		if _, visible := selected[relation.LeftID]; !visible {
			continue
		}
		if _, visible := selected[relation.RightID]; !visible {
			continue
		}
		normalized.SupportIDs = append(normalized.SupportIDs, relation.EvidenceIDs...)
	}
	normalized.SupportIDs = uniqueSorted(normalized.SupportIDs)
	return normalized, !slices.Equal(provided, normalized.SupportIDs)
}

func localFallbackHypothesis(
	bundle Bundle,
	selected map[string]struct{},
	index bundleIndex,
) (ProposedClause, bool) {
	if relations := rankedLocalProposalRelations(bundle, selected); len(relations) > 0 {
		relation := relations[0]
		return ProposedClause{
			Status: HypothesisSupported, Text: localRelationExplanation(relation.Kind),
			SupportIDs: append([]string(nil), relation.EvidenceIDs...), RelationIDs: []string{relation.ID},
		}, true
	}
	for _, anchor := range bundle.Anchors {
		if _, visible := selected[anchor.ID]; !visible || len(anchor.EvidenceIDs) == 0 {
			continue
		}
		return ProposedClause{
			Status: HypothesisPlausible, Text: plausibleClauseText(anchor.EvidenceIDs, index),
			SupportIDs: append([]string(nil), anchor.EvidenceIDs...),
		}, false
	}
	return ProposedClause{
		Status: HypothesisUnresolved,
		Text:   "No tracked source or document anchor matched the bounded exact-term retrieval; no repository mechanism is asserted.",
	}, false
}

func containsGuidance(values []Guidance, candidate Guidance) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}

func hasRepositoryBackedGuidance(values []ProposedGuidance) bool {
	for _, value := range values {
		switch value.Authority {
		case AuthorityRepositoryDocument, AuthorityRepositoryTest, AuthorityRepositoryObservation:
			return true
		}
	}
	return false
}

func localFallbackAreas(bundle Bundle, selected map[string]struct{}, index bundleIndex) []ProposedArea {
	targetIDs := make([]string, 0, len(selected))
	for _, anchor := range bundle.Anchors {
		if _, visible := selected[anchor.ID]; visible {
			targetIDs = append(targetIDs, anchor.ID)
		}
	}
	if len(targetIDs) == 0 {
		return nil
	}
	return []ProposedArea{{
		Label: localAreaLabel(targetIDs, index), Why: localAreaWhy(), TargetIDs: targetIDs,
	}}
}

func selectedBundleAnchors(bundle Bundle, selected map[string]struct{}) []Anchor {
	anchors := make([]Anchor, 0, len(selected))
	for _, anchor := range bundle.Anchors {
		if _, visible := selected[anchor.ID]; visible {
			anchors = append(anchors, anchor)
		}
	}
	return anchors
}

func localFallbackReproduction(bundle Bundle, selected map[string]struct{}) ProposedGuidance {
	if TaskProvidesConcreteReproductionOrObservation(bundle.Task.Text) {
		return ProposedGuidance{
			Text:      "Use the reproduction or observation already supplied by the task.",
			Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID},
		}
	}
	anchors := selectedBundleAnchors(bundle, selected)
	if authority, evidenceID := firstLocalObservationEvidence(anchors, bundle); evidenceID != "" {
		return ProposedGuidance{
			Text:      "Observe the exact retained repository evidence without treating it as executed behavior.",
			Authority: authority, EvidenceIDs: []string{evidenceID},
		}
	}
	return ProposedGuidance{
		Text: "No concrete reproduction or repository observation was retained.", Authority: AuthorityMissing,
	}
}

func localFallbackVerification(bundle Bundle, selected map[string]struct{}) ProposedGuidance {
	return localFrontierVerification(bundle, selected)
}

func verificationGuidanceUsesExactFrontier(
	guidance ProposedGuidance,
	bundle Bundle,
	selected map[string]struct{},
) bool {
	switch guidance.Authority {
	case AuthorityRepositoryDocument, AuthorityRepositoryTest, AuthorityRepositoryObservation:
	default:
		return true
	}
	if len(guidance.EvidenceIDs) == 0 {
		return false
	}
	exactEvidence := make(map[string]struct{})
	for _, item := range bundle.Verification.allItems() {
		switch item.Authority {
		case VerificationExactExistingTest,
			VerificationExactGeneratedFixture,
			VerificationExactExample,
			VerificationDocumentedCommand:
		default:
			continue
		}
		if item.AnchorID != "" {
			if _, visible := selected[item.AnchorID]; !visible {
				continue
			}
		}
		for _, evidenceID := range item.EvidenceIDs {
			exactEvidence[evidenceID] = struct{}{}
		}
	}
	for _, evidenceID := range guidance.EvidenceIDs {
		if _, exact := exactEvidence[evidenceID]; !exact {
			return false
		}
	}
	return true
}

func localFallbackProbe(bundle Bundle, selected map[string]struct{}) ProposedProbe {
	for _, anchor := range bundle.Anchors {
		if _, visible := selected[anchor.ID]; visible {
			return ProposedProbe{
				Action: ProbeInspectSymbol, AnchorIDs: []string{anchor.ID},
				Text: "Inspect this exact symbol and resolve one task-relevant reference beyond the retained excerpt.",
			}
		}
	}
	return ProposedProbe{
		Action: ProbeSearchTaskTerms, AnchorIDs: []string{},
		Text: "Search one exact task term within the bounded tracked repository.",
	}
}

func locallyAllowedAnchorRole(role AnchorRole, anchor Anchor) bool {
	return slices.Contains(anchor.RoleHints, role)
}

func localAnchorWhy() string {
	return "Exact retained repository evidence selected by bounded task-conditioned retrieval."
}

func localAreaWhy() string {
	return "Groups exact selected anchors from bounded task-conditioned retrieval."
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return strings.TrimSpace(value[:maxBytes])
}

func localAreaLabel(targetIDs []string, index bundleIndex) string {
	directories := make([]string, 0, len(targetIDs))
	for _, id := range targetIDs {
		directory := path.Dir(index.anchors[id].Path)
		if directory == "." {
			directory = "repository root"
		}
		directories = append(directories, directory)
	}
	directories = uniqueSorted(directories)
	if len(directories) == 1 {
		return directories[0]
	}
	return "Cross-file anchors"
}

func (bundle Bundle) Validate() error {
	if bundle.Version != BundleVersion || !validOpaque(bundle.ID) {
		return fmt.Errorf("task lens: invalid bundle header")
	}
	if err := validateRepository(bundle.Repository); err != nil {
		return err
	}
	if bundle.ID != OpaqueID(
		"task", bundle.Repository.Identity, bundle.Repository.Revision,
		bundle.Repository.StateSHA256, SHA256([]byte(bundle.Task.Text)),
	) {
		return fmt.Errorf("task lens: bundle identity does not match repository state and task")
	}
	if !validTaskKind(bundle.KindHint) || !validLocality(bundle.Locality) ||
		!validText(bundle.ObservableHint, maxTextBytes, true) ||
		len(bundle.Task.Text) == 0 || len(bundle.Task.Text) > maxTaskBytes ||
		!utf8.ValidString(bundle.Task.Text) || !validOpaque(bundle.Task.EvidenceID) {
		return fmt.Errorf("task lens: invalid task input")
	}
	if bundle.KindHint != classifyTaskKind(bundle.Task.Text) ||
		bundle.ObservableHint != taskObservable(bundle.Task.Text) {
		return fmt.Errorf("task lens: task classification does not match deterministic extraction")
	}
	if kind, found := secretscan.Detect(bundle.Task.Text); found {
		return fmt.Errorf("task lens: task rejected because %s was detected", kind)
	}
	if !slices.Equal(bundle.StagesSkipped, taskLensStagesSkipped) {
		return fmt.Errorf("task lens: skipped stages do not match the dedicated pipeline")
	}
	if len(bundle.Anchors) > MaxRetainedAnchors ||
		len(bundle.Evidence) != len(bundle.Anchors)+1 {
		return fmt.Errorf("task lens: bundle collection is outside bounds")
	}
	if err := validateBudget(bundle.Budgets); err != nil {
		return err
	}
	if bundle.Budgets.RetainedAnchors != len(bundle.Anchors) ||
		bundle.Budgets.RetainedSourceBytes != retainedAnchorSourceBytes(bundle.Anchors) ||
		bundle.Budgets.ReadFiles < len(bundle.AllowedPaths) ||
		bundle.Budgets.EvidenceFilesConsidered > bundle.Budgets.InitialCandidates ||
		bundle.Budgets.InitialCandidates != min(bundle.Budgets.CandidateItemsFound, MaxInitialCandidates) ||
		(bundle.Budgets.ReadFiles > 0 && bundle.Budgets.ReadBytes == 0) ||
		(bundle.Budgets.ReadFiles == 0 && bundle.Budgets.ReadBytes != 0) ||
		(bundle.Budgets.SourceScanLimitBound && bundle.Budgets.SourceScanBytes != MaxSourceScanBytes) ||
		bundle.Budgets.CandidateLimitBound != (bundle.Budgets.CandidateItemsFound > MaxInitialCandidates) ||
		bundle.Budgets.AnchorLimitBound != (bundle.Budgets.AnchorItemsFound > MaxRetainedAnchors) ||
		(bundle.Budgets.FileLimitBound &&
			(bundle.Budgets.ReadFiles != MaxReadFiles ||
				bundle.Metrics.EvidenceFilesRead >= bundle.Budgets.EvidenceFilesConsidered)) ||
		(bundle.Budgets.ByteLimitBound && bundle.Budgets.ReadBytes != MaxReadBytes) ||
		(bundle.Budgets.RetainedByteLimitBound && bundle.Budgets.AnchorItemsFound <= bundle.Budgets.RetainedAnchors) {
		return fmt.Errorf("task lens: bundle budget accounting is inconsistent")
	}
	if len(bundle.Modules) > MaxModules {
		return fmt.Errorf("task lens: module index is outside bounds")
	}
	if bundle.Metrics.ModuleFilesFound < 0 ||
		bundle.Metrics.ModuleFilesRead < len(bundle.Modules) ||
		bundle.Metrics.ModuleFilesRead > MaxModules ||
		bundle.Metrics.ModuleFilesFound < bundle.Metrics.ModuleFilesRead ||
		bundle.Metrics.ModuleBytesRead < 0 ||
		bundle.Metrics.ModuleBytesRead > bundle.Metrics.ManifestBytesRead ||
		(bundle.Metrics.ModuleFilesRead > 0 && bundle.Metrics.ModuleBytesRead == 0) ||
		bundle.Metrics.ManifestFilesRead < bundle.Metrics.ModuleFilesRead ||
		bundle.Metrics.ManifestFilesRead > bundle.Budgets.ReadFiles ||
		bundle.Metrics.ManifestBytesRead < bundle.Metrics.ModuleBytesRead ||
		bundle.Metrics.ManifestBytesRead > bundle.Budgets.ReadBytes ||
		(bundle.Metrics.ManifestFilesRead > 0 && bundle.Metrics.ManifestBytesRead == 0) {
		return fmt.Errorf("task lens: module retrieval accounting is invalid")
	}
	if bundle.Metrics.TrackedFiles < len(bundle.AllowedPaths) ||
		bundle.Budgets.CandidateItemsFound > bundle.Metrics.TrackedFiles ||
		bundle.Metrics.EvidenceFilesRead > bundle.Budgets.EvidenceFilesConsidered ||
		bundle.Metrics.EvidenceFilesRead > bundle.Budgets.ReadFiles ||
		bundle.Metrics.GitGrepQueries < 0 || bundle.Metrics.GitGrepQueries > maxGrepTerms ||
		bundle.Metrics.ASTParses < 0 || bundle.Metrics.ASTParses > bundle.Metrics.EvidenceFilesRead ||
		bundle.Metrics.RelationsRetained != len(bundle.Relations) {
		return fmt.Errorf("task lens: retrieval accounting is invalid")
	}
	moduleIDs := make(map[string]struct{}, len(bundle.Modules))
	moduleDirs := make(map[string]struct{}, len(bundle.Modules))
	var rootModule *Module
	for _, module := range bundle.Modules {
		expectedSourcePath := "go.mod"
		if module.Dir != "." {
			expectedSourcePath = path.Join(module.Dir, "go.mod")
		}
		if !validOpaque(module.ID) || module.ID != OpaqueID("module", module.Path, module.Dir) ||
			!validText(module.Path, 512, true) || strings.ContainsAny(module.Path, " \t\n\r\\") ||
			(module.Dir != "." && !validPath(module.Dir)) || module.SourcePath != expectedSourcePath {
			return fmt.Errorf("task lens: invalid module index entry")
		}
		if _, duplicate := moduleIDs[module.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate module id")
		}
		if _, duplicate := moduleDirs[module.Dir]; duplicate {
			return fmt.Errorf("task lens: duplicate module directory")
		}
		if kind, found := secretscan.Detect(module.Path + "\n" + module.Dir); found {
			return fmt.Errorf("task lens: module index rejected because %s was detected", kind)
		}
		moduleIDs[module.ID] = struct{}{}
		moduleDirs[module.Dir] = struct{}{}
		if module.Dir == "." {
			copy := module
			rootModule = &copy
		}
	}
	switch bundle.Repository.IdentitySource {
	case "root_module":
		if rootModule == nil || bundle.Repository.Identity != rootModule.Path ||
			bundle.Repository.IdentitySourcePath != rootModule.SourcePath {
			return fmt.Errorf("task lens: root module identity is not source-bound")
		}
	case "manifest":
		if bundle.Repository.IdentitySourcePath != "pyproject.toml" &&
			bundle.Repository.IdentitySourcePath != "package.json" &&
			bundle.Repository.IdentitySourcePath != "Cargo.toml" {
			return fmt.Errorf("task lens: manifest identity is not source-bound")
		}
	case "remote", "neutral_fallback":
		if bundle.Repository.IdentitySourcePath != "" {
			return fmt.Errorf("task lens: repository identity has an unexpected source path")
		}
	}
	allowed := make(map[string]struct{}, len(bundle.AllowedPaths))
	previousPath := ""
	for _, filePath := range bundle.AllowedPaths {
		if !validPath(filePath) || (previousPath != "" && filePath <= previousPath) {
			return fmt.Errorf("task lens: allowed paths must be valid, unique, and sorted")
		}
		previousPath = filePath
		allowed[filePath] = struct{}{}
	}
	expectedAllowed := make([]string, 0, len(bundle.Anchors)+len(bundle.Modules)+1)
	for _, anchor := range bundle.Anchors {
		expectedAllowed = append(expectedAllowed, anchor.Path)
	}
	for _, module := range bundle.Modules {
		expectedAllowed = append(expectedAllowed, module.SourcePath)
	}
	if bundle.Repository.IdentitySourcePath != "" {
		expectedAllowed = append(expectedAllowed, bundle.Repository.IdentitySourcePath)
	}
	expectedAllowed = uniqueSorted(expectedAllowed)
	if !slices.Equal(bundle.AllowedPaths, expectedAllowed) {
		return fmt.Errorf("task lens: allowed paths do not match exact model-visible sources")
	}
	evidence := make(map[string]Evidence, len(bundle.Evidence))
	taskEvidenceCount := 0
	for _, item := range bundle.Evidence {
		if !validOpaque(item.ID) || !validEvidenceKind(item.Kind) ||
			!validText(item.Summary, 1024, true) {
			return fmt.Errorf("task lens: invalid evidence")
		}
		if _, duplicate := evidence[item.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate evidence id")
		}
		if item.Kind == EvidenceTaskProvided {
			taskEvidenceCount++
			if item.ID != bundle.Task.EvidenceID ||
				item.ID != OpaqueID("evidence", "task", SHA256([]byte(bundle.Task.Text))) ||
				item.Path != "" || item.StartLine != 0 || item.EndLine != 0 || item.AnchorID != "" ||
				item.Summary != "Symptom or requested outcome supplied by the task; not repository truth." {
				return fmt.Errorf("task lens: invalid task-provided evidence")
			}
		} else if _, ok := allowed[item.Path]; !ok || item.StartLine <= 0 || item.EndLine < item.StartLine {
			return fmt.Errorf("task lens: repository evidence is outside allowed paths")
		}
		evidence[item.ID] = item
	}
	taskEvidence, ok := evidence[bundle.Task.EvidenceID]
	if !ok || taskEvidenceCount != 1 || taskEvidence.Kind != EvidenceTaskProvided || taskEvidence.AnchorID != "" {
		return fmt.Errorf("task lens: exact task-provided evidence is missing")
	}
	anchors := make(map[string]Anchor, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		if !validOpaque(anchor.ID) || !validPath(anchor.Path) || !validText(anchor.Symbol, 256, true) ||
			anchor.StartLine <= 0 || anchor.EndLine < anchor.StartLine ||
			len(anchor.Excerpt) == 0 || len(anchor.Excerpt) > 4096 ||
			anchorExcerptBytes(anchor) > maxCompleteFileBytes || len(anchor.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: invalid anchor")
		}
		if err := anchor.Scope.Validate(); err != nil ||
			anchor.Scope.ScopeStart != anchor.StartLine ||
			anchor.Scope.ScopeEnd != anchor.EndLine {
			return fmt.Errorf("task lens: invalid anchor source scope")
		}
		if anchor.ID != OpaqueID(
			"anchor", anchor.Path, anchor.Symbol,
			fmt.Sprintf("%d", anchor.StartLine), fmt.Sprintf("%d", anchor.EndLine),
			SourceExcerptSHA256(anchor.Excerpt),
		) {
			return fmt.Errorf("task lens: anchor identity does not match its exact source bounds")
		}
		if _, ok := allowed[anchor.Path]; !ok {
			return fmt.Errorf("task lens: anchor path is not allowed")
		}
		if _, duplicate := anchors[anchor.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate anchor id")
		}
		fragmentScope := anchor.Scope.ScopeKind == SourceScopeMatchedFragments ||
			anchor.Scope.ScopeKind == SourceScopePartialWindow
		for index, line := range anchor.Excerpt {
			outOfOrder := index > 0 && line.Line <= anchor.Excerpt[index-1].Line
			unexpectedGap := index > 0 && !fragmentScope &&
				line.Line != anchor.Excerpt[index-1].Line+1
			if line.Line < anchor.StartLine || line.Line > anchor.EndLine ||
				outOfOrder || unexpectedGap ||
				!utf8.ValidString(line.Text) || strings.ContainsAny(line.Text, "\x00\r\n") {
				return fmt.Errorf("task lens: invalid anchor excerpt")
			}
			if kind, found := secretscan.Detect(line.Text); found {
				return fmt.Errorf("task lens: anchor rejected because %s was detected", kind)
			}
		}
		if anchor.Excerpt[0].Line != anchor.StartLine || anchor.Excerpt[len(anchor.Excerpt)-1].Line != anchor.EndLine {
			return fmt.Errorf("task lens: anchor excerpt bounds do not match")
		}
		expectedEvidenceID := OpaqueID(
			"evidence", bundle.Repository.StateSHA256, anchor.Path,
			fmt.Sprintf("%d", anchor.StartLine), fmt.Sprintf("%d", anchor.EndLine),
			SourceExcerptSHA256(anchor.Excerpt),
		)
		if !slices.Equal(anchor.EvidenceIDs, []string{expectedEvidenceID}) {
			return fmt.Errorf("task lens: anchor evidence identity does not match")
		}
		item, ok := evidence[expectedEvidenceID]
		expectedKind := EvidenceRepositoryFact
		if isDocumentPath(anchor.Path) {
			expectedKind = EvidenceDocumentClaim
		}
		if !ok || item.Kind != expectedKind || item.AnchorID != anchor.ID || item.Path != anchor.Path ||
			item.StartLine != anchor.StartLine || item.EndLine != anchor.EndLine ||
			item.Summary != anchorEvidenceSummary(anchor, expectedKind) {
			return fmt.Errorf("task lens: anchor evidence does not match its exact source")
		}
		for _, role := range anchor.RoleHints {
			if !validAnchorRole(role) {
				return fmt.Errorf("task lens: invalid anchor role hint")
			}
		}
		if !slices.Equal(
			anchor.RoleHints,
			deterministicRoleHints(anchor, bundle.KindHint, bundle.Task.Text),
		) {
			return fmt.Errorf("task lens: anchor role hints do not match deterministic evidence")
		}
		anchors[anchor.ID] = anchor
	}
	if !reflect.DeepEqual(bundle.Terms, GroundedTaskTerms(bundle.Task.Text, bundle.Anchors)) {
		return fmt.Errorf("task lens: task terms do not match deterministic grounding")
	}
	relations := make(map[string]Relation, len(bundle.Relations))
	for _, relation := range bundle.Relations {
		if !validOpaque(relation.ID) || relation.LeftID == relation.RightID ||
			!validRelationKind(RelationKind(relation.Kind)) || relation.SupportType != SupportLocallyObserved ||
			!validText(relation.Scope, 1024, true) || len(relation.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: invalid local relation")
		}
		left, ok := anchors[relation.LeftID]
		if !ok {
			return fmt.Errorf("task lens: relation has unknown left anchor")
		}
		right, ok := anchors[relation.RightID]
		if !ok {
			return fmt.Errorf("task lens: relation has unknown right anchor")
		}
		for _, id := range relation.EvidenceIDs {
			if _, ok := evidence[id]; !ok {
				return fmt.Errorf("task lens: relation references unknown evidence")
			}
		}
		expectedEvidence := uniqueSorted(append(
			append([]string(nil), left.EvidenceIDs...),
			right.EvidenceIDs...,
		))
		expectedKey := relation.LeftID + "\x00" + relation.RightID + "\x00" + relation.Kind
		if relation.ID != OpaqueID("relation", expectedKey) ||
			!slices.Equal(relation.EvidenceIDs, expectedEvidence) ||
			relation.Scope != relationScope(relation.Kind) ||
			!locallyValidRelation(relation, bundle.Anchors, bundle.Terms) {
			return fmt.Errorf("task lens: local relation does not match retained syntax")
		}
		if _, duplicate := relations[relation.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate relation id")
		}
		relations[relation.ID] = relation
	}
	expectedRelations := collectRelations(bundle.Anchors, bundle.Terms)
	if !reflect.DeepEqual(bundle.Relations, expectedRelations) {
		return fmt.Errorf("task lens: local relations are not the complete deterministic relation set")
	}
	if bundle.Locality != classifyLocality(
		bundle.Task.Text,
		bundle.Terms,
		bundle.Anchors,
		expectedRelations,
	) {
		return fmt.Errorf("task lens: locality does not match deterministic classification")
	}
	return validateBundleV01Contract(bundle)
}

func locallyValidRelation(relation Relation, anchors []Anchor, terms []Term) bool {
	for _, expected := range collectRelations(anchors, terms) {
		if reflect.DeepEqual(expected, relation) {
			return true
		}
	}
	return false
}

func (pack Pack) Validate() error {
	if pack.Version != PackVersion || !validOpaque(pack.ID) || !validSHA256(pack.BundleSHA256) ||
		!validLocality(pack.Locality) || len(pack.Anchors) > MaxVisibleAnchors ||
		len(pack.LikelyAreas) > 3 ||
		(len(pack.Anchors) > 0 && len(pack.LikelyAreas) == 0) ||
		(len(pack.Anchors) == 0 && len(pack.LikelyAreas) != 0) ||
		len(pack.EvidenceJoins) > MaxEvidenceJoins {
		return fmt.Errorf("task lens: invalid pack header")
	}
	if err := validateRepository(pack.Repository); err != nil {
		return err
	}
	if pack.Profile != pack.RoleContract.Profile || pack.Profile != pack.RoleCoverage.Profile ||
		!validTaskProfile(pack.Profile) {
		return fmt.Errorf("task lens: invalid pack task profile")
	}
	if err := pack.RoleContract.Validate(); err != nil {
		return err
	}
	if err := pack.RoleCoverage.ValidateAgainst(pack.RoleContract); err != nil {
		return err
	}
	if err := pack.VerificationFrontier.Validate(); err != nil {
		return err
	}
	if pack.DecisiveRelationID != "" && !validOpaque(pack.DecisiveRelationID) {
		return fmt.Errorf("task lens: invalid decisive relation id")
	}
	if len(pack.Warnings) > 1 ||
		(len(pack.Warnings) == 1 && pack.Warnings[0] != PackWarningLocalPartial &&
			pack.Warnings[0] != PackWarningModelPartial) {
		return fmt.Errorf("task lens: invalid pack warning")
	}
	if !validTaskKind(pack.Interpretation.Kind) ||
		!validText(pack.Interpretation.Restatement, 1024, true) ||
		!validText(pack.Interpretation.Observable, 1024, true) {
		return fmt.Errorf("task lens: invalid task interpretation")
	}
	anchors := make(map[string]struct{}, len(pack.Anchors))
	for _, anchor := range pack.Anchors {
		if !validOpaque(anchor.ID) || !validPath(anchor.Path) || !validText(anchor.Symbol, 256, true) ||
			!validAnchorRole(anchor.Role) || !validText(anchor.Why, 1024, true) ||
			len(anchor.Excerpt) == 0 || len(anchor.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: invalid investigation anchor")
		}
		if err := anchor.Scope.Validate(); err != nil ||
			anchor.Scope.ScopeStart != anchor.StartLine ||
			anchor.Scope.ScopeEnd != anchor.EndLine {
			return fmt.Errorf("task lens: invalid investigation anchor source scope")
		}
		if _, duplicate := anchors[anchor.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate investigation anchor")
		}
		anchors[anchor.ID] = struct{}{}
	}
	for _, area := range pack.LikelyAreas {
		if !validText(area.Label, 256, true) || !validText(area.Why, 1024, true) || len(area.TargetIDs) == 0 {
			return fmt.Errorf("task lens: invalid likely area")
		}
		for _, id := range area.TargetIDs {
			if _, ok := anchors[id]; !ok {
				return fmt.Errorf("task lens: likely area has unknown target")
			}
		}
	}
	for _, join := range pack.EvidenceJoins {
		if !validOpaque(join.ID) || join.LeftID == join.RightID || !validSupportType(join.SupportType) ||
			!validText(join.Kind, 128, true) || !validText(join.Explanation, 1536, true) ||
			!validText(join.Scope, 1024, true) {
			return fmt.Errorf("task lens: invalid evidence join")
		}
		if _, ok := anchors[join.LeftID]; !ok {
			return fmt.Errorf("task lens: evidence join has unknown left anchor")
		}
		if _, ok := anchors[join.RightID]; !ok {
			return fmt.Errorf("task lens: evidence join has unknown right anchor")
		}
	}
	if len(pack.WorkingHypothesis) == 0 || len(pack.WorkingHypothesis) > MaxHypothesisClauses {
		return fmt.Errorf("task lens: invalid working hypothesis count")
	}
	for _, clause := range pack.WorkingHypothesis {
		if !validHypothesisStatus(clause.Status) || !validText(clause.Text, 1536, true) {
			return fmt.Errorf("task lens: invalid hypothesis clause")
		}
	}
	if len(pack.ReproduceOrObserve) == 0 || len(pack.ReproduceOrObserve) > MaxGuidanceSteps ||
		len(pack.Verify.Steps) == 0 || len(pack.Verify.Steps) > MaxGuidanceSteps ||
		!validText(pack.Verify.Effect, 1024, true) || len(pack.NextProbes) == 0 || len(pack.NextProbes) > MaxNextProbes {
		return fmt.Errorf("task lens: invalid guidance")
	}
	for _, guidance := range append(append([]Guidance(nil), pack.ReproduceOrObserve...), pack.Verify.Steps...) {
		if !validGuidanceAuthority(guidance.Authority) || !validText(guidance.Text, 1536, true) {
			return fmt.Errorf("task lens: invalid guidance step")
		}
	}
	for _, probe := range pack.NextProbes {
		if !validProbeAction(probe.Action) || !validText(probe.Text, 1024, true) ||
			len(probe.AnchorIDs) > 2 ||
			(probe.Action != ProbeSearchTaskTerms && len(probe.AnchorIDs) == 0) ||
			(probe.Action == ProbeSearchTaskTerms && len(probe.AnchorIDs) != 0) {
			return fmt.Errorf("task lens: invalid next probe")
		}
		for _, id := range probe.AnchorIDs {
			if _, ok := anchors[id]; !ok {
				return fmt.Errorf("task lens: next probe has unknown anchor")
			}
		}
	}
	return validateBudget(pack.Budgets)
}

type bundleIndex struct {
	anchors   map[string]Anchor
	evidence  map[string]Evidence
	relations map[string]Relation
	paths     map[string]struct{}
	taskText  string
}

func newBundleIndex(bundle Bundle) bundleIndex {
	index := bundleIndex{
		anchors:   make(map[string]Anchor, len(bundle.Anchors)),
		evidence:  make(map[string]Evidence, len(bundle.Evidence)),
		relations: make(map[string]Relation, len(bundle.Relations)),
		paths:     make(map[string]struct{}, len(bundle.AllowedPaths)),
		taskText:  bundle.Task.Text,
	}
	for _, anchor := range bundle.Anchors {
		index.anchors[anchor.ID] = anchor
	}
	for _, item := range bundle.Evidence {
		index.evidence[item.ID] = item
	}
	for _, relation := range bundle.Relations {
		index.relations[relation.ID] = relation
	}
	for _, filePath := range bundle.AllowedPaths {
		index.paths[filePath] = struct{}{}
		for directory := path.Dir(filePath); directory != "."; directory = path.Dir(directory) {
			index.paths[directory] = struct{}{}
		}
	}
	return index
}

func validateProposalHeader(proposal Proposal, index bundleIndex) error {
	if !validTaskKind(proposal.Interpretation.Kind) ||
		!validText(proposal.Interpretation.Restatement, 1024, true) ||
		!validText(proposal.Interpretation.Observable, 1024, true) ||
		!pathReferencesGrounded(proposal.Interpretation.Restatement, index.paths, index.taskText) ||
		!pathReferencesGrounded(proposal.Interpretation.Observable, index.paths, index.taskText) {
		return fmt.Errorf("task lens: invalid proposed interpretation")
	}
	return nil
}

func buildJoin(proposed ProposedJoin, selected map[string]struct{}, index bundleIndex) (EvidenceJoin, error) {
	if proposed.LeftID == proposed.RightID || !validText(proposed.Kind, 128, true) ||
		!validSupportType(proposed.SupportType) || !validText(proposed.Explanation, 1536, true) ||
		!validText(proposed.Scope, 1024, true) {
		return EvidenceJoin{}, fmt.Errorf("invalid content")
	}
	if _, ok := selected[proposed.LeftID]; !ok {
		return EvidenceJoin{}, fmt.Errorf("left anchor is not visible")
	}
	if _, ok := selected[proposed.RightID]; !ok {
		return EvidenceJoin{}, fmt.Errorf("right anchor is not visible")
	}
	if containsAbsenceClaim(proposed.Explanation+"\n"+proposed.Scope) &&
		(!sourceScopeAllowsUnstructuredAbsence(index.anchors[proposed.LeftID].Scope) ||
			!sourceScopeAllowsUnstructuredAbsence(index.anchors[proposed.RightID].Scope)) {
		return EvidenceJoin{}, fmt.Errorf("absence claim exceeds retained source scope")
	}
	for _, id := range proposed.SupportIDs {
		item, ok := index.evidence[id]
		if !ok {
			return EvidenceJoin{}, fmt.Errorf("unknown support id")
		}
		if item.AnchorID != "" {
			if _, visible := selected[item.AnchorID]; !visible {
				return EvidenceJoin{}, fmt.Errorf("support evidence is outside visible anchors")
			}
		}
	}
	supportIDs := uniqueSorted(proposed.SupportIDs)
	explanation := proposed.Explanation
	scope := proposed.Scope
	kind := proposed.Kind
	if proposed.SupportType == SupportLocallyObserved {
		relation, ok := index.relations[proposed.RelationID]
		if !ok || relation.SupportType != SupportLocallyObserved ||
			!sameEndpoints(relation.LeftID, relation.RightID, proposed.LeftID, proposed.RightID) {
			return EvidenceJoin{}, fmt.Errorf("locally observed support lacks a matching local relation")
		}
		if proposed.Kind != relation.Kind {
			return EvidenceJoin{}, fmt.Errorf("locally observed support changes the local relation kind")
		}
		if !subset(proposed.SupportIDs, relation.EvidenceIDs) || len(proposed.SupportIDs) == 0 {
			return EvidenceJoin{}, fmt.Errorf("locally observed support is outside relation evidence")
		}
		// A locally observed join is a presentation of deterministic evidence,
		// not a model-authored claim. Preserve the exact complete evidence set
		// and non-guarantee recorded by the collector, and synthesize the
		// explanation locally so semantic prose cannot overstate syntax.
		supportIDs = append([]string(nil), relation.EvidenceIDs...)
		explanation = localRelationExplanation(relation.Kind)
		scope = relation.Scope
	} else if proposed.RelationID != "" {
		return EvidenceJoin{}, fmt.Errorf("non-local support cannot cite a local relation id")
	}
	if proposed.SupportType == SupportDocument {
		if !allEvidenceKind(proposed.SupportIDs, index.evidence, EvidenceDocumentClaim) {
			return EvidenceJoin{}, fmt.Errorf("document support lacks document evidence")
		}
		if !evidenceMentionsEndpoint(proposed.SupportIDs, proposed.LeftID, index) ||
			!evidenceMentionsEndpoint(proposed.SupportIDs, proposed.RightID, index) {
			return EvidenceJoin{}, fmt.Errorf("document support is not grounded to both endpoints")
		}
		explanation = "The cited repository document evidence names both retained endpoints."
		scope = "Repository documentation is retained as claim evidence; it does not independently prove runtime behavior or exact implementation flow."
		kind = "document_names_endpoints"
	}
	if proposed.SupportType == SupportModelHypothesis {
		if !hasEndpointEvidence(proposed.SupportIDs, proposed.LeftID, index) ||
			!hasEndpointEvidence(proposed.SupportIDs, proposed.RightID, index) {
			return EvidenceJoin{}, fmt.Errorf("model hypothesis lacks exact evidence for both endpoints")
		}
		supportIDs = endpointEvidenceIDs(proposed.LeftID, proposed.RightID, index)
		if !modelTextGrounded(proposed.Explanation, supportIDs, index) ||
			!hypothesisTextNamesAnchor(proposed.Explanation, index.anchors[proposed.LeftID]) ||
			!hypothesisTextNamesAnchor(proposed.Explanation, index.anchors[proposed.RightID]) {
			return EvidenceJoin{}, fmt.Errorf("model hypothesis explanation is not grounded to both endpoints")
		}
		explanation = proposed.Explanation
		scope = "Runtime reachability, order, and causality are not locally proven."
		kind = "model_hypothesis"
	}
	if proposed.SupportType == SupportUnresolved {
		supportIDs = []string{}
		explanation = "The selected anchors are retained together for an unresolved bounded probe."
		scope = "The relation remains unresolved; no runtime reachability, order, or causality is established."
		kind = "unresolved_relation"
	}
	return EvidenceJoin{
		ID:     OpaqueID("join", proposed.LeftID, proposed.RightID, kind, string(proposed.SupportType)),
		LeftID: proposed.LeftID, RightID: proposed.RightID, RelationID: proposed.RelationID,
		Kind: kind, SupportType: proposed.SupportType,
		SupportIDs: supportIDs, Explanation: explanation, Scope: scope,
	}, nil
}

func endpointEvidenceIDs(leftID, rightID string, index bundleIndex) []string {
	result := append([]string(nil), index.anchors[leftID].EvidenceIDs...)
	result = append(result, index.anchors[rightID].EvidenceIDs...)
	return uniqueSorted(result)
}

func hasEndpointEvidence(ids []string, anchorID string, index bundleIndex) bool {
	for _, id := range ids {
		item := index.evidence[id]
		if item.AnchorID == anchorID && item.Kind != EvidenceTaskProvided {
			return true
		}
	}
	return false
}

func evidenceMentionsEndpoint(ids []string, anchorID string, index bundleIndex) bool {
	if hasEndpointEvidence(ids, anchorID, index) {
		return true
	}
	anchor, ok := index.anchors[anchorID]
	if !ok {
		return false
	}
	corpus := guidanceEvidenceCorpus(ids, index)
	if anchor.Path != "" && containsExactPathReference(corpus, anchor.Path) {
		return true
	}
	symbol := basePackSymbol(anchor.Symbol)
	return len(symbol) >= 3 && containsIdentifier(strings.ToLower(corpus), strings.ToLower(symbol))
}

func containsExactPathReference(text, wanted string) bool {
	for _, pattern := range []*regexp.Regexp{pathTokenPattern, slashPathPattern, specialPathPattern} {
		for _, token := range pattern.FindAllString(text, -1) {
			if token == wanted {
				return true
			}
		}
	}
	return false
}

func localRelationExplanation(kind string) string {
	switch kind {
	case string(RelationDirectCall):
		return "The retained left-anchor excerpt contains an unqualified call expression named by the retained right anchor."
	case string(RelationTestExercises):
		return "The retained test excerpt contains the exact identifier recorded for the retained implementation anchor."
	case string(RelationConfigApplied):
		return "The retained configuration copy or apply site references the retained configuration source or destination."
	case string(RelationFieldCopy):
		return "The retained assignment copies the exact named field between the retained source contexts."
	case string(RelationFieldRead):
		return "The retained source reads the exact field declared or written by the other retained anchor."
	case string(RelationFieldWrite):
		return "The retained source writes the exact field declared or read by the other retained anchor."
	case string(RelationErrorCreated):
		return "The retained source constructs or returns the exact error form consumed by the other retained anchor."
	case string(RelationErrorMapped):
		return "The retained error mapper names the exact retained error type or status path."
	case string(RelationErrorExposed):
		return "The retained public serializer names the exact retained error or normalization path."
	case string(RelationValueTransformed):
		return "The retained transformation path names the exact retained source or sink symbol."
	case string(RelationScriptInvokes):
		return "The complete retained operational source names the exact retained script or target."
	case string(RelationFixtureRecords):
		return "The retained generated fixture records an exact source symbol or task value from the retained production evidence."
	case string(RelationDocumentedUses):
		return "The retained repository document names the exact retained source, target, or command."
	case string(RelationSharedStateAlias):
		return "The retained excerpts share the exact task term; ownership, aliasing, and runtime flow remain unproven."
	case string(RelationScopeUnknown):
		return "The retained excerpt names the exact other anchor, while runtime direction and causality remain unresolved."
	default:
		return "The retained excerpts contain the exact syntactic relation recorded by the local collector."
	}
}

func buildClause(proposed ProposedClause, selected map[string]struct{}, index bundleIndex) (HypothesisClause, error) {
	if !validHypothesisStatus(proposed.Status) || !validText(proposed.Text, 1536, true) ||
		unknownPathInText(proposed.Text, index.paths) {
		return HypothesisClause{}, fmt.Errorf("invalid content")
	}
	for _, id := range proposed.SupportIDs {
		item, ok := index.evidence[id]
		if !ok {
			return HypothesisClause{}, fmt.Errorf("unknown support id")
		}
		if item.AnchorID != "" {
			if _, visible := selected[item.AnchorID]; !visible {
				return HypothesisClause{}, fmt.Errorf("support evidence is outside visible anchors")
			}
		}
	}
	if containsAbsenceClaim(proposed.Text) {
		hasRepositoryScope := false
		for _, id := range proposed.SupportIDs {
			item := index.evidence[id]
			if item.AnchorID == "" {
				continue
			}
			hasRepositoryScope = true
			if !sourceScopeAllowsUnstructuredAbsence(index.anchors[item.AnchorID].Scope) {
				return HypothesisClause{}, fmt.Errorf("absence claim exceeds retained source scope")
			}
		}
		if !hasRepositoryScope {
			return HypothesisClause{}, fmt.Errorf("absence claim exceeds retained source scope")
		}
	}
	for _, id := range proposed.RelationIDs {
		relation, ok := index.relations[id]
		if !ok {
			return HypothesisClause{}, fmt.Errorf("unknown relation id")
		}
		if _, visible := selected[relation.LeftID]; !visible {
			return HypothesisClause{}, fmt.Errorf("relation is outside visible anchors")
		}
		if _, visible := selected[relation.RightID]; !visible {
			return HypothesisClause{}, fmt.Errorf("relation is outside visible anchors")
		}
	}
	supportIDs := uniqueSorted(proposed.SupportIDs)
	relationIDs := uniqueSorted(proposed.RelationIDs)
	for _, relationID := range relationIDs {
		if !subset(index.relations[relationID].EvidenceIDs, supportIDs) {
			return HypothesisClause{}, fmt.Errorf("relation evidence is not included in clause support")
		}
	}
	text := proposed.Text
	switch proposed.Status {
	case HypothesisSupported:
		if len(proposed.SupportIDs) == 0 ||
			!hasRepositoryEvidence(proposed.SupportIDs, index.evidence) {
			return HypothesisClause{}, fmt.Errorf("supported clause exceeds local evidence")
		}
		text = supportedClauseText(supportIDs, relationIDs, index)
	case HypothesisPlausible:
		if len(proposed.SupportIDs) == 0 ||
			!hasRepositoryEvidence(proposed.SupportIDs, index.evidence) {
			return HypothesisClause{}, fmt.Errorf("plausible clause lacks visible repository evidence")
		}
		if !plausibleClausePreservesModelText(proposed, supportIDs, index) {
			text = plausibleClauseText(supportIDs, index)
		}
	case HypothesisUnresolved:
		text = "The relationship beyond the selected retained anchors remains unresolved; no runtime sequence or causality is asserted."
	}
	return HypothesisClause{
		Status: proposed.Status, Text: text,
		SupportIDs: supportIDs, RelationIDs: relationIDs,
	}, nil
}

var epistemicAbsencePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bmissing\s+evidence\b`),
	regexp.MustCompile(`(?i)\bevidence\s+(?:is|remains|was)\s+missing\b`),
	regexp.MustCompile(`(?i)\b(?:do(?:es)?\s+not|doesn't|cannot|can't)\s+(?:independently\s+)?(?:prove|establish|demonstrate|show|confirm|guarantee|assert)\b`),
	regexp.MustCompile(`(?i)\b(?:cannot|can't)\s+be\s+(?:determined|inferred|established|proven)\b`),
	regexp.MustCompile(`(?i)\b(?:is|are|was|were|remain|remains)\s+not\s+(?:locally\s+)?(?:proven|established|demonstrated|shown|confirmed|guaranteed|asserted)\b`),
	regexp.MustCompile(`(?i)\b(?:unproven|unresolved)\b`),
}

var absenceClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:absent|missing|lacks?|omits?|omitted|never|nowhere)\b`),
	regexp.MustCompile(`(?i)\b(?:no|without)\s+(?:test|guard|check|copy|assignment|reference|handler|mapping|command|field|implementation|definition|validation|branch|case)\b`),
	regexp.MustCompile(`(?i)\b(?:do(?:es)?\s+not|doesn't|is\s+not|isn't|fails?\s+to|cannot|can't)\s+(?:copy|read|write|check|handle|map|reference|call|contain|include|set|define|implement)`),
	regexp.MustCompile(`(?i)\bnot\s+(?:present|found|defined|implemented|set|handled|mapped|called|referenced)\b`),
}

func containsAbsenceClaim(text string) bool {
	for _, pattern := range epistemicAbsencePatterns {
		text = pattern.ReplaceAllString(text, " ")
	}
	for _, pattern := range absenceClaimPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func sourceScopeAllowsUnstructuredAbsence(scope SourceScope) bool {
	if !scope.NegativeClaimsAllowed {
		return false
	}
	if scope.ScopeKind == SourceScopeCompleteFile {
		return true
	}
	switch scope.NegativeEvidenceBasis {
	case NegativeEvidenceExhaustiveExactSearch, NegativeEvidenceDeterministicManifest:
		return true
	default:
		return false
	}
}

func supportedClauseText(supportIDs, relationIDs []string, index bundleIndex) string {
	for _, relationID := range relationIDs {
		relation := index.relations[relationID]
		if subset(relation.EvidenceIDs, supportIDs) {
			return localRelationExplanation(relation.Kind)
		}
	}
	names := supportedAnchorNames(supportIDs, index)
	if len(names) == 0 {
		return "The retained repository evidence contains the cited exact excerpt."
	}
	if len(names) > 3 {
		names = names[:3]
	}
	return fmt.Sprintf("The retained repository evidence contains exact excerpts for %s.", strings.Join(names, ", "))
}

func plausibleClauseText(supportIDs []string, index bundleIndex) string {
	names := supportedAnchorNames(supportIDs, index)
	if len(names) > 3 {
		names = names[:3]
	}
	if len(names) == 0 {
		return "Missing evidence: an exact repository anchor and a typed local relation needed to evaluate the task; no repository mechanism is asserted."
	}
	return fmt.Sprintf(
		"Missing evidence: a typed local relation with exact source support connecting %s; the retained excerpts alone do not establish runtime sequence or causality.",
		strings.Join(names, ", "),
	)
}

var concreteMissingEvidencePattern = regexp.MustCompile(`(?i)\bmissing\s+evidence\s*:\s*`)

func namesConcreteMissingEvidence(text string) bool {
	location := concreteMissingEvidencePattern.FindStringIndex(text)
	if location == nil {
		return false
	}
	detail := strings.Trim(strings.TrimSpace(text[location[1]:]), " .,:;-\t\r\n")
	return len(strings.Fields(detail)) >= 4
}

// plausibleClausePreservesModelText permits useful semantic synthesis only in
// the explicitly tentative status lane. Every cited repository excerpt must be
// named by the text, and every path, symbol, inline-code reference, and local
// relation must resolve back to that exact cited evidence. Supported clauses
// and locally observed joins remain deterministic local projections.
func plausibleClausePreservesModelText(
	proposed ProposedClause,
	supportIDs []string,
	index bundleIndex,
) bool {
	if !namesConcreteMissingEvidence(proposed.Text) ||
		commandPattern.MatchString(proposed.Text) ||
		!pathReferencesGrounded(proposed.Text, nil, guidanceEvidenceText(supportIDs, index)) ||
		!supportedCodeReferencesGrounded(proposed.Text, supportIDs, index) ||
		!mentionedSymbolsAreSupported(proposed.Text, supportIDs, index) {
		return false
	}

	repositoryAnchors := 0
	seenAnchors := make(map[string]struct{}, len(supportIDs))
	for _, id := range supportIDs {
		item := index.evidence[id]
		if item.Kind == EvidenceTaskProvided || item.AnchorID == "" {
			continue
		}
		if _, seen := seenAnchors[item.AnchorID]; seen {
			continue
		}
		anchor, ok := index.anchors[item.AnchorID]
		if !ok || !hypothesisTextNamesAnchor(proposed.Text, anchor) {
			return false
		}
		seenAnchors[item.AnchorID] = struct{}{}
		repositoryAnchors++
	}
	if repositoryAnchors == 0 {
		return false
	}
	for _, relationID := range proposed.RelationIDs {
		relation := index.relations[relationID]
		if !subset(relation.EvidenceIDs, supportIDs) {
			return false
		}
	}
	return true
}

func modelTextGrounded(text string, supportIDs []string, index bundleIndex) bool {
	paths := make(map[string]struct{}, len(supportIDs))
	for _, id := range supportIDs {
		if item := index.evidence[id]; item.Path != "" {
			paths[item.Path] = struct{}{}
		}
	}
	return !commandPattern.MatchString(text) &&
		pathReferencesGrounded(text, paths, "") &&
		supportedCodeReferencesGrounded(text, supportIDs, index) &&
		mentionedSymbolsAreSupported(text, supportIDs, index)
}

func hypothesisTextNamesAnchor(text string, anchor Anchor) bool {
	if containsExactPathReference(text, anchor.Path) {
		return true
	}
	symbol := basePackSymbol(anchor.Symbol)
	if len(symbol) >= 3 && containsIdentifier(text, symbol) {
		return true
	}
	section := normalizedGroundingText(anchor.Section)
	return len(section) >= 4 && strings.Contains(normalizedGroundingText(text), section)
}

func supportedAnchorNames(supportIDs []string, index bundleIndex) []string {
	var names []string
	for _, id := range supportIDs {
		item := index.evidence[id]
		anchor, ok := index.anchors[item.AnchorID]
		if !ok {
			continue
		}
		name := anchor.Symbol
		if anchor.Section != "" {
			name = anchor.Section
		}
		// Non-Go files use a path-like fallback symbol such as
		// "openapi.golden.json:1156". For a nested file that basename is not
		// itself an allowed repository path, so emitting it would make the
		// canonical clause fail its own saved-pack replay. Keep the useful line
		// identity, but bind it to the exact allowed path.
		if unknownPathInText(name, index.paths) {
			prefix := path.Base(anchor.Path) + ":"
			if strings.HasPrefix(name, prefix) {
				name = anchor.Path + strings.TrimPrefix(name, path.Base(anchor.Path))
			} else {
				name = fmt.Sprintf("%s:%d", anchor.Path, anchor.StartLine)
			}
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return uniqueSorted(names)
}

func supportedCodeReferencesGrounded(text string, supportIDs []string, index bundleIndex) bool {
	corpus := guidanceEvidenceCorpus(supportIDs, index)
	for _, token := range qualifiedIdentifierPattern.FindAllString(text, -1) {
		if !containsIdentifierTerm(corpus, token) {
			return false
		}
	}
	for _, location := range identifierPattern.FindAllStringIndex(text, -1) {
		token := text[location[0]:location[1]]
		if identifierLooksCodeLikeInText(token) && !containsIdentifierTerm(corpus, token) {
			return false
		}
	}
	for _, match := range inlineCodePattern.FindAllStringSubmatch(text, -1) {
		if !containsIdentifierTerm(corpus, normalizedGroundingText(match[1])) {
			return false
		}
	}
	return true
}

func identifierLooksCodeLikeInText(value string) bool {
	if identifierLooksCodeLike(value) {
		return true
	}
	runes := []rune(value)
	if len(runes) < 2 || !unicode.IsUpper(runes[0]) {
		return false
	}
	switch strings.ToLower(value) {
	case "a", "an", "the", "this", "these", "it", "plausible", "possible", "likely",
		"hypothesis", "runtime", "repository", "exact", "retained", "model", "missing", "evidence":
		return false
	default:
		return true
	}
}

func identifierLooksCodeLike(value string) bool {
	if strings.Contains(value, "_") {
		return true
	}
	uppercase := 0
	lowerSeen := false
	internalUpper := false
	for index, current := range value {
		switch {
		case unicode.IsLower(current):
			lowerSeen = true
		case unicode.IsUpper(current):
			uppercase++
			if index > 0 && lowerSeen {
				internalUpper = true
			}
		}
	}
	return internalUpper || uppercase >= 2
}

func mentionedSymbolsAreSupported(text string, supportIDs []string, index bundleIndex) bool {
	support := make(map[string]struct{}, len(supportIDs))
	for _, id := range supportIDs {
		support[id] = struct{}{}
	}
	for _, anchor := range index.anchors {
		symbol := anchor.Symbol
		if dot := strings.LastIndex(symbol, "."); dot >= 0 {
			symbol = symbol[dot+1:]
		}
		if len(symbol) < 4 || genericGrepTerm(strings.ToLower(symbol)) || !containsIdentifier(text, symbol) {
			continue
		}
		grounded := false
		for _, evidenceID := range anchor.EvidenceIDs {
			if _, ok := support[evidenceID]; ok {
				grounded = true
				break
			}
		}
		if !grounded {
			return false
		}
	}
	return true
}

func containsIdentifier(text, identifier string) bool {
	return containsIdentifierTerm(text, identifier)
}

func containsIdentifierTerm(text, term string) bool {
	textRunes := []rune(strings.ToLower(text))
	termRunes := []rune(strings.ToLower(term))
	if len(termRunes) == 0 || len(termRunes) > len(textRunes) {
		return false
	}
	firstIdentifier := isIdentifierRune(termRunes[0])
	lastIdentifier := isIdentifierRune(termRunes[len(termRunes)-1])
	for start := 0; start+len(termRunes) <= len(textRunes); start++ {
		if !slices.Equal(textRunes[start:start+len(termRunes)], termRunes) {
			continue
		}
		beforeOK := !firstIdentifier || start == 0 || !isIdentifierRune(textRunes[start-1])
		after := start + len(termRunes)
		afterOK := !lastIdentifier || after == len(textRunes) || !isIdentifierRune(textRunes[after])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func isIdentifierRune(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func buildGuidance(
	proposed ProposedGuidance,
	selected map[string]struct{},
	index bundleIndex,
) (Guidance, error) {
	if !validGuidanceAuthority(proposed.Authority) || !validText(proposed.Text, 1536, true) {
		return Guidance{}, fmt.Errorf("invalid content")
	}
	for _, id := range proposed.EvidenceIDs {
		item, ok := index.evidence[id]
		if !ok {
			return Guidance{}, fmt.Errorf("unknown evidence id")
		}
		if item.AnchorID != "" {
			if _, visible := selected[item.AnchorID]; !visible {
				return Guidance{}, fmt.Errorf("evidence is outside visible anchors")
			}
		}
	}
	switch proposed.Authority {
	case AuthorityTaskProvided:
		if !TaskProvidesConcreteReproductionOrObservation(index.taskText) ||
			!allEvidenceKind(proposed.EvidenceIDs, index.evidence, EvidenceTaskProvided) {
			return Guidance{}, fmt.Errorf("task-provided guidance lacks task evidence")
		}
	case AuthorityRepositoryDocument:
		if !allEvidenceKind(proposed.EvidenceIDs, index.evidence, EvidenceDocumentClaim) {
			return Guidance{}, fmt.Errorf("document guidance lacks document evidence")
		}
	case AuthorityRepositoryTest:
		if !allTestOrExampleEvidence(proposed.EvidenceIDs, index) {
			return Guidance{}, fmt.Errorf("test guidance lacks test or example evidence")
		}
	case AuthorityRepositoryObservation:
		if !allEvidenceKind(proposed.EvidenceIDs, index.evidence, EvidenceRepositoryFact) {
			return Guidance{}, fmt.Errorf("repository observation lacks source or configuration evidence")
		}
	case AuthorityMissing:
		if len(proposed.EvidenceIDs) != 0 {
			return Guidance{}, fmt.Errorf("missing-evidence guidance cites evidence")
		}
	}
	return Guidance{
		Text: localGuidanceText(proposed, index), Authority: proposed.Authority,
		EvidenceIDs: uniqueSorted(proposed.EvidenceIDs),
	}, nil
}

func localGuidanceText(proposed ProposedGuidance, index bundleIndex) string {
	if proposed.Authority == AuthorityMissing {
		return "No exact repository-owned reproduction, command, endpoint, or test step was retained; obtain the missing evidence before choosing an action."
	}
	switch proposed.Authority {
	case AuthorityTaskProvided:
		return "Task-provided reproduction or observation: " + taskGuidanceExcerpt(index.taskText)
	case AuthorityRepositoryDocument:
		return localRepositoryGuidanceText(
			"Exact retained repository document anchor", proposed.EvidenceIDs, index,
		)
	case AuthorityRepositoryTest:
		return localRepositoryGuidanceText(
			"Exact retained repository test or example anchor", proposed.EvidenceIDs, index,
		)
	case AuthorityRepositoryObservation:
		return localRepositoryObservationText(proposed.EvidenceIDs, index)
	default:
		return "Obtain exact evidence before choosing an observation or verification action."
	}
}

func taskGuidanceExcerpt(task string) string {
	type marker struct {
		text  string
		score int
	}
	markers := []marker{
		{text: "minimal reproduction:", score: 100},
		{text: "minimal reproduction uses", score: 100},
		{text: "minimal reproduction constructs", score: 100},
		{text: "minimal reproduction calls", score: 100},
		{text: "minimal reproduction sends", score: 100},
		{text: "minimal reproduction sets", score: 100},
		{text: "minimal reproduction passes", score: 100},
		{text: "minimal reproduction creates", score: 100},
		{text: "steps to reproduce:", score: 95},
		{text: "reproduction:", score: 90},
		{text: "reproduce by", score: 85},
		{text: "triggered by", score: 75},
		{text: "actual behavior", score: 70},
		{text: "observed behavior", score: 65},
		{text: "fails when", score: 60},
		{text: "panics when", score: 55},
		{text: "panics after", score: 55},
		{text: "does not", score: 50},
		{text: "is ignored", score: 45},
	}

	best := ""
	bestScore := 0
	for _, rawParagraph := range strings.Split(strings.ReplaceAll(task, "\r\n", "\n"), "\n\n") {
		paragraph := strings.Join(strings.Fields(rawParagraph), " ")
		if paragraph == "" {
			continue
		}
		lower := strings.ToLower(paragraph)
		score := 0
		for _, candidate := range markers {
			if strings.Contains(lower, candidate.text) && candidate.score > score {
				score = candidate.score
			}
		}
		if score > bestScore {
			best = paragraph
			bestScore = score
		}
	}
	if best == "" {
		return taskObservable(task)
	}
	return truncateUTF8(best, 1200)
}

func localRepositoryGuidanceText(prefix string, evidenceIDs []string, index bundleIndex) string {
	const maxGuidanceTextBytes = 1536

	seenAnchors := make(map[string]struct{}, len(evidenceIDs))
	locations := make([]string, 0, len(evidenceIDs))
	for _, id := range uniqueSorted(evidenceIDs) {
		item := index.evidence[id]
		anchor, ok := index.anchors[item.AnchorID]
		if !ok {
			continue
		}
		if _, seen := seenAnchors[anchor.ID]; seen {
			continue
		}
		seenAnchors[anchor.ID] = struct{}{}
		name := anchor.Symbol
		if anchor.Section != "" {
			name = anchor.Section
		}
		locations = append(locations, fmt.Sprintf(
			"%s at %s:%d-%d", name, anchor.Path, anchor.StartLine, anchor.EndLine,
		))
	}

	result := prefix + ": "
	retained := 0
	for _, location := range locations {
		separator := ""
		if retained > 0 {
			separator = "; "
		}
		if len(result)+len(separator)+len(location)+1 > maxGuidanceTextBytes {
			break
		}
		result += separator + location
		retained++
	}
	if omitted := len(locations) - retained; omitted > 0 {
		suffix := fmt.Sprintf("; plus %d additional cited anchor(s)", omitted)
		if len(result)+len(suffix)+1 <= maxGuidanceTextBytes {
			result += suffix
		}
	}
	return result + "."
}

func localRepositoryObservationText(evidenceIDs []string, index bundleIndex) string {
	prefix := ""
	if !TaskProvidesConcreteReproductionOrObservation(index.taskText) {
		prefix = "No concrete task-provided reproduction was retained. "
	}
	for _, id := range uniqueSorted(evidenceIDs) {
		item := index.evidence[id]
		anchor, ok := index.anchors[item.AnchorID]
		if !ok {
			continue
		}
		for _, line := range anchor.Excerpt {
			lineText := strings.Join(strings.Fields(line.Text), " ")
			if lineText == "" {
				continue
			}
			return truncateUTF8(fmt.Sprintf(
				"%sExact retained repository observation (not executed): %s at %s:%d.",
				prefix, lineText, anchor.Path, line.Line,
			), 1536)
		}
	}
	return localRepositoryGuidanceText(
		"Exact retained repository source or configuration anchor", evidenceIDs, index,
	)
}

func missingGuidanceIsGrounded(text string, index bundleIndex) bool {
	if commandPattern.MatchString(text) {
		return false
	}
	corpus := normalizedGroundingText(index.taskText)
	for _, match := range inlineCodePattern.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(corpus, normalizedGroundingText(match[1])) {
			return false
		}
	}
	for _, match := range qualifiedIdentifierPattern.FindAllString(text, -1) {
		if !strings.Contains(corpus, normalizedGroundingText(match)) {
			return false
		}
	}
	for _, token := range identifierPattern.FindAllString(text, -1) {
		if identifierLooksCodeLike(token) && !strings.Contains(corpus, strings.ToLower(token)) {
			return false
		}
	}
	return true
}

var (
	inlineCodePattern          = regexp.MustCompile("`([^`\\r\\n]{1,240})`")
	identifierPattern          = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)
	commandPattern             = regexp.MustCompile(`(?i)(?:\b(?:go\s+(?:test|run|build|vet|generate|install)(?:\s+(?:--?[A-Za-z0-9_.=-]+|\./[A-Za-z0-9_./*-]+|[A-Za-z0-9_.-]+/[A-Za-z0-9_./*-]+))*|make\s+[A-Za-z0-9_.-]+|(?:npm|pnpm|yarn)\s+(?:run\s+[A-Za-z0-9_.:-]+|test|install)|cargo\s+(?:test|run|build)(?:\s+--?[A-Za-z0-9_.=-]+)*|curl\s+\S+|(?:docker|kubectl)\s+[A-Za-z0-9_.:-]+|(?:pytest|mvn|gradle|bash|zsh|python3?|node|deno|bun|dotnet|mix|rebar3|cmake|ninja|bazel|just|tox|uv)(?:\s+\S+)?)|\./[A-Za-z0-9_./*-]+|\b[A-Za-z0-9_.-]+\s+--?[A-Za-z0-9][^\s,;]*)`)
	qualifiedIdentifierPattern = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*\b`)
)

func guidanceIsGrounded(proposed ProposedGuidance, index bundleIndex) bool {
	corpus := guidanceEvidenceCorpus(proposed.EvidenceIDs, index)
	if corpus == "" || !pathReferencesGrounded(proposed.Text, nil, corpus) {
		return false
	}
	for _, match := range commandPattern.FindAllString(proposed.Text, -1) {
		if !strings.Contains(corpus, normalizedGroundingText(match)) {
			return false
		}
	}
	for _, match := range inlineCodePattern.FindAllStringSubmatch(proposed.Text, -1) {
		if !strings.Contains(corpus, normalizedGroundingText(match[1])) {
			return false
		}
	}
	for _, match := range qualifiedIdentifierPattern.FindAllString(proposed.Text, -1) {
		if !strings.Contains(corpus, normalizedGroundingText(match)) {
			return false
		}
	}
	support := make(map[string]struct{}, len(proposed.EvidenceIDs))
	for _, id := range proposed.EvidenceIDs {
		support[id] = struct{}{}
	}
	_, taskSupported := support[taskEvidenceID(index.evidence)]
	for _, anchor := range index.anchors {
		symbol := basePackSymbol(anchor.Symbol)
		if len(symbol) < 4 || genericGrepTerm(strings.ToLower(symbol)) ||
			!containsIdentifier(proposed.Text, symbol) {
			continue
		}
		grounded := taskSupported && containsIdentifier(index.taskText, symbol)
		for _, evidenceID := range anchor.EvidenceIDs {
			if _, ok := support[evidenceID]; ok {
				grounded = true
				break
			}
		}
		if !grounded {
			return false
		}
	}
	return true
}

func guidanceEvidenceCorpus(ids []string, index bundleIndex) string {
	return normalizedGroundingText(guidanceEvidenceText(ids, index))
}

func guidanceEvidenceText(ids []string, index bundleIndex) string {
	var values []string
	for _, id := range ids {
		item := index.evidence[id]
		if item.Kind == EvidenceTaskProvided {
			values = append(values, index.taskText)
			continue
		}
		anchor, ok := index.anchors[item.AnchorID]
		if !ok {
			continue
		}
		values = append(values, anchor.Path, anchor.Symbol, anchor.Section)
		for _, line := range anchor.Excerpt {
			values = append(values, line.Text)
		}
	}
	return strings.Join(values, "\n")
}

func normalizedGroundingText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func normalizedPathGroundingText(value string) string {
	value = strings.ReplaceAll(normalizedGroundingText(value), "`", "")
	value = strings.ReplaceAll(value, " /", "/")
	return strings.ReplaceAll(value, "/ ", "/")
}

func taskEvidenceID(evidence map[string]Evidence) string {
	for id, item := range evidence {
		if item.Kind == EvidenceTaskProvided {
			return id
		}
	}
	return ""
}

func basePackSymbol(symbol string) string {
	if dot := strings.LastIndex(symbol, "."); dot >= 0 {
		symbol = symbol[dot+1:]
	}
	return strings.TrimPrefix(symbol, "*")
}

func buildProbe(proposed ProposedProbe, selected map[string]struct{}, allowedPaths map[string]struct{}) (Probe, error) {
	if !validProbeAction(proposed.Action) || !validText(proposed.Text, 1024, true) ||
		unknownPathInText(proposed.Text, allowedPaths) || len(proposed.AnchorIDs) > 2 ||
		(proposed.Action != ProbeSearchTaskTerms && len(proposed.AnchorIDs) == 0) ||
		(proposed.Action == ProbeSearchTaskTerms && len(proposed.AnchorIDs) != 0) {
		return Probe{}, fmt.Errorf("invalid content")
	}
	for _, id := range proposed.AnchorIDs {
		if _, ok := selected[id]; !ok {
			return Probe{}, fmt.Errorf("probe anchor is not visible")
		}
	}
	anchorIDs := uniqueSorted(proposed.AnchorIDs)
	if proposed.Action == ProbeSearchTaskTerms && anchorIDs == nil {
		anchorIDs = []string{}
	}
	return Probe{
		Action: proposed.Action, AnchorIDs: anchorIDs,
		Text: localProbeText(proposed.Action),
	}, nil
}

func localProbeText(action ProbeAction) string {
	switch action {
	case ProbeInspectSymbol:
		return "Inspect the exact retained anchor for one additional bounded repository fact."
	case ProbeResolveReference:
		return "Resolve one exact reference adjacent to the selected retained anchor."
	case ProbeCompareConfigCopies:
		return "Compare the selected retained configuration anchors without inferring runtime order."
	case ProbeInspectFixture:
		return "Inspect the selected retained fixture for one exact expected-output fact."
	case ProbeInspectSibling:
		return "Inspect one selected sibling implementation for an exact local obligation."
	case ProbeSearchTaskTerms:
		return "Search the tracked repository for one exact task term and retain only bounded matching source or document evidence."
	default:
		return "Inspect one exact retained anchor for the next unresolved fact."
	}
}

func validateRepository(repository Repository) error {
	if !validText(repository.Identity, 512, true) || !validText(repository.DisplayName, 256, false) ||
		!validText(repository.Revision, 128, true) || !validText(repository.TreeHash, 128, true) ||
		!validSHA256(repository.StateSHA256) || !validText(repository.IdentitySource, 64, true) {
		return fmt.Errorf("task lens: invalid repository identity")
	}
	if !safeRepositoryIdentity(repository.Identity) {
		return fmt.Errorf("task lens: repository identity contains secret-like content")
	}
	switch repository.IdentitySource {
	case "root_module", "manifest":
		if !validPath(repository.IdentitySourcePath) {
			return fmt.Errorf("task lens: invalid repository identity source path")
		}
	case "remote", "neutral_fallback":
		if repository.IdentitySourcePath != "" {
			return fmt.Errorf("task lens: invalid repository identity source path")
		}
	default:
		return fmt.Errorf("task lens: invalid repository identity source")
	}
	return nil
}

func safeRepositoryIdentity(identity string) bool {
	_, found := secretscan.Detect(identity)
	return !found
}

func validateBudget(budget Budgets) error {
	if budget.InitialCandidates < 0 || budget.InitialCandidates > MaxInitialCandidates ||
		budget.CandidateItemsFound < budget.InitialCandidates ||
		budget.RetainedAnchors < MinVisibleAnchors || budget.RetainedAnchors > MaxRetainedAnchors ||
		budget.AnchorItemsFound < budget.RetainedAnchors ||
		budget.EvidenceFilesConsidered < 0 || budget.EvidenceFilesConsidered > MaxReadFiles ||
		budget.ReadFiles < 0 || budget.ReadFiles > MaxReadFiles || budget.ReadBytes < 0 || budget.ReadBytes > MaxReadBytes ||
		budget.SourceScanBytes < 0 || budget.SourceScanBytes > MaxSourceScanBytes ||
		budget.RetainedSourceBytes < 0 || budget.RetainedSourceBytes > MaxRetainedSourceBytes ||
		budget.GoplsQueries < 0 || budget.GoplsQueries > MaxGoplsQueries ||
		budget.FrontierExpansions < 0 || budget.FrontierExpansions > MaxFrontierExpansions ||
		budget.LocalWallMillis < 0 || budget.LocalWallMillis > MaxLocalWallMillis {
		return fmt.Errorf("task lens: retrieval budget exceeded")
	}
	return nil
}

func validTaskKind(kind TaskKind) bool {
	switch kind {
	case TaskBug, TaskFeature, TaskExtension, TaskConfiguration, TaskOperational, TaskCompatibility, TaskUnknown:
		return true
	default:
		return false
	}
}

func validLocality(locality Locality) bool {
	switch locality {
	case LocalityLocalExact, LocalityBoundedCrossFile, LocalityExtension, LocalityBroadDynamic:
		return true
	default:
		return false
	}
}

func validEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceRepositoryFact, EvidenceDocumentClaim, EvidenceTaskProvided:
		return true
	default:
		return false
	}
}

func validAnchorRole(role AnchorRole) bool {
	switch role {
	case RoleSymptomSite, RolePublicOrCLIEntry, RoleStateOwner, RoleStateMutation,
		RoleConfigurationSource, RoleConfigurationCopy, RoleErrorCreation, RoleErrorMapping,
		RoleIntegrationBoundary, RoleRepresentativeImplementation, RoleGeneratedOutput,
		RoleReproductionAnchor, RoleVerificationAnchor, RoleDocumentationContract,
		RoleTransformation, RoleUnsafeOperation, RoleNilHandoff, RoleEffectiveDestination,
		RolePublicErrorType, RoleErrorNormalizer, RolePublicErrorExposure, RoleExtensionPort,
		RoleWiringComposition, RoleOperationalEntry, RoleProceduralBody, RoleModuleTopology,
		RoleSafetyCheck, RoleExample, RoleRepositoryVerificationCommand:
		return true
	default:
		return false
	}
}

func validSupportType(support SupportType) bool {
	switch support {
	case SupportLocallyObserved, SupportDocument, SupportModelHypothesis, SupportUnresolved:
		return true
	default:
		return false
	}
}

func validHypothesisStatus(status HypothesisStatus) bool {
	switch status {
	case HypothesisSupported, HypothesisPlausible, HypothesisUnresolved:
		return true
	default:
		return false
	}
}

func validGuidanceAuthority(authority GuidanceAuthority) bool {
	switch authority {
	case AuthorityTaskProvided, AuthorityRepositoryDocument, AuthorityRepositoryTest,
		AuthorityRepositoryObservation, AuthorityMissing:
		return true
	default:
		return false
	}
}

func validProbeAction(action ProbeAction) bool {
	switch action {
	case ProbeInspectSymbol, ProbeResolveReference, ProbeCompareConfigCopies, ProbeInspectFixture, ProbeInspectSibling, ProbeSearchTaskTerms:
		return true
	default:
		return false
	}
}

func validText(value string, maxBytes int, required bool) bool {
	if value != strings.TrimSpace(value) || len(value) > maxBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r") {
		return false
	}
	if required && value == "" {
		return false
	}
	return true
}

func validPath(value string) bool {
	cleaned := path.Clean(value)
	return value != "" && cleaned == value && cleaned != "." && !path.IsAbs(value) &&
		!strings.HasPrefix(cleaned, "../") && !strings.Contains(value, "\\") &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func validOpaque(value string) bool {
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "\x00\r\n/\\") {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); err == nil {
		return fmt.Errorf("task lens: artifact contains multiple json values")
	} else if err != io.EOF {
		return fmt.Errorf("task lens: artifact has trailing data: %w", err)
	}
	return nil
}

func uniqueSorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return slices.Compact(result)
}

func subset(values, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func sameEndpoints(leftA, rightA, leftB, rightB string) bool {
	return leftA == leftB && rightA == rightB
}

func hasEvidenceKind(ids []string, evidence map[string]Evidence, kind EvidenceKind) bool {
	for _, id := range ids {
		if evidence[id].Kind == kind {
			return true
		}
	}
	return false
}

func allEvidenceKind(ids []string, evidence map[string]Evidence, kind EvidenceKind) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if evidence[id].Kind != kind {
			return false
		}
	}
	return true
}

func hasRepositoryEvidence(ids []string, evidence map[string]Evidence) bool {
	return hasEvidenceKind(ids, evidence, EvidenceRepositoryFact) ||
		hasEvidenceKind(ids, evidence, EvidenceDocumentClaim)
}

func allTestOrExampleEvidence(ids []string, index bundleIndex) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		item, ok := index.evidence[id]
		if !ok || item.Path == "" {
			return false
		}
		if isTestPath(item.Path) {
			continue
		}
		return false
	}
	return true
}

var (
	pathTokenPattern   = regexp.MustCompile(`[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*\.(?:go|md|mdx|rst|json|ya?ml|toml|sh|mod|sum|proto|py|pyi|ts|tsx|js|jsx|sql|rs|java|kt|kts|c|cc|cpp|h|hpp|cs|rb|php|swift|scala|ex|exs|erl|hrl|tf|hcl|xml|html|css|scss|vue|svelte|bazel|gradle|properties|ini|cfg|conf)\b`)
	slashPathPattern   = regexp.MustCompile(`\b[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+\b`)
	specialPathPattern = regexp.MustCompile(`\b(?:Makefile|Dockerfile|CMakeLists\.txt|go\.work)\b`)
)

func unknownPathInText(text string, allowed map[string]struct{}) bool {
	return !pathReferencesGrounded(text, allowed, "")
}

func pathReferencesGrounded(text string, allowed map[string]struct{}, groundingText string) bool {
	grounding := normalizedPathGroundingText(groundingText)
	for _, pattern := range []*regexp.Regexp{pathTokenPattern, slashPathPattern, specialPathPattern} {
		for _, token := range pattern.FindAllString(text, -1) {
			if _, ok := allowed[token]; !ok {
				if grounding == "" || !strings.Contains(grounding, normalizedPathGroundingText(token)) {
					return false
				}
			}
		}
	}
	return true
}

func impliesRuntimeOrCausality(text string) bool {
	lower := " " + strings.ToLower(strings.Join(strings.Fields(text), " ")) + " "
	for _, marker := range []string{
		" causes ", " caused ", " causing ", " can cause ", " leads to ", " results in ",
		" because ", " therefore ", " due to ", " runtime ", " executes ", " invokes ",
		" calls ", " dispatches ", " delegates ", " directly ", " originates ",
		" triggers ", " propagates ", " routes ", " always ", " guarantees ", " then ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func impliesBehaviorBeyondCallSyntax(text string) bool {
	lower := " " + strings.ToLower(strings.Join(strings.Fields(text), " ")) + " "
	for _, marker := range []string{
		" causes ", " caused ", " causing ", " can cause ", " leads to ", " results in ",
		" because ", " therefore ", " due to ", " runtime ", " executes ",
		" dispatches ", " originates ", " triggers ", " propagates ", " routes ",
		" always ", " guarantees ", " then ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
