// Package studymap defines the bounded, replayable editorial contract used to
// turn existing repository facts into a source-backed reading guide. A Study
// Direction is navigation, not a runtime claim or a canonical Mechanism.
package studymap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

const (
	BundleVersion         = 1
	ProposalVersion       = 1
	RecordVersion         = 1
	PromptVersion         = "repository-study-map-json-v1"
	RecordFile            = "repository_study_map.json"
	BundleFile            = "repository_study_map_bundle.json"
	AttemptFile           = "repository_study_map_attempt.json"
	DirectionsAttemptFile = "study_direction_candidates_attempt.json"
	StatusFile            = "repository_study_map_status.json"

	MaxCandidates = 12
	MinDirections = 1
	// Every provider candidate is already bounded and independently reviewed.
	// Keep one publication bound so a second, smaller editorial cap cannot hide
	// valid reading packs after that work has completed.
	MaxDirections = MaxCandidates
	MaxAnchors    = 32
	MaxAreas      = 12
	MaxDocuments  = 12
	MaxMechanisms = 12

	maxRecordBytes = 4 << 20

	maxExactSourceLines     = 512
	maxExactSourceLineBytes = 64 << 10
	maxExactSourceBytes     = 1 << 20
)

type RepositoryType string

const (
	RepositoryService  RepositoryType = "service_application"
	RepositoryLibrary  RepositoryType = "library_framework"
	RepositoryCLI      RepositoryType = "cli_tool"
	RepositoryMonorepo RepositoryType = "monorepo"
	RepositoryMixed    RepositoryType = "mixed"
)

type LearningStage string

const (
	StageOrientation      LearningStage = "orientation"
	StageCentralOperation LearningStage = "central_operation"
	StageCoreModel        LearningStage = "core_model"
	StageIntegration      LearningStage = "integration"
	StageOperations       LearningStage = "operations"
	StageContribution     LearningStage = "contribution"
)

type TargetJob string

const (
	JobFirstContact TargetJob = "first_contact"
	JobUseOperate   TargetJob = "use_or_operate"
	JobIntegrate    TargetJob = "extend_or_integrate"
	JobContribute   TargetJob = "contribute"
	JobMaintain     TargetJob = "debug_or_maintain"
)

// Bundle contains only bounded, already-known repository objects. Function
// bodies are retained for local source projection; PromptBundle deliberately
// removes them before a provider call.
type Bundle struct {
	Version            int            `json:"version"`
	RepoName           string         `json:"repo_name"`
	DocumentedPurpose  string         `json:"documented_purpose,omitempty"`
	OrientationSummary string         `json:"orientation_summary,omitempty"`
	RepositoryTypeHint RepositoryType `json:"repository_type_hint,omitempty"`
	DomainTerms        []DomainTerm   `json:"domain_terms,omitempty"`
	Areas              []Area         `json:"areas"`
	Anchors            []Anchor       `json:"anchors"`
	Documents          []Document     `json:"documents,omitempty"`
	Mechanisms         []Mechanism    `json:"mechanisms,omitempty"`
	AllowedPaths       []string       `json:"allowed_paths"`
}

type DomainTerm struct {
	Term        string `json:"term"`
	Explanation string `json:"explanation"`
}

type Area struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	ComponentID    string `json:"component_id,omitempty"`
	Path           string `json:"path,omitempty"`
	Line           int    `json:"line,omitempty"`
}

type Anchor struct {
	ID           string                         `json:"id"`
	Path         string                         `json:"path"`
	Symbol       string                         `json:"symbol"`
	Line         int                            `json:"line"`
	Role         artifactrole.Role              `json:"role"`
	Statement    string                         `json:"statement"`
	Capabilities []semanticdiscovery.Capability `json:"capabilities,omitempty"`
	AreaIDs      []string                       `json:"area_ids,omitempty"`
	Function     sourcewindowfacts.Function     `json:"function"`
	ExactSource  *ExactSource                   `json:"exact_source,omitempty"`
}

// ExactSource is a bounded, hash-verified declaration window for a non-Go
// language. It is deliberately weaker than sourcewindowfacts.Function: the
// saved lines provide an exact reading anchor but make no syntax or behavior
// claim. Anchor selects exactly one of Function or ExactSource.
type ExactSource struct {
	Path          string   `json:"path"`
	Language      string   `json:"language"`
	Symbol        string   `json:"symbol"`
	Line          int      `json:"line"`
	StartLine     int      `json:"start_line"`
	EndLine       int      `json:"end_line"`
	Lines         []string `json:"lines"`
	ContentSHA256 string   `json:"content_sha256"`
}

type Document struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Label   string `json:"label"`
	Excerpt string `json:"excerpt,omitempty"`
}

type Mechanism struct {
	ID        string   `json:"id"`
	Question  string   `json:"question"`
	Title     string   `json:"title"`
	AnchorIDs []string `json:"anchor_ids,omitempty"`
	Paths     []string `json:"paths,omitempty"`
}

// PromptBundle is the model-visible projection. It preserves exact IDs and
// short fact text while withholding full source bodies.
type PromptBundle struct {
	Version            int            `json:"version"`
	RepoName           string         `json:"repo_name"`
	DocumentedPurpose  string         `json:"documented_purpose,omitempty"`
	OrientationSummary string         `json:"orientation_summary,omitempty"`
	RepositoryTypeHint RepositoryType `json:"repository_type_hint,omitempty"`
	DomainTerms        []DomainTerm   `json:"domain_terms,omitempty"`
	Areas              []Area         `json:"areas"`
	Anchors            []PromptAnchor `json:"code_anchors"`
	Documents          []Document     `json:"documents,omitempty"`
	Mechanisms         []Mechanism    `json:"canonical_mechanisms,omitempty"`
}

type PromptAnchor struct {
	ID           string                         `json:"id"`
	Path         string                         `json:"path"`
	Symbol       string                         `json:"symbol"`
	Line         int                            `json:"line"`
	Role         artifactrole.Role              `json:"role"`
	Statement    string                         `json:"bounded_fact"`
	Capabilities []semanticdiscovery.Capability `json:"capabilities,omitempty"`
	AreaIDs      []string                       `json:"area_ids,omitempty"`
}

type Proposal struct {
	Version        int            `json:"version"`
	RepositoryType RepositoryType `json:"repository_type"`
	Brief          Brief          `json:"brief"`
	ShapeAreaIDs   []string       `json:"shape_area_ids"`
	Candidates     []Candidate    `json:"candidates"`
}

type Brief struct {
	WhatItIs              BriefStatement    `json:"what_it_is"`
	Problem               BriefStatement    `json:"problem"`
	MainInput             BriefStatement    `json:"main_input"`
	CentralResponsibility BriefStatement    `json:"central_responsibility"`
	ObservableResult      BriefStatement    `json:"observable_result"`
	DomainTerms           []BriefDomainTerm `json:"domain_terms,omitempty"`
}

type BriefStatement struct {
	Text       string   `json:"text"`
	SupportIDs []string `json:"support_ids"`
}

type BriefDomainTerm struct {
	Term       string   `json:"term"`
	Meaning    string   `json:"meaning"`
	SupportIDs []string `json:"support_ids"`
}

type Candidate struct {
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
	Confidence      string          `json:"confidence"`
	SearchQueries   []string        `json:"search_queries,omitempty"`
}

type ReadingAnchor struct {
	AnchorID      string `json:"anchor_id"`
	Label         string `json:"label"`
	WhatToLookFor string `json:"what_to_look_for"`
}

type Direction struct {
	ID              string          `json:"id"`
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
	proposalIndex   int
	score           int
}

type ReductionIssue struct {
	CandidateIndex int    `json:"candidate_index"`
	Code           string `json:"code"`
	Detail         string `json:"detail,omitempty"`
}

type Reduction struct {
	Proposed  int              `json:"proposed"`
	Validated int              `json:"validated"`
	Selected  int              `json:"selected"`
	Issues    []ReductionIssue `json:"issues,omitempty"`
}

type Record struct {
	Version        int            `json:"version"`
	BundleSHA256   string         `json:"bundle_sha256"`
	Bundle         Bundle         `json:"bundle"`
	RepositoryType RepositoryType `json:"repository_type"`
	Brief          Brief          `json:"brief"`
	ShapeAreaIDs   []string       `json:"shape_area_ids"`
	Directions     []Direction    `json:"directions"`
	Reduction      Reduction      `json:"reduction"`
}

func (bundle Bundle) PromptBundle() PromptBundle {
	prompt := PromptBundle{
		Version: bundle.Version, RepoName: bundle.RepoName,
		DocumentedPurpose:  bundle.DocumentedPurpose,
		OrientationSummary: bundle.OrientationSummary,
		RepositoryTypeHint: bundle.RepositoryTypeHint,
		DomainTerms:        append([]DomainTerm(nil), bundle.DomainTerms...),
		Areas:              append([]Area(nil), bundle.Areas...),
		Documents:          append([]Document(nil), bundle.Documents...),
		Mechanisms:         append([]Mechanism(nil), bundle.Mechanisms...),
	}
	for _, anchor := range bundle.Anchors {
		prompt.Anchors = append(prompt.Anchors, PromptAnchor{
			ID: anchor.ID, Path: anchor.Path, Symbol: anchor.Symbol,
			Line: anchor.Line, Role: anchor.Role, Statement: anchor.Statement,
			Capabilities: append([]semanticdiscovery.Capability(nil), anchor.Capabilities...),
			AreaIDs:      append([]string(nil), anchor.AreaIDs...),
		})
	}
	return prompt
}

func DecodeProposal(raw []byte) (Proposal, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return Proposal{}, fmt.Errorf("study map: proposal is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, fmt.Errorf("study map: decode proposal: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Proposal{}, err
	}
	if proposal.Version != ProposalVersion {
		return Proposal{}, fmt.Errorf("study map: unsupported proposal version %d", proposal.Version)
	}
	if len(proposal.Candidates) == 0 || len(proposal.Candidates) > MaxCandidates {
		return Proposal{}, fmt.Errorf("study map: candidate count must be between 1 and %d", MaxCandidates)
	}
	return proposal, nil
}

func DecodeRecord(raw []byte) (Record, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return Record{}, fmt.Errorf("study map: record is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("study map: decode record: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Record{}, err
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// DecodeBundle decodes one saved local Study bundle. Unlike PromptBundle, this
// artifact may contain bounded source functions used for local presentation.
func DecodeBundle(raw []byte) (Bundle, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return Bundle{}, fmt.Errorf("study map: bundle is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var bundle Bundle
	if err := decoder.Decode(&bundle); err != nil {
		return Bundle{}, fmt.Errorf("study map: decode bundle: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Bundle{}, err
	}
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func BundleHash(bundle Bundle) (string, error) {
	normalized := canonicalBundle(bundle)
	if err := normalized.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("study map: marshal bundle: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func BuildRecord(bundle Bundle, proposal Proposal) (Record, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return Record{}, err
	}
	if proposal.Version != ProposalVersion {
		return Record{}, fmt.Errorf("study map: unsupported proposal version %d", proposal.Version)
	}
	if len(proposal.Candidates) == 0 || len(proposal.Candidates) > MaxCandidates {
		return Record{}, fmt.Errorf("study map: candidate count must be between 1 and %d", MaxCandidates)
	}
	hash, err := BundleHash(bundle)
	if err != nil {
		return Record{}, err
	}
	index := newBundleIndex(bundle)
	reduction := Reduction{Proposed: len(proposal.Candidates)}
	brief := validateBrief(proposal.Brief, index, &reduction)
	repositoryType := proposal.RepositoryType
	if !validRepositoryType(repositoryType) {
		repositoryType = bundle.RepositoryTypeHint
	}
	if !validRepositoryType(repositoryType) {
		repositoryType = RepositoryMixed
	}
	shape := validShapeAreaIDs(proposal.ShapeAreaIDs, index)
	if len(shape) == 0 {
		shape = defaultShapeAreaIDs(bundle.Areas)
	}

	valid := make([]Direction, 0, len(proposal.Candidates))
	for candidateIndex, candidate := range proposal.Candidates {
		candidate.ReadingAnchors = canonicalProviderReadingAnchors(candidate.ReadingAnchors)
		direction, issues := validateCandidate(candidate, candidateIndex, index)
		if len(issues) > 0 {
			reduction.Issues = append(reduction.Issues, issues...)
			continue
		}
		valid = append(valid, direction)
	}
	reduction.Validated = len(valid)
	selected, downranked := selectDirections(valid)
	reduction.Issues = append(reduction.Issues, downranked...)
	reduction.Selected = len(selected)
	for index := range selected {
		selected[index].proposalIndex = 0
		selected[index].score = 0
	}
	record := Record{
		Version: RecordVersion, BundleSHA256: hash, Bundle: bundle,
		RepositoryType: repositoryType, Brief: brief, ShapeAreaIDs: shape,
		Directions: selected, Reduction: reduction,
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (bundle Bundle) Validate() error {
	if bundle.Version != BundleVersion {
		return fmt.Errorf("study map: unsupported bundle version %d", bundle.Version)
	}
	if !validText(bundle.RepoName, 256, true) {
		return fmt.Errorf("study map: invalid repository name")
	}
	if !validText(bundle.DocumentedPurpose, 4096, false) ||
		!validText(bundle.OrientationSummary, 4096, false) ||
		len(bundle.DomainTerms) > 12 {
		return fmt.Errorf("study map: repository context is outside bounds")
	}
	for _, term := range bundle.DomainTerms {
		if !validText(term.Term, 128, true) || !validText(term.Explanation, 512, true) {
			return fmt.Errorf("study map: invalid domain term")
		}
	}
	if len(bundle.Anchors) == 0 || len(bundle.Anchors) > MaxAnchors ||
		len(bundle.Areas) > MaxAreas || len(bundle.Documents) > MaxDocuments ||
		len(bundle.Mechanisms) > MaxMechanisms {
		return fmt.Errorf("study map: bundle collection is outside bounds")
	}
	allowed := make(map[string]struct{}, len(bundle.AllowedPaths))
	for _, filePath := range bundle.AllowedPaths {
		if !validPath(filePath) {
			return fmt.Errorf("study map: invalid allowed path")
		}
		if _, duplicate := allowed[filePath]; duplicate {
			return fmt.Errorf("study map: duplicate allowed path")
		}
		allowed[filePath] = struct{}{}
	}
	areas := make(map[string]struct{}, len(bundle.Areas))
	for _, area := range bundle.Areas {
		if !validOpaque(area.ID) || !validText(area.Name, 256, true) ||
			!validText(area.Responsibility, 1024, false) {
			return fmt.Errorf("study map: invalid area")
		}
		if _, duplicate := areas[area.ID]; duplicate {
			return fmt.Errorf("study map: duplicate area id %q", area.ID)
		}
		areas[area.ID] = struct{}{}
		if area.Path != "" {
			if _, ok := allowed[area.Path]; !ok || !validPath(area.Path) || area.Line < 0 {
				return fmt.Errorf("study map: area path is not allowed")
			}
		}
	}
	anchors := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		if !validOpaque(anchor.ID) || !validPath(anchor.Path) || !validText(anchor.Symbol, 256, true) ||
			anchor.Line <= 0 || !validText(anchor.Statement, 2048, true) || !validRole(anchor.Role) {
			return fmt.Errorf("study map: invalid code anchor %q", anchor.ID)
		}
		if _, ok := allowed[anchor.Path]; !ok {
			return fmt.Errorf("study map: anchor path is not allowed")
		}
		if _, duplicate := anchors[anchor.ID]; duplicate {
			return fmt.Errorf("study map: duplicate anchor id %q", anchor.ID)
		}
		anchors[anchor.ID] = struct{}{}
		if err := anchor.validateSource(); err != nil {
			return fmt.Errorf("study map: anchor source is invalid")
		}
		for _, areaID := range anchor.AreaIDs {
			if _, ok := areas[areaID]; !ok {
				return fmt.Errorf("study map: anchor references unknown area")
			}
		}
	}
	documents := make(map[string]struct{}, len(bundle.Documents))
	for _, document := range bundle.Documents {
		if !validOpaque(document.ID) || !validPath(document.Path) ||
			!validText(document.Label, 256, true) || !validText(document.Excerpt, 2048, false) {
			return fmt.Errorf("study map: invalid document")
		}
		if _, ok := allowed[document.Path]; !ok {
			return fmt.Errorf("study map: document path is not allowed")
		}
		if _, duplicate := documents[document.ID]; duplicate {
			return fmt.Errorf("study map: duplicate document id")
		}
		documents[document.ID] = struct{}{}
	}
	mechanisms := make(map[string]struct{}, len(bundle.Mechanisms))
	for _, mechanism := range bundle.Mechanisms {
		if !validOpaque(mechanism.ID) || !validText(mechanism.Question, 512, true) ||
			!validText(mechanism.Title, 256, true) {
			return fmt.Errorf("study map: invalid mechanism reference")
		}
		if _, duplicate := mechanisms[mechanism.ID]; duplicate {
			return fmt.Errorf("study map: duplicate mechanism reference")
		}
		mechanisms[mechanism.ID] = struct{}{}
		for _, anchorID := range mechanism.AnchorIDs {
			if _, ok := anchors[anchorID]; !ok {
				return fmt.Errorf("study map: mechanism references unknown anchor")
			}
		}
		for _, filePath := range mechanism.Paths {
			if !validPath(filePath) {
				return fmt.Errorf("study map: mechanism references invalid path")
			}
			if _, ok := allowed[filePath]; !ok {
				return fmt.Errorf("study map: mechanism path is not allowed")
			}
		}
	}
	return nil
}

func (record Record) Validate() error {
	if record.Version != RecordVersion {
		return fmt.Errorf("study map: unsupported record version %d", record.Version)
	}
	if err := record.Bundle.Validate(); err != nil {
		return err
	}
	hash, err := BundleHash(record.Bundle)
	if err != nil || hash != record.BundleSHA256 {
		return fmt.Errorf("study map: bundle hash mismatch")
	}
	if !validRepositoryType(record.RepositoryType) ||
		len(record.Directions) < MinDirections || len(record.Directions) > MaxDirections {
		return fmt.Errorf("study map: invalid canonical selection")
	}
	index := newBundleIndex(record.Bundle)
	briefReduction := Reduction{}
	normalizedBrief := validateBrief(record.Brief, index, &briefReduction)
	if len(briefReduction.Issues) > 0 || !briefEqual(record.Brief, normalizedBrief) ||
		!completeBrief(record.Brief) {
		return fmt.Errorf("study map: saved repository brief is invalid")
	}
	if len(record.ShapeAreaIDs) < 1 || len(record.ShapeAreaIDs) > 7 {
		return fmt.Errorf("study map: repository shape must contain one to seven areas")
	}
	for _, areaID := range record.ShapeAreaIDs {
		if _, ok := index.areas[areaID]; !ok {
			return fmt.Errorf("study map: repository shape references unknown area")
		}
	}
	seen := make(map[string]struct{}, len(record.Directions))
	for _, direction := range record.Directions {
		if !validOpaque(direction.ID) {
			return fmt.Errorf("study map: invalid direction id")
		}
		if _, duplicate := seen[direction.ID]; duplicate {
			return fmt.Errorf("study map: duplicate direction id")
		}
		seen[direction.ID] = struct{}{}
		candidate := Candidate{
			Question: direction.Question, WhyItMatters: direction.WhyItMatters,
			LearningOutcome: direction.LearningOutcome, TargetJob: direction.TargetJob,
			LearningStage: direction.LearningStage, AnchorIDs: direction.AnchorIDs,
			DocumentIDs: direction.DocumentIDs, AreaIDs: direction.AreaIDs,
			MechanismID: direction.MechanismID, ReadingAnchors: direction.ReadingAnchors,
			Confidence: "high", SearchQueries: direction.SearchQueries,
		}
		normalized, issues := validateCandidate(candidate, 0, index)
		if len(issues) > 0 {
			return fmt.Errorf("study map: saved direction is invalid: %s", issues[0].Code)
		}
		if !directionEqual(direction, normalized) {
			return fmt.Errorf("study map: saved direction is not canonical")
		}
	}
	if record.Reduction.Selected != len(record.Directions) ||
		record.Reduction.Validated < record.Reduction.Selected ||
		record.Reduction.Proposed < record.Reduction.Validated ||
		record.Reduction.Proposed > MaxCandidates {
		return fmt.Errorf("study map: invalid reduction counts")
	}
	return nil
}

func completeBrief(brief Brief) bool {
	statements := []BriefStatement{
		brief.WhatItIs,
		brief.Problem,
		brief.MainInput,
		brief.CentralResponsibility,
		brief.ObservableResult,
	}
	for _, statement := range statements {
		if !validText(statement.Text, 1024, true) || len(statement.SupportIDs) == 0 {
			return false
		}
	}
	return true
}

func briefEqual(left, right Brief) bool {
	statementEqual := func(a, b BriefStatement) bool {
		return a.Text == b.Text && slices.Equal(a.SupportIDs, b.SupportIDs)
	}
	if !statementEqual(left.WhatItIs, right.WhatItIs) ||
		!statementEqual(left.Problem, right.Problem) ||
		!statementEqual(left.MainInput, right.MainInput) ||
		!statementEqual(left.CentralResponsibility, right.CentralResponsibility) ||
		!statementEqual(left.ObservableResult, right.ObservableResult) ||
		len(left.DomainTerms) != len(right.DomainTerms) {
		return false
	}
	for index := range left.DomainTerms {
		if left.DomainTerms[index].Term != right.DomainTerms[index].Term ||
			left.DomainTerms[index].Meaning != right.DomainTerms[index].Meaning ||
			!slices.Equal(left.DomainTerms[index].SupportIDs, right.DomainTerms[index].SupportIDs) {
			return false
		}
	}
	return true
}

func directionEqual(left, right Direction) bool {
	return left.ID == right.ID &&
		left.Question == right.Question &&
		left.WhyItMatters == right.WhyItMatters &&
		left.LearningOutcome == right.LearningOutcome &&
		left.TargetJob == right.TargetJob &&
		left.LearningStage == right.LearningStage &&
		slices.Equal(left.AnchorIDs, right.AnchorIDs) &&
		slices.Equal(left.DocumentIDs, right.DocumentIDs) &&
		slices.Equal(left.AreaIDs, right.AreaIDs) &&
		left.MechanismID == right.MechanismID &&
		slices.Equal(left.ReadingAnchors, right.ReadingAnchors) &&
		slices.Equal(left.SearchQueries, right.SearchQueries)
}

type bundleIndex struct {
	repoName   string
	anchors    map[string]Anchor
	areas      map[string]Area
	documents  map[string]Document
	mechanisms map[string]Mechanism
	support    map[string]struct{}
}

func newBundleIndex(bundle Bundle) bundleIndex {
	index := bundleIndex{
		repoName:   bundle.RepoName,
		anchors:    make(map[string]Anchor, len(bundle.Anchors)),
		areas:      make(map[string]Area, len(bundle.Areas)),
		documents:  make(map[string]Document, len(bundle.Documents)),
		mechanisms: make(map[string]Mechanism, len(bundle.Mechanisms)),
		support:    make(map[string]struct{}),
	}
	for _, anchor := range bundle.Anchors {
		index.anchors[anchor.ID] = anchor
		index.support[anchor.ID] = struct{}{}
	}
	for _, area := range bundle.Areas {
		index.areas[area.ID] = area
		index.support[area.ID] = struct{}{}
	}
	for _, document := range bundle.Documents {
		index.documents[document.ID] = document
		index.support[document.ID] = struct{}{}
	}
	for _, mechanism := range bundle.Mechanisms {
		index.mechanisms[mechanism.ID] = mechanism
	}
	return index
}

func validateBrief(brief Brief, index bundleIndex, reduction *Reduction) Brief {
	statements := []*BriefStatement{
		&brief.WhatItIs, &brief.Problem, &brief.MainInput,
		&brief.CentralResponsibility, &brief.ObservableResult,
	}
	for statementIndex, statement := range statements {
		if !validText(statement.Text, 1024, false) || len(statement.SupportIDs) == 0 ||
			!allKnown(statement.SupportIDs, index.support) {
			if strings.TrimSpace(statement.Text) != "" {
				reduction.Issues = append(reduction.Issues, ReductionIssue{
					CandidateIndex: -1, Code: "brief_statement_rejected",
					Detail: fmt.Sprintf("field_%d", statementIndex),
				})
			}
			*statement = BriefStatement{}
			continue
		}
		statement.SupportIDs = uniqueStrings(statement.SupportIDs)
	}
	if len(brief.DomainTerms) > 8 {
		brief.DomainTerms = brief.DomainTerms[:8]
	}
	terms := brief.DomainTerms[:0]
	for _, term := range brief.DomainTerms {
		if validText(term.Term, 128, true) && validText(term.Meaning, 512, true) &&
			len(term.SupportIDs) > 0 &&
			allKnown(term.SupportIDs, index.support) {
			term.SupportIDs = uniqueStrings(term.SupportIDs)
			terms = append(terms, term)
		}
	}
	brief.DomainTerms = terms
	return brief
}

func validateCandidate(candidate Candidate, candidateIndex int, index bundleIndex) (Direction, []ReductionIssue) {
	issue := func(code string) []ReductionIssue {
		return []ReductionIssue{{CandidateIndex: candidateIndex, Code: code}}
	}
	if !naturalQuestion(candidate.Question) {
		return Direction{}, issue("unnatural_question")
	}
	if !validText(candidate.WhyItMatters, 1024, true) || impliesRuntimeOrder(candidate.WhyItMatters) {
		return Direction{}, issue("why_missing")
	}
	if !validText(candidate.LearningOutcome, 1024, true) || impliesRuntimeOrder(candidate.LearningOutcome) {
		return Direction{}, issue("learning_outcome_missing")
	}
	if !validTargetJob(candidate.TargetJob) {
		return Direction{}, issue("target_job_invalid")
	}
	if !validLearningStage(candidate.LearningStage) {
		return Direction{}, issue("learning_stage_invalid")
	}
	anchorIDs := uniqueStrings(candidate.AnchorIDs)
	if len(anchorIDs) < 3 || len(anchorIDs) > 5 {
		return Direction{}, issue("anchor_count_outside_3_5")
	}
	strong := 0
	for _, anchorID := range anchorIDs {
		anchor, ok := index.anchors[anchorID]
		if !ok {
			return Direction{}, issue("unknown_anchor_id")
		}
		if artifactrole.IsProduction(anchor.Role) {
			strong++
		}
	}
	if strong == 0 {
		return Direction{}, issue("strong_production_anchor_missing")
	}
	documentIDs := uniqueStrings(candidate.DocumentIDs)
	for _, documentID := range documentIDs {
		if _, ok := index.documents[documentID]; !ok {
			return Direction{}, issue("unknown_document_id")
		}
	}
	areaIDs := uniqueStrings(candidate.AreaIDs)
	for _, areaID := range areaIDs {
		if _, ok := index.areas[areaID]; !ok {
			return Direction{}, issue("unknown_area_id")
		}
	}
	if len(areaIDs) == 0 && len(documentIDs) == 0 {
		return Direction{}, issue("purpose_or_area_relation_missing")
	}
	reading := append([]ReadingAnchor(nil), candidate.ReadingAnchors...)
	if len(reading) < 3 || len(reading) > 5 {
		return Direction{}, issue("reading_anchor_count_outside_3_5")
	}
	anchorSet := make(map[string]struct{}, len(anchorIDs))
	for _, anchorID := range anchorIDs {
		anchorSet[anchorID] = struct{}{}
	}
	seenReading := make(map[string]struct{}, len(reading))
	for _, item := range reading {
		if _, ok := anchorSet[item.AnchorID]; !ok {
			return Direction{}, issue("reading_anchor_not_selected")
		}
		if _, duplicate := seenReading[item.AnchorID]; duplicate {
			return Direction{}, issue("duplicate_reading_anchor")
		}
		if !validReadingLabel(item.Label) || !validText(item.WhatToLookFor, 768, true) || impliesRuntimeOrder(item.WhatToLookFor) {
			return Direction{}, issue("unsafe_reading_copy")
		}
		seenReading[item.AnchorID] = struct{}{}
	}
	mechanismID := strings.TrimSpace(candidate.MechanismID)
	if mechanismID != "" {
		mechanism, ok := index.mechanisms[mechanismID]
		if !ok || !intersects(anchorIDs, mechanism.AnchorIDs) {
			mechanismID = ""
		}
	}
	queries := uniqueBoundedText(candidate.SearchQueries, 8, 256)
	direction := Direction{
		ID:       stableID("study", candidate.Question, strings.Join(anchorIDs, "\x00")),
		Question: strings.TrimSpace(candidate.Question), WhyItMatters: strings.TrimSpace(candidate.WhyItMatters),
		LearningOutcome: strings.TrimSpace(candidate.LearningOutcome), TargetJob: candidate.TargetJob,
		LearningStage: candidate.LearningStage, AnchorIDs: anchorIDs,
		DocumentIDs: documentIDs, AreaIDs: areaIDs, MechanismID: mechanismID,
		ReadingAnchors: reading, SearchQueries: queries, proposalIndex: candidateIndex,
	}
	direction.score = candidateScore(candidate, direction)
	return direction, nil
}

func candidateScore(candidate Candidate, direction Direction) int {
	score := len(direction.AnchorIDs)*3 + len(direction.AreaIDs)*2
	if len(direction.DocumentIDs) > 0 {
		score += 2
	}
	if direction.MechanismID != "" {
		score += 3
	}
	switch strings.ToLower(strings.TrimSpace(candidate.Confidence)) {
	case "high":
		score += 4
	case "medium":
		score += 2
	}
	return score
}

func selectDirections(candidates []Direction) ([]Direction, []ReductionIssue) {
	ordered := append([]Direction(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return ordered[i].proposalIndex < ordered[j].proposalIndex
	})
	selected := make([]Direction, 0, min(MaxDirections, len(ordered)))
	dropped := make([]ReductionIssue, 0)
	seenAreas := make(map[string]struct{})
	seenJobs := make(map[TargetJob]struct{})
	chosen := make(map[string]struct{})
	for pass := 0; pass < 2 && len(selected) < MaxDirections; pass++ {
		for _, candidate := range ordered {
			if _, exists := chosen[candidate.ID]; exists || duplicateDirection(candidate, selected) {
				continue
			}
			newArea := hasNewString(candidate.AreaIDs, seenAreas)
			_, newJob := seenJobs[candidate.TargetJob]
			if pass == 0 && !newArea && newJob {
				continue
			}
			selected = append(selected, candidate)
			chosen[candidate.ID] = struct{}{}
			for _, areaID := range candidate.AreaIDs {
				seenAreas[areaID] = struct{}{}
			}
			seenJobs[candidate.TargetJob] = struct{}{}
			if len(selected) == MaxDirections {
				break
			}
		}
	}
	for _, candidate := range ordered {
		if _, ok := chosen[candidate.ID]; !ok {
			dropped = append(dropped, ReductionIssue{CandidateIndex: candidate.proposalIndex, Code: "downranked_after_diversity_selection"})
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left, right := stageRank(selected[i].LearningStage), stageRank(selected[j].LearningStage)
		if left != right {
			return left < right
		}
		return selected[i].proposalIndex < selected[j].proposalIndex
	})
	return selected, dropped
}

func duplicateDirection(candidate Direction, selected []Direction) bool {
	left := questionTerms(candidate.Question)
	for _, item := range selected {
		if candidate.MechanismID != "" && candidate.MechanismID == item.MechanismID {
			return true
		}
		anchorOverlap := stringSetJaccard(candidate.AnchorIDs, item.AnchorIDs)
		if anchorOverlap >= 0.8 ||
			anchorOverlap >= 0.6 && candidate.LearningStage == item.LearningStage {
			return true
		}
		right := questionTerms(item.Question)
		intersection := 0
		for term := range left {
			if _, ok := right[term]; ok {
				intersection++
			}
		}
		union := len(left) + len(right) - intersection
		if union > 0 && float64(intersection)/float64(union) >= 0.68 {
			return true
		}
	}
	return false
}

func stringSetJaccard(left, right []string) float64 {
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	intersection := 0
	for value := range leftSet {
		if _, ok := rightSet[value]; ok {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func canonicalBundle(bundle Bundle) Bundle {
	bundle.DomainTerms = append([]DomainTerm(nil), bundle.DomainTerms...)
	bundle.Areas = append([]Area(nil), bundle.Areas...)
	bundle.Anchors = append([]Anchor(nil), bundle.Anchors...)
	bundle.Documents = append([]Document(nil), bundle.Documents...)
	bundle.Mechanisms = append([]Mechanism(nil), bundle.Mechanisms...)
	bundle.AllowedPaths = uniqueStrings(bundle.AllowedPaths)
	sort.Slice(bundle.DomainTerms, func(i, j int) bool { return bundle.DomainTerms[i].Term < bundle.DomainTerms[j].Term })
	sort.Slice(bundle.Areas, func(i, j int) bool { return bundle.Areas[i].ID < bundle.Areas[j].ID })
	for index := range bundle.Anchors {
		bundle.Anchors[index].AreaIDs = uniqueStrings(bundle.Anchors[index].AreaIDs)
		bundle.Anchors[index].Capabilities = uniqueCapabilities(bundle.Anchors[index].Capabilities)
		if bundle.Anchors[index].ExactSource != nil {
			exact := *bundle.Anchors[index].ExactSource
			exact.Lines = append([]string(nil), exact.Lines...)
			bundle.Anchors[index].ExactSource = &exact
		}
	}
	sort.Slice(bundle.Anchors, func(i, j int) bool { return bundle.Anchors[i].ID < bundle.Anchors[j].ID })
	sort.Slice(bundle.Documents, func(i, j int) bool { return bundle.Documents[i].ID < bundle.Documents[j].ID })
	for index := range bundle.Mechanisms {
		bundle.Mechanisms[index].AnchorIDs = uniqueStrings(bundle.Mechanisms[index].AnchorIDs)
		bundle.Mechanisms[index].Paths = uniqueStrings(bundle.Mechanisms[index].Paths)
	}
	sort.Slice(bundle.Mechanisms, func(i, j int) bool { return bundle.Mechanisms[i].ID < bundle.Mechanisms[j].ID })
	return bundle
}

func (anchor Anchor) validateSource() error {
	hasFunction := sourceFunctionPresent(anchor.Function)
	hasExact := anchor.ExactSource != nil
	if hasFunction == hasExact {
		return fmt.Errorf("study map: anchor must select exactly one source arm")
	}
	if hasFunction {
		if err := anchor.Function.Validate(); err != nil || anchor.Function.Path != anchor.Path ||
			anchor.Function.Symbol != anchor.Symbol || anchor.Line < anchor.Function.StartLine ||
			anchor.Line > anchor.Function.EndLine {
			return fmt.Errorf("study map: Go anchor source does not match anchor")
		}
		return nil
	}
	if err := anchor.ExactSource.Validate(); err != nil ||
		anchor.ExactSource.Path != anchor.Path || anchor.ExactSource.Symbol != anchor.Symbol ||
		anchor.ExactSource.Line != anchor.Line {
		return fmt.Errorf("study map: exact anchor source does not match anchor")
	}
	return nil
}

func (anchor Anchor) sourceLines() (int, []string, error) {
	if err := anchor.validateSource(); err != nil {
		return 0, nil, err
	}
	if anchor.ExactSource != nil {
		return anchor.ExactSource.StartLine, anchor.ExactSource.Lines, nil
	}
	return anchor.Function.StartLine, anchor.Function.Lines, nil
}

func sourceFunctionPresent(function sourcewindowfacts.Function) bool {
	return function.Symbol != "" || function.Path != "" || function.StartLine != 0 ||
		function.EndLine != 0 || len(function.Lines) != 0 || function.ContentSHA256 != "" ||
		function.Partial || len(function.Observations) != 0
}

// Validate checks the bounded non-Go source arm without interpreting its
// syntax. Catalog resolution and repository freshness remain assembly-time
// responsibilities; this contract protects exact path, line, and saved bytes.
func (source ExactSource) Validate() error {
	if !validPath(source.Path) || !validText(source.Symbol, 256, true) ||
		source.Language != exactSourceLanguage(source.Path) || source.Language == "" {
		return fmt.Errorf("study map: invalid exact source identity")
	}
	if source.Line <= 0 || source.StartLine <= 0 || source.EndLine < source.StartLine ||
		source.Line < source.StartLine || source.Line > source.EndLine ||
		len(source.Lines) == 0 || len(source.Lines) > maxExactSourceLines ||
		len(source.Lines) != source.EndLine-source.StartLine+1 {
		return fmt.Errorf("study map: invalid exact source bounds")
	}
	totalBytes := 0
	for _, line := range source.Lines {
		if !utf8.ValidString(line) || len(line) > maxExactSourceLineBytes ||
			strings.ContainsAny(line, "\x00\r\n") {
			return fmt.Errorf("study map: invalid exact source line")
		}
		totalBytes += len(line)
		if totalBytes > maxExactSourceBytes {
			return fmt.Errorf("study map: exact source exceeds byte budget")
		}
	}
	raw, _ := json.Marshal(source.Lines)
	digest := sha256.Sum256(raw)
	if source.ContentSHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("study map: exact source content hash mismatch")
	}
	return nil
}

func exactSourceLanguage(sourcePath string) string {
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

func defaultShapeAreaIDs(areas []Area) []string {
	result := make([]string, 0, min(7, len(areas)))
	for _, area := range areas {
		result = append(result, area.ID)
		if len(result) == 7 {
			break
		}
	}
	return result
}

func validShapeAreaIDs(ids []string, index bundleIndex) []string {
	ids = uniqueStrings(ids)
	if len(ids) > 7 {
		ids = ids[:7]
	}
	result := ids[:0]
	for _, id := range ids {
		if _, ok := index.areas[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func validRepositoryType(value RepositoryType) bool {
	switch value {
	case RepositoryService, RepositoryLibrary, RepositoryCLI, RepositoryMonorepo, RepositoryMixed:
		return true
	default:
		return false
	}
}

func validLearningStage(value LearningStage) bool {
	switch value {
	case StageOrientation, StageCentralOperation, StageCoreModel, StageIntegration, StageOperations, StageContribution:
		return true
	default:
		return false
	}
}

func validTargetJob(value TargetJob) bool {
	switch value {
	case JobFirstContact, JobUseOperate, JobIntegrate, JobContribute, JobMaintain:
		return true
	default:
		return false
	}
}

func validRole(value artifactrole.Role) bool {
	switch value {
	case artifactrole.RolePrimaryProductionEntry, artifactrole.RoleProductionCore,
		artifactrole.RoleEffectBoundary, artifactrole.RolePublicAPI,
		artifactrole.RoleExample, artifactrole.RoleTest, artifactrole.RoleFixture,
		artifactrole.RoleGenerated, artifactrole.RolePlayground,
		artifactrole.RoleExperimental, artifactrole.RoleCurrentDocumentation,
		artifactrole.RoleHistoricalDocumentation:
		return true
	default:
		return false
	}
}

func validReadingLabel(value string) bool {
	switch strings.TrimSpace(value) {
	case "Start here", "Then inspect", "Related implementation", "Public boundary", "Core data type":
		return true
	default:
		return false
	}
}

// canonicalProviderReadingLabel keeps localized presentation labels out of the
// saved contract. Only the canonical labels and their report-owned Russian
// equivalents are accepted; unknown model-authored labels remain invalid.
func canonicalProviderReadingLabel(value string) (string, bool) {
	if len(value) > 64 {
		return "", false
	}
	switch strings.TrimSpace(value) {
	case "Start here", "С чего начать":
		return "Start here", true
	case "Then inspect", "Затем изучите":
		return "Then inspect", true
	case "Related implementation", "Связанная реализация":
		return "Related implementation", true
	case "Public boundary", "Публичная граница":
		return "Public boundary", true
	case "Core data type", "Основной тип данных":
		return "Core data type", true
	default:
		return "", false
	}
}

func canonicalProviderReadingAnchors(reading []ReadingAnchor) []ReadingAnchor {
	// Candidate validation accepts at most five anchors. Leave an oversized
	// provider collection untouched so the validator can reject it without a
	// second allocation proportional to untrusted input.
	if len(reading) > 5 {
		return reading
	}
	result := make([]ReadingAnchor, len(reading))
	copy(result, reading)
	for index := range result {
		if label, ok := canonicalProviderReadingLabel(result[index].Label); ok {
			result[index].Label = label
		}
	}
	return result
}

func impliesRuntimeOrder(value string) bool {
	value = strings.ToLower(value)
	for _, phrase := range []string{
		"then the system", "next the system", "first the system",
		"then it executes", "next it executes", "after it executes",
		"runtime step", "runtime order", "execution order", "proven sequence",
		"executes after", "executes before", "subsequently executes",
		"затем система", "далее система", "сначала система",
		"затем выполняется", "далее выполняется", "после этого выполняется",
		"шаг выполнения", "порядок выполнения", "порядок исполнения",
		"доказанная последовательность", "выполняется после",
		"выполняется до", "впоследствии выполняется",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	// Russian permits the temporal marker on either side of the runtime
	// subject. Cover that ordinary inflection/order without treating general
	// editorial instructions such as "сначала изучите" as runtime claims.
	for _, subject := range []string{
		"система", "код", "функция", "метод", "сервис", "обработчик",
		"процесс", "приложение",
	} {
		for _, marker := range []string{"сначала", "затем", "потом", "далее"} {
			if strings.Contains(value, subject+" "+marker) ||
				strings.Contains(value, marker+" "+subject) {
				return true
			}
		}
	}
	return false
}

func naturalQuestion(value string) bool {
	value = strings.TrimSpace(value)
	if !validText(value, 512, true) || !strings.HasSuffix(value, "?") {
		return false
	}
	words := strings.Fields(value)
	return len(words) >= 4
}

func validOpaque(value string) bool {
	trimmed := strings.TrimSpace(value)
	if value != trimmed || value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("-_.:/", char) {
			continue
		}
		return false
	}
	return true
}

func validPath(value string) bool {
	return value != "" && value == path.Clean(value) && !path.IsAbs(value) &&
		value != "." && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func validText(value string, limit int, required bool) bool {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return false
	}
	return len(value) <= limit && !strings.ContainsAny(value, "\x00\r")
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func uniqueCapabilities(values []semanticdiscovery.Capability) []semanticdiscovery.Capability {
	seen := make(map[semanticdiscovery.Capability]struct{}, len(values))
	result := make([]semanticdiscovery.Capability, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueBoundedText(values []string, limit, byteLimit int) []string {
	result := make([]string, 0, min(limit, len(values)))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validText(value, byteLimit, true) {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func allKnown(values []string, known map[string]struct{}) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, ok := known[value]; !ok {
			return false
		}
	}
	return true
}

func intersects(left, right []string) bool {
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func hasNewString(values []string, seen map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := seen[value]; !ok {
			return true
		}
	}
	return false
}

func questionTerms(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool { return !unicode.IsLetter(char) && !unicode.IsDigit(char) }) {
		if len(token) >= 4 {
			result[token] = struct{}{}
		}
	}
	return result
}

func stageRank(value LearningStage) int {
	switch value {
	case StageOrientation:
		return 0
	case StageCentralOperation:
		return 1
	case StageCoreModel:
		return 2
	case StageIntegration:
		return 3
	case StageOperations:
		return 4
	case StageContribution:
		return 5
	default:
		return 6
	}
}

func stableID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("study map: trailing JSON")
		}
		return fmt.Errorf("study map: invalid trailing JSON: %w", err)
	}
	return nil
}
