# Current approved implementation decision

Active decisions (each approved by the repository owner via its supervisory goal):

1. decisions/215-etcd-architecture-output-exhaustion-isolation.md
2. decisions/217-report-ui-ux-acceptance-repair.md
3. decisions/218-report-truth-corrective.md
4. decisions/220-surface-discovery-v2.md
5. decisions/221-overview-truth-first-action.md
6. decisions/222-architecture-truth-model-relations-ux.md
7. decisions/223-architecture-wire-aggregated-unit-edges.md
8. decisions/224-study-content-integrity-normalization.md
9. decisions/225-component-boundary-resource-association.md
10. decisions/226-mechanism-evidence-contract.md
11. decisions/227-cross-cutting-member-participation.md
12. decisions/228-loosen-architecture-caps.md
13. decisions/229-product-projection-vertical.md
14. decisions/230-archive8-interaction-corrective.md
15. decisions/231-architecture-model-backend-boundary.md
16. decisions/232-navigator-theme-contract-simplification.md
17. decisions/233-study-equivalence-overview-prominence.md
18. decisions/234-canvas-interaction-tls-bias.md
   (decisions/219-study-content-integrity-deferred.md is DEFERRED — superseded
   in priority by 218 per the owner's revised risk review; the pending change
   set is preserved at /tmp/d218-01-study-content-integrity-pending.patch)

## Decision 215

Status:
    Decision 215 active: attempted Architecture output/response resource
    exhaustion (finish_reason=length or response-byte overflow after the
    provider call was attempted) is isolated as an optional publishable
    failure — durable failed status v9 with the closed provider_output_limit
    code, exact failed-call accounting, one redacted exchange, and
    continuation to Study + report/manifest with the canonical local Canvas.
    D194's whole-run termination is superseded for exactly this clause; every
    other stage and every pre-call limit stays terminal.

Approved by:
    Repository owner via the active supervisory goal
    (repomap-hermes-d215-etcd-failure-isolation-goal.txt, 2026-08-05).

Notes:
    The etcd incident (run 20260805-064730-etcd-4d99f0f8a558) proved a
    visible 56-ref generation repetition loop (6,390 package refs, 90 unique,
    finish_reason=length at 64,000 output tokens), not capacity shortage, so
    the global 64,000-token ceiling is deliberately retained (no 128k). The
    partial response stays diagnostic-only: never parsed, applied, cached, or
    presented. Status version advances 8→9 with bounded configured/observed
    token evidence and no partial-response publication. D216 (bounded local
    Architecture units before one unit-grouping semantic call) is recorded as
    the next proposed root correction, not implemented. Provider-free
    acceptance: full CLI replay on the real etcd repo with a deterministic
    loop fixture passes — failed status + accounting durable, no synthesis
    record, Study executes, report + manifest publish, exit 0.

    Decision 214 builds the first slice of locally observed Boundaries and
    Resources on top of the accepted Atlas-first base. The Atlas schema
    already defines EntityBoundary and EntityResource (repositoryatlas/
    model.go:34-35) but the Go adapter emits only surface+operation entities
    (goadapter/adapter.go:318-321) — the accepted casdoor product proves the
    gap: the Atlas contains zero occurrences of http., database/sql, sql.,
    redis, mysql, postgres, grpc, smtp, os.file, ioutil while the D213 Study
    cards correctly claim DB lifecycle (object/ormer.go), SOCKS5 proxy
    (proxy/proxy.go), TLS certificates (certificate/) — source-true but not
    Atlas-proven. The model layer knows; the local evidence layer is blind.
    The slice is bounded (red-team P1-1) to exactly two boundary classes:
    persistent storage and outbound network clients, observed at exact AST
    call sites (the gofacts/entrypoints.go:48 findMainFunctionAnchors
    pattern) — NOT import classification (which already exists). The
    differentiator is exact call-site evidence: (path, line, column,
    enclosing symbol) bound to the boundary entity. Message publish/consume,
    caches/locks, config/secrets, OS boundaries, and Overview boundary canvas
    rendering are explicitly deferred (recorded list, not silently dropped).
    Emission is additive into the existing Go adapter: EntityResource for
    the storage/network target, EntityBoundary for the typed operation
    class, each with exact evidence observations — no new artifact family,
    no new semantic stage, no provider call. The D213 Study Scout consumes
    the new entities as additional exact seeds. No silent truncation
    (red-team P1-2): every observed boundary call site is either published
    or recorded in a closed wire-visible omission; the D213 seed-pack
    omission wart (seed_budget count=5 hidden as omitted:0 on the wire) is
    recorded as a separate corrective, not inherited. Fixed detector list in
    code, not a plugin registry (red-team contract condition). Privacy:
    producer is local/deterministic, the import graph stays in gofacts and
    never reaches any wire, canonical Atlas IDs never reach the model, no
    legacy adapter/migration, old Atlas versions fail closed. Acceptance is
    provider-free first (go test ./..., go vet ./..., detector contract
    tests, offline casdoor + service runs with typed boundary entities, D213
    shelf still rendering, make build → PATH binary), then exactly ONE fresh
    live Casdoor semantic A/B run (owner credentials in the zsh session,
    `repomap cache clear && repomap --github-url ... --no-secrets --lang ru`)
    judged by M1–M9 material improvement: the Atlas must show observable
    storage/network boundaries with exact source and the Study must cite
    them. Acceptance, never a tuning loop, no second live calibration.

    Decision 213 restores the editorial multi-anchor Study theme layer on top of
    the accepted Atlas-first base. One new deterministic local package
    `internal/themestudy` produces a flat names-only `f*` file vocabulary, bounded
    exact `a*` seed-anchor source packs, and two new versioned semantic stages —
    Theme Scout then Source Review / Theme Adjudication — separated by local
    source expansion, followed by a deterministic local reducer into clean Study
    theme cards plus the existing exact-source drawer. The single-stage atlas-study
    provider call is retired (its local compile remains the exact seed producer),
    so the Study pipeline has exactly two semantic stages; Navigator is untouched.
    The report projection advances AtlasStudyReportProjectionVersion 7→8 and
    CurrentFormatVersion 30→31; RunManifest v11→v12 binds eight new theme artifacts
    by SHA-256. D212's four-stage local browse is kept (never three states) and
    re-based onto the D213 pipeline: considered / seed-advertised / scout-anchored
    / published, with the distinct failed-run neutral label "Local question" ·
    «Локальный вопрос» preserved. Old v7 projections fail closed under the v8
    binary; historical self-contained reports remain untouched. Acceptance is
    provider-free first, then exactly one fresh Casdoor semantic A/B run on the
    same revision and dirty state judged by the M1–M9 material-improvement criteria
    — acceptance, never a tuning loop, no second live calibration. One decision per
    Monster run: after D213 PASS write MORNING.md and stop.

    Decision 212 makes the complete D211 considered-span set explorable in the
    Study tab through one distinct local "Study questions" surface, provider-free
    and derived only from already-validated local artifacts: zero new analysis,
    zero provider calls, zero artifact/wire/cache/request/result/status change.
    The report projection adds exactly `FrontierBrowse{Total,Shown,Spans}` with
    `Span{Ordinal,Title,Question,Stage,Source,Endpoint,DirectionID}`;
    `DirectionID` is the public manifest-relative report direction id (matching
    the `study_map` direction id used by `openStudyDirection`, script.js:4398),
    derived at projection time from the validated `result.Directions` array order
    (model rank) and present ONLY on accepted rows — no canonical span ID is
    serialized; an accepted row with no matching published direction (should not
    occur — fail closed) renders without the link. `Stage` is the four-value
    membership (considered/advertised/model_selected/accepted) computed by exact
    set arithmetic over the rebuilt input (`BuildAtlasStudyInput` +
    `ValidateRequestRecordAgainstInput`, atlas_study.go:1613–1618), the request
    catalog's advertised `RefRouteSpan` refs, `result.ModelSelectedSpanRefs`
    (artifact.go:80, rejected siblings included) and `result.Directions[].Span`.
    The browse is computed ONLY inside `readAtlasStudyReportProduct`
    (atlas_study.go:1572): in the accepted/accepted_partial branch after result
    decode + `ValidateResultRecordAgainstInput` (after :1657), with the chain
    accepted ⊆ model_selected ⊆ advertised ⊆ considered re-verified, every
    accepted row's `DirectionID` resolving to exactly one published direction
    whose span matches, and per-stage tallies over the FULL pre-truncation row
    set equal to the four status counts (68/32/10/10 on casdoor) enforced
    fail-closed; a status `failed` run renders a separate neutral local-question
    browse (Total from input count) exempt from accepted-stage tallies;
    unavailable/prepared/uncalled states carry no browse. The four user-language
    stage states, never three (STATE-001), are used ONLY in
    accepted/accepted_partial runs: (a) "Model pick" · «Выбор модели» (locally
    accepted; links to the numbered direction card via `DirectionID`), (b) "Picked
    by the model, rejected by local checks" · «Выбрано моделью, отклонено
    локальными проверками» (model_selected ∖ accepted, rendered only when
    accepted_partial), (c) "Shown to the model, not picked" · «Показано модели,
    не выбрано» (advertised not_selected — neutral wording, never
    "reviewed"/"rejected" as a per-span model verdict, AUTH-001), (d) "Local
    question — not shown to the model" · «Локальный вопрос — модели не
    показывался» (considered ∖ advertised). In the failed state rows carry a
    distinct neutral label WITHOUT the "not shown to the model" suffix — "Local
    question" · «Локальный вопрос» — because the advertised subset WAS included
    in the sent request (the request artifact exists in failed runs;
    `readAtlasStudyReportProduct` requires request+status for a failed state,
    atlas_study.go:1602–1603); the template keys this neutral label off
    `state == failed`, not off the Stage enum value alone, and the
    failure-banner-headed distinct surface is preserved. A visible "not a model
    ranking" caption states that local order is stage group + canonical span ID
    (locale-independent, byte-identical across EN/RU); the raw
    `advertised_budget` enum chip becomes a human bilingual sentence, the 12
    omission representatives become the first clickable rows of the Local group,
    and "Show all N" · «Показать все N» reveals the embedded group client-side
    (the report is already a JS app; no reportserver slice). Boundedness:
    `MaxAtlasStudyBrowseSpans` = 256 with truthful Total/Shown and deterministic
    first-N in canonical span-ID order when considered exceeds the ceiling; the
    complete set stays bound by the existing CandidateSHA256 digest. Per-row
    `Source`/`Endpoint` are published only for paths in `OpenablePaths` with an
    explicit neutral unavailable state for rows whose source cannot open (no dead
    buttons; `renderStudySourceAction` silently skips today, script.js:4761).
    No per-row Role, no canonical IDs, no package buckets, no raw edges in the
    projection (UX-001 + D211 exclusion; the v6 per-role histogram stays the only
    role surface). Version delta is `AtlasStudyReportProjectionVersion` 6→7 ONLY
    (report.go:30) because the report status JSON contract genuinely gains a
    bounded per-span slice while request v7, prompt v13, result/status v8,
    accepted-cache v7 (atlas_study_runtime.go:22), RunManifest v11
    (manifest.go:32) and CurrentFormatVersion 30 (report.go:28) serialize no
    browse bytes and stay unchanged; old v6 projection runs FAIL CLOSED under the
    v7 binary (projection gate manifest.go:682–688) and only already-rendered
    static HTML remains viewable — no compatibility reader, no migration. Test
    updates: pin atlas_study_diagnostics_asset_test.go:141 to projection_version
    7, add stage/tally/ceiling/unavailable/failed/DirectionID tests, regenerate
    internal/report/testdata/report.golden.html. Verification is fully
    provider-free: rebuild request (--repo), mock from the request catalog,
    replay the saved accepted fixture (zero provider calls), render + node asset
    journey (EN and RU, embedded and stripped, keyboard and pointer), manifest
    gate, built binary offline on casdoor and one larger Go repository, and a
    byte-identical two-run projection. On Study failure the browse is a distinct
    local surface whose heading IS the failure banner, outside/visually disjoined
    from the Study diagnostics block, never the default content of a failed Study
    tab (FAILURE-001). No focus lane, second semantic call, GoSurvey, Tree-sitter,
    deeper SSA/DFS, fuzzy repair, hidden fallback, compatibility reader,
    migration, interactive pagination endpoint, or change to
    MaxAdvertisedSpans/MaxDirections/wire/cache/request/result/status contracts is
    authorized; no live acceptance is authorized by this decision.
    Decision 210 fixes the upstream cause of the narrow TLS-heavy Study shelf.
    It records exact direct static handoffs from production process entries by
    reusing the already built SSA program, without another package load, SSA
    build, depth walk or name classifier. Study targets carry closed
    producer-owned evidence-shape supports; package declarations and
    declaration-family motifs remain drawer-only, and partial Surfaces remain
    explicitly partial. A deterministic observed-lane selector records the
    complete candidate digest and considered/selected coverage instead of
    hiding first-N loss. The backend compiles focused or exact joined system
    route spans with local EN/RU questions, required support refs and allowed
    targets. The provider returns a span ref rather than inventing the
    question; exact support coverage is item-local and unrelated readings are
    rejected as padding. Complete and partial requested-span coverage remain
    distinct across result, cache, report and manifest. Grounding, request,
    prompt, result/status, cache and report identities advance; D209 responses
    miss closed. Provider-free producer, contract and saved-response gates run
    before one final installed-binary Casdoor acceptance. No Casdoor checklist,
    all-symbol taxonomy, deeper SSA/DFS, library expansion, raw Orientation,
    fuzzy repair, semantic retry, migration or UI redesign is authorized.
    Decision 209 removes a language-dependent local four-word question floor
    that discarded two otherwise valid directions from the saved D208 response.
    The exact saved response must publish all five routes with zero provider
    calls; provider request bytes and prompt remain unchanged, while local
    validator/result, accepted cache/artifact and report projection identities
    advance and older identities miss closed. Study becomes symbol-first and
    source-centric without reordering same-file readings. Overview and
    Architecture stop selecting a sorted-first source, retain plural exact
    typed sources, distinguish package participants from symbol ancestry and
    keep every accepted conceptual component map-navigable. Brief and Atlas
    remain distinct authority objects in one visual Overview. A diverse typed
    target shelf, support-span semantics, route-promise validation and any new
    provider call are explicitly deferred to a later owner-approved decision.
    Decision 208 preserves exact local structural relations at member level
    with zero-or-many conceptual participants, bridges Study to the currently
    visible Architecture only through exact typed membership, and consolidates
    the full Repository Brief on Overview. Empty Integrations and the local
    model-remainder diagnostic no longer masquerade as product components;
    compact packages retain working exact source actions in embedded and
    stripped static reports. No provider contract or Study cardinality changes.
    Decision 207 keeps the global pre-call Study breadth gate at three distinct
    eligible exact locators, but lets each independently validated route use
    one through five exact readings. Zero, oversize, duplicate, unknown,
    wrong-kind and every existing identity/privacy failure remain item-local.
    Prompt, resolver, artifact, replay and report share the same versioned
    contract; old cache identities miss closed and no route is padded or
    repaired.
    Decision 206 accepts an exact partial model Architecture projection over
    the complete canonical D177 Canvas. Every returned typed ref remains
    fail-closed, while uncovered requested conceptual members are computed
    locally into one explicit deterministic unclassified-by-model remainder.
    Status distinguishes full and partial coverage, old cache identities miss
    closed, and model prose or flags cannot grant operational authority.
    Declaration-family evidence never removes hypothesis status; only exact
    scoped producer-owned process-entry or call-target proof can do so. Study
    remains independent of Architecture full/partial/rejected state.
    Decision 205 gives every behavior anchor a producer-owned proof mode,
    prevents name-only declaration families from becoming primary operational
    Canvas or Study evidence, removes silent family prefixes behind truthful
    grounding coverage, deduplicates identical reading sets, and makes Unicode
    route diagnostics local-evidence-only. Family declarations remain exact
    clickable local evidence; they are not erased or promoted to runtime facts.
    Decision 204 replaces the failed calibration shape with producer-owned
    conceptual-member and structural-locator roles. Only conceptual refs are
    model-grouped and counted for exact response coverage. Structural locators
    remain complete local read-only containment/source context, participate in
    zero or more components through exact local links, and never become model
    membership or ownership. Status records local/requested/structural counts
    separately and earlier contract identities miss closed.
    Decision 203 adds an exact typed required-member checklist and permits one
    live Casdoor calibration only. It does not reinterpret structural file
    containers as durable conceptual members. A second prompt-only retry is
    forbidden: any repeated omission or mechanical file duplication moves the
    contract to producer-owned conceptual members plus read-only structural
    locators while D177 retains the complete local graph and source authority.
    Decision 202 makes the Architecture request and authorized replay consume
    one byte-identical exact Go package graph, requires complete coverage of
    every explicitly requested candidate member, and makes invalid optional
    Study domain terms item-local. Exact-graph absence is provider-free
    unavailable with the D177 local Canvas retained; it cannot become accepted
    enrichment over a legacy or partial graph. The owner authorized these root
    corrections after a fresh Casdoor run lost all 90 exact import relations
    and accepted only 15 of 50 requested members.
    Decision 201 separates model-authored many-to-many conceptual membership
    from optional independently proven local ownership. Shared members remain
    valid in multiple components; distinct coverage and membership-edge bounds
    are tracked separately; flow and Surface consumers expose plural
    participants without choose-first ownership or structural cross-products.
    Atlas Study uses one target per exact locator with plural principal
    associations and runs from any complete usable local Canvas rather than
    depending on ProposalAccepted. Insufficient catalogs are provider-free
    unavailable local products. Decision 200 identities are not reinterpreted,
    and no raw Orientation, old fan-out, fuzzy repair, primary/related model
    owner, legacy reader, migration or UI workaround is restored.
    The current Architecture provider response is one complete flat ordered
    `records` array: response-local subsystem refs (`gN`) and component records
    that cite exact request-local typed member and anchor refs. It is not a
    nested ownership graph; repeated membership across components means
    conceptual participation and never changes local D177 facts or relations.
    The current Atlas Study request exposes each reading target's exact
    repository-relative path, positive line and optional qualified symbol as
    read-only locator context, plus an `allowed_paths` set equal to the exact
    sorted target-path set. It exposes no source bytes. Identity fields return
    only short request-local typed refs. Direction prose may repeat an exact
    locator only when its resolved reading target advertised it. Supported
    Brief statement text and domain-term meanings may do the same only for
    reading targets named by their exact support refs; domain-term names may
    not. Every echo remains decorative and is never parsed as authority.
    Canonical identities,
    artifact/catalog identity and ref restoration remain private and exact.
    The owner-authorized product continuation adds one optional exact
    package-declaration Evidence locator per locally proven Go package. It is
    used for drawer-first package navigation but does not widen the Study
    reading catalog unless the same locator is independently selected by a
    semantic Surface, Navigator or Architecture target. Atlas-first Overview
    renders exact anatomy and accepted Study routes before a compact inventory;
    identical repository/module/application names share one display card with
    role tags while canonical IDs and relations remain distinct.
    After provider-free acceptance and exact installation, the repository
    owner explicitly authorized the root supervisor to run the live Casdoor
    comparison with existing credentials and iterate on material product
    defects; implementation/source subagents remain provider-free and secrets
    remain closed.
    Decision 200 keeps the Decision 199 Atlas-first product sequence and
    replaces its Architecture parity wire with deterministic private
    request-local typed refs. Canonical opaque identities remain local,
    collision and wrong-kind refs fail closed, D177 local Canvas remains the
    canonical visible base, Architecture and Study own one truthful language
    instruction, Atlas Brief exposes only valid typed support choices, routes
    resolve independently, and status v4 records exact request/usage/completion
    evidence with a closed diagnostic registry. The saved 14:44 Casdoor Study
    response is a provider-free regression fixture. No raw Orientation, old
    semantic fan-out, UI branch, fuzzy repair, sharding, legacy reader,
    migration or live-provider development call is restored.
    Decision 199 restores the useful model-assisted Architecture, Repository
    Brief and Study products on top of the accepted Atlas-first run. It does
    not restore raw-signal Orientation. Architecture reuses the existing
    bounded component synthesis over local RepositoryGraph and grounding;
    Study uses one compact request-local typed projection over accepted Atlas,
    Architecture and exact reading anchors. Navigator, Architecture and Study
    remain separate task-shaped questions with local validation and explicit
    authority. The repository owner explicitly classified the D198 removal of
    Architecture/Study as a product regression and approved this correction.
    Decision 198 replaces the ordinary raw-signal Orientation call with one
    Atlas-first, task-shaped Navigator recommendation, makes incompatible old
    semantic stages explicitly unavailable, fixes semantic cache identity and
    accepted-only writes, and removes silent local multi-module coverage loss.
    No raw graph/signals, compatibility fallback, legacy reader, migration or
    live-provider development call is allowed. Decision 197 removed or
    graduated experiments and playgrounds only after
    the completed caller/contract inventory. It does not add a new analysis or
    UI layer. Decision 196's accepted Atlas workspace remains installed;
    Navigator production wiring is held for the separate product-transition
    decision because it cannot truthfully emit the old Orientation semantic
    shape. Decision 196 retains the accepted Decision 195 UI/source-coverage and
    semantic-output checkpoints, but replaces its raw-complete-facts
    Orientation lane with a local Atlas-first, task-shaped Navigator
    projection. Atlas grows locally and is persisted as normal-run evidence;
    the model receives only bounded question-shaped summaries, local refs,
    representative exact evidence and a backend-owned action catalog. Raw
    signals, full source, raw file tree and raw internal edges remain local.
    The workspace shows Authority-labelled observed/resolved/inferred/partial/
    conflicted/unknown facts and honest empty states. Optional live A/B is
    last and requires explicit owner approval. Decision 195 removes silent active-product loss behind unrelated length and
    cardinality limits. It lands in four sequential reviewed checkpoints:
    complete exact saved-source coverage for every eligible visible Overview
    Surface and Architecture Component location; one truthful semantic output
    ceiling with typed terminal exhaustion and no fallback/publication; one
    1 MiB-default globally overridable compact-input budget with complete facts
    or a pre-call typed terminal outcome; and explicit classification of all
    remaining production analysis ceilings. Existing captured-input/report/
    manifest source authority, compact provider boundary, semantic validation,
    secret rejection, schema and typed-identity invariants remain fail closed.
    No source contents, raw full tree, or raw internal edges become model input.
    No live LLM, PATH replacement, legacy reader, migration, binary build,
    commit, or push is part of an unaccepted checkpoint. Decision 194 remains
    the semantic output-envelope foundation incorporated by this decision.
    Decision 194 makes REPOMAP_LLM_MAX_TOKENS the one truthful hard ceiling
    for every semantic provider request, with a 64,000-token default and no
    automatic stage floors, raises, doubling, or semantic completion/proposal
    resends. Existing byte-identical bounded transport retries and stage-owned
    thinking profiles remain separate. An exact request/response hard-limit
    breach or any finish_reason=length is a typed terminal resource error: the
    existing safe exchange recorder may retain the one failed exchange, but
    the response is not cached or applied and the ordinary run exits non-zero
    before later calls, authorized report/manifest/latest publication, serve,
    or open. Architecture cache identity binds the exact provider request.
    Russian localization consumes in-memory canonical report data and
    authorized output is generated once after successful localization or the
    existing non-resource fallback. No schema/cardinality expansion, provider
    capability framework, per-stage output knob, new debug/cache framework,
    transaction, legacy reader, migration, README/edge change, or live-model
    path is added. Implementation lands in three reviewed checkpoints: shared
    output-envelope core; callers/cache/top-level propagation; then Russian
    single publication, truthful doctor/metadata, docs, and full verification.
    Decision 193 removes provider confidence as standalone publication
    authority for Orientation candidate directions. A direction is accepted
    only when the existing local proof is present or producer-owned local
    verification names at least one verified fact. Rejected candidates remain
    in their existing diagnostic detail and order, but the report component
    backend excludes them from both path/evidence related-flow matching and
    lexical fallback anchors. Existing typed Surface identities carried by an
    accepted LocalProof remain intact. Prompts, calls, candidate production,
    schemas, caches, Study, Architecture, Atlas, localization, UI, README and
    edge caps, retries, legacy readers, and migrations remain unchanged.
    Decision 192 adds one default debug-only semantic exchange journal below
    the existing root-confined debug writer. Every ordinary model question has
    one recording owner; safe post-redaction request/response bytes or truthful
    closed markers are published before a metadata commit marker. Unsafe
    payloads retain only original SHA-256, byte count, and a closed kind.
    Best-effort write failures emit one bounded closed warning per run/stage and
    never change provider, cache, validation, retry, accounting, canonical,
    report, manifest, replay, or publication behavior. Current cache formats
    remain unchanged and raw-unavailable cache responses are never fabricated.
    The checkpoint lands in three reviewed trunks: recorder plus
    Orientation/Targeted Research; Architecture/Guided Tour/Study; then
    Localization and removal of --dump-llm and replaced dump paths.
    Decision 191 preserves the mandatory raw DetectAlways rejection for every
    localization provider response. Only after that rejection fires, the
    existing strict response decoder may attribute unsafe material to one
    decoded translation. The in-memory outcome and CLI warning expose a closed
    unsafe-kind code and one-based batch-local translation index; zero means
    strict decode or attribution was unavailable. Raw response, translated
    text, stable field identity, path, endpoint, and error never enter the
    diagnostic, status, cache, or another artifact. The saved localization
    status and every prompt, request, provider, cache, retry, acceptance,
    locale, canonical report, Study, Architecture, Atlas, UI, legacy, and
    migration contract remain unchanged. The --no-secrets runtime warning now
    distinguishes disabled ordinary input detection from mandatory
    provider-response and persisted-artifact scans.
    Decision 190 removes the ordinary Orientation candidate count cutoff while
    preserving the existing request-byte boundary. The normal command path
    supplies no file-count override, and Orientation resolves that value to
    the complete eligible FilteredFiles count before the existing generic
    bundle builder runs. A positive --max-llm-files value remains an explicit
    debug/test override and keeps the existing count-bound diversity selector.
    If the exact canonical bundle exceeds its byte budget, one bounded binary
    search returns the largest deterministic prefix of the already-ranked
    candidate rows that fits, including its returned fit warning. Only the
    candidate-file cap changes during byte fitting; README, module,
    entrypoint, edge, source-signal, known-doc, command-trace, and bundle-byte
    caps remain fixed. The reduced candidate cap does not become the
    OrientationCandidate cap. Candidate rows, allowed paths, dependent facts,
    the private reference catalog, and typed wire remain one atomic closed
    projection. This adds no source contents and changes no prompt, response,
    provider, cache, Study, canonical report, Architecture, Atlas,
    localization, UI, retry, legacy, migration, or live-model behavior.
    Decision 189 adds one provider-free, language-neutral Repository Atlas
    canonical core and one pure Go adapter over existing authoritative local
    facts. The closed core keeps repository/module/service/app/package Units;
    Surface/Operation/Boundary/Resource/Contract entities; and scoped
    Observation, Evidence, and Relation substrates with exact typed refs,
    closed Phase and Authority, and descendant-aware scope validation. Files
    and symbols remain Evidence locators, never Overview entities. The Go
    adapter preserves producer-owned module ownership, keeps app and package
    as sibling children of their module, and emits only an exactly matched
    process Surface to Operation exposes/startup/resolved slice. Missing or
    ambiguous proof remains absence. Canonical ordering deep-copies its input.
    No report, UI, persistence, manifest, provider, cache, Study, Architecture,
    localization, flag, legacy, or migration behavior changes.
    Decision 188 changes only the debug-artifact seam that persists
    llm_bundle.json with orientation_context_selection.v2.json. One shared
    private prepared-primary writer applies mandatory redaction exactly once,
    passes those exact post-redaction bytes to the producer sidecar callback,
    and persists the same primary bytes plus the derived sidecar. Orientation
    context selection preserves the exact canonical/request bundle identity
    and separately hashes and counts those exact saved bytes, including a safe
    non-JSON redaction marker when credential assignment detection replaces the
    complete bundle. Run manifest v6 verifies the persisted identity and
    rejects selection v1 and manifest v5 without a legacy reader or migration.
    It no longer accepts earlier pre-write bytes through a writer-newline
    fallback. Existing orientation-report diagnostics use the same helper.
    Redaction, --no-secrets runtime semantics, model bundle and
    typed-wire semantics, prompt/request/cache identity, candidate
    composition/order, canonical report, Study, Architecture, Atlas, UI,
    localization, clients, retries, flags, and provider policy remain
    unchanged.
    Decision 187 changes only the Study direction candidate provider seam.
    Backend-owned anchor, document, area, and mechanism identities are
    projected as compact typed ordinal refs under one exact request-bound
    catalog_ref. The response token is checked atomically; unknown, wrong-kind,
    duplicate, cross-request, substituted, prefixed, compacted, and corrupted
    item refs fail closed without fuzzy or string repair. One invalid candidate
    retains valid siblings and records only a bounded position and closed code.
    Canonical IDs are restored before existing normalization, review,
    publication, and persistence, so valid canonical candidates, direction IDs,
    and Study JSON remain byte-identical. The old unique-prefix resolver is
    removed. The candidate prompt advances to v5; exact typed request bytes and
    private catalog identity bind catalog/order plus prompt, response, and
    validator contracts, so earlier candidate-stage identities miss without an
    old reader or migration. The downstream per-direction review cache remains
    on its existing v1 request/bundle/source identity and keeps one-fragment
    reuse. Saved incomplete-Study projection rebuilds the same exact catalog
    from the verified bundle and resolves only typed reading-anchor refs; it
    does not read the earlier canonical-ID provider shape.
    BriefShape, shape_area_ids, review prompt/response and splitting, candidate
    composition/order/evidence, publication, report, UI, localization,
    Architecture, Atlas, retries, clients, flags, and cache framework remain
    unchanged.
    Decision 186 removes the blocked Decision 185 semantic fallback without
    rewriting its historical checkpoint. A local source-signal aggregate keeps
    a producer-owned entrypoint package when present and otherwise publishes an
    empty likely_entrypoint with its exact LikelyFiles and evidence intact.
    Only that local candidate basis receives the structural allowance;
    provider/model and every other basis still require an exact entrypoint.
    Downstream local bundle selection uses the exact likely files as seeds and
    synthesizes no entrypoint/path query semantics. Report DTO/format and
    manifest authority remain unchanged; successful local output now
    truthfully represents the absent entrypoint while exact files and evidence
    stay intact. No prompt, request, cache, Study, Atlas, Architecture,
    localization, UI, retry, client, flag, or --llm-bundle-only behavior
    changes.
    Decision 185 corrects only the local operational CandidateFlow producer.
    When an operational candidate has no statically proven entrypoint package,
    the newly appended local flow uses that candidate's first exact existing
    OpenFile as likely_entrypoint. Existing provider flows are not repaired or
    normalized, and invalid whole provider output still fails closed. Candidate
    composition/order/IDs/evidence, model acceptance, prompts, requests, cache,
    Study, Architecture, localization, UI, reports, manifests, clients,
    retries, flags, and --llm-bundle-only bytes remain unchanged.
    Decision 184 adds one provider-free, versioned, safe local Orientation
    context-selection artifact produced by the actual llmbundle selection and
    byte-fit path. It records effective caps, exact before/after counts,
    bounded proven cutoff samples, canonical selected candidate rows, and the
    exact compact-bundle/typed-wire byte identities without rerunning ranking.
    The version-5 normal run manifest binds and verifies its SHA-256, requires
    model-bundle and selection identities as a pair, and rejects earlier
    manifest versions without migration. No full file tree,
    extra source contents/snippets, provider response, credentials, replay, or
    development artifacts enter it. Selected composition/order,
    prompt/request/cache identity, provider calls, canonical report, Study,
    Architecture, localization, UI, clients/retries, flags, and
    --llm-bundle-only output remain unchanged; there is no legacy reader or
    migration.
    Decision 183 replaces repository-bearing values in the Orientation model
    response with one exact request-local private typed reference catalog and a
    compact in-place wire projection. The provider returns only the small
    decision AST; it does not echo backend contract versions or the private
    catalog digest. One file table covers every visible
    concrete location exactly once; existing bounded facts carry refs without
    a duplicate catalog inventory, raw allowed_paths, or long candidate IDs.
    File and evidence namespaces are separate; research candidate selections
    resolve only files that have a current canonical candidate ID. Provider
    prose remains non-authoritative and is not path/ID parsed. Unknown,
    wrong-kind, duplicate, prefix, shortened, substituted, and
    raw paths in typed ref fields fail closed. No regex/fuzzy/semantic repair or
    entrypoint fallback remains on the new provider-response path. Prompt,
    backend-owned response, and cache contracts advance together. Exact request
    bytes plus the private catalog digest bind cache identity, so an equal wire
    with a different private candidate mapping is a miss. Old cache records are
    misses with no reader or migration. Canonical report DTOs, candidate
    composition/order/confidence, targeted planning, local operational
    candidates, LocalProof, Study, Architecture fan-out, localization, UI,
    clients/retries, and flags remain unchanged.
    Decision 182 makes semantic acceptance mandatory when replaying the generic
    StageResponse cache for orientation and Guided Tour monolithic, fan-out
    leaf, and fan-in stages. Each existing stage-local validator remains the
    semantic owner. A semantically invalid exact hit is removed and handled as
    a miss through the ordinary bounded recomputation path; valid hits still
    make zero provider calls, structurally corrupt records remain normal
    misses, and failed/canceled/invalid live responses are not cached.
    Targeted research, Study, prompts, requests, candidate composition,
    scheduling, canonical artifacts, locale, UI, publication, flags, and saved
    formats remain unchanged. There is no legacy reader or migration.
    Decision 181 gives exact localization requests the same bounded retry
    behavior as normal targeted provider requests through one shared private
    transport primitive. Retryable request errors, HTTP 429/5xx, and transient
    response-body read failures replay identical request bytes; cancellation
    and semantic/JSON rejection do not. Metrics count actual transport
    attempts. Retry-After, prompts, provider identity, cache, Study, canonical
    artifacts, locale UI, flags, and saved formats remain unchanged.
    Decision 180 makes the bounded orientation evidence-path grammar recognize
    an exact terminal `.tsx` path without accepting its `.ts` prefix. The same
    existing repository-relative and exact allowed-path validation remains in
    force, so invented `.tsx` evidence is rejected. Model prompts/requests,
    cache, retry, candidate composition, LocalProof, Study, locale, UI,
    report/manifest formats, flags, adapter discovery, and fallback policy are
    unchanged.
    Decision 179 makes every SurfaceFrontier terminal-prose address injective
    by including immutable canonical collection order as the local
    prose-independent discriminator. Distinct frontiers may share kind and
    exact location without colliding in the RU presentation inventory. The
    report presentation contract advances to v11 and inventory contract to v8;
    earlier localization cache entries are misses, with `repomap cache clear`
    as explicit invalidation and no migration. Canonical report bytes,
    surface collection/count/order/details, semantic IDs, source links, Study,
    generic model cache, request shapes, UI schema, and retrieval are
    unchanged.
    Decision 178 makes semantic acceptance a prerequisite for persisting or
    applying a targeted-research provider response. Decode/validation failures
    and all-findings-rejected outcomes are never cached. A corrupt or
    semantically rejected exact cached record is removed and handled as a miss;
    an accepted hit still passes the same local validation and makes no
    provider semantic call. The targeted v3 cache contract is bound into both
    fingerprint and record; earlier targeted records are not read or migrated,
    and `repomap cache clear` is the explicit whole-cache invalidation. No
    prompt, request, retry, Study, locale, Guided Tour, orientation, UI,
    report/manifest, or flag behavior changes.
    Decision 177 makes the local deterministic Architecture Canvas the sole
    canonical base map consumed by static rendering and reportserver. Accepted
    model synthesis may enrich grouping over the same exact candidates, but
    rejected or malformed synthesis cannot erase or substitute the base.
    Ordinary ReadRunDir no longer implicitly applies development semantic,
    Mechanism, onboarding, or paved-path replay files merely because they
    coexist in a run directory. Decision 177 publishes zero CandidateDirection
    FlowProof overlays: the current seed-surface field may come from a
    same-executable heuristic and is not an exact architecture relation. Those
    directions remain available to Study/suggestions until a future
    producer-owned typed binding slice. No adapter,
    prompt, cache, Study, locale, manifest, source-authority, migration, or new
    flag is introduced.
    Decision 175 removes only the harmful residual-Latin localization
    acceptance predicate exposed by the Decision 174 diagnostics. Provider
    response schema/index coverage, opaque placeholders, secret scan, bounded
    values, atomic batches, canonical English fallback, and cache safety remain
    strict. No glossary, model-selected exemption, prompt change, cache
    namespace, migration, legacy reader, or user flag is introduced.
    Decision 174 completed the release-blocking diagnostic checkpoint. It
    records only actually processed localization batches, adds a closed safe
    failure stage/code/counter status contract, saves rejected localization
    output only under --dump-llm after secret handling, and exposes the Guided
    Tour validator field/rule without changing prompts or semantic behavior.
    Decision 173 is approved but queued until this gate produces the exact
    invalid_projection boundary and its smallest root-cause correction.
    Decision 173 replaces every active Study model-output opaque-ID copy with
    an exact request-local typed handle table. Canonical IDs remain local and
    are restored only after exact type+handle resolution. The deterministic
    table and contract versions bind exact requests and review-cache identity;
    old cache entries are not read or migrated and `repomap cache clear` is the
    explicit invalidation path. Candidate composition, scheduling, review
    validation, publication, canonical IDs/paths/evidence/order, report, and UI
    remain unchanged.
    Decision 172 disables provider-default hidden reasoning for the bounded
    orientation classification on the official DeepSeek endpoint and adds one
    bounded orientation completion retry only after an
    explicit provider finish_reason=length outcome, preserving the exact
    canonical prompt, facts, endpoint, model, and validation while doubling
    output headroom once and aggregating attempt telemetry. It then measures
    the real Casdoor Study funnel against the saved baseline before any
    separate coverage correction. Decision 171's cache remains an accelerator
    and must not alter candidate composition, validation, publication, IDs,
    evidence, paths, or order.
    Decision 171 adds only a content-addressed persistent cache for the
    existing bounded canonical-English Study reading-pack review requests. It
    does not change candidates, scheduling, review validation, publication,
    canonical artifacts, DTOs, or UI. Identity binds exact provider request
    bytes, endpoint/model/profile, prompt/generation/output contracts, and
    exact review/source hashes while excluding repository revision, run ID,
    timestamps, and credentials. Hits replay through current local validation
    without an API key or HTTP request; --no-cache bypasses reads and writes;
    corrupt, unsafe, failed, canceled, partial, or invalid results are never
    applied or cached. Other model stages remain unmigrated.
    Decision 170 expands the owner-approved localization slice to one complete
    typed terminal PresentationTextInventory, one atomic EN-to-RU request on a
    cache miss, and explicit degradation for any incomplete or oversized
    projection. Fixed product copy remains in the typed EN/RU catalog and
    opaque technical values remain byte-identical. It also authorizes the
    bounded publication-trace, README sentence-splitter, serve-picker, offline,
    mixed-source selection, and typed Study diagnostic corrections recorded in
    the decision. Study scheduling must preserve the complete canonical
    proposal through review; cost reduction is deferred to a separate
    cache/batching layer rather than pre-filtering canonical candidates.
    Canonical report, snapshot, bundle, semantic caches, retrieval, grounding,
    manifest, source authority, and source routes remain unchanged. Live
    provider verification and the held Python work remain excluded.
    Decision 169 makes English the canonical output of every semantic model
    stage and moves Russian model-authored prose into one optional, separate,
    complete inventory-driven presentation projection. Fixed product copy remains in the
    typed EN/RU catalog. A content-addressed translation cache binds the full
    request, target locale, endpoint, model, and contract version; --no-cache
    bypasses shared reads and writes. Localization failure preserves the
    manifest-bound canonical English report, keeps the requested RU
    product-message catalog active, and explicitly marks canonical English
    model prose rather than silently labelling it translated. Cache hits are
    available before live API-key configuration; cache I/O cannot discard a
    valid per-run projection; provider output is secret-scanned; and
    repository freshness is re-confirmed after localization. IDs, paths, symbols,
    packages, source locations, evidence, links, facts, retrieval, ranking,
    grounding, report/manifest/HTTP formats, and source authority do not
    change. The held Python/D169 work is excluded.
    Decision 168 replaces every Russian typed-catalog renderer that still
    preserves English product prose. Only three complete opaque values remain
    byte-identical across locales: HTTP, a method/path route identity, and an
    exact location link. Technical names may remain inside otherwise Russian
    copy; repository/model prose and opaque parameters remain exact. No
    provider, semantic, cache, report/manifest/HTTP, source-authority,
    navigation, or runtime-translation behavior changes.
    Decision 167 atomically replaces the report's exact-string, regex,
    TreeWalker, and MutationObserver localization path with one explicit
    English/Russian product-message catalog shared by the main renderer,
    Architecture canvas, surface catalog, and fixed template chrome.
    Product-owned strings use stable message IDs and declared parameters at
    their render sites; repository/model prose, paths, symbols, packages,
    source, IDs, links, and lines remain opaque exact parameters. Unknown IDs
    or parameter mismatches fail explicitly and English is not a Russian
    fallback. The sole server-rendered EN/RU noscript notice is an explicit
    fixed exception because the JavaScript catalog cannot run when JavaScript
    is disabled; no second translation runtime is introduced. No ordinary
    Architecture projection, --lang behavior, provider, executor, retry,
    cache, report/manifest/HTTP/source-authority behavior, or semantic-stage
    localization changes.
    Decision 166 adds only an explicit provider-free, content-addressed
    Architecture localization projection record. It builds the exact
    Decision 165 prompt and one deterministic OpenAI-compatible request body
    without an API key or network call, then records only a complete accepted
    Russian projection under a fixed root-confined immutable path. Every hit
    re-derives current canonical English input and reruns Decision 164 replay.
    Missing is an explicit dev-only miss; corrupt or unsafe expected content
    fails closed. No live provider, retry, shared cache, ordinary `--lang`,
    report, manifest, HTTP, source-authority, or browser behavior changes.
    Decision 165 adds only one exact provider-neutral localization prompt and
    one explicit provider-free developer stage over the verified Decision 164
    replay. The stage re-derives current canonical English Architecture prose,
    emits exact prompt JSON when previewed, and may consume one explicitly
    supplied bounded local projection through an injected no-network seam.
    Prompt/response bytes are bounded and secret-scanned, the run remains
    unchanged, and field/envelope fallback remains the Decision 164 behavior.
    No live provider, direct HTTP path, retry, cache, persistence, ordinary
    `--lang`, semantic request, report, manifest, source-authority, or browser
    behavior changes.
    Decision 164 adds only an explicit provider-free replay over the verified
    Decision 163 English Architecture identity. An explicitly supplied strict,
    bounded Russian projection fixture is applied to a freshly re-derived
    current English Canvas and one deterministic bounded replay JSON value is
    emitted to stdout. Envelope or field failures retain canonical English
    with deterministic diagnostics. The command writes no run artifact and
    does not trust the B3 sidecars as cache authority. No live provider,
    RU-to-EN round trip, ordinary run, cache, `--lang`, semantic request,
    report, manifest, HTTP, source-authority, or browser behavior changes.
    Decision 163 adds only an explicit provider-free developer export through
    `make localization-check RUN=<run-dir>` and
    `repomap dev localization-check <run-dir>`. An eligible saved run may
    receive exactly three non-consumable English identity artifacts under
    `localization/`, with Architecture-specific names that intentionally
    narrow the draft whole-report filenames. Eligibility requires an explicit
    current v3 English accepted or accepted-with-normalization non-fallback
    synthesis, matching current v2 status, replay against current saved facts,
    and byte-exact English identity. The root-confined writer uses only fixed
    paths, `0700`/`0600` permissions, bounded inputs and outputs, pre-write
    secret scanning, exclusive temporary files with partial-install rollback,
    and refuses symlinks, incomplete sets, and conflicting files. No ordinary
    run path reads or writes these files, and no provider, Russian projection,
    cache, report JSON/HTML, manifest, HTTP, source-authority, or UI behavior
    changes.
    Decision 162 connects the isolated localization contract to exactly one
    real semantic shape: Architecture Canvas subsystem and component names and
    descriptions. Their identities come from validated local member sets and
    do not depend on localized prose. English identity must preserve the full
    canvas byte-for-byte; a supplied Russian projection may change only those
    allowlisted fields while paths, symbols, packages, members, relations,
    enums, evidence, order, and all other data remain exact. This decision
    adds no run artifacts, provider call, cache change, ordinary CLI wiring,
    report/HTTP/manifest change, DOM rewrite, or Russian-to-English translation.
    Decision 161 records the actual current English/Russian locale and cache
    behavior, including the safe-miss correction that makes part of the draft
    roadmap stale. It authorizes only one isolated provider-free
    `internal/localization` contract slice: allowlisted stable field IDs,
    canonical English identity, protected placeholders, deterministic hashes,
    and canonical fallback for invalid supplied projections. Ordinary CLI and
    report wiring, persisted localization artifacts, cache v2, provider calls,
    DOM localization replacement, and the attributable dirty Python-focus WIP
    remain held. The useful current comparison is English to Russian;
    EN-to-RU-to-EN is deferred until a locale-projection cache exists.
    Decision 160 adds `--github-url` as the GitHub-hosted equivalent of the
    existing standalone GitLab report. A complete repository URL or a
    host-only URL inferred from sanitized `origin` identity produces one
    self-contained no-server HTML artifact whose source actions open the exact
    captured GitHub commit and line. GitHub and GitLab remain mutually
    exclusive host adapters over the same static source authority; their blob
    paths and line-range fragments remain host-correct. Stable dirty paths stay
    local-only, no remote request is made, and canonical report JSON and
    manifest formats remain host-neutral.
    Decision 159 replaces the held Decision 158 exact-tree attempt with a
    generic shallow inventory. Framework adapters emit neutral Descriptor,
    Binding, and Activation facts: every build-selected typed descriptor is
    useful even when a complete route from process startup is unavailable,
    while only direct unambiguous bindings and activations enrich it with
    stronger structure. Cobra is the first adapter, not the product data model.
    Dynamic aliases and instance flow remain explicit partial frontiers instead
    of being interpreted, guessed, or hidden. Outbound clients and integrations
    may later use the same neutral contract, but no client adapter is authorized
    by this decision. Partial reading paths, lifecycle pairing, and ordinary
    Mechanism composition remain separate following slices.
    Decision 158 is preserved as a held and superseded exact Cobra tree
    reconstruction attempt. Its implementation review found that reconstructing
    global instance identity, mutation order, wrapper reachability, and command
    hierarchy required a fragile miniature interpreter. None of its deep
    reconstruction claims are active scope.
    Decision 157 corrects the provider-facing Study candidate contract after
    another ordinary run rejected ten of twelve drafts for anchor selection.
    The prose requires three to five anchors, while the JSON example visibly
    teaches one. The example now demonstrates three matching anchors and
    readings, the prompt version changes, diagnostics separate count,
    duplicate, and malformed-ID failures, and a bundle with fewer than three
    exact code anchors skips the impossible complete-pack request explicitly.
    Complete-pack validation, incomplete topics, IDs, source authority, report
    formats, HTTP behavior, Cobra discovery, lifecycle evidence, and Mechanism
    semantics remain unchanged.
    Decision 156 corrects two real product-observability failures. Architecture
    components already retain several bounded exact handler and entry anchors,
    but the inspector hides them behind one generic code start and renders
    exact package identities as inert text. The inspector now keeps component
    code, Study navigation, and one exact handler/registration target per
    owned runtime surface distinct, and routes package labels through a
    package-owned file when the existing local or pinned-GitLab authority can
    open it. Successful ordinary runs also gain one bounded publication
    summary for the stage counts and locally issued reason codes already
    available without provider prose, source contents, credentials, wire
    changes, new analysis, or cache changes.
    Decision 155 corrects a real Architecture inspector failure where a broad
    behavior-family anchor representative could outrank a selected component's
    own exact member location, send unrelated components to the same file, and
    create foreign file-level Study joins. Exact located members now own the
    code start; package/file-bound and anchor-only fallbacks remain available.
    Decision 154 removes the residual all-or-nothing Study publication floor
    and the reading-label localization mismatch exposed by a real run. One
    through seven independently reviewed directions may publish canonically;
    zero still fails closed. Closed schema literals remain canonical under
    `--lang ru`, and bounded model-authored Study labels reduce to existing UI
    labels without changing anchors, reading instructions, ordering, IDs, wire
    formats, or report behavior. Active orientation, targeted research, Guided
    Tour, and architecture synthesis caches isolate Russian output from
    English; language-unknown historical architecture records remain
    replayable but are not reused as active cache hits. Study ranking no longer
    penalizes Russian prose that cannot match code identifiers, and
    runtime-order and targeted-research certainty validation retain their
    safety boundaries in both supported report languages.
    Decision 153 removes two unnecessary static-sharing restrictions exposed
    by product use. A stable dirty checkout may publish, with unchanged source
    paths linked to the captured GitLab commit and dirty source paths marked
    local-only rather than falsely linked. A host-only `--gitlab-url` derives
    namespace/project from sanitized repository-local `origin` identity. No
    source contents, credentials, synthetic commit, GitLab request, or
    canonical report/manifest change is introduced.
    Decision 152 adds one explicit static-sharing mode. `--gitlab-url`
    produces a self-contained no-server report whose source actions open the
    exact captured commit and line in GitLab. The HTML omits saved source
    bodies and local-only authority metadata while canonical report JSON and
    manifest formats remain unchanged. A clean checkout is required, analyzed
    submodules and the source-episode combination are rejected, and link
    construction adds no provider request or network lookup.
    Decision 151 corrects fresh uncached Russian product review of lambroll and
    wireguard-ui. Runtime selector assignments no longer masquerade as literal
    credentials; exact file/symbol Architecture components remain inspectable
    without a package member; and a failed Study no longer causes Overview to
    substitute saved code snapshots. Russian fallback presentation copy and
    purpose selection now follow the report language while exact technical
    identifiers remain unchanged.
    Decision 150 corrects fresh uncached Russian product runs of pglogrepl and
    WAL. One-package libraries now reach bounded architecture synthesis; Study
    shape adapts to one through seven supplied areas; targeted research can
    retain two distant windows from one file and explicitly forbids absence
    claims from lexical-window omissions; exact component member locations
    make visible architecture cards inspectable; example-only executables no
    longer turn a library into an application; one-step partial traces are not
    advertised as code paths; local source actions prefer the editor; Russian
    prose is reinforced in every model message; and accepted package-grounded
    synthesis is labelled honestly. No new runtime relation, language adapter,
    Search surface, or source snapshot is introduced.
    Decision 149 corrects the real Chatto Guided Tour failure where structurally
    valid editorial hypotheses were rejected by a behavioral-word regex and an
    exact candidate title with a natural slash was mistaken for a repository
    path. Suggested-direction prose is now explicitly non-authoritative while
    exact IDs, gaps, evidence, locations, and model-authored repository
    references remain locally validated. The classifier remains diagnostic.
    Decision 148 corrects two ordinary-run failures found during real
    Decision 147 validation. Nested-module `go/packages` diagnostics now
    resolve only against actual loaded package files, so a module-relative
    position cannot invent a doubled non-existent path and disable the source
    catalog. The secret scanner recognizes only an all-zero credential value
    as a documented placeholder while keeping mixed values fail-closed.
    A live `github.com/devodev/go-office365/v0` run retains `/v0` in module,
    package, symbol, surface, and report identities.
    Decision 147 adds one product-wide `--lang en|ru` selection. Russian mode
    localizes ordinary model prose and the report UI while preserving exact
    paths, symbols, packages, protocols, product/library names, structured
    keys, enums, IDs, and quoted source. English remains byte-compatible when
    the option is absent, and the report format adds only an optional language
    marker.
    Decision 146 corrects an ordinary-run failure where Git represented an
    untracked nested checkout as one directory. Because that checkout is
    outside the parent repository's tracked snapshot, freshness now detects
    and excludes exactly that case without recursion or reading ignored
    contents. Tracked submodules and ordinary dirty files retain their current
    behavior, while non-repository directory entries still fail closed.
    Decision 144 adds only one provider-free discriminator for the real Beets
    failure observed after Decision 142. It must route the already recorded
    Pyright `_get_plugin:406` focus through the existing targeted research
    window planner and real Study bundle assembler, retain the exact
    `plugins.py:366-445` window, publish the exact Python declaration anchor,
    and invent no complete Mechanism. Production integration remains held
    pending concrete review of that result.
    Decision 143 corrects one ordinary-run terminal failure where optional
    model uncertainty metadata contained the safe directory-like value
    `a/b/c/`. Because unverified paths cannot authorize navigation or establish
    evidence, ordinary normalization now canonicalizes safe trailing-slash
    drift, drops unsafe values with deterministic warnings, deduplicates the
    result, and leaves strict validation unchanged for retained values and all
    grounded fields.
    Decision 142 corrects one ordinary provider-backed etcd run where semantic
    opportunity discovery and paved-path editing consumed about 124 seconds
    while publishing zero primary content, and report replay exposed eight
    pre-reduction Study proposals plus three locally ineligible semantic topics
    instead of the reducer-selected six canonical directions. A non-empty
    canonical Study Map becomes the sole Study publication projection for both
    Overview and Study; incomplete exact-start directions remain only as the
    no-canonical-map fallback. Ordinary mode stops eagerly scheduling semantic
    opportunity and paved-path editors while preserving their explicit
    developer entrypoints and saved-artifact replay.
    Decision 141 corrects four concrete failures observed in the completed
    Decision 140 review and the fresh Chatto run: nested Go modules must reach
    typed surface analysis; safe tracked in-repository symlinks must not erase
    paved-path evidence; one unambiguous JSON object may be recovered from
    provider prose before strict Study validation; and an empty Guided Tour
    preflight is an expected quiet no-call outcome. It adds no new provider
    request, language adapter, proof claim, Search surface, or report format.
    Decision 140 runs the verified D139 stable binary exactly once on Beets
    with `--no-search`. It records stage durations but does not stop because of
    elapsed time. No retry, code change, second repository, or timeout change
    is authorized before terminal artifact and product review.
    Decision 139 changes only the deterministic selection of the second
    targeted research round. The highest-scoring eligible round remains first;
    when a second slot is available, the planner uses existing bounded
    ProviderAllowedPaths to prefer the eligible round with the least average
    shared directory prefix, with score and stable ID order as tie-breakers.
    It does not change eligibility, evidence collection, provider inputs,
    prompts, round count, Search, Study, Architecture, or report presentation.
    The product supervisor accepted the provider-free Beets-shaped
    microexperiment and authorized this integration plus focused/full checks.
    The completed implementation passed those checks and the nearby etcd
    validation. The product supervisor accepted D139 with no blocker and
    authorized exactly one local checkpoint plus stable PATH-binary rebuild.
    Decision 138 completed exactly one normal `beets --no-search` run using
    the verified Decision 137 checkpoint and stable PATH binary. The run
    published three honest incomplete Python topics and exact editor starts,
    but its General Study stage failed because Study anchors accepted only the
    Go-specific sourcewindowfacts.Function contract. After the authorized
    two-file assembly fallback proved impossible and was fully reverted, the
    product supervisor authorized one six-file tagged Study-anchor contract:
    unchanged Go Function validation or a bounded, hash-verified, explicit
    non-Go ExactSource, never both. No provider or repository rerun is allowed
    before focused/full checks and concrete supervisor review. Those checks and
    the saved Beets provider-free assembly/review replay passed; the supervisor
    confirmed the real `import_files` anchor at line 50, zero complete
    mechanisms, no blocker, and authorized the local checkpoint.
    Decision 137 integrates only the supervisor-approved incomplete Study
    projection and detail route. It replays the saved bounded candidate
    response against the exact saved anchor catalog, preserves provider order,
    publishes only catalog-resolved exact starts, keeps the three existing
    topics on Overview, and exposes the incomplete directions through one
    Study route. The complete three-to-five-anchor pack gate and provider
    prompt remain unchanged.
    Decision 137 is complete: an authorized provider-free replay of the saved
    Chatto run published all twelve incomplete directions in provider order,
    preserved the three existing Overview topics, opened one exact saved source
    through the verified local server, exposed no Search navigation, and kept
    complete Study empty. The product supervisor reviewed production Overview,
    Study, detail, and source screenshots and accepted the slice with no
    blocker. Focused tests, the complete repository check, vet, offline quality
    replays, and diff checks passed.
    Decision 136 is complete: provider-free validation projected all twelve
    retained Chatto questions in provider order with twelve catalog-resolved
    exact starts, no unresolved or invented starts, and no complete-pack
    claim. Overview, Study, and direction detail were browser-inspected; the
    product supervisor reviewed all three screenshots plus validation.json
    and approved the narrow production integration with no blocker.
    Decision 135 completed one fresh normal Chatto acceptance run using the
    verified Decision 134 checkpoint and stable PATH binary.
    Decision 134 changes only the provider-facing Study candidate failure unit.
    The exact Decision 133 Chatto response contained eleven ordered candidates:
    nine satisfy the existing contract and two contain only two anchors. The
    current atomic decoder discards all eleven. The corrective must retain
    independently valid candidates in provider order, reject invalid siblings
    with bounded position/reason diagnostics, preserve every existing anchor
    and scalar rule, and fail closed when zero candidates survive. Acceptance
    is provider-free and includes the exact retained response plus coexistence
    of the three grounded topics, Study, Architecture, and no Search.
    Decision 134 is complete: the bounded decoder accepts the nine valid
    retained Chatto candidates, rejects the two invalid siblings with stable
    diagnostics, preserves ordering and local IDs, and keeps envelope and
    zero-survivor failure atomic. Focused tests, the full repository check,
    vet, offline quality replays, and diff checks passed; the product
    supervisor reviewed the corrected implementation and approved its local
    checkpoint.
    Decision 133 completed its one authorized run and continues to record that
    the stable product fallback shows three useful topics, reaches exact source
    within two clicks, and exposes no Search.
    Decision 132 remains completed and continues to govern the provider-only
    domain_terms placement normalization and strict canonical output.
    Decision 131 remains completed and continues to record the successful
    three-topic D130 product verification plus separate routing and
    1280-pixel overflow debt.
    Decision 130 remains completed and continues to govern supported Chatto
    incomplete-topic reasons and unconditional normal-report Search removal.
    Decision 126 remains unchanged and continues to govern operating-path
    completeness, evidence, publication, and runnable-path safeguards. Decision
    127 remains active for the mixed mechanism/topic shelf, default Overview,
    primary navigation, and two-click mechanism/topic presentation. Decision
    128 remains completed and continues to govern the language-neutral
    discovery-attempt invariant. Decision 130 changes only the supported
    incomplete-topic reason projection and normal-report Search presentation;
    it does not alter complete-Mechanism proof or publication.
    It does not delete, rewrite, or invalidate historical decisions 050, 060,
    070, 080, 090, 091, 092, 093, 094, 095, 096, 097, 098, 099, 100, 101, or
    102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, or
    116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 130, or
    131, 132, or 133.
    Unified surface accounting, behavior-grounded architecture, and
    Operational Flow Discovery remain part of the existing product.

## Decision 217

Status:
    Report UI/UX acceptance repair across application, CLI, service, and
    library reports — presentation/view-model corrective (no provider
    behavior, no pipeline change). Verified on the owner-supplied saved-run
    matrix (casdoor/telebot/restic/chatto plus the format-30 casdoor-old
    comparison): global shell (localization strip → collapsed "About this
    report" disclosure, provenance row, markdown-safe README labeled as
    source, localized guide hero), Overview "At a glance" four questions,
    deterministic evidence tiers (exact source / package-backed / hypothesis /
    unmapped), Study learning plan with coverage wording and collapsed
    "Coverage and provenance", Architecture relation honesty (labeled
    relations or explicit zero-relation notice), compact expandable
    unmapped-evidence disclosure, component list alternative, mobile list
    default, dialog drawer (role/aria-modal/focus trap/Escape/focus return),
    zero horizontal overflow at 390 px on all 15 route×repo combinations
    (baseline Casdoor Study 748 px → 390 px). Full suite, vet, build, node
    checks, quality and localization gates green; browser acceptance captured
    at 1440x1000 and 390x844.

Approved by:
    Repository owner via the active supervisory goal
    (report UI/UX acceptance repair, 2026-08-05).

Notes:
    Upstream data-quality candidates recorded for follow-up (not scope
    expansion): restic/chatto anchors published under persistence_limit,
    all-format-31 conceptual components carry hypothesis:true, zero
    flow_count/orientation_confidence across the matrix, study-theme overlap
    surfaced as limitation rather than rewritten.

## Decision 218

Status:
    ACTIVE (2026-08-05) — report truth corrective authorized by the owner's
    revised risk roadmap pack (repomap-hermes-d218-report-truth-corrective.txt).
    Provider-free presentation/view-model corrective on top of D215 + D217:
    A) Study renders every published theme (theme-level "show more" removed);
    B) typed source rows with closed kinds (function/method/type/call_site/
    package/file/document/boundary) as separate DOM nodes, visible even when
    no exact saved source is available for the run;
    C) Overview system spine — one representative card per supported role
    (entry, core, state, extension, operations), one explicit primary card,
    every other component reachable through Architecture;
    D) closed three-state relation presentation (proven_component_relations /
    member_relations_unprojected / no_supported_relation_evidence) with
    structured-list-primary when there is no proven edge evidence;
    E) truthful Architecture synthesis state mapping (not attempted / accepted
    / accepted for X of Y / provider responded-rejected / output limit /
    cached); "not performed" is never shown for attempted provider calls;
    F) repository/module units become structural headers with child counts
    when child applications exist;
    G) README-derived purpose is labeled source material, never repeated as
    hero + glance answer, with a neutral local fallback for residue heads.

Verification (2026-08-05):
    All five Archive 5 fixtures (etcd, Telebot, Chatto, Restic, Casdoor)
    rendered and inspected at 1440x1000 and 390x844 on Overview, Study, one
    Study detail, and Architecture; every published theme visible without
    "show more"; symbol/path/kind visibly distinct; packages never shown as
    functions; Overview spine 3-5 role cards with primary; zero-relation
    repos default to list/taxonomy; etcd shows attempted/rejected/local
    fallback (never uncalled); chatto shows accepted 81 of 89 with local
    remainder; repository/module hierarchy clear; README not duplicated;
    16/16 route×repo checks at 390 px with zero horizontal overflow. Full
    suite, vet, build, node checks, quality and localization gates green.

Approved by:
    Repository owner via the active supervisory goal
    (D218 report truth corrective, revised risk roadmap pack, 2026-08-05).

Notes:
    Decision 219 (Study content integrity) is deferred by the same review;
    its partial implementation is preserved as a patch and will resume after
    Surface Discovery v2 and the Study deep-reading contract.

## Decision 223

Status:
    Decision 223 active (Phase 1 of the overnight program
    hermes-repomap-overnight-goal-v3.txt): the Architecture provider wire
    replaces raw package_import supporting relations with the per-unit
    RelationOutCount aggregate that Decision 216 promised but never
    delivered (projectUnitWire hardcoded 0; raw edges kept shipping at
    59-65% of request bytes on Archive 6 runs: etcd 86KB of 141KB, restic
    35KB of 54KB). BuildSynthesisRequest compiles the unit catalog before
    serialization and, when units are present, drops package_import edges
    (behavior_handoff remains as exact read-only grouping context);
    unitOutgoingRelationCounts resolves membership against final post-split
    units so the aggregate is correct. SynthesisRequestVersion 11->12,
    SynthesisPromptVersion v14->v15; old cache identities miss closed. The
    model groups u* unit refs and cannot act on p*-level import edges, so
    no grouping signal is lost. Saved accepted responses (telebot/restic)
    replay under the new request identity; etcd/casdoor rejections remain
    honest (missing component ref / duplicate member set). Provider-free
    acceptance: full gates green, etcd output-exhaustion CLI replay green,
    casdoor complete-graph test asserts 0 raw relations + non-empty
    aggregate + compact request.

## 18. Decision 235 — v10 closure / MAP-READY contracts (v11 program, ACTIVE)

One member_refs response grammar (prompt v19, unit_refs decoder retained for
pre-v19 replay); backend normalization (missing anchor_refs → []+count, empty
component → item-local, Gotify trailing `]}` → bounded normalization); final
Architecture rebased into Study (accepted model canvas IS the Scout context);
span questions populated from backend-owned questions or omitted; theme
equivalence accounting; GOTOOLCHAIN=auto provenance; local failure
containment (maddy per-file secret closure, sqlc/syn/bench Study without
canvas, container-registry external frontier, chatto post-Scout typed
status, caddy facts cap, gemnasium duplicate merge). MAP_READY gate precedes
Decision 236 (Repository Map primary product).
