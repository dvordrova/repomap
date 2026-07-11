# AGENTS.md

## What repomap is

`repomap` is a tiny local-first repository orientation CLI for large unfamiliar codebases.
It produces a compact local snapshot of a git repository and optionally asks DeepSeek for
a structured orientation report.

Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) for the project vision and pipeline design.

## Core pipeline

1. **Local deterministic snapshot** — `git ls-files`, README, file tree, language hints
2. **Go facts** — `go list -json ./...` per discovered module, packages/edges/entrypoints
3. **Compact LLM bundle** — bounded subset (module summaries, entrypoints with open_files, important edges)
4. **Optional DeepSeek orientation** — sends only the compact bundle, not full repo contents
5. **Debug artifacts** — written under `--debug-dir` when requested

## Design rules

- DeepSeek must **never** receive full repo contents, raw `file_tree`, or raw `internal_edges`.
- Do **not** add LSP, gopls, AST parsing, embeddings, diagrams, or third-party dependencies unless explicitly requested.
- DeepSeek must only interpret a compact bounded facts bundle produced by local deterministic extraction.

## Guiding documents

- Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) before changing architecture.
- Read [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) before changing DeepSeek client.
- Do not invent ad-hoc DeepSeek request shapes; follow docs/DEEPSEEK_API_NOTES.md.
- If a debugging command is useful, add or update a script instead of leaving a one-off shell pipeline.

## Development rules

- Before finishing any code change, run:
  ```
  ./scripts/check.sh
  ```
- If the repo is Go and etcd is available nearby, also run:
  ```
  ./scripts/etcd_check.sh ../etcd
  ```
- If a command fails, **fix the failure** or clearly explain why it cannot be fixed.
- Never leave known broken tests.
- Never use one-off shell pipelines when a reusable script would be better.
- If a debugging command is useful twice, **turn it into a script** under `./scripts/`.
- Debug artifacts must **never** include API keys or Authorization headers.
- Debug artifacts must **never** be committed.

## Reusable scripts

```
./scripts/check.sh          # go test + go vet
./scripts/smoke.sh           # temp git repo, snapshot+bundle, no network/API key
./scripts/etcd_check.sh      # validate against etcd clone (skip if absent)
./scripts/deepseek_check.sh  # full DeepSeek call (skip without key)
./scripts/source_artifacts_check.sh # bounded source card/bundle, no model call
./scripts/source_check.sh    # replay fixed DeepSeek source response, no network
./scripts/source_prompt_experiment.sh # live source-stage DeepSeek experiment
./scripts/investigation_check.sh # replay M2 reducer path (local or DeepSeek)
./scripts/quality_check.sh   # replay committed quality task, no network/API key
./scripts/debug_last_run.sh  # inspect last debug run
./scripts/clean.sh           # remove tmp/.repomap-runs/.bin
```

## Test invariants

- `--llm-bundle-only` must not require `DEEPSEEK_API_KEY`
- LLM bundle must not include full `file_tree`
- LLM bundle must include `open_files` for entrypoints
- Debug dumps must redact sensitive keys (api_key, token, authorization, password)
- Debug dumps must never contain Authorization headers
- Invalid DeepSeek JSON must return a clear error
- Non-2xx DeepSeek responses must include status and response body in the error
- Committed quality tasks must replay without an API key or network call
- Quality artifacts must be manifest-relative, bounded, and verified by SHA-256
