# Decision 165: Provider-free Architecture localization stage replay

## Status

Approved by the repository owner through the explicit instruction to continue
the localization night plan after the completed Decision 164 checkpoint.

## Problem

Decision 164 proves that one strict Russian projection can be applied to the
freshly re-derived canonical English Architecture Canvas. It deliberately does
not define the model-stage request that would produce that projection.

An exact projection cache cannot be designed honestly before the exact
localization request exists. Persisting only a hand-supplied projection now
would create a content record, not the request/provider identity required by a
future cache. Wiring the current product UI first would likewise leave the
model-authored Architecture prose on the old locale-dependent semantic path.

The next checkpoint must therefore establish one bounded deterministic
localization prompt and replay a saved/fake response through the existing
Decision 164 validation boundary, without adding transport, cache, or product
wiring.

## Decision

Add one provider-neutral localization prompt contract and one explicit
developer operation:

```sh
make localization-stage RUN=<run-dir> [RESPONSE=<projection-json>]
```

which invokes:

```sh
repomap dev localization-stage <run-dir> [<projection-json>]
```

With no response path, the operation emits the exact compact versioned
localization prompt JSON plus one newline. With a response path, it invokes one
injected provider seam backed only by that bounded local file, strictly decodes
the returned `localization.Projection`, and emits the existing Decision 164
Architecture replay JSON plus one newline.

The stage always re-derives the current eligible canonical English
Architecture Canvas from saved synthesis and current local facts. It never
trusts the Decision 163 sidecars as input authority.

### Prompt contract

- source locale is exactly `en` and target locale is exactly `ru`;
- prompt version, system instruction, user instruction, exact output JSON
  shape, and embedded `localization.Input` are deterministic;
- only the already allowlisted Architecture subsystem/component prose and
  typed placeholders enter the prompt;
- the prompt explicitly requires every field ID to be copied exactly, every
  placeholder to be preserved exactly, Russian presentation prose, and no
  markdown or additional fields;
- exact paths, symbols, packages, products, protocols, APIs, libraries, opaque
  IDs, and other protected values remain placeholders or structured IDs;
- prompt bytes are bounded to 1 MiB, valid UTF-8, secret-scanned, and stable
  for identical canonical input.

The prompt is provider-neutral evidence. It is not yet an OpenAI-compatible
HTTP request and does not claim a provider/model/endpoint cache identity.

### Saved/fake response replay

- the injected provider seam receives the typed exact prompt once;
- the explicit developer file adapter returns only the supplied local bytes
  and never opens a network connection;
- response bytes are bounded to 1 MiB before hash, decode, validation, or
  secondary work;
- malformed JSON, unknown/trailing fields, invalid UTF-8, obvious credentials,
  and oversized responses fail closed;
- projection envelope mismatch retains the complete canonical English Canvas,
  while invalid individual translations retain canonical English only for
  those fields, exactly as in Decision 164;
- no retry is performed: transport/retry policy belongs to the future shared
  provider executor;
- provider errors are not echoed with potentially secret-bearing contents;
- cancellation before the injected call prevents the call.

The stage writes no run artifact, response, projection, status, cache,
manifest, report, or debug file.

## Explicitly not authorized

- a live provider or network call;
- an OpenAI/DeepSeek-specific request adapter or a new direct HTTP client;
- a projection cache, cache lookup, cache-hit claim, provider identity, stage
  manifest, or persisted Russian artifact;
- automatic localization during an ordinary `repomap` run;
- changing `--lang`, `deepseek.Client.OutputLanguage`, semantic request bytes,
  report JSON/HTML, manifests, HTTP routes, source authority, or browser UI;
- Russian-to-English translation or a locale round trip;
- expanding localization beyond the Architecture Canvas prose authorized by
  Decision 162.

## Acceptance

- two prompt builds from the same current run are byte-identical;
- the prompt contains every expected stable field ID and placeholder but no
  protected repository value;
- preview and injected-stage prompt bytes are exact;
- one saved Russian response produces bytes identical to direct Decision 164
  replay and leaves the run directory unchanged;
- one injected provider call occurs, with no retry on malformed or rejected
  output;
- cancellation prevents the injected call;
- oversized, malformed, secret-like, symlinked, and unsafe response files fail
  without network or writes;
- field and envelope fallback behavior remains unchanged;
- focused tests and the full repository check pass provider-free.

## Migration and rollback

This decision adds only a provider-neutral prompt contract and an explicit
provider-free developer replay. Rolling it back removes the prompt/stage
helpers, CLI/Make entrypoint, tests, and this decision. No persisted artifact,
ordinary report, cache, provider, manifest, HTTP, or browser contract requires
migration.
