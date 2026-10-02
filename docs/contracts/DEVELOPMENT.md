# Development and acceptance

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Mandatory fixtures and checks

- `testdata/acceptance/python-tutorial-game` is the acceptance fixture: a tracked copy of
  that repository at revision `78714d34ee` with `expected.json` and the sealed
  artifacts of one real run. Its focused test rebuilds the fact layer and
  asserts every expected row with its anchor.
- Focused discovery and ProgramIndex regressions keep exactly one cumulative
  real repository fixture per active language under
  `testdata/repositories/<language>`. Extend that repository with new scenario
  files instead of creating per-bug repositories. Every tracked fixture file
  must have an exact file-inventory expectation; deterministic language
  adapter stages run for real, while provider-backed stages use exact
  request-bound, fail-closed local presets with no network access. The
  categorizer's preset is `typesafetest.Categorizer`: Jev's own request and
  response code with the test's explicit decision per question, where an
  unknown question fails its request. The prepared Jev request bytes, the key
  of every cached closed decision, are pinned by a golden taken from main in
  `internal/atlas/table/testdata`; a deliberate change rewrites it with
  `-update`. Fixture success is focused test evidence only and never
  replaces ordinary online product acceptance.
- Every language-cube behavior change must be recorded in that language's
  cumulative `testdata/repositories` fixture and an executable expectation.
  Add or extend an understandable source example, run the actual extractor
  and adapter, and assert the changed contract at its consuming boundary.
  Existing coverage may be reused when it demonstrates the exact changed
  behavior. Ownership or resolution changes also keep a contrasting case that
  would expose an invented owner, call or relationship. A passing unrelated
  test or a prose note alone does not record the change.
  When a case found in one language has equivalents in other supported
  languages, immediately add or extend comparable examples and expectations
  in all of them, without waiting for a separate owner request. Check the
  actual language-specific semantics; a missing equivalent is recorded as
  such rather than fabricated. Code that already handles the case needs its
  regression example, not an unnecessary production change.
- Keep the cumulative language file inventories under `testdata/contracts`
  exact and update their executable expectations with each fixture change.
  The separate prompt, numeric-limit and test-file inventories were removed
  in `b9f1c182`; do not recreate them as a second bookkeeping step. Embedded
  prompts and provider contracts are checked by their owning packages, and
  `make test` covers all product packages under `cmd` and `internal`.
- Build the owner-facing binary with `make build`; it must write
  `.bin/repomap`.
- Canonical `make test` and `make vet` use the ambient system Go build and
  module caches, cap package-level parallelism at four, and give each test
  binary at most five minutes. Focused commands must keep the same or tighter
  bounds; ordinary development must not relocate `GOCACHE` or `GOMODCACHE`.
- The owner uses the default system repomap response cache for ordinary
  acceptance runs as well. Do not create a separate model cache per repository
  or UI investigation with `--debug-dir`. Temporary rendered HTML and browser
  servers may stay under the task's temporary directory. System Go caches and
  the configured official DeepSeek and Jev (TypeSafe) endpoints are
  authorized for these runs; both keys are required (owner decision
  2026-09-26).
- Product acceptance means running that binary on a real repository through
  the normal online provider path. Offline runs, fixtures, replay commands, and
  helper tools are not acceptance evidence.
- Verify the process exit status and the generated manifest, exact reduced
  documentation, sealed ProgramIndex set, each target-scoped
  `dependency-catalog.json`, `places.json`, `atlas.json`, `tables.md`, every
  projected GroupsIndex, the complete graph in report JSON, and report HTML. For a multi-target run, verify
  exactly one common manifest/report JSON and one physical report HTML in the
  successful owner run. For cache changes, also verify a real second run and `repomap
  cache clear`.
- Browser QA for a generated standalone report serves the narrow run root that
  contains the owner report and any backing sibling target-run directories from
  a temporary loopback-only `python3 -m http.server`, then opens the corresponding
  `http://127.0.0.1:<port>/<run>/report.html` URL; do not rely on `file://`
  behavior.
  This is a development-only inspection step, not a supported repomap command,
  product server, or checked-in sidecar entrypoint.
- Run focused Go tests for changed contracts and `go vet` for changed
  packages. Never leave known broken tests.
- Debug artifacts must never include API keys or Authorization headers and must
  never be committed.

## Native prerequisites

Language adapters run their native tools from the normal PATH, and `make test`
runs them for real: Git, Go, Python 3, Node for the JS/TS helper
([JSTS](JSTS.md)), Clojure CLI with clj-kondo ([Clojure](CLOJURE.md)), and
clang for C, with make for a repository that has a makefile ([C](C.md)).
Without clang, C targets are not analyzed with the required-tool reason;
other adapters' targets are unaffected.

## Before full expensive runs

An output-limit or request-packing change must first pass one saved complete-window probe; it does not replace ordinary acceptance. Preserve the exact compared inputs, accepted rows, rejected rows, timings and token accounting. Do not repeat Airflow while the prerequisite fixes and Freqtrade acceptance remain outstanding. See [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work).

## Claim audit

`internal/audit` (test-only) scores saved ordinary runs against reviewed ground
truth, `testdata/audit/<repo>/inventory.json`: redis-1.3.6 `7b7f987e`,
litestream-v24 `d26cb54e`, freqtrade `9f10e357`, othello `b8130da8`. Each
`generate.py` rebuilds its inventory from `~/git/<repo>` at that pinned
revision; after the freeze a change is a dated erratum with a code citation.
Run it at milestones only, on main run directories the current binary wrote at
those revisions (siblings come from report.json `files`):
`REPOMAP_AUDIT_RUNS='redis-1.3.6=<run>,freqtrade=<run>' REPOMAP_AUDIT_OUT=<dir> go test -p 2 -timeout 10m -count=1 -run TestClaimAudit ./internal/audit`.
It writes `<repo>.md` and `<repo>.json` there, never into a run or the cache.
Recall and precision have no thresholds. The test fails only on a broken Main
flow link, an invented code name, a quote its cited file does not write, or a
run the product's readers refuse. The package comment states the matching rules.

## Evidence before optimization

### How to judge an optimization against these results

For each expensive computation or request family, first identify the required
information it produces and its downstream reader outcome. Then evaluate
removal, reuse or batching against that dependency. Share identical evidence
and exact accepted answers while preserving source and ownership context. A
separate caption is justified by the explanation it adds; selecting an
operation, boundary or learning concept has its own information requirement,
even when the implementation currently obtains all of them in one request.

A structural observation is not an importance decision. A project-configured
test inventory helps distinguish tests from production declarations; it does
not justify discarding the evidence needed to explain how to check a change.
An exact compatibility redirect can explain where an API moved, whereas a
short file, `internal` path or an Airflow-specific helper name is not a general
exclusion rule. The existing requirement to analyze tools/examples remains.
In the reviewed Airflow snapshot, `pyproject.toml` configures `python_files`
as `test_*.py` and `example_*.py`; copying pytest's default filename patterns
would not reproduce even that declared selection. Pytest explicitly supports
[configured discovery patterns](https://docs.pytest.org/en/stable/example/pythoncollection.html#changing-naming-conventions).
This configuration observation is not an executed test collection or a proof
that every declaration in those files is irrelevant to orientation.
Any later eligibility rule must specify its evidence and effect before request
preparation. A response violating that declared rule is recorded as refused,
without rewriting the original answer into a model-approved positive decision.

Compare the same snapshot by preserved launches, operations, integrations,
concepts, useful question topics, original anchors and explicit unresolved
coverage. Counts alone are insufficient: inspect which subjects disappeared
and whether their responsibilities remain explained elsewhere. Then compare
elapsed time, provider tokens/requests, native preparation work and disk use.
No percentage of closed code or file-length cutoff is an acceptance rule.

## Interactive report assets

`internal/report/web` owns the React Flow + ELK display layer. `make ui-build`
uses the developer's Node/npm installation and ordinary shared npm cache to
rebuild the checked-in `templates/js/27-report-ui.js` and
`templates/css/47-flow.css`. Dependency versions and the lockfile are pinned;
licenses ship in the generated JS. `make ui-test` checks real ELK routing and
input inventory retention, then verifies that generated files match sources.
Neither `go install` nor `make build` invokes Node or a JavaScript builder.

After asset changes, run focused report tests/vet, `make build`, and ordinary
`repomap render` on the same completed runs and translations. Browser acceptance
covers source/question return, exact input selection, map camera stability,
connection-label navigation, dense areas and the separate reading column. The
report contains the runtime JS/CSS inline and needs no network asset requests.

### Canvas browser tests

`make ui-visual-test` runs Chromium against prepared input containing two
systems and five outside systems, with the saved scene a report would carry
for it (`visual/saved-scene.mjs`, scene.go's rules). The test host supplies
records and callbacks to the ordinary bundled canvas: the production code
builds the model, lays out every level and handles the pointer. No provider
request or saved layout is involved. This fixture supplements ordinary
report acceptance; it does not check analysis quality or replace
source/Back journeys in a complete generated report.

`visual/scene.spec.mjs` checks what a reader does: an arrow pointed at opens
its card and a click on it reads its connection; "−" leaves one level per
press down to the whole map; one pinch crosses at most one level boundary
and a pause lets the next cross one more; "Show whole map" keeps what is
read and fits every box; a click on an input kind on the whole map reads
that kind; an arrow keeps its screen width while the camera zooms; a
layout worker failing before the first drawing leaves the static map and no
pending canvas; a drag leaving the canvas keeps every arrow drawn as the
pointing drew it; and a drag from an arrow, a chip or a marker pans the map
as one from empty canvas does, selecting no text, scrolling no page and
opening no card (the arrows take no pointer event: the harness asks the
canvas's own hit test, `[data-map].sceneHitAt(x,y)`, what a point reaches). `visual/stable-labels.spec.mjs` checks that a pan keeps the
whole map's summaries' line boxes. `visual/geometry.spec.mjs`
(`visual/geometry.mjs`) checks the drawn fixture: no two texts overlap, no
text is cut without an ellipsis and a title, every box stands in its frame,
an arrow ends on the boxes it joins and runs on no other arrow's line, and
the canvas prints no kind label, plaque or number of its own (a digit in a
name is a name); on rendered reports named by `REPOMAP_GEOMETRY_REPORTS` it
prints each finding with its level. With `REPOMAP_REAL_RUN` naming a saved
run directory, the test server renders it once with `.bin/repomap render`
(no provider request) and `visual/real-navigation.spec.mjs` walks the
toolbar's breadcrumb up from a part at 1440×900 and 1280×800; without it
that spec is skipped.

`visual/invariants.spec.mjs` (`visual/invariants.mjs`) is the canvas's
invariant table (REPORT, canvas invariants), the acceptance of the scene
canvas. On each rendered report named by `REPOMAP_INVARIANT_REPORTS`
(served as `/invariant-<n>.html`) and each seeded synthetic graph named by
`REPOMAP_INVARIANT_GRAPHS` (`all`, or names of `fixtures/synthetic-*.json`:
no inputs, 150 outside systems, cycles, 40 loose parts; written by
`visual/synthetic-graphs.mjs` in the page data shape the canvas receives
with their saved scene, for node scene tests too), it visits the whole map
(its rest view, then "Show whole map"), every program and every area of the
two largest programs. At each level it checks the arrows at rest, makes 24
random pans and 50 random pointer moves, points at every arrow, marker,
port and faded chip (a marker's or port's tip naming each of its names
whole, within the canvas), and measures markers and ports at every camera; it
writes `<repo>.canvas.json` to `REPOMAP_INVARIANT_OUT`, and
`node visual/invariant-table.mjs DIR` writes `table.md` and `table.json`
(repo × level × invariant, each cell marked with the commit it ran on).
A cell is PASS (its checks ran, none failed), FAIL (one failed), NOT_APPLICABLE
(its phase finished and found nothing of its kind, as the invariant declares
in `visual/invariants.mjs`) or INCOMPLETE (its phase did not finish, or no
check ran and nothing was declared); zero checks are never a pass, and JSON
and Markdown read the status from one place (`visual/invariant-status.mjs`).
An exception anywhere in a level pass (the harness, Playwright or the page,
a browser error or not) leaves that level INCOMPLETE with what it checked
before and where it stopped; the run's JSON is written whatever stops it.
With `REPOMAP_INVARIANT_STRICT` a run fails on every FAIL or INCOMPLETE cell
and every stopped level. `visual/invariants-fault.spec.mjs` forces a harness
failure (`REPOMAP_INVARIANT_FAULT`) before the first check and after a few,
and holds that strict fails and every output says INCOMPLETE;
`invariant-status.test.mjs` holds the same for the table alone. Run it with
one browser:

    REPOMAP_INVARIANT_REPORTS=a.html,b.html REPOMAP_INVARIANT_OUT=dir \
      npx playwright test invariants --workers=1

`REPOMAP_FIXTURE_TEMPLATES` draws the fixture and the synthetic graphs with
another templates directory (a clean `git archive` export) instead of the
working tree's.

`visual/journeys.spec.mjs` (`visual/journeys.mjs`) runs on rendered reports
named by `REPOMAP_JOURNEY_REPORTS` (a comma list, served as
`/journey-<n>.html`; no provider call):

    REPOMAP_JOURNEY_REPORTS=a.html,b.html npx playwright test journeys

`REPOMAP_TEST_PORT` runs a suite on another port beside one already serving
8875 (with `--output` in a directory of its own).

Its frozen journeys print PASS or FAIL per repository and never fail the
run. Each tests the answer a newcomer came for in the rendered column
(dotted names whole across their line breaks) after the clicks it allows,
choosing the input and opening its State changes, never a name merely
listed: othello's `key-pressed` shows each key with its command (n with a
new game, u with undo, h, 1, 2); litestream's `replicate` is handled by
`ReplicateCommand.Run`, not a bare `Run`, and changes the replica, WAL or
snapshots; freqtrade's `trade` reaches `FreqtradeBot.process` and changes
the database (Trade, Order) or its orders; redis's `set` changes
`redisDb.dict` and lists no helper internals (`listNode`, `dict.used`,
shared reply objects). Its reading lints are tests, walking the whole map,
every program, Inputs, Outside, destination, area and part and a sample of
inputs (the journeys' among them) as the explorer reads them, with two
arrow cards at each frame: no `file:line` in a row of the column or a
card, no entry twice in one list, no word cut by an ellipsis or the
browser, no Outside chip naming its own program, no empty card (its words
and marks filling less than its top 45%) or empty frame (a box holding no
other box and no title) in sight, "Parts on this path" naming parts only,
each once, and no fold holding more than 67 rows of one kind (the 95th
percentile of the folded lists of the four reports measured on 2026-09-30)
under no named fold of their own. Each offender is printed with its level.
Leaving an input path and the programs list's clickable rows are checked on
the same reports.

Install the pinned browser once with `npx playwright install chromium
--only-shell` from `internal/report/web`. Browser downloads use Playwright's
standard shared location. The suite runs with `make ui-visual-test`. The
active acceptance project covers 1440×900 at DPR 1 on macOS 15 Intel with the
Chromium version selected by the pinned Playwright dependency. The owner
excluded narrow-window work on 2026-09-14. No pixel reference is compared: a
baseline pins incidental layout, so the tests assert what a reader needs,
readable names inside their frames, nothing overlapping, levels reachable,
geometry reused. Open the review page with
`npx playwright show-report --host 127.0.0.1`; images and the viewer are
ignored build artifacts and no image is committed.

