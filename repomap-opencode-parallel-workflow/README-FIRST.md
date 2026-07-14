# Repomap OpenCode workflow — three commands, parallel evidence

Normal user interface:

```text
/go
/ship
/next
```

`/go` now orchestrates bounded parallel subagents. You still type one command.

## Internal `/go` shape

```text
missing/stale repository oracles ─┐
                                  ├─ parallel, max 3
                                  ┘
affected fixture runs ────────────┐
                                  ├─ parallel, isolated outputs
                                  ┘
fixture auditors ─────────────────┐
semantic contract audit ──────────┼─ parallel
performance audit ────────────────┘
                                  ↓
cross-fixture synthesis
                                  ↓
one feature-builder edits code
                                  ↓
affected reruns and audits
                                  ↓
browser reviews per fixture ────── parallel
                                  ↓
final acceptance reviewer
```

Only the feature-builder edits production code. Parallel agents gather facts, run
fixtures, compare reports to independent truth, and review browser journeys.

## Install

From the repomap repository root:

```bash
./repomap-opencode-parallel-workflow/install.sh
```

Restart OpenCode, then:

```text
/go
```

## Paths used by fixture agents

Workflow agents explicitly allow external access to:

```text
~/Library/Caches/repomap/**
~/git/**
```

This prevents the workflow from repeatedly asking about those external directories.

To apply the same external-directory allowlist to all OpenCode agents globally, run:

```bash
./repomap-opencode-parallel-workflow/install-global-permissions.sh
```

See `GLOBAL-PERMISSIONS.md` before doing that.

## Parallel safety

- maximum three fixture-scoped tasks at once;
- one production-code writer;
- one Sol-high synthesizer/reviewer at a time;
- one Sol-xhigh blocker diagnosis at a time;
- saved provider replay by default;
- no concurrent live provider exploration during ordinary iteration;
- unique cache directories and report-server ports.

## Models

The installer resolves explicit OpenAI model IDs available through your ChatGPT
Plus/Pro OpenAI login. Agents do not inherit the currently selected TUI model.
