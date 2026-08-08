package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStudyProgressiveDisclosureD229Asset verifies the Decision 229 D6
// Study shelf: all theme titles visible, cards collapsed by default, at
// most two reading previews per collapsed card, independent evidence/scope
// badges, one-open detail, and repeated-symbol preview grouping.
func TestStudyProgressiveDisclosureD229Asset(t *testing.T) {
	scriptPath := filepath.Join("templates", "script.js")
	assetPath := filepath.Join(t.TempDir(), "study-d229-test.js")
	asset := `const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) { this.tagName = tag; this.children = []; this.attributes = {}; this.textContent = ""; this.hidden = false; }
  get className() { return this._className || ""; }
  set className(value) { this._className = value; }
  get classList() {
    const self = this;
    return {
      add(name) { self._className = (self._className ? self._className + " " : "") + name; },
      remove(name) { self._className = (self._className || "").split(/\s+/).filter((c) => c !== name).join(" "); },
      toggle(name, force) {
        const has = (self._className || "").split(/\s+/).includes(name);
        const want = force === undefined ? !has : !!force;
        if (want && !has) self._className = (self._className ? self._className + " " : "") + name;
        if (!want && has) self._className = (self._className || "").split(/\s+/).filter((c) => c !== name).join(" ");
      },
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
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 2,
    content_sha256: "a".repeat(64), presentation_sha256: ("" + line).padStart(64, "0"),
    lines: [
      { line, text: "func " + symbol + "() {", highlight: true },
      { line: line + 1, text: "  inspect()" },
      { line: line + 2, text: "}" },
    ],
  };
}
// 11 peer themes (casdoor acceptance), readings 1..5 per card; cards
// include a repeated public symbol (Send) to verify visual grouping.
const themeCards = [1, 2, 4, 2, 1, 4, 3, 4, 5, 4, 3].map((count, themeIndex) => {
  const readings = [];
  const kinds = ["user_journey", "lifecycle_concern", "cross_cutting_policy", "sibling_implementation_family"];
  for (let readingIndex = 0; readingIndex < count; readingIndex++) {
    const path = "study/theme-" + (themeIndex + 1) + "-reading-" + (readingIndex + 1) + ".go";
    const symbol = themeIndex === 2 && readingIndex > 0 ? "Send" : "Read" + (themeIndex + 1) + "_" + (readingIndex + 1);
    const readingLine = 10 + readingIndex;
    readings.push({
      label: "Start here",
      symbol,
      path,
      line: readingLine,
      role: readingIndex % 2 === 0 ? "direct" : "supporting",
      supported_observation: "Inspect exact reading " + (readingIndex + 1) + ".",
      source: snippet(path, symbol, readingLine),
    });
  }
  return {
    ordinal: themeIndex + 1,
    final_title: "Theme " + (themeIndex + 1),
    final_question: "Study question " + (themeIndex + 1) + "?",
    why_it_matters: "Reason " + (themeIndex + 1) + ".",
    expected_learning: "Outcome " + (themeIndex + 1) + ".",
    theme_kind: kinds[themeIndex % kinds.length],
    readings,
    badge: themeIndex % 3 === 0 ? "source_backed" : "partial",
    limitation: themeIndex === 0 ? "Coverage spans more source than was reviewed in this run." : "",
    unknowns: themeIndex === 0
      ? ["Runtime branch selection remains unresolved.", "Retry ordering remains unresolved."]
      : [],
    alternate_titles: themeIndex === 0 ? ["Lifecycle view of theme 1"] : [],
    alternate_questions: themeIndex === 0 ? ["Where does theme 1 cross its lifecycle boundary?"] : [],
    alternate_readings: themeIndex === 0 ? [
      {
        label: "Repeated primary",
        symbol: readings[0].symbol,
        path: readings[0].path,
        line: readings[0].line,
        role: readings[0].role,
        supported_observation: readings[0].supported_observation,
      },
      {
        label: "Alternate exact source",
        symbol: "AlternateTheme1",
        path: "study/theme-1-alternate.go",
        line: 41,
        role: "direct",
        supported_observation: "Inspect the co-projected exact reading.",
      },
    ] : [],
  };
});
const openablePaths = Array.from(new Set(themeCards.flatMap((card) =>
  card.readings.concat(card.alternate_readings || []).map((reading) => reading.path))));
const report = {
  repo_name: "fixture", report_language: "en", user_mechanisms: [], user_topics: [], user_sources: [],
  openable_paths: openablePaths,
  source_ids: {},
  github_source_links: { repository_url: "https://github.com/example/fixture", revision: "1".repeat(40), working_tree_paths: [] },
  repository_atlas: { version: 1, units: [], entities: [], evidence: [], relations: [] },
  atlas_study: {
    version: 1, projection_version: 8, state: "accepted",
    themes: { total: 11, shown: 11, cards: themeCards },
  },
  architecture_canvas: {
    version: 8, validation_outcome: "accepted", architecture_source: "validated_model",
    title: "Architecture", subtitle: "",
    subsystems: [], components: [], behavior_anchors: [], surfaces: [], flows: [],
  },
  architecture_synthesis: { state: "accepted" },
};
const roots = {};
["rm-overview", "rm-task-investigation", "rm-mechanisms", "rm-mechanism-detail",
 "rm-study-overview", "rm-study-detail", "rm-operate-detail", "rm-architecture", "rm-provenance",
].forEach((id) => {
  roots[id] = new Element("section");
  roots[id].className = "rm-tab-content" + (id === "rm-study-overview" ? " rm-active" : "");
});
roots["rm-tabs"] = new Element("nav");
roots["rm-source-drawer"] = new Element("aside");
roots["rm-source-drawer"].hidden = true;
roots["rm-source-drawer-content"] = new Element("div");
roots["rm-source-drawer-close"] = new Element("button");
const workspace = new Element("main");
const window = {
  location: { search: "", hash: "#/study", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
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
const studyTab = walk(roots["rm-tabs"]).find((node) => node.attributes && node.attributes["data-workspace-view"] === "study_overview");
if (studyTab) studyTab.onclick();
const studyRoot = roots["rm-study-overview"];
const cards = byClass(studyRoot, "rm-study-theme-card");
const titles = byClass(studyRoot, "rm-study-theme-card__title").map((node) => text(node));
// Collapsed cards show at most two preview rows; cards with more readings
// show "+N readings".
const previewCounts = cards.map((card) => byClass(card, "rm-study-theme-card__preview").length);
const moreCounts = cards.map((card) => {
  const more = byClass(card, "rm-study-theme-card__more")[0];
  if (!more) return 0;
  const match = String(text(more)).match(/\+\s*(\d+)/);
  return match ? Number(match[1]) : 0;
});
// Evidence and scope badges are independent and both present.
const evidenceBadges = byClass(studyRoot, "rm-study-theme-card__evidence").map((node) => text(node));
const scopeBadges = byClass(studyRoot, "rm-study-theme-card__scope").map((node) => text(node));
// Repeated symbol grouping: the theme with the repeated "Send" symbol shows
// a callsite count in its preview.
const previewTexts = cards.map((card) => text(card));
const sendGrouped = previewTexts.some((t) => t.includes("Send") && t.includes("callsites"));
const firstCardText = cards.length ? text(cards[0]) : "";
const shelfHasFirstLimitation = firstCardText.includes("Runtime branch selection remains unresolved.") &&
  !firstCardText.includes("Retry ordering remains unresolved.");
// Opening one card navigates to detail; siblings remain collapsed.
const firstTitle = titles.length ? byClass(studyRoot, "rm-study-theme-card__title")[0] : null;
firstTitle.onclick();
const detailRoot = roots["rm-study-detail"];
const detailText = text(detailRoot);
const detailHasExpectedLearning = detailText.includes("Expected learning: Outcome 1.");
const detailHasReading = detailText.includes("study/theme-1-reading-1.go");
const detailHasRole = detailText.includes("direct");
const detailHasObservation = detailText.includes("Inspect exact reading 1.");
const detailHasLimitations = !!detailRoot.querySelector(".rm-study-theme-card__limitations");
const detailHasAllLimitations = detailText.includes("Runtime branch selection remains unresolved.") &&
  detailText.includes("Retry ordering remains unresolved.");
const detailHasNoNestedDisclosure = !walk(detailRoot).some((node) => node.tagName === "details");
const detailHasAlternateWording = detailText.includes("Lifecycle view of theme 1") &&
  detailText.includes("Where does theme 1 cross its lifecycle boundary?");
const detailHasAlternateReading = detailText.includes("study/theme-1-alternate.go") &&
  detailText.includes("Inspect the co-projected exact reading.");
const detailReadingRows = byClass(detailRoot, "rm-study-reading-anchor").length;
const detailSourceActions = byClass(detailRoot, "rm-study-reading-anchor__open");
const alternateSourceIsAction = detailSourceActions.some((node) =>
  text(node).includes("study/theme-1-alternate.go") && (node.tagName === "a" || node.tagName === "button"));
process.stdout.write(JSON.stringify({
  cardCount: cards.length,
  titles,
  previewCounts,
  moreCounts,
  evidenceBadges,
  scopeBadges,
  sendGrouped,
  detailHasExpectedLearning,
  detailHasReading,
  detailHasRole,
  detailHasObservation,
  detailHasLimitations,
  detailHasAllLimitations,
  detailHasNoNestedDisclosure,
  detailHasAlternateWording,
  detailHasAlternateReading,
  detailReadingRows,
  alternateSourceIsAction,
  shelfHasFirstLimitation,
  previewTexts,
}));`
	if err := os.WriteFile(assetPath, []byte(asset), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", assetPath, scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node journey failed: %v\n%s", err, output)
	}
	var got struct {
		CardCount                   int      `json:"cardCount"`
		Titles                      []string `json:"titles"`
		PreviewCounts               []int    `json:"previewCounts"`
		MoreCounts                  []int    `json:"moreCounts"`
		EvidenceBadges              []string `json:"evidenceBadges"`
		ScopeBadges                 []string `json:"scopeBadges"`
		SendGrouped                 bool     `json:"sendGrouped"`
		DetailHasExpectedLearning   bool     `json:"detailHasExpectedLearning"`
		DetailHasReading            bool     `json:"detailHasReading"`
		DetailHasRole               bool     `json:"detailHasRole"`
		DetailHasObservation        bool     `json:"detailHasObservation"`
		DetailHasLimitations        bool     `json:"detailHasLimitations"`
		DetailHasAllLimitations     bool     `json:"detailHasAllLimitations"`
		DetailHasNoNestedDisclosure bool     `json:"detailHasNoNestedDisclosure"`
		DetailHasAlternateWording   bool     `json:"detailHasAlternateWording"`
		DetailHasAlternateReading   bool     `json:"detailHasAlternateReading"`
		DetailReadingRows           int      `json:"detailReadingRows"`
		AlternateSourceIsAction     bool     `json:"alternateSourceIsAction"`
		ShelfHasFirstLimitation     bool     `json:"shelfHasFirstLimitation"`
		PreviewTexts                []string `json:"previewTexts"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode study D229 journey: %v\n%s", err, output)
	}
	if got.CardCount != 11 {
		t.Fatalf("theme cards = %d, want 11 (peer themes must not be hidden)", got.CardCount)
	}
	for index, count := range got.PreviewCounts {
		if count > 2 {
			t.Fatalf("collapsed card %d shows %d previews, want <= 2", index+1, count)
		}
	}
	// Cards 3, 6, 8, 9, 10, 11 have >2 readings → must show "+N readings".
	expectedMore := []int{0, 0, 2, 0, 0, 2, 1, 2, 3, 2, 1}
	for index, want := range expectedMore {
		if got.MoreCounts[index] != want {
			t.Fatalf("card %d more-readings rows = %d, want %d; previews=%v more=%v\npreviewTexts=%v", index+1, got.MoreCounts[index], want, got.PreviewCounts, got.MoreCounts, got.PreviewTexts)
		}
	}
	if len(got.EvidenceBadges) != 11 || len(got.ScopeBadges) != 11 {
		t.Fatalf("evidence badges = %d, scope badges = %d, want 11 each (independent axes)", len(got.EvidenceBadges), len(got.ScopeBadges))
	}
	if !strings.Contains(strings.Join(got.EvidenceBadges, " "), "Source-backed") ||
		!strings.Contains(strings.Join(got.EvidenceBadges, " "), "Partial coverage") {
		t.Fatalf("evidence badges do not distinguish source-backed vs partial: %#v", got.EvidenceBadges)
	}
	if !strings.Contains(strings.Join(got.ScopeBadges, " "), "Exact scope") ||
		!strings.Contains(strings.Join(got.ScopeBadges, " "), "Partial scope") {
		t.Fatalf("scope badges do not distinguish exact vs partial: %#v", got.ScopeBadges)
	}
	if !got.SendGrouped {
		t.Fatalf("repeated public symbol (Send) was not grouped with a callsite count")
	}
	if !got.DetailHasExpectedLearning || !got.DetailHasReading || !got.DetailHasRole ||
		!got.DetailHasObservation || !got.DetailHasLimitations {
		t.Fatalf("expanded detail incomplete: learning=%v reading=%v role=%v observation=%v limitations=%v",
			got.DetailHasExpectedLearning, got.DetailHasReading, got.DetailHasRole, got.DetailHasObservation, got.DetailHasLimitations)
	}
	if !got.ShelfHasFirstLimitation {
		t.Fatal("collapsed theme card does not expose exactly the first material limitation")
	}
	if !got.DetailHasAllLimitations || !got.DetailHasNoNestedDisclosure {
		t.Fatalf("opened theme limitations are not directly complete: all=%v no_nested_disclosure=%v",
			got.DetailHasAllLimitations, got.DetailHasNoNestedDisclosure)
	}
	if !got.DetailHasAlternateWording || !got.DetailHasAlternateReading || !got.AlternateSourceIsAction {
		t.Fatalf("opened theme lost co-projected material: wording=%v reading=%v source_action=%v",
			got.DetailHasAlternateWording, got.DetailHasAlternateReading, got.AlternateSourceIsAction)
	}
	if got.DetailReadingRows != 2 {
		t.Fatalf("opened theme reading rows = %d, want 2 (primary + distinct alternate; repeated primary deduplicated)", got.DetailReadingRows)
	}
}
