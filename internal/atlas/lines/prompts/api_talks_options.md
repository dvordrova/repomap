# What a call does with other running programs or with files

Each option of the question "What does a call to `outside_symbol` do with
other running programs or with files?" with its criteria: what it is, what it includes,
what it is not for, and examples. The request sends them as each option's
criteria.

## serves

What: The call is the program's own listening side: it makes the program reachable by other programs, or takes in a connection another program opened to it.

Includes: listening on an address, a port or a socket path, alone or with a handler or an application the repository hands it; binding the listening socket to its address; running the application's server or serving loop; starting a consumer or worker that runs the handlers registered on it; accepting the next incoming connection that another program opened to a listening socket

Not for: opening a connection to another program, which is this program's own outgoing request; creating a socket or setting its options, which reaches no one yet; registering a handler, a route or a command that something else serves later (none)

Examples:
- an HTTP server's listen-and-serve call
- binding a server socket to its port and listening on it
- taking the next client connection a listening socket has waiting
- running the web application's server

## client_request

What: The call sends a request to another running program, or opens a connection to one, over a network or a local socket, whatever the protocol.

Includes: an HTTP request, an RPC, connecting a socket to a remote address, dialing a server, sending a query to a name server the code names, a message on a WebSocket the program opened

Not for: accepting a connection another program opened to this program (serves); the client library of a database, a message queue or a remote service (db, queue_producer, queue_consumer, sdk); building a request, a URL or an address without sending it

Examples:
- posting a form to a web API
- connecting a TCP socket to a server's host and port
- dialing a UDP connection to a name server
- calling a method of an RPC client stub

## db

What: The call reads or writes a database, or states what the program asks of it: it runs a query or a statement, builds the query or statement a database library runs, opens a connection or a session to a database server, or changes its schema.

Includes: executing SQL, fetching rows, committing a transaction, building a SELECT, INSERT, UPDATE or DELETE with a database library, a migration step that creates or alters a table, a key-value server's get and set, connecting to a database

Not for: constructing or configuring a database client or engine without asking the database anything; a helper that only builds a name; a message queue (queue_producer, queue_consumer); reading a result already fetched; closing a connection

Examples:
- executing a SELECT and fetching the rows
- building the SELECT a session then runs
- adding a column in a migration
- opening a connection from a database engine
- setting a key on a cache server

## queue_producer

What: The call puts a message, an event or a job on a queue, a topic or a stream of a message broker or job queue, for another program to take.

Includes: publishing to a topic, enqueuing a background job, appending to a stream

Not for: building a queue object or a message without sending it; taking messages (queue_consumer)

Examples:
- enqueuing a job on a task queue
- publishing an event to a topic

## queue_consumer

What: The call takes messages, events or jobs from a queue, a topic or a stream of a message broker.

Includes: polling, fetching or receiving the next messages, subscribing to a topic so that its messages come back from this call

Not for: putting messages on a queue (queue_producer); starting a worker that runs the handlers registered on it (serves)

Examples:
- fetching the next batch of messages from a topic
- receiving from a subscription

## sdk

What: The call goes through a client library or a system facility that talks to another running service on the program's behalf and hides the connection.

Includes: a call of a cloud or vendor service's client, a search engine client's index or search, sending mail through a mail service, looking up the addresses of a host name through the system's resolver

Not for: converting an address or a name already in hand from one form to another, which asks no one; constructing or configuring the client without calling the service; HTTP, RPC or socket calls the code makes itself (client_request); databases and queues (db, queue_producer, queue_consumer)

Examples:
- uploading a file to a cloud storage bucket
- indexing a document in a search engine
- looking up the addresses of a host name

## runs_program

What: The call names another program, with its arguments, to run as a separate process: it starts that program, or builds the command a later call on its result starts.

Includes: running a command line through a shell; starting an executable given by its name or its path, with its arguments; replacing this process with another program; opening a pipe to or from a command it names; building a command from a program and its arguments

Not for: a call on a command an earlier call built and named, which starts it, waits for it, reads its output or connects its pipes without naming a program itself (none); starting a thread, a goroutine or a task inside this program (none); forking a copy of this program that goes on running its own code (none); evaluating or compiling code inside this program's own process (none); looking up where a program is installed without starting it (none); sending a request to a program that is already running (client_request)

Examples:
- running a version-control tool's command line and reading its output
- starting a compiler with the files it should build
- running a command line through the system shell
- building a command from a program name and its arguments, then running it

## file

What: The call reaches a file or a directory of the machine the program runs on by a path it is given: it opens, creates, reads or writes a whole file named by its path, renames, moves or removes one, or makes or lists a directory.

Includes: opening a file by its path for reading, writing or appending; creating, truncating or replacing a file; renaming, moving, linking or deleting a file or a directory; making a directory or listing one by its path; reading or writing a whole file named by its path in one call; mapping a file named by its path into memory

Not for: reading from, writing to, seeking in, flushing or closing a file already open, which names no path (none); building or joining a path without opening anything (none); a database file opened through a database library (db); a file on another machine reached through a remote service or a network protocol (sdk, client_request); a socket, a pipe or a device of this program's own process (none)

Examples:
- opening the configuration file by its path
- writing a snapshot to a temporary file, then renaming it into place
- removing a lock file
- creating the directory the program keeps its data in

## none

What: The call talks to no other program, starts none and reaches no file by its path: it is the program's own work in its own process, with its own memory, threads, signals and clock, and the files it already has open.

Includes: converting, parsing, formatting or validating values, an address, a port, a host name or a path already in hand among them; building or configuring an object, a client, a request or a name without sending it; reading the body, the status, the fields or the id of a response, a result or a job an earlier call returned; creating a socket, setting its options, reading or writing a connection that is already open, closing one; reading from, writing to or closing a file already open; handing a value to the host program or runtime this code runs inside; registering a handler, a route, a command or a hook that something else serves or runs later; running a callable it is handed in place; logging, memory, threads, signals, time and random numbers; forking a copy of this program that goes on running its own code; evaluating code inside this program's own process; starting, waiting for, reading the output of or stopping a command an earlier call built and named, when this call names no program itself

Not for: a call that itself sends to, reads from or opens a connection to another running program, or makes this program reachable by one; a call that starts another program, or names the program and the arguments of the command that starts it (runs_program); a call that opens, creates, renames or removes a file or a directory by its path (file)

Examples:
- parsing an IP address from a string, or writing one back as text
- formatting a message into a buffer
- writing a buffer to a file that is already open
- starting a thread or installing a signal handler
- setting an option on a socket
