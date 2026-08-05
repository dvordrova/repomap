# Decision 220: Surface Discovery v2 — handlers, route registrations, honest coverage

## Status

ACTIVE — bounded analyzer/catalog corrective authorized by the owner's
revised risk review (2026-08-05, goal order item 2: "Surface Discovery v2 —
handlers, route registrations, and honest coverage"). Builds on D218; does
not implement D216 and does not change the report shell.

## Proven acceptance gaps (reproduced on Archive 5 runs)

1. **Casdoor** registers its API through Beego (`web.NewNamespace`,
   `NSInclude`, `NSRouter`); the current catalog only matched 2 net/http
   route records with unresolved dynamic identity. The real router
   (`routers/router.go`) is invisible to the catalog.
2. **Chatto** serves connect-go handlers (`connectrpc.com/connect`) through
   its own command dispatcher; Surface Discovery reports **0 routes, 0
   servers, 0 unresolved handlers** — the entire HTTP surface is invisible
   and the report reads as "no handlers exist".
3. **Verb methods are matched by name only**: gin/echo catalog entries cover
   `Handle`/`Add` but not `GET`/`POST`/`PUT`/`DELETE`/`PATCH`/`HEAD`/
   `OPTIONS`/`Any`, so verb-style registrations are missed.
4. There is no generic typed registration detector: a repository-local
   method whose signature is `(path string, handler func/…/http.Handler)`
   produces no route record unless an exact catalog seed names it.
5. The UI has no bounded copy for "no exact handlers resolved under this
   analysis coverage; N candidate registrations remain unresolved" — the
   empty case must not read as proof of absence.

## Product goal

Find exact HTTP handlers, route registrations, and server starts across
supported frameworks **and** unknown-but-typed registration shapes, with
explicit candidate/unresolved frontiers and exact coverage/limit/omission
counters, so a repository's request surface is never silently invisible.

## Required implementation

### A. Framework adapters (catalog)

Add bounded typed seeds for the registration shapes of:

- **Beego** (`github.com/beego/beego/v2/server/web`): `NewNamespace`,
  `NSNamespace`, `NSInclude`, `NSRouter`, `Namespace` methods.
- **connect-go** (`connectrpc.com/connect` + `connect-go` generated
  `*connect.Handler` and `ServeMux.Handle`): registration of typed handlers
  on a mux.
- **gin** verb methods: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `HEAD`,
  `OPTIONS`, `Any`, plus `Group` prefix propagation.
- **echo** verb methods: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `HEAD`,
  `OPTIONS`, `Any`, plus `Group`.
- **chi** (`github.com/go-chi/chi/v5`): `Get`, `Post`, `Put`, `Delete`,
  `Patch`, `Head`, `Options`, `Method`, `Route`, `Group`.
- **gorilla/mux**: `HandleFunc`, `Handle`, `Path`, `PathPrefix`,
  `Methods`.
- **fiber** (`github.com/gofiber/fiber/v2`): `Get`, `Post`, `Put`,
  `Delete`, `Patch`, `Head`, `Options`, `Add`, `Group`.

Seeds carry exact symbol identity (package path, name, receiver where
applicable) and typed argument projections (path index, handler index,
method constant where the verb is not part of the method name, dispatcher
index). No ad-hoc string matching.

### B. Generic typed registration detector

When a repository-local call does not match a catalog seed but the target
method's signature is a **typed registration shape** — first string
parameter (path) and a handler parameter whose type is one of the closed
handler kinds (func, func(http.ResponseWriter,*http.Request), http.Handler,
context handler interface) — record a route with:

- kind `http_route`, certainty `static`;
- resolution `exact` when the path argument is a known constant and the
  handler argument resolves to a repository-local symbol, otherwise
  `dynamic` with a bounded frontier;
- producer `typed_registration_detector`, discovery basis
  `typed_signature_registration`;
- the same wrapper/ownership/coverage accounting as catalog matches.

The detector must not double-report calls already claimed by a catalog
seed, must not treat library-internal calls as repository registrations,
and must not infer a verb when the method name and arguments do not
establish one.

### C. Route-table and descriptor shapes

Keep the existing returned-route-descriptor extraction; extend it to
repository-local **route-table slices** (a function returning a slice of
route descriptor literals consumed by a repository-local registration loop)
when the element type and fields are exact and bounded. Record each
descriptor element as an `http_route` with `resolution exact` when path and
handler are constants. Preserve the existing max-returned-descriptors
bound.

### D. Explicit frontiers and honest counts

- Every call to an eligible typed registration shape whose handler does not
  resolve to an exact repository symbol is recorded as
  `unresolved_handlers` (candidate), never silently dropped;
- `possible_registrations` counts route records whose resolution is not
  exact;
- coverage gains per-framework matched counts (`framework_*_matched`) and
  an exact `unresolved_handler_candidates` list is preserved in the
  coverage artifact (bounded);
- the scope statement names the detector and the new adapter set.

### E. UI: bounded zero-handler copy

When the run found no exact HTTP handler and no exact server start:

- the surfaces area shows: "No exact handlers resolved under this analysis
  coverage; N candidate registrations remain unresolved." with N from
  `unresolved_handler_count` (or "no candidate registrations were found
  under current coverage" when N is 0);
- never "No handlers exist";
- candidates/frontiers remain under a collapsed provenance disclosure.

## Acceptance matrix

Archive 5 fixtures at 1440x1000 and 390x844 (Overview, Study, Architecture,
Study detail, source drawer):

- Casdoor shows Beego-namespace route records (exact identity where the
  router constants allow) instead of 2 anonymous dynamic unknowns;
- Chatto shows connect-go handler registrations and a server start with
  exact counts, or an explicit bounded frontier when identity is dynamic —
  never an empty surface reading as "no handlers";
- etcd/Telebot/Restic/Casdoor totals do not regress (no previously exact
  record becomes dynamic);
- generic detector records are labeled with producer/basis and do not
  duplicate catalog records;
- zero-handler copy is honest; EN/RU parity; no horizontal overflow or JS
  errors.

Gates: gofmt on touched Go files; `go test ./...`; `go vet ./...`;
`make build`; `node --check` on touched assets; report, manifest,
localization, golden, replay, and browser-matrix gates; provider-free.

## Non-goals

No handler execution model; no third semantic stage; no model call (the
review's "optional model prioritization only for unresolved candidates"
remains a future optional stage, never an authority and not implemented
here); no D216; no shell entrypoints; no push.

## Implementation record (2026-08-05, per «го»)

### Delivered (working tree, not committed)

- **B — generic typed registration detector** (`internal/surfacediscovery/analyzer.go`):
  `typedRegistrationShape` recognizes the closed shape (string path, handler)
  where the handler is a plain func, `http.Handler`, `http.HandlerFunc`, or a
  cataloged framework `HandlerFunc` (gin/echo/chi). `recordTypedRegistration`
  writes an `http_route` with `Producer: typed_registration_detector`,
  `Resolution: exact` when path and handler resolve, `dynamic` + bounded
  frontier otherwise. Verb method names (GET/POST/PUT/DELETE/PATCH/HEAD/
  OPTIONS/Any) set the route method exactly; other names never guess.
  Catalog seeds and convenience wrappers that call a catalog seed win —
  the detector never double-reports (echo `GET`→`Add`, gin `GET`→`Handle`
  keep exact method/prefix via existing wrapper propagation).
- **A — adapters**: new `beego.json` seeds (`NewNamespace`, `NSNamespace`,
  `NSInclude` → `http_route_assembly` frontier — controller-bound paths are
  never invented); connect-go `ServeMux.Handle(pattern, http.Handler)` is
  covered by the generic detector (fixture `connect_go`).
- **D — coverage**: `SurfaceCoverage` gains `typed_registration_detector_matches`
  and `framework_matched` (per-framework counts); scope statement names the
  detector; `unresolved_handlers`/`possible_registrations` unchanged.
- **E — UI**: `main.overview.surfaces.zero_handlers_candidates/none` EN+RU;
  Overview shows the honest zero-handler copy when a surface analysis ran
  with no exact handlers and no candidate-free claim.
- **Fixtures/tests**: `custom_router` contract updated (typed route now
  exact + server root retained); new `beego` and `connect_go` fixtures and
  tests; catalog count 20→23.

### Gate results

- `go test ./... -count=1`: **74/74 ok, EXIT 0** (`/tmp/d220-full-test.log`)
- `go vet` (changed packages): 0; `gofmt -l` on touched Go files: 0
  (model.go formatted); `node --check` × 2 assets: OK
- `make quality-check`, `make localization-check`: ok
- `TestWriteReportHTML_Golden` regenerated + passing
- Browser: chatto 1440 Overview spine/units render; 390px Overview/Study/
  Architecture: zero overflow
- Provider-free: all replay/fixture gates run without credentials
