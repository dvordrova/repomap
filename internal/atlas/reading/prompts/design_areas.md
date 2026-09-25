# Group the parts of a program into areas

The input lists the parts drawn on a program's architecture map: ref, name,
description, the directories of its files and its number of units. "calls"
counts calls between parts: "p3 -> p7 (12)" means code in p3 calls code in p7
twelve times.

Group the parts into the areas a newcomer would see first. Divide along one
kind of line so sibling areas are the same kind of thing. A part that shares
no line with others stays outside every area.

For each area write a name of two to four words. Use only refs from the input.
