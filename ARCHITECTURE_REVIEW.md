# Architecture Review — Decisions & Future Considerations

Generated from a full codebase review of `repomap` (commit: current HEAD).

---

## ADR-000: Language Scope — Go-only or Multi-language?

**Status:** TBD (currently Go-only by design, per `CORE_IDEA.md` non-goals)

**Current state:** Every stage of the pipeline is Go-specific — `gofacts` runs `go list -json`, `flowexplain` has Go-centric scoring (v3rpc, etcdserver, mvcc, cobra), `llmbundle` entrypoint kinds are Go-only (primary_binary, cli, tool, etc.).

**Decision points:**
- Is multi-language support a long-term goal? If yes, the `gofacts` package must become a pluggable interface. If no, call it out in `CORE_IDEA.md` as an explicit non-goal with reasoning.
- Even if Go-only: should language-specific scoring constants live in `flowexplain` or be injected as a strategy?
- The `InterestingFiles` detection and `known docs` search in `snapshot.go` and `llmbundle.go` are already somewhat language-agnostic (path-based). This split between language-agnostic snapshot and Go-specific gofacts is intentional but undocumented.

**Recommendation:** Decide now. If Go-only forever, harden it by making the dependency on `gofacts` explicit in package naming and remove the `GoHints` field (rename to just `GoFacts`). If multi-language, define a `LanguageFacts` interface now before adding more Go-specific code to `flowexplain` and `orient`.

---

## ADR-001: LLM Provider Abstraction

**Status:** Not started

**Current state:** DeepSeek is the sole LLM provider, hardcoded in `internal/deepseek/client.go`. The `orient.go` orchestration directly instantiates `deepseek.NewFromEnv()`.

**What would change:** If OpenAI/Anthropic/local models were added, every call site references `deepseek.Client` directly.

**Decision points:**
- Should there be an `LLMClient` interface (`Orient(ctx, bundle) → json, FlowExplain(ctx, prompt) → json`)? Or is DeepSeek the permanent choice?
- Environment variable naming: `DEEPSEEK_API_KEY`, `DEEPSEEK_MODEL`, `DEEPSEEK_ENDPOINT`. If other providers are added, this naming becomes misleading.
- Prompt construction is coupled to the DeepSeek JSON mode requirement (`response_format: json_object`). Different providers have different JSON mode constraints.

**Recommendation:** Defer but plan. If provider abstraction is a future goal, extract an `LLMClient` interface in `internal/llm/` and rename env vars to `REPOMAP_LLM_*` now, before the surface area grows. Otherwise, explicitly declare DeepSeek-only as a design constraint in `docs/DEEPSEEK_API_NOTES.md`.

---

## ADR-002: Flow Analysis Depth — File Names Only or File Contents?

**Status:** TBD (mentioned as "planned, not implemented" in `CORE_IDEA.md`)

**Current state:** Flow bundles include file *paths* and package names, but not file *contents*. DeepSeek sees `selected_files: [{path: "server/etcdserver/server.go", kind: "source", score: 200}]` but not the code inside. This keeps the bundle compact but limits DeepSeek's ability to explain the flow.

**Decision points:**
- Should selected flow files have their contents included (truncated)? This would break the "no full repo contents" rule differently — sending *selected* contents with an explicit cap is different from sending everything.
- What's the priority? The current approach already produces plausible reports (see debug runs in `.repomap-runs/`). Does adding file contents meaningfully improve quality?
- If yes: add `max_flow_file_bytes` and `max_flow_files` limits to `FlowBundle`. Read and truncate file contents in `flowexplain.SelectFlowFiles()`.

**Recommendation:** Run an A/B comparison: one run with paths-only, one with the top 5 files' contents (head 200 lines each). Compare orientation quality. If it helps, add it with strict byte limits. The "never send full repo" invariant is preserved as long as per-flow limits are low.

---

## ADR-003: Persistent Snapshot Cache

**Status:** Not started

**Current state:** Every run re-executes `git ls-files` and `go list -json ./...` from scratch. For large repos (etcd: ~1500 Go files), `go list` takes 3-10 seconds. There is no caching.

**Decision points:**
- Should there be a `.repomap-cache/` directory with a TTL? Or is this premature optimization?
- What's the cache key? (repo root + git HEAD SHA + go.mod checksums)
- Should snapshot be cacheable by stage? (git ls-files → fast, `go list` → slow)
- Risk: stale cache could produce wrong orientation if code changed without a git commit.

**Recommendation:** Low priority but high value. Add a `--no-cache` flag and a simple file-based cache keyed on `git rev-parse HEAD` + go.mod SHA256. Invalidate on cache miss. This is a 20-line change in `snapshot.Build()` and saves 80%+ of runtime for repeated runs.

---

## ADR-004: Output Formats & Report Delivery

**Status:** Partially addressed (JSON + text stdout + HTML via `dev render-report`)

**Current state:** 
- Text output prints to stdout (`orient.Run()` returns `[]byte`)
- JSON output via `--json` flag writes to stdout
- HTML report via `repomap dev render-report <rundir>` reads debug artifacts
- No Markdown output, no terminal pager, no GitHub-flavored output

**Decision points:**
- Should Markdown be the primary output format? It renders well in terminals and on GitHub.
- Should `formatHumanReadable()` be extracted from `orient.go` into a `report/text.go` alongside `report/report.go` (HTML)?
- Should there be a `--format json|text|markdown|html` flag replacing `--json`?

**Recommendation:** Unify. Extract all rendering from `orient.go:formatHumanReadable()` into `internal/report/`. Add `--format` flag (default: `text`). Add Markdown renderer. The text renderer should use terminal width detection.

---

## ADR-005: Config File vs Environment Variables

**Status:** Not started

**Current state:** Configuration is split between:
- Environment variables: `DEEPSEEK_API_KEY`, `DEEPSEEK_MODEL`, `DEEPSEEK_MAX_TOKENS`, `DEEPSEEK_ENDPOINT`
- CLI flags: `--json`, `--offline`, `--flows`, `--debug-dir`, `--dump-llm`, `--max-llm-files`
- `.env` file: loaded if present (dotenv pattern in `loadDotEnv()`)
- Hardcoded defaults scattered across `cmd/main.go` and `orient.Options`

**Decision points:**
- Should there be a `.repomap.toml` or `.repomap.yaml` config file in the repo root?
- Should limits (MaxLLMFiles, MaxLLMEdges) be configurable per-repo? Some repos are bigger than others.
- Config precedence: CLI flag > env var > config file > default

**Recommendation:** Not urgent but worth designing. A `.repomap.toml` at the repo root could hold per-repo limits and exclude patterns, avoiding the current pattern of scattering defaults in `runDefault()` and `runOrient()` (note: these two functions duplicate the same defaults — `cmd/main.go:98-109` and `cmd/main.go:160-168`).

---

## ADR-006: Scoring System Consolidation

**Status:** Needs attention

**Current state:** There are **three independent scoring systems** with overlapping concerns:

1. **`snapshot.InterestingFiles`** (snapshot.go) — path/name-based, hardcoded interesting words. Produces `InterestingFiles` array in snapshot. Simple boolean: interesting or not.

2. **`llmbundle.scoreFile()`** (llmbundle.go) — scores files 0-100 for inclusion in `candidate_file_index`. Has domain-specific boosts (server=70, v3rpc=65, lease=65). Files ≤10 points are excluded.

3. **`flowexplain.scoreFileLayered()`** (flowexplain.go) — scores files for flow-specific selection. Has penalties (-20 patch, -100 v2store), basename/directory matches, term aliases, domain boosts.

**Problems:**
- Scoring 1 and 2 are partially redundant — both try to identify "interesting" files.
- Scoring 2 and 3 have different score scales and different boost values for the same domains.
- Adding a new domain keyword requires touching all three systems.
- Hardcoded etcd-specific terms (v3rpc, etcdserver, lease, mvcc, WAL, raft) are in the general-purpose tool's code.

**Decision points:**
- Should there be a single `internal/scoring/` package with a unified file scoring model?
- Should repo-specific domain knowledge (etcd terms) be extracted into a config or heuristics file rather than hardcoded?
- The `aliasExpansions` map in flowexplain.go is heavily etcd-biased (put→kv/txn/backend, lease→lessor/keepalive, raft→rafthttp/propose/wal). Should this be a configurable per-repo mapping?

**Recommendation:** Medium priority. Consolidate scoring into a single package. Keep domain heuristics configurable. The current code works for etcd but will be confusing for non-etcd Go repos (e.g., Kubernetes, Docker). At minimum, add a comment marking the etcd-specific constants.

---

## ADR-007: Test Strategy & Integration Coverage

**Status:** Needs discussion

**Current state:**
- Good unit test coverage: `deepseek` mocks HTTP, `gofacts` has 13 tests, `flowexplain` has 8, `llmbundle` has 10.
- **No integration tests** that call the real DeepSeek API.
- `smoke.sh` tests the snapshot+bundle pipeline against a synthetic tiny repo.
- `etcd_check.sh` tests snapshot+bundle against real etcd.
- `deepseek_check.sh` tests full pipeline against etcd + real DeepSeek API.
- No test coverage for the HTML report renderer.

**Decision points:**
- Should `deepseek_check.sh` be run in CI? It requires a `DEEPSEEK_API_KEY` secret.
- Should there be golden file tests for the report renderer?
- Should `orient.Run()` have an integration test with a small pre-baked repo?

**Recommendation:** 
- Add a CI job that runs `deepseek_check.sh` only when `DEEPSEEK_API_KEY` secret is available (skip otherwise).
- Add snapshot-based golden tests for `report.go` HTML output.
- Extract the small test repo creation from `smoke.sh` into a Go test helper so it can be used in unit tests.

---

## TECHNICAL DEBT

### T1. Duplicated Command-Line Defaults
`cmd/main.go:98-109` and `cmd/main.go:160-168` contain identical option defaults for `runDefault()` and `runOrient()`. If a limit changes, it must be updated in two places. Extract into `orient.DefaultOptions()` or a config struct.

### T2. Duplicated UTF-8 Truncation
`snapshot.truncateUTF8Bytes()` and `llmbundle.truncateStr()` implement the same logic differently. Consolidate into a shared utility (e.g., `internal/textutil/truncate.go`).

### T3. Duplicated Flow Report Parsing in `report.go`
`report.go:131-156` contains near-duplicate code for parsing flow reports in two different read paths. Refactor into a single helper.

### T4. Bubble Sort in `selectTopFlows`
`orient.go:selectTopFlows()` uses manual O(n^2) comparison sort instead of `sort.Slice`. Trivial fix, no performance impact at n=20, but a code smell.

### T5. `ProjectGuessConfidence()` Hardcodes 0.8
`orient.go:ProjectGuessConfidence()` returns `0.8` regardless of input. The TODO comment acknowledges this. Either remove the function and inline 0.8, or derive it from the response.

### T6. Makefile References Undefined `smoke-bundle` Target
`.github/workflows/ci.yml` runs `make smoke-bundle` but the Makefile has no such target. CI will fail on this step.

### T7. `CandidateFlow` Type Defined Twice
`flowexplain.CandidateFlow` (flowexplain.go:49) and an identical anonymous struct used in `orient.go:parseOrientationReport()`. Define once and share.

---

## SECURITY

### S1. `.env` Committed to Repository
The `.env` file containing `DEEPSEEK_API_KEY` is committed to git (visible in git history). This is a **critical** issue — the key must be rotated immediately. Add `.env` to `.gitignore` and use `.env.example` for documentation.

**Action required:** Rotate the DeepSeek API key, add `.env` to `.gitignore`, create `.env.example`.

### S2. Environment Variable Leakage via Debug Dumps
`debugdump.redactJSON()` redacts 15 key names from JSON but does not redact from plain text error messages or non-JSON files. If an error message includes the API key (e.g., in a URL or header), it could be written to `error.txt` unredacted.

### S3. Directory Permissions on Debug Dirs
Debug directories use mode `0700` (owner-only read/write/execute), which is appropriate. Verify this works correctly on all platforms (Windows may ignore Unix permissions).

---

## FUTURE CONSIDERATIONS

### F1. Progressive Degradation for Very Large Repos
Current limits are generous (150 files, 120 edges, 300 packages). For monorepos with 10,000+ Go packages, the tool will either:
- Hit `go list` timeout
- Exceed the 300-package cap silently (orient candidates may miss important packages)
- Send a bundle too large for the LLM context window

**Consider:** Add a `--repo-size auto|small|medium|large|monorepo` flag that adjusts limits automatically. Truncate with clear warnings rather than silently dropping packages.

### F2. Non-Git Repositories
The tool requires `git ls-files`. What about repos without git (tarballs, zip downloads)? Add a fallback to `os.ReadDir` recursive walk if git is unavailable.

### F3. `.repomapignore` File
Similar to `.gitignore`, repos may want to exclude custom paths from analysis (e.g., generated protobuf dirs, vendored forks not in vendor/). The current skip list in `snapshot.go` is hardcoded.

### F4. Parallel Go Module Loading
`gofacts.Load()` processes modules sequentially. For repos with multiple `go.mod` files (e.g., etcd has server/, etcdctl/, tools/), parallel loading would speed up the pipeline.

### F5. Streaming Output
The current pipeline is batch: snapshot → bundle → LLM → output. For large repos, showing progress (snapshot done, bundle done, calling LLM, explaining flow 1/4...) would improve UX. Add a `--progress` flag that writes to stderr.

### F6. `go.sum` Generation
There is no `go.sum` because the project has zero external dependencies. This is valid Go (modules with no deps don't need go.sum), but some tooling expects it. If any dependency is ever added, `go.sum` will appear automatically.

### F7. Symlink Handling Edge Cases
`gofacts.normalizePackagePaths()` handles symlinks but has known edge cases: if a symlink points outside the repo root, the tool silently excludes it. Should this be a warning? An error? Currently it's silent — packages outside the repo are dropped with no indication.

---

## ARCHITECTURE DIAGRAM (Current State)

```
┌──────────────────────────────────────────────────────────┐
│                   cmd/repomap/main.go                     │
│              CLI parsing, .env loading, routing            │
└──────────────────────┬───────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────┐
│                 internal/orient/orient.go                  │
│         Pipeline orchestration, report formatting          │
└──┬─────────┬──────────┬──────────┬───────────┬───────────┘
   │         │          │          │           │
   ▼         ▼          ▼          ▼           ▼
┌──────┐ ┌──────┐ ┌────────┐ ┌────────┐ ┌──────────┐
│git-  │ │snap- │ │gofacts │ │llm-    │ │flowexplain│
│files │ │shot  │ │        │ │bundle  │ │          │
└──────┘ └──┬───┘ └───┬────┘ └───┬────┘ └─────┬────┘
            │         │          │            │
            └─────────┴──────────┴────────────┘
                       │
                       ▼
              ┌────────────────┐
              │ deepseek/client│
              └───────┬────────┘
                      │
                      ▼
              ┌────────────────┐
              │  debugdump     │
              └────────────────┘

Pipeline flow:
  gitfiles.List() → snapshot.Build() → gofacts.Load()
  → llmbundle.Build() → deepseek.Orient() → flowexplain.SelectFlowFiles()
  → deepseek.FlowExplain() → format output

All packages in internal/ — no public API surface.
All I/O is filesystem + git subprocess + HTTP.
```

### Dependency Graph

```
orient ──→ snapshot ──→ gitfiles
  │              └──→ gofacts
  ├──→ llmbundle ──→ gofacts
  ├──→ flowexplain ──→ gofacts
  ├──→ deepseek
  └──→ debugdump
report (standalone, reads debug artifacts)
```

---

## PRIORITY MATRIX

| Priority | Item | Effort | Impact |
|----------|------|--------|--------|
| **P0 - Critical** | S1: Rotate API key, add `.env` to `.gitignore` | 5 min | Security |
| **P0 - Critical** | T6: Fix `smoke-bundle` Makefile target or CI config | 5 min | CI is broken |
| **P1 - High** | T1: Deduplicate CLI defaults | 15 min | Maintenance |
| **P1 - High** | ADR-006: Consolidate scoring or document split | 2-4 hours | Correctness for non-etcd repos |
| **P1 - High** | S2: Redact API keys in non-JSON error messages | 30 min | Security |
| **P2 - Medium** | T2: Consolidate UTF-8 truncation | 15 min | Consistency |
| **P2 - Medium** | T3: Deduplicate report parsing | 30 min | Maintenance |
| **P2 - Medium** | T4: Fix bubble sort | 2 min | Code quality |
| **P2 - Medium** | T5: Fix or remove hardcoded confidence | 1 hour | Correctness |
| **P2 - Medium** | T7: Share `CandidateFlow` type | 15 min | Consistency |
| **P3 - Low** | ADR-003: Snapshot cache | 1-2 hours | Performance |
| **P3 - Low** | ADR-004: Unified output formats | 2-3 hours | UX |
| **P3 - Low** | F1: Progressive degradation | 2-4 hours | Robustness |
| **P4 - Future** | ADR-000: Multi-language decision | Design only | Architecture |
| **P4 - Future** | ADR-001: LLM provider abstraction | Design only | Architecture |
| **P4 - Future** | ADR-002: Flow file contents | Experiment first | Quality |
| **P4 - Future** | ADR-005: Config file | Design + 2 hours | UX |
| **P4 - Future** | ADR-007: Integration test coverage | 3-5 hours | Quality |
| **P4 - Future** | F2-F7: Nice-to-haves | Varies | Various |
