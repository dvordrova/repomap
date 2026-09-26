# C adapter

The ordinary C adapter reads the C programs the repository's own build
describes, through `clang` run as a subprocess. It runs whenever the corpus has
a `.c` file outside the tooling directories; `internal/run/repository_target_c.go` registers it with the
selector prefix `c:` and rank 4 after Go, Python, JS/TS and Clojure. Nothing in
it knows a particular project, framework or protocol.

## Build description and targets

A `.c` file under a tooling directory (`.claude`, `.github`, `.vscode`,
`testdata`: `corpus.ToolingPath`) is an input of the tests or tools around it
and never a unit: a corpus whose only `.c` files are there runs no C
discovery, no dry run and no clang probe, and beside other sources no clang
default unit is made of one. An explicit `--target` whose every selector
another adapter owns runs no C discovery either.

`internal/cproject` takes compile and link lines from the first of:

1. a root `compile_commands.json`;
2. a dry run of the root `GNUmakefile`, `makefile` or `Makefile`:
   `make -n -B -w -o <makefile>` on the default goal, with `LC_ALL=C`, the
   caller's `MAKEFLAGS` and related variables removed, and a 30 s limit that
   stops make's whole process group;
3. clang's defaults for every `.c` file no other file includes.

A failed dry run is recorded (`c_build_error`) and the units fall back to (3);
a unit that then fails to parse reports both errors. `make -n` is not a pure
dry run: GNU make still runs `$(shell ...)`, recipes that start with `+` or
call `$(MAKE)`, and rules that remake the makefiles it reads, and `-B` makes
all of them out of date. `-o <makefile>` keeps the root makefile as it is;
makefiles it includes are still remade. The selected repository is trusted, so
this is recorded rather than prevented. macOS ships GNU make 3.81.

Each link line is one program (`c:<output>`, anchored on its makefile rule)
whose files are the units it links; a `-shared`/`-dynamiclib` line is a shared
library. Compile lines without `-o` map `x.o` to `x.c` through make's working
directory. Main is located when the program is parsed, not during discovery.
A unit no link line links (every unit without a build description, and with
a `compile_commands.json`, which has no link lines) that has an exact,
non-static `main` with a body (a filtered clang parse, since
`-ast-dump-filter` matches substrings) is a program `c:<path>` whose files the
linker closure decides: each unresolved external name goes to the one unit
that defines it. Units no program links and that define no main are their
directory's library (`c:<dir>/`). A `.c` file another file `#include`s belongs
to its includer; one no parsed unit enters on this platform (an `#ifdef` chose
another backend) is outside this platform's build, and the run prints it
beside its program when the program is parsed. Shared units stay complete in
every program that links them.

Discovery offers the target portfolio the files only one program compiles
(a program whose every file is shared offers its link line's file); every
unit of a program and its link-line file restore it. Native evidence rows are
`c_link` (the makefile line, `fields.output`, `values` = linked files),
`c_main` and `c_library`; the portfolio prompt defines them.

## Native tooling and platform view

Compile flags pass an allowlist of what changes the parse: defines and
undefines, include paths (`-I`, `-isystem`, `-iquote`, `-idirafter`,
`-include`, `-imacros`), the language standard, sysroot and target, one
`-arch`, `-pthread` and language `-f`/`-m` flags. Warnings, optimisation,
debug, dependency output and flags only gcc accepts are dropped and recorded
per unit. After the build's flags every unit gets `-U_FORTIFY_SOURCE
-D_FORTIFY_SOURCE=0`, so libc calls keep their written names (the macOS SDK
fortifies at every optimisation level), and the diagnostics clang 16 and later
make errors for pre-C99 code stay warnings that `-w` silences.

Each unit runs `clang -fsyntax-only -w -H -Xclang -ast-dump=json` once per run
store, at most four at a time; a unit several programs link is parsed once.
The plan's store keeps a decoded unit only while a planned program that has
not been projected yet still links it: once a program is projected, or its
parse fails, the units no remaining program needs are released, so decoded
units do not stay alive through the pages' model work. The dump is decoded as
it streams, one top-level declaration at a time. clang leaves out `file` and
`line` when they have not changed, so that state is replayed through every
location in document order, including inside dropped system declarations:
spelling precedes expansion, and `includedFrom` and presumed locations never
move it. Only declarations whose expansion location is a corpus file are kept.
`-H` gives the active include tree.

The view is the host platform's build as the build description gives it
(owner decision D3): on macOS a Makefile that picks kqueue under
`HAVE_KQUEUE` is read with kqueue, and the epoll and select backends are
outside this platform's build. `Toolchain` records clang's version, target
triple, sysroot, resource directory, system include directories and the
overrides, and the run prints clang's version and target beside the build
description. The page carries no label for it.

The clang resource directory and the sysroot's headers and frameworks are the
platform; any other include directory (`/usr/local/include`,
`/opt/homebrew/include`) is a package. A platform or package declaration is
named by the first header on its `-H` chain that a corpus file includes, as
that directive spells it: `qsort` declared in `_stdlib.h` is `stdlib.h`.

Without clang the C targets are not analyzed, with the reason that a required
tool is unavailable (`cproject.ErrClangUnavailable`). A unit that exits
non-zero or prints `error:` fails every program that links it; there is no
partial graph, and targets of other adapters are unaffected.

## ProgramIndex shape

The projection follows the linker. A name with external linkage is one object
per program; two external definitions of one name fail the program. A static
function, variable or type is identified by its definition location, so a
header's `static inline` function or a `.c` file several units include is one
object, and same-named statics in different units stay apart. An anonymous
`typedef struct {...} Name` is the type `Name`.

Objects are a module per file, functions at their definitions (prototypes are
declaration witnesses; `static` is internal visibility), a type per struct,
union and enum with fields and enum constants contained by it, and file-scope
variables. Signatures are clang's native text; parameter and result type ids
follow pointers and qualifiers to repository types. `main` is the callable
seed.

- A direct call is `calls`, exact after resolving names across units. A call
  of a platform or package function is `invokes_external`; `__builtin_*` calls
  belong to the platform.
- A call written in a macro argument keeps its own spelling location; a call
  the macro body writes is sited at the macro use, with the outermost macro
  name as written as its selector and the body's spelling as a
  `macro_expansion` witness.
- A function passed as an argument is `passes_callback` bound to that argument.
- A function stored into a field, variable or array (initializer rows,
  assignments, and a parameter a function stores joined with what its callers
  pass) is `passes_callback` with a primary `c_function_pointer_store` witness,
  so each store is a binding row. A row of a repository-owned table that names
  the function by a string literal is a registration fact the model classifies
  (owner decision D1). The literal is the row's name: `{"get", getCommand}`
  states no HTTP method, since nothing in the row is an address for a verb
  to qualify. A store into a platform record (`act.sa_sigaction`) is
  keyed on the root value's type and the field as written, not on the union a
  platform macro expands it to.
- A call through a field, parameter or variable has the stored functions as
  its targets. A store under a branch, or of a value the index cannot name,
  leaves the call unresolved with the candidates as witnesses; it never
  produces alternatives the program cannot reach.
- A function cast to an integer is an address used as data, never a callable.
- Every active `#include` is one `imports` relation and one dependency: a
  repository header is a workspace dependency, a platform header the standard
  library, any other header a package.

String-literal arguments are literal patterns (clang joins adjacent
literals); loop context reuses the existing `control_context` words.

## Facts and claims

Config reads, SQL statements and registrations are the language-neutral facts
over those patterns: `getenv("KEY")` is a config read, an SQL literal passed to
a library call is an SQL statement, and an external call that receives a
repository function (`pthread_create`, `qsort`, `signal`) is a registration.
In a C file, dynamic execution is `system` from `stdlib.h` and `popen` from
`stdio.h` only: C has no builtin that runs code, so a repository function
named `eval` or `exec` is the repository's own code. The database extractor
does not read `.c` files, so C SQL literals give no table entities.

C comments enter the ordinary claims layer. A `/* */` block or a run of `//`
lines that ends directly above a top-level declaration (a line starting at
column 0 with a name) is that declaration's docstring. Its declaration line is
where the declaration ends its header (`static int` / `foo(void)` ends on
`foo`'s line). Places and the page's symbol cards read it through one rule,
`claims.CDocstring`: it describes only the declaration located on that line or
up to two lines above it (an Allman brace), so a comment above a prototype
describes no neighbouring declaration. A comment before the file's first line
of code that no declaration follows directly is the file's description, which
describes no declaration; places shows it as the file's own documentation.

A licence, copyright or version-control stamp before the first line of code
is not a claim (owner decision D5), even when it also states the file's
purpose. Section banners, a decoration run such as `====` with a title of at
most eight words, are the author's layout and neither a claim nor structure
(D4). So is an undecorated title of one or two plain words on one line
directly above a declaration (`/* Implementation */`, `/* Global vars */`),
unless one of its words names a part of the declaration, ignoring case, C
keywords, the header's own comments and a plural s (`/* Timer events */`
above `struct timerEvent`); a longer comment, a sentence and a comment
written as code (`/* db->expires */`) stay its docstring. A comment a blank
line separates from the declaration below it was never its docstring. `NOTE:`, `WARNING:`, `IMPORTANT:` and `DEPRECATED` comment lines are
quoted from every other comment; comment markers inside strings and character
constants open nothing. C names follow the existing code-name glossary rule
(D6).

## Test sources and missing equivalents

C has no standard test runner, so Target.TestSources stays empty and a test
program is an ordinary program the model may place as a tool. Out of scope
for this first adapter:

- macros as declarations or named constants, and macro uses that are not
  calls;
- preprocessor views other than the host's (no cross sysroot or platform
  flag);
- C++, Objective-C and assembly; generating `compile_commands.json` from CMake
  or autotools;
- callbacks handed from one parameter to another;
- code that runs only in a forked child, and communication decided by where a
  file descriptor came from;
- Makefile rules as run recipes, test scripts in other languages, and
  configuration files the corpus does not admit;
- parts smaller than one file (D2), protocol commands as their own entry kind
  and joints inside one target (D7, D8).

## Environment

Install clang on the normal PATH: on macOS the Command Line Tools
(`xcode-select --install`), on Linux the distribution's `clang` package.
`make` is needed only for a repository with a makefile.

## Verification

`testdata/repositories/c` is the cumulative executable repository and
`testdata/contracts/c.files.json` binds its exact inventory. The run tests
cover discovery from link lines, repomap's own repository offering no C
target and running no tool for its fixture, tooling sources beside a program,
another adapter's explicit target, the files that restore each program, one
parse per plan for a shared unit and its release after the last projection
that needs it, a backend outside this platform's build, a missing clang, and
an ordinary offline run selecting a C program. Facts tests cover C config
reads, SQL, dynamic execution and command rows that state no HTTP method;
claims, places and report tests cover docstrings, licence blocks, banners,
section titles and the file description at their consuming boundaries, and
the fixture's own docstrings reach its declarations. The fixture's map of
parts is checked like every other language's (`TestCumulativeCMapOfParts`).
