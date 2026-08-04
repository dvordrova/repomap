# Decision 185: Local operational likely entrypoint

## Status

Approved by the repository owner as one narrow producer correction after
Decision 184.

## Problem

Local operational-flow discovery intentionally leaves
`OrientationCandidate.EntrypointPackage` empty when static evidence does not
prove an executable relation. The candidate still owns exact, allowlisted
`OpenFiles`. After Decision 183 removed provider-response entrypoint repair, the
local merge copied that empty package into a newly appended `CandidateFlow` and
caused the whole otherwise valid Orientation result to fail with a missing
`likely_entrypoint`.

## Decision

Only the local operational-flow producer is corrected. When it appends a new
local `CandidateFlow`, it retains a non-empty exact `EntrypointPackage` as
before; otherwise it uses the first exact existing `OpenFile` already owned by
that candidate as `likely_entrypoint`. If neither exists, ordinary whole-report
validation still rejects the result.

The correction does not repair, reinterpret, or normalize any provider/model
flow. A model flow that already exists under the same name retains its exact
provider-resolved `likely_entrypoint`, including an invalid empty value that
must still fail closed. No lexical/path parsing, fuzzy matching, package
inference, filesystem search, or provider-response fallback is introduced.

Candidate composition, order, names, IDs, evidence, confidence, model
acceptance, prompt/request/cache contracts, Study, Architecture, localization,
UI, reports, manifests, retries, clients, flags, and `--llm-bundle-only` bytes
remain unchanged.

## Proof

Provider-free focused tests establish that:

- existing provider flows remain byte-for-structure unchanged during local
  merge;
- one newly appended local operational candidate with an empty
  `EntrypointPackage` receives its first exact owned `OpenFile` as
  `likely_entrypoint` while files, order, and evidence remain unchanged;
- a matching invalid model flow is not repaired and whole-report validation
  still rejects its missing `likely_entrypoint`;
- a typed provider response missing `likely_entrypoint_ref` remains rejected.
