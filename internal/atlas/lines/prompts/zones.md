# Name the parts of a program

You receive a table about the boxes on the map of one program target. A box
is a directory of code with a title and one line about it. The request's
`context` says which of three questions is asked:

- `names`: there is one row: the target's largest boxes, each with its
  title, one line and its size. Fill every `part_N` cell with the name of
  one part of the program, two to four words, so that every box listed
  belongs to one of the parts; a part may hold one box. The number of
  cells is the number of parts asked for; all of them are filled, all
  distinct, none a copy of a single box's title unless that box is a part
  on its own. Name what a part does, not where it is. Return one row with
  key `r1`, containing every requested `part_N` cell together. A part name
  is a cell in that row, never a separate row.
- `assign`: the rows are boxes not yet placed. For each, write `part`: one
  of the names in `context.parts`, the part this box belongs to.
- `lines`: the rows are the parts themselves, each with the titles of the
  boxes it holds. For each, write `line`: one sentence, at most 160
  characters, saying what this part of the program does.

The result rows contain one object for each input row.
Each output object contains its original `key` and every column listed in
`fill`, with no other fields. The number of output rows comes from the
input rows, not from the number of cells or parts.

Rules:

- Every key from the request appears exactly once. Do not add, drop, rename
  or reorder keys, and do not add other fields.
- Names are English, plain and specific: what the part does, not what the
  directory is called. No paths, internal refs or Markdown in prose cells.
