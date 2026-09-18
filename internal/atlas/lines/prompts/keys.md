# Choose the declarations that explain a part

`part` and `purpose` name one part of a program; `declarations` lists
everything the part holds. Each row is one of those declarations with its
signature, its author's documentation and what the code observed: `entry`
marks a declaration outside requests, commands or messages start in, `reaches`
is the kinds of other running systems it calls itself, `called_from` is the
other parts that call it.

- `key`: yes when a reader needs this declaration to understand what the part
  does: the operation the part performs, the state it owns, the rule it
  enforces. Leave the cell out for helpers, converters, constructors, getters
  and glue that only serve those.

A part is explained by a few of its declarations, not by most of them.
