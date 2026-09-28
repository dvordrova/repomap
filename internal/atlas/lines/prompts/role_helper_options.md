# The helper question's options

Each option of the question "What is `declaration` on our map: a helper, the
code of a responsibility, or none of these?" with its criteria: what it is,
what it includes, what it is not for, and examples. The request sends them as
each option's criteria. "none of these" has only what it is and what it is
not for.

## helper

What: The declaration serves the work of other declarations instead of doing a responsibility of its own: it does one small, general job for the code that calls it, and the job would read the same in any box that calls it.

Includes: writing output, replies or messages for its callers, writing a log line, allocating, copying or freeing memory, counting references, creating, comparing, converting, encoding or matching values, small lookups, checks and wrappers, and a table of constants or shared values that other code reads

Not for: a program entry point; code that receives, dispatches or runs the program's commands, requests, events or jobs; the handler of one command, request, event or message; a job, loop or timer the program runs; work a newcomer would name as something the program does, even when other code calls it; the types that hold a responsibility's state

Examples:
- a function that writes a formatted line to the log, called from many places
- a function that writes the JSON body of a response, called by every HTTP handler
- a function that trims whitespace from a string
- a wrapper around the memory allocator that counts the bytes it hands out
- a function that increments an object's reference count

## responsibility

What: The declaration does a responsibility of the program or one of its main steps: code a newcomer looks for when they want to see how the program does something.

Includes: program entry points; code that receives, dispatches or runs commands, requests, events or jobs; the handler of each command, request, event or message; jobs, loops and timers the program runs; the steps of one responsibility's work; the types, state and settings a responsibility owns

Not for: small, general code that serves the work of other declarations, such as writing output, logging, memory, reference counting, or converting, comparing or matching values

Examples:
- the program's main function
- the handler of the 'create order' request
- the loop that reads client input and runs its commands
- the function that exports a report to a PDF file
- the timer that runs the periodic clean-up
- the type that holds a shopping cart

## none of these

What: The declaration is neither a helper nor the code of a responsibility as the other options describe them.

Not for: a declaration that the helper or responsibility option describes
