# Decision: repomap Report UX Redesign

**Date:** 2026-05-24
**Status:** Decided
**Input reviews:** 010-product-review.md, 020-maintainability-review.md, 030-system-architecture-review.md, 040-go-architecture-review.md

---

## 1. User-Visible Goal

After running `repomap <repo>`, a developer opens `.repomap-runs/latest/report.html` and:

1. Immediately sees **what repo this is** (name, project guess, confidence)
2. Sees **which runtime/use-case flows** were found, with a clear **"Start Here"** recommendation
3. Clicks into a flow and sees a **waterfall timeline** of what happens, step by step
4. Finds a **prominent numbered "Read Order" section** answering "what should I open next, and why?"
5. Sees **tests and docs** supporting each flow
6. Sees **uncertainties and warnings** without them dominating the page
7. Can expand **"Technical Details"** for raw bundle stats if curious

The report answers: **"What should I open next, and why?"** — NOT "Here is all the JSON/debug data we generated."

---

## 2. Non-Goals (explicitly NOT included)

- VS Code extension, LSP, gopls, AST parsing, embeddings
- React, Vite, npm, Webpack, or any JS build step
- External CDN dependencies (fonts, frameworks, images)
- Caching or session persistence
- Dark mode toggle (defer; CSS custom properties make it easy later)
- URL hash routing for deep-linking into tabs
- `file://` click-to-open links on file paths (browser-dependent; defer to v2)
- Reading source file contents in the report generator
- Changing the DeepSeek prompts to produce richer step data
- Adding any Go external dependencies (`go.mod` stays zero-`require`)

---

## 3. Exact Layout Spec

### 3.1 Overview page (first thing user sees)

```
┌──────────────────────────────────────────────────────────────┐
│  repomap — etcd                                              │
│  Distributed key-value store (Go)                  🟢 High   │
│                                                              │
│  Artifacts: .repomap-runs/20260523-...  [expandable footer]  │
├──────────────────────────────────────────────────────────────┤
│  [Overview] [gRPC Put Request] [Server Startup] [Watch] ...  │
│                                                              │
│  ┌─ Candidate Flows ──────────────────────────────────────┐ │
│  │                                                         │ │
│  │  ▸ Start Here                                           │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │ gRPC Put Request                      🟢 High 85% │ │ │
│  │  │ Client sends Put via gRPC, server validates and   │ │ │
│  │  │ persists through Raft consensus...                │ │ │
│  │  │ 23 source files / 5 tests / 3 docs                │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  │                                                         │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │ etcd Server Startup                   🟢 High 82% │ │ │
│  │  │ Initialization sequence: config loading, peer     │ │ │
│  │  │ discovery, Raft cluster formation...              │ │ │
│  │  │ 18 source files / 4 tests / 2 docs                │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  │                                                         │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │ Watch Stream                          🟡 Med 55%  │ │ │
│  │  │ ...                                                 │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─ Quick Start ──────────────────────────────────────────┐ │
│  │ 1. gRPC Put Request    → server/etcdserver/server.go   │ │
│  │ 2. etcd Server Startup → server/etcdserver/server.go   │ │
│  │ 3. Watch Stream        → server/mvcc/watchable_store.go│ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  [Global warnings banner — amber, only if present]           │
└──────────────────────────────────────────────────────────────┘
```

Key rules:
- Flow cards sorted by: recommended first, then confidence descending
- Each card shows: flow name, confidence badge (labeled "High/Medium/Low N%"), truncated one-line summary, human-readable metric line ("23 source files / 5 tests / 3 docs" — NOT "F: 23 T: 5 P: 3")
- "Start Here" card has a visual accent (blue left border, pill badge)
- Quick Start section: 1-line-per-flow with the first file from each read order
- Global warnings in amber banner, only if present

### 3.2 Flow detail page (when user clicks a flow card)

```
┌──────────────────────────────────────────────────────────────┐
│  gRPC Put Request                              🟢 High 85%  │
│  Client sends Put via gRPC, server validates and persists    │
│  through Raft consensus.                                     │
│                                                    [Summary] │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─ Execution Chain ──────────────────────────────────────┐ │
│  │                                                         │ │
│  │  ● Step 1: Receive gRPC Request              🟢 90%    │ │
│  │  │  The client sends a Put request via gRPC to the     │ │
│  │  │  etcd server. Intercepted by gRPC interceptor.      │ │
│  │  │  ⚠ Low confidence in auth interceptor path          │ │
│  │  │  📄 server/etcdserver/v3_server.go (gRPC handler)   │ │
│  │  │  📄 api/etcdserverpb/rpc.proto (service definition) │ │
│  │  ├───────────────────────────────────────────────────── │ │
│  │  ● Step 2: Validate Request                   🟢 85%   │ │
│  │  │  Request validated for auth, quotas, lease           │ │
│  │  │  constraints.                                        │ │
│  │  │  📄 server/auth/simple_token.go                      │ │
│  │  ├───────────────────────────────────────────────────── │ │
│  │  ● Step 3: Apply to Raft State Machine        🟡 70%   │ │
│  │  │  Proposal sent through Raft, committed to log,       │ │
│  │  │  applied to mvcc store.                              │ │
│  │  │  📄 server/etcdserver/raft.go                        │ │
│  │  │  📄 server/mvcc/kvstore.go                           │ │
│  │  ├───────────────────────────────────────────────────── │ │
│  │  ● Step 4: Return Response                   🟢 92%    │ │
│  │  │  Response marshaled and sent back to client.         │ │
│  │  │  📄 server/etcdserver/v3_server.go                   │ │
│  │                                                         │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─ Known Unknowns ───────────────────────────────────────┐ │
│  │ How does auth interact with this flow?                  │ │
│  │ Exact lease quota enforcement point is uncertain        │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─ Read Order — Open these files in sequence ────────────┐ │
│  │                                                         │ │
│  │  1. server/etcdserver/server.go                         │ │
│  │     Entry point for the etcd server. Contains gRPC      │ │
│  │     service registration and request routing.           │ │
│  │                                                         │ │
│  │  2. server/etcdserver/v3_server.go                      │ │
│  │     Implements the v3 gRPC API methods including Put.   │ │
│  │     Start here to understand the request lifecycle.     │ │
│  │                                                         │ │
│  │  3. server/etcdserver/raft.go                           │ │
│  │     Raft proposal and apply logic. Connects the gRPC    │ │
│  │     handler to the consensus layer.                     │ │
│  │                                                         │ │
│  │  4. server/mvcc/kvstore.go                              │ │
│  │     MVCC key-value store where committed entries are    │ │
│  │     applied. See Put implementation here.               │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─ Tests ────────────────────────────────────────────────┐ │
│  │  server/etcdserver/v3_server_test.go                    │ │
│  │  server/etcdserver/raft_test.go                         │ │
│  │  server/mvcc/kvstore_test.go                            │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─ Technical Details  [▶ Expand] ────────────────────────┐ │
│  │  (collapsed by default)                                 │ │
│  │  Source files selected:    23                           │ │
│  │  Test files selected:       5                           │ │
│  │  Docs selected:             3                           │ │
│  │  Packages selected:         4                           │ │
│  │  Related import edges:      7                           │ │
│  │  Unverified paths:          2                           │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

Key rules:
- Waterfall timeline: vertical line connecting numbered step circles; each step card to the right
- Step cards show: number, name, what_happens, per-step confidence badge, evidence files (expandable), per-step warnings (inline badge on the step card)
- "Known Unknowns" callout: informational (blue/gray), between chain and read order
- Read Order: the most prominent section. Numbered list. Each item: file path (monospace), reason (prose), priority indicated by visual weight (P1 bold, P2 normal, P3 muted)
- Tests section: collapsible, shows paths with reasons if available
- Technical Details: collapsed by default, fully spelled-out labels, never single letters

### 3.3 Error and partial-data states

- **Flow with error (no AI analysis)**: Muted overview card with ⚠ icon, "Analysis unavailable" label. Detail page shows error explanation in red box; no empty sections.
- **Flow with partial data (bundle only)**: Shows file list and bundle stats. Chain section says "No AI explanation available — rerun with DEEPSEEK_API_KEY."
- **Flow with low confidence**: Shows all data normally; the confidence badge (amber/red with text label) already signals the uncertainty.
- **No flows at all**: Overview shows "No flows identified." Warnings displayed prominently. No empty tabs.
- **Empty optional fields**: If `LikelyChain` is empty, the section is absent (not rendered with an empty heading). Same for `TestsToRead`, `Unknowns`, etc.

---

## 4. report.json Data Contract Changes

### New fields added to `ReportData`

| Field | Type | JSON key | Purpose |
|-------|------|----------|---------|
| `FormatVersion` | `int` | `format_version` | `1` = current, `2` = dashboard. For backward compat. |
| `RecommendedFlow` | `string` | `recommended_flow` | ID of the best flow to start with. Computed in Go. |
| `FlowCount` | `int` | `flow_count` | `len(Flows)` — convenience for overview. |

### New fields added to `FlowData`

| Field | Type | JSON key | Purpose |
|-------|------|----------|---------|
| `ConfidenceLabel` | `string` | `confidence_label` | Human-readable: "High", "Medium", "Low". Computed in Go. |
| `BundleStatsLabel` | `string` | `bundle_stats_label` | "23 source files / 5 tests / 3 docs". Computed in Go. |

### Existing fields — no changes

All existing fields in `ReportData`, `FlowData`, `ChainStep`, `FileItem`, `PathItem`, `BundleStats` remain unchanged. Field names are not renamed. JSON tags are stable.

### What is NOT added

- Code snippets or line numbers in `ChainStep`
- Per-step dependency edges between chain steps
- Per-flow "difficulty" or "complexity" score
- Presentational fields (colors, icons, layout hints)

---

## 5. Implementation Plan

### Phase 1: Structural refactor (prerequisite — ~2 hours)

**Do NOT start the UX redesign until this phase is complete.**

1. **Split `internal/report/report.go`** into three files:
   - `report.go` — public types (`ReportData`, `FlowData`, `ChainStep`, `FileItem`, `PathItem`, `BundleStats`) + enrichment functions (`confidenceLabel`, `bundleStatsLabel`, `findBestFlow`, `enrich`)
   - `parse.go` — `ReadRunDir` + private parse helpers using proper Go structs (no `map[string]interface{}`)
   - `render.go` — `WriteReportJSON`, `WriteReportHTML`, template execution

2. **Replace `map[string]interface{}` parsing** in `ReadRunDir` with proper Go struct types:
   - Private `flowReportJSON` struct for parsing `flow_report.json`
   - Import `flowexplain.FlowBundle` for parsing `flow_bundle.json`
   - Private structs for `snapshot.json` and `orientation_report.json` fields
   - Zero new `map[string]interface{}` type assertions anywhere

3. **Add enrichment** (`enrich()` function):
   - `confidenceLabel(c float64) string` — "High"/"Medium"/"Low" with thresholds 0.7/0.4
   - `bundleStatsLabel(bs BundleStats) string` — "N source files / N tests / N docs"
   - `findBestFlow(flows []FlowData) string` — highest confidence flow without error, tiebreak by `len(FilesToRead)`

4. **Add new fields** to Go structs with `omitempty` JSON tags

5. **Extract HTML/CSS/JS** to embedded files using `//go:embed`:
   - `templates/report.html` — minimal HTML skeleton with `text/template` directives
   - `templates/style.css` — all CSS rules
   - `templates/script.js` — all JavaScript rendering logic
   - Use `text/template` (NOT `html/template`) to avoid JSON escaping issues
   - Embed JSON as extractable data block: `<script type="application/json" id="report-data">`

### Phase 2: Test foundation (~1.5 hours)

**Do NOT start the UX redesign until these tests pass.**

6. **Unit tests for enrichment functions** (`report_test.go`):
   - `TestConfidenceLabel` — boundary test at 0.7, 0.4, negative input, NaN
   - `TestBundleStatsLabel` — various counts
   - `TestFindBestFlow` — no flows, error flows skipped, highest confidence wins, tiebreak

7. **Integration test for `ReadRunDir`** (`report_test.go`):
   - Synthetic debug artifacts in `t.TempDir()`
   - Verifies repo_name, project_guess, candidate_flows, flows, enrichment fields

8. **Golden file test for HTML output** (`render_test.go`):
   - Known `ReportData` → HTML output compared against `testdata/report.golden.html`
   - `-update` flag to regenerate golden file
   - Deterministic output: flows sorted by ID, steps sorted by step number, files in JSON order

9. **Edge case tests**:
   - Empty flows directory
   - Missing `flow_report.json` (error flow)
   - Missing `snapshot.json`
   - Malformed JSON in artifact files
   - Flow with `Error` field set

### Phase 3: Dashboard UX redesign (~3-4 hours)

10. **Redesign `style.css`**:
    - Add `rm-` prefix to all CSS class names (collision prevention)
    - Waterfall timeline: vertical line, step circles, step cards
    - "Start Here" card accent
    - Collapsible sections (Technical Details, Tests)
    - Keep existing CSS custom properties on `:root` for theming
    - Keep system font stack and existing color palette
    - Add coding-specific monospace font stack for file paths

11. **Redesign `script.js`**:
    - Structure as named `render*` functions returning DOM elements (NOT strings):
      - `renderHeader(data)` → header with repo name, project guess
      - `renderFlowCard(flow, isRecommended)` → overview card
      - `renderChainSteps(chain)` → waterfall timeline
      - `renderFileList(files, label)` → numbered read order
      - `renderWarnings(warnings)` → warning banner
      - `renderUnknowns(unknowns)` → "Known Unknowns" callout
      - `renderBundleStats(stats)` → collapsible technical details
      - `renderErrorBox(error)` → error display
    - Use `document.createElement` / `textContent` (NOT `innerHTML` for DOM construction)
    - Guard every field access: `if (f.likely_chain && f.likely_chain.length)`
    - Collect all UI labels in a single `LABELS` object
    - DOM as state for tab switching and collapsible toggles (no separate state object)
    - Define `confClass()` and `confLabel()` helper functions
    - Clamp confidence values: `Math.max(0, Math.min(1, c))`

12. **Update `template.html`**:
    - Minimal structure with `{{.Title}}`, `{{.CSS}}`, `{{.DataJSON}}`, `{{.JS}}` insertion points
    - Embed JSON as `<script type="application/json" id="report-data">`
    - Semantic HTML: `<nav>` for tabs, `<ol>` for read order, `<section>` for major blocks
    - `<noscript>` fallback pointing to `report.json`

13. **Update golden files** — regenerate `testdata/report.golden.html`

### Phase 4: Pipeline decoupling (~30 min)

14. **Move report generation out of `orient.Run()`**:
    - Remove `writeRunReport()` call from `orient.go` lines 222, 229
    - Call `report.Generate()` from `cmd/main.go` after `orient.Run()` returns
    - `repomap dev render-report` becomes an alias for `report.Generate()`

---

## 6. Likely Files/Packages to Edit

| File | Change | Phase |
|------|--------|-------|
| `internal/report/report.go` | Refactor: types + enrichment only; move parse/render out | 1 |
| `internal/report/parse.go` | **NEW** — `ReadRunDir`, JSON parsing with proper structs | 1 |
| `internal/report/render.go` | **NEW** — `WriteReportJSON`, `WriteReportHTML`, template execution | 1 |
| `internal/report/templates/report.html` | **NEW** — `//go:embed` HTML skeleton | 1, 3 |
| `internal/report/templates/style.css` | **NEW** — `//go:embed` CSS rules | 1, 3 |
| `internal/report/templates/script.js` | **NEW** — `//go:embed` JavaScript | 1, 3 |
| `internal/report/report_test.go` | **NEW** — enrichment tests, ReadRunDir tests, edge cases | 2 |
| `internal/report/render_test.go` | **NEW** — golden file test for HTML, JSON round-trip | 2 |
| `internal/report/testdata/` | **NEW** — synthetic artifact fixtures, golden HTML/JSON | 2 |
| `internal/orient/orient.go` | Remove `writeRunReport()` call, return runID | 4 |
| `cmd/repomap/main.go` | Add `report.Generate()` call after pipeline, update `dev render-report` | 4 |

---

## 7. Tests Required

### New tests (Phase 2)

1. **`TestConfidenceLabel`** — table-driven: 0.9→High, 0.7→High, 0.69→Medium, 0.4→Medium, 0.39→Low, 0.0→Low, -1.0→Low (no panic)
2. **`TestBundleStatsLabel`** — various input/output pairs
3. **`TestFindBestFlow`** — nil flows, error flows skipped, highest confidence wins, tiebreak by file count
4. **`TestReadRunDir`** — synthetic debug dir with all artifacts; verify every field populates
5. **`TestReadRunDir_EmptyFlowsDir`** — empty flows dir, no error, 0 flows
6. **`TestReadRunDir_FlowWithError`** — missing `flow_report.json` → `FlowData.Error` populated
7. **`TestReadRunDir_MissingSnapshot`** — no `snapshot.json`, still returns valid `ReportData`
8. **`TestReadRunDir_MalformedJSON`** — corrupted `flow_report.json` → `FlowData.Error` populated, not a panic
9. **`TestWriteReportHTML_Golden`** — known `ReportData` → HTML matches golden file
10. **`TestJSONRoundTrip`** — marshal `ReportData` → unmarshal → fields match
11. **`TestWriteReportJSON_RoundTrip`** — write JSON to temp file → `ReadRunDir` reads back → data matches

### Updated existing tests

12. `scripts/check.sh` — must pass (go test + go vet)
13. `scripts/smoke.sh` — must pass (smoke test still generates valid HTML)

---

## 8. Acceptance Criteria

### Functional

- [ ] `repomap <repo>` generates `.repomap-runs/latest/report.html` automatically
- [ ] Overview page shows readable flow cards with labeled metrics (no F/T/P/S/D/E abbreviations)
- [ ] "Start Here" flow card is visually distinct and corresponds to the highest-confidence actionable flow
- [ ] Each flow page shows: summary, waterfall timeline, known unknowns, prominent Read Order, tests, technical details (collapsed)
- [ ] `likely_chain` is rendered as vertical step cards connected by a timeline line/arrows
- [ ] `files_to_read_in_order` is a prominent numbered section with file paths and reasons
- [ ] Warnings and unknowns are visible but secondary (inline on steps, callout section)
- [ ] "Technical Details" section (bundle stats) is collapsed by default, fully spelled out when expanded
- [ ] Error flows display gracefully with clear explanation (not raw Go error strings)
- [ ] Partial data (missing chain, missing files) does not show broken/empty sections
- [ ] Report renders correctly on `file://` protocol with zero network requests
- [ ] Embedded JSON is extractable from `<script type="application/json" id="report-data">`

### Technical

- [ ] Zero new Go external dependencies (`go.mod` stays zero-`require`)
- [ ] Zero `map[string]interface{}` type assertions in new code
- [ ] All CSS classes use `rm-` prefix
- [ ] All JS rendering uses named functions returning DOM elements
- [ ] `go test ./internal/report/...` passes with coverage >80%
- [ ] `./scripts/check.sh` passes (go test + go vet)
- [ ] `./scripts/smoke.sh` passes (end-to-end smoke test)
- [ ] Golden file test catches unintended HTML structure changes
- [ ] Debug artifacts never include API keys or Authorization headers in report data

### UX

- [ ] User can identify the recommended first flow within 5 seconds of opening the report
- [ ] User can find the Read Order section on any flow page within one scroll
- [ ] Conservative: can find answer to "what should I open next?" — the number one file path with reason
- [ ] Report looks like a developer dashboard, not a JSON dump
- [ ] No single-letter abbreviations in visible UI

---

## 9. What NOT to Do

1. ❌ Do NOT expand the existing `fmt.Sprintf` string literal — extract to embedded files first
2. ❌ Do NOT add any new `map[string]interface{}` code — use proper Go structs with `json:"..."` tags
3. ❌ Do NOT add external Go dependencies — stdlib only
4. ❌ Do NOT add a JS build step (npm, webpack, etc.)
5. ❌ Do NOT add CDN links or external network resources
6. ❌ Do NOT use `html/template` for JSON injection — use `text/template` with `template.JS`
7. ❌ Do NOT read source files in the report generator — report is built from debug artifacts only
8. ❌ Do NOT mix Go template rendering with JS rendering — Go injects data; JS builds DOM
9. ❌ Do NOT render the report inside `orient.Run()` — move to `cmd/main.go`
10. ❌ Do NOT show raw Go error strings in the HTML — translate to user-facing messages
11. ❌ Do NOT use single-letter abbreviations (F, T, P, S, D, E) anywhere in visible UI
12. ❌ Do NOT skip tests for `ReadRunDir` — it's the most fragile code in the package
13. ❌ Do NOT change DeepSeek prompts to get richer chain data — out of scope for this feature
14. ❌ Do NOT version the debug artifact format (yet) — additive changes with `omitempty` are sufficient

---

## 10. Implementation Order Summary

```
Phase 1 (Structural):  split files → replace map[string]interface{} → add enrichment → extract assets
Phase 2 (Tests):       enrichment tests → ReadRunDir tests → golden file test → edge cases
Phase 3 (Dashboard):   redesign CSS → redesign JS → update template → regenerate golden
Phase 4 (Decoupling):  move report generation from orient.Run() to cmd/main.go
```

**Estimated total: ~7-9 hours** across all 4 phases.

---

*Reviews informing this decision:*
- `docs/agent-room/010-product-review.md` — product/user advocate
- `docs/agent-room/020-maintainability-review.md` — maintainability reviewer
- `docs/agent-room/030-system-architecture-review.md` — system architect
- `docs/agent-room/040-go-architecture-review.md` — Go architect
