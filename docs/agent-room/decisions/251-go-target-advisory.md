# 251 — Conservative Go target advisory

**Status:** ACTIVE (owner-authorized, 2026-08-09)

**Preserves:** D250's one explicit atomic Go target and all existing report,
manifest, provider, cache, server and UI contracts.

## Product problem

`--go-target` fixes analysis of a known non-host build, but a new user can
still run a Linux-oriented repository on macOS without realizing that the host
target hid important production code. A raw text search for `linux` would be
misleading because prose, tests and negated constraints are not evidence that
another Go build universe is more useful.

## Approved contract

- The ordinary console prints the resolved current Go target and the exact
  `--go-target GOOS/GOARCH` override syntax at snapshot start.
- The existing safe tracked regular-file inventory may be scanned once with
  bounded prefix reads. There is no second Git inventory pass.
- Only Go filename constraints and parsed leading Go build constraints count.
  Tests and paths under vendor, testdata, examples or tools do not count;
  negated OS mentions and expressions containing custom tags do not vote.
- At most one alternative is suggested. It must be the unique GOOS leader,
  have at least three production-file witnesses, and have at least twice the
  witness count of the current GOOS. At most three exact paths explain it.
- The current GOARCH is retained. Existing target parsing and Go loaders remain
  the authority for the selected pair; the advisory duplicates no toolchain
  support matrix.
- The advisory never changes the target. Ambiguous, incomplete or over-budget
  evidence produces no repository-specific suggestion.
- This is console-only in-memory guidance. It adds no snapshot JSON, report,
  manifest, provider, cache, gopls/reportserver or UI field and no new semantic
  stage.

## Acceptance

1. Three Linux production files select one `linux/<current-arch>` suggestion
   from a Darwin target.
2. Negative constraints, custom tags, tests and excluded path roles do not
   create a suggestion.
3. Tied alternatives and weak evidence remain silent.
4. A real Moby snapshot prints the current target and, on Darwin, one bounded
   Linux suggestion with exact evidence paths.

## Owner-risk check

This is the smallest product action that makes D250 discoverable without
guessing on the user's behalf. It will still matter next week for every
platform-oriented repository, fixes target-selection discoverability rather
than a test symptom, and adds one bounded local helper plus in-memory console
fields. Skipping it makes the next macOS Moby run repeat the wrong-program
analysis unless the user remembers the new flag manually; a later report
cannot recover the omitted build universe.
