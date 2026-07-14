// Package componentstudy contains the first bounded planning experiment for
// studying one selected repository component. It is deliberately pure: this
// package performs no repository discovery, model I/O, gopls calls, or report
// rendering.
package componentstudy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SeedVersion           = 1
	BundleVersion         = 1
	SelectionTraceVersion = 1
	BudgetVersion         = 1
	PlanVersion           = 2

	maxRepoNameBytes       = 128
	maxNameBytes           = 256
	maxObjectiveBytes      = 1_024
	maxPurposeBytes        = 1_024
	maxReasonBytes         = 512
	maxStatementBytes      = 1_024
	maxProvenanceBytes     = 256
	maxPathBytes           = 1_024
	maxResponseTextBytes   = 1_024
	maxResponseListItems   = 8
	maxQuestionCount       = 4
	maxSelectedFileCount   = 2
	maxSelectedSymbolCount = 3
	maxSeedCandidates      = 4_096
	maxModelBytes          = 1 << 20
)

var opaqueIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

type GoalKind string

const GoalOnboarding GoalKind = "onboarding"

type Certainty string

const (
	CertaintyHypothesis Certainty = "hypothesis"
	CertaintyPossible   Certainty = "possible"
	CertaintyNavigation Certainty = "navigation"
	CertaintyStatic     Certainty = "static"
	CertaintyObserved   Certainty = "observed"
	CertaintyVerified   Certainty = "verified"
)

type EvidenceKind string

const (
	EvidenceRelation  EvidenceKind = "relation"
	EvidenceDirection EvidenceKind = "direction"
)

type SelectionKind string

const (
	SelectionAnchor   SelectionKind = "anchor"
	SelectionFile     SelectionKind = "file"
	SelectionSymbol   SelectionKind = "symbol"
	SelectionEvidence SelectionKind = "evidence"
)

type SelectionReason string

const (
	SelectionWithinBudget     SelectionReason = "within_budget"
	SelectionAnchorLimit      SelectionReason = "anchor_limit"
	SelectionFileLimit        SelectionReason = "file_limit"
	SelectionSymbolLimit      SelectionReason = "symbol_limit"
	SelectionEvidenceLimit    SelectionReason = "evidence_limit"
	SelectionModelBytesLimit  SelectionReason = "model_bytes_limit"
	SelectionMissingReference SelectionReason = "referenced_item_omitted"
)

type Goal struct {
	ID        string   `json:"id"`
	Kind      GoalKind `json:"kind"`
	Objective string   `json:"objective"`
}

type Component struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

type Provenance struct {
	Source    string `json:"source"`
	Operation string `json:"operation"`
	Detail    string `json:"detail,omitempty"`
}

type AnchorCandidate struct {
	ID         string     `json:"id"`
	Rank       int        `json:"rank"`
	Path       string     `json:"path"`
	Line       int        `json:"line,omitempty"`
	Column     int        `json:"column,omitempty"`
	Reason     string     `json:"reason"`
	Provenance Provenance `json:"provenance"`
	Certainty  Certainty  `json:"certainty"`
}

type FileCandidate struct {
	ID         string     `json:"id"`
	Rank       int        `json:"rank"`
	Path       string     `json:"path"`
	Reason     string     `json:"reason"`
	Provenance Provenance `json:"provenance"`
	Certainty  Certainty  `json:"certainty"`
}

type SymbolCandidate struct {
	ID         string     `json:"id"`
	Rank       int        `json:"rank"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Path       string     `json:"path"`
	Line       int        `json:"line"`
	Column     int        `json:"column,omitempty"`
	Reason     string     `json:"reason"`
	Provenance Provenance `json:"provenance"`
	Certainty  Certainty  `json:"certainty"`
}

// EvidenceCandidate records either a local relation or a previously proposed
// direction. RelatedIDs may point only at the selected component or structural
// candidates in the same seed.
type EvidenceCandidate struct {
	ID         string       `json:"id"`
	Rank       int          `json:"rank"`
	Kind       EvidenceKind `json:"kind"`
	Statement  string       `json:"statement"`
	RelatedIDs []string     `json:"related_ids"`
	Reason     string       `json:"reason"`
	Provenance Provenance   `json:"provenance"`
	Certainty  Certainty    `json:"certainty"`
}

type Seed struct {
	Version   int                 `json:"version"`
	RepoName  string              `json:"repo_name"`
	Goal      Goal                `json:"goal"`
	Component Component           `json:"component"`
	Anchors   []AnchorCandidate   `json:"anchors"`
	Files     []FileCandidate     `json:"files"`
	Symbols   []SymbolCandidate   `json:"symbols"`
	Evidence  []EvidenceCandidate `json:"evidence"`
}

type Budget struct {
	Version       int `json:"version"`
	MaxAnchors    int `json:"max_anchors"`
	MaxFiles      int `json:"max_files"`
	MaxSymbols    int `json:"max_symbols"`
	MaxEvidence   int `json:"max_evidence"`
	MaxModelBytes int `json:"max_model_bytes"`
}

type Bundle struct {
	Version   int                 `json:"version"`
	RepoName  string              `json:"repo_name"`
	Goal      Goal                `json:"goal"`
	Component Component           `json:"component"`
	Anchors   []AnchorCandidate   `json:"anchors"`
	Files     []FileCandidate     `json:"files"`
	Symbols   []SymbolCandidate   `json:"symbols"`
	Evidence  []EvidenceCandidate `json:"evidence"`
}

type SelectionDecision struct {
	Kind           SelectionKind   `json:"kind"`
	ID             string          `json:"id"`
	Rank           int             `json:"rank"`
	Included       bool            `json:"included"`
	Reason         SelectionReason `json:"reason"`
	EstimatedBytes int             `json:"estimated_bytes"`
}

type SelectionTrace struct {
	Version             int                 `json:"version"`
	Budget              Budget              `json:"budget"`
	Decisions           []SelectionDecision `json:"decisions"`
	EstimatedModelBytes int                 `json:"estimated_model_bytes"`
}

type Question struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Why         string   `json:"why"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// Plan is locally normalized. Structural selections are reconstructed from
// Bundle rather than accepted from model-provided paths or symbols.
type Plan struct {
	Version           int               `json:"version"`
	Framing           string            `json:"framing"`
	Questions         []Question        `json:"questions"`
	PrimaryQuestionID string            `json:"primary_question_id,omitempty"`
	SelectedFiles     []FileCandidate   `json:"selected_files"`
	SelectedSymbols   []SymbolCandidate `json:"selected_symbols"`
	Unknowns          []string          `json:"unknowns"`
	Warnings          []string          `json:"warnings"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type Result struct {
	Plan        Plan         `json:"plan"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (s Seed) Validate() error {
	if s.Version != SeedVersion {
		return fmt.Errorf("component study: unsupported seed version %d", s.Version)
	}
	if err := validateText("repo_name", s.RepoName, maxRepoNameBytes); err != nil {
		return err
	}
	if err := validateGoal(s.Goal); err != nil {
		return err
	}
	if err := validateComponent(s.Component); err != nil {
		return err
	}
	if s.Goal.ID == s.Component.ID {
		return fmt.Errorf("component study: goal and component ids must differ")
	}
	if len(s.Anchors) > maxSeedCandidates || len(s.Files) > maxSeedCandidates ||
		len(s.Symbols) > maxSeedCandidates || len(s.Evidence) > maxSeedCandidates {
		return fmt.Errorf("component study: seed candidate count exceeds hard limit")
	}

	knownIDs := map[string]struct{}{s.Goal.ID: {}, s.Component.ID: {}}
	structuralIDs := map[string]struct{}{s.Goal.ID: {}, s.Component.ID: {}}
	for index, candidate := range s.Anchors {
		if err := validateAnchor(candidate); err != nil {
			return fmt.Errorf("component study: anchors[%d]: %w", index, err)
		}
		if err := addUniqueID(knownIDs, candidate.ID); err != nil {
			return err
		}
		structuralIDs[candidate.ID] = struct{}{}
	}
	for index, candidate := range s.Files {
		if err := validateFile(candidate); err != nil {
			return fmt.Errorf("component study: files[%d]: %w", index, err)
		}
		if err := addUniqueID(knownIDs, candidate.ID); err != nil {
			return err
		}
		structuralIDs[candidate.ID] = struct{}{}
	}
	for index, candidate := range s.Symbols {
		if err := validateSymbol(candidate); err != nil {
			return fmt.Errorf("component study: symbols[%d]: %w", index, err)
		}
		if err := addUniqueID(knownIDs, candidate.ID); err != nil {
			return err
		}
		structuralIDs[candidate.ID] = struct{}{}
	}
	for index, candidate := range s.Evidence {
		if err := validateEvidence(candidate); err != nil {
			return fmt.Errorf("component study: evidence[%d]: %w", index, err)
		}
		if err := addUniqueID(knownIDs, candidate.ID); err != nil {
			return err
		}
	}
	for index, candidate := range s.Evidence {
		for relatedIndex, id := range candidate.RelatedIDs {
			if _, exists := structuralIDs[id]; !exists {
				return fmt.Errorf(
					"component study: evidence[%d].related_ids[%d] references unknown id %q",
					index,
					relatedIndex,
					id,
				)
			}
		}
	}
	return nil
}

func (b Budget) Validate() error {
	if b.Version != BudgetVersion {
		return fmt.Errorf("component study: unsupported budget version %d", b.Version)
	}
	if b.MaxAnchors <= 0 || b.MaxFiles <= 0 || b.MaxSymbols <= 0 || b.MaxEvidence <= 0 {
		return fmt.Errorf("component study: every item budget must be positive")
	}
	if b.MaxModelBytes <= 0 || b.MaxModelBytes > maxModelBytes {
		return fmt.Errorf("component study: max_model_bytes must be between 1 and %d", maxModelBytes)
	}
	return nil
}

func (b Bundle) Validate() error {
	seed := Seed{
		Version:   SeedVersion,
		RepoName:  b.RepoName,
		Goal:      b.Goal,
		Component: b.Component,
		Anchors:   b.Anchors,
		Files:     b.Files,
		Symbols:   b.Symbols,
		Evidence:  b.Evidence,
	}
	if b.Version != BundleVersion {
		return fmt.Errorf("component study: unsupported bundle version %d", b.Version)
	}
	return seed.Validate()
}

func validateGoal(goal Goal) error {
	if err := validateID("goal.id", goal.ID); err != nil {
		return err
	}
	if goal.Kind != GoalOnboarding {
		return fmt.Errorf("component study: unsupported goal kind %q", goal.Kind)
	}
	return validateText("goal.objective", goal.Objective, maxObjectiveBytes)
}

func validateComponent(component Component) error {
	if err := validateID("component.id", component.ID); err != nil {
		return err
	}
	if err := validateText("component.name", component.Name, maxNameBytes); err != nil {
		return err
	}
	return validateText("component.purpose", component.Purpose, maxPurposeBytes)
}

func validateAnchor(candidate AnchorCandidate) error {
	if err := validateCandidate(
		candidate.ID,
		candidate.Rank,
		candidate.Reason,
		candidate.Provenance,
		candidate.Certainty,
	); err != nil {
		return err
	}
	if err := validateRepoPath(candidate.Path); err != nil {
		return err
	}
	if candidate.Line < 0 || candidate.Column < 0 || (candidate.Line == 0 && candidate.Column != 0) {
		return fmt.Errorf("invalid anchor position")
	}
	return nil
}

func validateFile(candidate FileCandidate) error {
	if err := validateCandidate(
		candidate.ID,
		candidate.Rank,
		candidate.Reason,
		candidate.Provenance,
		candidate.Certainty,
	); err != nil {
		return err
	}
	return validateRepoPath(candidate.Path)
}

func validateSymbol(candidate SymbolCandidate) error {
	if err := validateCandidate(
		candidate.ID,
		candidate.Rank,
		candidate.Reason,
		candidate.Provenance,
		candidate.Certainty,
	); err != nil {
		return err
	}
	if err := validateText("symbol.name", candidate.Name, maxNameBytes); err != nil {
		return err
	}
	if err := validateText("symbol.kind", candidate.Kind, 64); err != nil {
		return err
	}
	if err := validateRepoPath(candidate.Path); err != nil {
		return err
	}
	if candidate.Line <= 0 || candidate.Column < 0 {
		return fmt.Errorf("invalid symbol position")
	}
	return nil
}

func validateEvidence(candidate EvidenceCandidate) error {
	if err := validateCandidate(
		candidate.ID,
		candidate.Rank,
		candidate.Reason,
		candidate.Provenance,
		candidate.Certainty,
	); err != nil {
		return err
	}
	if candidate.Kind != EvidenceRelation && candidate.Kind != EvidenceDirection {
		return fmt.Errorf("invalid evidence kind %q", candidate.Kind)
	}
	if err := validateText("evidence.statement", candidate.Statement, maxStatementBytes); err != nil {
		return err
	}
	if len(candidate.RelatedIDs) == 0 || len(candidate.RelatedIDs) > 8 {
		return fmt.Errorf("evidence related_ids must contain 1-8 ids")
	}
	seen := make(map[string]struct{}, len(candidate.RelatedIDs))
	for _, id := range candidate.RelatedIDs {
		if err := validateID("evidence.related_id", id); err != nil {
			return err
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate related id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateCandidate(id string, rank int, reason string, provenance Provenance, certainty Certainty) error {
	if err := validateID("candidate.id", id); err != nil {
		return err
	}
	if rank <= 0 || rank > maxSeedCandidates {
		return fmt.Errorf("candidate rank must be between 1 and %d", maxSeedCandidates)
	}
	if err := validateText("candidate.reason", reason, maxReasonBytes); err != nil {
		return err
	}
	if err := validateProvenance(provenance); err != nil {
		return err
	}
	if !certainty.valid() {
		return fmt.Errorf("invalid certainty %q", certainty)
	}
	return nil
}

func validateProvenance(provenance Provenance) error {
	if err := validateText("provenance.source", provenance.Source, 64); err != nil {
		return err
	}
	if err := validateText("provenance.operation", provenance.Operation, 64); err != nil {
		return err
	}
	if provenance.Detail == "" {
		return nil
	}
	return validateText("provenance.detail", provenance.Detail, maxProvenanceBytes)
}

func validateID(field, id string) error {
	if !opaqueIDPattern.MatchString(id) || strings.Contains(id, ".go") {
		return fmt.Errorf("component study: %s is not an opaque id", field)
	}
	return nil
}

func validateText(field, value string, maxBytes int) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("component study: %s is empty or not trimmed", field)
	}
	if !utf8.ValidString(value) || len(value) > maxBytes {
		return fmt.Errorf("component study: %s exceeds its text bound", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("component study: %s contains control characters", field)
		}
	}
	return nil
}

func validateRepoPath(path string) error {
	if err := validateText("path", path, maxPathBytes); err != nil {
		return err
	}
	localPath := filepath.FromSlash(path)
	if strings.Contains(path, `\`) {
		return fmt.Errorf("component study: path %q must use repository slash separators", path)
	}
	if filepath.IsAbs(localPath) || !filepath.IsLocal(localPath) {
		return fmt.Errorf("component study: path %q is not repository-relative", path)
	}
	if filepath.ToSlash(filepath.Clean(localPath)) != path {
		return fmt.Errorf("component study: path %q is not canonical", path)
	}
	return nil
}

func addUniqueID(ids map[string]struct{}, id string) error {
	if _, exists := ids[id]; exists {
		return fmt.Errorf("component study: duplicate id %q", id)
	}
	ids[id] = struct{}{}
	return nil
}

func (c Certainty) valid() bool {
	switch c {
	case CertaintyHypothesis,
		CertaintyPossible,
		CertaintyNavigation,
		CertaintyStatic,
		CertaintyObserved,
		CertaintyVerified:
		return true
	default:
		return false
	}
}

func sortedCandidates[T interface {
	AnchorCandidate | FileCandidate | SymbolCandidate | EvidenceCandidate
}](items []T, rank func(T) int, id func(T) string) []T {
	result := append([]T{}, items...)
	sort.Slice(result, func(i, j int) bool {
		if rank(result[i]) != rank(result[j]) {
			return rank(result[i]) < rank(result[j])
		}
		return id(result[i]) < id(result[j])
	})
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}
