# Decision 181: Shared provider transport retries

## Status

Approved by the repository owner as one narrow provider-transport reliability
slice after Decision 180.

## Problem

Normal targeted provider stages already replay the same bounded request after a
retryable transport failure. Exact localization execution used the same low
level transport but made only one attempt. In addition, a response-body read
failure was classified as non-retryable even when it represented a transient
connection reset after headers had arrived.

This made an otherwise immutable localization request less reliable than the
normal provider path and under-reported the transport work actually attempted.

## Decision

One private bounded transport retry primitive owns replay of an already-built
exact chat request. Targeted research and localization both use it. The
primitive preserves the existing retry count and exponential-jitter backoff,
rebuilds an identical HTTP request from the same immutable body bytes for each
attempt, preserves endpoint and authentication inputs, stops on context
cancellation, and returns the actual transport-attempt count.

HTTP 429 and 5xx responses, retryable request errors, and retryable response
body read failures use that existing policy. A malformed response envelope,
invalid JSON completion, or locally rejected semantic projection is not a
transport failure and does not trigger transport replay. Exact localization
request evidence remains validated before any network activity.

Localization request-byte metrics count every actual identical transport
attempt. Targeted-research semantic behavior and accepted output remain
unchanged.

`Retry-After` interpretation is explicitly outside this decision. The current
shared transport boundary reports retryability without carrying response
headers; adding partial header-delay plumbing would expand the contract rather
than reuse the existing bounded backoff policy.

There is no change to prompts or bodies, provider identity, cache identity or
semantics, Study, canonical artifacts, locale UI, flags, or saved formats.
There is no legacy reader or migration.

## Proof

Provider-free tests establish that:

- localization retries a connection reset, HTTP 429, HTTP 503, and a mid-body
  read failure, then succeeds with two byte-identical requests and
  `Attempts=2`;
- cancellation after the first retryable response prevents a second request;
- malformed JSON and a structurally JSON but semantically invalid localization
  projection make one transport attempt only;
- invalid localization request evidence is still rejected before transport;
- existing targeted-research and orientation retry tests retain their behavior.
