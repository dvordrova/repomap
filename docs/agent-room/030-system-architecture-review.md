# System Architecture Review: Operational Flow Discovery

**Role**: System Architect
**Date**: 2026-05-24
**Verdict**: Proceed with conditions

## Core decision: Stage 2.5, new `internal/opflows` package

Operational flow candidates should be produced by a new local deterministic package that aggregates `source_signals` into operational candidate flow structs. This sits between signal scanning and bundle assembly — after `sourcesignals.ScanFiles()` but before DeepSeek sees the bundle.

**Not in gofacts** — gofacts does not scan source content; it parses `go list -json` output for package structure.
**Not purely in the DeepSeek prompt** — the LLM would hallucinate operational flows without deterministic grounding.
**Not in llmbundle** — llmbundle is a bundler, not a flow discoverer.

## Updated pipeline

```
1. Local snapshot (git ls-files, README, go list)
   ↓
2. Source signal scanning (sourcesignals.ScanFiles)
   ↓
2.5. Operational flow discovery (opflows.Discover)  ← NEW
   ↓
3. Compact LLM bundle (llmbundle.Build)
   ↓
4. DeepSeek orientation (produces candidate_flows)
   ↓
5. Focused flow bundles + DeepSeek explanation (per flow)
```

## Data model

Add a single `flow_type` field to `flowexplain.CandidateFlow`:

```go
type CandidateFlow struct {
    Name             string   `json:"name"`
    Trigger          string   `json:"trigger"`
    FlowType         string   `json:"flow_type"`        // "request" | "operational"
    LikelyEntrypoint string   `json:"likely_entrypoint"`
    LikelyFiles      []string `json:"likely_files"`
    WhyInteresting   string   `json:"why_interesting"`
    Evidence         []string `json:"evidence"`
    Confidence       float64  `json:"confidence"`
}
```

No separate struct type. The pipeline from candidate -> focused bundle -> explanation is identical for both flow types.

## Bundle size management

**Problem**: Raw `source_signals` are 40-60KB in the orientation bundle. Adding operational flow hints on top could push the bundle past DeepSeek's practical effective-context window.

**Solution — two-tier signal representation**:

1. **Orientation bundle** (stage 3): Replace raw `source_signals` with an aggregated `operational_flow_hints` object. This is a compact summary (2-5KB) showing: for each operational category, the top files with signals, the strongest signal matches, and a suggested flow name.

2. **Focused flow bundles** (stage 5): Keep raw `source_signals` for the files selected for each flow. DeepSeek gets per-file scoped evidence when explaining a specific flow.

```json
// In orientation bundle
{
  "operational_flow_hints": {
    "background_loop": {
      "strongest_signals": [
        {"file": "server/lease/lessor.go:50", "match": "time.NewTicker(500 * time.Millisecond)"},
        {"file": "server/storage/mvcc/kvstore.go:120", "match": "time.NewTicker(100 * time.Millisecond)"}
      ],
      "affected_packages": ["server/lease", "server/storage/mvcc"],
      "suggested_flow": "Lease expiry background loop"
    },
    "admin_maintenance": {
      "strongest_signals": [
        {"file": "server/etcdserver/compactor.go:80", "match": "compact(...)"}
      ],
      "affected_packages": ["server/etcdserver"],
      "suggested_flow": "Compaction maintenance cycle"
    }
  }
}
```

## Offline mode

Works naturally. `opflows.Discover()` is purely local regex-based signal aggregation. In `--offline` mode, operational flow candidates are produced and included in the output. The user sees stub flows with "(offline hint)" suffix, clearly communicating that DeepSeek did not analyze them.

## Hallucination prevention

**Evidence threshold**: An operational candidate requires:
1. At least 2 source signals across >= 2 distinct files (showing cross-file structural evidence), OR
2. At least 1 signal with weight >= 35 (high-confidence single match like `time.NewTicker`)

**Confidence penalty**: If only path/name signals support the candidate (no structural signal from source scanning), confidence is capped at 0.3.

**Prompt guardrail**: The orientation prompt must include: "Operational flows must cite specific source_signal evidence. If evidence is weak (weight < 30 or single-file only), set confidence <= 0.3 and add a warning."

## Integration with existing flow selection

`selectTopFlows` already sorts by confidence. The function does not need changes — high-confidence operational flows naturally rise to the top alongside request flows. No separate selection logic needed.

## What not to do

- Do NOT send raw source signal snippets to DeepSeek in the orientation bundle (privacy, bundle size)
- Do NOT add AST parsing, LSP, or graph analysis for flow discovery
- Do NOT create a separate pipeline or UI section for operational flows
- Do NOT hardcode etcd-specific flow names into the discovery logic
- Do NOT add event-driven or reactive architectures (no channels, no goroutines for discovery)
