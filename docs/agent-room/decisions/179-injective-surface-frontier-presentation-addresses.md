# Decision 179: Injective surface-frontier presentation addresses

## Status

Approved by the repository owner as a narrow RU presentation reliability
correction after Decision 178.

## Problem

The saved etcd run
`20260801-065819-etcd-de1c5c8572ff` contains distinct `SurfaceFrontier`
elements with the same kind and exact location but different visible details.
The presentation inventory derived the owner digest for located frontiers from
kind and location only. Those elements therefore received the same
presentation field address, and inventory construction failed because one
address owned conflicting canonical prose.

## Decision

Every `SurfaceFrontier` collection element receives an injective deterministic
presentation-only address. The existing kind and optional exact location remain
part of the owner digest; canonical collection order is always included as the
prose-independent discriminator. The address does not become semantic identity
or source authority, and it does not change the report collection, its order,
details, links, IDs, paths, or retrieval behavior.

The presentation localization contract advances to
`report-presentation-localization-v11` and the inventory contract advances to
`presentation-text-inventory-v8`. The presentation localization contract is
part of current cache identity; the inventory version records the corresponding
address-schema bump. Earlier localization cache entries are misses; they are
not read, migrated, or mapped. `repomap cache clear` is the explicit
whole-cache invalidation operation.

This decision changes no generic model cache, targeted research, orientation,
Study, model request shape, UI schema, report/manifest format, source link, or
canonical report data.

## Proof

Provider-free tests establish that:

- two frontiers with the same kind and exact location but different details
  receive distinct stable presentation field IDs;
- repeated inventory preparation is deterministic;
- a complete projection applies each translated detail to its original
  collection element atomically;
- inventory preparation and application leave canonical report bytes
  unchanged;
- a record under the previous localization contract is a cache miss;
- the saved etcd report now produces a nonempty 1,468-field presentation
  inventory instead of failing during inventory construction.
