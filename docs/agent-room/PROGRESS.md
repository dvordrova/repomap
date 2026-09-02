# Progress

## On now

The page. Owner's direction, given tonight: it lacks visual hierarchy, you
cannot see what matters. Wants a canvas graph and real visual work; says
288 KB of JS is fine; wants embedded, well-organised templates so UI changes
stay cheap, and one self-contained HTML you can drop in S3. Work until 11:00.

## Finished

1. **Categorization instrument.** `81fe3e02`. Coverage with denominators,
   objects vs connections, per-category share, empty requests, rows outside
   the index. Front was `338`; now `338/424 (79%)`, `core 281 (66%)`.
2. **Finding 2 settled.** `cbd4aac8`. All 172 non-object assignments are
   relation patterns with anchors; 0 of 338 name an id in neither set.
3. **Finding 1 decided.** `d2c250ba`. Do not narrow `core`: the regressed run
   is what narrow looks like — 6 groups over 22 subjects vs 10 over 55, and 5
   connections vs 14. Real defect is one stage down: front's largest group
   holds 127/424 subjects. Grouping now prints its shape.
4. **Speed, measured on the fixture, warm cache, both targets: 14.6 s → 10.3 s
   (−29%).** `f567a56f` gated the always-on credential scan and the structured
   scan by the literals their answer needs (5.9× on a 120 KB payload);
   `1730be93` replaced two O(n²) batch planners with a search (1 probe instead
   of n when everything fits). Same partitions, no cache moved.
   `defaultTimeout` 10 min → 3 min against a measured 86.7 s slowest attempt.

## Stuck on

Nothing. One trap learned by paying for it: the provider timeout is inside the
cache key, so that edit cold-started all 1,047 records. Recorded in HANDOFF.

## Next

- Restructure `internal/report` templates so a UI change is cheap to make.
- Group graph on the page with real hierarchy; keep one standalone HTML.
- Then a medium repository (chi) end to end for wall clock and cost.

`make test`, `make vet`, `gofmt -l cmd internal` green at every commit.
