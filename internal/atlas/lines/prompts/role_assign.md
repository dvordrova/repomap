
`context.file` is one source file of the program whose code goes in several
boxes of our map. Each question gives one declaration of that file in
`declaration`: a function, a variable, or a type together with its methods,
with its kind, its name and its signature. `calls` and `called_by` are the
declarations of the same file it calls and that call it; `calls_elsewhere`
is what it calls in other files, as "path:name". The options are the boxes
this file's code goes in, each with what it holds. The question asks which
box the declaration goes in: the box whose responsibility it does or serves.
