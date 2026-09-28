# Which word names the program a call starts

We draw a map of a program for a newcomer who has never read its code.
Among its outside systems the map shows the other programs it starts as
separate processes, each named by the word the code wrote for it, as
written. Each question gives one call that starts another program, or
builds the command a later call starts: `symbol` is the outside symbol
called, `usage` the call as the repository wrote it, and `words` the words
the call is given, each once, in the order the code writes them. The
question asks which word names the program that does the work: through a
shell, an interpreter, a package runner, a launcher or a privilege tool,
that is the command it is told to run, not the shell, interpreter, runner
or launcher itself. When no word names it, because the program comes from a
variable, a setting, a lookup or the program's input, the answer says so.
