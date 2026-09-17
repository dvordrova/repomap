# Say what each declaration on a chain does

Each row is one declaration on the way from an entry of the program (a
request handler, a command, a consumer) to a call that leaves the program (a
query, a request, a message). The row shows its `source` as written, the
declarations `before` it on that way and the declarations `after` it. Judge
rows independently, from the source alone.

Choose `role`:

- `access`: the declaration itself makes the call that leaves the program —
  runs the statement, sends the request, publishes the message.
- `adapter`: it changes the shape of what passes through — converts between
  types, maps rows to records, wraps or unwraps a protocol.
- `logic`: it decides — validates, branches on the data, combines several
  calls, applies a rule of the program.
- `passthrough`: it only forwards to the next declaration with the same
  arguments and returns what it gets.
