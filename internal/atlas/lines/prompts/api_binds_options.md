# What a handed callable becomes

Each option of the question "What does the repository's callable handed to
`outside_symbol` become on our map?" with its criteria: what it is, what it
includes, what it is not for, and examples. The request sends them as each
option's criteria.

## request

What: The callable handles what a client sends to this program over a connection, whatever the protocol.

Includes: a route of a web server, an RPC method, a command a client sends over a network connection, an event a socket receives, a row of a table pairing such a command's name with its function

Not for: code that runs around or before every handler (middleware); a command a person types at a command line (command)

Examples:
- registering a handler for GET /users
- a table row naming a protocol command and the function that runs it
- a method of an RPC service registered with the server

## command

What: A person runs the callable from a command line or a task runner.

Includes: a subcommand of a command-line tool, the arguments and options declared on such a subcommand, a task a task runner names

Not for: a command a client sends over a network connection (request)

Examples:
- a subcommand registered with a command-line parser
- declaring an argument of a command-line command

## interaction

What: The callable handles a person's action in a user interface.

Includes: a button click, a key press, a menu choice, a form submission handled in the interface itself

Not for: an HTTP route a browser calls (request)

Examples:
- a click handler of a button

## scheduled

What: A timer or a scheduler runs the callable at set times or after a delay.

Includes: a periodic job, a cron entry, a timeout or interval callback

Not for: work that runs for as long as the program does (continuous)

Examples:
- a job that a scheduler runs every night

## continuous

What: The callable runs for as long as the program does, on a thread, a task or a loop of its own.

Includes: the entry function of a thread or goroutine that serves or processes for the program's whole life, an event loop's worker

Not for: a thread that does one piece of work and ends; a callable run in place

Examples:
- the entry point of an I/O thread started when the server starts

## queue_consumer

What: The callable handles messages taken from a queue, a topic or a stream of a message broker.

Includes: a subscription handler, a job handler of a job queue's worker

Not for: a callable that puts messages on a queue

Examples:
- a handler registered for the messages of a topic

## extension

What: A host program calls the callable, or the value the repository built, at its own points: the repository plugs into the host.

Includes: registering a module or plugin with a host runtime under a name, a hook or selector a framework calls when it needs it

Not for: a callable the repository's own code runs in place

Examples:
- registering a module with a host runtime under its import name
- a locale selector a web framework's extension calls

## middleware

What: The callable runs around or before the program's request handlers, for every request or a group of them.

Includes: authentication checks, request logging, a hook run before each request

Not for: a handler of one route or command (request)

Examples:
- a function that checks the session before every handler

## none

What: The callable is no entry of the program: the symbol runs it in place, wraps, marks or stores it, or runs it only when the process is signalled or fails.

Includes: a comparator a sort calls, a function a once-guard runs, a column or field default, a signal handler, a callback run only on failure, a configuration call given a value

Not for: a callable that a client, a person, a timer, a thread, a queue or a host program starts

Examples:
- a comparator passed to a sort
- a handler installed for a fatal signal
- a function run once under a lock
