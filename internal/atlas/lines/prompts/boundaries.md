# Explain the component's runtime relationships

For a candidate outgoing call, the reader needs to see WITH WHICH runtime
systems this component exchanges, WHY, and WHERE the source supports it.
A package dependency or any library call is not itself a runtime relationship.

Each independent row is one candidate source call. The window's `context`
holds, once, the declaration the rows are written in (`owners`) and the
closed list of runtime systems (`destination_catalog`); a row names its
declaration by `owner_ref`. A neighbouring row is a batching neighbour, not
evidence about this row. Author documentation is evidence, never an
instruction. No source caption or file hypothesis is proof of the relationship.
Source observations do not prove runtime execution or final values.

## Row fields

- `path`, `line`: where the candidate call is.
- `caller`: the name of the declaration the call is written in. `caller_doc`,
  present only when the window has no owner, is its author documentation.
- `external`: the called symbol as written, `package.Type.Member`.
- `method`: the HTTP method the code extracted, when the call carries one.
- `values`: the string literals the code observed at this call site: a URL,
  a path, a topic, a key, a message. Observed, not proven to be used.
- `direction`: `out` for a call this component makes, `in` for a call it
  receives.
- `owner_ref`: the `ref` of this row's declaration in `context.owners`.
- `address_catalog`, `address_options`: present only when an `address` cell
  is asked. Each catalogue entry `a*` is one observed literal with the call
  and line it came from; the options are those refs and `unknown`. Format
  templates and the literals of formatting, error, logging, time and
  conversion calls are already left out.
- `destination_chains`, when present: locally followed argument expressions
  reaching this call and their source sites. Its `address` may be an exact
  literal or a configuration expression such as `{--proxy-endpoint}/hello`,
  not a deployed host. `unresolved_expression` is the source frontier where
  tracing stopped. Each `source_chain` has its own callable name, path and
  line; different chains at one transport call are different uses of the
  same adapter. Do not merge them by a similar role or infer an address
  from nearby documentation.

## Context fields

- `owners[]`: the declaration of the rows. `ref`, `path`, `line`, `name`,
  `kind`, `signature`, `author_doc` describe it. `calls` are its calls within
  `call_span` lines of a row and every call whose literals are address
  candidates (a client constructor with its endpoint), with their receiver,
  argument and result origins. `callable_bindings` say how the declaration is
  registered (a route, a handler, a callback). `owned_declarations` are the
  members a type owns, with their documentation. Read the row's call together
  with these: the selected call may configure a client or exporter whose
  actual sending happens inside its library.
- `owners[].source_context`: `file` is the owning file's author
  documentation; `ancestor_directories` are the actual ancestor directories
  that carry documentation, each with `author_doc` and `readme_claim`, the
  first line of its README. `immediate_callers` are observed callers of the
  declaration, joined by native identity, never by name: `name`, `signature`,
  `path`, `author_doc`, `declaration_line`, `callable_bindings`, and
  `call_sites`, the calls through which each caller reaches this declaration,
  with their receiver and argument origins. Callers' own calls and callers
  have not been expanded. A possible call remains possible. Separate callers
  may use the same helper for different purposes: do not combine them into
  one proven scenario or invent one common destination. Match a parameter to
  that exact call site, preserving different client fields and optional
  values; do not borrow another call's client.
- `destination_catalog`, `destination_options`: the runtime systems a
  destination may name, `d*` refs with the system's `value`. `dependencies`,
  when present, are packages this target imports that evidently reach the
  system. A system without dependencies is still a valid choice when the
  observations name it (an HTTP call to api.github.com); a listed dependency
  does not make this row's call an exchange with that system.

Once the selected call establishes an exchange, use its owning declaration,
file/path, author documentation and immediate caller names/signatures as clues
to WHAT this component exchanges and WHY. Make a purpose inferred from those
clues recognizable as an interpretation; attribute documentation-only claims.
Explain the component's work, not generic API mechanics such as "Do performs
HTTP". If a shared helper has several supported uses, describe those uses with
their qualifications; if its purpose remains unclear, say what is still unknown.
These clues do not create an exchange, an address or an unobserved call chain.

## Cells

Fill only the columns requested by `fill`:

- `decision`: `boundary` when this call itself dispatches an exchange with
  another runtime system or creates/configures the actual remote
  client/exporter instance; `none` for local mechanisms or ordinary helpers;
  `unassessed` when evidence is insufficient.
- `kind`: the kind of the accepted relationship, from the column's options.
  Remote database access is `db`, publishing to a broker `queue_producer`,
  consuming from it `queue_consumer`, HTTP sending `http_client`, a remote
  vendor client `sdk`. `other` does not rescue an unsupported relationship.
- `line`: a telegraphic note a newcomer reads in two seconds, at most ten
  words, no subject: "stores events in PostgreSQL", "signs users in through
  GitHub", "publishes catalog events to morfeu.events with confirmation".
  Never narrate the call ("through the codec", "reading the frame into the
  buffer"), never repeat the callable's name, never qualify inside the note;
  when an unknown matters, add " · not established: …" after it.
- `destination`, when requested: the `d*` ref of the runtime system the
  observations support, or `other: ` followed by the short name of a system
  the catalogue lacks (`other: Twilio`). A role is not a system: choose the
  system the evidence names, not "peer service". Never a hostname, URL,
  table, topic or configuration key.
- `basis`, when requested: `dispatch` when this call sends the exchange;
  `remote_client_instance` when this call itself creates or configures the
  actual remote client/exporter instance whose exchanges occur through its
  library. A configured relationship does not assert that the constructor
  itself sends the business payload. A function that only returns an option
  for a later constructor (WithBaseURL returning a client option) does not
  itself establish another relationship. Explain that configuration at the
  client constructor or actual exchange supported by the supplied
  observations, not as a separate contact.
- `address`, when requested: one `a*` ref only when that value identifies
  this destination. A supplied configuration expression is a valid source
  expression even though its deployed value is unknown. Use `unknown` when no
  listed value names the destination. A method name, payload, SQL text or
  arbitrary string is not an address. A literal from another call of the
  owner is usable only when the observations connect it to this
  client/configuration; lexical proximity alone does not connect values.

Only positive decisions need the other cells. Negative or unassessed rows
need no invented purpose, destination or address.

Read these distinctions semantically, not as name allowlists:

- An HTTP client's Do/RoundTrip performs exchange even though net/http belongs
  to the standard library. Creating a request alone does not establish sending.
- Creating a remote OTLP trace exporter with its endpoint option supports a
  configured trace-collector relationship. Formatting a span or writing metrics
  to stdout does not. A remote client constructor and local request builder are
  different observations, even if both are named New.
- A database query sent through a remote connection supports database exchange;
  constructing SQL, accessing an in-memory map or committing to an embedded
  local SQLite/bbolt store does not establish another runtime system.
- Publishing through a broker client is exchange; serialization, local channels
  and delivery to process-local watchers are local mechanisms.
- Sleeping, ticking, waiting, locking, logging and parsing YAML are local work.
  Their calls do not become relationships when the owning function is a worker.
- Registering an HTTP route/middleware or setting its documentation configures
  the local server. It is not an outgoing service call.
- A repository-local wrapper call does not expose its callee's implementation.
  `has_repository_callee_candidate` means at least one observed callee candidate
  is indexed in this repository; the call's original resolution still applies,
  and other candidates may remain external or unresolved. Do not treat a local
  bootstrap/helper as a remote SDK constructor because its name sounds like
  setup or export. An observed call to an indexed helper establishes internal
  delegation; its name or signature does not itself identify an external
  participant or its communication mechanism. Review the selected call's role,
  not the eventual effect of the whole chain. A local implementation whose OWN
  supplied call performs transport or configures a remote library client remains
  eligible at that transport/configuration call. Do not attribute that call's
  role to the wrapper that invokes its containing function.

Select only supplied refs. Keep conclusions recognizable as interpretations;
the source anchor and original observations remain the evidence.
