# Decision 161: Localization characterization and isolated contracts

## Status

Approved by the repository owner in the current session.

## Problem

`--lang ru` currently changes every model-authored semantic request and keeps
Russian responses in locale-specific stage caches. Static Russian UI copy is
then applied after rendering by matching English strings in the DOM. This makes
the language of the report part of semantic analysis, duplicates provider work,
and leaves localization completeness difficult to prove.

The longer localization/cache roadmap is a draft, not implementation
authority. Its diagnosis also predates several corrective commits: notably,
invalid shared research-cache records are already safe misses on the current
head.

## Decision

Authorize two rollback-safe, provider-free steps:

1. Record the current English and Russian report/cache behavior and the
   attributable Python-focus work in one characterization document.
2. Add an isolated `internal/localization` contract for canonical English
   fields, an allowlisted localization input, protected placeholders, an
   identity English projection, and fail-safe application of a supplied
   projection.

The isolated package must:

- derive field IDs from an allowlisted owner kind, a stable semantic owner ID,
  and an allowlisted field name;
- never derive identity from array position, prose, or translated text;
- compute deterministic canonical bytes and SHA-256;
- hide protected technical terms behind typed placeholders in localization
  input;
- restore paths, symbols, packages, protocols, product/library names, and
  other protected terms byte-for-byte;
- fall back to canonical English per field for missing/invalid translations;
- fall back to the complete canonical artifact when the projection names a
  different canonical hash;
- accept no more translations than there are locally known canonical fields,
  and reject a field before scanning when its translated bytes exceed
  `8 * canonical placeholder bytes + 4096`; this is a secondary-processing
  safety bound, not a repository-selection or report-content budget;
- make no provider or network call.

For this checkpoint the useful comparison is English baseline to Russian
projection. An `EN -> RU -> EN` cache round trip is deliberately deferred until
the locale-projection cache exists; before then, the third render proves
nothing new.

## Explicitly not authorized

- ordinary CLI/report wiring or new persisted run artifacts;
- changing provider prompts, model calls, ranking, or semantic validators;
- cache namespace v2, endpoint identity, a dependency manifest, or retries;
- replacing the current DOM localization path;
- porting the dirty Python-focus work;
- live provider calls or new external-repository runs;
- public HTTP, JSON, report, or manifest changes.

## Acceptance

- repeated canonical construction is byte-identical;
- identity projection reconstructs canonical English exactly;
- a supplied Russian projection can change only allowlisted prose;
- spaces, Unicode, and CJK text survive unchanged;
- protected terms survive byte-for-byte;
- unknown fields, missing translations, invalid UTF-8, and placeholder drift
  produce deterministic diagnostics and canonical field fallback;
- canonical-hash mismatch cannot apply any translated field;
- focused tests and `make localization-check` are provider-free;
- the current full repository check remains green.

## Migration and rollback

The package has no production consumer in this decision. Rolling back removes
the package, its Make target, and these documents without changing any existing
run, cache, report, or browser behavior. A later numbered decision is required
before canonical/localization artifacts are written beside ordinary runs.
