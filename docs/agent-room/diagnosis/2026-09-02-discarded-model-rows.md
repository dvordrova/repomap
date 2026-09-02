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

## Follow-up: the regex fallbacks were dead weight

The fact layer had two text-scanning passes beside its index-sourced ones, for
environment reads and for dangerous calls. Measured on the fixture and on chi:

| | fixture | chi |
|---|---|---|
| config reads found | 4 | 0 |
| of those from the sealed index | 4 | 0 |
| of those from the regexes | 0 | 0 |

Every row that landed came from adapter-emitted patterns and is `exact`; the
regex path emits `possible` and no row carried that resolution. Both passes
were removed. The fixture still yields the same 86 facts.

What the removal genuinely gives up, until the adapters emit it: an
environment read written as a subscript or member access, `os.environ["KEY"]`
and `process.env.KEY`, is not a call, so no pattern exists and nothing is
reported. The durable fix is the adapter emitting a `reads` relation, a kind
ProgramIndex already declares and no adapter uses. That needs a pattern form
beyond `call` and `decorator_call`, so it is a schema change rather than a
tweak.

## Follow-up: risk became dynamic execution

The `risk` fact kind imported a product repomap is not. The reader already
chose this repository, so a security finding is beside the point. What earns
its place is narrower and more useful: the places where control leaves code
the reader can follow, through `exec`, `eval`, a subprocess, or a deserializer
that constructs objects. On the fixture that single fact, `exec` inside
`make_step`, explains the whole design: why user code arrives as a string and
why a banned-word validator exists.

Renamed to `dynamic_execution`, and `dangerouslySetInnerHTML` was dropped with
the regexes because it is an injection concern rather than an orientation one.
The report section reads "Runs code it is given".
