# Maintainability Review — UX Redesign of `report.html`

**Date:** 2026-05-24  
**Reviewer:** maintainability-reviewer  
**Verdict:** Needs Changes — the UX goal is correct, but the implementation approach must be restructured to avoid creating a second generation of the same embedding anti-pattern that DX_REVIEW.md already identified.

---

## 1. Template Extraction Strategy

### Current state
`buildHTML()` in `internal/report/report.go` (lines 284–353) is a single `fmt.Sprintf` call that embeds CSS, HTML skeleton, and JavaScript inside a Go string literal. Every visual change requires recompiling the Go binary. No syntax highlighting, no linting, no separation of concerns.

### The question: is now the right time to extract?

**Yes — and it is not optional.** The proposed UX redesign (flow cards, arrow-connected step chains, numbered file sections, collapsible debug details, confidence badges) will roughly **double** the size of the existing inline HTML/CSS/JS. Embedding all of that inside `fmt.Sprintf` would produce a Go function that is 150+ lines of string-concatenation with no tooling support. That is a maintainability cliff.

### Recommended file structure

```
internal/report/
├── report.go              # ReadRunDir, WriteReportJSON, WriteReportHTML
├── types.go               # ReportData, FlowData, ChainStep, etc. (already here)
├── template.html          # HTML skeleton with Go template actions
├── style.css              # All CSS, separate file
├── report.js              # All JS rendering logic, separate file
├── embed.go               # //go:embed directives + template execution
├── report_test.go         # Tests
└── testdata/
    ├── single-flow.json
    ├── multi-flow.json
    ├── error-flow.json
    └── report.golden.html
```

### Using `//go:embed` vs `html/template`

**Use `//go:embed` for static assets (CSS, JS) — use `html/template` for the HTML skeleton.**

Rationale:
- `html/template` auto-escapes data (prevents XSS if flow names contain `<script>` tags). The current code uses a hand-rolled `esc()` function in JS. Go's template engine is battle-tested for this.
- `//go:embed` bakes the files into the binary at compile time. No file-reading at runtime. Works on all platforms. Go 1.16+ is guaranteed available (go.mod says 1.22).
- CSS and JS can be embedded as raw `string` via `//go:embed` and injected into `<style>` and `<script>` tags. This keeps the report self-contained (no CDN, no network — satisfies the constraint).
- The CSS and JS files get editor support (syntax highlighting, linting, Prettier) without any tooling changes.

**What to avoid:** Do NOT use `html/template` to render the entire report at Go-template time. The report has interactive tabs, collapsible sections, and data-driven rendering. Go's `html/template` is a server-side renderer. The report is a single-page app that loads JSON data and renders in the browser. The Go template's only job is to inject the JSON blob and the static CSS/JS.

Concretely:

```go
// internal/report/embed.go
package report

import (
    _ "embed"
    "html/template"
)

//go:embed style.css
var styleCSS string

//go:embed report.js
var reportJS string

//go:embed template.html
var templateHTML string

var reportTmpl = template.Must(template.New("report").Parse(templateHTML))
```

And `template.html` looks like:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1.0">
  <title>repomap — {{.Title}}</title>
  <style>{{.CSS}}</style>
</head>
<body>
  <div id="app"></div>
  <script>
    window.__REPOMAP_DATA__ = {{.DataJSON}};
  </script>
  <script>{{.JS}}</script>
</body>
</html>
```

The key decision: the Go template renders exactly once (writes the HTML file). The browser-side JS reads `window.__REPOMAP_DATA__` and constructs the DOM. This keeps the Go/JS boundary clean: Go owns data serialization, JS owns DOM rendering.

### When should this extraction happen?

**Before the UX redesign, not after.** Extracting the template is ~20 minutes of mechanical work. Doing it after adding 80+ lines of new inline HTML/CSS/JS would be much harder and error-prone. The sequence should be:

1. Extract existing template into `template.html`, `style.css`, `report.js` — verify nothing breaks (smoke test: same HTML output byte-for-byte)
2. Add tests for the extracted structure
3. Then start the UX redesign on the extracted files

---

## 2. Code Modularity

### JavaScript rendering logic

The current `render()` function (report.go:334–353) is one monolithic function that builds HTML strings with no structure. The redesign adds many more elements: step chains with arrows, numbered file sections, collapsible sections, confidence badges, bundle stats cards. Building all of this as a single string-concatenation function would produce unreadable code.

**Structure the JS as named rendering functions, one per UI component:**

```javascript
// report.js — suggested structure
(function() {
  'use strict';
  var DATA = window.__REPOMAP_DATA__;

  // --- Utility functions ---
  function esc(s) { /* ... */ }
  function confClass(c) { /* ... */ }
  function confLabel(c) { /* ... */ }  // "High (85%)" instead of bare percentage

  // --- Component renderers ---
  function renderHeader(data) { /* → DOM element */ }
  function renderOverviewCard(data) { /* → DOM element */ }
  function renderFlowCard(flow) { /* → DOM element */ }
  function renderChainSteps(chain) { /* → DOM element */ }
  function renderFileList(files, label) { /* → DOM element */ }
  function renderWarnings(warnings) { /* → DOM element */ }
  function renderUnknowns(unknowns) { /* → DOM element */ }
  function renderBundleStats(stats) { /* → DOM element */ }
  function renderErrorBox(error) { /* → DOM element */ }
  function renderCollapsibleSection(title, content, defaultOpen) { /* → DOM element */ }

  // --- Tab management ---
  var TabManager = { /* state for active tab */ };

  // --- Main ---
  function render() {
    var app = document.getElementById('app');
    app.appendChild(renderHeader(DATA));
    // ... compose components
  }

  window.addEventListener('DOMContentLoaded', render);
})();
```

Each `render*` function returns a DOM element (or DocumentFragment), not an HTML string. This enables:
- **Testing individual components** — a test harness can call `renderFlowCard(testFlowData)` and assert the DOM structure
- **Debugging** — browser DevTools shows a real DOM tree, not a flat string
- **Composition** — components can be reassembled as the UX evolves

**Use `document.createElement` / `appendChild` instead of `innerHTML`.** The current code sets `d.innerHTML = h` for every flow. This is a security risk if DeepSeek returns flow names containing HTML. The `esc()` function mitigates this, but `createElement` + `textContent` is safer and produces a cleaner DOM.

**Exception:** For sections where performance matters (e.g., a file list with 200+ items), use `innerHTML` with a pre-escaped string. But the default should be DOM API.

### CSS naming convention

The current CSS uses short, generic class names (`.card`, `.tab`, `.step`, `.tag`) that will collide with any browser extension or future embedded content.

**Use a prefix convention:** `rm-` (for "repomap"). This is lightweight (no build step needed), self-documenting, and prevents collisions:

```css
.rm-card { }
.rm-tab { }
.rm-tab--active { }
.rm-step { }
.rm-step__number { }
.rm-step__name { }
.rm-file-list { }
.rm-file-list__item { }
.rm-badge { }
.rm-badge--high { }
.rm-badge--medium { }
.rm-badge--low { }
.rm-collapsible { }
.rm-collapsible--open { }
```

**Use CSS custom properties for theming** (already partially done with `:root` variables). Keep this — it makes future dark-mode support trivial.

**Do NOT use a CSS framework** (Tailwind, Bootstrap, etc.). The constraint is self-contained HTML with network. Any framework would need to be inlined, and even minified Tailwind is 50KB+.

---

## 3. Separation of Concerns

### JS state management

The current code has minimal state: an active tab tracked implicitly via `classList` toggling. The redesign adds collapsible sections (raw details, debug info, bundle breakdown) which require more state.

**Question:** Should the JS maintain explicit state (an object tracking `activeTab`, `expandedSections`), or should state live only in the DOM (checking `classList`)?

**Recommendation: DOM as state, with thin helpers.**

Rationale:
- The report is read-only. There are no mutations to the data model, no form inputs, no server round-trips. State is purely "what is visible right now."
- Introducing a state management object (even a simple one) creates a second source of truth. The DOM already has `classList`, `hidden` attributes, and `display` styles. Keeping state in the DOM avoids drift.
- The constraint is "no React, no build step." A lightweight state manager is tempting but adds complexity with no benefit.

**Provide thin helper functions:**

```javascript
function showTab(tabId) {
  document.querySelectorAll('.rm-tab, .rm-tab-content').forEach(function(el) {
    el.classList.remove('rm-active');
  });
  document.getElementById(tabId).classList.add('rm-active');
  document.querySelector('.rm-tab[data-tab="' + tabId + '"]').classList.add('rm-active');
}

function toggleCollapsible(sectionId) {
  var el = document.getElementById(sectionId);
  el.classList.toggle('rm-collapsible--open');
}
```

**No URL hash routing.** Some SPAs update `window.location.hash` to enable deep-linking (e.g., `report.html#flow-2`). This is nice-to-have but adds complexity. Defer until requested.

---

## 4. Test Strategy

`internal/report/` currently has **zero tests**. Adding tests is mandatory before or during the UX redesign — otherwise every visual change is a blind edit with no safety net.

### Test categories and what they cover

| Test | What it validates | Priority |
|------|------------------|----------|
| **Golden file: HTML output** | Given a known `ReportData`, the generated HTML matches `testdata/report.golden.html`. Catches unintended changes to the HTML structure, CSS class names, or JS logic. | P0 — must exist before the redesign |
| **Golden file: single flow** | One flow with all fields populated (chain, files, tests, unknowns, warnings). Ensures every section renders. | P0 |
| **Golden file: error flow** | A flow with `Error` set but no other data. Ensures error display works; other sections are gracefully absent. | P0 |
| **Golden file: partial flow** | Flow with `Summary` but no `LikelyChain`, `FilesToRead` but no `TestsToRead`. Ensures optional sections don't crash. | P1 |
| **Golden file: empty report** | `Flows` is `nil` or empty. Warnings exist. Ensures report doesn't crash on empty data. | P1 |
| **Structural: `ReadRunDir` round-trip** | Write valid JSON → `ReadRunDir` reads it → data matches. Tests all the `interface{}` cast paths (until they're replaced). | P0 |
| **Structural: `WriteReportJSON` round-trip** | Create `ReportData` → write JSON → read back → same data. | P1 |
| **Structural: `ReportData` JSON tags** | Serialize, deserialize, verify all fields survive the round trip. Protects against tag typos. | P1 |
| **Unit: `esc()` function in JS** | Test string escaping logic (can be done with a Go test that parses the JS and extracts the function, or a simple Node test). | P2 (defer — coverage via golden tests) |
| **Integration: smoke test** | Build a temp git repo → run pipeline → verify `report.html` is valid HTML and contains expected strings. Already partially covered by `scripts/smoke.sh`. | P2 |

### Golden file test approach

```go
// internal/report/report_test.go
func TestWriteReportHTML_Golden(t *testing.T) {
    data := loadTestData(t, "testdata/multi-flow.json")
    var buf bytes.Buffer
    // WriteReportHTML currently writes to a file; add an io.Writer variant
    err := writeReportHTMLTo(&buf, data)
    if err != nil {
        t.Fatal(err)
    }
    golden := filepath.Join("testdata", "report.golden.html")
    if *update {
        os.WriteFile(golden, buf.Bytes(), 0o644)
        return
    }
    want, _ := os.ReadFile(golden)
    if !bytes.Equal(buf.Bytes(), want) {
        t.Errorf("HTML output differs from golden file. Use -update to regenerate.")
    }
}
```

**Important:** The HTML output must be **deterministic**. The current code uses `json.Marshal(data)` which produces stable output (no map iteration). The JS rendering must also be deterministic — no `Object.keys()` iteration without explicit sorting. If the order of flows, files, or steps can vary, the golden test will flake.

**Structural tests for `ReadRunDir`:**

```go
func TestReadRunDir_RoundTrip(t *testing.T) {
    dir := t.TempDir()
    // Write the files that ReadRunDir expects
    writeFile(t, dir, "snapshot.json", `{"repo_name":"testrepo"}`)
    writeFile(t, dir, "orientation_report.json", `{"project_guess":"test project","candidate_flows":[{"name":"flow1"},{"name":"flow2"}],"warnings":["warn1"]}`)
    // ... flow directories with bundle/report files ...

    data, err := ReadRunDir(dir)
    if err != nil {
        t.Fatal(err)
    }
    assert.Equal(t, "testrepo", data.RepoName)
    assert.Equal(t, "test project", data.ProjectGuess)
    assert.Len(t, data.CandidateFlows, 2)
    assert.Len(t, data.Warnings, 1)
}
```

**Testing the JS rendering from Go:** Since the JS runs in the browser, Go tests can't exercise the JS directly. The golden file test is the primary safety net. For deeper JS testing, a future step could add a headless browser test (e.g., using `chromedp`), but this adds a heavy dependency and is not recommended for the initial extraction.

### What NOT to test

- Don't test CSS visual output (pixel-perfect rendering). Golden file covers structure.
- Don't test browser-specific behavior (scroll position, hover states). These are visual polish, not correctness.
- Don't test third-party browser behavior (how Chrome vs Firefox renders a `<details>` element). Use standard elements.

---

## 5. Data Contract Stability

The `report.json` structure (the `ReportData`, `FlowData`, `ChainStep`, etc. Go structs) is consumed by two things:
1. `WriteReportJSON` / `WriteReportHTML` — the report generator
2. Future consumers of debug artifacts (developers inspecting `.repomap-runs/`)

### Adding fields: forward-compatible by default

Because JS reads JSON with `DATA.flows` and accesses fields like `f.summary`, adding a new field to the Go struct (e.g., `RecommendedFirstFlow string`) is safe: old JS ignores it, new JS can use it with a guard:

```javascript
var firstFlow = DATA.recommended_first_flow || null;
```

The `json:"...,omitempty"` tag on optional fields ensures they don't appear in the JSON when empty, keeping the output clean.

### Renaming fields: breaking change

If `BundleStats` is renamed to `FlowStats`, or `likely_chain` becomes `steps`, all existing debug artifacts become unreadable. There are two strategies:

1. **Version the report:** Add a `"report_version": 1` field to `ReportData`. When the JS loads data, it checks the version and applies a migration. This is what the web platform does. It is also overkill for a CLI tool with no persistent storage.

2. **Don't rename. Only add.** Treat the field names as a public API. The current names (`likely_chain`, `files_to_read_in_order`, `bundle_summary`) are already reasonable. If a name must change, add the new field alongside the old one, mark the old one deprecated, and remove it after a deprecation period. Since there are no external consumers yet, this is a low-risk constraint.

**Recommendation:** Strategy 2. Add a comment in `types.go`:

```go
// Data contract: the JSON field names in struct tags are part of the public
// debug-artifact format. Add new fields freely. Do not rename existing fields
// without a transitional period where both old and new names are populated.
type ReportData struct { ... }
```

### The `Flows` array order

The current code sorts flows by `ID` (report.go:179–181). This is deterministic and should stay. If a "recommended first flow" concept is added, that flow should appear first, with the remaining flows sorted by ID.

---

## 6. Error Handling in the Report UI

### What should the report look like when data is missing or malformed?

The report must handle several failure modes gracefully:

**Case 1: A flow has `Error` set (DeepSeek call failed).**
- Show the flow tab with an error icon (⚠️ or a red indicator)
- The flow content area shows the error message in a distinct error box
- Other flow tabs are unaffected
- The overview card for this flow shows "Error" instead of confidence

**Case 2: `Flows` is empty or nil.**
- The overview shows "No flows identified"
- No tabs rendered
- Warnings (if any) are displayed prominently
- The report does NOT produce a blank white page

**Case 3: A flow has `Summary` but no `LikelyChain`.**
- Summary renders normally
- "Likely Chain" section is absent (not an empty "Likely Chain" heading with nothing under it)
- Confidence badge still renders

**Case 4: `FilesToRead` has items but `Priority` is zero (unset).**
- Files render in their received order (the data is the source of truth)
- Do NOT sort by priority unless priority values are meaningful and non-zero

**Case 5: The `ReportData` JSON in the `<script>` tag is malformed.**
- The browser will throw a syntax error before any JS runs
- Add a `<noscript>` fallback: `<noscript><p>This report requires JavaScript to view. The raw data is available in <code>report.json</code>.</p></noscript>`
- Consider adding an inline `<pre>` with the raw JSON hidden behind a `<details>` tag that is always visible (rendered server-side in the Go template), so even if JS fails, the data is accessible

**Case 6: DeepSeek returns a flow with confidence > 1.0 or < 0.0.**
- Clamp the value in the JS: `Math.max(0, Math.min(1, flow.confidence))`
- Do not clamp in Go — preserve the raw value in the JSON for debugging

### Implementation approach

Every JS render function should start with a defensive check:

```javascript
function renderFlowCard(flow) {
  if (!flow) return emptyCard('No flow data');
  if (flow.error) return errorCard(flow.error);

  var card = document.createElement('div');
  card.className = 'rm-card rm-flow-card';

  if (flow.summary) {
    card.appendChild(renderSummary(flow.summary));
  }
  if (flow.confidence != null) {
    card.appendChild(renderConfidenceBadge(flow.confidence));
  }
  if (flow.likely_chain && flow.likely_chain.length > 0) {
    card.appendChild(renderChainSteps(flow.likely_chain));
  }
  // ... and so on
  return card;
}
```

No render function should assume a field exists. Every field access is guarded.

---

## 7. Performance

### Can the embedded `<script>DATA=...</script>` approach hit browser limits?

**Short answer: Probably not, but monitor it.**

The largest debug runs in the existing codebase (etcd with 4 flows, ~150 files per flow bundle) produce `report.json` files around 150–300 KB. When embedded as a `<script>` tag, the HTML file grows to 150–300 KB. Modern browsers handle multi-megabyte inline scripts without issue.

**When would this become a problem?**
- If `--flows` is set to 20+ and each flow has extensive `files_to_read_in_order`, `tests_to_read`, `likely_chain` with full `evidence_files` lists, and `unverified_paths`
- The `BundleSummary` is small (5 integers). The heavy data is in the flow arrays.
- Estimated worst case: 20 flows × 10KB JSON each = 200KB. Still fine.

**What if it does become a problem?**
- The report is opened from the local filesystem (`file://` protocol). `fetch()` from `file://` is blocked by CORS in most browsers. So loading `report.json` as a separate file via JS is not an option.
- The Go template could write the data as a JSON blob in a `<script type="application/json">` tag, which is exactly what the current approach does.
- If compression is needed, the Go template could gzip the JSON and include a small JS decompressor, but this is premature.

**Recommendation:** Stick with the inline `<script>` approach. Add a comment in `embed.go` noting the size limit and that `fetch()` is unavailable on `file://`.

### DOM rendering performance for large reports

If a flow has 200+ `files_to_read_in_order`, building DOM nodes via `document.createElement` for each file might be slower than setting `innerHTML` with a pre-built string. This is a micro-optimization.

**Do NOT pre-optimize.** Use `document.createElement` everywhere first. If a real-world repo produces a visibly slow report (criterion: > 500ms to render), then optimize the specific component by switching to `innerHTML` with escaped strings. The golden test will catch any regression.

### Memory for collapsible sections

The proposed UX has collapsible "raw details" sections. If a section is collapsed, the DOM nodes should still exist (hidden via CSS `display: none`) rather than being destroyed and recreated on expand. This avoids re-parsing on every toggle and keeps the JS stateless (state lives in the DOM).

---

## 8. Dependencies

### Should we add any Go dependencies for HTML templating?

**No.** The Go standard library provides everything needed:
- `html/template` for the HTML skeleton
- `encoding/json` for data serialization (already used)
- `//go:embed` for embedding static files (available since Go 1.16; go.mod requires 1.22)

The project's `go.mod` currently has **zero external dependencies**:
```
module github.com/dvordrova/repomap
go 1.22
```

This is a deliberate design choice (ARCHITECTURE_GO_REFACTOR.md §"What a mid dev should NOT change", item 1). The standard library is sufficient. Adding `go.sum` for a single dependency that duplicates stdlib functionality (e.g., a template engine, a CSS inliner, an HTML minifier) would violate this principle.

### What about JS dependencies?

**No.** The constraint is self-contained HTML with no CDN and no network. Shipping a JS framework inline (even a minimal one like Preact at 3KB) adds complexity with no proportionate benefit. The rendering logic is straightforward: read JSON → create DOM nodes → attach to tree. This is ~200 lines of vanilla JS.

---

## 9. What NOT to Do

These are the mistakes that would make future developers curse this code:

### 1. DO NOT keep the embedded string literal approach and just make it bigger

The current `buildHTML()` is 70 lines of `fmt.Sprintf`. The redesign would make it 150+ lines. This is the exact anti-pattern that DX_REVIEW.md item #4 calls out. Every future CSS change, JS bug fix, or template adjustment would require recompiling Go. A mid-level dev in 2 weeks would look at this and ask "why didn't they extract this when they had the chance?"

### 2. DO NOT mix Go template rendering with JS rendering

If the Go template renders the flow cards server-side AND the JS also manipulates the DOM (for tabs, collapsible sections), you now have two rendering paths that must stay in sync. This is a classic source of bugs: the initial render shows one thing, then JS runs and shows something slightly different. Pick one: either Go renders everything (static report, no JS), or Go injects data and JS renders everything (dynamic report). The UX requirements (tabs, collapsible sections) demand the dynamic approach.

### 3. DO NOT use `innerHTML` as the primary DOM construction method

The current code builds HTML strings in JS and sets `innerHTML`. This:
- Bypasses browser escaping (requires hand-rolled `esc()`, which can be forgotten)
- Makes the DOM harder to inspect (DevTools shows a flat string, not a tree)
- Makes unit testing components harder (can't call `renderFlowCard()` and assert on the result)

Use `document.createElement`, `textContent`, and `appendChild`. Reserve `innerHTML` for paragraphs of user-provided text where you've already escaped the content.

### 4. DO NOT add a build step (esbuild, webpack, TypeScript, CSS preprocessor)

The constraint is "no React/Vite/build step." Adding any build tooling — even a simple one — means:
- Every developer must install Node.js and run `npm install`
- CI/CD must add Node.js to the Go build pipeline
- The build output must be committed or generated in CI
- Version skew between Go and JS tooling becomes a problem

The JS and CSS are small enough (likely < 500 lines each) that vanilla JS and vanilla CSS work fine. If the team outgrows this, they can add tooling later — but the initial extraction should be zero-dependency.

### 5. DO NOT hardcode UI text in the JS

The current code has labels like `"Files to Read"`, `"Likely Chain"`, `"Warnings"` as string literals in the JS. If these need to be localized or customized in the future, they're scattered across render functions.

**Instead:** Collect them in a single object at the top of `report.js`:

```javascript
var LABELS = {
  overviewTitle: 'Flow Overview',
  candidateFlows: 'Candidate Flows',
  filesToRead: 'Files to Read (in order)',
  testsToRead: 'Tests',
  likelyChain: 'Execution Chain',
  unverified: 'Unverified Paths',
  unknowns: 'Unknowns',
  warnings: 'Warnings',
  bundleStats: 'Bundle Statistics',
  confidence: 'Confidence',
  noFlows: 'No flows identified.',
  collapsedDefault: 'Show raw details',
};
```

This also makes golden-file tests more stable — changing a label is a one-line edit, not a hunt through render functions.

### 6. DO NOT create a `report/types.go` that duplicates types from other packages

`ReportData`, `FlowData`, `ChainStep`, etc. are already defined in `internal/report/report.go`. If these types are extracted to `types.go` (which is fine), do NOT also define parallel types in `internal/orient/` or `internal/flowexplain/`. The existing codebase already has duplicated type definitions (DX_REVIEW §3, ARCHITECTURE_REFACTOR §5). The report types should be the single source of truth for the data contract.

### 7. DO NOT skip tests for `ReadRunDir`

`ReadRunDir` is 122 lines of JSON parsing with `map[string]interface{}` type assertions. It is the most fragile code in the report package. If you change the UX without adding tests for this function, you are one `json.Unmarshal` refactor away from silently breaking the data ingestion pipeline. The golden tests will catch it, but unit tests for `ReadRunDir` will tell you **exactly which field** is broken.

### 8. DO NOT use CSS class names that clash with browser defaults or extensions

Names like `.card`, `.tab`, `.overview`, `.tags`, `.step` are too generic. Browser extensions (password managers, ad blockers, grammar checkers) inject elements with generic class names. The `rm-` prefix recommended in §2 prevents collisions and makes it obvious in DevTools which elements belong to the report.

### 9. DO NOT rely on CSS `@import` or JS `import`

Everything must be inline in the HTML file. CSS `@import` would trigger a network request. JS `import` statements require a module server or a bundler. Inline only. The `//go:embed` + Go template approach achieves this.

### 10. DO NOT add a feature toggles or A/B testing infrastructure to the report

The report is a local HTML file opened in a browser. There is no server, no analytics, no backend to query for feature flags. Adding feature flag logic to the JS would add complexity for a use case that doesn't exist. If you need to test two versions of the report, generate both and open them side by side.

---

## Summary Checklist

Before the UX redesign begins:

- [ ] Extract existing HTML/CSS/JS into separate files (`template.html`, `style.css`, `report.js`)
- [ ] Set up `//go:embed` directives in `embed.go`
- [ ] Verify byte-for-byte identical HTML output with the extracted version
- [ ] Add golden file test for HTML output (single flow, multi-flow, error flow)
- [ ] Add structural tests for `ReadRunDir` round-trip
- [ ] Add `.rm-` prefix to all CSS class names
- [ ] Code-review all test data files — ensure golden files contain no absolute paths or timestamps

During the UX redesign:

- [ ] Structure JS as named `render*` functions returning DOM elements
- [ ] Use `document.createElement` / `textContent` (not `innerHTML`) for all new code
- [ ] Guard every field access in JS; handle missing/empty data gracefully
- [ ] Collect all UI labels in a single `LABELS` object
- [ ] Add collapsible sections with DOM-based state (no state object)
- [ ] Update golden files as the HTML changes

After the UX redesign:

- [ ] Run `./scripts/check.sh` (go test + go vet)
- [ ] Open the test data golden HTML in a browser — verify it looks correct
- [ ] Test with `file://` protocol (drag the file into a browser window)
- [ ] Verify no network requests in DevTools Network tab
- [ ] Verify report works when `Flows` is empty, when a flow has an error, and when optional fields are missing

---

## What Will Be Painful in 2 Weeks If Left As-Is

If the UX redesign is implemented by expanding the existing `fmt.Sprintf` pattern instead of extracting the template:

1. **Every visual tweak requires a Go recompile.** A mid-level dev asked to "make the confidence badge blue instead of green" must edit a Go string literal, recompile, and redeploy. With extracted files, they edit CSS and refresh the browser.

2. **JS bugs are invisible.** There's no console, no debugger breakpoints, no linting for JS embedded in a Go string literal. A missing semicolon or mismatched brace produces a runtime error in the browser with no line number mapping back to the Go source.

3. **The `buildHTML()` function hits 200+ lines.** The Go compiler doesn't care, but human reviewers do. Code review of "change the flow card layout" means scanning a 200-line `fmt.Sprintf` for the 3 lines that changed. Reviewers will rubber-stamp it.

4. **Golden file tests are impossible to write.** The current code produces HTML that is functionally identical but structurally unstable (no guarantee that `json.Marshal` output won't change field ordering between Go versions). Extracting the template doesn't fix this, but extracting first makes it possible to write the golden tests before the UX changes, giving a safety net.

5. **The next developer will have to do the extraction anyway.** Deferring it compounds the cost: they'll have to reverse-engineer the string concatenation logic to figure out where each piece of HTML comes from, then extract it, then make their change. The extraction takes 20 minutes now; it'll take 2 hours after 80 more lines are added.

**The verdict is "needs changes" not "reject" because the UX goals are correct and the data contract is well-structured. The only issue is the implementation approach: extract the template before expanding it.**
