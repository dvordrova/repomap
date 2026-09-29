# What the words one value is compared with are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start, and its settings, what a person
writes in its configuration file. It also shows its outside systems:
the other running programs it talks to, and the programs it starts. The
operating system, the language runtime and the libraries the program links
are not outside systems: calling them is the program's own work in its own
process.

Each question gives one place where the repository's own code compares
one value with several words and runs different code for each: the cases
of a switch, match or case statement on the value, or a chain of if
statements comparing the same value with a word. `compares` is the value
as the repository wrote it, `from` where that value comes from as the code
records it (a word, a parameter of a declaration, a field or an element of
another value, what a call returns, one of several values, or code that was
not followed), `in` the declaration the comparison is written in, with its
signature, and `cases` the words of each case in the order they are
written; a case listing several words runs the same code for each of them.
The question asks what the words this value is compared with are on our
map; decide from this comparison alone.
