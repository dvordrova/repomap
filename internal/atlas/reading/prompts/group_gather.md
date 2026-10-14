# Our architecture map of a program

We draw an architecture map of a program for a newcomer who has never read
its code. The map is ours, not the author's: we group the program's code by
the work it does, top down, and a newcomer opens it one box at a time.

The input is one box of our map that has already been divided into smaller
boxes: its name, the boxes it sits in (`inside`, outermost first) and its
smaller boxes, each with a `ref`, a `name`, `holds` (the code it holds) and
how many declarations it has.

A newcomer who opens this box sees all of its smaller boxes side by side, and
too many at once to take in. Find the smaller boxes that sit together in a
larger part of this box: the steps, partners or variants of one larger kind
of work, which a newcomer would explain in the same breath. Each larger part
stands between this box and its smaller boxes; opening it shows its smaller
boxes as they are.

A smaller box that sits with no other stays alone: leave it out of every
group. Most boxes may stay alone. Two boxes are never a group because they
are what is left after the others are grouped. A group is named by the work
its boxes do together, never by being common, shared, general, other or
miscellaneous. Do not put every smaller box in one group. Put each smaller
box in at most one group.

Give each group a name of two to four words, one sentence `holds` that names
the one larger kind of work its boxes share, and the refs of its smaller
boxes in `boxes`. When the only way to say what a group holds is to list
each box's own work ("A and B"), its boxes share no work: leave them alone.
