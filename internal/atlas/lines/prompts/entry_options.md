# What an entry question's answer means

Each option of every question that asks what something of the repository
becomes on our map (the callable handed to an outside symbol, the words a
call to an outside symbol is given) with its criteria: what it is, what it
includes, what it is not for, and examples. Every such question sends the
same criteria for an option.

## request

What: Work another running program sends, where its connection or message first reaches this program, whatever the protocol.

Includes: the handler each new connection another program opens gets; a handler of one route, RPC method or protocol command a client names; a table row pairing such a command with its function

Not for: opening or binding the listening socket (none); reading from or writing to a connection an earlier entry accepted or this program opened (none); a command a person types (command); code run around every handler (middleware)

Examples:
- a handler registered for GET /users
- a table row naming a protocol command and the function that runs it
- a method of an RPC service registered with its server

## command

What: What a person gives the program when starting it from a command line or a task runner.

Includes: an option, a flag, a positional argument, a subcommand or a task; a word the program checks among its command-line arguments; the names of the commands among which the program looks up what was typed; declaring an option or a subcommand with a command-line parser; a subcommand's handler

Not for: the command line this program gives another program it starts, or the name of a program it looks up or starts (none); text the program prints, such as a usage line, a format or an error message (none); the program's own name (none); an environment variable (none); a command a client sends over a network connection (request)

Examples:
- declaring a --verbose flag with a parser
- a -p option whose value becomes the port
- a subcommand registered with a command-line parser

## interaction

What: A person's action in the program's user interface.

Includes: a button click, a key press, a menu choice, a form submission handled in the interface itself

Not for: an HTTP route a browser calls (request); an option given when starting the program (command)

Examples:
- a click handler of a button

## scheduled

What: A timer the program keeps for as long as it runs calls it again and again, or a scheduler runs it at set times.

Includes: a periodic job set up at start, a cron entry, an interval or a ticker callback

Not for: a one-shot delay a step of work sets, such as a retry, an idle timeout or a debounce (none); a hook the program's loop runs on every turn (none); work on a thread of its own for the program's whole life (continuous)

Examples:
- a housekeeping callback a timer calls every few milliseconds
- a job that a scheduler runs every night

## continuous

What: Runs for as long as the program does, on a thread, a task or a loop of its own.

Includes: the entry function of a thread or goroutine that serves or processes for the program's whole life, an event loop's worker

Not for: a callable the program's loop calls when a descriptor is ready or a timer fires; a thread that does one piece of work and ends; a callable run in place

Examples:
- the entry point of an I/O thread started when the server starts

## queue_consumer

What: Handles messages taken from a queue, a topic or a stream of a message broker, another running program.

Includes: a subscription handler of a broker's topic, a job handler of a job queue's worker

Not for: a listener on events the program's own code fires (none); putting messages on a queue (none)

Examples:
- a handler registered for the messages of a broker's topic

## extension

What: A host program outside the repository calls it at its own points: the repository plugs into the host.

Includes: registering a module or a plugin with a host runtime under a name, a hook or a selector a framework calls when it needs it

Not for: a registry or an event list of the program's own, which its own code fills and later looks up or fires (none); a callable the repository's own code runs in place (none)

Examples:
- registering a module with a host runtime under its import name
- a locale selector a web framework's extension calls

## middleware

What: Runs around or before the program's request handlers, for every request or a group of them.

Includes: authentication checks, request logging, a hook run before each request

Not for: a handler of one route or command (request)

Examples:
- a function that checks the session before every handler

## none

What: No entry: a step of work already under way, the program's own work, data or output, or work run only on a signal, at exit or on a failure.

Includes: reading from or writing to a connection an earlier entry accepted or the program opened; opening, binding or listening on a socket, or opening a connection; a hook the loop runs on every turn; a notice from the program's own threads; a listener on the program's own events; a signal, exit or failure handler; a one-shot delay; a comparator, a once-guard or a default the code it is handed to runs in place; text the program prints or logs; the program's own name; comparing, searching, converting or formatting strings; a table of names the program uses only internally

Not for: the ways work comes in from outside the program (request, command, interaction, scheduled, continuous, queue_consumer, extension)

Examples:
- a comparator passed to a sort
- a handler installed for a fatal signal
- a format string passed to a print function
- a hook the event loop runs before it waits again
