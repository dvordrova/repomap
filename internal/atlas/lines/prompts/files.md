# Describe the files of a repository

You receive a table of code files from one repository. Each row is one file:
its path, one line about the directory it lives in (`directory_hypothesis` is
an earlier model description; `directory_facts` is an extracted description),
facts about the files that call into it (`callers`: their paths and
`declarations`, the names of their declarations the graph saw making the
calls; a caller without that list is known by its path alone), and its
declarations (name, kind and signature). A declaration marked
`"anonymous": true` is a function literal written inside another declaration
and named after it: that declaration's code, not a declaration of its own. A
name's characters never say so: `price$1` unmarked is a declaration.

Fill every cell listed in the request's `fill` for every row. The base cells are:

- `line`: one sentence, at most 160 characters, saying what this file does.
  Read the declarations; use the directory line and caller
  facts to say what the file is for. State only what the row shows. Do not
  guess frameworks, protocols or behaviour the declarations do not mention.

Membership is decided separately from complete declarations and relationships.
A file or directory does not prescribe an architecture part.

When `fill` also lists `open`, that cell is mandatory: return `yes` when an
architecture reader should look inside this file, or `no` for vendored,
generated, test or trivial code. Use these string choices, not booleans. Omit
`open` only when it is absent from `fill`.

The result rows contain every supplied `key` exactly once and all the cells
listed in `fill`. Include `open` only when `fill` requests it.

Rules:

- Each row is independent. Use only that row and the explicit shared context;
  neighbouring rows are batching neighbours, not evidence about this file.

- Every key from the request appears exactly once. Do not add, drop, rename
  or reorder keys, omit requested cells, or add unrequested fields.
- Describe this file using its own declarations. Directory
  and caller context explain its surroundings; do not copy their responsibilities
  onto the file. A file containing one constant or data object should be described
  as that object, not as the surrounding module's behavior.
- `declaration_count` is the number of declarations, every one of them listed,
  the most telling first. An empty list means there is no declaration
  evidence in this row. Say the purpose is unclear if the file has no own evidence;
  do not fill that gap with the directory's description.
- Write English, plain and specific. No paths, internal refs or Markdown in prose cells.
