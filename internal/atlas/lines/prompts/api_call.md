# What the words one call gives an outside symbol are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start. It also shows its outside systems:
the other running programs it talks to, and the programs it starts. The
operating system, the language runtime and the libraries the program links
are not outside systems: calling them is the program's own work in its own
process.

Each question gives one call to a symbol from outside the repository whose
words mean different things at different calls, such as a string
comparison. `symbol` is the symbol's name, `declared` its type as its
package declares it, `call` the call as the repository wrote it, `in` the
declaration the call is written in, with its signature, `called_by` the
declarations that call that one, `literals` the words this call gives,
`arguments` where each argument comes from (its kind and the code that
makes it), and `level` whether the call runs while the program starts,
reached from its entry, inside the code of an entry already found, or
neither. The question asks what the words this one call gives are on our
map; decide from this call alone.
