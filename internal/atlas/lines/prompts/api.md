# Read the external symbols the repository uses

Each row is one symbol outside the repository that the code calls: a
function, a method, a decorator. Decide what the symbol does with what the
repository gives it. Judge rows independently, from the row alone.

The row supplies `symbol` (package and name), `word` (the call as written),
`hands_callable` when the repository passes one of its own callables to
this symbol, `literals` the code gives it, `sites` how many places call it,
and `beside`: other symbols called on the same value, such as `Start`
beside `GET` on one router.

Fill only the cells that hold; leave a cell out when the symbol does not do
that.

- `binds`: what a repository callable handed to this symbol becomes.
  `http_server` a handler of received requests on the given path;
  `queue_consumer` a handler of messages from a topic, queue or
  subscription; `scheduled` work a timer, cron or scheduler activates;
  `interaction` a handler of a user's action in an interface; `extension` a
  hook, plugin or module registered with a host runtime; `other` an
  established entry of another kind. A symbol that runs the callable in
  place — a sort comparator, a map function, a middleware wrapper, a
  deferred cleanup — binds nothing.
- `publishes`: `yes` when the call makes what its holder holds reachable
  from outside: starts the server on the given address, runs the
  application, connects the consumer to its broker. A symbol that only
  configures or registers does not publish.
- `talks`: the kind of other running system this symbol sends to, reads
  from or creates a client of: `http_client`, `db`, `queue_producer`,
  `queue_consumer`, `sdk` for a remote API behind its own client, `other`.
  A library that works inside the process — parsing, logging, local files,
  time, an in-memory store — talks to nothing.

One symbol may hold several cells: a call that takes a handler and an
address and serves it both binds and publishes.
