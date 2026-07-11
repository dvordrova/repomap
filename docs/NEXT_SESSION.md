# Current handoff

Last updated: 2026-07-10.

## Current direction

repomap is evolving from a one-shot repository report into a progressive,
evidence-guided investigation tool. The next architectural slice is a shared
investigation state machine and source-supported understanding of one selected
Go symbol.

Read these first:

1. [CORE_IDEA.md](CORE_IDEA.md) — current pipeline and constraints;
2. [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md) — shared workflow proposal;
3. [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md) — demonstrated implementation gaps;
4. [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) — unresolved product/research decisions;
5. [DEEPSEEK_API_NOTES.md](DEEPSEEK_API_NOTES.md) — current provider contract.

## What works

- deterministic repository snapshot, Go facts, source signals, and bounded LLM
  bundle;
- orientation and flow reports with tolerant response parsing;
- language-neutral evidence graph with certainty, provenance, and scenarios;
- isolated gopls analyzer and example runs for large Go repositories;
- exact-symbol bundle with bounded callers/callees and locally rebuilt structure;
- JSON and tagged symbol prompts, tolerant normalization, evaluator, fixtures,
  and replayable HTTP integration tests;
- prompt experiment scripts and etcd `kvServer.Put` calibration artifacts under
  ignored `tmp/` directories.

## Recent local-provider experiment

Ollama 0.30.10 was tested through its OpenAI-compatible endpoint on an Intel
MacBook Pro. Ollama correctly used CPU-only inference. The full 21 KB symbol
request was too slow for useful local interaction, and sub-billion-parameter
models produced weak or invalid explanations. Exact measurements and resulting
debt are recorded in [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).

Do not conclude that local inference is generally unsuitable. The next fair test
requires a compact 1,000–2,000-token local prompt profile and a quantized model.

## Recommended next implementation

1. Add the minimal records and pure reducer in `internal/investigation`.
2. Express the existing symbol workflow as reducer events/actions without
   changing provider or evidence behavior.
3. Add a Go source-evidence playground for the selected target only.
4. Reassess one important claim using source evidence.
5. Add stored-response replay and semantic-usefulness evaluator fixtures.
6. Repeat DeepSeek versus local-provider calibration on the compact profile.

## Verification

```bash
./scripts/check.sh
./scripts/etcd_check.sh ../etcd
./scripts/symbol_check.sh ../etcd kvServer.Put
```

## Workspace caution

The current worktree contains pre-existing, uncommitted rewrites under
`docs/agent-room/` and an untracked `opencode.json`. They were intentionally
excluded from the focused commit stack. Do not stage, restore, or overwrite them
without deciding their intended fate.
