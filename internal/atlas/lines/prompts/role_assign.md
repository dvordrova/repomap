
`context.file` is one source file of the program whose code goes in several
boxes of our map. Each question gives one declaration of that file in
`declaration`: a function, a variable, or a type together with its methods,
with its kind, its name and its signature. `calls` and `called_by` are the
declarations of the same file it calls and that call it; `calls_elsewhere`
is what it calls in other files, as "path:name". The options are the boxes
this file's code goes in, each with what it holds. The question asks which
box the declaration goes in: the box whose responsibility it does or serves.

A box that receives, looks up or runs every command, request or job of the
program holds that machinery. The code of one particular command, request or
job goes in the box of the work that command does, not in the box that runs
it.

`registered` is what the code wrote where it hands the declaration over to be
called later, such as a command table row or a route: the words it wrote
there.
