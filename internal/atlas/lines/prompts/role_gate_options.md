# The gate's options

Each option of the question "Does the code of `file` go in one box of our map,
or in several boxes?" with its criteria: what it is, what it includes, what
it is not for, and examples. The request sends them as each option's criteria.

## one box

What: All of the file's code does or serves one responsibility, so the map draws the whole file inside one box.

Includes: that responsibility's data types, constants, entry points, validation, encoding, helpers and the steps of its work, however many of them there are

Not for: a file whose declarations form groups that do different responsibilities a newcomer would name and look for separately

Examples:
- a priority queue: its type, push, pop, heapify and iteration
- a Markdown renderer: its tokens, block rules, inline rules and HTML output
- a rate limiter: its buckets, refill, checks and settings

## several boxes

What: The file's code falls into two or more groups, each doing a different responsibility a newcomer would name and look for on its own, so the map draws each group as a box of its own.

Includes: groups with their own entry points and their own vocabulary of names, where a newcomer could understand one group without the others

Not for: a file whose groups are only the data types, steps, validation, encoding or helpers of one responsibility

Examples:
- one file with the web request handlers, the database migrations and the e-mail sender
- a module with both the order API and the invoice PDF renderer
- a game file with the physics engine, the level loader and the sound mixer
