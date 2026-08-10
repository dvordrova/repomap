package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Executable targets open directly in their exact Entrypoints context. There
// is no lens selector; the target-rooted handoff group is selected on Canvas.
func TestEntrypointLensUsesCanvasWithoutLegacySideList(t *testing.T) {
	if ArchitectureCanvasVersion != 15 || EntrypointHandoffGroupVersion != 2 {
		t.Fatalf("fixture requires Canvas15/group v2, got %d/%d", ArchitectureCanvasVersion, EntrypointHandoffGroupVersion)
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
  "rm-architecture": new Element("section"),
  "rm-tabs": new Element("div"),
};
const workspace = new Element("div");
function transition(label, path, line, column, target, componentIDs) {
  return {
    claim_kind: target ? "direct_static_call" : "process_entry",
    support_mode: "resolved_static", label, path, line, column,
    component_ids: componentIDs || [],
    symbol: label, evidence: "go_ssa secret diagnostic detail",
    evidence_ref: { kind: "architecture_entry_handoff", id: "entry-handoff-secret" },
    provenance: { provider: "go_ssa", version: "surface-ssa-v12", operation: "collect_entry_direct_static_handoff" },
    scenario: "go:linux/amd64:tags=", limitation: "raw repeated limitation",
    ordering: "resolved_path_order", target,
  };
}
function group(id, entrySymbol, entryPath, componentIDs, handoffs) {
  const entry = transition(entrySymbol || "process entry", entryPath, 10, 6, null, [componentIDs[0]]);
  entry.symbol = entrySymbol;
  return {
    version: 2, id, component_ids: componentIDs,
    entry,
    entry_handoffs: Array.isArray(handoffs) ? handoffs : [handoffs],
    frontier: { ordering: "not_established", unresolved: ["continuation beyond first hop"], limitation: "raw frontier limitation" },
  };
}
const sameComponentHandoffs = Array.from({ length: 13 }, (_, index) =>
  transition(
    "fixture.local" + index, "main.go", 30 + index, 4,
    { label: "fixture.local" + index, path: "main.go", line: 200 + index, column: 6 },
    ["c1"]
  )
);
const groupA = group("entry-group-a", "fixture.main", "main.go", ["c1", "c2"], [
  transition("fixture.service.Start", "main.go", 15, 9,
    { label: "fixture.service.Start", path: "service.go", line: 20, column: 4 }, ["c2"]),
].concat(sameComponentHandoffs));
const groupB = group("entry-group-b", "fixture.main", "worker.go", ["c3"],
  transition("fixture.client.Send", "worker.go", 25, 8,
    { label: "fixture.client.Send", path: "client.go", line: 30, column: 3 }, []));
// Zero/plural entry ownership must fall back to the exact path leaf, never a
// guessed component from group.component_ids.
groupB.entry.component_ids = [];
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en",
  analysis_target: {
    version: 2, ref: "fixture-target", kind: "executable_package", module_dir: ".",
    package_dir: ".", package_path: "fixture", roots: [{ path: "main.go", line: 10 }],
  },
  github_source_links: { repository_url: "https://github.com/acme/fixture", revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
  user_mechanisms: [], user_topics: [],
  // Mixed snippets must not suppress exact pinned source actions.
  user_sources: [{ path: "other.go", start_line: 1, end_line: 1, lines: ["package fixture"] }],
  openable_paths: ["client.go", "cmd/zero/main.go", "main.go", "other.go", "service.go", "worker.go"], source_ids: {},
  architecture_canvas: {
    version: 15, local_remainder_component_id: "component-r",
    components: [
      { id: "c1", name: "Entry", participating_surface_ids: ["surface-main"] },
      { id: "c2", name: "Service" }, { id: "c3", name: "Client" },
    ],
    subsystems: [], behavior_anchors: [], flows: [], structural_edges: [],
    surfaces: [
      { id: "surface-main", kind: "process_entry", name: "main", participating_component_ids: ["c1"] },
      { id: "surface-zero", kind: "process_entry", name: "zero.main", participating_component_ids: [],
        evidence: [{ path: "cmd/zero/main.go", line: 7, column: 6 }] },
    ],
    entry_handoff_groups: [groupA, groupB],
  },
  repository_atlas: { version: 1, units: [], entities: [], observations: [], evidence: [], relations: [] },
};
const window = {
  location: { hash: "#canvas", host: "fixture.test", pathname: "/index.html", search: "" },
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
  querySelector(selector) {
    if (selector === ".rm-workspace") return workspace;
    return Object.values(roots).flatMap((root) => walk(root)).find((node) => matches(node, selector)) || null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: report.report_language };
window.document = document;
const context = { window, document, Element, URLSearchParams, Set, Map, AbortController, Promise };
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), context);
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "architecture_canvas.js"), "utf8"), context);
const realCanvas = window.RepomapArchitectureCanvas;
const selectedGroups = [];
const selectedLenses = [];
window.RepomapArchitectureCanvas = {
  projectArchitectureLens: realCanvas.projectArchitectureLens,
  projectEntrypointHandoffOverlay: realCanvas.projectEntrypointHandoffOverlay,
  mount() { return {
    ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {},
    clearStudyMechanismOverlay() {}, setLens(id) { selectedLenses.push(id); },
    selectEntrypointHandoffGroup(id) { selectedGroups.push(id); },
  }; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), context);
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.renderWorkspaceTabs();
api.restoreWorkspaceFromRoute({ replace: true });
Promise.resolve().then(() => Promise.resolve()).then(() => {
  const root = roots["rm-architecture"];
  const targetLinks = roots["rm-tabs"].querySelectorAll(".rm-target-link");
  const lenses = walk(root).filter((node) => node.attributes["data-map-lens"]);
  const selectors = root.querySelectorAll(".rm-map-entry-selector");
  const sourceActions = root.querySelectorAll(".rm-source-action-link");
  const zeroHopEntries = root.querySelectorAll(".rm-map-entry-zero-hop");
  const zeroHopActions = zeroHopEntries.flatMap((entry) => entry.querySelectorAll(".rm-source-action-link"));
  const overflowCalls = root.querySelectorAll(".rm-map-entry-overflow__call");
  const lensHost = document.getElementById("rm-map-lens-objects");
  const contextDetails = lensHost ? walk(lensHost).filter((node) => node.tagName === "DETAILS") : [];
  process.stdout.write(JSON.stringify({
    targetCount: targetLinks.length,
    targetHref: targetLinks[0] && targetLinks[0].getAttribute("href") || "",
    targetCurrent: targetLinks[0] && targetLinks[0].getAttribute("aria-current") || "",
    lensIDs: lenses.map((node) => node.attributes["data-map-lens"]),
    lensHostPresent: !!lensHost,
    lensHostMode: lensHost && lensHost.getAttribute("data-lens") || "",
    selectorLabels: selectors.map((node) => node.textContent),
    selectorCount: selectors.length, overflowCallCount: overflowCalls.length,
    sourceActionCount: sourceActions.length,
    sourceHrefs: sourceActions.map((item) => item.getAttribute("href") || ""),
    zeroHopCount: zeroHopEntries.length,
    zeroHopHrefs: zeroHopActions.map((item) => item.getAttribute("href") || ""),
    zeroHopText: zeroHopEntries.map((item) => item.textContent),
    selectedGroups,
    selectedLenses,
    selectedStates: selectors.map((item) => item.getAttribute("aria-pressed")),
    contextDetails: contextDetails.length,
    hash: window.location.hash,
    text: root.textContent.replace(/\s+/g, " ").trim(),
  }));
}).catch((error) => { process.stderr.write(String(error && error.stack || error)); process.exit(2); });
`
	runnerPath := filepath.Join(t.TempDir(), "entry-handoff-group-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(language string) entryHandoffAssetResult {
		t.Helper()
		output, err := exec.Command(node, runnerPath, asset, language).CombinedOutput()
		if err != nil {
			t.Fatalf("run entry handoff workspace (%s): %v\n%s", language, err, output)
		}
		var got entryHandoffAssetResult
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode entry handoff workspace (%s): %v\n%s", language, err, output)
		}
		return got
	}

	en := run("en")
	if en.TargetCount != 1 || en.TargetHref != "#canvas" || en.TargetCurrent != "page" || en.Hash != "#canvas" {
		t.Fatalf("single executable target rail = %#v", en)
	}
	if len(en.LensIDs) != 0 || !en.LensHostPresent || en.LensHostMode != "entrypoints" {
		t.Fatalf("Entrypoints must be default context without a lens selector: %#v", en)
	}
	if en.SelectorCount != 2 || en.OverflowCallCount == 0 || en.ContextDetails != 0 || en.SourceActionCount == 0 {
		t.Fatalf("default Entrypoints context = selectors %d overflow calls %d details %d sources %d",
			en.SelectorCount, en.OverflowCallCount, en.ContextDetails, en.SourceActionCount)
	}
	if en.ZeroHopCount != 1 || len(en.ZeroHopHrefs) != 1 ||
		!strings.Contains(en.ZeroHopHrefs[0], "/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cmd/zero/main.go#L7") ||
		!strings.Contains(strings.Join(en.ZeroHopText, " "), "zero.main") {
		t.Fatalf("unowned zero-hop process entry is not visible and source-backed: %#v", en)
	}
	if !strings.Contains(strings.Join(en.SelectedGroups, ","), "entry-group-a") ||
		!strings.Contains(strings.Join(en.SelectedLenses, ","), "entrypoints") ||
		strings.Join(en.SelectedStates, ",") != "true,false" {
		t.Fatalf("exact target root was not default-selected on Canvas: %#v", en)
	}
	for _, href := range en.SourceHrefs {
		if !strings.Contains(href, "github.com/acme/fixture/blob/") {
			t.Fatalf("Entrypoints source is not pinned: %q", href)
		}
	}
	for _, leaked := range []string{"claim_kind", "resolved_static", "architecture_entry_handoff", "entry-handoff-secret", "go:linux", "raw repeated limitation", "raw frontier limitation"} {
		if strings.Contains(en.Text, leaked) {
			t.Fatalf("diagnostic field %q leaked into ordinary HTML: %s", leaked, en.Text)
		}
	}

	ru := run("ru")
	if len(ru.LensIDs) != 0 || ru.SelectorCount != en.SelectorCount ||
		ru.SourceActionCount != en.SourceActionCount || ru.LensHostMode != "entrypoints" {
		t.Fatalf("default Entrypoints context changed across locales: EN %#v RU %#v", en, ru)
	}
}

func TestEntrypointHandoffCanvasOverlayKeepsLayoutAndOneSelection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = String(tag || "div").toUpperCase(); this.children = []; this.attributes = {};
    this.className = ""; this._text = ""; this.hidden = false; this.style = {}; this.dataset = {};
    this.parentNode = null; this.handlers = {};
    this.classList = {
      add: (...names) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); names.forEach((name) => values.add(name)); this.className = Array.from(values).join(" "); },
      remove: (...names) => { const removed = new Set(names); this.className = String(this.className).split(/\s+/).filter((name) => name && !removed.has(name)).join(" "); },
      toggle: (name, force) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); const active = force === undefined ? !values.has(name) : !!force; if (active) values.add(name); else values.delete(name); this.className = Array.from(values).join(" "); return active; },
      contains: (name) => String(this.className).split(/\s+/).includes(name),
    };
  }
  get childNodes() { return this.children; }
  get childElementCount() { return this.children.length; }
  get textContent() { return this._text + this.children.map((child) => child.textContent).join(""); }
  set textContent(value) { this._text = value == null ? "" : String(value); }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { if (child) { child.parentNode = this; this.children.unshift(child); } }
  replaceChildren(...children) { this.children = []; this._text = ""; children.forEach((child) => this.appendChild(child)); }
  remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); }
  setAttribute(name, value) { this.attributes[name] = String(value); if (name === "class") this.className = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : String(this.attributes[name]); }
  removeAttribute(name) { delete this.attributes[name]; }
  addEventListener(name, handler) { (this.handlers[name] ||= []).push(handler); }
  removeEventListener() {}
  dispatch(name) { (this.handlers[name] || []).forEach((handler) => handler({ target: this, stopPropagation() {}, preventDefault() {} })); }
  focus() {}
  contains(node) { if (node === this) return true; return this.children.some((child) => child.contains && child.contains(node)); }
  closest() { return null; }
  getBoundingClientRect() { return { left: 0, top: 0, right: 900, bottom: 640, width: 900, height: 640 }; }
  querySelector(selector) { return walk(this).find((node) => matches(node, selector)) || null; }
  querySelectorAll(selector) { return walk(this).filter((node) => matches(node, selector)); }
  scrollIntoView() {}
}
function walk(node, out) { out = out || []; (node.children || []).forEach((child) => { out.push(child); walk(child, out); }); return out; }
function matches(node, selector) {
  if (selector[0] === ".") return String(node.className || "").split(/\s+/).includes(selector.slice(1));
  const attr = selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
  return !!(attr && Object.prototype.hasOwnProperty.call(node.attributes || {}, attr[1]) && (attr[2] == null || node.attributes[attr[1]] === attr[2]));
}
const document = {
  createElement: (tag) => new Element(tag), createElementNS: (_, tag) => new Element(tag),
  createTextNode: (value) => { const node = new Element("#text"); node.textContent = value; return node; },
  querySelector: () => null, querySelectorAll: () => [], getElementById: () => null,
  addEventListener() {}, removeEventListener() {}, body: new Element("body"), documentElement: new Element("html"),
};
function ELK() {}
const window = {
  document, ELK, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (fn) => fn(), setTimeout, clearTimeout,
  innerWidth: 1200, innerHeight: 800, location: { hash: "" }, history: { replaceState() {} },
  addEventListener() {}, removeEventListener() {},
  RepomapUI: { message(id) { return id; } },
};
const sandbox = {
  window, document, Element, ELK, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (fn) => fn(), setTimeout, clearTimeout, console,
  addEventListener() {}, removeEventListener() {},
};
sandbox.global = sandbox; vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandbox);
function transition(path, line, column, symbol, componentIDs, target) {
  return { claim_kind: target ? "direct_static_call" : "process_entry", support_mode: "resolved_static",
    label: symbol, path, line, column, symbol, component_ids: componentIDs, target };
}
function group(id, entryIDs, handoffs) {
  return { version: 2, id, component_ids: Array.from(new Set(entryIDs.concat(...handoffs.map((item) => item.component_ids)))),
    entry: transition("main.go", 10, 6, "fixture.main", entryIDs, null), entry_handoffs: handoffs,
    frontier: { ordering: "not_established", limitation: "continuation unknown" } };
}
const cross = transition("main.go", 15, 9, "fixture.service.Start", ["c2"],
  { label: "fixture.service.Start", path: "service.go", line: 20, column: 4 });
const local = Array.from({ length: 13 }, (_, index) => transition(
  "main.go", 30 + index, 5, "fixture.local" + index, ["c1"],
  { label: "fixture.local" + index, path: "main.go", line: 100 + index, column: 2 }
));
const second = transition("worker.go", 12, 7, "fixture.worker.cross", ["c1"],
  { label: "fixture.worker.local", path: "worker.go", line: 22, column: 3 });
const repeated = Array.from({ length: 19 }, (_, index) => transition(
  "main.go", 130 + index, 2, "fixture.http.Register" + index, ["c2"],
  { label: "fixture.http.Register" + index, path: "http/register" + index + ".go", line: 20 + index, column: 6 }
));
const data = {
  version: 15,
  components: [{ id: "c1", name: "Entry" }, { id: "c2", name: "Service" }],
  subsystems: [{ id: "s1", name: "Infrastructure and integration services",
    description: "Exact HTTP and provider wiring", component_ids: ["c1", "c2"] }],
  structural_edges: [], behavior_anchors: [], flows: [], flow_edges: [], surfaces: [],
  entry_handoff_groups: [
    group("group-a", ["c1"], [cross].concat(local)),
    group("group-b", ["c2"], [second]),
    group("group-c", ["c1"], repeated),
  ],
};
const opened = [];
const host = new Element("div");
const app = window.RepomapArchitectureCanvas.mount(host, data, {
  userMode: true, message: (id) => id === "architecture.relation.calls" ? "calls" : id,
  openLocation(path, line, column) { opened.push([path, line, column]); },
});
app.ready.then(() => {
  app.setLens("entrypoints");
  const surface = host.querySelector(".rm-arch__surface");
  const components = host.querySelectorAll(".rm-arch__component");
  const groupTitle = host.querySelector(".rm-arch__group-title");
  const groupShell = host.querySelector(".rm-arch__group");
  const before = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  const firstCard = components[0].querySelector(".rm-arch__component-card");
  firstCard.dispatch("click");
  const selectedBeforeEntry = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedBeforeEntry = components.filter((node) => node.classList.contains("is-dimmed")).length;
  app.selectEntrypointHandoffGroup("group-a");
  const selectedWithEntry = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedWithEntry = components.filter((node) => node.classList.contains("is-dimmed")).length;
  const edgesA = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  const badgesA = host.querySelectorAll(".rm-arch__entry-handoff-source");
  const markersA = edgesA.map((edge) => edge.querySelector(".rm-arch__edge-visible")).filter(Boolean)
    .map((path) => path.getAttribute("marker-end"));
  const participantsA = components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-entry-handoff-participant"));
  if (badgesA[0]) badgesA[0].dispatch("click");
  const after = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.selectEntrypointHandoffGroup("group-b");
  const edgesB = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  app.selectEntrypointHandoffGroup("group-c");
  const aggregateEdges = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  const aggregateBadges = host.querySelectorAll(".rm-arch__entry-handoff-source");
  const aggregateBadgeCounts = aggregateBadges.map((item) => item.getAttribute("data-entry-handoff-call-count") || "");
  const aggregateBadgeText = aggregateBadges.map((item) => item.textContent);
  firstCard.dispatch("click");
  const edgesAfterComponent = host.querySelectorAll(".rm-arch__edge--entry-handoff").length;
  const badgesAfterComponent = host.querySelectorAll(".rm-arch__entry-handoff-source").length;
  const participantsAfterComponent = components.filter((node) => node.classList.contains("rm-arch__is-entry-handoff-participant")).length;
  const selectedAfterComponent = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedAfterComponent = components.filter((node) => node.classList.contains("is-dimmed")).length;
  const studyMechanism = {
    id: "study-mechanism-1", ordinal: 1,
    nodes: [
      { id: "n1", label: "fixture.main", component_ids: ["c1"] },
      { id: "n2", label: "fixture.start", component_ids: ["c2"] },
      { id: "n3", label: "fixture.local", component_ids: ["c2"] },
      { id: "n4", label: "fixture.offmap", component_ids: [] },
      { id: "n5", label: "fixture.shared", component_ids: ["c1", "c2"] },
      { id: "n6", label: "fixture.finish", component_ids: ["c1"] },
    ],
    edges: [
      { id: "e1", from_node_id: "n1", to_node_id: "n2", invocation: "synchronous" },
      { id: "e2", from_node_id: "n2", to_node_id: "n3", invocation: "goroutine" },
      { id: "e3", from_node_id: "n3", to_node_id: "n4", invocation: "deferred" },
      { id: "e4", from_node_id: "n4", to_node_id: "n5", invocation: "synchronous" },
      { id: "e5", from_node_id: "n5", to_node_id: "n6", invocation: "synchronous" },
    ],
  };
  app.setLens("landscape");
  const studyBefore = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.setStudyMechanismOverlay(studyMechanism);
  const studyEdges = host.querySelectorAll(".rm-arch__edge--study-mechanism");
  const studyParticipants = components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-study-mechanism-participant"));
  const studyMarkers = studyEdges.map((edge) => edge.querySelector(".rm-arch__edge-visible")).filter(Boolean)
    .map((path) => path.getAttribute("marker-end"));
  const studyProjection = window.RepomapArchitectureCanvas.projectStudyMechanismOverlay(studyMechanism, ["c1", "c2"]);
  const studyAfter = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.clearStudyMechanismOverlay();
  process.stdout.write(JSON.stringify({
    edgesA: edgesA.length, badgesA: badgesA.length, markersA,
    participantsA: participantsA.length, badgeText: badgesA.map((item) => item.textContent),
    opened, layoutUnchanged: before === after, edgesB: edgesB.length,
    aggregateEdges: aggregateEdges.length, aggregateBadges: aggregateBadges.length,
    aggregateBadgeCounts, aggregateBadgeText,
    selectedBeforeEntry, dimmedBeforeEntry, selectedWithEntry, dimmedWithEntry,
    edgesAfterComponent, badgesAfterComponent, participantsAfterComponent,
    selectedAfterComponent, dimmedAfterComponent,
    groupTitleText: groupTitle && groupTitle.textContent || "",
    groupTitleTooltip: groupTitle && groupTitle.title || "",
    groupTooltip: groupShell && groupShell.title || "",
    studyEdges: studyEdges.length, studyParticipants: studyParticipants.length,
    studyMarkers, studySideReasons: studyProjection.side_rows.map((row) => row.reason),
    studyComponentIDs: studyProjection.component_ids,
    studyLayoutUnchanged: studyBefore === studyAfter,
    studyEdgesAfterClear: host.querySelectorAll(".rm-arch__edge--study-mechanism").length,
    studyParticipantsAfterClear: components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-study-mechanism-participant")).length,
  }));
}).catch((error) => { process.stderr.write(String(error && error.stack || error)); process.exit(2); });
`
	runnerPath := filepath.Join(t.TempDir(), "entry-handoff-overlay-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, asset).CombinedOutput()
	if err != nil {
		t.Fatalf("run entry handoff overlay: %v\n%s", err, output)
	}
	var got struct {
		EdgesA                      int      `json:"edgesA"`
		BadgesA                     int      `json:"badgesA"`
		MarkersA                    []string `json:"markersA"`
		ParticipantsA               int      `json:"participantsA"`
		BadgeText                   []string `json:"badgeText"`
		Opened                      [][]any  `json:"opened"`
		LayoutUnchanged             bool     `json:"layoutUnchanged"`
		EdgesB                      int      `json:"edgesB"`
		AggregateEdges              int      `json:"aggregateEdges"`
		AggregateBadges             int      `json:"aggregateBadges"`
		AggregateBadgeCounts        []string `json:"aggregateBadgeCounts"`
		AggregateBadgeText          []string `json:"aggregateBadgeText"`
		SelectedBeforeEntry         int      `json:"selectedBeforeEntry"`
		DimmedBeforeEntry           int      `json:"dimmedBeforeEntry"`
		SelectedWithEntry           int      `json:"selectedWithEntry"`
		DimmedWithEntry             int      `json:"dimmedWithEntry"`
		EdgesAfterComponent         int      `json:"edgesAfterComponent"`
		BadgesAfterComponent        int      `json:"badgesAfterComponent"`
		ParticipantsAfterComponent  int      `json:"participantsAfterComponent"`
		SelectedAfterComponent      int      `json:"selectedAfterComponent"`
		DimmedAfterComponent        int      `json:"dimmedAfterComponent"`
		GroupTitleText              string   `json:"groupTitleText"`
		GroupTitleTooltip           string   `json:"groupTitleTooltip"`
		GroupTooltip                string   `json:"groupTooltip"`
		StudyEdges                  int      `json:"studyEdges"`
		StudyParticipants           int      `json:"studyParticipants"`
		StudyMarkers                []string `json:"studyMarkers"`
		StudySideReasons            []string `json:"studySideReasons"`
		StudyComponentIDs           []string `json:"studyComponentIDs"`
		StudyLayoutUnchanged        bool     `json:"studyLayoutUnchanged"`
		StudyEdgesAfterClear        int      `json:"studyEdgesAfterClear"`
		StudyParticipantsAfterClear int      `json:"studyParticipantsAfterClear"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode entry handoff overlay: %v\n%s", err, output)
	}
	if got.EdgesA != 1 || got.BadgesA != 1 || got.ParticipantsA != 2 || !got.LayoutUnchanged {
		t.Fatalf("selected overlay = edges:%d badges:%d participants:%d layout unchanged:%t",
			got.EdgesA, got.BadgesA, got.ParticipantsA, got.LayoutUnchanged)
	}
	if got.EdgesB != 1 {
		t.Fatalf("second selection rendered %d edges, want exactly its one edge", got.EdgesB)
	}
	if got.AggregateEdges != 1 || got.AggregateBadges != 1 ||
		strings.Join(got.AggregateBadgeCounts, ",") != "19" ||
		strings.Join(got.AggregateBadgeText, ",") != "19 calls" {
		t.Fatalf("19 parallel component-pair calls = edges:%d badges:%d counts:%#v labels:%#v",
			got.AggregateEdges, got.AggregateBadges, got.AggregateBadgeCounts, got.AggregateBadgeText)
	}
	if got.SelectedBeforeEntry != 1 || got.DimmedBeforeEntry != 1 ||
		got.SelectedWithEntry != 0 || got.DimmedWithEntry != 0 {
		t.Fatalf("entry activation did not exclusively clear component focus: selected/dimmed %d/%d -> %d/%d",
			got.SelectedBeforeEntry, got.DimmedBeforeEntry, got.SelectedWithEntry, got.DimmedWithEntry)
	}
	if got.EdgesAfterComponent != 0 || got.BadgesAfterComponent != 0 ||
		got.ParticipantsAfterComponent != 0 || got.SelectedAfterComponent != 1 || got.DimmedAfterComponent != 1 {
		t.Fatalf("component click did not exclusively clear entry overlay: edges %d badges %d participants %d selected %d dimmed %d",
			got.EdgesAfterComponent, got.BadgesAfterComponent, got.ParticipantsAfterComponent,
			got.SelectedAfterComponent, got.DimmedAfterComponent)
	}
	if got.GroupTitleText != "Infrastructure and integration services" ||
		got.GroupTitleTooltip != "Infrastructure and integration services — Exact HTTP and provider wiring" ||
		got.GroupTooltip != got.GroupTitleTooltip {
		t.Fatalf("full group title/tooltip lost: text %q title %q shell %q",
			got.GroupTitleText, got.GroupTitleTooltip, got.GroupTooltip)
	}
	if len(got.MarkersA) != 1 || got.MarkersA[0] != "url(#rm-arch-entry-handoff-arrow)" ||
		len(got.BadgeText) != 1 || !strings.Contains(strings.Join(got.BadgeText, " "), "main.go:15:9") {
		t.Fatalf("overlay arrows/source badges = markers %#v labels %#v", got.MarkersA, got.BadgeText)
	}
	if len(got.Opened) != 1 || len(got.Opened[0]) != 3 || got.Opened[0][0] != "main.go" ||
		got.Opened[0][1] != float64(15) || got.Opened[0][2] != float64(9) {
		t.Fatalf("overlay source action lost exact column: %#v", got.Opened)
	}
	if got.StudyEdges != 1 || got.StudyParticipants != 2 || !got.StudyLayoutUnchanged {
		t.Fatalf("Study overlay = edges:%d participants:%d layout unchanged:%t",
			got.StudyEdges, got.StudyParticipants, got.StudyLayoutUnchanged)
	}
	if strings.Join(got.StudyMarkers, ",") != "url(#rm-arch-study-mechanism-arrow)" ||
		strings.Join(got.StudySideReasons, ",") != "same_component,zero_component,zero_component,plural_components" ||
		strings.Join(got.StudyComponentIDs, ",") != "c1,c2" {
		t.Fatalf("Study projection = markers %#v sides %#v participants %#v",
			got.StudyMarkers, got.StudySideReasons, got.StudyComponentIDs)
	}
	if got.StudyEdgesAfterClear != 0 || got.StudyParticipantsAfterClear != 0 {
		t.Fatalf("cleared Study overlay retained edges %d or participants %d",
			got.StudyEdgesAfterClear, got.StudyParticipantsAfterClear)
	}
}

type entryHandoffAssetResult struct {
	TargetCount                int      `json:"targetCount"`
	TargetHref                 string   `json:"targetHref"`
	TargetCurrent              string   `json:"targetCurrent"`
	LensIDs                    []string `json:"lensIDs"`
	LensHostPresent            bool     `json:"lensHostPresent"`
	LensHostMode               string   `json:"lensHostMode"`
	SelectorLabels             []string `json:"selectorLabels"`
	SelectorCount              int      `json:"selectorCount"`
	OverflowCallCount          int      `json:"overflowCallCount"`
	ImmediateSourceActionCount int      `json:"immediateSourceActionCount"`
	DefaultSelectionCount      int      `json:"defaultSelectionCount"`
	DefaultSelectedStates      []string `json:"defaultSelectedStates"`
	FocusSelectionCount        int      `json:"focusSelectionCount"`
	SourceActionCount          int      `json:"sourceActionCount"`
	SourceHrefs                []string `json:"sourceHrefs"`
	ZeroHopCount               int      `json:"zeroHopCount"`
	ZeroHopHrefs               []string `json:"zeroHopHrefs"`
	ZeroHopText                []string `json:"zeroHopText"`
	SelectedGroups             []string `json:"selectedGroups"`
	SelectedLenses             []string `json:"selectedLenses"`
	SelectedStates             []string `json:"selectedStates"`
	ContextDetails             int      `json:"contextDetails"`
	Hash                       string   `json:"hash"`
	Text                       string   `json:"text"`
}
