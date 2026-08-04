# Decision 184: Orientation context-selection manifest

## Status

Approved by the repository owner as one provider-free observability slice after
Decision 183.

## Problem

The bounded Orientation request is deterministic, but a saved run does not
explain which locally available facts survived ranking, configured caps,
request-byte fitting, and the final candidate-file allowlist. A missing fact in
model output can therefore be mistaken for model failure even when that fact
never reached the request.

## Decision

The existing `llmbundle` selection and byte-fit path returns one versioned
selection trace together with the exact bundle it produced. The trace records
configured and final effective caps, byte-fit attempts, bounded before/after
counts, existing warning/cutoff reasons, the exact selected candidate-file rows
in canonical bundle order, and bounded samples of facts omitted at a selection
seam that can prove the omission. It never reruns ranking or reconstructs a
second selection heuristic.

The normal debug run persists that trace as
`orientation_context_selection.v1.json` after binding it to the exact compact
bundle and Decision 183 typed wire bytes. The artifact contains no provider
response, repository file contents, signal snippets outside the compact
bundle, raw full file tree, replay/development artifact, or credentials. It is
strictly decoded, bounded, secret-scanned before persistence, and deterministic
for identical inputs.

The normal run manifest contract advances to version 5, records the artifact
SHA-256, and verifies the fixed, regular, bounded artifact before exposing the
run as authorized. A model-bundle identity and Orientation selection identity
must either both be present or both be absent; only genuinely non-Orientation
or view-only shapes may omit both. The current reader rejects earlier manifest
versions. There is no legacy reader, migration, or inference from other
artifacts.

Selected composition/order, prompt/request/cache identity, provider calls,
canonical report, Study, Architecture, localization, UI, clients, retry,
flags, and `--llm-bundle-only` output remain unchanged.

## Proof

Provider-free tests establish:

- traced selected candidate rows equal the exact actual bundle rows and retain
  their canonical order;
- configured cap and byte-fit reductions expose deterministic before/after and
  omitted counts with bounded samples from the actual selection seam;
- exact compact-bundle and typed-wire byte counts/hashes match the request
  inputs without changing either input;
- identical inputs encode byte-identical JSON;
- the artifact rejects unknown versions, unsafe structure, credentials, and
  oversized input and contains no full file tree or extra source snippets;
- a normal run manifest authorizes the exact artifact hash and rejects missing,
  substituted, symlinked, or tampered bytes;
- the current version-5 reader rejects earlier manifest contracts and rejects
  an unpaired model-bundle or Orientation-selection identity.
