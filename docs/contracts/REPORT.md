# Report, source navigation and translation

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Projection and first-screen overview

- `groupindex.ProjectAtlas` turns the atlas into the GroupsIndex the page,
  the orientation and the publication read: a box is a group whose members
  are explicitly selected declarations and their native lexical children, a
  zone is a container, and original native relations provide cross-part
  connections with exact endpoints and source locations. A joint is a connection
  into another target. Lanes follow a box's side: `triggers` where the
  outside calls in or execution starts, `dependencies` where it only calls
  out, `core` otherwise. No request sends repository source text: paths,
  names, signatures, first sentences of docstrings and documentation excerpts,
  literal values and the model's own earlier lines cross the wire. Question
  excerpts preserve Markdown-authored command/code examples; implementation
  source-file bodies are not sent.

- Observed HTTP routes join their target's catalogue through the original
  FactID restored by atlas projection. The renderer neither repairs foreign
  identities nor merges model operations by matching names or paths. Listener
  addresses remain source observations, outside the request-operation list.
  Accepted model wrapper/request interpretations retain their existing status.
  Route summaries count only original displayed HTTP registrations. The combined
  incoming catalogue counts records, showing native registrations and unmatched
  interpreted handlers separately; those records can describe the same endpoint.

- The report is one static page rendered in Go. Its reader is a newcomer, so
  pipeline vocabulary never reaches the screen: retained, source-bound,
  authority, projection, selector, outcome, target contract, and raw selector
  strings such as `python:backend:guard:main` are banned on screen. All answers
  and source evidence are rendered in HTML. The interactive map
  uses React Flow for its viewport and ELK for compound layout and routing.
  Their compiled JS/CSS is checked in and embedded with the ordinary templates;
  the report needs no CDN, Node runtime or external asset directory.

- The home page contains one common System map, built from the existing
  translated component maps by `pageView.SystemMap`. Components and saved areas
  are frames, with their actual parts inside. All selected targets are present;
  unread components keep their original failure note. An exact existing remote
  href can resolve to a local part; names and file paths never establish that
  identity. Every original relation and its source endpoints survives display
  folding. This is a display assembly after translation, not another semantic
  graph, analysis payload or provider stage.

- The ordinary entrance includes every saved request, command, activity and
  interaction. An operation with an explicit implementing part is represented
  by a named blue input card inside that part; all inputs are visible, with
  their original kinds. Each owner groups its named inputs by the saved activation
  type under one heading; the kind is not repeated on every card. Their heights
  are reserved before layout. An unbound
  operation or native HTTP registration remains its own input node. External
  communication records retain separate selectable nodes inside amber
  destination frames, using the same grouping as the external catalogue.
  Each keeps its original source, description and known address. Integration
  arrows attach to the exact communication through its saved connection ID
  (bound by caller subject and complete call location), and continue to the
  original peer input. Equal destination text never establishes this link.
  The frame is a display collection, not a newly inferred component. They use amber cards; ordinary parts and area
  frames use neutral tones. Core parts use purple, inputs blue and external
  communications amber; the saved lane supplies the core identity at both zoom
  levels. Colour names types only; a single dark outline and Reading
  label identify the card open on the right. Dark arrows and outlined participants
  identify the currently emphasized connections.
  Component frames retain their language, kind, role and purpose above their parts. Colour never replaces
  type labels or the independent fact/model provenance in the reading panel.
  Equal destination names do not merge records.
  Existing source-owned cross-component links connect the same common canvas.

- Across zoom levels, selection never changes topology, layout or box size. The reserved reading
  column beside the canvas shows the selected description, exact code, original connections
  and related inputs. There is no separate expansion action to place function
  lists inside a part. Selecting an operation emphasizes its saved path on the
  same map; a one-part operation highlights its implementing part without
  manufacturing an extra operation-to-part diagram. Selecting another part
  retains the selected operation. Its sidebar lists the exact recorded path
  participants and connected inputs; selecting a participant retains the input
  and its original source-backed call-path explanation. A grouped connection
  reading never replaces that explanation. No first part or operation is chosen
  automatically. Search and numbered destinations reveal their result at a
  readable scale, opening its enclosing frames in the fixed world. A hidden
  child's bounds inside the viewport do not count as visible content. A fully
  visible, legible part keeps the current camera when opened from a reading.

- The drawing has exactly one reason for emphasis: search results, the area
  under the pointer, the pinned input path, or the selected part's neighbours.
  Hover temporarily replaces the drawing emphasis; it never unions another
  area's edges into a pinned input path. Leaving the canvas restores that path
  or selection. Reading an off-path part does not make it a path participant;
  the reading card explicitly says it is outside the saved input path. Search does
  not mix old selected or hovered connections into its matches.
  Ancestor frames retain a neutral outline while their descendant is in focus;
  this containment context never adds the ancestor's other connections. Frame
  titles retain a light background and readable role/purpose text. The duplicate
  `Reading` line and close action do not occupy space above the canvas. That
  space has a static type legend, not changing hover prose. The input context
  and leave-path action live in the reading card. An up-chevron in
  the reading card's own header closes details, preserving camera and any pinned
  input path; leaving that path remains a separate action.

- Hovering an area or a part inside it labels its inner parts with local
  numbers. One outside participant and direction form one label with every
  exact inner endpoint number (for example `2 · 3`). Different outside
  identities and opposite directions remain separate. Hovering that label
  shows all original relations, sources and possible-call marks in a separate
  section of the reading column; selected details and their links stay present.
  Moving into that column keeps the connection evidence available. Clicking its
  number pans to the named outside participant. The name
  is plain text. Lines have no click target or native tooltip. These numbers
  identify parts, never execution order. The toolbar has no connection-style
  selector; the same real endpoints remain connected across zoom levels.

- ELK computes one fixed world before setting the viewport. Natural area sizes
  determine a local content scale; the final layout reserves each area's full
  summary height and its scaled internal drawing. The compact summary lists
  every member part and named input under its saved kind. Zoom reveals actual
  parts when their effective screen scale is readable, with hysteresis at the
  boundary. Summaries and detailed contents occupy the same rectangle: no
  coordinates, dimensions, topology or camera centre change at that boundary.
  At distant zoom, component and external-destination frames hide all their
  descendants and internal routes. Their summaries show the saved name and
  purpose where space permits, all saved area names as direct entrances, actual
  part/input counts, and a compact magnifier-plus action. Group names replace the
  bare group count. Text remains at screen-readable size inside the existing
  frame; measured space gives the area list priority over role, counts and
  purpose. Dense lists scroll within the frame without removing any names.
  The full purpose remains in the reading panel. A single toolbar hint explains
  zooming to see inside and dragging to move. The component boundary has its
  own hysteresis, also saved with the camera. No layout runs during wheel zoom.
  Every closed component, area and external collection shows that same action
  in its actual top-right corner, independently of the text column width. Its
  screen size stays fixed while zooming. Open frames and leaf parts do not
  promise another hidden layer.
  Zooming into a component animates toward its first content at readable scale.
  Its header stays within the visible horizontal part of the same frame, so
  a first child placed far from the left edge cannot crop that content or
  remove the component name during the entrance.
  Unbound inputs, external records and unread components keep their identities.
  Routes are clipped at closed areas using their existing orthogonal segments;
  opening an area reveals the original continuation. Internal routes wait for
  their visible parts. Floating-point hierarchy offsets must not produce a
  diagonal clipping segment. A persistent location row names the visible area
  or standalone item and its known ancestors.
  The initial overview fits all root component and communication frames. The
  source SVG stays hidden while the fixed world is being arranged; a loading
  indicator appears in the reserved canvas. Workspace height accounts for the
  actual header and controls. Whole-map mode refits when that space resizes;
  manual pan/zoom ends that mode and keeps the reader's camera. Component
  entrance shows its title and first contents at readable scale.
  The minimum camera scale permits the complete root bounds even in a short
  window. The initial world choice accounts for readable root-header widths and heights,
  not only its bounding rectangle. After placing the full area summaries,
  narrow root frames reserve overview text width before the final layout is
  shown. Compact component purposes use the remaining whole lines, with an
  ellipsis when shortened, and stay hidden if fewer than two lines fit; the complete
  purpose remains in the reading column. External frames reserve summary space.
  The fixed world places frames, parts, complete input cards, component
  purposes and grouped labels. It routes the actual part-to-part
  endpoints through compound containers. Label positions belong to those
  routes; there is no additional boundary route planner, A* layer, custom marker
  packing or replacement path. Orientation candidates receive independent graph
  objects: ELK mutates its input, so reusing a computed candidate can retain stale
  bends. React Flow owns pan/zoom and camera restoration.
  Selection and hover never change box positions or sizes. Normal part clicks
  update the reading column without centering; explicit destination clicks and
  Find move to their exact result.

- The hover area persists through gaps between its labels and clears on leaving
  the canvas, window blur, hidden document or clicking empty canvas. Hovering
  nodes changes only the drawing, never the reading panel. Only clicking another
  card replaces the selected details; the smaller duplicate node preview is removed.
  Connection previews clear on leaving the canvas/reading workspace, window
  blur or a new selection, and never cover parts or routes. A pinned input keeps
  its exact saved path when exploring other parts. All participants of a hovered
  area stay readable. The map uses available window width independently of
  prose width. The initial view is the whole-map summary; reset centers the selected
  item at readable scale (or the topmost part when nothing is selected). A
  regenerated layout invalidates old camera coordinates and reveals the selected
  item instead. A navigation, click or camera move cannot trigger a different
  hover emphasis under a stationary pointer; real pointer movement resumes hover.
  Viewport history includes its zoom, open areas and the fixed world's geometry identity.
  All no-script sections, operation catalogues and source links remain available.

An input path follows native call/execution edges. Reads of declared values or
types by its reached code appear as terminal data dependencies, with a distinct
read label and the original read site. They never execute a data owner's other
methods or activate a stored callback, integration or mutation. One shortest
call/read witness explains each reached part; imports and part membership alone
do not establish a path. All original structural relations remain available.

Both map legends list only categories present in that map. The expanded legend
describes current selection and static paths, without removed toolbar controls.

Group readings put outgoing connections before incoming connections, followed
by Code in this part. The original native relation is available even when the
model supplied no sentence for that pair. Relations between declarations in
the same part remain available under Connections within this part, with their
original call/read sites, destination declarations and resolution. Membership
alone adds no relation; containment remains the source inventory. Native code labels keep their own
provenance; model summaries remain marked. Distinct relation IDs preserve
same-line call occurrences. Selecting a peer pans to it; Back restores the
camera and expanded source evidence. Connections share one heading per
exact participant href and direction, with each distinct saved summary once.
Source details retain every original row, possible-call mark and source pair.
Unknown destinations and different identities never merge by title. The reading
states when no connection to another part exists in this report. That absence
does not classify the declaration as unused or invent a connecting edge;
internal relations and the complete source inventory remain available.
The reading column uses these same grouped sections; the duplicate full-group link and vague
All disclosure are removed. Returning to a part restores its expanded evidence
and reading scroll as well as the canvas camera. Saved core/entry/dependency
lanes remain named in the map and reading; an import is not promoted into an
external communication.

## Reader context

The complete component/input catalogue remains available beside the same map,
including all original purposes, manifest/entrypoint anchors, incoming and
outgoing observations. Catalogue rows are all visible within their saved type
or destination; only an individual record's evidence needs disclosure.
This does not expand or change the canvas. The Parts count uses the same local
leaf groups across all lanes, excluding operations, frames and foreign nodes.
Its link focuses the component on the common map. The separate core-lane code
reference is labelled Core, and cannot imply an empty component map.

When no item is selected, the reading column shows entrances to the complete
saved question menu, run material, terminology, missing observations and author
claims, plus the component purposes with original provenance and source links.
It uses ordinary rendered content, not an additional model summary. Selecting
an item replaces that home reading; closing details returns to its top. Answers
and reference material continue below the same mounted canvas, and Back
restores the camera and selected reading.

The header, sticky toolbar, canvas and reading sections share the page's left
gutter. The toolbar spans the available width so scrolling content cannot peek
around a narrower, centred bar. Question reading keeps its line-length limit
without shifting into a separate centred column on wide windows.
The reviewed repository name links to the reviewed directory and revision when
it is nested in a repository. It sits on the left beside an icon-only Home
action; the repomap name and GitHub mark link to repomap on the right. The same
header contains Questions, the component list and one visible Find field.
All is the list's default and runs the same whole-map action as Show whole map,
clearing selection, input context and the visible Find query after recording
the new visit. Component choices and Back restore their
matching selection and camera.

The toolbar retains the current question and the exact component/area/part
path. Browser Back/Forward, a source-details side trip and reload preserve the
question, selected operation, exact code selection, map search/filter and viewport. The completed initial overview camera is saved
even with no selection or hash, and restoring an empty selection clears the
previous inspector. Imperative camera changes are saved after they finish.
Restoring a complete visit is not followed by another hash-driven
selection; an explicit overview/detail transition remains a separate visit
even when the selected item's URL is unchanged. Reset keeps the current level.
A named Back to map returns from full source
details; Back to question returns to the original answer. Old component and
map links resolve to the same common canvas. Answers, component reference and
repository material open below that mounted canvas, and each has a direct
return to it. Component sidebar links expose the existing flow, configuration,
data, core, dependencies, dynamic execution, coverage and TODO sections when
present. The redundant single-component entrance is not generated alongside
the common canvas. The repository summary and useful links remain, with no
"Understand this repository" or visible "Starting points" label.
The introductory sentence has no model badge or source popover. Its complete
saved citations and model attribution live in Repository summary sources,
reachable from the home reading column and without scripting.
Display-only iteration uses the
same saved analysis and translations, with no provider calls.

Answer provenance, checks, supporting readings and question origins share one
collapsed apparatus; existing source IDs and excerpts survive. Terms receive
one underline per answer, and source-unavailable labels remain inspectable.
Header revision noise, redundant answer actions and root Path:. are removed;
run information remains linked from the footer.

The page exposes the complete saved question menu, one open answer, its
position in that menu, a next question and an explicit return to all
questions. That menu is a single vertical list of topic headings with every
question immediately visible under its saved topics. It has no parallel all-
questions grid or nested topic disclosures. Questions absent from the accepted
topic plan remain in a plain fallback group. The current question remains
visible while scrolling or taking a map side trip. Repository search lives in
the header; compact results have short plain-text excerpts, without source or
model popovers. Full answers and provenance remain at the result's destination.
Exact code-name matches precede matches buried in answer prose.
The former Learn/Work switch is gone: the
owner could not tell the two entrances apart, and the switch changed only
the intro, the question list and where the search field stood. A `mode`
query parameter in an old link is ignored. Component reading entrances use
only the saved answer's exact map links. Inline terms now link directly to their existing map
memberships and select that declaration by its source, without a catalogue hop.

The selected part's panel reads its existing interpreted concepts and group
highlights; otherwise it shows the original declaration index without inventing
an explanation. Equal names with different source locations remain separate.
Original source links and full target evidence retain their order and are
available without scripting.

## External communication and data

Selecting a component on the common map opens its existing purpose, entrypoints and complete input catalogue in the panel. Its original full-reference section remains available with the main flow, configuration, group cards, dependencies, coverage and TODO lists. A component without a flow or start list shows no flow section.

The entrance reads accepted outbound communication directly from GroupsIndex, independently of dependency lanes and key captions. Each observation shows the other participant's role, purpose, known literal address or explicit unknown, and its original call/source chain. Dispatch and explicit remote-client configuration stay distinguishable. Native addresses and code names remain original; role/purpose use the existing display bindings. For display the observations are grouped by destination text (case-insensitive; native label or kind when the model named none): one row per destination carries the record count, the shared kind, basis and address (or the number of distinct addresses); its records are compact nested lines (the native method and address, else the callable, else the first sentence of the purpose, with the source location), all lines visible, and each line opens its full purpose, address, basis and call/source chain. The section count and the first screen count destination groups; a destination text is still not proof of one remote system. Dependency/import groups stay in collapsed code reference and do not supply the integration count. Empty observations do not prove that the service contacts nothing.

The existing Data shelf lists source-scoped models/tables, written columns and keys, queries and original sources. Query-to-table references are reversible, so a table exposes its referring queries. Equal table names never merge source scopes; SQL mentions and JOINs do not prove schema ownership or foreign keys.

An outbound record also lists native callers of its exact sending callable.
Each caller links to its original part and preserves the native caller/callee
names, possible-call status and source pair. A shared client group is not
enough to attribute a caller to another method. This is source reading in the
panel; it does not add a map edge or infer a runtime caller.

An operation or native route exposes Data links only when the existing accepted path reaches an exact native model owner or a query's explicitly observed callable owner. Tables link back through those same query references. Exact versus possible call status remains visible; source association is not proof of database execution. A legacy class owner must not imply that every method uses every query in that class. Callable-owner projection requires its exact path, line and column; explicit extractor ownership is accepted. Native embedded SQL literals currently lack that lexical callable authority, even when a language retains their original literal anchor, so the consuming function is not assigned locally. An unresolved endpoint-to-method link remains a gap rather than a completed endpoint-to-table flow. No additional semantic stage or graph is introduced.

## English identities and display translation

- Semantic output is canonical English. The owner approved final presentation
  localization on 2026-09-07/08: `--lang ru` translates the already-built English
  frontend structure, with an ordinary UI dictionary and a separate LLM cube
  for generated display prose, then renders `report.<repo>.ru.html`. Translation
  requests carry one local dictionary of exact spelling/definition pairs and
  each text's applicable refs. Equal spellings with different definitions stay
  separate; each partition rebuilds its complete dictionary. Source
  excerpts, names, IDs and topology remain original. Operation names stay
  English in every display language; exact command/path labels remain verbatim,
  while descriptions and UI labels are localized. When an interpreted action's
  name is exactly its native declaration name, the report uses that subject's
  existing English alias with the native name beside it. A distinct action
  label is never replaced with its function's alias; literal commands and paths
  remain verbatim even if they match a declaration name. The default English file
  remains `report.html`. Current ordinary acceptance is recorded in [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work). `llm.Prepare` adds one shared response-language system fragment
  before provider encoding, execution, fit checks and memo identity. Empty
  ResponseLanguage means English; the final translator supplies its language.
  Exact-byte replay does not rebuild or add this instruction.

- Display translation alone also halves a whole refused response window after
  JSON or translation validation fails, per the owner's 2026-09-09 request for
  automatic recovery. Original entries remain complete; no refused fragments
  are published or cached as accepted answers. The same exact-request split
  memo records this as `response_validation`, not a provider resource limit,
  and applies it only while the owning stage opts in. Valid child windows keep
  their ordinary cache entries. A singleton the provider answered but refused
  (missing entry, placeholder mismatch, validation or envelope failure, resource
  refusal) keeps its source-language text: the entry is published untranslated,
  `rejected.jsonl` gets an `entry_untranslated` row per text and the console
  names them once; a failure before any provider answer still fails the stage.
- Display translation starts with at least eight complete windows (or one per
  text for a smaller catalogue), balanced by original text bytes, after checking
  for an accepted whole-window answer. The existing four-worker pool executes
  them. Unused translated-entry fields such as `terms` are ignored; the required
  text and original placeholders still validate. Each child retains its own
  complete term dictionary. A failed window does not cancel its neighbours:
  all divisible failures from the round split together, while successful
  windows remain in memory even with the cache disabled. Only unfinished
  windows run again, through the same shared pool and rate-limit gate. Per the owner's
  2026-09-09 endpoint timeout report, translation attempts have a local four-minute
  deadline. Its expiry returns directly to the lossless splitter and is memoized
  with that deadline; it never retries identical bytes. The owner also approved
  immediate splitting on HTTP 500 for divisible translation windows, regardless
  of elapsed time or response wording. This exact-request observation is stored
  as `http_500`, not a resource limit, and reused only while that policy is enabled.
  Singleton HTTP 500 responses and all other stages keep ordinary transport
  retries. Parent cancellation stays terminal. Retry
  reasons, attempt numbers and actual retry starts are printed immediately.

## Translation persistence

`--lang ru` selects one physical `report.<repo>.ru.html`, using the selected
repository checkout's directory name. A Go module suffix such as `chi/v5`
does not produce `report.v5.ru.html`. Default English publication retains
`report.html`. Canonical `report.json` remains English.
A separately saved display translation retains its language, original text
catalogue hash and closed text refs; the manifest names that presentation and
the receipt carries it in memory. The server restores the same values when it
re-renders source links. UI translation is an embedded dictionary, not another
model call. `--no-model` must still make zero provider calls and uses only that
dictionary. Initial supported display languages are `en` and `ru`.
The question's eight fixed scope explanations also use this dictionary at
rendering time. Their canonical English route values remain unchanged and
never enter the model's display-translation catalogue. Saved rendering applies
the same vocabulary without analysis or provider access.

Translation reports each adaptive request-plan size and its closing count of
new logical model calls versus accepted cache hits. Before new calls it plans
at least eight windows (fewer when there are fewer texts), using the existing
four-worker pool and balancing complete entries by original UTF-8
text bytes. This is work distribution, with no row quota or smaller provider
envelope. Every child rebuilds its complete term dictionary. A validated cached
whole-window answer takes precedence, so an upgrade does not retranslate it.
Failed live calls count as new; HTTP retries remain separate exchange metrics.
A 1,585-text regression verifies eight initial windows, four requests starting before any response,
complete ordered output, protected spans, dictionaries and zero-call warm reuse.

Independent translation windows use the shared adaptive `Each` executor. One
failed request no longer cancels its running neighbours or restarts the complete
plan. All divisible failures in a round split together; successful windows
remain in memory even with the persistent cache disabled. Each child builds
only from its own complete original entries. Progress reports the unfinished
requests, and original exchange observers run once per executed request.
Extra translated-entry fields such as echoed `terms` are ignored. The required
`text` and original protected spans still validate; unused metadata neither
authorizes a changed translation nor triggers another provider call.


## One publication

The current manifest records the repository and publication source. Across a
successful repository run the following artifacts are persisted, as applicable:

- repository corpus and repository-guidance authority;
- `reduced-documentation.json`;
- the target ProgramIndex reference, shared `program-facts` input and `program-index-set.json`;
- the target-scoped `dependency-catalog.json` artifact;
- the matched `groups-index.json` artifact;
- `program-page-portfolio.json`;
- `target-outcome-portfolio.json`;
- `facts.json`, `claims.json`, `orientation.json`, and `rejected.jsonl`;
- manifest-bound `report.json`;
- the single owner `report.html`.

Multi-target publication retains each target's ProgramIndex, dependency
catalogue and GroupsIndex. Shared artifacts, the manifest, report JSON and HTML
are published once in the owner run from values already held in memory. A
served report has the same complete set of target sections; it needs no sibling
report files. Saved report restoration reads the common JSON and manifest.

UI iteration uses `repomap render RUN_DIR --output FILE.html`. It restores the
current common report and completed display translations with `ReadRunReceipt`,
restores captured remote source links, and calls the same `RenderHTMLWithOptions`
as ordinary publication. No provider or cache is initialized and no analysis is
run. Missing or incompatible saved data fails without regeneration. The original
analysis and translations stay unchanged; the chosen HTML is atomically replaced
only after successful rendering. This is the owner-requested 2026-09-08 remedy
for hand-assembled UI prototypes and cache-dependent analysis reruns. Editing the
ordinary templates, building and rendering saved data is the UI acceptance loop;
analysis changes still require ordinary pipeline acceptance.
Adding destination-side views of existing cross-component connections exposed
an ordering dependency in display translation refs. Saved rendering now rebinds
only a complete bijection of the same catalogue entries (role, text and protected
spans), using the original saved display catalogue. Both catalogues and both
translation bindings are validated. It never translates, drops or invents an
entry, and leaves saved files unchanged. Missing or changed prose still fails.

## Source links and serving

- Repository changes during a run do not fail publication. Do not reintroduce a
  freshness gate or strict-snapshot mode.
- `--no-serve` requires resolvable GitHub or GitLab source links and fails in
  preflight with corrective flag guidance otherwise. A corpus file absent
  from the captured revision or changed locally never blocks HTML publication:
  keep its path and line as plain text with a `No source` hover explanation.
  Preserve the code cube, explanation and navigation, and every unaffected
  permalink. The outer run checks path availability once; its standalone
  manifest retains that list for repository-free saved rendering. Served
  reports may only
  add manifest-authorized local editor opening; do not add browser APIs for
  workspace reads, investigation, symbols, source context, or run selection.

A report without a remote source link retains the original code, explanation and navigation. A served report opens the analyzed working tree only through the existing local editor action; it adds no browser analysis, investigation, symbol or workspace APIs.

## Three layers of truth

- Everything the report shows is a deterministic fact, a claim quoted from a
  human-written artifact, or a model hypothesis, and the three are always
  labeled. `facts.json` holds the anchored fact layer: entrypoints, HTTP routes
  and client calls with method and path literals, cross-target portals,
  environment keys, the places where the program runs code it was given,
  manifest rows, TODO markers, imports, dead modules, negatives, and
  dependencies. `claims.json` holds quotes with their
  source path, date and age. `orientation.json` holds the model's repository
  summary, roles, run recipe, and main flow; every row cites fact, claim, or
  subject ids. The orientation request walks a packing ladder: 40 members per
  group with 6 calls and 6 callers of observation per listed member, then 20
  members with 3 and 3, then 12 members without observations; `member_count`
  is always the real size and the facts and claims are complete at every
  rung (Freqtrade: 1.74 → 1.18 → 0.92 MB). A refusal by size or context, local from a
  declared context window or remote, moves to the next rung; when the last
  is refused the report is published with an empty orientation, the refusal
  in `rejected.jsonl` and an `unavailable` state in the console. Unknown or incompatible set refs are recorded and removed;
  repeated refs are deduplicated. A row with no required evidence, an invalid
  scalar choice or conflicting interpretation goes to `rejected.jsonl`
  with its raw output and reason. Independent sections and rows survive a bad
  neighbour. Complete prose is preserved; short labels normalize whitespace.
  Orientation prepares its original input against the actual provider envelope,
  without an artificial 2 MiB cap or size-triggered evidence removal. Validation
  annotates and never aborts the run.

## UI iteration and review

UI experiments change the ordinary report templates and use this render path on
the same saved reports. The owner permits separate Git worktrees for competing
designs. Keep them temporary: accepted implementation commits move into `main`
through merge or cherry-pick, never by reimplementing the experiment. Remove a
worktree and its branch after its useful work is integrated or explicitly
discarded; preserve uncommitted work and watch disk space. Current acceptance is
desktop with a mouse; mobile layout does not drive this redesign.

The owner's UI review method, clarified on 2026-09-07, follows a pair of real
questions through the visible interface. Choose actions by their visual
affordances, search first among prominent elements, then explore beside the
relevant item. An unfamiliar term starts a side question. After navigation,
assume the reader remembers learned terms but has lost their place; the page
must help them orient again and start the next question. Record these journeys
in `repomap-ui-ux-review.md`; DOM existence checks do not substitute for them.

The owner added a separate return-after-a-break check: after leaving and
returning to the tab, can the reader tell both where they are and what they
were doing? Evaluate the current screenshot with the click history forgotten.
Retained UI state alone is insufficient. UX31 records this for both Learn
side questions and Work investigations; its importance is still for the owner
to assess. Do not infer an unrecorded user intention from the selected object.

### Whole input paths and reverse reading

The common map joins already saved operation paths only through an exact
activation endpoint. It retains all reached paths across multiple components
and cycles, without borrowing a sibling operation in the same part. Native
source steps and possible integration steps remain distinct in the sidebar.
Outgoing communication is attached to inputs through execution reachability
of its saved caller subject. Clicking a part or communication lists those
inputs by activation type; selecting one restores its full path.

The main toolbar contains search and camera controls. Search retains its full
inventory; the former type/style switches are not displayed. Leaving an input
path or closing details is a contextual action. The color key includes only
present categories. Repeated `model` badges are suppressed while source and
model-response inspection remain available. Same-level focus transitions take
420 ms; history is captured after the latest camera movement settles. Automatic
level changes remain pending; the explicit two-level layout contract above
still applies.

Reachability is not entity mutation. The current concept projection reads
accepted native type declarations only; value-based domain models without a
named type are not silently invented during rendering. Target-bound native writes
carry their original call-site location into GroupsIndex. The report lists writes
only when the input reaches the writer through execution edges and the written
variable has an exact native type owner. It retains possible receiver/call
resolution, the write source, and the source-backed call witness; reachability
does not claim execution on every request. Entity readings reverse these same
records. Matched inputs may extend the witness across a possible integration;
sibling inputs gain no effects. Unresolved writes, ordinary reads and mere
membership in a type-bearing part do not establish mutation. Older saved graphs
without write locations produce no invented evidence.
