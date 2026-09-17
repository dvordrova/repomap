# What the repository's outside symbols do

Each row is one symbol outside the repository that the code calls. Decide
what the symbol does with what the repository gives it, from the row alone:
`symbol`, `declared` (its type as its package declares it), `usage` (one
line of the repository that calls it), `literals` the code gives it, and
`hands_callable` when the repository passes one of its own callables to it.

One row, one decision per cell. Fill only the cells that hold; a cell left
out means the symbol does not do that. Read the cell notes in `fill`.
