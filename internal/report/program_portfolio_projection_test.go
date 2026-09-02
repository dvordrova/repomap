package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramPortfolioKeepsEveryExactTargetAndOneDefaultEntry(t *testing.T) {
	pythonIndex := reportProgramIndexFixture(t, "python", "executable")
	goIndex := reportProgramIndexFixture(t, "go", "library")
	portfolio, err := NewProgramPortfolio(pythonIndex.Target.ID, []programindex.Index{pythonIndex, goIndex})
	if err != nil {
		t.Fatalf("NewProgramPortfolio: %v", err)
	}
	if len(portfolio.Entries) != 2 || portfolio.Entries[0].Target.ID >= portfolio.Entries[1].Target.ID {
		t.Fatalf("portfolio entries are not a complete canonical set: %#v", portfolio.Entries)
	}
	defaultEntry, err := portfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	if defaultEntry.Target.ID != pythonIndex.Target.ID || defaultEntry.View.TargetID != pythonIndex.Target.ID {
		t.Fatalf("default entry = %#v", defaultEntry)
	}
}

func TestProgramPortfolioRejectsDuplicateTargetEntries(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "library")
	if _, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index, index}); err == nil ||
		!strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("duplicate target error = %v", err)
	}
}

func TestProgramPortfolioAcceptsSyntheticAdapterLanguage(t *testing.T) {
	index := reportProgramIndexFixture(t, "synthetic-jvm", "application")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	if len(portfolio.Entries) != 1 || portfolio.Entries[0].Target.ID != index.Target.ID ||
		portfolio.Entries[0].View.TargetID != index.Target.ID {
		t.Fatalf("synthetic adapter entry = %#v", portfolio.Entries)
	}
}
