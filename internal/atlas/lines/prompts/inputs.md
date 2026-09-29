# What the repository's own registrations, tables and tagged fields are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start, and its settings, what a person
writes in its configuration file. The operating system, the
language runtime and the libraries the program links are not outside
systems: calling them is the program's own work in its own process.

Each question gives one thing of the repository's own that may be an
entry or a setting. `candidate` is a callable the repository hands to one
of its own functions that keeps it for later, a table of names the
repository declares, or a field of one of the repository's structures whose
tag names a key.

For a kept callable: `callable` is its name and signature, `kept_by` the
repository function it is handed to, with that function's signature and
the fields or variables it keeps the callable in, `call` the call as the
repository wrote it, `literals` the words that call gives, and `during`
what runs when the call is made: from the program's start through the
named declarations, or while callables another registration hands over
run.

For a table: `table` is its name, `declared` its type, `file` the file
declaring it, `rows` every row's
words as the repository wrote them, and `read_by` each declaration that
reads the table: its name and signature, `reads`, each line that reads the
table as the repository wrote it, and `called_by`, the declarations that
call it, each with its signature and the lines calling the reader as the
repository wrote them. How the table is read, and what its reader's
callers hand it, say what its rows are.

For a field: `field` is its name and type, `structure` the structure
declaring it with its file, `tag` the field's tag as written (each key the
tag names, after the name of the format it is for), and `structure_use`
what the code does with the structure where the index shows it: the calls
to symbols from outside the repository given a value of it, with the
declaration making each call and the call as written, and the tagged
fields of other structures whose type it is.

The question asks what the candidate is on our map; decide from it alone.
