package report

// Decision 242 DOM acceptance: the real pure Canvas projection and workspace
// renderer consume plural MechanismFragment v3 objects from the Canvas. Each
// fragment is a compact first-hop fan-out, never a fabricated path; exact
// component participation drives emphasis and the frontier stays explicit.
import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMechanismFragmentAssetRendersPluralFirstHopFanout(t *testing.T) {
	if MechanismFragmentVersion != 3 {
		t.Fatalf("MechanismFragmentVersion = %d, fixture requires current v3", MechanismFragmentVersion)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset := filepath.Join("templates", "script.js")
	runner := `
const fs = require("fs");
const vm = require("vm");
function Element(tag) {
  this.tagName = String(tag).toUpperCase(); this._text = ""; this.children = [];
  this.attributes = {}; this.hidden = false; this.className = "";
  const self = this;
  this.classList = {
    add(value) { const parts = String(self.className || "").split(/\s+/).filter(Boolean); if (!parts.includes(value)) parts.push(value); self.className = parts.join(" "); },
    remove(value) { self.className = String(self.className || "").split(/\s+/).filter((part) => part && part !== value).join(" "); },
    toggle(value, active) { if (active) this.add(value); else this.remove(value); },
  };
}
Object.defineProperty(Element.prototype, "childNodes", { get() { return this.children; } });
Object.defineProperty(Element.prototype, "textContent", {
  get() { return this._text + this.children.map((child) => child.textContent).join(""); },
  set(value) { this._text = value == null ? "" : String(value); },
});
Element.prototype.setAttribute = function (name, value) { this.attributes[name] = String(value); if (name === "id") this.id = String(value); };
Element.prototype.getAttribute = function (name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; };
Element.prototype.removeAttribute = function (name) { delete this.attributes[name]; };
Element.prototype.appendChild = function (child) { child.parentNode = this; this.children.push(child); return child; };
Element.prototype.append = function (...children) { children.forEach((child) => this.appendChild(child)); };
Element.prototype.replaceChildren = function (...children) { this.children = []; children.forEach((child) => this.appendChild(child)); };
Element.prototype.remove = function () { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); };
Element.prototype.prepend = function (...children) { children.reverse().forEach((child) => { child.parentNode = this; this.children.unshift(child); }); };
function walk(node, out) { out = out || []; (node.children || []).forEach((child) => { out.push(child); walk(child, out); }); return out; }
function matches(node, selector) {
  if (selector[0] === ".") return String(node.className || "").split(/\s+/).includes(selector.slice(1));
  const attr = selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
  if (attr) return Object.prototype.hasOwnProperty.call(node.attributes || {}, attr[1]) && (attr[2] == null || node.attributes[attr[1]] === attr[2]);
  return false;
}
Element.prototype.querySelector = function (selector) { return walk(this).find((node) => matches(node, selector)) || null; };
Element.prototype.querySelectorAll = function (selector) { return walk(this).filter((node) => matches(node, selector)); };
const roots = {
  "rm-overview": new Element("div"), "rm-architecture": new Element("section"),
  "rm-study-overview": new Element("div"), "rm-study-detail": new Element("div"),
  "rm-tabs": new Element("div"),
};
const workspace = new Element("div");
const limitation = "Exact repository-local direct static call from a build-selected production process entry; runtime order, successful execution, ownership, and transitive reachability are not observed.";
function transition(label, path, line, target) {
  return {
    claim_kind: "direct_static_call", support_mode: "resolved_static", label,
    path, line, symbol: target && target.label || "", evidence: "go_ssa surface-ssa-v12 connect_architecture_anchors",
    scenario: "go:linux", limitation, ordering: "resolved_path_order", target,
  };
}
function fragment(id, entrySymbol, entryPath, componentIDs, handoff) {
  return {
    version: 3, id, component_ids: componentIDs,
    entry: {
      claim_kind: "process_entry", support_mode: "resolved_static",
      label: entrySymbol ? "process entry " + entrySymbol : "process entry", symbol: entrySymbol, path: entryPath, line: 10,
      evidence: "behavior anchor proof_mode process_entry", scenario: "go:linux",
      limitation: "process entry identity only; runtime reachability not proven", ordering: "exact_local_order",
    },
    handoffs: [handoff],
    frontier: {
      ordering: "not_established", unresolved: ["continuation beyond the first-hop handoffs"],
      limitation: "No further transition is locally proven to continue from these first-hop handoffs; execution order beyond them is not established.",
    },
  };
}
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en",
  github_source_links: { repository_url: "https://github.com/acme/fixture", revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
  user_mechanisms: [],
  // A different embedded source makes this a mixed-snippet report. Exact
  // mechanism locations must still fall back to pinned host actions.
  user_sources: [{ path: "other.go", start_line: 1, end_line: 1, lines: ["package fixture"] }],
  user_topics: [],
  openable_paths: ["client.go", "main.go", "other.go", "service.go", "worker.go"], source_ids: {},
  architecture_canvas: {
    version: 13, local_remainder_component_id: "component-r",
    components: [{ id: "c1", name: "Entry" }, { id: "c2", name: "Service" }, { id: "c3", name: "Client" }],
    subsystems: [], behavior_anchors: [], surfaces: [], flows: [],
    // Adversarial flow data cannot become Mechanisms authority.
    flow_edges: [{ flow_id: "legacy-flow", from_component_id: "c2", to_component_id: "c3" }],
    structural_edges: [],
    mechanism_fragments: [
      fragment("fragment-a", "fixture.main", "main.go", ["c1", "c2"],
        transition("handoff to service.Start", "main.go", 15, { label: "service.Start", path: "service.go", line: 20 })),
      fragment("fragment-b", "", "worker.go", ["c3"],
        transition("handoff to client.Send", "worker.go", 25, { label: "client.Send", path: "client.go", line: 30 })),
    ],
  },
  repository_atlas: { version: 1, units: [], entities: [], observations: [], evidence: [], relations: [] },
};
const window = {
  location: { hash: "#/architecture", host: "fixture.test", pathname: "/index.html", search: "" },
  history: { state: null, pushState(state, _, hash) { this.state = state; window.location.hash = hash; }, replaceState(state, _, hash) { this.state = state; window.location.hash = hash; }, back() {} },
  __REPOMAP_WORKSPACE_TEST__: {}, __REPOMAP_LAYOUT_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    if (roots[id]) return roots[id];
    return Object.values(roots).flatMap((root) => [root].concat(walk(root))).find((node) => node.id === id) || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: report.report_language };
window.document = document;
const context = { window, document, Element, URLSearchParams, Set, Map, AbortController, Promise };
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), context);
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "architecture_canvas.js"), "utf8"), context);
const realCanvas = window.RepomapArchitectureCanvas;
window.RepomapArchitectureCanvas = {
  projectArchitectureLens: realCanvas.projectArchitectureLens,
  mount() { return { ready: Promise.resolve(), destroy() {}, openComponent() {}, setLens() {} }; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), context);
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.renderWorkspaceTabs();
const nav = roots["rm-tabs"].children.slice();
const mapTab = nav.find((node) => node.attributes["data-workspace-view"] === "map");
if (!mapTab) throw new Error("map tab missing");
mapTab.onclick();
const root = roots["rm-architecture"];
const mechanismLens = walk(root).find((node) => node.attributes["data-map-lens"] === "mechanisms");
if (!mechanismLens) throw new Error("mechanisms lens missing");
mechanismLens.onclick();
const projection = realCanvas.projectArchitectureLens(report, "mechanisms");
const items = root.querySelectorAll(".rm-mechanism-fragment__lane");
const details = root.querySelectorAll(".rm-map-lens-object--mechanism-fragment");
const fanouts = root.querySelectorAll(".rm-mechanism-fragment__fanout");
const targets = root.querySelectorAll(".rm-mechanism-fragment__target");
const frontiers = root.querySelectorAll(".rm-mechanism-fragment__frontier");
const empty = root.querySelectorAll(".rm-map-lens-object--empty");
const legacy = root.querySelectorAll(".rm-architecture-mechanism-disclosure");
const sourceActions = root.querySelectorAll(".rm-source-action-link");
const labels = items.map((item) => { const node = item.querySelector(".rm-mechanism-fragment__label"); return node ? node.textContent : ""; });
const orderings = items.map((item) => { const node = item.querySelector(".rm-mechanism-fragment__ordering"); return node ? node.textContent : ""; });
const limitations = items.map((item) => { const node = item.querySelector(".rm-mechanism-fragment__limitation"); return node ? node.textContent : ""; });
process.stdout.write(JSON.stringify({
  projectionCount: projection.mechanisms.length, emphasized: projection.emphasized,
  detailCount: details.length, detailsOpen: details.map((item) => item.open === true),
  itemCount: items.length, fanoutCount: fanouts.length, targetCount: targets.length,
  frontierCount: frontiers.length, emptyCount: empty.length, legacyCount: legacy.length,
  sourceActionCount: sourceActions.length,
  sourceHrefs: sourceActions.map((item) => item.getAttribute("href") || ""),
  labels, orderings, limitations, text: root.textContent.replace(/\s+/g, " ").trim(),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "mechanism-fragment-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(language string) mechanismAssetResult {
		t.Helper()
		output, err := exec.Command(node, runnerPath, asset, language).CombinedOutput()
		if err != nil {
			t.Fatalf("run mechanism fragment workspace (%s): %v\n%s", language, err, output)
		}
		var got mechanismAssetResult
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode mechanism fragment workspace (%s): %v\n%s", language, err, output)
		}
		return got
	}

	en := run("en")
	if en.ProjectionCount != 2 || strings.Join(en.Emphasized, ",") != "c1,c2,c3" {
		t.Fatalf("projection = count %d emphasized %#v, want two exact fragments and c1,c2,c3", en.ProjectionCount, en.Emphasized)
	}
	if en.DetailCount != 2 || en.FanoutCount != 2 || en.TargetCount != 2 || en.FrontierCount != 2 || en.ItemCount != 4 {
		t.Fatalf("v3 DOM shape = details=%d fanouts=%d targets=%d frontiers=%d lanes=%d",
			en.DetailCount, en.FanoutCount, en.TargetCount, en.FrontierCount, en.ItemCount)
	}
	if en.EmptyCount != 0 || en.LegacyCount != 0 || len(en.DetailsOpen) != 2 || en.DetailsOpen[0] || en.DetailsOpen[1] {
		t.Fatalf("Map-first disclosure = empty %d legacy %d open %#v", en.EmptyCount, en.LegacyCount, en.DetailsOpen)
	}
	if en.SourceActionCount != 6 {
		t.Fatalf("mixed-snippet mechanism source actions = %d, want 6 exact entry/callsite/target actions: %#v", en.SourceActionCount, en.SourceHrefs)
	}
	for _, href := range en.SourceHrefs {
		if !strings.Contains(href, "https://github.com/acme/fixture/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/") {
			t.Fatalf("mechanism source action is not pinned to the captured revision: %q", href)
		}
	}
	if !strings.Contains(en.Text, "Exact first-hop static calls from this entry") || !strings.Contains(en.Text, "Entry handoff · service.Start") ||
		!strings.Contains(en.Text, "Continuation beyond these first-hop calls is not established") {
		t.Fatalf("English mechanism copy incomplete: %s", en.Text)
	}
	if len(en.Orderings) != 4 || en.Orderings[0] != "Exact source identity" ||
		en.Orderings[1] != "Canonical callsite display order; not runtime order" {
		t.Fatalf("ordering copy = %#v", en.Orderings)
	}
	for _, limitation := range en.Limitations {
		if limitation == "" {
			t.Fatalf("primary limitation missing: %#v", en.Limitations)
		}
	}

	ru := run("ru")
	for _, leaked := range []string{"process entry", "handoff to", "continuation beyond", "Exact repository-local", "No further transition"} {
		if strings.Contains(ru.Text, leaked) {
			t.Fatalf("English persisted mechanism copy leaked into RU HTML (%q): %s", leaked, ru.Text)
		}
	}
	if !strings.Contains(ru.Text, "Точные статические вызовы первого уровня из этой точки входа") ||
		!strings.Contains(ru.Text, "Вызов из точки входа · service.Start") ||
		!strings.Contains(ru.Text, "Продолжение после этих вызовов первого уровня не установлено") {
		t.Fatalf("Russian mechanism copy incomplete: %s", ru.Text)
	}
}

type mechanismAssetResult struct {
	ProjectionCount   int      `json:"projectionCount"`
	Emphasized        []string `json:"emphasized"`
	DetailCount       int      `json:"detailCount"`
	DetailsOpen       []bool   `json:"detailsOpen"`
	ItemCount         int      `json:"itemCount"`
	FanoutCount       int      `json:"fanoutCount"`
	TargetCount       int      `json:"targetCount"`
	FrontierCount     int      `json:"frontierCount"`
	EmptyCount        int      `json:"emptyCount"`
	LegacyCount       int      `json:"legacyCount"`
	SourceActionCount int      `json:"sourceActionCount"`
	SourceHrefs       []string `json:"sourceHrefs"`
	Labels            []string `json:"labels"`
	Orderings         []string `json:"orderings"`
	Limitations       []string `json:"limitations"`
	Text              string   `json:"text"`
}
