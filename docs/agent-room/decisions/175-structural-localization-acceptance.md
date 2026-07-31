# Decision 175: Structural localization acceptance

## Status

Approved by the repository owner after the Decision 174 diagnostic checkpoint.

## Problem

Decision 174 proved that a complete, index-bound, placeholder-safe Russian
projection was rejected only because familiar technical prose retained Latin
spelling such as `HTTP`, `Go`, `Grafana`, and `curl`. That lexical predicate is
not a safety or identity boundary. It suppresses useful translated output and
invites unsafe glossary and exemption heuristics.

## Decision

Localization acceptance is structural. A projection remains fail-closed for:

- invalid response JSON or response schema;
- missing, duplicated, reordered, or extra positional translations;
- canonical hash or locale mismatch;
- missing, duplicated, altered, or invented opaque placeholders;
- invalid UTF-8, oversized values, secret-scan failure, unsafe cache data, or
  an incomplete batch set.

Residual Latin, CamelCase, acronyms, product names, commands, or partially
English prose do not reject an otherwise structurally valid projection. Typed
placeholders continue to preserve locally owned paths, symbols, packages,
URLs, and other opaque identities, but are not expanded to satisfy a lexical
quality rule.

The provider response shape, prompt, cache namespace, canonical English
report, presentation inventory, UI, retry behavior, and batching remain
unchanged. Previously rejected responses were not cached; no migration or
legacy reader is added.

## Proof

- a provider-free projection containing Russian prose plus `HTTP`, `Go`,
  `Grafana`, and `curl` is accepted, applied, and cached;
- opaque placeholders are still restored byte-for-byte;
- an extra placeholder still rejects the batch and prevents cache write;
- existing strict provider-response tests retain positional index and schema
  rejection.
