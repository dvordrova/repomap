# Draw the parts of a program

A newcomer wants a coarse architecture map of a program: a few boxes they can
name and follow.

The input lists every unit of the program's code. A unit is a whole file, or
one box of a file whose code goes in several boxes of the map; a box carries
its name in "box". Each unit has a ref, its path, its number of declarations
(a function, a variable, or a type together with its methods), and the names
of its types, functions and variables; exported ones carry their signature.
"calls" counts calls between units: "f3 -> c7 (12)" means code in f3 calls
code in c7 twelve times. "imports" lists imports between whole files: "f3 ->
f7" means f3 imports f7.

Group the units into the parts a newcomer would draw on a coarse architecture
map. Put every unit in exactly one part.

For each part write a name of two to four words. Use only refs from the input.
