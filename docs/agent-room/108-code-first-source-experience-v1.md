# Decision: Code-first Source Experience v1

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product outcome

Every source action in the default production report either shows saved,
bounded source code or opens an exact authorized editor location. A path-only
drawer is not a source experience.

## Approved implementation

1. Keep the User Report v2 workspace, canonical Mechanism, semantic search,
   architecture canvas, and existing report renderer.
2. Add a presentation-only `SourceSnippet` projection derived
   deterministically from accepted facts, evidence references, saved source
   windows, and probe content. It may carry path, language, enclosing symbol,
   bounds, highlight ranges, exact saved content and its hash, related opaque
   evidence IDs, and a presentation role.
3. Source snippets do not change Mechanism IDs, hashes, claims, evidence,
   verdicts, or replay semantics. No model chooses or edits source.
4. A Mechanism step shows one primary source group inline and at most two
   related groups. Group references by enclosing symbol and file instead of
   rendering one button per raw reference. Preserve the selected Mechanism and
   step while browsing code.
5. Static and replayed reports embed the primary saved snippets. A built-in
   server may return additional bounded context only through a run-manifest
   authorized, repository-root-bound opaque source capability. Raw client
   paths never authorize reads.
6. Saved report content is the canonical snapshot. Current working-tree bytes
   must never silently replace it; revision differences are explicit.
7. Exact path, symbol, and location search results open code or an exact editor
   target. Architecture is never a source fallback. Coarse/no-op map controls
   may be hidden.
8. Overview without a Mechanism remains useful through code-bearing entrypoint,
   core, example, or test excerpts when such saved source exists; path-only
   cards are omitted.

## Truth and safety boundary

- No new model call, probe, analyzer, repository-wide analysis, semantic
  relation, fact, evidence, claim, search ranking, or architecture object.
- Only locally validated saved bytes enter an embedded snippet.
- Snippet selection is deterministic and bounded: one primary plus at most two
  related source groups per step.
- Report-server expansion reuses existing manifest and repository freshness
  authority and enforces bounded UTF-8 reads with traversal/symlink defense.

## Focused checks

- Caddy and chi Mechanism steps render real code with exact highlighted lines;
- raw reference lists collapse into bounded source groups;
- source browsing preserves Mechanism and step context;
- exact search opens code rather than path metadata;
- static/replay reports retain embedded primary snippets;
- a no-Mechanism report has no path-only source action;
- canonical Mechanism hashes remain unchanged;
- server source requests cannot escape the authorized repository/run;
- the review bundle contains Caddy, chi, and no-Mechanism reports plus the
  requested walkthrough and screenshots.
