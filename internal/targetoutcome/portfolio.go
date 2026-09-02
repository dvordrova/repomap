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
	Version          = 2
	ArtifactFilename = "target-outcome-portfolio.json"

	MaxOutcomes           = 4_096
	AdvisoryArtifactBytes = 8 * 1024 * 1024
	// MaxArtifactBytes is a compatibility sentinel for report readers. Zero
	// means the complete validated local artifact is read without a byte cutoff.
	MaxArtifactBytes           = 0
	MaxAllowedProgramLanguages = 16
)

const (
	digestDomain       = "target-outcome-portfolio-v2\x00"
	legacyDigestDomain = "target-outcome-portfolio-v1\x00"
)

// LanguageGroup is the bounded public adapter family that owns a selected
// target. The separately persisted AllowedProgramLanguages set is the exact
// materialization authority.
type LanguageGroup string

const (
	LanguageGroupGo                   LanguageGroup = "go"
	LanguageGroupPython               LanguageGroup = "python"
	LanguageGroupJavaScriptTypeScript LanguageGroup = "javascript_typescript"
)

func (group LanguageGroup) Valid() bool {
	return validExactText(string(group))
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
	ID                      string        `json:"id"`
	LanguageGroup           LanguageGroup `json:"language_group"`
	AllowedProgramLanguages []string      `json:"allowed_program_languages"`
	ScopeKind               ScopeKind     `json:"scope_kind"`
	DisplayName             string        `json:"display_name"`
	Selector                string        `json:"selector"`
}

// NewSelectedTarget validates and seals the exact public selected-target
// tuple. Its identity changes when any tuple field changes.
func NewSelectedTarget(
	languageGroup LanguageGroup,
	scopeKind ScopeKind,
	displayName string,
	selector string,
) (SelectedTarget, error) {
	return NewSelectedTargetWithLanguages(
		languageGroup, defaultAllowedProgramLanguages(languageGroup),
		scopeKind, displayName, selector,
	)
}

// NewSelectedTargetWithLanguages binds an adapter family to the exact
// ProgramIndex languages it may materialize. Report code checks membership in
// this identity-bound set instead of registering language families itself.
func NewSelectedTargetWithLanguages(
	languageGroup LanguageGroup,
	allowedProgramLanguages []string,
	scopeKind ScopeKind,
	displayName string,
	selector string,
) (SelectedTarget, error) {
	target := SelectedTarget{
		LanguageGroup: languageGroup, AllowedProgramLanguages: canonicalLanguages(allowedProgramLanguages),
		ScopeKind: scopeKind, DisplayName: displayName, Selector: selector,
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

// Snapshot returns a consumer-owned copy of the selected target contract.
func (target SelectedTarget) Snapshot() SelectedTarget {
	result := target
	result.AllowedProgramLanguages = append([]string(nil), target.AllowedProgramLanguages...)
	return result
}

func (target SelectedTarget) validateShape() error {
	if !target.LanguageGroup.Valid() || !target.ScopeKind.Valid() ||
		!validExactText(target.DisplayName) || !validExactText(target.Selector) ||
		len(target.AllowedProgramLanguages) == 0 ||
		!canonicalTextSet(target.AllowedProgramLanguages) {
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
		SelectedTarget: selected.Snapshot(),
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
		SelectedTarget: selected.Snapshot(),
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
	result.SelectedTarget = outcome.SelectedTarget.Snapshot()
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
		if !selectedLanguageMatches(outcome.SelectedTarget.AllowedProgramLanguages, outcome.Analysis.ProgramTarget.Language) {
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
	if len(outcomes) == 0 {
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
	return encoded, nil
}

// Decode accepts only exact canonical artifact bytes. Unknown fields,
// trailing values, alternate whitespace, invalid union members, and seal
// tampering fail closed.
func Decode(encoded []byte) (Portfolio, error) {
	if len(encoded) == 0 {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: invalid artifact size")
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: decode version: %w", err)
	}
	if header.Version == 1 {
		return decodeLegacyPortfolio(encoded)
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

// The v1 wire contract had a closed language group and no explicit allowed
// ProgramIndex language set. Decode verifies its exact old shape and seal,
// then returns a newly sealed v2 value. No v1 authority is guessed: the only
// migration mapping is the same closed mapping v1 itself enforced.
type legacySelectedTarget struct {
	ID            string        `json:"id"`
	LanguageGroup LanguageGroup `json:"language_group"`
	ScopeKind     ScopeKind     `json:"scope_kind"`
	DisplayName   string        `json:"display_name"`
	Selector      string        `json:"selector"`
}

type legacyOutcome struct {
	SelectedTarget legacySelectedTarget `json:"selected_target"`
	State          State                `json:"state"`
	Analysis       *Analysis            `json:"analysis,omitempty"`
	Failure        *Failure             `json:"failure,omitempty"`
}

type legacyPortfolio struct {
	Version                 int             `json:"version"`
	DefaultSelectedTargetID string          `json:"default_selected_target_id"`
	Outcomes                []legacyOutcome `json:"outcomes"`
	SHA256                  string          `json:"sha256"`
}

func decodeLegacyPortfolio(encoded []byte) (Portfolio, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var legacy legacyPortfolio
	if err := decoder.Decode(&legacy); err != nil {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: decode v1 artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: invalid v1 trailing data")
	}
	if err := validateLegacyPortfolio(legacy); err != nil {
		return Portfolio{}, err
	}
	canonical, err := json.Marshal(legacy)
	if err != nil {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: encode v1 canonical bytes: %w", err)
	}
	if !bytes.Equal(encoded, canonical) {
		return Portfolio{}, fmt.Errorf("target outcome portfolio: v1 artifact is not canonical")
	}

	newOutcomes := make([]Outcome, 0, len(legacy.Outcomes))
	defaultID := ""
	for _, oldOutcome := range legacy.Outcomes {
		selected, err := NewSelectedTarget(
			oldOutcome.SelectedTarget.LanguageGroup,
			oldOutcome.SelectedTarget.ScopeKind,
			oldOutcome.SelectedTarget.DisplayName,
			oldOutcome.SelectedTarget.Selector,
		)
		if err != nil {
			return Portfolio{}, fmt.Errorf("target outcome portfolio: migrate v1 selected target: %w", err)
		}
		if oldOutcome.SelectedTarget.ID == legacy.DefaultSelectedTargetID {
			defaultID = selected.ID
		}
		var migrated Outcome
		if oldOutcome.State == StateAnalyzed {
			migrated, err = NewAnalyzed(
				selected, oldOutcome.Analysis.ProgramTarget, oldOutcome.Analysis.RunID,
			)
		} else {
			migrated, err = NewNotAnalyzed(
				selected, oldOutcome.Failure.Stage, oldOutcome.Failure.Reason,
			)
		}
		if err != nil {
			return Portfolio{}, fmt.Errorf("target outcome portfolio: migrate v1 outcome: %w", err)
		}
		newOutcomes = append(newOutcomes, migrated)
	}
	return Build(defaultID, newOutcomes)
}

func validateLegacyPortfolio(portfolio legacyPortfolio) error {
	if portfolio.Version != 1 || !validSelectedTargetID(portfolio.DefaultSelectedTargetID) ||
		portfolio.Outcomes == nil || len(portfolio.Outcomes) == 0 {
		return fmt.Errorf("target outcome portfolio: invalid v1 identity")
	}
	defaultMatches := 0
	programTargets := make(map[string]struct{}, len(portfolio.Outcomes))
	runIDs := make(map[string]struct{}, len(portfolio.Outcomes))
	previousID := ""
	for position, outcome := range portfolio.Outcomes {
		selected := outcome.SelectedTarget
		if !legacyLanguageGroupValid(selected.LanguageGroup) || !selected.ScopeKind.Valid() ||
			!validExactText(selected.DisplayName) || !validExactText(selected.Selector) ||
			selected.ID != legacySelectedTargetID(selected) {
			return fmt.Errorf("target outcome portfolio: invalid v1 selected target %d", position)
		}
		if previousID != "" && previousID >= selected.ID {
			return fmt.Errorf("target outcome portfolio: v1 outcomes are not canonical")
		}
		previousID = selected.ID
		if selected.ID == portfolio.DefaultSelectedTargetID {
			defaultMatches++
		}
		switch outcome.State {
		case StateAnalyzed:
			if outcome.Analysis == nil || outcome.Failure != nil ||
				outcome.Analysis.ProgramTarget.Validate() != nil ||
				!legacySelectedLanguageMatches(selected.LanguageGroup, outcome.Analysis.ProgramTarget.Language) ||
				programpage.ValidateRunID(outcome.Analysis.RunID) != nil {
				return fmt.Errorf("target outcome portfolio: invalid v1 analyzed outcome %d", position)
			}
			if _, duplicate := programTargets[outcome.Analysis.ProgramTarget.ID]; duplicate {
				return fmt.Errorf("target outcome portfolio: duplicate v1 program target")
			}
			if _, duplicate := runIDs[outcome.Analysis.RunID]; duplicate {
				return fmt.Errorf("target outcome portfolio: duplicate v1 run id")
			}
			programTargets[outcome.Analysis.ProgramTarget.ID] = struct{}{}
			runIDs[outcome.Analysis.RunID] = struct{}{}
		case StateNotAnalyzed:
			if outcome.Analysis != nil || outcome.Failure == nil ||
				!outcome.Failure.Stage.Valid() || !outcome.Failure.Reason.Valid() {
				return fmt.Errorf("target outcome portfolio: invalid v1 failed outcome %d", position)
			}
		default:
			return fmt.Errorf("target outcome portfolio: invalid v1 state")
		}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("target outcome portfolio: invalid v1 default")
	}
	want, err := legacyPortfolioDigest(portfolio)
	if err != nil {
		return err
	}
	if !validSHA256(portfolio.SHA256) || portfolio.SHA256 != want {
		return fmt.Errorf("target outcome portfolio: v1 sha256 mismatch")
	}
	return nil
}

func legacyPortfolioDigest(portfolio legacyPortfolio) (string, error) {
	portfolio.SHA256 = ""
	encoded, err := json.Marshal(portfolio)
	if err != nil {
		return "", fmt.Errorf("target outcome portfolio: encode v1 digest material: %w", err)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(legacyDigestDomain))
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func legacySelectedTargetID(target legacySelectedTarget) string {
	digest := sha256.New()
	for _, field := range []string{
		"selected-target", string(target.LanguageGroup), string(target.ScopeKind),
		target.DisplayName, target.Selector,
	} {
		_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(field))
	}
	return "selected-target-" + hex.EncodeToString(digest.Sum(nil))
}

func legacyLanguageGroupValid(group LanguageGroup) bool {
	return group == LanguageGroupGo || group == LanguageGroupPython ||
		group == LanguageGroupJavaScriptTypeScript
}

func legacySelectedLanguageMatches(group LanguageGroup, language string) bool {
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

func (portfolio Portfolio) validateShape() error {
	if portfolio.Version != Version || !validSelectedTargetID(portfolio.DefaultSelectedTargetID) {
		return fmt.Errorf("target outcome portfolio: invalid identity")
	}
	if portfolio.Outcomes == nil || len(portfolio.Outcomes) == 0 {
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
	for _, language := range target.AllowedProgramLanguages {
		_, _ = digest.Write([]byte(strconv.Itoa(len(language))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(language))
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

func validExactText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
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

func selectedLanguageMatches(allowed []string, language string) bool {
	position := sort.SearchStrings(allowed, language)
	return position < len(allowed) && allowed[position] == language
}

func defaultAllowedProgramLanguages(group LanguageGroup) []string {
	if group == LanguageGroupJavaScriptTypeScript {
		return []string{"javascript", "typescript"}
	}
	if group.Valid() {
		return []string{string(group)}
	}
	return nil
}

func canonicalLanguages(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if len(result) < 2 {
		return result
	}
	compacted := result[:1]
	for _, value := range result[1:] {
		if compacted[len(compacted)-1] != value {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func canonicalTextSet(values []string) bool {
	for position, value := range values {
		if !validExactText(value) || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
