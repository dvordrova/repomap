# Decision: Chatto Incomplete Study Microexperiment

Status: Complete. Approved by the repository owner and product supervisor in
the current session.

## Attributable input and question

Decision 135 retained twelve useful Study questions, each with at least one
catalog-resolved exact reading anchor, but none satisfied the complete
three-to-five-anchor Reading Pack contract. The current product discards all
twelve and publishes only three separately discovered incomplete topics.

The microexperiment asks one product question: is the already-paid signal
useful when presented honestly as incomplete Study directions without claiming
a complete pack, reviewed path, behavior, ordering, or answer?

## Provider-free projection

Replay only:

- `study_direction_candidates_attempt.json`;
- `repository_study_map_bundle.json`; and
- the three observed Decision 135 fallback topics.

Preserve provider order. Project a candidate only when its natural question is
nonempty and at least one `reading_anchors` entry resolves to an exact anchor
in the saved catalog. Show its question, why it matters, learning outcome,
learning stage, incomplete status, and exact starting path/symbol/line. Do not
repair, synthesize, or infer missing anchors.

The existing three topics remain the Overview shelf. A separate Study surface
shows the retained incomplete directions. No Search, code excerpt, answer,
runtime order, or complete-mechanism claim is allowed.

## Mutation boundary

The experiment is limited to ignored files beneath:

`tmp/chatto-incomplete-study-microexperiment/`

and decision bookkeeping in this document plus
`docs/agent-room/CURRENT.md`. No production, report format, prompt, provider,
decoder, persistence, HTTP, source authority, or renderer file may change.

## Acceptance

Provider-free checks must prove:

1. all twelve retained candidates are considered in provider order;
2. every published direction has at least one exact catalog-resolved reading
   anchor;
3. no missing reading entry or unresolved ID is invented;
4. the three existing topics remain visible on Overview;
5. Study visibly distinguishes incomplete directions from complete packs;
6. one click reveals why/outcome plus exact starts; and
7. no Search or code excerpt is introduced.

Capture Overview, Study, and one direction-detail screenshot. Ask the product
supervisor for a concrete usefulness verdict before any integration decision.

## Completion condition

This decision completes after local projection validation, browser inspection,
screenshots, and product-supervisor review. It authorizes no production
integration or provider/repository run.

## Result

The provider-free replay projected all twelve retained candidates in provider
order. Every published direction has one catalog-resolved exact reading start;
none has an unresolved or invented start, and no complete Reading Pack is
claimed. The three Decision 135 topics remain the Overview front door. Study is
one click away, and one further click opens an incomplete direction with its
question, relevance, learning outcome, exact path, symbol, line, and reading
instruction.

The product supervisor inspected the final Overview, Study, and detail
screenshots plus `validation.json` and returned:

`VERDICT: INTEGRATE INCOMPLETE STUDY`

The approved next action is recorded in Decision 137.
