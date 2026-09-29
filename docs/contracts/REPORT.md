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
  connections with exact endpoints and source locations. Containers keep the
  atlas's zone order, the order the areas answer listed them; no key sort
  replaces it. A joint is a connection
  into another target. A group is `triggers` only when it holds a target
  seed (the program's launch point), `dependencies` when its box only calls
  out, `core` otherwise (GroupsIndex 19). No request sends repository source text: paths,
  names, signatures, first sentences of docstrings and documentation excerpts,
  literal values and the model's own earlier lines cross the wire. Question
  excerpts preserve Markdown-authored command/code examples; implementation
  source-file bodies are not sent.

- No file or inventory box is drawn: every box is a part of the map of
  parts. The atlas's explicit off-map record lists, per target, every file (or
  stray declaration) no drawn part holds with its reason (`left_out`,
  `conflict`, `no_units` for a file that declares nothing, `map_failure`,
  `undecided` for a split file's declarations no box of it took after a
  question, `blocked` for its helpers no question reached because a
  declaration that uses them got no box), keeping
  its file line, captions and keys. Stray declarations in a file a part
  holds name the file's part (`box_id`) when it has one; their file stays
  on the map. A file whose code several parts hold (READING, the role split)
  is on the map through them. GroupsIndex (since v17) lists the declarations off
  the map in a file a part still holds by their subjects (`subject_ids`),
  under their own reason (`undecided`, `blocked`, or `left_out`/`conflict` for a box
  or a stray method), never as a file off the map; a file no part holds is
  listed whole. GroupsIndex carries that record as `off_map`, adds the files of
  parts made only of test code under the reason `tests` with their part's
  name, adds the declarations of a part its program never runs (atlas
  `unreached`, READING) by file under the reason `unreachable` with their
  part's name and subjects, and carries `map_failure`. Subjects off the map keep their interpretations
  outside every group; a boundary in a file off the map names no box and its
  operation belongs to no group, yet it stays in the component's inputs. An
  input whose handler is off the map, undecided included, names no box
  either (READING, "A file in several boxes"): a command table row's `set`
  request whose handler is undecided is never shown as handled in the part
  that holds the table. A part
  made only of test code is a fact (every file is a `TestSources` file): it is
  not a group and leaves the canvas. So is a part its program never runs: it
  is not a group, draws no arrow and leaves that program's canvas; kvcli's
  part of `loop.c`, the event loop it links and never calls, is not drawn.

- The component card lists, after its link to its parts on the system map
  and before its main flow, the compact inventories **Tests** (test-only
  parts' files, with their part) and **Not on the map** (every other off-map
  file, with its reason; a split file's row shows its undecided
  declarations as source chips and reads "In no part of its file", and its
  blocked helpers in a row of their own that reads "Used by code in no
  part"), five
  rows each and
  `All N` for the rest. Find lists each of those undecided declarations as
  Code with Open code and no "In part" link; its result opens that row, scrolled below the sticky toolbar as every
  page destination is. A target
  with a map failure says "The map of parts is unavailable:" with the reader's
  words for its closed reason (`refused`: the model's answer was refused;
  `no_model`: no model was asked; the refusals themselves are rejected
  rows, never card text) and lists all of its files under Not on the map. A
  part or area without a description shows the explicit "No description"
  state; its title is never repeated as a description.

- Observed HTTP routes join their target's catalogue through the original
  FactID restored by atlas projection. The renderer neither repairs foreign
  identities nor merges model operations by matching names or paths. Listener
  addresses remain source observations, outside the request-operation list.
  Accepted model wrapper/request interpretations retain their existing status.
  Route summaries count only original displayed HTTP registrations. The combined
  incoming catalogue counts records, showing native registrations and unmatched
  interpreted handlers separately; those records can describe the same endpoint.
  Its registrations are labelled "Registrations in source", not HTTP ones: a
  command a client sends by name is listed there too. A registration's method
  badge appears only when the registration states a method; a command has none.

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
  identity. Every original relation and its source endpoints survives in the
  node readings. The canvas draws one physical arrow per directed visible-node
  pair, combining its operation membership and short labels: coincident call,
  callback and implementation rows are evidence on one connection, not extra
  geometry. Opposite directed rows share that route and put an arrowhead at
  each end; their original directions remain in the node readings. This is a
  display assembly after translation, not another semantic
  graph, analysis payload or provider stage.

- Groups and containers keep target-local `g*`/`k*` identities in GroupsIndex.
  The single HTML document qualifies their map/DOM identities with the existing
  target ID (`n-t1-g1`, `n-t2-g1`) so equal local ordinals cannot collapse two
  component maps. Page construction likewise resolves every target-local `n*`
  subject and source location through its owning target; there is no global
  unqualified subject table in which one executable can replace another.
  A component section reuses its `t*` target ID; group, operation and data
  anchors append their existing `g*`, `o*` or `y*` ID instead of minting a
  second name-derived section identity.
  Equal source locations in sibling executables do not create a foreign node or
  operation path; only an explicit cross-target connection can do that.

- The ordinary entrance includes every saved request, command, activity and
  interaction. SystemMap collects the original input nodes into one blue
  display frame per component owner, outside the component. Its heading is
  "Inputs", the word the colour key uses for them; its component is named by
  its arrow into it, in its zoom mark's accessible name and in the location
  row ("Inputs · kvd (executable)"). It stands attached to its component,
  in the layer next to it with its arrow straight into it.
  Its distant summary
  shows the actual catalogue types (requests, commands, background work,
  interactions, other operations); zoom reveals the original named input nodes.
  Inside the collection the inputs stand together by the part holding their
  handler (the saved implementation owner), each group framed and titled by
  that part's name and ordered by it; choosing a group's title reads that
  part. The collection opens to its groups first, each closed and named as
  a closed area is, with a zoom mark that enters it; a group opens to its
  inputs when their headings read (14px to open, 12px to stay open), as
  areas open to their parts. An input with no owner stays loose after the
  groups, and a collection whose inputs share one part keeps them loose. An
  input whose handler is not established (GroupsIndex `handler_unknown`: an
  option a call declares, a value handed over) has no owner: it stands
  loose, no implementation arrow binds it to a part, and a request of that kind is no route
  and joins no portal. It draws the ordinary Inputs arrow into the part
  where its code takes it in, from saved `DeclaredBy` and catalogue data
  (page_catalogue.go `takenInPlaces`): an option or word into the part of
  its declaring function ("declared in parseOptions"), a table's row into
  the part of every function reading the table ("looked up in
  lookupCommand"), each part when readers stand in several and none when
  the table has no reader. The arrow means "taken in here", never
  "implemented in": its card and Connections line count inputs ("100
  inputs"), not handlers, the reading still says the handler is not
  established, and the function is no handler for reach or phases. Every
  collection draws its Inputs arrow into its own component: one none of
  whose inputs has an arrow into a part of it draws each input's arrow into
  the component itself, since that the program takes its inputs in is a
  fact, and ELK places the collection beside its component by that arrow. A
  tile names its kind only when it is not the collection's most common
  kind. The groups are display containment, not architectural areas; they
  add no relation.
  No synthetic type nodes or runtime relations are added. Bound inputs keep
  their exact identity and existing directed implementation relation; they are
  not replaced by their part or duplicated inside it. An unbound operation or
  native HTTP registration keeps its original absence of an implementation
  attachment. Selecting an input opens the same saved path and sources. External
  communication records without an exact local peer retain selectable nodes
  inside amber destination frames, using each component's external catalogue
  grouping: one frame per destination its records name, one tile per native
  outside symbol it calls (the same call made from several places is one
  tile; every caller keeps its line and source on the arrow, and an input's
  path into any of those calls leads to that tile). Every frame and
  tile belongs to its component, and only that component's arrows reach it:
  equal destination text proves no identity. One call written once is the
  exception (owner's decision a, 2026-09-28): components built from the same
  code make the same outside call at the same saved location, so a tile of
  the same destination and symbol whose calls include one at the same path
  and line as another component's tile is that tile, in the first
  component's frame, with an arrow from each component making the call; the
  other components' records lead to it. Redis's redis-server, redis-cli and
  redis-benchmark had drawn three "DNS resolver" frames, each holding
  gethostbyname at anet.c:146 (and anet.c:115). Only the saved location
  decides it, never a name. Frames of different
  components that name the same destination stand together in one display
  group, an amber frame around them in the outer layout, so they stand side
  by side instead of scattered. The group is no
  participant: it has no reading, selection or hover, ends no arrow and adds
  no relation; each frame in it keeps its own program's arrow.
  When every frame in the group spells its destination alike, the page data
  gives the group that text (`DisplayGroupTitle`) and the group's frame
  carries it once; its frames stand as small plain amber tiles with their
  zoom marks, one per program, each hit by its own program's arrow.
  The text is what all the frames name, not the name of a merged
  participant: each frame keeps its title in its reading, search and
  accessible name, and different spellings grouped regardless of case keep
  their own headings and give the group none. The tiles do not name their
  programs: a program's name inside an amber frame would read as an outside
  participant, and the arrow already says whose tile it is. The heading
  stands in a band on the side of the group no arrow enters, under the
  tiles when arrows run down and after them when they run right. Like
  closed summaries it is laid out at the whole-map camera and zooms with
  the map; once the tiles
  open it reads at their open frames' title size and the open tiles stay
  plain under it, so the destination is named once in every state.
  The band is the heading's room and shrinks with it: an open group's frame
  wraps its tiles and their heading, and the room its closed heading takes
  at the whole-map camera stays outside the frame. Entering any tile of the
  group opens every tile of it and frames the group, heading included, at
  the scale its calls are drawn at. The tiles stay
  separate records, each hit by its own program's arrow; the group still
  ends no arrow and is not read or chosen. A plain tile keeps its open calls' proportion: at the whole-map
  fit it grows whole until its zoom mark has its room, and its calls grow
  with it, so it opens with no room of its own below them.
  Every record stays in its own component's catalogue, and records of
  different symbols stay separate tiles.
  When a saved connection identifies one already displayed peer in another
  target, the canvas connects the original caller directly to that peer/input
  instead of adding a third participant. Its outbound catalogue and source
  reading remain intact. Missing, partial or ambiguous matches stay separate;
  equal destination text never establishes this link. Both endpoint sources,
  operation membership and possible status survive the display projection.
  The frame is a display collection, not a newly inferred component. They use amber cards; ordinary parts and area
  frames use neutral tones. Core parts use rose, entry parts green, inputs
  blue and external communications amber; saved lanes supply those identities.
  Each colour means one thing (owner, 2026-09-28): purple is a link and
  nothing else, a key declaration is bold ink with no colour of its own, and
  an input's name on the canvas is the inputs' blue.
  Core/entry cards use distinct diamond/arrow glyphs with the shared legend and
  accessible names, instead of repeating Core/Entrypoints above every title.
  The entry arrow is wider than the border it stands on and has a thin halo
  in its card's colour, so the border stops at it and its shaft reads as a
  shaft. The legend draws the same arrow.
  An area's mark is its GroupsIndex container's (READING): the core mark
  when any part in it is the domain, the area holding the program's entry
  included (owner, 2026-09-27: marked when any part is the domain); the
  entry mark only on the area holding the program's entry (a declaration
  its execution starts from, a target seed) when no domain part stands in
  it; otherwise none, or the dependencies mark. Parts that only take
  requests or listen do not make their area the entry. The same holds for a
  part: only the part holding the program's launch point has the entry
  mark; a part that only takes requests or listens has none, and its
  inputs' blue arrows say where the outside calls in. The component
  reference's "Input responsibilities" list holds only that part. A launch
  point no part holds (such as a `main` undecided between two parts) makes
  no entry part and no label on the canvas; the component's heading and
  reading and its "Not on the map" list name it as the program's entry
  with its off-map reason, the heading and reading by its name and its
  file:line anchor, as the page's other source links are written. The
  green entry mark on an area that also held a domain part (4a6892a4) was
  an agent's rule and is reversed.
  Concrete input captions remain. A dark outline identifies the card open on
  the right without inserting another row or changing its text position. Dark arrows and outlined participants
  identify the currently emphasized connections: a participant takes the
  arrows' dark on its border, and the card open on the right keeps its
  heavier outline. Nothing pointed at is greyed: the subject takes the
  existing dark (a part its outline, a frame its border at the arrows'
  2.5px), the parts across its dark arrows take the same outline, and a
  pointed frame's own parts and the arrows between them stay as they are.
  Hover highlights and never dims (owner, 2026-09-28): only the reader's own
  choice (a chosen part, frame or declaration, a pinned input path, search
  results) recedes to 40% opacity every part, frame and arrow it does not
  involve. The pointer, or an open arrow end, changes nothing else: what it
  outlines and darkens comes forward, and every other part, frame and arrow
  stays as it is with nothing pointed at, so crossing the gaps between tiles
  flashes nothing. This replaces the rule that every emphasis, hover
  included, receded what it did not involve: moving across Redis's Data type
  commands receded every other part on each tile and restored them in the
  gaps, and the area flickered. Frames grouped under one destination's
  text recede with their heading when none of them is involved and stay when
  one is. No veil or tile fill marks the pointed thing.
  Component frames retain their language, kind, role and purpose above their parts. Colour never replaces
  the visible type cues or independent fact/model provenance in the reading
  panel. Text has at least 4.5:1 contrast; meaningful frames and connections
  retain at least 3:1 contrast, including non-selected neighbours; only
  what a chosen emphasis does not involve recedes below it while that
  choice lasts. A grayscale
  screenshot and actual browser colour measurements cover this palette; they
  are not a claim of full WCAG conformance for the entire report.
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
  automatically. Search and chosen destinations reveal their result at a
  readable scale, opening its enclosing frames in the fixed world. A hidden
  child's bounds inside the viewport do not count as visible content. A fully
  visible, legible part keeps the current camera when opened from a reading;
  so does a frame (a component, an area, an Inputs collection) that is drawn
  and mostly in sight, and a declaration whose tile is drawn in sight (owner,
  2026-09-28: the camera moves only to what is out of sight; otherwise the
  canvas marks it).

- The drawing's dark arrows and outlines have exactly one reason: search
  results, the part or frame under the pointer, the pinned input path, or
  the selected part's neighbours. A pointed part is the subject itself: only its own arrows
  darken. A frame's title, border and empty space look at the frame, whose
  arrows crossing its border darken.
  A pinned input path is drawn from its saved reach (GroupsIndex, READING)
  with the existing dark emphasis: one arrow for every pair of parts where a
  call (or read) of the reach enters a part from a part reached earlier, at
  a lower depth; none is chosen by length, every such call is listed in the
  reading and the others are counted. An arrow is dashed only when none of
  its calls is exact. A caller off the map stands for the earlier parts that
  reach it through code off the map. Other calls among the same parts stay
  ordinary arrows. GroupsIndex marks an arrow quiet
  (`Connection.Quiet`, READING), and the canvas and the static zone picture
  draw a quiet arrow only while one of its ends is looked at; the page
  decides nothing of its own. Quiet is initialization (a relation whose
  source only the target's seeds reach, no input's handler) or a call into
  a helper (a declaration the helper question decided serves the work of
  others; quiet even on an input's path, since every command handler calls
  its reply helpers), and only in a target that serves something. One
  exception, defined once over each target's own connections: when every
  one of them would be quiet, its calls into helpers are drawn, so quieting
  them never empties a map. One arrow drawing several relations is quiet
  only when every one of them is. The page's one figure is the system
  canvas; the page writes no static picture of it.
  Hover temporarily replaces the dark emphasis; it never unions another
  area's edges into a pinned input path. What the chosen path or selection
  recedes stays receded under the pointer, except what the pointer
  highlights. Leaving the canvas restores that path or selection. Reading an off-path part does not make it a path participant;
  the reading card explicitly says it is outside the saved input path. Search does
  not mix old selected or hovered connections into its matches.
  Ancestor frames retain a neutral outline while their descendant is in focus;
  this containment context never adds the ancestor's other connections. Frame
  titles retain a light background and readable role/purpose text. The duplicate
  `Reading` line and close action do not occupy space above the canvas. That
  space has the map's one key, not changing hover prose: each kind of card
  drawn as a small card in the fill and border the canvas paints it with,
  its mark on its border, then the solid "calls" and dashed "possible
  calls" strokes the arrows are drawn with, and, when a part's tiles draw
  one, the dotted slate link from a function to the type it returns or
  from a type to the function taking it ("returns or takes a type"), drawn
  in the tiles' own slate and grey head. There is no folded prose legend
  under the map. The input context
  and leave-path action live in the reading card. An input chosen from
  Find, a link or a reading is entered as its path, while the reading
  column reads the input; its own tile clicked on the canvas is read with
  its path pinned and the camera staying, as every canvas click reads
  without moving (owner, 2026-09-29: slaveof's tile had flown the camera
  to its path). The camera takes the part
  holding its handler, then each part the trace reaches from a part already
  taken while all of them fit at a scale where their headings stay about
  twelve pixels (and their layer open), and frames them; when the next step
  does not fit, the camera stays at that scale and leans toward it, keeping
  what it took inside, so the dark arrows leaving the frame show the way.
  The path's parts, and a closed frame standing for parts hidden in it, are
  outlined in the dark of the path's arrows. An
  input without a trace is entered as its tile, and such a tile clicked on
  the canvas keeps the camera. "Show input" stands in the reading card while
  the camera may be away from the input's tile (on its path, or on a part
  read since) and frames that tile among the inputs its handler's part
  takes, the group it stands in, never the whole collection. A group
  larger than a readable camera is entered at the tile. Once the tile is
  framed there is nothing to return to; choosing the tile again returns to
  the path. An input's reading
  names its handler ("handled by getCommand"); the name reads that
  declaration in its part when the part lists it, and a modifier-click
  opens its code. An input whose handler is not established says so
  ("handler not established") and where it is declared: "declared in" the
  part its call is written in, and the call's source link; some code acts
  on it, and the facts do not say which yet. A
  registration the model did not explain has no line: its given text
  ("kvd.h.kvCommand.proc get in getCommand") only restates the fact, and
  the reading and Find name its handler. A chosen input's
  reading opens at its path (owner's choice 3c, 2026-09-27), drawn in the
  Inputs blue of its tile and collection, never core's rose: its
  heading's bar and kind and its links. The path projects GroupsIndex's
  saved reach and dispatch sites; the page walks no code. It is, first, one
  box per dispatch site whose alternatives hold the handler, the first open:
  "Dispatched from processCommand · one of 6 handlers", holding the
  dispatch fact and the statement "How a request for get gets to
  processCommand is not established.": how a request for the input
  arrives at the dispatch site from outside. No route to the site is
  drawn. The way to its program's Main flow is the column's one "Main
  flow" link above every reading of the component (below). The inputs whose own code reaches a site are not listed in a
  dispatched input's reading, where a reader takes them for its route;
  they are the
  site's own reading, with the declaration: "{site} is reached from these
  inputs:", each input a button to its reading with its calls to the site,
  then "Which of these, if any, leads to an input dispatched here is not
  established.", or "No input reaches processCommand by calls". Every
  count says what it counts, from the page data GroupsIndex's sites give:
  a site's "one of N" counts its alternatives as handlers, or, when some
  alternative handles no input, as functions with how many of them are
  handlers; its inputs are counted as inputs ("N inputs dispatched here"),
  and the site's reading names each handler several of them share
  ("{handler} handles 2 of these inputs: {a}, {b}"), which the dispatched
  input's line shows on hover. An
  input whose own code reaches a site says so in its reading, as its
  handler's own call back into the site ("{input}'s handler itself calls
  {site}, where {n} inputs are dispatched:") with those calls. An input a
  running declaration of another input's reach hands over reads "Registered
  by" with those inputs; the other reads "Registers". Then the parts the input
  enters, nearest the handler first: each part's name (a link to its
  reading when the map draws it), the handler under the part holding it
  ("handled by getCommand"), every call entering it from a part reached
  earlier, the first five and the rest folded under "+N", and "{n} more
  calls into this part come from other code on this path": calls into it
  along the path from parts reached no earlier, none of them the
  handler's. The parts the handler calls directly (depth 1) stand open;
  every part reached deeper is folded under one line, "Reaches {n} more
  parts deeper", which opens them as they are. The fold is by depth
  alone, the handler's own calls against the rest: it chooses no route
  and drops no call. A call is its two declarations' names,
  with no line number, a read or a possible call marked as elsewhere; a
  name in a drawn part reads that declaration there, as a click on its tile
  does, and a modifier-click opens its code; a name in a part the map does
  not draw is only named. The words "Shared by" and "through" are gone.
  The list of the parts on the path is left for an input without one.
  A part read while an input is pinned says "Outside this input path" in
  its heading when neither it nor a part inside it is an end of the path's
  arrows. The line is drawn with the reading, from the reading's own state,
  never from what the pointer is over. Nothing in
  the column moves after it is shown except by the reader's own action. An
  up-chevron in
  the reading card's own header closes details, preserving camera and any pinned
  input path; leaving that path remains a separate action.

- The frame being looked at marks its arrow ends: hovering an area or a
  part inside it marks the area's, and pointing anywhere in an open
  component, its own space, border or title included, marks the
  component's, as choosing it does. A closed component marks nothing and
  does not take the marks of an open one beside it. Nothing on the map is
  a digit (owner's choice 2a, finished 2026-09-28): there are no part
  number badges, no digit chips, no key line explaining numbers and no
  numbered/arrows switch. The digits named members in member order and
  changed with the pointer, and the owner saw them everywhere.
  Each arrow end on the looked-at frame has one small plaque where its
  arrow meets the frame. It reads "all" only when the parts behind the end
  are every part of that frame (an area's parts, or a component's areas
  and loose parts), and only for a frame of more than one part; otherwise
  it is a plain handle with no text, the size and colour of the former
  one-digit chip. One outside participant stands behind one plaque on a
  side: both directions between the frame and it, a two-headed arrow or
  two opposite arrows, share the incoming direction's plaque, which stands
  for the parts behind both (two "all" plaques side by side read as two
  things). Different outside identities remain separate. Resting on a
  plaque opens its card (the incoming direction's when it stands for two,
  whose "go the other way" link opens the other); selected details and
  their links stay present. The plaque's accessible name is the outside
  participant; the reading column lists it too.
  A line has no click target or native tooltip; its arrowhead does (below).
  The same real endpoints remain connected across zoom levels.
  While an end's card is open or kept open, the parts behind that end take
  the dark outline in place (a closed frame hiding them takes it for them),
  the end's own arrows are dark and its plaque is dark; like the pointer,
  the end recedes nothing, and the parts at the arrow's other end stay as
  they are. A plaque stands where its arrow meets the frame it marks: both
  directions of a pair of frames share one drawn route, and the end one
  direction took could be the other frame's.

- A card (a label's calls) opens on intent: the pointer rests on its handle for about a
  tenth of a second, so a handle crossed on the way elsewhere opens nothing.
  A connection has two handles, its plaque and its arrowhead. Every drawn
  arrowhead of an arrow meeting an area's or a component's border is one,
  at every level and whether or not that frame is the looked-at one: the
  head's own few pixels on empty canvas, found from the drawn route, with
  nothing drawn for them and no competition with a title, plaque or
  summary under the pointer. A head stands for the incoming connection of
  the area or component it points into; a head on a destination, the
  inputs or a loose part stands for the outgoing connection of the frame at
  the arrow's other end; a head beside a plaque of the looked-at frame
  opens the card of its own direction. A head of an arrow between two parts of one frame has
  no card.
  It stands flush with its handle, outside the frame being read so it covers
  none of that frame's parts, and clear of the handle itself, on the side
  with room, and wholly inside the canvas; a label's card goes out through
  the border its label stands on. An arrowhead stands outside its frame's
  border.
  With no room outside the frame it stands beside its handle toward the
  roomier side. It stands in the map's coordinates and moves with it, but
  at the screen's own type size, and is placed again when the zoom
  changes. Beside a frame it takes the room
  there is, from 300 to 500 px wide, so that it stands outside the frame;
  with less room it keeps its 500 px and stands beside its handle. While a card is open or kept open, the frame being read stays: the way to
  the card crosses other parts, frames and empty canvas without changing
  the emphasis or the labels. Leaving the handle, the pointer is safe inside
  the triangle between where it left and the card: the card lasts and no
  other handle on the way takes it; from an arrowhead onto its own card or
  plaque the pointer has not left at all. A click on a card keeps the card
  open; a click on an arrow end, its plaque or its arrowhead, reads,
  in the column, the frame whose connection it is, scrolled to that frame's
  Connections with that connection open, and leaves the camera where it is
  (owner's choice 3b, 2026-09-27).
  A kept card has a ✕, and ✕, Escape or a click on empty canvas
  closes it. That click closes the card and nothing else: it neither
  selects nor moves the camera. The card's list scrolls under its sticky
  heading and ✕. The dwell, the linger after leaving and the triangle are
  interaction timing, tuned on recorded pointer paths, not evidence limits.

  A label's card (owner's choice 1b, 2026-09-27) is headed by the two frames
  its arrow joins, from → into, and one line of counts: how many calls of
  each kind, from how many parts of the one frame into how many of the
  other ("291 calls, from all 8 parts into 8 of 9"), and how many go the
  other way, a link that opens that direction's card. Its calls stand under
  the part they are made from, the part with most calls first, then under
  the part they go into; both headings stay at the top while the list
  scrolls. A call is caller → callee, the caller a link to where the call
  is written and the callee to its declaration, in the order the call
  sites are written; a caller is written once for its run of calls, and a
  caller calling one callee from several sites is one call. Kept open, the
  card also shows on top an index of the parts at each end with their
  counts; a part at the calling end leads to its calls. The page data marks
  the calls of a dispatch site whose adapter retained several alternatives
  (kvd's processCommand reaches `cmd->proc`, one of six command functions),
  and the calls of a declaration that hands every member of such a set over
  by another relation (cmdTable passes all six as callbacks); each caller's
  marked calls are one line (on an arrow into a part holding four of them,
  "processCommand → one of 6 · 4 here", "cmdTable passes callback the same 6
  as processCommand · 4 here"), with the callees under it by part, each part
  opening to their names. Which calls belong to a set, and how
  many the set holds, is decided in Go from the relations' retained targets;
  the browser only counts what stands behind the arrow. A caller calling
  every member itself, or handing over part of a set, keeps its rows. A
  label's card always shows the whole end.

- ELK prepares native area interiors, then places each component's ready
  immediate child rectangles in a flat graph. These children retain their
  interior coordinates and routes through translation; directed connections
  between children are bundled by directed visible endpoints. Original certainty
  and source relations remain distinct in the reading data.
  The component frame wraps the actual placed contents with its measured
  header and insets, without aspect padding around a compound column.
  A separate flat ELK call places the participant rectangles and routes their aggregated
  outside connections. Uniform transforms place their interior drawings in
  one fixed world before setting the viewport. Outer arrows stop at the
  participant frames even while their contents are open; continuations from
  boundary ports to inner parts are not painted. At the outer participant's
  boundary, a plaque stands for the explored area's actual inner parts behind
  it; shared participant labels retain every matching part and source.
  The original endpoints, possible status and source relations remain available for
  reading and operation paths. Aggregated outside strokes are drawn once; they
  are display geometry, not new semantic relations. Each area's interior is
  laid out alone, from the arrows between its own parts; arrows between areas
  are the component's bundles between ready rectangles. The two directions of
  a pair of ends share one drawn route, so ELK lays out one edge per pair.
  That edge is the pair's first.
  Each area takes, of ELK's directions (rightward, downward) with and without
  wrapping a long chain into rows toward the canvas proportion, the layout
  that fits the initial canvas while its 17px part headings stay at the 12px
  their layer stays open at; then the one whose routes run shortest (the
  fewest detours around the area); then the squarer box. Its parts keep
  their own size, the size of the loose parts beside it, so an area is as
  large as what it holds. Each area wraps its drawing and heading without
  reserving a second member-list height. A closed group
  shows its name and nested-content hint, then directly reveals its actual
  objects at the next common layer. There is no intermediate member-list view.
  An architectural area always holds at least two parts: reading draws no area
  of fewer than two and GroupsIndex keeps no container of fewer than two
  groups, and every group is a node of its component's map, so the page draws
  every area it is given as a frame. Participant, input and external
  destination frames keep their distinct boundary even with one child. Component summaries list their actual immediate areas, in the
  order the model listed them, then their loose parts, so folding a wrapper
  never removes its responsibility from the overview. The component frame's
  children carry that order to its overview list and into ELK's input; ELK's
  placement still follows the connections. The parts entrance below the map
  lists each component's areas the same way, then its loose parts by name.
  That overview keeps the saved area caption; the single card and its source
  reading keep the part's own title. A direct (loose) part beside areas is
  their closed summaries' peer while they are closed: it uses the same fixed
  first-reveal heading fit and column width as neighbouring groups, without
  promising hidden children. Its box is its own card's, whatever the areas
  beside it hold, so on the component overview its title reads smaller than
  theirs when their boxes are larger. This is left. The camera caps every
  closed heading at a scale the whole map sets, so matching the cap needs
  the interiors laid out again once the whole map is placed. Once the areas
  open it is their parts' peer: the same card at the same scale, filling
  its box, so its title reads at their parts' size. A component without
  areas keeps its direct parts' fitted headings.
  Input collections remain outside the component at every scale.

  Detail is synchronized by hierarchy depth. The whole-map layer shows targets,
  external participants and input collections. When the first participant's
  contents become readable, every participant reveals its next level. When the
  first area's contents become readable, every area at that depth reveals its
  objects. The trigger uses the largest child heading at that depth: 14px to
  enter, 12px to retain on retreat. The first layer also opens when any root's
  longest screen dimension reaches 75% of the corresponding canvas dimension,
  retaining it down to 65%. Whole-map fit always keeps summaries. Pan does not
  change layers; Back restores the common hysteresis state. Smaller siblings
  never delay or independently close the layer. No camera coordinates, box
  dimensions or routes change at a detail switch, and no layout runs during zoom.

  Summaries lay out their complete text against their own frame, using the
  whole-map reading scale as a fixed reference. Pan clips the card at the
  viewport edge; it never squeezes or rewraps text into the remaining visible
  sliver. Zoom scales that fixed text layout. Compact area labels start from
  the first common entrance scale, then fit their complete names and hint to
  each actual group rectangle once. Smaller groups retain smaller text instead
  of becoming blank; zoom enlarges that same layout without hiding or rewrapping
  it. The native frame paints the outline at both detail levels; the title
  overlay adds no second, magnified border. Hover and selection change outline
  paint only, never border geometry, padding, title position or wrapping.
  Nothing inside a card moves when an area is looked at or chosen.
  Closed frames display a subtle outlined child element with a question mark
  as their nested-content hint. The accessible action and the card both enter
  its contents. A toolbar hint explains zooming and dragging. Leaf cards and
  open frames do not promise another hidden layer.

  A part with declarations has a zoom button; zoomed far into the part, its
  declarations stand inside its own card as tiles, each a declaration to
  choose: a type with its fields and methods under it, a function or a
  module's variable alone. A tile keeps its link column, the longest chain
  of calls, returns and takes leading to it, so a caller stands left of what
  it calls. Tiles stand by file, the files in the order their first
  declaration is listed and each file's declarations in the order the page
  lists them: the model's keys first, then the types, then the rest, so a
  part's data stands before the code that works on it (owner, 2026-09-28);
  the file is a hint. They
  stack in that order within their column and a column too tall spills into
  the next. A tile is as wide as the longest name among the part's
  declarations, so no name is cut, and the columns share the card's width.
  The tiles are
  drawn at a quarter of the card's scale, or smaller when a quarter does not
  hold them all whole, however many the part holds: nothing is counted away
  and no scale is too small to search, and a large part is a larger
  drawing to pan. The
  part's name stands over them at the same screen size at any such scale.
  The zoom button
  enters at the scale the declarations read at, their own 13px: the part
  whole when it fits there, else its head and first column a margin from
  the canvas's top left. A drag over the
  declarations pans the map, as a drag over the part does; a drag that
  moved chooses nothing.

  Pointing at a declaration darkens only its own arrows: its links to the
  part's other declarations, and among the part's arrows those carrying a
  call into it (the call's callee is its source) or out of it (the call's
  caller is its name); nothing recedes. A
  click on a tile chooses that declaration: the part is read with it named
  in the reading column (the existing `map.explainSource`), the tile keeps
  the dark outline of what is read, its own arrows stay dark, and the
  declarations its links do not join recede while it stays chosen; the
  camera stays on a tile in sight. A modifier click still opens its code. A
  declaration the reading column names by its source (Find's code hit, a
  declaration chosen in the reading, a restored visit) is the one chosen on
  the canvas, and a newly named one out of sight is shown with its part: at
  the zoom where the part's tiles are drawn and the part stands whole across
  the canvas, framed whole when it fits and else across with the tile centred
  down it, never deeper than the part's own title fits (owner, 2026-09-29: a
  name chosen in the column had zoomed to the tile's own size under giant
  cut titles); a restored visit keeps its camera. The page data gives each tile its file and the
  same source link its reading uses, served or static.

  Root summaries prioritize saved area names over role, counts and purpose;
  complete dense inventories scroll without dropping entries. Ordinary wheel
  scrolls an overflowing inventory, while pinch passes through to the map;
  past the inventory's end the wheel stays with it. Anywhere else on the
  canvas, its location row over the map included, an ordinary wheel moves
  the map and never scrolls the page. One pinch
  (ctrl+wheel) crosses at most one level boundary, the zoom where the
  level changes, and stops short of the next, going on or back; a pause
  of about a third of a second ends it and the next pinch crosses the next.
  The levels are the whole map, then one per open hierarchy depth, then a
  part's tiles in sight; two layers that open at one zoom are one
  boundary. The zoom a tick
  asks for is React Flow's own; only a tick that would cross a second
  boundary is held at the last zoom short of it.
  Secondary purpose text uses remaining complete lines, with the full text in
  the reading column. Text and controls remain inside their own frame; viewport
  clipping does not move them to a different visible corner. The persistent
  location row names the visible item and its known ancestors when its world
  header leaves the screen. Pinch uses the gesture's actual aim for that context;
  an explicit entrance names its destination instead of a neighbouring frame
  at the canvas centre. Root summaries and revealed interiors are exclusive.

  Between-group arrows stop at group boundaries whether the groups are open or
  closed. Connections wholly inside one group reveal with that group's objects.
  Multiple relations sharing a directed visible pair use one existing native
  route; it is dashed only when every original relation is possible. A call
  left unresolved whose store witnesses name its candidates is possible
  toward each of them (READING, Operation ownership), the same dashed arrow
  alternatives draw, and its rows read "possible" in the source details
  like theirs, in the muted text colour; the key's dashed stroke says what
  it is. All original endpoint IDs and sources remain on it;
  the drawing does not invent another relation. Plaques sit just inside the
  frame, centred on the native connection point.
  They do not encode execution order. Floating-point offsets cannot create diagonal
  clipping segments. React Flow receives the known native measured dimensions
  of nodes on every presentation update so that handle recalculation cannot
  temporarily remove the arrows between animation frames.
  The initial overview fits all root component and communication frames. The
  source SVG and unpositioned React drawing stay hidden until both the fixed
  world and camera are ready; a loading indicator appears in the reserved canvas.
  Workspace height accounts for the
  actual header and controls. Whole-map mode remeasures the outer layout when
  that space changes size, reusing every prepared interior and then fitting
  the new bounds. Initial placement and
  resize use the same inner canvas dimensions. Manual pan/zoom ends that mode
  and keeps the reader's world and camera; choosing the whole map again may
  remeasure for the changed space. A pending resize layout cannot replace a
  world after a manual gesture. Saved whole-map intent survives a changed
  geometry identity. Component entrance fits the complete participant frame
  before exploring a group, keeping siblings on both sides in view. Entering
  a group or a call then uses its actual content scale for reading. A
  focused area that fits the canvas at a zoom where its parts' headings stay
  about twelve pixels (and its layer open) is fitted whole, never wider than
  the visible canvas; only an area too large even so is entered at its
  first part at that scale. Entering a frame or an input's path
  opens what it enters: those frames stay open through the camera move's own
  zooms, so they arrive open although the camera stands smaller than a
  closed layer needs to open by itself.
  The minimum camera scale permits the complete root bounds even in a short
  window. The initial world choice accounts for readable root-header widths and heights,
  not only its bounding rectangle. After placing the native area interiors,
  narrow root frames reserve overview text width before the final layout is
  shown. That initial placement measures headings at the fitted screen width,
  bounded below by a whole word's readable width. Small area lists reserve their
  complete height; a list taller than the available canvas reserves its first
  entrance and keeps every remaining entry in the existing scrollable frame.
  Preferred component width also measures two-line area-name entries within
  the existing text column, so a short component name does not squeeze its
  inventory into isolated words. This is a text measurement, not a fixed wider
  frame applied to every participant.
  Fitting a full oversized list must not enlarge the world repeatedly and
  collapse its fitted width. Long words reserve enough width; a narrow
  heading can continue below its zoom mark instead of breaking a name midway.
  The fit that sizes those reserves frames exactly what the whole-map camera
  frames, a display group's frame with its padding and heading band
  included.
  A title wraps only between words, the way the browser wraps it: closing
  punctuation stays with the word before it and an opening bracket with the
  word after it, so no line starts with ")" or is ")" alone. A word breaks
  inside only when it alone is wider than the whole line, after a separator
  or between camelCase words when it can, and still never before closing
  punctuation. Map titles are drawn as those measured lines and the browser
  never breaks a word of them again. A heading whose room is still short of
  its longest word shrinks its type until that word fits instead of breaking
  it; a part's title leaves room for its zoom button. Card text is measured
  at the card's real text column, its width less its border and padding, and
  less its zoom button's room beside a part's title. A part's description
  is not broken into lines on the page; the browser wraps it in that column, and its lines counted there only size
  the card, at most three, the third cut with an ellipsis. The description
  is held to that column's width, so a browser drawing the border thinner
  than its width cannot fit whole a line the count broke and leave the card
  an empty line.
  When the whole-map fit still cannot give a summary its reserved width or
  height, in a short window or on a crowded map, the summary is laid out at
  that reserve and drawn scaled down whole, its zoom mark with it until zoom
  gives the mark its ordinary size, as a small group keeps a smaller complete
  label; a frame too narrow for its text at full size keeps that smaller
  summary instead of standing blank beside a display group's plain tiles.
  External width minima allow that arrangement; height reserves the actual
  wrapped heading and nested-content hint with their insets, without an unrelated floor.
  Orientation comparison uses those measured minima before world size.
  Compact component purposes use the remaining whole lines, with an
  ellipsis when shortened, and stay hidden if fewer than two lines fit; the complete
  purpose remains in the reading column. External frames reserve summary space.
  The fixed world places frames, parts, complete input cards, component
  purposes and grouped labels. Routes inside one group retain actual part-to-part
  endpoints; routes between groups and participants stop at their boundaries. ELK chooses the actual endpoints. Plaques sit just inside those
  native endpoints without moving them. There is no additional boundary route planner, A* layer, custom marker
  packing or replacement path. Ordinary and natively unzipped outer layers are
  compared in both directions with free endpoints and with the prepared native
  ports: eight flat candidates, ranked by fitted text readability then world size.
  If the selected fit would shrink component inventories, collection headings or input types below
  their measured minima, a final placement of that same native shape reserves
  the missing physical space. If ELK changes the packing after that growth and
  the measured text still does not fit, one further sizing pass uses its actual
  resulting positions. There are at most two correction passes, both before
  display. Their in-memory reserve follows nonoverlapping
  projected chains, so parallel rows do not count their widths or heights
  repeatedly; it also protects initially readable roots from the smaller final
  fit. Fixed-side ports follow their new boundary. One uniform transform fits
  each prepared interior to its final participant rectangle, including its
  cards, text and native routes; the prepared drawing remains unchanged. This is a bounded
  initial sizing correction, not another orientation search or a zoom-time layout.
  Unzipping is not forced on small maps. Cross-participant labels reserve no
  interior space: their plaques belong to the outer endpoints, so their
  former title corridors cannot push real contents away from a pinch target.
  Each component compares ordinary and natively unzipped placement of its
  ready child rectangles against its own measured summary aspect. Every
  original edge ID and relation survives the display bundling. Each input collection also compares
  the ordinary and natively unzipped interior against its own measured summary
  aspect, avoiding empty padding around a long column while retaining compact
  short catalogues. Area interiors retain the ordinary layout
  options. Browser calculations run in a real Worker embedded once in
  the self-contained HTML and reused throughout the mounted report. Worker errors
  reject pending calculations. An initial failure leaves the ordinary report
  available; a later failure retains the last complete world and offers reload.
  Neither case leaves a permanent loading state or retries on the main thread.
  Orientation candidates receive independent graph
  objects: ELK mutates its input, so reusing a computed candidate can retain stale
  bends. React Flow owns pan/zoom and camera restoration.
  Emphasis darkens and thickens a line, never its head: every arrowhead is
  an ordinary one's size, as is every head of a link between a part's
  declarations.
  Connections retain their screen stroke, casing, dash and arrowhead sizes
  through React Flow’s ancestor transform. Frame outlines use inset paint so
  browser minimum-border rounding cannot turn a fractional world stroke into
  a thick close-up border; corner radii also stay at screen size.
  Fixed overview text scales with the viewport independently of graph props.
  Ordinary camera movement does not rebuild the graph or remeasure unchanged
  text. Only common detail-layer transitions update the displayed contents.
  Selection and hover never change box positions or sizes. The first click
  reads and zoom is separate (owner, 2026-09-28): a click on a part, a
  frame's title or a component's, a collection's or a destination's whole-map
  card, an area named on a component's card, or an input's tile updates the
  reading column and marks it without moving the camera;
  the magnifier, "+" and a double-click zoom. Explicit destination clicks
  and Find move to their exact result when it is out of sight.

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
  part is centred in the canvas; a part taller than the canvas shows its head
  a screen margin below the top. The margin is screen pixels. A
  regenerated layout invalidates old camera coordinates and reveals the selected
  item instead. A navigation, click or camera move cannot trigger a different
  hover emphasis under a stationary pointer; real pointer movement resumes hover.
  Viewport history includes its zoom, open areas and the fixed world's geometry identity.
  Every section and source link stays reachable from the page.

An input path is its saved reach: GroupsIndex follows native call/execution
edges (READING) and the page draws what it saved. Reads of declared values or
types by its reached code appear as terminal data dependencies, with a distinct
read label. They never execute a data owner's other methods or activate a
stored callback, integration or mutation. Every call entering a part from a
part reached earlier explains that part; imports and part membership alone do
not establish a path. The path is not a neighbourhood: its arrows are those
entering calls' part pairs, and its reading lists the parts by call depth from
the handler, then by the first declaration reached in each, with a part
reached through a matched input after them. Call depth is a
static measure, not a recorded execution order. All original structural
relations remain available. With GET pinned, a part's card says "Why it
appears in get" with the calls entering it and the count of the others,
never a single shortest path.

The key lists only the kinds and strokes present in that map.

The reading column (owner's choices of 2026-09-28, the designer's variant A
with his changes) renders page data Go prepared and sorted: the script
sorts nothing and repairs nothing.

The page's data holds every answer, and the page needs JavaScript to read
it (owner decision 2026-09-29, "b"); a `<noscript>` line says so. Nothing
the reading column renders from that data is also printed as HTML: a part
has no printed card (its reading, its code in this part, its connections
and its source index were each written twice, in both parts' cards for a
cross-part row), an input has no printed catalogue row or state changes,
the canvas has no static drawing, and a glossary term's files and lines
are written from the data when "Files in which this term appears" is
opened (litestream's 2.2 MB of printed links). A part's page anchor (`#t1-g6`)
stays, an empty element naming its map node, so links to it read it. The
data is written once in one `<script type="application/json"
id="rm-page-data">` (`page_data_table.go`): each value, each declaration and
the links' base once; a link its place says ("redis.c:9068") as 1 and a
declaration's link to all its lines as its last line; a call's ends as
declaration indices with its words dropped when "caller verb callee" says
them; a tile's declaration by index; a reading's call kind by default; and
any part written again elsewhere once in `shared`, referred to as
`{"$": index}`. The script (10-ui.js `rmPage`) reads every value back
exactly as Go registered it, and a test reads the compact data back through
that script on GitHub and GitLab links. An arrow whose calls say its words
carries only the relation an input's arrow reads by ("implemented in"), not
the joined words of every call. Model text is told apart by its style
alone (italic, a hover "written by the model") with no chip before it; a
part named anywhere in the column stands in a small box drawn as the
canvas draws that part (its frame, core's rose with its diamond, entry's
green); lists are plain names, a model's key in bold, with no square or
chip before a name. Every name the column reads is shown on the canvas too:
a declaration's tile is chosen, a part, a component or an input is read
and marked, the camera moving only when it is out of sight, and the
address follows it (a new declaration in the same part is a new visit, so
Back returns to the one read before). A modifier-click on a name opens
its code.

A part's reading (`pageGroupReading`, `data-reading` on its card) has its
kind and the frame holding it, a link up, in its heading ("Part · Core
infrastructure ↑"). "Called from" lists the parts calling into it, each
in its box with its caller → callee pairs counted as the arrow's card
counts them, and under each caller the declarations of this part it
reaches, calls first and then the other relations in their own words
("beforeSleep() passed as a callback"), each list by name; every input
registered at the part is one neighbour, Inputs, counted by its inputs.
Many callers fold each part to its line. Then the part's own box, the
model's description, its files, and its declarations by kind, each kind
its own heading and list by name whatever its case ("12 functions",
"4 variables", types), the keys in bold. "Calls into" lists the parts it
calls, each in its box with its callees by name and the variables it uses
there. With an input pinned, the input's witness stands under the
description ("Why it appears in {0}"); the inputs reaching the part and
the state changes of its types close the reading, folded under their
counts. Every original row and possible-call mark is in the reading's
relations, each end by its name alone, and every source pair on the
arrows' cards; no card of the part is printed.
A model's sentence naming no two declarations stays on the arrow's card;
the original native relation is in the reading even when the model
supplied no sentence for that pair. Membership alone
adds no relation; containment remains the source inventory. Distinct
relation IDs preserve same-line call occurrences, and on the card
connections share one heading per exact participant href and direction,
with each distinct saved summary once.

A declaration chosen in the reading, on its tile, from Find or from
another declaration's reading has a reading of its own in place of its
part's: its kind and its part, a link up, in the heading ("Function ·
Server lifecycle and cron ↑"); who calls it ("Called by", "Used by" for a
variable or a type), grouped by the part at the other end, its own part
first; its name as the one link into its code with what its tile writes
after it, underlined under the pointer and marked by a small "</>" in the
link's own colour (owner, 2026-09-29: readers had taken it for a title), then its file alone ("redis.c") and, when its author wrote one,
the comment as written, standing in the reading and marked as the
author's claim; the model's line when there is one; what it writes and
what else it reads, one line each ("Writes: …", "Reads: …", below); a
type's every field with its type, a link reading that type when it is the
repository's (ProgramIndex `Object.Types`: `listNode *head` reads
listNode), and the functions of its part that return or take it; a
function's flow; and what else it relates to ("Calls", "Uses"), by part.
No name in the column carries a line number (owner, 2026-09-29: "человек
будет видеть код"): a caller, a callee or a variable is its name, a link
reading its declaration, once however many places the relation is
written, a part named on its hover; the declaration's own name is the
link to its code. The same holds for a catalogue's callers ("called from
processCommand") and the Inputs reading's "How these were found": a
function making undecided calls is named once per symbol and reason, and
one with calls the code cannot follow once, with their count. The page
data keeps no place of such a relation.

Who changes a field and who reads it (owner, 2026-09-29) is read from the
program's exact reads and writes of record fields (`page_field_uses.go`):
a C relation with its `field_path`, a Python typed receiver's field and a
JS/TS declared property, whose words are then `Type.field`. A record
type's reading gives each of its fields, under its row in the fields
grid, "Written by" and "Read by": the functions writing and reading it by
any path, each side grouped by the part they stand in, in the part's box,
its own part first, then the part naming most, each by name and each
once. A global variable's reading lists, in the source order of each
path's first field, the fields as the code reaches them through it
(`server.masterhost`, `server.db.expires`: a path whose root is its name,
made by a function that reads the variable itself) with the same two
sides. A function's reading has one line "Writes: server.masterhost, …",
the fields it writes in the order it first writes them, each name reading
the type that declares the field. A declaration's reading has one line
"Reads: redisClient.argv, shared.czero, …" in place of the "Uses
variables" list, which had printed `c->argv` as `argv` with every line
using it: the fields it reads, each by the path the code reaches it by
(`field_path`: the root global variable, or the record type holding the
chain's first field when the root is a parameter, a local or a call's
result; `Type.field` without one), and the module variables it reads
whole, each once, in the order the code first uses them, each name
reading the type declaring the field or the variable. A field it also
writes stays on "Writes:"; a name leading to a longer path it reads or
writes is said by that path (`server` by `server.dirty`, `server.db` by
`server.db.dict.size`); a local or a parameter is never listed. No line numbers
("человек будет видеть код"); a side of more than twelve names folds
under its count. Each list is one value of its part's reading (`fields`,
`writes`, `reads` on the declaration's own entry, names by declaration
index), written once; a list two readings repeat is written once in
`shared`. Go records no field
access (GO), so a Go type lists none.

A declaration two programs of the report hold is one declaration by
exact identity (`groupindex.DeclarationKey`: path, line, column, kind and
name; never a name alone; `page_shared_code.go`). Its "Called by" lists,
after its own program's callers, the calls each other program's own
reachable code makes into it, each group named by that program
("redis-cli: [Command line client] cliConnect()"), a name reading it there;
when its own program's adapter proved it never runs the declaration while
another program calls it, the reading says "Not called in redis-benchmark"
and its tile is drawn quiet. A call that leaves its program names each
program's side from that program's own code: the shortest run of calls
from the nearest declaration no other program holds to the declaration
making or taking the call ("redis-cli: cliConnect → anetTcpConnect →
anetTcpGenericConnect ⇢ redis-server: acceptHandler → anetAccept"); a call
to an outside endpoint has the one side and says it is outgoing
("redis-server: syncWithMaster → anetTcpConnect → anetTcpGenericConnect →
socket.h.connect outgoing"), and its row in the component's reference says
the program connects out from that run. The shared anet pair alone had
named neither program.

A setting a compared word declares (a configuration directive,
`strcasecmp(argv[0],"slaveof")`) is read by its key as the code compares it,
its comparison as written linking that line, then "Writes: server.masterhost,
server.masterport, server.replstate": the exact field writes the declaring
function makes on the lines that comparison guards (ProgramIndex pattern
`branch`, `page_settings.go`), each field once in the order written, each
reading the type declaring it; without a branch it names none. What else the
declaring code uses folds under one line with its count ("loadServerConfig
also uses 7 variables"), each variable's other users under theirs, in the
setting's reading and in its collection (owner, 2026-09-29: the Settings
catalogue had read as walls of "uses server, also used by …"). A Go
struct-tag setting keeps its field and tag.

A function's flow (owner-approved 2026-09-29, the designer's flow v2;
`page_flow.go`, `32-flow.js`) is what it calls in the order the calls are
written: GroupsIndex's calls, callbacks, executions and library calls
from the declaration, ordered by their first call site's file, line and
column, each callee once with every place it is called, a dispatch site
one call ("one of 94"). The flow carries no caption repeating the
function's name and no count or meta word. Each run of calls into one
part stands under that part's box (its description on hover, a click
reads it); a library's call (`fork`, `wait3`) is a plain row naming its
library on hover; a declaration no part holds (`lookupKeyRead`) is a
plain name whose own flow still opens, carried in the reading that calls
it, so no call is dropped. A call a macro's expansion makes is shown as
the code writes it: the macro as written, once (C's `macro_expansion`
witness and the call's selector; `assert`, not `__assert_rtn` and
`__builtin_expect`), a plain row naming the header of what it calls
when its body is the platform's; when its expansion calls repository
declarations (redisAssert's `_redisAssert`, dictHashKey's hash functions)
the row opens to them as a call does, its hover saying what it expands
to. A compiler builtin (the adapter's `builtin` package) is never a call
of its own. A twist, shown while a call that can open is
pointed at or focused, opens that call in place to its callee's flow,
grouped the same way; when all of those stay in its part no box repeats.
A call one of its ancestors makes says "↑ shown above" instead. What a
call hands over ("passed as a callback") and where it is written ("called
at redis.c:1273 · 1288") are on its name's hover. A helper call folds
into one muted line under its step, "+ helpers: createListObject,
listAddNodeHead, dictAdd", each name a link reading its declaration as
elsewhere; "+ helpers" opens that step's helpers into rows in place,
lighter, and "− helpers" folds them again. The one quiet toggle "Show
helper calls" opens every step's. No name is hidden (owner, 2026-09-29:
the fold had hidden the answer of three of thirteen benchmark
questions). A call folds when the callee is a declaration the helper
question decided serves the work of others and it stands in a part more
than half of the program's other parts call into (decided from the
calls, never by name). A call into the caller's own part is its work and
stays (rdbLoad's rdbLoadType, expireGenericCommand's setExpire and
deleteKey); a helper into any other part stays, since its part says what
it is for; a step whose every call is a helper shows them as its calls.
What is open stays open across the toggle. A step of the component's Main flow opens in place to its code
flow the same way, the model's sentence kept in its style above it.

The Main flow is read once, at the top of the component's reading; the
component's page keeps it hidden as the column's source, and no other
reading repeats or links it (owner, 2026-09-29: two copies had cost a
reader six actions and five dead ends). Above every reading of a part,
declaration, input or frame of a component that has a Main flow stands one
small link, "Main flow", reading the component at that section without
moving the camera. A step citing a registration of a repository callable,
or naming a callable some registration hands over, reads as that callable,
a name reading it, never as the registrar
(`aeCreateFileEvent`, owner, 2026-09-29), with how it comes to run, from
the program's facts and calls (`page_flow_steps.go`): where it is
registered, the run of exact calls from the most recent earlier step (the
program's entries for the first) to the function making the registering
call ("acceptHandler → createClient registers it"), every registration of
that callable that step reaches, or every one when it reaches none, never
an arbitrary first one (readQueryFromClient's step had linked beforeSleep's
resume path); and what runs it, each function calling it through a value
(a dispatch's alternatives, or an open call whose stores name it), after
the run of exact calls from the entries reaching that function the first
time the flow shows it ("main → aeMain → aeProcessEvents runs it"). The
step links its registration line when it names one. A program no model
flow passes reads forward from its entry: the start list's entry names its
part and key, and with one entry its calls stand open under it in the
order they are written.

An input's reading opens at how a request reaches it: "How a request
reaches get:", then the first way (GroupsIndex's outer inputs of the
dispatch site running its handler, requests first, each by the shortest
run from the callable it hands over, else by its own calls) as one chain
in call order, a line per part, its handler last in its part
("acceptHandler() → readQueryFromClient() → processInputBuffer() →
processCommand() → call() / String commands getCommand()"); the callable
handed over says so on its hover ("passed as a callback by createClient,
which acceptHandler calls"). "Other ways in:" follows on one folded line,
each way named by the part of the declaration where it leaves the first
("from Replication"; another dispatch site by its own part) and "+N" for
the inputs whose own code runs the site; opened, each way is its chain.
Then what its handler does, its handler's flow, a single call opening by
itself while its calls go one at a time; and last, one quiet line naming
who sends it ("redis-cli sends get."), a model's match. The
place each relation is written is on its name's hover; a relation other
than a call keeps its own words. A dispatch site's reading follows. A
variable read by a hundred functions folds each larger part to its line.
Choosing a name reads that declaration, so a chain such as
readQueryFromClient → processInputBuffer → processCommand is followed one
call at a time; equal names with different source locations remain
separate. A declaration without a line says nothing about one.

An area's reading starts under its description with "Made of 9 parts",
each part a link to its reading with its counts and files and its
declarations as links, keys first and bold, the rest by name; a
declaration is read in its part without moving the camera. An area's or a
component's reading then lists its Connections as its arrow ends group
them: one line per frame or participant at the other end and direction,
incoming first, with the count of its calls and the parts they are made
from, each opening to the same rows as the arrow's card; they replace the
list of neighbours by name. A click on an arrow end opens its connection
alone. A line from inputs counts the handlers they are implemented in,
with that unit ("← Inputs 12 handlers"; its card "12 handlers, into 3 of
5"), and inputs sharing a handler stand in its one row, each named ("{a},
{b} → {handler}"), in the column each a name reading that input (owner,
2026-09-29: "sync" and "slaveof" had been plain text); no input is dropped
from it. Inputs taken in where
their handler is not established count as inputs and share one row per
place, named in order ("-a, -h declared in parseOptions"). In the column a
name in those rows reads its declaration in its part, as a click on its
tile does, and a modifier-click still opens the code it linked to; a row's
code is one explicit "Open code ↗" at its end, where the call is written,
and a row wraps with its names whole. The page data names the
declarations at a call's two ends (`caller`, `callee`, keyed as the
reading keys a declaration) apart from where the call is written and where
it lands. A name whose part lists no such declaration is only named. The
canvas's own card keeps its links. An input collection is named Inputs
there, as its canvas heading is. A relation row is said through one
closed vocabulary of the report's UI messages, chosen by its kind and
filled with the two declarations' names ("main passes acceptHandler as a
callback"), never the stored kind ("passes_callback"); a C program's
import is said as an include, and a joint between two programs names the
declarations at both of its ends ("{caller} connects to {callee}", not
"integrates with"). An arrow's card reads each of its calls as caller,
relation and callee and links both names, so an arrow's call is those
three words: the phrase's words when they stand between the names, the
relation's kind when the phrase wraps the callee ("cmdTable passes
callback delCommand"). A row the model wrote in its own words keeps them.
Rows of one caller and one relation kind in one evidence list fold into
one line with their count and callees, each row inside it with its
sources. A list with folds has one "Open all" control that opens and
closes them together. What a reader opens in the reading column (a fold,
Open all, any disclosure) comes into view: the column scrolls by what it
overflows at the column's foot, and no further than bringing its summary
to the column's top; closing scrolls nothing, and a reading restored with
its evidence open keeps its place. The reading column stands beside the
map's controls and key as well as its canvas and takes their height, with
the canvas keeping its own. Only the canvas's workspace sets that height;
the static map a canvas failure leaves keeps the reading's own scrolling
box. It has no "More details" step, "To explanation" or "To code" action,
which moved to what was already on screen. Unknown destinations and
different identities never merge by title. The reading states when no
connection to another part exists in this report. That absence does not
classify the declaration as unused or invent a connecting edge; internal
relations and the complete source inventory remain available. A new
selection reads from its top. Returning to a part (Back, or the same part
shown again) restores its expanded evidence and reading scroll as well as
the canvas camera; a reading reached anew by a click opens in its default
state, whatever "Expand all" or a fold opened on an earlier visit (owner,
2026-09-29). Saved core/entry/dependency
lanes remain named in the map and reading; an import is not promoted into an
external communication.

## Reader context

The complete component/input catalogue remains available beside the same map,
including all original purposes, manifest/entrypoint anchors, incoming and
outgoing observations. Catalogue rows are all visible within their saved type
or destination; only an individual record's evidence needs disclosure.
An input's row names the input by where it is registered and its handler
by its code; when the row's own link to the input (its "To explanation",
or its title's anchor) names one input, a plain click on the name or the
handler reads that input in the report, in the column and on the map, and
a modifier-click still opens the code. Nothing is matched by name.
This does not expand or change the canvas. The Parts count uses the same local
leaf groups across all lanes, excluding operations, frames and foreign nodes.
Its link focuses the component on the common map. The separate core-lane code
reference is labelled Core, and cannot imply an empty component map.

When no item is selected, the reading column shows the repository summary,
marked as the model's by its style alone, and entrances to the complete saved
question menu, run material, terminology, missing observations and author
claims. The components, their areas and inputs are on the canvas and are not
listed again (owner, 2026-09-28); a target the run could not read is named
there with why. It uses ordinary rendered content, not an additional model
summary. Selecting
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
All is the list's default and runs the whole-map action, clearing selection,
input context and the visible Find query after recording the new visit. Show
whole map above the canvas moves only the camera: the reading, its emphasis
and any input path stay, as a reader zooming out to look around expects.
The "−" beside it
steps out one level, as a zoom mark steps in one, and, as Show whole map
does, moves only the camera: from a part's tiles to the frame holding
that part, from an open area to its component, from an open component to
the whole map. The camera takes the frame as entering it would, and no
closer than the zoom at which the level it leaves closes; at the whole
map it zooms out by a fifth. "+"
zooms in by a quarter. Component choices and Back restore their
matching selection and camera.

The toolbar retains the current question and the exact component/area/part
path, with the pinned input before it and the declaration read after it
("get · kvd (executable) / {area} / {part} · getCommand"). Each segment
is its own link that goes up to its level,
the reading and the camera together: a component or area is read and
framed as entering it frames it, the part is read and entered without the
declaration, the declaration is read in its part with its tile centred,
the input is entered as its path. With nothing read it names the System
map.
Browser Back/Forward, a source-details side trip and reload preserve the
question, selected operation, exact code selection, map search/filter and viewport. The completed initial overview camera is saved
even with no selection or hash, and restoring an empty selection clears the
previous inspector. Imperative camera changes are saved after they finish.
Restoring a complete visit is not followed by another hash-driven
selection; an explicit overview/detail transition remains a separate visit
even when the selected item's URL is unchanged. Reset keeps the current level.
A named Back to map returns from full source
details, the glossary or any section below to the page as it stood at the
last click made while the map was read, the canvas where it was on screen
and its camera unchanged (owner, 2026-09-29: it had come back shifted);
Back to question returns to the original answer. Old component and
map links resolve to the same common canvas. Answers, component reference and
repository material open below that mounted canvas, and each has a direct
return to it. Component sidebar links expose the existing flow, configuration,
data, core, dependencies, dynamic execution, coverage and TODO sections when
present. A link to a section's heading opens the sections holding it and
the list the heading heads.
The redundant single-component entrance is not generated alongside
the common canvas. The repository summary and useful links remain, with no
"Understand this repository" or visible "Starting points" label.
The introductory sentence has no model badge or source popover. Its complete
saved citations and model attribution live in Repository summary sources,
reachable from the home reading column.
Display-only iteration uses the
same saved analysis and translations, with no provider calls.

Answer provenance, checks, supporting readings and question origins share one
collapsed apparatus; existing source IDs and excerpts survive. Terms receive
one underline per answer, and source-unavailable labels remain inspectable.
An underline finds a glossary name as a whole word in any letter case, alone
or with an English plural `s`/`es` (snapshots for Snapshot, classes for
Class, never good or goes for Go; a code declaration's name only as
written, an acronym's plural only in lower case), by the shared
[term lookup](TERMINOLOGY.md#term-lookup) (owner decision 2026-09-26).
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
Exact code-name matches precede matches buried in answer prose. A result says
what it is: an outside call or its destination is External communication (with
its own filter), never a Part; an input collection is Inputs; a component is
listed once. A declaration is one result by its file and line, however many
programs compile it. It shows
the declaration as its tile does (a type with its fields) and its file and
line, and has one "In program / part →" link per program that holds it,
reading it there; a program that leaves it off its map links to the list
that says so (Not on the map, or Not reachable from the entrypoints). A
declaration no map holds opens at its row in the report by its title.
Choosing a component in the header closes the results and makes it the
results' component. Back to search returns to the same list at the place
the reader left it.
The former Learn/Work switch is gone: the
owner could not tell the two entrances apart, and the switch changed only
the intro, the question list and where the search field stood. A `mode`
query parameter in an old link is ignored. Component reading entrances use
only the saved answer's exact map links. Inline terms now link directly to their existing map
memberships and select that declaration by its source, without a catalogue hop.

The selected part's panel reads its existing interpreted concepts and group
highlights; otherwise it shows the original declaration index without inventing
an explanation. A declaration chosen in the reading, on its tile, from Find
or from another declaration's reading has a reading of its own: its name as
its tile writes it (a function with what it takes and returns, a type with
its fields), the model's line only when there is one, its code ("Open code
↗" and its file and line), then Called by and Calls, read from the relation
rows its part already lists, grouped by the part at the other end, one
line per declaration, its name alone however many places the call is
written. A relation other than a call keeps its own
words. Choosing a line reads that declaration, in place when it is in the
same part, so a chain such as readQueryFromClient → processInputBuffer →
processCommand is followed one call at a time. A declaration without a
line says nothing about one. Equal names with different source locations
remain separate.
Original source links and full target evidence retain their order and stay
reachable.

## External communication and data

Selecting a component on the common map reads it without moving the camera (owner, 2026-09-28). Its reading names it without the kind its label adds ("redis-server", its heading "Component · C executable"; the kind stays only when another component would read the same, and the canvas's cards name it so too), then its role and purpose, the model's by their style; its entrypoints, each one link by its name ("main()") that reads the program's seed in its part when it is that seed, a modifier-click opening its code; its inputs counted by kind ("96 requests · 5 commands · 37 settings"), each count lighting its tiles on the canvas while pointed at (or, while their collection is closed, its row of that kind), dimming nothing, and reading the collection when chosen; its Connections; then its Main flow, what its program never runs, its TODOs and its analysis coverage, each a list opening in place, and a link to its whole page, which links its Inputs collection. Its areas are on the canvas and are not listed again. An Inputs collection is read by its catalogues (`pageInputCollection`, `data-collection`): its component first, in its box, a link to its reading; then each catalogue under its kind's heading with its count, one line each for where its inputs are listed or declared, where they are looked up and what else the declaring code uses, the model's count of them matched to another program's inputs by name, and its inputs as a grid of names by name, each reading its input; then the inputs no catalogue holds, by kind; requests first. Each input's record, with its registration line as written, is its own reading. The "Entrypoints" link among its sections lands on the program's entry, from GroupsIndex's entries in the page data: the part holding every seed, the seed read there when it is one; when a seed stands in no part, or the seeds in two parts, the component's reading, opened at its entry line when it has one. Its full-reference section remains available with the main flow, configuration, its parts (each a link reading the part on the canvas), dependencies, coverage and TODO lists. Its traversal coverage, under the existing heading "Not reachable from the entrypoints", lists the files no entrypoint reaches, then the parts its program never runs, one row per file with the file, the part's name and its declarations there as source chips (GroupsIndex `unreachable`), and then, under "{0} symbols" and by file in line order, the other declarations the component's adapter proved its program never runs (ProgramIndex `unreachable`). Their outgoing calls, listener, registrations and settings are not the component's (READING), and this list is where a reader finds them: kvcli lists `net.c`'s `netListen`, and does not listen on its map, and it lists its part of `loop.c` with that file's functions, a part it does not draw. A declaration is listed once: a part's row does not repeat among the symbols. Find lists each declaration of such a row as Code with no "In part" link and opens its row. Above the parts and symbols the list says, as the files' list does, that nothing this program runs reaches them and that this does not establish that the code is unused. Each declaration another program of the same report runs names those programs, "(run by kvd)", each linking to its component, in page order: kvcli lists `netListen` run by kvd, and kvd lists `netConnect` run by kvcli. This is page data joined from the saved ProgramIndex set, not a walk: a program runs a declaration when its index holds the same declaration (GroupsIndex's cross-program identity, `groupindex.DeclarationKey`: path, line, column, kind and name; never a name alone), does not mark it `unreachable`, and marks some other callable `unreachable`. An index that marks nothing (a library, a program other code can enter by any name, every adapter but C) proves nothing and names no program; a program that reaches every one of its callables is not named either. It stays a page join over the adapters' saved proofs; GroupsIndex's reach is the input handlers' and is per program. The list reuses the existing heading, the off-map row and the "{0} symbols" summary; no badge or map mark is added. A component without a flow or start list shows no flow section. Where no model flow passes, the start list reads each entrypoint forward: its part, then the outgoing connections of that part, the entrypoint's own calls first and then the part's others, each in the order they are written, the first few, each line once. Several call sites of one caller calling one callee are one step and the next distinct connection takes the freed place. The model's main flow is the orientation's (READING): its order is the model's, read from each member's calls in the order they are written.

An outbound kind is shown by the protocol-neutral label its kind has:
`client_request` is "Request", never "HTTP", and the counts and headings of
accepted client requests read "requests sent", whatever the protocol. A
request's method is only the one its code states (the call word or a
literal, Program index): a socket connect or a call whose verb is not
written is listed without a method, never as a GET. The link from a request
to another target's route compares two stated methods; a side that states
none differs from nothing, so a request without a method joins a route by its
path alone and the link stays possible. The system map counts such links as
"requests" between components, and their sources are "Request link sources".

The entrance reads accepted outbound communication directly from GroupsIndex, independently of dependency lanes and key captions. Each observation shows the other participant's role, purpose, known literal address or explicit unknown, and its original call/source chain. Dispatch and explicit remote-client configuration stay distinguishable. Native addresses and code names remain original; role/purpose use the existing display bindings. For display the observations are grouped by destination text (case-insensitive; native label or kind when the model named none): one row per destination carries the record count, the shared kind, basis and address (or the number of distinct addresses); its records are compact nested lines (the native method and address, else the callable, else the first sentence of the purpose, with the source location), all lines visible, and each line opens its full purpose, address, basis and call/source chain. The section count and the first screen count destination groups; a destination text is still not proof of one remote system. Dependency/import groups stay in collapsed code reference and do not supply the integration count. Empty observations do not prove that the service contacts nothing.
A started program (`runs_program`, "Runs a program") is one destination only with the very word its calls wrote, compared as written: another case is another word, and a program no word names ("A program named at run time") or one the model did not decide ("Program not established") is its own call's destination. The frame on the map is the same outward frame, named by the word, one tile per symbol; each record's line is the callable followed by every word its call writes, each in its own code span, in the call's order. A started program whose word equals the name the repository's build gives one of the report's programs (ProgramIndex `target.executables`: a C link output, a Go main package's `go build` name, a Python console script, a package.json `bin` command; `programsNamed`, nothing matched loosely) is that program: its record reads "Runs this repository's program" with that component's name, which reads it, and on the system map the call's arrow goes from the launching part into that component, with no outside tile for it. A program starting itself keeps its tile and the same line (litestream's MCP server starts `litestream`, cmd/litestream-test starts it too).

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
  each text's applicable refs, found by the
  [term lookup](TERMINOLOGY.md#term-lookup); a saved translation
  whose texts now find another name is rejected by `render`, not adapted.
  Equal spellings with different definitions stay
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

- Display translation accepts texts independently. Each text's value is
  judged alone: a string, or an object whose `text` is a string; one wrapper
  member or a `{ref, text}` list (including the echoed `entries` shape) keeps
  the same ref identity, and only the window's refs count. A missing or
  malformed text, a ref listed twice with different texts, or an invalid text
  (placeholder mismatch) is refused alone and journaled; its neighbours are
  published and the answer is cached. A repeated object key still keeps only
  its last value. The refused texts of each window are asked
  once more, in a request of only those texts, and a text refused in that
  answer keeps its source language. Display translation alone also halves a
  whole refused response window, a follow-up included, when an answer
  translates nothing (unreadable JSON, a bare number, no usable text), per the
  owner's 2026-09-09 request for automatic recovery. Original
  entries remain complete; no refused fragments are published or cached as
  accepted answers. The same exact-request split
  memo records this as `response_validation`, not a provider resource limit,
  and applies it only while the owning stage opts in. Valid child windows keep
  their ordinary cache entries. A text refused again after its follow-up, or a
  singleton the provider answered but refused (missing entry, placeholder
  mismatch, validation or envelope failure, resource refusal), keeps its
  source-language text: the entry is published untranslated,
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
- the complete target `program-index.json` and `program-index-set.json`;
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
The common JSON contains the exact selected ProgramIndexes, not a copied
presentation graph. Its group-graph field contains only thin GroupsIndex
overlays; native subjects and structural edges are joined from those embedded
ProgramIndexes in memory before rendering. Consequently `report.json` has one
native graph schema and one semantic overlay schema, with no `ProgramView`.

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

When the ordinary run serves, publication renders the served page (session
source IDs, the run's local roots scrubbed) beside `report.html`, each from its
own shallow copy of the same report data; a render never writes the data it
shares, including the openable-path inventory. The server serves that page
instead of rendering again; a run restored from disk, or one whose early render
failed, is rendered by the server through the same `RenderServedPage`.

## Three layers of truth

- Everything the report shows is a deterministic fact, a claim quoted from a
  human-written artifact, or a model hypothesis, and the three are always
  labeled. `facts.json` holds the anchored fact layer: entrypoints,
  registrations (call word, literals, stated verb, callable handed over),
  SQL statements with their tables, environment keys, the places where the program runs code it was given,
  manifest rows, TODO markers, imports, dead modules, negatives,
  dependencies, and the files in languages no adapter analyses with their
  language and lines. A negative says only what it knows: missing tests read "No
  recognized test files found in the inspected paths", never "No test
  files". Beside the negatives, "What is missing" names that unanalysed code
  by language, the most lines first, each file with its lines
  (`test-redis.tcl · 2 083 lines`), so an unrecognised test suite is not read
  as no tests. Routes, client requests and the portals between targets are
  not facts: they are registrations the reading stage classified, read from
  the GroupsIndex operations and outbound rows and joined on literals in the
  report. `claims.json` holds quotes with their
  source path, date and age. `orientation.json` holds the model's repository
  summary, roles, run recipe, and main flow; every row cites fact, claim, or
  subject ids. The orientation request walks a packing ladder: 40 members per
  group with 6 calls and 6 callers of observation per listed member, then 20
  members with 3 and 3, then 12 members without observations; `member_count`
  is always the real size and the facts and claims are complete at every
  rung. A refusal by size or context, local from a
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
and cycles, without borrowing a sibling operation in the same part. A matched
input's parts and state changes continue the root's path marked a possible
integration, with that input's own entering calls; no chain is prefixed to
them. Outgoing communication is attached to inputs whose saved reach holds its
caller subject.
Clicking a part or communication lists those
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
only when the input's saved reach holds the writer and the written
variable has an exact native type owner. It retains possible receiver/call
resolution, the write source, and the writer's callers on the path (every call
of the reach into it, none chosen as a route; none when the handler writes
itself); reachability does not claim execution on every request. Entity
readings reverse these same records. A matched input's writes join as a
possible integration;
sibling inputs gain no effects. Unresolved writes, ordinary reads and mere
membership in a type-bearing part do not establish mutation. Older saved graphs
without write locations produce no invented evidence.
