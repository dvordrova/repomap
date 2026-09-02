package report

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

const ProgramPortfolioVersion = 2

// ProgramPortfolio is the complete browser-facing projection of the selected
// language-neutral ProgramIndex artifact set. It never chooses, drops, or
// repairs a target: every set entry has one exact Target/View pair.
type ProgramPortfolio struct {
	Version         int                     `json:"version"`
	DefaultTargetID string                  `json:"default_target_id"`
	Entries         []ProgramPortfolioEntry `json:"entries"`
}

type ProgramPortfolioEntry struct {
	Target programindex.Target `json:"target"`
	View   ProgramView         `json:"view"`
}

// NewProgramPortfolio projects every validated selected index and preserves
// the artifact-set default by exact ProgramTarget ID.
func NewProgramPortfolio(defaultTargetID string, indexes []programindex.Index) (*ProgramPortfolio, error) {
	if len(indexes) == 0 {
		return nil, fmt.Errorf("program portfolio: entries are empty")
	}
	result := &ProgramPortfolio{
		Version: ProgramPortfolioVersion, DefaultTargetID: defaultTargetID,
		Entries: make([]ProgramPortfolioEntry, 0, len(indexes)),
	}
	for _, index := range indexes {
		view, err := NewProgramView(index)
		if err != nil {
			return nil, fmt.Errorf("program portfolio: project target %q: %w", index.Target.ID, err)
		}
		result.Entries = append(result.Entries, ProgramPortfolioEntry{
			Target: index.Target.Snapshot(),
			View:   *view,
		})
	}
	sort.Slice(result.Entries, func(left, right int) bool {
		return result.Entries[left].Target.ID < result.Entries[right].Target.ID
	})
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

func (portfolio ProgramPortfolio) Validate() error {
	if portfolio.Version != ProgramPortfolioVersion ||
		!validProgramViewText(portfolio.DefaultTargetID) ||
		portfolio.Entries == nil || len(portfolio.Entries) == 0 {
		return fmt.Errorf("program portfolio: invalid identity or empty entries")
	}
	defaultMatches := 0
	previousID := ""
	for _, entry := range portfolio.Entries {
		if err := entry.Target.Validate(); err != nil {
			return fmt.Errorf("program portfolio: target: %w", err)
		}
		if err := entry.View.Validate(); err != nil {
			return fmt.Errorf("program portfolio: view for %q: %w", entry.Target.ID, err)
		}
		if entry.View.TargetID != entry.Target.ID {
			return fmt.Errorf("program portfolio: target/view identity mismatch")
		}
		if previousID != "" && previousID >= entry.Target.ID {
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

func (portfolio ProgramPortfolio) defaultEntry() (ProgramPortfolioEntry, error) {
	if err := portfolio.Validate(); err != nil {
		return ProgramPortfolioEntry{}, err
	}
	for _, entry := range portfolio.Entries {
		if entry.Target.ID == portfolio.DefaultTargetID {
			return entry, nil
		}
	}
	return ProgramPortfolioEntry{}, fmt.Errorf("program portfolio: default target is missing")
}
