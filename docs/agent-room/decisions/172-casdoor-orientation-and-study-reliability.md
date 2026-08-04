# Decision 172: Casdoor orientation and Study reliability

## Status

Approved by the repository owner in the current session after the Decision 171
checkpoint. Decision 171 remains historical and is not rewritten.

## Scope

One real uncached Casdoor run reproduced a provider completion ending with
`finish_reason=length` at the existing 6000-token orientation envelope. The
request returned a truncated JSON object, so the run stopped before Study and
no report was published. The immediately preceding Casdoor baseline used the
same prompt version and envelope and completed normally.

Orientation therefore gets one bounded logical completion retry only for the
provider's explicit length outcome. The retry preserves the exact canonical
English prompt, facts bundle, endpoint, model, validation, and authority; it
only doubles output headroom once. Ordinary invalid JSON is not retried.

The control run then showed a second provider-specific failure: official
DeepSeek V4 spent all 12,000 recovery tokens on hidden reasoning and returned
no JSON content. Orientation is a bounded classification over an already
compact local facts bundle, so its official-DeepSeek envelope now disables
thinking explicitly, matching the existing architecture grouping policy.
Compatible endpoints receive no DeepSeek-specific field. This does not change
the facts, schema, prompt prose, or local validation.
Transport retries remain independently bounded, and usage plus attempt
telemetry includes every transport attempt. Debug metadata keeps one semantic
call while recording the exact transport-attempt count and aggregate attempted
request bytes.

The existing orientation cache owns only one exact request identity per stage
fingerprint. A response accepted from the larger recovery envelope is therefore
kept for the current run but is not written under the original smaller request
identity. The prompt/request contract version changes so an older cached stage
cannot masquerade as this recovery-aware stage.

After this recovery is delivered, the same Casdoor repository is rerun and its
measured candidate, reading-pack review, and publication funnel is compared to
the saved baseline. Subsequent Study changes must target a demonstrated loss
boundary and may not use Decision 171's cache to change candidate composition,
review validation, published IDs, evidence, paths, or order.

The saved post-recovery Casdoor Study artifact then demonstrated that exact
loss boundary without another provider call: all nine proposed directions had
one accepted reading-pack review, but the review reducer stopped at its smaller
six-direction cap before the canonical seven-direction selector ran. The three
discarded valid packs included the controller authentication and management
direction, while two ACME directions survived. The historical artifact is a
Study/discovery baseline only; it predates the final localization checkpoint
and is not RU acceptance evidence.

The corrective removes those two smaller output caps as independent policy.
The already bounded twelve-candidate input limit now owns review compression
and canonical publication as one shared maximum. Deterministic semantic
duplicate suppression, ordering, local review validation, candidate discovery,
provider requests, cache identity, IDs, paths, and evidence remain unchanged.
Any reviewed pack still removed as a semantic duplicate (or by the shared
defensive cap) receives a stable bounded issue code in the saved reduction, so
the loss is visible without provider prose or jq archaeology.

That control run scheduled all eight accepted candidates, but four independent
reading-pack reviews returned no JSON content because official DeepSeek spent
the complete output on hidden reasoning. One of the four even ended with
`finish_reason=stop`, so more response headroom alone is not a recovery. Each
review is only a bounded classification over three to five exact anchors; the
official endpoint therefore disables thinking for this stage, while compatible
endpoints remain unchanged. The prompt contract version changes, which safely
invalidates the exact-request Decision 171 cache without changing candidates,
scheduling, validation, publication, IDs, evidence, paths, or order.

The same run also exposed a presentation-only preparation failure: inferred
opaque tokenization split `s3` out of the already protected path
`storage/aws_s3.go`, then rejected its own token under identifier-boundary
validation. Inference now uses that same boundary predicate before adding an
opaque value. Canonical prose and explicit opaque values remain unchanged.
After preparation recovered, the complete 557-field Casdoor projection then
reproducibly exhausted the ordinary 6,000-token response envelope. Repeating
long presentation addresses in the provider response then consumed 53,990
avoidable bytes, so the transient provider envelope now uses strict ordered
index/text pairs and restores the same stable ID-keyed internal projection
before validation. Even that compact response ended at exactly 32,000 tokens.

The owner and supervisor therefore explicitly supersede Decision 170's
one-request localization constraint for this measured failure. Localization
now partitions the complete inventory deterministically in stable field-ID
order by a predicted output budget. Every batch has an exact manifest and
content hash, one compact ordered `[index, text]` provider request, its own
exact-request cache entry, and the same strict live/cache validation. Provider
output remains capped at 32,000 tokens per batch; the implementation does not
replace the demonstrated limit with a speculative 64,000-token monolith. Each
batch contains at most 64 fields. The current saved Casdoor replay contains 508
fields and deterministically partitions into eight batches: seven batches of
64 fields and one final batch of 60. This is a replay measurement of the
current inventory and batching contract, not a claim that a complete live RU
projection has succeeded.

Every live response or cache hit passes the same strict completeness,
placeholder, target-language, secret, and tuple-order validation. A rejected
result is neither applied nor cached; localization does not add a separate
repair request or weaken validation.

The internal projection and persisted sidecar remain stable ID-keyed values.
No sidecar is published until all independently validated batch results
are merged and the complete projection validates against the full canonical
English inventory. A missing, corrupt, truncated, or rejected batch therefore
degrades the whole presentation to canonical English; partial RU is never
published, cached as a complete projection, or labelled successful.

The complete response then exposed a prompt-quality failure rather than a
validator ambiguity: 167 fields retained unprotected English prose. The
translation contract keeps opaque technical values outside model discretion
through typed field semantics and reversible object-local placeholders, while
every unprotected heading, label, name, diagnostic, and explanation is
translated as human prose. It does not add a lexical dictionary that treats
words such as `Controllers`, `main`, or `s3` as globally translatable or opaque.
It also does not impose a blanket policy to translate or transliterate every
Latin span: opaque spans are classified by typed ownership before the request,
and the model translates the remaining human prose normally.
The validator remains strict and the whole projection still degrades atomically
if any field or placeholder violates that rule.

Localization cache contract changes do not decode or classify older cache
entries. `repomap cache clear [--debug-dir DIR]` is the single explicit clean
invalidation operation and leaves saved run artifacts intact.

## Verification

Before the first checkpoint, focused fake-provider tests prove the exact
6000-to-12000 recovery, one logical retry, aggregate telemetry, and no retry
for an ordinary invalid JSON completion. Compile and focused tests precede the
checkpoint; the full repository check and real UI review follow the pushed
binary.
