# 254 — Package-path context with nested exact symbols

**Status:** ACTIVE (owner-authorized, 2026-08-09)
**Preserves:** Decisions 201, 204, 206, 238, 241 and 252; one bounded
Architecture call; request-local refs; exact local Canvas/remainder authority;
many-to-many conceptual participation; response grammar and validation;
provider byte limits; and the prohibition on source-file trees, raw edges,
locations and provider-authored identity.

## Problem

The exact Linux-target Moby Architecture request advertised all 471 legal
conceptual members, but its 378 packages were represented by basename labels
and its 93 exact symbols were separated from package context in one flat list.
There were 50 package-basename collision groups covering 131 occurrences. The
model therefore lacked the package responsibility and immediate containment
context needed to distinguish many mechanically split production units.

D241 already rejected prompt-only exhaustive/cohesive coverage wording: it
alternated between package-tree mirroring and large catch-all components. A
separate assignment ledger would merely preserve the same weak interpretation
in a larger response. D254 changes only the input projection used by the
existing one-call Architecture stage.

## Provider request projection

The backend retains its complete flat `RequiredMemberRefs` and private catalog
for coverage, replay and response resolution. Only the provider JSON is
nested:

```json
{
  "ref": {"kind": "package", "ref": "p1"},
  "package_path": "v2/daemon/server/router",
  "unit_ref": "u1",
  "coverage_role": "primary_scope",
  "symbols": [
    {
      "ref": {"kind": "symbol", "ref": "s1"},
      "label": "Handle",
      "facts": []
    }
  ],
  "facts": []
}
```

For package candidates:

- `label` is omitted;
- `package_path` comes only from the exact static package declaration/import
  identity, never from presentation `Candidate.Name`;
- the exact common qualified prefix is removed when that leaves a non-empty
  suffix; otherwise the complete clean declaration is retained;
- exact package-owned symbols occur once under `symbols`, with their label,
  nonredundant facts and flow participation preserved; and
- a static package declaration fact is omitted only when its projected label
  merely repeats the package path/basename.

Other existing non-package candidates retain their prior flat provider shape
for compatibility. This focused decision adds no generic file/Bash scope.

Package and nested symbol refs remain independently legal response
`member_refs`; nesting proves containment only. It neither requires package
and symbol co-selection nor assigns them to the same component. Behavior
anchors may continue to reference a nested symbol. The provider cannot return
`package_path`, `symbols`, unit, role or parent fields.

The exact paid Moby artifacts contain 93 symbols. All 93 have an exact direct
file parent and an exact package ancestor, so all are eligible for this focused
projection with no guessed owner or fallback. The local catalog, canonical
members, response decoder and Landscape restoration are unchanged.

## Identity and compatibility

Architecture request identity advances 19 → 20. The exact prompt SHA changes
automatically from its bytes. Cache identity binds request version, request
bytes and prompt identity, so earlier entries miss closed. Response, proposal,
record, Landscape, status, report and manifest schemas remain unchanged;
`SynthesisRecordVersion` remains 16.

## Verification

Provider-free tests prove:

- Moby-like package paths derive from exact declarations rather than display
  labels;
- package labels and redundant static declaration facts are absent;
- exact package-owned symbols appear once, adjacent to their package, without
  a redundant `parent_ref`;
- nonredundant package/symbol facts and flow context survive;
- the local flat checklist and private catalog still retain every package and
  symbol ref independently;
- an absent nested package parent fails provider projection; and
- the request remains under the existing 1 MiB boundary.

No extra provider call, retry, ledger, exhaustive prompt, generic language
scope, new stage, UI change or response-authority change is part of D254.

Approved by:
    Repository owner during the 2026-08-09 Moby Architecture review, after
    rejecting both an assignment ledger and extra package-label arrays in
    favor of the smallest focused provider-request correction.
