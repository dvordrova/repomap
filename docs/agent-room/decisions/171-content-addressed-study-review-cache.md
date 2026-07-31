# Decision 171: Content-addressed Study review cache

## Status

Approved by the repository owner in the current session after the green
Decision 170 checkpoint `f7a5de6`. Decision 170 remains historical and is not
rewritten. This decision changes only repeated bounded Study reading-pack
reviews.

## Scope

Each existing Study direction still produces the same bounded review bundle,
the same canonical English provider request, and the same local review and
publication validation. The cache unit is exactly that existing provider
request. It is not a file, repository revision, candidate filter, new model
stage, batch, or adaptive fan-out decision.

The persistent cache is content-addressed under one narrow versioned Study
review namespace in the existing run-cache root. The existing generic
model-research cache is not reused because its current identity includes the
whole repository revision, omits the endpoint, and its storage contract is not
bounded or immutable enough for this replay authority. General cache
refactoring is outside this decision.

## Identity and replay

The cache identity binds:

- the cache, validator, and Study review stage contract versions;
- the exact canonical provider request bytes;
- normalized endpoint identity, model, auth/profile, and generation settings;
- the reading-pack prompt version and thinking profile;
- exact bounded review-bundle and source-fragment hashes; and
- the canonical English output-language contract version.

It excludes authorization values, API keys, timestamps, run IDs, and whole
repository revisions. An unrelated repository change therefore cannot evict
an unchanged exact review request, while any changed source fragment produces
a different content identity. Immutable A and B entries coexist, so A→B→A
reuses A.

Every cache hit is decoded and checked against the current exact review bundle
before it can enter the unchanged Study reducer. Unknown or missing anchor IDs,
wrong direction identity, stale request/source identity, malformed JSON,
credentials, unsafe files, and corrupt records are cache misses with bounded
diagnostics. They never become Study input. Only a live response that passes
the same local review checks is cacheable. Provider failures, cancellation,
partial responses, and rejected responses are not cached.

`--no-cache` bypasses both reads and writes. A hit is available through a
prompt-only provider configuration and creates no live client, requires no API
key, and makes no HTTP request. A miss uses the ordinary existing request.

## Preserved semantics

DirectionProposal bytes, candidate count and order, scheduling, review rules,
published Study records, IDs, paths, evidence, source authority, canonical
artifacts, report/manifest formats, HTTP DTOs, and UI remain unchanged. Brief,
direction planning, Orientation, Guided Tour, Architecture, localization, and
other model stages are not migrated.

## Verification

Provider-free fake-HTTP tests cover cold and warm review sets, one-fragment
invalidation, A→B→A, endpoint/model/prompt/contract drift, `--no-cache`, corrupt
entries, invalid cached IDs/source identity, secret rejection, and identical
normalized cold/warm Study JSON and hashes. After focused tests, run exactly
one `./scripts/check.sh`. No live provider, external repository, or UI smoke is
required.
