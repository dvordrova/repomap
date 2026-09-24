package report

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

const ProgramPortfolioVersion = 4

// ProgramPortfolio is the complete selected ProgramIndex set. The report uses
// the same indexes as every other consumer; it does not copy them into a
// presentation-specific graph.
type ProgramPortfolio struct {
	Version         int                  `json:"version"`
	DefaultTargetID string               `json:"default_target_id"`
	Entries         []programindex.Index `json:"entries"`
}

// NewProgramPortfolio owns every validated selected index and preserves the
// artifact-set default by exact ProgramTarget ID.
func NewProgramPortfolio(defaultTargetID string, indexes []programindex.Index) (*ProgramPortfolio, error) {
	if len(indexes) == 0 {
		return nil, fmt.Errorf("program portfolio: entries are empty")
	}
	result := &ProgramPortfolio{
		Version: ProgramPortfolioVersion, DefaultTargetID: defaultTargetID,
		Entries: make([]programindex.Index, 0, len(indexes)),
	}
	for _, index := range indexes {
		result.Entries = append(result.Entries, index.Snapshot())
	}
	sort.Slice(result.Entries, func(left, right int) bool {
		return programindex.TargetIDLess(result.Entries[left].Target.ID, result.Entries[right].Target.ID)
	})
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// BindProgramPortfolio installs the complete target set into a repository
// report. The same ProgramIndexes then back both persistence and rendering.
func BindProgramPortfolio(data *ReportData, defaultTargetID string, indexes []programindex.Index) error {
	if data == nil {
		return fmt.Errorf("program portfolio: report data is missing")
	}
	portfolio, err := NewProgramPortfolio(defaultTargetID, indexes)
	if err != nil {
		return err
	}
	data.ProgramPortfolio = portfolio
	data.programIndexes = make([]programindex.Index, len(portfolio.Entries))
	copy(data.programIndexes, portfolio.Entries)
	data.defaultProgramIndex = nil
	for position := range data.programIndexes {
		if data.programIndexes[position].Target.ID == defaultTargetID {
			data.defaultProgramIndex = &data.programIndexes[position]
			break
		}
	}
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("program portfolio: default ProgramIndex is missing")
	}
	return nil
}

func (portfolio ProgramPortfolio) Validate() error {
	if portfolio.Version != ProgramPortfolioVersion ||
		!programindex.ValidTargetID(portfolio.DefaultTargetID) ||
		portfolio.Entries == nil || len(portfolio.Entries) == 0 {
		return fmt.Errorf("program portfolio: invalid identity or empty entries")
	}
	defaultMatches := 0
	previousID := ""
	for _, entry := range portfolio.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("program portfolio: target %q: %w", entry.Target.ID, err)
		}
		if previousID != "" && !programindex.TargetIDLess(previousID, entry.Target.ID) {
			return fmt.Errorf("program portfolio: entries are not canonical")
		}
		previousID = entry.Target.ID
		if entry.Target.ID == portfolio.DefaultTargetID {
			defaultMatches++
		}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("program portfolio: default target must have exactly one entry")
	}
	return nil
}

// defaultEntry looks the default target up. It does not validate the entries
// again: a portfolio is validated where it is built (NewProgramPortfolio) and
// where a publication or render boundary receives it
// (validateProgramPresentation), and every lookup runs inside one of those.
func (portfolio ProgramPortfolio) defaultEntry() (programindex.Index, error) {
	for _, entry := range portfolio.Entries {
		if entry.Target.ID == portfolio.DefaultTargetID {
			return entry, nil
		}
	}
	return programindex.Index{}, fmt.Errorf("program portfolio: default target is missing")
}
