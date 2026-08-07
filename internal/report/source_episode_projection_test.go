package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestSourceEpisodeFixturesRemainPinnedAndValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		digest   string
		approval sourceEpisodeApproval
	}{
		{
			name:   "go etcd",
			path:   filepath.Join("testdata", "source_episode", "etcd-put", "episode.json"),
			digest: "1f41085eea5fc0c59ddbb7ae66b7e3a67c82b8b588babd97edfe71ec873aa21a",
			approval: sourceEpisodeApproval{
				episodeID:  "etcd-put-recoverability",
				repository: "etcd-io/etcd",
				revision:   "58f45a9ff1c083130830eb02b0cc7d9783609095",
			},
		},
		{
			name:   "python django",
			path:   filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
			digest: "9599553a777e8d8fd582bb1874dd4ab534c1f24d9d87e82cfce09cc775281665",
			approval: sourceEpisodeApproval{
				episodeID:  "django-nested-atomic",
				repository: "django/django",
				revision:   "3e389b7ddaf08109900da5415ddaac5a355a170f",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			raw, episode := readSourceEpisodeFixture(t, test.path)
			digest := sha256.Sum256(raw)
			if got := hex.EncodeToString(digest[:]); got != test.digest {
				t.Fatalf("fixture SHA-256 = %s, want %s", got, test.digest)
			}
			approval, ok := approvedSourceEpisodes[test.digest]
			if !ok {
				t.Fatalf("fixture digest %s is absent from the product approval catalog", test.digest)
			}
			if approval != test.approval {
				t.Fatalf("fixture approval = %#v, want %#v", approval, test.approval)
			}
			if err := validateSourceEpisode(episode, approval); err != nil {
				t.Fatalf("fixture violates the product source-episode schema: %v", err)
			}
		})
	}
}

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
			path:       filepath.Join("testdata", "source_episode", "etcd-put", "episode.json"),
			episodeID:  "etcd-put-recoverability",
			repository: "etcd-io/etcd",
		},
		{
			name:       "python django",
			path:       filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
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

func TestGenerateAuthorizedWithSourceEpisodeKeepsPersistedAuthorityOrdinary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{
			name: "go etcd",
			path: filepath.Join("testdata", "source_episode", "etcd-put", "episode.json"),
		},
		{
			name: "python django",
			path: filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			raw, episode := readSourceEpisodeFixture(t, test.path)
			runDir, authority := sourceEpisodeGenerationFixture(t, episode)
			if err := GenerateAuthorized(runDir, authority); err != nil {
				t.Fatalf("generate ordinary authorized report: %v", err)
			}
			ordinaryJSON := readSourceEpisodeGeneratedFile(t, runDir, "report.json")
			ordinaryManifest := readSourceEpisodeGeneratedFile(t, runDir, RunManifestFilename)
			ordinaryHTML := readSourceEpisodeGeneratedFile(t, runDir, "report.html")

			if err := GenerateAuthorizedWithSourceEpisode(runDir, authority, raw); err != nil {
				t.Fatalf("generate authorized source episode: %v", err)
			}
			episodeJSON := readSourceEpisodeGeneratedFile(t, runDir, "report.json")
			episodeManifest := readSourceEpisodeGeneratedFile(t, runDir, RunManifestFilename)
			episodeHTML := readSourceEpisodeGeneratedFile(t, runDir, "report.html")

			if !bytes.Equal(episodeJSON, ordinaryJSON) {
				t.Fatal("source episode changed persisted report.json")
			}
			if !bytes.Equal(episodeManifest, ordinaryManifest) {
				t.Fatal("source episode changed the report manifest")
			}
			if bytes.Equal(episodeHTML, ordinaryHTML) {
				t.Fatal("source episode did not change transient report.html")
			}
			for _, required := range []string{
				`"source_episode":`,
				episode.EpisodeID,
				episode.Question,
				episode.Claims[0].Statement,
			} {
				if !bytes.Contains(episodeHTML, []byte(required)) {
					t.Fatalf("transient report.html is missing %q", required)
				}
			}
			for _, forbidden := range []string{
				`"semantic_search":`,
				`id="rm-semantic-search-css"`,
				`id="rm-semantic-search-js"`,
				`id="rm-semantic-search-entry"`,
			} {
				if bytes.Contains(episodeHTML, []byte(forbidden)) {
					t.Fatalf("transient report.html retained Search payload %q", forbidden)
				}
			}
			for _, forbidden := range []string{
				`"source_episode":`,
				episode.EpisodeID,
				episode.Question,
			} {
				if bytes.Contains(episodeJSON, []byte(forbidden)) {
					t.Fatalf("persisted report.json contains transient token %q", forbidden)
				}
				if bytes.Contains(episodeManifest, []byte(forbidden)) {
					t.Fatalf("report manifest contains transient token %q", forbidden)
				}
			}
			manifest, err := ReadRunManifest(runDir)
			if err != nil {
				t.Fatalf("read generated manifest: %v", err)
			}
			if err := manifest.VerifyReportJSON(episodeJSON); err != nil {
				t.Fatalf("verify ordinary report binding: %v", err)
			}

			if err := GenerateAuthorized(runDir, authority); err != nil {
				t.Fatalf("regenerate ordinary authorized report: %v", err)
			}
			if regenerated := readSourceEpisodeGeneratedFile(t, runDir, "report.html"); !bytes.Equal(regenerated, ordinaryHTML) {
				t.Fatal("ordinary GenerateAuthorized HTML changed after transient generation")
			}
		})
	}
}

func TestSourceEpisodeAuthorityOmitsOnlyCapturedSymlinkPath(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	data := &ReportData{OpenablePaths: []string{"linked.go", "regular.go"}}
	authority := RunAuthority{
		analysisRoot: analysisRoot,
		repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repository,
			Head:     strings.Repeat("a", 40),
		},
		inputs: []freshness.CapturedInput{
			{
				Version: freshness.CapturedInputVersion, ID: strings.Repeat("b", 64),
				Path: "service/linked.go", Kind: freshness.FileSymlink,
				Mode: "120000", ContentSHA256: strings.Repeat("c", 64),
				Stages: []string{"report_evidence"},
			},
			{
				Version: freshness.CapturedInputVersion, ID: strings.Repeat("d", 64),
				Path: "service/regular.go", Kind: freshness.FileRegular,
				Mode: "100644", ContentSHA256: strings.Repeat("e", 64),
				Stages: []string{"report_evidence"},
			},
		},
	}

	if err := retainSourceEpisodeRegularOpenablePaths(data, authority); err != nil {
		t.Fatalf("retain regular source episode paths: %v", err)
	}
	if len(data.OpenablePaths) != 1 || data.OpenablePaths[0] != "regular.go" {
		t.Fatalf("retained openable paths = %#v, want regular.go only", data.OpenablePaths)
	}
	if authority.inputs[0].Kind != freshness.FileSymlink || len(authority.inputs) != 2 {
		t.Fatalf("source episode filtering mutated captured authority: %#v", authority.inputs)
	}
	catalog, available, err := authorizedExactSearchCatalog(data, authority)
	if err != nil || !available {
		t.Fatalf("regular subset catalog available=%t err=%v", available, err)
	}
	if catalog.Len() != 1 {
		t.Fatalf("regular subset catalog length = %d, want 1", catalog.Len())
	}
	if _, ok := catalog.Lookup("regular.go"); !ok {
		t.Fatal("regular captured path is missing from source episode catalog")
	}
	if _, ok := catalog.Lookup("linked.go"); ok {
		t.Fatal("tracked symlink reached source episode catalog")
	}
}

func TestGenerateAuthorizedWithSourceEpisodeRejectsCrossRevisionBeforeMutation(t *testing.T) {
	t.Parallel()

	raw, episode := readSourceEpisodeFixture(
		t,
		filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
	)
	runDir, authority := sourceEpisodeGenerationFixture(t, episode)
	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("generate ordinary authorized report: %v", err)
	}
	beforeJSON := readSourceEpisodeGeneratedFile(t, runDir, "report.json")
	beforeHTML := readSourceEpisodeGeneratedFile(t, runDir, "report.html")
	beforeManifest := readSourceEpisodeGeneratedFile(t, runDir, RunManifestFilename)

	mismatched := authority
	mismatched.repository.Head = strings.Repeat("0", 40)
	if err := GenerateAuthorizedWithSourceEpisode(runDir, mismatched, raw); err == nil {
		t.Fatal("cross-revision source episode was accepted")
	}
	if got := readSourceEpisodeGeneratedFile(t, runDir, "report.json"); !bytes.Equal(got, beforeJSON) {
		t.Fatal("cross-revision rejection changed report.json")
	}
	if got := readSourceEpisodeGeneratedFile(t, runDir, "report.html"); !bytes.Equal(got, beforeHTML) {
		t.Fatal("cross-revision rejection changed report.html")
	}
	if got := readSourceEpisodeGeneratedFile(t, runDir, RunManifestFilename); !bytes.Equal(got, beforeManifest) {
		t.Fatal("cross-revision rejection changed the report manifest")
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
			path:        filepath.Join("testdata", "source_episode", "etcd-put", "episode.json"),
			claimTitle:  "The WAL-side recovery bytes are a raft entry carrying the encoded request",
			claimState:  "inferred",
			uncertainty: "Successful handler return does not prove when the client process receives response bytes.",
		},
		{
			path:        filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
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
		filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
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
		filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
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
  getAttribute(name) { return this.attributes[name] || null; }
  removeAttribute(name) { delete this.attributes[name]; }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  replaceChildren(...children) { this.children = []; this.append(...children); }
}
const report = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const listeners = {};
const elements = {
  "rm-overview": new Element("section"),
  "rm-tabs": new Element("nav"),
};
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  RepomapSemanticSearch: { mount() { return { destroy() {} }; } },
  addEventListener(name, callback) { listeners[name] = callback; },
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
    return elements[id] || null;
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
if (listeners.DOMContentLoaded) listeners.DOMContentLoaded();
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
const sourceLinks = nodes.filter((node) =>
  String(node.className).split(/\s+/).includes("rm-source-episode__source"));
const sourceButtons = sourceLinks.filter((node) => node.tagName === "A");
if (sourceButtons.length) sourceButtons[0].onclick && sourceButtons[0].onclick();
const openedSourcePath = sourceButtons[0] ? (sourceButtons[0].attributes.href || "") : "";
report.source_episode.claims.forEach((claim) => { claim.sources = []; });
report.source_episode.uncertainties.forEach((uncertainty) => { uncertainty.sources = []; });
const withoutAuthority = api.renderSourceEpisode(report.source_episode);
const withoutAuthorityButtons = walk(withoutAuthority)
  .filter((node) => String(node.className).split(/\s+/).includes("rm-source-episode__source"));
const overviewSearchDestinations = walk(elements["rm-overview"]).filter((node) =>
  node.tagName === "BUTTON" && text(node).trim() === "Search").length;
const tabSearchDestinations = elements["rm-tabs"].children.filter((node) =>
  node.attributes["data-workspace-view"] === "search").length;
const directSearch = api.parseWorkspaceHash("#/search", [], null);
process.stdout.write(JSON.stringify({
  text: text(root),
  claimTitles,
  states,
  sourceButtonCount: sourceButtons.length,
  sourceButtonsAreDirect: sourceButtons.every((node) => typeof node.attributes.href === "string" && node.attributes.href.length > 0),
  openedSourcePath: openedSourcePath,
  withoutAuthorityButtonCount: withoutAuthorityButtons.length,
  overviewSearchDestinations,
  tabSearchDestinations,
  directSearchValid: directSearch.valid,
  directSearchCanonicalHash: directSearch.canonicalHash,
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
			path: filepath.Join("testdata", "source_episode", "etcd-put", "episode.json"),
			required: []string{
				"The WAL-side recovery bytes are a raft entry carrying the encoded request",
				"Client acknowledgment and this Ready loop's WAL Save completion are not ordered here",
				"The selected code does not determine whether the local WAL Save relevant to this Put has completed",
			},
		},
		{
			name: "python django",
			path: filepath.Join("testdata", "source_episode", "django-atomic", "episode.json"),
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
				RepoName       string                   `json:"repo_name"`
				OpenablePaths  []string                 `json:"openable_paths"`
				SourceIDs      map[string]string        `json:"source_ids"`
				GitHubSource   map[string]any           `json:"github_source_links"`
				UserSources    []map[string]any         `json:"user_sources"`
				SemanticSearch map[string]any           `json:"semantic_search"`
				SourceEpisode  *sourceEpisodeProjection `json:"source_episode"`
			}{
				RepoName:      episode.Repository.Name,
				OpenablePaths: data.OpenablePaths,
				SourceIDs:     data.SourceIDs,
				GitHubSource: map[string]any{
					"repository_url": "https://github.com/example/fixture",
					"revision":       strings.Repeat("1", 40),
				},
				SemanticSearch: map[string]any{
					"version": 1,
				},
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
				OverviewSearchDestinations  int      `json:"overviewSearchDestinations"`
				TabSearchDestinations       int      `json:"tabSearchDestinations"`
				DirectSearchValid           bool     `json:"directSearchValid"`
				DirectSearchCanonicalHash   string   `json:"directSearchCanonicalHash"`
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
			if !strings.Contains(got.OpenedSourcePath, "github.com/example/fixture/blob") {
				t.Fatalf("source control opened %q, want GitHub blob jump", got.OpenedSourcePath)
			}
			if got.WithoutAuthorityButtonCount != 0 {
				t.Fatalf("missing SourceIDs left %d broken source controls", got.WithoutAuthorityButtonCount)
			}
			if got.OverviewSearchDestinations != 0 || got.TabSearchDestinations != 0 {
				t.Fatalf(
					"source episode retained Search destinations: overview=%d tabs=%d",
					got.OverviewSearchDestinations,
					got.TabSearchDestinations,
				)
			}
			if got.DirectSearchValid || got.DirectSearchCanonicalHash != "#/overview" {
				t.Fatalf(
					"source episode direct Search route = valid %t canonical %q",
					got.DirectSearchValid,
					got.DirectSearchCanonicalHash,
				)
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
		{"01-etcd-put.html", filepath.Join("testdata", "source_episode", "etcd-put", "episode.json")},
		{"02-django-atomic.html", filepath.Join("testdata", "source_episode", "django-atomic", "episode.json")},
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

func sourceEpisodeGenerationFixture(
	t *testing.T,
	episode sourceEpisodeInput,
) (string, RunAuthority) {
	t.Helper()

	repository := newRunManifestRepository(t)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initial, current)
	if err != nil {
		t.Fatal(err)
	}
	// The pinned fixtures name immutable external revisions. This local
	// generation fixture exercises report assembly without reading or running
	// either target repository.
	authority.repository.Head = episode.Repository.Revision

	runDir := t.TempDir()
	writeRunManifestMetadata(t, runDir, repository)
	snapshot, err := json.Marshal(map[string]any{
		"repo_name": episode.Repository.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshot))
	writeTestFile(t, runDir, "orientation_report.json", `{
		"project_guess":"source episode generation fixture",
		"candidate_flows":[],
		"warnings":[]
	}`)
	return runDir, authority
}

func readSourceEpisodeGeneratedFile(t *testing.T, runDir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
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
	digest := sha256.Sum256(raw)
	encoded := hex.EncodeToString(digest[:])
	if _, ok := approvedSourceEpisodes[encoded]; !ok {
		t.Fatalf("fixture SHA-256 %s is not approved", encoded)
	}
	return encoded
}

func sourceEpisodeContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
