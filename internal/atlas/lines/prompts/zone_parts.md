# Place functions and types in the parts of a program

`context.parts` lists the parts a reader would draw on an architecture map of
this program, each with a title and one sentence of purpose. Each row is one
function or one type of the program: its package, name, kind, signature or
methods, the first sentence of its author's documentation, and the other
functions and types it calls.

- `part`: the ref of the one part whose purpose this row serves. Use the
  documentation, the name and what it calls together; the package is a hint,
  not the answer. Write `none` when no listed part fits.
