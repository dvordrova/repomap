# Read the external symbols the repository uses

Each row is one symbol outside the repository that the code calls: a
function, a method, a decorator. Decide what the symbol does with what the
repository gives it. Judge rows independently, from the row alone.

The row supplies `symbol` (package and name), `word` (the call as written),
`declared` (the symbol's type as its package declares it, when known),
`usage` (one line of the repository that calls it),
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
  deferred cleanup — binds nothing; neither does a decorator that only
  transforms what it decorates (`dataclass`, `lru_cache`, `property`).
- `publishes`: `yes` when the call makes what its holder holds reachable
  from outside: starts the server on the given address, runs the
  application, connects the consumer to its broker. A symbol that only
  configures or registers does not publish.
- `talks`: the kind of other running system this symbol sends to, reads
  from or creates a client of: `http_client`, `db`, `queue_producer`,
  `queue_consumer`, `sdk` for a remote API behind its own client, `other`.
  A library that works inside the process — parsing, logging, local files,
  time, an in-memory store — talks to nothing.

- `middleware`: `yes` when the callable handed over runs around or before
  the handlers (`Router.use`, a cors or body parser) rather than being an
  entry of its own. A middleware symbol binds no entry.
- `reads_input`: which part of a received request this symbol reads —
  `body`, `path`, `query` or `header` (`Context.Param`, `req.body`,
  `request.args`).
- `writes_output`: `yes` when this symbol writes the response a received
  request gets (`Context.JSON`, `res.status`, `jsonify`).
- `auth`: what the symbol does with credentials — `verifies` a token or a
  password, `issues` a token, `hashes` a secret.
- `config`: `reads` one configuration value (`os.Getenv`, `viper.Get`) or
  `loads` configuration from a source (`load_dotenv`, `ReadInConfig`).
- `validates`: `yes` when the symbol checks input against rules.
- `test`: `yes` when the symbol belongs to testing — a runner, an
  assertion, a mock, fake data. A testing symbol binds, publishes and talks
  to nothing the program serves.

One symbol may hold several cells: a call that takes a handler and an
address and serves it both binds and publishes.
