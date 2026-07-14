# Decision: Operational Flow Discovery

**Date**: 2026-05-24
**Status**: Approved for implementation with conditions

Synthesized from:
- [010-product-review.md](010-product-review.md) — Product/user advocate
- [020-maintainability-review.md](020-maintainability-review.md) — Maintainability
- [030-system-architecture-review.md](030-system-architecture-review.md) — System architecture
- [040-go-architecture-review.md](040-go-architecture-review.md) — Go architecture

---

## User-visible goal

`repomap <repo>` discovers and explains operational flows alongside request-driven flows, with clear visual distinction. Example output:

```
━ gRPC Put request              [Request] ━
━ Raft log write path           [Request] ━
━ Lease expiry background loop  [Operational] ━
━ Backend quota enforcement     [Operational] ━
```

No new flags required. No separate UI section. The `[Request]` / `[Operational]` badge on flow cards provides at-a-glance distinction.

---

## Non-goals

- Do NOT implement a troubleshooting or incident-response assistant
- Do NOT add AST parsing, LSP, or graph analysis
- Do NOT create separate pipeline or UI sections for operational flows
- Do NOT hardcode etcd-specific flow names
- Do NOT add goroutines or concurrency
- Do NOT add `--max-operational-flows` command-line flag
- Do NOT add new third-party dependencies

---

## Implementation plan

### Phase 1: New package `internal/opflows/`

Create `internal/opflows/opflows.go` with one exported function:

```go
func Discover(signals []sourcesignals.Signal, entrypoints []gofacts.Entrypoint) ([]gofacts.OrientationCandidate, []string)
```

**Logic**: Groups signals by category. For each operational category (`background_loop`, `admin_maintenance`, `threshold_limit`, `consensus_state`, `storage_durability`), if the evidence threshold is met, produces one `OrientationCandidate` with:
- `Kind`: `"signal_flow"` (new value)
- `Name`: generated from category and top signal evidence
- `OpenFiles`: files with the strongest signals
- `Priority`: `base=2 + min(signal_count/3, 3)`; if all signals from 1 file, subtract 1
- `Why`: human-readable evidence summary citing matched snippets

**Evidence threshold**: At least 2 signals across >= 2 distinct files, OR 1 signal with weight >= 35.

**Max candidates**: 10 total per category, top 5 returned.

### Phase 2: Data model changes

1. **`internal/flowexplain/flowexplain.go`**: Add `FlowType string \`json:"flow_type,omitempty"\`` to `CandidateFlow`. Values: `"request"`, `"operational"`.

2. **`internal/gofacts/gofacts.go`**: Add `"signal_flow"` case to `priorityForKind` (return 2) and `whyForKind` (return "operational flow discovered from source signals").

3. **`internal/orient/orient.go`**: In `Run()`, after building the snapshot and before building the LLM bundle, call `opflows.Discover()` and merge the returned candidates into the orientation candidates list.

4. **`internal/llmbundle/llmbundle.go`**: Include operational flow candidates in the bundle (they pass through `goSection.OrientationCandidates` naturally).

### Phase 3: Prompt updates

**`internal/deepseek/client.go`**: Update the orientation prompt to:
- Accept `flow_type` in the `candidate_flows` output schema
- Require operational flows to cite source_signal evidence
- Cap confidence at 0.3 for weak-evidence operational flows
- Prefer strongest evidence regardless of flow type

### Phase 4: Offline mode support

**`internal/orient/orient.go`**: In `buildFlowBundlesFromSnapshot()`, call `opflows.Discover()` to produce operational flow candidates even in offline mode. Mark them with `(offline hint)` in the flow name.

### Phase 5: HTML report rendering

**`internal/report/templates/script.js`**: Render a `flow_type` pill badge next to flow names in:
- Overview flow cards
- Flow detail page headers

**`internal/report/templates/style.css`**: Add styles for `.rm-pill--request` and `.rm-pill--operational`.

---

## Likely files/packages

| File | Action | Lines (est.) |
|------|--------|-------------|
| `internal/opflows/opflows.go` | **Create** | ~200 |
| `internal/opflows/opflows_test.go` | **Create** | ~200 |
| `internal/gofacts/gofacts.go` | Modify (add signal_flow kind) | +5 |
| `internal/flowexplain/flowexplain.go` | Modify (add FlowType) | +3 |
| `internal/llmbundle/llmbundle.go` | Modify (pass candidates) | +5 |
| `internal/orient/orient.go` | Modify (call Discover, merge) | +30 |
| `internal/deepseek/client.go` | Modify (prompt update) | +30 |
| `internal/report/templates/script.js` | Modify (flow_type badge) | +15 |
| `internal/report/templates/style.css` | Modify (badge style) | +10 |

---

## Tests required

### Unit tests (`internal/opflows/opflows_test.go`)

1. Two background_loop signals across 2 files -> 1 candidate produced
2. One weak signal (weight < 35) in 1 file -> 0 candidates (below threshold)
3. Single signal weight >= 35 -> 1 candidate produced
4. Five admin_maintenance signals across 4 files -> 1 candidate, priority = 2 + min(5/3, 3) = 4
5. All signals from 1 file -> priority penalty applied
6. Empty signals -> no candidates, no errors
7. Request_handler signals -> 0 operational candidates (not an operational category)
8. Mix of categories -> correct grouping and count
9. Max 10 candidates total, top 5 returned
10. Warning produced for malformed signal (missing fields) — skipped gracefully

### Integration tests

11. `etcd_check.sh` (existing) should now assert operational candidates in output
12. Synthetic test repo with known background loop patterns -> verified in bundle

### Report tests

13. `flow_type` field present in report.json for operational flows
14. HTML contains `[Request]` and `[Operational]` badges
15. Offline mode produces operational stubs with `(offline hint)` suffix

---

## Acceptance criteria

1. `repomap ../etcd --offline` produces at least 1 operational flow candidate in the output
2. `repomap ../etcd` (with API key) produces operational flows with `flow_type: "operational"` in orientation_report.json
3. Report HTML shows `[Request]` and `[Operational]` badges on flow cards
4. No operational flow cites invented file paths (all paths validated against file index)
5. Existing request-driven flows continue to work unchanged (no regressions in test suite)
6. `go test ./...` passes; `go vet ./...` clean
7. `--llm-bundle-only` works without DEEPSEEK_API_KEY (no network access for discovery)
8. No hardcoded etcd-specific flow names in opflows package

---

## What not to do

- Do NOT add `--operational-flows` CLI flag (natural confidence sort is sufficient)
- Do NOT add YAML/JSON config files for signal rules (stay in Go code)
- Do NOT send full source snippets to DeepSeek in the orientation bundle (privacy, bundle size)
- Do NOT create separate HTML sections for operational flows (keep flat flow list)
- Do NOT modify `buildOrientationCandidates()` in gofacts (merge downstream)
- Do NOT add goroutines or concurrency
- Do NOT invent operational flows in the prompt without local deterministic evidence
