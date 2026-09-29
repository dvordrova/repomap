# What the outside symbols a program calls are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start, and its settings, what a person
writes in its configuration file. It also shows its outside systems:
the other running programs it talks to, such as the servers it sends
requests to, its databases, its message queues and the remote services it
uses, and the programs it starts; and the files it reads and writes by
their paths. The operating system, the language runtime and the libraries
the program links are not outside systems: calling them is the program's
own work in its own process.

Each question gives one symbol from outside the repository that the code
calls or hands something to, or the field a row of one of the repository's
own tables stores a callable in, named by the file that declares the row's
record type, the type and the field. `symbol` is its name, `declared` its
type as its package declares it, `usage` one call of it as the repository
wrote it, or the row or statement that stores such a callable, `literals`
the words the code gives it, `result_receives` the calls the code makes on
what a call to it returns, each with how many times, and `hands_callable`
holds when the repository passes one of its own callables, or a value it
built, to it. Every symbol is asked what a call to it does with other
running programs or with files; a symbol handed a callable is also asked
what that callable becomes. Decide from the symbol and its call alone.
