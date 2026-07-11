# Current handoff

Last updated: 2026-07-10.

## Product direction

repomap is a trustable, inspectable Go repository investigation CLI for an
engineer using an OpenAI-compatible company model. It is not another general
coding agent. It builds bounded local facts, offers runtime/event-oriented
directions, and then follows one explicit evidence branch through symbols,
source, tests, and unknowns.

The company-engineer acceptance track has three goals:

1. calibrate onboarding output on a project the engineer already knows;
2. explore a genuinely unfamiliar Go project progressively;
3. use a real pre-change commit and ticket to produce a bounded `ChangeBrief`.

These are policies over one investigation engine, not three prompt pipelines.
The canonical implementation order remains [MILESTONES.md](MILESTONES.md): M4
fresh local memory is complete, the friend onboarding trial is active in M5,
free repository exploration follows in M6, and feature work moves to M7. The
exact acceptance contract is in
[ENGINEER_TRIAL.md](ENGINEER_TRIAL.md).

If the user starts with “так, а че мы там дальше делаем?”, recommend:

1. give the current binary and five-line setup to the knowledgeable friend and
   have them run the first **Friend Onboarding Trial** on a Go project they know;
2. collect their `onboarding-feedback.md`: what was correct, missing,
   misleading, and which direction/file became useful first;
3. implement the next bounded cube, `SelectedFlow + verified files -> ranked
   exact symbol candidates`, without promoting model prose to analyzer truth;
4. keep the six M3 tasks and Qwen 1.5B structured regression green while wiring
   the selected exact symbol into the existing resumable investigation.

Do not restart Qwen prompt tuning. Keep the staged 1.5B regression runnable, but
the default product path uses the configured OpenAI-compatible provider.

## What works now

- deterministic tracked-file survey, bounded tracked README, Go facts, source
  signals, and compact LLM bundle;
- repository-confined reads through `os.OpenRoot`, including protection against
  README/source/go.mod symlink escape;
- `go list -e` using the engineer's normal Go environment, so internal proxies
  work and missing packages can still yield partial facts/warnings;
- atomic `REPOMAP_LLM_*` configuration with bearer or explicit no-auth, timeout,
  DeepSeek compatibility aliases, and no implicit repository `.env` loading;
- `repomap doctor llm [--check]`, where `--check` is exactly one small synthetic
  JSON request with no repository content and no retries;
- `--llm-bundle-only` and `--preview-request` inspection without an API key or
  model call;
- bounded orientation preserving the full known response shape, with structured
  path validation and credential gates on outbound/retained content;
- `./repomap` targets the current directory, reports bounded context/request
  bytes, makes one orientation call, preserves the complete onboarding report,
  and prepares a clickable bounded local evidence neighborhood for every
  validated direction without another provider call;
- safe run metadata (model, context/request bytes, latency, direction count)
  appears in the report, and every run creates a non-overwriting
  `onboarding-feedback.md`;
- opt-in top-N focused flow expansion; flow output is flexibly parsed,
  normalized to known fields, allowlist-validated, and rejected when empty or
  unsupported;
- debug artifacts in the OS user cache by default, including model, endpoint,
  and the attempted request even when the provider fails;
- exact-symbol evidence, bounded source cards, source assessment, related test
  references, and a pure resumable investigation reducer as isolated connected
  slices;
- versioned, hash-verified offline quality tasks for etcd, Grafana k6,
  Prometheus, NATS Server, and golangci-lint;
- a separate generic `REPOMAP_LLM_*` DeepSeek calibration task covering doctor,
  exact preview metadata, and the same offline quality dimensions;
- staged Qwen 1.5B protocol verification retained as non-critical regression
  tooling.
- stable repository/fact/claim freshness with separate content-addressed
  investigation facts, model claims, and session checkpoints;
- unchanged etcd investigations resume without a second model call; repository,
  tool/options, and prompt/evaluator changes invalidate only the applicable
  layer.

## Honest current boundary

- The browser can choose a named direction and inspect its saved deterministic
  file/test/package/import neighborhood, but cannot yet choose an exact symbol
  and hand it into the resumable reducer.
- `--flows 1` expands the highest-ranked direction, not a user-selected one.
- `--offline` means no model call; it is not a hard network sandbox for Go tools.
- Public `onboarding` and natural-language `feature` commands do not exist.
- Test references are navigation evidence until bounded test source/assertions
  are read.
- Static Go/gopls facts are build-scenario evidence, not runtime truth.
- `internal/deepseek` still owns OpenAI-compatible transport plus concrete
  prompts; runtime configuration is provider-neutral, but orientation lacks a
  consumer-owned model interface.
- M3 is complete. The generic calibration proves the configuration/transport
  path only; an actual company-hosted model quality run still needs that
  engineer's endpoint and remains an open research question.
- M4 is complete for the current exact-symbol investigation slice. Repository
  changes conservatively reset all focused facts rather than selectively
  retaining unrelated neighborhoods; optimize this only after friend-trial
  latency or repetition demonstrates a product problem.
- The next product blocker is `SelectedFlow + verified likely files -> ranked
  exact symbol candidates`. Search must run lazily after selection, filter gopls
  results to verified Go files before truncation, and require exact identity
  confirmation before entering the existing investigation reducer. The model's
  `likely_entrypoint` must never be promoted directly to a symbol.
- Prometheus is captured at revision
  `af77de9a5fd8b5391eb65ad770a454c9e84346c2`: the raw
  `deepseek-v4-flash` orientation proposes a TSDB write direction containing
  `model/labels/labels_common.go`, and the clean source response grounds how
  `Labels.IsValid` uses the result of `Validate`. This is a model-proposed
  cross-stage link, not runtime proof of ingestion.
- NATS Server is captured at revision
  `1be499156d9bc757ea08bd148608b622e38b7514`: the clean orientation links a
  client-message direction to `server/client.go`, and the clean source response
  grounds four `case`-selected calls in `client.processInboundMsg`. The static
  branch and related test reference do not prove runtime selection or coverage.
- golangci-lint is captured at revision
  `9b5e24cba6e9964465bc892ab9377fae5a60cb97`: the clean orientation links its
  CLI lint-run direction to `pkg/commands/run.go`, and the clean source response
  grounds three checked results plus one direct returned call in
  `runCommand.runAnalysis`. Callee behavior and test coverage remain unknown.

## Read in this order

1. [ENGINEER_TRIAL.md](ENGINEER_TRIAL.md) — external acceptance contract;
2. [MILESTONES.md](MILESTONES.md) — canonical order and completion gates;
3. [CORE_IDEA.md](CORE_IDEA.md) — current bounded pipeline;
4. [SYSTEM_MAP.md](SYSTEM_MAP.md) — modules and independently challengeable seams;
5. [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md) — shared workflow/reducer;
6. [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md) — demonstrated gaps and local-model measurements;
7. [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) — unresolved product decisions;
8. [DEEPSEEK_API_NOTES.md](DEEPSEEK_API_NOTES.md) — current transport/prompt contract.

## Verification

```bash
./scripts/check.sh
./scripts/etcd_check.sh ../etcd
./scripts/symbol_check.sh ../etcd kvServer.Put
./scripts/source_artifacts_check.sh ../etcd kvServer.Put
./scripts/source_check.sh
./scripts/investigation_check.sh ../etcd kvServer.Put
./scripts/investigation_handoff_check.sh ../etcd kvServer.Put
./scripts/quality_check.sh
```

The detailed Ollama 0.5B/1.5B/3B timings, failures, and staged-protocol results
are preserved in [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md). The useful conclusion is
small: 1.5B can select constrained evidence/actions, but source-grounded
behavioral claims still require the same source/test/runtime cubes.

## Workspace caution

Pre-existing uncommitted rewrites under `docs/agent-room/` and untracked
`opencode.json` belong to the user. Do not stage, restore, or overwrite them.
Generated artifacts remain under ignored local/cache directories and must not be
committed.
