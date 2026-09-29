# Name what outgoing calls reach

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its outside systems: the
servers it sends requests to, its databases, its message queues, the
storage and remote services it uses. Each system is one box.

Each row is one destination. The code followed the value that names what
each of its calls reaches, and all of them reach the same place: the same
server (one scheme and host, whatever the path), the same setting, or the
same value where the code stopped following it. Name that place once, for
all of its calls.

- `ends`: where that value ends: an `address` as written (a URL, a path,
  `{--name}` for a command-line option's value, `{env:KEY}` for an
  environment variable, `{expression}` for a part the code could not
  follow) or an `unresolved` expression, each with `written`, the code
  where it ends as the repository wrote it.
- `calls`: each call as the repository wrote it, with its `kind` (`db`,
  `client_request`: a request sent or a connection opened to another
  running service, `queue_producer`, `sdk`), the outside `package` it goes
  through, when the code names one, and the literal `values` it was given.
- `callers`: the functions making the calls, each with its file, its
  `signature` and the `calls` it makes beside them, as written. A function
  handed over to be called later says by whom and to which call
  (`handed_over`); a function handing one over says which and to which
  call (`hands_over`).
- `reached_from`: the functions of the program that reach the calls from
  outside the files they are written in.

Fill `destination` with the outside system these calls reach, the service
or program at the other end as a newcomer would name it.
`context.destination_catalog` lists, as `d*` refs, the systems the
program's outside packages reach, each with its `packages`. Choose the
entry that is the system these calls reach. When no entry is, write
`other: ` and that system's short name: name it by what it is to this
program, such as the server a client sends its commands to or the primary
a replica copies from. A package, a protocol, a host, a URL, a file path
or a key is not a system's name.
