# Decision: Golden Mechanism v0.3 claim-and-coverage validation

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can the saved Caddy directory-listing response be accepted and published when
all of its actual claims are supported and all key answer aspects are covered,
without requiring it to cite every available fact that it does not use?

## Fixed inputs

- Candidate: `File Server Directory Listing`.
- Candidate ID: `semantic-candidate-8d47b99879d5cfbc1413052a`.
- Question: `How does the file server generate and sort directory listings?`.
- Caddy revision: `873fac5fc094fe538d0c477509127bb321d51a32`.
- The saved seven-fact v0.2 projection remains byte-identical.
- The saved v0.2 response is the only synthesis fixture; no new model call is
  permitted.

## Approved implementation

1. Keep available facts, required answer aspects, and claim-required support as
   separate concepts. An available fact is not mandatory content.
2. Remove the blanket `local_sequence_claim_missing` rejection. A claim that
   asserts temporal or ordering semantics must still cite sequence-capable
   support and any cited local sequence must remain conditional and
   same-function/same-branch scoped.
3. Reuse the existing intent contract and local aspect markers. Derive
   claim/aspect coverage locally; do not create a parallel Golden schema.
4. Require every key answer aspect to be covered by direct or compositional
   claims, keep remaining unknowns explicit, and reject every unsupported claim.
5. Treat array order as editorial display order, not evidence of runtime
   sequence.
6. Save used fact IDs, unused available fact IDs, and the existing covered and
   uncovered aspect partition on the canonical artifact.
7. Revalidate the exact saved v0.2 response through all existing validators and
   the new claim-and-coverage contract, with no provider or probe call.
8. On acceptance only, publish through the existing canonical supplement,
   semantic record, report, evidence, and Super Search paths.
9. Prove replay without provider, repository analyzers, targeted probe, or raw
   v2/v3 response input; verify the five fixed semantic searches and exact
   source-path precedence without changing search.
10. Save an ignored supervisor report with the full claim, fact-use, aspect,
    replay, HTML, and search result.

## Required regressions

- unused sequence fact plus no temporal claim is accepted;
- unused sequence fact plus unsupported temporal language is rejected;
- a sequence claim with sequence-capable support is accepted;
- a sequence claim that widens the local fact into a runtime guarantee is
  rejected;
- used and unused fact IDs partition the candidate's available facts;
- all key aspects require direct or compositional coverage;
- replay derives the same usage and coverage metadata without raw model output.

## Hard exclusions

- no model call, provider/model change, retry, response rewriting, or prompt
  rewrite;
- no targeted-probe rerun, opportunity scan, package loading, SSA, call graph,
  runtime-surface discovery, or other repository-wide analyzer;
- no changes to the seven saved facts;
- no opaque-ID, repository-reference, temporal-language, candidate-lineage,
  claim-basis, or intent-retention weakening;
- no UI, renderer, search-path, or search-algorithm change;
- no full structured relation schema unless the saved response actually needs
  sequence semantics.

## Acceptance

The fixed response may publish only if every actual claim passes the existing
support validators, every key answer aspect is covered directly or
compositionally, remaining unknowns are explicit, temporal semantics are either
absent or sequence-supported without scope widening, and replay/report/search
preserve the exact canonical artifact without model output.
