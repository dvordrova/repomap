package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtlasFirstWorkspacePublishesArchitectureAndStudy(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	const runner = `
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
    this.parentNode = null;
    this.classList = {
      add: (name) => {
        if (!this.className.split(/\s+/).includes(name)) this.className = (this.className + " " + name).trim();
      },
      remove: (name) => {
        this.className = this.className.split(/\s+/).filter((value) => value && value !== name).join(" ");
      },
      toggle: (name, force) => { if (force) this.classList.add(name); else this.classList.remove(name); },
    };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] || null; }
  removeAttribute(name) { delete this.attributes[name]; }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { child.parentNode = this; this.children.unshift(child); }
  replaceChildren(...children) { this.children = []; this.append(...children); }
  remove() {
    if (!this.parentNode) return;
    this.parentNode.children = this.parentNode.children.filter((child) => child !== this);
    this.parentNode = null;
  }
  querySelector(selector) {
    if (!selector.startsWith(".")) return null;
    const className = selector.slice(1);
    return walk(this).find((node) => node !== this && String(node.className).split(/\s+/).includes(className)) || null;
  }
  querySelectorAll(selector) {
    if (!selector.startsWith(".")) return [];
    const className = selector.slice(1);
    return walk(this).filter((node) => node !== this && String(node.className).split(/\s+/).includes(className));
  }
  addEventListener() {}
  contains(node) { return this === node || (this.children || []).includes(node); }
  focus() { this.focused = true; }
}
function walk(root) {
  const result = [];
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(root) { return walk(root).map((node) => String(node.textContent || "")).join(""); }
function byClass(root, className) {
  return walk(root).filter((node) => String(node.className).split(/\s+/).includes(className));
}
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 2,
    content_sha256: "a".repeat(64), presentation_sha256: ("" + line).padStart(64, "0"),
    lines: [
      { line, text: "func " + symbol + "() {", highlight: true },
      { line: line + 1, text: "  inspect()" },
      { line: line + 2, text: "}" },
    ],
  };
}
const readingCounts = [1, 4, 1, 2, 1, 1];
const paths = [];
const directions = readingCounts.map((count, directionIndex) => {
  const readings = [];
  for (let readingIndex = 0; readingIndex < count; readingIndex++) {
    const path = "study/route-" + (directionIndex + 1) + "-" + (readingIndex + 1) + ".go";
    const line = 10 + readingIndex;
    paths.push(path);
    readings.push({
      label: readingIndex === 0 ? "Start here" : "Then inspect",
      symbol: "Read" + (directionIndex + 1) + "_" + (readingIndex + 1),
      what_to_look_for: "Inspect exact reading " + (readingIndex + 1) + ".",
      location: { path, line },
      source: snippet(path, "Read" + (directionIndex + 1) + "_" + (readingIndex + 1), line),
    });
  }
  return {
    id: "study-route-" + (directionIndex + 1),
    question: "Study question " + (directionIndex + 1) + "?",
    why_it_matters: "Reason " + (directionIndex + 1) + ".",
    learning_outcome: "Outcome " + (directionIndex + 1) + ".",
    principal_anchors: [{
      path: readings[0].location.path,
      symbol: readings[0].source.enclosing_symbol,
      line: readings[0].location.line,
    }],
    reading_anchors: readings,
  };
});
const report = {
  repo_name: "fixture", report_language: "en", user_mechanisms: [], user_topics: [], user_sources: [],
  openable_paths: paths, source_ids: {},
  github_source_links: {
    repository_url: "https://github.com/example/fixture",
    revision: "1".repeat(40),
  },
  repository_atlas: {
    version: 1,
    units: [
      { id: "repo", kind: "repository", name: "fixture" },
      { id: "module", kind: "module", parent_id: "repo", name: "fixture" },
      { id: "app", kind: "app", parent_id: "repo", name: "fixture" },
      { id: "package", kind: "package", parent_id: "module", name: "fixture/internal" },
    ],
    entities: [], evidence: [], relations: [],
  },
  // A stale legacy failure must not contradict the independently accepted
  // Atlas Study product rendered from study_map.
  study_publication: { version: 1, state: "failed" },
  study_map: {
    brief: {
      what_it_is: "A saved repository brief.", problem: "It solves a bounded problem.",
      main_input: "Input", central_responsibility: "Responsibility", observable_result: "Result",
	  domain_terms: [{ term: "Fixture term", meaning: "Fixture meaning" }],
    },
    shape: [], directions,
  },
  architecture_canvas: {
    version: 8, validation_outcome: "accepted_partial", architecture_source: "partial_model",
    local_remainder_component_id: "component-remainder",
    title: "Saved Architecture", subtitle: "Exact saved conceptual grouping.",
    subsystems: [
      { id: "subsystem-runtime", name: "Runtime" },
      { id: "subsystem-security", name: "Security" },
    ],
    components: [
      { id: "component-runtime", name: "Runtime component", subsystem_id: "subsystem-runtime", members: [] },
      { id: "component-security", name: "Security component", subsystem_id: "subsystem-security", members: [] },
      { id: "component-remainder", name: "Not classified by model", members: [
        { id: { kind: "symbol", value: "remainder-one" }, name: "Local remainder one" },
        { id: { kind: "symbol", value: "remainder-two" }, name: "Local remainder two" },
      ] },
    ],
    behavior_anchors: [], surfaces: [], flows: [],
  },
};
const roots = {};
[
  "rm-task-investigation", "rm-operate-detail", "rm-architecture", "rm-provenance",
].forEach((id) => {
  roots[id] = new Element("section");
  roots[id].className = "rm-tab-content" + (id === "rm-architecture" ? " rm-active" : "");
});
roots["rm-tabs"] = new Element("nav");
roots["rm-source-drawer"] = new Element("aside");
roots["rm-source-drawer"].hidden = true;
roots["rm-source-drawer-content"] = new Element("div");
roots["rm-source-drawer-close"] = new Element("button");
const workspace = new Element("main");
const window = {
  location: { search: "", hash: "#canvas", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  history: {
    state: null,
    pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
    replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
    back() {},
  },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
};
const mounts = [];
window.RepomapArchitectureCanvas = {
  mount(host, data) {
    mounts.push(JSON.parse(JSON.stringify(data)));
    return {
      ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {},
      openFlowStep() {}, openSurface() {},
    };
  },
};
const tabSections = Object.values(roots).filter((node) => String(node.className).split(/\s+/).includes("rm-tab-content"));
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    if (roots[id]) return roots[id];
    return Object.values(roots).flatMap((root) => walk(root)).find((node) => node.id === id) || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll(selector) {
    if (selector === ".rm-main-content > .rm-tab-content") return tabSections;
    if (selector === "[data-workspace-view]") {
      return walk(roots["rm-tabs"]).filter((node) => node.attributes && node.attributes["data-workspace-view"]);
    }
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
api.renderWorkspaceTabs();
api.renderArchitectureWorkspace();
api.mountArchitectureCanvas();
const nav = byClass(roots["rm-tabs"], "rm-target-link");
const navLabels = nav.map((node) => text(node));
const navViews = nav.map((node) => node.attributes["data-workspace-view"]);
const navHrefs = nav.map((node) => node.attributes.href || "");
const navCurrents = nav.map((node) => node.attributes["aria-current"] || "");
const overviewText = "";
const studyRoot = byClass(roots["rm-architecture"], "rm-target-study")[0];
const studyOverviewText = text(studyRoot);
const directionCards = byClass(studyRoot, "rm-study-direction-card");
const routeResults = [];
directionCards.forEach((directionCard, directionIndex) => {
  const directionTitle = byClass(directionCard, "rm-study-direction-card__title")[0];
  const directionSources = byClass(directionCard, "rm-study-direction-card__source");
  const directionExactClicks = directionSources.map((action, readingIndex) => {
    // Decision 222: source actions are GitHub/GitLab jumps (new tab), never
    // an inline code drawer and never a local state mutation.
    return {
      href: action.attributes && action.attributes.href || "",
      expectedPath: directions[directionIndex].reading_anchors[readingIndex].location.path,
      expectedLine: directions[directionIndex].reading_anchors[readingIndex].location.line,
    };
  });
  const detail = directionCard;
  const readingCards = byClass(detail, "rm-study-reading-anchor");
  const readingActions = byClass(detail, "rm-study-reading-anchor__open");
  const exactClicks = [];
  readingActions.forEach((action, readingIndex) => {
    exactClicks.push({
      href: action.attributes && action.attributes.href || "",
      drawerHidden: !roots["rm-source-drawer"] || roots["rm-source-drawer"].hidden,
      expectedPath: directions[directionIndex].reading_anchors[readingIndex].location.path,
      expectedLine: directions[directionIndex].reading_anchors[readingIndex].location.line,
    });
  });
  routeResults.push({
    id: directions[directionIndex].id,
    detailText: text(detail),
    readingCards: readingCards.length,
    readingActions: readingActions.length,
    directionCardTag: String(directionCard && directionCard.tagName || "").toLowerCase(),
    summaryCount: walk(directionCard).filter((node) => String(node.tagName || "").toLowerCase() === "summary").length,
    directionTitleTag: String(directionTitle && directionTitle.tagName || "").toLowerCase(),
    directionSources: directionSources.length,
    directionExactClicks,
    exactClicks,
  });
});
process.stdout.write(JSON.stringify({
  navLabels, navViews, navHrefs, navCurrents, overviewText, studyOverviewText,
  directionCards: directionCards.length, routeResults,
  architectureText: text(roots["rm-architecture"]),
  architectureCurrent: nav[0] && nav[0].attributes["aria-current"] || "",
  architectureHash: window.location.hash,
  mounts,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "atlas-first-products-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Atlas-first product workspace: %v\n%s", err, output)
	}
	var got struct {
		NavLabels         []string `json:"navLabels"`
		NavViews          []string `json:"navViews"`
		NavHrefs          []string `json:"navHrefs"`
		NavCurrents       []string `json:"navCurrents"`
		OverviewText      string   `json:"overviewText"`
		StudyOverviewText string   `json:"studyOverviewText"`
		DirectionCards    int      `json:"directionCards"`
		RouteResults      []struct {
			ID                   string `json:"id"`
			DetailText           string `json:"detailText"`
			ReadingCards         int    `json:"readingCards"`
			ReadingActions       int    `json:"readingActions"`
			DirectionCardTag     string `json:"directionCardTag"`
			SummaryCount         int    `json:"summaryCount"`
			DirectionTitleTag    string `json:"directionTitleTag"`
			DirectionSources     int    `json:"directionSources"`
			DirectionExactClicks []struct {
				Href         string `json:"href"`
				ExpectedPath string `json:"expectedPath"`
				ExpectedLine int    `json:"expectedLine"`
			} `json:"directionExactClicks"`
			ExactClicks []struct {
				Href         string `json:"href"`
				DrawerHidden bool   `json:"drawerHidden"`
				ExpectedPath string `json:"expectedPath"`
				ExpectedLine int    `json:"expectedLine"`
			} `json:"exactClicks"`
		} `json:"routeResults"`
		ArchitectureText    string `json:"architectureText"`
		ArchitectureCurrent string `json:"architectureCurrent"`
		ArchitectureHash    string `json:"architectureHash"`
		Mounts              []struct {
			ValidationOutcome         string `json:"validation_outcome"`
			LocalRemainderComponentID string `json:"local_remainder_component_id"`
			Subsystems                []any  `json:"subsystems"`
			Components                []struct {
				ID      string `json:"id"`
				Members []any  `json:"members"`
			} `json:"components"`
		} `json:"mounts"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Atlas-first product workspace: %v\n%s", err, output)
	}
	if len(got.NavLabels) != 1 || !strings.Contains(got.NavLabels[0], "fixture") ||
		strings.Join(got.NavViews, ",") != "map" || strings.Join(got.NavHrefs, ",") != "#canvas" ||
		strings.Join(got.NavCurrents, ",") != "page" {
		t.Fatalf("Atlas-first product navigation = labels %#v views %#v", got.NavLabels, got.NavViews)
	}
	if got.DirectionCards != 6 || len(got.RouteResults) != 6 ||
		strings.Contains(got.StudyOverviewText, "Repository brief") ||
		strings.Contains(got.StudyOverviewText, "A saved repository brief.") ||
		strings.Contains(got.StudyOverviewText, "Study unavailable for this run") {
		t.Fatalf("Atlas-first Study overview = directions %d routes %d text %q",
			got.DirectionCards, len(got.RouteResults), got.StudyOverviewText)
	}
	wantReadings := []int{1, 4, 1, 2, 1, 1}
	for index, route := range got.RouteResults {
		if route.ReadingCards != 0 || route.ReadingActions != 0 ||
			route.DirectionCardTag != "details" || route.SummaryCount != 1 ||
			route.DirectionTitleTag != "span" || route.DirectionSources != wantReadings[index] ||
			len(route.DirectionExactClicks) != wantReadings[index] ||
			len(route.ExactClicks) != 0 ||
			!strings.Contains(route.DetailText, "Study question "+string(rune('1'+index))+"?") {
			t.Fatalf("Study route %d = %#v", index+1, route)
		}
		for _, click := range route.DirectionExactClicks {
			if !strings.Contains(click.Href, "github.com/example/fixture/blob") ||
				!strings.Contains(click.Href, click.ExpectedPath) {
				t.Fatalf("Study card exact source href = %#v", click)
			}
		}
		for _, click := range route.ExactClicks {
			if !click.DrawerHidden ||
				!strings.Contains(click.Href, "github.com/example/fixture/blob") ||
				!strings.Contains(click.Href, click.ExpectedPath) {
				t.Fatalf("Study exact source href = %#v", click)
			}
		}
	}
	if len(got.Mounts) != 1 || got.ArchitectureCurrent != "page" ||
		got.ArchitectureHash != "#canvas" ||
		!strings.Contains(got.ArchitectureText, "Components") {
		t.Fatalf("Atlas-first Architecture route = mounts %d current %q hash %q text %q",
			len(got.Mounts), got.ArchitectureCurrent, got.ArchitectureHash, got.ArchitectureText)
	}
	mount := got.Mounts[0]
	if mount.ValidationOutcome != "accepted_partial" ||
		mount.LocalRemainderComponentID != "component-remainder" ||
		len(mount.Subsystems) != 2 || len(mount.Components) != 3 ||
		mount.Components[2].ID != "component-remainder" || len(mount.Components[2].Members) != 2 {
		t.Fatalf("Atlas-first Architecture exact partial payload = %#v", mount)
	}
}
