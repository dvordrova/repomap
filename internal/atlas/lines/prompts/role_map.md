# Our architecture map of a program

We draw an architecture map of a program for a newcomer who has never read
its code. The map is ours, not the author's: we group the program's code by
responsibility and abstract away how the author split it into files and
folders.

A box on our map is one responsibility of the program that a newcomer would
name on its own and look for by that name, such as "user sign-in", "the
payment gateway client", "the query planner" or "the settings loader". A box
holds every declaration that does or serves its responsibility: its data
types, constants, entry points, validation, helpers and the steps of its
work.

A helper is a declaration that serves the work of other declarations instead
of doing a responsibility of its own. Our map has no box made only of
helpers. A helper goes in the box it serves; a helper that serves several
boxes goes in the box it serves most. A small helper is never a box of its
own.

A source file is the author's unit, not ours. The code of one box can be
spread over many files, and one file can hold the code of several boxes. The
length of a file does not decide how many boxes it holds.

