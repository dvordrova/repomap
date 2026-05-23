# repomap

`repomap` is a tiny local-first repository orientation CLI for large unfamiliar codebases.

It creates a compact local snapshot of a git repository, then (optionally) asks DeepSeek for a structured orientation report with likely entrypoints, first files to inspect, and candidate flows.

This is step 1 only: orientation before deeper flow analysis.

## What it does

- Works on a **local** git repository only
- Uses `git -C <repo> ls-files` for repository inventory
- Builds a snapshot including:
  - repo name
  - truncated README
  - small file tree
  - top-level directory stats
  - language hints
  - interesting files
  - Go hints (`go.mod`, module name, `cmd/**` entrypoints, important Go filenames)
- Skips secret/binary/noisy paths (`.env`, keys/certs, `.git`, `.github`, `vendor`, `node_modules`, `dist`, `build`, `coverage`, images, archives, binaries)

## Build

```bash
go build ./cmd/repomap
```

## Usage

```bash
repomap orient --repo /path/to/local/git/repo
```

Flags:

- `--snapshot-only` print local snapshot JSON only (no API call)
- `--out <file>` write output JSON to a file
- `--max-readme-bytes` (default `20000`)
- `--max-tree-lines` (default `400`)
- `--max-interesting-files` (default `200`)

Environment:

- `DEEPSEEK_API_KEY` (required unless `--snapshot-only`)
- `DEEPSEEK_MODEL` (optional, default `deepseek-chat`)

## Run on a local etcd clone

```bash
git clone https://github.com/etcd-io/etcd.git
export DEEPSEEK_API_KEY=...
go run ./cmd/repomap orient --repo ../etcd --snapshot-only
go run ./cmd/repomap orient --repo ../etcd | jq .
```