# Decision 163: Explicit Architecture localization identity artifacts

## Status

Approved by the repository owner through the explicit instruction to continue
the localization night plan after Decision 162.

## Problem

Decision 162 proves the provider-free localization contract against one real
semantic shape, but only in memory. A following checkpoint needs a durable,
inspectable English identity fixture without silently turning that fixture
into a product cache or claiming that the whole report has a canonical
localization representation.

The draft night plan proposed whole-report-shaped names:

```text
semantic-report.canonical.json
localization/input.v1.json
localization/en.projection.json
```

That scope is not yet proven. The current adapter covers only allowlisted prose
in a validated Architecture Canvas. Reusing whole-report names would overstate
both the semantic coverage and the artifact's authority.

## Decision

Add one explicit, provider-free developer operation:

```sh
make localization-check RUN=<run-dir>
```

The Make target continues to run the focused localization tests and, only when
`RUN` is supplied, invokes:

```sh
repomap dev localization-check <run-dir>
```

The command may write exactly these three files:

```text
<run-dir>/localization/architecture.canonical.v1.json
<run-dir>/localization/architecture.en.input.v1.json
<run-dir>/localization/architecture.en.projection.v1.json
```

These are Architecture-only English identity artifacts. They are
non-consumable developer evidence: ordinary generation, report assembly,
manifest verification, HTTP handlers, and browser code must not read them or
use them as cache hits.

### Eligibility

Materialization must fail closed unless all of the following are true:

- `architecture_synthesis.json` is the current version-3 synthesis record;
- the record contains explicit `output_language=en`;
- its result is non-fallback and has validation outcome `accepted` or
  `accepted_with_normalization`;
- accepted results use validated-model source metadata and no normalization;
- accepted-with-normalization results use normalized-model source metadata and
  a non-empty normalization record;
- the matching version-2 Architecture synthesis status records a successful or
  cached, parsed, accepted, non-fallback result with consistent source, level,
  and normalization metadata;
- the status loaded into the current report matches the saved status exactly;
- the current report contains a non-fallback Architecture Canvas;
- rebuilding the Architecture input from the current saved facts, replaying
  the saved synthesis record, and projecting a new Canvas reproduces the
  current Canvas byte-for-byte;
- the generated English identity projection replays without diagnostics or
  fallback and preserves that Canvas byte-for-byte.

Historical records without explicit language, Russian synthesis, rejected or
fallback synthesis, stale status, stale facts, mismatched metadata, or an
incomplete identity set are ineligible. No best-effort artifact is written.

### Persistence boundary

The writer must:

- use only the fixed directory and filenames above;
- confine all writes beneath the supplied run directory through a rooted
  filesystem handle;
- reject symlinked or non-directory `localization` paths and symlinked or
  non-regular artifact leaves;
- create the localization directory with mode `0700` and artifact files with
  mode `0600`;
- validate and secret-scan all three bounded payloads before the first write;
- write temporary files, sync them, install the complete set, sync the
  directory, and remove a partially installed set if installation fails;
- accept an already complete byte-identical set without rewriting it;
- refuse an incomplete or byte-conflicting existing set rather than overwrite
  it.

Each saved identity payload remains bounded to 1 MiB. The saved synthesis
status and record inputs are bounded before decoding to 64 KiB and 512 KiB,
respectively.

## Explicitly not authorized

- automatic writing during an ordinary `repomap` run or report render;
- a provider call, Russian projection, translation, or localization cache;
- treating these files as a canonical representation of Study, Guided Tour,
  mechanisms, surfaces, orientation, warnings, static UI copy, or the complete
  report;
- changing report JSON, report HTML, the run manifest, HTTP routes, source
  authority, freshness, or browser UI;
- adding these files to the manifest or making them load-bearing for any
  product path;
- changing the existing `--lang` behavior or semantic request/cache identity.

## Acceptance

- both accepted and accepted-with-normalization English fixtures produce the
  exact three-file set;
- repeated materialization is byte-stable;
- the persisted English identity replay preserves Architecture Canvas bytes;
- current v3 synthesis, matching v2 status, current-facts replay, language,
  fallback, and metadata mismatches fail closed;
- symlink, permission, conflict, partial-install, byte-bound, and secret-scan
  behavior are covered provider-free;
- unrelated run artifacts remain byte-identical;
- focused tests and the full repository check pass without a provider or
  external repository.

## Migration and rollback

This decision intentionally narrows and departs from the draft whole-report
artifact names. A future decision may introduce a complete semantic canonical
artifact only after every included semantic owner is explicitly adapted and
the artifacts are bound to verified run state.

Rolling back removes the explicit developer command, the three-file writer,
its tests, and this decision. No ordinary run, saved report, manifest, HTTP
contract, or UI needs migration because none consumes these artifacts.
