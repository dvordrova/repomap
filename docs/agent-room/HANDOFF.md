# Handoff: what is known, what is open

Written 2026-09-02 for an agent picking this up cold. Everything here is
measured, not remembered. Authority for the product is
[docs/CONSTITUTION.md](../CONSTITUTION.md); the architecture is
[CURRENT.md](CURRENT.md); the flags and acceptance ritual are in the README.

## Baseline, measured

One live run over the fixture, both targets, no cache:

| what | value |
|---|---|
| wall clock, backend + front | 66 s |
| accepted provider calls, all four 2026-09-02 runs | 58 |
| input tokens, those runs | 1,122,672 |
| largest single request | 112,338 tokens (one grouping call) |
| largest categorization request | 36,236 tokens |
| calls over the 131,072 window | 0 |
| max output tokens against a 128,000 cap | 1,667 |
| provider latency, all calls | 468.9 s, of which 333.9 s is ONE call with attempts=2 |
| the other 57 calls | 135 s total |
| `defaultTimeout` | 10 min, against a corpus p99 of 50.7 s |

The single largest measured wall-clock win available is lowering
`defaultTimeout`. It is one constant.

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

## Known gaps, recorded not fixed

- Nested route prefixes are wrong. chi's `ListArticles` reads `GET /`, not
  `GET /articles`, because the mount prefix is not composed.
- `os.environ["KEY"]` and `process.env.KEY` subscript reads are not captured.
  Catching them needs a `reads` relation from the adapter, a schema change.
- `targetportfolio.Compile` and `CompileWithExecutableAuthority` are now
  production-dead, reachable only from their own tests. Removing them rewrites
  about ten call sites.
- The `resolveGoTarget` field on `defaultRunDeps` is injected by nothing, in
  production or in tests.

## Traps

- A name-based dead-code scan misses interface satisfaction.
  `runOutputWarningSink.Write` looked unreachable and has four call sites.
- Do not reorder request fields for provider prefix caching. Measured dead: the
  shared prefix is 141 bytes and subject sets differ per batch.
- The 429 collapse is not worth touching: 1,013 of 1,014 calls succeeded on the
  first attempt.
- Failing a whole run is never an acceptable answer to a model disagreement.
  Log it to `rejected.jsonl` and continue.
