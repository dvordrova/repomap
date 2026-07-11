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

## Focused symbol investigation

Symbol investigation is a separate bounded request. It does not send the raw
evidence graph, file tree, package edge list, repository contents, environment,
or local working directory.

The local pipeline is:

1. gopls fuzzy candidates (`possible`)
2. one unique case-sensitive exact symbol resolution (`static`)
3. direct incoming/outgoing call hierarchy edges (`static`)
4. bounded `symbol_bundle.json` with evidence IDs and `allowed_paths`
5. DeepSeek request with a compact JSON or `KEY: VALUE` interpretation contract
6. tolerant local parsing with explicit repair warnings
7. deterministic reconstruction and strict validation of the normalized report

Generate and inspect the exact request without an API key:

```bash
go run ./cmd/symbol-playground \
  --repo ../etcd \
  --symbol kvServer.Put \
  --out-dir tmp/symbol-examples/etcd-put
```

This writes:

- `evidence_graph.json` — full local analyzer output; never sent
- `symbol_bundle.json` — bounded facts sent to the model
- `deepseek_request.redacted.json` — exact request body; no Authorization header

Add `--deepseek` to make the API call. Use `--format tagged` (the default) or
`--format json` as a control format. Raw output is always saved before parsing. The parser accepts
common weak-model drift such as fenced JSON, trailing commas, scalar list items,
and the tagged line format. Invalid evidence and paths are dropped with warnings;
only an unreadable response is fatal.

Both symbol prompts ask the model only for:

- a summary and likely responsibility, both explicitly treated as inference
- evidence IDs for those interpretations and for the reading order
- unknowns, concrete next queries, and warnings

The model does not copy target, caller, callee, path, or structural-role fields.
Those are reconstructed from the local bundle. The normalized report still
enforces:

- every substantive statement cites an existing `evidence_id`
- static-only summary/responsibility inference confidence to be capped at 0.75
- static call edges to never be presented as observed runtime execution
- caller/callee identity to match the cited call fact
- every referenced file to appear in `allowed_paths` and in cited evidence
- file recommendations to use validated structural roles instead of semantic prose
- `tests_to_read` to contain only evidence-backed `*_test.go` files
- runtime paths/hypotheses to be omitted at depth one; missing runtime evidence belongs in unknowns and next queries

The JSON wire contract is intentionally smaller than `internal/symbol.Report`:

```json
{
  "summary": {"statement":"...","evidence_ids":["resolution-001"],"confidence":0.7},
  "responsibility": {"statement":"...","evidence_ids":["call-out-001"],"confidence":0.6},
  "read_evidence_ids": ["resolution-001", "call-out-001"],
  "test_evidence_ids": [],
  "unknowns": [],
  "next_queries": [],
  "warnings": []
}
```

The tagged contract expresses the same information as repeated `KEY: VALUE`
lines. `symbol_evaluation.json` scores observable contract adherence out of 100;
it deliberately does not pretend to score semantic truth. Run and compare real
prompt versions with:

```bash
./scripts/symbol_prompt_experiment.sh baseline ../etcd kvServer.Put json
./scripts/symbol_prompt_experiment.sh tagged ../etcd kvServer.Put tagged
./scripts/symbol_prompt_compare.sh \
  tmp/prompt-experiments/baseline-json \
  tmp/prompt-experiments/tagged-tagged
```

Stable bundle/response fixtures and an in-memory explainer live in
`internal/deepseektest`; they let higher layers test without calling DeepSeek.

## Error handling

- Non-2xx HTTP response: return error with status code and response body (redacted).
- Orientation responses still require JSON. Focused symbol responses are parsed
  tolerantly and fail only when neither a JSON object nor tagged report can be recovered.
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
