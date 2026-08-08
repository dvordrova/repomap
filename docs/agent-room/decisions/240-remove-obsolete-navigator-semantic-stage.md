# 240 — Remove the obsolete Navigator semantic stage

**Status:** ACTIVE (owner-authorized, 2026-08-08)
**Supersedes:** the Navigator semantic-selection and product-projection clauses
of Decisions 221 and 232. It does not supersede the canonical Repository Atlas,
Surface Discovery, Architecture, Theme Study, or Decision 239's provider-request
effectiveness investigation.
**Preserves:** the one ordinary product command, local exact startup evidence,
the Map → Study product IA, the Entrypoints lens, offline behavior, bounded
provider payloads, historical git history, and the requirement that mechanisms
remain evidence-backed rather than model-invented.

## Evidence

Decision 239's fresh sqlc, Gotify, Maddy, PocketBase, Dive, and goargs runs
showed that Navigator does not discover an identity or evidence. The backend
already owns every advertised startup action; the provider returns only one
request-local action ref.

The apparent multi-action choices were deterministic extraction/classification
defects rather than semantic questions: generators, test setup programs, and
helpers competed with the primary product. More importantly, the selected
Navigator action is not a first-class object in the current Map UI:

- `#/overview` canonicalizes to `#/map`;
- the old Overview and full Atlas recommendation renderers are not reached by
  an ordinary Map report with an Architecture Canvas;
- the empty-selection Map inspector required by Decision 236 does not consume
  the Navigator product;
- the Entrypoints lens already exposes exact entry identities;
- Mechanisms are required to be per-entry/per-flow and must not depend on one
  model-selected repository entry.

Calling a provider to choose one invisible editorial recommendation is
therefore not an exploration-science MVP capability. Adding a cosmetic Map
badge merely to justify the call is explicitly rejected.

## Decision

Remove Navigator vertically from the ordinary product:

- no Navigator provider request, prompt, transport adapter, cache entry,
  semantic exchange, status, request/result artifact, or model accounting;
- no Navigator report field, manifest binding, user-facing renderer, message,
  route action, or stale Overview/Atlas recommendation branch;
- no Navigator-specific candidate gate or hidden ranking layer;
- remove the obsolete package/runtime/tests/fixtures rather than retaining an
  unused compatibility implementation in production source.

The canonical local substrates remain:

- Surface Discovery retains exact process entries and their closed roles;
- Repository Atlas retains every exact/static/non-provisional process identity
  that passes its exact symbol/evidence join, independently of model choice,
  executable role, or downstream typed-trace availability;
- the Map Entrypoints lens presents the bounded exact entry inventory and its
  categories;
- Architecture and Study continue independently;
- the current single-entry Mechanism fragment is emitted only when the exact
  local process identity is unambiguous; repositories with 2+ entries wait for
  the approved per-entry projection instead of silently choosing the first;
- Mechanism projection derives only from exact local entry/handoff evidence.

The ordinary cold provider path is now bounded to at most three semantic
calls, in order: Architecture synthesis, Theme Scout, Theme Adjudication.
Repositories without sufficient input may make fewer calls. No replacement
semantic call or new flag is authorized.

Historical Navigator behavior remains available in git history. Existing
committed replay fixtures may be migrated at their explicit replay boundary,
but current production artifacts and report schema must not continue emitting
Navigator state.

Identity advances are explicit: report format 38 removes the Navigator product
field, run manifest 13 removes its three material hashes, and UI catalog 11
removes the obsolete recommendation/status copy.

## Acceptance

- ordinary reports contain no `navigator` product field or Navigator material
  input hashes;
- ordinary run directories contain no `navigator_request`, `navigator_result`,
  `navigator_status`, Navigator cache entry, or Navigator semantic exchange;
- provider accounting and semantic stage order contain at most Architecture,
  Scout, and Adjudication calls;
- Map Entrypoints remains populated from exact local data when entries exist;
- Mechanism projection and Study do not require a selected Navigator action;
- offline runs remain provider-free;
- focused tests, full `go test -count=1 ./...`, `go vet ./...`, exact clean
  build, fresh six-repository D239 runs, then fresh etcd and casdoor runs pass.
