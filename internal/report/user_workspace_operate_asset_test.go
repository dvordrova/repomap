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

func TestUserWorkspaceOperateRoutesCopySourceAndStudy(t *testing.T) {
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
    this.dataset = {};
    this.hidden = false;
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childNodes() { return this.children; }
  get childElementCount() { return this.children.length; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  removeAttribute(name) { delete this.attributes[name]; }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  prepend(child) { this.children.unshift(child); }
  replaceChildren(...children) { this.children = children; }
  querySelector() { return null; }
}

function snippet(path, line, text) {
  return {
    path, language: "text", start_line: line, end_line: line,
    content_sha256: "a".repeat(64), presentation_sha256: (path === "README.md" ? "b" : "c").repeat(64),
    lines: [{ line, text }],
  };
}
function text(node) {
  return String(node && node.textContent || "") + (node && node.children || []).map(text).join("");
}
function descendants(node) {
  return [node].concat((node && node.children || []).flatMap(descendants));
}

const normalSource = snippet("README.md", 12, "go run ./cmd/server --listen 127.0.0.1:8080");
const redactedSource = snippet("config.example", 3, "TOKEN=[redacted]");
const normalReference = {
  label: "Development server", role: "documented_procedure", redacted: false,
  location: { path: "README.md", line: 12 }, source: normalSource,
};
const redactedReference = {
  label: "Environment", role: "environment_declaration", redacted: true,
  location: { path: "config.example", line: 3 }, source: redactedSource,
};
const operation = {
  id: "operate/server", title: "Run the development server",
  goal: "Start the local server and check its health endpoint.",
  ordering_basis: "documented_procedure",
  prerequisites: [redactedReference],
  actions: [
    {
      instruction: "Start the server from the repository root.",
      command: "go run ./cmd/server --listen 127.0.0.1:8080",
      copy_text: "go run ./cmd/server --listen 127.0.0.1:8080",
      reference: normalReference,
    },
    {
      instruction: "Open the health endpoint.",
      endpoint: "http://127.0.0.1:8080/health",
      reference: normalReference,
    },
  ],
  expected_results: [{
    kind: "generated_artifact", value: "./server", after_action: 1,
    result_evidence_ids: ["operational-evidence-result"], reference: normalReference,
  }],
  expected: [normalReference], troubleshooting: [normalReference],
  related_study_direction_ids: ["study-run"],
};
const study = {
  id: "study-run", question: "How is the development server wired?",
  why_it_matters: "This connects the documented command to its implementation.",
  learning_outcome: "You can locate the server entrypoint.",
  reading_anchors: [
    { label: "Start here", what_to_look_for: "Inspect the command.", location: normalReference.location, source: normalSource },
    { label: "Then inspect", what_to_look_for: "Inspect the endpoint.", location: normalReference.location, source: normalSource },
    { label: "Related implementation", what_to_look_for: "Inspect configuration.", location: normalReference.location, source: normalSource },
  ],
};
const report = {
  repo_name: "fixture", user_mechanisms: [], user_sources: [],
  openable_paths: ["README.md", "config.example"], source_ids: {}, source_context_ids: {},
  study_map: {
    brief: { what_it_is: "A development server fixture." },
    shape: [], directions: [study],
  },
  operations: { version: 1, paths: [operation], landmarks: [] },
};

const roots = {
  "rm-overview": new Element("section"),
  "rm-study-detail": new Element("section"),
  "rm-operate-detail": new Element("section"),
  "rm-toast": new Element("div"),
};
const entries = [{ hash: "#/overview", state: null }];
let historyIndex = 0;
let copied = "";
const navigator = {
  clipboard: { writeText(value) { copied = value; return Promise.resolve(); } },
};
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {}, setTimeout(fn) { fn(); return 1; }, clearTimeout() {},
};
window.history = {
  get state() { return entries[historyIndex].state; },
  pushState(state, _title, hash) {
    entries.splice(historyIndex + 1);
    entries.push({ hash, state });
    historyIndex = entries.length - 1;
    window.location.hash = hash;
  },
  replaceState(state, _title, hash) {
    entries[historyIndex] = { hash, state };
    window.location.hash = hash;
  },
  back() {
    if (historyIndex > 0) historyIndex--;
    window.location.hash = entries[historyIndex].hash;
  },
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return roots[id] || null;
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
};

vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, navigator, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
const validRoute = api.parseWorkspaceHash("#/operate/operate%2Fserver", [], null);
const invalidRoute = api.parseWorkspaceHash("#/operate/missing", [], null);

api.renderOverviewWorkspace();
const overviewText = text(roots["rm-overview"]);
api.openPavedPath("operate/server");
const openedHash = window.location.hash;
const openedState = api.workspaceStateSnapshot();
const detail = roots["rm-operate-detail"];
const detailText = text(detail);
const detailNodes = descendants(detail);
const buttonTexts = detailNodes.filter((node) => node.tagName === "button").map(text);
const copyButtons = detailNodes.filter((node) => node.tagName === "button" && text(node) === "Copy command");
if (copyButtons[0]) copyButtons[0].onclick();

const sourceCards = detailNodes.filter((node) => String(node.className).split(/\s+/).includes("rm-source-card"));
const redactedCard = sourceCards.find((card) => text(card).includes("TOKEN=[redacted]"));
const redactedButtons = descendants(redactedCard).filter((node) => node && node.tagName === "button").map(text);

api.openSourceSnippet(normalSource, normalReference.location, false);
const sourceHash = window.location.hash;
const sourceState = api.workspaceStateSnapshot();
api.closeSourceDrawer();
api.restoreWorkspaceFromRoute();
const closedState = api.workspaceStateSnapshot();
const closedHash = window.location.hash;

const relatedButton = descendants(roots["rm-operate-detail"]).find((node) =>
  node.tagName === "button" && text(node) === study.question
);
if (relatedButton) relatedButton.onclick();
const relatedHash = window.location.hash;
api.openSemanticSearchTarget({ kind: "paved_path", paved_path_id: "operate/server" });

process.stdout.write(JSON.stringify({
  validRoute, invalidRoute, overviewText, openedHash, openedState, detailText,
  buttonTexts, copyButtonCount: copyButtons.length, copied,
  redactedButtons, sourceHash, sourceState, closedState, closedHash,
  relatedHash, searchHash: window.location.hash,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "operate-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run operate workspace smoke: %v\n%s", err, output)
	}
	var got struct {
		ValidRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
			State         struct {
				View        string `json:"view"`
				OperationID string `json:"operationID"`
			} `json:"state"`
		} `json:"validRoute"`
		InvalidRoute struct {
			Valid         bool   `json:"valid"`
			CanonicalHash string `json:"canonicalHash"`
		} `json:"invalidRoute"`
		OverviewText    string   `json:"overviewText"`
		OpenedHash      string   `json:"openedHash"`
		DetailText      string   `json:"detailText"`
		ButtonTexts     []string `json:"buttonTexts"`
		CopyButtonCount int      `json:"copyButtonCount"`
		Copied          string   `json:"copied"`
		RedactedButtons []string `json:"redactedButtons"`
		SourceHash      string   `json:"sourceHash"`
		ClosedHash      string   `json:"closedHash"`
		RelatedHash     string   `json:"relatedHash"`
		SearchHash      string   `json:"searchHash"`
		OpenedState     struct {
			View        string `json:"view"`
			OperationID string `json:"operationID"`
		} `json:"openedState"`
		SourceState struct {
			View           string `json:"view"`
			OperationID    string `json:"operationID"`
			SourceLocation any    `json:"sourceLocation"`
		} `json:"sourceState"`
		ClosedState struct {
			View           string `json:"view"`
			OperationID    string `json:"operationID"`
			SourceLocation any    `json:"sourceLocation"`
		} `json:"closedState"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode operate workspace smoke: %v\n%s", err, output)
	}
	if !got.ValidRoute.Valid || got.ValidRoute.CanonicalHash != "#/operate/operate%2Fserver" ||
		got.ValidRoute.State.View != "operate" || got.ValidRoute.State.OperationID != "operate/server" {
		t.Fatalf("valid operation route = %#v", got.ValidRoute)
	}
	if got.InvalidRoute.Valid || got.InvalidRoute.CanonicalHash != "#/overview" {
		t.Fatalf("invalid operation route = %#v", got.InvalidRoute)
	}
	if strings.Index(got.OverviewText, "What to study") < 0 ||
		strings.Index(got.OverviewText, "What to study") > strings.Index(got.OverviewText, "How to run and verify") {
		t.Fatalf("overview section order = %q", got.OverviewText)
	}
	if got.OpenedHash != "#/operate/operate%2Fserver" || got.OpenedState.View != "operate" ||
		got.OpenedState.OperationID != "operate/server" {
		t.Fatalf("opened operation = hash %q state %#v", got.OpenedHash, got.OpenedState)
	}
	for _, required := range []string{
		"Run the development server", "Start the local server and check its health endpoint.",
		"go run ./cmd/server --listen 127.0.0.1:8080", "http://127.0.0.1:8080/health",
		"TOKEN=[redacted]", "How is the development server wired?", "Expected result",
		"After action 1", "Generated artifact", "./server",
	} {
		if !strings.Contains(got.DetailText, required) {
			t.Errorf("operation detail missing %q: %q", required, got.DetailText)
		}
	}
	for _, forbidden := range []string{
		"documented_procedure", "safe_to_copy", "operate/server", "operational-evidence-result",
		"provider", "verdict",
	} {
		if strings.Contains(got.DetailText, forbidden) {
			t.Errorf("operation detail exposed internal text %q", forbidden)
		}
	}
	if got.CopyButtonCount != 1 || got.Copied != "go run ./cmd/server --listen 127.0.0.1:8080" {
		t.Fatalf("copy action = count %d copied %q", got.CopyButtonCount, got.Copied)
	}
	for _, label := range got.ButtonTexts {
		if label == "Run" || label == "Execute" {
			t.Fatalf("operation detail exposed execution action %q", label)
		}
	}
	if !slices.Contains(got.ButtonTexts, "Show source") {
		t.Fatalf("expected result source action is missing: %#v", got.ButtonTexts)
	}
	if len(got.RedactedButtons) != 0 {
		t.Fatalf("redacted source exposed navigation actions: %#v", got.RedactedButtons)
	}
	if got.SourceHash != "#/operate/operate%2Fserver" || got.SourceState.View != "operate" ||
		got.SourceState.OperationID != "operate/server" || got.SourceState.SourceLocation == nil {
		t.Fatalf("operation source drawer changed route/context: hash %q state %#v", got.SourceHash, got.SourceState)
	}
	if got.ClosedHash != "#/operate/operate%2Fserver" || got.ClosedState.View != "operate" ||
		got.ClosedState.OperationID != "operate/server" || got.ClosedState.SourceLocation != nil {
		t.Fatalf("closing operation source did not restore route/context: hash %q state %#v", got.ClosedHash, got.ClosedState)
	}
	if got.RelatedHash != "#/study/study-run" {
		t.Fatalf("related Study route = %q", got.RelatedHash)
	}
	if got.SearchHash != "#/operate/operate%2Fserver" {
		t.Fatalf("paved path Search route = %q", got.SearchHash)
	}
}
