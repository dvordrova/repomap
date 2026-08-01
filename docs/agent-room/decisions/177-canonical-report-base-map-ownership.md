# Decision 177: Canonical report and base-map ownership

## Status

Approved by the repository owner for the first bounded overnight slice.

## Problem

The ordinary saved-run reader implicitly replayed development artifacts merely
because their files existed beside canonical run inputs. Architecture also had
two competing producers: saved model synthesis in the run reader and a local
deterministic canvas created later by presentation coherence. A rejected model
proposal could therefore hide a valid local landscape, while heuristic
same-executable FlowProof associations could be presented as exact overlays.

## Decision

Ordinary report generation reads only explicit canonical run artifacts.
Semantic-discovery experiments, golden and fresh Mechanism records, onboarding
editorial records, and paved-path replay records are not implicitly applied by
`ReadRunDir`.

The canonical local Architecture Canvas is built once by the saved-run reader
from the exact local candidate bundle. Static rendering and reportserver both
consume that persisted canvas from `report.json`; presentation coherence only
joins existing product objects and never creates a second canvas. A current,
accepted model synthesis may replace conceptual grouping over the same exact
candidate bundle. Missing, malformed, or rejected synthesis remains optional
diagnostic state and cannot erase or substitute the local base map. Ordinary UI
availability depends only on the valid canvas and does not expose fallback
state or wording.

Architecture publishes zero `CandidateDirection`/`LocalProof` flow overlays in
this decision. The current proof producer can attach a seed surface through a
same-executable/first-surface heuristic, so that field is not an exact
architecture relation. Directions remain available to Study and as bounded
suggestions. A future slice may add overlays only after a producer owns an
explicit typed surface-to-operation binding; that adapter is outside this
decision.

There is no compatibility reader, migration, new flag, provider call, prompt,
cache, Study, locale, manifest, or source-authority change. Old static HTML is
not rewritten.

## Proof

- provider-free fixtures produce the same canonical base IDs with and without
  co-located development/replay artifacts;
- static rendering and reportserver serve the same persisted base canvas;
- missing, malformed, and rejected model synthesis cannot remove the base;
- CandidateDirection proofs publish no flow overlays, including proofs carrying
  a seed surface ID; structural facts remain available independently.
