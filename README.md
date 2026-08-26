# repomap

`repomap` is an online, model-assisted repository orientation tool. It
extracts exact repository facts locally, sends bounded request-local catalogs
to an OpenAI-compatible model, and publishes a manifest, report JSON, and
report HTML. Every selected Go, Python, and JavaScript/TypeScript typed target
runs through its complete target-local ProgramIndex, dependency, semantic-cube,
and report-page path. A multi-target run seals a language-neutral program-page
portfolio and a repository-level runtime portfolio that bind those complete
pages without reducing a non-default target to a structural-only substitute.

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

A selected JavaScript/TypeScript target additionally requires Node.js on
`PATH` and an owner-prepared, repository-local TypeScript compiler in
`node_modules`. Prepare dependencies with the repository's normal package
manager before running repomap; repomap never installs packages.

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
`REPOMAP_LLM_MAX_TOKENS` (default `128000`). An explicitly unauthenticated
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

Target discovery is high-recall. By default one repository-wide portfolio must
retain a canonical file representative for every exact native target, may also
retain positively supported repository-guidance candidates, and chooses one
retained target as the default. Every restored typed target receives a complete
target-local report page; a mixed-language run publishes those pages through
the neutral program-page and runtime portfolios. `--target` bypasses the
portfolio choice and analyzes exactly one supported explicit target, while the
README classifier and active language scouts still run for downstream context.
An unselected language's compiler and page-local analysis do not run. For every
non-explicit discovered candidate set — even one eligible file — a fully
validated live or cached model selection is required. Unavailable providers,
transport failures, invalid responses, or incomplete target, activity,
dependency, or snapshot evidence end the run instead of publishing a locally
guessed or partial map.
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
