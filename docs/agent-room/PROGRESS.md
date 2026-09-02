# Progress

## On now

Reading both reports as a newcomer and fixing what is hard. The owner's
mid-run redirection governs: visual hierarchy, a canvas graph, one
self-contained HTML, script size is not the constraint. Until 11:00.

## Finished

1. **Instruments.** Categorization coverage with denominators, and grouping
   shape, printed on every run.
2. **Findings settled.** The 172 non-object assignments are relation patterns,
   0 invented. `core` is not worth narrowing; the regressed run is what narrow
   looks like. What finding 1 pointed at was two other things, fixed below.
3. **Speed.** Fixture 14.6 s → 5.6 s warm, 55 s cold. chi 111 s cold, 12.4 s
   warm, 4/4 targets. Biggest single win: the run id sat in the facts digest,
   so orientation never hit its cache. Reports are byte-identical across runs
   now. Credential scanning removed on the owner's call (−1523 lines).
4. **The page.** Templates one file per region and per layer. Every target
   opens with an SVG map of its groups sized by membership, with hover preview
   and click-to-group; a repository map of the targets and the calls between
   them; the summary leads and quotes are last and clamped; the main flow is a
   numbered sequence; a sticky target bar and a per-target jump bar with
   counts. 145 KB, 5.4 KB of script, no overflow at 375 px.
5. **Truth.** A key repeated with an identical value no longer discards whole
   answers. Mounted route prefixes compose: `GET /articles/{articleID}` where
   chi printed `GET /`. Listen addresses are facts, so "on which port" is
   answered. **The grouping prompt had no size rule** — the same repository
   gave 4 to 13 groups and the 4-group answers put two thirds of the target in
   one box; now 8–13 groups, largest at 4%, across every draw. The main flow
   reaches the response instead of stopping at the first handler.
6. **Robustness.** A provider timeout is retried instead of losing a whole
   target page. `repomap <path>` no longer fails when VS Code is absent.

## Stuck on

Nothing. `~/git/fuego` fails before analysis on incomplete `go list`
authority; pre-existing, recorded in the handoff, not investigated.

## Next

Keep reading both reports and fixing what is hard. `make test`, `make vet` and
`gofmt -l cmd internal` are green at every commit.