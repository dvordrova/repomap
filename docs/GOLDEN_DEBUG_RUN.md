# Reproducible debug run

How to capture a complete, reproducible Atlas-first run for the configured
OpenAI-compatible provider.

## Capture a provider-free local run

```bash
repomap ../etcd --offline --debug-dir .repomap-runs --no-open --no-serve
```

This persists the complete local Atlas and local Map/Entrypoints projections
without making a provider call.

## Capture a full provider run (requires configured auth)

```bash
repomap ../etcd --debug-dir .repomap-runs --no-open --no-serve
```

Writes under `.repomap-runs/<run-id>/`:
- `metadata.json`
- `snapshot.json`
- `repository_atlas.v1.json`
- `architecture_synthesis_status.json` with bounded conceptual and
  primary/supporting coverage evidence when Architecture is attempted
- `semantic_exchanges/<content-addressed-id>/request.{json,txt}`
- `semantic_exchanges/<content-addressed-id>/response.{json,txt}` or `response.marker.json`
- `semantic_exchanges/<content-addressed-id>/exchange.v2.json`
- `report.json`
- `report.html`
- `error.txt` (if failure)

## Inspect the last run

The CLI prints the exact artifact directory near the start of every ordinary
run. Open its `run_manifest.json`, `report.json` and `report.html` directly.

## Debug run directory structure

```
.repomap-runs/
  20260523-173804-etcd/
    metadata.json
    snapshot.json
    repository_atlas.v1.json
    semantic_exchanges/
      <content-addressed-id>/
        request.json or request.txt
        response.json, response.txt, or response.marker.json
        exchange.v2.json
    run_manifest.json
    report.json
    report.html
    error.txt
```

## Important

- **Do not commit** `.repomap-runs/` — it may contain repository information.
- Debug artifacts never include API keys or Authorization headers.
- Sensitive values are redacted (api_key, token, password, secret, authorization, bearer).
- Semantic journal payloads are bounded; unavailable or unsafe raw bytes use closed markers.
- Semantic journal v2 records a closed phase/code/safe-detail outcome and
  bounded named metrics; raw provider and error text never enters metadata.
- `metadata.json` binds the run to the exact Go/module/VCS build identity.
- Architecture status v13 and its exchange/console projection distinguish
  requested, covered, and uncovered primary scope from covered supporting
  evidence. A symbol-only proposal is therefore reported as a closed quality
  rejection instead of an ambiguous aggregate `N/M` result.
- Directories use mode 0700, files use 0600.
