package report

import (
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// TargetOutcomePortfolioView is the compact browser-facing projection of the
// sealed result inventory for every selected repository target. Full
// ProgramTarget values and child run IDs stay in the canonical artifacts.
type TargetOutcomePortfolioView struct {
	Version                 int                 `json:"version"`
	DefaultSelectedTargetID string              `json:"default_selected_target_id"`
	Outcomes                []TargetOutcomeView `json:"outcomes"`
}

// TargetOutcomeView carries only the public selected-target identity and the
// closed outcome union. An analyzed row joins the existing target navigation
// by ProgramTarget ID; a failed row carries only a sanitized stage and reason.
type TargetOutcomeView struct {
	SelectedTargetID        string                      `json:"selected_target_id"`
	Language                targetoutcome.LanguageGroup `json:"language"`
	AllowedProgramLanguages []string                    `json:"allowed_program_languages"`
	ScopeKind               targetoutcome.ScopeKind     `json:"scope_kind"`
	DisplayName             string                      `json:"display_name"`
	Selector                string                      `json:"selector"`
	State                   targetoutcome.State         `json:"state"`
	ProgramTargetID         string                      `json:"program_target_id,omitempty"`
	FailureStage            targetoutcome.Stage         `json:"failure_stage,omitempty"`
	FailureReason           targetoutcome.Reason        `json:"failure_reason,omitempty"`
}

// NewTargetOutcomePortfolioView verifies the exact analyzed-outcome/page
// bijection before removing full ProgramTarget and run identity from the
// browser projection.
func NewTargetOutcomePortfolioView(
	outcomes targetoutcome.Portfolio,
	pages programpage.Portfolio,
) (*TargetOutcomePortfolioView, error) {
	if err := outcomes.Validate(); err != nil {
		return nil, fmt.Errorf("target outcome portfolio view: outcome artifact: %w", err)
	}
	if err := pages.Validate(); err != nil {
		return nil, fmt.Errorf("target outcome portfolio view: program page artifact: %w", err)
	}

	pageByTargetID := make(map[string]programpage.Page, len(pages.Pages))
	for _, page := range pages.Pages {
		pageByTargetID[page.Target.ID] = page
	}
	view := &TargetOutcomePortfolioView{
		Version:                 outcomes.Version,
		DefaultSelectedTargetID: outcomes.DefaultSelectedTargetID,
		Outcomes:                make([]TargetOutcomeView, 0, len(outcomes.Outcomes)),
	}
	analyzedCount := 0
	defaultAnalyzedTargetID := ""
	for _, outcome := range outcomes.Outcomes {
		row := TargetOutcomeView{
			SelectedTargetID: outcome.SelectedTarget.ID,
			Language:         outcome.SelectedTarget.LanguageGroup,
			AllowedProgramLanguages: append(
				[]string(nil), outcome.SelectedTarget.AllowedProgramLanguages...,
			),
			ScopeKind:   outcome.SelectedTarget.ScopeKind,
			DisplayName: outcome.SelectedTarget.DisplayName,
			Selector:    outcome.SelectedTarget.Selector,
			State:       outcome.State,
		}
		switch outcome.State {
		case targetoutcome.StateAnalyzed:
			analyzedCount++
			page, present := pageByTargetID[outcome.Analysis.ProgramTarget.ID]
			if !present || page.RunID != outcome.Analysis.RunID ||
				!reflect.DeepEqual(page.Target, outcome.Analysis.ProgramTarget) {
				return nil, fmt.Errorf("target outcome portfolio view: analyzed outcome has no exact program page")
			}
			delete(pageByTargetID, outcome.Analysis.ProgramTarget.ID)
			row.ProgramTargetID = outcome.Analysis.ProgramTarget.ID
			if outcome.SelectedTarget.ID == outcomes.DefaultSelectedTargetID {
				defaultAnalyzedTargetID = row.ProgramTargetID
			}
		case targetoutcome.StateNotAnalyzed:
			row.FailureStage = outcome.Failure.Stage
			row.FailureReason = outcome.Failure.Reason
		}
		view.Outcomes = append(view.Outcomes, row)
	}
	if analyzedCount != len(pages.Pages) || len(pageByTargetID) != 0 {
		return nil, fmt.Errorf("target outcome portfolio view: analyzed outcomes do not exactly cover program pages")
	}
	if defaultAnalyzedTargetID != "" && defaultAnalyzedTargetID != pages.DefaultTargetID {
		return nil, fmt.Errorf("target outcome portfolio view: analyzed selected default does not own the program page default")
	}
	if err := view.Validate(); err != nil {
		return nil, err
	}
	return view, nil
}

// Validate checks the standalone compact shape. Exact ProgramTarget and run
// equality is re-derived from the two manifest-bound artifacts.
func (view TargetOutcomePortfolioView) Validate() error {
	if view.Version != targetoutcome.Version || view.Outcomes == nil ||
		len(view.Outcomes) == 0 {
		return fmt.Errorf("target outcome portfolio view: invalid identity")
	}
	defaultMatches := 0
	programTargetIDs := make(map[string]struct{}, len(view.Outcomes))
	previousSelectedTargetID := ""
	for index, outcome := range view.Outcomes {
		selected := targetoutcome.SelectedTarget{
			ID: outcome.SelectedTargetID, LanguageGroup: outcome.Language,
			AllowedProgramLanguages: append([]string(nil), outcome.AllowedProgramLanguages...),
			ScopeKind:               outcome.ScopeKind, DisplayName: outcome.DisplayName, Selector: outcome.Selector,
		}
		if err := selected.Validate(); err != nil {
			return fmt.Errorf("target outcome portfolio view: outcome %d selected target: %w", index, err)
		}
		if previousSelectedTargetID != "" && previousSelectedTargetID >= outcome.SelectedTargetID {
			return fmt.Errorf("target outcome portfolio view: outcomes are not canonical")
		}
		previousSelectedTargetID = outcome.SelectedTargetID
		if outcome.SelectedTargetID == view.DefaultSelectedTargetID {
			defaultMatches++
		}
		if !outcome.State.Valid() {
			return fmt.Errorf("target outcome portfolio view: outcome %d state is invalid", index)
		}
		switch outcome.State {
		case targetoutcome.StateAnalyzed:
			if !validTargetNavigationText(outcome.ProgramTargetID) ||
				outcome.FailureStage != "" || outcome.FailureReason != "" {
				return fmt.Errorf("target outcome portfolio view: analyzed outcome %d is invalid", index)
			}
			if _, duplicate := programTargetIDs[outcome.ProgramTargetID]; duplicate {
				return fmt.Errorf("target outcome portfolio view: duplicate analyzed program target")
			}
			programTargetIDs[outcome.ProgramTargetID] = struct{}{}
		case targetoutcome.StateNotAnalyzed:
			if outcome.ProgramTargetID != "" || !outcome.FailureStage.Valid() ||
				!outcome.FailureReason.Valid() {
				return fmt.Errorf("target outcome portfolio view: not-analyzed outcome %d is invalid", index)
			}
		}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("target outcome portfolio view: selected default must match exactly one outcome")
	}
	return nil
}

func (view TargetOutcomePortfolioView) validateTargetNavigation(
	navigation *TargetNavigationPortfolio,
) error {
	if navigation == nil {
		return fmt.Errorf("target outcome portfolio view: target navigation is missing")
	}
	analyzed := make(map[string]struct{})
	defaultAnalyzedTargetID := ""
	for _, outcome := range view.Outcomes {
		if outcome.State != targetoutcome.StateAnalyzed {
			continue
		}
		analyzed[outcome.ProgramTargetID] = struct{}{}
		if outcome.SelectedTargetID == view.DefaultSelectedTargetID {
			defaultAnalyzedTargetID = outcome.ProgramTargetID
		}
	}
	if len(analyzed) != len(navigation.Targets) {
		return fmt.Errorf("target outcome portfolio view: analyzed targets do not cover target navigation")
	}
	for _, target := range navigation.Targets {
		if _, present := analyzed[target.TargetID]; !present {
			return fmt.Errorf("target outcome portfolio view: target navigation contains an unknown analyzed target")
		}
	}
	if defaultAnalyzedTargetID != "" && defaultAnalyzedTargetID != navigation.DefaultTargetID {
		return fmt.Errorf("target outcome portfolio view: analyzed selected default does not own the navigation default")
	}
	return nil
}

func restoreTargetOutcomePortfolioView(runDir string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: target outcome portfolio report data is missing")
	}
	outcomeRaw, outcomePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, targetoutcome.ArtifactFilename),
		targetoutcome.MaxArtifactBytes,
		"target outcome portfolio",
		true,
	)
	if err != nil {
		return err
	}
	pageRaw, pagePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, programpage.ArtifactFilename),
		programpage.MaxArtifactBytes,
		"program page portfolio for target outcomes",
		true,
	)
	if err != nil {
		return err
	}
	if outcomePresent != pagePresent {
		return fmt.Errorf("report: target outcome and program page portfolio authority must be published together")
	}
	if !outcomePresent {
		if data.TargetOutcomePortfolio != nil {
			return fmt.Errorf("report: unbound target outcome portfolio projection is present")
		}
		return nil
	}
	outcomes, err := targetoutcome.Decode(outcomeRaw)
	if err != nil {
		return fmt.Errorf("report: decode target outcome portfolio: %w", err)
	}
	pages, err := programpage.Decode(pageRaw)
	if err != nil {
		return fmt.Errorf("report: decode target outcome program page portfolio: %w", err)
	}
	view, err := NewTargetOutcomePortfolioView(outcomes, pages)
	if err != nil {
		return fmt.Errorf("report: project target outcome portfolio: %w", err)
	}
	if data.TargetOutcomePortfolio != nil && !reflect.DeepEqual(data.TargetOutcomePortfolio, view) {
		return fmt.Errorf("report: target outcome portfolio projection conflicts with artifacts")
	}
	data.TargetOutcomePortfolio = view
	return nil
}
