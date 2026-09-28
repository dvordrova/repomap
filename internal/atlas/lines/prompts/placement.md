# Place a unit in one part of a program

`parts` lists the parts drawn on a program's architecture map: ref, name and
the directories of their files. Each row is one unit of the program's code: a
whole file, or one box of a file whose code goes in several boxes of the map,
which carries its name in `box`. A row has its path, its number of
declarations (a function, a variable, or a type together with its methods)
and the names of its types, functions and variables; exported ones carry their
signature. `calls` counts its calls to and from the parts: "-> p3 (4)" means
code in this unit calls code in p3 four times, "p5 -> (2)" means code in p5
calls this unit twice. `imports` lists, for a whole file, the parts it
imports and the parts that import it the same way.

For each row choose the one part in `part_options` this unit belongs to.
