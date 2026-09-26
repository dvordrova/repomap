# What the symbols the repository hands things to do

Each row is one symbol the code calls or hands something to: a symbol outside
the repository, or the field a row of one of the repository's own tables
stores a callable in, named by the file that declares the row's record type,
the type and the field. Decide what the symbol does with what the repository
gives it, from the row alone: `symbol`, `declared` (its type as its package
declares it), `usage` (one line of the repository that calls it or writes
such a row), `literals` the code gives it, and `hands_callable` when the
repository passes one of its own callables to it.

One row, one decision per cell. Fill only the cells that hold; a cell left
out means the symbol does not do that. Read the cell notes in `fill`.
