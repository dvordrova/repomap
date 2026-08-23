# repomap

`repomap` is an online, model-assisted repository orientation tool. It
extracts exact repository facts locally, sends bounded request-local catalogs
to an OpenAI-compatible model, and publishes a manifest, report JSON, and
report HTML. Go currently has the complete semantic cube-map path. Python has
target discovery, an AST-backed language-neutral program index, an exact
dependency catalog, potential-integration and concrete-operation classifiers,
the shared core-responsibility map, and model-selected activity entrypoints
restored to exact program objects. For every selected integration operation,
Python also publishes a deterministic route from a selected activity to the
exact caller when the retained program graph supports one, preserving
`exact`, `possible`, `frontier`, and `unconnected` as distinct claims.

The supported product surface is deliberately small:

```text
repomap [repository] [flags]
repomap cache clear [--debug-dir DIR]
```

There are no offline, replay, investigate, doctor, dev, experiment, or separate
serve commands.

## Build

Go 1.26 or newer is required.

Python repositories additionally require Python 3.10 or newer on `PATH`. The
adapter requires the runtime's exact standard-library module catalog and does
not guess when that authority is unavailable.

```bash
make build
.bin/repomap --help
```

`make build` writes the owner-facing binary to `.bin/repomap`.

## Configure the model provider

For the default DeepSeek endpoint:

```bash
export DEEPSEEK_API_KEY=...
.bin/repomap /path/to/repository
```

For another OpenAI-compatible `chat/completions` endpoint:

```bash
export REPOMAP_LLM_ENDPOINT=https://llm.example/v1/chat/completions
export REPOMAP_LLM_MODEL=company-code-model
export REPOMAP_LLM_API_KEY=...
export REPOMAP_LLM_AUTH=bearer
```

Optional settings are `REPOMAP_LLM_TIMEOUT` (default `10m`) and
`REPOMAP_LLM_MAX_TOKENS` (default `64000`). An explicitly unauthenticated
endpoint uses `REPOMAP_LLM_AUTH=none` and still requires
`REPOMAP_LLM_ENDPOINT`.

If any `REPOMAP_LLM_*` variable is present, that namespace is authoritative and
no `DEEPSEEK_*` value is inherited. The complete transport contract is in
[docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md).

## Run

```bash
.bin/repomap
.bin/repomap ../etcd
```

Target discovery is high-recall. By default the model positively selects the
supported targets to analyze and chooses their default. Go target pages receive
the complete Go semantic path. A Python-only root receives complete Python
semantics for its default view; additional selected Python views are published
as explicitly structural. A mixed Go/Python positive selection currently ends
with corrective `--target` or project-root guidance because the report cannot
yet publish complete cross-language target pages. `--target` bypasses the
portfolio choice and analyzes exactly one supported explicit target, while the
parallel README classifier still runs for downstream context. For every
discovered candidate set — even one eligible file — a fully validated live or
cached model selection is required. Unavailable providers, transport failures,
invalid responses, or incomplete target, activity, dependency, or snapshot
evidence end the run instead of publishing a locally guessed or partial map.
The current flags are:

```text
--target TARGET
--force-platform GOOS/GOARCH
--depth N
--edges-limit N
--github-url URL
--gitlab-url URL
--no-open
--no-serve
--port PORT
--debug-dir DIR
--no-cache
--scan-secrets
```

Without `--no-serve`, repomap starts a loopback server. Report code links use
that server to open manifest-authorized local files in VS Code. `--no-open`
keeps the browser closed, and `--port` selects a fixed port.

With `--no-serve`, repomap writes standalone HTML whose code links point to the
captured revision on GitHub or GitLab. The repository `origin` must identify a
supported host, or the matching `--github-url`/`--gitlab-url` must be supplied.
Invalid static-link configuration fails in preflight before analysis or model
requests.

Report runs and model-response caches default to the OS user-cache directory.
Use `--debug-dir` to choose another root. `--no-cache` forces live provider
calls for that run; it does not disable run diagnostics. Clear persistent model
caches with:

```bash
.bin/repomap cache clear
.bin/repomap cache clear --debug-dir /path/to/repomap/runs
```

Repository input is trusted by default. `--scan-secrets` enables heuristic
credential scanning. Provider API keys and Authorization headers are never
written to report, cache, or debug artifacts.

## Development

The built binary on the ordinary online path is the acceptance authority:

```bash
make test
make vet
make build
.bin/repomap /path/to/a/real/repository --no-open
```

For a successful product check, verify the process exit status and the emitted
manifest, sealed ProgramIndex set, semantic cube artifact, `report.json`, and
`report.html`. Cache changes also require a second real run and `repomap cache
clear`.

The current architecture and product decisions live in
[docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md). Static prompt prose
lives in Markdown beside each model-assisted domain cube and is embedded in the
binary; dynamic bounded catalogs and semantic validation remain in Go.
