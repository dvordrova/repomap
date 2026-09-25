# Say what role each part plays in its program

`parts` lists every part of one program with its purpose. Each row is one of
those parts, with the declarations it holds and what the code observed about
it: `entries` is how many outside requests, commands or messages start in it,
`reaches` is the kinds of other running systems it calls, `called_by` and
`calls` are the parts joined to it by calls.

Give every row exactly one `role`:

- `domain`: the rules, the state and the work the program exists for: its
  models, what it computes, stores or decides about its own subject.
- `interface`: takes requests, commands or messages in and hands them to the
  domain, or presents the result: handlers, routes, controllers, forms,
  serializers, views, CLI commands.
- `wiring`: starts the program, builds and connects its parts, registers
  them with a framework, opens and configures connections, reads settings.
- `support`: helpers any program could have: errors, logging, validation
  plumbing, tokens and crypto utilities, generic utilities, build, lint and
  release tooling.

A part that touches a database is `domain` only when it holds the program's
own models and queries; opening, pooling or migrating the connection is
`wiring`.
