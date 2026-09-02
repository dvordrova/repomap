# Progress

## On now

Item 3: decide with numbers whether `core` at 97% of backend objects is worth
narrowing.

## Finished

1. **The instrument.** `81fe3e02`. Categorization prints coverage with
   denominators, split objects vs connections, per-category share, requests
   and how many assigned nothing, and rows naming a subject outside the index.
   Fixture, front: was `categorized subjects: 338`; now
   `338/424 (79%) (objects 166/222 (74%), connections 172/202 (85%))`,
   `core 281 (66%)`, `requests: 14, of which 0 assigned nothing`, `outside: 0`.
   Request shape untouched, so the cache did not cold start: whole run 14 s.
2. **Finding 2 settled.** `cbd4aac8`. The 172 are relation patterns of the
   same index — 103 `invokes_external`, 69 `calls`, each anchored. 0 of 338
   name an id in neither set; same for backend's 25 of 239. Not invention, and
   `Result.Validate` cannot accept one; that refusal now has a test.

## Stuck on

Nothing.

## Next

- Item 3: `core` covers 225/296 subjects on backend, 281/424 on front. Measure
  what narrowing costs in group and connection counts before changing a word
  of the prompt — one prompt edit cold-starts 1,014 cache entries in real money,
  so any experiment goes in one batch.
- Item 4: chi from `~/git`, wall clock + call count + tokens; `defaultTimeout`
  10 min against a measured p99 of 50.7 s is the known largest win.
- Item 5: open the report and answer the first-day questions from it alone.

`make test`, `make vet`, `gofmt -l cmd internal` green at both commits.
