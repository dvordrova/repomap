# Current approved implementation decision

Decision:
    decisions/161-localization-characterization-contracts.md

Status:
    Decision 161 active; isolated localization contracts authorized

Approved by:
    Repository owner in the current session

Notes:
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
