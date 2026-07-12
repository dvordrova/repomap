# AGENTS.md

## What repomap is

`repomap` is a tiny local-first repository orientation CLI for large unfamiliar codebases.
It produces a compact local snapshot of a git repository and optionally asks an explicitly
configured OpenAI-compatible model for a structured orientation report. DeepSeek is the
reference provider and compatibility default. The active product outcome is a five-minute
friend onboarding trial on a known Go project, not a broader agent framework.

Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) for the project vision and pipeline design.

## Core pipeline

1. **Local deterministic snapshot** — `git ls-files`, README, file tree, language hints
2. **Go facts** — `go list -json ./...` per discovered module, packages/edges/entrypoints
3. **Compact LLM bundle** — bounded subset (module summaries, entrypoints with open_files, important edges)
4. **Optional model orientation** — sends only the compact bundle, not full repo contents
5. **Local direction bundles** — bounded file/test/package/import neighborhoods, no extra model call
6. **Browser/debug artifacts** — full onboarding view plus inspectable saved facts under `--debug-dir`

## Design rules

- A model provider must **never** receive full repo contents, raw `file_tree`, or raw `internal_edges`.
- The existing gopls adapter is confined to exact-symbol investigation. Do **not** move it
  into the default repository survey or add another LSP/AST/embedding/diagram/dependency
  layer unless explicitly requested.
- A model must only interpret a compact bounded facts bundle produced by local deterministic extraction.

## Guiding documents

- Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) before changing architecture.
- Read [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) before changing DeepSeek client.
- Do not invent ad-hoc DeepSeek request shapes; follow docs/DEEPSEEK_API_NOTES.md.
- Reusable developer entrypoints belong in `Makefile`. Keep a script only when
  it contains a substantive multi-step check or is also a standalone CI entrypoint;
  expose that script through a Make target.

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
- Never leave a useful one-off shell pipeline undocumented.
- If a debugging command is useful twice, **turn it into a Make target**. Extract
  a script under `./scripts/` only when the recipe is too substantial for Make.
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
./scripts/investigation_handoff_check.sh # copied split-memory restart, no model call
./scripts/friend_check.sh     # build handoff binary + one-request browser journey fixture
./scripts/friend_artifact_check.sh # validate one generated friend-trial run
./scripts/quality_check.sh   # replay committed quality task, no network/API key
./scripts/quality_preflight.sh # verify a linked orientation/symbol target, no network
./scripts/debug_last_run.sh  # inspect last debug run
./scripts/clean.sh           # remove tmp/.repomap-runs/.bin
```

## Test invariants

- `--llm-bundle-only` must not require `DEEPSEEK_API_KEY`
- LLM bundle must not include full `file_tree`
- LLM bundle must include `open_files` for entrypoints
- Every model-visible repository file path must occur in `allowed_paths`
- Debug dumps must redact sensitive keys (api_key, token, authorization, password)
- Debug dumps must never contain Authorization headers
- Invalid DeepSeek JSON must return a clear error
- Non-2xx DeepSeek responses must include status and response body in the error
- Committed quality tasks must replay without an API key or network call
- Quality artifacts must be manifest-relative, bounded, and verified by SHA-256
- Quality manifests and artifacts must reject obvious credentials without echoing them
- Saved investigation facts, claims, and session state must remain separate and hash-verified
- Current repository/fact/claim context must be reconciled before a saved action is executable
- Repository freshness must hash dirty contents without reading unrelated ignored secrets
- Interactive report actions require a versioned run manifest bound to the exact report and repository state
- Browser clients may request local analysis only through manifest-authorized opaque IDs, never raw paths or symbols
