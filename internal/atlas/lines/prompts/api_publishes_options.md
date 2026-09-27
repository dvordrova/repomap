# Whether a call starts serving what it is handed

Each option of the question "Does this call to `outside_symbol` make what the
repository hands it reachable by other programs?" with its criteria: what it
is, what it includes, what it is not for, and examples. The request sends
them as each option's criteria.

## serves

What: The call starts serving what it is handed: other programs can reach it from then on.

Includes: listening on an address or a port with the handed handler or application, running the application's server, starting the consumer or worker that runs the handed handlers

Not for: registering a handler, a route or a command that something else serves later; running the callable in place

Examples:
- listen-and-serve given an address and the program's router
- running a server with the program's application object

## none

What: The call does not start serving: it registers, runs, stores or configures what it is handed, and serving starts elsewhere or never.

Includes: registering a route, a command, a module or a hook; running a callable in place; starting a thread

Not for: a call that itself listens or runs the server with what it is handed

Examples:
- registering a handler for one route
- starting a background thread
