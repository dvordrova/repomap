# Decision 188: persisted Orientation bundle selection identity

## Status

Approved by the repository owner in the current session as the immediate
reliability checkpoint after Decision 187. Decision 187 and earlier historical
checkpoints are not rewritten.

## Problem

Orientation context selection hashed the compact model bundle before the debug
writer persisted it. The writer then applied mandatory artifact redaction. A
bundle containing a credential assignment could therefore be replaced by a
safe redaction marker while the adjacent selection artifact still described
the unsaved pre-redaction bytes. Manifest verification correctly rejected the
mismatched identities, including the supported `--no-secrets` run where
runtime scanning is disabled but debug artifacts remain redacted.

## Decision

The existing private prepared-primary-plus-sidecar writer pattern is shared by
Orientation reports and model bundles. It prepares the primary bytes exactly
once, applying mandatory redaction when configured, passes those exact bytes to
the producer callback, and persists the same primary bytes with the derived
sidecar. `WriteOrientationReportWithSidecar` remains a thin wrapper and a thin
LLM-bundle wrapper owns `llm_bundle.json`.

Orientation constructs and encodes `orientation_context_selection.v2.json`
inside the LLM-bundle sidecar callback. The artifact preserves the canonical
compact-bundle SHA-256 and byte count that describe the exact bytes selected
before typed provider projection and measured by the byte-fit trace. Separate
persisted-bundle SHA-256 and byte-count fields bind the exact post-redaction
bytes actually written as `llm_bundle.json`, including a safe non-JSON
redaction marker when mandatory
credential detection replaces the entire artifact. The already-built bounded
Bundle remains the semantic source for selection rows and counts, while the
typed provider wire must remain valid JSON and retains its existing exact
identity.

The earlier pre-write selection path is removed. Run manifest v6 verifies the
saved model bundle against the new persisted identity and no longer trims a
writer newline to read the old identity. Selection v1 and run manifest v5 are
rejected. There is no legacy reader, fallback, rewrite, or migration.

Mandatory artifact redaction, `--no-secrets` runtime semantics, compact bundle
semantics, typed provider wire, prompts, requests, cache identity, candidate
composition/order, canonical report and manifest authority, Study,
Architecture, Atlas, report UI, localization, clients, provider retry, flags,
and live-model policy remain unchanged.

## Proof

Provider-free tests establish that:

- a redaction-changing model bundle gives the sidecar callback byte-for-byte
  the saved bundle, and the sidecar keeps distinct exact canonical/request and
  persisted SHA-256/length identities;
- a non-redacted model bundle remains byte-identical and its two identities
  match;
- the exact `--no-secrets` credential-assignment regression produces a valid
  authorized manifest without leaking the credential;
- tampering with the saved model bundle is still rejected;
- a selection bound to pre-write bytes is rejected rather than accepted through
  the removed newline compatibility path;
- selection v1 and run manifest v5 are rejected without migration; and
- the full repository and nearby etcd provider-free checks pass.
