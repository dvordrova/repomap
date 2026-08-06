package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Decision 230 D2 regression (fresh review S1): association rows must never
// nest interactive elements — button>button, button>a, a>button, a>a must be
// zero in the expanded state. The inspector renders rows as
// div.rm-arch__association-row > button.toggle + div.rm-arch__association-witnesses
// (sibling), with witness buttons inside the sibling list only.
func TestAssociationRowsNeverNestInteractiveElements(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	canvasPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runnerPath := filepath.Join(t.TempDir(), "association-nesting-runner.js")
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
    this.style = {};
    this.dataset = {};
    this.parentNode = null;
    this.focused = false;
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
  get childNodes() { return this.children; }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { if (child) { child.parentNode = this; this.children.unshift(child); } }
  replaceChildren(...children) {
    this.children = [];
    this.textContent = "";
    children.forEach((child) => this.appendChild(child));
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : String(this.attributes[name]); }
  removeAttribute(name) { delete this.attributes[name]; }
  addEventListener() {}
  removeEventListener() {}
  focus() { this.focused = true; }
  getBoundingClientRect() { return { left: 0, top: 0, right: 300, bottom: 200, width: 300, height: 200 }; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  scrollIntoView() {}
}

const document = {
  createElement: (tag) => new Element(tag),
  createElementNS: (ns, tag) => new Element(tag),
  createTextNode: (value) => { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById: () => null,
  querySelector: () => null,
  querySelectorAll: () => [],
  addEventListener() {},
  removeEventListener() {},
  body: new Element("body"),
  documentElement: new Element("html"),
};

const messages = null;

const window = {
  document,
  AbortController,
  Set, Map, URLSearchParams, Promise, requestAnimationFrame: (fn) => fn(),
  clearTimeout, setTimeout,
  innerWidth: 1440, innerHeight: 1000,
  addEventListener() {},
  RepomapUI: { message(id, params) { return id; } },
};
const global = {
  document,
  addEventListener() {},
  removeEventListener() {},
};
global.window = window;
window.document = document;
window.global = global;

// Build one contextified sandbox whose global property IS the sandbox
// itself (mirroring the browser/Node global object), then execute the asset.
const sandboxGlobal = {
  window, document,
  addEventListener() {}, removeEventListener() {},
  Element,
  requestAnimationFrame: (fn) => fn(),
  setTimeout, clearTimeout,
  console,
  AbortController, Set, Map, URLSearchParams, Promise,
};
sandboxGlobal.global = sandboxGlobal;
sandboxGlobal.window = window;
sandboxGlobal.document = document;

vm.createContext(sandboxGlobal);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandboxGlobal);

const host = new Element("div");
const data = {
  components: [{
    id: "comp-one",
    name: "Component one",
    members: [{ id: { kind: "package", value: "pkg/one" }, name: "pkg/one" }],
  }],
  subsystems: [{ id: "sub-one", name: "Subsystem", component_ids: ["comp-one"] }],
  groups: [],
  structural_edges: [],
  behavior_anchors: [],
  relations: [],
  surfaces: [],
};
const options = {
  userMode: true,
  message: (id) => id,
  componentContexts: {
    "comp-one": {
      package_paths: ["pkg/one"],
      surface_starts: [],
      structural_relations: [],
    },
  },
  associations: {
    components: [{
      component_id: "comp-one",
      associations: [{
        kind: "boundary",
        imported_family: "net/http",
        owning_unit: "pkg/one",
        observation_count: 2,
        witnesses: [
          { symbol: "ServeHTTP", path: "server.go", line: 42, role: "production" },
          { symbol: "ListenAndServe", path: "main.go", line: 10, role: "production" },
        ],
      }],
    }],
  },
};
const app = window.RepomapArchitectureCanvas.mount(host, data, options);
app.ready.then(() => {
  app.openComponent("comp-one");
  // Force the witness list visible (toggle click equivalent) to test the
  // expanded state.
  const rows = [];
  (function walk(node) {
    if (!node) return;
    rows.push(node);
    (node.children || []).forEach(walk);
  })(host);
  const toggles = rows.filter((node) => String(node.className).includes("rm-arch__association-row__toggle"));
  toggles.forEach((toggle) => {
    const row = toggle.parentNode;
    (row.children || []).forEach((child) => {
      if (String(child.className).includes("rm-arch__association-witnesses")) child.hidden = false;
    });
  });
  // Nested interactive scan: any button/button, button/a, a/button, a/a.
  const interactive = (node) => node.tagName === "BUTTON" || node.tagName === "A";
  const nested = [];
  (function scan(node) {
    if (!node) return;
    if (interactive(node)) {
      (node.children || []).forEach((child) => {
        if (interactive(child)) nested.push(node.tagName + ">" + child.tagName + ":" + String(child.className));
        scan(child);
      });
    } else {
      (node.children || []).forEach(scan);
    }
  })(host);
  process.stdout.write(JSON.stringify({
    nestedCount: nested.length,
    nested,
    witnessButtons: rows.filter((node) => String(node.className).includes("rm-arch__edge-jump")).length,
  }));
}).catch((error) => {
  process.stdout.write(JSON.stringify({ mountError: String(error && error.stack || error) }));
  process.exit(2);
});
`
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run association nesting check: %v\n%s", err, output)
	}
	var result struct {
		NestedCount   int      `json:"nestedCount"`
		Nested        []string `json:"nested"`
		WitnessButtons int     `json:"witnessButtons"`
		MountError    string   `json:"mountError"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("parse runner output: %v\n%s", err, output)
	}
	if result.MountError != "" {
		t.Fatalf("canvas mount failed: %s", result.MountError)
	}
	if result.NestedCount != 0 {
		t.Fatalf("nested interactive elements = %d (%s); must be zero in the expanded association state", result.NestedCount, strings.Join(result.Nested, ", "))
	}
	if result.WitnessButtons < 2 {
		t.Fatalf("expected at least 2 witness buttons, got %d", result.WitnessButtons)
	}
}
