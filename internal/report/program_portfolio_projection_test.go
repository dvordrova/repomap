package report

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramPortfolioReadersOwnEachCompleteIndexBeforeTheNextRead(t *testing.T) {
	first := reportProgramIndexFixture(t, "python", "executable")
	second := reportProgramIndexFixture(t, "go", "library")
	indexes, err := programindex.RebindTargetSet([]programindex.Index{first, second})
	if err != nil {
		t.Fatal(err)
	}
	first, second = indexes[0], indexes[1]
	want, err := NewProgramPortfolio(first.Target.ID, []programindex.Index{first, second})
	if err != nil {
		t.Fatal(err)
	}
	firstName := first.Objects[0].Name
	var order []string
	data := &ReportData{}
	err = BindProgramPortfolioReaders(data, first.Target.ID, []func() (programindex.Index, error){
		func() (programindex.Index, error) {
			order = append(order, first.Target.ID)
			return first, nil
		},
		func() (programindex.Index, error) {
			// A reader may release/reuse its previous input before returning the
			// next. The already owned native rows must survive unchanged.
			first.Objects[0].Name = "released previous input"
			order = append(order, second.Target.ID)
			return second, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Objects[0].Name = firstName
	if !reflect.DeepEqual(order, []string{first.Target.ID, second.Target.ID}) || !reflect.DeepEqual(data.ProgramPortfolio, want) {
		t.Fatal("reader binding lost original target/native rows or ordered single reads")
	}
	second.Objects[0].Name = "released final input"
	if !reflect.DeepEqual(data.ProgramPortfolio, want) || data.defaultProgramIndex == nil || data.defaultProgramIndex.SHA256 != first.SHA256 {
		t.Fatal("report retained a mutable reader input or lost its default native binding")
	}
}

func TestProgramPortfolioReaderFailureDoesNotReplaceAcceptedBinding(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "library")
	data := &ReportData{}
	if err := BindProgramPortfolio(data, index.Target.ID, []programindex.Index{index}); err != nil {
		t.Fatal(err)
	}
	accepted := data.ProgramPortfolio
	refused := errors.New("original sealed target is unavailable")
	for _, test := range []struct {
		name string
		read func() (programindex.Index, error)
	}{
		{"missing-reader", nil},
		{"missing-native-file", func() (programindex.Index, error) { return programindex.Index{}, refused }},
		{"mismatched-seal", func() (programindex.Index, error) {
			bad := index.Snapshot()
			bad.Objects[0].Name += "changed"
			return bad, nil
		}},
		{"duplicate-target", func() (programindex.Index, error) { return index, nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := BindProgramPortfolioReaders(data, index.Target.ID, []func() (programindex.Index, error){
				func() (programindex.Index, error) { return index, nil }, test.read,
			})
			if err == nil || data.ProgramPortfolio != accepted {
				t.Fatal("failed complete native binding replaced the accepted portfolio")
			}
			if test.name == "missing-native-file" && !errors.Is(err, refused) {
				t.Fatalf("original read error changed: %v", err)
			}
		})
	}
}

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
