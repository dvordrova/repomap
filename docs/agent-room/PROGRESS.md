# Progress

## On now

The page. Owner redirected mid-run: it lacks visual hierarchy, wants a canvas
graph, says 288 KB of JS is fine, wants embedded well-organised templates and
one self-contained HTML. Working until 11:00. The design authority for the map
is `artifacts/repomap-product-lab/iterations/008-cross-target-system-story/UI-UX-CONSILIUM.md`.

## Finished

1. **Instrument.** `81fe3e02` categorization coverage with denominators;
   `d2c250ba` grouping shape (lanes, subjects in a group, largest group).
2. **Finding 2 settled.** `cbd4aac8` all 172 are relation patterns, 0 invented.
3. **Finding 1 decided.** `d2c250ba` do not narrow `core`; the regressed run is
   what narrow looks like (6 groups over 22 subjects vs 10 over 55).
4. **Speed 14.6 s → 5.6 s (−62%).** `f567a56f` literal gates on the credential
   scans (5.9×); `1730be93` batch planners by search, not O(n²) re-encoding;
   `39014072` **the run id was in the facts digest, so orientation never hit
   its cache** — one live call per run, gone, and the report is now
   byte-identical across runs; `495e41d3` credential scanning removed
   (owner's call, −1523 lines).
5. **chi, medium Go repo, live provider: 110.9 s, 4/4 targets analyzed.**
6. **The page.** `4b65db19` templates split one file per region and per layer;
   `eee34ef9` target map + the parser fix it uncovered: a key repeated with an
   identical value was throwing away whole answers. Front went from 7 groups
   with a 127-member bucket to **12 groups, largest 42**, connections 10 → 20.

## Stuck on

Nothing. Trap learned by paying: the provider timeout is inside the cache key.

## Next

- Repository-level map on the overview (targets and the portals between them).
- Visual hierarchy pass on the linear sections below the map.
- Re-run chi and read that page as a newcomer; it is the real acceptance.

`make test`, `make vet`, `gofmt -l cmd internal` green at every commit.
