# Handoff: what is known, what is open

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
| repomap itself, twenty targets | 4,162 largest | 502 largest | 1,287 s cold, 18/20 |
| type-fest | 3,405 | 283 | not run; the plan was announced and refused |
| beets | 26,218 | ~820 | stopped after 388 calls |

repomap's two unanalyzed targets are TypeScript fixtures under `testdata`
with no `node_modules`; the run says so and continues. Getting from "run
failed, nothing analyzed" to 18/20 took three fixes, each with the provider's
own words behind it: a module `go list` cannot describe is skipped rather than
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
- Do not reorder request fields for provider prefix caching. Measured dead: the
  shared prefix is 141 bytes and subject sets differ per batch.
- The 429 collapse is not worth touching: 1,013 of 1,014 calls succeeded on the
  first attempt.
- Failing a whole run is never an acceptable answer to a model disagreement.
  Log it to `rejected.jsonl` and continue.
