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

A compact static-facts prompt reduced Qwen 0.5B input from 5,512 to 634 tokens.
The tagged attempt completed in 18.85 seconds but misunderstood `KEY: VALUE`,
generated Markdown, hit the 320-token limit, and scored 40/100. A subsequent
JSON-Schema experiment with a 523-token input completed without truncation in
142.58 seconds and scored 45/100. The schema guaranteed the outer shape, but the
model copied an instruction as both interpretations and invented evidence IDs;
local normalization discarded or repaired those claims. Constrained decoding is
therefore reliable enough for contract testing but too slow and semantically weak
for this repository task on the 0.5B model.

The same 523-token JSON-Schema experiment on Qwen2.5-Coder 1.5B completed in
22.64 seconds (18.71 output tokens/second) and scored 60/100. It followed the
schema, used only known evidence IDs, and proposed a grounded reading order, but
invented concrete `exampleKey`/`exampleValue` request values and cited valid but
semantically irrelevant call edges for its claims. The model is fast enough for
iteration on this machine; the result is not yet trustworthy repository
understanding.

`local-symbol-v2` then replaced the monolithic prose request with deterministic
name preclassification plus two constrained model decisions: choose a role over
prioritized evidence, then choose an executable next action. Qwen 1.5B has no
free-text output fields in this protocol. Three consecutive `kvServer.Put` runs
were identical:

| Target | Model calls | Input/output tokens | Time | Protocol checks |
| --- | ---: | ---: | ---: | ---: |
| `kvServer.Put` | 2 | 380 / 63 | 3.98–4.08 s | 9/9 |
| `kvServer.DeleteRange` | 2 | 387 / 63 | 5.83 s | 9/9 |
| `WAL.Save` | 3 | 579 / 118 | 8.12 s | 8/8 |

All runs had zero parser warnings and the locally rendered report scored 100/100
on the existing contract evaluator. Put/DeleteRange selected validation, error
translation, and delegation evidence; WAL selected `sync`, `saveEntry`, and
`saveState`. Each result conservatively chose `read_target` because source
behavior was still absent. These scores validate the staged protocol and reducer,
not semantic truth about the source.

The same Qwen 0.5B model succeeds on genuinely small requests:

| Smoke test | Context | Time | Outcome |
| --- | ---: | ---: | --- |
| one-sentence Go explanation | 22 input tokens | 10.60 s cold | normal text response |
| code review with runtime JSON Schema | 47 input tokens | 8.16 s warm | valid required fields and enums |

The structured review was parseable but semantically generic: it suggested
general read error handling instead of naming `scanner.Err()`. This is sufficient
for provider/contract validation, not proof of useful repository understanding.

The machine is an x86 Intel Mac. Ollama correctly uses CPU-only inference on
this platform; the Radeon Pro 560X is not used by the macOS Ollama runtime.

**Done when:** the staged profile is fed by indexed context rather than a saved
bundle, executes its chosen `read_target` action into bounded source evidence,
and is measured against Qwen 3B. The full remote bundle remains available.

### TD-004: Prompt evaluator overestimates semantically useless responses

**Evidence:** the Qwen 0.5B response followed most tags but copied the placeholder
responsibility, cited a non-test target as `TEST`, claimed there were no unknowns,
and repeated an instruction as `NEXT_QUERY`. The current structural rubric would
still assign a relatively high score. Qwen 1.5B then scored 60/100 while inventing
concrete request values: its evidence IDs existed, but did not support the claims
that cited them.

**Consequence:** prompt experiments can appear successful while producing little
useful repository understanding.

The staged protocol avoids this failure mode by eliminating model prose and
scoring constrained decisions separately. It does not repair the generic prose
evaluator, which is still used by DeepSeek and monolithic experiments.

**Done when:** fixtures cover prompt/template echo, invalid test evidence, empty or
vacuous unknowns, malformed next queries, and claims that merely restate symbol
identity. They must also cover novel concrete literals and valid-but-irrelevant
citations. Contract score and semantic-usefulness score should be reported
separately.

### TD-005: Experiment artifacts lack explicit contract versions

**Evidence:** `cmd/symbol-evaluate` now normalizes and scores a captured response
without another model call, and `scripts/ollama_symbol_experiment.sh` records the
native Ollama request, envelope, raw response, timing, normalized report, parser
warnings, and evaluation. The artifacts do not yet record stable prompt, schema,
parser, or evaluator version identifiers.

`ollama_symbol_staged_experiment.sh` does record protocol, prompt, schema,
reducer, evaluator, model, Ollama, options, and bundle-hash metadata. The older
monolithic path still lacks equivalent versioning.

**Done when:** experiment metadata identifies provider plus stable prompt, schema,
parser, and evaluator versions, so results remain comparable after any of those
contracts change. Artifacts must continue to exclude credentials and
authorization headers.

### TD-006: Investigation orchestration is still scenario-specific

**Evidence:** the first symbol/source/test-reference path now runs through a
versioned, replayable investigation session and pure reducer. Repository
orientation still produces its own report and cannot hand a selected flow or
symbol into that session; ticket, bug, onboarding, and impact policies are not
wired either.

**Consequence:** new product modes risk becoming separate prompts and orchestration
paths instead of policies over the same evidence loop.

**Done when:** repository orientation hands one selected flow or symbol into the
same reducer/action model, and a saved session can resume without weakening the
current evidence guarantees. Other playbooks may remain unimplemented.

### TD-007: Test references do not establish test support

**Evidence:** the M1 `find_tests` capability produces bounded gopls locations
with static provenance and build scenario, but deliberately marks them only as
`test_reference`. It does not read the test body or determine what is asserted.

**Consequence:** related tests are useful navigation, but no behavioral claim may
be promoted to `test_supported` from a reference location alone.

**Done when:** an investigation can select one reference, collect bounded test
source, identify the relevant test/case and assertion with cited evidence IDs,
and either promote a matching claim or preserve an explicit contradiction or
unknown. This must remain lazy rather than parsing every test eagerly.

### TD-008: Local index freshness metadata is not collected automatically

**Evidence:** `internal/index` persists bounded symbol neighborhoods and can
invalidate every neighborhood that references a changed path. Its repository and
revision metadata are currently supplied by the caller, and no production path
yet records dirty-file content, Go/gopls versions, or build context.

**Consequence:** the storage mechanism is usable and testable, but treating a
loaded snapshot as fresh without an explicit caller policy could serve stale
static evidence.

**Done when:** the symbol collection path derives a stable repository identity,
HEAD plus dirty-file content hashes, analyzer versions, and build context; load
either rejects incompatible snapshots or invalidates only affected records.

## Maintenance rules

- Add an item only when there is concrete evidence or a demonstrated gap.
- State the consequence and an observable completion condition.
- Do not use this file as a feature wishlist.
- Remove or move resolved items into the commit/decision that resolved them.
