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
The canonical implementation order remains [MILESTONES.md](MILESTONES.md): M3
quality is active, feature work is M6, and onboarding is M7. The exact acceptance
contract is in [ENGINEER_TRIAL.md](ENGINEER_TRIAL.md).

If the user starts with “так, а че мы там дальше делаем?”, recommend:

1. let one engineer run the new one-call browser orientation baseline on a known
   repository; record useful directions, omissions, unsupported claims, and
   whether the output is worth a second step;
2. capture the third M3 replay task for Prometheus from the prepared linked
   `QueryRange` target using `deepseek-v4-flash`;
3. add equivalent small tasks for NATS Server and golangci-lint;
4. compare direction usefulness, grounding, omissions, request size, and latency
   separately from JSON/contract adherence.

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
  bytes, makes one orientation call, and opens all validated directions in a
  static browser report;
- opt-in top-N focused flow expansion; flow output is flexibly parsed,
  normalized to known fields, allowlist-validated, and rejected when empty or
  unsupported;
- debug artifacts in the OS user cache by default, including model, endpoint,
  and the attempted request even when the provider fails;
- exact-symbol evidence, bounded source cards, source assessment, related test
  references, and a pure resumable investigation reducer as isolated connected
  slices;
- versioned, hash-verified offline quality tasks for etcd and Grafana k6;
- staged Qwen 1.5B protocol verification retained as non-critical regression
  tooling.

## Honest current boundary

- The main CLI shows ranked directions but cannot yet choose one by name and
  hand it into the resumable reducer.
- `--flows 1` expands the highest-ranked direction, not a user-selected one.
- `--offline` means no model call; it is not a hard network sandbox for Go tools.
- Public `onboarding` and natural-language `feature` commands do not exist.
- Test references are navigation evidence until bounded test source/assertions
  are read.
- Static Go/gopls facts are build-scenario evidence, not runtime truth.
- `internal/deepseek` still owns OpenAI-compatible transport plus concrete
  prompts; runtime configuration is provider-neutral, but orientation lacks a
  consumer-owned model interface.
- M3 has two of five tasks; Prometheus, NATS Server, and golangci-lint remain, so
  cross-repository quality is not established.
- Prometheus is preflighted, not captured: `QueryRange` is linked to the bounded
  orientation context, its source request is ready, and `TestQueryRange` passes;
  a live DeepSeek key is still needed for the two raw provider responses.

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
./scripts/quality_preflight.sh prometheus-query-range tmp/example-repos/prometheus QueryRange
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
