# Decision 182: Semantic cache-hit revalidation

## Status

Approved by the repository owner as one narrow generic stage-cache reliability
slice after Decision 181.

## Problem

The generic `StageResponse` cache validates its record envelope, hashes, and
exact request identity before returning a hit. Semantic acceptance belongs to
the consuming stage. Orientation, the monolithic Guided Tour, Guided Tour
fan-out leaves, and Guided Tour fan-in all validate a cached response locally,
but a semantic rejection currently terminates or degrades that stage instead
of treating the rejected exact record as a miss. The same unchanged cache entry
can therefore permanently suppress the stage's ordinary recomputation path.

Targeted research and Study use separate accepted-response cache contracts and
are outside this decision.

## Decision

One small shared primitive removes only an exact generic `StageResponse` cache
record after a consuming stage rejects its semantics. The existing loader and
each stage-owned validator remain separate. A valid semantic hit is returned
unchanged. A semantically invalid hit is removed and becomes a cache miss; the
caller then follows its existing budget, scheduling, provider-call, validation,
and save path in the same run.

Validation remains stage-local:

- orientation uses its existing parse, grounding normalization, local-proof
  attachment, confidence classification, and final orientation validator;
- monolithic Guided Tour uses its existing proposal parser and validator;
- fan-out leaves use their existing parse, normalization, and task validator;
- fan-in uses its existing parser and bundle/result validator.

Structurally corrupt cache records remain ordinary misses under the existing
loader. Failed, canceled, undecodable, or semantically rejected live responses
continue to be excluded from the cache. A valid hit still requires zero
provider calls.

There is no prompt, request, candidate, Study, canonical artifact, locale, UI,
publication, policy, scheduling, or saved-format change. There is no new cache
framework, flag, legacy reader, or migration.

## Proof

Provider-free focused tests establish that every in-scope stage evicts a
semantically invalid exact hit and performs its ordinary recomputation, while a
valid warm hit makes zero provider calls. They also retain the existing
structural-corruption miss behavior and prove that invalid live replacements
are not cached.
