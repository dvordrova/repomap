# repomap

`repomap` turns a repository you have just been handed into one static HTML
page that answers the first-day questions: what this is, how to run it, where
the parts talk to each other and on which port, what runs code it was given,
what is dead, what is missing, and what the main flow looks like end to end.
Every
answer is anchored to an exact `path:line` you can open in one click, so any
claim on the page can be checked in seconds.

Everything on the page is one of three labeled things:

- **facts** — extracted from the code deterministically: entrypoints, HTTP
  routes and client calls with their method and path literals, the portals
  where one target calls another, environment keys, the places where the
  program runs code it was given such as `exec` and `subprocess`, manifest
  settings, TODO markers, unreachable files, and what the repository lacks
  (no tests, no CI, no Dockerfile, a stub README);
- **claims** — quoted from README files, docstrings, comments, and commit
  messages, always with their source and age, because they may be stale;
- **model** — the repository summary, the role of each target, the run recipe,
  and the main flow, written by the model and marked as such. Every model
  sentence cites facts by id; a row citing something that does not exist is
  rejected into `rejected.jsonl` with its raw output and the reason, never
  repaired.

Under the page, each selected Go, Python, and JavaScript/TypeScript target
builds one complete target-local ProgramIndex. The deterministic fact and
claim stages run over the indexes, and then the atlas reads every target as
tables: directories by depth, independent files with direct caller facts,
the key symbols of each file, the boundaries where the code touches the
outside, the parts a target is made of, the arrows between its boxes, the
portfolio of targets, and the joints between targets. Each row gets one line
or one closed choice from the model; membership, arrows, direction and joints
are the code's. The boxes are projected into one target-local
`groups-index.json`, which is what the page draws, and the model orientation
runs over that. `--no-model` makes the same walk with every cell on its
fallback line.

The supported product surface is deliberately small:

```text
repomap [repository] [flags]
repomap conf [repository]
repomap cache clear [--debug-dir DIR]
repomap replay --file REQUEST.json [--debug-dir DIR]
repomap read READING_INPUT.json [--through STAGE] [flags]
```

`read` exercises the same atlas stages on saved evidence without rescanning
source code or producing a report. It is the development loop for context,
prompts and intermediate results.

`repomap conf` creates `.repomap.conf` in the current directory if absent
and opens it for editing. Pass a repository directory to edit its settings
instead. An existing file, including comments, is preserved. Git is not required;
parent directories and the home directory are not searched.

The local file uses YAML. It supports an editor command and optional reading questions:

```yaml
editor: [code, --goto, '{{ .File }}:{{ .Line }}:{{ .Column }}']
questions:
  - How do I run this project?
  - Where is state stored and changed?
```

This runs `code --goto file:line:column`. Each template expands inside its own
argument, so paths with spaces work without additional quoting. `.File` is an
absolute path; `.Line` and `.Column` start at 1. For example, Vim can use
`editor: [vim, '+{{ .Line }}', '{{ .File }}']`. Commands run in the selected
repository directory. The same setting opens the configuration and source links
in served reports. Settings are read once per run and passed in memory to target analysis and serving;
editing the configuration takes effect on the next repomap launch. If the
editor cannot start, the error names the configuration file and that file stays
available to edit manually. Per-target build variants are still planned.
Questions share the repository graph and accumulated descriptions; each question
has its own retrieval, reading route, answer and cache entries. Repeated `--question`
flags add to this list, keeping order and dropping duplicate question texts.
Learn links to these questions below the map. Opening a question shows a short
model answer with its sources and any unresolved part. “Check this interpretation”
shows what the model relied on alongside original declarations or documentation
excerpts and exact source links. Deductions from names and signatures are welcome
when identifiable and easy to check. The detailed reading route stays under a
separate disclosure. The answer uses the evidence already
selected for that route. Questions are currently supplied by the user;
automatic adaptation of general learning questions is planned.

## Work from the evidence upward

A normal analysis saves `reading-input.json` before the first atlas call.
It contains the places graph and target metadata; copy this one file to
iterate without the repository checkout, compilers, or `report.json`.
Only the current input format is supported. Generate a new analysis when its
format changes; there are no readers for previous formats.

```bash
# Read through files. Earlier directory requests reuse the shared cache.
.bin/repomap read /path/to/run/reading-input.json --through files

# Try a prompt and smaller contexts on the same evidence.
.bin/repomap read /path/to/run/reading-input.json --through files \
  --prompt internal/atlas/lines/prompts/files.md \
  --window-rows 12 --input-bytes 32768 --output /tmp/reading-small

# Run every atlas stage, producing atlas.json without orientation or HTML.
.bin/repomap read /path/to/run/reading-input.json

# Find sources, order a reading route and answer from that evidence.
.bin/repomap read /path/to/run/reading-input.json \
  --question 'Where does this program store state?' --output /tmp/reading-state

# Iterate on retrieval alone, or edit only the route selector after retrieval.
.bin/repomap read /path/to/run/reading-input.json --through question \
  --question 'Where does this program store state?'
.bin/repomap read /path/to/run/reading-input.json --through route \
  --question 'Where does this program store state?' \
  --prompt internal/atlas/lines/prompts/route.md

# Revise only the answer, reusing retrieval and the selected route.
.bin/repomap read /path/to/run/reading-input.json --through answer \
  --question 'Where does this program store state?' \
  --prompt internal/atlas/lines/prompts/answer.md
```

Use the same `--debug-dir` as the original run to share its model cache.
`--no-cache` requests fresh model responses. Each reading writes a new
output directory and reports its path. `--through` accepts `directories`,
`files`, `symbols`, `operations`, `boundaries`, `zones`, `arrows`, `targets`, `joints`, `question`, `route`, or `answer`.
The prompt and budget overrides apply to that stage; budget overrides without
`--through` apply to all stages. A prompt override requires `--through`.

Inspect `tables.md`, or compare `tables/*.result.json`: the latter holds
normalized cells with source IDs, paths and lines, or an explicit rejection
reason. The matching `.prompt.ref.json`, `.request.ref.json` and `.response.ref.json`
link to the exact bytes in the shared `.llm-cache/payloads/` directory.
`.input.ref.json` holds the table input before the provider envelope is built. `reading-result.json` records completion,
stage counts and wall time. Stopping early produces no partial `atlas.json`
and no HTML. These artifacts are for development, not another reader UI.

`knowledge.json` records descriptions of internal directory, file, symbol and
boundary entities, their exact input evidence, model cells and dependencies on
earlier interpretations. Accepted independent rows are reused before batching:
changing the row or byte budget no longer reanalyses unchanged entities. A
changed prompt, model configuration or evidence actually supplied to the model
gets a new answer basis. Ownership changes and new parent interpretation IDs
only rebind the answer to the current entities; changing the parent's supplied
text still invalidates the dependent answer. `knowledge.json` v2 keeps these
current ownership and provenance bindings separately from answer reuse.
Missing rows still go to the model in batches;
this does not introduce one provider call per function. The entity cache stores
only a reference to a request and a row in its current response. It reads and
validates that row again on reuse, so replayed answers cannot leave a second
copy of the description stale. `--no-cache` bypasses answer reuse and index
updates; `cache clear` removes responses and entity references.

The record describes what the supplied declarations and extracted facts
support. It does not claim that a function body was read or that changing an
uninspected body invalidates a declaration-only description. Question-only
readings recall available descriptions with the same ordinary row builders,
without making description calls. They send these as labelled model hints
beside the original facts, with used knowledge IDs restored locally on stops.

`--question TEXT` runs candidate retrieval, route selection and an answer in
`read`; the ordinary command appends them to the normal atlas run. The output
`question-routes.json` v1 contains a list of v7 reading routes: selected stops, their original
evidence and internal subjects, locally restored source positions, and
connections with distinct call, declaration and inventory evidence.
Every declaration and boundary in the graph is partitioned into complete
chunks, including generated files. Unanswered chunks remain explicitly
unresolved. The pass sees names, signatures and author documentation, not
function bodies; it neither verifies implementation behavior nor turns these
file connections into an execution trace. The `guide` selects a short list of distinct
locations in reading order, preserving their original reasons and an open question. For many
candidates it compares bounded pools and then their selections, always carrying
the original evidence forward. Every candidate remains in the artifact, and
every round records coverage and unresolved pools. Editing the route prompt
does not invalidate unchanged retrieval. Six stops is a preference: additional
valid selections are kept in every round. Pools split by the input-byte budget,
without a fixed number of candidate sources. If the model keeps every source
and another comparison cannot fit, the report keeps separate reading orders;
it does not invent a global order or repeat the same request. The ordinary command publishes the guide in
the common HTML report with source links and unresolved gaps. `read` prints and
saves it without rendering HTML.

A table is split at complete row boundaries by row count and system + user
UTF-8 bytes (64 KiB by default). Bytes are a reproducible planning budget,
not a token estimate or a promise of model quality. Shared context and every
row survive splitting; an oversized single row is an error naming that row.
Do not ask another model to summarize the same oversized blob: change the
owning stage's evidence selection or split the question itself.

File descriptions use the directory's model line and deterministic facts
about direct callers. They do not inherit other file descriptions, so files
can be processed in parallel and a reworded file does not invalidate callers'
next descriptions. Check extracted evidence first, then whether model cells
point to the right code, then the downstream guide. Valid JSON alone is not
an assessment of usefulness.

The facts stage runs built-in sqlc and optional external commands through one
small interface: nodes and labeled links. Configure your command in
`.repomap.json`; it receives JSON on stdin and returns JSON on stdout.
See [Writing an extractor](docs/EXTRACTORS.md) for the complete contract and
a small Python example. Exact exchanges are saved in `extractions.json`;
normalized entities and relationships enter `facts.json` (version 2).
The same entities and relationships enter the places graph and question table,
with producer declarations distinguished from compiler calls. Saved graph and
reading input are version 2; previous inputs need a fresh analysis. The plugin
protocol stays version 1. This supplies reading evidence, not a generated change
recipe; the report layout is unchanged.

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

Optional settings are `REPOMAP_LLM_TIMEOUT` (default `10m`, the bound on one
provider attempt; an attempt that hits it is retried) and
`REPOMAP_LLM_MAX_TOKENS` (default `128000`). An explicitly unauthenticated
endpoint uses `REPOMAP_LLM_AUTH=none` and still requires
`REPOMAP_LLM_ENDPOINT`.

If any `REPOMAP_LLM_*` variable is present, that namespace is authoritative and
no `DEEPSEEK_*` value is inherited. The complete transport contract is in
[docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md).

For a custom endpoint, requests default to
`"chat_template_kwargs":{"enable_thinking":false}`. Qwen servers such as
vLLM and SGLang use this to turn off thinking, including on answer-table
requests. The actual endpoint host selects this behavior; setting it through
`DEEPSEEK_ENDPOINT` works too. Official `api.deepseek.com` keeps its native
thinking controls, with the final answer table's existing reasoning opt-in.

`REPOMAP_LLM_CHAT_TEMPLATE_KWARGS` can replace this object. When using the
legacy configuration family, use `DEEPSEEK_CHAT_TEMPLATE_KWARGS` instead.
An explicit `{}` omits the field for a server that rejects it; that leaves
thinking behavior to that server. This is a server extension, so an endpoint
that does not support it may return HTTP 400. No retry silently removes it.

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
target-local section; a mixed-language run publishes those sections through
the neutral program-page and target-outcome portfolios in one HTML report. Its
target picker jumps between sections of that report.
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
transport failures, invalid responses, or incomplete categorization, grouping,
matching, dependency, or snapshot evidence never produce a locally guessed or
partial map. A
target-local failure is recorded as not analyzed when another complete page
can host the repository report; shared selection, repository overview,
persistence, manifest, or bundle failures still end publication.
The current flags are:

```text
--target TARGET
--no-model
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
```

The ordinary Go call graph is complete for the selected target: `--depth 0`
and `--edges-limit 0` are the defaults and keep every reachable exact call and
edge. Positive values opt into narrower local analysis. There is no size at
which repomap warns, samples or stops: a large graph is processed completely.

Without `--no-serve`, repomap starts a loopback server. Report code links use
that server to open the files the page names in the configured editor (VS Code
by default). `--no-open`
keeps the browser closed, and `--port` selects a fixed port. In a multi-target
run, every target is a section of one common report. The server receives the
generated data in memory; reopening a saved run reads its common report and
manifest once.

With `--no-serve`, repomap writes one standalone HTML whose code links point to
the captured revision on GitHub or GitLab. The repository `origin` must identify
a supported host, or the matching `--github-url`/`--gitlab-url` must be supplied.
Invalid static-link configuration fails in preflight before analysis or model
requests. For a multi-target run, the standalone document is projected directly
from the completed in-memory results. Repository-wide data is written once in
the owner's `report.json`; target directories keep their own analysis artifacts.
These flags remain presentation configuration
and do not download or switch the analyzed checkout.

Report runs and model-response caches default to the OS user-cache directory.
Use `--debug-dir` to choose another root. `--no-cache` forces live provider
calls for that run; it does not disable run diagnostics. Clear persistent model
caches with:

```bash
.bin/repomap cache clear
.bin/repomap cache clear --debug-dir /path/to/repomap/runs
```

To repeat one saved provider request and update its cached answer:

```bash
.bin/repomap replay --file /path/to/.llm-cache/payloads/REQUEST_SHA.json \
  --debug-dir /path/to/repomap/runs
```

Use the request payload linked by `tables/*.request.ref.json` or a semantic
exchange's `request.file`. Replay always contacts the provider through the same
configured client. The exact saved model, messages, token limit, temperature,
thinking mode and response format are preserved; endpoint, authentication,
timeout and retries come from the client configuration. Assistant content goes
to stdout; shared request/response paths, timing and token usage go to stderr.
Replay checks the provider envelope and JSON. The owning stage validates its
own schema when it next uses that answer. A failed replay leaves the previous
accepted answer available. It generates no report and does not rewrite an old
run; the next reading uses the updated answer.

Requests and responses are stored once by content hash under
`.llm-cache/payloads/`. Run directories contain exchange metadata and relative
references, along with their normalized results and report snapshot. Cache
clear removes these payloads too: existing HTML and result snapshots remain,
but their raw-exchange links stop resolving. `--no-cache` still saves shared
payloads for diagnostics, without updating reusable answer pointers.

Repository input is trusted. repomap does not scan it for credentials, and it
does not redact what it writes: whatever a prompt or a response contains is
what lands in the run directory and the model cache under your user-cache
directory. The provider key itself is read from the environment and is never
part of a request body or a cache record, but a credential committed to the
repository can reach those files like any other repository text. Treat a run
directory as being as sensitive as the repository it came from.

## Development

The built binary on the ordinary online path is the acceptance authority:

```bash
make test
make vet
make build
.bin/repomap /path/to/a/real/repository --no-open
```

For a product check, look at the exit status, at the run directory —
`reduced-documentation.json`, the enriched ProgramIndex set, every
`dependency-catalog.json` and `groups-index.json`, `report.json`, `facts.json`,
`claims.json`, `orientation.json`, `rejected.jsonl`, `report.html` — and at the
`Time` stage the run prints last, which says where the minutes went. Then open
the page and answer the first-day questions from it alone; that dogfood read is
the real acceptance. In a multi-target run only the first successful owner run
holds the common `report.json`, manifest and physical `report.html`. Cache changes also need a second real run
`repomap replay`, and `repomap cache clear`.

The product constitution lives in [docs/CONSTITUTION.md](docs/CONSTITUTION.md)
and the current architecture in
[docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md).
`testdata/acceptance/python-tutorial-game` is the acceptance fixture: its `expected.json`
lists the facts that must be present with their anchors, and a focused test
rebuilds them from the sealed artifacts of a real run. Static prompt prose
lives in Markdown beside documentation reduction, the atlas tables and the
orientation and is embedded in the binary;
complete dynamic reservoirs, provider-sized request partitions, ref restoration,
and semantic validation remain in Go.
