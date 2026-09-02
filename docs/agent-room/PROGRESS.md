# Progress

## On now

The page, per the owner's mid-run redirection: visual hierarchy, a canvas
graph, 288 KB of JS is fine, one self-contained HTML. Until 11:00. Map design
authority: `artifacts/.../008-cross-target-system-story/UI-UX-CONSILIUM.md`.

## Finished

1. **Instruments.** `81fe3e02` coverage with denominators; `d2c250ba` grouping
   shape (lanes, subjects grouped, largest group).
2. **Findings settled.** `cbd4aac8` the 172 are relation patterns, 0 invented.
   `d2c250ba` do not narrow `core` — the regressed run is what narrow looks
   like (6 groups over 22 subjects vs 10 over 55).
3. **Speed 14.6 s → 5.6 s (−62%).** Literal gates on the credential scans
   (5.9×); batch planners by search, not O(n²) re-encoding; **the run id was in
   the facts digest so orientation never hit its cache** — one live call per run
   gone, report byte-identical across runs; scanning removed (−1523 lines).
4. **chi, medium Go repo, 4/4 targets: 110.9 s cold, 12.4 s warm.**
5. **The page.** Templates split one file per region and layer; every target
   page opens with an SVG map of its groups; a repository map of the targets
   and the calls between them; the summary leads and README quotes are last.
6. **Truth.** `eee34ef9` a key repeated with an identical value no longer
   discards whole answers — front went from 7 groups with a 127-member bucket
   to 12 groups, largest 42, connections 10 → 20. `e4730fdf` mounted route
   prefixes compose: chi prints `GET /articles/{articleID}` where it printed
   `GET /`. `0bd00990` a listen address is a fact, so "on which port" is
   answered on the overview.

## Stuck on

`~/git/fuego` fails before analysis on incomplete `go list` authority.
Pre-existing, unrelated to tonight, not investigated.

## Next

Check the map's hover and click behaviour in a browser, then keep reading both
reports as a newcomer and fixing what is hard.
`make test`, `make vet`, `gofmt -l cmd internal` green at every commit.
