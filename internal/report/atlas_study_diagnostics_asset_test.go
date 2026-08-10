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
// templates/script.js + templates/ui_messages.js in Node. The fixture carries
// rich internal diagnostics on purpose; the product contract is that Study
// renders only navigable themes and exact readings, while provider failure and
// pipeline accounting stay out of the generated report.
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
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 4,
    content_sha256: "a".repeat(64), presentation_sha256: ("" + line).padStart(64, "0"),
    lines: [
      { line, text: "func " + symbol + "() {", highlight: true },
      { line: line + 1, text: "  inspect()" },
      { line: line + 2, text: "}" },
    ],
  };
}
const paths = [];
const themeCards = [1, 1].map((count, themeIndex) => {
  const readings = [];
  for (let readingIndex = 0; readingIndex < count; readingIndex++) {
    const path = "study/route-" + (themeIndex + 1) + ".go";
    const line = 10 + readingIndex;
    paths.push(path);
    readings.push({
      label: "Start here",
      symbol: "Read" + (themeIndex + 1),
      path,
      line,
      // Decision 224 (D219 C/G): per-reading role and bounded model
      // observation render next to the exact source row.
      role: readingIndex % 2 === 0 ? "direct" : "supporting",
      supported_observation: "Inspect exact reading " + (readingIndex + 1) + ".",
      source: snippet(path, "Read" + (themeIndex + 1), line),
    });
  }
  return {
    ordinal: themeIndex + 1,
    final_title: "Theme " + (themeIndex + 1),
    final_question: "Study question " + (themeIndex + 1) + "?",
    why_it_matters: "Reason " + (themeIndex + 1) + ".",
    expected_learning: "Outcome " + (themeIndex + 1) + ".",
    theme_kind: "sibling_implementation_family",
    badge: "editorial_source_backed",
    readings,
  };
});
function browseSpan(ordinal, stage, themeRefs, openable, symbol) {
  const path = openable ? "study/route-" + ((ordinal % 2) + 1) + ".go" : "hidden/internal/unopenable.go";
  const span = {
    ordinal,
    title: symbol || ("Sym" + ordinal),
    question: "Browse question " + ordinal + "?",
    stage,
    source: { path, line: 10 + (ordinal % 5) },
  };
  if (themeRefs) span.theme_refs = themeRefs;
  return span;
}
const browseSpans = [];
let browseOrdinal = 1;
for (let i = 0; i < 10; i++) browseSpans.push(browseSpan(browseOrdinal++, "published", [(i % 2) + 1], true));
for (let i = 0; i < 22; i++) browseSpans.push(browseSpan(browseOrdinal++, "seed_advertised", null, true));
for (let i = 0; i < 36; i++) browseSpans.push(browseSpan(browseOrdinal++, "considered", null, i < 34, "Local" + i));
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en", user_mechanisms: [], user_topics: [], user_sources: [],
  openable_paths: paths, source_ids: {},
  github_source_links: {
    repository_url: "https://github.com/example/repository",
    revision: "a".repeat(40),
  },
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
  atlas_study: {
    version: 1, projection_version: 9, state: "accepted",
    considered_span_count: 68,
    advertised_span_count: 32,
    model_selected_span_count: 10,
    accepted_span_count: 10,
    frontier_complete: false,
    selected_items_complete: true,
    support_coverage_complete: true,
    portfolio_target_met: true,
    omissions: [{ reason: "seed_budget", count: 36, representative_count: 12 }],
    themes: { total: 2, shown: 2, cards: themeCards },
    frontier_browse: { total: 68, shown: 68, spans: browseSpans },
  },
  architecture_synthesis: { state: "failed", error_code: "provider_output_limit" },
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
function journey(report, lang) {
  report = Object.assign({}, report, { report_language: lang });
  const roots = {};
["rm-task-investigation", "rm-operate-detail", "rm-architecture", "rm-provenance"].forEach((id) => {
  roots[id] = new Element("section");
  roots[id].className = "rm-tab-content" + (id === "rm-architecture" ? " rm-active" : "");
});
roots["rm-tabs"] = new Element("nav");
roots["rm-source-drawer"] = new Element("aside");
roots["rm-source-drawer"].hidden = true;
roots["rm-source-drawer-content"] = new Element("div");
roots["rm-source-drawer-close"] = new Element("button");
const workspace = new Element("main");
const window = {
  location: { search: "", hash: "#canvas", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
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
    if (roots[id]) return roots[id];
    return Object.values(roots).flatMap((root) => walk(root)).find((node) => node.id === id) || null;
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
    if (selector === ".rm-main-content > .rm-tab-content") return tabSections;
    if (selector === "[data-workspace-view]") {
      return walk(roots["rm-tabs"]).filter((node) => node.attributes && node.attributes["data-workspace-view"]);
    }
    return [];
  },
};
document.documentElement = { lang: report.report_language };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.renderWorkspaceTabs();
api.renderArchitectureWorkspace();
const overviewText = "";
const studyRoot = byClass(roots["rm-architecture"], "rm-target-study")[0];
const studyOverviewText = text(studyRoot);
const themeShelf = byClass(studyRoot, "rm-study-theme-shelf")[0] || null;
const themeCards = byClass(studyRoot, "rm-study-theme-card");
const contents = byClass(studyRoot, "rm-study-theme-contents")[0] || null;
const contentActions = byClass(studyRoot, "rm-study-theme-contents__action");
const openActions = byClass(studyRoot, "rm-study-theme-card__open");
const previewActions = byClass(studyRoot, "rm-study-reading-anchor__open").map((node) => ({
  tag: String(node.tagName || "").toLowerCase(),
  href: (node.attributes && node.attributes.href) || "",
  target: (node.attributes && node.attributes.target) || "",
  rel: (node.attributes && node.attributes.rel) || "",
  ariaLabel: (node.attributes && node.attributes["aria-label"]) || "",
}));
const removedChromeCount = [
  "rm-study-diagnostics", "rm-study-frontier-browse", "rm-study-browse-show-all",
  "rm-study-diagnostics-show-all", "rm-study-theme-card__evidence",
  "rm-study-theme-card__scope", "rm-study-theme-card__concentration",
  "rm-study-theme-card__more",
].reduce((count, className) => count + byClass(studyRoot, className).length, 0);
const detailsCount = walk(studyRoot).filter((node) => String(node.tagName || "").toLowerCase() === "details").length;
if (contentActions.length) contentActions[0].onclick();
const detailRoot = themeCards[0];
const detailText = text(detailRoot);
const detailReadings = byClass(detailRoot, "rm-study-reading-anchor__open");
const readingJump = detailReadings.length ? {
  tag: String(detailReadings[0].tagName || "").toLowerCase(),
  href: (detailReadings[0].attributes && detailReadings[0].attributes.href) || "",
  target: (detailReadings[0].attributes && detailReadings[0].attributes.target) || "",
  rel: (detailReadings[0].attributes && detailReadings[0].attributes.rel) || "",
  ariaLabel: (detailReadings[0].attributes && detailReadings[0].attributes["aria-label"]) || "",
} : null;
const readingRoleBadgeCount = byClass(detailRoot, "rm-study-theme-card__reading-role").length;
const readingExplainCount = byClass(detailRoot, "rm-study-theme-card__reading-explain").length;
const architectureText = text(roots["rm-architecture"]);
  return {
    overviewText, studyOverviewText, architectureText, detailText,
    themeShelfPresent: !!themeShelf, themeCardCount: themeCards.length,
    contentCount: contentActions.length,
    contentLabel: contents && contents.getAttribute("aria-label"),
    openActionCount: openActions.length, previewActions,
    removedChromeCount, detailsCount, readingJump,
    readingRoleBadgeCount, readingExplainCount,
  };
}
const strippedReport = JSON.parse(JSON.stringify(report));
strippedReport.user_sources = [];
strippedReport.openable_paths = ["study/route-1.go", "study/route-2.go"];
strippedReport.github_source_links = { repository_url: "https://github.com/example/repository", revision: "a".repeat(40) };
process.stdout.write(JSON.stringify({ en: journey(report, "en"), ru: journey(report, "ru"), stripped: journey(strippedReport, "en") }));
`
	runnerPath := filepath.Join(t.TempDir(), "atlas-study-diagnostics-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	type sourceAction struct {
		Tag       string `json:"tag"`
		Href      string `json:"href"`
		Target    string `json:"target"`
		Rel       string `json:"rel"`
		AriaLabel string `json:"ariaLabel"`
	}
	type journey struct {
		OverviewText          string         `json:"overviewText"`
		StudyOverviewText     string         `json:"studyOverviewText"`
		ArchitectureText      string         `json:"architectureText"`
		DetailText            string         `json:"detailText"`
		ThemeShelfPresent     bool           `json:"themeShelfPresent"`
		ThemeCardCount        int            `json:"themeCardCount"`
		ContentCount          int            `json:"contentCount"`
		ContentLabel          string         `json:"contentLabel"`
		OpenActionCount       int            `json:"openActionCount"`
		PreviewActions        []sourceAction `json:"previewActions"`
		RemovedChromeCount    int            `json:"removedChromeCount"`
		DetailsCount          int            `json:"detailsCount"`
		ReadingJump           *sourceAction  `json:"readingJump"`
		ReadingRoleBadgeCount int            `json:"readingRoleBadgeCount"`
		ReadingExplainCount   int            `json:"readingExplainCount"`
	}
	type journeySet struct {
		En       journey `json:"en"`
		Ru       journey `json:"ru"`
		Stripped journey `json:"stripped"`
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run diagnostics workspace: %v\n%s", err, output)
	}
	var out journeySet
	if err := json.Unmarshal(output, &out); err != nil {
		t.Fatalf("decode diagnostics workspace: %v\n%s", err, output)
	}
	en, ru, stripped := out.En, out.Ru, out.Stripped

	for name, j := range map[string]journey{"en": en, "ru": ru, "stripped": stripped} {
		if !j.ThemeShelfPresent || j.ThemeCardCount != 2 || j.ContentCount != 2 || j.OpenActionCount != 0 {
			t.Fatalf("%s Study navigation = shelf=%v cards=%d contents=%d open=%d, want true/2/2/0\n%s",
				name, j.ThemeShelfPresent, j.ThemeCardCount, j.ContentCount, j.OpenActionCount, j.StudyOverviewText)
		}
		wantContents := "Contents"
		if name == "ru" {
			wantContents = "Содержание"
		}
		if j.ContentLabel != wantContents {
			t.Fatalf("%s contents aria-label = %q, want %q", name, j.ContentLabel, wantContents)
		}
		if j.RemovedChromeCount != 0 || j.DetailsCount != 2 {
			t.Fatalf("%s internal Study chrome leaked: removed=%d details=%d\n%s",
				name, j.RemovedChromeCount, j.DetailsCount, j.StudyOverviewText)
		}
		if len(j.PreviewActions) != 2 {
			t.Fatalf("%s exact reading actions = %d, want 2", name, len(j.PreviewActions))
		}
		for index, action := range j.PreviewActions {
			if action.Tag != "a" || !strings.Contains(action.Href, "github.com/example/repository/blob/") ||
				!strings.Contains(action.Href, "#L") || action.Target != "_blank" ||
				action.Rel != "noopener noreferrer" || action.AriaLabel == "" {
				t.Fatalf("%s preview action %d is not one pinned, labelled source jump: %#v", name, index+1, action)
			}
		}
		if j.ReadingJump == nil || j.ReadingJump.Tag != "a" ||
			!strings.Contains(j.ReadingJump.Href, "github.com/example/repository/blob/") ||
			j.ReadingJump.Target != "_blank" || j.ReadingJump.Rel != "noopener noreferrer" ||
			j.ReadingJump.AriaLabel == "" {
			t.Fatalf("%s opened-theme reading jump = %#v", name, j.ReadingJump)
		}
		if j.ReadingRoleBadgeCount != 0 || j.ReadingExplainCount != 1 ||
			!strings.Contains(j.DetailText, "Study question 1?") ||
			!strings.Contains(j.DetailText, "study/route-1.go:10") {
			t.Fatalf("%s contents action did not open the useful direct-only theme detail without a redundant role pill: role=%d explanation=%d\n%s",
				name, j.ReadingRoleBadgeCount, j.ReadingExplainCount, j.DetailText)
		}
		for _, forbidden := range []string{
			"Study diagnostics", "All study questions", "Considered spans", "Model-selected spans",
			"Show all 36", "seed_budget", "Every question the local analysis can answer",
		} {
			if strings.Contains(j.StudyOverviewText, forbidden) {
				t.Fatalf("%s Study leaked internal accounting %q:\n%s", name, forbidden, j.StudyOverviewText)
			}
		}
	}

	if !strings.Contains(en.StudyOverviewText, "Topics") ||
		!strings.Contains(en.StudyOverviewText, "Theme 1") ||
		!strings.Contains(en.StudyOverviewText, "Theme 2") ||
		!strings.Contains(en.StudyOverviewText, "Study question 1?") ||
		!strings.Contains(en.StudyOverviewText, "study/route-1.go:10") ||
		!strings.Contains(en.StudyOverviewText, "study/route-2.go:10") {
		t.Fatalf("English Study lost its themes or exact readings:\n%s", en.StudyOverviewText)
	}
	if !strings.Contains(ru.StudyOverviewText, "Темы") ||
		!strings.Contains(ru.StudyOverviewText, "Содержание") ||
		!strings.Contains(ru.StudyOverviewText, "Theme 1") ||
		!strings.Contains(ru.StudyOverviewText, "study/route-1.go:10") {
		t.Fatalf("Russian Study lost its themes, contents, or exact readings:\n%s", ru.StudyOverviewText)
	}

	// Provider failure belongs to the ordinary console. The generated HTML is
	// calm user documentation and keeps only the useful local product surface.
	if strings.Contains(en.ArchitectureText, "Architecture synthesis failed") ||
		strings.Contains(en.ArchitectureText, "model exceeded its response budget") {
		t.Fatalf("provider failure leaked into generated HTML:\noverview: %s\narchitecture: %s", en.OverviewText, en.ArchitectureText)
	}
	if strings.Contains(ru.ArchitectureText, "модель исчерпала лимит ответа") {
		t.Fatalf("provider failure leaked into RU generated HTML:\n%s", ru.ArchitectureText)
	}
}
