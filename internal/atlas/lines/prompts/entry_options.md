# What an entry question's answer means

Each option of every question that asks what something of the repository
becomes on our map (the callable handed to an outside symbol, the words a
call to an outside symbol is given, the key a field's tag names) with its
criteria: what it is, what it includes, what it is not for, and examples.
Every such question sends the same criteria for an option.

## request

What: Work another running program sends, where its connection or message first reaches this program, whatever the protocol.

Includes: the handler each new connection another program opens gets, such as the callable the program's own event loop runs when its listening socket has a connection to accept; a handler of one route, RPC method or protocol command a client names; a table row pairing such a command with its function; a message, a command or a button press a person makes in another program's interface, which that program delivers to this one over a connection

Not for: opening or binding the listening socket (none); reading from or writing to a connection an earlier entry accepted or this program opened (none); what this program sends to another program, such as the names of the commands it sends, even when it looks them up in a table of its own first (command when a person types them to this program); a command a person types to this program (command); code run around every handler (middleware); a parameter's description, help or usage text (none); a character or prefix a word is tested to start with (none)

Examples:
- a handler registered for GET /users
- a table row naming a protocol command and the function that runs it
- a method of an RPC service registered with its server

## command

What: What a person types or passes to the program: when starting it from a command line or a task runner, or at its own prompt while it runs.

Includes: an option, a flag, a positional argument, a subcommand or a task; a word the program checks among its command-line arguments; the names of the commands among which the program looks up what was typed; declaring an option or a subcommand with a command-line parser; a subcommand's handler; the commands a person types into a client program, which it looks up, checks and sends on to another program: they are that client's commands

Not for: the command line this program gives another program it starts, or the name of a program it looks up or starts (none); the name a parser or an option set is given for its own messages (none); text the program prints, such as a usage line, a format or an error message (none); a parameter's description, help or usage text (none); a character or prefix a word is tested to start with, such as the dash options begin with (none); the program's own name (none); an environment variable (none); a command another program sends to this one over a network connection (request); a directive or key a person writes in the program's configuration file (setting)

Examples:
- declaring a --verbose flag with a parser
- a -p option whose value becomes the port
- a subcommand registered with a command-line parser

## setting

What: What a person writes in the program's own configuration file to change how it runs.

Includes: a directive or a key name the program looks for in a configuration file it reads, such as a word it compares with the first word of each line of that file; a key a structure the program decodes its configuration file into maps to one of its fields, such as the key a field's tag names or a schema's key; a configuration entry declared under its key with a settings facility

Not for: an option, a flag or a word given on the command line (command); an environment variable the program reads (none: reading it is already on the map as a configuration read beside the settings, while a setting is a key a person writes in a file); a key of data the program merely parses, stores or sends, such as a field of a message another program sends or receives, a record it saves or a file format it converts, which is not the program's own configuration (none); a word a client sends over a connection to read or change a setting while the program runs (none: it belongs to the request that carries it); a value a key may take: a word the program compares with what follows a key once it found the key, such as a later word of a configuration line whose first word named the key (none: the value belongs to its key, which is the setting)

Examples:
- a key the program compares with the first word of each line of the configuration file it loads
- a structure field whose tag names the key it is read from in the configuration file
- a default declared for a named configuration key

## interaction

What: A person's action in a window, a page or a screen this program itself draws.

Includes: a button click, a key press, a menu choice, a form submission handled in the interface this program draws

Not for: an HTTP route a browser calls (request); an option given when starting the program (command); a press, a command or a message a person makes in another program's interface, which that program delivers to this one over a connection (request); a button, a keyboard or a menu this program puts into a message it sends to be shown by another program (none)

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

Includes: reading from or writing to a connection an earlier entry accepted or the program opened; opening, binding or listening on a socket, or opening a connection; a hook the loop runs on every turn; a notice from the program's own threads; a listener on the program's own events; a signal, exit or failure handler; a one-shot delay; a comparator, a once-guard or a default the code it is handed to runs in place; text the program prints or logs; the program's own name; comparing, searching, converting or formatting strings; a table of names the program uses only internally; a value a key of the configuration file may take, such as a word compared with a later word of a line whose first word is the key; a button, a keyboard or a menu this program builds into a message it sends

Not for: the ways work comes in from outside the program (request, command, interaction, scheduled, continuous, queue_consumer, extension) and what a person writes in its configuration file (setting)

Examples:
- a comparator passed to a sort
- a handler installed for a fatal signal
- a format string passed to a print function
- a hook the event loop runs before it waits again
