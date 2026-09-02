# Why model rows get discarded

Date: 2026-09-02
Evidence: the raw `semantic_exchanges` of three chi runs and two
python-tutorial-game runs, reconstructed against the documented discard rules.

## What the numbers were

Across ~1,550 provider exchanges only 3 were rejected outright (2 grouping,
1 matching). The interesting loss is invisible: rows dropped inside responses
the run accepted.

For chi, over three runs:

| rows the model returned | 2,965 |
| assignments kept | 2,126 |
| dropped: ref outside `categorize_refs` | 850 |
| dropped: `dependency` claimed on a platform symbol | 163 |

The console had reported 852 discarded rows over those runs, which matches the
850 figure. Almost every discarded row had one cause.

## The 850 were correct answers to a question we did not ask

Each categorization request advertises about 32 owned refs plus a much larger
context catalog, then asks about the 32. The model categorizes what it can
see. The dropped rows named real subjects: `dbGetArticleBySlug`, `GetArticle`,
`DeleteArticle`, `UpdateArticle`, `paginate$1`.

Every one of those subjects is owned by another request in the same exhaustive
cover, so the run paid for the answer, discarded it, and paid again later.

Fix applied: a row whose ref is outside the asked batch but is a real subject
of the same target is now accepted. Every other check still applies, and
assignments already union by subject id, so a subject answered twice
converges. Rows naming something that is not a subject at all are still
discarded and now logged with the offending ref.

## The 163 are a real prompt weakness

`fmt.Println`, `net/http.ListenAndServe`, `flag.Parse` and
`net/http.ResponseWriter.Write` were labelled `dependency`. The request already
carries `external_authority_kind: platform` for each of them, so the
information is present and the model ignores it. These are category-pair
discards rather than row discards, which is why they sat outside the reported
852.

Not yet fixed. The durable fix is to make the answer unrepresentable rather
than to add prompt words: advertise the allowed categories per subject so a
platform symbol is never offered `dependency`.

## The one grouping failure was a bookkeeping slip

`merge response omitted validated candidate memberships from g2` came from a
263 KB request asking the model to re-list complete membership across 57
candidate groups. Losing a member there is not a judgement error.

The durable fix is to ask only which candidate groups merge together and let
Go union their members. Membership omission then becomes impossible by
construction, because the model never lists members.

## A change that had to be reverted

Alongside the acceptance fix, the prompt was briefly changed to invite the
model to categorize context subjects. Discards fell further, but the accepted
rows were worthless: chi's router went from 264 categorized subjects to 858,
of which 841 were `core`, covering 90% of every subject in the target. Group
counts fell as the signal diluted, from 10 to 6 for the rest example. A
category carried by nearly everything routes no attention, which is the whole
point of the product.

The invitation was reverted; the two code changes were kept. A volunteered row
is still accepted when it arrives, but the model is no longer asked for one.

The lesson for later measurement: a falling discard count is not by itself
evidence of improvement. Check what the accepted rows are worth. Run-to-run
variance is also large on this stage, so single-run comparisons of counts are
weak; only large shifts such as 30% to 90% coverage are conclusive.

## What is logged now

Every model stage appends counted rows with a bounded sample of offending refs
to `rejected.jsonl` in its run directory, and the console prints the breakdown
by reason instead of one number. Logging never fails a run.
