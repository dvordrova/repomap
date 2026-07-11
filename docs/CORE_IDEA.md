# repomap core idea

repomap helps understand unfamiliar local Go repositories by extracting
deterministic local facts and optionally asking DeepSeek to interpret them
as structured orientation reports.

## Pipeline

### 1. Deterministic local extraction

Run entirely locally, no network, no API keys:

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

### 3. DeepSeek orientation

DeepSeek receives only the compact facts bundle.
DeepSeek returns a JSON orientation report.

The report proposes **candidate runtime/event flows**, not folder summaries.
Every candidate flow must cite evidence from the bundle.
Confidence must be explicit, warnings for low confidence.

### 4. Later flow analysis (planned, not implemented)

- user chooses one candidate flow
- repomap gathers focused files/tests/docs for that flow
- DeepSeek explains only that flow

## Non-goals for now

- no AST parsing yet
- no LSP/gopls yet
- no embeddings yet
- no diagrams/UI yet
- no automatic huge repo upload
- no autonomous code modification

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
