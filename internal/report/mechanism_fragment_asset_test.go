package report

// Decision 226 DOM acceptance: the mechanism fragment renders as a compact
// DFD-like list with every transition carrying claim_kind/support_mode/
// ordering, the entry first, and the unresolved frontier always visible —
// no BPMN/SIPOC/swimlane/FFBD claims, no hover-only limitations.
import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMechanismFragmentAssetRendersContractFieldsAndFrontier(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset := filepath.Join("templates", "script.js")
	runner := `
const fs = require("fs");
const vm = require("vm");
function Element(tag) {
  this.tagName = String(tag).toUpperCase();
  this._text = "";
  this.children = [];
  this.attributes = {};
  this.hidden = false;
  this.classList = { add() {}, remove() {}, toggle() {} };
}
Object.defineProperty(Element.prototype, "childNodes", { get() { return this.children; } });
Object.defineProperty(Element.prototype, "textContent", {
  get() { return this._text + this.children.map((c) => c.textContent).join(""); },
  set(value) { this._text = value == null ? "" : String(value); },
});
Element.prototype.setAttribute = function (name, value) { this.attributes[name] = String(value); };
Element.prototype.appendChild = function (child) { this.children.push(child); return child; };
Element.prototype.append = function (...children) { this.children.push(...children); };
Element.prototype.replaceChildren = function (...children) { this.children = children; };
Element.prototype.remove = function () { if (this.parentNode) { const i = this.parentNode.children.indexOf(this); if (i >= 0) this.parentNode.children.splice(i, 1); } };
Element.prototype.prepend = function (...children) { this.children.unshift(...children); };
function walk(node, out) {
  out = out || [];
  (node.children || []).forEach((child) => { out.push(child); walk(child, out); });
  return out;
}
Element.prototype.querySelector = function (selector) {
  const cls = selector.replace(".", "");
  return walk(this).find((node) => String(node.className || "").split(/\s+/).includes(cls)) || null;
};
Element.prototype.querySelectorAll = function (selector) {
  const cls = selector.replace(".", "");
  return walk(this).filter((node) => String(node.className || "").split(/\s+/).includes(cls));
};
const roots = {
  "rm-overview": new Element("div"),
  "rm-architecture": new Element("section"),
  "rm-study-overview": new Element("div"),
  "rm-study-detail": new Element("div"),
  "rm-tabs": new Element("div"),
};
const workspace = new Element("div");
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en",
  user_mechanisms: [], user_sources: [], user_topics: [],
  openable_paths: [], source_ids: {},
  architecture_canvas: { version: 1, local_remainder_component_id: "component-r", components: [], behavior_anchors: [], surfaces: [], flows: [], structural_edges: [] },
  mechanism_fragment: {
    version: 1,
    entry: {
      ordinal: 0, claim_kind: "process_entry", support_mode: "resolved_static",
      label: "process entry fixture.main", path: "main.go", line: 36,
      evidence: "behavior anchor proof_mode process_entry", scenario: "go:linux",
      limitation: "process entry identity only; runtime reachability not proven",
      ordering: "exact_local_order",
    },
    transitions: [
      {
        ordinal: 1, claim_kind: "direct_static_call", support_mode: "resolved_static",
        label: "handoff", path: "main.go", line: 150, symbol: "service.Start",
        evidence: "go_ssa surface-ssa-v12 connect_architecture_anchors",
        scenario: "Recorded Go build scenario",
        limitation: "runtime dispatch beyond the recorded build scenario not proven",
        ordering: "resolved_path_order",
      },
      {
        ordinal: 2, claim_kind: "unresolved_continuation", support_mode: "unknown",
        label: "beyond the observed handoffs", path: "", line: 0,
        evidence: "no locally saved transition", scenario: "",
        limitation: "execution order and further transitions not established",
        ordering: "not_established",
      },
    ],
    frontier: {
      ordering: "not_established",
      unresolved: ["further locally saved transitions beyond the observed handoffs"],
      limitation: "No locally saved transitions beyond the observed handoffs; execution order beyond them is not established.",
    },
  },
  navigator: { version: 1, state: "empty" },
  repository_atlas: { version: 1, units: [], entities: [], observations: [], evidence: [], relations: [] },
};
const window = {
  location: { hash: "#/architecture", host: "fixture.test", pathname: "/index.html", search: "" },
  history: {
    state: null,
    pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
    replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
    back() {},
  },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
};
window.RepomapArchitectureCanvas = {
  mount(host, data) { return { ready: Promise.resolve(), destroy() {}, openComponent() {} }; },
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll(selector) { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window, document, URLSearchParams, Set, Map, AbortController, Promise });
const api = window.__REPOMAP_WORKSPACE_TEST__;
if (!api || typeof api.renderWorkspaceTabs !== "function") {
  process.stderr.write("workspace test API missing\n");
  process.exit(2);
}
api.renderWorkspaceTabs();
const nav = roots["rm-tabs"].children.slice();
const architectureTab = nav.find((node) => node.attributes && node.attributes["data-workspace-view"] === "architecture");
if (!architectureTab) {
  process.stderr.write("architecture tab missing: " + JSON.stringify(nav.map((n) => n.attributes["data-workspace-view"])) + "\n");
  process.exit(2);
}
architectureTab.onclick();
const root = roots["rm-architecture"];
// Decision 229 D5: lanes (entry + transitions) render as separate cards —
// never one linearized numbered sequence.
const items = root.querySelectorAll(".rm-mechanism-fragment__lane");
const frontier = root.querySelector(".rm-mechanism-fragment__frontier");
const kinds = items.map((item) => {
  const strong = item.querySelector(".rm-mechanism-fragment__kind");
  return strong ? strong.textContent : "";
});
const orderings = items.map((item) => {
  const el = item.querySelector(".rm-mechanism-fragment__ordering");
  return el ? el.textContent : "";
});
// Decision 226/229: evidence and limitation are visible per transition —
// evidence (including raw enums) under a secondary disclosure, limitation
// as primary copy.
const evidenceSpans = items.map((item) => {
  const details = item.querySelector(".rm-mechanism-fragment__evidence-details");
  return details ? details.textContent.replace(/\s+/g, " ").trim() : "";
});
const limitationSpans = items.map((item) => {
  const el = item.querySelector(".rm-mechanism-fragment__limitation");
  return el ? el.textContent : "";
});
process.stdout.write(JSON.stringify({
  itemCount: items.length,
  kinds,
  orderings,
  evidenceSpans,
  limitationSpans,
  frontierPresent: !!frontier,
  frontierText: frontier ? frontier.textContent.replace(/\s+/g, " ").trim() : "",
  architectureText: root.textContent.slice(0, 400),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "mechanism-fragment-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, asset, "en").CombinedOutput()
	if err != nil {
		t.Fatalf("run mechanism fragment workspace: %v\n%s", err, output)
	}
	var out struct {
		ItemCount        int      `json:"itemCount"`
		Kinds            []string `json:"kinds"`
		Orderings        []string `json:"orderings"`
		EvidenceSpans    []string `json:"evidenceSpans"`
		LimitationSpans  []string `json:"limitationSpans"`
		FrontierPresent  bool     `json:"frontierPresent"`
		FrontierText     string   `json:"frontierText"`
		ArchitectureText string   `json:"architectureText"`
	}
	if err := json.Unmarshal(output, &out); err != nil {
		t.Fatalf("decode mechanism fragment workspace: %v\n%s", err, output)
	}
	// Entry + direct_static_call + unresolved_continuation, as human copy
	// (Decision 229 D4: raw enums never primary UI).
	if out.ItemCount != 3 {
		t.Fatalf("items = %d, want 3: %#v", out.ItemCount, out.Kinds)
	}
	wantKinds := []string{"Process entry", "Direct static call", "Continuation not recovered"}
	for index, want := range wantKinds {
		if index >= len(out.Kinds) || out.Kinds[index] != want {
			t.Fatalf("kinds = %#v, want %#v", out.Kinds, wantKinds)
		}
	}
	// Raw enums never appear as primary kind copy.
	for _, kind := range out.Kinds {
		if kind == "direct_static_call" || kind == "resolved_static" ||
			kind == "resolved_path_order" || kind == "not_established" || kind == "entry" {
			t.Fatalf("raw enum leaked into primary mechanism copy: %q", kind)
		}
	}
	// Orderings: human copy — Local callsite order, Continuation not
	// recovered (resolved_path_order / not_established stay in details).
	if len(out.Orderings) != 3 || out.Orderings[1] != "Local callsite order" || out.Orderings[2] != "Continuation not recovered" {
		t.Fatalf("orderings = %#v", out.Orderings)
	}
	// Evidence (including raw enums) under Evidence details — always
	// present, never hover-only.
	if len(out.EvidenceSpans) != 3 || out.EvidenceSpans[0] == "" || out.EvidenceSpans[1] == "" || out.EvidenceSpans[2] == "" {
		t.Fatalf("evidence spans = %#v, want 3 non-empty", out.EvidenceSpans)
	}
	if len(out.LimitationSpans) != 3 || out.LimitationSpans[0] == "" || out.LimitationSpans[1] == "" || out.LimitationSpans[2] == "" {
		t.Fatalf("limitation spans = %#v, want 3 non-empty", out.LimitationSpans)
	}
	// Frontier always visible without hover, and carries the unresolved item.
	if !out.FrontierPresent || !strings.Contains(out.FrontierText, "No locally saved transitions") ||
		!strings.Contains(out.FrontierText, "further locally saved transitions beyond the observed handoffs") {
		t.Fatalf("frontier missing/hover-only or unresolved item dropped: %#v", out.FrontierText)
	}
	// No BPMN/SIPOC/swimlane/FFBD claims in the fragment copy.
	lower := strings.ToLower(out.ArchitectureText)
	for _, forbidden := range []string{"swimlane", "sipoc", "bpmn", "gateway", "ffbd"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("fragment copy contains forbidden representation claim %q: %s", forbidden, out.ArchitectureText)
		}
	}
}
