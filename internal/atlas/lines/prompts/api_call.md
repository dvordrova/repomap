# What the words one call gives an outside symbol are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start. It also shows its outside systems:
the other running programs it talks to, and the programs it starts. The
operating system, the language runtime and the libraries the program links
are not outside systems: calling them is the program's own work in its own
process.

Each question gives one call the repository makes to a symbol from outside
the repository, a call that gives the symbol at least one word. `symbol`
is the symbol's name, `declared` its type as its package declares it,
`call` the call as the repository wrote it, `in` the declaration the call
is written in, with its signature, `literals` every word this call gives,
`arguments` where each argument of the call comes from as the code records
it (a word, a parameter of a declaration, a field or an element of another
value, what a call returns, or code that was not followed), and `talks`,
when already decided, what a call to the symbol does with other running
programs. One symbol's words can mean different things at different calls.
The question asks what the words this one call gives are on our map;
decide from this call alone.
