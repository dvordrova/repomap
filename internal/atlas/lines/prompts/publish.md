# Say which holder a publishing call serves

Each row is a call that publishes: it starts a server, runs an application,
connects a consumer. The code could not follow the value it acts on back to
the value that holds the callables. The window's `holders` are those values,
each with what it holds. Choose the `h*` holder this call makes reachable, from
the row's `caller`, `path`, `values` and the holders' own sites; leave the cell
out when none of them is it.
