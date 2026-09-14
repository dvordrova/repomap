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
  request-bound, fail-closed local presets with no network access. Fixture
  success is focused test evidence only and never replaces ordinary online
  product acceptance.
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
  the configured official DeepSeek endpoint are authorized for these runs.
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

## Before full expensive runs

An output-limit or request-packing change must first pass one saved complete-window probe; it does not replace ordinary acceptance. Preserve the exact compared inputs, accepted rows, rejected rows, timings and token accounting. Do not repeat Airflow while the prerequisite fixes and Freqtrade acceptance remain outstanding. See [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work).

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

### Canvas screenshot tests

`make ui-visual-test` runs Chromium against prepared input containing two
systems and five external participants. The test host supplies records and
callbacks to the ordinary bundled canvas: the production code measures cards,
chooses the layout, routes connections and handles mouse gestures. No provider
request or saved layout is involved. This fixture supplements ordinary report
acceptance; it does not check analysis quality or replace source/Back journeys
in a complete generated report.

Each system has one compact input collection outside its component frame on
the initial map. Its existing input types remain readable; opening the collection
reveals the original named inputs. Selecting an input retains its exact saved
input-to-part relation and reading; returning preserves world geometry. An
individual input promises a path and sources, not another hidden container.

The same prepared input also covers dense area inventories and a separate
shape with two components of twenty parts each, seventeen external participants
and six distinct calls per participant. Assertions check actual text ranges
inside their frames, complete words and separation from zoom controls. Journey
attachments cover component/external entry and intermediate zoom-out states.
Zoom-out checks that a root summary does not coexist with its visible interior.
A restored close view checks that a visible part is not covered by its parent's
summary when the intermediate area's heading has left the viewport.
A partial-to-whole-frame pan at an unchanged readable scale must reveal the
area's original parts and internal arrow without another zoom. Restoring the
before/after cameras restores their corresponding detail states, with identical
world geometry. Both states are captured in the screenshot journey.
An additional five-participant fixture uses short component names, seven/five
area names and eleven/three inputs in the ordinary report's 1054×580 canvas.
Its complete initial inventories must fit, retain whole words and keep those
area names within two lines; the PNG captures the resulting reading layout.

Install the pinned browser once with `npx playwright install chromium
--only-shell` from `internal/report/web`. Browser downloads use Playwright's
standard shared location. Normal comparison is `make ui-visual-test`; explicitly
review and update references with `npm run test:visual:update` in the web
directory. Missing references fail normal runs. The active acceptance project covers
1440×900 at DPR 1 on macOS 15 Intel with the Chromium version
selected by the pinned Playwright dependency. The owner excluded narrow-window
work on 2026-09-14; earlier 1024×768 references remain historical artifacts,
not an active acceptance project. Other operating systems need
their own reviewed references, not automatically accepted images.
The component entrance comparison allows three differing pixels for the observed
macOS glyph-edge rasterization difference; all others allow none. No canvas
region is masked, and bounds, readability and pointer checks remain strict.

The suite checks readable names, complete area lists, external call focus,
stable geometry and the successive stages of pointer-anchored zoom. PNG
checks at the component threshold require a visible child heading or the
retained component heading with a real area entrance; empty frame borders and
the location row alone are insufficient. PNG
attachments show the aim and action for each journey step. Open the review
page with `npx playwright show-report --host 127.0.0.1`; its HTML is a viewer for
the screenshots and, on mismatch, expected/actual/diff images. Actual output
also appears as one scrollable image sequence in `playwright-report/journey.html`.
Failed sequences are explicitly marked as diagnostic output. The images
and the viewer are ignored build artifacts; only reviewed reference PNGs are
committed. CI compares references without updating them and publishes the
review report as `canvas-screenshots`.

The same prepared participants also have a dense inventory variant with forty
additional areas per system. Its default screenshot must keep all nine root
headings and zoom controls readable without overlap. Every area stays in the
scrollable list; the journey scrolls to its last entry, opens that area's actual
part and returns to the same whole-map geometry. This catches repeated
height-reservation and fit calculations collapsing a dense world into a strip.
Both nineteen-root and twenty-one-root journeys instrument real Worker layout
requests. Pan, wheel, zoom, selection and same-size All perform no native layout;
a real desktop resize places only the outer frames. Every original item and
the affine geometry of each interior survive. Outer arrows stop at participant
frames at every zoom while retaining their original endpoint/source records.
These structural work assertions have no machine-dependent timing limit.
Connection-size checks combine actual SVG screen transforms with raster
samples at different zooms. Frame checks cover the browser’s minimum CSS border
width as well as the visible corner radius. A partly offscreen input collection
keeps its complete type list within the frame; scrolling that list leaves the
camera and world unchanged.

A one-target/twenty-destination fixture has twenty parts and six calls per
destination. Its default screenshot must keep every full heading in its own
frame, and no native outer segment may intersect any participant interior.
Trackpad pinch over a scrolling target inventory must change camera scale
without scrolling the text. A gesture journey opens the twenty-part target
without using its zoom button or changing world geometry. Numbered-boundary
checks match an inner part's number to its external arrow endpoint and capture
the part followed by a pan to the actual boundary badge. Each gesture frame also
checks that a partly visible child card cannot cover a retained root summary.

A real vertical wheel pan must move the camera without changing zoom or world
geometry, and must not remeasure text whose visible width is unchanged. This
checks redundant work directly, without a machine-dependent timing threshold.
The resize journey enlarges the desktop window while reading an external collection:
its world remains fixed until the whole map is requested. The fitted result
must keep every full heading and zoom control inside its own frame. Returning
to a whole-map camera saved before remeasurement must honor that intent rather
than restore stale coordinates or retain the detail view.
