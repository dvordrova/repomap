# Night brief: make the data worth looking at, fast, on a real repository

> Historical document, not current instructions or acceptance. Original wording
> and historical line numbers are preserved. Use [CURRENT](CURRENT.md) for the
> active decision and [AGENTS](../../AGENTS.md) for work-specific contracts.

You are working alone. The owner is asleep and will judge this in the morning
by opening a report, not by reading a diff.

## Goal

By morning, `repomap` should produce a report on a medium repository, in a few
minutes, that a newcomer can read and learn something true from. Three
properties, in priority order when they conflict:

1. **True.** Every number on the page is checkable and every model sentence
   cites facts that exist. No invention survives.
2. **Worth reading.** The page routes attention. A category that lands on
   nearly every object routes none.
3. **Fast.** Minutes on a medium repository, not tens of minutes.

## Read first, in this order

- `docs/agent-room/HANDOFF.md` — measured baseline, two open findings, the
  traps, and the exact snippet that measures a run. Start here.
- `docs/CONSTITUTION.md` — the product's rules, including the banned on-screen
  vocabulary and the three labelled layers.
- `docs/agent-room/CURRENT.md` — the architecture.
- `README.md` — flags and the acceptance ritual.

## The work, in order

Do these in order. Each one makes the next cheaper to judge. Do not skip ahead
to the interesting one.

1. **Print the numbers that would have caught a 60% signal loss.** Today the
   categorization console line is an absolute count with no denominator and no
   per-category split. Print coverage as a fraction of indexed objects, the
   per-category counts, the empty-shard count, and the count of assignments
   naming a subject that is not in the index. This is the instrument for
   everything below; without it you are guessing, and the owner has already
   been burned once by exactly that.
2. **Settle open finding 2 from the handoff.** 172 of 338 front assignments
   name a subject that is not an object in that target's index. Determine what
   they are. If they are real subjects, say so with evidence and leave them. If
   any are invention, they must stop being accepted.
3. **Settle open finding 1.** Core lands on 97% of backend objects. Decide,
   with numbers, whether that is worth narrowing, and if so narrow it and show
   the before and after including group and connection counts. Remember that
   more rows is not better; the last person to believe that shipped a
   regression.
4. **Run on a medium repository and make it fast.** Use something real from
   `~/git` — chi is the known-good medium case. Report wall clock, call count,
   and tokens. The known largest win is `defaultTimeout`, 10 minutes against a
   measured p99 of 50.7 seconds. Do not redesign request sizing; the handoff
   explains why that premise is already disproved.
5. **Then, and only then, the page itself.** Open the report and answer the
   first-day questions from it alone. Fix what makes that hard. Keep the JS in
   tens of kilobytes.

## Rules

- Work on this branch. Commit each finished item separately with the numbers
  in the message. A commit body that claims an improvement without a before and
  after is not finished work.
- `make test`, `make vet`, and `gofmt -l cmd internal` stay green at every
  commit.
- Never fail a whole run because a model disagreed. Log to `rejected.jsonl`.
- Never add an on-screen word the constitution bans.
- A prompt or request-shape edit cold-starts the whole cache in real money.
  Batch prompt experiments; do not iterate one word at a time.
- Provider key is in `~/.zshrc`. Reading `~/git` and
  `~/Library/Caches/repomap` is allowed.
- If you find yourself designing something with no measurement behind it,
  stop and measure instead.

## Progress file

Keep `docs/agent-room/PROGRESS.md` at **40 lines or fewer**. Rewrite it, never
append past that. It holds only: the item you are on, what you have finished
with one number each, what you are stuck on, and what you plan next. This file
is the supervisor's only window, so it must be honest, especially about items
that failed.

## Supervisor

After each finished item, and never more often than every 40 minutes, spawn a
supervisor to check you are still working toward the goal.

```
Agent(
  subagent_type: "general-purpose",
  model: "fable",
  description: "supervise night run",
  run_in_background: false,
  prompt: """
    Read exactly two files and nothing else:
      docs/agent-room/NIGHT-BRIEF.md   (the goal)
      docs/agent-room/PROGRESS.md      (what the worker claims)
    Do not read source code. Do not run the build or the tests.
    Do not use more than three tool calls.

    Judge one thing: is the worker moving toward the goal, or has it drifted
    into work the brief did not ask for or into changes with no measurement
    behind them?

    Reply in exactly three lines, no preamble, no markdown:
    VERDICT: on-track | drifting | stop
    WHY: one sentence, max 25 words
    NEXT: one sentence naming the single next thing, max 25 words
  """
)
```

Then obey it. `drifting` means drop what you are doing and take NEXT. `stop`
means commit what is green, write down why you stopped, and end.

The cost control here is what the supervisor **reads**, not what it writes.
Two small files and a three-line contract keep each check to a small fraction
of one item's own cost. Do not hand it the transcript, the diff, or the repo.
