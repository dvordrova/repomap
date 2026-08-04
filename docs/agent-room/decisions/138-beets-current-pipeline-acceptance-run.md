# Decision: Beets Current-Pipeline Acceptance Run

Status: Complete. Corrective and saved-run replay accepted; local checkpoint
authorized by the product supervisor.
Approved by the repository owner and product supervisor in the current
session.

## Baseline

Use the completed Decision 137 checkpoint:

- commit:
  `ba839917f2c44c97fbbc6e8d43a3d9488d1a9cdc`;
- binary:
  `/Users/dvordrova/git/repomap/.bin/repomap`;
- binary SHA-256:
  `6d4f7dbb4aaf540bf2f2f807abb873e327a83f29b8b8519cad4c465e11662192`.

The target is the clean Beets checkout:

- repository: `/Users/dvordrova/git/beets`;
- revision: `9acb1ecff6c7ee0a1e83e3b983c94792345712c5`;
- tracked files: 577;
- tracked Python files: 303;
- tracked symlinks: 0.

The only earlier Beets run is an obsolete format-9 orientation artifact. It
contains no current topics, Study, mechanisms, Architecture, or current
product UI and is not acceptance evidence.

## Authorized action

Run exactly once:

```sh
repomap /Users/dvordrova/git/beets --no-search
```

No retry, replay, timeout inflation, code change, Django run, python-dotenv
run, or second repository is authorized by this decision.

That run completed before any corrective work. Its concrete result was then
returned to the product supervisor, satisfying the run-only stop condition.

## Evidence to retain

Preserve:

1. the complete command log and exact run path;
2. binary and repository identity, revision, and cleanliness;
3. `run_manifest.json` authority and the recorded no-Search policy;
4. all stage names, timings, warnings, fallbacks, and terminal status;
5. whether semantic opportunity discovery ran exactly once;
6. exact Python declaration path, symbol, and line starts;
7. explicit `proof_adapter_unavailable` routing and evidence that the Go
   planner and probe did not run for Python weak signals;
8. `report.json` counts for topics, incomplete Study, complete Study, and
   mechanisms; and
9. screenshots of Overview, Study or topic detail, exact source, and
   navigation showing Search absent and Architecture secondary.

## Stop conditions

Stop at the first terminal report publication or command failure. Return to
the product supervisor before any corrective. Also stop and report
immediately on:

- source-authority failure;
- Go planner or probe use for a Python weak signal;
- a complete-Mechanism claim from the unsupported Python proof path;
- missing manifest;
- a published source start that is not exact; or
- any request to retry or expand to another repository.

## Completion condition

This decision completes only after terminal artifact and log inspection,
browser verification when a report exists, and a concrete product-supervisor
verdict.

## Run result

- run: `/Users/dvordrova/Library/Caches/repomap/runs/20260729-083155-beets-abc816cd853f`;
- log: `tmp/beets-d138/run.log`;
- the manifest v4 repository root, revision, freshness, and report hash were
  exact, and both the effective option and report disabled Search;
- the report published three incomplete Python topics, zero complete
  mechanisms, and exact editor starts; no Go proof adapter ran;
- `Importing Music Files` was a useful central start, while the logfile topic
  was peripheral and the dynamic-attribute/plugin topic was misleading;
- General Study failed locally before a provider call because its saved Python
  path entered the Go-only sourcewindowfacts validation path;
- Architecture was empty and Paved Paths spent about 20 seconds to publish no
  paths, both recorded as separate observations outside this corrective.

## Corrective authorization

The first supervisor authorization limited the fallback to
`cmd/repomap/study_map.go` and its test. The provider-free fixture proved that
slice impossible: `internal/studymap.Bundle.Validate` and review projection
unconditionally required `sourcewindowfacts.Function`. Both attempted files
were restored byte-for-byte before asking for a revised verdict.

The revised supervisor verdict authorizes exactly these six files:

1. `internal/studymap/studymap.go`;
2. `internal/studymap/studymap_test.go`;
3. `internal/studymap/editing.go`;
4. `internal/studymap/editing_test.go`;
5. `cmd/repomap/study_map.go`; and
6. `cmd/repomap/study_map_test.go`.

Every Study anchor must select exactly one source arm:

- the existing Go `sourcewindowfacts.Function`, validated unchanged; or
- a non-Go `ExactSource` with a canonical repository-relative path, explicit
  path-matching language, exact symbol/line, bounded contiguous saved lines
  containing that line, and a matching content hash.

Anchor path, symbol, and line must match the selected arm. Review projection
uses that arm's already validated lines. Existing Go JSON and bundle-hash bytes
must remain unchanged. The assembly fallback may reuse only the existing
catalog-resolved, provenance-checked, openable declarations returned by
`freshSavedDiscoverySources`; it may not fabricate a Go function or publish a
complete mechanism.

## Corrective verification

The first focused command exposed only a missing parent directory in the new
temporary Python fixture. After creating that fixture directory, the identical
focused commands passed:

```text
ok github.com/dvordrova/repomap/internal/studymap 0.406s
ok github.com/dvordrova/repomap/cmd/repomap 0.723s
```

The tests accept unchanged Go anchors and one exact Python declaration through
bundle assembly, BundleHash/Validate, strict decode, and review-source
projection. They reject neither/both arms, Go data in ExactSource,
path/symbol/line mismatch, an out-of-range line, malformed source, and a hash
mismatch. The provider-visible prompt still contains no saved source text, and
the Python fixture publishes no complete mechanism.

`./scripts/check.sh` then passed with exit code 0: shell syntax, all
`go test ./...` packages, `go vet ./...`, and all six committed offline quality
replays. `git diff --check` passed. No provider call or repository rerun was
made during the corrective.

## Corrective stop conditions

Do not change sourcewindowfacts or its Go path validation; do not fabricate Go
data; do not bypass BundleHash/Validate; do not admit unresolved, non-openable,
non-catalog-resolved, invented, or unverified source; and do not change the
prompt, provider, complete-pack gate, renderer, Search, Architecture, Paved
Paths, mechanisms, or unrelated schemas. Do not rerun a repository or provider
until the product supervisor reviews this exact result.

## Saved Beets provider-free replay

The supervisor accepted the corrective with no blocker and authorized one
provider-free replay of the already saved Beets run, then a stop for review.
The existing saved-run test entrypoint assembled the current bundle from:

- run: `/Users/dvordrova/Library/Caches/repomap/runs/20260729-083155-beets-abc816cd853f`;
- repository: `/Users/dvordrova/git/beets`.

The first assembly-only replay passed in 0.05 seconds with four anchors, seven
areas, and four production anchors. A temporary replay assertion, removed
immediately after the check, then passed the same saved bundle through
`studymap.BuildReviewBundle`, asserted zero complete mechanisms, and printed
the exact review projection:

```text
beets/ui/commands/__init__.py:22 __getattr__
  def __getattr__(name: str):
beets/ui/commands/import_/__init__.py:15 paths_from_logfile
  def paths_from_logfile(path):
beets/ui/commands/import_/__init__.py:35 parse_logfiles
  def parse_logfiles(logfiles):
beets/ui/commands/import_/__init__.py:50 import_files
  def import_files(lib, paths: list[bytes], query):
```

The command passed in 0.06 seconds (package time 0.627 seconds), made no
provider call, and wrote no run artifact. The temporary assertion left no file
or diff. The real saved declaration is line 50, not line 41: line 41 in the
current Beets checkout is `raise UserError(`. The supervisor's requested
`:41 import_files` came from the synthetic corrective fixture and is corrected
by this real-run replay rather than being forced into production.

The product supervisor accepted the replay, confirmed zero complete
mechanisms, explicitly kept candidate quality and the suspicious `__getattr__`
topic outside this transport claim, reported no blocker, and authorized one
local commit containing the six implementation/test files plus this decision
bookkeeping.
