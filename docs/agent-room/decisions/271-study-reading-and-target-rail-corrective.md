# 271 — Study reading and target rail corrective

**Status:** ACTIVE (owner-authorized white-box UI corrective, 2026-08-10)

**Preserves:** Decision 269 sibling-page authority, Decision 256 source-trace
authority, the exact saved Study readings, target navigation v1, Report45,
Manifest17, Canvas, every persisted schema, every route and every
provider/backend contract.

## Product defects

Study rendered a reading's bare symbol and exact source location as adjacent
inline nodes without visible spacing. A root `main` reading therefore appeared
as the false identifier `mainmain.go:36`; the neutral non-link fallback had the
same defect.

The generic `.rm-tab` no-wrap rule overrode the target rail's intended wrapping,
so a long Go import path escaped the fixed-width rail. The legacy one-target
fallback also used the full package import path even though its exact
module-relative `package_dir` was available.

A `direct` role pill was repeated on every reading in cards where every reading
was direct. It made no distinction. A `prepared_investigation` with no accepted
mechanism also rendered an empty trace heading and generic explanation after
the exact readings, implying additional content where none existed.

## Approved corrective

The existing source action and its exact non-link fallback use an inline-flex
row with a visible gap and wrapping. The symbol and `path:line` remain separate
DOM nodes inside the same source action; neither evidence nor source authority
changes.

The target rail's target-link rule overrides the generic tab no-wrap rule and
width-bounds the link. The one-target fallback displays the exact
module-relative `package_dir` when available, while the accessible `title`
retains the full canonical `package_path`. Multi-target display paths and
ordinary sibling links remain unchanged.

Study shows reading-role pills only when direct and supporting readings coexist
within the same card, including its distinct alternate readings. Direct-only
and supporting-only cards omit the redundant pill without changing the saved
role.

Study renders a mechanism section only for an accepted non-empty mechanism.
For `prepared_investigation`, the exact readings remain visible and no empty
trace heading, generic fallback paragraph or inert trace container is rendered.
The saved outcome and diagnostics remain unchanged.

This removes the now-unreachable generic prepared-trace copy and adds no copy,
route, source mode, provider call, data projection or compatibility reader.
UI catalog identity advances to UI24; every report, manifest, Canvas and
provider identity remains unchanged.

## Acceptance

- A Casdoor-shaped root reading renders `main` and `main.go:36` with visible
  separation in both exact-action and non-link fallback modes.
- A legacy one-target Casdoor rail displays `.` inside the rail, exposes
  `github.com/casdoor/casdoor` as its title, and wraps despite the global tab
  rule.
- A direct-only card has no role pill; a mixed direct/supporting card retains
  both role labels.
- A prepared investigation returns no mechanism section while the card's exact
  readings remain in place; an accepted mechanism still renders its trace.

Approved by:
    Repository owner during white-box review of the fresh Casdoor report,
    2026-08-10.
