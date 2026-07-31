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

## Verification

Before the first checkpoint, focused fake-provider tests prove the exact
6000-to-12000 recovery, one logical retry, aggregate telemetry, and no retry
for an ordinary invalid JSON completion. Compile and focused tests precede the
checkpoint; the full repository check and real UI review follow the pushed
binary.
