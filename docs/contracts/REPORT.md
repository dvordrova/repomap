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
  into another target. Lanes follow a box's side: `triggers` where the
  outside calls in or execution starts, `dependencies` where it only calls
  out, `core` otherwise. No request sends repository source text: paths,
  names, signatures, first sentences of docstrings and documentation excerpts,
  literal values and the model's own earlier lines cross the wire. Question
  excerpts preserve Markdown-authored command/code examples; implementation
  source-file bodies are not sent.

- No file or inventory box is drawn: every box is a part of the map of
  parts. The atlas's explicit off-map record lists, per target, every file (or
  stray declaration) no drawn part holds with its reason (`left_out`,
  `conflict`, `no_units` for a file that declares nothing, `map_failure`,
  `undecided` for a split file's declarations no box of it took), keeping
  its file line, captions and keys. Stray declarations in a file a part
  holds name that part (`box_id`); their file stays on the map and is not
  listed. A file whose code several parts hold (READING, the role split) is
  on the map through them: GroupsIndex (v16) lists it only for its
  undecided declarations, by their subjects (`subject_ids`), never as a
  file off the map. GroupsIndex carries that record as `off_map`, adds the files of
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
  is not a group, draws no arrow and leaves that program's canvas; redis-cli's
  "Linked list", whose thirteen adlist functions redis-cli links and never
  calls, is not drawn.

- The component card lists, after its link to its parts on the system map
  and before its main flow, the compact inventories **Tests** (test-only
  parts' files, with their part) and **Not on the map** (every other off-map
  file, with its reason; a split file's row shows its undecided
  declarations as source chips and reads "In no part of its file"), five
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
  row ("Inputs · redis-server (executable)"). Headed with its component's
  name, Redis's collection had read as a second redis-server beside the
  programs. It stands attached to its component, in the layer next to it
  with its arrow straight into it (measured at 1440×900 and 1280×800).
  Its distant summary
  shows the actual catalogue types (requests, commands, background work,
  interactions, other operations); zoom reveals the original named input nodes.
  Inside the collection the inputs stand together by the part holding their
  handler (the saved implementation owner), each group framed and titled by
  that part's name and ordered by it; choosing a group's title reads that
  part. The collection opens to its groups first, each closed and named as
  a closed area is, with a zoom mark that enters it; a group opens to its
  inputs when their headings read (14px to open, 12px to stay open), as
  areas open to their parts. Opened with the collection, Redis's 95 inputs
  had stood as a wall of 5px tiles under 5px group names. An input with no owner stays loose after the groups, and a collection
  whose inputs share one part keeps them loose. A tile names its kind only
  when it is not the collection's most common kind: "Request" on 97 of Redis's
  98 tiles repeated the collection's own summary. The groups are display
  containment, not architectural areas; they add no relation.
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
  witness to any of those calls leads to that tile). Every frame and
  tile belongs to its component, and only that component's arrows reach it:
  equal destination text proves no identity. One "TCP endpoint" box had taken
  arrows from all three Redis programs, though for redis-cli that endpoint is
  redis-server and for redis-server its master. Frames of different
  components that name the same destination stand together in one display
  group, an amber frame around them in the outer layout, so the three "DNS
  resolver" frames stand side by side instead of scattered. The group is no
  participant: it has no reading, selection or hover, ends no arrow and adds
  no relation; each frame in it keeps its own program's arrow.
  When every frame in the group spells its destination alike, the page data
  gives the group that text (`DisplayGroupTitle`) and the group's frame
  carries it once; its frames stand as small plain amber tiles with their
  zoom marks, one per program, each hit by its own program's arrow. Three
  "DNS resolver" headings side by side had said one thing three times.
  The text is what all the frames name, not the name of a merged
  participant: each frame keeps its title in its reading, search and
  accessible name, and different spellings grouped regardless of case keep
  their own headings and give the group none. The tiles do not name their
  programs: a program's name inside an amber frame would read as an outside
  participant, and the arrow already says whose tile it is. The heading
  stands in a band on the side of the group no arrow enters, under the
  tiles when arrows run down and after them when they run right; above the
  tiles, Redis's three arrows ran through it. Like closed summaries it is
  laid out at the whole-map camera and zooms with the map; once the tiles
  open it reads at their open frames' title size and the open tiles stay
  plain under it, so the destination is named once in every state. Titled
  one by one, Redis's three open DNS tiles said "DNS resolver" three times.
  The band is the heading's room and shrinks with it: an open group's frame
  wraps its tiles and their heading, and the room its closed heading takes
  at the whole-map camera stays outside the frame. Entering any tile of the
  group opens every tile of it and frames the group, heading included, at
  the scale its calls are drawn at: framed alone, an entered tile had read
  only "gethostbyname" with the heading below the camera. The tiles stay
  separate records, each hit by its own program's arrow; the group still
  ends no arrow and is not read or chosen. A plain tile keeps its open calls' proportion: at the whole-map
  fit it grows whole until its zoom mark has its room, and its calls grow
  with it, so it opens with no room of its own below them. Stretched to the
  mark's proportion and grown to its 50 by 44 pixels, each of Redis's DNS
  tiles opened half empty below its one call; kept at its calls' own box
  without growing, its mark was drawn at 0.64 of the size of the TCP
  endpoint's beside it on a 1440×900 first screen and at 0.41 on 1280×720.
  Every record stays in its own component's catalogue, and records of
  different symbols stay separate tiles.
  When a saved connection identifies one already displayed peer in another
  target, the canvas connects the original caller directly to that peer/input
  instead of adding a third participant. Its outbound catalogue and source
  reading remain intact. Missing, partial or ambiguous matches stay separate;
  equal destination text never establishes this link. Both endpoint sources,
  operation membership and possible status survive the display projection.
  The frame is a display collection, not a newly inferred component. They use amber cards; ordinary parts and area
  frames use neutral tones. Core parts use purple, entry parts green, inputs
  blue and external communications amber; saved lanes supply those identities.
  Core/entry cards use distinct diamond/arrow glyphs with the shared legend and
  accessible names, instead of repeating Core/Entrypoints above every title.
  The entry arrow is wider than the border it stands on and has a thin halo
  in its card's colour, so the border stops at it and its shaft reads as a
  shaft; cut from a 14px square, its shaft ran along the border in the
  border's colour and only a small head read. The legend draws the same
  arrow.
  Only an area holding the program's entry (a declaration its execution
  starts from, a target seed) carries the entry mark, even when a core part
  stands in it. Parts that only take requests or listen do not make their
  area the entry: Networking's listen/bind boundary had drawn Redis's "Core
  infrastructure" as a second entry area beside Server runtime. Such an area
  keeps the core mark when it is core and no mark otherwise; each part keeps
  its own mark.
  Concrete input captions remain. A dark outline identifies the card open on
  the right without inserting another row or changing its text position. Dark arrows and outlined participants
  identify the currently emphasized connections: a participant takes the
  arrows' dark on its border, and the card open on the right keeps its
  heavier outline. Emphasis recedes the rest instead of greying what is
  pointed at: the subject takes the existing dark (a part its outline, a
  frame its border at the arrows' 2.5px), the parts across its dark arrows
  take the same outline, a pointed frame's own parts and the arrows between
  them stay as they are, and every part, frame and arrow the emphasis does
  not involve recedes to 40% opacity. Frames grouped under one destination's
  text recede with their heading when none of them is involved and stay when
  one is. No veil or tile fill marks the pointed thing: a grey veil on the
  pointed area's parts and a grey fill on the pointed declaration had made
  them look deader than their outlined neighbours.
  Component frames retain their language, kind, role and purpose above their parts. Colour never replaces
  the visible type cues or independent fact/model provenance in the reading
  panel. Text has at least 4.5:1 contrast; meaningful frames and connections
  retain at least 3:1 contrast, including non-selected neighbours; only
  what an emphasis does not involve recedes below it while that emphasis
  lasts. A grayscale
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
  automatically. Search and numbered destinations reveal their result at a
  readable scale, opening its enclosing frames in the fixed world. A hidden
  child's bounds inside the viewport do not count as visible content. A fully
  visible, legible part keeps the current camera when opened from a reading.

- The drawing has exactly one reason for emphasis: search results, the part
  or frame under the pointer, the pinned input path, or the selected part's
  neighbours. A pointed part is the subject itself: only its own arrows
  darken. A frame's title, border and empty space look at the frame, whose
  arrows crossing its border darken. Lifted to its area, a part pointed at
  in Data type commands had lit all of the area's arrows.
  A pinned input path is drawn as its trace with the existing dark emphasis:
  the arrows of its shortest call/read witnesses, each reached part joined
  from the part its witness enters through. Other calls among the same parts
  stay ordinary arrows. Initialization arrows (every relation reached only
  from the target's seeds) are drawn only while one of their ends is looked
  at, and only in a target that serves something: a target with no operation
  and no chain does all its work from main, and Redis's benchmark, client and
  dump checker had drawn none of their arrows. A call that code reached from
  an input makes across two parts is work, not wiring.
  Hover temporarily replaces the drawing emphasis; it never unions another
  area's edges into a pinned input path. Leaving the canvas restores that path
  or selection. Reading an off-path part does not make it a path participant;
  the reading card explicitly says it is outside the saved input path. Search does
  not mix old selected or hovered connections into its matches.
  Ancestor frames retain a neutral outline while their descendant is in focus;
  this containment context never adds the ancestor's other connections. Frame
  titles retain a light background and readable role/purpose text. The duplicate
  `Reading` line and close action do not occupy space above the canvas. That
  space has the map's one key, not changing hover prose: each kind of card
  drawn as a small card in the fill and border the canvas paints it with,
  its mark on its border, then the solid "calls" and dashed "possible
  calls" strokes the arrows are drawn with. Its glyphs had been painted in
  the marks' dark colours, so Redis's pale green and purple cards matched
  nothing in it, and no line was keyed. There is no folded prose legend
  under the map. The input context
  and leave-path action live in the reading card. An input chosen from
  Find, a link, a reading or its own tile on the canvas is entered as its
  path, while the reading column reads the input. The camera takes the part
  holding its handler, then each part the trace reaches from a part already
  taken while all of them fit at a scale where their headings stay about
  twelve pixels (and their layer open), and frames them; when the next step
  does not fit, the camera stays at that scale and leans toward it, keeping
  what it took inside, so the dark arrows leaving the frame show the way.
  The path's parts, and a closed frame standing for parts hidden in it, are
  outlined in the dark of the path's arrows. Framing its tile had shown GET
  as one of 98 tiles with no arrow in sight, and centring String commands
  showed four of its nine dark arrows and none of the parts they reach, drawn
  like every other part. In Redis the handler's first step, Client
  connections and replies, stands farther from String commands than the
  canvas holds at a readable scale: the camera frames String commands,
  Object and key store, Server configuration and Sorted set commands, six of
  the nine dark arrows, and leans toward it. An
  input without a trace is entered as its tile, and such a tile clicked on
  the canvas keeps the camera. "Show input" stands in the reading card while
  the camera may be away from the input's tile (on its path, or on a part
  read since) and frames that tile among the inputs its handler's part
  takes, the group it stands in, never the whole collection: GET among
  String commands' fourteen inputs, SET beside it, not a wall of 95. A group
  larger than a readable camera is entered at the tile. Once the tile is
  framed there is nothing to return to; choosing the tile again returns to
  the path. An input's reading
  names its handler ("handled by getCommand") as a link into the code. A
  registration the model did not explain keeps its own call words as its
  line ("redis.c.redisCommand.proc get in getCommand"); that line is the fact
  said again and is not shown, in the reading or in Find. An up-chevron in
  the reading card's own header closes details, preserving camera and any pinned
  input path; leaving that path remains a separate action.

- Hovering an area or a part inside it labels its inner parts with local
  numbers. One outside participant and direction form one label with every
  exact inner endpoint number (for example `2 · 3`). Different outside
  identities and opposite directions remain separate. Resting on that label
  opens its card: all original relations, sources and possible-call marks
  behind it; selected details and their links stay present. Clicking its
  number pans to the named outside participant. Connection endpoints use compact number badges; their outside names remain
  available to assistive navigation and the reading column.
  Lines have no click target or native tooltip. These numbers
  identify parts, never execution order. The toolbar has no connection-style
  selector; the same real endpoints remain connected across zoom levels.
  An end that joins every numbered part of the frame says so once, "all",
  in the same chip, instead of listing every number (owner's choice 2a,
  2026-09-27); the numbers stay wherever an end joins some of the parts.
  While an end's card is open or kept open, the parts behind that end, or
  behind its one number pointed at, take the dark outline in place, the
  end's own arrows are dark and whatever the end does not involve recedes;
  the parts at the arrow's other end stay as they are. A label stands where
  its arrow meets the frame it numbers: both directions of a pair of frames
  share one drawn route, and the end one direction took could be the other
  frame's, so Data type commands' incoming numbers stood on Server
  runtime's border, in the gap where the pointer looks at the whole
  component, and could not be reached. The two directions' labels then
  meet the frame at one point and stand either side of it along the border.
  An open area numbers its parts and an open component its areas and loose
  parts, with those numbers on the frame's border. When an area or component
  in the map holds anything, one line says that the numbers on an area's or
  component's border are the numbered parts inside that the arrow connects,
  not an execution order, and that "all" is every part inside. It stands visible beside the map's controls, above
  the key, where the row had room: folded into a legend at the bottom of a
  900 px window, no reader of Redis's map found it.

- A card (a label's calls, or everything a numbered part is joined to outside
  its frame) opens on intent: the pointer rests on its handle for about a
  tenth of a second, so a handle crossed on the way elsewhere opens nothing.
  It stands flush with its handle, outside the frame being read so it covers
  none of that frame's parts, on the side with room, and wholly inside the
  canvas; a label's card goes out through the border its label stands on.
  With no room outside the frame it stands beside its handle toward the
  roomier side. It stands in the map's coordinates and moves with it, but
  at the screen's own type size, and is placed again when the zoom
  changes: drawn at the parts' scale, a card of 291 calls read at 10 px
  beside Redis's Core infrastructure. While a card is open or kept open, the frame being read stays: the way to
  the card crosses other parts, frames and empty canvas without changing
  the emphasis or the labels. Leaving the handle, the pointer is safe inside
  the triangle between where it left and the card: the card lasts and no
  other handle on the way takes it. Re-targeting on every pointer move had
  closed the card on the way to it, and with the pointer on it, as soon as
  it stood outside the frame. A click on a part's number or on a card keeps
  the card open; a click on an arrow end reads, in the column, the frame it
  stands on, scrolled to that frame's Connections with that connection
  open, and leaves the camera where it is (owner's choice 3b, 2026-09-27).
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
  (Redis's call reaches `c->cmd->proc`, one of 94 command functions), and
  the calls of a declaration that hands every member of such a set over by
  another relation (cmdTable passes all 94 as callbacks); each caller's
  marked calls are one line, "call → one of 94 · 77 here", "cmdTable passes
  callback the same 94 as call · 77 here", with the callees under it by part,
  each part opening to their names. Server runtime → Data type commands had
  listed both 77 times, one row each. Which calls belong to a set, and how
  many the set holds, is decided in Go from the relations' retained targets;
  the browser only counts what stands behind the arrow. A caller calling
  every member itself, or handing over part of a set, keeps its rows. A
  part's number opens the same rows, per label, without the index.

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
  boundary, compact numbers correspond to the explored area's actual inner parts;
  shared participant/direction labels retain every matching number and source.
  The original endpoints, possible status and source relations remain available for
  reading and operation paths. Aggregated outside strokes are drawn once; they
  are display geometry, not new semantic relations. Each area's interior is
  laid out alone, from the arrows between its own parts; arrows between areas
  are the component's bundles between ready rectangles. The two directions of
  a pair of ends share one drawn route, so ELK lays out one edge per pair:
  laid out twice, each such pair made ELK reverse one arrow into a
  wrap-around, and Server runtime's 21 arrows among six parts had 68 bends.
  That edge is the pair's first. Laid out instead from the program's entry
  side (a `triggers` part) toward the other, Redis's Server runtime was
  measured at 14 window sizes and refused: 84 bends became 70 at four sizes,
  54 became 70 or the routes ran 9% longer at eight, and at 1440×900 the
  area took a one-row layout that entered cut in half. Which of ELK's layouts
  the area takes, and how it wraps, moved more than the pair's direction.
  Each area takes, of ELK's directions (rightward, downward) with and without
  wrapping a long chain into rows toward the canvas proportion, the layout
  that fits the initial canvas while its 17px part headings stay at the 12px
  their layer stays open at; then the one whose routes run shortest (the
  fewest detours around the area); then the squarer box. Wrapping had run
  Persistence's one arrow around the area and drawn Server runtime 1300 px
  wide in a 1214 px canvas. Its parts keep their own size,
  the size of the loose parts beside it, so an area is as large as what it
  holds: laid out with the whole component and shrunk to a peer's width,
  Redis's Server runtime stood as a staircase of postage stamps under a
  full-size title. Each area wraps its drawing and heading without reserving a
  second member-list height. A closed group
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
  theirs when their boxes are larger: Redis's Debug symbols read 10 px beside
  14.8 to 17.1 px area titles (about 11 px beside 15.7 px in the proxy's
  window). This is measured and left. The camera caps every closed heading
  at a scale the whole map sets, so matching the cap needs the interiors laid
  out again once the whole map is placed. Growing the box in the one layout
  until its heading fits at its smallest area's heading scale reads 14.9 px
  there, but the part fills that box once the areas open: beside two areas
  of fourteen parts it filled 1036 by 739 px beside 260 by 88 px parts (400
  by 200 px as its card), and Redis's Debug symbols 244 by 143 px beside 128
  by 61 px parts (196 by 98 px). Once the areas open it is their parts' peer:
  the same card at the same scale, filling its box, so its title reads at
  their parts' size. Kept at the summary scale, Redis's Debug symbols read
  41 px beside 17 px parts. A component without areas keeps its direct
  parts' fitted headings.
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
  Nothing inside a card moves when an area is looked at or chosen: a part's
  number stands in room its card already leaves. A rule reserving a row
  under the title for that number had dropped every description of Redis's
  Data type commands 18px whenever the pointer crossed the area's border.
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
  lists them, the model's keys first; the file is a hint, and Redis's
  linked-list functions had stood scattered among the dictionary's. They
  stack in that order within their column and a column too tall spills into
  the next. A tile is as wide as the longest name among the part's
  declarations, so no name is cut, and the columns share the card's width;
  cut to 190px, Redis's names read "_dictStringCopyHTKe…". The tiles are
  drawn at a quarter of the card's scale, or smaller when a quarter does not
  hold them all whole, however many the part holds: nothing is counted away
  and no scale is too small to search, and a large part is a larger
  drawing to pan (at 1440×900 Data structures had drawn 40 tiles and
  counted 63 away as "+63"). The
  part's name stands over them at the same screen size at any such scale.
  Placed by link column first, the repomap self-run's parts hid 88 of their
  256 keys while drawing declarations listed after them. The zoom button
  enters at the scale the declarations read at, their own 13px: the part
  whole when it fits there, else its head and first column a margin from
  the canvas's top left. Fitted to the canvas, Redis's Persistence had
  opened at its title's scale with no declaration drawn. A drag over the
  declarations pans the map, as a drag over the part does; a drag that
  moved chooses nothing.

  Pointing at a declaration darkens only its own arrows: its links to the
  part's other declarations, and among the part's arrows those carrying a
  call into it (the call's callee is its source) or out of it (the call's
  caller is its name); the declarations its links do not join recede. A
  click on a tile chooses that declaration: the part is read with it named
  in the reading column (the existing `map.explainSource`), the tile keeps
  the dark outline of what is read, its own arrows stay dark, and the camera
  centres it at its reading scale. A modifier click still opens its code. A
  declaration the reading column names by its source (Find's code hit, a
  declaration chosen in the reading, a restored visit) is the one chosen on
  the canvas, and a newly named one is centred at its reading scale; a
  restored visit keeps its camera. Clicked, Redis's tiles had opened GitHub
  in a new tab or bubbled to the part already selected, and a Find code hit
  had stopped at the part. The page data gives each tile its file and the
  same source link its reading uses, served or static.

  Root summaries prioritize saved area names over role, counts and purpose;
  complete dense inventories scroll without dropping entries. Ordinary wheel
  scrolls an overflowing inventory, while pinch passes through to the map.
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
  like theirs, in the muted text colour: in the warning red it read as an
  error, and the key's dashed stroke says what it is. All original endpoint IDs and sources remain on it;
  the drawing does not invent another relation. Number badges sit inside the
  frame, centred at the native connection point, and match the associated
  inner part badge in appearance and scale. They preserve every matching number. They do
  not encode execution order. Floating-point offsets cannot create diagonal
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
  first part at that scale. Fitted at its layer's floor instead, Redis's
  Server runtime stood at 8px headings at 1440×900 and 4 to 7px at 1280×800. Entering a frame or an input's path
  opens what it enters: those frames stay open through the camera move's own
  zooms, so they arrive open although the camera stands smaller than a
  closed layer needs to open by itself. Closed on the way, microblog's
  /explore stood on the closed Web routes summary with its title at 45 px.
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
  included. Measured over the participants alone, the camera framing Redis's
  group of "DNS resolver" frames stood 0.85% smaller than the fit, and every
  heading reserved to the pixel lost its last letter ("DNS resolve", "TCP
  endpoin", "redis-server (executable" over a lone ")").
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
  less its zoom button's room beside a part's title: measured 3px wider,
  "Implements Redis set commands and" (225.84px) broke again in the 225px
  column and left "and" alone on a line. A part's description is not broken
  into lines on the page; the browser wraps it in that column, and its lines
  counted there only size the card, at most three, the third cut with an
  ellipsis. The description is held to that column's width: a browser that
  draws the 1.5px border 1px wide leaves 226px, where pykrx's 225.39px
  "Fetches Korean market fundamentals" fit whole and its card kept an empty
  line.
  When the whole-map fit still cannot give a summary its reserved width or
  height, in a short window or on a crowded map, the summary is laid out at
  that reserve and drawn scaled down whole, its zoom mark with it until zoom
  gives the mark its ordinary size, as a small group keeps a smaller complete
  label; a frame too narrow for its text at full size keeps that smaller
  summary instead of standing blank beside a display group's plain tiles.
  Drawn into the short box, Redis's 1280×720 first screen cut "TCP
  endpoint" below its frame and "Background" out of its input list.
  External width minima allow that arrangement; height reserves the actual
  wrapped heading and nested-content hint with their insets, without an unrelated floor.
  Orientation comparison uses those measured minima before world size.
  Compact component purposes use the remaining whole lines, with an
  ellipsis when shortened, and stay hidden if fewer than two lines fit; the complete
  purpose remains in the reading column. External frames reserve summary space.
  The fixed world places frames, parts, complete input cards, component
  purposes and grouped labels. Routes inside one group retain actual part-to-part
  endpoints; routes between groups and participants stop at their boundaries. ELK chooses the actual endpoints. Number badges sit just inside those
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
  interior space: their number badges belong to the outer endpoints, so their
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
  declarations. Drawn in the thicker line's stroke widths, an emphasised
  head stood 17.5px beside 10.5px and covered the number at the frame.
  Connections retain their screen stroke, casing, dash and arrowhead sizes
  through React Flow’s ancestor transform. Frame outlines use inset paint so
  browser minimum-border rounding cannot turn a fractional world stroke into
  a thick close-up border; corner radii also stay at screen size.
  Fixed overview text scales with the viewport independently of graph props.
  Ordinary camera movement does not rebuild the graph or remeasure unchanged
  text. Only common detail-layer transitions update the displayed contents.
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
  part is centred in the canvas; a part taller than the canvas shows its head
  a screen margin below the top. The margin is screen pixels: taken as world
  units at a close-up zoom it put Command dispatch under the canvas edge. A
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
do not establish a path. The path is that trace, not a neighbourhood: its arrows
are the witnesses' steps between parts (a caller off the map passes the step to
its own caller), and its reading lists the parts by call depth from the handler,
in the order the walk met them within one depth, with a part reached through a
matched input after them. Selecting GET had lit every call among fourteen parts
and listed them starting with Strings. Call depth is a static measure, not a
recorded execution order. All original structural relations remain available.

The key lists only the kinds and strokes present in that map.

Group readings put the description first, then the input's witness when an
input is pinned, then what the part is made of, then incoming connections
before outgoing connections: Command dispatch had listed some 5,000 characters of
connections before its code, and Client connections and replies the
nineteen parts it calls, mostly utilities, before the fourteen that call
it. What a part is made of (owner's choice 3a, 2026-09-27) is headed by
its declarations counted by the kind its tiles carry ("Made of 18
functions", "2 functions, 3 types") with the files they are written in
once beside it, and lists every declaration of the part as a link into its
code, the model's keys first and in bold as the part's tiles draw them,
then the rest by name whatever their case, with no line number: by file
and line, List commands' eighteen functions read as a column of line
numbers. The tiles keep the page's own order. It had listed only the keys
under a heading that said all, and Replication's showed
replicationFeedSlaves and not syncWithMaster. An area's reading starts
the same way under its description: "Made of 9 parts", then each part, a
link to its reading, with its counts and files and its declarations as
links, keys first and bold, the rest by name; a declaration is read in
its part without moving the camera. An area's or a component's reading
then lists its Connections as its arrow ends group them: one line per
frame or participant at the other end and direction, incoming first, with
the count of its calls and the parts they are made from, each opening to
the same rows as the arrow's card; they replace the list of neighbours by
name. An input collection is named Inputs there, as its canvas heading
is. Each declaration is one line, as a tile is, a type's fields
on a line under it, and a key type the model explained keeps its mark and
its fields: a bordered box each had put Client connections' callers some
1,700 px further down the reading. The reading column does not repeat the source index, which
is that list again by file and stays on the part's card. A type's fields stand inside its row, in the code
list and the source index alike, never as peers of the part's functions. The original native relation is available even when the
model supplied no sentence for that pair. Relations between declarations in
the same part remain available under Connections within this part, with their
original call/read sites, destination declarations and resolution. Membership
alone adds no relation; containment remains the source inventory. Native code labels keep their own
provenance; model summaries remain marked. Distinct relation IDs preserve
same-line call occurrences. Selecting a peer pans to it; Back restores the
camera and expanded source evidence. Connections share one heading per
exact participant href and direction, with each distinct saved summary once.
Source details retain every original row, possible-call mark and source pair.
A relation row is said through one closed vocabulary of the report's UI
messages, chosen by its kind and filled with the two declarations' names
("initServer passes acceptHandler as a callback"), never the stored kind
("passes_callback"); a C program's import is said as an include, and a
joint between two programs names the declarations at both of its ends
("anetTcpGenericConnect connects to anetAccept", not "integrates with").
An arrow's card reads each of its calls as caller, relation and callee and
links both names, so an arrow's call is those three words: the phrase's
words when they stand between the names (the joint's card had shown only
"anet.c:158"), the relation's kind when the phrase wraps the callee
("cmdTable passes callback delCommand"; the full sentence showed neither
name). A row the model wrote
in its own words keeps them. Rows of one caller and one relation kind in
one evidence list fold into one line with their count and callees, each
row inside it with its sources: Client connections and replies listed
"cmdTable calls …" 97 times. A list with folds has one "Open all" control
that opens and closes them together. Inputs reaching this part is
collapsed with its count. The reading column stands beside the map's
controls and key as well as its canvas and takes their height, with the
canvas keeping its own: 695 px of a 1440×900 window instead of 610. Only
the canvas's workspace sets that height; the static map a canvas failure
leaves keeps the reading's own scrolling box. It
has no "More details" step, "To explanation" or "To code" action, which
moved to what was already on screen. Unknown destinations and different identities never merge by title. The reading
states when no connection to another part exists in this report. That absence
does not classify the declaration as unused or invent a connecting edge;
internal relations and the complete source inventory remain available.
The reading column uses these same grouped sections; the duplicate full-group link and vague
All disclosure are removed. A new selection reads from its top. Returning to
a part (Back, or the same part shown again) restores its expanded evidence
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
All is the list's default and runs the whole-map action, clearing selection,
input context and the visible Find query after recording the new visit. Show
whole map above the canvas moves only the camera: the reading, its emphasis
and any input path stay, as a reader zooming out to look around expects;
clearing them had sent that reader back to the start. Component choices and
Back restore their
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
programs compile it: adlist.c's listCreate had been three results. It shows
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
rows its part already lists (each a fact with the line it is written on),
grouped by the part at the other end, one line per declaration with every
place the call is written. A relation other than a call keeps its own
words. Choosing a line reads that declaration, in place when it is in the
same part, so a chain such as processInputBuffer → processCommand → call is
followed one call at a time. A declaration without a line says nothing
about one: "No explanation saved" had answered a click on every tile but
the keys. Equal names with different source locations remain separate.
Original source links and full target evidence retain their order and are
available without scripting.

## External communication and data

Selecting a component on the common map opens its existing purpose, entrypoints and complete input catalogue in the panel. Its original full-reference section remains available with the main flow, configuration, group cards, dependencies, coverage and TODO lists. Its traversal coverage, under the existing heading "Not reachable from the entrypoints", lists the files no entrypoint reaches, then the parts its program never runs, one row per file with the file, the part's name and its declarations there as source chips (GroupsIndex `unreachable`), and then, under "{0} symbols" and by file in line order, the other declarations the component's adapter proved its program never runs (ProgramIndex `unreachable`). Their outgoing calls, listener, registrations and settings are not the component's (READING), and this list is where a reader finds them: redis-cli lists `anet.c`'s `anetTcpServer` and `anetAccept`, and neither listens nor accepts on its map, and it lists "adlist.c · Linked list" with its thirteen functions and "adlist.h · Linked list" with its types, a part it no longer draws. A declaration is listed once: a part's row does not repeat among the symbols. Find lists each declaration of such a row as Code with no "In part" link and opens its row. The list reuses the existing heading, the off-map row and the "{0} symbols" summary; no label, badge or map mark is added. A component without a flow or start list shows no flow section. Where no model flow passes, the start list reads each entrypoint forward: its part, then the outgoing connections of that part, the entrypoint's own calls first and then the part's others, each in the order they are written, the first few, each line once. In the connections' stored order, grouped by the part they reach, redis-benchmark's start read "main calls aeMain" before the `aeCreateEventLoop` main calls thirty lines earlier; three call sites of `main` calling `aeMain` are one step and the next distinct connection takes the freed place. The model's main flow is the orientation's (READING): its order is the model's, read from each member's calls in the order they are written.

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
  manifest rows, TODO markers, imports, dead modules, negatives, and
  dependencies. Routes, client requests and the portals between targets are
  not facts: they are registrations the reading stage classified, read from
  the GroupsIndex operations and outbound rows and joined on literals in the
  report. `claims.json` holds quotes with their
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
