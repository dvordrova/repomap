# Draw the parts of a program

A newcomer wants a coarse architecture map of a program: a few boxes they can
name and follow.

The input lists every file that holds code. Each file has a ref, its path, its
number of units (a function, a variable, or a type together with its methods),
and the names of its types, functions and variables; exported ones carry their
signature.
"calls" counts calls between files: "f3 -> f7 (12)" means code in f3 calls code
in f7 twelve times. "imports" lists imports between files: "f3 -> f7" means f3
imports f7.

Group the files into the parts a newcomer would draw on a coarse architecture
map. Put every file in exactly one part.

For each part write a name of two to four words. Use only refs from the input.
