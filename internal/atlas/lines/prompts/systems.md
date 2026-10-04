# Which outside system a package reaches

We draw a map of a program for a newcomer who has never read its code.
Around the program's own parts the map shows its outside systems: the
servers it sends requests to, its databases, its message queues, the
storage and remote services it uses. The program reaches them through
packages written outside the repository.

Each row is one such package. `package` is its path as the code names it:
an import path, a module, a header or a namespace. `dependency`, when
present, is what the project's manifest records for it: the module and its
version. `calls` lists every symbol of the package the program calls, each
with every distinct call of it as the repository wrote it.
`imported_for_effect_by`, when present, lists the program's packages that
import this package for the effect of importing it, whatever else they use
of it: the package registers itself with another library when it loads, as
a database driver registers with a database library or a plugin with its
host. Such a package reaches the system it registers for: a database driver
reaches its database.

Fill `system` with the outside systems that calls through this package
reach, as a newcomer would name each:

- Name the system, not the library: a client library of a storage service
  is named by that service, a database driver by its database.
- Use the vendor's usual product name, in its usual spelling.
- One system has one name: write it the same way for every package that
  reaches it.
- When the calls choose the system, such as a driver name or a URL
  scheme given to them, name the system each call chooses. Calls that
  choose different systems reach each of them: write every one, separated
  by `; `. Never name the package after one of its calls when another
  chooses differently.
- Write `none` when the package reaches no one outside system: the
  operating system, the language runtime, a library that does its work
  inside the program, or a general protocol whose calls reach whatever
  address they are given.

Decide from the package and its calls alone.
