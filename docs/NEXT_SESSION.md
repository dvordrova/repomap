# NEXT_SESSION.md — repomap Report UX Redesign

## Current project goal

Improve `report.html` UX from a raw technical dump into a developer dashboard that answers:
**"What should I open next, and why?"**

## What already works

### Phase 1 — Structural refactor (COMPLETE)
- `internal/report/report.go` — types + enrichment functions
- `internal/report/parse.go` — `ReadRunDir()` using proper Go structs (zero `map[string]interface{}`)
- `internal/report/render.go` — `WriteReportJSON()`, `WriteReportHTML()`, `Generate()` using `//go:embed` + `text/template`
- `internal/report/templates/report.html` — minimal HTML skeleton
- `internal/report/templates/style.css` — ~200 lines CSS with `rm-` prefix
- `internal/report/templates/script.js` — ~250 lines JS, named `render*` functions, DOM-based (not `innerHTML`)
- All existing tests pass (`./scripts/check.sh` green)

### Phase 2 — Tests (COMPLETE)
- 14 tests in `internal/report/report_test.go` (enrichment, ReadRunDir, edge cases)
- 3 tests in `internal/report/render_test.go` (golden file, file writes, Generate integration)
- Golden file at `internal/report/testdata/report.golden.html`
- Coverage: enrichment functions, JSON parsing, partial data, error states, missing files

### Phase 3 — Dashboard UX (COMPLETE — embedded in Phase 1 templates)
The CSS and JS already implement the full dashboard:
- Overview: "Start Here" flow card with blue accent, labeled metrics, Quick Start section
- Flow pages: waterfall timeline with step circles + connecting vertical line, evidence files
- "Known Unknowns" callout section
- Prominent numbered "Read Order" section with priority-based numbering
- Collapsible "Technical Details" (bundle stats), collapsed by default
- Error states: muted overview cards, user-facing messages (no raw Go errors)
- Zero single-letter abbreviations (F/T/P/S/D/E) in visible UI
- Embedded JSON in `<script type="application/json" id="rm-report-data">`
- CSS custom properties on `:root` for theming

## What is broken

**Nothing is broken.** All 17 report tests pass. `./scripts/check.sh` is green.

## What still needs doing

### Phase 4 — Pipeline decoupling (NOT STARTED)
The decision requires moving report generation out of `orient.Run()` into `cmd/main.go`:

1. Remove `writeRunReport()` call from `internal/orient/orient.go` (lines 222, 229)
2. Add `report.Generate()` call in `cmd/repomap/main.go` after `orient.Run()` returns
3. `repomap dev render-report` becomes an alias for `report.Generate()`

Files to edit:
- `internal/orient/orient.go` — remove lines 222-223 and 228-229 (the `writeRunReport` calls)
- `cmd/repomap/main.go` — add `report.Generate()` call in both `runDefault()` and `runOrient()`; update `runRenderReport()` to call `report.Generate()`

### Smoke test verification
Run `./scripts/smoke.sh` to verify end-to-end still works with the new report.

## Decisions already made

All from `docs/agent-room/050-decision.md`:
- `text/template` not `html/template` (avoids JSON escaping)
- `//go:embed` with separate CSS/JS/HTML files
- Enrichment computed in Go (not JavaScript)
- New fields: `FormatVersion`, `RecommendedFlow`, `FlowCount`, `ConfidenceLabel`, `BundleStatsLabel`
- All additive with `omitempty` — backward compatible
- `rm-` CSS prefix convention
- `document.createElement`/`textContent` not `innerHTML` for DOM construction
- Zero new Go dependencies
- No CDN, no build step, no React/Vite

## Open problems

None blocking. Known edge cases already tested (empty flows, missing files, malformed JSON).

## Exact next step for tomorrow

**Complete Phase 4 — pipeline decoupling (~30 min)**

1. Edit `internal/orient/orient.go`:
   - Remove `writeRunReport(...)` calls on lines ~222 and ~229
   - Keep everything else; `writeRunReport` function itself can stay as dead code or be removed

2. Edit `cmd/repomap/main.go`:
   - In `runDefault()`: after `orient.Run()`, call `report.Generate()` if `dDir != ""`
   - In `runOrient()`: same pattern
   - Replace `runRenderReport()` body with `report.Generate(rundir)`

3. Run `./scripts/check.sh` — must pass
4. Run `./scripts/smoke.sh` — verify end-to-end report generation

## Commands to run

```bash
# Quick check
./scripts/check.sh

# Run just report tests
go test ./internal/report/ -v

# Regenerate golden file (if template changes)
go test ./internal/report/ -run TestWriteReportHTML_Golden -update

# Smoke test (needs to be run after Phase 4)
./scripts/smoke.sh
```

## Important files

| File | Purpose |
|------|---------|
| `internal/report/report.go` | Types + enrichment |
| `internal/report/parse.go` | `ReadRunDir()` with struct-based JSON parsing |
| `internal/report/render.go` | `//go:embed`, `text/template`, `Generate()` |
| `internal/report/templates/report.html` | HTML skeleton |
| `internal/report/templates/style.css` | Dashboard CSS |
| `internal/report/templates/script.js` | Dashboard JS |
| `internal/report/report_test.go` | 14 tests |
| `internal/report/render_test.go` | 3 tests + golden file |
| `internal/report/testdata/report.golden.html` | Golden HTML output |
| `internal/orient/orient.go` | Pipeline orchestration — **needs Phase 4 edit** |
| `cmd/repomap/main.go` | CLI entry point — **needs Phase 4 edit** |
| `docs/agent-room/050-decision.md` | Full decision spec |

## Things we explicitly decided NOT to do

- No VS Code extension, LSP, gopls, AST, embeddings
- No React/Vite/npm/Webpack build step
- No CDN or external network resources
- No Go external dependencies (`go.mod` stays zero-`require`)
- No `html/template` (use `text/template` to avoid JSON escaping)
- No `map[string]interface{}` in new code
- No source file reading in the report generator
- No changing DeepSeek prompts
- No caching or session model
- No dark mode toggle (defer)
- No `file://` click-to-open links (defer to v2)
- No URL hash routing for tabs

## Known agent/opencode pitfalls

- `//go:embed` directives must use paths relative to the source file's directory
- `text/template` does NOT escape; `html/template` DOES escape — using `text/template` was intentional
- The `writeTestFile` helper takes `(t, dir, name, content)` — 4 args, not a full path
- Golden file tests need `-update` flag to regenerate; first run always needs it
- The `report` package imports `flowexplain` (for `FlowBundle` type) — that's the only cross-package import

## Suggested first prompt for tomorrow

```
Complete Phase 4 of the repomap report UX redesign:
1. Move report generation from orient.Run() to cmd/main.go
2. Remove writeRunReport calls from orient.go
3. Add report.Generate() calls in runDefault() and runOrient()
4. Update runRenderReport() to use report.Generate()
5. Run ./scripts/check.sh and ./scripts/smoke.sh
```
