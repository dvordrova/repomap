# Decision: Chatto D134 Fresh Acceptance Run

Status: Completed verification. Approved and reviewed by the repository owner
and product supervisor in the current session.

## Baseline and purpose

The verification baseline is commit
`06495a127a4027527307cd57592e202db6ad3621`. The stable PATH binary must be:

- path: `/Users/dvordrova/git/repomap/.bin/repomap`
- SHA-256:
  `8332df3a26b2704313a2482b8e42082d8ae1f8580038e62acaeab5b2df727753`

Decision 134 changed only the failure unit at the bounded Study candidate
boundary. Provider-free tests prove that the retained eleven-candidate Chatto
shape accepts nine valid siblings and rejects the two invalid candidates
without losing order or weakening anchor rules. One normal Chatto invocation
is now required to verify actual orchestration and product publication.

## Authorized invocation

Record the Chatto revision and dirty state, then run exactly once from
`/Users/dvordrova/git/chatto`:

```sh
repomap "$PWD"
```

The command must resolve to the stable binary above. Existing provider cache
reuse is allowed. No retry, timeout inflation, alternate repomap checkout, or
second repository is authorized by this decision.

## Evidence and acceptance

Preserve the full log, debug run path, stage latency, diagnostics, warnings,
fallbacks, report publication, and authority-bound serving outcome beneath
ignored `tmp/chatto-d135/` and the ordinary run cache.

If the retained Decision 133 response is reused, the candidate boundary must
record eleven received, nine accepted, and two rejected candidates at
zero-based positions 8 and 10. If a fresh provider response differs, judge its
own bounded diagnostics rather than forcing those counts.

The product check must establish:

1. the Study stage does not lose valid siblings because another candidate is
   invalid;
2. accepted Study directions are routable alongside the three existing
   incomplete topics and Architecture;
3. Search remains absent;
4. one useful direction or topic opens within one click and reaches an
   authorized exact source within the second click; and
5. any expensive continuation after a decisive failure is recorded
   explicitly.

## Mutation boundary and stop conditions

This decision may modify only this decision document and
`docs/agent-room/CURRENT.md`. It authorizes no production or test source
change. Stop after terminal report publication or failure, browser evidence,
and supervisor review. Do not retry or implement a discovered corrective
under this decision.

## Completion condition

This decision completes only after exact artifact/log inspection, browser
evidence, and concrete product-supervisor review. It authorizes no subsequent
repository run or implementation slice.

## Completed verification result

The stable binary and clean Chatto revision matched. The single invocation
saved run `20260729-034513-chatto-1adc0451c0c3`; no retry occurred. Decision
134 behaved correctly: the candidate boundary independently examined twelve
candidates and recorded zero accepted plus twelve rejected. Positions 0 and 5
had three or four selected anchors but only one reading anchor. The other ten
had only two selected anchors. Diagnostics therefore contained two
`invalid_reading_anchor_count` and ten `invalid_anchor_selection` issues.

The result is not an atomic-loss regression. The retained response contains
twelve useful developer questions, but every item violates the unchanged
complete-pack contract. The prompt's JSON example visibly contains one anchor
placeholder and one reading-anchor object even though its prose requires three
to five anchors and one reading entry for each.

The semantic opportunity scan spent 113,232 milliseconds and published no
mechanism. Study took 129,756 milliseconds, including a 34,110-millisecond
accepted Brief and 94,613-millisecond rejected candidate call. Reconciliation
took 41,534 milliseconds. The fallback report remained coherent: three
distinct incomplete topics, no Search, and Architecture available. A topic
opened in one click and exact `runtimeUnitRegistrations` dispatched through
the authorized source-open path in the second click.

The product supervisor reviewed the full log, exact attempt JSON, and
screenshots. It confirmed that the missing product boundary is a visibly
incomplete Study projection for useful grounded weak signals while retaining
the three-to-five-anchor rule for complete Study packs.
