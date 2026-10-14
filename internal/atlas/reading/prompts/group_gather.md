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

Every group you name costs the newcomer one more box to open, so name a
group only when it is worth it: when its smaller boxes do one larger kind of
work that has a name of its own, a name that is not made of the boxes' own
names. "Order handling" over checkout, payment and shipping is such a group;
"Search and e-mails" over the product search and the e-mail sender is not:
it only joins two boxes' names and says nothing a newcomer did not already
read. A group named "A and B" after its boxes A and B is never a group. When
you hesitate, leave the boxes alone. Most boxes stay alone; naming no group
at all is a good answer.

Two boxes are never a group because they are what is left after the others
are grouped, or because they sit under one broad word (common, shared,
general, platform, core, support, other, miscellaneous). Do not put every
smaller box in one group. Put each smaller box in at most one group.

Give each group a name of two to four words, one sentence `holds` that names
the one larger kind of work its boxes share, and the refs of its smaller
boxes in `boxes`.
