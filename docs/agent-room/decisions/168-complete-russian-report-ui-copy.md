# Decision 168: Complete Russian report UI copy

## Status

Approved by the repository owner through the instruction to continue after the
green Decision 167 checkpoint. The current product supervisor selected this as
the next ordinary Russian-copy slice.

## Problem

Decision 167 established one explicit typed English/Russian product-message
catalog and removed the post-render translation machinery. Its migration
preserved every prior Russian value exactly, including entries that had never
been translated.

The resulting catalog has 1052 entries per locale, but 750 Russian renderers
are still byte-identical to English:

- 363 under `main.*`;
- 254 under `architecture.*`;
- 133 under `surfaces.*`.

These are now visible ownership gaps rather than runtime fallbacks. They make a
Russian report mix languages in navigation, diagnostics, Architecture
inspectors, surface details, and dynamically rendered status text.

## Decision

Replace every byte-identical English renderer in the Russian catalog when it
contains product-owned prose.

Keep exact English bytes only when the complete rendered value is an opaque
technical value rather than product copy:

- `surfaces.value.http` (`HTTP`);
- `surfaces.identity.http_route` (`{method} {path}`);
- `surfaces.location.open` (`{location} ↗`).

English technical terms may remain inside otherwise Russian copy when they are
names or established identifiers, including Go, gopls, API, HTTP, CLI, RPC,
Raft, VS Code, FlowProof, provider/model names, repository paths, symbols, and
package names.

Dynamic messages keep the exact parameter contract from Decision 167.
Repository- and model-owned parameters remain byte-exact and are never
translated or scanned. Russian grammar is implemented only inside the Russian
formatter; no English-to-Russian runtime lookup and no Russian-to-English path
is added.

## Explicitly not authorized

- localization of model-authored Orientation, Architecture, Guided Tour,
  Study, operation, or investigation prose;
- changes to semantic prompts, `OutputLanguage`, providers, retries, caches,
  facts, ranking, selection, report JSON, manifests, HTTP, source authority, or
  navigation;
- a DOM translator, regex translation, runtime machine translation, or
  compatibility fallback;
- changing English catalog values or message IDs;
- new product surfaces or presentation redesign.

## Acceptance

- English and Russian catalogs retain exact ID and parameter parity;
- every Russian static or dynamic renderer that is byte-identical to English is
  one of the three explicit opaque-value exceptions above;
- representative Russian messages cover main report chrome, saved-run
  diagnostics, dynamic counts/progress, Architecture actions/labels/copy, and
  surface filters/status/details;
- count formatting is checked for `1`, `2`, `5`, `11`, and `21`;
- Unicode repository paths, symbols, packages, model prose, IDs, and links
  remain byte-exact parameters;
- provider-free English and Russian saved-run replay produces unchanged
  canonical report JSON;
- the Russian UI checklist is exercised through a local served report;
- focused tests, `./scripts/check.sh`, provider-free etcd check, and
  `git diff --check` pass.

## Stop condition

Stop this checkpoint without broadening it if a remaining English string:

- comes from repository/model-owned data rather than the typed UI catalog;
- is a closed persisted enum that would require changing semantic data;
- requires a new message parameter or report field to translate safely;
- would require semantic regeneration, a provider request, or runtime
  translation.

## Migration and rollback

This commit changes only Russian values in the existing typed catalog and its
tests. Rollback restores the Decision 167 copy; no saved run, cache, report,
manifest, or source-authority migration is required.
