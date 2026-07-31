package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestImplementationDetailRendererIncludesAttachedExactReferences(t *testing.T) {
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
  constructor(tag) { this.tagName = tag; this.className = ""; this.textContent = ""; this.children = []; }
  setAttribute() {}
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
const report = { user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {} };
const window = {
  location: { search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const source = {
  path: "replica.go", enclosing_symbol: "Replica.monitor", start_line: 428, end_line: 432,
  lines: [{ line: 428, text: "func monitor() {" }, { line: 430, text: "syncOnce()", highlight: true }],
};
const mechanism = {
  steps: [{ title: "Sync trigger", explanation: "The monitor calls sync once.", locations: [{ path: "replica.go", line: 430 }], sources: [source] }],
  phases: [{
    title: "Sync trigger", explanation: "The monitor calls sync once.", sources: [source],
    implementation_step_indexes: [0],
    implementation_details: [{
      title: "Sync execution internals",
      explanation: "The replica sync function calls sync once and returns its error.",
      locations: [{ path: "replica.go", line: 145 }, { path: "replica.go", line: 146 }],
    }],
  }],
};
function text(node) { return String(node && node.textContent || "") + (node && node.children || []).map(text).join(""); }
const items = api.mechanismImplementationSteps(mechanism, mechanism.phases[0]);
const rendered = api.renderImplementationDetails(mechanism, mechanism.phases[0]);
process.stdout.write(JSON.stringify({ titles: items.map((item) => item.title), text: text(rendered) }));
`
	runnerPath := filepath.Join(t.TempDir(), "implementation-details-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run implementation detail renderer: %v\n%s", err, output)
	}
	var got struct {
		Titles []string `json:"titles"`
		Text   string   `json:"text"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode renderer result: %v\n%s", err, output)
	}
	if strings.Join(got.Titles, "|") != "Sync trigger|Sync execution internals" {
		t.Fatalf("implementation details = %#v", got.Titles)
	}
	for _, token := range []string{
		"Show implementation details (2)", "Sync execution internals",
		"The replica sync function calls sync once and returns its error.",
		"replica.go:145", "replica.go:146", "Show code",
	} {
		if !strings.Contains(got.Text, token) {
			t.Fatalf("rendered implementation details are missing %q: %q", token, got.Text)
		}
	}
}
