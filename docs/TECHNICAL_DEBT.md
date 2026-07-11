# Technical debt ledger

This file tracks concrete implementation debt discovered by experiments. Product
questions belong in [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md); planned investigation
architecture belongs in [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md).

Last reviewed: 2026-07-10.

## Active

### TD-001: Model integration is named and configured as DeepSeek

**Evidence:** the OpenAI-compatible Ollama endpoint accepted the existing request,
but configuration and artifacts still use `DEEPSEEK_*` and `deepseek_*` names.

**Consequence:** provider details leak into CLI configuration and orchestration,
making local or alternative hosted providers look like unsupported hacks.

**Done when:** the consuming layer owns a small model-client contract; endpoint,
model, authentication, timeout, and output mode are provider-neutral; DeepSeek
configuration remains available through documented compatibility aliases.

### TD-002: HTTP timeout and cancellation do not fit local inference

**Evidence:** the client has a fixed 60-second timeout. Qwen 3B on local CPU did
not return headers before it expired. Ollama continued generation after the HTTP
client timed out, and a retry waited behind the abandoned request.

**Consequence:** slow local providers fail even when healthy and may waste CPU
after cancellation.

**Done when:** timeout is configurable per provider/profile, cancellation behavior
has an integration test, and retries do not amplify an already-running local
generation.

### TD-003: The full symbol prompt is unsuitable for CPU-local models

**Evidence:** the etcd `kvServer.Put` experiment produced a 16,962-byte bundle
and a 21,007-byte tagged request. Ollama counted 5,512 input tokens for Qwen 0.5B.

Local baselines on an Intel i7-9750H MacBook Pro:

| Model | Format | Time | Outcome |
| --- | --- | ---: | --- |
| Qwen2.5-Coder 3B Q4 | tagged | >120 s | interrupted; context fit, latency did not |
| Qwen2.5-Coder 0.5B Q4 | tagged | 81.91 s | parseable but mostly copied prompt placeholders |
| SmolLM2 135M F16 | tagged | 142.81 s | ignored contract and hallucinated unrelated code |

The machine is an x86 Intel Mac. Ollama correctly uses CPU-only inference on
this platform; the Radeon Pro 560X is not used by the macOS Ollama runtime.

**Done when:** a local profile selects a bounded subset of evidence, targets
roughly 1,000–2,000 input tokens, limits output to 250–400 tokens, and is measured
again with a quantized local model. The full remote bundle remains available.

### TD-004: Prompt evaluator overestimates semantically useless responses

**Evidence:** the Qwen 0.5B response followed most tags but copied the placeholder
responsibility, cited a non-test target as `TEST`, claimed there were no unknowns,
and repeated an instruction as `NEXT_QUERY`. The current structural rubric would
still assign a relatively high score.

**Consequence:** prompt experiments can appear successful while producing little
useful repository understanding.

**Done when:** fixtures cover prompt/template echo, invalid test evidence, empty or
vacuous unknowns, malformed next queries, and claims that merely restate symbol
identity. Contract score and semantic-usefulness score should be reported
separately.

### TD-005: Provider experiments are not replayable offline

**Evidence:** `symbol-playground` can call a provider and score the response, but
cannot parse and evaluate an already captured raw response. Direct Ollama calls
therefore required manual inspection.

**Done when:** a saved response can be normalized and evaluated without another
model call. Experiment metadata records provider, model, prompt/evaluator version,
input/output tokens, latency, finish reason, and parse warnings. Artifacts must not
contain credentials or authorization headers.

### TD-006: Investigation orchestration is still scenario-specific

**Evidence:** repository orientation and symbol investigation exist, but there is
no persisted goal/focus/question/claim/action session shared by explore, ticket,
bug, onboarding, and impact-analysis modes.

**Consequence:** new product modes risk becoming separate prompts and orchestration
paths instead of policies over the same evidence loop.

**Done when:** the first symbol workflow runs through the reducer and action model
described in [INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md), without changing
its current evidence guarantees.

### TD-007: Source-supported function understanding is missing

**Evidence:** current symbol explanations are based on identity and depth-one
static calls. A claim such as “validates a request” is still an inference from
names until source or tests corroborate it.

**Done when:** a selected Go symbol can produce bounded source evidence for its
signature, documentation, body-level calls/conditions/returns, referenced local
types, and relevant test locations. Source-supported claims must cite those
evidence IDs; the system must not parse every function eagerly.

## Maintenance rules

- Add an item only when there is concrete evidence or a demonstrated gap.
- State the consequence and an observable completion condition.
- Do not use this file as a feature wishlist.
- Remove or move resolved items into the commit/decision that resolved them.
