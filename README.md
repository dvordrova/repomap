# repomap

`repomap` is a local-first Go repository investigation CLI. It builds bounded,
inspectable facts locally, asks an OpenAI-compatible model for useful directions,
and keeps model hypotheses separate from source, test, and runtime evidence.

The current product path orients you in an unfamiliar repository and can expand
selected runtime/event directions. Source-grounded symbol investigation,
content-addressed resumable memory, and offline quality replay exist as
connected slices. The active product goal is a handoff-ready onboarding pass
for an engineer evaluating a Go project they already know; natural-language
feature work follows it.

## Build

```bash
go build -o ./repomap ./cmd/repomap
./repomap --help
```

repomap requires Go 1.24 or newer. Go facts use the installed toolchain, the
target repository's build files, and the user's normal Go environment (including
an internal `GOPROXY`). `go list -e` keeps partial facts when some packages are
unavailable.

## Configure a model

For the current DeepSeek reference path, only the key is required. The default
model is `deepseek-v4-flash`:

```bash
export DEEPSEEK_API_KEY=...
./repomap doctor llm --check
./repomap /path/to/repo
```

The normal human run shows local-context and outbound-request byte counts,
writes a report under the OS user cache, and opens that report in the browser.
Run `./repomap` with no repository argument to analyse the current directory;
use `--no-open` when the browser handoff is not wanted.

For a company or other compatible model, use a full OpenAI-compatible
`chat/completions` URL:

```bash
export REPOMAP_LLM_ENDPOINT=https://llm.company.example/v1/chat/completions
export REPOMAP_LLM_MODEL=company-code-model
export REPOMAP_LLM_API_KEY=...
export REPOMAP_LLM_AUTH=bearer
export REPOMAP_LLM_TIMEOUT=90s
```

For an explicitly unauthenticated local endpoint, set
`REPOMAP_LLM_AUTH=none`. No-auth always requires an explicit endpoint; it can
never fall back to the public DeepSeek URL. Existing `DEEPSEEK_*` variables
remain compatibility aliases.

The two namespaces are never mixed. Once any `REPOMAP_LLM_*` variable is used,
`REPOMAP_LLM_ENDPOINT` is required and stale `DEEPSEEK_*` credentials cannot be
inherited accidentally.

The current compatibility contract is intentionally small: OpenAI-style
`chat/completions` plus `response_format: {"type":"json_object"}`. Run the
doctor against a company endpoint before sending repository facts.

repomap does not load `.env` from the repository being analysed.

Verify configuration without repository content. `--check` adds one tiny
synthetic JSON request:

```bash
./repomap doctor llm
./repomap doctor llm --check
```

## Inspect before sending

See the compact semantic bundle or the exact provider request body without an
API call:

```bash
./repomap orient --repo ../etcd --llm-bundle-only > /tmp/etcd-bundle.json
./repomap ../etcd --preview-request > /tmp/etcd-request.json
wc -c /tmp/etcd-request.json
```

The preview is the exact JSON body, so its file size is the outbound body size.
The request is bounded; repomap never sends the raw file tree, full import graph,
or full repository contents. Obvious credential markers fail closed before
remote use.

## Explore a repository

The default performs one orientation request, preserves validated candidate
directions in the browser report, and stops without expanding all of them:

```bash
./repomap
./repomap ../etcd
```

Expand the top direction when you want more detail:

```bash
./repomap ../etcd --flows 1
./repomap ../etcd --flows 1 --json | jq .
```

Run local extraction without model calls:

```bash
./repomap ../etcd --offline
```

`--offline` is a model/privacy boundary, not an air-gap switch: the Go tool may
use the module proxy or toolchain source already configured by the engineer.

Debug artifacts default to the OS user-cache directory, not the analysed
repository. Use `--no-debug` to retain nothing or `--debug-dir` to choose a
trusted location.

## What can be trusted

- tracked-file survey, Go package facts, and bounded source cards are local;
- structured model path fields and detectable path-like evidence mentions are
  checked against the exact context allowlist before presentation; remaining
  free-form prose is still model interpretation;
- source-supported claims cite exact source evidence;
- test references are navigation hints, not proof of coverage or assertions;
- runtime behavior remains unknown until separately observed;
- saved quality tasks report grounding, omissions, contracts, bytes, and latency
  separately.
- saved investigations keep local facts, model claims, and session state in
  separate hash-verified documents; unchanged sessions resume without another
  model call, while repository/tool/prompt changes invalidate the applicable
  layer before an action can execute.

## Company engineer trial

[docs/ENGINEER_TRIAL.md](docs/ENGINEER_TRIAL.md) defines the external evaluation:

1. onboarding calibration on a project the engineer already knows;
2. exploration of an unfamiliar project;
3. a held-out feature counterfactual on the commit before a real change.

The three goals will be policies over one investigation engine, not independent
prompt pipelines. The active acceptance target is the first item: one command,
one browser report, named direction selection, and one evidence-backed
drill-down that a knowledgeable friend can critique. The trial document
distinguishes that target from what already works and from later feature work.

## Development

```bash
./scripts/check.sh
./scripts/smoke.sh
./scripts/etcd_check.sh ../etcd
./scripts/quality_check.sh
# Calibrate the atomic generic namespace against the DeepSeek reference:
./scripts/with_deepseek_generic_config.sh go run ./cmd/repomap doctor llm --check
# Before a new live baseline, after choosing a linked source-capable symbol:
./scripts/quality_preflight.sh LABEL PATH_TO_REPO EXACT_SYMBOL
```

Contributor architecture and current execution order:

- [docs/CORE_IDEA.md](docs/CORE_IDEA.md)
- [docs/MILESTONES.md](docs/MILESTONES.md)
- [docs/SYSTEM_MAP.md](docs/SYSTEM_MAP.md)
- [docs/INVESTIGATION_ENGINE.md](docs/INVESTIGATION_ENGINE.md)
- [docs/TECHNICAL_DEBT.md](docs/TECHNICAL_DEBT.md)
- [AGENTS.md](AGENTS.md)
