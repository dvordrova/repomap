# What a declaration on the way does

Each row is one declaration on the way from an entry of the program to a
call that leaves it, shown as its source. Judge from the source alone.

`role`: `adapter` changes the shape of what passes through (converts
between types, maps rows to records); `logic` decides (validates, branches
on the data, combines calls, applies a rule); `passthrough` only forwards
to the next declaration and returns what it gets. When more than one
holds, the first in this list wins.
