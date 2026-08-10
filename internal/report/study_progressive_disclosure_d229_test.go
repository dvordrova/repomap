package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestStudyProgressiveDisclosureD229Asset verifies the reader-facing Study
// shelf: every theme and exact reading is directly visible, a compact contents
// menu opens a theme, and internal coverage/provenance accounting stays out of
// the product surface.
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
    concentration_marker: "cmd:7/11",
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
        role: "supporting",
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
api.renderIncompleteStudyOverview();
const studyRoot = roots["rm-study-overview"];
const cards = byClass(studyRoot, "rm-study-theme-card");
const titles = byClass(studyRoot, "rm-study-theme-card__title").map((node) => text(node));
const previewCounts = cards.map((card) => byClass(card, "rm-study-reading-anchor").length);
const previewActions = byClass(studyRoot, "rm-study-reading-anchor__open");
const previewActionHrefs = previewActions.map((node) => node.getAttribute("href") || "");
const previewActionLabels = previewActions.map((node) => node.getAttribute("aria-label") || "");
const contentNav = byClass(studyRoot, "rm-study-theme-contents")[0] || null;
const contentActions = byClass(studyRoot, "rm-study-theme-contents__action");
const openActions = byClass(studyRoot, "rm-study-theme-card__open");
const previewTexts = cards.map((card) => text(card));
const removedProductChromeCount = [
  "rm-study-theme-card__more", "rm-study-theme-card__evidence", "rm-study-theme-card__scope",
  "rm-study-theme-card__concentration", "rm-study-diagnostics", "rm-study-frontier-browse",
  "rm-study-theme-card__limitation",
].reduce((count, className) => count + byClass(studyRoot, className).length, 0);
const detailsCount = walk(studyRoot).filter((node) => node.tagName === "details").length;
// The contents menu is navigation, not decoration.
if (contentActions[0]) contentActions[0].onclick();
const detailRoot = cards[0];
const detailText = text(detailRoot);
const detailHasExpectedLearning = detailText.includes("What to verify: Outcome 1.");
const detailHasSupportedExplanation = detailText.includes("Every reading has an exact source anchor. The theme wording is a model interpretation to verify against that code.");
const detailHasReading = detailText.includes("study/theme-1-reading-1.go");
const detailHasRole = detailText.includes("direct");
const directOnlyHasRole = text(cards[4]).includes("direct support") || text(cards[4]).includes("supporting");
const detailHasObservation = detailText.includes("Inspect exact reading 1.");
const detailHasLimitations = !!detailRoot.querySelector(".rm-study-theme-card__limitations");
const detailHasAllLimitations = detailText.includes("Runtime branch selection remains unresolved.") &&
  detailText.includes("Retry ordering remains unresolved.");
const detailHasNoNestedDisclosure = !walk(detailRoot).some((node) => node !== detailRoot && node.tagName === "details");
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
  previewActionCount: previewActions.length,
  previewActionHrefs,
  previewActionLabels,
  contentCount: contentActions.length,
  contentLabel: contentNav && contentNav.getAttribute("aria-label"),
  openActionCount: openActions.length,
  removedProductChromeCount,
  detailsCount,
  shelfLeaksRawConcentrationMarker: text(studyRoot).includes("cmd:7/11"),
  detailHasExpectedLearning,
  detailHasSupportedExplanation,
  detailHasReading,
  detailHasRole,
  directOnlyHasRole,
  detailHasObservation,
  detailHasLimitations,
  detailHasAllLimitations,
  detailHasNoNestedDisclosure,
  detailHasAlternateWording,
  detailHasAlternateReading,
  detailReadingRows,
  alternateSourceIsAction,
  firstDetailOpen: !!detailRoot.open,
  contentTags: contentActions.map((node) => node.tagName),
  contentHrefs: contentActions.map((node) => node.getAttribute("href") || ""),
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
		CardCount                        int      `json:"cardCount"`
		Titles                           []string `json:"titles"`
		PreviewCounts                    []int    `json:"previewCounts"`
		PreviewActionCount               int      `json:"previewActionCount"`
		PreviewActionHrefs               []string `json:"previewActionHrefs"`
		PreviewActionLabels              []string `json:"previewActionLabels"`
		ContentCount                     int      `json:"contentCount"`
		ContentLabel                     string   `json:"contentLabel"`
		OpenActionCount                  int      `json:"openActionCount"`
		RemovedProductChromeCount        int      `json:"removedProductChromeCount"`
		DetailsCount                     int      `json:"detailsCount"`
		ShelfLeaksRawConcentrationMarker bool     `json:"shelfLeaksRawConcentrationMarker"`
		DetailHasExpectedLearning        bool     `json:"detailHasExpectedLearning"`
		DetailHasSupportedExplanation    bool     `json:"detailHasSupportedExplanation"`
		DetailHasReading                 bool     `json:"detailHasReading"`
		DetailHasRole                    bool     `json:"detailHasRole"`
		DirectOnlyHasRole                bool     `json:"directOnlyHasRole"`
		DetailHasObservation             bool     `json:"detailHasObservation"`
		DetailHasLimitations             bool     `json:"detailHasLimitations"`
		DetailHasAllLimitations          bool     `json:"detailHasAllLimitations"`
		DetailHasNoNestedDisclosure      bool     `json:"detailHasNoNestedDisclosure"`
		DetailHasAlternateWording        bool     `json:"detailHasAlternateWording"`
		DetailHasAlternateReading        bool     `json:"detailHasAlternateReading"`
		DetailReadingRows                int      `json:"detailReadingRows"`
		AlternateSourceIsAction          bool     `json:"alternateSourceIsAction"`
		FirstDetailOpen                  bool     `json:"firstDetailOpen"`
		ContentTags                      []string `json:"contentTags"`
		ContentHrefs                     []string `json:"contentHrefs"`
		PreviewTexts                     []string `json:"previewTexts"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode study D229 journey: %v\n%s", err, output)
	}
	if got.CardCount != 11 {
		t.Fatalf("theme cards = %d, want 11 (peer themes must not be hidden)", got.CardCount)
	}
	expectedReadings := []int{1, 2, 4, 2, 1, 4, 3, 4, 5, 4, 3}
	readingTotal := 0
	for index, want := range expectedReadings {
		readingTotal += want
		if index == 0 {
			want++ // one distinct alternate reading is inline in the same disclosure
		}
		if got.PreviewCounts[index] != want {
			t.Fatalf("card %d exact reading rows = %d, want %d; all=%v", index+1, got.PreviewCounts[index], want, got.PreviewCounts)
		}
	}
	if got.PreviewActionCount != readingTotal+1 || len(got.PreviewActionHrefs) != readingTotal+1 || len(got.PreviewActionLabels) != readingTotal+1 {
		t.Fatalf("exact Study source actions = %d hrefs=%d aria=%d, want %d", got.PreviewActionCount, len(got.PreviewActionHrefs), len(got.PreviewActionLabels), readingTotal+1)
	}
	for index, href := range got.PreviewActionHrefs {
		if !strings.Contains(href, "github.com/example/fixture/blob/") || !strings.Contains(href, "#L") || got.PreviewActionLabels[index] == "" {
			t.Fatalf("preview %d is not one pinned, labelled source action: href=%q aria=%q", index+1, href, got.PreviewActionLabels[index])
		}
	}
	if got.ContentCount != 11 || got.ContentLabel != "Contents" || got.OpenActionCount != 0 || !got.FirstDetailOpen {
		t.Fatalf("Study navigation = contents %d aria=%q open-actions=%d first-open=%v, want 11/Contents/0/true", got.ContentCount, got.ContentLabel, got.OpenActionCount, got.FirstDetailOpen)
	}
	for index := range got.ContentTags {
		if got.ContentTags[index] != "a" || got.ContentHrefs[index] != "#study-theme-"+strconv.Itoa(index+1) {
			t.Fatalf("contents item %d = <%s href=%q>, want anchor to inline disclosure", index+1, got.ContentTags[index], got.ContentHrefs[index])
		}
	}
	if got.RemovedProductChromeCount != 0 || got.DetailsCount != 11 || got.ShelfLeaksRawConcentrationMarker {
		t.Fatalf("removed Study accounting leaked: chrome=%d details=%d raw-concentration=%v\n%s", got.RemovedProductChromeCount, got.DetailsCount, got.ShelfLeaksRawConcentrationMarker, strings.Join(got.PreviewTexts, "\n"))
	}
	for index, title := range got.Titles {
		want := "Theme " + string(rune('1'+index))
		if index == 9 {
			want = "Theme 10"
		} else if index == 10 {
			want = "Theme 11"
		}
		if title != want {
			t.Fatalf("theme title %d = %q, want %q", index+1, title, want)
		}
	}
	if got.DetailHasExpectedLearning || got.DetailHasSupportedExplanation || !got.DetailHasReading || !got.DetailHasRole ||
		!got.DetailHasObservation || !got.DetailHasLimitations {
		t.Fatalf("theme detail has internal prose or lost useful evidence: expected_learning=%v generic_truth=%v reading=%v role=%v observation=%v limitations=%v",
			got.DetailHasExpectedLearning, got.DetailHasSupportedExplanation, got.DetailHasReading, got.DetailHasRole, got.DetailHasObservation, got.DetailHasLimitations)
	}
	if got.DirectOnlyHasRole {
		t.Fatal("a direct-only Study card repeats a non-discriminating role badge")
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
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "investigation.mechanisms.forEach(function (mechanism, index)") ||
		strings.Contains(string(script), "investigation.mechanisms.slice(") {
		t.Fatal("inline Study must publish every persisted mechanism without a frontend slice")
	}
}
