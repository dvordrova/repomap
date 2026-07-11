# Go Architecture Review — Report Redesign for Developer Dashboard

**Verdict:** ✅ The UX proposal is sound, but the Go-side technical debt in
`internal/report/` must be addressed *first* — in lockstep with the HTML
redesign. Attempting the new dashboard on top of the current report package
(bare `map[string]interface{}` JSON parsing, 70-line `fmt.Sprintf` with inline
everything, zero tests) would turn a tactical UX fix into a maintenance
hairball. The work should be sequenced as: **(1) structural refactor of
`internal/report/` → (2) data model cleanup → (3) HTML/JS redesign**.

---

## 1. Package structure — single file must be split

**Current state:** `internal/report/report.go` (354 lines) has three distinct
concerns jammed into one file:

| Lines | Concern |
|-------|---------|
| 63–184 | `ReadRunDir` — JSON parsing from debug artifacts (snapshot.json, orientation_report.json, flow_bundle.json, flow_report.json) |
| 261–282 | `WriteReportJSON` / `WriteReportHTML` — serialization + rendering |
| 284–353 | `buildHTML` — inline HTML/CSS/JS template generation |

The proposed dashboard roughly doubles the HTML/CSS/JS complexity (vertical chain
cards, collapsible sections, numbered file lists, expandable "Technical Details",
sidebar layout). Putting that into the existing `buildHTML` would produce a
300-line Go string literal — completely unmaintainable.

**Recommendation — three files:**

```
internal/report/
  parse.go          # ReadRunDir + all low-level JSON parsing helpers
  render.go         # WriteReportJSON, WriteReportHTML, buildHTML (shortened),
                    # template execution, asset combination
  report.go         # Public types: ReportData, FlowData, ChainStep, etc.
                    # (moved from current report.go lines 12–61)
```

If the HTML/CSS/JS grows large enough, add embedded asset files:

```
internal/report/
  parse.go
  render.go
  report.go
  template.html     # //go:embed target
  style.css         # //go:embed target, optional
  script.js         # //go:embed target, optional
  report_test.go    # NEW — current coverage is nil
```

**Why this split:** A mid dev fixing a CSS bug opens `style.css`. A mid dev
fixing a data parsing bug opens `parse.go`. No more scrolling through a
single 350+ line file to find the right section.

**Edge case — keep it simple:** If `style.css` and `script.js` are <50 lines
each, embedding them as string constants in `render.go` is acceptable. The
pragmatic threshold: once any embedded asset exceeds 80 lines, extract it.

---

## 2. Go types and JSON parsing — kill `map[string]interface{}`

**Current state:** `ReadRunDir` uses `map[string]interface{}` with unchecked
type assertions for all four JSON artifact formats. This is fragile, unreadable,
and completely untestable in isolation. The codebase already *has* proper structs
for these formats in other packages:

| Artifact | Struct defined in | Package |
|----------|-------------------|---------|
| `orientation_report.json` | `orientResponse` (orient.go:249) | `orient` |
| `flow_bundle.json` | `FlowBundle` (flowexplain.go:23) | `flowexplain` |
| `flow_report.json` | `flowReportFields` (orient.go:71) | `orient` |
| `snapshot.json` | `Snapshot` (snapshot.go:28) | `snapshot` |

The report package should either **(A)** import and reuse these types, or **(B)**
define its own canonical types in `report.go` that exactly match the JSON
contract.

**Recommendation: Option B — `report.go` owns the canonical types.**

Rationale from the existing refactoring proposals (ARCHITECTURE_GO_REFACTOR.md,
section 5): the codebase already has duplicated conceptual types across packages
(`CandidateFlow` in `flowexplain` and `orient`, `scoredFile` in `llmbundle` and
`flowexplain`). If `report` imports types from `orient` and `flowexplain`, it
creates dependency cycles (`orient → report → orient`). By having `report`
define the canonical public types, other packages can eventually migrate to
them — or at minimum, `report` is independent.

However, a pragmatic middle ground: the debug artifact writing code in
`orient.go` uses `flowexplain.FlowBundle` and `flowReportFields`. So `report`
can import `flowexplain` for parsing `flow_bundle.json` without creating a
cycle. The types to define in `report` are:

```go
// report/report.go — canonical types for self-contained report
package report

// ReportData is the top-level container for the self-contained HTML/JSON report.
// It is computed by ReadRunDir() from debug artifacts and enriched for display.
type ReportData struct {
    RepoName       string     `json:"repo_name"`
    ProjectGuess   string     `json:"project_guess"`
    CandidateFlows []string   `json:"candidate_flows"`
    Flows          []FlowData `json:"flows"`
    ArtifactsDir   string     `json:"artifacts_dir"`
    Warnings       []string   `json:"warnings,omitempty"`

    // Enriched — computed by ReadRunDir, not present in any artifact.
    RecommendedFlow string `json:"recommended_flow,omitempty"` // ID of best flow
    FlowCount       int    `json:"flow_count"`
}

type FlowData struct {
    // ... existing fields from current report.go:21-33 ...

    // Enriched — human-readable labels computed in Go, not JS.
    ConfidenceLabel string `json:"confidence_label"` // "High", "Medium", "Low"
    BundleStatsLabel string `json:"bundle_stats_label"` // "23 files / 5 tests / 3 docs"
}

// ... ChainStep, FileItem, PathItem, BundleStats unchanged ...
```

The parsing functions (`parseSnapshot`, `parseOrientationReport`,
`parseFlowBundle`, `parseFlowReport`) become unexported helpers in `parse.go`
that each take `[]byte` and return a populated `*ReportData` or `FlowData`.
These use `json.Unmarshal` with proper struct types from `flowexplain` or
locally defined structs that match the artifact schemas.

**Never write another `map[string]interface{}` type assertion in this codebase.
The rule: every JSON artifact has a defined Go struct.**

---

## 3. Data enrichment — compute in Go, not JavaScript

The proposed dashboard needs computed values:

| Computed value | Where proposed | Recommendation |
|---------------|----------------|----------------|
| "Recommended first flow" | UX proposal | **Go** — in `ReadRunDir()` |
| Flow card F/T/P metrics | Overview cards | **Go** — as `BundleStatsLabel` |
| Confidence labels ("High"/"Medium"/"Low") | Flow pages | **Go** — as `ConfidenceLabel` |
| Bundle stats in "Technical Details" | Collapsible section | **HTML/JS** (just hide/show, no data logic) |
| Chain arrow connectors | Visual chain cards | **HTML/CSS** (pure presentation) |
| Collapse/expand behavior | Sidebar sections | **JavaScript** (interaction, no data logic) |

**Rationale:** Data transformations belong in Go because:
1. **Testable.** `makeConfidenceLabel(0.85) == "High"` is a pure function testable
   with table-driven tests. JavaScript in a `file://` SPA has no test harness.
2. **Consistent.** If the CLI text output (`formatHumanReadable`) later shows
   confidence labels, it uses the same Go logic.
3. **The JS stays thin.** The JavaScript in the SPA should handle rendering and
   interaction (tab switching, collapse toggles), not data transformation.

**Where to add enrichment:** In `ReadRunDir()`, after all flows are parsed,
before sorting and returning. Add a private function `enrich(data *ReportData)`:

```go
func enrich(data *ReportData) {
    // Find the recommended first flow: highest confidence, no error.
    for i := range data.Flows {
        data.Flows[i].ConfidenceLabel = confidenceLabel(data.Flows[i].Confidence)
        data.Flows[i].BundleStatsLabel = bundleStatsLabel(data.Flows[i].BundleSummary)
    }
    data.FlowCount = len(data.Flows)
    data.RecommendedFlow = findBestFlow(data.Flows)
}

func confidenceLabel(c float64) string {
    switch {
    case c >= 0.7: return "High"
    case c >= 0.4: return "Medium"
    default:       return "Low"
    }
}

func bundleStatsLabel(bs BundleStats) string {
    return fmt.Sprintf("%d files / %d tests / %d docs",
        bs.SelectedFilesCount, bs.SelectedTestsCount, bs.SelectedDocsCount)
}
```

Do **not** pre-populate `CandidateFlows` from the orientation artifact in
`ReadRunDir` with another `map[string]interface{}` loop — use a proper
`[]CandidateFlowName` struct.

---

## 4. Interface design — start small, add only what's needed

The ARCHITECTURE_GO_REFACTOR.md rightfully identifies "zero interfaces" as the
root pain point. For the report package specifically, the question is: what
interfaces enable testability of the new dashboard without over-engineering?

**Minimal viable interfaces:**

```go
// internal/report/render.go

// ReportRenderer produces the final output (HTML or JSON).
// Useful for testing: swap between filesystem and bytes.Buffer.
type ReportRenderer interface {
    RenderHTML(data *ReportData) ([]byte, error)
    RenderJSON(data *ReportData) ([]byte, error)
}
```

For `ReadRunDir`, an interface is **not** needed right now. The function reads
from the filesystem, and that's fine — tests can create temp directories
with synthetic artifacts using `testing.T.TempDir()`.

**What to avoid:** Do not create a `ReportWriter` interface that combines read
and write. Do not create an `ArtifactReader` interface for a single function.
Follow the Go proverb: "The bigger the interface, the weaker the abstraction."
Wait until a second implementation exists before extracting an interface.

**Exception:** If we want to test `orient.Run()` end-to-end with mocked
report generation, an interface at the `orient` boundary helps:

```go
// internal/orient/orient.go
type ReportGenerator interface {
    WriteRunReport(debugDir, runID string) error
}
```

This allows `orient.Run()` to accept a `ReportGenerator` parameter, defaulting
to the real `report` package. Tests inject a no-op or in-memory implementation.
But this is **optional** for the dashboard work — don't block the UX on
interface extraction.

---

## 5. Embedding with `//go:embed` — yes, with `text/template`

**Current state:** `buildHTML()` is a `fmt.Sprintf` call that interpolates two
Go strings (CSS, data JSON) into a 23-line format string. The CSS is a Go
string literal. The JavaScript is a Go string literal. There is no separation
of concerns.

**Recommended approach:**

```go
// internal/report/template.html — the HTML shell
//go:embed template.html
var templateHTML string

// internal/report/style.css — extracted CSS
//go:embed style.css
var styleCSS string

// internal/report/script.js — extracted JavaScript
//go:embed script.js
var scriptJS string
```

Then `render.go` composes them:

```go
import "text/template"

//go:embed template.html
var templateHTML string

var reportTmpl = template.Must(template.New("report").Parse(templateHTML))

type templateData struct {
    Title      string
    CSS        template.CSS       // mark as safe — we author it
    DataJSON   template.JS        // mark as safe — json.Marshal output
    Script     template.JS        // mark as safe — we author it
}

func buildHTML(title, dataJSON string) (string, error) {
    var buf strings.Builder
    err := reportTmpl.Execute(&buf, templateData{
        Title:    title,
        CSS:      template.CSS(styleCSS),
        DataJSON: template.JS(dataJSON),
        Script:   template.JS(scriptJS),
    })
    return buf.String(), err
}
```

**The template:**

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
  <!-- static HTML structure -->
  <header>...</header>
  <div class="container">...</div>
  <script>const DATA = {{.DataJSON}};</script>
  <script>{{.Script}}</script>
</body>
</html>
```

**Why `text/template` and not `html/template`:**
- `html/template` auto-escapes `{{.DataJSON}}`, which would turn `"flow-b"`
  into `&#34;flow-b&#34;` — breaking the JavaScript.
- We use `template.JS` and `template.CSS` types to mark trusted strings as safe.
  This is correct because: `dataJSON` is produced by `json.Marshal` (guaranteed
  valid JSON, no HTML injection possible), and CSS/JS are authored by us (no
  user input).
- `text/template` gives us layout control without fighting auto-escaping.

**Alternative — if templates feel heavy:** Keep a Go function that assembles
with `strings.Builder`:

```go
func buildHTML(title, dataJSON string) string {
    var buf strings.Builder
    buf.WriteString("<!DOCTYPE html>...<style>")
    buf.WriteString(styleCSS)
    buf.WriteString("</style>...<script>const DATA=")
    buf.WriteString(dataJSON)
    buf.WriteString(";</script><script>")
    buf.WriteString(scriptJS)
    buf.WriteString("</script>...")
    return buf.String()
}
```

This is acceptable for a single-page report that will never change structure —
but the template approach scales better when the HTML structure grows complex
(collapsible sections, sidebar, chain cards).

**Decision: use `text/template`.** It's in stdlib, adds zero dependencies, and
makes the HTML readable as HTML.

---

## 6. Error handling — accumulate, don't swallow

**Current state:** `ReadRunDir()` discards errors from `json.Unmarshal` with
`_` throughout (lines 72, 78, 110, 131). If `snapshot.json` is corrupted, the
repo name silently becomes `""`. If `orientation_report.json` is missing, all
candidate flows are silently omitted. The user sees a half-populated report
with no indication something went wrong.

**Recommendation — partial failure with a Warnings accumulator:**

```go
func ReadRunDir(runDir string) (*ReportData, error) {
    data := &ReportData{ArtifactsDir: absDir}
    var warnings []string

    // Parse snapshot — non-critical, collect warning
    if err := parseSnapshot(runDir, data); err != nil {
        warnings = append(warnings, fmt.Sprintf("snapshot: %v", err))
    }

    // Parse orientation report — non-critical
    if err := parseOrientationReport(runDir, data); err != nil {
        warnings = append(warnings, fmt.Sprintf("orientation: %v", err))
    }

    // Parse flows — critical for a useful report
    flows, flowWarnings, err := parseFlows(runDir)
    if err != nil {
        return nil, fmt.Errorf("read flows from %s: %w", runDir, err)
    }
    data.Flows = flows
    warnings = append(warnings, flowWarnings...)

    // Enrich before sorting
    enrich(data)

    sort.Slice(data.Flows, func(i, j int) bool {
        return data.Flows[i].ID < data.Flows[j].ID
    })

    data.Warnings = append(data.Warnings, warnings...)
    return data, nil
}
```

**Rules for error classification:**
- **Fatal:** Run directory does not exist → return `error`
- **Fatal:** No flow directories found → return `error`
- **Non-fatal:** Individual JSON file is missing or malformed → add to `Warnings`
- **Non-fatal:** Unrecognized fields in a known JSON file → silently ignore
  (forward-compatible), but log if >0 fields were skipped

The `parseFlowReport` helper should return `([]FileItem, []PathItem, []string, []string, error)`
instead of silently zeroing fields.

**What about the existing `_ = json.Unmarshal(...)` lines in `orient.go`?**
Those are a separate issue (pipeline producing data that "should never fail").
They should be `if err := ...; err != nil { panic(...) }` or logged with a
clear message. But that's out of scope for this review — focus on `report.go`.

---

## 7. Testing strategy — from zero to comprehensive

**Current state:** `go test ./internal/report` → `?` (no test files).
The most impactful thing we can do for the HTML redesign.

**Required test files:**

```
internal/report/
  report_test.go        # unit tests for types, enrichment, ReadRunDir
  render_test.go        # unit tests for WriteReportHTML, template execution
  testdata/
    minimal_artifacts/  # synthetic debug dir for ReadRunDir tests
      snapshot.json
      orientation_report.json
      flows/
        flow-grpc-put/
          flow_bundle.json
          flow_report.json
    report.golden.html  # golden file for HTML output
    report.golden.json  # golden file for JSON output
```

### 7a. Unit tests for enrichment functions

```go
func TestConfidenceLabel(t *testing.T) {
    tests := []struct{
        conf float64
        want string
    }{
        {0.9, "High"},
        {0.7, "High"},    // boundary: >=
        {0.69, "Medium"},
        {0.4, "Medium"},
        {0.39, "Low"},
        {0.0, "Low"},
        {-1.0, "Low"},    // bogus input, should still not panic
    }
    for _, tt := range tests {
        got := confidenceLabel(tt.conf)
        if got != tt.want {
            t.Errorf("confidenceLabel(%.2f) = %q, want %q", tt.conf, got, tt.want)
        }
    }
}

func TestFindBestFlow(t *testing.T) {
    t.Run("no flows", func(t *testing.T) {
        if got := findBestFlow(nil); got != "" {
            t.Error("expected empty string for nil flows")
        }
    })
    t.Run("error flow skipped", func(t *testing.T) {
        flows := []FlowData{
            {ID: "a", Confidence: 0.9, Error: "broken"},
            {ID: "b", Confidence: 0.3, Error: ""},
        }
        if got := findBestFlow(flows); got != "b" {
            t.Errorf("expected b (error flow skipped), got %s", got)
        }
    })
    t.Run("high confidence wins", func(t *testing.T) {
        flows := []FlowData{
            {ID: "a", Confidence: 0.5, Error: ""},
            {ID: "b", Confidence: 0.8, Error: ""},
        }
        if got := findBestFlow(flows); got != "b" {
            t.Errorf("expected b, got %s", got)
        }
    })
}
```

### 7b. Integration test for ReadRunDir with synthetic artifacts

```go
func TestReadRunDir(t *testing.T) {
    // Create a temp dir with synthetic artifact files.
    dir := t.TempDir()
    writeFile(t, filepath.Join(dir, "snapshot.json"),
        `{"repo_name":"etcd","readme":"..."}`)

    writeFile(t, filepath.Join(dir, "orientation_report.json"),
        `{"project_guess":"KV store","candidate_flows":[{"name":"gRPC Put"}]}`)

    flowDir := filepath.Join(dir, "flows", "flow-grpc-put")
    os.MkdirAll(flowDir, 0o755)
    writeFile(t, filepath.Join(flowDir, "flow_bundle.json"),
        `{"flow_seed":{"name":"gRPC Put"},"selected_files":[{"path":"a.go","kind":"source"}],"selected_tests":[],"selected_docs":[],"selected_packages":["pkg"],"related_edges":[]}`)

    writeFile(t, filepath.Join(flowDir, "flow_report.json"),
        `{"summary":"handles gRPC put","confidence":0.85,"likely_chain":[{"step":1,"name":"receive","what_happens":"gets request","evidence_files":["a.go"]}],"files_to_read_in_order":[{"path":"a.go","reason":"entrypoint","priority":1}],"tests_to_read":[],"unverified_paths":[],"unknowns":[],"warnings":[]}`)

    data, err := ReadRunDir(dir)
    if err != nil {
        t.Fatalf("ReadRunDir: %v", err)
    }

    if data.RepoName != "etcd" {
        t.Errorf("repo_name = %q, want %q", data.RepoName, "etcd")
    }
    if data.ProjectGuess != "KV store" {
        t.Errorf("project_guess = %q, want %q", data.ProjectGuess, "KV store")
    }
    if len(data.Flows) != 1 {
        t.Fatalf("expected 1 flow, got %d", len(data.Flows))
    }
    f := data.Flows[0]
    if f.ID != "flow-grpc-put" {
        t.Errorf("flow ID = %q", f.ID)
    }
    if f.ConfidenceLabel != "High" {
        t.Errorf("confidence label = %q, want High", f.ConfidenceLabel)
    }
}
```

### 7c. Golden file test for HTML output

```go
func TestWriteReportHTML_Golden(t *testing.T) {
    data := ReportData{
        RepoName:       "testrepo",
        ProjectGuess:   "test project",
        CandidateFlows: []string{"flow-a"},
        Flows: []FlowData{{
            ID: "flow-a", Name: "Test Flow",
            Summary:    "does something",
            Confidence: 0.75,
            ConfidenceLabel: "High",
            BundleStatsLabel: "5 files / 2 tests / 1 doc",
            LikelyChain: []ChainStep{{
                Step: 1, Name: "start", WhatHappens: "begins",
                Confidence: 0.8,
            }},
            FilesToRead: []FileItem{{Path: "main.go", Reason: "entrypoint", Priority: 1}},
        }},
        RecommendedFlow: "flow-a",
        FlowCount:        1,
    }

    var buf bytes.Buffer
    // Suppose we change the function signature to accept io.Writer
    err := writeReportHTMLTo(&buf, &data)
    if err != nil {
        t.Fatalf("writeReportHTMLTo: %v", err)
    }

    golden := filepath.Join("testdata", "report.golden.html")
    if *update {
        os.WriteFile(golden, buf.Bytes(), 0o644)
        return
    }

    want, err := os.ReadFile(golden)
    if err != nil {
        t.Fatalf("read golden: %v", err)
    }
    if !bytes.Equal(buf.Bytes(), want) {
        t.Errorf("HTML output differs from golden file.\nGot:\n%s\n---\nRun with -update to regenerate.", buf.String())
    }
}

var update = flag.Bool("update", false, "update golden files")
```

### 7d. Round-trip test for JSON serialization

```go
func TestJSONRoundTrip(t *testing.T) {
    data := ReportData{
        RepoName:     "test",
        Flows: []FlowData{{
            ID: "f1", Confidence: 0.5,
            FilesToRead: []FileItem{{Path: "x.go", Reason: "test"}},
        }},
    }

    b, err := json.Marshal(data)
    if err != nil {
        t.Fatal(err)
    }

    var got ReportData
    if err := json.Unmarshal(b, &got); err != nil {
        t.Fatal(err)
    }

    if got.Flows[0].FilesToRead[0].Path != "x.go" {
        t.Errorf("round-trip broken")
    }
}
```

### 7e. Edge case tests

```go
func TestReadRunDir_EmptyFlowsDir(t *testing.T) {
    dir := t.TempDir()
    os.Mkdir(filepath.Join(dir, "flows"), 0o755)
    writeFile(t, filepath.Join(dir, "snapshot.json"), `{"repo_name":"test"}`)

    data, err := ReadRunDir(dir)
    if err != nil {
        t.Fatal(err) // empty flows dir is not an error
    }
    if len(data.Flows) != 0 {
        t.Errorf("expected 0 flows, got %d", len(data.Flows))
    }
}

func TestReadRunDir_FlowWithError(t *testing.T) {
    dir := t.TempDir()
    flowDir := filepath.Join(dir, "flows", "bad-flow")
    os.MkdirAll(flowDir, 0o755)
    // flow_report.json is missing — should produce FlowData with Error field set
    writeFile(t, filepath.Join(flowDir, "flow_bundle.json"),
        `{"flow_seed":{"name":"bad"},"selected_files":[],"selected_tests":[],"selected_docs":[],"selected_packages":[],"related_edges":[]}`)

    data, err := ReadRunDir(dir)
    if err != nil {
        t.Fatal(err)
    }
    if len(data.Flows) != 1 {
        t.Fatal("expected 1 flow")
    }
    if data.Flows[0].Error == "" {
        t.Error("expected error field to be populated for missing flow report")
    }
}

func TestReadRunDir_MissingSnapshot(t *testing.T) {
    dir := t.TempDir()
    data, err := ReadRunDir(dir)
    if err != nil {
        t.Fatal(err)
    }
    if data.RepoName != "" {
        t.Error("expected empty repo name when snapshot missing")
    }
}
```

---

## 8. Performance — `fmt.Sprintf` is fine here

**Current concern:** `buildHTML()` creates a single giant string via
`fmt.Sprintf`. For reports with 20 flows, each with 50 chain steps, could this
be slow?

**Analysis:** The HTML report is written once per `repomap` invocation, not
streamed to 1000 requests/second. Even a 1MB HTML string (roughly 50 flows
with full data) takes <10ms to allocate and format. The bottleneck is
`ReadRunDir` (disk I/O), not `buildHTML` (memory allocation).

**Recommendation:** Keep `fmt.Sprintf` or `strings.Builder` — both are fine.
If using `text/template` (recommended above), it handles the builder internally.
No performance work needed.

**One caveat:** The JavaScript constant `const DATA = %s` embeds the entire
report as a JavaScript literal. For very large data, consider whether the JS
engine needs to parse all of it at once. But for the expected use case (<50
flows, <500KB JSON), this is perfectly fine on modern JS engines. If reports
grow beyond that, the fix is lazy loading in JS, not a Go-side change.

---

## 9. Specific "what NOT to do"

### ❌ Do not add any new `map[string]interface{}` code
The existing `ReadRunDir` is the *last* place this pattern exists. Replace it;
do not extend it. Every new JSON file parsed in the report package must use a
proper Go struct with `json:"..."` tags.

### ❌ Do not add external Go dependencies
The `go.mod` currently says `go 1.22` with zero `require` lines. This is a
deliberate design choice (see ARCHITECTURE_GO_REFACTOR.md, section "What a mid
dev should not change"). No testify, no chroma, no templating frameworks. All
the features needed (embedding, template rendering, JSON, file I/O) are in
stdlib. The proposal already respects this — the dashboard is HTML/CSS/JS, no
new Go deps needed.

### ❌ Do not add more functions to the single-file package without splitting
Adding `enrich()`, `parseSnapshot()`, `parseFlowReport()`, `parseOrientationReport()`
all to `report.go` would push it past 500 lines. Split into `parse.go` and
`render.go` on day one of this work.

### ❌ Do not add JavaScript libraries or CDN links
The constraint says "self-contained HTML, no CDN, works on `file://`". The inline
JS in the current implementation is ~20 lines. The dashboard proposal adds maybe
50-100 lines for collapsible sections, tab switching, and "Technical Details"
toggles. This is well within the sweet spot for vanilla JS. If the JS grows
beyond ~200 lines, consider whether the UX is over-engineered.

### ❌ Do not render the report from `orient.Run()`
Currently `orient.go:234-242` calls `writeRunReport()` synchronously inside the
pipeline. This blocks the terminal output until the HTML is written. The
ARCHITECTURE_GO_REFACTOR.md (section 14) recommends moving this to `cmd/main.go`.
The dashboard redesign is an opportunity to fix this: `orient.Run()` returns
data; `cmd/main.go` calls the report writer separately.

### ❌ Do not use `html/template` for the data injection
As explained in section 5, `html/template` will HTML-escape the JSON data
(`"` → `&#34;`), breaking `const DATA = ...`. Use `text/template` with
`template.JS` and `template.CSS` type markers, or use `strings.Builder`.

---

## 10. Backward compatibility — version the report format

When `report.json` or `report.html` fields change, old debug runs (e.g.,
`.repomap-runs/20260523-123456-etcd/`) will produce different or broken reports
if re-read.

**Recommendation — add a format version:**

```go
type ReportData struct {
    FormatVersion  int        `json:"format_version"` // 1 = current, 2 = dashboard
    RepoName       string     `json:"repo_name"`
    // ...
}
```

In `ReadRunDir`, if a debug directory has no `format_version`, treat as v1
(backward compat). The dashboard HTML template checks the version and renders
accordingly. Old debug dirs with v1 data get the old-style rendering (or a
graceful "this report was generated with an older version" message).

**Migration strategy:**
1. Add `FormatVersion: 1` to `ReportData`.
2. When the new dashboard ships, bump to `FormatVersion: 2`.
3. `ReadRunDir` can detect v1 vs v2 and populate enrichment fields
   (`ConfidenceLabel`, `BundleStatsLabel`, `RecommendedFlow`) differently — v1
   computes them from raw data, v2 may have them pre-serialized.
4. The HTML template checks `DATA.format_version` at render time and branches.

**Alternative — no versioning:** If old debug runs don't need to be
re-renderable with the new dashboard (nobody keeps `.repomap-runs` long-term),
then skip versioning. The dashboard always computes enrichment fields from raw
data. This is simpler and sufficient for the current use case. Only add
versioning if backward compat is explicitly requested.

---

## Summary of recommended file structure

```
internal/report/
  report.go          # Public types (ReportData, FlowData, ChainStep, etc.)
                     # + enrichment functions (confidenceLabel, findBestFlow, bundleStatsLabel)
                     # ~100 lines
  parse.go           # ReadRunDir + parseSnapshot, parseOrientationReport,
                     # parseFlowBundle, parseFlowReport
                     # All use proper Go structs, no map[string]interface{}
                     # ~200 lines
  render.go          # WriteReportJSON, WriteReportHTML, buildHTML,
                     # template execution via text/template
                     # ~60 lines
  template.html      # //go:embed — HTML structure, template directives
                     # ~40 lines
  style.css          # //go:embed — all CSS rules
                     # ~100 lines
  script.js          # //go:embed — all JavaScript
                     # ~80 lines
  report_test.go     # Unit tests: enrichment functions, JSON round-trip
                     # ~150 lines
  render_test.go     # Golden file test for HTML output, integration test
                     # ~120 lines
  testdata/
    minimal_artifacts/   # Synthetic debug dir for integration tests
    report.golden.html   # Golden file for HTML output
```

**Total: ~850 lines, well-factored, fully tested, zero new dependencies.**

## Implementation sequence (recommended order)

1. **Split file structure** — create `parse.go`, `render.go`, move types to
   `report.go`, make existing tests pass (`./scripts/check.sh`).
2. **Replace `map[string]interface{}`** — add proper JSON structs in
   `parse.go`, rewrite parsing functions. Add unit tests for each parser.
3. **Add enrichment** — `enrich()` function with `confidenceLabel`,
   `bundleStatsLabel`, `findBestFlow`. Tests for each.
4. **Extract assets** — create `template.html`, `style.css`, `script.js`.
   Use `//go:embed` + `text/template`. Golden file test.
5. **Redesign the dashboard** — now that the Go side is clean and tested,
   edit the HTML/CSS/JS freely with a CI safety net (golden file catches
   regressions, enrichment unit tests catch data bugs).
6. **Move `writeRunReport` to `cmd/main.go`** — decouple rendering from
   the pipeline. Optional but recommended while touching this code.

---

## Open question: should the report package import `flowexplain` types?

The debug artifacts written by `orient.go` use `flowexplain.FlowBundle` and
`orient.flowReportFields`. If `report` imports `flowexplain`, it can reuse the
`FlowBundle` and `FlowSeed` structs directly for parsing. If `report` imports
`orient`, it creates a cycle (`orient → report → orient`).

**Recommendation:** Have `report/parse.go` import `flowexplain` (for
`FlowBundle`, `FlowSeed` types) and define its own `flowReportJSON` struct
internally for parsing `flow_report.json`. This avoids the cycle. The internal
struct is a private implementation detail of `parse.go` — the public type is
`report.FlowData`, which is the canonical form.

```go
// report/parse.go
import "github.com/dvordrova/repomap/internal/flowexplain"

// flowReportJSON is the on-disk format of flow_report.json.
// Private — used only for JSON unmarshaling.
type flowReportJSON struct {
    Summary            string          `json:"summary"`
    Confidence         float64         `json:"confidence"`
    FilesToReadInOrder []fileToReadJSON `json:"files_to_read_in_order"`
    TestsToRead        []fileToReadJSON `json:"tests_to_read"`
    LikelyChain        []chainStepJSON  `json:"likely_chain"`
    UnverifiedPaths    []pathItemJSON   `json:"unverified_paths"`
    Unknowns           []string         `json:"unknowns"`
    Warnings           []string         `json:"warnings"`
}
```

This pattern (private JSON struct → public data struct) is clean, testable,
and avoids import cycles.
