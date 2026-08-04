# Decision 193: Grounded Orientation direction publication

## Status

Approved by the repository owner through the active supervisory instruction as
the reliability checkpoint after Decision 192.

## Problem

The local Orientation disposition reducer currently accepts a model candidate
when it has an existing local proof, producer-owned verified evidence, or only
a confidence value of at least `0.4`. The confidence-only branch can therefore
publish provider prose as a normal user direction even when the local pipeline
has no exact verification for it.

Rejected candidates are retained intentionally for bounded diagnostic detail,
but the report component reducer also matches every candidate direction by
path/evidence overlap and lexical fallback without checking its disposition.
That allows rejected provider and local source-signal candidates to reappear as
ordinary component `related_flow_ids` and fallback anchors.

## Decision

Orientation accepts a candidate direction if and only if it already has a
local `LocalProof` or its producer-owned `LocalVerification.Verified` collection
is non-empty. Provider confidence remains available for ranking and diagnostic
display, but it is not publication authority. An unaccepted candidate receives
a closed rejection reason that truthfully states that exact local verification
is absent regardless of its confidence.

The report component backend excludes every explicitly rejected direction from
both exact path/evidence matching and the component-name lexical fallback.
Accepted directions remain eligible under the existing ordering and matching
rules. When an accepted direction already carries a typed local proof, its
existing `SeedSurfaceID`, `TraceEvidenceSurfaceIDs`, anchors, and transitions
remain intact; this decision adds no new Surface projection or repair path.

Raw Orientation candidates, candidate order, rejected diagnostic details, and
the existing collapsed rejected section remain unchanged. This decision does
not change prompts, requests, provider calls, confidence gates, candidate
production, source-signal aggregation, schemas, caches, Study, Architecture,
Atlas, localization, UI, README or edge caps, retry behavior, flags, legacy
readers, or migrations. It adds no lexical heuristic, fuzzy repair, fallback,
or live-model path.

## Proof

Provider-free focused tests establish that:

- a high-confidence candidate without local proof or verified evidence is
  rejected with the closed truthful reason;
- an existing typed `LocalProof` is accepted without mutating its seed or trace
  evidence Surface identities;
- non-empty producer-owned `LocalVerification.Verified` remains sufficient for
  acceptance;
- rejected provider and `local_source_signal_aggregate` candidates enter
  neither component `related_flow_ids` nor lexical fallback anchors; and
- an accepted typed direction remains eligible for the existing component
  relation and fallback behavior.

All verification uses in-memory fixtures and saved provider-free candidate
shapes. It makes no provider request.
