# repomap

`repomap` is an online, model-assisted repository orientation tool. It
extracts exact repository facts locally, sends bounded request-local catalogs
to an OpenAI-compatible model, and publishes manifest-bound report data plus
one user-facing report HTML. Every selected Go, Python, and
JavaScript/TypeScript typed target runs through its complete target-local
ProgramIndex, dependency, semantic-cube, and report-page path. A multi-target
run seals a language-neutral program-page portfolio and a repository-level
runtime portfolio that bind those complete pages without reducing a non-default
target to a structural-only substitute.
Their validated page-local data remains separate, while one deterministic
successful target run owns the publication's only physical `report.html`.

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

`[repo]` is a local checkout (the current directory when omitted). The
`--github-url` and `--gitlab-url` flags only authorize source links in a
standalone report; they never clone or select the repository to analyze. When
`[repo]` is omitted, a supplied source URL must match the current checkout's
origin or the run fails in preflight with corrective guidance.

Target discovery is high-recall. By default one repository-wide portfolio must
retain a canonical file representative for every exact native target, may also
retain positively supported repository-guidance candidates, and chooses one
retained target as the default. Every restored typed target receives a complete
target-local report page; a mixed-language run publishes those pages through
the neutral program-page and runtime portfolios in one HTML report. Its target
picker replaces the old workflow of opening or merging one HTML file per target.
If one selected target cannot complete its own preparation, typed analysis,
semantic validation, or page validation, the other targets continue. The
report keeps that target visible as a red, non-clickable `Not analyzed` row and
accounts for it separately in the repository overview; it never presents a
partial target page as analyzed. A target-local JavaScript/TypeScript compiler
problem likewise affects only that package. If no selected target produces a
validated page, there is no page from which to publish a report and the run
still fails with diagnostics.
`--target` bypasses the
portfolio choice and analyzes exactly one supported explicit target, while the
README classifier and active language scouts still run for downstream context.
An exact repository-relative Python anchor path is accepted only when it names
one native target; if the file anchors several launch modes, the error lists
the matching sealed selectors and requires an explicit choice.
An unselected language's compiler and page-local analysis do not run. For every
non-explicit discovered candidate set — even one eligible file — a fully
validated live or cached model selection is required. Unavailable providers,
transport failures, invalid responses, or incomplete activity, dependency, or
snapshot evidence never produce a locally guessed or partial map. A
target-local failure is recorded as not analyzed when another complete page
can host the repository report; shared selection, repository overview,
persistence, manifest, or bundle failures still end publication.
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
keeps the browser closed, and `--port` selects a fixed port. In a multi-target
run, sibling target URLs are served virtually from their validated page data;
they do not require sibling `report.html` files on disk.

With `--no-serve`, repomap writes one standalone HTML whose code links point to
the captured revision on GitHub or GitLab. The repository `origin` must identify
a supported host, or the matching `--github-url`/`--gitlab-url` must be supplied.
Invalid static-link configuration fails in preflight before analysis or model
requests. For a multi-target run, the standalone document is projected directly
from every manifest-verified `report.json` and page-local artifact; repomap does
not merge child HTML documents. These flags remain presentation configuration
and do not download or switch the analyzed checkout.

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
`report.html`. For a multi-target run, verify every backing manifest and
`report.json`, and verify that only the default run contains the physical
`report.html`. When the selected default was not analyzed, verify that only the
first successful owner run contains it instead. Cache changes also require a
second real run and `repomap cache
clear`.

The current architecture and product decisions live in
[docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md). Static prompt prose
lives in Markdown beside each model-assisted domain cube and is embedded in the
binary; dynamic bounded catalogs and semantic validation remain in Go.
