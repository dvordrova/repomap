// Package targetoutcome owns the sealed, adapter-neutral result inventory for
// every selected repository target. It records whether each selected target
// produced a validated ProgramIndex page without persisting adapter-native
// refs or raw analysis errors.
package targetoutcome

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
)

const (
	Version          = 1
	ArtifactFilename = "target-outcome-portfolio.json"

	MaxOutcomes      = programindex.MaxArtifactSetEntries
	MaxArtifactBytes = 8 * 1024 * 1024
)

const digestDomain = "target-outcome-portfolio-v1\x00"

// LanguageGroup is the closed, adapter-neutral family that owns a selected
// target. JavaScript and TypeScript share a group because one package target
// can be materialized as either exact ProgramIndex language.
type LanguageGroup string

const (
	LanguageGroupGo                   LanguageGroup = "go"
	LanguageGroupPython               LanguageGroup = "python"
	LanguageGroupJavaScriptTypeScript LanguageGroup = "javascript_typescript"
)

func (group LanguageGroup) Valid() bool {
	switch group {
	case LanguageGroupGo, LanguageGroupPython, LanguageGroupJavaScriptTypeScript:
		return true
	default:
		return false
	}
}

// ScopeKind describes the selected pre-analysis scope without exposing an
// adapter-native target kind or ref.
type ScopeKind string

const (
	ScopeExecutable ScopeKind = "executable"
	ScopeLibrary    ScopeKind = "library"
	ScopePackage    ScopeKind = "package"
)

func (kind ScopeKind) Valid() bool {
	switch kind {
	case ScopeExecutable, ScopeLibrary, ScopePackage:
		return true
	default:
		return false
	}
}

// State is the exhaustive result state for one selected target.
type State string

const (
	StateAnalyzed    State = "analyzed"
	StateNotAnalyzed State = "not_analyzed"
)

func (state State) Valid() bool {
	return state == StateAnalyzed || state == StateNotAnalyzed
}

// Stage is a closed public failure boundary. It deliberately excludes raw
// pipeline stage names, provider details, and adapter-specific operations.
type Stage string

const (
	StageTargetPreparation  Stage = "target_preparation"
	StageProgramAnalysis    Stage = "program_analysis"
	StageDependencyAnalysis Stage = "dependency_analysis"
	StageSemanticAnalysis   Stage = "semantic_analysis"
	StageTargetPage         Stage = "target_page"
)

func (stage Stage) Valid() bool {
	switch stage {
	case StageTargetPreparation, StageProgramAnalysis, StageDependencyAnalysis,
		StageSemanticAnalysis, StageTargetPage:
		return true
	default:
		return false
	}
}

// Reason is a closed, sanitized explanation of why a target was not analyzed.
// Raw error text is intentionally not representable in the persisted model.
type Reason string

const (
	ReasonSourceNotAnalyzable     Reason = "source_not_analyzable"
	ReasonRequiredToolUnavailable Reason = "required_tool_unavailable"
	ReasonResourceLimit           Reason = "resource_limit"
	ReasonModelResultRejected     Reason = "model_result_rejected"
	ReasonAnalysisFailed          Reason = "analysis_failed"
	ReasonTargetOutputInvalid     Reason = "target_output_invalid"
)

func (reason Reason) Valid() bool {
	switch reason {
	case ReasonSourceNotAnalyzable, ReasonRequiredToolUnavailable, ReasonResourceLimit,
		ReasonModelResultRejected, ReasonAnalysisFailed, ReasonTargetOutputInvalid:
		return true
	default:
		return false
	}
}

// SelectedTarget is the public identity available before adapter-local page
// analysis. Selector is the exact user-facing selector, never the native
// adapter ref used by command orchestration.
type SelectedTarget struct {
	ID            string        `json:"id"`
	LanguageGroup LanguageGroup `json:"language_group"`
	ScopeKind     ScopeKind     `json:"scope_kind"`
	DisplayName   string        `json:"display_name"`
	Selector      string        `json:"selector"`
}

// NewSelectedTarget validates and seals the exact public selected-target
// tuple. Its identity changes when any tuple field changes.
func NewSelectedTarget(
	languageGroup LanguageGroup,
	scopeKind ScopeKind,
	displayName string,
	selector string,
) (SelectedTarget, error) {
	target := SelectedTarget{
		LanguageGroup: languageGroup,
		ScopeKind:     scopeKind,
		DisplayName:   displayName,
		Selector:      selector,
	}
	if err := target.validateShape(); err != nil {
		return SelectedTarget{}, err
	}
	target.ID = selectedTargetID(target)
	if err := target.Validate(); err != nil {
		return SelectedTarget{}, err
	}
	return target, nil
}

// Validate checks the closed selected-target shape and its stable identity.
func (target SelectedTarget) Validate() error {
	if err := target.validateShape(); err != nil {
		return err
	}
	if target.ID != selectedTargetID(target) {
		return fmt.Errorf("target outcome portfolio: selected target identity mismatch")
	}
	return nil
}

func (target SelectedTarget) validateShape() error {
	if !target.LanguageGroup.Valid() || !target.ScopeKind.Valid() ||
		!validText(target.DisplayName) || !validText(target.Selector) {
		return fmt.Errorf("target outcome portfolio: invalid selected target")
	}
	return nil
}

// Analysis binds one selected target to the complete validated public target
// and safe child run that own its backing page.
type Analysis struct {
	ProgramTarget programindex.Target `json:"program_target"`
	RunID         string              `json:"run_id"`
}

// Failure is the complete persisted failure surface. It has no free-form
// detail field by design.
type Failure struct {
	Stage  Stage  `json:"stage"`
	Reason Reason `json:"reason"`
}

// Outcome is a strict tagged union. Exactly one of Analysis or Failure is
// present, as selected by State.
type Outcome struct {
	SelectedTarget SelectedTarget `json:"selected_target"`
	State          State          `json:"state"`
	Analysis       *Analysis      `json:"analysis,omitempty"`
	Failure        *Failure       `json:"failure,omitempty"`
}

// NewAnalyzed builds one analyzed outcome without retaining caller-owned
// ProgramTarget slices.
func NewAnalyzed(selected SelectedTarget, target programindex.Target, runID string) (Outcome, error) {
	outcome := Outcome{
		SelectedTarget: selected,
		State:          StateAnalyzed,
		Analysis:       &Analysis{ProgramTarget: target.Snapshot(), RunID: runID},
	}
	if err := outcome.Validate(); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

// NewNotAnalyzed builds one failed outcome from closed public classifications.
func NewNotAnalyzed(selected SelectedTarget, stage Stage, reason Reason) (Outcome, error) {
	outcome := Outcome{
		SelectedTarget: selected,
		State:          StateNotAnalyzed,
		Failure:        &Failure{Stage: stage, Reason: reason},
	}
	if err := outcome.Validate(); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

// Snapshot returns a consumer-owned copy, including nested ProgramTarget
// storage and tagged-union pointers.
func (outcome Outcome) Snapshot() Outcome {
	result := outcome
	if outcome.Analysis != nil {
		analysis := *outcome.Analysis
		analysis.ProgramTarget = analysis.ProgramTarget.Snapshot()
		result.Analysis = &analysis
	}
	if outcome.Failure != nil {
		failure := *outcome.Failure
		result.Failure = &failure
	}
	return result
}

// Validate checks one standalone tagged outcome.
func (outcome Outcome) Validate() error {
	if err := outcome.SelectedTarget.Validate(); err != nil {
		return err
	}
	if !outcome.State.Valid() {
		return fmt.Errorf("target outcome portfolio: invalid outcome state")
	}
	switch outcome.State {
	case StateAnalyzed:
		if outcome.Analysis == nil || outcome.Failure != nil {
			return fmt.Errorf("target outcome portfolio: analyzed outcome has invalid binding")
		}
		if err := outcome.Analysis.ProgramTarget.Validate(); err != nil {
			return fmt.Errorf("target outcome portfolio: analyzed program target: %w", err)
		}
		if !selectedLanguageMatches(outcome.SelectedTarget.LanguageGroup, outcome.Analysis.ProgramTarget.Language) {
			return fmt.Errorf("target outcome portfolio: analyzed program target language mismatch")
		}
		if err := programpage.ValidateRunID(outcome.Analysis.RunID); err != nil {
			return fmt.Errorf("target outcome portfolio: analyzed run id: %w", err)
		}
	case StateNotAnalyzed:
		if outcome.Analysis != nil || outcome.Failure == nil ||
			!outcome.Failure.Stage.Valid() || !outcome.Failure.Reason.Valid() {
			return fmt.Errorf("target outcome portfolio: not-analyzed outcome has invalid failure")
		}
	}
	return nil
}

// Portfolio is the complete result set for all selected targets. The selected
// default is explicit and remains authoritative even when its outcome failed.
type Portfolio struct {
	Version                 int       `json:"version"`
	DefaultSelectedTargetID string    `json:"default_selected_target_id"`
	Outcomes                []Outcome `json:"outcomes"`
	SHA256                  string    `json:"sha256"`
}

// Build canonicalizes and seals every selected-target outcome. At least one
// selected target is required, but zero analyzed targets is legitimate.
func Build(defaultSelectedTargetID string, outcomes []Outcome) (Portfolio, error) {
	if len(outcomes) == 0 || len(outcomes) > MaxOutcomes {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: outcome bound exceeded")
	}
	portfolio := Portfolio{
		Version:                 Version,
		DefaultSelectedTargetID: defaultSelectedTargetID,
		Outcomes:                cloneOutcomes(outcomes),
	}
	sort.Slice(portfolio.Outcomes, func(i, j int) bool {
		return portfolio.Outcomes[i].SelectedTarget.ID < portfolio.Outcomes[j].SelectedTarget.ID
	})
	if err := portfolio.validateShape(); err != nil {
		return Portfolio{}, err
	}
	digest, err := portfolioDigest(portfolio)
	if err != nil {
		return Portfolio{}, err
	}
	portfolio.SHA256 = digest
	if err := portfolio.Validate(); err != nil {
		return Portfolio{}, err
	}
	if _, err := portfolio.CanonicalJSON(); err != nil {
		return Portfolio{}, err
	}
	return portfolio, nil
}

// Snapshot returns a consumer-owned deep copy.
func (portfolio Portfolio) Snapshot() Portfolio {
	result := portfolio
	result.Outcomes = cloneOutcomes(portfolio.Outcomes)
	return result
}

// Validate checks canonical order, the exact selected default, unique page
// bindings, the bounded envelope, and the artifact self-seal.
func (portfolio Portfolio) Validate() error {
	if err := portfolio.validateShape(); err != nil {
		return err
	}
	want, err := portfolioDigest(portfolio)
	if err != nil {
		return err
	}
	if !validSHA256(portfolio.SHA256) || portfolio.SHA256 != want {
		return fmt.Errorf("target outcome portfolio: sha256 mismatch")
	}
	return nil
}

// CanonicalJSON returns the exact compact bytes suitable for persistence.
func (portfolio Portfolio) CanonicalJSON() ([]byte, error) {
	if err := portfolio.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(portfolio)
	if err != nil {
		return nil, fmt.Errorf("target outcome portfolio: encode artifact: %w", err)
	}
	if len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("target outcome portfolio: artifact exceeds bounded envelope")
	}
	return encoded, nil
}

// Decode accepts only exact canonical artifact bytes. Unknown fields,
// trailing values, alternate whitespace, invalid union members, and seal
// tampering fail closed.
func Decode(encoded []byte) (Portfolio, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var portfolio Portfolio
	if err := decoder.Decode(&portfolio); err != nil {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Portfolio{}, fmt.Errorf("target outcome portfolio: trailing JSON value")
		}
		return Portfolio{}, fmt.Errorf("target outcome portfolio: trailing data: %w", err)
	}
	if err := portfolio.Validate(); err != nil {
		return Portfolio{}, err
	}
	canonical, err := portfolio.CanonicalJSON()
	if err != nil {
		return Portfolio{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: artifact is not canonical")
	}
	return portfolio, nil
}

func (portfolio Portfolio) validateShape() error {
	if portfolio.Version != Version || !validSelectedTargetID(portfolio.DefaultSelectedTargetID) {
		return fmt.Errorf("target outcome portfolio: invalid identity")
	}
	if portfolio.Outcomes == nil || len(portfolio.Outcomes) == 0 || len(portfolio.Outcomes) > MaxOutcomes {
		return fmt.Errorf("target outcome portfolio: outcome bound exceeded")
	}
	defaultMatches := 0
	programTargetIDs := make(map[string]struct{}, len(portfolio.Outcomes))
	runIDs := make(map[string]struct{}, len(portfolio.Outcomes))
	for position, outcome := range portfolio.Outcomes {
		if err := outcome.Validate(); err != nil {
			return fmt.Errorf("target outcome portfolio: outcome %d: %w", position, err)
		}
		selectedID := outcome.SelectedTarget.ID
		if position > 0 && portfolio.Outcomes[position-1].SelectedTarget.ID >= selectedID {
			return fmt.Errorf("target outcome portfolio: outcomes are not canonical")
		}
		if selectedID == portfolio.DefaultSelectedTargetID {
			defaultMatches++
		}
		if outcome.Analysis == nil {
			continue
		}
		programTargetID := outcome.Analysis.ProgramTarget.ID
		if _, duplicate := programTargetIDs[programTargetID]; duplicate {
			return fmt.Errorf("target outcome portfolio: duplicate analyzed program target")
		}
		programTargetIDs[programTargetID] = struct{}{}
		if _, duplicate := runIDs[outcome.Analysis.RunID]; duplicate {
			return fmt.Errorf("target outcome portfolio: duplicate analyzed run id")
		}
		runIDs[outcome.Analysis.RunID] = struct{}{}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("target outcome portfolio: default selected target must match exactly one outcome")
	}
	return nil
}

func portfolioDigest(portfolio Portfolio) (string, error) {
	payload := portfolio.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("target outcome portfolio: encode digest material: %w", err)
	}
	if len(encoded)+sha256.Size*2 > MaxArtifactBytes {
		return "", fmt.Errorf("target outcome portfolio: canonical substrate exceeds bounded envelope")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(digestDomain))
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func selectedTargetID(target SelectedTarget) string {
	digest := sha256.New()
	for _, field := range []string{
		"selected-target",
		string(target.LanguageGroup),
		string(target.ScopeKind),
		target.DisplayName,
		target.Selector,
	} {
		_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(field))
	}
	return "selected-target-" + hex.EncodeToString(digest.Sum(nil))
}

func cloneOutcomes(outcomes []Outcome) []Outcome {
	if outcomes == nil {
		return nil
	}
	result := make([]Outcome, len(outcomes))
	for position, outcome := range outcomes {
		result[position] = outcome.Snapshot()
	}
	return result
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > programindex.MaxTextBytes ||
		!utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSelectedTargetID(value string) bool {
	const prefix = "selected-target-"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	return validSHA256(strings.TrimPrefix(value, prefix))
}

func selectedLanguageMatches(group LanguageGroup, language string) bool {
	switch group {
	case LanguageGroupGo:
		return language == "go"
	case LanguageGroupPython:
		return language == "python"
	case LanguageGroupJavaScriptTypeScript:
		return language == "javascript" || language == "typescript"
	default:
		return false
	}
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
