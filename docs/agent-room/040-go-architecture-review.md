# Go Architecture Review: Operational Flow Discovery

**Role**: Go Architect
**Date**: 2026-05-24
**Verdict**: Approved with structural notes

## 1. Where should operational flow candidate generation live?

**Recommendation**: New package `internal/opflows/`.

| Location | Pros | Cons | Verdict |
|----------|------|------|---------|
| `internal/gofacts/` | Already owns OrientationCandidate | gofacts does not scan content; adding signal-to-flow logic tangles concerns | Reject |
| `internal/sourcesignals/` | Already owns signals | Sourcesignals is a pure scanner; aggregation is a different responsibility | Reject |
| `internal/llmbundle/` | Already consumes signals | llmbundle bundles, doesn't discover flows | Reject |
| New `internal/opflows/` | Single responsibility, testable, deletable | One new package | **Accept** |

The new package should export one main function:
```go
func Discover(signals []sourcesignals.Signal, entrypoints []gofacts.Entrypoint) []gofacts.OrientationCandidate
```

This accepts signals and entrypoints as input (pure data, no I/O), and returns candidates that can be merged into the existing pipeline.

## 2. Return type

**Recommendation**: Reuse `gofacts.OrientationCandidate` with a new `Kind` value.

Add `Kind = "signal_flow"` to the existing `Kind` enum. The existing fields fit:
- `Name`: becomes the flow name (e.g., "Lease expiry background loop")
- `Kind`: "signal_flow"
- `EntrypointPackage`: the strongest-signal package
- `OpenFiles`: files with the strongest signals
- `Why`: human-readable evidence summary
- `Priority`: derived from signal weights and count

Do NOT create a new struct type. The existing pipeline from `OrientationCandidate` -> `CandidateFlow` -> focused flow bundle -> DeepSeek explanation works unchanged.

## 3. Error handling

**Recommendation**: Best-effort, never fatal.

```go
func Discover(signals []sourcesignals.Signal, entrypoints []gofacts.Entrypoint) ([]gofacts.OrientationCandidate, []string)
```

The second return value is a warnings slice. If signal scanning returned an error upstream (already handled), `Discover` receives an empty signal slice and returns no candidates + no warnings. If a signal has an unexpected shape or missing fields, skip it and add a warning. Never panic, never return an error that blocks the pipeline.

## 4. Test strategy

**Table-driven unit tests** with synthetic signal and entrypoint input:

```go
func TestDiscover_BackgroundLoop(t *testing.T) {
    signals := []sourcesignals.Signal{
        {Path: "server/lease/lessor.go", Line: 50, Category: "background_loop",
         Match: "time.NewTicker(500 * time.Millisecond)", Weight: 40},
        {Path: "server/lease/lessor.go", Line: 60, Category: "background_loop",
         Match: "for {", Weight: 25},
    }
    candidates, warnings := Discover(signals, nil)
    if len(candidates) == 0 {
        t.Fatal("expected at least one background loop candidate")
    }
    c := candidates[0]
    if c.Kind != "signal_flow" {
        t.Errorf("expected signal_flow kind, got %s", c.Kind)
    }
    if len(c.OpenFiles) == 0 {
        t.Error("expected open files from signal evidence")
    }
}
```

Integration test via `etcd_check.sh` or equivalent smoke test that runs the full pipeline against etcd and asserts that `orientation_candidates` includes signal_flow candidates.

## 5. Function signature testability

**Recommendation**: Accept `[]Signal` and `[]Entrypoint` as pure data, no I/O, no `context.Context`.

This keeps the function trivially testable:
- No filesystem access needed for tests
- No `go list` needed
- No network access
- Fake signals and entrypoints are enough

The caller (`orient.Run()` or `llmbundle.Build()`) is responsible for providing the already-computed signals and entrypoints.

## 6. Sorting and prioritization

**Recommendation**: Assign priority in Go code before passing to DeepSeek.

Priority formula per operational candidate:
```
base_priority = 2  (lower than primary_binary=5, cli=4, on par with example=2)
+ signal count bonus (min(total_signals / 3, 3))
- single_file_penalty (if all signals from 1 file: -1)
```

This produces priorities in the 2-5 range, placing strong operational candidates alongside request candidates. DeepSeek can still reorder via `confidence` in the orientation response.

## 7. Concurrency

**Recommendation**: Stay sequential. No goroutines needed.

The signal scanner processes files sequentially. Discovery aggregates signals sequentially. The entire pipeline from snapshot to bundle is sequential. Adding goroutines would add complexity (error propagation, ordering, resource limits) for no gain at this scale (200 signals, 200 files).

## Files to create/modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/opflows/opflows.go` | **Create** | Core discovery logic |
| `internal/opflows/opflows_test.go` | **Create** | Unit tests |
| `internal/gofacts/gofacts.go` | Modify | Add `Kind = "signal_flow"` to `buildOrientationCandidates`? No — add to `whyForKind` and `priorityForKind` switches only |
| `internal/flowexplain/flowexplain.go` | Modify | Add `FlowType` field to `CandidateFlow` |
| `internal/llmbundle/llmbundle.go` | Modify | Add `operational_flow_hints` to bundle, call `opflows.Discover` |
| `internal/orient/orient.go` | Modify | Merge operational candidates into orientation candidates list |
| `internal/deepseek/client.go` | Modify | Update orientation prompt for `flow_type` and operational evidence |
| `internal/report/report.go` | Modify | Pass `flow_type` through to report model |
| `internal/report/templates/script.js` | Modify | Render `flow_type` badge in HTML |
