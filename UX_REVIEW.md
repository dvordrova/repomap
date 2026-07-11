# repomap — UX & User Flow Review

## Critical (blocks or confuses first-time users)

### 1. No `--help` or `--version` flags
Standard expectation: `repomap --help` prints usage. Currently usage prints only on missing/wrong args. `-h`/`--help` do nothing. Same for `--version`.

### 2. Mandatory `DEEPSEEK_API_KEY` with poor recovery
Default mode requires an API key but users learn this only after the pipeline runs. If the key is missing, the tool fails with an error after already doing snapshot+bundle work. Recovery message is good ("use `--offline`"), but the tool should detect this upfront before doing expensive file scans, and ideally offer a friendlier path.

**Suggestion:** Check env early. If missing, offer an interactive choice: "No API key found. Run offline? [Y/n]" or just print a clear message and exit before any work.

### 3. `.env` auto-discovery is invisible
`loadDotEnv()` reads `.env` from the current directory, but this is undocumented in usage output. Users reading only the README and CLI output think they must `export` the key. Document `.env` support in the usage text.

### 4. Confusing dual invocation syntax
```
repomap <repo>          ← shorthand (positional arg)
repomap orient --repo <repo>  ← legacy subcommand
```
Both work but use different syntax. The README shows both. Pick one canonical form and deprecate the other. The shorthand is better; `orient` subcommand should print a deprecation warning.

### 5. No progress feedback during slow operations
`git ls-files` and `go list -json ./...` can take 10+ seconds on large repos with zero output. Users stare at a blank terminal. At minimum, print "Scanning repository..." to stderr before the snapshot phase begins.

---

## High (real friction in daily use)

### 6. `--no-debug` silently overrides `--debug-dir`
If a user runs:
```
repomap ./repo --debug-dir /tmp/out --no-debug
```
The debug dir is silently ignored (set to `""`). Either make them mutually exclusive with an error, or document the override.

### 7. Output mixes stdout and debug paths
The human-readable output ends with `Artifacts: .repomap-runs/20260523-...`. A user piping output to a file or another tool gets this artifact path in the middle of their data. Artifact paths should go to stderr, not stdout.

### 8. `--out` flag doesn't create parent directories
`repomap ./repo --out reports/my-analysis.json` fails with a cryptic OS error if `reports/` doesn't exist. Create parent directories automatically.

### 9. No validation that `--flows` is reasonable
- Negative values silently fall back to 4 (no warning)
- `--flows 10000` makes 10,000 DeepSeek calls with no guard
- Cap it and warn.

### 10. No validation that repo path exists
Passing a non-existent path yields an opaque git error message. Check `os.Stat` early and provide a clear "directory not found" error.

### 11. `--json` mode still prints artifact paths to stdout
Same issue as #7, but worse because the Artifacts line breaks strict JSON output for programs that parse repomap's output.

---

## Medium (quality-of-life)

### 12. Silent error swallowing at debug writer
`dw, _ = debugdump.NewWriter(...)` ignores all creation errors. If the debug dir is not writable, the user gets no warning and debug artifacts are silently lost. At minimum, print a stderr warning.

### 13. `--offline` mode still requires a git repo
Offline mode must run `git ls-files`. This is by design, but first-time users might assume "offline" means "no external dependencies at all." Consider documenting this or using a plain file listing fallback.

### 14. No cache for expensive steps
Every run re-scans the entire repo. For a user iterating on the same repo (e.g., tweaking flow bundle parameters), this is wasteful. A `--cache` flag or `.repomap-cache` directory would make iteration faster.

### 15. DeepSeek errors surface late in the pipeline
Snapshot+LlmBundle work is done before the first API call. If the API is down or the key is wrong, 5-30 seconds of local work is wasted before the user learns this. Run a fast API connectivity check (or the orientation call itself) earlier.

### 16. Hardcoded repo type biases
`gofacts.go`, `flowexplain.go`, and `snapshot.go` contain etcd-specific and Go-ecosystem-specific terminology in interesting-word lists, alias maps, and scoring functions (e.g., `v2store` penalty, `v3rpc` boost, `etcdserverpb` aliases). For a Python or Rust repo, the scoring produces misleading weights and the tool offers no way to customize these.

### 17. No input sanitization on flow names
Flow names from DeepSeek are slugified (`go-grpc-put-request`) but never sanitized beyond that. Very long or unusual names could produce unusable filesystem paths in debug artifacts.

---

## Low (nice-to-have)

### 18. HTML report is auto-generated but not documented
The `dev render-report` subcommand generates a self-contained HTML SPA with tabs per flow, but it's completely undocumented in README or usage output. If the feature exists, surface it.

### 19. No `--quiet` flag
Debug mode writes many files. A `--quiet` flag to suppress non-error stderr output would help scripting.

### 20. No structured error codes
The tool uses `os.Exit(1)` for all failures. Scripts wrapping repomap can't distinguish "bad args" from "API error" from "repo not found" programmatically.

### 21. Unused confidence value
`formatHumanReadable` receives `confidence` but throws it away, hardcoding 0.8. If DeepSeek returns low confidence, the user should see that.

---

## Summary by impact

| # | Issue | Fix effort | User impact |
|---|-------|-----------|-------------|
| 1 | No `--help` / `--version` | Low | Very high |
| 2 | Late API key error | Medium | High |
| 4 | Confusing dual syntax | Low | High |
| 5 | No progress output | Low | High |
| 11 | JSON output corrupted by artifact line | Low | High |
| 6 | `--no-debug` vs `--debug-dir` conflict | Low | Medium |
| 7 | Artifact paths in stdout | Low | Medium |
| 8 | `--out` no parent dir creation | Low | Medium |
| 9 | No `--flows` guard | Low | Medium |
| 10 | No repo path validation | Low | Medium |
| 15 | Late API error surfacing | Medium | Medium |
| 16 | Hardcoded Go/etcd biases | High | Medium |
| 3 | Invisible `.env` support | Low | Low |
| 12 | Silent debug writer failures | Low | Low |
| 17 | No flow name sanitization | Low | Low |
| 13 | `--offline` git dependency unclear | Low | Low |
| 14 | No caching | High | Low (for now) |
| 18 | Undocumented HTML report | Low | Low |
| 19 | No `--quiet` | Low | Low |
| 20 | No exit codes | Low | Low |
| 21 | Unused confidence | Low | Low |
