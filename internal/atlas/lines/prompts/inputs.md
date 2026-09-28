# What the repository's own registrations and tables are on our map

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its entries, the ways work
comes in to this program: what other programs send to it, what people run
or click, what timers and threads start, and its settings, what a person
writes in its configuration file. The operating system, the
language runtime and the libraries the program links are not outside
systems: calling them is the program's own work in its own process.

Each question gives one thing of the repository's own that may be an
entry. `candidate` is either a callable the repository hands to one of its
own functions that keeps it for later, or a table of names the repository
declares.

For a kept callable: `callable` is its name and signature, `kept_by` the
repository function it is handed to, with that function's signature and
the fields or variables it keeps the callable in, `call` the call as the
repository wrote it, `literals` the words that call gives, and `during`
what runs when the call is made: from the program's start through the
named declarations, or while callables another registration hands over
run.

For a table: `table` is its name, `declared` its type, `rows` every row's
words as the repository wrote them, and `read_by` the declarations that
read the table.

The question asks what the candidate is on our map; decide from it alone.
