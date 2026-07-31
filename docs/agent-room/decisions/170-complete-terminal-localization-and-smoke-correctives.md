# Decision 170: Complete terminal localization and smoke correctives

## Status

Approved by the repository owner in the current session. This decision expands
the active localization slice and supersedes Decision 169 only for the
implementation scope below. Decision 169 remains historical and must not be
rewritten. The held Python work remains excluded and uncommitted.

## Terminal localization projection

English remains the canonical language of every semantic/model stage, cache,
fact, ID, and `report.json`. Russian is one optional terminal presentation
projection applied only after canonical analysis and authority publication:

```
canonical English report
→ complete typed PresentationTextInventory
→ one English-to-Russian provider request on cache miss
→ complete validated Russian projection
→ render-only copy
```

Every visible string has exactly one classification:

- fixed product copy is rendered through the single typed EN/RU message
  catalog;
- repository- or model-authored prose is addressed by the typed
  `PresentationTextInventory` and included in the single projection;
- paths, symbols, IDs, URLs, source/code, enums, packages, exact technical
  identities, and canonical evidence remain opaque and byte-identical.

Presentation field IDs are stable presentation addresses, not semantic IDs.
Objects use their stable object IDs; singleton fields use stable schema paths.
The inventory covers all visible repository guide/thesis/system-story prose,
first-file reasons, Orientation directions, Study, Architecture, Guided Tour,
Mechanism, and other visible repository/model explanations. Raw evidence stays
canonical English authority even when a separate presentation copy of its
explanation is translated.

Russian status is `succeeded` only when the projection contains every expected
prose address, contains no unknown address, preserves every placeholder, passes
target-language and secret validation, and applies atomically. An oversized
single request, provider/cache/validation failure, or partial projection keeps
the Russian product catalog, canonical English model prose, and an explicit
Russian degradation banner. It never triggers batching, DOM translation, a
second translation strategy, or another cache layer.

Canonical `report.json`, semantic caches, snapshot, bundle, retrieval,
grounding, manifest, HTTP/source DTOs, source authority, source hashes, and
navigation do not change. EN and RU runs over the same repository state retain
the same canonical report domain.

## Smoke-test correctives

The same owner approval authorizes these bounded corrections discovered by
ordinary product smoke tests:

1. Publication traces distinguish model expansions from local evidence-only
   bundles. `--flows 0` reports `expanded=0`, a separate `local_bundles` count,
   and `state=not_requested`.
2. The README sentence splitter preserves inline code, URLs, semantic versions,
   decimals, and repository paths such as `go run ./cmd/server`.
3. The serve run picker distinguishes same-minute runs with repository,
   requested locale, cache mode, `HH:mm:ss`, and a short run ID. Date formatting
   follows the report language rather than the browser locale. `/api/runs` may
   gain only bounded optional presentation metadata; source routes are
   unchanged.
4. Offline Russian runs use the explicit reason `offline_requested`. Because no
   external localization call occurs, they do not perform a second freshness
   capture/render pass.
5. General tracked-source classification recognizes ordinary TS, JS, Python,
   Java, Rust, C, and C++ sources. A small repository source file is not dropped
   solely because its name lacks `server` or `main`. This does not add a TS
   parser.
6. Study discovery without a supported deep adapter returns a typed
   `no_supported_source_adapter` or `no_eligible_source_functions` outcome
   instead of a missing research-file error.
7. Study scheduling preserves the complete canonical proposal through review.
   It must not pre-filter evidence-backed candidates, impose a smaller output
   cap, or otherwise change accepted Study IDs or artifacts. Provider-cost
   optimization is deferred to a separate cache/batching layer outside this
   decision.

## Verification

Verification is provider-free:

- focused localization and EN/RU canonical JSON comparisons;
- a fake HTTP provider proving RU miss is one localization call after
  orientation, RU hit is zero calls, RU `--no-cache` is one localization call,
  and EN never localizes;
- a rich sentinel fixture proving no English prose sentinel survives a
  successful RU render while opaque sentinels remain byte-identical;
- JavaScript syntax and EN/RU message-catalog parity;
- focused publication trace, sentence splitter, offline, source-extension,
  Study diagnostic, and Study canonical-preservation tests;
- one browser serve smoke covering EN/RU selection, picker labels/date,
  degradation banner, source-open, and console errors;
- one final `./scripts/check.sh`.

Live provider calls are prohibited for this verification. Expected fake-provider
warnings from deliberately unsupported responses are not failures.
