# Decision 186: Truthful local operational entrypoint absence

## Status

Approved by the repository owner as the smallest semantic correction to the
blocked Decision 185 checkpoint.

## Problem

Decision 185 copied a local operational candidate's first exact `OpenFile` into
`likely_entrypoint` when discovery intentionally left `EntrypointPackage` empty
because it had no static reachability proof. Exact file identity proves a useful
seed, not an executable entrypoint relation. The fallback therefore
manufactured the semantic binding that the local producer had declined to
claim.

## Decision

The Decision 185 fallback is removed without rewriting its historical commit.
A newly appended local operational flow keeps a non-empty producer-owned
`EntrypointPackage` when one exists. Otherwise its `likely_entrypoint` remains
empty, while its exact ordered `LikelyFiles` and source-signal evidence remain
available under the existing
`CandidateBasisSourceSignalAggregate` classification.

Only that exact local candidate basis may pass Orientation structural
validation with an empty `likely_entrypoint`. Provider/model flows still
require an exact typed `likely_entrypoint_ref`; local-entrypoint,
runtime-activity, unspecified, and every other candidate basis still fail
closed when the field is empty. Matching provider flows are not repaired or
normalized.

The focused local bundle continues to validate and select `LikelyFiles` as
exact high-priority seeds. An absent entrypoint contributes no query term, and
the existing selector does not derive query semantics from likely-file paths.
No path parsing, filesystem search, package inference, fuzzy repair, or new
semantic relation is introduced.

Candidate composition, order, names, IDs, exact files, evidence, confidence,
model acceptance, prompt/request/cache contracts, Study, Atlas, Architecture,
localization, UI, retries, clients, flags, and `--llm-bundle-only` bytes remain
unchanged. Report DTO/format and manifest authority remain unchanged;
successful local output now truthfully represents the absent entrypoint while
its exact files and evidence stay intact.

## Proof

Provider-free focused tests establish that:

- existing provider flows remain structurally unchanged during local merge;
- a local source-signal aggregate validates with an empty entrypoint and its
  exact ordered likely files and evidence intact;
- downstream local bundle selection retains the exact likely-file seed without
  synthesizing entrypoint/path query terms;
- model, local-entrypoint, runtime-activity, unspecified, and other empty-
  entrypoint candidates reject;
- a typed provider response missing `likely_entrypoint_ref` remains rejected.
