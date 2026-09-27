# C adapter

The ordinary C adapter reads the C programs the repository's own build
describes, through `clang` run as a subprocess. It runs whenever the corpus
has a `.c` file outside the tooling directories;
`internal/run/repository_target_c.go` registers it with the selector prefix
`c:` and rank 4 after Go, Python, JS/TS and Clojure. Nothing in it knows a
particular project, framework or protocol.

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

The view is the host platform's build as the build description gives it (owner
decision D3): on macOS a Makefile that picks kqueue under `HAVE_KQUEUE` is
read with kqueue, and the epoll and select backends are outside this
platform's build. `Toolchain` records clang's version, target triple, sysroot,
resource directory, system include directories and the overrides, and the run
prints clang's version and target beside the build description and each
program's sources outside this platform's build beside the program. Each C
target's run records its view in that run's `metadata.json` as `c_platform`,
where a reader of the saved run finds it: clang's version line, target triple
and sysroot, the overrides every unit gets after its own flags (the fortify
override and the pre-C99 diagnostics), the build description's failure when
the units fell back to clang's defaults, each unit's kept and dropped flags
and whether the build or clang's defaults gave them, and the included sources
outside this platform's build (an epoll backend beside a kqueue build on
macOS). The page and the report JSON carry no label for it.

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
  (owner decision D1). Its registrar is the row's record type and the field
  the function is stored in (`kvd.h.kvCommand.proc`), which the registrar
  table asks once what its callables become: a command a client sends is a
  `request`. The entry is named by the word the model chooses among the row's
  own (`get`), like any entry of any protocol. A row that stores a second
  callable (`kvd.c`'s `{"get", getCommand, 2, preloadKey}`, Redis's
  `vm_preload_proc`) is still one registration, of the callable it writes
  first; `preloadKey` stays a callback the row stores and is no input named
  `get` (PROGRAM_INDEX). The literal is the row's name:
  `{"get", getCommand}` states no HTTP method, since nothing in the row is an
  address for a verb to qualify; a route row `{"GET", "/health", health}`
  states GET beside its path. A store into a platform record (`act.sa_sigaction`) is
  keyed on the root value's type and the field as written, not on the union a
  platform macro expands it to.
- A call through a field, parameter or variable has the stored functions as
  its targets. A store under a branch, or of a value the index cannot name,
  leaves the call unresolved with the candidates as witnesses; it never
  produces alternatives the program cannot reach. Each candidate witness
  names its function by identity as well as by words, so `loop.c`'s
  `fe->rfileProc(...)` reaches the map as possible arrows to `acceptHandler`,
  `readQueryFromClient` and `sendReplyToClient` (READING), and stays
  unresolved.
- A function cast to an integer is an address used as data, never a callable.
- Every active `#include` is one `imports` relation and one dependency: a
  repository header is a workspace dependency, a platform header the standard
  library, any other header a package.

String-literal arguments are literal patterns (clang joins adjacent
literals); loop context reuses the existing `control_context` words.

Every object with a located range carries `code_lines`, counted by the
adapter's own lexer: `//` and `/* */` comments are not code, a
backslash-newline continues a `//` comment, string and character literals are
code and end at an unescaped newline, and preprocessor lines are code; a
module counts its whole file. C has no in-file repeated declaration name for
the map of parts to merge: only a function's body is its declaration, a
typedef merges with its record, a tentative definition is one variable and an
`#if` alternative parses one branch. `kvd.c`'s `bgsaveCommand` (17 of its 18
lines; one comment line) asserts the count.

## What a program never runs

A C function runs only when code that runs names it: calls it directly, or
uses its address (stores, passes, returns or compares it, or casts it to an
integer that can be cast back). For a program with a `main`, the adapter
starts from `main`; from functions the runtime calls without a name in the
code (a `constructor` or `destructor` attribute); from every function a
file-scope initializer names, since those tables exist before `main` (Redis's
command table and `staticsymbols.h`'s integer casts); from an external
function whose name a platform or package header also declares, which that
library may call instead of its own; and from a name the C standard reserves
for the implementation (a file-scope name beginning with `_`). It follows
what each definition's body names, in every unit's copy of it (a header's
`static inline` function reaches each unit's own statics), including a
`cleanup` attribute's function. Assembly statements, file-scope assembly,
asm labels and `alias`/`ifunc` attributes name functions by text: every
repository function their written text, or its macro's, names is named there
(a call of `extern void f(void) __asm__("impl")` runs `impl`), and a function whose
own symbol an asm label renames is called by that symbol from outside, so it
is where running may start. A label a platform header writes names a
platform symbol and is not read. Each function
nothing reached is `unreachable` in that program's index: a deterministic
fact about that program as its link line (or, without one, the linker
closure) gives it, not about the file, so `anet.c`'s `anetTcpServer` and
`anetAccept` are unreachable in redis-cli and redis-benchmark and reached in
redis-server. An address taken only inside code that never runs is never
taken.

Code the adapter does not read can call any function with external linkage
by its name: a link input no compile line produced (`Missing`: a prebuilt
object or archive, an assembly or C++ source, a `.c` file outside the
corpus, a response file `@file`), a library other than the C runtime's own
(`-lc`, `-lm`, `-lpthread`, `-ldl`, `-lrt`; `-pthread` is a flag), a
framework, other programs calling a shared object (`-shared`) that also
defines `main`, and code the program loads or looks up at run time
(`dlopen`, `dlmopen`, `dlsym`, `dlvsym`, `dlfunc`). Then every external
function is where running may start, and only static functions are proven.
A link line that names another entry (`-e`, `--entry`, `-init`, `-fini`, a
linker script `-T`/`--script` with its `ENTRY`), defines one symbol as
another (`--defsym`, `--wrap`, Apple's `-alias`/`-alias_list`) or reads
linker options from a file (`-Wl,@file`), through `-Wl,` or `-Xlinker` too,
or drops the runtime's start files (`-nostartfiles`, `-nostdlib`), and
assembly text the adapter cannot read, prove nothing; a library (no `main`)
is called from outside and marks nothing.

The fixture's `net.c` is linked into kvd and kvcli like `anet.c`: kvd never
runs `netConnect`, kvcli never runs `netListen` (nor `strbuf.c`'s
`sbConsume`), and the backlog `netListen` reads (`KVD_BACKLOG`) is kvd's
setting alone (PROGRAM_INDEX, READING).

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

A licence, copyright or version-control stamp before the first line of code is
not a claim (owner decision D5), even when it also states the file's purpose.
Section banners, a decoration run such as `====` with a title of at most eight
words, are the author's layout and neither a claim nor structure (D4). So is
an undecorated title of one or two plain words on one line directly above a
declaration (`Implementation`, `Global vars`), unless one of its words names a
part of the declaration, ignoring case, C keywords, the header's own comments
and a plural s (`Timer events` above `struct timerEvent`); a longer comment, a
sentence and a comment written as code (`db->expires`) stay its docstring. A
comment a blank line separates from the declaration below it was never its
docstring. `NOTE:`, `WARNING:`, `IMPORTANT:` and `DEPRECATED` comment lines
are quoted from every other comment; comment markers inside strings and
character constants open nothing. C names follow the existing code-name
glossary rule (D6).

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
  and joints inside one target (D7, D8);
- a callable handed to a repository function that keeps it for its own loop
  (`loopCreateFileEvent(..., acceptHandler, ...)`, Redis's
  `aeCreateTimeEvent(..., serverCron, ...)`): the constitution defines a
  registration as a call outside the repository, and D1 excepts only table
  rows, so an accept handler or a timer registered with the repository's own
  event loop is no input until the owner decides otherwise.

The fixture has no HTTP route table: a row whose verb qualifies the path beside
it is covered by the facts tests alone, beside the Go-shaped record
`{Method: "GET", Path: "/users", Handler: h}` of an outside type, which the Go
fixture cannot build without a module dependency. Python's `methods=["GET"]`
and JavaScript's `{method: "GET"}` are no literals in the index and state no
method.

## Environment

Install clang on the normal PATH: on macOS the Command Line Tools
(`xcode-select --install`), on Linux the distribution's `clang` package.
`make` is needed only for a repository with a makefile.

## Verification

`testdata/repositories/c` is the cumulative executable repository and
`testdata/contracts/c.files.json` binds its exact inventory. The run tests
cover discovery from link lines, repomap's own repository offering no C target
and running no tool for its fixture, tooling sources beside a program, another
adapter's explicit target, the files that restore each program, one parse per
plan for a shared unit and its release after the last projection that needs it,
a backend outside this platform's build, a missing clang, an ordinary
offline run selecting a C program, whose metadata records the platform view and
whose page shows none of it, and kvcli's page listing `netListen` and
`sbConsume` under "Not reachable from the entrypoints". Facts tests cover C
config reads, SQL, dynamic execution, command rows that state no HTTP method
and route rows that state one, each with its record field as registrar.
`TestIndexProvesWhatAProgramNeverRuns` checks each way a function is named
and every unit's copy of a shared static;
`TestIndexProvesOnlyStaticsWhereOtherCodeCanCallByName` checks run-time
loading, link lines (entries, linker scripts, symbol definitions, response
files, shared objects) and libraries;
`TestCFixtureProvesWhatEachProgramNeverRuns` checks the fixture's programs,
their facts and places, and
`TestCFixturePresetReadingKeepsSharedSocketsWithTheProgramThatRunsThem` reads
kvd and kvcli together: the listener stays kvd's, the connect kvcli's. The kvd
preset reading
(`TestCFixturePresetReadingTurnsTableRowsIntoNamedRequests`) answers each
registrar and each entry from its row alone, without captions, and checks the
inputs: six requests named `get`, `set`, `del`, `keys`, `ping` and `bgsave`,
the stats thread as continuous work, and neither the signal handler nor the
`qsort` comparator; claims, places and report tests cover docstrings, licence blocks, banners,
section titles and the file description at their consuming boundaries, and the
fixture's own docstrings reach its declarations. The fixture's map of parts is
checked like every other language's (`TestCumulativeCMapOfParts`).
