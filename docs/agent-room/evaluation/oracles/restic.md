# Restic independent source oracle

## Scope and method

This is a pre-report source oracle for the Restic regression fixture. It was
prepared from the source checkout only; no generated repomap report was read.
Statements under **Exact facts** are directly supported by the cited checkout
paths and symbols. Statements under **Informed expectations** are architectural
interpretations and are not claims of observed execution.

## Fixture identity and state

### Exact facts

| Field | Value |
| --- | --- |
| Fixture name | `restic` |
| Absolute source path | `/Users/dvordrova/git/restic` |
| Required and checked-out revision | `987caba4089fc4345bb201e62c5a2ba96b168049` |
| Branch | `master` |
| Dirty state | clean: `git status --short` produced no entries; neither index nor worktree diff was present |
| Module | `github.com/restic/restic` (`go.mod:1`) |
| Go language declaration | `go 1.25.8`, toolchain `go1.25.10` (`go.mod:3-5`) |
| Generated-source/build-output state | no path matching `**/*generated*` was found in the checkout during this inspection; the clean Git state supplies no evidence of generated or built files needing regeneration. This is not proof that no tool can generate artifacts. |

### Exact executable inventory

* **Primary executable:** `restic`, package `main` at
  `cmd/restic/main.go:161` (`main`). The release helper builds this package with
  `go build ... ./cmd/restic` at
  `helpers/build-release-binaries/main.go:116-121`.
* **Secondary executables (release/developer tooling, not normal backup
  surfaces):** package `main` at
  `helpers/build-release-binaries/main.go` and package `main` at
  `helpers/prepare-release/main.go`. The former cross-builds/compresses release
  binaries (`build` at lines 97-141); the latter performs release preparation
  and can edit release files (`replace` at lines 73-86).

## User-facing entry surfaces

### Exact facts

* The CLI root is `newRootCommand` in `cmd/restic/main.go:36-113`, a Cobra
  command whose `Use` is `restic` and whose stated purpose is "Backup and
  restore files" (`main.go:37-45`). `main` executes it with a global context at
  `cmd/restic/main.go:180-196`.
* The root registers the normal command families at
  `cmd/restic/main.go:76-105`: `backup`, `cache`, `cat`, `check`, `copy`,
  `diff`, `dump`, `features`, `find`, `forget`, `generate`, `init`, `key`,
  `list`, `ls`, `migrate`, `options`, `prune`, `rebuild-index`, `recover`,
  `repair`, `restore`, `rewrite`, `snapshots`, `stats`, `tag`, `unlock`, and
  `version`. `debug`, `mount`, and `self-update` are registered separately at
  `main.go:107-110`.
* The key task-level surfaces for onboarding are:
  * `restic init` → `newInitCommand` / `runInit`
    (`cmd/restic/cmd_init.go:20-43`, `:58-109`);
  * `restic backup [FILE/DIR] ...` → `newBackupCommand` / `runBackup`
    (`cmd/restic/cmd_backup.go:35-67`, `:498-719`);
  * `restic restore snapshotID --target DIR` → `newRestoreCommand` /
    `runRestore` (`cmd/restic/cmd_restore.go:23-60`, `:99-260`);
  * `restic check` → `newCheckCommand` / `runCheck`
    (`cmd/restic/cmd_check.go:27-72`, `:229-250` onward).
* Availability is build-selected for two root children: `mount` is compiled on
  `darwin || freebsd || linux` (`cmd/restic/cmd_mount.go:1-3`) and
  `self-update` requires the `selfupdate` tag
  (`cmd/restic/cmd_self_update.go:1-3`). On this Darwin host, the source
  selection admits `mount`; actual FUSE availability and successful mounting
  are dynamic facts.
* `internal/backend/all.Backends` registers Azure, B2, GCS, local, rclone,
  REST, S3, SFTP, and Swift factories
  (`internal/backend/all/all.go:3-27`), which the CLI supplies in
  `global.Options` (`cmd/restic/main.go:180-182`).

### Important internal runtime activities — not entry surfaces

Do **not** promote these to separate user entry surfaces:

* root setup and dispatch: `main`, `tweakGoGC`, feature-flag application,
  terminal setup, error-to-exit-code handling
  (`cmd/restic/main.go:127-243`);
* CLI pre-run password decision: `global.Options.PreRun` is invoked through
  `PersistentPreRunE` (`cmd/restic/main.go:50-56`);
* repository/backend opening, decryption, cache setup, and locking:
  `global.OpenRepository` (`internal/global/global.go:299-337`),
  `decryptRepository` (`:370-405`), `setupCache` (`:420-460`), and
  `internalOpenWithLocked` (`cmd/restic/lock.go:11-32`);
* backup scanning, filtering, progress reporting, worker coordination, and
  archiving inside `runBackup` (`cmd/restic/cmd_backup.go:630-718`);
* repository blob/index/pack mechanics represented by `repository.Repository`
  (`internal/repository/repository.go:31-57`) and the storage `backend.Backend`
  interface (`internal/backend/backend.go:12-90`);
* restoration tree traversal, overwrite decisions, and file restoration inside
  `restorer.Restorer` / `RestoreTo`
  (`internal/restorer/restorer.go:20-36`, `:343-444`);
* checker traversal and integrity reporting inside `checker.Checker` /
  `Checker.Structure` (`internal/checker/checker.go:15-35`, `:147-202`).

## New-engineer flows

### Flow 1: initialize an encrypted repository

**Exact source path:** Cobra `restic init` → `runInit`
(`cmd/restic/cmd_init.go:20-43`, `:58-109`) →
`global.CreateRepository` (`internal/global/global.go:463-496`) → backend
selection/opening (`internal/global/global.go:498-521`) → `Repository.Init`
(invoked at `internal/global/global.go:490`).

**Exact facts:** `runInit` selects an allowed repository format version,
optionally reads chunker parameters, and calls `CreateRepository`
(`cmd/restic/cmd_init.go:65-87`). `CreateRepository` reads a repository
location, reads a new password twice, opens a backend in create mode, makes a
repository instance, then initializes it (`internal/global/global.go:468-495`).

### Flow 2: archive filesystem content into a snapshot

**Exact source path:** Cobra `restic backup` → `runBackup`
(`cmd/restic/cmd_backup.go:498-719`) → append lock / `global.OpenRepository`
(`cmd/restic/lock.go:11-44`) → `Repository.LoadIndex`
(`cmd/restic/cmd_backup.go:542-580`) → `archiver.New` / `Archiver.Snapshot`
(`cmd/restic/cmd_backup.go:655-698`; `internal/archiver/archiver.go:177-193`,
`:883`) → snapshot creation/saving through the repository.

**Exact facts:** the command resolves targets and optional parent snapshot,
loads the repository index, optionally scans in an errgroup, configures the
archiver, and invokes `Snapshot` (`cmd/restic/cmd_backup.go:519-718`). The
archiver delegates directory traversal work to file/tree worker machinery
(`internal/archiver/archiver.go:83-117`).

### Flow 3: restore a selected snapshot to a target directory

**Exact source path:** Cobra `restic restore` → `runRestore`
(`cmd/restic/cmd_restore.go:99-260`) → read lock / repository opening
(`cmd/restic/cmd_restore.go:149-153`; `cmd/restic/lock.go:34-39`) → snapshot
selection and index load (`cmd/restic/cmd_restore.go:155-168`) →
`restorer.NewRestorer` (`cmd/restic/cmd_restore.go:170-178`) → `RestoreTo`
(`internal/restorer/restorer.go:343-444`).

**Exact facts:** `runRestore` validates target and filter combinations, finds a
snapshot, loads the index, creates a restorer, and calls `RestoreTo`
(`cmd/restic/cmd_restore.go:122-249`). `RestoreTo` traverses the snapshot,
creates directories, batches file restoration, then performs a second metadata
pass (`internal/restorer/restorer.go:370-443`).

### Flow 4: verify repository structure and optionally data

**Exact source path:** Cobra `restic check` → `runCheck`
(`cmd/restic/cmd_check.go:27-72`, `:229` onward) → exclusive lock
(`cmd/restic/cmd_check.go:242-249`) → `checker.New`
(`internal/checker/checker.go:42-53`) → `Checker.LoadSnapshots` /
`Checker.Structure` (`internal/checker/checker.go:55-61`, `:147-202`) →
repository checker and backend reads.

**Exact facts:** the command documents structural/integrity checks by default
and optional data reads (`cmd/restic/cmd_check.go:31-42`). `Structure` streams
trees, records referenced blobs, and reports errors through a channel
(`internal/checker/checker.go:147-202`).

## Conceptual architecture responsibilities

### Informed expectations (grounded by the cited facts)

| Responsibility | Likely owning areas |
| --- | --- |
| Command parsing, global options, terminal/JSON output, and process exit policy | `cmd/restic`, especially `main.go`, command files, and `lock.go` |
| Repository lifecycle, credentials, location parsing, backend wrapping, and cache | `internal/global`; `internal/backend/all`; `internal/backend/*` |
| Encrypted, compressed repository objects, indexes, packs, locks, and blob I/O | `internal/repository`, `internal/repository/crypto`, `internal/repository/index`, `internal/repository/pack` |
| Snapshot data model, selection, grouping, and tree data | `internal/data` |
| Filesystem capture, change detection, chunking, and concurrent save work | `internal/archiver`, `internal/fs`, `internal/filter` |
| Reconstructing a snapshot safely onto a filesystem | `internal/restorer` |
| Structural/data integrity validation | `internal/checker` and repository checking code |

## Analysis frontiers

### Exact static frontiers

* Build tags change the command set (`cmd/restic/cmd_mount.go:1-3`,
  `cmd/restic/cmd_self_update.go:1-3`) and platform-specific filesystem code
  changes behavior.
* The `backend.Backend` interface is the abstraction boundary for local and
  remote persistence (`internal/backend/backend.go:12-90`); all registered
  factory implementations cannot be understood as one concrete transport.
* The archiver explicitly uses worker goroutines and callbacks
  (`internal/archiver/archiver.go:83-117`); runtime scheduling/order is not
  recoverable as a fixed static trace.
* Restore is a two-pass operation with file batching (`internal/restorer/restorer.go:370-443`),
  so simple symbol order does not establish file-level execution order.

### Dynamic frontiers

* Repository location, backend credentials, password source, cache state,
  locks, remote network service behavior, and remote object contents are
  runtime inputs to `OpenRepository` and backend opening
  (`internal/global/global.go:299-337`, `:498-521`).
* Filesystem permissions, source mutations during backup, filters, host/time,
  OS-specific metadata, FUSE support, and cancellation alter flow outcomes.
* `self-update`, when built, downloads from GitHub and writes the running
  executable by default (`cmd/restic/cmd_self_update.go:29-35`, `:62-89`);
  network and local write results must be observed, not inferred.

## Claims that would be misleading

* “Every function named `new*Command` is a top-level command.” Some construct
  nested command families such as `key`, `repair`, and `debug`; root ownership
  is established by the registrations in `cmd/restic/main.go:76-110`.
* “`mount` and `self-update` are always available.” Their build constraints
  prove otherwise.
* “A command’s presence proves it ran, contacted a backend, or changed a
  repository.” Registration and static call edges do not prove runtime events.
* “All backends are HTTP services.” The registry includes local and rclone as
  well as cloud/network backends (`internal/backend/all/all.go:18-26`).
* “Backup always writes a complete snapshot.” Source-read failures can yield
  `ErrInvalidSourceData` after reporting an incomplete backup
  (`cmd/restic/cmd_backup.go:176-205`, `:707-718`).
* “The helper `main` packages are restic product commands.” They are separate
  release/developer programs and should not be reported as normal CLI entry
  surfaces.

## Minimum useful onboarding journey

1. Start at `cmd/restic/main.go:36-113` to learn the actual root command graph
   and build-conditioned registrations.
2. Read `README.md:13-39` for the intended `init` → `backup` → `restore` user
   journey, then verify it against the four flows above.
3. Follow `global.OpenRepository` and `internalOpenWithLocked` before changing
   any repository-facing command: they establish credentials, backend/cache,
   and lock behavior.
4. For write-path changes, continue through `runBackup` and
   `Archiver.Snapshot`; for read-path changes, continue through `runRestore`
   and `Restorer.RestoreTo`; use `runCheck`/`checker.Checker` for integrity
   semantics.
