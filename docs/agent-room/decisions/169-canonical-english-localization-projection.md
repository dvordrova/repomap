# Decision 169: Canonical English with optional localization projection

## Status

Approved by the repository owner in the current session. Localization is the
next implementation priority. The held Python/D169 work is not part of this
decision and must not be committed.

## Problem

The existing `--lang ru` path changes semantic model requests themselves.
Consequently:

- English and Russian runs can retrieve, rank, validate, and cache different
  semantic outputs;
- Russian prose can enter canonical report artifacts and semantic cache
  identity;
- a localization failure cannot reliably preserve one complete canonical
  result;
- fixed product copy and repository/model-authored prose have different
  ownership but are not yet completed through separate mechanisms.

The repository already has two useful foundations:

- a provider-neutral allowlisted localization contract with protected opaque
  values; and
- one typed, versioned English/Russian product-message catalog with parity
  validation.

This decision connects those foundations to ordinary product runs without
changing semantic authority or canonical wire artifacts.

## Decision

### Canonical semantic language

Every non-localization model request uses one shared, versioned output-language
contract that requires English human-readable prose.

The contract is applied by the common semantic request constructor used by
Orientation and every generic semantic stage. The dedicated localization
request is the sole exception. A stage-specific prompt must not override the
canonical-English contract.

`--lang` does not change semantic prompts, retrieval, ranking, validation,
grounding, accepted facts, or semantic cache identity. English remains the
canonical language for:

- Orientation and targeted research;
- Architecture synthesis;
- Guided Tour;
- Study editing and review;
- semantic discovery and Mechanism editing;
- source, symbol, component, investigation, and provider-compatibility stages.

### Presentation projection

`--lang en` renders the canonical English result directly.

`--lang ru` first completes the same canonical English analysis, then performs
one separate localization request with:

- source locale `en`;
- target locale `ru`;
- the exact canonical English localization input;
- only allowlisted presentation fields;
- opaque placeholders for protected technical values.

The ordinary report-wide allowlist is limited to the already defined
localization owner kinds and fields:

- repository purpose and project guess;
- Architecture subsystem/component names and descriptions;
- Guided Tour title, summary, step title/explanation, and gap explanation;
- Study brief prose, conceptual-area name/responsibility, direction question,
  why, outcome, reading guidance, and search query;
- Mechanism title, question, answer, phase/step title, and explanation.

Warnings, evidence prose, source contents, diagnostics, closed enums, and
legacy presentation projections outside this list are not translated by this
slice.

IDs, keys, repository paths, symbols, package/module names, source locations,
evidence, links, product/library/API/protocol names, ordering, and structural
relationships remain byte-identical. The projection is applied to a copy for
rendering and never mutates the canonical report.

### Persistent cache and per-run presentation state

The translation cache is immutable and content-addressed. Its identity includes:

- the complete exact localization request body;
- target language;
- canonical input hash;
- provider endpoint;
- model;
- provider-request version; and
- translation-contract version.

A valid cache hit performs no provider call and is read before live API-key
configuration is required. A miss performs one localization request and writes
the complete validated projection atomically.

`--no-cache` bypasses both reading and writing the shared translation cache.
It does not suppress a successful per-run presentation result: the validated
projection and status needed to render that run remain beside the run as
presentation-only sidecars.

Every cache hit and run-sidecar read re-derives the canonical English input and
revalidates the projection before use. Missing, malformed, corrupt, unsafe, or
identity-mismatched cache content is a miss, never authority. A cache read,
cleanup, or write failure cannot invalidate an already validated per-run
projection.

### Failure behavior

Localization is optional. A localization network, decode, validation, or
sidecar failure:

- does not fail the analysis or report publication;
- preserves the canonical English `report.json` and existing manifest binding;
- keeps the requested Russian product-message catalog active;
- leaves untranslated model-authored prose in canonical English;
- writes and logs one bounded localization failure status; and
- shows an explicit product-catalog notice that Russian localization is
  unavailable and canonical English is being shown.

Partially English model output and English fallback must never be labelled or
silently presented as successful Russian localization. A successful Russian
projection is also explicit in the presentation status.

### Fixed product copy

Fixed product copy continues to use the single Decision 167 catalog:

- one explicit catalog version;
- identical typed message IDs and parameter contracts for English and Russian;
- parity validation before use;
- no DOM translation, English-string search, regex replacement, component-local
  dictionaries, or model-generated fixed chrome.

Repository- and model-authored presentation prose does not enter the catalog;
it is handled only by the allowlisted localization projection above.
Product-owned deterministic copy is excluded from that model allowlist even
when it is stored beside model-authored report data.

### Persistence and publication safety

Provider output is secret-scanned before it can enter the shared cache or
per-run sidecar. After a potentially long localization request, the captured
repository inputs are reconciled again before the final authority-bound report
is rendered; authority confirmed before translation is not silently reused
after repository changes.

## Canonical and wire compatibility

`report.json`, run manifests, HTTP DTOs, source authority, source-open behavior,
IDs, links, and canonical semantic artifacts remain unchanged in shape and
English content. Requested locale and presentation status live in existing run
metadata plus bounded presentation-only sidecars. The browser receives a
localized rendering copy only after the manifest-bound canonical report has
been verified.

An RU sidecar is consumed only when the run explicitly requested `ru`.
Existing saved runs without Decision 169 sidecars remain canonical English
model prose and are never inferred to have a successful projection from their
historical locale marker.

## Explicitly not authorized

- Python support or D169 Python changes;
- translating facts, evidence, source, identifiers, paths, packages, symbols,
  links, or canonical artifacts;
- changing retrieval, ranking, grounding, validators, HTTP DTOs, report JSON,
  manifest format, or source authority;
- a second report format, DOM translator, translation regexes, or component
  dictionaries;
- a Russian-to-English round trip;
- a new provider, language adapter, Search surface, MCP surface, or UI redesign.

## Acceptance

- every non-localization request contains the exact shared canonical-English
  contract, and no stage-specific prompt asks for Russian output;
- the localization request receives canonical English input and requested
  target locale without the semantic-English wrapper;
- cache hit, miss, endpoint/model/contract-version changes, corrupt cache, and
  `--no-cache` behavior are provider-free tested;
- network or projection failure leaves the canonical run publishable, keeps
  the Russian fixed-copy catalog active, and visibly reports that model prose
  remains canonical English;
- valid cache hits work without a live API key, and cache I/O errors do not
  discard an already validated per-run projection;
- partially English RU output and secret-bearing provider output are rejected
  before persistence;
- repository freshness is re-confirmed after localization before final
  authority-bound rendering;
- all protected values and all non-allowlisted report data are byte-identical
  before and after projection;
- English/Russian product catalogs have identical IDs and parameter contracts;
- focused JSON/replay tests require no live provider;
- one local English render, one successful Russian render, and one Russian
  degradation render are exercised end to end;
- `./scripts/check.sh`, provider-free etcd check, and `git diff --check` pass.

## Stop condition

Stop with the last green checkpoint rather than widening scope if completing
this slice would require:

- changing canonical report or manifest wire formats;
- translating a field without a stable owner ID;
- trusting an unvalidated projection or cache record;
- changing semantic acceptance, retrieval, ranking, or grounding;
- sending source contents or evidence outside the existing bounded semantic
  contracts; or
- introducing Python or another language adapter.

## Migration and rollback

Historical semantic cache entries generated for Russian output are never
selected by the new canonical-English path. Historical reports remain readable
as canonical saved data but are not claimed to have a Decision 169 projection.

Rollback removes the presentation sidecars and ordinary projection wiring.
The manifest-bound canonical English report remains valid throughout.
