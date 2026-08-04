# Decision: local authority for semantic verdicts

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can a locally validated partial semantic answer be published as `mixed` even
when the model proposed another final verdict, while unsupported claims remain
hard validation failures?

## Approved implementation

1. Add one pure local verdict reducer over already validated claim bases,
   resolved and unresolved status, retained contradictions and missing
   evidence, and required-answer-aspect coverage.
2. Treat the verdict returned by the model as a proposal only. Canonical
   semantic artifacts store the locally derived verdict. A disagreement is a
   bounded diagnostic and is not by itself a reason to reject unchanged
   claims.
3. Keep opaque-ID, claim-support, temporal/scope, intent, and aspect validation
   ahead of verdict derivation. Unknown IDs and unsupported semantics still
   reject the whole proposal before a verdict can be derived.
4. Derive `supported` only for meaningful resolved content with complete
   required coverage and no retained gap; `mixed` for meaningful resolved
   content plus an unresolved claim, retained missing evidence, or uncovered
   required aspect; `insufficient_evidence` for gaps without a meaningful
   supported claim; and `unsupported` when validated evidence contradicts the
   central claim.
5. Replay the unchanged saved chi request-dispatch response through the normal
   parser and local validators without a model call, probe, or repository
   analysis. If accepted, publish it through the existing Mechanism v1,
   Artifact, HTML, and Super Search path. If a sequence claim remains invalid,
   retain its exact claim-level rejection and stop.
6. Recheck the accepted Caddy Golden Mechanism without a model call. Its
   canonical verdict must remain `mixed`; its semantic hash must remain stable
   unless a deliberate artifact version change is required.
7. Keep the existing response and canonical wire shapes. The fixed model
   response retains its proposed `verdict`; the reduction/status diagnostic
   retains both values on mismatch; the existing canonical `verdict` field is
   populated locally. Because previously accepted Records and Mechanisms
   already satisfied the same verdict matrix, their replay interpretation and
   serialized payload do not change and no version bump is required.
8. Aspect coverage may credit each fact in a validated compositional claim
   using a bounded per-fact lexical threshold that still rejects unrelated
   support padding. It must not require the combined editorial claim to repeat
   half of every constituent fact merely to retain already grounded coverage.

## Fixed chi inputs

- `chi_request_dispatch_fixture.json`
- `chi_request_dispatch_projection.json`
- `chi_request_dispatch_supplement.json`
- `chi_request_dispatch_response_attempt.json`

Their facts, candidate, question, aspects, claims, prose, support IDs, missing
evidence, aliases, and evidence windows are immutable for this replay.

## Focused checks

- resolved claims plus complete required coverage derive `supported`;
- an unresolved claim, retained missing evidence, or an uncovered required
  aspect alongside resolved claims derives `mixed`;
- gaps without a meaningful supported claim derive `insufficient_evidence`;
- a validated contradiction of the central claim derives `unsupported`;
- model/derived disagreement emits `model_verdict_mismatch` with stable local
  reasons but does not reject an otherwise valid proposal;
- unknown opaque IDs and unsupported claim semantics reject before derivation;
- the saved chi response is replayed offline and either publishes through the
  existing path or stops with the exact claim-level sequence reason;
- the Caddy mechanism remains `mixed` and hash-stable.

## Hard exclusions

- editing or retrying the saved model response, changing prompts, adding facts
  or evidence, or making any model request;
- running a probe, repository-wide analysis, package loading, SSA, call graph,
  runtime-surface discovery, or the full repository pipeline;
- weakening claim support, temporal, scope, opaque-ID, intent, or evidence
  validation;
- a new semantic or rendering architecture.

## Stop condition

Stop rather than publish chi if its unchanged claims fail support,
temporal/scope, opaque-ID, or intent validation. Verdict derivation must never
mask such a failure.
