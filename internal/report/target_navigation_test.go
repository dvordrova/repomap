package report

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestBuildTargetNavigationProjectsExactLanguageNeutralPages(t *testing.T) {
	current := targetNavigationProgramTarget(t, "go", "executable", "api", "cmd/api/main.go", "4")
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, current)

	got, err := BuildTargetNavigation(pages, defaultTargetID, currentTargetID)
	if err != nil {
		t.Fatal(err)
	}
	want := &TargetNavigationPortfolio{
		Version:         TargetNavigationVersion,
		DefaultTargetID: defaultTargetID,
		CurrentTargetID: currentTargetID,
		Targets: []TargetNavigationItem{
			{
				TargetID: pages[0].ProgramTarget.ID, Language: "go", Kind: "executable",
				DisplayName: "server",
				Href:        "../20260810-120000-server-a1b2c3/report.html#/program",
			},
			{
				TargetID: pages[1].ProgramTarget.ID, Language: pages[1].ProgramTarget.Language,
				Kind: pages[1].ProgramTarget.Kind, DisplayName: pages[1].ProgramTarget.Name,
				Href: "#/program",
			},
			{
				TargetID: pages[2].ProgramTarget.ID, Language: "python", Kind: "worker",
				DisplayName: "event worker",
				Href:        "../20260810-120000-worker-a1b2c3/report.html#/program",
			},
			{
				TargetID: pages[3].ProgramTarget.ID, Language: "bash", Kind: "tool",
				DisplayName: "release scripts",
				Href:        "../20260810-120000-tool-a1b2c3/report.html#/program",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("target navigation = %s, want %s", gotJSON, wantJSON)
	}
}

func TestTargetNavigationRenderOptionsStayTransient(t *testing.T) {
	data, navigation := targetNavigationFixture(t)

	if _, err := RenderHTMLWithOptions(data, RenderOptions{}); err == nil ||
		!strings.Contains(err.Error(), "complete target navigation") {
		t.Fatalf("missing mandatory target navigation error = %v", err)
	}

	withNavigation, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatalf("RenderHTMLWithOptions navigation: %v", err)
	}
	// Sections come from the analyzed graph the run actually produced, and
	// they are named the way the reader sees them. Render-only navigation
	// never puts an internal identity on the page.
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(withNavigation, []byte(entry.Target.Name)) {
		t.Fatalf("rendered page is missing the analyzed target %q", entry.Target.Name)
	}
	for _, item := range navigation.Targets {
		if bytes.Contains(withNavigation, []byte(item.TargetID)) {
			t.Fatalf("rendered page exposed the internal identity of %q", item.DisplayName)
		}
	}

	canonical, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(canonical, []byte("target_navigation")) {
		t.Fatal("render-only navigation leaked into canonical ReportData JSON")
	}
}

func TestTargetNavigationSiblingHrefResolvesForFileAndHostedReports(t *testing.T) {
	const href = "../20260810-120000-worker-a1b2c3/report.html#/program"
	reference, err := url.Parse(href)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		base string
		want string
	}{
		{
			base: "file:///Users/test/runs/20260810-120000-api-a1b2c3/report.html#study-theme-25",
			want: "file:///Users/test/runs/20260810-120000-worker-a1b2c3/report.html#/program",
		},
		{
			base: "http://127.0.0.1:55948/_repomap/token/runs/20260810-120000-api-a1b2c3/report.html#study-theme-25",
			want: "http://127.0.0.1:55948/_repomap/token/runs/20260810-120000-worker-a1b2c3/report.html#/program",
		},
	}
	for _, test := range tests {
		base, parseErr := url.Parse(test.base)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got := base.ResolveReference(reference).String(); got != test.want {
			t.Errorf("resolve %q from %q = %q, want %q", href, test.base, got, test.want)
		}
	}
}

func TestBuildTargetNavigationRejectsIncompleteOrTamperedPages(t *testing.T) {
	current := targetNavigationProgramTarget(t, "go", "executable", "api", "cmd/api/main.go", "4")
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, current)
	tests := map[string]func([]TargetNavigationPage) ([]TargetNavigationPage, string, string){
		"unknown current": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			return value, defaultTargetID, "unknown"
		},
		"unknown default": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			return value, "unknown", currentTargetID
		},
		"duplicate target": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].ProgramTarget = value[0].ProgramTarget
			return value, defaultTargetID, currentTargetID
		},
		"duplicate run": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].RunID = value[0].RunID
			return value, defaultTargetID, currentTargetID
		},
		"invalid target": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].ProgramTarget.Selector = "tampered"
			return value, defaultTargetID, currentTargetID
		},
		"missing artifact filename": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].ArtifactFilename = ""
			return value, defaultTargetID, currentTargetID
		},
		"artifact path": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].ArtifactFilename = "nested/program-index.json"
			return value, defaultTargetID, currentTargetID
		},
		"target-specific artifact filename": func(value []TargetNavigationPage) ([]TargetNavigationPage, string, string) {
			value[2].ArtifactFilename = "program-index.worker.json"
			return value, defaultTargetID, currentTargetID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := cloneTargetNavigationPages(pages)
			candidate, candidateDefault, candidateCurrent := mutate(candidate)
			if _, err := BuildTargetNavigation(candidate, candidateDefault, candidateCurrent); err == nil {
				t.Fatal("tampered target navigation page was accepted")
			}
		})
	}
}

func TestTargetNavigationRejectsUnboundOrUnsafeProjection(t *testing.T) {
	data, navigation := targetNavigationFixture(t)
	tests := map[string]func(*TargetNavigationPortfolio){
		"unknown current": func(value *TargetNavigationPortfolio) { value.CurrentTargetID = "unknown" },
		"unknown default": func(value *TargetNavigationPortfolio) { value.DefaultTargetID = "unknown" },
		"duplicate":       func(value *TargetNavigationPortfolio) { value.Targets[1].TargetID = value.Targets[0].TargetID },
		"current language drift": func(value *TargetNavigationPortfolio) {
			targetNavigationCurrentItem(t, value).Language = "tampered-language"
		},
		"current kind drift": func(value *TargetNavigationPortfolio) {
			targetNavigationCurrentItem(t, value).Kind = "tampered-kind"
		},
		"current name drift": func(value *TargetNavigationPortfolio) {
			targetNavigationCurrentItem(t, value).DisplayName = "wrong"
		},
		"missing href": func(value *TargetNavigationPortfolio) { value.Targets[2].Href = "" },
		"absolute href": func(value *TargetNavigationPortfolio) {
			value.Targets[0].Href = "https://example.test/report.html#/program"
		},
		"escaping href": func(value *TargetNavigationPortfolio) {
			value.Targets[0].Href = "../../report.html#/program"
		},
		"foreign fragment": func(value *TargetNavigationPortfolio) {
			value.Targets[0].Href = "../safe/report.html#study-theme-25"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := cloneTargetNavigation(t, navigation)
			mutate(candidate)
			if _, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: candidate}); err == nil {
				t.Fatal("unsafe target navigation was accepted")
			}
		})
	}
}

func targetNavigationCurrentItem(t *testing.T, value *TargetNavigationPortfolio) *TargetNavigationItem {
	t.Helper()
	for position := range value.Targets {
		if value.Targets[position].TargetID == value.CurrentTargetID {
			return &value.Targets[position]
		}
	}
	t.Fatal("current navigation item is missing")
	return nil
}

func targetNavigationFixture(t *testing.T) (*ReportData, *TargetNavigationPortfolio) {
	t.Helper()
	// Navigation rendering is independent of semantic cubes, but an ordinary
	// default ProgramIndex page always carries the generic semantic cube family.
	// Reuse the complete neutral fixture instead of teaching this test a fake
	// language-specific structural exception.
	data := reportProgramShellDataFixture(t, "workspace")
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, entry.Target)
	navigation, err := BuildTargetNavigation(pages, defaultTargetID, currentTargetID)
	if err != nil {
		t.Fatal(err)
	}
	data.TargetOutcomePortfolio = reportTargetOutcomeViewFixture(t, pages, defaultTargetID)
	return &data, navigation
}

func targetNavigationPages(
	t *testing.T,
	current programindex.Target,
) ([]TargetNavigationPage, string, string) {
	t.Helper()
	server := targetNavigationProgramTarget(t, "go", "executable", "server", "cmd/server/main.go", "1")
	worker := targetNavigationProgramTarget(t, "python", "worker", "event worker", "worker/app.py", "2")
	tool := targetNavigationProgramTarget(t, "bash", "tool", "release scripts", "scripts/release.sh", "3")
	pages := []TargetNavigationPage{
		{RunID: "20260810-120000-server-a1b2c3", ProgramTarget: server, ArtifactFilename: programindex.ArtifactFilename},
		{RunID: "20260810-120000-api-a1b2c3", ProgramTarget: current.Snapshot(), ArtifactFilename: programindex.ArtifactFilename},
		{RunID: "20260810-120000-worker-a1b2c3", ProgramTarget: worker, ArtifactFilename: programindex.ArtifactFilename},
		{RunID: "20260810-120000-tool-a1b2c3", ProgramTarget: tool, ArtifactFilename: programindex.ArtifactFilename},
	}
	return pages, server.ID, current.ID
}

func targetNavigationProgramTarget(
	t *testing.T,
	language, kind, name, sourcePath, digestCharacter string,
) programindex.Target {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat(digestCharacter, 64),
		SourceSHA256:   strings.Repeat(digestCharacter, 64),
		Target: programindex.TargetInput{
			Language: language, Kind: kind, Name: name, Selector: name,
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: sourcePath}},
			AnchorFileRef: "f1",
		},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatalf("build target navigation program target: %v", err)
	}
	return index.Target
}

func cloneTargetNavigationPages(source []TargetNavigationPage) []TargetNavigationPage {
	result := make([]TargetNavigationPage, len(source))
	for index := range source {
		result[index] = source[index]
		result[index].ProgramTarget = source[index].ProgramTarget.Snapshot()
	}
	return result
}

func cloneTargetNavigation(t *testing.T, source *TargetNavigationPortfolio) *TargetNavigationPortfolio {
	t.Helper()
	wire, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var result TargetNavigationPortfolio
	if err := json.Unmarshal(wire, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}
