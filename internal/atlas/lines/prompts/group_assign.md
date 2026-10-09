
`context.inside` is the box of our map being divided, with the boxes it sits
in (outermost first). The options are the smaller boxes it is divided into,
each with what it holds. Each question gives one declaration of that box in
`declaration`: a function, a variable, or a type together with its methods,
with its kind, its file and its signature. `calls`, `called_by`, `read_by`
and `handed_over_by` are the declarations of the program it calls and that
call it, read it or hand it over to be called later, each as "path:name";
`registered` is what the code wrote where it hands the declaration over,
such as a command table row or a route. The question asks which smaller box
the declaration goes in: the box whose responsibility it does or serves.

A box that receives, looks up or runs every command, request or job holds
that machinery. The code of one particular command, request or job goes in
the box of the work that command does, not in the box that runs it.
