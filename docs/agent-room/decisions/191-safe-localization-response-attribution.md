# Decision 191: Safe localization response attribution

## Status

Approved by the repository owner through the active supervisory instruction as
the immediate reliability checkpoint after Decision 190. The broader
Repository Atlas inventory remains read-only planning and is not implemented
by this decision.

## Problem

An exact Casdoor Russian-localization run rejected batch 6 of 8 at the
mandatory provider-response secret scan. The existing closed failure stage and
validation code prove that the raw response was unsafe, but they cannot
distinguish unsafe material inside one strict translated value from unsafe
material in a malformed raw response. The rejected response is correctly
absent from ordinary artifacts, so there is no safe field-level diagnostic.

The `--no-secrets` warning also says that credential detection is disabled
without explaining that provider-response and persisted-artifact scans remain
mandatory.

## Decision

Keep the existing mandatory `secretscan.DetectAlways` scan over the complete raw
localization provider response in its current position. A detection still
rejects the batch immediately with the same reason, failure stage, and
validation code. It never reaches localization apply or cache publication.

Only after that raw scan fires, attempt the existing strict
`DecodeRussianProviderResponse` contract for diagnostic attribution. If strict
decode succeeds, scan each decoded translation text independently in exact
batch order with `DetectAlways`. Record only:

- a closed unsafe-kind code: `private_key`, `bearer_credential`, `secret_key`,
  `github_token`, `aws_access_key`, `credential_assignment`, or `unknown`; and
- the one-based batch-local translation index of the first attributable unsafe
  translation.

The translation index is zero when strict decode fails or no decoded
translation can be attributed. The raw scan's closed kind remains available in
that case. Decode failure details, raw response bytes, translated text, stable
field IDs, paths, provider endpoint, and errors are neither returned nor
logged.

Expose both values only on the in-memory localization outcome and the existing
CLI failure warning. Do not add them to the saved localization status or any
other artifact. Status, projection, cache, prompt, request, provider, retry,
acceptance, batch, locale, canonical report, Study, Architecture, Atlas, UI,
legacy, and migration contracts remain unchanged.

Clarify the runtime `--no-secrets` warning: the override disables ordinary
input credential detection, while mandatory provider-response and
persisted-artifact credential scans remain active.

## Proof

Provider-free focused tests establish that:

- a strict complete response containing a real secret in one translated value
  fails at the same mandatory scan with its exact closed kind and one-based
  batch-local index;
- a malformed raw-only unsafe response keeps the same rejection and reports
  translation index zero;
- unsafe responses are not applied, cached, or persisted in ordinary runs, and
  the saved v2 status contains neither attribution field nor provider text;
- `--dump-llm` retains its existing bounded redacted diagnostic behavior;
- ordinary decode failures retain their byte-identical CLI warning shape;
- successful and other invalid localization paths retain their existing
  behavior; and
- the `--no-secrets` warning names both the disabled ordinary input scan and
  the mandatory response/artifact scans.

All verification uses fixed local provider responses. It makes no network or
live-model request.
