# Say which parts a program exists for

`parts` lists every part of one program with its purpose. Each row is one of
those parts, with the declarations it holds and what the code observed about
it: `entries` is how many outside requests, commands or messages start in it,
`reaches` is the kinds of other running systems it calls, `called_by` and
`calls` are the parts joined to it by calls.

Fill a cell only when it is true; leave it out otherwise.

- `core`: yes when the program exists for what this part does. Its domain
  rules, the state it owns and the operations a user came for are core. What
  wires the other parts together, connects the program, configures it, logs,
  shapes errors or is shared plumbing is not core, however many parts call it.
- `for_tests`: yes when the part exists only so the program's tests can run:
  mocks, fixtures, helpers that build test requests or inspect results.

A program usually exists for a few of its parts, not for most of them.
