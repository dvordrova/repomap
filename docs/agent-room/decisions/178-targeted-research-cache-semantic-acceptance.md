# Decision 178: Targeted-research cache semantic acceptance

## Status

Approved by the repository owner as a narrow reliability correction after
Decision 177.

## Problem

`modelresearch.ExecuteRound` persisted a targeted provider response before the
local response decoder and semantic evidence validator ran. A response whose
entire finding set was rejected could therefore become a structurally valid
persistent cache record. Later runs replayed that record even though it could
not produce an accepted research round.

## Decision

The targeted-research cache stores only responses that have passed the same
decode, evidence-ID, and semantic acceptance path used by an ordinary live
round. Decode or validation errors and `all_findings_rejected` outcomes are not
cached.

A cache hit is decoded and validated into an isolated round before any cached
response is applied or published. If the exact cache record is corrupt or its
response is semantically rejected, only that record is removed and the request
continues as an ordinary cache miss. A configured provider may recompute the
request once through the existing execution path. If recomputation fails, the
round fails explicitly and the rejected record remains absent.

Targeted research uses the versioned cache contract
`targeted-research-cache-v3`. The contract is included in both its exact cache
fingerprint and persisted record. Earlier targeted-research entries are not
read, migrated, or mapped. `repomap cache clear` is the explicit operation for
whole-cache invalidation.

Accepted cache hits continue through the current local validator, preserve the
accepted canonical output/order/IDs/evidence, and make no provider semantic
call. This decision does not change prompts, request shapes, provider retries,
orientation, Study, Guided Tour, localization, report/manifest formats, UI, or
flags. Non-targeted stage contracts are unchanged.

## Proof

Provider-free tests establish that:

- a semantically rejected or malformed live response creates no cache entry;
- a subsequent accepted response creates one current-contract record;
- a structurally valid record whose response is semantically rejected is
  removed, recomputed once, and never published;
- a failed recomputation leaves the rejected exact record absent;
- an accepted warm hit makes zero additional provider calls and has the same
  normalized accepted round output as the cold run;
- corrupt record versions are treated as misses and recomputed;
- changing the targeted cache contract changes the exact cache key.
