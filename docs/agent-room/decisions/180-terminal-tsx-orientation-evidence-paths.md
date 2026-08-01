# Decision 180: Terminal TSX orientation evidence paths

## Status

Approved by the repository owner as one narrow orientation reliability
correction after Decision 179.

## Problem

The saved failed run
`20260801-065931-python-tutorial-game-eb5f039ecf18` contains the exact allowed
path `front/src/index.tsx` in both the bounded orientation response and
`allowed_paths`. The evidence-path grammar recognized `.ts` without requiring
a terminal boundary, so it extracted `front/src/index.ts` from the valid
`.tsx` path. Normalization and final validation then disagreed about the same
model evidence and aborted orientation as outside the allowlist.

## Decision

The existing bounded orientation evidence-path grammar recognizes `tsx` as a
complete extension and requires a terminal RE2 boundary after every recognized
extension and optional line suffix. A shorter extension cannot match a prefix
of a longer path token. Matching resumes at the captured-path boundary so the
consumed delimiter cannot hide an adjacent path.

This decision does not broaden path authority: every extracted path still
passes the existing repository-relative and exact `allowed_paths` validation.
A genuinely invented `.tsx` path remains rejected.

There is no change to model prompts or requests, cache contracts, clients,
retries, candidate-flow composition, LocalProof, Study, locale, UI,
report/manifest formats, flags, adapter discovery, or fallback policy.

## Proof

Provider-free tests establish that:

- a parsed orientation containing exact allowed `.tsx` evidence survives
  normalization and final validation without being truncated to `.ts`;
- an invented `.tsx` evidence path is still rejected outside `allowed_paths`;
- `.tsx` and `.ts` are recognized only at terminal boundaries, including line
  suffixes, while longer suffixes and path continuations are not partially
  matched;
- adjacent terminal paths remain independently discoverable.
