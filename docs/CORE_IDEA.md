# repomap core idea

repomap helps understand unfamiliar local Go repositories by extracting
deterministic local facts and optionally asking an OpenAI-compatible model to
interpret them as structured orientation reports. DeepSeek remains the reference
provider and calibration target; company-hosted compatible endpoints use the
same bounded request contract.

Product and research decisions that intentionally remain unresolved are tracked
in [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md). The concrete client package is still
named `deepseek`, but endpoint/model/auth/timeout configuration is provider-neutral.

The proposed shared investigation workflow is documented in
[INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md). Demonstrated implementation
gaps and experiment follow-ups are tracked in
[TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).

For a current/planned module map and independently runnable challenge points,
read [SYSTEM_MAP.md](SYSTEM_MAP.md).

The ordered product outcomes and their observable completion conditions are in
[MILESTONES.md](MILESTONES.md). Exactly one milestone is active at a time.

## Pipeline

### 1. Deterministic local extraction

Run without a model or API key. Go package discovery deliberately respects the
engineer's normal Go environment and may use a configured company proxy:

- `git ls-files` for tracked file inventory
- README (truncated)
- top-level directory stats
- language hints by file extension
- interesting files (entrypoint-ish, storage-ish, messaging-ish, config-ish, background-ish)
- `go.mod` files per discovered module
- `go list -json ./...` per module
- Go package import edges (internal, external top-50)
- entrypoint detection (Name == "main")
- module summaries (role guess, top internal imports, top external imports)
- orientation candidates (ranked entrypoints with repo-relative `open_files`)
- known docs (Documentation/, docs/, architecture .md files)

### 2. Compact LLM bundle

Derived from deterministic facts, bounded by limits:

```json
{
  "repo_name": "...",
  "readme_excerpt": "...",
  "top_level_directory_stats": {...},
  "language_hints": [...],
  "go": {
    "modules_count": 0,
    "packages_count": 0,
    "module_summaries": [...],
    "entrypoints": [...],
    "orientation_candidates": [...],
    "important_edges": [...]
  },
  "known_docs": [...],
  "warnings": [...]
}
```

Must NOT include:
- full file_tree
- full repository contents
- secrets, env files, private keys
- full README
- raw internal_edges beyond limits

### 3. Model orientation

The configured provider receives only the compact facts bundle and returns a
JSON orientation report. Structured verified paths are normalized against the
bundle allowlist, and path-like mentions inside evidence prose cannot name an
unprovided file. If a provider abbreviates or invents a path inside free-form
evidence, that evidence item is removed with a parser warning instead of losing
the whole report. An ungrounded `likely_entrypoint` may fall back to the flow's
first already-allowed `likely_file`; structured file lists remain fail-closed.
Other prose remains an explicit model interpretation.

The report proposes **candidate runtime/event flows**, not folder summaries.
Every candidate flow must cite evidence from the bundle.
Confidence must be explicit, warnings for low confidence.

### 4. Focused flow analysis (implemented, opt-in)

- the default browser run saves a bounded deterministic file/test/package/import
  neighborhood for every validated direction, with no additional provider call;
- selecting a direction renders that saved local evidence;
- `--flows N` expands only the top N candidate directions;
- repomap gathers focused files/tests/docs/source signals for each selected flow;
- the provider explains only the focused bundle;
- known fields are normalized, verified paths are allowlisted, and unsafe or
  incomplete reports are rejected locally.

Named user choice now reaches the first saved local neighborhood. Exact-symbol
candidate selection and the resumable orientation-to-investigation handoff are
not yet wired into the main CLI; they remain the important integration boundary
for the progressive product journey.

### 5. Durable focused investigation (implemented, isolated from the browser)

The exact-symbol investigation stores deterministic symbol/source/test facts,
model-derived source claims, and reducer/session state separately. Repository
identity, HEAD and dirty contents, Go/gopls/collector/build inputs, and
prompt/parser/evaluator versions are reconciled before a saved action becomes
executable. Unchanged sessions resume without a second model call; changed
facts re-resolve the symbol, while changed claim logic retains local source and
returns to assessment. M5 next connects a user-selected browser direction's
verified files to exact symbol candidates and this already-durable path rather
than adding another persistence mechanism.

## Experimental local evidence layer

The production pipeline above remains unchanged. New analyzers are developed
behind a language-neutral evidence graph before they are connected to it:

- every relation has a `certainty` (`possible`, `static`, `observed`, ...)
- every relation cites `provenance` (provider, version, operation, location)
- build/runtime conditions are explicit `scenarios`
- language-specific adapters implement the same `analyzer.Provider` port and
  emit the same graph

The first adapter is an isolated Go/gopls playground. Fuzzy workspace-symbol
matches are `possible`; direct call-hierarchy edges are `static`. Static means
"supported by analysis under this build configuration", not "observed at
runtime". The playground writes JSON for machines and Markdown for human
inspection:

```sh
go run ./cmd/gopls-playground \
  --repo ../etcd \
  --query kvServer.Put \
  --out tmp/evidence-examples/etcd.json \
  --summary-out tmp/evidence-examples/etcd.md
```

Run `./scripts/gopls_examples.sh --fetch` to produce the same artifacts for
etcd, k6, Prometheus, NATS Server, and golangci-lint. Fetched clones and
generated artifacts stay under `tmp/` and are not part of the LLM bundle.

The first focused vertical slice resolves one exact symbol, expands only its
direct static callers/callees, and produces a bounded DeepSeek request:

```bash
./scripts/symbol_check.sh ../etcd kvServer.Put
```

The raw local evidence graph is retained for debugging but is never sent.
DeepSeek receives only `symbol_bundle.json`; every report claim must cite its
evidence IDs, and the response is rejected if it invents paths, evidence,
caller/callee identities, observed runtime behavior, or test files.

The completed source-grounded slice is explicit and independently replayable:

```bash
./scripts/source_artifacts_check.sh ../etcd kvServer.Put  # local source bundle
./scripts/source_check.sh                                 # fixed DeepSeek replay
./scripts/source_prompt_experiment.sh LABEL ../etcd kvServer.Put  # live API
```

Source lines remain bounded lexical evidence, never a claim that the whole
function body was parsed. For validation-shaped calls, the local source cube can
lexically connect an assigned multiline call to an immediately following
`if err != nil` or returned `err == nil` comparison and exposes only the minimal
supporting source IDs. This uses Go token scanning, not AST semantic inference,
and still leaves callee behavior and runtime reachability unknown. A weak model
that cites only the call anchor is reduced to `ambiguous` with a warning rather
than being allowed to manufacture or inherit the missing proof. Related
`_test.go` locations are `test_reference` navigation evidence with gopls
provenance; they are not `test_supported` until their bounded test source is
assessed.

When a callee name offers no semantic hint, the same bounded scanner can seed
three syntax-only questions: an immediately checked call result, a direct return
of a call, or a standalone call under a locally visible `case`/`if`/`else`
branch. Those predicates deliberately say nothing about callee behavior. Their
locally reconstructed claims require the complete minimal proof (for example,
both the branch line and call anchor), and runtime branch selection remains an
explicit unknown.

## Non-goals for now

- no AST parsing yet
- no gopls in the default repository-wide survey; it remains a lazy focused
  investigation adapter
- no long-lived LSP client yet; the playground uses the experimental gopls CLI
- no embeddings yet
- no autonomous graph/diagram UI; the current browser report is static and M5
  adds only the first user-selected onboarding drill-down
- no automatic huge repo upload
- no autonomous code modification

These are present scope boundaries, not permanent answers to the questions in
[OPEN_QUESTIONS.md](OPEN_QUESTIONS.md).

## Good vs bad output

Good output (runtime/event-oriented flows):
- "client gRPC Put request"
- "etcd server startup"
- "watch stream"
- "lease lifecycle"
- "raft replication/write path"
- "etcdctl command execution"

Bad output (folder-oriented):
- "server module"
- "client folder"
- "pkg package"
