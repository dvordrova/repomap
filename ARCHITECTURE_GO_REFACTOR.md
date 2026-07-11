# Go Architecture Refactoring — for Future Maintainers

This document is written assuming a mid-level Go developer inherits this codebase
and will need to add features, fix bugs, and make architectural decisions.
Every recommendation is concrete, with before/after code sketches.

---

## 1. NO INTERFACES — THE ROOT OF ALL PAIN

**Problem:** Every package depends on concrete implementations. There is not a single
interface anywhere in `internal/`. A maintainer cannot mock, cannot test in isolation,
cannot swap implementations without touching call sites.

**Concrete coupling map:**

```
orient.Run() depends on:
  snapshot.Build()           ← concrete function, calls git + go list
  llmbundle.Build()          ← concrete function
  deepseek.NewFromEnv()      ← concrete constructor, reads env vars
  flowexplain.SelectFlowFiles() ← concrete function
  debugdump.NewWriter()      ← concrete function, writes to disk
```

This means:
- To unit-test `orient.Run()`, you need a real git repo on disk
- To swap the LLM provider, you must rewrite `orient.go`
- To test flow explanation logic, you must call the real `flowexplain` package

**What to do:**

Extract 3-4 interfaces. Start small.

```go
// internal/llm/client.go
type Client interface {
    Orient(ctx context.Context, bundleJSON []byte) (json.RawMessage, error)
    FlowExplain(ctx context.Context, userPrompt, systemPrompt string) (json.RawMessage, error)
}

// internal/source/scanner.go
type Scanner interface {
    Snapshot(ctx context.Context, repoPath string) (Snapshot, error)
}

// internal/viz/writer.go
type ReportWriter interface {
    WriteText(report Report) ([]byte, error)
    WriteJSON(report Report) ([]byte, error)
    WriteHTML(report Report) error
}
```

Then `orient.Run()` accepts these as parameters instead of calling concrete functions.
The existing concrete implementations stay as the default, but a mid dev testing a new
feature can inject mocks. A mid dev adding OpenAI support implements `llm.Client`
without touching `orient.go`.

**Priority: HIGH.** This is the single change that makes everything else possible.

---

## 2. ORIENT IS A GOD PACKAGE — SPLIT IT

**Problem:** `internal/orient/orient.go` is 487 lines with ~6 distinct responsibilities:

| Lines | Responsibility |
|-------|---------------|
| 21-44 | Options struct definition |
| 46-92 | Report type definitions (combinedReport, orientationPart, explainedFlow, flowBundleSummary, flowReportFields, fileToOpen, unverifiedPath) |
| 94-232 | Pipeline orchestration (Run) |
| 234-247 | HTML report generation side-effect |
| 249-253 | orientResponse parsing type |
| 255-279 | Offline flow bundle construction |
| 281-341 | Single flow explanation orchestration |
| 343-431 | Human-readable text formatting |
| 433-435 | Hardcoded confidence |
| 437-454 | Bubble sort |
| 456-487 | DeepSeek flow call wrapper |

**What to do:** Split into at least 3 packages:

```
internal/orient/          ← pipeline orchestration only (Run)
internal/orient/parse.go  ← LLM response parsing (orientResponse → combinedReport)
internal/orient/format.go ← human-readable text + JSON output
```

The types living at the top of `orient.go` (combinedReport, explainedFlow, etc.) 
should move into `internal/report/` where they belong — `report` already has 
file-reading code for these exact structures.

**What a mid dev gains:**
- When adding a `--markdown` output format, they add `format_markdown.go`, not another
  50 lines into the already-bloated `Run()` function.
- When debugging LLM response parsing, they look at `parse.go` (~50 lines), not a
  487-line file.
- `Run()` shrinks to ~60 lines: build snapshot → build bundle → call LLM → parse → format.

**Priority: HIGH.** After interfaces, this is the next prerequisite for maintainability.

---

## 3. OPTIONS THREADING IS MANUAL AND FRAGILE

**Problem:** Options are copied field-by-field across package boundaries with different
`Options` structs in every package.

```go
// orient.go:95-102 — manual field copy
s, err := snapshot.Build(snapshot.Options{
    RepoPath:            opts.RepoPath,
    MaxReadmeBytes:      opts.MaxReadmeBytes,
    MaxTreeLines:        opts.MaxTreeLines,
    MaxInterestingFiles: opts.MaxInterestingFiles,
    MaxGoPkgs:           opts.MaxGoPkgs,
    MaxGoEdges:          opts.MaxGoEdges,
})

// orient.go:116-122 — another manual field copy
bundle := llmbundle.Build(s, s.FilteredFiles, llmbundle.Options{
    MaxReadmeBytes: opts.MaxReadmeLLMBytes,
    MaxModules:     opts.MaxLLMModules,
    MaxEntrypoints: opts.MaxLLMEntrypoints,
    MaxFiles:       opts.MaxLLMFiles,
    MaxEdges:       opts.MaxLLMEdges,
})

// cmd/main.go:98-109 and cmd/main.go:160-168 — duplicated defaults
```

Adding a new `--max-foo` flag requires:
1. Add field to `orient.Options` 
2. Add field to `snapshot.Options` or `llmbundle.Options` (whichever it affects)
3. Add manual assignment in `Run()` 
4. Add flag in `cmd/main.go` (twice if both `runDefault` and `runOrient` need it)
5. Add default value (twice)

**What to do:** Use a single Config struct that flows through the pipeline, plus
functional options at the package boundary.

```go
// internal/config/config.go
type Config struct {
    RepoPath string

    // Limits
    MaxReadmeBytes      int
    MaxReadmeLLMBytes   int
    MaxTreeLines        int
    MaxInterestingFiles int
    MaxGoPkgs           int
    MaxGoEdges          int
    MaxLLMFiles         int
    MaxLLMEdges         int
    MaxLLMModules       int
    MaxLLMEntrypoints   int
    FlowCount           int

    // Mode
    Offline         bool
    SnapshotOnly    bool
    LLMBundleOnly   bool
    FlowBundlesOnly bool
    OutputJSON      bool

    // I/O
    DebugDir     string
    DumpLLM      bool
    DumpRedacted bool
    RunID        string
}

func Default(repoPath string) Config {
    return Config{
        RepoPath:            repoPath,
        MaxReadmeBytes:      20000,
        MaxReadmeLLMBytes:   6000,
        MaxTreeLines:        400,
        MaxInterestingFiles: 200,
        MaxGoPkgs:           300,
        MaxGoEdges:          500,
        MaxLLMFiles:         150,
        MaxLLMEdges:         120,
        MaxLLMModules:       20,
        MaxLLMEntrypoints:   20,
        FlowCount:           4,
    }
}
```

Then `orient.Run(ctx, config, llmClient)` receives a single Config. Packages that
only need a subset accept subset interfaces or a narrower options struct —
but the *pipeline orchestration* no longer manually threads 20 fields.

**Priority: MEDIUM.** Not urgent but every new flag adds pain. Fix before the next
feature flag is added.

---

## 4. SCORING IS A HARDCODED BLACK-BOX SPRAWL

**Problem:** Three independent scoring systems, each with magic numbers and no
shared vocabulary.

| Location | Magic Numbers |
|----------|--------------|
| `snapshot.go` interestingWords | Binary: interesting/not (no scores) |
| `llmbundle.go` scoreFile | 100, 70, 65, 60, 50, 40, 30, 20, 10, ≤10 excluded |
| `flowexplain.go` scoreFileLayered | 200, 100, 70, 60, 50, 45, 40, 30, 25, 20, -10, -20, -30, -100 |
| `flowexplain.go` aliasExpansions | etcd-specific: put→kv/txn, lease→lessor, raft→rafthttp/wal |

For a mid dev adding support for a non-etcd repo (say, a Kubernetes controller), the
etcd-specific terms (`v3rpc`, `mvcc`, `wal`, `lease`, `lessor`, `rafthttp`, 
`etcdserverpb`, `cobra`) produce garbage scores. The mid dev has to grep for every
occurrence and either fork or generalize.

**What to do:**

### Step 1: Extract magic numbers into named constants

```go
// internal/scoring/scores.go
const (
    ScoreSeedFile       = 200
    ScoreEntrypoint     = 100
    ScoreExactBasename  = 100
    ScoreDirSegment     = 60
    ScorePathContains   = 50
    ScoreBasenameMatch  = 70
    ScoreAPIType        = 70
    ScoreDomainKeyword  = 30
    ScoreProtoFile      = 20
    ScoreSourceFile     = 30
    ScoreGenerated      = 10
    ScoreConfigFile     = 20

    PenaltyUnknown = -100  // e.g., v2store — legacy code
    PenaltyUtility  = -30  // e.g., pathutil — boring utility
    PenaltyDiff     = -20  // e.g., patch/diff files
)
```

### Step 2: Extract domain knowledge into a pluggable registry

```go
// internal/scoring/domain.go
type DomainProfile struct {
    Name string
    // Keywords that boost file score when found in path
    BoostKeywords map[string]int   // "v3rpc" → 40
    // Alias expansions for term search
    TermAliases map[string][]string // "lease" → ["lessor", "keepalive", ...]
    // Classification patterns for entrypoints
    EntrypointClassifiers []EntrypointRule
}

var DefaultProfile = &DomainProfile{
    BoostKeywords: map[string]int{
        "server":  25,
        "handler": 25,
        "grpc":    30,
    },
    // Minimal defaults — no etcd-specific terms
}
```

Then `flowexplain` and `llmbundle` accept a `*DomainProfile` parameter. The etcd
profile lives in a test or config file, not in the core scoring logic.

### Step 3: Merge llmbundle scoring and snapshot interestingFiles

They serve the same purpose: "which files matter?" The snapshot's binary
interesting/not check should use the same scoring model as llmbundle, just with
a lower threshold.

**Priority: HIGH-MEDIUM.** Current code works for its primary target (etcd) but a
mid dev tasked with "make this work for $OTHER_REPO" will face a wall of
hardcoded etcd assumptions.

---

## 5. NO SHARED DOMAIN TYPES — EACH PACKAGE INVENTS ITS OWN

**Problem:** The same conceptual type is defined in 2-3 different packages.

```
CandidateFlow (conceptual type):
  flowexplain.CandidateFlow    ← line 49 of flowexplain.go
  orient.orientResponse        ← line 249, has CandidateFlows []flowexplain.CandidateFlow
  These are the SAME TYPE but orientResponse wraps it

scoredFile (conceptual type):
  llmbundle.fileIndexEntry     ← different field names (Signals, Reasons)
  flowexplain.scoredFile       ← different field names (Score, Reasons, MatchedTerms)
  These are SIMILAR but incompatible

Edge (conceptual type):
  gofacts.Edge                 ← {From, To} — just paths
  flowexplain.flowEdge         ← {From, To, Reason} — paths + reason
  Nearly identical
```

**What to do:**

Create `internal/report/types.go` (or `internal/domain/types.go`) with the canonical
definitions:

```go
package domain  // or package report

type CandidateFlow struct {
    Name             string   `json:"name"`
    Trigger          string   `json:"trigger"`
    LikelyEntrypoint string   `json:"likely_entrypoint"`
    LikelyFiles      []string `json:"likely_files"`
    WhyInteresting   string   `json:"why_interesting"`
    Evidence         []string `json:"evidence"`
    Confidence       float64  `json:"confidence"`
}

type ScoredFile struct {
    Path         string   `json:"path"`
    Kind         string   `json:"kind"`
    Score        int      `json:"score"`
    Reasons      []string `json:"reasons"`
    MatchedTerms []string `json:"matched_terms,omitempty"`
}

// Edge with optional reason — backward compatible with gofacts.Edge
type Edge struct {
    From   string `json:"from"`
    To     string `json:"to"`
    Reason string `json:"reason,omitempty"`
}
```

All packages import and use these. A mid dev adding a new field to `ScoredFile`
updates one place and gets consistent JSON serialization everywhere.

**Priority: MEDIUM.** Not blocking but reduces "why are there two of these?"
confusion that every new contributor hits.

---

## 6. ERROR WRAPPING IS INCONSISTENT

**Problem:** Some errors use `%w` (preserving the chain), some use `%v` (breaking it),
some construct entirely new errors discarding the cause.

```go
// GOOD — wraps with %w
return fmt.Errorf("write stdout: %w", err)              // main.go:121
return fmt.Errorf("resolve path: %w", err)              // main.go:193

// BAD — discards cause
return fmt.Errorf("invalid orientation JSON: %s", ...)  // orient.go:195
return Snapshot{}, err                                   // snapshot.go:127 (bare, no context)

// BAD — constructs new error  
fmt.Fprintf(os.Stderr, "Report: %s/%s/report.html\n", ...) // orient.go:241 (uses fmt.Fprintf, not error return)

// SUSPICIOUS — interface{} parameter
func explainOneFlow(ctx context.Context, client *deepseek.Client, 
    cf flowexplain.CandidateFlow, trackedFiles []string, 
    facts interface{}, // ← WAT? This is *gofacts.Facts, just type it
    maxFiles int, dw *debugdump.Writer, opts Options, 
    callDeepSeek bool) explainedFlow
```

**What to do:**

1. Always use `fmt.Errorf("doing X: %w", err)` when wrapping.
2. Replace the `facts interface{}` parameter with `facts *gofacts.Facts`.
3. Define sentinel errors for known failure modes:

```go
// internal/orient/errors.go
var (
    ErrNoAPIKey     = fmt.Errorf("DEEPSEEK_API_KEY is not set")
    ErrInvalidJSON  = fmt.Errorf("DeepSeek returned invalid JSON")
    ErrNoGitRepo    = fmt.Errorf("not a git repository")
)
```

So callers can `errors.Is(err, ErrNoAPIKey)` instead of string-matching.

**Priority: MEDIUM-LOW.** Doesn't block features but makes debugging harder. The
`interface{}` parameter is a code smell worth fixing immediately.

---

## 7. PATH HANDLING IS SCATTERED AND INCONSISTENT

**Problem:** Path operations (`filepath.Clean`, `filepath.Rel`, repo-relative
resolution, symlink normalization) happen in at least 5 places with different
assumptions.

- `snapshot.go:130`: `filepath.Base(filepath.Clean(opts.RepoPath))` for repo name
- `gofacts.go`: `normalizePackagePaths()` — custom symlink resolution (~50 lines)
- `flowexplain.go:197-203`: `filepath.Clean` for seed validation
- `flowexplain.go:239`: `filepath.Clean` for tracked files
- `llmbundle.go`: `filepath.Rel` for open_files computation

A mid dev wondering "is this path repo-relative or absolute?" has no single source
of truth.

**What to do:**

```go
// internal/paths/paths.go
type Resolver struct {
    RepoRoot string  // absolute
}

func (r *Resolver) Rel(abs string) string { ... }
func (r *Resolver) Abs(rel string) string { ... }
func (r *Resolver) Clean(rel string) string { ... }
func (r *Resolver) RepoName() string { return filepath.Base(r.RepoRoot) }
```

Inject a `*paths.Resolver` into snapshot, gofacts, and flowexplain. All path
computation goes through one type with clear semantics.

Also: `gofacts.normalizePackagePaths()` silently drops packages outside the repo
with no warning. This should at least emit a warning to `Facts.Warnings`.

**Priority: MEDIUM.** Not urgent but prevents a class of "path doesn't exist"
bugs that are hard to debug across package boundaries.

---

## 8. CONTEXT PROPAGATION IS BROKEN

**Problem:** `buildFlowBundlesFromSnapshot()` creates `context.Background()` internally
(orient.go:275) instead of accepting and propagating the caller's context.

```go
func buildFlowBundlesFromSnapshot(s snapshot.Snapshot, n int, 
    dw *debugdump.Writer) []explainedFlow {
    // ...
    ef := explainOneFlow(context.Background(), nil, cf, ...)
    //                     ^^^^^^^^^^^^^^^^^^
    // Cancelling the parent context does NOT cancel this
}
```

This means:
- SIGTERM doesn't stop flow bundle construction
- Timeouts don't propagate to `go list` calls inside flowexplain
- A mid dev adding a `--timeout` flag will be confused why it doesn't work for
  offline mode

**What to do:** Thread the context through.

```go
func buildFlowBundlesFromSnapshot(ctx context.Context, s snapshot.Snapshot, 
    n int, dw *debugdump.Writer) []explainedFlow {
    for _, oc := range s.GoFacts.OrientationCandidates {
        select {
        case <-ctx.Done():
            return flows  // stop early on cancellation
        default:
        }
        ef := explainOneFlow(ctx, nil, cf, ...)
        // ...
    }
}
```

**Priority: MEDIUM.** Small change, big correctness improvement.

---

## 9. INTERNAL STATE LEAKS THROUGH PUBLIC TYPES

**Problem:** `snapshot.Snapshot` has a `FilteredFiles []string` field tagged `json:"-"`.
This is the tracked file list used as internal state by `orient.Run()` and
`flowexplain.SelectFlowFiles()`. It's passed around as raw data but semantically
it's pipeline glue, not snapshot data.

```go
type Snapshot struct {
    // ... public fields ...
    FilteredFiles []string `json:"-"`  // ← internal state disguised as data
}
```

**What to do:** Either:
- Pass it separately: `orient.Run()` builds a `[]string` from the snapshot and
  passes it downstream explicitly.
- Or make it a first-class field with a real name: `TrackedSourceFiles []string`.

The `json:"-"` tag signals "this doesn't belong here." Listen to it.

**Priority: LOW.** Not breaking, but makes the data flow harder to understand.
Fix when touching snapshot anyway.

---

## 10. REPORT PACKAGE NEEDS TESTS AND REFACTORING

**Problem:** `internal/report/report.go` has:
- No test file (confirmed: `? github.com/dvordrova/repomap/internal/report [no test files]`)
- Inline HTML/CSS/JS generation via `fmt.Sprintf` — unreadable, untestable
- Duplicated flow report parsing (lines 130-156 are copy-paste of earlier block)

**What to do:**

```go
// The HTML template should be a separate file, embedded at compile time.
//go:embed templates/report.html
var reportTemplate string

// Or at minimum, extract the HTML building into:
func (rd *RunData) ToHTML() string { ... }  // testable!
```

Then: `report_test.go` with a golden-file test. Build a known `RunData`, render it,
compare against `testdata/report.golden.html`.

**Priority: MEDIUM.** Any change to the HTML report is currently untestable and
risky. A mid dev tasked with "make the report look better" has no safety net.

---

## 11. DEEPSEEK CLIENT COUPLING TO ENV VARS

**Problem:** `deepseek.NewFromEnv()` reads environment variables directly.
This is a violation of dependency injection — any code that uses the client
cannot control its configuration without modifying environment state.

```go
func NewFromEnv() (*Client, error) {
    apiKey := os.Getenv("DEEPSEEK_API_KEY")  // ← global mutable state
    // ...
}
```

**What to do:** Add a constructor that accepts parameters:

```go
func NewClient(opts ClientOptions) *Client { ... }

type ClientOptions struct {
    APIKey    string
    Model     string
    MaxTokens int
    Endpoint  string
}

func ClientOptionsFromEnv() ClientOptions { ... }  // convenience
```

`orient.Run()` receives an already-constructed client. The `client_test.go` already
provides a test client constructor — extract that pattern.

**Priority: MEDIUM.** Clean separation of configuration from construction.

---

## 12. GLOBAL VARS PREVENT CUSTOMIZATION

**Problem:** Static configuration lives in package-level `var` blocks:

```go
// snapshot.go:55-63
var skipDirPrefixes = []string{".git/", ".github/", "vendor/", ...}

// snapshot.go:65-98
var skipFileExt = map[string]struct{}{".png": {}, ".jpg": {}, ...}

// snapshot.go:100-106
var interestingWords = []string{"server", "handler", "grpc", ...}

// flowexplain.go:59-87
var ignoredTerms = map[string]bool{"flow": true, "path": true, ...}

// flowexplain.go:89-100
var aliasExpansions = map[string][]string{...}
```

A user who wants to analyze a Python repo (2030 wish) or exclude custom paths
(`generated/`, `mocks/`, `fixtures/`) cannot do so without forking the code.

**What to do:** Move into Config or accept as parameters:

```go
// internal/snapshot/snapshot.go
type Options struct {
    // ...
    SkipDirPrefixes  []string  // if nil, use defaults
    SkipExtensions   map[string]struct{}
    InterestingWords []string
}
```

The `DefaultOptions()` function returns the current hardcoded lists. Tests and
custom deployments can override.

**Priority: MEDIUM-LOW.** Doesn't matter until someone tries to use repomap on
a non-standard repo. But the fix is trivial — just parameterize what's already
a `var` block.

---

## 13. FLOWEXPLAIN HAS UNEXPORTED INTERNAL TYPES

**Problem:** `flowexplain.go` has `fileCandidate` (unexported) defined at line 221
and a separate `candidate` struct defined at line 227 — both inside `SelectFlowFiles()`.
The latter shadows the former but has different fields (no `isSeed`). This is a 
bug waiting to happen.

```go
// line 212 — unexported type at package level
type fileCandidate struct { ... }

// line 227 — ANOTHER candidate struct defined inside SelectFlowFiles
type candidate struct { ... }  // ← different fields! 
```

**What to do:** Use the package-level `fileCandidate` consistently. Remove the
inline `candidate` definition.

**Priority: LOW.** Doesn't cause a bug today (they're used in different scopes)
but confused me for 5 minutes. A mid dev refactoring this function will be confused.

---

## 14. REPORT RENDERING BLOCKS THE PIPELINE

**Problem:** `Run()` calls `writeRunReport()` synchronously (line 222, 229), which
reads files from disk, parses JSON, and writes HTML — all blocking the response
to the user.

```go
// orient.go:222
writeRunReport(dw, runDir(dw), opts.DebugDir, runID)
// orient.go:236
func writeRunReport(...) {
    rd, err := report.ReadRunDir(runDirAbs)  // ← reads all bundles from disk
    report.WriteReportJSON(rd, ...)          // ← writes JSON
    report.WriteReportHTML(rd, ...)          // ← writes HTML
    fmt.Fprintf(os.Stderr, "Report: %s/%s/report.html\n", ...)
}
```

For a future feature like `--serve` (live HTTP server showing reports), this is
a problem — the pipeline should produce the report, and rendering should be a
separate step.

**What to do:** Move the `writeRunReport` call out of `Run()` and into `cmd/main.go`
after the pipeline completes. The pipeline returns data; the CLI layer decides
how to render it.

**Priority: LOW.** Fine for CLI usage but blocks server/integration patterns.

---

## 15. THE `smoke-bundle` CI TARGET IS UNDEFINED

**Problem:** `.github/workflows/ci.yml` runs `make smoke-bundle`, but the Makefile
has no such target.

```makefile
# Makefile has:
#   check, test, vet, build, clean, smoke, etcd-check, run-*, debug-last, deepseek-check
# NO smoke-bundle target
```

**What to do:** Add it or remove it from CI. A mid dev changing the Makefile
will wonder what this target does and whether their changes break it.

**Priority: P0.** CI is silently failing on this step (make returns non-zero for
undefined targets).

---

## PRIORITY SUMMARY

| # | Item | Effort | Impact |
|---|------|--------|--------|
| 15 | Fix `smoke-bundle` CI target | 1 min | CI broken |
| 1 | Extract interfaces (LLM, Scanner) | 2-4 hours | Foundation for all future work |
| 2 | Split `orient.go` god package | 2-3 hours | Maintainability cliff |
| 4 | Extract scoring constants + domain profiles | 2-3 hours | Correctness for non-etcd repos |
| 3 | Single Config struct | 1 hour | Every new flag costs less |
| 5 | Shared domain types | 1 hour | Remove type duplication |
| 8 | Context propagation | 30 min | Correct cancellation |
| 11 | DI for deepseek client | 30 min | Testability |
| 10 | Test report renderer | 2 hours | Safety net for HTML changes |
| 6 | Consistent error wrapping | 1 hour | Debuggability |
| 7 | Path resolver abstraction | 1-2 hours | Fewer path bugs |
| 12 | Parameterize global vars | 30 min | Customizability |
| 14 | Decouple report rendering | 1 hour | Async/server readiness |
| 13 | Fix shadowed types in flowexplain | 5 min | Code clarity |
| 9 | Remove `json:"-"` internal state | 15 min | Clean data model |

---

## WHAT A MID DEV SHOULD NOT CHANGE

Some things are correct by design and should stay:

1. **Zero external dependencies.** The `go.mod` with only `go 1.22` is intentional.
   Resist the urge to add gorilla/mux, testify, logrus, or any framework. Standard
   library is a feature.

2. **`git ls-files` as the source of truth.** Don't replace with `filepath.Walk`
   unless adding a non-git fallback. Git's tracked file list is exactly correct.

3. **`go list -json` output format.** Don't add `gopls` or AST parsing. The
   design doc says so explicitly. If more Go facts are needed, `go list` has
   flags for them (`-deps`, `-find`, `-json` with `Deps`, `Imports`, `TestImports`).

4. **The security invariant.** DeepSeek never receives raw file contents, full
   `file_tree`, or full `internal_edges`. Any feature that sends more data to the
   LLM must be bounded by explicit counters in Config.

5. **Debug artifact redaction.** `debugdump.redactJSON()` must run on every
   artifact that could contain secrets. If you add a new artifact type, add
   redaction logic.
