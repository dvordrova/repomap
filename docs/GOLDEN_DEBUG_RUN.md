# Reproducible debug run

How to capture a complete, reproducible orientation run for the configured
OpenAI-compatible provider.

## Inspect the compact LLM bundle only (no API key)

```bash
repomap orient --repo ../etcd --llm-bundle-only > /tmp/etcd-llm-bundle.json
```

This mode prints one artifact and deliberately does not create a debug run.
Use `--preview-request` to inspect the complete provider body without sending it.

## Capture a full provider run (requires configured auth)

```bash
repomap orient --repo ../etcd --debug-dir .repomap-runs
```

Writes under `.repomap-runs/<run-id>/`:
- `metadata.json`
- `snapshot.json`
- `llm_bundle.json`
- `semantic_exchanges/<content-addressed-id>/request.{json,txt}`
- `semantic_exchanges/<content-addressed-id>/response.{json,txt}` or `response.marker.json`
- `semantic_exchanges/<content-addressed-id>/exchange.v1.json`
- `orientation_report.json`
- `error.txt` (if failure)

## Inspect the last run

```bash
./scripts/debug_last_run.sh
./scripts/debug_last_run.sh .repomap-runs
```

With no argument the script inspects the platform user-cache directory used by
the normal CLI. Pass an explicit directory for the reproducible layout above.

## Debug run directory structure

```
.repomap-runs/
  20260523-173804-etcd/
    metadata.json
    snapshot.json
    llm_bundle.json
    semantic_exchanges/
      <content-addressed-id>/
        request.json or request.txt
        response.json, response.txt, or response.marker.json
        exchange.v1.json
    orientation_report.json
    error.txt
```

## Important

- **Do not commit** `.repomap-runs/` — it may contain repository information.
- Debug artifacts never include API keys or Authorization headers.
- Sensitive values are redacted (api_key, token, password, secret, authorization, bearer).
- Semantic journal payloads are bounded; unavailable or unsafe raw bytes use closed markers.
- Directories use mode 0700, files use 0600.
