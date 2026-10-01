# Describe the key symbols of a repository

You receive declarations already selected for the repository overview.
Each row is one declaration:
its name, kind and signature, one line about the file it lives in, and how
many callers it has in the program graph.

Fill the cells advertised by fill:

- `line`: one sentence, at most 120 characters, saying what this
  declaration does or is. Read the name and the signature and say what they
  show, no more.
- `alias`: asked only for a name that is not written in Latin letters: a
  short English reader label, at most 40 characters, alongside the original
  name. Base it on the supplied declaration; do not invent
  behaviour or expand an unexplained acronym. Translate the supported meaning,
  not just the sound of the name. Write `none` if the evidence does not
  establish a useful alias. This label does not rename code.

`calls`, when present, are neutral extracted observations: call names, literal
values, callback argument names and source lines. `local_calls` names exact
repository callees as `name@line` with the control statements holding the
call. Interpret them regardless of framework or language. They are not a
complete function body or execution trace.
A function returning a command/router object constructs an operation; the
callback doing its work implements it. Describe only the supplied declaration. Never invent a call edge.

The result rows contain every supplied `key` exactly once and only the
columns advertised by `fill` for this request.

Rules:

- Each row is independent. Use only that row and the explicit shared context;
  neighbouring rows are batching neighbours, not evidence about this declaration.
- `file_hypothesis` is a previous model interpretation of the file, not a
  source fact or proof of this declaration's implementation. No function body
  was supplied. Describe only what the declaration supports.

- Every key from the request appears exactly once. Do not add, drop, rename
  or reorder keys, and do not add other fields.
- Write English, plain and specific. No paths, internal refs or Markdown in prose cells.
