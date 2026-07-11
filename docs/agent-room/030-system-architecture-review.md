# System Architecture Review — UX Redesign for report.html

**Date:** 2026-05-24
**Reviewer:** System Architect
**Subject:** Proposed developer-dashboard UX for `repomap` self-contained HTML reports

---

## Verdict

**Proceed with the UX redesign, but restructure the report generation boundary FIRST.**

The proposed feature (flow cards, visual step chains, prominent files-to-read, collapsed debug details) is low-risk at the presentation layer. The risk is architectural: report generation is currently a side-effect buried inside `orient.Run()`, HTML is an unmaintainable 70-line `fmt.Sprintf` string, and `report.ReadRunDir()` parses JSON via unchecked `map[string]interface{}` casts. These must be fixed before — not after — adding new UX features.

---

## 1. Data Flow Analysis

### Current path (end-to-end)

```
deepseek.FlowExplain()                    [internal/deepseek/client.go:188]
  → returns json.RawMessage            [orient.go:471]
  → written to flows/<fid>/flow_report.json  [orient.go:483]
  → stored as explainedFlow.FlowReport (json.RawMessage)  [orient.go:334-336]

report.ReadRunDir()                       [internal/report/report.go:63]
  → reads snapshot.json                  [line 70] → RepoName
  → reads orientation_report.json        [line 76] → ProjectGuess, CandidateFlows, Warnings
  → reads flows/*/flow_bundle.json       [line 108] → BundleSummary, Name
  → reads flows/*/flow_report.json       [line 124] → Summary, Confidence, LikelyChain,
                                              FilesToRead, TestsToRead, UnverifiedPaths,
                                              Unknowns, Warnings
  → assembles ReportData struct           [line 68]

WriteReportHTML()                         [internal/report/report.go:269]
  → json.Marshal(ReportData)             [line 270]
  → buildHTML(title, dataJSON)           [line 280]
  → fmt.Sprintf with inline CSS/JS       [line 330]
  → embeds DATA = <json> in <script>      [line 330]

Browser DOM                               [line 334-353]
  → render() reads DATA global
  → builds tabbed UI with vanilla JS
```

### Where enrichment should happen

**Go-side (`ReportData` enrichment):**
Computed fields that depend on cross-flow comparisons, file system access, or parsing of bundled data that JavaScript cannot do without re-reading files. Examples:
- `RecommendedFirstFlow` — requires comparing confidence scores across all flows
- `ReadingTimeEstimate` — `FilesToRead.length * 2 minutes` (deterministic)
- `KeyFindings` — extracted from top-3 flows' summaries

**JS-side (render-time enrichment):**
Purely presentational transformations. Examples:
- Sorting files by priority within a flow
- Grouping unknowns by category
- Computing CSS class names from confidence values (already done)
- Toggle states, expand/collapse behavior

**Rule:** If the computation requires iterating across flows or accessing data outside the current flow, do it in Go. If it's a view transformation of data already in the flow struct, do it in JS.

---

## 2. Data Contract Decisions

### Current report.json (from user prompt)

```json
{
  "repo_name": "...",
  "project_guess": "...",
  "candidate_flows": ["name1", "name2"],
  "flows": [{
    "id": "grpc-put-request",
    "name": "gRPC Put Request",
    "summary": "...",
    "confidence": 0.85,
    "likely_chain": [{ "step": 1, "name": "Receive", "what_happens": "...", "evidence_files": [...], "confidence": 0.9 }],
    "files_to_read_in_order": [{ "path": "...", "reason": "...", "priority": 1 }],
    "tests_to_read": [...],
    "unverified_paths": [...],
    "unknowns": [...],
    "warnings": [...],
    "bundle_summary": { "selected_files_count": 12, "selected_tests_count": 5, "selected_docs_count": 2, "selected_packages_count": 3, "related_edges_count": 7 },
    "error": ""
  }],
  "artifacts_dir": "...",
  "warnings": []
}
```

### Recommended additions to the Go struct

| Field | Type | Location | Computed in | Rationale |
|-------|------|----------|-------------|-----------|
| `recommended_first_flow` | `string` (flow ID) | `ReportData` | Go (`ReadRunDir`) | Cross-flow comparison; JS cannot know which flow is the best starting point without reimplementing the scoring logic |
| `reading_time_estimate` | `int` (minutes) | `FlowData` | Go (`ReadRunDir`) | Deterministic: `files_to_read * 2 min + tests * 1 min`. Trivial to compute, meaningless to duplicate in JS |
| `key_findings` | `[]string` | `ReportData` | Go (`ReadRunDir`) | Top-3 flow names + their one-line summaries. Used for overview dashboard card |
| `total_files_analyzed` | `int` | `ReportData` | Go (`ReadRunDir`) | Sum of all flow `selected_files_count` — useful overview metric |
| `total_flows_explained` | `int` | `ReportData` | Go (`ReadRunDir`) | `len(flows)` — convenience for the overview |
| `generated_at` | `string` (ISO8601) | `ReportData` | Go (`Generate`) | Timestamp of report generation, not pipeline run. Important for shareable HTML files |

### What NOT to add

- **Code snippets / line numbers in ChainStep.** The report is generated from debug artifacts which contain file *paths*, not file *contents*. Adding snippets would require `ReadRunDir()` to read source files from the repo — a hidden dependency that breaks when the HTML is shared without the repo present. If snippets are desired later, they must be captured during the pipeline (in `flowexplain.SelectFlowFiles()`), stored in debug artifacts, and passed through passively.
- **Richer ChainStep data beyond existing fields.** The existing `step`, `name`, `what_happens`, `evidence_files`, `confidence` are sufficient for visual step cards with arrows. Additional structure (e.g., "depends_on" edges between steps) would require changes to the DeepSeek prompt and response parsing, which is a separate project.
- **Per-flow "difficulty" or "complexity" scores.** These would be LLM-generated judgments, not deterministic facts. If added, they must be returned by DeepSeek in `flow_report.json`, not computed in `ReadRunDir()`.
- **Presentational fields** (colors, icons, layout hints). These are UI-layer concerns. JS can derive them from existing data (e.g., `confidence > 0.7` → green badge).

### Enrichment location decision

| Computation | Go | JS | Reason |
|-------------|----|----|--------|
| `recommended_first_flow` | ✅ | ❌ | Requires cross-flow comparison |
| `reading_time_estimate` | ✅ | ❌ | Deterministic, single source of truth |
| `key_findings` | ✅ | ❌ | Cross-flow extraction |
| `generated_at` | ✅ | ❌ | Go runtime knows the time |
| Confidence → CSS class | ❌ | ✅ | Presentational |
| Sort files by priority | ❌ | ✅ | View concern |
| Flow card ordering on overview | ❌ | ✅ | Already sorted by ID in Go, but tab rendering order is a view choice |
| Collapse/expand raw JSON details | ❌ | ✅ | Pure UI state |
| Label translations ("F" → "Source Files") | ❌ | ✅ | Presentational |

---

## 3. Separation of Concerns

### Current state (unacceptable)

`internal/report/report.go:284-353` — the entire HTML/CSS/JS application lives inside a `fmt.Sprintf` call. The format string spans 70 lines. CSS is 40 lines of inline rules. JavaScript is inline in a `<script>` tag. There is no syntax highlighting, no linting, and no separation between Go data extraction and HTML rendering.

This is the **single highest-priority fix** before any UX changes. The DX Review (item #4) and Architecture Review (#10) both flag this.

### Recommended architecture

```
internal/report/
├── report.go            # ReadRunDir(), EnrichReportData(), WriteReportJSON()
├── report_test.go       # Golden-file tests
├── render_html.go       # HTMLRenderer.Render(), //go:embed directives
├── render_json.go       # JSONRenderer.Render()
├── renderer.go          # Renderer interface
├── templates/
│   ├── report.html      # Go text/template — minimal structure, data injection points
│   ├── style.css        # Embedded via //go:embed, injected into <style> tag
│   └── script.js        # Embedded via //go:embed, injected into <script> tag
└── testdata/
    ├── report.golden.json
    └── report.golden.html
```

### Template engine choice

**Use `text/template`, not `html/template`.**

Reason: The template's job is to inject a JSON data blob and CSS/JS files into an HTML wrapper. `html/template` auto-escapes all inserted values — it would mangle the JSON blob (escaping `<`, `>`, `&`), breaking `JSON.parse()` on the JavaScript side. The JSON blob is the one insertion point that must not be escaped; everything else in the template is static HTML structure from embedded files.

The template structure should be minimal:

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
  <header>...</header>
  <div class="container">
    <div class="tabs" id="tabs"></div>
    <div id="overview" class="tab-content active"></div>
    <div id="flows-container"></div>
  </div>
  <script type="application/json" id="report-data">{{.DataJSON}}</script>
  <script>{{.JS}}</script>
</body>
</html>
```

The Go code:
```go
//go:embed templates/report.html
var templateStr string

//go:embed templates/style.css
var cssStr string

//go:embed templates/script.js
var jsStr string

func (r *HTMLRenderer) Render(data *ReportData) ([]byte, error) {
    dataJSON, _ := json.Marshal(data)
    tmpl, _ := template.New("report").Parse(templateStr)
    var buf bytes.Buffer
    tmpl.Execute(&buf, map[string]any{
        "Title":    data.RepoName + " — " + data.ProjectGuess,
        "DataJSON": template.JSStr(string(dataJSON)), // raw JSON, no escaping
        "CSS":      template.CSS(cssStr),
        "JS":       template.JSStr(jsStr),
    })
    return buf.Bytes(), nil
}
```

### Where data transformation lives

| Transformation | Location | File |
|---------------|----------|------|
| Parse debug artifacts → `ReportData` | Go | `report.go:ReadRunDir()` |
| Enrich `ReportData` (computed fields) | Go | `report.go:EnrichReportData()` |
| Serialize to JSON | Go | `json.Marshal(ReportData)` |
| Inject JSON into HTML template | Go | `render_html.go:Render()` |
| Build DOM from JSON | JS | `script.js:render()` |
| Tab switching, expand/collapse | JS | `script.js` (event handlers) |
| Confidence → color mapping | JS | `script.js:confClass()` |
| Warning/error visibility toggles | JS | `script.js` |

---

## 4. Pipeline Integration

### Current state (problematic)

`orient.Run()` calls `writeRunReport()` as a side-effect at lines 222 and 229. This function:
1. Reads back debug artifacts from disk via `report.ReadRunDir()`
2. Writes `report.json`
3. Writes `report.html`
4. Prints the report path to stderr

Problems:
1. **Report rendering blocks pipeline response.** The user waits for JSON parsing + HTML generation before seeing output.
2. **`orient.Run()` has a hidden filesystem dependency.** The function's contract says it returns `([]byte, error)`, but it also writes files as a side-effect.
3. **Report generation is coupled to the debug directory.** If `--no-debug` is passed, no HTML is generated. These should be orthogonal concerns.
4. **`orient.Run()` cannot be tested without a real debug directory on disk.**

### Recommended architecture

**Decouple pipeline from rendering.** Report generation should be a separate step called from `cmd/main.go`, not from `orient.Run()`.

```
cmd/main.go:
  1. orient.Run(ctx, opts)  → returns (output []byte, runID string, err error)
  2. if debugDir != "":
       report.Generate(debugDir, runID)  → writes report.json, report.html
  3. write output to stdout

orient.Run():
  - REMOVE calls to writeRunReport()
  - RETURN runID alongside output
  - Pipeline: snapshot → bundle → LLM → flow explain → return
```

`report.Generate()` becomes the single entry point:

```go
// internal/report/report.go (or generate.go)
func Generate(runDir string) error {
    data, err := ReadRunDir(runDir)
    if err != nil {
        return err
    }
    EnrichReportData(data)
    
    if err := WriteReportJSON(data, filepath.Join(runDir, "report.json")); err != nil {
        return err
    }
    if err := WriteReportHTML(data, filepath.Join(runDir, "report.html")); err != nil {
        return err
    }
    return nil
}
```

### Existing `repomap dev render-report`

This command already calls `report.ReadRunDir()` + `WriteReportJSON()` + `WriteReportHTML()` from `cmd/main.go:runRenderReport()`. After the refactor, `runRenderReport()` becomes an alias for `report.Generate()`. This is consistent and clean.

### Pipeline steps (after refactor)

```
1. gitfiles.List()           → tracked files
2. snapshot.Build()          → local deterministic snapshot
3. gofacts.Load()            → Go package/edge/entrypoint facts
4. llmbundle.Build()         → compact bounded LLM bundle
5. deepseek.Orient()         → orientation report
6. flowexplain.SelectFlowFiles()  → per-flow file selection
7. deepseek.FlowExplain()    → per-flow explanation
   ─────── pipeline boundary ───────
8. report.Generate()         → read debug artifacts, enrich, write report.json + report.html
```

Step 8 is a **read-only consumer** of steps 1-7's output. It does not participate in the pipeline; it renders its results.

---

## 5. Backward Compatibility

### Problem

If we add fields to `ReportData` and `FlowData`, old debug runs in `.repomap-runs/` (generated by previous versions of repomap) will not have those fields in their `snapshot.json`, `orientation_report.json`, or `flow_report.json` files. Running `repomap dev render-report` against an old run directory must not break.

### Current tolerance

`ReadRunDir()` currently uses unchecked type assertions throughout:

```go
fd.Summary, _ = fr["summary"].(string)      // missing → "" (zero value)
fd.Confidence, _ = fr["confidence"].(float64) // missing → 0.0 (zero value)
```

This means **old debug runs already produce valid (if sparse) `ReportData`**. Zero values propagate harmlessly. The existing HTML renderer handles missing fields by checking truthiness in JS (`if (f.summary)`, `if (f.likely_chain && f.likely_chain.length)`).

### Recommendations

1. **Additive changes only.** New fields added to Go structs use `omitempty` JSON tags. Old runs produce zero values for new fields; new runs include them. No migration needed.

2. **Do NOT version the debug artifact format.** The artifacts are a write-only log of one pipeline run. They are not a stable API. The report generator is the consumer, and it lives in the same binary.

3. **If a field must be renamed** (e.g., `selected_files_count` → `source_files_count`), support both old and new names in `ReadRunDir()` for one release:

   ```go
   // In ReadRunDir, when reading flow_bundle.json:
   if v, ok := fb["selected_files_count"]; ok {
       fd.BundleSummary.SelectedFilesCount = int(v.(float64))
   } else if v, ok := fb["source_files_count"]; ok {
       fd.BundleSummary.SelectedFilesCount = int(v.(float64))
   }
   ```

   Then drop the old name in the next release.

4. **JS renderer must be defensive.** Every field access in `script.js` should handle `undefined`/`null`:

   ```javascript
   // GOOD:
   if (f.recommended_first_flow) { ... }
   const time = f.reading_time_estimate || 0;
   
   // BAD:
   f.reading_time_estimate.toString()  // TypeError if undefined
   ```

5. **Test with old debug runs.** Keep a `testdata/old-run/` directory with artifacts from a previous version. The golden-file test should verify `ReadRunDir()` + `Render()` produces valid HTML without crashing.

---

## 6. Reproducibility

### Problem

If a user shares only the `report.html` file (not the `report.json` or debug artifacts), can we regenerate the structured data? Currently: **no**. The HTML embeds the JSON as `const DATA=<json>` in inline JavaScript, but:
- This is in executable JS context, not an extractable data block
- The JSON is not pretty-printed
- There's no standard ID or MIME type to locate it

### Recommendation: Embed as extractable data block

```html
<script type="application/json" id="report-data">
{"repo_name":"etcd","project_guess":"...","flows":[...]}
</script>
<script>
  const DATA = JSON.parse(document.getElementById('report-data').textContent);
  render();
</script>
```

Why:
1. **Standard MIME type** (`application/json`) — tools, browser extensions, and scripts can locate it
2. **Known ID** (`report-data`) — trivial to extract with a one-liner:
   ```bash
   # Extract report.json from report.html:
   grep -oP '<script type="application/json" id="report-data">\K.*(?=</script>)' report.html | python3 -m json.tool > report.json
   ```
3. **Self-contained.** The HTML carries its own structured data. Share the HTML; recipient can extract `report.json`.
4. **The JS still works.** `document.getElementById('report-data').textContent` returns the raw JSON string.

### Caveats

- The embedded JSON is the **enriched** `ReportData`, not the raw debug artifacts. This is acceptable: `report.json` is the canonical structured output. The debug artifacts are an implementation detail.
- If the user wants the raw `orientation_report.json` or `flow_report.json`, they need the `.repomap-runs/` directory. This is correct behavior — those are debug artifacts, not user-facing output.
- The `artifacts_dir` field in `report.json` preserves the link back to the debug run if it exists.

---

## 7. What NOT to Do

### Critical architectural mistakes to explicitly avoid

1. **Do NOT read source files in the report generator.**
   The report is built from debug artifacts (JSON files written during the pipeline). Adding `os.ReadFile("server/etcdserver/server.go")` inside `ReadRunDir()` or `buildHTML()` creates a hidden dependency on the source repo being present at render time. The HTML would not be shareable. If richer file data is desired (code snippets, line counts), it must be captured during the pipeline (in `flowexplain.SelectFlowFiles()` or `snapshot.Build()`) and stored in debug artifacts.

2. **Do NOT add external CDN dependencies.**
   No Google Fonts, no CDN-hosted JS libraries, no external CSS frameworks, no external images. The report must render correctly on `file://` protocol with no network access. This is a hard constraint from the feature spec.

3. **Do NOT add a JS build step.**
   No npm, webpack, vite, babel, or minification pipeline. The entire tool compiles with `go build` and must continue to do so. The JS is vanilla ES5/ES6 embedded via `//go:embed`. If the JS grows beyond ~200 lines, consider splitting it into multiple embedded files, but never a build step.

4. **Do NOT replicate the inline `fmt.Sprintf` HTML pattern.**
   The current `buildHTML()` function (70-line format string) is the primary technical debt. Any new UX code must use `//go:embed` with separate `templates/*.html`, `templates/*.css`, and `templates/*.js` files.

5. **Do NOT send full repo contents to DeepSeek for richer reports.**
   The invariant from `CORE_IDEA.md` is inviolable: "DeepSeek must never receive full repo contents, raw `file_tree`, or raw `internal_edges`." A richer UX must not become an excuse to expand the LLM bundle. The data displayed in the report is what DeepSeek already produced from the compact bounded bundle.

6. **Do NOT embed API keys or Authorization headers.**
   `debugdump.redactJSON()` already redacts 15 sensitive key names from debug artifacts. The report generator must not accidentally include unredacted data. The `report.json` and `report.html` are designed to be shareable.

7. **Do NOT add interfaces prematurely.**
   The report package needs exactly one interface: `Renderer`. Do not create `ReportDataLoader`, `FlowDataParser`, `ArtifactReader`, or other abstractions until there is a second implementation. The DX Review correctly notes that zero interfaces is a problem, but 15 interfaces is also a problem.

8. **Do NOT use `html/template` for the JSON data injection.**
   `html/template` auto-escapes `<`, `>`, `&`, and `"` in all inserted values. If the JSON blob contains a string like `"gRPC Put request via <stream>"`, it becomes `"gRPC Put request via \u003cstream\u003e"`, which is valid JSON but may break poorly-written parsers. Use `text/template` with `template.JSStr` for the JSON injection point.

9. **Do NOT make report generation dependent on the DeepSeek API key.**
   `ReadRunDir()` reads only local debug artifacts. `WriteReportHTML()` writes only local files. Neither should require `DEEPSEEK_API_KEY`. The `repomap dev render-report` command works offline — this must continue.

10. **Do NOT change the `likely_chain` DeepSeek prompt without an A/B test.**
    The current prompt asks DeepSeek for `files_to_read_in_order`, which the report parses into `likely_chain` steps. If the new UX wants richer step data (dependencies, data flow arrows between steps), the DeepSeek prompt must change — and prompt changes can degrade output quality. Run an A/B comparison against etcd before committing to prompt changes.

---

## 8. Interface Boundaries

### Current state

The `internal/report/` package has zero interfaces. Every function is a concrete implementation that reads/writes the filesystem. There are no tests (confirmed: `? github.com/dvordrova/repomap/internal/report [no test files]`).

### Recommended interfaces

**Exactly one: `Renderer`**

```go
// internal/report/renderer.go
package report

// Renderer converts ReportData into a self-contained output format.
type Renderer interface {
    Render(data *ReportData) ([]byte, error)
}
```

Implementations:
- `JSONRenderer` — produces pretty-printed `report.json` (already exists as `WriteReportJSON`, just needs the interface)
- `HTMLRenderer` — produces self-contained `report.html` (replaces `WriteReportHTML`)

### What this enables

1. **Golden-file tests:**
   ```go
   func TestHTMLRenderer(t *testing.T) {
       data := loadTestReportData(t)
       renderer := &HTMLRenderer{}
       got, err := renderer.Render(data)
       // compare against testdata/report.golden.html
   }
   ```

2. **Test without filesystem:**
   ```go
   func TestJSONRenderer(t *testing.T) {
       data := &ReportData{RepoName: "test"}
       renderer := &JSONRenderer{}
       got, _ := renderer.Render(data)
       var roundtrip ReportData
       json.Unmarshal(got, &roundtrip)
       // verify roundtrip
   }
   ```

3. **Future output formats** (Markdown, terminal, etc.) implement the same interface.

### What NOT to add yet

- **`ReportDataLoader` interface.** `ReadRunDir()` reads from disk. Adding a loader interface now is over-engineering for a single implementation. Tests can use `t.TempDir()` with real files on disk — this is standard Go practice.
- **`FlowData` behavior interface.** `FlowData` is a data struct. It has no behavior to abstract.
- **`ArtifactReader` interface.** The debug artifact format is not versioned and is consumed only by `ReadRunDir()`. An interface here would constrain the artifact format without benefit.

### Coordination with `orient` package

After decoupling (see section 4), the `orient` package no longer imports `report`. The dependency direction becomes:

```
cmd/main.go → orient (pipeline)
cmd/main.go → report (rendering)
report → (reads debug artifacts from disk, no code dependency on orient)
```

This is cleaner: `report` is a pure consumer of on-disk artifacts; `orient` is a pure producer. They communicate through the filesystem, not through Go types.

### Test strategy

| Test type | What | How |
|-----------|------|-----|
| Unit: `ReadRunDir` | Parses known debug artifacts correctly | `t.TempDir()` with pre-written JSON files |
| Unit: `EnrichReportData` | Computed fields are correct | Pure function, no I/O |
| Unit: `HTMLRenderer.Render` | Produces valid HTML with embedded JSON | Golden-file comparison |
| Unit: `JSONRenderer.Render` | Produces valid round-trippable JSON | Marshal → Unmarshal → compare |
| Integration: `Generate` | End-to-end report generation | `t.TempDir()` with real debug artifacts |
| Smoke: `scripts/smoke.sh` | Existing smoke test still passes | No change expected |

---

## Summary of Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Enrichment location** | Go for cross-flow computed fields; JS for presentational | Go has access to full `ReportData`; JS handles view concerns |
| **New fields** | Add `recommended_first_flow`, `reading_time_estimate`, `key_findings`, `generated_at` to `ReportData`/`FlowData` | Cross-flow computation requires Go; all use `omitempty` for backward compat |
| **Template engine** | `text/template` with `//go:embed`, NOT `html/template` | Avoid escaping JSON blob; separate CSS/JS files with syntax highlighting |
| **Pipeline boundary** | Move `writeRunReport()` from `orient.Run()` to `cmd/main.go` | Decouple pipeline from rendering; enable testing |
| **Report generation** | `report.Generate()` as standalone step; `dev render-report` becomes alias | Single entry point; works against any valid debug directory |
| **Backward compat** | Additive fields with `omitempty`; defensive JS; test with old-run fixtures | Old debug runs produce valid (sparse) reports without migration |
| **Reproducibility** | Embed JSON in `<script type="application/json" id="report-data">` | Standard MIME type; extractable; self-contained |
| **Interface** | Exactly one: `Renderer` with JSON and HTML implementations | Enables golden-file tests without over-engineering |
| **Filesystem** | `report` package reads debug artifacts from disk, not source files | Keeps HTML shareable; source file reading stays in pipeline |

## Recommended Implementation Order

1. **Extract HTML/CSS/JS to embedded files** (30 min) — prerequisite for any UX change
2. **Move `writeRunReport` out of `orient.Run()`** (30 min) — decouple pipeline from rendering
3. **Add `Renderer` interface and golden-file tests** (1 hour) — safety net for UX changes
4. **Add enriched fields to `ReportData` + `EnrichReportData()`** (30 min) — data contract
5. **Implement new dashboard UX in `script.js` and `style.css`** (2-4 hours) — the actual feature
6. **Update `report.json` to include new fields** (15 min) — backward compat via `omitempty`
7. **Add `<script type="application/json" id="report-data">`** (15 min) — reproducibility
8. **Test with old debug runs** (30 min) — backward compat verification
