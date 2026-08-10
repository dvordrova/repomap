# AGENTS.md

## What repomap is

`repomap` is a local-first, evidence-backed repository orientation tool. It produces an
architecture landscape, discovered runtime surfaces, model-assisted conceptual grouping,
and bounded saved flow traces. DeepSeek is the reference provider and compatibility default.

Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) for the project vision and pipeline design.

## Core pipeline

1. **Local deterministic snapshot** — tracked files, documentation and language hints
2. **Repository facts and Atlas** — language adapters produce canonical local entities and relations
3. **Bounded semantic projections** — Architecture and Study interpret compact request-local facts
4. **Browser/debug artifacts** — the authoritative Atlas, semantic state and report under `--debug-dir`

## Design rules

- A model provider must **never** receive full repo contents, raw `file_tree`, or raw `internal_edges`.
- Do not add a new analysis or presentation layer unless it is explicitly requested by the
  repository owner or included in the currently approved decision.
- A model must only interpret a compact bounded facts bundle produced by local deterministic extraction.

## Guiding documents

- Read [docs/CORE_IDEA.md](docs/CORE_IDEA.md) before changing architecture.
- Read [docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md) and its referenced
  decision before implementing feature-specific scope.
- Read [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) before changing DeepSeek client.
- Do not invent ad-hoc DeepSeek request shapes; follow docs/DEEPSEEK_API_NOTES.md.
- Do not add shell-script entrypoints. Keep `Makefile` as a small router to Go
  commands, Go tests and the built `repomap` binary. Put substantive reusable
  logic in Go; keep one-off experiments outside the production workflow.

## Decision workflow

- Durable repository rules live in `AGENTS.md`.
- Feature-specific approved scope lives in numbered decision documents under
  `docs/agent-room/`.
- `docs/agent-room/CURRENT.md` points to the active implementation decision.
- Historical decisions are preserved and are not silently rewritten or invalidated.
- Explicit repository-owner approval is required to change the active decision.
- Implementation agents must not silently expand scope or select the numerically latest
  decision.

## Development rules

- Build the working binary with `make build`; this is the canonical owner-facing
  build entrypoint and writes the exact trimpath binary to `.bin/repomap`.
  Use a direct `go build -trimpath -o PATH ./cmd/repomap` only when an explicitly
  separate temporary candidate path is required.
- Exercise the built binary directly on a real repository. For a provider-free
  gate use `PATH REPO --offline --no-open --no-serve --debug-dir DIR`.
- Verify the process exit status and the generated manifest, Atlas, report JSON
  and report HTML. A wrapper reporting success is not product acceptance.
- Run focused Go tests for changed contracts and `go vet` for changed packages.
- If a command fails, **fix the failure** or clearly explain why it cannot be fixed.
- Never leave known broken tests.
- If a debugging operation is useful twice, implement it as a Go test, Go
  command or a short Make target that invokes one of those entrypoints.
- Debug artifacts must **never** include API keys or Authorization headers.
- Debug artifacts must **never** be committed.

## Test invariants

- Offline runs must not require provider credentials or make provider calls.
- Provider requests must not include the full file tree, raw internal edges,
  canonical Atlas IDs or unadvertised repository paths.
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
