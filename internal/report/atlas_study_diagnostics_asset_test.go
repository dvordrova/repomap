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
// templates/script.js + templates/ui_messages.js in Node and verifies the D211
// HOLD-repair UI contract: the four-stage diagnostics panel renders the stage
// counts, the four independent flags (including false values), and the bounded
// omission aggregates; and the Overview/Architecture surfaces show an honest
// synthesis-failed notice instead of the unconditional acceptance copy.
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
const directions = [1, 1].map((count, directionIndex) => {
  const readings = [];
  for (let readingIndex = 0; readingIndex < count; readingIndex++) {
    const path = "study/route-" + (directionIndex + 1) + ".go";
    const line = 10 + readingIndex;
    paths.push(path);
    readings.push({
      label: "Start here",
      symbol: "Read" + (directionIndex + 1),
      what_to_look_for: "Inspect exact reading " + (directionIndex + 1) + ".",
      location: { path, line },
      source: snippet(path, "Read" + (directionIndex + 1), line),
    });
  }
  return {
    id: "study-route-" + (directionIndex + 1),
    question: "Study question " + (directionIndex + 1) + "?",
    why_it_matters: "Reason " + (directionIndex + 1) + ".",
    learning_outcome: "Outcome " + (directionIndex + 1) + ".",
    principal_anchors: [{
      path: readings[0].location.path,
      symbol: readings[0].source.enclosing_symbol,
      line: readings[0].location.line,
    }],
    reading_anchors: readings,
  };
});
function browseSpan(ordinal, stage, directionID, openable, symbol) {
  const path = openable ? "study/route-" + ((ordinal % 2) + 1) + ".go" : "hidden/internal/unopenable.go";
  const span = {
    ordinal,
    title: symbol || ("Sym" + ordinal),
    question: "Browse question " + ordinal + "?",
    stage,
    source: { path, line: 10 + (ordinal % 5) },
  };
  if (directionID) span.direction_id = directionID;
  return span;
}
const browseSpans = [];
let browseOrdinal = 1;
for (let i = 0; i < 10; i++) browseSpans.push(browseSpan(browseOrdinal++, "accepted", "study-route-" + ((i % 2) + 1), true));
for (let i = 0; i < 22; i++) browseSpans.push(browseSpan(browseOrdinal++, "advertised", null, true));
for (let i = 0; i < 36; i++) browseSpans.push(browseSpan(browseOrdinal++, "considered", null, i < 34, "Local" + i));
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en", user_mechanisms: [], user_topics: [], user_sources: [],
  openable_paths: paths, source_ids: {},
  navigator: { version: 1, state: "empty" },
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
  study_map: {
    brief: {
      what_it_is: "A saved repository brief.", problem: "It solves a bounded problem.",
      main_input: "Input", central_responsibility: "Responsibility", observable_result: "Result",
      domain_terms: [{ term: "Fixture term", meaning: "Fixture meaning" }],
    },
    shape: [], directions,
  },
  atlas_study: {
    version: 8, projection_version: 7, state: "accepted",
    direction_count: 2,
    considered_span_count: 68,
    advertised_span_count: 32,
    model_selected_span_count: 10,
    accepted_span_count: 10,
    frontier_complete: false,
    selected_items_complete: true,
    support_coverage_complete: true,
    portfolio_target_met: true,
    omissions: [{ reason: "advertised_budget", count: 36, representative_count: 12 }],
    frontier_browse: { total: 68, shown: 68, spans: browseSpans },
  },
  architecture_synthesis: { state: "failed" },
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
const roots = {};
["rm-overview", "rm-task-investigation", "rm-mechanisms", "rm-mechanism-detail",
 "rm-study-overview", "rm-study-detail", "rm-operate-detail", "rm-architecture", "rm-provenance"].forEach((id) => {
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
api.renderOverviewWorkspace();
const overviewText = text(roots["rm-overview"]);
const nav = roots["rm-tabs"].children.slice();
const studyTab = nav.find((node) => node.attributes["data-workspace-view"] === "study_overview");
studyTab.onclick();
const studyOverviewText = text(roots["rm-study-overview"]);
const diagnosticsPanel = byClass(roots["rm-study-overview"], "rm-study-diagnostics")[0];
const stageCounts = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-stage").map((node) => text(node));
const flagItems = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-flag").map((node) => text(node));
const omissionItems = byClass(diagnosticsPanel || roots["rm-study-overview"], "rm-study-diagnostics-omission").map((node) => text(node));
const architectureTab = nav.find((node) => node.attributes["data-workspace-view"] === "architecture");
architectureTab.onclick();
const architectureText = text(roots["rm-architecture"]);
const browsePanel = byClass(roots["rm-study-overview"], "rm-study-frontier-browse")[0] || null;
const browseRoot = browsePanel || roots["rm-study-overview"];
const browseRows = byClass(browseRoot, "rm-study-browse-row");
const browseStageTexts = byClass(browseRoot, "rm-study-browse-row__stage").map((node) => text(node));
const unavailableRows = byClass(browseRoot, "rm-study-browse-row__unavailable").map((node) => text(node));
const modelPickBadges = byClass(browseRoot, "rm-study-browse-row__stage-accepted");
const localGroup = browsePanel ? browsePanel.querySelector(".rm-study-browse-group--local") : null;
const beyondBefore = localGroup ? byClass(localGroup, "rm-study-browse-row--beyond").length : 0;
const localCollapsedBefore = localGroup ? String(localGroup.className).split(/\s+/).includes("rm-study-browse-group--collapsed") : false;
const showAllButtons = byClass(browseRoot, "rm-study-browse-show-all");
let directionCardAfterPick = "";
if (modelPickBadges.length) {
  modelPickBadges[0].onclick();
  directionCardAfterPick = text(roots["rm-study-detail"]);
}
if (showAllButtons.length) showAllButtons[0].onclick();
const localCollapsedAfter = localGroup ? String(localGroup.className).split(/\s+/).includes("rm-study-browse-group--collapsed") : false;
process.stdout.write(JSON.stringify({ overviewText, studyOverviewText, stageCounts, flagItems, omissionItems, architectureText, browseRowCount: browseRows.length, browseStageTexts, unavailableRows, modelPickCount: modelPickBadges.length, directionCardAfterPick, beyondBefore, localCollapsedBefore, localCollapsedAfter, showAllCount: showAllButtons.length, browsePanelPresent: !!browsePanel }));
`
	runnerPath := filepath.Join(t.TempDir(), "atlas-study-diagnostics-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run diagnostics workspace: %v\n%s", err, output)
	}
	type journey struct {
		OverviewText         string   `json:"overviewText"`
		StudyOverviewText    string   `json:"studyOverviewText"`
		StageCounts          []string `json:"stageCounts"`
		FlagItems            []string `json:"flagItems"`
		OmissionItems        []string `json:"omissionItems"`
		ArchitectureText     string   `json:"architectureText"`
		BrowseRowCount       int      `json:"browseRowCount"`
		BrowseStageTexts     []string `json:"browseStageTexts"`
		UnavailableRows      []string `json:"unavailableRows"`
		ModelPickCount       int      `json:"modelPickCount"`
		DirectionCardAfter   string   `json:"directionCardAfterPick"`
		BeyondBefore         int      `json:"beyondBefore"`
		LocalCollapsedBefore bool     `json:"localCollapsedBefore"`
		LocalCollapsedAfter  bool     `json:"localCollapsedAfter"`
		ShowAllCount         int      `json:"showAllCount"`
		BrowsePanelPresent   bool     `json:"browsePanelPresent"`
	}
	runJourney := func(lang string) journey {
		output, err := exec.Command(node, runnerPath, assetPath, lang).CombinedOutput()
		if err != nil {
			t.Fatalf("run diagnostics workspace (%s): %v\n%s", lang, err, output)
		}
		var got journey
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode diagnostics workspace (%s): %v\n%s", lang, err, output)
		}
		return got
	}
	got := runJourney("en")

	// Four-stage diagnostics: stage counts present with exact values.
	for _, want := range []string{
		"Study diagnostics", "Considered spans", "68",
		"Advertised spans", "32",
		"Model-selected spans", "10",
		"Locally accepted spans", "10",
	} {
		if !strings.Contains(got.StudyOverviewText, want) {
			t.Fatalf("Study diagnostics missing %q:\n%s", want, got.StudyOverviewText)
		}
	}
	// The four independent flags render with exact true/false presentation.
	wantFlagText := strings.Join([]string{
		"Frontier completeNo", "Selected items completeYes",
		"Support coverage completeYes", "Portfolio target metYes",
	}, "|")
	if strings.Join(got.FlagItems, "|") != wantFlagText {
		t.Fatalf("flag presentation = %#v, want %q", got.FlagItems, wantFlagText)
	}
	// The raw advertised_budget chip is replaced by the human omission
	// sentence and a "Show all N" disclosure button.
	if len(got.OmissionItems) != 1 ||
		!strings.Contains(got.OmissionItems[0], "Left out of the model's review to keep the request bounded — these are full local questions.") ||
		!strings.Contains(got.OmissionItems[0], "Show all 36") ||
		strings.Contains(got.OmissionItems[0], "advertised_budget") {
		t.Fatalf("omission aggregates = %#v", got.OmissionItems)
	}
	// D212 frontier browse renders below the diagnostics panel.
	if !got.BrowsePanelPresent {
		t.Fatalf("frontier browse panel missing from the Study overview:\n%s", got.StudyOverviewText)
	}
	if !strings.Contains(got.StudyOverviewText, "All study questions") ||
		!strings.Contains(got.StudyOverviewText, "Every question the local analysis can answer for this repository, in a fixed local order. This is not a model ranking.") {
		t.Fatalf("frontier browse title/caption missing:\n%s", got.StudyOverviewText)
	}
	if got.BrowseRowCount != 68 {
		t.Fatalf("browse rows = %d, want 68", got.BrowseRowCount)
	}
	// Four distinct stage states with exact counts 10/0/22/36 (a/b/c/d).
	stageCount := func(label string) int {
		count := 0
		for _, item := range got.BrowseStageTexts {
			if item == label {
				count++
			}
		}
		return count
	}
	if got.ModelPickCount != 10 || stageCount("Model pick") != 10 {
		t.Fatalf("Model pick badges = %d/%d, want 10", got.ModelPickCount, stageCount("Model pick"))
	}
	if stageCount("Shown to the model, not picked") != 22 {
		t.Fatalf("advertised rows = %d, want 22", stageCount("Shown to the model, not picked"))
	}
	if stageCount("Local question — not shown to the model") != 36 {
		t.Fatalf("considered rows = %d, want 36", stageCount("Local question — not shown to the model"))
	}
	// A Model pick badge opens exactly one numbered direction card.
	if !strings.Contains(got.DirectionCardAfter, "Study question 1?") {
		t.Fatalf("Model pick badge did not open the matching direction card:\n%s", got.DirectionCardAfter)
	}
	// Show all N reveals the Local group (12 representatives visible first).
	if got.ShowAllCount < 1 || got.BeyondBefore != 24 || !got.LocalCollapsedBefore || got.LocalCollapsedAfter {
		t.Fatalf("Show all N = %d, beyond rows = %d, collapsed before/after = %v/%v, want 1, 24, true, false",
			got.ShowAllCount, got.BeyondBefore, got.LocalCollapsedBefore, got.LocalCollapsedAfter)
	}
	// Rows without an openable source render the neutral unavailable state.
	if len(got.UnavailableRows) != 2 || !strings.Contains(got.UnavailableRows[0], "Source unavailable") {
		t.Fatalf("unavailable rows = %#v, want 2 neutral states", got.UnavailableRows)
	}
	// Honest synthesis-failed copy replaces the unconditional acceptance copy.
	if !strings.Contains(got.OverviewText, "Architecture synthesis failed; showing the locally available architecture with exact symbol sources.") {
		t.Fatalf("Overview does not show the synthesis-failed copy:\n%s", got.OverviewText)
	}
	if strings.Contains(got.OverviewText, "Accepted conceptual components open on the map") {
		t.Fatalf("Overview still shows the unconditional acceptance copy:\n%s", got.OverviewText)
	}
	if !strings.Contains(got.ArchitectureText, "Architecture synthesis failed; showing the locally available architecture.") {
		t.Fatalf("Architecture tab does not show the synthesis-failed notice:\n%s", got.ArchitectureText)
	}
	// RU journey: same browse with the Russian catalog.
	ru := runJourney("ru")
	if !strings.Contains(ru.StudyOverviewText, "Все вопросы изучения") ||
		!strings.Contains(ru.StudyOverviewText, "Каждый вопрос, который локальный анализ может поставить для этого репозитория, в фиксированном локальном порядке. Это не ранжирование моделью.") {
		t.Fatalf("RU frontier browse title/caption missing:\n%s", ru.StudyOverviewText)
	}
	ruStageCount := func(label string) int {
		count := 0
		for _, item := range ru.BrowseStageTexts {
			if item == label {
				count++
			}
		}
		return count
	}
	if ru.BrowseRowCount != 68 || ruStageCount("Выбор модели") != 10 ||
		ruStageCount("Показано модели, не выбрано") != 22 ||
		ruStageCount("Локальный вопрос — модели не показывался") != 36 ||
		len(ru.UnavailableRows) != 2 || !strings.Contains(ru.UnavailableRows[0], "Источник недоступен") {
		t.Fatalf("RU browse journey failed: rows=%d badges=%d/%d/%d unavailable=%#v",
			ru.BrowseRowCount, ruStageCount("Выбор модели"), ruStageCount("Показано модели, не выбрано"),
			ruStageCount("Локальный вопрос — модели не показывался"), ru.UnavailableRows)
	}
}
