# Handoff: what is known, what is open

> Historical document, not current instructions or acceptance. Original wording
> and historical line numbers are preserved. Use [CURRENT](CURRENT.md) for the
> active decision and [AGENTS](../../AGENTS.md) for work-specific contracts.

Rewritten 2026-09-03 after a night of work. Everything here is measured, not
remembered. Authority for the product is
[docs/CONSTITUTION.md](../CONSTITUTION.md); the architecture is
[CURRENT.md](CURRENT.md); the flags and acceptance ritual are in the README.

## Baseline, measured 2026-09-03

| what | fixture | chi |
|---|---|---|
| wall clock, all targets, warm cache | 5.6 s | 12.4 s |
| wall clock, all targets, cold | 55 s | 111 s |
| targets analyzed | 2/2 | 4/4 |
| report.html | 145 KB | 210 KB |
| of which script | 5.4 KB | 5.4 KB |

The fixture was 14.6 s warm at the start of that night. What closed the gap,
in order of size: the run id was inside the facts digest, so the orientation
stage never hit its cache and paid for a live call on every run; the two
semantic batch planners re-encoded the whole request once per subject; the
always-on credential scans ran eight regexps and a full JSON decode over every
payload. The credential scanning was then removed entirely on the owner's
instruction — see the constitution for what that gives up.

Two consecutive runs of the same repository now produce a byte-identical
`report.html`. If that stops being true, something run-varying has got into a
stage digest again; that is what to look for first.

## There is no request-sizing defect

A five-role council was convened on the premise that requests are bounded by
transport bytes rather than the model's context window. The premise is false
for the code in the tree, and the veto checked it rather than argued it.

All 1,014 cache records were bucketed by response schema and date. Every call
that ever exceeded 131,072 input tokens is dated Aug 25-29 and belongs to
`activity_refs`, `blocks`, `uses`, or bare `roles`. The first three have zero
JSON tags anywhere in `internal/` or `cmd/` — deleted packages. Bare `roles` is
`internal/experiments/clientrecipe`, referenced only by its own tests.

Do not reopen this without new measurements. Specifically do not add
`MaxInputTokens`, do not raise `ownedSubjectsPerRequest` from 32, do not cap
`programgrouping` or `groupmatching`, and do not add a run-level token budget.
`cubeState` hashes exact request bytes, so any prompt or request-shape edit
cold-starts all 1,014 cache entries in real money.

`programgrouping/run.go:93` gates `runMergeTournament` on `len(finalPlan) > 1`,
and grouping is always one batch today, so that tournament has never run on a
real repository. `merge.go` fails the whole run at three sites. Capping
grouping routes every repository through that never-exercised path. Print the
number first; cap it the day a real printout crosses the window, and exercise
merge on the fixture deliberately before that.

## Settled: the grouping answer had no size rule

For one unchanged repository the model returned between four and thirteen
groups across cold draws, and the four-group answers put two thirds of the
target in one box. The prompt described lanes, membership and evidence in
detail and said nothing about how big a group should be, so "everything" was a
valid answer. It now sizes a group for reading: one responsibility a reader
would name out loud, eight to fifteen for a target of a few hundred subjects,
and no group over about a fifth of what it was given.

| draw | before | after |
|---|---|---|
| fixture backend | 10 groups / 4%, 7 / 5%, 4 / **66%** | 8 / 4%, 10 / 4% |
| fixture front | 12 / 17%, 4 / **73%** | 13 / 4%, 9 / 4% |

Read that column as "groups returned / share of the target held by the largest
one". Variance is the thing to watch here: measure several cold draws, never
one.

## Settled finding 1: core is diluted, and narrowing it costs more than it saves

`core` is a gate, not a ranking. It decides only which subjects the grouping
model is allowed to place in the core lane; grouping then does the actual
routing. On the fixture the gate passes 209 of 219 backend objects and 45 of
them reach a group. Narrowing the gate cannot reorder anything downstream —
it can only take candidates away from the stage that does the routing.

Every real run on the fixture, categorization against what grouping made of it:

| run | obj cov | core | groups | largest group | grouped subjects | connections |
|---|---|---|---|---|---|---|
| backend, before the regression | 55% | 152 | 7 | 8 | 36 | 7 |
| backend, regressed | 16% | 35 | 6 | 9 | 22 | 5 |
| backend, after revert | 98% | 225 | 10 | 13 | 55 | 14 |
| front, before the regression | 46% | 89 | 8 | 28 | 85 | 10 |
| front, regressed | 34% | 94 | 9 | 17 | 54 | 9 |
| front, after revert | 75% | 281 | 7 | **127** | 237 | 12 |

Backend moves the right way at every step: more core, more groups, more
grouped subjects, more connections. The regressed run is what a narrower core
looks like — 6 groups over 22 subjects. **Do not narrow `core`.**

The defect the 97% was standing in for is real but lives one stage down and on
one target: front's `Simulation rendering and animation` holds 127 of 424
subjects, 29% of the target, and renders as three rows and `+94 more`. That is
a group-size problem in grouping, not a category problem, and the console now
prints it on every run:

    groups: 7 (core 4, dependencies 1, triggers 2)
    subjects in a group: 237/424 (55%)
    largest group: Simulation rendering and animation, 127/424 (29%) of this target

The earlier experiment stays the cautionary tale for this area. Inviting the
model to categorize context subjects cut discards and looked like a win: chi
went from 264 categorized subjects to 858. But 841 of those were `core`,
covering 90% of the target, and group counts fell as the signal diluted. It was
caught by a human diffing two reports by hand. **Row count is not quality.
Always report the denominator and the per-category split.**

## Settled finding 2: they are connections, not invention

172 of front's 338 assignments name a `subject_id` that is not an object.
All 172 are relation patterns of the same sealed index: 103 `invokes_external`
and 69 `calls`, each with its own `file:line:col` anchor. Backend's 25 of 239
are the same thing. Zero assignments, on either target, name an id belonging
to neither set.

The categorizer is asked about two kinds of subject, objects and the relation
patterns between them, and the earlier count only had a denominator for the
first. `Result.Validate` resolves every `subject_id` through `subjectByID`,
which searches objects and then relation patterns and refuses the whole
result otherwise, so an invented id cannot be accepted — that refusal is now
pinned by `TestValidateRefusesASubjectThatIsNeitherObjectNorConnection`. The
console prints the two denominators separately on every run.

Nothing to fix here. The finding was an artifact of the measurement.

## The instrument, shipped

Every categorization now prints, on the console, the fraction of subjects
covered split into objects and connections, the per-category counts with their
share, the executed request plan with how many requests assigned nothing, and
the number of accepted rows naming a subject outside the sealed index. A run
that loses 60% of its signal can no longer read as success.

The snippet below is still the fastest way to re-check a finished run
directory without rerunning the binary. Note that it counts objects only, so
its `ids - objs` is the connection count, not an error count:

```python
import json, collections
pi = json.load(open(f'{run}/program-index.json'))
objs = {o['id'] for o in pi['objects']}
a = pi['categorization']['assignments']
ids = {r['subject_id'] for r in a}
cc = collections.Counter(c for r in a for c in r['categories'])
print(len(objs), len(a), len(ids & objs), len(ids - objs), dict(cc))
```

Note the field names: assignments live at `categorization.assignments`, each
row is `{subject_id, categories}`. Categorization exchanges are the ones whose
`request.json` contains `categorize_refs`; the request body is an encoded
string, so grep it, do not walk it as JSON.

## Scale: what a repository costs

One chi run, the medium case the brief names, counted from its own artifacts:

| what | value |
|---|---|
| wall clock, cold | 118 s |
| wall clock, warm | 7.6 s |
| provider calls | 55 — 42 categorization, 5 matching, 4 grouping, 4 once each |
| input tokens | 1,635,718 |
| output tokens | 24,585 |
| provider latency, summed | 196 s |
| targets analyzed | 4/4 |

| repository | objects | categorization requests | result |
|---|---|---|---|
| fixture, both targets | 441 | 24 | 5.9 s warm, 55 s cold, 2/2 |
| chi, four targets | 1,516 | ~60 | 7.6 s warm, 118 s cold, 4/4 |
| python-dotenv, three targets | 900 each | 53 each | 21 s warm, 3/3 |
| repomap itself, twenty targets | 4,162 largest | 506 largest | 2,063 s cold, 19/20 |
| type-fest | 3,405 | 283 | not run; the plan was announced and refused |
| beets | 26,218 | ~820 | stopped after 388 calls |

repomap's unanalyzed targets are TypeScript fixtures under `testdata` with no
`node_modules`; the run says so and continues. There were two, and the
difference between them matters. `testdata/acceptance/python-tutorial-game/front`
is an ordinary project: install its dependencies and it reads, which is how
the count is 19/20 and not 18/20. `testdata/repositories/jsts` declares
`@fixture/kafka-client@1.0.0`, which does not exist on npm, so no install can
ever succeed there and 19/20 is the ceiling for this repository. Installing
dependencies used to break `make test`, because the fixture copier refused the
symlinks an install leaves behind; it skips `node_modules` now, so the tests
pass either way.

Getting from "run failed, nothing analyzed" to 19/20 took three fixes, each
with the provider's own words behind it: a module `go list` cannot describe is skipped rather than
fatal; the completion reservation was six times the largest answer ever
produced and was eating the context window; and a grouping request is bounded
at three megabytes because a 15.4 MB one was refused outright.

beets was pointed at on 2026-09-03 as a third repository to read. It is not
medium by this tool's standards: nine minutes and 388 accepted calls in, at
12.05 M input tokens, it was roughly halfway through categorization alone. The
run was stopped rather than finished, so nothing is known about what its
report would have looked like.

**The model's context window is 1,048,576 tokens, not 131,072.** The provider
says so when a request misses it, and the completion reservation is subtracted
from it before the request is read. Both numbers in the old baseline table
were wrong about this, which is why "no call ever crossed the window" held
right up until a real repository crossed it twice in one run.

The request plan is now announced before it executes, so the next person sees
the number rather than the bill. Nothing caps it. If a bound is ever wanted,
the number to bound is categorization requests, which is subjects ÷ 32.

python-dotenv is the shape to keep in mind: a quarter of chi's size but eight
times its cold wall clock, because it is three targets and each one carries
the whole test tree, so the same test files are categorized three times and
its groups are named after tests as often as after the library. Whether a
target should include its own tests is a product question nobody has answered.

## The merge phase asks for a copy, and copies lose things

Grouping splits a large target into request shards, and a merge phase then
consolidates what the shards proposed. A merge response must re-emit every
membership of every candidate it was handed, so the task is a copy of
thousands of refs, and a response that drops one is refused whole. chi's
router package sent 33 candidates in a 1.35 MB merge request and lost the
merge to exactly that.

Three things were done and one was not.

Done: a failed merge keeps the shards' groups instead of losing the target; a
rejection is local to the candidates it was given, so the consolidations the
model got right survive; and candidates per merge request are bounded at
twelve, which took chi from one 1.35 MB request to three of about 700 KB, two
of which merged cleanly.

Not done, and this is the real fix: the response should name which candidates
belong together and let the code union their members. Nothing can be dropped
from a union. That changes the merge request and response shape and the prompt,
so it cold-starts grouping everywhere, and it is the next thing worth doing in
this area.

Two experiments were tried against this and reverted, with their numbers:

- telling the model that tests are one responsibility took python-dotenv's
  test-heavy targets from 16 and 15 groups to 5 and 7, each with a single test
  group — and destabilised chi's router package, whose longer answer stopped
  fitting in one attempt. Whether tests belong on the map at all is a product
  question nobody has answered.
- capping grouping at 256 subjects per request, added when a 1.64 MB request
  went unanswered twice, split that same package into four shards and 34
  unconsolidated groups. It made the one target it existed for worse.

## What a day of running this actually costs

Measured on 2026-09-03 from the provider's own per-call metrics, 3,574 live
requests in one day:

| | |
|---|---|
| input tokens | 133,588,160 |
| output tokens | 680,019 |
| requests answered `{"assignments":[]}` | 2,361 of 3,574 (66%) |
| input tokens spent on those | 76,744,675 (57%) |
| repomap analyzing its own checkout | 2,243 calls, 98 M tokens, **84% of the day** |
| chi, python-dotenv and the fixture together | 413 calls, 13 M tokens, 10% |

**Never run repomap on its own checkout as a routine check.** It is the worst
repository this tool owns: one target of 4,162 objects and 11,889 relation
patterns, 502 categorization requests, and every commit here invalidates all
of them. Four self-runs in one day cost roughly $38 of a $45 bill to answer a
question a log line already answered. chi and python-dotenv cost about a
dollar cold and nothing warm; they are the loop.

The empty answers are not evenly spread. On chi and python-dotenv, relation
patterns are productive — chi categorized 107 of 107, python-dotenv 445 of
795. On repomap's own big target, 11,889 patterns produced 613 assignments,
of which 19 were not `core`, and those 19 read as noise: `Fprintf` as
inbound, `Lstat` as background activity, `Parent` as a dependency. Yield per
request collapses as a target grows, and type-fest is the limit case: 5,636
patterns, 288 requests, zero pattern assignments.

## Grouping variance, measured and narrowed

The grouping stage is a coin flip until cached, and on 2026-09-03 it was
measured. chi's router package, 892 categorized subjects, eight cold draws of
one 892-subject request returned 1, 3, 4, 17, 26, 30, 30 and 33 groups, the
largest holding up to 75% of the target. Four causes, all in what the request
was, none in the model:

- the request asked about all 892 subjects at once; a grouping shard is now
  capped at 64 owned refs, applied before the byte search not after;
- subjects were ordered by id, a content hash, so a shard was 64 unrelated
  symbols and — because ids are prefixed `program-object-`/`program-pattern-`
  and `o` sorts before `p` — a function and its call sites could never be
  selectable together; subjects are ordered by file, line, column now;
- the size rule was written for "a few hundred subjects" and read by a request
  carrying 64, so the request now states its own selectable count and the rule
  is written against it;
- a group holding a fifth of the target was named into a container before it
  could be split, so it never was; groups are now settled — joined, then split
  — before the parts are named.

Three cold draws on the fixed code returned 28, 31 and 29 groups, largest
holding 7-11%. Spread 1.1x instead of 33x, and the degenerate one-group
outcome is gone. Take three draws into separate `--debug-dir` caches to
measure this; one draw proves nothing.

Still open: split's first attempt is not always a partition
(`split_not_a_partition`), and there is no semantic retry, so that group stays
whole for the run. A repeat of the partition instruction at the end of the
request is the cheap thing to try.

## What actually moves grouping variance, measured on the live endpoint

Probed the largest consolidate request (108 candidates, dotenv core) directly
against the DeepSeek endpoint on 2026-09-03, thinking disabled to match the
pipeline. A first probe left thinking on by mistake and reported 131 s and
17,779 completion tokens; disabled, the same call is 8.1 s and 1,769 tokens.
Any measurement of this stage MUST send `"thinking":{"type":"disabled"}` or it
describes a mode production never runs.

- **Temperature: keep 0.1.** Three draws at 0.1 held 15 of 31 distinct merges
  stable across >=2 runs; at 1.0, 4 of 34. Raising temperature to sample and
  vote makes it worse here, not better.
- **Response bloat is real but small in tokens.** The answer is 6,163 bytes vs
  3,521 for a bare ref->cluster form — about half — but only ~1,769 tokens
  either way. The reason to shrink the response is the model's freedom to
  reword titles and dither on borderline merges, not speed.
- **The size of the question is what is left.** At 108 candidates even 0.1
  keeps only half its merges and the group count swings 19-35; the stable
  ones are the large obvious merges, the churn is two-or-three-candidate
  borderline ones. Cut the consolidate question into windows over several
  passes so borderline merges get another chance to meet.

## The grouping cubes, and what they are worth

Every model call in this stage is now a cube in the owner's sense: a simple
question, one decision, a flat answer the code validates and assembles.
Grouping answers `{"ref","group"}` per selectable ref plus `{from,to,label}`
links; consolidation answers `{"ref","cluster"}`; naming answers
`{"ref","title"}` per part. Lanes, members, evidence and assembly are the
code's, derived from the categories a subject already carries.

Three cold chi draws at each step, partition agreement between draws as the
measure (share of symbol pairs that are together-or-apart the same way):

| | groups | coverage | largest | agreement |
|---|---|---|---|---|
| 892-subject question | 1-33 | | up to 75% | |
| shard 64, code order, split before parts | 28/31/29 | 52-63% | 7-11% | 96/98/98% |
| grouping cube alone | 47/123/8 | 90-94% | up to 60% | 96/61/58% |
| + consolidation in windows of 40 | 39/66/82 | 93-95% | 10-18% | 92/97/94% |
| + naming cube | 78/68/75 | 90-94% | 10-13% | **97/99/98%** |

Coverage nearly doubled and agreement is at its best, so the page now
describes almost the whole target instead of half of it. Group count is the
loose end: 68-78 where it used to be about 30, because a partition must place
everything. Parts are what make that readable and they are not settled — one
draw produced ten parts, another one.

Three traps found the hard way, all worth keeping:

- **A cube with its own short prompt must still contain the word "json".**
  DeepSeek refuses `response_format: json_object` otherwise, and the refusal
  arrives as a provider error that looked exactly like a bad answer. Two fixes
  were aimed at the wrong cause before anyone read the response body.
- **Give a small cube the whole multi-phase prompt and it answers in the shape
  of its neighbours.** The naming cube replied `{"assign":[{"ref","part"}]}`
  until it got an instruction about nothing but naming.
- **Validation must not punish a missing optional field.** A shard whose
  groups say nothing to each other returns no `links`, and refusing that
  failed the whole target.

## Known gaps, recorded not fixed

- `os.environ["KEY"]` and `process.env.KEY` subscript reads are not captured.
  Catching them needs a `reads` relation from the adapter, a schema change.
- **The index records no value for a numeric or boolean literal argument.**
  `PatternArgument.Kind` has `literal_string`, `string_template` and
  `dynamic`, and everything else arrives as `dynamic` with no value. Three
  answers are missing because of it: a JavaScript `app.listen(3000)` port, a
  Python `Field(default=8080, env='APP_PORT')` default — the page shows
  `APP_HOST 0.0.0.0` beside a bare `APP_PORT` for exactly this reason — and
  any numeric manifest-like literal. One adapter change would close all three,
  and it is the same shape of change the `os.environ["KEY"]` gap needs.
- `~/git/fuego` fails before analysis: `go list` authority is incomplete for a
  package of templates that does not build. Not investigated.
- `targetportfolio.Compile` and `CompileWithExecutableAuthority` are now
  production-dead, reachable only from their own tests. Removing them rewrites
  about ten call sites.
- The `resolveGoTarget` field on `defaultRunDeps` is injected by nothing, in
  production or in tests.

## Traps

- **The provider account can run out.** On 2026-09-03 it started answering
  402 Payment Required. Runs whose every call is cached still work and still
  produce a report; anything that needs one new call fails at that stage and
  publishes nothing. There is no way to tell the two apart before running.
- **A fixture inside this repository re-buys its orientation call on every
  commit.** Commit subjects are quoted claims, so the orientation request for
  `testdata/acceptance/python-tutorial-game` changes whenever anything is
  committed here, even when nothing in the fixture changed. Re-rendering that
  report is therefore never free, unlike chi or python-dotenv. This is correct
  behaviour, not a cache defect — it is only expensive.
- **The cache key is the provider state plus the request.** Transport settings
  were in that state, so editing `defaultTimeout` re-bought all 1,047 cached
  answers; they have since been taken out and only the endpoint, model, auth
  mode, temperature and token cap remain. Anything added there is paid for in
  real money on the next run of every repository.
- **Every model stage is a coin flip until it is cached.** Categorization
  coverage, group counts and the orientation text all differ between live
  calls at temperature 0.1. A single cold run proves nothing about quality;
  take several draws before believing a prompt change helped or hurt.
- A name-based dead-code scan misses interface satisfaction.
  `runOutputWarningSink.Write` looked unreachable and has four call sites.
- **Do not reorder request fields for provider prefix caching, and this time
  there are numbers.** The prefix cache is already the biggest discount the
  tool gets: on a cold chi run the provider reported 64-72% of categorization
  input tokens as prefix hits. Moving `documentation` to the front of the
  request — on the reasoning that it is the one part identical in every
  request for a target — dropped that to 6-7%. Reverted; chi came back to
  184/747/15/100 subjects covered at 8.5 s warm without spending anything,
  because the old cache records were still there.
- **A bigger categorization shard buys fewer answers, not cheaper ones.** At
  128 owned subjects per request instead of 32, chi's request count fell from
  42 to 12 and its coverage collapsed: the core library went from 747 of 950
  subjects to 182, with 0 of 433 objects named, and 6 of its 8 requests came
  back empty. Measured and refused.
- The 429 collapse is not worth touching: 1,013 of 1,014 calls succeeded on the
  first attempt.
- Failing a whole run is never an acceptable answer to a model disagreement.
  Log it to `rejected.jsonl` and continue.
