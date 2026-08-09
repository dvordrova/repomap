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

func TestUserWorkspaceKeepsStudyPublicationFailuresOutOfHTML(t *testing.T) {
	t.Parallel()

	for name, markers := range map[string][]string{
		"script.js": {
			"renderStudyPublicationNotice(root);",
			"function renderStudyPublicationNotice(root)",
			"rm-study-publication-notice",
			"main.study.unavailable.for.this.run",
			"main.no.study.directions.were.published.because.the.editing.stage.did.not.pass.its.required.checks.the.overview.below.uses.independently.accepted.inputs.it.is.not.a.substitute.study.result",
		},
		"style.css":      {"rm-study-publication-notice"},
		"ui_messages.js": {"main.study.unavailable.for.this.run"},
	} {
		raw, err := os.ReadFile(filepath.Join("templates", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if strings.Contains(string(raw), marker) {
				t.Fatalf("ordinary report asset %s retains Study failure banner marker %q", name, marker)
			}
		}
	}
}

func TestUserWorkspaceReducerCanonicalizesOverviewToMap(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const report = {
  user_sources: [], openable_paths: [], source_ids: {},
  study_map: { brief: {}, shape: [], directions: [
    { id: "study-a", question: "Study A", reading_anchors: [] },
  ] },
  operations: { version: 1, paths: [
    { id: "operate-a", title: "Operate A", actions: [] },
  ], landmarks: [] },
};
const window = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const initial = { view: "study", directionID: "study-a" };
const overview = api.reduceWorkspaceState(initial, { type: "view", view: "overview" });
const architecture = api.reduceWorkspaceState(initial, { type: "view", view: "architecture" });
const operate = api.reduceWorkspaceState(initial, { type: "open_operation", operationID: "operate-a" });
process.stdout.write(JSON.stringify({ overview, architecture, operate }));
`
	runnerPath := filepath.Join(t.TempDir(), "user-workspace-reducer-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run user workspace reducer: %v\n%s", err, output)
	}
	var got struct {
		Overview struct {
			View string `json:"view"`
		} `json:"overview"`
		Architecture struct {
			View string `json:"view"`
		} `json:"architecture"`
		Operate struct {
			View        string `json:"view"`
			OperationID string `json:"operationID"`
			DirectionID string `json:"directionID"`
		} `json:"operate"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode reducer result: %v\n%s", err, output)
	}
	if got.Overview.View != "map" || got.Architecture.View != "map" {
		t.Fatalf("legacy view aliases = overview %q architecture %q, want map", got.Overview.View, got.Architecture.View)
	}
	if got.Operate.View != "operate" || got.Operate.OperationID != "operate-a" || got.Operate.DirectionID != "" {
		t.Fatalf("Study to Operate reducer state = %#v", got.Operate)
	}
}

func TestUserWorkspaceStudyMapRoutesAndOverviewRemainEditorial(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 2,
    highlight_ranges: [{ start_line: line + 1, end_line: line + 1 }],
    content_sha256: "a".repeat(64), presentation_sha256: "b".repeat(64),
    lines: [
      { line, text: "func " + symbol + "() {" },
      { line: line + 1, text: "  inspect()", highlight: true },
      { line: line + 2, text: "}" },
    ],
  };
}
const reading = {
  id: "study-routing",
  question: "How should I study request routing?",
  why_it_matters: "Routing is a central repository responsibility.",
  learning_outcome: "You can name the public and core routing code.",
  principal_anchors: [
    { path: "router.go", symbol: "Route", line: 11 },
    { path: "tree.go", symbol: "Find", line: 21 },
    { path: "context.go", symbol: "Reset", line: 31 },
  ],
  reading_anchors: [
    { label: "Start here", symbol: "Route", what_to_look_for: "Inspect the public boundary.", location: { path: "router.go", line: 11 }, source: snippet("router.go", "Route", 10) },
    { label: "Then inspect", symbol: "Find", what_to_look_for: "Inspect the core data lookup.", location: { path: "tree.go", line: 21 }, source: snippet("tree.go", "Find", 20) },
    { label: "Related implementation", symbol: "Reset", what_to_look_for: "Inspect request-local state.", location: { path: "context.go", line: 31 }, source: snippet("context.go", "Reset", 30) },
  ],
  areas: [{ id: "area-router", name: "Router", responsibility: "Matches requests.", code_location: { path: "router.go", line: 11 } }],
};
const attached = {
  id: "study-dispatch", question: "How does dispatch reach a handler?",
  why_it_matters: "Dispatch connects the public API to a handler.",
  learning_outcome: "You can follow the accepted code path.",
  mechanism_id: "mechanism-dispatch", principal_anchors: reading.principal_anchors,
  reading_anchors: reading.reading_anchors,
};
const incomplete = {
  id: "study-incomplete",
  question: "Where should I begin examining request admission?",
  why_it_matters: "Admission decides which requests enter the core.",
  learning_outcome: "You can locate the exact saved admission declaration.",
  principal_anchors: [reading.principal_anchors[0]],
  reading_anchors: [reading.reading_anchors[0]],
};
const rejected = {
  id: "study-rejected",
  question: "How do integration test utilities launch clusters?",
  why_it_matters: "This proposal was not selected by the canonical reducer.",
  learning_outcome: "This raw proposal must not be published.",
  principal_anchors: [reading.principal_anchors[0]],
  reading_anchors: [reading.reading_anchors[0]],
};
const selected = [reading, attached].concat([
  "How are Raft proposals applied to replicated state?",
  "How does etcd grant, renew, and revoke leases?",
  "How does etcd persist and recover durable state?",
  "How do etcdctl and clientv3 send requests to an etcd server?",
].map((question, index) => Object.assign({}, reading, {
  id: "study-selected-" + String(index + 3),
  question,
})));
const mechanism = { artifact_id: "mechanism-dispatch", steps: [{ title: "Dispatch" }] };
const topics = [
  {
    candidate_id: "topic-message",
    title: "Message creation and real-time delivery",
    question: "How does a chat message get created and delivered to recipients in real time?",
    starting_symbols: [{ path: "router.go", symbol: "Route", line: 11 }],
    uncertainty: "The local evidence stops before the observable result.",
  },
  {
    candidate_id: "topic-upload",
    title: "Asset upload and storage",
    question: "How does the server persist an uploaded asset?",
    starting_symbols: [{ path: "tree.go", symbol: "Find", line: 21 }],
    uncertainty: "The local evidence stops before the observable result.",
  },
  {
    candidate_id: "topic-startup",
    title: "Server initialization and bootstrap",
    question: "How does the server initialize its core components?",
    starting_symbols: [{ path: "context.go", symbol: "Reset", line: 31 }],
    uncertainty: "The local evidence stops before the observable result.",
  },
];
const report = {
  repo_name: "fixture", user_mechanisms: [mechanism], user_topics: topics, user_sources: [],
  openable_paths: ["README.md", "router.go", "tree.go", "context.go"], source_ids: {},
  study_map: {
    brief: {
      what_it_is: "Fixture is an HTTP routing library.",
      problem: "It connects requests to handlers.",
      main_input: "An HTTP request.",
      central_responsibility: "Match a route.",
      observable_result: "Invoke a handler.",
    },
    shape: reading.areas,
    directions: selected,
  },
  incomplete_study: { version: 1, directions: [incomplete, rejected] },
};
const roots = {
  "rm-overview": new Element("section"),
  "rm-study-overview": new Element("section"),
  "rm-study-detail": new Element("section"),
};
const window = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function text(node) { return String(node.textContent || "") + (node.children || []).map(text).join(""); }
const route = api.parseWorkspaceHash("#/study/study-routing", [mechanism], null);
const incompleteOverviewRoute = api.parseWorkspaceHash("#/study", [mechanism], null);
const incompleteRoute = api.parseWorkspaceHash("#/study/study-incomplete", [mechanism], null);
const attachedRoute = api.parseWorkspaceHash("#/study/study-dispatch", [mechanism], null);
const invalidRoute = api.parseWorkspaceHash("#/study/missing", [mechanism], null);
let state = api.reduceWorkspaceState({
  view: "overview", artifactID: "", directionID: "", stepIndex: 0,
  sourceLocation: null, mapReturn: null, mapTarget: null,
}, { type: "open_study", directionID: "study-routing" }, [mechanism]);
state = api.reduceWorkspaceState(state, {
  type: "open_source", selection: { path: "router.go", line: 11, snippet: reading.reading_anchors[0].source },
}, [mechanism]);
const sourceState = state;
state = api.reduceWorkspaceState(state, { type: "close_source" }, [mechanism]);
const closedState = state;
const returned = api.reduceWorkspaceState({
  view: "architecture", artifactID: "", directionID: "study-routing", stepIndex: 0,
  sourceLocation: null, mapReturn: { directionID: "study-routing" }, mapTarget: { kind: "component", component_id: "router" },
}, { type: "return_from_map" }, [mechanism]);
api.renderMapSummaryInto("rm-overview");
const shelfOverviewText = text(roots["rm-overview"]);
api.renderIncompleteStudyOverview();
const incompleteOverviewText = text(roots["rm-study-overview"]);
api.openStudyDirection("study-routing");
const completeDetailText = text(roots["rm-study-detail"]);
api.openStudyDirection("study-incomplete");
const incompleteDetailText = text(roots["rm-study-detail"]);
const canonicalStudyMap = report.study_map;
delete report.incomplete_study;
delete report.study_map;
report.user_mechanisms.length = 0;
const topicRoots = { "rm-overview": new Element("section") };
const topicWindow = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const topicDocument = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return topicRoots[id] || null;
  },
  querySelectorAll() { return []; },
};
topicDocument.documentElement = { lang: "en" };
topicWindow.document = topicDocument;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window: topicWindow });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window: topicWindow, document: topicDocument, URLSearchParams, Set, Map, AbortController,
});
topicWindow.__REPOMAP_WORKSPACE_TEST__.renderMapSummaryInto("rm-overview");
report.user_topics.length = 0;
report.study_map = canonicalStudyMap;
const emptyRoots = { "rm-overview": new Element("section") };
const emptyWindow = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const emptyDocument = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return emptyRoots[id] || null;
  },
  querySelectorAll() { return []; },
};
emptyDocument.documentElement = { lang: "en" };
emptyWindow.document = emptyDocument;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window: emptyWindow });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window: emptyWindow, document: emptyDocument, URLSearchParams, Set, Map, AbortController,
});
const emptyAPI = emptyWindow.__REPOMAP_WORKSPACE_TEST__;
emptyAPI.renderMapSummaryInto("rm-overview");
const completeOverviewRoute = emptyAPI.parseWorkspaceHash("#/study", [], null);
const card = api.renderStudyDirectionCard(reading, 0);
process.stdout.write(JSON.stringify({
  route, incompleteOverviewRoute, completeOverviewRoute, incompleteRoute, attachedRoute, invalidRoute,
  sourceState, closedState, returned,
  shelfOverviewText, topicOverviewText: text(topicRoots["rm-overview"]),
  emptyShelfOverviewText: text(emptyRoots["rm-overview"]), cardText: text(card),
  incompleteOverviewText, completeDetailText, incompleteDetailText,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "study-map-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Study Map workspace smoke: %v\n%s", err, output)
	}
	var got struct {
		Route struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View        string `json:"view"`
				DirectionID string `json:"directionID"`
			} `json:"state"`
		} `json:"route"`
		AttachedRoute struct {
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View        string `json:"view"`
				DirectionID string `json:"directionID"`
			} `json:"state"`
		} `json:"attachedRoute"`
		IncompleteOverviewRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View string `json:"view"`
			} `json:"state"`
		} `json:"incompleteOverviewRoute"`
		CompleteOverviewRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View string `json:"view"`
			} `json:"state"`
		} `json:"completeOverviewRoute"`
		IncompleteRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View        string `json:"view"`
				DirectionID string `json:"directionID"`
			} `json:"state"`
		} `json:"incompleteRoute"`
		InvalidRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
		} `json:"invalidRoute"`
		SourceState struct {
			View           string `json:"view"`
			DirectionID    string `json:"directionID"`
			SourceLocation any    `json:"sourceLocation"`
		} `json:"sourceState"`
		ClosedState struct {
			View           string `json:"view"`
			DirectionID    string `json:"directionID"`
			SourceLocation any    `json:"sourceLocation"`
		} `json:"closedState"`
		Returned struct {
			View        string `json:"view"`
			DirectionID string `json:"directionID"`
		} `json:"returned"`
		ShelfOverviewText      string `json:"shelfOverviewText"`
		TopicOverviewText      string `json:"topicOverviewText"`
		EmptyShelfOverviewText string `json:"emptyShelfOverviewText"`
		CardText               string `json:"cardText"`
		IncompleteOverviewText string `json:"incompleteOverviewText"`
		CompleteDetailText     string `json:"completeDetailText"`
		IncompleteDetailText   string `json:"incompleteDetailText"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Study Map workspace smoke: %v\n%s", err, output)
	}
	if !got.Route.Valid || got.Route.CanonicalHash != "#/study/study-routing" ||
		got.Route.State.View != "study" || got.Route.State.DirectionID != "study-routing" {
		t.Fatalf("reading route = %#v", got.Route)
	}
	if got.AttachedRoute.CanonicalHash != "#/study/study-dispatch" ||
		got.AttachedRoute.State.View != "study" || got.AttachedRoute.State.DirectionID != "study-dispatch" {
		t.Fatalf("attached route = %#v", got.AttachedRoute)
	}
	if !got.IncompleteOverviewRoute.Valid ||
		got.IncompleteOverviewRoute.CanonicalHash != "#/study" ||
		got.IncompleteOverviewRoute.State.View != "study_overview" {
		t.Fatalf("Study overview route = %#v", got.IncompleteOverviewRoute)
	}
	if !got.CompleteOverviewRoute.Valid ||
		got.CompleteOverviewRoute.CanonicalHash != "#/study" ||
		got.CompleteOverviewRoute.State.View != "study_overview" {
		t.Fatalf("complete-only Study overview route = %#v", got.CompleteOverviewRoute)
	}
	if !got.IncompleteRoute.Valid ||
		got.IncompleteRoute.CanonicalHash != "#/study/study-incomplete" ||
		got.IncompleteRoute.State.View != "study" ||
		got.IncompleteRoute.State.DirectionID != "study-incomplete" {
		t.Fatalf("incomplete Study detail route = %#v", got.IncompleteRoute)
	}
	if got.InvalidRoute.Valid || got.InvalidRoute.CanonicalHash != "#/map" {
		t.Fatalf("invalid route = %#v", got.InvalidRoute)
	}
	if got.SourceState.View != "study" || got.SourceState.DirectionID != "study-routing" || got.SourceState.SourceLocation == nil ||
		got.ClosedState.View != "study" || got.ClosedState.DirectionID != "study-routing" || got.ClosedState.SourceLocation != nil {
		t.Fatalf("source drawer changed reading context: source=%#v closed=%#v", got.SourceState, got.ClosedState)
	}
	if got.Returned.View != "study" || got.Returned.DirectionID != "study-routing" {
		t.Fatalf("map return = %#v", got.Returned)
	}
	if !strings.Contains(got.CompleteDetailText, "← All Study directions") ||
		strings.Contains(got.CompleteDetailText, "← Repository overview") {
		t.Fatalf("complete Study detail has inconsistent return action: %q", got.CompleteDetailText)
	}
	for _, token := range []string{
		"Repository brief",
		"A useful path through the repository",
		"How should I study request routing?",
		"How does dispatch reach a handler?",
	} {
		if !strings.Contains(got.ShelfOverviewText, token) {
			t.Errorf("canonical Study Overview is missing %q: %q", token, got.ShelfOverviewText)
		}
	}
	for _, forbidden := range []string{
		"Pick a path worth following.",
		"Message creation and real-time delivery",
		"Where should I begin examining request admission?",
	} {
		if strings.Contains(got.ShelfOverviewText, forbidden) {
			t.Fatalf("canonical Study Overview exposed fallback %q: %q", forbidden, got.ShelfOverviewText)
		}
	}
	for _, forbidden := range []string{
		"Questions worth exploring",
		"Message creation and real-time delivery",
		"Asset upload and storage",
		"Server initialization and bootstrap",
		"Repository brief",
		"Search",
	} {
		if strings.Contains(got.TopicOverviewText, forbidden) {
			t.Fatalf(
				"Map summary exposed removed legacy topic authority %q: %q",
				forbidden,
				got.TopicOverviewText,
			)
		}
	}
	for _, token := range []string{
		"Repository brief",
		"What to study",
		"Fixture is an HTTP routing library.",
		"How should I study request routing?",
	} {
		if !strings.Contains(got.EmptyShelfOverviewText, token) {
			t.Errorf("empty-shelf Study Map fallback is missing %q: %q", token, got.EmptyShelfOverviewText)
		}
	}
	for _, token := range []string{"Route", "Find", "Reset"} {
		if !strings.Contains(got.CardText, token) {
			t.Fatalf("Study Direction card is missing symbol action %q: %q", token, got.CardText)
		}
	}
	if strings.Contains(got.CardText, "Explore this direction") || strings.Contains(got.CardText, "runtime") {
		t.Fatalf("Study Direction card = %q", got.CardText)
	}
	for _, token := range []string{
		"A useful path through the repository",
		"How should I study request routing?",
		"How does dispatch reach a handler?",
		"How are Raft proposals applied to replicated state?",
		"How does etcd grant, renew, and revoke leases?",
		"How does etcd persist and recover durable state?",
		"How do etcdctl and clientv3 send requests to an etcd server?",
	} {
		if !strings.Contains(got.IncompleteOverviewText, token) {
			t.Errorf("canonical Study overview is missing %q: %q", token, got.IncompleteOverviewText)
		}
	}
	for _, rendered := range []string{got.IncompleteOverviewText, got.EmptyShelfOverviewText, got.ShelfOverviewText} {
		if strings.Contains(rendered, "Explore this direction") || strings.Contains(rendered, "Open ready deep dive") {
			t.Fatalf("canonical Study retained a generic route CTA: %q", rendered)
		}
	}
	for _, forbidden := range []string{
		"Where should I begin examining request admission?",
		"How do integration test utilities launch clusters?",
		"Message creation and real-time delivery",
		"Asset upload and storage",
		"Server initialization and bootstrap",
	} {
		if strings.Contains(got.IncompleteOverviewText+got.EmptyShelfOverviewText, forbidden) {
			t.Fatalf("canonical Study publication exposed rejected or ineligible %q: %q / %q", forbidden, got.IncompleteOverviewText, got.EmptyShelfOverviewText)
		}
	}
	for _, token := range []string{
		"Incomplete Study direction",
		"Where should I begin examining request admission?",
		"You can locate the exact saved admission declaration.",
		"Route",
		"router.go:11",
	} {
		if !strings.Contains(got.IncompleteDetailText, token) {
			t.Errorf("incomplete Study detail is missing %q: %q", token, got.IncompleteDetailText)
		}
	}
	if strings.Contains(got.IncompleteOverviewText+got.IncompleteDetailText, "Search") {
		t.Fatalf("incomplete Study reintroduced Search: %q / %q", got.IncompleteOverviewText, got.IncompleteDetailText)
	}
}

func TestUserWorkspaceOverviewAnatomyUsesOnlyExactUnambiguousSavedSource(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.id = "";
    this.classList = {
      add: (name) => { if (!this.className.split(/\s+/).includes(name)) this.className = (this.className + " " + name).trim(); },
      remove: (name) => { this.className = this.className.split(/\s+/).filter((value) => value && value !== name).join(" "); },
      toggle: (name, force) => { if (force) this.classList.add(name); else this.classList.remove(name); },
    };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] || ""; }
  removeAttribute(name) { delete this.attributes[name]; }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  prepend(child) { this.children.unshift(child); }
  replaceChildren(...children) { this.children = children; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  addEventListener() {}
  contains(node) { return this === node || (this.children || []).includes(node); }
  focus() { this.focused = true; }
  remove() {}
}
function snippet(path, line, sha) {
  return {
    path, start_line: line, end_line: line + 2, role: "core",
    highlight_ranges: [{ start_line: line, end_line: line }],
    content_sha256: sha.repeat(64), presentation_sha256: sha.repeat(64),
    lines: [
      { line, text: "func saved() {}", highlight: true },
      { line: line + 1, text: "" },
      { line: line + 2, text: "// saved" },
    ],
  };
}
function member(path, line) {
  return {
    id: { kind: "symbol", value: "member-" + path + "-" + line },
    name: path,
    facts: [{ kind: "declaration", value: path, location: { path, line } }],
  };
}
function component(id, name, path, line) {
  return { id, name, description: "Saved component", members: [member(path, line)] };
}
function surface(id, path, line, overrides) {
  return Object.assign({
    id, kind: "process_entry", surface_role: "entry_surface",
    application_classification: "application_surface", executable_role: "primary_application",
    availability: "available", provisional_id: false,
    identity: { name: "Same visible entry" }, registration_site: { path, line },
    process_entrypoint: { name: "main", location: { path, line } },
  }, overrides || {});
}
const sourceA = snippet("surface-a.go", 10, "a");
const sourceAWide = snippet("surface-a.go", 9, "7");
sourceAWide.end_line = 14;
const sourceB = snippet("surface-b.go", 20, "b");
const sourceBView = JSON.parse(JSON.stringify(sourceB));
sourceBView.presentation_sha256 = "8".repeat(64);
const componentA = snippet("component-a.go", 30, "c");
const componentASecond = snippet("component-a2.go", 35, "6");
const componentB = snippet("component-b.go", 40, "d");
const ambiguousOne = snippet("ambiguous.go", 50, "e");
const ambiguousTwo = snippet("ambiguous.go", 50, "f");
const report = {
  repo_name: "fixture", project_guess: "Saved repository orientation.",
  user_mechanisms: [], user_sources: [
    sourceA, sourceAWide, sourceB, JSON.parse(JSON.stringify(sourceB)), sourceBView,
    componentA, componentASecond, componentB, ambiguousOne, ambiguousTwo,
  ],
  openable_paths: [
    "surface-a.go", "surface-b.go", "component-a.go", "component-a2.go", "component-b.go",
    "ambiguous.go", "dead.go", "duplicate.go", "study.go", "./surface-a.go",
  ],
  source_ids: {},
  github_source_links: {
    repository_url: "https://github.com/example/fixture",
    revision: "1".repeat(40), working_tree_paths: [],
  },
  architecture_canvas: {
    version: 5, subsystems: [], behavior_anchors: [], surfaces: [], flows: [],
    components: [
      Object.assign(component("component-a", "Same visible component", "component-a.go", 30), {
        members: [member("component-a.go", 30), member("component-a2.go", 35)],
      }),
      component("component-b", "Same visible component", "component-b.go", 40),
      component("component-ambiguous", "Ambiguous", "ambiguous.go", 50),
      component("component-dead", "Dead", "dead.go", 60),
      component("component-duplicate", "Duplicate one", "duplicate.go", 70),
      component("component-duplicate", "Duplicate two", "duplicate.go", 70),
    ],
  },
  architecture_component_navigation: {
    version: 1,
    components: [
      {
        component_id: "component-a",
        map_target: { kind: "component", component_id: "component-a" },
        symbol_sources: [
          { member_id: { kind: "symbol", value: "component-a" }, symbol: "ComponentA", location: { path: "component-a.go", line: 30 } },
          { member_id: { kind: "symbol", value: "component-a-2" }, symbol: "ComponentASecond", location: { path: "component-a2.go", line: 35 } },
        ],
      },
      {
        component_id: "component-b",
        map_target: { kind: "component", component_id: "component-b" },
        symbol_sources: [
          { member_id: { kind: "symbol", value: "component-b" }, symbol: "ComponentB", location: { path: "component-b.go", line: 40 } },
        ],
      },
      {
        component_id: "component-ambiguous",
        map_target: { kind: "component", component_id: "component-ambiguous" },
        symbol_sources: [
          { member_id: { kind: "symbol", value: "component-ambiguous" }, symbol: "Ambiguous", location: { path: "ambiguous.go", line: 50 } },
        ],
      },
      { component_id: "component-dead", map_target: { kind: "component", component_id: "component-dead" } },
    ],
  },
  discovered_surfaces: { triggers: [
    surface("surface-a", "surface-a.go", 10),
    surface("surface-b", "surface-b.go", 20),
    surface("surface-ambiguous", "ambiguous.go", 50),
    surface("surface-dead", "dead.go", 60),
    surface("surface-duplicate", "duplicate.go", 70),
    surface("surface-duplicate", "duplicate.go", 70),
    surface("surface-dynamic", "surface-a.go", 10, { surface_role: "dynamic_frontier" }),
    surface("surface-test", "surface-a.go", 10, { executable_role: "test_or_helper" }),
    surface("surface-unavailable", "surface-a.go", 10, { availability: "unavailable" }),
    surface("surface-unknown", "surface-a.go", 10, { availability: "unknown" }),
    surface("surface-provisional", "surface-a.go", 10, { provisional_id: true }),
  ] },
  study_map: { directions: [{
    id: "study-exact", question: "Where should I read next?",
    why_it_matters: "This is a saved reading route.", learning_outcome: "You can inspect it.",
    principal_anchors: [{ path: "study.go", symbol: "Study", line: 80 }],
    reading_anchors: [{
      label: "Start here", what_to_look_for: "Read the saved declaration.",
      symbol: "Study",
      location: { path: "study.go", line: 80 }, source: snippet("study.go", 80, "9"),
    }],
  }] },
};
const roots = {
  "rm-overview": new Element("section"),
  "rm-study-detail": new Element("section"),
  "rm-study-overview": new Element("section"),
  "rm-architecture": new Element("section"),
  "rm-source-drawer": new Element("aside"),
  "rm-source-drawer-content": new Element("div"),
  "rm-source-drawer-close": new Element("button"),
};
const workspace = new Element("main");
let openedURLs = 0;
let openedFirstURL = "";
const window = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  history: {
    state: null,
    pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
    replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
    back() {},
  },
  open(url) { openedURLs += 1; if (!openedFirstURL) openedFirstURL = String(url || ""); },
  scrollTo() {},
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function walk(root) {
  const result = [];
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(root) { return walk(root).map((node) => String(node.textContent || "")).join(""); }
function cards(kind) {
  return walk(roots["rm-overview"]).filter((node) => node.attributes && node.attributes["data-rm-object-kind"] === kind);
}
const anatomy = api.repositoryOverviewAnatomy();
api.renderMapSummaryInto("rm-overview");
const surfaceCards = cards("surface");
const componentCards = cards("component");
const renderedText = text(roots["rm-overview"]);
surfaceCards[0].children[0].onclick();
const drawerState = api.workspaceStateSnapshot();
const drawerHash = window.location.hash;
const drawerHistory = window.history.state && window.history.state.sourceDrawer;
const drawerHidden = roots["rm-source-drawer"].hidden;
const studyCard = walk(roots["rm-overview"]).find((node) => String(node.className).split(/\s+/).includes("rm-study-direction-card"));
walk(studyCard).find((node) => String(node.className).split(/\s+/).includes("rm-study-direction-card__title")).onclick();
const studyState = api.workspaceStateSnapshot();
const studyHash = window.location.hash;
const firstComponentPrimary = componentCards[0] && walk(componentCards[0]).find((node) => String(node.className).split(/\s+/).includes("rm-overview-object-primary"));
firstComponentPrimary.onclick();
const architectureState = api.workspaceStateSnapshot();
const architectureHash = window.location.hash;
const exactStart = api.exactOverviewSourceForLocation({ path: "surface-a.go", line: 10 });
const exactEnd = api.exactOverviewSourceForLocation({ path: "surface-a.go", line: 12 });
function renderIsolatedOverview(isolatedReport, isolatedLocation) {
  const isolatedRoot = new Element("section");
  const isolatedWindow = {
    location: isolatedLocation || { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
  };
  const isolatedDocument = {
    createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
    createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
    getElementById(id) {
      if (id === "rm-report-data") return { textContent: JSON.stringify(isolatedReport) };
      if (id === "rm-overview") return isolatedRoot;
      return null;
    },
    querySelector() { return null; }, querySelectorAll() { return []; },
  };
  isolatedDocument.documentElement = { lang: "en" };
  isolatedWindow.document = isolatedDocument;
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window: isolatedWindow });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
    window: isolatedWindow, document: isolatedDocument, URLSearchParams, Set, Map, AbortController, Promise,
  });
  isolatedWindow.__REPOMAP_WORKSPACE_TEST__.renderMapSummaryInto("rm-overview");
  const isolatedNodes = walk(isolatedRoot);
  return {
    sections: isolatedRoot.children.map((node) => String(node.className || "")),
    studyDirectionCount: isolatedNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-study-direction-card")).length,
    surfaceCount: isolatedNodes.filter((node) => node.attributes && node.attributes["data-rm-object-kind"] === "surface").length,
    componentCount: isolatedNodes.filter((node) => node.attributes && node.attributes["data-rm-object-kind"] === "component").length,
    primaryTargets: isolatedNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-overview-object-primary")).map((node) => ({
      tag: node.tagName,
      href: node.attributes && node.attributes.href || "",
      hasClick: typeof node.onclick === "function",
    })),
    sourceTargets: isolatedNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-overview-object-source")).map((node) => ({
      tag: node.tagName,
      href: node.attributes && node.attributes.href || "",
      hasClick: typeof node.onclick === "function",
    })),
    rendered: text(isolatedRoot),
  };
}
const atlasFirstReport = JSON.parse(JSON.stringify(report));
atlasFirstReport.repository_atlas = {
  version: 1,
  units: [{ id: "atlas-repository", kind: "repository", name: "fixture" }],
  entities: [], evidence: [], relations: [],
};
const atlasFirstOverview = renderIsolatedOverview(atlasFirstReport);
const atlasFirstStudyOnlyReport = JSON.parse(JSON.stringify(atlasFirstReport));
delete atlasFirstStudyOnlyReport.discovered_surfaces;
const atlasFirstStudyOnlyOverview = renderIsolatedOverview(atlasFirstStudyOnlyReport);
const strippedStaticReport = JSON.parse(JSON.stringify(report));
function stripSourceLines(value) {
  if (!value || typeof value !== "object") return;
  if (Object.prototype.hasOwnProperty.call(value, "lines")) delete value.lines;
  Object.keys(value).forEach((key) => stripSourceLines(value[key]));
}
stripSourceLines(strippedStaticReport);
const strippedStaticOverview = renderIsolatedOverview(strippedStaticReport);
const mixedServedReport = JSON.parse(JSON.stringify(report));
delete mixedServedReport.github_source_links;
mixedServedReport.source_ids = {};
mixedServedReport.openable_paths.forEach((path, index) => {
  mixedServedReport.source_ids[path] = "source-" + index;
});
const mixedServedOverview = renderIsolatedOverview(mixedServedReport, {
  search: "", hash: "#/map", hostname: "127.0.0.1", protocol: "http:",
  pathname: "/_repomap/token/runs/run/report.html",
});
const fallbackReport = JSON.parse(JSON.stringify(report));
fallbackReport.user_sources = [componentA];
const fallbackRoot = new Element("section");
const fallbackWindow = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const fallbackDocument = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(fallbackReport) };
    if (id === "rm-overview") return fallbackRoot;
    return null;
  },
  querySelector() { return null; }, querySelectorAll() { return []; },
};
fallbackDocument.documentElement = { lang: "en" };
fallbackWindow.document = fallbackDocument;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window: fallbackWindow });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window: fallbackWindow, document: fallbackDocument, URLSearchParams, Set, Map, AbortController, Promise,
});
const fallbackAPI = fallbackWindow.__REPOMAP_WORKSPACE_TEST__;
const fallbackAnatomy = fallbackAPI.repositoryOverviewAnatomy();
fallbackAPI.renderMapSummaryInto("rm-overview");
const noStudyReport = JSON.parse(JSON.stringify(report));
delete noStudyReport.study_map;
delete noStudyReport.incomplete_study;
noStudyReport.study_publication = {
  version: 1, state: "failed",
  failure_reason: "study map: shape references unknown area \"doc-b335630551682c19\"",
};
const noStudyRoot = new Element("section");
const noStudyWindow = {
  location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const noStudyDocument = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(noStudyReport) };
    if (id === "rm-overview") return noStudyRoot;
    return null;
  },
  querySelector() { return null; }, querySelectorAll() { return []; },
};
noStudyDocument.documentElement = { lang: "en" };
noStudyWindow.document = noStudyDocument;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window: noStudyWindow });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window: noStudyWindow, document: noStudyDocument, URLSearchParams, Set, Map, AbortController, Promise,
});
const noStudyAPI = noStudyWindow.__REPOMAP_WORKSPACE_TEST__;
const noStudyAnatomy = noStudyAPI.repositoryOverviewAnatomy();
noStudyAPI.renderMapSummaryInto("rm-overview");
const savedSurfaceKinds = [
  "async_task", "cli_command", "http_route", "http_route_descriptor",
  "http_route_frontier", "http_server", "process_entry", "worker", "future_kind",
];
const enSurfaceKindLabels = savedSurfaceKinds.map((kind) => api.overviewSurfaceKindLabel(kind));
document.documentElement.lang = "ru";
const ruSurfaceKindLabels = savedSurfaceKinds.map((kind) => api.overviewSurfaceKindLabel(kind));
document.documentElement.lang = "en";
process.stdout.write(JSON.stringify({
  entries: anatomy.entries, components: anatomy.components,
  componentASourcePaths: anatomy.components.objects.find((object) => object.id === "component-a").sources.map((source) => source.location.path),
  surfaceIDs: surfaceCards.map((card) => card.attributes["data-rm-object-id"]),
  componentIDs: componentCards.map((card) => card.attributes["data-rm-object-id"]),
  cardText: surfaceCards.concat(componentCards).map(text), renderedText,
  drawerState, drawerHash, drawerHistory, drawerHidden, openedURLs, openedFirstURL,
  studyState, studyHash, architectureState, architectureHash,
  exactStart: exactStart && exactStart.snippet.presentation_sha256,
  exactEnd: exactEnd && exactEnd.snippet.presentation_sha256,
  ambiguous: api.exactOverviewSourceForLocation({ path: "ambiguous.go", line: 50 }),
  basename: api.exactOverviewSourceForLocation({ path: "a.go", line: 10 }),
  prefix: api.exactOverviewSourceForLocation({ path: "./surface-a.go", line: 10 }),
  stringLine: api.exactOverviewSourceForLocation({ path: "surface-a.go", line: "10" }),
  atlasFirstOverview,
  atlasFirstStudyOnlyOverview,
  strippedStaticOverview,
  mixedServedOverview,
  fallbackAnatomy, fallbackText: text(fallbackRoot),
  noStudyAnatomy, noStudyText: text(noStudyRoot),
  noStudyDirectionCards: walk(noStudyRoot).filter((node) => String(node.className).split(/\s+/).includes("rm-study-direction-card")).length,
  enSurfaceKindLabels, ruSurfaceKindLabels,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "overview-anatomy-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Overview anatomy workspace smoke: %v\n%s", err, output)
	}
	var got struct {
		Entries struct {
			Total   int `json:"total"`
			Omitted int `json:"omitted"`
		} `json:"entries"`
		Components struct {
			Total   int `json:"total"`
			Omitted int `json:"omitted"`
		} `json:"components"`
		SurfaceIDs            []string `json:"surfaceIDs"`
		ComponentIDs          []string `json:"componentIDs"`
		ComponentASourcePaths []string `json:"componentASourcePaths"`
		CardText              []string `json:"cardText"`
		RenderedText          string   `json:"renderedText"`
		DrawerState           struct {
			View           string `json:"view"`
			SourceLocation *struct {
				Path        string `json:"path"`
				Line        int    `json:"line"`
				DrawerFirst bool   `json:"drawerFirst"`
			} `json:"sourceLocation"`
		} `json:"drawerState"`
		DrawerHash    string `json:"drawerHash"`
		DrawerHistory struct {
			Path        string `json:"path"`
			Line        int    `json:"line"`
			DrawerFirst bool   `json:"drawer_first"`
		} `json:"drawerHistory"`
		DrawerHidden      bool                               `json:"drawerHidden"`
		OpenedURLs        int                                `json:"openedURLs"`
		OpenedFirstURL    string                             `json:"openedFirstURL"`
		StudyState        struct{ View, DirectionID string } `json:"studyState"`
		StudyHash         string                             `json:"studyHash"`
		ArchitectureState struct {
			View      string `json:"view"`
			MapTarget struct {
				Kind        string `json:"kind"`
				ComponentID string `json:"component_id"`
			} `json:"mapTarget"`
		} `json:"architectureState"`
		ArchitectureHash   string `json:"architectureHash"`
		ExactStart         string `json:"exactStart"`
		ExactEnd           string `json:"exactEnd"`
		Ambiguous          any    `json:"ambiguous"`
		Basename           any    `json:"basename"`
		Prefix             any    `json:"prefix"`
		StringLine         any    `json:"stringLine"`
		AtlasFirstOverview struct {
			Sections            []string `json:"sections"`
			StudyDirectionCount int      `json:"studyDirectionCount"`
			SurfaceCount        int      `json:"surfaceCount"`
			ComponentCount      int      `json:"componentCount"`
			PrimaryTargets      []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"primaryTargets"`
			SourceTargets []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"sourceTargets"`
			Rendered string `json:"rendered"`
		} `json:"atlasFirstOverview"`
		AtlasFirstStudyOnlyOverview struct {
			Sections            []string `json:"sections"`
			StudyDirectionCount int      `json:"studyDirectionCount"`
			SurfaceCount        int      `json:"surfaceCount"`
			ComponentCount      int      `json:"componentCount"`
			Rendered            string   `json:"rendered"`
		} `json:"atlasFirstStudyOnlyOverview"`
		StrippedStaticOverview struct {
			SurfaceCount   int `json:"surfaceCount"`
			ComponentCount int `json:"componentCount"`
			PrimaryTargets []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"primaryTargets"`
			SourceTargets []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"sourceTargets"`
			Rendered string `json:"rendered"`
		} `json:"strippedStaticOverview"`
		MixedServedOverview struct {
			SurfaceCount   int `json:"surfaceCount"`
			ComponentCount int `json:"componentCount"`
			PrimaryTargets []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"primaryTargets"`
			SourceTargets []struct {
				Tag      string `json:"tag"`
				Href     string `json:"href"`
				HasClick bool   `json:"hasClick"`
			} `json:"sourceTargets"`
		} `json:"mixedServedOverview"`
		FallbackAnatomy   any      `json:"fallbackAnatomy"`
		FallbackText      string   `json:"fallbackText"`
		NoStudyAnatomy    any      `json:"noStudyAnatomy"`
		NoStudyText       string   `json:"noStudyText"`
		NoStudyDirections int      `json:"noStudyDirectionCards"`
		ENSurfaceKinds    []string `json:"enSurfaceKindLabels"`
		RUSurfaceKinds    []string `json:"ruSurfaceKindLabels"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Overview anatomy workspace smoke: %v\n%s", err, output)
	}
	if strings.Join(got.SurfaceIDs, ",") != "surface-a,surface-b" ||
		strings.Join(got.ComponentIDs, ",") != "component-a,component-b,component-ambiguous,component-dead" {
		t.Fatalf("exact-ID cards = surfaces %#v components %#v", got.SurfaceIDs, got.ComponentIDs)
	}
	if got.Entries.Total != 5 || got.Entries.Omitted != 3 ||
		got.Components.Total != 5 || got.Components.Omitted != 1 {
		t.Fatalf("bounded anatomy counts = entries %#v components %#v", got.Entries, got.Components)
	}
	if strings.Count(strings.Join(got.CardText, "\n"), "Same visible entry") != 2 ||
		strings.Count(strings.Join(got.CardText, "\n"), "Same visible component") != 2 {
		t.Fatalf("same-label exact IDs collapsed: %#v", got.CardText)
	}
	if joined := strings.Join(got.CardText, "\n"); strings.Count(joined, "Process entry") != 2 || strings.Contains(joined, "process_entry") {
		t.Fatalf("Overview Surface cards did not use typed kind labels: %#v", got.CardText)
	}
	for _, forbidden := range []string{"surface-ambiguous", "surface-dead", "surface-duplicate", "surface-dynamic", "surface-test", "surface-unknown", "component-duplicate"} {
		if strings.Contains(strings.Join(got.SurfaceIDs, "\n")+strings.Join(got.ComponentIDs, "\n"), forbidden) {
			t.Fatalf("Overview rendered rejected or dead object %q", forbidden)
		}
	}
	if strings.Join(got.ComponentASourcePaths, ",") != "component-a.go,component-a2.go" {
		t.Fatalf("component plural exact sources = %#v", got.ComponentASourcePaths)
	}
	for _, token := range []string{"Entry surfaces", "Components", "Study directions", "Where should I read next?"} {
		if !strings.Contains(got.RenderedText, token) {
			t.Errorf("Overview anatomy is missing %q: %q", token, got.RenderedText)
		}
	}
	if strings.Contains(got.RenderedText, "No saved exact integration evidence") {
		t.Fatalf("Overview rendered an empty Integrations product section: %q", got.RenderedText)
	}
	for _, card := range got.CardText {
		for _, forbidden := range []string{"→", "execution", "reachability", "connected", "depends on"} {
			if strings.Contains(strings.ToLower(card), strings.ToLower(forbidden)) {
				t.Fatalf("taxonomy card implies a join through %q: %q", forbidden, card)
			}
		}
	}
	anatomyPosition, atlasPosition := -1, -1
	for index, className := range got.AtlasFirstOverview.Sections {
		if anatomyPosition < 0 && strings.Contains(className, "rm-overview-anatomy-zone") {
			anatomyPosition = index
		}
		if atlasPosition < 0 && strings.Contains(className, "rm-atlas-shelf") {
			atlasPosition = index
		}
	}
	// Decision 217: the Atlas unit ontology is demoted below the
	// user-facing orientation — anatomy zones (entry surfaces, components)
	// precede the unit shelf.
	if anatomyPosition < 0 || atlasPosition < 0 || atlasPosition < anatomyPosition ||
		got.AtlasFirstOverview.StudyDirectionCount != 1 || got.AtlasFirstOverview.SurfaceCount != 2 ||
		got.AtlasFirstOverview.ComponentCount != 4 {
		t.Fatalf("Atlas-first Overview order/dedup = %#v", got.AtlasFirstOverview)
	}
	studyPosition, studyOnlyAtlasPosition := -1, -1
	for index, className := range got.AtlasFirstStudyOnlyOverview.Sections {
		if studyPosition < 0 && (strings.Contains(className, "rm-study-map-section") ||
			strings.Contains(className, "rm-overview-study-routes")) {
			studyPosition = index
		}
		if studyOnlyAtlasPosition < 0 && strings.Contains(className, "rm-atlas-shelf") {
			studyOnlyAtlasPosition = index
		}
	}
	// Decision 217: the Atlas unit shelf is demoted below user-facing
	// orientation here as well — study routes precede the unit ontology.
	if studyPosition < 0 || studyOnlyAtlasPosition < 0 || studyOnlyAtlasPosition < studyPosition ||
		got.AtlasFirstStudyOnlyOverview.StudyDirectionCount != 1 ||
		got.AtlasFirstStudyOnlyOverview.SurfaceCount != 0 ||
		got.AtlasFirstStudyOnlyOverview.ComponentCount != 4 ||
		!strings.Contains(got.AtlasFirstStudyOnlyOverview.Rendered, "Where should I read next?") {
		t.Fatalf("Atlas-first Study fallback / Atlas order = %#v", got.AtlasFirstStudyOnlyOverview)
	}
	// Decision 217: component cards show at most one representative exact
	// anchor (plus a bounded "+N more" note), so the stripped-source static
	// anatomy exposes one pinned source action per exact-backed component
	// (component-a, component-b, ambiguous) rather than the full list.
	// Decision 233 F4: the entry grid builds LAZILY — the static anatomy
	// renders the 3 representatives plus the Show-all N toggle (the full
	// 4-card grid appears only on demand).
	if got.StrippedStaticOverview.SurfaceCount != 3 || got.StrippedStaticOverview.ComponentCount != 4 ||
		len(got.StrippedStaticOverview.PrimaryTargets) != 7 ||
		len(got.StrippedStaticOverview.SourceTargets) != 3 ||
		!strings.Contains(got.StrippedStaticOverview.Rendered, "Unclassified entries") ||
		!strings.Contains(got.StrippedStaticOverview.Rendered, "Show all 4") {
		t.Fatalf("stripped-source static anatomy = %#v", got.StrippedStaticOverview)
	}
	for _, target := range got.StrippedStaticOverview.PrimaryTargets {
		if target.Tag == "button" && target.HasClick && target.Href == "" {
			continue
		}
		if target.Tag != "a" || target.HasClick ||
			!strings.HasPrefix(target.Href, "https://github.com/example/fixture/blob/"+strings.Repeat("1", 40)+"/") {
			t.Fatalf("stripped-source anatomy target = %#v", target)
		}
	}
	for _, target := range got.StrippedStaticOverview.SourceTargets {
		if target.Tag != "a" || target.HasClick || !strings.Contains(target.Href, "#L") {
			t.Fatalf("stripped-source component source target = %#v", target)
		}
	}
	// Decision 217: one representative exact source action per component;
	// the ambiguous component carries no exact-unambiguous source.
	if got.MixedServedOverview.SurfaceCount != 2 || got.MixedServedOverview.ComponentCount != 4 ||
		len(got.MixedServedOverview.PrimaryTargets) != 6 || len(got.MixedServedOverview.SourceTargets) != 2 {
		t.Fatalf("mixed served report escaped excerpt-only anatomy = %#v", got.MixedServedOverview)
	}
	for _, target := range append(got.MixedServedOverview.PrimaryTargets, got.MixedServedOverview.SourceTargets...) {
		if target.Tag != "button" || target.Href != "" || !target.HasClick {
			t.Fatalf("mixed served anatomy target = %#v", target)
		}
	}
	if got.DrawerState.View != "map" || got.DrawerState.SourceLocation != nil ||
		got.DrawerHash != "#/map" ||
		got.OpenedURLs != 1 ||
		!strings.Contains(got.OpenedFirstURL, "github.com/example/fixture/blob") {
		t.Fatalf("primary click did not jump to GitHub source: state %#v hash %q history %#v hidden=%t opened=%d first=%q",
			got.DrawerState, got.DrawerHash, got.DrawerHistory, got.DrawerHidden, got.OpenedURLs, got.OpenedFirstURL)
	}
	if got.StudyState.View != "study" || got.StudyState.DirectionID != "study-exact" ||
		got.StudyHash != "#/study/study-exact" {
		t.Fatalf("Study route = state %#v hash %q", got.StudyState, got.StudyHash)
	}
	if got.ArchitectureState.View != "map" || got.ArchitectureState.MapTarget.Kind != "component" ||
		got.ArchitectureState.MapTarget.ComponentID != "component-a" ||
		got.ArchitectureHash != "#/map?focus=component%3Acomponent-a" {
		t.Fatalf("same-ID Architecture focus = state %#v hash %q", got.ArchitectureState, got.ArchitectureHash)
	}
	if got.ExactStart != strings.Repeat("a", 64) || got.ExactEnd != strings.Repeat("a", 64) ||
		got.Ambiguous != nil || got.Basename != nil ||
		got.Prefix != nil || got.StringLine != nil {
		t.Fatalf("exact source resolver accepted repair/conflict or missed most-specific overlap: start=%q end=%q ambiguous=%#v basename=%#v prefix=%#v string=%#v",
			got.ExactStart, got.ExactEnd, got.Ambiguous, got.Basename, got.Prefix, got.StringLine)
	}
	if got.FallbackAnatomy == nil ||
		!strings.Contains(got.FallbackText, "Components") ||
		!strings.Contains(got.FallbackText, "Where should I read next?") ||
		strings.Contains(got.FallbackText, "Entry surfaces") {
		t.Fatalf("source-less component report lost map navigation or Study routes: anatomy=%#v text=%q",
			got.FallbackAnatomy, got.FallbackText)
	}
	if got.NoStudyAnatomy == nil || got.NoStudyDirections != 0 {
		t.Fatalf("failed-Study report lost anatomy or invented Study directions: anatomy=%#v directions=%d text=%q",
			got.NoStudyAnatomy, got.NoStudyDirections, got.NoStudyText)
	}
	wantENSurfaceKinds := []string{
		"Async task", "CLI command", "HTTP route", "HTTP route descriptor",
		"HTTP route frontier", "HTTP server start", "Process entry", "Worker",
		"Other saved surface kind",
	}
	wantRUSurfaceKinds := []string{
		"Асинхронная задача", "CLI-команда", "HTTP-маршрут", "Дескриптор HTTP-маршрута",
		"Граница HTTP-маршрутов", "Запуск HTTP-сервера", "Точка входа процесса", "Воркер",
		"Другой сохранённый тип точки запуска",
	}
	if !slices.Equal(got.ENSurfaceKinds, wantENSurfaceKinds) ||
		!slices.Equal(got.RUSurfaceKinds, wantRUSurfaceKinds) {
		t.Fatalf("Overview Surface labels = EN %#v RU %#v", got.ENSurfaceKinds, got.RUSurfaceKinds)
	}
	for _, token := range []string{"Entry surfaces", "Components"} {
		if !strings.Contains(got.NoStudyText, token) {
			t.Errorf("failed-Study anatomy is missing %q: %q", token, got.NoStudyText)
		}
	}
	for _, forbidden := range []string{
		"Study unavailable for this run",
		"No Study directions were published because the editing stage did not pass its required checks.",
		"Repository brief",
		"Where should I read next?",
	} {
		if strings.Contains(got.NoStudyText, forbidden) {
			t.Fatalf("failed-Study anatomy exposed fallback or invented Study content %q: %q", forbidden, got.NoStudyText)
		}
	}
}

func TestUserWorkspaceOverviewKeepsSourceLessComponentsWithoutSurfaceZone(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, marker := range []string{
		"if (!entries.objects.length && !components.objects.length) return null;",
		"if (anatomy.components.objects.length)",
		"var anatomy = repositoryOverviewAnatomy();",
		"renderStudyMapOverview(root, false);",
		"if (conflict) return { source: null, conflict: true };",
		"sources: resolved,",
		"mapTarget: mapTarget",
		"exactOverviewSourcePath(snippet.path) !== location.path",
	} {
		if !strings.Contains(script, marker) {
			t.Fatalf("Overview fallback/exact-source guard is missing %q", marker)
		}
	}
}

func TestUserWorkspaceSourceRendererShowsCodeAndExactReferences(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
const report = { user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {} };
const window = {
  location: { search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(text) { return { nodeType: 3, textContent: String(text), children: [], attributes: {}, appendChild() {} }; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
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
const code = api.renderSourceCode({
  path: "router.go",
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  if value < 3 {", highlight: true, gap_before: true },
    { line: 42, text: "    dispatch()" },
  ],
});
const codeNodes = walk(code);
const highlighted = codeNodes.find((node) => String(node.className).includes("is-highlighted"));
const exact = api.renderExactReferences([
  { path: "router.go", line: 41, column: 2 },
  { path: "tree.go", line: 88 },
]);
const exactNodes = walk(exact);
const card = api.renderSourceSnippetCard({
  path: "router.go",
  revision: "deadbeef",
  start_line: 40,
  end_line: 42,
  highlight_ranges: [{ start_line: 41, end_line: 41 }],
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
  full_function_start_line: 38,
  full_function_end_line: 44,
  full_function_lines: [
    { line: 38, text: "// route dispatches" },
    { line: 41, text: "  dispatch()" },
    { line: 44, text: "}" },
  ],
}, {
  primary: true,
  notices: [
    { text: "This line dispatches the request.", path: "router.go", supporting_ranges: [{ start_line: 41, end_line: 41 }] },
    { text: "Unsafe unrelated line.", path: "router.go", supporting_ranges: [{ start_line: 99, end_line: 99 }] },
  ],
});
const remaining = api.remainingExactReferences([
  { path: "router.go", line: 41 },
  { path: "router.go", line: 7 },
  { path: "tree.go", line: 88 },
], {
  path: "router.go",
  start_line: 40,
  end_line: 42,
  highlight_ranges: [{ start_line: 41, end_line: 41 }],
  lines: [{ line: 41, text: "dispatch()", highlight: true }],
});
const deduped = api.uniqueSourceSnippets([
  { path: "router.go", enclosing_symbol: "route", lines: [{ line: 1, text: "a" }] },
  { path: "router.go", enclosing_symbol: "route", lines: [{ line: 2, text: "b" }] },
  { path: "tree.go", enclosing_symbol: "FindRoute", lines: [{ line: 3, text: "c" }] },
  { path: "other.go", enclosing_symbol: "fallback", lines: [{ line: 4, text: "d" }] },
], { "router.go\u0000route": true }, 2, false).map(api.sourceSnippetIdentity);
process.stdout.write(JSON.stringify({
  sourceContent: code.attributes["data-source-content"],
  sourcePath: code.attributes["data-source-path"],
  codeText: text(code),
  highlightedLine: highlighted && highlighted.attributes["data-source-line"],
  highlightedText: highlighted && text(highlighted),
  exactClass: exact.className,
  exactSummary: text(exactNodes.find((node) => node.tagName === "summary")),
  exactText: text(exact),
  cardText: text(card),
  remaining,
  identity: api.sourceSnippetIdentity({ path: "router.go", enclosing_symbol: "route" }),
  deduped,
  landmarkRole: api.overviewSourceRoleLabel({ landmark_kind: "public_api", landmark_reason: "Exported API: Serve." }),
  landmarkStrong: api.overviewSourceIsStrong({ landmark_kind: "public_api" }),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-source-renderer-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run source renderer: %v\n%s", err, output)
	}
	var got struct {
		SourceContent   string `json:"sourceContent"`
		SourcePath      string `json:"sourcePath"`
		CodeText        string `json:"codeText"`
		HighlightedLine string `json:"highlightedLine"`
		HighlightedText string `json:"highlightedText"`
		ExactClass      string `json:"exactClass"`
		ExactSummary    string `json:"exactSummary"`
		ExactText       string `json:"exactText"`
		CardText        string `json:"cardText"`
		Remaining       []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"remaining"`
		Identity       string   `json:"identity"`
		Deduped        []string `json:"deduped"`
		LandmarkRole   string   `json:"landmarkRole"`
		LandmarkStrong bool     `json:"landmarkStrong"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode renderer result: %v\n%s", err, output)
	}
	if got.SourceContent != "true" || got.SourcePath != "router.go" {
		t.Fatalf("source renderer markers = content %q, path %q", got.SourceContent, got.SourcePath)
	}
	for _, token := range []string{"func route() {", "if value < 3 {", "dispatch()", "lines omitted"} {
		if !strings.Contains(got.CodeText, token) {
			t.Errorf("rendered code is missing %q: %q", token, got.CodeText)
		}
	}
	if got.HighlightedLine != "41" || !strings.Contains(got.HighlightedText, "if value < 3 {") {
		t.Fatalf("highlighted source row = line %q, text %q", got.HighlightedLine, got.HighlightedText)
	}
	if got.ExactClass != "rm-exact-references" || got.ExactSummary != "Show 2 exact references" {
		t.Fatalf("exact reference disclosure = class %q, summary %q", got.ExactClass, got.ExactSummary)
	}
	for _, token := range []string{"router.go:41:2", "tree.go:88"} {
		if !strings.Contains(got.ExactText, token) {
			t.Errorf("exact references are missing %q: %q", token, got.ExactText)
		}
	}
	for _, token := range []string{"What to notice", "Line 41", "This line dispatches the request.", "Show full function"} {
		if !strings.Contains(got.CardText, token) {
			t.Errorf("source card is missing %q: %q", token, got.CardText)
		}
	}
	if strings.Contains(got.CardText, "Unsafe unrelated line") {
		t.Fatalf("source card rendered a callout outside supporting evidence: %q", got.CardText)
	}
	if strings.Contains(got.CardText, "saved snapshot") {
		t.Fatalf("default source card exposed snapshot metadata: %q", got.CardText)
	}
	if len(got.Remaining) != 2 || got.Remaining[0].Path != "router.go" || got.Remaining[0].Line != 7 ||
		got.Remaining[1].Path != "tree.go" || got.Remaining[1].Line != 88 {
		t.Fatalf("remaining exact references = %#v", got.Remaining)
	}
	if got.Identity != "router.go\x00route" {
		t.Fatalf("source identity = %q", got.Identity)
	}
	if len(got.Deduped) != 2 || got.Deduped[0] != "tree.go\x00FindRoute" || got.Deduped[1] != "other.go\x00fallback" {
		t.Fatalf("deduplicated related sources = %#v", got.Deduped)
	}
	if got.LandmarkRole != "Public API" || !got.LandmarkStrong {
		t.Fatalf("overview landmark presentation = role %q, strong %v", got.LandmarkRole, got.LandmarkStrong)
	}
}

func TestUserWorkspaceHashRoutes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const report = {
  user_sources: [], openable_paths: [], source_ids: {},
  study_map: { brief: {}, shape: [], directions: [
    { id: "study-a", question: "Study A", reading_anchors: [] },
  ] },
};
const window = {
  location: { hash: "", search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
};
const document = {
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const parsed = {};
[
  "#/overview",
  "#/map",
  "#/mechanisms",
  "#/search",
  "#/mechanism/mechanism%2Fone",
  "#/architecture?focus=component%3Arouter%252Fcore",
  "#/study/study-a",
].forEach((hash) => { parsed[hash] = api.parseWorkspaceHash(hash, null); });
const flowStep = { kind: "flow_step", flow_id: "flow/one", step_id: "step:two" };
process.stdout.write(JSON.stringify({
  parsed,
  studyHash: api.workspaceHashForState({ view: "study", directionID: "study-a" }),
  flowFocus: api.architectureFocusValue(flowStep),
  flowRoundTrip: api.architectureTargetFromFocus(api.architectureFocusValue(flowStep)),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-workspace-route-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run workspace route test: %v\n%s", err, output)
	}
	var got struct {
		Parsed map[string]struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View      string `json:"view"`
				Direction string `json:"directionID"`
				MapTarget *struct {
					Kind        string `json:"kind"`
					ComponentID string `json:"component_id"`
				} `json:"mapTarget"`
			} `json:"state"`
		} `json:"parsed"`
		StudyHash     string `json:"studyHash"`
		FlowFocus     string `json:"flowFocus"`
		FlowRoundTrip struct {
			Kind   string `json:"kind"`
			FlowID string `json:"flow_id"`
			StepID string `json:"step_id"`
		} `json:"flowRoundTrip"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode workspace route result: %v\n%s", err, output)
	}
	for _, hash := range []string{"#/mechanisms", "#/search", "#/mechanism/mechanism%2Fone"} {
		route := got.Parsed[hash]
		if route.Valid || route.State.View != "map" || route.CanonicalHash != "#/map" {
			t.Fatalf("legacy route %q did not fail closed: %#v", hash, route)
		}
	}
	if overview := got.Parsed["#/overview"]; !overview.Valid || overview.State.View != "map" || overview.CanonicalHash != "#/map" {
		t.Fatalf("overview alias = %#v", overview)
	}
	if study := got.Parsed["#/study/study-a"]; !study.Valid || study.State.View != "study" || study.State.Direction != "study-a" {
		t.Fatalf("Study route = %#v", study)
	}
	focus := got.Parsed["#/architecture?focus=component%3Arouter%252Fcore"]
	if !focus.Valid || focus.State.View != "map" || focus.State.MapTarget == nil || focus.State.MapTarget.ComponentID != "router/core" {
		t.Fatalf("architecture focus route = %#v", focus)
	}
	if got.StudyHash != "#/study/study-a" {
		t.Fatalf("Study hash = %q", got.StudyHash)
	}
	if got.FlowRoundTrip.Kind != "flow_step" || got.FlowRoundTrip.FlowID != "flow/one" || got.FlowRoundTrip.StepID != "step:two" {
		t.Fatalf("flow focus round trip = %q %#v", got.FlowFocus, got.FlowRoundTrip)
	}
}

func TestUserWorkspaceRouteAwareScroll(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const snippet = {
  path: "router.go", start_line: 40, end_line: 42,
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
};
const report = {
  user_sources: [], openable_paths: ["router.go"], source_ids: {},
  study_map: { brief: {}, shape: [], directions: [
    { id: "study-a", question: "Study A", reading_anchors: [] },
    { id: "study-b", question: "Study B", reading_anchors: [] },
  ] },
  operations: { version: 1, paths: [
    { id: "operate-a", title: "Operate A", actions: [] },
  ], landmarks: [] },
};
const entries = [{ hash: "#/map", state: null }];
let historyIndex = 0;
const scrollCalls = [];
const window = {
  location: { hash: "#/map", search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  scrollY: 0,
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
  scrollTo(x, y) { scrollCalls.push([x, y]); this.scrollY = y; },
};
window.history = {
  get state() { return entries[historyIndex].state; },
  pushState(state, _title, hash) {
    entries.splice(historyIndex + 1);
    entries.push({ hash, state });
    historyIndex = entries.length - 1;
    window.location.hash = hash;
  },
  replaceState(state, _title, hash) {
    entries[historyIndex] = { hash, state };
    window.location.hash = hash;
  },
};
const document = {
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
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
function snapshot() {
  const state = api.workspaceStateSnapshot();
  return {
    hash: window.location.hash,
    scrollY: window.scrollY,
    scrollCalls: scrollCalls.length,
    view: state.view,
  };
}
window.scrollY = 840;
api.openStudyDirection("study-a");
const studyA = snapshot();
window.scrollY = 510;
api.openStudyDirection("study-b");
const studyB = snapshot();
window.scrollY = 320;
api.openSourceSnippet(snippet, { path: "router.go", line: 41 });
const drawer = snapshot();
api.closeSourceDrawer();
api.restoreWorkspaceFromRoute();
const closedDrawer = snapshot();
window.scrollY = 430;
api.openPavedPath("operate-a");
const operate = snapshot();
process.stdout.write(JSON.stringify({ studyA, studyB, drawer, closedDrawer, operate }));
`
	runnerPath := filepath.Join(t.TempDir(), "user-workspace-scroll-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run workspace scroll test: %v\n%s", err, output)
	}
	type snapshot struct {
		Hash        string `json:"hash"`
		ScrollY     int    `json:"scrollY"`
		ScrollCalls int    `json:"scrollCalls"`
		View        string `json:"view"`
	}
	var got struct {
		StudyA       snapshot `json:"studyA"`
		StudyB       snapshot `json:"studyB"`
		Drawer       snapshot `json:"drawer"`
		ClosedDrawer snapshot `json:"closedDrawer"`
		Operate      snapshot `json:"operate"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode workspace scroll result: %v\n%s", err, output)
	}
	assertSnapshot := func(name string, got snapshot, hash, view string, scrollY, calls int) {
		t.Helper()
		if got.Hash != hash || got.View != view || got.ScrollY != scrollY || got.ScrollCalls != calls {
			t.Errorf("%s = %#v, want hash %q view %q scrollY %d scroll calls %d",
				name, got, hash, view, scrollY, calls)
		}
	}
	assertSnapshot("Map to Study A", got.StudyA, "#/study/study-a", "study", 0, 1)
	assertSnapshot("Study A to Study B", got.StudyB, "#/study/study-b", "study", 0, 2)
	assertSnapshot("open source drawer", got.Drawer, "#/study/study-b", "study", 320, 2)
	assertSnapshot("close source drawer", got.ClosedDrawer, "#/study/study-b", "study", 320, 2)
	assertSnapshot("Study to Operate", got.Operate, "#/operate/operate-a", "operate", 0, 3)
}

func TestUserWorkspaceNavigationWritesAndRestoresHistory(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
function navButton(view) {
  const values = new Set();
  return {
    attributes: { "data-workspace-view": view },
    classList: {
      toggle(name, force) { if (force) values.add(name); else values.delete(name); },
      contains(name) { return values.has(name); },
    },
    getAttribute(name) { return this.attributes[name] || ""; },
    setAttribute(name, value) { this.attributes[name] = String(value); },
    removeAttribute(name) { delete this.attributes[name]; },
  };
}
const mapTab = navButton("map");
const studyTab = navButton("study_overview");
const report = {
  user_sources: [], openable_paths: [], source_ids: {},
  study_map: { brief: {}, shape: [], directions: [
    { id: "study-a", question: "Study A", reading_anchors: [] },
  ] },
  operations: { version: 1, paths: [
    { id: "operate-a", title: "Operate A", actions: [] },
  ], landmarks: [] },
};
const entries = [{ hash: "#/study/study-a", state: null }];
let historyIndex = 0;
const window = {
  location: { hash: "#/study/study-a", search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, scrollTo() {},
};
window.history = {
  get state() { return entries[historyIndex].state; },
  pushState(state, _title, hash) {
    entries.splice(historyIndex + 1);
    entries.push({ hash, state });
    historyIndex = entries.length - 1;
    window.location.hash = hash;
  },
  replaceState(state, _title, hash) {
    entries[historyIndex] = { hash, state };
    window.location.hash = hash;
  },
  back() {
    if (historyIndex > 0) historyIndex--;
    window.location.hash = entries[historyIndex].hash;
  },
};
const document = {
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelector() { return null; },
  querySelectorAll(selector) {
    if (selector === "[data-workspace-view]") return [mapTab, studyTab];
    return [];
  },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.restoreWorkspaceFromRoute();
const study = api.workspaceStateSnapshot();
api.openPavedPath("operate-a");
const operate = api.workspaceStateSnapshot();
const operateHash = window.location.hash;
const operateMapActive = mapTab.classList.contains("rm-active") && mapTab.attributes["aria-current"] === "page";
window.history.back();
api.restoreWorkspaceFromRoute();
const backed = api.workspaceStateSnapshot();
const backedHash = window.location.hash;
const backStudyActive = studyTab.classList.contains("rm-active") && studyTab.attributes["aria-current"] === "page";
api.navigateWorkspace("overview");
const overviewAlias = api.workspaceStateSnapshot();
process.stdout.write(JSON.stringify({
  study, operate, operateHash, operateMapActive,
  backed, backedHash, backStudyActive,
  overviewAlias, overviewHash: window.location.hash,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-workspace-navigation-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run workspace navigation test: %v\n%s", err, output)
	}
	type state struct {
		View        string `json:"view"`
		DirectionID string `json:"directionID"`
		OperationID string `json:"operationID"`
	}
	var got struct {
		Study            state  `json:"study"`
		Operate          state  `json:"operate"`
		OperateHash      string `json:"operateHash"`
		OperateMapActive bool   `json:"operateMapActive"`
		Backed           state  `json:"backed"`
		BackedHash       string `json:"backedHash"`
		BackStudyActive  bool   `json:"backStudyActive"`
		OverviewAlias    state  `json:"overviewAlias"`
		OverviewHash     string `json:"overviewHash"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode workspace navigation result: %v\n%s", err, output)
	}
	if got.Study.View != "study" || got.Study.DirectionID != "study-a" {
		t.Fatalf("initial Study state = %#v", got.Study)
	}
	if got.OperateHash != "#/operate/operate-a" || got.Operate.View != "operate" ||
		got.Operate.OperationID != "operate-a" || !got.OperateMapActive {
		t.Fatalf("Study to Operate = hash %q state %#v map active %t",
			got.OperateHash, got.Operate, got.OperateMapActive)
	}
	if got.BackedHash != "#/study/study-a" || got.Backed.View != "study" ||
		got.Backed.DirectionID != "study-a" || !got.BackStudyActive {
		t.Fatalf("Operate browser Back = hash %q state %#v Study active %t",
			got.BackedHash, got.Backed, got.BackStudyActive)
	}
	if got.OverviewHash != "#/map" || got.OverviewAlias.View != "map" {
		t.Fatalf("legacy Overview navigation = hash %q state %#v", got.OverviewHash, got.OverviewAlias)
	}
}

func TestGenericHostedSourceActionOpensExactEmbeddedDrawer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = String(tag || "div").toUpperCase();
    this.children = [];
    this.attributes = {};
    this.className = "";
    this.textContent = "";
    this.hidden = false;
    this.parentNode = null;
    this.style = {};
    this.classList = {
      add: (...names) => {
        const values = new Set(String(this.className).split(/\s+/).filter(Boolean));
        names.forEach((name) => values.add(name));
        this.className = Array.from(values).join(" ");
      },
      remove: (...names) => {
        const removed = new Set(names);
        this.className = String(this.className).split(/\s+/).filter((name) => name && !removed.has(name)).join(" ");
      },
      contains: (name) => String(this.className).split(/\s+/).includes(name),
    };
  }
  get childNodes() { return this.children; }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  replaceChildren(...children) { this.children = []; this.textContent = ""; this.append(...children); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : this.attributes[name]; }
  addEventListener() {}
  contains(candidate) {
    if (candidate === this) return true;
    return this.children.some((child) => child && typeof child.contains === "function" && child.contains(candidate));
  }
  focus() { document.activeElement = this; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
}
function walk(root) {
  const result = [];
  (function visit(node) { if (!node) return; result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
const snippet = {
  path: "pkg/router.go", start_line: 40, end_line: 42,
  presentation_sha256: "a".repeat(64),
  highlight_ranges: [{ start_line: 41, end_line: 41 }],
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
};
const report = {
  user_mechanisms: [], user_sources: [snippet],
  openable_paths: ["pkg/router.go"], source_ids: {},
};
const workspace = new Element("main");
workspace.className = "rm-workspace";
const drawer = new Element("aside");
drawer.hidden = true;
const content = new Element("div");
const close = new Element("button");
const body = new Element("body");
body.append(workspace, drawer);
const secondClick = new Element("button");
workspace.appendChild(secondClick);
const historyEntries = [{ hash: "#/map", state: null }];
let historyIndex = 0;
const window = {
  location: {
    hash: "#/map", search: "", hostname: "127.0.0.1", protocol: "http:",
    pathname: "/saved-run/report.html",
  },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
  setTimeout, clearTimeout,
};
window.history = {
  get state() { return historyEntries[historyIndex].state; },
  pushState(state, _title, hash) {
    historyEntries.splice(historyIndex + 1);
    historyEntries.push({ state, hash });
    historyIndex = historyEntries.length - 1;
    window.location.hash = hash;
  },
  replaceState(state, _title, hash) {
    historyEntries[historyIndex] = { state, hash };
    window.location.hash = hash;
  },
  back() {},
};
const roots = {
  "rm-source-drawer": drawer,
  "rm-source-drawer-content": content,
  "rm-source-drawer-close": close,
};
const document = {
  body, activeElement: secondClick,
  documentElement: { lang: "en" },
  createElement: (tag) => new Element(tag),
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll() { return []; },
};
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise, setTimeout, clearTimeout,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const location = { path: "pkg/router.go", line: 41, column: 3 };
const available = api.sourceLocationActionAvailable(location);
secondClick.onclick = () => api.openSourceLocation(location);
secondClick.onclick();
const state = api.workspaceStateSnapshot();
const sourceCard = walk(content).find((node) => node.attributes["data-source-path"] === "pkg/router.go");
const highlighted = walk(content).filter((node) =>
  String(node.className).split(/\s+/).includes("is-highlighted") &&
  node.attributes["data-source-line"] === "41");
process.stdout.write(JSON.stringify({
  available,
  drawerVisible: !drawer.hidden,
  workspaceHasDrawer: workspace.classList.contains("has-source-drawer"),
  sourcePath: state.sourceLocation && state.sourceLocation.path,
  sourceLine: state.sourceLocation && state.sourceLocation.line,
  sourceColumn: state.sourceLocation && state.sourceLocation.column,
  historyHasDrawer: !!(window.history.state && window.history.state.sourceDrawer),
  renderedPath: sourceCard && sourceCard.attributes["data-source-path"],
  exactHighlightedLines: highlighted.length,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "generic-host-source-drawer-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run generic-host source drawer journey: %v\n%s", err, output)
	}
	var got struct {
		Available             bool   `json:"available"`
		DrawerVisible         bool   `json:"drawerVisible"`
		WorkspaceHasDrawer    bool   `json:"workspaceHasDrawer"`
		SourcePath            string `json:"sourcePath"`
		SourceLine            int    `json:"sourceLine"`
		SourceColumn          int    `json:"sourceColumn"`
		HistoryHasDrawer      bool   `json:"historyHasDrawer"`
		RenderedPath          string `json:"renderedPath"`
		ExactHighlightedLines int    `json:"exactHighlightedLines"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode generic-host source drawer journey: %v\n%s", err, output)
	}
	if !got.Available || !got.DrawerVisible || !got.WorkspaceHasDrawer ||
		!got.HistoryHasDrawer || got.SourcePath != "pkg/router.go" ||
		got.SourceLine != 41 || got.SourceColumn != 3 ||
		got.RenderedPath != "pkg/router.go" || got.ExactHighlightedLines != 1 {
		t.Fatalf("generic-host embedded source action = %#v", got)
	}
}

func TestUserWorkspaceStaticSourceActionsRequireCodeBackedLocations(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const snippet = {
  path: "router.go", start_line: 40, end_line: 43,
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
};
const report = {
  user_mechanisms: [], user_sources: [snippet],
  openable_paths: ["router.go", "other.go"], source_ids: {},
};
const window = {
  location: { search: "", hostname: "", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
};
const document = {
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
process.stdout.write(JSON.stringify({
  path: api.sourceLocationActionAvailable({ path: "router.go" }),
  exact: api.sourceLocationActionAvailable({ path: "router.go", line: 41 }),
  outsideSnippet: api.sourceLocationActionAvailable({ path: "router.go", line: 99 }),
  noCode: api.sourceLocationActionAvailable({ path: "other.go" }),
  outsideSnippetResolved: !!api.embeddedSourceForLocation({ path: "router.go", line: 99 }),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-static-search-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run static source eligibility check: %v\n%s", err, output)
	}
	var got struct {
		Path                   bool `json:"path"`
		Exact                  bool `json:"exact"`
		OutsideSnippet         bool `json:"outsideSnippet"`
		NoCode                 bool `json:"noCode"`
		OutsideSnippetResolved bool `json:"outsideSnippetResolved"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode source eligibility result: %v\n%s", err, output)
	}
	if !got.Path || !got.Exact || got.OutsideSnippet || got.NoCode || got.OutsideSnippetResolved {
		t.Fatalf("static source eligibility = %#v", got)
	}
}
