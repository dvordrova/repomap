package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTaskLensWorkspaceUsesTaskRouteAndKeepsSourceProgressivelyDisclosed(t *testing.T) {
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
  removeAttribute(name) { delete this.attributes[name]; }
	  appendChild(child) { this.children.push(child); return child; }
	  append(...children) { this.children.push(...children); }
	  replaceChildren(...children) { this.children = children; }
	  scrollIntoView() { this.scrolled = true; }
	  focus() { this.focused = true; }
	}
function source(path, symbol, start) {
  return {
    path, enclosing_symbol: symbol, start_line: start, end_line: start + 2,
    highlight_ranges: [{ start_line: start, end_line: start }],
    content_sha256: "a".repeat(64), presentation_sha256: "b".repeat(64),
    lines: [
      { line: start, text: "RAW_SOURCE_SHOULD_STAY_FOLDED", highlight: true },
      { line: start + 1, text: "  inspect()" },
      { line: start + 2, text: "}" },
    ],
  };
}
const anchors = [
  { path: "api.go", symbol: "Handle", role: "symptom_site", start_line: 10, end_line: 12, why: "Observed request boundary.", source: source("api.go", "Handle", 10) },
  { path: "validate.go", symbol: "Validate", role: "representative_implementation", start_line: 20, end_line: 22, why: "Owns validation.", source: source("validate.go", "Validate", 20) },
  { path: "validate_test.go", symbol: "TestValidate", role: "verification_anchor", start_line: 30, end_line: 32, why: "Checks the effect.", source: source("validate_test.go", "TestValidate", 30) },
];
const taskID = "task-1234567890abcdef";
const report = {
  repo_name: "example.test/fuego", openable_paths: anchors.map((anchor) => anchor.path),
  source_ids: {}, source_context_ids: {}, user_sources: [],
  user_mechanisms: [{ artifact_id: "generic", title: "GENERIC_ORIENTATION_SHOULD_NOT_RENDER", steps: [{ title: "Start here" }] }],
  project_guess: "GENERIC_ORIENTATION_SHOULD_NOT_RENDER",
  task_investigation: {
    task_id: taskID,
    task: "Fix request validation.",
    state: "accepted", sufficient: true, locality: "bounded_cross_file",
	    interpretation: {
	      restatement: "Trace request validation.", task_kind: "bug",
	      observable_or_outcome: "The request returns the intended result.",
	      repository_terms_found: ["Validate"],
	      user_provided_only_terms: ["connection closes"],
	    },
    likely_areas: [
      { label: "Request validation", why: "The boundary delegates here.", anchor_indexes: [0, 1] },
      { label: "Validation test", why: "The retained test observes the effect.", anchor_indexes: [2] },
    ],
    anchors,
	    evidence_joins: [
	      { left_anchor: 0, right_anchor: 1, support_anchor_indexes: [0, 1], kind: "direct_call_expression", support: "locally_observed", explanation: "The call is present in saved source.", scope_non_guarantees: "Static source does not prove every runtime branch." },
	      { left_anchor: 1, right_anchor: 2, support_anchor_indexes: [1, 2], kind: "shared_task_term", support: "unresolved", explanation: "Both anchors contain one selected task term.", scope_non_guarantees: "Text overlap does not establish behavior." },
	    ],
	    working_hypothesis: [{ status: "supported", text: "Handle contains a direct Validate call.", support_anchor_indexes: [0, 1] }],
	    reproduce_or_observe: [{ text: "Use the reported request.", authority: "task_provided" }],
	    verify: { effect_to_observe: "The request succeeds.", steps: [{ text: "Run the focused test.", authority: "repository_test_or_example", support_anchor_indexes: [2] }] },
	    next_probes: [{ action: "resolve_reference", anchor_indexes: [1], text: "Resolve callers outside this window." }],
	    stages_skipped: ["orientation", "architecture", "study_map"],
	    budget: { read_files: 3, read_bytes: 256 }, provider: { calls: 1 },
	    warnings: ["Evidence remains bounded to the retained source windows."],
	  },
	};

const roots = { "rm-task-investigation": new Element("section") };
const window = {
  location: { search: "", hash: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
	  createElement(tag) { return new Element(tag); },
	  getElementById(id) {
	    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
	    if (roots[id]) return roots[id];
	    const pending = Object.values(roots).slice();
	    while (pending.length) {
	      const candidate = pending.shift();
	      if (candidate && candidate.id === id) return candidate;
	      pending.push(...(candidate && candidate.children || []));
	    }
	    return null;
  },
  querySelectorAll() { return []; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function text(node) { return String(node && node.textContent || "") + (node && node.children || []).map(text).join(""); }
function descendants(node) { return [node].concat((node && node.children || []).flatMap(descendants)); }
api.renderTaskInvestigationWorkspace();
const root = roots["rm-task-investigation"];
	const rendered = text(root);
	const buttonNodes = descendants(root).filter((node) => node.tagName === "button");
	const citation = buttonNodes.find((node) => String(node.className).includes("rm-task-citation") && text(node).includes("Anchor 2"));
	if (citation && typeof citation.onclick === "function") citation.onclick();
	const buttons = buttonNodes.map(text);
	const scrolledAnchorIDs = descendants(root).filter((node) => node.scrolled).map((node) => node.id);
const exactRoute = api.parseWorkspaceHash("#/investigate/" + taskID, [], null);
const defaultRoute = api.parseWorkspaceHash("", [], null);
const overviewRoute = api.parseWorkspaceHash("#/overview", [], null);
const restoredSource = api.embeddedSourceForLocation({ path: "validate.go", line: 20 });
process.stdout.write(JSON.stringify({
	  rendered, buttons, exactRoute, defaultRoute, overviewRoute,
	  restoredSourcePath: restoredSource && restoredSource.path || "", scrolledAnchorIDs,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "task-lens-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Task Lens workspace smoke: %v\n%s", err, output)
	}
	var got struct {
		Rendered   string   `json:"rendered"`
		Buttons    []string `json:"buttons"`
		ExactRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View   string `json:"view"`
				TaskID string `json:"taskID"`
			} `json:"state"`
		} `json:"exactRoute"`
		DefaultRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
		} `json:"defaultRoute"`
		OverviewRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
		} `json:"overviewRoute"`
		RestoredSourcePath string   `json:"restoredSourcePath"`
		ScrolledAnchorIDs  []string `json:"scrolledAnchorIDs"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Task Lens workspace smoke: %v\n%s", err, output)
	}
	const taskHash = "#/investigate/task-1234567890abcdef"
	if !got.ExactRoute.Valid || got.ExactRoute.CanonicalHash != taskHash ||
		got.ExactRoute.State.View != "investigate" || got.ExactRoute.State.TaskID == "" {
		t.Fatalf("exact Task Lens route = %#v", got.ExactRoute)
	}
	if !got.DefaultRoute.Valid || got.DefaultRoute.CanonicalHash != taskHash {
		t.Fatalf("default Task Lens route = %#v", got.DefaultRoute)
	}
	if got.OverviewRoute.Valid || got.OverviewRoute.CanonicalHash != taskHash {
		t.Fatalf("generic overview remained reachable in task mode: %#v", got.OverviewRoute)
	}
	for _, expected := range []string{
		"Trace request validation.",
		"The request returns the intended result.",
		"Request validation",
		"Validation test",
		"Found in repository evidence",
		"Task-provided only",
		"connection closes",
		"Limits to keep in view",
		"Evidence remains bounded to the retained source windows.",
		"What the bounded evidence supports",
		"Files and symbols to inspect",
		"How the selected anchors connect",
		"Handle → Direct Call Expression → Validate",
		"Validate ↔ Shared Task Term ↔ TestValidate",
		"Supporting anchors",
		"Collect the relevant signal",
		"Confirm the intended effect",
		"What remains unresolved",
	} {
		if !strings.Contains(got.Rendered, expected) {
			t.Errorf("task workspace is missing %q: %s", expected, got.Rendered)
		}
	}
	for _, forbidden := range []string{
		"RAW_SOURCE_SHOULD_STAY_FOLDED",
		"GENERIC_ORIENTATION_SHOULD_NOT_RENDER",
		"Start here",
		"Recommended next step",
		"Inspect recommended anchor",
	} {
		if strings.Contains(got.Rendered, forbidden) {
			t.Errorf("task-first projection exposed %q: %s", forbidden, got.Rendered)
		}
	}
	if countString(got.Buttons, "Show source") != 3 ||
		countString(got.Buttons, "Anchor 3 · TestValidate") == 0 {
		t.Fatalf("task source actions = %#v", got.Buttons)
	}
	if !slices.Equal(got.ScrolledAnchorIDs, []string{"rm-task-anchor-2"}) {
		t.Fatalf("task citation did not scroll to its retained anchor: %#v", got.ScrolledAnchorIDs)
	}
	if got.RestoredSourcePath != "validate.go" {
		t.Fatalf("task source was not available to the shared drawer: %q", got.RestoredSourcePath)
	}
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
