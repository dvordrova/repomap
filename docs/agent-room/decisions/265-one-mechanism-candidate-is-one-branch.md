# 265 — One mechanism candidate is one branch

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-10)

**Preserves:** Decisions 243, 260, 264; exact target-root and reading
authority; refs-only responses; the three-mechanism and eight-edge ceilings;
and item-local rejection.

## Product defect

The first real Decision 264 request exposed exact target roots and eliminated
all `not_target_rooted` failures. The provider nevertheless returned four
candidates that unioned sibling continuations after the reading. Each edge set
contained useful target-rooted paths, but the union was a fork rather than the
one simple directed path required by the reducer. A fifth one-edge candidate
was correctly too short. All five candidates were therefore rejected and the
report again contained zero mechanisms.

This is a prompt ambiguity, not a graph or resource limit. Backend branch
decomposition is unsafe: each affected card expands to four paths, exceeding
the existing maximum of three and forcing a new arbitrary selection authority.

## Exact corrective

The prompt says that one candidate selects exactly one branch. Within a
candidate no selected node may have more than one selected incoming or outgoing
edge. Independently useful sibling branches must be separate candidates within
the existing limit; the provider must omit rather than union or pad them.

No request or response field, validator, reducer, graph, call count, report,
manifest or UI contract changes. The content-derived prompt identity advances
to `mechanism-study-prompt-a4099fdb0893`.

The observed card whose question named `runDefault` but whose exact reading was
`writeDefaultRunError` is a separate upstream Study-content defect. This change
does not manufacture a mechanism for it; an empty result remains correct until
Study supplies a matching exact reading.

## Acceptance

1. Prompt tests pin the one-branch and no-union requirements.
2. Existing reducer tests continue rejecting forks and one-edge candidates.
3. Full mechanismstudy tests and vet remain green.
4. The next real run is performed only after repository edits are frozen, so
   freshness failure cannot obscure the semantic result.

Approved by:
    Repository owner after requesting a fresh real call instead of preview
    work, and after the exact v3 response proved branch unions were the sole
    remaining mechanism-response validation failure, 2026-08-10.
