# The gate's options

Each option of the question "Does the code of `file` go in one box of our map,
or in several boxes?" with its criteria: what it is, what it includes, what
it is not for, and examples. The request sends them as each option's criteria.

## one box

What: All of the file's code does or serves one responsibility of the program, so the map draws the whole file inside one box.

Includes: that responsibility's data types, constants, entry points, options, validation, encoding and decoding, request building, output, helpers and the steps of its work, however many of them there are and even when each step has names of its own

Not for: a file that holds several responsibilities of the program, each one a newcomer would name and look for on its own

Examples:
- a priority queue: its type, push, pop, heapify and iteration
- a Markdown renderer: its tokens, block rules, inline rules and HTML output
- a rate limiter: its buckets, refill, checks and settings
- a client for one web service: its options, request building, retries, response decoding and errors

## several boxes

What: The file's code falls into two or more groups, each doing a different responsibility of the program that a newcomer would name and look for on its own, so the map draws each group as a box of its own.

Includes: groups that are separate jobs of the program, such as different features, commands, services or subsystems, that the rest of the program uses or reaches on their own

Not for: a file whose groups are only the data types, steps, stages, validation, encoding, decoding, options, output or helpers of one responsibility, even when each group has names of its own; a group that is only a helper or two

Examples:
- one file with the web request handlers, the database migrations and the e-mail sender
- a module with both the order API and the invoice PDF renderer
- a game file with the physics engine, the level loader and the sound mixer
