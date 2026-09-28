# What an answer to "which word names the program a call starts" means

Every word the call is given is offered as an option with the criteria of
`word`; `not_named` is the one other option.

## word

What: This word, as the call writes it, names the program the call starts: its name, its path, or a command line that begins with it.

Includes: an executable's name or its path; the command a shell, an interpreter, a package runner, a launcher or a privilege tool is told to run, rather than that shell, interpreter, runner or launcher; a whole command line written as one word

Not for: an option, a flag or an argument the started program is given; a shell, an interpreter, a package runner, a launcher or a privilege tool when the call also names the command it runs (that command's word)

Examples:
- the name of the version-control tool a command line starts with
- the command line a shell is given after -c
- the module an interpreter is told to run as a program

## not_named

What: No word the call is given names the program it starts: the program comes from a variable, a setting, a lookup or this program's input.

Includes: a program path held in a variable or found at run time; a command line built from values; a call whose words are only options and arguments

Not for: a program the call writes as a word, even with options and arguments beside it (that word)

Examples:
- starting a program whose path a setting holds
- running a compiler found on the search path earlier
