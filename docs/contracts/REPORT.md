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
  layer next to it, its arrow straight into it; the component is named by
  that arrow, by the frame's zoom-mark accessible name and in the location
  row. Its distant summary shows the actual
  catalogue types (requests, commands, background work, interactions, other
  operations); zoom reveals the named input nodes. Inside, inputs are grouped
  by the part holding their handler (the saved implementation owner), or,
  for an input whose handler is not established, the one part its Inputs
  arrow goes into, each group framed, titled and ordered by that part's name,
  its inputs in rows wider than tall; choosing its title
  reads that part. The collection opens to its groups first, each closed like
  a closed area with a zoom mark entering it; a group opens to its inputs
  when their headings read (14px to open, 12px to stay open). An input with
  neither stays loose after the groups and opens with them; a collection
  whose inputs share one part keeps them loose. The groups are display
  containment, not architectural areas, and add no relation. A tile says its
  kind by a small muted mark before its name (Primer Octicons, the kind's
  name on hover), never by a printed kind row; the same mark stands before
  a kind's name in the reading column and the map's key, and a kind with
  no mark of its own (an entry whose kind is not established) wears a
  neutral dot. A collection lists its kinds as the column names its
  sections (Scheduled tasks apart from Background work, Queue consumers,
  Extension points, Kind not established): Redis's Inputs had listed three
  kinds beside a column of four. A tile names a callable written inline as
  the column does, "anonymous function in {function}". A program's Inputs
  and Outside frames are named with their program in Connections and cards
  ("← Inputs · redis-server"), and a part's reading is headed "Part".
- An input whose handler is not established (GroupsIndex `handler_unknown`:
  an option a call declares, a value handed over) has no owner: it stands
  loose, no implementation arrow binds it, and such a request is no route and
  joins no portal. From saved `DeclaredBy` and catalogue data its Inputs arrow
  goes into the part where its code takes it in: an option or word into its
  declaring function's part ("declared in …"), a table's row into the part
  of every function reading the table ("looked up in …"), none when nothing
  reads the table. The arrow means "taken in here", never "implemented in":
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
  chip per destination its records name, every chip one size, in rows
  toward a square, naming it in at most two lines with the rest on hover,
  and the records naming none in one muted "not established" chip last.
  A destination's calls are read in the column when its chip is clicked,
  the camera staying; no call tile is drawn, and the program's arrows to
  its destinations are one arrow to its Outside frame, the calls behind it
  in its card. That card reads the destinations the arrow reaches, each
  under the parts calling it, not the calls one to a line: litestream's
  had listed 89 calls over half the map; the calls are read in the column.
  Equal destination text proves no identity: each program
  keeps its own destinations. One call written once (the same destination
  and symbol at the same saved path and line, owner, 2026-09-28) is one
  call, in the first program's destination, with an arrow from each program
  making it. An Outside frame and its chips are display collections, not
  inferred components.
- When a saved connection identifies one displayed peer in another target,
  the canvas connects the original caller straight to that peer/input, with
  no third participant; its outbound catalogue and source reading stay
  intact. Missing, partial or ambiguous matches stay separate; equal
  destination text never establishes the link. Both endpoint sources,
  operation membership and possible status survive the display projection.
  Source-owned cross-component links connect the same common canvas.

- Outside frames and their chips are amber, parts and area frames neutral; core parts are
  rose, entry parts green, inputs blue and external communications amber,
  identities the saved lanes supply. Each colour means one thing (owner,
  2026-09-28): purple is a link and nothing else, a key declaration is bold
  ink with no colour of its own, and an input's name on the canvas is the
  inputs' blue. Core/entry cards carry distinct diamond/arrow glyphs, shared
  with the legend and accessible names, instead of Core/Entrypoints above
  every title. The border stops at the entry arrow, and the legend draws the
  same arrow.
- An area's mark is its GroupsIndex container's (READING): the core mark when
  any part in it is the domain, the entry's area included (owner,
  2026-09-27); the entry mark only on the area holding the program's entry (a
  declaration its execution starts from, a target seed) when no domain part
  stands in it; otherwise none, or the dependencies mark. Only the part
  holding the program's launch point has the entry mark; a part that only
  takes requests or listens has none and does not make its area the entry,
  its inputs' blue arrows showing where the outside calls in. The component
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
  crossing the gaps between tiles flashes nothing. Frames grouped under one
  destination's text recede with their heading only when none of them is
  involved.
- Component frames keep their language, kind, role and purpose above their
  parts. Colour never replaces the visible type cues or the independent
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
  exact result when it is out of sight, at a readable scale, opening its
  enclosing frames in the fixed world; a hidden child's bounds inside the
  viewport do not count as visible. The camera moves only to what is out of
  sight, otherwise the canvas marks it (owner, 2026-09-28): a fully visible,
  legible part opened from a reading, a frame (a component, an area, an
  Inputs collection) drawn and mostly in sight, and a declaration whose tile
  is drawn in sight keep the camera.

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
  an input's path. The canvas draws a quiet arrow only while one of its ends
  is looked at and decides nothing of its own. An arrow drawing several
  relations is quiet only when all of them are. The system canvas is the
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
  border the canvas paints it with, its mark on its border; the solid "calls"
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
  ordinary pointer, and only its magnifier zooms. Entering the path, the camera takes the part
  holding its handler, then each part the trace reaches from a part already
  taken while all fit at a scale where their headings stay about twelve
  pixels (their layer open), and frames them; when the next step does not
  fit, it stays at that scale leaning toward it, keeping what it took, so the
  dark arrows leaving the frame show the way. The path's parts, and a closed
  frame standing for parts hidden in it, are outlined in the path's dark. An
  input without a trace is entered as its tile, and such a tile clicked keeps
  the camera. "Show input" stands in the reading card while the camera may be
  away from the input's tile (on its path, or on a part read since) and
  frames the tile within its group, the inputs its handler's part takes,
  never the whole collection; a group larger than a readable camera is
  entered at the tile. Once the tile is framed there is nothing to return
  to; choosing it again returns to the path. The input's reading is under
  "An input's reading" below.

- The looked-at frame marks its arrow ends: hovering an area or a part inside
  it marks the area's; pointing anywhere in an open component (its space,
  border or title) marks the component's, as choosing it does. A closed
  component marks nothing and takes no marks of an open one beside it.
  Nothing on the map is a digit (owner, 2026-09-28): no part number badges,
  digit chips, key line explaining numbers or numbered/arrows switch.
  No plaque stands on an arrow's end (owner, 2026-09-29): the arrow is its
  own handle, its end an ordinary head on the border it enters. Both
  directions of a pair of frames share one route. The same real endpoints
  stay connected across zoom levels. While an end's card is open or kept, the parts behind that end take
  the dark outline in place (a closed frame hiding them takes it for them)
  and the end's arrows are dark; the end recedes nothing, and the
  parts at the arrow's other end stay as they are.

- A card (a label's calls) opens on intent, only after the pointer pauses on
  its handle; a handle crossed on the way elsewhere opens nothing. A
  connection's handle is its drawn arrow, at every level: a wide unpainted
  hit path along it, under the boxes it joins, which take the pointer first.
  The pointer on an arrow is on the connection of the head nearest it. A
  head stands for the incoming
  connection of the area or component it points into; a head on a destination,
  the inputs or a loose part, for the outgoing connection of the frame at the
  other end; on the looked-at frame's border, for its own direction. An arrow between two parts of one frame has no card. An
  arrowhead stands outside its frame's border. A card stands flush with its
  handle, outside the frame being read so it covers none of its parts, clear
  of the handle, on the side with room and wholly inside the canvas; a label's
  card goes out through the border its label stands on. Without room outside
  the frame it stands beside its handle toward the roomier side. It moves with
  the map but keeps the screen's type size, placed again when the zoom
  changes. While a card is open or kept, the frame being read stays: the way
  to the card crosses parts, frames and empty canvas without changing emphasis
  or labels. Leaving the handle, the pointer is safe inside the triangle
  between where it left and the card: the card lasts and no other handle on
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

  A label's card (owner, 2026-09-27) is headed by the two frames its arrow
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
  or handing over part of a set, keeps its rows. A label's card always shows
  the whole end.

- The layout is computed in `internal/report/web/split-layout.mjs` and
  `layout.mjs`; this contract states only what the reader sees and what the
  layout must never do. The component frame wraps its contents, with no
  aspect padding around a compound column. Outer arrows stop at participant
  frames even while their contents are open; continuations to inner parts
  are not painted, and the arrow's card at the boundary stands for the
  explored area's inner parts behind it. Shared participant labels keep every
  matching part and source. Aggregated strokes are display geometry drawn
  once, not semantic relations; original endpoints, certainty, possible
  status and source relations stay distinct in the reading data and
  operation paths, and every original edge ID and relation survives the
  bundling.
  An area's layout fits the initial canvas with its part headings readable
  at the size their layer stays open at, then takes the fewest detours, then
  the squarer box. Its parts keep their own size, that of the loose parts
  beside it, so an area is as large as what it holds, with no second
  member-list height; but no closed card of a component (an area or a loose
  part) is smaller than nine twentieths of its largest area on either side,
  or half again its own size, an area so grown holding its parts in its
  middle and a loose part drawn open at its parts' size in the middle of its
  box. The arrows between a component's areas keep the interiors' spacing in
  the unit of those areas' median height. A closed card's title reads at
  twelve pixels where its layer opens, or where its program is entered
  whole when that is closer; its description takes the whole lines left
  under that title, cut short with its whole text on hover. A program's
  title grows as the camera leaves it only within the band its frame keeps
  for it: grown past it, "freqtrade" had stood over its first area. The
  magnifier frames a part whole only while
  its declarations still read at eleven pixels there; else at that size,
  its head and first column in sight. A closed group shows its name and nested-content hint,
  then reveals its objects directly at the next common layer; there is no
  intermediate member-list view. An architectural area holds at least two
  parts (reading draws none smaller; GroupsIndex keeps no container of fewer
  than two groups) and every group is a node of its component's map, so the
  page draws every area it is given as a frame. Participant, input and
  Outside frames keep their boundary even with one child.
  Component summaries list their immediate areas in the order the model
  listed them, then their loose parts, so folding a wrapper never removes its
  responsibility from the overview; placement still follows the connections.
  The parts entrance below the map lists each component's areas the same
  way, then its loose parts by name, with the saved area caption; the single
  card and its source reading keep the part's own title. A loose part beside
  areas is their closed summaries' peer while they are closed: the same
  first-reveal heading fit and column width as neighbouring groups, promising
  no hidden children. Its box is its own card's, whatever the areas beside it
  hold, so on the component overview its title can read smaller than theirs;
  this is accepted. Once the areas open it is their parts' peer: the same
  card at the same scale, filling its box. A component without areas keeps
  its direct parts' fitted headings. Input collections stay outside the
  component at every scale.

  Detail is synchronized by hierarchy depth. The whole-map layer shows
  targets, external participants and input collections. When the first
  participant's contents become readable, every participant reveals its next
  level; when the first area's do, every area at that depth reveals its
  objects. The trigger is the largest child heading at that depth: 14px to
  enter, 12px to retain on retreat. The first layer also opens when any
  root's longest screen dimension reaches 75% of the canvas's, retained down
  to 65%. Whole-map fit always keeps summaries. Pan does not change layers;
  Back restores the common hysteresis state. Smaller siblings never delay or
  independently close the layer. A detail switch changes no camera
  coordinates, box dimensions or routes, and no layout runs during zoom.

  Summaries lay out their complete text against their own frame at the
  whole-map reading scale and zoom scales that fixed layout; pan clips a
  card at the viewport edge, never squeezing or rewrapping it. Compact area
  labels fit their complete names and hint to each group rectangle once;
  smaller groups keep smaller text instead of going blank, and zoom enlarges
  that layout without hiding or rewrapping it. A frame's outline is painted
  once at both detail levels, with no second border over its title. Hover
  and selection change outline paint only, never border geometry, padding,
  title position or wrapping; nothing inside a card moves when an area is
  looked at or chosen. Closed frames show a subtle outlined child element
  with a question mark as their nested-content hint; the accessible action
  and the card both enter the contents. A toolbar hint explains zooming and
  dragging. Leaf cards and open frames promise no further hidden layer.

  A part with declarations has a zoom button; zoomed far in, its
  declarations stand inside its card as tiles, each a declaration to choose:
  a type with its fields and methods under it, a function or a module's
  variable alone. A module variable its file keeps only as its handle on the
  platform is no tile (GroupsIndex `PlatformHandles`: a standard-library
  call's result only its file's functions read, Python's module logger;
  freqtrade's Trading bot core had three "logger" tiles); the reading still
  lists it. A tile keeps its link column, the longest chain of calls,
  returns and takes leading to it, so a caller stands left of what it calls.
  Tiles stand by file, the files in the order their first declaration is
  listed, each file's declarations in the page's order: the model's keys,
  then the types, then the rest (owner, 2026-09-28); the file is a hint.
  A part whose declarations share one namespace (Clojure's
  `othello.ui.host/`) names them without it, the whole name on the tile's
  hover and in the reading. In the column a name breaks only after a dot or
  a slash, never at an underscore, a hyphen or inside a word; a piece too
  long for its line ends in "…", the whole name on its hover.
  They stack in that order in their column, a column too tall spilling into
  the next. No name is cut, and the columns share the card's width. Tiles
  are drawn small enough to hold them all whole: nothing is counted away and
  no scale is too small to search. The part's name stands over them at the
  same screen size at any such scale. The zoom button enters at the scale
  the declarations read at: the part whole when it fits, else its head and
  first column at the canvas's top left. A drag over the declarations pans
  the map; a drag that moved chooses nothing.

  Pointing at a declaration darkens only its own arrows: its links to the
  part's other declarations, and the part's arrows carrying a call into it
  (the call's callee is its source) or out of it (the caller is its name);
  nothing recedes. A click on a tile chooses that declaration: the part is
  read with it named in the reading column, the tile keeps the read outline,
  its arrows stay dark, the declarations its links do not join recede while
  it stays chosen, and the camera stays on a tile in sight. A modifier click
  opens its code. A declaration the reading column names by its source
  (Find's code hit, a declaration chosen in the reading, a restored visit) is
  the one chosen on the canvas. A newly named one out of sight is shown with
  its part at the zoom where the part's tiles are drawn and the part stands
  whole across the canvas: framed whole when it fits, else across with the
  tile centred down it, never deeper than the part's own title fits (owner,
  2026-09-29); a restored visit keeps its camera. A part too dense for its
  tiles to be read there (names below the size an open frame's text stays
  open at) is shown as its frame instead: its card closed at the scale a
  part is read at, the chosen declaration alone in it as its tile, the
  camera staying when that card is in sight (owner, 2026-09-29). The page
  data gives each tile its file and the same source link its reading uses,
  served or static.

  Root summaries put saved area names before role, counts and purpose;
  complete dense inventories scroll without dropping entries. An ordinary
  wheel scrolls an overflowing inventory, staying with it past its end;
  pinch passes through to the map. Anywhere else on the canvas, its location
  row included, the wheel moves the map and never scrolls the page. One
  pinch (ctrl+wheel) crosses at most one level boundary, going on or back,
  and stops short of the next; a pause ends it. The levels are the whole
  map, one per open hierarchy depth, then a part's tiles in sight; two layers
  opening at one zoom are one boundary. Only a tick that would cross a
  second boundary is held short of it. Secondary purpose text uses the
  remaining complete lines, the full text in the reading column. Text and
  controls stay inside their own frame; viewport clipping never moves them
  to another corner. Where the reader is is said once, by the page's
  breadcrumb, with the reading column's heading naming what is read and the
  frame holding it (owner, 2026-09-29: the canvas's own location row, the
  breadcrumb and the column had named three places, and "System map" stood
  three times on the home). The canvas keeps its location only for
  assistive technology and for a layout that failed. Root summaries and revealed interiors are exclusive.

  Arrows between groups and participants stop at their boundaries, open or
  closed; routes inside one group keep their part-to-part endpoints, and
  connections wholly inside a group reveal with its objects. Relations
  sharing a directed visible pair use one route, dashed only when every
  original relation is possible. A call left unresolved whose store
  witnesses name its candidates is possible toward each of them (READING,
  Operation ownership): dashed like alternatives, its rows in the source
  details reading "possible" in the muted text, the key's dashed stroke
  saying what it is. All original endpoint IDs and sources stay on the
  route; the drawing invents no relation. Plaques sit just inside the frame,
  centred on the arrow's connection point without moving it, and encode no
  execution order. No route shows a diagonal clipping segment, and no
  update drops arrows between animation frames. There is no additional
  boundary route planner, A* layer, custom marker packing or replacement
  path.
  The initial overview fits all root component and communication frames.
  The unpositioned drawing stays hidden, with a loading indicator in the
  reserved canvas, until the fixed world and camera are both ready.
  Workspace height accounts for the actual header and controls. In
  whole-map mode a resize lays out the outer frames again, keeping every
  interior, and fits the new bounds. Manual pan/zoom ends that mode, keeping
  the reader's world and camera; choosing the whole map again may lay out
  again. A pending resize layout cannot replace a world after a manual
  gesture. Saved whole-map intent survives a changed geometry identity.
  Component entrance fits the whole participant frame before a group is
  explored, siblings on both sides in view; entering a group or a call then
  uses its content scale. A focused area that fits the canvas with its
  parts' headings at about twelve pixels is fitted whole, never wider than
  the visible canvas; only an area too large even so is entered at its first
  part at that scale. Entering a frame or an input's path opens what it
  enters, keeping those frames open through the camera move even when the
  camera ends smaller than a closed layer needs to open by itself.
  Even in a short window the camera can show the complete root bounds, and
  the initial layout accounts for readable root headings, not only the
  bounding rectangle. Overview text has room at the fitted size:
  small area lists show whole, a list taller than the canvas shows its first
  entrance and scrolls the rest, a short component name never squeezes its
  inventory into isolated words, and an oversized list never grows the world
  repeatedly or collapses its width. Long words are not broken; a narrow
  heading may continue below its zoom mark instead. A title wraps only
  between words, as the browser wraps: closing punctuation stays with the
  word before and an opening bracket with the word after, so no line starts
  with ")" or is ")" alone. A word breaks inside only when it alone is wider
  than the line, after a separator or between camelCase words when it can,
  never before closing punctuation; a path breaks only after a "/", a
  segment itself only when it alone is wider than the line, and never
  before its extension ("scripts/ rest_client. py" had read on freqtrade's
  map, 2026-09-30). Map titles are drawn as those measured
  lines, never broken again by the browser. A heading still short of its
  longest word shrinks its type until it fits; a part's title leaves room for
  its zoom button. A part's description takes at most three lines, the third
  cut with an ellipsis, and never leaves the card an empty line. When the
  whole-map fit cannot give a summary its room, the summary is drawn scaled
  down whole, its zoom mark with it until zoom gives the mark its ordinary
  size, as a small group keeps a smaller complete label; a frame too narrow
  for its text at full size keeps that smaller summary rather than stand
  blank. Compact component purposes use
  the remaining whole lines, with an ellipsis when shortened, hidden if fewer
  than two lines fit; the complete purpose stays in the reading column.
  An Outside frame's summary is its chips, at their own size at the
  preferred camera.
  The fixed world places frames, parts, complete input cards, component
  purposes and grouped labels. The outer layout prefers readable text, then
  a smaller world; unzipping is not forced on small maps. Arrows keep room
  between frames (owner, 2026-09-29): the outer spacing is three fifths of
  the interiors' in screen pixels at the camera it is laid out for, and a
  correction that grows the boxes for a smaller camera grows it with them,
  so it shrinks on screen only as the square root of that camera and never
  below half; a summary may be drawn smaller than its reserve for it. When
  the fit would shrink component inventories, collection headings or input
  types below their readable size, a correction before display reserves the
  missing space: at most four passes, each placing the grown boxes over the
  same eight candidates, a root readable in the first placement staying
  readable after the correction, never laying an interior out again, and no
  zoom-time layout. A component grows whole, in the arrangement of its areas
  (of both directions, with and without unzipping, all prepared once) nearest
  the box it grows to, so its open areas fill its frame with no empty band.
  A component whose arrangements all leave its smallest card under 64
  pixels tall where it is entered whole packs its cards instead (owner's
  review, 2026-09-30: freqtrade's fourteen areas had covered 7% of their
  frame inside a hundred crossing arrows): a grid toward the canvas's
  proportion, in the reading order of the first arrangement, its arrows
  orthogonal in the gutters between rows and columns, never over a card.
  Every arrow at one side of a card meets it at one point and runs in that
  card's one lane of the gutter, a trunk until they part, one arrowhead; a
  gutter is wide enough for its lanes five pixels apart where the program
  is entered, the frame's padding kept outside its outer lanes. In a
  layered arrangement the arrows into one side of an area likewise merge
  (ELK `mergeEdges`). Cross-participant labels take no interior
  space; their cards belong to the outer endpoints. Components and input
  collections show no empty padding around a long column and keep short
  catalogues compact. Their reserves are measured from the text, never a fixed
  wider frame for every participant, and the fit that sizes them frames
  exactly what the whole-map camera frames. Initial placement and resize use the same
  inner canvas dimensions. Layout runs off the main thread, inside the
  self-contained page. An initial failure leaves the ordinary report
  available; a later one keeps the last complete world and offers reload;
  neither leaves a permanent loading state or retries on the main thread.
  Emphasis darkens and thickens a line, never its head: every arrowhead, a
  link's head between declarations included, is an ordinary one's size.
  Strokes, casings, dashes, arrowheads, frame outlines and corner radii keep
  their screen size at every zoom. Ordinary camera movement never rebuilds the
  graph or remeasures unchanged text; only common detail-layer transitions
  update displayed contents. The first click reads and zoom is separate
  (owner, 2026-09-28): a click on a part, a frame's title, a component's, a
  collection's or a destination's whole-map card, an area named on a
  component's card, or an input's tile reads and marks it without moving the
  camera; the magnifier, "+" and a double-click zoom.

- The hover area persists through gaps between its labels and clears on
  leaving the canvas, window blur, a hidden document or a click on empty
  canvas. Hovering changes only the drawing, never the reading panel; only
  clicking another card replaces the selected details, and there is no
  duplicate node preview. Connection previews clear on leaving the
  canvas/reading workspace, window blur or a new selection, and never cover
  parts or routes. A pinned input keeps its exact saved path while other
  parts are explored. All participants of a hovered area stay readable. The
  map uses the available window width independently of prose width. The
  initial view is the whole-map summary. Reset keeps the current level and
  centres the selected item at readable scale (or the topmost part when
  nothing is selected). A part is centred in the canvas; one taller than the
  canvas shows its head just below the top. A regenerated layout
  invalidates old camera coordinates and reveals the selected item instead.
  A navigation, click or camera move cannot trigger a different hover
  emphasis under a stationary pointer; real pointer movement resumes hover.
  Viewport history includes its zoom, open areas and the fixed world's
  geometry identity. Every section and source link stays reachable from the
  page.

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
opens the code. No name carries a line number and the page prints no
separate code marks (owner, 2026-09-29): a caller, callee or variable is its
name, once however many places the relation is written, its part named on
its hover; where each relation is written is on its name's hover. Of those
places the page data keeps a "Called by" caller's first, in source order,
with every place in its words (`site`).

A part's reading heading gives its kind and its frame as a link up ("Part ·
{frame} ↑"). "Called from" lists the parts calling into it, each
in its box with its caller → callee pairs, and under each caller the
declarations of this part it reaches, calls first, then other relations in
their own words, each list by name; every input registered at the part is one
neighbour, Inputs. Many callers fold each part to its line. Then come the
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
separate.

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
count or meta word. Each run of calls into one part stands under that part's
box (its description on hover, a click reads it). A library call is no row:
one muted line ends the step, "also calls: …", each name once in
written order, its library on hover. A declaration no part holds is a plain
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
"↑ shown above" instead. What a call hands over ("passed as a callback") and
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
(owner, 2026-09-29), with how it comes to run, from the program's facts and
calls: where it is registered, as the run of exact calls from the most recent
earlier step (the program's entries for the first) to the function making
the registering call ("{a} → {b} registers it"), for every
registration of that callable the step reaches, or every one when it reaches
none, never an arbitrary first one, registrations by the same run read once;
and what runs it, each function calling it through a value (a dispatch's
alternatives, a call through a function value resolved to it alone, an open
call whose stores name it), after the run of exact calls from the entries to
that function the first time the flow shows it ("{entry} → … → {runner}
runs it"), from the last runner already shown on that run ("{runner} → …
runs it"). The step's name and every name in
its registration and runners read that declaration; the words "registers it"
link the registering call's line; the step prints no line (owner, 2026-09-29).
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
shortest exact route, through its shutdown). What runs it reads as
a Main flow step's runners do. A callable no saved registration hands over
is its name alone. Nothing is looked for beyond the saved kinds and
facts, so a program without such inputs lists none. Pointing at a line lights
its input's tile. A scheduled or continuous input's own reading opens at that
line, its line link going with it.

An input's reading is drawn in the Inputs blue of its tile and collection,
never core's rose: its heading's bar and kind and its links. A chosen input's
reading opens at its path (owner, 2026-09-27). It names its handler ("handled
by {handler}"), reading that declaration in its part when the part lists
it. An input whose handler is not established says so ("handler not
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
opened, each way is its chain. Then comes what its handler does, its flow, a
single call opening by itself while its calls go one at a time; last, one
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
Then come the parts the input enters, nearest the handler first: each part's
name (a link to its reading when the map draws it), the handler under its
part ("handled by {handler}"), every call entering it from a part reached
earlier (the first five, the rest under "+N"), and "{n} more calls into this
part come from other code on this path" for calls into it from parts reached
no earlier, none of them the handler's. The parts the handler calls directly
(depth 1) stand open; every deeper part folds under one line, "Reaches {n}
more parts deeper", opening them as they are. The fold is by depth alone: it
chooses no route and drops no call. A call is its two declarations' names, a
read or possible call marked as elsewhere; a name in a drawn part reads that
declaration there, as a click on its tile does; a name in a part the map does
not draw is only named. There are no "Shared by" or "through" words. The list
of the parts on the path is left for an input without one.
A part read while an input is pinned says "Outside this input path" in its
heading when neither it nor a part inside it ends one of the path's arrows,
decided from the reading's own state, never from the pointer. An up-chevron
in the reading card's own header closes details, keeping the camera and any
pinned input path; leaving the path is a separate action.

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
together. What a reader opens in the column (a fold, Open all, any disclosure)
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
author claims: ordinary rendered content, not another model summary. The
components, their areas and inputs are on the canvas and are not listed
again (owner, 2026-09-28); the targets the run could not read are named
there with why. On the canvas they are one note, "Not analysed", naming
them, sized with its words as a summary among the programs' and taking
their connections: one pale card apiece, litestream's two failed packages
had read at four pixels. The introductory sentence has no model badge or source popover;
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
it zooms out one step. "+" zooms in one step.

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
the same title. The home's table of programs says what each is "Built from":
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
each in its box with its description on one line and its key declarations
alone, bold, one to a line; the rest is the part's own reading.

An Inputs collection is read by its catalogues: its component first, in its
box, linking to its reading; then each kind once, under its heading
(owner, 2026-09-30: litestream's had read "Incoming requests" once per
declaring function), each catalogue of it under a quiet line for where its
inputs are listed or declared, where they are looked up and what else the
declaring code uses, the model's words that some are matched by name to
another program's inputs, and its inputs one to a line, each reading its
input, a name that is a sentence (a query parameter's description) in the
reading's own type, not code; then the inputs no catalogue holds; requests
first. Each input's record, with its registration
line as written, is its own reading.

The component's "Entrypoints" link lands on the program's entry, from
GroupsIndex's entries in the page data: the part holding every seed, the seed
read there when it is one; when a seed stands in no part, or the seeds in two
parts, the component's reading, opened at its entry line when it has one. Its
full-reference section keeps the main flow, configuration, its parts (each
reading the part on the canvas), dependencies, coverage and TODO lists.

Its traversal coverage, on the "What is missing" page under the heading "Not
reachable from the entrypoints", lists the files no entrypoint reaches; then the parts its
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
several programs, or none, stays outside.

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
and infers no runtime caller.

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
presentation graph. It is compact JSON (format 93) and writes each
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
  and main flow; every row cites fact, claim or subject ids. The orientation
  asks an overview, then the main flow of the target it names (READING §
  Orientation). When the overview is refused by size or context, the report
  is published with an empty orientation, the refusal in `rejected.jsonl` and
  an `unavailable` state in the console; a refused flow request is journaled
  under `flow_request`, the overview stands, and the report opens its entry's
  calls. Unknown or incompatible set refs are recorded and removed; repeated
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
are not invented during rendering. Target-bound native writes carry their
original call-site location into GroupsIndex. The report lists writes only
when the input's saved reach holds the writer and the written variable has an
exact native type owner. It keeps possible receiver/call resolution, the
write source, and the writer's callers on the path (every call of the reach
into it, none chosen as a route; none when the handler writes itself);
reachability does not claim execution on every request. Entity readings
reverse these same records. A matched input's writes join as a possible
integration; sibling inputs gain no effects. Unresolved writes, ordinary reads
and mere membership in a type-bearing part do not establish mutation. Saved
graphs without write locations produce no invented evidence.
