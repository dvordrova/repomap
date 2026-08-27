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
```

If any `REPOMAP_LLM_*` variable is present, that namespace is authoritative:
`REPOMAP_LLM_ENDPOINT` is required and no `DEEPSEEK_*` value is inherited.
This prevents a credential configured for one endpoint from silently reaching
another.

When no generic variable is present, the legacy `DEEPSEEK_ENDPOINT`,
`DEEPSEEK_MODEL`, `DEEPSEEK_API_KEY`, `DEEPSEEK_AUTH`, and
`DEEPSEEK_TIMEOUT` names remain accepted. Their default endpoint is
`https://api.deepseek.com/chat/completions`. There is no legacy max-token
override; `REPOMAP_LLM_MAX_TOKENS` is the only one.

`bearer` requires a key and sends `Authorization: Bearer ...`. `none`
requires an explicit endpoint and sends no Authorization header. Endpoints must
be HTTP(S) URLs with a host and without userinfo, query, or fragment.

`repomap` does not source an analyzed repository's `.env` file. Configuration
comes from the caller's environment.

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
- Independent batch calls start behind a run-shared bounded attempt gate. The
  ordinary product limit is four live provider attempts. A DeepSeek HTTP 429
  atomically collapses that gate to one before releasing the failed attempt and
  entering the existing retry backoff. Attempts already on the wire are not
  replayed or canceled merely because of the 429; once they finish, that retry
  and all later attempts in the run are serialized. If retries are exhausted,
  the terminal batch item cancels the batch child context, stops queued work,
  and asks in-flight HTTP requests to terminate through their request context.
  Client cancellation is a fail-fast transport mechanism, not a guarantee
  about provider-side billing after disconnect.
- Responses are byte-bounded. The adapter decodes exactly one provider choice
  and its finish reason; the shared executor then accepts one unambiguous JSON
  object or array with harmless whitespace, one JSON fence, or short leading
  prose. It never repairs fields, refs, schema, or values. Non-2xx outcomes
  retain their bounded response bytes for the normal secret-guarded semantic
  journal; a sensitive body is reduced to its guarded hash/count metadata.
  The user-facing error reports only a closed failure class, safe HTTP status,
  transport-attempt count, and fixed corrective guidance.
- Provider inputs remain bounded and request-local. A domain cube may include
  a complete names-only tracked-file dictionary when its contract requires
  repository-wide matching; that is not permission to include corresponding
  source contents or raw internal edges. Full repository source, raw internal
  edges, canonical Atlas IDs, API keys, and Authorization headers must never
  enter saved debug artifacts.
