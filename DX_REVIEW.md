# repomap — DX Review

> Imagine you're a mid-level developer who just inherited this repo on a Friday afternoon.
> This is what's going to hurt you.

---

## 1. This tool only works on one repo

Every file in this codebase claims to be a "generic repository orientation" tool. It is not. It is an etcd orientation tool that happens to accept a `--repo` flag.

**Where the body is buried:**

- `internal/gofacts/gofacts.go:375-398` — `classifyEntrypoint` switches on strings like `"server"`, `"etcdctl"`, `"etcdutl"`, `"tools/"`, `"contrib/"`, `"tests/"`. These are etcd directory conventions. Feed it any other Go project and every entrypoint becomes `"unknown"`.
- `internal/gofacts/gofacts.go:456-479` — `guessModuleRole` returns labels like `"server_runtime"`, `"client_library"`, `"api_definitions"`. These work for etcd's multi-module layout. For a Kubernetes or Docker codebase they are meaningless.
- `internal/llmbundle/llmbundle.go:256-339` — `scoreFile` is an 84-line function of hardcoded etcd keywords: `etcdserver`, `v3rpc`, `lease`, `mvcc`, `wal`, `backend`, `rafthttp`, `etcdctl`, `etcdserverpb`. Scoring for any non-etcd repo produces garbage.
- `internal/flowexplain/flowexplain.go:89-100` — `aliasExpansions` maps terms like `"put"→"kv/txn"`, `"watch"→"watcher/stream"`, `"lease"→"lessor/keepalive"`. These aliases make sense only if you know etcd internals.
- `internal/flowexplain/flowexplain.go:339-451` — `scoreFileLayered` applies a -100 point penalty for `"v2store"` and +40 for `"v3rpc"`. Those are etcd version migration artifacts. A mid-level dev on a different project will have no idea why their files score wildly wrong.

**What this means for you:** If the next task is "make this work for our Rust microservice repo," you are essentially rewriting the scoring, classification, and alias systems from scratch. None of it is pluggable. There are no interfaces, no configuration files, no strategy pattern. Just hardcoded strings.

---

## 2. Nothing is mockable. Nothing is testable.

The codebase defines **zero interfaces**. Every package calls concrete types directly:

- `gitfiles.List()` calls `exec.Command("git", ...)` — you can't test anything that calls it without a real git repo on disk.
- `gofacts.Load()` calls `exec.Command("go", ...)` — same problem, plus you need a valid Go module tree.
- `deepseek.NewFromEnv()` reads 4 environment variables from `os.Getenv()` — you can't inject a fake client, so every integration test needs a real API key and burns real API credits.
- `debugdump.NewWriter()` calls `os.MkdirAll` and `os.WriteFile` — you can't test error paths without manipulating filesystem permissions in tests.

The result: **the most important function in the codebase — `orient.Run()` — has zero test coverage**. The test file `internal/orient/validate_test.go` only covers two helper functions (`selectTopFlows` and `formatHumanReadable`). Not `Run`. Not `explainOneFlow`. Not the offline path. Not the DeepSeek path.

Other untested critical code:
- `cmd/repomap/main.go` — all CLI parsing, flag handling, `.env` loading: zero tests.
- `internal/report/report.go` — `ReadRunDir`, `WriteReportJSON`, `buildHTML`: zero tests.
- `internal/snapshot/snapshot_test.go` — tests `shouldSkipPath` and `truncateUTF8Bytes`. The main `Build` function: zero tests.
- `internal/gitfiles/gitfiles_test.go` — tests `splitNull`. `List` (the only exported function): zero tests.

**What this means for you:** Any refactor is blind. You change `orient.Run`, run `./scripts/check.sh`, it passes, you ship, and a bug surfaces in production because the test suite doesn't exercise the code path you changed.

---

## 3. Identical code exists in two places — and they've already diverged

This is the worst kind of duplication: same purpose, different packages, already diverging.

### `detectFileKind` / `detectKind`

- `internal/llmbundle/llmbundle.go:229` — `detectFileKind`: recognizes `.conf`, `.sample`, `.drawio`
- `internal/flowexplain/flowexplain.go:551` — `detectKind`: does **not** recognize `.conf`, `.sample`, `.drawio`

Both classify files as "test," "config," "doc," "go_source," "proto," etc. They share ~80% of their logic. But they live in separate packages so a fix to one won't reach the other. If you add `.cue` support to the LLM bundle detector, `flowexplain` will silently classify `.cue` files differently.

### `truncateUTF8Bytes` / `truncateStr`

- `internal/snapshot/snapshot.go:194` — `truncateUTF8Bytes`: uses `utf8.ValidString()` with incremental backoff
- `internal/llmbundle/llmbundle.go:408` — `truncateStr`: uses byte-level continuation detection (`s[cut]&0xC0 != 0x80`)

Same goal. Different algorithms. The byte-level version is harder to understand and harder to trust. If a UTF-8 edge case breaks one but not the other, debugging is a nightmare.

### `openFiles` path assembly

The pattern `pkgDir + "/" + fileName` with special handling for root packages (`"."` or `""`) appears in three files:
- `internal/gofacts/gofacts.go:577-584`
- `internal/llmbundle/llmbundle.go:120-126`
- `internal/orient/orient.go:275` (implicit)

### Retry loop

- `internal/deepseek/client.go:206-227` — `Orient()` retry loop
- `internal/deepseek/client.go:239-259` — `FlowExplain()` retry loop
- These are **identical** except for which function builds the request.

---

## 4. The report HTML is a 70-line Go string literal

`internal/report/report.go:284-353` embeds an entire single-page web application — HTML, CSS, JavaScript — inside a `fmt.Sprintf` call. The format string is 23 lines. The CSS is 40 lines of inline rules. The JavaScript is inline in a `<script>` tag.

If you need to:
- Add a new tab to the report → edit Go code, recompile, redeploy
- Fix a CSS layout bug → edit Go code, recompile, redeploy
- Add a filter or sort to the JS → edit Go code, recompile, redeploy
- Change a label or color → edit Go code, recompile, redeploy

There is no syntax highlighting on the CSS. No linting on the JS. No separation between the Go backend and the HTML frontend. This is how web apps were built in 1998.

Go has `//go:embed` since 1.16. Put the template in `report/template.html`, the CSS in `report/style.css`, the JS in `report/script.js`, embed them, and use `html/template`. This is 30 minutes of work that saves every future developer hours.

---

## 5. Prompts are embedded in Go source code

For a tool whose core value proposition is "ask DeepSeek about your codebase," the prompts are unmaintainable.

`internal/deepseek/client.go:90-167` — `buildRequest()` contains:
- The system prompt (lines 94-96)
- The user prompt with instructions (lines 99-161)
- The full JSON schema template DeepSeek should return (lines 104-151)
- All of it is a Go raw string literal

`internal/orient/orient.go:459-463` — `callDeepSeekForFlow()` also has prompt strings embedded in the function body.

If you need to iterate on the prompt (and you will — prompt engineering is the main tuning knob for AI quality):
1. Find the right Go file
2. Navigate to the right string literal
3. Edit it carefully (Go string escaping rules apply)
4. Recompile
5. Run the tool
6. Repeat

There is no hot-reload. No A/B testing. No prompt version history (beyond git). The prompts should be in `.txt` files loaded via `//go:embed` or read at startup.

---

## 6. Errors are swallowed everywhere

The codebase has a pattern of `result, _ := functionThatReturnsError()` — the error is discarded with `_`. This happens 12+ times across the codebase:

| Location | What's ignored |
|----------|---------------|
| `orient.go:107` | `s.JSON()` error |
| `orient.go:123` | `json.MarshalIndent(bundle)` |
| `orient.go:137` | `debugdump.NewWriter()` — if debug dir creation fails, ALL debug artifacts are silently lost |
| `orient.go:313` | `json.MarshalIndent(fb)` |
| `orient.go:457` | `json.MarshalIndent(fb)` |
| `orient.go:480` | `json.Unmarshal(raw, &pretty)` — whole `pretty` variable is unused |
| `report.go:72` | `json.Unmarshal(b, &snap)` |
| `report.go:78` | `json.Unmarshal(b, &or)` |
| `report.go:110` | `json.Unmarshal(b, &fb)` |
| `debugdump.go:108-109` | `os.MkdirAll` and `os.WriteFile` |
| `debugdump.go:117` | `WriteFile("error.txt", ...)` |

Some of these are defensible (e.g., `json.MarshalIndent` on a well-formed struct won't fail). But most are not. If `debugdump.NewWriter` fails because the disk is full or permissions are wrong, the user gets zero feedback — the tool continues as if debugging is enabled, but no files appear. They'll waste time looking in the wrong directory.

---

## 7. `orient.Options` is a 18-field god-struct

Every function in the pipeline receives this struct. Most functions use 2-3 fields. `explainOneFlow()` takes `opts Options` but only reads `opts.DumpLLM`. `llmbundle.Build()` takes its own separate `Options` with overlapping field names.

The struct itself is constructed identically in two places (`main.go` lines 97-115 and 156-176) with the same magic numbers. Changing a default requires editing both locations.

This is the classic "configuration blob" antipattern. Split into focused configs: `SnapshotConfig`, `LLMBundleConfig`, `FlowExplainConfig`, `DeepSeekConfig`. Use functional options or at least named defaults.

---

## 8. Magic numbers have no home

There is no central constants file. Default thresholds live in call sites:

| Value | Appears in | Meaning |
|-------|-----------|---------|
| `150` | `main.go`, `llmbundle.go` | Max files in LLM bundle |
| `120` | `main.go`, `llmbundle.go` | Max edges in LLM bundle |
| `50` | `flowexplain.go`, `orient.go` | Max files in flow bundle |
| `20` | `gofacts.go`, `llmbundle.go` | Max entrypoints / modules |
| `10` | `gofacts.go` | Max external imports per module |
| `500` | `gofacts.go`, `main.go` | Max internal edges |
| `300` | `gofacts.go`, `main.go` | Max Go packages |
| `200` | `snapshot.go` | Max interesting files |
| `400` | `snapshot.go`, `main.go` | Max file tree lines |
| `4` | `orient.go` | Default flow count |
| `5` | `gofacts.go` | Max priority ("primary_binary") |
| `30` | `llmbundle.go` | Max known docs |
| `32` | `snapshot.go` | Why 32? |

If someone asks "what's the maximum number of files repomap will ever send to DeepSeek?" you have to trace through four packages to find the answer. It should be one constant in one place.

---

## 9. The `facts interface{}` pattern

`internal/orient/orient.go:284`:

```go
func explainOneFlow(ctx context.Context, client *deepseek.Client, dw *debugdump.Writer,
    repoPath string, trackedFiles []string, facts interface{}, cf *gofacts.CandidateFlow, opts Options) (FlowResult, error) {
```

`facts` is typed as `interface{}`, then immediately type-asserted:

```go
gf, ok := facts.(*gofacts.Facts)
if !ok {
    return FlowResult{}, fmt.Errorf("facts must be *gofacts.Facts, got %T", facts)
}
```

This function is only ever called with `s.GoFacts` (which is `*gofacts.Facts`). The empty interface adds no flexibility, hides the type from IDEs, and requires a runtime check that can never fail in practice. Just use `*gofacts.Facts`.

---

## 10. `report.go` parses JSON by hand with `interface{}` casts

`internal/report/report.go:80-173` is a wall of `map[string]interface{}` → `[]interface{}` → `string` type assertions. This is what happens when you work with untyped JSON. Every field access requires:

```go
if m, ok := raw.(map[string]interface{}); ok {
    if v, ok := m["field"]; ok {
        if s, ok := v.(string); ok {
            // finally use s
        }
    }
}
```

The codebase already has proper struct types for DeepSeek responses (`chatRequest`, `chatResponse`, `flowReportFields` in `orient.go`). `report.go` should use those same types instead of re-parsing the raw JSON by hand.

There are also debug artifacts still in the code — `report.go:135-140` collects all JSON keys into a `keys` slice then immediately discards it with `_ = keys`. This is committed debug code.

---

## 11. Functions that do too much

| Function | File | Lines | Concerns |
|----------|------|-------|----------|
| `Load()` | `gofacts.go:128` | 106 | Module discovery, `go list` execution, entrypoint building, edge building, external imports, module summaries, orientation candidates, facts assembly |
| `ReadRunDir()` | `report.go:63` | 122 | Snapshot parsing, orientation parsing, flow directory iteration, flow bundle parsing, flow report parsing, report assembly |
| `scoreFileLayered()` | `flowexplain.go:339` | 113 | Scoring, etcd-specific penalties, etcd-specific boosts, kind detection |
| `SelectFlowFiles()` | `flowexplain.go:221` | 96 | File selection, test/doc separation, package selection, edge selection, nil-slice normalization |
| `Run()` | `orient.go:82` | 105+ | Snapshot building, LLM bundling, debug setup, DeepSeek orchestration, offline path, output formatting |

If you're asked to fix a bug in "flow file selection," you have to read 96 lines of `SelectFlowFiles` plus 113 lines of `scoreFileLayered` plus 96 lines of `selectPackagesAndEdges`. That's ~300 lines of dense scoring logic before you understand the full picture.

---

## 12. Inconsistent CLI parsing

`cmd/repomap/main.go` detects the default mode with:

```go
if !strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "orient" && os.Args[1] != "dev" {
```

Adding a fourth subcommand requires editing this condition. Ten subcommands means ten negations. This is an unrolled if-else chain masquerading as a router.

Use a `map[string]func()` or a proper subcommand library (even stdlib's `flag.NewFlagSet` for subcommands). The code is 240 lines of main — it doesn't need a framework, just structure.

---

## 13. The two UTF-8 truncation functions

Two different algorithms for the same problem in two different packages:

- `snapshot.truncateUTF8Bytes` — uses `utf8.ValidString()`, clean and readable
- `llmbundle.truncateStr` — uses manual byte masking `s[cut]&0xC0 != 0x80`, cryptic

Both work. One is obviously better than the other. If you need to change truncation behavior, you have to change it in two places, and the byte-level implementation is easy to break.

---

## 14. Dead code and misleading comments

- `orient.go:246-247` — Comment says `"reflection hack — but simpler"`. There is zero reflection anywhere in the function. The comment is completely wrong.
- `orient.go:433-434` — `ProjectGuessConfidence()` always returns `0.8`. The DeepSeek response has a real confidence value but it's ignored. The user always sees "80%" regardless of what DeepSeek actually returned.
- `orient.go:414-415` — `_ = chainLen; _ = confidence` — these variables are computed then explicitly discarded. Remove them or use them.
- `report.go:135-140` — Debug `keys` slice collected and discarded.
- `deepseek/client.go:261` — `doOrient()` is called for both orientation AND flow explanation requests. It's misnamed.
- `gofacts.go:137` — Variable named `absRepoPath` but it falls back to possibly-relative path on error.

---

## 15. No structured error handling

Every failure path calls `fmt.Fprintf(os.Stderr, ...)` or returns a `fmt.Errorf`. There are no error types, no sentinel errors, no error wrapping beyond basic `%w`. Scripts wrapping repomap have no way to distinguish "bad arguments" from "network error" from "DeepSeek returned nonsense" — they all produce exit code 1 and a text string on stderr.

---

## What to fix first (ordered by ROI)

| Priority | What | Why | Effort |
|----------|------|-----|--------|
| P0 | Extract etcd-specific logic behind interfaces | Tool is useless on any other repo | Days |
| P0 | Add integration test for `orient.Run` | Can't refactor safely without it | Hours |
| P1 | Define interfaces for git/go/deepseek/debug | Enables unit testing of everything above | Hours |
| P1 | Extract prompts and HTML template to files | Makes primary dev loops fast | Hours |
| P1 | Centralize magic numbers into a config package | Single source of truth | Minutes |
| P2 | Deduplicate `detectKind`, `truncateUTF8`, `openFiles`, retry loop | Eliminates divergence risk | Hours |
| P2 | Split god-functions (`Load`, `ReadRunDir`, `scoreFileLayered`) | Makes code understandable | Hours |
| P2 | Add proper struct types in `report.go` instead of `interface{}` casts | Safer, faster, cleaner | Hours |
| P3 | Clean up dead code, fix comments, remove `interface{}` params | Reduces confusion for next dev | Minutes |
| P3 | Standardize error handling with error types and sentinels | Enables scripting | Hours |
| P3 | Use `//go:embed` for HTML report | Separates concerns | Minutes |
