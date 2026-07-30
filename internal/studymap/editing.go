package studymap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/artifactrole"
)

const (
	BriefShapeProposalVersion = 1
	DirectionProposalVersion  = 1
	ReviewBundleVersion       = 1
	ReviewProposalVersion     = 1
	ReviewReductionVersion    = 1

	MinReviewedDirections = MinDirections
	MaxReviewedDirections = 6

	maxEditingArtifactBytes = 4 << 20
	maxReviewBundleBytes    = 64 << 10
	maxReviewSourceBytes    = 16 << 10
	maxReviewSourceLines    = 60

	// Provider recovery examines only a bounded number of object starts before
	// the existing strict envelope and semantic validators take over.
	maxProviderEnvelopeCandidates = 16

	// A provider sometimes preserves the distinguishing hash prefix while
	// shortening an opaque bundle reference. Only a sufficiently long prefix
	// that resolves to exactly one supplied ID can be restored locally.
	minUniqueBundleReferencePrefixBytes = 12
)

// AnchorFit describes how closely one exact source window answers a fixed
// Study Direction. It is an editorial assessment, not runtime evidence.
type AnchorFit string

const (
	AnchorFitDirect     AnchorFit = "direct"
	AnchorFitSupporting AnchorFit = "supporting"
	AnchorFitWeak       AnchorFit = "weak"
	AnchorFitIrrelevant AnchorFit = "irrelevant"
)

// ReadingRole is a closed presentation role for one reviewed source anchor.
type ReadingRole string

const (
	ReadingRoleDocumentationIntent          ReadingRole = "documentation_intent"
	ReadingRolePublicOrCLIEntry             ReadingRole = "public_or_cli_entry"
	ReadingRoleCoreOrchestration            ReadingRole = "core_orchestration"
	ReadingRoleStateOrDataModel             ReadingRole = "state_or_data_model"
	ReadingRoleEffectOrIntegrationBoundary  ReadingRole = "effect_or_integration_boundary"
	ReadingRoleRepresentativeImplementation ReadingRole = "representative_implementation"
	ReadingRoleConfigurationOrOperations    ReadingRole = "configuration_or_operations"
	ReadingRoleExampleOrUsage               ReadingRole = "example_or_usage"
	ReadingRoleTestOrVerification           ReadingRole = "test_or_verification"
)

// OverclaimReason is a closed explanation of why presentation copy may be
// broader than the supplied source window.
type OverclaimReason string

const (
	OverclaimNone                        OverclaimReason = "none"
	OverclaimWrongResponsibility         OverclaimReason = "wrong_responsibility"
	OverclaimBehaviorOutsideWindow       OverclaimReason = "behavior_outside_window"
	OverclaimUnsupportedRuntimeOrder     OverclaimReason = "unsupported_runtime_order"
	OverclaimUnsupportedCausality        OverclaimReason = "unsupported_causality"
	OverclaimQuestionScopeBroader        OverclaimReason = "question_scope_broader"
	OverclaimLearningOutcomeScopeBroader OverclaimReason = "learning_outcome_scope_broader"
	OverclaimVagueOrGeneric              OverclaimReason = "vague_or_generic"
)

// BriefShapeProposal is the independently replayable global editorial output
// for repository identity and shape. It deliberately contains no directions.
type BriefShapeProposal struct {
	Version        int            `json:"version"`
	RepositoryType RepositoryType `json:"repository_type"`
	Brief          Brief          `json:"brief"`
	ShapeAreaIDs   []string       `json:"shape_area_ids"`
}

type briefShapeProviderResponse struct {
	Version        int                   `json:"version"`
	RepositoryType RepositoryType        `json:"repository_type"`
	Brief          briefProviderResponse `json:"brief"`
	ShapeAreaIDs   []string              `json:"shape_area_ids"`
	DomainTerms    json.RawMessage       `json:"domain_terms,omitempty"`
}

type briefProviderResponse struct {
	WhatItIs              BriefStatement  `json:"what_it_is"`
	Problem               BriefStatement  `json:"problem"`
	MainInput             BriefStatement  `json:"main_input"`
	CentralResponsibility BriefStatement  `json:"central_responsibility"`
	ObservableResult      BriefStatement  `json:"observable_result"`
	DomainTerms           json.RawMessage `json:"domain_terms,omitempty"`
}

// DirectionProposal contains bounded direction drafts. Direction IDs are
// fixed before review so independently saved review artifacts cannot drift
// between candidates.
type DirectionProposal struct {
	Version    int                  `json:"version"`
	Directions []DirectionCandidate `json:"directions"`
}

// DirectionProposalDiagnostics records bounded provider-item losses without
// copying provider prose into a saved diagnostic.
type DirectionProposalDiagnostics struct {
	Received int                      `json:"received"`
	Accepted int                      `json:"accepted"`
	Rejected int                      `json:"rejected"`
	Issues   []DirectionProposalIssue `json:"issues,omitempty"`
}

type DirectionProposalIssue struct {
	Position int    `json:"position"`
	Code     string `json:"code"`
}

type directionProposalProviderResponse struct {
	Version    int             `json:"version"`
	Directions json.RawMessage `json:"directions"`
}

// DirectionCandidate is a Study Direction before its reading anchors have
// been checked against exact source. Confidence is intentionally absent: the
// local reducer ranks reviewed directions instead of trusting model confidence.
type DirectionCandidate struct {
	DirectionID     string          `json:"direction_id,omitempty"`
	Question        string          `json:"question"`
	WhyItMatters    string          `json:"why_it_matters"`
	LearningOutcome string          `json:"learning_outcome"`
	TargetJob       TargetJob       `json:"target_user_job"`
	LearningStage   LearningStage   `json:"learning_stage"`
	AnchorIDs       []string        `json:"anchor_ids"`
	DocumentIDs     []string        `json:"document_ids,omitempty"`
	AreaIDs         []string        `json:"area_ids,omitempty"`
	MechanismID     string          `json:"mechanism_id,omitempty"`
	ReadingAnchors  []ReadingAnchor `json:"reading_anchors"`
	SearchQueries   []string        `json:"search_queries,omitempty"`
}

// IncompleteDirection retains one bounded editorial question with only the
// exact reading entries that the provider actually supplied and the saved
// bundle authorizes. It is not a complete Reading Pack.
type IncompleteDirection struct {
	DirectionID     string          `json:"direction_id"`
	Question        string          `json:"question"`
	WhyItMatters    string          `json:"why_it_matters"`
	LearningOutcome string          `json:"learning_outcome"`
	TargetJob       TargetJob       `json:"target_user_job"`
	LearningStage   LearningStage   `json:"learning_stage"`
	ReadingAnchors  []ReadingAnchor `json:"reading_anchors"`
}

type incompleteDirectionCandidate struct {
	DirectionID     string          `json:"direction_id,omitempty"`
	Question        string          `json:"question"`
	WhyItMatters    string          `json:"why_it_matters"`
	LearningOutcome string          `json:"learning_outcome"`
	TargetJob       TargetJob       `json:"target_user_job"`
	LearningStage   LearningStage   `json:"learning_stage"`
	AnchorIDs       json.RawMessage `json:"anchor_ids"`
	DocumentIDs     json.RawMessage `json:"document_ids,omitempty"`
	AreaIDs         json.RawMessage `json:"area_ids,omitempty"`
	MechanismID     json.RawMessage `json:"mechanism_id,omitempty"`
	ReadingAnchors  json.RawMessage `json:"reading_anchors"`
	SearchQueries   json.RawMessage `json:"search_queries,omitempty"`
}

// ReviewBundle is one fixed direction plus only its selected source anchors.
// It is suitable for one independent, bounded semantic-fit call.
type ReviewBundle struct {
	Version         int            `json:"version"`
	DirectionID     string         `json:"direction_id"`
	Question        string         `json:"question"`
	LearningOutcome string         `json:"learning_outcome"`
	Anchors         []ReviewAnchor `json:"anchors"`
}

// ReviewAnchor contains local authority copied from Bundle. SourceFragment is
// line-numbered, contiguous, includes Line, and is capped locally.
type ReviewAnchor struct {
	AnchorID        string             `json:"anchor_id"`
	RepositoryRole  artifactrole.Role  `json:"repository_role"`
	Path            string             `json:"path"`
	Symbol          string             `json:"symbol"`
	Line            int                `json:"line"`
	Areas           []ReviewArea       `json:"areas,omitempty"`
	SourceFragment  []ReviewSourceLine `json:"source_fragment"`
	CurrentSentence string             `json:"current_sentence"`
}

type ReviewArea struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Responsibility string `json:"responsibility,omitempty"`
}

type ReviewSourceLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ReviewProposal is model-authored editorial metadata. ApplyReviews resolves
// every ID against a ReviewBundle/Bundle before any direction is retained.
type ReviewProposal struct {
	Version     int            `json:"version"`
	DirectionID string         `json:"direction_id"`
	Reviews     []AnchorReview `json:"reviews"`
}

type AnchorReview struct {
	AnchorID                string            `json:"anchor_id"`
	Fit                     AnchorFit         `json:"fit"`
	SupportedObservation    string            `json:"supported_observation"`
	Role                    ReadingRole       `json:"role"`
	OverclaimReasons        []OverclaimReason `json:"overclaim_reasons"`
	NarrowerDisplaySentence string            `json:"narrower_display_sentence,omitempty"`
}

type ReviewIssue struct {
	DirectionID string `json:"direction_id,omitempty"`
	Code        string `json:"code"`
	Detail      string `json:"detail,omitempty"`
}

type ReviewedDirection struct {
	DirectionID      string         `json:"direction_id"`
	Candidate        Candidate      `json:"candidate"`
	Reviews          []AnchorReview `json:"reviews"`
	QualityScore     int            `json:"quality_score"`
	RoleDiversity    int            `json:"role_diversity"`
	QuestionFitScore int            `json:"question_fit_score"`
	proposalIndex    int
}

// ReviewReduction keeps review failures local to their direction and records
// the independently useful accepted set before and after compression.
type ReviewReduction struct {
	Version    int                 `json:"version"`
	Proposed   int                 `json:"proposed"`
	Reviewed   int                 `json:"reviewed"`
	Selected   int                 `json:"selected"`
	Directions []ReviewedDirection `json:"directions,omitempty"`
	Issues     []ReviewIssue       `json:"issues,omitempty"`
}

func DecodeBriefShapeProposal(raw []byte) (BriefShapeProposal, error) {
	var response briefShapeProviderResponse
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"brief and shape proposal",
		&response,
	); err != nil {
		return BriefShapeProposal{}, err
	}
	if len(response.DomainTerms) > 0 && len(response.Brief.DomainTerms) > 0 {
		return BriefShapeProposal{},
			fmt.Errorf("study map: brief domain terms are present in both accepted locations")
	}
	domainTermsRaw := response.Brief.DomainTerms
	if len(response.DomainTerms) > 0 {
		domainTermsRaw = response.DomainTerms
	}
	domainTerms, err := decodeBriefProviderDomainTerms(domainTermsRaw)
	if err != nil {
		return BriefShapeProposal{}, err
	}
	// Domain terms are optional editorial context. Keep malformed terms local
	// instead of discarding an otherwise complete, supported repository brief.
	validTerms := make([]BriefDomainTerm, 0, min(len(domainTerms), 8))
	for _, term := range domainTerms {
		if len(validTerms) == 8 {
			break
		}
		if !validText(term.Term, 128, true) || !validText(term.Meaning, 512, true) ||
			len(term.SupportIDs) == 0 || !allOpaque(term.SupportIDs) {
			continue
		}
		validTerms = append(validTerms, term)
	}
	proposal := BriefShapeProposal{
		Version:        response.Version,
		RepositoryType: response.RepositoryType,
		Brief: Brief{
			WhatItIs:              response.Brief.WhatItIs,
			Problem:               response.Brief.Problem,
			MainInput:             response.Brief.MainInput,
			CentralResponsibility: response.Brief.CentralResponsibility,
			ObservableResult:      response.Brief.ObservableResult,
			DomainTerms:           validTerms,
		},
		ShapeAreaIDs: response.ShapeAreaIDs,
	}
	if err := validateBriefShapeStructure(proposal); err != nil {
		return BriefShapeProposal{}, err
	}
	return proposal, nil
}

// RecoverBriefShapeProviderJSON extracts one unambiguous strict brief envelope
// from bounded provider prose. It does not normalize fields or weaken any
// subsequent Study validation.
func RecoverBriefShapeProviderJSON(raw []byte) ([]byte, error) {
	return recoverEditingProviderJSON(
		raw,
		"brief and shape proposal",
		func(candidate []byte) error {
			var response briefShapeProviderResponse
			return decodeEditingJSON(
				candidate,
				maxEditingArtifactBytes,
				"brief and shape proposal",
				&response,
			)
		},
	)
}

func decodeBriefProviderDomainTerms(raw json.RawMessage) ([]BriefDomainTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("study map: brief domain terms must be an array")
	}
	var terms []BriefDomainTerm
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"brief domain terms",
		&terms,
	); err != nil {
		return nil, err
	}
	return terms, nil
}

func DecodeDirectionProposal(raw []byte) (DirectionProposal, error) {
	proposal, _, err := DecodeDirectionProposalWithDiagnostics(raw)
	return proposal, err
}

// RecoverDirectionProviderJSON extracts one unambiguous strict direction
// envelope from bounded provider prose. Candidate-level rejection, opaque-ID
// validation, and canonical saved-artifact decoding remain unchanged.
func RecoverDirectionProviderJSON(raw []byte) ([]byte, error) {
	return recoverEditingProviderJSON(
		raw,
		"direction proposal",
		func(candidate []byte) error {
			var response directionProposalProviderResponse
			return decodeEditingJSON(
				candidate,
				maxEditingArtifactBytes,
				"direction proposal",
				&response,
			)
		},
	)
}

// DecodeDirectionProposalWithDiagnostics keeps the bounded provider envelope
// strict while making independently proposed directions independently
// rejectable. Canonical saved proposals still contain only normalized valid
// directions.
func DecodeDirectionProposalWithDiagnostics(
	raw []byte,
) (DirectionProposal, DirectionProposalDiagnostics, error) {
	var envelope directionProposalProviderResponse
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"direction proposal",
		&envelope,
	); err != nil {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, err
	}
	if envelope.Version != DirectionProposalVersion {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study map: unsupported direction proposal version %d",
			envelope.Version,
		)
	}
	items, err := decodeBoundedDirectionItems(envelope.Directions)
	if err != nil {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, err
	}
	diagnostics := DirectionProposalDiagnostics{Received: len(items)}
	directions := make([]DirectionCandidate, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	reject := func(position int, code string) {
		diagnostics.Issues = append(diagnostics.Issues, DirectionProposalIssue{
			Position: position,
			Code:     code,
		})
	}
	for position, item := range items {
		var direction DirectionCandidate
		if err := decodeEditingJSON(
			item,
			maxEditingArtifactBytes,
			"direction candidate",
			&direction,
		); err != nil {
			reject(position, "decode_candidate")
			continue
		}
		if strings.TrimSpace(direction.DirectionID) != "" {
			reject(position, "model_direction_id")
			continue
		}
		direction.DirectionID = ""
		normalized, err := normalizeDirectionCandidate(direction)
		if err != nil {
			reject(position, directionCandidateValidationCode(err))
			continue
		}
		if _, duplicate := seen[normalized.DirectionID]; duplicate {
			reject(position, "duplicate_direction_id")
			continue
		}
		seen[normalized.DirectionID] = struct{}{}
		directions = append(directions, normalized)
	}
	diagnostics.Accepted = len(directions)
	diagnostics.Rejected = len(diagnostics.Issues)
	if diagnostics.Accepted == 0 {
		return DirectionProposal{}, diagnostics,
			fmt.Errorf("study map: direction proposal has no valid candidates")
	}
	return DirectionProposal{
		Version:    envelope.Version,
		Directions: directions,
	}, diagnostics, nil
}

// DecodeIncompleteDirections projects independently useful weak Study signals
// without weakening DecodeDirectionProposal. The raw envelope and item count
// stay unchanged; one invalid item never authorizes or discards a sibling.
func DecodeIncompleteDirections(
	raw []byte,
	bundle Bundle,
) ([]IncompleteDirection, DirectionProposalDiagnostics, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	var envelope directionProposalProviderResponse
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"incomplete direction projection",
		&envelope,
	); err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	if envelope.Version != DirectionProposalVersion {
		return nil, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study map: unsupported direction proposal version %d",
			envelope.Version,
		)
	}
	items, err := decodeBoundedDirectionItems(envelope.Directions)
	if err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	anchors := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchors[anchor.ID] = struct{}{}
	}
	diagnostics := DirectionProposalDiagnostics{Received: len(items)}
	result := make([]IncompleteDirection, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	reject := func(position int, code string) {
		diagnostics.Issues = append(diagnostics.Issues, DirectionProposalIssue{
			Position: position,
			Code:     code,
		})
	}
	for position, item := range items {
		var candidate incompleteDirectionCandidate
		if err := decodeEditingJSON(
			item,
			maxEditingArtifactBytes,
			"incomplete direction candidate",
			&candidate,
		); err != nil {
			reject(position, "decode_candidate")
			continue
		}
		if len(candidate.DirectionID) > 256 ||
			strings.TrimSpace(candidate.DirectionID) != "" {
			reject(position, "model_direction_id")
			continue
		}
		if !validIncompleteDirectionScalars(candidate) {
			reject(position, "invalid_candidate")
			continue
		}
		reading, err := decodeIncompleteReadingAnchors(candidate.ReadingAnchors, anchors)
		if err != nil {
			reject(position, "invalid_reading_anchors")
			continue
		}
		anchorIDs := make([]string, 0, len(reading))
		for _, item := range reading {
			anchorIDs = append(anchorIDs, item.AnchorID)
		}
		directionID := localDirectionID(DirectionCandidate{
			Question:  candidate.Question,
			AnchorIDs: anchorIDs,
		})
		if _, duplicate := seen[directionID]; duplicate {
			reject(position, "duplicate_direction_id")
			continue
		}
		seen[directionID] = struct{}{}
		result = append(result, IncompleteDirection{
			DirectionID:     directionID,
			Question:        strings.TrimSpace(candidate.Question),
			WhyItMatters:    strings.TrimSpace(candidate.WhyItMatters),
			LearningOutcome: strings.TrimSpace(candidate.LearningOutcome),
			TargetJob:       candidate.TargetJob,
			LearningStage:   candidate.LearningStage,
			ReadingAnchors:  reading,
		})
	}
	diagnostics.Accepted = len(result)
	diagnostics.Rejected = len(diagnostics.Issues)
	return result, diagnostics, nil
}

func validIncompleteDirectionScalars(candidate incompleteDirectionCandidate) bool {
	if len(candidate.Question) > 512 ||
		len(candidate.WhyItMatters) > 1024 ||
		len(candidate.LearningOutcome) > 1024 {
		return false
	}
	return naturalQuestion(candidate.Question) &&
		validText(candidate.WhyItMatters, 1024, true) &&
		!impliesRuntimeOrder(candidate.WhyItMatters) &&
		validText(candidate.LearningOutcome, 1024, true) &&
		!impliesRuntimeOrder(candidate.LearningOutcome) &&
		validTargetJob(candidate.TargetJob) &&
		validLearningStage(candidate.LearningStage)
}

func decodeIncompleteReadingAnchors(
	raw json.RawMessage,
	allowed map[string]struct{},
) ([]ReadingAnchor, error) {
	if len(raw) == 0 || len(raw) > maxEditingArtifactBytes {
		return nil, fmt.Errorf("study map: incomplete reading anchors are outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	start, ok := token.(json.Delim)
	if !ok || start != '[' {
		return nil, fmt.Errorf("study map: incomplete reading anchors must be an array")
	}
	result := make([]ReadingAnchor, 0, 5)
	seen := make(map[string]struct{}, 5)
	for decoder.More() {
		if len(result) == 5 {
			return nil, fmt.Errorf("study map: incomplete reading anchor count is outside bounds")
		}
		var reading ReadingAnchor
		if err := decoder.Decode(&reading); err != nil {
			return nil, err
		}
		if len(reading.AnchorID) > 256 ||
			len(reading.Label) > 64 ||
			len(reading.WhatToLookFor) > 768 ||
			!validOpaque(reading.AnchorID) ||
			!validReadingLabel(reading.Label) ||
			!validText(reading.WhatToLookFor, 768, true) ||
			impliesRuntimeOrder(reading.WhatToLookFor) {
			return nil, fmt.Errorf("study map: invalid incomplete reading anchor")
		}
		if _, ok := allowed[reading.AnchorID]; !ok {
			return nil, fmt.Errorf("study map: unresolved incomplete reading anchor")
		}
		if _, duplicate := seen[reading.AnchorID]; duplicate {
			return nil, fmt.Errorf("study map: duplicate incomplete reading anchor")
		}
		seen[reading.AnchorID] = struct{}{}
		result = append(result, ReadingAnchor{
			AnchorID:      strings.TrimSpace(reading.AnchorID),
			Label:         strings.TrimSpace(reading.Label),
			WhatToLookFor: strings.TrimSpace(reading.WhatToLookFor),
		})
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := end.(json.Delim); !ok || delim != ']' {
		return nil, fmt.Errorf("study map: invalid incomplete reading anchor closure")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("study map: invalid trailing incomplete reading anchors")
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("study map: incomplete reading anchor count is outside bounds")
	}
	return result, nil
}

func decodeBoundedDirectionItems(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maxEditingArtifactBytes {
		return nil, fmt.Errorf("study map: direction candidates are outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("study map: decode direction candidates: %w", err)
	}
	start, ok := token.(json.Delim)
	if !ok || start != '[' {
		return nil, fmt.Errorf("study map: direction candidates must be an array")
	}
	items := make([]json.RawMessage, 0, MaxCandidates)
	for decoder.More() {
		if len(items) == MaxCandidates {
			return nil, fmt.Errorf(
				"study map: direction count must be between 1 and %d",
				MaxCandidates,
			)
		}
		var item json.RawMessage
		if err := decoder.Decode(&item); err != nil {
			return nil, fmt.Errorf("study map: decode direction candidate: %w", err)
		}
		items = append(items, item)
	}
	token, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("study map: decode direction candidates: %w", err)
	}
	end, ok := token.(json.Delim)
	if !ok || end != ']' {
		return nil, fmt.Errorf("study map: direction candidates must end with an array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("study map: trailing JSON in direction candidates")
		}
		return nil, fmt.Errorf("study map: invalid trailing JSON in direction candidates: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf(
			"study map: direction count must be between 1 and %d",
			MaxCandidates,
		)
	}
	return items, nil
}

// DecodeNormalizedDirectionProposal validates a saved local projection. Unlike
// DecodeDirectionProposal, it requires every non-empty direction ID to be the
// exact value derived from the normalized question and selected anchors. This
// keeps model output and replayable local artifacts as separate contracts.
func DecodeNormalizedDirectionProposal(raw []byte) (DirectionProposal, error) {
	var proposal DirectionProposal
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"normalized direction proposal",
		&proposal,
	); err != nil {
		return DirectionProposal{}, err
	}
	if err := validateDirectionProposalStructure(proposal); err != nil {
		return DirectionProposal{}, err
	}
	return proposal, nil
}

// NormalizeDirectionProposal assigns locally derived stable IDs. A
// programmatic caller may pass an already-normalized proposal, but a non-empty
// ID must exactly match the value derived from question and selected anchors.
func NormalizeDirectionProposal(proposal DirectionProposal) (DirectionProposal, error) {
	if err := validateDirectionProposalBounds(proposal); err != nil {
		return DirectionProposal{}, err
	}
	proposal.Directions = append([]DirectionCandidate(nil), proposal.Directions...)
	seen := make(map[string]struct{}, len(proposal.Directions))
	for index := range proposal.Directions {
		direction := &proposal.Directions[index]
		if err := validateDirectionCandidateContent(*direction); err != nil {
			return DirectionProposal{}, err
		}
		derivedID := localDirectionID(*direction)
		if direction.DirectionID != "" && direction.DirectionID != derivedID {
			return DirectionProposal{}, fmt.Errorf(
				"study map: direction id %q is not locally derived",
				direction.DirectionID,
			)
		}
		direction.DirectionID = derivedID
		if _, duplicate := seen[derivedID]; duplicate {
			return DirectionProposal{}, fmt.Errorf("study map: duplicate direction id %q", derivedID)
		}
		seen[derivedID] = struct{}{}
	}
	return proposal, nil
}

// ResolveDirectionProposalReferences restores uniquely identifiable shortened
// bundle references before source review. It never guesses: an unknown, short,
// or ambiguous prefix remains invalid.
func ResolveDirectionProposalReferences(
	bundle Bundle,
	proposal DirectionProposal,
) (DirectionProposal, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return DirectionProposal{}, err
	}
	if err := validateDirectionProposalBounds(proposal); err != nil {
		return DirectionProposal{}, err
	}
	anchorIDs := make([]string, 0, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchorIDs = append(anchorIDs, anchor.ID)
	}
	documentIDs := make([]string, 0, len(bundle.Documents))
	for _, document := range bundle.Documents {
		documentIDs = append(documentIDs, document.ID)
	}
	areaIDs := make([]string, 0, len(bundle.Areas))
	for _, area := range bundle.Areas {
		areaIDs = append(areaIDs, area.ID)
	}
	mechanismIDs := make([]string, 0, len(bundle.Mechanisms))
	for _, mechanism := range bundle.Mechanisms {
		mechanismIDs = append(mechanismIDs, mechanism.ID)
	}

	proposal.Directions = append([]DirectionCandidate(nil), proposal.Directions...)
	for directionIndex := range proposal.Directions {
		direction := &proposal.Directions[directionIndex]
		direction.DirectionID = ""
		direction.AnchorIDs = append([]string(nil), direction.AnchorIDs...)
		direction.DocumentIDs = append([]string(nil), direction.DocumentIDs...)
		direction.AreaIDs = append([]string(nil), direction.AreaIDs...)
		direction.ReadingAnchors = append([]ReadingAnchor(nil), direction.ReadingAnchors...)

		for index, reference := range direction.AnchorIDs {
			resolved, err := resolveUniqueBundleReference(reference, anchorIDs)
			if err != nil {
				return DirectionProposal{}, fmt.Errorf("study map: anchor reference: %w", err)
			}
			direction.AnchorIDs[index] = resolved
		}
		for index, reading := range direction.ReadingAnchors {
			resolved, err := resolveUniqueBundleReference(reading.AnchorID, anchorIDs)
			if err != nil {
				return DirectionProposal{}, fmt.Errorf("study map: reading anchor reference: %w", err)
			}
			direction.ReadingAnchors[index].AnchorID = resolved
		}
		for index, reference := range direction.DocumentIDs {
			resolved, err := resolveUniqueBundleReference(reference, documentIDs)
			if err != nil {
				return DirectionProposal{}, fmt.Errorf("study map: document reference: %w", err)
			}
			direction.DocumentIDs[index] = resolved
		}
		for index, reference := range direction.AreaIDs {
			resolved, err := resolveUniqueBundleReference(reference, areaIDs)
			if err != nil {
				return DirectionProposal{}, fmt.Errorf("study map: area reference: %w", err)
			}
			direction.AreaIDs[index] = resolved
		}
		if direction.MechanismID != "" {
			resolved, err := resolveUniqueBundleReference(direction.MechanismID, mechanismIDs)
			if err != nil {
				return DirectionProposal{}, fmt.Errorf("study map: mechanism reference: %w", err)
			}
			direction.MechanismID = resolved
		}
	}
	return NormalizeDirectionProposal(proposal)
}

func resolveUniqueBundleReference(reference string, allowed []string) (string, error) {
	for _, candidate := range allowed {
		if reference == candidate {
			return reference, nil
		}
	}
	if len(reference) < minUniqueBundleReferencePrefixBytes {
		return "", fmt.Errorf("reference %q is not exact and its prefix is too short", reference)
	}
	match := ""
	for _, candidate := range allowed {
		if !strings.HasPrefix(candidate, reference) {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("reference %q has an ambiguous prefix", reference)
		}
		match = candidate
	}
	if match == "" {
		return "", fmt.Errorf("reference %q is not supplied by the bundle", reference)
	}
	return match, nil
}

func DecodeReviewBundle(raw []byte) (ReviewBundle, error) {
	var bundle ReviewBundle
	if err := decodeEditingJSON(raw, maxReviewBundleBytes, "review bundle", &bundle); err != nil {
		return ReviewBundle{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ReviewBundle{}, err
	}
	return bundle, nil
}

func DecodeReviewProposal(raw []byte) (ReviewProposal, error) {
	var proposal ReviewProposal
	if err := decodeEditingJSON(raw, maxEditingArtifactBytes, "review proposal", &proposal); err != nil {
		return ReviewProposal{}, err
	}
	if err := validateReviewProposal(proposal); err != nil {
		return ReviewProposal{}, err
	}
	return proposal, nil
}

// BuildReviewBundle copies one candidate's exact local anchors into a bounded
// line-numbered review input. It performs no repository I/O.
func BuildReviewBundle(bundle Bundle, direction DirectionCandidate) (ReviewBundle, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return ReviewBundle{}, err
	}
	normalized, err := normalizeDirectionCandidate(direction)
	if err != nil {
		return ReviewBundle{}, err
	}
	direction = normalized
	index := newBundleIndex(bundle)
	if _, issues := validateCandidate(direction.candidate(), 0, index); len(issues) > 0 {
		return ReviewBundle{}, fmt.Errorf("study map: review direction is invalid: %s", issues[0].Code)
	}
	readingByID := make(map[string]ReadingAnchor, len(direction.ReadingAnchors))
	for _, reading := range direction.ReadingAnchors {
		readingByID[reading.AnchorID] = reading
	}
	fragments := make([]reviewFragment, 0, len(direction.AnchorIDs))
	for _, anchorID := range direction.AnchorIDs {
		anchor := index.anchors[anchorID]
		fragment, err := fullReviewFragment(anchor)
		if err != nil {
			return ReviewBundle{}, err
		}
		fragments = append(fragments, fragment)
	}
	if err := shrinkReviewFragments(fragments); err != nil {
		return ReviewBundle{}, err
	}
	review := ReviewBundle{
		Version: ReviewBundleVersion, DirectionID: direction.DirectionID,
		Question: direction.Question, LearningOutcome: direction.LearningOutcome,
		Anchors: make([]ReviewAnchor, 0, len(fragments)),
	}
	for _, fragment := range fragments {
		anchor := index.anchors[fragment.anchorID]
		reviewAnchor := ReviewAnchor{
			AnchorID: anchor.ID, RepositoryRole: anchor.Role,
			Path: anchor.Path, Symbol: anchor.Symbol, Line: anchor.Line,
			SourceFragment:  append([]ReviewSourceLine(nil), fragment.lines...),
			CurrentSentence: readingByID[anchor.ID].WhatToLookFor,
		}
		for _, areaID := range anchor.AreaIDs {
			area := index.areas[areaID]
			reviewAnchor.Areas = append(reviewAnchor.Areas, ReviewArea{
				ID: area.ID, Name: area.Name, Responsibility: area.Responsibility,
			})
		}
		review.Anchors = append(review.Anchors, reviewAnchor)
	}
	if err := review.Validate(); err != nil {
		return ReviewBundle{}, err
	}
	return review, nil
}

// Validate checks that a replayed ReviewBundle is independently bounded and
// internally consistent. Bundle identity is checked separately by its caller.
func (bundle ReviewBundle) Validate() error {
	if bundle.Version != ReviewBundleVersion {
		return fmt.Errorf("study map: unsupported review bundle version %d", bundle.Version)
	}
	if !validOpaque(bundle.DirectionID) || !naturalQuestion(bundle.Question) ||
		!validText(bundle.LearningOutcome, 1024, true) || impliesRuntimeOrder(bundle.LearningOutcome) {
		return fmt.Errorf("study map: invalid review direction")
	}
	if len(bundle.Anchors) < 3 || len(bundle.Anchors) > 5 {
		return fmt.Errorf("study map: review bundle must contain three to five anchors")
	}
	seen := make(map[string]struct{}, len(bundle.Anchors))
	totalBytes := 0
	for _, anchor := range bundle.Anchors {
		if !validOpaque(anchor.AnchorID) || !validRole(anchor.RepositoryRole) ||
			!validPath(anchor.Path) || !validText(anchor.Symbol, 256, true) || anchor.Line <= 0 ||
			!validText(anchor.CurrentSentence, 768, true) || impliesRuntimeOrder(anchor.CurrentSentence) {
			return fmt.Errorf("study map: invalid review anchor %q", anchor.AnchorID)
		}
		if _, duplicate := seen[anchor.AnchorID]; duplicate {
			return fmt.Errorf("study map: duplicate review anchor %q", anchor.AnchorID)
		}
		seen[anchor.AnchorID] = struct{}{}
		if len(anchor.SourceFragment) == 0 || len(anchor.SourceFragment) > maxReviewSourceLines {
			return fmt.Errorf("study map: review source line count is outside bounds")
		}
		containsAnchorLine := false
		for index, line := range anchor.SourceFragment {
			if line.Line <= 0 || !utf8.ValidString(line.Text) || strings.ContainsAny(line.Text, "\x00\r\n") {
				return fmt.Errorf("study map: invalid review source line")
			}
			if index > 0 && line.Line != anchor.SourceFragment[index-1].Line+1 {
				return fmt.Errorf("study map: review source lines are not contiguous")
			}
			containsAnchorLine = containsAnchorLine || line.Line == anchor.Line
			totalBytes += reviewSourceLineBytes(line)
		}
		if !containsAnchorLine {
			return fmt.Errorf("study map: review source omits anchor line")
		}
		areaIDs := make(map[string]struct{}, len(anchor.Areas))
		for _, area := range anchor.Areas {
			if !validOpaque(area.ID) || !validText(area.Name, 256, true) ||
				!validText(area.Responsibility, 1024, false) {
				return fmt.Errorf("study map: invalid review area")
			}
			if _, duplicate := areaIDs[area.ID]; duplicate {
				return fmt.Errorf("study map: duplicate review area")
			}
			areaIDs[area.ID] = struct{}{}
		}
	}
	if totalBytes > maxReviewSourceBytes {
		return fmt.Errorf("study map: review source exceeds %d bytes", maxReviewSourceBytes)
	}
	return nil
}

// ApplyReviews validates each response independently. A missing, duplicate,
// unknown, or malformed response rejects only its matching direction.
func ApplyReviews(
	bundle Bundle,
	proposal DirectionProposal,
	reviews []ReviewProposal,
) (ReviewReduction, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return ReviewReduction{}, err
	}
	var err error
	proposal, err = NormalizeDirectionProposal(proposal)
	if err != nil {
		return ReviewReduction{}, err
	}
	reduction := ReviewReduction{
		Version: ReviewReductionVersion, Proposed: len(proposal.Directions),
	}
	directionIDs := make(map[string]struct{}, len(proposal.Directions))
	for _, direction := range proposal.Directions {
		directionIDs[direction.DirectionID] = struct{}{}
	}
	reviewsByDirection := make(map[string][]ReviewProposal, len(reviews))
	for _, review := range reviews {
		if !validOpaque(review.DirectionID) {
			reduction.Issues = append(reduction.Issues, ReviewIssue{Code: "review_direction_id_invalid"})
			continue
		}
		if _, ok := directionIDs[review.DirectionID]; !ok {
			reduction.Issues = append(reduction.Issues, ReviewIssue{
				DirectionID: review.DirectionID, Code: "review_direction_unknown",
			})
			continue
		}
		reviewsByDirection[review.DirectionID] = append(reviewsByDirection[review.DirectionID], review)
	}
	index := newBundleIndex(bundle)
	for proposalIndex, direction := range proposal.Directions {
		matched := reviewsByDirection[direction.DirectionID]
		if len(matched) == 0 {
			reduction.Issues = append(reduction.Issues, ReviewIssue{
				DirectionID: direction.DirectionID, Code: "review_missing",
			})
			continue
		}
		if len(matched) != 1 {
			reduction.Issues = append(reduction.Issues, ReviewIssue{
				DirectionID: direction.DirectionID, Code: "review_duplicate",
			})
			continue
		}
		if err := validateReviewProposal(matched[0]); err != nil {
			reduction.Issues = append(reduction.Issues, ReviewIssue{
				DirectionID: direction.DirectionID, Code: "review_malformed",
				Detail: boundedIssueDetail(err.Error()),
			})
			continue
		}
		reviewed, issue := applyDirectionReview(direction, matched[0], proposalIndex, index)
		if issue.Code != "" {
			reduction.Issues = append(reduction.Issues, issue)
			continue
		}
		reduction.Directions = append(reduction.Directions, reviewed)
	}
	reduction.Reviewed = len(reduction.Directions)
	return reduction, nil
}

// ReviewRoleDiversity counts distinct useful presentation roles.
func ReviewRoleDiversity(reviews []AnchorReview) int {
	roles := make(map[ReadingRole]struct{}, len(reviews))
	for _, review := range reviews {
		if review.Fit == AnchorFitIrrelevant || !validReadingRole(review.Role) {
			continue
		}
		roles[review.Role] = struct{}{}
	}
	return len(roles)
}

// ReviewQualityScore is a deterministic local ranking signal. It intentionally
// ignores model confidence and rewards source fit plus role diversity.
func ReviewQualityScore(reviews []AnchorReview) int {
	score := 0
	for _, review := range reviews {
		switch review.Fit {
		case AnchorFitDirect:
			score += 4
		case AnchorFitSupporting:
			score += 3
		case AnchorFitWeak:
			score++
		case AnchorFitIrrelevant:
			continue
		}
		if hasOverclaim(review.OverclaimReasons) {
			score--
		}
	}
	return score + 2*ReviewRoleDiversity(reviews)
}

func reviewQuestionFitScore(index bundleIndex, candidate Candidate, reviews []AnchorReview) int {
	terms := questionFitTerms(candidate.Question, index.repoName)
	if len(terms) == 0 {
		return 0
	}
	reviewByAnchor := make(map[string]AnchorReview, len(reviews))
	for _, review := range reviews {
		reviewByAnchor[review.AnchorID] = review
	}
	matched := 0
	for _, term := range terms {
		if questionFitTermMatched(index, candidate, reviewByAnchor, term) {
			matched++
		}
	}
	score := matched*4 - (len(terms)-matched)*2
	if matched == len(terms) {
		score += 4
	}
	return score
}

func questionFitTerms(question, repoName string) []string {
	stop := map[string]struct{}{
		"about": {}, "after": {}, "before": {}, "code": {}, "does": {}, "from": {},
		"have": {}, "into": {}, "learn": {}, "reader": {}, "repository": {}, "should": {},
		"system": {}, "that": {}, "their": {}, "these": {}, "this": {}, "through": {},
		"understand": {}, "what": {}, "when": {}, "where": {}, "which": {}, "with": {},
		"would": {},
	}
	for _, token := range questionFitTokens(repoName) {
		stop[token] = struct{}{}
	}
	seen := make(map[string]struct{})
	var terms []string
	for _, token := range questionFitTokens(question) {
		if len(token) < 4 {
			continue
		}
		if _, skip := stop[token]; skip {
			continue
		}
		term := questionFitCanonicalTerm(token)
		if len(term) < 4 {
			continue
		}
		if _, skip := stop[term]; skip {
			continue
		}
		if _, exists := seen[term]; exists {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) >= 8 {
			break
		}
	}
	return terms
}

func questionFitTermMatched(
	index bundleIndex,
	candidate Candidate,
	reviewByAnchor map[string]AnchorReview,
	term string,
) bool {
	for _, anchorID := range candidate.AnchorIDs {
		anchor, ok := index.anchors[anchorID]
		if !ok {
			continue
		}
		review := reviewByAnchor[anchorID]
		text := strings.Join([]string{
			anchor.Path,
			anchor.Symbol,
			questionFitAnchorSourceText(anchor),
			review.NarrowerDisplaySentence,
		}, "\n")
		if questionFitTextMatchesTerm(text, term) {
			return true
		}
		for _, areaID := range anchor.AreaIDs {
			if questionFitAreaMatches(index.areas[areaID], term) {
				return true
			}
		}
	}
	for _, reading := range candidate.ReadingAnchors {
		if questionFitTextMatchesTerm(reading.Label+"\n"+reading.WhatToLookFor, term) {
			return true
		}
	}
	for _, documentID := range candidate.DocumentIDs {
		document := index.documents[documentID]
		if questionFitTextMatchesTerm(document.Label+"\n"+document.Path+"\n"+document.Excerpt, term) {
			return true
		}
	}
	for _, areaID := range candidate.AreaIDs {
		if questionFitAreaMatches(index.areas[areaID], term) {
			return true
		}
	}
	if mechanism := index.mechanisms[candidate.MechanismID]; mechanism.ID != "" {
		return questionFitTextMatchesTerm(
			mechanism.Title+"\n"+mechanism.Question+"\n"+strings.Join(mechanism.Paths, "\n"),
			term,
		)
	}
	return false
}

func questionFitAnchorSourceText(anchor Anchor) string {
	fragment, err := fullReviewFragment(anchor)
	if err != nil {
		return ""
	}
	lines := make([]string, 0, len(fragment.lines))
	for _, line := range fragment.lines {
		lines = append(lines, line.Text)
	}
	return strings.Join(lines, "\n")
}

func questionFitAreaMatches(area Area, term string) bool {
	if area.ID == "" {
		return false
	}
	return questionFitTextMatchesTerm(area.Name+"\n"+area.Responsibility+"\n"+area.Path, term)
}

func questionFitTextMatchesTerm(text, term string) bool {
	tokens := questionFitTokenSet(text)
	for _, variant := range questionFitTermVariants(term) {
		if _, ok := tokens[variant]; ok {
			return true
		}
		if len(variant) < 5 {
			continue
		}
		for token := range tokens {
			if len(token) >= len(variant)+3 && strings.Contains(token, variant) {
				return true
			}
		}
	}
	return false
}

func questionFitTokenSet(text string) map[string]struct{} {
	tokens := questionFitTokens(text)
	result := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		result[token] = struct{}{}
		stem := questionFitStem(token)
		if stem != token {
			result[stem] = struct{}{}
		}
	}
	return result
}

func questionFitTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func questionFitTermVariants(term string) []string {
	variants := []string{term}
	stem := questionFitStem(term)
	if stem != term {
		variants = append(variants, stem)
	}
	if strings.HasSuffix(term, "e") && len(term) > 4 {
		variants = append(variants, strings.TrimSuffix(term, "e"))
	}
	if strings.HasSuffix(term, "ed") && len(term) > 4 {
		base := strings.TrimSuffix(term, "ed")
		variants = append(variants, base, base+"e")
	}
	if strings.HasSuffix(term, "y") && len(term) > 3 {
		variants = append(variants, strings.TrimSuffix(term, "y")+"ies")
	}
	if !strings.HasSuffix(term, "s") && len(term) > 3 {
		variants = append(variants, term+"s")
	}
	return variants
}

func questionFitCanonicalTerm(token string) string {
	switch {
	case len(token) > 5 && strings.HasSuffix(token, "ies"):
		return strings.TrimSuffix(token, "ies") + "y"
	case len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss"):
		return strings.TrimSuffix(token, "s")
	default:
		return token
	}
}

func questionFitStem(token string) string {
	switch {
	case len(token) > 5 && strings.HasSuffix(token, "ies"):
		return strings.TrimSuffix(token, "ies") + "y"
	case len(token) > 5 && strings.HasSuffix(token, "ing"):
		return strings.TrimSuffix(token, "ing")
	case len(token) > 4 && strings.HasSuffix(token, "ed") &&
		strings.HasSuffix(strings.TrimSuffix(token, "d"), "e"):
		return strings.TrimSuffix(token, "d")
	case len(token) > 4 && strings.HasSuffix(token, "ed"):
		return strings.TrimSuffix(token, "ed")
	case len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss"):
		return strings.TrimSuffix(token, "s")
	default:
		return token
	}
}

// CompressReviewedDirections selects three through six strong, distinct packs.
// It uses local fit/role scores and deterministic text/anchor overlap only.
// A reviewed first-contact pack owns one slot when present: losing the only
// onboarding question to several higher-scoring contributor topics makes the
// compact map less useful even though every individual pack is valid.
func CompressReviewedDirections(
	bundle Bundle,
	directions []ReviewedDirection,
) []ReviewedDirection {
	if len(directions) == 0 {
		return nil
	}
	index := newBundleIndex(canonicalBundle(bundle))
	remaining := append([]ReviewedDirection(nil), directions...)
	selected := make([]ReviewedDirection, 0, min(MaxReviewedDirections, len(remaining)))
	seenAreas := make(map[string]struct{})
	seenJobs := make(map[TargetJob]struct{})
	seenStages := make(map[LearningStage]struct{})
	for len(remaining) > 0 && len(selected) < MaxReviewedDirections {
		reserveFirstContact := len(selected) == 0 &&
			hasReviewedTargetJob(remaining, JobFirstContact)
		bestIndex := -1
		bestScore := -1 << 30
		for candidateIndex, direction := range remaining {
			if reserveFirstContact && direction.Candidate.TargetJob != JobFirstContact {
				continue
			}
			if semanticallyDuplicatesReviewed(direction, selected) {
				continue
			}
			score := direction.QualityScore*8 + direction.RoleDiversity*3 + direction.QuestionFitScore*2
			for _, anchorID := range direction.Candidate.AnchorIDs {
				if artifactrole.IsProduction(index.anchors[anchorID].Role) {
					score += 2
				}
			}
			if hasNewString(direction.Candidate.AreaIDs, seenAreas) {
				score += 6
			}
			if _, ok := seenJobs[direction.Candidate.TargetJob]; !ok {
				score += 3
			}
			if _, ok := seenStages[direction.Candidate.LearningStage]; !ok {
				score += 2
			}
			if len(direction.Candidate.DocumentIDs) > 0 {
				score++
			}
			if direction.Candidate.MechanismID != "" {
				score += 2
			}
			if bestIndex == -1 || score > bestScore ||
				score == bestScore && reviewedLess(direction, remaining[bestIndex]) {
				bestIndex, bestScore = candidateIndex, score
			}
		}
		if bestIndex == -1 {
			break
		}
		chosen := remaining[bestIndex]
		selected = append(selected, chosen)
		for _, areaID := range chosen.Candidate.AreaIDs {
			seenAreas[areaID] = struct{}{}
		}
		seenJobs[chosen.Candidate.TargetJob] = struct{}{}
		seenStages[chosen.Candidate.LearningStage] = struct{}{}
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left := stageRank(selected[i].Candidate.LearningStage)
		right := stageRank(selected[j].Candidate.LearningStage)
		if left != right {
			return left < right
		}
		return selected[i].proposalIndex < selected[j].proposalIndex
	})
	return selected
}

func hasReviewedTargetJob(directions []ReviewedDirection, job TargetJob) bool {
	for _, direction := range directions {
		if direction.Candidate.TargetJob == job {
			return true
		}
	}
	return false
}

// ComposeReviewedProposal reduces independently saved editor outputs into the
// existing v1 Proposal contract. Review diagnostics remain outside canonical
// Record semantics.
func ComposeReviewedProposal(
	bundle Bundle,
	brief BriefShapeProposal,
	directions DirectionProposal,
	reviews []ReviewProposal,
) (Proposal, ReviewReduction, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return Proposal{}, ReviewReduction{}, err
	}
	brief, err := validateBriefShapeAgainstBundle(brief, bundle)
	if err != nil {
		return Proposal{}, ReviewReduction{}, err
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		return Proposal{}, reduction, err
	}
	selected := CompressReviewedDirections(bundle, reduction.Directions)
	reduction.Selected = len(selected)
	if len(selected) < MinReviewedDirections {
		return Proposal{}, reduction, fmt.Errorf(
			"study map: reviewed selection has %d directions; need at least %d",
			len(selected), MinReviewedDirections,
		)
	}
	proposal := Proposal{
		Version: ProposalVersion, RepositoryType: brief.RepositoryType,
		Brief: brief.Brief, ShapeAreaIDs: append([]string(nil), brief.ShapeAreaIDs...),
		Candidates: make([]Candidate, 0, len(selected)),
	}
	for _, direction := range selected {
		candidate := direction.Candidate
		candidate.Confidence = ""
		proposal.Candidates = append(proposal.Candidates, candidate)
	}
	return proposal, reduction, nil
}

// BuildReviewedRecord preserves the canonical v1 Record contract by passing
// the locally composed v1 Proposal through the existing reducer.
func BuildReviewedRecord(
	bundle Bundle,
	brief BriefShapeProposal,
	directions DirectionProposal,
	reviews []ReviewProposal,
) (Record, ReviewReduction, error) {
	proposal, reduction, err := ComposeReviewedProposal(bundle, brief, directions, reviews)
	if err != nil {
		return Record{}, reduction, err
	}
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		return Record{}, reduction, err
	}
	return record, reduction, nil
}

func (direction DirectionCandidate) candidate() Candidate {
	return Candidate{
		Question: direction.Question, WhyItMatters: direction.WhyItMatters,
		LearningOutcome: direction.LearningOutcome, TargetJob: direction.TargetJob,
		LearningStage:  direction.LearningStage,
		AnchorIDs:      append([]string(nil), direction.AnchorIDs...),
		DocumentIDs:    append([]string(nil), direction.DocumentIDs...),
		AreaIDs:        append([]string(nil), direction.AreaIDs...),
		MechanismID:    direction.MechanismID,
		ReadingAnchors: append([]ReadingAnchor(nil), direction.ReadingAnchors...),
		SearchQueries:  append([]string(nil), direction.SearchQueries...),
	}
}

func validateBriefShapeStructure(proposal BriefShapeProposal) error {
	if proposal.Version != BriefShapeProposalVersion {
		return fmt.Errorf("study map: unsupported brief and shape proposal version %d", proposal.Version)
	}
	if !validRepositoryType(proposal.RepositoryType) || !completeBrief(proposal.Brief) {
		return fmt.Errorf("study map: invalid brief and shape proposal")
	}
	if len(proposal.ShapeAreaIDs) < 1 || len(proposal.ShapeAreaIDs) > 7 ||
		len(uniqueStrings(proposal.ShapeAreaIDs)) != len(proposal.ShapeAreaIDs) {
		return fmt.Errorf("study map: shape must contain one to seven unique areas")
	}
	for _, areaID := range proposal.ShapeAreaIDs {
		if !validOpaque(areaID) {
			return fmt.Errorf("study map: invalid shape area id")
		}
	}
	if len(proposal.Brief.DomainTerms) > 8 {
		return fmt.Errorf("study map: too many brief domain terms")
	}
	for _, term := range proposal.Brief.DomainTerms {
		if !validText(term.Term, 128, true) || !validText(term.Meaning, 512, true) ||
			len(term.SupportIDs) == 0 || !allOpaque(term.SupportIDs) {
			return fmt.Errorf("study map: invalid brief domain term")
		}
	}
	for _, statement := range []BriefStatement{
		proposal.Brief.WhatItIs, proposal.Brief.Problem, proposal.Brief.MainInput,
		proposal.Brief.CentralResponsibility, proposal.Brief.ObservableResult,
	} {
		if !allOpaque(statement.SupportIDs) {
			return fmt.Errorf("study map: invalid brief support id")
		}
	}
	return nil
}

func validateBriefShapeAgainstBundle(
	proposal BriefShapeProposal,
	bundle Bundle,
) (BriefShapeProposal, error) {
	if err := validateBriefShapeStructure(proposal); err != nil {
		return BriefShapeProposal{}, err
	}
	index := newBundleIndex(bundle)
	reduction := Reduction{}
	normalized := validateBrief(proposal.Brief, index, &reduction)
	// Support IDs are an unordered evidence set. The canonical reducer sorts
	// and de-duplicates them, so editor order must not turn otherwise identical
	// support into an "unsupported objects" failure.
	comparable := proposal.Brief
	normalizeBriefSupportIDs(&comparable)
	// Required statements remain fail-closed. Domain terms are optional: the
	// canonical reducer may drop only the unsupported terms while preserving
	// the supported brief and shape.
	comparable.DomainTerms = normalized.DomainTerms
	if len(reduction.Issues) > 0 || !briefEqual(comparable, normalized) || !completeBrief(normalized) {
		return BriefShapeProposal{}, fmt.Errorf("study map: brief references unsupported objects")
	}
	proposal.Brief = normalized
	for _, areaID := range proposal.ShapeAreaIDs {
		if _, ok := index.areas[areaID]; !ok {
			return BriefShapeProposal{}, fmt.Errorf("study map: shape references unknown area %q", areaID)
		}
	}
	return proposal, nil
}

func normalizeBriefSupportIDs(brief *Brief) {
	statements := []*BriefStatement{
		&brief.WhatItIs,
		&brief.Problem,
		&brief.MainInput,
		&brief.CentralResponsibility,
		&brief.ObservableResult,
	}
	for _, statement := range statements {
		statement.SupportIDs = uniqueStrings(statement.SupportIDs)
	}
	for index := range brief.DomainTerms {
		brief.DomainTerms[index].SupportIDs = uniqueStrings(brief.DomainTerms[index].SupportIDs)
	}
}

func validateDirectionProposalStructure(proposal DirectionProposal) error {
	if err := validateDirectionProposalBounds(proposal); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(proposal.Directions))
	for _, direction := range proposal.Directions {
		if err := validateDirectionCandidateStructure(direction); err != nil {
			return err
		}
		if _, duplicate := seen[direction.DirectionID]; duplicate {
			return fmt.Errorf("study map: duplicate direction id %q", direction.DirectionID)
		}
		seen[direction.DirectionID] = struct{}{}
	}
	return nil
}

func validateDirectionProposalBounds(proposal DirectionProposal) error {
	if proposal.Version != DirectionProposalVersion {
		return fmt.Errorf("study map: unsupported direction proposal version %d", proposal.Version)
	}
	if len(proposal.Directions) == 0 || len(proposal.Directions) > MaxCandidates {
		return fmt.Errorf("study map: direction count must be between 1 and %d", MaxCandidates)
	}
	return nil
}

func validateDirectionCandidateStructure(direction DirectionCandidate) error {
	if !validOpaque(direction.DirectionID) || direction.DirectionID != localDirectionID(direction) {
		return fmt.Errorf("study map: direction id is not locally derived")
	}
	return validateDirectionCandidateContent(direction)
}

func validateDirectionCandidateContent(direction DirectionCandidate) error {
	if !naturalQuestion(direction.Question) ||
		!validText(direction.WhyItMatters, 1024, true) || impliesRuntimeOrder(direction.WhyItMatters) ||
		!validText(direction.LearningOutcome, 1024, true) || impliesRuntimeOrder(direction.LearningOutcome) ||
		!validTargetJob(direction.TargetJob) || !validLearningStage(direction.LearningStage) {
		return newDirectionCandidateValidationError(
			"invalid_candidate",
			"study map: invalid direction candidate %q",
			direction.DirectionID,
		)
	}
	if len(direction.AnchorIDs) < 3 || len(direction.AnchorIDs) > 5 ||
		len(uniqueStrings(direction.AnchorIDs)) != len(direction.AnchorIDs) || !allOpaque(direction.AnchorIDs) {
		return newDirectionCandidateValidationError(
			"invalid_anchor_selection",
			"study map: direction %q must select three to five unique anchors",
			direction.DirectionID,
		)
	}
	if len(direction.ReadingAnchors) != len(direction.AnchorIDs) {
		return newDirectionCandidateValidationError(
			"invalid_reading_anchor_count",
			"study map: direction %q must describe every selected anchor",
			direction.DirectionID,
		)
	}
	readingIDs := make([]string, 0, len(direction.ReadingAnchors))
	for _, reading := range direction.ReadingAnchors {
		if !validOpaque(reading.AnchorID) || !validReadingLabel(reading.Label) ||
			!validText(reading.WhatToLookFor, 768, true) || impliesRuntimeOrder(reading.WhatToLookFor) {
			return newDirectionCandidateValidationError(
				"invalid_reading_copy",
				"study map: direction %q has invalid reading copy",
				direction.DirectionID,
			)
		}
		readingIDs = append(readingIDs, reading.AnchorID)
	}
	if !slices.Equal(uniqueStrings(readingIDs), uniqueStrings(direction.AnchorIDs)) {
		return newDirectionCandidateValidationError(
			"reading_anchor_mismatch",
			"study map: direction %q reading anchors do not match selected anchors",
			direction.DirectionID,
		)
	}
	if !uniqueOpaque(direction.DocumentIDs) || !uniqueOpaque(direction.AreaIDs) ||
		(direction.MechanismID != "" && !validOpaque(direction.MechanismID)) {
		return newDirectionCandidateValidationError(
			"invalid_object_ids",
			"study map: direction %q contains invalid object ids",
			direction.DirectionID,
		)
	}
	if len(direction.SearchQueries) > 8 {
		return newDirectionCandidateValidationError(
			"too_many_search_queries",
			"study map: direction %q contains too many search queries",
			direction.DirectionID,
		)
	}
	for _, query := range direction.SearchQueries {
		if !validText(query, 256, true) {
			return newDirectionCandidateValidationError(
				"invalid_search_query",
				"study map: direction %q contains invalid search query",
				direction.DirectionID,
			)
		}
	}
	return nil
}

type directionCandidateValidationError struct {
	code    string
	message string
}

func (err *directionCandidateValidationError) Error() string {
	return err.message
}

func newDirectionCandidateValidationError(
	code string,
	format string,
	args ...any,
) error {
	return &directionCandidateValidationError{
		code:    code,
		message: fmt.Sprintf(format, args...),
	}
}

func directionCandidateValidationCode(err error) string {
	validationErr, ok := err.(*directionCandidateValidationError)
	if !ok || validationErr.code == "" {
		return "invalid_candidate"
	}
	return validationErr.code
}

func normalizeDirectionCandidate(direction DirectionCandidate) (DirectionCandidate, error) {
	if err := validateDirectionCandidateContent(direction); err != nil {
		return DirectionCandidate{}, err
	}
	derivedID := localDirectionID(direction)
	if direction.DirectionID != "" && direction.DirectionID != derivedID {
		return DirectionCandidate{}, fmt.Errorf("study map: direction id is not locally derived")
	}
	direction.DirectionID = derivedID
	return direction, nil
}

func localDirectionID(direction DirectionCandidate) string {
	question := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(direction.Question))), " ")
	return stableID("study-direction", question, strings.Join(uniqueStrings(direction.AnchorIDs), "\x00"))
}

func validateReviewProposal(proposal ReviewProposal) error {
	if proposal.Version != ReviewProposalVersion {
		return fmt.Errorf("study map: unsupported review proposal version %d", proposal.Version)
	}
	if !validOpaque(proposal.DirectionID) || len(proposal.Reviews) < 3 || len(proposal.Reviews) > 5 {
		return fmt.Errorf("study map: invalid review proposal")
	}
	seen := make(map[string]struct{}, len(proposal.Reviews))
	for _, review := range proposal.Reviews {
		if !validOpaque(review.AnchorID) || !validAnchorFit(review.Fit) ||
			!validReadingRole(review.Role) ||
			!validText(review.SupportedObservation, 768, true) ||
			impliesRuntimeOrder(review.SupportedObservation) ||
			!validText(review.NarrowerDisplaySentence, 768, false) ||
			impliesRuntimeOrder(review.NarrowerDisplaySentence) ||
			!validOverclaimReasons(review.OverclaimReasons) {
			return fmt.Errorf("study map: malformed review for anchor %q", review.AnchorID)
		}
		if _, duplicate := seen[review.AnchorID]; duplicate {
			return fmt.Errorf("study map: duplicate anchor review %q", review.AnchorID)
		}
		seen[review.AnchorID] = struct{}{}
	}
	return nil
}

func applyDirectionReview(
	direction DirectionCandidate,
	proposal ReviewProposal,
	proposalIndex int,
	index bundleIndex,
) (ReviewedDirection, ReviewIssue) {
	issue := func(code string) (ReviewedDirection, ReviewIssue) {
		return ReviewedDirection{}, ReviewIssue{DirectionID: direction.DirectionID, Code: code}
	}
	if proposal.DirectionID != direction.DirectionID {
		return issue("review_direction_mismatch")
	}
	if _, candidateIssues := validateCandidate(direction.candidate(), proposalIndex, index); len(candidateIssues) > 0 {
		return issue("direction_invalid_before_review")
	}
	reviewByAnchor := make(map[string]AnchorReview, len(proposal.Reviews))
	selected := make(map[string]struct{}, len(direction.AnchorIDs))
	for _, anchorID := range direction.AnchorIDs {
		selected[anchorID] = struct{}{}
	}
	for _, review := range proposal.Reviews {
		if _, ok := selected[review.AnchorID]; !ok {
			return issue("review_anchor_unknown")
		}
		reviewByAnchor[review.AnchorID] = review
	}
	if len(reviewByAnchor) != len(selected) {
		return issue("review_anchor_missing")
	}
	retainedIDs := make([]string, 0, len(direction.AnchorIDs))
	retainedReading := make([]ReadingAnchor, 0, len(direction.ReadingAnchors))
	retainedReviews := make([]AnchorReview, 0, len(proposal.Reviews))
	directOrSupporting := 0
	productionOrOperational := false
	questionBroad := 0
	outcomeBroad := 0
	unsupportedOrder := 0
	// ReadingAnchors is the model's editorial reading order. AnchorIDs is an
	// unordered identity set and may be canonicalized later, so it must not
	// drive the visible Start-here sequence.
	for _, reading := range direction.ReadingAnchors {
		anchorID := reading.AnchorID
		review := reviewByAnchor[anchorID]
		if review.Fit == AnchorFitIrrelevant {
			continue
		}
		retainedIDs = append(retainedIDs, anchorID)
		retainedReviews = append(retainedReviews, review)
		replacement := strings.TrimSpace(review.NarrowerDisplaySentence)
		if replacement == "" && (review.Fit == AnchorFitWeak || hasOverclaim(review.OverclaimReasons)) {
			replacement = strings.TrimSpace(review.SupportedObservation)
		}
		if replacement != "" {
			reading.WhatToLookFor = replacement
		}
		retainedReading = append(retainedReading, reading)
		if review.Fit == AnchorFitDirect || review.Fit == AnchorFitSupporting {
			directOrSupporting++
			anchor := index.anchors[anchorID]
			productionOrOperational = productionOrOperational ||
				artifactrole.IsProduction(anchor.Role) ||
				anchor.Role == artifactrole.RoleCurrentDocumentation &&
					review.Role == ReadingRoleConfigurationOrOperations
		}
		if slices.Contains(review.OverclaimReasons, OverclaimQuestionScopeBroader) {
			questionBroad++
		}
		if slices.Contains(review.OverclaimReasons, OverclaimLearningOutcomeScopeBroader) {
			outcomeBroad++
		}
		if slices.Contains(review.OverclaimReasons, OverclaimUnsupportedRuntimeOrder) {
			unsupportedOrder++
		}
	}
	if directOrSupporting < 3 {
		return issue("fewer_than_three_direct_or_supporting_anchors")
	}
	if !productionOrOperational {
		return issue("production_or_operational_anchor_missing")
	}
	if questionBroad*2 > len(retainedReviews) {
		return issue("question_scope_broader")
	}
	if outcomeBroad*2 > len(retainedReviews) {
		return issue("learning_outcome_scope_broader")
	}
	if direction.MechanismID == "" && unsupportedOrder > 0 &&
		asksForRuntimeOrder(direction.Question+" "+direction.LearningOutcome) {
		return issue("unsupported_runtime_order")
	}
	candidate := direction.candidate()
	candidate.AnchorIDs = retainedIDs
	candidate.ReadingAnchors = retainedReading
	candidate.Confidence = ""
	if _, issues := validateCandidate(candidate, proposalIndex, index); len(issues) > 0 {
		return issue("direction_invalid_after_review")
	}
	return ReviewedDirection{
		DirectionID: direction.DirectionID, Candidate: candidate,
		Reviews: retainedReviews, QualityScore: ReviewQualityScore(retainedReviews),
		RoleDiversity:    ReviewRoleDiversity(retainedReviews),
		QuestionFitScore: reviewQuestionFitScore(index, candidate, retainedReviews),
		proposalIndex:    proposalIndex,
	}, ReviewIssue{}
}

func asksForRuntimeOrder(value string) bool {
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	}) {
		if term == "order" || term == "ordering" || term == "sequence" || term == "sequencing" {
			return true
		}
	}
	return false
}

type reviewFragment struct {
	anchorID   string
	anchorLine int
	lines      []ReviewSourceLine
}

func fullReviewFragment(anchor Anchor) (reviewFragment, error) {
	startLine, lines, err := anchor.sourceLines()
	if err != nil {
		return reviewFragment{}, fmt.Errorf("study map: invalid source for review anchor %q", anchor.ID)
	}
	center := anchor.Line - startLine
	if center < 0 || center >= len(lines) {
		return reviewFragment{}, fmt.Errorf("study map: review anchor line is outside source")
	}
	start := max(0, center-maxReviewSourceLines/2)
	end := min(len(lines), start+maxReviewSourceLines)
	start = max(0, end-maxReviewSourceLines)
	fragment := reviewFragment{anchorID: anchor.ID, anchorLine: anchor.Line}
	for index := start; index < end; index++ {
		fragment.lines = append(fragment.lines, ReviewSourceLine{
			Line: startLine + index, Text: lines[index],
		})
	}
	return fragment, nil
}

func shrinkReviewFragments(fragments []reviewFragment) error {
	total := 0
	for _, fragment := range fragments {
		for _, line := range fragment.lines {
			total += reviewSourceLineBytes(line)
		}
	}
	for total > maxReviewSourceBytes {
		fragmentIndex, fromStart := removableReviewLine(fragments)
		if fragmentIndex < 0 {
			return fmt.Errorf("study map: exact review anchor lines exceed %d bytes", maxReviewSourceBytes)
		}
		fragment := &fragments[fragmentIndex]
		if fromStart {
			total -= reviewSourceLineBytes(fragment.lines[0])
			fragment.lines = fragment.lines[1:]
		} else {
			last := len(fragment.lines) - 1
			total -= reviewSourceLineBytes(fragment.lines[last])
			fragment.lines = fragment.lines[:last]
		}
	}
	return nil
}

func removableReviewLine(fragments []reviewFragment) (int, bool) {
	bestFragment := -1
	bestFromStart := false
	bestDistance := -1
	bestBytes := -1
	for index := range fragments {
		fragment := fragments[index]
		if len(fragment.lines) <= 1 {
			continue
		}
		for _, fromStart := range []bool{true, false} {
			lineIndex := len(fragment.lines) - 1
			if fromStart {
				lineIndex = 0
			}
			line := fragment.lines[lineIndex]
			if line.Line == fragment.anchorLine {
				continue
			}
			distance := line.Line - fragment.anchorLine
			if distance < 0 {
				distance = -distance
			}
			lineBytes := reviewSourceLineBytes(line)
			if bestFragment == -1 || distance > bestDistance ||
				distance == bestDistance && lineBytes > bestBytes {
				bestFragment, bestFromStart = index, fromStart
				bestDistance, bestBytes = distance, lineBytes
			}
		}
	}
	return bestFragment, bestFromStart
}

func reviewSourceLineBytes(line ReviewSourceLine) int {
	return len(strconv.Itoa(line.Line)) + 2 + len(line.Text) + 1
}

func validAnchorFit(fit AnchorFit) bool {
	switch fit {
	case AnchorFitDirect, AnchorFitSupporting, AnchorFitWeak, AnchorFitIrrelevant:
		return true
	default:
		return false
	}
}

func validReadingRole(role ReadingRole) bool {
	switch role {
	case ReadingRoleDocumentationIntent, ReadingRolePublicOrCLIEntry,
		ReadingRoleCoreOrchestration, ReadingRoleStateOrDataModel,
		ReadingRoleEffectOrIntegrationBoundary, ReadingRoleRepresentativeImplementation,
		ReadingRoleConfigurationOrOperations, ReadingRoleExampleOrUsage,
		ReadingRoleTestOrVerification:
		return true
	default:
		return false
	}
}

func validOverclaimReasons(reasons []OverclaimReason) bool {
	if len(reasons) == 0 || len(reasons) > 8 {
		return false
	}
	seen := make(map[OverclaimReason]struct{}, len(reasons))
	for _, reason := range reasons {
		switch reason {
		case OverclaimNone, OverclaimWrongResponsibility, OverclaimBehaviorOutsideWindow,
			OverclaimUnsupportedRuntimeOrder, OverclaimUnsupportedCausality,
			OverclaimQuestionScopeBroader, OverclaimLearningOutcomeScopeBroader,
			OverclaimVagueOrGeneric:
		default:
			return false
		}
		if _, duplicate := seen[reason]; duplicate {
			return false
		}
		seen[reason] = struct{}{}
	}
	_, none := seen[OverclaimNone]
	return !none || len(seen) == 1
}

func hasOverclaim(reasons []OverclaimReason) bool {
	for _, reason := range reasons {
		if reason != OverclaimNone {
			return true
		}
	}
	return false
}

func semanticallyDuplicatesReviewed(candidate ReviewedDirection, selected []ReviewedDirection) bool {
	for _, other := range selected {
		if candidate.Candidate.MechanismID != "" &&
			candidate.Candidate.MechanismID == other.Candidate.MechanismID {
			return true
		}
		if stringSetJaccard(candidate.Candidate.AnchorIDs, other.Candidate.AnchorIDs) >= 0.6 {
			return true
		}
		questionOverlap := termSetJaccard(
			semanticTerms(candidate.Candidate.Question),
			semanticTerms(other.Candidate.Question),
		)
		outcomeOverlap := termSetJaccard(
			semanticTerms(candidate.Candidate.LearningOutcome),
			semanticTerms(other.Candidate.LearningOutcome),
		)
		combinedOverlap := termSetJaccard(
			semanticTerms(candidate.Candidate.Question+" "+candidate.Candidate.LearningOutcome),
			semanticTerms(other.Candidate.Question+" "+other.Candidate.LearningOutcome),
		)
		if combinedOverlap >= 0.7 || questionOverlap >= 0.62 && outcomeOverlap >= 0.5 {
			return true
		}
	}
	return false
}

func semanticTerms(value string) map[string]struct{} {
	stop := map[string]struct{}{
		"about": {}, "after": {}, "before": {}, "code": {}, "does": {}, "from": {},
		"have": {}, "into": {}, "learn": {}, "reader": {}, "repository": {}, "should": {},
		"system": {}, "that": {}, "their": {}, "these": {}, "this": {}, "through": {},
		"understand": {}, "what": {}, "when": {}, "where": {}, "which": {}, "with": {},
	}
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	}) {
		if len(token) < 4 {
			continue
		}
		if _, skip := stop[token]; !skip {
			result[token] = struct{}{}
		}
	}
	return result
}

func termSetJaccard(left, right map[string]struct{}) float64 {
	intersection := 0
	for term := range left {
		if _, ok := right[term]; ok {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func reviewedLess(left, right ReviewedDirection) bool {
	if left.proposalIndex != right.proposalIndex {
		return left.proposalIndex < right.proposalIndex
	}
	return left.DirectionID < right.DirectionID
}

func uniqueOpaque(values []string) bool {
	if len(uniqueStrings(values)) != len(values) {
		return false
	}
	return allOpaque(values)
}

func allOpaque(values []string) bool {
	for _, value := range values {
		if !validOpaque(value) {
			return false
		}
	}
	return true
}

func boundedIssueDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 512 {
		return value
	}
	return value[:512]
}

func decodeEditingJSON(raw []byte, limit int, label string, target any) error {
	if len(raw) == 0 || len(raw) > limit {
		return fmt.Errorf("study map: %s is outside bounds", label)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("study map: decode %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("study map: trailing JSON in %s", label)
		}
		return fmt.Errorf("study map: invalid trailing JSON in %s: %w", label, err)
	}
	return nil
}

func recoverEditingProviderJSON(
	raw []byte,
	label string,
	validate func([]byte) error,
) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxEditingArtifactBytes {
		return nil, fmt.Errorf("study map: %s is outside bounds", label)
	}
	trimmed := bytes.TrimSpace(raw)
	if validate(trimmed) == nil {
		return append([]byte(nil), trimmed...), nil
	}

	var accepted []byte
	starts := 0
	for cursor := 0; cursor < len(raw) && starts < maxProviderEnvelopeCandidates; {
		offset := bytes.IndexByte(raw[cursor:], '{')
		if offset < 0 {
			break
		}
		start := cursor + offset
		starts++
		decoder := json.NewDecoder(bytes.NewReader(raw[start:]))
		var candidate json.RawMessage
		if err := decoder.Decode(&candidate); err != nil ||
			len(candidate) == 0 || candidate[0] != '{' {
			cursor = start + 1
			continue
		}
		if validate(candidate) == nil {
			if accepted != nil {
				return nil, fmt.Errorf(
					"study map: ambiguous recoverable %s",
					label,
				)
			}
			accepted = append([]byte(nil), candidate...)
		}
		consumed := int(decoder.InputOffset())
		if consumed <= 0 {
			consumed = 1
		}
		cursor = start + consumed
	}
	if accepted == nil {
		return nil, fmt.Errorf("study map: no recoverable %s", label)
	}
	return accepted, nil
}
