# Input-scoped freshness and Go package identity

## Status

This design is an approved correctness correction. It does not replace the
active architecture-grounding decision and does not add a new analysis stage.

The two corrections described here are independent:

1. repository/report freshness is scoped to captured analysis inputs;
2. Go package ownership and display are derived from exact module and package
   metadata.

A semantic-major path element such as `/v2` does not cause submodule dirtiness,
and submodule dirtiness does not affect Go import-path parsing.

## Current freshness behavior

`freshness.CaptureRepository` currently records:

- the canonical Git top-level path;
- `HEAD`;
- every non-ignored dirty or untracked root-worktree path reported by
  `git status --porcelain=v1 -z --untracked-files=all --ignored=no`;
- the content hash of every dirty regular file;
- the link-target hash of every dirty symlink;
- selected ignored Go build inputs reported by `git ls-files --others
  --ignored --exclude-standard -z`.

Selected ignored inputs are limited to Go/C-family source, `go.mod`, `go.sum`,
`go.work`, `go.work.sum`, and `vendor/modules.txt`. Other ignored files are not
read. In particular, ignored secrets are not included merely to prove that they
did not change.

Clean tracked file contents are represented indirectly by `HEAD`; they are not
hashed individually. `CaptureRepository` repeats a full capture until two
consecutive repository-state digests match, with a three-capture bound.

The default command captures this global state before orientation and again
after orientation. `report.ConfirmRunAuthority` requires the two complete
repository-state digests to be identical. Any unrelated change therefore
returns `repository changed during orientation` and prevents publication of an
authorized manifest. Architecture synthesis currently happens after that
comparison, so the comparison is both too broad and placed before the final
repository-dependent producer.

## Why dirty submodules fail

Git reports a dirty submodule at the gitlink path. That path is a directory in
the superproject worktree. `fingerprintDirtyFile` currently rejects dirty
directories with `submodule freshness is not supported yet`. The capture fails
before repomap can decide whether any submodule content was analyzed.

The current representation also conflates:

- a changed gitlink;
- a different submodule `HEAD`;
- tracked modifications inside a submodule;
- untracked files inside a submodule;
- an unavailable or uninitialized submodule.

## Current report authority

`run_manifest.json` version 2 binds:

- the exact `report.json` bytes and report format;
- the complete global repository-state digest;
- the canonical repository and analysis roots;
- exact openable paths;
- component, anchor, line, and symbol-listing permissions.

The report server verifies the report hash and authority projection before
enabling actions. Browser actions accept manifest-authorized opaque IDs rather
than arbitrary paths or symbols. Same-origin, loopback-host, content-type,
bounded-input, symlink, path-containment, and short-lived capability checks are
separate security controls and remain unchanged.

Report authorization and current freshness are currently coupled because live
symbol/source actions compare the complete current repository digest with the
manifest digest. An unrelated change invalidates focused analysis. Opening an
authorized file checks path authority but does not currently describe whether
the live source differs from captured evidence.

## Current input reads

Inputs are not read once into one immutable source snapshot. Today:

- Git file names are listed for snapshot structure;
- README and bounded source inputs are read independently;
- `go list` reads build-selected Go packages;
- surface and flow analyzers read source independently;
- repository-state capture rereads dirty files;
- focused browser analysis rereads current source;
- report generation and reopening reread saved artifacts.

Before/after global repository comparison is therefore the current coherence
guard. The correction replaces that global guard with explicit captured input
records and bounded reconciliation.

## Separated concepts

### Repository working-tree state

Informational complete Git state: root changes, untracked paths, ignored-input
policy, gitlinks, and bounded submodule status. It is not itself browser
authorization.

### Captured analysis inputs

Versioned, bounded records for exact files and material metadata used by the
run. Each file record contains a stable opaque ID, repository-relative path,
content digest, mode/kind, owning module/package when known, and consuming
stage identifiers. Non-file inputs include selected revision, build context,
Go module/workspace files, analyzer/prompt/policy versions, provider-bundle
digest, and included gitlink metadata when actually consumed.

### Report freshness

The relationship between captured input records and current authorized files:

| State | Meaning |
| --- | --- |
| `fresh` | Every captured analyzed input still matches. |
| `unrelated_changes` | Repository state differs, but captured analyzed inputs match. |
| `partially_stale` | One or more captured analyzed inputs changed after capture; the saved report remains coherent for its captured snapshot. |
| `mixed_snapshot` | A required input changed during bounded capture twice, so one coherent captured snapshot cannot be established. |
| `unavailable` | Scoped comparison could not be completed safely. |
| `legacy_unknown` | An older manifest has no scoped-input contract. |

Affected output is bounded to authorized repository-relative paths or opaque
input IDs. Ignored secret names and contents are not exposed.

### Report authorization

Authorization validates access to the exact saved report and its manifest
capabilities. A dirty or stale worktree does not revoke access to the saved
artifact. Live actions reconcile only the captured inputs required by that
action. Unknown analyzer input scope remains fail-closed by using the broader
captured run scope.

## Default and strict behavior

Default `repomap <repo>` is permissive:

- unrelated changes and excluded dirty submodules do not fail the run;
- a report is saved for the captured inputs;
- stale analyzed inputs produce `partially_stale` plus a refresh warning;
- a completed provider response is retained when its exact request bundle is
  unchanged;
- no provider retry occurs solely because an unrelated file changed.

`--strict-snapshot` is the single reproducibility option. It fails when a
captured analyzed input, included gitlink, or required build input differs at
final reconciliation, or when a mixed snapshot cannot be resolved. The
affected local capture is retried once. Completed provider work is not repeated
when the provider bundle digest is unchanged.

## Excluded submodules

Submodules are separate repository boundaries and are excluded from source
analysis by default. The superproject status records these independent fields:

- recorded gitlink;
- current submodule `HEAD` when Git reports it;
- gitlink/head mismatch;
- tracked worktree modifications;
- untracked worktree content;
- unavailable/uninitialized state;
- whether the submodule was included in analysis.

For an excluded submodule, repomap does not read internal source, ignored files,
or untracked files and does not recurse into nested submodules. Worktree dirt is
informational and has no effect on analyzed-input freshness. A gitlink change is
metadata and is only stale when that exact gitlink was a captured input;
strict mode may reject that case.

Git status collection uses NUL-delimited porcelain v2 with optional locks
disabled so submodule commit, tracked-modification, and untracked indicators
remain machine-readable and independent of user color/output configuration.

## Go module and package behavior today

`gofacts.Load` discovers tracked `go.mod` files and runs `go list -e -json
./...` in each module directory. It currently obtains module identity from the
first successful package's `Module.Path`; empty or broken modules fall back to
their filesystem directory. Only module summaries, entrypoints, and bounded
import edges are persisted. Exact ordinary package directories and ownership
are discarded.

Internal import-edge classification uses exact import-path set membership and
is prefix-safe. Later report code reconstructs a package path for a file by
combining a longest matching module directory with the file directory. The
browser computes labels from import strings. A root package in module
`github.com/caddyserver/caddy/v2` is therefore displayed as `v2`, and package
cards repeat full canonical paths. GitHub source links also incorrectly assume
that import suffixes are physical repository directories.

## Exact package identity

Module identity comes from `go list -m -json`, not package order or path
appearance. Relevant exact metadata is retained with external absolute paths
omitted:

- declared module path;
- validated repository-relative module root and `go.mod`;
- `Main` status;
- local replacement provenance;
- owning module ID.

Each local package retains separate fields for:

- canonical import path;
- package name;
- exact repository-relative package directory;
- exact module-relative directory;
- module path and owning module ID;
- locality and ownership provenance;
- human display path.

A package is repository-local only when its exact `go list` source directory is
inside an analyzed module and the authorized repository scope. Import-path
owner, organization, `/vN` elements, and edge presence do not establish
locality. The most specific exact module directory owns a package; path-element
boundaries prevent `foo` from owning `foobar`.

Workspace modules and local replacements inside the repository can be local
when their exact directories are authorized. External workspace/replacement
directories remain external and their absolute paths are not persisted. A
module inside an excluded submodule is not analyzed local source.

## Package display

Display labels are computed in Go and persisted rather than reconstructed by
JavaScript. For a local non-root package, the normal display is its exact
module-relative directory:

```text
github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile
-> caddyconfig/httpcaddyfile

corp.example/platform/v0/internal/worker
-> internal/worker
```

The complete module prefix is removed because exact ownership metadata says it
is the owning module, not because `/v0` or `/v2` matches a version regex. A
module-root package uses the package name, with the full canonical import path
remaining in the inspector, exact evidence, tooltip, and copy action.

Package edges remain relationship evidence only. They no longer determine
whether a package exists or owns an anchored file.

## Compatibility

New manifests and repository graphs are versioned. Older manifests without
captured-input scope reopen as `legacy_unknown`: report bytes and saved
capabilities are still verified, but no claim of exact current freshness is
made. Historical artifacts are never rewritten in place.

Older report graphs without exact package records remain renderable through a
clearly legacy inferred ownership path. New reports always write exact package
records and server-computed labels.

## Implemented compatibility and limits

The implemented manifest contract is version 3. Version 2 manifests continue
to verify their saved report bytes and capabilities, but expose
`legacy_unknown` freshness rather than treating the old global dirty digest as
input-scoped evidence.

The current captured file set includes every report-authorized evidence path,
all build-selected local package source files returned by `go list`, relevant
module/workspace files, and package ownership metadata. The manifest separately
records selected revision, model-bundle hash, report contract, architecture
contract, and input-policy version. A selected revision change is informational
unless one of those captured inputs changed.

Module discovery remains bounded to tracked module roots already visible to the
repository snapshot. `go.work` repositories with tracked local module roots are
supported, and each root is analyzed with `GOWORK=off` so an ambient parent
workspace cannot silently change identity. External workspace roots and
external replacement absolute paths are not persisted. Full recursive analysis
of included submodules remains deferred; submodules are excluded boundaries in
this increment.

The local capture retries are bounded at the repository-capture boundary and
return a typed mixed-snapshot error after repeated instability. A future
optimization may retry only an affected collector rather than the bounded
repository capture. Completed architecture-provider work is keyed by the exact
candidate bundle and is not repeated because an unrelated worktree path changed.

The browser currently offers existing authorized current-source navigation and
marks stale source explicitly. A targeted “refresh affected analysis” action is
deferred. Import paths are no longer converted into guessed GitHub tree URLs;
source links are omitted until a verified repository-relative URL is available.
