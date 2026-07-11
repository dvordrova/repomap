# Current handoff

Last updated: 2026-07-10.

## Current direction

repomap is evolving from a one-shot repository report into a local Go evidence
and context engine for progressive investigation. The local index should retain
repository facts, derived claims, and investigation sessions separately. A
bounded context assembler selects the next small evidence slice for a human,
weak/local model, browser UI, or external coding agent.

If the user starts with “what do we do next?”, recommend this order:

1. implement the smallest `internal/index` vertical slice for existing symbol
   evidence: put, query target neighborhood, persist/reload, invalidate one file;
2. add goal-personalized graph ranking and pack selected evidence into a roughly
   1K-token budget;
3. rerun the compact context against Qwen 3B;
4. then connect the index to the investigation reducer and source evidence.

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

Ollama setup was explicitly verified:

- CLI and server version `0.30.10`;
- one normal `Ollama.app -> ollama serve` process tree;
- `/api/version` and `/api/tags` reachable at `localhost:11434`;
- no downloads, deletes, sudo, profile changes, or duplicate servers;
- x86 macOS correctly uses CPU-only inference.

Installed models include Qwen2.5-Coder 0.5B Q4, SmolLM2 135M F16, and
Qwen2.5-Coder 3B Q4. A short Qwen 0.5B plain-text smoke test succeeded, and a
47-token Go review request returned valid JSON matching a runtime JSON Schema in
8.16 seconds warm. This proves local structured inference works for small tasks;
it does not establish repository-analysis quality.

The compact repository experiment is now replayable. A 634-token tagged prompt
ran in 18.85 seconds but produced malformed verbose output and scored 40/100. A
523-token runtime-JSON-Schema prompt completed in 142.58 seconds and scored
45/100: its shape was valid, but its interpretations copied instructions and
invented evidence IDs. Details and completion criteria are in
[TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).

Do not conclude that local inference is generally unsuitable. The next fair test
requires adaptive evidence selection from a local index, a compact prompt, a
bounded schema, and separate contract versus semantic-usefulness evaluation.

## Relevant prior art

- OpenCode performs iterative `grep/read/LSP` tool calls and compacts long
  sessions; its LLM remains the main planner.
- Aider's repo map is the closest context-selection baseline: graph ranking plus
  an active token budget, commonly around 1K tokens.
- Sourcegraph/SCIP is the closest precise persistent-index baseline.
- Cursor-style content hashing demonstrates incremental invalidation, though its
  semantic/cloud design is not the initial repomap direction.

Do not compete by inventing another general coding agent. A plausible product
boundary is a Go-specific local evidence/context service usable through its own
browser and later through MCP/custom tools by OpenCode or other agents.

## Architecture after the local-index discussion

The intended layers are:

```text
Go collectors (git/go list/gopls/source/tests)
  -> local fact index
  -> adaptive context assembly
  -> investigation reducer and claim ledger
  -> browser / DeepSeek / Ollama / future MCP tools
```

Index progressively: repository survey first, symbol graph second, focused
source/tests on demand, runtime observations only for a concrete investigation.
Do not eagerly compute every function or send the whole index to a model. Use
depth together with beam width, node/edge/source/token budgets, and expand one
high-value frontier branch when evidence is insufficient.

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

The replay evaluator and Ollama experiment scripts are committed project tooling;
their generated artifacts remain under ignored `tmp/` directories.
