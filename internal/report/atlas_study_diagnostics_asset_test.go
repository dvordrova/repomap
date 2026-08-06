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
const modelPickBadges = byClass(browseRoot, "rm-study-browse-row__stage-published");
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
  const questionLinks = byClass(browseRoot, "rm-study-browse-row__question").map((node) => ({ tag: node.tagName, href: (node.attributes && node.attributes.href) || "" }));
  const themeShelf = byClass(roots["rm-study-overview"], "rm-study-theme-shelf")[0] || null;
  // Decision 224 (D219 C/G): per-reading role badges and bounded
  // observations render on the expanded theme detail.
  // Decision 229 D6: cards are collapsed by default with at most two
  // previews — the complete reading plan (role badges, observations,
  // exact source jumps) lives in the expanded detail workspace.
  const themeTitles = byClass(roots["rm-study-overview"], "rm-study-theme-card__title");
  let readingRoleBadges = [];
  let readingExplains = [];
  let readingJump = null;
  // Decision 229 D6: cards are collapsed by default — open every theme and
  // aggregate role badges/observations/jumps from the expanded details.
  themeTitles.forEach((title) => {
    title.onclick();
    const detailRoot = roots["rm-study-detail"];
    readingRoleBadges = readingRoleBadges.concat(byClass(detailRoot, "rm-study-theme-card__reading-role"));
    readingExplains = readingExplains.concat(byClass(detailRoot, "rm-study-theme-card__reading-explain"));
    const detailReadings = byClass(detailRoot, "rm-study-reading-anchor__open");
    if (!readingJump && detailReadings.length) {
      readingJump = { tag: String(detailReadings[0].tagName || "").toLowerCase(), href: (detailReadings[0].attributes && detailReadings[0].attributes.href) || "" };
    }
  });
  const drawerEl = roots["rm-source-drawer"];
  const drawerDialog = {
    role: drawerEl.getAttribute("role"),
    ariaModal: drawerEl.getAttribute("aria-modal"),
    ariaLabel: drawerEl.getAttribute("aria-label"),
  };
  return { overviewText, studyOverviewText, stageCounts, flagItems, omissionItems, architectureText, browseRowCount: browseRows.length, browseStageTexts, unavailableRows, modelPickCount: modelPickBadges.length, directionCardAfterPick, beyondBefore, localCollapsedBefore, localCollapsedAfter, showAllCount: showAllButtons.length, browsePanelPresent: !!browsePanel, questionLinks, themeShelfPresent: !!themeShelf, drawerDialog, readingJump, readingRoleBadgeCount: readingRoleBadges.length, readingExplainCount: readingExplains.length };
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
	type questionLink struct {
		Tag  string `json:"tag"`
		Href string `json:"href"`
	}
	type journey struct {
		OverviewText         string         `json:"overviewText"`
		StudyOverviewText    string         `json:"studyOverviewText"`
		StageCounts          []string       `json:"stageCounts"`
		FlagItems            []string       `json:"flagItems"`
		OmissionItems        []string       `json:"omissionItems"`
		ArchitectureText     string         `json:"architectureText"`
		BrowseRowCount       int            `json:"browseRowCount"`
		BrowseStageTexts     []string       `json:"browseStageTexts"`
		UnavailableRows      []string       `json:"unavailableRows"`
		ModelPickCount       int            `json:"modelPickCount"`
		DirectionCardAfter   string         `json:"directionCardAfterPick"`
		BeyondBefore         int            `json:"beyondBefore"`
		LocalCollapsedBefore bool           `json:"localCollapsedBefore"`
		LocalCollapsedAfter  bool           `json:"localCollapsedAfter"`
		ShowAllCount         int            `json:"showAllCount"`
		BrowsePanelPresent   bool           `json:"browsePanelPresent"`
		QuestionLinks        []questionLink `json:"questionLinks"`
		ThemeShelfPresent    bool           `json:"themeShelfPresent"`
		DrawerDialog         struct {
			Role      string `json:"role"`
			AriaModal string `json:"ariaModal"`
			AriaLabel string `json:"ariaLabel"`
		} `json:"drawerDialog"`
		ReadingJump *struct {
			Tag  string `json:"tag"`
			Href string `json:"href"`
		} `json:"readingJump"`
		ReadingRoleBadgeCount int `json:"readingRoleBadgeCount"`
		ReadingExplainCount   int `json:"readingExplainCount"`
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

	// Four-stage diagnostics: stage counts present with exact values.
	for _, want := range []string{
		"Study diagnostics", "Considered spans", "68",
		"Advertised spans", "32",
		"Model-selected spans", "10",
		"Locally accepted spans", "10",
	} {
		if !strings.Contains(en.StudyOverviewText, want) {
			t.Fatalf("Study diagnostics missing %q:\n%s", want, en.StudyOverviewText)
		}
	}
	// The four independent flags render with exact true/false presentation.
	wantFlagText := strings.Join([]string{
		"Frontier completeNo", "Selected items completeYes",
		"Support coverage completeYes", "Portfolio target metYes",
	}, "|")
	if strings.Join(en.FlagItems, "|") != wantFlagText {
		t.Fatalf("flag presentation = %#v, want %q", en.FlagItems, wantFlagText)
	}
	// The raw seed_budget chip is replaced by the human omission
	// sentence and a "Show all N" disclosure button.
	if len(en.OmissionItems) != 1 ||
		!strings.Contains(en.OmissionItems[0], "Left out of the model's review to keep the request bounded — these are full local questions.") ||
		!strings.Contains(en.OmissionItems[0], "Show all 36") ||
		strings.Contains(en.OmissionItems[0], "seed_budget") {
		t.Fatalf("omission aggregates = %#v", en.OmissionItems)
	}
	// D213 source-grounded theme shelf renders first with exact readings.
	if !en.ThemeShelfPresent {
		t.Fatalf("theme shelf missing from the Study overview:\n%s", en.StudyOverviewText)
	}
	if !strings.Contains(en.StudyOverviewText, "Source-grounded study themes") ||
		!strings.Contains(en.StudyOverviewText, "Theme 1") ||
		!strings.Contains(en.StudyOverviewText, "Theme 2") ||
		!strings.Contains(en.StudyOverviewText, "Study question 1?") {
		t.Fatalf("theme shelf copy missing:\n%s", en.StudyOverviewText)
	}
	// D212 frontier browse renders below the diagnostics panel.
	if !en.BrowsePanelPresent {
		t.Fatalf("frontier browse panel missing from the Study overview:\n%s", en.StudyOverviewText)
	}
	if !strings.Contains(en.StudyOverviewText, "All study questions") ||
		!strings.Contains(en.StudyOverviewText, "Every question the local analysis can answer for this repository, in a fixed local order. This is not a model ranking.") {
		t.Fatalf("frontier browse title/caption missing:\n%s", en.StudyOverviewText)
	}
	if en.BrowseRowCount != 68 {
		t.Fatalf("browse rows = %d, want 68", en.BrowseRowCount)
	}
	// Four distinct stage states with exact counts 10/0/22/36 (a/b/c/d).
	stageCount := func(label string) int {
		count := 0
		for _, item := range en.BrowseStageTexts {
			if item == label {
				count++
			}
		}
		return count
	}
	if en.ModelPickCount != 10 || stageCount("Published in a theme") != 10 {
		t.Fatalf("Model pick badges = %d/%d, want 10", en.ModelPickCount, stageCount("Published in a theme"))
	}
	if stageCount("Shown to the model, not picked") != 22 {
		t.Fatalf("advertised rows = %d, want 22", stageCount("Shown to the model, not picked"))
	}
	if stageCount("Local question — not shown to the model") != 36 {
		t.Fatalf("considered rows = %d, want 36", stageCount("Local question — not shown to the model"))
	}
	// A published badge opens exactly one numbered theme card.
	if !strings.Contains(en.DirectionCardAfter, "Study question 1?") {
		t.Fatalf("published badge did not open the matching theme card:\n%s", en.DirectionCardAfter)
	}
	// Show all N reveals the Local group (12 representatives visible first).
	if en.ShowAllCount < 1 || en.BeyondBefore != 24 || !en.LocalCollapsedBefore || en.LocalCollapsedAfter {
		t.Fatalf("Show all N = %d, beyond rows = %d, collapsed before/after = %v/%v, want 1, 24, true, false",
			en.ShowAllCount, en.BeyondBefore, en.LocalCollapsedBefore, en.LocalCollapsedAfter)
	}
	// Rows without an openable source render the neutral unavailable state.
	if len(en.UnavailableRows) != 2 || !strings.Contains(en.UnavailableRows[0], "Source unavailable") {
		t.Fatalf("unavailable rows = %#v, want 2 neutral states", en.UnavailableRows)
	}
	// Honest synthesis-failed copy replaces the unconditional acceptance copy.
	if !strings.Contains(en.OverviewText, "Architecture synthesis failed; showing the locally available architecture with exact symbol sources.") {
		t.Fatalf("Overview does not show the synthesis-failed copy:\n%s", en.OverviewText)
	}
	if strings.Contains(en.OverviewText, "Accepted conceptual components open on the map") {
		t.Fatalf("Overview still shows the unconditional acceptance copy:\n%s", en.OverviewText)
	}
	if !strings.Contains(en.ArchitectureText, "Conceptual grouping is unavailable because the model exceeded its response budget. The partial response was not used; exact local Architecture remains available.") {
		t.Fatalf("Architecture tab does not show the output-limit notice:\n%s", en.ArchitectureText)
	}
	// RU journey: same browse with the Russian catalog.
	if !strings.Contains(ru.StudyOverviewText, "Все вопросы изучения") ||
		!strings.Contains(ru.StudyOverviewText, "Каждый вопрос, который локальный анализ может поставить для этого репозитория, в фиксированном локальном порядке. Это не ранжирование моделью.") {
		t.Fatalf("RU frontier browse title/caption missing:\n%s", ru.StudyOverviewText)
	}
	if !ru.ThemeShelfPresent || !strings.Contains(ru.StudyOverviewText, "Темы изучения на основе источников") {
		t.Fatalf("RU theme shelf missing:\n%s", ru.StudyOverviewText)
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
	if ru.BrowseRowCount != 68 || ruStageCount("Опубликовано в теме") != 10 ||
		ruStageCount("Показано модели, не выбрано") != 22 ||
		ruStageCount("Локальный вопрос — модели не показывался") != 36 ||
		len(ru.UnavailableRows) != 2 || !strings.Contains(ru.UnavailableRows[0], "Источник недоступен") {
		t.Fatalf("RU browse journey failed: rows=%d badges=%d/%d/%d unavailable=%#v",
			ru.BrowseRowCount, ruStageCount("Опубликовано в теме"), ruStageCount("Показано модели, не выбрано"),
			ruStageCount("Локальный вопрос — модели не показывался"), ru.UnavailableRows)
	}
	if !strings.Contains(ru.ArchitectureText, "Концептуальная группировка недоступна: модель исчерпала лимит ответа. Частичный ответ не использован; точная локальная архитектура остаётся доступна.") {
		t.Fatalf("RU Architecture tab does not show the output-limit notice:\n%s", ru.ArchitectureText)
	}
	// Stripped static report: no embedded source bodies; the browse still
	// renders 68 rows with the same stage counts, and every openable row
	// resolves to one exact pinned GitHub source action instead of a dead
	// button, while non-openable rows keep the neutral unavailable state.
	if !stripped.BrowsePanelPresent || stripped.BrowseRowCount != 68 {
		t.Fatalf("stripped browse panel/rows = %v/%d, want true/68", stripped.BrowsePanelPresent, stripped.BrowseRowCount)
	}
	strippedStageCount := func(label string) int {
		count := 0
		for _, item := range stripped.BrowseStageTexts {
			if item == label {
				count++
			}
		}
		return count
	}
	if strippedStageCount("Published in a theme") != 10 ||
		strippedStageCount("Shown to the model, not picked") != 22 ||
		strippedStageCount("Local question — not shown to the model") != 36 ||
		len(stripped.UnavailableRows) != 2 || !strings.Contains(stripped.UnavailableRows[0], "Source unavailable") {
		t.Fatalf("stripped browse counts failed: badges=%d/%d/%d unavailable=%#v",
			strippedStageCount("Published in a theme"), strippedStageCount("Shown to the model, not picked"),
			strippedStageCount("Local question — not shown to the model"), stripped.UnavailableRows)
	}
	if len(stripped.QuestionLinks) < 66 {
		t.Fatalf("stripped question source actions = %d, want >= 66", len(stripped.QuestionLinks))
	}
	if stripped.QuestionLinks[0].Tag != "a" ||
		!strings.Contains(stripped.QuestionLinks[0].Href, "/blob/") ||
		!strings.Contains(stripped.QuestionLinks[0].Href, "#L") {
		t.Fatalf("stripped question source action is not a pinned link: %#v", stripped.QuestionLinks[0])
	}
	if !strings.Contains(stripped.DirectionCardAfter, "Study question 1?") {
		t.Fatalf("stripped Model pick badge did not open the matching direction card:\n%s", stripped.DirectionCardAfter)
	}
	// Decision 222: a theme reading is a GitHub/GitLab jump in a new tab —
	// never an inline code drawer.
	for name, j := range map[string]journey{"en": en, "ru": ru} {
		if j.ReadingJump == nil || j.ReadingJump.Tag != "a" ||
			!strings.Contains(j.ReadingJump.Href, "github.com/example/repository/blob") {
			t.Fatalf("%s reading jump = %#v, want GitHub blob link", name, j.ReadingJump)
		}
		// Decision 224 (D219 G): every reading renders a role badge and the
		// bounded supported observation on the theme card.
		if j.ReadingRoleBadgeCount < 2 {
			t.Fatalf("%s reading role badges = %d, want >= 2", name, j.ReadingRoleBadgeCount)
		}
		if j.ReadingExplainCount < 2 {
			t.Fatalf("%s reading explanations = %d, want >= 2", name, j.ReadingExplainCount)
		}
	}
}
