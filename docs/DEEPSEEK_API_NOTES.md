# DeepSeek API notes

Implementation reference for the DeepSeek client in `internal/deepseek/`.

## Endpoint

```
https://api.deepseek.com/chat/completions
```

Configurable via `DEEPSEEK_ENDPOINT` env var.

## Authentication

Bearer token via `DEEPSEEK_API_KEY`. Sent in `Authorization: Bearer <key>` header.

Must NEVER be written to disk or debug artifacts.

## Model

`DEEPSEEK_MODEL` env var. Default: `deepseek-v4-flash`.

## Request shape

```json
{
  "model": "deepseek-v4-flash",
  "messages": [
    {
      "role": "system",
      "content": "You are a senior Go engineer helping orient inside a large unfamiliar Go repository. Use only the provided facts. Do not pretend to have read files that were not provided. Return valid json only."
    },
    {
      "role": "user",
      "content": "Do not explain the whole repo. Help the developer choose what runtime/event flow to inspect next.\n\nProduce a json orientation report with this exact shape:\n{...}\n\nImportant rules:\n- Candidate flows must be runtime/event-oriented ...\n\nFacts bundle JSON:\n{...}"
    }
  ],
  "temperature": 0.1,
  "max_tokens": 4000,
  "response_format": {"type": "json_object"}
}
```

## JSON mode requirements

- Request must include `"response_format": {"type": "json_object"}`.
- Prompt must contain the word **json** (case-insensitive match is fine).
- Prompt must include an example JSON shape so the model knows the expected schema.
- `max_tokens` must be high enough to avoid truncation (default 4000, configurable via `DEEPSEEK_MAX_TOKENS`).

## Expected orientation report shape

```json
{
  "project_guess": "short guess",
  "confidence": 0.0,
  "high_level_map": [
    {
      "name": "component or subsystem name",
      "evidence": ["facts or paths from bundle"],
      "why_it_matters": "..."
    }
  ],
  "first_files_to_open": [
    {"path": "repo-relative path", "reason": "..."}
  ],
  "candidate_flows": [
    {
      "name": "runtime or event flow name",
      "trigger": "what starts this flow",
      "likely_entrypoint": "package or repo-relative file",
      "likely_files": ["repo-relative paths"],
      "why_interesting": "...",
      "evidence": ["facts from bundle supporting this flow"],
      "confidence": 0.0
    }
  ],
  "important_domain_words": [
    {
      "word": "term",
      "guess": "what it probably means",
      "evidence": ["paths or readme excerpts"]
    }
  ],
  "questions_for_human": [
    "question that helps guide next analysis step"
  ],
  "warnings": [
    "uncertainty or missing context"
  ]
}
```

## Error handling

- Non-2xx HTTP response: return error with status code and response body (redacted).
- Invalid JSON in response content: return error with raw content, save to debug artifact if `--dump-llm`.
- `DEEPSEEK_API_KEY` not required for `--snapshot-only` or `--llm-bundle-only`.

## Retry behavior

- Retry on network errors (unless context canceled/exceeded).
- Retry on HTTP 429 and 5xx.
- No retry on HTTP 4xx (except 429).
- Small exponential backoff with jitter, max 3 retries.

## Debugging

```bash
repomap orient --repo ../etcd --debug-dir .repomap-runs --dump-llm
./scripts/debug_last_run.sh
```

Artifacts produced:
- `metadata.json` — run metadata (model, endpoint, command)
- `snapshot.json` — full local deterministic snapshot
- `llm_bundle.json` — compact bounded bundle sent to DeepSeek
- `llm_request.redacted.json` — full request body (no Authorization header)
- `llm_response.raw.json` — raw DeepSeek HTTP response body (redacted)
- `orientation_report.json` — parsed/pretty orientation report
- `error.txt` — error message if any step failed

Never commit `.repomap-runs/`.
