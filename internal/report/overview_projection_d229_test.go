package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOverviewProjectionD229Asset verifies the Decision 229 Overview
// projection: repository perimeter, presentation-quality gate for
// value-shaped entry titles, and diagnostic remainder as collapsed
// disclosure (never a principal area).
func TestOverviewProjectionD229Asset(t *testing.T) {
	scriptPath := filepath.Join("templates", "script.js")
	assetPath := filepath.Join(t.TempDir(), "overview-d229-test.js")
	asset := `const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) { this.tagName = tag; this.children = []; this.attributes = {}; this.textContent = ""; this.hidden = false; }
  get className() { return this._className || ""; }
  set className(value) { this._className = value; }
  toggle(name, force) { if (force) this.classList.add(name); else this.classList.remove(name); }
  get classList() {
    const self = this;
    return { add(name) { self._className = (self._className ? self._className + " " : "") + name; }, remove(name) { self._className = (self._className || "").split(/\s+/).filter((c) => c !== name).join(" "); } };
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
// Fixture: entries include value-shaped titles (amount, payer,
// application_context) and a diagnostic remainder subsystem.
const report = {
  repo_name: "fixture", report_language: "en", user_mechanisms: [], user_topics: [], user_sources: [],
  openable_paths: ["main.go", "pp/paypal.go", "pp/wechatpay.go", "service/proxy.go", "scan/intranet_server.go"],
  source_ids: {},
  github_source_links: { repository_url: "https://github.com/example/fixture", revision: "1".repeat(40), working_tree_paths: [] },
  repository_atlas: { version: 1, units: [], entities: [], evidence: [], relations: [] },
  discovered_surfaces: {
    version: 7,
    triggers: [
      { id: "trigger-process", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "primary_application", kind: "process_entry", identity: { path: { kind: "declaration", text: "main.go", known: true }, name: "main" }, process_entrypoint: { name: "main", location: { path: "main.go", line: 36, column: 6 } } },
      { id: "trigger-amount", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "primary_application", kind: "http_route", identity: { path: { kind: "constant", text: "amount", known: true } }, handler: { known: true, text: "amount" }, registration_site: { path: "pp/wechatpay.go", line: 76, column: 15 } },
      { id: "trigger-payer", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "primary_application", kind: "http_route", identity: { path: { kind: "constant", text: "payer", known: true } }, handler: { known: true, text: "payer" }, registration_site: { path: "pp/wechatpay.go", line: 85, column: 15 } },
      { id: "trigger-ctx", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "primary_application", kind: "http_route", identity: { path: { kind: "constant", text: "application_context", known: true } }, handler: { known: true, text: "application_context" }, registration_site: { path: "pp/paypal.go", line: 67, column: 15 } },
      { id: "trigger-http1", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "secondary_service", kind: "http_server", identity: { path: { kind: "unknown", text: "unresolved value", known: false }, name: "HTTP server" }, registration_site: { path: "service/proxy.go", line: 367, column: 34 }, server_start_site: { path: "service/proxy.go", line: 367, column: 34 } },
      { id: "trigger-http2", surface_role: "entry_surface", application_classification: "application_surface", availability: "available", executable_role: "secondary_service", kind: "http_server", identity: { path: { kind: "unknown", text: "unresolved value", known: false }, name: "HTTP server" }, registration_site: { path: "service/proxy.go", line: 323, column: 29 }, server_start_site: { path: "service/proxy.go", line: 323, column: 29 } },
    ],
  },
  architecture_canvas: {
    version: 8, validation_outcome: "accepted_partial", architecture_source: "partial_model",
    local_remainder_component_id: "component-remainder",
    title: "Saved Architecture", subtitle: "Exact saved conceptual grouping.",
    subsystems: [
      { id: "subsystem-runtime", name: "Runtime" },
      { id: "subsystem-security", name: "Security" },
      { id: "subsystem-diagnostic", name: "Supporting evidence", category: "diagnostic" },
    ],
    components: [
      { id: "component-runtime", name: "Runtime component", subsystem_id: "subsystem-runtime", members: [] },
      { id: "component-security", name: "Security component", subsystem_id: "subsystem-security", members: [] },
      { id: "component-remainder", name: "Supporting repository evidence", subsystem_id: "subsystem-diagnostic", members: [
        { id: { kind: "symbol", value: "remainder-one" }, name: "Local remainder one" },
        { id: { kind: "symbol", value: "remainder-two" }, name: "Local remainder two" },
        { id: { kind: "symbol", value: "remainder-three" }, name: "Local remainder three" },
      ] },
    ],
    behavior_anchors: [], surfaces: [], flows: [],
  },
  architecture_associations: {
    version: 1,
    components: [
      { component_id: "component-security", name: "Security component", associations: [
        { kind: "boundary", imported_family: "database", witnesses: [], observation_count: 3, source_roles: [] },
        { kind: "resource", imported_family: "net", witnesses: [], observation_count: 5, source_roles: [] },
      ] },
    ],
  },
  architecture_synthesis: { state: "accepted_partial" },
};
const roots = {};
["rm-overview", "rm-task-investigation", "rm-mechanisms", "rm-mechanism-detail",
 "rm-study-overview", "rm-study-detail", "rm-operate-detail", "rm-architecture", "rm-provenance",
].forEach((id) => {
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
  mount(host, data) { return { ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {} }; },
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
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window, document, URLSearchParams, Set, Map, AbortController, Promise });
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.renderWorkspaceTabs();
api.renderMapSummaryInto("rm-overview");
const overviewRoot = roots["rm-overview"];
const overviewText = text(overviewRoot);
// 1. Perimeter section present.
const perimeter = overviewRoot.querySelector(".rm-overview-perimeter");
// 2. Value-shaped titles never appear as primary entry cards.
const entryCards = byClass(overviewRoot, "rm-overview-entry-group").map((group) => text(group));
const entryJoined = entryCards.join("\n");
// 3. Remainder is a collapsed disclosure, not a principal card.
const remainder = overviewRoot.querySelector(".rm-overview-remainder");
const spineCards = byClass(overviewRoot, "rm-overview-spine-card").map((card) => text(card));
const spineJoined = spineCards.join("\n");
const result = {
  hasPerimeter: !!perimeter,
  perimeterText: perimeter ? text(perimeter) : "",
  entryCards: entryCards.length,
  entryText: entryJoined,
  hasAmountCard: entryJoined.includes("amount"),
  hasPayerCard: entryJoined.includes("payer"),
  hasContextCard: entryJoined.includes("application_context"),
  hasMainEntry: entryJoined.includes("main.go"),
  hasHTTPServer: entryJoined.includes("HTTP server"),
  hasRemainderDisclosure: !!remainder,
  remainderText: remainder ? text(remainder) : "",
  supportingInSpine: spineJoined.includes("Supporting"),
  overviewText,
};
process.stdout.write(JSON.stringify(result));
`
	if err := os.WriteFile(assetPath, []byte(asset), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", assetPath, scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node journey failed: %v\n%s", err, output)
	}
	var got struct {
		HasPerimeter           bool   `json:"hasPerimeter"`
		PerimeterText          string `json:"perimeterText"`
		EntryCards             int    `json:"entryCards"`
		EntryText              string `json:"entryText"`
		HasAmountCard          bool   `json:"hasAmountCard"`
		HasPayerCard           bool   `json:"hasPayerCard"`
		HasContextCard         bool   `json:"hasContextCard"`
		HasMainEntry           bool   `json:"hasMainEntry"`
		HasHTTPServer          bool   `json:"hasHTTPServer"`
		HasRemainderDisclosure bool   `json:"hasRemainderDisclosure"`
		RemainderText          string `json:"remainderText"`
		SupportingInSpine      bool   `json:"supportingInSpine"`
		OverviewText           string `json:"overviewText"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode overview D229 journey: %v\n%s", err, output)
	}
	if !got.HasPerimeter {
		t.Fatalf("repository perimeter section missing; overview text:\n%s", got.OverviewText)
	}
	if !strings.Contains(got.PerimeterText, "Analyzed repository scope") || !strings.Contains(got.PerimeterText, "Observed use / entry") {
		t.Fatalf("perimeter flow missing entry/scope/touchpoints: %s", got.PerimeterText)
	}
	if got.HasAmountCard || got.HasPayerCard || got.HasContextCard {
		t.Fatalf("value-shaped titles leaked into primary entry cards: amount=%v payer=%v context=%v\nentries:\n%s", got.HasAmountCard, got.HasPayerCard, got.HasContextCard, got.EntryText)
	}
	if !got.HasMainEntry || !got.HasHTTPServer {
		t.Fatalf("valid entry cards missing: main.go=%v http=%v\nentries:\n%s", got.HasMainEntry, got.HasHTTPServer, got.EntryText)
	}
	if !got.HasRemainderDisclosure {
		t.Fatalf("diagnostic remainder disclosure missing; overview text:\n%s", got.OverviewText)
	}
	if !strings.Contains(got.RemainderText, "Unclassified exact scope") || !strings.Contains(got.RemainderText, "3 symbols") {
		t.Fatalf("remainder disclosure copy wrong: %s", got.RemainderText)
	}
	if got.SupportingInSpine {
		t.Fatalf("Supporting repository evidence is a principal spine card:\n%s", got.OverviewText)
	}
}
