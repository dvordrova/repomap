# Progress

## On now
Nothing in flight. Four repositories read end to end, the night's own diff
reviewed, all green.

## Finished
1. **Instruments.** Categorization coverage with denominators, grouping shape,
   and the size of a request plan before it runs.
2. **Findings settled.** The 172 non-object assignments are relation patterns.
   `core` is not worth narrowing; what finding 1 pointed at was a parser
   refusing a repeated key and a prompt with no size rule.
3. **Speed and coverage.** Fixture 5.9 s warm (from 14.6 s), 2/2 targets. chi
   7.6 s, 4/4. python-dotenv 21 s, 3/3. repomap on itself 18/20, where at the
   start of the night it failed before analyzing anything.
4. **The page.** Templates one file per region and per layer. Each target
   opens with an SVG map of its groups, sized by membership, marked with the
   main-flow steps through them, saying how much of the target it covers; a
   repository map; summary first, quotes last; the flow as a numbered
   sequence; sticky nav and per-target jump bars. 5.2 KB of script, nothing
   external, no overflow at 375 px.
5. **Truth, audited not assumed.** 1,172 anchors across four reports resolve;
   41 text-bearing facts and 37 route paths match their source. Wrong claims
   found and fixed: `APP_PORT` does have a default, a package `__init__.py` is
   not dead code, tests are not dead code, no pipeline word reaches the page.
6. **What the provider actually refuses.** Its own messages, twice: the window
   is 1,048,576 tokens and the completion reservation comes out of it. Every
   stage reserved 128,000 against a largest-ever answer of 20,444, now 32,768;
   a grouping request is bounded at 3 MB after a 15.4 MB one was refused; a
   module `go list` cannot describe no longer fails the repository.

## Stuck on
Nothing. In the handoff, recorded not fixed: merge should ask which candidates
belong together rather than for a copy; the index has no value for numeric
literals; `os.environ["KEY"]` reads; whether tests belong on the map.

## Next
Nothing queued. `make test`, `make vet`, `gofmt -l cmd internal` green.
