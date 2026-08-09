package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Decision 233 (Archive 9, PHASE 4 AREA COVERAGE): the Study diagnostics
// panel renders an EXACT missing-core-area diagnostic — accepted principal
// Architecture components whose member paths have no published theme reading
// — with the exact count and bounded names. Never filler.
func TestAtlasStudyMissingCoreAreaDiagnosticRenders(t *testing.T) {
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
      add: (name) => { if (!this.className.split(/\s+/).includes(name)) this.className = (this.className + " " + name).trim(); },
      remove: (name) => { this.className = this.className.split(/\s+/).filter((value) => value && value !== name).join(" "); },
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
  contains(node) { return walk(this).includes(node); }
  focus() { this.focused = true; }
  onclick = null;
  scrollIntoView() {}
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
const report = {
  repo_name: "fixture", report_language: "en",
  user_sources: [], user_mechanisms: [], user_topics: [], openable_paths: ["core/run.go", "core/store.go"],
  github_source_links: { repository_url: "https://github.com/example/fixture", revision: "a".repeat(40) },
  atlas_study: {
    version: 1, projection_version: 12, state: "accepted_partial",
    considered_span_count: 4, advertised_span_count: 4, model_selected_span_count: 2, accepted_span_count: 2,
    frontier_complete: false, selected_items_complete: false, support_coverage_complete: true, portfolio_target_met: true,
    missing_core_area_count: 2,
    missing_core_areas: ["Storage engine", "Replication"],
    themes: { total: 1, shown: 1, cards: [{
      ordinal: 1, final_title: "Startup path", final_question: "How does startup work?",
      why_it_matters: "reason", expected_learning: "outcome", theme_kind: "user_journey",
      badge: "editorial_source_backed",
      readings: [{ label: "Run", symbol: "Run", path: "core/run.go", line: 10, role: "direct", supported_observation: "inspect" }],
    }] },
    frontier_browse: { total: 4, shown: 4, spans: [
      { ordinal: 1, title: "Run", question: "Startup?", stage: "published", source: { path: "core/run.go", line: 10 } },
      { ordinal: 2, title: "Store", question: "Storage?", stage: "scout_anchored", source: { path: "core/store.go", line: 20 } },
      { ordinal: 3, title: "Repl", question: "Replication?", stage: "seed_advertised", source: { path: "core/repl.go", line: 30 } },
      { ordinal: 4, title: "Snap", question: "Snapshot?", stage: "considered", source: { path: "core/snap.go", line: 40 } },
    ] },
  },
  architecture_canvas: {
    version: 10, repository_archetype: "application", validation_outcome: "accepted",
    architecture_source: "validated_model", architecture_level: 2, title: "Fixture",
    components: [], behavior_anchors: [], surfaces: [], flows: [],
  },
};
function journey(report, lang) {
  report = Object.assign({}, report, { report_language: lang });
  const roots = {};
  ["rm-overview", "rm-study-overview", "rm-architecture"].forEach((id) => {
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
    history: { state: null, pushState(state, _, hash) { this.state = state; window.location.hash = hash; }, replaceState(state, _, hash) { this.state = state; window.location.hash = hash; }, back() {} },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
  };
  window.RepomapArchitectureCanvas = {
    mount() { return { ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {} }; },
  };
  const document = {
    createElement(tag) { return new Element(tag); },
    createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
    getElementById(id) {
      if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
      return roots[id] || null;
    },
    querySelector(selector) {
      if (selector === ".rm-workspace") return workspace;
      if (selector.startsWith(".")) {
        const wanted = selector.slice(1);
        for (const root of Object.values(roots)) {
          const found = walk(root).find((node) => String(node.className).split(/\s+/).includes(wanted));
          if (found) return found;
        }
      }
      return null;
    },
    querySelectorAll(selector) {
      if (selector === ".rm-main-content > .rm-tab-content") return Object.values(roots);
      return [];
    },
  };
  document.documentElement = { lang: lang };
  window.document = document;
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
    window, document, URLSearchParams, Set, Map, AbortController, Promise,
  });
  const api = window.__REPOMAP_WORKSPACE_TEST__;
  api.renderWorkspaceTabs();
  api.renderMapSummaryInto("rm-overview");
  const studyTab = roots["rm-tabs"].children.find((node) => node.attributes["data-workspace-view"] === "study_overview");
  studyTab.onclick();
  const missingItems = byClass(roots["rm-study-overview"], "rm-study-diagnostics-missing-core-item").map((node) => text(node));
  const missingHeading = byClass(roots["rm-study-overview"], "rm-study-diagnostics-subheading").map((node) => text(node)).filter((t) => t.includes("Core areas") || t.includes("Ключевые области"));
  const overviewText = text(roots["rm-study-overview"]);
  return { missingItems, missingHeading, overviewText };
}
const en = journey(report, "en");
const ru = journey(Object.assign({}, report, { report_language: "ru" }), "ru");
process.stdout.write(JSON.stringify({ en, ru }));
`
	runnerPath := filepath.Join(t.TempDir(), "missing-core-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run missing-core workspace: %v\n%s", err, output)
	}
	var out struct {
		En struct {
			MissingItems   []string `json:"missingItems"`
			MissingHeading []string `json:"missingHeading"`
			OverviewText   string   `json:"overviewText"`
		} `json:"en"`
		Ru struct {
			MissingItems   []string `json:"missingItems"`
			MissingHeading []string `json:"missingHeading"`
			OverviewText   string   `json:"overviewText"`
		} `json:"ru"`
	}
	if err := json.Unmarshal(output, &out); err != nil {
		t.Fatalf("decode missing-core workspace: %v\n%s", err, output)
	}
	if len(out.En.MissingItems) != 2 ||
		!strings.Contains(out.En.OverviewText, "Storage engine") ||
		!strings.Contains(out.En.OverviewText, "Replication") ||
		!strings.Contains(out.En.OverviewText, "Core areas without a published theme (2)") {
		t.Fatalf("EN missing-core diagnostic = %#v\n%s", out.En.MissingItems, out.En.OverviewText)
	}
	if len(out.Ru.MissingItems) != 2 ||
		!strings.Contains(out.Ru.OverviewText, "Storage engine") ||
		!strings.Contains(out.Ru.OverviewText, "Ключевые области без опубликованной темы (2)") {
		t.Fatalf("RU missing-core diagnostic = %#v\n%s", out.Ru.MissingItems, out.Ru.OverviewText)
	}
}
