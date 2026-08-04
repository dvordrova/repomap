# Decision 167: Typed report UI message catalog

## Status

Approved by the repository owner through the instruction to continue the
localization/cache night plan after the green Decision 166 checkpoint, with
the product boundary confirmed by the current product supervisor.

## Problem

The report currently localizes product-owned English UI after rendering:

- a large exact-English-to-Russian dictionary;
- regular expressions that infer parameters from already-rendered prose;
- a `TreeWalker` over arbitrary text nodes;
- a body-wide `MutationObserver` that rewrites later DOM mutations.

That makes localization depend on English punctuation and DOM timing. It also
makes it difficult to distinguish product copy from repository paths, symbols,
package names, source text, and model-authored semantic prose.

Decision 166 deliberately does not connect Architecture semantic projection to
the ordinary product. Adding that connection before the renderer has one
explicit UI ownership boundary would preserve two incompatible localization
models and make failures harder to explain.

## Decision

Replace the existing report DOM translator atomically with one versioned
English/Russian product-message catalog shared by:

- `templates/script.js`;
- `templates/architecture_canvas.js`;
- `templates/surface_catalog.js`;
- the fixed product-owned labels in `templates/report.html` that are rendered
  while JavaScript is available.

Every product-owned string is selected by a stable message ID at its render or
update site. Dynamic messages use declared named parameters. Repository- and
model-owned values are passed only as opaque parameters and are never looked up,
split, scanned, or translated by the catalog.

The runtime message API fails explicitly for:

- an unknown message ID;
- a missing required parameter;
- an undeclared parameter;
- a catalog whose English and Russian ID/parameter shapes differ.

English and Russian catalogs are both explicit. English is not an implicit
fallback. A missing Russian product message therefore cannot silently appear
in English.

Fixed template nodes use explicit message IDs rather than their rendered text
as lookup keys. Dynamic nodes call the same message API before insertion. Once
all current sites have parity, remove the exact-string dictionary, regular
expression translator, `TreeWalker`, `MutationObserver`, and the generic
`translateUI` callback.

The one `<noscript>` notice is an explicit server-rendered EN/RU exception:
the JavaScript catalog cannot execute when JavaScript is disabled. It remains
one fixed no-parameter node selected only from the report language. No other
template-localized copy or second translation runtime is permitted.

## Ownership boundary

The catalog owns only product chrome: navigation, headings, buttons, status
copy, count labels, accessibility labels, and renderer diagnostics.

It does not own:

- repository paths, symbols, packages, modules, protocols, product names, or
  source code;
- model-authored Orientation, Architecture, Guided Tour, Study, operation, or
  investigation prose;
- closed semantic enums or persisted report values.

Parameters that contain any of those values remain byte-exact.

## Explicitly not authorized

- ordinary Architecture localization projection or a change to `--lang`;
- provider, executor, retry, cache, or request-shape changes;
- Russian-to-English translation;
- a compatibility DOM translator or per-page fallback;
- new product copy, report fields, manifest fields, HTTP routes, source
  authority, navigation behavior, or presentation surfaces;
- changes to semantic-stage `OutputLanguage` propagation;
- broad CSS or browser redesign.

## Acceptance

- the old dictionary, regex translator, `TreeWalker`, `MutationObserver`, and
  generic translation callback are absent;
- every product-owned static and dynamic UI site uses a declared message ID;
- the sole server-rendered `<noscript>` exception has exact EN/RU coverage and
  no other template-localized copy exists;
- English and Russian catalogs have identical IDs and parameter contracts;
- unknown IDs and missing or extra parameters fail explicitly;
- plural/count formatting is covered for `1`, `2`, `5`, `11`, and `21`;
- provider-free English and Russian render replay covers Overview, Study,
  detail/drawer, Architecture, surfaces, and dynamic status updates;
- repository/model prose, paths, symbols, packages, source text, IDs, links,
  and line numbers remain byte-exact;
- report JSON, manifest, HTTP, source-open, and ordinary semantic behavior are
  unchanged;
- focused tests, full checks, and the provider-free etcd check pass.

## Stop condition

Stop this checkpoint without ordinary product wiring if complete removal of
the old translator would require:

- a hybrid catalog plus DOM rewrite;
- guessing whether a value is product- or repository-owned;
- changing a wire schema or semantic artifact;
- localizing only one page while leaving the shared renderer ambiguous.

## Migration and rollback

This is one renderer-owned conversion. Rollback restores the previous embedded
assets and Decision 166 as current; no saved run, report JSON, cache, or
provider artifact requires migration.
