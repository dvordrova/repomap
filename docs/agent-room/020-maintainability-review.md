# Maintainability Review: Operational Flow Discovery

**Role**: Maintainability Reviewer
**Date**: 2026-05-24
**Verdict**: Proceed with guardrails

## 1. Where should operational flow discovery live?

**Recommendation**: New package `internal/opflows/`, not in `gofacts`, `sourcesignals`, or `llmbundle`.

Rationale:
- `gofacts` already handles `go list -json` parsing, entrypoint detection, edge building. Adding signal-to-flow logic would push it well past its single responsibility.
- `sourcesignals` is a pure scanner — takes files, returns signals. Aggregating signals into flow candidates is a different concern.
- `llmbundle` is a bundle assembler, not a flow discoverer.
- A dedicated `internal/opflows/` package with one main exported function `Discover(signals []sourcesignals.Signal, entrypoints []gofacts.Entrypoint) []CandidateFlow` keeps the concern isolated, testable, and easy to replace or delete.

## 2. Pattern list maintainability

**Recommendation**: Convert `buildPatterns()` in `internal/sourcesignals/` to a declarative Go table.

Current state: `buildPatterns()` is a function body with repeated `add(...)` calls. Adding a category requires editing the function and the category list.

Better:
```go
type SignalRule struct {
    Category string
    Pattern  string
    Weight   int
    Reason   string
}

var signalRules = []SignalRule{...}
```

The function body compiles the regexps at package init time. Adding a rule is one line. No YAML/JSON config files (AGENTS.md rule: no new dependencies). No runtime config loading (bundle mode and offline mode must work without filesystem access beyond the repo).

If the rule list exceeds ~150 lines, split into a separate file `internal/sourcesignals/rules.go` with the table, keeping `sourcesignals.go` focused on scanning logic.

## 3. Before or after DeepSeek orientation?

**Recommendation**: Before DeepSeek. Produce operational candidates locally, then include them in the orientation bundle for DeepSeek to prioritize and explain.

Rationale:
- **Testable**: The local function is pure Go, no API key needed, trivially testable with fake signals.
- **Bounded**: The local function can enforce strict limits (max 5 candidates, evidence threshold >= 2 signals across >= 2 files).
- **Offline-compatible**: `--offline` mode gets operational flows without any LLM involvement.
- **Prompt-safe**: The orientation prompt can use the local candidates as hints rather than asking DeepSeek to discover them from raw signals.

## 4. Test surface

**Recommendation**: Three test layers.

1. **Unit: signal-to-flow mapping** — Table-driven tests feeding synthetic `[]Signal` slices and checking that `Discover()` produces the correct candidate names, evidence counts, and priorities. No filesystem, no `go list`, no DeepSeek.

2. **Unit: invariants** — Dedicated tests for each invariant (evidence threshold, path validation, count caps, no duplicate candidates).

3. **Integration: synthetic repo** — A small Go repo with known source patterns checked into testdata. `snapshot.Build()` runs the full local pipeline. Assert that `orientation_candidates` includes both entrypoint-based and signal-based candidates. This is the smoke-test that proves the new package wires correctly into the existing pipeline.

## 5. Preventing category explosion

**Rule**: Every source signal category must map to exactly one operational flow kind. No synonyms. No overlapping regexps.

Current categories map naturally:
- `background_loop` -> "background loop" operational flow (ticker, timer, goroutine)
- `admin_maintenance` -> "admin/maintenance" flow (compaction, defrag, snapshot)
- `threshold_limit` -> "threshold/enforcement" flow (quota, NOSPACE, limit exceeded)
- `consensus_state` -> "consensus transition" flow (leader election, step, apply)
- `storage_durability` -> "durability/storage" flow (WAL sync, backend commit)

`request_handler`, `observability`, `scope_marker` do NOT produce operational flows. They support existing request flows.

If a new category is added, the developer must answer: "What operational flow kind does this produce?" before adding it. If the answer is "it doesn't", it goes in a support-only list.

## 6. Boundedness

Current limits:
- Orientation bundle: 200 source signals, 150 file index entries
- Flow bundle: 30 source signals

**Recommendation**: 5 operational flow candidates by default, max 10. Configurable via `--operational-flows N` with default 5.

Why 5: At `--flows 4`, the top 4 overall (request + operational) are shown. With 5 operational candidates and ~8 request candidates, the mix is natural. At `--flows 8`, all candidates appear.

The function `Discover()` returns max 10 candidates sorted by priority. The calling code takes top N (default 5). If fewer than N candidates meet the evidence threshold, return only those.

## 7. Keeping existing pipeline unchanged

**Non-negotiable**: Do not modify `buildOrientationCandidates()` in `gofacts`. It continues to produce entrypoint-based candidates exactly as before.

**Merge point**: `orient.Run()` currently passes `s.GoFacts.OrientationCandidates` through to the LLM bundle. The new code appends operational candidates to this list before building the bundle. This is a single-line change in `orient.Run()` or `llmbundle.Build()`.

The existing `CandidateFlow` struct, `SelectFlowFiles`, `explainOneFlow`, and the DeepSeek flow explanation pipeline all work unchanged for operational candidate flows — they receive a name, trigger, likely files, and build a focused bundle the same way.

## 8. Invariants (must not break)

1. Every operational candidate must cite >= 1 source signal with file path and line number.
2. No operational candidate may include file paths not present in the file index or signal evidence.
3. Total candidates (entrypoint + operational) must respect global limits (default 20 in `goFacts`, max 10 operational).
4. The source signal scanner must never be invoked twice for the same file set (once in `llmbundle.Build`, once in `explainOneFlow`) — reuse the already-computed signals.
5. `--llm-bundle-only` and `--offline` must work without crashing, even if operational flow discovery fails. Degrade gracefully: return entrypoint candidates only, add a warning.
