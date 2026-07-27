package report

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceEpisodeProjectionRendersAcceptedGoAndPythonFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		episodeID  string
		repository string
	}{
		{
			name:       "go etcd",
			path:       filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json"),
			episodeID:  "etcd-put-recoverability",
			repository: "etcd-io/etcd",
		},
		{
			name:       "python django",
			path:       filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
			episodeID:  "django-nested-atomic",
			repository: "django/django",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			raw, episode := readSourceEpisodeFixture(t, test.path)
			data := authorizedSourceEpisodeReport(episode)
			projected, err := projectApprovedSourceEpisode(data, raw)
			if err != nil {
				t.Fatalf("project accepted episode: %v", err)
			}
			if projected.EpisodeID != test.episodeID || projected.Repository != test.repository {
				t.Fatalf("identity = %q %q", projected.EpisodeID, projected.Repository)
			}
			if len(projected.Claims) != len(episode.Claims) || len(projected.Claims) != 8 {
				t.Fatalf("projected claims = %d, want %d", len(projected.Claims), len(episode.Claims))
			}
			for index, claim := range episode.Claims {
				got := projected.Claims[index]
				if got.Title != claim.Title || got.Statement != claim.Statement || got.State != claim.State {
					t.Fatalf("claim %d drifted: %#v", index, got)
				}
				if len(got.Sources) != len(claim.AnchorIDs) {
					t.Fatalf("claim %d sources = %d, want %d", index, len(got.Sources), len(claim.AnchorIDs))
				}
			}
			if len(projected.Uncertainties) != len(episode.Uncertainties) {
				t.Fatalf("uncertainties = %d, want %d", len(projected.Uncertainties), len(episode.Uncertainties))
			}

			rendered, err := RenderHTMLWithSourceEpisode(data, raw)
			if err != nil {
				t.Fatalf("render source episode: %v", err)
			}
			for _, required := range []string{
				`"source_episode":`,
				episode.EpisodeID,
				episode.Question,
				episode.Claims[0].Title,
				episode.Claims[len(episode.Claims)-1].Statement,
				episode.Uncertainties[0].Statement,
				"renderSourceEpisode",
				"rm-source-episode__state--inferred",
				"rm-source-episode__state--unknown",
			} {
				if !bytes.Contains(rendered, []byte(required)) {
					t.Fatalf("rendered HTML is missing %q", required)
				}
			}
			if bytes.Contains(rendered, []byte("/Users/")) {
				t.Fatal("rendered source episode contains an absolute local path")
			}

			baseline, err := RenderHTML(data)
			if err != nil {
				t.Fatalf("render baseline: %v", err)
			}
			for _, forbidden := range []string{`"source_episode":`, episode.EpisodeID, "renderSourceEpisode"} {
				if bytes.Contains(baseline, []byte(forbidden)) {
					t.Fatalf("ordinary RenderHTML gained transient source episode token %q", forbidden)
				}
			}

			persistedPath := filepath.Join(t.TempDir(), "report.json")
			if err := WriteReportJSON(data, persistedPath); err != nil {
				t.Fatalf("persist report: %v", err)
			}
			persisted, err := os.ReadFile(persistedPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"source_episode", episode.EpisodeID, episode.Question} {
				if bytes.Contains(persisted, []byte(forbidden)) {
					t.Fatalf("persisted report contains transient token %q", forbidden)
				}
			}
		})
	}
}

func TestSourceEpisodeProjectionPreservesWeakSignalsWithoutSourceAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path        string
		claimTitle  string
		claimState  string
		uncertainty string
	}{
		{
			path:        filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json"),
			claimTitle:  "The WAL-side recovery bytes are a raft entry carrying the encoded request",
			claimState:  "inferred",
			uncertainty: "Successful handler return does not prove when the client process receives response bytes.",
		},
		{
			path:        filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
			claimTitle:  "Treat `on_commit()` as a post-commit handoff, not a delivery guarantee",
			claimState:  "inferred",
			uncertainty: "The in-memory callback queue does not establish delivery if the process exits after database commit but before or during callback execution.",
		},
	}

	for _, test := range tests {
		raw, episode := readSourceEpisodeFixture(t, test.path)
		data := &ReportData{
			FormatVersion:    CurrentFormatVersion,
			RepoName:         episode.Repository.Name,
			CapturedRevision: episode.Repository.Revision,
		}
		projected, err := projectApprovedSourceEpisode(data, raw)
		if err != nil {
			t.Fatalf("project episode without source authority: %v", err)
		}
		if len(projected.Claims) != len(episode.Claims) {
			t.Fatalf("claims without source authority = %d, want %d", len(projected.Claims), len(episode.Claims))
		}
		foundClaim := false
		for _, claim := range projected.Claims {
			if len(claim.Sources) != 0 {
				t.Fatalf("claim %q exposed an unauthorized source action", claim.Title)
			}
			if claim.Title == test.claimTitle {
				foundClaim = claim.State == test.claimState
			}
		}
		if !foundClaim {
			t.Fatalf("weak claim %q was hidden or promoted", test.claimTitle)
		}
		foundUncertainty := false
		for _, uncertainty := range projected.Uncertainties {
			if len(uncertainty.Sources) != 0 {
				t.Fatal("uncertainty exposed an unauthorized source action")
			}
			if uncertainty.Statement == test.uncertainty && uncertainty.State == "unknown" {
				foundUncertainty = true
			}
		}
		if !foundUncertainty {
			t.Fatalf("unknown boundary %q is missing", test.uncertainty)
		}
	}
}

func TestSourceEpisodeValidationPublishesAnchorlessWeakSignals(t *testing.T) {
	t.Parallel()

	raw, episode := readSourceEpisodeFixture(
		t,
		filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
	)
	approval := approvedSourceEpisodes[sourceEpisodeFixtureDigest(t, raw)]
	episode.Claims = append([]sourceEpisodeClaim(nil), episode.Claims...)
	episode.Uncertainties = append([]sourceEpisodeUncertainty(nil), episode.Uncertainties...)

	weakClaimIndex := len(episode.Claims) - 1
	if episode.Claims[weakClaimIndex].State != "inferred" {
		t.Fatalf("fixture weak claim state = %q", episode.Claims[weakClaimIndex].State)
	}
	episode.Claims[weakClaimIndex].AnchorIDs = nil
	episode.Uncertainties[0].AnchorIDs = nil
	if err := validateSourceEpisode(episode, approval); err != nil {
		t.Fatalf("anchorless weak signals were rejected: %v", err)
	}

	episode.Claims[0].AnchorIDs = nil
	if err := validateSourceEpisode(episode, approval); err == nil {
		t.Fatal("anchorless extracted claim was accepted")
	}
}

func TestSourceEpisodeProjectionRejectsUnapprovedOrUnsafeInput(t *testing.T) {
	t.Parallel()

	raw, episode := readSourceEpisodeFixture(
		t,
		filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
	)
	data := authorizedSourceEpisodeReport(episode)

	mismatched := *data
	mismatched.CapturedRevision = strings.Repeat("0", 40)
	if _, err := projectApprovedSourceEpisode(&mismatched, raw); err == nil {
		t.Fatal("episode was rendered over a different report revision")
	}
	mismatched.CapturedRevision = ""
	if _, err := projectApprovedSourceEpisode(&mismatched, raw); err == nil {
		t.Fatal("episode was rendered over a report without a captured revision")
	}

	mutated := append([]byte(nil), raw...)
	mutated[len(mutated)-2] = ' '
	if _, err := projectApprovedSourceEpisode(data, mutated); err == nil {
		t.Fatal("unapproved digest was accepted")
	}
	if _, err := projectApprovedSourceEpisode(data, bytes.Repeat([]byte("x"), maxSourceEpisodeBytes+1)); err == nil {
		t.Fatal("oversized episode was accepted")
	}

	approval := approvedSourceEpisodes[sourceEpisodeFixtureDigest(t, raw)]
	broken := episode
	broken.Anchors = append([]sourceEpisodeAnchor(nil), episode.Anchors...)
	broken.Anchors[0].Path = "/tmp/escape.py"
	if err := validateSourceEpisode(broken, approval); err == nil {
		t.Fatal("absolute source path was accepted")
	}
	broken = episode
	broken.Claims = append([]sourceEpisodeClaim(nil), episode.Claims...)
	broken.Claims[0].State = "verified"
	if err := validateSourceEpisode(broken, approval); err == nil {
		t.Fatal("unknown trust state was accepted")
	}
	broken = episode
	broken.Claims = append([]sourceEpisodeClaim(nil), episode.Claims...)
	broken.Claims[0].AnchorIDs = []string{"missing-anchor"}
	if err := validateSourceEpisode(broken, approval); err == nil {
		t.Fatal("missing anchor reference was accepted")
	}
	broken = episode
	broken.Anchors = append([]sourceEpisodeAnchor(nil), episode.Anchors...)
	broken.Anchors[1].ID = broken.Anchors[0].ID
	if err := validateSourceEpisode(broken, approval); err == nil {
		t.Fatal("duplicate anchor ID was accepted")
	}
}

func TestSourceEpisodeStudyDOMShowsOrderedTrustAndAuthority(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = String(tag).toUpperCase();
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.parentNode = null;
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  replaceChildren(...children) { this.children = []; this.append(...children); }
}
const report = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
window.history = {
  state: null,
  pushState(state) { this.state = state; },
  replaceState(state) { this.state = state; },
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function walk(root) {
  const result = [];
  (function visit(node) {
    result.push(node);
    (node.children || []).forEach(visit);
  })(root);
  return result;
}
function text(node) {
  return String(node.textContent || "") + (node.children || []).map(text).join("");
}
const root = api.renderSourceEpisode(report.source_episode);
const nodes = walk(root);
const claimTitles = nodes
  .filter((node) => String(node.className).includes("rm-source-episode__claim-heading"))
  .map((node) => (node.children.find((child) => child.tagName === "H3") || {}).textContent || "");
const states = nodes
  .filter((node) => String(node.className).includes("rm-source-episode__state "))
  .map((node) => node.textContent);
const sourceButtons = nodes.filter((node) =>
  String(node.className).split(/\s+/).includes("rm-source-episode__source"));
if (sourceButtons.length) sourceButtons[0].onclick();
const openedSource = api.workspaceStateSnapshot().sourceLocation;
report.source_episode.claims.forEach((claim) => { claim.sources = []; });
report.source_episode.uncertainties.forEach((uncertainty) => { uncertainty.sources = []; });
const withoutAuthority = api.renderSourceEpisode(report.source_episode);
const withoutAuthorityButtons = walk(withoutAuthority)
  .filter((node) => String(node.className).split(/\s+/).includes("rm-source-episode__source"));
process.stdout.write(JSON.stringify({
  text: text(root),
  claimTitles,
  states,
  sourceButtonCount: sourceButtons.length,
  sourceButtonsAreDirect: sourceButtons.every((node) => typeof node.onclick === "function"),
  openedSourcePath: openedSource && openedSource.path || "",
  withoutAuthorityButtonCount: withoutAuthorityButtons.length,
  preCount: nodes.filter((node) => node.tagName === "PRE").length,
  route: window.location.hash,
}));
`
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		path     string
		required []string
	}{
		{
			name: "go etcd",
			path: filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json"),
			required: []string{
				"The WAL-side recovery bytes are a raft entry carrying the encoded request",
				"Client acknowledgment and this Ready loop's WAL Save completion are not ordered here",
				"The selected code does not determine whether the local WAL Save relevant to this Put has completed",
			},
		},
		{
			name: "python django",
			path: filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
			required: []string{
				"Treat `on_commit()` as a post-commit handoff, not a delivery guarantee",
				"The in-memory callback queue does not establish delivery",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			raw, episode := readSourceEpisodeFixture(t, test.path)
			data := authorizedSourceEpisodeReport(episode)
			projected, err := projectApprovedSourceEpisode(data, raw)
			if err != nil {
				t.Fatal(err)
			}
			firstAnchor := episode.Anchors[0]
			reportJSON, err := json.Marshal(struct {
				RepoName      string                   `json:"repo_name"`
				OpenablePaths []string                 `json:"openable_paths"`
				SourceIDs     map[string]string        `json:"source_ids"`
				UserSources   []map[string]any         `json:"user_sources"`
				SourceEpisode *sourceEpisodeProjection `json:"source_episode"`
			}{
				RepoName:      episode.Repository.Name,
				OpenablePaths: data.OpenablePaths,
				SourceIDs:     data.SourceIDs,
				UserSources: []map[string]any{{
					"path":       firstAnchor.Path,
					"start_line": firstAnchor.StartLine,
					"end_line":   firstAnchor.EndLine,
					"lines": []map[string]any{{
						"line": firstAnchor.StartLine,
						"text": "synthetic saved source for browser-state testing",
					}},
				}},
				SourceEpisode: projected,
			})
			if err != nil {
				t.Fatal(err)
			}
			tempDir := t.TempDir()
			runnerPath := filepath.Join(tempDir, "source-episode-dom.js")
			reportPath := filepath.Join(tempDir, "source-episode-report.json")
			if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(reportPath, reportJSON, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command(node, runnerPath, assetPath, reportPath).CombinedOutput()
			if err != nil {
				t.Fatalf("run source episode DOM test: %v\n%s", err, output)
			}
			var got struct {
				Text                        string   `json:"text"`
				ClaimTitles                 []string `json:"claimTitles"`
				States                      []string `json:"states"`
				SourceButtonCount           int      `json:"sourceButtonCount"`
				SourceButtonsAreDirect      bool     `json:"sourceButtonsAreDirect"`
				OpenedSourcePath            string   `json:"openedSourcePath"`
				WithoutAuthorityButtonCount int      `json:"withoutAuthorityButtonCount"`
				PreCount                    int      `json:"preCount"`
				Route                       string   `json:"route"`
			}
			if err := json.Unmarshal(output, &got); err != nil {
				t.Fatalf("decode DOM result: %v\n%s", err, output)
			}
			wantTitles := make([]string, 0, len(episode.Claims))
			for _, claim := range episode.Claims {
				wantTitles = append(wantTitles, claim.Title)
			}
			if strings.Join(got.ClaimTitles, "\n") != strings.Join(wantTitles, "\n") {
				t.Fatalf("claim order drifted:\n got %q\nwant %q", got.ClaimTitles, wantTitles)
			}
			for _, label := range []string{"EXTRACTED", "CORROBORATED", "INFERRED", "UNKNOWN"} {
				if !sourceEpisodeContainsString(got.States, label) {
					t.Fatalf("visible trust states %q are missing %q", got.States, label)
				}
			}
			required := append([]string{episode.Question, "What remains uncertain"}, test.required...)
			for _, value := range required {
				if !strings.Contains(got.Text, value) {
					t.Fatalf("DOM text is missing %q", value)
				}
			}
			if got.SourceButtonCount == 0 || !got.SourceButtonsAreDirect {
				t.Fatalf("authorized one-click source controls = %d, direct=%t", got.SourceButtonCount, got.SourceButtonsAreDirect)
			}
			if got.OpenedSourcePath != firstAnchor.Path {
				t.Fatalf("source control opened %q, want %q", got.OpenedSourcePath, firstAnchor.Path)
			}
			if got.WithoutAuthorityButtonCount != 0 {
				t.Fatalf("missing SourceIDs left %d broken source controls", got.WithoutAuthorityButtonCount)
			}
			if got.PreCount != 0 {
				t.Fatal("source code was placed on first paint")
			}
			if got.Route != "#/overview" {
				t.Fatalf("episode rendering created a route: %q", got.Route)
			}
		})
	}

	css, err := os.ReadFile(filepath.Join("templates", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		".rm-source-episode__state--inferred",
		".rm-source-episode__state--unknown",
		".rm-source-episode__uncertainty-list { grid-template-columns: minmax(0, 1fr); }",
	} {
		if !bytes.Contains(css, []byte(token)) {
			t.Fatalf("source episode CSS is missing %q", token)
		}
	}
}

func TestWriteSourceEpisodePreviews(t *testing.T) {
	previewDir := strings.TrimSpace(os.Getenv("REPOMAP_SOURCE_EPISODE_PREVIEW_DIR"))
	if previewDir == "" {
		t.Skip("set REPOMAP_SOURCE_EPISODE_PREVIEW_DIR to write standalone review HTML")
	}
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
	}{
		{"01-etcd-put.html", filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json")},
		{"02-django-atomic.html", filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json")},
	}
	for _, test := range tests {
		raw, episode := readSourceEpisodeFixture(t, test.path)
		data := authorizedSourceEpisodeReport(episode)
		data.StudyMap = &RepositoryStudyMap{}
		data.SourceIDs = nil
		html, err := RenderHTMLWithSourceEpisode(data, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(previewDir, test.name), html, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readSourceEpisodeFixture(t *testing.T, fixturePath string) ([]byte, sourceEpisodeInput) {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var episode sourceEpisodeInput
	if err := json.Unmarshal(raw, &episode); err != nil {
		t.Fatal(err)
	}
	return raw, episode
}

func authorizedSourceEpisodeReport(episode sourceEpisodeInput) *ReportData {
	paths := make([]string, 0, len(episode.Anchors))
	sourceIDs := make(map[string]string, len(episode.Anchors))
	seen := make(map[string]struct{}, len(episode.Anchors))
	for index, anchor := range episode.Anchors {
		if _, ok := seen[anchor.Path]; ok {
			continue
		}
		seen[anchor.Path] = struct{}{}
		paths = append(paths, anchor.Path)
		sourceIDs[anchor.Path] = "opaque-source-" + string(rune('a'+index))
	}
	return &ReportData{
		FormatVersion:    CurrentFormatVersion,
		RepoName:         episode.Repository.Name,
		CapturedRevision: episode.Repository.Revision,
		OpenablePaths:    paths,
		SourceIDs:        sourceIDs,
	}
}

func sourceEpisodeFixtureDigest(t *testing.T, raw []byte) string {
	t.Helper()
	for digest, approval := range approvedSourceEpisodes {
		if approval.episodeID == func() string {
			var episode sourceEpisodeInput
			if err := json.Unmarshal(raw, &episode); err != nil {
				t.Fatal(err)
			}
			return episode.EpisodeID
		}() {
			return digest
		}
	}
	t.Fatal("fixture is not approved")
	return ""
}

func sourceEpisodeContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
