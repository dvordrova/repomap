# Decision 164: Provider-free Architecture Russian projection replay

## Status

Approved by the repository owner through the explicit instruction to continue
the localization night plan after the completed Decision 163 checkpoint.

## Problem

Decision 163 persists an exact English Architecture localization identity, but
does not yet prove the useful direction of travel: applying one Russian
projection to that same canonical semantic result.

The next checkpoint must demonstrate `EN canonical -> RU presentation` without
silently introducing a live provider call, a projection cache, ordinary
product wiring, or an unnecessary Russian-to-English translation. It must also
keep malformed provider-shaped data from changing paths, symbols, IDs, members,
relations, or any other structured Architecture field.

## Decision

Add one explicit developer operation:

```sh
make localization-replay RUN=<run-dir> PROJECTION=<projection-json>
```

which invokes:

```sh
repomap dev localization-replay <run-dir> <projection-json>
```

The operation re-runs the Decision 163 current-English eligibility proof from
the saved synthesis/status and current local facts. It does not trust or
consume the three B3 sidecars as cache authority. It then reads one explicitly
supplied, bounded, provider-free Russian projection fixture and emits one
deterministic compact replay JSON value to stdout.

The replay contains only its version, canonical SHA-256, locale, fallback
status, bounded diagnostics, and the projected Architecture Canvas. It is not
written into the run, cached, added to a manifest, or consumed by an ordinary
product path.

### Replay contract

- the canonical English artifact must still be eligible and byte-exact for the
  current saved Architecture Canvas under Decision 163;
- the Russian input is rebuilt locally from that canonical artifact with
  `source_locale=en` and `target_locale=ru`;
- the supplied fixture is a single strict version-1 `localization.Projection`
  JSON value, bounded to 1 MiB before decoding;
- projection version, locale, and canonical hash mismatches retain the complete
  canonical English Canvas with one deterministic envelope diagnostic;
- missing, oversized, or placeholder-invalid individual translations use the
  existing canonical English field fallback and produce deterministic bounded
  diagnostics;
- unknown translation IDs cannot change structured fields and produce the
  existing deterministic diagnostics; unknown JSON fields and raw invalid
  UTF-8 fail before replay;
- applying the replay may change only the existing allowlisted Architecture
  subsystem/component prose fields. Every structured field remains exact.

The output is bounded to 1 MiB and raw plus decoded typed values are
secret-scanned before stdout. Errors report only the detected credential kind,
never projection contents.

### Input and output boundary

- the projection fixture must be a non-empty regular non-symlink file;
- its size is checked before reading and decoding;
- raw UTF-8, secret content, unknown fields, and trailing JSON fail closed;
- the replay is rebuilt from current facts and is byte-identical for identical
  inputs;
- the run directory, English identity sidecars, and unrelated artifacts remain
  untouched.

## Explicitly not authorized

- any live provider or network call;
- a Russian-to-English translation or round trip;
- automatic translation during an ordinary `repomap` run;
- any new run artifact, exact projection cache, cache lookup, or cache-hit claim;
- changing `--lang`, `deepseek.Client.OutputLanguage`, semantic request bytes,
  report JSON/HTML, run manifests, HTTP routes, source authority, or browser UI;
- accepting markdown/prose extraction as a localization response in this
  provider-free replay checkpoint;
- expanding localization beyond the Architecture Canvas fields authorized by
  Decision 162.

## Acceptance

- a valid saved-shaped Russian projection produces deterministic replay JSON;
- repeated replay is byte-identical and leaves the run directory unchanged;
- a field-level placeholder failure preserves canonical English for exactly
  that field and records the reason;
- envelope/hash/locale/version failure retains the complete canonical Canvas
  with one deterministic diagnostic;
- malformed JSON, raw invalid UTF-8, oversize input, secret-like content, and
  symlinks fail before replay;
- replayed Russian prose preserves protected paths, symbols, packages, product
  names, Unicode, and CJK bytes;
- applying the projection changes only allowlisted Architecture prose;
- focused tests and the full repository check pass provider-free.

## Migration and rollback

This decision adds only an explicit replay consumer for the already isolated
Architecture identity. Rolling it back removes the CLI/Make entrypoint, replay
function, tests, and this decision. No run artifact, ordinary report, cache,
manifest, provider, HTTP, or browser contract requires migration.
