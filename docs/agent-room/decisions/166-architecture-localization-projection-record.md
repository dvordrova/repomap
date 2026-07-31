# Decision 166: Immutable Architecture localization projection record

## Status

Approved by the repository owner through the instruction to continue the
localization/cache night plan after the green Decision 165 checkpoint.

## Problem

Decision 165 proves one exact provider-neutral English-to-Russian prompt and
one saved-response replay. It does not yet provide the final provider request
body or a safe reusable projection record.

The existing research and Architecture caches cannot be reused:

- their identities omit the exact endpoint and some output-shaping request
  parameters;
- they do not consistently bind the final serialized provider request body;
- their older readers and writers are weaker than the root-confined bounded
  localization artifact boundary.

Calling the Decision 165 prompt hash a provider request hash would therefore
be false. A fixed mutable `architecture.ru.json` slot would also hide
identity changes and introduce overwrite races.

## Decision

Add one explicit provider-free developer operation:

```sh
make localization-record RUN=<run-dir> [RESPONSE=<projection.json>]
```

which invokes:

```sh
repomap dev localization-record <run-dir> [<projection.json>]
```

The operation builds the exact Decision 165 prompt, builds one deterministic
OpenAI-compatible request body without requiring an API key or making a
network call, computes a versioned request/projection identity, and uses an
immutable content-addressed record below:

```text
<run-dir>/.localization-projections/v1/architecture-<64 lowercase hex>.json
```

Without `RESPONSE`, it performs lookup only. A missing exact path returns a
typed `miss_not_found` result and performs no provider work. With `RESPONSE`,
the local file is read only after a miss, validated through the current
Decision 164 boundary, and recorded only when every field is accepted with no
fallback diagnostic.

This is a projection record and replay proof. It is not yet the shared
transport cache or an ordinary product cache.

## Exact request identity

The content key is SHA-256 over deterministic compact JSON containing:

- record schema and stage ID;
- Architecture localization projector version;
- canonical English SHA-256;
- localization input and projection schema versions;
- source and target locales;
- Decision 165 prompt version, exact byte count, and exact SHA-256;
- provider request adapter version and provider kind;
- canonical non-secret endpoint identity;
- auth mode, never the API key or headers;
- exact model;
- max tokens, temperature including nil versus zero, response format,
  thinking mode, and reasoning effort;
- exact final serialized provider request byte count and SHA-256.

The deterministic provider-free request builder uses the Decision 165 System
and User messages exactly. It does not reuse the ordinary output-language
wrapper and does not add another locale instruction. It uses JSON object mode,
temperature zero, the configured/default max-token value, and disables
DeepSeek thinking only for the official DeepSeek endpoint. Any future request
policy change increments the adapter version and produces a different key.

Provider identity scalars are limited to 4 KiB before URL parsing or whitespace
scanning. The standalone request builder and record boundary share a 2 MiB
limit for the exact request; the normalized projection remains limited to
1 MiB, and the base64-bearing complete encoded record or command result is
limited to 5 MiB. These are safety bounds, not product payload tuning.

Endpoint identity accepts only HTTP(S), rejects userinfo, query parameters,
and fragments, lowercases scheme and host, removes the default port, and
preserves the escaped endpoint path. Authentication secrets, timeout, retry
count, usage, latency, and timestamps never enter the key or record.

## Record contents and replay

One record contains:

- the complete exact identity and key;
- the exact bounded provider request bytes plus their hash;
- one normalized strict accepted `localization.Projection` byte sequence plus
  its hash.

The supplied response bytes themselves are not retained as an arbitrary raw
transport artifact. Decision 165 returns projection content rather than a
complete HTTP response envelope; raw transport response persistence belongs
to the later shared provider executor.

Every hit:

1. re-derives the current eligible canonical English Architecture input;
2. rebuilds the exact prompt and provider request;
3. bounds and strictly validates the record;
4. verifies key, identity, request bytes, projection bytes, hashes, secrets,
   and fixed path;
5. replays the saved projection through Decision 164;
6. requires a complete Russian result with no fallback or diagnostics.

Saved acceptance flags are never trusted.

## Filesystem and concurrency policy

- The record directory is separate from the exact three-file Decision 163
  `localization/` set.
- The run directory leaf must be a real directory. Its file identity is pinned
  during preparation and checked again on every record read or write.
- All paths below the verified run root are fixed or derived from a verified
  lowercase hexadecimal key.
- Symlink or non-directory record roots and symlink/non-regular leaves fail
  closed.
- Reads check the leaf before and after open, require the same file identity,
  and apply a fixed byte limit before JSON decoding.
- Writes use a random `O_EXCL` 0600 temporary file, file sync, an atomic
  no-replace hard-link publication, temporary cleanup, and directory sync.
- Concurrent same-key writers never overwrite each other. The loser reloads
  and validates the immutable winner.
- Loaded bytes must be the canonical compact record encoding with its one final
  newline, so duplicate keys and alternate outer encodings are rejected.
- Corrupt, tampered, secret-bearing, stale, or legacy content at the expected
  exact path is a clear safe dev-replay failure, not a miss eligible for
  replacement.
- Credential checks for provider evidence and persistent localization records
  are mandatory and cannot be disabled by the ordinary unsafe `--no-secrets`
  override.
- No automatic deletion, quarantine, migration, eviction, or broad cleanup is
  added. A total shared-cache size/eviction policy is required before product
  wiring.
- Publication requires a local filesystem that supports no-replace hard links
  and directory sync. Unsupported filesystems fail explicitly; there is no
  overwrite fallback.

## Cacheability

Only a complete accepted Russian projection is recordable.

- provider/file/read errors: not recorded;
- malformed, oversized, unsafe, or structurally invalid output: not recorded;
- canonical/locale envelope mismatch and complete English fallback: not
  recorded;
- any field-level fallback or diagnostic: not recorded;
- cancellation before or after the saved-response seam: not recorded.

An exact valid hit performs zero saved-response reads and zero provider calls.

## Explicitly not authorized

- a live provider, network call, retry, or new HTTP client;
- ordinary `repomap` or `--lang` integration;
- report JSON/HTML, manifest, HTTP, browser, or UI changes;
- a shared response/semantic/projection cache namespace;
- reuse or migration of `.model-research` or `.component-synthesis`;
- persistence of raw HTTP envelopes, headers, API keys, or Authorization;
- localization beyond the Architecture prose approved by Decision 162;
- Russian-to-English translation;
- treating this record as production cache authority.

## Acceptance

- identical prompt/config/request bytes produce an identical key and record;
- changing canonical input, prompt, endpoint, model, generation configuration,
  or one final request byte produces a different key and clean miss;
- an exact hit skips the saved response and replays byte-identically through
  Decision 164;
- rejected/fallback output creates no final record or usable temporary file;
- corrupt, tampered, oversized, unknown/trailing, secret-bearing, symlinked,
  and legacy entries fail safely without leaking their contents;
- concurrent same-key publication leaves exactly one immutable valid winner;
- the run changes only below the new fixed record directory;
- focused tests, race tests, full checks, and the provider-free etcd check pass.

## Migration and rollback

The record is versioned, dev-only, content-addressed, and has no ordinary
consumer. Rollback removes the request builder, record reader/writer, explicit
CLI/Make entrypoint, tests, and this decision. Existing unreferenced record
files remain inert and can be ignored; no automatic destructive migration is
performed.
