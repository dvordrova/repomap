# Decision 183: Orientation request-local typed references

## Status

Approved by the repository owner as one narrow production Orientation model-
contract slice after Decision 182.

## Problem

The Orientation provider currently copies repository paths, path-bearing
evidence prose, and long candidate-file IDs into its response. Local parsing
then recognizes, normalizes, drops, or repairs those copied values. This asks a
probabilistic producer to preserve canonical repository identity and leaves the
wire boundary dependent on path grammar and recovery policy.

## Decision

The backend derives one deterministic request-local private reference catalog
from the already bounded Orientation bundle. The file table covers the union of
every model-visible concrete file location, not only research candidates. Exact
sorted file records receive short `fNNNN` handles and exact sorted evidence
records receive separate `eNNNN` handles. The catalog digest binds the complete
canonical bundle and private file-handle to optional candidate-ID mapping.

The provider does not receive a second copy of that inventory. One compact
Orientation-specific wire projection replaces repository-bearing identities in
place: one `file_index` owns each concrete path exactly once,
`candidate_file_index` replaces its long ID and path with `file_ref`, raw
`allowed_paths` is omitted, and existing signals, entrypoint anchors/open files,
command traces, orientation candidates, and edges carry refs rather than copied
paths or restated evidence. Fact and candidate composition/order remain the
already bounded bundle's exact composition; the projection introduces no cap or
second byte-fitting pass.

The Orientation response is a small decision AST and uses:

- one file handle for every `first_files_to_open` item and likely entrypoint;
- file-handle lists for candidate-flow files;
- evidence-handle lists for map, flow, and domain-word support;
- file handles for research-question candidate selection.

The local resolver restores the existing canonical `orientationPart`, including
exact paths, deterministic evidence statements, and current candidate IDs,
before LocalProof, confidence gates, targeted planning, report persistence, or
other consumers run. Local operational candidates remain locally merged under
their existing policy.

The provider does not echo the private catalog digest, reference-contract
version, cache-contract version, or any other backend service field. Those
identities stay backend-owned and bind the exact request/cache envelope rather
than becoming probabilistic model output.

Handle lookup is exact and kind-specific. Unknown, wrong-kind, duplicate,
shortened, prefixed, or substituted references reject the whole provider
response. There is no unique-prefix, fuzzy, regex-path, entrypoint-
from-file, or evidence-from-prose repair on this provider-response path.
Provider prose is explicitly non-authoritative and is neither parsed nor
lexically classified as a path or ID. Repository evidence text and locations
are materialized only from exact evidence refs by the backend and do not pass
through the legacy evidence-path grammar again. A union file ref whose private
record has no candidate ID remains valid for bounded facts and navigation, but
fails if used as a research candidate. Because an absent file has no reference, the
provider wire contract no longer proposes `unverified_paths`; the canonical
field stays present and empty for downstream compatibility.

The Orientation prompt and backend-owned response/cache contracts advance
together. The generic Orientation cache fingerprint binds the exact provider
request bytes and private catalog digest, including the response contract and
canonical candidate-ID mapping. An exact provider-visible request whose private
mapping differs is therefore a miss. Older entries are misses and `repomap
cache clear` remains the explicit whole-cache invalidation operation. There is
no reader, fallback, or migration.

Candidate composition, ordering, confidence gates, targeted planner semantics,
local operational candidates, LocalProof, allowed-path and filesystem
authority, manifests, canonical report DTOs, UI, Study, Architecture fan-out,
localization, clients, retries, and flags remain unchanged.

## Proof

Provider-free focused tests establish:

- identical private catalog and wire bytes for equivalent inputs whose local
  map order differs;
- identical bounded candidate/fact counts, one occurrence of each concrete
  file path in the wire inventory, a measured bounded request delta, and no
  projection-time shrink or lost candidate;
- exact round-trip restoration of canonical files, evidence, and targeted
  candidate IDs;
- fail-closed unknown, wrong-kind, duplicate, prefix, shortened,
  substituted, and raw-path responses;
- equal accepted candidate count, order, confidence, and targeted selected
  paths for semantic-equivalent canonical and typed-response fixtures;
- zero provider calls on an exact valid hit, a miss after prompt or private
  catalog identity changes even when provider-visible wire bytes are equal,
  and Decision 182 invalid-hit eviction plus one live recomputation;
- a model-visible entrypoint/open file outside `candidate_file_index` resolves
  as a fact ref but is rejected as a research candidate;
- the typed provider-response path has no regex/path-like prose parsing or
  unique-prefix repair.
