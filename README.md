# repomap

`repomap` is a tiny local-first repository orientation CLI for large unfamiliar codebases.

Point it at any local git repo and get:

- A project-level orientation (what is this repo?)
- Candidate runtime/event flows (gRPC Put, raft write path, watch stream, lease lifecycle)
- Step-by-step flow walkthroughs with files to read in order, tests to consult, and unknowns flagged

```bash
repomap ../etcd
```

That's the whole MVP.

## Quick start

```bash
go build ./cmd/repomap

# Online (needs DeepSeek API key)
export DEEPSEEK_API_KEY=sk-...
repomap ../etcd

# Offline (no API key — local facts and flow bundles only)
repomap ../etcd --offline

# JSON output for scripts
repomap ../etcd --json | jq '.explained_flows[0].flow_report.summary'

# Limit explained flows
repomap ../etcd --flows 2
```

Output goes to stdout (human-readable) or `--json` (machine). Debug artifacts land in `.repomap-runs/`.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | Print combined JSON report instead of text |
| `--offline` | false | Skip DeepSeek calls, build local bundles only |
| `--flows N` | 4 | Number of candidate flows to explain |
| `--no-debug` | false | Disable writing debug artifacts |
| `--debug-dir <dir>` | `.repomap-runs` | Directory for debug artifacts |
| `--dump-llm` | false | Dump LLM request/response to debug dir |

## How it works

1. **Local facts** — `git ls-files`, README, Go packages/edges/entrypoints from `go list`
2. **Compact LLM bundle** — bounded subset sent to DeepSeek (not the full repo)
3. **Orientation** — DeepSeek returns `candidate_flows`, `high_level_map`, `first_files_to_open`
4. **Flow explanation** — each candidate flow gets a focused bundle and a step-by-step walkthrough
5. **Validation** — all file paths in the output are checked against real git tracked files

## Development

```bash
make check                    # go test + go vet
make smoke                   # smoke test (no network, no API key)
make run ETCD_REPO=../etcd   # full pipeline
make run-offline ETCD_REPO=../etcd  # offline mode
make run-json ETCD_REPO=../etcd     # JSON output
```

## Project docs

- [docs/CORE_IDEA.md](docs/CORE_IDEA.md) — project vision and pipeline design
- [docs/INVESTIGATION_ENGINE.md](docs/INVESTIGATION_ENGINE.md) — progressive evidence workflow
- [docs/TECHNICAL_DEBT.md](docs/TECHNICAL_DEBT.md) — demonstrated implementation debt
- [docs/OPEN_QUESTIONS.md](docs/OPEN_QUESTIONS.md) — unresolved product and research decisions
- [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) — API integration rules
- [docs/GOLDEN_DEBUG_RUN.md](docs/GOLDEN_DEBUG_RUN.md) — reproducible debugging
- [AGENTS.md](AGENTS.md) — agent instructions

## Advanced (legacy compatibility)

```bash
repomap orient --repo ../etcd --snapshot-only    # local snapshot JSON
repomap orient --repo ../etcd --llm-bundle-only   # compact LLM bundle
repomap orient --repo ../etcd --explain-flows 4   # low-level flow control
```
