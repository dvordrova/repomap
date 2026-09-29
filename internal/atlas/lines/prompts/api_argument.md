# Which argument names what a call reaches

We draw a map of a program for a newcomer who has never read its code.
Beside the program's own parts the map shows its outside systems, the other
running programs it talks to, and the files it reads and writes, each with
where the code says it is: an address, a host, a bucket, a key, a
database, a queue or a file's path. The code follows a value back through
the program's own functions to the words and settings it comes from; it
needs to know which value of a call to follow.

Each question gives one symbol from outside the repository that the
program calls. `symbol` is its name, `declared` its type as its package
declares it, `usage` one call of it as the repository wrote it, and
`talks` what a call to it does with other running programs or with files.
`arguments` are the call's arguments, each by its position or its keyword
and, when the declared type gives one, its parameter's name, and the
receiver the call is made on, each with where that call's value comes from
(a word, a parameter of a declaration, a field or an element of another
value, what a call returns, one of several values, or code that was not
followed). When `result_given_to` is there, the symbol reaches nothing
itself: what a call to it returns is given to those calls where they name
what they reach, so the question asks which of its own arguments that value
carries on. The question asks which one argument, or the receiver, names
the address or the file; decide from the symbol and its call alone.
