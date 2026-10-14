# The grouping question's options

Each option of the question "Does a newcomer read `box` as it is, or does it
need smaller boxes inside?" with its criteria: what it is, what it includes,
what it is not for, and examples. The request sends them as each option's
criteria.

## grouped enough

What: All of the box's declarations do or serve one responsibility a newcomer names and looks for as one thing, so they read the box as it is.

Includes: that responsibility's data types, constants, entry points, validation, encoding and decoding, helpers and the steps of its work, however many of them there are and even when each step has a name of its own

Not for: a box that holds several responsibilities a newcomer would name and look for on their own; a library whose list holds its data types with their options, the policies a caller chooses among and the engine that runs them, each a thing a newcomer looks up on its own

Examples:
- a priority queue: its type, push, pop, heapify and iteration
- the handlers of one HTTP resource, such as creating, reading, updating and deleting an order

## needs smaller boxes

What: The box's declarations fall into two or more groups, each doing a different responsibility a newcomer would name and look for on its own.

Includes: groups that are separate features, commands, services, subsystems or layers of the program; a library's data types and their options, the policies a caller chooses among, and the engine that runs them

Not for: a box whose groups are only the data types, steps, validation, encoding, options, output or helpers of one responsibility

Examples:
- web request handlers, database migrations and an e-mail sender
- the order API and the invoice PDF renderer
- a physics engine, a level loader and a sound mixer
- an HTTP client library's request options, its retry policies and its connection pool
