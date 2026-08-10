# 281 — Map context and library API presentation

**Status:** ACTIVE (owner-authorized product corrective, 2026-08-10)

**Supersedes:** Decision 236's automatic Entrypoints lens selection and its
primitive lens-object wall, Decision 271's package-only library start fallback,
and the removal of the user-reachable Integrations context. It preserves the
single Canvas authority, exact module-product targets, exact source actions,
bounded evidence and provider-free presentation.

## Product defects

The module-product model correctly stopped creating one report per compiler
package, but the report did not expose the public API that justified a module
library as a product. On Telebot the exact Go facts contain hundreds of exported
declarations, while the model Architecture happens to group only the small
`layout`, `middleware` and `react` packages. The root API remains in the honest
local remainder, and a selected component falls back to `react.go:1` rather
than showing `React`, `Reaction`, constructors, methods or values.

Executable pages automatically open Entrypoints, select `main`, dim the whole
map and dump every unjoined first-hop call above it. Runtime-activity surfaces
such as worker closures are also presented as independent entrypoints. The
accepted `entry_call` family projection, which already groups the useful exact
call structure, is ignored.

Boundary/resource observations are persisted and joined to Architecture, but
the Integrations control and context were removed. Evidence joined only to the
diagnostic local remainder is therefore invisible even though exact callsites
are authorized. Finally, Study trace labels such as `telebot.v3 · NewBot` are
misparsed as `v3 · NewBot()` by the browser.

## Approved presentation contract

The desktop Map has one explicit context switch over one stable Canvas:

- module library: `API | Map | Integrations`;
- executable: `Map | Entrypoints | Integrations`.

Module libraries open on API; executables open on neutral Map. No entry group,
component or integration family is selected automatically. Merely changing
context never globally dims the map or changes its layout/transform. A later
explicit object selection may focus exact on-map participants.

### Library API

Report format 47 adds a report-owned `library_api` projection derived only from
the selected module-library target and its already persisted exact Go facts.
It contains every exported function, method, type, constant and variable up to
a deterministic global bound of 4096 declarations, grouped by exact owning
package with exact source locations and truthful total/shown counts. Package and
declaration permutations produce the same projection. The module root remains
visible even when model Architecture leaves it in local remainder.

The API context is a searchable launchpad with package/kind counts and exact
source actions. Component Summary uses the same exact package slice, so React
shows `React` and `Reaction` instead of a line-1 package fallback. This is a
presentation projection, not a new semantic stage or provider payload.

### Entrypoints

Entrypoints contains only exact `entry_surface` objects. Worker and async-task
surfaces remain local runtime evidence but are not labeled as top-level entries.
Each entry is a compact card with its exact source and an explicit `Explore`
action. Detail groups accepted `entry_call` families by caller and keeps exact
callsites behind bounded disclosure. First-hop handoffs are summarized by
mapped component area and honest off-map count; they are not rendered as one
large raw wall. Canvas overlay is activated only by that explicit action.

### Integrations

Integrations restores user access to the existing bounded boundary/resource
association projection. Paired boundary/resource observations with the same
component, generic family, owning unit and witness set are one row. Rows expose
exact source actions and state explicitly that they are observed bounded
callsites, not a complete dependency inventory. Evidence outside conceptual
grouping remains visible and labeled as such; no fake remainder node is drawn.

### Trace labels

The Study renderer removes the backend package-disambiguation prefix through
the exact ` · ` separator before applying bare-symbol formatting. Backend labels
and mechanism identities remain unchanged.

## Identity and acceptance

Report format advances 46→47 and typed UI catalog advances 26→27. Manifest 18,
Canvas 15 and provider identities remain unchanged.

Acceptance requires desktop browser evidence that:

- Telebot opens with its root and auxiliary package APIs, searchable exact
  declarations, and React component context exposes `React` and `Reaction`;
- no Study trace displays the accidental `v3 ·` prefix;
- Restic opens on a neutral map, and Entrypoints becomes focused only after an
  explicit click with compact `entry_call` grouping;
- helper workers/async tasks are not presented as top-level entries; and
- Telebot and Restic Integrations expose exact observed HTTP/filesystem rows,
  including off-map evidence, without automatically dimming the Canvas.

Approved by:
    Repository owner after visual review of fresh Telebot and Restic reports and
    the explicit request for a hard Entrypoints redesign, useful library
    functions/objects/symbols, removal of `v3`, understandable context UI and
    restored Integrations, 2026-08-10.

## Owner correction — 2026-08-10

The Canvas is the permanent Map baseline, not a peer context and therefore not
a tab. Module libraries expose only `API | Integrations`; executables expose
only `Entrypoints | Integrations`. Closing or toggling the active context
returns to the neutral Canvas without moving, relaying out or dimming it.

Library API presentation is one vertical list: short module-relative package
labels, symbol-syntax declaration actions, functions separated from methods by
receiver, and initially collapsed types/constants/variables. A compact
`What to look at first` shelf reuses only persisted Study cards and their exact
path/line readings; an exact path/line plus compatible non-empty symbol may
join a declaration, while an unmatched reading remains a standalone exact
Study source action. Integrations is likewise one flat family/unit/count list,
with exact callsites behind disclosure. This follow-up changes presentation
only: report/provider schemas and backend identity authority remain unchanged;
the typed UI catalog advances 27→28.

Approved by:
    Repository owner after the Moby desktop review: Map remains visible under
    every optional context, API must read top-to-bottom rather than as a card
    grid, and Study prose may curate existing exact API evidence but may not
    own or guess source identity, 2026-08-10.

## Entrypoint catalog correction — 2026-08-10

The Entrypoints context locally enriches each exact Canvas `entry_surface` only
when exactly one persisted Trigger Catalog record has the same stable trigger
ID and kind. It presents one vertical sequence of processes, HTTP routes, legacy
CLI commands and HTTP server starts. Exact route or command identity may point
to a locally resolved callback; registration and handler locations remain
separate source actions. Unknown callbacks and provisional command segments are
labeled as partial rather than completed or placed into an invented hierarchy.

Groups initially show at most six semantically ordered rows and make every
remaining persisted row reachable through disclosure. Duplicate route or
command labels remain distinct exact records. A static-evidence note states
that runtime reachability, order and continuation are not established. This is
presentation over already persisted local authority: it does not restore the
D273-disabled framework/Cobra producers, add a provider call, or change report
schema. The typed UI catalog advances 29→30.

Approved by:
    Repository owner after asking to restore the useful `route → handler` and
    saved CLI-command view inside the existing Entrypoints context without
    giving model output ownership of deterministic facts, 2026-08-10.
