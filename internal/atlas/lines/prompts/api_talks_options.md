# What a call does with other running programs

Each option of the question "What does a call to `outside_symbol` do with
other running programs?" with its criteria: what it is, what it includes,
what it is not for, and examples. The request sends them as each option's
criteria.

## serves

What: The call is the program's own listening side: it makes the program reachable by other programs, or takes in a connection another program opened to it.

Includes: listening on an address, a port or a socket path; binding the listening socket to its address; running the application's server or serving loop; starting a consumer or worker that runs the handlers registered on it; accepting the next incoming connection that another program opened to a listening socket

Not for: opening a connection to another program, which is this program's own outgoing request; creating a socket or setting its options, which reaches no one yet

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

## none

What: The call talks to no other running program: it is the program's own work in its own process, with its own files, memory, threads, processes, signals and clock.

Includes: converting, parsing, formatting or validating values, an address, a port or a host name already in hand among them; building or configuring an object, a client, a request or a name without sending it; reading the body, the status, the fields or the id of a response, a result or a job an earlier call returned; creating a socket, setting its options, reading or writing a connection that is already open, closing one; handing a value to the host program or runtime this code runs inside; files, logging, memory, threads, processes, signals, time and random numbers

Not for: a call that itself sends to, reads from or opens a connection to another running program, or makes this program reachable by one

Examples:
- parsing an IP address from a string, or writing one back as text
- formatting a message into a buffer
- opening a local file for writing
- starting a thread or installing a signal handler
- setting an option on a socket
