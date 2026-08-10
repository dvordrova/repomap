package report

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestManifestBoundTargetNavigationUsesTheStaticPageProjection(t *testing.T) {
	container, portfolio, currentRef := targetNavigationArtifactFixture(t)
	runDir := t.TempDir()
	containerRaw, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, snapshot.TargetRunContainerArtifactFilename),
		containerRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename),
		portfolioRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	manifest := RunManifest{
		Version: CurrentRunManifestVersion,
		MaterialInputs: MaterialInputs{
			AnalysisTargetRef:         currentRef,
			TargetRunContainerSHA256:  manifestSHA256(containerRaw),
			TargetPagePortfolioSHA256: manifestSHA256(portfolioRaw),
		},
	}

	want, err := BuildTargetNavigation(container, portfolio, currentRef)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifestTargetNavigation(runDir, manifest)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("manifest navigation = %s, want static projection %s", gotJSON, wantJSON)
	}
	if len(got.Targets) < 2 {
		t.Fatalf("target navigation has %d targets, want at least 2", len(got.Targets))
	}
}

func TestTargetNavigationRenderOptionsStayTransient(t *testing.T) {
	data, navigation := targetNavigationFixture()

	ordinary, err := RenderHTML(data)
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
	if _, exists := reportDataJSONFields(t, embeddedReportDataJSON(t, ordinary))["target_navigation"]; exists {
		t.Fatal("ordinary single-target render gained target navigation")
	}

	withNavigation, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatalf("RenderHTMLWithOptions navigation: %v", err)
	}
	fields := reportDataJSONFields(t, embeddedReportDataJSON(t, withNavigation))
	var projected TargetNavigationPortfolio
	if err := json.Unmarshal(fields["target_navigation"], &projected); err != nil {
		t.Fatalf("decode target navigation: %v", err)
	}
	if projected.Version != TargetNavigationVersion ||
		projected.CurrentTargetRef != navigation.CurrentTargetRef ||
		len(projected.Targets) != len(navigation.Targets) {
		t.Fatalf("target navigation projection = %#v", projected)
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
	const href = "../20260810-120000-worker-a1b2c3/report.html#/map"
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
			want: "file:///Users/test/runs/20260810-120000-worker-a1b2c3/report.html#/map",
		},
		{
			base: "http://127.0.0.1:55948/_repomap/token/runs/20260810-120000-api-a1b2c3/report.html#study-theme-25",
			want: "http://127.0.0.1:55948/_repomap/token/runs/20260810-120000-worker-a1b2c3/report.html#/map",
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

func TestTargetNavigationRailGroupsAndUsesNativeSiblingLinks(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	data, navigation := targetNavigationFixture()
	html, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(t.TempDir(), "report-payload.json")
	if err := os.WriteFile(payloadPath, embeddedReportDataJSON(t, html), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
const payload = fs.readFileSync(process.argv[3], "utf8");
class Element {
  constructor(tag) { this.tagName = String(tag).toUpperCase(); this.className = ""; this.attributes = {}; this.children = []; this.textContent = ""; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  appendChild(child) { this.children.push(child); return child; }
}
const document = {
  documentElement: {lang:"en"},
  createElement(tag) { return new Element(tag); },
  getElementById(id) { return id === "rm-report-data" ? {textContent:payload} : null; },
  querySelector() { return null; }, querySelectorAll() { return []; },
};
const window = {
  document,
  location:{search:"",hash:"#study-theme-25",protocol:"file:",pathname:"/runs/current/report.html"},
  __REPOMAP_WORKSPACE_TEST__:{}, addEventListener(){}, removeEventListener(){},
};
const context = {window,document,URLSearchParams,Set,Map,AbortController,Promise};
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
const tabs = new Element("nav");
window.__REPOMAP_WORKSPACE_TEST__.renderAnalysisTargetMenu(tabs);
function hasClass(node, name) { return String(node.className || "").split(/\s+/).includes(name); }
function countClass(node, name) { return (hasClass(node,name) ? 1 : 0) + node.children.reduce((sum, child) => sum + countClass(child,name), 0); }
const groups = tabs.children.map((section) => ({
  label: section.children[0].textContent,
  items: section.children[1].children.map((row) => {
    const item = row.children[0];
    return {
      tag: item.tagName, ref: item.attributes["data-target-ref"] || "",
      href: item.attributes.href || "", ariaCurrent: item.attributes["aria-current"] || "",
      ariaDisabled: item.attributes["aria-disabled"] || "",
      workspaceView: item.attributes["data-workspace-view"] || "",
      active: hasClass(item,"rm-active"), disabled: hasClass(item,"rm-target-link--disabled"),
      defaultMarkers: countClass(item,"rm-target-link__default-dot"), hasClick: typeof item.onclick === "function",
    };
  }),
}));
process.stdout.write(JSON.stringify({groups,hash:window.location.hash}));
`
	runnerPath := filepath.Join(t.TempDir(), "target-navigation.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, scriptPath, payloadPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run target navigation asset: %v\n%s", err, output)
	}
	var got struct {
		Groups []struct {
			Label string
			Items []struct {
				Tag, Ref, Href, AriaCurrent, AriaDisabled, WorkspaceView string
				Active, Disabled, HasClick                               bool
				DefaultMarkers                                           int
			}
		}
		Hash string
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode target navigation asset: %v\n%s", err, output)
	}
	if len(got.Groups) != 3 || got.Groups[0].Label != "go.mod" ||
		got.Groups[1].Label != "services/api/go.mod" || got.Groups[2].Label != "tools/go.mod" {
		t.Fatalf("target groups = %#v", got.Groups)
	}
	defaultTarget := got.Groups[0].Items[0]
	if defaultTarget.Tag != "A" || defaultTarget.Href != "../20260810-120000-server-a1b2c3/report.html#/map" ||
		defaultTarget.DefaultMarkers != 1 || defaultTarget.Active || defaultTarget.AriaCurrent != "" ||
		defaultTarget.WorkspaceView != "" || defaultTarget.HasClick {
		t.Fatalf("default sibling target = %#v", defaultTarget)
	}
	current := got.Groups[1].Items[0]
	if current.Tag != "A" || current.Href != "#/map" || !current.Active ||
		current.AriaCurrent != "page" || current.WorkspaceView != "map" || current.HasClick {
		t.Fatalf("current target = %#v", current)
	}
	unavailable := got.Groups[1].Items[1]
	if unavailable.Tag != "SPAN" || unavailable.Href != "" || unavailable.AriaDisabled != "true" ||
		!unavailable.Disabled || unavailable.Active || unavailable.HasClick {
		t.Fatalf("unavailable target = %#v", unavailable)
	}
	if got.Hash != "#study-theme-25" {
		t.Fatalf("rendering target rail changed deep link %q", got.Hash)
	}
}

func TestTargetNavigationRejectsUnboundOrUnsafeProjection(t *testing.T) {
	data, navigation := targetNavigationFixture()
	tests := map[string]func(*TargetNavigationPortfolio){
		"unknown current":       func(value *TargetNavigationPortfolio) { value.CurrentTargetRef = "unknown" },
		"unknown default":       func(value *TargetNavigationPortfolio) { value.DefaultTargetRef = "unknown" },
		"duplicate":             func(value *TargetNavigationPortfolio) { value.Targets[1].TargetRef = value.Targets[0].TargetRef },
		"current mismatch":      func(value *TargetNavigationPortfolio) { value.Targets[1].DisplayPath = "services/api/wrong" },
		"availability mismatch": func(value *TargetNavigationPortfolio) { value.Targets[2].Href = "../missing/report.html#/map" },
		"absolute href": func(value *TargetNavigationPortfolio) {
			value.Targets[0].Href = "https://example.test/report.html#/map"
		},
		"escaping href":    func(value *TargetNavigationPortfolio) { value.Targets[0].Href = "../../report.html#/map" },
		"foreign fragment": func(value *TargetNavigationPortfolio) { value.Targets[0].Href = "../safe/report.html#study-theme-25" },
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

func targetNavigationFixture() (*ReportData, *TargetNavigationPortfolio) {
	data := &ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "workspace",
		AnalysisTarget: &analysistarget.Target{
			Version: analysistarget.Version,
			Ref:     "target-api", Kind: analysistarget.KindLibraryPackage,
			ModuleDir: "services/api", PackageDir: "services/api/pkg/client",
			PackagePath: "example.test/api/pkg/client",
		},
	}
	navigation := &TargetNavigationPortfolio{
		Version: TargetNavigationVersion, DefaultTargetRef: "target-server", CurrentTargetRef: "target-api",
		Targets: []TargetNavigationItem{
			{TargetRef: "target-server", ModuleDir: ".", DisplayPath: "cmd/server", Available: true, Href: "../20260810-120000-server-a1b2c3/report.html#/map"},
			{TargetRef: "target-api", ModuleDir: "services/api", DisplayPath: "services/api/pkg/client", Available: true, Href: "#/map"},
			{TargetRef: "target-worker", ModuleDir: "services/api", DisplayPath: "services/api/cmd/worker"},
			{TargetRef: "target-tool", ModuleDir: "tools", DisplayPath: "tools/cmd/check", Available: true, Href: "../20260810-120000-tool-a1b2c3/report.html#/map"},
		},
	}
	return data, navigation
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

func targetNavigationArtifactFixture(
	t *testing.T,
) (snapshot.TargetRunContainer, snapshot.TargetPagePortfolio, string) {
	t.Helper()
	repository := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.test/navigation\n\ngo 1.24\n",
		"cmd/server/main.go": "package main\nfunc main() {}\n",
		"pkg/client/client.go": "package client\n" +
			"func Open() {}\n",
	}
	for name, contents := range files {
		absolute := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "--", "go.mod", "cmd/server/main.go", "pkg/client/client.go"},
	} {
		command := exec.Command("git", append([]string{"-C", repository}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	deferred, err := snapshot.Build(snapshot.Options{
		RepoPath: repository, DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil || len(deferred.TargetCatalog.Entries) < 2 {
		t.Fatalf("target catalog = %#v, want at least 2 targets", deferred.TargetCatalog)
	}
	selected := make([]string, 0, 2)
	for _, entry := range deferred.TargetCatalog.Entries {
		if entry.Candidate.Target.PackageDir == "cmd/server" ||
			entry.Candidate.Target.PackageDir == "pkg/client" {
			selected = append(selected, entry.Candidate.Target.Ref)
		}
	}
	if len(selected) != 2 {
		t.Fatalf("selected target refs = %v, want server and client", selected)
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: selected[0],
		TargetRefs:       selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for index, projection := range container.Targets {
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			State:     snapshot.TargetPageReady,
			RunID:     []string{"20260810-120000-server-a1b2c3", "20260810-120001-client-a1b2c3"}[index],
		})
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	return container, portfolio, container.Targets[1].Target.Ref
}
