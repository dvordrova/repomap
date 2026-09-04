package report

import (
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"strings"
	"testing"
)

// reportProgramShellDataFixture is one complete analyzed repository as the
// page renderer sees it: a program portfolio, the final group graph, and the
// target inventory. The fact, claim and orientation layers stay absent so the
// fixture also proves the page renders without them.
func reportProgramShellDataFixture(t *testing.T, repoName string) ReportData {
	t.Helper()
	index := reportProgramIndexFixture(t, "python", "executable")
	enriched, groups, _ := reportFinalGraphFixture(t, index)
	portfolio, err := NewProgramPortfolio(enriched.Target.ID, []programindex.Index{enriched})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView([]groupindex.Index{groups}, enriched.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := ReportData{
		FormatVersion:    CurrentFormatVersion,
		RepoName:         repoName,
		CapturedRevision: strings.Repeat("a", 40),
		ProgramPortfolio: portfolio,
		GroupGraph:       graph,
		TargetOutcomePortfolio: reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
			RunID:            "20260810-120000-page-a1b2c3",
			ProgramTarget:    enriched.Target.Snapshot(),
			ArtifactFilename: programindex.ArtifactFilename,
		}}, enriched.Target.ID),
	}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	return data
}

// reportSingleTargetRenderOptionsFixture supplies the render-only navigation
// of a one-page repository.
func reportSingleTargetRenderOptionsFixture(t *testing.T, data *ReportData) RenderOptions {
	t.Helper()
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	pages := []TargetNavigationPage{{
		RunID:            "20260810-120000-page-a1b2c3",
		ProgramTarget:    entry.Target.Snapshot(),
		ArtifactFilename: programindex.ArtifactFilename,
	}}
	navigation, err := BuildTargetNavigation(pages, entry.Target.ID, entry.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	return RenderOptions{TargetNavigation: navigation}
}

// reportTargetOutcomeViewFixture states that every supplied page was analyzed.
func reportTargetOutcomeViewFixture(
	t *testing.T,
	pages []TargetNavigationPage,
	defaultTargetID string,
) *TargetOutcomePortfolioView {
	t.Helper()
	outcomes := make([]targetoutcome.Outcome, 0, len(pages))
	bindings := make([]programpage.Page, 0, len(pages))
	defaultSelectedID := ""
	for position, page := range pages {
		selected := reportSelectedTargetFixture(t, page.ProgramTarget, position)
		analyzed, err := targetoutcome.NewAnalyzed(selected, page.ProgramTarget, page.RunID)
		if err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, analyzed)
		bindings = append(bindings, programpage.Page{
			Target: page.ProgramTarget.Snapshot(), RunID: page.RunID,
		})
		if page.ProgramTarget.ID == defaultTargetID {
			defaultSelectedID = selected.ID
		}
	}
	if defaultSelectedID == "" && len(outcomes) > 0 {
		defaultSelectedID = outcomes[0].SelectedTarget.ID
	}
	portfolio, err := targetoutcome.Build(defaultSelectedID, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	pagePortfolio, err := programpage.Build(defaultTargetID, bindings)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewTargetOutcomePortfolioView(portfolio, pagePortfolio)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func reportSelectedTargetFixture(
	t *testing.T,
	target programindex.Target,
	position int,
) targetoutcome.SelectedTarget {
	t.Helper()
	group := targetoutcome.LanguageGroupPython
	switch target.Language {
	case "go":
		group = targetoutcome.LanguageGroupGo
	case "javascript", "typescript":
		group = targetoutcome.LanguageGroupJavaScriptTypeScript
	}
	// Scope follows the target kind so two targets of one language keep
	// distinct identities.
	scope := targetoutcome.ScopeLibrary
	switch target.Kind {
	case "executable":
		scope = targetoutcome.ScopeExecutable
	case "application", "package":
		scope = targetoutcome.ScopePackage
	}
	// The fixture binds the exact language the page materialized, so a
	// synthetic target of any language stays a valid analyzed outcome.
	selected, err := targetoutcome.NewSelectedTargetWithLanguages(
		group, []string{target.Language}, scope, target.Name, target.Selector,
	)
	if err != nil {
		t.Fatalf("selected target fixture %d: %v", position, err)
	}
	return selected
}

// reportTargetOutcomeArtifactFixture builds the persisted outcome inventory
// beside its view.
func reportTargetOutcomeArtifactFixture(
	t *testing.T,
	pages []TargetNavigationPage,
	defaultTargetID string,
) (targetoutcome.Portfolio, []byte) {
	t.Helper()
	outcomes := make([]targetoutcome.Outcome, 0, len(pages))
	defaultSelectedID := ""
	for position, page := range pages {
		selected := reportSelectedTargetFixture(t, page.ProgramTarget, position)
		analyzed, err := targetoutcome.NewAnalyzed(selected, page.ProgramTarget, page.RunID)
		if err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, analyzed)
		if page.ProgramTarget.ID == defaultTargetID {
			defaultSelectedID = selected.ID
		}
	}
	if defaultSelectedID == "" && len(outcomes) > 0 {
		defaultSelectedID = outcomes[0].SelectedTarget.ID
	}
	portfolio, err := targetoutcome.Build(defaultSelectedID, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return portfolio, raw
}

// TestSectionLabelsDistinguishRepeatedTargetNames proves a reader can tell two
// targets apart when a repository names them the same, as a Go module and its
// command in one directory do.
func TestSectionLabelsDistinguishRepeatedTargetNames(t *testing.T) {
	sections := []*pageSection{
		{Name: "versions", Kind: "library", Root: "_examples/versions"},
		{Name: "versions", Kind: "executable", Root: "_examples/versions"},
		{Name: "rest-example", Kind: "executable", Root: "_examples/rest"},
	}
	labelSections(sections)
	want := []string{"versions (library)", "versions (executable)", "rest-example"}
	for index, expected := range want {
		if sections[index].Label != expected {
			t.Fatalf("section %d label = %q, want %q", index, sections[index].Label, expected)
		}
	}
}
