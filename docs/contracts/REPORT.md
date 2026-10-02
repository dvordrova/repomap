# Report, source navigation and translation

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Projection and first-screen overview

- `groupindex.ProjectAtlas` turns the atlas into the GroupsIndex the page,
  the orientation and the publication read. A box is a group of explicitly
  selected declarations and their native lexical children; a zone is a
  container; original native relations give cross-part connections with
  exact endpoints and source locations. Containers keep the atlas's zone
  order, the order the areas answer listed them, never a key sort. A joint
  is a connection into another target. A group is `triggers` only when it
  holds a target seed (the program's launch point), `dependencies` when its
  box only calls out, otherwise `core`. No request sends repository source
  text or source-file bodies: only paths, names, signatures, first docstring
  sentences, documentation excerpts, literal values and the model's own
  earlier lines cross the wire; question excerpts keep Markdown-authored
  command/code examples.

- Every drawn box is a part of the map of parts; no file or inventory box is
  drawn. The atlas's off-map record lists, per target, each file (or stray
  declaration) no drawn part holds, with its file line, captions, keys and
  reason: `left_out`, `conflict`, `no_units` (the file declares nothing),
  `map_failure`, `undecided` (a split file's declarations no box of it took
  after a question) or `blocked` (their helpers, unreached by any question
  because a declaration using them got no box). A stray declaration in a file
  a part holds names that part (`box_id`) and the file stays on the map, as
  does a file whose code several parts hold (READING, the role split).
  GroupsIndex carries the record as `off_map`: off-map declarations in a file
  a part still holds by `subject_ids` under their own reason (`undecided`,
  `blocked`, or `left_out`/`conflict` for a box or a stray method), never as
  an off-map file; a file no part holds whole; the files of test-only parts
  as `tests`, and by file with their subjects the declarations of a part its
  program never runs (atlas `unreached`, READING) as `unreachable`, both with
  their part's name; and `map_failure`. Off-map subjects keep their
  interpretations outside every group. A boundary in an off-map file names no
  box and its operation joins no group, yet it stays in the component's
  inputs. An input whose handler is off the map, undecided included, names
  no box either (READING, "A file in several boxes"): a command table row's
  request whose handler is undecided is never shown as handled in the part
  holding the table. A test-only part is a fact (every file a `TestSources`
  file), not a model decision; it and a part its program never runs are not
  groups: they draw no arrow and leave
  that program's canvas.

- The component card lists, after its link to its parts on the system map
  and before its main flow, the compact inventories **Tests** (test-only
  parts' files, with their part) and **Not on the map** (every other off-map
  file with its reason; a split file's row shows its undecided declarations
  as source chips, reading "In no part of its file", and its blocked helpers
  in a row of their own, "Used by code in no part"), five rows each, then
  `All N`. Find lists each undecided declaration as Code with Open code and
  no "In part" link; its result opens that row, scrolled below the sticky
  toolbar like every page destination. A map failure reads "The map of parts
  is unavailable:" with its closed reason in the reader's words (`refused`:
  the model's answer was refused; `no_model`: no model was asked; the
  refusals are rejected rows, never card text) and lists all the target's
  files under Not on the map. A part or area without a description shows
  "No description", never its title again.

- Observed HTTP routes join their target's catalogue through the original
  FactID atlas projection restores. The renderer repairs no foreign identity
  and merges no model operation by name or path. Listener addresses stay
  source observations outside the request-operation list. Accepted model
  wrapper/request interpretations keep their status. Route summaries count
  only original displayed HTTP registrations. The combined incoming
  catalogue counts records, showing native registrations and unmatched
  interpreted handlers separately, though both may describe one endpoint.
  Its registrations are labelled "Registrations in source", not HTTP ones,
  since commands a client sends by name are listed there too; a method badge
  appears only when the registration states a method.

- The report is one static page rendered in Go for a newcomer. Pipeline
  vocabulary is banned on screen: retained, source-bound, authority,
  projection, selector, outcome, target contract and raw selector strings.
  Question answers and their source evidence are rendered in HTML; every
  reading the column renders is in the page data. The map uses React Flow for its viewport and ELK for
  compound layout and routing; their compiled JS/CSS is checked in and
  embedded with the ordinary templates: no CDN, Node runtime or external
  asset directory.

- The dated examples behind this contract's canvas rules are in the
  CHANGELOG entry of 2026-10-01, ["REPORT.md's canvas case law moved
  here"](../agent-room/CHANGELOG.md#2026-10-01--reportmds-canvas-case-law-moved-here).
- At every level the canvas keeps these invariants, checked by
  `internal/report/web/visual/invariants.spec.mjs`: each drawn arrow is one
  polyline from its own source to its own target, its head pointing in; no
  two arrows share a run longer than 6 px unless they are the two
  directions of one pair; every arrow end lies on its box, on a port or in
  the level's frame, never off the canvas; a pan never changes the level or
  removes an element; pointing changes only emphasis and drawing order;
  dark arrows are drawn after grey ones; every arrow opens its card; ports
  keep 20–28 screen pixels at every camera; titles at one level read within
  ±10% of each other; a frame whose body is in sight has its title wholly
  in sight and uncovered, and on the whole map so does a closed program or
  Inputs box (`title-sight`, below); a closed card, program or part-group
  whose part in sight can hold its words shows them wholly in sight
  (`name-sight`); no digit and no label (a badge, an
  arrow's caption) is printed on the canvas.

- The home page has one common System map built from the translated
  component maps; components and saved areas are frames holding their
  parts. Every selected target is present; unread components keep their
  failure note. An exact remote href can resolve to a local part; names and
  file paths never establish that identity. The node readings keep every
  original relation and its source endpoints. The canvas draws one arrow per
  directed visible-node pair, combining its operation membership and short
  labels: coincident call, callback and implementation rows are evidence on
  one connection, not extra geometry. Opposite directions share one route
  with a head at each end; the readings keep the original directions. This is
  a display assembly after translation, not another semantic graph, analysis
  payload or provider stage.

- Groups and containers keep target-local `g*`/`k*` identities in
  GroupsIndex; map/DOM identities add the target ID so equal local ordinals
  never collapse two maps. Every target-local `n*`
  subject and source location resolves through its owning target; no global
  unqualified subject table lets one executable replace another. A component
  section reuses its `t*` target ID; group, operation and data anchors append
  their `g*`, `o*` or `y*` ID, never a second name-derived identity. Equal
  source locations in sibling executables create no foreign node or
  operation path; only an explicit cross-target connection does.

- The ordinary entrance includes every saved request, command, activity and
  interaction. Each component's input nodes form one blue display frame,
  headed "Inputs" (the colour key's word), outside the component in the
  layer next to it, its arrow into it; the component is named by that
  arrow, by the frame's zoom-mark accessible name and in the location row.
  Closed, it shows its kinds' marks, each named on hover; its magnifier
  enters it. Entered, its inputs stand by kind, never by part ("Scene
  model"), each kind a framed group titled by its name and mark, its
  inputs' tiles in rows. A click on a kind's mark on the closed frame, or
  on an input marker on a box, reads that kind in the collection, as
  choosing the kind does (owner, 2026-09-30: "я в колонке не вижу, что я
  тыкнул на канвасе инпут какой-то"); a click on an open kind's group reads
  the collection, on a tile its input. The groups are display containment,
  not architectural areas, and add no relation. A tile says its kind by a
  small muted mark before its name (Primer Octicons), never by a printed
  kind row; the same mark stands before
  a kind's name in the reading column and the map's key, and a kind with
  no mark of its own (an entry whose kind is not established) wears a
  neutral dot. A collection lists its kinds as the column names its
  sections (Scheduled tasks apart from Background work, Queue consumers,
  Extension points, Kind not established). A tile names a callable written
  inline as the column does, "anonymous function in {function}". A program's
  Inputs and Outside frames are named with their program in Connections and
  cards ("← Inputs · redis-server"), and a part's reading is headed "Part".
- An input whose handler is not established (GroupsIndex `handler_unknown`:
  an option a call declares, a value handed over) has no owner: no
  implementation arrow binds it, and such a request is no route and joins no
  portal. From saved `DeclaredBy` and catalogue data its relation from the
  Inputs goes into the part where its code takes it in, where its marker
  stands ("declared here" on hover): an option or word into its declaring
  function's part ("declared in …"), a table's row into the part of every
  function reading the table ("looked up in …"), none when nothing reads
  the table. The relation means "taken in here", never "implemented in":
  its card and Connections line count inputs, not handlers, the reading still
  says the handler is not established, and the function is no handler for
  reach or phases. Every collection's Inputs arrow goes into its own
  component: when none of its inputs has an arrow into a part, each input's
  arrow goes into the component itself, and ELK places the collection beside
  it by that arrow.
- No synthetic type nodes or runtime relations are added. Bound inputs keep
  their exact identity and directed implementation relation, neither
  replaced by nor duplicated inside their part. An unbound operation or
  native HTTP registration stays without an implementation attachment.
  Selecting an input opens the same saved path and sources.

- External communication records without an exact local peer stand, per
  program, in one amber Outside frame beside it (owner, 2026-09-29): one
  chip per destination its records name, every chip one size, in rows in
  the order "Scene model" gives (its buckets included), naming it in at
  most two lines, its whole name on hover, and the records naming none in
  one muted "not established" chip last.
  A destination's calls are read in the column when its chip is clicked,
  the camera staying; no call tile is drawn, and the program's arrows to
  its destinations are one arrow to its Outside frame, the calls behind it
  in its card. That card reads the destinations the arrow reaches, each
  under the parts calling it, not the calls one to a line; the calls are
  read in the column. One call written once (the same destination and
  symbol at the same saved path and line, owner, 2026-09-28) is one call, in
  the first program's destination, with an arrow from each program making
  it. A system several programs call (the same destination name in each:
  the reading names one service behind a package alike in every program)
  stands once too, each program's calls its own arrow (owner,
  2026-09-30: "both targets use the database"). Such systems stand in an
  Outside frame of the programs calling them, named after them, its first
  program first ("cmd/litestream, cmd/litestream-test"), in its arrow
  cards, reading and breadcrumb; a system one program calls stays in that
  program's frame. An Outside frame and its chips are display collections,
  not inferred components.
- A program entered draws nothing beyond itself: its inputs and outside
  systems stand as markers on its boxes' edges and its links to other
  programs end on ports of its border ("Scene model"), the whole map
  keeping its Inputs and Outside frames. A system's marker wears the mark
  its calls' facts give (database, request, queue, SDK, runs a program; a
  plain dot for none), a port the program mark. A marker or a port pointed
  at names what it stands for, one name to a line (owner: hover answers
  what is this), and lights every marker reaching one of its systems or
  taking one of its inputs. A click reads that kind in its collection, that
  system (the Outside frame when the marker stands for several) or the
  program its port names first, the camera staying; reached by the
  keyboard, each is a button, Enter reading it. Markers and ports keep one
  screen size at every level and never grow with the map (owner,
  2026-10-01); the geometry lint reports a port item off 20 to 28 pixels
  (`port-size`).
- When a saved connection identifies one displayed peer in another target,
  the canvas connects the original caller straight to that peer/input, with
  no third participant; its outbound catalogue and source reading stay
  intact. Missing, partial or ambiguous matches stay separate; equal
  destination text never establishes the link. Both endpoint sources,
  operation membership and possible status survive the display projection.
  Source-owned cross-component links connect the same common canvas: the
  whole map draws them as one arrow between the two programs, an entered
  program as its box's port.

- Outside frames and their chips are amber, parts and area frames neutral; core parts are
  rose, entry parts green, inputs blue and external communications amber,
  identities the saved lanes supply. Each colour means one thing (owner,
  2026-09-28): purple is a link and nothing else, a key declaration is bold
  ink with no colour of its own, and an input's name on the canvas is the
  inputs' blue. A closed area's card carries a distinct diamond (core) or
  arrow (entry) glyph on its top border near its left end, the legend
  drawing the same glyphs, instead of Core/Entrypoints above every title; a
  part shows its lane by its colour.
- An area's mark is its GroupsIndex container's (READING): the core mark when
  any part in it is the domain, the entry's area included (owner,
  2026-09-27); the entry mark only on the area holding the program's entry (a
  declaration its execution starts from, a target seed) when no domain part
  stands in it; otherwise none, or the dependencies mark. Only the part
  holding the program's launch point has the entry mark; a part that only
  takes requests or listens has none and does not make its area the entry,
  its inputs' blue markers showing where the outside calls in. The component
  reference's "Input responsibilities" list holds only the entry part. A
  launch point no part holds makes no entry part and no canvas label; the
  component's heading, reading and "Not on the map" list name it as the
  program's entry with its off-map reason, the heading and reading by its name
  and file:line anchor, written as other source links are.

- Concrete input captions remain. A dark outline marks the card open on the
  right without inserting a row or moving its text. Dark arrows and outlined
  participants mark the emphasized connections: a participant takes the
  arrows' dark on its border, and the card open on the right keeps a heavier
  outline. Nothing pointed at is greyed: the subject takes the dark (a part
  its outline, a frame its border at the arrows' weight), the parts across its
  dark arrows take the same outline, and a pointed frame's own parts and
  inner arrows stay as they are. No veil or tile fill marks the pointed
  thing.
- Hover highlights and never dims (owner, 2026-09-28). Only the reader's own
  choice (a chosen part, frame or declaration, a pinned input path, search
  results) recedes every part, frame and arrow it does not involve. The
  pointer or an open arrow end changes nothing else: what it outlines and
  darkens comes forward, everything else stays as with nothing pointed at, so
  crossing the gaps between tiles flashes nothing.
- Colour never replaces the visible type cues or the independent
  fact/model provenance in the reading panel. Text keeps at least 4.5:1
  contrast; meaningful frames and connections, non-selected neighbours
  included, at least 3:1; only what a chosen emphasis does not involve
  recedes below that, while the choice lasts. A grayscale screenshot and
  browser colour measurements cover this palette; they claim no full WCAG
  conformance for the report.

- Across zoom levels, selection never changes topology, layout or box size.
  The reserved reading column beside the canvas shows the selected
  description, exact code, original connections and related inputs; no
  expansion action places function lists inside a part. Selecting an
  operation emphasizes its saved path on the same map; a one-part operation
  highlights its implementing part, with no extra operation-to-part diagram.
  Selecting another part keeps the selected operation. Its sidebar lists the
  recorded path participants and connected inputs; choosing a participant
  keeps the input and its source-backed call-path explanation, which no
  grouped connection reading replaces. No first part or operation is chosen
  automatically. Search, Find and explicit destination clicks move to their
  exact result when it is out of sight, entering the level that holds it
  at no less than that level's entry zoom; a hidden child's bounds inside the
  viewport do not count as visible. The camera moves only to what is out of
  sight, otherwise the canvas marks it (owner, 2026-09-28): a box drawn
  wholly in sight, the level entered itself, and a declaration of the part
  entered keep the camera.

- Dark arrows and outlines have exactly one reason: search results, the part
  or frame under the pointer, the pinned input path, or the selected part's
  neighbours. A pointed part is the subject itself: only its own arrows
  darken. A frame's title, border and empty space look at the frame,
  darkening its arrows that cross its border.
- A pinned input path is drawn from its saved reach (the input path, below)
  in the dark emphasis: one arrow per pair of parts where a call (or read) of
  the reach enters a part from one reached earlier, at a lower depth. None is
  chosen by length; the reading lists every such call and counts the rest.
  An arrow is dashed only when none of its calls is exact. A caller off the
  map stands for the earlier parts reaching it through off-map code. Other
  calls among the same parts stay ordinary arrows.
- GroupsIndex marks an arrow quiet (`Connection.Quiet`, defined in READING,
  Quiet: initialization or a call into a helper, only in a target that serves
  something, never emptying a map); a call into a helper stays quiet even on
  an input's path. The canvas draws a quiet arrow only where one of its ends
  is looked at: inside the box entered (an entered program draws all of its
  own) or the box chosen, whatever the pointer crosses. It decides nothing
  of its own: it is drawn as its calls are, solid or dashed, never in a
  style the key does not name. An arrow drawing several relations is quiet
  only when all of them are. The system canvas is the
  page's one figure; the page writes no static picture of it.
- Hover temporarily replaces the dark emphasis and never unions another
  area's edges into a pinned input path. What the chosen path or selection
  recedes stays receded under the pointer, except what the pointer
  highlights; leaving the canvas restores that path or selection. Reading an
  off-path part does not make it a path participant; its reading card says it
  is outside the saved input path. Search mixes no old selected or hovered
  connections into its matches.
- Ancestor frames keep a neutral outline while a descendant is in focus,
  adding none of their other connections. Frame titles keep a light
  background and readable role/purpose text. No duplicate `Reading` line or
  close action takes space above the canvas; that space holds the map's one
  key, not hover prose: each kind of card as a small card in the fill and
  border the canvas paints it with, its mark on its top border near its
  left end as on the cards; the solid "calls"
  and dashed "possible calls" strokes; and, when a part's tiles draw one, the
  dotted slate link from a function to the type it returns or from a type to
  the function taking it ("returns or takes a type"), drawn as the tiles
  draw it. The
  key lists only the kinds and strokes present in that map; no prose legend
  stands under the map. The input context and leave-path action live in the
  reading card.
- An input chosen from Find, a link or a reading is entered as its path while
  the column reads the input; its tile clicked on the canvas is read with its
  path pinned and the camera still, as every canvas click reads without
  moving (owner, 2026-09-29): a card's body anywhere reads its card with the
  ordinary pointer, and only its magnifier zooms. Entering the path, the
  camera enters the area holding every part the trace reaches, else their
  program, and frames those parts at no less than that level's entry zoom,
  their top left first when they do not fit there. Leaving the path returns
  to what was read before the first path entered, camera and all, never to an
  earlier path; with nothing read before it (a link, a reload), to the
  whole map the column then names. The path's parts, and a closed
  frame standing for parts hidden in it, are outlined in the path's dark. An
  input without a trace is entered at its kind's group, and such a tile
  clicked keeps the camera. "Show input" stands in the reading card while
  the camera may be away from the input's tile (on its path, or on a part
  read since) and frames the tile's kind group in its collection, never the
  whole collection; a group larger than the canvas at the collection's
  entry zoom is entered at its top left. Once the tile is framed there is
  nothing to return to; choosing it again returns to the path. The input's reading is under
  "An input's reading" below.

- Nothing on the map is a digit (owner, 2026-09-28): no part number badges,
  digit chips, key line explaining numbers or numbered/arrows switch.
  Nothing stands on an arrow's end (owner, 2026-09-29): the arrow is its
  own handle, its end an ordinary head on the border it enters. Both
  directions of a pair of frames share one route. The same real endpoints
  stay connected across zoom levels. An arrow pointed at is dark and the
  boxes at its two ends take the dark outline; nothing recedes.
  Dark arrows are drawn over every grey one in a halo of the canvas's
  colour, and while one is dark the grey ones fade; boxes are untouched
  (owner, 2026-09-30: "you can't tell where it comes from").

- A card (an arrow's calls) opens on intent, only after the pointer pauses on
  its handle; a handle crossed on the way elsewhere opens nothing. A
  connection's handle is its drawn arrow, at every level: a wide unpainted
  hit path along it, under the boxes it joins, which take the pointer first.
  The pointer on an arrow is on the connection of the head nearest it: in
  that direction, the outgoing connection of the box the arrow leaves, else
  (from a port or an Inputs frame) the incoming connection of the box it
  enters. An arrow between two parts of one frame is the calling part's
  connection to the other and opens its card. The geometry lint rests the
  pointer on sampled arrows at every level and reports one that opens no
  card (`no-card`). An arrowhead stands outside its frame's border. A card
  stands flush beside its handle, clear of it, on the side with room and
  wholly inside the canvas. While a card is open or kept, the frame being
  read stays: the way to the card crosses parts, frames and empty canvas
  without changing emphasis. Leaving the handle, the pointer is safe inside
  the triangle between where it left and the card: the card lasts and no other handle on
  the way takes it; moving from an arrow onto its own card is not leaving.
  A click on a card keeps it open; a click on an arrow reads in the column
  the frame whose connection it
  is, scrolled to its Connections with that connection open, and leaves the
  camera (owner, 2026-09-27). A kept card has a ✕; ✕, Escape or a click on
  empty canvas closes it, and that click closes the card and nothing else: it
  neither selects nor moves the camera.
  The card's list scrolls under its heading and ✕; the list's own headings
  are rows of it, never drawn over its rows, and the card prints no count. The pause, the
  linger and the triangle are interaction timing, not evidence limits.

  On the canvas an arrow's card answers what it is: headed by the two
  frames its arrow joins, it names each part the arrow goes into (not the
  one its heading names) and under it what it reaches there, one name to a
  line, each name once (a callee, a field, the inputs a handler takes; a
  call leaving its program as the function asking on one side and the one
  answering on the other, "cliConnect ⇢ acceptHandler"), the first dozen
  then "…", never a list of caller → callee rows; every call and its caller
  are read in the column, a click on the arrow away.
  In the column a connection (owner, 2026-09-27) is headed by the two frames its arrow
  joins, from → into, and one line of counts: calls of each kind, from how
  many parts of one frame into how many of the other ("{n} calls, from all
  {a} parts into {b} of {c}"), and how many go the other way, a link opening
  that direction's card. Its calls stand under the part they are made from
  (most calls first), then under the part they go into; both headings stay at
  the top while the list scrolls. A call is caller → callee, the caller
  linking to where the call is written and the callee to its declaration, in
  call-site order; a caller is written once for its run of calls, and one
  caller calling one callee from several sites is one call. Kept open, the card also shows
  on top an index of the parts at each end with their counts; a part at the
  calling end leads to its calls. The page data marks the calls of a dispatch
  site whose adapter retained several alternatives and the calls of a
  declaration handing every member of such a set over by another relation;
  each caller's marked calls are one line ("{site} → one of {n} · {k} here",
  "{caller} passes callback the same {n} as {site} · {k} here"), its callees
  under it by part, each part opening to their names. Go decides set
  membership and size from the relations' retained targets; the browser only
  counts what stands behind the arrow. A caller calling every member itself,
  or handing over part of a set, keeps its rows.

- The canvas's layout, levels, camera, words and markers are specified in
  "Scene model" below; this section states only what the reader sees
  besides. Areas and programs are laid out by ELK layered, never packed
  into a grid; what does not fit the canvas is the camera's job (owner,
  2026-09-30, who judged the layered Core "ахуенно" and the grid
  "пиздец"). Arrows keep room between frames (owner, 2026-09-29): the
  whole map's lanes stand as far apart as ELK spaces its edges. Layout runs
  off the main thread, in an ELK worker inside the self-contained page.
- An arrow joins two boxes of its level: one into an entered area stops at
  its frame, its card standing for the parts behind it, and the arrows
  inside an area are drawn where it is entered. Relations sharing a pair of
  boxes use one route, dashed only when every original relation is
  possible. A call left unresolved whose store witnesses name its
  candidates is possible toward each of them (READING, Operation
  ownership): dashed like alternatives, its rows in the source details
  reading "possible" in the muted text, the key's dashed stroke saying what
  it is. Aggregated strokes are display geometry drawn once, not semantic
  relations: original endpoints, certainty, possible status and source
  relations stay distinct in the reading data and operation paths, every
  original edge ID and relation survives the bundling, and the drawing
  invents no relation. A program is a member of its own connections: a
  call reaching a running copy of it (page_system_map.go) has its card and
  column line.
- A closed area's card shows faint outlines where its parts stand, never a
  blank box, as a program's card does once drawn large and while its role
  and description are too small to read (owner via the coordinator,
  2026-10-02: etcd's contrib/lock/storage and tools/etcd-dump-db had stood
  on the whole map as a name over a blank card); only a box that can be
  entered has a magnifier. An architectural area holds at least two parts
  (reading draws none smaller; GroupsIndex keeps no container of fewer than
  two groups), and the canvas draws every area it is given; Inputs and Outside
  frames keep their boundary even with one child. A title wraps only between
  words, as the browser wraps: closing punctuation stays with the word before
  and an opening bracket with the word after, so no line starts with ")" or is
  ")" alone. A word breaks inside only when it alone is wider than the line,
  after a separator or between camelCase words when it can, never before
  closing punctuation; a path breaks only after a "/", a segment itself only
  when it alone is wider than the line, and never before its extension. Map
  titles are drawn as those measured lines, laid out once against their box at
  its level's text size, so zoom and pan never rewrap them; a part's title
  leaves room for its magnifier.
- A level is entered framed whole at no less than its entry zoom; a box
  larger than the canvas there is entered at its top left, the rest a pan
  away. A part's magnifier enters it at the zoom its declarations read at
  (eleven pixels): the part whole when it fits there, else its head and
  first column at the canvas's top left. Entering a level (a magnifier, a
  reading, an input's path) sets it before the camera moves, and that move
  never changes it.

  A part with declarations has a magnifier; entered, its declarations
  stand inside its card as tiles, each a declaration to choose: a type with
  its fields and methods under it, a function or a module's variable alone.
  A module variable its file keeps only as its handle on the
  platform is no tile (GroupsIndex `PlatformHandles`: a standard-library
  call's result only its file's functions read, Python's module logger);
  the reading still lists it. A tile keeps its link column, the longest
  chain of calls, returns and takes leading to it, so a caller stands left
  of what it calls.
  Tiles stand by file, the files in the order their first declaration is
  listed, each file's declarations in the page's order: the model's keys,
  then the types, then the rest (owner, 2026-09-28); the file is a hint.
  A part most of whose declarations share one namespace (Clojure's
  `othello.ui.host/`) names those without it, the whole name on the tile's
  hover and in the reading; one of another namespace keeps its whole name. In the column a name breaks only after a dot or
  a slash, never at a hyphen or inside a word; a piece too long for its line
  also breaks after its underscores, or at its words' humps when it has none,
  and is never cut with "…" (reviewer, 2026-09-30), the whole name on its
  hover.
  They stack in that order in their column, a column too tall spilling into
  the next. No name is cut, and the columns share the card's width; the
  tiles are drawn small enough that every tile stands, however many the
  part holds. The part's name stands over them at the same screen size at
  any such scale. A drag over the declarations pans the map; a drag that
  moved chooses nothing.

  Pointing at a declaration darkens only its own arrows: its links to the
  part's other declarations, and the part's arrows carrying a call into it
  (the call's callee is its source) or out of it (the caller is its name);
  nothing recedes. A click on a tile chooses that declaration: the part is
  read with it named in the reading column, the tile keeps the read outline,
  its arrows stay dark, the declarations its links do not join recede while
  it stays chosen, and the camera stays. A modifier click opens its code. A
  declaration the reading column names by its source (Find's code hit, a
  declaration chosen in the reading, a restored visit) is the one chosen on
  the canvas; a restored visit keeps its camera. A newly named one out of
  sight is shown with its part at the zoom where the part's tiles are drawn
  and the part stands whole across the canvas: framed whole when it fits,
  else across with the tile centred down it, never deeper than the part's
  own title fits (owner, 2026-09-29). A part too dense for its tiles to be
  read there is shown as its frame instead: its card closed at the scale a
  part is read at, the chosen declaration alone in it as its tile, the
  camera staying when that card is in sight (owner, 2026-09-29;
  `scene.mjs` `memberView`). The page data gives each
  tile its file and the same source link its reading uses, served or
  static.

- On the canvas the wheel moves the map and never scrolls the page. Where
  the reader is is said once, by the page's breadcrumb, with the reading
  column's heading naming what is read and the frame holding it (owner,
  2026-09-29); the canvas keeps its location only for assistive
  technology. A toolbar hint explains zooming and dragging. The
  unpositioned drawing stays hidden, with a loading indicator in the
  reserved canvas, until the layout and the camera are both ready; the
  workspace height accounts for the actual header and controls. A layout
  failure on opening removes the canvas and leaves the ordinary report
  available, with no loading state left. A resize lays every level out
  again, fitting the whole map again or framing the level entered in its
  new place; a failed relayout keeps the last world. Viewport history keeps
  the camera, the level and the layout's identity: a saved camera returns
  only on the same layout, else the whole map is fitted again, at rest or
  whole as it was.
- Emphasis darkens and thickens a line, never its head: every arrowhead, a
  link's head between declarations included, is an ordinary one's size.
  Strokes, casings, dashes, arrowheads, frame outlines and corner radii keep
  their screen size at every zoom. The first click reads and zoom is
  separate (owner, 2026-09-28): a click on a box, a frame, a chip, a marker
  or an input's tile reads and marks it without moving the camera; only the
  magnifier, "+" and a zoom gesture zoom, never a double-click.
- Hovering changes only the drawing, never the reading column, and clears
  when the pointer leaves the canvas or crosses empty canvas; a click, or
  outside a pinned input path a zoom that enters a level, changes what the
  column reads, and there is no duplicate node preview. A navigation, click
  or camera move cannot trigger a different hover emphasis under a
  stationary pointer; real pointer movement resumes hover. The map uses the
  available window width independently of prose width. Every section and
  source link stays reachable from the page.

An input path is its saved reach: GroupsIndex follows native call/execution
edges (READING) and the page draws what it saved. Reads of declared values or
types by its reached code are terminal data dependencies with a distinct
read label; they never execute a data owner's other methods or activate a
stored callback, integration or mutation. Every call entering a part from a
part reached earlier explains that part; imports and part membership alone
do not establish a path. The path is not a neighbourhood: its arrows are the
entering calls' part pairs (above), and its reading lists the parts by call
depth from the handler, then by the first declaration reached in each, a
part reached through a matched input after them. Call depth is a static
measure, not an execution order. All original structural relations stay
available. With an input pinned, a part's card says "Why it appears in
{input}" with the calls entering it and the count of the others, never a
single shortest path.

The reading column (owner, 2026-09-28) renders page data Go prepared and
sorted; the script sorts and repairs nothing. Nothing in the column moves
after it is shown except by the reader's own action.

The page's data holds every answer, and the page needs JavaScript to read it
(owner, 2026-09-29); a `<noscript>` line says so. Nothing the column renders
from that data is also printed as HTML: no printed part card, input
catalogue row or state changes, no static canvas drawing; a glossary term's
files and lines are written from the data when "Files in which this term
appears" is opened. A part's page anchor stays as an empty element naming
its map node, so links to it read it. The page data is written compactly in
one `<script type="application/json" id="rm-page-data">`, each fact once,
and the script reads every value back exactly as Go registered it
(encoding: `page_data_table.go`); a test reads the compact data back through
the page script on both GitHub and GitLab link shapes. An arrow whose calls say its words
carries only the relation an input's arrow reads by ("implemented in"), not
every call's joined words.

Model text is told apart by its style alone (italic, a hover "written by the
model"), with no chip before it. A part named anywhere in the column stands
in a small box drawn as the canvas draws that part (its frame, core's rose
with its diamond, entry's green). Lists are plain names, a model's key in
bold, with no square or chip before a name. Every name the column reads is
shown on the canvas too: a declaration's tile is chosen, a part, component or
input is read and marked, the camera moving only when it is out of sight,
and the address follows (a new declaration in the same part is a new visit,
so Back returns to the one read before). Every name in the column links to
all of its declaration's code: a plain click reads it, a modifier-click
opens the code. No name, and no card's source, carries a line number (a
card's source reads as its file, linking to the line, and not at all under
its linked code as written) and the page prints no
separate code marks (owner, 2026-09-29): a caller, callee or variable is its
name, once however many places the relation is written, its part named on
its hover; where each relation is written is on its name's hover. Of those
places the page data keeps a "Called by" caller's first, in source order,
with every place in its words (`site`).

A part's reading heading gives its kind and its frame as a link up ("Part ·
{frame} ↑"). "Called from" lists the parts calling into it, each
in its box with its caller → callee pairs, and under each caller the
declarations of this part it reaches, each list by name; every input
registered at the part is one neighbour, Inputs. A relation is said once
for its run of names, in plain words (reviewer, 2026-09-30: "— passed as a
callback" had followed each of thirteen names): each name once with every
relation it has there, plain calls first, then the names of one set of
relations under one quiet line ("possibly called, passed as callbacks:"),
a lone name keeping its words on its row; a caller whose names are one run
says it on its own line ("cmdTable passes these as callbacks"), and a
caller dispatching into the part through one site reads "call() calls one
of these request handlers through cmdTable", folded. Callers of a
declaration and "Called from" of an outside call are said alike ("may
call it:"). Many callers fold each part to its line. Then come the
part's box, the model's description, its files one to a line, its key
declarations in bold one to a line, and every other declaration under one
closed "Other declarations" fold, file by file, those reached from outside (a
caller in another part, an input registered at it, a callable handed over)
first; a part with no keys stands its ways in open instead (owner,
2026-09-29). "Calls into" lists the parts it calls, each in its box with its
callees by name and the variables it uses there, folded as "Called from" is
when long. With an input pinned, the input's witness stands under the
description ("Why it appears in {0}"); the inputs reaching the part and the
state changes of its types close the reading, folded. The reading shows the part's saved interpreted concepts
and group highlights; without them it shows the original declaration index and
invents no explanation. The reading's relations hold every original row and
possible-call mark, each end by name alone, and the arrows' cards every source
pair. A model's sentence naming no two declarations stays on the arrow's card;
the original native relation is in the reading even without a model sentence
for that pair. Membership alone adds no relation; containment stays the source
inventory. Distinct relation IDs keep same-line call occurrences; on the card,
connections share one heading per exact participant href and direction, each
distinct saved summary once.

A declaration chosen in the reading, on its tile, from Find or from another
declaration's reading replaces its part's reading with its own, read from
the relation rows its part already lists: a heading with its kind and its
part as a link up ("Function · {part} ↑"); who calls it
("Called by", "Used by" for a variable or a type), by the part at the other
end, its own part first, one line per declaration; its name as its tile
writes it (a function with what it takes and returns, a type with its
fields), the one link into its code, underlined under the pointer (owner,
2026-09-29); its file alone; the author's comment as written,
when there is one, marked as the author's claim; the model's line only when
there is one (without it, nothing is said about one); one closed "Reads and
writes" holding "Writes:" and "Reads:" (below); for a type, every field with its type, linking that type when it is
the repository's (ProgramIndex `Object.Types`), and the functions of its part
that return or take it; for a function, its flow; what else it relates to
("Calls", "Uses"), by part; then a dispatch site's reading, named, its counts
on its hover. A relation other
than a call keeps its own words. A catalogue's callers ("called from
{caller}") and the Inputs reading's "How these were found" are named
the same way: a function making undecided calls once per symbol and reason,
one with calls the code cannot follow once, with their count. A variable
read by a hundred functions folds each larger part to its line. Choosing a
name reads that declaration in its own part, in place when that is the part
being read, so a chain is followed one call at a time; equal names with different source locations stay
separate. A method or a field is named with its type ("ReplicateCommand.Run",
"RPCMessageType.ANALYZED_DF"); two declarations a part's reading names
alike are named apart where the reading is made (`tellDeclsApart`,
`groupindex.TellApart`): by their file's folder, else their file
(headscale's policy.PolicyManager and v2.PolicyManager, beets's
library.Item.path and plugins.Item.path), and the column tells two one list
still names alike apart the same way, quiet after the name (litestream's
ReplicaClient, one per package). Inputs of one kind a program names
alike keep their names as registered and read apart by the words saved
beside each (`page_apart.go`, chosen when the page is assembled, never by
the browser), in the Inputs list (the whole told-apart name its link), on
the input's canvas tile, in its reading's heading, in "Input: …" and in the
page's path line, each word saying on its hover what it is and where it
is written. Each input takes the first word that tells the group apart and
that it has: the subcommands it is an option of (freqtrade's two
"--erase"), its catalogue's declaration (dataformat_ohlcv in
SCHEMA_TRADE_REQUIRED), its key (GroupsIndex `Operation.Key`: the first
word its registration wrote beyond its name, else a handled input's
handler, etcd's two cobra "start" startGateway and startGRPCProxy), a word
its handler's own code declares, of its kind (`Reach.SubArguments` the
handler declares, first in source order), the function registering it;
inputs still alike add the next word that differs (reviewer, 2026-10-02:
etcd's server listed fourteen "POST" under Election and lock APIs; now
"POST /v3electionpb.Election/Campaign · RegisterElectionHandlerServer",
the route argument being a package variable whose value no fact reads,
and the Observe whose handler declares no word "POST
RegisterElectionHandlerServer"). Words only, never code as written, which
stays behind the link (owner's review, 2026-09-30: redis's setting "save
strcasecmp(argv[0]"), never a URL composed, never two inputs merged;
inputs of two kinds (redis's command save and setting save) are not taken
for each other. An input's reading calls the words its handler declares
"Words its handler declares".

Who writes and reads a field (owner, 2026-09-29) comes from the program's
exact reads and writes of record fields (`page_field_uses.go`): a C or Go
relation with its `field_path` (C, GO), a Python typed receiver's field and a
JS/TS declared property, whose words are `Type.field`. A record type's
reading gives under each field's row
"Written by" and "Read by": the functions writing and reading it by any
path, each side grouped by part in the part's box, own part first, then the
part naming most, each name once. A global variable's reading lists the
fields the code reaches through it (paths rooted at its name, made by a
function reading the variable itself), in the source order of each path's
first field, with the same two sides. A function's reading lists "Writes:",
the fields it writes in first-write order. A declaration's reading lists
"Reads:" instead of a "Uses variables" list: the fields
it reads, each by the path the code reaches it by (`field_path`: the root
global variable, or the record type holding the chain's first field when the
root is a parameter, a local or a call's result; `Type.field` without one),
and the module variables it reads whole, each once, in first-use order. Each
name in "Writes:" and "Reads:" reads the type declaring the field, or the
variable. A field it also writes stays on "Writes:"; a name leading to a
longer path it reads or writes is said by that path; a local or parameter is
never listed. Every name stands on a line of its own; a side of more than
twelve names folds. Each
list is one value of its part's reading (`fields`, `writes`, `reads` on the
declaration's entry, names by declaration index), written once; a list two
readings repeat is written once in `shared`.

A declaration two programs of the report hold is one declaration by exact
identity (`groupindex.DeclarationKey`: path, line, column, kind and name;
never a name alone). A program holds the files its map claims, those of its
parts' declarations and those it names off the map (GroupsIndex `OffMap`);
an index drawing no map holds what its ProgramIndex declares. Programs
sharing one project index (Python's) each declare the whole project, but a
script program holds only its own file and what it imports there
(2026-09-30: freqtrade's `FreqtradeBot.process` had listed
`Worker._process_running` under its own part and again under each of five
script programs). Its "Called by" lists, after its own program's callers,
the calls each other program's reachable code makes into it, a caller only
under a program holding the caller, grouped and named by program
("{program}: [{part}] {caller}"), each name reading it there; the
declaration calling itself is no caller elsewhere, and in its own part no
declaration is listed among its own callers or callees (othello.ai/move's
two-argument form calls its three-argument form; its flow shows the call). When its own program's adapter proved it never runs the
declaration while another program calls it, the reading says "Not called in
{program}" and its tile is drawn quiet. A call leaving its program
names each program's side from that program's own code: the shortest run of
calls from the nearest declaration no other program holds to the one making
or taking the call ("{program}: … ⇢ {program}: …"). A call to an outside
endpoint has one side, its program unnamed, and says it is outgoing ("… →
{callee} outgoing"); its row in the component's reference says the program
connects out from that run.

A setting a compared word declares is read by its key as the code compares
it, the comparison as written linking that line, then "Writes: …": the exact
field writes the declaring function makes on the lines that comparison guards
(ProgramIndex pattern `branch`), each field once in written order, each
reading the type declaring it; without a branch it names none. What else the
declaring code uses folds under one line with its count ("{function} also uses
{n} variables"), each variable's other users under theirs, in the setting's
reading and its collection (owner, 2026-09-29). A Go struct-tag setting keeps
its field and tag.

A function's flow (owner, 2026-09-29) is what it calls in the order the calls
are written: GroupsIndex's calls, callbacks, executions and library calls
from the declaration, ordered by their first call site's file, line and
column, each callee once with every place it is called, a dispatch site one
call ("one of {n}"). It has no caption repeating the function's name and no
count or meta word. The calls stand under each part's box once (its
description on hover, a click reads it), the parts in the order of their
first call, each part's calls in written order (reviewer, 2026-09-30: redis
main had shown "Server lifecycle and cron" four times). A library call is no row:
one muted line ends the flow, "also calls: …", each name once in
written order, its library on hover, gathering those of every call opened
in place too, each such name's hover saying who calls it (reviewer,
2026-09-30: othello's key-pressed had stacked three). A declaration no part holds is a plain
name whose own flow still opens, carried in the calling reading, so no call
is dropped. A call a macro's expansion makes is shown as the code writes it:
the macro, once (C's `macro_expansion` witness and the call's selector),
never its expansion's platform calls, named on that line when
its body is the platform's; when its expansion calls repository declarations
the row opens to them as a call does, its hover saying what it expands to. A
compiler builtin (the adapter's `builtin` package) is never a call of its
own. A twist, shown while an openable call is pointed at or focused, opens
the call in place to its callee's flow, grouped the same way, with no box
repeated when all stay in its part. A call one of its ancestors makes says
"↑ shown above" instead. A function never lists itself among its calls: its
call of itself is one quiet line, "calls itself" (othello.ai/move's calls
had opened with "othello.ai/move() ↑ shown above"). What a call hands over ("passed as a callback") and
where it is written ("called at {file}:{line} · {line}") are on its name's
hover.
A function's calls stand under "Calls". Its helper calls fold under one
muted "+ helpers" naming none of them only when there are more than three;
three or fewer stand as rows (owner, 2026-09-29: processCommand's
lookupCommand and queueMultiCommand had waited among eighteen names). "+
helpers" opens them into lighter rows in place and "− helpers" folds them;
one quiet toggle, "Show helper calls", opens every step's. A call is a
helper when its callee is a declaration the helper question decided serves
others' work and stands in a part more than half of the program's other
parts call into (decided from the calls, never by name), never the caller's
own part, even a widely called one; a step whose every call is a helper
shows them as its calls. What is open stays open across the toggle. A step of the component's
Main flow opens in place to its code flow the same way, the model's sentence
kept in its style above it.

The Main flow is read once, at the top of the component's reading; the
component's page keeps it hidden as the column's source (owner, 2026-09-29).
No other reading repeats or links it, save one small "Main flow" link above
every reading of a part, declaration, input or frame of a component that has
one, reading the component at that section without moving the camera. The
model's main flow is the orientation's (READING): its order is the model's,
read from each member's calls in written order. Two steps in a row naming
one declaration are its one step, the later's words kept; a method is named
with its type, and two declarations the flow names alike by their module or
file ("trade_commands.start_trading"); a callable written inline reads as
GroupsIndex names it everywhere in the report ("ReplicateCommand.Run
(inline)", `ObjectFacts.Inline`), never `Run$1`. A step
citing a
registration of a repository callable, or naming a callable some
registration hands over, reads as that callable, never as the registrar
(owner, 2026-09-29), with where it is registered and what runs it as the
walk saved them (orientation `FlowStep.Registered` and `RunBy`, version 3,
READING); the page derives none of it. A run of calls reads its hops in
words, never a count: "→" for an exact call, "may call" for a callable the
name before may call through a value, "hands over" for one it hands over
("aeProcessEvents may call acceptHandler → createClient registers it"; redis
had read "processTimeEvents → serverCron → syncWithMaster → createClient
registers it", the replication path exact calls alone found). The step's
name and every name in
its registration and runners read that declaration; the words "registers it"
link the registering call's line; the step prints no line (owner, 2026-09-29).
What a step is comes from saved data alone (external review, 2026-10-02: the
flow had named calls and never said what they were for): each run of steps
read in one part stands under that part's box once, its title alone, its
description on hover, a click reading it; a method says its type's own atlas
line once for a run of the type's methods, "{Type} — {line}" in the model's
style, its first sentence whole and the rest on a click, " …" saying there
is more (owner via the coordinator, 2026-10-02: two clamped lines had cut
casdoor's KeycloakSyncerProvider and etcd's Etcd mid-sentence; such lines
run some 200 characters) (the step's `Explanation` stays its own
line only, READING); a step whose declaration handles saved inputs names
each by its kind's words and its name as written ("handles the command
trade"), the name reading that input and lighting its tile, several of one
kind folding under the kind's plural words; where the walk decided a split,
the calls the path did not follow fold under "also calls:" (orientation
`Passed`), never "one of N". The flow closes with "The path stops here. At
each step it follows one call the step before may make; open a step for all
of its calls."
Each step's twist, opening its calls in place, is always shown. A program no
model flow passes reads forward from its entry in a start list: each
entrypoint with its part and key, its name linking to all of its code, then
that part's outgoing connections, the entrypoint's own calls first and then
the part's others, each in written order, the first few, each line once;
several call sites of one caller calling one callee are one step, the next
distinct connection taking the freed place, each on a line of its own. With
one entry its calls stand open under it in written order and the connections'
line goes (owner, 2026-09-30: redis-cli's had run on). A component without a
flow or start list shows no flow section.

Closing the Main flow, "Also runs on its own:" lists what the program runs
without a request arriving (owner, 2026-09-29), from saved data only: its
scheduled inputs, then its continuous ones (the ways-in order), each in saved
order and each callable once, save one the Main flow names. Each names its
registering function alone ("{callable} — {registrar} registers it"), never a
run of calls from the entries (litestream's Replica.monitor had read "… →
ReplicateCommand.Run → Store.Close → … → Replica.Start registers it", the
shortest exact route, through its shutdown). What runs it is each function
calling it through a value (a dispatch's alternatives, a call through a
function value resolved to it alone, an open call whose stores name it),
after the run of exact calls from the entries to that function, from the
last declaration the Main flow already shows on that run ("{runner} → …
runs it"), a runner once read named alone after: a page-side reading of
saved facts, which a run without a model has too. A callable no saved registration hands over
is its name alone. Nothing is looked for beyond the saved kinds and
facts, so a program without such inputs lists none. Pointing at a line lights
its input's tile. A scheduled or continuous input's own reading opens at that
line, its line link going with it.

An input's reading is drawn in the Inputs blue of its tile and collection,
never core's rose: its heading's bar and kind and its links. A chosen input's
reading opens at its path (owner, 2026-09-27). It names its handler ("handled
by {handler}"), a method with its type and an input one case of its
handler declares with that case as written ("Main.Run (case "replicate")";
never a bare "Run"), reading that declaration in its part when the part
lists it. An input whose handler is not established says so ("handler not
established") and where it is declared: "declared in" the part its call is
written in, with the call's source link; some code acts on it and the facts
do not yet say which. Its options, the inputs nested under it as a
subcommand's flags are (READING; GroupsIndex `Reach.Options`), are no tiles:
they are listed in its reading, each at its source (page data
`inputPath.options`), and an object's members nested so are no longer listed
as what it "declares". A registration the model did not explain has no line:
its given text only restates the fact, and the reading and Find name its
handler. The reading projects GroupsIndex's saved reach, ways and dispatch
sites; the page walks no code.
It opens at how a request reaches the input: "How a request reaches {input}:",
then the first way (GroupsIndex's outer inputs of the dispatch site running
its handler, requests first, each by the shortest run from the callable it
hands over, else by its own calls) as one chain in call order, a line per
part, the handler last in its part; the callable handed over says so on its
hover ("passed as a callback by {a}, which {b} calls"). "Other
ways in:" follows on one folded line, each way named by the part of the
declaration where it leaves the first ("from {part}"; another dispatch
site by its own part), with "+N" for the inputs whose own code runs the site;
opened, each way is its chain. Then comes what its handler does, its flow
under "Its calls:" (external review, 2026-10-02: "What it does:" had promised
a meaning the calls do not say), a
single call opening by itself while its calls go one at a time; of an input a
case of its handler's comparison declares, what the case's lines call alone
(the reading's `cases`, as GroupsIndex's reach starts from those lines:
litestream's replicate is `NewReplicateCommand` and the command's
`ParseFlags`, `Run` and `Close`, never Main.Run's other cases); last, one
quiet line names who sends it ("{program} sends {input}."), a model's match.
Without a known way, the reading shows one box per dispatch site whose
alternatives hold the handler, the first open: "Dispatched from {site} ·
one of {n} handlers", with the dispatch fact and "How a request for {input}
gets to {site} is not established." (how a request
arrives at the site from outside). No route to the site is drawn. A
dispatched input's reading does not list the inputs whose own code reaches
the site, lest a reader take them for its route; they belong to the site's
own reading, with the declaration: its inputs dispatched there by name, each
with its handler ("Its handlers by input", with a filter over both), in the
column's own scroll, folded under its count past twelve, never a scroller of
its own (owner, 2026-09-29), each input's name reading the input; then
"{site} is reached from these inputs:", each input a button to its reading
with its calls to the site, then "Which of these, if any, leads to an input
dispatched here is not established.", or "No input reaches {site} by
calls". Every count says what it counts, from GroupsIndex's sites in the page
data: a site's "one of N" counts its alternatives as handlers or, when some
handle no input, as functions with how many are handlers; its inputs count
as inputs ("N inputs dispatched here"), and the site's reading names each
handler several share ("{handler} handles {n} of these inputs: {a}, {b}"),
shown on hover on the dispatched input's line. An input whose own code
reaches a site says so as its handler's own call back into it ("{input}'s
handler itself calls {site}, where {n} inputs are dispatched:"), with those
calls. An input a running declaration of another input's reach hands over
reads "Registered by" with those inputs; the other reads "Registers".
Then comes the handler's flow as its spine (GroupsIndex `Reach.Spine`,
critic 2026-09-30): each step whose work is one call into the next, from the
handler on, with its part; a class is one step with the methods of it the
step before calls (a constructor call and its class: freqtrade's `Worker`
with `__init__`, `run`, `exit`), a callable handed over is a registration
(under "Registers"), no step, and an outside call is no work. Where the work
splits, "then into:" names each branch with the members of it the step
calls (`FreqtradeBot` with `process`, `startup`), the helpers among them on
one line ("helpers: addReply, addReplyBulk"). A class step or branch reads
its own atlas line under it, the line its part's explained declarations
carry (page data `explained`), its first sentence until clicked: what a `Worker`
or a `FreqtradeBot` is (external review, 2026-10-02). Then the parts the input
enters fold under one line naming every one ("Parts on this path: String
commands, Keyspace, …"), each part, nearest the handler first, with its
name (a link to its reading when the map draws it), the handler under its
part ("handled by {handler}"), every call entering it from a part reached
earlier (the first five, the rest under "+N"), and "{n} more calls into
this part come from other code on this path" for calls into it from parts
reached no earlier, none of them the handler's. Nothing is hidden behind a
count: the fold chooses no route and drops no call. A call is its two declarations' names, a
read or possible call marked as elsewhere; a name in a drawn part reads that
declaration there, as a click on its tile does; a name in a part the map does
not draw is only named. There are no "Shared by" or "through" words. The list
of the parts on the path is left for an input without one.
A part read while an input is pinned says "Outside this input path" in its
heading when neither it nor a part inside it ends one of the path's arrows,
decided from the reading's own state, never from the pointer. An up-chevron
in the reading card's own header closes details, keeping the camera and any
pinned input path; leaving the path is a separate action. A declaration or
part chosen from the input's own reading (its flow, its path, its main flow)
is read inside the path: "Input: …" and "Leave input path" stay (owner via
the coordinator, 2026-10-02: freqtrade's trade dropped its path when its flow's
FreqtradeBot was chosen).

An area's reading starts under its description with its parts (below); a
key is read in its part without moving the camera. An area's or component's
reading then lists its Connections as its arrow ends group them, instead of
neighbours by name: one line per frame or participant at the other end and
direction, incoming first, each opening to the arrow card's rows; the column
prints none of the card's counts and names one to a line. An end written only
in its program's tests (every declaration of the part, or of each part of the
area, in ProgramTarget `TestSources`) stands last, under one closed "Tests"
(owner, 2026-09-30). A click on an arrow end opens its connection alone. A line
from inputs counts the handlers they are implemented in, with that unit ("←
Inputs {n} handlers"; its card "{n} handlers, into {a} of {b}"); inputs
sharing a handler stand in its one row, each named ("{a}, {b} → {handler}")
and each name reading that input (owner, 2026-09-29); no input is dropped.
Inputs taken in where their handler is not established count as inputs and
share one row per place, named in order ("{a}, {b} declared in {function}").
In those rows a name reads its declaration in its part, as a click on its tile
does; no row prints a place (owner, 2026-09-29). A row with no call of its own
that would only repeat its heading is not repeated, and ends of one name are
one heading in column and card alike. A row wraps with its names whole. The
page data names a call's two end declarations (`caller`, `callee`, keyed as
the reading keys a declaration) apart from where the call is written and
lands. A name whose part lists no such declaration is only named. The canvas's
own card keeps its links. An input collection is named Inputs there, as on its
canvas heading. A relation row is said through one closed vocabulary of the
report's UI messages, chosen by its kind and filled with the two declarations'
names ("{caller} passes {callee} as a callback"), never the stored kind
("passes_callback"); a C program's import is said as an include, and a joint
between two programs names the declarations at both ends ("{caller} connects
to {callee}", not "integrates with"). An arrow's card reads each call as
caller, relation and callee, linking both names: the phrase's words when they
stand between the names, the relation's kind when the phrase wraps the callee
("{caller} passes callback {callee}"). A row the model wrote in its own words
keeps them. Rows of one caller and one relation kind in one evidence list fold
into one line with their count and callees, each row inside with its sources.
A list with folds has one "Open all" control that opens and closes them
together. A list longer than forty stands by groups, each closed under its
name (reviewer, 2026-09-30: lists of up to 336 rows had stood under one
heading): inputs reaching a part by program, kind and the part where
each takes effect; a declaration's other users and a catalogue's inputs by part; a
part's declarations by type and kind; its callers and callees by file, its
variables by type; a path's calls by caller and many parts each closed; a
program's files by folder. Every closed section shows its ▶ (▼ open), the same wherever it
stands (reviewer, 2026-09-30: "Called from" and "Calls into" had read as
empty headings between two rules); a flow's call and a connection's line
open by their own twist and link. What a reader opens in the column (a fold, Open all, any disclosure)
comes into view: the column scrolls by what it overflows at its foot, no
further than bringing the opened summary to its top; closing scrolls nothing,
and a reading restored with its evidence open keeps its place. The column
stands beside the map's controls and key as well as its canvas and takes their
height, the canvas keeping its own; only the canvas's workspace sets that
height, so when the canvas cannot be drawn the reading keeps its own scrolling
box. The column has no "More details" step, "To explanation" or "To code"
action. Unknown destinations and different identities never merge by title.
The reading says when no connection to another part exists in this report;
that absence neither classifies the declaration as unused nor invents an edge,
and internal relations and the complete source inventory stay available. A new
selection, a declaration newly chosen in the part being read included (from
its tile or the column), reads from its top (owner, 2026-09-29). Returning to
a part (Back, or the same part shown again) restores its expanded evidence,
reading scroll and canvas camera; a reading reached anew by a click opens in
its default state, whatever "Expand all" or a fold opened before (owner,
2026-09-29). Every reading ends with one small line of the home's text pages
("How do I run it?", the glossary, what is missing, …), the links the home's
reading lists, in the column's muted meta text (owner, 2026-09-29). Saved
core/entry/dependency lanes stay named in map and reading; an import is not
promoted into an external communication.

## Scene model

The canvas is rewritten as four pure stages (the accepted plan of
2026-10-01), `internal/report/web/scene-canvas.jsx`. It is the page's
one canvas since every level held its invariants on every report
(21bb81f2: twelve runs, 151 levels; the beets whole map's lane room
excepted as data); the old canvas, its grid, wrapping, size floors and
post-layout edits are deleted.

- `model.mjs` `buildModel(page)` builds the display graph from the page's
  data and the saved scene: programs, areas, parts and the note keep the page's containment; a
  program's inputs stand in its Inputs frame by kind, never by part; an
  outside system is one chip standing for its call records. An Outside
  frame holds, in order, the systems two or more parts call (the most
  called first), one bucket per part with two or more systems of its own,
  then the rest (B′; casdoor: 94 systems, 33 items). Every box carries
  markers for what stands inside it: the kinds of the inputs taking effect
  in it (left) and of the outside systems it calls (right), at most three a
  side, the least held kinds folded into the third. An input takes effect
  in its handler's part, else where its code takes it in ("declared
  here"), else in its program. Inside an entered part a handled input's
  marker stands on its handler's tile and an outside call's on its caller's;
  an input with no known handler stays on the part's edge.
- The facts the canvas draws are saved with the report and read as saved
  (owner, 2026-10-01: "у html должна быть простая задача — вот данные,
  показываю"): `buildModel` derives none; where the page's relations say
  otherwise it follows the saved scene, and with none saved it draws no
  marker, caller or pair of programs (`model.test.mjs`, "buildModel draws
  the facts saved with the report and derives none of its own"). When the report is assembled
  (`report.generate`), `scene.go` reads them once off the system map the
  page draws, its declarations keyed by their places whatever links a later
  render gives the page, and saves them as report.json's `scene`; the page
  carries it as saved in `<script type="application/json" id="rm-scene">`
  (`programPairs` there, `program_pairs` in report.json), a render deriving
  nothing (`TestThePageShowsTheSavedSceneWhateverItsLinks`; on Redis
  the browser derivation it replaced agreed on all 242 inputs). Layout, camera, hover
  and emphasis stay in the browser. `buildModel` reads the page's canvas
  input `{items, relations, areas}` and `scene`, the `#rm-scene` the scene
  canvas reads: of an item `id`, `title`, `branch` (component, area,
  inputs, outside, communication), `category`, `activation` (an input is a
  record with one), `lane`, `summary`, `role`, `children`,
  `componentOwner`, `unestablished`, `trace`, `symbols` (their `path` and
  `line` the place a handler or a caller names), `symbolCalls` (and what
  `cards.mjs` measures); of a relation `from`, `to`,
  `displayFrom`/`displayTo`, `possible`, `init`, and for the arrow's card
  and the column's connections `scope`, `label`, `operations` and
  `calls`; `areas` `{id, nodes}`. IDs are the page's: programs
  `system-component-tN`, parts `n-tN-gM`, areas `tN-area-kM`, inputs
  `tN-oM`, Inputs frames `system-inputs-tN`, outside systems
  `system-tN-out-bM-destination` (a call record `system-tN-out-bM` stands
  for its system), Outside frames `system-outside-…`. The saved facts:
  - `inputs[inputID]`: `{kind, program, parts: [partID], handled, handler}`;
    `kind` one of request, command, setting, scheduled, continuous,
    interaction, consumer, extension, entry (an activation `background`
    is continuous, `queue_consumer` consumer); `parts` the handler's part
    when `handled`, else the parts taking it in (declared in, looked up
    in), empty when none, the input then taking effect in `program`;
    `handler` the handler declaration's place `{path, line}` when handled
    and known, as a part's `symbols` entries carry `path` and `line`.
  - `systems[systemID]`: `{kind, parts: [partID], programs: [programID]}`;
    `kind` one of database, request, sdk, queue, started, other (the kind
    most of its calls' facts give); `parts` the parts whose calls reach it,
    `programs` the programs calling it. Equal destination names across
    programs are one system where the page already shares its Outside
    frame.
  - `calls[partID]`: `[{system: systemID, caller: {path, line}}]`, the
    place of the declaration making each outside call, one entry per
    system and caller; no `caller` when an arrow's call names none.
  - `programPairs`: `[{from: programID, to: programID, runtime, relations}]`,
    one per directed pair of programs: `runtime` when any relation between
    them is an operation (scope other than structure), a part reaching
    another program's input counting for that program, and a call through
    an outside system served by another program's input (connects_to)
    counting as its caller's runtime pair to the served program;
    `relations` the relation indexes it stands for.
  - `reaching[nodeID]`: `[inputID]` in the page's order, for every part
    and outside system some input reaches: a relation listing the input
    among its `operations` starts or ends at the part, or at the system or
    one of its call records. No entry when no input reaches it. It is the
    column's "Inputs reaching" of a part or a system (format 96), which had
    run the projection's selection over every input on each card open.
  An input the scene does not name is of a kind not established and takes
  effect nowhere; an edge between two programs no saved pair names is not
  drawn on the whole map. From these `buildModel` keeps the grouping it draws: inputs by kind
  (groups `{inputs ID}#{kind}`), an Outside frame's items in order
  (systems two or more parts call, the most called first; buckets
  `{frame}~{part}`; the rest), a box's markers `markersOf(box)` `{in,
  out}` (`[{kind, members, systems, handled, folded}]`, at most three a
  side), markers on an entered part's declarations `memberMarkersOf(part)`,
  the whole map's arrows `homePairs` (`uses` when neither direction is
  runtime) and a frame's connections for the reading column
  `frameGroups(id)`. Levels, scene and overlay read only the model.
- `levels.mjs` `layoutLevels(model, canvas)` lays every level out once per
  canvas size with ELK, bottom-up, each level its own graph with final
  routes: an area its parts (layered down, arrows on the boxes' tops and
  bottoms, the left and right edges left to the markers); a program its
  closed areas (cards of their open drawing's proportion, holding that
  drawing when entered) and loose parts, of both directions the one fitting
  the canvas larger, a link to another program ending on a port of the
  program's border (one port a way per box, naming its programs on hover);
  the whole map its programs (cards of their name and role), Inputs frames (their kinds' marks) and
  Outside frames, of the arrangements tried (across or down, the Outside
  packed to four proportions) the one fitting the canvas best among those
  whose lanes stand at least four fifths of ELK's spacing apart, the one
  whose lanes stand widest where none does (`narrowestLane`; harness
  table, 829626d6: etcd's server laid out across had its arrows out of
  its right side 5 pixels apart; down they stand 9.5, beets' went from 7.1
  to 11.7). Every program and every outside system in sight is
  named at rest on the whole map (owner, 2026-10-02): a program's name at
  eleven pixels or more, an outside system's (a chip's, a bucket's) at
  nine and a half, a secondary word as a description and an input's name.
  A program's card is drawn at the text size its name reads at at the
  camera showing the whole map, an Outside frame's chips and buckets at
  the size theirs do (its title at the programs'), the map's lanes
  widening with the programs, an Inputs card (its kinds are icons) and a
  loose box at their own. The sizes grow while the names come near
  reading at the pace they do; where they cannot (the map grows as its
  names do: etcd, beets, casdoor, the 150-system graph), the sizes kept
  are those that brought them nearest, and the camera at rest frames the
  busiest program with the most other programs, then the most other
  names, a canvas holds, as close as they read; "Show whole map" shows all
  of it, the names that do not read faded and named on pointing. A closed
  program or Inputs box the camera at rest would cut at the canvas's edge
  is brought in whole, or taken wholly out of sight where that would push
  a framed name out, and an Outside frame likewise at its title's corner
  (`levels.mjs` `keepTitles`; owner via the coordinator, 2026-10-02:
  casdoor's second Inputs box had read "ts", etcd's tools/etcd-dump-db
  stood past the right edge of the harness's canvas). A frame whose body is in sight keeps its title in sight: its left
  edge out of the canvas, the title moves in along its own band, never out
  of the frame; its top out, the overlay names it at the canvas's top at
  its title's size, the frames holding it a row above where their names
  would overlap, and a frame whose own title such a name would cover is
  named below it (etcd's "Inputs" had read "puts", othello's program
  "hello", casdoor's Outside lost its title above the canvas). A closed
  card, program or part-group cut at the left or top edge moves its words
  in, under those names, never out of its own inside (etcd's "gRPC proxy",
  casdoor's "Email providers" beside an opened part-group).
  The camera framing a box (entering a level, a reading's declaration or
  arrow end, "Show input") stops where that level's words read at 1.35
  times their size, and no word on the canvas, nor a chip's or a bucket's
  mark, is drawn larger than 1.6 times its own size: the neighbours of a
  small box read inside a large level keep their names at that size (owner
  via the coordinator, 2026-10-02: casdoor's Custom Logout Endpoint had
  filled the canvas in 90-pixel letters, the part-groups beside it in
  50). The
  world is drawn a power of two larger, keeping every level's entry camera
  at most four screen pixels to a world pixel: the browser sizes a box in
  steps of 1/64 of a pixel, and at etcd's deepest levels arrows' ends had
  stood 2px off their boxes. A card's title, role and description each stand whole
  or are left out, never cut mid-text (a program's purpose in two lines at
  most, a part's or an area's description in four; the rest reads in the
  column), its size and its drawing by the same rule. No wrapping, grid, size floor or edit after layout.
  Arrowless collections (chips, inputs' names) are packed in rows. A graph
  ELK throws on is laid out again with ELK's own placement.
  A part's card holds its words: the card cards.mjs measures where its
  description stands whole, else as tall as its title, a tile's room under
  it when it holds declarations. A chip's and a bucket's box is as tall as
  its name's lines; a name is never ellipsized (owner, 2026-10-02).
  A closed part-group of the Outside frame (B′'s bucket: one of our parts
  and the outside systems only it calls) is drawn as that part's card, the
  parts' grey line and dark words, in front of a stack of outside chips,
  their brown line, the kinds of its systems marked under its name; opened,
  its frame holds those systems' chips under the part's name in the same
  dark words, never an outside system's brown (external review, item 4,
  2026-10-02, on a skeptic's choice among three looks). The reading
  column's Outside lists its destinations alone. A click on a closed
  part-group reads "The outside systems only this part calls": the part
  as its card, then those systems as the Outside reading lists them, each
  with where it is made and called from (owner via the coordinator,
  2026-10-02: the click had read the part's own files).
  A declaration the reading names out of sight (`scene.mjs` `memberView`,
  owner, 2026-09-29) is shown with its part entered across the canvas at
  the zoom its tiles read at, framed whole when it fits, else with the
  tile centred down it, never deeper than the part's title fits; a part
  too dense for that is shown closed in its level at the zoom a part reads
  at, its card centred holding the declaration alone as its tile under its
  title. One in sight, or a restored visit, keeps the camera.
- The whole map at rest (owner, 2026-10-01, on the skeptic's verdict):
  each program's Inputs into it, a program into each Outside frame it
  calls, and one arrow per two programs an operation joins (a program
  reaching another's input reaches that program; a call through an outside
  system the other program serves is the caller's arrow to it). Two
  programs joined only by code use are drawn while one of them is pointed
  at or chosen. Possible arrows stay dashed and are never hidden for it.
  Inside a program everything is drawn.
- `scene.mjs` `sceneAt(model, geometry, level, choice)` gives a level's
  boxes, arrows (one polyline each, its own ends, a head at each end an
  edge goes into), markers and ports, each with a drawing band, and the
  level's one text scale; `emphasisOf` changes only classes and the
  arrows' order (dark over grey); one `hitTest` answers hover and click,
  a box's outer two pixels belonging to an arrow meeting it there (its
  head, all a close camera may show of it).
  The level is the chain of entered boxes (program, area, part; an Inputs
  frame; a bucket): entered by an action (the magnifier, a reading) or by
  a zoom with hysteresis, its text reading at 12.75px to enter and under
  10.5px to leave, one pinch crossing one level boundary; a pan never
  changes it. Entered, a program draws nothing beyond itself; a part is
  entered at the zoom its declarations read at.
- `overlay.mjs` places markers, ports and magnifiers at one screen size on
  every camera tick: a marker touches its box's edge from outside, the
  stack from the top, stepping past every arrow running through its
  column (one meeting that edge, one bending beside the box), shown once
  the box's title reads and the stack fits beside it. A box's words fade
  out below their readable size (titles about 11px, an input's name and a
  description and a chip's or a bucket's name 9.5px, one rule for all
  words, each word by its own drawn size: its font size times the zoom
  times its level's text scale) and the pointer on such a box names it, as the keyboard's focus
  names a chip, in one line. `store.mjs` holds the level, the pointer and the
  choice (useSyncExternalStore); the camera has its own store.
- The node tests `model.test.mjs`, `scene.test.mjs` and `store.test.mjs`
  check every invariant at every level of the seeded synthetic graphs and
  of the reports named by `REPOMAP_SCENE_PAGES` (page JSON captured by
  `node scene-pages.mjs capture REPORT.html --out DIR`). Lanes keep at least
  7.5 screen pixels between them at a level's entry zoom: the smallest the
  owner-approved Step 1 drawing kept (daf1231e, redis's Core server
  infrastructure, measured on that commit's layout code). The page exposes
  `map.sceneState()` and `map.sceneEnter(id)` and the DOM contract of
  `visual/invariants.mjs`.

## Reader context

The complete component/input catalogue stays available beside the same map,
with all original purposes, manifest/entrypoint anchors, and incoming and
outgoing observations. Catalogue rows are all visible within their saved
type or destination; only an individual record's evidence needs disclosure.
Nothing is matched by name, and the canvas is not expanded or changed.
The Parts count uses the same local leaf groups across all lanes, excluding
operations, frames and foreign nodes; its link focuses the component on the
common map. The separate core-lane code reference is labelled Core and
cannot imply an empty component map.

With nothing selected, the reading column shows the repository summary,
marked as the model's by its style alone, and entrances to the complete
saved question menu, run material, terminology, missing observations and
author claims: ordinary rendered content, not another model summary. Those
entrances are one line at the home reading's foot, after its programs, as
every other reading ends with them (owner, 2026-09-30: at its top they had
come before anything to read). The
components, their areas and inputs are on the canvas and are not listed
again (owner, 2026-09-28); the targets the run could not read are named
there with why: the closed reason, then the failure in its own words as the
outcome saved it (litestream's `src`: "no build line compiles
src/litestream-vfs.c; parsed with clang's defaults …"), line breaks kept and
never translated. On the canvas they are one note, "Not analysed", naming
them, sized with its words as a summary among the programs' and taking
their connections; read, the note shows each target's card from Component
details (`targets-not-read`) with the same words. The introductory sentence
has no model badge or source popover;
its saved citations and model attribution live in Repository summary
sources, reachable from the home reading. The summary and useful links
remain, with no "Understand this repository" or visible "Starting points"
label. Selecting an item replaces the home reading; closing details returns
to its top.

The header, sticky toolbar, canvas and reading sections share the page's
left gutter; the toolbar spans the available width so scrolling content
cannot peek around it. Question reading keeps its line-length limit without
shifting into a centred column on wide windows. The reviewed repository name
links to the reviewed directory and revision when nested in a repository; it
sits on the left beside an icon-only Home action, and the repomap name and
GitHub mark link to repomap on the right. The same header holds Questions,
the component list and one visible Find field. All is the list's default and
runs the whole-map action, clearing selection, input context and the visible
Find query after recording the new visit. The main toolbar holds search and
camera controls; search keeps its full inventory, with no type/style
switches. Show whole map above the canvas moves only the camera; the reading,
its emphasis and any input path stay. The "−" beside it steps out one level,
as a zoom mark steps in one, also moving only the camera: from a part's tiles
to the frame holding it, from an open area to its component, from an open
component to the whole map. The camera takes the frame as entering it would,
no closer than the zoom at which the level it leaves closes; at the whole map
it zooms out one step. "+" zooms in one step. A reading revealed on the map
(a search result, a declaration named in the column, a component chosen)
scrolls the page only to bring a map not wholly in sight under the toolbar;
one already in sight stays put (freqtrade, 2026-10-02: a declaration chosen
from an input's flow moved the page 16 pixels under the pointer).

The toolbar keeps the current question and the exact component/area/part
path, the pinned input before it and the declaration read after it
("{input} · {component} / {area} / {part} · {declaration}"). Each segment
links up to its level, reading and camera together: a component or area is
read and framed as entering it frames it, the part is read and entered without
the declaration, the declaration is read in its part with its tile centred,
the input is entered as its path. With nothing read it names the System map.
Browser Back/Forward, a source-details side trip and reload preserve the
question, selected operation, exact code selection, map search/filter and
viewport; component choices and Back restore their selection and camera. The
completed initial overview camera is saved even with no selection or hash, and
restoring an empty selection clears the previous inspector. Imperative camera
changes are saved after the latest movement settles. A restored complete visit
is not followed by another hash-driven selection; an explicit overview/detail
transition stays a separate visit even when the selected item's URL is
unchanged. A named Back to map returns from full source details, the glossary
or any section below to the page as it stood at the last click made while the
map was read, the canvas where it was on screen and its camera unchanged
(owner, 2026-09-29); Back to question returns to the original answer. Old
component and map links resolve to the same common canvas. Answers, component
reference and repository material open below the mounted canvas, each with a
direct return to it. Component sidebar links expose the flow, configuration,
data, core, dependencies, dynamic execution, coverage and TODO sections when
present. A link to a section's heading opens the sections holding it and the
list the heading heads. No redundant single-component entrance is generated
beside the common canvas.

Answer provenance, checks, supporting readings and question origins share
one collapsed apparatus; source IDs and excerpts survive. Terms get one
underline per answer; source-unavailable labels stay inspectable. An
underline finds a glossary name by the shared
[term lookup](TERMINOLOGY.md#term-lookup) (owner, 2026-09-26). Inline terms
link directly to their map memberships and select that declaration by its
source, with no catalogue hop. Header revision noise, redundant answer
actions and root Path: are removed; run information stays linked from the
footer. Repeated `model` badges are suppressed; source and model-response
inspection stay available.

The page exposes the complete saved question menu, one open answer, its
position in the menu, a next question and an explicit return to all
questions. The menu is one vertical list of topic headings with every
question visible under its saved topics; there is no parallel all-questions
grid or nested topic disclosure, and questions absent from the accepted topic
plan stay in a plain fallback group. The current question stays visible
while scrolling or on a map side trip. Component reading entrances use only
the saved answer's exact map links. There is no Learn/Work switch; a `mode`
query parameter in an old link is ignored.
Repository search lives in the header; compact results show short plain-text
excerpts without source or model popovers, full answers and provenance
staying at the result's destination. Exact code-name matches precede matches
in answer prose. A result says what it is: an outside call or its
destination is External communication (with its own filter), never a Part;
an input collection is Inputs; a component is listed once. A declaration is
one result by its file and line, however many programs compile it: it shows
the declaration as its tile does (a type with its fields), its file and line,
and one "In program / part →" link per program holding it, reading it there;
a program leaving it off its map links to the list saying so (Not on the map,
or Not reachable from the entrypoints). A declaration no map holds opens at
its row in the report by its title. Choosing a component in the header closes
the results and makes it the results' component. Back to search returns to
the same list where the reader left it.
Original source links and full target evidence keep their order and stay
reachable.

## External communication and data

A program is titled by its directory unless the directory does not say it
(`page_program_title.go`, 2026-09-30): a script (ProgramTarget
`ScriptFile`: one source file, no name from its build) by its file
(`build_helpers/create_command_partials.py`, `etc/s3_mock.py`, not the dotted
module or "etc"), and a program its build names once by that name where its
directory ends otherwise (`freqtrade-client`, not
`ft_client/freqtrade_client`); `cmd/litestream` and `freqtrade` keep their
directories, and a title two programs would share falls back to the
directory rule. The one part of a script, which takes the program's name, reads
the same title. Every row of the home's table of programs does something,
in the link style (owner, 2026-09-30: "ничего не кликабельное"): a
program's name reads it, its entry reads that function in its part, an
input kind reads the program's inputs at that kind, and a connection reads
that arrow, or the outside frame it names. The table says what each is
"Built from", a list longer than a short section folded by its top
folders, names only:
a C program's link units; a script's file and what its code imports,
followed through each imported file's imports (ProgramIndex
`ImportedFiles`), tests left out; a Python program whose build declares
packages (ProgramTarget `libraries`), its entry files and what their code
imports with every file of those packages; otherwise the files its index
declares things in (a Python program declaring no package may load its
code by strings, as a Django project does). A Python program shares its
project's index with the project's other programs, whose files are the
whole project's: freqtrade's build_helpers scripts and `freqtrade` itself
had each read 373 files.

A component chosen on the map is read without moving the camera, as every
first click is (above). Its reading names it without the kind its label adds
(heading "Component · {language} {kind}"; the kind stays in the name only
when another component would read the same, as on the canvas's cards), then
gives at most five sections, counting nothing and naming one thing to a
line (owner, after the critic, 2026-09-29: nine sections had run to ten
screens with up to fifty-three standalone digits): its role and purpose, the
model's by their style, its entrypoints, each one link by name reading the
program's seed in its part when it is that seed, and the kinds of its inputs
in words ("Incoming requests", "Settings", …), each lighting its tiles on the
canvas while pointed at (or, while the collection is closed, its row of that
kind), dimming nothing, and when chosen reading the collection at that
kind's section, as a kind chosen in the collection's closed frame does
(Background work at the first of its scheduled and continuous sections); its
Main flow, closed by what it runs on its own; the files its program reaches
(below); its Connections, their counts left out; and one line of links, its
whole page ("Component details") first. Its areas and parts are the
canvas's. What its entrypoints do not reach, its TODOs and its analysis
coverage stand on the "What is missing" page, under the program's name
(`component-gaps`), and nowhere else. An area's reading lists its parts,
each in its box with its description whole, wrapped between words, and its key declarations
alone, bold, one to a line; the rest is the part's own reading.

An Inputs collection is read by its catalogues: its component first, in its
box, linking to its reading; then each kind once, under its heading
(owner, 2026-09-30: litestream's had read "Incoming requests" once per
declaring function), each catalogue of it under a quiet line for where its
inputs are listed or declared, where they are looked up and what else the
declaring code uses, the model's words that some are matched by name to
another program's inputs, and its inputs one to a line, each reading its
input, a name that is a sentence (a query parameter's description) in the
reading's own type, not code; then the inputs no catalogue holds. Within
a kind, what runs a handler stands before what only declares a value,
catalogued or not (reviewer, 2026-10-02: freqtrade's trade, backtesting
and webserver had stood after every option of AVAILABLE_CLI_OPTIONS, a
catalogue none of whose entries runs code); each option stays in its
command's reading and in its catalogue. The
kinds stand in the canvas's order with its marks (page_reading.go
inputKindOrder, scene.go sceneInputKinds; owner, 2026-10-01). Within a
kind, the inputs stand by the part where each takes effect, as saved
(`#rm-scene` `inputs[id].parts`, one part; owner, 2026-09-28: inputs
answer "where it takes effect"), the parts by title, its box the heading,
each catalogue's lines once inside it, the inputs taking effect in several
parts or in none after them (reviewer, 2026-09-30: Redis's ninety requests
had read A to Z; freqtrade's had been headed by a catalogue's module); a
kind of more than twelve inputs in several parts folds each part to its
box. The column reads where an input takes effect, its handler's part and
the inputs reaching a part or system (`reaching`) from the saved scene and
derives none of them; a report saving no `reaching` shows no "Inputs
reaching" list. A
catalogue whose every input has no established handler says "Where these
take effect is not established." once, under its own lines, never for the
kind (owner, 2026-09-30: Redis's had followed serverCron, which is
established). A kind chosen on the canvas or the home reads the collection
headed "Inputs · {kind}", its section brought into sight and marked, as a
connection opened from the home is: a one-second pale pulse, then a steady
link-coloured bar at its left (only the bar under reduced motion). Each
input's record, with its registration line as written, is its own reading;
a registration line over 160 characters folds the first braced body longer
than 80 into "{…}", the whole line behind its source link (etcd's Campaign
registration, 1,144 characters with its inline handler's body, had shown
that body's tail at the reading's top).

The component's "Entrypoints" link lands on the program's entry, from
GroupsIndex's entries in the page data: the part holding every seed, the seed
read there when it is one; when a seed stands in no part, or the seeds in two
parts, the component's reading, opened at its entry line when it has one. A
library's entries are its exports (PROGRAM_INDEX `target.exports`, the
`entrypoint` facts keyed `export`): its Entry list names them as callables
(`lua_absindex()`), and they are no way to run it, so the run recipe a run
without a model shows lists none of them; with nothing else to run, it says
that no manifest row or launch entrypoint was found and that a library's
exports are its API, not ways to run it. Each entry names the part holding
it (GroupsIndex's Entries) and reads its function there; a list over twelve
folds by those parts, each closed under its box and its count (liblua.a's
156: Core API · 83, Auxiliary library · 34, Standard libraries · 24,
Debugging · 8, Runtime and calls · 7), the entries no part holds after them.
Its full-reference section keeps the main flow, configuration, its parts (each
reading the part on the canvas), dependencies, coverage and TODO lists.

Its traversal coverage, on the "What is missing" page under the heading "Not
reachable from the entrypoints", lists the files no entrypoint reaches; a
target whose entrypoints reach all of its code reads one plain line,
"Reachability: its entrypoints reach all of its code.", with no such
heading over it (owner via the coordinator, 2026-10-02: Lua's test
libraries had read the heading over its denial); then the parts its
program never runs, one row per file with the file, the part's name and its
declarations there as source chips (GroupsIndex `unreachable`); then, under
"{0} symbols" and by file in line order, the other declarations the
component's adapter proved its program never runs (ProgramIndex
`unreachable`). Their outgoing calls, listener, registrations and settings
are not the component's (READING); this list is where a reader finds them. A
declaration is listed once: a part's row does not repeat among the symbols.
Find lists each declaration of such a row as Code with no "In part" link and
opens its row. Above the parts and symbols the list says, as the files' list
does, that nothing this program runs reaches them and that this does not
establish that the code is unused. Each declaration another program of the
report runs names those programs, "(run by {program})", each linking to its
component, in page order. This is a page join over the adapters' saved
proofs, not a walk: a program runs a declaration when its index holds the
same declaration (`groupindex.DeclarationKey`; never a name alone), does not
mark it `unreachable`, and marks some other callable `unreachable`. An index
marking nothing (a library, a program other code can enter by any name, every
adapter but C) proves nothing and names no program; a program reaching every
one of its callables is not named either. GroupsIndex's reach is the input
handlers', per program. The list reuses the existing heading, off-map row and
"{0} symbols" summary; no badge or map mark is added.

An outbound kind is shown by its protocol-neutral label: `client_request` is
"Request", never "HTTP", and counts and headings of accepted client requests
read "requests sent", whatever the protocol. A request's method is only one
its code states (the call word or a literal, ProgramIndex): a socket connect
or a call whose verb is not written has no method, never GET. Linking a
request to another target's route compares two stated methods; a side
stating none differs from nothing, so a request without a method joins a
route by path alone and the link stays possible. The system map counts such
links as "requests" between components; their sources are "Request link
sources".

The entrance reads accepted outbound communication directly from GroupsIndex,
independently of dependency lanes and key captions. Each observation shows
the other participant's role, purpose, known literal address or explicit
unknown, and its original call/source chain. Dispatch and explicit
remote-client configuration stay distinguishable. Native addresses and code
names stay original; role/purpose use the display bindings. For display,
observations group by destination text (case-insensitive; the native label or
kind when the model named none): one row per destination with the record
count, shared kind, basis and address (or the number of distinct addresses);
its records are compact nested lines (native method and address, else the
callable, else the purpose's first sentence, with the source location), all
visible, each opening its full purpose, address, basis and call/source
chain. The section count and first screen count destination groups; a
destination text is still no proof of one remote system. Dependency/import
groups stay in collapsed code reference and supply no integration count.
Empty observations do not prove that the service contacts nothing.
A started program (`runs_program`, "Runs a program") is one destination only
with the very word its calls wrote, compared as written (another case is
another word). A program no word names ("A program named at run time") or
one the model did not decide ("Program not established") is an unknown about
its one call, no destination (2026-09-30; litestream's `-exec` launch had
drawn a "Program not established" frame listed twice in Connections): it is
no catalogue row, frame, tile or connection, but a record under "What is
missing" ("Programs it starts that the code does not name", its line, words,
label and source), and its call in its function's reading says "(starts a
program the code does not name)". Its map frame is the same outward frame, named by the word, one
tile per symbol; each record's line is the callable followed by every word
its call writes, each in its own code span, in the call's order. A started
program whose word equals the name the repository's build gives one of the
report's programs (ProgramIndex `target.executables`; nothing matched
loosely) is that program: its record reads "Runs
this repository's program" with that component's name, which reads it, and
the call's arrow goes from the launching part into that component, with no
outside tile. A program starting itself runs its own entry: the call's
arrow goes from the launching part into the part holding the program's
seeds (pageSection `EntryGroup`), with no outside tile (2026-09-30:
litestream's MCP server running `litestream` had stood in its Outside
frame as a chip of its own name); a call written in that part draws no
arrow, and with no entry part drawn the tile stays.
A destination one of whose records has an integration connection (a joint
the reading confirmed by protocol and input) into another of the report's
programs is that program (`joinOwnPrograms`, 2026-09-30): every record of
the destination, by its name in that program, reads "Reaches this
repository's program" and its arrow goes into that component, with no
outside tile. redis-cli's "Redis server", its connect to redis-server's
listening socket and the gethostbyname resolving the server's host, had
stood outside beside the arrow into redis-server. A destination reaching
several programs, or none, stays outside. A record whose connection
reaches its own program's input (a request to a route the program serves
itself, READING) is that exchange of the program with itself: its arrow
goes from the calling part into that input, or into the part holding the
address it listens on, with no outside tile, and a request written in that
part draws none (2026-09-30, like a program starting itself); another
program's connection into the input joins no record of this one.

The Data shelf lists source-scoped models/tables, written columns and keys,
queries and original sources. Query-to-table references are reversible, so a
table exposes its referring queries. Equal table names never merge source
scopes; SQL mentions and JOINs prove no schema ownership or foreign keys.

The files a program keeps (READING § Outside systems, GroupsIndex data
records of kind `file`, `page_data_files.go`) are the shelf's "Files" and a
section of the component's reading after its Main flow (owner, 2026-09-29;
`31-reading-column.js` `rmComponentFiles`). No role is given
(skeptic, 2026-09-29): a file is its path as written, else the paths its
field's writes store and "from" the field, or the field in braces when they
store none (`dump.rdb from server.dbfilename`, `appendonly.aof`,
`{server.vm_swap_file}`); the shelf says the paths not established in one
line, the column leaves them out. The
shelf's static row prints no function and no place. The column adds what
else sets a field, each once: the setting whose branch makes the write (as a
setting's writes are read, its name reading its input), else the function
writing what is not established; each file opens to the functions whose
calls reach it by part in their boxes, one to a line, as a field's writers
and readers stand, the part naming most first. No line number is written: a
path links to where it is written, its place on hover, and a name reads its
function. Which function reads and which writes is not established, so one
list names both.

A program's Outside frame is read by its destinations, one to a line, each
reading its own, then "Called from" joined over all their calls; a
destination by that "Called from" of its calls and the calls themselves, one
name to a line under one closed "Its calls", each reading its record (owner,
2026-09-30: freqtrade's Outside had opened every record, 59,592 px). Under
its name a destination says where the value naming what its calls reach
ends, as written, linked to that line, when all its records' readable walks
end at one address or expression (`destinationWritten`): Redis's Primary
reads `server.masterhost`, where it had read its name alone. No
record opens by default and nothing is counted. A call's own reading names a
callable written inline as "anonymous function in {function}" (GroupsIndex
`ObjectFacts.Inline`), in its address sources too, with no place and no
numbering.

An outside call's tile is read with "Called from": where each program
whose record the tile stands for reaches the call from (GroupsIndex
`reached_from`, READING § Outside systems), the tile's own program first,
each part in its box as elsewhere in the column with its callers by name,
another program's part named with its program ("redis-cli: [Command line
client] cliConnect()"), and a path's start inside the call's own part under
that part's box. A caller is its name, once however many places it calls
from; no line number and no code mark is written (owner, 2026-09-29), the
places being kept for the name's hover. A name reads its function in the
column and on the canvas. More than twelve callers fold under their count
(freqtrade's exchange calls are reached from 39 functions). The record's
static row prints no callers. This is source reading; it adds no map edge
and infers no runtime caller. Before it, "Made in" names, the same way, the
function each call is written in, in its part (GroupsIndex
`OutboundCall.SubjectID` and `GroupID`, the part the canvas stands its
destination by), in a destination's and a frame's reading joined over their
calls as "Called from" is (external review, 2026-10-02: casdoor's Custom
Logout Endpoint stood under Core data models on the canvas and read only
"Called from API controllers"; it is made in Core data models'
callProviderLogoutUrl and called from API controllers'
ApiController.Logout).

In the column the tile's record (31-reading-column.js `rmOutboundRecord`)
stands open and prints no place (owner, 2026-09-29): its kind and address,
then what the call is, once, by its outside name ("netdb.h.gethostbyname"),
a link to the line making it with that place on hover, in place of the
intro's printed place; then "Called from" and the names its address passes
through, each a link. The model's note stands once, in the card's intro.
The run from the program's own code ("{program} connects out from … → …")
stands only for a record with no "Called from", which says it by part. A
destination chain whose frontier is the record's own callable, one step at
its line, names the call a second time and is no line on the page or in the
column; two steps of one declaration on one line are one name. A frontier
that names nothing (freqtrade's `getattr(ccxt, name)(config)`, a bare `()`)
is no address step: the chain prints its steps and no address line. A walk
ending at a value its adapter could not read (atlas `DestinationUse.Unread`,
READING) reads "Address not established from code" with its steps, never
the expression: Redis's connect had read "Address passes through (struct
sockaddr*)&sa" (2026-09-30).

An input its code reads under several spellings of one value (GroupsIndex
`Operation.Aliases`) is named by the first; its reading says the others,
each once in source order, in one muted line under its name: litestream's
`storageClass`, "also written storage-class" (2026-09-30).

An input's kind is named in the column by every kind GroupsIndex gives
(31-reading-column.js `rmInputKindTitles`): Incoming requests, Commands,
Settings, User interactions, Scheduled tasks, Background work, Queue
consumers, Extension points; `entry`, a kind the reading did not establish,
reads "Kind not established", never "Inputs" (litestream's vfs had read
"Inputs: Background work · Scheduled tasks · Inputs", 2026-09-30).

An operation or native route exposes Data links only when its accepted path
reaches an exact native model owner or a query's explicitly observed callable
owner; tables link back through those query references. Exact versus
possible call status stays visible; source association does not prove
database execution. A legacy class owner must not imply that every method
uses every query in the class. Callable-owner projection requires exact path,
line and column; explicit extractor ownership is accepted. Native embedded
SQL literals lack that lexical callable authority, even where a language
keeps their literal anchor, so the consuming function is not assigned
locally. An unresolved endpoint-to-method link stays a gap, not a completed
endpoint-to-table flow. No additional semantic stage or graph is introduced.

## English identities and display translation

- Semantic output is canonical English. `--lang ru` (owner, 2026-09-07/08)
  translates the built English frontend structure, with an ordinary UI
  dictionary and a separate LLM cube for generated display prose, and renders
  `report.<repo>.ru.html` (Translation persistence). `render` rejects, never
  adapts, a saved translation whose texts now find another name by the
  [term lookup](TERMINOLOGY.md#term-lookup).
  Source excerpts, names, IDs and topology stay original. Operation names stay
  English in every display language and exact command/path labels verbatim;
  descriptions and UI labels are localized. When an interpreted action's name
  is exactly its native declaration name, the report uses that subject's
  English alias with the native name beside it; a distinct action label is
  never replaced with its function's alias, and literal commands and paths
  stay verbatim even when they match a declaration name. Current ordinary
  acceptance is in [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work).
- Translation requests, their windows, refusals and deadline are in
  [Execution](EXECUTION.md#display-translation).

## Translation persistence

`--lang ru` selects one physical `report.<repo>.ru.html`, named after the
selected checkout's directory, never a Go module path's suffix. Default
English publication keeps `report.html`; canonical `report.json` stays
English. A separately saved display translation keeps its language, original
text catalogue hash and closed text refs; the manifest names that presentation
and the receipt carries it in memory. The server restores the same values when
it re-renders source links. UI translation is an embedded dictionary, not a
model call; `--no-model` still makes zero provider calls and uses only that
dictionary. Initial display languages are `en` and `ru`. The question's eight
fixed scope explanations use the same dictionary at rendering time; their
canonical English route values stay unchanged and never enter the model's
display-translation catalogue. Saved rendering applies the same vocabulary
without analysis or provider access.

## One publication

The current manifest records the repository and publication source. A
successful repository run persists, as applicable:

- repository corpus and repository-guidance authority;
- `reduced-documentation.json`;
- the complete target `program-index.json` and `program-index-set.json`;
- the target-scoped `dependency-catalog.json` artifact;
- the matched `groups-index.json` artifact;
- `program-page-portfolio.json`;
- `target-outcome-portfolio.json`;
- `facts.json`, `claims.json`, `orientation.json`, and `rejected.jsonl`;
- manifest-bound `report.json`;
- the single owner `report.html`.

Multi-target publication keeps each target's ProgramIndex, dependency
catalogue and GroupsIndex. Shared artifacts, the manifest, report JSON and
HTML are published once in the owner run from values already in memory. A
served report has the same complete set of target sections and needs no
sibling report files. Saved report restoration reads the common JSON, the
manifest and the files the JSON names in the run's own target directories:
the owner run directory and the target run directories beside it named by its
`program-page-portfolio.json`.
The common JSON contains the exact selected ProgramIndexes, not a copied
presentation graph. It is compact JSON (format 97: a failed target's
`failure_detail`) and writes each
ProgramIndex in its `program-index.json` encoding. A section byte for byte
equal to a file of those directories (the owner's `program-index.json`,
`facts.json`, `claims.json`, `orientation.json`, `glossary.json`; another
target's `program-index.json` in its own run directory) is not copied: `files`
names it by its path from the owner run directory
(`../<run-id>/program-index.json` for another target) and the SHA-256 of its
bytes, and restoration decodes that file in the section's place, refusing the
report when the file is missing or its bytes changed. A ProgramIndex no such
file holds stays in the JSON. The group-graph field holds only thin
GroupsIndex overlays; native subjects and structural edges are joined from
the ProgramIndexes in memory before rendering, so `report.json` has one
native graph schema and one semantic overlay schema, with no `ProgramView`.
Each overlay saves what analysis derived from its index (GroupsIndex 27,
`Overlay.Derived`: every input's reach and spine, the dispatch sites,
entries, catalogues, launch walk and phases) and from its test-free view
(`Overlay.TestFree`, the overview's view without known testing material,
derived over the whole program set). A rendering applies them and never
runs `Derive` (owner, 2026-10-01: "у html должна быть простая задача — вот
данные, показываю"; `TestARenderingNeverDerivesTheGroupsIndex`); a saved run
of an older format is incompatible and is not rendered.

UI iteration uses `repomap render RUN_DIR --output FILE.html` (owner,
2026-09-08). It restores the current common report and completed display
translations with `ReadRunReceipt`, restores captured remote source links,
and calls the same `RenderHTMLWithOptions` as ordinary publication. No
provider or cache is initialized and no analysis runs. Missing or
incompatible saved data fails without regeneration. The original analysis and
translations stay unchanged; the chosen HTML is atomically replaced only
after successful rendering. Saved rendering rebinds display translation
refs only through a complete bijection of the same catalogue entries (role,
text and protected spans), using the original saved display catalogue, and
validates both catalogues and both bindings. It never translates, drops or
invents an entry and leaves saved files unchanged. Missing or changed prose
still fails.

## Source links and serving

- Repository changes during a run do not fail publication. Do not
  reintroduce a freshness gate or strict-snapshot mode.
- `--no-serve` requires resolvable GitHub or GitLab source links and fails in
  preflight with corrective flag guidance otherwise. A corpus file absent from
  the captured revision or changed locally never blocks HTML publication: its
  path and line stay as plain text with a `No source` hover explanation,
  keeping the code cube, explanation, navigation and every unaffected
  permalink. A report without a remote source link likewise keeps the
  original code, explanation and navigation. The outer run checks path
  availability once; its standalone manifest keeps that list for
  repository-free saved rendering. The only thing a served report adds is the
  manifest-authorized local editor opening of the analyzed working tree; add no
  browser analysis and no browser APIs for workspace reads, investigation,
  symbols, source context or run selection.

When the ordinary run serves, publication renders the served page (session
source IDs, the run's local roots scrubbed) beside `report.html`, each from its
own shallow copy of the same report data; a render never writes the data it
shares, including the openable-path inventory. The server serves that page
instead of rendering again; a run restored from disk, or one whose early
render failed, is rendered by the server through the same `RenderServedPage`.

## Three layers of truth

- Everything the report shows is a deterministic fact, a claim quoted from a
  human-written artifact, or a model hypothesis, and the three are always
  labeled. `facts.json` holds the anchored fact layer: entrypoints,
  registrations (call word, literals, stated verb, callable handed over), SQL
  statements with their tables, environment keys, the places where the
  program runs code it was given, manifest rows, TODO markers, imports, dead
  modules, negatives, dependencies, and the files in languages no adapter
  analyses with their language and lines. A negative says only what it knows:
  missing tests read "No recognized test files found in the inspected paths",
  never "No test files". Beside the negatives, "What is missing" names that
  unanalysed code by language, most lines first, each file with its lines
  ("{file} · {n} lines"), so an unrecognised test suite is not read
  as no tests. Routes, client requests and the portals between targets are
  not facts: they are registrations the reading stage classified, read from
  the GroupsIndex operations and outbound rows and joined on literals in the
  report. `claims.json` holds quotes with their source path, date and age.
  `orientation.json` holds the model's repository summary, roles, run recipe
  and the main flow code walks from the target it names (READING §
  Orientation); every row cites fact, claim or subject ids. A recipe row
  says what it stands on: "Inferred from manifest settings" when it cites a
  manifest row (redis's `make`, its Makefile's default goal beside
  redis-server's main), else "Inferred from an entrypoint". When the overview
  is refused by size or context, the report is published with an empty
  orientation, the refusal in `rejected.jsonl` and an `unavailable` state in
  the console. A flow ending at a split the categorizer left undecided is
  journaled under `flow_fork`: its last step shows the fork on one folded
  line in words ("one of the calls `call` may make ▸", or "one of the calls
  it may make" when its candidates share no site; ru "один из вызовов,
  которые может сделать `call`", "один из вызовов, которые он может
  сделать"), its candidates' names inside, never a wall of names and never a
  count; each step shows how the step before reaches it ("called", "handed
  to quil.core.sketch.setup", "handed over", "one of the calls `call` may
  make", or "one of the calls the step before may make" when no function
  holds the site; ru «вызывается», «передаётся в quil.core.sketch.setup»,
  «передаётся», "один из вызовов, которые может сделать предыдущий шаг")
  under its name, always through the vocabulary (owner: no digits in the
  column; the saved `Via` keeps the code's "called" and "one of 94";
  `TestAFlowsViaReadsInThePagesLanguage`). A dispatch site is said by the function holding it, a name read as a
  step's is, never by a file and line. Fork candidates sharing a name are
  told apart as the categorizer read them (s3.ReplicaClient,
  gs.ReplicaClient; READING § Orientation). Unknown or incompatible set refs are recorded and removed; repeated
  refs are deduplicated. A row with no required evidence, an invalid scalar
  choice or a conflicting interpretation goes to `rejected.jsonl` with its raw
  output and reason. Independent sections and rows survive a bad neighbour.
  Complete prose is preserved; short labels normalize whitespace. Validation
  annotates and never aborts the run.

## UI iteration and review

UI work follows [AGENTS.md](../../AGENTS.md#supported-paths-and-verification)
and [Development](DEVELOPMENT.md): ordinary templates, `repomap render` on
the same saved run ([One publication](#one-publication)), temporary
worktrees, desktop acceptance, loopback browser QA, and real-question and
return-after-a-break journeys rather than DOM checks. Beyond those:
accepted worktree commits reach `main` by merge or cherry-pick, never by
reimplementing the experiment; watch disk space; mobile layout does not
drive this redesign.

The owner's UI review method (2026-09-07) follows a pair of real questions
through the visible interface. Choose actions by their visual affordances,
search first among prominent elements, then explore beside the relevant item.
An unfamiliar term starts a side question. After navigation, assume the
reader remembers learned terms but has lost their place; the page must help
them orient again and start the next question. Record these journeys in
`repomap-ui-ux-review.md`.

The return-after-a-break check asks whether, after leaving and returning to
the tab, the reader can tell where they are and what they were doing.
Evaluate the current screenshot with the click history forgotten; retained UI
state alone is insufficient. UX31 records this check; its importance is for
the owner to assess. Do not infer an unrecorded user intention from the
selected object.

### Whole input paths and reverse reading

The common map joins saved operation paths only through an exact activation
endpoint. It keeps all reached paths across components and cycles, without
borrowing a sibling operation in the same part. A matched input's parts and
state changes continue the root's path marked a possible integration, with
that input's own entering calls; no chain is prefixed to them. Outgoing
communication attaches to inputs whose saved reach holds its caller subject.
Clicking a part or communication lists those inputs by activation type;
selecting one restores its full path.

Leaving an input path or closing details is a contextual action in the
reading card. Automatic level changes remain pending; the explicit layout
contract above still applies.

Reachability is not entity mutation. The concept projection reads accepted
native type declarations only; value-based domain models without a named type
are not invented during rendering. An input's "State changes" are its
writes to the program's data, never its reads (critic, 2026-09-30; milestone
review: litestream's config and metrics fields, freqtrade's select had stood
there; `page_entity_writes.go`). Its work is its handler and what it calls
exactly, never entering a helper (a declaration the helper question decided
serves others' work, in a part most of the program's parts call into, other
than the handler's own). Listed, from facts only: a row of a table the
reach creates, a construct call (ProgramIndex invocation `construct`) of a
type owning a table ("Trade — new row", by execute_entry; Python and
TypeScript construct; a Go struct literal and C's allocation of a record
are no construct, so they create none yet); a field of a type owning a
database table (a table record's owner) the work
writes, save a constructor setting up the object its call makes; a field
one of the program's own files is written from (read by the code making a
call on that file, save the field naming its path: redis's rdbSave reads
redisDb.dict) handed to a helper whose code writes its type, said as handed
("redisDb.dict — handed to dictAdd, dictReplace"), no fact saying the helper
writes through that parameter; a database call a writing query statement is
written at (INSERT, UPDATE, ALTER, …), with its tables; every outside system
the reach calls, named once under "Sends to" (a GET or HEAD request reads,
and the facts tell no store from a service); the files the reach makes calls
on (the facts give no file call's direction). In-memory configuration and
counters are no data unless a table record owns their type. What
functions on the input's own path make (its spine's steps and branches and
their members) comes first; what only deeper code makes folds under "also
deeper in its reach", by name alone, unless nothing on the path changes
anything (milestone review, 2026-09-30: an entry's exit fields had stood
before its new Trade). Each target
once, with every function making the change, their names links into their
code; a field or table links to where it is first changed, that place on its
hover; no line is printed and no hedge line stands. A part's reading lists
its types' changes, by input. A matched input's changes join as a possible
integration; sibling inputs gain none.
