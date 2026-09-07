# Online LLM transport

This document covers only the OpenAI-compatible HTTP transport used by the
ordinary online `repomap` path. Product decisions live in
[agent-room/CURRENT.md](agent-room/CURRENT.md); prompt and response schemas live
next to their Go implementations.

## Configuration

The preferred configuration is one atomic namespace:

```text
REPOMAP_LLM_ENDPOINT      full chat/completions URL
REPOMAP_LLM_MODEL         model name (default: deepseek-v4-flash)
REPOMAP_LLM_API_KEY       bearer credential
REPOMAP_LLM_AUTH          bearer (default) or none
REPOMAP_LLM_MAX_TOKENS    positive integer (default: 128000)
REPOMAP_LLM_TIMEOUT       positive Go duration (default: 10m)
REPOMAP_LLM_CHAT_TEMPLATE_KWARGS  JSON object overriding template options
```

If any `REPOMAP_LLM_*` variable is present, that namespace is authoritative:
`REPOMAP_LLM_ENDPOINT` is required and no `DEEPSEEK_*` value is inherited.
This prevents a credential configured for one endpoint from silently reaching
another.

When no generic variable is present, the legacy `DEEPSEEK_ENDPOINT`,
`DEEPSEEK_MODEL`, `DEEPSEEK_API_KEY`, `DEEPSEEK_AUTH`, and
`DEEPSEEK_TIMEOUT` and `DEEPSEEK_CHAT_TEMPLATE_KWARGS` names remain accepted. Their default endpoint is
`https://api.deepseek.com/chat/completions`. There is no legacy max-token
override; `REPOMAP_LLM_MAX_TOKENS` is the only one.

`bearer` requires a key and sends `Authorization: Bearer ...`. `none`
requires an explicit endpoint and sends no Authorization header. Endpoints must
be HTTP(S) URLs with a host and without userinfo, query, or fragment.

`repomap` does not source an analyzed repository's `.env` file. Configuration
comes from the caller's environment.

Custom endpoint requests default to
`"chat_template_kwargs":{"enable_thinking":false}`. The final answer table's
reasoning preference does not override that default. The configured JSON object
replaces these template options; `{}` explicitly omits the field. Invalid JSON,
null, arrays and scalar values are rejected during configuration. The endpoint
host, independent of the environment-variable family, selects the native
DeepSeek behavior: exactly `api.deepseek.com` (case-insensitive) uses its
existing `thinking` control and receives no default template-kwargs extension.
No model-name or path-name heuristic selects a provider.

This extension is documented by [vLLM](https://docs.vllm.ai/en/stable/features/reasoning_outputs/)
and [SGLang for Qwen](https://docs.sglang.io/cookbook/autoregressive/Qwen/Qwen3.5).
It is not universal: unsupported servers may reject it with HTTP 400. Repomap
does not silently retry without the owner's thinking control. The options live
at the top level of the wire JSON, not under an `extra_body` object. They
participate in exact request/cache identity; retries and replay preserve the
saved request bytes rather than applying new environment settings to them.

## Wire and failure contract

- Requests are JSON `POST` calls with `model`, `messages`,
  `max_tokens`, and, for structured stages,
  `response_format: {"type":"json_object"}`.
- Domain cubes own system/user prompt text, response schemas, request-local
  catalogs, and semantic validation. The shared provider adapter alone builds
  the transport envelope, applies the configured output-token ceiling, and
  applies endpoint-specific transport fields such as DeepSeek thinking mode.
  It does not add semantic instructions.
- The serialized request is immutable across transport retries. Retryable
  network errors and HTTP statuses receive at most three retries after the
  first attempt; schema or semantic rejection never triggers a new model call.
- After HTTP 429, each retry waits at least one minute. A longer `Retry-After`
  delay, expressed in seconds or as an HTTP date, is honored. An absent,
  invalid, expired or shorter header retains the one-minute minimum. The
  wait can be canceled and does not change request bytes or cache identity.
  Other retryable failures keep their existing short exponential backoff.
- Compatible-server 429 bodies may also report relative waits as
  `retry after 9.636307001s, reset after 45.636307001s`. Positive Go-duration
  values in these two phrases extend the same cooldown. The adapter reads plain
  error text, a JSON `message`, a JSON string `error`, or `error.message`, and
  uses the longest valid body hint, `Retry-After` header and one-minute floor.
  Invalid, negative and unitless body values are ignored. The original error
  bytes remain unchanged. Other HTTP statuses do not use body wait hints.
- Independent batch calls start behind a run-shared bounded attempt gate. The
  ordinary product limit is four live provider attempts. A DeepSeek HTTP 429
  atomically collapses that gate to one and sets the same cooldown before
  releasing the failed attempt. New calls in other batches sharing this gate
  also wait; another 429 may extend, but never shorten, the cooldown.
  Attempts already on the wire are not
  replayed or canceled merely because of the 429; once they finish, that retry
  and all later attempts in the run are serialized. If retries are exhausted,
  the terminal batch item cancels the batch child context, stops queued work,
  and asks in-flight HTTP requests to terminate through their request context.
  Client cancellation is a fail-fast transport mechanism, not a guarantee
  about provider-side billing after disconnect.
- Responses are byte-bounded. The adapter decodes exactly one provider choice
  and its finish reason; the shared executor then accepts one unambiguous JSON
  object or array with harmless whitespace, one JSON fence, or short leading
  prose. A single complete leading `<think>...</think>` block is separated
  before JSON parsing, so draft JSON and code fences inside it are never
  mistaken for the final answer. Missing closing tags, nested/repeated leading
  blocks and invalid final JSON remain rejected. This does not disable provider
  reasoning; the complete original response remains in diagnostics and cache.
  It never repairs fields, refs, schema, or values. Non-2xx outcomes
  retain their bounded response bytes for the normal secret-guarded semantic
  journal; a sensitive body is reduced to its guarded hash/count metadata.
  The user-facing error reports only a closed failure class, safe HTTP status,
  transport-attempt count, and fixed corrective guidance.
- An explicit HTTP 400/413 context overflow is a typed `context_tokens`
  resource result, with the unchanged body retained for diagnostics. The
  adapter recognizes the closed `context_length_exceeded` code, the explicit
  `Input token exceed the limit` message, or a numeric maximum-context refusal
  whose requested/input/reserved-output counts agree. A generic 400, quota
  failure or unsupported parameter does not authorize splitting. Transport
  does not retry unchanged bytes for this failure. A cube with a lossless
  repartition can split complete input items through the shared executor;
  an indivisible input remains a resource failure. The actual 2026-09-07 etcd
  probe reported 1,656,470 input plus 128,000 reserved output tokens against
  a 1,048,576-token provider context. This is an observed refusal, not a new
  hardcoded product token limit.
- Provider inputs remain bounded and request-local. A domain cube may include
  a complete names-only tracked-file dictionary when its contract requires
  repository-wide matching; that is not permission to include corresponding
  source contents or raw internal edges. Full repository source, raw internal
  edges, canonical Atlas IDs, API keys, and Authorization headers must never
  enter saved debug artifacts.

## Saved request replay

`repomap replay --file REQUEST.json` uses the same Client.Complete path as an
ordinary run. It accepts the exact provider request payload, not an atlas table
input or a `.ref.json` file. Saved model, messages, temperature, output ceiling,
thinking mode, response format and unknown provider options are preserved byte
for byte. Current environment defaults do not rebuild the request. Endpoint,
authentication, timeout, bounded response handling and retry policy are supplied
by the configured client. Streaming requests are unsupported.

Replay always calls the provider and replaces the shared exact-request answer
on successful provider-envelope and JSON validation. Owning stages revalidate
their own schemas on reuse. Request/response payloads are content-addressed in
`.llm-cache/payloads`; journal records reference those files. Use the original
run's `--debug-dir` to update the same cache. Existing run snapshots are not
rewritten. A failed replay keeps the previous accepted answer.
