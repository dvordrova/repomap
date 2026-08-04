# Decision: Question-aware Study coverage guard

Status: Approved by the repository owner through "продолжаем" after the v3.3
supervisor report.

## Product outcome

Make Study directions easier to audit by recording local evidence coverage
signals and by preventing selected documentation excerpts from being starved by
sequential budget fill.

## Approved implementation scope

1. Keep Study directions presentation-only. They remain learning plans, not
   runtime traces or canonical Mechanisms.
2. Add local debug/provenance coverage projection for Study directions:
   reading-anchor count, referenced document count, sourced document count,
   path-only documents, and reducer reasons such as missing clause coverage.
3. Do not show coverage diagnostics in the default user view.
4. Keep the same total document excerpt budget and document count; redistribute
   excerpt bytes more fairly across selected Study documents.
5. Add focused regression tests for path-only document diagnostics and fair
   excerpt allocation.

## Truth boundary

The model may propose questions and selected IDs, but local code remains
authoritative for whether referenced anchors exist, source is available, and
documents have saved excerpts. The new debug coverage is diagnostic, not a
publication verdict.

## Explicit non-goals

- No provider, prompt, retry, or canonical Mechanism change.
- No repository-wide analysis, runtime-surface discovery, global call graph, or
  target-repository command execution.
- No default UI exposure of gaps or diagnostics.
- No global limit increase.

## Focused verification

- Study Map document allocation tests.
- Study projection/debug coverage tests.
- `git diff --check`.
