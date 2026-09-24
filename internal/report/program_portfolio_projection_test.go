package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramPortfolioKeepsEveryExactTargetAndOneDefaultEntry(t *testing.T) {
	pythonIndex := reportProgramIndexFixture(t, "python", "executable")
	goIndex := reportProgramIndexFixture(t, "go", "library")
	rebound, err := programindex.RebindTargetSet([]programindex.Index{pythonIndex, goIndex})
	if err != nil {
		t.Fatal(err)
	}
	pythonIndex, goIndex = rebound[0], rebound[1]
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
	if defaultEntry.Target.ID != pythonIndex.Target.ID || defaultEntry.SHA256 != pythonIndex.SHA256 {
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
		portfolio.Entries[0].SHA256 != index.SHA256 {
		t.Fatalf("synthetic adapter entry = %#v", portfolio.Entries)
	}
}

// Lookups inside one publication no longer revalidate the portfolio, so the
// boundaries that receive report data must still refuse a ProgramIndex that
// no longer matches its seal: report.json is not written and no page renders.
func TestPublicationRefusesAProgramIndexThatNoLongerMatchesItsSeal(t *testing.T) {
	data := reportProgramShellDataFixture(t, "example")
	options := reportSingleTargetRenderOptionsFixture(t, &data)
	if _, err := RenderHTMLWithOptions(&data, options); err != nil {
		t.Fatalf("sealed fixture does not render: %v", err)
	}
	data.ProgramPortfolio.Entries[0].Objects[0].Name += "_edited"
	if _, err := RenderHTMLWithOptions(&data, options); err == nil ||
		!strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("render of an edited ProgramIndex = %v", err)
	}
	if _, err := encodeReportJSON(&data, 0); err == nil ||
		!strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("report.json of an edited ProgramIndex = %v", err)
	}
}
