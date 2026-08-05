# Decision 218: Report truth corrective — stop hiding peer content, stop misdescribing evidence

## Status

ACTIVE — presentation/view-model corrective authorized by the owner goal
"repomap-hermes-d218-report-truth-corrective.txt" (revised risk roadmap pack,
2026-08-05). Provider-free. No model call, no D216 implementation, no
Surface Discovery change, no Study source-expansion redesign.

## Proven acceptance gaps (owner's revised risk review)

1. Study is a dedicated theme page but hides peer themes behind "show more".
2. Function/symbol and path text visually concatenate.
3. Package/file starting points can look like functions (reading kind not
   explicit in the public projection).
4. Overview gives too many components similar visual weight.
5. Architecture may show a graph-like four-box view with no proven
   inter-component relations.
6. "Component relations were not established" does not distinguish no
   evidence from unresolved/unprojectable evidence.
7. Repository/module units appear as low-value peer cards.
8. Architecture UI may say synthesis was not performed although the provider
   responded and local validation rejected it.
9. Raw README-derived purpose may be unsafe as a primary hero.

## Required behavior

### A. Study shows every theme

Remove theme-level "show more/show fewer" from the dedicated Study route;
render every published theme title/question in one navigable page.
Progressive disclosure may collapse card detail, reading previews, and
provenance — never peer theme existence. Keep diagnostics/frontier coverage
collapsed. A compact TOC/grouping is allowed if fully derived from existing
cards.

### B. Typed source rows

Advance the report projection so a reading/source action has a closed kind:

    function | method | type | call_site | package | file | document | boundary

Never infer a stronger kind than exact local evidence supports. Render:

    Primary label/symbol
    kind · path:line
    optional explanation

Requirements: visible spacing/line separation; symbol and path in separate
DOM nodes; long values wrap; repeated controls have distinct accessible
labels; package/file entries explicitly labeled, never presented as
functions; exact source navigation stays <= 2 actions.

### C. Overview system spine

Overview shows at most one representative card per supported role: entry /
consumption; core coordination/domain; state/resource; extension/integration;
operations/support. Target 3–5 cards, one explicit primary card when
evidence supports it. Every other component stays reachable through
Architecture; nothing deleted from report data. Never rank by array order or
canonical hash alone.

### D. Architecture representation depends on relation evidence

Closed presentation state:

    proven_component_relations
    member_relations_unprojected
    no_supported_relation_evidence

- proven relations: list/group them, optionally show canvas;
- only unprojected member relations: show exact count and why no safe
  component edge was created;
- no supported relation evidence: label content as conceptual/package
  grouping, structured list primary;
- never make a zero-edge canvas look like a runtime graph;
- package imports are structural dependencies, not execution order.

### E. Truthful Architecture synthesis state

Map exact status: uncalled/offline/unavailable → not attempted; accepted →
accepted; accepted_partial → accepted for X/Y with local remainder shown;
validation/invalid response → provider responded, proposal rejected, local
Architecture shown; output/response limit → partial response unused, local
Architecture shown; cached → accepted grouping replayed. "Not performed" is
forbidden for attempted provider calls. Primary UI uses bounded user copy;
closed diagnostic codes stay under provenance.

### F. Repository/module unit hierarchy

Repository and module units become structural headers/sections rather than
equal-weight application/domain cards when child applications or libraries
exist. Project already-available metadata only (module path, existing
counts). No new module analysis; Go version/toolchain/workspace/replace
metadata deferred to the Repository topology goal.

### G. README source truth

Until the later Repository Brief goal: raw README/documented-purpose text is
labeled source material; never repeat it as both hero and "At a glance"
answer; if it begins with warning/badge/ASCII-art/marketing residue, use a
neutral local fallback assembled from repository name, archetype, and exact
entry facts rather than claiming the text is the product purpose; preserve
original prose under disclosure; no ad hoc translation in JavaScript.

## Acceptance matrix

etcd, Telebot, Chatto, Restic, Casdoor from Archive 5, at 1440x1000 and
390x844, routes: Overview, Study, Architecture, one Study detail, one
component/source drawer.

Assert: every published theme visible without "show more"; symbol/path/kind
visibly distinct; packages never look like functions; Overview has 3–5 spine
cards when enough areas exist; all other components remain reachable;
zero-relation repos default to list/taxonomy, not graph implication; etcd
rejection says attempted/rejected/local fallback, not uncalled;
accepted_partial shows coverage counts; repository/module hierarchy clear;
README source not duplicated as authoritative hero; no horizontal overflow,
clipping, JS error, or keyboard regression; EN/RU catalog parity.

Gates: gofmt on touched Go files; `go test ./...`; `go vet ./...`;
`make build`; `node --check` on touched assets; all report, manifest,
localization, golden, and browser-matrix gates.

## Non-goals

No model call; no Surface Discovery change; no handler detector; no Study
source expansion redesign; no D216 implementation; no module Go-version
analysis; no new frontend framework; no push.
