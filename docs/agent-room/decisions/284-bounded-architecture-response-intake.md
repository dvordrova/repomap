# 284 — Bounded Architecture response intake before publication normalization

**Status:** ACTIVE (owner-authorized corrective, 2026-08-10)

**Preserves:** one bounded Architecture call, D228's published 12/24/48 shape,
D235's local normalization and item-local salvage, D241/D255's exact local
membership and anchor authority, request-local refs, deterministic remainder,
and every existing provider/privacy/report/UI boundary.

## Failure

A fresh GitLab CLI provider response was valid nested `member_refs` JSON with
four subsystems containing `[2, 1, 44, 20]` components, 67 total. The live
decoder rejected the 44-component subsystem and 67-component total before the
existing `Apply` normalizer could merge the useful grouping into the published
bounded shape. The same response contained one known anchor association whose
advertised members had no intersection with the resolved component members.

## Approved corrective

- Only the active nested `member_refs` decoder gains an intermediate intake:
  at most 24 subsystems, at most 100 components in one subsystem, and at most
  100 components total.
- The existing local normalization remains publication authority and reduces
  accepted input to at most 12 subsystems, 24 components per subsystem and 48
  components total. It preserves exact returned member/anchor identity while
  merging; it does not invent model membership.
- Historical flat responses and nested `unit_refs` replay remain strict at the
  published limits because their compatibility semantics are not changed.
- A returned known anchor is retained only when at least one of its advertised
  members intersects the resolved component's effective membership. An
  unbound association is dropped item-locally and recorded with the existing
  recoverable `proposal.empty_anchor_slice` diagnostic; the component and valid
  sibling anchors survive.
- The prompt is unchanged. The model is not asked to count or reproduce local
  presentation limits.

`SynthesisRecordVersion` advances 17→18 because current-record replay now
accepts this bounded intermediate shape. Request, prompt, response wire,
proposal, contract and synthesis-status identities remain unchanged. There is
no new provider call, retry, schema, source/path authority, privacy exposure,
analysis stage, report field or UI behavior.

## Acceptance

Provider-free replay of the 4-subsystem `[2, 1, 44, 20]` class must reach the
existing normalizer and publish within 12/24/48 with every exact returned member
accounted for. A 25th subsystem or 101st component fails closed. Flat and
`unit_refs` overages remain strict. An unbound known anchor never reaches the
published component, while valid membership and sibling anchors remain.

Approved by:
    Repository owner after the fresh GitLab CLI Architecture failure and the
    explicit instruction to set the intake limit to 100, 2026-08-10.
