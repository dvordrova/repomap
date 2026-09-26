The input is one source file of the program whose code goes in several boxes
of our map: its path and its declarations. A declaration is a function, a
variable, or a type together with its methods; each has its kind, its name
and its signature, and "lines" is how many lines of code it has (a type
together with its methods). "calls" lists the declarations of the same file
it calls and "called_by" the declarations of the same file that call it;
"callers_elsewhere" is how many declarations in other files call it.

Name the boxes of our map that this file's code goes in, as a newcomer
reading the map needs them: as many boxes as the file really holds, and as
few as possible. Do not split one responsibility into its data types, steps,
validation or helpers. Do not merge responsibilities that a newcomer would
name and look for separately. Give each box a name of two to four words and
one sentence, `holds`, that says which of the file's code the box holds.
Every declaration should fit exactly one box. Do not list the declarations.
