# Current handoff

Last updated: 2026-07-10.

## Current direction

repomap is evolving from a one-shot repository report into a local Go evidence
and context engine for progressive investigation. The local index should retain
repository facts, derived claims, and investigation sessions separately. A
bounded context assembler selects the next small evidence slice for a human,
weak/local model, browser UI, or external coding agent.

The canonical execution order is [MILESTONES.md](MILESTONES.md). DeepSeek through
its OpenAI-compatible API is the default interpretation and product-quality
target. Existing Qwen 1.5B work remains a useful prototype, but no new local
model work is on the critical path. Later, `--really-dumb-model` may select
alternate implementations of the same typed capability contracts.

If the user starts with “what do we do next?”, recommend this order:

1. implement the M2 investigation state/event/action types and a pure reducer;
2. replay the completed `kvServer.Put` symbol -> source -> claims -> test
   references path through reducer-requested capability actions;
3. persist and resume that state only after the reducer transitions are stable;
4. then let repository orientation hand one chosen flow/symbol into the same
   reducer. Dumb-model implementations remain deferred.

Read these first:

1. [CORE_IDEA.md](CORE_IDEA.md) — current pipeline and constraints;
2. [MILESTONES.md](MILESTONES.md) — ordered product outcomes and completion gates;
3. [SYSTEM_MAP.md](SYSTEM_MAP.md) — current/planned modules and challenge cards;
4. [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md) — shared workflow proposal;
5. [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md) — demonstrated implementation gaps;
6. [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) — unresolved product/research decisions;
7. [DEEPSEEK_API_NOTES.md](DEEPSEEK_API_NOTES.md) — current provider contract.

## What works

- deterministic repository snapshot, Go facts, source signals, and bounded LLM
  bundle;
- orientation and flow reports with tolerant response parsing;
- language-neutral evidence graph with certainty, provenance, and scenarios;
- isolated gopls analyzer and example runs for large Go repositories;
- exact-symbol bundle with bounded callers/callees and locally rebuilt structure;
- bounded lexical source cards with containment, size, credential, hash, and
  line-addressability checks;
- DeepSeek source-assessment v2 with predicate-specific lexical support,
  conservative locally reconstructed claims, explicit unknowns, replay fixture,
  and raw-response retention on parse failure;
- gopls test-reference collection with provider/version/provenance/build scenario;
  references remain navigation evidence and are not called test-supported;
- pure investigation reducer plus an explicit capability runner; the real etcd
  path reaches `assessing_source` locally and `waiting_user` with DeepSeek,
  while stale action IDs, revision changes, redirects, cancellation, and failure
  are table-tested;
- versioned local symbol evidence index with defensive put/query,
  deterministic persistence/reload, and path-based invalidation;
- JSON and tagged symbol prompts, tolerant normalization, evaluator, fixtures,
  and replayable HTTP integration tests;
- prompt experiment scripts and etcd `kvServer.Put` calibration artifacts under
  ignored `tmp/` directories.
- replayable `local-symbol-v2` Ollama protocol with deterministic name signals,
  dynamic schemas, constrained role/action decisions, and offline artifact
  verification.

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

Installed models include Qwen2.5-Coder 0.5B, 1.5B, and 3B Q4 plus SmolLM2 135M
F16. A short Qwen 0.5B plain-text smoke test succeeded, and a 47-token Go review
request returned valid JSON matching a runtime JSON Schema in 8.16 seconds warm.
This proves local structured inference works for small tasks; it does not
establish repository-analysis quality.

The compact repository experiment is now replayable. A 634-token tagged prompt
ran in 18.85 seconds but produced malformed verbose output and scored 40/100. A
523-token runtime-JSON-Schema prompt completed in 142.58 seconds and scored
45/100: its shape was valid, but its interpretations copied instructions and
invented evidence IDs. Details and completion criteria are in
[TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).

Qwen2.5-Coder 1.5B ran the same 523-token compact JSON-Schema request in 22.64
seconds at 18.71 output tokens/second and scored 60/100. It is fast enough to
iterate with, but hallucinated `exampleKey`/`exampleValue` and attached valid yet
irrelevant evidence IDs. This exposed another evaluator blind spot rather than
establishing acceptable semantic quality.

The follow-up `local-symbol-v2` protocol removed model prose entirely. It
preclassifies high-confidence name signals locally, ranks deterministic evidence
above ambiguous model hints, constrains role choices by available capabilities,
and asks the model to choose an executable action. Three consecutive
`kvServer.Put` runs were identical at 3.98–4.08 seconds with two model calls,
380 input tokens, 63 output tokens, 9/9 protocol checks, and a locally rendered
100/100 contract report. Additional `kvServer.DeleteRange` and `WAL.Save` runs
also passed all protocol checks. The staged protocol's `read_target` action has
now been executed by the default DeepSeek path. This does not make local-model
source assessment solved; it gives future dumb-model work a stable source-card
and assessment-cube contract to target.

Do not conclude that local inference is generally solved. The staged result proves
that 1.5B is useful for constrained selection and planning; behavioral claims
still require the selected source/test/runtime evidence.

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
./scripts/source_artifacts_check.sh ../etcd kvServer.Put
./scripts/source_check.sh
./scripts/investigation_check.sh ../etcd kvServer.Put
```

## Workspace caution

The current worktree contains pre-existing, uncommitted rewrites under
`docs/agent-room/` and an untracked `opencode.json`. They were intentionally
excluded from the focused commit stack. Do not stage, restore, or overwrite them
without deciding their intended fate.

The replay evaluator and Ollama experiment scripts are committed project tooling;
their generated artifacts remain under ignored `tmp/` directories.

The first `internal/index` slice is implemented but its repository metadata is
still supplied by the caller. Before treating it as an automatic incremental
cache, wire revision, dirty-file content, analyzer version, and build context
into its freshness policy; see TD-008 in [TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).
