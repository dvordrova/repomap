# Explain a native observation

Each row is one fact the code already established; its existence and kind
are not decisions. `kind_given` says what it is:

- `config`, a configuration read, and `listen_address`, an address the
  program listens on;
- an entry kind (`request`, `command`, `interaction`, `scheduled`,
  `continuous`, `queue_consumer`, `extension`): work a registration brings
  in;
- `client_request` (a request sent, or a connection opened, to another
  running service, whatever the protocol), `db`, `queue_producer` or `sdk`:
  work sent out.

`path`, `line`, `caller`, `external` and `values` are the fact's source
site, enclosing declaration or handler, called symbol and observed
literals. An outgoing row may carry the `method` its call states and the
outside `package` its call goes through. An entry's `words` are what its
registration wrote, as written: the call word, its literals and the address
its mounts compose. `context.owners` holds the declaration once, with the
calls near the fact, and `owner_ref` names it. A neighbouring row is a
batching neighbour, not evidence.

Fill only the columns in `fill`:

- `line`: at most ten words, no subject, what this observation reads,
  receives or sends: "reads the broker URL from BROKER_URL", "receives level
  requests at /levels", "fetches partner prices". Describe the observation,
  not the whole function; never repeat the callable's name; a configuration
  read is an input, not a remote exchange.
- `name`, when requested: the `w*` refs from `words` that name this entry
  the way whoever sends it names it, in the order they are read. A route is
  named by its verb and its path (`w1 w2` for `GET` and `/users/:id`), a
  command by the command a client sends, a message handler by its topic or
  event, an RPC by its service and method. Choose the address with its mounts
  over the bare literal it was composed from. Leave out a word that only
  names the registering call or the record type, and answer `none` when no
  word names the entry.
- `address`, when requested: where the other end of this call is, as one
  `a*` ref from `address_catalog`, else `unknown`. The catalogue holds every
  value the code was seen to hand the call, and `destination_chains` where
  each came from: each place the value ends once, with its shortest chain
  and, when several reach it, the number of `routes`; a value is not an
  address because it is offered.
  - Answer the `a*` of a value that writes where the call connects, of the
    kind the call reaches: a URL whose host is written, whatever placeholders
    follow the host in its path or query (`https://login.example.com/%s/token`,
    `https://api.example.com/v2/me?token=%s`); a host
    and port, a socket path or a database file the call opens; a connection
    string that writes its server; a setting that holds the address, as the
    code writes it (`{env:API_URL}`, `{--socket}`) or as the key its chain's
    configuration read is given. When several values do, the call reaches
    each of them: answer the first.
  - Answer `unknown` when no value does. A value that writes no host, port,
    socket or file path and is no such setting says nowhere: a bare word or
    identifier (a field, a table or a key), `/`, or a mode with no place
    such as an in-memory database (`:memory:`). Nor does a template whose
    host or server is itself a placeholder (`%s:%d`, `app:%s@tcp(%s:%d)/%s`,
    `https://%s/api/users`), which shows how the address is built, not where
    it is; a key, a name or a path inside the destination (an object or a
    file key, a table, an RPC method, a URL path without its host); SQL, a
    query, a message or request body, a JSON document or another payload;
    logging, timing or formatting configuration; a value of another kind
    than the call reaches (a web URL handed to a database driver).
