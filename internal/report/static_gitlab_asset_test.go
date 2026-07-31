package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStaticGitLabAssetRoutesSourceWithoutLocalAPIs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runnerPath := filepath.Join(t.TempDir(), "static-gitlab-runner.js")
	runner := `
const fs = require("fs");
const vm = require("vm");

class Element {
  constructor(tagName) {
    this.tagName = String(tagName || "div").toUpperCase();
    this.children = [];
    this.attributes = {};
    this.className = "";
    this.hidden = true;
    this.textContent = "";
    this.style = {};
    this.parentElement = null;
    this.classList = {
      add: (...names) => {
        const values = new Set(String(this.className || "").split(/\s+/).filter(Boolean));
        names.forEach((name) => values.add(name));
        this.className = Array.from(values).join(" ");
      },
      remove: (...names) => {
        const removed = new Set(names);
        this.className = String(this.className || "").split(/\s+/).filter((name) => name && !removed.has(name)).join(" ");
      },
      toggle: (name, force) => {
        const values = new Set(String(this.className || "").split(/\s+/).filter(Boolean));
        const enabled = force === undefined ? !values.has(name) : !!force;
        if (enabled) values.add(name); else values.delete(name);
        this.className = Array.from(values).join(" ");
        return enabled;
      },
      contains: (name) => String(this.className || "").split(/\s+/).includes(name),
    };
  }
  appendChild(child) {
    if (child) {
      child.parentElement = this;
      this.children.push(child);
    }
    return child;
  }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) {
    if (!child) return;
    child.parentElement = this;
    this.children.unshift(child);
  }
  replaceChildren(...children) {
    this.children = [];
    this.textContent = "";
    this.append(...children);
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] || ""; }
  hasAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name); }
  removeAttribute(name) { delete this.attributes[name]; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  remove() {}
  get childNodes() { return this.children; }
}

const elements = {
  "rm-report-data": new Element("script"),
  "rm-editor-hint": new Element("span"),
  "rm-toast": new Element("div"),
  "rm-architecture": new Element("section"),
  "rm-task-investigation": new Element("section"),
};
elements["rm-report-data"].textContent = JSON.stringify({
  report_language: "ru",
  openable_paths: ["dir/space #.go", "local/changed.go"],
  gitlab_source_links: {
    repository_url: "https://gitlab.example/team/sub/project",
    revision: "0123456789abcdef0123456789abcdef01234567",
    path_prefix: "nested worktree",
    working_tree_dirty: true,
    working_tree_paths: ["local/changed.go"],
  },
  architecture_canvas: {
    groups: [],
    components: [{
      id: "component-one",
      anchor_ids: ["anchor-one"],
      members: [],
      name: "Component one",
    }],
    behavior_anchors: [{
      id: "anchor-one",
      label: "Exact component anchor",
      location: { path: "dir/space #.go", line: 12, column: 4 },
    }],
    relations: [],
    surfaces: [],
  },
  discovered_surfaces: { surfaces: [] },
  user_sources: [],
  user_mechanisms: [],
  user_topics: [],
  task_investigation: {
    task: "",
    sufficient: true,
    locality: "single_file",
    interpretation: {},
    anchors: [{
      path: "dir/space #.go",
      symbol: "Serve",
      role: "representative_implementation",
      start_line: 12,
      end_line: 18,
      why: "Exact static anchor.",
      source: {
        path: "dir/space #.go",
        start_line: 12,
        end_line: 18,
        enclosing_symbol: "Serve",
        lines: [],
      },
    }],
  },
});

const opened = [];
let fetchCount = 0;
let architectureOptions = null;
let surfaceOptions = null;
const document = {
  body: new Element("body"),
  documentElement: new Element("html"),
  getElementById(id) { return elements[id] || null; },
  createElement(tag) { return new Element(tag); },
  createTextNode(value) {
    const node = new Element("#text");
    node.textContent = String(value);
    return node;
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
};
const window = {
  __REPOMAP_WORKSPACE_TEST__: {},
  location: {
    hostname: "127.0.0.1",
    protocol: "http:",
    pathname: "/_repomap/capability/runs/run/report.html",
    search: "?debug=1",
    hash: "",
  },
  history: { state: null, pushState() {}, replaceState() {} },
  addEventListener() {},
  setTimeout,
  clearTimeout,
  open(url, target, features) { opened.push({ url, target, features }); },
  RepomapArchitectureCanvas: {
    mount(host, data, options) {
      architectureOptions = options;
      return { ready: Promise.resolve(), destroy() {} };
    },
  },
  RepomapSurfaceCatalog: {
    mount(host, data, options) {
      surfaceOptions = options;
      return { destroy() {} };
    },
  },
};
const sandbox = {
  console,
  document,
  window,
  navigator: { clipboard: { writeText: () => Promise.resolve() } },
  URLSearchParams,
  Set,
  Node: { TEXT_NODE: 3, ELEMENT_NODE: 1, DOCUMENT_NODE: 9 },
  NodeFilter: { SHOW_TEXT: 4 },
  MutationObserver: class { observe() {} },
  fetch() {
    fetchCount++;
    throw new Error("static GitLab mode must not fetch localhost APIs");
  },
  setTimeout,
  clearTimeout,
};
vm.createContext(sandbox);
document.documentElement.lang = "en";
window.document = document;
vm.runInContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), sandbox);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandbox);

const api = window.__REPOMAP_WORKSPACE_TEST__;
const location = { path: "dir/space #.go", line: 12 };
const snippet = {
  path: location.path,
  start_line: 12,
  end_line: 18,
  enclosing_symbol: "Serve",
  lines: [],
};
const directURL = api.gitLabSourceURL(location.path, 12, 18);
const dirtyLocation = { path: "local/changed.go", line: 7 };
const dirtySnippet = {
  path: dirtyLocation.path,
  start_line: 7,
  end_line: 11,
  enclosing_symbol: "Changed",
  lines: [],
};
const dirtyURL = api.gitLabSourceURL(dirtyLocation.path, 7, 11);
const dirtyReference = api.renderFileReference(
  dirtyLocation.path,
  "source-ref",
  7,
  dirtyLocation.path + ":7"
);
const dirtyCard = api.renderSourceSnippetCard(dirtySnippet, { location: dirtyLocation });
const reference = api.renderFileReference(location.path, "source-ref", 12, location.path + ":12");
const card = api.renderSourceSnippetCard(snippet, { location });
const studyCard = api.renderStudyReadingAnchor({
  label: "Study anchor",
  location,
  source: snippet,
}, 0);
const operationalCard = api.renderOperationalLandmark({
  label: "Operational anchor",
  reference: { location, source: snippet },
});
const repositoryArea = api.renderRepositoryArea({
  label: "Repository area",
  code_location: location,
});
const componentContext = api.architectureComponentContexts()["component-one"];

function walk(node) {
  if (!node) return [];
  return [node].concat((node.children || []).flatMap(walk));
}
function text(node) {
  return String(node && node.textContent || "") + (node && node.children || []).map(text).join("");
}

api.setupServerFeatures();
const dirtyCardNodes = walk(dirtyCard);
const dirtyAction = dirtyCardNodes.find((node) => node.tagName === "BUTTON");
if (dirtyAction && typeof dirtyAction.onclick === "function") dirtyAction.onclick();
const dirtyToast = elements["rm-toast"].textContent;
api.openSourceLocation(location);
api.openReportTarget({ kind: "location", location });
api.renderTaskInvestigationWorkspace();
api.renderArchitectureWorkspace();

(async () => {
  await api.mountArchitectureCanvas();
  architectureOptions.openLocation(location.path, 23, 0);
  api.mountDebugSurfaceCatalog(new Element("section"));
  surfaceOptions.openLocation({ path: location.path, line: 31, end_line: 33 });

  const cardNodes = walk(card);
  const anchorHrefs = (node) => walk(node)
    .filter((candidate) => candidate.tagName === "A")
    .map((candidate) => candidate.attributes.href);
  process.stdout.write(JSON.stringify({
    directURL,
    dirtyURL,
    dirtyReference: {
      tagName: dirtyReference.tagName,
      href: dirtyReference.attributes.href,
      title: dirtyReference.attributes.title,
    },
    dirtySnippetAvailable: api.sourceSnippetAvailable(dirtySnippet),
    dirtyCardLinks: dirtyCardNodes.filter((node) => node.tagName === "A").map((node) => node.attributes.href),
    dirtyCardButtonTexts: dirtyCardNodes.filter((node) => node.tagName === "BUTTON").map(text),
    dirtyToast,
    mode: api.gitLabSourceMode(),
    serverMode: api.serverMode(),
    reference: {
      tagName: reference.tagName,
      href: reference.attributes.href,
      target: reference.attributes.target,
      rel: reference.attributes.rel,
    },
    snippetAvailable: api.sourceSnippetAvailable(snippet),
    cardSourceContentCount: cardNodes.filter((node) => node.attributes["data-source-content"] === "true").length,
    cardLinks: cardNodes.filter((node) => node.tagName === "A").map((node) => ({
      text: text(node),
      href: node.attributes.href,
    })),
    cardButtonTexts: cardNodes.filter((node) => node.tagName === "BUTTON").map(text),
    studyLinks: anchorHrefs(studyCard),
    operationalLinks: anchorHrefs(operationalCard),
    repositoryArea: {
      tagName: repositoryArea.tagName,
      href: repositoryArea.attributes.href,
    },
    componentSource: componentContext && componentContext.sources && componentContext.sources[0],
    taskLinks: anchorHrefs(elements["rm-task-investigation"]),
    hint: { hidden: elements["rm-editor-hint"].hidden, text: elements["rm-editor-hint"].textContent },
    fetchCount,
    opened,
    architectureCallback: typeof architectureOptions.openLocation,
    surfaceCallback: typeof surfaceOptions.openLocation,
  }));
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
`
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run static GitLab asset test: %v\n%s", err, output)
	}
	var got struct {
		DirectURL        string `json:"directURL"`
		DirtyURL         string `json:"dirtyURL"`
		Mode             bool   `json:"mode"`
		ServerMode       bool   `json:"serverMode"`
		SnippetAvailable bool   `json:"snippetAvailable"`
		DirtyReference   struct {
			TagName string `json:"tagName"`
			Href    string `json:"href"`
			Title   string `json:"title"`
		} `json:"dirtyReference"`
		DirtySnippetAvailable bool     `json:"dirtySnippetAvailable"`
		DirtyCardLinks        []string `json:"dirtyCardLinks"`
		DirtyCardButtonTexts  []string `json:"dirtyCardButtonTexts"`
		DirtyToast            string   `json:"dirtyToast"`
		Reference             struct {
			TagName string `json:"tagName"`
			Href    string `json:"href"`
			Target  string `json:"target"`
			Rel     string `json:"rel"`
		} `json:"reference"`
		CardSourceContentCount int `json:"cardSourceContentCount"`
		CardLinks              []struct {
			Text string `json:"text"`
			Href string `json:"href"`
		} `json:"cardLinks"`
		CardButtonTexts  []string `json:"cardButtonTexts"`
		StudyLinks       []string `json:"studyLinks"`
		OperationalLinks []string `json:"operationalLinks"`
		RepositoryArea   struct {
			TagName string `json:"tagName"`
			Href    string `json:"href"`
		} `json:"repositoryArea"`
		ComponentSource struct {
			Location struct {
				Path   string `json:"path"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"location"`
		} `json:"componentSource"`
		TaskLinks []string `json:"taskLinks"`
		Hint      struct {
			Hidden bool   `json:"hidden"`
			Text   string `json:"text"`
		} `json:"hint"`
		FetchCount int `json:"fetchCount"`
		Opened     []struct {
			URL      string `json:"url"`
			Target   string `json:"target"`
			Features string `json:"features"`
		} `json:"opened"`
		ArchitectureCallback string `json:"architectureCallback"`
		SurfaceCallback      string `json:"surfaceCallback"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode node output: %v\n%s", err, output)
	}

	wantRange := "https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12-18"
	if got.DirectURL != wantRange {
		t.Fatalf("GitLab URL = %q, want %q", got.DirectURL, wantRange)
	}
	if got.DirtyURL != "" || got.DirtyReference.TagName != "SPAN" || got.DirtyReference.Href != "" {
		t.Fatalf("dirty source exposed a GitLab link: URL %q reference %#v", got.DirtyURL, got.DirtyReference)
	}
	if !got.DirtySnippetAvailable || len(got.DirtyCardLinks) != 0 {
		t.Fatalf(
			"dirty source presentation = available %t links %#v",
			got.DirtySnippetAvailable,
			got.DirtyCardLinks,
		)
	}
	if !slices.Contains(got.DirtyCardButtonTexts, "Local-only source") {
		t.Fatalf("dirty source actions = %#v", got.DirtyCardButtonTexts)
	}
	if !strings.Contains(got.DirtyToast, "does not exactly match the captured GitLab commit") {
		t.Fatalf("dirty source toast = %q", got.DirtyToast)
	}
	if !got.Mode || got.ServerMode {
		t.Fatalf("mode = GitLab %v, server %v; want true, false", got.Mode, got.ServerMode)
	}
	if !got.SnippetAvailable {
		t.Fatal("metadata-only source snippet was unavailable in GitLab mode")
	}
	if got.Reference.TagName != "A" ||
		got.Reference.Href != "https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12" ||
		got.Reference.Target != "_blank" ||
		got.Reference.Rel != "noopener noreferrer" {
		t.Fatalf("source reference = %#v", got.Reference)
	}
	if got.CardSourceContentCount != 0 {
		t.Fatalf("source card rendered %d embedded code blocks", got.CardSourceContentCount)
	}
	if len(got.CardLinks) < 2 {
		t.Fatalf("source card links = %#v, want location and GitLab action", got.CardLinks)
	}
	hasGitLabAction := false
	for _, link := range got.CardLinks {
		if link.Text == "Open in GitLab" {
			hasGitLabAction = true
		}
	}
	if !hasGitLabAction {
		t.Fatalf("source card links = %#v, want English GitLab action", got.CardLinks)
	}
	wantLine := "https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12"
	for label, links := range map[string][]string{
		"Study":      got.StudyLinks,
		"Operations": got.OperationalLinks,
		"Task":       got.TaskLinks,
	} {
		if len(links) == 0 {
			t.Fatalf("%s source surface has no GitLab links", label)
		}
		for _, link := range links {
			if link != wantRange && link != wantLine {
				t.Fatalf("%s source link = %q, want %q or %q", label, link, wantLine, wantRange)
			}
		}
	}
	if got.RepositoryArea.TagName != "A" ||
		got.RepositoryArea.Href != wantLine {
		t.Fatalf("repository area = %#v", got.RepositoryArea)
	}
	if got.ComponentSource.Location.Path != "dir/space #.go" ||
		got.ComponentSource.Location.Line != 12 ||
		got.ComponentSource.Location.Column != 4 {
		t.Fatalf("component source = %#v, want exact behavior anchor", got.ComponentSource)
	}
	for _, label := range got.CardButtonTexts {
		if label == "Open in editor" || label == "Show more context" || label == "Show full function" {
			t.Fatalf("static source card retained local action %q", label)
		}
	}
	if got.Hint.Hidden || !strings.Contains(got.Hint.Text, "stable local changes") {
		t.Fatalf("GitLab hint = %#v", got.Hint)
	}
	if got.FetchCount != 0 {
		t.Fatalf("static GitLab mode made %d fetch calls", got.FetchCount)
	}
	if len(got.Opened) != 4 {
		t.Fatalf("opened URLs = %#v, want source, report target, architecture, and surface callbacks", got.Opened)
	}
	wantOpened := []string{
		"https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12",
		"https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12",
		"https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L23",
		"https://gitlab.example/team/sub/project/-/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L31-33",
	}
	for index := range wantOpened {
		if got.Opened[index].URL != wantOpened[index] ||
			got.Opened[index].Target != "_blank" ||
			got.Opened[index].Features != "noopener,noreferrer" {
			t.Fatalf("opened[%d] = %#v, want URL %q", index, got.Opened[index], wantOpened[index])
		}
	}
	if got.ArchitectureCallback != "function" || got.SurfaceCallback != "function" {
		t.Fatalf("callbacks = architecture %q, surface %q", got.ArchitectureCallback, got.SurfaceCallback)
	}

	gitHubRunner := strings.Replace(runner, "gitlab_source_links:", "github_source_links:", 1)
	gitHubRunner = strings.ReplaceAll(
		gitHubRunner,
		"https://gitlab.example/team/sub/project",
		"https://github.example/team/project",
	)
	if err := os.WriteFile(runnerPath, []byte(gitHubRunner), 0o600); err != nil {
		t.Fatal(err)
	}
	gitHubOutput, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run static GitHub asset test: %v\n%s", err, gitHubOutput)
	}
	gitHubGot := got
	if err := json.Unmarshal(gitHubOutput, &gitHubGot); err != nil {
		t.Fatalf("decode GitHub node output: %v\n%s", err, gitHubOutput)
	}
	gitHubRange := "https://github.example/team/project/blob/0123456789abcdef0123456789abcdef01234567/nested%20worktree/dir/space%20%23.go#L12-L18"
	if gitHubGot.DirectURL != gitHubRange {
		t.Fatalf("GitHub URL = %q, want %q", gitHubGot.DirectURL, gitHubRange)
	}
	if gitHubGot.DirtyURL != "" || gitHubGot.DirtyReference.TagName != "SPAN" {
		t.Fatalf("dirty source exposed a GitHub link: URL %q reference %#v", gitHubGot.DirtyURL, gitHubGot.DirtyReference)
	}
	hasGitHubAction := false
	for _, link := range gitHubGot.CardLinks {
		if link.Text == "Open in GitHub" {
			hasGitHubAction = true
		}
	}
	if !hasGitHubAction {
		t.Fatalf("source card links = %#v, want English GitHub action", gitHubGot.CardLinks)
	}
	if gitHubGot.FetchCount != 0 || gitHubGot.ServerMode || !gitHubGot.Mode {
		t.Fatalf(
			"GitHub static mode = mode %t server %t fetches %d",
			gitHubGot.Mode,
			gitHubGot.ServerMode,
			gitHubGot.FetchCount,
		)
	}
	if !strings.Contains(gitHubGot.DirtyToast, "GitHub") ||
		!strings.Contains(gitHubGot.Hint.Text, "GitHub") {
		t.Fatalf("GitHub localized state = toast %q hint %q", gitHubGot.DirtyToast, gitHubGot.Hint.Text)
	}
}
