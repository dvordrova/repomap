# Decision 174: Localization reliability diagnostics

## Status

Approved by the repository owner in the current session as the immediate
release-blocking checkpoint. Decision 173 remains approved but queued and is
not partially implemented here.

## Scope

A real Russian micro-repository run started one localization batch, rejected
its response as `invalid_projection`, and then printed seven impossible
`batch 1/0` lines for batches that never started. Another run failed during
preparation before any batch existed. The current status and console output do
not distinguish those boundaries safely enough to diagnose the root cause.

Localization execution records only batches actually processed. Its persisted
safe status advances to a closed v2 diagnostic contract containing a bounded
failure stage, validation code, total batch count, attempted count, completed
count, and failed batch number. It contains no provider response, path,
endpoint, header, credential, or unbounded error prose. Old status records are
not migrated or interpreted as v2.

When and only when the existing `--dump-llm` diagnostic mode is enabled, a
rejected localization response may be saved under the existing bounded debug
authority after the existing secret scan and redaction rules. The ordinary run
does not persist rejected provider prose. Guided Tour rejection records a
closed validator field and rule without changing its existing path-like
validation or model prompt.

This first checkpoint changes no prompt, cache identity, candidate or Study
behavior, canonical English data, successful localization projection, report
DTO, or UI rendering. The exact provider-free micro-repository reproduction is
then used to identify the actual `invalid_projection` predicate; any root-cause
correction must be the smallest local fix to that proven predicate and must not
add retries.

## Verification

Focused provider-free tests prove no phantom batches, pre-provider versus
provider versus projection failure classification, safe bounded status JSON,
`--dump-llm`-only rejected artifacts after redaction, and typed Guided Tour
field/rule diagnostics. Compile and focused tests precede the early checkpoint
push and exact binary rebuild. The owner then supplies real-run logs before the
checkpoint is considered complete; the full repository check follows the
early delivery.
