# What a function one statement starts on its own is on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start, and its settings, what a person
writes in its configuration file.

Each question gives one statement in the repository that starts one of the
repository's own functions to run on its own, beside the code that started
it: a Go `go` statement, or a coroutine handed to a call that runs it as a
task, such as asyncio.create_task. `statement` is the statement as the
repository wrote it, a function body written in it included; `in` the
declaration the statement is written in, with its signature; `starts` the
function it starts, with its signature; `calls` what that function calls
itself, by name, in the order it writes them; and `literals` the words the
started call is given, when it is given any. The same function can be
started for different work at different statements. The question asks what
the function this one statement starts is on our map; decide from this
statement alone.
