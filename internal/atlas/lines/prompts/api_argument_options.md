# What an answer to "which argument names what a call reaches" means

Every argument of the call, and its receiver, is offered as an option with
the criteria of `argument`; `none` is the one other option.

## argument

What: This argument, or the receiver the call is made on, is the value that names where the call reaches: the address, the host, the bucket, the key, the database, the queue, the topic or the file's path, or a value built from it that carries it.

Includes: a URL, a host and port or a socket path; a request, a configuration or an options value that holds the address; a file's or a directory's path; the handle of a bucket, an object, a database or a client that an earlier call opened on the address, when the call is made on it; the name of a queue, a topic or an object the call reaches

Not for: a context, a timeout, a flag, a mode, a buffer, a length, a body or data the call sends or writes; a callback; a value that only says how the call is made, not where to

Examples:
- the URL of a request that sends it
- the request a client sends, built earlier with its URL
- the path of a file a call opens or removes
- the object handle an upload is made on
- the name of the topic a message is published to

## none

What: No argument and no receiver of the call names where it reaches: it goes on with a connection, a file or a client already open without naming it, or names its destination nowhere in its call.

Includes: a call on a descriptor or a stream whose value says nothing of its address; a call whose arguments are only data, sizes, modes or options that name no destination

Not for: a call made on a handle, a client or a request that an earlier call built with its address (that receiver or argument)

Examples:
- writing a buffer to an already open descriptor
- closing a connection
