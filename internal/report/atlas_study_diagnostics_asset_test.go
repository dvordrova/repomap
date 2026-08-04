package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAtlasStudyDiagnosticsAndSynthesisFailureRendering executes the real
// templates/script.js + templates/ui_messages.js in Node and verifies the D211
// HOLD-repair UI contract: the four-stage diagnostics panel renders the stage
// counts, the four independent flags (including false values), and the bounded
// omission aggregates; and the Overview/Architecture surfaces show an honest
// synthesis-failed notice instead of the unconditional acceptance copy.
func TestAtlasStudyDiagnosticsAndSynthesisFailureRendering(t *testing.T) {
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
  onclick = null;
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
const paths = [];
const directions = [1, 1].map((count, directionIndex) => {
  const readings = [];
  for (let readingIndex = 0; readingIndex < count; readingIndex++) {
    const path = "study/route-" + (directionIndex + 1) + ".go";
    const line = 10 + readingIndex;
    paths.push(path);
    readings.push({
      label: "Start here",
      symbol: "Read" + (directionIndex + 1),
      what_to_look_for: "Inspect exact reading " + (directionIndex + 1) + ".",
      location: { path, line },
      source: snippet(path, "Read" + (directionIndex + 1), line),
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
  navigator: { version: 1, state: "empty" },
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
  study_publication: { version: 1, state: "accepted" },
  study_map: {
    brief: {
      what_it_is: "A saved repository brief.", problem: "It solves a bounded problem.",
      main_input: "Input", central_responsibility: "Responsibility", observable_result: "Result",
      domain_terms: [{ term: "Fixture term", meaning: "Fixture meaning" }],
    },
    shape: [], directions,
  },
  atlas_study: {
    version: 8, projection_version: 6, state: "accepted",
    direction_count: 2,
    considered_span_count: 68,
    advertised_span_count: 32,
    model_selected_span_count: 10,
    accepted_span_count: 10,
    frontier_complete: false,
    selected_items_complete: true,
    support_coverage_complete: true,
    portfolio_target_met: true,
    omissions: [{ reason: "advertised_budget", count: 36, representative_count: 12 }],
  },
  architecture_synthesis: { state: "failed" },
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
      { id: "component-remainder", name: "Not classified by model", members: [] },
    ],
    behavior_anchors: [], surfaces: [], flows: [],
  },
};
const roots = {};
["rm-overview", "rm-task-investigation", "rm-mechanisms", "rm-mechanism-detail",
 "rm-study-overview", "rm-study-detail", "rm-operate-detail", "rm-architecture", "rm-provenance"].forEach((id) => {
  roots[id] = new Element("section");
  roots[id].className = "rm-tab-content" + (id === "rm-overview" ? " rm-active" : "");
});
roots["rm-tabs"] = new Element("nav");
roots["rm-source-drawer"] = new Element("aside");
roots["rm-source-drawer"].hidden = true;
roots["rm-source-drawer-content"] = new Element("div");
roots["rm-source-drawer-close"] = new Element("button");
const workspace = new Element("main");
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  history: {
    state: null,
    pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
    replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
    back() {},
  },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
};
window.RepomapArchitectureCanvas = {
  mount() { return { ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {} }; },
};
const tabSections = Object.values(roots).filter((node) => String(node.className).split(/\s+/).includes("rm-tab-content"));
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
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
api.renderOverviewWorkspace();
const overviewText = text(roots["rm-overview"]);
const nav = roots["rm-tabs"].children.slice();
const studyTab = nav.find((node) => node.attributes["data-workspace-view"] === "study_overview");
studyTab.onclick();
const studyOverviewText = text(roots["rm-study-overview"]);
const diagnosticsPanel = byClass(roots["rm-study-overview"], "rm-study-diagnostics")[0];
const stageCounts = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-stage").map((node) => text(node));
const flagItems = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-flag").map((node) => text(node));
const omissionItems = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-omission").map((node) => text(node));
const architectureTab = nav.find((node) => node.attributes["data-workspace-view"] === "architecture");
architectureTab.onclick();
const architectureText = text(roots["rm-architecture"]);
process.stdout.write(JSON.stringify({ overviewText, studyOverviewText, stageCounts, flagItems, omissionItems, architectureText }));
`
	runnerPath := filepath.Join(t.TempDir(), "atlas-study-diagnostics-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run diagnostics workspace: %v\n%s", err, output)
	}
	var got struct {
		OverviewText      string   `json:"overviewText"`
		StudyOverviewText string   `json:"studyOverviewText"`
		StageCounts       []string `json:"stageCounts"`
		FlagItems         []string `json:"flagItems"`
		OmissionItems     []string `json:"omissionItems"`
		ArchitectureText  string   `json:"architectureText"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode diagnostics workspace: %v\n%s", err, output)
	}

	// Four-stage diagnostics: stage counts present with exact values.
	for _, want := range []string{
		"Study diagnostics", "Considered spans", "68",
		"Advertised spans", "32",
		"Model-selected spans", "10",
		"Locally accepted spans", "10",
	} {
		if !strings.Contains(got.StudyOverviewText, want) {
			t.Fatalf("Study diagnostics missing %q:\n%s", want, got.StudyOverviewText)
		}
	}
	// The four independent flags render with exact true/false presentation.
	wantFlagText := strings.Join([]string{
		"Frontier completeNo", "Selected items completeYes",
		"Support coverage completeYes", "Portfolio target metYes",
	}, "|")
	if strings.Join(got.FlagItems, "|") != wantFlagText {
		t.Fatalf("flag presentation = %#v, want %q", got.FlagItems, wantFlagText)
	}
	// Bounded omission aggregates render by closed reason.
	if len(got.OmissionItems) != 1 || !strings.Contains(got.OmissionItems[0], "advertised_budget") ||
		!strings.Contains(got.OmissionItems[0], "Omitted: 36") ||
		!strings.Contains(got.OmissionItems[0], "Representative refs: 12") {
		t.Fatalf("omission aggregates = %#v", got.OmissionItems)
	}
	// Honest synthesis-failed copy replaces the unconditional acceptance copy.
	if !strings.Contains(got.OverviewText, "Architecture synthesis failed; showing the locally available architecture with exact symbol sources.") {
		t.Fatalf("Overview does not show the synthesis-failed copy:\n%s", got.OverviewText)
	}
	if strings.Contains(got.OverviewText, "Accepted conceptual components open on the map") {
		t.Fatalf("Overview still shows the unconditional acceptance copy:\n%s", got.OverviewText)
	}
	if !strings.Contains(got.ArchitectureText, "Architecture synthesis failed; showing the locally available architecture.") {
		t.Fatalf("Architecture tab does not show the synthesis-failed notice:\n%s", got.ArchitectureText)
	}
}
