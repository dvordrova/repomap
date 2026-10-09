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
	files           []programFile
}

// NewProgramPortfolio owns every validated selected index and preserves the
// artifact-set default by exact ProgramTarget ID.
func NewProgramPortfolio(defaultTargetID string, indexes []programindex.Index) (*ProgramPortfolio, error) {
	return newProgramPortfolio(defaultTargetID, len(indexes), func(position int) (programindex.Index, error) {
		return indexes[position], nil
	})
}

// newProgramPortfolio owns one index before reading the next. Public input
// values remain isolated without retaining a second complete native set.
func newProgramPortfolio(defaultTargetID string, count int, read func(int) (programindex.Index, error)) (*ProgramPortfolio, error) {
	if count == 0 {
		return nil, fmt.Errorf("program portfolio: entries are empty")
	}
	result := &ProgramPortfolio{
		Version: ProgramPortfolioVersion, DefaultTargetID: defaultTargetID,
		Entries: make([]programindex.Index, 0, count),
	}
	for position := 0; position < count; position++ {
		index, err := read(position)
		if err != nil {
			return nil, err
		}
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
	return bindProgramPortfolio(data, portfolio)
}

// BindProgramPortfolioReaders installs every original sealed target, reading
// and owning one at a time. No loaded original set remains beside the report's
// complete native portfolio. Reader failures refuse the binding; existing
// report data is unchanged until the whole portfolio is validated.
func BindProgramPortfolioReaders(data *ReportData, defaultTargetID string, readers []func() (programindex.Index, error)) error {
	if data == nil {
		return fmt.Errorf("program portfolio: report data is missing")
	}
	portfolio, err := newProgramPortfolio(defaultTargetID, len(readers), func(position int) (programindex.Index, error) {
		if readers[position] == nil {
			return programindex.Index{}, fmt.Errorf("program portfolio: index reader %d is missing", position)
		}
		return readers[position]()
	})
	if err != nil {
		return err
	}
	return bindProgramPortfolio(data, portfolio)
}

func bindProgramPortfolio(data *ReportData, portfolio *ProgramPortfolio) error {
	data.ProgramPortfolio = portfolio
	data.programIndexes = make([]programindex.Index, len(portfolio.Entries))
	copy(data.programIndexes, portfolio.Entries)
	data.defaultProgramIndex = nil
	for position := range data.programIndexes {
		if data.programIndexes[position].Target.ID == portfolio.DefaultTargetID {
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
	if len(portfolio.files) > 0 {
		if err := portfolio.validateBindings(); err != nil {
			return err
		}
		// Public inline entries can change after binding. File entries are fresh
		// values whose complete seal and graph were validated by file.read's
		// Decode; validating them again repeats the same full-graph digest.
		for _, index := range portfolio.Entries {
			if err := index.Validate(); err != nil {
				return err
			}
		}
		for _, file := range portfolio.files {
			if _, err := file.read(); err != nil {
				return err
			}
		}
		return nil
	}
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
	if len(portfolio.files) > 0 {
		entry, found, err := portfolio.readTarget(portfolio.DefaultTargetID)
		if err != nil {
			return programindex.Index{}, err
		}
		if found {
			return entry, nil
		}
	}
	for _, entry := range portfolio.Entries {
		if entry.Target.ID == portfolio.DefaultTargetID {
			return entry, nil
		}
	}
	return programindex.Index{}, fmt.Errorf("program portfolio: default target is missing")
}
