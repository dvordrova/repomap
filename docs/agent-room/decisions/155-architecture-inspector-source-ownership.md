# Decision 155: Keep architecture code starts owned by the selected component

## Status

Active corrective, authorized by the repository owner after a real report
opened an unrelated location from an Architecture component's **Start in
code** action.

## Attributable failure

A deterministic behavior anchor may describe a family whose exact members are
split across several conceptual components. Its saved `location` is one
representative member, not the canonical start for every component that
references the family.

The browser projection mixed that representative location with each
component's own exact member locations, sorted all locations by path and line,
and selected the first one. This made unrelated components open the same
lexicographically early source location and also allowed the foreign file to
join unrelated Study directions.

## Corrective contract

- A component with an exact located member starts at one of its own members.
- A broad family-anchor representative does not enlarge that component's file
  set or Study joins.
- A component without a precise member may use an anchor only when the anchor
  is inside its exact package/file set.
- An anchor-only component retains its existing anchor fallback.
- Component IDs, architecture data, Study data, source authority, report
  format, HTTP behavior, and provider requests stay unchanged.

## Acceptance

- a provider-free fixture gives two components the same family anchor and
  distinct exact members;
- each component starts at its own exact member and excludes the foreign
  representative file;
- an anchor-only component still opens its anchor;
- a real small-report audit confirms every serialized path, line, range, and
  saved source line against the captured checkout;
- human-readable report strings contain no corrupted or suspicious trailing
  fragments;
- focused tests, `./scripts/check.sh`, `./scripts/etcd_check.sh ../etcd`, and
  `git diff --check` pass.

No provider call, new analysis layer, cache change, wire change, or UI redesign
is part of this corrective.
