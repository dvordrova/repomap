# Decision: Visible Study coverage publication guard

Status: Approved by the repository owner through the active overnight
continuation goal after question-aware Study selection exposed a reducer/visible
coverage mismatch.

## Product outcome

Do not lead first-time readers into Study directions whose visible projected
code/docs do not cover enough of the question terms, when enough better
directions remain.

## Approved implementation scope

1. Keep canonical Study records unchanged.
2. Add a presentation-only publication guard in `report.json` projection.
3. Use only already projected visible Study material:
   reading anchor labels/sentences/snippets, document labels/paths/excerpts,
   area labels/responsibilities, paths, and symbols.
4. Move weak visible directions to hidden debug/provenance data when at least
   three user-visible directions remain.
5. Keep the default user UI free of gap/coverage/verdict language.
6. Do not change prompts, model calls, provider, validators, repository-wide
   analysis, candidate limits, or source budgets.

## Truth boundary

Visible coverage is a local presentation-quality guard. It does not prove the
remaining visible Study directions are complete explanations. It only prevents
known weak directions from being published as normal user Study paths when the
report has enough stronger alternatives.

## Explicit non-goals

- No canonical Record version bump.
- No retry or model re-edit.
- No new renderer or UI surface.
- No hiding all Study paths when the repository only has the minimum useful
  set.

## Focused verification

- Unit tests for weak direction suppression and minimum-count fallback.
- Replay saved copied restic/repomap runs without model calls.
- Visible UI check that debug coverage remains hidden.
- `git diff --check`.
