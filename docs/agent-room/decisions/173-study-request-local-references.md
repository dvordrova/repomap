# Decision 173: Study request-local typed references

## Status

Approved by the repository owner in the current session after the Decision 172
checkpoint, then explicitly queued behind the release-blocking Decision 174
localization diagnostic gate. No production migration is included in the
Decision 174 checkpoint. Decision 172 remains historical and is not rewritten.

## Scope

One provider-backed Study candidate response copied an otherwise valid anchor
ID with two hexadecimal bytes deleted from the middle. Prefix recovery could
not identify it, and edit-distance repair would turn canonical identity into a
heuristic model-output protocol.

Every active Study request now projects each opaque object that the model must
return into a deterministic request-local typed handle. Repository bundle
requests use distinct area, anchor, document, and mechanism handle namespaces;
each reading-pack review uses distinct direction, anchor, and area namespaces.
The provider sees and returns only these handles. Canonical IDs remain local and
are restored only after an exact handle lookup with the field's expected type.
Unknown, wrong-type, duplicate, cross-request, shortened, or otherwise altered
handles fail closed before the existing canonical validators run.

The handle table and its deterministic ordering are bound to the exact request
and Study review cache identity together with an explicit reference-contract
version. Study prompt, output, validator, stage, and cache contract versions
change. Old cache entries are not read, migrated, or reclassified; the only
supported invalidation operation is `repomap cache clear`.

This decision does not change candidate count or composition, scheduling,
review validation, publication limits or order, canonical direction IDs,
paths, evidence, canonical Study JSON, or report behavior. It adds no fuzzy,
prefix, deletion, edit-distance, or legacy parser and makes no provider call in
verification.

## Verification

Provider-free tests cover the observed mid-ID deletion, exact and wrong-type
handle resolution, prompt absence of canonical IDs, Brief and Direction
round-trips, per-review round-trips, cache identity binding, corrupt cache
recomputation, and equality of cold and warm canonical Study output. Focused
tests precede the checkpoint push and binary rebuild; the full repository check
follows that delivery checkpoint.
