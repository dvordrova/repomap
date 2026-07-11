# Reproducible debug run

How to capture a complete, reproducible debug run for DeepSeek orientation.

## Capture the compact LLM bundle only (no API key)

```bash
repomap orient --repo ../etcd --llm-bundle-only --debug-dir .repomap-runs
# or
./scripts/capture_etcd_bundle.sh ../etcd
```

Writes under `.repomap-runs/<run-id>/`:
- `metadata.json`
- `snapshot.json`
- `llm_bundle.json`

## Capture a full DeepSeek run (requires API key)

```bash
repomap orient --repo ../etcd --debug-dir .repomap-runs --dump-llm
# or
./scripts/capture_deepseek_run.sh ../etcd
```

Writes under `.repomap-runs/<run-id>/`:
- `metadata.json`
- `snapshot.json`
- `llm_bundle.json`
- `llm_request.redacted.json`
- `llm_response.raw.json`
- `orientation_report.json`
- `error.txt` (if failure)

## Inspect the last run

```bash
./scripts/debug_last_run.sh
```

## Debug run directory structure

```
.repomap-runs/
  20260523-173804-etcd/
    metadata.json
    snapshot.json
    llm_bundle.json
    llm_request.redacted.json
    llm_response.raw.json
    orientation_report.json
    error.txt
```

## Important

- **Do not commit** `.repomap-runs/` — it may contain repository information.
- Debug artifacts never include API keys or Authorization headers.
- Sensitive values are redacted (api_key, token, password, secret, authorization, bearer).
- Directories use mode 0700, files use 0600.
