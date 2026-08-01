# Decision 194: Honest global semantic output ceiling

## Status

Approved by the repository owner through the active supervisory instruction as
the reliability checkpoint after Decision 193.

## Problem

The configured provider maximum is not the maximum that every semantic request
actually sends. The default client reports `max_tokens: 6000`, while ordinary
request builders silently raise selected official-DeepSeek stages to 10,000,
12,000, 20,000, or 32,000 tokens. Orientation and Guided Tour may also resend a
completion after truncation, and Guided Tour may request a fresh proposal after
local semantic rejection. The resulting resource contract is difficult to
understand, can spend several output envelopes for one semantic question, and
can cache a doubled-envelope Guided Tour result under the initial request
identity.

The shared response decoder does not reject every explicit
`finish_reason=length`: a non-empty syntactically valid JSON value, and every
non-empty response on a caller-owned JSON path, currently pass through without
retaining the finish reason. Optional-stage errors are normally allowed to
degrade, so a resource-limit failure can be swallowed and followed by later
model stages and an authorized report. Russian localization additionally
publishes an authorized canonical report before its model batches complete.

## Decision

Use one exact semantic output ceiling for every model request. The default is
`64_000` tokens. `REPOMAP_LLM_MAX_TOKENS` remains the only override and is a
truthful hard ceiling: every ordinary, experimental, official-DeepSeek, and
generic OpenAI-compatible semantic request serializes that exact configured
value. Request builders never raise, lower, or double it. Stage-owned thinking
profiles remain unchanged. No provider capability registry, probe, automatic
fallback, or per-stage output knob is added; a provider that does not support
the configured value returns a normal terminal provider error.

Remove every automatic stage floor and every semantic completion or proposal
resend. In particular, Orientation never doubles after truncation, the Guided
Tour provider adapter never retries malformed, invalid, or truncated response
content, and the Guided Tour caller never asks for a replacement proposal after
local rejection. Existing bounded transport retries remain byte-identical and
conceptually separate from semantic calls.

An exact pre-call request hard-limit breach, response hard-limit breach, HTTP
response-body overflow, or any `finish_reason=length` is a typed resource-limit
error. This is true even when returned content is valid JSON. The error exposes
only bounded stage, configured limit, authoritative usage, byte counts, and
finish evidence; parsed provider content already available at the transport
boundary remains available only so the existing Decision 192 exchange owner can
apply its mandatory redaction and secret handling. There is no semantic resend,
cache write, apply, partial publication, or fallback for the failed response.
The existing semantic exchange recorder writes the failed exchange once on a
best-effort basis and no new artifact path or format is introduced.

A typed resource-limit error terminates the complete ordinary run with a
non-zero exit. Later stages are not called and no authorized report, run
manifest, `latest` link, server, or opened report is produced. Previously valid
cache entries are not rolled back and no stage-wide cache transaction is added.
Only the invalid or length-ended response is forbidden from cache and apply.
Other non-resource optional-stage and localization fallback policies remain
unchanged.

Architecture synthesis cache identity additionally binds the exact provider
request, including the configured output ceiling. Removing Guided Tour response
resends removes its doubled-response-under-initial-request identity defect
without a new cache framework.

Russian localization consumes the already-read in-memory canonical report data
and prepared presentation under the confirmed repository authority. The
authorized report is generated exactly once after localization succeeds or
after the existing non-resource localization policy chooses canonical English.
A typed resource-limit error returns before any report JSON, HTML, manifest, or
`latest` publication.

Implementation is split into three reviewed checkpoints:

1. shared provider output-envelope core: default 64,000, exact request values,
   removal of floors and Orientation/Guided provider response resends, plus
   typed `finish_reason=length` and response-body-limit evidence;
2. ordinary stage owners, Decision 192 recording, Architecture exact request
   cache identity, Guided caller retry removal, and terminal top-level
   propagation; and
3. single-publication Russian localization, truthful doctor/run metadata,
   documentation cleanup, and full provider-free verification.

This decision does not add response schema/cardinality limits, provider
capability discovery, a new debug schema, a new cache framework, cache
transactions, semantic fallback, legacy readers, migrations, README/edge/file
knobs, or a live-model verification path. Decision 193 grounding behavior and
all earlier accepted local evidence contracts remain intact.

## Proof

Provider-free focused tests establish that:

- default, lowered, and raised configurations serialize the exact same global
  ceiling across official and generic provider request builders while retaining
  their existing thinking profiles;
- no request builder mutates `MaxTokens` and no completion or locally rejected
  response triggers another semantic request;
- valid, invalid, and empty length-ended responses fail with one typed resource
  error and preserve bounded safe evidence without exposing provider prose in
  the error;
- retryable transport failures retain their existing bounded byte-identical
  replay, while cancellation and non-retryable or semantic failures do not;
- the failed response is not cached or applied, Architecture misses after an
  exact provider-request ceiling change, and Decision 192 records at most one
  failed exchange through its existing safe payload handling; and
- a typed resource-limit failure at every ordinary stage exits non-zero before
  later calls, authorized report/manifest generation, `latest`, serving, or
  opening, including the Russian localization path.

All verification uses in-memory providers or local HTTP fixtures. It makes no
live model request.
