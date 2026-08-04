# Decision 192: Default semantic exchange journal

## Status

Approved by the repository owner through the active supervisory instruction as
the reliability checkpoint after Decision 191.

## Problem

Ordinary debug runs do not retain one consistent, inspectable record of each
bounded semantic model question and answer. Orientation writes its request and
response only under `--dump-llm` or on selected failures, Targeted Research
writes a separate per-round pair, optional stages retain different subsets in
their product or replay artifacts, and some successful calls retain no pair at
all. The result is incomplete human debugging evidence and several independent
one-off dump paths.

## Decision

Add one debug-only semantic exchange journal below the existing confined
`debugdump.Writer`. Every ordinary `repomap <repo>` semantic request has one
recording owner at its current stage or cache boundary. The recorder writes a
bounded request payload, a bounded response payload or truthful closed marker,
and then an `exchange.v1.json` commit marker. An exchange without that final
metadata file is incomplete and not a committed journal entry.

The journal records only closed metadata: stage, stable stage-local and
semantic-attempt ordinals, state, validation code, request provenance,
semantic-call count, transport-attempt count, and original and saved byte
identities. Request provenance distinguishes a prepared request from bytes the
existing provider seam proves were sent exactly. Metadata and paths contain no
repository path, provider endpoint, authorization, API key, error text, model
prose, or repository-derived identifier.

Payload preparation always applies persisted-artifact redaction and mandatory
`secretscan.DetectAlways`. A safe payload is retained as its exact
post-redaction bytes. An unsafe payload is replaced completely by a bounded
marker containing only its original SHA-256, byte count, and a closed unsafe
kind. An unavailable raw response is represented by a truthful closed marker;
the journal never fabricates provider wire bytes from a projection or another
canonical artifact. Authorization and API keys are never persisted.

Journal recording is best-effort and cannot alter provider execution, cache
selection, validation, canonical output, replay, or publication. A journal
write failure emits at most one bounded warning per run and closed stage. The
warning contains only the stage and a closed code. `--no-debug` disables the
journal because there is no run-artifact writer.

Implementation is split into three reviewable checkpoints under this decision:

1. the shared recorder plus Orientation and Targeted Research;
2. Architecture, monolithic Guided Tour, and production Study v3.2; and
3. Localization, removal of `--dump-llm`/`DumpLLM`, replaced one-off dump
   paths, and documentation cleanup.

Existing provider, retry, accounting, cache, replay, canonical report,
manifest, Study attempt, Architecture record, Localization projection, and
Task Lens contracts remain unchanged. Current caches expose raw response bytes
only where they already retain and validate them. Otherwise the journal writes
`raw_unavailable`; this decision adds no cache or record upgrade, legacy reader,
or migration. Journal artifacts are never manifest-bound and are never inputs
to report generation, cache, replay, or canonical output.

## Proof

Provider-free focused tests at each checkpoint establish:

- root confinement, traversal and symlink rejection, bounded payloads,
  metadata-last publication, deterministic collision-free ordinary writes,
  and safe concurrent stage writes;
- exact safe post-redaction bytes, complete unsafe replacement with original
  identity and closed kind, and absence of credential material;
- one deduplicated bounded warning per failed run/stage write without changing
  the semantic result;
- accepted, rejected, failed, canceled, and validated cache-hit observations at
  each covered ordinary stage seam, with one recording owner per request;
- cache hits have zero semantic and transport attempts while live exchanges
  retain existing transport telemetry;
- `--no-debug` writes no journal, and journal files are ignored by cache,
  replay, report, manifest, and canonical readers; and
- after the final checkpoint, ordinary debug runs need no dump flag and the
  removed `--dump-llm` flag is rejected.

All verification uses fixed local providers or HTTP fixtures. It makes no live
model request.
