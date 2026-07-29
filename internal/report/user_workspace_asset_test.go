package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserWorkspaceReducerPreservesMechanismContext(t *testing.T) {
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
const report = { user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {} };
const window = {
  location: { search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
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
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const mechanisms = [{
  artifact_id: "mechanism-1",
  steps: [{ title: "one" }, { title: "two" }, { title: "three" }],
}];
const snippet = {
  path: "router.go",
  start_line: 40,
  end_line: 43,
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
};
let state = {
  view: "overview", artifactID: "", stepIndex: 0,
  sourceLocation: null, mapReturn: null, mapTarget: null,
};
const snapshots = {};
state = api.reduceWorkspaceState(state, {
  type: "open_mechanism", artifactID: "mechanism-1", stepIndex: 1,
}, mechanisms);
snapshots.open = state;
state = api.reduceWorkspaceState(state, {
  type: "open_source", location: { path: "router.go", line: 42, column: 3 },
}, mechanisms);
snapshots.bare = state;
state = api.reduceWorkspaceState(state, {
  type: "open_source",
  selection: { path: "router.go", line: 41, column: 3, snippet },
}, mechanisms);
snapshots.source = state;
state = api.reduceWorkspaceState(state, { type: "close_source" }, mechanisms);
snapshots.closed = state;
state = api.reduceWorkspaceState(state, {
  type: "open_source",
  selection: { path: "router.go", line: 41, column: 3, snippet },
}, mechanisms);
state = api.reduceWorkspaceState(state, {
  type: "show_map", target: { kind: "component", component_id: "router" },
}, mechanisms);
snapshots.map = state;
state = api.reduceWorkspaceState(state, { type: "return_from_map" }, mechanisms);
snapshots.returned = state;
state = api.reduceWorkspaceState(state, { type: "move_step", delta: 99 }, mechanisms);
snapshots.bounded = state;
process.stdout.write(JSON.stringify(snapshots));
`
	runnerPath := filepath.Join(t.TempDir(), "user-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run user workspace reducer: %v\n%s", err, output)
	}
	type sourceSelection struct {
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Snippet struct {
			Path  string `json:"path"`
			Lines []struct {
				Line      int    `json:"line"`
				Text      string `json:"text"`
				Highlight bool   `json:"highlight"`
			} `json:"lines"`
		} `json:"snippet"`
	}
	type mechanismState struct {
		View           string           `json:"view"`
		ArtifactID     string           `json:"artifactID"`
		StepIndex      int              `json:"stepIndex"`
		SourceLocation *sourceSelection `json:"sourceLocation"`
		MapReturn      *struct {
			ArtifactID string `json:"artifactID"`
			StepIndex  int    `json:"stepIndex"`
		} `json:"mapReturn"`
	}
	var got struct {
		Open     mechanismState `json:"open"`
		Bare     mechanismState `json:"bare"`
		Source   mechanismState `json:"source"`
		Closed   mechanismState `json:"closed"`
		Map      mechanismState `json:"map"`
		Returned mechanismState `json:"returned"`
		Bounded  mechanismState `json:"bounded"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode reducer result: %v\n%s", err, output)
	}
	if got.Open.View != "mechanism" || got.Open.ArtifactID != "mechanism-1" || got.Open.StepIndex != 1 {
		t.Fatalf("open mechanism state = %#v", got.Open)
	}
	if got.Bare.SourceLocation != nil {
		t.Fatalf("bare path opened a source drawer: %#v", got.Bare.SourceLocation)
	}
	if got.Bare.ArtifactID != "mechanism-1" || got.Bare.StepIndex != 1 {
		t.Fatalf("bare path changed mechanism context: %#v", got.Bare)
	}
	if got.Source.SourceLocation == nil || got.Source.SourceLocation.Path != "router.go" || got.Source.SourceLocation.Line != 41 {
		t.Fatalf("real snippet did not open source: %#v", got.Source.SourceLocation)
	}
	if got.Source.SourceLocation.Snippet.Path != "router.go" || len(got.Source.SourceLocation.Snippet.Lines) != 3 || !got.Source.SourceLocation.Snippet.Lines[1].Highlight {
		t.Fatalf("source selection lost code or highlights: %#v", got.Source.SourceLocation.Snippet)
	}
	if got.Source.ArtifactID != "mechanism-1" || got.Source.StepIndex != 1 {
		t.Fatalf("source selection changed mechanism context: %#v", got.Source)
	}
	if got.Closed.SourceLocation != nil || got.Closed.View != "mechanism" || got.Closed.ArtifactID != "mechanism-1" || got.Closed.StepIndex != 1 {
		t.Fatalf("closing source did not restore the same mechanism step: %#v", got.Closed)
	}
	if got.Map.View != "architecture" || got.Map.MapReturn == nil || got.Map.MapReturn.ArtifactID != "mechanism-1" || got.Map.MapReturn.StepIndex != 1 {
		t.Fatalf("map state = %#v", got.Map)
	}
	if got.Returned.View != "mechanism" || got.Returned.ArtifactID != "mechanism-1" || got.Returned.StepIndex != 1 {
		t.Fatalf("return state = %#v", got.Returned)
	}
	if got.Returned.SourceLocation == nil || got.Returned.SourceLocation.Path != "router.go" {
		t.Fatalf("source selection was lost across map round-trip: %#v", got.Returned.SourceLocation)
	}
	if got.Bounded.StepIndex != 2 {
		t.Fatalf("bounded step = %d, want 2", got.Bounded.StepIndex)
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
    { label: "Start here", what_to_look_for: "Inspect the public boundary.", location: { path: "router.go", line: 11 }, source: snippet("router.go", "Route", 10) },
    { label: "Then inspect", what_to_look_for: "Inspect the core data lookup.", location: { path: "tree.go", line: 21 }, source: snippet("tree.go", "Find", 20) },
    { label: "Related implementation", what_to_look_for: "Inspect request-local state.", location: { path: "context.go", line: 31 }, source: snippet("context.go", "Reset", 30) },
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
const mechanism = { artifact_id: "mechanism-dispatch", steps: [{ title: "Dispatch" }] };
const report = {
  repo_name: "fixture", user_mechanisms: [mechanism], user_sources: [],
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
    directions: [reading, attached],
  },
};
const roots = { "rm-overview": new Element("section") };
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelectorAll() { return []; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function text(node) { return String(node.textContent || "") + (node.children || []).map(text).join(""); }
const route = api.parseWorkspaceHash("#/study/study-routing", [mechanism], null);
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
api.renderOverviewWorkspace();
const shelfOverviewText = text(roots["rm-overview"]);
report.user_mechanisms.length = 0;
const emptyRoots = { "rm-overview": new Element("section") };
const emptyWindow = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const emptyDocument = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return emptyRoots[id] || null;
  },
  querySelectorAll() { return []; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window: emptyWindow, document: emptyDocument, URLSearchParams, Set, Map, AbortController,
});
emptyWindow.__REPOMAP_WORKSPACE_TEST__.renderOverviewWorkspace();
const card = api.renderStudyDirectionCard(reading, 0);
process.stdout.write(JSON.stringify({
  route, attachedRoute, invalidRoute, sourceState, closedState, returned,
  shelfOverviewText, emptyShelfOverviewText: text(emptyRoots["rm-overview"]), cardText: text(card),
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
				View       string `json:"view"`
				ArtifactID string `json:"artifactID"`
			} `json:"state"`
		} `json:"attachedRoute"`
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
		EmptyShelfOverviewText string `json:"emptyShelfOverviewText"`
		CardText               string `json:"cardText"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Study Map workspace smoke: %v\n%s", err, output)
	}
	if !got.Route.Valid || got.Route.CanonicalHash != "#/study/study-routing" ||
		got.Route.State.View != "study" || got.Route.State.DirectionID != "study-routing" {
		t.Fatalf("reading route = %#v", got.Route)
	}
	if got.AttachedRoute.CanonicalHash != "#/mechanism/mechanism-dispatch" ||
		got.AttachedRoute.State.View != "mechanism" || got.AttachedRoute.State.ArtifactID != "mechanism-dispatch" {
		t.Fatalf("attached route = %#v", got.AttachedRoute)
	}
	if got.InvalidRoute.Valid || got.InvalidRoute.CanonicalHash != "#/overview" {
		t.Fatalf("invalid route = %#v", got.InvalidRoute)
	}
	if got.SourceState.View != "study" || got.SourceState.DirectionID != "study-routing" || got.SourceState.SourceLocation == nil ||
		got.ClosedState.View != "study" || got.ClosedState.DirectionID != "study-routing" || got.ClosedState.SourceLocation != nil {
		t.Fatalf("source drawer changed reading context: source=%#v closed=%#v", got.SourceState, got.ClosedState)
	}
	if got.Returned.View != "study" || got.Returned.DirectionID != "study-routing" {
		t.Fatalf("map return = %#v", got.Returned)
	}
	for _, token := range []string{
		"Pick a path worth following.",
		"Full mechanism · 1 source-backed step",
		"How this code works",
	} {
		if !strings.Contains(got.ShelfOverviewText, token) {
			t.Errorf("mixed shelf is missing %q: %q", token, got.ShelfOverviewText)
		}
	}
	if strings.Contains(got.ShelfOverviewText, "Repository brief") {
		t.Fatalf("non-empty mixed shelf fell through to Study Map: %q", got.ShelfOverviewText)
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
	if !strings.Contains(got.CardText, "Explore this direction →") || strings.Contains(got.CardText, "runtime") {
		t.Fatalf("Study Direction card = %q", got.CardText)
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
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
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

func TestUserWorkspaceOverviewRendersExplicitCTAAndRankedCodeLandmarks(t *testing.T) {
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
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
const snippet = {
  path: "api/public.go", enclosing_symbol: "Serve", start_line: 8, end_line: 10,
  role: "core", landmark_kind: "public_api", landmark_reason: "Exported API: Serve.",
  revision: "deadbeef", highlight_ranges: [{ start_line: 9, end_line: 9 }],
  lines: [
    { line: 8, text: "func Serve() {" },
    { line: 9, text: "  run()", highlight: true },
    { line: 10, text: "}" },
  ],
};
const report = {
  repo_name: "fixture", user_mechanisms: [], user_sources: [snippet],
  openable_paths: ["api/public.go"], source_ids: {},
  semantic_search: { version: 1, items: [], suggestions: [] },
};
const roots = { "rm-overview": new Element("section") };
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  RepomapSemanticSearch: {}, __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelectorAll() { return []; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function walk(root) {
  const result = [];
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(node) { return String(node.textContent || "") + (node.children || []).map(text).join(""); }
api.renderOverviewWorkspace();
const overviewNodes = walk(roots["rm-overview"]);
const mechanismCard = api.renderUserMechanismCard({
  artifact_id: "mechanism-1", question: "How does routing work?",
  presentation_title: "How routing works", steps: [{ title: "Route" }], files: [{ path: "router.go" }],
});
process.stdout.write(JSON.stringify({
  overviewText: text(roots["rm-overview"]),
  sourceCards: overviewNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-source-card")).length,
  codeBlocks: overviewNodes.filter((node) => node.attributes && node.attributes["data-source-content"] === "true").length,
  snapshots: overviewNodes.filter((node) => String(node.className).includes("rm-source-card__snapshot")).length,
  mechanismCardText: text(mechanismCard),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-overview-renderer-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run overview renderer: %v\n%s", err, output)
	}
	var got struct {
		OverviewText      string `json:"overviewText"`
		SourceCards       int    `json:"sourceCards"`
		CodeBlocks        int    `json:"codeBlocks"`
		Snapshots         int    `json:"snapshots"`
		MechanismCardText string `json:"mechanismCardText"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode overview renderer result: %v\n%s", err, output)
	}
	for _, token := range []string{"Where to start", "Public API", "Exported API: Serve.", "func Serve() {"} {
		if !strings.Contains(got.OverviewText, token) {
			t.Errorf("overview is missing %q: %q", token, got.OverviewText)
		}
	}
	if got.SourceCards != 1 || got.CodeBlocks != 1 {
		t.Fatalf("overview source rendering = %d cards, %d code blocks", got.SourceCards, got.CodeBlocks)
	}
	if got.Snapshots != 0 || strings.Contains(got.OverviewText, "saved snapshot") {
		t.Fatalf("overview exposed snapshot metadata: %q", got.OverviewText)
	}
	if strings.Contains(got.OverviewText, "Search") {
		t.Fatalf("empty-shelf overview exposed Search fallback: %q", got.OverviewText)
	}
	if !strings.Contains(got.MechanismCardText, "Open code path →") ||
		strings.Contains(got.MechanismCardText, "Open a code path") {
		t.Fatalf("mechanism CTA = %q", got.MechanismCardText)
	}
}

func TestUserWorkspaceOnboardingRendersThesisRolesAndNarrativePhases(t *testing.T) {
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
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 2,
    highlight_ranges: [{ start_line: line + 1, end_line: line + 1 }],
    lines: [
      { line, text: "func " + symbol + "() {" },
      { line: line + 1, text: "  work()", highlight: true },
      { line: line + 2, text: "}" },
    ],
  };
}
const receive = snippet("cmd/main.go", "receive", 10);
const prepare = snippet("internal/prepare.go", "prepare", 20);
const apply = snippet("internal/apply.go", "apply", 30);
const publish = snippet("internal/publish.go", "publish", 40);
const extension = snippet("internal/registry.go", "register", 50);
const main = {
  artifact_id: "main-mechanism",
  role: "primary_behavior",
  title: "How Fixture handles an input",
  question: "How does Fixture handle an input?",
  answer: "Fixture receives an input, prepares the core operation, applies it, and publishes the result.",
  files: [
    { path: "cmd/main.go" }, { path: "internal/prepare.go" },
    { path: "internal/apply.go" }, { path: "internal/publish.go" },
    { path: "internal/extra.go" },
  ],
  context: [{
    label: "Configuration", responsibility: "Supplies settings used by this path.",
    code_location: { path: "cmd/main.go", line: 11 },
  }],
  steps: [
    { title: "Receive input", explanation: "Receive the input.", locations: [{ path: "cmd/main.go", line: 11 }], sources: [receive] },
    { title: "Prepare work", explanation: "Prepare the operation.", locations: [{ path: "internal/prepare.go", line: 21 }], sources: [prepare] },
    { title: "Apply work", explanation: "Apply the operation.", locations: [{ path: "internal/apply.go", line: 31 }], sources: [apply] },
    { title: "Publish result", explanation: "Publish the result.", locations: [{ path: "internal/publish.go", line: 41 }], sources: [publish] },
  ],
  phases: [
    { title: "Accept the input", explanation: "The boundary accepts the input.", locations: [{ path: "cmd/main.go", line: 11 }], sources: [receive], implementation_step_indexes: [0] },
    { title: "Perform the core work", explanation: "The core prepares and applies the operation.", locations: [{ path: "internal/prepare.go", line: 21 }, { path: "internal/apply.go", line: 31 }], sources: [prepare, apply], implementation_step_indexes: [1, 2] },
    { title: "Publish the result", explanation: "The boundary publishes the result.", locations: [{ path: "internal/publish.go", line: 41 }], sources: [publish], implementation_step_indexes: [3] },
  ],
};
const secondary = {
  artifact_id: "extension-mechanism", role: "extension_point",
  title: "How Fixture registers extensions",
  question: "How does Fixture register extensions?",
  answer: "Fixture stores a validated extension factory for later lookup.",
  files: [{ path: "internal/registry.go" }],
  steps: [
    { title: "Read registration", explanation: "Read registration.", sources: [extension] },
    { title: "Validate factory", explanation: "Validate factory.", sources: [extension] },
    { title: "Store factory", explanation: "Store factory.", sources: [extension] },
  ],
};
const report = {
  repo_name: "fixture",
  repository_thesis: {
    purpose: "Fixture turns incoming work into a published result.",
    system_story: ["A boundary receives work.", "Core code prepares and applies it.", "An output boundary publishes the result."],
    recommended_artifact_id: "main-mechanism",
    areas: [
      { label: "Command boundary", responsibility: "Receives work.", code_location: { path: "cmd/main.go", line: 11 } },
      { label: "Core operation", responsibility: "Applies work.", code_location: { path: "internal/apply.go", line: 31 } },
      { label: "Output boundary", responsibility: "Publishes results.", code_location: { path: "internal/publish.go", line: 41 } },
    ],
  },
	repository_guide: {
		purpose: "Fixture turns incoming work into a published result.",
		system_story: ["A boundary receives work.", "Core code prepares and applies it.", "An output boundary publishes the result."],
		start_here_artifact_id: "main-mechanism",
		extension_artifact_ids: ["extension-mechanism"],
		more_path_artifact_ids: [],
		read_next: [
			{ label: "prepare", path: "internal/prepare.go", line: 21, step_index: 1 },
			{ label: "publish", path: "internal/publish.go", line: 41, step_index: 2 },
		],
		areas: [
			{ label: "Command boundary", responsibility: "Receives work.", code_location: { path: "cmd/main.go", line: 11 } },
			{ label: "Core operation", responsibility: "Applies work.", code_location: { path: "internal/apply.go", line: 31 } },
			{ label: "Output boundary", responsibility: "Publishes results.", code_location: { path: "internal/publish.go", line: 41 } },
		],
		architecture_useful: false,
	},
  user_mechanisms: [main, secondary], user_sources: [],
  openable_paths: ["cmd/main.go", "internal/prepare.go", "internal/apply.go", "internal/publish.go", "internal/registry.go", "internal/extra.go"],
  source_ids: {}, architecture_canvas: {}, semantic_search: {},
};
const roots = {
  "rm-overview": new Element("section"),
  "rm-mechanism-detail": new Element("section"),
};
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  RepomapSemanticSearch: {}, __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
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
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(node) { return String(node.textContent || "") + (node.children || []).map(text).join(""); }
api.renderOverviewWorkspace();
const overviewNodes = walk(roots["rm-overview"]);
const mainCard = api.renderUserMechanismCard(main);
const secondaryCard = api.renderUserMechanismCard(secondary);
api.openUserMechanism("main-mechanism", 1, true);
process.stdout.write(JSON.stringify({
  overviewText: text(roots["rm-overview"]),
  areaCards: overviewNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-repository-area")).length,
  mechanismCards: overviewNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-mechanism-card")).length,
  mainCardText: text(mainCard), secondaryCardText: text(secondaryCard),
  principalFiles: walk(mainCard).filter((node) => node.tagName === "code").map((node) => node.textContent),
  narrativeTitles: api.mechanismNarrativeItems(main).map((item) => item.title),
  implementationTitles: api.mechanismImplementationSteps(main, main.phases[1]).map((item) => item.title),
  searchStepPhase: api.narrativeIndexForImplementationStep(main, 2),
  detailText: text(roots["rm-mechanism-detail"]),
	detailSourceCards: walk(roots["rm-mechanism-detail"]).filter((node) => String(node.className).split(/\s+/).includes("rm-source-card")).length,
  detailHash: window.location.hash,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "user-onboarding-renderer-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run onboarding renderer: %v\n%s", err, output)
	}
	var got struct {
		OverviewText         string   `json:"overviewText"`
		AreaCards            int      `json:"areaCards"`
		MechanismCards       int      `json:"mechanismCards"`
		MainCardText         string   `json:"mainCardText"`
		SecondaryCardText    string   `json:"secondaryCardText"`
		PrincipalFiles       []string `json:"principalFiles"`
		NarrativeTitles      []string `json:"narrativeTitles"`
		ImplementationTitles []string `json:"implementationTitles"`
		SearchStepPhase      int      `json:"searchStepPhase"`
		DetailText           string   `json:"detailText"`
		DetailSourceCards    int      `json:"detailSourceCards"`
		DetailHash           string   `json:"detailHash"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode onboarding renderer result: %v\n%s", err, output)
	}
	ordered := []string{
		"Understand the repository",
		"Pick a path worth following.",
		"Complete mechanisms",
		"Source-backed paths",
	}
	previous := -1
	for _, token := range ordered {
		index := strings.Index(got.OverviewText, token)
		if index < 0 || index <= previous {
			t.Fatalf("overview section %q is absent or out of order: %q", token, got.OverviewText)
		}
		previous = index
	}
	if got.AreaCards != 0 || got.MechanismCards != 2 {
		t.Fatalf("overview cards = %d areas, %d mechanisms", got.AreaCards, got.MechanismCards)
	}
	for _, token := range []string{
		"How does Fixture handle an input?",
		"Full mechanism · 3 source-backed steps",
		"Open code path →",
	} {
		if !strings.Contains(got.OverviewText, token) {
			t.Errorf("mixed shelf is missing %q: %q", token, got.OverviewText)
		}
	}
	for _, obsolete := range []string{
		"Purpose", "Repository shape", "Primary path", "Extension paths", "Read next", "System story", "Explore", "Search",
	} {
		if strings.Contains(got.OverviewText, obsolete) {
			t.Errorf("mixed shelf retained obsolete Overview heading %q: %q", obsolete, got.OverviewText)
		}
	}
	for _, token := range []string{
		"Main code path", "Fixture receives an input", "Accept the input", "Perform the core work", "Publish the result",
	} {
		if !strings.Contains(got.MainCardText, token) {
			t.Errorf("main mechanism card is missing %q: %q", token, got.MainCardText)
		}
	}
	if !strings.Contains(got.SecondaryCardText, "Extension path") {
		t.Errorf("secondary mechanism role is absent: %q", got.SecondaryCardText)
	}
	if len(got.PrincipalFiles) != 4 || strings.Contains(strings.Join(got.PrincipalFiles, "\n"), "internal/extra.go") {
		t.Fatalf("principal files = %#v", got.PrincipalFiles)
	}
	if strings.Join(got.NarrativeTitles, "|") != "Accept the input|Perform the core work|Publish the result" ||
		strings.Join(got.ImplementationTitles, "|") != "Prepare work|Apply work" || got.SearchStepPhase != 1 {
		t.Fatalf("narrative projection = phases %#v, implementation %#v, search phase %d",
			got.NarrativeTitles, got.ImplementationTitles, got.SearchStepPhase)
	}
	for _, token := range []string{
		"Phase 2 of 3", "Perform the core work", "Around this path", "Configuration",
		"Show implementation details (2)", "Prepare work", "Apply work", "func prepare() {", "func apply() {",
	} {
		if !strings.Contains(got.DetailText, token) {
			t.Errorf("phase detail is missing %q: %q", token, got.DetailText)
		}
	}
	if got.DetailSourceCards != 2 {
		t.Fatalf("phase source cards = %d, want both compact implementation sources visible", got.DetailSourceCards)
	}
	if got.DetailHash != "#/mechanism/main-mechanism/step/2" {
		t.Fatalf("phase deep link = %q", got.DetailHash)
	}
	for _, forbidden := range []string{
		"primary_behavior", "extension_point", "model verdict", "derived verdict", "Known gaps", "Unresolved",
	} {
		if strings.Contains(got.OverviewText+got.DetailText, forbidden) {
			t.Errorf("default onboarding exposed internal text %q", forbidden)
		}
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
const report = { user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {} };
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
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const mechanisms = [{
  artifact_id: "mechanism/one",
  title: "Machine Mechanism Title",
  question: "How does the router dispatch a request?",
  steps: [{ title: "one" }, { title: "two" }, { title: "three" }, { title: "four" }],
}];
const parsed = {};
[
  "#/overview",
  "#/mechanisms",
  "#/search",
  "#/mechanism/mechanism%2Fone",
  "#/mechanism/mechanism%2Fone/step/4",
  "#/mechanism/missing/step/2",
  "#/mechanism/mechanism%2Fone/step/99",
  "#/architecture?focus=component%3Arouter%252Fcore",
].forEach((hash) => { parsed[hash] = api.parseWorkspaceHash(hash, mechanisms, null); });
const flowStep = { kind: "flow_step", flow_id: "flow/one", step_id: "step:two" };
process.stdout.write(JSON.stringify({
  parsed,
  mechanismStep: api.workspaceHashForState({ view: "mechanism", artifactID: "mechanism/one", stepIndex: 3 }),
  mechanismRoot: api.workspaceHashForState({ view: "mechanism", artifactID: "mechanism/one", stepIndex: 0 }, true),
  flowFocus: api.architectureFocusValue(flowStep),
  flowRoundTrip: api.architectureTargetFromFocus(api.architectureFocusValue(flowStep)),
  focusReset: {
    initial: api.architectureFocusNeedsReset(null, { kind: "component", component_id: "router" }),
    same: api.architectureFocusNeedsReset("component:router", { kind: "component", component_id: "router" }),
    plain: api.architectureFocusNeedsReset("component:router", null),
    changed: api.architectureFocusNeedsReset("component:router", { kind: "component", component_id: "server" }),
  },
  title: api.mechanismPresentationTitle(mechanisms[0]),
  shortAnswer: api.mechanismShortAnswer({ answer: "Source-backed path: A → B" }),
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
				View       string `json:"view"`
				ArtifactID string `json:"artifactID"`
				StepIndex  int    `json:"stepIndex"`
				MapTarget  *struct {
					Kind        string `json:"kind"`
					ComponentID string `json:"component_id"`
				} `json:"mapTarget"`
			} `json:"state"`
		} `json:"parsed"`
		MechanismStep string `json:"mechanismStep"`
		MechanismRoot string `json:"mechanismRoot"`
		FlowFocus     string `json:"flowFocus"`
		FlowRoundTrip struct {
			Kind   string `json:"kind"`
			FlowID string `json:"flow_id"`
			StepID string `json:"step_id"`
		} `json:"flowRoundTrip"`
		FocusReset struct {
			Initial bool `json:"initial"`
			Same    bool `json:"same"`
			Plain   bool `json:"plain"`
			Changed bool `json:"changed"`
		} `json:"focusReset"`
		Title       string `json:"title"`
		ShortAnswer string `json:"shortAnswer"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode workspace route result: %v\n%s", err, output)
	}
	step := got.Parsed["#/mechanism/mechanism%2Fone/step/4"]
	if !step.Valid || step.State.View != "mechanism" || step.State.ArtifactID != "mechanism/one" || step.State.StepIndex != 3 {
		t.Fatalf("step deep link = %#v", step)
	}
	missing := got.Parsed["#/mechanism/missing/step/2"]
	if missing.Valid || missing.State.View != "overview" || missing.CanonicalHash != "#/overview" {
		t.Fatalf("invalid mechanism route = %#v", missing)
	}
	search := got.Parsed["#/search"]
	if search.Valid || search.State.View != "overview" || search.CanonicalHash != "#/overview" {
		t.Fatalf("Search route = %#v, want normal-report Overview canonicalization", search)
	}
	bounded := got.Parsed["#/mechanism/mechanism%2Fone/step/99"]
	if !bounded.Valid || bounded.State.StepIndex != 3 || bounded.CanonicalHash != "#/mechanism/mechanism%2Fone/step/4" {
		t.Fatalf("bounded mechanism route = %#v", bounded)
	}
	focus := got.Parsed["#/architecture?focus=component%3Arouter%252Fcore"]
	if !focus.Valid || focus.State.MapTarget == nil || focus.State.MapTarget.ComponentID != "router/core" {
		t.Fatalf("architecture focus route = %#v", focus)
	}
	if got.MechanismStep != "#/mechanism/mechanism%2Fone/step/4" || got.MechanismRoot != "#/mechanism/mechanism%2Fone" {
		t.Fatalf("mechanism hashes = step %q, root %q", got.MechanismStep, got.MechanismRoot)
	}
	if got.FlowRoundTrip.Kind != "flow_step" || got.FlowRoundTrip.FlowID != "flow/one" || got.FlowRoundTrip.StepID != "step:two" {
		t.Fatalf("flow focus round trip = %q %#v", got.FlowFocus, got.FlowRoundTrip)
	}
	if got.FocusReset.Initial || got.FocusReset.Same || !got.FocusReset.Plain || !got.FocusReset.Changed {
		t.Fatalf("architecture focus reset decisions = %#v", got.FocusReset)
	}
	if got.Title != "How does the router dispatch a request" || got.ShortAnswer != "" {
		t.Fatalf("presentation fallback = title %q, short answer %q", got.Title, got.ShortAnswer)
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
const mechanisms = [{
  artifact_id: "mechanism-1",
  steps: [{ title: "one" }, { title: "two" }],
}];
const directions = [
  { id: "study-a", question: "Study A", reading_anchors: [] },
  { id: "study-b", question: "Study B", reading_anchors: [] },
];
const report = {
  user_mechanisms: mechanisms, user_sources: [],
  openable_paths: ["router.go"], source_ids: {},
  study_map: { brief: {}, shape: [], directions },
};
const entries = [{ hash: "#/overview", state: null }];
let historyIndex = 0;
const scrollCalls = [];
const window = {
  location: { hash: "#/overview", search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
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
  back() {
    if (historyIndex > 0) historyIndex--;
    window.location.hash = entries[historyIndex].hash;
  },
  forward() {
    if (historyIndex + 1 < entries.length) historyIndex++;
    window.location.hash = entries[historyIndex].hash;
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

window.history.back();
window.scrollY = 510; // Native session-history restoration happens outside the workspace router.
api.restoreWorkspaceFromRoute();
const back = snapshot();

window.history.forward();
window.scrollY = 0;
api.restoreWorkspaceFromRoute();
const forward = snapshot();

window.scrollY = 320;
api.openSourceSnippet(snippet, { path: "router.go", line: 41 });
const drawer = snapshot();
api.closeSourceDrawer();
api.restoreWorkspaceFromRoute();
const closedDrawer = snapshot();

api.openUserMechanism("mechanism-1", 0, false);
const mechanism = snapshot();
window.scrollY = 275;
api.selectUserMechanismStep(1);
const mechanismStep = snapshot();

api.navigateWorkspace("search");
const search = snapshot();
window.scrollY = 430;
api.openStudyDirection("study-a");
const searchStudy = snapshot();

process.stdout.write(JSON.stringify({
  studyA, studyB, back, forward, drawer, closedDrawer,
  mechanism, mechanismStep, search, searchStudy,
}));
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
		StudyA        snapshot `json:"studyA"`
		StudyB        snapshot `json:"studyB"`
		Back          snapshot `json:"back"`
		Forward       snapshot `json:"forward"`
		Drawer        snapshot `json:"drawer"`
		ClosedDrawer  snapshot `json:"closedDrawer"`
		Mechanism     snapshot `json:"mechanism"`
		MechanismStep snapshot `json:"mechanismStep"`
		Search        snapshot `json:"search"`
		SearchStudy   snapshot `json:"searchStudy"`
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
	assertSnapshot("overview to Study A", got.StudyA, "#/study/study-a", "study", 0, 1)
	assertSnapshot("Study A to Study B", got.StudyB, "#/study/study-b", "study", 0, 2)
	assertSnapshot("browser Back", got.Back, "#/study/study-a", "study", 510, 2)
	assertSnapshot("browser Forward", got.Forward, "#/study/study-b", "study", 0, 2)
	assertSnapshot("open source drawer", got.Drawer, "#/study/study-b", "study", 320, 2)
	assertSnapshot("close source drawer", got.ClosedDrawer, "#/study/study-b", "study", 320, 2)
	assertSnapshot("open Mechanism", got.Mechanism, "#/mechanism/mechanism-1", "mechanism", 0, 3)
	assertSnapshot("select Mechanism step", got.MechanismStep, "#/mechanism/mechanism-1/step/2", "mechanism", 275, 3)
	assertSnapshot("Search canonicalizes to Overview", got.Search, "#/overview", "overview", 0, 4)
	assertSnapshot("Search to Study", got.SearchStudy, "#/study/study-a", "study", 0, 5)
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
const snippet = {
  path: "router.go", start_line: 40, end_line: 42,
  highlight_ranges: [{ start_line: 41, end_line: 41 }],
  lines: [
    { line: 40, text: "func route() {" },
    { line: 41, text: "  dispatch()", highlight: true },
    { line: 42, text: "}" },
  ],
};
const mechanisms = [{
  artifact_id: "mechanism-1", title: "How the router dispatches a request",
  question: "How does the router dispatch a request?",
  steps: [
    { title: "one", explanation: "one", sources: [snippet] },
    { title: "two", explanation: "two", sources: [snippet] },
    { title: "three", explanation: "three", sources: [snippet] },
    { title: "four", explanation: "four", sources: [snippet] },
  ],
}];
const report = {
  user_mechanisms: mechanisms, user_sources: [],
  openable_paths: ["router.go"], source_ids: {}, architecture_canvas: {},
};
const entries = [{ hash: "#/overview", state: null }];
let historyIndex = 0;
let backCalls = 0;
const window = {
  location: { hash: "#/overview", search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
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
    backCalls++;
    if (historyIndex === 0) return;
    historyIndex--;
    window.location.hash = entries[historyIndex].hash;
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
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.openUserMechanism("mechanism-1", 0, false);
const openedHash = window.location.hash;
api.selectUserMechanismStep(3);
const stepHash = window.location.hash;
const historyCountBeforeSameStep = entries.length;
api.selectUserMechanismStep(3);
const historyCountAfterSameStep = entries.length;
api.restoreWorkspaceFromRoute();
const reloaded = api.workspaceStateSnapshot();
window.history.back();
api.restoreWorkspaceFromRoute();
const backed = api.workspaceStateSnapshot();

api.selectUserMechanismStep(3);
api.openSourceSnippet(snippet, { path: "router.go", line: 41 });
const drawerHash = window.location.hash;
const drawerState = window.history.state;
api.closeSourceDrawer();
api.restoreWorkspaceFromRoute();
const closed = api.workspaceStateSnapshot();
api.navigateWorkspace("search");
api.openSourceSnippet(snippet, { path: "router.go", line: 41 });
const searchDrawer = api.workspaceStateSnapshot();
const searchDrawerHasMechanism = !!api.activeSourceDrawerMechanism();
const searchHash = window.location.hash;
api.openUserMechanism("mechanism-1", 1, true);
api.showMechanismStepOnMap({ kind: "component", component_id: "router" });
const mapHash = window.location.hash;
const backCallsBeforeMapReturn = backCalls;
api.returnFromArchitecture();
const mapReturnHash = window.location.hash;
const mapReturned = api.workspaceStateSnapshot();
process.stdout.write(JSON.stringify({
  openedHash, stepHash, reloaded, backed, drawerHash,
  historyCountBeforeSameStep, historyCountAfterSameStep,
  drawerHistory: !!(drawerState && drawerState.sourceDrawer), closed,
  searchDrawer, searchDrawerHasMechanism, searchHash,
  mapHash, mapReturnHash, mapReturned,
  explicitMapReturnBackCalls: backCalls - backCallsBeforeMapReturn,
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
		View           string          `json:"view"`
		ArtifactID     string          `json:"artifactID"`
		StepIndex      int             `json:"stepIndex"`
		SourceLocation json.RawMessage `json:"sourceLocation"`
	}
	var got struct {
		OpenedHash                 string `json:"openedHash"`
		StepHash                   string `json:"stepHash"`
		Reloaded                   state  `json:"reloaded"`
		Backed                     state  `json:"backed"`
		DrawerHash                 string `json:"drawerHash"`
		DrawerHistory              bool   `json:"drawerHistory"`
		Closed                     state  `json:"closed"`
		SearchDrawer               state  `json:"searchDrawer"`
		SearchHash                 string `json:"searchHash"`
		SearchDrawerHasMechanism   bool   `json:"searchDrawerHasMechanism"`
		MapHash                    string `json:"mapHash"`
		MapReturnHash              string `json:"mapReturnHash"`
		MapReturned                state  `json:"mapReturned"`
		ExplicitMapReturnBackCalls int    `json:"explicitMapReturnBackCalls"`
		HistoryCountBeforeSameStep int    `json:"historyCountBeforeSameStep"`
		HistoryCountAfterSameStep  int    `json:"historyCountAfterSameStep"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode workspace navigation result: %v\n%s", err, output)
	}
	if got.OpenedHash != "#/mechanism/mechanism-1" || got.StepHash != "#/mechanism/mechanism-1/step/4" {
		t.Fatalf("navigation hashes = open %q, step %q", got.OpenedHash, got.StepHash)
	}
	if got.HistoryCountAfterSameStep != got.HistoryCountBeforeSameStep {
		t.Fatalf("active step added a no-op history entry: before %d, after %d",
			got.HistoryCountBeforeSameStep, got.HistoryCountAfterSameStep)
	}
	if got.Reloaded.View != "mechanism" || got.Reloaded.ArtifactID != "mechanism-1" || got.Reloaded.StepIndex != 3 {
		t.Fatalf("reloaded state = %#v", got.Reloaded)
	}
	if got.Backed.View != "mechanism" || got.Backed.StepIndex != 0 {
		t.Fatalf("back state = %#v", got.Backed)
	}
	if got.DrawerHash != "#/mechanism/mechanism-1/step/4" || !got.DrawerHistory {
		t.Fatalf("drawer history = hash %q, state %t", got.DrawerHash, got.DrawerHistory)
	}
	if got.Closed.View != "mechanism" || got.Closed.StepIndex != 3 || string(got.Closed.SourceLocation) != "null" {
		t.Fatalf("closed drawer state = %#v", got.Closed)
	}
	if got.SearchHash != "#/overview" || got.SearchDrawer.View != "overview" ||
		got.SearchDrawer.SourceLocation == nil || got.SearchDrawerHasMechanism {
		t.Fatalf("Search fallback did not canonicalize to Overview: hash %q, state %#v, has mechanism %t",
			got.SearchHash, got.SearchDrawer, got.SearchDrawerHasMechanism)
	}
	if got.MapHash != "#/architecture?focus=component%3Arouter" ||
		got.MapReturnHash != "#/mechanism/mechanism-1/step/2" ||
		got.MapReturned.View != "mechanism" || got.MapReturned.StepIndex != 1 {
		t.Fatalf("explicit architecture return = map %q, return %q, state %#v",
			got.MapHash, got.MapReturnHash, got.MapReturned)
	}
	if got.ExplicitMapReturnBackCalls != 0 {
		t.Fatalf("explicit architecture return called history.back %d time(s)", got.ExplicitMapReturnBackCalls)
	}
}

func TestUserWorkspaceStaticSearchExposesOnlyCodeBackedLocations(t *testing.T) {
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
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
process.stdout.write(JSON.stringify({
  path: api.semanticSearchTargetAvailable({ kind: "location", location: { path: "router.go" } }),
  exact: api.semanticSearchTargetAvailable({ kind: "location", location: { path: "router.go", line: 41 } }),
  outsideSnippet: api.semanticSearchTargetAvailable({ kind: "location", location: { path: "router.go", line: 99 } }),
  noCode: api.semanticSearchTargetAvailable({ kind: "location", location: { path: "other.go" } }),
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

func TestUserWorkspaceCodeFirstAssetContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"function sourceSnippetHasCode(",
		"function renderSourceCode(",
		"function renderSourceSnippetCard(",
		"function sourceSnippetIdentity(",
		"function uniqueSourceSnippets(",
		"function remainingExactReferences(",
		"function renderExactReferences(",
		"'Primary implementation'",
		"'Open in editor'",
		"'Copy file:line'",
		"'Show code'",
		"'Show full function'",
		"'What to notice'",
		"'Open code path →'",
		"'All files ('",
		"if (!action.selection || !action.selection.snippet",
		"function parseWorkspaceHash(",
		"window.addEventListener('hashchange'",
		"window.addEventListener('popstate'",
		"'/step/' + (Number(state.stepIndex) + 1)",
		"function renderMobileStepControls(",
		"'rm-step-actions is-before'",
		"'rm-step-actions is-after'",
		"function mechanismNarrativeItems(",
		"function mechanismImplementationSteps(",
		"function narrativeIndexForImplementationStep(",
		"DATA.repository_thesis",
		"DATA.repository_guide",
		"function renderGuideReadNext(",
		"function userArchitectureAvailable(",
		"'Main code path'",
		"'Other code path'",
		"'Show implementation details ('",
		"function returnFromArchitecture(",
		"commitWorkspaceState(next, { replace: true })",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing code-first source token %q", token)
		}
	}
	for _, token := range []string{
		".rm-source-card",
		".rm-source-code__line.is-highlighted",
		".rm-source-code__gap",
		".rm-exact-references",
		".rm-source-notices",
		".rm-mobile-step-controls",
		".rm-system-story",
		".rm-repository-area-grid",
		".rm-mechanism-context",
		".rm-implementation-details",
		"position: sticky",
	} {
		if !strings.Contains(styleCSS, token) {
			t.Errorf("report CSS is missing code-first source token %q", token)
		}
	}
	for _, forbidden := range []string{
		"function renderCodeLocationButton(",
		"'Open a code path'",
		"'Open this code'",
		"'Open the implementation'",
	} {
		if strings.Contains(scriptJS, forbidden) {
			t.Errorf("report JS still contains obsolete path-only action %q", forbidden)
		}
	}
}
