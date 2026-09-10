# Say what one box does to another

You receive a table of arrows between boxes on the map of a program. Each row
is one arrow the code drew from the program graph: the box it starts from
(title and one line), the box it points to (title and one line), up to three
witnesses (a caller in the first box and what it calls in the second), and
how many call sites there are.

Fill one cell for every row and nothing else:

- `sentence`: one sentence, at most 160 characters, saying what the first
  box does with the second, as a reader would say it out loud: "The run
  orchestration asks the report package to render the page." Name the
  boxes by what they do, not by their paths. State only what the witnesses
  show.

The result rows contain every supplied `key` exactly once and only the
columns advertised by `fill` for this request.

Rules:

- Every key from the request appears exactly once. Do not add, drop, rename
  or reorder keys, and do not add other fields.
- Write English, plain and specific. No paths, internal refs or Markdown in prose cells.
