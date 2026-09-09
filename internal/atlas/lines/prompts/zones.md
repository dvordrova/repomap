# Name the parts of a program

You receive a table about the boxes on the map of one program target. A box
is a directory of code with a title and one line about it. The request's
`context` says which of three questions is asked:

- `names`: there is one row: the target's largest boxes, each with its
  title, one line and its size. Fill every `part_N` cell with the name of
  one part of the program, two to four words, so that every box listed
  belongs to one of the parts and each part holds several boxes. The
  number of cells is the number of parts asked for; all of them are
  filled, all distinct, none a copy of a single box's title unless that box
  is a part on its own. Name what a part does, not where it is. Return one
  row with key `r1`, containing every requested `part_N` cell together.
  A part name is a cell in that row, never a separate row.
- `assign`: the rows are boxes not yet placed. For each, write `part`: one
  of the names in `context.parts`, the part this box belongs to.
- `lines`: the rows are the parts themselves, each with the titles of the
  boxes it holds. For each, write `line`: one sentence, at most 160
  characters, saying what this part of the program does.

Return a JSON object with `rows`: one output object for each input row.
Each output object contains its original `key` and every column listed in
`fill`, with no other fields. The number of output rows comes from the
input rows, not from the number of cells or parts.

The modes have different cells. These examples illustrate the distinction;
use the actual keys and complete `fill` columns from your request.

For `names`, when `fill` asks for `part_1` and `part_2`, both names belong
inside the single `r1` object:

```json
{"rows":[{"key":"r1","part_1":"Report rendering","part_2":"Program indexing"}]}
```

For `assign`, when the input has two boxes and those names are offered in
`context.parts`, each box receives one `part` cell:

```json
{"rows":[{"key":"r1","part":"Report rendering"},{"key":"r2","part":"Program indexing"}]}
```

For `lines`, when the input has two parts, each part receives one `line`
cell:

```json
{"rows":[{"key":"r1","line":"Renders the repository report."},{"key":"r2","line":"Indexes program declarations and calls."}]}
```

Rules:

- Every key from the request appears exactly once. Do not add, drop, rename
  or reorder keys, and do not add other fields.
- Names are English, plain and specific: what the part does, not what the
  directory is called. No paths, no keys, no markdown.
- Return JSON only.
