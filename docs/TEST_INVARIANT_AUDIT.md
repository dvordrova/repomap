# Test invariant audit

This audit describes the working-tree test inventory owned by
`testdata/contracts/tests.json`. It covers all 140 production-root Go test files
under `cmd/` and `internal/`, including two helper-only `_test.go` files, and the
complete repository-owned JavaScript/TypeScript test-script inventory (currently
empty). The descriptions below state the product contract each file protects;
ordinary in-process test mechanics are omitted. Exceptional execution mechanics
are listed separately at the end.

## Cross-cutting gaps and weak assurance

- **There is no automated ordinary-path online-provider acceptance test.** The
  inventory's default provider profile is `stubbed-or-none`; transport tests use
  a loopback server. The suite proves request, retry, validation, cache, and
  publication contracts, but a real provider run and its generated artifacts
  remain manual product-acceptance evidence.
- **Browser assurance is partly optional and partly lexical.** The Node and real
  Chrome/Chromium contracts skip when those runtimes are unavailable. Several
  report-asset tests assert required/forbidden source tokens; those checks can
  detect ownership or feature drift, but token presence alone cannot prove that
  code is reachable, visually correct, or accessible in every browser. The Node
  VM and one headless-browser path reduce, but do not eliminate, that gap.
- **Repository fixtures do not exercise a mixed-language repository end to
  end.** The cumulative Go, Python, and JS/TS fixtures independently prove their
  adapters and exact inventories. Mixed-language target planning is covered with
  in-process catalogs and temporary inputs, not one cumulative repository-shaped
  acceptance case.
- **The inventory has no native JavaScript/TypeScript test files and no fuzz
  targets.** Embedded browser JavaScript is exercised from Go-owned harnesses,
  but there is no independently inventoried JS test runner. Parsers, canonical
  codecs, path validation, and tamper rejection rely on example/table tests
  rather than fuzzing.
- **Prompt tests are contract tests, not semantic-quality evaluations.** They
  bind embedded prompt prose, schemas, versions, and closed-ref rules; they do
  not show how a changing hosted model behaves on real repositories.
- **Two inventoried files contain helpers rather than direct test entrypoints.**
  `internal/contracttest/fixture_test.go` and
  `internal/surfacediscovery/test_helpers_test.go` enforce setup preconditions
  and are exercised transitively, but cannot fail as standalone `Test*`
  functions if their callers disappear. Exact inventory coverage still keeps
  both files visible.

## Command and orchestration contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `cmd/repomap/cache_clear_test.go` | Cache clearing removes only the known persistent cache directories, rejects a symlinked cache root, and does not leave a partially cleared state. |
| `cmd/repomap/go_target_auto_test.go` | Explicit Go target selection wins over automatic/environment selection, while environment fallback remains exact and canonical. |
| `cmd/repomap/javascript_target_runtime_test.go` | Repository language detection, exact JS/TS package selection, nested-manifest and package-bin corpus binding, product-versus-library authority, compiler-preparation guidance, stale-ref rejection, and preservation of every selected package binding. |
| `cmd/repomap/publication_mode_flags_test.go` | `--port` is invalid only with static publication, semantic stop points remain closed, positional repository parsing survives flag order, and implicit static source-link preflight is enforced. |
| `cmd/repomap/python_target_runtime_test.go` | README-selected Python files resolve to framework-neutral module views; explicit selectors must be exact; unrelated discovery omissions cannot erase an exact selected target. |
| `cmd/repomap/readme_role_authority_test.go` | README role authority is persisted exactly, canonical empty authority means absence, and persistence failure is terminal. |
| `cmd/repomap/repository_target_runtime_test.go` | Mixed Go/Python/JS/TS target planning restores every adapter and native target, defers JS/TS compilation to target dispatch, preserves canonical representatives and Python execution identity, honors exact selectors, rejects ambiguity/suppression, and isolates Go targets after shared-workspace failure. |
| `cmd/repomap/runtime_portfolio_runtime_test.go` | Runtime evidence is retained in target inputs, page identity comes from validated published pages, persisted runtime artifacts are byte-identical, one full semantic validation is sufficient only when every remaining artifact still matches byte-for-byte, and evidence labels are bounded valid UTF-8. |
| `cmd/repomap/semantic_diagnostics_test.go` | Preflight/child failures flush exactly one first-layer semantic journal, failed runs do not create sibling run directories, and accounting metadata is updated exactly once. |
| `cmd/repomap/target_outcome_runtime_test.go` | Selected targets retain pre-analysis Go identity, failures map only to closed public stages/reasons, and one canonical target-outcome portfolio is written identically to all bound run directories. |
| `cmd/repomap/target_page_publication_quarantine_test.go` | A global publication failure leaves completed pages classified as analyzed, while quarantine removes their publication authority and product filenames. |
| `cmd/repomap/target_portfolio_runtime_test.go` | README and native scouts feed one exact file-ref portfolio; explicit targets bypass only portfolio selection; invalid JSON and authority loss fail closed; executable/default and supporting-library rules hold; repository naming strips semantic-major suffixes correctly. |

## Semantic cube and model-boundary contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `internal/activityentrypoint/activityentrypoint_test.go` | Compiled batches are digest-bound without leaking object inventory, candidate eligibility is complete, batch refs remain local, dynamic/seed/library candidates retain authority, and responses restore only known deduplicated activity refs into a sealed artifact. |
| `internal/activityentrypoint/prompt_contract_test.go` | The embedded prompt matches the closed selection contract and complete selections are accepted above the former global quota. |
| `internal/activitypath/activitypath_test.go` | Activity routes are classified and codec-stable; frontiers bind exact caller relations, route tie-breaking minimizes uncertainty deterministically, and seeded modules can form legitimate zero-hop activities. |
| `internal/activitysurface/activitysurface_test.go` | Only bounded advertised surface refs reach the model; strict response shape, incompatible proposals, missing substrate, empty candidates, and pre-call omissions are handled fail-closed or as the specified legitimate empty result. |
| `internal/coremap/coremap_test.go` | Language-neutral objects drive CoreMap; sharded map/reduce packing never slices facts, dynamic relations advertise only authorized evidence, invalid fields are named, unsupported proposal rows are discarded, empty grounded output is terminal, and exact deduplication preserves distinct claims. |
| `internal/coremap/grouping_pipeline_test.go` | Model-owned refined groups restore closed refs, enforce resource bounds, normalize set membership, preserve overlapping complete cover, account partial groups honestly, avoid exposing program IDs, round-trip sparse large topology, and derive stable group IDs from claim plus membership. |
| `internal/coremap/prompt_contract_test.go` | Embedded CoreMap map/reduce/group prompt prose remains aligned with the current request and response schemas. |
| `internal/entrycall/entrycall_test.go` | Compilation keeps the exact substrate private while advertising complete bounded surface authority; reduction restores only exact refs and rejects unknown refs atomically without dropping roots before catalog bounding. |
| `internal/integrationdependency/classifier_test.go` | Dependency selection restores exact dependencies and importers, refuses partial catalogs/missing arrays/oversized catalogs, filters unknown refs, preserves complete selections, shards by serialized bytes with global restoration, and keeps declarations/frontiers separate. |
| `internal/integrationusage/classifier_test.go` | Usage classification restores exact Go/Python callsite witnesses and aliases, rejects conflicting assignments or incomplete authority, filters unknown/cross-batch refs before bounds, deduplicates exact rows, keeps strict schemas, exhaustively batches high volume, and restores only typed external operations. |
| `internal/integrationusage/javascript_adapter_test.go` | JS/TS adaptation preserves exact external calls, unresolved frontiers, possible alternatives, language origin, and the producer's complete external-witness authority. |
| `internal/runtimeportfolio/compact_reduce_regression_test.go` | Higher-level runtime reduction replaces exact evidence catalogs with bounded validated summaries while retaining the complete hidden candidate authority, reaches one global batch without semantic truncation, and restores every selected role's exact evidence union. |
| `internal/runtimeportfolio/prompt_contract_test.go` | Runtime map/reduce prompts and contract versions are current, and an implementation cannot be selected without evidence for the matching target. |
| `internal/runtimeportfolio/runtimeportfolio_test.go` | Runtime roles/modes/mappings support single, multiple, and library-only targets; unknown refs and duplicate sets normalize locally while mandatory gaps fail; artifacts and cache identity are canonical/tamper-evident; sharding/reduction restores exact global authority; accounting distinguishes cached/live work; bounds reject rather than truncate; secret and evidence envelopes fail closed. |
| `internal/runtimeportfolio/sharded_state_test.go` | Map and reduce cache state is bound to the exact phase, shard/batch identity, schema versions, request bytes, and hidden candidate-authority digest, so distinct exact evidence cannot collide behind identical compact summaries. |
| `internal/targetportfolio/targetportfolio_test.go` | Candidate compilation and resolution use exact merged corpus refs, preserve mandatory native/executable authority, permit honest library-only catalogs, enforce the strict two-field schema, remain permutation-stable and request-bound, reject truncation/tampering, and block visible secrets. |
| `internal/targetviewchoice/targetviewchoice_test.go` | Target-view refs and state are canonical and owned, strict responses restore only exact authority, non-ambiguous/duplicate inputs are rejected, and execution uses the shared LLM executor. |

## Repository, target, and deterministic fact contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `internal/analysistarget/catalog_test.go` | Exact executable and one-per-module library surfaces are catalogued deterministically across nested modules; internal/main/empty/unprovable APIs are excluded; codecs reject tamper and display drift. |
| `internal/analysistarget/direct_call_scope_test.go` | Direct-call scope is bound to the complete exact target and explicit graph/resource bounds. |
| `internal/analysistarget/go_file_discovery_test.go` | Go file candidates project exact mains/public declarations, private canonical refs restore locally, ordering is stable, missing source authority and package/catalog drift fail closed. |
| `internal/analysistarget/hypothesis_test.go` | Candidate hypotheses union only by exact file ref, never fuzzy-match, reject unknown/bad hypotheses, and allow legitimate empty parallel results. |
| `internal/analysistarget/resolver_test.go` | Module directory is only a closed routing hint and resolution returns every exact candidate without prematurely selecting one. |
| `internal/analysistarget/roots_test.go` | Executable/library roots bind exact declarations and main locators, require the direct-call index, and distinguish true resource overflow from semantic omission. |
| `internal/analysistarget/scope_test.go` | Scoped Go facts retain only the exact executable or module-library package universe, filter dependency importers consistently, handle known repository shapes, and reject target-ref drift. |
| `internal/corpus/corpus_test.go` | The shared corpus is deterministic, supports empty input, excludes all forbidden credential/config/dependency/generated paths component-exactly, binds executable mode rather than contents to names-only identity, detects tamper/symlinks/replacement, bounds current reads, handles concurrent close/cancellation, and trusts only stage-zero regular index modes. |
| `internal/dependencies/catalog_test.go` | Dependency catalogs canonicalize identity/importers, persist exact validated bytes, support stable subsets, reject unknown importers/identity drift, and record honest omissions as partial coverage. |
| `internal/dependencydeclaration/model_test.go` | Declaration ledgers seal canonical authority and frontier sources require an exact boundary. |
| `internal/freshness/freshness_test.go` | Captured repository state includes only tracked changes, binds revision and dirty identity to inputs, rejects unavailable captured trees, and rejects duplicate paths in the digest. |
| `internal/gitfiles/gitfiles_test.go` | Git listing cancellation terminates the child command, NUL/index parsing admits only regular stage-zero paths, and the command environment neutralizes ambient Git configuration injection. |
| `internal/gocoreobject/index_test.go` | Go core-object declarations are canonicalized, sealed, independently snapshotted, and exact. |
| `internal/godynamichandoff/index_test.go` | Dynamic handoffs seal exact versus uncertain authority, reject interface candidates lacking value-flow evidence, and retain honest omission accounting. |
| `internal/gofacts/dependencies_test.go` | One Go inventory builds exact dependency kinds/importers across nested modules, deduplicates importers, keeps `DepOnly` metadata from contaminating roots, and marks missing/broken imports as partial. |
| `internal/gofacts/entrypoints_test.go` | Only build-selected, correctly signed `main` declarations become process anchors, resolved from exact selected files. |
| `internal/gofacts/gofacts_test.go` | Go loading requires an exact platform target, exhaustively covers modules/facts, fails on incomplete diagnostics or escaped module/replacement authority, handles cancellation and nested ownership, excludes gitlinks/test-only root rows, scopes exact modules, normalizes paths, and deterministically builds entrypoints/internal edges/external imports. |
| `internal/gofacts/package_declarations_test.go` | Declarations come only from build-selected non-test files, package extraction fails atomically, callable bodies and opaque exported receivers remain distinct, and canonicalization is strict and permutation-stable. |
| `internal/gotarget/target_test.go` | Explicit `GOOS/GOARCH` is atomic and wins over independent environment fallback; parsing rejects noncanonical targets and applying a target replaces the pair. |
| `internal/jstsproject/helper_test.go` | Owner-prepared local TypeScript compiler discovery binds source bytes and package ownership; package/bin/script identities, compiler aliases/fallback/ambiguity/exports, project references, workspace delegates, nested boundaries, exact selection, and credential scrubbing all remain closed and target-local. |
| `internal/jstsproject/model_test.go` | JS/TS artifact sealing/decoding/projection is exact; reserved platform authority, surface and CLI seed rules, unresolved/dangling refs, optional sensitive metadata/signatures, credential-bearing manifest facts, and target identity all validate or fail closed. |
| `internal/jstsproject/target_scout_test.go` | The corpus-only scout discovers every source-owning root/nested package without invoking the compiler, derives nameless identity safely, ignores source-less suppression, preserves exact scout identity, and permits only content-derived name drift at materialization. |
| `internal/pythondeclareddependencies/parser_test.go` | Requirements and PEP 621/Poetry declarations preserve package identity/location while redacting credentials, stay inside the selected project, and retain include/continuation source locations. |
| `internal/pythondependencies/catalog_test.go` | Python dependency projection distinguishes workspace, standard-library, and external imports; dynamic imports remain program frontiers and unnamed direct imports produce honest partial coverage. |
| `internal/pythonprogramindex/build_test.go` | Python indexing preserves dynamic/mutable frontiers, exact imports/call tokens/aliases/decorators/module seeds, omits sensitive display literals, visits nested expressions once, bounds interpreter output, shares parsing across ordered targets, keeps target semantics isolated, binds corpus identity, and indexes only selected modules. |
| `internal/pythontarget/artifact_persistence_test.go` | Python target catalogs persist canonical exact authority. |
| `internal/pythontarget/discovery_test.go` | Python packaging roots, guards, shebang executables, executable mode, corpus refs, omissions, anchors, and sealed module scope are deterministic; arbitrary objects are not promoted and tampering fails. |
| `internal/pythontarget/file_candidates_test.go` | Exact executable/library file candidates merge without module fanout, namespace packaging is supported, resolver-only views are not advertised, hypotheses are exhaustive/plain, and a corpus is mandatory. |
| `internal/pythontarget/file_target_resolver_test.go` | File resolution uses only sealed source authority, can project arbitrary framework-neutral modules and README-only projects, owns returned snapshots, rejects shared-file ambiguity, distinguishes unknown from unmatched refs, and binds corpus identity. |
| `internal/readmetargetscout/readmetargetscout_test.go` | Initial guidance compilation sends the complete lossless names-only tree, complete prose refs, and complete README/AGENTS text; responses are strict sparse set catalogs with local filtering/deduplication; incompatible roles are discarded; identity/tamper/bounds hold; guidance absence is legitimate; trusted prose is not secret-scanned by heuristic. |
| `internal/reporead/reporead_test.go` | Repository reads are size-bounded, reject unsafe paths and symlink escape, and require a directory root. |
| `internal/secretscan/secretscan_test.go` | Opt-in credential detection covers common config forms while distinguishing placeholders, selectors, prose/templates, and opaque bearer values; scanning is off by default; the always-on persistence-sensitive detector remains deliberately narrow. |
| `internal/snapshot/go_target_advisory_test.go` | Go target evidence is platform-only, automatic advice requires one strong product alternative, non-product paths are excluded, and selection remains bound to the exact advisory. |
| `internal/snapshot/snapshot_test.go` | Snapshot construction requires shared corpus/exact target, binds semantic repository identity, handles automatic versus explicit platform selection, keeps the complete unselected target catalog, scopes exact targets, isolates Git environment, treats symlinks safely, fails unavailable Go facts, and can skip incidental Go for another selected language. |
| `internal/sourcecatalog/catalog_test.go` | Source catalogs deterministically map repository/subdirectory analysis roots, prefer exact inputs, reject unsafe/ambiguous scopes and tracked symlinks, do not depend on current filesystem existence, and remain language-neutral without report authority. |

## Orientation, ProgramIndex, and page-authority contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `internal/cubemap/core_object_projection_test.go` | Core-object projection retains only exact representative callables/receivers and refuses to guess ambiguous generic receivers. |
| `internal/cubemap/pipeline_test.go` | Accepted activity roots pass through completely, dependency and semantic catalog coverage must be exhaustive, and exact external uses join from captured call facts without rereading source. |
| `internal/cubemap/surface_core_effect_binder_test.go` | Surface-to-core relations preserve direction/minimum hops and graph/declaration identity; reduction restores only local advertised pairs and rejects unknown/duplicate assignments. |
| `internal/orient/analysis_target_handoff_test.go` | Analysis-target handoff owns independent snapshot/metadata authority, preserves module-library identity without invented packages, and persists/projects multiple exact target runs. |
| `internal/orient/direct_call_index_handoff_test.go` | Direct-call indexes are live-run-only independent snapshots, reuse one existing SSA build, remain exact, and are skipped for disabled or non-Go runs. |
| `internal/orient/entry_call_substrate_handoff_test.go` | Entry-call substrate delivery hands off an independent snapshot rather than shared mutable state. |
| `internal/orient/external_call_index_handoff_test.go` | External-call index delivery hands off an independent exact snapshot. |
| `internal/orient/go_target_auto_test.go` | Automatic Go selection is finalized from the exact catalog before crossing the provider seam. |
| `internal/orient/precomputed_target_test.go` | A precomputed run retains the exact scoped target rather than rediscovering or widening it. |
| `internal/orient/prepared_workspace_portfolio_test.go` | Selected Go targets share one prepared union workspace when valid; union failure is not reused and each healthy exact target is retried locally. |
| `internal/orient/run_test.go` | Cancellation stops before snapshot publication, required artifacts require a usable debug directory, and blocked artifact destinations fail closed. |
| `internal/orient/surface_discovery_test.go` | Surface discovery delivers its result, gives module-library roots their full owning-module scope, and refuses to run without the required artifact directory. |
| `internal/pipeline/pipeline_test.go` | The backend owns one complete validated semantic chain, may stop only after persisted activity entrypoints, and never synthesizes missing declaration authority during dependency selection. |
| `internal/programindex/artifact_set_test.go` | ProgramIndex artifact sets persist/inventory canonical multi-target views, bind targets and indexes exactly, decode strictly, and reject invalid, noncanonical, or tampered content. |
| `internal/programindex/index_test.go` | ProgramIndex requires measured coverage/visibility/relations, validates ownership/resolution shapes and bounds, canonicalizes/seals strict codecs, preserves typed witness digests, exact/possible relations, envelope-versus-semantic budgets, identity-bound seeds/sources/selectors, and rejects all tampering. |
| `internal/programindex/goadapter/adapter_test.go` | The Go adapter projects existing facts without inventing semantics, supports value-only public APIs, requires corpus identity, includes Go flags in scenario identity, restricts SSA init ordinals, and merges identical unresolved caller frontiers. |
| `internal/programpage/portfolio_test.go` | Language-neutral page portfolios round-trip every exact target, require complete unambiguous safe target/run bindings, reject tamper/noncanonical bytes, and constrain portable run IDs. |
| `internal/snapshot/target_page_portfolio_test.go` | Legacy target-page portfolios round-trip canonical container bindings, require complete safe sibling manifest authority, and reject unsafe IDs, incomplete publication, tamper, and noncanonical bytes. |
| `internal/snapshot/target_run_container_test.go` | A deferred snapshot projects multiple exact targets and rejects invalid selection or artifact drift. |
| `internal/surfacediscovery/core_object_capture_test.go` | Core objects are captured from the existing typed program for the exact target, without a second semantic authority. |
| `internal/surfacediscovery/dynamic_handoff_capture_test.go` | Dynamic handoff capture reuses existing SSA and preserves runtime uncertainty instead of promoting candidates. |
| `internal/surfacediscovery/external_call_index_test.go` | External calls are opt-in, canonical, root-independent facts and reject callers outside loaded packages or artifact tampering. |
| `internal/surfacediscovery/module_library_target_test.go` | Module-library targets root every exact exported package only, seal canonical input/index authority, reject main packages, and fail closed at resource limits. |
| `internal/surfacediscovery/prepared_workspace_test.go` | Prepared workspaces keep compatible cross-module replacements in one typed universe, separate incompatible load contexts, and expose bounded exact diagnostic keys. |
| `internal/surfacediscovery/target_direct_call_test.go` | Direct-call analysis requires exact target/roots and explicit limits, bounds edges without losing declarations, closes on edge overflow, roots all exact public library APIs (including generic methods), and validates repository locations from one nonrecursive base. |
| `internal/surfacediscovery/test_helpers_test.go` | No direct test entrypoint; shared helpers construct temporary Go sources and invoke typed surface analysis. Their setup failures and compiler-backed assumptions are exercised by the six sibling surface-discovery test files. |
| `internal/targetoutcome/portfolio_test.go` | Target-outcome portfolios canonically retain analyzed and failed targets (including failed defaults and zero successes), bind every public selected-target field, enforce the analyzed/failure union, and reject duplicate, unsafe, tampered, or noncanonical bindings. |

## Persistence, debug, report, and serving contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `internal/debugdump/debugdump_test.go` | Debug exchanges contain only live stages and payloads, mark persistence-sensitive output, bound/deduplicate warnings, validate bytes after writes, account only live request state, preserve build identity and directory confinement, and reject ambiguous run directories. |
| `internal/debugdump/semantic_observer_test.go` | The observer persists raw invalid responses for diagnostics, flushes accepted bound exchanges, omits redacted Authorization requests, and explicitly marks unavailable redacted responses. |
| `internal/deepseek/client_test.go` | Environment configuration keeps generic and DeepSeek aliases separate, applies closed defaults/no-auth rules, rejects invalid or cross-family credentials, and always redacts explicit credentials from provider errors. |
| `internal/deepseek/llm_provider_test.go` | Provider preparation uses only cube prompt/state and bounded requests; state is credential-free; completion metrics/heartbeat/concurrency work; 429 collapses the shared gate before exact retry; retry bytes/accounting and terminal length/timeout/HTTP failures are structured and closed. |
| `internal/llm/execute_test.go` | Shared execution binds provider/cube/input state, revalidates and evicts unsafe/tampered cache entries, honors cache bypass and failure non-caching, preserves batch order and bounded parallelism, replays observers in caller order, cancels queued work on terminal failure, shares persistent rate-limit collapse, distinguishes semantic failure, retains accepted operational issues, protects credentials, and copies prepared bytes immutably. |
| `internal/llm/json_syntax_test.go` | JSON normalization accepts exactly one unambiguous object/array, rejects garbage/ambiguity/truncation, and never repairs schema, refs, or values. |
| `internal/report/analysis_target_manifest_test.go` | Snapshot parsing projects only validated analysis targets and binds report target identity exactly. |
| `internal/report/core_map_view_projection_test.go` | Report CoreMap keeps model grouping separate from exact evidence, retains relation frontiers and overlapping complete cover, requires language-specific artifacts without fallback, binds integration usage, and rejects missing/unbound/changed projections. |
| `internal/report/cube_map_view_projection_test.go` | CubeMap view projects exact joins and reverse navigation, permits the specified baseline child, and rejects invalid sources or over-limit projections. |
| `internal/report/english_language_test.go` | Rendering is canonical English-only and persisted metadata cannot reactivate removed language selection. |
| `internal/report/github_source_test.go` | GitHub repository URLs normalize only safe roots, infer exact origin identity, reject unsafe/non-root forms, and remain an HTML presentation concern. |
| `internal/report/gitlab_source_test.go` | GitLab URL/remote/revision/prefix authority is canonical and credential-safe, rejects missing/mismatched/unsafe sources and analyzed submodules, tolerates captured dirty state, preserves source content, and scrubs local roots from standalone payloads. |
| `internal/report/html_payload_test.go` | Ordinary HTML embeds one canonical report payload, rejects malformed/ambiguous data, preserves only validated standalone/render navigation, and omits backend producer digests and legacy Go target authority from the browser. |
| `internal/report/integration_usage_view_projection_test.go` | Only selected integration uses with exact source authority are published, and partially bound material authority is rejected. |
| `internal/report/js_ts_manifest_projection_test.go` | JS/TS manifests bind artifact, exact ProgramIndex, surfaces, paths, and source authority; substituting another otherwise-valid index fails. |
| `internal/report/js_ts_surface_catalog_view_projection_test.go` | JS/TS views preserve product surface kinds (including CLI), explicit HTTP cross-surface boundaries, exact citations, and reject authority drift. |
| `internal/report/manifest_cube_map_test.go` | Manifest verification reconstructs and compares the bound CubeMap view rather than trusting stored projection bytes alone. |
| `internal/report/manifest_fixture_test.go` | Standalone source authority in a run manifest is closed, canonical, and derived from a temporary captured repository. |
| `internal/report/manifest_program_index_test.go` | Manifest verification rebuilds the complete ProgramPortfolio and binds every ProgramIndex artifact to its set and exact target. |
| `internal/report/manifest_report_authority_test.go` | `report.json` requires exact captured and semantic ProgramIndex authority for Go/Python, while report and manifest decoders reject unknown fields, trailing values, and obsolete versions. |
| `internal/report/program_index_projection_test.go` | Run restoration rebuilds the default portfolio/openable paths from exact artifacts, keeps snapshot facts as inputs only, refuses missing Go target or mismatched set/snapshot/metadata, and never promotes invalid paths or package directories. |
| `internal/report/program_map_asset_test.go` | Rendered assets include the current orientation workflow and isolated canvas layers, omit retired surfaces and truncation, keep complete evidence/disclosures/strict source actions, order runtime before program detail, project JS/TS surfaces and overlapping groups, and require exact persisted portfolio/view authority. |
| `internal/report/program_page_portfolio_manifest_test.go` | Manifest verification binds the neutral page portfolio, enforces mutual exclusion with legacy page authority, binds runtime evidence to pages, and requires target-outcome analyzed rows to form an exact bijection with validated pages. |
| `internal/report/program_portfolio_projection_test.go` | ProgramPortfolio keeps every exact target and one default, rejects duplicates, fails presentation closed by language capability, and requires exact JS/TS surface authority. |
| `internal/report/program_view_projection_test.go` | Program views retain typed external authority, resolve exact seeds and bounded relations, and use relation limits without dropping the seed neighborhood. |
| `internal/report/publication_assessment_test.go` | Publication assessment fails closed when manifest authority is absent. |
| `internal/report/render_transaction_test.go` | Authorized report installation commits the manifest last, backing-only target runs lose local HTML, and any transaction failure removes every product filename. |
| `internal/report/report_app_behavior_test.go` | The embedded report application restores strict closed report data, preserves adapter-owned relation invocation text, groups and condenses exact connections without losing authority, routes hashes to real canvas targets, renders repository/target failure accounting, and exposes no inert navigation controls. |
| `internal/report/report_test.go` | Snapshot parsing exposes only material Go paths and includes every module boundary among material inputs. |
| `internal/report/runtime_portfolio_view_projection_test.go` | Runtime evidence extends initial authority without drift, preserves mappings/target coverage/library roles, requires atomic target-page or neutral-page binding, and verifies exact manifest/artifact/evidence projection. |
| `internal/report/standalone_program_page_bundle_test.go` | Neutral multi-target bundles publish exactly one owner HTML (including one-page cases), retain language-neutral portfolio authority, reject binding or resealed-payload drift atomically, and prohibit simultaneous legacy/neutral identity. |
| `internal/report/standalone_target_bundle_test.go` | Legacy standalone bundles publish canonical self-contained targets atomically, bind exact payloads/routes/assets, select before app startup, resolve hosted/file links, pin remote source and scrub local authority, reject legacy aliases/drift/resealed rewrites, preserve existing HTML on failure, detect tampering, and enforce aggregate limits. |
| `internal/report/system_canvas_modules_test.go` | Pure graph/interaction/geometry modules preserve their contracts, and a real browser interaction changes emphasis without rebuilding geometry. |
| `internal/report/target_navigation_test.go` | Target navigation projects exact language-neutral pages, keeps render options transient, creates portable sibling links, and rejects incomplete, tampered, unbound, or unsafe pages. |
| `internal/report/target_page_portfolio_manifest_test.go` | Manifest verification binds every legacy target-page portfolio artifact and sibling authority exactly. |
| `internal/reportserver/editor_test.go` | VS Code launch requires an installed CLI, passes the exact authorized target, waits for success, and surfaces dispatch failure. |
| `internal/reportserver/server_test.go` | The loopback server serves only manifest-authorized initial/virtual pages, requires portfolio routing for siblings, binds opaque source-open capabilities to host/origin/action, rejects raw paths/symlink replacement/report drift, and stops with context. |
| `internal/sourcecatalog/dependency_test.go` | The neutral source-catalog production package has no import dependency on presentation/report layers. |
| `internal/workspaceopen/dependency_test.go` | Workspace-open authorization remains independent of presentation/report packages. |
| `internal/workspaceopen/open_test.go` | Local opening resolves only exact authorized/current paths, rejects aliases, unavailable/unauthorized inputs, symlink/root replacement, returns typed cancellation, and never leaks authority through construction or errors. |
| `internal/workspacesnapshot/dependency_test.go` | Workspace-snapshot production code remains independent of presentation/report packages. |
| `internal/workspacesnapshot/snapshot_test.go` | Workspace snapshots build immutable source authority, require every allowed path to have captured input, and reject oversized authority rather than truncate. |

## Contract-inventory and cumulative-fixture contracts

| Test file | Contract and invariants asserted |
| --- | --- |
| `internal/contracttest/fixture_test.go` | No direct test entrypoint; shared fixture helpers copy one approved cumulative language fixture, reject symlinks/nonregular files, create an isolated Git index, verify the exact file inventory, and expose canonical artifact helpers to the language contract tests. |
| `internal/contracttest/go_repository_test.go` | The one cumulative Go fixture has exact tracked-file inventory and exercises real Go discovery through deterministic ProgramIndex construction with request-bound local presets. |
| `internal/contracttest/jsts_repository_test.go` | The one cumulative JS/TS fixture has an exact tracked-file inventory; every scenario file must be represented in the contract. |
| `internal/contracttest/production_limit_inventory_test.go` | Every governed production limit is inventoried, source-bound to the expected declaration, canonical, unique, and unchanged without an explicit contract update. |
| `internal/contracttest/prompt_inventory_test.go` | Every static prompt Markdown file is classified active/dormant, active prompts have exact `go:embed` ownership, and prompt inventory is complete and canonical. |
| `internal/contracttest/python_repository_test.go` | The one cumulative Python fixture has exact tracked-file inventory and exercises real interpreter-backed discovery/ProgramIndex construction with deterministic provider-free expectations. |
| `internal/contracttest/test_inventory_test.go` | `tests.json` exactly equals all owned Go and JS test files; package/file order, default profile, exception characteristics, external tools, fixtures, and detected nonordinary mechanics are complete and canonical. |

## Non-ordinary execution mechanics

All files not listed here use the inventory default: ordinary resources, no
network, a stubbed-or-absent provider, and inline-or-no fixture. This table is a
projection of the 39 explicit exceptions in `tests.json`; mechanics are kept
separate from the invariant descriptions above.

| Test file | Exceptional mechanics |
| --- | --- |
| `internal/contracttest/fixture_test.go` | Temporary repository fixture; invokes `git` as a subprocess. |
| `internal/contracttest/go_repository_test.go` | Cumulative Go fixture; invokes `git` and the Go compiler/toolchain in external processes. |
| `internal/contracttest/jsts_repository_test.go` | Cumulative JS/TS fixture; invokes `git` externally. |
| `internal/contracttest/python_repository_test.go` | Cumulative Python fixture; invokes `git` and `python3` externally. |
| `internal/corpus/corpus_test.go` | Temporary Git repository, subprocess and filesystem-platform behavior; some platform conditions may skip. |
| `internal/deepseek/client_test.go` | Reads/overrides ambient environment variables under test control. |
| `internal/deepseek/llm_provider_test.go` | Uses loopback HTTP networking. |
| `internal/freshness/freshness_test.go` | Temporary repository; invokes `git`. |
| `internal/gitfiles/gitfiles_test.go` | Invokes `git` and tests isolation from ambient environment. |
| `internal/gofacts/dependencies_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/gofacts/entrypoints_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/gofacts/gofacts_test.go` | Temporary repository; invokes the Go toolchain/compiler, observes controlled ambient environment, and has platform-dependent skips. |
| `internal/jstsproject/helper_test.go` | Cumulative JS/TS repository plus temporary projects; invokes Node/npm/TypeScript compiler paths and conditionally skips when toolchain prerequisites are absent. |
| `internal/jstsproject/model_test.go` | Optional ambient external checkout; invokes `git`, Node, and compiler paths and skips when the checkout/toolchain is unavailable. |
| `internal/orient/analysis_target_handoff_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/orient/direct_call_index_handoff_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/orient/go_target_auto_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/orient/precomputed_target_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/orient/prepared_workspace_portfolio_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/orient/run_test.go` | Temporary Git repository; invokes `git`. |
| `internal/orient/surface_discovery_test.go` | Temporary Git repository; invokes `git` and the Go toolchain/compiler. |
| `internal/pythonprogramindex/build_test.go` | Temporary repository; invokes the `python3` interpreter. |
| `internal/pythontarget/discovery_test.go` | Temporary repository; invokes `git`. |
| `internal/reporead/reporead_test.go` | Exercises filesystem-platform behavior and may skip where required semantics are unavailable. |
| `internal/report/manifest_fixture_test.go` | Temporary Git repository; invokes `git`. |
| `internal/report/report_app_behavior_test.go` | Runs embedded JavaScript in a Node VM subprocess and skips when Node is unavailable. |
| `internal/report/standalone_target_bundle_test.go` | Runs standalone bootstrap JavaScript in a Node subprocess and skips when Node is unavailable. |
| `internal/report/system_canvas_modules_test.go` | Runs Node pure-module checks and a Chrome/Chromium headless browser over loopback HTTP; runtime absence causes skips. |
| `internal/reportserver/editor_test.go` | Uses ambient executable lookup and launches a controlled test-binary subprocess. |
| `internal/reportserver/server_test.go` | Uses loopback HTTP and filesystem-platform behavior; unavailable platform semantics may skip. |
| `internal/snapshot/snapshot_test.go` | Temporary Git repository, `git` subprocesses, and filesystem-platform cases with conditional skips. |
| `internal/surfacediscovery/core_object_capture_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/dynamic_handoff_capture_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/external_call_index_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/module_library_target_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/prepared_workspace_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/target_direct_call_test.go` | Temporary repository; invokes the Go toolchain/compiler. |
| `internal/surfacediscovery/test_helpers_test.go` | Shared compiler-backed temporary-repository helpers used by sibling tests. |
| `internal/workspaceopen/open_test.go` | Exercises filesystem-platform behavior and conditionally skips unsupported symlink semantics. |

## Mechanical coverage result

The audit's unique `_test.go` paths were compared with the flattened
`go_packages[].directory + files[]` set in `testdata/contracts/tests.json`, and
the inventory itself was compared with the filesystem. Result: **140 mapped,
0 missing, 0 unmapped, 0 duplicate inventory entries**. The declared
`javascript_test_files` set and the discovered repository-owned JavaScript test
set are both empty: **0 mapped, 0 missing, 0 unmapped**. All **39** exception
paths appear in both the invariant catalog and the mechanics table.
