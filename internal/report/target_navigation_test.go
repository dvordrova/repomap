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

func embeddedBrowserRepositoryPayload(t *testing.T, html []byte) BrowserRepositoryPayload {
	t.Helper()
	transport, err := extractStandaloneBundleTransportV4HTML(html)
	if err != nil {
		t.Fatalf("extract rendered browser transport: %v", err)
	}
	payload, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload)
	if err != nil {
		t.Fatalf("decode rendered repository payload: %v", err)
	}
	canonical, err := EncodeBrowserRepositoryPayload(payload)
	if err != nil {
		t.Fatalf("re-encode rendered repository payload: %v", err)
	}
	if !bytes.Equal(canonical, transport.RepositoryPayload) {
		t.Fatal("rendered repository payload is not canonical typed JSON")
	}
	return payload
}

func TestBuildTargetNavigationProjectsExactLanguageNeutralPages(t *testing.T) {
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, cubeMapProgramTargetFixture(t))

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

	ordinary, err := RenderHTMLWithOptions(data, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	zeroOptions, err := RenderHTMLWithOptions(data, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderHTMLWithOptions zero: %v", err)
	}
	if !bytes.Equal(ordinary, zeroOptions) {
		t.Fatal("zero render options changed the existing single-target HTML")
	}
	ordinaryRepository := embeddedBrowserRepositoryPayload(t, ordinary)
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	wantOrdinaryTarget := BrowserTargetIndexItem{
		SelectedTargetID: entry.Target.ID,
		ProgramTargetID:  entry.Target.ID,
		Language:         entry.Target.Language,
		Kind:             entry.Target.Kind,
		DisplayName:      entry.Target.Name,
		State:            "analyzed",
		Href:             "#/program",
	}
	if ordinaryRepository.LogicalDefaultSelectedTargetID != entry.Target.ID ||
		!reflect.DeepEqual(ordinaryRepository.Targets, []BrowserTargetIndexItem{wantOrdinaryTarget}) {
		t.Fatalf("ordinary browser target index = %#v with default %q", ordinaryRepository.Targets,
			ordinaryRepository.LogicalDefaultSelectedTargetID)
	}

	withNavigation, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatalf("RenderHTMLWithOptions navigation: %v", err)
	}
	repository := embeddedBrowserRepositoryPayload(t, withNavigation)
	wantTargets := make([]BrowserTargetIndexItem, 0, len(navigation.Targets))
	for _, item := range navigation.Targets {
		wantTargets = append(wantTargets, BrowserTargetIndexItem{
			SelectedTargetID: item.TargetID,
			ProgramTargetID:  item.TargetID,
			Language:         item.Language,
			Kind:             item.Kind,
			DisplayName:      item.DisplayName,
			State:            "analyzed",
			Href:             item.Href,
		})
	}
	if repository.LogicalDefaultSelectedTargetID != navigation.DefaultTargetID ||
		!reflect.DeepEqual(repository.Targets, wantTargets) {
		t.Fatalf("rendered browser target index = %#v with default %q, want %#v with default %q",
			repository.Targets, repository.LogicalDefaultSelectedTargetID,
			wantTargets, navigation.DefaultTargetID)
	}
	transport, err := extractStandaloneBundleTransportV4HTML(withNavigation)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range [][]byte{[]byte(`"target_navigation"`), []byte(`"artifact_filename"`)} {
		if bytes.Contains(transport.RepositoryPayload, legacy) {
			t.Fatalf("typed repository payload exposes retired navigation field %q", legacy)
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
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, cubeMapProgramTargetFixture(t))
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
			value.Targets[1].Language = "python"
		},
		"current kind drift": func(value *TargetNavigationPortfolio) {
			value.Targets[1].Kind = "library"
		},
		"current name drift": func(value *TargetNavigationPortfolio) {
			value.Targets[1].DisplayName = "wrong"
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

func targetNavigationFixture(t *testing.T) (*ReportData, *TargetNavigationPortfolio) {
	t.Helper()
	// Navigation rendering is independent of semantic cubes. Use a structural
	// language target so this fixture does not preserve the retired Go CubeMap
	// presentation merely to test transient navigation options.
	index := reportProgramIndexFixture(t, "bash", "tool")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		FormatVersion:      CurrentFormatVersion,
		RepoName:           "workspace",
		CapturedRevision:   strings.Repeat("a", 40),
		CapturedInputCount: 0,
		ProgramPortfolio:   portfolio,
	}
	if err := collectOpenablePaths(data); err != nil {
		t.Fatal(err)
	}
	pages, defaultTargetID, currentTargetID := targetNavigationPages(t, index.Target)
	navigation, err := BuildTargetNavigation(pages, defaultTargetID, currentTargetID)
	if err != nil {
		t.Fatal(err)
	}
	return data, navigation
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
		{RunID: "20260810-120000-server-a1b2c3", ProgramTarget: server, ArtifactFilename: "program-index-server.json"},
		{RunID: "20260810-120000-api-a1b2c3", ProgramTarget: current.Snapshot(), ArtifactFilename: "program-index-api.json"},
		{RunID: "20260810-120000-worker-a1b2c3", ProgramTarget: worker, ArtifactFilename: "program-index-worker.json"},
		{RunID: "20260810-120000-tool-a1b2c3", ProgramTarget: tool, ArtifactFilename: "program-index-tool.json"},
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
